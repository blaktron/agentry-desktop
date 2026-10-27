package meeting

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The audio child: where the audio stack is reached through code that can
// fault the process (Windows' COM interfaces called by hand), it runs in a
// child process of the app, and only reports what it finds and records.
// What it decides (which device to record, when a call is over) is here,
// apart from the platform, so it is tested on every computer.

// CaptureFlag is the argument the app is started with to run as its own
// audio child.
const CaptureFlag = "--audio-capture"

// endpointFlow is an endpoint's direction.
type endpointFlow int

const (
	flowRender endpointFlow = iota
	flowCapture
)

// audioSession is an app's streams on one endpoint, as the audio stack
// lists them.
type audioSession struct {
	Endpoint string       `json:"endpoint"`
	Flow     endpointFlow `json:"flow"`
	PID      uint32       `json:"pid"`
	Exe      string       `json:"exe,omitempty"`
	App      string       `json:"app,omitempty"`
	Active   bool         `json:"active"`
}

// audioSystem is the audio stack as the child sees it.
type audioSystem interface {
	// Sessions lists every app's sessions on every endpoint.
	Sessions() ([]audioSession, error)
	// Default is the endpoint the system uses by default for flow.
	Default(flow endpointFlow) (string, error)
	// Endpoints lists the active endpoints of flow, by id and the name the
	// system shows for each.
	Endpoints(flow endpointFlow) ([]Microphone, error)
	// Record opens a stream on an endpoint: a capture endpoint's input, or
	// what a render endpoint plays.
	Record(endpoint string, flow endpointFlow) (audioStream, error)
	// Changed waits until an app starts or stops using a microphone, or
	// for timeout.
	Changed(timeout time.Duration)
	Close()
}

// audioStream is one endpoint being recorded.
type audioStream interface {
	// Read returns what arrived since the last Read, 16 kHz mono; an
	// error wrapping errDeviceGone when the endpoint went away.
	Read() ([]int16, error)
	Close()
}

// errDeviceGone is a stream whose device went away or changed shape.
var errDeviceGone = errors.New("the audio device went away")

// binaryName is how an executable is keyed: its file name, lower-cased.
func binaryName(exe string) string {
	if exe == "" {
		return ""
	}
	return strings.ToLower(filepath.Base(strings.ReplaceAll(exe, `\`, "/")))
}

// withoutExe drops the sessions of an executable: this app's own, the
// child's streams included.
func withoutExe(sessions []audioSession, exe string) []audioSession {
	if exe == "" {
		return sessions
	}
	return slices.DeleteFunc(slices.Clone(sessions), func(s audioSession) bool { return strings.EqualFold(s.Exe, exe) })
}

// sessionSnapshot puts the sessions in the shape the calls are found in on
// every platform (findCalls): an app reading a microphone (an active
// capture session) that also plays (a render session of the same process or
// executable). A capture session that is not running counts as a stopped
// stream.
func sessionSnapshot(sessions []audioSession) paSnapshot {
	var s paSnapshot
	for _, x := range sessions {
		st := paStream{App: x.App, Binary: binaryName(x.Exe), PID: strconv.FormatUint(uint64(x.PID), 10), Corked: !x.Active}
		if x.Flow == flowCapture {
			s.Outputs = append(s.Outputs, st)
		} else {
			s.Inputs = append(s.Inputs, st)
		}
	}
	return s
}

// recordTarget is the app whose call is recorded; zero for the whole
// default devices.
type recordTarget struct {
	PID    uint32
	Binary string
}

func (t recordTarget) zero() bool { return t.PID == 0 && t.Binary == "" }

// owns says whether a session is the target app's: its process, or another
// process of the same executable.
func (t recordTarget) owns(s audioSession) bool {
	return (t.PID != 0 && s.PID == t.PID) || (t.Binary != "" && binaryName(s.Exe) == t.Binary)
}

// appEndpoint is the endpoint of flow the target uses: one where a session
// of its process is running, else one of another process of its executable;
// among several, the default; "" when it uses none. An output the app has a
// session on but is not playing to right now (a call's silence, a stream
// stopped between calls) is still its output, taken when it plays to none;
// a microphone counts only while the app reads it.
func appEndpoint(sessions []audioSession, t recordTarget, flow endpointFlow, def string) string {
	if ep := appEndpointIn(sessions, t, flow, def, true); ep != "" || flow == flowCapture {
		return ep
	}
	return appEndpointIn(sessions, t, flow, def, false)
}

func appEndpointIn(sessions []audioSession, t recordTarget, flow endpointFlow, def string, active bool) string {
	var own, same []string
	for _, s := range sessions {
		if s.Flow != flow || s.Active != active || !t.owns(s) {
			continue
		}
		if t.PID != 0 && s.PID == t.PID {
			own = append(own, s.Endpoint)
		} else {
			same = append(same, s.Endpoint)
		}
	}
	for _, eps := range [][]string{own, same} {
		if len(eps) == 0 {
			continue
		}
		if slices.Contains(eps, def) {
			return def
		}
		slices.Sort(eps)
		return eps[0]
	}
	return ""
}

// followAfter is how many scans in a row must show the far side on another
// endpoint before the recording follows it: an app opens a stream on a new
// device before it closes the old.
const followAfter = 2

// scanEvery is how often the recording reads the sessions again, and
// readEvery how often it collects what the streams have.
const (
	scanEvery = time.Second
	readEvery = 20 * time.Millisecond
)

// childRecording is the recording the child keeps: which endpoints are
// recorded, and when the app stopped using its microphone.
type childRecording struct {
	sys    audioSystem
	out    *frameWriter
	self   string
	target recordTarget
	now    func() time.Time

	mic, far     audioStream
	micEP, farEP string
	// appFarEP is the endpoint the app was last seen playing to.
	appFarEP   string
	candidate  string
	seen       int
	farSide    FarSide
	quietSince time.Time
	// room records the microphone alone. fixedMic is the microphone chosen
	// and found when the recording started, "" to follow the default; micName
	// and micMissing say which is recorded and whether the chosen one was
	// missing.
	room       bool
	fixedMic   string
	micName    string
	micMissing bool
	paused     bool
	ended      bool
	logged     map[string]bool
	scanNow    bool
}

func (r *childRecording) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if r.logged[msg] {
		return
	}
	r.logged[msg] = true
	_ = r.out.event(wireEvent{Type: evLog, Message: msg})
}

func (r *childRecording) closeMic() {
	if r.mic != nil {
		r.mic.Close()
	}
	r.mic, r.micEP = nil, ""
}

func (r *childRecording) closeFar() {
	if r.far != nil {
		r.far.Close()
	}
	r.far, r.farEP = nil, ""
}

// scan reads the sessions and brings the streams in line with them. With a
// target app, the microphone is recorded only where the app reads one, so
// it is never opened before the app has, and is let go as soon as the app
// lets go; the far side is what the endpoint the app plays to plays,
// followed when the app moves, or the default output's when the app is
// found playing nowhere. With no target, the default microphone and the
// default output are recorded, followed when the defaults change, the
// microphone chosen standing for the default one when it was found. A
// meeting in the room records that microphone alone, no output at all.
//
// While paused nothing is recorded, and the app is still watched, so a call
// that ends during a pause ends the recording.
func (r *childRecording) scan(first bool) error {
	defRender, _ := r.sys.Default(flowRender)
	var micEP, farEP string
	if r.target.zero() {
		micEP = r.fixedMic
		if micEP == "" {
			micEP, _ = r.sys.Default(flowCapture)
		}
		farEP = defRender
	} else {
		sessions, err := r.sys.Sessions()
		if err != nil {
			return err
		}
		sessions = withoutExe(sessions, r.self)
		micEP = appEndpoint(sessions, r.target, flowCapture, "")
		farEP = appEndpoint(sessions, r.target, flowRender, defRender)
	}
	if !r.target.zero() {
		r.checkEnded(micEP != "")
		if farEP != "" {
			r.appFarEP = farEP
		} else {
			// Playing nowhere for now: the device it played to last.
			farEP = r.appFarEP
		}
	}
	if r.paused {
		return nil
	}
	if err := r.followMic(micEP); err != nil && first {
		return err
	}
	if r.room {
		return nil
	}
	if err := r.followFar(farEP, defRender, first); err != nil && first {
		return err
	}
	return nil
}

func (r *childRecording) followMic(ep string) error {
	if ep == r.micEP && (r.mic != nil || ep == "") {
		return nil
	}
	if r.micEP != "" && ep != "" {
		r.log("the microphone moved to another device")
	}
	r.closeMic()
	if ep == "" {
		return nil
	}
	s, err := r.sys.Record(ep, flowCapture)
	if err != nil {
		r.log("the microphone could not be recorded: %v", err)
		return fmt.Errorf("the microphone could not be recorded: %w", err)
	}
	r.mic, r.micEP = s, ep
	return nil
}

func (r *childRecording) followFar(ep, def string, first bool) error {
	whole := false
	if ep == "" {
		if r.far != nil {
			// The app plays nowhere for now: keep what is recorded.
			return nil
		}
		if r.target.zero() {
			ep = def
		} else {
			ep, whole = def, true
		}
	}
	if ep == "" {
		return errors.New("this computer has no output device")
	}
	switch {
	case ep == r.farEP && r.far != nil:
		r.candidate, r.seen = "", 0
		return nil
	case r.far != nil:
		if ep != r.candidate {
			r.candidate, r.seen = ep, 0
		}
		r.seen++
		if r.seen < followAfter {
			return nil
		}
	}
	s, err := r.sys.Record(ep, flowRender)
	if err != nil {
		r.log("the sound played could not be recorded: %v", err)
		return fmt.Errorf("the sound played could not be recorded: %w", err)
	}
	if r.far != nil {
		r.log("the far side moved to another device")
	}
	// The new stream is open before the old one closes.
	r.closeFar()
	r.far, r.farEP, r.candidate, r.seen = s, ep, "", 0
	side := FarSideAppDevice
	if r.target.zero() || whole {
		side = FarSideWholeDevice
	}
	if r.farSide == FarSideWholeDevice {
		return nil
	}
	if side != r.farSide {
		r.farSide = side
		if !first {
			_ = r.out.event(wireEvent{Type: evFarSide, FarSide: side})
		}
	}
	return nil
}

// chooseMic settles, as a recording that follows no app starts, which
// microphone it reads: the one chosen when it is there, else the default,
// and says which.
func (r *childRecording) chooseMic(chosen string) error {
	mics, err := r.sys.Endpoints(flowCapture)
	if err != nil {
		return fmt.Errorf("the microphones could not be listed: %w", err)
	}
	def, _ := r.sys.Default(flowCapture)
	id, name, missing := pickMic(mics, chosen, def)
	if id == "" {
		return errors.New("this computer has no microphone")
	}
	if id != def {
		r.fixedMic = id
	}
	r.micName, r.micMissing = name, missing
	return nil
}

// checkEnded says the call is over once the app has not used a microphone
// for callEndWait.
func (r *childRecording) checkEnded(reading bool) {
	if reading || r.ended {
		r.quietSince = time.Time{}
		return
	}
	if r.quietSince.IsZero() {
		r.quietSince = r.now()
		return
	}
	if r.now().Sub(r.quietSince) >= callEndWait {
		r.ended = true
		_ = r.out.event(wireEvent{Type: evEnded})
	}
}

// collect sends what the streams have; a stream whose device went away is
// closed and taken up again at the next scan.
func (r *childRecording) collect() {
	for _, x := range []struct {
		s     *audioStream
		track byte
		drop  func()
	}{{&r.mic, wireMic, r.closeMic}, {&r.far, wireFar, r.closeFar}} {
		if *x.s == nil {
			continue
		}
		samples, err := (*x.s).Read()
		if len(samples) > 0 {
			_ = r.out.audio(x.track, samples)
		}
		if err != nil {
			r.log("a device went away: %v", err)
			x.drop()
			r.scanNow = true
		}
	}
}

func (r *childRecording) setPaused(paused bool) {
	r.paused = paused
	if paused {
		r.closeMic()
		r.closeFar()
	}
	r.scanNow = true
}

// captureMain runs the child: the first command says what to do, and the
// child does it until its input closes.
func captureMain(in io.Reader, out io.Writer, open func() (audioSystem, error), self string) int {
	w := &frameWriter{w: out}
	cmds := make(chan wireCommand, 8)
	go func() {
		defer close(cmds)
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			var c wireCommand
			if json.Unmarshal(sc.Bytes(), &c) == nil {
				cmds <- c
			}
		}
	}()
	first, ok := <-cmds
	if !ok {
		return 0
	}
	sys, err := open()
	if err != nil {
		_ = w.event(wireEvent{Type: evError, Message: "the audio system could not be reached: " + err.Error()})
		return 1
	}
	defer sys.Close()
	switch first.Cmd {
	case cmdDetect:
		return detectLoop(sys, w, cmds, self)
	case cmdRecord:
		return recordLoop(sys, w, cmds, self, first)
	case cmdMics:
		mics, err := sys.Endpoints(flowCapture)
		if err != nil {
			_ = w.event(wireEvent{Type: evError, Message: "the microphones could not be listed: " + err.Error()})
			return 1
		}
		if mics == nil {
			mics = []Microphone{}
		}
		_ = w.event(wireEvent{Type: evMics, Mics: mics})
		return 0
	}
	_ = w.event(wireEvent{Type: evError, Message: fmt.Sprintf("an unknown command %q", first.Cmd)})
	return 2
}

// detectLoop reports the sessions, this app's left out, whenever they
// change. It reads them again when an app starts or stops using a
// microphone, every second while one is in use, and every few seconds
// otherwise.
func detectLoop(sys audioSystem, w *frameWriter, cmds <-chan wireCommand, self string) int {
	var last []byte
	for {
		select {
		case _, ok := <-cmds:
			if !ok {
				return 0
			}
		default:
		}
		sessions, err := sys.Sessions()
		if err != nil {
			_ = w.event(wireEvent{Type: evError, Message: "the audio sessions could not be read: " + err.Error()})
			return 1
		}
		sessions = withoutExe(sessions, self)
		b, _ := json.Marshal(sessions)
		if string(b) != string(last) {
			if w.event(wireEvent{Type: evSessions, Sessions: sessions}) != nil {
				return 1
			}
			last = b
		}
		wait := 5 * time.Second
		if slices.ContainsFunc(sessions, func(s audioSession) bool { return s.Flow == flowCapture && s.Active }) {
			wait = scanEvery
		}
		sys.Changed(wait)
	}
}

// recordLoop records until the parent closes the child's input.
func recordLoop(sys audioSystem, w *frameWriter, cmds <-chan wireCommand, self string, first wireCommand) int {
	r := &childRecording{sys: sys, out: w, self: self, now: time.Now, logged: map[string]bool{}}
	pid, _ := strconv.ParseUint(first.PID, 10, 32)
	r.target = recordTarget{PID: uint32(pid), Binary: strings.ToLower(first.Binary)}
	var call *Call
	r.room = first.Room
	if r.room {
		// A room follows no app, whatever else the command names.
		r.target = recordTarget{}
	}
	if r.target.zero() && !r.room {
		// The call under way, if there is one.
		if sessions, err := sys.Sessions(); err == nil {
			if c, ok := findCall(sessionSnapshot(withoutExe(sessions, self)), ""); ok {
				call = &c
				pid, _ := strconv.ParseUint(c.PID, 10, 32)
				r.target = recordTarget{PID: uint32(pid), Binary: c.Binary}
			}
		}
	}
	r.paused = first.Paused
	if r.room {
		r.farSide = FarSideNone
	}
	if r.target.zero() {
		if err := r.chooseMic(first.Mic); err != nil {
			_ = w.event(wireEvent{Type: evError, Message: err.Error()})
			return 1
		}
	}
	if err := r.scan(true); err != nil {
		r.closeMic()
		r.closeFar()
		_ = w.event(wireEvent{Type: evError, Message: err.Error()})
		return 1
	}
	if err := w.event(wireEvent{Type: evStarted, Call: call, FarSide: r.farSide, MicName: r.micName, MicMissing: r.micMissing}); err != nil {
		return 1
	}
	defer func() { r.closeMic(); r.closeFar() }()
	lastScan := r.now()
	tick := time.NewTicker(readEvery)
	defer tick.Stop()
	for {
		select {
		case c, ok := <-cmds:
			if !ok {
				return 0
			}
			if c.Cmd == cmdPause {
				r.setPaused(c.Paused)
			}
		case <-tick.C:
		}
		r.collect()
		if r.scanNow || r.now().Sub(lastScan) >= scanEvery {
			r.scanNow = false
			lastScan = r.now()
			if err := r.scan(false); err != nil {
				r.log("the audio sessions could not be read: %v", err)
			}
		}
	}
}
