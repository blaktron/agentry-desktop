package app

// The persistent agents' set-ups that are more than a switch: the
// Assistant's tasks (each paused, resumed or removed through the agent's
// owner, whether it runs here, elsewhere or not at all), its memory copied
// into a new folder the person picks, and the Coding agent's git host
// accounts, whose token travels inward only and is kept in the system
// keychain before the change records the account.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage"
)

// PersistentTaskView is one of the agent's tasks as the Tasks view draws
// it. Every text is the agent's or the person's, drawn as text.
type PersistentTaskView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Cron        string `json:"cron"`
	Zone        string `json:"zone"`
	Instruction string `json:"instruction"`
	// State is active or paused.
	State string `json:"state"`
	// MadeBy says who made it: the agent, at the person's asking.
	MadeBy string `json:"madeBy"`
	// Next, LastFire are RFC 3339, "" for none; LastResult is the first
	// line of the last fire's answer, or why it did not answer.
	Next       string `json:"next,omitempty"`
	LastFire   string `json:"lastFire,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
}

func taskViews(ts []persistent.Task) []PersistentTaskView {
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format(time.RFC3339)
	}
	out := make([]PersistentTaskView, 0, len(ts))
	for _, t := range ts {
		out = append(out, PersistentTaskView{
			ID: t.ID, Name: t.Name, Cron: t.Cron, Zone: t.Zone, Instruction: t.Instruction, State: t.State, MadeBy: t.MadeBy,
			Next: stamp(t.Next), LastFire: stamp(t.LastFire), LastResult: t.LastResult,
		})
	}
	return out
}

// Tasks are the agent's tasks, each with its next fire: its owner's while
// it runs, here or in another client, its file's while it does not.
func (p *PersistentAPI) Tasks(name string) ([]PersistentTaskView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ts, _, err := persistent.TasksOf(ctx, home, a)
	if err != nil {
		return nil, err
	}
	return taskViews(ts), nil
}

// ChangeTask pauses, resumes or removes one of the agent's tasks (op is
// pause, resume or remove) and answers the tasks as they now are. Adding a
// task is asked of the agent in conversation, never done here.
func (p *PersistentAPI) ChangeTask(name, id, op string) ([]PersistentTaskView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	switch op {
	case persistent.TaskOpPause, persistent.TaskOpResume, persistent.TaskOpRemove:
	default:
		return nil, fmt.Errorf("a task is paused, resumed or removed, not %q", op)
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ts, _, err := persistent.ChangeTaskOf(ctx, home, a, persistent.ClientDesktop, persistent.TaskOp{ID: id, Op: op})
	if err != nil {
		return nil, err
	}
	return taskViews(ts), nil
}

// PersistentExportView is what an export of the agent's memory wrote.
type PersistentExportView struct {
	// Folder is the new folder the memory was copied into, "" for a cancel.
	Folder  string `json:"folder"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Skipped int    `json:"skipped,omitempty"`
}

// ExportMemory copies the agent's memory, its pages, index and log as they
// are at this moment, into a new folder made inside the one the native
// picker returns, named for the agent and the time. A cancel writes
// nothing.
func (p *PersistentAPI) ExportMemory(name string) (*PersistentExportView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if !a.Builtin.KeepsMemory() {
		return nil, fmt.Errorf("the %s keeps no memory", a.Title)
	}
	parent, err := p.pickExportFolder()
	if err != nil || parent == "" {
		return &PersistentExportView{}, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	d := p.dockerClient()
	tag := pol.RunnerImageTag(runnerimage.ContentVersion())
	if p.exportMemory == nil {
		if err := gateway.EnsureRunnerImage(ctx, d, pol, tag, filepath.Join(home, "image"), discardLog{}); err != nil {
			return nil, err
		}
	}
	dir := filepath.Join(filepath.Clean(parent), exportFolderName(a.Name, time.Now()))
	export := p.exportMemory
	if export == nil {
		export = func(ctx context.Context, home string, a persistent.Agent, dir string) (persistent.Exported, error) {
			return persistent.ExportMemory(ctx, d, home, a, tag, pol.Docker.Runtime, pol.Docker.Limits, dir)
		}
	}
	out, err := export(ctx, home, a, dir)
	if err != nil {
		return nil, err
	}
	return &PersistentExportView{Folder: dir, Files: out.Files, Bytes: out.Bytes, Skipped: out.Skipped}, nil
}

// exportFolderName is the new folder an export makes: the agent's name and
// the time, to the second, so two exports never meet.
func exportFolderName(agent string, t time.Time) string {
	return agent + "-memory-" + t.Format("2006-01-02-150405")
}

// pickExportFolder opens the native folder picker for where the memory's
// new folder goes; "" is a cancel.
func (p *PersistentAPI) pickExportFolder() (string, error) {
	if p.pickFolder != nil {
		return p.pickFolder()
	}
	return runtime.OpenDirectoryDialog(p.app.ctx, runtime.OpenDialogOptions{Title: "Choose where the memory's copy goes"})
}

// discardLog drops the runner image build's progress.
type discardLog struct{}

func (discardLog) Write(b []byte) (int, error) { return len(b), nil }

// PersistentAccountView is one of the Coding agent's git host accounts, as
// its form shows it: never its token.
type PersistentAccountView struct {
	Host  string `json:"host"`
	Kind  string `json:"kind"`
	Own   bool   `json:"own"`
	User  string `json:"user"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func accountViews(gs []persistent.GitAccount) []PersistentAccountView {
	out := make([]PersistentAccountView, 0, len(gs))
	for _, g := range gs {
		out = append(out, PersistentAccountView{Host: g.Host, Kind: string(g.Kind), Own: g.Own, User: g.User, Name: g.Name, Email: g.Email})
	}
	return out
}

// PersistentAccountForm is the account form: the host, its kind where the
// host's own is not known, whose account it is, the user, the commit name
// (the user name when empty) and email, and the token, which goes to the
// keychain and never comes back.
type PersistentAccountForm struct {
	Host  string `json:"host"`
	Kind  string `json:"kind,omitempty"`
	Own   bool   `json:"own"`
	User  string `json:"user"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
	Token string `json:"token"`
}

// AccountKind is the kind of a git host the app knows by name (github,
// gitlab, gitea), "" for one the person must say.
func (p *PersistentAPI) AccountKind(host string) string {
	return string(spec.GitHostKindOf(strings.ToLower(strings.TrimSpace(host))))
}

// AccountGuidance is what the form says before the token is asked for: for
// an account of the agent's own, why and how to share repositories with it;
// for the person's, what the token lets the agent do and how to bound it.
func (p *PersistentAPI) AccountGuidance(kind string, own bool) string {
	return persistent.AccountGuidance(spec.GitHostKind(kind), own)
}

// SetAccount gives the Coding agent an account on a git host, or sets its
// token again: the account is held to the CLI's rules, the token goes into
// the keychain before the change records the account (which asks first),
// and a change refused or failed puts the keychain back. A token left
// behind by a changed user name is removed once the change stands.
func (p *PersistentAPI) SetAccount(name string, f PersistentAccountForm) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if !a.Builtin.Codes() {
		return nil, fmt.Errorf("the %s has no git host accounts: the Coding agent alone is given them", a.Title)
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	g, err := readyAccount(cfg, f)
	if err != nil {
		return nil, err
	}
	if f.Token == "" {
		return nil, errors.New("the token is empty: nothing was kept")
	}
	if err := persistent.CheckSecretValue(f.Token); err != nil {
		return nil, err
	}
	keys := p.secretStore()
	account := persistent.AccountKeychainAccount(g.Host, g.User)
	previous := ""
	if slices.ContainsFunc(cfg.Accounts, func(x persistent.GitAccount) bool { return x.Host == g.Host && x.User == g.User }) {
		// A token set again widens nothing, so the change asks nothing:
		// the native dialog asks here.
		if err := p.app.askConsent("Replace the token for "+g.User+" on "+g.Host+"?", "This replaces the token "+called(a, cfg)+" signs in to "+g.Host+" with, in your system keychain. The old token is not kept."); err != nil {
			return nil, err
		}
		if previous, err = keys.Get(account); err != nil {
			return nil, fmt.Errorf("the keychain's token for %s on %s could not be read, so it is left as it is: %w", g.User, g.Host, err)
		}
	}
	if err := keys.Set(account, f.Token); err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{SetAccount: &g}); err != nil {
		if previous != "" {
			_ = keys.Set(account, previous)
		} else {
			_ = keys.Delete(account)
		}
		return nil, err
	}
	if i := slices.IndexFunc(cfg.Accounts, func(x persistent.GitAccount) bool { return x.Host == g.Host }); i >= 0 && cfg.Accounts[i].User != g.User {
		_ = keys.Delete(persistent.AccountKeychainAccount(g.Host, cfg.Accounts[i].User))
	}
	return p.Get(name)
}

// readyAccount is the account the form describes, held to the CLI's rules:
// a host it takes, a kind of its own or the one the person says, whose
// account it is, a user and an email, the commit name the user's when the
// form leaves it empty.
func readyAccount(cfg persistent.Config, f PersistentAccountForm) (persistent.GitAccount, error) {
	host := strings.ToLower(strings.TrimSpace(f.Host))
	if err := spec.CheckGitHost(host); err != nil {
		return persistent.GitAccount{}, err
	}
	g := persistent.GitAccount{Host: host, Own: f.Own, Kind: spec.GitHostKindOf(host)}
	if i := slices.IndexFunc(cfg.Accounts, func(x persistent.GitAccount) bool { return x.Host == host }); i >= 0 {
		g.Kind = cfg.Accounts[i].Kind
	}
	if k := strings.TrimSpace(f.Kind); k != "" {
		g.Kind = spec.GitHostKind(k)
	}
	if g.Kind == "" {
		return g, fmt.Errorf("say what kind of host %s is: GitHub, GitLab, Gitea or another git host", host)
	}
	if err := spec.CheckGitHostKind(g.Kind); err != nil {
		return g, err
	}
	g.User, g.Email, g.Name = strings.TrimSpace(f.User), strings.TrimSpace(f.Email), strings.TrimSpace(f.Name)
	if g.Name == "" {
		g.Name = g.User
	}
	switch {
	case g.User == "":
		return g, fmt.Errorf("name the account's user name on %s", host)
	case g.Email == "":
		return g, fmt.Errorf("name the email its commits carry on %s (the account's own, or the host's private address for it)", host)
	}
	if err := spec.CheckGitIdentity(g.User, g.Name, g.Email); err != nil {
		return g, err
	}
	g.Set = time.Now().UTC()
	return g, nil
}

// RemoveAccount takes one of the Coding agent's accounts away, then its
// token out of the keychain.
func (p *PersistentAPI) RemoveAccount(name, host string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{RemoveAccount: host}); err != nil {
		return nil, err
	}
	for _, g := range cfg.Accounts {
		if g.Host == host {
			if err := p.secretStore().Delete(persistent.AccountKeychainAccount(g.Host, g.User)); err != nil {
				return nil, fmt.Errorf("the account is removed, but its token stays in your system keychain: %w", err)
			}
		}
	}
	return p.Get(name)
}
