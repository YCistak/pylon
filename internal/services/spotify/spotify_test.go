package spotify

import (
	"context"
	"strings"
	"testing"

	"github.com/YCistak/pylon/internal/intent"
)

type fakeSp struct {
	pb        Playback
	calls     []string
	volSet    int
	playedQ   string
	playLabel string
	err       error
}

func (f *fakeSp) resume(context.Context) error                 { f.calls = append(f.calls, "resume"); return f.err }
func (f *fakeSp) pause(context.Context) error                  { f.calls = append(f.calls, "pause"); return f.err }
func (f *fakeSp) next(context.Context) error                   { f.calls = append(f.calls, "next"); return f.err }
func (f *fakeSp) previous(context.Context) error               { f.calls = append(f.calls, "previous"); return f.err }
func (f *fakeSp) nowPlaying(context.Context) (Playback, error) { return f.pb, f.err }
func (f *fakeSp) setVolume(_ context.Context, p int) error {
	f.volSet = p
	f.calls = append(f.calls, "setVolume")
	return f.err
}
func (f *fakeSp) playQuery(_ context.Context, q string) (string, error) {
	f.playedQ = q
	return f.playLabel, f.err
}

func runAction(t *testing.T, fake spAPI, action intent.Action, args map[string]string) string {
	t.Helper()
	out, err := (&Spotify{api: fake}).Execute(context.Background(), action, args)
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", action, err)
	}
	return out
}

func TestTransportControls(t *testing.T) {
	cases := []struct {
		action intent.Action
		call   string
	}{
		{ActionPlay, "resume"},
		{ActionPause, "pause"},
		{ActionNext, "next"},
		{ActionPrevious, "previous"},
	}
	for _, c := range cases {
		f := &fakeSp{}
		runAction(t, f, c.action, nil)
		if len(f.calls) != 1 || f.calls[0] != c.call {
			t.Errorf("%s → calls %v, want [%s]", c.action, f.calls, c.call)
		}
	}
}

func TestVolumeUpClamps(t *testing.T) {
	f := &fakeSp{pb: Playback{HasDevice: true, Volume: 95}}
	out := runAction(t, f, ActionVolumeUp, nil)
	if f.volSet != 100 {
		t.Errorf("volume set to %d, want 100 (clamped)", f.volSet)
	}
	if !strings.Contains(out, "100") {
		t.Errorf("reply %q missing new volume", out)
	}
}

func TestVolumeDownClamps(t *testing.T) {
	f := &fakeSp{pb: Playback{HasDevice: true, Volume: 5}}
	runAction(t, f, ActionVolumeDown, nil)
	if f.volSet != 0 {
		t.Errorf("volume set to %d, want 0 (clamped)", f.volSet)
	}
}

func TestVolumeNoDevice(t *testing.T) {
	f := &fakeSp{pb: Playback{HasDevice: false}}
	if _, err := (&Spotify{api: f}).Execute(context.Background(), ActionVolumeUp, nil); err == nil {
		t.Fatal("expected error when no active device")
	}
	if len(f.calls) != 0 {
		t.Errorf("should not set volume without a device, calls: %v", f.calls)
	}
}

func TestPlayTrack(t *testing.T) {
	f := &fakeSp{playLabel: "Bohemian Rhapsody — Queen"}
	out := runAction(t, f, ActionPlayTrack, map[string]string{"query": "bohemian"})
	if f.playedQ != "bohemian" {
		t.Errorf("query not forwarded: %q", f.playedQ)
	}
	if !strings.Contains(out, "Bohemian Rhapsody — Queen") {
		t.Errorf("reply %q missing track label", out)
	}
}

func TestPlayTrackNoQuery(t *testing.T) {
	if _, err := (&Spotify{api: &fakeSp{}}).Execute(context.Background(), ActionPlayTrack, map[string]string{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestPlayTrackNoMatch(t *testing.T) {
	f := &fakeSp{playLabel: ""}
	out := runAction(t, f, ActionPlayTrack, map[string]string{"query": "zzzz"})
	if !strings.Contains(out, "found nothing matching") {
		t.Errorf("expected no-match reply, got %q", out)
	}
}

func TestNowPlaying(t *testing.T) {
	playing := runAction(t, &fakeSp{pb: Playback{HasDevice: true, IsPlaying: true, Track: "Song — Artist"}}, ActionNowPlaying, nil)
	if !strings.Contains(playing, "Song — Artist") {
		t.Errorf("now_playing reply %q missing track", playing)
	}
	idle := runAction(t, &fakeSp{pb: Playback{HasDevice: false}}, ActionNowPlaying, nil)
	if !strings.Contains(idle, "Nothing is playing") {
		t.Errorf("idle reply %q unexpected", idle)
	}
}
