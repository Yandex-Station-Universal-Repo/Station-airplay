package main

import (
	"context"
	"testing"
	"time"
)

func TestUnavailableFeedbackDoesNotStarveIndication(t *testing.T) {
	c, r, s := testControls()
	ctx := context.Background()
	now := time.Now()
	r.s.Available = false
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	s.v = 35
	_ = c.tick(ctx, now.Add(time.Second))
	if len(s.frames) != 1 {
		t.Fatal("native overlay was overwritten immediately")
	}
	// The native delta remains pending because the phone has no DACP endpoint.
	if err := c.tick(ctx, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(s.frames) != 2 {
		t.Fatal("unavailable reverse volume starved LED refresh")
	}
	if len(r.volumes) != 0 {
		t.Fatal("sent volume to unavailable sender")
	}
	_ = c.speech(ctx, speechEvent{Phase: "speaking"}, now.Add(6*time.Second))
	_ = c.tick(ctx, now.Add(7*time.Second))
	if len(s.frames) != 2 || s.off != 0 {
		t.Fatal("painted over Alice")
	}
}

func TestIndicationRefreshRequiresFreshIdle(t *testing.T) {
	c, _, s := testControls()
	ctx := context.Background()
	now := time.Now()
	c.speechFreshUntil = now.Add(2 * time.Second)
	_ = c.tick(ctx, now)
	_ = c.tick(ctx, now.Add(4*time.Second))
	if len(s.frames) != 1 {
		t.Fatal("stale idle allowed a refresh")
	}
	_ = c.speech(ctx, speechEvent{Phase: "idle"}, now.Add(5*time.Second))
	_ = c.tick(ctx, now.Add(5*time.Second))
	if len(s.frames) != 2 {
		t.Fatal("fresh idle did not restore indication")
	}
}

func TestIndicationBrightnessIsOptIn(t *testing.T) {
	for _, test := range []struct {
		level uint8
		want  string
	}{{0, "#001008"}, {64, "#004020"}, {255, "#00ff7f"}} {
		c, _, s := testControls()
		c.cfg.LEDLevel = test.level
		if err := c.tick(context.Background(), time.Now()); err != nil {
			t.Fatal(err)
		}
		if len(s.frames) != 1 || s.frames[0][0] != test.want {
			t.Fatal(test, s.frames)
		}
	}
}
