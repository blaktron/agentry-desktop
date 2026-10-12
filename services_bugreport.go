// Report bug in the desktop (plan report-bug.md §6, D1, D2, D6, D7;
// agentry-desktop#841): the sidebar's link above the version opens a card
// whose Submit files the text with the active registry's bug-reports
// extension (agentry-enterprise#544, POST /api/v1/x/bug-reports/reports) as
// the signed-in person. The report carries the app's version and OS and
// nothing else (D6). Staff read it in Staff › Bug reports on the site and
// mail the reporter "Reported bug fixed" when it is.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// The registry's bounds on a report's text (D7), counted as it counts them:
// characters, after trimming, with every line break as \n.
const (
	bugReportMin = 10
	bugReportMax = 4000
)

// BugReportResult is what the card says after Submit. Filed is true when
// the registry took the report. Otherwise Reason is one word the card maps
// to its sentence and its way on: "text" (out of bounds, nothing sent),
// "signin" (no credential here, or one the registry refused), "absent"
// (this registry takes no bug reports: no extension), "limited" (ten in
// the last hour), "refused" (the registry's own words are in Message) or
// "unreachable".
type BugReportResult struct {
	Filed   bool   `json:"filed"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ReportBug files text as a bug report from this app with the registry at
// url, the active one, with its stored credential. An error is only an
// address Settings' rule refuses; every answer of the registry is a result.
// The address is held to storedCredential's rule (D-030) before the token
// is looked up; a missing token is a result here, not an error, so the
// card can offer Sign in.
func (s *SettingsAPI) ReportBug(url, text string) (*BugReportResult, error) {
	url = settings.Normalize(url)
	if settings.ValidateURL(url) != nil {
		// Named by origin only: the address may carry a credential someone
		// typed into it (agentry-desktop#541).
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	body := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	if n := utf8.RuneCountInString(body); n < bugReportMin || n > bugReportMax {
		return &BugReportResult{Reason: "text"}, nil
	}
	token := settings.TokenFor(url)
	if token == "" {
		return &BugReportResult{Reason: "signin"}, nil
	}
	doc, err := json.Marshal(map[string]any{
		"body":    body,
		"source":  "desktop",
		"context": map[string]string{"appVersion": version.Version, "os": runtime.GOOS + "/" + runtime.GOARCH},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(s.app.ctx, http.MethodPost, strings.TrimRight(url, "/")+"/api/v1/x/bug-reports/reports", bytes.NewReader(doc))
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := noRedirects(registry.HTTPClient).Do(req)
	if err != nil {
		return &BugReportResult{Reason: "unreachable"}, nil
	}
	defer func() { _ = res.Body.Close() }()
	answer, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	return bugReportOutcome(res.StatusCode, answer), nil
}

// noRedirects is client with redirects answered, not followed: the bearer
// token is set by hand, and Go keeps it on a redirect within the same
// domain whatever the scheme, so a redirect to http:// would carry it in
// clear (the CLI refuses redirects on its token path the same way, DR-71).
// A redirect is then an answer the card reads as the registry refusing.
func noRedirects(c *http.Client) *http.Client {
	cp := *c
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &cp
}

// bugReportOutcome reads the registry's answer to a filing. A 403 is a
// sign-in the registry will not take for this (a token without
// user:profile) unless it is the core's refusal of an unactivated
// account's write, whose words say what to do instead.
func bugReportOutcome(status int, answer []byte) *BugReportResult {
	slug, detail := registry.ProblemSlug(nil, answer)
	switch {
	case status == http.StatusCreated:
		return &BugReportResult{Filed: true}
	case status == http.StatusNotFound:
		return &BugReportResult{Reason: "absent"}
	case status == http.StatusUnauthorized, status == http.StatusForbidden && slug != "account-unactivated":
		return &BugReportResult{Reason: "signin"}
	case status == http.StatusTooManyRequests:
		return &BugReportResult{Reason: "limited", Message: detail}
	case status >= 500:
		return &BugReportResult{Reason: "unreachable"}
	default:
		return &BugReportResult{Reason: "refused", Message: detail}
	}
}
