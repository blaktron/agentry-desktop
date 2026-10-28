package meeting

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Settings are the person's choices, read afresh each time they matter.
type Settings struct {
	// On is the feature's switch. While it is off nothing records and no
	// device is opened.
	On bool
	// Profile is the language profile's id.
	Profile string
	// Model is the model the person chose, "" for automatic.
	Model string
	// Hardware is what the person chose to transcribe on, where a
	// graphics card is offered: HardwareGPU or HardwareCPU, or "" until
	// they answer the offer, which is the CPU.
	Hardware Hardware
	// OutputDir is the folder transcripts go to.
	OutputDir string
	// KeepAudio keeps recordings for KeepFor after their transcript.
	KeepAudio bool
	// Automatic are the meeting apps, by AppKey, whose calls are recorded
	// as soon as they are detected; every other app's call is offered.
	Automatic []string
	// MaxLength ends a recording that has run this long; 0 is
	// DefaultMaxLength.
	MaxLength time.Duration
	// Mic is the microphone chosen, by its Microphone ID; "" for the
	// system's default.
	Mic string
	// SpeakersOff turns off telling the speakers of a meeting in the room
	// apart, which is on by default: its lines then stay Room.
	SpeakersOff bool
}

// Hardware is what transcriptions run on, as the person chose it.
type Hardware string

// The hardware a person can choose between where a graphics card is
// offered.
const (
	HardwareGPU Hardware = "gpu"
	HardwareCPU Hardware = "cpu"
)

// DefaultMaxLength is how long a recording runs at most unless the person
// chose otherwise.
const DefaultMaxLength = 4 * time.Hour

// DefaultQuietAfter is how long a meeting in the room goes without speech
// before the person is asked whether it is still on.
const DefaultQuietAfter = 15 * time.Minute

// Notice is a desktop notification the feature raises.
type Notice struct {
	// Tag names a notification that a later one with the same tag
	// replaces, and Withdraw takes away; "" for one that stands alone.
	Tag   string
	Title string
	Body  string
	// Actions are its buttons, where the platform draws them; a press
	// comes back to HandleAction with the action's ID.
	Actions []NoticeAction
	// Resident keeps it on screen until it is replaced or withdrawn.
	Resident bool
}

// NoticeAction is one button on a notification.
type NoticeAction struct {
	ID    string
	Label string
}

// The tags of the feature's notifications.
const (
	tagRecording = "meeting-recording"
	tagOffer     = "meeting-offer"
	tagQuiet     = "meeting-quiet"
	tagMic       = "meeting-mic"
)

// How says how a recording started.
type How string

const (
	// ByHand is the person's own Record.
	ByHand How = "hand"
	// FromOffer is Record pressed on the offer of a detected call.
	FromOffer How = "offer"
	// Automatically is a detected call of an app on the automatic list.
	Automatically How = "automatic"
	// InWindow is a detected call inside a window the Assistant armed.
	InWindow How = "armed"
	// InRoom is the person's own Record in the room: the microphone alone.
	InRoom How = "room"
)

// Manager runs Meeting notes: one recording at a time, then its
// transcription, in the background, one meeting after another.
type Manager struct {
	// Root holds the models, the meetings' folders and the calibration.
	Root      string
	Catalogue Catalogue
	Recorder  Recorder
	Engine    Engine
	Models    *Models
	// Diarizer tells the speakers of a meeting in the room apart; nil where
	// this copy carries none, and then the room's lines stay Room.
	Diarizer Diarizer
	// DiarizeTimeout bounds one diarization, beyond the audio's own length;
	// 0 is DefaultDiarizeTimeout.
	DiarizeTimeout time.Duration
	// Detector reports calls; nil where calls are not detected, and then
	// nothing is offered or started but by hand.
	Detector Detector
	Settings func() Settings
	// Tier is this computer's hardware tier.
	Tier string
	// MeasuresGPU: on the CPU tier, the transcriber's graphics devices are
	// measured, to offer the graphics tier's model where one keeps up.
	MeasuresGPU bool
	// EngineID names the transcriber as installed, so a newer one's
	// graphics devices are measured again; nil names none.
	EngineID func() string
	// EngineReady says whether the transcriber can run; nil means it can.
	EngineReady func() error
	// Notify raises a desktop notification, and Withdraw takes away the
	// one with a tag. An error says it was not raised.
	Notify   func(Notice) error
	Withdraw func(tag string)
	// Indicate is told the recording in progress each time it starts,
	// pauses, resumes or ends (nil): the tray shows it.
	Indicate func(*RecordingStatus)
	// OfferFor is how long an offer to record stands; 0 is two minutes.
	OfferFor time.Duration
	// QuietAfter is how long a meeting in the room goes without speech
	// before the person is asked whether it is still on; 0 is
	// DefaultQuietAfter.
	QuietAfter time.Duration
	// Changed is told whenever the status changes.
	Changed func()
	// Transcribed is told each meeting whose transcript has just been
	// written, in the transcription worker: it hands the transcript on.
	Transcribed func(id string)
	Now         func() time.Time

	mu sync.Mutex
	// noticeMu orders the recording's notification: raised, replaced and
	// taken away one at a time.
	noticeMu sync.Mutex
	rec      *recording
	prep     PrepareStatus
	jobs     map[string]int
	queue    chan string
	ctx      context.Context
	cancel   context.CancelFunc
	started  bool
	// live are the calls under way, by AppKey; offer is the one offer
	// standing; refresh asks the watcher to read the switch again.
	live    map[string]Call
	order   []string
	handled map[string]bool
	offer   *Offer
	refresh chan struct{}
	// closing is set as the app quits: nothing new starts.
	closing bool
	// wg is the background work Shutdown waits for.
	wg sync.WaitGroup
}

// PrepareStatus is the model's download and calibration.
type PrepareStatus struct {
	Busy  bool   `json:"busy"`
	Step  string `json:"step,omitempty"`
	Done  int64  `json:"done,omitempty"`
	Total int64  `json:"total,omitempty"`
	Error string `json:"error,omitempty"`
}

// RecordingStatus is the recording in progress.
type RecordingStatus struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	App     string    `json:"app,omitempty"`
	FarSide FarSide   `json:"farSide"`
	Paused  bool      `json:"paused"`
	How     How       `json:"how"`
	// Title is the calendar's, for a recording made in an armed window.
	Title string `json:"title,omitempty"`
	// Warning is what the recorder found the person should put right, if
	// anything.
	Warning *Warning `json:"warning,omitempty"`
	// Microphone is the microphone recorded, where the recorder says, and
	// MicMissing that the one chosen was missing so the default is.
	Microphone string `json:"microphone,omitempty"`
	MicMissing bool   `json:"micMissing,omitempty"`
	// MicLost is the microphone that went away during the recording and
	// is not read again, "" for none; MicNone says no microphone is read
	// now because of it.
	MicLost string `json:"micLost,omitempty"`
	MicNone bool   `json:"micNone,omitempty"`
}

// Status is the feature's state as a page shows it.
type Status struct {
	On        bool             `json:"on"`
	Recording *RecordingStatus `json:"recording,omitempty"`
	Prepare   PrepareStatus    `json:"prepare"`
	// Model is the model a transcription uses now, and ModelReady whether
	// it and the voice detection are downloaded.
	Model      string `json:"model"`
	ModelName  string `json:"modelName"`
	ModelReady bool   `json:"modelReady"`
	// EngineError says why the transcriber cannot run, "" when it can.
	EngineError string `json:"engineError,omitempty"`
	// GPU is the graphics device measured fastest here, nil when none ran
	// a transcription; GPUChecked says the devices were measured; OnGPU
	// says transcriptions use it now, and GPUOffer that its model is
	// offered and the offer not yet answered.
	GPU        *GPUDevice `json:"gpu,omitempty"`
	GPUChecked bool       `json:"gpuChecked"`
	OnGPU      bool       `json:"onGPU"`
	GPUOffer   bool       `json:"gpuOffer"`
	// Transcribing is each meeting being transcribed, by id, with its
	// progress from 0 to 100.
	Transcribing map[string]int `json:"transcribing"`
	// Offer is the detected call offered for recording, if one stands.
	Offer *Offer `json:"offer,omitempty"`
}

// recording is the one recording in progress.
type recording struct {
	m        *Meeting
	capture  Capture
	tracks   []*wavTrack
	names    map[string]bool
	pausedAt time.Duration
	paused   bool
	stopping bool
	how      How
	// window is the armed window it was started in, "" for none.
	window string
	// warning is the capture's warning, once it raised one.
	warning *Warning
	// quietFrom is when the room's silence is counted from: the start, or
	// the last resume. quietAsked is set once the person was asked whether
	// the meeting is still on, until someone speaks again; askedAt is when.
	quietFrom  time.Time
	quietAsked bool
	askedAt    time.Time
	// micSeen is how many of the capture's microphone changes were said.
	micSeen int
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) store() Store { return Store{Dir: filepath.Join(m.Root, "meetings")} }

func (m *Manager) changed() {
	if m.Changed != nil {
		m.Changed()
	}
}

// Start recovers what an earlier session left (a recording cut short by a
// crash is kept and transcribed; a transcription cut short is done again),
// deletes audio that has outlived its keep, and starts the transcription
// worker and the hourly sweep. It is called once.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.queue = make(chan string, 64)
	m.jobs = map[string]int{}
	m.live, m.handled = map[string]Call{}, map[string]bool{}
	m.refresh = make(chan struct{}, 1)
	ctx, m.cancel = context.WithCancel(ctx)
	m.ctx = ctx
	m.mu.Unlock()

	st := m.store()
	ms, _ := st.List()
	var pending []string
	for _, mt := range ms {
		switch mt.State {
		case StateRecording:
			for _, t := range mt.Tracks {
				_ = repairWAV(st.TrackPath(mt, t))
			}
			if mt.Ended.IsZero() {
				mt.Ended = mt.Started
			}
			mt.State = StatePending
			_ = st.Save(mt)
			pending = append(pending, mt.ID)
		case StateTranscribing, StatePending:
			mt.State = StatePending
			_ = st.Save(mt)
			pending = append(pending, mt.ID)
		}
	}
	_ = st.Sweep(m.now(), m.Settings().KeepAudio)
	m.rearm()
	m.wg.Add(3)
	go m.work(ctx)
	go m.watch(ctx)
	if m.Settings().On {
		for _, id := range pending {
			m.enqueue(id)
		}
	}
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = m.store().Sweep(m.now(), m.Settings().KeepAudio)
				m.changed()
			}
		}
	}()
}

// Shutdown stops a recording in progress, keeping what was recorded for
// the next launch to transcribe, and stops the background work and waits
// for it: a download or a transcription in flight is cancelled.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.closing = true
	r := m.rec
	cancel := m.cancel
	m.mu.Unlock()
	if r != nil {
		_ = m.finish(r, false)
	}
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	// Stopped, and able to start again: an update that could not begin
	// hands the feature back.
	m.mu.Lock()
	m.started, m.closing = false, false
	m.mu.Unlock()
}

func (m *Manager) enqueue(id string) {
	select {
	case m.queue <- id:
	default:
	}
}

// Status reads the feature's state.
func (m *Manager) Status() Status {
	s := m.Settings()
	model := m.ChosenModel(s)
	asset := m.Catalogue.Models[model]
	st := Status{
		On: s.On, Model: model, ModelName: asset.Name,
		ModelReady:   m.Models.Present(asset) && m.Models.Present(m.Catalogue.VAD),
		Transcribing: map[string]int{},
		OnGPU:        m.tierFor(s) == TierGPU,
		GPUOffer:     m.GPUOffered(s),
	}
	if m.MeasuresGPU {
		cal := m.readCalibration()
		if d, ok := cal.GPU.best(); ok {
			st.GPU = &d
		}
		st.GPUChecked = cal.GPU != nil
	}
	if m.EngineReady != nil {
		if err := m.EngineReady(); err != nil {
			st.EngineError = err.Error()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st.Prepare = m.prep
	for id, p := range m.jobs {
		st.Transcribing[id] = p
	}
	if r := m.rec; r != nil {
		st.Recording = r.status()
	}
	if o := m.offer; o != nil {
		cp := *o
		st.Offer = &cp
	}
	return st
}

func (r *recording) status() *RecordingStatus {
	st := &RecordingStatus{
		ID: r.m.ID, Started: r.m.Started, App: r.m.App, FarSide: r.m.FarSide, Paused: r.paused, How: r.how, Warning: r.warning,
		Microphone: r.m.Microphone, MicMissing: r.m.MicMissing,
	}
	st.MicLost, st.MicNone = micState(r.m.MicChanges)
	if r.window != "" {
		st.Title = r.m.Title
	}
	return st
}

// indicate tells Indicate the recording as it now stands.
func (m *Manager) indicate() {
	if m.Indicate == nil {
		return
	}
	m.mu.Lock()
	var st *RecordingStatus
	if r := m.rec; r != nil && !r.stopping {
		st = r.status()
	}
	m.mu.Unlock()
	m.Indicate(st)
}

func (m *Manager) notify(n Notice) error {
	if m.Notify == nil {
		return nil
	}
	return m.Notify(n)
}

func (m *Manager) withdraw(tag string) {
	if m.Withdraw != nil {
		m.Withdraw(tag)
	}
}

// The actions of the feature's notifications' buttons.
const (
	ActionPause  = "pause"
	ActionResume = "resume"
	ActionStop   = "stop"
	// ActionRecord and ActionNotNow answer an offer: each is followed by
	// ":" and the offer's ID.
	ActionRecord = "record"
	ActionNotNow = "not-now"
	// ActionKeepGoing answers "Still recording?": the meeting is still on.
	ActionKeepGoing = "keep-going"
)

// announce raises, or replaces, the notification that says a recording is
// under way, with its Pause or Resume and its Stop. An error says it was not
// raised.
func (m *Manager) announce(r *recording) error {
	// noticeMu orders this against finish's withdrawal: a recording that
	// has stopped is never announced again.
	m.noticeMu.Lock()
	defer m.noticeMu.Unlock()
	m.mu.Lock()
	if m.rec != r || r.stopping {
		m.mu.Unlock()
		return nil
	}
	app, far, paused, how, title, missing := r.m.App, r.m.FarSide, r.paused, r.how, r.m.Title, r.m.MicMissing
	m.mu.Unlock()
	what := "your microphone and everything your computer plays"
	switch {
	case how == InRoom:
		what = "your microphone in this room"
	case far == FarSideAppDevice && app != "":
		what = "your microphone and everything played on the speakers or headset " + app + " uses"
	case far != FarSideWholeDevice && app != "":
		what = "your microphone and " + app
	}
	n := Notice{Tag: tagRecording, Resident: true, Title: "Recording a meeting", Body: "Agentry is recording " + what + ". " + ConsentSentence}
	if how == InRoom {
		n.Title = "Recording a meeting in the room"
	}
	if how == Automatically {
		n.Title = "Recording a meeting automatically"
		n.Body = "Agentry is recording " + what + ", which you chose to record automatically. " + ConsentSentence
	}
	if how == InWindow {
		n.Title = "Recording " + title
		n.Body = "Agentry is recording " + what + " for this meeting from your calendar, which your Assistant set up to record. " + ConsentSentence
	}
	if missing {
		n.Body += " " + micMissingSentence
	}
	pause := NoticeAction{ID: ActionPause, Label: "Pause"}
	if paused {
		n.Title = "Meeting recording paused"
		n.Body = "Nothing is recorded until you resume. The transcript marks the gap."
		pause = NoticeAction{ID: ActionResume, Label: "Resume"}
	}
	n.Actions = []NoticeAction{pause, {ID: ActionStop, Label: "Stop"}}
	return m.notify(n)
}

// micWords names a microphone in a sentence.
func micWords(name string) string {
	if name == "" {
		return "the default microphone"
	}
	return name
}

// micMissingSentence says the chosen microphone was not there.
const micMissingSentence = "The microphone chosen in Meeting notes' settings was not found, so the default microphone is recorded."

// Meetings lists the meetings, newest first.
func (m *Manager) Meetings() ([]*Meeting, error) { return m.store().List() }

// HasAudio says whether a meeting's audio is still kept.
func (m *Manager) HasAudio(mt *Meeting) bool { return m.store().HasAudio(mt) }

// ErrOff refuses a recording or a transcription while the feature is off.
var ErrOff = errors.New("meeting notes is off: turn it on first")

// Record starts recording, at the person's own Record: the call under way,
// or the whole default devices when there is none. It refuses while the
// feature is off or while another recording runs.
func (m *Manager) Record(ctx context.Context) (*Meeting, error) {
	return m.startRecording(ctx, Call{Mic: m.Settings().Mic}, ByHand, nil)
}

// ErrNoRooms refuses a meeting in the room where the recorder cannot
// record one.
var ErrNoRooms = errors.New("recording a meeting in the room is not available on this computer")

// RecordRoom starts recording a meeting in the room, at the person's own
// Record in the room: the chosen microphone alone, no far side, no app
// followed. It refuses while the feature is off, while another recording
// runs, and where the recorder cannot record a room.
func (m *Manager) RecordRoom(ctx context.Context, people int) (*Meeting, error) {
	if err := CheckPeople(people); err != nil {
		return nil, err
	}
	mt, err := m.startRecording(ctx, Call{Room: true, Mic: m.Settings().Mic}, InRoom, nil)
	if err != nil {
		return nil, err
	}
	if people > 0 {
		// On the recording's own record, which its end saves.
		m.mu.Lock()
		if r := m.rec; r != nil && r.m.ID == mt.ID {
			r.m.People = people
			_ = m.store().Save(r.m)
		}
		m.mu.Unlock()
	}
	m.prepareSpeakers()
	return mt, nil
}

// prepareSpeakers fetches the speaker separation's models in the
// background, with its progress shown as the transcriber's is, the first
// time a meeting in the room is recorded with it on. The transcription
// fetches them again if this has not finished.
func (m *Manager) prepareSpeakers() {
	s := m.Settings()
	if s.SpeakersOff || m.Diarizer == nil {
		return
	}
	d := m.Catalogue.Diarization
	if m.Models.Present(d.Segmentation) && m.Models.Present(d.Embedding) {
		return
	}
	m.mu.Lock()
	if m.prep.Busy {
		m.mu.Unlock()
		return
	}
	m.prep = PrepareStatus{Busy: true, Step: "Getting ready to tell speakers apart"}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	m.wg.Add(1)
	m.mu.Unlock()
	m.changed()
	go func() {
		defer m.wg.Done()
		var err error
		for _, a := range []Asset{d.Segmentation, d.Embedding} {
			if _, err = m.fetch(ctx, a, "Downloading "+a.Name); err != nil {
				break
			}
		}
		m.mu.Lock()
		m.prep = PrepareStatus{}
		if err != nil {
			m.prep.Error = "the speaker separation's models could not be downloaded: " + err.Error()
		}
		m.mu.Unlock()
		m.changed()
	}()
}

// RecordsRooms says whether this computer's recorder records a meeting in
// the room.
func (m *Manager) RecordsRooms() bool {
	_, ok := m.Recorder.(RoomRecorder)
	return ok
}

// Microphones lists the microphones a recording may use, where the
// recorder can say; none where it cannot.
func (m *Manager) Microphones() ([]Microphone, error) {
	rr, ok := m.Recorder.(RoomRecorder)
	if !ok {
		return nil, nil
	}
	return rr.Microphones()
}

// startRecording is every way into a recording: it records call (zero for
// whatever the recorder finds) and announces it, whatever started it. w is
// the armed window it is recorded in, nil for none: the meeting takes its
// title and invitees.
func (m *Manager) startRecording(ctx context.Context, call Call, how How, w *Window) (*Meeting, error) {
	s := m.Settings()
	if !s.On {
		return nil, ErrOff
	}
	// A room is the person's own press, never an offer, an app on the
	// automatic list or an armed window; and only a recorder that records
	// rooms is asked for one.
	if call.Room != (how == InRoom) {
		return nil, errors.New("only Record in the room records a meeting in the room")
	}
	if call.Room && !m.RecordsRooms() {
		return nil, ErrNoRooms
	}
	m.mu.Lock()
	if m.rec != nil {
		m.mu.Unlock()
		return nil, errors.New("a meeting is already being recorded")
	}
	started := m.now()
	st := m.store()
	mt := &Meeting{
		ID: st.NewID(started), Started: started, State: StateRecording,
		Title:   "Meeting " + started.Format("2006-01-02 15:04"),
		Profile: m.Catalogue.Profile(s.Profile).ID,
	}
	if err := st.Create(mt); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if call.Room {
		mt.Title = "Meeting in the room " + started.Format("2006-01-02 15:04")
	}
	r := &recording{m: mt, names: map[string]bool{}, how: how, quietFrom: started}
	if w != nil {
		mt.Title, mt.Window, mt.Invitees = w.Title, w.ID, w.Invitees
		r.window = w.ID
		_ = st.Save(mt)
		m.recordedInLocked(w.ID, mt.ID)
	}
	m.rec = r
	offer := m.offer
	m.offer = nil
	m.mu.Unlock()
	if offer != nil {
		m.withdrawOffer()
	}

	capture, err := m.Recorder.Record(ctx, call, &recordingTracks{m: m, r: r})
	if err != nil {
		m.mu.Lock()
		m.rec = nil
		m.mu.Unlock()
		for _, t := range r.tracks {
			t.End()
		}
		_ = st.Delete(mt.ID)
		return nil, err
	}
	m.mu.Lock()
	if m.rec != r || r.stopping {
		// Stopped while the capture was starting: let it go at once, so no
		// device stays open for a recording nobody sees.
		m.mu.Unlock()
		_ = capture.Stop()
		for _, t := range r.tracks {
			t.End()
		}
		return nil, errors.New("the recording was stopped as it started")
	}
	r.capture = capture
	mt.App = capture.Call().App
	if mt.App == "" {
		mt.App = call.App
	}
	mt.FarSide = capture.FarSide()
	if mr, ok := capture.(MicReporter); ok {
		mt.Microphone, mt.MicMissing = mr.Microphone()
	}
	_ = st.Save(mt)
	m.mu.Unlock()

	if err := m.announce(r); err != nil && (how == Automatically || how == InWindow) {
		// Nobody pressed Record, and nothing could say it is recording:
		// what was started is let go and nothing is kept.
		m.abandon(r)
		return nil, fmt.Errorf("no notification could say it is recording: %w", err)
	}
	m.indicate()
	go m.follow(ctx, r, capture)
	m.changed()
	return mt, nil
}

// abandon lets a recording go that has only just started, keeping nothing.
func (m *Manager) abandon(r *recording) {
	m.mu.Lock()
	if m.rec != r {
		m.mu.Unlock()
		return
	}
	r.stopping = true
	m.rec = nil
	m.mu.Unlock()
	if r.capture != nil {
		_ = r.capture.Stop()
	}
	for _, t := range r.tracks {
		t.End()
	}
	_ = m.store().Delete(r.m.ID)
	m.changed()
}

// follow watches a recording until it ends: a capture whose far side
// widens to the whole device is said in its meeting and a notification, a
// warning the capture raises is said once in a notification and shown with
// the recording, and a capture that ends by itself, or reaches the longest
// a recording runs, is finished and transcribed.
func (m *Manager) follow(ctx context.Context, r *recording, capture Capture) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-capture.Ended():
			_ = m.finish(r, true)
			return
		case <-ctx.Done():
			return
		case <-tick.C:
			m.mu.Lock()
			if m.rec != r || r.stopping {
				m.mu.Unlock()
				return
			}
			widened := capture.FarSide() == FarSideWholeDevice && r.m.FarSide != FarSideWholeDevice
			if widened {
				r.m.FarSide = FarSideWholeDevice
				_ = m.store().Save(r.m)
			}
			// A microphone that goes away, or another taken up in its
			// place, is said, and kept for the transcript.
			var micNews []MicChangeRecord
			if mf, ok := capture.(MicFollower); ok {
				changes := mf.MicChanges()
				for _, c := range changes[min(r.micSeen, len(changes)):] {
					rec := MicChangeRecord{At: c.At.Sub(r.m.Started), Lost: c.Lost, Now: c.Now}
					r.m.MicChanges = append(r.m.MicChanges, rec)
					micNews = append(micNews, rec)
					if c.Now != "" {
						r.m.Microphone = c.Now
					}
				}
				r.micSeen = len(changes)
				if len(micNews) > 0 {
					_ = m.store().Save(r.m)
				}
			}
			// A microphone that changes while recording is said, never
			// taken silently: by its change where the capture keeps them,
			// otherwise when the microphone it reports changes.
			var micChanged bool
			_, follows := capture.(MicFollower)
			if mr, ok := capture.(MicReporter); ok {
				name, missing := mr.Microphone()
				if name != r.m.Microphone || missing != r.m.MicMissing {
					micChanged = !follows && (r.m.Microphone != "" || missing)
					r.m.Microphone, r.m.MicMissing = name, missing
					_ = m.store().Save(r.m)
				}
			}
			// A new warning is said; one that cleared is taken off the page.
			var warned *Warning
			cleared := false
			if w, ok := capture.(Warner); ok {
				got := w.Warning()
				switch {
				case got != nil && (r.warning == nil || *r.warning != *got):
					r.warning, warned = got, got
				case got == nil && r.warning != nil:
					r.warning, cleared = nil, true
				}
			}
			now := m.now()
			ran := now.Sub(r.m.Started)
			ask, spoke := m.quietLocked(r, now)
			m.mu.Unlock()
			if ask || spoke {
				m.quietNotice(r, ask)
			}
			if warned != nil {
				_ = m.notify(Notice{Title: "Meeting notes may not be hearing the meeting", Body: warned.Text})
			}
			if warned != nil || cleared {
				m.changed()
			}
			if limit := m.maxLength(); ran >= limit {
				_ = m.finish(r, true)
				_ = m.notify(Notice{Title: "Meeting recording stopped", Body: "Agentry stopped recording after " + lengthWords(limit) + ", the longest a recording runs. The transcript is on its way."})
				return
			}
			if widened {
				_ = m.notify(Notice{Title: "Recording everything your computer plays", Body: "The meeting app's sound could not be recorded on its own, so Agentry now records everything its output plays. The transcript says so."})
				_ = m.announce(r)
				m.changed()
			}
			for i, c := range micNews {
				before := r.m.MicChanges[:len(r.m.MicChanges)-len(micNews)+i]
				n := micChangeNotice(c, before)
				if c.Lost == "" && c.Now != "" && r.m.MicMissing {
					n.Body += " " + micMissingSentence
				}
				_ = m.notify(n)
			}
			if len(micNews) > 0 {
				m.changed()
			}
			if micChanged {
				name, missing := capture.(MicReporter).Microphone()
				body := "Agentry now records " + micWords(name) + "."
				if missing {
					body += " " + micMissingSentence
				}
				_ = m.notify(Notice{Title: "Recording another microphone", Body: body})
				_ = m.announce(r)
				m.changed()
			}
		}
	}
}

// micChangeNotice says one change of the microphone read: one that went
// away, and what is read instead or that nothing is; or, after that, the
// microphone read from now on. before are the changes that came before c.
func micChangeNotice(c MicChangeRecord, before []MicChangeRecord) Notice {
	n := Notice{Tag: tagMic}
	switch {
	case c.Lost != "" && c.Now != "":
		n.Title = "Microphone disconnected"
		n.Body = c.Lost + " was disconnected, so Agentry now records " + c.Now + " instead."
	case c.Lost != "":
		n.Title = "Microphone disconnected"
		n.Body = c.Lost + " was disconnected and no other microphone could be read, so nothing is recorded from a microphone until one can be. The transcript marks the gap."
	case c.Now != "":
		n.Title = "Recording another microphone"
		n.Body = "Agentry now records " + c.Now + "."
		if lost, _ := micState(before); lost == c.Now {
			n.Title = "Microphone connected again"
			n.Body = c.Now + " is connected again, and Agentry records it."
		}
	}
	return n
}

// micState is what the changes leave: the microphone that went away and
// is not read again ("" for none), and whether none is read now.
func micState(changes []MicChangeRecord) (lost string, none bool) {
	for _, c := range changes {
		if c.Lost != "" {
			lost = c.Lost
		}
		if c.Now == lost {
			lost = ""
		}
		none = c.Now == ""
	}
	return lost, none
}

// quietLocked follows a meeting in the room's silence: ask is set when it
// has gone QuietAfter without speech and the person has not been asked in
// this quiet stretch; spoke when someone spoke after they were asked, which
// ends the stretch. A paused recording is not quiet: nothing is listened
// to. m.mu is held.
func (m *Manager) quietLocked(r *recording, now time.Time) (ask, spoke bool) {
	if r.how != InRoom || r.paused {
		return false, false
	}
	last := r.quietFrom
	for _, t := range r.tracks {
		if l := t.LastLoud(); l.After(last) {
			last = l
		}
	}
	if r.quietAsked {
		if last.After(r.askedAt) {
			r.quietAsked = false
			return false, true
		}
		return false, false
	}
	if now.Sub(last) < m.quietAfter() {
		return false, false
	}
	r.quietAsked, r.askedAt = true, now
	return true, false
}

// quietNotice asks whether a quiet meeting in the room is still on (ask),
// or takes the question away once someone has spoken. Without an answer
// the recording goes on: only Stop, the longest length or quitting end it.
// noticeMu orders it against finish's withdrawal, so a recording that has
// stopped is never asked about.
func (m *Manager) quietNotice(r *recording, ask bool) {
	m.noticeMu.Lock()
	defer m.noticeMu.Unlock()
	m.mu.Lock()
	gone := m.rec != r || r.stopping
	m.mu.Unlock()
	if gone {
		return
	}
	if !ask {
		m.withdraw(tagQuiet)
		return
	}
	_ = m.notify(Notice{
		Tag: tagQuiet, Resident: true, Title: "Still recording?",
		Body: "Nobody has spoken for " + lengthWords(m.quietAfter()) + ". Agentry is still recording your microphone in this room, " +
			"until you press Stop or it has run " + lengthWords(m.maxLength()) + ".",
		Actions: []NoticeAction{{ID: ActionStop, Label: "Stop"}, {ID: ActionKeepGoing, Label: "Keep going"}},
	})
}

// quietAfter is how long a room goes without speech before it is asked
// about.
func (m *Manager) quietAfter() time.Duration {
	if m.QuietAfter > 0 {
		return m.QuietAfter
	}
	return DefaultQuietAfter
}

// KeepGoing answers "Still recording?": the meeting is on, so the question
// is taken away and not asked again until someone has spoken.
func (m *Manager) KeepGoing() error {
	m.withdraw(tagQuiet)
	return nil
}

// SetPaused pauses or resumes the recording in progress.
func (m *Manager) SetPaused(paused bool) error {
	m.mu.Lock()
	r := m.rec
	if r == nil || r.capture == nil || r.stopping {
		m.mu.Unlock()
		return errors.New("no meeting is being recorded")
	}
	if r.paused == paused {
		m.mu.Unlock()
		return nil
	}
	capture := r.capture
	m.mu.Unlock()
	// Outside the lock: the recorder may be opening a track, which takes it.
	if err := capture.SetPaused(paused); err != nil {
		return err
	}
	defer m.changed()
	defer func() {
		_ = m.announce(r)
		m.indicate()
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec != r || r.paused == paused {
		return nil
	}
	at := m.now().Sub(r.m.Started)
	if paused {
		r.pausedAt = at
	} else {
		r.m.Gaps = append(r.m.Gaps, Gap{From: r.pausedAt, To: at, Reason: "paused"})
		// The silence of a room is counted again from the resume.
		r.quietFrom = m.now()
	}
	r.paused = paused
	_ = m.store().Save(r.m)
	return nil
}

// Stop ends the recording in progress and queues its transcription.
func (m *Manager) Stop() error {
	m.mu.Lock()
	r := m.rec
	m.mu.Unlock()
	if r == nil {
		return errors.New("no meeting is being recorded")
	}
	return m.finish(r, true)
}

// finish ends a recording: the capture stopped, its tracks closed, and the
// meeting left pending; transcribe queues it now, otherwise the next
// launch does.
func (m *Manager) finish(r *recording, transcribe bool) error {
	m.mu.Lock()
	if m.rec != r || r.stopping {
		m.mu.Unlock()
		return nil
	}
	r.stopping = true
	m.mu.Unlock()

	var err error
	if r.capture != nil {
		err = r.capture.Stop()
	}
	defer m.changed()
	// After the lock is let go: the recording's notification taken away
	// and the tray told.
	defer func() {
		m.noticeMu.Lock()
		m.withdraw(tagRecording)
		if r.how == InRoom {
			m.withdraw(tagQuiet)
		}
		m.noticeMu.Unlock()
		m.indicate()
		// A call that waited while this one recorded has its turn.
		m.consider()
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if r.paused {
		r.m.Gaps = append(r.m.Gaps, Gap{From: r.pausedAt, To: now.Sub(r.m.Started), Reason: "paused"})
	}
	r.m.Ended = now
	r.m.Gaps = append(r.m.Gaps, micGaps(r.m.MicChanges, now.Sub(r.m.Started))...)
	slices.SortStableFunc(r.m.Gaps, func(a, b Gap) int { return cmp.Compare(a.From, b.From) })
	for i, t := range r.tracks {
		t.End()
		r.m.Tracks[i].Silent = t.Silent()
	}
	r.m.State = StatePending
	_ = m.store().Save(r.m)
	m.rec = nil
	if transcribe {
		m.enqueue(r.m.ID)
	}
	return err
}

// recordingTracks opens a recording's tracks as WAV files in its folder.
type recordingTracks struct {
	m *Manager
	r *recording
}

func (t *recordingTracks) Open(label Label, name string) (Track, error) {
	t.m.mu.Lock()
	defer t.m.mu.Unlock()
	base := Slug(name)
	file := base
	for i := 2; t.r.names[file]; i++ {
		file = fmt.Sprintf("%s-%d", base, i)
	}
	t.r.names[file] = true
	file += ".wav"
	st := t.m.store()
	w, err := newWAVTrack(filepath.Join(st.folder(t.r.m.ID), file), t.r.m.Started, t.m.now)
	if err != nil {
		return nil, err
	}
	t.r.tracks = append(t.r.tracks, w)
	t.r.m.Tracks = append(t.r.m.Tracks, TrackRecord{File: file, Label: label})
	_ = st.Save(t.r.m)
	return w, nil
}

// Retry queues a failed or pending meeting's transcription again.
func (m *Manager) Retry(id string) error {
	if !m.Settings().On {
		return ErrOff
	}
	st := m.store()
	mt, err := st.Get(id)
	if err != nil {
		return err
	}
	if mt.State != StateFailed && mt.State != StatePending {
		return fmt.Errorf("the meeting is %s, not waiting to be transcribed", mt.State)
	}
	if !st.HasAudio(mt) {
		return errors.New("the meeting's audio is no longer kept")
	}
	mt.State, mt.Error = StatePending, ""
	if err := st.Save(mt); err != nil {
		return err
	}
	m.enqueue(id)
	m.changed()
	return nil
}

// DeleteAudio deletes a finished meeting's kept audio now.
func (m *Manager) DeleteAudio(id string) error {
	st := m.store()
	mt, err := st.Get(id)
	if err != nil {
		return err
	}
	if mt.State == StateRecording || mt.State == StateTranscribing {
		return fmt.Errorf("the meeting is %s", mt.State)
	}
	if err := st.DeleteAudio(mt); err != nil {
		return err
	}
	m.changed()
	return nil
}

// Forget removes a meeting from the list, with any audio it keeps. Its
// transcript stays in the output folder.
func (m *Manager) Forget(id string) error {
	st := m.store()
	mt, err := st.Get(id)
	if err != nil {
		return err
	}
	if mt.State == StateRecording || mt.State == StateTranscribing {
		return fmt.Errorf("the meeting is %s", mt.State)
	}
	if err := st.Delete(id); err != nil {
		return err
	}
	m.changed()
	return nil
}

func (m *Manager) work(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-m.queue:
			if !m.Settings().On {
				continue
			}
			m.transcribe(ctx, id)
		}
	}
}

func (m *Manager) setJob(id string, pct int) {
	m.mu.Lock()
	if pct < 0 {
		delete(m.jobs, id)
	} else {
		m.jobs[id] = pct
	}
	m.mu.Unlock()
	m.changed()
}

// transcribe turns a pending meeting into its transcript: each track that
// is not silent transcribed, the sides merged, the transcript written to
// the output folder, and then the audio deleted unless it is kept.
func (m *Manager) transcribe(ctx context.Context, id string) {
	st := m.store()
	mt, err := st.Get(id)
	if err != nil || mt.State != StatePending {
		return
	}
	mt.State = StateTranscribing
	_ = st.Save(mt)
	m.setJob(id, 0)
	defer m.setJob(id, -1)

	path, err := m.transcribeTracks(ctx, mt)
	s := m.Settings()
	if err != nil {
		if ctx.Err() != nil {
			mt.State = StatePending // the app is quitting: the next launch tries again
		} else {
			mt.State, mt.Error = StateFailed, err.Error()
			mt.AudioUntil = m.now().Add(KeepFor)
		}
		_ = st.Save(mt)
		return
	}
	mt.State, mt.Error, mt.Transcript = StateDone, "", path
	if s.KeepAudio {
		mt.AudioUntil = m.now().Add(KeepFor)
		_ = st.Save(mt)
	} else if err := st.DeleteAudio(mt); err != nil {
		mt.Error = "the audio could not be deleted: " + err.Error()
		mt.AudioUntil = m.now()
		_ = st.Save(mt)
	}
	if m.Transcribed != nil {
		m.Transcribed(id)
	}
}

// Meeting reads one meeting's record.
func (m *Manager) Meeting(id string) (*Meeting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store().Get(id)
}

// Update changes a meeting's record under the manager's lock, so two
// changes are never lost to each other.
func (m *Manager) Update(id string, change func(*Meeting)) (*Meeting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.store()
	mt, err := st.Get(id)
	if err != nil {
		return nil, err
	}
	change(mt)
	if err := st.Save(mt); err != nil {
		return nil, err
	}
	return mt, nil
}

func (m *Manager) transcribeTracks(ctx context.Context, mt *Meeting) (string, error) {
	if m.EngineReady != nil {
		if err := m.EngineReady(); err != nil {
			return "", err
		}
	}
	s := m.Settings()
	profile := m.Catalogue.Profile(mt.Profile)
	model := m.ChosenModel(s)
	mt.Model = model
	gpu, device := m.gpuFor(s)
	modelPath, err := m.Models.Ensure(ctx, m.Catalogue.Models[model], nil)
	if err != nil {
		return "", err
	}
	vadPath, err := m.Models.Ensure(ctx, m.Catalogue.VAD, nil)
	if err != nil {
		return "", err
	}
	st := m.store()
	var todo []TrackRecord
	inRoom := mt.InRoom()
	silent := map[Label]bool{Me: true, Them: true}
	if inRoom {
		silent = map[Label]bool{Room: true}
	}
	for _, t := range mt.Tracks {
		quiet := t.Silent
		if !quiet {
			// A track repaired after a crash was never measured.
			if q, err := SilentWAV(st.TrackPath(mt, t)); err == nil {
				quiet = q
			}
		}
		if !quiet {
			todo = append(todo, t)
			silent[t.Label] = false
		}
	}
	var me, them, room []Segment
	var roomWords []Word
	var roomTrack string
	lang := profile.Language
	// Telling a room's speakers apart is one more step after the tracks,
	// and its progress shows with theirs.
	steps := len(todo)
	if inRoom {
		steps++
	}
	for i, t := range todo {
		res, err := m.Engine.Transcribe(ctx, TranscribeRequest{
			Audio: st.TrackPath(mt, t), Model: modelPath, VADModel: vadPath, Language: profile.Language,
			GPU: gpu, Device: device,
			Progress: func(p int) { m.setJob(mt.ID, (i*100+p)/steps) },
		})
		if err != nil {
			return "", err
		}
		if res.Backend != "" {
			mt.Backend = res.Backend
		}
		if res.Language != "" && lang == "auto" {
			lang = res.Language
		}
		switch t.Label {
		case Me:
			me = append(me, res.Segments...)
		case Room:
			room = append(room, res.Segments...)
			if roomTrack == "" {
				roomTrack, roomWords = st.TrackPath(mt, t), wordsOf(res)
			}
		default:
			them = append(them, res.Segments...)
		}
	}
	info := TranscriptInfo{
		Title: mt.Title, Started: mt.Started, Ended: mt.Ended, App: mt.App, FarSide: mt.FarSide,
		Language: lang, Profile: profile.ID, Model: model, Gaps: mt.Gaps,
		FromCalendar: mt.Window != "", Participants: mt.Invitees,
		InRoom: inRoom, Microphone: mt.Microphone, MicMissing: mt.MicMissing,
		MicChanges: mt.MicChanges,
	}
	for _, l := range []Label{Me, Them, Room} {
		if on, ok := silent[l]; ok && on {
			info.SilentSides = append(info.SilentSides, l)
		}
	}
	lines := Merge(me, them)
	if inRoom {
		lines = MergeRoom(room)
		if speaker, ok := m.separateSpeakers(ctx, mt, s, roomTrack, roomWords, len(todo)*100/steps); ok {
			lines = speaker
		}
		if ctx.Err() != nil {
			// The app is quitting: the meeting waits for the next launch.
			return "", ctx.Err()
		}
		info.Diarization, info.Speakers, info.People, info.DiarizationError = mt.Diarization, mt.Speakers, mt.People, mt.DiarizationError
	}
	doc, err := Render(info, lines)
	if err != nil {
		return "", err
	}
	if s.OutputDir == "" {
		return "", errors.New("no output folder is set for the transcripts")
	}
	slug := mt.App
	if mt.Window != "" {
		slug = mt.Title
	}
	if inRoom {
		slug = "meeting in the room"
	}
	if slug == "" {
		slug = "meeting"
	}
	return WriteTranscript(s.OutputDir, TranscriptName(mt.Started, slug), doc)
}

// separateSpeakers tells the voices of a meeting in the room apart and
// puts each line to its voice. It never costs the transcript: when it is
// off, cannot run, fails or runs too long (the models' download included),
// the meeting says why in a few plain words, the details go to the log,
// and the lines stay Room (ok false). from is where its progress starts.
func (m *Manager) separateSpeakers(ctx context.Context, mt *Meeting, s Settings, track string, words []Word, from int) (lines []Segment, ok bool) {
	mt.Speakers, mt.DiarizationError = 0, ""
	give := func(status, why string, err error) ([]Segment, bool) {
		mt.Diarization, mt.DiarizationError = status, why
		if err != nil {
			log.Printf("meeting notes: telling the speakers of %s apart: %v", mt.ID, err)
		}
		return nil, false
	}
	switch {
	case s.SpeakersOff:
		return give(DiarizationOff, "", nil)
	case m.Diarizer == nil:
		return give(DiarizationUnavailable, "this copy of Agentry does not carry the speaker separation", nil)
	case track == "" || len(words) == 0:
		return give(DiarizationUnavailable, "nothing was said that could be told apart", nil)
	}
	limit := m.DiarizeTimeout
	if limit <= 0 {
		limit = DefaultDiarizeTimeout
	}
	if !mt.Ended.IsZero() {
		limit += mt.Ended.Sub(mt.Started)
	}
	dctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	tooLong := func() bool { return dctx.Err() == context.DeadlineExceeded }
	d := m.Catalogue.Diarization
	seg, err := m.Models.Ensure(dctx, d.Segmentation, nil)
	var emb string
	if err == nil {
		emb, err = m.Models.Ensure(dctx, d.Embedding, nil)
	}
	if err != nil {
		if tooLong() {
			return give(DiarizationFailed, "it ran longer than "+lengthWords(limit)+" and was stopped", err)
		}
		return give(DiarizationUnavailable, "its models could not be downloaded or checked", err)
	}
	spans, err := m.Diarizer.Diarize(dctx, DiarizeRequest{
		Audio: track, Segmentation: seg, Embedding: emb, Speakers: mt.People,
		Progress: func(p int) { m.setJob(mt.ID, from+p*(100-from)/100) },
	})
	switch {
	case tooLong():
		return give(DiarizationFailed, "it ran longer than "+lengthWords(limit)+" and was stopped", err)
	case err != nil:
		return give(DiarizationFailed, "the speaker separation stopped with an error", err)
	}
	lines, voices, ok := SpeakerLines(words, spans)
	if !ok {
		return give(DiarizationFailed, "no word could be put to a voice", nil)
	}
	mt.Diarization, mt.Speakers = DiarizationDone, voices
	return lines, true
}

// calibration is calibration.json: each CPU model's measured real-time
// factor on this computer (processing time over audio time), and the
// graphics devices' measurement.
type calibration struct {
	Version int                `json:"version"`
	RTF     map[string]float64 `json:"rtf"`
	GPU     *GPUProbe          `json:"gpu,omitempty"`
}

// calibrationVersion is the measurement's version: a measurement of
// another version is taken again. A CPU model's pace is measured with the
// graphics backends held off, which a measurement without a version
// cannot be counted on to have done.
const calibrationVersion = 2

// GPUProbe is the measurement of the transcriber's graphics devices: each
// device's pace with Model, the CPU tier's first model.
type GPUProbe struct {
	// Engine is the transcriber measured (Manager.EngineID).
	Engine  string      `json:"engine"`
	Model   string      `json:"model"`
	Devices []GPUDevice `json:"devices"`
}

// GPUDevice is one graphics device as measured. Backend is "" when the
// run on it did not use it (it failed, or fell back to the CPU).
type GPUDevice struct {
	Index   int     `json:"index"`
	Name    string  `json:"name"`
	Backend string  `json:"backend,omitempty"`
	RTF     float64 `json:"rtf,omitempty"`
}

// gpuTierCost is how much longer the graphics tier's model takes than the
// CPU tier's first model on the same audio and device: large-v3-turbo
// against small.en, on a minute of speech, took 2.7 times as long on an
// integrated graphics card, 3.6 on a discrete one and 3.7 on the CPU.
const gpuTierCost = 4.0

// best is the device that ran fastest, if one ran at all.
func (p *GPUProbe) best() (GPUDevice, bool) {
	var out GPUDevice
	found := false
	if p == nil {
		return out, false
	}
	for _, d := range p.Devices {
		if d.Backend != "" && d.RTF > 0 && d.Index >= 0 && d.Index < maxDevices && (!found || d.RTF < out.RTF) {
			out, found = d, true
		}
	}
	return out, found
}

// usable says whether the fastest device is expected to run the graphics
// tier's model faster than real time.
func (p *GPUProbe) usable() bool {
	d, ok := p.best()
	return ok && d.Fast()
}

// Fast says whether the device is expected to run the graphics tier's
// model faster than real time.
func (d GPUDevice) Fast() bool { return d.Backend != "" && d.RTF > 0 && d.RTF*gpuTierCost <= 1 }

func (m *Manager) calibrationPath() string { return filepath.Join(m.Root, "calibration.json") }

func (m *Manager) readCalibration() calibration {
	c := calibration{RTF: map[string]float64{}}
	if raw, err := os.ReadFile(m.calibrationPath()); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	if c.Version < calibrationVersion {
		c = calibration{}
	}
	if c.RTF == nil {
		c.RTF = map[string]float64{}
	}
	c.Version = calibrationVersion
	return c
}

func (m *Manager) writeCalibration(c calibration) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.calibrationPath(), raw, 0o600)
}

// onCPUTier says whether this computer's own tier is the CPU's, where a
// graphics card may be offered.
func (m *Manager) onCPUTier() bool { return m.Tier == TierCPU || m.Tier == "" }

// tierFor is the hardware tier transcriptions use: the computer's, or the
// graphics tier where the person chose the graphics card and a device ran
// a transcription when measured.
func (m *Manager) tierFor(s Settings) string {
	if !m.onCPUTier() {
		return m.Tier
	}
	if m.MeasuresGPU && s.Hardware == HardwareGPU {
		if _, ok := m.readCalibration().GPU.best(); ok {
			return TierGPU
		}
	}
	return TierCPU
}

// gpuFor says whether transcriptions run on a graphics device, and which.
func (m *Manager) gpuFor(s Settings) (bool, int) {
	switch m.tierFor(s) {
	case TierCPU:
		return false, 0
	case TierGPU:
		d, _ := m.readCalibration().GPU.best()
		return true, d.Index
	}
	return true, 0
}

// GPUModel is the graphics tier's model for the person's language profile.
func (m *Manager) GPUModel(s Settings) Asset {
	return m.Catalogue.Models[m.Catalogue.Choices(m.Catalogue.Profile(s.Profile).ID, TierGPU)[0]]
}

// GPUOffered says whether the graphics tier's model is offered: a device
// measured fast enough for it, and the person has not answered the offer,
// either way.
func (m *Manager) GPUOffered(s Settings) bool {
	return m.MeasuresGPU && m.onCPUTier() && s.Hardware == "" && m.readCalibration().GPU.usable()
}

// ModelChoices are the models offered where transcriptions run now.
func (m *Manager) ModelChoices(s Settings) []string {
	return m.Catalogue.Choices(m.Catalogue.Profile(s.Profile).ID, m.tierFor(s))
}

// ChosenModel is the model a transcription uses: the person's choice when
// the profile offers it; otherwise the tier's first model, or on the CPU
// the first that calibration measured faster than real time (the last one
// when none was).
func (m *Manager) ChosenModel(s Settings) string {
	p := m.Catalogue.Profile(s.Profile)
	if s.Model != "" && m.Catalogue.Allowed(p.ID, s.Model) {
		return s.Model
	}
	tier := m.tierFor(s)
	choices := m.Catalogue.Choices(p.ID, tier)
	if tier != TierCPU {
		return choices[0]
	}
	cal := m.readCalibration()
	for _, id := range choices {
		rtf, ok := cal.RTF[id]
		if !ok || rtf <= 1 {
			return id
		}
	}
	return choices[len(choices)-1]
}

// Prepare downloads what transcription needs and, on the CPU with the
// model left automatic, measures each model in turn until one runs faster
// than real time. It runs in the background; Status shows its progress.
func (m *Manager) Prepare(ctx context.Context) {
	m.mu.Lock()
	if m.prep.Busy {
		m.mu.Unlock()
		return
	}
	m.prep = PrepareStatus{Busy: true, Step: "Getting ready"}
	if m.ctx != nil {
		ctx = m.ctx // cancelled by Shutdown
	}
	m.wg.Add(1)
	m.mu.Unlock()
	m.changed()
	go func() {
		defer m.wg.Done()
		err := m.prepare(ctx)
		m.mu.Lock()
		m.prep = PrepareStatus{}
		if err != nil {
			m.prep.Error = err.Error()
		}
		m.mu.Unlock()
		m.changed()
	}()
}

func (m *Manager) step(step string, done, total int64) {
	m.mu.Lock()
	m.prep.Step, m.prep.Done, m.prep.Total = step, done, total
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) fetch(ctx context.Context, a Asset, what string) (string, error) {
	last := time.Time{}
	return m.Models.Ensure(ctx, a, func(done, total int64) {
		if time.Since(last) > 250*time.Millisecond || done == total {
			last = time.Now()
			m.step(what, done, total)
		}
	})
}

func (m *Manager) prepare(ctx context.Context) error {
	s := m.Settings()
	if !s.On {
		return ErrOff
	}
	vad, err := m.fetch(ctx, m.Catalogue.VAD, "Downloading voice detection")
	if err != nil {
		return err
	}
	p := m.Catalogue.Profile(s.Profile)
	auto := s.Model == "" || !m.Catalogue.Allowed(p.ID, s.Model)
	if !auto || m.tierFor(s) != TierCPU {
		model := m.ChosenModel(s)
		if _, err := m.fetch(ctx, m.Catalogue.Models[model], "Downloading "+m.Catalogue.Models[model].Name); err != nil {
			return err
		}
	} else if err := m.calibrateCPU(ctx, p, vad); err != nil {
		return err
	}
	return m.probeGPU(ctx, p, vad)
}

// calibrateCPU measures each CPU model in turn until one runs faster than
// real time.
func (m *Manager) calibrateCPU(ctx context.Context, p Profile, vad string) error {
	cal := m.readCalibration()
	choices := m.Catalogue.Choices(p.ID, TierCPU)
	for i, id := range choices {
		asset := m.Catalogue.Models[id]
		path, err := m.fetch(ctx, asset, "Downloading "+asset.Name)
		if err != nil {
			return err
		}
		if _, ok := cal.RTF[id]; !ok && i < len(choices)-1 {
			if m.EngineReady != nil && m.EngineReady() != nil {
				return nil // nothing to measure with; the first model stays the choice
			}
			_, audio, seconds, err := m.calibrationClip(ctx)
			if err != nil {
				return err
			}
			m.step("Measuring "+asset.Name+" on this computer", 0, 0)
			res, err := m.Engine.Transcribe(ctx, TranscribeRequest{Audio: audio, Model: path, VADModel: vad, Language: p.Language})
			_ = os.RemoveAll(filepath.Dir(audio))
			if err != nil {
				return fmt.Errorf("the calibration run failed: %w", err)
			}
			cal.RTF[id] = res.Took.Seconds() / seconds
			if err := m.writeCalibration(cal); err != nil {
				return err
			}
		}
		if rtf, ok := cal.RTF[id]; !ok || rtf <= 1 {
			return nil
		}
	}
	return nil
}

// calibrationClip fetches the calibration clip and writes from it the
// minute of speech a measurement transcribes; it returns both. The caller
// removes the minute's folder.
func (m *Manager) calibrationClip(ctx context.Context) (clip, audio string, seconds float64, err error) {
	clip, err = m.fetch(ctx, m.Catalogue.Calibration, "Downloading the calibration clip")
	if err != nil {
		return "", "", 0, err
	}
	audio, seconds, err = calibrationAudio(clip, filepath.Join(m.Root, "calibration"))
	return clip, audio, seconds, err
}

// probeGPU measures every graphics device the transcriber finds, once for
// each transcriber installed, with the CPU tier's first model (the one
// gpuTierCost is measured against), and only once it is downloaded. Each
// device has a warm-up run first, since its first run compiles shaders and
// can take half a minute, then a minute of speech timed. A device whose
// run fails or falls back to the CPU is recorded as not running it, and is
// not the preparation's failure: transcriptions stay on the CPU.
func (m *Manager) probeGPU(ctx context.Context, p Profile, vad string) error {
	if !m.MeasuresGPU || !m.onCPUTier() {
		return nil
	}
	if m.EngineReady != nil && m.EngineReady() != nil {
		return nil
	}
	engine := ""
	if m.EngineID != nil {
		engine = m.EngineID()
	}
	cal := m.readCalibration()
	if cal.GPU != nil && cal.GPU.Engine == engine {
		return nil
	}
	modelID := m.Catalogue.Choices(p.ID, TierCPU)[0]
	asset := m.Catalogue.Models[modelID]
	if !m.Models.Present(asset) {
		return nil // measured once the model is downloaded
	}
	path, err := m.Models.Ensure(ctx, asset, nil)
	if err != nil {
		return err
	}
	clip, audio, seconds, err := m.calibrationClip(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(audio)) }()
	probe := &GPUProbe{Engine: engine, Model: modelID, Devices: []GPUDevice{}}
	req := TranscribeRequest{Model: path, VADModel: vad, Language: p.Language, GPU: true}
	m.step("Checking your graphics card", 0, 0)
	warm := req
	warm.Audio = clip
	first, err := m.Engine.Transcribe(ctx, warm)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	devices := first.Devices
	if len(devices) == 0 && first.Backend != "" {
		devices = []string{first.Backend}
	}
	if err == nil {
		for i, name := range devices {
			d := GPUDevice{Index: i, Name: name}
			run := req
			run.Device = i
			if i > 0 {
				warm.Device = i
				_, _ = m.Engine.Transcribe(ctx, warm)
			}
			run.Audio = audio
			res, err := m.Engine.Transcribe(ctx, run)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err == nil && res.Backend != "" {
				d.Backend, d.RTF = res.Backend, res.Took.Seconds()/seconds
			}
			probe.Devices = append(probe.Devices, d)
		}
	}
	cal = m.readCalibration()
	cal.GPU = probe
	return m.writeCalibration(cal)
}

// ForgetGPU drops the graphics devices' measurement, so the next
// preparation measures them again.
func (m *Manager) ForgetGPU() error {
	cal := m.readCalibration()
	cal.GPU = nil
	return m.writeCalibration(cal)
}

// calibrationSeconds is how much speech a calibration run transcribes.
// Whisper encodes a whole 30-second window however little of it holds
// speech, so a short clip would make every model look slower than it is
// on a meeting; a minute and more of speech measures the steady pace.
const calibrationSeconds = 60

// calibrationAudio writes, into a fresh folder dir, the calibration clip
// said again and again with a second of silence between, until it lasts
// calibrationSeconds. It returns the file and its length in seconds. The
// clip must be 16 kHz mono 16-bit, as the pinned one is.
func calibrationAudio(clip, dir string) (string, float64, error) {
	samples, err := readPCM16(clip)
	if err != nil {
		return "", 0, fmt.Errorf("the calibration clip: %w", err)
	}
	if len(samples) == 0 {
		return "", 0, errors.New("the calibration clip is empty")
	}
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, err
	}
	gap := make([]int16, SampleRate)
	var all []int16
	for len(all) < calibrationSeconds*SampleRate {
		all = append(append(all, samples...), gap...)
	}
	out := filepath.Join(dir, "calibration.wav")
	raw := wavHeader(int64(len(all)))
	for _, v := range all {
		raw = append(raw, byte(v), byte(uint16(v)>>8))
	}
	if err := os.WriteFile(out, raw, 0o600); err != nil {
		return "", 0, err
	}
	return out, float64(len(all)) / SampleRate, nil
}
