#!/usr/bin/env bash
# Pylon TTS via Microsoft Edge neural voices (edge-tts) — free, no API key, no
# quota, natural, multilingual, low latency. Reads the text to speak on stdin
# and writes a WAV to $1. Voice = $2, else PYLON_EDGE_VOICE, else Turkish Emel.
#
# Voices (edge-tts --list-voices for the full set):
#   Turkish: tr-TR-EmelNeural (F), tr-TR-AhmetNeural (M)
#   English: en-US-ChristopherNeural (M), en-GB-RyanNeural (M, British),
#            en-US-AriaNeural (F)
#   Multilingual (one voice, many languages incl. Turkish & English):
#            en-US-AndrewMultilingualNeural, en-US-BrianMultilingualNeural,
#            en-US-AvaMultilingualNeural (F), en-US-EmmaMultilingualNeural (F)
set -euo pipefail

out="${1:?usage: edge_tts.sh <out.wav> [voice]}"
voice="${2:-${PYLON_EDGE_VOICE:-tr-TR-EmelNeural}}"

text="$(cat)"
[ -z "${text//[[:space:]]/}" ] && exit 0

venv="$HOME/.local/share/pylon/edge-venv"
mp3="$(mktemp --suffix=.mp3)"
trap 'rm -f "$mp3"' EXIT

if [ -x "$venv/bin/edge-tts" ]; then
  "$venv/bin/edge-tts" --voice "$voice" --text "$text" --write-media "$mp3" >/dev/null 2>&1
else
  uvx edge-tts --voice "$voice" --text "$text" --write-media "$mp3" >/dev/null 2>&1
fi

# Edge returns 24 kHz mono mp3 → WAV for the player.
ffmpeg -y -i "$mp3" -ar 24000 -ac 1 "$out" >/dev/null 2>&1
