//go:build windows

package watcher

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ListProcesses returns the set of running process names via tasklist — no cgo,
// keeping the daemon cross-compilable. The ".exe" suffix is stripped so a
// watch_processes entry reads the same as on Unix ("code", not "Code.exe").
// Names keep their original case, which is how Windows reports them.
func ListProcesses() (map[string]struct{}, error) {
	// /nh drops the header row; /fo csv quotes fields so a space in an image
	// name cannot be mistaken for a column break.
	out, err := exec.Command("tasklist", "/nh", "/fo", "csv").Output()
	if err != nil {
		return nil, fmt.Errorf("watcher: tasklist: %w", err)
	}

	names := make(map[string]struct{})
	r := csv.NewReader(bytes.NewReader(out))
	r.FieldsPerRecord = -1 // tolerate rows with unexpected column counts
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) == 0 {
			continue
		}
		name := strings.TrimSuffix(rec[0], ".exe")
		name = strings.TrimSuffix(name, ".EXE")
		if name = strings.TrimSpace(name); name != "" {
			names[name] = struct{}{}
		}
	}
	return names, nil
}
