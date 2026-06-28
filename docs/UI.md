# Pylon UI — Design

> Status: **planning**. Design evolves through the build; this captures the agreed
> starting shape and the architecture chosen so it doesn't block the character system.

## Vision

Pylon is voice-first and runs as a headless daemon. The UI is an **optional
companion window**, not the brain. The main assistant is **Pylon**; later, extra
**characters** join it in a left sidebar (PLANNED Phase 5). Start simple, grow.

## Stack

- **Wails v2** — Go backend + web frontend, one cross-platform desktop binary.
  Chosen so animations / sprites / characters are easy later (web tech), while the
  backend stays Go and reuses the existing packages.
- **Frontend: Svelte** (proposed) — small, fast, no virtual-DOM overhead; good for
  a widget dashboard and later canvas/sprite animation.

## Architecture — the GUI is a daemon client

The daemon stays the single source of truth and runs independently. The Wails app
is **another IPC client of the daemon** (like the CLI), talking over the existing
Unix socket (`/tmp/pylon.sock`, `internal/ipc` JSON Request/Response).

```
┌────────────────────┐        ┌──────────────────────────┐
│ Wails GUI (Go+web)  │  IPC   │ pylon daemon (headless)  │
│  - reads widget data│◄──────►│  socket /tmp/pylon.sock  │
│  - sends "say" etc. │ socket │  services, intent, tasks │
│  - writes secrets ──┼──┐     └──────────────────────────┘
└────────────────────┘  │ direct (same module)
                         ▼
                 internal/secrets (AES vault)
```

- **Widget data + actions** → IPC to the daemon (`status`, `say`, `recall`, plus new
  read commands per widget, e.g. calendar-today / rss-unread / github-prs).
- **Secrets** ("parola ekle" button) → the GUI calls `internal/secrets` **directly**
  (local AES file vault); no daemon round-trip needed. Same code the CLI uses.
- If the daemon isn't running, the GUI can offer to start it (spawn `pylon start`).

## Layout

**Pylon is the main character — not one of the sidebar icons.** It is always
present on the home stage (the host you talk to). The sidebar holds only the
*secondary* characters (Phase C) plus settings. Pylon is differentiated by being
the persistent figure on the stage, never a peer in the character list.

**Pylon sits in the center; widgets flank it left and right.** Pylon is the fixed
focal point of the home; the service widgets are arranged in columns on either side.

```
┌────┬───────────────────────────────────────────┐
│ S  │  ┌──────────┐     ╭─────────╮    ┌────────┐ │
│ I  │  │📅 Takvim │     │         │    │🐙 GitHub│ │
│ D  │  │ bugün 2  │     │  PYLON  │    │0 beklyn │ │
│ E  │  └──────────┘     │ (merkez)│    └────────┘ │
│ B  │  ┌──────────┐     │         │    ┌────────┐ │
│ A  │  │📰 FreshRSS│     ╰─────────╯    │ ...    │ │
│ R  │  │ 5206     │      her zaman      └────────┘ │
│ ⚙  │  └──────────┘       ortada                   │
└────┴───────────────────────────────────────────┘
  └ ikincil karakterler (sonra) + ⚙ ayar
```

- **Pylon (ana karakter):** the fixed **center** of the home — the persistent presence
  you address directly. Widgets never overlap it; they live in the left/right columns.
  Distinct from the sidebar characters by being centered and framed, never a peer icon.
  *Possible later:* promote it to a **floating avatar** that overlays the whole app
  (user noted this as a future option).
- **Sidebar (sol bar):** *only* the secondary characters (added in Phase C) and a ⚙
  settings entry. In the MVP it is nearly empty (just ⚙) since no characters exist yet.
- **Home (ana sayfa):** Pylon centered, with **widgets in left & right columns** — each
  a service's at-a-glance view. Widgets are enabled/disabled from Settings; none on Home.
- **Settings (ayarlar):** opened from the sidebar ⚙, a separate view. Holds service
  config, the **secret entry** ("type password → save" → AES vault), widget toggles,
  and voice settings.

## MVP widgets (map to existing services)

| Widget   | Source (daemon)            | Shows                    |
|----------|----------------------------|--------------------------|
| Takvim   | `calendar.list_today`      | today's events / count   |
| FreshRSS | `freshrss.unread_count`    | unread count (5206)      |
| GitHub   | `github.list_prs`          | review-requested / open  |

(Each widget is read-only at first; clicking opens detail later.)

## Roadmap — simple first, evolution-ready

- **Phase A — MVP shell.** Wails skeleton; sidebar (Pylon + ⚙) and home widget grid
  (read-only, polling the daemon); Settings view with secret entry + service/widget
  toggles. Add the daemon IPC read-commands the widgets need.
- **Phase B — interaction.** Talk to Pylon from the UI (text box → `say`; later a mic
  button → the voice pipeline). Live updates instead of polling. Conversation history.
- **Phase C — characters.** The sidebar gains characters; tapping one enters its
  domain; it animates out to perform a task and returns. This is PLANNED Phase 5 —
  design-first session (grouping, art style, lore) before building.

## Implementation (Phase A — in progress)

Scaffolded with `wails init -t svelte` in **`pylon-ui/`** — a **separate Go module**
on purpose: the daemon's `go build ./...` / `go test ./...` never descend into it, so
they stay CGo-free (Wails needs CGo + webkit). The GUI talks to the daemon over the
socket and carries its own tiny copy of the wire protocol; it imports nothing from the
daemon module.

- **Widget data path:** the daemon gained a **`do <action>`** IPC command (and `pylon do`
  CLI) that dispatches a service action through the registry **without the LLM**. Widgets
  call `App.Do("freshrss.unread_count")` → the daemon runs the service → text back.
  Live-verified headless: `pylon do freshrss.unread_count` → "5204 okunmamış haberin var."
- **Go backend** (`pylon-ui/app.go`): bound methods `DaemonRunning()`, `Status()`,
  `Do(action)` — a thin Unix-socket client.
- **Frontend** (`pylon-ui/frontend/src/`): `App.svelte` (3-zone layout) + `lib/Sidebar.svelte`,
  `lib/PylonStage.svelte` (centered orb), `lib/Widget.svelte` (calls `Do`). Builds clean
  (`npm run build`). Settings is a stub — detailed design deferred to step 4 (per user).

### Build & run

Linux needs **webkit2gtk** (the one missing system dep): `sudo pacman -S webkit2gtk-4.1`.
Since that is the **4.1** variant (Wails defaults to 4.0), build with the `webkit2_41`
tag. With the daemon running (`pylon start`):

```
cd pylon-ui
wails dev   -tags webkit2_41    # hot-reload dev window
wails build -tags webkit2_41    # produces build/bin/pylon-ui
```

## Open decisions

- Settings view — full design at **step 4** (secret entry, service/widget toggles, voice).
- Widget detail interactions (Phase B+).
- Settings ↔ daemon: which config is editable live vs needs a restart.
- Window chrome / frameless + custom titlebar (later, ties into the character look).
