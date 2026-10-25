package meeting

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jfreymuth/pulse/proto"
)

// NewDetector is this platform's detector: the PulseAudio protocol's
// subscription to the streams, which PipeWire's pipewire-pulse also serves.
func NewDetector() Detector { return PulseDetector{} }

// PulseDetector watches the streams over the PulseAudio protocol. It only
// listens: it opens no stream, so it records nothing and changes nothing
// another app hears.
type PulseDetector struct {
	// Server is the server to connect to; "" for the session's own.
	Server string
}

// Watch reports calls starting and ending until ctx ends or the connection
// fails, which it returns.
func (d PulseDetector) Watch(ctx context.Context, events chan<- CallEvent) error {
	c, conn, err := proto.Connect(d.Server)
	if err != nil {
		return fmt.Errorf("the audio system could not be reached: %w", err)
	}
	defer func() { _ = conn.Close() }()
	c.SetTimeout(5 * time.Second)
	changed := make(chan struct{}, 1)
	closed := make(chan struct{})
	c.Callback = func(msg any) {
		switch msg.(type) {
		case *proto.SubscribeEvent:
			select {
			case changed <- struct{}{}:
			default:
			}
		case *proto.ConnectionClosed:
			select {
			case <-closed:
			default:
				close(closed)
			}
		}
	}
	me := strconv.Itoa(os.Getpid())
	props := proto.PropList{
		"application.name":       proto.PropListString("Agentry Desktop"),
		"application.process.id": proto.PropListString(me),
	}
	if err := c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		return fmt.Errorf("the audio system refused the connection: %w", err)
	}
	// The library calls source-outputs "source inputs".
	if err := c.Request(&proto.Subscribe{Mask: proto.SubscriptionMaskSinkInput | proto.SubscriptionMaskSourceInput}, nil); err != nil {
		return err
	}
	tracker := newCallTracker(me)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		snap, err := listStreams(c)
		if err != nil {
			return err
		}
		for _, ev := range tracker.update(snap, time.Now()) {
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
		// A call that has gone is read again each second until its wait is
		// over; otherwise the listing is read when the server says a stream
		// changed.
		var wake <-chan time.Time
		if tracker.pending() {
			wake = tick.C
		}
		select {
		case <-ctx.Done():
			return nil
		case <-closed:
			return fmt.Errorf("the audio system closed the connection")
		case <-changed:
		case <-wake:
		}
	}
}
