package main

import (
	"context"
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

func run(ctx context.Context) error {
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
	p, ok := profileFor(d.Model)
	if !ok {
		return fmt.Errorf("unsupported YAM model %q", d.Model)
	}
	if p.pcm == "" {
		return runPlum(ctx)
	}
	if d.Architecture != "aarch64" {
		return errors.New("shared profiles require an AArch64 kernel")
	}
	return runShared(ctx, p)
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

func audioEnv(environ []string, home, bus string) []string {
	env := []string{}
	for _, s := range environ {
		if strings.HasPrefix(s, "YAM_API_") || strings.HasPrefix(s, "LD_LIBRARY_PATH=") || (bus != "" && strings.HasPrefix(s, "DBUS_SYSTEM_BUS_ADDRESS=")) || strings.HasPrefix(s, "HOME=") {
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
	if serial == "" || len(serial) > 32 {
		return "", errors.New("missing/invalid boot serial for stock mDNS hostname")
	}
	for _, c := range serial {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return "", errors.New("invalid boot serial")
		}
	}
	return prefix + "-" + serial, nil
}

type childFailure struct {
	name string
	err  error
}

func (e *childFailure) Error() string {
	return fmt.Sprintf("%s exited unexpectedly: %v", e.name, e.err)
}

func runShared(ctx context.Context, p stationProfile) error {
	for attempts := 0; ; attempts++ {
		err := serveShared(ctx, p)
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
	var api *yamAPI
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
	if err = os.WriteFile(conf, []byte(receiverConfig(p)), 0600); err != nil {
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
	defer func() { stopChildren(children) }()
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
	if err = launch("dbus-daemon", "/usr/bin/dbus-daemon", "--config-file="+filepath.Join(state, "bus.conf"), "--nofork"); err != nil {
		return err
	}
	ready := func(check func() bool) error {
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			if check() {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ex := <-exits:
				return fmt.Errorf("%s exited during startup: %v", ex.name, ex.err)
			case <-deadline.C:
				return errors.New("private Avahi/D-Bus startup timed out")
			case <-tick.C:
			}
		}
	}
	if err = ready(func() bool { fi, e := os.Stat(bus); return e == nil && fi.Mode()&os.ModeSocket != 0 }); err != nil {
		return err
	}
	ld32 := filepath.Join(root, "lib/ld-linux-armhf.so.3")
	libs := filepath.Join(root, "lib")
	if err = launch("avahi-namespace", ld32, "--library-path", libs, filepath.Join(root, "bin/avahi-namespace"), state,
		ld32, "--library-path", libs, filepath.Join(root, "bin/avahi-daemon"),
		"-f", avahiConf, "--no-drop-root", "--no-chroot", "--no-rlimits", "--no-proc-title"); err != nil {
		return err
	}
	if err = ready(func() bool {
		checkCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		c := exec.CommandContext(checkCtx, "/usr/bin/dbus-send", "--system", "--print-reply", "--dest=org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus.NameHasOwner", "string:org.freedesktop.Avahi")
		c.Env = env
		b, e := c.Output()
		return e == nil && strings.Contains(string(b), "boolean true")
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
				preload = []string{"--preload", "/usr/lib/libasound.so.2"}
			}
		}
		argv := append([]string{"--library-path", paths}, preload...)
		argv = append(argv, filepath.Join(root, "bin", binary))
		return launch(name, loader, append(argv, args...)...)
	}
	if err = launchAP2("nqptp", "-v"); err != nil {
		return err
	}
	if err = ready(func() bool {
		socket, e := net.ListenPacket("udp4", "127.0.0.1:9000")
		if e == nil {
			socket.Close()
			return false
		}
		return true
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
	select {
	case <-ctx.Done():
		return nil
	case ex := <-exits:
		return &childFailure{ex.name, ex.err}
	}
}
