package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry/pkg/apiclient"
)

// The account on Settings (plan subscription-tiers.md §8, agentry-
// desktop#74): who the stored credential is and, where the registry
// mounts the billing extension, the subscription tier and the credits.
// No client takes money: Upgrade opens the site's subscription page in
// the browser, and nothing here holds a billing credential.

// SubscriptionView mirrors the billing extension's GET /x/billing/account
// (agentry-enterprise/schemas/x-billing.yaml). The route is the closed
// build's, so it is read by hand rather than through the generated client.
type SubscriptionView struct {
	Tier      string `json:"tier"`
	Standing  string `json:"standing,omitempty"`
	Unmetered bool   `json:"unmetered,omitempty"`
	Seats     *Seats `json:"seats,omitempty"`
	// MemberOf names the organization whose pool this person spends from
	// (D22). When it is set, everything above is the organization's
	// account and not this person's own.
	MemberOf       string `json:"memberOf,omitempty"`
	Balance        int64  `json:"balance"`
	Allowance      int64  `json:"allowance"`
	SubmissionCost int64  `json:"submissionCost"`
	PeriodEnd      string `json:"periodEnd"`
	PendingTier    string `json:"pendingTier,omitempty"`
	// TierSince, Knowledge and Promo are what the Profile card reads beyond
	// the account line (agentry-desktop#423); a registry from before each
	// omits it.
	TierSince     string `json:"tierSince,omitempty"`
	PrivateAgents struct {
		Max  int64 `json:"max"`
		Used int64 `json:"used"`
		// Enabled false means none may be added and Max says nothing;
		// nil from a registry that predates it (agentry-enterprise#40).
		Enabled *bool `json:"enabled,omitempty"`
	} `json:"privateAgents"`
	Knowledge *KnowledgeAllocation `json:"knowledge,omitempty"`
	Promo     *PromoView           `json:"promo,omitempty"`
	// Upgrade is the registry's own answer about buying and managing:
	// whether either is possible here and, in URL, the page to open — the
	// organization's console for a member, the subscription page for a
	// person (agentry-enterprise#41, D28). The app never derives it.
	Upgrade UpgradeRef `json:"upgrade"`
}

// KnowledgeAllocation is the bytes of knowledge packs kept in Agentry
// against the cap (agentry-enterprise#170): Enabled false means no pack
// may be added or grown and Max says nothing; Max 0 is no cap.
type KnowledgeAllocation struct {
	Enabled bool  `json:"enabled"`
	Bytes   int64 `json:"bytes"`
	Max     int64 `json:"max"`
}

// PromoView is the promo trial on a person's own account
// (agentry-enterprise#280): "running" with its code, the credits it
// granted and when it ends, or "eligible" while a code may be redeemed.
type PromoView struct {
	State       string `json:"state"`
	Code        string `json:"code,omitempty"`
	Credits     int64  `json:"credits,omitempty"`
	CreditsUsed int64  `json:"creditsUsed,omitempty"`
	EndsAt      string `json:"endsAt,omitempty"`
}

// Seats is the organization's seats: held against the allowance, where 0
// means no cap.
type Seats struct {
	Used int64 `json:"used"`
	Max  int64 `json:"max"`
}

// UpgradeRef mirrors the extension's. URL says *where* the subscription is
// managed and the booleans say *whether* anything can be done there, so
// the app shows the console to a member whose organization is fully paid
// for without offering them a purchase.
type UpgradeRef struct {
	Available bool   `json:"available"`
	Manage    bool   `json:"manage"`
	URL       string `json:"url,omitempty"`
}

// AccountView is the signed-in account with its subscription where the
// registry has tiers; Subscription is nil against a core registry.
type AccountView struct {
	User         UserView          `json:"user"`
	Subscription *SubscriptionView `json:"subscription,omitempty"`
	// SubscriptionError says why the subscription could not be read when
	// the registry has one; the account line still renders.
	SubscriptionError string `json:"subscriptionError,omitempty"`
}

// OpenUpgrade opens the page where this account's subscription is
// managed. The destination is the registry's to name — the organization's
// console for a member, the subscription page for a person — because it
// turns on a membership only the registry knows; an app deriving it from
// the registry URL would send every member to a page about an account
// that does not pay for them (agentry-enterprise#41). So this reads the
// account and opens what it answered.
//
// The caller names a registry, never a destination: the app will not open
// a URL the front end chose, and the one the registry named is opened
// only if it is on that registry's own origin, which is where the site
// lives (the edge sends /api/ to the server and everything else to the
// site). A registry cannot use this to send a person anywhere else.
func (s *SettingsAPI) OpenUpgrade(url string) error {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return err
	}
	page, err := upgradeDestination(s.app.ctx, url, settings.TokenFor(url))
	if err != nil {
		return err
	}
	return s.openExternal(page)
}

// upgradeDestination asks the registry where this account's subscription
// is managed and checks the answer before anything opens it.
func upgradeDestination(ctx context.Context, registryURL, token string) (string, error) {
	// A registry may be configured with userinfo in its URL, and these
	// errors are rendered on Settings: name the registry by origin, never
	// by the credential someone typed into it.
	named := originOf(registryURL)
	if token == "" {
		return "", fmt.Errorf("not signed in to %s", named)
	}
	sub, err := fetchSubscription(ctx, registryURL, token)
	if err != nil {
		return "", err
	}
	if sub == nil || sub.Upgrade.URL == "" {
		return "", fmt.Errorf("%s names no page for this subscription", named)
	}
	if !sameOrigin(registryURL, sub.Upgrade.URL) {
		return "", fmt.Errorf("%s named a subscription page on another site; not opening it", named)
	}
	return sub.Upgrade.URL, nil
}

// originOf is a URL with any userinfo dropped, for a message a person
// reads.
func originOf(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return "the registry"
	}
	return u.Scheme + "://" + u.Host
}

// sameOrigin reports whether a registry-named page belongs to that
// registry: same scheme, same host, same port. The site and the server
// share an origin, so anything else is a registry pointing the app off
// its own ground, which the app declines to follow.
func sameOrigin(registryURL, page string) bool {
	a, err1 := neturl.Parse(registryURL)
	b, err2 := neturl.Parse(page)
	return err1 == nil && err2 == nil && a.Scheme == b.Scheme && a.Host == b.Host
}

// fetchSubscription returns nil, nil for a registry without the billing
// extension; an error only when the registry has it and the read failed.
// A registry whose extensions cannot be read is taken as one without.
func fetchSubscription(ctx context.Context, url, token string) (*SubscriptionView, error) {
	return readSubscription(ctx, url, token, false)
}

// fetchSubscriptionStrictly is fetchSubscription for a question that must
// not guess: Submit's warning (agentry-desktop#425), where "charges no
// credits" is a claim the person consents on. A registry whose extensions
// cannot be read is an error, not a registry without billing.
func fetchSubscriptionStrictly(ctx context.Context, url, token string) (*SubscriptionView, error) {
	return readSubscription(ctx, url, token, true)
}

func readSubscription(ctx context.Context, url, token string, strict bool) (*SubscriptionView, error) {
	client, err := registry.New(url, "")
	if err != nil {
		return nil, err
	}
	meta, err := client.GetMetaWithResponse(ctx)
	if strict {
		switch {
		case err != nil:
			return nil, fmt.Errorf("reach %s: %w", url, err)
		case meta.JSON200 == nil:
			return nil, registry.Problem("read the registry's extensions", meta.StatusCode(), nil, meta.Body)
		}
	}
	if err != nil || meta.JSON200 == nil || meta.JSON200.Extensions == nil {
		return nil, nil
	}
	mounted := false
	for _, e := range *meta.JSON200.Extensions {
		if e == "billing" {
			mounted = true
		}
	}
	if !mounted {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/v1/x/billing/account", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := registry.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode != http.StatusOK {
		var p apiclient.Problem
		if json.Unmarshal(body, &p) == nil && p.Title != "" {
			return nil, registry.Problem("read subscription", res.StatusCode, &p, body)
		}
		return nil, registry.Problem("read subscription", res.StatusCode, nil, body)
	}
	var sub SubscriptionView
	if err := json.Unmarshal(body, &sub); err != nil {
		return nil, fmt.Errorf("read subscription: %w", err)
	}
	return &sub, nil
}
