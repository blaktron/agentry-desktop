package meeting

// The audio stack's streams as the PulseAudio protocol lists them, and the
// choices made from them, apart from the connection so they are tested on
// any computer.

// paStream is a recording stream (a source-output: an app reading a
// microphone) or a playback stream (a sink-input: an app playing).
type paStream struct {
	Index uint32
	// Device is the source a source-output reads, or the sink a
	// sink-input plays to.
	Device uint32
	App    string
	Binary string
	PID    string
	Corked bool
	// OnMonitor is a source-output reading a sink's monitor rather than a
	// microphone.
	OnMonitor bool
}

// paSnapshot is one listing of the streams.
type paSnapshot struct {
	Outputs []paStream // source-outputs
	Inputs  []paStream // sink-inputs
	// MonitorOf is each sink's monitor source.
	MonitorOf map[uint32]uint32
}

// findCall is the call under way: an app, not this one, that reads a
// microphone (an uncorked source-output on a source that is not a monitor)
// and also plays something. The first such app in the listing wins, named
// as its playback is: an app often gives its microphone stream a name of
// its own ("Google Chrome input").
func findCall(s paSnapshot, me string) (Call, bool) {
	for _, o := range s.Outputs {
		if o.Corked || o.OnMonitor || o.PID == "" || o.PID == me {
			continue
		}
		for _, in := range s.Inputs {
			if in.PID == o.PID {
				name := in.App
				if name == "" {
					name = o.App
				}
				return Call{App: name, Binary: o.Binary, PID: o.PID}, true
			}
		}
	}
	return Call{}, false
}

// appMic is the app's microphone stream, if it has one.
func appMic(s paSnapshot, pid string) (paStream, bool) {
	for _, o := range s.Outputs {
		if o.PID == pid && !o.OnMonitor {
			return o, true
		}
	}
	return paStream{}, false
}

// appPlayback are the app's playback streams that are flowing. A corked
// stream is left until it uncorks: asking to record one times out.
func appPlayback(s paSnapshot, pid string) []paStream {
	var out []paStream
	for _, in := range s.Inputs {
		if in.PID == pid && !in.Corked {
			out = append(out, in)
		}
	}
	return out
}

// hasInput says whether the sink-input is still listed.
func hasInput(s paSnapshot, index uint32) bool {
	for _, in := range s.Inputs {
		if in.Index == index {
			return true
		}
	}
	return false
}

// int16s reads signed 16-bit little-endian samples.
func int16s(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(uint16(b[2*i]) | uint16(b[2*i+1])<<8)
	}
	return out
}
