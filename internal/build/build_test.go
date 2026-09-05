package build

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	m.MCPServers = []MCPServerRow{{Source: "npm", Package: "@scope/server-x", Version: "1.2.3",
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
	// DEV-060 / desktop#5: the declared MCP server survives the round
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

// A PyPI server with a wildcard egress renders as the manifest expects
// (quoted "*"), and a manifest without servers loads to an empty list.
func TestMCPServerRowsRenderAndValidate(t *testing.T) {
	dir := t.TempDir()
	m := model()
	m.MCPServers = []MCPServerRow{{Source: "pypi", Package: "mcp-server-fetch", Version: "0.6.2", Egress: []string{"*"},
		Tools: []string{}, Secrets: []string{}}}
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(raw), "- pypi: mcp-server-fetch\n      version: 0.6.2\n      egress: ['*']") {
		t.Fatalf("rendered:\n%s", raw)
	}
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("real parser: %v\n%s", err, raw)
	}
	if parsed.Capabilities.MCPServers[0].PyPI != "mcp-server-fetch" || parsed.Capabilities.MCPServers[0].Egress[0] != "*" {
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
// does not exist (404 → create), the upload is accepted, and the version
// reaches published on the second poll.
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
		if body["namespace"] != "acme" || body["visibility"] != "private" {
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
	res, err := Publish(t.Context(), srv.URL, "agy_test", dir, "private", &log)
	if err != nil {
		t.Fatalf("Publish: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "published" || res.Version != "0.1.0" {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(log.String(), "created agent acme/hello-build") {
		t.Fatalf("log misses agent creation:\n%s", log.String())
	}
}

func TestPublishRefusesAnonymous(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, model()); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(t.Context(), "http://reg.invalid", "", dir, "private", io.Discard); err == nil {
		t.Fatal("anonymous publish should be refused before any network call")
	}
}
