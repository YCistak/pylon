// Package daemon implements the long-running Pylon process: it owns the Unix
// socket, dispatches CLI requests, and shuts down cleanly on SIGTERM/SIGINT.
package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/ipc"
)

// Daemon is the core Pylon process.
type Daemon struct {
	socketPath string
	pidPath    string
	log        *slog.Logger
	db         *db.DB
	startedAt  time.Time

	ln       net.Listener
	cancel   context.CancelFunc // cancels the run context (set in Run)
	wg       sync.WaitGroup
	mu       sync.Mutex // guards handlers
	hnd      map[string]Handler
	services []service
}

// Handler processes a single request and returns a response.
type Handler func(req ipc.Request) ipc.Response

// service is a long-running background task tied to the daemon lifecycle.
type service struct {
	name string
	run  func(context.Context) error
}

// Options configures a Daemon.
type Options struct {
	SocketPath string // defaults to ipc.DefaultSocketPath()
	PIDPath    string // defaults to ipc.DefaultPIDPath()
	Logger     *slog.Logger
	DB         *db.DB // optional persistence handle for handlers
}

// New constructs a Daemon. It does not touch the filesystem or network yet;
// call Run to start listening.
func New(opts Options) *Daemon {
	if opts.SocketPath == "" {
		opts.SocketPath = ipc.DefaultSocketPath()
	}
	if opts.PIDPath == "" {
		opts.PIDPath = ipc.DefaultPIDPath()
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	d := &Daemon{
		socketPath: opts.SocketPath,
		pidPath:    opts.PIDPath,
		log:        opts.Logger,
		db:         opts.DB,
		hnd:        make(map[string]Handler),
	}
	d.registerBuiltins()
	return d
}

// Handle registers a handler for a command name, overriding any existing one.
func (d *Daemon) Handle(cmd string, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hnd[cmd] = h
}

// Register adds a background service that runs under the daemon's context for
// the lifetime of Run. Call before Run. The service should return when its
// context is cancelled (on stop / signal).
func (d *Daemon) Register(name string, run func(context.Context) error) {
	d.services = append(d.services, service{name: name, run: run})
}

// registerBuiltins wires up the commands every daemon must answer.
func (d *Daemon) registerBuiltins() {
	d.Handle("ping", func(ipc.Request) ipc.Response {
		return ipc.Response{OK: true, Text: "pong"}
	})
	d.Handle("status", func(ipc.Request) ipc.Response {
		up := time.Since(d.startedAt).Round(time.Second)
		text := fmt.Sprintf("running (pid %d, uptime %s)", os.Getpid(), up)
		if d.db != nil {
			if pending, err := d.db.PendingTasks(); err == nil {
				text += fmt.Sprintf(", %d pending task(s)", len(pending))
			}
		}
		return ipc.Response{OK: true, Text: text}
	})
	d.Handle("stop", func(ipc.Request) ipc.Response {
		// Acknowledge first, then cancel the run context: that closes the
		// listener and stops every background service uniformly.
		go func() {
			time.Sleep(50 * time.Millisecond)
			if d.cancel != nil {
				d.cancel()
			}
		}()
		return ipc.Response{OK: true, Text: "stopping"}
	})
}

// Run starts the daemon: writes the PID file, opens the socket, and serves
// requests until ctx is cancelled or a shutdown signal arrives. It blocks.
func (d *Daemon) Run(ctx context.Context) error {
	// Neither the PID write nor AF_UNIX creates its parent directory, and the
	// PID file goes first — so both directories have to exist before anything
	// else runs. On Unix this never showed, the defaults living in /tmp; on
	// Windows they sit under %LocalAppData%\pylon, absent until a first run.
	if err := ensureDirs(d.pidPath, d.socketPath); err != nil {
		return err
	}

	if err := d.writePIDFile(); err != nil {
		return err
	}
	defer d.removePIDFile()

	// Remove a stale socket from an unclean previous shutdown.
	if err := d.clearStaleSocket(); err != nil {
		return err
	}

	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.socketPath, err)
	}
	// Connecting to a Unix socket needs write permission on it, so its mode is
	// the last gate — and it was whatever the umask happened to leave.
	if err := secureSocket(d.socketPath); err != nil {
		_ = ln.Close()
		return fmt.Errorf("secure %s: %w", d.socketPath, err)
	}
	d.ln = ln
	d.startedAt = time.Now()
	d.log.Info("pylon daemon started", "socket", d.socketPath, "pid", os.Getpid())

	// One cancellable context drives shutdown: signals, an explicit stop
	// command (via d.cancel), or serve() returning all converge here.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	defer cancel()

	go func() {
		<-ctx.Done()
		d.log.Info("shutdown initiated")
		_ = d.ln.Close()
	}()

	d.startServices(ctx)

	d.serve()
	cancel() // serve() returned (e.g. listener closed) → stop services too

	d.wg.Wait()
	_ = os.Remove(d.socketPath)
	d.log.Info("pylon daemon stopped")
	return nil
}

// startServices launches every registered background service under ctx.
func (d *Daemon) startServices(ctx context.Context) {
	for _, svc := range d.services {
		d.wg.Add(1)
		go func(s service) {
			defer d.wg.Done()
			d.log.Info("service started", "name", s.name)
			if err := s.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				d.log.Warn("service exited", "name", s.name, "err", err)
			}
		}(svc)
	}
}

// serve accepts connections until the listener is closed.
func (d *Daemon) serve() {
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			d.log.Warn("accept failed", "err", err)
			continue
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConn(conn)
		}()
	}
}

// handleConn reads one request, dispatches it, and writes the response.
func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	var req ipc.Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		d.writeResponse(conn, ipc.Response{OK: false, Error: "bad request: " + err.Error()})
		return
	}

	d.mu.Lock()
	h, ok := d.hnd[req.Cmd]
	d.mu.Unlock()
	if !ok {
		d.writeResponse(conn, ipc.Response{OK: false, Error: "unknown command: " + req.Cmd})
		return
	}
	d.writeResponse(conn, h(req))
}

func (d *Daemon) writeResponse(conn net.Conn, resp ipc.Response) {
	b, err := json.Marshal(resp)
	if err != nil {
		d.log.Error("marshal response", "err", err)
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(append(b, '\n')); err != nil {
		d.log.Warn("write response", "err", err)
	}
}
