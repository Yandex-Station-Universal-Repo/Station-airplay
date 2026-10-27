package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The playback indication must disappear when AirPlay leaves, not ten seconds
// later: the active tail is shortened and a remembered pause is dropped as soon
// as the sender's control channel is gone.
func TestReceiverControlConfigUsesShortActiveTail(t *testing.T) {
	plain := receiverControlConfig(mini2Profile, nil)
	if strings.Contains(plain, "active_state_timeout") {
		t.Fatal("legacy receiver default changed")
	}
	config := receiverControlConfig(mini2Profile, &controlsConfig{LEDs: true})
	if !strings.Contains(config, "active_state_timeout = 1.0;") {
		t.Fatal("controls receiver keeps the ten-second active tail")
	}
}

func TestResumeClearsPausedWindow(t *testing.T) {
	c, _, _ := testControls()
	ctx, now := context.Background(), time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "1", Final: true, Transcript: "стоп"}, now)
	if c.pausedUntil.IsZero() {
		t.Fatal("pause did not open the resume window")
	}
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "2", Final: true, Transcript: "продолжи"}, now.Add(time.Second))
	if !c.pausedUntil.IsZero() {
		t.Fatal("resume left a stale paused window")
	}
}

func TestIndicationClearsWhenSenderLeaves(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	_ = c.tick(ctx, now)
	r.s.Active = false
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "1", Final: true, Transcript: "стоп"}, now)
	_ = c.speech(ctx, speechEvent{Phase: "idle"}, now)
	_ = c.tick(ctx, now.Add(time.Second))
	if len(s.frames) == 0 {
		t.Fatal("indication not shown during the paused session")
	}
	r.s.Available = false
	r.s.Client = ""
	_ = c.tick(ctx, now.Add(2*time.Second))
	if s.off == 0 {
		t.Fatal("indication not cleared after the sender left")
	}
	if !c.pausedUntil.IsZero() {
		t.Fatal("paused window survived the sender")
	}
}
