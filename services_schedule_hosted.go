package main

// Schedules In Agentry: the When step's Later and Repeat for a run In
// Agentry, and the account's hosted schedules on the Scheduled page and
// Home's card beside the ones on this computer, labelled In Agentry.
//
// The registry keeps them (the enterprise schedules extension) and decides
// every refusal; the app drives its routes with the token it already holds,
// whose run:read and run:write are what they take, and keeps nothing of its
// own. A registry that cannot be reached leaves the hosted half unavailable,
// in a sentence, and the local half as it was: no read here answers an error
// the page would draw over the schedules on this computer.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// hostedReadTimeout bounds a hosted read the page waits on: a registry that
// does not answer makes the hosted half unavailable, not the page slow.
const hostedReadTimeout = 15 * time.Second

// HostedScheduleOfferView says whether Later and Repeat are offered In
// Agentry here, can be chosen, and why not (DecideSchedule).
type HostedScheduleOfferView struct {
	Offered   bool   `json:"offered"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
	// Upgrade: the way out is a higher plan; the page offers the
	// registry's upgrade page.
	Upgrade bool `json:"upgrade,omitempty"`
}

func scheduleOfferView(o hostedrun.Offer) *HostedScheduleOfferView {
	if !o.Offered {
		return nil
	}
	return &HostedScheduleOfferView{Offered: true, Available: o.Available, Reason: o.Reason, Remedy: o.Remedy, Upgrade: o.Upgrade}
}

// hostedFacts reads what DecideSchedule needs of reg: the token, the
// extensions, its scopes and the caller's payer. reach is why the registry
// could not be asked, when it could not.
func hostedFacts(ctx context.Context, reg string) (f hostedrun.Facts, token, reach string) {
	token = settings.TokenFor(reg)
	f.SignedIn = token != ""
	if !f.SignedIn {
		return f, "", ""
	}
	exts, err := extensionsOf(ctx, reg)
	if err != nil {
		return f, token, fmt.Sprintf("%s could not be reached: %v", originOf(reg), err)
	}
	f.Extension, f.Vault, f.Schedules = hasString(exts, "runs"), hasString(exts, "vault"), hasString(exts, "schedules")
	if !f.Schedules {
		return f, token, ""
	}
	f.Scopes, f.ScopesKnown = tokenScopes(ctx, reg, token)
	if st, err := hostedrun.New(reg, token).Status(ctx); err == nil && st.Caller != nil && st.Caller.Session != nil {
		allowed := st.Caller.Session.Allowed
		f.ScheduleAllowed = &allowed
	}
	return f, token, ""
}

// HostedSchedulesView is the account's schedules In Agentry, as the
// Scheduled page lists them.
type HostedSchedulesView struct {
	// Registry is the one asked, by its origin.
	Registry string `json:"registry"`
	// SignedOut: no token is kept for it, so nothing In Agentry is read;
	// the page says signing in shows them.
	SignedOut bool `json:"signedOut,omitempty"`
	// Unavailable is why the account's schedules could not be read: the
	// registry did not answer, or refused. The page says it, and shows the
	// schedules on this computer as they are.
	Unavailable string               `json:"unavailable,omitempty"`
	Schedules   []HostedScheduleItem `json:"schedules"`
	// Limit is how many schedules that have not ended the account may keep.
	Limit int `json:"limit,omitempty"`
}

// HostedScheduleItem is one schedule In Agentry, in the shape the page's
// list of local schedules has, so one list holds both.
type HostedScheduleItem struct {
	Hosted   bool   `json:"hosted"`
	Registry string `json:"registry"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	// Ref is ns/name and Version the version pinned, as a local item's.
	Ref      string         `json:"ref"`
	Version  string         `json:"version"`
	Describe string         `json:"describe"`
	When     schedules.When `json:"when"`
	Paused   bool           `json:"paused,omitempty"`
	// PausedReason is who paused it: owner, refusals, entitlement or
	// account.
	PausedReason string `json:"pausedReason,omitempty"`
	Ended        bool   `json:"ended,omitempty"`
	Runs         int    `json:"runs,omitempty"`
	Refusals     int    `json:"refusals,omitempty"`
	Next         string `json:"next,omitempty"`
	// Files are the names of the files it copied, which each run is given.
	Files []string `json:"files,omitempty"`
}

// whenOfHosted is a hosted recurrence as the When form takes it.
func whenOfHosted(r hostedrun.Recurrence) schedules.When {
	w := schedules.When{Kind: r.Kind, Date: r.Date, Time: r.Time, Days: r.Days, MonthDay: r.MonthDay, Zone: r.Zone}
	if r.End != nil {
		w.End, w.Until, w.Times = r.End.Kind, r.End.On, r.End.After
	}
	return w
}

// hostedRecurrence is the When form's answer as the extension takes it,
// checked by the same rule a local schedule's is; a zone left empty is this
// computer's, since the extension needs one named.
func hostedRecurrence(w schedules.When) (*hostedrun.Recurrence, error) {
	if w.Zone == "" {
		w.Zone = schedule.LocalZone()
	}
	if _, err := w.Args(); err != nil {
		return nil, err
	}
	r := &hostedrun.Recurrence{Kind: w.Kind, Time: w.Time, Zone: w.Zone}
	switch w.Kind {
	case "once":
		r.Date = w.Date
	case "weekly":
		r.Days = w.Days
	case "monthly":
		r.MonthDay = w.MonthDay
	}
	if w.Kind != "once" {
		switch w.End {
		case "on":
			r.End = &hostedrun.End{Kind: "on", On: w.Until}
		case "after":
			r.End = &hostedrun.End{Kind: "after", After: w.Times}
		}
	}
	return r, nil
}

func hostedItem(reg string, s hostedrun.Schedule) HostedScheduleItem {
	ref, version, _ := strings.Cut(s.Ref, "@")
	it := HostedScheduleItem{
		Hosted: true, Registry: reg, ID: s.ID, Name: s.Name, Ref: ref, Version: version, Describe: s.Repeats,
		When: whenOfHosted(s.When), Paused: s.State == "paused", PausedReason: s.PausedReason, Ended: s.State == "ended",
		Runs: s.Runs, Refusals: s.Refusals, Next: s.NextAt,
	}
	for _, in := range s.Inputs {
		it.Files = append(it.Files, in.Name)
	}
	return it
}

// scheduleClient is reg's client for the account's schedules (hostedClientFor).
func scheduleClient(reg string) (*hostedrun.Client, error) {
	return hostedClientFor(reg, "schedule runs In Agentry")
}

// notShown is the sentence a calendar read In Agentry says when the
// registry did not answer.
func notShown(reach string) string {
	return "Your schedules In Agentry are not shown: " + reach + "."
}

// hostedSaid is a refusal as a sentence, or err where it is none.
func hostedSaid(err error) error {
	if r := hostedrun.RefusalOf(err); r != nil {
		return errors.New(r.Sentence())
	}
	return err
}

// HostedOffer is whether a new schedule can be made In Agentry at reg
// (DecideSchedule): nil where none is offered (signed out, a registry
// without schedules, or one that did not answer). The page reads it once,
// apart from the list it reads at every change: its facts are four reads of
// the registry, and an edge that spaces out a burst made a refresh that read
// them every time outlast its time.
func (s *ScheduleAPI) HostedOffer(reg string) (*HostedScheduleOfferView, error) {
	reg = settings.Normalize(reg)
	if settings.ValidateURL(reg) != nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(s.ctx(), hostedReadTimeout)
	defer cancel()
	f, _, reach := hostedFacts(ctx, reg)
	if reach != "" || !f.SignedIn || !f.Schedules {
		return nil, nil
	}
	return scheduleOfferView(hostedrun.DecideSchedule(f)), nil
}

// HostedSchedules is the account's schedules In Agentry at reg, in one read
// of the registry. Nothing is listed signed out, or where the registry
// schedules no runs (its route a 404); a registry that cannot be reached is
// Unavailable, never an error.
func (s *ScheduleAPI) HostedSchedules(reg string) (*HostedSchedulesView, error) {
	reg = settings.Normalize(reg)
	v := &HostedSchedulesView{Registry: originOf(reg), Schedules: []HostedScheduleItem{}}
	if err := settings.ValidateURL(reg); err != nil {
		return v, nil
	}
	token := settings.TokenFor(reg)
	if token == "" {
		v.SignedOut = true
		return v, nil
	}
	ctx, cancel := context.WithTimeout(s.ctx(), hostedReadTimeout)
	defer cancel()
	items, limit, err := hostedrun.New(reg, token).Schedules(ctx)
	if r := hostedrun.RefusalOf(err); r != nil && r.Status == http.StatusNotFound {
		return v, nil
	}
	if err != nil {
		v.Unavailable = "Your schedules In Agentry could not be read: " + hostedSaid(err).Error() + "."
		return v, nil
	}
	v.Limit = limit
	for _, it := range items {
		v.Schedules = append(v.Schedules, hostedItem(reg, it))
	}
	sort.SliceStable(v.Schedules, func(i, j int) bool { return v.Schedules[i].Name < v.Schedules[j].Name })
	return v, nil
}

// CalendarFire is one occurrence as a merged calendar shows it: a
// schedules.Fire's shape, marked hosted for one In Agentry, with the
// extension's reason for a fire that started no run.
type CalendarFire struct {
	Hosted     bool   `json:"hosted"`
	ScheduleID string `json:"scheduleId"`
	Name       string `json:"name"`
	At         string `json:"at"`
	// Outcome is due, pending (being tried, or starting), ran (a run was
	// started), skipped or failed.
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
	RunID   string `json:"runId,omitempty"`
	Zone    string `json:"zone,omitempty"`
}

// hostedOutcome is an occurrence's state in the calendar's words: a coming
// one is due, as a local one is, and one being tried is pending.
func hostedOutcome(state string) string {
	switch state {
	case hostedrun.OccurrenceUpcoming:
		return schedules.Due
	case hostedrun.OccurrencePending, hostedrun.OccurrenceFiring:
		return "pending"
	}
	return state
}

// HostedCalendarView is what happened and what is due In Agentry in a
// calendar's range.
type HostedCalendarView struct {
	Fires       []CalendarFire `json:"fires"`
	Unavailable string         `json:"unavailable,omitempty"`
}

// hostedOccurrences reads [from, to) at reg: nil and "" where nothing is
// listed (signed out, or a registry without the schedules extension, whose
// route is a 404), and why not where the registry did not answer.
func hostedOccurrences(ctx context.Context, reg string, from, to time.Time) ([]CalendarFire, string) {
	reg = settings.Normalize(reg)
	token := settings.TokenFor(reg)
	if settings.ValidateURL(reg) != nil || token == "" {
		return nil, ""
	}
	occ, err := hostedrun.New(reg, token).Occurrences(ctx, from, to)
	if r := hostedrun.RefusalOf(err); r != nil && r.Status == http.StatusNotFound {
		return nil, ""
	}
	if err != nil {
		return nil, fmt.Sprintf("%s did not answer for your schedules In Agentry: %v", originOf(reg), hostedSaid(err))
	}
	out := make([]CalendarFire, 0, len(occ))
	for _, o := range occ {
		out = append(out, CalendarFire{Hosted: true, ScheduleID: o.ScheduleID, Name: o.Name, At: o.At, Outcome: hostedOutcome(o.State), Reason: o.Reason, RunID: o.RunID, Zone: o.Zone})
	}
	return out, ""
}

// HostedCalendar is the account's occurrences In Agentry in [from, to)
// (RFC 3339, as Calendar takes it).
func (s *ScheduleAPI) HostedCalendar(reg, from, to string) (*HostedCalendarView, error) {
	f, t, err := calendarRange(from, to)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), hostedReadTimeout)
	defer cancel()
	fires, reach := hostedOccurrences(ctx, reg, f, t)
	v := &HostedCalendarView{Fires: fires}
	if v.Fires == nil {
		v.Fires = []CalendarFire{}
	}
	if reach != "" {
		v.Unavailable = notShown(reach)
	}
	return v, nil
}

// hostedHistoryDays is how far back the History list reads In Agentry,
// inside the extension's 92-day range.
const hostedHistoryDays = 31

// HostedHistory is what the account's schedules In Agentry met in the last
// month, newest first, at most limit (1 to 200).
func (s *ScheduleAPI) HostedHistory(reg string, limit int) (*HostedCalendarView, error) {
	now := s.clock()
	ctx, cancel := context.WithTimeout(s.ctx(), hostedReadTimeout)
	defer cancel()
	fires, reach := hostedOccurrences(ctx, reg, now.Add(-hostedHistoryDays*24*time.Hour), now)
	v := &HostedCalendarView{Fires: []CalendarFire{}}
	for _, f := range fires {
		if f.Outcome != schedules.Due {
			v.Fires = append(v.Fires, f)
		}
	}
	sort.SliceStable(v.Fires, func(i, j int) bool { return v.Fires[i].At > v.Fires[j].At })
	if n := min(max(limit, 1), 200); len(v.Fires) > n {
		v.Fires = v.Fires[:n]
	}
	if reach != "" {
		v.Unavailable = notShown(reach)
	}
	return v, nil
}

// HostedScheduleRequest is a schedule In Agentry as the run setup describes
// it: the run's own answers, and When. Files are paths this session's picker
// returned, uploaded when it is made; Knowledge names the account's packs
// kept in Agentry.
type HostedScheduleRequest struct {
	Registry  string            `json:"registry"`
	Ref       string            `json:"ref"`
	Name      string            `json:"name,omitempty"`
	Task      string            `json:"task,omitempty"`
	Files     []string          `json:"files,omitempty"`
	Knowledge []string          `json:"knowledge,omitempty"`
	Intake    map[string]string `json:"intake,omitempty"`
	Voice     bool              `json:"voice"`
	When      schedules.When    `json:"when"`
	// AskOptionalSecrets makes the registry's check refuse an optional
	// secret the account does not keep, as a required one, so the When step
	// asks for it; a schedule made again after the ask leaves it false.
	AskOptionalSecrets bool `json:"askOptionalSecrets,omitempty"`
	// SourceTokens are tokens typed in the setup for the release's input
	// sources, by question id; SaveSourceKeys is the person's leave to save
	// each source's token (typed, else the key saved on this computer for
	// its host) in Agentry as the key for that host, which a schedule's
	// fires use. A token crosses the app's IPC inward only.
	SourceTokens   map[string]string `json:"sourceTokens,omitempty"`
	SaveSourceKeys bool              `json:"saveSourceKeys,omitempty"`
}

// HostedScheduleMadeView is a schedule just made In Agentry, or the gap
// that kept it from being made: the registry's refusal, its slug for the
// page to offer the fix (secret-missing and secret-not-kept: the secrets to
// keep, in Missing; connection-missing: the servers to connect), and its
// sentence.
type HostedScheduleMadeView struct {
	ID      string   `json:"id,omitempty"`
	Next    string   `json:"next,omitempty"`
	Refusal string   `json:"refusal,omitempty"`
	Detail  string   `json:"detail,omitempty"`
	Missing []string `json:"missing,omitempty"`
	// Optional are the release's optional secrets a secret-missing names:
	// those of Missing the ask may leave empty.
	Optional []string `json:"optional,omitempty"`
}

// HostedAdd makes the schedule req describes In Agentry. The registry's
// create is the check: it runs the run's own create, stopped before anything
// is held, as the schedule's fires will, and stores nothing unless every
// input a fire needs is in place. A gap comes back as the refusal, not an
// error, for the When step to offer its fix and make the schedule again; the
// files go up at each try (an upload nobody claims is swept by the registry
// after a day).
func (s *ScheduleAPI) HostedAdd(req HostedScheduleRequest) (*HostedScheduleMadeView, error) {
	when, err := hostedRecurrence(req.When)
	if err != nil {
		return nil, err
	}
	reg := settings.Normalize(req.Registry)
	c, err := scheduleClient(reg)
	if err != nil {
		return nil, err
	}
	for _, p := range req.Files {
		if err := s.app.requirePicked("input", p); err != nil {
			return nil, err
		}
	}
	ctx := s.ctx()
	var keys map[string]string
	// An unscanned release is not run In Agentry from here, so it is not
	// scheduled there either.
	if sig, err := run.ResolveSignature(ctx, reg, req.Ref, false); err == nil {
		if sig.Unscanned() {
			return nil, fmt.Errorf("%s was admitted by the registry without a scan; a run In Agentry is not offered for an unscanned release until the platform records that consent, so it is not scheduled there either", req.Ref)
		}
		// A target host is reached only from this computer.
		if err := run.RefuseHostedTargetHosts(sig.Manifest); err != nil {
			return nil, err
		}
		// A schedule's fire has no setup to send a one-run token from: it
		// uses the key Agentry keeps for each source's host. A source given
		// a token here has it saved there, with the person's leave, once the
		// schedule is made; without the leave, the schedule is held, since
		// its fires would fetch without the token the person meant them to
		// use.
		if keys, err = run.HostedScheduleKeys(sig.Manifest, req.Intake, req.SourceTokens, nil); err != nil {
			return nil, err
		}
		if len(keys) > 0 && !req.SaveSourceKeys {
			return nil, fmt.Errorf("a schedule In Agentry fetches its input sources with the keys Agentry keeps for their hosts: tick \u201cSave in Agentry\u201d to keep the token there for %s, or schedule it on this computer", strings.Join(sortedKeys(keys), ", "))
		}
	} else if len(req.SourceTokens) > 0 {
		return nil, fmt.Errorf("the release %s could not be read, so its input sources' tokens were not saved: %w", req.Ref, err)
	}
	var packs []string
	if s.run != nil {
		if packs, err = s.run.packsInAgentry(reg, req.Knowledge); err != nil {
			return nil, err
		}
	} else if len(req.Knowledge) > 0 {
		return nil, errors.New("the knowledge packs to attach could not be read")
	}
	files, err := hostedrun.StageInputs(req.Files)
	if err != nil {
		return nil, err
	}
	ids, err := c.UploadInputs(ctx, files)
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return &HostedScheduleMadeView{Refusal: r.Slug, Detail: r.Sentence(), Missing: r.Missing, Optional: r.Optional}, nil
		}
		return nil, err
	}
	voice := req.Voice
	made, err := c.CreateSchedule(ctx, hostedrun.ScheduleRequest{
		Name: strings.TrimSpace(req.Name), Ref: req.Ref, Task: req.Task, Intake: req.Intake, Knowledge: packs, Voice: &voice, Inputs: ids, When: when,
		AskOptionalSecrets: req.AskOptionalSecrets,
	})
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return &HostedScheduleMadeView{Refusal: r.Slug, Detail: r.Sentence(), Missing: r.Missing, Optional: r.Optional}, nil
		}
		return nil, err
	}
	// The keys its fires use, saved once the schedule is made, so a
	// refused create leaves none behind; one that cannot be saved takes
	// the schedule back, so it never fires without the key it was given.
	for _, host := range sortedKeys(keys) {
		if err := c.SetSourceKey(ctx, host, keys[host]); err != nil {
			if derr := c.DeleteSchedule(context.WithoutCancel(ctx), made.ID); derr != nil {
				emitEvent(s.app, eventSchedulesChanged)
				return nil, fmt.Errorf("the schedule was made, but the key for %s could not be saved in Agentry (%v), and the schedule could not be taken back (%v): delete it on the Scheduled page, or save the key on the site's Secrets page", host, err, derr)
			}
			if r := hostedrun.RefusalOf(err); r != nil {
				return &HostedScheduleMadeView{Refusal: r.Slug, Detail: r.Sentence()}, nil
			}
			return nil, err
		}
	}
	emitEvent(s.app, eventSchedulesChanged)
	return &HostedScheduleMadeView{ID: made.ID, Next: made.NextAt}, nil
}

// hostedID is a schedule id as the extension issues it, a UUID: checked
// before any path is built.
var hostedID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func checkHostedID(id string) error {
	if !hostedID.MatchString(id) {
		return fmt.Errorf("%q is not a schedule In Agentry", id)
	}
	return nil
}

// hostedAct runs one change on schedule id at reg and tells the page.
func (s *ScheduleAPI) hostedAct(reg, id string, f func(ctx context.Context, c *hostedrun.Client) error) error {
	if err := checkHostedID(id); err != nil {
		return err
	}
	c, err := scheduleClient(reg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	defer cancel()
	if err := f(ctx, c); err != nil {
		return hostedSaid(err)
	}
	emitEvent(s.app, eventSchedulesChanged)
	return nil
}

// HostedEdit changes a schedule In Agentry: its name, or when it runs,
// which starts its repeat over from now (the registry's rule, as the
// CLI's). The registry checks what it runs again.
func (s *ScheduleAPI) HostedEdit(reg, id string, e schedules.Edit) error {
	var sr hostedrun.ScheduleRequest
	sr.Name = strings.TrimSpace(e.Name)
	if e.When != nil {
		when, err := hostedRecurrence(*e.When)
		if err != nil {
			return err
		}
		sr.When = when
	}
	if sr.Name == "" && sr.When == nil {
		return errors.New("nothing to change")
	}
	return s.hostedAct(reg, id, func(ctx context.Context, c *hostedrun.Client) error {
		_, err := c.UpdateSchedule(ctx, id, sr)
		return err
	})
}

// HostedPause pauses a schedule In Agentry: nothing fires until Resume.
func (s *ScheduleAPI) HostedPause(reg, id string) error {
	return s.hostedAct(reg, id, func(ctx context.Context, c *hostedrun.Client) error {
		_, err := c.PauseSchedule(ctx, id)
		return err
	})
}

// HostedResume resumes it from its next occurrence after now; the
// registry checks it again as it does a new one.
func (s *ScheduleAPI) HostedResume(reg, id string) error {
	return s.hostedAct(reg, id, func(ctx context.Context, c *hostedrun.Client) error {
		_, err := c.ResumeSchedule(ctx, id)
		return err
	})
}

// HostedRemove deletes a schedule In Agentry after a native Yes/No; the runs
// it started stay. A No is no error.
func (s *ScheduleAPI) HostedRemove(reg, id, name string) (bool, error) {
	if err := checkHostedID(id); err != nil {
		return false, err
	}
	if err := s.app.askConsent("Delete the schedule "+name+" In Agentry?", "It will not run again, and its files go from Agentry. The runs it started are kept.\n\nDelete it?"); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return false, nil
		}
		return false, err
	}
	return true, s.hostedAct(reg, id, func(ctx context.Context, c *hostedrun.Client) error {
		return c.DeleteSchedule(ctx, id)
	})
}

// HostedRunNowView is a Run now In Agentry: the run it started, or the
// registry's refusal (run-active, credits-exhausted, …) in a sentence.
type HostedRunNowView struct {
	RunID   string `json:"runId,omitempty"`
	Refusal string `json:"refusal,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// HostedRunNow starts an ordinary run of the schedule's setup now, which
// uses up no occurrence; its page is the run's own In Agentry.
func (s *ScheduleAPI) HostedRunNow(reg, id string) (*HostedRunNowView, error) {
	if err := checkHostedID(id); err != nil {
		return nil, err
	}
	c, err := scheduleClient(reg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	defer cancel()
	runID, err := c.RunScheduleNow(ctx, id)
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return &HostedRunNowView{Refusal: r.Slug, Detail: r.Sentence()}, nil
		}
		return nil, err
	}
	emitEvent(s.app, eventSchedulesChanged)
	return &HostedRunNowView{RunID: runID}, nil
}

// sortedKeys are m's keys, sorted.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
