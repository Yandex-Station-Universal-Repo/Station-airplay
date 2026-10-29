package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func clearRuntimeEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"LUNA_API_URL", "LUNA_API_TOKEN", "YAM_API_URL", "YAM_API_TOKEN"} {
		t.Setenv(key, "")
	}
}

func TestRuntimeCredentialSelection(t *testing.T) {
	const local = "http://127.0.0.1:8099/api/runtime/fixture"
	const token = "synthetic-runtime-secret"
	for _, tc := range []struct {
		name, lunaURL, lunaToken, yamURL, yamToken, header string
	}{
		{"luna", local, token, "", "", "X-Luna-Token"},
		{"legacy", "", "", local, token, "X-YAM-Token"},
		{"identical aliases prefer Luna", local, token, local, token, "X-Luna-Token"},
		{"trailing slash alias", local + "/", token, local, token, "X-Luna-Token"},
		{"missing", "", "", "", "", ""},
		{"Luna URL only", local, "", "", "", ""},
		{"Luna token only", "", token, "", "", ""},
		{"legacy URL only", "", "", local, "", ""},
		{"legacy token only", "", "", "", token, ""},
		{"do not mix families", local, "", "", token, ""},
		{"partial Luna must not fall back", local, "", local, token, ""},
		{"partial legacy alongside Luna", local, token, local, "", ""},
		{"conflicting tokens", local, token, local, "other-synthetic-secret", ""},
		{"conflicting endpoints", local, token, "http://127.0.0.1:8100/other", token, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRuntimeAPI(tc.lunaURL, tc.lunaToken, tc.yamURL, tc.yamToken)
			if tc.header == "" {
				if err == nil || got != (runtimeCredentials{}) {
					t.Fatal("invalid environment must fail closed")
				}
				if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "other-synthetic-secret") {
					t.Fatal("credential error leaked a token")
				}
				return
			}
			if err != nil || got.header != tc.header || got.token != token {
				t.Fatalf("credential selection failed: %v", err)
			}
			wantURL := tc.lunaURL
			if wantURL == "" {
				wantURL = tc.yamURL
			}
			if got.url != wantURL {
				t.Fatal("wrong runtime endpoint selected")
			}
		})
	}
}

func TestRuntimeRequestUsesOnlySelectedHeader(t *testing.T) {
	for _, family := range []string{"LUNA", "YAM", "ALIASES"} {
		t.Run(family, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			header := "X-Luna-Token"
			if family == "YAM" {
				header = "X-YAM-Token"
			}
			var count atomic.Int32
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				other := "X-YAM-Token"
				if header == other {
					other = "X-Luna-Token"
				}
				if r.Header.Get(header) != "synthetic-token" || r.Header.Get(other) != "" {
					t.Error("wrong or duplicate runtime authentication headers")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"model":"yandexmini_2"}`))
			}))
			defer host.Close()
			if family != "YAM" {
				t.Setenv("LUNA_API_URL", host.URL)
				t.Setenv("LUNA_API_TOKEN", "synthetic-token")
			}
			if family != "LUNA" {
				t.Setenv("YAM_API_URL", host.URL)
				t.Setenv("YAM_API_TOKEN", "synthetic-token")
			}
			api, err := newYAMAPI()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = api.device(); err != nil {
				t.Fatal(err)
			}
			if err = api.requestContext(context.Background(), "POST", "/command/cancel", map[string]string{"request_id": "fixture"}, nil); err != nil {
				t.Fatal(err)
			}
			if count.Load() != 2 {
				t.Fatal("unexpected request retries")
			}
			if transport := api.client.Transport.(*http.Transport); transport.Proxy != nil {
				t.Fatal("runtime requests must bypass environment proxies")
			}
		})
	}
}

func TestLunaAuthenticationDoesNotDowngrade(t *testing.T) {
	clearRuntimeEnvironment(t)
	var requests atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-YAM-Token") != "" {
			t.Error("must not retry with legacy authentication")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer host.Close()
	t.Setenv("LUNA_API_URL", host.URL)
	t.Setenv("LUNA_API_TOKEN", "synthetic-token")
	t.Setenv("YAM_API_URL", host.URL)
	t.Setenv("YAM_API_TOKEN", "synthetic-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.device(); err == nil || requests.Load() != 1 {
		t.Fatal("failed authentication must not retry or downgrade")
	}
}

func TestLunaTokenCannotFollowRedirect(t *testing.T) {
	clearRuntimeEnvironment(t)
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/device", http.StatusFound)
	}))
	defer origin.Close()
	t.Setenv("LUNA_API_URL", origin.URL)
	t.Setenv("LUNA_API_TOKEN", "synthetic-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = api.device(); err == nil || forwarded.Load() != 0 {
		t.Fatal("Luna runtime request followed a redirect")
	}
}

func TestLunaRuntimeRejectsExternalEndpoint(t *testing.T) {
	for _, address := range []string{"https://127.0.0.1:8099/runtime", "http://example.invalid/runtime", "http://192.0.2.1:8099/runtime"} {
		t.Run(address, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			t.Setenv("LUNA_API_URL", address)
			t.Setenv("LUNA_API_TOKEN", "synthetic-token")
			if _, err := newYAMAPI(); err == nil {
				t.Fatal("non-local or non-HTTP runtime endpoint accepted")
			}
		})
	}
}

func TestAudioEnvironmentStripsBothRuntimeCredentialFamilies(t *testing.T) {
	for _, bus := range []string{"", "/run/fixture/private-bus"} {
		env := audioEnv([]string{
			"LUNA_API_URL=http://127.0.0.1:8099", "LUNA_API_TOKEN=synthetic-luna-secret",
			"YAM_API_URL=http://127.0.0.1:8099", "YAM_API_TOKEN=synthetic-yam-secret",
			"LUNA_API_FUTURE=also-private", "YAM_API_FUTURE=also-private",
			"LD_LIBRARY_PATH=/legacy", "HOME=/old", "PATH=/usr/bin:/bin",
			"YAM_ROOT=/data/yam", "LUNA_PLUGIN_ID=fixture",
		}, "/run/fixture/home", bus)
		for _, entry := range env {
			if strings.HasPrefix(entry, "LUNA_API_") || strings.HasPrefix(entry, "YAM_API_") || strings.HasPrefix(entry, "LD_LIBRARY_PATH=") {
				t.Fatal("audio child inherited runtime credentials or loader path")
			}
		}
		joined := strings.Join(env, "\n")
		for _, want := range []string{"PATH=/usr/bin:/bin", "YAM_ROOT=/data/yam", "LUNA_PLUGIN_ID=fixture", "HOME=/run/fixture/home"} {
			if !strings.Contains(joined, want) {
				t.Fatal("non-secret child environment was lost")
			}
		}
	}
}
