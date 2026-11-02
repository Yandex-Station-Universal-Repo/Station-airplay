package main

// Glue only: native wire protocol and acknowledgement stay in station-volume.
// Respect BOTH its YAM compatibility manifest and its firmware write guard.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

// volumeControl is the station-volume backend used by the shared controls.
// Every implementation must keep the same three operations: read the current
// level, write a level and confirm the write, and prove before startup that
// writing is safe on this board.
type volumeControl interface {
	GetVolume(context.Context) (float64, error)
	SetVolume(context.Context, int) (float64, error)
	CheckWrite(context.Context) error
	Backend() string
}

type packageVolume struct {
	script string
	root   string
	device yamDevice
	api    *yamAPI
	alias  string
}

func (v *packageVolume) Backend() string { return "the station-volume package" }

func findPackageVolume(root string, d yamDevice) (*packageVolume, error) {
	dir := filepath.Join(filepath.Dir(root), "station-volume")
	data, e := os.ReadFile(filepath.Join(filepath.Dir(root), "enabled.json"))
	if e != nil {
		return nil, errors.New("cannot verify station-volume is enabled")
	}
	var enabled []string
	if e = json.Unmarshal(data, &enabled); e != nil {
		return nil, errors.New("invalid package enable state")
	}
	found := false
	for _, id := range enabled {
		found = found || id == "station-volume"
	}
	if !found {
		return nil, errors.New("station-volume dependency is disabled")
	}
	var manifest struct {
		ID            string `toml:"id"`
		Compatibility struct {
			Models   []string `toml:"models"`
			Features []string `toml:"features"`
		} `toml:"compatibility"`
	}
	if _, e := toml.DecodeFile(filepath.Join(dir, "plugin.toml"), &manifest); e != nil {
		return nil, errors.New("station-volume dependency is missing or invalid")
	}
	if manifest.ID != "station-volume" {
		return nil, errors.New("invalid volume package identity")
	}
	if len(manifest.Compatibility.Models) > 0 {
		matched := false
		for _, model := range manifest.Compatibility.Models {
			matched = matched || model == d.Model
		}
		if !matched {
			return nil, errors.New("station-volume does not declare this model compatible")
		}
	}
	for _, f := range manifest.Compatibility.Features {
		if !feature(d, f) {
			return nil, errors.New("station-volume feature requirements are not met")
		}
	}
	script := filepath.Join(dir, "station-volume.sh")
	if st, e := os.Stat(script); e != nil || !st.Mode().IsRegular() {
		return nil, errors.New("station-volume launcher is unavailable")
	}
	return &packageVolume{script: script, root: root, device: d}, nil
}

type smallOutput struct{ text strings.Builder }

func (b *smallOutput) Write(p []byte) (int, error) {
	if b.text.Len()+len(p) > 4096 {
		return 0, errors.New("dependency output exceeds limit")
	}
	return b.text.Write(p)
}
func (v *packageVolume) call(ctx context.Context, args ...string) (string, error) {
	var current *packageVolume
	var e error
	if v.api != nil {
		current, e = findRuntimePackageVolume(ctx, v.api, v.alias, v.device)
	} else {
		current, e = findPackageVolume(v.root, v.device)
	}
	if e != nil {
		return "", e
	}
	step, done := context.WithTimeout(ctx, 6*time.Second)
	defer done()
	cmd := exec.CommandContext(step, "sh", append([]string{current.script}, args...)...)
	if v.api != nil {
		cmd.Dir = current.root
	}
	cmd.Env = []string{"PATH=/system/bin:/usr/bin:/bin", "LANG=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 100 * time.Millisecond
	var out, errOut smallOutput
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if e := cmd.Run(); e != nil {
		if detail := strings.TrimSpace(errOut.text.String()); detail != "" {
			return "", fmt.Errorf("station-volume command failed or firmware is not verified: %s", detail)
		}
		return "", errors.New("station-volume command failed or firmware is not verified")
	}
	return strings.TrimSpace(out.text.String()), nil
}
func (v *packageVolume) CheckWrite(ctx context.Context) error {
	out, e := v.call(ctx, "check-write")
	if e != nil {
		return e
	}
	if out != "ok" {
		return errors.New("station-volume write preflight is unsupported")
	}
	return nil
}
func (v *packageVolume) GetVolume(ctx context.Context) (float64, error) {
	out, e := v.call(ctx, "get")
	if e != nil {
		return 0, e
	}
	value, e := strconv.ParseFloat(out, 64)
	if e != nil {
		return 0, errors.New("invalid station-volume result")
	}
	if _, e = nativePercent(value); e != nil {
		return 0, e
	}
	return value, nil
}
func (v *packageVolume) SetVolume(ctx context.Context, pct int) (float64, error) {
	if pct < 0 || pct > 100 {
		return 0, errors.New("volume must be0..100")
	}
	// Never call the native binary directly, and never pass --force.
	if _, e := v.call(ctx, "set", strconv.Itoa(pct)); e != nil {
		return 0, e
	}
	actual, e := v.GetVolume(ctx)
	if e != nil {
		return 0, e
	}
	expected := (pct + 2) / 5 * 5
	if actual != float64(expected) {
		return 0, errors.New("station-volume readback did not match")
	}
	return actual, nil
}
