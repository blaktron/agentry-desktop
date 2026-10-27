package meeting

import (
	"context"
	"errors"
	"log"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"
)

// The macOS capture's decisions, apart from Core Audio, so they are tested
// on every computer: recorder_darwin.go is the caSystem that reaches the
// real one.

// caSystem is what the capture asks of Core Audio.
type caSystem interface {
	// Processes is Core Audio's process list.
	Processes() ([]caProcess, error)
	// DefaultDevice is the default input (input true) or output device.
	DefaultDevice(input bool) uint32
	// SelfObject is this app's own process object, 0 when it has none.
	SelfObject() uint32
	// MicAllowed says whether macOS lets this app use the microphone.
	MicAllowed() bool
	// AskMic has macOS ask the person for the microphone, if it has not
	// asked yet, without waiting for the answer.
	AskMic()
	// StartTap taps the processes objects names, or with global every
	// process but those.
	StartTap(objects []uint32, global bool) (caSource, error)
	// Mics lists the input devices, by UID and name, and says which is
	// the default.
	Mics() (mics []Microphone, def string, err error)
	// StartMic reads the input device with UID uid, the default input when
	// uid is "".
	StartMic(uid string) (caSource, error)
}

// caSource is one running capture: mono float samples at its own rate.
type caSource interface {
	Rate() float64
	// Read moves up to len(buf) samples out and says how many.
	Read(buf []float32) int
	// Frames is how many frames Core Audio delivered, and Heard how many
	// samples of those were not exact silence.
	Frames() uint64
	Heard() uint64
	Stop()
}

// The capture's pace: the sources are drained every drainEvery, and the
// processes and devices are read again every lookEvery.
const (
	drainEvery = 50 * time.Millisecond
	lookEvery  = time.Second
)

// tapStall is how long a tap may deliver nothing before it is built again:
// an aggregate device delivers on the output's clock, silence included, so
// a tap that stops has lost its device.
const tapStall = 3 * time.Second

// caStream is one running source, the converter of its samples, and what
// was last seen of it.
type caStream struct {
	src    caSource
	rs     *resampler
	frames uint64    // frames delivered when last looked at
	moved  time.Time // when frames last grew
	heard  uint64    // samples heard when last looked at
}

func newStream(src caSource, now time.Time) *caStream {
	return &caStream{src: src, rs: newResampler(src.Rate()), moved: now}
}

// drain moves what the source holds to t, or drops it when t is nil.
func (s *caStream) drain(t Track, buf []float32) {
	for {
		n := s.src.Read(buf)
		if n <= 0 {
			return
		}
		if t != nil {
			t.Write(s.rs.push(buf[:n]))
		}
	}
}

// look says whether the stream has delivered nothing for tapStall, and
// whether anything but exact silence arrived since it was last looked at.
func (s *caStream) look(now time.Time) (stalled, heard bool) {
	f, h := s.src.Frames(), s.src.Heard()
	if f != s.frames {
		s.frames, s.moved = f, now
	}
	heard = h != s.heard
	s.heard = h
	return now.Sub(s.moved) >= tapStall, heard
}

// caCapture is a recording through Core Audio.
//
// With a call under way it records the far side through a process tap of
// the meeting app's own processes; the tap is built again when the app's
// processes or the default output change, or when it stops delivering. A
// tap of the app that cannot be made falls back to a tap of everything but
// this app, the whole device, which the recording says from then on. An app
// whose processes have all gone leaves nothing tapped, and nothing else is
// taken up in its place. The microphone is the default input, read only
// while the app reads one. With no call it records the microphone chosen
// (else the default input) and everything but this app, the whole device. A
// meeting in the room records that microphone alone: no tap, no far side.
// A pause lets go of every device; the call's end is followed meanwhile.
type caCapture struct {
	sys    caSystem
	call   Call
	key    string // the app's key, "" when the whole devices are recorded
	tracks Tracks
	me     int    // this app's process id
	self   uint32 // this app's own process object

	// mu is the capture's state as others read it. The manager asks for it
	// under its own lock and opening a track takes that lock, so no track is
	// opened or written under mu.
	mu      sync.Mutex
	farSide FarSide
	paused  bool
	warning *Warning
	// micName and micMissing say which microphone a recording that follows
	// no app reads, and whether the one chosen was missing.
	micName    string
	micMissing bool

	// room is a meeting in the room; micUID the microphone chosen and found
	// when it started, "" to follow the default input.
	room   bool
	micUID string

	// The rest belongs to run, and to the start and Stop before and after it.
	far, mic           *caStream
	farObjects         []uint32
	farGlobal          bool // the far side is everything but this app
	outDev, inDev      uint32
	micAllowed         bool // macOS allowed the microphone when it was opened
	micOpened          bool
	farTrack, micTrack Track
	farQuiet, micQuiet silenceCheck
	inputGone          time.Time
	lastErr            string

	ended   chan struct{}
	endOnce sync.Once
	quit    chan struct{}
	done    chan struct{}
	stopMu  sync.Mutex
	stopped bool
}

// startCoreAudio starts recording call (with a zero Call, the call under
// way or the whole default devices) and returns before anything is drained;
// run follows it.
func startCoreAudio(sys caSystem, call Call, tracks Tracks, now time.Time) (*caCapture, error) {
	me := os.Getpid()
	ps, err := sys.Processes()
	if err != nil {
		return nil, err
	}
	if call.Room {
		// A room follows no app, whatever else the call names.
		call = Call{Room: true, Mic: call.Mic}
	} else if call.PID == "" {
		found, _ := findCall(processSnapshot(ps), strconv.Itoa(me))
		found.Mic = call.Mic
		call = found
	}
	c := &caCapture{
		sys: sys, call: call, key: call.AppKey(), tracks: tracks, me: me, self: sys.SelfObject(), room: call.Room,
		ended: make(chan struct{}), quit: make(chan struct{}), done: make(chan struct{}),
	}
	if c.key == "" {
		if err := c.chooseMic(call.Mic); err != nil {
			return nil, err
		}
	}
	input := c.key == "" || appInput(ps, c.key)
	if input {
		// macOS's own prompt, never waited for: the recording is announced
		// meanwhile, and the microphone is opened again once it is allowed.
		sys.AskMic()
	}
	if c.room {
		// The microphone is all a room records: one that cannot be opened
		// fails the start, as it does where the recorder is asked for it.
		c.farSide = FarSideNone
		if err := c.startMic(now); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err := c.startFar(ps, now); err != nil {
		return nil, err
	}
	// The far side has a track from the start: a far side silent throughout
	// is a silent track in the transcript, not a missing one.
	far, err := tracks.Open(Them, "far")
	if err != nil {
		c.far.src.Stop()
		return nil, err
	}
	c.farTrack = far
	if input {
		// A call's microphone that cannot be opened yet is tried again at
		// each look.
		_ = c.startMic(now)
	}
	return c, nil
}

// recordCoreAudio starts recording through sys and follows the recording
// until Stop or ctx's end.
func recordCoreAudio(ctx context.Context, sys caSystem, call Call, tracks Tracks) (*caCapture, error) {
	c, err := startCoreAudio(sys, call, tracks, time.Now())
	if err != nil {
		return nil, err
	}
	go c.run(ctx)
	return c, nil
}

// startFar taps the app's processes, or everything but this app when
// there is no app or its tap cannot be made.
func (c *caCapture) startFar(ps []caProcess, now time.Time) error {
	c.outDev = c.sys.DefaultDevice(false)
	if c.key != "" && !c.farGlobal {
		if objs := appObjects(ps, c.key); len(objs) > 0 {
			src, err := c.sys.StartTap(objs, false)
			if err == nil {
				c.far, c.farObjects = newStream(src, now), objs
				c.setFarSide(FarSideApp)
				return nil
			}
			c.logOnce(err)
		}
	}
	var exclude []uint32
	if c.self != 0 {
		exclude = []uint32{c.self}
	}
	src, err := c.sys.StartTap(exclude, true)
	if err != nil {
		c.logOnce(err)
		return err
	}
	c.far, c.farObjects, c.farGlobal = newStream(src, now), nil, true
	c.setFarSide(FarSideWholeDevice)
	return nil
}

// logOnce logs a failure to take up a device, once until another one.
func (c *caCapture) logOnce(err error) {
	if err.Error() != c.lastErr {
		c.lastErr = err.Error()
		log.Printf("meeting notes: %v", err)
	}
}

// setFarSide says how the far side is recorded; once any of it was the
// whole device, the recording says so.
func (c *caCapture) setFarSide(f FarSide) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.farSide != FarSideWholeDevice {
		c.farSide = f
	}
}

func (c *caCapture) startMic(now time.Time) error {
	src, err := c.sys.StartMic(c.micUID)
	if err != nil {
		c.logOnce(err)
		return err
	}
	c.mic, c.inDev = newStream(src, now), c.sys.DefaultDevice(true)
	c.micAllowed, c.micOpened = c.sys.MicAllowed(), true
	if c.micTrack == nil {
		label, name := Me, "mic"
		if c.room {
			label, name = Room, "room"
		}
		t, err := c.tracks.Open(label, name)
		if err != nil {
			// A source with no track is read for nothing.
			c.mic.src.Stop()
			c.mic = nil
			return err
		}
		c.micTrack = t
	}
	return nil
}

// chooseMic settles, as a recording that follows no app starts, which
// microphone it reads: the one chosen when it is there, else the default
// input, and says which. def is never empty in a room's id: the start
// fails instead.
func (c *caCapture) chooseMic(chosen string) error {
	mics, def, err := c.sys.Mics()
	if err != nil {
		return err
	}
	id, name, missing := pickMic(mics, chosen, def)
	if id == "" {
		return errors.New("this Mac has no microphone")
	}
	// A room keeps the microphone it starts on, chosen or the default, so
	// a new default input never moves it; a chosen microphone that is
	// there is kept the same way. Otherwise the default input is followed.
	if c.room || (chosen != "" && !missing) {
		c.micUID = id
	}
	c.mu.Lock()
	c.micName, c.micMissing = name, missing
	c.mu.Unlock()
	return nil
}

// stopMic and stopFar let go of a device, keeping what its source still
// holds unless the recording is paused.
func (c *caCapture) stopMic() {
	if c.mic != nil {
		c.mic.drain(c.unlessPaused(c.micTrack), make([]float32, 4096))
		c.mic.src.Stop()
		c.mic = nil
	}
}

func (c *caCapture) stopFar() {
	if c.far != nil {
		c.far.drain(c.unlessPaused(c.farTrack), make([]float32, 4096))
		c.far.src.Stop()
		c.far = nil
	}
}

// unlessPaused is t, or nil while the recording is paused: what a device
// delivers then is read and dropped.
func (c *caCapture) unlessPaused(t Track) Track {
	if c.isPaused() {
		return nil
	}
	return t
}

func (c *caCapture) isPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

// drain moves what both sources hold to their tracks.
func (c *caCapture) drain(buf []float32) {
	if c.far != nil {
		c.far.drain(c.unlessPaused(c.farTrack), buf)
	}
	if c.mic != nil {
		c.mic.drain(c.unlessPaused(c.micTrack), buf)
	}
}

// run drains the sources and follows the app and the devices until Stop
// or ctx's end, and lets go of every device as it returns.
func (c *caCapture) run(ctx context.Context) {
	defer close(c.done)
	defer c.stopMic()
	defer c.stopFar()
	buf := make([]float32, 16384)
	tick := time.NewTicker(drainEvery)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.quit:
			return
		case now := <-tick.C:
			c.drain(buf)
			if now.Sub(last) >= lookEvery {
				last = now
				c.look(now)
			}
		}
	}
}

// look reads the processes and the devices again: the call's end, a pause,
// the app's microphone, a change of the app's processes or of a default
// device, a stalled tap, and silence.
func (c *caCapture) look(now time.Time) {
	ps, err := c.sys.Processes()
	if err != nil {
		return
	}
	// The call ends once the app has read no microphone for callEndWait,
	// paused or not.
	input := c.key == "" || appInput(ps, c.key)
	if c.key != "" {
		switch {
		case input:
			c.inputGone = time.Time{}
		case c.inputGone.IsZero():
			c.inputGone = now
		case now.Sub(c.inputGone) >= callEndWait:
			c.endOnce.Do(func() { close(c.ended) })
		}
	}
	if c.isPaused() {
		c.stopFar()
		c.stopMic()
		return
	}
	// The microphone, only while the app reads one, opened again when the
	// default input changes or macOS has just allowed it.
	switch {
	case input && c.mic == nil:
		_ = c.startMic(now)
	case !input && c.mic != nil:
		c.stopMic()
	case input && ((c.micUID == "" && c.sys.DefaultDevice(true) != c.inDev) || (!c.micAllowed && c.sys.MicAllowed())):
		c.stopMic()
		_ = c.startMic(now)
	}
	// The far side; a room has none.
	var farHeard, playing bool
	if !c.room {
		farHeard = c.lookFar(ps, now)
		playing = appOutput(ps, c.key)
		if c.key == "" {
			playing = othersPlay(ps, c.me)
		}
	}
	// Silence: macOS gives a recording it has not allowed only zeros. A
	// side heard once is not warned about again.
	var micHeard bool
	if c.mic != nil {
		_, micHeard = c.mic.look(now)
	}
	farSilent := c.farQuiet.note(now, farHeard, playing)
	micQuiet := c.micQuiet.note(now, micHeard, c.mic != nil)
	micSilent := c.micOpened && !c.micQuiet.heard && (micQuiet || !c.sys.MicAllowed())
	w := silenceWarning(micSilent, farSilent)
	c.mu.Lock()
	c.warning = w
	c.mu.Unlock()
}

// lookFar rebuilds the far side's tap where it must be built again, and
// says whether anything but exact silence arrived on it.
func (c *caCapture) lookFar(ps []caProcess, now time.Time) (heard bool) {
	tapsApp := c.key != "" && !c.farGlobal
	objs := appObjects(ps, c.key)
	rebuild := c.far == nil
	if c.far != nil {
		var stalled bool
		stalled, heard = c.far.look(now)
		rebuild = stalled || c.sys.DefaultDevice(false) != c.outDev
	}
	if tapsApp && !slices.Equal(objs, c.farObjects) {
		rebuild = true
	}
	switch {
	case tapsApp && len(objs) == 0:
		c.stopFar()
		c.farObjects = nil
	case rebuild:
		c.stopFar()
		_ = c.startFar(ps, now)
	}
	return heard
}

// othersPlay says whether any process but this app plays now.
func othersPlay(ps []caProcess, me int) bool {
	for _, p := range ps {
		if p.Output && p.PID != me {
			return true
		}
	}
	return false
}

func (c *caCapture) Call() Call { return c.call }

func (c *caCapture) FarSide() FarSide {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.farSide
}

func (c *caCapture) Warning() *Warning {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warning
}

func (c *caCapture) Ended() <-chan struct{} { return c.ended }

func (c *caCapture) Microphone() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.micName, c.micMissing
}

// SetPaused lets go of the devices, or takes them up again, within a
// second; meanwhile nothing reaches the tracks.
func (c *caCapture) SetPaused(paused bool) error {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopped {
		return errors.New("the recording has stopped")
	}
	c.mu.Lock()
	c.paused = paused
	c.mu.Unlock()
	return nil
}

// Stop ends every track and lets go of the devices.
func (c *caCapture) Stop() error {
	c.stopMu.Lock()
	if c.stopped {
		c.stopMu.Unlock()
		return nil
	}
	c.stopped = true
	close(c.quit)
	c.stopMu.Unlock()
	<-c.done
	for _, t := range []Track{c.micTrack, c.farTrack} {
		if t != nil {
			t.Write(nil)
			t.End()
		}
	}
	return nil
}

// CoreAudioDetector reads Core Audio's process list once a second: an app,
// not this one, whose processes both record from an input device and play
// to an output device has a call under way (the call rule of every
// platform, findCalls). Reading the list needs no permission and opens no
// device.
type CoreAudioDetector struct {
	// endWait is how long an app may stop recording before its call is
	// over; 0 is callEndWait.
	endWait time.Duration
	// list reads the processes.
	list func() ([]caProcess, error)
	// every is how often; 0 is lookEvery.
	every time.Duration
}

// Watch reports calls starting and ending until ctx ends, or until the
// list cannot be read, which it returns.
func (d CoreAudioDetector) Watch(ctx context.Context, events chan<- CallEvent) error {
	if d.list == nil {
		return ErrUnsupported
	}
	tracker := newCallTracker(strconv.Itoa(os.Getpid()))
	if d.endWait > 0 {
		tracker.endWait = d.endWait
	}
	every := d.every
	if every == 0 {
		every = lookEvery
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		ps, err := d.list()
		if err != nil {
			return err
		}
		for _, ev := range tracker.update(processSnapshot(ps), time.Now()) {
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
