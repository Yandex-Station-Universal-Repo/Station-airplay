package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func volumeFixture(t *testing.T, script string) (string, yamDevice) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "station-volume")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "enabled.json"), []byte(`["station-volume"]`), 0600); e != nil {
		t.Fatal(e)
	}
	m := `id="station-volume"
[compatibility]
models=["yandexmidi"]
features=["arch.aarch64"]
`
	if e := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(m), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "station-volume.sh"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(root, "station-airplay"), yamDevice{Model: "yandexmidi", Features: []string{"arch.aarch64"}}
}
func TestVolumeDependencyRespectsCompatibility(t *testing.T) {
	root, d := volumeFixture(t, "exit1")
	if _, e := findPackageVolume(root, d); e != nil {
		t.Fatal(e)
	}
	for _, model := range []string{"yandexmini_2", "yandexplum", "yandexstation_2", "unknown"} {
		d.Model = model
		if _, e := findPackageVolume(root, d); e == nil {
			t.Fatal("model guard bypassed", model)
		}
	}
	d.Model = "yandexmidi"
	d.Features = nil
	if _, e := findPackageVolume(root, d); e == nil {
		t.Fatal("ABI guard bypassed")
	}
	d.Features = []string{"arch.aarch64"}
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(root), "enabled.json"), []byte(`[]`), 0600)
	if _, e := findPackageVolume(root, d); e == nil {
		t.Fatal("disabled dependency executed")
	}
	if _, e := v.GetVolume(context.Background()); e == nil {
		t.Fatal("disable after preflight ignored")
	}
}
func TestVolumeDependencyGuardReadbackAndNoToken(t *testing.T) {
	script := `#!/bin/sh
test -z "${YAM_API_TOKEN:-}" || exit 9
test -z "${YAM_API_URL:-}" || exit 9
test -z "${LD_PRELOAD:-}" || exit 9
case "$*" in
 check-write) echo ok;;
 get) echo 65;;
 "set 67") exit 0;;
 *) exit 2;;
esac
`
	root, d := volumeFixture(t, script)
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("YAM_API_TOKEN", "fixture-private-do-not-forward")
	t.Setenv("YAM_API_URL", "fixture-private")
	ctx := context.Background()
	if e = v.checkWrite(ctx); e != nil {
		t.Fatal(e)
	}
	if value, e := v.SetVolume(ctx, 67); e != nil || value != 65 {
		t.Fatal(value, e)
	}
	if _, e = v.SetVolume(ctx, 101); e == nil {
		t.Fatal("invalid percent accepted")
	}
	if _, e = v.SetVolume(ctx, 60); e == nil {
		t.Fatal("wrapper failure ignored")
	}
	if strings.Contains(script, "--force") {
		t.Fatal("fixture forces firmware")
	}
}
func TestVolumeDependencyMissingPreflightFailsClosed(t *testing.T) {
	root, d := volumeFixture(t, "#!/bin/sh\nexit 2\n")
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	if e = v.checkWrite(context.Background()); e == nil {
		t.Fatal("unsupported write firmware accepted")
	}
}

func retryWindow(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	oldTimeout, oldInterval := nativeVolumeReadyTimeout, nativeVolumeRetryInterval
	nativeVolumeReadyTimeout, nativeVolumeRetryInterval = timeout, interval
	t.Cleanup(func() {
		nativeVolumeReadyTimeout, nativeVolumeRetryInterval = oldTimeout, oldInterval
	})
}

// A cold boot starts this plugin before mediad publishes the volume IPC.
func TestNativeVolumePreflightRetriesUntilIPCReady(t *testing.T) {
	script := `#!/bin/sh
case "$*" in
 check-write) echo ok;;
 get)
  n=$(cat "$0.count" 2>/dev/null || echo 0)
  n=$((n+1))
  echo "$n" > "$0.count"
  if [ "$n" -lt 3 ]; then
   echo 'native volume connection: connect: no such file or directory' >&2
   exit 1
  fi
  echo 40
  ;;
 *) exit 2;;
esac
`
	root, d := volumeFixture(t, script)
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	retryWindow(t, 5*time.Second, 5*time.Millisecond)
	if e = waitNativeVolume(context.Background(), v); e != nil {
		t.Fatal("cold boot race was not absorbed:", e)
	}
	count, e := os.ReadFile(filepath.Join(filepath.Dir(root), "station-volume", "station-volume.sh.count"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.TrimSpace(string(count)) != "3" {
		t.Fatal("unexpected attempt count", string(count))
	}
}

func TestNativeVolumePreflightGivesUpWithDetail(t *testing.T) {
	script := `#!/bin/sh
case "$*" in
 check-write) echo ok;;
 get) echo 'native volume connection: connect: no such file or directory' >&2; exit 1;;
 *) exit 2;;
esac
`
	root, d := volumeFixture(t, script)
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	retryWindow(t, 60*time.Millisecond, 10*time.Millisecond)
	e = waitNativeVolume(context.Background(), v)
	if e == nil {
		t.Fatal("persistent volume failure accepted")
	}
	if !strings.Contains(e.Error(), "no such file or directory") {
		t.Fatal("stderr detail lost:", e)
	}
}

func TestNativeVolumePreflightHonorsCancellation(t *testing.T) {
	root, d := volumeFixture(t, "#!/bin/sh\ncase \"$*\" in\n check-write) echo ok;;\n get) exit 1;;\n *) exit 2;;\nesac\n")
	v, e := findPackageVolume(root, d)
	if e != nil {
		t.Fatal(e)
	}
	retryWindow(t, 30*time.Second, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if e = waitNativeVolume(ctx, v); e == nil {
		t.Fatal("cancelled wait succeeded")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancellation ignored")
	}
}
