package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeCLI mimics the stock SDK console: it announces the directive sequencer,
// accepts the explicit pause input and exits on "x".
const fakeCLI = `#!/bin/sh
[ "$1" = "cli" ] || exit 2
echo "Connected to DirectiveSequencer"
while c=$(dd bs=1 count=1 2>/dev/null); do
  [ -z "$c" ] && exit 0
  case "$c" in
    x) exit 0;;
    o) echo "UserInput: Player Pause";;
  esac
done
`

func writeFakeCLI(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNativePlayerPausesAndStops(t *testing.T) {
	p := newNativePlayer(writeFakeCLI(t, fakeCLI))
	if err := p.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.sess == nil || p.sess.markers.Load() == 0 {
		t.Fatal("SDK pause was not acknowledged")
	}
	done := p.sess.done
	p.Stop()
	if p.sess != nil {
		t.Fatal("session retained after stop")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CLI process left running")
	}
}

func TestNativePlayerStopKillsWedgedCLI(t *testing.T) {
	path := writeFakeCLI(t, `#!/bin/sh
echo "Connected to DirectiveSequencer"
exec sleep 30
`)
	p := newNativePlayer(path)
	if err := p.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := p.sess.done
	start := time.Now()
	p.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("wedged CLI survived stop")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("stop was not bounded")
	}
}

func TestNativePlayerRejectsMissingCLI(t *testing.T) {
	p := newNativePlayer(filepath.Join(t.TempDir(), "absent"))
	if err := p.Pause(context.Background()); err == nil {
		t.Fatal("missing CLI accepted")
	}
	p.Stop()
}
