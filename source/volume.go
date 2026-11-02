package main

// Station volume behind the sender's AirPlay slider.
//
// On models where Alice keeps running (Midi), the station owns the canonical
// volume: buttons, voice and the app change it, and it already scales
// everything played through the "speakers" softvol. So shairport-sync plays at
// 0 dB (ignore_volume_control) and the sender's AirPlay slider is forwarded to
// the station volume, exactly like a button press. The station-side changes are
// reported back, which the clock and ring show as the volume overlay. The
// sender's own slider is not moved back: that would need AirPlay remote
// control, which this Shairport build lacks.
//
// Two transports are available:
//   - Luna's /api/volume (preferred): Luna already owns the volume and falls
//     back to ALSA itself, so no YAM package is needed. There is no change
//     stream, so station-side changes are polled during a session.
//   - the station-volume YAM package: the legacy native wire protocol with its
//     per-firmware write guard, kept for hosts without the Luna runtime API.

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// nativeVolume is the station-volume transport behind the sender's slider.
// Polling reports station-side changes so the receiver can show the overlay.
type nativeVolume interface {
	start() error
	set(percent int)
	poll(active bool, show func(int))
	stop()
	backend() string
}

type stationVolume struct {
	script   string
	watch    *exec.Cmd
	updates  chan int
	requests chan int
	cancel   context.CancelFunc
}

// newStationVolume returns the station-volume transport, or nil when the
// station volume is not in charge: the model needs Alice stopped, Alice is not
// running, or no transport is available. Then shairport-sync applies the
// sender's volume itself, as before.
func newStationVolume(root string) nativeVolume {
	if !hw.quasarCoexist || !quasarRunning() {
		return nil
	}
	if v, e := newLunaVolume(); e == nil {
		return v
	}
	script := filepath.Join(filepath.Dir(root), "station-volume", "station-volume.sh")
	if _, e := os.Stat(script); e != nil {
		fmt.Println("station-volume package not installed and Luna volume API unavailable: the sender's slider uses shairport-sync's own volume")
		return nil
	}
	return &stationVolume{script: script, updates: make(chan int, 16), requests: make(chan int, 1)}
}

func (v *stationVolume) backend() string { return "the station-volume package" }

func (v *stationVolume) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.watch = exec.CommandContext(ctx, "sh", v.script, "watch")
	v.watch.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	v.watch.Stderr = os.Stderr
	out, e := v.watch.StdoutPipe()
	if e != nil {
		cancel()
		return e
	}
	if e = v.watch.Start(); e != nil {
		cancel()
		return fmt.Errorf("station-volume watch: %w", e)
	}
	go func() {
		lines := bufio.NewScanner(out)
		for lines.Scan() {
			if p, ok := parsePercent(lines.Text()); ok {
				select {
				case v.updates <- p:
				default: // UI is behind; the next update carries the level anyway
				}
			}
		}
		_ = v.watch.Wait()
	}()
	go func() {
		for p := range v.requests {
			setCtx, done := context.WithTimeout(ctx, 6*time.Second)
			cmd := exec.CommandContext(setCtx, "sh", v.script, "set", strconv.Itoa(p))
			if msg, e := cmd.CombinedOutput(); e != nil {
				fmt.Fprintf(os.Stderr, "station-volume set %d: %v %s\n", p, e, strings.TrimSpace(string(msg)))
			}
			done()
		}
	}()
	return nil
}

// set forwards the sender's volume; only the latest pending value is kept.
func (v *stationVolume) set(percent int) {
	select {
	case <-v.requests:
	default:
	}
	v.requests <- percent
}

// poll hands station-side volume changes to show without blocking.
func (v *stationVolume) poll(_ bool, show func(int)) {
	for {
		select {
		case p := <-v.updates:
			show(p)
		default:
			return
		}
	}
}

func (v *stationVolume) stop() {
	close(v.requests)
	v.cancel()
}

func parsePercent(line string) (int, bool) {
	f, e := strconv.ParseFloat(strings.TrimSpace(line), 64)
	if e != nil || math.IsNaN(f) || f < 0 || f > 100 {
		return 0, false
	}
	return int(math.Round(f)), true
}
