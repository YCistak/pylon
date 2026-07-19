//go:build !linux && !darwin && !windows

package watcher

import (
	"fmt"
	"runtime"
)

// ListProcesses has no implementation on platforms beyond Linux, macOS and
// Windows. Callers there must inject a Lister explicitly (watcher.Options.List).
func ListProcesses() (map[string]struct{}, error) {
	return nil, fmt.Errorf("watcher: process listing not implemented on %s", runtime.GOOS)
}
