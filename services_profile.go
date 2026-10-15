package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry/pkg/apiclient"
)

// The Profile card on Settings: what the site's /settings/profile shows in
// its Profile and Subscription cards, read with the desktop's own
// credential. Every read here takes user:profile or any signed-in caller;
// buying, the billing portal, cancelling and deleting the account are the
// site's session's alone, so the card opens the browser for those and this
// file writes nothing but the display name.

// ProfileView is the account with what only the Profile card shows: the
// social provider the account signed up with, and the caller's hosted-run
// caps where the registry takes hosted runs.
type ProfileView struct {
	AccountView
	// SignedUpWith is the provider's display name, "" when the account has
	// none or the directory did not answer.
	SignedUpWith string `json:"signedUpWith,omitempty"`
	// HostedRuns is nil where the registry takes no hosted runs, or the
	// status could not be read.
	HostedRuns *HostedRunsTermsView `json:"hostedRuns,omitempty"`
}

// HostedRunsTermsView is what a hosted run of the caller's holds and the
// caps it holds it for, in credits, time and turns. The registry's token cap
// and the rates a settlement is computed at are not carried.
type HostedRunsTermsView struct {
	// HoldCredits is 0 for a member of an unmetered organization.
	HoldCredits int64          `json:"holdCredits"`
	Caps        HostedCapsView `json:"caps"`
}

// MaxDisplayName is the site's form's limit on a display name; the
// Profile card's field carries the same maxLength.
const MaxDisplayName = 100

// Profile reads the Profile card for the registry's stored credential.
func (s *SettingsAPI) Profile(url string) (*ProfileView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return profileFor(s.app, url, token)
}

// SetDisplayName saves the account's display name through the registry
// and answers the user as the registry now has it.
func (s *SettingsAPI) SetDisplayName(url, name string) (*UserView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return setDisplayName(s.app, url, token, name)
}

// profileFor is Profile for a given credential. The account is required;
// the directory's answer and the runs status are lines the card leaves out
// when they cannot be read.
func profileFor(app *App, url, token string) (*ProfileView, error) {
	user, err := whoami(app, url, token)
	if err != nil {
		return nil, err
	}
	out := &ProfileView{AccountView: AccountView{User: *user}}
	if sub, err := fetchSubscription(app.ctx, url, token); err != nil {
		out.SubscriptionError = err.Error()
	} else {
		out.Subscription = sub
	}
	if client, err := registry.New(url, token); err == nil {
		if dir, err := client.GetUserDirectoryWithResponse(app.ctx); err == nil && dir.JSON200 != nil {
			out.SignedUpWith = providerName(dir.JSON200)
		}
	}
	if st, err := hostedrun.New(url, token).Status(app.ctx); err == nil && st.Enabled && st.Caller != nil {
		out.HostedRuns = &HostedRunsTermsView{HoldCredits: st.Caller.HoldCredits, Caps: *hostedCaps(st.Caller.Caps)}
	}
	return out, nil
}

// providerName is the first social provider's display name, its slug when
// it has none, as the site's signedUpWith reads it.
func providerName(d *apiclient.UserDirectory) string {
	p := d.FirstSocialProvider
	if p == nil {
		return ""
	}
	if n := strings.TrimSpace(p.Name); n != "" {
		return n
	}
	return strings.TrimSpace(p.Id)
}

func setDisplayName(app *App, url, token, name string) (*UserView, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > MaxDisplayName {
		return nil, fmt.Errorf("a display name is at most %d characters", MaxDisplayName)
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.UpdateCurrentUserWithResponse(app.ctx, apiclient.UpdateCurrentUserJSONRequestBody{DisplayName: &name})
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("save the display name", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return userViewOf(resp.JSON200), nil
}
