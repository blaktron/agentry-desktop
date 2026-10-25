package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/deliverables"
	"github.com/blaktron/agentry-desktop/internal/meeting"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventMeetingChanged tells the Meeting notes page to read its state again.
const eventMeetingChanged = "meeting:changed"

// meetingNoticeKey is the key of Meeting notes' notifications: a click
// opens its page.
const meetingNoticeKey = "meeting-notes"

// eventOpenMeetings asks the page to open Meeting notes.
const eventOpenMeetings = "meeting:open"

// MeetingOutputFolder is the folder inside the Agentry folder in Documents
// that transcripts go to unless the person chose another.
const MeetingOutputFolder = "Meeting notes"

// meetingWarnings are what turning Meeting notes on means, listed on the
// page and said again in the native dialog that turns it on.
var meetingWarnings = []string{
	meeting.ConsentSentence,
	"While it records a meeting, it captures your microphone even when you muted yourself in the meeting app. Use Pause for anything you do not want kept.",
	"When no meeting app can be recorded on its own, it records everything your computer plays, other sounds included, and the transcript says so.",
	"It downloads a speech model of about 200 MB the first time. No audio or transcript leaves this computer.",
}

// MeetingAPI is Meeting notes: record a meeting by hand, transcribe it on
// this computer, and write the transcript to a folder.
type MeetingAPI struct {
	app *App

	once sync.Once
	mgr  *meeting.Manager
	// unavailable says why this copy cannot record, "" when it can.
	unavailable string
	// sidecar is where this copy carries the transcriber; a test sets it.
	sidecar string
	// newRecorder and notifyShow are the platform's, a test's seams.
	newRecorder func() meeting.Recorder
	notifyShow  func(notify.Notice) error
	// root is the feature's data folder; "" is meeting-notes in the
	// config folder.
	root string
	// client fetches the models; nil is meeting.DownloadClient's.
	client *http.Client
}

// manager builds the feature's manager once.
func (m *MeetingAPI) manager() *meeting.Manager {
	m.once.Do(func() {
		cat, err := meeting.LoadCatalogue()
		if err != nil {
			m.unavailable = err.Error()
			return
		}
		root := m.root
		if root == "" {
			base, err := configdir.Dir()
			if err != nil {
				m.unavailable = err.Error()
				return
			}
			root = filepath.Join(base, "meeting-notes")
		}
		sidecar := m.sidecar
		if sidecar == "" {
			sidecar = meeting.SidecarPath()
		}
		rec := meeting.NewRecorder
		if m.newRecorder != nil {
			rec = m.newRecorder
		}
		show := notify.Show
		if m.notifyShow != nil {
			show = m.notifyShow
		}
		if runtime.GOOS != "linux" {
			m.unavailable = "Meeting notes records on Linux only."
		} else if err := meeting.CheckSidecar(sidecar); err != nil {
			m.unavailable = err.Error() + "."
		}
		m.mgr = &meeting.Manager{
			Root: root, Catalogue: cat, Recorder: rec(),
			Engine:      meeting.Sidecar{Path: sidecar},
			Models:      &meeting.Models{Dir: filepath.Join(root, "models"), Client: m.client},
			Settings:    meetingSettings,
			Tier:        meeting.TierCPU,
			EngineReady: func() error { return meeting.CheckSidecar(sidecar) },
			Notify: func(title, body string) {
				if err := show(notify.Notice{Title: title, Body: body, Key: meetingNoticeKey}); err != nil {
					log.Printf("meeting notes: notification: %v", err)
				}
			},
			Changed: func() { emitEvent(m.app, eventMeetingChanged) },
		}
	})
	return m.mgr
}

// meetingSettings reads Meeting notes' settings, the output folder
// defaulted.
func meetingSettings() meeting.Settings {
	st, err := settings.Load()
	if err != nil {
		return meeting.Settings{}
	}
	mn := st.MeetingNotes
	out := mn.OutputDir
	if out == "" {
		out, _ = defaultMeetingFolder()
	}
	return meeting.Settings{On: mn.On, Profile: mn.Profile, Model: mn.Model, OutputDir: out, KeepAudio: mn.KeepAudio}
}

func defaultMeetingFolder() (string, error) {
	root, err := deliverables.DefaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, MeetingOutputFolder), nil
}

// start is the feature's part of OnStartup: an earlier session's leftovers
// recovered, old audio swept, and the transcription worker started, even
// in a copy that cannot record, so no audio outlives its keep; with the
// feature on where it records, the model checked, fetched or measured if
// it was not. Nothing here opens an audio device.
func (m *MeetingAPI) start(ctx context.Context) {
	mgr := m.manager()
	if mgr == nil {
		return
	}
	mgr.Start(ctx)
	if m.unavailable == "" && meetingSettings().On {
		mgr.Prepare(ctx)
	}
}

// shutdown keeps a recording in progress for the next launch to transcribe.
func (m *MeetingAPI) shutdown() {
	if m.mgr != nil {
		m.mgr.Shutdown()
	}
}

// available is the manager when this copy can record, else why not.
func (m *MeetingAPI) available() (*meeting.Manager, error) {
	mgr, err := m.built()
	if err != nil || m.unavailable != "" {
		return nil, errors.New(m.unavailable)
	}
	return mgr, nil
}

// built is the manager, for what a copy that cannot record still does
// with the meetings it has: list, delete, open.
func (m *MeetingAPI) built() (*meeting.Manager, error) {
	if mgr := m.manager(); mgr != nil {
		return mgr, nil
	}
	return nil, errors.New(m.unavailable)
}

// MeetingChoice is a language profile or a model in a list to pick from.
type MeetingChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// SizeBytes is a model's download.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// MeetingRecordingView is the recording in progress.
type MeetingRecordingView struct {
	ID      string `json:"id"`
	Started string `json:"started"`
	App     string `json:"app"`
	// WholeDevice: no meeting app could be recorded on its own, so the
	// far side is everything the computer plays.
	WholeDevice bool `json:"wholeDevice"`
	Paused      bool `json:"paused"`
}

// MeetingView is one meeting in the list.
type MeetingView struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	App             string `json:"app"`
	WholeDevice     bool   `json:"wholeDevice"`
	Started         string `json:"started"`
	DurationSeconds int64  `json:"durationSeconds"`
	// State is recording, pending, transcribing, done or failed.
	State string `json:"state"`
	Error string `json:"error,omitempty"`
	// Progress is a transcription's, 0 to 100, while it runs.
	Progress       int    `json:"progress"`
	TranscriptName string `json:"transcriptName,omitempty"`
	HasTranscript  bool   `json:"hasTranscript"`
	HasAudio       bool   `json:"hasAudio"`
	AudioUntil     string `json:"audioUntil,omitempty"`
}

// MeetingNotesView is the Meeting notes page.
type MeetingNotesView struct {
	// Available: this copy records here. Unavailable says why not.
	Available   bool   `json:"available"`
	Unavailable string `json:"unavailable,omitempty"`
	On          bool   `json:"on"`
	// Warnings are what turning it on means.
	Warnings  []string              `json:"warnings"`
	Recording *MeetingRecordingView `json:"recording,omitempty"`
	// Preparing is the model's download or calibration, PrepareStep what
	// it is doing, PrepareError what stopped it.
	Preparing    bool   `json:"preparing"`
	PrepareStep  string `json:"prepareStep,omitempty"`
	PrepareDone  int64  `json:"prepareDone,omitempty"`
	PrepareTotal int64  `json:"prepareTotal,omitempty"`
	PrepareError string `json:"prepareError,omitempty"`
	// Model is the model transcription uses now; ModelReady whether it is
	// downloaded.
	Model      string `json:"model"`
	ModelName  string `json:"modelName"`
	ModelReady bool   `json:"modelReady"`
	// ModelChoice is the person's choice, "" for automatic.
	ModelChoice string          `json:"modelChoice"`
	Models      []MeetingChoice `json:"models"`
	Profile     string          `json:"profile"`
	Profiles    []MeetingChoice `json:"profiles"`
	OutputDir   string          `json:"outputDir"`
	// OutputDirChosen: the person chose the folder.
	OutputDirChosen bool          `json:"outputDirChosen"`
	KeepAudio       bool          `json:"keepAudio"`
	Meetings        []MeetingView `json:"meetings"`
}

// Status reads the page.
func (m *MeetingAPI) Status() (*MeetingNotesView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	mn := st.MeetingNotes
	v := &MeetingNotesView{
		On: mn.On, Warnings: meetingWarnings, ModelChoice: mn.Model, KeepAudio: mn.KeepAudio,
		OutputDirChosen: mn.OutputDir != "", Models: []MeetingChoice{}, Profiles: []MeetingChoice{}, Meetings: []MeetingView{},
	}
	mgr := m.manager()
	v.Available, v.Unavailable = mgr != nil && m.unavailable == "", m.unavailable
	if mgr == nil {
		return v, nil
	}
	s := meetingSettings()
	v.OutputDir = s.OutputDir
	cat := mgr.Catalogue
	profile := cat.Profile(mn.Profile)
	v.Profile = profile.ID
	for _, p := range cat.Profiles {
		v.Profiles = append(v.Profiles, MeetingChoice{ID: p.ID, Name: p.Name})
	}
	for _, id := range cat.Choices(profile.ID, mgr.Tier) {
		a := cat.Models[id]
		v.Models = append(v.Models, MeetingChoice{ID: id, Name: a.Name, SizeBytes: a.Size})
	}
	status := mgr.Status()
	v.Model, v.ModelName, v.ModelReady = status.Model, status.ModelName, status.ModelReady
	v.Preparing, v.PrepareStep, v.PrepareDone, v.PrepareTotal, v.PrepareError = status.Prepare.Busy, status.Prepare.Step, status.Prepare.Done, status.Prepare.Total, status.Prepare.Error
	if r := status.Recording; r != nil {
		v.Recording = &MeetingRecordingView{ID: r.ID, Started: r.Started.Format(time.RFC3339), App: r.App, WholeDevice: r.FarSide == meeting.FarSideWholeDevice, Paused: r.Paused}
	}
	ms, err := mgr.Meetings()
	if err != nil {
		return nil, err
	}
	for _, mt := range ms {
		mv := MeetingView{
			ID: mt.ID, Title: mt.Title, App: mt.App, WholeDevice: mt.FarSide == meeting.FarSideWholeDevice,
			Started: mt.Started.Format(time.RFC3339), State: mt.State, Error: mt.Error, Progress: -1,
			HasAudio: mgr.HasAudio(mt),
		}
		if !mt.Ended.IsZero() {
			mv.DurationSeconds = int64(mt.Ended.Sub(mt.Started).Seconds())
		}
		if p, ok := status.Transcribing[mt.ID]; ok {
			mv.Progress = p
		}
		if mt.Transcript != "" {
			mv.TranscriptName = filepath.Base(mt.Transcript)
			_, err := os.Stat(mt.Transcript)
			mv.HasTranscript = err == nil
		}
		if mv.HasAudio && !mt.AudioUntil.IsZero() {
			mv.AudioUntil = mt.AudioUntil.Format(time.RFC3339)
		}
		v.Meetings = append(v.Meetings, mv)
	}
	sort.SliceStable(v.Meetings, func(i, j int) bool { return v.Meetings[i].Started > v.Meetings[j].Started })
	return v, nil
}

// TurnOn turns Meeting notes on after the native dialog says what that
// means, then downloads and measures the model in the background.
func (m *MeetingAPI) TurnOn() (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if err := m.app.askConsent("Turn on Meeting notes?", "Meeting notes records your meetings on this computer and transcribes them here. "+strings.Join(meetingWarnings, " ")); err != nil {
		return nil, err
	}
	if err := settings.SetMeetingNotesOn(true); err != nil {
		return nil, err
	}
	mgr.Start(m.app.ctxOrBackground())
	mgr.Prepare(m.app.ctxOrBackground())
	return m.Status()
}

// TurnOff turns Meeting notes off. A recording in progress is stopped
// first and kept; it is transcribed when the feature is on again.
func (m *MeetingAPI) TurnOff() (*MeetingNotesView, error) {
	if mgr := m.manager(); mgr != nil && mgr.Status().Recording != nil {
		if err := mgr.Stop(); err != nil {
			return nil, err
		}
	}
	if err := settings.SetMeetingNotesOn(false); err != nil {
		return nil, err
	}
	return m.Status()
}

// Prepare downloads and measures the model again, after a failure.
func (m *MeetingAPI) Prepare() (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if !meetingSettings().On {
		return nil, meeting.ErrOff
	}
	mgr.Prepare(m.app.ctxOrBackground())
	return m.Status()
}

// Record starts recording a meeting, after the native dialog has asked:
// the page asking is not the person pressing Record.
func (m *MeetingAPI) Record() (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if !meetingSettings().On {
		return nil, meeting.ErrOff
	}
	if err := m.app.askConsent("Record this meeting?", "Agentry records your microphone and the meeting app's sound until you press Stop or the meeting app stops. "+meeting.ConsentSentence); err != nil {
		return nil, err
	}
	if _, err := mgr.Record(m.app.ctxOrBackground()); err != nil {
		return nil, err
	}
	return m.Status()
}

// Pause stops capturing until Resume; the transcript marks the gap.
func (m *MeetingAPI) Pause() (*MeetingNotesView, error) { return m.setPaused(true) }

// Resume captures again after Pause.
func (m *MeetingAPI) Resume() (*MeetingNotesView, error) { return m.setPaused(false) }

func (m *MeetingAPI) setPaused(p bool) (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if err := mgr.SetPaused(p); err != nil {
		return nil, err
	}
	return m.Status()
}

// Stop ends the recording and starts its transcription.
func (m *MeetingAPI) Stop() (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if err := mgr.Stop(); err != nil {
		return nil, err
	}
	return m.Status()
}

// Transcribe tries a failed or waiting meeting's transcription again.
func (m *MeetingAPI) Transcribe(id string) (*MeetingNotesView, error) {
	mgr, err := m.available()
	if err != nil {
		return nil, err
	}
	if err := mgr.Retry(id); err != nil {
		return nil, err
	}
	return m.Status()
}

// DeleteAudio deletes a meeting's kept audio now.
func (m *MeetingAPI) DeleteAudio(id string) (*MeetingNotesView, error) {
	mgr, err := m.built()
	if err != nil {
		return nil, err
	}
	if err := mgr.DeleteAudio(id); err != nil {
		return nil, err
	}
	return m.Status()
}

// Forget takes a meeting off the list, with any audio it keeps; its
// transcript stays in the output folder.
func (m *MeetingAPI) Forget(id string) (*MeetingNotesView, error) {
	mgr, err := m.built()
	if err != nil {
		return nil, err
	}
	if err := mgr.Forget(id); err != nil {
		return nil, err
	}
	return m.Status()
}

// transcriptOf is a finished meeting's transcript, by the meeting's id:
// the page names a meeting, never a path.
func (m *MeetingAPI) transcriptOf(id string) (string, error) {
	mgr, err := m.built()
	if err != nil {
		return "", err
	}
	ms, err := mgr.Meetings()
	if err != nil {
		return "", err
	}
	for _, mt := range ms {
		if mt.ID == id && mt.Transcript != "" {
			if _, err := os.Stat(mt.Transcript); err != nil {
				return "", fmt.Errorf("the transcript %s is no longer there", filepath.Base(mt.Transcript))
			}
			return mt.Transcript, nil
		}
	}
	return "", fmt.Errorf("no transcript for that meeting")
}

// OpenTranscript opens a meeting's transcript in the system's program for
// Markdown.
func (m *MeetingAPI) OpenTranscript(id string) error {
	p, err := m.transcriptOf(id)
	if err != nil {
		return err
	}
	return openWithSystemViewer(p)
}

// ShowTranscript shows a meeting's transcript in the file manager.
func (m *MeetingAPI) ShowTranscript(id string) error {
	p, err := m.transcriptOf(id)
	if err != nil {
		return err
	}
	return revealFileInFileManager(p)
}

// OpenOutputFolder shows the transcripts' folder, making it first.
func (m *MeetingAPI) OpenOutputFolder() error {
	dir := meetingSettings().OutputDir
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not a folder this app can show", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return openFolderInFileManager(dir)
}

// saveMeetingNotes changes Meeting notes' settings, never its switch.
func saveMeetingNotes(change func(*settings.MeetingNotes) error) error {
	st, err := settings.Load()
	if err != nil {
		return err
	}
	if err := change(&st.MeetingNotes); err != nil {
		return err
	}
	return settings.Save(st)
}

// SetProfile picks the language profile; a model chosen for another
// profile goes back to automatic.
func (m *MeetingAPI) SetProfile(id string) (*MeetingNotesView, error) {
	mgr, err := m.built()
	if err != nil {
		return nil, err
	}
	if mgr.Catalogue.Profile(id).ID != id {
		return nil, fmt.Errorf("no language profile %q", id)
	}
	err = saveMeetingNotes(func(mn *settings.MeetingNotes) error {
		mn.Profile = id
		if mn.Model != "" && !mgr.Catalogue.Allowed(id, mn.Model) {
			mn.Model = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	m.prepareIfOn(mgr)
	return m.Status()
}

// SetModel picks the model, "" for automatic.
func (m *MeetingAPI) SetModel(id string) (*MeetingNotesView, error) {
	mgr, err := m.built()
	if err != nil {
		return nil, err
	}
	err = saveMeetingNotes(func(mn *settings.MeetingNotes) error {
		if id != "" && !mgr.Catalogue.Allowed(mgr.Catalogue.Profile(mn.Profile).ID, id) {
			return fmt.Errorf("the language profile does not offer the model %q", id)
		}
		mn.Model = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	m.prepareIfOn(mgr)
	return m.Status()
}

// prepareIfOn downloads what a changed profile or model needs.
func (m *MeetingAPI) prepareIfOn(mgr *meeting.Manager) {
	if m.unavailable == "" && meetingSettings().On {
		mgr.Prepare(m.app.ctxOrBackground())
	}
}

// SetKeepAudio keeps recordings for seven days, or deletes kept audio at
// the next sweep when turned off.
func (m *MeetingAPI) SetKeepAudio(on bool) (*MeetingNotesView, error) {
	if err := saveMeetingNotes(func(mn *settings.MeetingNotes) error { mn.KeepAudio = on; return nil }); err != nil {
		return nil, err
	}
	return m.Status()
}

// ChooseOutputDir opens the native picker for the transcripts' folder and
// keeps what it returned; the binding takes no path, so the folder is the
// picker's and never the page's.
func (m *MeetingAPI) ChooseOutputDir() (*MeetingNotesView, error) {
	dir, err := m.app.pickDirectoryUnmarked("Choose the folder for meeting transcripts")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		if err := saveMeetingNotes(func(mn *settings.MeetingNotes) error { mn.OutputDir = dir; return nil }); err != nil {
			return nil, err
		}
	}
	return m.Status()
}

// ResetOutputDir puts the transcripts back in the default folder.
func (m *MeetingAPI) ResetOutputDir() (*MeetingNotesView, error) {
	if err := saveMeetingNotes(func(mn *settings.MeetingNotes) error { mn.OutputDir = ""; return nil }); err != nil {
		return nil, err
	}
	return m.Status()
}
