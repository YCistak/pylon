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
	"strconv"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/config"
	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/db"
	"github.com/YCistak/pylon/internal/intent"
	"github.com/YCistak/pylon/internal/ipc"
	"github.com/YCistak/pylon/internal/profile"
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
	case "recall":
		err = cmdRecall(os.Args[2:])
	case "listen":
		err = cmdListen()
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
	registerIntent(d, cfg, database, log)

	return d.Run(context.Background())
}

// registerIntent wires the two-tier intent path into the daemon's "say"
// command: the local Router runs first (free), and only unresolved input falls
// back to the configured LLM chain, which tries each model in order and falls
// through on quota/rate-limit.
func registerIntent(d *daemon.Daemon, cfg config.Config, database *db.DB, log *slog.Logger) {
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

		resp := executeCommand(cmd, database)
		rememberTurn(database, text, resp, log)
		return resp
	})

	registerRecall(d, database)
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
// Shared by the local router and the Gemini fallback so both behave identically.
func executeCommand(cmd intent.Command, database *db.DB) ipc.Response {
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
