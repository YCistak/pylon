# Changelog

Notable changes per release. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semver](https://semver.org/), and `internal/selfupdate` compares them by
semver precedence — including prerelease ordering.

Cutting a release: [docs/RELEASE.md](docs/RELEASE.md).

## [Unreleased]

### Added

- **The briefing now opens with the weather.** The slot had been left empty
  since the briefing was written. It reads the raw forecast from the same
  `weather` service the spoken action uses and phrases its own short clause —
  condition, current temperature, today's high — rather than quoting weather's
  full sentence. Rain is mentioned only from 30% up, and a failed fetch drops
  the clause instead of announcing that the weather is unavailable.

- **A text box in the GUI.** Under the mic button, and it goes to the same
  place: `Ask()` → the daemon's `say` → the intent engine. Until now the only
  way in from the GUI was the microphone, which is no use in a quiet room or
  when a container name is easier to type than to pronounce. Enter sends, the
  answer lands in the same bubble the mic uses, and the two cannot run at once.

- **A stand-up nudge** (`work.break_after_hours`, default 2, `0` disables).
  Once a tracked app has been open that long with no gap, the briefing banner
  says how long it has been and suggests a break, repeating every 30 minutes
  while the stretch lasts. Closing every tracked app is what counts as the
  break — it measures how long the app was open, not how long you were at the
  keyboard, which errs toward reminding you too often rather than too late.

- **A systemd user unit** (`packaging/user/pylon.service`), installed by
  `make install-user`. Installed but not enabled: `WantedBy=graphical-session
  .target` starts nothing on the bare Wayland sessions Pylon is developed on,
  where that target is never reached, so the compositor starts the unit instead
  — which also guarantees `WAYLAND_DISPLAY` is exported before the daemon tries
  to bind a hotkey. [packaging/user/README.md](packaging/user/README.md) has the
  autostart lines.
- **`make install-user ARGS=--link`** symlinks the binaries out of the checkout
  instead of copying them, so `make build` alone updates what the menu entry and
  the service launch. When the checkout is on a different mount than `$HOME` it
  also writes a `RequiresMountsFor` drop-in, because otherwise the unit can
  start before that disk is mounted and fail for no visible reason.

### Fixed

- **The briefing banner gets out of the way on its own.** It sat on screen for
  30 seconds, long enough to read as "stuck until you hit the ×". It now
  dismisses itself after 5; `PYLON_BANNER_SECONDS` in the `banner_cmd`
  environment picks another value (0 keeps the old click-to-close behaviour).

- **Service action arguments now reach the service.** The LLM schema hard-coded
  four argument fields — `process`, `content`, `reply`, `datetime` — so an
  action declaring anything else had nowhere to put its value. The model chose
  the right action and the service was handed an empty string: "12 çarpı 7 kaç
  eder" reached `calc.eval` with no expression, "dolar kaç lira" reached
  `exchange.currency` and asked back which currency. It affected every argument
  a service contributes: `expr`, `container`, `lines`, `base`, `quote`, `coin`,
  `vs`, `query`, `app`. The schema and the prompt are now built from the live
  action catalog, for all three providers, so registering a service is enough.

- **A missed briefing is now delivered once, instead of never.** The scheduler
  only looks forward: starting the daemon at 09:00 with the briefing set to
  08:00 scheduled it for *tomorrow* and dropped today's entirely. For anyone who
  turns the machine on in the morning that meant the briefing almost never ran.
  The day of the last delivery is persisted, so the catch-up fires once and a
  restart does not repeat it.
- **The push-to-talk shortcut is released when the daemon stops.** It was bound
  at startup and never unbound, so after a stop the key still launched
  `pylon listen`, which recorded audio and then failed to reach the daemon —
  inside a process the compositor started, where the error went nowhere. The
  binding now lives exactly as long as the daemon does.

## [0.1.0-alpha.1] — 2026-08-07

The first published release. An earlier tag of this name existed but was never
published: its draft carried assets built from a different tag, so nothing could
have installed or verified it. It has been removed and the name reused.

Alpha means alpha. It is used daily on Linux, which is where it is developed;
macOS and Windows run the daemon and most services but not all of them, and the
[platform table](README.md#platform-support) is the honest account of which.

### Added

- **Work sessions.** The `sessions` table and the `work:` config block had both
  existed for a while with nothing writing to them. Tracked apps are now
  recorded as they open and close, and `pylon work` / `pylon work week` report
  where the time went — by voice too, since they are service actions. Sessions
  are clipped to the window they are queried for, an open one counts up to now,
  and a heartbeat bounds what an unclean shutdown can lose.
- **Accounts and keys from the GUI.** Spotify can be connected, both services
  can be signed out of, and the API-key card covers Gemini, GitHub and FreshRSS
  with deletion. The daemon's `auth` command is service-agnostic
  (`auth <service> <status|login|logout>`), and `pylon auth <service> logout`
  exists on the CLI.
- **Silence-stop capture and a warm STT server.** A spoken turn ends when you
  stop talking instead of waiting out `record_seconds`, and whisper.cpp's
  server keeps the model resident. Measured end to end: ~5.9 s → ~0.9 s.
- **Push-to-talk from the GUI and a shortcut settings card.** The shortcut is
  registered over the compositor's control socket (Hyprland, Sway), so nothing
  on disk is edited and it re-applies on every daemon start.
- **`make install-user`** installs both binaries, the icon, a desktop entry and
  — only when you have none — the example config, all under `~/.local`. Run it
  with `ARGS=--dry-run` first. `make gui` builds the GUI with the Wails build
  tags it actually needs.

### Fixed

- **`~/.config/pylon/pylon.yaml` is found.** The search stopped at the working
  directory, so a Pylon launched from a desktop entry or an application menu
  silently ran on defaults — no voice, no services, no briefing, and nothing on
  screen saying why.
- **OAuth token refreshes are persisted.** Both Google and Spotify refreshed
  the access token in memory only, so every daemon restart began with an
  expired one, and a provider that rotates refresh tokens would have signed the
  user out silently.
- **`selfupdate.Newer` understands prereleases.** The suffix was discarded, so
  every `v1.0.0-alpha.N` compared equal to `v1.0.0` and to each other: an alpha
  could never see a later alpha, or the release it led to, as an upgrade.
- **Two tests assumed Linux**, failing every macOS and Windows CI run:
  `configPath`'s fallback expected `$XDG_CONFIG_HOME`, and the GUI's fake
  daemon built a Unix socket path past macOS's ~104-byte limit.
- **The widget dialog is a dialog.** It announced itself as nothing, never took
  focus, never gave it back, and let Tab walk out into the page behind it.
- Config keys the code does not read are gone from the shipped `pylon.yaml`,
  and a test now decodes it strictly so the two cannot drift apart again.

### Everything else in this release

The groundwork, listed once because there is no earlier release to compare it
against:

- Go daemon with Unix-socket JSON-lines IPC; CLI and Wails/Svelte GUI as
  clients. The daemon holds all state and stays CGo-free, which is what lets it
  cross-compile.
- Intent engine: a local router for common phrasings, falling through to an LLM
  chain (Gemini / OpenAI / Anthropic) that moves on when a model hits its quota.
- Services: calendar and Drive (Google), GitHub, FreshRSS, Spotify, Docker,
  calculator, exchange rates, weather, machine vitals, system control.
- Voice: whisper.cpp for speech-to-text, any command-line synthesiser for
  speech, with per-OS recording and playback defaults.
- Daily briefing as a desktop banner, process watching with reminders held
  until an app exits, SQLite memory, and a persona that learns writing style.
- Encrypted credential vault (AES-256-GCM) referenced from config as
  `secret:<name>`.
- Signed self-update (`pylon update`), an AUR `pylon-bin` PKGBUILD, and CI
  running the daemon and GUI on Linux, macOS and Windows.

### Known limitations

- **Google and Spotify are unavailable in this build.** Both sign-in flows are
  implemented and work, but a release build needs the project's OAuth client
  baked in and no client is configured yet, so the Accounts screen reports them
  as not yet active. Self-hosters can set `services.google.client_id` /
  `services.spotify.client_id` in `pylon.yaml` and sign in normally.
- **`pylon update` will not offer this release.** GitHub's "latest release"
  endpoint skips prereleases, so an alpha never reaches the updater. Update by
  downloading until the first stable tag.
- Docker on Windows needs named-pipe support, and screen lock, volume and media
  keys are Linux-only. See the
  [platform table](README.md#platform-support).

[0.1.0-alpha.1]: https://github.com/YCistak/pylon/releases/tag/v0.1.0-alpha.1
