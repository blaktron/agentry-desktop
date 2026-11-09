package app

// Settings → Models: the page's side of internal/run/models.go. Every call
// reads and writes the same operator policy the CLI reads, through the CLI's
// own packages, so the cap of ten, the check on save, the default and the
// removal's refusal hold as `agentry models` and `agentry providers` hold
// them. A key crosses into Go only through StoreProviderKey, a call of its
// own; nothing here returns a key or a token.

import (
	"github.com/blaktron/agentry-desktop/internal/run"
)

// SavedModels lists this computer's providers and saved models, with each
// provider's credential state and what names each model.
func (r *RunAPI) SavedModels() (*run.ModelsList, error) {
	return run.ListSavedModels()
}

// ModelChoices are the saved models a Model dropdown offers (run setup, the
// schedule form, a persistent agent's settings): the default it starts on,
// and each model with why a run on it would be refused on this computer now.
func (r *RunAPI) ModelChoices() (*run.ModelChoices, error) {
	return run.ListModelChoices()
}

// ProviderModels asks the provider id which models it offers, for the model
// field; a list it cannot give is said, and the id can be typed.
func (r *RunAPI) ProviderModels(id string) (*run.ProviderModels, error) {
	return run.ListProviderModels(r.ctx(), id)
}

// EndpointModels asks an endpoint of the person's own being added for its
// models, with the key typed for it or none; like ListModels, the key
// reaches that endpoint and nothing else.
func (r *RunAPI) EndpointModels(baseURL, key string) (*run.ProviderModels, error) {
	return run.ListEndpointModels(r.ctx(), baseURL, key)
}

// AddSavedModel saves a model on the provider providerID, checked first. A
// Claude subscription's check sends one message on the person's plan, so it
// is only made once the draft says they agreed. A check that needs Claude
// Code's or Copilot CLI's image takes the app's docker job.
func (r *RunAPI) AddSavedModel(providerID string, d run.ModelDraft) (*run.SaveResult, error) {
	if err := r.acquire(jobSetup, "the check of a saved model"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	res, err := run.AddModel(r.ctx(), providerID, d, logw)
	if err == nil && res.Saved != nil {
		resealSchedules(r.app)
	}
	return res, err
}

// AddProviderModel adds a provider and its first saved model in one act,
// the model checked first; nothing is written when the check refuses.
func (r *RunAPI) AddProviderModel(np run.NewProvider, d run.ModelDraft) (*run.SaveResult, error) {
	if err := r.acquire(jobSetup, "the check of a saved model"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	res, err := run.AddProviderModel(r.ctx(), np, d, logw)
	if err == nil && res.Saved != nil {
		resealSchedules(r.app)
	}
	return res, err
}

// EditSavedModel changes a saved model's name, model id or context window;
// a new model id is checked first.
func (r *RunAPI) EditSavedModel(id string, d run.ModelDraft) (*run.SaveResult, error) {
	if err := r.acquire(jobSetup, "the check of a saved model"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	res, err := run.EditModel(r.ctx(), id, d, logw)
	if err == nil && res.Saved != nil {
		resealSchedules(r.app)
	}
	return res, err
}

// CheckSavedModel checks a saved model again, as a save does, changing
// nothing; agreed is the person's yes to a Claude subscription's message.
func (r *RunAPI) CheckSavedModel(id string, agreed bool) (*run.SaveResult, error) {
	if err := r.acquire(jobSetup, "the check of a saved model"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return run.CheckSavedModel(r.ctx(), id, agreed, logw)
}

// SetDefaultModel makes a saved model the default.
func (r *RunAPI) SetDefaultModel(id string) error {
	if err := run.SetDefaultModel(id); err != nil {
		return err
	}
	resealSchedules(r.app)
	return nil
}

// RemoveSavedModel removes a saved model. What names it moves to moveTo,
// which is also the new default when it is the default; with no moveTo, a
// model something names, or the default, is not removed and the answer
// lists what names it.
func (r *RunAPI) RemoveSavedModel(id, moveTo string) (*run.Removed, error) {
	out, err := run.RemoveSavedModel(r.ctx(), id, moveTo)
	if err == nil && len(out.InUse) == 0 {
		resealSchedules(r.app)
	}
	return out, err
}

// RemoveProvider removes a provider and its saved models as
// RemoveSavedModel does, then forgets its credential on this computer
// unless keepCredential. Forgetting a Claude sign-in runs Claude Code's own
// logout in its image, which takes the docker job.
func (r *RunAPI) RemoveProvider(id, moveTo string, keepCredential bool) (*run.Removed, error) {
	if err := r.acquire(jobSetup, "the removal of a provider"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	out, err := run.RemoveProvider(r.ctx(), id, moveTo, keepCredential, logw)
	if err == nil && len(out.InUse) == 0 {
		resealSchedules(r.app)
	}
	return out, err
}

// StoreProviderKey keeps a key in this computer's keychain for the provider
// np names, in a call of its own so the key travels alone, and returns the
// provider's id. For a provider here whose key is missing it is the fix.
func (r *RunAPI) StoreProviderKey(np run.NewProvider, key string) (string, error) {
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	id, err := run.StoreProviderKey(np, key, logw)
	if err == nil {
		resealSchedules(r.app)
	}
	return id, err
}
