package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry/pkg/reportdesign"
)

// reportDesignRegistry answers GET and PUT /user/report-settings as the
// core does since agentry#1026 (DEV-230): {} for never chosen, {"design": …}
// kept in its canonical form when the core's pkg/reportdesign parses it, a
// body with any other field refused with 400. found=false is a registry
// from before the route. stored is the design kept, "" for none; a test may
// set it to a document the core of a later day would hold.
func reportDesignRegistry(t *testing.T, found bool) (*httptest.Server, *string, *int) {
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

// builtin is a built-in design, failing the test when there is none.
func builtin(t *testing.T, id string) reportdesign.Design {
	t.Helper()
	d, ok := reportdesign.Builtin(id)
	if !ok {
		t.Fatalf("no built-in %q", id)
	}
	return d
}

// throughThePage is a design as the page hands it back: the JSON the
// binding sent it, decoded as Wails decodes a binding's argument.
func throughThePage(t *testing.T, d reportdesign.Design) reportdesign.Design {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var back reportdesign.Design
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// The editor's choices are the core's: the five themes in its order, each
// its own document exactly, with the pinned CLI's thumbnail of it; the
// families, and which role each may set; the closed lists; the limits.
// Invariant 8: each theme, loaded into the page and handed back unchanged,
// is byte for byte the document the core ships, valid, and unedited.
func TestReportDesignOptionsAreTheCores(t *testing.T) {
	o := reportDesignOptions()
	var ids []string
	for _, th := range o.Themes {
		ids = append(ids, th.ID)
		doc, _ := reportdesign.BuiltinDocument(th.ID)
		back := throughThePage(t, th.Design)
		if !bytes.Equal(back.Canonical(), doc) {
			t.Fatalf("%s through the page:\n%s\nwant\n%s", th.ID, back.Canonical(), doc)
		}
		if c := checkReportDesign(back); c.Problem != nil || !c.Unedited || c.Label != th.Design.Name {
			t.Fatalf("%s checks as %+v", th.ID, c)
		}
		raw, ok := strings.CutPrefix(th.Thumbnail, "data:image/png;base64,")
		if !ok {
			t.Fatalf("%s thumbnail is not a PNG data URL: %.40q", th.ID, th.Thumbnail)
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.DecodeConfig(bytes.NewReader(b)); err != nil {
			t.Fatalf("%s thumbnail: %v", th.ID, err)
		}
	}
	if got := strings.Join(ids, ","); got != strings.Join(reportdesign.BuiltinIDs(), ",") || o.DefaultThemeID != "plain" {
		t.Fatalf("themes %s, default %s", got, o.DefaultThemeID)
	}
	if len(o.Families) != len(reportdesign.Families()) || len(o.CodeFonts) != 3 || strings.Join(o.Papers, ",") != "a4,letter" {
		t.Fatalf("options %+v", o)
	}
	for _, f := range o.BodyFonts {
		if f == "Playfair Display" || f == "Montserrat" || f == "JetBrains Mono" {
			t.Fatalf("%s offered for body text", f)
		}
	}
	if o.MaxName != 60 || o.MaxFooterText != 120 {
		t.Fatalf("limits %d, %d", o.MaxName, o.MaxFooterText)
	}
}

// The page's checks are the core's: an edited theme is said as one; an
// accent the core refuses is refused with its words and the nearest shade
// that passes, and the shade it offers is saved by the core; whether the
// accent colours text decides the ratio it needs (D17).
func TestCheckReportDesignIsTheCoresVerdict(t *testing.T) {
	d := builtin(t, "classic")
	d.Name = "Ours"
	if c := checkReportDesign(d); c.Problem != nil || c.Unedited || c.Label != "Ours (from Classic)" {
		t.Fatalf("renamed: %+v", c)
	}
	d.Name = "Classic"
	d.Paper = "letter"
	if c := checkReportDesign(d); c.Label != "Classic, edited" {
		t.Fatalf("edited: %+v", c)
	}

	// Classic sets small-capital headings in the accent: text, 4.5:1.
	d.Accent = "#7fa7d9"
	c := checkReportDesign(d)
	if c.Problem == nil || c.Problem.Field != "accent" || c.Accent.OK || !c.Accent.OnText || c.Accent.Need != 4.5 {
		t.Fatalf("pale accent on text: %+v", c)
	}
	if want := d.Validate().Error(); !strings.HasSuffix(want, c.Problem.Message) {
		t.Fatalf("the page's words %q are not the core's %q", c.Problem.Message, want)
	}
	if !strings.Contains(c.Problem.Message, c.Accent.Nearest) {
		t.Fatalf("the offered shade %s is not the one the core names: %s", c.Accent.Nearest, c.Problem.Message)
	}
	d.Accent = c.Accent.Nearest
	if c := checkReportDesign(d); c.Problem != nil || !c.Accent.OK {
		t.Fatalf("the offered shade must pass: %+v", c)
	}

	// The same pale accent on rules only (graphics) needs 3:1.
	d.Accent, d.Headings, d.TitleBlock = "#7fa7d9", "rule", "ruled"
	if c := checkReportDesign(d); c.Accent.OnText || c.Accent.Need != 3 {
		t.Fatalf("accent on graphics: %+v", c)
	}

	d = builtin(t, "plain")
	d.Accent = "blue"
	if c := checkReportDesign(d); c.Accent.Valid || c.Problem == nil || c.Problem.Field != "accent" || c.Tint != "" {
		t.Fatalf("not hex: %+v", c)
	}
	d = builtin(t, "modern")
	if c := checkReportDesign(d); c.Tint == "" || c.Tint == d.Accent {
		t.Fatalf("tint %q", c.Tint)
	}
	d.Footer.Text = "line one\nline two"
	if c := checkReportDesign(d); c.Problem == nil || c.Problem.Field != "footer.text" {
		t.Fatalf("two-line footer: %+v", c)
	}
}

// Reading: never chosen is no design, which is not Plain (DEV-227); a
// saved design reads back whole; one this build cannot parse is said, not
// shown; a registry without the setting is not offered, and that is not an
// error; a refused credential is.
func TestReportDesignReadsTheAccountsDesign(t *testing.T) {
	srv, stored, _ := reportDesignRegistry(t, true)
	ctx := context.Background()
	v, err := reportDesignOf(ctx, srv.URL, "agy_test")
	if err != nil || !v.Available || v.Design != nil || v.Unreadable != "" {
		t.Fatalf("never chosen: %+v, %v", v, err)
	}
	ours := builtin(t, "editorial")
	ours.Name, ours.Accent, ours.Paper = "Ours", "#1f6feb", "letter"
	*stored = string(ours.Canonical())
	v, err = reportDesignOf(ctx, srv.URL, "agy_test")
	if err != nil || v.Design == nil || v.Design.SHA256() != ours.SHA256() || v.Label != "Ours (from Editorial)" {
		t.Fatalf("saved: %+v, %v", v, err)
	}
	*stored = strings.Replace(*stored, `"agentryTheme": 2,`, `"agentryTheme": 2, "logo": {"png": "x"},`, 1)
	v, err = reportDesignOf(ctx, srv.URL, "agy_test")
	if err != nil || v.Design != nil || !strings.Contains(v.Unreadable, "logo") {
		t.Fatalf("a later core's design: %+v, %v", v, err)
	}
	if _, err := reportDesignOf(ctx, srv.URL, "agy_wrong"); err == nil {
		t.Fatal("a refused credential must be an error")
	}
	old, _, _ := reportDesignRegistry(t, false)
	if v, err := reportDesignOf(ctx, old.URL, "agy_test"); err != nil || v.Available {
		t.Fatalf("no setting: %+v, %v", v, err)
	}
}

// Saving sends the design whole, in the core's canonical form, and the
// core keeps exactly that; a design the core would refuse is refused here
// in its words and nothing is sent; Reset clears it with {}.
func TestSaveReportDesignSavesRefusesAndResets(t *testing.T) {
	srv, stored, puts := reportDesignRegistry(t, true)
	ctx := context.Background()
	for _, id := range reportdesign.BuiltinIDs() {
		d := throughThePage(t, builtin(t, id))
		v, err := saveReportDesign(ctx, srv.URL, "agy_test", &d)
		doc, _ := reportdesign.BuiltinDocument(id)
		if err != nil || *stored != string(doc) || v.Design == nil || v.Label != d.Name {
			t.Fatalf("save %s: %+v, %v\nstored %s", id, v, err, *stored)
		}
	}
	d := builtin(t, "technical")
	d.Name, d.Headings, d.Footer = "House", "rule", reportdesign.Footer{PageNumbers: "n", Text: "Acme & Co <internal>"}
	if v, err := saveReportDesign(ctx, srv.URL, "agy_test", &d); err != nil || *stored != string(d.Canonical()) || v.Label != "House (from Technical)" {
		t.Fatalf("save edited: %+v, %v\nstored %s", v, err, *stored)
	}

	before := *puts
	bad := builtin(t, "modern")
	bad.Accent = "#9ec5ff"
	if _, err := saveReportDesign(ctx, srv.URL, "agy_test", &bad); err == nil || !strings.Contains(err.Error(), "accent") {
		t.Fatalf("pale accent: %v", err)
	}
	bad = builtin(t, "plain")
	bad.Fonts.Body = "JetBrains Mono"
	if _, err := saveReportDesign(ctx, srv.URL, "agy_test", &bad); err == nil || !strings.Contains(err.Error(), "fonts.body") {
		t.Fatalf("mono body: %v", err)
	}
	if *puts != before {
		t.Fatal("a refused design must send nothing")
	}

	if v, err := saveReportDesign(ctx, srv.URL, "agy_test", nil); err != nil || v.Design != nil || !v.Available || *stored != "" {
		t.Fatalf("reset: %+v, %v (stored %q)", v, err, *stored)
	}
}

// A local run takes the account's design whole, edited or not, from the
// account, and the pinned CLI stamps it as the core hashes it, which is the
// digest its receipt names (invariant 9). Never chosen, signed out, an
// account it cannot read, and a design this build cannot draw are Plain,
// the last two said in the run's log: how a PDF looks is no reason to stop
// a run (D20, as the CLI does).
func TestALocalRunTakesTheAccountsDesign(t *testing.T) {
	srv, stored, _ := reportDesignRegistry(t, true)
	ctx := context.Background()
	var log strings.Builder

	if got := accountReportTheme(ctx, srv.URL, "agy_test", &log); got.Design != nil || got.Name != "" || got.From != "" {
		t.Fatalf("never chosen: %+v, want the zero value (Plain by default)", got)
	}
	ours := builtin(t, "modern")
	ours.Name, ours.Accent, ours.Fonts.Body = "House", "#0f766e", "Lora"
	for _, d := range []reportdesign.Design{builtin(t, "classic"), ours} {
		*stored = string(d.Canonical())
		got := accountReportTheme(ctx, srv.URL, "agy_test", &log)
		if !bytes.Equal(got.Design, d.Canonical()) || got.From != engine.ThemeFromAccount {
			t.Fatalf("%s: %+v", d.Name, got)
		}
		stamp, err := toolsrv.StampTheme(got.Design)
		if err != nil || stamp.SHA256 != d.SHA256() || stamp.Name != d.Name || stamp.BasedOn != d.BasedOn {
			t.Fatalf("%s: the pinned CLI stamps %+v, %v", d.Name, stamp, err)
		}
	}
	if log.Len() != 0 {
		t.Fatalf("nothing to warn about yet: %q", log.String())
	}

	if got := accountReportTheme(ctx, srv.URL, "agy_wrong", &log); got.Design != nil || !strings.Contains(log.String(), "report design could not be read") {
		t.Fatalf("unreadable: %+v, log %q", got, log.String())
	}
	log.Reset()
	*stored = strings.Replace(string(ours.Canonical()), `"agentryTheme": 2,`, `"agentryTheme": 3,`, 1)
	if got := accountReportTheme(ctx, srv.URL, "agy_test", &log); got.Design != nil || !strings.Contains(log.String(), "not one this app can draw") {
		t.Fatalf("a later document: %+v, log %q", got, log.String())
	}
	log.Reset()
	if got := accountReportTheme(ctx, srv.URL, "", &log); got.Design != nil || log.Len() != 0 {
		t.Fatalf("signed out: %+v, log %q", got, log.String())
	}
}

// reportDesignFixture is the frontend tests' copy of the core's report
// designs (frontend/tests/reportDesign.test.ts): each theme's canonical
// document as text, the families, and the closed lists. It is held here to
// the core this app pins, so the frontend's round trip (invariant 8) is
// tested against the documents the core ships. AGENTRY_UPDATE_FIXTURES=1
// rewrites it after a core bump.
const reportDesignFixture = "frontend/tests/fixtures/reportDesigns.json"

func TestTheFrontendsReportDesignFixtureIsTheCores(t *testing.T) {
	o := reportDesignOptions()
	themes := map[string]string{}
	for _, id := range reportdesign.BuiltinIDs() {
		doc, _ := reportdesign.BuiltinDocument(id)
		themes[id] = string(doc)
	}
	want, err := json.MarshalIndent(map[string]any{
		"ids":           reportdesign.BuiltinIDs(),
		"themes":        themes,
		"families":      o.Families,
		"headingFonts":  o.HeadingFonts,
		"bodyFonts":     o.BodyFonts,
		"codeFonts":     o.CodeFonts,
		"papers":        o.Papers,
		"titleBlocks":   o.TitleBlocks,
		"headingStyles": o.HeadingStyles,
		"quoteStyles":   o.QuoteStyles,
		"pageNumbers":   o.PageNumbers,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("AGENTRY_UPDATE_FIXTURES") == "1" {
		if err := os.MkdirAll(filepath.Dir(reportDesignFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reportDesignFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(reportDesignFixture)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s is not the pinned core's report designs (%v): regenerate with AGENTRY_UPDATE_FIXTURES=1 go test -run %s", reportDesignFixture, err, t.Name())
	}
}
