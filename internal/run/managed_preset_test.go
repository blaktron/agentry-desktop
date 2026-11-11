package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// holdPreset holds a document of acme's assigning acme/demo with a preset
// model, locked or not.
func holdPreset(t *testing.T, provider, model string, locked bool) {
	t.Helper()
	raw, err := json.Marshal([]managedsettings.Assignment{{Ref: "acme/demo", Version: "latest", Mode: managedsettings.AssignOffered,
		Preset: &managedsettings.Preset{Model: &managedsettings.Model{Provider: provider, Model: model}}, PresetLocked: locked}})
	if err != nil {
		t.Fatal(err)
	}
	doc := managedsettings.Document{Schema: managedsettings.Schema, Registry: "https://reg.example", Org: "acme", Version: 4,
		IssuedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		Settings: map[string]managedsettings.Setting{managedsettings.QuickAgents: {Value: raw, Locked: true}}}
	was := heldManaged
	t.Cleanup(func() { heldManaged = was })
	heldManaged = func() managed.Effective {
		return managed.Effective{Documents: []managed.State{{Registry: doc.Registry, Document: doc}}}
	}
}

// presetPolicy is the default policy with a second saved model, GPT, on
// openai's gpt-5.5.
func presetPolicy(t *testing.T) (*policy.Policy, policy.SavedModel) {
	t.Helper()
	pol := policy.Default()
	if err := pol.AddProvider("openai", policy.Provider{APIKeyEnv: "OPENAI_API_KEY", ProxyAllow: []string{"api.openai.com:443"}}); err != nil {
		t.Fatal(err)
	}
	gpt, err := pol.AddModel(policy.SavedModel{Name: "GPT", Provider: "openai", Model: "gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	return pol, gpt
}

// A QuickAgent assigned with a preset model runs on the saved model that is
// it when nothing is picked; a locked preset refuses another pick, and
// refuses a run when no saved model is its model, in the organization's
// name; a default one leaves the pick with a warning.
func TestAPresetModelIsTheRunsModel(t *testing.T) {
	pol, gpt := presetPolicy(t)
	var w bytes.Buffer
	holdPreset(t, "openai", "gpt-5.5", false)
	if got, err := presetModel(pol, "acme/demo@1.0.0", "", &w); err != nil || got != gpt.ID {
		t.Fatalf("no pick: %q, %v", got, err)
	}
	def := pol.DefaultModel().Name
	if got, err := presetModel(pol, "acme/demo", def, &w); err != nil || got != def {
		t.Fatalf("a pick under a default preset: %q, %v", got, err)
	}
	if got, err := presetModel(pol, "acme/other", "", &w); err != nil || got != "" {
		t.Fatalf("a release with no assignment: %q, %v", got, err)
	}

	holdPreset(t, "openai", "gpt-5.5", true)
	_, err := presetModel(pol, "acme/demo", def, &w)
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), "acme's managed settings, which fix the model of acme/demo at openai's gpt-5.5 (quickagents)") ||
		!strings.Contains(err.Error(), "Model list") {
		t.Fatalf("another pick under a locked preset: %v", err)
	}

	holdPreset(t, "openai", "gpt-4", true)
	if _, err := presetModel(pol, "acme/demo", "", &w); err == nil || !strings.Contains(err.Error(), "no saved model on this computer is that model: save it in Settings → Models and keys first") {
		t.Fatalf("a locked preset model with no saved model: %v", err)
	}
	if w.Len() != 0 {
		t.Fatalf("warned %q", w.String())
	}
	holdPreset(t, "openai", "gpt-4", false)
	if got, err := presetModel(pol, "acme/demo", "", &w); err != nil || got != "" || !strings.Contains(w.String(), "warning: acme's managed settings suggest openai's gpt-4 for acme/demo") {
		t.Fatalf("a default preset model with no saved model: %q, %v, %q", got, err, w.String())
	}
}

// CreditsPay opens an operation on the registry with this app's sign-in
// there, says what it cost, and never says its token.
func TestCreditsPayOpensAnOperationWithTheSignIn(t *testing.T) {
	isolateRunEnv(t)
	keyring.MockInit()
	var auth, purpose string
	credits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/x/review/operations" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		var body struct{ Purpose string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		purpose = body.Purpose
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "agyr_desktop", "credits": credits,
			"operation": map[string]any{"id": "op-1", "purpose": body.Purpose, "expiresAt": time.Now().Add(2 * time.Hour)}})
	}))
	defer srv.Close()
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = srv.URL
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	wasClient := httpClientForTest(srv.Client())
	t.Cleanup(wasClient)
	if _, err := settings.StoreToken(srv.URL, "agy_fakesigninforthedesktoptestonly00000000000000000"); err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	op, err := CreditsPay(&w)(context.Background(), srv.URL, managed.PurposeRun)
	if err != nil {
		t.Fatal(err)
	}
	if op.Token != "agyr_desktop" || purpose != managed.PurposeRun || !strings.HasPrefix(auth, "Bearer agy_") {
		t.Fatalf("op %+v, purpose %q, auth %q", op, purpose, auth[:min(len(auth), 10)])
	}
	if got := w.String(); !strings.Contains(got, "an operation on "+srv.URL+", with no credit charged.") || strings.Contains(got, "agyr_") {
		t.Fatalf("said %q", got)
	}
	credits = 1
	w.Reset()
	if _, err := CreditsPay(&w)(context.Background(), srv.URL, managed.PurposeSearch); err != nil {
		t.Fatal(err)
	}
	if got := w.String(); !strings.Contains(got, "Web search, with AI Review reading every result, is paid") || !strings.Contains(got, "1 credit, spent whether or not the work finishes.") {
		t.Fatalf("said %q", got)
	}
}

// httpClientForTest has operations opened with c, and returns the undo.
func httpClientForTest(c *http.Client) func() {
	was := operationsClient
	operationsClient = func() *http.Client { return c }
	return func() { operationsClient = was }
}

// Run setup shows the saved model a preset's model is on this computer, or
// none, and the managed model rows' refusal of the preset's model itself.
func TestRunSetupSeesThePresetModelHere(t *testing.T) {
	pol, gpt := presetPolicy(t)
	holdPreset(t, "openai", "gpt-5.5", true)
	if got := presetModelOn(pol, "openai", "gpt-5.5"); got.SavedID != gpt.ID || got.SavedName != "GPT" || got.Refusal != "" {
		t.Fatalf("a saved model of it: %+v", got)
	}
	if got := presetModelOn(pol, "openai", "gpt-6"); got.SavedID != "" || got.Refusal != "" {
		t.Fatalf("no saved model of it: %+v", got)
	}
	if got := presetModelOn(nil, "openai", "gpt-5.5"); got.SavedID != "" {
		t.Fatalf("no policy read: %+v", got)
	}

	raw, _ := json.Marshal([]managedsettings.Assignment{{Ref: "acme/demo", Version: "latest", Mode: managedsettings.AssignOffered,
		Preset: &managedsettings.Preset{Model: &managedsettings.Model{Provider: "openai", Model: "gpt-5.5"}}, PresetLocked: true}})
	doc := managedsettings.Document{Schema: managedsettings.Schema, Registry: "https://reg.example", Org: "acme", Version: 5,
		IssuedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		Settings: map[string]managedsettings.Setting{
			managedsettings.QuickAgents:    {Value: raw, Locked: true},
			managedsettings.ModelProviders: {Value: json.RawMessage(`["anthropic"]`), Locked: true},
		}}
	heldManaged = func() managed.Effective {
		return managed.Effective{Documents: []managed.State{{Registry: doc.Registry, Document: doc}}}
	}
	if got := presetModelOn(pol, "openai", "gpt-5.5"); got.SavedID != "" || !strings.Contains(got.Refusal, "acme") {
		t.Fatalf("a preset model the model rows refuse: %+v", got)
	}
}
