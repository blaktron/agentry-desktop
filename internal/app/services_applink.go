// The app link's answers: the tools the runner's app link lists for Agentry
// Helper (Main), which the Assistant's lists too, each answered here with the
// operation the page would use for it. The CLI's supervisor hands every call
// of these tools to this handler, one at a time per connection, from a
// server with no network of its own; nothing here changes a setting's value,
// starts or stops an agent, publishes, signs in or touches a credential.
//
// What an agent asks for is a claim, as a page's is: an install, a first
// run of a QuickAgent, a schedule and a schedule's removal are each put to
// the person in the app's native dialog, which names the agent asking, and a
// No does nothing. Each agent has a handler of its own: a run it starts
// takes its files from its own share folder alone, and the runs the person
// agreed to let it start are its own, so a Yes for one agent is never a Yes
// for another. A run is an ordinary local run under the run lock: it is in
// Recent runs and writes to the output folder and the Deliverables folder.
// What the registry and the docs say reaches the agent as quoted data.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/schedule"
	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventOpenSettings asks the page to open a Settings section: the one thing
// settings_open does.
const eventOpenSettings = "applink:open-settings"

// eventHomeChanged tells Home that an install or a run the agent asked for
// changed what its cards read, so it reads them again.
const eventHomeChanged = "applink:home-changed"

// mainAgent is Main's name, its built-in's.
const mainAgent = "main"

// mainAsks is how a dialog names Main as the one asking.
const mainAsks = "Agentry Helper"

// maxAskerName bounds the name an agent gave itself in a dialog's words.
const maxAskerName = 40

// docsBase is the documentation site docs_read reads from, and nothing else.
const docsBase = "https://docs.secureagentry.net"

const (
	// maxSearchHits is how many matches registry_search answers.
	maxSearchHits = 8
	// maxRunFiles bounds the files one quickagent_run takes.
	maxRunFiles = 20
	// resultWait bounds how long run_result waits for a run Main started.
	resultWait = 10 * time.Minute
	// maxDocsBytes bounds a docs page read; maxDocsText the text answered.
	maxDocsBytes = 2 << 20
	maxDocsText  = 24 << 10
	// maxResultText bounds a run's result in run_result's answer.
	maxResultText = 8 << 10
	// docsTimeout bounds one docs page read.
	docsTimeout = 20 * time.Second
)

// mainOps are the app's own operations Main's tools reach: the app's in
// appMainOps, a test's its own.
type mainOps interface {
	registry() string
	outputDir() string
	search(reg string, q SearchQuery) (*SearchView, error)
	agent(reg, ns, name string) (*AgentDetailView, error)
	report(reg, ns, name, version string) (*ReportView, error)
	requirements(reg, ns, name string) (*MCPView, error)
	// usage is what the run setup reads of a release before a run: its
	// questions, the files it takes, its keys and its guide.
	usage(reg, ref string) (*run.ProfileInfo, error)
	installed() ([]install.Item, error)
	install(reg, ref, who string) (*install.Result, error)
	run(reg, ref, task string, inputs []string, intake map[string]string, share, outDir string, began func(id string, queued bool)) (*RunResultView, error)
	recorded(runID string) (*RecordedRunView, error)
	schedules() (*SchedulesView, error)
	addSchedule(req schedules.Request, who string) (*ScheduleMadeView, error)
	pauseSchedule(id string, paused bool) error
	removeSchedule(id, name, who string) (bool, error)
	docs(ctx context.Context, page string) ([]byte, int, error)
}

// mainLink answers one agent's calls of Agentry Helper's tools: Main's, or
// the Assistant's.
type mainLink struct {
	app   *App
	ops   mainOps
	share func() (string, error)
	// agreedPath is where the runs the person agreed to let this agent
	// start are kept, by ns/name@version.
	agreedPath func() (string, error)
	// asks is how a dialog names the agent asking; nil is Main's name.
	asks func() string
	// meetings answers the Assistant's meeting tools; nil where this link
	// does not.
	meetings func(ctx context.Context, tool string, raw json.RawMessage, who string) (string, error)

	mu   sync.Mutex
	runs map[string]*mainRun
}

// mainRun is a run Main started in this app: its outcome once it ends.
type mainRun struct {
	ref  string
	done chan struct{}
	res  *RunResultView
	err  error
}

// who is how a dialog names the agent asking.
func (m *mainLink) who() string {
	if m.asks == nil {
		return mainAsks
	}
	return m.asks()
}

// askerName is how a dialog names an agent: its title, which no one can
// change, then the name it goes by when that differs, quoted and cut, since
// the agent can set that name itself and must not pass for someone else.
func askerName(title, name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == title {
		return title
	}
	return title + " (" + strconv.Quote(cut(name, maxAskerName)) + ")"
}

// answer answers one of the agent's tool calls.
func (m *mainLink) answer(ctx context.Context, req applink.Request) applink.Answer {
	text, err := m.call(ctx, req.Tool, req.Arguments)
	switch {
	case errors.Is(err, errNotConfirmed):
		return applink.Answer{Text: "Not done: the person said no in Agentry.", IsError: true}
	case err != nil:
		return applink.Answer{Text: err.Error(), IsError: true}
	}
	return applink.Answer{Text: text}
}

// args decodes a call's arguments into v, refusing a field the tool does
// not take.
func args(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("the arguments do not fit the tool: %w", err)
	}
	return nil
}

func (m *mainLink) call(ctx context.Context, tool string, raw json.RawMessage) (string, error) {
	if applink.IsMeetingTool(tool) && m.meetings != nil {
		return m.meetings(ctx, tool, raw, m.who())
	}
	switch tool {
	case "registry_search":
		var a struct {
			Query string `json:"query"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.search(a.Query)
	case "quickagent_show":
		var a struct {
			Ref string `json:"ref"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.show(a.Ref)
	case "quickagent_install":
		var a struct {
			Ref string `json:"ref"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.install(a.Ref)
	case "quickagent_run":
		var a struct {
			Ref     string            `json:"ref"`
			Task    string            `json:"task"`
			Files   []string          `json:"files"`
			Answers map[string]string `json:"answers"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.run(ctx, a.Ref, a.Task, a.Files, a.Answers)
	case "run_result":
		var a struct {
			Run string `json:"run"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.result(ctx, a.Run)
	case "installed_list":
		if err := args(raw, &struct{}{}); err != nil {
			return "", err
		}
		return m.installedList()
	case "schedule_add":
		var a struct {
			Ref     string            `json:"ref"`
			When    string            `json:"when"`
			Task    string            `json:"task"`
			Answers map[string]string `json:"answers"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.scheduleAdd(a.Ref, a.When, a.Task, a.Answers)
	case "schedule_list":
		if err := args(raw, &struct{}{}); err != nil {
			return "", err
		}
		return m.scheduleList()
	case "schedule_pause":
		var a struct {
			Schedule string `json:"schedule"`
			Paused   *bool  `json:"paused"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		if a.Paused == nil {
			return "", errors.New("say paused: true to pause it or false to resume it")
		}
		return m.schedulePause(a.Schedule, *a.Paused)
	case "schedule_remove":
		var a struct {
			Schedule string `json:"schedule"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.scheduleRemove(a.Schedule)
	case "settings_open":
		var a struct {
			Section string `json:"section"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.settingsOpen(a.Section)
	case "docs_read":
		var a struct {
			Page string `json:"page"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.docsRead(ctx, a.Page)
	}
	return "", fmt.Errorf("this app does not answer %s", tool)
}

// quoted is text a publisher or a page wrote, as Main is given it: quoted,
// so it reads as data and never as Agentry's own words.
func quoted(s string) string { return strconv.Quote(strings.TrimSpace(s)) }

// refPart is one part of a QuickAgent's reference.
var refPart = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// versionPattern is a version as a reference names it.
var versionPattern = regexp.MustCompile(`^[0-9A-Za-z.+\-]{1,64}$`)

// parseRef reads namespace/name, or namespace/name@version.
func parseRef(ref string) (ns, name, version string, err error) {
	ref = strings.TrimSpace(ref)
	base, version, _ := strings.Cut(ref, "@")
	ns, name, ok := strings.Cut(base, "/")
	if !ok || !refPart.MatchString(ns) || !refPart.MatchString(name) || (version != "" && !versionPattern.MatchString(version)) {
		return "", "", "", fmt.Errorf("%s is not a QuickAgent's reference: write namespace/name, or namespace/name@version", quoted(ref))
	}
	return ns, name, version, nil
}

// installedHere is the runner home's install of ns/name, if there is one.
func (m *mainLink) installedHere(ns, name string) (*install.Item, error) {
	items, err := m.ops.installed()
	if err != nil {
		return nil, err
	}
	for i := range items {
		it := items[i]
		if it.Target == installdir.Target && it.Namespace == ns && it.Name == name && it.Present {
			return &it, nil
		}
	}
	return nil, nil
}

func (m *mainLink) search(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 200 {
		return "", errors.New("say what the QuickAgent should do, in a few words (at most 200 characters)")
	}
	reg := m.ops.registry()
	v, err := m.ops.search(reg, SearchQuery{Q: query, Limit: maxSearchHits * 2})
	if err != nil {
		return "", err
	}
	items, _ := m.ops.installed()
	var b strings.Builder
	n := 0
	for _, h := range v.Results {
		if n == maxSearchHits {
			break
		}
		// A persistent release is a built-in's, which no search can install.
		if h.Quarantine != "" || (slices.Contains(h.Modes, "persistent") && !slices.Contains(h.Modes, "autonomous") && !slices.Contains(h.Modes, "interactive")) {
			continue
		}
		n++
		fmt.Fprintf(&b, "- %s/%s (%s; %d installs in 30 days)", h.Namespace, h.Name, strings.Join(h.Modes, ", "), h.Installs30d)
		for _, it := range items {
			if it.Target == installdir.Target && it.Namespace == h.Namespace && it.Name == h.Name {
				fmt.Fprintf(&b, ", installed here at %s", it.Version)
				break
			}
		}
		fmt.Fprintf(&b, ": %s\n", quoted(h.Summary))
	}
	if n == 0 {
		return fmt.Sprintf("No QuickAgent on %s matches %s.", settings.ShownURL(reg), quoted(query)), nil
	}
	return fmt.Sprintf("QuickAgents on %s matching %s (each summary is its publisher's words):\n%s", settings.ShownURL(reg), quoted(query), b.String()), nil
}

func (m *mainLink) show(ref string) (string, error) {
	ns, name, _, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	reg := m.ops.registry()
	d, err := m.ops.agent(reg, ns, name)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s on %s\n", ns, name, settings.ShownURL(reg))
	fmt.Fprintf(&b, "Summary (its publisher's words): %s\n", quoted(d.Agent.Summary))
	fmt.Fprintf(&b, "Modes: %s\n", strings.Join(d.Agent.Modes, ", "))
	if d.Agent.Quarantine != "" {
		fmt.Fprintf(&b, "Quarantined (%s): it cannot be installed.\n", d.Agent.Quarantine)
	}
	if d.Latest == "" {
		b.WriteString("No version is published.\n")
	} else {
		fmt.Fprintf(&b, "Latest version: %s\n", d.Latest)
	}
	it, _ := m.installedHere(ns, name)
	if it != nil {
		fmt.Fprintf(&b, "Installed on this computer: %s\n", it.Version)
	} else {
		b.WriteString("Not installed on this computer.\n")
	}
	// How to run it is the installed version's when there is one, since
	// that is the version a run takes, else the latest's.
	var usage *run.ProfileInfo
	var usageErr error
	switch {
	case it != nil && !it.Local:
		usage, usageErr = m.ops.usage(it.Origin, ns+"/"+name+"@"+it.Version)
	case d.Latest != "":
		usage, usageErr = m.ops.usage(reg, ns+"/"+name+"@"+d.Latest)
	}
	if d.Latest != "" {
		if req, err := m.ops.requirements(reg, ns, name); err == nil {
			if len(req.CapabilityLines) > 0 {
				b.WriteString("What it may reach:\n")
				for _, l := range req.CapabilityLines {
					fmt.Fprintf(&b, "- %s\n", l)
				}
			}
			for _, s := range req.Servers {
				fmt.Fprintf(&b, "- MCP server: %s\n", s)
			}
			if len(req.SecretNames) > 0 && usage == nil {
				fmt.Fprintf(&b, "- Keys it needs: %s\n", strings.Join(req.SecretNames, ", "))
			}
		}
		if r, err := m.ops.report(reg, ns, name, d.Latest); err == nil {
			b.WriteString(securityLine(r))
		}
	}
	switch {
	case usage != nil:
		b.WriteString(usageText(usage))
	case usageErr != nil:
		fmt.Fprintf(&b, "How to run it: Agentry could not read it (%v). Show it again before a run.\n", usageErr)
	}
	return b.String(), nil
}

// securityLine is a version's security report in a line or two: what the
// scan found by severity, and no verdict beyond it.
func securityLine(r *ReportView) string {
	switch r.Verdict {
	case "unscanned":
		return "Security report: the registry admitted this version without a scan.\n"
	case "":
		return "Security report: the scan has not finished.\n"
	}
	if len(r.FindingCounts) == 0 {
		return fmt.Sprintf("Security report: scanned (ruleset %s); no findings were listed.\n", r.RulesetVersion)
	}
	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "low", "info"} {
		if n := r.FindingCounts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, sev))
		}
	}
	return fmt.Sprintf("Security report: scanned (ruleset %s); findings: %s.\n", r.RulesetVersion, strings.Join(parts, ", "))
}

func (m *mainLink) install(ref string) (string, error) {
	ns, name, version, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	if it, err := m.installedHere(ns, name); err != nil {
		return "", err
	} else if it != nil && (version == "" || version == it.Version) {
		return fmt.Sprintf("%s/%s is already installed on this computer, at %s.", ns, name, it.Version), nil
	}
	res, err := m.ops.install(m.ops.registry(), ref, m.who())
	if err != nil {
		return "", err
	}
	emitEvent(m.app, eventHomeChanged)
	out := "Installed: " + res.Message
	if res.Warning != "" {
		out += " (" + res.Warning + ")"
	}
	return out, nil
}

// shareFile is a file Main names (share/…) as a path in its share folder.
func shareFile(share, name string) (string, error) {
	rel := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(name), "/agent/"), "share/")
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return "", fmt.Errorf("%s is not a file in the share folder: name it as share/<file>", quoted(name))
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("%s is not a file in the share folder", quoted(name))
	}
	p := filepath.Join(share, clean)
	if err := fileIn(share, p); err != nil {
		return "", fmt.Errorf("%s: %w", quoted(name), err)
	}
	return p, nil
}

// fileIn refuses p unless it is a regular file inside dir, named by its own
// path: no link on the way, nothing beside or above dir.
func fileIn(dir, p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || !within(dir, p) {
		return fmt.Errorf("%s is not a file in %s", p, dir)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return fmt.Errorf("no such file in the share folder: %w", err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(realDir, real); err != nil || filepath.Join(dir, rel) != p {
		return errors.New("it is reached through a link, which a run does not follow")
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("it is not a regular file")
	}
	return nil
}

func (m *mainLink) run(ctx context.Context, ref, task string, files []string, answers map[string]string) (string, error) {
	ns, name, version, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	it, err := m.installedHere(ns, name)
	if err != nil {
		return "", err
	}
	if it == nil {
		return "", fmt.Errorf("%s/%s is not installed on this computer: install it with quickagent_install first", ns, name)
	}
	if version != "" && version != it.Version {
		return "", fmt.Errorf("%s/%s is installed at %s, not %s", ns, name, it.Version, version)
	}
	if it.Local {
		return "", fmt.Errorf("%s/%s was installed from a folder, and only the person runs it, from its page in the Library", ns, name)
	}
	if len(files) > maxRunFiles {
		return "", fmt.Errorf("a run takes at most %d files", maxRunFiles)
	}
	share, err := m.share()
	if err != nil {
		return "", err
	}
	inputs := make([]string, 0, len(files))
	for _, f := range files {
		p, err := shareFile(share, f)
		if err != nil {
			return "", err
		}
		inputs = append(inputs, p)
	}
	exact := ns + "/" + name + "@" + it.Version
	info, err := m.ops.usage(it.Origin, exact)
	if err != nil {
		return "", fmt.Errorf("%s was not started: its questions could not be read: %w", exact, err)
	}
	if !slices.Contains(info.Modes, string(manifest.ModeAutonomous)) {
		return "", fmt.Errorf("%s runs only as a session the person takes part in: they start it from its page", exact)
	}
	given, err := checkAnswers(info, answers, share, true)
	if err != nil {
		return "", err
	}
	if err := m.agreeToRun(exact, task, files, given); err != nil {
		return "", err
	}
	// began carries the run's id the first time it is known: when it waits
	// behind another job for the run lock, or when it begins.
	type start struct {
		id     string
		queued bool
	}
	began := make(chan start, 1)
	r := &mainRun{ref: exact, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		defer emitEvent(m.app, eventHomeChanged)
		r.res, r.err = m.ops.run(it.Origin, ns+"/"+name+"@"+it.Version, task, inputs, given.intake, share, m.ops.outputDir(), func(id string, queued bool) {
			m.mu.Lock()
			if m.runs == nil {
				m.runs = map[string]*mainRun{}
			}
			m.runs[id] = r
			m.mu.Unlock()
			select {
			case began <- start{id, queued}:
			default:
			}
		})
	}()
	started := func(s start) string {
		if s.queued {
			return fmt.Sprintf("Queued run %s of %s: another job is under way on this computer, and the run starts when the jobs before it end. It is an ordinary run on this computer and shows in Recent runs. Call run_result with this id to wait for its outcome.", s.id, exact)
		}
		return fmt.Sprintf("Started run %s of %s. It is an ordinary run on this computer and shows in Recent runs. Call run_result with this id to wait for its outcome.", s.id, exact)
	}
	select {
	case s := <-began:
		return started(s), nil
	case <-r.done:
		// A run that began and ended at once has both channels ready, and
		// select picks either: one that began did start.
		select {
		case s := <-began:
			return started(s), nil
		default:
		}
		if r.err != nil {
			return "", fmt.Errorf("the run of %s did not start: %w", exact, r.err)
		}
		return "", fmt.Errorf("the run of %s did not start", exact)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// agreed is the record of runs the person agreed to let Main start.
type agreed struct {
	Version int      `json:"version"`
	Runs    []string `json:"runs"`
}

// agreeToRun asks the person before Main's first run of a QuickAgent at a
// version, and records a Yes; a later run of the same version asks nothing.
// A run that fetches a repository or a file is agreed to with its
// addresses, so the same version fetching from another address asks again.
func (m *mainLink) agreeToRun(exact, task string, files []string, given *checkedAnswers) error {
	path, err := m.agreedPath()
	if err != nil {
		return err
	}
	var rec agreed
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // the app's own record
		_ = json.Unmarshal(b, &rec)
	}
	key := agreedKey(exact, given.sources)
	if slices.Contains(rec.Runs, key) {
		return nil
	}
	who := m.who()
	var msg strings.Builder
	fmt.Fprintf(&msg, "%s asks to run %s on this computer", who, exact)
	if len(files) > 0 {
		fmt.Fprintf(&msg, " on %s from its share folder", strings.Join(files, ", "))
	}
	msg.WriteString(".")
	if len(given.lines) > 0 {
		fmt.Fprintf(&msg, "\n\nThe answers %s gave its questions:\n- %s", who, strings.Join(given.lines, "\n- "))
	}
	if len(given.sources) > 0 {
		msg.WriteString("\n\nThe run fetches from those addresses before it starts, with the key saved in Agentry for each address's host, if there is one.")
	}
	if t := strings.TrimSpace(task); t != "" {
		// Main's words, quoted, so they cannot pass for the dialog's own.
		fmt.Fprintf(&msg, "\n\nThe task %s wrote for it: %s", who, quoted(cut(t, 500)))
	}
	again := " may run this version again without asking."
	if len(given.sources) > 0 {
		again = " may run this version on the same addresses again without asking; other addresses ask again."
	}
	msg.WriteString("\n\nIt runs as any run of it does, under what it was installed with: it is listed in Recent runs and writes to your output folder. " + who + again + "\n\nRun it?")
	if err := m.app.askConsent("Run "+exact+" for "+who+"?", msg.String()); err != nil {
		return err
	}
	rec.Version = 1
	rec.Runs = append(rec.Runs, key)
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// agreedKey is a run's entry in the record of agreed runs: the exact
// version, and each source it fetches as id=address, sorted.
func agreedKey(exact string, sources []string) string {
	if len(sources) == 0 {
		return exact
	}
	s := slices.Clone(sources)
	slices.Sort(s)
	return exact + " " + strings.Join(s, " ")
}

// cut is s at most n bytes long, on a rune boundary, marked where cut.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + " [cut]"
}

func (m *mainLink) result(ctx context.Context, id string) (string, error) {
	id = strings.TrimSpace(id)
	if !validRunID(id) {
		return "", fmt.Errorf("%s is not a run id", quoted(id))
	}
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r != nil {
		wait, cancel := context.WithTimeout(ctx, resultWait)
		defer cancel()
		select {
		case <-r.done:
		case <-wait.Done():
			return fmt.Sprintf("Run %s of %s has not finished after %s: it is running, or waiting to start behind another job. Call run_result again to wait for it.", id, r.ref, resultWait), nil
		}
		if r.err != nil {
			return "", fmt.Errorf("run %s of %s ended without a result: %w", id, r.ref, r.err)
		}
		return runOutcome(id, r.ref, r.res), nil
	}
	v, err := m.ops.recorded(id)
	if err != nil {
		return "", fmt.Errorf("no run %s that Agentry remembers: %w", id, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Run %s of %s: %s.\n", id, v.Run.Ref, v.Run.Outcome)
	if v.Folder != "" {
		fmt.Fprintf(&b, "Its folder: %s (%s)\n", v.Folder, v.FolderState)
	}
	if v.ResultJSON != "" {
		fmt.Fprintf(&b, "Its result (the QuickAgent's words): %s\n", cut(v.ResultJSON, maxResultText))
	}
	return b.String(), nil
}

// runOutcome is a finished run's outcome as Main is told it.
func runOutcome(id, ref string, res *RunResultView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run %s of %s ended: %s", id, ref, res.Status)
	if res.ExitClass != "" {
		fmt.Fprintf(&b, " (%s)", res.ExitClass)
	}
	b.WriteString(".\n")
	if res.Error != "" {
		fmt.Fprintf(&b, "Why: %s\n", res.Error)
	}
	if res.OutDir != "" {
		fmt.Fprintf(&b, "Its files are in %s on this computer.\n", res.OutDir)
	}
	if res.ResultJSON != "" {
		fmt.Fprintf(&b, "Its result (the QuickAgent's words): %s\n", cut(res.ResultJSON, maxResultText))
	}
	return b.String()
}

func (m *mainLink) installedList() (string, error) {
	items, err := m.ops.installed()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, it := range items {
		if it.Target != installdir.Target || !it.Present {
			continue
		}
		fmt.Fprintf(&b, "- %s/%s@%s", it.Namespace, it.Name, it.Version)
		if it.Local {
			b.WriteString(" (from a folder: only the person runs it)")
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "No QuickAgent is installed on this computer.", nil
	}
	return "Installed on this computer:\n" + b.String(), nil
}

func (m *mainLink) scheduleAdd(ref, when, task string, answers map[string]string) (string, error) {
	ns, name, _, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	it, err := m.installedHere(ns, name)
	if err != nil {
		return "", err
	}
	if it == nil {
		return "", fmt.Errorf("%s/%s is not installed on this computer: install it with quickagent_install first", ns, name)
	}
	if it.Local {
		return "", fmt.Errorf("%s/%s was installed from a folder: the person schedules it from the Schedule page", ns, name)
	}
	w, err := parseWhen(when, time.Now())
	if err != nil {
		return "", err
	}
	exact := ns + "/" + name + "@" + it.Version
	info, err := m.ops.usage(it.Origin, exact)
	if err != nil {
		return "", fmt.Errorf("%s was not scheduled: its questions could not be read: %w", exact, err)
	}
	given, err := checkAnswers(info, answers, "", false)
	if err != nil {
		return "", err
	}
	req := schedules.Request{
		Registry: it.Origin, Ref: exact, Task: strings.TrimSpace(task),
		OutDir: m.ops.outputDir(), MaxTurns: -1, When: w,
	}
	if len(given.intake) > 0 {
		req.Intake = given.intake
	}
	made, err := m.ops.addSchedule(req, m.who())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Scheduled %s: %s. The schedule's id is %s.", req.Ref, describeWhen(w), made.ID), nil
}

func (m *mainLink) scheduleList() (string, error) {
	v, err := m.ops.schedules()
	if err != nil {
		return "", err
	}
	if len(v.Schedules) == 0 {
		return "No run is scheduled on this computer.", nil
	}
	var b strings.Builder
	b.WriteString("Scheduled on this computer:\n")
	for _, s := range v.Schedules {
		fmt.Fprintf(&b, "- %s: %s (%s@%s), %s", s.ID, quoted(s.Name), s.Ref, s.Version, describeWhen(s.When))
		switch {
		case s.Ended:
			b.WriteString(", ended")
		case s.Paused:
			b.WriteString(", paused")
		case s.Next != "":
			fmt.Fprintf(&b, ", next %s", s.Next)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// scheduleNamed is the schedule with id, from the store.
func (m *mainLink) scheduleNamed(id string) (*schedules.Item, error) {
	id = strings.TrimSpace(id)
	if !schedule.ValidID(id) {
		return nil, fmt.Errorf("%s is not a schedule's id: schedule_list names them", quoted(id))
	}
	v, err := m.ops.schedules()
	if err != nil {
		return nil, err
	}
	for i := range v.Schedules {
		if v.Schedules[i].ID == id {
			return &v.Schedules[i], nil
		}
	}
	return nil, fmt.Errorf("no schedule %s on this computer", id)
}

func (m *mainLink) schedulePause(id string, paused bool) (string, error) {
	s, err := m.scheduleNamed(id)
	if err != nil {
		return "", err
	}
	if err := m.ops.pauseSchedule(s.ID, paused); err != nil {
		return "", err
	}
	if paused {
		return fmt.Sprintf("Paused %s: it does not run until it is resumed.", quoted(s.Name)), nil
	}
	return fmt.Sprintf("Resumed %s.", quoted(s.Name)), nil
}

func (m *mainLink) scheduleRemove(id string) (string, error) {
	s, err := m.scheduleNamed(id)
	if err != nil {
		return "", err
	}
	removed, err := m.ops.removeSchedule(s.ID, s.Name, m.who())
	if err != nil {
		return "", err
	}
	if !removed {
		return "", errNotConfirmed
	}
	return fmt.Sprintf("Deleted the schedule %s.", quoted(s.Name)), nil
}

// settingsSections are the Settings sections Main may open, by the id the
// page knows them by (Settings.tsx's SECTIONS) and the title it shows.
var settingsSections = []struct{ id, title string }{
	{"profile", "Profile"}, {"report-design", "Report design"}, {"registry", "Registry settings"},
	{"updates", "Update settings"}, {"coding-agents", "Coding tools"}, {"persistent-agents", "Persistent Agents"},
	{"run-defaults", "Run defaults"}, {"provider", "Provider settings"}, {"connections", "Connections"},
	{"destinations", "Destinations"}, {"home", "Home"}, {"licences", "Licences"},
}

// settingsSection is the section a name means: its id or its title, in any
// case.
func settingsSection(name string) (id, title string, ok bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range settingsSections {
		if n == s.id || n == strings.ToLower(s.title) || n == strings.ToLower(strings.TrimSuffix(s.title, " settings")) {
			return s.id, s.title, true
		}
	}
	return "", "", false
}

func (m *mainLink) settingsOpen(section string) (string, error) {
	id, title, ok := settingsSection(section)
	if !ok {
		names := make([]string, 0, len(settingsSections))
		for _, s := range settingsSections {
			names = append(names, s.title)
		}
		return "", fmt.Errorf("there is no Settings section %s; the sections are %s", quoted(section), strings.Join(names, ", "))
	}
	emitEvent(m.app, eventOpenSettings, id)
	return fmt.Sprintf("Opened Settings → %s for the person. Nothing was changed: any change there is theirs to make.", title), nil
}

// docsPage is a path on the docs site.
var docsPage = regexp.MustCompile(`^/[A-Za-z0-9/_.\-]*$`)

func (m *mainLink) docsRead(ctx context.Context, page string) (string, error) {
	page = strings.TrimSpace(page)
	if page == "" {
		page = "/"
	}
	if !strings.HasPrefix(page, "/") {
		page = "/" + page
	}
	if len(page) > 200 || !docsPage.MatchString(page) || strings.Contains(page, "..") || strings.Contains(page, "//") {
		return "", fmt.Errorf("%s is not a page of the docs: give its path, such as /agentry/desktop/", quoted(page))
	}
	ctx, cancel := context.WithTimeout(ctx, docsTimeout)
	defer cancel()
	body, status, err := m.ops.docs(ctx, page)
	if err != nil {
		return "", fmt.Errorf("the docs did not answer: %w", err)
	}
	if status == http.StatusNotFound {
		return fmt.Sprintf("The docs have no page %s. docs_read of / lists the pages.", page), nil
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("the docs answered %d for %s", status, page)
	}
	text := docsText(body, page)
	return fmt.Sprintf("%s%s (the page's text):\n\n%s", docsBase, page, cut(text, maxDocsText)), nil
}

// docsText is a docs page's text: its article (or its body), with the links
// within the docs written as their paths, so a next docs_read can follow
// them.
func docsText(body []byte, page string) string {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	root := findElement(doc, "article")
	if root == nil {
		root = findElement(doc, "main")
	}
	if root == nil {
		root = findElement(doc, "body")
	}
	if root == nil {
		return ""
	}
	base, _ := url.Parse(docsBase + page)
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(strings.Join(strings.Fields(n.Data), " "))
			if strings.HasSuffix(n.Data, " ") || strings.HasSuffix(n.Data, "\n") {
				b.WriteString(" ")
			}
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "nav", "header", "footer", "svg", "button", "form":
				return
			case "p", "div", "section", "li", "tr", "pre", "h1", "h2", "h3", "h4", "h5", "h6", "br", "table", "ul", "ol", "dl", "dt", "dd", "blockquote":
				b.WriteString("\n")
				if strings.HasPrefix(n.Data, "h") && len(n.Data) == 2 {
					b.WriteString(strings.Repeat("#", int(n.Data[1]-'0')) + " ")
				}
				if n.Data == "li" {
					b.WriteString("- ")
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" || base == nil {
					continue
				}
				if u, err := base.Parse(a.Val); err == nil && u.Host == base.Host && u.Path != base.Path {
					fmt.Fprintf(&b, " (%s)", u.Path)
				}
			}
		}
	}
	walk(root)
	lines := strings.Split(b.String(), "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" && l != "-" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// findElement is the first element named tag beneath n.
func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findElement(c, tag); f != nil {
			return f
		}
	}
	return nil
}

// The ways a schedule's time can be said: "every weekday at 09:00",
// "every day at 7pm", "every monday and thursday at 18:30", "monthly on the
// 3rd at 08:00", "once on 2026-11-02 at 10:00", "tomorrow at 9".
var (
	whenTime     = regexp.MustCompile(`(?:\bat\s+)?\b(\d{1,2})(?::(\d{2}))?\s*(am|pm)?\b`)
	whenDate     = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)
	whenMonthDay = regexp.MustCompile(`\b(?:day\s+)?(\d{1,2})(?:st|nd|rd|th)?\b`)
	whenDays     = map[string]string{
		"monday": "mon", "mondays": "mon", "mon": "mon", "tuesday": "tue", "tuesdays": "tue", "tue": "tue", "tues": "tue",
		"wednesday": "wed", "wednesdays": "wed", "wed": "wed", "thursday": "thu", "thursdays": "thu", "thu": "thu", "thurs": "thu",
		"friday": "fri", "fridays": "fri", "fri": "fri", "saturday": "sat", "saturdays": "sat", "sat": "sat",
		"sunday": "sun", "sundays": "sun", "sun": "sun",
	}
	dayOrder = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
)

const whenForms = `say when as one of: "every day at 09:00", "every weekday at 09:00", "every monday and thursday at 18:30", "monthly on the 3rd at 08:00", "once on 2026-11-02 at 10:00", "tomorrow at 9am"`

// parseWhen reads when a schedule runs, in the words whenForms lists.
func parseWhen(s string, now time.Time) (schedules.When, error) {
	t := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(s, ",", " , ")), " "))
	if t == "" {
		return schedules.When{}, errors.New(whenForms)
	}
	w := schedules.When{}
	rest := t
	if d := whenDate.FindStringSubmatch(t); d != nil {
		w.Kind, w.Date = "once", d[1]
		rest = strings.Replace(rest, d[1], " ", 1)
	} else if strings.Contains(t, "tomorrow") {
		w.Kind, w.Date = "once", now.AddDate(0, 0, 1).Format("2006-01-02")
	} else if strings.Contains(t, "today") {
		w.Kind, w.Date = "once", now.Format("2006-01-02")
	}
	if w.Kind == "" {
		switch {
		case strings.Contains(t, "weekday"):
			w.Kind = "weekdays"
		case strings.Contains(t, "month"):
			w.Kind = "monthly"
		case strings.Contains(t, "every day") || strings.Contains(t, "daily") || strings.Contains(t, "each day"):
			w.Kind = "daily"
		default:
			for _, word := range strings.Fields(t) {
				if d, ok := whenDays[word]; ok && !slices.Contains(w.Days, d) {
					w.Days = append(w.Days, d)
				}
			}
			if len(w.Days) == 0 {
				return schedules.When{}, errors.New(whenForms)
			}
			w.Kind = "weekly"
			slices.SortFunc(w.Days, func(a, b string) int { return slices.Index(dayOrder, a) - slices.Index(dayOrder, b) })
		}
	}
	// The time is the one after "at", else the last number that reads as one.
	timeText := rest
	if i := strings.LastIndex(rest, " at "); i >= 0 {
		timeText = rest[i:]
	} else if strings.HasPrefix(rest, "at ") {
		timeText = rest
	}
	if w.Kind == "monthly" {
		before := timeText
		if i := strings.LastIndex(rest, " at "); i >= 0 {
			before = rest[:i]
		}
		md := whenMonthDay.FindStringSubmatch(before)
		if md == nil || strings.LastIndex(rest, " at ") < 0 {
			return schedules.When{}, errors.New("a monthly schedule needs its day and its time, as in \"monthly on the 3rd at 08:00\"")
		}
		w.MonthDay, _ = strconv.Atoi(md[1])
		if w.MonthDay < 1 || w.MonthDay > 31 {
			return schedules.When{}, errors.New("the day of the month is 1 to 31")
		}
	}
	m := whenTime.FindAllStringSubmatch(timeText, -1)
	if len(m) == 0 {
		return schedules.When{}, errors.New("say the time it runs, as in \"at 09:00\"; " + whenForms)
	}
	tm := m[len(m)-1]
	h, _ := strconv.Atoi(tm[1])
	mi := 0
	if tm[2] != "" {
		mi, _ = strconv.Atoi(tm[2])
	}
	switch tm[3] {
	case "pm":
		if h < 12 {
			h += 12
		}
	case "am":
		if h == 12 {
			h = 0
		}
	}
	if h > 23 || mi > 59 || (tm[3] != "" && (strings.TrimLeft(tm[1], "0") == "" || len(tm[1]) > 2 || mustAtoi(tm[1]) > 12)) {
		return schedules.When{}, fmt.Errorf("%s is not a time of day", quoted(tm[0]))
	}
	w.Time = fmt.Sprintf("%02d:%02d", h, mi)
	if _, err := w.Args(); err != nil {
		return schedules.When{}, err
	}
	return w, nil
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// describeWhen is when a schedule runs, in a phrase.
func describeWhen(w schedules.When) string {
	var s string
	switch w.Kind {
	case "once":
		s = "once, on " + w.Date + " at " + w.Time
	case "daily":
		s = "every day at " + w.Time
	case "weekdays":
		s = "every weekday at " + w.Time
	case "weekly":
		s = "every " + strings.Join(w.Days, ", ") + " at " + w.Time
	case "monthly":
		s = fmt.Sprintf("on day %d of every month at %s", w.MonthDay, w.Time)
	default:
		s = w.Kind + " at " + w.Time
	}
	if w.Zone != "" {
		s += " (" + w.Zone + ")"
	}
	return s
}

// appMainOps are the app's own operations, through its bound APIs.
type appMainOps struct {
	browse    *BrowseAPI
	installer *InstallAPI
	runner    *RunAPI
	scheduler *ScheduleAPI
	client    *http.Client
}

func (o appMainOps) registry() string {
	if s, err := settings.Load(); err == nil && s.DefaultRegistry != "" {
		return s.DefaultRegistry
	}
	return settings.SecureAgentryRegistry()
}

func (o appMainOps) outputDir() string {
	if s, err := settings.Load(); err == nil {
		return s.DefaultOutputDir
	}
	return ""
}

func (o appMainOps) search(reg string, q SearchQuery) (*SearchView, error) {
	return o.browse.Search(reg, q)
}

func (o appMainOps) agent(reg, ns, name string) (*AgentDetailView, error) {
	return o.browse.Agent(reg, ns, name)
}

func (o appMainOps) report(reg, ns, name, version string) (*ReportView, error) {
	return o.browse.Report(reg, ns, name, version)
}

func (o appMainOps) requirements(reg, ns, name string) (*MCPView, error) {
	return o.runner.GatewayRequirements(reg, ns, name)
}

func (o appMainOps) usage(reg, ref string) (*run.ProfileInfo, error) {
	return run.ProfileStatus(o.runner.app.ctx, reg, ref)
}

func (o appMainOps) installed() ([]install.Item, error) { return o.installer.ListInstalled() }

func (o appMainOps) install(reg, ref, who string) (*install.Result, error) {
	return o.installer.installRunner(reg, ref, who)
}

func (o appMainOps) run(reg, ref, task string, inputs []string, intake map[string]string, share, outDir string, began func(id string, queued bool)) (*RunResultView, error) {
	h := runHooks{
		began:    func(id string) { began(id, false) },
		queued:   func(id string) { began(id, true) },
		inputsIn: share,
	}
	return o.runner.runAgent(h, reg, ref, task, inputs, outDir, nil, 0, -1, false, intake, false, false, nil, false, nil, nil, nil)
}

func (o appMainOps) recorded(runID string) (*RecordedRunView, error) {
	return o.runner.RecordedRun(runID)
}

func (o appMainOps) schedules() (*SchedulesView, error) { return o.scheduler.List() }

func (o appMainOps) addSchedule(req schedules.Request, who string) (*ScheduleMadeView, error) {
	return o.scheduler.add(req, nil, who)
}

func (o appMainOps) pauseSchedule(id string, paused bool) error {
	if paused {
		return o.scheduler.Pause(id)
	}
	return o.scheduler.Resume(id)
}

func (o appMainOps) removeSchedule(id, name, who string) (bool, error) {
	return o.scheduler.remove(id, name, who)
}

func (o appMainOps) docs(ctx context.Context, page string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docsBase+page, nil)
	if err != nil {
		return nil, 0, err
	}
	c := o.client
	if c == nil {
		c = &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
			// The docs site's own pages only: a redirect off it is not
			// followed.
			if r.URL.Scheme != "https" || r.URL.Host != strings.TrimPrefix(docsBase, "https://") || len(via) > 5 {
				return http.ErrUseLastResponse
			}
			return nil
		}}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxDocsBytes))
	return b, resp.StatusCode, err
}

// newAgentLink is the named agent's handler of Agentry Helper's tools over
// the app's own operations: its share folder, its own record of agreed runs
// (Main's main-runs.json, any other agent's <name>-runs.json beside it), and
// its name in the dialogs.
func newAgentLink(app *App, ops mainOps, home func() (string, error), agent string) *mainLink {
	config := func() (persistent.Agent, persistent.Config, string, error) {
		h, err := home()
		if err != nil {
			return persistent.Agent{}, persistent.Config{}, "", err
		}
		a, err := persistent.Lookup(agent)
		if err != nil {
			return persistent.Agent{}, persistent.Config{}, "", err
		}
		cfg, err := persistent.LoadConfig(h, a.Name)
		return a, cfg, h, err
	}
	l := &mainLink{
		app: app, ops: ops,
		share: func() (string, error) {
			a, cfg, h, err := config()
			if err != nil {
				return "", err
			}
			share, err := persistent.ShareFolder(a, cfg)
			if err != nil {
				return "", err
			}
			if err := persistent.CheckShareFolder(h, share); err != nil {
				return "", err
			}
			return share, nil
		},
		agreedPath: func() (string, error) {
			p, err := settings.Path()
			if err != nil {
				return "", err
			}
			return filepath.Join(filepath.Dir(p), agent+"-runs.json"), nil
		},
	}
	if agent != mainAgent {
		l.asks = func() string {
			title := agent
			if a, err := persistent.Lookup(agent); err == nil {
				title = a.Title
			}
			a, cfg, _, err := config()
			if err != nil {
				return title
			}
			return askerName(title, a.DisplayName(cfg))
		}
	}
	return l
}
