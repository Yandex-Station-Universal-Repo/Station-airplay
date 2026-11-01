package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func testLunaVolume(t *testing.T, handler http.HandlerFunc) *lunaVolume {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &lunaVolume{
		api:      &yamAPI{base: server.URL, token: "test", tokenHeader: "X-Luna-Token", client: server.Client()},
		updates:  make(chan int, 16),
		requests: make(chan int, 1),
	}
}

func TestLunaServerRootDropsInvocationPath(t *testing.T) {
	root, err := lunaServerRoot("http://127.0.0.1:8099/api/runtime/station.airplay")
	if err != nil || root != "http://127.0.0.1:8099" {
		t.Fatalf("root=%q err=%v", root, err)
	}
	if _, err = lunaServerRoot("http://127.0.0.1:8099"); err == nil {
		t.Fatal("a runtime URL without the invocation path must be rejected")
	}
}

func TestLunaVolumeSetUsesTheVolumeEndpoint(t *testing.T) {
	received := make(chan int, 4)
	volume := testLunaVolume(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/volume/set" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var request struct {
			Percent int `json:"percent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode: %v", err)
		}
		received <- request.Percent
		_, _ = w.Write([]byte(`{"percent":` + strconv.Itoa(request.Percent) + `}`))
	})
	if err := volume.start(); err != nil {
		t.Fatal(err)
	}
	defer volume.stop()
	volume.set(42)
	select {
	case percent := <-received:
		if percent != 42 {
			t.Fatalf("sent %d", percent)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no volume set request")
	}
}

func TestLunaVolumePollReadsOnlyWhileActive(t *testing.T) {
	requests := make(chan struct{}, 4)
	volume := testLunaVolume(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/volume" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		requests <- struct{}{}
		_, _ = w.Write([]byte(`{"percent":63}`))
	})
	var got int
	show := func(percent int) { got = percent }

	volume.poll(false, show)
	select {
	case <-requests:
		t.Fatal("an idle station must not be polled")
	case <-time.After(50 * time.Millisecond):
	}

	volume.poll(true, show)
	deadline := time.Now().Add(3 * time.Second)
	for got != 63 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		volume.poll(true, show)
	}
	if got != 63 {
		t.Fatalf("polled volume %d", got)
	}
}
