package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
)

const receiverBusName = "org.gnome.ShairportSync"
const remoteInterface = receiverBusName + ".RemoteControl"

type dbusReceiver struct{ conn *dbus.Conn }

func openReceiver(bus string) (*dbusReceiver, error) {
	c, err := dbus.Connect("unix:path=" + bus)
	if err != nil {
		return nil, err
	}
	return &dbusReceiver{c}, nil
}
func (r *dbusReceiver) State(ctx context.Context) (receiverState, error) {
	o := r.conn.Object(receiverBusName, "/org/gnome/ShairportSync")
	get := func(iface string) (map[string]dbus.Variant, error) {
		var props map[string]dbus.Variant
		err := o.CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&props)
		return props, err
	}
	a, err := get(receiverBusName)
	if err != nil {
		return receiverState{}, err
	}
	b, err := get(remoteInterface)
	if err != nil {
		return receiverState{}, err
	}
	s := receiverState{}
	var ok bool
	if s.Active, ok = a["Active"].Value().(bool); !ok {
		return s, errors.New("missing receiver Active")
	}
	if s.Available, ok = b["Available"].Value().(bool); !ok {
		return s, errors.New("missing receiver remote Available")
	}
	if s.Client, ok = b["Client"].Value().(string); !ok {
		return s, errors.New("missing receiver Client")
	}
	if s.DB, ok = b["AirplayVolume"].Value().(float64); !ok {
		return s, errors.New("missing receiver AirplayVolume")
	}
	// PlayerState distinguishes a sender pause from an idle receiver. Older
	// builds may omit it; Active remains the fallback.
	if v, ok := b["PlayerState"].Value().(string); ok {
		s.PlayerState = v
	}
	// Always ask, even when classic DACP is available. Its retained Client and
	// Active metadata may lag the initial AirPlay 2 attachment until audio starts.
	// This method reports the principal live event connection and its own client,
	// so it is authoritative over stale/empty classic metadata.
	var version uint32
	var modern bool
	var client string
	err = o.CallWithContext(ctx, receiverBusName+".GetControlStatus", 0).Store(&version, &modern, &client)
	if err == nil && version == 1 && modern && client != "" {
		s.Connected = true
		s.Client = client
		s.Available = true
	}
	return s, nil
}
func (r *dbusReceiver) Command(ctx context.Context, command string) error {
	switch command {
	case "Pause", "Play", "Next", "Previous":
	default:
		return errors.New("unsupported remote method")
	}
	return r.conn.Object(receiverBusName, "/org/gnome/ShairportSync").CallWithContext(ctx, remoteInterface+"."+command, 0).Err
}
func (r *dbusReceiver) Volume(ctx context.Context, v float64) error {
	if !validDB(v) {
		return errors.New("invalid remote volume")
	}
	return r.conn.Object(receiverBusName, "/org/gnome/ShairportSync").CallWithContext(ctx, remoteInterface+".SetAirplayVolume", 0, v).Err
}
func (r *dbusReceiver) Gain(ctx context.Context, percent, lease uint32) error {
	if percent > 100 || lease > 5000 {
		return errors.New("invalid stream gain lease")
	}
	return r.conn.Object(receiverBusName, "/org/gnome/ShairportSync").CallWithContext(ctx, receiverBusName+".SetStreamGain", 0, percent, lease).Err
}

type yamControlStation struct {
	api    *yamAPI
	volume volumeControl
}

func (s yamControlStation) GetVolume(ctx context.Context) (float64, error) {
	if s.volume == nil {
		return 0, errors.New("volume dependency not enabled")
	}
	return s.volume.GetVolume(ctx)
}
func (s yamControlStation) SetVolume(ctx context.Context, v int) (float64, error) {
	if s.volume == nil {
		return 0, errors.New("volume dependency not enabled")
	}
	return s.volume.SetVolume(ctx, v)
}
func (s yamControlStation) Colors(ctx context.Context, v []string) error {
	return s.api.requestContext(ctx, "POST", "/led", map[string]any{"colors": v}, nil)
}
func (s yamControlStation) Off(ctx context.Context) error {
	return s.api.requestContext(ctx, "POST", "/led/off", map[string]any{}, nil)
}
func (s yamControlStation) ClaimSpeech(ctx context.Context, id string) error {
	return s.api.requestContext(ctx, "POST", "/command/cancel", map[string]any{"request_id": id}, nil)
}

func serveControls(ctx context.Context, bus string, api *yamAPI, d yamDevice, cfg controlsConfig) error {
	r, err := openReceiver(bus)
	if err != nil {
		return err
	}
	defer r.conn.Close()
	volume, err := controlsVolume(ctx, api, d, cfg)
	if err != nil {
		return err
	}
	station := controlStation(yamControlStation{api: api, volume: volume})
	ledCount := supportedLEDCount(d)
	if cfg.LEDs && d.Model == "yandexstation_2" {
		// Station 2 exposes an 89x1 framebuffer instead of a sysfs ring and YAM
		// has no framebuffer LED backend. Paint the ring here; a geometry
		// mismatch leaves indication off rather than writing an unknown frame.
		if fb, e := openStation2LED(); e == nil {
			station = framebufferStation{yamControlStation: yamControlStation{api: api, volume: volume}, fb: fb}
			ledCount = 1
			defer func() {
				stop, done := context.WithTimeout(context.Background(), time.Second)
				defer done()
				_ = fb.Off(stop)
				_ = fb.Close()
			}()
		} else {
			fmt.Fprintf(os.Stderr, "station-airplay: Station 2 indication unavailable: %v\n", e)
		}
	}
	c := &sharedController{cfg: cfg, receiver: r, station: station, ledCount: ledCount}
	if cfg.NativePlayer {
		c.native = newNativePlayer(quasarCLIPath)
	}
	buttonEvents := make(chan struct{}, 1)
	if cfg.AliceButton {
		event, e := findAliceButtonEvent("/dev/input")
		if e != nil {
			return e
		}
		go watchAliceButton(ctx, filepath.Join("/dev/input", event), buttonEvents)
	}
	groupEvents := make(chan groupMessage, 8)
	if cfg.Group != nil {
		if !cfg.NativeVolume {
			return errors.New("group volume requires native_volume")
		}
		p, ok := profileForDevice(d, true)
		if !ok {
			return errors.New("unknown group profile")
		}
		p, err = profileWithRoom(p, d.Room)
		if err != nil {
			return err
		}
		bridge, e := openGroup(*cfg.Group, p.name, r)
		if e != nil {
			return e
		}
		defer bridge.conn.Close()
		c.broadcast = bridge.broadcast
		done := make(chan struct{})
		go func() { defer close(done); bridge.listen(ctx, groupEvents) }()
		defer func() { <-done }()
		return runControlLoop(ctx, c, api, cfg, groupEvents, buttonEvents, bridge)
	}
	return runControlLoop(ctx, c, api, cfg, groupEvents, buttonEvents, nil)
}
func runControlLoop(ctx context.Context, c *sharedController, api *yamAPI, cfg controlsConfig, groupEvents <-chan groupMessage, buttonEvents <-chan struct{}, bridge *groupBridge) error {
	defer func() {
		stop, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		c.close(stop)
	}()
	events := make(chan speechEvent, 8)
	speechDone := make(chan struct{})
	if cfg.Voice || cfg.Ducking || cfg.LEDs {
		go func() {
			defer close(speechDone)
			for ctx.Err() == nil {
				var event *speechEvent
				err := api.requestContext(ctx, "POST", "/speech/next", map[string]any{}, &event)
				if err == nil && event != nil {
					select {
					case events <- *event:
					case <-ctx.Done():
						return
					}
				}
				if err != nil {
					select {
					case events <- speechEvent{Phase: "unavailable"}:
					case <-ctx.Done():
						return
					}
					select {
					case <-time.After(time.Second):
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	} else {
		close(speechDone)
	}
	defer func() { <-speechDone }()
	lastError := time.Time{}
	report := func(err error) {
		if err != nil && time.Since(lastError) > 30*time.Second {
			fmt.Println("shared controls: operation unavailable (details intentionally redacted)")
			aliceButtonDebug("control: operation failed: %v", err)
			lastError = time.Now()
		}
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case m := <-groupEvents:
			if bridge != nil {
				step, done := context.WithTimeout(ctx, 4*time.Second)
				report(c.group(step, bridge, m, time.Now()))
				done()
			}
		case e := <-events:
			step, done := context.WithTimeout(ctx, 4*time.Second)
			report(c.speech(step, e, time.Now()))
			done()
		case <-buttonEvents:
			step, done := context.WithTimeout(ctx, 4*time.Second)
			report(c.button(step, time.Now()))
			done()
		case now := <-tick.C:
			step, done := context.WithTimeout(ctx, 4*time.Second)
			report(c.tick(step, now))
			done()
		}
	}
}

// controlsVolume picks the station volume owner. Without an explicit backend the
// runtime API decides: a Luna invocation gets Luna's /api/volume, a legacy YAM
// host keeps the station-volume package. An explicit volume_dependency always
// means the package, so existing device configurations keep their behaviour.
func controlsVolume(ctx context.Context, api *yamAPI, d yamDevice, cfg controlsConfig) (volumeControl, error) {
	if !cfg.NativeVolume {
		return nil, nil
	}
	backend := cfg.VolumeBackend
	if backend == "" {
		if cfg.VolumeDependency != "" {
			backend = volumeBackendPackage
		} else if api != nil {
			if _, err := lunaServerRoot(api.base); err == nil {
				backend = volumeBackendLuna
			} else {
				backend = volumeBackendPackage
			}
		} else {
			backend = volumeBackendPackage
		}
	}
	switch backend {
	case volumeBackendLuna:
		// Some firmwares publish volume_manager.port but never listen on it.
		// Luna then retries the dead port and still falls back to ALSA, which
		// costs about two seconds per slider step. When the port is closed the
		// same ALSA write can be done directly; an explicit volume_backend
		// keeps Luna's path either way.
		if cfg.VolumeBackend == "" && !volumeManagerListening() {
			if luna, lerr := newLunaControlVolume(api); lerr == nil {
				if alsa, aerr := newALSAControlVolume(luna); aerr == nil {
					fmt.Printf("station volume: the sender's slider sets the station volume via %s\n", alsa.Backend())
					return alsa, nil
				}
			}
		}
		v, err := newLunaControlVolume(api)
		if err == nil {
			fmt.Printf("station volume: the sender's slider sets the station volume via %s\n", v.Backend())
		}
		return v, err
	case volumeBackendPackage:
		var v volumeControl
		var err error
		if cfg.VolumeDependency != "" {
			v, err = findRuntimePackageVolume(ctx, api, cfg.VolumeDependency, d)
		} else {
			exe, e := os.Executable()
			if e != nil {
				return nil, e
			}
			v, err = findPackageVolume(filepath.Dir(filepath.Dir(exe)), d)
		}
		if err == nil && v != nil {
			fmt.Printf("station volume: the sender's slider sets the station volume via %s\n", v.Backend())
		}
		return v, err
	default:
		return nil, errors.New("unknown volume backend")
	}
}
func controlsPreflight(ctx context.Context, api *yamAPI, d yamDevice, cfg controlsConfig) error {
	var caps struct {
		Version  int  `json:"version"`
		Speech   bool `json:"speech"`
		LED      bool `json:"led"`
		Commands bool `json:"commands"`
	}
	if cfg.Voice || cfg.Ducking || cfg.LEDs {
		if err := api.requestContext(ctx, "GET", "/media", nil, &caps); err != nil {
			return fmt.Errorf("speech-enabled YAM required: %w", err)
		}
		if caps.Version != 1 || !caps.Speech || cfg.Voice && !caps.Commands || cfg.LEDs && !caps.LED {
			return errors.New("missing YAM speech runtime capability/permission")
		}
	}
	if cfg.NativePlayer {
		if !cfg.Voice {
			return errors.New("native_player requires voice")
		}
		st, err := os.Stat(quasarCLIPath)
		if err != nil || st.Mode()&0o111 == 0 {
			return errors.New("stock Quasar CLI is required for native player handover")
		}
	}
	if cfg.AliceButton {
		if !cfg.NativePlayer {
			return errors.New("alice_button requires native_player")
		}
		if !aliceButtonModels[d.Model] {
			return fmt.Errorf("alice_button is not qualified for model %q", d.Model)
		}
		if _, err := findAliceButtonEvent("/dev/input"); err != nil {
			return fmt.Errorf("Alice button keypad not found: %w", err)
		}
	}
	if cfg.NativeVolume {
		v, err := controlsVolume(ctx, api, d, cfg)
		if err != nil {
			return err
		}
		if err = v.CheckWrite(ctx); err != nil {
			return err
		}
		if err = waitNativeVolume(ctx, v); err != nil {
			return fmt.Errorf("native volume preflight failed; receiver was not started: %w", err)
		}
	}
	return nil
}

// yamd starts this plugin before mediad publishes the native volume IPC:
// measured plugin start at T+16.2 s against mediad at T+16.5..24 s, so a cold
// boot raced the volume read and the whole service exited. Retry the read for a
// bounded window instead of leaving the station without AirPlay until a restart.
var (
	nativeVolumeReadyTimeout  = 60 * time.Second
	nativeVolumeRetryInterval = time.Second
)

func waitNativeVolume(ctx context.Context, v volumeControl) error {
	deadline := time.Now().Add(nativeVolumeReadyTimeout)
	reported := false
	for {
		_, err := v.GetVolume(ctx)
		if err == nil {
			return nil
		}
		if !reported {
			fmt.Printf("station volume: native volume is not ready yet: %v\n", err)
			reported = true
		}
		if !time.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(nativeVolumeRetryInterval):
		}
	}
}
