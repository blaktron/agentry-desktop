package app

// Schedules on this computer: the Scheduled page, the run setup's When step,
// the fires in Recent runs, and the re-seal after a key change.
//
// The CLI owns them. Its store under the runner home is the truth, and each
// schedule's entry in the system's scheduler pins the agentry binary that
// wrote it, so every change is the installed agentry command's
// (internal/schedules): an entry written from this process would name the
// desktop. What the page shows is read in process from the same store. The
// page names a schedule by its id and never a path; the paths a schedule is
// given come from this session's pickers, as a run's do.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"

	cliknowledge "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/cliinstall"
	"github.com/blaktron/agentry-desktop/internal/destinations"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Events the Scheduled page listens for.
const (
	// eventSchedulesChanged: a schedule was made, changed or removed, or a
	// Run now ended; the page reads the store again.
	eventSchedulesChanged = "schedules:changed"
	// eventSchedulesReseal: a key change left signed-out schedules whose
	// keys could not be sealed again (the sentence).
	eventSchedulesReseal = "schedules:reseal"
)

// lingerSentence is what turning linger on does, in the CLI's words.
const lingerSentence = "Agentry's schedules will start when this computer starts, even if you are not signed in"

// cliTimeout bounds a call that changes or checks a schedule: add's
// install step pulls images, so it is long; the others are short.
const (
	cliTimeout      = 2 * time.Minute
	cliAddTimeout   = 30 * time.Minute
	cliQuickTimeout = 30 * time.Second
)

// ScheduleAPI is the Scheduled page's and the When step's.
type ScheduleAPI struct {
	app *App
	// run is the hosted runs' API, whose Knowledge list resolves the packs a
	// schedule In Agentry attaches.
	run *RunAPI
	// Seams for tests; nil is this computer's.
	findCLI  func(ctx context.Context) (*schedules.CLI, error)
	home     func() (string, error)
	now      func() time.Time
	lingerOn func() bool
	// locateCLI only finds the command on disk (Home's card), where
	// findCLI also runs it for its version.
	locateCLI func() (string, error)
	// installOffered and installRun stand in for the Options menu's
	// install of the agentry command (services_schedule_cli.go).
	installOffered func() bool
	installRun     func(ctx context.Context) (cliinstall.Result, error)
	// carriedCLI and shellLookup stand in for the command a macOS bundle
	// carries and what a new terminal finds as `agentry`.
	carriedCLI  func() string
	shellLookup func() string
	// readyScheduleHosts readies a schedule's target hosts; nil is
	// run.ReadyScheduleHosts.
	readyScheduleHosts func(ctx context.Context, s run.ScheduleHosts) (int, error)

	mu sync.Mutex
	// running is the Run now under way, by schedule id.
	running map[string]bool
}

// resealMu guards resealProblem: what the last re-seal could not do,
// shown on the Scheduled page until a re-seal succeeds.
var (
	resealMu      sync.Mutex
	resealProblem string
)

func (s *ScheduleAPI) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *ScheduleAPI) store() (schedules.Store, error) {
	home := run.Home
	if s.home != nil {
		home = s.home
	}
	h, err := home()
	if err != nil {
		return schedules.Store{}, err
	}
	return schedules.StoreIn(h), nil
}

// installedCLI is the agentry command this app drives, refused when it is
// older than the one that has what the app asks of it.
func installedCLI(ctx context.Context) (*schedules.CLI, error) {
	path, err := schedules.FindCLI()
	if err != nil {
		return nil, err
	}
	c := schedules.New(path)
	vctx, cancel := context.WithTimeout(ctx, cliQuickTimeout)
	defer cancel()
	line, err := c.Version(vctx)
	if err != nil {
		return nil, fmt.Errorf("the agentry command at %s did not answer: %w", path, err)
	}
	if why := schedules.TooOld(line); why != "" {
		return nil, errors.New(why)
	}
	return c, nil
}

func (s *ScheduleAPI) cli() (*schedules.CLI, error) {
	if s.findCLI != nil {
		return s.findCLI(s.ctx())
	}
	return installedCLI(s.ctx())
}

func (s *ScheduleAPI) ctx() context.Context {
	if s.app != nil && s.app.ctx != nil {
		return s.app.ctx
	}
	return context.Background()
}

// thisUserLingers reports whether linger is on for the person: their
// schedules already start with the computer, so turning it on asks
// nothing.
func thisUserLingers() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	on, err := schedule.NewLinger(u.Username).On()
	return err == nil && on
}

func (s *ScheduleAPI) lingers() bool {
	if s.lingerOn != nil {
		return s.lingerOn()
	}
	return thisUserLingers()
}

// SignedOutView is whether a schedule can also run signed out here.
type SignedOutView struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// ScheduleAvailabilityView is whether this computer can schedule a run at
// all, and with what.
type ScheduleAvailabilityView struct {
	// Available: the agentry command is installed and new enough.
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	// Zone is this computer's time zone, a new schedule's default.
	Zone      string        `json:"zone"`
	SignedOut SignedOutView `json:"signedOut"`
	// LingerOn: schedules already start with this computer, so the
	// signed-out checkbox asks nothing more.
	LingerOn bool `json:"lingerOn"`
	// InstallCLI is the Options menu's entry that installs or reinstalls the
	// agentry command, "" where the app offers none.
	InstallCLI string `json:"installCLI,omitempty"`
}

// Availability says whether a run can be scheduled on this computer, and
// whether a schedule can also run while the person is signed out.
func (s *ScheduleAPI) Availability() (*ScheduleAvailabilityView, error) {
	v := &ScheduleAvailabilityView{Zone: schedule.LocalZone(), InstallCLI: s.installCLILabelFor()}
	c, err := s.cli()
	if err != nil {
		v.Reason = err.Error()
		v.SignedOut.Reason = err.Error()
		return v, nil
	}
	v.Available = true
	ctx, cancel := context.WithTimeout(s.ctx(), cliQuickTimeout)
	defer cancel()
	so, err := c.SignedOutHere(ctx)
	if err != nil {
		v.SignedOut.Reason = "whether this computer can run a schedule while you are signed out could not be read: " + err.Error()
		return v, nil
	}
	v.SignedOut = SignedOutView{Available: so.Available, Reason: so.Reason}
	v.LingerOn = so.Available && s.lingers()
	return v, nil
}

// SchedulesView is the Scheduled page's list.
type SchedulesView struct {
	Schedules []schedules.Item `json:"schedules"`
	// Running are the schedules whose Run now is under way.
	Running []string `json:"running"`
	// ResealProblem is what the last key change could not seal again.
	ResealProblem string `json:"resealProblem,omitempty"`
}

// List is every schedule on this computer, read from the store.
func (s *ScheduleAPI) List() (*SchedulesView, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	items, err := st.List(s.clock())
	if err != nil {
		return nil, err
	}
	v := &SchedulesView{Schedules: items, Running: []string{}}
	s.mu.Lock()
	for id := range s.running {
		v.Running = append(v.Running, id)
	}
	s.mu.Unlock()
	resealMu.Lock()
	v.ResealProblem = resealProblem
	resealMu.Unlock()
	return v, nil
}

// maxCalendarRange is the longest range one call reads: a month view
// asks for six weeks.
const maxCalendarRange = 62 * 24 * time.Hour

// Calendar is what happened and what is due in [from, to) (RFC 3339).
func (s *ScheduleAPI) Calendar(from, to string) ([]schedules.Fire, error) {
	f, t, err := calendarRange(from, to)
	if err != nil {
		return nil, err
	}
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.Between(f, t, s.clock())
}

// calendarRange is a calendar's [from, to), RFC 3339, at most
// maxCalendarRange long.
func calendarRange(from, to string) (time.Time, time.Time, error) {
	f, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("from: %w", err)
	}
	t, err := time.Parse(time.RFC3339, to)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("to: %w", err)
	}
	if !t.After(f) || t.Sub(f) > maxCalendarRange {
		return time.Time{}, time.Time{}, errors.New("a calendar reads at most 62 days at a time")
	}
	return f, t, nil
}

// ScheduledCardView is Home's Scheduled card: the month's occurrences and
// the next three to come, on this computer and In Agentry.
type ScheduledCardView struct {
	// Reason is why this computer cannot schedule a run, when the agentry
	// command cannot be found: the card says it rather than an empty month.
	Reason string `json:"reason,omitempty"`
	// Schedules is how many schedules the store holds.
	Schedules int `json:"schedules"`
	// Month is what happened and what is due on this computer in the range
	// asked, and HostedMonth In Agentry.
	Month       []schedules.Fire `json:"month"`
	HostedMonth []CalendarFire   `json:"hostedMonth"`
	// HostedSchedules is how many the account keeps In Agentry, and
	// HostedReason why they are not shown, when the registry did not
	// answer.
	HostedSchedules int    `json:"hostedSchedules"`
	HostedReason    string `json:"hostedReason,omitempty"`
	// Next are the next occurrences due, here or In Agentry, soonest
	// first, at most three.
	Next []CalendarFire `json:"next"`
}

// cardNext is how many occurrences the Home card lists, and cardAhead how
// far ahead it looks for them: a year here, so a once schedule months away
// is still found, and the registry's widest range In Agentry.
const (
	cardNext        = 3
	cardAhead       = 366 * 24 * time.Hour
	cardHostedAhead = hostedrun.MaxOccurrenceRange
	// cardHostedTimeout bounds what Home waits on the registry.
	cardHostedTimeout = 8 * time.Second
)

// HomeCard is the Scheduled card's read: the month in [from, to)
// (RFC 3339, as Calendar takes it) and the next three occurrences, from
// the store on this computer and, signed in to reg, from the account's
// schedules In Agentry. It finds the agentry command on disk and runs
// nothing, as Home starts nothing.
func (s *ScheduleAPI) HomeCard(from, to, reg string) (*ScheduledCardView, error) {
	month, err := s.Calendar(from, to)
	if err != nil {
		return nil, err
	}
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	now := s.clock()
	items, err := st.List(now)
	if err != nil {
		return nil, err
	}
	ahead, err := st.Between(now, now.Add(cardAhead), now)
	if err != nil {
		return nil, err
	}
	v := &ScheduledCardView{Schedules: len(items), Month: month, HostedMonth: []CalendarFire{}, Next: []CalendarFire{}}
	var next []CalendarFire
	for _, f := range ahead {
		if f.Outcome == schedules.Due {
			next = append(next, CalendarFire{ScheduleID: f.ScheduleID, Name: f.Name, At: f.At, Outcome: f.Outcome})
		}
	}
	if reg != "" {
		// Calendar above has refused a range out of shape already.
		f, t, _ := calendarRange(from, to)
		ctx, cancel := context.WithTimeout(s.ctx(), cardHostedTimeout)
		hv, herr := s.hostedForCard(ctx, reg, f, t, now)
		cancel()
		if herr != "" {
			v.HostedReason = herr
		} else if hv != nil {
			v.HostedSchedules, v.HostedMonth = hv.schedules, hv.month
			next = append(next, hv.next...)
		}
	}
	sort.SliceStable(next, func(i, j int) bool { return next[i].At < next[j].At })
	if len(next) > cardNext {
		next = next[:cardNext]
	}
	v.Next = append(v.Next, next...)
	locate := schedules.FindCLI
	if s.locateCLI != nil {
		locate = s.locateCLI
	}
	if _, err := locate(); err != nil {
		v.Reason = err.Error()
	}
	return v, nil
}

// hostedCard is the account's schedules In Agentry as Home's card shows
// them.
type hostedCard struct {
	schedules int
	month     []CalendarFire
	next      []CalendarFire
}

// hostedForCard reads the account's schedules In Agentry at reg for Home's
// card: nil where nothing is listed (signed out, no schedules extension),
// and why not where the registry did not answer.
func (s *ScheduleAPI) hostedForCard(ctx context.Context, reg string, from, to, now time.Time) (*hostedCard, string) {
	c, err := scheduleClient(reg)
	if err != nil {
		return nil, ""
	}
	items, _, err := c.Schedules(ctx)
	if r := hostedrun.RefusalOf(err); r != nil && r.Status == http.StatusNotFound {
		return nil, ""
	}
	if err != nil {
		return nil, notShown(originOf(settings.Normalize(reg)) + " did not answer (" + hostedSaid(err).Error() + ")")
	}
	hc := &hostedCard{schedules: len(items)}
	if len(items) == 0 {
		return hc, ""
	}
	month, reach := hostedOccurrences(ctx, reg, from, to)
	if reach != "" {
		return nil, notShown(reach)
	}
	hc.month = month
	ahead, reach := hostedOccurrences(ctx, reg, now, now.Add(cardHostedAhead))
	if reach != "" {
		return nil, notShown(reach)
	}
	for _, f := range ahead {
		if f.Outcome == schedules.Due && len(hc.next) < cardNext {
			hc.next = append(hc.next, f)
		}
	}
	return hc, ""
}

// History is the fires recorded, newest first, at most limit (1 to 200).
func (s *ScheduleAPI) History(limit int) ([]schedules.Fire, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	return st.Fires(min(max(limit, 1), 200))
}

// gates are a run's gates for a schedule (localRunGates' first half): the
// files and the output folder from this session's pickers, and a
// vulnerable server's consent only in Unsafe Mode.
func (s *ScheduleAPI) gates(req *schedules.Request, unsafe bool) error {
	// The registry is held to Settings' rule, as a run's is.
	req.Registry = settings.Normalize(req.Registry)
	if err := settings.ValidateURL(req.Registry); err != nil {
		return err
	}
	// A pack is attached by its name in the store, as a run's is: a
	// path-shaped value would name a folder no picker chose.
	for _, k := range req.Knowledge {
		if err := cliknowledge.ValidName(k); err != nil {
			return err
		}
	}
	for _, p := range req.Files {
		if err := s.app.requirePicked("input", p); err != nil {
			return err
		}
	}
	if err := s.app.requireOutputDir(req.OutDir); err != nil {
		return err
	}
	if req.AllowVulnerableMCP && !unsafe {
		return errors.New("a declared MCP server the registry's gate rates high or critical runs on this computer only in Unsafe Mode, and Unsafe Mode is off: turn it on in Settings → Run defaults, or schedule a release that pins a fixed version")
	}
	return nil
}

// Check is the When step's: whether the schedule req describes could be
// made, and every gap a fire would meet. It asks nothing and keeps nothing.
func (s *ScheduleAPI) Check(req schedules.Request) (*schedules.Check, error) {
	if err := s.gates(&req, unsafeModeOn()); err != nil {
		return nil, err
	}
	c, err := s.cli()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	defer cancel()
	if err := s.setDelivery(ctx, c, &req); err != nil {
		return nil, err
	}
	return c.Check(ctx, req)
}

// setDelivery sets where a schedule's fires send, as the person's settings and
// choices have it, never as the page names it. The folder is the Deliverables
// folder in effect, when the switch is on; the page cannot name another. The
// destinations are the saved ones the setup chose, each held to the CLI's
// rule. A command that does not take the flags (older than MinDeliverCLI)
// makes no schedule that needs them: the folder, which nobody chose by hand,
// is left off with a note in the log, and a destination somebody did choose
// refuses.
func (s *ScheduleAPI) setDelivery(ctx context.Context, c *schedules.CLI, req *schedules.Request) error {
	req.DeliverTo = deliverFolder()
	for _, n := range req.Destinations {
		if !destinations.CanSend(n) {
			return fmt.Errorf("the destination %q is not saved with a signing secret: set it up in Settings → Destinations", n)
		}
	}
	if req.DeliverTo == "" && len(req.Destinations) == 0 {
		return nil
	}
	line, err := c.Version(ctx)
	if err != nil {
		return fmt.Errorf("the agentry command did not answer: %w", err)
	}
	if why := schedules.OlderThan(line, schedules.MinDeliverCLI, "a schedule that delivers"); why != "" {
		if len(req.Destinations) > 0 {
			return errors.New(why)
		}
		log.Printf("agentry-desktop: schedule %s: the agentry command is too old to keep where its runs deliver, so the Deliverables folder is not passed: %s", req.Ref, why)
		req.DeliverTo = ""
	}
	return nil
}

// ScheduleMadeView is a schedule just made.
type ScheduleMadeView struct {
	ID string `json:"id"`
}

// Add makes the schedule req describes: checked again, the person's
// consents asked in one native dialog, then made by the agentry command,
// which pins the version, installs it and writes its alarm.
//
// A release that asks for a target host has each host readied first, with
// the person here: a host key no known_hosts file holds is put to them in a
// native dialog, and the sign-in, hostSignIns' for that host or the one
// saved, is kept in the system keychain, where the schedule's fires read it.
// The command then finds both, and pins the key.
func (s *ScheduleAPI) Add(req schedules.Request, hostSignIns map[string]run.HostSignIn) (*ScheduleMadeView, error) {
	unsafe := unsafeModeOn()
	if err := s.gates(&req, unsafe); err != nil {
		return nil, err
	}
	c, err := s.cli()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	if err := s.setDelivery(ctx, c, &req); err != nil {
		cancel()
		return nil, err
	}
	ck, err := c.Check(ctx, req)
	cancel()
	if err != nil {
		return nil, err
	}
	if !ck.Ready {
		return nil, fmt.Errorf("a scheduled run of %s would not have what it needs:\n  - %s", req.Ref, strings.Join(ck.Gaps, "\n  - "))
	}
	yes, err := s.askScheduleConsent(req, ck, unsafe)
	if err != nil {
		return nil, err
	}
	hctx, hcancel := context.WithTimeout(s.ctx(), cliTimeout)
	err = s.readyHosts(hctx, c, req, hostSignIns)
	hcancel()
	if err != nil {
		return nil, err
	}
	actx, acancel := context.WithTimeout(s.ctx(), cliAddTimeout)
	defer acancel()
	id, err := c.Add(actx, req, yes)
	if err != nil {
		return nil, err
	}
	emitEvent(s.app, eventSchedulesChanged)
	return &ScheduleMadeView{ID: id}, nil
}

// readyHosts readies the target hosts of the release req schedules, each
// sign-in kept in the keychain (run.ReadyScheduleHosts). A release that
// asks for none readies nothing; a sign-in given for one refuses. What it
// reports goes to the app's log.
func (s *ScheduleAPI) readyHosts(ctx context.Context, c *schedules.CLI, req schedules.Request, given map[string]run.HostSignIn) error {
	ready := s.readyScheduleHosts
	if ready == nil {
		ready = run.ReadyScheduleHosts
	}
	n, err := ready(ctx, run.ScheduleHosts{
		Registry: req.Registry, Ref: req.Ref, Answers: req.Intake, Given: given,
		Picked: s.app.keyFiles.require, Trust: s.app.trustHostKey,
		Say: func(msg string) { log.Printf("agentry-desktop: schedule %s: %s", req.Ref, msg) },
		// Only an agentry that readies target hosts pins the key a fire
		// holds the host to.
		Before: func() error {
			line, err := c.Version(ctx)
			if err != nil {
				return fmt.Errorf("the agentry command did not answer: %w", err)
			}
			if why := schedules.OlderThan(line, schedules.MinTargetHostsCLI, "scheduling a QuickAgent that asks for a server"); why != "" {
				return errors.New(why)
			}
			return nil
		},
	})
	// A sign-in kept now reaches the schedules that run while the person
	// is signed out, as every other key change does.
	if n > 0 {
		resealSchedules(s.app)
	}
	return err
}

// askScheduleConsent asks, in one native dialog, for every consent making
// the schedule takes: the release's capabilities when this version needs
// them confirmed, the refusals the person lifted in the setup (each run
// will carry them), and turning linger on. It reports whether one was
// given, which the command takes as --yes; nothing to ask asks nothing.
func (s *ScheduleAPI) askScheduleConsent(req schedules.Request, ck *schedules.Check, unsafe bool) (bool, error) {
	var lines []string
	if ck.NeedsConsent {
		why := ck.ConsentReason
		if why == "" {
			why = "its declared capabilities need your consent"
		}
		lines = append(lines, fmt.Sprintf("Its capabilities, as its page lists them (%s). Each scheduled run uses them with nobody there.", why))
	}
	var accepting []string
	if req.AllowUnsigned {
		accepting = append(accepting, "a release with no attestation")
	}
	if req.AllowUnscanned {
		accepting = append(accepting, "a release the registry admitted without a scan")
	}
	if req.AllowVulnerableMCP {
		accepting = append(accepting, "any declared MCP server the registry's gate rates high or critical, which runs only in Unsafe Mode")
	}
	if len(accepting) > 0 {
		lines = append(lines, "Every run of it accepts "+strings.Join(accepting, ", and ")+". Each receipt records the consent.")
	}
	if unsafe && req.AllowVulnerableMCP {
		lines = append(lines, "Unsafe Mode is on: every run of this schedule is an Unsafe Mode run, and its receipt names each server it let through.")
	}
	linger := req.SignedOut && !s.lingers()
	if linger {
		lines = append(lines, lingerSentence+": linger is turned on for you (your distribution may ask for an administrator's password). It is turned off again with the last schedule that runs while you are signed out.")
	}
	if len(lines) == 0 {
		return false, nil
	}
	if err := s.app.askConsent("Schedule "+req.Ref+"?", strings.Join(lines, "\n\n")+"\n\nSchedule it?"); err != nil {
		return false, err
	}
	return ck.NeedsConsent || linger, nil
}

// Edit changes a schedule's name, when it runs, or whether it also runs
// while the person is signed out; turning that on asks first where linger
// is off.
func (s *ScheduleAPI) Edit(id string, e schedules.Edit) error {
	c, err := s.cli()
	if err != nil {
		return err
	}
	yes := false
	if e.SignedOut != nil && *e.SignedOut && !s.lingers() {
		if err := s.app.askConsent("Run this schedule while you are signed out?", lingerSentence+": linger is turned on for you (your distribution may ask for an administrator's password). Its keys are sealed beside it.\n\nTurn it on?"); err != nil {
			return err
		}
		yes = true
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	defer cancel()
	if err := c.EditSchedule(ctx, id, e, yes); err != nil {
		return err
	}
	emitEvent(s.app, eventSchedulesChanged)
	return nil
}

// act runs one of pause, resume and remove.
func (s *ScheduleAPI) act(verb, id string) error {
	c, err := s.cli()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliTimeout)
	defer cancel()
	if err := c.Act(ctx, verb, id); err != nil {
		return err
	}
	emitEvent(s.app, eventSchedulesChanged)
	return nil
}

// Pause stops a schedule from running until Resume; what falls due
// meanwhile is not caught up.
func (s *ScheduleAPI) Pause(id string) error { return s.act("pause", id) }

// Resume lets a paused schedule run again.
func (s *ScheduleAPI) Resume(id string) error { return s.act("resume", id) }

// Remove deletes a schedule after a native Yes/No (the rule for what cannot
// be undone); its runs' folders are kept. A No is no error.
func (s *ScheduleAPI) Remove(id, name string) (bool, error) {
	if err := s.app.askConsent("Delete the schedule "+name+"?", "It will not run again, and its alarm goes from this computer's scheduler. The folders its runs wrote are kept.\n\nDelete it?"); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return false, nil
		}
		return false, err
	}
	return true, s.act("remove", id)
}

// RunNow runs a schedule once, now, as an ordinary run that uses up no
// occurrence; the agentry command runs it, so it goes on if the app is
// closed. The page hears schedules:changed when it ends.
func (s *ScheduleAPI) RunNow(id string) error {
	if !schedule.ValidID(id) {
		return fmt.Errorf("%q is not a schedule id", id)
	}
	c, err := s.cli()
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.running[id] {
		s.mu.Unlock()
		return errors.New("this schedule is already running now")
	}
	if s.running == nil {
		s.running = map[string]bool{}
	}
	s.running[id] = true
	s.mu.Unlock()
	emitEvent(s.app, eventSchedulesChanged)
	go func() {
		// Not the app's context: quitting the app does not stop the run,
		// which the command records in the schedule's history.
		err := c.Act(context.Background(), "run", id)
		if err != nil {
			log.Printf("agentry-desktop: schedule %s: run now: %v", id, err)
		}
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
		emitEvent(s.app, eventSchedulesChanged)
	}()
	return nil
}

// start rewrites every schedule's alarm from the store once the app is up:
// an entry removed by hand, or written by an older agentry, comes back.
// Nothing is done where no schedule exists.
func (s *ScheduleAPI) start(ctx context.Context) {
	go func() {
		st, err := s.store()
		if err != nil || !st.Any() {
			return
		}
		c, err := installedCLI(ctx)
		if err != nil {
			log.Printf("agentry-desktop: schedules: %v", err)
			return
		}
		sctx, cancel := context.WithTimeout(ctx, cliTimeout)
		defer cancel()
		if err := c.Sync(sctx); err != nil {
			log.Printf("agentry-desktop: schedules: sync: %v", err)
		}
	}()
}

// resealCLI is the command a re-seal runs; a test replaces it.
var resealCLI = installedCLI

// resealing coalesces re-seals: one runs at a time, and a key change made
// while one runs asks for one more after it, which reads the keychain as
// it then stands.
var (
	resealing   sync.Mutex
	resealState struct {
		sync.Mutex
		running, again bool
	}
)

// resealSchedules re-seals in the background, so a key change returns at
// once (the command may take a while), and coalesces the changes made
// while one runs into one more. A variable so a test can see that a key
// change asks for it.
var resealSchedules = func(app *App) {
	resealState.Lock()
	if resealState.running {
		resealState.again = true
		resealState.Unlock()
		return
	}
	resealState.running = true
	resealState.Unlock()
	go func() {
		for {
			resealNow(app)
			resealState.Lock()
			if !resealState.again {
				resealState.running = false
				resealState.Unlock()
				return
			}
			resealState.again = false
			resealState.Unlock()
		}
	}()
}

// resealNow seals the keys of every schedule that runs while the person is
// signed out again, after a key or a sign-in changed in this app (the CLI
// does it after setup, login and logout). What fails is said on the
// Scheduled page and to the page that made the change, never fatal: the
// change it follows is made. A schedule whose keys were not sealed again
// loses the old ones (the command's rule) and shows so.
func resealNow(app *App) {
	resealing.Lock()
	defer resealing.Unlock()
	home, err := run.Home()
	if err != nil || !schedules.StoreIn(home).SignedOutLive() {
		return
	}
	ctx := context.Background()
	if app != nil && app.ctx != nil {
		ctx = app.ctx
	}
	c, err := resealCLI(ctx)
	problem := ""
	if err != nil {
		problem = "The keys of your schedules that run while you are signed out were not sealed again: " + err.Error() + ". They run with the keys sealed before until they are."
	} else {
		rctx, cancel := context.WithTimeout(ctx, cliTimeout)
		res, err := c.Reseal(rctx)
		cancel()
		switch {
		case err != nil:
			problem = "The keys of your schedules that run while you are signed out were not sealed again: " + err.Error()
		case len(res.Failed) > 0:
			var names []string
			for _, f := range res.Failed {
				names = append(names, f.Name+" ("+f.Error+")")
			}
			problem = "The keys of " + strings.Join(names, ", ") + " were not sealed again, so the keys sealed before were removed: it does not run while you are signed out until you edit it and turn Also run when I'm signed out on again."
		}
	}
	resealMu.Lock()
	resealProblem = problem
	resealMu.Unlock()
	if problem != "" {
		log.Printf("agentry-desktop: schedules: %s", problem)
		emitEvent(app, eventSchedulesReseal, problem)
	}
	emitEvent(app, eventSchedulesChanged)
}
