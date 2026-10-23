package build

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/validate"
)

func model() *Model {
	m := Default("acme")
	m.Name = "hello-build"
	m.Summary = "Build-surface round-trip fixture"
	m.EgressMode = "hosts"
	m.EgressHosts = []string{"api.example.com"}
	m.FSWrite = "paths"
	m.FSWritePaths = []string{"out"}
	m.ExecCommands = []string{"jq"}
	m.Secrets = []string{"EXAMPLE_TOKEN"}
	m.MCPServers = []MCPServerRow{{Source: "npm", Package: "@scope/server-x", Version: "1.2.3", Args: []string{},
		Tools: []string{"list_issues"}, Secrets: []string{"GITHUB_PAT"}, Egress: []string{"api.github.com"}}}
	m.Tags = []string{"fixture", "build"}
	m.Instructions = "# hello-build\n\nDo the one job.\n"
	return m
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := model()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	// The written manifest must parse under the REAL closed-schema parser,
	// not merely our own loose loader.
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Parse(raw); err != nil {
		t.Fatalf("generated agent.yaml fails manifest.Parse: %v\n%s", err, raw)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]string{
		"name":    {got.Name, m.Name},
		"summary": {got.Summary, m.Summary},
		"egress":  {got.EgressMode, "hosts"},
		"fswrite": {got.FSWrite, "paths"},
		"instr":   {got.Instructions, m.Instructions},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s round-trip = %q, want %q", name, pair[0], pair[1])
		}
	}
	if len(got.Secrets) != 1 || got.Secrets[0] != "EXAMPLE_TOKEN" {
		t.Errorf("secrets round-trip = %v", got.Secrets)
	}
	// The declared MCP server survives the round
	// trip as a structured row, and the real parser sees the exact pin.
	if len(got.MCPServers) != 1 {
		t.Fatalf("mcpServers round-trip = %+v", got.MCPServers)
	}
	srv := got.MCPServers[0]
	if srv.Source != "npm" || srv.Package != "@scope/server-x" || srv.Version != "1.2.3" ||
		len(srv.Tools) != 1 || srv.Secrets[0] != "GITHUB_PAT" || srv.Egress[0] != "api.github.com" {
		t.Errorf("mcpServers row = %+v", srv)
	}
	parsed, _ := manifest.Parse(raw)
	if len(parsed.Capabilities.MCPServers) != 1 || parsed.Capabilities.MCPServers[0].NPM != "@scope/server-x" {
		t.Errorf("real parser sees %+v", parsed.Capabilities.MCPServers)
	}
}

// A PyPI server with its env renders as the manifest expects, a declared
// server's "*" egress is refused (the runner always refuses it), and
// a manifest without servers loads to an empty list.
func TestMCPServerRowsRenderAndValidate(t *testing.T) {
	dir := t.TempDir()
	m := model()
	m.MCPServers = []MCPServerRow{{Source: "pypi", Package: "mcp-server-fetch", Version: "0.6.2", Egress: []string{"*"},
		Tools: []string{}, Secrets: []string{}}}
	if err := Save(dir, m); err == nil || !strings.Contains(err.Error(), `refuses "*" for a declared server`) {
		t.Fatalf("a declared server's \"*\" must be refused, got %v", err)
	}
	m.MCPServers[0].Egress = []string{"example.org"}
	m.MCPServers[0].Env = []EnvRow{{Name: "USER_AGENT", Value: "agentry fetch: probe"}}
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(raw), "- pypi: mcp-server-fetch\n      version: 0.6.2\n      egress: [example.org]\n      env:\n        USER_AGENT: 'agentry fetch: probe'") {
		t.Fatalf("rendered:\n%s", raw)
	}
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("real parser: %v\n%s", err, raw)
	}
	if parsed.Capabilities.MCPServers[0].PyPI != "mcp-server-fetch" || parsed.Capabilities.MCPServers[0].Env["USER_AGENT"] != "agentry fetch: probe" {
		t.Errorf("parsed %+v", parsed.Capabilities.MCPServers)
	}
	got, err := Load(dir)
	if err != nil || len(got.MCPServers) != 1 || got.MCPServers[0].Source != "pypi" || len(got.MCPServers[0].Tools) != 0 {
		t.Fatalf("load: %v %+v", err, got.MCPServers)
	}

	plain := model()
	plain.MCPServers = nil
	if err := Save(dir, plain); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(dir); got.MCPServers == nil || len(got.MCPServers) != 0 {
		t.Fatalf("no servers must load as an empty list, got %#v", got.MCPServers)
	}

	for name, rows := range map[string][]MCPServerRow{
		"bad source": {{Source: "cargo", Package: "x", Version: "1"}},
		"no package": {{Source: "npm", Package: " ", Version: "1"}},
		"no version": {{Source: "npm", Package: "x"}},
		"duplicate":  {{Source: "npm", Package: "x", Version: "1"}, {Source: "npm", Package: "x", Version: "2"}},
	} {
		bad := model()
		bad.MCPServers = rows
		if err := bad.Validate(); err == nil {
			t.Errorf("%s: expected a refusal", name)
		}
	}
}

func TestSavedWorkspacePassesTheRealValidators(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	res, err := validate.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("scaffolded workspace fails validate: %+v", res)
	}
}

func TestSavePreservesHandEditedReadme(t *testing.T) {
	dir := t.TempDir()
	m := model()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hand-written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Summary = "changed summary"
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(b) != "hand-written\n" {
		t.Fatalf("Save overwrote a hand-edited README: %q", b)
	}
}

func TestValidateRefusals(t *testing.T) {
	m := model()
	m.Modes = []string{"daemon"}
	if err := m.Validate(); err == nil {
		t.Fatal("an unknown run mode should be refused by the form")
	}
	m = model()
	m.Modes = nil
	if err := m.Validate(); err == nil {
		t.Fatal("no run mode should be refused by the form")
	}
	m = model()
	m.Intake = []IntakeRow{{ID: "api_token", Prompt: "Token", Type: "secret"}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "API_TOKEN") {
		t.Fatalf("a secret intake question without its declared secret should be refused: %v", err)
	}
	m.Secrets = []string{"API_TOKEN"}
	if err := m.Validate(); err != nil {
		t.Fatalf("secret intake with its declared secret: %v", err)
	}
	m.Intake = []IntakeRow{{ID: "Region", Prompt: "Which?", Type: "choice"}}
	if err := m.Validate(); err == nil {
		t.Fatal("a bad intake id should be refused")
	}
	m = model()
	m.EgressMode = "hosts"
	m.EgressHosts = nil
	if err := m.Validate(); err == nil {
		t.Fatal("hosts mode with no hosts should be refused")
	}
}

// TestPublishHappyPath drives Publish against a fake registry: the agent
// does not exist (404 → create, public), the upload is accepted, and the
// version reaches published on the second poll.
func TestPublishHappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	polls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["namespace"] != "acme" || body["visibility"] != "public" {
			t.Errorf("create agent body = %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("submit content type = %s", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"version":"0.1.0","status":"scanning","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0", func(w http.ResponseWriter, _ *http.Request) {
		polls++
		status := "scanning"
		if polls > 1 {
			status = "published"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.1.0","status":"` + status + `","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versionStatus":"published","findings":[],"findingCounts":{}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var log strings.Builder
	res, err := Publish(t.Context(), srv.URL, "agy_test", dir, false, &log)
	if err != nil {
		t.Fatalf("Publish: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "published" || res.Version != "0.1.0" || res.Visibility != "public" {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(log.String(), "created agent acme/hello-build (public)") {
		t.Fatalf("log misses agent creation:\n%s", log.String())
	}
}

func TestPublishRefusesAnonymous(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(t.Context(), "http://reg.invalid", "", dir, false, io.Discard); err == nil {
		t.Fatal("anonymous publish should be refused before any network call")
	}
}

// A spent credit allowance: the registry answers 402 credits-exhausted
// before storing anything, and Publish returns it as a refused result —
// the sentence and the slug — rather than an error, so the page can
// offer the way onward.
func TestPublishReturnsTheAllowanceRefusal(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	var refusal string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespace":"acme","name":"hello-build","visibility":"private","modes":["autonomous"]}`))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(refusal))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	refusal = `{"type":"https://agentry.dev/problems/credits-exhausted","title":"Not enough credits for this submission","status":402,"detail":"A submission costs 10 credits; this account has 0, and its allowance resets on 11 October 2026.","balance":0,"resetsAt":"2026-10-11T00:00:00Z"}`
	var log strings.Builder
	res, err := Publish(t.Context(), srv.URL, "agy_test", dir, true, &log)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Status != StatusRefused || res.Refusal != "credits-exhausted" {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Detail, "A submission costs 10 credits; this account has 0") || !strings.HasPrefix(res.Detail, "Not enough credits for this submission: ") {
		t.Fatalf("detail = %q", res.Detail)
	}
	if res.Account != "" {
		t.Fatalf("a refusal naming no account carried %q", res.Account)
	}
	if !strings.Contains(log.String(), "refused:") {
		t.Fatalf("log lacks the refusal:\n%s", log.String())
	}

	// A refusal that names an account: the pool that is short is the
	// organization's, not the publisher's, so the page can say whose it is
	// — the sentence names an organization this person never typed.
	refusal = `{"type":"https://agentry.dev/problems/credits-exhausted","title":"Not enough credits for this submission","status":402,"detail":"a submission costs 10 credits; the acme organization's pool has 0, and its allowance resets on 11 October 2026.","account":"acme","balance":0,"tier":"business"}`
	res, err = Publish(t.Context(), srv.URL, "agy_test", dir, true, &log)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Refusal != "credits-exhausted" || res.Account != "acme" {
		t.Fatalf("a refusal naming an organization = %+v", res)
	}

	// An organization whose subscription never started is its own
	// refusal, not an empty balance, and the page offers the same way on.
	refusal = `{"type":"https://agentry.dev/problems/subscription-inactive","title":"The organization's subscription is not active","status":402,"detail":"the acme organization is awaiting payment: nothing is admitted until its subscription starts","account":"acme","standing":"pending","tier":"business"}`
	res, err = Publish(t.Context(), srv.URL, "agy_test", dir, true, &log)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Status != StatusRefused || res.Refusal != "subscription-inactive" || res.Account != "acme" {
		t.Fatalf("an inactive subscription = %+v", res)
	}
	if !strings.Contains(res.Detail, "awaiting payment") {
		t.Fatalf("detail = %q", res.Detail)
	}
}

// Load then Save loses nothing but comments. The template's agent.yaml
// is the file an author starts from: it carries authors, a homepage,
// runtime.models, a publish channel and an include order the form does not
// edit, and a save keeps them.
func TestTemplateManifestSurvivesLoadAndSave(t *testing.T) {
	orig, err := os.ReadFile(filepath.Join("testdata", "template-agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), orig, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("# my-quickagent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.HadComments {
		t.Error("the template is commented; HadComments must say so")
	}
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	want, err := manifest.Parse(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manifest.Parse(saved)
	if err != nil {
		t.Fatalf("saved file does not parse: %v\n%s", err, saved)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Load then Save changed the manifest:\nwant %+v\n got %+v\nsaved:\n%s", want, got, saved)
	}
	if again, _ := Load(dir); again.HadComments {
		t.Error("a file the form wrote has no comments")
	}
}

// Every v2 field the Model carries goes out and comes back.
func TestEveryFieldRoundTrips(t *testing.T) {
	dir := t.TempDir()
	m := model()
	m.Modes = []string{"autonomous", "interactive"}
	m.Opening = "What should I look at?"
	m.IdleTimeoutSeconds = 600
	m.Authors = []AuthorRow{{Name: "Ada: the author", Email: "ada@example.org"}}
	m.Homepage = "https://example.org/x"
	m.Guide = GuideModel{
		Overview: "Reads the incident.\n\nReturns a **summary**.",
		Prepare:  []string{"The ticket number", "Access to the logs"},
		Files:    "The log excerpt",
		Task:     "Summarise INC-1",
		Connect:  []ConnectRow{{Secret: "EXAMPLE_TOKEN", Help: "Create a read-only key: Settings, API"}},
	}
	m.RuntimeModels = []string{"claude-*"}
	m.MCPServers[0].Env = []EnvRow{{Name: "LOG_LEVEL", Value: "debug # not a comment"}}
	m.MCPServers[0].Args = []string{"mcp", "--transport=stdio,quiet", "#not-a-comment"}
	m.Dependencies = []DependencyRow{{Ref: "acme/helper", Version: "^1.2.0"}}
	m.PublishChannel = "beta"
	m.Intake = []IntakeRow{{ID: "region", Prompt: "Which region?", Type: "choice", Choices: []string{"emea", "apac"}, Required: true}}
	// The knowledge block: the form edits accepts and the ships list.
	m.KnowledgePrompt = "Attach your runbook: the steps it quotes come from it."
	m.KnowledgeRequired = true
	m.KnowledgeShips = []string{"glossary"}
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("does not parse: %v\n%s", err, raw)
	}
	if errs := manifest.Blocking(manifest.Validate(parsed)); len(errs) > 0 {
		t.Fatalf("V-rules: %v\n%s", errs, raw)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got.HadComments = false
	// An empty include means the four files the form writes, and the packs
	// it ships; loaded, it is the list the file names.
	m.PublishInclude = []string{"agent.yaml", "AGENT.md", "README.md", "LICENSE", "knowledge/**"}
	if !reflect.DeepEqual(m, got) {
		t.Errorf("round trip:\nwant %+v\n got %+v", m, got)
	}
}

// A value typed into a quoted position comes back from the real parser as
// typed, whatever characters it holds.
func TestTypedValuesCannotChangeTheShape(t *testing.T) {
	nasty := `x": 1, evil: true # ] } {`
	dir := t.TempDir()
	m := model()
	m.Summary = nasty
	m.License = nasty
	m.Tags = []string{nasty}
	m.Guide.Task = nasty
	m.MCPServers[0].Env = []EnvRow{{Name: "E", Value: nasty}}
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	p, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("does not parse: %v\n%s", err, raw)
	}
	if p.Summary != nasty || p.License != nasty || p.Tags[0] != nasty || p.Guide.Task != nasty || p.Capabilities.MCPServers[0].Env["E"] != nasty {
		t.Errorf("a typed value changed on the way through:\n%s", raw)
	}
}

// Check gives each section its own lines and its first problem, from the
// form's rules and the core's V-rules, so a step refuses what `agentry
// validate` would.
func TestCheckAttributesProblemsToSections(t *testing.T) {
	m := model()
	if res := m.Check(); res.First != nil {
		t.Fatalf("the fixture passes, got %+v", res.First)
	}
	m.EgressHosts = []string{"http://api.example.com"}
	m.Namespace = strings.Repeat("a", 40)
	m.Guide.Connect = []ConnectRow{{Secret: "NOT_DECLARED", Help: "help"}}
	m.Tags = []string{"Not OK"}
	res := m.Check()
	for sec, want := range map[string]string{
		"tools":   "not a valid DNS host",
		"guide":   "NOT_DECLARED",
		"publish": "namespace",
	} {
		if !strings.Contains(res.Problems[sec], want) {
			t.Errorf("section %s: want a problem naming %q, got %q", sec, want, res.Problems[sec])
		}
	}
	if res.First == nil || res.First.Section != SectionTools {
		t.Errorf("first problem in step order is Tools and web access, got %+v", res.First)
	}
	if _, ok := res.Problems["basics"]; ok {
		t.Errorf("basics has no problem, got %q", res.Problems["basics"])
	}
	if !strings.Contains(res.Fragments["tools"], "egress:") || !strings.Contains(res.Fragments["basics"], "name: hello-build") {
		t.Errorf("fragments: %+v", res.Fragments)
	}
	m = model()
	m.EgressMode = "all"
	if res := m.Check(); res.First != nil || len(res.Warnings) == 0 {
		t.Errorf("\"*\" egress for the QuickAgent is a warning, not a refusal: %+v %v", res.First, res.Warnings)
	}
}

// A key's help is markdown of up to 2,000 characters that may span lines,
// through the pinned core's V10: Check takes it, writes it as one quoted
// string, and refuses 2,001 and an escape character.
func TestCheckTakesMarkdownKeyHelpOfUpTo2000Characters(t *testing.T) {
	md := "1. Open **Settings → Developer settings → Fine-grained tokens**.\n2. Give it *read* access.\n\n\t- [Their guide](https://docs.github.com/)\n"
	pad := func(n int) string { return md + strings.Repeat("é", n-utf8.RuneCountInString(md)) }
	for _, c := range []struct {
		name, help string
		ok         bool
	}{
		{"2,000 characters over lines", pad(2000), true},
		{"2,001 characters", pad(2001), false},
		{"an escape character", "Create a key \x1b]8;;https://evil.example/\x07here", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := model()
			m.Guide.Connect = []ConnectRow{{Secret: "EXAMPLE_TOKEN", Help: c.help}}
			res := m.Check()
			got := res.Problems["guide"] + res.Problems["publish"]
			if c.ok && got != "" {
				t.Fatalf("valid help refused: %q", got)
			}
			if !c.ok && !strings.Contains(got, "guide.connect.EXAMPLE_TOKEN") {
				t.Fatalf("want a problem at guide.connect.EXAMPLE_TOKEN, got %q", got)
			}
			if c.ok && !strings.Contains(res.YAML, "EXAMPLE_TOKEN: \"1. Open") {
				t.Fatalf("the help is not one quoted string:\n%s", res.YAML)
			}
		})
	}
}

// Every value written bare is held to a grammar before Save writes it, so a
// value typed, loaded or imported cannot add a key the form does not show:
// a newline or a colon in an env name once could have declared a hidden
// server.
func TestBareValuesAreHeldToAGrammar(t *testing.T) {
	for name, mutate := range map[string]func(m *Model){
		"env name":          func(m *Model) { m.MCPServers[0].Env = []EnvRow{{Name: "A: {x", Value: "1"}} },
		"env name newline":  func(m *Model) { m.MCPServers[0].Env = []EnvRow{{Name: "A\n    - npm: evil", Value: "1"}} },
		"help key":          func(m *Model) { m.Guide.Connect = []ConnectRow{{Secret: "EXAMPLE_TOKEN: x", Help: "h"}} },
		"data category":     func(m *Model) { m.DataAccess = []string{"code, evil: true"} },
		"runtime interface": func(m *Model) { m.RuntimeInterfaces = []string{"agents-md]\nevil: [x"} },
		"publish channel":   func(m *Model) { m.PublishChannel = "stable\nevil: true" },
		"instruction file":  func(m *Model) { m.InstructionFile = "AGENT.md\nevil: true" },
		"intake id":         func(m *Model) { m.Intake = []IntakeRow{{ID: "a: b", Prompt: "p"}} },
	} {
		m := model()
		mutate(m)
		if err := Save(t.TempDir(), m); err == nil {
			t.Errorf("%s: a hostile value was saved", name)
		}
		if res := m.Check(); res.First == nil {
			t.Errorf("%s: Check reports no problem", name)
		}
	}
}

// A secret answer is carried by the QuickAgent's own declared secret; the
// runner does not count a server's (agentry-cli pkg/runner/intake).
func TestSecretQuestionNeedsTheQuickAgentsOwnKey(t *testing.T) {
	m := model()
	m.Secrets = []string{}
	m.MCPServers[0].Secrets = []string{"API_KEY"}
	m.Intake = []IntakeRow{{ID: "api_key", Prompt: "Your key", Type: "secret"}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "key of its own named API_KEY") {
		t.Fatalf("a secret question carried only by a server's key must be refused, got %v", err)
	}
	m.Secrets = []string{"API_KEY"}
	if err := m.Validate(); err != nil {
		t.Fatalf("with the QuickAgent's own key it passes, got %v", err)
	}
}

// Load reads the instruction file from the picked folder only, and refuses
// an oversized agent.yaml.
func TestLoadStaysInsideTheFolder(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "outside.md"), []byte("not yours"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "ws")
	m := model()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	hostile := strings.Replace(string(raw), "instruction: AGENT.md", "instruction: ../outside.md", 1)
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Instructions, "not yours") {
		t.Error("Load followed an entrypoint outside the folder")
	}
	if err := got.Validate(); err == nil {
		t.Error("an entrypoint outside the folder must not be saved")
	}
	big := make([]byte, maxManifestBytes+1)
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("an oversized agent.yaml must be refused")
	}
}

// privateAgentFixture is a registry holding acme/hello-build as private: it
// answers the upload with submit, publishes on the first poll, and fails
// the test on any visibility change; it records the order of the uploads.
func privateAgentFixture(t *testing.T, submit func(w http.ResponseWriter)) (string, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespace":"acme","name":"hello-build","visibility":"private","modes":["autonomous"]}`))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "submit")
		submit(w)
	})
	mux.HandleFunc("PATCH /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a Submit patched the agent: the desktop never changes a QuickAgent's visibility")
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.1.0","status":"published","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

func accepted(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"version":"0.1.0","status":"scanning","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
}

// TestPublishLeavesAPrivateAgentPrivate: a desktop sign-in cannot change a
// QuickAgent's visibility, so an agent that exists private takes the
// version and stays private; nothing is patched, and the result says why
// (the desktop's binding adds the site's settings page).
func TestPublishLeavesAPrivateAgentPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	reg, calls := privateAgentFixture(t, accepted)
	var log strings.Builder
	res, err := Publish(t.Context(), reg, "agy_test", dir, true, &log)
	if err != nil {
		t.Fatalf("Publish: %v\n%s", err, log.String())
	}
	if res.Status != "published" || res.Visibility != "private" || res.VisibilityNote != PrivateStaysNote || strings.Join(*calls, ",") != "submit" {
		t.Fatalf("result = %+v, calls = %v", res, *calls)
	}
}

// A refused submission leaves a private agent private: nothing is patched.
func TestPublishLeavesAPrivateAgentAloneWhenTheSubmissionIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	reg, calls := privateAgentFixture(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/credits-exhausted","title":"Not enough credits for this submission","status":402}`))
	})
	res, err := Publish(t.Context(), reg, "agy_test", dir, true, io.Discard)
	if err != nil || res.Status != StatusRefused || strings.Join(*calls, ",") != "submit" {
		t.Fatalf("result = %+v, %v, calls = %v", res, err, *calls)
	}
}

// The refusals to create the agent that the person can act on come back
// in words: a namespace they may not publish to, and the private-agent cap.
func TestPublishSaysWhyTheAgentCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		status int
		body   string
		slug   string
		words  string
	}{
		{http.StatusForbidden, `{"type":"https://agentry.dev/problems/forbidden","title":"Access denied","status":403,"detail":"Creating a QuickAgent here needs the namespace's owner, or active membership of the organization."}`, "forbidden", "Access denied: Creating a QuickAgent here needs the namespace's owner"},
		{http.StatusUnprocessableEntity, `{"type":"https://agentry.dev/problems/private-agent-cap","title":"Private QuickAgent limit reached","status":422,"detail":"This namespace holds 3 private QuickAgents, as many as its tier allows."}`, "private-agent-cap", "Private QuickAgent limit reached: This namespace holds 3"},
	} {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
		mux.HandleFunc("POST /api/v1/agents", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		})
		mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, _ *http.Request) { t.Error("uploaded after a refused create") })
		srv := httptest.NewServer(mux)
		res, err := Publish(t.Context(), srv.URL, "agy_test", dir, false, io.Discard)
		srv.Close()
		if err != nil || res.Status != StatusRefused || res.Refusal != c.slug || !strings.HasPrefix(res.Detail, c.words) {
			t.Errorf("%d: result = %+v, %v", c.status, res, err)
		}
	}
}

// A namespace the person may not publish to, met at the upload of an agent
// that exists, is said in words too.
func TestPublishSaysWhyTheUploadWasRefused(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespace":"acme","name":"hello-build","visibility":"public","modes":["autonomous"]}`))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/forbidden","title":"Access denied","status":403,"detail":"Publishing to acme needs the namespace's owner, or active membership of the organization."}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	res, err := Publish(t.Context(), srv.URL, "agy_test", dir, false, io.Discard)
	if err != nil || res.Status != StatusRefused || res.Refusal != "forbidden" || !strings.Contains(res.Detail, "Publishing to acme needs") {
		t.Fatalf("result = %+v, %v", res, err)
	}
}

// Submit is public: an include list that leaves out README.md or a
// licence file is Submit's problem, named before it, not the registry's
// refusal after the agent was created.
func TestCheckHoldsAPublicSubmitToV7(t *testing.T) {
	m := model()
	if c := m.Check(); c.Public != "" {
		t.Fatalf("the default include: %s", c.Public)
	}
	m.PublishInclude = []string{"agent.yaml", m.instructionFile()}
	c := m.Check()
	if !strings.Contains(c.Public, "LICENSE") || !strings.Contains(c.Public, "publish.include must carry it") {
		t.Fatalf("public problem = %q", c.Public)
	}
	// Submit's rule alone: it does not hold Save or a local Install.
	if c.First != nil || c.Problems[string(SectionPublish)] != "" {
		t.Fatalf("V7 reached the form's own problems: %+v", c.Problems)
	}
	m.PublishInclude = []string{"agent.yaml", m.instructionFile(), "LICENSE"}
	if p := m.Check().Public; !strings.Contains(p, "README.md") {
		t.Fatalf("public problem without README.md = %q", p)
	}
	for _, inc := range [][]string{{"*"}, {"agent.yaml", m.instructionFile(), "README.md", "LICENSE.md"}} {
		m.PublishInclude = inc
		if p := m.Check().Public; p != "" {
			t.Fatalf("%v carries both: %q", inc, p)
		}
	}
}

// An agent the warning said stays private that is now public, or gone,
// would take the version public: it is refused before anything is sent.
func TestPublishRefusesWhenTheAgentIsNoLongerPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{`{"namespace":"acme","name":"hello-build","visibility":"public","modes":["autonomous"]}`, ""} {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
			if agent == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(agent))
		})
		mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("%s %s after the refusal", r.Method, r.URL.Path)
		})
		srv := httptest.NewServer(mux)
		res, err := Publish(t.Context(), srv.URL, "agy_test", dir, true, io.Discard)
		srv.Close()
		if err != nil || res.Status != StatusRefused || res.Refusal != "visibility-changed" {
			t.Errorf("agent %q: result = %+v, %v", agent, res, err)
		}
	}
}

// The knowledge a QuickAgent asks for (knowledge.accepts, V11)
// is checked in the Before a run section: one line of
// at most 300 characters, and a required ask names what it asks for.
func TestCheckHoldsTheKnowledgeAskToV11(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prompt   string
		required bool
		want     string
	}{
		{"two lines", "Attach your runbook.\nAnd more.", false, "one line"},
		{"too long", strings.Repeat("a", 301), false, "at most 300 characters"},
		{"required with no words", "", true, "say which knowledge pack it needs"},
	} {
		m := model()
		m.KnowledgePrompt, m.KnowledgeRequired = tc.prompt, tc.required
		c := m.Check()
		if c.First == nil || c.First.Section != SectionGuide || !strings.Contains(c.First.Problem, tc.want) {
			t.Errorf("%s: %+v", tc.name, c.First)
		}
	}
	m := model()
	m.KnowledgePrompt, m.KnowledgeRequired = "Attach your runbook.", true
	if c := m.Check(); c.First != nil {
		t.Fatalf("a fit ask: %+v", c.First)
	}
	if !strings.Contains(m.fragment(SectionGuide), "accepts:") {
		t.Fatalf("the section's fragment shows no ask:\n%s", m.fragment(SectionGuide))
	}
}

// Save's LICENSE says the licence the manifest declares: the MIT text for MIT or an empty field, and for
// any other id a file naming it and pointing at its SPDX page. A LICENSE
// already in the folder is left alone.
func TestSaveWritesTheLicenceTheManifestDeclares(t *testing.T) {
	for license, want := range map[string][]string{
		"MIT":              {"MIT License", "Copyright (c)", "chris"},
		"":                 {"MIT License"},
		"Apache-2.0":       {"Apache-2.0", "licensed under the Apache-2.0 license", "https://spdx.org/licenses/Apache-2.0.html"},
		"GPL-3.0-or-later": {"https://spdx.org/licenses/GPL-3.0-or-later.html"},
	} {
		m := Default("chris")
		m.Name, m.Summary, m.License = "licence-check", "Checks the licence file.", license
		if license == "" {
			m.License = "MIT"
			if got := licenseFile("", "chris"); !strings.Contains(got, "MIT License") {
				t.Errorf("an empty field: %q", got)
			}
			continue
		}
		dir := t.TempDir()
		if err := Save(dir, m); err != nil {
			t.Fatalf("%s: %v", license, err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s: LICENSE lacks %q:\n%s", license, w, b)
			}
		}
		if license != "MIT" && strings.Contains(string(b), "MIT License") {
			t.Errorf("%s: LICENSE is the MIT text", license)
		}
	}
	// A licence the person placed is kept.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("my own text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Default("chris")
	m.Name, m.Summary, m.License = "licence-check", "Checks the licence file.", "Apache-2.0"
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "LICENSE")); string(b) != "my own text\n" {
		t.Fatalf("an existing LICENSE was replaced: %q", b)
	}
}

// A list item holding a comma, a bracket or a brace is one item after a
// save and a load: inside a flow sequence a plain scalar ends at the
// comma, so such an item is written double-quoted.
func TestAListItemWithACommaIsOneItemAfterASave(t *testing.T) {
	m := model()
	m.Guide.Overview = "Profiles a dataset."
	m.Guide.Prepare = []string{"The data file as CSV, TSV or JSON lines.", "Rules [if any], {as a list}", "plain"}
	m.Intake = []IntakeRow{{ID: "format", Prompt: "Which format?", Type: "choice", Choices: []string{"CSV, comma separated", "TSV", "a [bracketed] one"}}}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Guide.Prepare, m.Guide.Prepare) {
		t.Fatalf("prepare %q, want %q", got.Guide.Prepare, m.Guide.Prepare)
	}
	if len(got.Intake) != 1 || !slices.Equal(got.Intake[0].Choices, m.Intake[0].Choices) {
		t.Fatalf("choices %+v", got.Intake)
	}
	// A plain item stays plain, so files the form wrote before read the same.
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(raw), `"The data file as CSV, TSV or JSON lines."`) || !strings.Contains(string(raw), "plain]") {
		t.Fatalf("written:\n%s", raw)
	}
}

// Publish this version of a revision drafted In Agentry names its run as
// the upload's draftRun; every
// other upload names none.
func TestOnlyPublishRevisionSendsDraftRun(t *testing.T) {
	const run = "01a1019e-3224-70c9-a2a3-5a5f954de9e0"
	for _, c := range []struct {
		draftRun string
		publish  func(reg, dir string) error
	}{
		{"", func(reg, dir string) error {
			_, err := Publish(t.Context(), reg, "agy_test", dir, false, io.Discard)
			return err
		}},
		{run, func(reg, dir string) error {
			_, err := PublishRevision(t.Context(), reg, "agy_test", dir, false, run, io.Discard)
			return err
		}},
	} {
		dir := t.TempDir()
		if err := Save(dir, model()); err != nil {
			t.Fatal(err)
		}
		got, seen := "", false
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"namespace":"acme","name":"hello-build","visibility":"public","modes":["autonomous"],"summary":"s","quarantineState":"none","createdAt":"2026-08-31T00:00:00Z"}`))
		})
		mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Errorf("multipart: %v", err)
			}
			_, seen = r.MultipartForm.Value["draftRun"]
			got = r.FormValue("draftRun")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"version":"0.1.0","status":"published","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
		})
		mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.1.0","status":"published","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
		})
		mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0/report", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"versionStatus":"published","findings":[],"findingCounts":{}}`))
		})
		srv := httptest.NewServer(mux)
		if err := c.publish(srv.URL, dir); err != nil {
			t.Fatalf("draftRun %q: %v", c.draftRun, err)
		}
		srv.Close()
		if got != c.draftRun || seen != (c.draftRun != "") {
			t.Fatalf("draftRun sent %q (field present %v), want %q", got, seen, c.draftRun)
		}
	}
}

// A package server's args go from a loaded manifest to the saved one as
// they were written: in order, each word whole, none dropped or trimmed.
func TestAPackageServersArgsComeThroughTheForm(t *testing.T) {
	src := "schemaVersion: \"2\"\nname: scan\nnamespace: acme\nversion: 0.1.0\nsummary: s\nmodes: [autonomous]\n" +
		"capabilities:\n  mcpServers:\n    - pypi: semgrep\n      version: 1.140.0\n      args: [mcp, --transport, stdio]\n"
	m, err := decodeManifest([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp", "--transport", "stdio"}
	if got := m.MCPServers[0].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("read %q, want %q", got, want)
	}
	m.Instructions = "# scan\n"
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	p, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("does not parse: %v\n%s", err, raw)
	}
	if got := p.Capabilities.MCPServers[0].Args; !reflect.DeepEqual(got, want) {
		t.Errorf("saved %q, want %q:\n%s", got, want, raw)
	}
	if s := m.Summarize(nil); !reflect.DeepEqual(s.Servers[0].Args, want) {
		t.Errorf("the summary names %q, want %q", s.Servers[0].Args, want)
	}
}

// The form holds args to the core's rules before a save, and a server's
// row that is switched to remote writes none.
func TestTheFormChecksAServersArgs(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"empty":         {[]string{"mcp", ""}, "arg 2 is empty"},
		"two words":     {[]string{"mcp --stdio"}, "arg 1 must be one word"},
		"a tab":         {[]string{"mcp\t"}, "arg 1 must be one word"},
		"a format char": {[]string{"mc\u200bp"}, "arg 1 must be one word"},
		"expanded":      {[]string{"--home=$HOME"}, "arg 1 holds a $"},
		"a template":    {[]string{"{{.token}}"}, "arg 1 holds a $ or a {{ }}"},
		"too long":      {[]string{strings.Repeat("a", manifest.MaxMCPServerArgLen+1)}, "longer than 256"},
		"too many":      {make([]string, manifest.MaxMCPServerArgs+1), "at most 16 args"},
	}
	for name, c := range cases {
		m := model()
		m.MCPServers[0].Args = c.args
		err := m.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", name, c.want, err)
		}
	}
	m := model()
	m.MCPServers[0].Args = []string{"mcp", "--port=8080", "a,b"}
	if err := m.Validate(); err != nil {
		t.Errorf("literal words pass: %v", err)
	}
	if res := m.Check(); res.First != nil {
		t.Errorf("and the core's rules pass them: %+v", res.First)
	}
	m.MCPServers[0].Source, m.MCPServers[0].URL, m.MCPServers[0].Auth = "url", "https://mcp.example.com/mcp", "none"
	if strings.Contains(m.manifestYAML(), "args") {
		t.Errorf("a remote row wrote its args:\n%s", m.manifestYAML())
	}
}
