package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestYAMRuntimeAPI(t *testing.T) {
	var paths []string
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-YAM-Token") != "secret" {
			t.Error("missing token")
		}
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/device" {
			_ = json.NewEncoder(w).Encode(yamDevice{Model: "yandexplum", Features: []string{"display.clock.segment.v1", "led.ring.rgb", "led.ring.3"}})
			return
		}
		if r.URL.Path == "/led" || r.URL.Path == "/clock" {
			var data map[string]interface{}
			if e := json.NewDecoder(r.Body).Decode(&data); e != nil {
				t.Error(e)
			}
			if r.URL.Path == "/led" && len(data["colors"].([]interface{})) != 3 {
				t.Error("expected three LED positions")
			}
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(204)
	}))
	defer h.Close()
	t.Setenv("YAM_API_URL", h.URL)
	t.Setenv("YAM_API_TOKEN", "secret")
	a, e := newYAMAPI()
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyYAM(a); e != nil {
		t.Fatal(e)
	}
	if e = a.segments(segmentsFor("PAUS", false)); e != nil {
		t.Fatal(e)
	}
	if e = a.ledOff(); e != nil {
		t.Fatal(e)
	}
	if e = a.clockOff(); e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(paths, "\n")
	for _, p := range []string{"GET /device", "POST /led", "POST /clock", "POST /led/off"} {
		if !strings.Contains(joined, p) {
			t.Errorf("missing API call %s", p)
		}
	}
}
func TestRejectNonLoopbackAPI(t *testing.T) {
	t.Setenv("YAM_API_TOKEN", "secret")
	t.Setenv("YAM_API_URL", "http://192.168.1.100:8099/api/runtime/test")
	if _, e := newYAMAPI(); e == nil {
		t.Fatal("accepted non-local YAM token target")
	}
}
func TestAcceptOwnInterfaceAPI(t *testing.T) {
	// BetterYAM passes its LAN listen address; that is still this host.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip(err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			t.Setenv("YAM_API_TOKEN", "secret")
			t.Setenv("YAM_API_URL", "http://"+n.IP.String()+":8099/api/runtime/test")
			if _, e := newYAMAPI(); e != nil {
				t.Fatalf("rejected own address %s: %v", n.IP, e)
			}
			return
		}
	}
	t.Skip("no non-loopback IPv4 interface")
}
func TestMetadataAndAnimations(t *testing.T) {
	s := &airplayState{status: "stopped"}
	now := time.Now()
	s.onMetadata("73736e63", "70766f6c", []byte("-15,0,0,0"), now)
	if s.volume != 50 {
		t.Fatalf("volume %d", s.volume)
	}
	s.onMetadata("73736e63", "70766f6c", []byte("200,0,0,0"), now)
	if s.volume != 50 {
		t.Fatal("unsafe volume accepted")
	}
	s.onMetadata("636f7265", "6d696e6d", []byte("Песня"), now)
	if s.title == "" {
		t.Fatal("missing title metadata")
	}
	s.onMetadata("73736e63", "70626567", nil, now)
	if !s.playing(now) {
		t.Fatal("playing state lost")
	}
	s.onMetadata("73736e63", "70617573", nil, now)
	if s.playing(now) {
		t.Fatal("paused state lost")
	}
	s.onMetadata("73736e63", "70656e64", nil, now)
	if s.title != "" || s.status != "stopped" {
		t.Fatal("end did not clear track")
	}
	for _, c := range s.ringFrame(now.Add(3 * time.Second)) {
		if c != "#000000" {
			t.Errorf("idle LED %s", c)
		}
	}
}
func TestClockSegmentsAndSafety(t *testing.T) {
	segments := segmentsFor("1234", true)
	if segments["DH"] == 0 || segments["DL"] == 0 {
		t.Fatal("missing Plum colon")
	}
	if _, bad := segments["A4"]; bad {
		t.Fatal("wrong digit indices")
	}
	for k := range segments {
		if k != "DH" && k != "DL" && (len(k) != 2 || k[1] < '0' || k[1] > '3') {
			t.Fatalf("bad segment: %s", k)
		}
	}
	s := &airplayState{status: "stopped"}
	when := time.Date(2026, 10, 25, 12, 35, 0, 0, time.UTC)
	text, frame := s.display(when, time.UTC)
	if text != "12:35" || frame != nil {
		t.Fatalf("idle display %q", text)
	}
}
func TestNoHardwareInputOrLEDWrites(t *testing.T) {
	data, e := os.ReadFile("service.go")
	if e != nil {
		t.Fatal(e)
	}
	// Guard against introducing direct hardware access back into the YAM adapter.
	for _, needle := range []string{"\"/dev/input", "\"/sys/class/leds", "aw9610x", "gpio-keys", "keyMicrophone", "keyAction"} {
		if strings.Contains(string(data), needle) && !strings.Contains(needle, "IGNORE") { // mentions in comments acceptable
			// Only fail when used as a program string outside documentation comments.
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, needle) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
					t.Fatalf("hardware access reintroduced: %s", line)
				}
			}
		}
	}
}
func TestRetryWaitForReady(t *testing.T) {
	attempt := 0
	err := waitForReady(context.Background(), "mock", 4*time.Second, func() error {
		attempt++
		if attempt == 1 {
			return errors.New("server not yet up")
		}
		return nil
	})
	if err != nil || attempt != 2 {
		t.Fatalf("wait failed: attempts=%d err=%v", attempt, err)
	}
}

func TestSpectrumDisplay(t *testing.T) {
	s := &airplayState{status: "playing"}
	now := time.Now()
	s.onMetadata("636f7265", "6d696e6d", []byte("Very Long Track"), now)
	// 440 Hz stereo sine wave, 48kHz S16LE
	pcm := make([]byte, 48000*4/4)
	for i := 0; i < len(pcm)/4; i++ {
		v := int16(18000 * math.Sin(2*math.Pi*440*float64(i)/48000))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
	}
	s.audio(pcm, now)
	if s.history[1] <= 0 {
		t.Fatalf("no frequency response: %v", s.history)
	}
	_, frame := s.display(now, time.UTC)
	if len(frame) == 0 {
		t.Fatalf("no visualizer frame: %v", frame)
	}
}

func TestCenteredWaveAndOutwardLag(t *testing.T) {
	s := &airplayState{status: "playing"}
	now := time.Now()
	for i := 0; i < 32; i++ {
		s.wave[i] = 0.1
	}
	s.wavePos = 16
	s.wave[15] = 1.0
	_, frame := s.display(now, time.UTC)
	if frame["G0"] == 0 || frame["G1"] == 0 || frame["G2"] == 0 || frame["G3"] == 0 {
		t.Fatalf("missing center line: %v", frame)
	}
	if frame["A1"] == 0 || frame["D1"] == 0 || frame["A2"] == 0 || frame["D2"] == 0 {
		t.Fatalf("missing paired top/bottom beat: %v", frame)
	}
	if frame["A0"] != 0 || frame["D3"] != 0 {
		t.Fatalf("outer digits must lag inner: %v", frame)
	}
	for k := range frame {
		if k[0] != 'A' && k[0] != 'D' && k[0] != 'G' {
			t.Fatalf("unexpected side segment %q", k)
		}
	}
}
