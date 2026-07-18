package sysmon

import (
	"strings"
	"testing"
	"time"
)

func TestParseLoad1(t *testing.T) {
	got, err := parseLoad1("0.55 0.90 0.78 3/2000 190739")
	if err != nil || got != 0.55 {
		t.Fatalf("parseLoad1 = %v, %v; want 0.55", got, err)
	}
	if _, err := parseLoad1(""); err == nil {
		t.Error("empty loadavg should error")
	}
}

func TestParseMem(t *testing.T) {
	// 32603040 kB total, 23816908 kB available → used ≈ 8.38 GB of 31.09 GB, 27%.
	meminfo := "MemTotal:       32603040 kB\nMemFree:        18626016 kB\nMemAvailable:   23816908 kB\n"
	used, tot, pct, err := parseMem(meminfo)
	if err != nil {
		t.Fatalf("parseMem: %v", err)
	}
	if pct != 27 {
		t.Errorf("pct = %d, want 27", pct)
	}
	if tot < 31 || tot > 32 {
		t.Errorf("total = %.2f GB, want ~31", tot)
	}
	if used < 8 || used > 9 {
		t.Errorf("used = %.2f GB, want ~8.4", used)
	}
}

func TestParseMemMissingFields(t *testing.T) {
	if _, _, _, err := parseMem("MemFree: 100 kB\n"); err == nil {
		t.Error("meminfo without MemTotal/MemAvailable should error")
	}
}

func TestParseTempMilliC(t *testing.T) {
	c, err := parseTempMilliC("46000\n")
	if err != nil || c != 46 {
		t.Fatalf("parseTempMilliC = %v, %v; want 46", c, err)
	}
}

func TestParseUptime(t *testing.T) {
	d, err := parseUptime("190739.12 1500000.00")
	if err != nil {
		t.Fatal(err)
	}
	if d != 190739*time.Second {
		t.Errorf("uptime = %v, want 190739s", d)
	}
}

func TestHumanUptime(t *testing.T) {
	cases := map[time.Duration]string{
		50 * time.Hour:                    "2 gün 2 saat",
		5*time.Hour + 12*time.Minute:      "5 saat 12 dakika",
		8 * time.Minute:                   "8 dakika",
	}
	for d, want := range cases {
		if got := humanUptime(d); got != want {
			t.Errorf("humanUptime(%v) = %q, want %q", d, got, want)
		}
	}
}

// A snapshot with no temp or disk (an absent sensor) must drop those parts, not
// speak a zero — otherwise the user hears "0 derece" for a machine with no
// thermal zone.
func TestSummaryDropsMissingSensors(t *testing.T) {
	st := Stats{Load1: 0.5, MemTotGB: 31, MemUsedGB: 8.4, MemPct: 27} // no temp, no disk, no uptime
	got := st.summary()
	if strings.Contains(got, "derece") {
		t.Errorf("summary spoke a temperature that was not read: %q", got)
	}
	if strings.Contains(got, "disk") {
		t.Errorf("summary spoke disk with no reading: %q", got)
	}
	if !strings.Contains(got, "CPU yükü 0.50") || !strings.Contains(got, "RAM %27") {
		t.Errorf("summary missing the parts it does have: %q", got)
	}
}
