package main

// Station volume through Luna's own HTTP API, used by the shared controls.
//
// Luna already owns the station volume: it prefers the maind/VQE volume manager
// and falls back to ALSA. Writing through /api/volume therefore needs neither
// the station-volume YAM package nor a runtime dependency alias. The sender's
// slider keeps the same contract as before: write, then confirm the value that
// actually reached the station.
//
// The inherited runtime URL points at /api/runtime/<id>; /api/volume belongs to
// the server, so the client is rebuilt against the server root. A volume write
// may wait for maind or amixer, which is slower than the 1.2 s runtime budget,
// especially on a cold boot or under emulation.

import (
	"context"
	"errors"
	"time"
)

type lunaControlVolume struct {
	api *yamAPI
}

func newLunaControlVolume(api *yamAPI) (*lunaControlVolume, error) {
	if api == nil {
		return nil, errors.New("Luna volume requires the runtime API")
	}
	root, err := lunaServerRoot(api.base)
	if err != nil {
		return nil, err
	}
	client := *api.client
	client.Timeout = 6 * time.Second
	server := *api
	server.base = root
	server.client = &client
	return &lunaControlVolume{api: &server}, nil
}

func (v *lunaControlVolume) Backend() string { return "Luna /api/volume" }

type lunaVolumeState struct {
	Percent int `json:"percent"`
}

func (v *lunaControlVolume) read(ctx context.Context) (int, error) {
	var state lunaVolumeState
	if err := v.api.requestContext(ctx, "GET", "/api/volume", nil, &state); err != nil {
		return 0, err
	}
	if state.Percent < 0 || state.Percent > 100 {
		return 0, errors.New("Luna returned an invalid station volume")
	}
	return state.Percent, nil
}

// CheckWrite replaces the station-volume firmware allowlist. That guard existed
// because the package wrote typed protobuf frames straight into the Quasar
// volume port on a short list of verified builds. Luna performs the write on
// every board it supports, through the vendor volume path, so the preflight that
// matters is that the endpoint is answering before the receiver starts.
func (v *lunaControlVolume) CheckWrite(ctx context.Context) error {
	_, err := v.read(ctx)
	return err
}

func (v *lunaControlVolume) GetVolume(ctx context.Context) (float64, error) {
	percent, err := v.read(ctx)
	if err != nil {
		return 0, err
	}
	return float64(percent), nil
}

func (v *lunaControlVolume) SetVolume(ctx context.Context, pct int) (float64, error) {
	if pct < 0 || pct > 100 {
		return 0, errors.New("volume must be 0..100")
	}
	var applied lunaVolumeState
	if err := v.api.requestContext(ctx, "POST", "/api/volume/set", map[string]int{"percent": pct}, &applied); err != nil {
		return 0, err
	}
	actual, err := v.GetVolume(ctx)
	if err != nil {
		return 0, err
	}
	// Luna applies the station's own step count (the emulator exposes 32 ALSA
	// steps, so 47 becomes 50). Accept the hardware grid but still detect a
	// write that never landed.
	if diff := actual - float64(pct); diff > 5 || diff < -5 {
		return 0, errors.New("Luna volume readback did not match")
	}
	return actual, nil
}
