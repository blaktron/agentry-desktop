package app

// A runhost's saved models from the app: Settings → Models' "Available on"
// makes one of this computer's saved models available on a runhost, through
// the relay its credential takes (its key sent to the runhost's store, a
// sign-in made on the runhost from this computer's browser, a credits token
// minted for it), and the runhost checks it before it keeps it. A runhost's
// agents and schedules name only what it holds. The rules are the CLI's
// (remote.Prepare, remote.PutSavedModel and the rest); the consent, the
// sign-in relay and the credits token are this app's own, as for a
// runhost's other credentials.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	"github.com/blaktron/agentry-cli/pkg/runner/modelcheck"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// runhostModelTimeout bounds making a saved model available on a runhost:
// its check there may build a harness's image the first time.
const runhostModelTimeout = 15 * time.Minute

// RunhostModelsView is a runhost's saved models as it says them: never a
// credential.
type RunhostModelsView struct {
	Runhost string              `json:"runhost"`
	Models  []remote.SavedModel `json:"models"`
	// Default is the default's id; "" when the runhost holds none.
	Default string `json:"default"`
	Max     int    `json:"max"`
}

func modelsView(name string, list remote.Models) *RunhostModelsView {
	v := &RunhostModelsView{Runhost: name, Models: list.Models, Default: list.Default, Max: list.Max}
	if v.Models == nil {
		v.Models = []remote.SavedModel{}
	}
	return v
}

// rememberModels keeps what runhost name said of its saved models in this
// computer's record of it, which Settings → Models reads for "Available on"
// until it asks again.
func (r *RunhostsAPI) rememberModels(name string, list remote.Models) {
	home, err := r.home()
	if err != nil {
		return
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return
	}
	rec, ok := recs.Find(name)
	if !ok {
		return
	}
	rec.Models = []runhost.ModelRecord{}
	for _, sm := range list.Models {
		rec.Models = append(rec.Models, runhost.ModelRecord{ID: sm.ID, Name: sm.Name, Provider: sm.Provider, Model: sm.Model, Default: sm.Default})
	}
	_ = recs.Save(home)
}

// dialSame reaches runhost name and holds it to the CLI release this app
// carries, which its saved-model verbs need.
func (r *RunhostsAPI) dialSame(ctx context.Context, name string) (*runhost.Record, *runhost.Client, error) {
	rec, _, c, err := r.dial(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	v, err := runhostVersion(ctx, c)
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	if carried := carriedCLIVersion(); v.Version != carried {
		_ = c.Close()
		return nil, nil, fmt.Errorf("runhost %s runs CLI %s and this app carries %s: update it from its card first", name, v.Version, carried)
	}
	return rec, c, nil
}

// Models asks runhost name for the saved models it holds, each with why a
// run on it would be refused there now and what names it.
func (r *RunhostsAPI) Models(name string) (*RunhostModelsView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), time.Minute)
	defer cancel()
	_, c, err := r.dialSame(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	list, err := remote.ListModels(ctx, c)
	if err != nil {
		return nil, err
	}
	r.rememberModels(name, list)
	return modelsView(name, list), nil
}

// MakeModelAvailable makes this computer's saved model id available on
// runhost name. What a runhost cannot take is refused before anything is
// sent. A key is read here and, once the person agrees in a native dialog,
// sent to the runhost's store; a sign-in the runhost does not keep is made
// there from this computer's browser; a credits token it does not keep is
// minted for it, after its own consent. A Claude subscription's model is
// checked there by one message, which the person agrees to first. The
// runhost then checks the model and keeps it, or refuses with the reason
// and keeps nothing. Nothing of this computer's own sign-ins is sent.
func (r *RunhostsAPI) MakeModelAvailable(name, id string) (*RunhostModelsView, error) {
	pol, err := devicePolicy()
	if err != nil {
		return nil, err
	}
	prep, err := remote.Prepare(pol, id, run.RenewedHere)
	if err != nil {
		return nil, err
	}
	sm, m := prep.Saved, prep.Model
	what := fmt.Sprintf("%q (%s/%s)", sm.Name, sm.Provider, sm.Model)
	ctx, cancel := context.WithTimeout(r.ctx(), runhostModelTimeout)
	defer cancel()
	rec, c, err := r.dialSame(ctx, name)
	if err != nil {
		return nil, err
	}
	doc := prep.Doc
	switch prep.Kind {
	case remote.ModelKindKey:
		_ = c.Close()
		if err := r.app.askConsent("Make "+sm.Name+" available on "+name+"?",
			fmt.Sprintf("Your %s key is kept on runhost %s (%s), encrypted in its store, so its persistent agents and schedules can run on %s. The runhost checks the model before it keeps it.", sm.Provider, name, rec.Host, what)); err != nil {
			return nil, err
		}
		key, err := modelkey.Personal(m.APIKeyEnv, m.APIKeySource, m.KeyProvider())
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, fmt.Errorf("no key for the saved model %s on this computer: add it in Settings → Models first", what)
		}
		doc.Key = key
	case remote.ModelKindSignIn:
		vendor := modelsignin.VendorOf(m.APIKeyKind)
		kept := remote.KeepsSignIn(ctx, c, vendor)
		_ = c.Close()
		if !kept {
			r.say(name)(fmt.Sprintf("runhost %s keeps no working %s sign-in: it signs in now, from your browser here", name, vendor))
			if _, err := r.SignIn(name, vendor); err != nil {
				return nil, err
			}
		}
		if m.APIKeyKind == policy.KeyKindClaudeSignIn {
			if err := r.app.askConsent("Check "+sm.Name+" on "+name+"?", modelcheck.ClaudeNotice); err != nil {
				return nil, err
			}
			doc.Yes = true
		}
	case remote.ModelKindCredits:
		reg, err := policy.AgentryRegistry(m)
		if err != nil {
			_ = c.Close()
			return nil, err
		}
		kept := remote.KeepsCredits(ctx, c, reg)
		_ = c.Close()
		if !kept {
			r.say(name)(fmt.Sprintf("runhost %s keeps no credits token for %s: one is made for it now", name, reg))
			if _, err := r.Credits(name); err != nil {
				return nil, err
			}
		}
	default:
		_ = c.Close()
	}
	_, c, err = r.dialSame(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	ans, err := remote.PutSavedModel(ctx, c, doc, r.say(name))
	if err != nil {
		return nil, fmt.Errorf("runhost %s: %w", name, err)
	}
	if ans.Checked.Warning != "" {
		r.say(name)("warning: " + ans.Checked.Warning)
	}
	r.rememberModels(name, ans.Models)
	return modelsView(name, ans.Models), nil
}

// RemoveRunhostModel takes the saved model ref off runhost name. While one
// of its agents or schedules names it, the runhost refuses unless moveTo
// names another of its saved models to move them to; removing its default
// needs moveTo too. Its provider's credential leaves the runhost with its
// last saved model unless keepCredential.
func (r *RunhostsAPI) RemoveRunhostModel(name, ref, moveTo string, keepCredential bool) (*RunhostModelsView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 5*time.Minute)
	defer cancel()
	_, c, err := r.dialSame(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	out, err := remote.RemoveSavedModel(ctx, c, remote.RemoveModel{Ref: ref, MoveTo: moveTo, KeepCredential: keepCredential}, r.say(name))
	if err != nil {
		return nil, fmt.Errorf("runhost %s: %w", name, err)
	}
	r.rememberModels(name, out.Models)
	r.forgetEnded(ctx, c, name)
	if out.Credential != "" && out.Credential != "removed" && out.Credential != "kept" {
		r.say(name)("warning: " + out.Credential)
	}
	return modelsView(name, out.Models), nil
}

// forgetEnded brings this computer's record of runhost name's sign-ins and
// credits token in line with what its store keeps, after a removal that may
// have ended them there.
func (r *RunhostsAPI) forgetEnded(ctx context.Context, c *runhost.Client, name string) {
	list, err := remote.ListStore(ctx, c)
	if err != nil {
		return
	}
	recs, rec, _, home, err := r.record(name)
	if err != nil && rec == nil {
		return
	}
	kept := rec.SignIns[:0]
	for _, s := range rec.SignIns {
		for _, k := range list.SignIns {
			if k.Vendor == s.Vendor {
				kept = append(kept, s)
				break
			}
		}
	}
	rec.SignIns = kept
	if list.Credits == nil {
		rec.Credits = nil
	}
	_ = recs.Save(home)
}

// SetRunhostDefaultModel makes the saved model ref runhost name's default:
// its agents and schedules that name none run on it.
func (r *RunhostsAPI) SetRunhostDefaultModel(name, ref string) (*RunhostModelsView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), time.Minute)
	defer cancel()
	_, c, err := r.dialSame(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	list, err := remote.SetDefaultModel(ctx, c, ref)
	if err != nil {
		return nil, fmt.Errorf("runhost %s: %w", name, err)
	}
	r.rememberModels(name, list)
	return modelsView(name, list), nil
}

// runhostModelID is the id of the saved model ref names among those
// runhost name holds: "" and "default" are its default. One it does not
// hold is refused by name, before anything is changed.
func (r *RunhostsAPI) runhostModelID(ctx context.Context, name, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.EqualFold(ref, "default") {
		return "default", nil
	}
	_, c, err := r.dialSame(ctx, name)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	list, err := remote.ListModels(ctx, c)
	if err != nil {
		return "", err
	}
	r.rememberModels(name, list)
	sm, ok := list.Find(ref)
	if !ok {
		return "", fmt.Errorf("runhost %s holds no saved model %q: make it available there in Settings → Models first", name, ref)
	}
	return sm.ID, nil
}

// SetAgentModel puts agent on runhost name on one of the runhost's saved
// models, by its id or name, or back on the runhost's default ("" or
// "default"). A running agent takes it at its next turn there, with no stop
// and nothing new to agree to.
func (r *RunhostsAPI) SetAgentModel(name, agent, ref string) ([]RunhostAgentView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 2*time.Minute)
	defer cancel()
	id, err := r.runhostModelID(ctx, name, ref)
	if err != nil {
		return nil, err
	}
	code, _, stderr, err := r.agentCommand(ctx, name, "configure", agent, "--model", id)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, failed("configure", code, stderr)
	}
	return r.Agents(name)
}

// devicePolicy is this computer's policy, which holds the saved models a
// runhost is given.
func devicePolicy() (*policy.Policy, error) {
	pf, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.Load(pf.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	if !run.ModelsSetUp(pol) {
		return nil, errors.New("this computer keeps no saved model yet: add one in Settings → Models first")
	}
	return pol, nil
}
