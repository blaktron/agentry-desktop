package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The account on Settings (agentry-desktop#74): the credential's user;
// the subscription only where the registry mounts the billing extension;
// Upgrade opens the site's subscription page for the registry.
func TestAccountShowsTheSubscriptionWhereThereIsOne(t *testing.T) {
	billing := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if billing {
			_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{}}`))
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z"}`))
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tier":"standard","balance":40,"allowance":50,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":10,"used":4},"upgrade":{"available":false}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sub, err := fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err != nil || sub != nil {
		t.Fatalf("a core registry: sub=%+v err=%v; want none", sub, err)
	}
	billing = true
	sub, err = fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Tier != "standard" || sub.Balance != 40 || sub.Allowance != 50 || sub.PrivateAgents.Max != 10 || sub.UpgradeURL != srv.URL+"/settings/subscription?upgrade=pro" {
		t.Fatalf("subscription = %+v", sub)
	}
	if _, err := fetchSubscription(context.Background(), srv.URL, "agy_wrong"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a refused read must be an error, got %v", err)
	}

	opened := make(chan string, 1)
	s := &SettingsAPI{app: &App{ctx: context.Background()}, open: func(u string) error { opened <- u; return nil }}
	if err := s.OpenUpgrade(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}
	if got := <-opened; got != srv.URL+"/settings/subscription?upgrade=pro" {
		t.Fatalf("opened %q", got)
	}
}
