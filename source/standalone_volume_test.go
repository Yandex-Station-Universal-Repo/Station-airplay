package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStandaloneSenderShowsQuantizedNativeVolume(t *testing.T) {
	c, r, s := testControls()
	c.quantizeSender = true
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	r.s.DB = percentDB(23)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.v != 25 || len(r.volumes) != 1 || r.volumes[0] != percentDB(25) {
		t.Fatalf("native step not returned to sender: native=%v feedback=%v", s.v, r.volumes)
	}
	r.s.DB = r.volumes[0]
	if err := c.tick(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(s.sets) != 1 || len(r.volumes) != 1 || c.remotePending {
		t.Fatal("quantization acknowledgement caused a feedback loop")
	}
}

func TestStandaloneVolumeSliderEndpoints(t *testing.T) {
	for _, pct := range []int{0, 100} {
		c, r, s := testControls()
		c.quantizeSender = true
		ctx, now := context.Background(), time.Now()
		if err := c.tick(ctx, now); err != nil {
			t.Fatal(err)
		}
		r.s.DB = percentDB(pct)
		if err := c.tick(ctx, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if s.v != float64(pct) || len(s.sets) != 1 {
			t.Fatalf("slider %d: native=%v writes=%v", pct, s.v, s.sets)
		}
	}
}

func TestStandaloneExactMini2Board(t *testing.T) {
	board := "yandex,station-mini2\x00amlogic, axg\x00"
	if d, err := standaloneDevice(board); err != nil || d.Model != "yandexmini_2" {
		t.Fatalf("qualified build rejected: %+v %v", d, err)
	}
	for _, c := range []string{
		"", "yandex,cucumber\x00", "prefix-yandex,station-mini2-suffix\x00",
	} {
		if _, err := standaloneDevice(c); err == nil {
			t.Fatalf("unqualified build/board accepted: %q", c)
		}
	}
}

func TestStandaloneCannotBypassRuntimeCredentials(t *testing.T) {
	for _, name := range []string{"LUNA_API_URL", "LUNA_API_TOKEN", "YAM_API_URL", "YAM_API_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"LUNA_API_URL", "LUNA_API_TOKEN", "YAM_API_URL", "YAM_API_TOKEN"} {
				t.Setenv(key, "")
			}
			t.Setenv(name, "fixture")
			if err := runMini2Standalone(context.Background(), true, &controlsConfig{NativeVolume: true}, "/missing"); err == nil || !strings.Contains(err.Error(), "credentials") {
				t.Fatalf("partial credentials ignored: %v", err)
			}
		})
	}
}

func TestStandaloneLauncherGuardAndReadback(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "station-volume.sh")
	write := func(script string) {
		t.Helper()
		if err := os.WriteFile(launcher, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("#!/bin/sh\ncase \"$*\" in\ncheck-write) echo ok;;\nget) echo 25;;\n'set 23') exit 0;;\n*) exit 2;;\nesac\n")
	v := standaloneVolume{script: launcher}
	if actual, err := v.SetVolume(context.Background(), 23); err != nil || actual != 25 {
		t.Fatalf("readback: %v %v", actual, err)
	}
	if _, err := v.SetVolume(context.Background(), 101); err == nil {
		t.Fatal("out of range accepted")
	}
	write("#!/bin/sh\ncase \"$1\" in\ncheck-write) exit 2;;\nset) touch \"$0.written\";;\nesac\n")
	if _, err := v.SetVolume(context.Background(), 25); err == nil {
		t.Fatal("firmware guard bypassed")
	}
	if _, err := os.Stat(launcher + ".written"); !os.IsNotExist(err) {
		t.Fatal("rejected firmware reached set")
	}
	write("#!/bin/sh\ncase \"$1\" in\ncheck-write) echo unsupported;;\nget) echo nan;;\nset) exit 0;;\nesac\n")
	if err := v.checkWrite(context.Background()); err == nil {
		t.Fatal("invalid preflight accepted")
	}
	if _, err := v.GetVolume(context.Background()); err == nil {
		t.Fatal("invalid native state accepted")
	}
	if validateVolumeLauncher("relative/script.sh") == nil {
		t.Fatal("relative launcher accepted")
	}
	if validateVolumeLauncher(root) == nil {
		t.Fatal("directory accepted")
	}
	if err := os.Chmod(launcher, 0777); err != nil {
		t.Fatal(err)
	}
	if validateVolumeLauncher(launcher) == nil {
		t.Fatal("world-writable launcher accepted")
	}
}

func TestStandaloneHelperCancellationAndCleanEnvironment(t *testing.T) {
	launcher := filepath.Join(t.TempDir(), "station-volume.sh")
	script := `#!/bin/sh
test -z "${LUNA_API_TOKEN:-}" || exit 9
test -z "${YAM_API_TOKEN:-}" || exit 9
test -z "${LD_PRELOAD:-}" || exit 9
case "$1" in
check-write) echo ok;;
get) sleep 30;;
esac
`
	if err := os.WriteFile(launcher, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUNA_API_TOKEN", "fixture-private")
	t.Setenv("YAM_API_TOKEN", "fixture-private")
	t.Setenv("LD_PRELOAD", "fixture-library")
	v := standaloneVolume{script: launcher}
	if err := v.checkWrite(context.Background()); err != nil {
		t.Fatalf("ambient environment reached helper: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := v.GetVolume(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("helper process group outlived cancellation")
	}
}

func TestStandaloneRejectsHostDependentControls(t *testing.T) {
	if err := standaloneConfig(&controlsConfig{NativeVolume: true}); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []controlsConfig{
		{}, {NativeVolume: true, Voice: true}, {NativeVolume: true, Ducking: true},
		{NativeVolume: true, LEDs: true}, {NativeVolume: true, NativePlayer: true},
		{NativeVolume: true, AliceButton: true}, {NativeVolume: true, Group: &groupConfig{}},
	} {
		if standaloneConfig(&cfg) == nil {
			t.Fatalf("unsupported standalone controls accepted: %+v", cfg)
		}
	}
}
