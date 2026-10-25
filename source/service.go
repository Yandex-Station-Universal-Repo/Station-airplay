// Plum AirPlay for YAM. Copyright (c) 2026, GPL-3.0-or-later.
// Audio/metadata originate from the verified Shairport Sync 4.3.7 Plum build.
// All LED/clock rendering goes through YAM's token-authenticated runtime API.
// This executable never opens /dev/input or /sys/class/leds.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	version      = "Station airplay 1.3.1"
	audioFIFO    = "/tmp/sps.fifo"
	metadataFIFO = "/tmp/shairport-sync-ap2-metadata"
	brightness   = 170
)

type yamAPI struct {
	base, token string
	client      *http.Client
}
type yamDevice struct {
	Model        string   `json:"model"`
	Architecture string   `json:"architecture"`
	Features     []string `json:"features"`
}

func newYAMAPI() (*yamAPI, error) {
	raw, token := os.Getenv("YAM_API_URL"), os.Getenv("YAM_API_TOKEN")
	if raw == "" || token == "" {
		return nil, errors.New("missing YAM_API_URL or YAM_API_TOKEN: start through yam enable/run, not directly")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		return nil, errors.New("YAM_API_URL must be a loopback HTTP address")
	}
	return &yamAPI{base: strings.TrimRight(raw, "/"), token: token, client: &http.Client{Timeout: 1200 * time.Millisecond}}, nil
}
func (a *yamAPI) request(method, path string, payload interface{}, dest interface{}) error {
	var body io.Reader
	if payload != nil {
		j, e := json.Marshal(payload)
		if e != nil {
			return e
		}
		body = bytes.NewReader(j)
	}
	req, e := http.NewRequest(method, a.base+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("X-YAM-Token", a.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := a.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("YAM %s HTTP %d: %s", path, res.StatusCode, string(b))
	}
	if dest != nil {
		return json.NewDecoder(res.Body).Decode(dest)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}
func (a *yamAPI) device() (yamDevice, error) {
	var d yamDevice
	err := a.request("GET", "/device", nil, &d)
	return d, err
}
func (a *yamAPI) leds(colors [3]string) error {
	return a.request("POST", "/led", map[string]interface{}{"colors": colors[:]}, nil)
}
func (a *yamAPI) ledOff() error { return a.request("POST", "/led/off", map[string]interface{}{}, nil) }
func (a *yamAPI) time(s string) error {
	return a.request("POST", "/clock", map[string]interface{}{"time": s, "brightness": brightness}, nil)
}
func (a *yamAPI) segments(seg map[string]uint8) error {
	return a.request("POST", "/clock", map[string]interface{}{"segments": seg}, nil)
}
func (a *yamAPI) clockOff() error {
	return a.request("POST", "/clock", map[string]interface{}{"off": true}, nil)
}
func feature(d yamDevice, name string) bool {
	for _, f := range d.Features {
		if f == name {
			return true
		}
	}
	return false
}

func readOldName() string {
	return "Я.Станция"
}
func hardwareID() (string, error) {
	for _, dev := range []string{"wlan0", "eth0"} {
		raw, e := os.ReadFile("/sys/class/net/" + dev + "/address")
		if e != nil {
			continue
		}
		h := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(string(raw)), ":", ""))
		if len(h) != 12 {
			continue
		}
		if _, e = strconv.ParseUint(h, 16, 64); e == nil {
			return h, nil
		}
	}
	return "", errors.New("cannot read MAC address from wlan0 or eth0")
}
func createConfigs(root string) (string, string, error) {
	mac, e := hardwareID()
	if e != nil {
		return "", "", e
	}
	st := filepath.Join(root, "state")
	if e = os.MkdirAll(st, 0700); e != nil {
		return "", "", e
	}
	// Identical ALSA FIFO tee as stable Plum 1.1.3, but no hardware UI process.
	out := "sps_out"
	if b, e := os.ReadFile("/etc/asound.conf"); e == nil && strings.Contains(string(b), "pcm.speakers") {
		out = "speakers"
	}
	alsa := fmt.Sprintf(`pcm.sps_out {
 type plug
 slave { pcm "hw:0,0"; rate 48000 }
}
pcm.spk_file {
 type file
 slave.pcm "%s"
 file "%s"
 format "raw"
 perm 0666
}
pcm.spk_tee {
 type plug
 slave.pcm "spk_file"
}
`, out, audioFIFO)
	name := readOldName()
	conf := fmt.Sprintf(`general = {
 name = "%s";
 output_backend = "alsa";
 mdns_backend = "avahi";
 drift_tolerance_in_seconds = 0.005;
 ignore_volume_control = "no";
 volume_range_db = 60;
 default_airplay_volume = -30.0;
 airplay_device_id = 0x%sL;
};
alsa = { output_device = "spk_tee"; output_format = "S16"; use_mmap_if_available = "no"; };
sessioncontrol = { allow_session_interruption = "yes"; };
diagnostics = { log_verbosity = 0; };
metadata = { enabled = "yes"; include_cover_art = "no"; pipe_name = "%s"; };
`, name, mac, metadataFIFO)
	if e = os.WriteFile(filepath.Join(st, ".asoundrc"), []byte(alsa), 0600); e != nil {
		return "", "", e
	}
	p := filepath.Join(st, "shairport.conf")
	if e = os.WriteFile(p, []byte(conf), 0600); e != nil {
		return "", "", e
	}
	return st, p, nil
}
func ensureFIFO(path string) (int, error) {
	fi, e := os.Lstat(path)
	if e == nil {
		if fi.Mode()&os.ModeNamedPipe == 0 || fi.Mode()&os.ModeSymlink != 0 {
			return -1, fmt.Errorf("unsafe existing FIFO path %s", path)
		}
	} else if os.IsNotExist(e) {
		if e = syscall.Mkfifo(path, 0600); e != nil && e != syscall.EEXIST {
			return -1, e
		}
	} else {
		return -1, e
	}
	fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	return fd, e
}
func processExists(program string) bool {
	ents, e := os.ReadDir("/proc")
	if e != nil {
		return false
	}
	for _, ent := range ents {
		if _, e := strconv.Atoi(ent.Name()); e != nil {
			continue
		}
		cmd, e := os.ReadFile("/proc/" + ent.Name() + "/cmdline")
		if e != nil || len(cmd) == 0 {
			continue
		}
		// Private libc loaders keep their own argv[0] in /proc/cmdline.
		for _, part := range bytes.Split(cmd, []byte{0}) {
			if filepath.Base(string(part)) == program {
				return true
			}
		}
	}
	return false
}
func preflight() error {
	for _, program := range []string{"shairport-sync-ap2", "airplayd", "nqptp"} {
		if processExists(program) {
			return fmt.Errorf("legacy %s is still running; first run: adb shell sh /data/shairport/airplay.sh stop", program)
		}
	}
	if processExists("quasar_launcher2") || processExists("maind") {
		return errors.New("Quasar/Alice is running and may own ALSA. Stop it using the existing AirPlay-only procedure first")
	}
	ln, e := net.Listen("tcp4", ":7000")
	if e != nil {
		return fmt.Errorf("RTSP port 7000 occupied: %w", e)
	}
	_ = ln.Close()
	return nil
}

type child struct {
	name string
	cmd  *exec.Cmd
}
type exitInfo struct {
	name string
	err  error
}

func startChild(ctx context.Context, bin string, args []string, env []string) (*child, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(bin), err)
	}
	fmt.Printf("started %s pid=%d\n", filepath.Base(bin), cmd.Process.Pid)
	return &child{name: filepath.Base(bin), cmd: cmd}, nil
}
func stopChildren(children []*child) {
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		if c != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	// Never wait indefinitely: YAM may stop the parent with SIGKILL.
	time.Sleep(350 * time.Millisecond)
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		if c != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	}
}

// The only media metadata reader in this package. There is intentionally no /dev/input reader.
type airplayState struct {
	meta   metadataParser
	status string
	title  string

	volume        int
	volumeUntil   time.Time
	progress      progress
	lastAudio     time.Time
	level         float64
	history       [4]float64
	samples       int
	offset        int
	pcmWindow     [256]float64
	pcmCount      int
	wave          [32]float64
	wavePos       int
	waveAverageDB float64
	waveSmooth    float64
	waveInit      bool
	pcmFrames     uint64
}

func (s *airplayState) onMetadata(typ, code string, raw []byte, now time.Time) {
	if typ == "73736e63" {
		switch code {
		case "70766f6c": // ssnc/pvol: confirmed software volume
			parts := strings.Split(string(raw), ",")
			if len(parts) != 4 {
				return
			}
			for _, p := range parts {
				f, e := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if e != nil || math.IsNaN(f) || math.IsInf(f, 0) {
					return
				}
			}
			v, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			if v != -144 && (v < -30 || v > 0) {
				return
			}
			n := 0
			if v != -144 {
				n = int(math.Round((v + 30) * 100 / 30))
			}
			if n < 0 {
				n = 0
			}
			if n > 100 {
				n = 100
			}
			s.volume = n
			s.volumeUntil = now.Add(1700 * time.Millisecond)
		case "70626567", "70666c73", "7072736d", "70726573": // pbeg, pfls, prsm, pres
			s.status = "playing"
			s.progress.resume(now)
		case "70617573": // paus
			s.status = "paused"
			s.progress.pause(now)
		case "70656e64": // pend
			s.status = "stopped"
			s.title = ""
			s.progress = progress{}
		case "70726772": // prgr RTP progress
			_ = s.progress.update(string(raw), now)
		}
	} else if typ == "636f7265" && code == "6d696e6d" { // core/minm title
		title := strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
		if title != "" && utf8.RuneCountInString(title) <= 250 && title != s.title {
			s.title = title
		}
	}
}

// Four independent audio-frequency bands measured from the actual PCM stream.
// Decimate 48 kHz stereo to 12 kHz mono, then analyze 256-sample windows.
func (s *airplayState) analyzeWindow() {
	frequencies := [4]float64{95, 330, 1050, 3100}
	for band, hz := range frequencies {
		omega := 2 * math.Pi * hz / 12000
		coeff := 2 * math.Cos(omega)
		var q0, q1, q2 float64
		for _, sample := range s.pcmWindow {
			q0 = sample + coeff*q1 - q2
			q2, q1 = q1, q0
		}
		power := q1*q1 + q2*q2 - coeff*q1*q2
		if power < 0 {
			power = 0
		}
		magnitude := math.Sqrt(power) / 128.0
		// Logarithmic response keeps quiet passages visible without constant full bars.
		level := (20*math.Log10(math.Max(magnitude, 1e-6)) + 48) / 42
		if level < 0 {
			level = 0
		}
		if level > 1 {
			level = 1
		}
		if level > s.history[band] {
			s.history[band] = .4*s.history[band] + .6*level
		} else {
			s.history[band] = .78*s.history[band] + .22*level
		}
	}
	s.level = (s.history[0] + s.history[1] + s.history[2] + s.history[3]) * .25
	// Keep the original Plum waveform independent of the four-band LED spectrum.
	// The historical wave moves from the two center digits toward the edges.
	var sum float64
	for _, x := range s.pcmWindow {
		sum += x * x
	}
	rms := math.Sqrt(sum / float64(len(s.pcmWindow)))
	db := -100.0
	if rms > 1e-5 {
		db = 20 * math.Log10(rms)
	}
	waveLevel := 0.0
	if db > -58 {
		if !s.waveInit {
			s.waveAverageDB = db
			s.waveInit = true
		}
		s.waveAverageDB += (db - s.waveAverageDB) * .03
		waveLevel = .5 + (db-s.waveAverageDB)/11
	}
	waveLevel = math.Max(0, math.Min(1, waveLevel))
	if waveLevel >= s.waveSmooth {
		s.waveSmooth = waveLevel
	} else {
		s.waveSmooth = .8*s.waveSmooth + .2*waveLevel
	}
	s.wave[s.wavePos%len(s.wave)] = s.waveSmooth
	s.wavePos++
}
func (s *airplayState) audio(pcm []byte, now time.Time) {
	// S16LE interleaved stereo at 48kHz. Downsample by four.
	for i := 0; i+15 < len(pcm); i += 16 {
		l := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		r := float64(int16(binary.LittleEndian.Uint16(pcm[i+2:])))
		s.pcmWindow[s.pcmCount] = (l + r) / 65536
		s.pcmCount++
		if s.pcmCount == len(s.pcmWindow) {
			s.analyzeWindow()
			s.pcmCount = 0
		}
	}
	if len(pcm) >= 16 {
		s.lastAudio = now
		s.pcmFrames += uint64(len(pcm) / 4)
	}
}
func (s *airplayState) playing(now time.Time) bool {
	return s.status == "playing" || (s.status != "paused" && now.Sub(s.lastAudio) < 1800*time.Millisecond)
}
func (s *airplayState) rotate() {
	// Frequency bands are updated by PCM analysis; do not shift them like a marquee.
	s.offset++
}
func hsv(h, v float64) string {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	c := v * .94
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	return fmt.Sprintf("#%02X%02X%02X", int(r*255), int(g*255), int(b*255))
}
func (s *airplayState) ringFrame(now time.Time) [3]string {
	var frame [3]string
	if now.Before(s.volumeUntil) {
		for i := 0; i < 3; i++ {
			level := float64(s.volume)*3/100 - float64(i)
			if level < 0 {
				level = 0
			}
			if level > 1 {
				level = 1
			}
			frame[i] = hsv(120-float64(s.volume)*1.2, level*.8)
		}
		return frame
	}
	if s.status == "paused" {
		return [3]string{"#061425", "#082036", "#061425"}
	}
	if !s.playing(now) {
		return [3]string{"#000000", "#000000", "#000000"}
	}
	for i := 0; i < 3; i++ {
		lv := s.history[i]
		v := .055 + math.Pow(lv, 1.3)*.9
		frame[i] = hsv(float64(s.offset)*10+float64(i)*120+190, v)
	}
	return frame
}

// The Plum has 4 positions 0..3, segments A..G plus DH and DL colon.
var digitFont = map[rune]byte{
	'0': 0x3f, '1': 0x06, '2': 0x5b, '3': 0x4f, '4': 0x66, '5': 0x6d, '6': 0x7d, '7': 0x07, '8': 0x7f, '9': 0x6f,
	'A': 0x77, 'B': 0x7c, 'C': 0x39, 'D': 0x5e, 'E': 0x79, 'F': 0x71, 'G': 0x3d, 'H': 0x76, 'I': 0x06,
	'J': 0x1e, 'L': 0x38, 'N': 0x54, 'O': 0x5c, 'P': 0x73, 'R': 0x50, 'S': 0x6d, 'T': 0x78, 'U': 0x3e, 'Y': 0x6e,
	'-': 0x40, '_': 0x08, ' ': 0,
}

func segmentsFor(text string, colon bool) map[string]uint8 {
	runes := []rune(strings.ToUpper(text))
	for len(runes) < 4 {
		runes = append(runes, ' ')
	}
	result := map[string]uint8{}
	for i := 0; i < 4; i++ {
		mask := digitFont[runes[i]]
		for j := 0; j < 7; j++ {
			if mask&(1<<uint(j)) != 0 {
				result[fmt.Sprintf("%c%d", 'A'+j, i)] = brightness
			}
		}
	}
	if colon {
		result["DH"] = brightness
		result["DL"] = brightness
	}
	return result
}
func (s *airplayState) display(now time.Time, tz *time.Location) (string, map[string]uint8) {
	if now.Before(s.volumeUntil) {
		return "", segmentsFor(fmt.Sprintf("%4d", s.volume), false)
	}
	if s.status == "paused" {
		return "", segmentsFor("PAUS", false)
	}
	if s.playing(now) {
		result := map[string]uint8{}
		// Original Plum/Alice wave: horizontal G midlines are always present,
		// while A (top) and D (bottom) expand symmetrically on strong beats.
		// Inner digits react first, then the outer digits -> outward ripple.
		for digit, lag := range [4]int{3, 0, 0, 3} {
			idx := (s.wavePos - 1 - lag + len(s.wave)*2) % len(s.wave)
			lv := s.wave[idx]
			mid := uint8(math.Round(6 + lv*164))
			result[fmt.Sprintf("G%d", digit)] = mid
			peak := math.Max(0, math.Min(1, (lv-.45)/.55))
			v := uint8(math.Round(peak * peak * float64(brightness)))
			if v > 0 {
				result[fmt.Sprintf("A%d", digit)] = v
				result[fmt.Sprintf("D%d", digit)] = v
			}
		}
		return "", result
	}
	return now.In(tz).Format("15:04"), nil
}
func clockZone() *time.Location {
	raw := strings.TrimSpace(os.Getenv("PLUM_TZ"))
	if raw == "" {
		if b, e := os.ReadFile("/data/shairport/tz"); e == nil {
			raw = strings.TrimSpace(string(b))
		}
	}
	if raw == "" {
		raw = "MSK-3"
	}
	i := 0
	for i < len(raw) && ((raw[i] >= 'A' && raw[i] <= 'Z') || (raw[i] >= 'a' && raw[i] <= 'z')) {
		i++
	}
	if i >= 3 && i < len(raw) {
		if n, e := strconv.Atoi(raw[i:]); e == nil && n >= -14 && n <= 14 {
			return time.FixedZone(raw[:i], -n*3600)
		}
	}
	if loc, e := time.LoadLocation(raw); e == nil {
		return loc
	}
	return time.Local
}
func readFD(fd int, visit func([]byte)) error {
	buf := make([]byte, 16384)
	for i := 0; i < 16; i++ {
		n, e := syscall.Read(fd, buf)
		if e == syscall.EAGAIN || e == syscall.EWOULDBLOCK || e == syscall.EINTR {
			return nil
		}
		if e != nil {
			return e
		}
		if n == 0 {
			return nil
		}
		visit(buf[:n])
	}
	return nil
}
func logAPIError(name string, e error, last *time.Time) {
	if e != nil && time.Since(*last) > 10*time.Second {
		fmt.Fprintf(os.Stderr, "YAM %s: %v\n", name, e)
		*last = time.Now()
	}
}
func verifyYAM(api *yamAPI) error {
	d, e := api.device()
	if e != nil {
		return fmt.Errorf("YAM runtime API failed: %w", e)
	}
	if d.Model != "yandexplum" && d.Model != "plum" {
		return fmt.Errorf("YAM detected model %q, expected Yandex Station Light 2", d.Model)
	}
	for _, f := range []string{"display.clock.segment.v1", "led.ring.rgb", "led.ring.3"} {
		if !feature(d, f) {
			return fmt.Errorf("YAM profile missing %s; configure LED profile plum with count=3 and enable clock support", f)
		}
	}
	if e = api.leds([3]string{"#000000", "#000000", "#000000"}); e != nil {
		return fmt.Errorf("YAM RGB ring preflight failed: %w", e)
	}
	if e = api.time(time.Now().In(clockZone()).Format("15:04")); e != nil {
		return fmt.Errorf("YAM clock preflight failed: %w", e)
	}
	return nil
}

// YAM starts enabled services while the HTTP router is still coming up;
// the boot hook may also be stopping Quasar at the same time. Retry preflight
// without touching hardware until both conditions have cleared.
func waitForReady(ctx context.Context, name string, timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if err := check(); err == nil {
			return nil
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become ready: %w", name, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}
func runPlum(ctx context.Context) error {
	api, e := newYAMAPI()
	if e != nil {
		return e
	}
	if e = waitForReady(ctx, "AirPlay audio ownership", 32*time.Second, preflight); e != nil {
		return e
	}
	if e = waitForReady(ctx, "YAM clock/RGB API", 32*time.Second, func() error { return verifyYAM(api) }); e != nil {
		return e
	}
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	root := filepath.Dir(filepath.Dir(executable))
	home, conf, e := createConfigs(root)
	if e != nil {
		return e
	}
	// Keep the short-lived YAM runtime token confined to this supervisor.
	// Audio/Avahi child processes never need access to YAM's hardware API.
	env := []string{}
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "YAM_API_URL=") || strings.HasPrefix(value, "YAM_API_TOKEN=") || strings.HasPrefix(value, "LD_LIBRARY_PATH=") || strings.HasPrefix(value, "HOME=") {
			continue
		}
		env = append(env, value)
	}
	env = append(env, "LD_LIBRARY_PATH="+filepath.Join(root, "lib"), "HOME="+home)
	lockPath := "/tmp/plum-yam-airplay.lock"
	lk, e := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lk.Close()
	if e = syscall.Flock(int(lk.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("another Station airplay service is already running")
	}
	defer syscall.Flock(int(lk.Fd()), syscall.LOCK_UN)
	audioFD, e := ensureFIFO(audioFIFO)
	if e != nil {
		return e
	}
	defer syscall.Close(audioFD)
	metaFD, e := ensureFIFO(metadataFIFO)
	if e != nil {
		return e
	}
	defer syscall.Close(metaFD)
	children := []*child{}
	defer func() { stopChildren(children) }()
	nq, e := startChild(ctx, filepath.Join(root, "bin/nqptp"), nil, env)
	if e != nil {
		return e
	}
	children = append(children, nq)
	if !processExists("avahi-daemon") {
		// We intentionally never remount / or edit D-Bus policy from a YAM plugin.
		if _, e := os.Stat("/etc/dbus-1/system.d/avahi-dbus.conf"); e != nil {
			return errors.New("Avahi D-Bus policy is missing; run the included prepare-host.sh outside YAM")
		}
		av, e := startChild(ctx, filepath.Join(root, "bin/avahi-daemon"), []string{"--no-drop-root", "--no-chroot", "-f", filepath.Join(root, "config/avahi-daemon.conf")}, env)
		if e != nil {
			return e
		}
		children = append(children, av)
	} else {
		fmt.Println("reusing system avahi-daemon")
	}
	time.Sleep(1600 * time.Millisecond)
	for _, c := range children {
		if e := c.cmd.Process.Signal(syscall.Signal(0)); e != nil {
			return fmt.Errorf("%s exited during startup", c.name)
		}
	}
	sp, e := startChild(ctx, filepath.Join(root, "bin/shairport-sync-ap2"), []string{"-c", conf}, env)
	if e != nil {
		return e
	}
	children = append(children, sp)
	fmt.Println(version, "running (YAM clock+ring; no buttons)")
	exits := make(chan exitInfo, len(children))
	for _, c := range children {
		go func(c *child) { exits <- exitInfo{c.name, c.cmd.Wait()} }(c)
	}
	state := &airplayState{status: "stopped"}
	tz := clockZone()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	lastUIError := time.Time{}
	lastClockError := time.Time{}
	var oldRing [3]string
	oldFrameKey := ""
	lastRingSent, lastClockSent := time.Time{}, time.Time{}
	lastAudioReport := time.Now()
	lastAudioCount := uint64(0)
	for {
		select {
		case <-ctx.Done():
			// Graceful SIGTERM path; YAM's kill is SIGKILL, children have Pdeathsig.
			_ = api.ledOff()
			_ = api.clockOff()
			return nil
		case ex := <-exits:
			return fmt.Errorf("%s exited unexpectedly: %v", ex.name, ex.err)
		case now := <-tick.C:
			if e := readFD(metaFD, func(buf []byte) {
				state.meta.feed(buf, func(t, c string, raw []byte) { state.onMetadata(t, c, raw, now) })
			}); e != nil {
				return fmt.Errorf("metadata FIFO: %w", e)
			}
			if e := readFD(audioFD, func(buf []byte) { state.audio(buf, now) }); e != nil {
				return fmt.Errorf("audio FIFO: %w", e)
			}
			state.rotate()
			if now.Sub(lastAudioReport) >= 15*time.Second {
				if state.status == "playing" {
					fmt.Printf("station-airplay visualizer: pcm_frames=%d wave=%.2f active=%t\n", state.pcmFrames-lastAudioCount, state.waveSmooth, state.playing(now))
				}
				lastAudioCount = state.pcmFrames
				lastAudioReport = now
			}
			colors := state.ringFrame(now)
			if colors != oldRing || now.Sub(lastRingSent) > 3*time.Second {
				e := api.leds(colors)
				logAPIError("ring", e, &lastUIError)
				if e == nil {
					oldRing = colors
					lastRingSent = now
				}
			}
			timeValue, frame := state.display(now, tz)
			var key string
			if frame == nil {
				key = "time:" + timeValue
			} else {
				b, _ := json.Marshal(frame)
				key = "seg:" + string(b)
			}
			if key != oldFrameKey || now.Sub(lastClockSent) >= 1200*time.Millisecond {
				var err error
				if frame == nil {
					err = api.time(timeValue)
				} else {
					err = api.segments(frame)
				}
				logAPIError("clock", err, &lastClockError)
				if err == nil {
					oldFrameKey = key
					lastClockSent = now
				}
			}
		}
	}
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	// yamd 0.1.x does not set a parent-death signal on binary plugins. Retire
	// this service if its owning daemon disappears, before a replacement starts.
	go watchParent(ctx, cancel, os.Getppid(), os.Getppid)
	if e := run(ctx); e != nil {
		fmt.Fprintln(os.Stderr, "station-airplay:", e)
		os.Exit(1)
	}
}
