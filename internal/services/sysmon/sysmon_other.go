//go:build !linux

package sysmon

import (
	"fmt"
	"runtime"
)

// readStats has no implementation off Linux: the vitals come from /proc and
// /sys. The service still registers so its action stays in the vocabulary; it
// just reports that it cannot read them here rather than returning wrong data.
func readStats(string) (Stats, error) {
	return Stats{}, fmt.Errorf("sysmon: not supported on %s", runtime.GOOS)
}
