package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	corejudge "github.com/blaktron/agentry/pkg/judge"
	"github.com/blaktron/agentry/pkg/manifest"
	corewebsearch "github.com/blaktron/agentry/pkg/websearch"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
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

// A listing's plain-http warning reaches the page (agentry-desktop#317), and
// the binding the form asks as the URL is typed says the same sentence.
func TestListModelsAndEndpointWarningCarryThePlainHTTPWarning(t *testing.T) {
	run.SetListModelsForTest(t, func(_ context.Context, provider, baseURL, key string) (*setup.Catalog, error) {
		return &setup.Catalog{Provider: provider, Models: []setup.Model{{ID: "m"}}, Warning: setup.PlainHTTPWarning(baseURL)}, nil
	})
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.ListModels("custom", "http://gpu-box.lan:8000/v1", "k")
	if err != nil || !strings.Contains(v.Warning, "plain http") {
		t.Fatalf("view = %+v, %v", v, err)
	}
	if w := r.EndpointWarning("http://gpu-box.lan:8000/v1"); w != v.Warning {
		t.Fatalf("EndpointWarning = %q, listing warning = %q", w, v.Warning)
	}
	if w := r.EndpointWarning("https://llm.example.net/v1"); w != "" {
		t.Fatalf("https warned: %q", w)
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

// The person's own web search key (#301), as the AI Review key is held:
// StoreSearchKey is the one call that carries it inward; no view
// StoreSearchKey, SearchKeyStatus, TestSearchKey or ForgetSearchKey returns
// carries it back, and no progress line the page is sent does either, even
// when the key replaces another provider's.
func TestTheSearchKeyIsNeverShownBackOrLogged(t *testing.T) {
	const secret = "search-never-shown-back-8e41"
	t.Setenv("HOME", t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	for _, p := range run.SearchProviders() {
		t.Setenv(p.EnvName, "")
	}
	var events []string
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) { events = append(events, fmt.Sprint(name, data)) }
	t.Cleanup(func() { emitEvent = emit })

	r := &RunAPI{app: &App{ctx: context.Background()}}
	views := []any{}
	if _, err := r.StoreSearchKey("brave", "brave-"+secret); err != nil {
		t.Fatal(err)
	}
	v, err := r.StoreSearchKey("exa", secret)
	if err != nil || !v.Stored || !v.Ready || v.Provider != "exa" || v.ProviderLabel != "Exa" || v.Source != "keyring" ||
		v.EnvName != "EXA_API_KEY" || len(v.Providers) < 5 || v.Providers[0].Name != "brave" {
		t.Fatalf("store: %+v %v", v, err)
	}
	views = append(views, v)
	s, err := r.SearchKeyStatus()
	if err != nil {
		t.Fatal(err)
	}
	views = append(views, s)
	run.SetAskSearchForTest(t, func(_ context.Context, provider, key string) error {
		return &corewebsearch.StatusError{Provider: provider, Status: 401, Snippet: "bad key " + key}
	})
	tv, err := r.TestSearchKey()
	if err != nil || tv.OK || !strings.Contains(tv.Message, "Exa refused the key (401)") || tv.From != "the keychain" {
		t.Fatalf("test: %+v %v", tv, err)
	}
	views = append(views, tv)
	f, err := r.ForgetSearchKey()
	if err != nil || f.Stored || f.Ready || f.Provider != "exa" {
		t.Fatalf("forget: %+v %v", f, err)
	}
	views = append(views, f)
	for _, v := range views {
		if b, _ := json.Marshal(v); strings.Contains(string(b), secret) {
			t.Fatalf("a view carries the key: %s", b)
		}
	}
	if len(events) == 0 {
		t.Fatal("no progress line was sent: the check below would prove nothing")
	}
	for _, e := range events {
		if strings.Contains(e, secret) {
			t.Fatalf("a progress line carries the key: %s", e)
		}
	}
}

// The person's own AI Review key (#282, plan builder-rework.md D25), as the
// model key is held: StoreJevKey is the one call that carries it inward;
// no view StoreJevKey, JevKeyStatus, TestJevKey or ForgetJevKey returns
// carries it back, and no progress line the page is sent does either.
func TestTheJevKeyIsNeverShownBackOrLogged(t *testing.T) {
	const secret = "jev-never-shown-back-3b9e"
	t.Setenv("HOME", t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	t.Setenv(policy.DefaultJudgeKeyEnv, "")
	var events []string
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) { events = append(events, fmt.Sprint(name, data)) }
	t.Cleanup(func() { emitEvent = emit })

	r := &RunAPI{app: &App{ctx: context.Background()}}
	views := []any{}
	v, err := r.StoreJevKey(secret)
	if err != nil || !v.Stored || !v.Ready || v.Source != "keyring" || v.EnvName != policy.DefaultJudgeKeyEnv {
		t.Fatalf("store: %+v %v", v, err)
	}
	views = append(views, v)
	s, err := r.JevKeyStatus()
	if err != nil {
		t.Fatal(err)
	}
	views = append(views, s)
	run.SetAskJevForTest(t, func(_ context.Context, key string) error {
		return &corejudge.StatusError{Status: 401, Snippet: "bad key " + key}
	})
	tv, err := r.TestJevKey()
	if err != nil || tv.OK || !strings.Contains(tv.Message, "(401)") || tv.From != "the keychain" {
		t.Fatalf("test: %+v %v", tv, err)
	}
	views = append(views, tv)
	f, err := r.ForgetJevKey()
	if err != nil || f.Stored || f.Ready {
		t.Fatalf("forget: %+v %v", f, err)
	}
	views = append(views, f)
	for _, v := range views {
		if b, _ := json.Marshal(v); strings.Contains(string(b), secret) {
			t.Fatalf("a view carries the key: %s", b)
		}
	}
	if len(events) == 0 {
		t.Fatal("no progress line was sent: the check below would prove nothing")
	}
	for _, e := range events {
		if strings.Contains(e, secret) {
			t.Fatalf("a progress line carries the key: %s", e)
		}
	}
}
