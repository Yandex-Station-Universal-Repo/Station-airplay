package main

// Diagnostic log for the Alice button handover. The receiver deployment is
// read-only apart from /tmp and the plugin data directory, so the log lives in
// /tmp and is truncated by the supervisor at start.

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const aliceButtonDebugPath = "/tmp/station-airplay-button.log"

var (
	aliceButtonDebugMu   sync.Mutex
	aliceButtonDebugOnce sync.Once
)

func aliceButtonDebug(format string, args ...any) {
	aliceButtonDebugOnce.Do(func() {
		_ = os.Remove(aliceButtonDebugPath)
	})
	aliceButtonDebugMu.Lock()
	defer aliceButtonDebugMu.Unlock()
	f, err := os.OpenFile(aliceButtonDebugPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
