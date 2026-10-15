package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/version"
)

// Report bug: the binding files the text with the stored credential, as the
// desktop, with the app's version and OS and nothing else, and says in one
// word what stopped it otherwise, so the card can offer the right way on.
func TestReportBugFilesAsTheDesktopWithTheStoredCredential(t *testing.T) {
	var got map[string]any
	var auth string
	status, answer := http.StatusCreated, `{"id":7,"createdAt":"2026-10-12T00:00:00Z"}`
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/x/bug-reports/reports", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		got = nil
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv(configdir.EnvVar, t.TempDir())
	keyring.MockInit()
	sapi := &SettingsAPI{app: &App{ctx: context.Background()}}

	// Signed out: nothing is sent.
	res, err := sapi.ReportBug(srv.URL, "The run card overlaps the version.")
	if err != nil || res.Filed || res.Reason != "signin" || got != nil {
		t.Fatalf("signed out: %+v, %v, sent %v", res, err, got)
	}

	storeTestToken(t, srv.URL)
	res, err = sapi.ReportBug(srv.URL+"/", "  The run card overlaps\r\nthe version.  ")
	if err != nil || !res.Filed {
		t.Fatalf("filed: %+v, %v", res, err)
	}
	if auth != "Bearer agy_test" {
		t.Fatalf("Authorization %q", auth)
	}
	wantCtx := map[string]any{"appVersion": version.Version, "os": runtime.GOOS + "/" + runtime.GOARCH}
	if got["body"] != "The run card overlaps\nthe version." || got["source"] != "desktop" || !equalAny(got["context"], wantCtx) || len(got) != 3 {
		t.Fatalf("sent %v", got)
	}

	// Text out of bounds is the card's to say, and is not sent.
	got = nil
	for _, text := range []string{"too short", strings.Repeat("é", 4001)} {
		res, err = sapi.ReportBug(srv.URL, text)
		if err != nil || res.Filed || res.Reason != "text" || got != nil {
			t.Fatalf("%d characters: %+v, %v, sent %v", len([]rune(text)), res, err, got)
		}
	}

	for _, c := range []struct {
		status        int
		answer        string
		reason, words string
	}{
		{404, `{"type":"https://agentry.dev/problems/not-found","title":"Not found"}`, "absent", ""},
		{401, `{"type":"https://agentry.dev/problems/unauthorized","title":"Sign in required"}`, "signin", ""},
		{403, `{"type":"https://agentry.dev/problems/forbidden","title":"Scope missing","detail":"This token needs the user:profile scope to report a bug."}`, "signin", ""},
		{403, `{"type":"https://agentry.dev/problems/account-unactivated","title":"Account not activated","detail":"Activate your account first."}`, "refused", "Activate your account first."},
		{429, `{"type":"https://agentry.dev/problems/too-many-reports","title":"Too many reports","detail":"You have sent 10 bug reports in the last hour. Try again later."}`, "limited", "You have sent 10 bug reports in the last hour. Try again later."},
		{422, `{"type":"https://agentry.dev/problems/validation","title":"Validation failed","detail":"The report has a character that is not text."}`, "refused", "The report has a character that is not text."},
		{502, `bad gateway`, "unreachable", ""},
	} {
		status, answer = c.status, c.answer
		res, err = sapi.ReportBug(srv.URL, "Something broke here.")
		if err != nil || res.Filed || res.Reason != c.reason || res.Message != c.words {
			t.Fatalf("HTTP %d: %+v, %v; want %s %q", c.status, res, err, c.reason, c.words)
		}
	}

	// A registry address held to Settings' rule: no token goes over http to
	// a host that is not this machine, and the refusal names the origin
	// only, never a credential typed into the address.
	if _, err := sapi.ReportBug("http://registry.example", "Something broke here."); err == nil {
		t.Fatal("a cleartext registry address was accepted")
	}
	if _, err := sapi.ReportBug("http://me:hunter2@registry.example", "Something broke here."); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("userinfo address: %v", err)
	}

	// A redirect is not followed: the token and the report stay with the
	// registry that was asked.
	var elsewhere int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere++
		w.WriteHeader(http.StatusCreated)
	}))
	defer other.Close()
	redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})
	srv.Config.Handler = redirect
	res, err = sapi.ReportBug(srv.URL, "Something broke here.")
	if err != nil || res.Filed || elsewhere != 0 {
		t.Fatalf("a redirect was followed: %+v, %v, %d requests elsewhere", res, err, elsewhere)
	}
}

func equalAny(a any, b map[string]any) bool {
	m, ok := a.(map[string]any)
	if !ok || len(m) != len(b) {
		return false
	}
	for k, v := range b {
		if m[k] != v {
			return false
		}
	}
	return true
}
