package run

// An input source's access token on this computer (agentry-desktop#794):
// a key saved for a host is the CLI's own keychain entry source:<host>,
// shown as saved and never as its value; a run's source takes the token
// typed for this run first, then the saved key, then none; and the token
// reaches the engine's source alone, never the receipt words or the log.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
)

const (
	testSourceToken = "ghp-the-persons-source-token-91d2"
	testTypedToken  = "ghp-typed-for-this-run-only-5a0e"
	repoAnswer      = "https://GitHub.com/acme/private-repo#main"
)

// sourceKeychain gives the test an in-memory OS keychain under the CLI's
// own sources code.
func sourceKeychain(t *testing.T) {
	t.Helper()
	isolateRunEnv(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
}

func noToken(t *testing.T, what string, v any) {
	t.Helper()
	s := fmt.Sprintf("%+v", v)
	for _, tok := range []string{testSourceToken, testTypedToken} {
		if strings.Contains(s, tok) {
			t.Fatalf("%s carries a token: %s", what, s)
		}
	}
}

func TestASourceAnswerIsCheckedWithTheCLIsParserAndNamesItsHost(t *testing.T) {
	sourceKeychain(t)
	st, err := GetSourceKeyStatus("repository", repoAnswer)
	if err != nil || st.Problem != "" || st.Host != "github.com" || st.Saved || st.KeychainOff {
		t.Fatalf("status %+v (%v); want host github.com, nothing saved", st, err)
	}
	for answer, want := range map[string]string{
		"http://github.com/acme/repo":            "https",
		"https://user:pw@github.com/acme/repo":   "credentials",
		"https://github.com:8443/acme/repo":      "port",
		"https://github.com/acme/repo#main:docs": "ref",
		"":                                       "address first",
	} {
		st, err := GetSourceKeyStatus("repository", answer)
		if err != nil || !strings.Contains(st.Problem, want) || st.Host != "" {
			t.Errorf("%q: status %+v (%v); want a problem naming %q", answer, st, err, want)
		}
	}
	// A url source keeps its query out of what is shown.
	st, err = GetSourceKeyStatus("url", "https://files.example.com/report.pdf?sig=abc")
	if err != nil || st.Problem != "" || st.Host != "files.example.com" || strings.Contains(st.Shown, "sig=abc") {
		t.Fatalf("url status %+v (%v)", st, err)
	}
	if _, err := GetSourceKeyStatus("text", "anything"); err == nil {
		t.Fatal("a question that asks for no source must be refused")
	}
}

func TestASavedKeyIsTheCLIsEntryForTheHostAndIsNeverShownBack(t *testing.T) {
	sourceKeychain(t)
	if err := SaveSourceKey("repository", repoAnswer, "  \n"); err == nil {
		t.Fatal("an empty token must be refused")
	}
	if err := SaveSourceKey("repository", "http://github.com/acme/repo", testSourceToken); err == nil {
		t.Fatal("a token for an address the run would refuse must not be saved")
	}
	if err := SaveSourceKey("repository", repoAnswer, " "+testSourceToken+"\n"); err != nil {
		t.Fatal(err)
	}
	// The CLI's `agentry run` and a schedule's fire read this entry.
	got, err := keyring.Get(modelkey.Service, "source:github.com")
	if err != nil || got != testSourceToken {
		t.Fatalf("keychain source:github.com = %q (%v)", got, err)
	}
	if v, err := sources.SavedToken("github.com"); err != nil || v != testSourceToken {
		t.Fatalf("the CLI reads %q (%v)", v, err)
	}
	// Any address on the host shows it saved, a url question's too.
	st, err := GetSourceKeyStatus("url", "https://github.com/acme/other/raw/main/a.pdf")
	if err != nil || !st.Saved {
		t.Fatalf("status %+v (%v); want saved", st, err)
	}
	noToken(t, "the status", st)

	if err := ForgetSourceKey("repository", repoAnswer); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(modelkey.Service, "source:github.com"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("the entry is still there: %v", err)
	}
	if err := ForgetSourceKey("repository", repoAnswer); err != nil {
		t.Fatalf("forgetting a key that is not there: %v", err)
	}
}

func TestWithTheKeychainOffNothingIsReadOrSaved(t *testing.T) {
	sourceKeychain(t)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	st, err := GetSourceKeyStatus("repository", repoAnswer)
	if err != nil || !st.KeychainOff || st.Saved {
		t.Fatalf("status %+v (%v)", st, err)
	}
	if err := SaveSourceKey("repository", repoAnswer, testSourceToken); err == nil || !strings.Contains(err.Error(), "AGENTRY_NO_KEYRING") {
		t.Fatalf("save with the keychain off: %v", err)
	}
}

// answersFor parses repository answers as a run's intake does.
func answersFor(t *testing.T, given map[string]string) *intake.Answers {
	t.Helper()
	var qs []manifest.IntakeQuestion
	for id := range given {
		qs = append(qs, manifest.IntakeQuestion{ID: id, Prompt: id, Type: manifest.IntakeRepository, Required: true})
	}
	a, err := intake.Collect(nil, qs, given, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestASourceTakesTheTokenForThisRunThenTheSavedKeyThenNone(t *testing.T) {
	sourceKeychain(t)
	if err := SaveSourceKey("repository", repoAnswer, testSourceToken); err != nil {
		t.Fatal(err)
	}
	a := answersFor(t, map[string]string{
		"typed":     "https://github.com/acme/one",
		"saved":     "https://github.com/acme/two#v1.2.0",
		"anonymous": "https://gitlab.example.org/acme/three",
	})
	var warned []string
	if err := giveSourceTokens(a, map[string]string{"typed": " " + testTypedToken + " ", "saved": "  "}, func(m string) { warned = append(warned, m) }); err != nil {
		t.Fatal(err)
	}
	got := map[string][2]string{}
	for _, s := range a.SourceList() {
		got[s.ID] = [2]string{s.Token, s.TokenFrom}
	}
	want := map[string][2]string{
		"typed":     {testTypedToken, sources.TokenFromPrompt},
		"saved":     {testSourceToken, sources.TokenFromKeychain},
		"anonymous": {"", ""},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tokens %v; want %v", got, want)
	}
	if len(warned) != 0 {
		t.Fatalf("warned %v", warned)
	}
	// The intake document and the receipt name where it came from, never
	// the token.
	noToken(t, "intake.json", a.Document("/inputs"))
	for _, s := range a.SourceList() {
		noToken(t, "the receipt line", sources.Line(*s, sources.Fetched{}, ""))
	}
}

func TestATokenForNoSourceOfTheRunRefusesIt(t *testing.T) {
	sourceKeychain(t)
	a := answersFor(t, map[string]string{"codebase": "https://github.com/acme/one"})
	err := giveSourceTokens(a, map[string]string{"elsewhere": testTypedToken}, nil)
	if err == nil || !strings.Contains(err.Error(), "elsewhere") {
		t.Fatalf("want a refusal naming the id, got %v", err)
	}
	noToken(t, "the refusal", err)
	// An empty field for such an id is no token at all.
	if err := giveSourceTokens(a, map[string]string{"elsewhere": " "}, nil); err != nil {
		t.Fatal(err)
	}
}

const sourceRunManifest = `schemaVersion: "2"
name: reporter
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "desktop run fixture that asks for a repository"
entrypoints:
  instruction: SKILL.md
inputs:
  kinds: [code]
intake:
  - id: codebase
    prompt: "The repository to read"
    type: repository
    required: true
capabilities:
  filesystem:
    read: workspace
    write: workspace
publish:
  include: [agent.yaml, SKILL.md]
`

// A run prepared from the desktop's request gives the engine each source
// with its token, and only there: the log and the plan carry none.
func TestAPreparedRunGivesTheEngineTheSourceAndItsToken(t *testing.T) {
	isolateRunEnv(t)
	saveCataloguedPolicy(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, sourceRunManifest),
		archive: buildRunArchive(t, map[string]string{
			"agent.yaml": sourceRunManifest,
			"SKILL.md":   "# Reporter\n\nRead the repository.\n",
		}),
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	if err := SaveSourceKey("repository", repoAnswer, testSourceToken); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, typed, want, from string
	}{
		{"the saved key", "", testSourceToken, sources.TokenFromKeychain},
		{"a token for this run", testTypedToken, testTypedToken, sources.TokenFromPrompt},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := request(regURL, filepath.Join(t.TempDir(), "out"))
			req.Intake = map[string]string{"codebase": repoAnswer}
			req.SourceTokens = map[string]string{"codebase": c.typed}
			var log strings.Builder
			p, err := prepare(context.Background(), req, &log, manifest.ModeAutonomous)
			if err != nil {
				t.Fatalf("prepare: %v\n%s", err, log.String())
			}
			s := p.params.Intake.Sources["codebase"]
			if s == nil || s.Host != "github.com" || s.Ref != "main" || s.Token != c.want || s.TokenFrom != c.from {
				t.Fatalf("source %+v", s)
			}
			noToken(t, "the log", log.String())
			noToken(t, "the plan", p.plan)
		})
	}

	// A token for a question the release does not ask is refused before the
	// engine.
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.Intake = map[string]string{"codebase": repoAnswer}
	req.SourceTokens = map[string]string{"other": testTypedToken}
	_, err := prepare(context.Background(), req, &strings.Builder{}, manifest.ModeAutonomous)
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitCLI {
		t.Fatalf("want exit %d, got %v", engine.ExitCLI, err)
	}
}

// sourceManifest asks for a repository, a file at an address and a text
// answer.
func sourceManifest() *manifest.Manifest {
	return &manifest.Manifest{Intake: []manifest.IntakeQuestion{
		{ID: "typed", Prompt: "p", Type: manifest.IntakeRepository},
		{ID: "saved", Prompt: "p", Type: manifest.IntakeRepository},
		{ID: "anonymous", Prompt: "p", Type: manifest.IntakeRepository},
		{ID: "doc", Prompt: "p", Type: manifest.IntakeURL},
		{ID: "note", Prompt: "p", Type: manifest.IntakeText},
	}}
}

// A run In Agentry sends each answered source its token for the create's
// sourceTokens (agentry-desktop#833): typed for this run, else the key
// saved on this computer for its host, else none; an unanswered source and
// a text answer send nothing; a token typed for a question that is no
// answered source refuses the run, and so does an address the CLI would
// refuse.
func TestARunInAgentrySendsEachSourceItsToken(t *testing.T) {
	sourceKeychain(t)
	if err := SaveSourceKey("repository", repoAnswer, testSourceToken); err != nil {
		t.Fatal(err)
	}
	m := sourceManifest()
	answers := map[string]string{
		"typed":     "https://github.com/acme/one",
		"saved":     "https://github.com/acme/two#v1.2.0",
		"anonymous": "https://gitlab.example.org/acme/three",
		"doc":       "  ",
		"note":      "https://github.com/acme/not-a-source",
	}
	got, err := HostedSourceTokens(m, answers, map[string]string{"typed": " " + testTypedToken + " "}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"typed": testTypedToken, "saved": testSourceToken}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("sourceTokens %v; want %v", got, want)
	}
	for name, typed := range map[string]map[string]string{
		"a token for a text question":      {"note": testTypedToken},
		"a token for an unanswered source": {"doc": testTypedToken},
		"a token for a question it lacks":  {"nope": testTypedToken},
	} {
		if _, err := HostedSourceTokens(m, answers, typed, nil); err == nil {
			t.Errorf("%s: sent", name)
		} else {
			noToken(t, name, err.Error())
		}
	}
	bad := map[string]string{"typed": "http://github.com/acme/one"}
	if _, err := HostedSourceTokens(m, bad, nil, nil); err == nil || !strings.Contains(err.Error(), "typed") {
		t.Fatalf("an address the CLI refuses: %v", err)
	}
}

// A browser's site is no source In Agentry (agentry-desktop#903): a key
// saved for its host is not sent, an http:// site is not put through the
// https-only source parse, and a token typed for it refuses the run, as for
// any question that is no source.
func TestABrowserSiteTakesNoTokenInAgentry(t *testing.T) {
	sourceKeychain(t)
	if err := SaveSourceKey("url", "https://shop.example.com/", testSourceToken); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Intake:       []manifest.IntakeQuestion{{ID: "site", Prompt: "p", Type: manifest.IntakeURL}, {ID: "doc", Prompt: "p", Type: manifest.IntakeURL}},
		Capabilities: manifest.Capabilities{Browser: &manifest.Browser{Sites: []string{"site"}}},
	}
	for _, site := range []string{"https://shop.example.com/login", "http://shop.example.com/"} {
		got, err := HostedSourceTokens(m, map[string]string{"site": site, "doc": "https://shop.example.com/spec.pdf"}, nil, nil)
		if err != nil || got["site"] != "" || got["doc"] != testSourceToken {
			t.Fatalf("site %s: %v %v", site, got, err)
		}
	}
	if _, err := HostedSourceTokens(m, map[string]string{"site": "https://shop.example.com/"}, map[string]string{"site": testTypedToken}, nil); err == nil {
		t.Fatal("a token typed for the site was sent")
	} else {
		noToken(t, "a token for the site", err.Error())
	}
	keys, err := HostedScheduleKeys(m, map[string]string{"site": "https://shop.example.com/"}, nil, nil)
	if err != nil || len(keys) != 0 {
		t.Fatalf("a schedule's keys for a site: %v %v", keys, err)
	}
}

// A schedule In Agentry saves each source's token in Agentry as its
// host's key (agentry-enterprise#532): one key a host, so two sources on
// one host given different tokens are refused; a source with no token
// saves nothing.
func TestAScheduleInAgentrySavesOneKeyAHost(t *testing.T) {
	sourceKeychain(t)
	if err := SaveSourceKey("repository", repoAnswer, testSourceToken); err != nil {
		t.Fatal(err)
	}
	m := sourceManifest()
	answers := map[string]string{"saved": "https://github.com/acme/two", "anonymous": "https://gitlab.example.org/acme/three"}
	keys, err := HostedScheduleKeys(m, answers, nil, nil)
	if err != nil || fmt.Sprint(keys) != fmt.Sprint(map[string]string{"github.com": testSourceToken}) {
		t.Fatalf("keys %v %v", keys, err)
	}
	two := map[string]string{"typed": "https://github.com/acme/one", "saved": "https://github.com/acme/two"}
	if _, err := HostedScheduleKeys(m, two, map[string]string{"typed": testTypedToken}, nil); err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("two tokens for github.com: %v", err)
	} else {
		noToken(t, "the refusal", err.Error())
	}
	if keys, err := HostedScheduleKeys(m, two, map[string]string{"typed": testSourceToken}, nil); err != nil || len(keys) != 1 {
		t.Fatalf("the same token for both: %v %v", keys, err)
	}
}
