package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// connectSeams substitutes the consent and records what Connect emits and
// what it hands the browser.
type connectSeams struct {
	mu     sync.Mutex
	events []string
	opened []string
}

func (c *connectSeams) record(name string, data ...any) {
	b, _ := json.Marshal(data)
	c.mu.Lock()
	c.events = append(c.events, name+" "+string(b))
	c.mu.Unlock()
}

func withConnect(t *testing.T, consent func(ctx context.Context, ui run.ConsentUI) (*run.ServerConnection, error)) (*RunAPI, *connectSeams) {
	t.Helper()
	seams := &connectSeams{}
	emit, conn := emitEvent, runConnect
	emitEvent = func(_ *App, name string, data ...any) { seams.record(name, data...) }
	runConnect = func(ctx context.Context, _, _, _ string, ui run.ConsentUI) (*run.ServerConnection, error) {
		return consent(ctx, ui)
	}
	t.Cleanup(func() { emitEvent, runConnect = emit, conn })
	r := &RunAPI{app: &App{}, openBrowser: func(u string) error {
		seams.mu.Lock()
		seams.opened = append(seams.opened, u)
		seams.mu.Unlock()
		return nil
	}}
	return r, seams
}

const consentAt = "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=s"

func TestConnectOpensTheConsentAddressAndSendsItToThePage(t *testing.T) {
	r, seams := withConnect(t, func(_ context.Context, ui run.ConsentUI) (*run.ServerConnection, error) {
		ui.Present(consentAt)
		return &run.ServerConnection{Server: "https://gmailmcp.googleapis.com/mcp/v1", State: run.ConnConnected}, nil
	})
	v, err := r.Connect("https://reg.example", "acme/inbox@1.0.0", "https://gmailmcp.googleapis.com/mcp/v1")
	if err != nil || v.Connection == nil || v.Connection.State != run.ConnConnected {
		t.Fatalf("connect: %+v %v", v, err)
	}
	if len(seams.opened) != 1 || seams.opened[0] != consentAt {
		t.Fatalf("the browser was given %v", seams.opened)
	}
	if len(seams.events) != 1 || !strings.HasPrefix(seams.events[0], connectURLEvent+" ") || !strings.Contains(seams.events[0], "accounts.google.com") {
		t.Fatalf("events: %v", seams.events)
	}
}

func TestConnectNeverHandsTheOSAnAddressThatIsNotHTTPS(t *testing.T) {
	for _, bad := range []string{"http://accounts.example/auth", "file:///etc/passwd", "javascript:alert(1)", "https:///nohost"} {
		r, seams := withConnect(t, func(_ context.Context, ui run.ConsentUI) (*run.ServerConnection, error) {
			ui.Present(bad)
			return nil, errors.New("the sign-in timed out")
		})
		_, _ = r.Connect("https://reg.example", "acme/inbox@1.0.0", "https://mcp.example/mcp")
		if len(seams.opened) != 0 {
			t.Fatalf("%s was opened", bad)
		}
		for _, e := range seams.events {
			if strings.HasPrefix(e, connectURLEvent) {
				t.Fatalf("%s was sent to the page: %s", bad, e)
			}
		}
	}
}

func TestConnectAnswersWithTheOwnClientStepsWhereTheVendorNeedsThem(t *testing.T) {
	r, _ := withConnect(t, func(context.Context, run.ConsentUI) (*run.ServerConnection, error) {
		return nil, &run.NeedsOwnClient{Issuer: run.GoogleIssuer, Guide: run.OwnClientGuide{Issuer: run.GoogleIssuer, Known: true, Steps: []string{"x"}}}
	})
	v, err := r.Connect("https://reg.example", "acme/inbox@1.0.0", "https://gmailmcp.googleapis.com/mcp/v1")
	if err != nil || v.NeedsOwnClient == nil || !v.NeedsOwnClient.Known || v.Connection != nil {
		t.Fatalf("want the steps, got %+v %v", v, err)
	}
}

func TestOneSignInAtATimeAndPasteAndCancelReachTheOneUnderWay(t *testing.T) {
	got := make(chan string, 1)
	started := make(chan struct{})
	r, _ := withConnect(t, func(ctx context.Context, ui run.ConsentUI) (*run.ServerConnection, error) {
		close(started)
		select {
		case a := <-ui.Pasted:
			got <- a
		case <-time.After(5 * time.Second):
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err := r.PasteRedirect("http://127.0.0.1:1/callback?code=c&state=s"); err == nil {
		t.Fatal("a paste with no sign-in under way must be refused")
	}
	if r.CancelConnect() {
		t.Fatal("nothing was under way to cancel")
	}
	done := make(chan *ConnectView, 1)
	go func() {
		v, err := r.Connect("https://reg.example", "acme/inbox@1.0.0", "https://mcp.example/mcp")
		if err != nil {
			t.Errorf("a cancelled sign-in is not an error: %v", err)
		}
		done <- v
	}()
	<-started
	if _, err := r.Connect("https://reg.example", "acme/inbox@1.0.0", "https://mcp.example/mcp"); err == nil || !strings.Contains(err.Error(), "already under way") {
		t.Fatalf("a second sign-in must wait for the first: %v", err)
	}
	if err := r.PasteRedirect("  http://127.0.0.1:1/callback?code=c&state=s  "); err != nil {
		t.Fatal(err)
	}
	if a := <-got; a != "http://127.0.0.1:1/callback?code=c&state=s" {
		t.Fatalf("pasted %q", a)
	}
	if !r.CancelConnect() {
		t.Fatal("the sign-in under way was not cancelled")
	}
	if v := <-done; v == nil || !v.Cancelled {
		t.Fatalf("want a cancelled view, got %+v", v)
	}
}

func TestKeepOwnClientRefusesAnEmptyID(t *testing.T) {
	r := &RunAPI{app: &App{}}
	if err := r.KeepOwnClient(run.GoogleIssuer, "  ", "secret"); err == nil {
		t.Fatal("an empty client ID must be refused")
	}
}
