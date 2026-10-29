package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A custom token header must not follow even a local server's redirect.
// Only synthetic tokens and loopback fixtures are used in this test.
func TestYAMTokenCannotFollowRedirect(t *testing.T) {
	clearRuntimeEnvironment(t)
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-YAM-Token") != "" {
			forwarded.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"yandexmini_2"}`))
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/device", http.StatusFound)
	}))
	defer origin.Close()
	t.Setenv("YAM_API_URL", origin.URL)
	t.Setenv("YAM_API_TOKEN", "synthetic-fixture-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.device()
	if err == nil || forwarded.Load() != 0 {
		t.Fatalf("redirect was followed: request_error=%v forwarded_token_requests=%d", err, forwarded.Load())
	}
}

func TestOwnInterfaceYAMTrafficCannotUseEnvironmentProxy(t *testing.T) {
	clearRuntimeEnvironment(t)
	t.Setenv("YAM_API_URL", "http://127.0.0.1:8099/api/runtime/fixture")
	t.Setenv("YAM_API_TOKEN", "synthetic-fixture-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := api.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("local authenticated API must explicitly bypass all environment proxies")
	}
	defer transport.CloseIdleConnections()
}
