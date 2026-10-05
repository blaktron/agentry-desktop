// Publisher Earnings' run-setup line and the star ask after a hosted run
// (plan creator-monetization.md D10, D13; agentry-desktop#613), against a
// registry that answers the reads the site's rules make (agentry-web
// #1087): the program's public answer for the publisher, the agent's
// visibility, the reader and their organizations, and the reader's star.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// earningsFake is a registry for acme/reporter. Every field is the
// registry's answer; a zero fake is a public QuickAgent of a publisher in
// the program, read by someone else who has not starred it.
type earningsFake struct {
	mu sync.Mutex

	// noBilling leaves billing out of /meta, as a core registry does.
	noBilling bool
	// notEarning answers the program's read false; earnsStatus, when
	// set, answers it with that status instead (a registry from before
	// the program answers 404).
	notEarning  bool
	earnsStatus int
	private     bool
	// builtin marks the agent one of the packages the CLI embeds.
	builtin bool
	// handle and orgs are the reader's; orgs map a handle to its state.
	handle string
	orgs   map[string]string
	// starred, or noStarred for a stats answer that omits the field.
	starred   bool
	noStarred bool
	// userStatus, when set, answers GET /user with it.
	userStatus int
	// starStatus, when set, answers the star with it; stars records each
	// star the app sent, by path, with its bearer.
	starStatus int
	stars      []string
	// earnsAuth records the Authorization the program's read carried: it
	// is public and is sent none.
	earnsAuth []string
}

func (f *earningsFake) start(t *testing.T) string {
	t.Helper()
	if f.handle == "" {
		f.handle = "pat"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		exts := []string{"billing", "runs"}
		if f.noBilling {
			exts = []string{"runs"}
		}
		hostedJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{}, "extensions": exts})
	})
	mux.HandleFunc("GET /api/v1/x/billing/earnings/publishers/{handle}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.earnsAuth = append(f.earnsAuth, r.Header.Get("Authorization"))
		if f.earnsStatus != 0 {
			w.WriteHeader(f.earnsStatus)
			return
		}
		hostedJSON(w, http.StatusOK, map[string]any{"handle": r.PathValue("handle"), "earns": r.PathValue("handle") == "acme" && !f.notEarning})
	})
	mux.HandleFunc("GET /api/v1/agents/acme/reporter", func(w http.ResponseWriter, _ *http.Request) {
		vis := "public"
		if f.private {
			vis = "private"
		}
		hostedJSON(w, http.StatusOK, map[string]any{
			"id": "00000000-0000-0000-0000-0000000000b1", "namespace": "acme", "name": "reporter", "visibility": vis,
			"builtin": f.builtin, "createdAt": "2026-01-01T00:00:00Z",
		})
	})
	mux.HandleFunc("GET /api/v1/agents/acme/reporter/stats", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		st := map[string]any{"stars": 3, "installsTotal": 9, "installs30d": 2}
		if !f.noStarred && r.Header.Get("Authorization") == "Bearer "+hostedTestToken {
			st["starred"] = f.starred
		}
		hostedJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if f.userStatus != 0 {
			w.WriteHeader(f.userStatus)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		orgs := []map[string]any{}
		for h, s := range f.orgs {
			orgs = append(orgs, map[string]any{"handle": h, "role": "member", "state": s})
		}
		hostedJSON(w, http.StatusOK, map[string]any{
			"id": "00000000-0000-0000-0000-0000000000c1", "handle": f.handle, "displayName": "Pat", "email": "pat@example.com",
			"emailVerified": true, "createdAt": "2026-01-01T00:00:00Z", "orgs": orgs,
		})
	})
	mux.HandleFunc("PUT /api/v1/user/stars/{ns}/{name}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.stars = append(f.stars, r.URL.Path+" "+r.Header.Get("Authorization"))
		if f.starStatus != 0 {
			hostedJSON(w, f.starStatus, map[string]any{"type": "about:blank", "title": "Forbidden", "status": f.starStatus, "detail": "You cannot read this agent."})
			return
		}
		f.starred = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// The line is drawn exactly where the site draws it: a public QuickAgent
// whose publisher the program says earns, read by someone who is neither
// that publisher nor an active member of it. Anything unread draws nothing.
func TestPublisherEarnsHere(t *testing.T) {
	cases := []struct {
		name string
		set  func(*earningsFake)
		want bool
	}{
		{"someone else's public QuickAgent, its publisher in the program", func(*earningsFake) {}, true},
		{"a pending membership holds nothing yet", func(f *earningsFake) { f.orgs = map[string]string{"acme": "pending"} }, true},
		{"a core registry", func(f *earningsFake) { f.noBilling = true }, false},
		{"the publisher has not joined", func(f *earningsFake) { f.notEarning = true }, false},
		{"a registry from before the program", func(f *earningsFake) { f.earnsStatus = http.StatusNotFound }, false},
		{"a private QuickAgent", func(f *earningsFake) { f.private = true }, false},
		{"the reader is its publisher", func(f *earningsFake) { f.handle = "acme" }, false},
		{"the reader is an active member of it", func(f *earningsFake) { f.orgs = map[string]string{"acme": "active"} }, false},
		{"the reader could not be read", func(f *earningsFake) { f.userStatus = http.StatusServiceUnavailable }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &earningsFake{}
			c.set(f)
			reg := f.start(t)
			exts, err := extensionsOf(context.Background(), reg)
			if err != nil {
				t.Fatal(err)
			}
			if got := publisherEarnsHere(context.Background(), reg, hostedTestToken, "acme/reporter@1.0.0", exts); got != c.want {
				t.Fatalf("publisherEarnsHere = %v, want %v", got, c.want)
			}
			for _, a := range f.earnsAuth {
				if a != "" {
					t.Fatalf("the program's public read carried a token: %q", a)
				}
			}
		})
	}
}

// The run setup's availability carries the answer, so the panel draws the
// line under what a run In Agentry costs.
func TestHostedAvailabilityCarriesTheEarningsLine(t *testing.T) {
	for _, earns := range []bool{true, false} {
		t.Run(map[bool]string{true: "earning", false: "not earning"}[earns], func(t *testing.T) {
			f := &hostedFake{enabled: true, earns: earns}
			reg := f.start(t)
			signIn(t, reg)
			r, _ := newRunAPI(t)
			v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if v.PublisherEarns != earns {
				t.Fatalf("publisherEarns = %v with the publisher earning %v", v.PublisherEarns, earns)
			}
		})
	}
}

// The ask after a run that ended well: someone else's QuickAgent the
// reader has not starred. Their own, an organization's they are an active
// member of, one already starred, and a star that cannot be read ask
// nothing.
func TestStarAsk(t *testing.T) {
	cases := []struct {
		name string
		set  func(*earningsFake)
		want bool
	}{
		{"someone else's, not starred", func(*earningsFake) {}, true},
		{"whatever the program says", func(f *earningsFake) { f.noBilling, f.notEarning = true, true }, true},
		{"already starred", func(f *earningsFake) { f.starred = true }, false},
		{"a built-in (a build, a draft, a Voice Writer run)", func(f *earningsFake) { f.builtin = true }, false},
		{"the star not said", func(f *earningsFake) { f.noStarred = true }, false},
		{"their own", func(f *earningsFake) { f.handle = "acme" }, false},
		{"their organization's", func(f *earningsFake) { f.orgs = map[string]string{"acme": "active"} }, false},
		{"the reader could not be read", func(f *earningsFake) { f.userStatus = http.StatusServiceUnavailable }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &earningsFake{}
			c.set(f)
			reg := f.start(t)
			signIn(t, reg)
			r, _ := newRunAPI(t)
			v, err := r.StarAsk(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if v.Ask != c.want {
				t.Fatalf("ask = %v, want %v (%+v)", v.Ask, c.want, v)
			}
			if v.Ask && v.Name != "acme/reporter" {
				t.Fatalf("the ask names %q, want acme/reporter", v.Name)
			}
		})
	}
}

// Nobody signed in, or a ref that is not one: no ask, and nothing asked.
func TestStarAskSignedOut(t *testing.T) {
	f := &earningsFake{}
	reg := f.start(t)
	signIn(t, "http://127.0.0.1:1") // a token for another registry only
	r, _ := newRunAPI(t)
	if v, err := r.StarAsk(reg, "acme/reporter@1.0.0"); err != nil || v.Ask {
		t.Fatalf("signed out: %+v, %v", v, err)
	}
	signIn(t, reg)
	if v, err := r.StarAsk(reg, "reporter"); err != nil || v.Ask {
		t.Fatalf("a ref that is not one: %+v, %v", v, err)
	}
	if _, err := r.StarAsk("ftp://"+strings.TrimPrefix(reg, "http://"), "acme/reporter"); err == nil {
		t.Fatal("a registry address Settings refuses was asked")
	}
}

// Star sends the reader's star for the QuickAgent the ask named, and only
// a name: a version range, a path or an empty part is refused unsent. A
// refusal says the registry's words.
func TestStarQuickAgent(t *testing.T) {
	f := &earningsFake{}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	if err := r.StarQuickAgent(reg, "acme/reporter"); err != nil {
		t.Fatal(err)
	}
	if len(f.stars) != 1 || f.stars[0] != "/api/v1/user/stars/acme/reporter Bearer "+hostedTestToken {
		t.Fatalf("stars sent = %q", f.stars)
	}
	if v, _ := r.StarAsk(reg, "acme/reporter@1.0.0"); v.Ask {
		t.Fatal("still asked once starred")
	}
	for _, bad := range []string{"acme/reporter@1.0.0", "acme", "acme/", "/reporter", "acme/reporter/x", "../x/y"} {
		if err := r.StarQuickAgent(reg, bad); err == nil {
			t.Fatalf("starred %q", bad)
		}
	}
	if len(f.stars) != 1 {
		t.Fatalf("a refused name was sent: %q", f.stars)
	}
	// A registry Settings refuses (plain http off this machine) is never
	// sent the token, whatever name is asked.
	if err := r.StarQuickAgent("http://agentry.example", "acme/reporter"); err == nil {
		t.Fatal("starred through plain http off this machine")
	}
	if len(f.stars) != 1 {
		t.Fatalf("a refused registry was sent a star: %q", f.stars)
	}
	f.starStatus = http.StatusForbidden
	err := r.StarQuickAgent(reg, "acme/reporter")
	if err == nil || !strings.Contains(err.Error(), "You cannot read this agent.") {
		t.Fatalf("refused star: %v", err)
	}
}
