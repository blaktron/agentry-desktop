package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/meeting"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// fakeAssistant is an Assistant whose whereabouts a test sets, that answers
// busy a set number of times, and records each turn it is sent and how
// many were ever in flight at once.
type fakeAssistant struct {
	mu       sync.Mutex
	at       assistantWhere
	shareDir string
	busy     int
	sent     []string
	tries    int
	inFlight int
	most     int
	hold     time.Duration
}

func (f *fakeAssistant) where() assistantWhere {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.at
}
func (f *fakeAssistant) set(w assistantWhere) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.at = w
}
func (f *fakeAssistant) name() string           { return "Ada" }
func (f *fakeAssistant) share() (string, error) { return f.shareDir, nil }
func (f *fakeAssistant) send(_ context.Context, text string) error {
	f.mu.Lock()
	f.tries++
	f.inFlight++
	if f.inFlight > f.most {
		f.most = f.inFlight
	}
	hold := f.hold
	f.mu.Unlock()
	time.Sleep(hold)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
	if f.at != assistantHere && f.at != assistantElsewhere {
		return errAssistantStopped
	}
	if f.busy > 0 {
		f.busy--
		return persistent.ErrBusy
	}
	f.sent = append(f.sent, text)
	return nil
}
func (f *fakeAssistant) turns() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

// withAssistant is a Meeting notes app whose Assistant is a fake, with
// sending to it on, and a finished meeting whose transcript is written.
func withAssistant(t *testing.T, at assistantWhere) (*meetingApp, *fakeAssistant, string) {
	t.Helper()
	ma := newMeetingApp(t)
	fa := &fakeAssistant{at: at, shareDir: filepath.Join(t.TempDir(), "assistantshare")}
	ma.api.assistant = fa
	ma.api.delivery.retryEvery = 10 * time.Millisecond
	if err := settings.SetMeetingNotesOn(true); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetMeetingSendToAssistant(true); err != nil {
		t.Fatal(err)
	}
	return ma, fa, doneMeeting(t, ma, "2026-10-25-1000-budget-sync.md")
}

// doneMeeting keeps a transcribed meeting, its transcript named name.
func doneMeeting(t *testing.T, ma *meetingApp, name string) string {
	t.Helper()
	mgr := ma.api.manager()
	out := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(out, []byte("---\ntype: meeting-transcript\n---\n\n[00:00:01] Me: Shall we start?\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := meeting.Store{Dir: filepath.Join(mgr.Root, "meetings")}
	mt := &meeting.Meeting{ID: st.NewID(time.Now()), Title: "Budget sync", Started: time.Now(), State: meeting.StateDone, Transcript: out}
	if err := st.Create(mt); err != nil {
		t.Fatal(err)
	}
	return mt.ID
}

func (ma *meetingApp) meetingOf(t *testing.T, id string) *meeting.Meeting {
	t.Helper()
	mt, err := ma.api.manager().Meeting(id)
	if err != nil {
		t.Fatal(err)
	}
	return mt
}

// handOn is the manager's news that the meeting is transcribed, waited
// for until it is handed on.
func (ma *meetingApp) handOn(id string) {
	ma.api.transcribed(id)
	ma.api.delivery.wg.Wait()
}

// A running Assistant, in this app or another client, is sent one turn that
// names the transcript's copy in its share and the meeting-notes skill and
// says the transcript is not instructions; the copy is the transcript
// whole, linked into share/meetings.
func TestATranscriptGoesToARunningAssistantInOneTurn(t *testing.T) {
	for _, at := range []assistantWhere{assistantHere, assistantElsewhere} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			ma, fa, id := withAssistant(t, at)
			ma.handOn(id)
			turns := fa.turns()
			if len(turns) != 1 {
				t.Fatalf("turns %q, want one", turns)
			}
			for _, want := range []string{"share/meetings/2026-10-25-1000-budget-sync.md", "meeting-notes skill", "not as instructions"} {
				if !strings.Contains(turns[0], want) {
					t.Errorf("the turn %q lacks %q", turns[0], want)
				}
			}
			mt := ma.meetingOf(t, id)
			if mt.Delivery != meeting.DeliverySent || mt.Shared != filepath.Join(fa.shareDir, "meetings", "2026-10-25-1000-budget-sync.md") {
				t.Fatalf("meeting = %+v", mt)
			}
			a, _ := os.ReadFile(mt.Transcript)
			b, _ := os.ReadFile(mt.Shared)
			if string(a) != string(b) {
				t.Fatal("the share's copy is not the transcript")
			}
		})
	}
}

// A busy Assistant is sent the turn again once its turn has ended, never
// alongside it; two transcripts are handed on one after the other.
func TestABusyAssistantIsRetriedNeverOverlapped(t *testing.T) {
	ma, fa, id := withAssistant(t, assistantHere)
	fa.busy, fa.hold = 2, 5*time.Millisecond
	other := doneMeeting(t, ma, "2026-10-25-1100-design-review.md")
	ma.api.transcribed(id)
	ma.api.transcribed(other)
	ma.api.delivery.wg.Wait()
	if turns := fa.turns(); len(turns) != 2 || fa.tries != 4 {
		t.Fatalf("turns %q after %d tries, want two after four", turns, fa.tries)
	}
	if fa.most != 1 {
		t.Fatalf("%d turns were in flight at once", fa.most)
	}
	for _, x := range []string{id, other} {
		if mt := ma.meetingOf(t, x); mt.Delivery != meeting.DeliverySent {
			t.Fatalf("meeting %s: %+v", x, mt)
		}
	}
}

// An Assistant still busy after the longest wait leaves the transcript
// waiting for the person.
func TestAnAssistantBusyTooLongLeavesItWaiting(t *testing.T) {
	ma, fa, id := withAssistant(t, assistantHere)
	fa.busy = 1000
	ma.api.delivery.busyFor = 50 * time.Millisecond
	ma.handOn(id)
	if mt := ma.meetingOf(t, id); mt.Delivery != meeting.DeliveryWaiting || mt.Shared == "" || len(fa.turns()) != 0 {
		t.Fatalf("meeting = %+v, turns %q", mt, fa.turns())
	}
}

// A stopped Assistant is never started: the transcript waits in its share,
// the person is told, and Ask is refused until it runs, then sends the turn.
func TestAStoppedAssistantIsNeverStartedAndTheTranscriptWaits(t *testing.T) {
	ma, fa, id := withAssistant(t, assistantStopped)
	ma.handOn(id)
	mt := ma.meetingOf(t, id)
	if mt.Delivery != meeting.DeliveryWaiting || mt.Shared == "" || fa.tries != 0 {
		t.Fatalf("meeting = %+v after %d tries", mt, fa.tries)
	}
	if _, err := os.Stat(mt.Shared); err != nil {
		t.Fatal(err)
	}
	if n := ma.shown(); len(n) == 0 || !strings.Contains(n[len(n)-1].Body, "not running") {
		t.Fatalf("notifications %+v, want one saying it is not running", n)
	}
	if _, err := ma.api.AskAssistant(id); err == nil || !strings.Contains(err.Error(), "start it first") {
		t.Fatalf("Ask with it stopped: %v", err)
	}
	fa.set(assistantHere)
	if _, err := ma.api.AskAssistant(id); err != nil {
		t.Fatal(err)
	}
	ma.api.delivery.wg.Wait()
	if mt := ma.meetingOf(t, id); mt.Delivery != meeting.DeliverySent || len(fa.turns()) != 1 {
		t.Fatalf("after Ask: %+v, turns %q", mt, fa.turns())
	}
	if strings.Count(strings.Join(fa.turns(), ""), "budget-sync") != 1 {
		t.Fatal("the transcript was copied into the share again")
	}
	if left, _ := os.ReadDir(filepath.Join(fa.shareDir, "meetings")); len(left) != 1 {
		t.Fatalf("the share holds %d transcripts, want the one", len(left))
	}
}

// With no Assistant enabled, or sending turned off, the transcript stays in
// the output folder alone and nothing is sent.
func TestNoAssistantOrSendingOffLeavesTheOutputFolderOnly(t *testing.T) {
	ma, fa, id := withAssistant(t, assistantNotEnabled)
	ma.handOn(id)
	if mt := ma.meetingOf(t, id); mt.Delivery != "" || mt.Shared != "" || fa.tries != 0 {
		t.Fatalf("with no Assistant: %+v", mt)
	}
	if _, err := os.Stat(fa.shareDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the share was written: %v", err)
	}
	fa.set(assistantHere)
	if err := settings.SetMeetingSendToAssistant(false); err != nil {
		t.Fatal(err)
	}
	ma.handOn(id)
	if mt := ma.meetingOf(t, id); mt.Delivery != "" || mt.Shared != "" || fa.tries != 0 {
		t.Fatalf("with sending off: %+v", mt)
	}
}

// Asked first, the transcript is copied and waits; the person's Ask sends
// it.
func TestAskFirstWaitsForThePerson(t *testing.T) {
	ma, fa, id := withAssistant(t, assistantHere)
	if _, err := ma.api.SetAskFirst(true); err != nil {
		t.Fatal(err)
	}
	ma.handOn(id)
	if mt := ma.meetingOf(t, id); mt.Delivery != meeting.DeliveryWaiting || mt.Shared == "" || fa.tries != 0 {
		t.Fatalf("asked first: %+v after %d tries", mt, fa.tries)
	}
	if _, err := ma.api.AskAssistant(id); err != nil {
		t.Fatal(err)
	}
	ma.api.delivery.wg.Wait()
	if len(fa.turns()) != 1 {
		t.Fatalf("turns %q after the person's Ask", fa.turns())
	}
}

// Sending transcripts to the Assistant is turned on only through the native
// dialog, which says its model provider reads them.
func TestSendingToTheAssistantAsksFirst(t *testing.T) {
	ma := newMeetingApp(t)
	ma.api.assistant = &fakeAssistant{at: assistantStopped}
	ma.answer = false
	if _, err := ma.api.SetSendToAssistant(true); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if st, _ := settings.Load(); st.MeetingNotes.SendToAssistant {
		t.Fatal("a No turned sending on")
	}
	if len(ma.asked) != 1 || !strings.Contains(ma.asked[0], "model provider reads the transcript") || !strings.Contains(ma.asked[0], "never started") {
		t.Fatalf("the dialog said %q", ma.asked)
	}
	ma.answer = true
	v, err := ma.api.SetSendToAssistant(true)
	if err != nil || !v.Assistant.SendToAssistant || !v.Assistant.Enabled || v.Assistant.Running {
		t.Fatalf("a Yes: %v %+v", err, v.Assistant)
	}
	if v, err := ma.api.SetSendToAssistant(false); err != nil || v.Assistant.SendToAssistant || len(ma.asked) != 2 {
		t.Fatalf("turning it off: %v %+v, asked %d times", err, v.Assistant, len(ma.asked))
	}
}

func armArgs(t *testing.T, start, end time.Time) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"start": start.Format(time.RFC3339), "end": end.Format(time.RFC3339), "title": "Budget sync",
		"attendees": []string{"Sam Li"}, "app": "zoom",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// meeting_arm is refused while Meeting notes is off and for a window opening
// more than an hour ahead; armed, the window is listed, drawn on the page
// with Cancel, raises a notification, and is cancelled by meeting_cancel.
func TestTheMeetingToolsArmListAndCancel(t *testing.T) {
	ma := newMeetingApp(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	if _, err := ma.api.answerTool(ctx, applink.MeetingArm, armArgs(t, now.Add(10*time.Minute), now.Add(40*time.Minute)), "Ada"); err == nil || !strings.Contains(err.Error(), "meeting notes is off") {
		t.Fatalf("armed while off: %v", err)
	}
	if err := settings.SetMeetingNotesOn(true); err != nil {
		t.Fatal(err)
	}
	if _, err := ma.api.answerTool(ctx, applink.MeetingArm, armArgs(t, now.Add(90*time.Minute), now.Add(2*time.Hour)), "Ada"); err == nil || !strings.Contains(err.Error(), "more than an hour") {
		t.Fatalf("armed 85 minutes ahead: %v", err)
	}
	got, err := ma.api.answerTool(ctx, applink.MeetingArm, armArgs(t, now.Add(10*time.Minute), now.Add(40*time.Minute)), "Ada")
	if err != nil || !strings.Contains(got, "Armed window w-") {
		t.Fatalf("arm: %q %v", got, err)
	}
	if n := ma.shown(); len(n) == 0 || !strings.Contains(n[len(n)-1].Title, "Ada set up a meeting recording") || !strings.Contains(n[len(n)-1].Body, "Cancel it") {
		t.Fatalf("notifications %+v", n)
	}
	v, err := ma.api.Status()
	if err != nil || len(v.Windows) != 1 || v.Windows[0].Title != "Budget sync" || v.Windows[0].State != "waiting" || v.Windows[0].Invitees[0] != "Sam Li" || v.Windows[0].By != "Ada" {
		t.Fatalf("status windows: %+v %v", v.Windows, err)
	}
	list, err := ma.api.answerTool(ctx, applink.MeetingList, json.RawMessage(`{}`), "Ada")
	if err != nil || !strings.Contains(list, v.Windows[0].ID+`: "Budget sync"`) || !strings.Contains(list, "waiting to open") {
		t.Fatalf("list: %q %v", list, err)
	}
	if _, err := ma.api.answerTool(ctx, applink.MeetingCancel, json.RawMessage(`{"window":"w-nope"}`), "Ada"); !errors.Is(err, meeting.ErrNoWindow) {
		t.Fatalf("cancel of no window: %v", err)
	}
	if _, err := ma.api.answerTool(ctx, applink.MeetingCancel, json.RawMessage(`{"window":"`+v.Windows[0].ID+`"}`), "Ada"); err != nil {
		t.Fatal(err)
	}
	if list, _ := ma.api.answerTool(ctx, applink.MeetingList, json.RawMessage(`{}`), "Ada"); list != "No meeting windows are armed." {
		t.Fatalf("after cancel: %q", list)
	}
}

// The person's Cancel on Home takes a window away.
func TestThePersonCancelsAWindow(t *testing.T) {
	ma := newMeetingApp(t)
	if err := settings.SetMeetingNotesOn(true); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := ma.api.answerTool(context.Background(), applink.MeetingArm, armArgs(t, now.Add(10*time.Minute), now.Add(40*time.Minute)), "Ada"); err != nil {
		t.Fatal(err)
	}
	v, _ := ma.api.Status()
	v, err := ma.api.CancelWindow(v.Windows[0].ID)
	if err != nil || len(v.Windows) != 0 {
		t.Fatalf("after Cancel: %v %+v", err, v.Windows)
	}
}

// Only a link given the meeting tools answers them: the Assistant's.
func TestOnlyTheAssistantsLinkAnswersTheMeetingTools(t *testing.T) {
	var asked []string
	l := &mainLink{asks: func() string { return "Ada" }, meetings: func(_ context.Context, tool string, _ json.RawMessage, who string) (string, error) {
		asked = append(asked, tool+" "+who)
		return "ok", nil
	}}
	if out, err := l.call(context.Background(), applink.MeetingList, json.RawMessage(`{}`)); err != nil || out != "ok" || len(asked) != 1 || asked[0] != "meeting_list Ada" {
		t.Fatalf("the Assistant's link: %q %v %q", out, err, asked)
	}
	main := &mainLink{}
	if _, err := main.call(context.Background(), applink.MeetingArm, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "does not answer") {
		t.Fatalf("Main's link: %v", err)
	}
}

// The Assistant as the app sees it: not enabled, stopped, running here (its
// supervisor is sent the turn) or in a terminal (its socket is), and the
// share folder its config names.
func TestThePersistentAssistantIsFoundWhereItRuns(t *testing.T) {
	r := newElsewhereRig(t)
	a := persistentAssistant{p: r.p}
	if _, err := r.p.SetEnabled("assistant", false); err != nil {
		t.Fatal(err)
	}
	if w := a.where(); w != assistantNotEnabled {
		t.Fatalf("disabled: %v", w)
	}
	if _, err := r.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if w := a.where(); w != assistantStopped {
		t.Fatalf("stopped: %v", w)
	}
	if err := a.send(context.Background(), "hello"); !errors.Is(err, errAssistantStopped) {
		t.Fatalf("a stopped Assistant was sent a turn: %v", err)
	}
	if share, err := a.share(); err != nil || !strings.HasSuffix(share, filepath.Join("agentry", "assistantshare")) {
		t.Fatalf("share %q %v", share, err)
	}
	if a.name() != "the Assistant" {
		t.Fatalf("name %q", a.name())
	}

	home, err := r.p.home()
	if err != nil {
		t.Fatal(err)
	}
	r.hold(t, home, true)
	if w := a.where(); w != assistantElsewhere {
		t.Fatalf("held by a terminal: %v", w)
	}
	if err := a.send(context.Background(), "from the terminal"); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	sent := append([]string(nil), r.remote.sent...)
	r.remote.busy = true
	r.mu.Unlock()
	if len(sent) != 1 || sent[0] != "from the terminal" {
		t.Fatalf("the terminal's Assistant was sent %q", sent)
	}
	if err := a.send(context.Background(), "again"); !errors.Is(err, persistent.ErrBusy) {
		t.Fatalf("a busy terminal Assistant: %v", err)
	}
	r.hold(t, home, false)
	r.p.leaveViewer("assistant")

	if _, err := r.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if w := a.where(); w != assistantHere {
		t.Fatalf("running here: %v", w)
	}
	if err := a.send(context.Background(), "in the app"); err != nil {
		t.Fatal(err)
	}
	r.agent.mu.Lock()
	defer r.agent.mu.Unlock()
	if len(r.agent.sent) != 1 || r.agent.sent[0] != "in the app" {
		t.Fatalf("the app's Assistant was sent %q", r.agent.sent)
	}
}
