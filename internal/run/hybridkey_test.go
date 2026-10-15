package run

// The person's own Hybrid Analysis key: kept in the OS keychain as
// provider:hybrid-analysis, recorded in the policy's hybridAnalysis block
// as a source, shown as saved and never as its value, read by the CLI
// exactly as this app stores it, and tested with one lookup of a hash whose
// answer is said in plain words.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/hybrid"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

const testHybridKey = "ha-the-persons-own-key-41d9"

func hybridKeychain(t *testing.T) {
	t.Helper()
	isolateRunEnv(t)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	t.Setenv(policy.DefaultHybridKeyEnv, "")
}

// Saving a key writes the policy's hybridAnalysis the way the CLI expects:
// the CLI's own reader finds the key the app stored. Clearing it removes
// the entry and the source.
func TestStoreHybridKeyIsReadByTheCLIAndForgetRemovesIt(t *testing.T) {
	hybridKeychain(t)
	var log strings.Builder
	if err := StoreHybridKey("  "+testHybridKey+"\n", &log); err != nil {
		t.Fatal(err)
	}
	path, _ := policy.DefaultPath()
	pol, _, err := policy.Load(path)
	if err != nil || pol.HybridAnalysis.APIKeySource != modelkey.SourceKeyring {
		t.Fatalf("policy hybridAnalysis %+v (%v); want apiKeySource keyring", pol.HybridAnalysis, err)
	}
	if got, err := hybrid.Key(pol); err != nil || got != testHybridKey {
		t.Fatalf("the CLI reads %q (%v); want the stored key", got, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), testHybridKey) || strings.Contains(log.String(), testHybridKey) || !strings.Contains(log.String(), "provider:hybrid-analysis") {
		t.Fatalf("the key reached the policy or the log: %q", log.String())
	}
	st, err := GetHybridKeyStatus()
	if err != nil || !st.Stored || st.Source != modelkey.SourceKeyring || st.EnvName != "HYBRID_API_KEY" || !st.Ready() {
		t.Fatalf("status %+v (%v)", st, err)
	}
	if strings.Contains(strings.Join([]string{st.Source, st.EnvName, st.KeyStoreErr}, " "), testHybridKey) {
		t.Fatal("the status carries the key")
	}

	if err := ForgetHybridKey(&log); err != nil {
		t.Fatal(err)
	}
	if present, _ := modelkey.Present(policy.HybridKeyProvider); present {
		t.Fatal("the key is still in the keychain")
	}
	pol, _, _ = policy.Load(path)
	if pol.HybridAnalysis.APIKeySource != "" {
		t.Fatalf("the source outlived the key: %+v", pol.HybridAnalysis)
	}
	if got, _ := hybrid.Key(pol); got != "" {
		t.Fatalf("the CLI still reads a key: %q", got)
	}
}

// A keychain that cannot be reached refuses the store by name, and the
// policy is not written.
func TestStoreHybridKeyRefusedByTheKeychainLeavesThePolicy(t *testing.T) {
	hybridKeychain(t)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	var log strings.Builder
	err := StoreHybridKey(testHybridKey, &log)
	if err == nil || strings.Contains(err.Error(), testHybridKey) {
		t.Fatalf("err = %v", err)
	}
	path, _ := policy.DefaultPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("a refused store wrote the policy")
	}
	if err := StoreHybridKey("   ", &log); err == nil {
		t.Fatal("an empty key was stored")
	}
}

// Test key asks about one hash with the key a run would use and says the
// answer in the lookup's own words: works, refused, limited, unreachable;
// with no key it asks nothing.
func TestTestHybridKeySaysTheAnswer(t *testing.T) {
	hybridKeychain(t)
	res, err := TestHybridKey(context.Background())
	if err != nil || res.OK || !strings.Contains(res.Message, "No Hybrid Analysis key") {
		t.Fatalf("no key: %+v (%v)", res, err)
	}
	var log strings.Builder
	if err := StoreHybridKey(testHybridKey, &log); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		r    hybrid.Result
		ok   bool
		want string
	}{
		{hybrid.Result{Outcome: hybrid.Refused, ThreatScore: 100}, true, "The key works"},
		{hybrid.Result{Outcome: hybrid.Clean}, true, "The key works"},
		{hybrid.Result{Outcome: hybrid.NotChecked, Reason: hybrid.ReasonKeyRefused}, false, "refused the key"},
		{hybrid.Result{Outcome: hybrid.NotChecked, Reason: hybrid.ReasonQuota}, false, "limiting"},
		{hybrid.Result{Outcome: hybrid.NotChecked, Reason: hybrid.ReasonUnavailable}, false, "could not be reached"},
	} {
		var asked string
		SetLookHybridForTest(t, func(_ context.Context, key string) hybrid.Result { asked = key; return tc.r })
		res, err := TestHybridKey(context.Background())
		if err != nil || res.OK != tc.ok || !strings.Contains(res.Message, tc.want) || res.From != "the keychain" || asked != testHybridKey {
			t.Errorf("%+v: %+v (%v), asked with the stored key: %v", tc.r, res, err, asked == testHybridKey)
		}
		if strings.Contains(res.Message, testHybridKey) {
			t.Error("the answer carries the key")
		}
	}
}

// A local run's result carries its receipt's decode line, which names what
// ClamAV and the Hybrid Analysis lookup made of the files, for the run view
// to show.
func TestAResultCarriesTheDecodeLine(t *testing.T) {
	const line = "... 1 decoded; ...: 1 clean; Hybrid Analysis (CrowdStrike), asked by SHA-256 alone and never sent a file, on the files of a type it is asked about: 1 not checked: no key"
	env := &receipt.Envelope{CapabilitiesEnforced: map[string]string{"decode": line, "knowledge": "k"}}
	if got := resultFromEnvelope(env, &spec.Plan{}); got.Decode != line || got.Knowledge != "k" {
		t.Fatalf("decode %q, knowledge %q", got.Decode, got.Knowledge)
	}
	if got := resultFromEnvelope(&receipt.Envelope{}, &spec.Plan{}); got.Decode != "" {
		t.Fatalf("a run with no files has a decode line %q", got.Decode)
	}
}
