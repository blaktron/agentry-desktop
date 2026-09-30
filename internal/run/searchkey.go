// The person's own web search key (plan builder-rework.md §7.3; #301): the
// key of the search provider they choose, which Describe it, on this
// computer searches the web with. It is kept as the AI Review key is: the OS
// keychain entry provider:<provider> under the CLI's service, and the runner
// policy's webSearch block records the provider and that the keychain is its
// source, so `agentry draft` and this app read the same key the same way
// (pkg/drafter's SearchKey). The providers are the CLI's
// (policy.WebSearchProviders), so the app offers exactly what the tools
// server can search with. The key crosses the app's IPC once, inward, in a
// call of its own, and no surface returns it.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	corewebsearch "github.com/blaktron/agentry/pkg/websearch"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// SearchProvider is one provider a person may choose: its name as the
// policy says it, as a person knows it, and the variable a draft reads its
// key from first.
type SearchProvider struct {
	Name    string
	Label   string
	EnvName string
}

// SearchProviders is every provider the runner policy admits, Brave first
// (the registry's own, DEV-157), the rest in the policy's order.
func SearchProviders() []SearchProvider {
	names := policy.WebSearchProviders()
	if i := slices.Index(names, corewebsearch.BraveName); i > 0 {
		names = append([]string{corewebsearch.BraveName}, slices.Delete(names, i, i+1)...)
	}
	out := make([]SearchProvider, 0, len(names))
	for _, n := range names {
		k := policy.WebSearchKey{Provider: n}
		out = append(out, SearchProvider{Name: n, Label: k.ProviderName(), EnvName: k.KeyEnv()})
	}
	return out
}

// SearchKeyStatus is where the person's search key stands on this machine.
// It never carries the key.
type SearchKeyStatus struct {
	// Provider is the policy's webSearch.provider ("" when none), and
	// ProviderLabel its name as a person knows it.
	Provider      string
	ProviderLabel string
	// Stored: the keychain holds provider:<Provider> (read from its
	// presence marker, never the key).
	Stored bool
	// Source is the policy's webSearch.apiKeySource.
	Source string
	// EnvName is the variable a draft reads first; EnvSet says it is set in
	// this app's environment.
	EnvName string
	EnvSet  bool
	// KeyStoreErr is why the keychain could not be asked.
	KeyStoreErr string
}

// Ready reports whether a draft on this computer would search: a provider
// named, and its key in the variable or, for a keyring source, the keychain.
// It is pkg/drafter's rule, answered from presence alone.
func (s *SearchKeyStatus) Ready() bool {
	return s.Provider != "" && (s.EnvSet || (s.Stored && s.Source == modelkey.SourceKeyring))
}

// GetSearchKeyStatus reads the search key's standing without writing
// anything and without reading the key.
func GetSearchKeyStatus() (*SearchKeyStatus, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	w := pol.WebSearch
	if !w.On() {
		return &SearchKeyStatus{}, nil
	}
	st := &SearchKeyStatus{Provider: w.Provider, ProviderLabel: w.ProviderName(), Source: w.APIKeySource, EnvName: w.KeyEnv()}
	st.EnvSet = strings.TrimSpace(os.Getenv(st.EnvName)) != ""
	stored, err := keychainPresent(w.Provider)
	if err != nil {
		st.KeyStoreErr = err.Error()
	}
	st.Stored = stored
	return st, nil
}

// StoreSearchKey puts the key into the OS keychain as provider:<provider>,
// then records the provider and the keychain as its source in the runner
// policy. The keychain goes first: a store that fails leaves the policy as
// it was (D-024's rule for the model key). A variable the policy named for
// another provider is dropped, since it held that provider's key; and the
// keychain entry of the provider this one replaces is removed, so no key is
// kept for a provider nothing reads.
func StoreSearchKey(provider, key string, logw io.Writer) error {
	provider = strings.TrimSpace(provider)
	if !slices.Contains(policy.WebSearchProviders(), provider) {
		return fmt.Errorf("choose a search provider: %s", strings.Join(policy.WebSearchProviders(), ", "))
	}
	label := policy.WebSearchKey{Provider: provider}.ProviderName()
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("paste your %s key first", label)
	}
	if err := keychainStore(provider, key); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "web search key stored in the OS keychain as %s\n", modelkey.Account(provider))
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	old := pol.WebSearch
	pol.WebSearch = policy.WebSearchKey{Provider: provider, APIKeySource: modelkey.SourceKeyring}
	if old.Provider == provider {
		pol.WebSearch.APIKeyEnv = old.APIKeyEnv
	}
	if err := policy.Save(path, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s (webSearch.provider: %s, apiKeySource: keyring)\n", path, provider)
	if old.On() && old.Provider != provider && old.APIKeySource == modelkey.SourceKeyring {
		if err := keychainDelete(old.Provider); err != nil {
			_, _ = fmt.Fprintf(logw, "the %s key could not be removed from the keychain: %v\n", old.ProviderName(), err)
		} else {
			_, _ = fmt.Fprintf(logw, "the %s key it replaces was removed from the OS keychain (%s)\n", old.ProviderName(), modelkey.Account(old.Provider))
		}
	}
	return nil
}

// ForgetSearchKey removes the chosen provider's key from the keychain, and
// the provider from the policy, so a draft on this computer reads the
// registry and the pages it knows rather than look for a key that is gone.
func ForgetSearchKey(logw io.Writer) error {
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil || !pol.WebSearch.On() {
		return err
	}
	provider := pol.WebSearch.Provider
	if err := keychainDelete(provider); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "web search key removed from the OS keychain (%s)\n", modelkey.Account(provider))
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol.WebSearch = policy.WebSearchKey{}
	return policy.Save(path, pol)
}

// SearchKeyTest is what Test key found: whether the key worked, the
// sentence that says so, and which key it was (the keychain's, or the
// variable's).
type SearchKeyTest struct {
	OK      bool
	Message string
	From    string
}

// askSearch makes Test key's one small call: one search for one result,
// through the core's client, the one the tools server builds. A seam: the
// tests have no provider.
var askSearch = func(ctx context.Context, provider, key string) error {
	p, err := corewebsearch.New(provider, "", key)
	if err != nil {
		return err
	}
	_, err = p.Search(ctx, "Agentry", 1)
	return err
}

// SetAskSearchForTest points Test key's call at a stand-in for the test's
// duration (the bound surface's tests have no provider to ask).
func SetAskSearchForTest(t interface{ Cleanup(func()) }, f func(context.Context, string, string) error) {
	old := askSearch
	askSearch = f
	t.Cleanup(func() { askSearch = old })
}

// TestSearchKey tries the key a draft on this computer would search with
// (pkg/drafter's SearchKey: the variable first, then the keychain) with one
// search, and says the provider's answer in plain words. Only a local
// failure is an error; the provider's refusal is an answer.
func TestSearchKey(ctx context.Context) (*SearchKeyTest, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	key, state, err := drafter.SearchKey(pol)
	if err != nil {
		return nil, err
	}
	switch state {
	case drafter.SearchNoProvider:
		return &SearchKeyTest{Message: "No search provider is chosen on this computer. Choose one under Settings → Provider settings → Web search, paste its key, then test it."}, nil
	case drafter.SearchNoKey:
		return &SearchKeyTest{Message: fmt.Sprintf("No %s key is saved on this computer. Paste it under Settings → Provider settings → Web search, then test it.", pol.WebSearch.ProviderName())}, nil
	}
	res := &SearchKeyTest{From: "the keychain"}
	if strings.TrimSpace(os.Getenv(pol.WebSearch.KeyEnv())) != "" {
		res.From = "$" + pol.WebSearch.KeyEnv()
	}
	res.OK, res.Message = saySearchAnswer(pol.WebSearch.ProviderName(), askSearch(ctx, pol.WebSearch.Provider, key))
	res.Message = strings.ReplaceAll(res.Message, key, "[key]")
	return res, nil
}

// saySearchAnswer words Test key's answer: it worked, the provider refused
// the key, wants payment, is limiting it, could not be reached, or answered
// in a way a draft cannot use. Only the status is said of a refusal: the
// provider's body can echo what was asked.
func saySearchAnswer(name string, err error) (bool, string) {
	var se *corewebsearch.StatusError
	var ue *url.Error
	switch {
	case err == nil:
		return true, fmt.Sprintf("The key works: %s answered a search with it.", name)
	case errors.As(err, &se) && (se.Status == 401 || se.Status == 403):
		return false, fmt.Sprintf("%s refused the key (%d). Check that you pasted the whole key and that it has not been revoked.", name, se.Status)
	case errors.As(err, &se) && (se.Status == 402 || se.Status == 432 || se.Status == 433):
		return false, fmt.Sprintf("%s took the key but will not search with it (%d): the account's plan or credit has run out.", name, se.Status)
	case errors.As(err, &se) && se.Status == 429:
		return false, fmt.Sprintf("%s is limiting requests with this key right now (429). Try again in a minute.", name)
	case errors.As(err, &se):
		return false, fmt.Sprintf("%s answered %d, so the key was not confirmed. Try again later.", name, se.Status)
	case errors.Is(err, context.DeadlineExceeded):
		return false, fmt.Sprintf("%s did not answer in time, so the key was not confirmed. Check this computer's connection and try again.", name)
	case errors.As(err, &ue):
		// The dial's own words (no such host, connection refused) say what
		// to check; the request's URL and method do not.
		return false, fmt.Sprintf("%s could not be reached, so the key was not confirmed: %s.", name, ue.Err.Error())
	default:
		return false, fmt.Sprintf("%s's answer was not one a draft can use, so the key was not confirmed: %s", name, err.Error())
	}
}
