package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// daemonManager launches the Pylon daemon as a child process when the GUI
// starts, and stops it when the GUI quits — but only if the GUI is the one that
// started it. The GUI stays a pure IPC client (see app.go): this shells out to
// the same `pylon` binary the user would run by hand, so no daemon logic is
// imported into the GUI module.
type daemonManager struct {
	mu    sync.Mutex
	cmd   *exec.Cmd // non-nil only while WE own a spawned daemon
	owned bool      // true only if this process started the daemon
}

// ensureRunning starts the daemon if nothing is answering on the socket. If a
// daemon is already up (e.g. the user ran `pylon start` by hand), it is left
// untouched and we take no ownership — so shutdown never kills a daemon we did
// not start.
func (m *daemonManager) ensureRunning() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if daemonReachable() {
		return
	}

	bin, err := locatePylonBinary()
	if err != nil {
		log.Printf("pylon: daemon otomatik başlatılamadı: %v", err)
		return
	}

	cmd := exec.Command(bin, "start")
	cmd.Dir = filepath.Dir(bin) // repo root, so config's relative paths resolve
	cmd.Env = os.Environ()
	if cfg := pylonConfigPath(bin); cfg != "" {
		cmd.Env = append(cmd.Env, "PYLON_CONFIG="+cfg)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	killOnParentDeath(cmd) // die with the GUI on a crash (see procattr_*.go)

	if err := cmd.Start(); err != nil {
		log.Printf("pylon: daemon başlatılamadı (%s): %v", bin, err)
		return
	}
	m.cmd = cmd
	m.owned = true
	log.Printf("pylon: daemon başlatıldı (%s, pid %d)", bin, cmd.Process.Pid)

	// Wait (bounded) for the socket so the first status poll already sees it up.
	waitForDaemon(6 * time.Second)
}

// stop shuts the daemon down, but only if this process started it.
func (m *daemonManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.owned || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	// Graceful: ask it to stop over IPC, then reap the child.
	if bin, err := locatePylonBinary(); err == nil {
		stop := exec.Command(bin, "stop")
		stop.Dir = filepath.Dir(bin)
		stop.Env = os.Environ()
		if cfg := pylonConfigPath(bin); cfg != "" {
			stop.Env = append(stop.Env, "PYLON_CONFIG="+cfg)
		}
		_ = stop.Run()
	}

	done := make(chan struct{})
	go func() { _ = m.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		_ = m.cmd.Process.Kill() // last resort if graceful stop hung
		<-done
	}
	m.owned = false
	m.cmd = nil
}

// daemonReachable reports whether a daemon answers on the socket right now.
func daemonReachable() bool {
	_, err := send(request{Cmd: "ping"})
	return err == nil
}

// waitForDaemon polls the socket until a daemon answers or the timeout elapses.
func waitForDaemon(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonReachable() {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// locatePylonBinary finds the `pylon` executable: $PYLON_BIN, then next to and
// one level up from the GUI executable and the working directory (covers both
// `wails dev` — cwd is pylon-ui, binary at repo root — and a bundled build),
// then $PATH.
func locatePylonBinary() (string, error) {
	if p := os.Getenv("PYLON_BIN"); p != "" {
		return p, nil
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, d, filepath.Join(d, ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd, filepath.Join(wd, ".."))
	}
	name := "pylon"
	if runtime.GOOS == "windows" {
		name = "pylon.exe" // a bare "pylon" stat never matches on Windows
	}
	for _, d := range dirs {
		c := filepath.Join(d, name)
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			if abs, err := filepath.Abs(c); err == nil {
				return abs, nil
			}
			return c, nil
		}
	}
	if p, err := exec.LookPath("pylon"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("pylon binary bulunamadı (PYLON_BIN ile yol verebilirsin)")
}

// pylonConfigPath picks the config the daemon should load: $PYLON_CONFIG, else a
// pylon.local.yaml sitting next to the binary, else "" (daemon uses its own
// default pylon.yaml).
func pylonConfigPath(bin string) string {
	if p := os.Getenv("PYLON_CONFIG"); p != "" {
		return p
	}
	local := filepath.Join(filepath.Dir(bin), "pylon.local.yaml")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}
