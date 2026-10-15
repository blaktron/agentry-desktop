// Package registry wraps the generated API client (agentry pkg/apiclient)
// with the same client-side rules the CLI applies: one timed HTTP client for
// every request, bearer tokens attached only to the registry's own host
// (never a CDN), buffered downloads capped, and RFC 9457 problem
// documents rendered into readable errors.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Client-side safety caps for release archives, mirroring the CLI. The
// registry enforces the real quotas; these only bound what this app is
// willing to buffer and unpack.
const (
	MaxArchiveBytes      int64 = 512 << 20
	ExtractMaxFileBytes  int64 = 25 << 20
	ExtractMaxTotalBytes int64 = 200 << 20
)

// ExtractLimits bounds what the app unpacks: an install, a run and a
// local install of a folder alike (identical to the CLI's). The client's
// job is only to refuse a bomb.
func ExtractLimits() pkgarchive.ExtractLimits {
	return pkgarchive.ExtractLimits{
		MaxFileBytes:  ExtractMaxFileBytes,
		MaxTotalBytes: ExtractMaxTotalBytes,
		MaxFiles:      20000,
		MaxRatio:      500,
	}
}

// httpTimeout bounds every request. Archive downloads are the long pole.
const httpTimeout = 5 * time.Minute

// HTTPClient is the one client every request goes through (also handed to
// pkg/trust so key fetches share the timeout). A request on it that carries
// the stored token never follows a redirect (KeepTokenHome).
var HTTPClient = &http.Client{Timeout: httpTimeout, CheckRedirect: KeepTokenHome}

// maxRedirects is net/http's own limit, kept for the requests that still
// follow one: setting CheckRedirect replaces the default that enforced it.
const maxRedirects = 10

// KeepTokenHome is the redirect policy of every client the stored token
// travels on. A request that carries an Authorization header is answered
// with the redirect, never sent on: Go keeps a hand-set Authorization on a
// redirect within the same domain or its subdomains, whatever the scheme, so
// following one could carry the token to http:// or to another host, past
// the https rule the registry address is held to before its token is looked
// up (the CLI refuses redirects on its token path the same way). A request
// without one, a public read, follows redirects as net/http does by default;
// that is decided on purpose, as the rule exists for the token.
func KeepTokenHome(_ *http.Request, via []*http.Request) error {
	if via[0].Header.Get("Authorization") != "" {
		return http.ErrUseLastResponse
	}
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	return nil
}

// New builds a generated client rooted at {registry}/api/v1, attaching
// "Authorization: Bearer <token>" when a token is known.
func New(regURL, token string) (*apiclient.ClientWithResponses, error) {
	opts := []apiclient.ClientOption{apiclient.WithHTTPClient(HTTPClient)}
	if token != "" {
		opts = append(opts, apiclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}))
	}
	c, err := apiclient.NewClientWithResponses(strings.TrimRight(regURL, "/")+"/api/v1", opts...)
	if err != nil {
		return nil, fmt.Errorf("create API client for %s: %w", regURL, err)
	}
	return c, nil
}

// Problem renders a failed API call as an error, preferring the RFC 9457
// problem document when the server sent one.
func Problem(op string, status int, p *apiclient.Problem, body []byte) error {
	if p != nil {
		msg := p.Title
		if p.Detail != nil && *p.Detail != "" {
			msg += ": " + *p.Detail
		}
		if p.Errors != nil {
			for _, fe := range *p.Errors {
				msg += fmt.Sprintf("; %s: %s", fe.Field, fe.Message)
			}
		}
		return fmt.Errorf("%s: %s (HTTP %d)", op, msg, status)
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return fmt.Errorf("%s: HTTP %d: %s", op, status, msg)
}

// ProblemSlug is a refused call's problem slug, the last segment of its
// type URI (https://agentry.dev/problems/<slug>), and its detail. The
// generated client decodes the problem only for a status the spec leaves
// to its default, so for a declared one (a delete's 409 and 422) it is
// read from the body. Both are empty when the body holds no problem.
func ProblemSlug(p *apiclient.Problem, body []byte) (slug, detail string) {
	if p == nil {
		var doc apiclient.Problem
		if json.Unmarshal(body, &doc) != nil {
			return "", ""
		}
		p = &doc
	}
	if i := strings.LastIndex(p.Type, "/"); i >= 0 {
		slug = p.Type[i+1:]
	}
	if p.Detail != nil {
		detail = *p.Detail
	}
	return slug, detail
}

// Digest computes the "sha256:<hex>" digest of b (the identity form).
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SameOrigin reports whether two URLs share an origin (scheme, host and
// port), so the bearer token is only ever attached to the registry itself,
// never to a CDN and never to an http:// spelling of the registry's host.
func SameOrigin(a, b string) bool {
	return settings.SameOrigin(a, b)
}

// Resolve calls GET /resolve for a ns/name[@range] reference. installIntent
// adds the X-Agentry-Install hint so the registry counts an install without
// a download round-trip through the API.
func Resolve(ctx context.Context, client *apiclient.ClientWithResponses, ref string, installIntent bool) (*apiclient.ResolveResult, error) {
	var editors []apiclient.RequestEditorFn
	if installIntent {
		editors = append(editors, func(_ context.Context, req *http.Request) error {
			req.Header.Set("X-Agentry-Install", "1")
			return nil
		})
	}
	resp, err := client.ResolveRefWithResponse(ctx, &apiclient.ResolveRefParams{Ref: ref}, editors...)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", ref, err)
	}
	if resp.JSON200 == nil {
		return nil, Problem("resolve "+ref, resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return resp.JSON200, nil
}

// Download fetches the release archive named by a resolve result.
// Relative archive URLs (self-host) resolve against the registry
// base URL.
func Download(ctx context.Context, regURL, token string, res *apiclient.ResolveResult) ([]byte, error) {
	archiveURL := res.ArchiveUrl
	if strings.HasPrefix(archiveURL, "/") {
		archiveURL = strings.TrimRight(regURL, "/") + archiveURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}
	if token != "" && SameOrigin(archiveURL, regURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", archiveURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, Problem("download archive", resp.StatusCode, nil, body)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read archive body: %w", err)
	}
	if int64(len(data)) > MaxArchiveBytes {
		return nil, fmt.Errorf("archive exceeds the %d-byte client cap", MaxArchiveBytes)
	}
	return data, nil
}

// ManifestFromResolve decodes the canonical manifest JSON carried by a
// resolve result into the typed manifest.
func ManifestFromResolve(res *apiclient.ResolveResult) (*manifest.Manifest, error) {
	raw, err := json.Marshal(res.Manifest)
	if err != nil {
		return nil, fmt.Errorf("re-encode resolved manifest: %w", err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode resolved manifest: %w", err)
	}
	return &m, nil
}

// Readme fetches the API's server-sanitized README HTML for a version
// ("" = latest published). The client never parses markdown itself: hostile
// READMEs are an XSS vector, and the sanitizer lives server-side.
func Readme(ctx context.Context, client *apiclient.ClientWithResponses, ns, name, version string) (string, error) {
	params := &apiclient.GetAgentReadmeParams{}
	if version != "" {
		params.Version = &version
	}
	resp, err := client.GetAgentReadmeWithResponse(ctx, ns, name, params)
	if err != nil {
		return "", fmt.Errorf("fetch readme: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", Problem("readme", resp.StatusCode(), nil, resp.Body)
	}
	return string(resp.Body), nil
}

// Unscanned says whether the registry admitted the resolved version
// without a scan: `scanStatus: skipped`, read before any attestation is
// looked for, because the absence is the fact and not a missing
// signature. A registry that predates the field never published one, so
// an absent answer is "no".
func Unscanned(res *apiclient.ResolveResult) bool {
	return res != nil && res.ScanStatus != nil && *res.ScanStatus == apiclient.ResolveResultScanStatusSkipped
}
