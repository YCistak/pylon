package sysmon

import (
	"github.com/YCistak/pylon/internal/i18n"
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
		50 * time.Hour:               "2 days 2 hours",
		5*time.Hour + 12*time.Minute: "5 hours 12 minutes",
		8 * time.Minute:              "8 minutes",
		61 * time.Minute:             "1 hour 1 minute", // singular on both sides
	}
	for d, want := range cases {
		if got := i18n.Uptime(d); got != want {
			t.Errorf("Uptime(%v) = %q, want %q", d, got, want)
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
	if strings.Contains(got, "free on disk") {
		t.Errorf("summary spoke disk with no reading: %q", got)
	}
	if !strings.Contains(got, "CPU load 0.50") || !strings.Contains(got, "RAM 27%") {
		t.Errorf("summary missing the parts it does have: %q", got)
	}
}

// The fractional readings used to come out of a `%.2f` in the catalog, so a
// Turkish window said "CPU yükü 0.18" — Go's decimal mark inside a Turkish
// sentence. The whole-number readings are deliberately not part of this: "177 GB"
// has no decimal mark to get wrong.
func TestSummaryPunctuatesReadingsByLanguage(t *testing.T) {
	st := Stats{Load1: 0.5, MemTotGB: 31.1, MemUsedGB: 8.4, MemPct: 27}
	for lang, want := range map[string][]string{
		"en": {"CPU load 0.50", "8.4/31.1 GB"},
		"tr": {"CPU yükü 0,50", "8,4/31,1 GB"},
		"de": {"CPU-Last 0,50", "8,4/31,1 GB"},
	} {
		i18n.SetLanguage(lang)
		got := st.summary()
		for _, part := range want {
			if !strings.Contains(got, part) {
				t.Errorf("summary in %s = %q, want it to contain %q", lang, got, part)
			}
		}
	}
	i18n.SetLanguage(i18n.Default)
}
