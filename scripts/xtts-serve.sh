#!/usr/bin/env bash
# Launch Pylon's local XTTS v2 TTS server. Loads the model once — keep it
# running (a terminal, a tmux pane, or a user systemd service). Pylon's tts_cmd
# posts text to it. Voice is set via env below.
set -euo pipefail

VENV="${PYLON_XTTS_VENV:-$HOME/.local/share/pylon/xtts-venv}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ ! -x "$VENV/bin/python" ]]; then
  echo "XTTS venv yok: $VENV — kurulum yapılmamış." >&2
  exit 1
fi

export COQUI_TOS_AGREED=1
export XTTS_LANG="${XTTS_LANG:-tr}"
export XTTS_PORT="${XTTS_PORT:-5067}"
# Deep/JARVIS-style default. Swap for any name printed on boot (e.g. "Royston Min",
# "Viktor Eka", "Craig Gutsy") or clone a 6-10s reference clip (overrides SPEAKER):
#   export XTTS_SPEAKER_WAV="$HOME/.local/share/pylon/voices/jarvis_ref.wav"
export XTTS_SPEAKER="${XTTS_SPEAKER:-Damien Black}"

exec "$VENV/bin/python" "$HERE/xtts_server.py"
