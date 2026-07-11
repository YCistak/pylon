package docker

import (
	"context"
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

func svc(api dockerAPI) *Docker { return &Docker{api: api} }

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
