# Changelog

Notable changes per release. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semver](https://semver.org/), and `internal/selfupdate` compares them by
semver precedence — including prerelease ordering.

Cutting a release: [docs/RELEASE.md](docs/RELEASE.md).

## [Unreleased]

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

## [0.1.0-alpha.1] — 2026-07-18

First tagged build. Not published — see
[docs/RELEASE.md](docs/RELEASE.md#before-you-tag) for what is still in the way.

### Added

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

[Unreleased]: https://github.com/YCistak/pylon/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/YCistak/pylon/releases/tag/v0.1.0-alpha.1
