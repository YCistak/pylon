# Pylon

> A personal AI assistant daemon — voice-first, context-aware, runs on your machine.

[![CI](https://github.com/YCistak/pylon/actions/workflows/ci.yml/badge.svg)](https://github.com/YCistak/pylon/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/YCistak/pylon?sort=semver)](https://github.com/YCistak/pylon/releases)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](#license)
![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)

Pylon watches what you do — when work ends, when a game closes, when the day
starts — and acts on it. You talk to it; it carries the context. Everything runs
locally: a Go daemon owns the state, and the GUI is just one of its clients.

**Status: early.** Usable daily on Linux, which is where it is developed. macOS
and Windows run the daemon and most services, but not everything — see
[Platform support](#platform-support) before installing.

---

## What it does

Ask in plain language (Turkish or English) and Pylon routes it. A local router
resolves the common phrasings for free; anything it cannot place falls through
to an LLM chain (Gemini / OpenAI / Anthropic), which tries each model in order
and moves on when one hits its quota.

| Service | Actions |
| --- | --- |
| **calc** | arithmetic, spoken back |
| **exchange** | live currency and crypto rates |
| **weather** | current conditions, today's high/low, rain chance (Open-Meteo) |
| **github** | open PRs and issues, PR polling, a daily commit nudge |
| **freshrss** | unread count from your FreshRSS instance |
| **docker** | list, inspect, start/stop/restart containers |
| **sysmon** | CPU load, RAM, free disk, temperature, uptime (Linux) |
| **calendar / drive** | today's events, recent files (Google) |
| **spotify** | playback control, now playing |
| **system** | lock the screen, volume, media keys, close an app |

Services register only when configured, so a missing credential removes the
feature rather than breaking the daemon.

Beyond commands, Pylon watches processes: it knows when you closed your editor
or quit a game, and can hold reminders until then.

---

## Platform support

The daemon is pure Go and cross-compiles cleanly. What differs is everything
that touches the OS. Tested on real runners for each platform:

| | Linux | macOS | Windows |
| --- | :---: | :---: | :---: |
| Daemon, intent engine, memory | ✅ | ✅ | ✅ |
| GUI ↔ daemon | ✅ | ✅ | ✅ |
| calc, exchange, github, freshrss | ✅ | ✅ | ✅ |
| Google, Spotify | ✅ | ✅ | ✅ |
| Docker | ✅ | ✅ | ❌ needs npipe |
| Voice (STT/TTS) | ✅ | ✅ | ✅ (`scripts/tts.ps1`) |
| Screen lock, volume, media keys | ✅ | ❌ | ❌ |
| Process watching | ✅ | ✅ | ✅ |

Process watching uses `/proc` on Linux, `ps` on macOS and `tasklist` on
Windows. Machine control still shells out to `loginctl`/`pactl`/`playerctl`, so
it is Linux-only for now; it degrades with a message rather than crashing — the
rest of Pylon works.

---

## Install

Download the archive for your platform from
[Releases](https://github.com/YCistak/pylon/releases), unpack it, and run the
daemon:

```sh
./pylon start      # foreground; Ctrl-C stops it
./pylon status
```

Each archive holds the daemon (`pylon`), the GUI (`pylon-ui`), and a commented
`pylon.yaml` to copy from. The GUI starts the daemon itself if one is not
already running, so `pylon-ui` alone is enough for day-to-day use.

### Build from source

Needs Go 1.26+, and for the GUI [Wails](https://wails.io) v2 plus Node 20:

```sh
go build -o pylon ./cmd/pylon

cd pylon-ui && wails build -tags webkit2_41   # the tag is Linux-only
```

On Linux the GUI needs `libwebkit2gtk-4.1-dev` and `libgtk-3-dev`.

---

## Configure

Pylon reads `pylon.yaml` from the working directory, or `$PYLON_CONFIG`. Every
field has a default, so start by changing only what you need.

Credentials never go in the file. Save them to the encrypted vault and refer to
them by name:

```sh
./pylon secret set github        # prompts, then stores encrypted
```

```yaml
services:
  github:
    token: secret:github         # resolved at runtime
  freshrss:
    url: "http://localhost:8080"
    username: "you"
    api_password: secret:freshrss
```

The vault is AES-256-GCM at rest under your user config dir. `${ENV_VAR}` is
also expanded if you prefer the environment. Google and Spotify use OAuth:

```sh
./pylon auth google
```

### Voice

Speech is optional and engine-agnostic — Pylon shells out, so any tool that
reads text and writes a WAV works.

```yaml
voice:
  stt_bin: whisper-cli                 # whisper.cpp
  stt_model: /path/ggml-large-v3.bin
  tts_cmd: ["/path/edge_tts.sh", "{file}", "tr-TR-EmelNeural"]
```

Two ready examples ship in `scripts/`: `edge_tts.sh` wraps
[edge-tts](https://github.com/rany2/edge-tts) on Unix, and `tts.ps1` uses
Windows' built-in SAPI voices (no install). Recording and playback default
per-OS (`pw-record`/`pw-play` on Linux, `sox`/`afplay` on macOS,
`ffmpeg`/PowerShell on Windows) and are overridable.

Two more settings decide how fast a spoken turn feels.

`silence_stop` (on by default) ends the capture the moment you stop talking, so a
two-second question takes two seconds instead of waiting out `record_seconds` —
which is now only a ceiling. It needs `sox` on Linux and Windows (macOS already
records through sox); without it Pylon logs a line and keeps the fixed window.

`stt_server` points at whisper.cpp's server binary. The daemon starts it, keeps
the model resident, and stops it on shutdown — taking the model load (~0.6 s with
large-v3-turbo) out of every turn:

```yaml
voice:
  stt_server:
    bin: /path/whisper.cpp/build/bin/whisper-server
    port: 8910
```

If the server is unreachable, transcription falls back to `stt_bin`, so voice
keeps working — just slower.

Then bind `pylon listen` to a hotkey in your desktop environment for
push-to-talk.

---

## How it fits together

```
  GUI (Wails)  ─┐
  CLI          ─┼─→  Unix socket  ─→  daemon  ─→  services
  hotkey       ─┘    (JSON lines)       │
                                        ├─ intent: local router → LLM chain
                                        ├─ memory: SQLite
                                        └─ watcher: process lifecycle
```

The daemon owns everything. The GUI is a separate module and a pure IPC client —
that keeps the daemon free of CGo, which is what lets it cross-compile at all.

Socket and PID default to `/tmp` on Unix and `%LocalAppData%\pylon` on Windows;
`PYLON_SOCKET` / `PYLON_PID` override both sides.

---

## License

AGPL-3.0.
