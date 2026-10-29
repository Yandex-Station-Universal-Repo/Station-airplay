package main

// AirPlay owns the speaker while a sender is playing, but Alice's own music
// player can resume in parallel when she hears a media phrase such as
// "continue". The stock Quasar SDK CLI exposes an explicit Player Pause input;
// use it for a short handover window after a voice transport command. No
// Quasar patch, no preload and no second volume protocol.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const quasarCLIPath = "/system/vendor/quasar/q"

// nativePauser is the handover surface used by the controller; tests
// substitute a counter for the real SDK session.
type nativePauser interface {
	Pause(context.Context) error
	PauseNow() error
	Stop()
}

type nativePlayer struct {
	path string
	mu   sync.Mutex
	sess *cliSession
}

func newNativePlayer(path string) *nativePlayer { return &nativePlayer{path: path} }

// Pause keeps one short-lived CLI session for the whole handover window so a
// late resume by Alice is still caught, then Stop tears it down.
func (n *nativePlayer) Pause(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sess == nil {
		s, err := startCLI(ctx, n.path)
		if err != nil {
			aliceButtonDebug("pause: CLI unavailable: %v", err)
			return err
		}
		n.sess = s
	}
	err := n.sess.pause(ctx)
	aliceButtonDebug("pause: sent Player Pause err=%v", err)
	return err
}

// PauseNow enqueues the same explicit pause input without waiting for the SDK
// acknowledgement. The handover cadence uses it so a refresh never stalls the
// control loop on the 300 ms acknowledgement wait; the input itself is still
// written before returning.
func (n *nativePlayer) PauseNow() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sess == nil {
		aliceButtonDebug("pause-now: no CLI session")
		return nil
	}
	_, err := io.WriteString(n.sess.in, "o")
	if err != nil {
		aliceButtonDebug("pause-now: write failed: %v", err)
	}
	return err
}

func (n *nativePlayer) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sess != nil {
		aliceButtonDebug("stop: closing CLI")
		n.sess.stop()
		n.sess = nil
	}
}

type cliSession struct {
	cmd       *exec.Cmd
	in        io.WriteCloser
	markers   atomic.Int64
	ready     chan struct{}
	readyOnce sync.Once
	done      chan struct{}
	stopOnce  sync.Once
}

// stockCLIEnv removes the package library overrides so the stock shell script
// and its loader use the system runtime. Everything else is inherited.
func stockCLIEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, v := range env {
		if strings.HasPrefix(v, "LD_LIBRARY_PATH=") || strings.HasPrefix(v, "LD_PRELOAD=") {
			continue
		}
		out = append(out, v)
	}
	return out
}

func startCLI(ctx context.Context, path string) (*cliSession, error) {
	if path == "" {
		return nil, errors.New("stock Quasar CLI path is empty")
	}
	cmd := exec.Command(path, "cli")
	// The service inherits the package's bundled LD_LIBRARY_PATH (a newer glibc
	// for shairport and avahi). The stock CLI is a shell script, so /bin/sh
	// would load that glibc and fail before the script can select Quasar's own
	// libraries. Run it with the system environment, like aplay in the pipe
	// backend; the script sets LD_LIBRARY_PATH for maind itself.
	cmd.Env = stockCLIEnv(os.Environ())
	// Own process group: a wedged CLI is terminated with its helpers, never
	// left behind after the handover.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	s := &cliSession{cmd: cmd, in: in, ready: make(chan struct{}), done: make(chan struct{})}
	if err = cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		_ = pw.Close()
		close(s.done)
	}()
	go s.read(pr)
	timer := time.NewTimer(2500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-s.ready:
		return s, nil
	case <-s.done:
		return nil, errors.New("stock Quasar CLI exited before connecting")
	case <-timer.C:
		s.stop()
		return nil, errors.New("stock Quasar CLI did not connect")
	case <-ctx.Done():
		s.stop()
		return nil, ctx.Err()
	}
}

func (s *cliSession) read(r io.ReadCloser) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 256*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "Connected to DirectiveSequencer") {
			s.readyOnce.Do(func() { close(s.ready) })
		}
		if strings.Contains(line, "UserInput: Player Pause") {
			s.markers.Add(1)
		}
	}
	// Never let an over-long SDK log line block the CLI's output pipe.
	_, _ = io.Copy(io.Discard, r)
}

// pause sends the SDK's explicit pause input, never a play/pause toggle. The
// SDK routes input asynchronously, so wait briefly for its acknowledgement;
// silence is normal when no native playback is active.
func (s *cliSession) pause(ctx context.Context) error {
	before := s.markers.Load()
	if _, err := io.WriteString(s.in, "o"); err != nil {
		return err
	}
	deadline := time.NewTimer(300 * time.Millisecond)
	defer deadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return nil
		case <-poll.C:
			if s.markers.Load() > before {
				return nil
			}
		}
	}
}

func (s *cliSession) stop() {
	s.stopOnce.Do(func() {
		_, _ = io.WriteString(s.in, "x")
		_ = s.in.Close()
		select {
		case <-s.done:
		case <-time.After(500 * time.Millisecond):
			if s.cmd.Process != nil {
				_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			}
			select {
			case <-s.done:
			case <-time.After(500 * time.Millisecond):
			}
		}
	})
}
