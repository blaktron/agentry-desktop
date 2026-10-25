package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/meeting"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// countingRecorder counts the times it was asked to record, and records
// nothing.
type countingRecorder struct {
	mu    sync.Mutex
	calls int
}

func (c *countingRecorder) Record(context.Context, meeting.Call, meeting.Tracks) (meeting.Capture, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &idleCapture{ended: make(chan struct{})}, nil
}

func (c *countingRecorder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type idleCapture struct{ ended chan struct{} }

func (c *idleCapture) Call() meeting.Call       { return meeting.Call{} }
func (c *idleCapture) FarSide() meeting.FarSide { return meeting.FarSideWholeDevice }
func (c *idleCapture) Ended() <-chan struct{}   { return c.ended }
func (c *idleCapture) SetPaused(bool) error     { return nil }
func (c *idleCapture) Stop() error              { return nil }

type refuseTransport struct{}

func (refuseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network in this test")
}

type meetingApp struct {
	api     *MeetingAPI
	rec     *countingRecorder
	asked   []string
	answer  bool
	notices []notify.Notice
	picked  string
}

func newMeetingApp(t *testing.T) *meetingApp {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Meeting notes records on Linux in this version")
	}
	t.Setenv(configdir.EnvVar, t.TempDir())
	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })
	sidecar := filepath.Join(t.TempDir(), "whisper-cli")
	if err := os.WriteFile(sidecar, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil { //nolint:gosec // a stand-in that is never run here
		t.Fatal(err)
	}
	ma := &meetingApp{rec: &countingRecorder{}}
	app := &App{
		ctx: context.Background(),
		confirm: func(_ context.Context, title, message string) (bool, error) {
			ma.asked = append(ma.asked, title+"\n"+message)
			return ma.answer, nil
		},
		pickDirectory: func(context.Context, string) (string, error) { return ma.picked, nil },
	}
	ma.api = &MeetingAPI{
		app: app, sidecar: sidecar, root: t.TempDir(),
		// No test reaches the network for a model.
		client:      &http.Client{Transport: refuseTransport{}},
		newRecorder: func() meeting.Recorder { return ma.rec },
		notifyShow:  func(n notify.Notice) error { ma.notices = append(ma.notices, n); return nil },
	}
	t.Cleanup(ma.api.shutdown)
	return ma
}

func TestMeetingNotesIsOffUntilTheDialogSaysYes(t *testing.T) {
	ma := newMeetingApp(t)
	v, err := ma.api.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !v.Available || v.On {
		t.Fatalf("fresh status: available %v (%s), on %v", v.Available, v.Unavailable, v.On)
	}
	for _, want := range []string{"consent", "muted yourself in the meeting app", "everything your computer plays"} {
		if !strings.Contains(v.Warning, want) {
			t.Errorf("the turn-on warning lacks %q", want)
		}
	}
	ma.answer = false
	if _, err := ma.api.TurnOn(); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("TurnOn answered No: %v", err)
	}
	if st, _ := settings.Load(); st.MeetingNotes.On {
		t.Fatal("a No turned Meeting notes on")
	}
	if len(ma.asked) != 1 || !strings.Contains(ma.asked[0], v.Warning) {
		t.Fatalf("the dialog asked %q, want the warning", ma.asked)
	}
	ma.answer = true
	if v, err = ma.api.TurnOn(); err != nil || !v.On {
		t.Fatalf("TurnOn answered Yes: %v %+v", err, v)
	}
}

func TestNothingRecordsWithoutTheSwitchAndTheDialog(t *testing.T) {
	ma := newMeetingApp(t)
	ma.answer = true
	if _, err := ma.api.Record(); !errors.Is(err, meeting.ErrOff) {
		t.Fatalf("Record with the switch off: %v", err)
	}
	if len(ma.asked) != 0 || ma.rec.count() != 0 {
		t.Fatalf("with the switch off: asked %q, recorder asked %d times", ma.asked, ma.rec.count())
	}
	if err := settings.SetMeetingNotesOn(true); err != nil {
		t.Fatal(err)
	}
	ma.answer = false
	if _, err := ma.api.Record(); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("Record answered No: %v", err)
	}
	if ma.rec.count() != 0 {
		t.Fatal("a No still recorded")
	}
	ma.answer = true
	v, err := ma.api.Record()
	if err != nil {
		t.Fatal(err)
	}
	if ma.rec.count() != 1 || v.Recording == nil || !v.Recording.WholeDevice {
		t.Fatalf("Record answered Yes: recorder %d, view %+v", ma.rec.count(), v.Recording)
	}
	if !strings.Contains(ma.asked[len(ma.asked)-1], "consent") {
		t.Fatalf("the Record dialog lacks the consent sentence: %q", ma.asked)
	}
	if len(ma.notices) != 1 || ma.notices[0].Key != meetingNoticeKey || !strings.Contains(ma.notices[0].Body, "everything your computer plays") {
		t.Fatalf("notices %+v, want one saying what is recorded", ma.notices)
	}
	if v, err = ma.api.TurnOff(); err != nil || v.Recording != nil || v.On {
		t.Fatalf("TurnOff: %v %+v", err, v)
	}
}

func TestTheCopyWithoutATranscriberDoesNotOfferMeetingNotes(t *testing.T) {
	ma := newMeetingApp(t)
	ma.api.sidecar = filepath.Join(t.TempDir(), "absent")
	v, err := ma.api.Status()
	if err != nil {
		t.Fatal(err)
	}
	if v.Available || !strings.Contains(v.Unavailable, "does not carry the transcriber") {
		t.Fatalf("status %+v", v)
	}
	ma.answer = true
	if _, err := ma.api.TurnOn(); err == nil {
		t.Fatal("turned on with no transcriber")
	}
}

func TestMeetingSettingsTakeOnlyWhatTheCatalogueAndThePickerGive(t *testing.T) {
	ma := newMeetingApp(t)
	if _, err := ma.api.SetModel("ggml-evil"); err == nil {
		t.Fatal("a model the profile does not offer was kept")
	}
	if _, err := ma.api.SetProfile("klingon"); err == nil {
		t.Fatal("an unknown profile was kept")
	}
	if v, err := ma.api.SetModel("base.en-q5_1"); err != nil || v.ModelChoice != "base.en-q5_1" || v.Model != "base.en-q5_1" {
		t.Fatalf("SetModel: %v %+v", err, v)
	}
	// Another profile does not offer that model: it goes back to automatic.
	if v, err := ma.api.SetProfile("multilingual"); err != nil || v.ModelChoice != "" || v.Model != "small-q5_1" {
		t.Fatalf("SetProfile: %v choice %q model %q", err, v.ModelChoice, v.Model)
	}
	v, _ := ma.api.Status()
	if v.OutputDirChosen || !strings.HasSuffix(v.OutputDir, filepath.Join("Agentry", MeetingOutputFolder)) {
		t.Fatalf("default output folder %q", v.OutputDir)
	}
	ma.picked = filepath.Join(t.TempDir(), "notes")
	if v, err := ma.api.ChooseOutputDir(); err != nil || v.OutputDir != ma.picked || !v.OutputDirChosen {
		t.Fatalf("ChooseOutputDir: %v %+v", err, v)
	}
	ma.picked = ""
	if v, err := ma.api.ChooseOutputDir(); err != nil || v.OutputDir == "" {
		t.Fatalf("a cancelled picker changed the folder: %v %+v", err, v)
	}
	if v, err := ma.api.SetKeepAudio(true); err != nil || !v.KeepAudio {
		t.Fatalf("SetKeepAudio: %v", err)
	}
	if st, _ := settings.Load(); st.MeetingNotes.On {
		t.Fatal("a settings change turned the feature on")
	}
}

func TestATranscriptIsOpenedByMeetingNeverByPath(t *testing.T) {
	ma := newMeetingApp(t)
	if err := ma.api.OpenTranscript("../../etc/passwd"); err == nil {
		t.Fatal("OpenTranscript took a path")
	}
	if err := ma.api.ShowTranscript("20261024T140000Z"); err == nil {
		t.Fatal("ShowTranscript of an unknown meeting succeeded")
	}
}
