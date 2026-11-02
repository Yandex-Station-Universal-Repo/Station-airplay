package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func watchParent(ctx context.Context, cancel context.CancelFunc, parent int, current func() int) {
	if parent <= 1 {
		return
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if current() != parent {
				cancel()
				return
			}
		}
	}
}

// High-volume receiver statistics stay in bounded RAM files, not YAM's flash log.
type boundedLog struct {
	mu sync.Mutex
	f  *os.File
	n  int
}

func (l *boundedLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	const limit = 128 * 1024
	length := len(b)
	if len(b) > limit {
		b = b[len(b)-limit:]
	}
	if l.n+len(b) > limit {
		if err := l.f.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := l.f.Seek(0, 0); err != nil {
			return 0, err
		}
		l.n = 0
	}
	n, err := l.f.Write(b)
	l.n += n
	if err != nil {
		return n, err
	}
	return length, nil
}

func run(ctx context.Context, sharedAudio bool) error {
	return runWithControls(ctx, sharedAudio, nil)
}
func runWithControls(ctx context.Context, sharedAudio bool, controls *controlsConfig) error {
	api, err := newYAMAPI()
	if err != nil {
		return err
	}
	var d yamDevice
	err = waitForReady(ctx, "YAM device API", 32*time.Second, func() error {
		var e error
		d, e = api.device()
		return e
	})
	if err != nil {
		return err
	}
	p, ok := profileForDevice(d, sharedAudio)
	if !ok {
		return fmt.Errorf("unsupported YAM model %q", d.Model)
	}
	if err = validateControlRenderer(p, controls); err != nil {
		return err
	}
	if p.pcm == "" {
		return runPlum(ctx)
	}
	p, err = profileWithRoom(p, d.Room)
	if err != nil {
		return err
	}
	if controls != nil {
		if err = controlsPreflight(ctx, api, d, *controls); err != nil {
			return err
		}
	}
	if d.Architecture != "aarch64" {
		return errors.New("shared profiles require an AArch64 kernel")
	}
	return runSharedControls(ctx, p, api, d, controls)
}

// A renderer must have only one UI owner. Controls keep their explicit
// Light 2 audio-only profile; upstream display mode remains available without them.
func validateControlRenderer(p stationProfile, controls *controlsConfig) error {
	if controls == nil {
		return nil
	}
	if p.ui {
		return errors.New("shared controls on clock-equipped Light 2 require --shared-audio; upstream display mode cannot own the same UI")
	}
	if p.pcm == "" {
		return errors.New("shared controls are not enabled for legacy Plum/Midi renderer")
	}
	if p.receiverABI == "aarch64" {
		return errors.New("shared controls require the control-enabled ARMhf receiver; AArch64 control build is not qualified")
	}
	if p.packagedDBus {
		return errors.New("shared controls for the original Mini packaged D-Bus profile are not qualified; audio-only upstream mode remains available")
	}
	return nil
}

// Clock capability alone does not say whether shared audio is wanted or usable.
// Explicit opt-in also supports Plum firmware advertising a segment clock.
// Without opt-in, retain upstream 1.4.2 clock-equipped Light 2 display mode.
func profileForDevice(d yamDevice, sharedAudio bool) (stationProfile, bool) {
	p, ok := profileFor(d.Model)
	if ok && (d.Model == "plum" || d.Model == "yandexplum") && (sharedAudio || !feature(d, "display.clock.segment.v1")) {
		return plumSharedProfile, true
	}
	return p, ok
}

func receiverConfig(p stationProfile) string {
	device := p.pcm
	meta := `metadata = { enabled = "no"; };`
	if p.ui {
		// spk_tee copies PCM into the visualizer FIFO and plays the same
		// frames on the shared mixer. The mixer name stays in uiAsound.
		device = "spk_tee"
		meta = fmt.Sprintf("metadata = { enabled = \"yes\"; include_cover_art = \"no\"; pipe_name = %q; };", metadataFIFO)
	}
	return fmt.Sprintf(`general = {
 name = %q;
 model = %q;
 output_backend = "alsa";
 mdns_backend = "avahi";
 port = 5000;
 interpolation = "basic";
 volume_range_db = 50;
 resync_threshold_in_seconds = 0.050;
 drift_tolerance_in_seconds = 0.002;
};
sessioncontrol = { allow_session_interruption = "yes"; session_timeout = 60; };
alsa = {
 output_device = %q;
 output_rate = 48000;
 output_channels = 2;
 output_format = "S16_LE";
 disable_standby_mode = "never";
};
%s
`, p.name, p.airplayModel, device, meta)
}

// uiAsound is the receiver's private ~/.asoundrc. The file plugin feeds the
// clock visualizer; its slave is the firmware shared mixer, not hw:0,0.
func uiAsound(pcm, fifo string) string {
	return fmt.Sprintf(`pcm.spk_file {
 type file
 slave.pcm %q
 file %q
 format "raw"
 perm 0666
}
pcm.spk_tee {
 type plug
 slave.pcm "spk_file"
}
`, pcm, fifo)
}

func receiverControlConfig(p stationProfile, controls *controlsConfig) string {
	config := receiverConfig(p)
	if controls == nil {
		return config
	}
	// Shairport's internal metadata hub also carries DACP credentials/client state.
	// Disabling metadata disables remote-control discovery even with D-Bus built.
	// Keep its unused FIFO private and volatile; never request artwork/cache files.
	config = strings.Replace(config, `metadata = { enabled = "no"; };`,
		`metadata = { enabled = "yes"; include_cover_art = "no"; pipe_name = "/run/station-airplay/control-metadata"; cover_art_cache_directory = "/run/station-airplay/coverart"; };`, 1)
	// The default ten-second active tail keeps the playback indication lit after
	// AirPlay has gone. A short tail still absorbs jitter without lingering.
	config = strings.Replace(config, `sessioncontrol = { allow_session_interruption = "yes"; session_timeout = 60; };`,
		`sessioncontrol = { allow_session_interruption = "yes"; session_timeout = 60; active_state_timeout = 1.0; };`, 1)
	if controls.NativeVolume {
		config = strings.Replace(config, " volume_range_db = 50;", " volume_range_db = 50;\n ignore_volume_control = \"yes\";", 1)
	}
	return config
}

func audioEnv(environ []string, home, bus string) []string {
	env := []string{}
	for _, s := range environ {
		if strings.HasPrefix(s, "YAM_API_") || strings.HasPrefix(s, "LUNA_API_") || strings.HasPrefix(s, "LD_LIBRARY_PATH=") || (bus != "" && strings.HasPrefix(s, "DBUS_SYSTEM_BUS_ADDRESS=")) || strings.HasPrefix(s, "HOME=") {
			continue
		}
		env = append(env, s)
	}
	if bus != "" {
		env = append(env, "DBUS_SYSTEM_BUS_ADDRESS=unix:path="+bus)
	}
	return append(env, "HOME="+home)
}

// Stock mDNS already owns this address/name. Publishing another address record
// triggers a rename loop on these firmwares, even with a different host label.
func stockHostname(prefix, cmdline string) (string, error) {
	serial := ""
	for _, field := range strings.Fields(cmdline) {
		if strings.HasPrefix(field, "androidboot.serialno=") {
			serial = strings.TrimPrefix(field, "androidboot.serialno=")
		}
	}
	return joinHostID(prefix, serial)
}

// Linkplay factory settings (JSON). The Mini's maind publishes its mv_uuid.
const linkplayNvram = "/vendor/new_nvram_config"

func nvramHostname(prefix string, raw []byte) (string, error) {
	var nv struct {
		UUID string `json:"mv_uuid"`
	}
	if err := json.Unmarshal(raw, &nv); err != nil {
		return "", fmt.Errorf("Linkplay nvram: %w", err)
	}
	return joinHostID(prefix, nv.UUID)
}

func joinHostID(prefix, id string) (string, error) {
	if id == "" || len(id) > 32 {
		return "", errors.New("missing/invalid device id for stock mDNS hostname")
	}
	for _, c := range id {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return "", errors.New("invalid device id")
		}
	}
	return prefix + "-" + id, nil
}

func stockHost(p stationProfile) (string, error) {
	if p.nvramHostID {
		b, e := os.ReadFile(linkplayNvram)
		if e == nil {
			return nvramHostname(p.mdnsHostPrefix, b)
		}
		if !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
		// Bench images and repaired boards may not carry the Linkplay factory
		// file. A missing file must not take AirPlay down: fall back to the same
		// serial-based identity every other model uses.
		fmt.Fprintf(os.Stderr, "station-airplay: %s missing, using serial host id\n", linkplayNvram)
	}
	b, e := os.ReadFile("/proc/cmdline")
	if e != nil {
		return "", e
	}
	return stockHostname(p.mdnsHostPrefix, string(b))
}

// dbusCommand returns the loader and leading arguments for a D-Bus tool:
// firmware /usr/bin, or the packaged ARMhf build behind the packaged loader.
func dbusCommand(root string, p stationProfile, tool string) (string, []string) {
	if !p.packagedDBus {
		return "/usr/bin/" + tool, nil
	}
	return filepath.Join(root, "lib/ld-linux-armhf.so.3"), []string{"--library-path", filepath.Join(root, "lib"), filepath.Join(root, "bin", tool)}
}

func firmwareALSA(p stationProfile) string {
	if p.alsaLib != "" {
		return p.alsaLib
	}
	return "/usr/lib/libasound.so.2"
}

type childFailure struct {
	name string
	err  error
}

func (e *childFailure) Error() string {
	return fmt.Sprintf("%s exited unexpectedly: %v", e.name, e.err)
}

func runShared(ctx context.Context, p stationProfile) error {
	return runSharedControls(ctx, p, nil, yamDevice{}, nil)
}
func runSharedControls(ctx context.Context, p stationProfile, api *yamAPI, d yamDevice, controls *controlsConfig) error {
	for attempts := 0; ; attempts++ {
		err := serveSharedControls(ctx, p, api, d, controls)
		if ctx.Err() != nil {
			return nil
		}
		var failed *childFailure
		if !errors.As(err, &failed) || attempts >= 5 {
			return err
		}
		fmt.Fprintf(os.Stderr, "station-airplay: %v; restarting owned children in 3 seconds (attempt %d/5)\n", err, attempts+1)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func serveShared(ctx context.Context, p stationProfile) error {
	return serveSharedControls(ctx, p, nil, yamDevice{}, nil)
}
func serveSharedControls(ctx context.Context, p stationProfile, api *yamAPI, d yamDevice, controls *controlsConfig) error {
	if err := validateControlRenderer(p, controls); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(filepath.Dir(exe))
	state := "/run/station-airplay"
	if err = os.MkdirAll(state, 0700); err != nil {
		return err
	}
	lk, err := os.OpenFile(filepath.Join(state, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lk.Close()
	if err = waitForReady(ctx, "previous station-airplay retirement", 3*time.Second, func() error {
		return syscall.Flock(int(lk.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}); err != nil {
		return err
	}
	defer syscall.Flock(int(lk.Fd()), syscall.LOCK_UN)
	if err = preflight(p); err != nil {
		return err
	}
	var metaFD, audioFD int
	if p.ui {
		if api, err = newYAMAPI(); err != nil {
			return err
		}
		if err = verifyYAM(api); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(state, ".asoundrc"), []byte(uiAsound(p.pcm, audioFIFO)), 0600); err != nil {
			return err
		}
		if audioFD, err = ensureFIFO(audioFIFO); err != nil {
			return err
		}
		defer syscall.Close(audioFD)
		if metaFD, err = ensureFIFO(metadataFIFO); err != nil {
			return err
		}
		defer syscall.Close(metaFD)
	}
	for _, port := range []int{319, 320, 9000} {
		socket, e := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
		if e != nil {
			return fmt.Errorf("AirPlay 2 timing port UDP %d occupied: %w", port, e)
		}
		socket.Close()
	}
	conf := filepath.Join(state, "shairport.conf")
	config := receiverControlConfig(p, controls)
	if err = os.WriteFile(conf, []byte(config), 0600); err != nil {
		return err
	}
	bus := filepath.Join(state, "private-bus")
	// A private bus and mount namespace preserve the system D-Bus and read-only root.
	for _, file := range []struct{ name, addition string }{
		{"passwd", "avahi:x:70:70:Avahi mDNS:/run/station-airplay:/bin/false\n"},
		{"group", "avahi:x:70:\n"},
	} {
		b, e := os.ReadFile("/etc/" + file.name)
		if e != nil {
			return e
		}
		if !strings.Contains("\n"+string(b), "\navahi:") {
			b = append(b, []byte("\n"+file.addition)...)
		}
		if err = os.WriteFile(filepath.Join(state, file.name), b, 0600); err != nil {
			return err
		}
	}
	busConf := fmt.Sprintf(`<busconfig><type>system</type><listen>unix:path=%s</listen><auth>EXTERNAL</auth><policy user="root"><allow own="*"/><allow send_destination="*"/><allow receive_sender="*"/></policy></busconfig>`, bus)
	if err = os.WriteFile(filepath.Join(state, "bus.conf"), []byte(busConf), 0600); err != nil {
		return err
	}
	avahiConf, err := avahiConfig(root, state, p)
	if err != nil {
		return err
	}
	env := audioEnv(os.Environ(), state, bus)
	children := []*child{}
	controlDone := make(chan struct{})
	controlErrors := make(chan error, 1)
	controlsStarted := false
	defer func() {
		cancel()
		if controlsStarted {
			<-controlDone
		}
		stopChildren(children)
	}()
	exits := make(chan exitInfo, 4)
	launch := func(name, bin string, args ...string) error {
		file, e := os.OpenFile(filepath.Join(state, name+".log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		c, e := startChild(ctx, bin, args, env, &boundedLog{f: file})
		if e != nil {
			file.Close()
			return e
		}
		c.name = name
		fmt.Printf("started %s pid=%d\n", name, c.cmd.Process.Pid)
		children = append(children, c)
		go func() {
			err := c.cmd.Wait()
			file.Close()
			exits <- exitInfo{c.name, err}
		}()
		return nil
	}
	dbusBin, dbusArgs := dbusCommand(root, p, "dbus-daemon")
	if err = launch("dbus-daemon", dbusBin, append(dbusArgs, "--config-file="+filepath.Join(state, "bus.conf"), "--nofork")...); err != nil {
		return err
	}
	ready := func(stage string, check func(context.Context) error) error {
		return waitSharedStartup(ctx, stage, sharedStartupTimeout, sharedStartupInterval, exits, check)
	}
	if err = ready("private D-Bus socket", func(context.Context) error {
		fi, e := os.Stat(bus)
		if e != nil {
			return e
		}
		if fi.Mode()&os.ModeSocket == 0 {
			return errors.New("private bus path is not a socket")
		}
		return nil
	}); err != nil {
		return err
	}
	ld32 := filepath.Join(root, "lib/ld-linux-armhf.so.3")
	libs := filepath.Join(root, "lib")
	if err = launch("avahi-namespace", ld32, "--library-path", libs, filepath.Join(root, "bin/avahi-namespace"), state,
		ld32, "--library-path", libs, filepath.Join(root, "bin/avahi-daemon"),
		"-f", avahiConf, "--no-drop-root", "--no-chroot", "--no-rlimits", "--no-proc-title"); err != nil {
		return err
	}
	if err = ready("private Avahi D-Bus registration", func(parent context.Context) error {
		checkCtx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		sendBin, sendArgs := dbusCommand(root, p, "dbus-send")
		c := exec.CommandContext(checkCtx, sendBin, append(sendArgs, "--system", "--print-reply", "--dest=org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus.NameHasOwner", "string:org.freedesktop.Avahi")...)
		c.Env = env
		b, e := c.CombinedOutput()
		if e != nil {
			return fmt.Errorf("Avahi NameHasOwner: %w: %.1024s", e, b)
		}
		if !strings.Contains(string(b), "boolean true") {
			return errors.New("Avahi has not acquired its D-Bus name")
		}
		return nil
	}); err != nil {
		return err
	}
	launchAP2 := func(name string, args ...string) error {
		loader, paths, binary := ld32, libs, name
		var preload []string
		if name == "shairport-sync" {
			binary = "shairport-sync-ap2"
			if p.receiverABI == "aarch64" {
				loader, paths, binary = "/lib/ld-linux-aarch64.so.1", filepath.Join(libs, "aarch64")+":/usr/lib:/lib", binary+"-aarch64"
			} else {
				// Shared dmix must use the firmware ALSA ABI, not the legacy Light library.
				preload = []string{"--preload", firmwareALSA(p)}
			}
		}
		argv := append([]string{"--library-path", paths}, preload...)
		argv = append(argv, filepath.Join(root, "bin", binary))
		return launch(name, loader, append(argv, args...)...)
	}
	if err = launchAP2("nqptp", "-v"); err != nil {
		return err
	}
	if err = ready("nqptp UDP 9000", func(context.Context) error {
		socket, e := net.ListenPacket("udp4", "127.0.0.1:9000")
		if e == nil {
			socket.Close()
			return errors.New("nqptp has not bound UDP 9000")
		}
		return nil
	}); err != nil {
		return err
	}
	if err = launchAP2("shairport-sync", "-c", conf, "-v", "--statistics"); err != nil {
		return err
	}
	if p.ui {
		fmt.Printf("%s model=%s protocol=AirPlay2 pcm=%s Alice=shared UI=clock+led\n", version, p.models[0], p.pcm)
		return renderDisplay(ctx, api, metaFD, audioFD, exits)
	}
	fmt.Printf("%s model=%s protocol=AirPlay2 pcm=%s Alice=shared UI=off\n", version, p.models[0], p.pcm)
	if controls != nil {
		if err = ready("receiver controls D-Bus API", func(parent context.Context) error {
			r, e := openReceiver(bus)
			if e != nil {
				return fmt.Errorf("open receiver: %w", e)
			}
			defer r.conn.Close()
			check, done := context.WithTimeout(parent, 3*time.Second)
			defer done()
			_, e = r.State(check)
			if e == nil && controls.Ducking {
				e = r.Gain(check, 100, 0)
			}
			return e
		}); err != nil {
			return fmt.Errorf("shared controls need receiver D-Bus support: %w", err)
		}
		controlsStarted = true
		go func() {
			defer close(controlDone)
			if e := serveControls(ctx, bus, api, d, *controls); e != nil {
				controlErrors <- e
			}
		}()
	}
	select {
	case <-ctx.Done():
		return nil
	case e := <-controlErrors:
		return fmt.Errorf("shared controls stopped: %w", e)
	case ex := <-exits:
		return &childFailure{ex.name, ex.err}
	}
}
