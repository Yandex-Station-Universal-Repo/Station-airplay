package main

// Explicit Mini 2 adapter for plain YAM 0.2.x. The existing controller and
// receiver handle AirPlay feedback; the upstream native-volume helper owns
// Quasar framing and its normal volume indication. No speech or LED API host.
import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func hasRuntimeCredentials() bool {
	return os.Getenv("LUNA_API_URL") != "" || os.Getenv("LUNA_API_TOKEN") != "" ||
		os.Getenv("YAM_API_URL") != "" || os.Getenv("YAM_API_TOKEN") != ""
}

func standaloneDevice(compatible string) (yamDevice, error) {
	board := false
	for _, id := range strings.Split(compatible, "\x00") {
		board = board || id == "yandex,station-mini2"
	}
	if !board {
		return yamDevice{}, errors.New("standalone volume supports only the qualified Mini 2 board")
	}
	return yamDevice{Model: "yandexmini_2", Architecture: "aarch64"}, nil
}

func standaloneConfig(cfg *controlsConfig) error {
	if cfg == nil || !cfg.NativeVolume || cfg.Voice || cfg.Ducking || cfg.LEDs || cfg.NativePlayer || cfg.AliceButton || cfg.Group != nil {
		return errors.New("standalone mode supports native volume only")
	}
	return nil
}

func runMini2Standalone(ctx context.Context, sharedAudio bool, cfg *controlsConfig, launcher string) error {
	if !sharedAudio {
		return errors.New("--standalone requires --shared-audio")
	}
	if hasRuntimeCredentials() {
		return errors.New("standalone mode cannot ignore Luna/legacy YAM runtime credentials")
	}
	if err := standaloneConfig(cfg); err != nil {
		return err
	}
	if err := validateVolumeLauncher(launcher); err != nil {
		return err
	}
	board, err := os.ReadFile("/proc/device-tree/compatible")
	if err != nil {
		return err
	}
	d, err := standaloneDevice(string(board))
	if err != nil {
		return err
	}
	v := standaloneVolume{script: launcher}
	if err := v.checkWrite(ctx); err != nil {
		return err
	}
	if err = waitForReady(ctx, "standalone native volume", 60*time.Second, func() error {
		_, err := v.GetVolume(ctx)
		return err
	}); err != nil {
		return fmt.Errorf("native volume unavailable: %w", err)
	}
	// Keep firmware qualification in station-volume, the single source of truth.
	// A copy also keeps caller-owned configuration immutable.
	local := *cfg
	local.volumeLauncher = launcher
	p, _ := profileFor(d.Model)
	fmt.Println("standalone Mini 2: native volume and stock volume indication enabled")
	return runSharedControls(ctx, p, nil, d, &local)
}

type standaloneVolume struct{ script string }

func validateVolumeLauncher(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("--volume-launcher must name an absolute station-volume launcher path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("station-volume launcher: %w", err)
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 {
		return errors.New("station-volume launcher must be a regular file without group/other write permission")
	}
	return nil
}

func (v standaloneVolume) checkWrite(ctx context.Context) error {
	out, err := v.call(ctx, "check-write")
	if err != nil {
		return fmt.Errorf("native volume guard failed: %w", err)
	}
	if out != "ok" {
		return errors.New("station-volume does not confirm check-write support")
	}
	return nil
}

func (v standaloneVolume) call(ctx context.Context, args ...string) (string, error) {
	if err := validateVolumeLauncher(v.script); err != nil {
		return "", err
	}
	step, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(step, "sh", append([]string{v.script}, args...)...)
	cmd.Env = []string{"PATH=/system/bin:/usr/bin:/bin", "LANG=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 100 * time.Millisecond
	var out, detail smallOutput
	cmd.Stdout, cmd.Stderr = &out, &detail
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("native volume: %w: %s", err, strings.TrimSpace(detail.text.String()))
	}
	return strings.TrimSpace(out.text.String()), nil
}

func (v standaloneVolume) GetVolume(ctx context.Context) (float64, error) {
	out, err := v.call(ctx, "get")
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseFloat(out, 64)
	if err != nil {
		return 0, errors.New("invalid native volume readback")
	}
	_, err = nativePercent(n)
	return n, err
}

func (v standaloneVolume) SetVolume(ctx context.Context, percent int) (float64, error) {
	if percent < 0 || percent > 100 {
		return 0, errors.New("native volume must be between 0 and 100")
	}
	if err := v.checkWrite(ctx); err != nil {
		return 0, err
	}
	if _, err := v.call(ctx, "set", strconv.Itoa(percent)); err != nil {
		return 0, err
	}
	actual, err := v.GetVolume(ctx)
	if err != nil {
		return 0, err
	}
	if actual != float64((percent+2)/5*5) {
		return 0, errors.New("native volume acknowledgement did not match requested level")
	}
	return actual, nil
}

func (v standaloneVolume) Colors(context.Context, []string) error {
	return errors.New("standalone LED rendering disabled")
}
func (v standaloneVolume) Off(context.Context) error {
	return errors.New("standalone LED rendering disabled")
}
func (v standaloneVolume) ClaimSpeech(context.Context, string) error {
	return errors.New("standalone speech controls disabled")
}

func serveStandaloneVolume(ctx context.Context, bus string, cfg controlsConfig) error {
	if err := standaloneConfig(&cfg); err != nil {
		return err
	}
	if err := validateVolumeLauncher(cfg.volumeLauncher); err != nil {
		return err
	}
	r, err := openReceiver(bus)
	if err != nil {
		return err
	}
	defer r.conn.Close()
	c := sharedController{cfg: cfg, receiver: r, station: standaloneVolume{script: cfg.volumeLauncher}, quantizeSender: true}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastError time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			step, cancel := context.WithTimeout(ctx, 7*time.Second)
			err := c.tick(step, now)
			cancel()
			if err != nil && now.Sub(lastError) >= 10*time.Second {
				fmt.Fprintf(os.Stderr, "volume sync: %v\n", err)
				lastError = now
			}
		}
	}
}
