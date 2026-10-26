package meeting

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework CoreAudio -framework AudioToolbox -framework AVFoundation
#include <stdlib.h>
#include "coreaudio_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"
)

// NewRecorder is this platform's recorder: Core Audio, macOS 14.4 and
// later.
func NewRecorder() Recorder { return CoreAudioRecorder{} }

// NewDetector is this platform's detector: Core Audio's process list.
func NewDetector() Detector { return CoreAudioDetector{list: listProcesses} }

// CoreAudioRecorder records through Core Audio's process taps and the
// default input; caCapture says how.
type CoreAudioRecorder struct{}

// Record starts recording.
func (CoreAudioRecorder) Record(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
	if s := PlatformSupport(); s != "" {
		return nil, errors.New(s)
	}
	c, err := recordCoreAudio(ctx, coreAudio{}, call, tracks)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// listProcesses reads Core Audio's process list.
func listProcesses() ([]caProcess, error) {
	buf := make([]C.ca_proc, 256)
	for {
		n := int(C.ca_processes(&buf[0], C.int(len(buf))))
		if n < 0 {
			return nil, errors.New("the audio system could not list the apps that use audio")
		}
		if n > len(buf) {
			buf = make([]C.ca_proc, n+32)
			continue
		}
		out := make([]caProcess, n)
		for i := range out {
			p := &buf[i]
			out[i] = caProcess{
				Object: uint32(p.object), PID: int(p.pid),
				Input: p.input != 0, Output: p.output != 0,
				Path: C.GoString(&p.path[0]),
			}
		}
		return out, nil
	}
}

// coreAudio is the caSystem of this Mac.
type coreAudio struct{}

func (coreAudio) Processes() ([]caProcess, error) { return listProcesses() }

func (coreAudio) DefaultDevice(input bool) uint32 {
	in := C.int(0)
	if input {
		in = 1
	}
	return uint32(C.ca_default_device(in))
}

func (coreAudio) SelfObject() uint32 { return uint32(C.ca_self_object()) }

func (coreAudio) MicAllowed() bool { return C.ca_mic_authorization() == C.CA_AUTH_AUTHORIZED }

func (coreAudio) AskMic() { C.ca_mic_ask() }

func (coreAudio) StartTap(objects []uint32, global bool) (caSource, error) {
	var ptr *C.uint32_t
	if len(objects) > 0 {
		ptr = (*C.uint32_t)(unsafe.Pointer(&objects[0]))
	}
	g := C.int(0)
	if global {
		g = 1
	}
	var errbuf [256]C.char
	c := C.ca_tap_start(ptr, C.int(len(objects)), g, &errbuf[0], C.int(len(errbuf)))
	if c == nil {
		return nil, errors.New(C.GoString(&errbuf[0]))
	}
	return &coreAudioSource{c: c}, nil
}

func (coreAudio) StartMic() (caSource, error) {
	var errbuf [256]C.char
	c := C.ca_mic_start(&errbuf[0], C.int(len(errbuf)))
	if c == nil {
		return nil, errors.New(C.GoString(&errbuf[0]))
	}
	return &coreAudioSource{c: c}, nil
}

// coreAudioSource is one capture's ring, filled on Core Audio's thread.
// Only the capture's run (or its start, before run) reads or stops it.
type coreAudioSource struct{ c *C.ca_capture }

func (s *coreAudioSource) Rate() float64 { return float64(C.ca_rate(s.c)) }

func (s *coreAudioSource) Read(buf []float32) int {
	return int(C.ca_read(s.c, (*C.float)(unsafe.Pointer(&buf[0])), C.int(len(buf))))
}

func (s *coreAudioSource) Frames() uint64 { return uint64(C.ca_frames(s.c)) }
func (s *coreAudioSource) Heard() uint64  { return uint64(C.ca_heard(s.c)) }
func (s *coreAudioSource) Stop()          { C.ca_stop(s.c) }
