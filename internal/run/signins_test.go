package run

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/mcpauth/mcpauthtest"
	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/adapter/copilottest"
	"github.com/blaktron/agentry-cli/pkg/runner/connection"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/harnessbuild"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// signInFixture isolates the runner home and the keychain: secrets go to
// 0600 files, and a connected key to an in-memory model-key keychain.
func signInFixture(t *testing.T) (string, *sync.Map) {
	t.Helper()
	home := isolateRunEnv(t)
	oldKeys, oldStore, oldDelete := connectionKeys, connectStoreKey, connectDeleteKey
	keys := &sync.Map{}
	connectionKeys = func() connection.Keychain { return nil }
	connectStoreKey = func(provider, key string) error { keys.Store(provider, key); return nil }
	connectDeleteKey = func(provider string) error { keys.Delete(provider); return nil }
	t.Cleanup(func() { connectionKeys, connectStoreKey, connectDeleteKey = oldKeys, oldStore, oldDelete })
	return home, keys
}

// signInPerson is the person: it follows every page it is sent to with c (the
// fake vendor's client, which approves), and records the lines, pages and
// codes the sign-in showed.
type signInPerson struct {
	mu                  sync.Mutex
	said, opened, codes []string
}

func (b *signInPerson) ui(c *http.Client) SignInUI {
	return SignInUI{
		Say: func(l string) { b.mu.Lock(); b.said = append(b.said, l); b.mu.Unlock() },
		Open: func(u string) {
			b.mu.Lock()
			b.opened = append(b.opened, u)
			b.mu.Unlock()
			if c != nil {
				go func() {
					if res, err := c.Get(u); err == nil {
						_ = res.Body.Close()
					}
				}()
			}
		},
		Code: func(page, code string) { b.mu.Lock(); b.codes = append(b.codes, code); b.mu.Unlock() },
	}
}

// noSecret fails if any of the fake's token prefixes appear in v.
func noSecret(t *testing.T, v any, prefixes ...string) {
	t.Helper()
	b, _ := json.Marshal(v)
	for _, p := range prefixes {
		if strings.Contains(string(b), p) {
			t.Fatalf("%s reached the page: %s", p, b)
		}
	}
}

// Sign in with ChatGPT through the CLI's own flow and store: the account
// and the plan's models come back, never a token; model setup can then
// make it the active choice, which runs on the Codex image; Remove revokes
// it at OpenAI and forgets it.
func TestSignInWithChatGPTThenChooseItThenRemoveIt(t *testing.T) {
	signInFixture(t)
	v := mcpauthtest.NewChatGPT(t)
	old := chatgptEndpoints
	chatgptEndpoints = func(f *mcpauth.ChatGPT) {
		fake := v.Flow()
		present, say := f.Present, f.Say
		*f = fake
		f.Present, f.Say = present, say
	}
	t.Cleanup(func() { chatgptEndpoints = old })
	// Refresh and revoke go to the vendor the store has registered: the
	// fake, never OpenAI.
	t.Cleanup(modelsignin.RegisterVendor(modelsignin.ChatGPT, modelsignin.ChatGPTVendor{Flow: v.Flow()}))
	var b signInPerson
	got, err := SignInChatGPT(context.Background(), b.ui(v.Client()), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Account != "person@example.com" || len(got.Models) == 0 || len(b.opened) != 1 {
		t.Fatalf("signed in %+v, opened %v", got, b.opened)
	}
	list, err := ModelSignIns()
	if err != nil || len(list) != 1 || list[0].Vendor != modelsignin.ChatGPT || list[0].Provider != policy.ProviderOpenAI || list[0].Account != "person@example.com" {
		t.Fatalf("kept %+v (%v)", list, err)
	}
	noSecret(t, []any{got, list}, v.AccessPrefix, v.RefreshPrefix)

	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderOpenAI, Model: got.Models[0], APIKeyKind: policy.KeyKindChatGPTPlan}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	pol := loadedPolicy(t)
	if pol.Model.APIKeyKind != policy.KeyKindChatGPTPlan || adapter.DefaultFor(pol) != adapter.CodexImageAdapterName {
		t.Fatalf("model %+v runs on %s", pol.Model, adapter.DefaultFor(pol))
	}
	outcome, err := RemoveModelSignIn(context.Background(), modelsignin.ChatGPT, &strings.Builder{})
	if err != nil || !strings.Contains(outcome, "revoked") || len(v.Revocations) == 0 {
		t.Fatalf("remove = %q, %v; revocations %v", outcome, err, v.Revocations)
	}
	if list, _ := ModelSignIns(); len(list) != 0 {
		t.Fatalf("still kept %+v", list)
	}
}

// Sign in with GitHub hands the page GitHub's device code and opens
// GitHub's page; the grant is kept, never shown.
func TestSignInWithGitHubShowsTheCodeAndKeepsTheGrant(t *testing.T) {
	signInFixture(t)
	t.Setenv("AGENTRY_GITHUB_APP_CLIENT_ID", mcpauthtest.GitHubClientID)
	v := mcpauthtest.NewGitHub(t)
	old := githubEndpoints
	githubEndpoints = func(f *mcpauth.GitHub) {
		fake := v.Flow()
		present := f.Present
		*f = fake
		f.Present = present
	}
	t.Cleanup(func() { githubEndpoints = old })
	t.Cleanup(modelsignin.RegisterVendor(modelsignin.Copilot, modelsignin.CopilotVendor{Flow: v.Flow()}))
	var b signInPerson
	got, err := SignInCopilot(context.Background(), b.ui(nil), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Account != "octo-fixture" || len(b.codes) != 1 || b.codes[0] == "" || len(b.opened) != 1 || !strings.HasSuffix(b.opened[0], "/login/device") {
		t.Fatalf("signed in %+v; codes %v; opened %v", got, b.codes, b.opened)
	}
	list, _ := ModelSignIns()
	noSecret(t, []any{got, list}, v.AccessPrefix, v.RefreshPrefix)
	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderCopilot, Model: "gpt-5", APIKeyKind: policy.KeyKindCopilotSignIn, CopilotPlan: "business"}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if pol := loadedPolicy(t); adapter.DefaultFor(pol) != adapter.CopilotImageAdapterName || !strings.Contains(strings.Join(pol.Model.ProxyAllow, " "), "business") {
		t.Fatalf("model %+v runs on %s", pol.Model, adapter.DefaultFor(pol))
	}
}

// Connect your account keeps the minted key in the model key's keychain
// entry for its provider, where a pasted key would be; the record only
// marks it connected; Disconnect deletes the key.
func TestConnectOpenRouterKeepsAnOrdinaryKeyThenDisconnects(t *testing.T) {
	_, keys := signInFixture(t)
	v := mcpauthtest.NewOpenRouter(t)
	old := openRouterEndpoints
	openRouterEndpoints = func(f *mcpauth.OpenRouter) {
		fake := v.Flow()
		present, say := f.Present, f.Say
		*f = fake
		f.Present, f.Say = present, say
	}
	t.Cleanup(func() { openRouterEndpoints = old })
	var b signInPerson
	got, err := ConnectAccount(context.Background(), b.ui(v.Client()), modelsignin.OpenRouter)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := keys.Load(policy.ProviderOpenRouter)
	if !ok || !strings.HasPrefix(key.(string), v.KeyPrefix) {
		t.Fatalf("keychain entry %v", key)
	}
	list, _ := ModelSignIns()
	if len(list) != 1 || !list[0].Connected {
		t.Fatalf("kept %+v", list)
	}
	noSecret(t, []any{got, list}, v.KeyPrefix)
	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderOpenRouter, Model: "anthropic/claude-sonnet-4.5", Connect: true}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if pol := loadedPolicy(t); pol.Model.Provider != policy.ProviderOpenRouter || adapter.DefaultFor(pol) != adapter.DefaultName {
		t.Fatalf("model %+v", pol.Model)
	}
	if _, err := RemoveModelSignIn(context.Background(), modelsignin.OpenRouter, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys.Load(policy.ProviderOpenRouter); ok {
		t.Fatal("the connected key outlived its Disconnect")
	}
}

// Model setup never signs in itself: a sign-in choice with nothing signed
// in is refused, and the policy is left as it was.
func TestChoosingASignInNothingIsSignedInToIsRefused(t *testing.T) {
	signInFixture(t)
	for _, c := range []SetupOpts{
		{Provider: policy.ProviderAnthropic, Model: "sonnet", APIKeyKind: policy.KeyKindClaudeSignIn},
		{Provider: policy.ProviderOpenAI, Model: "gpt-5", APIKeyKind: policy.KeyKindChatGPTPlan},
		{Provider: policy.ProviderCopilot, Model: "gpt-5", APIKeyKind: policy.KeyKindCopilotSignIn},
		{Provider: policy.ProviderPoe, Model: "Claude-Sonnet-4.5", Connect: true},
	} {
		err := ApplySetup(context.Background(), c, &strings.Builder{})
		if err == nil || !strings.Contains(err.Error(), "sign in first") {
			t.Errorf("%+v: %v", c, err)
		}
	}
	if pol := loadedPolicy(t); policy.IsModelSignIn(pol.Model.APIKeyKind) {
		t.Fatalf("the policy names a sign-in: %+v", pol.Model)
	}
}

// loadedPolicy is the operator policy model setup wrote, or the default.
func loadedPolicy(t *testing.T) *policy.Policy {
	t.Helper()
	pol, err := loadPolicyIfPresent()
	if err != nil {
		t.Fatal(err)
	}
	if pol == nil {
		return policy.Default()
	}
	return pol
}

// Use your Claude subscription from the desktop finishes in the browser
// alone (agentry-cli#848): the page gives Claude Code no input, Claude
// Code's address is opened, and the browser's answer reaches its callback
// through the relay; the account is kept, never a credential, and model
// setup can then choose it, which runs on the Claude Code image.
func TestUseYourClaudeSubscriptionSignsInThroughTheBrowser(t *testing.T) {
	signInFixture(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	auto := "https://claude.com/cai/oauth/authorize?code=true&redirect_uri=http%3A%2F%2Flocalhost%3A" + strconv.Itoa(port) + "%2Fcallback&state=abc"
	called := make(chan struct{}, 1)
	pol := policy.Default()
	h, _ := pol.Images.HarnessImageFor("claude-image")
	fd := &dockerx.Fake{
		ExistingImages: map[string]bool{h.Ref: true},
		Labels:         map[string]map[string]string{h.Ref: {harnessbuild.LabelArtifact: h.Artifact, harnessbuild.LabelRecipe: harnessbuild.ClaudeRecipe}},
		Foreground: map[string]*dockerx.FakeForeground{
			"agentry-claude-login": {Handler: func(ctx context.Context, _ dockerx.RunSpec, _ io.Reader, stdout, stderr io.Writer) (int, error) {
				_, _ = io.WriteString(stdout, "Opening browser to sign in…\nIf the browser didn't open, visit: https://claude.com/cai/oauth/authorize?redirect_uri=https%3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback\n")
				_, _ = io.WriteString(stderr, "agentry-open "+auto+"\n")
				select {
				case <-called:
					_, _ = io.WriteString(stdout, "Login successful.\n")
					return 0, nil
				case <-ctx.Done():
					return 137, nil
				}
			}},
			"agentry-claude-status": {Stdout: `{"loggedIn":true,"authMethod":"claude.ai","email":"person@example.com","orgName":"Example"}`},
		},
		Exec: func(_ context.Context, _ string, _ []string, stdin io.Reader, stdout io.Writer) (int, error) {
			if _, err := http.ReadRequest(bufio.NewReader(stdin)); err != nil {
				return 1, nil
			}
			_, _ = io.WriteString(stdout, "HTTP/1.1 302 Found\r\nLocation: https://platform.claude.com/oauth/code/success\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			called <- struct{}{}
			return 0, nil
		},
	}
	withFakeDocker(t, fd)
	var b signInPerson
	ui := b.ui(nil)
	open := ui.Open
	ui.Open = func(u string) {
		open(u)
		go func() {
			c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			if res, err := c.Get("http://localhost:" + strconv.Itoa(port) + "/callback?code=THE-CODE&state=abc"); err == nil {
				_ = res.Body.Close()
			}
		}()
	}
	var log strings.Builder
	got, err := SignInClaude(context.Background(), ui, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if got.Account != "person@example.com (Example)" || len(b.opened) != 1 || b.opened[0] != auto {
		t.Fatalf("signed in %+v; opened %v", got, b.opened)
	}
	if strings.Contains(log.String()+strings.Join(b.said, "\n"), "THE-CODE") {
		t.Fatal("the code was shown")
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderAnthropic, Model: "sonnet", APIKeyKind: policy.KeyKindClaudeSignIn}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if pol := loadedPolicy(t); adapter.DefaultFor(pol) != adapter.ClaudeImageAdapterName {
		t.Fatalf("model %+v runs on %s", pol.Model, adapter.DefaultFor(pol))
	}
}

// A Copilot plan's models are asked of Copilot CLI through the CLI's own
// list (agentry-cli#808): in a run's container behind a proxy allowing
// GitHub's API and the chosen plan's host alone, the kept sign-in's token
// handed over the session and never to docker; a disabled model is not
// listed. With nothing kept there is nothing to list.
func TestCopilotModelsAreListedThroughCopilotCLI(t *testing.T) {
	signInFixture(t)
	if _, err := CopilotModels(context.Background(), "individual", io.Discard); err == nil {
		t.Fatal("listed with no Copilot sign-in kept")
	}
	// The plan is the page's: anything but GitHub's three is refused before
	// docker is asked anything.
	refusing := &dockerx.Fake{}
	withFakeDocker(t, refusing)
	if _, err := CopilotModels(context.Background(), "api.example.com", io.Discard); err == nil || len(refusing.Calls) != 0 {
		t.Fatalf("a plan that is not one: %v, docker calls %v", err, refusing.Ops())
	}
	t.Setenv("AGENTRY_GITHUB_APP_CLIENT_ID", mcpauthtest.GitHubClientID)
	v := mcpauthtest.NewGitHub(t)
	old := githubEndpoints
	githubEndpoints = func(f *mcpauth.GitHub) {
		fake := v.Flow()
		present := f.Present
		*f = fake
		f.Present = present
	}
	t.Cleanup(func() { githubEndpoints = old })
	t.Cleanup(modelsignin.RegisterVendor(modelsignin.Copilot, modelsignin.CopilotVendor{Flow: v.Flow()}))
	var b signInPerson
	if _, err := SignInCopilot(context.Background(), b.ui(nil), false); err != nil {
		t.Fatal(err)
	}

	pol := policy.Default()
	h, _ := pol.Images.HarnessImageFor(adapter.CopilotImageAdapterName)
	srv := &copilottest.Server{Models: []map[string]any{
		{"id": "claude-sonnet-5.5", "name": "Claude Sonnet 5.5", "billing": map[string]any{"multiplier": 1}},
		{"id": "claude-opus-5.5", "policy": map[string]any{"state": "disabled"}},
	}}
	d := &dockerx.Fake{
		ExistingImages: map[string]bool{h.Ref: true, runnerImageTag(pol): true, pol.Images.Gateway: true},
		Labels:         map[string]map[string]string{h.Ref: {harnessbuild.LabelArtifact: h.Artifact}},
		Foreground: map[string]*dockerx.FakeForeground{"-models": {Handler: func(_ context.Context, _ dockerx.RunSpec, stdin io.Reader, stdout, _ io.Writer) (int, error) {
			if err := srv.Serve(stdin, stdout); err != nil {
				return 1, err
			}
			return 0, nil
		}}},
	}
	withFakeDocker(t, d)
	var log strings.Builder
	got, err := CopilotModels(context.Background(), "business", &log)
	if err != nil {
		t.Fatal(err)
	}
	noSecret(t, []any{got, log.String()}, v.AccessPrefix, v.RefreshPrefix)
	if len(got) != 1 || got[0].ID != "claude-sonnet-5.5" || got[0].Name != "Claude Sonnet 5.5" || got[0].Multiplier != 1 {
		t.Fatalf("models %+v", got)
	}
	proxy, ok := d.FindRun("-proxy")
	if !ok || !strings.Contains(strings.Join(proxy.Argv, " "), "api.business.githubcopilot.com:443") {
		t.Fatalf("proxy %+v", proxy)
	}
	var calls []any
	for _, c := range d.Calls {
		calls = append(calls, c)
	}
	noSecret(t, calls, v.AccessPrefix, v.RefreshPrefix)
	_, _, toks, _, _ := srv.Snapshot()
	if len(toks) != 1 || !strings.HasPrefix(toks[0]["accessToken"].(string), v.AccessPrefix) {
		t.Fatalf("tokens %v", toks)
	}
}
