package main

import (
	"context"
	"testing"
	"time"
)

func TestPreviousAndRestartVoice(t *testing.T) {
	for _, tc := range []struct {
		phrase string
		count  int
	}{
		{"прошлый трек", 2}, {"предыдущий трек", 2},
		{"Алиса, прошлую песню", 2}, {"назад", 2},
		{"повтори", 1}, {"повтори трек", 1}, {"повтори песню", 1},
		{"сначала", 1}, {"начни сначала", 1},
	} {
		t.Run(tc.phrase, func(t *testing.T) {
			c, r, _ := testControls()
			ctx, now := context.Background(), time.Now()
			if err := c.tick(ctx, now); err != nil {
				t.Fatal(err)
			}
			e := speechEvent{Phase: "recognition", RequestID: "previous", Transcript: tc.phrase, Final: true}
			if err := c.speech(ctx, e, now); err != nil {
				t.Fatal(err)
			}
			if err := c.speech(ctx, e, now); err != nil {
				t.Fatal(err)
			}
			if len(r.commands) != tc.count {
				t.Fatalf("commands = %v, want %d (including deduplication)", r.commands, tc.count)
			}
			for _, command := range r.commands {
				if command != "Previous" {
					t.Fatal(r.commands)
				}
			}
		})
	}
}

func TestPreviousSecondPressGuards(t *testing.T) {
	for _, mode := range []string{"cancel", "changed", "disconnected", "offline"} {
		t.Run(mode, func(t *testing.T) {
			c, r, _ := testControls()
			c.client = r.s.Client
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "cancel":
				cancel()
			case "changed":
				r.s.Client = "other-sender"
			case "disconnected":
				r.s.Available = false
			case "offline":
				r.fail = true
			}
			if err := c.voiceCommand(ctx, "Previous"); err == nil {
				t.Fatal("missing second-press guard")
			}
			if len(r.commands) != 1 {
				t.Fatalf("unsafe second press: %v", r.commands)
			}
		})
	}
}
