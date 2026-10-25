package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// pipeRate is the pipe backend's output rate: AirPlay's native 44.1 kHz, so
// shairport-sync does not resample. The visualizer only needs approximate bands.
const pipeRate = 44100

// playPipe plays audioFIFO with the system aplay on "speakers" (the shared
// dmix behind the station volume) and copies every block to the visualizer.
// aplay runs with the system alsa-lib: the environment drops the package's
// LD_LIBRARY_PATH, whose older alsa-lib cannot join Quasar's dmix.
func playPipe(env []string) (*child, <-chan []byte, error) {
	fifo, e := os.OpenFile(audioFIFO, os.O_RDWR, 0) // RDWR: no EOF between sessions
	if e != nil {
		return nil, nil, e
	}
	sysEnv := make([]string, 0, len(env))
	for _, v := range env {
		if !strings.HasPrefix(v, "LD_LIBRARY_PATH=") {
			sysEnv = append(sysEnv, v)
		}
	}
	cmd := exec.Command("aplay", "-q", "-D", "speakers", "-f", "S16_LE", "-c", "2", "-r", strconv.Itoa(pipeRate), "-t", "raw")
	cmd.Env = sysEnv
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	in, e := cmd.StdinPipe()
	if e != nil {
		fifo.Close()
		return nil, nil, e
	}
	if e = cmd.Start(); e != nil {
		fifo.Close()
		return nil, nil, fmt.Errorf("start aplay: %w", e)
	}
	fmt.Printf("started aplay pid=%d (pipe -> speakers)\n", cmd.Process.Pid)
	vis := make(chan []byte, 64)
	go func() {
		defer fifo.Close()
		buf := make([]byte, 16384)
		for {
			n, e := fifo.Read(buf)
			if e != nil {
				return
			}
			if _, e = in.Write(buf[:n]); e != nil {
				return
			}
			block := append([]byte(nil), buf[:n]...)
			select {
			case vis <- block:
			default: // visualizer is behind; audio has priority
			}
		}
	}()
	return &child{name: "aplay", cmd: cmd}, vis, nil
}
