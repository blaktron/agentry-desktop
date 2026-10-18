// Package schedules is the desktop's side of schedules on this computer.
//
// The CLI owns them. Its store under the runner home is the truth, and each
// schedule's entry in the system's own scheduler pins the absolute path of
// the agentry binary that wrote it (a systemd user timer, a LaunchAgent, a
// Task Scheduler task). So every change goes through the installed agentry
// command (the Windows install's bin\agentry.exe; the site's install
// script's ~/.local/bin/agentry; the Linux package's /usr/bin/agentry),
// never through this process: an entry written
// from here would name the desktop's own executable. What the page shows is
// read in process, through the CLI's pkg/schedule: the schedules, what each
// fire did (fires.ndjson) and the occurrences to come.
package schedules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/cliinstall"
)

// MinCLI is the oldest agentry that has what the desktop drives: add
// --check, reseal and check-signed-out.
var MinCLI = [3]int{0, 6, 86}

// MinTargetHostsCLI is the oldest agentry whose schedule add readies a
// target host and pins its key: a QuickAgent that asks for a server is
// scheduled only through it.
var MinTargetHostsCLI = [3]int{0, 6, 111}

// MinDeliverCLI is the oldest agentry whose schedule add keeps where its fires
// deliver (--deliver-to, --destination).
var MinDeliverCLI = [3]int{0, 6, 139}

// MinBucketCLI is the oldest agentry that sends to a bucket: an older one
// refuses a destinations file that holds one, as a whole.
var MinBucketCLI = [3]int{0, 6, 140}

// ErrNoCLI says why scheduling on this computer is not offered here.
var ErrNoCLI = errors.New("scheduling a run on this computer needs the agentry command, which was not found")

// goos, executable, getenv, lookPath and statFile are replaced in tests.
var (
	goos       = runtime.GOOS
	executable = os.Executable
	getenv     = os.Getenv
	lookPath   = exec.LookPath
	statFile   = os.Stat
)

// linuxPackageCLI is where the Linux packages install the command.
const linuxPackageCLI = "/usr/bin/agentry"

// FindCLI is the installed agentry this app drives. On Windows, the one its
// installer put down. On macOS and Linux, the one in ~/.local/bin, which an
// app's PATH often lacks: on macOS the link the app makes there to the CLI
// its bundle carries (or a copy the site's install script put there), and
// failing that the bundle's own; on Linux the script's copy, which the
// Scheduled page's Options menu installs and reinstalls
// (internal/cliinstall), before the package's /usr/bin/agentry, as on the
// person's own PATH, so a reinstall to the site's latest is the one used.
// Else the one on PATH. A schedule pins the path FindCLI gives, so the link
// in ~/.local/bin, which outlives an update that replaces the bundle, comes
// first.
func FindCLI() (string, error) {
	var candidates []string
	switch goos {
	case "windows":
		if exe, err := executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "bin", "agentry.exe"))
		}
	case "darwin":
		if p := cliinstall.Path(getenv); p != "" {
			candidates = append(candidates, p)
		}
		if exe, err := executable(); err == nil {
			if p := cliinstall.Carried(exe); p != "" && cliinstall.Installed(p) {
				candidates = append(candidates, p)
			}
		}
	case "linux":
		if p := cliinstall.Path(getenv); p != "" {
			candidates = append(candidates, p)
		}
		candidates = append(candidates, linuxPackageCLI)
	}
	for _, c := range candidates {
		if info, err := statFile(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	if p, err := lookPath("agentry"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs, nil
		}
	}
	return "", ErrNoCLI
}

// versionPattern finds the version in `agentry --version`.
var versionPattern = regexp.MustCompile(`\bv?(\d+)\.(\d+)\.(\d+)`)

// TooOld reports why the version line of an agentry is older than MinCLI,
// or "" when it is new enough or names no version (a build from source).
func TooOld(versionLine string) string {
	return OlderThan(versionLine, MinCLI, "scheduling a run on this computer")
}

// OlderThan reports why the version line of an agentry is older than min,
// which what needs, or "" when it is new enough or names no version.
func OlderThan(versionLine string, min [3]int, what string) string {
	m := versionPattern.FindStringSubmatch(versionLine)
	if m == nil {
		return ""
	}
	var got [3]int
	for i := range got {
		got[i], _ = strconv.Atoi(m[i+1])
	}
	for i := range got {
		if got[i] != min[i] {
			if got[i] < min[i] {
				return fmt.Sprintf("%s needs agentry v%d.%d.%d or later, and the agentry command found is %s.%s.%s: update it", what, min[0], min[1], min[2], m[1], m[2], m[3])
			}
			return ""
		}
	}
	return ""
}

// CLI runs the installed agentry.
type CLI struct {
	Path string
	// Exec runs it; tests replace it. stdin is empty: the command never
	// waits on a question.
	Exec func(ctx context.Context, path string, args []string) (stdout, stderr []byte, err error)
}

// New is a CLI that runs path.
func New(path string) *CLI { return &CLI{Path: path, Exec: execCLI} }

func execCLI(ctx context.Context, path string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(nil)
	hideWindow(cmd)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	return out.Bytes(), errOut.Bytes(), err
}

// Run runs agentry args and returns its stdout; a failure carries what it
// said on stderr, which is its reason in its own words.
func (c *CLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	out, errOut, err := c.Exec(ctx, c.Path, args)
	if err != nil {
		said := reason(string(errOut))
		if said == "" {
			said = err.Error()
		}
		return out, &Error{Said: said, Err: err}
	}
	return out, nil
}

// Error is a refusal or failure of the agentry command.
type Error struct {
	Said string
	Err  error
}

func (e *Error) Error() string { return e.Said }
func (e *Error) Unwrap() error { return e.Err }

// reason is the command's reason in its own words: what it printed from
// its last "agentry: " line on (main prints its error so, gaps on the
// lines after it), without the progress the run step printed before, else
// the last lines it printed.
func reason(stderr string) string {
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if rest, ok := strings.CutPrefix(lines[i], "agentry: "); ok && !strings.HasPrefix(rest, "warning: ") {
			return strings.TrimSpace(strings.Join(append([]string{rest}, lines[i+1:]...), "\n"))
		}
	}
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// When is the run setup's When step, as the page sends it.
type When struct {
	// Kind is once, daily, weekdays, weekly or monthly.
	Kind string `json:"kind"`
	// Date is a once schedule's day (YYYY-MM-DD).
	Date string `json:"date,omitempty"`
	// Time is HH:MM, 24-hour.
	Time string `json:"time"`
	// Days are a weekly schedule's (mon … sun).
	Days     []string `json:"days,omitempty"`
	MonthDay int      `json:"monthDay,omitempty"`
	// End is never, on or after.
	End   string `json:"end,omitempty"`
	Until string `json:"until,omitempty"`
	Times int    `json:"times,omitempty"`
	// Zone is an IANA zone; "" is this computer's.
	Zone string `json:"zone,omitempty"`
}

// hhmm is a time as the CLI takes it.
var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ymd is a date as the CLI takes it.
var ymd = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// zonePattern is what an IANA zone name looks like; the CLI checks it is
// one.
var zonePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-]*(/[A-Za-z0-9_+\-]+)*$`)

// Args is w as `agentry schedule add` and `edit` take it.
func (w When) Args() ([]string, error) {
	if !hhmm.MatchString(w.Time) {
		return nil, fmt.Errorf("the time %q is not HH:MM", w.Time)
	}
	var args []string
	switch w.Kind {
	case "once":
		if !ymd.MatchString(w.Date) {
			return nil, fmt.Errorf("the date %q is not YYYY-MM-DD", w.Date)
		}
		args = []string{"--once", w.Date + " " + w.Time}
	case "daily":
		args = []string{"--daily", w.Time}
	case "weekdays":
		args = []string{"--weekdays", w.Time}
	case "weekly":
		days, err := schedule.ParseDays(strings.Join(w.Days, ","))
		if err != nil {
			return nil, err
		}
		args = []string{"--weekly", strings.Join(days, ","), "--at", w.Time}
	case "monthly":
		if w.MonthDay < 1 || w.MonthDay > 31 {
			return nil, fmt.Errorf("the day of the month %d is not 1 to 31", w.MonthDay)
		}
		args = []string{"--monthly", strconv.Itoa(w.MonthDay), "--at", w.Time}
	default:
		return nil, fmt.Errorf("unknown repeat %q", w.Kind)
	}
	if w.Kind != "once" {
		switch w.End {
		case "", "never":
		case "on":
			if !ymd.MatchString(w.Until) {
				return nil, fmt.Errorf("the end date %q is not YYYY-MM-DD", w.Until)
			}
			args = append(args, "--until", w.Until)
		case "after":
			if w.Times < 1 {
				return nil, errors.New("a repeat that ends after a number of runs needs at least one")
			}
			args = append(args, "--times", strconv.Itoa(w.Times))
		default:
			return nil, fmt.Errorf("unknown end %q", w.End)
		}
	}
	if w.Zone != "" {
		if !zonePattern.MatchString(w.Zone) {
			return nil, fmt.Errorf("%q is not a time zone name", w.Zone)
		}
		args = append(args, "--zone", w.Zone)
	}
	return args, nil
}

// Request is a schedule as the run setup describes it: what to run, how
// and when.
type Request struct {
	Registry string `json:"registry"`
	// Ref is ns/name@version: the version the setup shows, which the CLI
	// pins with its digest.
	Ref    string `json:"ref"`
	Name   string `json:"name,omitempty"`
	OutDir string `json:"outDir"`
	// Task is kept with the schedule (task.txt); "" runs the agent's own
	// instruction.
	Task      string            `json:"task,omitempty"`
	Files     []string          `json:"files,omitempty"`
	Knowledge []string          `json:"knowledge,omitempty"`
	Intake    map[string]string `json:"intake,omitempty"`
	// Voice: true attaches this computer's Writing Voice, false leaves it
	// off, as the setup's box says.
	Voice          bool `json:"voice"`
	TimeoutSeconds int  `json:"timeoutSeconds,omitempty"`
	// MaxTurns is -1 for the policy's.
	MaxTurns           int  `json:"maxTurns"`
	AllowUnsigned      bool `json:"allowUnsigned,omitempty"`
	AllowUnscanned     bool `json:"allowUnscanned,omitempty"`
	AllowVulnerableMCP bool `json:"allowVulnerableMcp,omitempty"`
	SignedOut          bool `json:"signedOut,omitempty"`
	// DeliverTo is the absolute folder each fire copies its deliverables to
	// (`--deliver-to`), Destinations the saved destinations each fire sends its
	// result to (`--destination`, by name). Either needs an agentry that
	// takes the flags (MinDeliverCLI).
	DeliverTo    string   `json:"deliverTo,omitempty"`
	Destinations []string `json:"destinations,omitempty"`
	When         When     `json:"when"`
}

// refPattern is ns/name@version.
var refPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*@[0-9A-Za-z.+\-]+$`)

// intakeID is an intake question's id.
var intakeID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// AddArgs is r as `agentry schedule add` takes it, after the global flags.
func (r Request) AddArgs() ([]string, error) {
	if !refPattern.MatchString(r.Ref) {
		return nil, fmt.Errorf("%q is not a QuickAgent version (ns/name@version)", r.Ref)
	}
	if r.OutDir == "" {
		return nil, errors.New("a schedule needs an output folder: each scheduled run gets its own folder in it")
	}
	when, err := r.When.Args()
	if err != nil {
		return nil, err
	}
	args := append([]string{"schedule", "add", r.Ref, "--out", r.OutDir}, when...)
	if r.Name != "" {
		args = append(args, "--name", r.Name)
	}
	if r.Task != "" {
		args = append(args, "--task", r.Task)
	}
	for _, f := range r.Files {
		args = append(args, "--file", f)
	}
	for _, k := range r.Knowledge {
		args = append(args, "--knowledge", k)
	}
	ids := make([]string, 0, len(r.Intake))
	for id := range r.Intake {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !intakeID.MatchString(id) {
			return nil, fmt.Errorf("%q is not an intake question's id", id)
		}
		args = append(args, "--intake", id+"="+r.Intake[id])
	}
	if r.Voice {
		args = append(args, "--voice")
	} else {
		args = append(args, "--no-voice")
	}
	if r.TimeoutSeconds > 0 {
		args = append(args, "--timeout", strconv.Itoa(r.TimeoutSeconds))
	}
	if r.MaxTurns >= 0 {
		args = append(args, "--max-turns", strconv.Itoa(r.MaxTurns))
	}
	if r.AllowUnsigned {
		args = append(args, "--allow-unsigned")
	}
	if r.AllowUnscanned {
		args = append(args, "--allow-unscanned")
	}
	if r.AllowVulnerableMCP {
		args = append(args, "--allow-vulnerable-mcp")
	}
	if r.SignedOut {
		args = append(args, "--signed-out")
	}
	if r.DeliverTo != "" {
		if !filepath.IsAbs(r.DeliverTo) {
			return nil, fmt.Errorf("the delivery folder %q is not an absolute path", r.DeliverTo)
		}
		args = append(args, "--deliver-to", r.DeliverTo)
	}
	for _, d := range r.Destinations {
		if !destination.ValidName(d) {
			return nil, fmt.Errorf("%q is not a destination's name", d)
		}
		args = append(args, "--destination", d)
	}
	return args, nil
}

// Check is what `agentry schedule add --check --json` says.
type Check struct {
	Ref           string    `json:"ref"`
	Version       string    `json:"version"`
	Digest        string    `json:"digest"`
	Modes         []string  `json:"modes"`
	Ready         bool      `json:"ready"`
	Gaps          []string  `json:"gaps"`
	NeedsConsent  bool      `json:"needsConsent"`
	ConsentReason string    `json:"consentReason,omitempty"`
	Refusals      []Refusal `json:"refusals"`
	Installed     bool      `json:"installed"`
	SignedOut     SignedOut `json:"signedOut"`
	Describe      string    `json:"describe,omitempty"`
	Next          string    `json:"next,omitempty"`
}

// Refusal is one refusal a run would meet.
type Refusal struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Lift    string `json:"lift,omitempty"`
}

// SignedOut is whether this computer can run schedules while its person
// is signed out.
type SignedOut struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// globals are the flags every call carries: the registry the setup shows,
// and machine-readable output.
func globals(registry string) []string {
	g := []string{"--json"}
	if registry != "" {
		g = append(g, "--registry", registry)
	}
	return g
}

// Check asks whether r could be made, and keeps nothing.
func (c *CLI) Check(ctx context.Context, r Request) (*Check, error) {
	args, err := r.AddArgs()
	if err != nil {
		return nil, err
	}
	out, err := c.Run(ctx, append(append(globals(r.Registry), args...), "--check")...)
	if err != nil {
		return nil, err
	}
	var res Check
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("read the agentry command's check: %w", err)
	}
	return &res, nil
}

// Add makes r. yes is the consent the person gave in the app's own dialog
// (the release's capabilities, linger), passed as --yes.
func (c *CLI) Add(ctx context.Context, r Request, yes bool) (string, error) {
	args, err := r.AddArgs()
	if err != nil {
		return "", err
	}
	args = append(globals(r.Registry), args...)
	if yes {
		args = append(args, "--yes")
	}
	out, err := c.Run(ctx, args...)
	if err != nil {
		return "", err
	}
	var made struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &made); err != nil || !schedule.ValidID(made.ID) {
		return "", fmt.Errorf("the agentry command made the schedule but did not say its id: %s", strings.TrimSpace(string(out)))
	}
	return made.ID, nil
}

// Edit is what can change on a schedule from the page: its name, when it
// runs, and whether it also runs signed out.
type Edit struct {
	Name      string `json:"name,omitempty"`
	When      *When  `json:"when,omitempty"`
	SignedOut *bool  `json:"signedOut,omitempty"`
}

// EditArgs is e for schedule id as `agentry schedule edit` takes it.
func (e Edit) EditArgs(id string) ([]string, error) {
	if !schedule.ValidID(id) {
		return nil, fmt.Errorf("%q is not a schedule id", id)
	}
	args := []string{"schedule", "edit", id}
	if e.Name != "" {
		args = append(args, "--name", e.Name)
	}
	if e.When != nil {
		w, err := e.When.Args()
		if err != nil {
			return nil, err
		}
		args = append(args, w...)
	}
	if e.SignedOut != nil {
		args = append(args, "--signed-out="+strconv.FormatBool(*e.SignedOut))
	}
	if len(args) == 3 {
		return nil, errors.New("nothing to change")
	}
	return args, nil
}

// EditSchedule changes schedule id; yes confirms turning linger on.
func (c *CLI) EditSchedule(ctx context.Context, id string, e Edit, yes bool) error {
	args, err := e.EditArgs(id)
	if err != nil {
		return err
	}
	args = append(globals(""), args...)
	if yes {
		args = append(args, "--yes")
	}
	_, err = c.Run(ctx, args...)
	return err
}

// Act runs pause, resume, remove or run on schedule id.
func (c *CLI) Act(ctx context.Context, verb, id string) error {
	switch verb {
	case "pause", "resume", "remove", "run":
	default:
		return fmt.Errorf("unknown schedule action %q", verb)
	}
	if !schedule.ValidID(id) {
		return fmt.Errorf("%q is not a schedule id", id)
	}
	_, err := c.Run(ctx, append(globals(""), "schedule", verb, id)...)
	return err
}

// Sync rewrites every schedule's alarm from the store (the desktop does
// it at every start).
func (c *CLI) Sync(ctx context.Context) error {
	_, err := c.Run(ctx, "schedule", "sync")
	return err
}

// SignedOutHere is whether this computer can run schedules while its
// person is signed out.
func (c *CLI) SignedOutHere(ctx context.Context) (SignedOut, error) {
	out, err := c.Run(ctx, "--json", "schedule", "check-signed-out")
	if err != nil {
		return SignedOut{}, err
	}
	var s SignedOut
	if err := json.Unmarshal(out, &s); err != nil {
		return SignedOut{}, fmt.Errorf("read the agentry command's answer: %w", err)
	}
	return s, nil
}

// Resealed is what `agentry schedule reseal --json` says.
type Resealed struct {
	Sealed []string `json:"sealed"`
	Failed []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Error string `json:"error"`
	} `json:"failed"`
}

// Reseal seals every signed-out schedule's keys again from the keychain.
// A schedule that failed is in the answer, and the command's exit is not
// an error here: the answer says which.
func (c *CLI) Reseal(ctx context.Context) (*Resealed, error) {
	out, _, err := c.Exec(ctx, c.Path, []string{"--json", "schedule", "reseal"})
	var r Resealed
	if jerr := json.Unmarshal(bytes.TrimSpace(out), &r); jerr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("read the agentry command's reseal: %w", jerr)
	}
	return &r, nil
}

// Version is the command's own version line.
func (c *CLI) Version(ctx context.Context) (string, error) {
	out, err := c.Run(ctx, "--version")
	return strings.TrimSpace(firstLine(string(out))), err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
