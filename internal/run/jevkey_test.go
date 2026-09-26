package run

// The person's own AI Review key (plan builder-rework.md D25, #282): kept in
// the OS keychain beside the model key, recorded in the policy as a source,
// shown as saved and never as its value, never logged, and tested with one
// small call whose answer is said in plain words.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	corejudge "github.com/blaktron/agentry/pkg/judge"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

const testJevKey = "jev-the-persons-own-key-7c1f"

// jevKeychain gives the test an in-memory OS keychain under the real
// modelkey code, and no key in the environment.
func jevKeychain(t *testing.T) {
	t.Helper()
	isolateRunEnv(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	t.Setenv(policy.DefaultJudgeKeyEnv, "")
}

// noValue fails when the key appears anywhere in what a surface returned.
func noValue(t *testing.T, what string, v any) {
	t.Helper()
	if s := fmt.Sprintf("%+v", v); strings.Contains(s, testJevKey) {
		t.Fatalf("%s carries the key: %s", what, s)
	}
}

func TestStoreJevKeyKeepsItInTheKeychainAndRecordsTheSource(t *testing.T) {
	jevKeychain(t)
	var log strings.Builder
	if err := StoreJevKey("  "+testJevKey+"\n", &log); err != nil {
		t.Fatal(err)
	}
	got, err := modelkey.Lookup(policy.JudgeKeyProvider)
	if err != nil || got != testJevKey {
		t.Fatalf("keychain holds %q (%v); want the trimmed key", got, err)
	}
	path, _ := policy.DefaultPath()
	pol, _, err := policy.Load(path)
	if err != nil || pol.Judge.APIKeySource != modelkey.SourceKeyring {
		t.Fatalf("policy judge block %+v (%v); want apiKeySource keyring", pol.Judge, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), testJevKey) {
		t.Fatal("the key reached the policy file")
	}
	if strings.Contains(log.String(), testJevKey) || !strings.Contains(log.String(), "provider:typesafe") {
		t.Fatalf("log %q", log.String())
	}
	st, err := GetJevKeyStatus()
	if err != nil || !st.Stored || st.Source != modelkey.SourceKeyring || st.EnvName != policy.DefaultJudgeKeyEnv || st.EnvSet || !st.Ready() {
		t.Fatalf("status %+v (%v)", st, err)
	}
	noValue(t, "the status", st)
}

// A keychain that cannot be reached refuses the store by name and leaves
// the policy exactly as it was (D-024's rule, as for the model key).
func TestStoreJevKeyRefusedByTheKeychainLeavesThePolicy(t *testing.T) {
	jevKeychain(t)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	var log strings.Builder
	err := StoreJevKey(testJevKey, &log)
	if err == nil || !strings.Contains(err.Error(), modelkey.EnvNoKeyring) || strings.Contains(err.Error(), testJevKey) {
		t.Fatalf("err = %v", err)
	}
	path, _ := policy.DefaultPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("a refused store wrote the policy")
	}
	if strings.Contains(log.String(), testJevKey) {
		t.Fatalf("log %q", log.String())
	}
	st, err := GetJevKeyStatus()
	if err != nil || st.Stored || st.KeyStoreErr == "" || st.Ready() {
		t.Fatalf("status %+v (%v): the switch must be reported, not read as 'not saved'", st, err)
	}
	if err := StoreJevKey("   ", &log); err == nil {
		t.Fatal("an empty key was stored")
	}
	// A policy that exists is left byte for byte, too.
	pol := policy.Default()
	pol.Judge.APIKeyEnv = "MY_JEV"
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := StoreJevKey(testJevKey, &log); err == nil {
		t.Fatal("stored with the keychain switched off")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("a refused store changed the policy:\n%s", after)
	}
}

func TestForgetJevKeyRemovesTheEntryAndTheSource(t *testing.T) {
	jevKeychain(t)
	var log strings.Builder
	if err := StoreJevKey(testJevKey, &log); err != nil {
		t.Fatal(err)
	}
	if err := ForgetJevKey(&log); err != nil {
		t.Fatal(err)
	}
	if present, _ := modelkey.Present(policy.JudgeKeyProvider); present {
		t.Fatal("the key is still in the keychain")
	}
	path, _ := policy.DefaultPath()
	pol, _, _ := policy.Load(path)
	if pol.Judge.APIKeySource != "" {
		t.Fatalf("the source outlived the key: %+v", pol.Judge)
	}
	st, _ := GetJevKeyStatus()
	if st.Stored || st.Ready() {
		t.Fatalf("status %+v", st)
	}
}

// The status says the variable goes first when it is set where the app was
// started, and it reads the keychain's presence marker, never the key.
func TestJevKeyStatusReadsTheMarkerAndSaysTheVariableGoesFirst(t *testing.T) {
	jevKeychain(t)
	var log strings.Builder
	if err := StoreJevKey(testJevKey, &log); err != nil {
		t.Fatal(err)
	}
	old := keychainLookup
	keychainLookup = func(string) (string, error) { t.Fatal("the status read the key"); return "", nil }
	t.Cleanup(func() { keychainLookup = old })
	t.Setenv(policy.DefaultJudgeKeyEnv, "jev-from-the-variable")
	st, err := GetJevKeyStatus()
	if err != nil || !st.EnvSet || !st.Stored || !st.Ready() {
		t.Fatalf("status %+v (%v)", st, err)
	}
	noValue(t, "the status", st)
}

// Test key makes one small call with the key a draft on this computer would
// scan with, and says the answer in plain words, never with the key in it.
func TestTestJevKeySaysTheVendorsAnswer(t *testing.T) {
	jevKeychain(t)
	var log strings.Builder
	if err := StoreJevKey(testJevKey, &log); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		err  error
		ok   bool
		want string
	}{
		{"works", nil, true, "The key works"},
		{"401", &corejudge.StatusError{Status: 401, Snippet: "unauthorized " + testJevKey}, false, "TypeSafe refused the key (401)"},
		{"403", &corejudge.StatusError{Status: 403}, false, "TypeSafe refused the key (403)"},
		{"429", &corejudge.StatusError{Status: 429}, false, "limiting requests with this key right now (429)"},
		{"500", &corejudge.StatusError{Status: 500}, false, "TypeSafe answered 500"},
		{"unreachable", fmt.Errorf("judge: typesafe request: %w", &url.Error{Op: "Post", URL: "https://api.typesafe.ai/v1/systemone?k=" + testJevKey,
			Err: errors.New("dial tcp: lookup api.typesafe.ai: no such host")}), false, "TypeSafe could not be reached"},
		{"timeout", fmt.Errorf("judge: typesafe request: %w", context.DeadlineExceeded), false, "TypeSafe did not answer in time"},
		{"odd answer", errors.New(`judge: typesafe served model "jev-2", not the pinned "jev-1.13.0"`), false, "not one AI Review can use"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var asked []string
			old := askJev
			askJev = func(_ context.Context, key string) error { asked = append(asked, key); return c.err }
			t.Cleanup(func() { askJev = old })
			res, err := TestJevKey(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(asked) != 1 || asked[0] != testJevKey {
				t.Fatalf("asked with %d keys", len(asked))
			}
			if res.OK != c.ok || !strings.Contains(res.Message, c.want) || res.From != "the keychain" || strings.Contains(res.Message, "api.typesafe.ai/v1") {
				t.Fatalf("result %+v", res)
			}
			noValue(t, "the result", res)
		})
	}

	// The variable goes first, as it does for a draft.
	t.Setenv(policy.DefaultJudgeKeyEnv, "jev-from-the-variable")
	var asked string
	old := askJev
	askJev = func(_ context.Context, key string) error { asked = key; return nil }
	t.Cleanup(func() { askJev = old })
	res, err := TestJevKey(context.Background())
	if err != nil || asked != "jev-from-the-variable" || res.From != "$"+policy.DefaultJudgeKeyEnv {
		t.Fatalf("%+v %v asked=%q", res, err, asked)
	}

	// No key: nothing is asked, and it says where to add one.
	t.Setenv(policy.DefaultJudgeKeyEnv, "")
	if err := ForgetJevKey(&log); err != nil {
		t.Fatal(err)
	}
	askJev = func(context.Context, string) error { t.Fatal("asked with no key"); return nil }
	res, err = TestJevKey(context.Background())
	if err != nil || res.OK || !strings.Contains(res.Message, "Settings") {
		t.Fatalf("%+v %v", res, err)
	}
}
