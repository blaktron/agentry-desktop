// Package voice is the desktop's Writing Voice: the two places a person
// keeps one, never synced on their own. This computer's is the CLI's local
// store (agentry-cli's pkg/knowledge/store, V1): sealed in the runner home,
// written by the Voice Writer or by hand, never reviewed, free. The
// account's is the registry's (the core's /api/v1/voice, V2, through
// agentry-cli's pkg/voice/remote, V3): reviewed by AI Review before it is
// live. Submit sends this computer's up for review; Use keeps the account's
// live one here. Discard, Restore and Delete in the account go to the core's
// routes through its generated client, since the CLI's client does not speak
// them; every account they answer is read again through the CLI's client,
// which holds it to its digests.
//
// The wiring is the Knowledge panel's (internal/knowledge.Env): the runner
// home the CLI shares, the OS keychain, and the sign-in Settings keeps. A
// registry URL is held to Settings' URL rule before a token is looked up
// for it, because the token is found by host alone.
package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/voice"
	"github.com/blaktron/agentry-cli/pkg/voice/build"
	"github.com/blaktron/agentry-cli/pkg/voice/remote"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// Env is what the voice is wired to: the knowledge store's wiring, whose
// key wrapping the voice shares.
type Env struct {
	knowledge.Env
	// Now dates a kept version; nil is time.Now.
	Now func() time.Time
}

// Default is this app's wiring.
func Default() Env { return Env{Env: knowledge.Default()} }

func (e Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Local is this computer's voice as the panel shows it: its text, which is
// at most 32 KiB and the person's own, and its record. Exists is false when
// this computer keeps none.
type Local struct {
	Exists       bool   `json:"exists"`
	Text         string `json:"text"`
	Digest       string `json:"digest"`
	Bytes        int    `json:"bytes"`
	Source       string `json:"source"`
	Phrase       string `json:"phrase"`
	AlwaysAttach bool   `json:"alwaysAttach"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
	// Problem is why a kept voice did not open (signed out of the registry
	// that wraps its key, or altered); the record is shown, the text not.
	Problem string `json:"problem,omitempty"`
}

func localOf(text []byte, m voice.Meta) *Local {
	return &Local{
		Exists: true, Text: string(text), Digest: m.SHA256, Bytes: m.Bytes, Source: string(m.Source), Phrase: m.Source.Phrase(),
		AlwaysAttach: m.AlwaysAttach, Created: m.Created, Updated: m.Updated,
	}
}

// Local reads this computer's voice. reg is the registry the panel shows:
// a voice whose key that account wraps opens only while signed in to it.
func (e Env) Local(ctx context.Context, reg string) (*Local, error) {
	s, err := e.Store(reg)
	if err != nil {
		return nil, err
	}
	kept, err := s.Voice()
	if errors.Is(err, voice.ErrNoVoice) {
		return &Local{}, nil
	}
	if err != nil {
		return nil, err
	}
	text, kept, err := s.OpenVoice(ctx)
	if err != nil {
		l := localOf(nil, kept.Meta)
		l.Problem = "This computer's voice did not open: " + err.Error()
		return l, nil
	}
	return localOf(text, kept.Meta), nil
}

// SaveLocal keeps text as this computer's voice, written by hand: free and
// immediate, never reviewed. created says it was the first voice here,
// which comes on for every run.
func (e Env) SaveLocal(ctx context.Context, reg, text string) (*Local, bool, error) {
	return e.keep(ctx, reg, []byte(text), voice.SourceEdit, nil)
}

func (e Env) keep(ctx context.Context, reg string, text []byte, src voice.Source, by *voice.Creator) (*Local, bool, error) {
	s, err := e.Store(reg)
	if err != nil {
		return nil, false, err
	}
	v, created, err := s.KeepVoice(ctx, text, src, by, e.now())
	if err != nil {
		return nil, false, err
	}
	return localOf(text, v.Meta), created, nil
}

// SetLocalAttach sets whether every run on this computer writes in its
// voice unless the run says otherwise.
func (e Env) SetLocalAttach(reg string, on bool) error {
	s, err := e.Store(reg)
	if err != nil {
		return err
	}
	_, err = s.SetVoiceAttach(on)
	return err
}

// DeleteLocal removes this computer's voice, its sealed copy and its key.
func (e Env) DeleteLocal(reg string) error {
	s, err := e.Store(reg)
	if err != nil {
		return err
	}
	return s.DeleteVoice()
}

// Version is one version of the account's voice, with its text.
type Version struct {
	Review     string           `json:"review"`
	Source     string           `json:"source"`
	Digest     string           `json:"digest"`
	Bytes      int              `json:"bytes"`
	Text       string           `json:"text"`
	Verdict    string           `json:"verdict,omitempty"`
	Findings   []remote.Finding `json:"findings,omitempty"`
	CreatedAt  time.Time        `json:"createdAt"`
	ReviewedAt *time.Time       `json:"reviewedAt,omitempty"`
	// QuestionSet is the AI Review question set the version was reviewed
	// under; a previous version under the account's current set is
	// restored at once, under another it is reviewed again.
	QuestionSet string `json:"questionSet,omitempty"`
}

// Account is the account's voice as the panel shows it. Exists is false
// when the account keeps none.
type Account struct {
	Exists       bool     `json:"exists"`
	AlwaysAttach bool     `json:"alwaysAttach"`
	Live         *Version `json:"live,omitempty"`
	Pending      *Version `json:"pending,omitempty"`
	Previous     *Version `json:"previous,omitempty"`
	// QuestionSet is the set AI Review asks now.
	QuestionSet string `json:"questionSet,omitempty"`
}

func versionOf(v *remote.Version) *Version {
	if v == nil {
		return nil
	}
	return &Version{
		Review: string(v.Review), Source: v.Source, Digest: v.Digest, Bytes: v.Bytes, Text: v.Text,
		Verdict: v.Verdict, Findings: v.Findings, CreatedAt: v.CreatedAt, ReviewedAt: v.ReviewedAt, QuestionSet: v.QuestionSet,
	}
}

func accountOf(a remote.Account) *Account {
	return &Account{
		Exists: true, AlwaysAttach: a.AlwaysAttach, Live: versionOf(a.Live), Pending: versionOf(a.Pending), Previous: versionOf(a.Previous),
		QuestionSet: a.QuestionSet,
	}
}

// client is the signed-in client of the account at reg, or
// remote.ErrSignedOut.
func (e Env) client(reg string) (*remote.Client, error) {
	reg, token, err := e.signedIn(reg)
	if err != nil {
		return nil, err
	}
	return remote.New(reg, token, e.HTTP)
}

// signedIn is reg held to Settings' URL rule, then its token: the token is
// found by host alone, so the rule comes first.
func (e Env) signedIn(reg string) (string, string, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return "", "", fmt.Errorf("the registry: %w", err)
	}
	token := ""
	if e.TokenFor != nil {
		token = e.TokenFor(reg)
	}
	if token == "" {
		return "", "", fmt.Errorf("%s: %w (sign in to it in Settings)", reg, remote.ErrSignedOut)
	}
	return reg, token, nil
}

// api is the core's generated client at reg, signed in, for the voice
// routes the CLI's client does not speak: Discard, Restore and
// Delete.
func (e Env) api(reg string) (*apiclient.ClientWithResponses, error) {
	reg, token, err := e.signedIn(reg)
	if err != nil {
		return nil, err
	}
	hc := e.HTTP
	if hc == nil {
		hc = registry.HTTPClient
	}
	return apiclient.NewClientWithResponses(reg+"/api/v1",
		apiclient.WithHTTPClient(hc),
		apiclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}))
}

// voiceProblem is a refused voice call in the registry's words, made
// printable on one line and cut as the CLI's voice client cuts them (a
// title at 120 characters, a detail at 400).
func voiceProblem(op string, res *http.Response, p *apiclient.Problem, body []byte) error {
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("%s: %w (sign in to it again in Settings)", op, remote.ErrSignedOut)
	}
	// A status the spec declares is not parsed as a problem by the
	// generated client; its body still is one.
	if p == nil && len(body) > 0 {
		var parsed apiclient.Problem
		if json.Unmarshal(body, &parsed) == nil && parsed.Title != "" {
			p = &parsed
		}
	}
	if p != nil {
		clean := apiclient.Problem{Title: printable(p.Title, 120)}
		if p.Detail != nil {
			d := printable(*p.Detail, 400)
			clean.Detail = &d
		}
		return registry.Problem(op, status, &clean, nil)
	}
	return registry.Problem(op, status, nil, []byte(printable(string(body), 300)))
}

// printable is s with every control character a space, cut at n
// characters.
func printable(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// DiscardAccount drops the account's pending version, in review or
// flagged: DELETE /voice/pending. The live voice stays live. The account
// is read again through the CLI's client, which holds every version to its
// digest and to plain text.
func (e Env) DiscardAccount(ctx context.Context, reg string) (*Account, error) {
	c, err := e.api(reg)
	if err != nil {
		return nil, err
	}
	res, err := c.DiscardVoicePendingWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, voiceProblem("discard your new version", res.HTTPResponse, res.ApplicationproblemJSONDefault, res.Body)
	}
	return e.Account(ctx, reg)
}

// RestoreAccount puts the previous version back: POST /voice/restore. One
// reviewed under the set AI Review asks now trades places with the live one
// at once; one reviewed under another becomes the pending version and is
// reviewed again, which inReview says (and which the registry may charge
// for, as any review).
func (e Env) RestoreAccount(ctx context.Context, reg string) (a *Account, inReview bool, err error) {
	c, err := e.api(reg)
	if err != nil {
		return nil, false, err
	}
	res, err := c.RestoreVoiceWithResponse(ctx)
	if err != nil {
		return nil, false, err
	}
	switch {
	case res.JSON200 != nil:
	case res.JSON202 != nil:
		inReview = true
	default:
		return nil, false, voiceProblem("restore your previous version", res.HTTPResponse, res.ApplicationproblemJSONDefault, res.Body)
	}
	a, err = e.Account(ctx, reg)
	return a, inReview, err
}

// DeleteAccount deletes the account's voice, every version of it and its
// data key: DELETE /voice. This computer's voice is not touched.
func (e Env) DeleteAccount(ctx context.Context, reg string) error {
	c, err := e.api(reg)
	if err != nil {
		return err
	}
	res, err := c.DeleteVoiceWithResponse(ctx)
	if err != nil {
		return err
	}
	if res.StatusCode() != http.StatusNoContent {
		return voiceProblem("delete your account's voice", res.HTTPResponse, res.ApplicationproblemJSONDefault, res.Body)
	}
	return nil
}

// Account reads the account's voice at reg. An account that keeps none is
// an Account with Exists false; signed out is remote.ErrSignedOut and a
// registry that keeps no voices remote.ErrUnconfigured.
func (e Env) Account(ctx context.Context, reg string) (*Account, error) {
	c, err := e.client(reg)
	if err != nil {
		return nil, err
	}
	a, err := c.Get(ctx)
	if errors.Is(err, remote.ErrNoAccountVoice) {
		return &Account{}, nil
	}
	if err != nil {
		return nil, err
	}
	return accountOf(a), nil
}

// Submit sends this computer's voice to the account at reg as its pending
// version: AI Review reads it before it is live, and the registry may
// charge for that. Nothing here changes.
func (e Env) Submit(ctx context.Context, reg string) (*Account, error) {
	s, err := e.Store(reg)
	if err != nil {
		return nil, err
	}
	text, _, err := s.OpenVoice(ctx)
	if err != nil {
		return nil, err
	}
	return e.SaveAccount(ctx, reg, string(text))
}

// SaveAccount sends text as the account's pending version, as a client's
// upload: AI Review reads it before it is live.
func (e Env) SaveAccount(ctx context.Context, reg, text string) (*Account, error) {
	c, err := e.client(reg)
	if err != nil {
		return nil, err
	}
	a, err := c.Upload(ctx, []byte(text))
	if err != nil {
		return nil, err
	}
	return accountOf(a), nil
}

// ErrNoLiveVoice is an account with no voice that AI Review has passed.
var ErrNoLiveVoice = errors.New("your account has no Writing Voice that AI Review has passed, so there is none to use here")

// UseAccount keeps the account's live voice as this computer's, in place of
// the one here, if any. The client held the text to its digest and to plain
// text when it read it; this computer records it as downloaded, and a run's
// receipt says it was reviewed there and not checked again here.
func (e Env) UseAccount(ctx context.Context, reg string) (*Local, bool, error) {
	c, err := e.client(reg)
	if err != nil {
		return nil, false, err
	}
	a, err := c.Get(ctx)
	if errors.Is(err, remote.ErrNoAccountVoice) {
		return nil, false, ErrNoLiveVoice
	}
	if err != nil {
		return nil, false, err
	}
	if a.Live == nil {
		return nil, false, ErrNoLiveVoice
	}
	return e.keep(ctx, reg, []byte(a.Live.Text), voice.SourceDownload, nil)
}

// SetAccountAttach sets whether runs In Agentry write in the account's live
// voice unless a run says otherwise.
func (e Env) SetAccountAttach(ctx context.Context, reg string, on bool) (*Account, error) {
	c, err := e.client(reg)
	if err != nil {
		return nil, err
	}
	a, err := c.SetAlwaysAttach(ctx, on)
	if err != nil {
		return nil, err
	}
	return accountOf(a), nil
}

// Attach is this computer's voice as a local run attaches it: its text,
// digested, and how the receipt names where it came from.
// nil when this computer keeps none. A kept voice that will not open is a
// refusal, as a kept pack's is.
func (e Env) Attach(ctx context.Context, reg string) (*engine.AttachedVoice, error) {
	s, err := e.Store(reg)
	if err != nil {
		return nil, err
	}
	if _, err := s.Voice(); errors.Is(err, voice.ErrNoVoice) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	text, kept, err := s.OpenVoice(ctx)
	if err != nil {
		return nil, fmt.Errorf("this computer's Writing Voice did not open: %w", err)
	}
	return engine.AttachVoice(text, kept.Meta.Origin())
}

// Builder is the Voice Writer's build: agentry-cli's pkg/voice/build, or a
// test's.
type Builder func(ctx context.Context, d dockerx.Client, o build.Options) (*build.Result, error)

// BuildRequest is one Voice Writer run on this computer.
type BuildRequest struct {
	Registry string
	// Job is build.Create, Improve or Edit.
	Job build.Job
	// Samples are files and folders the native picker returned (Create,
	// Improve); Brief is the brief (Create, Improve) or the note (Edit).
	Samples []string
	Brief   string
	// URLs are web addresses of the person's writing, beside or instead of
	// samples; each page is read by AI Review with the person's own
	// TypeSafe key first.
	URLs    []string
	RunID   string
	Build   Builder
	Log     io.Writer
	OnUsage func(receipt.Usage)
}

// BuildResult is what a Voice Writer run on this computer did.
type BuildResult struct {
	Voice *Local `json:"voice"`
	// Created says it was the first voice here, now on for every run.
	Created bool     `json:"created"`
	Samples []string `json:"samples,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	// Pages are the web addresses it was given and what became of each.
	Pages []Page `json:"pages,omitempty"`
}

// Page is one web address a build was given: the sample it became, or that
// the Voice Writer read it rendered, or why AI Review withheld it.
type Page struct {
	URL      string `json:"url"`
	Sample   string `json:"sample,omitempty"`
	Rendered bool   `json:"rendered,omitempty"`
	Withheld string `json:"withheld,omitempty"`
}

// Model names the model the runner policy gives the Voice Writer.
func Model() string { return knowledge.PolicyModel() }

// Build runs the Voice Writer on this computer against the runner policy's
// model, as `agentry voice create|improve|edit` does, and keeps what it
// wrote as this computer's voice. It costs no credits and nothing it reads
// or writes is reviewed.
func (e Env) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	s, err := e.Store(req.Registry)
	if err != nil {
		return nil, err
	}
	var current []byte
	if req.Job != build.Create {
		if current, _, err = s.OpenVoice(ctx); err != nil {
			if errors.Is(err, voice.ErrNoVoice) {
				return nil, errors.New("this computer keeps no Writing Voice to improve or edit: create one first")
			}
			return nil, err
		}
	}
	home, err := e.Home()
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, polSHA, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	logw := req.Log
	if logw == nil {
		logw = io.Discard
	}
	urls, review, web, err := pageReview(pol, req.URLs)
	if err != nil {
		return nil, err
	}
	reviewed := "nothing it reads or writes is reviewed"
	if len(urls) > 0 {
		reviewed = "nothing it reads from files, or writes, is reviewed"
	}
	_, _ = fmt.Fprintf(logw, "The Voice Writer runs on this computer against the model your runner policy names (%s). "+
		"It costs no credits, and %s.\n", knowledge.CreatorModel(pol), reviewed)
	if len(urls) > 0 {
		_, _ = fmt.Fprintln(logw, "Each page from a web address is fetched first and read by AI Review with your TypeSafe key; one it flags is withheld. "+
			"A page drawn by scripts is rendered in a headless browser and read by AI Review there.")
	}
	// The run's reports hold the voice in the clear: a private folder in
	// the runner home, removed with the build.
	outDir, err := os.MkdirTemp(home, ".voice-run-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(outDir) }()
	if outDir, err = filepath.Abs(outDir); err != nil {
		return nil, err
	}
	b := req.Build
	if b == nil {
		b = build.Build
	}
	budgets := settings.BuiltinBudgets(localrun.VoiceWriter(), pol.Budgets).Run
	res, err := b(ctx, dockerx.New(), build.Options{
		Job: req.Job, Samples: req.Samples, URLs: urls, Review: review, Web: web, Brief: strings.TrimSpace(req.Brief), Current: current,
		Home: home, Policy: pol, PolicySHA: polSHA, PolicyFile: &polFile, Build: version.Build(),
		OutDir: outDir, Budgets: &budgets, RunID: req.RunID, Stderr: logw, OnUsage: req.OnUsage,
	})
	if res != nil {
		for _, p := range res.Pages {
			if p.Withheld != "" {
				_, _ = fmt.Fprintf(logw, "withheld: %s (%s)\n", p.URL, p.Withheld)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	by := &voice.Creator{CLI: version.Build(), Model: pol.Model.Name}
	l, created, err := e.keep(ctx, req.Registry, res.Text, req.Job.Source(), by)
	if err != nil {
		return nil, fmt.Errorf("the Voice Writer wrote a voice that could not be kept: %w", err)
	}
	out := &BuildResult{Voice: l, Created: created, Samples: res.Samples, Skipped: res.Skipped}
	for _, p := range res.Pages {
		out.Pages = append(out.Pages, Page{URL: p.URL, Sample: p.Sample, Rendered: p.Rendered, Withheld: p.Withheld})
	}
	return out, nil
}
