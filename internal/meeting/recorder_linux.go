package meeting

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jfreymuth/pulse/proto"
)

// NewRecorder is this platform's recorder: the PulseAudio protocol, which
// PipeWire's pipewire-pulse also serves.
func NewRecorder() Recorder { return PulseRecorder{} }

// PulseRecorder records over the PulseAudio protocol.
//
// With a call under way it records the meeting app's own playback streams,
// each on its own track and each tied to its stream (NoMove): a stream that
// goes away ends its track, and the recording is never moved onto whatever
// else the sink plays. The microphone track reads the source the app
// reads, and only while the app reads it: when the app's stream goes, the
// microphone stops being recorded at once. With no call it records the
// chosen microphone (the default when none is chosen or it is missing) and
// the monitor of the default output, the whole device, and says so. A
// meeting in the room records the chosen microphone alone. Where the
// recording reads a microphone of its own like this, one that goes away
// gives way to the default at once and is taken up again when it is back
// (micFollow), and each change is said.
type PulseRecorder struct {
	// Server is the server to connect to; "" for the session's own.
	Server string
}

// fragment is the size of the pieces the server sends: 100 ms. The
// server's default holds the first samples back for about two seconds,
// which would put the two sides out of step.
const fragment = SampleRate / 10 * 2

// endWait is how long a recording of an app goes on after the app has
// stopped reading the microphone, in case it comes back (a call rejoined, a
// device switched). Playing alone does not keep it going: a browser plays
// other tabs' sound long after its call.
const endWait = callEndWait

// Record starts recording.
func (r PulseRecorder) Record(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
	c, conn, err := proto.Connect(r.Server)
	if err != nil {
		return nil, fmt.Errorf("the audio system could not be reached: %w", err)
	}
	c.SetTimeout(5 * time.Second)
	p := &pulseCapture{
		c: c, conn: conn, tracks: tracks, me: strconv.Itoa(os.Getpid()),
		changed: make(chan struct{}, 1), done: make(chan struct{}), ended: make(chan struct{}),
		lostInputs: map[uint32]bool{}, farByInput: map[uint32]*paRecord{},
		waiting: map[uint32]Track{}, failures: map[uint32]int{},
		monitors: map[uint32]*paRecord{}, monitorTracks: map[uint32]Track{},
	}
	c.Callback = p.callback
	props := proto.PropList{
		"application.name":       proto.PropListString("Agentry Desktop"),
		"application.process.id": proto.PropListString(p.me),
	}
	if err := c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("the audio system refused the connection: %w", err)
	}
	mask := proto.SubscriptionMaskSinkInput | proto.SubscriptionMaskSourceInput | proto.SubscriptionMaskSink | proto.SubscriptionMaskSource
	if err := c.Request(&proto.Subscribe{Mask: mask}, nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	snap, err := p.list()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if call.Room {
		p.call, p.farSide = call, FarSideNone
		if err := p.recordRoom(call.Mic); err != nil {
			p.stopAll()
			return nil, err
		}
		go p.watch(ctx)
		return p, nil
	}
	mic := call.Mic
	if call.PID == "" {
		call, _ = findCall(snap, p.me)
	}
	p.call = call
	if call.PID != "" {
		p.farSide = FarSideApp
		if err := p.reconcile(snap); err != nil {
			p.stopAll()
			return nil, err
		}
	} else {
		p.farSide = FarSideWholeDevice
		if err := p.recordWholeDevice(mic); err != nil {
			p.stopAll()
			return nil, err
		}
	}
	go p.watch(ctx)
	return p, nil
}

// paRecord is one record stream of ours and the track it writes.
type paRecord struct {
	stream uint32 // our record stream's index
	output uint32 // the source-output the server lists it as
	track  Track
	source uint32 // the source it reads
}

type pulseCapture struct {
	c      *proto.Client
	conn   net.Conn
	tracks Tracks
	me     string

	// streams maps our record streams' indexes to their tracks, for the
	// data the server sends. The callback runs on the connection's reader
	// and reads only this, so it never waits on a lock held across a
	// request (whose answer that same reader delivers).
	streams sync.Map

	mu         sync.Mutex // everything below
	call       Call
	farSide    FarSide
	paused     bool
	stopped    bool
	mic        *paRecord
	micTrack   Track
	farByInput map[uint32]*paRecord
	lostInputs map[uint32]bool // playback streams recorded and gone: never taken up again
	// waiting are the tracks of playback streams that could not be
	// recorded yet, to be tried again, and failures how often each failed
	// while it played.
	waiting  map[uint32]Track
	failures map[uint32]int
	whole    []*paRecord
	// monitors record a sink's whole output where one of the app's
	// playback streams could not be recorded on its own, by sink; their
	// tracks, kept for a monitor taken up again.
	monitors      map[uint32]*paRecord
	monitorTracks map[uint32]Track
	quietSince    time.Time
	// micName is the microphone a recording with no app reads, by its
	// description, and micMissing that the one chosen was missing.
	micName    string
	micMissing bool
	// own is the record stream of the microphone a recording with no app
	// reads, nil while none is read, and follow which one it reads.
	own    *paRecord
	follow micFollow
	// killed are our record streams the server killed: their device went
	// away. The callback writes it, so it is not under mu.
	killed sync.Map

	changed chan struct{}
	done    chan struct{}
	ended   chan struct{}
	endOnce sync.Once
}

func (p *pulseCapture) Call() Call { return p.call }

func (p *pulseCapture) FarSide() FarSide {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.farSide
}

func (p *pulseCapture) Ended() <-chan struct{} { return p.ended }

func (p *pulseCapture) Microphone() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.micName, p.micMissing
}

func (p *pulseCapture) callback(msg any) {
	switch m := msg.(type) {
	case *proto.DataPacket:
		if t, ok := p.streams.Load(m.StreamIndex); ok {
			t.(Track).Write(int16s(m.Data))
		}
	case *proto.RecordStreamKilled:
		p.killed.Store(m.StreamIndex, true)
		p.poke()
	case *proto.SubscribeEvent:
		p.poke()
	case *proto.ConnectionClosed:
		p.poke()
	}
}

func (p *pulseCapture) poke() {
	select {
	case p.changed <- struct{}{}:
	default:
	}
}

// watch follows the streams until the recording stops.
func (p *pulseCapture) watch(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case <-p.changed:
		case <-tick.C:
		}
		// A recording of the whole devices, or of a room, has no app to
		// follow, only its microphone. One that follows an app goes on
		// following it after part of its far side widened to its sink's
		// monitor, so it still ends with the call.
		if p.call.PID == "" {
			p.followMic(time.Now())
			continue
		}
		snap, err := p.list()
		if err != nil {
			continue
		}
		_ = p.reconcile(snap)
	}
}

// reconcile brings the recording in line with the app's streams: the
// microphone followed to the source the app reads now and dropped when the
// app stops reading; each playback stream that is flowing taken up once
// while the app reads the microphone, and one that is gone ended for good.
func (p *pulseCapture) reconcile(s paSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return nil
	}
	pid := p.call.PID
	o, reading := appMic(s, pid)
	if reading && o.Corked {
		reading = false
	}
	switch {
	case !reading && p.mic != nil:
		p.dropLocked(p.mic)
		p.mic = nil
	case reading && p.mic != nil && p.mic.source != o.Device:
		p.dropLocked(p.mic)
		p.mic = nil
		fallthrough
	case reading && p.mic == nil:
		if p.micTrack == nil {
			t, err := p.tracks.Open(Me, "mic")
			if err != nil {
				return err
			}
			p.micTrack = t
		}
		rec, err := p.openLocked(p.micTrack, o.Device, "", proto.Undefined, "Agentry meeting notes (microphone)", false)
		if err != nil {
			return err
		}
		p.mic = rec
	}
	for in, rec := range p.farByInput {
		if !hasInput(s, in) || !alive(s, rec) {
			p.dropLocked(rec)
			rec.track.End()
			delete(p.farByInput, in)
			p.lostInputs[in] = true
		}
	}
	for _, in := range appPlayback(s, pid) {
		if p.mic == nil || p.farByInput[in.Index] != nil || p.lostInputs[in.Index] {
			continue
		}
		mon, ok := s.MonitorOf[in.Device]
		if !ok {
			continue
		}
		t := p.waiting[in.Index]
		if t == nil {
			var err error
			if t, err = p.tracks.Open(Them, fmt.Sprintf("far-%d", in.Index)); err != nil {
				return err
			}
		}
		rec, err := p.openLocked(t, mon, "", in.Index, "Agentry meeting notes ("+in.App+")", false)
		if err != nil {
			// An app corks and drops streams as a call settles, so a
			// stream that could not be recorded widens the recording only
			// when it is still there, playing, and failed twice.
			listed, corked := p.inputState(in.Index)
			switch {
			case !listed:
				p.loseLocked(in.Index, t)
			case corked:
				// Taken up when it plays again.
				p.waiting[in.Index] = t
			case p.failures[in.Index] == 0:
				p.failures[in.Index]++
				p.waiting[in.Index] = t
			default:
				p.loseLocked(in.Index, t)
				// The stream plays on but cannot be recorded on its own:
				// the whole of its sink is, and the recording says so.
				if err := p.recordMonitorLocked(in.Device, mon); err != nil {
					return err
				}
			}
			continue
		}
		delete(p.waiting, in.Index)
		p.farByInput[in.Index] = rec
	}
	for in, t := range p.waiting {
		if !hasInput(s, in) {
			p.loseLocked(in, t)
		}
	}
	if len(appPlayback(s, pid)) == 0 {
		for sink, rec := range p.monitors {
			p.dropLocked(rec)
			delete(p.monitors, sink)
		}
	}
	if p.mic == nil {
		if p.quietSince.IsZero() {
			p.quietSince = time.Now()
		} else if time.Since(p.quietSince) > endWait {
			p.endOnce.Do(func() { close(p.ended) })
		}
	} else {
		p.quietSince = time.Time{}
	}
	return nil
}

// alive says whether our record stream is still listed: the server kills
// a stream tied to a playback stream that went away.
func alive(s paSnapshot, rec *paRecord) bool {
	for _, o := range s.Outputs {
		if o.Index == rec.output {
			return true
		}
	}
	return false
}

// inputState asks whether a playback stream is still there, and whether it
// is corked.
func (p *pulseCapture) inputState(index uint32) (listed, corked bool) {
	var reply proto.GetSinkInputInfoReply
	if p.c.Request(&proto.GetSinkInputInfo{SinkInputIndex: index}, &reply) != nil {
		return false, false
	}
	return true, reply.Corked
}

// loseLocked ends a playback stream's track for good: the stream went, or
// could not be recorded on its own.
func (p *pulseCapture) loseLocked(index uint32, t Track) {
	t.End()
	delete(p.waiting, index)
	p.lostInputs[index] = true
}

// recordMonitorLocked records the whole output of sink, through its monitor
// source mon, unless it is already recorded, and marks the far side as the
// whole device.
func (p *pulseCapture) recordMonitorLocked(sink, mon uint32) error {
	if p.monitors[sink] != nil {
		return nil
	}
	t := p.monitorTracks[sink]
	if t == nil {
		var err error
		if t, err = p.tracks.Open(Them, fmt.Sprintf("far-sink-%d", sink)); err != nil {
			return err
		}
		p.monitorTracks[sink] = t
	}
	rec, err := p.openLocked(t, mon, "", proto.Undefined, "Agentry meeting notes (output)", false)
	if err != nil {
		return err
	}
	p.monitors[sink] = rec
	p.farSide = FarSideWholeDevice
	return nil
}

// recordRoom records the chosen microphone alone, as the Room track.
func (p *pulseCapture) recordRoom(chosen string) error {
	src, err := p.chooseMic(chosen)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.tracks.Open(Room, "room")
	if err != nil {
		return err
	}
	p.micTrack = t
	return p.openOwnLocked(src)
}

// micStreamName is how our microphone stream is listed.
const micStreamName = "Agentry meeting notes (microphone)"

// openOwnLocked opens the microphone a recording with no app reads, by
// source name. The stream is never moved by the server: a microphone that
// goes away kills it, and followMic decides what is read next.
func (p *pulseCapture) openOwnLocked(src string) error {
	rec, err := p.openLocked(p.micTrack, proto.Undefined, src, proto.Undefined, micStreamName, true)
	if err != nil {
		return err
	}
	p.own = rec
	return nil
}

// followMic keeps a recording that reads a microphone of its own on one:
// a stream the server killed, or a microphone no longer listed, is a loss;
// then the one micFollow names is read, and each change is kept for the
// recording to say.
func (p *pulseCapture) followMic(now time.Time) {
	var info proto.GetServerInfoReply
	if p.c.Request(&proto.GetServerInfo{}, &info) != nil {
		return
	}
	mics, err := listMics(p.c)
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.micTrack == nil {
		return
	}
	dead := false
	if p.own != nil {
		_, dead = p.killed.Load(p.own.stream)
	}
	gone, id, name := p.follow.look(mics, info.DefaultSourceName, now)
	if p.own != nil && (dead || gone) {
		p.dropLocked(p.own)
		p.own = nil
	}
	if id == "" || (id == p.follow.id && p.own != nil) {
		return
	}
	// The next is open before the one read is let go, so a microphone not
	// readable yet (one just plugged in) leaves the one read, and is tried
	// again at the next look.
	old := p.own
	if err := p.openOwnLocked(id); err != nil {
		p.own = old
		return
	}
	if old != nil {
		p.dropLocked(old)
	}
	p.follow.reading(id, name, now)
	p.micName = name
}

// MicChanges is each change of the microphone read, after the start.
func (p *pulseCapture) MicChanges() []MicChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.follow.list()
}

// chooseMic is the source a recording with no app reads: the chosen one
// when it is there, otherwise the default, which is then said.
func (p *pulseCapture) chooseMic(chosen string) (string, error) {
	var info proto.GetServerInfoReply
	if err := p.c.Request(&proto.GetServerInfo{}, &info); err != nil {
		return "", err
	}
	mics, err := listMics(p.c)
	if err != nil {
		return "", err
	}
	id, name, missing := pickMic(mics, chosen, info.DefaultSourceName)
	if id == "" {
		return "", errors.New("this computer has no microphone")
	}
	p.mu.Lock()
	p.micName, p.micMissing = name, missing
	// The microphone it starts on is kept, chosen or the default: a new
	// default never moves it, and it is taken up again if it goes away and
	// comes back.
	p.follow.pinned = id
	p.follow.started(id, name)
	p.mu.Unlock()
	return id, nil
}

// Microphones lists the sources that are not a monitor of an output.
func (r PulseRecorder) Microphones() ([]Microphone, error) {
	c, conn, err := proto.Connect(r.Server)
	if err != nil {
		return nil, fmt.Errorf("the audio system could not be reached: %w", err)
	}
	defer func() { _ = conn.Close() }()
	c.SetTimeout(5 * time.Second)
	// The server answers nothing else before the client has a name.
	props := proto.PropList{
		"application.name":       proto.PropListString("Agentry Desktop"),
		"application.process.id": proto.PropListString(strconv.Itoa(os.Getpid())),
	}
	if err := c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		return nil, fmt.Errorf("the audio system refused the connection: %w", err)
	}
	return listMics(c)
}

func listMics(c *proto.Client) ([]Microphone, error) {
	var sources proto.GetSourceInfoListReply
	if err := c.Request(&proto.GetSourceInfoList{}, &sources); err != nil {
		return nil, err
	}
	var out []Microphone
	for _, src := range sources {
		if src.MonitorSourceIndex != proto.Undefined {
			continue
		}
		name := src.Device
		if name == "" {
			name = src.SourceName
		}
		out = append(out, Microphone{ID: src.SourceName, Name: name})
	}
	return out, nil
}

// recordWholeDevice records the chosen microphone and the monitor of the
// default output.
func (p *pulseCapture) recordWholeDevice(chosen string) error {
	var info proto.GetServerInfoReply
	if err := p.c.Request(&proto.GetServerInfo{}, &info); err != nil {
		return err
	}
	if info.DefaultSinkName == "" {
		return errors.New("this computer has no default microphone or output")
	}
	src, err := p.chooseMic(chosen)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	mic, err := p.tracks.Open(Me, "mic")
	if err != nil {
		return err
	}
	p.micTrack = mic
	if err := p.openOwnLocked(src); err != nil {
		return err
	}
	far, err := p.tracks.Open(Them, "far-device")
	if err != nil {
		return err
	}
	rec, err := p.openLocked(far, proto.Undefined, info.DefaultSinkName+".monitor", proto.Undefined, "Agentry meeting notes (output)", false)
	if err != nil {
		return err
	}
	p.whole = append(p.whole, rec)
	return nil
}

// openLocked asks for a record stream: 16 kHz mono in 100 ms pieces, from
// a source by index or name, tied to one playback stream when direct is
// set; pin keeps it on its source, killed rather than moved when the
// source goes. It starts corked while the recording is paused.
func (p *pulseCapture) openLocked(t Track, source uint32, sourceName string, direct uint32, name string, pin bool) (*paRecord, error) {
	req := proto.CreateRecordStream{
		SampleSpec:         proto.SampleSpec{Format: proto.FormatInt16LE, Channels: 1, Rate: SampleRate},
		ChannelMap:         proto.ChannelMap{proto.ChannelMono},
		SourceIndex:        source,
		SourceName:         sourceName,
		BufferMaxLength:    2 * fragment,
		BufferFragSize:     fragment,
		AdjustLatency:      true,
		Corked:             p.paused,
		DirectOnInputIndex: direct,
		// A stream tied to a playback stream, or our own microphone, is
		// never moved onto another device: the recording decides.
		NoMove:         direct != proto.Undefined || pin,
		Properties:     proto.PropList{"media.name": proto.PropListString(name)},
		ChannelVolumes: proto.ChannelVolumes{0x10000},
	}
	var reply proto.CreateRecordStreamReply
	if err := p.c.Request(&req, &reply); err != nil {
		return nil, fmt.Errorf("the audio system would not record %s: %w", name, err)
	}
	rec := &paRecord{stream: reply.StreamIndex, output: reply.SourceOutputIndex, track: t, source: source}
	p.streams.Store(reply.StreamIndex, t)
	return rec, nil
}

// dropLocked deletes our record stream; its track stays open.
func (p *pulseCapture) dropLocked(rec *paRecord) {
	p.streams.Delete(rec.stream)
	p.killed.Delete(rec.stream)
	_ = p.c.Request(&proto.DeleteRecordStream{StreamIndex: rec.stream}, nil)
}

func (p *pulseCapture) allLocked() []*paRecord {
	out := append([]*paRecord(nil), p.whole...)
	if p.own != nil {
		out = append(out, p.own)
	}
	if p.mic != nil {
		out = append(out, p.mic)
	}
	for _, rec := range p.farByInput {
		out = append(out, rec)
	}
	for _, rec := range p.monitors {
		out = append(out, rec)
	}
	return out
}

// SetPaused corks or uncorks every record stream of ours: while corked the
// server sends nothing.
func (p *pulseCapture) SetPaused(paused bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return errors.New("the recording has stopped")
	}
	p.paused = paused
	var first error
	for _, rec := range p.allLocked() {
		if err := p.c.Request(&proto.CorkRecordStream{StreamIndex: rec.stream, Corked: paused}, nil); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Stop deletes every record stream, ends every track and closes the
// connection.
func (p *pulseCapture) Stop() error {
	p.stopAll()
	return nil
}

func (p *pulseCapture) stopAll() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	for _, rec := range p.allLocked() {
		p.dropLocked(rec)
		rec.track.End()
	}
	if p.micTrack != nil {
		p.micTrack.End()
	}
	for _, t := range p.monitorTracks {
		t.End()
	}
	for _, t := range p.waiting {
		t.End()
	}
	p.mic, p.own, p.whole, p.farByInput, p.monitors = nil, nil, nil, map[uint32]*paRecord{}, map[uint32]*paRecord{}
	p.mu.Unlock()
	close(p.done)
	_ = p.conn.Close()
}

func (p *pulseCapture) list() (paSnapshot, error) { return listStreams(p.c) }

// listStreams reads the streams, the sinks' monitors and which sources are
// monitors.
func listStreams(c *proto.Client) (paSnapshot, error) {
	s := paSnapshot{MonitorOf: map[uint32]uint32{}}
	var sinks proto.GetSinkInfoListReply
	if err := c.Request(&proto.GetSinkInfoList{}, &sinks); err != nil {
		return s, err
	}
	for _, k := range sinks {
		s.MonitorOf[k.SinkIndex] = k.MonitorSourceIndex
	}
	var sources proto.GetSourceInfoListReply
	if err := c.Request(&proto.GetSourceInfoList{}, &sources); err != nil {
		return s, err
	}
	monitor := map[uint32]bool{}
	for _, src := range sources {
		monitor[src.SourceIndex] = src.MonitorSourceIndex != proto.Undefined
	}
	var outs proto.GetSourceOutputInfoListReply
	if err := c.Request(&proto.GetSourceOutputInfoList{}, &outs); err != nil {
		return s, err
	}
	for _, o := range outs {
		s.Outputs = append(s.Outputs, paStream{
			Index: o.SourceOutpuIndex, Device: o.SourceIndex, Corked: o.Corked, OnMonitor: monitor[o.SourceIndex],
			Meter: isMeter(o.ResampleMethod, map[string]string{
				"stream.monitor": prop(o.Properties, "stream.monitor"),
				"resample.peaks": prop(o.Properties, "resample.peaks"),
			}),
			App: prop(o.Properties, "application.name"), Binary: prop(o.Properties, "application.process.binary"), PID: prop(o.Properties, "application.process.id"),
		})
	}
	var ins proto.GetSinkInputInfoListReply
	if err := c.Request(&proto.GetSinkInputInfoList{}, &ins); err != nil {
		return s, err
	}
	for _, in := range ins {
		s.Inputs = append(s.Inputs, paStream{
			Index: in.SinkInputIndex, Device: in.SinkIndex, Corked: in.Corked,
			App: prop(in.Properties, "application.name"), Binary: prop(in.Properties, "application.process.binary"), PID: prop(in.Properties, "application.process.id"),
		})
	}
	return s, nil
}

func prop(p proto.PropList, k string) string {
	v, ok := p[k]
	if !ok {
		return ""
	}
	return strings.TrimRight(string(v), "\x00")
}
