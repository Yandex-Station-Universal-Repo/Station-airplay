package main

// Switching Alice (Quasar) off for AirPlay and back on, for models where she
// may also keep running (Midi). The mode comes from STATION_AIRPLAY_QUASAR or
// the file /data/shairport/quasar: "on" (default: AirPlay plays next to Alice)
// or "off" (Alice is stopped while the receiver runs, as on the Plum).
//
// Stopping is a TERM to quasar_launcher2, which takes all maind/bulbik daemons
// down with it; starting is the firmware's own run.sh. A detached watcher
// restarts Alice once this process is gone, so she also comes back when YAM
// stops the service with SIGKILL or the receiver crashes.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	quasarDir      = "/system/vendor/quasar"
	quasarModeEnv  = "STATION_AIRPLAY_QUASAR"
	quasarModeFile = "/data/shairport/quasar"
)

// quasarWanted reports whether Alice should keep running ("on" unless set to "off").
func quasarWanted() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(quasarModeEnv))
	if raw == "" {
		if b, e := os.ReadFile(quasarModeFile); e == nil {
			raw = strings.TrimSpace(string(b))
		}
	}
	switch strings.ToLower(raw) {
	case "", "on":
		return true, nil
	case "off":
		return false, nil
	}
	return false, fmt.Errorf("%s: unknown Quasar mode %q (on|off)", quasarModeFile, raw)
}

// stopQuasar stops Alice and arranges for her restart when pid exits.
func stopQuasar(pid int) error {
	if _, e := os.Stat(quasarDir + "/run.sh"); e != nil {
		return errors.New("Quasar run.sh not found; cannot stop Alice safely")
	}
	if e := startQuasarAfter(pid); e != nil {
		return e
	}
	for _, p := range pidsOf("quasar_launcher2") {
		_ = syscall.Kill(p, syscall.SIGTERM)
	}
	deadline := time.Now().Add(20 * time.Second)
	for quasarRunning() {
		if time.Now().After(deadline) {
			return errors.New("Quasar did not stop within 20 s")
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println("Quasar stopped (mode off); it restarts when the receiver exits")
	return nil
}

// startQuasarAfter starts a detached shell (own session, no Pdeathsig) that
// waits for pid to exit and then starts Quasar unless it already runs.
func startQuasarAfter(pid int) error {
	script := `while kill -0 ` + strconv.Itoa(pid) + ` 2>/dev/null; do sleep 1; done
pidof quasar_launcher2 >/dev/null && exit 0
cd ` + quasarDir + ` && LD_LIBRARY_PATH=` + quasarDir + `:` + quasarDir + `/libs PATH=` + quasarDir + `:$PATH GST_PLUGIN_PATH=` + quasarDir + `/libs ./run.sh`
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e := cmd.Start(); e != nil {
		return fmt.Errorf("Quasar restart watcher: %w", e)
	}
	return cmd.Process.Release()
}

func pidsOf(program string) []int {
	var pids []int
	ents, _ := os.ReadDir("/proc")
	for _, ent := range ents {
		pid, e := strconv.Atoi(ent.Name())
		if e != nil {
			continue
		}
		cmd, e := os.ReadFile("/proc/" + ent.Name() + "/cmdline")
		if e != nil {
			continue
		}
		if first, _, _ := strings.Cut(string(cmd), "\x00"); strings.HasSuffix(first, "/"+program) || first == program {
			pids = append(pids, pid)
		}
	}
	return pids
}
