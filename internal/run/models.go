// Saved models and providers, as Settings → Models shows and changes them.
// A provider is a credential: a vendor's key, a model sign-in, a connected
// account, Agentry credits, or an endpoint of the person's own. A saved
// model is a provider plus a model id, under a name the person gives it.
// The computer keeps up to ten, and the default is what a run, a schedule
// or an agent uses unless it names another.
//
// Every rule is the CLI's, called in process: the policy package holds the
// lists, the cap, the names and the default; modelcheck checks a model
// before it is saved; savedmodels knows what names a model, refuses a
// removal until those are moved, and says whether a provider's credential is
// here; setup makes a named vendor's provider as `agentry setup` does. This
// file wires them to this app's keychain, sign-ins and registry sign-in.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/inference"
	"github.com/blaktron/agentry-cli/pkg/runner/modelcheck"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/savedmodels"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The ways a provider is paid, as the page names them.
const (
	WayKey     = "key"
	WayChatGPT = "chatgpt"
	WayClaude  = "claude"
	WayCopilot = "copilot"
	WayConnect = "connect"
	WayCredits = "credits"
)

// Provider is one provider as the page lists it: never a key or a token.
type Provider struct {
	ID string `json:"id"`
	// Vendor is the provider's vendor: custom for an endpoint of the
	// person's own.
	Vendor string `json:"vendor"`
	// Name is an endpoint of the person's own's name; BaseURL its address.
	Name    string `json:"name,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
	// Way is how it is paid (WayKey, WayChatGPT, …); KeyEnv and KeySource
	// where a key is read from.
	Way       string `json:"way"`
	KeyEnv    string `json:"keyEnv,omitempty"`
	KeySource string `json:"keySource,omitempty"`
	// TakesNoKey: an endpoint of the person's own saved without a key.
	TakesNoKey bool `json:"takesNoKey,omitempty"`
	// State is savedmodels' code: ready, key-missing or needs-sign-in.
	State string `json:"state"`
	// Account is the sign-in's account, or Agentry credits' registry.
	Account string `json:"account,omitempty"`
	// Broken is why a kept sign-in no longer works.
	Broken string `json:"broken,omitempty"`
	// SignInVendor is the sign-in store's vendor (chatgpt, claude,
	// copilot, or a connectable vendor's own id).
	SignInVendor string `json:"signInVendor,omitempty"`
	// CopilotPlan is a Copilot provider's plan.
	CopilotPlan string `json:"copilotPlan,omitempty"`
	// Models are the ids of the saved models on it.
	Models []string `json:"models"`
}

// SavedModel is one saved model as the page lists it.
type SavedModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ContextWindow int64  `json:"contextWindow,omitempty"`
	Default       bool   `json:"default"`
	// Users are the schedules and persistent agents that name it.
	Users []savedmodels.User `json:"users"`
}

// ModelsList is the computer's providers and saved models.
type ModelsList struct {
	Providers []Provider   `json:"providers"`
	Models    []SavedModel `json:"models"`
	Default   string       `json:"default"`
	Max       int          `json:"max"`
	// SetUp: a person chose a model here. A policy written from defaults
	// (by an AI Review key's save, a coding agent's choice or a first run)
	// holds a model nobody chose, and it is listed as none.
	SetUp      bool   `json:"setUp"`
	PolicyPath string `json:"policyPath"`
	// UsersErr is why what names each model could not be read; the lists
	// are shown without it.
	UsersErr string `json:"usersErr,omitempty"`
}

// The seams the tests replace: no keychain, no provider, no registry.
var (
	listModelsToCheck = setup.ListModelsToCheck
	modelCheckers     = realCheckers
)

// readModelsPolicy is the policy Settings → Models changes: the file, or
// the defaults when there is none yet, and whether a person chose its
// model.
func readModelsPolicy() (*policy.Policy, string, bool, error) {
	path, err := policy.DefaultPath()
	if err != nil {
		return nil, "", false, err
	}
	pol, _, err := policy.Load(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return policy.Default(), path, false, nil
	case err != nil:
		return nil, "", false, fmt.Errorf("operator policy: %w", err)
	}
	return pol, path, setUp(pol), nil
}

// setUp reports whether a person chose pol's model: it keeps saved models
// of its own, or its model block is not the defaults'.
func setUp(pol *policy.Policy) bool {
	if pol.HasSavedModels() {
		return true
	}
	return !reflect.DeepEqual(pol.Model, policy.Default().Model)
}

// wayOf is how provider id is paid.
func wayOf(id string, pr policy.Provider) string {
	switch {
	case policy.ProviderVendor(id) == policy.ProviderAgentry:
		return WayCredits
	case pr.APIKeyKind == policy.KeyKindChatGPTPlan:
		return WayChatGPT
	case pr.APIKeyKind == policy.KeyKindClaudeSignIn:
		return WayClaude
	case pr.APIKeyKind == policy.KeyKindCopilotSignIn:
		return WayCopilot
	}
	return WayKey
}

// registryToken is this computer's sign-in at a registry, for Agentry
// credits' state.
func registryToken(reg string) (string, error) { return settings.TokenFor(reg), nil }

// ListSavedModels is the computer's providers and saved models, each provider's
// credential state and what names each model.
func ListSavedModels() (*ModelsList, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	out := &ModelsList{Max: policy.MaxSavedModels, SetUp: chosen, PolicyPath: path, Providers: []Provider{}, Models: []SavedModel{}}
	if !chosen {
		return out, nil
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	providers, models, def := pol.SavedModels()
	out.Default = def
	users, uerr := savedmodels.Users(home, pol, models)
	if uerr != nil {
		out.UsersErr = uerr.Error()
	}
	for _, sm := range models {
		row := SavedModel{ID: sm.ID, Name: sm.Name, Provider: sm.Provider, Model: sm.Model, ContextWindow: sm.ContextWindow,
			Default: sm.ID == def, Users: []savedmodels.User{}}
		for _, u := range users {
			if u.Model == sm.ID {
				row.Users = append(row.Users, u)
			}
		}
		out.Models = append(out.Models, row)
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		pr := providers[id]
		st := savedmodels.ProviderState(home, id, pr, registryToken)
		p := Provider{ID: id, Vendor: policy.ProviderVendor(id), Name: pr.Name, BaseURL: pr.BaseURL, Way: wayOf(id, pr),
			KeyEnv: pr.APIKeyEnv, KeySource: pr.APIKeySource, State: st.Code, Account: st.Account, Broken: st.Broken,
			SignInVendor: st.Vendor, Models: []string{}}
		if p.Way == WayKey && st.Kind == "connected account" {
			p.Way = WayConnect
		}
		p.TakesNoKey = policy.ProviderVendor(id) == policy.ProviderCustom && pr.APIKeyEnv == ""
		if p.Way == WayCopilot {
			p.CopilotPlan = policy.CopilotPlanOf(pr.ProxyAllow)
		}
		for _, sm := range models {
			if sm.Provider == id {
				p.Models = append(p.Models, sm.ID)
			}
		}
		out.Providers = append(out.Providers, p)
	}
	return out, nil
}

// realCheckers are the save check's askers on this computer, wired to this
// app's keychain, sign-ins and registry sign-in.
func realCheckers(home string, pol *policy.Policy, path string, logw io.Writer) modelcheck.Checkers {
	return modelcheck.Checkers{
		Key: func(m policy.Model) (string, error) {
			return modelkey.Personal(m.APIKeyEnv, m.APIKeySource, m.KeyProvider())
		},
		ListModels:         listModelsToCheck,
		ProbeContextWindow: probeContextWindow,
		ChatGPTModels:      ChatGPTModels,
		CopilotModels: func(ctx context.Context, plan string) ([]string, error) {
			got, err := CopilotModels(ctx, plan, logw)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(got))
			for _, m := range got {
				ids = append(ids, m.ID)
			}
			return ids, nil
		},
		Claude: func(ctx context.Context, model string) (modelcheck.ClaudeAnswer, error) {
			s := claude(home)
			s.Policy, s.PolicyPath = pol, path
			return s.CheckModel(ctx, model)
		},
		Credits: func(ctx context.Context, m policy.Model) error {
			c := CreditsClient(&policy.Policy{Model: m})
			if c == nil {
				return errors.New("this computer is not signed in to the registry the credits are paid through: sign in to your account first")
			}
			o, err := c.Open(ctx, inference.PurposeRun)
			if err != nil {
				return err
			}
			_, _ = c.End(context.WithoutCancel(ctx), o.Session.ID)
			return nil
		},
	}
}

// ModelDraft is a saved model as the page sends it to be saved.
type ModelDraft struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	// ContextWindow is what the person typed for an endpoint of their own,
	// 0 to take what the endpoint reports.
	ContextWindow int64 `json:"contextWindow"`
	MakeDefault   bool  `json:"makeDefault"`
	// AgreedToMessage is the person's yes to the Claude subscription's
	// check, which sends one message on their plan.
	AgreedToMessage bool `json:"agreedToMessage"`
}

// Refusal is a save the check refused: Kind is modelcheck's
// (not-offered, cannot-check, needs-sign-in, key-missing, not-eligible),
// Reason its words, Offered what the provider does offer where it said,
// and Retry whether the same save may be tried again as it is.
type Refusal struct {
	Kind    string   `json:"kind"`
	Reason  string   `json:"reason"`
	Offered []string `json:"offered"`
	Retry   bool     `json:"retry"`
}

// SaveResult is how a save ended: the saved model, or the check's refusal,
// or the notice the person must agree to before the check spends a message
// on their plan.
type SaveResult struct {
	Saved   *SavedModel `json:"saved,omitempty"`
	Refusal *Refusal    `json:"refusal,omitempty"`
	// NeedsAgreement is the sentence to put to the person before a Claude
	// subscription's model is checked; nothing was asked of anyone yet.
	NeedsAgreement string `json:"needsAgreement,omitempty"`
	// Resolved is the id a Claude alias resolved to at the check.
	Resolved string `json:"resolved,omitempty"`
	// Warning is the plain-http sentence for an endpoint reached
	// unencrypted.
	Warning string `json:"warning,omitempty"`
	// WindowSource says where an endpoint's reported window came from;
	// UnknownWindow is the sentence for an endpoint that reported none.
	WindowSource  string `json:"windowSource,omitempty"`
	UnknownWindow string `json:"unknownWindow,omitempty"`
}

// check checks sm on pol before it is saved, filling its context window
// from what an endpoint of the person's own reports unless the draft names
// one. A refusal is a result, never an error.
func check(ctx context.Context, home string, pol *policy.Policy, path string, sm *policy.SavedModel, d ModelDraft, logw io.Writer) (*SaveResult, error) {
	view, err := pol.ModelView(*sm)
	if err != nil {
		return nil, err
	}
	if view.APIKeyKind == policy.KeyKindClaudeSignIn && !d.AgreedToMessage {
		return &SaveResult{NeedsAgreement: modelcheck.ClaudeNotice}, nil
	}
	res, err := modelcheck.Check(ctx, modelCheckers(home, pol, path, logw), view)
	var r *modelcheck.Refusal
	if errors.As(err, &r) {
		offered := r.Offered
		if offered == nil {
			offered = []string{}
		}
		return &SaveResult{Refusal: &Refusal{Kind: r.Kind, Reason: r.Reason, Offered: offered, Retry: r.Kind == modelcheck.CannotCheck}}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &SaveResult{Resolved: res.Resolved, Warning: res.Warning}
	switch {
	case d.ContextWindow != 0:
		if err := policy.CheckContextWindow(d.ContextWindow); err != nil {
			return nil, fmt.Errorf("the context window: %w", err)
		}
		sm.ContextWindow = d.ContextWindow
	case res.ContextWindow > 0:
		sm.ContextWindow, out.WindowSource = res.ContextWindow, res.Source
	case view.OwnEndpoint() && sm.ContextWindow == 0:
		out.UnknownWindow = setup.UnknownWindowSentence(sm.Model)
	}
	return out, nil
}

// saved is sm as the page lists it once pol holds it.
func saved(pol *policy.Policy, sm policy.SavedModel) *SavedModel {
	return &SavedModel{ID: sm.ID, Name: sm.Name, Provider: sm.Provider, Model: sm.Model, ContextWindow: sm.ContextWindow,
		Default: pol.DefaultModel().ID == sm.ID, Users: []savedmodels.User{}}
}

// AddModel saves a model on the provider this computer has, checked first.
// The cap, the name and the provider are refused before the provider is
// asked anything, so a refused save spends nothing. The first saved model
// is the default.
func AddModel(ctx context.Context, providerID string, d ModelDraft, logw io.Writer) (*SaveResult, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	if !chosen {
		return nil, errors.New("this computer has no provider yet: add one first")
	}
	return addModel(ctx, pol, path, providerID, d, logw)
}

func addModel(ctx context.Context, pol *policy.Policy, path, providerID string, d ModelDraft, logw io.Writer) (*SaveResult, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	sm := policy.SavedModel{Name: strings.TrimSpace(d.Name), Provider: providerID, Model: strings.TrimSpace(d.Model)}
	if err := pol.CanAddModel(sm); err != nil {
		return nil, err
	}
	res, err := check(ctx, home, pol, path, &sm, d, logw)
	if err != nil || res.Saved != nil || res.Refusal != nil || res.NeedsAgreement != "" {
		return res, err
	}
	kept, err := pol.AddModel(sm)
	if err != nil {
		return nil, err
	}
	if d.MakeDefault {
		if _, err := pol.SetDefault(kept.ID); err != nil {
			return nil, err
		}
	}
	if err := policy.Save(path, pol); err != nil {
		return nil, err
	}
	res.Saved = saved(pol, kept)
	return res, nil
}

// EditModel changes the saved model id: its name, its model id or its
// context window. A new model id is checked first, as a save is.
func EditModel(ctx context.Context, id string, d ModelDraft, logw io.Writer) (*SaveResult, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	old, ok := pol.FindModel(id)
	if !chosen || !ok {
		return nil, fmt.Errorf("no saved model %q on this computer", id)
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	sm := old
	if n := strings.TrimSpace(d.Name); n != "" {
		sm.Name = n
	}
	if m := strings.TrimSpace(d.Model); m != "" {
		sm.Model = m
	}
	res := &SaveResult{}
	if sm.Model != old.Model {
		sm.ContextWindow = 0
		if res, err = check(ctx, home, pol, path, &sm, d, logw); err != nil || res.Refusal != nil || res.NeedsAgreement != "" {
			return res, err
		}
	} else if d.ContextWindow != 0 {
		if err := policy.CheckContextWindow(d.ContextWindow); err != nil {
			return nil, fmt.Errorf("the context window: %w", err)
		}
		sm.ContextWindow = d.ContextWindow
	}
	kept, err := pol.EditModel(old.ID, sm)
	if err != nil {
		return nil, err
	}
	if d.MakeDefault {
		if _, err := pol.SetDefault(kept.ID); err != nil {
			return nil, err
		}
	}
	if err := policy.Save(path, pol); err != nil {
		return nil, err
	}
	res.Saved = saved(pol, kept)
	return res, nil
}

// CheckSavedModel checks the saved model id again, as a save does, and
// changes nothing.
func CheckSavedModel(ctx context.Context, id string, agreed bool, logw io.Writer) (*SaveResult, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	sm, ok := pol.FindModel(id)
	if !chosen || !ok {
		return nil, fmt.Errorf("no saved model %q on this computer", id)
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	checked := sm
	res, err := check(ctx, home, pol, path, &checked, ModelDraft{AgreedToMessage: agreed}, logw)
	if err != nil || res.Refusal != nil || res.NeedsAgreement != "" {
		return res, err
	}
	res.Saved = saved(pol, sm)
	return res, nil
}

// SetDefaultModel makes the saved model id the default, which runs,
// schedules and agents that name no model use.
func SetDefaultModel(id string) error {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return err
	}
	if !chosen {
		return fmt.Errorf("no saved model %q on this computer", id)
	}
	if _, err := pol.SetDefault(id); err != nil {
		return err
	}
	return policy.Save(path, pol)
}

// Removed is how a removal ended: done, with what moved; or refused, with
// what names the models removed, for the page to ask where they go.
type Removed struct {
	Removed []string           `json:"removed"`
	Moved   []savedmodels.User `json:"moved"`
	MovedTo string             `json:"movedTo,omitempty"`
	// InUse are what names the models; the removal was refused and nothing
	// changed.
	InUse []savedmodels.User `json:"inUse,omitempty"`
	// Credential is what became of a removed provider's credential.
	Credential string `json:"credential,omitempty"`
}

// remove is the removal of sms through savedmodels.Remove, as this app's
// client.
func remove(ctx context.Context, pol *policy.Policy, path string, sms []policy.SavedModel, moveTo string, do func(newDefault string) error) (*Removed, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	r, err := savedmodels.Remove(ctx, home, pol, path, sms, moveTo, persistent.ClientDesktop, do)
	var used *savedmodels.InUseError
	if errors.As(err, &used) {
		return &Removed{Removed: []string{}, Moved: []savedmodels.User{}, InUse: used.Users}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &Removed{Removed: []string{}, Moved: r.Moved, MovedTo: r.MovedTo.ID}
	if out.Moved == nil {
		out.Moved = []savedmodels.User{}
	}
	for _, sm := range r.Removed {
		out.Removed = append(out.Removed, sm.ID)
	}
	return out, nil
}

// RemoveSavedModel removes the saved model id. What names it moves to the
// saved model moveTo, which is also the new default when id is the default;
// with no moveTo, the removal of a model something names, or of the
// default, is refused and nothing moves.
func RemoveSavedModel(ctx context.Context, id, moveTo string) (*Removed, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	sm, ok := pol.FindModel(id)
	if !chosen || !ok {
		return nil, fmt.Errorf("no saved model %q on this computer", id)
	}
	return remove(ctx, pol, path, []policy.SavedModel{sm}, moveTo, func(newDefault string) error {
		_, err := pol.RemoveModel(sm.ID, newDefault)
		return err
	})
}

// RemoveProvider removes the provider id and the saved models on it, moving
// what names them to moveTo as RemoveSavedModel does, and then forgets its
// credential on this computer unless keepCredential: its key leaves the
// keychain, its sign-in or connected account is removed.
func RemoveProvider(ctx context.Context, id, moveTo string, keepCredential bool, logw io.Writer) (*Removed, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	providers, models, _ := pol.SavedModels()
	pr, ok := providers[id]
	if !chosen || !ok {
		return nil, fmt.Errorf("no provider %q on this computer", id)
	}
	var on []policy.SavedModel
	for _, sm := range models {
		if sm.Provider == id {
			on = append(on, sm)
		}
	}
	if len(on) == len(models) {
		return nil, errors.New("this provider holds every saved model on this computer: add a model on another provider first, then remove this one")
	}
	out, err := remove(ctx, pol, path, on, moveTo, func(newDefault string) error {
		_, err := pol.RemoveProvider(id, newDefault)
		return err
	})
	if err != nil || len(out.InUse) > 0 {
		return out, err
	}
	out.Credential = "kept on this computer"
	if !keepCredential {
		out.Credential = forgetCredential(ctx, id, pr, logw)
	}
	return out, nil
}

// forgetCredential forgets a removed provider's credential on this
// computer, as `agentry providers remove` does, and says what became of it.
func forgetCredential(ctx context.Context, id string, pr policy.Provider, logw io.Writer) string {
	if sv := savedmodels.SignInVendor(id, pr); sv != "" {
		outcome, err := RemoveModelSignIn(ctx, sv, logw)
		switch {
		case errors.Is(err, modelsignin.ErrNone):
		case err != nil:
			return "its sign-in is still kept: " + err.Error()
		default:
			return "sign-in removed (" + outcome + ")"
		}
	}
	if pr.APIKeySource == modelkey.SourceKeyring {
		if ok, _ := keychainPresent(id); ok {
			if err := keychainDelete(id); err != nil {
				return "its key is still in the keychain: " + err.Error()
			}
			return "key removed from the keychain"
		}
	}
	return "nothing was kept on this computer"
}

// NewProvider is a provider the page adds: a vendor and the way it is
// paid; for an endpoint of the person's own, its name, its address and
// whether it takes a key (kept beforehand with StoreProviderKey); for
// Copilot, the plan.
type NewProvider struct {
	// ID names a provider this computer has, for StoreProviderKey's fix of
	// a missing key; "" for a provider being added.
	ID          string `json:"id"`
	Vendor      string `json:"vendor"`
	Way         string `json:"way"`
	Name        string `json:"name"`
	BaseURL     string `json:"baseUrl"`
	TakesKey    bool   `json:"takesKey"`
	CopilotPlan string `json:"copilotPlan"`
}

// newProvider is the provider np makes, with its id: an endpoint of the
// person's own through savedmodels.CustomProvider, any other vendor through
// the CLI's setup, as `agentry setup` writes it, made on a policy of its own
// so nothing else is touched. The model is the draft's, which setup holds
// to a ChatGPT plan's list.
func newProvider(ctx context.Context, home string, np NewProvider, model string, logw io.Writer) (string, policy.Provider, error) {
	vendor := strings.ToLower(strings.TrimSpace(np.Vendor))
	if vendor == policy.ProviderCustom {
		id, pr, _, err := savedmodels.CustomProvider(np.Name, np.BaseURL, np.TakesKey)
		if err != nil {
			return "", pr, err
		}
		if np.TakesKey {
			if ok, err := keychainPresent(id); err != nil || !ok {
				return "", pr, fmt.Errorf("the keychain holds no key for %s: type the key, or save the endpoint as taking none", np.Name)
			}
		}
		return id, pr, nil
	}
	scratch := policy.Default()
	opts := SetupOpts{Provider: vendor, Model: model}
	var err error
	switch np.Way {
	case WayCredits:
		err = configureCredits(scratch, logw)
	case WayChatGPT:
		opts.APIKeyKind = policy.KeyKindChatGPTPlan
		err = configureSignIn(ctx, home, scratch, vendor, opts, logw)
	case WayClaude:
		opts.APIKeyKind = policy.KeyKindClaudeSignIn
		err = configureSignIn(ctx, home, scratch, vendor, opts, logw)
	case WayCopilot:
		opts.APIKeyKind, opts.CopilotPlan = policy.KeyKindCopilotSignIn, np.CopilotPlan
		err = configureSignIn(ctx, home, scratch, vendor, opts, logw)
	case WayConnect:
		opts.Connect = true
		err = configureSignIn(ctx, home, scratch, vendor, opts, logw)
	case WayKey:
		present, perr := keychainPresent(vendor)
		if perr != nil {
			return "", policy.Provider{}, perr
		}
		if !present {
			return "", policy.Provider{}, fmt.Errorf("the keychain holds no key for %s: type the key first", vendor)
		}
		stored, lerr := keychainLookup(vendor)
		if lerr != nil {
			return "", policy.Provider{}, lerr
		}
		err = configureKey(scratch, vendor, modelkey.SourceKeyring, policy.CredentialKindOf(vendor, stored), opts, logw)
	default:
		return "", policy.Provider{}, fmt.Errorf("unknown way to pay %q", np.Way)
	}
	if err != nil {
		return "", policy.Provider{}, err
	}
	providers, _, _ := scratch.SavedModels()
	id := scratch.Model.KeyProvider()
	pr, ok := providers[id]
	if !ok {
		return "", pr, fmt.Errorf("setup made no provider for %s", vendor)
	}
	return id, pr, nil
}

// ProviderIDFor is the id the provider np would have: the vendor, or for an
// endpoint of the person's own custom-<slug> from its name.
func ProviderIDFor(np NewProvider) (string, error) {
	vendor := strings.ToLower(strings.TrimSpace(np.Vendor))
	if vendor == policy.ProviderCustom {
		return policy.CustomProviderID(np.Name)
	}
	return vendor, nil
}

// AddProviderModel adds the provider np and its first saved model d in one
// act, the model checked first: nothing is written when the check refuses.
// A computer with no model a person chose takes it as its first, which is
// the default. One provider per vendor: a vendor already here is refused.
func AddProviderModel(ctx context.Context, np NewProvider, d ModelDraft, logw io.Writer) (*SaveResult, error) {
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	if np.BaseURL != "" {
		if err := checkBaseURL(strings.ToLower(np.Vendor), np.BaseURL); err != nil {
			return nil, err
		}
	}
	id, err := ProviderIDFor(np)
	if err != nil {
		return nil, err
	}
	if chosen {
		providers, models, _ := pol.SavedModels()
		if hasProvider(providers, id) || slices.ContainsFunc(models, func(m policy.SavedModel) bool { return m.Provider == id }) {
			return nil, fmt.Errorf("this computer already has the provider %s: add a model on it instead", id)
		}
		// The cap and the name before anyone is asked anything.
		if len(models) >= policy.MaxSavedModels {
			return nil, fmt.Errorf("this computer has %d saved models, the most it keeps: remove one first", policy.MaxSavedModels)
		}
		if other, ok := pol.FindModel(d.Name); ok && strings.EqualFold(other.Name, strings.TrimSpace(d.Name)) {
			return nil, fmt.Errorf("there is already a saved model named %q", other.Name)
		}
	}
	id, pr, err := newProvider(ctx, home, np, strings.TrimSpace(d.Model), logw)
	if err != nil {
		return nil, err
	}
	if chosen {
		if err := pol.AddProvider(id, pr); err != nil {
			return nil, err
		}
		return addModel(ctx, pol, path, id, d, logw)
	}
	// The defaults' model is nobody's choice: the first provider and model
	// take its place, and the model is the default.
	newID, err := freshModelID()
	if err != nil {
		return nil, err
	}
	sm := policy.SavedModel{ID: newID, Name: strings.TrimSpace(d.Name), Provider: id, Model: strings.TrimSpace(d.Model)}
	if err := pol.OnlyModel(id, pr, sm); err != nil {
		return nil, err
	}
	res, err := check(ctx, home, pol, path, &sm, d, logw)
	if err != nil || res.Refusal != nil || res.NeedsAgreement != "" {
		return res, err
	}
	if err := pol.OnlyModel(id, pr, sm); err != nil {
		return nil, err
	}
	if err := policy.Save(path, pol); err != nil {
		return nil, err
	}
	res.Saved = saved(pol, sm)
	return res, nil
}

// freshModelID is a new saved model id, the shape the CLI gives one.
func freshModelID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hasProvider(providers map[string]policy.Provider, id string) bool {
	_, ok := providers[id]
	return ok
}

// ListedModel is one model a provider lists, with its price in US dollars
// a million tokens where it gives one.
type ListedModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	InputUSD    string `json:"inputUsd,omitempty"`
	OutputUSD   string `json:"outputUsd,omitempty"`
}

// ProviderModels is what a provider offers to save, for the model field:
// the ids its credential lists, or none where it has no list to ask and the
// id is typed (a connected account's large catalogues, Claude Code's
// aliases, Copilot before its list is asked). Error is why a list could
// not be had; the id can still be typed.
type ProviderModels struct {
	Models []ListedModel `json:"models"`
	// Suggested is a few ids to offer where the provider has no list
	// (Claude Code's aliases).
	Suggested []string `json:"suggested"`
	Error     string   `json:"error,omitempty"`
	Warning   string   `json:"warning,omitempty"`
}

// ListProviderModels asks the provider id for the models it offers, with the
// credential it keeps: a key's or an endpoint's models listing, a ChatGPT
// plan's list, Agentry credits' one model. A Copilot plan's list is asked of
// Copilot CLI only on CopilotModels, since it can take minutes.
func ListProviderModels(ctx context.Context, id string) (*ProviderModels, error) {
	pol, _, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	providers, _, _ := pol.SavedModels()
	pr, ok := providers[id]
	if !chosen || !ok {
		return nil, fmt.Errorf("no provider %q on this computer", id)
	}
	out := &ProviderModels{Models: []ListedModel{}, Suggested: []string{}}
	switch wayOf(id, pr) {
	case WayCredits:
		out.Models = []ListedModel{{ID: setup.AgentryModelName}}
		return out, nil
	case WayChatGPT:
		ids, err := ChatGPTModels(ctx)
		if err != nil {
			out.Error = err.Error()
			return out, nil
		}
		for _, m := range ids {
			out.Models = append(out.Models, ListedModel{ID: m})
		}
		return out, nil
	case WayClaude:
		out.Suggested = []string{"sonnet", "opus", "haiku"}
		return out, nil
	case WayCopilot:
		out.Suggested = []string{modelcheck.CopilotAuto}
		return out, nil
	}
	m := policy.Model{Provider: policy.ProviderVendor(id), ProviderID: id, APIKeyEnv: pr.APIKeyEnv, APIKeySource: pr.APIKeySource, BaseURL: pr.BaseURL}
	if setup.IsConnectProvider(m.Provider) {
		// Their catalogues are large and typed; the id is typed and the
		// check asks the catalogue for it.
		return out, nil
	}
	key, err := modelkey.Personal(m.APIKeyEnv, m.APIKeySource, id)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	provider, base := m.Provider, ""
	if m.OwnEndpoint() {
		provider, base = policy.ProviderCustom, m.BaseURL
	}
	cat, err := listModels(ctx, provider, base, key)
	if err != nil {
		msg := err.Error()
		if key != "" {
			msg = strings.ReplaceAll(msg, key, "<key>")
		}
		out.Error = msg
		return out, nil
	}
	for _, lm := range cat.Models {
		out.Models = append(out.Models, ListedModel{ID: lm.ID, DisplayName: lm.DisplayName, InputUSD: lm.InputUSD, OutputUSD: lm.OutputUSD})
	}
	out.Warning = cat.Warning
	return out, nil
}

// StoreProviderKey keeps key in the OS keychain for the provider np names
// (provider:<id>), on its own, before AddProviderModel saves it; or for a
// provider this computer has whose key is missing, setting its source to
// the keychain. The key is written nowhere else.
func StoreProviderKey(np NewProvider, key string, logw io.Writer) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("no key typed")
	}
	pol, path, chosen, err := readModelsPolicy()
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(np.ID)
	if id != "" {
		if providers, _, _ := pol.SavedModels(); !chosen || !hasProvider(providers, id) {
			return "", fmt.Errorf("no provider %q on this computer", id)
		}
	} else if id, err = ProviderIDFor(np); err != nil {
		return "", err
	}
	if _, err := storeModelKey(id, key, logw); err != nil {
		return "", err
	}
	if !chosen {
		return id, nil
	}
	providers, _, _ := pol.SavedModels()
	pr, ok := providers[id]
	if !ok || pr.APIKeySource == modelkey.SourceKeyring || pr.APIKeyEnv == "" {
		return id, nil
	}
	// A provider here that read its key from its variable alone now reads
	// the keychain too, where the key just went.
	pr.APIKeySource = modelkey.SourceKeyring
	if k := policy.CredentialKindOf(policy.ProviderVendor(id), key); k != policy.KeyKindAPI {
		pr.APIKeyKind = k
	}
	if pol.HasSavedModels() {
		pol.Providers[id] = pr
	} else {
		pol.Model.APIKeySource, pol.Model.APIKeyKind = pr.APIKeySource, pr.APIKeyKind
	}
	return id, policy.Save(path, pol)
}
