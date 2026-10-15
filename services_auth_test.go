package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A browser sign-in that the user abandons must end at once, say so, and
// leave the app ready for the next attempt.
func TestBrowserSignInCancel(t *testing.T) {
	opened := make(chan string, 2)
	s := &SettingsAPI{app: &App{ctx: context.Background()}, open: func(u string) error { opened <- u; return nil }}
	if s.CancelBrowserSignIn() {
		t.Fatal("nothing in flight, yet cancel reported one")
	}

	start := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := s.SignInViaBrowser("https://registry.example.test")
			done <- err
		}()
		return done
	}
	waitOpen := func() string {
		select {
		case u := <-opened:
			return u
		case <-time.After(5 * time.Second):
			t.Fatal("the browser was never opened")
			return ""
		}
	}
	waitDone := func(done <-chan error) error {
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("cancel did not end the wait")
			return nil
		}
	}

	done := start()
	if u := waitOpen(); !strings.HasPrefix(u, "https://registry.example.test/ui/authorize?port=") {
		t.Fatalf("authorize URL: %s", u)
	}
	// A second attempt while one is in flight is refused, and says how out.
	if _, err := s.SignInViaBrowser("https://registry.example.test"); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("concurrent sign-in: %v", err)
	}
	if !s.CancelBrowserSignIn() {
		t.Fatal("cancel found nothing in flight")
	}
	if err := waitDone(done); !errors.Is(err, errSignInCancelled) {
		t.Fatalf("after cancel: %v", err)
	}
	if s.CancelBrowserSignIn() {
		t.Fatal("cancel handle survived the flow")
	}

	// The lock is free again: a fresh sign-in starts, and cancels, cleanly.
	done = start()
	waitOpen()
	if !s.CancelBrowserSignIn() {
		t.Fatal("second flow: cancel found nothing in flight")
	}
	if err := waitDone(done); !errors.Is(err, errSignInCancelled) {
		t.Fatalf("second flow after cancel: %v", err)
	}
}

// The scopes a browser sign-in asks for: everything the app's surfaces used
// before, plus run:read and run:write for a run In Agentry. The registry
// mints from its own fixed set, so this is what the app asks for — and what
// a token lacks is what HostedAvailability reports back as the reason In
// Agentry is unavailable.
func TestAuthorizeURLAsksForTheRunScopes(t *testing.T) {
	opened := make(chan string, 1)
	s := &SettingsAPI{app: &App{ctx: context.Background()}, open: func(u string) error { opened <- u; return nil }}
	go func() { _, _ = s.SignInViaBrowser("https://registry.example.test") }()
	var u string
	select {
	case u = <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the browser was never opened")
	}
	defer s.CancelBrowserSignIn()

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/ui/authorize" || parsed.Query().Get("port") == "" || parsed.Query().Get("state") == "" {
		t.Fatalf("authorize URL = %s", u)
	}
	scopes := strings.Fields(parsed.Query().Get("scopes"))
	for _, want := range []string{"run:read", "run:write", "agent:read", "agent:publish", "repo:read", "repo:write", "user:profile"} {
		found := false
		for _, got := range scopes {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("the authorize URL does not ask for %s: %v", want, scopes)
		}
	}
}
