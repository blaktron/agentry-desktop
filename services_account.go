package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Tier           string `json:"tier"`
	Balance        int64  `json:"balance"`
	Allowance      int64  `json:"allowance"`
	SubmissionCost int64  `json:"submissionCost"`
	PeriodEnd      string `json:"periodEnd"`
	PendingTier    string `json:"pendingTier,omitempty"`
	PrivateAgents  struct {
		Max  int64 `json:"max"`
		Used int64 `json:"used"`
	} `json:"privateAgents"`
	// UpgradeURL is the site's subscription page for this registry, or ""
	// when the account is already Pro.
	UpgradeURL string `json:"upgradeUrl,omitempty"`
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

// Account reports the stored credential's account and subscription.
func (s *SettingsAPI) Account(url string) (*AccountView, error) {
	url = settings.Normalize(url)
	token := settings.TokenFor(url)
	if token == "" {
		return nil, fmt.Errorf("not signed in to %s", url)
	}
	user, err := whoami(s.app, url, token)
	if err != nil {
		return nil, err
	}
	out := &AccountView{User: *user}
	sub, err := fetchSubscription(s.app.ctx, url, token)
	if err != nil {
		out.SubscriptionError = err.Error()
		return out, nil
	}
	out.Subscription = sub
	return out, nil
}

// OpenUpgrade opens the site's subscription page for a registry in the
// browser. The product's site fronts the registry's own origin (the edge
// sends /api/ to the server and everything else to the site), which is
// why the page is the registry URL plus a path; a core registry has no
// billing extension and never shows the button.
func (s *SettingsAPI) OpenUpgrade(url string) error {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return err
	}
	return s.open(upgradeURL(url))
}

func upgradeURL(registryURL string) string {
	return strings.TrimRight(registryURL, "/") + "/settings/subscription?upgrade=pro"
}

// fetchSubscription returns nil, nil for a registry without the billing
// extension; an error only when the registry has it and the read failed.
func fetchSubscription(ctx context.Context, url, token string) (*SubscriptionView, error) {
	client, err := registry.New(url, "")
	if err != nil {
		return nil, err
	}
	meta, err := client.GetMetaWithResponse(ctx)
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
	if sub.Tier != "pro" {
		sub.UpgradeURL = upgradeURL(url)
	}
	return &sub, nil
}
