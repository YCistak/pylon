package voice

import (
	"context"
	"strings"
	"testing"
)

// call records one fake subprocess invocation.
type call struct {
	name  string
	args  []string
	stdin string
}

// recorder of calls returning scripted stdout.
type fakeRun struct {
	calls []call
	out   map[string][]byte // keyed by binary name
	err   error
}

func (f *fakeRun) run(_ context.Context, stdin []byte, name string, args []string) ([]byte, error) {
	f.calls = append(f.calls, call{name: name, args: args, stdin: string(stdin)})
	if f.err != nil {
		return nil, f.err
	}
	return f.out[name], nil
}

func TestSubstituteArgs(t *testing.T) {
	got := substituteArgs([]string{"-d", "{seconds}", "{file}"}, "/tmp/a.wav", 7)
	want := []string{"-d", "7", "/tmp/a.wav"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v", got)
	}
}

func TestWhisperArgs(t *testing.T) {
	w := whisperArgs("m.bin", "auto", "a.wav")
	js := strings.Join(w, " ")
	for _, want := range []string{"-m m.bin", "-l auto", "-nt", "-f a.wav"} {
		if !strings.Contains(js, want) {
			t.Fatalf("whisper args missing %q: %v", want, w)
		}
	}
}

func TestParseWhisperOutput(t *testing.T) {
	in := []byte("  \n  ekranı kilitle  \n\n lütfen \n")
	if got := parseWhisperOutput(in); got != "ekranı kilitle lütfen" {
		t.Fatalf("parsed = %q", got)
	}
}

func TestCommandUsesSeconds(t *testing.T) {
	if !commandUsesSeconds([]string{"arecord", "-d", "{seconds}", "{file}"}) {
		t.Fatal("should detect {seconds}")
	}
	if commandUsesSeconds([]string{"pw-record", "{file}"}) {
		t.Fatal("should not detect {seconds}")
	}
}

func TestTranscribe(t *testing.T) {
	fr := &fakeRun{out: map[string][]byte{"whisper-cli": []byte("sesi kıs")}}
	w := &whisperTranscriber{bin: "whisper-cli", model: "m.bin", lang: "tr", run: fr.run}

	got, err := w.Transcribe(context.Background(), "rec.wav")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got != "sesi kıs" {
		t.Fatalf("text = %q", got)
	}
	if len(fr.calls) != 1 || fr.calls[0].name != "whisper-cli" {
		t.Fatalf("unexpected calls: %+v", fr.calls)
	}
	if !strings.Contains(strings.Join(fr.calls[0].args, " "), "rec.wav") {
		t.Fatalf("wav not passed: %v", fr.calls[0].args)
	}
}

func TestTranscribeRequiresModel(t *testing.T) {
	w := &whisperTranscriber{bin: "whisper-cli", lang: "auto", run: (&fakeRun{}).run}
	if _, err := w.Transcribe(context.Background(), "x.wav"); err == nil {
		t.Fatal("expected error without model")
	}
}

func TestSpeakSynthesizesAndPlays(t *testing.T) {
	fr := &fakeRun{}
	sp := &cmdSpeaker{
		ttsCmd:  []string{"piper", "-m", "v.onnx", "-f", "{file}"},
		playCmd: []string{"pw-play", "{file}"},
		run:     fr.run,
		tmpWav:  func() (string, func(), error) { return "/tmp/out.wav", func() {}, nil },
	}
	if err := sp.Say(context.Background(), "merhaba"); err != nil {
		t.Fatalf("say: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected synth + play, got %d calls", len(fr.calls))
	}
	// Synth: text on stdin, {file} substituted in args.
	if fr.calls[0].name != "piper" || fr.calls[0].stdin != "merhaba" {
		t.Fatalf("synth call wrong: %+v", fr.calls[0])
	}
	if strings.Join(fr.calls[0].args, " ") != "-m v.onnx -f /tmp/out.wav" {
		t.Fatalf("synth args wrong: %v", fr.calls[0].args)
	}
	if fr.calls[1].name != "pw-play" || fr.calls[1].args[0] != "/tmp/out.wav" {
		t.Fatalf("play call wrong: %+v", fr.calls[1])
	}
}

func TestSpeakEmptyIsNoop(t *testing.T) {
	fr := &fakeRun{}
	sp := &cmdSpeaker{ttsCmd: []string{"piper"}, run: fr.run, tmpWav: tempWav}
	if err := sp.Say(context.Background(), "   "); err != nil {
		t.Fatalf("empty say: %v", err)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("empty text should not invoke tts")
	}
}

func TestSpeakUnconfiguredErrors(t *testing.T) {
	sp := &cmdSpeaker{run: (&fakeRun{}).run, tmpWav: tempWav}
	if err := sp.Say(context.Background(), "merhaba"); err == nil {
		t.Fatal("empty tts_cmd should error")
	}
}

// --- pipeline-level with fakes ---

type fakeRec struct{ err error }

func (f fakeRec) Record(context.Context, string) error { return f.err }

type fakeSTT struct {
	text string
	err  error
}

func (f fakeSTT) Transcribe(context.Context, string) (string, error) { return f.text, f.err }

type fakeTTS struct{ spoken string }

func (f *fakeTTS) Say(_ context.Context, text string) error { f.spoken = text; return nil }

func TestPipelineCaptureAndSpeak(t *testing.T) {
	tts := &fakeTTS{}
	p := &Pipeline{
		rec:    fakeRec{},
		stt:    fakeSTT{text: "ekranı kilitle"},
		tts:    tts,
		tmpWav: func() (string, func(), error) { return "/tmp/r.wav", func() {}, nil },
	}
	got, err := p.Capture(context.Background())
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if got != "ekranı kilitle" {
		t.Fatalf("captured = %q", got)
	}
	if err := p.Speak(context.Background(), "tamam"); err != nil {
		t.Fatalf("speak: %v", err)
	}
	if tts.spoken != "tamam" {
		t.Fatalf("tts got %q", tts.spoken)
	}
}
