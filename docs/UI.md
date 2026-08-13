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
│    │                  [___] [Gönder]              │
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

**VoiceBar** — two ways into one intent engine. The mic button calls `Listen()`,
which runs the daemon's whole voice pipeline (record → transcribe → intent →
speak) and answers with `» <heard>\n<reply>`; the bar splits that into what it
heard and what it said. The box under it calls `Ask()` — the daemon's `say`, the
same command the CLI sends — so the GUI still works with the mic off. Both land
in the same answer bubble, and only one can run at a time.

A turn can be stopped: Escape, or the mic button, which becomes **Durdur** while
the microphone is open. Before that there was no way out of a turn started by
accident — Escape did nothing, the button was disabled, and the only thing that
ended it was the silence timer. The stop cannot travel on `Listen()`'s own
connection, which stays blocked until the turn finishes, so it is a second
request (`listen cancel`) on a second connection; the daemon serves each
connection in its own goroutine, which is what makes that work at all. It
cancels the turn's context, so `sox` is interrupted rather than killed and the
WAV still closes cleanly. Stopping a turn that has just ended is a no-op, not an
error — Escape losing that race is ordinary. The same bookkeeping makes the
microphone single-user: a second `listen` while one is open is refused, because
two open devices would leave the cancel no way to say which turn it meant.

**Docker page** (`DockerPage.svelte`) — a full-screen container manager, shown
when the Docker page is pinned to the dock. List or grid, all/running filter,
optional auto-refresh; preferences in `localStorage` under
`pylon.dockerpage.v1`.

**Settings** (`Settings.svelte`) — four tabs, so each screen answers one
question:

| Tab | Holds |
| --- | --- |
| **Genel** | `Language.svelte` — the language Pylon speaks |
| **Görünüm** | Widget instances (add/edit/remove via a dialog) and which pages are pinned to the dock |
| **Hesaplar** | `Accounts.svelte` (OAuth sign-in) and `ApiKeys.svelte` (vault keys) |
| **Ses** | `VoiceSettings.svelte` — the push-to-talk shortcut |

Language is the first tab because it is the one setting you go looking for when
you cannot read any of the others.

The tablist is keyboard-navigable (arrow keys, roving tabindex) and the widget
editor is a real dialog: it takes focus, traps Tab, closes on Escape, and hands
focus back on close.

"Hands focus back" needs the trigger to still exist, and twice it does not:
deleting the widget removes its row, and the pen icon on the home screen unmounts
that whole page on its way to Görünüm. So the dialog walks a list — the trigger,
then the row for the widget that was being edited, then the button that adds one —
and something in it is always on screen. Before that, a keyboard user who opened
the dialog from the home screen landed back at the top of the document every time.

Focus itself is visible everywhere, from one `:focus-visible` rule in
`style.css`. It used to be visible in five places (the tablist and some text
fields) and left to the engine's default outline elsewhere, which against these
dark surfaces is not visible at all: tabbing across the sidebar, a widget's pen
icon or **Konuş** showed nothing. A control with its own focus treatment still
wins — the rule is deliberately the least specific one in the interface.

### Language

The GUI has no language setting of its own. `Language.svelte` calls the daemon
(`App.SetLanguage` → `lang set`), which switches immediately and remembers the
choice. One setting, so the buttons around an answer are never in a different
language from the answer.

`App.LanguageState` (→ `lang state`) returns
`<speaking>\t<chosen>\t<source>\t<detail>`, and the card needs every field.
`Language()` alone cannot tell a chosen Turkish from one that merely followed
something else, so **Otomatik** could never show as selected. And knowing only
that nothing was chosen is not enough either: the fallback may be `language:`
in `pylon.yaml` or the desktop locale, and the card names which — a button
reading "system language" above a value that came out of `pylon.yaml` is untrue
on exactly the machines whose owner will spot it. `detail` carries the config
path or the winning `LC_ALL`/`LC_MESSAGES`/`LANG` assignment, which is what
`pylon lang` prints to stderr.

Interface strings live in `src/lib/locales/*.json`, separate from the daemon's
catalogs — the GUI is its own Go module and cannot import `internal/i18n`, and
the vocabularies barely overlap ("Cancel" vs. "3 events in your calendar").

`npm run check:i18n` (also part of `npm run build`, so CI runs it) guards three
things that nothing else catches: every language carries the same keys with the
same `{0}` placeholders; every `ui.*` name in the code exists; and no catalog
key is rendered without `$t()`. A line that is deliberately not translatable —
a widget the user renamed, a product name — says `i18n-raw` above it.

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
| `CancelListen()` | stop the turn that is running (Escape, the stop button) |
| `RestartDaemon()` | bounce the daemon so new config/credentials take effect |
| `Platform()` | which desktop this is (`hyprland`, `sway`, `gnome`, `kde`, `macos`, `windows`, …) |
| `Hotkey()` / `SetHotkey(combo)` | read and change the push-to-talk shortcut |
| `SetSecret` / `HasSecret` / `DeleteSecret` | write a vault key, ask whether one exists (never read it back), remove it |
| `AuthStatus` / `AuthLogin` / `AuthLogout` | per service (`google`, `spotify`): connection state, the browser consent, signing out |

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
- **Service toggles.** A service is on when it is configured; there is no switch
  in Settings, and the screen says so.
- **Characters.** An early plan had a sidebar of secondary assistants. It was
  never started, and the dock became a page launcher instead.
