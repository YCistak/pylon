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

	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/ipc"
)

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

// cmdStart runs the daemon in the foreground until interrupted.
func cmdStart() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	d := daemon.New(daemon.Options{Logger: log})
	err := d.Run(context.Background())
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		return err
	}
	return err
}

// cmdStop asks the running daemon to shut down.
func cmdStop() error {
	resp, err := daemon.Send(ipc.DefaultSocketPath, ipc.Request{Cmd: "stop"})
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
	resp, err := daemon.Send(ipc.DefaultSocketPath, ipc.Request{Cmd: "status"})
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
