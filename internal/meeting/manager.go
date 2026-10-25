package meeting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// OutputDir is the folder transcripts go to.
	OutputDir string
	// KeepAudio keeps recordings for KeepFor after their transcript.
	KeepAudio bool
}

// Manager runs Meeting notes: one recording at a time, then its
// transcription, in the background, one meeting after another.
type Manager struct {
	// Root holds the models, the meetings' folders and the calibration.
	Root      string
	Catalogue Catalogue
	Recorder  Recorder
	Engine    Engine
	Models    *Models
	Settings  func() Settings
	// Tier is this computer's hardware tier.
	Tier string
	// EngineReady says whether the transcriber can run; nil means it can.
	EngineReady func() error
	// Notify tells the person something in a desktop notification.
	Notify func(title, body string)
	// Changed is told whenever the status changes.
	Changed func()
	Now     func() time.Time

	mu      sync.Mutex
	rec     *recording
	prep    PrepareStatus
	jobs    map[string]int
	queue   chan string
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
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
	// Transcribing is each meeting being transcribed, by id, with its
	// progress from 0 to 100.
	Transcribing map[string]int `json:"transcribing"`
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
	m.wg.Add(2)
	go m.work(ctx)
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
		st.Recording = &RecordingStatus{ID: r.m.ID, Started: r.m.Started, App: r.m.App, FarSide: r.m.FarSide, Paused: r.paused}
	}
	return st
}

// Meetings lists the meetings, newest first.
func (m *Manager) Meetings() ([]*Meeting, error) { return m.store().List() }

// HasAudio says whether a meeting's audio is still kept.
func (m *Manager) HasAudio(mt *Meeting) bool { return m.store().HasAudio(mt) }

// ErrOff refuses a recording or a transcription while the feature is off.
var ErrOff = errors.New("meeting notes is off: turn it on first")

// Record starts recording. It refuses while the feature is off or while
// another recording runs.
func (m *Manager) Record(ctx context.Context) (*Meeting, error) {
	s := m.Settings()
	if !s.On {
		return nil, ErrOff
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
	r := &recording{m: mt, names: map[string]bool{}}
	m.rec = r
	m.mu.Unlock()

	capture, err := m.Recorder.Record(ctx, Call{}, &recordingTracks{m: m, r: r})
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
	r.capture = capture
	mt.App = capture.Call().App
	mt.FarSide = capture.FarSide()
	_ = st.Save(mt)
	m.mu.Unlock()

	what := "your microphone and everything your computer plays"
	if mt.App != "" {
		what = "your microphone and " + mt.App
	}
	if m.Notify != nil {
		m.Notify("Recording a meeting", "Agentry is recording "+what+". "+ConsentSentence)
	}
	go m.follow(ctx, r, capture)
	m.changed()
	return mt, nil
}

// follow watches a recording until it ends: a capture whose far side
// widens to the whole device is said in its meeting and a notification,
// and a capture that ends by itself is finished and transcribed.
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
			m.mu.Unlock()
			if widened {
				if m.Notify != nil {
					m.Notify("Recording everything your computer plays", "The meeting app's sound could not be recorded on its own, so Agentry now records everything its output plays. The transcript says so.")
				}
				m.changed()
			}
		}
	}
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
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if r.paused {
		r.m.Gaps = append(r.m.Gaps, Gap{From: r.pausedAt, To: now.Sub(r.m.Started), Reason: "paused"})
	}
	r.m.Ended = now
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
	silent := map[Label]bool{Me: true, Them: true}
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
	var me, them []Segment
	lang := profile.Language
	for i, t := range todo {
		res, err := m.Engine.Transcribe(ctx, TranscribeRequest{
			Audio: st.TrackPath(mt, t), Model: modelPath, VADModel: vadPath, Language: profile.Language,
			Progress: func(p int) { m.setJob(mt.ID, (i*100+p)/len(todo)) },
		})
		if err != nil {
			return "", err
		}
		if res.Language != "" && lang == "auto" {
			lang = res.Language
		}
		if t.Label == Me {
			me = append(me, res.Segments...)
		} else {
			them = append(them, res.Segments...)
		}
	}
	info := TranscriptInfo{
		Title: mt.Title, Started: mt.Started, Ended: mt.Ended, App: mt.App, FarSide: mt.FarSide,
		Language: lang, Profile: profile.ID, Model: model, Gaps: mt.Gaps,
	}
	for _, l := range []Label{Me, Them} {
		if silent[l] {
			info.SilentSides = append(info.SilentSides, l)
		}
	}
	doc, err := Render(info, Merge(me, them))
	if err != nil {
		return "", err
	}
	if s.OutputDir == "" {
		return "", errors.New("no output folder is set for the transcripts")
	}
	slug := mt.App
	if slug == "" {
		slug = "meeting"
	}
	return WriteTranscript(s.OutputDir, TranscriptName(mt.Started, slug), doc)
}

// calibration is calibration.json: each CPU model's measured real-time
// factor on this computer (processing time over audio time).
type calibration struct {
	RTF map[string]float64 `json:"rtf"`
}

func (m *Manager) calibrationPath() string { return filepath.Join(m.Root, "calibration.json") }

func (m *Manager) readCalibration() calibration {
	c := calibration{RTF: map[string]float64{}}
	if raw, err := os.ReadFile(m.calibrationPath()); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	if c.RTF == nil {
		c.RTF = map[string]float64{}
	}
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

// ChosenModel is the model a transcription uses: the person's choice when
// the profile offers it; otherwise the tier's first model, or on the CPU
// the first that calibration measured faster than real time (the last one
// when none was).
func (m *Manager) ChosenModel(s Settings) string {
	p := m.Catalogue.Profile(s.Profile)
	if s.Model != "" && m.Catalogue.Allowed(p.ID, s.Model) {
		return s.Model
	}
	choices := m.Catalogue.Choices(p.ID, m.Tier)
	if m.Tier != TierCPU && m.Tier != "" {
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
	if !auto || (m.Tier != TierCPU && m.Tier != "") {
		model := m.ChosenModel(s)
		_, err := m.fetch(ctx, m.Catalogue.Models[model], "Downloading "+m.Catalogue.Models[model].Name)
		return err
	}
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
			clip, err := m.fetch(ctx, m.Catalogue.Calibration, "Downloading the calibration clip")
			if err != nil {
				return err
			}
			audio, seconds, err := calibrationAudio(clip, filepath.Join(m.Root, "calibration"))
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
