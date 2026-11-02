package main

// Station volume through Luna's own HTTP API.
//
// Luna already owns the station volume: it prefers the maind/VQE volume
// manager and falls back to ALSA. Using /api/volume therefore needs neither the
// station-volume YAM package nor a Luna dependency alias, and it keeps working
// when that package is disabled or missing.
//
// The API has no change stream, so station-side changes (buttons, voice, the
// app) are polled once per second while an AirPlay session is active. Polling
// is triggered from the UI tick and runs in its own goroutine: the ticker that
// paints the ring never waits for HTTP.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const lunaVolumePollInterval = time.Second

type lunaVolume struct {
	api      *yamAPI
	updates  chan int
	requests chan int
	cancel   context.CancelFunc

	mu       sync.Mutex
	lastPoll time.Time
	inFlight bool
}

// newLunaVolume builds the transport from the Luna runtime environment. The
// inherited API URL points at /api/runtime/<id>; the volume endpoint belongs to
// the server, not to the invocation.
func newLunaVolume() (*lunaVolume, error) {
	api, err := newYAMAPI()
	if err != nil {
		return nil, err
	}
	root, err := lunaServerRoot(api.base)
	if err != nil {
		return nil, err
	}
	api.base = root
	// The server runs the maind IPC and amixer before it answers. Under TCG and
	// on a cold boot that is slower than the 1.2 s runtime-API budget, and a
	// timed-out set still lands, so give the volume calls more room.
	client := *api.client
	client.Timeout = 6 * time.Second
	api.client = &client
	return &lunaVolume{
		api:      api,
		updates:  make(chan int, 16),
		requests: make(chan int, 1),
	}, nil
}

func lunaServerRoot(runtimeURL string) (string, error) {
	const marker = "/api/runtime/"
	i := strings.Index(runtimeURL, marker)
	if i <= 0 {
		return "", errors.New("runtime API URL is not a Luna invocation URL")
	}
	return strings.TrimRight(runtimeURL[:i], "/"), nil
}

func (v *lunaVolume) backend() string { return "Luna /api/volume" }

func (v *lunaVolume) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case percent := <-v.requests:
				step, done := context.WithTimeout(ctx, 5500*time.Millisecond)
				if err := v.api.requestContext(step, "POST", "/api/volume/set", map[string]int{"percent": percent}, nil); err != nil {
					fmt.Fprintf(os.Stderr, "station volume set %d via Luna: %v\n", percent, err)
				}
				done()
			}
		}
	}()
	return nil
}

// set forwards the sender's volume; only the latest pending value is kept.
func (v *lunaVolume) set(percent int) {
	select {
	case <-v.requests:
	default:
	}
	v.requests <- percent
}

// poll drains station-side changes and, during a session, asks Luna for the
// current level. A failed or slow request never blocks the caller.
func (v *lunaVolume) poll(active bool, show func(int)) {
	for {
		select {
		case percent := <-v.updates:
			show(percent)
		default:
			if !active {
				return
			}
			v.mu.Lock()
			due := !v.inFlight && time.Since(v.lastPoll) >= lunaVolumePollInterval
			if due {
				v.inFlight = true
				v.lastPoll = time.Now()
			}
			v.mu.Unlock()
			if !due {
				return
			}
			go v.refresh()
			return
		}
	}
}

func (v *lunaVolume) refresh() {
	defer func() {
		v.mu.Lock()
		v.inFlight = false
		v.mu.Unlock()
	}()
	ctx, done := context.WithTimeout(context.Background(), 4*time.Second)
	defer done()
	var response struct {
		Percent int `json:"percent"`
	}
	if err := v.api.requestContext(ctx, "GET", "/api/volume", nil, &response); err != nil {
		return
	}
	select {
	case v.updates <- response.Percent:
	default: // UI is behind; the next poll carries the level anyway
	}
}

func (v *lunaVolume) stop() {
	if v.cancel != nil {
		v.cancel()
	}
}
