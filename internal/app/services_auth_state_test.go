package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// A stored token stands for a sign-in until the registry refuses it: its
// 401 to that token flips the settings view to signed out, with the
// rejection said, and a sign-in the registry accepts flips it back. A 401
// to any other bearer, a token being checked at sign-in among them, says
// nothing about the stored one.
func TestARejectedTokenReadsAsSignedOut(t *testing.T) {
	freshConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			http.Error(w, `{"title":"not found"}`, http.StatusNotFound)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer agy_fresh":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "00000000-0000-0000-0000-000000000002", "handle": "ada", "email": "a@example.test",
				"displayName": "Ada", "emailVerified": true, "totpEnabled": false, "createdAt": "2026-01-02T03:04:05Z"})
		default:
			w.Header().Set("Content-Type", "application/problem+json")
			http.Error(w, `{"title":"unauthorized","detail":"the token was revoked"}`, http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	storeTestToken(t, srv.URL)
	tokenAccepted(srv.URL)
	watchAuth()
	t.Cleanup(func() { registry.Unauthorized = nil; tokenAccepted(srv.URL) })

	signedIn := func() (bool, bool) {
		v, err := settingsView()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range v.Registries {
			if r.URL == settings.Normalize(srv.URL) {
				return r.HasToken, r.TokenRejected
			}
		}
		t.Fatalf("the registry is not in the view: %+v", v.Registries)
		return false, false
	}
	if has, rejected := signedIn(); !has || rejected {
		t.Fatalf("before any answer: hasToken %v, rejected %v", has, rejected)
	}

	// Another bearer refused says nothing about the stored token.
	if _, err := whoami(&App{ctx: context.Background()}, srv.URL, "agy_other"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("whoami with another token: %v", err)
	}
	if has, rejected := signedIn(); !has || rejected {
		t.Fatalf("after another token's 401: hasToken %v, rejected %v", has, rejected)
	}

	// The stored token refused: signed out, and said so.
	s := &SettingsAPI{app: &App{ctx: context.Background()}}
	if _, err := s.Profile(srv.URL); err == nil {
		t.Fatal("Profile read with a token the registry refuses")
	}
	if has, rejected := signedIn(); has || !rejected {
		t.Fatalf("after the stored token's 401: hasToken %v, rejected %v", has, rejected)
	}
	if settings.TokenFor(srv.URL) == "" {
		t.Fatal("the refused token was removed from the store; the person signs out or in again themselves")
	}

	// A sign-in the registry accepts clears the mark.
	if _, err := s.SignIn(srv.URL, "agy_fresh"); err != nil {
		t.Fatal(err)
	}
	if has, rejected := signedIn(); !has || rejected {
		t.Fatalf("after signing in again: hasToken %v, rejected %v", has, rejected)
	}
	// And so does signing out.
	markTokenRejected(srv.URL)
	if _, err := s.SignOut(srv.URL); err != nil {
		t.Fatal(err)
	}
	if has, rejected := signedIn(); has || rejected {
		t.Fatalf("after signing out: hasToken %v, rejected %v", has, rejected)
	}
}
