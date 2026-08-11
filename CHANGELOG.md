# Changelog

Notable changes per release. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semver](https://semver.org/), and `internal/selfupdate` compares them by
semver precedence — including prerelease ordering.

Cutting a release: [docs/RELEASE.md](docs/RELEASE.md).

## [Unreleased]

### Added

- **"What's playing?" without an account.** Asking Pylon what is playing needed
  a Spotify app, an OAuth client id in `pylon.yaml`, a Premium subscription and
  a browser round-trip — for a question the machine can answer about itself. It
  now reads MPRIS, the D-Bus interface every Linux player publishes, so it works
  for Spotify, a browser tab, VLC or mpv with nothing configured at all. The
  local router resolves the phrasings, so it costs no model call either, and the
  new **Çalan** widget puts it on the home screen. Spotify's own `now_playing`
  stays: it is the one that can answer for a phone in another room, and the only
  one that can play a track by name.

- **A language picker in Settings** (Genel), and `pylon lang <code>` on the
  command line. Until now the seven languages could only be reached by editing
  `pylon.yaml`, which is invisible to anyone who has just installed Pylon. The
  choice belongs to the daemon, not to the GUI — the interface asks it to switch
  and follows, so the buttons around an answer are never in a different language
  from the answer, and the CLI agrees with both. It applies immediately: nothing
  restarts, and the next reply is already in the new language. **Otomatik**
  forgets the choice and says which of `pylon.yaml` or the desktop locale it
  fell back to, rather than calling both of them "the system". The choice is
  remembered in a one-line `language` file beside the config it overrides —
  rewriting the YAML would strip the comments that make it readable, and the
  database is only ever opened by the daemon, so every CLI process would have
  disagreed with the window.

- **Pylon speaks seven languages** (`language:` in `pylon.yaml`; empty follows
  the desktop locale). English, German, Spanish, French, Portuguese, Russian
  and Turkish, covering every reply, the CLI, and the GUI — including weekday
  and month names, which Go's `time` package only knows in English. Plural
  rules are per language: Turkish marks none after a number, English needs two
  forms, Russian four, and money follows the fractional rule of each language
  ("0.87 dollars" but "0,87 euro"). A missing translation falls back to English
  key by key, so a partly translated language stays usable, and the LLM is told
  which language to reply in rather than left to infer it from the question.
  Translations beyond English and Turkish are unreviewed by native speakers.

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

- **Arithmetic and prices answer in the language they were asked in.** The
  seven-language work translated the sentences that came from a catalog and
  missed the two services that built their own: the calculator answered
  `12 times 7` with "84 eder." in every language, because the wording was a
  `fmt.Sprintf("%s eder.", …)` in the middle of the arithmetic, and nothing
  about arithmetic looks like a place where a language would hide.

  The numbers themselves were wrong in a way that is worse than untranslated
  text. Both services punctuated them as Turkish does — a point between
  thousands, a comma before the decimals — so an English reader was told
  "1 dollar is 47,71 Turkish lira", which is not a foreign-looking spelling of
  the rate but one-hundredth of it, and "2.850.000,50" for a bitcoin price.
  Punctuation now comes from `i18n.Decimal` and `i18n.Money`, one table for all
  seven languages, including the no-break space French and Russian group with.
  The two services no longer format numbers themselves, and the case that used
  to be frozen into their tests as correct is now the case that fails.

  The same mark was hiding in the catalogs themselves, where it was harder to
  see: `"CPU load %.2f"` looks like a translated string but the `%.2f` is Go's
  punctuation, so a fully translated Turkish window still said "CPU yükü 0.18",
  and the machine-vitals line disagreed with itself in six languages out of
  seven. The fractional readings — load average and the RAM figures — and the
  local-match confidence score are formatted at the call site now. Whole-number
  verbs (`%.0f`, for disk and temperature) stay as they are: there is no decimal
  mark in "177 GB" to get wrong. A test walks every catalog and fails on any
  message that punctuates its own decimals, because this is the second time the
  same mistake was invisible in review.

- **The interface is actually translated now.** Adding the language picker
  turned up text the seven-language work had missed, because nothing checked
  for it: the voice bar said "Konuş" inside a Russian window, the API-key hints
  printed their own catalog keys (`ui.keys.gemini_hint`) because the key never
  reached `$t()`, and the settings tab hint did the same. The sidebar, the
  Docker page and widget, and the widget editor were partly hard-coded too.
  `npm run check:i18n` now fails the build on all three causes — a key missing
  from one language, a key that does not exist, and a catalog key rendered
  without `$t()` — and `npm run build` runs it, so CI does.

- **Docker's own address format now works.** `services.docker.host` was
  documented as taking `tcp://…` — the way Docker writes it everywhere, from
  `DOCKER_HOST` to its own `--host` flag — but the value went straight to Go's
  HTTP client, which rejects that scheme outright. Every call failed with
  `unsupported protocol scheme "tcp"`, which reads as Pylon being broken rather
  than the address being in the wrong dialect. It is the same plain HTTP either
  way, so `tcp://` is translated instead of refused, and a bare `host:port`
  gets `http://`. This is the only route to Docker on Windows, where the Engine
  listens on a named pipe Go cannot dial; the README now says so, along with the
  macOS socket path Docker Desktop stopped creating.

- **Arrow keys in Settings moved the wrong tab.** The tablist's `findIndex`
  predicate ignored its own parameter and closed over `tab` from the markup, so
  it always matched the last-rendered tab; both arrows stepped from the same
  place. Focus now follows the selection too, as a roving tabindex requires.

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
