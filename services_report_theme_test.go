package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// reportThemeRegistry answers GET and PUT /user/report-settings as the core
// does since agentry#989 (DEV-227): {} for never chosen, a built-in name
// kept, anything else 422. found=false is a registry from before the route.
func reportThemeRegistry(t *testing.T, found bool) (*httptest.Server, *string, *int) {
	t.Helper()
	theme, puts := "", 0
	mux := http.NewServeMux()
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return false
		}
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		return true
	}
	answer := func(w http.ResponseWriter) {
		if theme == "" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"theme":"` + theme + `"}`))
	}
	mux.HandleFunc("GET /api/v1/user/report-settings", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			answer(w)
		}
	})
	mux.HandleFunc("PUT /api/v1/user/report-settings", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		puts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		name, has := body["theme"].(string)
		switch {
		case !has && len(body) == 0:
			theme = ""
		case has && strings.Contains(" plain formal brief technical letter ", " "+name+" "):
			theme = name
		default:
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"validation","status":422,"detail":"theme is not a report theme"}`))
			return
		}
		answer(w)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &theme, &puts
}

// The Profile's Report theme reads the account's choice, "" for never
// chosen, which is not Plain (DEV-227).
func TestReportThemeReadsTheAccountsChoice(t *testing.T) {
	srv, theme, _ := reportThemeRegistry(t, true)
	ctx := context.Background()
	v, err := reportThemeOf(ctx, srv.URL, "agy_test")
	if err != nil || !v.Available || v.Theme != "" {
		t.Fatalf("never chosen: %+v, %v", v, err)
	}
	*theme = "letter"
	if v, err = reportThemeOf(ctx, srv.URL, "agy_test"); err != nil || v.Theme != "letter" {
		t.Fatalf("chosen: %+v, %v", v, err)
	}
	if _, err := reportThemeOf(ctx, srv.URL, "agy_wrong"); err == nil {
		t.Fatal("a refused credential must be an error")
	}
}

// A registry from before the setting answers 404: the card is not offered,
// and that is not an error.
func TestReportThemeOnARegistryWithoutTheSetting(t *testing.T) {
	srv, _, _ := reportThemeRegistry(t, false)
	v, err := reportThemeOf(context.Background(), srv.URL, "agy_test")
	if err != nil || v.Available {
		t.Fatalf("got %+v, %v; want not available, no error", v, err)
	}
}

// Saving sends the theme; "" clears it with {}; a name that is not a
// built-in theme's is refused before anything is sent.
func TestSetReportThemeSavesClearsAndRefuses(t *testing.T) {
	srv, theme, puts := reportThemeRegistry(t, true)
	ctx := context.Background()
	v, err := setReportTheme(ctx, srv.URL, "agy_test", "formal")
	if err != nil || v.Theme != "formal" || *theme != "formal" {
		t.Fatalf("save: %+v, %v (stored %q)", v, err, *theme)
	}
	if v, err = setReportTheme(ctx, srv.URL, "agy_test", ""); err != nil || v.Theme != "" || *theme != "" {
		t.Fatalf("clear: %+v, %v (stored %q)", v, err, *theme)
	}
	before := *puts
	if _, err := setReportTheme(ctx, srv.URL, "agy_test", "Fancy"); err == nil || !strings.Contains(err.Error(), "not a report theme") {
		t.Fatalf("unknown name: %v", err)
	}
	if *puts != before {
		t.Fatal("an unknown name must send nothing")
	}
}

// A local run takes the account's theme when the account chose one;
// otherwise Plain, chosen by the default. An account it cannot read, or a
// name this build does not have, is a line in the run's log and Plain:
// how a PDF looks is no reason to stop a run (D21, as the CLI does).
func TestALocalRunTakesTheAccountsTheme(t *testing.T) {
	srv, theme, _ := reportThemeRegistry(t, true)
	ctx := context.Background()
	var log strings.Builder

	got := accountReportTheme(ctx, srv.URL, "agy_test", &log)
	if got != (engine.ReportTheme{}) {
		t.Fatalf("never chosen: %+v, want the zero value (Plain by default)", got)
	}
	*theme = "technical"
	got = accountReportTheme(ctx, srv.URL, "agy_test", &log)
	if got.Name != "technical" || got.From != engine.ThemeFromAccount {
		t.Fatalf("chosen: %+v", got)
	}
	if log.Len() != 0 {
		t.Fatalf("nothing to warn about yet: %q", log.String())
	}
	got = accountReportTheme(ctx, srv.URL, "agy_wrong", &log)
	if got != (engine.ReportTheme{}) || !strings.Contains(log.String(), "report theme could not be read") {
		t.Fatalf("unreadable: %+v, log %q", got, log.String())
	}
	*theme = "future"
	log.Reset()
	got = accountReportTheme(ctx, srv.URL, "agy_test", &log)
	if got != (engine.ReportTheme{}) || !strings.Contains(log.String(), `"future"`) {
		t.Fatalf("unknown: %+v, log %q", got, log.String())
	}
	// Signed out: no account to ask, nothing to say.
	log.Reset()
	if got = accountReportTheme(ctx, srv.URL, "", &log); got != (engine.ReportTheme{}) || log.Len() != 0 {
		t.Fatalf("signed out: %+v, log %q", got, log.String())
	}
}
