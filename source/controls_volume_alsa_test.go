package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

type stubVolume struct {
	get  float64
	sets []int
	fail bool
}

func (s *stubVolume) GetVolume(context.Context) (float64, error) {
	if s.fail {
		return 0, errors.New("stub unavailable")
	}
	return s.get, nil
}

func (s *stubVolume) SetVolume(_ context.Context, pct int) (float64, error) {
	s.sets = append(s.sets, pct)
	return float64(pct), nil
}

func (s *stubVolume) CheckWrite(context.Context) error { return nil }
func (s *stubVolume) Backend() string                  { return "stub" }

func amixerFixture(t *testing.T, initial string, followSet bool) (*alsaControlVolume, string) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	calls := filepath.Join(dir, "calls")
	if err := os.WriteFile(state, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"STATE=" + state + "\n" +
		"CALLS=" + calls + "\n" +
		"case \"$1\" in\n" +
		" get) echo \"  Mono: Playback 102 [$(cat \"$STATE\")%] [-12.00dB] [on]\" ;;\n" +
		" set) case \"$3\" in\n"
	if followSet {
		script += "   *%) echo \"$3\" | tr -d '%' > \"$STATE\"; echo \"$2 $3\" >> \"$CALLS\" ;;\n"
	} else {
		script += "   *%) echo \"$2 $3\" >> \"$CALLS\" ;;\n"
	}
	script += "   unmute) echo unmute >> \"$CALLS\" ;;\n" +
		" esac ;;\n" +
		"esac\n"
	path := filepath.Join(dir, "amixer")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return &alsaControlVolume{path: path}, calls
}

func TestALSAGetReadsLastPercent(t *testing.T) {
	out := "Simple mixer control 'Master',0\n" +
		"  Limits: Playback 0 - 255\n" +
		"  Front Left: Playback 102 [40%] [-12.00dB] [on]\n" +
		"  Front Right: Playback 89 [35%] [-15.00dB] [on]\n"
	if got, err := lastPercent(out); err != nil || got != 35 {
		t.Fatalf("lastPercent = %d, %v", got, err)
	}
	if _, err := lastPercent("  Limits: Playback 0 - 255\n"); err == nil {
		t.Fatal("missing percentage accepted")
	}
}

func TestALSASetWritesUnmutesAndConfirms(t *testing.T) {
	v, calls := amixerFixture(t, "10", true)
	got, err := v.SetVolume(context.Background(), 60)
	if err != nil || got != 60 {
		t.Fatalf("SetVolume = %v, %v", got, err)
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); text != "Master 60%\nunmute\n" {
		t.Fatalf("unexpected amixer calls %q", text)
	}
	if cur, err := v.GetVolume(context.Background()); err != nil || cur != 60 {
		t.Fatalf("GetVolume = %v, %v", cur, err)
	}
}

func TestALSASetRejectsReadbackMismatch(t *testing.T) {
	v, _ := amixerFixture(t, "10", false)
	if _, err := v.SetVolume(context.Background(), 90); err == nil {
		t.Fatal("stale readback accepted")
	}
}

func TestALSASetRejectsOutOfRange(t *testing.T) {
	v, _ := amixerFixture(t, "10", true)
	for _, pct := range []int{-1, 101} {
		if _, err := v.SetVolume(context.Background(), pct); err == nil {
			t.Fatalf("accepted %d", pct)
		}
	}
}

func TestALSAFallsBackToLunaWhileTheMixerIsMissing(t *testing.T) {
	luna := &stubVolume{get: 35}
	v := &alsaControlVolume{path: "/nonexistent/amixer", luna: luna}
	if got, err := v.GetVolume(context.Background()); err != nil || got != 35 {
		t.Fatalf("GetVolume = %v, %v", got, err)
	}
	if got, err := v.SetVolume(context.Background(), 42); err != nil || got != 42 {
		t.Fatalf("SetVolume = %v, %v", got, err)
	}
	if len(luna.sets) != 1 || luna.sets[0] != 42 {
		t.Fatalf("Luna fallback calls = %v", luna.sets)
	}
	// Once the softvol control exists, the fast path takes over again.
	fast, _ := amixerFixture(t, "10", true)
	v.path = fast.path
	if got, err := v.SetVolume(context.Background(), 60); err != nil || got != 60 {
		t.Fatalf("fast path after mixer appeared = %v, %v", got, err)
	}
	if len(luna.sets) != 1 {
		t.Fatalf("Luna used while ALSA worked: %v", luna.sets)
	}
}

func TestVolumeManagerProbe(t *testing.T) {
	t.Setenv("LUNA_VOLUME_MANAGER_ADDR", "")
	old := quasarConfigPath
	quasarConfigPath = filepath.Join(t.TempDir(), "quasar.cfg")
	t.Cleanup(func() { quasarConfigPath = old })

	if volumeManagerListening() {
		t.Fatal("missing configuration reported as listening")
	}
	if err := os.WriteFile(quasarConfigPath, []byte(`{"volume_manager":{"port":0}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if volumeManagerListening() {
		t.Fatal("unconfigured port reported as listening")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = os.WriteFile(quasarConfigPath, []byte(fmt.Sprintf(`{"volume_manager":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	if !volumeManagerListening() {
		t.Fatal("listening manager not detected")
	}
	listener.Close()
	if volumeManagerListening() {
		t.Fatal("closed port reported as listening")
	}
}
