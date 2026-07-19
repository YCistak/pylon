//go:build darwin

package watcher

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ListProcesses returns the set of running process names. macOS has no /proc,
// so it shells out to ps — no cgo, keeping the daemon cross-compilable.
//
// ps prints the comm column, which may be a bare name or a full bundle path
// depending on the process; taking the basename normalises both to the
// executable name ("Code", not /Applications/…/MacOS/Code), so a
// watch_processes entry matches what a user would actually write.
func ListProcesses() (map[string]struct{}, error) {
	out, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("watcher: ps: %w", err)
	}

	names := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names[filepath.Base(line)] = struct{}{}
	}
	return names, nil
}
