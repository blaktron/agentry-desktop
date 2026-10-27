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
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/meeting"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/tray"
)

// countingRecorder counts the times it was asked to record, and records
// nothing.
type countingRecorder struct {
	mu    sync.Mutex
	calls int
	last  meeting.Call
	// listed counts the times the microphones were listed.
	listed int
}

func (c *countingRecorder) Record(_ context.Context, call meeting.Call, _ meeting.Tracks) (meeting.Capture, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.last = call
	far := meeting.FarSideWholeDevice
	if call.Room {
		far = meeting.FarSideNone
	}
	return &idleCapture{ended: make(chan struct{}), far: far}, nil
}

// Microphones lists two microphones, as an audio system with a laptop's
// own and a USB speakerphone would.
func (c *countingRecorder) Microphones() ([]meeting.Microphone, error) {
	c.mu.Lock()
	c.listed++
	c.mu.Unlock()
	return []meeting.Microphone{{ID: "alsa_input.pci", Name: "Built-in Audio"}, {ID: "alsa_input.usb", Name: "USB Speakerphone"}}, nil
}

func (c *countingRecorder) lastCall() meeting.Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

func (c *countingRecorder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type idleCapture struct {
	ended chan struct{}
	far   meeting.FarSide
}

func (c *idleCapture) Call() meeting.Call       { return meeting.Call{} }
func (c *idleCapture) FarSide() meeting.FarSide { return c.far }
func (c *idleCapture) Ended() <-chan struct{}   { return c.ended }
func (c *idleCapture) SetPaused(bool) error     { return nil }
func (c *idleCapture) Stop() error              { return nil }

type refuseTransport struct{}

func (refuseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network in this test")
}

// oneCall is a detector that reports one call, once, when a test says.
type oneCall struct{ calls chan meeting.CallEvent }

func (d *oneCall) Watch(ctx context.Context, events chan<- meeting.CallEvent) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-d.calls:
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

type meetingApp struct {
	api      *MeetingAPI
	rec      *countingRecorder
	detector *oneCall
	asked    []string
	answer   bool
	mu       sync.Mutex
	notices  []notify.Notice
	picked   string
}

func (ma *meetingApp) shown() []notify.Notice {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return append([]notify.Notice(nil), ma.notices...)
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
	ma := &meetingApp{rec: &countingRecorder{}, detector: &oneCall{calls: make(chan meeting.CallEvent)}}
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
		newDetector: func() meeting.Detector { return ma.detector },
		notifyShow: func(n notify.Notice) error {
			ma.mu.Lock()
			defer ma.mu.Unlock()
			ma.notices = append(ma.notices, n)
			return nil
		},
		notifyWithdraw: func(string) error { return nil },
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
		if !strings.Contains(strings.Join(v.Warnings, " "), want) {
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
	if len(ma.asked) != 1 || !strings.Contains(ma.asked[0], strings.Join(v.Warnings, " ")) {
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
	if n := ma.shown(); len(n) != 1 || n[0].Key != meetingNoticeKey || !strings.Contains(n[0].Body, "everything your computer plays") {
		t.Fatalf("notices %+v, want one saying what is recorded", n)
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
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"..", "../..", outside, "a/../.."} {
		if _, err := ma.api.Forget(id); err == nil {
			t.Fatalf("Forget(%q) succeeded", id)
		}
		if _, err := ma.api.DeleteAudio(id); err == nil {
			t.Fatalf("DeleteAudio(%q) succeeded", id)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("a folder outside the meetings went: %v", err)
	}
	if err := ma.api.OpenTranscript("../../etc/passwd"); err == nil {
		t.Fatal("OpenTranscript took a path")
	}
	if err := ma.api.ShowTranscript("20261024T140000Z"); err == nil {
		t.Fatal("ShowTranscript of an unknown meeting succeeded")
	}
}

var zoomCall = meeting.Call{App: "Zoom Meeting", Binary: "zoom", PID: "4242"}

// turnedOnWithACall is Meeting notes turned on with the switch's dialog,
// and zoomCall reported by the detector and offered.
func turnedOnWithACall(t *testing.T) (*meetingApp, string) {
	t.Helper()
	ma := newMeetingApp(t)
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	ma.asked = nil
	ma.detector.calls <- meeting.CallEvent{Call: zoomCall}
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, err := ma.api.Status()
		if err != nil {
			t.Fatal(err)
		}
		if v.Offer != nil {
			if !v.Detects || v.Offer.App != "Zoom Meeting" {
				t.Fatalf("status %+v", v)
			}
			return ma, v.Offer.ID
		}
		if time.Now().After(deadline) {
			t.Fatal("the detected call was not offered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheOfferIsRecordedFromThePageOnlyOnTheDialogsYes(t *testing.T) {
	ma, id := turnedOnWithACall(t)
	if ma.rec.count() != 0 {
		t.Fatal("a detected call recorded before anyone said so")
	}
	offer := ma.shown()[len(ma.shown())-1]
	if len(offer.Actions) != 2 || offer.Actions[0].ID != meeting.ActionRecord+":"+id || offer.Key != meetingNoticeKey || !offer.Resident {
		t.Fatalf("the offer's notification %+v", offer)
	}
	ma.answer = false
	if _, err := ma.api.RecordOffer(id); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("RecordOffer answered No: %v", err)
	}
	if ma.rec.count() != 0 || len(ma.asked) != 1 || !strings.Contains(ma.asked[0], "Zoom Meeting") || !strings.Contains(ma.asked[0], "consent") {
		t.Fatalf("after a No: recorder %d, asked %q", ma.rec.count(), ma.asked)
	}
	if _, err := ma.api.RecordOffer("another"); !errors.Is(err, meeting.ErrNoOffer) {
		t.Fatalf("RecordOffer of an offer that is not standing: %v", err)
	}
	ma.answer = true
	v, err := ma.api.RecordOffer(id)
	if err != nil {
		t.Fatal(err)
	}
	if ma.rec.count() != 1 || v.Recording == nil || v.Recording.App != "Zoom Meeting" || v.Offer != nil {
		t.Fatalf("after Yes: recorder %d, view %+v", ma.rec.count(), v)
	}
}

// The offer's own Record button is the person's press, reported by the
// notification server: it records without another dialog.
func TestTheOffersButtonRecordsWithoutAnotherDialog(t *testing.T) {
	ma, id := turnedOnWithACall(t)
	ma.answer = false
	ma.api.handleAction(meeting.ActionRecord + ":" + id)
	if ma.rec.count() != 1 || len(ma.asked) != 0 {
		t.Fatalf("Record on the notification: recorder %d, asked %q", ma.rec.count(), ma.asked)
	}
	ma.api.handleAction(meeting.ActionStop)
	if v, _ := ma.api.Status(); v.Recording != nil {
		t.Fatal("Stop on the notification left the recording running")
	}
}

func TestNotNowRecordsNothing(t *testing.T) {
	ma, id := turnedOnWithACall(t)
	v, err := ma.api.NotNow(id)
	if err != nil || v.Offer != nil || ma.rec.count() != 0 || len(ma.asked) != 0 {
		t.Fatalf("NotNow: %v, offer %+v, recorder %d, asked %q", err, v.Offer, ma.rec.count(), ma.asked)
	}
}

func TestAddingAnAppToTheAutomaticListAsks(t *testing.T) {
	ma, _ := turnedOnWithACall(t)
	v, _ := ma.api.Status()
	if len(v.Apps) != 1 || v.Apps[0].Key != "zoom" || v.Apps[0].Automatic {
		t.Fatalf("apps %+v, want zoom asking", v.Apps)
	}
	if _, err := ma.api.SetAutomatic("teams", true); err == nil {
		t.Fatal("an app never seen here was put on the automatic list")
	}
	ma.answer = false
	if _, err := ma.api.SetAutomatic("zoom", true); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("SetAutomatic answered No: %v", err)
	}
	if st, _ := settings.Load(); len(st.MeetingNotes.Automatic) != 0 {
		t.Fatal("a No put the app on the automatic list")
	}
	if len(ma.asked) != 1 || !strings.Contains(ma.asked[0], "without asking") || !strings.Contains(ma.asked[0], "consent") {
		t.Fatalf("the dialog asked %q", ma.asked)
	}
	ma.answer = true
	if v, err := ma.api.SetAutomatic("zoom", true); err != nil || !v.Apps[0].Automatic {
		t.Fatalf("SetAutomatic answered Yes: %v %+v", err, v.Apps)
	}
	ma.asked, ma.answer = nil, false
	if v, err := ma.api.SetAutomatic("zoom", false); err != nil || v.Apps[0].Automatic || len(ma.asked) != 0 {
		t.Fatalf("taking an app off asks nothing: %v %+v %q", err, v.Apps, ma.asked)
	}
	if !browserApp("chrome") || !browserApp("firefox-bin") || browserApp("zoom") {
		t.Fatal("browserApp misreads an app")
	}
}

func TestMeetingNotesKeepsTheAppRunningAndOffersOnlyItsLengths(t *testing.T) {
	ma := newMeetingApp(t)
	if ma.api.keepsRunning() {
		t.Fatal("keeps running with the switch off")
	}
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	if !ma.api.keepsRunning() {
		t.Fatal("does not keep running with the switch on")
	}
	if _, err := ma.api.SetMaxHours(5); err == nil {
		t.Fatal("a length the page does not offer was kept")
	}
	if v, err := ma.api.SetMaxHours(2); err != nil || v.MaxHours != 2 || meetingSettings().MaxLength != 2*time.Hour {
		t.Fatalf("SetMaxHours(2): %v %+v", err, v)
	}
	if maxHours(0) != 4 || maxHours(7) != 4 {
		t.Fatal("an unset or unknown length is not four hours")
	}
}

// With Meeting notes on, the window's close hides the window and the tray
// starts, so calls are noticed with it closed; the tray shows a recording
// as it starts, pauses and ends.
func TestMeetingNotesRunsInTheBackgroundAndTheTrayShowsTheRecording(t *testing.T) {
	r := newBGRig(t)
	ma := newMeetingApp(t)
	r.s.meeting = ma.api
	var mu sync.Mutex
	var shown []tray.Meeting
	saved := trayMeeting
	trayMeeting = func(m tray.Meeting) { mu.Lock(); shown = append(shown, m); mu.Unlock() }
	t.Cleanup(func() { trayMeeting = saved })
	last := func() tray.Meeting {
		mu.Lock()
		defer mu.Unlock()
		if len(shown) == 0 {
			return tray.Meeting{}
		}
		return shown[len(shown)-1]
	}
	ma.api.setHooks(func() { r.s.followAgents(context.Background()) }, func(*meeting.RecordingStatus) { r.s.followAgents(context.Background()) })

	if prevent := r.s.BeforeClose(context.Background()); prevent || r.hidden != 0 {
		t.Fatalf("with Meeting notes off the close hid the window: prevent=%v hidden=%d", prevent, r.hidden)
	}
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	if r.trays == 0 {
		t.Fatal("turning Meeting notes on started no tray")
	}
	if prevent := r.s.BeforeClose(context.Background()); !prevent || r.hidden != 1 {
		t.Fatalf("with Meeting notes on: prevent=%v hidden=%d", prevent, r.hidden)
	}
	if _, err := ma.api.Record(); err != nil {
		t.Fatal(err)
	}
	if m := last(); !m.Recording || m.What != "everything this computer plays" {
		t.Fatalf("the tray shows %+v while recording", m)
	}
	if _, err := ma.api.Pause(); err != nil {
		t.Fatal(err)
	}
	if m := last(); !m.Paused {
		t.Fatalf("the tray shows %+v while paused", m)
	}
	if _, err := ma.api.Stop(); err != nil {
		t.Fatal(err)
	}
	if m := last(); m.Recording {
		t.Fatalf("the tray shows %+v after Stop", m)
	}
}

func TestOpenPrivacySettingsOpensOnlyTheNamedPanes(t *testing.T) {
	m := &MeetingAPI{}
	for _, which := range []string{"", "x-apple.systempreferences:com.apple.preference.security", "file:///Applications", "camera"} {
		if err := m.OpenPrivacySettings(which); err == nil {
			t.Errorf("OpenPrivacySettings(%q) opened something", which)
		}
	}
	for _, which := range []string{meeting.SettingsMicrophone, meeting.SettingsAudio, meeting.SettingsPrivacy} {
		if !strings.HasPrefix(meetingPrivacyPanes[which], "x-apple.systempreferences:com.apple.preference.security?Privacy") {
			t.Errorf("the pane for %q is %q", which, meetingPrivacyPanes[which])
		}
	}
}

func TestTheGraphicsCardIsChosenOnlyWhereOneRanAndDecliningKeepsTheModel(t *testing.T) {
	ma := newMeetingApp(t)
	if _, err := ma.api.UseGPU(true); err == nil {
		t.Fatal("the graphics card was chosen where none was measured")
	}
	if st, _ := settings.Load(); st.MeetingNotes.Hardware != "" {
		t.Fatalf("a refused choice was saved: %q", st.MeetingNotes.Hardware)
	}
	if _, err := ma.api.SetModel("base.en-q5_1"); err != nil {
		t.Fatal(err)
	}
	v, err := ma.api.UseGPU(false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := settings.Load()
	if st.MeetingNotes.Hardware != "cpu" || st.MeetingNotes.Model != "base.en-q5_1" || v.ModelChoice != "base.en-q5_1" {
		t.Fatalf("declined: hardware %q, model %q", st.MeetingNotes.Hardware, st.MeetingNotes.Model)
	}
	if v.GPU != nil || !v.ChecksGPU {
		t.Fatalf("no card measured: gpu %+v, checks %v", v.GPU, v.ChecksGPU)
	}
}

// Record in the room asks the native dialog first, then records the
// microphone chosen and nothing else; a No records nothing.
func TestRecordInTheRoomAsksThenRecordsTheChosenMicrophone(t *testing.T) {
	ma := newMeetingApp(t)
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	v, err := ma.api.SetMic("alsa_input.usb")
	if err != nil {
		t.Fatal(err)
	}
	if !v.RecordsRooms || v.Mic != "alsa_input.usb" || len(v.Microphones) != 2 {
		t.Fatalf("view: records rooms %v, mic %q, microphones %+v", v.RecordsRooms, v.Mic, v.Microphones)
	}
	ma.answer = false
	if _, err := ma.api.RecordRoom(); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("RecordRoom answered No: %v", err)
	}
	if n := ma.rec.count(); n != 0 {
		t.Fatalf("a No recorded %d times", n)
	}
	ma.answer = true
	v, err = ma.api.RecordRoom()
	if err != nil {
		t.Fatal(err)
	}
	asked := ma.asked[len(ma.asked)-1]
	if !strings.HasPrefix(asked, "Record a meeting in the room?") || !strings.Contains(asked, "nothing your computer plays") || !strings.Contains(asked, meeting.ConsentSentence) {
		t.Fatalf("the dialog said %q", asked)
	}
	if got := ma.rec.lastCall(); !got.Room || got.Mic != "alsa_input.usb" {
		t.Fatalf("the recorder was asked for %+v", got)
	}
	if v.Recording == nil || !v.Recording.InRoom {
		t.Fatalf("recording %+v, want one in the room", v.Recording)
	}
	if _, err := ma.api.Stop(); err != nil {
		t.Fatal(err)
	}
	v, _ = ma.api.Status()
	if len(v.Meetings) != 1 || !v.Meetings[0].InRoom {
		t.Fatalf("meetings %+v, want one in the room", v.Meetings)
	}
}

// Only a microphone the audio system lists, or the default, can be chosen:
// a page that names anything else (an output's monitor, say) is refused.
func TestOnlyAListedMicrophoneCanBeChosen(t *testing.T) {
	ma := newMeetingApp(t)
	// Off, the audio system is not asked even for its microphones.
	if v, err := ma.api.Status(); err != nil || len(v.Microphones) != 0 {
		t.Fatalf("status with the switch off: %v, %+v", err, v)
	}
	if _, err := ma.api.SetMic("alsa_input.pci"); !errors.Is(err, meeting.ErrOff) {
		t.Fatalf("SetMic with the switch off: %v, want ErrOff", err)
	}
	ma.rec.mu.Lock()
	listed := ma.rec.listed
	ma.rec.mu.Unlock()
	if listed != 0 {
		t.Fatalf("the microphones were listed %d times with the switch off", listed)
	}
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"alsa_output.pci.monitor", "nothing"} {
		if _, err := ma.api.SetMic(id); err == nil {
			t.Fatalf("SetMic(%q) was taken", id)
		}
	}
	if st, _ := settings.Load(); st.MeetingNotes.Mic != "" {
		t.Fatalf("a refused microphone was kept: %q", st.MeetingNotes.Mic)
	}
	if _, err := ma.api.SetMic("alsa_input.pci"); err != nil {
		t.Fatal(err)
	}
	if v, err := ma.api.SetMic(""); err != nil || v.Mic != "" {
		t.Fatalf("back to the default: %v, %+v", err, v)
	}
}

// The tray offers Record a meeting in the room while Meeting notes is on and
// nothing is recorded, and names a room it records.
func TestTheTrayOffersARoomAndShowsOne(t *testing.T) {
	r := newBGRig(t)
	ma := newMeetingApp(t)
	r.s.meeting = ma.api
	var mu sync.Mutex
	var shown []tray.Meeting
	saved := trayMeeting
	trayMeeting = func(m tray.Meeting) { mu.Lock(); shown = append(shown, m); mu.Unlock() }
	t.Cleanup(func() { trayMeeting = saved })
	last := func() tray.Meeting {
		mu.Lock()
		defer mu.Unlock()
		if len(shown) == 0 {
			return tray.Meeting{}
		}
		return shown[len(shown)-1]
	}
	ma.api.setHooks(func() { r.s.followAgents(context.Background()) }, func(*meeting.RecordingStatus) { r.s.followAgents(context.Background()) })
	r.s.followAgents(context.Background())
	if last().CanRecordRoom {
		t.Fatal("the tray offers a room with Meeting notes off")
	}
	ma.answer = true
	if _, err := ma.api.TurnOn(); err != nil {
		t.Fatal(err)
	}
	if m := last(); !m.CanRecordRoom || m.Recording {
		t.Fatalf("the tray shows %+v with Meeting notes on and idle", m)
	}
	if _, err := ma.api.RecordRoom(); err != nil {
		t.Fatal(err)
	}
	if m := last(); !m.Recording || m.CanRecordRoom || m.What != "a meeting in the room" {
		t.Fatalf("the tray shows %+v while a room is recorded", m)
	}
}
