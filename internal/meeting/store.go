package meeting

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/dirpublish"
)

// Where a meeting stands.
const (
	StateRecording    = "recording"
	StatePending      = "pending" // recorded, waiting to be transcribed
	StateTranscribing = "transcribing"
	StateDone         = "done"
	StateFailed       = "failed"
)

// KeepFor is how long recordings are kept when the person keeps them, and
// how long the audio of a failed transcription waits for another try.
const KeepFor = 7 * 24 * time.Hour

// TrackRecord is one track of a meeting.
type TrackRecord struct {
	File   string `json:"file"`
	Label  Label  `json:"label"`
	Silent bool   `json:"silent,omitempty"`
}

// Meeting is one recording, kept as meeting.json in its own folder beside
// its tracks.
type Meeting struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	App        string        `json:"app,omitempty"`
	FarSide    FarSide       `json:"farSide"`
	Started    time.Time     `json:"started"`
	Ended      time.Time     `json:"ended,omitempty"`
	State      string        `json:"state"`
	Error      string        `json:"error,omitempty"`
	Transcript string        `json:"transcript,omitempty"`
	Profile    string        `json:"profile,omitempty"`
	Model      string        `json:"model,omitempty"`
	Tracks     []TrackRecord `json:"tracks,omitempty"`
	Gaps       []Gap         `json:"gaps,omitempty"`
	// AudioUntil is when the kept audio is deleted; zero once it is gone.
	AudioUntil time.Time `json:"audioUntil,omitempty"`
	// Window is the armed window it was recorded in, and Invitees that
	// window's attendees; "" and none for any other recording.
	Window   string   `json:"window,omitempty"`
	Invitees []string `json:"invitees,omitempty"`
	// Shared is the transcript's copy in the Assistant's share folder, ""
	// until it is copied there.
	Shared string `json:"shared,omitempty"`
	// Delivery is where handing the transcript to the Assistant stands:
	// "" for not handed to it, or one of the Delivery constants.
	Delivery      string `json:"delivery,omitempty"`
	DeliveryError string `json:"deliveryError,omitempty"`
}

// Where handing a transcript to the Assistant stands.
const (
	// DeliveryWaiting: the transcript is in the share, and the person asks
	// the Assistant to summarise it (it was stopped, or they chose to be
	// asked first).
	DeliveryWaiting = "waiting"
	// DeliverySending: the turn is being sent, or waits for a turn in
	// flight to end.
	DeliverySending = "sending"
	// DeliverySent: the Assistant was asked to summarise it.
	DeliverySent = "sent"
	// DeliveryFailed: it could not be copied or sent; DeliveryError says
	// why.
	DeliveryFailed = "failed"
)

// Store keeps the meetings' folders under Dir.
type Store struct {
	Dir string
}

// recordMu keeps a meeting's record from being read while it is replaced:
// Windows refuses to rename over a file this process has open, and a
// replacement that moves the old file aside first (dirpublish.ReplaceFile,
// when a scanner holds it) leaves an instant with no file.
var recordMu sync.RWMutex

func (s Store) folder(id string) string { return filepath.Join(s.Dir, id) }

// Create makes a meeting's folder and record.
func (s Store) Create(m *Meeting) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(s.folder(m.ID), 0o700); err != nil {
		return err
	}
	return s.Save(m)
}

// Save writes the meeting's record, replacing it whole.
func (s Store) Save(m *Meeting) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	return dirpublish.ReplaceFile(filepath.Join(s.folder(m.ID), "meeting.json"), raw, 0o600)
}

// Get reads one meeting.
func (s Store) Get(id string) (*Meeting, error) {
	if !validID(id) {
		return nil, fmt.Errorf("no meeting %q", id)
	}
	recordMu.RLock()
	raw, err := os.ReadFile(filepath.Join(s.folder(id), "meeting.json")) //nolint:gosec // a meeting folder of ours, by a checked id
	recordMu.RUnlock()
	if err != nil {
		return nil, err
	}
	var m Meeting
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("meeting %s: %w", id, err)
	}
	return &m, nil
}

// List reads every meeting, newest first. A folder whose record cannot be
// read is skipped.
func (s Store) List() ([]*Meeting, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Meeting
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if m, err := s.Get(e.Name()); err == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out, nil
}

// TrackPath is a track's file.
func (s Store) TrackPath(m *Meeting, t TrackRecord) string {
	return filepath.Join(s.folder(m.ID), filepath.Base(t.File))
}

// HasAudio says whether any of the meeting's tracks is still on disk.
func (s Store) HasAudio(m *Meeting) bool {
	for _, t := range m.Tracks {
		if _, err := os.Stat(s.TrackPath(m, t)); err == nil {
			return true
		}
	}
	return false
}

// DeleteAudio removes the meeting's tracks and says so in its record.
func (s Store) DeleteAudio(m *Meeting) error {
	var first error
	for _, t := range m.Tracks {
		if err := os.Remove(s.TrackPath(m, t)); err != nil && !errors.Is(err, fs.ErrNotExist) && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	m.AudioUntil = time.Time{}
	return s.Save(m)
}

// Delete removes the meeting's folder: its record and any audio. The
// transcript in the output folder is the person's and is left alone.
func (s Store) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("no meeting %q", id)
	}
	return os.RemoveAll(s.folder(id))
}

// Sweep deletes the audio that has outlived its keep: audio of a finished
// meeting when recordings are not kept, and any audio past its AudioUntil.
// A meeting still recording or being transcribed is left alone.
func (s Store) Sweep(now time.Time, keep bool) error {
	ms, err := s.List()
	if err != nil {
		return err
	}
	var first error
	for _, m := range ms {
		if m.State == StateRecording || m.State == StateTranscribing || m.State == StatePending {
			continue
		}
		if !s.HasAudio(m) {
			continue
		}
		expired := !m.AudioUntil.IsZero() && now.After(m.AudioUntil)
		if (m.State == StateDone && !keep) || expired || m.AudioUntil.IsZero() {
			if err := s.DeleteAudio(m); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// NewID is a meeting's id: its start, so folders sort, and a counter for
// two in one second.
func (s Store) NewID(started time.Time) string {
	base := started.UTC().Format("20060102T150405Z")
	id := base
	for i := 2; ; i++ {
		if _, err := os.Stat(s.folder(id)); errors.Is(err, fs.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`) && filepath.Base(id) == id
}
