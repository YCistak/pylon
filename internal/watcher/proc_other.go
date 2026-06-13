//go:build !linux

package watcher

import (
	"fmt"
	"runtime"
)

// ListProcesses is not yet implemented outside Linux. Windows (WMI) and macOS
// (kqueue / ps) listers are planned with the cross-platform work in Phase 4.
// Until then, callers should inject a Lister explicitly on these platforms.
func ListProcesses() (map[string]struct{}, error) {
	return nil, fmt.Errorf("watcher: process listing not implemented on %s yet", runtime.GOOS)
}
