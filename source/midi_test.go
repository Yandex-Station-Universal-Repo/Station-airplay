package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func withProfile(t *testing.T, p stationProfile) {
	old := hw
	hw = p
	t.Cleanup(func() { hw = old })
}

func TestMidiProfileFromYAM(t *testing.T) {
	var ledCount int
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device":
			_ = json.NewEncoder(w).Encode(yamDevice{Model: "yandexmidi", Features: []string{"display.clock.segment.v1", "led.ring.rgb", "led.ring.24"}})
		case "/led":
			var data map[string][]string
			_ = json.NewDecoder(r.Body).Decode(&data)
			ledCount = len(data["colors"])
			w.WriteHeader(204)
		default:
			w.WriteHeader(204)
		}
	}))
	defer h.Close()
	t.Setenv("YAM_API_URL", h.URL)
	t.Setenv("YAM_API_TOKEN", "secret")
	withProfile(t, plumProfile)
	a, e := newYAMAPI()
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyYAM(a); e != nil {
		t.Fatal(e)
	}
	if hw.name != "midi" || ledCount != 24 {
		t.Fatalf("profile %q, ring preflight wrote %d LEDs", hw.name, ledCount)
	}
}

func TestUnknownModelOrMissingRingRejected(t *testing.T) {
	for _, d := range []yamDevice{
		{Model: "yandexmini_2", Features: []string{"display.clock.segment.v1", "led.ring.rgb"}},
		{Model: "yandexmidi", Features: []string{"display.clock.segment.v1", "led.ring.rgb", "led.ring.3"}},
	} {
		h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(d) }))
		t.Setenv("YAM_API_URL", h.URL)
		t.Setenv("YAM_API_TOKEN", "secret")
		a, _ := newYAMAPI()
		if _, e := detectProfile(a); e == nil {
			t.Errorf("accepted %+v", d)
		}
		h.Close()
	}
}

func TestMidiSegmentsAndRing(t *testing.T) {
	withProfile(t, midiProfile)
	seg := segmentsFor("PAUS", true)
	for k := range seg {
		if len(k) != 2 || k[0] < 'A' || k[0] > 'G' || k[1] < '1' || k[1] > '4' {
			t.Fatalf("segment %q outside Midi digits A..G 1..4", k)
		}
	}
	if seg["A1"] == 0 || seg["E3"] == 0 || seg["E4"] != 0 { // P has A, U has E, S has none
		t.Fatalf("PAUS not on digits 1..4: %v", seg)
	}
	s := &airplayState{status: "stopped"}
	now := time.Now()
	s.showVolume(50, now)
	ring := s.ringFrame(now)
	if len(ring) != 24 {
		t.Fatalf("ring has %d LEDs", len(ring))
	}
	if ring[11] == "#000000" || ring[12] != "#000000" {
		t.Fatalf("50%% must light the first 12 LEDs: %v", ring)
	}
	_, frame := s.display(now, time.UTC)
	if frame["G3"] == 0 || frame["E4"] == 0 || frame["A1"] != 0 { // "  50": 5 has G, 0 has E, digit 1 blank
		t.Fatalf("volume overlay %v", frame)
	}
}

func TestSenderVolumeIsForwardedOnce(t *testing.T) {
	s := &airplayState{status: "playing"}
	s.onMetadata("73736e63", "70766f6c", []byte("-15,0,0,0"), time.Now())
	if !s.senderVolume || s.volume != 50 {
		t.Fatalf("pvol not marked for forwarding: %+v", s.senderVolume)
	}
	if p, ok := parsePercent("45\n"); !ok || p != 45 {
		t.Fatal("watch output not parsed")
	}
	if _, ok := parsePercent("nan"); ok {
		t.Fatal("accepted NaN")
	}
}
