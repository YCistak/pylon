package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/config"
	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/feedback"
	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/ipc"
)

// feedbackEnv gathers the facts attached to a report. It reads only what is
// already in this process — the version it was built with, the platform it is
// running on, the language it is speaking — and nothing off disk. The form
// shows the user this exact line before sending, so the list has to stay short
// enough to read.
func feedbackEnv() feedback.Env {
	desktop := ""
	if runtime.GOOS == "linux" {
		// The raw value, not a normalised one: "Hyprland" and "hyprland:wlroots"
		// mean different things to whoever reads the issue, and flattening them
		// throws away the distinction at exactly the wrong moment.
		desktop = strings.TrimSpace(os.Getenv("XDG_CURRENT_DESKTOP"))
	}
	return feedback.Env{
		Version:  version,
		OS:       runtime.GOOS,
		Desktop:  desktop,
		Language: i18n.Language(),
	}
}

// registerFeedback exposes the form's two questions over IPC.
//
// "feedback env" is what the form shows under the box before anything is sent,
// so nothing leaves the machine that the user has not already seen on screen.
//
// "feedback send <category> <body>" files it. The token is the user's own, the
// one already in the vault for the GitHub widget: Pylon has no server of its
// own and inventing an identity to post under would be worse than asking. With
// no token there is still somewhere to go — the reply carries a prefilled
// new-issue URL and the window opens it — because a Send button that dead-ends
// is worse than one that changes its mind about how.
func registerFeedback(d *daemon.Daemon, cfg config.Config) {
	d.Handle("feedback", func(req ipc.Request) ipc.Response {
		if len(req.Args) == 0 {
			return ipc.Response{OK: false, Error: "usage: feedback <env|send> [category] [body]"}
		}

		switch req.Args[0] {
		case "env":
			return ipc.Response{OK: true, Text: feedbackEnv().Line()}

		case "send":
			if len(req.Args) < 3 {
				return ipc.Response{OK: false, Error: "usage: feedback send <category> <body>"}
			}
			category, body := req.Args[1], req.Args[2]
			if !feedback.Valid(category) {
				return ipc.Response{OK: false, Error: "unknown category: " + category}
			}
			if strings.TrimSpace(body) == "" {
				return ipc.Response{OK: false, Error: i18n.T("feedback.empty")}
			}

			r := feedback.Report{Category: category, Body: body, Env: feedbackEnv()}

			// "<how>\t<url>": sent, and the issue to look at; or browser, and
			// the page to open. The GUI has to act differently on the two, and
			// telling them apart by matching on translated prose would break in
			// six languages at once — the same reason "update check" answers in
			// fields rather than a sentence.
			token := resolveSecret(cfg.Services.GitHub.Token)
			if strings.TrimSpace(token) == "" {
				return ipc.Response{OK: true, Text: "browser\t" + r.BrowserURL()}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			issue, err := feedback.Submit(ctx, token, r)
			if err != nil {
				// Falling back rather than failing: a token that cannot open
				// issues is a permissions detail the user cannot fix from here,
				// and their words are still worth delivering.
				return ipc.Response{OK: true, Text: "browser\t" + r.BrowserURL()}
			}
			return ipc.Response{OK: true, Text: "sent\t" + issue}

		default:
			return ipc.Response{OK: false, Error: fmt.Sprintf("unknown operation: %s", req.Args[0])}
		}
	})
}
