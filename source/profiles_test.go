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
		{"yandexmini_3_pro", "speakers", "aarch64", "AudioAccessory5,1"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			p, err := sharedProfileFor(yamDevice{Model: tc.model, Architecture: "aarch64"})
			if err != nil {
				t.Fatal(err)
			}
			if p.PCM != tc.pcm || p.ReceiverABI != tc.abi {
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
	for _, model := range []string{"", "yandexmidi", "cucumber", "unknown", "yandexplum"} {
		if _, err := sharedProfileFor(yamDevice{Model: model}); err == nil {
			t.Fatalf("model %q guessed instead of rejected", model)
		}
	}
}

func TestChildrenCannotInheritYAMTokenOrPlumLibraries(t *testing.T) {
	env := audioEnv([]string{"PATH=/bin", "YAM_API_TOKEN=private", "YAM_API_URL=http://localhost", "LD_LIBRARY_PATH=/plum/lib", "HOME=/plum", "DBUS_SYSTEM_BUS_ADDRESS=unix:path=/system"}, "/private/bus")
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
	for model, want := range map[string]string{"yandexstation_2": "yandex-station-gen2-TEST123", "yandexmini_3_pro": "Yandex-Station-Mini-3-Pro-TEST123"} {
		got, err := sharedHostname(model, "root=ubi androidboot.serialno=TEST123 quiet")
		if err != nil || got != want {
			t.Fatalf("hostname %q %v", got, err)
		}
	}
	for _, cmdline := range []string{"", "androidboot.serialno=../bad", "androidboot.serialno=bad\""} {
		if _, err := sharedHostname("yandexstation_2", cmdline); err == nil {
			t.Fatalf("accepted unsafe serial %q", cmdline)
		}
	}
}
