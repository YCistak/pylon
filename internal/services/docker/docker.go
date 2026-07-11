// Package docker lets Pylon observe and control Docker containers on the host.
// The point is that the assistant can reach any self-hosted container — check
// whether it's running, what it consumes, and start/stop/restart it — so
// widgets are just an optional window onto capabilities Pylon already has.
//
// It talks to the Docker Engine API directly over the local Unix socket with
// plain net/http (a custom dialer), no docker SDK — consistent with the other
// services, which speak HTTP behind a small interface so they can be faked in
// tests.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/intent"
)

// Docker actions. The observe actions (ps/status/stats) back the widgets; the
// control actions (start/stop/restart) are reached by voice/intent.
const (
	ActionPS      intent.Action = "docker.ps"
	ActionStatus  intent.Action = "docker.status"
	ActionStats   intent.Action = "docker.stats"
	ActionStart   intent.Action = "docker.start"
	ActionStop    intent.Action = "docker.stop"
	ActionRestart intent.Action = "docker.restart"
)

// DefaultSocket is the standard Docker Engine socket on Linux.
const DefaultSocket = "/var/run/docker.sock"

// Config selects how to reach the Docker Engine. Socket is the local Unix
// socket (default). Host overrides it with an http(s) base URL for a remote
// Engine (e.g. "http://10.0.0.5:2375"); Token, if set, is sent as a Bearer
// header — for a proxy that fronts the Engine with auth.
type Config struct {
	Socket string
	Host   string
	Token  string
}

// Container is the minimal shape Pylon needs, decoupled from the Engine JSON.
type Container struct {
	Name   string
	State  string // "running", "exited", ...
	Status string // human blurb, e.g. "Up 2 hours"
	Image  string
}

func (c Container) Running() bool { return c.State == "running" }

// Stats is a one-shot resource sample for a container.
type Stats struct {
	CPUPercent float64
	MemBytes   uint64
	MemLimit   uint64
}

// dockerAPI is the slice of the Engine API the service uses; a fake implements
// it in tests.
type dockerAPI interface {
	list(ctx context.Context) ([]Container, error)
	stats(ctx context.Context, name string) (Stats, error)
	control(ctx context.Context, name, verb string) error // verb: start|stop|restart
}

// Docker is the Docker Service.
type Docker struct {
	cfg Config
	api dockerAPI // injected in tests; otherwise built lazily
}

// New builds the service from config, defaulting the socket. It does not touch
// Docker until first use.
func New(cfg Config) *Docker {
	if strings.TrimSpace(cfg.Socket) == "" {
		cfg.Socket = DefaultSocket
	}
	return &Docker{cfg: cfg}
}

// Configured reports whether Docker is reachable — a remote Host is set, or the
// local socket exists. main.go registers the service only when true, so hosts
// without Docker simply don't get the actions.
func Configured(cfg Config) bool {
	if strings.TrimSpace(cfg.Host) != "" {
		return true
	}
	sock := strings.TrimSpace(cfg.Socket)
	if sock == "" {
		sock = DefaultSocket
	}
	_, err := os.Stat(sock)
	return err == nil
}

func (d *Docker) Name() string { return "docker" }

func (d *Docker) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionPS,
			Desc: `"docker.ps": list the running Docker containers on this machine. No args. Use for "hangi konteynerler çalışıyor", "docker'da neler açık".`,
		},
		{
			Name: ActionStatus,
			Args: []string{"container"},
			Desc: `"docker.status": is a specific container running? Put its name in "container". Use for "freshrss ayakta mı", "grafana çalışıyor mu".`,
		},
		{
			Name: ActionStats,
			Args: []string{"container"},
			Desc: `"docker.stats": CPU and memory a container uses right now. Put its name in "container". Use for "freshrss ne kadar ram yiyor", "grafana kaç cpu kullanıyor".`,
		},
		{
			Name: ActionStart,
			Args: []string{"container"},
			Desc: `"docker.start": start a stopped container. Put its name in "container". Use for "freshrss'i başlat", "grafana'yı aç".`,
		},
		{
			Name: ActionStop,
			Args: []string{"container"},
			Desc: `"docker.stop": stop a running container. Put its name in "container". Use for "freshrss'i durdur", "grafana'yı kapat".`,
		},
		{
			Name: ActionRestart,
			Args: []string{"container"},
			Desc: `"docker.restart": restart a container. Put its name in "container". Use for "freshrss'i yeniden başlat", "grafana'yı resetle".`,
		},
	}
}

func (d *Docker) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	api, err := d.client()
	if err != nil {
		return "", err
	}
	switch action {
	case ActionPS:
		return d.ps(ctx, api)
	case ActionStatus:
		return d.status(ctx, api, args["container"])
	case ActionStats:
		return d.statsReply(ctx, api, args["container"])
	case ActionStart:
		return d.controlReply(ctx, api, args["container"], "start")
	case ActionStop:
		return d.controlReply(ctx, api, args["container"], "stop")
	case ActionRestart:
		return d.controlReply(ctx, api, args["container"], "restart")
	default:
		return "", fmt.Errorf("docker: bilinmeyen aksiyon %q", action)
	}
}

func (d *Docker) ps(ctx context.Context, api dockerAPI) (string, error) {
	all, err := api.list(ctx)
	if err != nil {
		return "", err
	}
	var running []string
	stopped := 0
	for _, c := range all {
		if c.Running() {
			running = append(running, c.Name)
		} else {
			stopped++
		}
	}
	sort.Strings(running)
	if len(running) == 0 {
		if stopped == 0 {
			return "Hiç konteyner yok.", nil
		}
		return "Çalışan konteyner yok.", nil
	}
	reply := fmt.Sprintf("%d konteyner çalışıyor: %s.", len(running), strings.Join(running, ", "))
	if stopped > 0 {
		reply += fmt.Sprintf(" (%d tanesi durdurulmuş)", stopped)
	}
	return reply, nil
}

func (d *Docker) status(ctx context.Context, api dockerAPI, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("konteyner adı gerekli")
	}
	c, err := find(ctx, api, name)
	if err != nil {
		return "", err
	}
	if c.Running() {
		if c.Status != "" {
			return fmt.Sprintf("%s çalışıyor (%s).", c.Name, c.Status), nil
		}
		return fmt.Sprintf("%s çalışıyor.", c.Name), nil
	}
	return fmt.Sprintf("%s çalışmıyor (%s).", c.Name, statusOr(c, "durduruldu")), nil
}

func (d *Docker) statsReply(ctx context.Context, api dockerAPI, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("konteyner adı gerekli")
	}
	// A stopped container has no stats — say so plainly instead of erroring.
	c, err := find(ctx, api, name)
	if err != nil {
		return "", err
	}
	if !c.Running() {
		return fmt.Sprintf("%s çalışmıyor.", c.Name), nil
	}
	s, err := api.stats(ctx, c.Name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %%%.0f CPU, %s RAM.", c.Name, s.CPUPercent, humanBytes(s.MemBytes)), nil
}

func (d *Docker) controlReply(ctx context.Context, api dockerAPI, name, verb string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("konteyner adı gerekli")
	}
	// Resolve the friendly name first so control errors read cleanly.
	c, err := find(ctx, api, name)
	if err != nil {
		return "", err
	}
	if err := api.control(ctx, c.Name, verb); err != nil {
		return "", err
	}
	switch verb {
	case "start":
		return fmt.Sprintf("%s başlatıldı.", c.Name), nil
	case "stop":
		return fmt.Sprintf("%s durduruldu.", c.Name), nil
	case "restart":
		return fmt.Sprintf("%s yeniden başlatıldı.", c.Name), nil
	default:
		return "", fmt.Errorf("docker: bilinmeyen işlem %q", verb)
	}
}

// find locates a container by name, case-insensitively, tolerating the leading
// slash Docker puts on names.
func find(ctx context.Context, api dockerAPI, name string) (Container, error) {
	want := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
	all, err := api.list(ctx)
	if err != nil {
		return Container{}, err
	}
	for _, c := range all {
		if strings.ToLower(c.Name) == want {
			return c, nil
		}
	}
	return Container{}, fmt.Errorf("%q adında konteyner yok", name)
}

func statusOr(c Container, fallback string) string {
	if c.Status != "" {
		return c.Status
	}
	return fallback
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(b)/float64(div), "KMGT"[exp])
}

func (d *Docker) client() (dockerAPI, error) {
	if d.api != nil {
		return d.api, nil
	}
	return newEngine(d.cfg), nil
}

// engine calls the Docker Engine API over HTTP — the Unix socket by default, or
// a remote Host if configured.
type engine struct {
	base  string // URL base; "http://docker" for the socket transport
	token string
	hc    *http.Client
}

func newEngine(cfg Config) *engine {
	if host := strings.TrimSpace(cfg.Host); host != "" {
		return &engine{
			base:  strings.TrimRight(host, "/"),
			token: cfg.Token,
			hc:    &http.Client{Timeout: 15 * time.Second},
		}
	}
	sock := strings.TrimSpace(cfg.Socket)
	if sock == "" {
		sock = DefaultSocket
	}
	return &engine{
		base:  "http://docker",
		token: cfg.Token,
		hc: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", sock)
				},
			},
		},
	}
}

func (e *engine) do(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, e.base+path, nil)
	if err != nil {
		return nil, err
	}
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	return e.hc.Do(req)
}

func (e *engine) list(ctx context.Context) ([]Container, error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/json?all=1")
	if err != nil {
		return nil, dialErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiErr(resp)
	}
	var raw []struct {
		Names  []string `json:"Names"`
		State  string   `json:"State"`
		Status string   `json:"Status"`
		Image  string   `json:"Image"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, Container{Name: name, State: r.State, Status: r.Status, Image: r.Image})
	}
	return out, nil
}

func (e *engine) stats(ctx context.Context, name string) (Stats, error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+name+"/stats?stream=false")
	if err != nil {
		return Stats{}, dialErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Stats{}, apiErr(resp)
	}
	var s statsJSON
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return Stats{}, err
	}
	return s.compute(), nil
}

func (e *engine) control(ctx context.Context, name, verb string) error {
	resp, err := e.do(ctx, http.MethodPost, "/containers/"+name+"/"+verb)
	if err != nil {
		return dialErr(err)
	}
	defer resp.Body.Close()
	// 204 = done; 304 = already in the target state (start an already-running
	// container / stop a stopped one) — treat as success, the end state matches.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	return apiErr(resp)
}

// statsJSON mirrors the Engine stats payload we care about; compute() turns the
// two CPU samples into a percentage the way `docker stats` does.
type statsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			Cache        uint64 `json:"cache"`
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
}

func (s statsJSON) compute() Stats {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		cpus = 1
	}
	var pct float64
	if sysDelta > 0 && cpuDelta > 0 {
		pct = (cpuDelta / sysDelta) * cpus * 100
	}

	// Match `docker stats`: subtract page cache (cgroup v1 "cache", v2
	// "inactive_file") from usage so RAM reflects the working set.
	mem := s.MemoryStats.Usage
	cache := s.MemoryStats.Stats.Cache
	if cache == 0 {
		cache = s.MemoryStats.Stats.InactiveFile
	}
	if cache <= mem {
		mem -= cache
	}
	return Stats{CPUPercent: pct, MemBytes: mem, MemLimit: s.MemoryStats.Limit}
}

func dialErr(err error) error {
	return fmt.Errorf("docker: Engine'e ulaşılamadı (socket erişimi var mı?): %w", err)
}

func apiErr(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	msg := strings.TrimSpace(string(body))
	// Docker error bodies are JSON like {"message":"No such container: x"}.
	var j struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(msg), &j) == nil && j.Message != "" {
		msg = j.Message
	}
	return fmt.Errorf("docker API %d: %s", resp.StatusCode, msg)
}
