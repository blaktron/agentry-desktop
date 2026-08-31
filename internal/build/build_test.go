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
	m.Kind = "mcp-server"
	if err := m.Validate(); err == nil {
		t.Fatal("mcp-server should be refused by the form")
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
