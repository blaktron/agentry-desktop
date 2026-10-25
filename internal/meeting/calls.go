package meeting

import "time"

// The calls under way, as the audio stack's streams show them, worked out
// apart from any connection so they are tested on every computer.

// callEndWait is how long a call's app may stop reading the microphone
// before the call is over: a call dropped and rejoined within it, or a
// device switched, stays one call.
const callEndWait = 30 * time.Second

// AppKey is how a meeting app is known from one call to the next, for the
// automatic list and for not offering a call twice: its executable's name,
// or the name it gives the audio stack when it reports no executable.
func (c Call) AppKey() string {
	if c.Binary != "" {
		return c.Binary
	}
	return c.App
}

// isMeter is a stream that only measures a level: a volume meter's
// (pavucontrol's, a desktop's sound applet), not an app that listens.
// PulseAudio lists it with the "peaks" resampler, PipeWire with
// stream.monitor or resample.peaks set.
func isMeter(resampler string, props map[string]string) bool {
	return resampler == "peaks" || props["stream.monitor"] == "true" || props["resample.peaks"] == "true"
}

// findCalls are the calls under way, one per app: an app, not this one,
// that reads a microphone (an uncorked source-output on a source that is
// not a monitor, and not a meter) and also plays something from the same
// process, or from another process of the same executable. Each is named as
// its playback is: an app often gives its microphone stream a name of its
// own ("Google Chrome input"). The listing's order decides between two
// processes of one app.
func findCalls(s paSnapshot, me string) []Call {
	var out []Call
	seen := map[string]bool{}
	for _, o := range s.Outputs {
		if o.Corked || o.OnMonitor || o.Meter || o.PID == "" || o.PID == me {
			continue
		}
		for _, in := range s.Inputs {
			if in.PID == me || (in.PID != o.PID && (o.Binary == "" || in.Binary != o.Binary)) {
				continue
			}
			name := in.App
			if name == "" {
				name = o.App
			}
			c := Call{App: name, Binary: o.Binary, PID: o.PID}
			if !seen[c.AppKey()] {
				seen[c.AppKey()] = true
				out = append(out, c)
			}
			break
		}
	}
	return out
}

// callTracker turns listings of the streams into calls starting and
// ending: a call starts the first time its app is seen in one, and ends
// once the app has been out of the listings for callEndWait.
type callTracker struct {
	me      string
	endWait time.Duration
	calls   map[string]*trackedCall
}

type trackedCall struct {
	call   Call
	goneAt time.Time // zero while it is under way
}

func newCallTracker(me string) *callTracker {
	return &callTracker{me: me, endWait: callEndWait, calls: map[string]*trackedCall{}}
}

// update reads one listing, taken at now, and says what changed.
func (t *callTracker) update(s paSnapshot, now time.Time) []CallEvent {
	var events []CallEvent
	live := map[string]bool{}
	for _, c := range findCalls(s, t.me) {
		k := c.AppKey()
		live[k] = true
		if tc, ok := t.calls[k]; ok {
			tc.goneAt = time.Time{}
			tc.call = c
			continue
		}
		t.calls[k] = &trackedCall{call: c}
		events = append(events, CallEvent{Call: c, At: now})
	}
	for k, tc := range t.calls {
		if live[k] {
			continue
		}
		if tc.goneAt.IsZero() {
			tc.goneAt = now
			continue
		}
		if now.Sub(tc.goneAt) >= t.endWait {
			delete(t.calls, k)
			events = append(events, CallEvent{Call: tc.call, Ended: true, At: now})
		}
	}
	return events
}

// pending says whether a call has gone and is waiting out callEndWait, so
// the listing must be read again even if nothing on the server changes.
func (t *callTracker) pending() bool {
	for _, tc := range t.calls {
		if !tc.goneAt.IsZero() {
			return true
		}
	}
	return false
}
