package meeting

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/blaktron/agentry-desktop/internal/wasapi"
)

// NewRecorder is this platform's recorder: the audio child, which records
// through the Windows audio stack in a process of its own.
func NewRecorder() Recorder { return childRecorder{spawn: spawnAudioChild} }

// NewDetector is this platform's detector: the audio child, which reads the
// audio sessions whenever an app starts or stops using a microphone.
func NewDetector() Detector { return childDetector{spawn: spawnAudioChild} }

// selfExe is this app's executable, whose own sessions are never a call.
func selfExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe
}

// spawnAudioChild starts this executable again as the audio child.
func spawnAudioChild() (*childProc, error) {
	exe := selfExe()
	if exe == "" {
		return nil, errors.New("this app's executable cannot be found")
	}
	cmd := exec.Command(exe, CaptureFlag) //nolint:gosec // this app itself
	hideConsole(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	stop := func() {
		_ = in.Close()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}
	return &childProc{in: in, out: out, stop: stop}, nil
}

// CaptureMain runs this process as the audio child, reading its commands
// from in and writing its frames to out.
func CaptureMain(in io.Reader, out io.Writer) int {
	return captureMain(in, out, openWASAPI, selfExe())
}

func openWASAPI() (audioSystem, error) {
	s, err := wasapi.Open()
	if err != nil {
		return nil, err
	}
	return wasapiSystem{s}, nil
}

// wasapiSystem is the audio stack through internal/wasapi.
type wasapiSystem struct{ s *wasapi.System }

func flowOf(f endpointFlow) wasapi.Flow {
	if f == flowCapture {
		return wasapi.Capture
	}
	return wasapi.Render
}

func flowFrom(f wasapi.Flow) endpointFlow {
	if f == wasapi.Capture {
		return flowCapture
	}
	return flowRender
}

func (w wasapiSystem) Sessions() ([]audioSession, error) {
	ss, err := w.s.Sessions()
	if err != nil {
		return nil, err
	}
	out := make([]audioSession, 0, len(ss))
	for _, x := range ss {
		out = append(out, audioSession{Endpoint: x.Endpoint, Flow: flowFrom(x.Flow), PID: x.PID, Exe: x.Exe, App: x.App, Active: x.Active})
	}
	return out, nil
}

func (w wasapiSystem) Default(f endpointFlow) (string, error) { return w.s.Default(flowOf(f)) }

func (w wasapiSystem) Record(ep string, f endpointFlow) (audioStream, error) {
	st, err := w.s.Record(ep, flowOf(f))
	if err != nil {
		if errors.Is(err, wasapi.ErrGone) {
			return nil, errDeviceGone
		}
		return nil, err
	}
	return wasapiStream{st}, nil
}

func (w wasapiSystem) Changed(timeout time.Duration) { w.s.Changed(timeout) }

func (w wasapiSystem) Close() { w.s.Close() }

type wasapiStream struct{ st *wasapi.Stream }

func (s wasapiStream) Read() ([]int16, error) {
	b, err := s.st.Read()
	if errors.Is(err, wasapi.ErrGone) {
		err = errDeviceGone
	}
	return b, err
}

func (s wasapiStream) Close() { s.st.Close() }
