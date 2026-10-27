package main

// Station 2 has no /sys/class/leds ring: its 89-LED ring is an 89x1 XRGB
// framebuffer. YAM has no framebuffer LED backend yet, so the AirPlay
// supervisor paints the same solid frames the previous package wrote. The
// backend is gated on the exact YAM model and framebuffer geometry; every
// sysfs model keeps using YAM's /led API. Frames are painted only while
// Alice is idle (the controls state machine owns that gate).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	station2FBPath = "/dev/fb0"
	station2FBSize = "/sys/class/graphics/fb0/virtual_size"
	station2LEDs   = 89
)

type framebufferLED struct {
	mu   sync.Mutex
	path string
}

func openStation2LED() (*framebufferLED, error) {
	return openFramebufferLED(station2FBPath, station2FBSize)
}

// openFramebufferLED verifies the exact Station 2 geometry before exposing the
// device. Tests substitute temporary paths.
func openFramebufferLED(path, sizePath string) (*framebufferLED, error) {
	size, err := os.ReadFile(sizePath)
	if err != nil {
		return nil, err
	}
	geometry := strings.TrimSpace(string(size))
	if geometry != "89,1" {
		return nil, fmt.Errorf("unexpected Station 2 framebuffer geometry %q", geometry)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return &framebufferLED{path: path}, nil
}

// station2Frame reproduces the old package's solid ring frames: a bright
// magenta while playing, a dim one while paused, and black when released.
// The framebuffer is 4 bytes per LED and the old frames had R == B.
func station2Frame(level uint8, state string) []byte {
	value, alpha := level, byte(0xff)
	if state == "paused" {
		value = uint8(uint16(level) * 70 / 203)
	}
	if state == "off" {
		value, alpha = 0, 0
	}
	frame := make([]byte, station2LEDs*4)
	for i := 0; i < station2LEDs; i++ {
		frame[i*4], frame[i*4+1], frame[i*4+2], frame[i*4+3] = value, 0, value, alpha
	}
	return frame
}

func (f *framebufferLED) Show(_ context.Context, state string, level uint8) error {
	switch state {
	case "playing", "paused", "off":
	default:
		return errors.New("unsupported framebuffer indication state")
	}
	if level == 0 {
		level = 200
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.OpenFile(f.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	frame := station2Frame(level, state)
	for len(frame) > 0 {
		n, err := file.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("short Station 2 framebuffer write")
		}
		frame = frame[n:]
	}
	return nil
}

func (f *framebufferLED) Off(ctx context.Context) error { return f.Show(ctx, "off", 0) }

// framebufferStation keeps YAM volume/speech and replaces only the ring.
type framebufferStation struct {
	yamControlStation
	fb *framebufferLED
}

func (s framebufferStation) Indicate(ctx context.Context, state string, level uint8) error {
	return s.fb.Show(ctx, state, level)
}

func (s framebufferStation) Off(ctx context.Context) error { return s.fb.Off(ctx) }
