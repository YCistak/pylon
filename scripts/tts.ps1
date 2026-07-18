# Pylon TTS on Windows via the built-in SAPI voices (System.Speech) — no install,
# no API key. Reads the text to speak on stdin and writes a PCM WAV to the first
# argument, matching the tts_cmd contract (text on stdin, WAV at {file}). The
# WAV is PCM, which the default Windows play_cmd (Media.SoundPlayer) requires.
#
# Point tts_cmd at it in pylon.yaml (use Windows PowerShell 5.1, which ships
# System.Speech; pwsh 7 does not):
#   tts_cmd: ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
#             "-File", "C:\\path\\to\\tts.ps1", "{file}"]
#
# Voice: pass a name as the second argument or set PYLON_SAPI_VOICE; otherwise
# the system default is used. List installed voices with:
#   Add-Type -AssemblyName System.Speech
#   (New-Object System.Speech.Synthesis.SpeechSynthesizer).GetInstalledVoices() `
#     | ForEach-Object { $_.VoiceInfo.Name }
param(
    [Parameter(Mandatory = $true)][string]$Out,
    [string]$Voice = $env:PYLON_SAPI_VOICE
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Speech

# Read the whole of stdin; an empty utterance is a no-op, mirroring edge_tts.sh.
$text = [Console]::In.ReadToEnd()
if ([string]::IsNullOrWhiteSpace($text)) { exit 0 }

$synth = New-Object System.Speech.Synthesis.SpeechSynthesizer
try {
    if ($Voice) { $synth.SelectVoice($Voice) }
    $synth.SetOutputToWaveFile($Out)
    $synth.Speak($text)
}
finally {
    $synth.Dispose()
}
