package main

import (
	"context"
	"testing"
	"time"
)

func TestConnectionBeforePlaybackStartsVolumeAndIndication(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	r.s.Active, r.s.Connected, r.s.DB = false, true, percentDB(90)
	s.v = 25
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if !c.active || len(r.volumes) != 1 || r.volumes[0] != percentDB(25) || len(s.frames) != 1 || len(s.sets) != 0 {
		t.Fatalf("connection not handled before audio: active=%v volume=%v frames=%d native=%v", c.active, r.volumes, len(s.frames), s.sets)
	}
	r.s.DB = percentDB(25)
	r.s.Active = true
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(r.volumes) != 1 {
		t.Fatal("audio start repeated the connection handoff")
	}
	r.s.Active, r.s.Connected, r.s.Available, r.s.Client = false, false, false, ""
	_ = c.tick(ctx, now.Add(2*time.Second))
	if c.active || s.off != 1 {
		t.Fatal("indication survived actual disconnect")
	}
}

func TestRetainedClassicMetadataDoesNotInventConnectedSession(t *testing.T) {
	c, r, s := testControls()
	r.s.Active, r.s.Connected = false, false
	if err := c.tick(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if c.active || len(r.volumes) != 0 || len(s.frames) != 0 {
		t.Fatal("retained metadata treated as attachment")
	}
}

func TestConnectedFramebufferUsesDimFrameBeforeAudio(t *testing.T) {
	c, r, s := testControls()
	c.cfg.NativeVolume = false
	ind := &mockIndicationStation{mockStation: s}
	c.station = ind
	r.s.Active, r.s.Connected = false, true
	ctx, now := context.Background(), time.Now()
	_ = c.tick(ctx, now)
	if len(ind.states) != 1 || ind.states[0] != "paused" {
		t.Fatal(ind.states)
	}
	r.s.Active = true
	_ = c.tick(ctx, now.Add(time.Second))
	if len(ind.states) != 2 || ind.states[1] != "playing" {
		t.Fatal(ind.states)
	}
}

type busyFramebufferStation struct{ *mockStation }

func (s busyFramebufferStation) Indicate(context.Context, string, uint8) error {
	return errFramebufferBusy
}

func TestForeignFramebufferYieldIsNotAnErrorOrBlank(t *testing.T) {
	c, r, s := testControls()
	c.station = busyFramebufferStation{s}
	c.ledOwned = true
	c.ledRefreshAt = time.Time{}
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal("cooperative LED yield surfaced as transport error", err)
	}
	if c.ledOwned || s.off != 0 {
		t.Fatal("claimed/blanked a foreign framebuffer")
	}
	r.s.Active, r.s.Available, r.s.Connected, r.s.Client = false, false, false, ""
	_ = c.tick(ctx, now.Add(time.Second))
	c.close(ctx)
	if s.off != 0 {
		t.Fatal("disconnect blanked vendor frame after yield")
	}
}
