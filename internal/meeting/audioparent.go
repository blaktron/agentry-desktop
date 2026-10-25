package meeting

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// The app's side of the audio child: a Recorder and a Detector that start
// it, send it a command and read what it reports.

// childProc is a running audio child.
type childProc struct {
	in  io.WriteCloser
	out io.Reader
	// stop closes the child's input and waits for it to end, ending it if
	// it does not.
	stop func()
}

// spawnFunc starts an audio child.
type spawnFunc func() (*childProc, error)

func (p *childProc) send(c wireCommand) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = p.in.Write(append(b, '\n'))
	return err
}

// childStartWait is how long a child may take to say it is recording.
const childStartWait = 20 * time.Second

// childRestarts is how many children in a row may fail soon after their
// start before a recording gives up, and childSettled how long one must run
// to count as having started well.
const (
	childRestarts = 5
	childSettled  = time.Minute
)

// childBackoff is the wait before the nth restart in a row: 1, 2, 4, 8, 16 s.
func childBackoff(n int) time.Duration {
	return time.Second << min(n-1, 4)
}

// childRecorder records through an audio child.
type childRecorder struct {
	spawn spawnFunc
}

// Record starts a child, asks it to record the call (or, with a zero Call,
// the call it finds under way or the whole default devices) and returns
// once it says it is recording.
func (r childRecorder) Record(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
	c := &childCapture{spawn: r.spawn, call: call, tracks: tracks, ended: make(chan struct{}), quit: make(chan struct{}), done: make(chan struct{})}
	p, out, err := c.startChild()
	if err != nil {
		return nil, err
	}
	ev, err := c.awaitStart(out)
	if err != nil {
		p.stop()
		return nil, err
	}
	if ev.Call != nil && call.PID == "" {
		c.call = *ev.Call
	}
	c.farSide = ev.FarSide
	c.proc = p
	go c.supervise(ctx, out)
	return c, nil
}

type childCapture struct {
	spawn  spawnFunc
	tracks Tracks

	mu       sync.Mutex
	call     Call
	farSide  FarSide
	paused   bool
	stopped  bool
	proc     *childProc
	mic, far Track

	ended   chan struct{}
	endOnce sync.Once
	// quit is closed by Stop, done by supervise as it returns.
	quit chan struct{}
	done chan struct{}
}

func (c *childCapture) startChild() (*childProc, *bufio.Reader, error) {
	p, err := c.spawn()
	if err != nil {
		return nil, nil, fmt.Errorf("the audio recorder could not be started: %w", err)
	}
	c.mu.Lock()
	cmd := wireCommand{Cmd: "record", PID: c.call.PID, Binary: c.call.Binary, Paused: c.paused}
	c.mu.Unlock()
	if err := p.send(cmd); err != nil {
		p.stop()
		return nil, nil, fmt.Errorf("the audio recorder could not be started: %w", err)
	}
	return p, bufio.NewReader(p.out), nil
}

// awaitStart reads until the child says it records, or why it cannot. Audio
// that comes first is kept.
func (c *childCapture) awaitStart(out *bufio.Reader) (wireEvent, error) {
	type result struct {
		ev  wireEvent
		err error
	}
	res := make(chan result, 1)
	go func() {
		for {
			kind, track, payload, err := readFrame(out)
			if err != nil {
				res <- result{err: fmt.Errorf("the audio recorder stopped before it started: %w", err)}
				return
			}
			if kind == frameAudio {
				c.write(track, payload)
				continue
			}
			var ev wireEvent
			if json.Unmarshal(payload, &ev) != nil {
				continue
			}
			switch ev.Type {
			case "started":
				res <- result{ev: ev}
				return
			case "error":
				res <- result{err: errors.New(ev.Message)}
				return
			case "log":
				log.Printf("meeting notes: %s", ev.Message)
			}
		}
	}()
	select {
	case r := <-res:
		return r.ev, r.err
	case <-time.After(childStartWait):
		return wireEvent{}, errors.New("the audio recorder did not start")
	}
}

// write puts samples on a track, opening the track at its first samples;
// nothing is written while paused.
func (c *childCapture) write(track byte, payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.paused {
		return
	}
	t := &c.mic
	label, name := Me, "mic"
	if track == wireFar {
		t, label, name = &c.far, Them, "far"
	}
	if *t == nil {
		opened, err := c.tracks.Open(label, name)
		if err != nil {
			log.Printf("meeting notes: a track could not be opened: %v", err)
			return
		}
		*t = opened
	}
	samples := make([]int16, len(payload)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(payload[2*i:])) //nolint:gosec // the sample's bits as they are
	}
	(*t).Write(samples)
}

// supervise reads the child until the recording stops. A child that ends
// early is started again after a wait; after childRestarts quick failures in
// a row the recording ends with what it has.
func (c *childCapture) supervise(ctx context.Context, out *bufio.Reader) {
	defer close(c.done)
	failures := 0
	for {
		began := time.Now()
		err := c.follow(out)
		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if stopped {
			return
		}
		if time.Since(began) >= childSettled {
			failures = 0
		}
		failures++
		if failures > childRestarts {
			log.Printf("meeting notes: the audio recorder kept stopping (%v); the recording ends", err)
			c.endOnce.Do(func() { close(c.ended) })
			return
		}
		log.Printf("meeting notes: the audio recorder stopped (%v); starting it again", err)
		select {
		case <-ctx.Done():
			return
		case <-c.quit:
			return
		case <-time.After(childBackoff(failures)):
		}
		c.mu.Lock()
		if c.stopped {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		p, o, err := c.startChild()
		if err != nil {
			log.Printf("meeting notes: %v", err)
			continue
		}
		c.mu.Lock()
		if c.stopped {
			c.mu.Unlock()
			p.stop()
			return
		}
		c.proc = p
		c.mu.Unlock()
		out = o
	}
}

// follow reads one child's frames until it ends.
func (c *childCapture) follow(out *bufio.Reader) error {
	for {
		kind, track, payload, err := readFrame(out)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = errors.New("it exited")
			}
			return err
		}
		if kind == frameAudio {
			c.write(track, payload)
			continue
		}
		var ev wireEvent
		if json.Unmarshal(payload, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "far-side":
			c.mu.Lock()
			c.farSide = ev.FarSide
			c.mu.Unlock()
		case "ended":
			c.endOnce.Do(func() { close(c.ended) })
		case "log":
			log.Printf("meeting notes: %s", ev.Message)
		case "error":
			return errors.New(ev.Message)
		}
	}
}

func (c *childCapture) Call() Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.call
}

func (c *childCapture) FarSide() FarSide {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.farSide
}

func (c *childCapture) Ended() <-chan struct{} { return c.ended }

// SetPaused tells the child to let go of the devices, or to take them up
// again; meanwhile nothing reaches the tracks.
func (c *childCapture) SetPaused(paused bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return errors.New("the recording has stopped")
	}
	c.paused = paused
	return c.proc.send(wireCommand{Cmd: "pause", Paused: paused})
}

// Stop ends the child and every track.
func (c *childCapture) Stop() error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	p := c.proc
	close(c.quit)
	c.mu.Unlock()
	p.stop()
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range []Track{c.mic, c.far} {
		if t != nil {
			t.End()
		}
	}
	return nil
}

// childDetector watches for calls through an audio child.
type childDetector struct {
	spawn spawnFunc
	// endWait is how long an app may stop using its microphone before its
	// call is over; 0 is callEndWait.
	endWait time.Duration
}

// Watch reports calls starting and ending until ctx ends, or until the
// child stops, which it returns.
func (d childDetector) Watch(ctx context.Context, events chan<- CallEvent) error {
	p, err := d.spawn()
	if err != nil {
		return fmt.Errorf("the audio watcher could not be started: %w", err)
	}
	defer p.stop()
	if err := p.send(wireCommand{Cmd: "detect"}); err != nil {
		return fmt.Errorf("the audio watcher could not be started: %w", err)
	}
	snaps := make(chan paSnapshot)
	errc := make(chan error, 1)
	go func() {
		out := bufio.NewReader(p.out)
		for {
			kind, _, payload, err := readFrame(out)
			if err != nil {
				errc <- err
				return
			}
			var ev wireEvent
			if kind != frameEvent || json.Unmarshal(payload, &ev) != nil {
				continue
			}
			switch ev.Type {
			case "sessions":
				select {
				case snaps <- sessionSnapshot(ev.Sessions):
				case <-ctx.Done():
					return
				}
			case "error":
				errc <- errors.New(ev.Message)
				return
			}
		}
	}()
	tracker := newCallTracker(strconv.Itoa(os.Getpid()))
	if d.endWait > 0 {
		tracker.endWait = d.endWait
	}
	var snap paSnapshot
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			return fmt.Errorf("the audio watcher stopped: %w", err)
		case snap = <-snaps:
		case <-tick.C:
			if !tracker.pending() {
				continue
			}
		}
		for _, ev := range tracker.update(snap, time.Now()) {
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
