// Package sysmon reports the machine's own vitals — load, memory, disk,
// temperature, uptime — as a spoken summary ("CPU yükü 0.5, RAM %27 dolu…").
// It is distinct from the system service, which *controls* the machine (lock,
// volume); this only observes it.
//
// The readings come from /proc and /sys, so the reader is Linux-only
// (sysmon_linux.go); other platforms return a graceful "not supported"
// (sysmon_other.go). The parsing here is pure and file-free so it can be unit
// tested on any OS.
package sysmon

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// ActionStats reports a one-line summary of the machine's vitals.
const ActionStats intent.Action = "sysmon.stats"

// Stats is a point-in-time snapshot. Fields a platform cannot fill are left
// zero and dropped from the spoken summary, so a missing sensor never turns
// into a wrong reading.
type Stats struct {
	Load1     float64       // 1-minute load average
	MemUsedGB float64       // used = total - available
	MemTotGB  float64       //
	MemPct    int           // used percent, rounded
	DiskFreeG float64       // free space on the watched path
	DiskTotG  float64       //
	TempC     float64       // CPU package temperature; 0 when no sensor
	Uptime    time.Duration //
}

// Service reports machine vitals. It needs no configuration beyond which disk
// to report free space for, so it is always registered.
type Service struct {
	diskPath string
}

// New builds the service. diskPath is the mount whose free space is reported;
// empty means the root filesystem.
func New(diskPath string) *Service {
	if strings.TrimSpace(diskPath) == "" {
		diskPath = "/"
	}
	return &Service{diskPath: diskPath}
}

func (s *Service) Name() string { return "sysmon" }

func (s *Service) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionStats,
			Desc: `"sysmon.stats": report this machine's vitals — CPU load, RAM use, free disk, CPU temperature, uptime — as one spoken line. No args. Use for "sistem durumu", "bilgisayar nasıl", "ram doluluk", "cpu sıcaklığı", "ne kadar açık kaldı".`,
		},
	}
}

func (s *Service) Execute(_ context.Context, action intent.Action, _ map[string]string) (string, error) {
	switch action {
	case ActionStats:
		st, err := readStats(s.diskPath)
		if err != nil {
			return i18n.T("sysmon.unavailable"), nil
		}
		return st.summary(), nil
	default:
		return "", fmt.Errorf("sysmon: bilinmeyen aksiyon %q", action)
	}
}

// summary renders the snapshot as one line, dropping any part whose sensor was
// unavailable.
func (st Stats) summary() string {
	var parts []string
	parts = append(parts, i18n.T("sysmon.load", st.Load1))
	if st.MemTotGB > 0 {
		parts = append(parts, i18n.T("sysmon.ram", st.MemPct, st.MemUsedGB, st.MemTotGB))
	}
	if st.DiskTotG > 0 {
		parts = append(parts, i18n.T("sysmon.disk", st.DiskFreeG))
	}
	if st.TempC > 0 {
		parts = append(parts, i18n.T("sysmon.temp", st.TempC))
	}
	if st.Uptime > 0 {
		parts = append(parts, i18n.T("sysmon.uptime", i18n.Uptime(st.Uptime)))
	}
	return strings.Join(parts, ", ") + "."
}

// --- pure parsers (unit-tested; the Linux reader feeds them real file bytes) ---

// parseLoad1 pulls the 1-minute load average from /proc/loadavg's first field.
func parseLoad1(loadavg string) (float64, error) {
	f := strings.Fields(loadavg)
	if len(f) == 0 {
		return 0, fmt.Errorf("empty loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}

// parseMem pulls MemTotal and MemAvailable (both kB) from /proc/meminfo and
// returns used GB, total GB, and used percent.
func parseMem(meminfo string) (usedGB, totGB float64, pct int, err error) {
	var totKB, availKB float64
	var haveTot, haveAvail bool
	for _, line := range strings.Split(meminfo, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			totKB, err = strconv.ParseFloat(f[1], 64)
			haveTot = err == nil
		case "MemAvailable:":
			availKB, err = strconv.ParseFloat(f[1], 64)
			haveAvail = err == nil
		}
	}
	if !haveTot || !haveAvail || totKB <= 0 {
		return 0, 0, 0, fmt.Errorf("meminfo missing MemTotal/MemAvailable")
	}
	usedKB := totKB - availKB
	return usedKB / 1024 / 1024, totKB / 1024 / 1024, int((usedKB/totKB)*100 + 0.5), nil
}

// parseTempMilliC turns a /sys thermal-zone reading (millidegrees) into °C.
func parseTempMilliC(raw string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, err
	}
	return v / 1000, nil
}

// parseUptime pulls the seconds field from /proc/uptime.
func parseUptime(uptime string) (time.Duration, error) {
	f := strings.Fields(uptime)
	if len(f) == 0 {
		return 0, fmt.Errorf("empty uptime")
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs) * time.Second, nil
}
