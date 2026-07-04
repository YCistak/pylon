package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// daemonSocket is where the Pylon daemon listens (ipc.DefaultSocketPath). The
// GUI is just another client of the daemon — it never embeds the daemon logic.
const daemonSocket = "/tmp/pylon.sock"

// request / response mirror internal/ipc.{Request,Response}. The GUI is a
// separate Go module (so the daemon's CGo-free build never pulls in Wails), so
// it carries its own copy of this tiny wire protocol rather than importing it.
type request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

type response struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// App is the Wails-bound backend. Its exported methods are callable from the
// Svelte frontend.
type App struct {
	ctx    context.Context
	daemon *daemonManager
}

func NewApp() *App { return &App{daemon: &daemonManager{}} }

// startup launches the daemon in the background if it isn't already up, so the
// user just opens the window — no separate terminal. Runs in a goroutine so the
// window appears immediately; the frontend's status poll flips to online once
// the socket answers.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.daemon.ensureRunning()
}

// shutdown stops the daemon on window close, but only if the GUI started it
// (a daemon the user launched by hand is left running).
func (a *App) shutdown(ctx context.Context) {
	a.daemon.stop()
}

// send dials the daemon, sends one request, and returns the reply. A dial error
// here means the daemon isn't running.
func send(req request) (response, error) {
	conn, err := net.DialTimeout("unix", daemonSocket, 2*time.Second)
	if err != nil {
		return response{}, fmt.Errorf("daemon çalışmıyor (pylon start): %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response{}, err
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return response{}, err
	}
	return resp, nil
}

// DaemonRunning reports whether the daemon is reachable — drives the sidebar
// status dot and lets the UI offer to start it.
func (a *App) DaemonRunning() bool {
	_, err := send(request{Cmd: "ping"})
	return err == nil
}

// Status returns the daemon's status line ("running (pid …), N pending task(s)").
func (a *App) Status() (string, error) {
	resp, err := send(request{Cmd: "status"})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}

// Do runs a service action directly (no LLM) and returns its speakable text —
// the data source for every home widget. e.g. Do("freshrss.unread_count").
func (a *App) Do(action string) (string, error) {
	resp, err := send(request{Cmd: "do", Args: []string{action}})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}
