package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeAPI implements dockerAPI for tests, recording control calls.
type fakeAPI struct {
	containers []Container
	stat       Stats
	statErr    error
	listErr    error
	controlled []string // "name:verb"
	controlErr error
	logText    string
	logTail    int // records the tail requested
}

func (f *fakeAPI) list(context.Context) ([]Container, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.containers, nil
}
func (f *fakeAPI) stats(_ context.Context, name string) (Stats, error) {
	if f.statErr != nil {
		return Stats{}, f.statErr
	}
	return f.stat, nil
}
func (f *fakeAPI) control(_ context.Context, name, verb string) error {
	if f.controlErr != nil {
		return f.controlErr
	}
	f.controlled = append(f.controlled, name+":"+verb)
	return nil
}
func (f *fakeAPI) logs(_ context.Context, name string, tail int) (string, error) {
	f.logTail = tail
	return f.logText, nil
}

func svc(api dockerAPI) *Docker { return &Docker{api: api} }

func TestListJSON(t *testing.T) {
	api := &fakeAPI{containers: []Container{
		{Name: "grafana", State: "exited", Status: "Exited (0)", Image: "grafana/grafana"},
		{Name: "freshrss", State: "running", Status: "Up 2 hours", Image: "freshrss/freshrss"},
	}}
	out, err := svc(api).Execute(context.Background(), ActionList, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []Container
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, out)
	}
	// Sorted by name → freshrss first.
	if len(got) != 2 || got[0].Name != "freshrss" || got[1].Name != "grafana" {
		t.Errorf("unexpected list: %+v", got)
	}
	// json tags are lowercase for the GUI.
	if !strings.Contains(out, `"state":"running"`) {
		t.Errorf("expected lowercase json tags, got %s", out)
	}
}

func TestPS(t *testing.T) {
	tests := []struct {
		name string
		cs   []Container
		want string
	}{
		{
			name: "running with one stopped",
			cs: []Container{
				{Name: "grafana", State: "running"},
				{Name: "freshrss", State: "running"},
				{Name: "old", State: "exited"},
			},
			want: "2 konteyner çalışıyor: freshrss, grafana. (1 tanesi durdurulmuş)",
		},
		{
			name: "none running",
			cs:   []Container{{Name: "old", State: "exited"}},
			want: "Çalışan konteyner yok.",
		},
		{
			name: "empty host",
			cs:   nil,
			want: "Hiç konteyner yok.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc(&fakeAPI{containers: tt.cs}).Execute(context.Background(), ActionPS, nil)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	api := &fakeAPI{containers: []Container{
		{Name: "freshrss", State: "running", Status: "Up 2 hours"},
		{Name: "grafana", State: "exited", Status: "Exited (0) 1 hour ago"},
	}}
	d := svc(api)

	got, err := d.Execute(context.Background(), ActionStatus, map[string]string{"container": "freshrss"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "freshrss çalışıyor (Up 2 hours)." {
		t.Errorf("running: got %q", got)
	}

	got, _ = d.Execute(context.Background(), ActionStatus, map[string]string{"container": "grafana"})
	if !strings.Contains(got, "çalışmıyor") {
		t.Errorf("stopped: got %q", got)
	}

	// Case-insensitive + leading-slash tolerance.
	got, err = d.Execute(context.Background(), ActionStatus, map[string]string{"container": "/FreshRSS"})
	if err != nil || !strings.Contains(got, "çalışıyor") {
		t.Errorf("normalize: got %q err %v", got, err)
	}

	// Unknown container is a clean error, not a crash.
	if _, err := d.Execute(context.Background(), ActionStatus, map[string]string{"container": "nope"}); err == nil {
		t.Error("expected error for unknown container")
	}

	// Missing arg.
	if _, err := d.Execute(context.Background(), ActionStatus, map[string]string{}); err == nil {
		t.Error("expected error for missing container arg")
	}
}

func TestStats(t *testing.T) {
	api := &fakeAPI{
		containers: []Container{{Name: "freshrss", State: "running"}},
		stat:       Stats{CPUPercent: 2.4, MemBytes: 134217728}, // 128 MiB
	}
	got, err := svc(api).Execute(context.Background(), ActionStats, map[string]string{"container": "freshrss"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "freshrss: %2 CPU, 128 MB RAM." {
		t.Errorf("got %q", got)
	}

	// Stopped container: no stats call, plain reply.
	api2 := &fakeAPI{containers: []Container{{Name: "old", State: "exited"}}, statErr: errors.New("should not be called")}
	got, err = svc(api2).Execute(context.Background(), ActionStats, map[string]string{"container": "old"})
	if err != nil {
		t.Fatalf("stopped stats errored: %v", err)
	}
	if got != "old çalışmıyor." {
		t.Errorf("stopped: got %q", got)
	}
}

func TestControl(t *testing.T) {
	api := &fakeAPI{containers: []Container{{Name: "freshrss", State: "running"}}}
	d := svc(api)

	got, err := d.Execute(context.Background(), ActionStop, map[string]string{"container": "freshrss"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "freshrss durduruldu." {
		t.Errorf("stop: got %q", got)
	}
	if len(api.controlled) != 1 || api.controlled[0] != "freshrss:stop" {
		t.Errorf("control calls: %v", api.controlled)
	}

	got, _ = d.Execute(context.Background(), ActionRestart, map[string]string{"container": "freshrss"})
	if got != "freshrss yeniden başlatıldı." {
		t.Errorf("restart: got %q", got)
	}

	// Start uses the resolved name even if the API errors surface.
	api.controlErr = errors.New("boom")
	if _, err := d.Execute(context.Background(), ActionStart, map[string]string{"container": "freshrss"}); err == nil {
		t.Error("expected control error to propagate")
	}
}

func TestLogs(t *testing.T) {
	api := &fakeAPI{
		containers: []Container{{Name: "freshrss", State: "running"}},
		logText:    "line1\nline2\n",
	}
	d := svc(api)

	// Default tail, trailing newline trimmed.
	got, err := d.Execute(context.Background(), ActionLogs, map[string]string{"container": "freshrss"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "line1\nline2" {
		t.Errorf("got %q", got)
	}
	if api.logTail != defaultLogTail {
		t.Errorf("default tail = %d, want %d", api.logTail, defaultLogTail)
	}

	// Explicit lines arg is passed through; over-cap clamps.
	d.Execute(context.Background(), ActionLogs, map[string]string{"container": "freshrss", "lines": "50"})
	if api.logTail != 50 {
		t.Errorf("tail = %d, want 50", api.logTail)
	}
	d.Execute(context.Background(), ActionLogs, map[string]string{"container": "freshrss", "lines": "9999"})
	if api.logTail != logsMaxTail {
		t.Errorf("clamped tail = %d, want %d", api.logTail, logsMaxTail)
	}

	// Empty logs → plain reply.
	api.logText = ""
	got, _ = d.Execute(context.Background(), ActionLogs, map[string]string{"container": "freshrss"})
	if got != "freshrss için log yok." {
		t.Errorf("empty: got %q", got)
	}

	// Missing name.
	if _, err := d.Execute(context.Background(), ActionLogs, map[string]string{}); err == nil {
		t.Error("expected error for missing container")
	}
}

func TestDemuxLogs(t *testing.T) {
	// Two framed stdout messages: header = stream(1) 0 0 0 + size(4 BE).
	frame := func(stream byte, payload string) []byte {
		h := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
		return append(h, []byte(payload)...)
	}
	framed := append(frame(1, "hello\n"), frame(2, "err\n")...)
	if got := demuxLogs(framed); got != "hello\nerr\n" {
		t.Errorf("framed demux = %q", got)
	}

	// Raw (TTY) data with no valid framing is returned as-is.
	raw := []byte("plain tty output\n")
	if got := demuxLogs(raw); got != "plain tty output\n" {
		t.Errorf("raw demux = %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[uint64]string{
		512:        "512 B",
		134217728:  "128 MB",
		1073741824: "1 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestComputeCPU(t *testing.T) {
	var s statsJSON
	s.CPUStats.CPUUsage.TotalUsage = 200
	s.PreCPUStats.CPUUsage.TotalUsage = 100
	s.CPUStats.SystemUsage = 2000
	s.PreCPUStats.SystemUsage = 1000
	s.CPUStats.OnlineCPUs = 4
	s.MemoryStats.Usage = 200
	s.MemoryStats.Stats.Cache = 50

	out := s.compute()
	// (100/1000)*4*100 = 40%
	if out.CPUPercent != 40 {
		t.Errorf("cpu%% = %v, want 40", out.CPUPercent)
	}
	if out.MemBytes != 150 {
		t.Errorf("mem = %d, want 150 (usage-cache)", out.MemBytes)
	}
}

func TestConfigured(t *testing.T) {
	if !Configured(Config{Host: "http://remote:2375"}) {
		t.Error("remote host should be configured")
	}
	if Configured(Config{Socket: "/nonexistent/docker.sock"}) {
		t.Error("missing socket should not be configured")
	}
}
