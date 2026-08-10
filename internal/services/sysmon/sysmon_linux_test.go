//go:build linux

package sysmon

import (
	"context"
	"strings"
	"testing"
)

// The reader only has real /proc and /sys to work against on Linux, so its
// real proof runs there: gather this machine's vitals and check they are
// plausible, not just non-zero.
func TestReadStatsOnLinux(t *testing.T) {
	st, err := readStats("/")
	if err != nil {
		t.Fatalf("readStats: %v", err)
	}
	if st.Load1 < 0 {
		t.Errorf("negative load: %v", st.Load1)
	}
	if st.MemTotGB <= 0 {
		t.Error("no total memory read")
	}
	if st.MemUsedGB > st.MemTotGB {
		t.Errorf("used %.1f > total %.1f GB", st.MemUsedGB, st.MemTotGB)
	}
	if st.DiskTotG <= 0 {
		t.Error("no disk total read for /")
	}
	if st.Uptime <= 0 {
		t.Error("no uptime read")
	}
	t.Logf("vitals: %s", st.summary())
}

func TestExecuteStatsSpeaks(t *testing.T) {
	out, err := New("/").Execute(context.Background(), ActionStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CPU load") {
		t.Errorf("stats summary missing load: %q", out)
	}
}
