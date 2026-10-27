package main

// Optional shared controls. All state belongs to this YAM service invocation;
// no log follower, personal paths, extra supervisor or global preload.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

type controlsConfig struct {
	Voice        bool         `json:"voice"`
	NativeVolume bool         `json:"native_volume"`
	Ducking      bool         `json:"ducking"`
	LEDs         bool         `json:"leds"`
	LEDLevel     uint8        `json:"led_level,omitempty"`
	NativePlayer bool         `json:"native_player,omitempty"`
	Group        *groupConfig `json:"group,omitempty"`
}

func loadControls(path string) (*controlsConfig, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16385))
	d.DisallowUnknownFields()
	var c controlsConfig
	if err = d.Decode(&c); err != nil {
		return nil, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing control configuration")
	}
	return &c, nil
}

// loadControlsOptional treats a missing configuration file as "controls
// disabled": the package ships --controls-config, so a host without the file
// must keep working as an audio-only receiver instead of failing to start.
func loadControlsOptional(path string) (*controlsConfig, error) {
	c, err := loadControls(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return c, err
}

type receiverState struct {
	Active, Available bool
	Client            string
	DB                float64
}
type speechEvent struct {
	Phase           string `json:"phase"`
	RequestID       string `json:"request_id"`
	Transcript      string `json:"transcript"`
	Final           bool   `json:"final"`
	InitialSnapshot bool   `json:"initial_snapshot"`
}
type controlReceiver interface {
	State(context.Context) (receiverState, error)
	Command(context.Context, string) error
	Volume(context.Context, float64) error
	Gain(context.Context, uint32, uint32) error
}
type controlStation interface {
	GetVolume(context.Context) (float64, error)
	SetVolume(context.Context, int) (float64, error)
	Colors(context.Context, []string) error
	Off(context.Context) error
	ClaimSpeech(context.Context, string) error
}

// indicationStation is implemented where the ring is not a sysfs colour list.
// Station 2 exposes an 89x1 framebuffer, so the supervisor paints a whole
// frame instead of a list of per-LED colours.
type indicationStation interface {
	Indicate(context.Context, string, uint8) error
}

// controlLeadIns are polite lead-ins that may precede a transport phrase.
// They are stripped one token at a time, and only when a complete command
// remains, so "не ставь паузу" or "расскажи про прошлый трек" stay with Alice.
var controlLeadIns = map[string]bool{
	"алиса": true, "пожалуйста": true, "включи": true, "поставь": true,
	"переключи": true, "верни": true, "давай": true, "мне": true,
}

// controlPhrases is an exact-match table. It is only consulted while an
// AirPlay sender owns the session, so broad words such as "хватит" cannot
// affect Alice outside AirPlay.
var controlPhrases = map[string]string{
	// Pause: the sender keeps its position and the indication stays lit.
	"стоп": "Pause", "стоп музыка": "Pause", "стоп музыку": "Pause",
	"останови": "Pause", "останови музыку": "Pause", "останови трек": "Pause",
	"останови песню": "Pause", "пауза": "Pause", "паузу": "Pause",
	"на паузу": "Pause", "хватит": "Pause", "хватит музыку": "Pause",
	"хватит музыки": "Pause", "хватит играть": "Pause", "хватит петь": "Pause",
	"достаточно": "Pause", "достаточно музыки": "Pause", "прекрати": "Pause",
	"прекрати музыку": "Pause", "прекрати играть": "Pause",
	"выключи музыку": "Pause", "pause": "Pause",
	// Play: resume the current sender item.
	"продолжи": "Play", "продолжи музыку": "Play", "продолжай": "Play",
	"продолжай музыку": "Play", "возобнови": "Play", "возобнови музыку": "Play",
	"играй": "Play", "играй музыку": "Play", "включи музыку": "Play",
	"включи мне музыку": "Play", "play": "Play", "resume": "Play",
	"continue": "Play",
	// Next: skip forward inside the sender's queue.
	"дальше": "Next", "далее": "Next", "следующий": "Next", "следующая": "Next",
	"следующий трек": "Next", "следующая песня": "Next", "следующую песню": "Next",
	"следующий трек пожалуйста": "Next", "пропусти": "Next",
	"пропусти трек": "Next", "пропусти песню": "Next", "next": "Next",
	"скип": "Next", "скипни": "Next",
	// Previous: restart or step back inside the sender's queue.
	"назад": "Previous", "прошлый трек": "Previous", "прошлая песня": "Previous",
	"прошлую песню": "Previous", "предыдущий": "Previous", "предыдущая": "Previous",
	"предыдущий трек": "Previous", "предыдущая песня": "Previous",
	"предыдущую песню": "Previous", "верни трек": "Previous",
	"верни песню": "Previous", "previous": "Previous", "prev": "Previous",
	"back": "Previous",
}

// normalizePhrase lowercases a transcript and turns punctuation into token
// separators, so "Алиса, прошлый трек!" and "прошлый трек" compare equal.
func normalizePhrase(phrase string) []string {
	phrase = strings.ToLower(phrase)
	phrase = strings.Map(func(r rune) rune {
		switch r {
		case ',', '.', '!', '?', ';', ':', '—', '-', '"', '«', '»':
			return ' '
		}
		return r
	}, phrase)
	return strings.Fields(phrase)
}

func controlPhrase(phrase string) string {
	fields := normalizePhrase(phrase)
	if command := controlPhrases[strings.Join(fields, " ")]; command != "" {
		return command
	}
	for len(fields) > 1 && controlLeadIns[fields[0]] {
		fields = fields[1:]
		if command := controlPhrases[strings.Join(fields, " ")]; command != "" {
			return command
		}
	}
	for len(fields) > 1 && controlLeadIns[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
		if command := controlPhrases[strings.Join(fields, " ")]; command != "" {
			return command
		}
	}
	return ""
}

func validDB(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && (v == -144 || v >= -30 && v <= 0)
}
func dbPercent(v float64) int {
	if v == -144 {
		return 0
	}
	return int(math.Round((v + 30) * 100 / 30))
}
func percentDB(v int) float64 {
	if v == 0 {
		return -144
	}
	return float64(v)*30/100 - 30
}
func nativePercent(v float64) (int, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
		return 0, errors.New("invalid native volume")
	}
	return int(math.Round(v/5)) * 5, nil
}

type sharedController struct {
	cfg                         controlsConfig
	receiver                    controlReceiver
	station                     controlStation
	ledCount                    int
	active                      bool
	client                      string
	pausedUntil                 time.Time
	duckUntil                   time.Time
	ledOwned, ledSuspended      bool
	ledState                    string
	ledRefreshAt, ledQuietUntil time.Time
	observedNative              int
	nativeObserved              bool
	aliceIdle                   bool
	speechFreshUntil            time.Time
	volumeReady                 bool
	lastDB                      float64
	lastNative                  int
	pendingNative               int
	remotePending               bool
	remoteOldDB                 float64
	remoteDeadline              time.Time
	seen                        map[string]time.Time
	broadcast                   func(context.Context, int) error
	native                      nativePauser
	nativeArmUntil              time.Time
	nativeNextAt                time.Time
}

func (c *sharedController) speech(ctx context.Context, e speechEvent, now time.Time) error {
	// Enum names come from the public Quasar AliceState schema. A lost IPC
	// observer is UNKNOWN, never fabricated IDLE. Gain also expires in receiver.
	switch e.Phase {
	case "idle":
		c.aliceIdle = true
		c.speechFreshUntil = now.Add(3 * time.Second)
		c.ledSuspended = false
		if c.cfg.Ducking && !c.duckUntil.IsZero() {
			c.duckUntil = time.Time{}
			return c.receiver.Gain(ctx, 100, 0)
		}
		return nil // ignore retained final ASR on an IDLE snapshot.
	case "listening", "busy", "speaking", "shazam", "recognition":
		c.aliceIdle = false
		c.speechFreshUntil = now.Add(3 * time.Second)
	default:
		c.aliceIdle = false
		c.speechFreshUntil = time.Time{}
		c.ledOwned = false
		c.ledState = ""
		c.ledSuspended = true
		return nil
	}
	if !c.active {
		return nil
	}
	c.ledSuspended = true
	c.ledOwned = false // do NOT send Off over Alice's freshly acquired indication.
	c.ledState = ""
	if c.cfg.Ducking {
		c.duckUntil = now.Add(4 * time.Second)
		if err := c.receiver.Gain(ctx, 20, 4500); err != nil {
			return err
		}
	}
	if e.InitialSnapshot && e.RequestID != "" {
		if c.seen == nil {
			c.seen = make(map[string]time.Time)
		}
		c.seen[e.RequestID] = now
		return nil
	}
	if !c.cfg.Voice || !e.Final || e.RequestID == "" {
		return nil
	}
	command := controlPhrase(e.Transcript)
	if command == "" {
		return nil
	}
	if c.seen == nil {
		c.seen = make(map[string]time.Time)
	}
	for id, at := range c.seen {
		if now.Sub(at) > 30*time.Second {
			delete(c.seen, id)
		}
	}
	if _, duplicate := c.seen[e.RequestID]; duplicate {
		return nil
	}
	if len(c.seen) >= 128 {
		return errors.New("speech deduplication capacity exceeded")
	}
	s, err := c.receiver.State(ctx)
	if err != nil {
		return err
	}
	if !s.Available || s.Client == "" || s.Client != c.client {
		return errors.New("sender remote is unavailable or changed")
	}
	c.seen[e.RequestID] = now // don't replay an ambiguous/timed-out remote command.
	if err = c.station.ClaimSpeech(ctx, e.RequestID); err != nil {
		return err
	}
	if err = c.receiver.Command(ctx, command); err != nil {
		return err
	}
	if command == "Pause" {
		c.pausedUntil = now.Add(30 * time.Minute)
	} else {
		// A resume/skip ends the paused window; otherwise a stale window would
		// keep the indication (and the session) alive after the sender leaves.
		c.pausedUntil = time.Time{}
	}
	if c.cfg.NativePlayer && c.native != nil && command != "Pause" {
		// Alice also acts on "continue"/"next" herself and can restart her own
		// player next to AirPlay. Arm a bounded handover that pauses it as soon
		// as she is idle again; never while she is listening or speaking.
		c.nativeArmUntil = now.Add(12 * time.Second)
		c.nativeNextAt = time.Time{}
	}
	return nil
}

func (c *sharedController) tick(ctx context.Context, now time.Time) error {
	s, err := c.receiver.State(ctx)
	if err != nil {
		c.volumeReady = false
		c.ledOwned = false
		c.ledState = ""
		c.ledSuspended = true
		return err
	}
	if !validDB(s.DB) {
		return errors.New("invalid AirPlay volume")
	}
	if s.Client != c.client {
		// A changed sender must never receive a stale resume or volume request.
		c.client = s.Client
		c.pausedUntil = time.Time{}
		c.volumeReady = false
		c.remotePending = false
		c.nativeObserved = false
		c.seen = nil
		c.ledSuspended = false
		c.ledState = ""
		c.releaseNative()
	}
	if !s.Available || s.Client == "" {
		// The sender's control channel is gone: a remembered pause must not keep
		// the indication lit after AirPlay has left.
		c.pausedUntil = time.Time{}
	}
	wasActive := c.active
	c.active = s.Active || s.Available && s.Client != "" && now.Before(c.pausedUntil)
	if !c.active {
		c.volumeReady = false
		c.ledSuspended = false
		c.releaseNative()
		if c.cfg.Ducking && !c.duckUntil.IsZero() {
			c.duckUntil = time.Time{}
			if err = c.receiver.Gain(ctx, 100, 0); err != nil {
				return err
			}
		}
		if c.ledOwned {
			c.ledOwned = false
			c.ledState = ""
			return c.station.Off(ctx)
		}
		return nil
	}
	if !wasActive {
		c.volumeReady = false
	}
	if c.cfg.Ducking && now.Before(c.duckUntil) && now.Before(c.speechFreshUntil) {
		if err = c.receiver.Gain(ctx, 20, 4500); err != nil {
			return err
		}
	}
	if c.cfg.Ducking && !c.duckUntil.IsZero() && !now.Before(c.duckUntil) {
		if err = c.receiver.Gain(ctx, 100, 0); err != nil {
			return err
		}
		c.duckUntil = time.Time{}
	}
	var nativeErr error
	if c.cfg.NativePlayer && c.native != nil && !c.nativeArmUntil.IsZero() {
		switch {
		case !now.Before(c.nativeArmUntil):
			c.releaseNative()
		// The bounded window is the freshness proof. Alice's own player can
		// start several seconds after the utterance, so a stale speech event
		// must not hide it: aliceIdle already excludes listening/speaking.
		case c.aliceIdle && !now.Before(c.nativeNextAt):
			c.nativeNextAt = now.Add(time.Second)
			if nativeErr = c.native.Pause(ctx); nativeErr != nil {
				c.releaseNative()
			}
		}
	}
	var volumeErr error
	if c.cfg.NativeVolume {
		volumeErr = c.syncVolume(ctx, s, now)
	}
	// Sender feedback must not gate unrelated indication. Repaint slowly after
	// native volume has settled: its overlay can replace our one-shot frame.
	if c.cfg.LEDs && c.ledCount > 0 && c.aliceIdle && now.Before(c.speechFreshUntil) && !c.ledSuspended && !now.Before(c.ledQuietUntil) {
		state := "playing"
		if now.Before(c.pausedUntil) {
			state = "paused"
		}
		level := c.cfg.LEDLevel
		if level == 0 {
			if _, ok := c.station.(indicationStation); ok {
				level = 200 // Station 2's previous playback frame was 0xcb
			} else {
				level = 16 // retain the original opt-in default
			}
		}
		if ind, ok := c.station.(indicationStation); ok {
			// Framebuffer rings paint a whole frame. Repaint on a state change
			// and slowly afterwards in case Alice redrew her own indication.
			if !c.ledOwned || c.ledState != state || !now.Before(c.ledRefreshAt) {
				if err = ind.Indicate(ctx, state, level); err != nil {
					return errors.Join(volumeErr, err)
				}
				c.ledOwned, c.ledState = true, state
				c.ledRefreshAt = now.Add(3 * time.Second)
			}
		} else if !c.ledOwned || !now.Before(c.ledRefreshAt) {
			colors := make([]string, c.ledCount)
			for i := range colors {
				colors[i] = fmt.Sprintf("#00%02x%02x", level, level/2)
			}
			if err = c.station.Colors(ctx, colors); err != nil {
				return errors.Join(volumeErr, err)
			}
			c.ledOwned = true
			c.ledRefreshAt = now.Add(3 * time.Second)
		}
	}
	return errors.Join(nativeErr, volumeErr)
}

// releaseNative ends a handover and terminates the SDK session; it is called
// on expiry, sender change, inactivity and shutdown so no CLI process remains.
func (c *sharedController) releaseNative() {
	c.nativeArmUntil = time.Time{}
	c.nativeNextAt = time.Time{}
	if c.native != nil {
		c.native.Stop()
	}
}

func (c *sharedController) syncVolume(ctx context.Context, s receiverState, now time.Time) error {
	v, err := c.station.GetVolume(ctx)
	if err != nil {
		c.volumeReady = false
		c.ledQuietUntil = now.Add(3 * time.Second)
		return err
	}
	native, err := nativePercent(v)
	if err != nil {
		c.volumeReady = false
		return err
	}
	if c.nativeObserved && native != c.observedNative {
		c.ledQuietUntil = now.Add(3 * time.Second)
	}
	c.nativeObserved = true
	c.observedNative = native
	if c.remotePending {
		if math.Abs(s.DB-c.lastDB) <= 0.01 {
			c.remotePending = false
		} else if math.Abs(s.DB-c.remoteOldDB) <= 0.01 && now.Before(c.remoteDeadline) {
			return nil
		} else if !now.Before(c.remoteDeadline) {
			c.remotePending = false
			c.volumeReady = false
			return errors.New("sender volume acknowledgement timed out")
		} else {
			c.remotePending = false
		} // A genuinely new phone-side change wins.
	}
	if !c.volumeReady {
		// Never turn a new stream up to a remembered handset volume. Native
		// volume wins initially; subsequent sender events have a baseline.
		c.lastDB = s.DB
		c.lastNative = native
		c.pendingNative = -1
		c.volumeReady = true
	} else if math.Abs(s.DB-c.lastDB) > 0.01 {
		actual, err := c.station.SetVolume(ctx, dbPercent(s.DB))
		if err != nil {
			c.volumeReady = false
			return err
		}
		c.lastNative, err = nativePercent(actual)
		if err != nil {
			c.volumeReady = false
			return err
		}
		c.lastDB = s.DB
		c.pendingNative = -1
		c.ledQuietUntil = now.Add(3 * time.Second)
	} else if native != c.lastNative && !now.Before(c.duckUntil) {
		// Two stable observations suppress native transition/quantisation noise.
		if c.pendingNative == native {
			if !s.Available {
				return nil
			}
			if err = c.receiver.Volume(ctx, percentDB(native)); err != nil {
				return err
			}
			c.remotePending = true
			c.remoteOldDB = s.DB
			c.remoteDeadline = now.Add(3 * time.Second)
			c.lastNative = native
			c.lastDB = percentDB(native)
			c.pendingNative = -1
			if c.broadcast != nil {
				if err = c.broadcast(ctx, native); err != nil {
					return err
				}
			}
		} else {
			c.pendingNative = native
		}
	} else {
		c.pendingNative = -1
	}
	return nil
}

// Stop must not restore Master, restart Alice, touch clocks, or paint over Alice.
func (c *sharedController) close(ctx context.Context) {
	if c.cfg.Ducking {
		_ = c.receiver.Gain(ctx, 100, 0)
	}
	if c.ledOwned {
		_ = c.station.Off(ctx)
		c.ledOwned = false
		c.ledState = ""
	}
	c.releaseNative()
}

func supportedLEDCount(d yamDevice) int {
	// Only documented YAM ring capabilities, never a guessed Station2 framebuffer.
	if !feature(d, "led.ring.rgb") {
		return 0
	}
	for _, n := range []int{3, 6, 12, 24} {
		if feature(d, fmt.Sprintf("led.ring.%d", n)) {
			return n
		}
	}
	return 0
}
