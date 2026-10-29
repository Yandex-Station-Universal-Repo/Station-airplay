package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func aliceInputEvent(kind, code uint16, value int32) []byte {
	buf := make([]byte, inputEventSize)
	binary.LittleEndian.PutUint16(buf[inputTypeAt:], kind)
	binary.LittleEndian.PutUint16(buf[inputCodeAt:], code)
	binary.LittleEndian.PutUint32(buf[inputValueAt:], uint32(value))
	return buf
}

func aliceClock(times ...time.Time) func() time.Time {
	queue := append([]time.Time(nil), times...)
	return func() time.Time {
		if len(queue) == 0 {
			return time.Time{}
		}
		value := queue[0]
		queue = queue[1:]
		return value
	}
}

func decodeAll(t *testing.T, data []byte, now func() time.Time) int {
	t.Helper()
	out := make(chan struct{}, 4)
	err := decodeAliceButtonEvents(bytes.NewReader(data), out, now)
	if err == nil {
		t.Fatal("expected end of stream")
	}
	return len(out)
}

func TestDecodeAliceButtonShortPress(t *testing.T) {
	start := time.Unix(100, 0)
	data := append(aliceInputEvent(1, aliceButtonKey, 1), aliceInputEvent(1, aliceButtonKey, 0)...)
	if got := decodeAll(t, data, aliceClock(start, start.Add(150*time.Millisecond))); got != 1 {
		t.Fatalf("short press signals = %d, want 1", got)
	}
}

func TestDecodeAliceButtonIgnoresHoldAndBounce(t *testing.T) {
	start := time.Unix(100, 0)
	hold := append(aliceInputEvent(1, aliceButtonKey, 1), aliceInputEvent(1, aliceButtonKey, 0)...)
	if got := decodeAll(t, hold, aliceClock(start, start.Add(time.Second))); got != 0 {
		t.Fatalf("long press signals = %d, want 0", got)
	}
	bounce := append(aliceInputEvent(1, aliceButtonKey, 1), aliceInputEvent(1, aliceButtonKey, 0)...)
	if got := decodeAll(t, bounce, aliceClock(start, start.Add(5*time.Millisecond))); got != 0 {
		t.Fatalf("bounce signals = %d, want 0", got)
	}
}

func TestDecodeAliceButtonIgnoresOtherKeysAndRepeats(t *testing.T) {
	start := time.Unix(100, 0)
	data := []byte{}
	data = append(data, aliceInputEvent(1, 114, 1)...) // volume down
	data = append(data, aliceInputEvent(1, 114, 0)...)
	data = append(data, aliceInputEvent(1, aliceButtonKey, 2)...) // autorepeat
	data = append(data, aliceInputEvent(2, aliceButtonKey, 1)...) // not EV_KEY
	if got := decodeAll(t, data, aliceClock(start)); got != 0 {
		t.Fatalf("unrelated input signals = %d, want 0", got)
	}
}

func TestDecodeAliceButtonReleaseWithoutPress(t *testing.T) {
	if got := decodeAll(t, aliceInputEvent(1, aliceButtonKey, 0), aliceClock(time.Unix(1, 0))); got != 0 {
		t.Fatalf("orphan release signals = %d, want 0", got)
	}
}

func TestKeyBitmapHas(t *testing.T) {
	bitmap := make([]byte, keyBitmapBytes)
	if keyBitmapHas(bitmap, aliceButtonKey) {
		t.Fatal("empty bitmap must not report the Alice key")
	}
	bitmap[aliceButtonKey/8] |= 1 << uint(aliceButtonKey%8)
	if !keyBitmapHas(bitmap, aliceButtonKey) {
		t.Fatal("set bit was not detected")
	}
	if keyBitmapHas(bitmap, aliceButtonKey+1) {
		t.Fatal("neighbouring bit reported as set")
	}
	if keyBitmapHas(bitmap, keyBitmapBytes*8) {
		t.Fatal("out-of-range key reported as set")
	}
}

func TestEVIOCGBitRequest(t *testing.T) {
	// EVIOCGBIT(EV_KEY, 96): read, type 'E', nr 0x21, size 96.
	if got := evIOCGBit(evKey, keyBitmapBytes); got != 0x80604521 {
		t.Fatalf("request = %#x, want 0x80604521", got)
	}
}

func TestFindAliceButtonEventUsesCapability(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"event0", "event1", "event2"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	original := inputDeviceHasKey
	defer func() { inputDeviceHasKey = original }()
	inputDeviceHasKey = func(path string, code int) (bool, error) {
		return filepath.Base(path) == "event1" && code == aliceButtonKey, nil
	}
	event, err := findAliceButtonEvent(root)
	if err != nil {
		t.Fatal(err)
	}
	if event != "event1" {
		t.Fatalf("event = %q, want event1", event)
	}
	if _, err = findAliceButtonEvent(t.TempDir()); err == nil {
		t.Fatal("missing keypad must be an error")
	}

	// Two keypads that both report the key are ambiguous and must be rejected.
	inputDeviceHasKey = func(string, int) (bool, error) { return true, nil }
	if _, err = findAliceButtonEvent(root); err == nil {
		t.Fatal("ambiguous keypad must be an error")
	}
}

func TestAliceButtonTogglesOnlyWithLiveSender(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	c.cfg.AliceButton = true
	p := &mockPauser{}
	c.native = p
	defer c.releaseNative()

	r.s = receiverState{Active: true, Available: true, Client: "sender", PlayerState: "Playing", DB: -15}
	if err := c.button(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || r.commands[0] != "Pause" {
		t.Fatalf("playing commands = %v, want [Pause]", r.commands)
	}

	r.commands = nil
	r.s = receiverState{Active: false, Available: true, Client: "sender", PlayerState: "Paused", DB: -15}
	if err := c.button(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 1 || r.commands[0] != "Play" {
		t.Fatalf("paused commands = %v, want [Play]", r.commands)
	}

	r.commands = nil
	r.s = receiverState{Available: false, DB: -15}
	if err := c.button(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 0 {
		t.Fatalf("idle commands = %v, want none", r.commands)
	}
	if p.pauseCount() != 2 {
		t.Fatalf("native pauses = %d, want 2 (no pause without a session)", p.pauseCount())
	}
}

func TestAliceButtonStaysOutOfConversation(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	c.cfg.AliceButton = true
	c.native = &mockPauser{}
	c.aliceIdle = false
	c.speechFreshUntil = now.Add(3 * time.Second)
	r.s = receiverState{Active: true, Available: true, Client: "sender", PlayerState: "Playing", DB: -15}
	if err := c.button(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 0 {
		t.Fatalf("commands during conversation = %v, want none", r.commands)
	}
}

func TestAliceButtonSuppressionSurvivesPause(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	c.cfg.AliceButton = true
	c.buttonCadence = 5 * time.Millisecond
	p := &mockPauser{}
	c.native = p
	defer c.releaseNative()
	r.s = receiverState{Active: true, Available: true, Client: "sender", PlayerState: "Playing", DB: -15}
	if err := c.button(ctx, now); err != nil {
		t.Fatal(err)
	}
	if p.pauseCount() != 1 || c.buttonSupp == nil {
		t.Fatalf("button pause not armed: pauses=%d supp=%v", p.pauseCount(), c.buttonSupp)
	}
	// The bounded cadence refreshes the native pause off the control loop.
	deadline := time.Now().Add(time.Second)
	for p.pauseNowCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if p.pauseNowCount() == 0 {
		t.Fatal("cadence did not refresh the native pause")
	}
	// The sender is paused now; the tick must keep the native player suppressed
	// instead of tearing the handover down as inactive.
	r.s.Active = false
	r.s.PlayerState = "Paused"
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.buttonSupp == nil || p.stopCount() != 0 {
		t.Fatalf("suppression lost: supp=%v stops=%d", c.buttonSupp, p.stopCount())
	}
	// Window expiry releases the cadence and the CLI session exactly once.
	if err := c.tick(ctx, now.Add(aliceButtonPauseHold+time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.buttonSupp != nil || p.stopCount() != 1 {
		t.Fatalf("handover not released after window: supp=%v stops=%d", c.buttonSupp, p.stopCount())
	}
	after := p.pauseNowCount()
	time.Sleep(20 * time.Millisecond)
	if p.pauseNowCount() != after {
		t.Fatal("cadence kept running after release")
	}
}
