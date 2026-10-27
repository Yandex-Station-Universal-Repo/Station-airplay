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

type mockRemoteService struct{ calls chan string }

func (s *mockRemoteService) Pause() *dbus.Error { s.calls <- "Pause"; return nil }
func (s *mockRemoteService) SetAirplayVolume(v float64) *dbus.Error {
	if !validDB(v) {
		return dbus.NewError("fixture.Invalid", nil)
	}
	s.calls <- "Volume"
	return nil
}

type mockGainService struct{ calls chan string }

func (s *mockGainService) SetStreamGain(v, lease uint32) *dbus.Error {
	if v > 100 || lease > 5000 {
		return dbus.NewError("fixture.Invalid", nil)
	}
	s.calls <- "Gain"
	return nil
}
func TestPrivateDBusControlWireContract(t *testing.T) {
	daemon, e := exec.LookPath("dbus-daemon")
	if e != nil {
		t.Skip("dbus-daemon unavailable; integration not tested")
	}
	socket := filepath.Join(t.TempDir(), "bus")
	address := "unix:path=" + socket
	cmd := exec.Command(daemon, "--session", "--nofork", "--address="+address)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var server *dbus.Conn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		server, e = dbus.Connect(address)
		if e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	if _, e = server.RequestName(receiverBusName, dbus.NameFlagDoNotQueue); e != nil {
		t.Fatal(e)
	}
	path := dbus.ObjectPath("/org/gnome/ShairportSync")
	_, e = prop.Export(server, path, prop.Map{
		receiverBusName: {"Active": {Value: true}},
		remoteInterface: {"Available": {Value: true}, "Client": {Value: "fixture-sender"}, "AirplayVolume": {Value: float64(-15)}},
	})
	if e != nil {
		t.Fatal(e)
	}
	calls := make(chan string, 8)
	if e = server.Export(&mockRemoteService{calls}, path, remoteInterface); e != nil {
		t.Fatal(e)
	}
	if e = server.Export(&mockGainService{calls}, path, receiverBusName); e != nil {
		t.Fatal(e)
	}
	r, e := openReceiver(socket)
	if e != nil {
		t.Fatal(e)
	}
	defer r.conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, e := r.State(ctx)
	if e != nil || !s.Active || !s.Available || s.Client != "fixture-sender" || s.DB != -15 {
		t.Fatal(s, e)
	}
	if e = r.Command(ctx, "Pause"); e != nil {
		t.Fatal(e)
	}
	if e = r.Volume(ctx, -9); e != nil {
		t.Fatal(e)
	}
	if e = r.Gain(ctx, 20, 4500); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"Pause", "Volume", "Gain"} {
		select {
		case got := <-calls:
			if got != want {
				t.Fatal(got, want)
			}
		case <-ctx.Done():
			t.Fatal("missing D-Bus call")
		}
	}
	if e = r.Command(ctx, "Quit"); e == nil {
		t.Fatal("unrestricted remote method")
	}
	if e = r.Gain(ctx, 101, 1); e == nil {
		t.Fatal("out of range gain")
	}
}
