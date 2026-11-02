package app

// The app's side of a persistent agent's helper tools and subagent runs,
// which the agent's supervisor answers itself: it decides what may happen
// and asks the person, and these adapters reach the app's own operations
// for it. The install, schedule and schedule-removal dialogs are the app's
// own, which show what is installed or scheduled and name the agent asking,
// so the supervisor asks nothing more for those (persistent.HelperOpsAsk).
// Every other question the supervisor puts to the person (the first run of
// a version, a subagent run the person asked to be asked before) is drawn
// as the app's native dialog by the agent's own start (askFor).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// appHelper is persistent.HelperOps over the app's operations.
type appHelper struct {
	app *App
	ops mainOps
}

var _ persistent.HelperOpsAsk = appHelper{}

// AsksThePerson: the app's install and schedule operations ask in its own
// dialogs.
func (h appHelper) AsksThePerson() bool { return true }

func (h appHelper) Search(_ context.Context, query string) (string, []persistent.HelperHit, error) {
	reg := h.ops.registry()
	v, err := h.ops.search(reg, SearchQuery{Q: query, Limit: maxSearchHits * 2})
	if err != nil {
		return "", nil, err
	}
	hits := make([]persistent.HelperHit, 0, len(v.Results))
	for _, r := range v.Results {
		hits = append(hits, persistent.HelperHit{Ref: r.Namespace + "/" + r.Name, Summary: r.Summary, Modes: r.Modes, Installs30d: r.Installs30d, Quarantined: r.Quarantine != ""})
	}
	return settings.ShownURL(reg), hits, nil
}

func (h appHelper) Show(_ context.Context, ref string) (string, error) {
	return (&mainLink{app: h.app, ops: h.ops}).show(ref)
}

func (h appHelper) Installed(context.Context) ([]persistent.HelperInstalled, error) {
	items, err := h.ops.installed()
	if err != nil {
		return nil, err
	}
	var out []persistent.HelperInstalled
	for _, it := range items {
		if it.Target != installdir.Target || !it.Present {
			continue
		}
		out = append(out, persistent.HelperInstalled{Ref: it.Namespace + "/" + it.Name, Version: it.Version, Registry: it.Origin, Local: it.Local})
	}
	return out, nil
}

func (h appHelper) Install(_ context.Context, ref, who string) (string, error) {
	res, err := h.ops.install(h.ops.registry(), ref, who)
	if err != nil {
		return "", asPersonsNo(err)
	}
	emitEvent(h.app, eventHomeChanged)
	if res.Warning != "" {
		return res.Message + " (" + res.Warning + ")", nil
	}
	return res.Message, nil
}

func (h appHelper) Schedules(context.Context) ([]persistent.HelperSchedule, error) {
	v, err := h.ops.schedules()
	if err != nil {
		return nil, err
	}
	out := make([]persistent.HelperSchedule, 0, len(v.Schedules))
	for _, s := range v.Schedules {
		out = append(out, persistent.HelperSchedule{ID: s.ID, Name: s.Name, Ref: s.Ref + "@" + s.Version, When: describeWhen(s.When), Next: s.Next, Paused: s.Paused, Ended: s.Ended})
	}
	return out, nil
}

func (h appHelper) AddSchedule(_ context.Context, s persistent.HelperScheduleSpec, who string) (string, error) {
	req := schedules.Request{
		Registry: s.Registry, Ref: s.Exact, Task: s.Task, OutDir: h.ops.outputDir(), MaxTurns: -1,
		When: schedules.When{Kind: s.When.Kind, Date: s.When.Date, Time: s.When.Time, Days: s.When.Days, MonthDay: s.When.MonthDay},
	}
	if len(s.Answers) > 0 {
		req.Intake = s.Answers
	}
	made, err := h.ops.addSchedule(req, who)
	if err != nil {
		return "", asPersonsNo(err)
	}
	return made.ID, nil
}

func (h appHelper) PauseSchedule(_ context.Context, id string, paused bool) error {
	return h.ops.pauseSchedule(id, paused)
}

func (h appHelper) RemoveSchedule(ctx context.Context, id, who string) error {
	all, err := h.Schedules(ctx)
	if err != nil {
		return err
	}
	name := id
	if i := slices.IndexFunc(all, func(s persistent.HelperSchedule) bool { return s.ID == id }); i >= 0 {
		name = all[i].Name
	}
	removed, err := h.ops.removeSchedule(id, name, who)
	if err != nil {
		return asPersonsNo(err)
	}
	if !removed {
		return persistent.ErrNotAgreed
	}
	return nil
}

func (h appHelper) Docs(ctx context.Context, page string) ([]byte, int, error) {
	return h.ops.docs(ctx, page)
}

// asPersonsNo is err as the supervisor reads it: the app's no is the person's.
func asPersonsNo(err error) error {
	if errors.Is(err, errNotConfirmed) {
		return persistent.ErrNotAgreed
	}
	return err
}

// appSubagents is persistent.SubagentHost over the app's run service: a
// subagent run, or a run quickagent_run starts, is an ordinary run in this
// app, listed in Recent runs, and its envelope is read back from its folder.
type appSubagents struct {
	ops mainOps
}

func (s appSubagents) Check(_ context.Context, registry, exact string) (persistent.SubagentUsage, error) {
	var none persistent.SubagentUsage
	if registry == "" {
		registry = s.ops.registry()
	}
	base, version, _ := strings.Cut(exact, "@")
	ns, name, ok := strings.Cut(base, "/")
	if !ok || version == "" {
		return none, fmt.Errorf("%q is not namespace/name@version", exact)
	}
	d, err := s.ops.agent(registry, ns, name)
	if err != nil {
		return none, fmt.Errorf("the registry could not be asked about it: %w", err)
	}
	if d.Agent.Quarantine != "" && d.Agent.Quarantine != "none" {
		return none, errors.New("the registry quarantined it, so it runs no more")
	}
	i := slices.IndexFunc(d.Versions, func(v VersionView) bool { return v.Version == version })
	switch {
	case i < 0:
		return none, errors.New("that version is no longer in the registry")
	case d.Versions[i].Yanked || d.Versions[i].Status == "yanked":
		return none, errors.New("its publisher withdrew it, so it runs no more")
	case d.Versions[i].Status != "published":
		return none, fmt.Errorf("it is %s on the registry, not published", d.Versions[i].Status)
	}
	info, err := s.ops.usage(registry, exact)
	if err != nil {
		return none, fmt.Errorf("its questions could not be read: %w", err)
	}
	if slices.Contains(info.Modes, string(manifest.ModePersistent)) || !slices.Contains(info.Modes, string(manifest.ModeAutonomous)) {
		return none, errors.New("it does not run as an autonomous QuickAgent, so it never runs as a subagent")
	}
	var u persistent.SubagentUsage
	for _, q := range info.Intake {
		if personsOwn(q) {
			continue
		}
		u.Questions = append(u.Questions, persistent.SubagentQuestion{ID: q.ID, Type: string(questionType(q)), Required: q.Required, Prompt: q.Prompt, Choices: q.Choices, Answer: howToAnswer(info, q)})
	}
	return u, nil
}

func (s appSubagents) Run(_ context.Context, r persistent.SubagentRunSpec) (*receipt.Envelope, error) {
	registry := r.Registry
	if registry == "" {
		registry = s.ops.registry()
	}
	in := filepath.Join(filepath.Dir(r.OutDir), "in")
	res, err := s.ops.run(registry, r.Exact, r.Task, r.Files, r.Answers, in, r.OutDir, func(string, bool) {})
	if err != nil && res == nil {
		return nil, err
	}
	dir := r.OutDir
	if res != nil && res.OutDir != "" {
		dir = res.OutDir
	}
	b, rerr := os.ReadFile(filepath.Join(dir, "result.json")) //nolint:gosec // the run's own output folder
	if rerr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the run's envelope could not be read: %w", rerr)
	}
	var env receipt.Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("the run's envelope does not read: %w", err)
	}
	return &env, nil
}
