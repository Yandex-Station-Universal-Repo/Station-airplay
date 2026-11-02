package main

// Direct ALSA station volume for boards whose Quasar volume manager is absent.
//
// Luna owns the station volume and prefers the maind/VQE volume manager at
// 127.0.0.1:9922. When that port is closed Luna falls back to exactly this:
// `amixer set Master N%`, then a best-effort unmute, then a readback. Some
// shipped firmwares list volume_manager.port in quasar.cfg but never listen on
// it, so every Luna write first retries the dead port for about a second and
// the sender's slider ends up seconds behind the phone.
//
// The plugin chooses this backend only when that port is not listening, so a
// firmware with a working manager keeps Luna's path. The ALSA mixer is still the
// canonical station level on the fallback boards: Luna's GET reads the same
// control, and the station buttons and Alice write it as well.
//
// The mixer control can appear only after the first PCM open on softvol
// firmware, so Luna stays as a per-call fallback instead of a startup veto.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	quasarAMixerTimeout      = 4 * time.Second
	quasarVolumeProbeTimeout = 300 * time.Millisecond
)

// quasarConfigPath is a variable so tests can point the probe at a fixture.
var quasarConfigPath = "/system/vendor/quasar/quasar.cfg"

type alsaControlVolume struct {
	path string
	luna volumeControl // optional: used when ALSA is not ready on this board
}

func (v *alsaControlVolume) Backend() string { return "ALSA Master (Luna volume manager unavailable)" }

// amixerEnv is deliberately clean. The plugin inherits the package's loader
// environment, and the system amixer must not load that libc.
func amixerEnv() []string {
	return []string{
		"PATH=/system/bin:/usr/bin:/bin:/sbin:/usr/sbin",
		"HOME=/tmp",
		"LANG=C",
		"LC_ALL=C",
	}
}

func (v *alsaControlVolume) run(ctx context.Context, args ...string) (string, error) {
	step, done := context.WithTimeout(ctx, quasarAMixerTimeout)
	defer done()
	cmd := exec.CommandContext(step, v.path, args...)
	cmd.Env = amixerEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("amixer command failed")
	}
	if len(out) > 65536 {
		return "", errors.New("amixer output exceeds limit")
	}
	return string(out), nil
}

// lastPercent mirrors Luna's parser: the final "[NN%]" token in amixer output.
func lastPercent(out string) (int, error) {
	i := strings.LastIndexByte(out, '%')
	if i < 0 {
		return 0, errors.New("amixer reported no volume percentage")
	}
	j := i
	for j > 0 && out[j-1] >= '0' && out[j-1] <= '9' {
		j--
	}
	if j == i {
		return 0, errors.New("amixer reported no volume percentage")
	}
	n, err := strconv.Atoi(out[j:i])
	if err != nil || n < 0 || n > 100 {
		return 0, errors.New("amixer reported an invalid volume")
	}
	return n, nil
}

func (v *alsaControlVolume) read(ctx context.Context) (int, error) {
	out, err := v.run(ctx, "get", "Master")
	if err != nil {
		return 0, err
	}
	return lastPercent(out)
}

func (v *alsaControlVolume) GetVolume(ctx context.Context) (float64, error) {
	percent, err := v.read(ctx)
	if err == nil {
		return float64(percent), nil
	}
	if v.luna != nil {
		return v.luna.GetVolume(ctx)
	}
	return 0, err
}

func (v *alsaControlVolume) SetVolume(ctx context.Context, pct int) (float64, error) {
	if pct < 0 || pct > 100 {
		return 0, errors.New("volume must be 0..100")
	}
	if _, err := v.run(ctx, "set", "Master", fmt.Sprintf("%d%%", pct)); err == nil {
		// Not every control carries a playback switch (softvol without one is
		// common), so the switch is best-effort, exactly as Luna does it.
		_, _ = v.run(ctx, "set", "Master", "unmute")
		if actual, err := v.read(ctx); err == nil && actual-pct >= -5 && actual-pct <= 5 {
			return float64(actual), nil
		}
	}
	if v.luna != nil {
		return v.luna.SetVolume(ctx, pct)
	}
	return 0, errors.New("ALSA volume readback did not match")
}

func (v *alsaControlVolume) CheckWrite(ctx context.Context) error {
	_, err := v.GetVolume(ctx)
	return err
}

// findAmixer accepts the vendor location too: some firmwares keep the utility
// outside the service PATH.
func findAmixer() (string, error) {
	if path, err := exec.LookPath("amixer"); err == nil {
		return path, nil
	}
	for _, path := range []string{"/system/workdir/bin/amixer", "/usr/bin/amixer", "/system/bin/amixer"} {
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("amixer is not available on this station")
}

// newALSAControlVolume only proves the utility is present. The mixer control is
// probed on every call so a softvol control that appears on the first PCM open
// does not pin the slower Luna path for the whole boot.
func newALSAControlVolume(luna volumeControl) (*alsaControlVolume, error) {
	path, err := findAmixer()
	if err != nil {
		return nil, err
	}
	return &alsaControlVolume{path: path, luna: luna}, nil
}

// quasarVolumeManagerPort resolves the same endpoint Luna uses: the explicit
// environment override first, then volume_manager.port from quasar.cfg. A
// missing or invalid configuration means Luna cannot reach maind either, so the
// caller must fall back to ALSA.
func quasarVolumeManagerPort() (int, bool) {
	if address := os.Getenv("LUNA_VOLUME_MANAGER_ADDR"); address != "" {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return 0, false
		}
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return 0, false
		}
		return n, true
	}
	f, err := os.Open(quasarConfigPath)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	var cfg struct {
		VolumeManager struct {
			Port int `json:"port"`
		} `json:"volume_manager"`
	}
	if err = json.NewDecoder(io.LimitReader(f, 4<<20)).Decode(&cfg); err != nil {
		return 0, false
	}
	if cfg.VolumeManager.Port <= 0 || cfg.VolumeManager.Port > 65535 {
		return 0, false
	}
	return cfg.VolumeManager.Port, true
}

func volumeManagerListening() bool {
	port, ok := quasarVolumeManagerPort()
	if !ok {
		return false
	}
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), quasarVolumeProbeTimeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
