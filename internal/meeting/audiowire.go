package meeting

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Where the audio stack's code runs in a child process of the app (Windows),
// the two talk over the child's standard input and output. The parent sends
// one command per line, as JSON; closing the child's input ends it. The
// child sends frames: a kind, a track, a length and that many bytes, which
// are a track's samples (16-bit little-endian, 16 kHz mono) or an event as
// JSON.

const (
	frameAudio = 'a'
	frameEvent = 'e'
)

// frameHead is a frame's kind, track and length.
const frameHead = 6

// The parent's commands.
const (
	cmdDetect = "detect"
	cmdRecord = "record"
	cmdPause  = "pause"
	cmdMics   = "mics"
)

// The child's events.
const (
	evSessions = "sessions"
	evStarted  = "started"
	evFarSide  = "far-side"
	evEnded    = "ended"
	evLog      = "log"
	evError    = "error"
	evMics     = "mics"
)

// The child's tracks.
const (
	wireMic = 0
	wireFar = 1
)

// maxFrame bounds what a frame may claim to carry: a second of audio is
// 32 KB, an event a few.
const maxFrame = 4 << 20

// wireCommand is the parent's instruction: "detect", to report the audio
// stack's sessions as they change; "record", to record the call of PID or
// Binary (or, with neither, the call found under way or the whole default
// devices; or, with Room, the microphone alone); "pause", to stop or
// restart the flow of audio; "mics", to list the microphones once. Mic is
// the microphone chosen, by endpoint id, for a recording that follows no
// app.
type wireCommand struct {
	Cmd    string `json:"cmd"`
	PID    string `json:"pid,omitempty"`
	Binary string `json:"binary,omitempty"`
	Paused bool   `json:"paused,omitempty"`
	Room   bool   `json:"room,omitempty"`
	Mic    string `json:"mic,omitempty"`
}

// wireEvent is the child's news:
//   - "sessions": the audio stack's sessions, whenever they change;
//   - "started": recording, of Call, and how the far side is recorded;
//   - "far-side": the far side is now recorded another way;
//   - "ended": the call's app stopped using the microphone long enough;
//   - "log": something worth the parent's log, such as a device followed;
//   - "error": the child cannot go on, and why;
//   - "mics": the microphones, the answer to "mics".
//
// A "started" with no app followed also names the microphone recorded
// (MicName) and whether the one chosen was missing (MicMissing).
type wireEvent struct {
	Type       string         `json:"type"`
	Call       *Call          `json:"call,omitempty"`
	FarSide    FarSide        `json:"farSide,omitempty"`
	Message    string         `json:"message,omitempty"`
	Sessions   []audioSession `json:"sessions,omitempty"`
	Mics       []Microphone   `json:"mics,omitempty"`
	MicName    string         `json:"micName,omitempty"`
	MicMissing bool           `json:"micMissing,omitempty"`
}

// frameWriter writes whole frames from any goroutine.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (f *frameWriter) frame(kind, track byte, payload []byte) error {
	head := make([]byte, frameHead)
	head[0], head[1] = kind, track
	binary.LittleEndian.PutUint32(head[2:], uint32(len(payload))) //nolint:gosec // a frame is far below 4 GB
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.w.Write(head); err != nil {
		return err
	}
	_, err := f.w.Write(payload)
	return err
}

func (f *frameWriter) audio(track byte, samples []int16) error {
	b := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(s)) //nolint:gosec // the sample's bits as they are
	}
	return f.frame(frameAudio, track, b)
}

func (f *frameWriter) event(ev wireEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return f.frame(frameEvent, 0, b)
}

// readFrame reads one frame.
func readFrame(r *bufio.Reader) (kind, track byte, payload []byte, err error) {
	head := make([]byte, frameHead)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, 0, nil, err
	}
	n := binary.LittleEndian.Uint32(head[2:])
	if n > maxFrame {
		return 0, 0, nil, fmt.Errorf("the audio child sent a frame of %d bytes, more than any audio or event", n)
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return 0, 0, nil, err
	}
	return head[0], head[1], payload, nil
}
