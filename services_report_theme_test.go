package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry/pkg/reportdesign"
)

// reportThemeRegistry answers GET and PUT /user/report-settings as the core
// does since agentry#1026 (DEV-230): {} for never chosen, {"design": …}
// kept when the core's pkg/reportdesign parses it, a body with any other
// field refused with 400 (the old {"theme": …} included). found=false is a
// registry from before the route. stored is the design kept, "" for none.
func reportThemeRegistry(t *testing.T, found bool) (*httptest.Server, *string, *int) {
	t.Helper()
	stored, puts := "", 0
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
		if stored == "" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"design":` + stored + `}`))
	}
	refuse := func(w http.ResponseWriter, status int, detail string) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"validation","status":` + strconv.Itoa(status) + `,"detail":"` + detail + `"}`))
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
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			refuse(w, http.StatusBadRequest, "not JSON")
			return
		}
		design, has := body["design"]
		switch {
		case len(body) == 0:
			stored = ""
		case has && len(body) == 1:
			d, err := reportdesign.Parse(design)
			if err != nil {
				refuse(w, http.StatusUnprocessableEntity, "design is not a report design")
				return
			}
			stored = string(d.Canonical())
		default:
			refuse(w, http.StatusBadRequest, "unknown field")
			return
		}
		answer(w)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &stored, &puts
}

// builtinDoc is a built-in design's document as the core stores it.
func builtinDoc(t *testing.T, id string) string {
	t.Helper()
	d, ok := reportdesign.Builtin(id)
	if !ok {
		t.Fatalf("no built-in %q", id)
	}
	return string(d.Canonical())
}

// The Profile's Report theme reads the built-in the account's design is
// based on, "" for never chosen, which is not Plain (DEV-227); a design
// changed on the site reads as Edited; the choices are the core's five
// built-ins, the two the pinned agentry command can draw marked.
func TestReportThemeReadsTheAccountsChoice(t *testing.T) {
	srv, stored, _ := reportThemeRegistry(t, true)
	ctx := context.Background()
	v, err := reportThemeOf(ctx, srv.URL, "agy_test")
	if err != nil || !v.Available || v.Theme != "" || v.Edited {
		t.Fatalf("never chosen: %+v, %v", v, err)
	}
	var ids, here []string
	for _, c := range v.Choices {
		ids = append(ids, c.ID)
		if c.Here {
			here = append(here, c.ID)
		}
	}
	if got := strings.Join(ids, ","); got != "plain,classic,modern,editorial,technical" {
		t.Fatalf("choices %s", got)
	}
	if got := strings.Join(here, ","); got != "plain,technical" {
		t.Fatalf("drawn here %s", got)
	}
	*stored = builtinDoc(t, "classic")
	if v, err = reportThemeOf(ctx, srv.URL, "agy_test"); err != nil || v.Theme != "classic" || v.Edited {
		t.Fatalf("chosen: %+v, %v", v, err)
	}
	edited, _ := reportdesign.Builtin("classic")
	edited.Name = "Ours"
	*stored = string(edited.Canonical())
	if v, err = reportThemeOf(ctx, srv.URL, "agy_test"); err != nil || v.Theme != "classic" || !v.Edited {
		t.Fatalf("edited on the site: %+v, %v (stored %s)", v, err, *stored)
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

// Saving sends the built-in's own document, which the core accepts and
// keeps; "" clears it with {}; a name that is not a built-in's (the old
// formal, brief and letter among them) is refused before anything is sent.
func TestSetReportThemeSavesClearsAndRefuses(t *testing.T) {
	srv, stored, puts := reportThemeRegistry(t, true)
	ctx := context.Background()
	v, err := setReportTheme(ctx, srv.URL, "agy_test", "editorial")
	if err != nil || v.Theme != "editorial" || v.Edited || *stored != builtinDoc(t, "editorial") {
		t.Fatalf("save: %+v, %v (stored %s)", v, err, *stored)
	}
	if v, err = setReportTheme(ctx, srv.URL, "agy_test", ""); err != nil || v.Theme != "" || *stored != "" {
		t.Fatalf("clear: %+v, %v (stored %q)", v, err, *stored)
	}
	before := *puts
	for _, name := range []string{"Fancy", "formal", "letter"} {
		if _, err := setReportTheme(ctx, srv.URL, "agy_test", name); err == nil || !strings.Contains(err.Error(), "not a report theme") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if *puts != before {
		t.Fatal("an unknown name must send nothing")
	}
}

// A local run takes the account's theme when the account chose one the
// pinned agentry command can draw; otherwise Plain, chosen by the default. An account it cannot read, or a
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
	*theme = builtinDoc(t, "technical")
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
	// A built-in the pinned agentry command cannot draw yet is Plain, said
	// in the log (D20), until the app carries agentry-cli#784.
	*theme = builtinDoc(t, "modern")
	log.Reset()
	got = accountReportTheme(ctx, srv.URL, "agy_test", &log)
	if got != (engine.ReportTheme{}) || !strings.Contains(log.String(), `"modern" is not one this app can draw yet`) {
		t.Fatalf("not drawable here: %+v, log %q", got, log.String())
	}
	// Signed out: no account to ask, nothing to say.
	log.Reset()
	if got = accountReportTheme(ctx, srv.URL, "", &log); got != (engine.ReportTheme{}) || log.Len() != 0 {
		t.Fatalf("signed out: %+v, log %q", got, log.String())
	}
}
