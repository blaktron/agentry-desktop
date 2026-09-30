package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// profileRegistry is a registry with the billing and runs extensions and a
// directory, answering the Profile card's reads (agentry-desktop#423).
func profileRegistry(t *testing.T, directory, runs bool) (*httptest.Server, *string) {
	t.Helper()
	displayName := "Chris"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing","runs"]}`))
	})
	user := func() string {
		return `{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"` + displayName + `","emailVerified":false,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z"}`
	}
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		return true
	}
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			_, _ = w.Write([]byte(user()))
		}
	})
	mux.HandleFunc("PATCH /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		var body struct {
			DisplayName *string `json:"displayName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DisplayName == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		displayName = *body.DisplayName
		_, _ = w.Write([]byte(user()))
	})
	mux.HandleFunc("GET /api/v1/user/directory", func(w http.ResponseWriter, r *http.Request) {
		if !directory {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if authed(w, r) {
			_, _ = w.Write([]byte(`{"firstSocialProvider":{"id":"google","name":"Google"}}`))
		}
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			_, _ = w.Write([]byte(`{"kind":"user","tier":"promo_pro","balance":90,"allowance":0,"submissionCost":1,"periodStart":"2026-09-29T00:00:00Z","periodEnd":"2026-10-29T00:00:00Z","tierSince":"2026-09-29T00:00:00Z","pendingTier":"standard","privateAgents":{"max":0,"used":2,"enabled":true},"knowledge":{"enabled":true,"bytes":1048576,"max":5368709120},"promo":{"state":"running","code":"LAUNCH","credits":100,"creditsUsed":10,"endsAt":"2026-10-29T00:00:00Z"},"upgrade":{"available":true,"manage":false,"url":"http://example.test/settings/subscription"}}`))
		}
	})
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, r *http.Request) {
		if !runs {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if authed(w, r) {
			_, _ = w.Write([]byte(`{"enabled":true,"workersOnline":0,"hostsOnline":1,"queued":0,"caller":{"tier":"promo_pro","caps":{"timeoutSeconds":900,"maxTurns":60,"maxTokens":400000},"holdCredits":20,"inputTokensPerCredit":20000,"outputTokensPerCredit":4000}}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &displayName
}

// The Profile card reads what the site's Profile and Subscription cards
// show, with the desktop's own credential (agentry-desktop#423): the user
// with whether the email is verified, how the account signed up, the
// subscription with its promo trial, knowledge and private-agent answers,
// and the caller's hosted-run caps.
func TestProfileReadsTheAccountTheSignUpAndTheRunCaps(t *testing.T) {
	srv, _ := profileRegistry(t, true, true)
	app := &App{ctx: context.Background()}

	p, err := profileFor(app, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if p.User.Handle != "chris" || p.User.EmailVerified || p.User.DisplayName != "Chris" {
		t.Fatalf("user = %+v", p.User)
	}
	if p.SignedUpWith != "Google" {
		t.Fatalf("signed up with %q, want Google", p.SignedUpWith)
	}
	s := p.Subscription
	if s == nil || s.Tier != "promo_pro" || s.TierSince != "2026-09-29T00:00:00Z" || s.PendingTier != "standard" {
		t.Fatalf("subscription = %+v", s)
	}
	if s.PrivateAgents.Enabled == nil || !*s.PrivateAgents.Enabled || s.PrivateAgents.Used != 2 {
		t.Fatalf("private agents = %+v", s.PrivateAgents)
	}
	if s.Knowledge == nil || !s.Knowledge.Enabled || s.Knowledge.Bytes != 1048576 || s.Knowledge.Max != 5368709120 {
		t.Fatalf("knowledge = %+v", s.Knowledge)
	}
	if s.Promo == nil || s.Promo.State != "running" || s.Promo.Code != "LAUNCH" || s.Promo.Credits != 100 || s.Promo.EndsAt != "2026-10-29T00:00:00Z" {
		t.Fatalf("promo = %+v", s.Promo)
	}
	if p.HostedRuns == nil || p.HostedRuns.HoldCredits != 20 || p.HostedRuns.Caps.MaxTurns != 60 || p.HostedRuns.OutputTokensPerCredit != 4000 {
		t.Fatalf("hosted runs = %+v", p.HostedRuns)
	}
}

// A directory that does not answer and a registry without hosted runs
// leave their lines out; the card still renders the account.
func TestProfileWithoutADirectoryOrHostedRuns(t *testing.T) {
	srv, _ := profileRegistry(t, false, false)
	p, err := profileFor(&App{ctx: context.Background()}, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if p.SignedUpWith != "" || p.HostedRuns != nil {
		t.Fatalf("signedUpWith=%q hostedRuns=%+v; want neither", p.SignedUpWith, p.HostedRuns)
	}
	if p.User.Handle != "chris" || p.Subscription == nil {
		t.Fatalf("the account must still render: %+v", p)
	}
	if _, err := profileFor(&App{ctx: context.Background()}, srv.URL, "agy_wrong"); err == nil {
		t.Fatal("a refused credential must be an error")
	}
}

// The display name saves through the registry with the desktop's token
// (user:profile), trimmed, and a name the site's form would refuse is
// refused here before anything is sent.
func TestSetDisplayNameSavesThroughTheRegistry(t *testing.T) {
	srv, saved := profileRegistry(t, true, true)
	app := &App{ctx: context.Background()}

	u, err := setDisplayName(app, srv.URL, "agy_test", "  Chris Q  ")
	if err != nil {
		t.Fatal(err)
	}
	if *saved != "Chris Q" || u.DisplayName != "Chris Q" {
		t.Fatalf("saved %q, answered %q; want Chris Q", *saved, u.DisplayName)
	}
	if _, err := setDisplayName(app, srv.URL, "agy_test", strings.Repeat("x", 101)); err == nil || !strings.Contains(err.Error(), "100") {
		t.Fatalf("a 101-character name must be refused, got %v", err)
	}
	if *saved != "Chris Q" {
		t.Fatalf("a refused name reached the registry: %q", *saved)
	}
	if _, err := setDisplayName(app, srv.URL, "agy_wrong", "Other"); err == nil {
		t.Fatal("a refused credential must be an error")
	}
}

// A page-supplied registry URL is held to Settings' rule before its token
// is looked up (D-030): the keychain keys a token by host, so an http://
// spelling of an added https registry must be refused, not answered with
// that registry's token in clear. The refusal comes before any lookup.
func TestProfileBindingsRefuseAPlainHTTPRegistry(t *testing.T) {
	s := &SettingsAPI{app: &App{ctx: context.Background()}}
	const plain = "http://registry.example.test"
	// "not signed in" would mean the token was looked up first; the URL
	// rule's refusal is what must answer.
	byTheRule := func(name string, err error) {
		t.Helper()
		if err == nil || strings.Contains(err.Error(), "not signed in") {
			t.Fatalf("%s: want the URL rule's refusal before any token lookup, got %v", name, err)
		}
	}
	_, err := s.Profile(plain)
	byTheRule("Profile", err)
	_, err = s.SetDisplayName(plain, "x")
	byTheRule("SetDisplayName", err)
	_, err = s.Whoami(plain)
	byTheRule("Whoami", err)
}
