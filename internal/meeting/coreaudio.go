package meeting

import (
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// macOS: Core Audio lists every process that uses audio as a process
// object, saying whether it records from an input device and whether it
// plays to an output device. A meeting app runs its audio in helper
// processes (a browser's audio service, Zoom's engine) inside its own
// bundle, so each process is known by the outermost app bundle around its
// executable. What is decided from the listing is here, apart from Core
// Audio, so it is tested on every computer.

// caProcess is one process Core Audio lists.
type caProcess struct {
	Object uint32 // its audio process object
	PID    int
	Input  bool // it records from an input device now
	Output bool // it plays to an output device now
	Path   string
}

// outerApp is the app a process belongs to: the outermost .app bundle in
// its executable's path ("Google Chrome" for a Chrome helper), or the
// executable's own name outside any bundle. key is how the app is known
// from one call to the next, and name how it is shown.
func outerApp(exe string) (key, name string) {
	if exe == "" {
		return "", ""
	}
	if i := strings.Index(exe, ".app/"); i >= 0 {
		name = path.Base(exe[:i])
	} else {
		name = path.Base(exe)
	}
	return strings.ToLower(name), name
}

// processSnapshot puts a Core Audio listing in the shape the call tracker
// reads, which keeps PulseAudio's names: a process that records is a
// microphone stream (a source-output, in Outputs), and one that plays is a
// playback stream (a sink-input, in Inputs), each under its app. A process
// that does neither is left out.
func processSnapshot(ps []caProcess) paSnapshot {
	var s paSnapshot
	for _, p := range ps {
		key, name := outerApp(p.Path)
		st := paStream{App: name, Binary: key, PID: strconv.Itoa(p.PID)}
		if p.Input {
			s.Outputs = append(s.Outputs, st)
		}
		if p.Output {
			s.Inputs = append(s.Inputs, st)
		}
	}
	return s
}

// appObjects are the process objects of the app known by key, in order, so
// a change in the set is seen by comparing two of them.
func appObjects(ps []caProcess, key string) []uint32 {
	var out []uint32
	for _, p := range ps {
		if k, _ := outerApp(p.Path); key != "" && k == key {
			out = append(out, p.Object)
		}
	}
	slices.Sort(out)
	return out
}

// appInput says whether any process of the app known by key records now.
func appInput(ps []caProcess, key string) bool {
	for _, p := range ps {
		if k, _ := outerApp(p.Path); p.Input && k == key {
			return true
		}
	}
	return false
}

// appOutput says whether any process of the app known by key plays now.
func appOutput(ps []caProcess, key string) bool {
	for _, p := range ps {
		if k, _ := outerApp(p.Path); p.Output && k == key {
			return true
		}
	}
	return false
}

// resampler turns mono float samples at one rate into 16 kHz 16-bit ones
// (the same filter as internal/wasapi's converter, which reads a Windows
// device's interleaved formats in a package only Windows builds import):
// a moving average as wide as the rates' ratio, which keeps what is above
// 8 kHz from folding into speech, then linear interpolation. It keeps its
// place between calls.
type resampler struct {
	step float64 // input samples per output sample
	pos  float64 // the next output sample's place, from prev
	prev float64 // the last input sample of the previous call
	win  []float64
	sum  float64
	wi   int
}

func newResampler(rate float64) *resampler {
	r := &resampler{step: rate / SampleRate, pos: 1}
	if w := int(math.Round(r.step)); w > 1 {
		r.win = make([]float64, w)
	}
	return r
}

func (r *resampler) push(in []float32) []int16 {
	n := len(in)
	mono := make([]float64, n)
	for i, v := range in {
		mono[i] = r.smooth(float64(v))
	}
	out := make([]int16, 0, int(float64(n)/r.step)+1)
	// pos counts from prev (0) through mono (1 to n).
	for ; r.pos <= float64(n); r.pos += r.step {
		i := int(r.pos)
		frac := r.pos - float64(i)
		a := r.prev
		if i > 0 {
			a = mono[i-1]
		}
		v := a
		if frac > 0 && i < n {
			v = a + (mono[i]-a)*frac
		}
		out = append(out, toInt16(v))
	}
	r.pos -= float64(n)
	if n > 0 {
		r.prev = mono[n-1]
	}
	return out
}

func (r *resampler) smooth(v float64) float64 {
	if r.win == nil {
		return v
	}
	r.sum += v - r.win[r.wi]
	r.win[r.wi] = v
	r.wi = (r.wi + 1) % len(r.win)
	return r.sum / float64(len(r.win))
}

func toInt16(v float64) int16 {
	v *= 32767
	switch {
	case v > 32767:
		return 32767
	case v < -32768:
		return -32768
	}
	return int16(math.Round(v))
}

// Where macOS keeps the permissions a recording needs, for the link a
// warning carries.
const (
	SettingsMicrophone = "microphone"
	SettingsAudio      = "audio"
	// SettingsPrivacy is Privacy & Security itself, for both at once.
	SettingsPrivacy = "privacy"
)

// silenceWarning is what the person is told when macOS gives a recording
// only silence from the microphone, from the meeting, or from both; nil
// when neither.
func silenceWarning(mic, meeting bool) *Warning {
	const goesOn = "; the recording goes on meanwhile."
	switch {
	case mic && meeting:
		return &Warning{
			Text:     "Agentry is getting only silence from your microphone and from the meeting. If macOS has not allowed them, allow Agentry Desktop under System Settings, Privacy & Security, in both Microphone and Screen & System Audio Recording (System Audio Recording Only)" + goesOn,
			Settings: SettingsPrivacy,
		}
	case mic:
		return &Warning{
			Text:     "Agentry is getting only silence from your microphone. If macOS has not allowed it, allow Agentry Desktop under System Settings, Privacy & Security, Microphone" + goesOn,
			Settings: SettingsMicrophone,
		}
	case meeting:
		return &Warning{
			Text:     "Agentry is getting only silence from the meeting. If macOS has not allowed it, allow Agentry Desktop under System Settings, Privacy & Security, Screen & System Audio Recording, in System Audio Recording Only" + goesOn,
			Settings: SettingsAudio,
		}
	}
	return nil
}

// silenceAfter is how long a side may deliver nothing but exact zeros,
// while there is something it should hear, before the person is told macOS
// may be withholding it. macOS gives a tap or a microphone it has not
// allowed silence, never an error, and has no public way to ask about the
// system-audio permission first.
const silenceAfter = 30 * time.Second

// silenceCheck follows one side of a recording: whether anything but exact
// zeros arrived, and for how long it has been expected. A real microphone
// is never exactly silent; a meeting app that plays and delivers only zeros
// for silenceAfter almost certainly is not being heard.
type silenceCheck struct {
	heard    bool
	expected time.Duration
	last     time.Time
}

// note records a moment of the recording: whether anything was heard since
// the last one, and whether something should have been. It says whether
// the side has now been silent for silenceAfter while it should not have.
func (s *silenceCheck) note(now time.Time, heard, expected bool) bool {
	if heard {
		s.heard = true
	}
	if !s.last.IsZero() && expected {
		s.expected += now.Sub(s.last)
	}
	s.last = now
	return !s.heard && s.expected >= silenceAfter
}
