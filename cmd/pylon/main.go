// Command pylon is the CLI and daemon entry point for the Pylon assistant.
//
// Usage:
//
//	pylon start    run the daemon (foreground)
//	pylon stop     ask a running daemon to shut down
//	pylon status   report whether the daemon is running
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/YCistak/pylon/internal/config"
	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/ipc"
	"github.com/YCistak/pylon/internal/watcher"
)

// configPath is the path to pylon.yaml, overridable via PYLON_CONFIG.
func configPath() string {
	if p := os.Getenv("PYLON_CONFIG"); p != "" {
		return p
	}
	return "pylon.yaml"
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "start":
		err = cmdStart()
	case "stop":
		err = cmdStop()
	case "status":
		err = cmdStatus()
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pylon — personal AI assistant daemon

usage:
  pylon start    run the daemon (foreground)
  pylon stop     stop a running daemon
  pylon status   show daemon status
`)
}

// cmdStart loads config, opens the database, and runs the daemon in the
// foreground until interrupted.
func cmdStart() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	database, err := db.Open(cfg.Paths.DB)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()
	log.Info("database ready", "path", cfg.Paths.DB)

	d := daemon.New(daemon.Options{
		SocketPath: cfg.Paths.Socket,
		PIDPath:    cfg.Paths.PID,
		Logger:     log,
		DB:         database,
	})

	registerWatcher(d, cfg, database, log)

	return d.Run(context.Background())
}

// registerWatcher wires a process watcher into the daemon: when a watched
// process with tasks_on_exit set exits, its pending tasks are pulled from the
// queue and (for now) logged — TTS read-aloud lands with the voice module.
func registerWatcher(d *daemon.Daemon, cfg config.Config, database *db.DB, log *slog.Logger) {
	var names []string
	onExit := make(map[string]bool)
	for _, p := range cfg.WatchProcesses {
		names = append(names, p.Name)
		onExit[p.Name] = p.TasksOnExit
	}
	if len(names) == 0 {
		return
	}

	w := watcher.New(watcher.Options{
		Names:  names,
		Logger: log,
		OnEvent: func(e watcher.Event) {
			if e.Kind != watcher.Exited || !onExit[e.Name] {
				return
			}
			tasks, err := database.PendingForProcess(e.Name)
			if err != nil {
				log.Warn("watcher: fetch tasks failed", "process", e.Name, "err", err)
				return
			}
			for _, t := range tasks {
				log.Info("reminder", "process", e.Name, "task", t.Content)
			}
		},
	})

	d.Register("watcher", w.Run)
}

// socketPath resolves the daemon socket from config, falling back to the default.
func socketPath() string {
	cfg, err := config.Load(configPath())
	if err != nil {
		return ipc.DefaultSocketPath
	}
	return cfg.Paths.Socket
}

// cmdStop asks the running daemon to shut down.
func cmdStop() error {
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "stop"})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// cmdStatus reports whether the daemon is running.
func cmdStatus() error {
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "status"})
	if err != nil {
		fmt.Println("not running")
		return nil
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}
