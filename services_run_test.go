package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/setup"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// RunAPI.ListModels (agentry-desktop#131) carries the key inward and
// returns names: the view holds the ids, the display names, the kind and
// the sentence, never the key — and an error from the listing reaches the
// page as the provider's words with the key absent (setup.ListModels
// scrubs it; this surface adds nothing that could carry it).
func TestListModelsReturnsNamesNeverTheKey(t *testing.T) {
	var got []string
	run.SetListModelsForTest(t, func(_ context.Context, provider, baseURL, key string) (*setup.Catalog, error) {
		got = append(got, provider+"|"+baseURL+"|"+key)
		if key == "wrong-secret" {
			// Echoes the key on purpose: the surface must not pass it on.
			return nil, errors.New("custom: list models: 401 Unauthorized: wrong-secret rejected")
		}
		return &setup.Catalog{Provider: provider, Models: []setup.Model{{ID: "qwen3.8-flash"}, {ID: "glm-5.2", DisplayName: "GLM 5.2"}}}, nil
	})
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.ListModels("custom", "https://llm.example.net/v1", "typed-secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Models) != 2 || v.Models[0].ID != "qwen3.8-flash" || v.Models[1].DisplayName != "GLM 5.2" || v.Kind != "api" || v.Notice != "" || v.Provider != "custom" {
		t.Fatalf("view = %+v", v)
	}
	if got[0] != "custom|https://llm.example.net/v1|typed-secret" {
		t.Fatalf("asked = %v", got)
	}
	_, err = r.ListModels("custom", "https://llm.example.net/v1", "wrong-secret")
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("error = %v", err)
	}
	v, err = r.ListModels("anthropic", "", "sk-ant-oat01-typed")
	if err != nil || v.Kind != "subscription" || !strings.Contains(v.Notice, "Claude Code only") {
		t.Fatalf("subscription: %+v %v", v, err)
	}
}

// The setup view carries which providers have a stored key (desktop#141)
// as names only — the badge's fact for whichever provider the form shows;
// the policy-provider boolean it carried is not on the wire any more. An
// empty list is a list, not an absent field, so the page's includes() has
// something to ask.
func TestSetupStatusViewListsStoredKeyProviders(t *testing.T) {
	v := setupStatusView(&run.Status{Provider: "openai", KeysStored: []string{"anthropic", "custom"}})
	if strings.Join(v.KeysStored, ",") != "anthropic,custom" {
		t.Fatalf("view = %+v", v)
	}
	if v := setupStatusView(&run.Status{Provider: "anthropic"}); v.KeysStored == nil || len(v.KeysStored) != 0 {
		t.Fatalf("an empty list must still be a list: %+v", v.KeysStored)
	}
}

// TestProfileStatusViewCarriesTheGuide (agentry-desktop#228): the run
// setup's view hands the page every guide field the author wrote, with
// arrays and maps never null, and no guide at all when the release has
// none. Profile values never appear: the view has names only.
func TestProfileStatusViewCarriesTheGuide(t *testing.T) {
	v := profileStatusView(&run.ProfileInfo{
		Ref:             "acme/reporter@1.0.0",
		DeclaredSecrets: []string{"RUN_TEST_TOKEN"},
		Guide: &manifest.Guide{
			Overview: "Writes a **report**.",
			Files:    "Your notes.",
			Task:     "e.g. the weekly status",
			Connect:  map[string]string{"RUN_TEST_TOKEN": "Create one at https://api.example.com/tokens."},
		},
	})
	g := v.Guide
	if g == nil {
		t.Fatal("the view dropped the guide")
	}
	if g.Overview != "Writes a **report**." || g.Files != "Your notes." || g.Task != "e.g. the weekly status" {
		t.Fatalf("guide view = %+v", g)
	}
	if g.Prepare == nil || len(g.Prepare) != 0 {
		t.Fatalf("an absent prepare list must be an empty array, got %#v", g.Prepare)
	}
	if g.Connect["RUN_TEST_TOKEN"] != "Create one at https://api.example.com/tokens." {
		t.Fatalf("connect = %v", g.Connect)
	}

	plain := profileStatusView(&run.ProfileInfo{Ref: "acme/reporter@1.0.0"})
	if plain.Guide != nil {
		t.Fatalf("no guide in the manifest, yet the view has one: %+v", plain.Guide)
	}
	if noConnect := profileStatusView(&run.ProfileInfo{Guide: &manifest.Guide{Overview: "x"}}); noConnect.Guide.Connect == nil {
		t.Fatal("an absent connect map must be an empty object")
	}
}
