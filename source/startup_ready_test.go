package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSharedStartupRetries(t *testing.T) {
	calls := 0
	err := waitSharedStartup(context.Background(), "receiver", time.Second, time.Millisecond, nil, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("not registered")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestSharedStartupTimeoutDetail(t *testing.T) {
	err := waitSharedStartup(context.Background(), "receiver D-Bus", 20*time.Millisecond, time.Millisecond, nil, func(context.Context) error { return errors.New("name absent") })
	if err == nil || !strings.Contains(err.Error(), "receiver D-Bus startup timed out") || !strings.Contains(err.Error(), "name absent") {
		t.Fatalf("err=%v", err)
	}
}

func TestSharedStartupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitSharedStartup(ctx, "receiver", time.Second, time.Millisecond, nil, func(context.Context) error { t.Fatal("probe after cancellation"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestSharedStartupChildExit(t *testing.T) {
	exits := make(chan exitInfo, 1)
	exits <- exitInfo{"avahi", errors.New("exit 1")}
	err := waitSharedStartup(context.Background(), "Avahi registration", time.Second, time.Millisecond, exits, func(context.Context) error { t.Fatal("probe after exit"); return nil })
	if err == nil || !strings.Contains(err.Error(), "avahi exited during startup") {
		t.Fatalf("err=%v", err)
	}
}

func TestSharedStartupBoundsSlowProbe(t *testing.T) {
	start := time.Now()
	err := waitSharedStartup(context.Background(), "slow probe", 20*time.Millisecond, time.Millisecond, nil, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
}
