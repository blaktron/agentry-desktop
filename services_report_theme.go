package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/reportdesign"
)

// The report theme (plan scratch-and-pdf-reports.md D13, D18, D21;
// agentry-desktop#730): how the PDFs a run exports with export_pdf are laid
// out. It is a profile setting on the account, kept by the core at GET/PUT
// /user/report-settings (agentry#989, DEV-227) on user:profile, which the
// app's sign-in carries; Settings → Profile reads and saves it, and a local
// run or chat takes it. The model never chooses it. An account that never
// chose reads back with no theme, which is not Plain: the CLI then falls to
// its own config, and this app, which has none, to Plain.
//
// Since core agentry#1026 (report designs M2, DEV-230) the account keeps a
// report design, not a theme name, and the built-ins are plain, classic,
// modern, editorial and technical. Until agentry-desktop#759 brings the
// design editor, the card reads a design's basedOn as its theme and saves a
// built-in's own document (agentry-desktop#766, D-072's addendum).

// ReportThemeView is the account's report theme as the Profile card shows
// it. Available is false on a registry from before the setting, where the
// card is not offered; Theme is "" when the account never chose one, else
// the built-in the account's design is based on. Edited says the design is
// not that built-in as shipped (changed on the site), which saving a
// built-in here replaces. Choices are the built-ins, in the order offered.
type ReportThemeView struct {
	Available bool                `json:"available"`
	Theme     string              `json:"theme"`
	Edited    bool                `json:"edited,omitempty"`
	Choices   []ReportThemeChoice `json:"choices"`
}

// ReportThemeChoice is one built-in design the card offers.
type ReportThemeChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Here says the agentry command this app pins can draw it; a run
	// takes Plain for one it cannot (D20) until the app carries one that
	// can (agentry-cli#784).
	Here bool `json:"here"`
}

// reportThemeChoices are the core's built-in designs.
func reportThemeChoices() []ReportThemeChoice {
	out := []ReportThemeChoice{}
	for _, d := range reportdesign.Builtins() {
		out = append(out, ReportThemeChoice{ID: string(d.BasedOn), Name: d.Name, Here: toolsrv.IsBuiltinTheme(string(d.BasedOn))})
	}
	return out
}

// reportThemeReadTimeout bounds the read a run makes before it starts.
const reportThemeReadTimeout = 10 * time.Second

// ReportTheme reads the account's report theme at the registry.
func (s *SettingsAPI) ReportTheme(url string) (*ReportThemeView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return reportThemeOf(s.app.ctx, url, token)
}

// SetReportTheme saves the account's report theme: a built-in theme's
// name, or "" to clear the choice.
func (s *SettingsAPI) SetReportTheme(url, theme string) (*ReportThemeView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return setReportTheme(s.app.ctx, url, token, theme)
}

func reportThemeOf(ctx context.Context, url, token string) (*ReportThemeView, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.GetUserReportSettingsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return &ReportThemeView{Choices: reportThemeChoices()}, nil
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("read your report theme", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return reportThemeView(resp.JSON200), nil
}

func setReportTheme(ctx context.Context, url, token, theme string) (*ReportThemeView, error) {
	body := apiclient.UpdateUserReportSettingsJSONRequestBody{}
	if theme != "" {
		doc, ok := reportdesign.BuiltinDocument(theme)
		if !ok {
			return nil, fmt.Errorf("%q is not a report theme", theme)
		}
		var d apiclient.ReportDesign
		if err := json.Unmarshal(doc, &d); err != nil {
			return nil, fmt.Errorf("the %s report theme this app carries cannot be read: %w", theme, err)
		}
		body.Design = &d
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.UpdateUserReportSettingsWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("save your report theme", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return reportThemeView(resp.JSON200), nil
}

func reportThemeView(s *apiclient.ReportSettings) *ReportThemeView {
	v := &ReportThemeView{Available: true, Choices: reportThemeChoices()}
	if s.Design == nil {
		return v
	}
	v.Theme = string(s.Design.BasedOn)
	v.Edited = !isBuiltinAsShipped(s.Design)
	return v
}

// isBuiltinAsShipped reports whether d is its basedOn built-in unchanged,
// compared by the core's canonical form. One this build cannot parse is
// taken as edited. It sees d through the generated type, so a field a
// later core adds is not compared; #759 replaces this with the editor.
func isBuiltinAsShipped(d *apiclient.ReportDesign) bool {
	raw, err := json.Marshal(d)
	if err != nil {
		return false
	}
	got, err := reportdesign.Parse(raw)
	if err != nil {
		return false
	}
	want, ok := reportdesign.Builtin(string(d.BasedOn))
	return ok && got.SHA256() == want.SHA256()
}

// reportThemeFor is the theme a local run or chat at reg takes: the
// account's, read with the app's stored credential for that registry.
func (r *RunAPI) reportThemeFor(ctx context.Context, reg string, logw io.Writer) engine.ReportTheme {
	url := settings.Normalize(reg)
	return accountReportTheme(ctx, url, settings.TokenFor(url), logw)
}

// accountReportTheme is the account's report theme for a run: the zero
// value (Plain, chosen by the default) when there is no credential, the
// account never chose, the registry has no such setting, or it cannot be
// read or names a theme this build lacks. The last two say so in the
// run's log; the run goes on, since how a PDF looks is no reason to stop
// one (D21, as the CLI's chooseReportTheme does).
func accountReportTheme(ctx context.Context, url, token string, logw io.Writer) engine.ReportTheme {
	if token == "" {
		return engine.ReportTheme{}
	}
	ctx, cancel := context.WithTimeout(ctx, reportThemeReadTimeout)
	defer cancel()
	v, err := reportThemeOf(ctx, url, token)
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report theme could not be read (%v); PDFs use Plain\n", err)
	case v.Theme == "":
	case !toolsrv.IsBuiltinTheme(v.Theme):
		_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report theme %q is not one this app can draw yet (update the app); PDFs use Plain\n", v.Theme)
	default:
		if v.Edited {
			_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report design was changed on the site; this app draws %q as it ships until it is updated\n", v.Theme)
		}
		return engine.ReportTheme{Name: v.Theme, From: engine.ThemeFromAccount}
	}
	return engine.ReportTheme{}
}
