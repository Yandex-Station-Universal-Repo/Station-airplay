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
// must only become controllable when the receiver confirms its live event
// channel for the same client; mismatched or failed status stays unavailable.
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
	if _, e = prop.Export(server, path, prop.Map{
		receiverBusName: {"Active": {Value: true}},
		remoteInterface: {"Available": {Value: false}, "Client": {Value: "fe80::1"}, "AirplayVolume": {Value: float64(-12)}},
	}); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name   string
		modern bool
		client string
		want   bool
	}{
		{"live exact client", true, "fe80::1", true},
		{"channel closed", false, "fe80::1", false},
		{"different client", true, "fe80::2", false},
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
		if e != nil || state.Available != test.want {
			t.Fatalf("%s: available=%v err=%v", test.name, state.Available, e)
		}
	}
}
