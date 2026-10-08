package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry/pkg/apiclient"
)

// The report theme (plan scratch-and-pdf-reports.md D13, D18, D21;
// agentry-desktop#730): how the PDFs a run exports with export_pdf are laid
// out. It is a profile setting on the account, kept by the core at GET/PUT
// /user/report-settings (agentry#989, DEV-227) on user:profile, which the
// app's sign-in carries; Settings → Profile reads and saves it, and a local
// run or chat takes it. The model never chooses it. An account that never
// chose reads back with no theme, which is not Plain: the CLI then falls to
// its own config, and this app, which has none, to Plain.

// ReportThemeView is the account's report theme as the Profile card shows
// it. Available is false on a registry from before the setting, where the
// card is not offered; Theme is "" when the account never chose one.
type ReportThemeView struct {
	Available bool   `json:"available"`
	Theme     string `json:"theme"`
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
		return &ReportThemeView{}, nil
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("read your report theme", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return reportThemeView(resp.JSON200), nil
}

func setReportTheme(ctx context.Context, url, token, theme string) (*ReportThemeView, error) {
	body := apiclient.UpdateUserReportSettingsJSONRequestBody{}
	if theme != "" {
		if !toolsrv.IsBuiltinTheme(theme) {
			return nil, fmt.Errorf("%q is not a report theme", theme)
		}
		t := apiclient.ReportTheme(theme)
		body.Theme = &t
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
	v := &ReportThemeView{Available: true}
	if s.Theme != nil {
		v.Theme = string(*s.Theme)
	}
	return v
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
		_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report theme could not be read (%v); PDFs use plain\n", err)
	case v.Theme == "":
	case !toolsrv.IsBuiltinTheme(v.Theme):
		_, _ = fmt.Fprintf(logw, "agentry: warning: your account's report theme %q is not one this app has (update the app); PDFs use plain\n", v.Theme)
	default:
		return engine.ReportTheme{Name: v.Theme, From: engine.ThemeFromAccount}
	}
	return engine.ReportTheme{}
}
