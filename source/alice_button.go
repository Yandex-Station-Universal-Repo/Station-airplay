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
	// The Alice key as reported by every qualified stock keypad: Mini 2
	// (sar0_pad0), Lite 2 (aw933xx/aw9610x) and Station 2 (midi_touch).
	aliceButtonKey = 582
	// EVIOCGBIT(EV_KEY) returns a bitmap of KEY_CNT (768) bits.
	evKey          = 1
	keyBitmapBytes = 96

	aliceButtonShortMin   = 30 * time.Millisecond
	aliceButtonShortMax   = 600 * time.Millisecond
	aliceButtonPauseHold  = 8 * time.Second
	aliceButtonPauseEvery = 200 * time.Millisecond
)

// aliceButtonModels lists the models whose stock keypad reports the Alice key
// as KEY_ALICE and which have been mapped for the AirPlay shortcut. Discovery
// itself is capability-based; this list only decides whether the option is
// allowed on the device at all.
var aliceButtonModels = map[string]bool{
	"yandexmini_2":    true,
	"yandexplum":      true,
	"yandexstation_2": true,
}

// input_event is 8 bytes of timeval plus type/code/value. The supervisor is
// built for 32-bit ARM, but deriving the size keeps the decoder honest if it is
// ever run on a 64-bit test host.
var (
	inputEventSize = int(unsafe.Sizeof(syscall.Timeval{})) + 8
	inputTypeAt    = inputEventSize - 8
	inputCodeAt    = inputEventSize - 6
	inputValueAt   = inputEventSize - 4
)

// findAliceButtonEvent locates the keypad by asking evdev which input node
// reports the Alice key and returns its event node name (for example
// "event2"). Device names differ across models and event numbers are not
// stable, so the capability bitmap is the only reliable selector. No matching
// node, or more than one, is an error: the shortcut must never read an
// unrelated input device.
func findAliceButtonEvent(devRoot string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(devRoot, "event*"))
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	var found []string
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		ok, err := inputDeviceHasKey(path, aliceButtonKey)
		if err != nil || !ok {
			continue
		}
		found = append(found, filepath.Base(path))
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no input device reports key %d", aliceButtonKey)
	default:
		return "", fmt.Errorf("multiple input devices report key %d: %s", aliceButtonKey, strings.Join(found, ", "))
	}
}

// inputDeviceHasKey is indirected so capability discovery can be unit-tested
// without real evdev nodes.
var inputDeviceHasKey = probeInputDeviceKey

// probeInputDeviceKey asks evdev for the device's key bitmap. The descriptor is
// opened read-only and never grabbed.
func probeInputDeviceKey(path string, code int) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	bitmap := make([]byte, keyBitmapBytes)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), evIOCGBit(evKey, len(bitmap)), uintptr(unsafe.Pointer(&bitmap[0]))); errno != 0 {
		return false, errno
	}
	return keyBitmapHas(bitmap, code), nil
}

// evIOCGBit builds the EVIOCGBIT(ev, len) request number.
func evIOCGBit(ev, length int) uintptr {
	const (
		iocRead = 2
		iocType = 'E'
	)
	return uintptr(iocRead)<<30 | uintptr(length)<<16 | uintptr(iocType)<<8 | uintptr(0x20+ev)
}

// keyBitmapHas reports whether code is set in an EVIOCGBIT(EV_KEY) bitmap.
func keyBitmapHas(bitmap []byte, code int) bool {
	if code < 0 || code/8 >= len(bitmap) {
		return false
	}
	return bitmap[code/8]&(1<<uint(code%8)) != 0
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
