// Package spotify integrates Spotify playback control into Pylon via the Spotify
// Web API (OAuth2). Like the other services it speaks through a small interface
// so it can be faked in tests, and contributes its actions to the intent
// vocabulary. Playback control requires Spotify Premium and an active device.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// Spotify actions.
const (
	ActionPlay       intent.Action = "spotify.play"
	ActionPause      intent.Action = "spotify.pause"
	ActionNext       intent.Action = "spotify.next"
	ActionPrevious   intent.Action = "spotify.previous"
	ActionVolumeUp   intent.Action = "spotify.volume_up"
	ActionVolumeDown intent.Action = "spotify.volume_down"
	ActionPlayTrack  intent.Action = "spotify.play_track"
	ActionNowPlaying intent.Action = "spotify.now_playing"
)

// volumeStep is how much volume_up/down move the device volume, in percent.
const volumeStep = 10

const apiBase = "https://api.spotify.com/v1"

// Playback is the minimal now-playing state Pylon needs (decoupled from the API
// for testing).
type Playback struct {
	Track     string // "Song — Artist"; empty when nothing is loaded
	IsPlaying bool
	Volume    int  // device volume percent (0..100)
	HasDevice bool // false when no active Spotify device exists
}

// spAPI is the slice of the Spotify Web API the service uses; a fake implements
// it in tests.
type spAPI interface {
	resume(ctx context.Context) error
	pause(ctx context.Context) error
	next(ctx context.Context) error
	previous(ctx context.Context) error
	nowPlaying(ctx context.Context) (Playback, error)
	setVolume(ctx context.Context, percent int) error
	playQuery(ctx context.Context, query string) (label string, err error) // search + play first hit
}

// Spotify is the Spotify Service.
type Spotify struct {
	cfg Config
	api spAPI // injected in tests; otherwise built lazily from the token
}

// New builds the service from config. It does not touch the network or token
// until first use, so it can be registered before `pylon auth spotify`.
func New(cfg Config) *Spotify { return &Spotify{cfg: cfg} }

func (s *Spotify) Name() string { return "spotify" }

func (s *Spotify) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{Name: ActionPlay, Desc: `"spotify.play": resume Spotify playback. No args. Use for "müziği başlat", "spotify devam et".`},
		{Name: ActionPause, Desc: `"spotify.pause": pause Spotify playback. No args. Use for "müziği durdur", "duraklat".`},
		{Name: ActionNext, Desc: `"spotify.next": skip to the next track. No args. Use for "sonraki şarkı", "geç".`},
		{Name: ActionPrevious, Desc: `"spotify.previous": go to the previous track. No args. Use for "önceki şarkı".`},
		{Name: ActionVolumeUp, Desc: `"spotify.volume_up": raise Spotify volume. No args. Use for "sesi aç".`},
		{Name: ActionVolumeDown, Desc: `"spotify.volume_down": lower Spotify volume. No args. Use for "sesi kıs".`},
		{
			Name: ActionPlayTrack,
			Args: []string{"query"},
			Desc: `"spotify.play_track": search Spotify and play the best match. Put the song/artist/mood in "query". Use for "lo-fi çal", "Tarkan çal", "Bohemian Rhapsody oynat".`,
		},
		{Name: ActionNowPlaying, Desc: `"spotify.now_playing": say what's currently playing on Spotify. No args. Use for "şu an ne çalıyor".`},
	}
}

func (s *Spotify) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	api, err := s.client()
	if err != nil {
		return "", err
	}
	switch action {
	case ActionPlay:
		if err := api.resume(ctx); err != nil {
			return "", err
		}
		return i18n.T("spotify.playing"), nil
	case ActionPause:
		if err := api.pause(ctx); err != nil {
			return "", err
		}
		return i18n.T("spotify.paused"), nil
	case ActionNext:
		if err := api.next(ctx); err != nil {
			return "", err
		}
		return i18n.T("spotify.next"), nil
	case ActionPrevious:
		if err := api.previous(ctx); err != nil {
			return "", err
		}
		return i18n.T("spotify.prev"), nil
	case ActionVolumeUp:
		return s.nudgeVolume(ctx, api, +volumeStep)
	case ActionVolumeDown:
		return s.nudgeVolume(ctx, api, -volumeStep)
	case ActionPlayTrack:
		return s.playTrack(ctx, api, args)
	case ActionNowPlaying:
		return s.currentlyPlaying(ctx, api)
	default:
		return "", fmt.Errorf("spotify: unknown action %q", action)
	}
}

func (s *Spotify) nudgeVolume(ctx context.Context, api spAPI, delta int) (string, error) {
	pb, err := api.nowPlaying(ctx)
	if err != nil {
		return "", err
	}
	if !pb.HasDevice {
		return "", errNoDevice
	}
	target := clampVolume(pb.Volume + delta)
	if err := api.setVolume(ctx, target); err != nil {
		return "", err
	}
	return fmt.Sprintf("Ses %%%d.", target), nil
}

func (s *Spotify) playTrack(ctx context.Context, api spAPI, args map[string]string) (string, error) {
	query := strings.TrimSpace(args["query"])
	if query == "" {
		return "", errors.New("spotify: nothing to play was named")
	}
	label, err := api.playQuery(ctx, query)
	if err != nil {
		return "", err
	}
	if label == "" {
		return i18n.T("spotify.no_match", query), nil
	}
	return i18n.T("spotify.now_playing", label), nil
}

func (s *Spotify) currentlyPlaying(ctx context.Context, api spAPI) (string, error) {
	pb, err := api.nowPlaying(ctx)
	if err != nil {
		return "", err
	}
	if !pb.HasDevice || pb.Track == "" {
		return i18n.T("spotify.nothing_playing"), nil
	}
	if !pb.IsPlaying {
		return i18n.T("spotify.paused_track", pb.Track), nil
	}
	return i18n.T("spotify.current_track", pb.Track), nil
}

func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// client lazily builds the real Spotify API from the saved OAuth token, unless
// one was injected (tests).
func (s *Spotify) client() (spAPI, error) {
	if s.api != nil {
		return s.api, nil
	}
	if !Configured(s.cfg) {
		return nil, errors.New("spotify: not configured (client_id + `pylon auth spotify`)")
	}
	return &realSpotify{cfg: s.cfg}, nil
}

// errNoDevice / errPremium are the two friendly failure modes users actually hit
// with Spotify playback control.
var (
	errNoDevice = errors.New("no active Spotify device — open Spotify somewhere and try again")
	errPremium  = errors.New("this needs Spotify Premium")
)

// realSpotify calls the Spotify Web API over the OAuth http.Client (auto-refresh).
type realSpotify struct {
	cfg Config
	hc  *http.Client // built on first use from the saved token
}

func (r *realSpotify) http(ctx context.Context) (*http.Client, error) {
	if r.hc == nil {
		hc, err := httpClient(ctx, r.cfg)
		if err != nil {
			return nil, err
		}
		r.hc = hc
	}
	return r.hc, nil
}

// call performs one Web API request. out may be nil for control endpoints (which
// return 204). It maps Spotify's playback-specific errors to friendly messages.
func (r *realSpotify) call(ctx context.Context, method, path string, body io.Reader, out any) error {
	hc, err := r.http(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNoContent:
		return nil // 204: success with no body (control endpoints; or nothing playing)
	case resp.StatusCode == http.StatusNotFound:
		return errNoDevice // Spotify returns 404 NO_ACTIVE_DEVICE
	case resp.StatusCode == http.StatusForbidden:
		return errPremium
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("spotify API %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (r *realSpotify) resume(ctx context.Context) error {
	return r.call(ctx, http.MethodPut, "/me/player/play", nil, nil)
}
func (r *realSpotify) pause(ctx context.Context) error {
	return r.call(ctx, http.MethodPut, "/me/player/pause", nil, nil)
}
func (r *realSpotify) next(ctx context.Context) error {
	return r.call(ctx, http.MethodPost, "/me/player/next", nil, nil)
}
func (r *realSpotify) previous(ctx context.Context) error {
	return r.call(ctx, http.MethodPost, "/me/player/previous", nil, nil)
}

func (r *realSpotify) setVolume(ctx context.Context, percent int) error {
	return r.call(ctx, http.MethodPut, fmt.Sprintf("/me/player/volume?volume_percent=%d", percent), nil, nil)
}

func (r *realSpotify) nowPlaying(ctx context.Context) (Playback, error) {
	var out struct {
		IsPlaying bool `json:"is_playing"`
		Device    struct {
			VolumePercent int `json:"volume_percent"`
		} `json:"device"`
		Item *struct {
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := r.call(ctx, http.MethodGet, "/me/player", nil, &out); err != nil {
		return Playback{}, err
	}
	// A 204 leaves out zero-valued → no active device.
	if out.Device.VolumePercent == 0 && out.Item == nil && !out.IsPlaying {
		return Playback{HasDevice: false}, nil
	}
	pb := Playback{IsPlaying: out.IsPlaying, Volume: out.Device.VolumePercent, HasDevice: true}
	if out.Item != nil {
		pb.Track = trackLabel(out.Item.Name, artistNames(out.Item.Artists))
	}
	return pb, nil
}

func (r *realSpotify) playQuery(ctx context.Context, query string) (string, error) {
	var sr struct {
		Tracks struct {
			Items []struct {
				URI     string `json:"uri"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	path := "/search?type=track&limit=1&q=" + url.QueryEscape(query)
	if err := r.call(ctx, http.MethodGet, path, nil, &sr); err != nil {
		return "", err
	}
	if len(sr.Tracks.Items) == 0 {
		return "", nil
	}
	t := sr.Tracks.Items[0]
	payload, _ := json.Marshal(map[string]any{"uris": []string{t.URI}})
	if err := r.call(ctx, http.MethodPut, "/me/player/play", bytes.NewReader(payload), nil); err != nil {
		return "", err
	}
	return trackLabel(t.Name, artistNames(t.Artists)), nil
}

// artistNames flattens an artist list (same shape in the player and search
// responses) into a comma-separated string.
func artistNames(a []struct {
	Name string `json:"name"`
}) string {
	names := make([]string, 0, len(a))
	for _, x := range a {
		names = append(names, x.Name)
	}
	return strings.Join(names, ", ")
}

func trackLabel(name, artists string) string {
	if artists == "" {
		return name
	}
	return name + " — " + artists
}
