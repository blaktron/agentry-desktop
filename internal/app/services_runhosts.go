package app

// Remote runhosts: the person's own Linux servers, set up from this app over
// SSH, that run their QuickAgents. The app drives them in process through the
// CLI's runhost packages, exactly as `agentry runhost` does: setup and removal
// with the server's admin sign-in, which is used for that call alone and kept
// nowhere; everything else over this computer's run key for the runhost, held
// in the system keychain. The admin sign-in (a key file chosen in the native
// picker, a password, a sudo password) reaches Go inward only and no call
// returns it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/clirelease"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	rhsetup "github.com/blaktron/agentry-cli/pkg/runhost/setup"
	"github.com/blaktron/agentry-cli/pkg/runhost/signin"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Events a runhost's setup, removal and sign-in narrate on.
const (
	runhostSayEvent = "runhost:say"
)

// RunhostSayView is one line of a runhost call's narration.
type RunhostSayView struct {
	Name string `json:"name"`
	Line string `json:"line"`
}

// RunhostsAPI is the Runhosts settings, the wizard's Remote path and the run
// setup's runhost choices.
type RunhostsAPI struct {
	app *App
	run *RunAPI

	// busy is held by a setup, a removal or a change of a setting, one at a
	// time: each signs in to a server as its administrator.
	busy   sync.Mutex
	cmu    sync.Mutex
	cancel context.CancelFunc
}

// RunhostSignInView names a model sign-in a runhost keeps: never a token.
type RunhostSignInView struct {
	Vendor string `json:"vendor"`
	Label  string `json:"label,omitempty"`
}

// RunhostView is one runhost as this computer records it: never a secret.
type RunhostView struct {
	Name        string              `json:"name"`
	Host        string              `json:"host"`
	Port        int                 `json:"port"`
	Fingerprint string              `json:"fingerprint"`
	Owner       string              `json:"owner"`
	CLIVersion  string              `json:"cliVersion"`
	OS          string              `json:"os"`
	Arch        string              `json:"arch"`
	AddedAt     string              `json:"addedAt"`
	SignIns     []RunhostSignInView `json:"signIns"`
	// Credits names the runhost's Agentry credits token, when it has one.
	Credits string `json:"credits,omitempty"`
	// Default says runs go here unless the run setup says otherwise.
	Default bool `json:"default"`
	// Update is the runhost's last update by this app, when there was one.
	Update *RunhostUpdateView `json:"update,omitempty"`
}

// RunhostStatusView is what a runhost says of itself over the run key.
type RunhostStatusView struct {
	Reachable  bool   `json:"reachable"`
	Problem    string `json:"problem,omitempty"`
	CLIVersion string `json:"cliVersion,omitempty"`
	// SameVersion says the runhost runs this app's CLI version, which a run
	// on it needs.
	SameVersion bool `json:"sameVersion"`
	UnsafeMode  bool `json:"unsafeMode"`
	Running     int  `json:"running"`
	Waiting     int  `json:"waiting"`
}

// RunhostAdmin is a server's admin sign-in, given for one call: the
// account, and a key file chosen in the native picker or a password, and the
// sudo password when sudo asks for one.
type RunhostAdmin struct {
	User         string `json:"user"`
	KeyFile      string `json:"keyFile,omitempty"`
	Passphrase   string `json:"passphrase,omitempty"`
	Password     string `json:"password,omitempty"`
	SudoPassword string `json:"sudoPassword,omitempty"`
}

// RunhostAddRequest is the wizard's or Settings' Server form.
type RunhostAddRequest struct {
	Name string `json:"name"`
	// Host and Port are the server's SSH address.
	Host  string       `json:"host"`
	Port  int          `json:"port,omitempty"`
	Admin RunhostAdmin `json:"admin"`
}

func (r *RunhostsAPI) home() (string, error) { return r.run.runnerHome() }

func (r *RunhostsAPI) ctx() context.Context { return r.run.ctx() }

func (r *RunhostsAPI) say(name string) func(string) {
	return func(line string) { emitEvent(r.app, runhostSayEvent, RunhostSayView{Name: name, Line: line}) }
}

// carriedCLIVersion is the CLI release this app carries, which a runhost it sets up
// runs: "" for a build whose CLI module is not a release (a workspace build),
// which cannot set one up.
func carriedCLIVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, d := range bi.Deps {
		if d.Path == "github.com/blaktron/agentry-cli" {
			if d.Replace != nil {
				return ""
			}
			v := strings.TrimPrefix(d.Version, "v")
			if v == "" || strings.HasPrefix(v, "0.0.0-") || v == "(devel)" {
				return ""
			}
			return v
		}
	}
	return ""
}

// releaseKeys are the update-signing keys a CLI release is checked against:
// this app's own, the publisher's one key set.
func releaseKeys() (*clirelease.KeySet, error) { return clirelease.ParseKeys(selfupdate.KeysJSON()) }

func viewOf(rec runhost.Record, def string) RunhostView {
	v := RunhostView{Name: rec.Name, Host: rec.Host, Port: rec.Port, Fingerprint: rec.HostKeyFingerprint, Owner: rec.Owner.String(),
		CLIVersion: rec.CLIVersion, OS: rec.OS, Arch: rec.Arch, AddedAt: rec.AddedAt, SignIns: []RunhostSignInView{},
		Default: def == settings.RunWhereRunhostPrefix+rec.Name}
	for _, s := range rec.SignIns {
		v.SignIns = append(v.SignIns, RunhostSignInView{Vendor: s.Vendor, Label: s.Label})
	}
	if rec.Credits != nil {
		v.Credits = rec.Credits.TokenName
	}
	return v
}

// List is the runhosts this computer is paired with.
func (r *RunhostsAPI) List() ([]RunhostView, error) {
	home, err := r.home()
	if err != nil {
		return nil, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, err
	}
	s, _ := settings.Load()
	u := loadRunhostUpdates()
	out := []RunhostView{}
	for _, rec := range recs.Runhosts {
		v := viewOf(rec, s.RunWhere)
		if up, ok := u.Runhosts[rec.Name]; ok {
			v.Update = &up
		}
		out = append(out, v)
	}
	return out, nil
}

// record is the runhost named name, with its keys.
func (r *RunhostsAPI) record(name string) (*runhost.Records, *runhost.Record, *runhost.DeviceKeys, string, error) {
	home, err := r.home()
	if err != nil {
		return nil, nil, nil, "", err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, nil, nil, "", err
	}
	rec, ok := recs.Find(name)
	if !ok {
		return nil, nil, nil, "", fmt.Errorf("no runhost is named %q on this computer", name)
	}
	keys, err := runhost.LoadKeys(name)
	if err != nil {
		return recs, rec, nil, home, err
	}
	return recs, rec, keys, home, nil
}

// dial reaches runhost name over this computer's run key.
func (r *RunhostsAPI) dial(ctx context.Context, name string) (*runhost.Record, *runhost.DeviceKeys, *runhost.Client, error) {
	_, rec, keys, _, err := r.record(name)
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return rec, keys, nil, err
	}
	return rec, keys, c, nil
}

// Status asks runhost name how it is, over the run key. A runhost that
// cannot be reached is a status, not an error.
func (r *RunhostsAPI) Status(name string) (*RunhostStatusView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 30*time.Second)
	defer cancel()
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		return &RunhostStatusView{Problem: err.Error()}, nil
	}
	defer func() { _ = c.Close() }()
	v, err := runhostVersion(ctx, c)
	if err != nil {
		return &RunhostStatusView{Problem: err.Error()}, nil
	}
	st := &RunhostStatusView{Reachable: true, CLIVersion: v.Version, UnsafeMode: v.UnsafeMode, SameVersion: v.Version == carriedCLIVersion()}
	if jobs, err := remoteJobs(ctx, c); err == nil {
		for _, j := range jobs {
			if j.Running() {
				st.Running++
			} else {
				st.Waiting++
			}
		}
	}
	return st, nil
}

// runhostVersion asks a runhost its version and settings.
func runhostVersion(ctx context.Context, c *runhost.Client) (remote.Version, error) {
	a, err := c.Ask(ctx, "version", nil, nil)
	if err != nil {
		return remote.Version{}, err
	}
	var v remote.Version
	if a.Code != 0 || json.Unmarshal(a.Result, &v) != nil {
		return remote.Version{}, errors.New("the runhost did not say its version")
	}
	return v, nil
}

// remoteJob is one of a runhost's jobs, as its status lists them.
type remoteJob struct {
	State string `json:"state"`
}

// Running says the job runs still.
func (j remoteJob) Running() bool { return j.State == "running" }

// remoteJobs are the jobs a runhost runs and the finished ones waiting.
func remoteJobs(ctx context.Context, c *runhost.Client) ([]remoteJob, error) {
	a, err := c.Ask(ctx, "status", nil, nil)
	if err != nil {
		return nil, err
	}
	var st struct {
		Jobs []remoteJob `json:"jobs"`
	}
	if a.Code != 0 || json.Unmarshal(a.Result, &st) != nil {
		return nil, errors.New("the runhost did not say its jobs")
	}
	return st.Jobs, nil
}

// adminAuth is an admin sign-in as setup takes it. A key file must be one
// the native picker returned.
func (r *RunhostsAPI) adminAuth(a RunhostAdmin) (rhsetup.AdminAuth, error) {
	var auth rhsetup.AdminAuth
	if strings.TrimSpace(a.User) == "" {
		return auth, errors.New("name the server's admin account: root, or one that can sudo")
	}
	switch {
	case a.KeyFile != "":
		if err := r.app.keyFiles.require("the admin key file", a.KeyFile); err != nil {
			return auth, err
		}
		b, err := os.ReadFile(a.KeyFile) //nolint:gosec // a key file the person chose in the native picker
		if err != nil {
			return auth, fmt.Errorf("read the admin key file: %w", err)
		}
		auth.Key, auth.Passphrase = b, a.Passphrase
	case a.Password != "":
		auth.Password = a.Password
	default:
		return auth, errors.New("give the admin account's key file or its password")
	}
	auth.Sudo = a.SudoPassword
	if auth.Sudo == "" && auth.Password != "" {
		auth.Sudo = auth.Password
	}
	return auth, nil
}

// dialogAsker is the person in the app: a host key is put to them in a
// native dialog; a typed answer or a password is never asked mid-call (the
// form gave them).
type dialogAsker struct{ app *App }

func (d dialogAsker) Ask(prompt, _ string) (string, error) {
	return "", fmt.Errorf("the server asked something this app cannot answer: %s", prompt)
}

func (d dialogAsker) AskSecret(prompt string) (string, error) {
	return "", fmt.Errorf("the server asked for a password the form did not give (%s): give the sudo password and try again", strings.TrimSpace(prompt))
}

func (d dialogAsker) Confirm(prompt string, _ bool) (bool, error) {
	if d.app.confirm == nil {
		return false, errors.New("no dialog is available to confirm this")
	}
	return d.app.confirm(d.app.ctx, "Remote runhost", prompt)
}

// begin takes the one admin call at a time and makes it cancellable.
func (r *RunhostsAPI) begin() (context.Context, func(), error) {
	if !r.busy.TryLock() {
		return nil, nil, errors.New("another runhost is being set up, changed or removed: wait for it to finish")
	}
	ctx, cancel := context.WithCancel(r.ctx())
	r.cmu.Lock()
	r.cancel = cancel
	r.cmu.Unlock()
	return ctx, func() {
		r.cmu.Lock()
		r.cancel = nil
		r.cmu.Unlock()
		cancel()
		r.busy.Unlock()
	}, nil
}

// Cancel stops the setup, change or removal under way.
func (r *RunhostsAPI) Cancel() {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}

// Add sets a server up as a runhost and pairs this computer with it: the
// preflight's findings are put to the person in a native dialog before
// anything changes there, and an unknown host key too. Each step is said on
// runhost:say. The runhost is recorded once its install succeeds, so a
// failed check can still be looked at and removed.
func (r *RunhostsAPI) Add(req RunhostAddRequest) (*RunhostView, error) {
	ver := carriedCLIVersion()
	if ver == "" {
		return nil, errors.New("this build of Agentry carries no released CLI, so there is no signed build of it to put on a server: set up a runhost from a release of the app")
	}
	if err := runhost.ValidName(req.Name); err != nil {
		return nil, err
	}
	port := req.Port
	if port == 0 {
		port = 22
	}
	answer, err := targethost.ParseAnswer(fmt.Sprintf("%s@%s:%d", strings.TrimSpace(req.Admin.User), strings.TrimSpace(req.Host), port))
	if err != nil {
		return nil, err
	}
	auth, err := r.adminAuth(req.Admin)
	if err != nil {
		return nil, err
	}
	keys, err := releaseKeys()
	if err != nil {
		return nil, err
	}
	home, err := r.home()
	if err != nil {
		return nil, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, err
	}
	if old, ok := recs.Find(req.Name); ok && (old.Host != answer.Host || old.Port != answer.Port) {
		return nil, fmt.Errorf("a runhost named %s is already %s:%d: remove it first, or choose another name", req.Name, old.Host, old.Port)
	}
	deviceID, err := runhost.DeviceID(home)
	if err != nil {
		return nil, err
	}
	ctx, done, err := r.begin()
	if err != nil {
		return nil, err
	}
	defer done()
	label, _ := os.Hostname()
	ask := dialogAsker{app: r.app}
	o := rhsetup.Options{
		Name: req.Name, Answer: answer, Auth: auth,
		KnownHosts: targethost.DefaultKnownHostsFiles(home),
		Ask:        ask, Say: r.say(req.Name),
		Version: ver, Keys: keys,
		Owner:    r.owner(),
		DeviceID: deviceID, DeviceLabel: label,
		Reach: reachHosts(),
		Confirm: func(p rhsetup.Plan) (bool, error) {
			return ask.Confirm(planSentence(answer.Host, p), false)
		},
	}
	out, err := rhsetup.Add(ctx, o)
	if out != nil && out.Record.Name != "" {
		recs.Put(out.Record)
		if serr := recs.Save(home); serr != nil {
			return nil, errors.Join(err, serr)
		}
	}
	if err != nil {
		return nil, err
	}
	s, _ := settings.Load()
	v := viewOf(out.Record, s.RunWhere)
	return &v, nil
}

// planSentence is the preflight's outcome as the person is asked to agree to
// it.
func planSentence(host string, p rhsetup.Plan) string {
	var b strings.Builder
	what := "the agentry account, gVisor, this app's CLI and the runhost's service"
	if p.InstallDocker {
		what = "Docker Engine, " + what
	}
	fmt.Fprintf(&b, "Install %s on %s?\n\n", what, host)
	for _, f := range p.Findings {
		if f.Level == rhsetup.LevelOK {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", f.Name, f.Detail)
	}
	return strings.TrimSpace(b.String())
}

// reachHosts are the hosts whose port 443 a runhost must reach: the image
// registry, Docker's repository and the registry this app uses.
func reachHosts() []string {
	reach := []string{"ghcr.io", rhsetup.DockerRepoHost}
	s, _ := settings.Load()
	if u, err := url.Parse(s.DefaultRegistry); err == nil && u.Hostname() != "" && u.Hostname() != "localhost" {
		reach = append(reach, u.Hostname())
	}
	return reach
}

// owner is the Agentry account this app is signed in to on its default
// registry, else the paired devices alone.
func (r *RunhostsAPI) owner() runhost.Owner {
	s, _ := settings.Load()
	reg := settings.Normalize(s.DefaultRegistry)
	if reg == "" || settings.TokenFor(reg) == "" {
		return runhost.Owner{Kind: runhost.OwnerDevices}
	}
	u, err := whoami(r.app, reg, settings.TokenFor(reg))
	if err != nil || u == nil || u.Handle == "" {
		return runhost.Owner{Kind: runhost.OwnerDevices}
	}
	return runhost.Owner{Kind: runhost.OwnerAccount, Registry: reg, Handle: u.Handle}
}

// SignIn signs runhost name in to a plan, vendor claude, chatgpt or copilot,
// from this computer's browser: the sign-in runs on the runhost, and the
// vendor's page opens here (signin:url, signin:say, signin:code, as the app's
// own sign-ins do). It shares the app's one-consent lock.
func (r *RunhostsAPI) SignIn(name, vendor string) (*RunhostView, error) {
	if !signin.IsVendor(vendor) {
		return nil, fmt.Errorf("a runhost signs in to claude, chatgpt or copilot, not %q", vendor)
	}
	if !consentNow.mu.TryLock() {
		return nil, errors.New("a sign-in is already under way: finish it in the browser, or cancel it")
	}
	defer consentNow.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.ctx(), 30*time.Minute)
	defer cancel()
	setConsent(cancel, nil)
	defer setConsent(nil, nil)
	recs, rec, keys, home, err := r.record(name)
	if err != nil {
		return nil, err
	}
	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	if v, err := runhostVersion(ctx, c); err != nil {
		return nil, err
	} else if v.Version != carriedCLIVersion() {
		return nil, fmt.Errorf("%s runs CLI %s and this app %s; update the runhost first", name, v.Version, carriedCLIVersion())
	}
	ui := r.run.signInUI("runhost:" + vendor)
	kept, err := signin.Run(ctx, c, vendor, "", signin.Person{Say: ui.Say, Open: ui.Open, Code: ui.Code})
	if err != nil {
		return nil, err
	}
	rec.SignIns = upsertSignIn(rec.SignIns, runhost.SignInRecord{Vendor: kept.Vendor, Label: kept.Label, AddedAt: time.Now().UTC().Format(time.RFC3339)})
	if err := recs.Save(home); err != nil {
		return nil, err
	}
	s, _ := settings.Load()
	v := viewOf(*rec, s.RunWhere)
	return &v, nil
}

func upsertSignIn(list []runhost.SignInRecord, s runhost.SignInRecord) []runhost.SignInRecord {
	for i := range list {
		if list[i].Vendor == s.Vendor {
			list[i] = s
			return list
		}
	}
	return append(list, s)
}

// RemoveSignIn ends runhost name's sign-in to vendor there.
func (r *RunhostsAPI) RemoveSignIn(name, vendor string) (*RunhostView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 5*time.Minute)
	defer cancel()
	recs, rec, keys, home, err := r.record(name)
	if err != nil {
		return nil, err
	}
	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	if _, err := remote.RemoveSignIn(ctx, c, vendor, r.say(name)); err != nil {
		return nil, err
	}
	kept := rec.SignIns[:0]
	for _, s := range rec.SignIns {
		if s.Vendor != vendor {
			kept = append(kept, s)
		}
	}
	rec.SignIns = kept
	if err := recs.Save(home); err != nil {
		return nil, err
	}
	s, _ := settings.Load()
	v := viewOf(*rec, s.RunWhere)
	return &v, nil
}

// unsafeModeMeaning is what Unsafe Mode on a runhost does, as the person is
// asked before turning it on.
const unsafeModeMeaning = "In Unsafe Mode a release's declared commands are installed from the Unsafe Mode image (a Kali Linux base) and run with that image's whole PATH, without the command catalogue. Every run on this runhost, from every paired device, is compiled this way until it is turned off. Runs stay sandboxed under gVisor, behind the egress proxy."

// SetUnsafeMode turns runhost name's Unsafe Mode on or off with the server's
// admin sign-in, after a native confirmation when turning it on.
func (r *RunhostsAPI) SetUnsafeMode(name string, on bool, admin RunhostAdmin) error {
	_, rec, _, _, err := r.record(name)
	if err != nil && rec == nil {
		return err
	}
	if on {
		if err := r.app.askConsent("Turn Unsafe Mode on for "+name+"?", unsafeModeMeaning); err != nil {
			return err
		}
	}
	auth, err := r.adminAuth(admin)
	if err != nil {
		return err
	}
	ctx, done, err := r.begin()
	if err != nil {
		return err
	}
	defer done()
	return rhsetup.SetUnsafeMode(ctx, rhsetup.SetOptions{
		Record: *rec, Auth: auth, Answer: targethost.Answer{User: strings.TrimSpace(admin.User), Host: rec.Host, Port: rec.Port},
		Ask: dialogAsker{app: r.app}, Say: r.say(name),
	}, on)
}

// SetDefault makes runhost name where runs go unless the run setup says
// otherwise.
func (r *RunhostsAPI) SetDefault(name string) error {
	home, err := r.home()
	if err != nil {
		return err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return err
	}
	if _, ok := recs.Find(name); !ok {
		return fmt.Errorf("no runhost is named %q on this computer", name)
	}
	return setRunWhere(settings.RunWhereRunhostPrefix + name)
}

// Remove takes runhost name off its server with the admin sign-in, after a
// native confirmation: what it holds of the person's (its credits token, its
// model sign-ins) ends first over the run key, then the server is cleaned,
// and this computer forgets it.
func (r *RunhostsAPI) Remove(name string, admin RunhostAdmin, removeImages, removeDocker bool) error {
	recs, rec, keys, home, err := r.record(name)
	if err != nil && rec == nil {
		return err
	}
	if err := r.app.askConsent("Remove runhost "+name+"?", "Agentry's account, its service and everything in its home are taken off "+rec.Host+". Its Agentry credits token is revoked and its model sign-ins end."); err != nil {
		return err
	}
	auth, err := r.adminAuth(admin)
	if err != nil {
		return err
	}
	ctx, done, err := r.begin()
	if err != nil {
		return err
	}
	defer done()
	say := r.say(name)
	if keys != nil {
		if c, err := runhost.DialRunhost(ctx, *rec, keys); err != nil {
			say("warning: the runhost could not be reached over its run key (" + err.Error() + "): revoke its credits token among your tokens and end its model sign-ins at the vendors")
		} else {
			e := remote.EndAll(ctx, c, say)
			_ = c.Close()
			for what, ferr := range e.Failed {
				say(fmt.Sprintf("warning: %s could not be ended (%v)", what, ferr))
			}
		}
	}
	if err := rhsetup.Remove(ctx, rhsetup.RemoveOptions{
		Record: *rec, Auth: auth, Answer: targethost.Answer{User: strings.TrimSpace(admin.User), Host: rec.Host, Port: rec.Port},
		Ask: dialogAsker{app: r.app}, Say: say, RemoveImages: removeImages, RemoveDocker: removeDocker,
	}); err != nil {
		return err
	}
	if err := runhost.DeleteKeys(name); err != nil {
		say("warning: this computer's keys for " + name + " could not be removed from the system keychain: " + err.Error())
	}
	recs.Drop(name)
	if err := recs.Save(home); err != nil {
		return err
	}
	if s, _ := settings.Load(); s.RunWhere == settings.RunWhereRunhostPrefix+name {
		return setRunWhere(settings.RunWhereHere)
	}
	return nil
}

// ChooseAdminKeyFile opens the native file dialog on ~/.ssh for a server's
// admin key, and returns the path ("" when cancelled).
func (r *RunhostsAPI) ChooseAdminKeyFile(host string) (string, error) {
	return r.run.ChooseHostKeyFile(host)
}

// Credits gives runhost name a token of its own on the person's account that
// can only pay for model use with their Agentry credits, named "Runhost
// <name> (credits only)": the registry's authorize page opens in the browser
// to approve it, and the token goes over the run key to the runhost's store,
// encrypted. This computer's own sign-in never reaches the server, and the
// token is kept nowhere here. Running it again replaces the token, and the
// runhost revokes the earlier one. It shares the app's one-consent lock.
func (r *RunhostsAPI) Credits(name string) (*RunhostView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	reg := settings.Normalize(s.DefaultRegistry)
	owner := r.owner()
	if owner.Kind != runhost.OwnerAccount {
		return nil, errors.New("a runhost's credits token is made on your Agentry account: sign in first")
	}
	recs, rec, keys, home, err := r.record(name)
	if err != nil {
		return nil, err
	}
	if rec.Owner.Kind == runhost.OwnerAccount && (rec.Owner.Handle != owner.Handle || settings.Normalize(rec.Owner.Registry) != reg) {
		return nil, fmt.Errorf("runhost %s belongs to %s, and this app is signed in as %s on %s", name, rec.Owner, owner.Handle, reg)
	}
	if err := r.app.askConsent("Give "+name+" your Agentry credits?",
		fmt.Sprintf("A token on your account %s that can only pay for model use with your Agentry credits, named \"Runhost %s (credits only)\", is kept on %s, encrypted. Your browser opens to approve it.", owner.Handle, name, rec.Host)); err != nil {
		return nil, err
	}
	if !consentNow.mu.TryLock() {
		return nil, errors.New("a sign-in is already under way: finish it in the browser, or cancel it")
	}
	defer consentNow.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.ctx(), 10*time.Minute)
	defer cancel()
	setConsent(cancel, nil)
	defer setConsent(nil, nil)
	ui := r.run.signInUI("runhost:agentry")
	pat, err := remote.MintCredits(ctx, reg, name, ui.Open, func() { ui.Say("Waiting for you to approve the token in your browser…") })
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 30 * time.Second}
	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return nil, errors.Join(err, remote.RevokeUnsent(r.ctx(), hc, reg, pat))
	}
	defer func() { _ = c.Close() }()
	doc := remote.Credits{Registry: reg, TokenID: pat.ID, Name: pat.Name, Token: pat.Token, Device: rec.DeviceID}
	if err := remote.PutCredits(ctx, c, doc, r.say(name)); err != nil {
		return nil, errors.Join(err, remote.RevokeUnsent(r.ctx(), hc, reg, pat))
	}
	rec.Credits = &runhost.CreditsRecord{Registry: reg, TokenID: pat.ID, TokenName: pat.Name, AddedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := recs.Save(home); err != nil {
		return nil, err
	}
	v := viewOf(*rec, s.RunWhere)
	return &v, nil
}

// RemoveCredits has runhost name revoke its credits token and remove it.
func (r *RunhostsAPI) RemoveCredits(name string) (*RunhostView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 2*time.Minute)
	defer cancel()
	recs, rec, keys, home, err := r.record(name)
	if err != nil {
		return nil, err
	}
	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	out, err := remote.RemoveCredits(ctx, c)
	if err != nil {
		return nil, err
	}
	if out.Credits != nil && !out.Revoked {
		r.say(name)(fmt.Sprintf("warning: the token %q left the runhost, but its registry did not revoke it (%s): revoke it among your tokens", out.Credits.Name, out.RevokeError))
	}
	rec.Credits = nil
	if err := recs.Save(home); err != nil {
		return nil, err
	}
	s, _ := settings.Load()
	v := viewOf(*rec, s.RunWhere)
	return &v, nil
}
