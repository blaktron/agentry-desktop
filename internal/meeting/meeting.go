// Package meeting is Meeting notes: a meeting's two sides recorded through
// the operating system's audio stack, transcribed on this computer by the
// whisper.cpp sidecar, and written as one Markdown transcript to a folder.
//
// The rest of the feature never imports a platform. Each platform is one
// implementation of Recorder (and, where calls are detected, Detector):
// recorder_linux.go over the PulseAudio protocol, which PipeWire also
// serves; audiochild_windows.go through WASAPI in a child process;
// recorder_darwin.go through Core Audio's process taps. Engine is the
// transcriber behind its own boundary, so the sidecar could be replaced by
// an engine in this process.
//
// What a recording keeps, in order: two kinds of track (the microphone,
// "Me", and the far side, "Them"), each a 16 kHz mono WAV whose sample
// position is its time since the recording started; then, after Stop, one
// transcript per recording, merged from the tracks by time; then, unless the
// person keeps recordings, nothing of the audio. A meeting in the room is
// recorded as one track, "Room": the microphone alone, with everyone in the
// room on it.
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
	// Room is the microphone of a meeting held in the room: everyone in the
	// room, not split by person.
	Room Label = "Room"
)

// FarSide is how the far side was recorded, written into the transcript.
type FarSide string

const (
	// FarSideApp is the meeting app's own playback streams alone.
	FarSideApp FarSide = "app"
	// FarSideAppDevice is everything played on the output device the
	// meeting app plays to (Windows records a device, not an app): the
	// meeting, and any other sound played there.
	FarSideAppDevice FarSide = "app-device"
	// FarSideWholeDevice is everything the output device played: no
	// meeting app was found to record on its own, so other sounds the
	// computer made are in the recording too.
	FarSideWholeDevice FarSide = "whole-device"
	// FarSideNone is a meeting in the room: there is no far side, and
	// nothing the computer plays is recorded.
	FarSideNone FarSide = "none"
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
	// Room asks for a meeting in the room: the microphone alone, no far
	// side and no app followed. Only the person's own Record in the room
	// sets it.
	Room bool
	// Mic is the microphone chosen in the settings, by its Microphone ID,
	// "" for the system's default. A recorder records it wherever it does
	// not follow the meeting app's own microphone.
	Mic string
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

// RoomRecorder is a Recorder that records a meeting in the room (a Call
// with Room set) and lists the microphones it can record. A Recorder that
// is not one is never asked for a room.
type RoomRecorder interface {
	Recorder
	Microphones() ([]Microphone, error)
}

// Microphone is an input device a recording may use.
type Microphone struct {
	// ID is how the audio stack names it, stable while it stays plugged in.
	ID string `json:"id"`
	// Name is how the person knows it.
	Name string `json:"name"`
}

// MicReporter is a Capture that says which microphone it records: its
// name, and whether the one chosen in the settings was missing so the
// default is recorded instead.
type MicReporter interface {
	Microphone() (name string, chosenMissing bool)
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

// Warning is something the person should know while a recording runs and
// can put right: macOS withholding a permission the recording needs.
type Warning struct {
	Text string `json:"text"`
	// Settings names the privacy settings that put it right
	// (SettingsMicrophone, SettingsAudio, SettingsPrivacy), "" for none.
	Settings string `json:"settings,omitempty"`
}

// Warner is a Capture that can raise a Warning while it runs. Warning is
// nil while there is none.
type Warner interface {
	Warning() *Warning
}

// ErrUnsupported is a platform this build cannot record on yet.
var ErrUnsupported = errors.New("meeting notes cannot record on this computer")

// ConsentSentence is said wherever a recording is turned on or started.
const ConsentSentence = "Recording others may need their consent; tell them you are recording."

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
	// GPU runs it on the graphics device numbered Device; without it the
	// run stays on the CPU, even where a graphics backend would load.
	GPU    bool
	Device int
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
	// Backend is the graphics backend the run used ("Vulkan0"), "" when it
	// ran on the CPU. Devices are the graphics devices the engine found,
	// by number.
	Backend string
	Devices []string
	// Words are the segments' words, each timed as the engine heard it, in
	// order; empty where the engine does not report words.
	Words []Word
}

// Word is one word of a transcription, with its time from the start of
// the audio.
type Word struct {
	Text       string
	Start, End time.Duration
}
