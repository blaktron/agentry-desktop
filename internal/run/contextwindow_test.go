package run

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
)

// recordedServer answers the one path a model server publishes its window at,
// with the body that server sends, and records the bearer it was asked with.
func recordedServer(t *testing.T, path, body string, bearer *string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if bearer != nil {
			*bearer = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1"
}

// realProbe puts the CLI's own probe back for one test.
func realProbe(t *testing.T) {
	t.Helper()
	prev := probeContextWindow
	probeContextWindow = setup.ProbeContextWindow
	t.Cleanup(func() { probeContextWindow = prev })
}

// The servers people run on their own computers, as each one answers.
var recordedWindows = []struct {
	name, path, body string
	want             int64
}{
	{"vLLM", "/v1/models", `{"object":"list","data":[{"id":"qwen3","object":"model","max_model_len":131072}]}`, 131072},
	{"LM Studio", "/api/v0/models/qwen3", `{"id":"qwen3","type":"llm","state":"loaded","max_context_length":262144,"loaded_context_length":65536}`, 65536},
	{"llama.cpp", "/props", `{"default_generation_settings":{"n_ctx":40960},"total_slots":1}`, 40960},
	{"Ollama", "/api/ps", `{"models":[{"name":"qwen3:latest","model":"qwen3:latest","context_length":32768}]}`, 32768},
}

// Choosing a model on a server of the person's own writes the window the
// server reports into the policy, and the form's probe says the same.
func TestApplySetupWritesTheWindowTheServerReports(t *testing.T) {
	for _, tc := range recordedWindows {
		t.Run(tc.name, func(t *testing.T) {
			isolateRunEnv(t)
			withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
			fakeKeychain(t, map[string]string{})
			realProbe(t)
			base := recordedServer(t, tc.path, tc.body, nil)

			found, err := ContextWindow(context.Background(), "custom", base, "qwen3", "")
			if err != nil || found.Tokens != tc.want || !strings.HasSuffix(found.Source, tc.path) {
				t.Fatalf("the form's probe = %+v, %v; want %d from %s", found, err, tc.want, tc.path)
			}
			if err := ApplySetup(context.Background(), SetupOpts{Provider: "custom", Model: "qwen3", BaseURL: base, APIKeyEnv: "LOCAL_KEY"}, &strings.Builder{}); err != nil {
				t.Fatal(err)
			}
			st, err := GetStatus(context.Background())
			if err != nil || st.ContextWindow != tc.want {
				t.Fatalf("status window = %d, %v; want %d", st.ContextWindow, err, tc.want)
			}
		})
	}
}

// A window typed in the form is written as typed, and one outside the CLI's
// bounds is refused; an endpoint that reports nothing leaves it unset.
func TestApplySetupTypedAndUnknownWindows(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	fakeKeychain(t, map[string]string{})
	realProbe(t)
	base := recordedServer(t, "/v1/models", `{"data":[{"id":"qwen3","max_model_len":131072}]}`, nil)
	opts := SetupOpts{Provider: "custom", Model: "qwen3", BaseURL: base, APIKeyEnv: "LOCAL_KEY", ContextWindow: 98304}
	if err := ApplySetup(context.Background(), opts, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if st, _ := GetStatus(context.Background()); st.ContextWindow != 98304 {
		t.Errorf("typed window written as %d", st.ContextWindow)
	}
	opts.ContextWindow = policy.MaxContextWindow + 1
	if err := ApplySetup(context.Background(), opts, &strings.Builder{}); err == nil {
		t.Error("a window over the CLI's bound was written")
	}

	silent := recordedServer(t, "/elsewhere", `{}`, nil)
	found, err := ContextWindow(context.Background(), "custom", silent, "qwen3", "")
	if err != nil || found.Tokens != 0 {
		t.Fatalf("an endpoint that says nothing: %+v, %v", found, err)
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "custom", Model: "other", BaseURL: silent, APIKeyEnv: "LOCAL_KEY"}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if st, _ := GetStatus(context.Background()); st.ContextWindow != 0 {
		t.Errorf("an endpoint that says nothing left a window of %d", st.ContextWindow)
	}
}

// With no key typed, the probe goes with the key a run would use: the one in
// the keychain. A provider that is not an endpoint of the person's own is
// never asked.
func TestContextWindowProbeUsesTheStoredKey(t *testing.T) {
	isolateRunEnv(t)
	fakeKeychain(t, map[string]string{"custom": "sk-kept"})
	realProbe(t)
	var bearer string
	base := recordedServer(t, "/v1/models", `{"data":[{"id":"qwen3","max_model_len":8192}]}`, &bearer)
	if found, err := ContextWindow(context.Background(), "custom", base, "qwen3", ""); err != nil || found.Tokens != 8192 {
		t.Fatalf("probe = %+v, %v", found, err)
	}
	if bearer != "Bearer sk-kept" {
		t.Errorf("the endpoint was asked with %q", bearer)
	}
	if _, err := ContextWindow(context.Background(), "custom", base, "qwen3", "sk-typed"); err != nil || bearer != "Bearer sk-typed" {
		t.Errorf("a typed key: %v, asked with %q", err, bearer)
	}

	asked := false
	probeContextWindow = func(context.Context, string, string, string) (setup.ContextWindow, error) {
		asked = true
		return setup.ContextWindow{Tokens: 1 << 20}, nil
	}
	if found, err := ContextWindow(context.Background(), "anthropic", "", "claude-sonnet-5-5", ""); err != nil || found.Tokens != 0 || asked {
		t.Errorf("a known provider: %+v, %v, asked %v", found, err, asked)
	}
}
