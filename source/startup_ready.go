package main

import (
	"context"
	"fmt"
	"time"
)

const sharedStartupTimeout = 60 * time.Second
const sharedStartupInterval = 100 * time.Millisecond

// Each probe must respect ctx so a slow dependency cannot exceed the total budget.
func waitSharedStartup(ctx context.Context, stage string, timeout, interval time.Duration, exits <-chan exitInfo, check func(context.Context) error) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last error
	failure := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s startup timed out after %s: %w", stage, timeout, last)
	}
	for {
		select {
		case <-probeCtx.Done():
			return failure()
		case ex := <-exits:
			return fmt.Errorf("%s: %s exited during startup: %v", stage, ex.name, ex.err)
		default:
		}
		last = check(probeCtx)
		// Cancellation and child exits take precedence even if a probe just succeeded.
		select {
		case <-probeCtx.Done():
			return failure()
		case ex := <-exits:
			return fmt.Errorf("%s: %s exited during startup: %v", stage, ex.name, ex.err)
		default:
		}
		if last == nil {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			return failure()
		case ex := <-exits:
			timer.Stop()
			return fmt.Errorf("%s: %s exited during startup: %v", stage, ex.name, ex.err)
		case <-timer.C:
		}
	}
}
