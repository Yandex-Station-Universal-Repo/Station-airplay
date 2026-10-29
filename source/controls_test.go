package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockReceiver struct {
	s        receiverState
	commands []string
	volumes  []float64
	gains    [][2]uint32
	fail     bool
}

func (r *mockReceiver) State(context.Context) (receiverState, error) {
	if r.fail {
		return receiverState{}, errors.New("offline")
	}
	return r.s, nil
}
func (r *mockReceiver) Command(_ context.Context, v string) error {
	r.commands = append(r.commands, v)
	return nil
}
func (r *mockReceiver) Volume(_ context.Context, v float64) error {
	r.volumes = append(r.volumes, v)
	return nil
}
func (r *mockReceiver) Gain(_ context.Context, v, lease uint32) error {
	r.gains = append(r.gains, [2]uint32{v, lease})
	return nil
}

type mockStation struct {
	v      float64
	sets   []int
	frames [][]string
	off    int
	fail   bool
}

func (s *mockStation) GetVolume(context.Context) (float64, error) {
	if s.fail {
		return 0, errors.New("native unavailable")
	}
	return s.v, nil
}
func (s *mockStation) SetVolume(_ context.Context, v int) (float64, error) {
	s.sets = append(s.sets, v)
	s.v = float64(int(math.Round(float64(v)/5)) * 5)
	return s.v, nil
}
func (s *mockStation) Colors(_ context.Context, v []string) error {
	s.frames = append(s.frames, v)
	return nil
}
func (s *mockStation) Off(context.Context) error { s.off++; return nil }
func (s *mockStation) ClaimSpeech(context.Context, string) error {
	if s.fail {
		return errors.New("claim rejected")
	}
	return nil
}
func testControls() (*sharedController, *mockReceiver, *mockStation) {
	// Unrelated control tests start with matching phone/native levels. Volume
	// handoff tests below explicitly exercise differing initial values.
	r := &mockReceiver{s: receiverState{Active: true, Available: true, Client: "mock-sender", DB: percentDB(70)}}
	s := &mockStation{v: 70}
	c := &sharedController{cfg: controlsConfig{Voice: true, NativeVolume: true, Ducking: true, LEDs: true}, receiver: r, station: s, ledCount: 3}
	c.aliceIdle = true
	c.speechFreshUntil = time.Now().Add(time.Hour)
	return c, r, s
}
func TestControlPhraseIsExact(t *testing.T) {
	positive := map[string]string{
		"Алиса, следующий трек": "Next",
		"пауза":            "Pause",
		"продолжи музыку":  "Play",
		"предыдущая песня": "Previous",
		// Owner-reported phrases that Alice used to handle herself.
		"прошлый трек":            "Previous",
		"Алиса, прошлый трек":     "Previous",
		"включи прошлый трек":     "Previous",
		"верни прошлый трек":      "Previous",
		"прошлую песню":           "Previous",
		"дальше":                  "Next",
		"Алиса, дальше":           "Next",
		"далее":                   "Next",
		"пропусти":                "Next",
		"хватит":                  "Pause",
		"хватит музыку":           "Pause",
		"хватит играть":           "Pause",
		"Алиса, хватит":           "Pause",
		"останови":                "Pause",
		"поставь на паузу":        "Pause",
		"включи музыку":           "Play",
		"продолжай":               "Play",
		"стоп, пожалуйста":        "Pause",
		"Алиса, включи следующий": "Next",
	}
	for phrase, want := range positive {
		if got := controlPhrase(phrase); got != want {
			t.Fatalf("%q = %q, want %q", phrase, got, want)
		}
	}
	for _, phrase := range []string{
		"не ставь паузу", "агент следующий трек", "расскажи про следующий трек",
		"стоп таймер", "выключи свет", "следующий трек и включи свет",
		"не хватит", "хватит ли времени", "расскажи про прошлый трек",
		"что дальше", "дальше по плану", "прошлый трек и включи свет",
		"включи свет", "поставь будильник", "какой сегодня день",
	} {
		if got := controlPhrase(phrase); got != "" {
			t.Fatalf("%q = %q, want no command", phrase, got)
		}
	}
}
func TestControlVoiceFinalDedupAndSenderGuard(t *testing.T) {
	c, r, _ := testControls()
	ctx := context.Background()
	now := time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	e := speechEvent{Phase: "recognition", RequestID: "id1", Transcript: "следующий трек"}
	_ = c.speech(ctx, e, now)
	if len(r.commands) != 0 {
		t.Fatal("partial ASR executed")
	}
	e.Final = true
	_ = c.speech(ctx, e, now)
	_ = c.speech(ctx, e, now)
	if len(r.commands) != 1 || r.commands[0] != "Next" {
		t.Fatal(r.commands)
	}
	r.s.Client = "different-sender"
	e.RequestID = "id2"
	if c.speech(ctx, e, now) == nil {
		t.Fatal("sender change not rejected")
	}
	if len(r.commands) != 1 {
		t.Fatal("stale remote command sent")
	}
	r.s.Client = c.client
	e.RequestID = ""
	_ = c.speech(ctx, e, now)
	if len(r.commands) != 1 {
		t.Fatal("unkeyed final executed")
	}
}
func TestControlPausedResumeLease(t *testing.T) {
	c, r, _ := testControls()
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "pause", Transcript: "пауза", Final: true}, now)
	r.s.Active = false
	_ = c.tick(ctx, now.Add(time.Second))
	_ = c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "play", Transcript: "продолжи", Final: true}, now.Add(time.Second))
	if len(r.commands) != 2 || r.commands[1] != "Play" {
		t.Fatal(r.commands)
	}
	_ = c.tick(ctx, now.Add(31*time.Minute))
	if c.active {
		t.Fatal("pause lease never expires")
	}
}
func TestControlVolumePhoneNativeQuantisationNoFeedback(t *testing.T) {
	c, r, s := testControls()
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	if len(s.sets) != 0 || len(r.volumes) != 0 {
		t.Fatal("startup changed volume")
	}
	r.s.DB = -10
	_ = c.tick(ctx, now.Add(time.Second)) // 67 -> native65
	if len(s.sets) != 1 || s.v != 65 {
		t.Fatal(s.sets, s.v)
	}
	_ = c.tick(ctx, now.Add(2*time.Second))
	_ = c.tick(ctx, now.Add(3*time.Second))
	if len(r.volumes) != 0 {
		t.Fatal("quantised phone update echoed back")
	}
	s.v = 80
	_ = c.tick(ctx, now.Add(4*time.Second))
	if len(r.volumes) != 0 {
		t.Fatal("unstable native update sent")
	}
	_ = c.tick(ctx, now.Add(5*time.Second))
	if len(r.volumes) != 1 || r.volumes[0] != -6 {
		t.Fatal(r.volumes)
	}
	_ = c.tick(ctx, now.Add(5500*time.Millisecond))
	if len(s.sets) != 1 {
		t.Fatal("old sender value overwrote pending native request")
	}
	r.s.DB = -6
	_ = c.tick(ctx, now.Add(6*time.Second))
	if len(r.volumes) != 1 || len(s.sets) != 1 {
		t.Fatal("echo loop")
	}
}
func TestControlVolumeRejectsInvalidAndAPIError(t *testing.T) {
	c, r, s := testControls()
	ctx := context.Background()
	now := time.Now()
	for _, v := range []float64{math.NaN(), math.Inf(1), 1, -31, -145} {
		r.s.DB = v
		if c.tick(ctx, now) == nil {
			t.Fatal(v)
		}
	}
	r.s.DB = -15
	s.fail = true
	if c.tick(ctx, now) == nil {
		t.Fatal("native failure ignored")
	}
	if len(s.sets) != 0 || len(r.volumes) != 0 {
		t.Fatal("failure changed volume")
	}
}
func TestControlDuckLeaseAndNativeLEDYield(t *testing.T) {
	c, r, s := testControls()
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	if len(s.frames) != 1 || len(s.frames[0]) != 3 {
		t.Fatal("missing bounded indication")
	}
	_ = c.speech(ctx, speechEvent{Phase: "speaking", RequestID: "reply"}, now)
	if len(r.gains) != 1 || r.gains[0][0] != 20 {
		t.Fatal(r.gains)
	}
	if s.off != 0 {
		t.Fatal("native Alice indication was blanked")
	}
	_ = c.tick(ctx, now.Add(5*time.Second))
	if r.gains[len(r.gains)-1][0] != 100 {
		t.Fatal("duck not restored")
	}
	if len(s.frames) != 1 {
		t.Fatal("LED painted over Alice")
	}
	c.close(ctx)
	if s.off != 0 {
		t.Fatal("shutdown blanked Alice")
	}
}
func TestControlConfigOptInAndNoUnknownFields(t *testing.T) {
	if c, e := loadControls(""); c != nil || e != nil {
		t.Fatal(c, e)
	}
	path := filepath.Join(t.TempDir(), "controls.json")
	for _, s := range []string{`{"voice":true,"typo":true}`, `{"voice":true} {}`, `[]`} {
		_ = os.WriteFile(path, []byte(s), 0600)
		if _, e := loadControls(path); e == nil {
			t.Fatal(s)
		}
	}
	_ = os.WriteFile(path, []byte(`{"voice":true,"native_volume":true}`), 0600)
	if c, e := loadControls(path); e != nil || !c.Voice || !c.NativeVolume {
		t.Fatal(c, e)
	}
}
func TestControlUnknownLEDBackendSkipped(t *testing.T) {
	if supportedLEDCount(yamDevice{Model: "yandexstation_2"}) != 0 {
		t.Fatal("invented framebuffer backend")
	}
	if supportedLEDCount(yamDevice{Features: []string{"led.ring.6", "led.ring.rgb"}}) != 6 {
		t.Fatal("Mini2 declared ring not recognised")
	}
}

func TestControlStateSchemaIdleRestoreAndStaleSnapshot(t *testing.T) {
	c, r, s := testControls()
	ctx := context.Background()
	now := time.Now()
	_ = c.tick(ctx, now)
	_ = c.speech(ctx, speechEvent{Phase: "busy", RequestID: "old", Transcript: "следующий трек", Final: true, InitialSnapshot: true}, now)
	if len(r.commands) != 0 {
		t.Fatal("initial native snapshot replayed")
	}
	_ = c.speech(ctx, speechEvent{Phase: "idle", RequestID: "retained", Transcript: "следующий трек", Final: true}, now.Add(time.Second))
	if len(r.commands) != 0 {
		t.Fatal("retained ASR on IDLE executed")
	}
	if r.gains[len(r.gains)-1][0] != 100 {
		t.Fatal("explicit IDLE did not restore stream gain")
	}
	_ = c.tick(ctx, now.Add(time.Second))
	if len(s.frames) != 2 {
		t.Fatal("IDLE did not return indicator ownership")
	}
	_ = c.speech(ctx, speechEvent{Phase: "unavailable"}, now.Add(2*time.Second))
	_ = c.tick(ctx, now.Add(2*time.Second))
	if len(s.frames) != 2 {
		t.Fatal("missing observer treated as IDLE")
	}
}

func TestMissingControlsConfigDisablesControls(t *testing.T) {
	if c, e := loadControlsOptional(filepath.Join(t.TempDir(), "absent.json")); c != nil || e != nil {
		t.Fatal("missing file must disable controls", c, e)
	}
	if c, e := loadControlsOptional(""); c != nil || e != nil {
		t.Fatal("empty path must disable controls", c, e)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`{"typo":true}`), 0600)
	if _, e := loadControlsOptional(bad); e == nil {
		t.Fatal("malformed configuration must still fail")
	}
}
