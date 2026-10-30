package app

import (
	"context"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/inference"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// CreditsOfferView is what model setup's Agentry credits tile shows: whether
// this computer is signed in to the SecureAgentry registry this build is for,
// the account's credit balance there, and the model the registry forwards
// credit-paid runs to. A figure the registry did not answer is not known,
// never zero.
type CreditsOfferView struct {
	SignedIn bool   `json:"signedIn"`
	Registry string `json:"registry"`
	// Balance is the paying account's credits; BalanceKnown says it was read.
	Balance      int64 `json:"balance"`
	BalanceKnown bool  `json:"balanceKnown"`
	// Model is the model in use for credit-paid runs; "" when not known.
	Model string `json:"model"`
	// Unavailable is the registry's reason when it serves no credit-paid
	// model now (no model chosen, or the endpoint is off).
	Unavailable string `json:"unavailable"`
}

// creditsModelTTL is how long the model read for the tile is kept: the
// registry's staff change it rarely, and reading it opens a session.
const creditsModelTTL = 10 * time.Minute

// creditsModel keeps the last model read for the tile, per registry.
var creditsModel struct {
	sync.Mutex
	registry string
	model    string
	at       time.Time
}

// CreditsOffer is the Agentry credits tile's facts. The model is read by
// opening a credits session and ending it at once, which charges nothing:
// the registry names the model only to a session.
func (r *RunAPI) CreditsOffer() (*CreditsOfferView, error) {
	ctx, cancel := context.WithTimeout(r.app.ctx, 20*time.Second)
	defer cancel()
	reg := settings.SecureAgentryRegistry()
	v := &CreditsOfferView{Registry: reg}
	if reg == "" {
		return v, nil
	}
	token := settings.TokenFor(reg)
	if token == "" {
		return v, nil
	}
	v.SignedIn = true
	if sub, err := fetchSubscription(ctx, reg, token); err == nil && sub != nil {
		v.Balance, v.BalanceKnown = sub.Balance, true
	}
	creditsModel.Lock()
	if creditsModel.registry == reg && time.Since(creditsModel.at) < creditsModelTTL {
		v.Model = creditsModel.model
	}
	creditsModel.Unlock()
	if v.Model != "" {
		return v, nil
	}
	c := inference.New(reg, func() (string, error) { return settings.TokenFor(reg), nil }, registry.HTTPClient)
	o, err := c.Open(ctx, inference.PurposeRun)
	if err != nil {
		v.Unavailable = err.Error()
		return v, nil
	}
	_, _ = c.End(context.WithoutCancel(ctx), o.Session.ID)
	v.Model = o.Model
	creditsModel.Lock()
	creditsModel.registry, creditsModel.model, creditsModel.at = reg, o.Model, time.Now()
	creditsModel.Unlock()
	return v, nil
}
