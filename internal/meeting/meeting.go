// Package meeting is Meeting notes: a meeting's two sides recorded through
// the operating system's audio stack, transcribed on this computer by the
// whisper.cpp sidecar, and written as one Markdown transcript to a folder.
//
// The rest of the feature never imports a platform. Each platform is one
// implementation of Recorder (and, where calls are detected, Detector):
// recorder_linux.go over the PulseAudio protocol, which PipeWire also
// serves. Engine is the transcriber behind its own boundary, so the sidecar
// could be replaced by an engine in this process.
//
// What a recording keeps, in order: two kinds of track (the microphone,
// "Me", and the far side, "Them"), each a 16 kHz mono WAV whose sample
// position is its time since the recording started; then, after Stop, one
// transcript per recording, merged from the tracks by time; then, unless the
// person keeps recordings, nothing of the audio.
package meeting

import (
	"context"
	"errors"
	"time"
)

// SampleRate is the rate every track is recorded at: Whisper's own.
const SampleRate = 16000

// Label says which side of the meeting a track carries.
type Label string

const (
	// Me is the microphone: the person who owns this computer.
	Me Label = "Me"
	// Them is everything the meeting app played: every other participant,
	// not split by person.
	Them Label = "Them"
)

// FarSide is how the far side was recorded, written into the transcript.
type FarSide string

const (
	// FarSideApp is the meeting app's own playback streams alone.
	FarSideApp FarSide = "app"
	// FarSideWholeDevice is everything the output device played: no
	// meeting app was found to record on its own, so other sounds the
	// computer made are in the recording too.
	FarSideWholeDevice FarSide = "whole-device"
)

// Call is a meeting app that has a call under way, as the audio stack shows
// it: the process that both records from a microphone and plays audio.
type Call struct {
	// App is the name the app gives the audio stack ("Google Chrome",
	// "ZOOM VoiceEngine").
	App string
	// Binary is the app's executable name, when the stack reports it.
	Binary string
	// PID is the app's process id, as the stack reports it.
	PID string
}

// CallEvent is a Detector's news: a call started or ended.
type CallEvent struct {
	Call  Call
	Ended bool
	At    time.Time
}

// Detector reports calls as they start and end. Watch runs until ctx ends.
type Detector interface {
	Watch(ctx context.Context, events chan<- CallEvent) error
}

// Track is where a recorder writes one stream's samples, as they arrive.
// The track places them on the recording's timeline by when they arrive, so
// a stream that starts late or stops for a while keeps its time.
type Track interface {
	Write(samples []int16)
	// End closes the track; nothing more is written to it.
	End()
}

// Tracks opens the tracks a recorder writes to: one per stream, labelled
// with the side it carries and a short name for the file.
type Tracks interface {
	Open(label Label, name string) (Track, error)
}

// Recorder starts recording. With a zero Call it records the call it finds
// under way, or the whole default devices when it finds none.
type Recorder interface {
	Record(ctx context.Context, call Call, tracks Tracks) (Capture, error)
}

// Capture is a recording in progress.
type Capture interface {
	// Call is the meeting app being recorded; zero for the whole devices.
	Call() Call
	// FarSide says how the far side is recorded.
	FarSide() FarSide
	// SetPaused stops or restarts the flow of audio; while paused nothing
	// is captured, and the timeline goes on.
	SetPaused(paused bool) error
	// Ended is closed when every track has ended by itself: the meeting
	// app's streams are gone.
	Ended() <-chan struct{}
	// Stop ends every track and lets go of the audio stack.
	Stop() error
}

// ErrUnsupported is a platform this build cannot record on yet.
var ErrUnsupported = errors.New("meeting notes cannot record on this computer yet")

// Segment is one piece of transcribed speech, its times from the start of
// the recording.
type Segment struct {
	Label Label
	Start time.Duration
	End   time.Duration
	Text  string
}

// TranscribeRequest is one track to transcribe.
type TranscribeRequest struct {
	Audio    string // a 16 kHz mono WAV
	Model    string // the model file
	VADModel string // the Silero voice-detection model file
	Language string // "en", or "auto" to detect it
	// Progress, when set, is told how far along the track is, 0 to 100.
	Progress func(percent int)
}

// Engine transcribes one track. Its segments' times are from the start of
// the audio file, and their Label is left for the caller to set.
type Engine interface {
	Transcribe(ctx context.Context, req TranscribeRequest) (Transcription, error)
}

// Transcription is an Engine's answer for one track.
type Transcription struct {
	Segments []Segment
	// Language is what the engine detected or was told.
	Language string
	// Took is how long the engine ran, model loading left out where the
	// engine reports it.
	Took time.Duration
}
