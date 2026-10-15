package main

// A run In Agentry's secrets, from the desktop. The account's secrets are
// kept by the registry's vault; this app holds a token carrying
// secret:write, which may set and delete one and never read one back. So the
// app renders no stored value, stores none on this machine, and learns which
// declared names are missing from run create's 409 secret-missing.
//
// The value travels in a call of its own, as the model key does, and a
// dispatch error on it is logged without its arguments by internal/applog,
// as for every secret-carrying call; the value is in no log line, no event
// and no error.

import (
	"fmt"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// vaultClient is a hosted-run client for reg, held to Settings' URL rule
// before any token is looked up.
func vaultClient(reg string) (*hostedrun.Client, string, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, reg, err
	}
	token := settings.TokenFor(reg)
	if token == "" {
		return nil, reg, fmt.Errorf("sign in to %s to keep secrets there", originOf(reg))
	}
	return hostedrun.New(reg, token), reg, nil
}

// SetHostedSecret sets the account's secret name in reg's vault. keep says
// whether the account keeps it for later runs; false holds it for the one
// start this panel is making, and the vault removes it when that run ends.
// The error names the secret, never the value.
func (r *RunAPI) SetHostedSecret(reg, name, value string, keep bool) error {
	c, reg, err := vaultClient(reg)
	if err != nil {
		return err
	}
	if err := c.SetSecret(r.ctx(), name, value, keep); err != nil {
		if ref := hostedrun.RefusalOf(err); ref != nil {
			return fmt.Errorf("%s was not saved: %s", name, ref.Sentence())
		}
		return fmt.Errorf("%s was not saved: the registry could not be reached", name)
	}
	r.heldMu.Lock()
	defer r.heldMu.Unlock()
	if r.held == nil {
		r.held = map[string]map[string]bool{}
	}
	if r.held[reg] == nil {
		r.held[reg] = map[string]bool{}
	}
	if keep {
		delete(r.held[reg], name)
	} else {
		r.held[reg][name] = true
	}
	return nil
}

// DeleteHostedSecret deletes the account's secret name from reg's vault
// (the Settings card). A name the account does not hold is not an error.
func (r *RunAPI) DeleteHostedSecret(reg, name string) error {
	c, reg, err := vaultClient(reg)
	if err != nil {
		return err
	}
	if err := c.DeleteSecret(r.ctx(), name); err != nil {
		if ref := hostedrun.RefusalOf(err); ref != nil {
			if ref.Slug == "not-found" {
				r.heldMu.Lock()
				delete(r.held[reg], name)
				r.heldMu.Unlock()
				return nil
			}
			return fmt.Errorf("%s was not deleted: %s", name, ref.Sentence())
		}
		return fmt.Errorf("%s was not deleted: the registry could not be reached", name)
	}
	r.heldMu.Lock()
	delete(r.held[reg], name)
	r.heldMu.Unlock()
	return nil
}

// DiscardHostedSecrets deletes the secrets this app set for a start In
// Agentry at reg and did not keep, when that start will not happen: the run
// was refused, or the ask was cancelled or closed. A secret the person kept
// from the site since is out of this token's reach (409 secret-kept). Best
// effort: one the registry does not delete now its sweep deletes a day after
// it was set.
func (r *RunAPI) DiscardHostedSecrets(reg string) {
	reg = settings.Normalize(reg)
	r.heldMu.Lock()
	names := r.held[reg]
	delete(r.held, reg)
	r.heldMu.Unlock()
	if len(names) == 0 {
		return
	}
	c, _, err := vaultClient(reg)
	if err != nil {
		return
	}
	for n := range names {
		_ = c.DeleteSecret(r.ctx(), n)
	}
}

// forgetHeld drops the record of the secrets held for a start that
// started: the run holds them now, and the vault removes them at its end.
func (r *RunAPI) forgetHeld(reg string) {
	r.heldMu.Lock()
	delete(r.held, reg)
	r.heldMu.Unlock()
}

// SecretsPageURL is where the account's secrets are listed: the site's
// Secrets page at the registry's origin, the list being a signed-in
// session's alone. The panel opens it through App.OpenExternal.
func (r *RunAPI) SecretsPageURL(reg string) (string, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return "", err
	}
	return originOf(reg) + "/settings/secrets", nil
}

// KeepsSecrets reports whether reg keeps secrets for runs In Agentry: its
// /meta lists the vault. Settings draws its Secrets card only then.
func (r *RunAPI) KeepsSecrets(reg string) bool {
	reg = settings.Normalize(reg)
	if settings.ValidateURL(reg) != nil || settings.TokenFor(reg) == "" {
		return false
	}
	exts, err := extensionsOf(r.ctx(), reg)
	return err == nil && hasString(exts, "vault")
}
