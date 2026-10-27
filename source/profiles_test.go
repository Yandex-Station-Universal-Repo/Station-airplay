package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonDeathCancelsService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchParent(ctx, cancel, 42, func() int { return 1 })
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("orphaned plugin did not stop")
	}
}

func TestReceiverLogsStayBounded(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "receiver.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &boundedLog{f: f}
	large := []byte(strings.Repeat("x", 300*1024))
	if n, err := w.Write(large); err != nil || n != len(large) {
		t.Fatalf("write contract %d %v", n, err)
	}
	if _, err := w.Write([]byte("new session")); err != nil {
		t.Fatal(err)
	}
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > 128*1024 {
		t.Fatalf("unbounded RAM log: %d", fi.Size())
	}
}

func TestSharedProfilesKeepProvenAudioContract(t *testing.T) {
	for _, tc := range []struct{ model, pcm, abi, icon string }{
		{"yandexstation_2", "plug:dmix_spk", "armhf", "AudioAccessory1,1"},
		{"yandexmini_2", "default", "armhf", "AudioAccessory5,1"},
		{"yandexmini_3_pro", "speakers", "aarch64", "AudioAccessory5,1"},
		{"bergamot", "speakers", "armhf", "AudioAccessory5,1"},
		{"yandexmicro", "plug:dmix_spk", "armhf", "AudioAccessory5,1"},
		{"yandexmini", "softvol", "armhf", "AudioAccessory5,1"},
		{"YandexStationMini", "softvol", "armhf", "AudioAccessory5,1"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			p, ok := profileFor(tc.model)
			if !ok {
				t.Fatal("missing shared profile")
			}
			if p.pcm != tc.pcm || p.receiverABI != tc.abi || !p.quasarCoexist {
				t.Fatalf("wrong ABI/audio contract: %+v", p)
			}
			conf := receiverConfig(p)
			for _, s := range []string{tc.pcm, tc.icon, "port = 5000", "output_rate = 48000", "output_channels = 2", "session_timeout = 60"} {
				if !strings.Contains(conf, s) {
					t.Fatalf("lost known-working setting %q", s)
				}
			}
			if strings.Contains(conf, "hw:0,0") || strings.Contains(conf, "spk_tee") {
				t.Fatal("exclusive/FIFO audio must not replace shared PCM")
			}
		})
	}
	for _, model := range []string{"", "yandexmini_2_unverified", "cucumber", "unknown"} {
		if _, ok := profileFor(model); ok {
			t.Fatalf("model %q guessed instead of rejected", model)
		}
	}
}

func TestLight2SharesMixerAndHoldsDisplay(t *testing.T) {
	for _, model := range []string{"yandexplum", "plum"} {
		p, ok := profileFor(model)
		if !ok || !p.ui || p.leds != 3 || p.pcm != "plug:dmix_spk" || p.receiverABI != "armhf" || !p.quasarCoexist || p.airplayModel != "AudioAccessory5,1" {
			t.Fatalf("%s contract: %+v", model, p)
		}
		conf := receiverConfig(p)
		for _, s := range []string{"spk_tee", "port = 5000", "output_rate = 48000", "AudioAccessory5,1", metadataFIFO, "Yandex Station Light 2 AirPlay"} {
			if !strings.Contains(conf, s) {
				t.Fatalf("%s config lost %q", model, s)
			}
		}
		if strings.Contains(conf, "hw:0,0") || strings.Contains(conf, "plug:dmix_spk") {
			t.Fatal("shared mixer must stay in the private asoundrc, not the receiver device string")
		}
		asound := uiAsound(p.pcm, audioFIFO)
		if !strings.Contains(asound, "plug:dmix_spk") || strings.Contains(asound, "hw:0,0") {
			t.Fatalf("tee slave: %s", asound)
		}
	}
}

func TestChildrenCannotInheritYAMTokenOrPlumLibraries(t *testing.T) {
	env := audioEnv([]string{"PATH=/bin", "YAM_API_TOKEN=private", "YAM_API_URL=http://localhost", "LD_LIBRARY_PATH=/plum/lib", "HOME=/plum", "DBUS_SYSTEM_BUS_ADDRESS=unix:path=/system"}, "/run/station-airplay", "/private/bus")
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"YAM_API_", "private\n", "/plum", "/system"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unsafe inherited environment: %q", joined)
		}
	}
	if !strings.Contains(joined, "DBUS_SYSTEM_BUS_ADDRESS=unix:path=/private/bus") || !strings.Contains(joined, "PATH=/bin") {
		t.Fatal("lost child bus or PATH")
	}
}

func TestStockMDNSHostname(t *testing.T) {
	for model, want := range map[string]string{"yandexplum": "YandexLite-gen2-TEST123", "yandexstation_2": "yandex-station-gen2-TEST123", "yandexmini_3_pro": "Yandex-Station-Mini-3-Pro-TEST123", "bergamot": "YandexMini3-TEST123", "yandexmicro": "yandexmicro-TEST123"} {
		p, _ := profileFor(model)
		got, err := stockHostname(p.mdnsHostPrefix, "root=ubi androidboot.serialno=TEST123 quiet")
		if err != nil || got != want {
			t.Fatalf("hostname %q %v", got, err)
		}
	}
	for _, cmdline := range []string{"", "androidboot.serialno=../bad", "androidboot.serialno=bad\""} {
		if _, err := stockHostname("yandex-station-gen2", cmdline); err == nil {
			t.Fatalf("accepted unsafe serial %q", cmdline)
		}
	}
	// The Mini's maind publishes the Linkplay mv_uuid, not the boot serial.
	p, _ := profileFor("yandexmini")
	if !p.nvramHostID {
		t.Fatal("Mini must take its mDNS host id from Linkplay nvram")
	}
	got, err := nvramHostname(p.mdnsHostPrefix, []byte(`{"Private_PrjName":"YandexStationMini","SSID1":"YandexStationMini_TEST","mv_uuid":"FF00TEST123"}`))
	if err != nil || got != "yandexmini-FF00TEST123" {
		t.Fatalf("nvram hostname %q %v", got, err)
	}
	for _, raw := range []string{"", "{}", `{"mv_uuid":"../bad"}`, `{"mv_uuid":"bad\""}`} {
		if _, err := nvramHostname("yandexmini", []byte(raw)); err == nil {
			t.Fatalf("accepted unsafe nvram %q", raw)
		}
	}
}

func TestMiniUsesFirmwareALSAAndPackagedDBus(t *testing.T) {
	mini, _ := profileFor("yandexmini")
	if got := firmwareALSA(mini); got != "/system/vendor/quasar/libs/libasound.so.2" {
		t.Fatalf("Mini firmware ALSA %q", got)
	}
	bin, args := dbusCommand("/pkg", mini, "dbus-daemon")
	if bin != "/pkg/lib/ld-linux-armhf.so.3" || strings.Join(args, " ") != "--library-path /pkg/lib /pkg/bin/dbus-daemon" {
		t.Fatalf("Mini D-Bus %s %v", bin, args)
	}
	station2, _ := profileFor("yandexstation_2")
	if got := firmwareALSA(station2); got != "/usr/lib/libasound.so.2" {
		t.Fatalf("Station 2 firmware ALSA changed: %q", got)
	}
	if bin, args := dbusCommand("/pkg", station2, "dbus-send"); bin != "/usr/bin/dbus-send" || len(args) != 0 {
		t.Fatalf("Station 2 must keep firmware D-Bus: %s %v", bin, args)
	}
}

func TestClocklessLiteUsesSharedAudio(t *testing.T) {
	for _, model := range []string{"plum", "yandexplum"} {
		p, ok := profileForDevice(yamDevice{Model: model}, false)
		if !ok || p.pcm != "speakers" || !p.quasarCoexist || p.receiverABI != "armhf" || p.leds != 0 {
			t.Fatalf("clockless Lite must share audio without UI: %+v", p)
		}
		p, ok = profileForDevice(yamDevice{Model: model, Features: []string{"display.clock.segment.v1"}}, false)
		if !ok || p.pcm != "plug:dmix_spk" || p.leds != 3 || !p.ui || !p.quasarCoexist {
			t.Fatal("upstream Light 2 display renderer changed")
		}
	}
	if _, ok := profileForDevice(yamDevice{Model: "unknown"}, false); ok {
		t.Fatal("ambiguous hardware must not be guessed")
	}
}

// Captured YAM reports from the Lite pilot include this clock capability.
// Shared audio must therefore be explicit, not inferred from clock absence.
func TestPlumSharedOptInWithReportedClock(t *testing.T) {
	for _, model := range []string{"plum", "yandexplum"} {
		d := yamDevice{Model: model, Architecture: "aarch64", Features: []string{"display.clock.segment.v1", "led.ring.rgb", "led.ring.3", "audio.volume.alsa"}}
		p, ok := profileForDevice(d, true)
		if !ok || p.pcm != "speakers" || !p.quasarCoexist || p.receiverABI != "armhf" || p.leds != 0 {
			t.Fatalf("shared opt-in entered exclusive/UI profile: %+v", p)
		}
		legacy, ok := profileForDevice(d, false)
		if !ok || legacy.pcm != "plug:dmix_spk" || legacy.leds != 3 || !legacy.ui || !legacy.quasarCoexist {
			t.Fatal("upstream clock-equipped Light 2 profile changed")
		}
	}
	if _, ok := profileForDevice(yamDevice{Model: "unknown"}, true); ok {
		t.Fatal("shared mode must not bypass the model guard")
	}
}

func TestControlsDoNotSilentlyLoseTheirLoopToUpstreamDisplay(t *testing.T) {
	clock := yamDevice{Model: "yandexplum", Features: []string{"display.clock.segment.v1"}}
	upstream, ok := profileForDevice(clock, false)
	if !ok || !upstream.ui {
		t.Fatal("missing upstream display mode")
	}
	if err := validateControlRenderer(upstream, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateControlRenderer(upstream, &controlsConfig{}); err == nil || !strings.Contains(err.Error(), "--shared-audio") {
		t.Fatalf("display and controls were combined: %v", err)
	}
	audioOnly, ok := profileForDevice(clock, true)
	if !ok || audioOnly.ui {
		t.Fatal("explicit shared mode retained upstream renderer")
	}
	if err := validateControlRenderer(audioOnly, &controlsConfig{}); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"yandexmini_2", "yandexstation_2", "yandexmicro"} {
		p, ok := profileFor(model)
		if !ok {
			t.Fatal(model)
		}
		if err := validateControlRenderer(p, &controlsConfig{}); err != nil {
			t.Fatal(model, err)
		}
	}
	for _, model := range []string{"yandexmidi", "yandexmini_3_pro", "yandexmini", "YandexStationMini"} {
		p, ok := profileFor(model)
		if !ok {
			t.Fatal(model)
		}
		if err := validateControlRenderer(p, &controlsConfig{}); err == nil {
			t.Fatal("unsupported controls enabled", model)
		}
	}
}
