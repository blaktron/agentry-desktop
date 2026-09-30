package run

// The person's own web search key (#301): a provider the runner policy
// admits, its key kept in the OS keychain as provider:<provider>, the
// provider and the source recorded in the policy, shown as saved and never
// as its value, never logged, and tested with one search whose answer is
// said in plain words. A key saved for one provider never serves another.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	corewebsearch "github.com/blaktron/agentry/pkg/websearch"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

const testSearchKey = "search-the-persons-own-key-5d2a"

// searchKeychain gives the test an in-memory OS keychain under the real
// modelkey code, and no provider's key in the environment.
func searchKeychain(t *testing.T) {
	t.Helper()
	isolateRunEnv(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	for _, p := range SearchProviders() {
		t.Setenv(p.EnvName, "")
	}
}

func searchPolicy(t *testing.T) *policy.Policy {
	t.Helper()
	path, _ := policy.DefaultPath()
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

func TestSearchProvidersAreThePolicysBraveFirst(t *testing.T) {
	ps := SearchProviders()
	if len(ps) != len(policy.WebSearchProviders()) || ps[0].Name != "brave" || ps[0].Label != "Brave" || ps[0].EnvName != "BRAVE_API_KEY" {
		t.Fatalf("%+v", ps)
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Name] || p.Label == "" || p.EnvName == "" {
			t.Fatalf("%+v", ps)
		}
		seen[p.Name] = true
		if _, err := corewebsearch.New(p.Name, "", "k"); err != nil {
			t.Fatalf("the core does not build %s: %v", p.Name, err)
		}
	}
	for _, want := range []string{"brave", "tavily", "exa", "perplexity", "youcom"} {
		if !seen[want] {
			t.Fatalf("%s is not offered: %+v", want, ps)
		}
	}
}

func TestStoreSearchKeyKeepsItInTheKeychainAndRecordsTheProvider(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := StoreSearchKey("tavily", "  "+testSearchKey+"\n", &log); err != nil {
		t.Fatal(err)
	}
	got, err := modelkey.Lookup("tavily")
	if err != nil || got != testSearchKey {
		t.Fatalf("keychain holds %q (%v); want the trimmed key", got, err)
	}
	pol := searchPolicy(t)
	if pol.WebSearch != (policy.WebSearchKey{Provider: "tavily", APIKeySource: modelkey.SourceKeyring}) {
		t.Fatalf("policy webSearch %+v", pol.WebSearch)
	}
	path, _ := policy.DefaultPath()
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), testSearchKey) || strings.Contains(log.String(), testSearchKey) {
		t.Fatal("the key reached the policy file or the log")
	}
	st, err := GetSearchKeyStatus()
	if err != nil || !st.Stored || !st.Ready() || st.Provider != "tavily" || st.ProviderLabel != "Tavily" || st.EnvName != "TAVILY_API_KEY" {
		t.Fatalf("status %+v %v", st, err)
	}
	noSearchValue(t, "the status", st)
}

func TestStoreSearchKeyRefusesAProviderThePolicyDoesNotAdmitOrNoKey(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	for _, p := range []string{"", "none", "bing", "Brave"} {
		if err := StoreSearchKey(p, testSearchKey, &log); err == nil || !strings.Contains(err.Error(), "choose a search provider") {
			t.Fatalf("%q: %v", p, err)
		}
	}
	if err := StoreSearchKey("exa", "   ", &log); err == nil || !strings.Contains(err.Error(), "paste your Exa key") {
		t.Fatalf("no key: %v", err)
	}
	if pol, _ := loadPolicyIfPresent(); pol != nil {
		t.Fatal("a refused save wrote a policy")
	}
}

func TestStoreSearchKeyRefusedByTheKeychainLeavesThePolicy(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := StoreSearchKey("brave", testSearchKey, &log); err != nil {
		t.Fatal(err)
	}
	before := searchPolicy(t).WebSearch
	old := keychainStore
	keychainStore = func(string, string) error { return errors.New("the keychain is locked") }
	t.Cleanup(func() { keychainStore = old })
	if err := StoreSearchKey("exa", "another-key", &log); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("err = %v", err)
	}
	if after := searchPolicy(t).WebSearch; after != before {
		t.Fatalf("policy moved on a refused keychain: %+v → %+v", before, after)
	}
}

// Changing provider moves the policy to the new one, drops a variable named
// for the old one and removes the old one's keychain entry, so neither key
// can serve the other provider.
func TestChangingProviderNeverLetsOneKeyServeAnother(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := StoreSearchKey("brave", "brave-key-1", &log); err != nil {
		t.Fatal(err)
	}
	path, _ := policy.DefaultPath()
	pol := searchPolicy(t)
	pol.WebSearch.APIKeyEnv = "MY_BRAVE"
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	if err := StoreSearchKey("brave", "brave-key-2", &log); err != nil {
		t.Fatal(err)
	}
	if got := searchPolicy(t).WebSearch.APIKeyEnv; got != "MY_BRAVE" {
		t.Fatalf("a re-save for the same provider dropped its variable: %q", got)
	}
	if err := StoreSearchKey("perplexity", testSearchKey, &log); err != nil {
		t.Fatal(err)
	}
	if w := searchPolicy(t).WebSearch; w != (policy.WebSearchKey{Provider: "perplexity", APIKeySource: modelkey.SourceKeyring}) {
		t.Fatalf("policy %+v", w)
	}
	if ok, _ := modelkey.Present("brave"); ok {
		t.Fatal("the replaced provider's key is still in the keychain")
	}
	if !strings.Contains(log.String(), "Brave key it replaces was removed") {
		t.Fatalf("log: %s", log.String())
	}
	path, _ = policy.DefaultPath()
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), testSearchKey) || strings.Contains(string(raw), "brave-key") {
		t.Fatal("a key reached the policy file on a switch")
	}
	t.Setenv("BRAVE_API_KEY", "brave-from-the-variable")
	var asked []string
	SetAskSearchForTest(t, func(_ context.Context, provider, key string) error {
		asked = append(asked, provider+":"+key)
		return nil
	})
	if _, err := TestSearchKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "perplexity:"+testSearchKey {
		t.Fatalf("asked %v", asked)
	}
}

func TestForgetSearchKeyRemovesTheEntryAndTheProvider(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := ForgetSearchKey(&log); err != nil {
		t.Fatalf("forget with nothing saved: %v", err)
	}
	if err := StoreSearchKey("exa", testSearchKey, &log); err != nil {
		t.Fatal(err)
	}
	if err := ForgetSearchKey(&log); err != nil {
		t.Fatal(err)
	}
	if ok, _ := modelkey.Present("exa"); ok {
		t.Fatal("the key is still in the keychain")
	}
	if w := searchPolicy(t).WebSearch; w != (policy.WebSearchKey{Provider: "exa"}) {
		t.Fatalf("policy webSearch %+v; want the provider kept and the keychain source gone", w)
	}
	st, err := GetSearchKeyStatus()
	if err != nil || st.Provider != "exa" || st.Stored || st.Ready() {
		t.Fatalf("status %+v %v", st, err)
	}
	// The provider stays chosen, so its variable still serves a draft.
	t.Setenv("EXA_API_KEY", "exa-from-the-variable")
	if st, _ := GetSearchKeyStatus(); !st.Ready() {
		t.Fatalf("the variable no longer serves: %+v", st)
	}
}

// A keychain that refuses the delete leaves the policy as it was, so the
// policy never stops reading a key the keychain still holds.
func TestForgetSearchKeyRefusedByTheKeychainLeavesThePolicy(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := StoreSearchKey("tavily", testSearchKey, &log); err != nil {
		t.Fatal(err)
	}
	before := searchPolicy(t).WebSearch
	old := keychainDelete
	keychainDelete = func(string) error { return errors.New("the keychain is locked") }
	t.Cleanup(func() { keychainDelete = old })
	if err := ForgetSearchKey(&log); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("err = %v", err)
	}
	if after := searchPolicy(t).WebSearch; after != before {
		t.Fatalf("policy moved on a refused delete: %+v → %+v", before, after)
	}
}

func TestSearchKeyStatusSaysTheVariableGoesFirst(t *testing.T) {
	searchKeychain(t)
	st, err := GetSearchKeyStatus()
	if err != nil || st.Provider != "" || st.Ready() {
		t.Fatalf("no policy: %+v %v", st, err)
	}
	path, _ := policy.DefaultPath()
	pol := policy.Default()
	pol.WebSearch = policy.WebSearchKey{Provider: "exa"}
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	if st, _ := GetSearchKeyStatus(); st.Ready() || st.EnvName != "EXA_API_KEY" {
		t.Fatalf("no key: %+v", st)
	}
	t.Setenv("EXA_API_KEY", "exa-from-the-variable")
	if st, _ := GetSearchKeyStatus(); !st.Ready() || !st.EnvSet || st.Stored {
		t.Fatalf("variable: %+v", st)
	}
}

func TestTestSearchKeySaysTheProvidersAnswer(t *testing.T) {
	searchKeychain(t)
	var log strings.Builder
	if err := StoreSearchKey("tavily", testSearchKey, &log); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		err  error
		ok   bool
		want string
	}{
		{"works", nil, true, "The key works: Tavily answered a search"},
		{"401", &corewebsearch.StatusError{Provider: "tavily", Status: 401, Snippet: "bad " + testSearchKey}, false, "Tavily refused the key (401)"},
		{"401 with a body of its own", &corewebsearch.StatusError{Provider: "tavily", Status: 401, Snippet: `{"detail":"PROVIDER-BODY-ECHO"}`}, false, "Tavily refused the key (401)"},
		{"403", &corewebsearch.StatusError{Status: 403}, false, "Tavily refused the key (403)"},
		{"432", &corewebsearch.StatusError{Status: 432}, false, "plan or credit has run out"},
		{"402", &corewebsearch.StatusError{Status: 402}, false, "plan or credit has run out"},
		{"429", &corewebsearch.StatusError{Status: 429}, false, "limiting requests with this key right now (429)"},
		{"500", &corewebsearch.StatusError{Status: 500}, false, "Tavily answered 500"},
		{"unreachable", fmt.Errorf("websearch: tavily request: %w", &url.Error{Op: "Post", URL: "https://api.tavily.com/search",
			Err: errors.New("dial tcp: lookup api.tavily.com: no such host")}), false, "Tavily could not be reached"},
		{"timeout", fmt.Errorf("websearch: tavily request: %w", context.DeadlineExceeded), false, "Tavily did not answer in time"},
		{"odd answer", errors.New("websearch: tavily response is not the result shape " + testSearchKey), false, "not one a draft can use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var asked []string
			SetAskSearchForTest(t, func(_ context.Context, provider, key string) error {
				asked = append(asked, provider+":"+key)
				return c.err
			})
			res, err := TestSearchKey(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(asked) != 1 || asked[0] != "tavily:"+testSearchKey {
				t.Fatalf("asked %v", asked)
			}
			if res.OK != c.ok || !strings.Contains(res.Message, c.want) || res.From != "the keychain" || strings.Contains(res.Message, "api.tavily.com/search") ||
				strings.Contains(res.Message, "PROVIDER-BODY-ECHO") {
				t.Fatalf("result %+v", res)
			}
			noSearchValue(t, "the result", res)
		})
	}

	// The variable goes first, as it does for a draft.
	t.Setenv("TAVILY_API_KEY", "tvly-from-the-variable")
	var asked string
	SetAskSearchForTest(t, func(_ context.Context, _, key string) error { asked = key; return nil })
	res, err := TestSearchKey(context.Background())
	if err != nil || asked != "tvly-from-the-variable" || res.From != "$TAVILY_API_KEY" {
		t.Fatalf("%+v %v asked=%q", res, err, asked)
	}

	// No provider: nothing is asked, and it says where to choose one.
	t.Setenv("TAVILY_API_KEY", "")
	if err := ForgetSearchKey(&log); err != nil {
		t.Fatal(err)
	}
	SetAskSearchForTest(t, func(context.Context, string, string) error { t.Fatal("asked with no key"); return nil })
	res, err = TestSearchKey(context.Background())
	if err != nil || res.OK || !strings.Contains(res.Message, "Web search") {
		t.Fatalf("%+v %v", res, err)
	}
}

// noSearchValue fails when the key appears anywhere in what a surface
// returned.
func noSearchValue(t *testing.T, what string, v any) {
	t.Helper()
	if s := fmt.Sprintf("%+v", v); strings.Contains(s, testSearchKey) {
		t.Fatalf("%s carries the key: %s", what, s)
	}
}
