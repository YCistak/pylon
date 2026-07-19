package watcher

import (
	"runtime"
	"testing"
)

// The real lister must enumerate processes on whichever OS the test runs on —
// /proc on Linux, ps on macOS, tasklist on Windows. This is the point of the
// cross-platform listers, and it can only be verified on each OS, so CI runs it
// on all three runners. The test process itself is always running, so a
// non-empty, error-free result is the honest proof the enumeration works.
func TestListProcessesEnumeratesThisOS(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skipf("no native lister on %s", runtime.GOOS)
	}

	got, err := ListProcesses()
	if err != nil {
		t.Fatalf("ListProcesses on %s: %v", runtime.GOOS, err)
	}
	if len(got) == 0 {
		t.Fatalf("ListProcesses returned no processes on %s; enumeration is broken", runtime.GOOS)
	}
	t.Logf("%s: %d processes", runtime.GOOS, len(got))
}
