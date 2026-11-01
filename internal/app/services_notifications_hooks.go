package app

// What each ending the app sees becomes in the Notifications page: the
// words of an entry, by what ended and how, and the two watches that find
// endings nobody reported (a schedule's fire, which the agentry command
// runs out of process, and the version this copy moved to since it last
// ran).

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/highlights"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/notifications"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// firesPoll is how often the schedules' fires are read for ones that
// ended; a test shortens it.
var firesPoll = 30 * time.Second

// Keys of the entries that are about one thing, which a later one replaces.
const (
	keyUpdateAvailable = "update:available"
	keyUpdateInstalled = "update:installed"
)

// agentName is the QuickAgent's name in a reference, "" for words that are
// not one.
func agentName(ref string) string {
	parsed, err := manifest.ParseRef(ref)
	if err != nil {
		return ""
	}
	return parsed.Name
}

// nameOr is the QuickAgent's name, or what to call it when the reference
// names none.
func nameOr(ref, fallback string) string {
	if name := agentName(ref); name != "" {
		return name
	}
	return fallback
}

// classBody is a failed run's cause in one phrase: its exit class's
// meaning from the CLI's table, else a plain sentence. Never the runner's
// own error line, which can quote what the agent wrote.
func classBody(exitClass string, exitCode int) string {
	if meaning, _ := exitClassWords(exitClass, exitCode); meaning != "" {
		return meaning + "."
	}
	return "The run page says what happened."
}

// shownAt is where a job's result is shown besides the page its entry
// opens: a draft and an AI Review land in Build, a knowledge build in
// Knowledge, a Writing Voice build on the Voice page. A person on that page
// is looking at the result, so no notification is raised for it.
func shownAt(kind string) []string {
	switch kind {
	case runhistory.KindDraft, runhistory.KindReview:
		return []string{"build"}
	case runhistory.KindKnowledge:
		return []string{"knowledge"}
	case runhistory.KindVoice:
		return []string{"voice"}
	}
	return nil
}

// ended says whether an outcome is something that ended on its own, which
// a notification is for: a run the person stopped is not.
func ended(outcome string) bool { return outcome != runhistory.OutcomeStopped }

// localEntry is a run, chat or built-in's job on this computer as an entry,
// from its history entry (recorded or not) and the reference it ran.
func localEntry(e runhistory.Entry, ref string) notifications.Entry {
	n := notifications.Entry{
		Kind: e.Kind, Tone: notifications.ToneOK,
		Open: notifications.Open{Page: notifications.PageRun, ID: e.ID, Registry: e.Registry},
	}
	name := nameOr(ref, "The QuickAgent")
	var what, finished, failed, stopped, blocked string
	switch e.Kind {
	case runhistory.KindChat:
		what, finished, failed, stopped = "The chat with "+name, "The chat ended.", "The chat with "+name+" failed", "The chat was ended."
	case runhistory.KindDraft:
		what, finished, failed, stopped = "Your draft", "The Drafter finished writing it; open the draft in Build.", "The draft failed", "You stopped the Drafter."
	case runhistory.KindReview:
		what, finished, failed, stopped = "The AI Review", "The Examiner finished; its findings are in Build.", "The AI Review failed", "You stopped the Examiner."
	case runhistory.KindKnowledge:
		what, finished, failed, stopped = "The knowledge build", "The pack is in your Knowledge.", "The knowledge build failed", "You stopped the Knowledge Creator."
	case runhistory.KindVoice:
		what, finished, failed, stopped = "Your Writing Voice", "The Voice Writer finished; it is on the Voice page.", "The Writing Voice build failed", "You stopped the Voice Writer."
	default:
		what, finished, failed, stopped = name, "The run finished.", name+" failed", "You stopped the run."
		blocked = name + " could not do the task"
	}
	switch e.Outcome {
	case runhistory.OutcomeOK:
		switch e.Kind {
		case runhistory.KindChat:
			n.Title = what + " ended"
		case runhistory.KindDraft, runhistory.KindVoice:
			n.Title = what + " is ready"
		default:
			n.Title = what + " finished"
		}
		n.Body = finished
	case runhistory.OutcomeStopped:
		n.Title, n.Body, n.Tone = what+" was stopped", stopped, notifications.ToneWarn
	case runhistory.OutcomeBlocked:
		if blocked == "" {
			blocked = what + " was blocked"
		}
		n.Title, n.Body, n.Tone = blocked, "It did not have what it needed; the run page says what.", notifications.ToneWarn
	default:
		n.Title, n.Body, n.Tone = failed, classBody(e.ExitClass, e.ExitCode), notifications.ToneBad
	}
	return n
}

// hostedEntry is a run In Agentry that ended as an entry, from the
// registry's record of it.
func hostedEntry(reg string, h *hostedrun.Run) notifications.Entry {
	verdict := hostedrun.Read(h)
	n := notifications.Entry{
		Kind: hostedKind(h), Tone: verdict.Tone,
		Open: notifications.Open{Page: notifications.PageRun, ID: h.ID, Registry: settings.Normalize(reg), Hosted: true},
	}
	ok := h.Status == hostedrun.StatusFinished && h.Outcome == hostedrun.OutcomeSucceeded
	name := nameOr(h.Ref, "The QuickAgent")
	var done, undone string
	switch n.Kind {
	case runhistory.KindDraft:
		done, undone = "Your draft In Agentry is ready", "The draft In Agentry did not finish"
	case runhistory.KindKnowledge:
		done, undone = "The knowledge build In Agentry finished", "The knowledge build In Agentry did not finish"
	case runhistory.KindVoice:
		done, undone = "Your Writing Voice In Agentry is ready", "The Writing Voice build In Agentry did not finish"
	case runhistory.KindChat:
		done, undone = "The chat with "+name+" In Agentry ended", "The chat with "+name+" In Agentry ended"
	default:
		done, undone = name+" finished In Agentry", name+" did not finish In Agentry"
	}
	n.Title = undone
	if ok {
		n.Title = done
	}
	if line := strings.TrimSpace(verdict.Line); line != "" {
		n.Body = firstCapital(line)
	}
	return n
}

// firstCapital is line as one sentence: its first letter a capital, a full
// stop at the end.
func firstCapital(line string) string {
	r := []rune(line)
	r[0] = unicode.ToUpper(r[0])
	return string(r) + "."
}

// fireEntryOf is a schedule's fire as an entry: the run page where it ran,
// the Scheduled page where it did not.
func fireEntryOf(f schedules.Fire) (notifications.Entry, bool) {
	n := notifications.Entry{Kind: notifications.KindSchedule}
	name := f.Name
	if name == "" {
		name = nameOr(f.Ref, "A schedule")
	}
	if f.RunID != "" && validRunID(f.RunID) {
		n.Open = notifications.Open{Page: notifications.PageRun, ID: f.RunID}
	} else {
		n.Open = notifications.Open{Page: notifications.PageScheduled, ID: f.ScheduleID}
	}
	switch f.Outcome {
	case schedules.Ran:
		n.Title, n.Body, n.Tone = name+" ran", "The scheduled run finished.", notifications.ToneOK
	case schedules.Failed:
		n.Title, n.Body, n.Tone = name+" failed", classBody(f.ExitClass, f.ExitCode), notifications.ToneBad
	case schedules.Missed:
		n.Title, n.Body, n.Tone = name+" was missed", "This computer was off or asleep at the time.", notifications.ToneWarn
	default:
		return notifications.Entry{}, false
	}
	return n, true
}

// installEntry is a QuickAgent installed or updated as an entry.
func installEntry(reg, namespace, name, versionInstalled string) notifications.Entry {
	id := namespace + "/" + name
	title := name + " is installed"
	if versionInstalled != "" {
		title = name + " " + versionInstalled + " is installed"
	}
	return notifications.Entry{
		Kind: notifications.KindInstall, Title: title, Body: "It is in your Library, ready to run.", Tone: notifications.ToneOK,
		Open: notifications.Open{Page: notifications.PageAgent, ID: id, Registry: settings.Normalize(reg)},
	}
}

// updateAvailableEntry is a newer Agentry Desktop found by the check.
func updateAvailableEntry(newer string, retired bool) notifications.Entry {
	body := "Install it from Update settings; the app closes and reopens on the new version."
	if retired {
		body = "It is signed with a key that has since been retired, so this copy will not install it; Update settings says more."
	}
	return notifications.Entry{
		Kind: notifications.KindUpdate, Key: keyUpdateAvailable, Title: "Agentry Desktop " + newer + " is available", Body: body,
		Open: notifications.Open{Page: notifications.PageSettings, Section: "updates", Part: "desktop"},
	}
}

// updatedEntry is this copy running a newer version than it last ran.
func updatedEntry(current string, first string) notifications.Entry {
	body := "What's new in Update settings lists what it brings."
	if first != "" {
		body = first
	}
	return notifications.Entry{
		Kind: notifications.KindUpdate, Key: keyUpdateInstalled, Title: "Agentry Desktop is now " + current, Body: body, Tone: notifications.ToneOK,
		Open: notifications.Open{Page: notifications.PageSettings, Section: "updates", Part: "desktop"},
	}
}

// taskEntry is a persistent agent's task that answered: the agent and the
// person's own name for the task, never the answer, which is on the
// agent's page (the notification the task raises shows its first line and
// keeps nothing).
func taskEntry(agent, title string) notifications.Entry {
	return notifications.Entry{
		Kind: notifications.KindTask, Title: title, Body: "Its answer is on the agent's page.",
		Open: notifications.Open{Page: notifications.PagePersistent, ID: agent},
	}
}

// meetingEntry is one of Meeting notes' notices.
func meetingEntry(title, body string) notifications.Entry {
	return notifications.Entry{Kind: notifications.KindMeeting, Title: title, Body: body, Open: notifications.Open{Page: notifications.PageMeetings}}
}

// start runs the two watches: a schedule's fires, and the version moved to.
func (n *NotificationsAPI) start(ctx context.Context) {
	if n == nil {
		return
	}
	go n.noticeVersionMove()
	go n.watchFires(ctx)
}

// noticeVersionMove makes an entry when this copy runs a newer version
// than the one whose What's new the person last closed. A fresh install
// has nothing to move from.
func (n *NotificationsAPI) noticeVersionMove() {
	current, ok := highlights.Version(version.Version)
	if !ok {
		return
	}
	st, err := settings.Load()
	if err != nil || st.WhatsNewSeen == "" || highlights.Compare(st.WhatsNewSeen, current) >= 0 {
		return
	}
	first := ""
	if entries, err := highlights.All(); err == nil {
		if e, ok := highlights.For(entries, current); ok && len(e.Highlights) > 0 {
			first = e.Highlights[0]
		}
	}
	n.noticed(updatedEntry(current, first), false)
}

// watchFires reads the schedules' fires every firesPoll and makes an entry
// for each that ended since the last one noticed. The first run of this
// copy starts from when it began watching: the fires before it are the
// Scheduled page's history, not news. That start is kept in memory until
// a fire is noticed, so a copy that has nothing to record writes no file.
func (n *NotificationsAPI) watchFires(ctx context.Context) {
	for {
		n.noticeFires()
		select {
		case <-ctx.Done():
			return
		case <-time.After(firesPoll):
		}
	}
}

// fires is the fires read, newest first; a test replaces it.
var fires = func() ([]schedules.Fire, error) {
	home, err := run.Home()
	if err != nil {
		return nil, err
	}
	return schedules.StoreIn(home).Fires(0)
}

// noticeFires is one read of the fires.
func (n *NotificationsAPI) noticeFires() {
	seen, err := notifications.FiresSeen()
	if err != nil {
		return
	}
	now := time.Now()
	if n.now != nil {
		now = n.now()
	}
	if seen.IsZero() {
		n.mu.Lock()
		if n.firesSince.IsZero() {
			n.firesSince = now
		}
		seen = n.firesSince
		n.mu.Unlock()
	}
	list, err := fires()
	if err != nil {
		return
	}
	newest := seen
	// Oldest first, so the entries read in the order the fires ended.
	for i := len(list) - 1; i >= 0; i-- {
		f := list[i]
		ended, err := time.Parse(time.RFC3339, f.Ended)
		if err != nil || !ended.After(seen) {
			continue
		}
		if e, ok := fireEntryOf(f); ok {
			e.CreatedAt = ended.UTC()
			n.noticed(e, f.Outcome != schedules.Missed)
		}
		if ended.After(newest) {
			newest = ended
		}
	}
	if newest.After(seen) {
		if err := notifications.SetFiresSeen(newest); err != nil {
			log.Printf("agentry-desktop: notifications: %v", err)
		}
	}
}
