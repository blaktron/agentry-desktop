// The person's own AI Review key (plan builder-rework.md D25; #282): their
// TypeSafe Jev key, which every local action that requires a scan uses —
// today, Describe it on this computer. It is kept where the model key is
// kept, the OS keychain entry provider:typesafe under the CLI's service, and
// the runner policy records only where it comes from (judge.apiKeySource:
// keyring), so `agentry draft` and this app read the same key the same way
// (pkg/drafter's LocalWeb). The key crosses the app's IPC once, inward, in a
// call of its own, and no surface returns it.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	corejudge "github.com/blaktron/agentry/pkg/judge"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// JevKeyStatus is where the person's Jev key stands on this machine. It
// never carries the key.
type JevKeyStatus struct {
	// Stored: the keychain holds the entry provider:typesafe (read from its
	// presence marker, never the key).
	Stored bool
	// Source is the policy's judge.apiKeySource: "keyring" when the
	// keychain is read, "" or "env" when only the variable is.
	Source string
	// EnvName is the variable read first; EnvSet says it is set in this
	// app's environment, and so is the key a draft uses.
	EnvName string
	EnvSet  bool
	// KeyStoreErr is why the keychain could not be asked.
	KeyStoreErr string
}

// Ready reports whether a draft on this computer would find a key. It is
// pkg/drafter's read, the variable and then the keychain for a keyring
// source, answered from presence alone, since a status never reads a key.
func (s *JevKeyStatus) Ready() bool {
	return s.EnvSet || (s.Stored && s.Source == modelkey.SourceKeyring)
}

// GetJevKeyStatus reads the key's standing without writing anything and
// without reading the key.
func GetJevKeyStatus() (*JevKeyStatus, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	st := &JevKeyStatus{Source: pol.Judge.APIKeySource, EnvName: pol.Judge.KeyEnv()}
	st.EnvSet = strings.TrimSpace(os.Getenv(st.EnvName)) != ""
	stored, err := keychainPresent(policy.JudgeKeyProvider)
	if err != nil {
		st.KeyStoreErr = err.Error()
	}
	st.Stored = stored
	return st, nil
}

// StoreJevKey puts the key into the OS keychain as provider:typesafe, then
// records the keychain as its source in the runner policy. The keychain goes
// first: a store that fails leaves the policy exactly as it was and says why
// (D-024's rule for the model key), rather than record a source the machine
// cannot honour.
func StoreJevKey(key string, logw io.Writer) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("paste your TypeSafe Jev key first")
	}
	if err := keychainStore(policy.JudgeKeyProvider, key); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "AI Review key stored in the OS keychain as %s\n", modelkey.Account(policy.JudgeKeyProvider))
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	pol.Judge.APIKeySource = modelkey.SourceKeyring
	if err := policy.Save(path, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s (judge.apiKeySource: keyring)\n", path)
	return nil
}

// ForgetJevKey removes the key from the keychain and the keychain from the
// policy's sources, so nothing goes looking for a key that is not there.
func ForgetJevKey(logw io.Writer) error {
	if err := keychainDelete(policy.JudgeKeyProvider); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "AI Review key removed from the OS keychain (%s)\n", modelkey.Account(policy.JudgeKeyProvider))
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil || pol.Judge.APIKeySource == "" {
		return err
	}
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol.Judge.APIKeySource = ""
	return policy.Save(path, pol)
}

// JevKeyTest is what Test key found: whether the key worked, the sentence
// that says so, and which key it was (the keychain's, or the variable's).
type JevKeyTest struct {
	OK      bool
	Message string
	From    string
}

// askJev makes Test key's one small call: one model-input question about a
// fixed line of text, one request. A seam: the tests have no TypeSafe.
var askJev = func(ctx context.Context, key string) error {
	c, err := corejudge.New(corejudge.Config{APIKey: key})
	if err != nil {
		return err
	}
	_, _, err = c.Ask(ctx, map[string]string{"source": string(corejudge.SourceDescription), "content": "Agentry Desktop is checking this AI Review key."},
		corejudge.ModelInputQuestions()[:1])
	return err
}

// SetAskJevForTest points Test key's call at a stand-in for the test's
// duration (the bound surface's tests have no TypeSafe to ask).
func SetAskJevForTest(t interface{ Cleanup(func()) }, f func(context.Context, string) error) {
	old := askJev
	askJev = f
	t.Cleanup(func() { askJev = old })
}

// TestJevKey tries the key a draft on this computer would scan with (read
// as pkg/drafter's LocalWeb reads it: the variable first, then the keychain)
// with one small call, and says TypeSafe's answer in plain words. Only a
// local failure is an error; TypeSafe's refusal is an answer.
func TestJevKey(ctx context.Context) (*JevKeyTest, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	w, _, err := drafter.LocalWeb(pol, "")
	switch {
	case errors.Is(err, drafter.ErrNoJevKey):
		return &JevKeyTest{Message: "No AI Review key is saved on this computer. Paste your TypeSafe Jev key under Settings → Provider settings → AI Review key, then test it."}, nil
	case err != nil:
		return nil, err
	}
	res := &JevKeyTest{From: "the keychain"}
	if strings.TrimSpace(os.Getenv(pol.Judge.KeyEnv())) != "" {
		res.From = "$" + pol.Judge.KeyEnv()
	}
	res.OK, res.Message = sayJevAnswer(askJev(ctx, w.JevKey))
	res.Message = strings.ReplaceAll(res.Message, w.JevKey, "[key]")
	return res, nil
}

// sayJevAnswer words Test key's answer: it worked, TypeSafe refused the key,
// TypeSafe is limiting it, TypeSafe could not be reached, or TypeSafe
// answered in a way AI Review cannot use. Only the status is said of a
// refusal: the vendor's body can echo what was asked.
func sayJevAnswer(err error) (bool, string) {
	var se *corejudge.StatusError
	var ue *url.Error
	switch {
	case err == nil:
		return true, "The key works: TypeSafe answered one question with it."
	case errors.As(err, &se) && (se.Status == 401 || se.Status == 403):
		return false, fmt.Sprintf("TypeSafe refused the key (%d). Check that you pasted the whole key and that it has not been revoked.", se.Status)
	case errors.As(err, &se) && se.Status == 429:
		return false, "TypeSafe is limiting requests with this key right now (429). Try again in a minute."
	case errors.As(err, &se):
		return false, fmt.Sprintf("TypeSafe answered %d, so the key was not confirmed. Try again later.", se.Status)
	case errors.Is(err, context.DeadlineExceeded):
		return false, "TypeSafe did not answer in time, so the key was not confirmed. Check this computer's connection and try again."
	case errors.As(err, &ue):
		// The dial's own words (no such host, connection refused) say what
		// to check; the request's URL and method do not.
		return false, "TypeSafe could not be reached, so the key was not confirmed: " + ue.Err.Error() + "."
	default:
		return false, "TypeSafe's answer was not one AI Review can use, so the key was not confirmed: " + err.Error()
	}
}
