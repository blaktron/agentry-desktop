package app

// A runhost runs the exact CLI version this app carries. After the app
// updates itself, PostUpdate brings every paired runhost to the CLI the app
// now carries, over each one's run key, with the signed release checked here
// and checked again there, and records how each went; Settings → Runhosts
// shows that, updates one again, pairs this computer with a runhost set up
// from another, and takes this computer off one.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	rhsetup "github.com/blaktron/agentry-cli/pkg/runhost/setup"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// runhostUpdatesFile keeps the post-update step's record beside the app's
// settings: the CLI version it last ran for, and each runhost's last update.
const runhostUpdatesFile = "runhost-updates.json"

// RunhostUpdateView is a runhost's last update, as this app recorded it.
type RunhostUpdateView struct {
	At string `json:"at"`
	// From and To are the runhost's CLI before and after.
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Updated bool   `json:"updated"`
	// WaitsForJobs is how many running jobs the runhost finishes before it
	// restarts into the new CLI.
	WaitsForJobs int `json:"waitsForJobs,omitempty"`
	// DeviceBehind is the runhost's CLI when it is newer than this app's:
	// this app is the one to update.
	DeviceBehind string `json:"deviceBehind,omitempty"`
	// NeedsAttention says what only the admin sign-in can change.
	NeedsAttention []string `json:"needsAttention,omitempty"`
	// Problem is why the runhost could not be updated, such as its not
	// being reached.
	Problem string `json:"problem,omitempty"`
}

type runhostUpdates struct {
	For      string                       `json:"for"`
	Runhosts map[string]RunhostUpdateView `json:"runhosts"`
}

// runhostUpdatesPath is where the record is kept; a test replaces it.
var runhostUpdatesPath = func() (string, error) {
	p, err := settings.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), runhostUpdatesFile), nil
}

func loadRunhostUpdates() runhostUpdates {
	u := runhostUpdates{Runhosts: map[string]RunhostUpdateView{}}
	p, err := runhostUpdatesPath()
	if err != nil {
		return u
	}
	if b, err := os.ReadFile(p); err == nil { //nolint:gosec // the app's own file
		_ = json.Unmarshal(b, &u)
	}
	if u.Runhosts == nil {
		u.Runhosts = map[string]RunhostUpdateView{}
	}
	return u
}

func (u runhostUpdates) save() error {
	p, err := runhostUpdatesPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// runhostUpdate is remote.Update, and runhostCarriedCLI the CLI version this
// app carries; a test replaces them.
var (
	runhostUpdate     = remote.Update
	runhostCarriedCLI = carriedCLIVersion
)

// updateOne brings rec's runhost to the CLI this app carries, changing rec's
// versions in place, and says how it went.
func (r *RunhostsAPI) updateOne(ctx context.Context, rec *runhost.Record, ver string) RunhostUpdateView {
	v := RunhostUpdateView{At: time.Now().UTC().Format(time.RFC3339), From: rec.CLIVersion}
	keys, err := runhost.LoadKeys(rec.Name)
	if err != nil {
		v.Problem = err.Error()
		return v
	}
	rk, err := releaseKeys()
	if err != nil {
		v.Problem = err.Error()
		return v
	}
	out, err := runhostUpdate(ctx, rec, keys, remote.UpdateOptions{Version: ver, Keys: rk, Say: r.say(rec.Name)})
	if err != nil {
		v.Problem = err.Error()
		return v
	}
	v.From, v.To, v.Updated, v.WaitsForJobs = out.From, out.To, out.Updated, out.WaitsForJobs
	v.DeviceBehind, v.NeedsAttention = out.DeviceBehind, out.NeedsAttention
	return v
}

// updateNamed updates the named runhosts, saves their records and keeps
// each outcome.
func (r *RunhostsAPI) updateNamed(ctx context.Context, names []string, ver string) error {
	home, err := r.home()
	if err != nil {
		return err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return err
	}
	u := loadRunhostUpdates()
	for _, n := range names {
		rec, ok := recs.Find(n)
		if !ok {
			continue
		}
		u.Runhosts[n] = r.updateOne(ctx, rec, ver)
	}
	if err := recs.Save(home); err != nil {
		return err
	}
	return u.save()
}

// PostUpdate is the step after every update of this app: the first time a
// version of the app carries a CLI it has not run this for, it updates each
// runhost on another CLI version, one at a time, and records each outcome.
// Later calls with the same CLI do nothing. A runhost it cannot reach keeps
// its outcome for Settings, where Update tries it again.
func (r *RunhostsAPI) PostUpdate() ([]RunhostView, error) {
	ver := runhostCarriedCLI()
	if ver == "" {
		return []RunhostView{}, nil
	}
	u := loadRunhostUpdates()
	if u.For == ver {
		return []RunhostView{}, nil
	}
	home, err := r.home()
	if err != nil {
		return nil, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, err
	}
	// With no runhost there is nothing to update or record, and a fresh
	// install writes nothing.
	if len(recs.Runhosts) == 0 {
		return []RunhostView{}, nil
	}
	var stale []string
	for _, rec := range recs.Runhosts {
		if rec.CLIVersion != ver {
			stale = append(stale, rec.Name)
		}
	}
	if len(stale) > 0 {
		ctx, done, err := r.begin()
		if err != nil {
			// A setup or a change is under way: the next start tries again.
			return []RunhostView{}, nil
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		err = r.updateNamed(ctx, stale, ver)
		cancel()
		done()
		if err != nil {
			return nil, err
		}
	}
	u = loadRunhostUpdates()
	u.For = ver
	if err := u.save(); err != nil {
		return nil, err
	}
	all, err := r.List()
	if err != nil {
		return nil, err
	}
	out := []RunhostView{}
	for _, v := range all {
		for _, n := range stale {
			if v.Name == n {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

// Update brings runhost name to the CLI this app carries now, over its run
// key, and records the outcome.
func (r *RunhostsAPI) Update(name string) (*RunhostView, error) {
	ver := runhostCarriedCLI()
	if ver == "" {
		return nil, errors.New("this build of Agentry carries no released CLI, so there is no signed build of it to send a runhost")
	}
	ctx, done, err := r.begin()
	if err != nil {
		return nil, err
	}
	defer done()
	if err := r.updateNamed(ctx, []string{name}, ver); err != nil {
		return nil, err
	}
	return r.view(name)
}

// view is runhost name as List shows it.
func (r *RunhostsAPI) view(name string) (*RunhostView, error) {
	all, err := r.List()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("no runhost is named %q on this computer", name)
}

// Pair pairs this computer with a runhost set up from another of the
// person's: with the server's admin sign-in, for this call alone, it
// installs this computer's own run key and sealing key and nothing else.
// The runhost must belong to the account this app is signed in to; one set
// up signed out is bound to it. An unknown host key is put to the person in
// a native dialog. A runhost on an older CLI than this app's is then
// updated to it over the new run key.
func (r *RunhostsAPI) Pair(req RunhostAddRequest) (*RunhostView, error) {
	ver := runhostCarriedCLI()
	if ver == "" {
		return nil, errors.New("this build of Agentry carries no released CLI, so it cannot pair with a runhost: use a release of the app")
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
	home, err := r.home()
	if err != nil {
		return nil, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, err
	}
	if _, ok := recs.Find(req.Name); ok {
		return nil, fmt.Errorf("this computer already pairs with a runhost named %s", req.Name)
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
	out, err := rhsetup.Pair(ctx, rhsetup.PairOptions{
		Name: req.Name, Answer: answer, Auth: auth,
		KnownHosts: targethost.DefaultKnownHostsFiles(home),
		Ask:        dialogAsker{app: r.app}, Say: r.say(req.Name),
		Version: ver, Owner: r.owner(),
		DeviceID: deviceID, DeviceLabel: label,
	})
	if out != nil && out.Record.Name != "" {
		recs.Put(out.Record)
		if serr := recs.Save(home); serr != nil {
			return nil, errors.Join(err, serr)
		}
	}
	if err != nil {
		return nil, err
	}
	if out.Record.CLIVersion != ver {
		if err := r.updateNamed(ctx, []string{req.Name}, ver); err != nil {
			return nil, err
		}
	}
	return r.view(req.Name)
}

// Unpair takes this computer off runhost name, over its run key: the
// runhost drops this computer's keys and revokes the credits token this
// computer gave it, and the runhost and its other devices go on. Then this
// computer's keys and record of it are removed. The last device of a
// runhost cannot unpair: removing the runhost does that.
func (r *RunhostsAPI) Unpair(name string) error {
	recs, rec, keys, home, err := r.record(name)
	if err != nil {
		return err
	}
	if err := r.app.askConsent("Unpair this computer from runhost "+name+"?", "The runhost on "+rec.Host+" drops this computer's keys, and revokes the Agentry credits token this computer gave it. It goes on for your other computers."); err != nil {
		return err
	}
	ctx, done, err := r.begin()
	if err != nil {
		return err
	}
	defer done()
	say := r.say(name)
	out, err := remote.Unpair(ctx, *rec, keys, say)
	if err != nil {
		return err
	}
	if out.Credits.Credits != nil && !out.Credits.Revoked {
		say(fmt.Sprintf("warning: the credits token %q is removed from the runhost but %s did not revoke it (%s): revoke it among your tokens there", out.Credits.Credits.Name, out.Credits.Credits.Registry, out.Credits.RevokeError))
	}
	if err := runhost.DeleteKeys(name); err != nil {
		say("warning: this computer's keys for " + name + " could not be removed from the system keychain: " + err.Error())
	}
	if jobs, err := remote.LoadJobs(home); err == nil {
		for _, j := range jobs {
			if j.Runhost == name {
				remote.DropJob(home, j.Job)
			}
		}
	}
	recs.Drop(name)
	if err := recs.Save(home); err != nil {
		return err
	}
	u := loadRunhostUpdates()
	delete(u.Runhosts, name)
	_ = u.save()
	if s, _ := settings.Load(); s.RunWhere == settings.RunWhereRunhostPrefix+name {
		return setRunWhere(settings.RunWhereHere)
	}
	return nil
}
