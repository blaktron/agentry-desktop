package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/blaktron/agentry-cli/pkg/reportthumbs"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/reportdesign"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The report design (plan report-designs.md §4, D2, D8, D10, D14, D17, D18,
// D22; agentry-desktop#759, D-075): how the PDFs a run exports with
// export_pdf look, the person's setting, never the model's. The account
// keeps it whole, a theme document version 2, at GET/PUT
// /user/report-settings (agentry#1024, DEV-230) on user:profile, which the
// app's sign-in carries. Settings → Report design edits it, and a local run
// or chat takes it. An account that never chose reads back with no design,
// which is not Plain at the API (DEV-227); this app, which has no config of
// its own for it, draws Plain then, and when signed out.
//
// The core's pkg/reportdesign is the one definition (D22): the page's
// controls are its fields, its closed lists and families are the page's
// choices, and every check the page shows (the contrast guard, the field a
// save would be refused for) is that package's answer through
// CheckReportDesign, so the page cannot disagree with the core's save. The
// thumbnails are the pinned CLI's (pkg/reportthumbs), rendered by the
// runner's own template.
//
// The logo (plan §4 "The logo", D9; agentry#1025, agentry-desktop#777) is
// the person's own image, drawn by the pinned CLI in the title block. The
// core keeps it beside the design and answers it only when asked
// (include=logo), which this app always does: the page shows it, and a
// local run carries it whole to the CLI. It is uploaded and removed on its
// own; a Save sends the design without it, and the core keeps the stored
// one. The page never holds its bytes in the design it edits: the view
// gives it as a PNG data: URL beside the design, so the editor's checks do
// not carry half a megabyte back and forth on every change.

// ReportDesignView is the account's report design as the page shows it.
// Available is false on a registry from before the setting, where the page
// says so; Design is nil when the account never chose one. Unreadable says
// the account holds a design this build cannot read (a later core's), which
// Save replaces; it is the reason, never the document.
type ReportDesignView struct {
	Available  bool                 `json:"available"`
	Design     *reportdesign.Design `json:"design,omitempty"`
	Label      string               `json:"label,omitempty"`
	Unreadable string               `json:"unreadable,omitempty"`
	// Logo is the design's logo as a PNG data: URL, "" for none. The
	// Design beside it never carries the logo.
	Logo string `json:"logo,omitempty"`
}

// ReportDesignOptions are the editor's choices, all of them the core's:
// the five themes with their thumbnails, the families and which role each
// may set, the closed lists, and the two texts' limits.
type ReportDesignOptions struct {
	Themes         []ReportDesignTheme  `json:"themes"`
	Families       []ReportDesignFamily `json:"families"`
	HeadingFonts   []string             `json:"headingFonts"`
	BodyFonts      []string             `json:"bodyFonts"`
	CodeFonts      []string             `json:"codeFonts"`
	Papers         []string             `json:"papers"`
	TitleBlocks    []string             `json:"titleBlocks"`
	HeadingStyles  []string             `json:"headingStyles"`
	QuoteStyles    []string             `json:"quoteStyles"`
	PageNumbers    []string             `json:"pageNumbers"`
	MaxName        int                  `json:"maxName"`
	MaxFooterText  int                  `json:"maxFooterText"`
	DefaultThemeID string               `json:"defaultThemeId"`
}

// ReportDesignTheme is one built-in theme: its document, exactly as the
// core ships it, and its thumbnail as a data: URL (the page's CSP admits
// data: images), "" when the pinned CLI has none for it.
type ReportDesignTheme struct {
	ID        string              `json:"id"`
	Design    reportdesign.Design `json:"design"`
	Thumbnail string              `json:"thumbnail"`
}

// ReportDesignFamily is a family the Typst image carries.
type ReportDesignFamily struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	HeadingsOnly bool   `json:"headingsOnly,omitempty"`
}

// ReportDesignCheck is the core's verdict on a design the editor holds.
// Problem is the first rule it breaks, in document order, as the core's
// save would refuse it; nil when it can be saved. Accent is the contrast
// guard's reading, Tint the accent's derived tint ("" for an accent that is
// not #rrggbb), Unedited whether it is its theme's own document, and Label
// its name as a status line says it.
type ReportDesignCheck struct {
	Problem  *ReportDesignProblem `json:"problem,omitempty"`
	Accent   ReportAccentCheck    `json:"accent"`
	Tint     string               `json:"tint"`
	Unedited bool                 `json:"unedited"`
	Label    string               `json:"label"`
}

// ReportDesignProblem is a field the core refuses and its words.
type ReportDesignProblem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ReportAccentCheck is the contrast rule for the design's accent (D17):
// its ratio on white, the ratio it needs (4.5 where it colours text, 3
// where it colours only graphics), and the nearest shade that passes,
// which is the accent itself when it does. Valid is false for an accent
// that is not #rrggbb.
type ReportAccentCheck struct {
	Valid   bool    `json:"valid"`
	OK      bool    `json:"ok"`
	Ratio   float64 `json:"ratio"`
	Need    float64 `json:"need"`
	OnText  bool    `json:"onText"`
	Nearest string  `json:"nearest"`
}

// reportDesignReadTimeout bounds the read a run makes before it starts.
const reportDesignReadTimeout = 10 * time.Second

// ReportDesignOptions are the editor's choices. It reads nothing but this
// build, so it needs no sign-in.
func (s *SettingsAPI) ReportDesignOptions() ReportDesignOptions {
	return reportDesignOptions()
}

func reportDesignOptions() ReportDesignOptions {
	o := ReportDesignOptions{
		HeadingFonts:   reportdesign.FontsFor("headings"),
		BodyFonts:      reportdesign.FontsFor("body"),
		CodeFonts:      reportdesign.FontsFor("code"),
		Papers:         append([]string(nil), reportdesign.Papers...),
		TitleBlocks:    append([]string(nil), reportdesign.TitleBlocks...),
		HeadingStyles:  append([]string(nil), reportdesign.HeadingStyles...),
		QuoteStyles:    append([]string(nil), reportdesign.QuoteStyles...),
		PageNumbers:    append([]string(nil), reportdesign.PageNumberStyles...),
		MaxName:        reportdesign.MaxNameRunes,
		MaxFooterText:  reportdesign.MaxFooterTextRunes,
		DefaultThemeID: reportdesign.DefaultID,
	}
	for _, d := range reportdesign.Builtins() {
		t := ReportDesignTheme{ID: d.BasedOn, Design: d}
		if png, ok := reportthumbs.PNG(d.BasedOn); ok {
			t.Thumbnail = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
		o.Themes = append(o.Themes, t)
	}
	for _, f := range reportdesign.Families() {
		o.Families = append(o.Families, ReportDesignFamily{Name: f.Name, Kind: string(f.Kind), HeadingsOnly: f.HeadingsOnly})
	}
	return o
}

// CheckReportDesign is the core's verdict on the design the editor holds.
// It reads nothing but the design.
func (s *SettingsAPI) CheckReportDesign(d reportdesign.Design) ReportDesignCheck {
	return checkReportDesign(d)
}

func checkReportDesign(d reportdesign.Design) ReportDesignCheck {
	c := ReportDesignCheck{Label: designLabel(d), Unedited: isUnedited(d)}
	if err := d.Validate(); err != nil {
		var fe *reportdesign.FieldError
		if errors.As(err, &fe) {
			c.Problem = &ReportDesignProblem{Field: fe.Field, Message: fe.Message}
		} else {
			c.Problem = &ReportDesignProblem{Message: err.Error()}
		}
	}
	need := d.MinContrast()
	c.Accent = ReportAccentCheck{Need: need, OnText: d.AccentOnText(), Nearest: "#000000", Ratio: 1}
	if ratio, err := reportdesign.Contrast(d.Accent, reportdesign.White); err == nil {
		c.Accent.Valid = true
		c.Accent.Ratio = ratio
		c.Accent.OK = ratio >= need
		if near, err := reportdesign.NearestPassing(d.Accent, need); err == nil {
			c.Accent.Nearest = near
		}
		c.Tint, _ = reportdesign.Tint(d.Accent)
	}
	return c
}

// isUnedited reports whether d is its theme's own document, compared by
// the core's canonical form.
func isUnedited(d reportdesign.Design) bool {
	b, ok := reportdesign.Builtin(d.BasedOn)
	return ok && bytes.Equal(d.Canonical(), b.Canonical())
}

// designLabel is a design's name as a status line says it: a theme's own
// name when it is that theme unedited, marked when it is a theme edited,
// and the person's name with its theme otherwise (the site's words).
func designLabel(d reportdesign.Design) string {
	theme := d.BasedOn
	if b, ok := reportdesign.Builtin(d.BasedOn); ok {
		theme = b.Name
	}
	switch {
	case isUnedited(d):
		return d.Name
	case d.Name == theme:
		return d.Name + ", edited"
	default:
		return fmt.Sprintf("%s (from %s)", d.Name, theme)
	}
}

// ReportDesign reads the account's report design at the registry.
func (s *SettingsAPI) ReportDesign(url string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return reportDesignOf(s.app.ctx, url, token)
}

// SaveReportDesign holds a design to the core's rules and saves it as the
// account's, whole.
func (s *SettingsAPI) SaveReportDesign(url string, d reportdesign.Design) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return saveReportDesign(s.app.ctx, url, token, &d)
}

// ResetReportDesign clears the account's report design: never chosen
// again, which runs from this app draw as Plain.
func (s *SettingsAPI) ResetReportDesign(url string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return saveReportDesign(s.app.ctx, url, token, nil)
}

// withLogo asks the core for the design with its logo (agentry#1025).
func withLogo() *apiclient.ReportSettingsInclude {
	include := apiclient.ReportSettingsIncludeLogo
	return &include
}

// accountDesign is the account's report design document as the registry
// answers it, its logo included: nil when never chosen. found is false on a
// registry from before the setting. The document is the core's own bytes,
// not read through the generated type, so a field a later core adds is kept
// for the parse to see rather than dropped.
func accountDesign(ctx context.Context, url, token string) (doc []byte, found bool, err error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, false, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.GetUserReportSettingsWithResponse(ctx, &apiclient.GetUserReportSettingsParams{Include: withLogo()})
	if err != nil {
		return nil, false, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, false, registry.Problem("read your report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return settingsDesign(resp.Body)
}

// settingsDesign is the design in a report settings body, nil for {}.
func settingsDesign(body []byte) ([]byte, bool, error) {
	var s struct {
		Design json.RawMessage `json:"design"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, true, fmt.Errorf("the registry's report settings are not JSON: %w", err)
	}
	if len(s.Design) == 0 || bytes.Equal(s.Design, []byte("null")) {
		return nil, true, nil
	}
	return s.Design, true, nil
}

func reportDesignOf(ctx context.Context, url, token string) (*ReportDesignView, error) {
	doc, found, err := accountDesign(ctx, url, token)
	if err != nil || !found {
		return &ReportDesignView{}, err
	}
	return reportDesignView(doc), nil
}

func reportDesignView(doc []byte) *ReportDesignView {
	v := &ReportDesignView{Available: true}
	if doc == nil {
		return v
	}
	d, err := reportdesign.Parse(doc)
	if err != nil {
		v.Unreadable = err.Error()
		return v
	}
	// Parse held the logo to a PNG within the bounds; the page draws it
	// only as this data: URL.
	if d.Logo != (reportdesign.Logo{}) {
		v.Logo = "data:image/png;base64," + d.Logo.PNG
		d.Logo = reportdesign.Logo{}
	}
	v.Design, v.Label = &d, designLabel(d)
	return v
}

// saveReportDesign stores d, or clears the setting when d is nil (the
// logo with it). A design the core would refuse is refused here first, in
// the core's words, and nothing is sent. The design goes without a logo,
// whatever d holds, so the core keeps the one the account has.
func saveReportDesign(ctx context.Context, url, token string, d *reportdesign.Design) (*ReportDesignView, error) {
	body := []byte("{}")
	if d != nil {
		sent := *d
		sent.Logo = reportdesign.Logo{}
		if err := sent.Validate(); err != nil {
			return nil, err
		}
		body = append(append([]byte(`{"design":`), sent.Canonical()...), '}')
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.UpdateUserReportSettingsWithBodyWithResponse(ctx, &apiclient.UpdateUserReportSettingsParams{Include: withLogo()}, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, registry.Problem("save your report design", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	doc, _, err := settingsDesign(resp.Body)
	if err != nil {
		return nil, err
	}
	return reportDesignView(doc), nil
}

// ChooseReportLogo asks for a PNG or JPEG with the system's file picker and
// uploads it as the logo of the account's design. A cancelled pick changes
// nothing and answers no view. The path never comes from the page: the
// picker is opened here.
func (s *SettingsAPI) ChooseReportLogo(url string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	path, err := pickReportLogo(s.app.ctx)
	if err != nil || path == "" {
		return nil, err
	}
	return uploadReportLogo(s.app.ctx, url, token, path)
}

// RemoveReportLogo takes the logo off the account's design and keeps the
// design.
func (s *SettingsAPI) RemoveReportLogo(url string) (*ReportDesignView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return removeReportLogo(s.app.ctx, url, token)
}

// pickReportLogo is the system's file picker for a logo; a test replaces it.
var pickReportLogo = func(ctx context.Context) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:   "Choose your logo",
		Filters: []runtime.FileFilter{{DisplayName: "PNG or JPEG images", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
}

// errLogoTooLarge is the core's own bound, told before the file is sent.
var errLogoTooLarge = errors.New("a logo is at most 512 KB; nothing changed")

// uploadReportLogo sends the file at path as the design's logo: one
// multipart part named file, its bytes alone (not its name), at most the
// core's MaxLogoUploadBytes. The core decides what the bytes are, encodes
// the logo again from its pixels, and refuses with its own words (415 for
// anything but a PNG or a JPEG, 413 over a bound, 409 with no design saved).
func uploadReportLogo(ctx context.Context, url, token, path string) (*ReportDesignView, error) {
	f, err := os.Open(path) //nolint:gosec // the file the person picked
	if err != nil {
		return nil, fmt.Errorf("open the logo: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, reportdesign.MaxLogoUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the logo: %w", err)
	}
	if len(data) > reportdesign.MaxLogoUploadBytes {
		return nil, errLogoTooLarge
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "logo")
	if err == nil {
		_, err = part.Write(data)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return nil, err
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.PutUserReportLogoWithBodyWithResponse(ctx, mw.FormDataContentType(), &body)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, registry.Problem("set your logo", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	doc, _, err := settingsDesign(resp.Body)
	if err != nil {
		return nil, err
	}
	return reportDesignView(doc), nil
}

// removeReportLogo takes the logo off, then reads the design back.
func removeReportLogo(ctx context.Context, url, token string) (*ReportDesignView, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.DeleteUserReportLogoWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode()/100 != 2 {
		return nil, registry.Problem("remove your logo", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return reportDesignOf(ctx, url, token)
}

// reportThemeFor is the design a local run or chat at reg takes: the
// account's, read with the app's stored credential for that registry.
func (r *RunAPI) reportThemeFor(ctx context.Context, reg string, logw io.Writer) engine.ReportTheme {
	url := settings.Normalize(reg)
	return accountReportTheme(ctx, url, settings.TokenFor(url), logw)
}

// accountReportTheme is the account's report design for a run, whole, as
// the CLI's chooseReportTheme takes it: the zero value (Plain, chosen by
// the default) when there is no credential, the account never chose, or
// the registry has no such setting. A registry address Settings would not
// accept is never sent the token (D-030). An account it cannot read, or a
// design this build cannot draw, says so in the run's log and is Plain too;
// the run goes on, since how a PDF looks is no reason to stop one (D20).
func accountReportTheme(ctx context.Context, url, token string, logw io.Writer) engine.ReportTheme {
	if token == "" {
		return engine.ReportTheme{}
	}
	if settings.ValidateURL(url) != nil {
		_, _ = fmt.Fprintln(logw, "agentry: warning: the run's registry address is not one Settings accepts, so your account's report design was not read; PDFs use Plain")
		return engine.ReportTheme{}
	}
	ctx, cancel := context.WithTimeout(ctx, reportDesignReadTimeout)
	defer cancel()
	doc, _, err := accountDesign(ctx, url, token)
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report design could not be read (%v); PDFs use Plain\n", err)
	case doc == nil:
	default:
		d, err := reportdesign.Parse(doc)
		if err == nil {
			// The engine stamps the design with the pinned CLI's copy of
			// the core's rules, which a core bump could move apart from
			// this build's: a design that copy refuses would fail the run
			// in grantPDF, so it is held to that copy here too.
			_, err = toolsrv.StampTheme(d.Canonical())
		}
		if err != nil {
			_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report design is not one this app can draw (%v; update the app); PDFs use Plain\n", err)
			break
		}
		return engine.ReportTheme{Design: d.Canonical(), From: engine.ThemeFromAccount}
	}
	return engine.ReportTheme{}
}
