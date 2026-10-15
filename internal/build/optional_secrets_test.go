package build

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// Optional secrets: each key the QuickAgent or a server declares is
// Required or Optional, the model holds the Required ones, and one the
// author attaches starts as Optional.

func keyed() *Model {
	m := bare()
	m.Secrets = []string{"API_TOKEN"}
	m.MCPServers = []MCPServerRow{{Source: "npm", Package: "some-search-server", Version: "1.2.3", Secrets: []string{"SEARCH_KEY"}}}
	return m
}

func saved(t *testing.T, m *Model) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Parse(raw); err != nil {
		t.Fatalf("agent.yaml fails manifest.Parse: %v\n%s", err, raw)
	}
	return dir, string(raw)
}

func TestEveryAttachedKeyStartsOptional(t *testing.T) {
	_, y := saved(t, keyed())
	if !strings.Contains(y, `  optionalSecrets: [API_TOKEN, SEARCH_KEY]`) {
		t.Fatalf("a key nobody marked is optional:\n%s", y)
	}
}

func TestRequiredKeysRoundTrip(t *testing.T) {
	m := keyed()
	m.RequiredSecrets = []string{"SEARCH_KEY"}
	dir, y := saved(t, m)
	if !strings.Contains(y, `  optionalSecrets: [API_TOKEN]`) {
		t.Fatalf("only the unmarked key is optional:\n%s", y)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.RequiredSecrets, []string{"SEARCH_KEY"}) {
		t.Fatalf("RequiredSecrets after a load = %v", got.RequiredSecrets)
	}
	m.RequiredSecrets = []string{"API_TOKEN", "SEARCH_KEY"}
	if _, y := saved(t, m); strings.Contains(y, "optionalSecrets") {
		t.Fatalf("every key Required writes no optionalSecrets:\n%s", y)
	}
}

// A release without optionalSecrets, as every one before it, comes in with
// every key Required, as it runs, and saves unchanged in that respect.
func TestAManifestWithoutOptionalSecretsLoadsEveryKeyRequired(t *testing.T) {
	m := keyed()
	m.RequiredSecrets = []string{"API_TOKEN", "SEARCH_KEY"}
	dir, _ := saved(t, m)
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.RequiredSecrets, []string{"API_TOKEN", "SEARCH_KEY"}) {
		t.Fatalf("RequiredSecrets = %v, want every declared key", got.RequiredSecrets)
	}
	if _, y := saved(t, got); strings.Contains(y, "optionalSecrets") {
		t.Fatalf("a release without optionalSecrets saves without it:\n%s", y)
	}
}

func TestARequiredMarkOnAnUndeclaredKeyIsRefused(t *testing.T) {
	m := keyed()
	m.RequiredSecrets = []string{"OTHER_KEY"}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "OTHER_KEY is marked Required but is not a declared key") {
		t.Fatalf("err = %v", err)
	}
}

// A secret question carried by the QuickAgent's own key is required
// exactly when that key is, whatever its box says.
func TestASecretQuestionFollowsItsKey(t *testing.T) {
	m := keyed()
	m.Intake = []IntakeRow{{ID: "api_token", Prompt: "Your API token", Type: "secret", Required: true}}
	if _, y := saved(t, m); strings.Contains(y, "required: true") {
		t.Fatalf("an optional key's question is not required:\n%s", y)
	}
	m.Intake[0].Required = false
	m.RequiredSecrets = []string{"API_TOKEN"}
	if _, y := saved(t, m); !strings.Contains(y, "required: true") {
		t.Fatalf("a required key's question is required:\n%s", y)
	}
}

func TestADraftsOptionalSecretsComeIn(t *testing.T) {
	draft := strings.Replace(draftManifest, `"secrets": ["SLACK_TOKEN"], "dataAccess"`, `"secrets": ["SLACK_TOKEN", "OTHER_TOKEN"], "optionalSecrets": ["OTHER_TOKEN"], "dataAccess"`, 1)
	if draft == draftManifest {
		t.Fatal("the fixture's secrets line moved")
	}
	m, notes, err := FromDraft([]byte(draft), "# x\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.RequiredSecrets, []string{"SLACK_TOKEN"}) || !slices.Equal(m.OptionalSecrets(), []string{"OTHER_TOKEN"}) {
		t.Fatalf("required %v optional %v", m.RequiredSecrets, m.OptionalSecrets())
	}
	for _, n := range notes {
		if strings.Contains(n, "optionalSecrets") {
			t.Fatalf("optionalSecrets was dropped from the draft: %q", notes)
		}
	}
}

// A draft's optionalSecrets naming a key it does not declare marks nothing:
// the model holds the declared keys alone, so the name is never written.
func TestADraftsUndeclaredOptionalNameIsDropped(t *testing.T) {
	draft := strings.Replace(draftManifest, `"secrets": ["SLACK_TOKEN"], "dataAccess"`, `"secrets": ["SLACK_TOKEN"], "optionalSecrets": ["NOT_DECLARED"], "dataAccess"`, 1)
	m, _, err := FromDraft([]byte(draft), "# x\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.RequiredSecrets, []string{"SLACK_TOKEN"}) || len(m.OptionalSecrets()) != 0 {
		t.Fatalf("required %v optional %v", m.RequiredSecrets, m.OptionalSecrets())
	}
}
