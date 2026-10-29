package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInitialVolumeComesFromStationNotPhone(t *testing.T) {
	for _, native := range []float64{0, 5, 35, 70, 100} {
		c, r, s := testControls()
		ctx, now := context.Background(), time.Now()
		s.v, r.s.DB = native, percentDB(90)
		if err := c.tick(ctx, now); err != nil {
			t.Fatal(err)
		}
		if len(s.sets) != 0 || s.v != native || len(r.volumes) != 1 || r.volumes[0] != percentDB(int(native)) {
			t.Fatalf("native=%v: sets=%v remote=%v", native, s.sets, r.volumes)
		}
		if err := c.tick(ctx, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if len(s.sets) != 0 || len(r.volumes) != 1 {
			t.Fatal("stale phone metadata echoed into native volume")
		}
		r.s.DB = r.volumes[0]
		if err := c.tick(ctx, now.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
		if c.remotePending || len(s.sets) != 0 || len(r.volumes) != 1 {
			t.Fatal("acknowledgement caused volume feedback")
		}
	}
}

func TestVolumeHandoffWaitsForSenderControl(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	s.v, r.s.DB, r.s.Available = 35, percentDB(90), false
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if c.volumeReady || len(r.volumes) != 0 || len(s.sets) != 0 {
		t.Fatal("unavailable sender was synchronized")
	}
	// Alice's current level, not the value sampled before DACP became ready.
	s.v, r.s.Available = 40, true
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(r.volumes) != 1 || r.volumes[0] != percentDB(40) || len(s.sets) != 0 {
		t.Fatal("did not inherit the latest station volume")
	}
}

func TestGenuinePhoneChangeDuringHandoffWins(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	s.v, r.s.DB = 35, percentDB(90)
	_ = c.tick(ctx, now)
	r.s.DB = percentDB(60)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.remotePending || s.v != 60 || len(s.sets) != 1 {
		t.Fatal("genuine phone adjustment was blocked")
	}
}

func TestAirPlayEndKeepsLastNativeLevel(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	_ = c.tick(ctx, now)
	r.s.DB = percentDB(40)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.v != 40 || len(s.sets) != 1 {
		t.Fatal("phone adjustment did not reach Alice's native level")
	}
	r.s.Active, r.s.Available, r.s.Client = false, false, ""
	_ = c.tick(ctx, now.Add(2*time.Second))
	c.close(ctx)
	if s.v != 40 || len(s.sets) != 1 || c.remotePending {
		t.Fatal("AirPlay end restored old native volume")
	}
	// A later session starts at whatever Alice currently uses, not old history.
	s.v = 25
	r.s.Active, r.s.Available, r.s.Client, r.s.DB = true, true, "next-sender", percentDB(80)
	if err := c.tick(ctx, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if r.volumes[len(r.volumes)-1] != percentDB(25) || len(s.sets) != 1 {
		t.Fatal("new session did not inherit current native volume")
	}
}

type handoffFailReceiver struct{ *mockReceiver }

func (r handoffFailReceiver) Volume(context.Context, float64) error {
	return errors.New("sender refused volume")
}

func TestFailedInitialVolumeDoesNotWriteNative(t *testing.T) {
	c, r, s := testControls()
	s.v, r.s.DB = 35, percentDB(90)
	c.receiver = handoffFailReceiver{r}
	if err := c.tick(context.Background(), time.Now()); err == nil {
		t.Fatal("sender failure ignored")
	}
	if c.volumeReady || c.remotePending || len(s.sets) != 0 || s.v != 35 {
		t.Fatal("sender failure changed native volume or committed handoff")
	}
}

func TestInitialVolumeAckTimeoutDoesNotRaiseNative(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	s.v, r.s.DB = 35, percentDB(90)
	_ = c.tick(ctx, now)
	if err := c.tick(ctx, now.Add(4*time.Second)); err == nil {
		t.Fatal("missing acknowledgement accepted")
	}
	if len(s.sets) != 0 || s.v != 35 {
		t.Fatal("sender timeout changed Alice volume")
	}
}

func TestNativeVolumeDisabledDoesNotHandoff(t *testing.T) {
	c, r, s := testControls()
	c.cfg.NativeVolume = false
	s.v, r.s.DB = 35, percentDB(90)
	_ = c.tick(context.Background(), time.Now())
	if len(s.sets) != 0 || len(r.volumes) != 0 {
		t.Fatal("disabled native-volume feature wrote volume")
	}
}

func TestSenderChangeDropsPendingInitialVolume(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	s.v, r.s.DB = 35, percentDB(90)
	_ = c.tick(ctx, now)
	s.v = 45
	r.s.Client, r.s.DB = "new-sender", percentDB(80)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(r.volumes) != 2 || r.volumes[1] != percentDB(45) || len(s.sets) != 0 {
		t.Fatal("old sender's pending volume survived sender change")
	}
}

func TestPausedSessionDropsPendingVolumeBeforeResume(t *testing.T) {
	c, r, s := testControls()
	ctx, now := context.Background(), time.Now()
	s.v, r.s.DB = 35, percentDB(90)
	_ = c.tick(ctx, now)
	r.s.Active = false
	_ = c.tick(ctx, now.Add(time.Second))
	if c.remotePending || c.volumeReady {
		t.Fatal("inactive session retained pending volume")
	}
	s.v, r.s.Active = 40, true
	if err := c.tick(ctx, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(r.volumes) != 2 || r.volumes[1] != percentDB(40) || len(s.sets) != 0 {
		t.Fatal("resume did not inherit current native volume")
	}
}
