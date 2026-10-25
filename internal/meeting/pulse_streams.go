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
	// Meter is a source-output that only measures a level (isMeter).
	Meter bool
}

// paSnapshot is one listing of the streams.
type paSnapshot struct {
	Outputs []paStream // source-outputs
	Inputs  []paStream // sink-inputs
	// MonitorOf is each sink's monitor source.
	MonitorOf map[uint32]uint32
}

// findCall is the call under way: the first of findCalls.
func findCall(s paSnapshot, me string) (Call, bool) {
	calls := findCalls(s, me)
	if len(calls) == 0 {
		return Call{}, false
	}
	return calls[0], true
}

// appMic is the app's microphone stream, if it has one.
func appMic(s paSnapshot, pid string) (paStream, bool) {
	for _, o := range s.Outputs {
		if o.PID == pid && !o.OnMonitor && !o.Meter {
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
