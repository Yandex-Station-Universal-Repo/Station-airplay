package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mockPauser struct {
	pauses atomic.Int64
	nows   atomic.Int64
	stops  atomic.Int64
}

func (m *mockPauser) Pause(context.Context) error { m.pauses.Add(1); return nil }
func (m *mockPauser) PauseNow() error             { m.nows.Add(1); return nil }
func (m *mockPauser) Stop()                       { m.stops.Add(1) }

func (m *mockPauser) pauseCount() int64    { return m.pauses.Load() }
func (m *mockPauser) pauseNowCount() int64 { return m.nows.Load() }
func (m *mockPauser) stopCount() int64     { return m.stops.Load() }

// A voice "continue"/"next" must not leave Alice's own player running next to
// AirPlay: arm a bounded handover, pause only while she is freshly idle, and
// tear the SDK session down when the window ends.
func TestNativeHandoverFollowsVoiceTransport(t *testing.T) {
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	p := &mockPauser{}
	c.native = p
	ctx := context.Background()
	now := time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	baselineStops := p.stopCount()
	// Idle snapshot first, then the final recognition that carries the phrase.
	_ = c.speech(ctx, speechEvent{Phase: "idle", RequestID: "s1"}, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "s1", Final: true, Transcript: "продолжи"}, now)
	if len(r.commands) != 1 || r.commands[0] != "Play" {
		t.Fatalf("voice transport not sent: %v", r.commands)
	}
	if c.nativeArmUntil.IsZero() {
		t.Fatal("handover not armed")
	}
	// Alice is busy again: never touch her player.
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.pauseCount() != 0 {
		t.Fatal("paused native player while Alice was busy")
	}
	// She is idle now, so the handover pauses the resumed native music.
	c.aliceIdle = true
	c.speechFreshUntil = now.Add(time.Minute)
	if err := c.tick(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.pauseCount() != 1 {
		t.Fatalf("handover did not pause native player: %d", p.pauseCount())
	}
	// Keep catching a late resume, but rate-limited.
	_ = c.tick(ctx, now.Add(2100*time.Millisecond))
	if p.pauseCount() != 1 {
		t.Fatal("handover was not rate-limited")
	}
	_ = c.tick(ctx, now.Add(3*time.Second))
	if p.pauseCount() != 2 {
		t.Fatal("late native resume was not caught")
	}
	// Expiry ends the session; no stray CLI and no further input.
	_ = c.tick(ctx, now.Add(15*time.Second))
	if p.stopCount() <= baselineStops {
		t.Fatal("handover session not released")
	}
	if !c.nativeArmUntil.IsZero() {
		t.Fatal("handover window not cleared")
	}
	_ = c.tick(ctx, now.Add(16*time.Second))
	if p.pauseCount() != 2 {
		t.Fatal("handover ran after expiry")
	}
}

// Alice can start her own player several seconds after the utterance. The
// bounded handover must still pause it even when the speech event is no
// longer fresh, because aliceIdle already proves she is not talking.
func TestNativeHandoverCatchesLateResume(t *testing.T) {
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	p := &mockPauser{}
	c.native = p
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "idle", RequestID: "late"}, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "late", Final: true, Transcript: "дальше"}, now)
	if len(r.commands) != 1 || r.commands[0] != "Next" {
		t.Fatalf("voice transport not sent: %v", r.commands)
	}
	c.speechFreshUntil = now.Add(time.Second)
	c.aliceIdle = true
	if err := c.tick(ctx, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.pauseCount() != 1 {
		t.Fatalf("late native resume was not paused: %d", p.pauseCount())
	}
}

func TestNativeHandoverIgnoresPauseCommand(t *testing.T) {
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	p := &mockPauser{}
	c.native = p
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "p1", Final: true, Transcript: "стоп"}, now)
	if len(r.commands) != 1 || r.commands[0] != "Pause" {
		t.Fatalf("pause not sent: %v", r.commands)
	}
	if !c.nativeArmUntil.IsZero() {
		t.Fatal("pause must not arm native handover")
	}
	_ = c.tick(ctx, now.Add(time.Second))
	if p.pauseCount() != 0 {
		t.Fatal("native player paused by a stop command")
	}
}

func TestNativeHandoverReleasedOnSenderChange(t *testing.T) {
	c, r, _ := testControls()
	c.cfg.NativePlayer = true
	p := &mockPauser{}
	c.native = p
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "n1", Final: true, Transcript: "следующий трек"}, now)
	if c.nativeArmUntil.IsZero() {
		t.Fatal("next did not arm handover")
	}
	// Let the handover open its SDK session, then change the sender.
	c.aliceIdle = true
	c.speechFreshUntil = now.Add(time.Minute)
	_ = c.tick(ctx, now.Add(time.Second))
	if p.pauseCount() != 1 {
		t.Fatalf("handover did not start: pauses=%d", p.pauseCount())
	}
	r.s.Client = "other-sender"
	_ = c.tick(ctx, now.Add(2*time.Second))
	if !c.nativeArmUntil.IsZero() {
		t.Fatal("handover survived sender change")
	}
	if p.stopCount() == 0 {
		t.Fatal("session not stopped on sender change")
	}
}
