# Changelog

Notable changes per release. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semver](https://semver.org/), and `internal/selfupdate` compares them by
semver precedence — including prerelease ordering.

Cutting a release: [docs/RELEASE.md](docs/RELEASE.md).

## [Unreleased]

### Security

- **"Close X" passed a language model's words to `pkill` as a regular
  expression.** `pkill` matches an extended regex against process names, and
  the name came straight from the model with no validation: `pkill .` matches
  every process whose name contains any character — 479 of 480 on the machine
  this was found on, which is the user's entire session. `-x` alone does not
  fix it, because `.*` still matches every name there is, and the guard against
  Pylon killing itself compared literals, so `pylo.` walked past it and then
  matched `pylon`.

  The name is now validated as a name (letters, digits, dot, dash, underscore,
  at least one alphanumeric, 64 characters), matched with `-x`, and its dots
  are escaped — real names contain them, `mount.ntfs-3g` and `python3.11` are
  on an ordinary desktop, and unescaped a dot is what made `pylo.` work.

- **Three smaller edges from the same review.** The exchange service put the
  model's currency and coin names into URLs unescaped — the host is fixed, so
  this could not reach another server, but a stray `?` or `&` would quietly
  request something else of that one. The GUI's `OpenURL` is bound to the
  frontend and would open any scheme it was handed, including `file://`; it
  takes http and https now. And the PID file is written 0600 rather than 0644,
  since on Unix it sits in `/tmp` beside every other user's files.

- **Four reachable vulnerabilities closed by dependency bumps**, found by
  `govulncheck`: two infinite loops (`golang.org/x/text` on invalid input,
  `golang.org/x/net`'s HTTP/2 transport on a bad `SETTINGS_MAX_FRAME_SIZE`), an
  IDNA validation failure in `x/net`, and an xDS/HTTP2 pair in
  `google.golang.org/grpc`. All three modules are reached from Pylon's own HTTP
  calls — the feedback POST and the Google consent server — so they were
  reachable, not merely present. `govulncheck ./...` now reports zero for both
  modules.

## [0.1.0] — 2026-08-13

The first stable release, and the first one `pylon update` can actually see:
GitHub's "latest" endpoint skips prereleases, so while `v0.1.0-alpha.1` was the
only tag it answered 404 and every update check reported an HTTP error. Nothing
about the alpha was wrong; it was simply invisible to the thing meant to replace
it.

Alpha users update by downloading this one, the same way they installed. From
here `pylon update` takes over — and now updates the interface as well as the
daemon, which no release before this one did.

Scope is unchanged from the alpha: developed and used daily on Linux, where
every service works. macOS and Windows run the daemon and most services; screen
lock, audio and media keys are Linux-only, and Docker needs `npipe` support that
is not written. Google and Spotify still report as unavailable in published
builds — no OAuth client is baked in.

### Added

- **Feedback, from inside the window.** Saying something about Pylon meant
  knowing the project had a tracker, finding it, and having an account — which
  selects for contributors and against exactly the users worth hearing from.
  Hakkında now has a category, a box and a Send button, and what comes out is
  an issue on the project page.

  Pylon has no server of its own, so there are two ways it gets there and the
  reply says which happened: with the GitHub token already in the vault it
  files the issue and hands back its URL, and with no token — or one that
  cannot open issues — it opens the prefilled page in the browser instead. It
  falls back rather than failing, because a token's permissions are not
  something the user can fix from that screen, and it invents no identity to
  post under.

  Nothing is sent that the user has not seen: the diagnostics are one short
  line — version, OS, desktop, language — shown under the box before Send, and
  built once so what is on screen and what is in the issue cannot drift apart.
  No log, no config, nothing read off disk.

- **Updating from the window, and a Hakkında tab to do it in.** `pylon update`
  existed and nothing in the GUI reached it, so the only way to install a
  release was to know there was a terminal command for it. The new tab shows
  what is installed and offers the update in two steps — check, then install —
  because replacing Pylon on disk is not something to do on one click without
  saying what is about to land.

  **The update now replaces the GUI too.** It never could before: the archive
  ships both binaries, and `selfupdate` extracted only `pylon`, so every update
  left the interface behind at the old version with nothing on screen to say
  so. The GUI still cannot replace itself — on Windows a process cannot
  overwrite the binary it is running from — but the daemon can, and does. macOS
  is excluded, where the GUI is a `.app` bundle rather than a file.

  Both versions are shown side by side for the window between the two: the
  daemon is swapped first and this window keeps running the old code until it is
  closed and opened again.

### Fixed

- **"No release published yet" is no longer an HTTP error.** GitHub's "latest"
  endpoint answers 404 when there is nothing to serve, and it skips prereleases
  — so a project whose only tag is an alpha hits it as well. `pylon update`
  passed that through as "check for updates: github returned 404 Not Found",
  which reads as a fault on the user's own machine rather than as the project
  simply not having released yet.

- **The GUI now knows its own version.** The Makefile and the release workflow
  were both passing `-X main.version=` to the GUI build, but `pylon-ui` had no
  such variable, so the linker discarded the value in silence and the binary
  never carried a version at all.

- **A push-to-talk turn can be stopped.** Once the microphone opened there was
  no way out of it: Escape did nothing, the button went disabled and read
  "Dinliyorum…", and the only thing that ended the turn was the silence timer —
  so a mistaken click meant sitting through a recording, a transcription, and
  whatever the model decided to say about the room. Escape now stops it, and the
  mic button becomes **Durdur** while it is open. The stop cannot ride on
  `Listen()`'s own connection, which is blocked for the length of the turn, so
  it is a second request (`listen cancel`) that cancels the turn's context —
  interrupting `sox` rather than killing it, so the WAV still closes cleanly.
  Cancelling a turn that has already ended is silent, because Escape losing that
  race is ordinary and there is nothing the user would do about it. The same
  bookkeeping makes the microphone single-user: a second `listen` while one is
  running is refused instead of opening the device twice.

- **You can see where the keyboard is now.** The interface had five `:focus`
  rules — the settings tablist and a few text fields — and left everything else
  to the engine's default outline, a thin dark line that against these surfaces
  is not visible at all. Tabbing across the sidebar, a widget's pen icon or
  **Konuş** showed nothing whatsoever, so the keyboard path was unusable even
  though it worked. One `:focus-visible` rule in `style.css` now draws a cyan
  ring on anything focused, deliberately as the least specific rule in the
  interface so a control with its own focus treatment still wins.

- **Closing the widget dialog no longer drops you at the top of the page.**
  Handing focus back needs the trigger to still exist, and the pen icon on the
  home screen destroys itself on the way to the dialog: it switches the view to
  Görünüm, which unmounts the page it lives on. The `isConnected` check noticed
  and quietly gave up, so *every* close from the home screen sent a keyboard
  user back to the start of the document — the one case the fallback was written
  for was the rare one (a deleted widget), and the common one went unnoticed.
  The dialog now walks a list: the trigger, then the row for the widget that was
  being edited, then the button that adds one.

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

[0.1.0]: https://github.com/YCistak/pylon/releases/tag/v0.1.0
[0.1.0-alpha.1]: https://github.com/YCistak/pylon/releases/tag/v0.1.0-alpha.1
