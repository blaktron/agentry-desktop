// An input source's access token on this computer. A QuickAgent asks for a
// repository or a file at a web address with an intake question of that type,
// and the run setup asks beside it for an access token, which the run's fetch
// step alone uses (the CLI's engine writes it to that step's stdin and nowhere
// else). A token can be saved for its host in the OS keychain, as the entry
// source:<host> under the CLI's service: the CLI's `agentry run` and a
// schedule's fire read the same entry (pkg/runner/sources), so a key saved here
// is the one a scheduled run uses. The token crosses the app's IPC inward only,
// and no surface returns it.
package run

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
)

// SourceKeyStatus is what the run setup shows for a source question's
// answer. It never carries a token.
type SourceKeyStatus struct {
	// Problem is why the answer cannot be fetched, in the CLI's words
	// (not https, a port, credentials in the address, a bad ref); "" when
	// it can. The rest is empty when there is a problem.
	Problem string
	// Host is the one host the fetch reaches, and the saved key's name.
	Host string
	// Shown is the address as a person reads it: a query is "?…".
	Shown string
	// Saved: the keychain holds a key for Host (read, never returned).
	Saved bool
	// KeychainOff: AGENTRY_NO_KEYRING is set, so no key is read or saved.
	KeychainOff bool
	// KeyStoreErr is why the keychain could not be asked.
	KeyStoreErr string
}

// parseSource reads an answer as the kind its question's type asks for,
// with the CLI's own parser, so the setup refuses what the run would.
func parseSource(qtype, answer string) (sources.Source, error) {
	t := manifest.IntakeType(qtype)
	if !t.IsSource() {
		return sources.Source{}, fmt.Errorf("%q is not an input source's question type", qtype)
	}
	k, ok := sources.Lookup(t)
	if !ok {
		return sources.Source{}, sources.Unfetched("", t)
	}
	if strings.TrimSpace(answer) == "" {
		return sources.Source{}, errors.New("give the address first")
	}
	return k.Parse(answer)
}

// SourceShown checks a source question's answer with the CLI's own parser
// and returns the address as a run shows it, or why it is refused.
func SourceShown(qtype, answer string) (string, error) {
	s, err := parseSource(qtype, answer)
	if err != nil {
		return "", err
	}
	return s.Shown(), nil
}

// GetSourceKeyStatus checks an answer and says whether a key is saved for
// its host. An answer the CLI would refuse is a Problem, not an error; an
// error is a question type that asks for no source.
func GetSourceKeyStatus(qtype, answer string) (*SourceKeyStatus, error) {
	if !manifest.IntakeType(qtype).IsSource() {
		return nil, fmt.Errorf("%q is not an input source's question type", qtype)
	}
	s, err := parseSource(qtype, answer)
	if err != nil {
		return &SourceKeyStatus{Problem: err.Error()}, nil
	}
	st := &SourceKeyStatus{Host: s.Host, Shown: s.Shown(), KeychainOff: modelkey.Disabled()}
	if st.KeychainOff {
		return st, nil
	}
	v, err := sources.SavedToken(s.Host)
	if err != nil {
		st.KeyStoreErr = err.Error()
	}
	st.Saved = v != ""
	return st, nil
}

// SaveSourceKey saves token in the OS keychain for the host of answer.
// The host comes from the CLI's parse of the answer, never from the page,
// so an entry is only ever made for an address a run would fetch.
func SaveSourceKey(qtype, answer, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("paste the access token first")
	}
	s, err := parseSource(qtype, answer)
	if err != nil {
		return err
	}
	return sources.SaveToken(s.Host, token)
}

// ForgetSourceKey removes the key saved for the host of answer. A host
// with none is not an error.
func ForgetSourceKey(qtype, answer string) error {
	s, err := parseSource(qtype, answer)
	if err != nil {
		return err
	}
	if modelkey.Disabled() {
		return errors.New("the system keychain is off (AGENTRY_NO_KEYRING)")
	}
	err = keyring.Delete(modelkey.Service, sources.KeyAccount(s.Host))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("remove the key saved for %s from the system keychain: %w", s.Host, err)
	}
	return nil
}

// giveSourceTokens gives each input source of a run its access token: the
// one the run setup was given for it, for this run, else the key saved for
// its host, else none, and the fetch is anonymous. Unlike the CLI's order
// (sources.ResolveTokens), a token typed for this run comes before the
// saved key: the setup offers "another token for this run" beside a saved
// one. A token for an id that is no source of the run refuses it, so a
// token is never dropped without a word. A keychain that cannot be read is
// said through warn and the source goes on without it.
func giveSourceTokens(a *intake.Answers, given map[string]string, warn func(string)) error {
	srcs := a.SourceList()
	known := map[string]bool{}
	for _, s := range srcs {
		known[s.ID] = true
	}
	for id, tok := range given {
		if strings.TrimSpace(tok) != "" && !known[id] {
			return fmt.Errorf("an access token was given for %q, which is not one of this run's input sources", id)
		}
	}
	for _, s := range srcs {
		if tok := strings.TrimSpace(given[s.ID]); tok != "" {
			s.Token, s.TokenFrom = tok, sources.TokenFromPrompt
			continue
		}
		saved, err := sources.SavedToken(s.Host)
		if err != nil && warn != nil {
			warn(err.Error())
		}
		if saved != "" {
			s.Token, s.TokenFrom = saved, sources.TokenFromKeychain
		}
	}
	return nil
}

// HostedSourceTokens are what a run In Agentry sends as its create's
// sourceTokens: for each input-source question of m the answers give an
// address to, the token typed for this run, else the key saved on this
// computer for the host the CLI's own parse reads from the answer, else
// none, and the fetch on the sandbox host is anonymous (or uses the key
// the account saved in Agentry for that host). The registry holds each
// token in the account's vault for that run alone. A token typed for an id
// that is no answered source refuses the run, as giveSourceTokens does,
// and an answer the CLI would refuse is said here rather than at the host.
// A keychain that cannot be read is said through warn.
func HostedSourceTokens(m *manifest.Manifest, answers, typed map[string]string, warn func(string)) (map[string]string, error) {
	out := map[string]string{}
	if m == nil {
		return out, nil
	}
	answered := map[string]bool{}
	for _, q := range m.Intake {
		answer := strings.TrimSpace(answers[q.ID])
		if !q.Type.IsSource() || answer == "" || browserSite(m, q.ID) {
			continue
		}
		answered[q.ID] = true
		s, err := parseSource(string(q.Type), answer)
		if err != nil {
			return nil, fmt.Errorf("the input source %s: %w", q.ID, err)
		}
		if tok := strings.TrimSpace(typed[q.ID]); tok != "" {
			out[q.ID] = tok
			continue
		}
		saved, err := sources.SavedToken(s.Host)
		if err != nil && warn != nil {
			warn(err.Error())
		}
		if saved != "" {
			out[q.ID] = saved
		}
	}
	for id, tok := range typed {
		if strings.TrimSpace(tok) != "" && !answered[id] {
			return nil, fmt.Errorf("an access token was given for %q, which is not one of this run's input sources", id)
		}
	}
	return out, nil
}

// browserSite says a url question is the site the release's browser opens
// (capabilities.browser.sites): the runner never fetches it, so it takes
// no token and no saved key, In Agentry as on this computer.
func browserSite(m *manifest.Manifest, id string) bool {
	return m.Capabilities.Browser != nil && slices.Contains(m.Capabilities.Browser.Sites, id)
}

// HostedScheduleKeys are the keys a schedule In Agentry saves in the
// account's vault for its fires, by host: for each answered source with a
// token (typed for this schedule, else the key saved on this computer for
// its host), that token. A schedule's fire has no setup to send a one-run
// token from, so it uses the key saved in Agentry for each source's host;
// one with no token here is fetched with the key Agentry already keeps for
// the host, if any, else anonymously. Two sources on one host given
// different tokens are refused: a host keeps one key.
func HostedScheduleKeys(m *manifest.Manifest, answers, typed map[string]string, warn func(string)) (map[string]string, error) {
	keys := map[string]string{}
	if m == nil {
		return keys, nil
	}
	tokens, err := HostedSourceTokens(m, answers, typed, warn)
	if err != nil {
		return nil, err
	}
	for _, q := range m.Intake {
		tok, ok := tokens[q.ID]
		if !ok {
			continue
		}
		s, err := parseSource(string(q.Type), strings.TrimSpace(answers[q.ID]))
		if err != nil {
			return nil, err
		}
		if prev, ok := keys[s.Host]; ok && prev != tok {
			return nil, fmt.Errorf("two input sources on %s were given different tokens; Agentry keeps one key for a host", s.Host)
		}
		keys[s.Host] = tok
	}
	return keys, nil
}
