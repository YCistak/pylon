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
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/YCistak/pylon/internal/briefing"
	"github.com/YCistak/pylon/internal/config"
	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/intent"
	"github.com/YCistak/pylon/internal/ipc"
	"github.com/YCistak/pylon/internal/profile"
	"github.com/YCistak/pylon/internal/scheduler"
	"github.com/YCistak/pylon/internal/secrets"
	"github.com/YCistak/pylon/internal/services"
	"github.com/YCistak/pylon/internal/services/docker"
	"github.com/YCistak/pylon/internal/services/freshrss"
	ghsvc "github.com/YCistak/pylon/internal/services/github"
	"github.com/YCistak/pylon/internal/services/google"
	"github.com/YCistak/pylon/internal/services/spotify"
	"github.com/YCistak/pylon/internal/voice"
	"github.com/YCistak/pylon/internal/watcher"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

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
	case "say":
		err = cmdSay(os.Args[2:])
	case "do":
		err = cmdDo(os.Args[2:])
	case "recall":
		err = cmdRecall(os.Args[2:])
	case "listen":
		err = cmdListen()
	case "auth":
		err = cmdAuth(os.Args[2:])
	case "secret":
		err = cmdSecret(os.Args[2:])
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
  pylon recall [n]    show the last n remembered turns (default 5)
  pylon listen        push-to-talk: record, transcribe, run, speak the reply
  pylon auth google   authorize Google (Calendar) — one-time OAuth consent
  pylon secret set <name>   save a credential to the encrypted vault (ref as secret:<name>)
  pylon secret rm  <name>   remove a saved credential
`)
}

// cmdStart loads config, opens the database, and runs the daemon in the
// foreground until interrupted.
func cmdStart() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

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

	// Services are built once and shared: the intent engine dispatches user
	// commands to them, and the scheduler's morning briefing composes their
	// replies. Registering their actions teaches the LLM the vocabulary.
	registry := buildServiceRegistry(cfg, log)
	intent.SetActions(registry.Specs()...)

	registerWatcher(d, cfg, database, log)
	registerScheduler(d, cfg, registry, log)
	registerIntent(d, cfg, database, registry, log)

	return d.Run(context.Background())
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

	d.Handle("say", func(req ipc.Request) ipc.Response {
		text := strings.TrimSpace(strings.Join(req.Args, " "))
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

	registerRecall(d, database)
}

// buildServiceRegistry registers the external services that are configured and
// authorized. Each service contributes actions to the LLM vocabulary.
func buildServiceRegistry(cfg config.Config, log *slog.Logger) *services.Registry {
	var svcs []services.Service

	gcfg := googleConfig(cfg)
	switch {
	case google.Configured(gcfg):
		svcs = append(svcs, google.NewCalendar(gcfg), google.NewDrive(gcfg))
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
		svcs = append(svcs, freshrss.New(fcfg))
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

	// The morning briefing composes the other services' replies, so it registers
	// last and is wired to dispatch through the very registry it lives in.
	brief := briefing.New()
	svcs = append(svcs, brief)
	reg := services.NewRegistry(svcs...)
	brief.SetDispatcher(reg)
	return reg
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
		key := os.Getenv(m.APIKeyEnv)
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
		// System/media actions are executed by the system module (Phase 3).
		return ipc.Response{OK: true, Text: fmt.Sprintf("komut anlaşıldı: %s (conf %.2f) — eylem Faz 3'te bağlanacak", cmd.Action, cmd.Confidence)}
	}
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

	// Reminders are spoken via the same TTS the assistant uses (edge-tts). Built
	// once; nil when TTS isn't configured, in which case we just log.
	var speaker voice.Speaker
	if len(cfg.Voice.TTSCmd) > 0 {
		speaker = voice.NewSpeaker(cfg.Voice.TTSCmd, cfg.Voice.PlayCmd)
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
		},
	})

	d.Register("watcher", w.Run)
}

// registerScheduler wires Pylon's clock-driven background jobs. For now these
// are GitHub's 15-minute PR poll and the daily commit-reminder (Phase 2.2);
// Phase 3's briefing/report will register here too. Jobs notify through the
// same TTS path the watcher uses (logging when TTS is off).
func registerScheduler(d *daemon.Daemon, cfg config.Config, registry *services.Registry, log *slog.Logger) {
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

	// Morning briefing: speak the composed briefing.today every day at the
	// configured time. The registry owns the briefing service (buildServiceRegistry).
	if cfg.Briefing.Enabled {
		if h, m, ok := parseHM(cfg.Briefing.Time); ok {
			sched.DailyAt("morning-briefing", h, m, func(ctx context.Context) {
				if text, ok, err := registry.Dispatch(ctx, intent.Command{Action: briefing.ActionToday}); ok && err == nil {
					notify(text)
				} else if err != nil {
					log.Warn("scheduler: briefing failed", "err", err)
				}
			})
			log.Info("scheduler: morning briefing enabled", "at", cfg.Briefing.Time)
		} else {
			log.Warn("scheduler: briefing time malformed, disabled", "time", cfg.Briefing.Time)
		}
	}

	d.Register("scheduler", sched.Run)
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
	cfg, err := config.Load(configPath())
	if err != nil {
		return ipc.DefaultSocketPath
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

// cmdListen runs one push-to-talk cycle: record from the mic, transcribe it,
// send the text through the daemon's intent engine, and speak the reply. Bind
// this to a hotkey in your DE/OS (hyprland, AutoHotkey, Hammerspoon, ...).
func cmdListen() error {
	cfg, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	pipe := voice.NewPipeline(voice.Options{
		STTBin:        cfg.Voice.STTBin,
		STTModel:      cfg.Voice.STTModel,
		Language:      cfg.Voice.Language,
		TTSCmd:        cfg.Voice.TTSCmd,
		RecordCmd:     cfg.Voice.RecordCmd,
		RecordSeconds: cfg.Voice.RecordSeconds,
		PlayCmd:       cfg.Voice.PlayCmd,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	secs := cfg.Voice.RecordSeconds
	if secs <= 0 {
		secs = 5
	}
	fmt.Fprintf(os.Stderr, "dinliyorum (%d sn) — konuş ve bekle, Ctrl+C YAPMA...\n", secs)
	text, err := pipe.Capture(ctx)
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

// cmdAuth runs a service authorization flow. Currently: `pylon auth google`.
func cmdAuth(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pylon auth <google|spotify>")
	}
	cfg, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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
