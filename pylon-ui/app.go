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
		return response{}, fmt.Errorf("daemon is not running (pylon start): %w", err)
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

// Ask runs one typed command through the same intent engine the microphone
// feeds — the daemon's "say", which is what `pylon say` uses. It exists so the
// GUI stays usable with the mic off: in a quiet room, with the headset
// unplugged, or when a container name is easier to type than to pronounce.
//
// The deadline is Listen's minus the recording: an LLM round trip plus whatever
// service it resolves to can take a while, and cutting it short would look like
// the daemon had died.
func (a *App) Ask(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	resp, err := sendTimeout(request{Cmd: "say", Args: []string{text}}, 60*time.Second)
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}

// Language reports the language the daemon is speaking, so the interface can
// label itself the same way. The GUI deliberately stores no language of its
// own: two settings would eventually disagree, and a window whose buttons are
// English while the answers inside them are Turkish looks broken. Settings
// changes the daemon's language (SetLanguage) and then reads it back — the
// daemon stays the single source, the interface just follows.
//
// With no daemon reachable it returns "" and the frontend keeps its default.
func (a *App) Language() string {
	resp, err := send(request{Cmd: "lang"})
	if err != nil || !resp.OK {
		return ""
	}
	return resp.Text
}

// LanguagePref reports the language explicitly chosen, or "" when none was and
// Pylon is following pylon.yaml or the desktop's locale. Settings needs the
// difference: Language() alone cannot tell a chosen Turkish from a Turkish that
// simply followed the system, so "follow the system" could never show as the
// selected option.
//
// "" is also what an unreachable daemon gives, which lands on the same default.
func (a *App) LanguagePref() string {
	resp, err := send(request{Cmd: "lang", Args: []string{"pref"}})
	if err != nil || !resp.OK {
		return ""
	}
	return resp.Text
}

// Languages lists what Settings can offer: one line per language, "<code>\t<its
// own name>". The list comes from the daemon rather than being hard-coded in
// the frontend so that adding a catalog adds a row on its own — and so a GUI
// paired with an older daemon offers only what that daemon can actually speak.
//
// Tab-separated plain text, matching Hotkey, rather than a bound struct for two
// strings.
func (a *App) Languages() string {
	resp, err := send(request{Cmd: "lang", Args: []string{"list"}})
	if err != nil || !resp.OK {
		return ""
	}
	return resp.Text
}

// SetLanguage switches the language Pylon speaks and remembers the choice,
// returning the language that took effect. It applies immediately: nothing is
// restarted, and the next reply is already in the new language.
//
// An empty tag (or "auto") forgets the choice and follows pylon.yaml, or the
// desktop's locale when that says nothing.
func (a *App) SetLanguage(tag string) (string, error) {
	if tag == "" {
		tag = "auto"
	}
	resp, err := send(request{Cmd: "lang", Args: []string{"set", tag}})
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

// DeleteSecret removes a stored credential. Without it a key saved by mistake —
// or one that has to be revoked — could only be overwritten, never taken back,
// unless the user found `pylon secret rm` on the command line.
func (a *App) DeleteSecret(name string) error {
	resp, err := send(request{Cmd: "secret", Args: []string{"rm", name}})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// RestartDaemon bounces the daemon so a newly saved key/config is picked up.
// It only restarts a daemon the GUI started; a hand-started one is left alone.
func (a *App) RestartDaemon() {
	a.daemon.restart()
}

// AuthStatus reports a service's connection ("google", "spotify"): "connected"
// (signed in), "ready" (can sign in), "unavailable" (no OAuth client in this
// build), or "offline" (the daemon did not answer).
//
// "offline" is separate on purpose. Folding an unreachable daemon into
// "unavailable" made a cold launch — when the GUI has spawned the daemon but it
// is still coming up — claim sign-in is missing from the build, which is both
// wrong and sounds permanent. The card re-checks once the daemon answers.
func (a *App) AuthStatus(service string) string {
	resp, err := send(request{Cmd: "auth", Args: []string{service, "status"}})
	if err != nil {
		return "offline"
	}
	if !resp.OK {
		return "unavailable"
	}
	return resp.Text
}

// AuthLogin runs the browser OAuth consent for a service. It blocks until the
// user finishes in the browser, so it uses a long deadline.
func (a *App) AuthLogin(service string) error {
	resp, err := sendTimeout(request{Cmd: "auth", Args: []string{service, "login"}}, 5*time.Minute)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// AuthLogout forgets a service's stored token. The services it enabled are only
// dropped when the daemon reloads its registry, so the caller bounces it.
func (a *App) AuthLogout(service string) error {
	resp, err := send(request{Cmd: "auth", Args: []string{service, "logout"}})
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

// Hotkey reports the push-to-talk shortcut as "<combo>\t<compositor>". The
// second field names the compositor that registered it (Hyprland, Sway) and is
// empty when this desktop has no way to bind one at runtime — the GUI's cue to
// show the user the line to add themselves. Tab-separated to match the daemon's
// plain-text IPC replies rather than introduce a bound struct for two strings.
func (a *App) Hotkey() string {
	resp, err := send(request{Cmd: "hotkey", Args: []string{"get"}})
	if err != nil || !resp.OK {
		return ""
	}
	return resp.Text
}

// SetHotkey changes the push-to-talk shortcut and registers it immediately,
// returning the same "<combo>\t<compositor>" pair Hotkey does. The daemon owns
// the binding, so it takes effect without restarting anything and is re-applied
// on the next daemon start.
func (a *App) SetHotkey(combo string) (string, error) {
	resp, err := send(request{Cmd: "hotkey", Args: []string{"set", combo}})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Text, nil
}
