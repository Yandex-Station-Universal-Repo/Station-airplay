package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// The upstream Available property is DACP-specific. A modern AirPlay 2 sender
// must only become connected when the receiver confirms its live event
// channel. Its principal client wins over retained classic metadata.
type mockStatusService struct {
	modern bool
	client string
}

func (s *mockStatusService) GetControlStatus() (uint32, bool, string, *dbus.Error) {
	return 1, s.modern, s.client, nil
}

func TestModernStatusRequiresLiveExactClient(t *testing.T) {
	daemon, e := exec.LookPath("dbus-daemon")
	if e != nil {
		t.Skip("dbus-daemon unavailable; integration not tested")
	}
	socket := filepath.Join(t.TempDir(), "bus")
	cmd := exec.Command(daemon, "--session", "--nofork", "--address=unix:path="+socket)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	server, e := dbus.Connect("unix:path=" + socket)
	for tries := 0; e != nil && tries < 100; tries++ {
		time.Sleep(10 * time.Millisecond)
		server, e = dbus.Connect("unix:path=" + socket)
	}
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	if _, e = server.RequestName(receiverBusName, dbus.NameFlagDoNotQueue); e != nil {
		t.Fatal(e)
	}
	path := dbus.ObjectPath("/org/gnome/ShairportSync")
	properties, e := prop.Export(server, path, prop.Map{
		receiverBusName: {"Active": {Value: true}},
		remoteInterface: {"Available": {Value: false}, "Client": {Value: "fe80::1"}, "AirplayVolume": {Value: float64(-12)}},
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name      string
		modern    bool
		client    string
		want      bool
		connected bool
	}{
		{"live exact client", true, "fe80::1", true, true},
		{"channel closed", false, "fe80::1", false, false},
		{"principal client replaces stale metadata", true, "fe80::2", true, true},
		{"empty client is not a session", true, "", false, false},
	} {
		if e = server.Export(&mockStatusService{test.modern, test.client}, path, receiverBusName); e != nil {
			t.Fatal(e)
		}
		r, e := openReceiver(socket)
		if e != nil {
			t.Fatal(e)
		}
		state, e := r.State(context.Background())
		r.conn.Close()
		if e != nil || state.Available != test.want || state.Connected != test.connected {
			t.Fatalf("%s: available=%v err=%v", test.name, state.Available, e)
		}
		if test.connected && state.Client != test.client {
			t.Fatal("retained classic client overrode the principal event connection")
		}
	}
	// Attaching an output precedes audio and classic Client publication.
	properties.SetMust(receiverBusName, "Active", false)
	properties.SetMust(remoteInterface, "Client", "")
	properties.SetMust(remoteInterface, "Available", true)
	if e = server.Export(&mockStatusService{true, "192.0.2.10"}, path, receiverBusName); e != nil {
		t.Fatal(e)
	}
	r, e := openReceiver(socket)
	if e != nil {
		t.Fatal(e)
	}
	defer r.conn.Close()
	state, e := r.State(context.Background())
	if e != nil || state.Active || !state.Connected || state.Client != "192.0.2.10" {
		t.Fatalf("pre-play connection was lost: %+v %v", state, e)
	}
}
