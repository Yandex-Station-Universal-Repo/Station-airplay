package main

// Optional Alice button shortcut. While an AirPlay sender owns the session a
// short press of the station's Alice button pauses or resumes that sender. The
// stock Quasar handler is not grabbed or remapped: the keypad is only read, and
// presses that are long enough to start a conversation are ignored here so the
// firmware keeps its own long-press behaviour. Without an AirPlay session the
// reader never acts, so the button stays a normal Alice button.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	// Mini 2's Alice key as reported by the stock SAR keypad.
	aliceButtonKey = 582
	// The stock keypad node is named after the SAR pad; an event number is not
	// stable enough to hard-code.
	aliceButtonDeviceName = "sar0_pad0"
	aliceButtonShortMin   = 30 * time.Millisecond
	aliceButtonShortMax   = 600 * time.Millisecond
	aliceButtonPauseHold  = 5 * time.Second
	aliceButtonPauseEvery = 200 * time.Millisecond
)

// input_event is 8 bytes of timeval plus type/code/value. The supervisor is
// built for 32-bit ARM, but deriving the size keeps the decoder honest if it is
// ever run on a 64-bit test host.
var (
	inputEventSize = int(unsafe.Sizeof(syscall.Timeval{})) + 8
	inputTypeAt    = inputEventSize - 8
	inputCodeAt    = inputEventSize - 6
	inputValueAt   = inputEventSize - 4
)

// findAliceButtonEvent locates the keypad by device name and returns its event
// node name (for example "event0"). A missing node is an error, never a silent
// fallback to an unrelated input device.
func findAliceButtonEvent(sysRoot string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(sysRoot, "event*", "device", "name"))
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) != aliceButtonDeviceName {
			continue
		}
		return filepath.Base(filepath.Dir(filepath.Dir(path))), nil
	}
	return "", fmt.Errorf("input device %q not found", aliceButtonDeviceName)
}

// decodeAliceButtonEvents turns the keypad stream into short-press signals.
// Presses shorter than aliceButtonShortMin (bounce) and presses long enough to
// be a deliberate hold are dropped. Other keys, including volume and mute, are
// only skipped; the firmware still handles them.
func decodeAliceButtonEvents(r io.Reader, out chan<- struct{}, now func() time.Time) error {
	buf := make([]byte, inputEventSize)
	var pressedAt time.Time
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}
		if binary.LittleEndian.Uint16(buf[inputTypeAt:]) != 1 { // EV_KEY
			continue
		}
		if binary.LittleEndian.Uint16(buf[inputCodeAt:]) != aliceButtonKey {
			continue
		}
		switch int32(binary.LittleEndian.Uint32(buf[inputValueAt:])) {
		case 1: // press
			pressedAt = now()
		case 0: // release
			if pressedAt.IsZero() {
				continue
			}
			held := now().Sub(pressedAt)
			pressedAt = time.Time{}
			if held < aliceButtonShortMin || held > aliceButtonShortMax {
				continue
			}
			select {
			case out <- struct{}{}:
			default: // a pending press is already waiting for the control loop
			}
		}
	}
}

// watchAliceButton keeps the read-only keypad watcher alive across transient
// device errors. It never grabs the device, so Quasar keeps every native
// action and the long-press conversation path.
func watchAliceButton(ctx context.Context, path string, out chan<- struct{}) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := readAliceButton(ctx, path, out)
		if ctx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(os.Stderr, "station-airplay: Alice button input unavailable: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// pollReader turns a non-blocking evdev descriptor into a reader that stops on
// context cancellation. Closing the descriptor from another goroutine would not
// reliably wake a blocked read on Linux, so the reader polls instead.
type pollReader struct {
	f   *os.File
	ctx context.Context
}

func (p *pollReader) Read(b []byte) (int, error) {
	for {
		n, err := p.f.Read(b)
		if err == nil {
			return n, nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return n, err
		}
		select {
		case <-p.ctx.Done():
			return 0, io.EOF
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func readAliceButton(ctx context.Context, path string, out chan<- struct{}) error {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	err = decodeAliceButtonEvents(&pollReader{f: f, ctx: ctx}, out, time.Now)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
