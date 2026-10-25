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
// default microphone and the monitor of the default output, the whole
// device, and says so.
type PulseRecorder struct {
	// Server is the server to connect to; "" for the session's own.
	Server string
}

// fragment is the size of the pieces the server sends: 100 ms. The
// server's default holds the first samples back for about two seconds,
// which would put the two sides out of step.
const fragment = SampleRate / 10 * 2

// endWait is how long a recording of an app goes on after the app has
// stopped both reading the microphone and playing, in case it comes back
// (a call rejoined, a device switched).
const endWait = 30 * time.Second

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
		if err := p.recordWholeDevice(); err != nil {
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
	whole      []*paRecord
	quietSince time.Time

	changed chan struct{}
	done    chan struct{}
	ended   chan struct{}
	endOnce sync.Once
}

func (p *pulseCapture) Call() Call       { return p.call }
func (p *pulseCapture) FarSide() FarSide { return p.farSide }

func (p *pulseCapture) Ended() <-chan struct{} { return p.ended }

func (p *pulseCapture) callback(msg any) {
	switch m := msg.(type) {
	case *proto.DataPacket:
		if t, ok := p.streams.Load(m.StreamIndex); ok {
			t.(Track).Write(int16s(m.Data))
		}
	case *proto.SubscribeEvent, *proto.RecordStreamKilled:
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
		if p.farSide != FarSideApp {
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
// app stops reading; each playback stream that is flowing taken up once,
// and one that is gone ended for good.
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
		rec, err := p.openLocked(p.micTrack, o.Device, "", proto.Undefined, "Agentry meeting notes (microphone)")
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
		if p.farByInput[in.Index] != nil || p.lostInputs[in.Index] {
			continue
		}
		mon, ok := s.MonitorOf[in.Device]
		if !ok {
			continue
		}
		t, err := p.tracks.Open(Them, fmt.Sprintf("far-%d", in.Index))
		if err != nil {
			return err
		}
		rec, err := p.openLocked(t, mon, "", in.Index, "Agentry meeting notes ("+in.App+")")
		if err != nil {
			// A stream that went between the listing and the request.
			t.End()
			p.lostInputs[in.Index] = true
			continue
		}
		p.farByInput[in.Index] = rec
	}
	if p.mic == nil && len(p.farByInput) == 0 {
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

// recordWholeDevice records the default microphone and the monitor of the
// default output.
func (p *pulseCapture) recordWholeDevice() error {
	var info proto.GetServerInfoReply
	if err := p.c.Request(&proto.GetServerInfo{}, &info); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if info.DefaultSourceName == "" || info.DefaultSinkName == "" {
		return errors.New("this computer has no default microphone or output")
	}
	mic, err := p.tracks.Open(Me, "mic")
	if err != nil {
		return err
	}
	p.micTrack = mic
	rec, err := p.openLocked(mic, proto.Undefined, info.DefaultSourceName, proto.Undefined, "Agentry meeting notes (microphone)")
	if err != nil {
		return err
	}
	p.whole = append(p.whole, rec)
	far, err := p.tracks.Open(Them, "far-device")
	if err != nil {
		return err
	}
	rec, err = p.openLocked(far, proto.Undefined, info.DefaultSinkName+".monitor", proto.Undefined, "Agentry meeting notes (output)")
	if err != nil {
		return err
	}
	p.whole = append(p.whole, rec)
	return nil
}

// openLocked asks for a record stream: 16 kHz mono in 100 ms pieces, from
// a source by index or name, tied to one playback stream when direct is
// set. It starts corked while the recording is paused.
func (p *pulseCapture) openLocked(t Track, source uint32, sourceName string, direct uint32, name string) (*paRecord, error) {
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
		NoMove:             direct != proto.Undefined,
		Properties:         proto.PropList{"media.name": proto.PropListString(name)},
		ChannelVolumes:     proto.ChannelVolumes{0x10000},
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
	_ = p.c.Request(&proto.DeleteRecordStream{StreamIndex: rec.stream}, nil)
}

func (p *pulseCapture) allLocked() []*paRecord {
	out := append([]*paRecord(nil), p.whole...)
	if p.mic != nil {
		out = append(out, p.mic)
	}
	for _, rec := range p.farByInput {
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
	p.mic, p.whole, p.farByInput = nil, nil, map[uint32]*paRecord{}
	p.mu.Unlock()
	close(p.done)
	_ = p.conn.Close()
}

// list reads the streams, the sinks' monitors and which sources are
// monitors.
func (p *pulseCapture) list() (paSnapshot, error) {
	s := paSnapshot{MonitorOf: map[uint32]uint32{}}
	var sinks proto.GetSinkInfoListReply
	if err := p.c.Request(&proto.GetSinkInfoList{}, &sinks); err != nil {
		return s, err
	}
	for _, k := range sinks {
		s.MonitorOf[k.SinkIndex] = k.MonitorSourceIndex
	}
	var sources proto.GetSourceInfoListReply
	if err := p.c.Request(&proto.GetSourceInfoList{}, &sources); err != nil {
		return s, err
	}
	monitor := map[uint32]bool{}
	for _, src := range sources {
		monitor[src.SourceIndex] = src.MonitorSourceIndex != proto.Undefined
	}
	var outs proto.GetSourceOutputInfoListReply
	if err := p.c.Request(&proto.GetSourceOutputInfoList{}, &outs); err != nil {
		return s, err
	}
	for _, o := range outs {
		s.Outputs = append(s.Outputs, paStream{
			Index: o.SourceOutpuIndex, Device: o.SourceIndex, Corked: o.Corked, OnMonitor: monitor[o.SourceIndex],
			App: prop(o.Properties, "application.name"), Binary: prop(o.Properties, "application.process.binary"), PID: prop(o.Properties, "application.process.id"),
		})
	}
	var ins proto.GetSinkInputInfoListReply
	if err := p.c.Request(&proto.GetSinkInputInfoList{}, &ins); err != nil {
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
