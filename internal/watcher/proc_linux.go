//go:build linux

package watcher

import (
	"os"
	"strconv"
	"strings"
)

// ListProcesses returns the set of running process names by reading /proc.
//
// Names come from /proc/<pid>/comm, which the kernel truncates to 15 bytes
// (TASK_COMM_LEN-1). For the apps Pylon watches (code, cs2, steam) this is
// fine; very long executable names would need /proc/<pid>/stat parsing instead.
func ListProcesses() (map[string]struct{}, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		// Only numeric directories are PIDs.
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue // process likely exited between ReadDir and now
		}
		name := strings.TrimSpace(string(comm))
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names, nil
}
