#!/usr/bin/env python3
"""Pylon local TTS server — Coqui XTTS v2 on GPU.

Loads the XTTS v2 model once and serves POST /tts (text in the request body →
WAV in the response). Run it persistently; Pylon's `tts_cmd` posts to it with a
small curl client, so synthesis is local (no quota, no network) and fast
(~0.5-1s on an RTX 4060 with the model already resident).

Runs in the coqui-tts venv (see the project notes). Env:
  XTTS_LANG         language code (default tr)
  XTTS_SPEAKER      built-in speaker name (see the list printed on boot)
  XTTS_SPEAKER_WAV  reference clip (6-10s) to CLONE a voice (overrides SPEAKER)
  XTTS_PORT         listen port (default 5067)
"""
import os
import tempfile
from http.server import BaseHTTPRequestHandler, HTTPServer

import torch
from TTS.api import TTS

LANG = os.environ.get("XTTS_LANG", "tr")
SPEAKER = os.environ.get("XTTS_SPEAKER", "")
SPEAKER_WAV = os.environ.get("XTTS_SPEAKER_WAV", "")
PORT = int(os.environ.get("XTTS_PORT", "5067"))
MODEL = "tts_models/multilingual/multi-dataset/xtts_v2"

device = "cuda" if torch.cuda.is_available() else "cpu"
print(f"[xtts] model yükleniyor ({device})...", flush=True)
tts = TTS(MODEL).to(device)

speakers = []
try:
    speakers = list(tts.synthesizer.tts_model.speaker_manager.speakers.keys())
except Exception:
    pass
print(f"[xtts] {len(speakers)} hazır ses:", ", ".join(speakers[:25]), flush=True)
print("[xtts] Sesi seçmek için XTTS_SPEAKER='<isim>', klonlamak için "
      "XTTS_SPEAKER_WAV=<referans.wav>.", flush=True)


def speaker_kwargs():
    if SPEAKER_WAV and os.path.exists(SPEAKER_WAV):
        return {"speaker_wav": SPEAKER_WAV}
    if SPEAKER and SPEAKER in speakers:
        return {"speaker": SPEAKER}
    if speakers:
        return {"speaker": speakers[0]}
    return {}


# Stability params: lower temperature + repetition penalty tame XTTS's wobble
# (artifacts, random speed-ups), and text splitting keeps pacing steady across
# sentences. Tunable via env if needed.
GEN = dict(
    temperature=float(os.environ.get("XTTS_TEMPERATURE", "0.70")),
    # rep penalty 2.0 (XTTS default): higher values (5+) make it *swallow*
    # syllables. length penalty slightly >1 discourages clipped word endings.
    repetition_penalty=float(os.environ.get("XTTS_REP_PENALTY", "2.0")),
    length_penalty=float(os.environ.get("XTTS_LENGTH_PENALTY", "1.0")),
    top_k=int(os.environ.get("XTTS_TOP_K", "50")),
    top_p=float(os.environ.get("XTTS_TOP_P", "0.85")),
    speed=float(os.environ.get("XTTS_SPEED", "1.0")),
    enable_text_splitting=True,
)


def synth(text: str, path: str):
    tts.tts_to_file(text=text, language=LANG, file_path=path, **speaker_kwargs(), **GEN)


# Warm the pipeline so the first real request is fast.
_tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False).name
try:
    synth("Sistemler hazır.", _tmp)
    os.remove(_tmp)
except Exception as e:  # noqa: BLE001
    print("[xtts] ısınma hatası:", e, flush=True)
print(f"[xtts] HAZIR — http://127.0.0.1:{PORT}/tts", flush=True)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):  # quiet
        pass

    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        text = self.rfile.read(n).decode("utf-8", "ignore").strip()
        if not text:
            self.send_response(400)
            self.end_headers()
            return
        try:
            path = tempfile.NamedTemporaryFile(suffix=".wav", delete=False).name
            synth(text, path)
            data = open(path, "rb").read()
            os.remove(path)
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except Exception as e:  # noqa: BLE001
            self.send_response(500)
            self.end_headers()
            self.wfile.write(str(e).encode())


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
