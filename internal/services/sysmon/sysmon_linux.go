//go:build linux

package sysmon

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// readStats gathers the machine's vitals from /proc, /sys and statfs. A missing
// or unreadable source leaves its field zero rather than failing the whole
// snapshot — one absent sensor should not silence the rest.
func readStats(diskPath string) (Stats, error) {
	var st Stats

	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if v, err := parseLoad1(string(b)); err == nil {
			st.Load1 = v
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		if used, tot, pct, err := parseMem(string(b)); err == nil {
			st.MemUsedGB, st.MemTotGB, st.MemPct = used, tot, pct
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if v, err := parseUptime(string(b)); err == nil {
			st.Uptime = v
		}
	}
	st.TempC = readCPUTemp()

	var fs syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fs); err == nil {
		bsize := float64(fs.Bsize)
		gb := 1024.0 * 1024 * 1024
		st.DiskTotG = float64(fs.Blocks) * bsize / gb
		st.DiskFreeG = float64(fs.Bavail) * bsize / gb
	}

	return st, nil
}

// readCPUTemp returns the CPU package temperature in °C, or 0 if no usable
// sensor is found. Thermal zones are unordered and vary by machine, so it
// prefers a CPU-ish zone (x86_pkg_temp, coretemp, cpu) and falls back to the
// first zone that reads a plausible value.
func readCPUTemp() float64 {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	var fallback float64
	for _, z := range zones {
		tb, err := os.ReadFile(filepath.Join(z, "temp"))
		if err != nil {
			continue
		}
		c, err := parseTempMilliC(string(tb))
		if err != nil || c <= 0 || c > 150 { // guard against bogus readings
			continue
		}
		typ, _ := os.ReadFile(filepath.Join(z, "type"))
		t := strings.ToLower(strings.TrimSpace(string(typ)))
		if strings.Contains(t, "pkg") || strings.Contains(t, "coretemp") || strings.Contains(t, "cpu") {
			return c
		}
		if fallback == 0 {
			fallback = c
		}
	}
	return fallback
}
