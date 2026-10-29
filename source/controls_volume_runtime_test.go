package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRuntimeVolumeUsesGuardedLauncherAndRevalidatesEveryCall(t *testing.T) {
	clearRuntimeEnvironment(t)
	root := t.TempDir()
	script := filepath.Join(root, "station-volume.sh")
	if err := os.WriteFile(script, []byte("case \"$1\" in check-write) echo ok;; get) echo 30;; set) [ \"$2\" = 30 ];; *) exit 2;; esac\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var disabled atomic.Bool
	var resolutions atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolutions.Add(1)
		if r.URL.Path != "/dependencies/station-volume" || r.Header.Get("X-Luna-Token") != "synthetic-token" {
			t.Error("dependency lookup has wrong path or runtime authentication")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if disabled.Load() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"plugin_id": "station.volume", "directory": root, "program": script, "interpreter": "sh", "args": []string{}, "library_path": ""})
	}))
	defer host.Close()
	t.Setenv("LUNA_API_URL", host.URL)
	t.Setenv("LUNA_API_TOKEN", "synthetic-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	v, err := findRuntimePackageVolume(context.Background(), api, "station-volume", yamDevice{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.checkWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := v.GetVolume(context.Background()); err != nil || got != 30 {
		t.Fatalf("get: %v, %v", got, err)
	}
	if got, err := v.SetVolume(context.Background(), 30); err != nil || got != 30 {
		t.Fatalf("set/readback: %v, %v", got, err)
	}
	if resolutions.Load() != 5 {
		t.Fatal("each check/get/set/readback must resolve the dependency again")
	}
	disabled.Store(true)
	if _, err = v.GetVolume(context.Background()); err == nil {
		t.Fatal("disabled provider used a cached launcher or legacy fallback")
	}
	if _, err = v.SetVolume(context.Background(), 30); err == nil {
		t.Fatal("disabled provider accepted a write")
	}
	if _, err = v.SetVolume(context.Background(), 101); err == nil {
		t.Fatal("out-of-range write accepted")
	}
}

func TestRuntimeVolumeRejectsUnsafeLauncherMetadata(t *testing.T) {
	for _, mode := range []string{"interpreter", "force", "library", "relative", "escape", "missing", "provider-error"} {
		t.Run(mode, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			root := t.TempDir()
			script := filepath.Join(root, "station-volume.sh")
			if err := os.WriteFile(script, []byte("echo 30\n"), 0600); err != nil {
				t.Fatal(err)
			}
			metadata := map[string]any{"plugin_id": "station.volume", "directory": root, "program": script, "interpreter": "sh"}
			switch mode {
			case "interpreter":
				metadata["interpreter"] = "python"
			case "force":
				metadata["args"] = []string{"--force"}
			case "library":
				metadata["library_path"] = "/different/lib"
			case "relative":
				metadata["program"] = "station-volume.sh"
			case "escape":
				metadata["program"] = filepath.Join(filepath.Dir(root), "outside.sh")
			case "missing":
				metadata["program"] = filepath.Join(root, "missing.sh")
			}
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "provider-error" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(metadata)
			}))
			defer host.Close()
			t.Setenv("LUNA_API_URL", host.URL)
			t.Setenv("LUNA_API_TOKEN", "synthetic-token")
			api, err := newYAMAPI()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = findRuntimePackageVolume(context.Background(), api, "station-volume", yamDevice{}); err == nil {
				t.Fatal("unsafe or unavailable provider accepted")
			}
		})
	}
}

func TestRuntimeVolumeConfigurationRequiresExplicitSafeAlias(t *testing.T) {
	for _, alias := range []string{"../volume", "volume/other", "volume?query", strings.Repeat("a", 65)} {
		if validVolumeDependency(alias) {
			t.Fatal("unsafe alias accepted")
		}
	}
	for _, text := range []string{`{"native_volume":false,"volume_dependency":"station-volume"}`, `{"native_volume":true,"volume_dependency":"../bad"}`} {
		path := filepath.Join(t.TempDir(), "controls.json")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadControls(path); err == nil {
			t.Fatal("unsafe dependency config accepted")
		}
	}
}
