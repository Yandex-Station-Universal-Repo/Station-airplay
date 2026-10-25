package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A model profile is an audio/ABI contract, not a marketing name or kernel arch.
// AirPlay 2 receivers retain each model's proven shared ALSA device.
type modelProfile struct {
	Model, Name, PCM, ReceiverABI string
}

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

func profileFor(d yamDevice) (modelProfile, error) {
	switch d.Model {
	case "yandexstation_2":
		return modelProfile{d.Model, "Yandex Station 2 AirPlay", "plug:dmix_spk", "armhf"}, nil
	case "yandexmini_3_pro":
		return modelProfile{d.Model, "Yandex Mini 3 Pro AirPlay", "speakers", "aarch64"}, nil
	default:
		return modelProfile{}, fmt.Errorf("no shared-audio profile for YAM model %q", d.Model)
	}
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
	if d.Model == "plum" || d.Model == "yandexplum" {
		return runPlum(ctx)
	}
	p, err := profileFor(d)
	if err != nil {
		return err
	}
	if runtime.GOARCH != "arm64" || d.Architecture != "aarch64" {
		return errors.New("Station 2/Mini 3 Pro profiles require an AArch64 kernel and supervisor")
	}
	return runShared(ctx, p)
}

func receiverConfig(p modelProfile) string {
	return fmt.Sprintf(`general = {
 name = %q;
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
metadata = { enabled = "no"; };
`, p.Name, p.PCM)
}

func audioEnv(environ []string, bus string) []string {
	env := []string{}
	for _, s := range environ {
		if strings.HasPrefix(s, "YAM_API_") || strings.HasPrefix(s, "LD_LIBRARY_PATH=") || strings.HasPrefix(s, "DBUS_SYSTEM_BUS_ADDRESS=") || strings.HasPrefix(s, "HOME=") {
			continue
		}
		env = append(env, s)
	}
	return append(env, "DBUS_SYSTEM_BUS_ADDRESS=unix:path="+bus, "HOME=/run/station-airplay")
}

// Stock mDNS already owns this address/name. Publishing another address record
// triggers a rename loop on these firmwares, even with a different host label.
func sharedHostname(model, cmdline string) (string, error) {
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
	switch model {
	case "yandexstation_2":
		return "yandex-station-gen2-" + serial, nil
	case "yandexmini_3_pro":
		return "Yandex-Station-Mini-3-Pro-" + serial, nil
	default:
		return "", errors.New("unknown stock mDNS hostname profile")
	}
}

type childFailure struct {
	name string
	err  error
}

func (e *childFailure) Error() string {
	return fmt.Sprintf("%s exited unexpectedly: %v", e.name, e.err)
}

func runShared(ctx context.Context, p modelProfile) error {
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

func serveShared(ctx context.Context, p modelProfile) error {
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
	// Do not terminate somebody else's receiver, change ALSA, or stop Alice.
	listener, err := net.Listen("tcp4", ":5000")
	if err != nil {
		return fmt.Errorf("AirPlay port 5000 occupied; stop the previous receiver before migration: %w", err)
	}
	listener.Close()
	for _, program := range []string{"shairport-sync", "shairport-sync-ap2", "airplayd", "nqptp"} {
		if processExists(program) {
			return fmt.Errorf("existing %s receiver must be stopped explicitly", program)
		}
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
		found := false
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "avahi:") {
				found = true
			}
		}
		if !found {
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
	cmdline, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return err
	}
	host, err := sharedHostname(p.Model, string(cmdline))
	if err != nil {
		return err
	}
	avahiConf := fmt.Sprintf(`[server]
host-name=%s
use-ipv4=yes
use-ipv6=no
allow-interfaces=wlan0
disallow-other-stacks=no
enable-dbus=yes
[wide-area]
enable-wide-area=no
[publish]
publish-hinfo=no
publish-workstation=no
publish-addresses=no
`, host)
	if err = os.WriteFile(filepath.Join(state, "avahi.conf"), []byte(avahiConf), 0600); err != nil {
		return err
	}
	env := audioEnv(os.Environ(), bus)
	children := []*child{}
	defer func() { stopChildren(children) }()
	exits := make(chan exitInfo, 4)
	launch := func(name, bin string, args ...string) error {
		file, e := os.OpenFile(filepath.Join(state, name+".log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		output := &boundedLog{f: file}
		cmd.Stdout = output
		cmd.Stderr = output
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
		if e = cmd.Start(); e != nil {
			file.Close()
			return fmt.Errorf("start %s: %w", name, e)
		}
		c := &child{name: name, cmd: cmd}
		fmt.Printf("started %s pid=%d (bounded RAM log: %s)\n", name, cmd.Process.Pid, filepath.Join(state, name+".log"))
		children = append(children, c)
		go func() {
			err := c.cmd.Wait()
			file.Close()
			if err != nil {
				if b, e := os.ReadFile(filepath.Join(state, name+".log")); e == nil {
					if len(b) > 2048 {
						b = b[len(b)-2048:]
					}
					fmt.Fprintf(os.Stderr, "%s last output: %s\n", name, b)
				}
			}
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
	rt := filepath.Join(root, "runtime", "aarch64")
	if err = launch("avahi-namespace-aarch64", filepath.Join(root, "bin/avahi-namespace-aarch64"), state,
		filepath.Join(rt, "lib/ld-linux-aarch64.so.1"), "--library-path", filepath.Join(rt, "lib"), filepath.Join(rt, "bin/avahi-daemon"),
		"-f", filepath.Join(state, "avahi.conf"), "--no-drop-root", "--no-chroot", "--no-rlimits", "--no-proc-title"); err != nil {
		return err
	}
	// Query our private bus, not another Avahi instance in /proc.
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
	ap2 := filepath.Join(root, "runtime", "ap2-"+p.ReceiverABI)
	loader := "ld-linux-aarch64.so.1"
	if p.ReceiverABI == "armhf" {
		loader = "ld-linux-armhf.so.3"
	}
	launchAP2 := func(name string, args ...string) error {
		// Only this process uses the private libc; stock ALSA/D-Bus stay on device.
		argv := []string{"--library-path", filepath.Join(ap2, "lib") + ":/usr/lib:/lib", filepath.Join(ap2, "bin", name)}
		return launch(name, filepath.Join(ap2, "lib", loader), append(argv, args...)...)
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
	fmt.Printf("%s model=%s protocol=AirPlay2 pcm=%s Alice=shared UI=off\n", version, p.Model, p.PCM)
	select {
	case <-ctx.Done():
		return nil
	case ex := <-exits:
		return &childFailure{ex.name, ex.err}
	}
}
