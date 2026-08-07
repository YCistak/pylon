# Pylon UI

The GUI as it is built. This file used to be a design plan written before the
code; it now describes the code, because the plan and the program had drifted
far enough apart that the document was worse than nothing — it claimed, among
other things, an architecture the GUI does not and cannot have.

For running and building it, see [`../pylon-ui/README.md`](../pylon-ui/README.md).

## The GUI is a daemon client, and only that

The daemon owns every piece of state. The GUI is another IPC client of it, like
the CLI: it opens the Unix socket, sends a JSON request, reads one back.

```
┌──────────────────────┐        ┌──────────────────────────┐
│ pylon-ui (Go + web)  │  IPC   │ pylon daemon (headless)   │
│  own Go module       │◄──────►│  /tmp/pylon.sock          │
│  imports nothing     │ socket │  services, intent, memory │
│  from the daemon     │        │  secrets vault            │
└──────────────────────┘        └──────────────────────────┘
```

`pylon-ui/` is a **separate Go module** on purpose. Wails needs CGo and webkit;
keeping it out of the daemon's module is what lets `go build ./...` stay
CGo-free and cross-compile to three platforms. The cost is that the GUI cannot
import the daemon's packages at all, so it carries its own small copy of the
wire protocol (`request`/`response` in `app.go`), kept in sync by hand.

That constraint is worth stating plainly because it is easy to get backwards:
**the GUI never touches the secrets vault directly.** It cannot — `internal/secrets`
lives in the other module. Saving an API key is `SetSecret` → IPC → the daemon's
`secret set` handler → the same AES-256-GCM vault the CLI writes.

If no daemon is running, the GUI starts one itself (`app.go` `startup` →
`daemon.go` `ensureRunning`) and stops it again on exit — but only the one it
started. A daemon you launched yourself is left alone.

## Screens

`App.svelte` holds a three-zone layout: the dock on the left, and one of home,
settings, or a pinned page filling the rest.

```
┌────┬──────────────────────────────────────────────┐
│ P  │   ┌──────────┐                  ┌──────────┐ │
│    │   │ Takvim   │      ╭──────╮    │ GitHub   │ │
│ 🐳 │   └──────────┘      │ orb  │    └──────────┘ │
│    │   ┌──────────┐      ╰──────╯    ┌──────────┐ │
│    │   │ FreshRSS │       Pylon      │ Sistem   │ │
│    │   └──────────┘   [ 🎤 Konuş ]   └──────────┘ │
│ ●  │                                              │
│ ⚙  │        sol sütun          sağ sütun          │
└────┴──────────────────────────────────────────────┘
```

**Dock** (`Sidebar.svelte`) — 72px, expanding to 240px on hover (the workspace
dims while it is open). Holds the brand button (back to home), any pinned pages,
a daemon status dot, and settings.

**Home** — `PylonStage.svelte` (the orb) centred, with `VoiceBar.svelte` under
it and widget instances in a left and a right column. Empty on first launch;
widgets are added from Settings.

**VoiceBar** — push-to-talk. Calls `Listen()`, which runs the daemon's whole
voice pipeline (record → transcribe → intent → speak) and answers with
`» <heard>\n<reply>`; the bar splits that into what it heard and what it said.

**Docker page** (`DockerPage.svelte`) — a full-screen container manager, shown
when the Docker page is pinned to the dock. List or grid, all/running filter,
optional auto-refresh; preferences in `localStorage` under
`pylon.dockerpage.v1`.

**Settings** (`Settings.svelte`) — three tabs, so each screen answers one
question:

| Tab | Holds |
| --- | --- |
| **Görünüm** | Widget instances (add/edit/remove via a dialog) and which pages are pinned to the dock |
| **Hesaplar** | `Accounts.svelte` (OAuth sign-in) and `ApiKeys.svelte` (vault keys) |
| **Ses** | `VoiceSettings.svelte` — the push-to-talk shortcut |

The tablist is keyboard-navigable (arrow keys, roving tabindex) and the widget
editor is a real dialog: it takes focus, traps Tab, closes on Escape, and hands
focus back on close.

## Widgets

A widget is an **instance**, not a toggle: you can have two GitHub cards showing
different things. Each carries `{id, type, title, column, mode, params, refresh,
accent}` and lives in `localStorage` under `pylon.widgets.v2` (`widgets.js`
migrates a `v1` layout once, and drops any instance whose type has left the
catalog).

Eight types, twelve modes:

| Type | Modes |
| --- | --- |
| `calendar` | today's events |
| `freshrss` | unread count |
| `github` | open PRs · open issues |
| `drive` | recent files · search |
| `weather` | current conditions |
| `spotify` | now playing |
| `sysmon` | machine vitals |
| `docker` | container list · one container (rich card) |

Every widget reads through one command: `Do(action, params)` → the daemon's
`do` handler → the service registry, **skipping the LLM**. So a widget shows
exactly what `pylon do <action>` prints in a terminal.

`DockerWidget.svelte` is the exception to "widgets are read-only": it shows
state and CPU/memory, and can start, stop and restart the container.

Pinned pages work the same way (`sidebarPages.js`, `pylon.sidebar.v1`), with one
entry in the catalog so far: Docker.

## Talking to the daemon

`app.go` binds these to the frontend:

| Method | Does |
| --- | --- |
| `DaemonRunning()` | is the socket answering |
| `Status()` | daemon status line |
| `Do(action, params)` | run a service action, no LLM — the widget data path |
| `Listen()` | one full push-to-talk turn |
| `RestartDaemon()` | bounce the daemon so new config/credentials take effect |
| `Platform()` | which desktop this is (`hyprland`, `sway`, `gnome`, `kde`, `macos`, `windows`, …) |
| `Hotkey()` / `SetHotkey(combo)` | read and change the push-to-talk shortcut |
| `SetSecret` / `HasSecret` | write a vault key, ask whether one exists (never read it back) |
| `GoogleStatus()` / `GoogleLogin()` | account state and the browser OAuth consent |

Timeouts differ by what is behind them: 20 s by default, 70 s for `Listen`
(someone has to finish talking), 5 minutes for a sign-in that opens a browser.

`daemon.js` is one shared readable store polling `DaemonRunning()` every 1.5 s —
`null` while probing, then `true`/`false`. Everything subscribes to it rather
than keeping its own timer, so the whole window flips to "online" together the
moment a cold-launched daemon answers.

Polling is not the end state; it is what is there. Push updates from the daemon
were planned and have not been built.

## The shortcut

Wayland gives an application no way to grab a global hotkey for itself, and
editing the compositor's config means writing a file Pylon does not own and
cannot cleanly take back. So the daemon registers the binding over the
compositor's control socket instead — Hyprland and Sway both accept one — and
re-applies it on every start. Nothing on disk changes.

On desktops with no such socket, `VoiceSettings.svelte` shows the line to add by
hand instead of pretending it worked.

## Not built

Named because the absence is deliberate, not forgotten:

- **Push updates.** Still polling (see above).
- **Conversation history.** The voice bar shows one turn and forgets it.
- **A text box.** Typing to Pylon goes through the CLI (`pylon say`); the GUI
  went straight to the microphone.
- **Service toggles.** A service is on when it is configured; there is no switch
  in Settings, and the screen says so.
- **Characters.** An early plan had a sidebar of secondary assistants. It was
  never started, and the dock became a page launcher instead.
