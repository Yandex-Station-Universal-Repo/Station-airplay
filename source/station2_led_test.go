package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The old Station 2 package wrote 89 XRGB pixels: bright magenta 0xcb while
// playing, 0x46 while paused and black when released.
func TestStation2FrameMatchesOldRing(t *testing.T) {
	playing := station2Frame(203, "playing")
	if len(playing) != 89*4 {
		t.Fatalf("frame length %d", len(playing))
	}
	if !bytes.Equal(playing[:4], []byte{0xcb, 0x00, 0xcb, 0xff}) {
		t.Fatalf("playing pixel % x", playing[:4])
	}
	if !bytes.Equal(playing[88*4:], []byte{0xcb, 0x00, 0xcb, 0xff}) {
		t.Fatalf("last pixel % x", playing[88*4:])
	}
	paused := station2Frame(203, "paused")
	if !bytes.Equal(paused[:4], []byte{0x46, 0x00, 0x46, 0xff}) {
		t.Fatalf("paused pixel % x", paused[:4])
	}
	off := station2Frame(200, "off")
	if !bytes.Equal(off, make([]byte, 89*4)) {
		t.Fatal("off frame is not black")
	}
}

func TestStation2ShowPaintsWholeFrame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fb0")
	if err := os.WriteFile(path, make([]byte, 89*4), 0o600); err != nil {
		t.Fatal(err)
	}
	size := filepath.Join(dir, "virtual_size")
	if err := os.WriteFile(size, []byte("89,1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fb, err := openFramebufferLED(path, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fb.Close() })
	ctx := context.Background()
	if err := fb.Show(ctx, "playing", 200); err != nil {
		t.Fatal(err)
	}
	frame, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame, station2Frame(200, "playing")) {
		t.Fatal("playing frame not written")
	}
	if err = fb.Show(ctx, "paused", 200); err != nil {
		t.Fatal(err)
	}
	frame, _ = os.ReadFile(path)
	if !bytes.Equal(frame, station2Frame(200, "paused")) {
		t.Fatal("paused frame not written")
	}
	// A refresh must repaint even when the state did not change: Alice's
	// volume overlay can overwrite the ring between ticks.
	if err = fb.Off(ctx); err != nil {
		t.Fatal(err)
	}
	frame, _ = os.ReadFile(path)
	if !bytes.Equal(frame, make([]byte, 89*4)) {
		t.Fatal("off frame not written")
	}
	if err = fb.Show(ctx, "unknown", 200); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestStation2GeometryIsExact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fb0")
	if err := os.WriteFile(path, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	size := filepath.Join(dir, "virtual_size")
	for _, geometry := range []string{"8,1", "89,2", "89,1,extra", ""} {
		if err := os.WriteFile(size, []byte(geometry), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := openFramebufferLED(path, size); err == nil {
			t.Fatalf("geometry %q accepted", geometry)
		}
	}
	if err := os.WriteFile(size, []byte("89,1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fb, err := openFramebufferLED(path, size); err != nil {
		t.Fatal(err)
	} else {
		_ = fb.Close()
	}
}

func TestFramebufferDoesNotFightOrBlankForeignFrame(t *testing.T) {
	dir := t.TempDir()
	path, size := filepath.Join(dir, "fb0"), filepath.Join(dir, "virtual_size")
	_ = os.WriteFile(path, make([]byte, station2LEDs*4), 0600)
	_ = os.WriteFile(size, []byte("89,1"), 0600)
	fb, err := openFramebufferLED(path, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fb.Close() })
	now := time.Now()
	fb.now = func() time.Time { return now }
	ctx := context.Background()
	if err := fb.Show(ctx, "playing", 200); err != nil {
		t.Fatal(err)
	}
	foreign := bytes.Repeat([]byte{0, 90, 0, 0}, station2LEDs)
	_ = os.WriteFile(path, foreign, 0600)
	for i := 0; i < 3; i++ {
		now = now.Add(3 * time.Second)
		if err := fb.Show(ctx, "playing", 200); err != errFramebufferBusy {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		if !bytes.Equal(got, foreign) {
			t.Fatal("fought a vendor animation")
		}
	}
	if err := fb.Off(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, foreign) {
		t.Fatal("shutdown blanked a foreign frame")
	}
	_ = os.WriteFile(path, make([]byte, station2LEDs*4), 0600)
	if err := fb.Show(ctx, "playing", 200); err != errFramebufferBusy {
		t.Fatal("missing quiet window", err)
	}
	now = now.Add(2 * time.Second)
	if err := fb.Show(ctx, "playing", 200); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(got, station2Frame(200, "playing")) {
		t.Fatal("did not return after vendor released")
	}
}

func TestFramebufferForeignClearIsNotImmediatelyOverwritten(t *testing.T) {
	dir := t.TempDir()
	path, size := filepath.Join(dir, "fb0"), filepath.Join(dir, "virtual_size")
	_ = os.WriteFile(path, make([]byte, station2LEDs*4), 0600)
	_ = os.WriteFile(size, []byte("89,1"), 0600)
	fb, err := openFramebufferLED(path, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fb.Close() })
	now := time.Now()
	fb.now = func() time.Time { return now }
	ctx := context.Background()
	_ = fb.Show(ctx, "playing", 200)
	_ = os.WriteFile(path, make([]byte, station2LEDs*4), 0600)
	if err := fb.Show(ctx, "playing", 200); err != errFramebufferBusy {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, make([]byte, station2LEDs*4)) {
		t.Fatal("immediately raced a native clear")
	}
	now = now.Add(2 * time.Second)
	if err := fb.Show(ctx, "paused", 200); err != nil {
		t.Fatal(err)
	}
}

// The controls loop must drive the framebuffer station by state, not by the
// sysfs colour list, and it must clear the ring when the sender leaves.
func TestFramebufferIndicationFollowsPlaybackState(t *testing.T) {
	r := &mockReceiver{s: receiverState{Active: true, Available: true, Client: "mock-sender", DB: -15}}
	base := &mockStation{v: 70}
	ind := &mockIndicationStation{mockStation: base}
	c := &sharedController{
		cfg:      controlsConfig{Voice: true, LEDs: true, LEDLevel: 200},
		receiver: r,
		station:  ind,
		ledCount: 1,
	}
	c.aliceIdle = true
	c.speechFreshUntil = time.Now().Add(time.Hour)
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(ind.states) != 1 || ind.states[0] != "playing" || ind.levels[0] != 200 {
		t.Fatalf("playing indication %v %v", ind.states, ind.levels)
	}
	if len(base.frames) != 0 {
		t.Fatal("framebuffer station used the sysfs colour path")
	}
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "1", Final: true, Transcript: "стоп"}, now)
	_ = c.speech(ctx, speechEvent{Phase: "idle"}, now)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(ind.states) != 2 || ind.states[1] != "paused" {
		t.Fatalf("paused indication %v", ind.states)
	}
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "2", Final: true, Transcript: "продолжи"}, now.Add(2*time.Second))
	_ = c.speech(ctx, speechEvent{Phase: "idle"}, now.Add(2*time.Second))
	if err := c.tick(ctx, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(ind.states) != 3 || ind.states[2] != "playing" {
		t.Fatalf("resumed indication %v", ind.states)
	}
	r.s.Available = false
	r.s.Active = false
	r.s.Client = ""
	if err := c.tick(ctx, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if ind.off == 0 {
		t.Fatal("framebuffer ring not cleared after the sender left")
	}
}

type mockIndicationStation struct {
	*mockStation
	states []string
	levels []uint8
}

func (s *mockIndicationStation) Indicate(_ context.Context, state string, level uint8) error {
	s.states = append(s.states, state)
	s.levels = append(s.levels, level)
	return nil
}
