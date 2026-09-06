package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A browser sign-in that the user abandons must end at once, say so, and
// leave the app ready for the next attempt (agentry-desktop#24).
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
