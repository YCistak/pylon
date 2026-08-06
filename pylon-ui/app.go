package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// daemonSocket reports where the Pylon daemon listens. It must agree with
// internal/ipc.DefaultSocketPath(), which this module cannot import: that path
// is under the daemon's internal/, and the GUI is a separate module on purpose
// (see the wire-protocol note below), so the rule forbids it. The values are
// therefore kept in sync by hand — PYLON_SOCKET overrides both sides, which is
// also the escape hatch when a config sets a custom paths.socket.
func daemonSocket() string {
	if p := os.Getenv("PYLON_SOCKET"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		// Windows has no /tmp; mirror ipc/paths_windows.go.
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		return filepath.Join(dir, "pylon", "pylon.sock")
	}
	return "/tmp/pylon.sock"
}

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
	return sendTimeout(req, 20*time.Second)
}

// sendTimeout is send with an explicit read/write deadline, for calls that run
// longer than a widget fetch — e.g. push-to-talk records for several seconds
// before it can answer.
func sendTimeout(req request, timeout time.Duration) (response, error) {
	conn, err := net.DialTimeout("unix", daemonSocket(), 2*time.Second)
	if err != nil {
		return response{}, fmt.Errorf("daemon çalışmıyor (pylon start): %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

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

// Listen runs one push-to-talk cycle in the daemon: record from the mic,
// transcribe, run the intent, and speak the reply. Returns the reply text
// (prefixed with what was heard) for the UI to show. It can take several seconds
// — the mic records first — so it uses a long deadline.
func (a *App) Listen() (string, error) {
	resp, err := sendTimeout(request{Cmd: "listen"}, 70*time.Second)
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}

// Platform reports the desktop the GUI is running on, so the settings UI can
// show the right way to bind the push-to-talk key. Returns one of: "hyprland",
// "sway", "gnome", "kde", "linux" (other), "macos", "windows".
func (a *App) Platform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	}
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return "hyprland"
	}
	if os.Getenv("SWAYSOCK") != "" {
		return "sway"
	}
	switch d := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")); {
	case strings.Contains(d, "hyprland"):
		return "hyprland"
	case strings.Contains(d, "sway"), strings.Contains(d, "i3"):
		return "sway"
	case strings.Contains(d, "gnome"):
		return "gnome"
	case strings.Contains(d, "kde"), strings.Contains(d, "plasma"):
		return "kde"
	}
	return "linux"
}

// SetSecret saves a credential to the daemon's encrypted vault (e.g. the Gemini
// API key under "gemini"). The value is AES-encrypted at rest — the Settings
// form never writes it to config in plaintext.
func (a *App) SetSecret(name, value string) error {
	resp, err := send(request{Cmd: "secret", Args: []string{"set", name, value}})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// HasSecret reports whether a secret is stored, without revealing it — so the
// Settings form can show "saved" without decrypting anything into view.
func (a *App) HasSecret(name string) bool {
	resp, err := send(request{Cmd: "secret", Args: []string{"has", name}})
	return err == nil && resp.OK && resp.Text == "true"
}

// RestartDaemon bounces the daemon so a newly saved key/config is picked up.
// It only restarts a daemon the GUI started; a hand-started one is left alone.
func (a *App) RestartDaemon() {
	a.daemon.restart()
}

// GoogleStatus reports the Google connection: "connected" (signed in),
// "ready" (can sign in), "unavailable" (no OAuth client in this build), or
// "offline" (the daemon did not answer).
//
// "offline" is separate on purpose. Folding an unreachable daemon into
// "unavailable" made a cold launch — when the GUI has spawned the daemon but it
// is still coming up — claim Google sign-in is missing from the build, which is
// both wrong and sounds permanent. The card re-checks once the daemon answers.
func (a *App) GoogleStatus() string {
	resp, err := send(request{Cmd: "auth", Args: []string{"google", "status"}})
	if err != nil {
		return "offline"
	}
	if !resp.OK {
		return "unavailable"
	}
	return resp.Text
}

// GoogleLogin runs the browser OAuth consent for Google (Calendar, Drive). It
// blocks until the user finishes in the browser, so it uses a long deadline.
func (a *App) GoogleLogin() error {
	resp, err := sendTimeout(request{Cmd: "auth", Args: []string{"google", "login"}}, 5*time.Minute)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// Do runs a service action directly (no LLM) and returns its speakable text —
// the data source for every home widget. e.g. Do("freshrss.unread_count").
func (a *App) Do(action string, params map[string]string) (string, error) {
	args := []string{action}
	for k, v := range params {
		args = append(args, k+"="+v)
	}
	resp, err := send(request{Cmd: "do", Args: args})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}
