package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
