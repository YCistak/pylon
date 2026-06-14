#!/usr/bin/env python3
"""Pylon TTS via Gemini's native prebuilt voices (Charon, Puck, Kore, ...).

Reads the text to speak on stdin and writes a WAV file to argv[1]. Used as
Pylon's `tts_cmd`, so it stays engine-agnostic. Standard library only — no pip,
no venv. The API key comes from PYLON_TTS_KEY or GEMINI_API_KEY in the env.

Note: Gemini's batch TTS endpoint generates the whole clip before returning, so
there's an inherent ~2-3s latency before playback and the free-tier quota is
tight (429s under frequent use). For low-latency, real-time Charon voice, the
Gemini Live API (native audio) is the path — see PLANNED's `pylon live` backlog.

Env:
  GEMINI_API_KEY / PYLON_TTS_KEY   API key (required)
  PYLON_TTS_VOICE                  voice name (default Charon — deep, JARVIS-like)
  PYLON_TTS_MODEL                  TTS model (default gemini-2.5-flash-preview-tts)
"""
import sys
import os
import json
import base64
import wave
import urllib.request

API = "https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent"


def main() -> int:
    out = sys.argv[1] if len(sys.argv) > 1 else "out.wav"
    text = sys.stdin.read().strip()
    if not text:
        return 0

    key = os.environ.get("PYLON_TTS_KEY") or os.environ.get("GEMINI_API_KEY", "")
    if not key:
        sys.stderr.write("gemini_tts: GEMINI_API_KEY (veya PYLON_TTS_KEY) gerekli\n")
        return 1
    voice = os.environ.get("PYLON_TTS_VOICE", "Charon")
    model = os.environ.get("PYLON_TTS_MODEL", "gemini-2.5-flash-preview-tts")

    body = json.dumps({
        "contents": [{"parts": [{"text": text}]}],
        "generationConfig": {
            "responseModalities": ["AUDIO"],
            "speechConfig": {"voiceConfig": {"prebuiltVoiceConfig": {"voiceName": voice}}},
        },
    }).encode("utf-8")

    req = urllib.request.Request(
        API.format(model=model), data=body,
        headers={"Content-Type": "application/json", "x-goog-api-key": key},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.load(resp)
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"gemini_tts: API {e.code}: {e.read().decode('utf-8', 'ignore')[:300]}\n")
        return 1

    part = data["candidates"][0]["content"]["parts"][0]["inlineData"]
    pcm = base64.b64decode(part["data"])

    rate = 24000
    for tok in part.get("mimeType", "").split(";"):
        tok = tok.strip()
        if tok.startswith("rate="):
            try:
                rate = int(tok[5:])
            except ValueError:
                pass

    with wave.open(out, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(rate)
        w.writeframes(pcm)
    return 0


if __name__ == "__main__":
    sys.exit(main())
