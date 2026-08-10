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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/YCistak/pylon/internal/banner"
	"github.com/YCistak/pylon/internal/config"
	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/hotkey"
	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
	"github.com/YCistak/pylon/internal/ipc"
	"github.com/YCistak/pylon/internal/profile"
	"github.com/YCistak/pylon/internal/scheduler"
	"github.com/YCistak/pylon/internal/secrets"
	"github.com/YCistak/pylon/internal/services"
	"github.com/YCistak/pylon/internal/services/briefing"
	"github.com/YCistak/pylon/internal/services/calc"
	"github.com/YCistak/pylon/internal/services/docker"
	"github.com/YCistak/pylon/internal/services/exchange"
	"github.com/YCistak/pylon/internal/services/freshrss"
	ghsvc "github.com/YCistak/pylon/internal/services/github"
	"github.com/YCistak/pylon/internal/services/google"
	"github.com/YCistak/pylon/internal/services/spotify"
	"github.com/YCistak/pylon/internal/services/sysmon"
	"github.com/YCistak/pylon/internal/services/system"
	"github.com/YCistak/pylon/internal/services/weather"
	"github.com/YCistak/pylon/internal/services/work"
	"github.com/YCistak/pylon/internal/voice"
	"github.com/YCistak/pylon/internal/watcher"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// loadConfig reads the configuration and puts the process into the language it
// names, before anything has had a chance to speak. Every entry point goes
// through it rather than config.Load directly: a CLI command that skipped it
// would answer in English while the daemon answered in Turkish.
//
// An empty language follows the desktop locale, so a fresh install needs no
// configuration to speak the right language.
func loadConfig() (config.Config, error) {
	cfg, err := config.Load(configPath())
	if err != nil {
		return cfg, err
	}
	lang := strings.TrimSpace(cfg.Language)
	if lang == "" {
		lang = i18n.FromEnv()
	}
	i18n.SetLanguage(lang)
	return cfg, nil
}

// configPath finds pylon.yaml, in order: $PYLON_CONFIG, ./pylon.yaml, then
// ~/.config/pylon/pylon.yaml.
//
// The last one is what makes an installed Pylon usable at all. Until it existed
// the search stopped at the working directory, so launching from a desktop
// entry or an application menu — where the working directory is the user's home
// — found nothing and silently ran on defaults: no voice, no services, no
// briefing, with nothing on screen to say why. The working directory still wins
// so a checkout keeps using its own pylon.yaml without any setup.
func configPath() string {
	if p := os.Getenv("PYLON_CONFIG"); p != "" {
		return p
	}
	if _, err := os.Stat("pylon.yaml"); err == nil {
		return "pylon.yaml"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "pylon", "pylon.yaml")
	}
	// No home directory to fall back on; Load treats a missing file as defaults.
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
	case "say":
		err = cmdSay(os.Args[2:])
	case "do":
		err = cmdDo(os.Args[2:])
	case "briefing":
		err = cmdBriefing(os.Args[2:])
	case "work":
		err = cmdWork(os.Args[2:])
	case "recall":
		err = cmdRecall(os.Args[2:])
	case "listen":
		err = cmdListen()
	case "auth":
		err = cmdAuth(os.Args[2:])
	case "secret":
		err = cmdSecret(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("pylon", version)
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
  pylon start         run the daemon (foreground)
  pylon stop          stop a running daemon
  pylon status        show daemon status
  pylon say <text>    send a text command through the intent engine
  pylon briefing      compose today's briefing and show it as a desktop banner
  pylon briefing --speak    the same, read aloud
  pylon work [week]   how long the tracked apps were used today (or this week)
  pylon recall [n]    show the last n remembered turns (default 5)
  pylon listen        push-to-talk: record, transcribe, run, speak the reply
  pylon auth <google|spotify>          one-time OAuth consent in the browser
  pylon auth <google|spotify> logout   forget the saved token
  pylon secret set <name>   save a credential to the encrypted vault (ref as secret:<name>)
  pylon secret rm  <name>   remove a saved credential
  pylon update [--check]    install the newest release (--check only reports)
`)
}

// cmdStart loads config, opens the database, and runs the daemon in the
// foreground until interrupted.
func cmdStart() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := loadConfig()
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

	// The service registry is shared: the intent engine dispatches to it, and the
	// scheduler's daily briefing composes its sections through it.
	registry := buildServiceRegistry(cfg, database, log)
	intent.SetActions(registry.Specs()...)

	// Built before the watcher so the watcher can feed it: the tracker turns
	// process events into work sessions, and is nil when nothing is tracked.
	tracker := registerSessions(d, cfg, database, log)

	registerWatcher(d, cfg, database, tracker, log)
	registerScheduler(d, cfg, database, registry, log)
	registerIntent(d, cfg, database, registry, log)
	registerSecrets(d)
	registerAuth(d, cfg)
	registerHotkey(d, cfg, database, log)
	registerSTTServer(d, cfg, log)

	return d.Run(context.Background())
}

// authProvider is one signable service reduced to what the GUI asks of it. The
// two predicates are called per request rather than sampled once, because login
// and logout change their answers while the daemon keeps running.
type authProvider struct {
	hasClient func() bool // this build can run the consent flow
	connected func() bool // a user token is stored
	login     func(context.Context) error
	logout    func() error
	// unavailable explains, to the user, why login can't run in this build.
	unavailable string
}

// authProviders enumerates the services the GUI can sign in and out. Adding one
// here is all it takes for the Accounts card to grow a row — the IPC verbs, the
// status vocabulary and the GUI bindings are all service-agnostic.
func authProviders(cfg config.Config) map[string]authProvider {
	gcfg := googleConfig(cfg)
	scfg := spotifyConfig(cfg)
	return map[string]authProvider{
		"google": {
			hasClient:   func() bool { return google.HasClient(gcfg) },
			connected:   func() bool { return google.Configured(gcfg) },
			login:       func(ctx context.Context) error { return google.Authorize(ctx, gcfg) },
			logout:      google.Logout,
			unavailable: "Google girişi bu Pylon sürümünde henüz yapılandırılmadı",
		},
		"spotify": {
			hasClient: func() bool { return spotify.HasClient(scfg) },
			connected: func() bool { return spotify.Configured(scfg) },
			login:     func(ctx context.Context) error { return spotify.Authorize(ctx, scfg) },
			logout:    spotify.Logout,
			unavailable: "Spotify bağlantısı bu Pylon sürümünde henüz yapılandırılmadı " +
				"(Redirect URI: " + spotify.RedirectURI(scfg) + ")",
		},
	}
}

// registerAuth lets the GUI sign services in and out over IPC:
//
//	auth <service> status   → connected | ready | unavailable
//	auth <service> login    → runs the browser OAuth consent
//	auth <service> logout   → forgets the stored token
//
// login is the same flow as `pylon auth <service>`, saving the token to the
// encrypted vault. Signing out only drops the token: the services it enabled
// stay registered until the daemon restarts, which the GUI triggers.
func registerAuth(d *daemon.Daemon, cfg config.Config) {
	providers := authProviders(cfg)
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	usage := "usage: auth <" + strings.Join(names, "|") + "> <status|login|logout>"

	d.Handle("auth", func(req ipc.Request) ipc.Response {
		if len(req.Args) < 2 {
			return ipc.Response{OK: false, Error: usage}
		}
		p, ok := providers[req.Args[0]]
		if !ok {
			return ipc.Response{OK: false, Error: "bilinmeyen servis: " + req.Args[0] + " — " + usage}
		}
		switch req.Args[1] {
		case "status":
			switch {
			case p.connected():
				return ipc.Response{OK: true, Text: "connected"}
			case p.hasClient():
				return ipc.Response{OK: true, Text: "ready"}
			default:
				return ipc.Response{OK: true, Text: "unavailable"}
			}
		case "login":
			if !p.hasClient() {
				return ipc.Response{OK: false, Error: p.unavailable}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := p.login(ctx); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true, Text: "bağlandı"}
		case "logout":
			if err := p.logout(); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true, Text: "çıkış yapıldı"}
		default:
			return ipc.Response{OK: false, Error: "bilinmeyen işlem: " + req.Args[1] + " — " + usage}
		}
	})
}

// registerSecrets lets the GUI manage the encrypted vault over IPC: the Settings
// API-key field saves keys here (set), checks whether one exists (has), or clears
// it (rm). Values ride the local Unix socket and are AES-encrypted at rest — the
// same path as `pylon secret set`, never written to config in plaintext.
func registerSecrets(d *daemon.Daemon) {
	d.Handle("secret", func(req ipc.Request) ipc.Response {
		if len(req.Args) < 2 {
			return ipc.Response{OK: false, Error: "usage: secret <set|rm|has> <name> [value]"}
		}
		op, name := req.Args[0], req.Args[1]
		switch op {
		case "set":
			if len(req.Args) < 3 {
				return ipc.Response{OK: false, Error: "secret set için değer gerekli"}
			}
			if err := secrets.Set(name, req.Args[2]); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true, Text: "kaydedildi"}
		case "rm":
			if err := secrets.Delete(name); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true, Text: "silindi"}
		case "has":
			return ipc.Response{OK: true, Text: strconv.FormatBool(secrets.Has(name))}
		default:
			return ipc.Response{OK: false, Error: "bilinmeyen işlem: " + op}
		}
	})
}

// hotkeyContextKey is where the chosen push-to-talk shortcut is remembered, so
// the daemon can re-register it on the next start.
const hotkeyContextKey = "voice.hotkey"

// registerHotkey owns the push-to-talk shortcut. Wayland gives an application no
// way to grab a global hotkey for itself, and editing the user's compositor
// config to get one means writing to a file Pylon does not own and cannot
// cleanly take back — a change that is then hard to find again. Hyprland and
// Sway both accept bindings over their control socket, so the shortcut is
// registered at runtime instead: nothing on disk changes, and the daemon
// re-applies it whenever it starts.
//
// "hotkey get" reports the current shortcut and which compositor claimed it;
// "hotkey set <combo>" changes it. Desktops with no runtime binding API answer
// with an empty compositor field, which is the GUI's cue to show the user the
// line to add themselves.
func registerHotkey(d *daemon.Daemon, cfg config.Config, database *db.DB, log *slog.Logger) {
	mgr := hotkey.Detect()
	wm := ""
	if mgr != nil {
		wm = mgr.Name()
	}

	// Bind the running binary by absolute path: "pylon" alone only works if the
	// compositor's PATH happens to include it, which it will not for a binary
	// run out of a build or release directory.
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "pylon"
	}
	command := self + " listen"

	// bound is what the compositor currently holds for us, so shutdown can hand
	// exactly that back — it is not always what config or the database says,
	// because "hotkey set" moves it while the daemon runs.
	var mu sync.Mutex
	var bound *hotkey.Combo

	apply := func(combo hotkey.Combo) error {
		if mgr == nil {
			return nil // nothing to apply; the GUI explains the manual route
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mgr.Bind(ctx, combo, command); err != nil {
			return err
		}
		mu.Lock()
		bound = &combo
		mu.Unlock()
		return nil
	}

	// unbind gives the shortcut back. Deliberately uses its own context: it runs
	// during shutdown, when the daemon's context is already cancelled.
	unbind := func() {
		mu.Lock()
		combo := bound
		bound = nil
		mu.Unlock()
		if mgr == nil || combo == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mgr.Unbind(ctx, *combo); err != nil {
			log.Warn("hotkey: bağlantı kaldırılamadı", "hotkey", combo.String(), "err", err)
			return
		}
		log.Info("hotkey: bağlantı kaldırıldı", "hotkey", combo.String())
	}

	// stored falls back to the config's hotkey, which is what a fresh install
	// has before the user picks one in the GUI.
	stored := func() string {
		if database != nil {
			if v, ok, err := database.GetContext(hotkeyContextKey); err == nil && ok && v != "" {
				return v
			}
		}
		return cfg.Voice.Hotkey
	}

	// Bound as a service, not once at startup, so that the binding has a
	// lifetime: it exists exactly as long as the daemon does. A binding left
	// behind after the daemon exits still launches `pylon listen`, which records
	// audio and then fails with "daemon not reachable" — inside a process the
	// compositor started, where nobody ever sees the error. A shortcut that does
	// nothing is easier to diagnose than one that lies.
	d.Register("hotkey", func(ctx context.Context) error {
		switch combo, err := hotkey.Parse(stored()); {
		case err != nil:
			log.Warn("hotkey: kayıtlı kısayol okunamadı", "value", stored(), "err", err)
		case mgr == nil:
			log.Info("hotkey: bu masaüstünde çalışma-anı bağlama yok", "hotkey", combo.String())
		default:
			if err := apply(combo); err != nil {
				log.Warn("hotkey: bağlanamadı", "hotkey", combo.String(), "wm", wm, "err", err)
			} else {
				log.Info("hotkey: bağlandı", "hotkey", combo.String(), "wm", wm, "cmd", command)
			}
		}
		<-ctx.Done()
		unbind()
		return ctx.Err()
	})

	d.Handle("hotkey", func(req ipc.Request) ipc.Response {
		if len(req.Args) == 0 {
			return ipc.Response{OK: false, Error: "usage: hotkey <get|set> [combo]"}
		}
		switch req.Args[0] {
		case "get":
			// "<combo>\t<compositor>" — an empty second field means the user has
			// to add the binding themselves. Normalized, so the GUI shows the
			// same shape whether the value came from config ("super+p") or from
			// a previous set.
			combo := stored()
			if c, err := hotkey.Parse(combo); err == nil {
				combo = c.String()
			}
			return ipc.Response{OK: true, Text: combo + "\t" + wm}

		case "set":
			if len(req.Args) < 2 {
				return ipc.Response{OK: false, Error: "hotkey set için kısayol gerekli"}
			}
			combo, err := hotkey.Parse(req.Args[1])
			if err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			// Drop the previous binding first, or the old shortcut keeps working
			// alongside the new one. What to drop is what we actually bound, not
			// what is stored — those differ if an earlier bind failed.
			mu.Lock()
			old := bound
			mu.Unlock()
			if old != nil && old.String() != combo.String() {
				unbind()
			}
			if err := apply(combo); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			if database != nil {
				if err := database.SetContext(hotkeyContextKey, combo.String()); err != nil {
					log.Warn("hotkey: kaydedilemedi", "err", err)
				}
			}
			log.Info("hotkey: değişti", "hotkey", combo.String(), "wm", wm)
			return ipc.Response{OK: true, Text: combo.String() + "\t" + wm}

		default:
			return ipc.Response{OK: false, Error: "bilinmeyen işlem: " + req.Args[0]}
		}
	})
}

// briefingPresenter builds the desktop banner presenter from config.
func briefingPresenter(cfg config.Config) *banner.Presenter {
	return banner.NewPresenter(cfg.Briefing.BannerCmd)
}

// briefingSpeaker builds the TTS speaker from config, or nil when no tts_cmd is
// set (the briefing is then banner-only).
func briefingSpeaker(cfg config.Config) voice.Speaker {
	if len(cfg.Voice.TTSCmd) == 0 {
		return nil
	}
	return voice.NewSpeaker(cfg.Voice.TTSCmd, cfg.Voice.PlayCmd)
}

// voiceOptions maps the voice config onto the pipeline, so `pylon listen` and the
// daemon's "listen" command always capture and transcribe identically.
func voiceOptions(cfg config.Config) voice.Options {
	o := voice.Options{
		STTBin:           cfg.Voice.STTBin,
		STTModel:         cfg.Voice.STTModel,
		Language:         cfg.Voice.Language,
		TTSCmd:           cfg.Voice.TTSCmd,
		RecordCmd:        cfg.Voice.RecordCmd,
		RecordSeconds:    cfg.Voice.RecordSeconds,
		SilenceStop:      cfg.Voice.SilenceStop,
		SilenceSeconds:   cfg.Voice.SilenceSeconds,
		SilenceThreshold: cfg.Voice.SilenceThreshold,
		PlayCmd:          cfg.Voice.PlayCmd,
	}
	// Both the daemon and the CLI talk to the warm server; only the daemon owns
	// the process (see registerSTTServer).
	if cfg.Voice.STTServer.Enabled() {
		o.STTServerAddr = cfg.Voice.STTServer.Addr()
	}
	return o
}

// registerSTTServer keeps a whisper.cpp server warm for the daemon's lifetime,
// so each spoken turn skips reloading the model (~610 ms with large-v3-turbo).
// Without stt_server.bin configured, transcription shells out per turn as before.
func registerSTTServer(d *daemon.Daemon, cfg config.Config, log *slog.Logger) {
	s := cfg.Voice.STTServer
	if !s.Enabled() || cfg.Voice.STTModel == "" {
		return
	}
	addr := s.Addr()
	log.Info("stt server", "addr", addr, "bin", s.Bin)
	d.Register("stt-server", func(ctx context.Context) error {
		return voice.RunSTTServer(ctx, voice.ServerOptions{
			Bin:       s.Bin,
			Addr:      addr,
			Model:     cfg.Voice.STTModel,
			Language:  cfg.Voice.Language,
			ExtraArgs: s.Args,
		})
	})
}

// registerIntent wires the two-tier intent path into the daemon's "say"
// command: the local Router runs first (free), and only unresolved input falls
// back to the configured LLM chain, which tries each model in order and falls
// through on quota/rate-limit.
func registerIntent(d *daemon.Daemon, cfg config.Config, database *db.DB, registry *services.Registry, log *slog.Logger) {
	router := intent.NewRouter(cfg.Intent.RouterThreshold)
	chain := buildIntentChain(cfg, log)
	if chain.Configured() {
		log.Info("intent: llm fallback enabled", "models", len(cfg.Intent.Models))
	} else {
		log.Info("intent: llm fallback disabled (no usable models)")
	}

	var persona *profile.Engine
	if cfg.Persona.Enabled {
		persona = profile.NewEngine(database, cfg.Persona.DecayHalfLifeDays, cfg.Persona.AdoptThreshold, cfg.Persona.StyleCardRefreshN)
		log.Info("persona: style learning enabled")
	}

	// runIntent resolves one utterance (local router first, LLM chain on miss),
	// executes it, and remembers the turn. Shared by the "say" and "listen"
	// handlers so typed and spoken input take the exact same path.
	runIntent := func(text string) ipc.Response {
		text = strings.TrimSpace(text)
		if text == "" {
			return ipc.Response{OK: false, Error: "empty command"}
		}

		// Persona learns from every utterance and shapes conversational replies.
		var styleCard string
		if persona != nil {
			if err := persona.Observe(text); err != nil {
				log.Warn("persona: observe failed", "err", err)
			}
			styleCard = persona.StyleCard()
		}

		cmd := router.Resolve(text)
		source := "local"
		if !cmd.Resolved() {
			if !chain.Configured() {
				return ipc.Response{OK: true, Text: fmt.Sprintf("anlayamadım (lokal eşleşme %.2f) ve model bağlı değil", cmd.Confidence)}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var err error
			cmd, source, err = chain.Parse(ctx, text, styleCard)
			if err != nil {
				log.Warn("intent: llm chain failed", "err", err)
				return ipc.Response{OK: false, Error: "model hatası: " + err.Error()}
			}
		}
		log.Info("intent", "text", text, "action", string(cmd.Action), "confidence", cmd.Confidence, "source", source)

		resp := executeCommand(cmd, database, registry)
		rememberTurn(database, text, resp, log)
		return resp
	}

	d.Handle("say", func(req ipc.Request) ipc.Response {
		return runIntent(strings.Join(req.Args, " "))
	})

	// "listen" runs one push-to-talk cycle inside the daemon: record from the
	// mic, transcribe, resolve+execute the intent, then speak the reply. The GUI's
	// mic button calls this, so voice works from a click without a terminal. The
	// reply text comes back prefixed with what was heard, for the UI to show.
	d.Handle("listen", func(ipc.Request) ipc.Response {
		if cfg.Voice.STTBin == "" || cfg.Voice.STTModel == "" {
			return ipc.Response{OK: false, Error: "ses tanıma yapılandırılmadı (voice.stt_bin / stt_model)"}
		}
		pipe := voice.NewPipeline(voiceOptions(cfg))
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		heard, err := pipe.Capture(ctx)
		if voice.IsNoSpeech(err) {
			return ipc.Response{OK: true, Text: "ses algılanamadı"}
		}
		if err != nil {
			log.Warn("listen: capture failed", "err", err)
			return ipc.Response{OK: false, Error: "ses alınamadı: " + err.Error()}
		}
		if heard = strings.TrimSpace(heard); heard == "" {
			return ipc.Response{OK: true, Text: "ses algılanamadı"}
		}
		log.Info("listen", "heard", heard)

		resp := runIntent(heard)
		if resp.OK && resp.Text != "" {
			if err := pipe.Speak(ctx, resp.Text); err != nil {
				log.Warn("listen: speak failed", "err", err)
			}
			resp.Text = "» " + heard + "\n" + resp.Text
		}
		return resp
	})

	// "do" runs a service action directly (no LLM), for the GUI's widgets and
	// scripted use: `do <action> [k=v ...]`. It dispatches through the same
	// service registry as "say", so any service action is reachable.
	d.Handle("do", func(req ipc.Request) ipc.Response {
		if len(req.Args) == 0 {
			return ipc.Response{OK: false, Error: "usage: do <action> [k=v ...]"}
		}
		args := map[string]string{}
		for _, a := range req.Args[1:] {
			if k, v, ok := strings.Cut(a, "="); ok {
				args[k] = v
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		text, ok, err := registry.Dispatch(ctx, intent.Command{Action: intent.Action(req.Args[0]), Args: args})
		switch {
		case err != nil:
			return ipc.Response{OK: false, Error: err.Error()}
		case !ok:
			return ipc.Response{OK: false, Error: "bilinmeyen/servissiz aksiyon: " + req.Args[0]}
		default:
			return ipc.Response{OK: true, Text: text}
		}
	})

	// "briefing speak" composes the briefing and reads it aloud. It exists
	// because "do" is generic and therefore silent: the briefing service shows
	// the banner and returns text, and the caller decides whether that text is
	// spoken. The scheduler does at 08:00; `pylon briefing` did not, so the only
	// way to hear one on demand was to say "brifing ver" into the microphone.
	//
	// Speaking belongs here rather than inside the service for the same reason:
	// the listen path already speaks whatever a command returns, so a service
	// that spoke for itself would read every briefing out twice.
	d.Handle("briefing", func(req ipc.Request) ipc.Response {
		if len(req.Args) != 1 || req.Args[0] != "speak" {
			return ipc.Response{OK: false, Error: "usage: briefing speak"}
		}
		// Long enough to compose (calendar and news are both networked) and then
		// to read the whole thing out.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		text, ok, err := registry.Dispatch(ctx, intent.Command{Action: briefing.ActionToday})
		switch {
		case err != nil:
			return ipc.Response{OK: false, Error: err.Error()}
		case !ok:
			return ipc.Response{OK: false, Error: "brifing servisi kayıtlı değil"}
		}

		// No TTS configured is not a failure: you still get the banner and the
		// text, which is exactly what plain `pylon briefing` gives.
		if speaker := briefingSpeaker(cfg); speaker != nil {
			if err := speaker.Say(ctx, text); err != nil {
				log.Warn("briefing: speak failed", "err", err)
			}
		}
		return ipc.Response{OK: true, Text: text}
	})

	registerRecall(d, database)
}

// buildServiceRegistry registers the external services that are configured and
// authorized. Each service contributes actions to the LLM vocabulary.
func buildServiceRegistry(cfg config.Config, database *db.DB, log *slog.Logger) *services.Registry {
	var svcs []services.Service

	// The briefing reads these directly (typed), so capture the concrete
	// instances as they're built; each stays nil when its service is off.
	var calSrc briefing.CalendarSource
	var newsSrc briefing.NewsSource

	gcfg := googleConfig(cfg)
	switch {
	case google.Configured(gcfg):
		cal := google.NewCalendar(gcfg)
		svcs = append(svcs, cal, google.NewDrive(gcfg))
		calSrc = cal
		log.Info("services: google calendar + drive enabled")
	case google.HasClient(gcfg):
		log.Info("services: google credentials found — run `pylon auth google` to enable calendar/drive")
	}

	ghcfg := githubConfig(cfg)
	if ghsvc.Configured(ghcfg) {
		svcs = append(svcs, ghsvc.New(ghcfg))
		log.Info("services: github enabled")
	}

	fcfg := freshrssConfig(cfg)
	if freshrss.Configured(fcfg) {
		fr := freshrss.New(fcfg)
		svcs = append(svcs, fr)
		newsSrc = fr
		log.Info("services: freshrss enabled")
	}

	scfg := spotifyConfig(cfg)
	if spotify.Configured(scfg) {
		svcs = append(svcs, spotify.New(scfg))
		log.Info("services: spotify enabled")
	}

	dcfg := dockerConfig(cfg)
	if docker.Configured(dcfg) {
		svcs = append(svcs, docker.New(dcfg))
		log.Info("services: docker enabled")
	}

	// Calculator needs no configuration — always available.
	svcs = append(svcs, calc.New())

	// Exchange rates/crypto use free key-less APIs — always available.
	svcs = append(svcs, exchange.New())

	// Weather uses Open-Meteo (no key). Always available; defaults to İstanbul.
	// The briefing reads the same instance, so both speak the same location.
	wx := weather.New(cfg.Services.Weather.Latitude, cfg.Services.Weather.Longitude, cfg.Services.Weather.Name)
	svcs = append(svcs, wx)

	// System vitals read /proc and /sys — no configuration, always available.
	svcs = append(svcs, sysmon.New(""))

	// System control (lock/volume/media/close) needs no configuration — always
	// available. It owns the media/lock actions the local router emits.
	svcs = append(svcs, system.New())

	// Work sessions read what the tracker recorded. Registered whenever there is
	// a database: with no tracked apps the answer is simply "nothing recorded",
	// which is more useful than the question not being understood at all. The
	// day boundary follows the briefing's timezone — one place to say where the
	// user lives.
	if database != nil {
		svcs = append(svcs, work.NewService(work.ServiceOptions{
			Store:     database,
			GoalHours: cfg.Work.DailyGoalHours,
			Timezone:  cfg.Briefing.Timezone,
		}))
		log.Info("services: work sessions enabled", "tracked", cfg.Work.TrackedApps)
	}

	// The briefing reads the weather/calendar/news sources directly and phrases
	// its own clauses; running its action also shows the desktop banner, so every
	// trigger (voice, scheduler, CLI) presents it the same way.
	brief := briefing.New()
	brief.SetSources(wx, calSrc, newsSrc)
	brief.SetPresenter(briefingPresenter(cfg))
	svcs = append(svcs, brief)
	return services.NewRegistry(svcs...)
}

func spotifyConfig(cfg config.Config) spotify.Config {
	return spotify.Config{
		ClientID:     cfg.Services.Spotify.ClientID,
		RedirectPort: cfg.Services.Spotify.RedirectPort,
	}
}

func githubConfig(cfg config.Config) ghsvc.Config {
	return ghsvc.Config{Token: resolveSecret(cfg.Services.GitHub.Token)}
}

func freshrssConfig(cfg config.Config) freshrss.Config {
	return freshrss.Config{
		URL:         cfg.Services.FreshRSS.URL,
		Username:    cfg.Services.FreshRSS.Username,
		APIPassword: resolveSecret(cfg.Services.FreshRSS.APIPassword),
		APIKey:      resolveSecret(cfg.Services.FreshRSS.APIKey),
	}
}
func dockerConfig(cfg config.Config) docker.Config {
	return docker.Config{
		Socket: cfg.Services.Docker.Socket,
		Host:   cfg.Services.Docker.Host,
		Token:  resolveSecret(cfg.Services.Docker.Token),
	}
}

func googleConfig(cfg config.Config) google.Config {
	return google.Config{
		ClientID:        cfg.Services.Google.ClientID,
		ClientSecret:    resolveSecret(cfg.Services.Google.ClientSecret),
		CredentialsPath: cfg.Services.Google.Credentials,
		CalendarID:      cfg.Services.Google.CalendarID,
	}
}

// resolveSecret turns a "keyring:<name>" config value into the stored secret
// (plain values pass through). A keyring miss is logged, not fatal: the secret
// comes back empty and the owning service simply stays disabled.
func resolveSecret(value string) string {
	v, err := secrets.Resolve(value)
	if err != nil {
		slog.Default().Warn("secret resolve failed", "ref", value, "err", err)
		return ""
	}
	return v
}

// rememberTurn records the exchange in context memory (Phase 1.8) so Pylon can
// answer "what did we talk about". Each turn is a timestamped entry.
func rememberTurn(database *db.DB, text string, resp ipc.Response, log *slog.Logger) {
	if !resp.OK {
		return
	}
	key := fmt.Sprintf("turn:%d", time.Now().UnixNano())
	if err := database.SetContext(key, text+" ⟶ "+resp.Text); err != nil {
		log.Warn("context: remember failed", "err", err)
	}
}

// registerRecall serves recent conversation memory over IPC.
func registerRecall(d *daemon.Daemon, database *db.DB) {
	d.Handle("recall", func(req ipc.Request) ipc.Response {
		limit := 5
		if len(req.Args) > 0 {
			if n, err := strconv.Atoi(req.Args[0]); err == nil && n > 0 {
				limit = n
			}
		}
		entries, err := database.RecentContext(limit)
		if err != nil {
			return ipc.Response{OK: false, Error: "hafıza okunamadı: " + err.Error()}
		}
		if len(entries) == 0 {
			return ipc.Response{OK: true, Text: "henüz konuşma geçmişi yok"}
		}
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%s — %s\n", e.UpdatedAt.Local().Format("15:04"), e.Value)
		}
		return ipc.Response{OK: true, Text: strings.TrimRight(b.String(), "\n")}
	})
}

// buildIntentChain constructs the LLM fallback chain from config, resolving each
// model's API key from the environment and skipping entries that are unusable
// (missing key or unknown provider).
func buildIntentChain(cfg config.Config, log *slog.Logger) *intent.Chain {
	var parsers []intent.Parser
	for _, m := range cfg.Intent.Models {
		// Prefer the environment; fall back to a key saved in the encrypted vault
		// under the provider name (what the GUI's API-key field writes), so a user
		// who never set an env var can still enable the model from Settings.
		key := os.Getenv(m.APIKeyEnv)
		if key == "" {
			if v, err := secrets.Resolve("secret:" + m.Provider); err == nil {
				key = v
			}
		}
		if key == "" {
			log.Warn("intent: skipping model, no API key", "provider", m.Provider, "model", m.Model, "env", m.APIKeyEnv)
			continue
		}
		p, err := intent.NewParser(intent.ProviderSpec{
			Provider: m.Provider, Model: m.Model, APIKey: key, BaseURL: m.BaseURL,
		}, 10*time.Second) // voice is interactive — a stalled model should fall through fast
		if err != nil {
			log.Warn("intent: skipping model", "provider", m.Provider, "model", m.Model, "err", err)
			continue
		}
		parsers = append(parsers, p)
		log.Info("intent: model enabled", "name", p.Name())
	}
	return intent.NewChain(parsers, log)
}

// executeCommand acts on a resolved Command and produces a user-facing response.
// Built-in actions are handled here; anything else is offered to the service
// registry (calendar, etc.). Shared by the local router and the LLM fallback.
func executeCommand(cmd intent.Command, database *db.DB, registry *services.Registry) ipc.Response {
	switch cmd.Action {
	case intent.ActionRemindOnExit:
		if cmd.Args["process"] == "" || cmd.Args["content"] == "" {
			return ipc.Response{OK: false, Error: "hatırlatma için process ve içerik gerekli"}
		}
		id, err := database.AddTask(db.Task{
			Content:        cmd.Args["content"],
			TriggerProcess: cmd.Args["process"],
		})
		if err != nil {
			return ipc.Response{OK: false, Error: "task eklenemedi: " + err.Error()}
		}
		return ipc.Response{OK: true, Text: fmt.Sprintf(
			"tamam — '%s' kapanınca hatırlatacağım: %q (task #%d)",
			cmd.Args["process"], cmd.Args["content"], id)}

	case intent.ActionChat:
		reply := cmd.Args["reply"]
		if reply == "" {
			reply = "hımm, ne demek istedin tam anlamadım"
		}
		return ipc.Response{OK: true, Text: reply}

	default:
		// A service (calendar, ...) may own this action.
		if registry != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if text, ok, err := registry.Dispatch(ctx, cmd); ok {
				if err != nil {
					return ipc.Response{OK: false, Error: err.Error()}
				}
				return ipc.Response{OK: true, Text: text}
			}
		}
		// Recognized but owned by no service (should be rare now that the system
		// service handles media/lock/close).
		return ipc.Response{OK: true, Text: fmt.Sprintf("komut anlaşıldı ama karşılığı yok: %s (conf %.2f)", cmd.Action, cmd.Confidence)}
	}
}

// registerWatcher wires the daemon's one process watcher. Two features depend
// on process lifecycle — task reminders (`watch_processes`) and work-session
// tracking (`work.tracked_apps`) — and the two lists usually overlap, so they
// share a single poller rather than each walking /proc every two seconds.
func registerWatcher(d *daemon.Daemon, cfg config.Config, database *db.DB, tracker *work.Tracker, log *slog.Logger) {
	names := map[string]struct{}{}
	onExit := make(map[string]bool)
	for _, p := range cfg.WatchProcesses {
		names[p.Name] = struct{}{}
		onExit[p.Name] = p.TasksOnExit
	}
	if tracker != nil {
		for _, n := range tracker.Names() {
			names[n] = struct{}{}
		}
	}
	if len(names) == 0 {
		return
	}
	watched := make([]string, 0, len(names))
	for n := range names {
		watched = append(watched, n)
	}
	sort.Strings(watched) // deterministic log line

	// Reminders are spoken via the same TTS the assistant uses (edge-tts). Built
	// once; nil when TTS isn't configured, in which case we just log.
	var speaker voice.Speaker
	if len(cfg.Voice.TTSCmd) > 0 {
		speaker = voice.NewSpeaker(cfg.Voice.TTSCmd, cfg.Voice.PlayCmd)
	}

	remind := func(e watcher.Event) {
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
			if speaker != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := speaker.Say(ctx, "Hatırlatma. "+t.Content); err != nil {
					log.Warn("reminder: speak failed", "err", err)
				}
				cancel()
			}
			// Mark done so the same reminder doesn't fire on the next exit.
			if err := database.CompleteTask(t.ID); err != nil {
				log.Warn("reminder: complete failed", "id", t.ID, "err", err)
			}
		}
	}

	w := watcher.New(watcher.Options{
		Names:  watched,
		Logger: log,
		OnEvent: func(e watcher.Event) {
			remind(e)
			if tracker != nil {
				tracker.Observe(e.Name, e.Kind == watcher.Started, e.At)
			}
		},
		OnBaseline: func(running []string) {
			if tracker != nil {
				tracker.Seed(running)
			}
		},
	})

	d.Register("watcher", w.Run)
}

// registerSessions wires work-session tracking: which of your apps were open,
// and for how long. It returns the tracker so the watcher can feed it, and nil
// when `work.tracked_apps` is empty — nothing is recorded unless asked for.
func registerSessions(d *daemon.Daemon, cfg config.Config, database *db.DB, log *slog.Logger) *work.Tracker {
	// The break nudge reuses the briefing's banner: it is the one channel the
	// user already agreed to be interrupted on, and an unconfigured banner_cmd
	// makes Show a no-op, so this stays silent rather than half-working.
	pres := briefingPresenter(cfg)
	tracker := work.NewTracker(work.TrackerOptions{
		Store:      database,
		Apps:       cfg.Work.TrackedApps,
		Logger:     log,
		BreakAfter: time.Duration(cfg.Work.BreakAfterHours * float64(time.Hour)),
		Nudge: func(text string) {
			if err := pres.Show(context.Background(), text); err != nil {
				log.Warn("sessions: break nudge failed", "err", err)
			}
		},
	})
	if tracker == nil {
		return nil
	}

	// Recover before anything opens new rows: a daemon that was killed (or a
	// machine that lost power) left sessions open, and they are closed at the
	// last moment the app was actually seen. Doing this here rather than inside
	// the tracker keeps it strictly ordered against the watcher's baseline,
	// which would otherwise race it and have its fresh session closed as stale.
	if n, err := database.CloseOpenSessions(); err != nil {
		log.Warn("sessions: recovery failed", "err", err)
	} else if n > 0 {
		log.Info("sessions: closed sessions left open by a previous run", "count", n)
	}

	d.Register("sessions", tracker.Run)
	return tracker
}

// registerScheduler wires Pylon's clock-driven background jobs. For now these
// are GitHub's 15-minute PR poll and the daily commit-reminder (Phase 2.2);
// Phase 3's briefing/report will register here too. Jobs notify through the
// same TTS path the watcher uses (logging when TTS is off).
func registerScheduler(d *daemon.Daemon, cfg config.Config, database *db.DB, registry *services.Registry, log *slog.Logger) {
	sched := scheduler.New(scheduler.Options{Logger: log})

	var speaker voice.Speaker
	if len(cfg.Voice.TTSCmd) > 0 {
		speaker = voice.NewSpeaker(cfg.Voice.TTSCmd, cfg.Voice.PlayCmd)
	}
	notify := func(msg string) {
		log.Info("scheduler: notify", "msg", msg)
		if speaker == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := speaker.Say(ctx, msg); err != nil {
			log.Warn("scheduler: speak failed", "err", err)
		}
	}

	gh := cfg.Services.GitHub
	if ghsvc.Configured(githubConfig(cfg)) {
		if every, err := time.ParseDuration(gh.PollInterval); err == nil && every > 0 {
			poller := ghsvc.New(githubConfig(cfg)).NewPoller()
			sched.Every("github-pr-poll", every, func(ctx context.Context) {
				switch msg, ok, err := poller.Poll(ctx); {
				case err != nil:
					log.Warn("scheduler: github poll failed", "err", err)
				case ok:
					notify(msg)
				}
			})
			log.Info("scheduler: github PR poll enabled", "every", every)
		}
		if h, m, ok := parseHM(gh.CommitReminder); ok && len(gh.Repos) > 0 {
			cr := ghsvc.NewCommitReminder(gh.Repos)
			sched.DailyAt("github-commit-reminder", h, m, func(ctx context.Context) {
				if msg, ok := cr.Check(ctx); ok {
					notify(msg)
				}
			})
			log.Info("scheduler: commit reminder enabled", "at", gh.CommitReminder, "repos", len(gh.Repos))
		}
	}

	// Daily briefing: at the configured time, run the briefing action (which shows
	// the desktop banner) and speak the result. Silently idle if the time is
	// malformed.
	if h, m, ok := parseHM(cfg.Briefing.Time); ok {
		speaker := briefingSpeaker(cfg)
		deliver := func(ctx context.Context) {
			text, ok, err := registry.Dispatch(ctx, intent.Command{Action: briefing.ActionToday})
			if err != nil || !ok {
				log.Warn("scheduler: briefing failed", "err", err)
				return
			}
			// Recorded only after it was actually composed: a failed briefing
			// must stay eligible for the catch-up below.
			if database != nil {
				if err := database.SetContext(briefingLastRunKey, time.Now().Format(dayKey)); err != nil {
					log.Warn("scheduler: briefing last-run not saved", "err", err)
				}
			}
			if speaker != nil && strings.TrimSpace(text) != "" {
				if err := speaker.Say(ctx, text); err != nil {
					log.Warn("scheduler: briefing speak failed", "err", err)
				}
			}
		}
		sched.DailyAt("briefing", h, m, deliver)
		log.Info("scheduler: daily briefing enabled", "at", cfg.Briefing.Time)
		if database != nil {
			registerBriefingCatchup(d, database, h, m, deliver, log)
		}
	}

	d.Register("scheduler", sched.Run)
}

// briefingLastRunKey remembers, as a local YYYY-MM-DD, the day a briefing was
// last delivered.
const briefingLastRunKey = "briefing.last_run"

// dayKey is the date format both the stored key and the comparison use.
const dayKey = "2006-01-02"

// briefingCatchupDelay lets the session finish coming up before a missed
// briefing is composed. It reads calendar and news over the network, and at
// login the daemon is usually running before the network is.
const briefingCatchupDelay = 20 * time.Second

// registerBriefingCatchup delivers today's briefing once if it was missed.
//
// The scheduler only ever looks forward: seed() gives each daily job its *next*
// fire time, so a daemon that starts at 09:00 with the briefing set to 08:00
// schedules it for tomorrow and today's is simply lost. For someone who turns
// the machine on in the morning that means the briefing rarely ever runs.
//
// The stored day is what keeps this to once: restarting the daemon four times
// after a delivered briefing delivers nothing more.
func registerBriefingCatchup(d *daemon.Daemon, database *db.DB, hour, min int, deliver func(context.Context), log *slog.Logger) {
	d.Register("briefing-catchup", func(ctx context.Context) error {
		last, _, err := database.GetContext(briefingLastRunKey)
		if err != nil {
			log.Warn("scheduler: briefing last-run unreadable", "err", err)
			return nil
		}
		if !briefingMissed(time.Now(), hour, min, last) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(briefingCatchupDelay):
		}
		log.Info("scheduler: today's briefing was missed, catching up", "last_run", last)
		deliver(ctx)
		return nil
	})
}

// briefingMissed reports whether a briefing due at hour:min today has come due
// and has not been delivered. lastRun is the stored day, empty if never.
// Local time throughout, because that is the clock the scheduler fires on.
func briefingMissed(now time.Time, hour, min int, lastRun string) bool {
	due := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	if now.Before(due) {
		return false
	}
	return lastRun != now.Format(dayKey)
}

// parseHM parses an "HH:MM" 24-hour time. ok is false for empty or malformed input.
func parseHM(s string) (hour, min int, ok bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// socketPath resolves the daemon socket from config, falling back to the default.
func socketPath() string {
	cfg, err := loadConfig()
	if err != nil {
		return ipc.DefaultSocketPath()
	}
	return cfg.Paths.Socket
}

// cmdSay sends a text command to the daemon's intent engine.
func cmdSay(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pylon say <text>")
	}
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "say", Args: args})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// cmdDo runs a service action directly (no LLM): `pylon do <action> [k=v ...]`.
// Same path the GUI widgets use over IPC.
func cmdDo(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pylon do <action> [k=v ...]")
	}
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "do", Args: args})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// cmdWork reports where today's (or the week's) time went. It goes through the
// daemon rather than reading the database directly: the daemon holds the open
// sessions, so asking it is the only way to count the app you are in right now.
func cmdWork(args []string) error {
	action := work.ActionToday
	if len(args) > 0 {
		switch args[0] {
		case "today", "bugun", "bugün":
		case "week", "hafta", "haftalik", "haftalık":
			action = work.ActionWeek
		default:
			return fmt.Errorf("usage: pylon work [today|week] (bilinmeyen: %q)", args[0])
		}
	}
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "do", Args: []string{string(action)}})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// cmdBriefing runs the briefing action, which shows the desktop banner, and
// prints the text. Bind this to a hotkey for an on-demand briefing; the
// scheduler fires the same action daily, and saying "brifing ver" does too.
//
// --speak reads it aloud as well. Speaking is opt-in rather than the default
// because a briefing bound to a hotkey is often wanted quietly — and once
// spoken there is no way to take it back.
func cmdBriefing(args []string) error {
	req, timeout, err := briefingRequest(args)
	if err != nil {
		return err
	}
	resp, err := daemon.SendTimeout(socketPath(), req, timeout)
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// briefingRequest maps the command line onto a request and the deadline it
// needs. Split out from cmdBriefing so the argument handling is testable
// without a daemon to talk to.
func briefingRequest(args []string) (ipc.Request, time.Duration, error) {
	switch {
	case len(args) == 0:
		return ipc.Request{Cmd: "do", Args: []string{string(briefing.ActionToday)}}, 30 * time.Second, nil
	case len(args) == 1 && (args[0] == "--speak" || args[0] == "-s"):
		// Reading it out takes as long as the briefing is, which is well past
		// the default deadline.
		return ipc.Request{Cmd: "briefing", Args: []string{"speak"}}, 3 * time.Minute, nil
	default:
		return ipc.Request{}, 0, errors.New("usage: pylon briefing [--speak]")
	}
}

// cmdListen runs one push-to-talk cycle: record from the mic, transcribe it,
// send the text through the daemon's intent engine, and speak the reply. Bind
// this to a hotkey in your DE/OS (hyprland, AutoHotkey, Hammerspoon, ...).
func cmdListen() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	pipe := voice.NewPipeline(voiceOptions(cfg))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	secs := cfg.Voice.RecordSeconds
	if secs <= 0 {
		secs = 5
	}
	if cfg.Voice.SilenceStop {
		fmt.Fprintf(os.Stderr, "dinliyorum — konuş, susunca duracağım (en fazla %d sn)...\n", secs)
	} else {
		fmt.Fprintf(os.Stderr, "dinliyorum (%d sn) — konuş ve bekle, Ctrl+C YAPMA...\n", secs)
	}
	text, err := pipe.Capture(ctx)
	if voice.IsNoSpeech(err) {
		fmt.Fprintln(os.Stderr, "ses algılanamadı")
		return nil
	}
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		fmt.Fprintln(os.Stderr, "ses algılanamadı")
		return nil
	}
	fmt.Printf("» %s\n", text)

	resp, err := daemon.Send(cfg.Paths.Socket, ipc.Request{Cmd: "say", Args: []string{text}})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	if err := pipe.Speak(ctx, resp.Text); err != nil {
		// Speaking is best-effort; the text reply already printed.
		fmt.Fprintln(os.Stderr, "seslendirme hatası:", err)
	}
	return nil
}

// cmdAuth runs a service authorization flow: `pylon auth google`,
// `pylon auth spotify`, or either with `logout` to forget the saved token.
func cmdAuth(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pylon auth <google|spotify> [logout]")
	}
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Signing out is the same everywhere: drop the token and let the next start
	// re-derive what is available. Handled before the per-service branches so
	// each one only has to describe its consent flow.
	if len(args) > 1 && args[1] == "logout" {
		logout := map[string]func() error{"google": google.Logout, "spotify": spotify.Logout}[args[0]]
		if logout == nil {
			return fmt.Errorf("usage: pylon auth <google|spotify> logout (bilinmeyen: %q)", args[0])
		}
		if err := logout(); err != nil {
			return err
		}
		fmt.Printf("✔ %s bağlantısı kaldırıldı.\n", args[0])
		return nil
	}

	switch args[0] {
	case "google":
		gcfg := googleConfig(cfg)
		if !google.HasClient(gcfg) {
			return errors.New("bu Pylon derlemesine Google OAuth client'ı gömülmemiş. " +
				"Yapımcı: `make build GOOGLE_CLIENT_ID=... GOOGLE_CLIENT_SECRET=...` ile göm, " +
				"ya da services.google.client_id / client_secret ayarla")
		}
		fmt.Println("Google ile giriş yap — tarayıcı açılıyor...")
		if err := google.Authorize(ctx, gcfg); err != nil {
			return err
		}
		fmt.Println("✔ Giriş tamam — artık takvimine ve Drive'ına erişebilirim.")
		return nil

	case "spotify":
		scfg := spotifyConfig(cfg)
		if !spotify.HasClient(scfg) {
			return errors.New("bu Pylon derlemesine Spotify OAuth client'ı gömülmemiş. " +
				"Yapımcı: `make build SPOTIFY_CLIENT_ID=...` ile göm, " +
				"ya da services.spotify.client_id ayarla (Redirect URI olarak " +
				spotify.RedirectURI(scfg) + " kayıtlı olmalı)")
		}
		fmt.Println("Spotify ile giriş yap — tarayıcı açılıyor...")
		if err := spotify.Authorize(ctx, scfg); err != nil {
			return err
		}
		fmt.Println("✔ Spotify bağlandı.")
		return nil

	default:
		return fmt.Errorf("usage: pylon auth <google|spotify> (bilinmeyen: %q)", args[0])
	}
}

// cmdSecret manages credentials in Pylon's encrypted vault (AES-256-GCM, under
// the user config dir). Secrets are referenced from config as "secret:<name>",
// so saving one here is all that's needed:
//
//	pylon secret set freshrss     # prompts (no echo), or reads piped stdin
//	pylon secret rm  freshrss
//
// This is the CLI stand-in for the future settings UI — both call internal/secrets.
func cmdSecret(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: pylon secret <set|rm> <name>")
	}
	action, name := args[0], args[1]
	switch action {
	case "set":
		value, err := readSecretValue(name)
		if err != nil {
			return err
		}
		if err := secrets.Set(name, value); err != nil {
			return err
		}
		fmt.Printf("✔ %q şifrelenip kaydedildi. Config'de: secret:%s\n", name, name)
		return nil
	case "rm", "delete", "remove":
		if err := secrets.Delete(name); err != nil {
			return err
		}
		fmt.Printf("✔ %q silindi.\n", name)
		return nil
	default:
		return fmt.Errorf("usage: pylon secret <set|rm> <name> (bilinmeyen: %q)", action)
	}
}

// readSecretValue reads the secret from a no-echo terminal prompt, or from
// stdin when piped (e.g. `cat token | pylon secret set github`).
func readSecretValue(name string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("%s değerini gir (görünmez): ", name)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("okuma: %w", err)
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("stdin okuma: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// cmdRecall prints recent conversation memory from the daemon.
func cmdRecall(args []string) error {
	resp, err := daemon.Send(socketPath(), ipc.Request{Cmd: "recall", Args: args})
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
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
