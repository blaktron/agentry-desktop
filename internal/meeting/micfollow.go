package meeting

import "time"

// MicChange is a change, after the start, of the microphone a recording
// reads where it reads one of its own (a meeting in the room, or a
// recording that follows no app): the one recorded went away (Lost names
// it), or another is recorded. Now names the microphone recorded from At
// on, "" while none is.
type MicChange struct {
	At   time.Time
	Lost string
	Now  string
}

// MicFollower is a Capture that follows the microphone it reads going away
// and coming back. MicChanges is every change so far, oldest first.
type MicFollower interface {
	MicChanges() []MicChange
}

// micFollow decides, for a recording that reads a microphone of its own,
// which one it reads as microphones come and go, and keeps what changed.
//
// The rule: the pinned microphone while it is connected; otherwise the
// default, when the default is a microphone; otherwise none until one is
// connected. So a microphone unplugged mid-recording gives way to the
// default at once, and is taken up again when it is plugged back in; each
// change is kept, so it is said and the transcript can mark the time with
// no microphone. A recording that follows the default pins nothing and
// moves with it.
type micFollow struct {
	// pinned is the microphone the recording keeps, by id, "" to follow
	// the default.
	pinned string
	// id and name are the microphone read now, "" while none is.
	id, name string
	changes  []MicChange
}

// look is the step every recorder takes at each look, given the
// microphones listed now and the default's id: gone says the one read is
// no longer listed, a loss now kept, so its stream is let go; id and name
// are the microphone to read next, "" for none, or the one read when
// neither the pinned one nor the default is listed while it still is.
func (f *micFollow) look(mics []Microphone, def string, now time.Time) (gone bool, id, name string) {
	if f.gone(mics) {
		f.lost(now)
		gone = true
	}
	id, name = f.target(mics, def)
	if id == "" && f.id != "" {
		return gone, f.id, f.name
	}
	return gone, id, name
}

// target is the microphone to read, given the microphones listed now and
// the default's id.
func (f *micFollow) target(mics []Microphone, def string) (id, name string) {
	if f.pinned != "" {
		if m, ok := findMic(mics, f.pinned); ok {
			return m.ID, m.Name
		}
	}
	if m, ok := findMic(mics, def); ok {
		return m.ID, m.Name
	}
	return "", ""
}

// gone says whether the microphone read now is no longer listed.
func (f *micFollow) gone(mics []Microphone) bool {
	if f.id == "" {
		return false
	}
	_, ok := findMic(mics, f.id)
	return !ok
}

// lost notes that the microphone read went away at now; none is read
// until reading says which is.
func (f *micFollow) lost(now time.Time) {
	if f.id == "" {
		return
	}
	f.changes = append(f.changes, MicChange{At: now, Lost: f.name})
	f.id, f.name = "", ""
}

// reading notes the microphone read from now on, "" for none. A change
// made in the same moment as a loss is kept as one change.
func (f *micFollow) reading(id, name string, now time.Time) {
	if id == f.id {
		return
	}
	f.id, f.name = id, name
	if n := len(f.changes); n > 0 && f.changes[n-1].At.Equal(now) && f.changes[n-1].Lost != "" && f.changes[n-1].Now == "" {
		f.changes[n-1].Now = name
		return
	}
	f.changes = append(f.changes, MicChange{At: now, Now: name})
}

// started notes the microphone the recording starts on: not a change.
func (f *micFollow) started(id, name string) { f.id, f.name = id, name }

// list is every change so far, oldest first.
func (f *micFollow) list() []MicChange { return append([]MicChange(nil), f.changes...) }

func findMic(mics []Microphone, id string) (Microphone, bool) {
	if id == "" {
		return Microphone{}, false
	}
	for _, m := range mics {
		if m.ID == id {
			return m, true
		}
	}
	return Microphone{}, false
}
