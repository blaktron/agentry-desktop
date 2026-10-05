// The person's own Hybrid Analysis key (plan file-handling.md §7, D7, D8,
// D17; #530): the key with which a run on this computer looks up the risky
// files among its inputs on CrowdStrike's Hybrid Analysis, by their SHA-256
// alone (agentry-cli#564). It is kept as the AI Review key is: the OS
// keychain entry provider:hybrid-analysis under the CLI's service, and the
// runner policy's hybridAnalysis block records only that the keychain is its
// source, so `agentry run` and this app read the same key the same way
// (pkg/runner/hybrid's Key). No key means no lookup and no warning; the run's
// receipt says "not checked: no key". The key crosses the app's IPC once,
// inward, in a call of its own, and no surface returns it.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/hybrid"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// HybridKeyStatus is where the person's Hybrid Analysis key stands on this
// machine. It never carries the key.
type HybridKeyStatus struct {
	// Stored: the keychain holds the entry provider:hybrid-analysis (read
	// from its presence marker, never the key).
	Stored bool
	// Source is the policy's hybridAnalysis.apiKeySource.
	Source string
	// EnvName is the variable a run reads first; EnvSet says it is set in
	// this app's environment, and so is the key a run uses.
	EnvName string
	EnvSet  bool
	// KeyStoreErr is why the keychain could not be asked.
	KeyStoreErr string
}

// Ready reports whether a run on this computer would look up with a key:
// pkg/runner/hybrid's read, the variable and then the keychain for a
// keyring source, answered from presence alone.
func (s *HybridKeyStatus) Ready() bool {
	return s.EnvSet || (s.Stored && s.Source == modelkey.SourceKeyring)
}

// GetHybridKeyStatus reads the key's standing without writing anything and
// without reading the key.
func GetHybridKeyStatus() (*HybridKeyStatus, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	st := &HybridKeyStatus{Source: pol.HybridAnalysis.APIKeySource, EnvName: pol.HybridAnalysis.KeyEnv()}
	st.EnvSet = strings.TrimSpace(os.Getenv(st.EnvName)) != ""
	stored, err := keychainPresent(policy.HybridKeyProvider)
	if err != nil {
		st.KeyStoreErr = err.Error()
	}
	st.Stored = stored
	return st, nil
}

// StoreHybridKey puts the key into the OS keychain as
// provider:hybrid-analysis, then records the keychain as its source in the
// runner policy. The keychain goes first: a store that fails leaves the
// policy exactly as it was and says why, as for the AI Review key.
func StoreHybridKey(key string, logw io.Writer) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("paste your Hybrid Analysis API key first")
	}
	if err := keychainStore(policy.HybridKeyProvider, key); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "Hybrid Analysis key stored in the OS keychain as %s\n", modelkey.Account(policy.HybridKeyProvider))
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	pol.HybridAnalysis.APIKeySource = modelkey.SourceKeyring
	if err := policy.Save(path, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s (hybridAnalysis.apiKeySource: keyring)\n", path)
	return nil
}

// ForgetHybridKey removes the key from the keychain and the keychain from
// the policy's sources, so nothing goes looking for a key that is not
// there.
func ForgetHybridKey(logw io.Writer) error {
	if err := keychainDelete(policy.HybridKeyProvider); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "Hybrid Analysis key removed from the OS keychain (%s)\n", modelkey.Account(policy.HybridKeyProvider))
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil || pol.HybridAnalysis.APIKeySource == "" {
		return err
	}
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol.HybridAnalysis.APIKeySource = ""
	return policy.Save(path, pol)
}

// HybridKeyTest is what Test key found: whether the key worked, the
// sentence that says so, and which key it was.
type HybridKeyTest struct {
	OK      bool
	Message string
	From    string
}

// eicarSHA256 is what Test key asks about: the SHA-256 of the EICAR
// anti-malware test file, which every engine knows. Only the hash is sent.
const eicarSHA256 = "275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f"

// lookHybrid makes Test key's one lookup. A seam: the tests have no Hybrid
// Analysis.
var lookHybrid = func(ctx context.Context, key string) hybrid.Result {
	return (&hybrid.Client{Key: key}).Look(ctx, []string{eicarSHA256})[eicarSHA256]
}

// SetLookHybridForTest points Test key's lookup at a stand-in for the
// test's duration (the bound surface's tests have no Hybrid Analysis).
func SetLookHybridForTest(t interface{ Cleanup(func()) }, f func(context.Context, string) hybrid.Result) {
	old := lookHybrid
	lookHybrid = f
	t.Cleanup(func() { lookHybrid = old })
}

// TestHybridKey looks up one known hash with the key a run on this
// computer would use (read as pkg/runner/hybrid reads it: the variable
// first, then the keychain), and says the answer in plain words. Only a
// local failure is an error; the service's refusal is an answer.
func TestHybridKey(ctx context.Context) (*HybridKeyTest, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	key, err := hybrid.Key(pol)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return &HybridKeyTest{Message: "No Hybrid Analysis key is saved on this computer. Paste one under Settings → Provider settings → Hybrid Analysis key, then test it."}, nil
	}
	res := &HybridKeyTest{From: "the keychain"}
	if strings.TrimSpace(os.Getenv(pol.HybridAnalysis.KeyEnv())) != "" {
		res.From = "$" + pol.HybridAnalysis.KeyEnv()
	}
	res.OK, res.Message = sayHybridAnswer(lookHybrid(ctx, key))
	return res, nil
}

// sayHybridAnswer words Test key's answer from the lookup's fixed
// vocabulary, never from the service's own text.
func sayHybridAnswer(r hybrid.Result) (bool, string) {
	switch {
	case r.Outcome != hybrid.NotChecked:
		return true, "The key works: Hybrid Analysis answered a lookup of the EICAR test file's fingerprint with it."
	case r.Reason == hybrid.ReasonKeyRefused:
		return false, "Hybrid Analysis refused the key. Check that you pasted the whole API key and that it has not been revoked."
	case r.Reason == hybrid.ReasonQuota:
		return false, "Hybrid Analysis is limiting requests with this key right now. Try again in a few minutes."
	default:
		return false, "Hybrid Analysis could not be reached, or did not answer as expected, so the key was not confirmed. Check this computer's connection and try again."
	}
}
