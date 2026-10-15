// Target hosts in the desktop's run setup. A QuickAgent that declares a
// target host asks, with an intake question of type host, for a machine
// the person names when it runs, [user@]host[:port], and the run reaches
// it over SSH from the host link, never from the agent's containers. The
// run setup takes the address and the sign-in (a key file chosen in the
// native picker, with its passphrase, or a password, and an optional sudo
// password), or uses the sign-in saved for that account and host in the
// system keychain, as service agentry, account host:<user>@<host>:<port>:
// the entry `agentry run` and a schedule's fire read. Each host is readied
// by the CLI's own targethost.Prepare before anything of the run starts.
// Its host key is looked up in the person's known_hosts, then the runner
// home's; a key on neither file is shown to the person in a native dialog
// (its type and SHA256 fingerprint, in the CLI's words), and accepting it
// keeps it in the runner home's file; a changed key refuses the run with
// both fingerprints. A sign-in crosses the app's IPC inward only, and no
// surface returns it.
package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"
)

// HostSignIn is the sign-in the run setup was given for one target host,
// for this run: a key file a native picker returned and its passphrase,
// or a password, and a sudo password. Empty fields leave each to the
// sign-in saved for the host. It travels inward only.
type HostSignIn struct {
	KeyFile    string `json:"keyFile,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Password   string `json:"password,omitempty"`
	Sudo       string `json:"sudo,omitempty"`
	// Save keeps what the run signed in with in the system keychain for
	// the account and host, where later runs, the CLI and a schedule's
	// fire read it (a key as its file's path, never its bytes).
	Save bool `json:"save,omitempty"`
}

// signsIn reports whether s gives a way to sign in (a key or a password),
// rather than only a sudo password or nothing.
func (s HostSignIn) signsIn() bool {
	return strings.TrimSpace(s.KeyFile) != "" || s.Password != ""
}

// TargetHostStatus is what the run setup shows for a host question's
// answer. It never carries a credential.
type TargetHostStatus struct {
	// Problem is why the answer is not a host the run would take, in the
	// CLI's words; "" when it is. The rest is empty when there is one.
	Problem string
	// Answer is the answer as ssh takes it, the account filled in where
	// none was named (this computer's user name, as ssh's own default).
	Answer string
	User   string
	Host   string
	Port   int
	// Account is the keychain account a sign-in for it is saved under.
	Account string
	// Saved: a sign-in is saved for it (read, never returned).
	Saved bool
	// KeychainOff: AGENTRY_NO_KEYRING is set, so no sign-in is read or
	// saved.
	KeychainOff bool
	// KeyStoreErr is why the keychain could not be asked.
	KeyStoreErr string
}

// GetTargetHostStatus reads a host question's answer with the CLI's own
// parser and says whether a sign-in is saved for it. It reaches no host:
// the address is resolved and the host key read when the run starts.
func GetTargetHostStatus(answer string) *TargetHostStatus {
	a, err := targethost.ParseAnswer(answer)
	if err != nil {
		return &TargetHostStatus{Problem: err.Error()}
	}
	st := &TargetHostStatus{Answer: a.String(), User: a.User, Host: a.Host, Port: a.Port, Account: a.Account(), KeychainOff: modelkey.Disabled()}
	if st.KeychainOff {
		return st
	}
	saved, err := targethost.HasSavedSignIn(a)
	if err != nil {
		st.KeyStoreErr = err.Error()
	}
	st.Saved = saved
	return st
}

// ForgetHostSignIn removes the sign-in saved for the account and host of
// answer. One with none is not an error.
func ForgetHostSignIn(answer string) error {
	a, err := targethost.ParseAnswer(answer)
	if err != nil {
		return err
	}
	if modelkey.Disabled() {
		return errors.New("the system keychain is off (AGENTRY_NO_KEYRING)")
	}
	err = keyring.Delete(modelkey.Service, a.Account())
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("remove the sign-in saved for %s from the system keychain: %w", a, err)
	}
	return nil
}

// prepareTargetHost readies one target host; a variable so a test can
// stand in for the network.
var prepareTargetHost = targethost.Prepare

// errNoSignIn is the run setup's refusal of a target host it was given no
// sign-in for, where none is saved: the run asks nothing at its start.
var errNoSignIn = errors.New("no sign-in was given for it in the run setup, and none is saved for it on this computer: choose a key file or type a password")

// setupAsker is targethost.Asker for a run the desktop starts. The setup
// already took the sign-in, so the run prompts for none: a sign-in it
// would ask for is refused (errNoSignIn), and a passphrase or a sudo
// password it would ask for is none. What it puts to the person is a host
// key no known_hosts file holds, in a native dialog the page cannot
// answer (trust), in the CLI's own words: the key's type and SHA256
// fingerprint.
type setupAsker struct {
	trust func(prompt string) (bool, error)
}

func (setupAsker) Ask(string, string) (string, error) { return "", errNoSignIn }

func (setupAsker) AskSecret(string) (string, error) { return "", nil }

// Confirm puts only a host key's question to the dialog, which is titled
// for one: any other question the CLI might ask (saving a sign-in it
// prompted for, which the setup never lets it do) is answered no.
func (s setupAsker) Confirm(prompt string, _ bool) (bool, error) {
	if !strings.Contains(prompt, "host key") || !strings.Contains(prompt, "SHA256:") {
		return false, nil
	}
	return s.trust(prompt)
}

// hostFlags are the setup's sign-ins as the CLI's flags take them: a key
// file's path, and the names of the values held in getenv. Nothing is
// set in the process's environment.
func hostFlags(given map[string]HostSignIn) (targethost.Flags, func(string) string) {
	f := targethost.Flags{Key: map[string]string{}, Passphrase: map[string]string{}, Password: map[string]string{}, Sudo: map[string]string{}}
	env := map[string]string{}
	put := func(m map[string]string, id, kind, v string) {
		name := "AGENTRY_DESKTOP_HOST_" + kind + "_" + id
		m[id], env[name] = name, v
	}
	for id, s := range given {
		switch {
		case strings.TrimSpace(s.KeyFile) != "":
			f.Key[id] = strings.TrimSpace(s.KeyFile)
			if s.Passphrase != "" {
				put(f.Passphrase, id, "PASSPHRASE", s.Passphrase)
			}
		case s.Password != "":
			put(f.Password, id, "PASSWORD", s.Password)
		}
		if s.Sudo != "" {
			put(f.Sudo, id, "SUDO", s.Sudo)
		}
	}
	return f, func(name string) string { return env[name] }
}

// hostReadiness is what readying a run's target hosts reads besides the
// answers.
type hostReadiness struct {
	pol  *policy.Policy
	home string
	// given are the setup's sign-ins, by question id.
	given map[string]HostSignIn
	// picked judges a key file's path: one a native picker returned, or it is
	// refused. nil judges nothing (a test).
	picked func(what, path string) error
	// trust asks the person whether to accept a host key no file holds;
	// nil is no one to ask, and such a key refuses the run.
	trust func(prompt string) (bool, error)
	say   func(string)
	// keep saves every sign-in in the keychain, whatever the setup's Save
	// says: a schedule's fires read it there.
	keep bool
}

// readyTargetHosts readies each target host ids names, in that order, as
// `agentry run` does: its address resolved and checked against the policy,
// its host key checked, and its sign-in taken from the setup, else the
// keychain. A refusal is a policy refusal with the CLI's reason,
// host-refused (exit 11). A sign-in for an id that is no target host of
// the run refuses it, so a credential is never dropped without a word.
func readyTargetHosts(ctx context.Context, a *intake.Answers, ids []string, r hostReadiness) error {
	for id, s := range r.given {
		if (s.signsIn() || s.Sudo != "") && !slices.Contains(ids, id) {
			return engine.Classify(engine.ExitCLI, fmt.Errorf("a sign-in was given for %q, which is not one of this run's target hosts", id))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if r.picked != nil {
		for _, id := range ids {
			if p := strings.TrimSpace(r.given[id].KeyFile); p != "" {
				if err := r.picked("the key file for target host "+id, p); err != nil {
					return engine.Classify(engine.ExitUsage, err)
				}
			}
		}
	}
	say := r.say
	if say == nil {
		say = func(string) {}
	}
	flags, getenv := hostFlags(r.given)
	o := targethost.Options{
		Hosts: r.pol.Hosts, KnownHosts: targethost.DefaultKnownHostsFiles(r.home), Flags: flags,
		Getenv: getenv, Say: say,
	}
	if r.trust != nil {
		o.Ask = setupAsker{trust: r.trust}
	}
	for _, id := range ids {
		h := a.Hosts[id]
		if h == nil {
			// Never answered: the engine refuses it by name.
			continue
		}
		o.Keep = r.keep || r.given[id].Save
		ready, err := prepareTargetHost(ctx, id, h.Answer, o)
		if err != nil {
			var refusal *targethost.Refusal
			switch {
			case errors.As(err, &refusal):
				return engine.Classify(engine.ExitPolicy, receipt.WithReason(err, receipt.ReasonHostRefused, refusal.Detail, id))
			case errors.Is(err, errNoSignIn):
				return engine.Classify(engine.ExitPolicy, receipt.WithReason(fmt.Errorf("target host %s (%s): %w", id, h.Answer, err), receipt.ReasonHostRefused, receipt.HostCredential, id))
			}
			return engine.Classify(engine.ExitCLI, err)
		}
		// The receipt says where the sign-in came from in the CLI's words.
		// The desktop has no flags: what the setup was given was given at
		// this run's start.
		if ready.Credential.From == targethost.FromFlag {
			ready.Credential.From = targethost.FromPrompt
		}
		if ready.Credential.SudoFrom == targethost.FromFlag {
			ready.Credential.SudoFrom = targethost.FromPrompt
		}
		a.Hosts[id] = ready
		say(fmt.Sprintf("target host %s: %s at %s, host key %s (%s)", id, ready.Answer, ready.Address, ready.HostKey.Fingerprint(), ready.HostKey.From))
		if err := rememberHost(ready.Answer); err != nil {
			say("warning: the host could not be added to this computer's recent hosts: " + err.Error())
		}
	}
	return nil
}

// TargetHostIDs are the ids of m's target hosts, in its order.
func TargetHostIDs(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	return m.Capabilities.TargetHosts
}

// ScheduleHosts is a schedule being made, as ReadyScheduleHosts readies
// its target hosts.
type ScheduleHosts struct {
	Registry, Ref string
	// Answers are the schedule's intake answers, by question id.
	Answers map[string]string
	// Given are the setup's sign-ins; Picked, Trust and Say are as a
	// run's (hostReadiness).
	Given  map[string]HostSignIn
	Picked func(what, path string) error
	Trust  func(prompt string) (bool, error)
	Say    func(string)
	// Before is asked once the release is known to declare a target
	// host, before anything is readied or kept: the agentry command that
	// makes the schedule must be one that knows them.
	Before func() error
}

// ReadyScheduleHosts readies, as a schedule on this computer is made, each
// target host the release asks for: the host key is accepted now, in the
// native dialog where no file holds it, and the sign-in, from the setup or
// the keychain, is kept in the keychain, where the schedule's fires read
// it. The agentry command that then makes the schedule finds both and pins
// the key. It reports how many hosts it readied, each sign-in now in the
// keychain: a release with none readies nothing, and a sign-in given for
// one refuses.
func ReadyScheduleHosts(ctx context.Context, s ScheduleHosts) (int, error) {
	sig, err := ResolveSignature(ctx, s.Registry, s.Ref, false)
	if err != nil {
		return 0, err
	}
	ids := TargetHostIDs(sig.Manifest)
	if len(ids) > 0 && s.Before != nil {
		if err := s.Before(); err != nil {
			return 0, err
		}
	}
	a := &intake.Answers{Hosts: map[string]*targethost.Host{}}
	for _, id := range ids {
		v := strings.TrimSpace(s.Answers[id])
		if v == "" {
			return 0, engine.Classify(engine.ExitUsage, fmt.Errorf("target host %s: a schedule is given its server when it is made: answer it", id))
		}
		ans, err := targethost.ParseAnswer(v)
		if err != nil {
			return 0, engine.Classify(engine.ExitUsage, fmt.Errorf("target host %s: %w", id, err))
		}
		a.Hosts[id] = &targethost.Host{ID: id, Answer: ans}
	}
	if len(ids) == 0 {
		return 0, readyTargetHosts(ctx, a, ids, hostReadiness{given: s.Given})
	}
	home, err := Home()
	if err != nil {
		return 0, err
	}
	pf, err := policy.Resolve("")
	if err != nil {
		return 0, err
	}
	pol, _, err := policy.EnsureDefault(pf.Path)
	if err != nil {
		return 0, fmt.Errorf("operator policy: %w", err)
	}
	if err := readyTargetHosts(ctx, a, ids, hostReadiness{pol: pol, home: home, given: s.Given, picked: s.Picked, trust: s.Trust, say: s.Say, keep: true}); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// RefuseHostedTargetHosts refuses a run or a schedule In Agentry of a
// release that declares a target host: a machine the person names is
// reached over SSH only from their own computer. The run host's CLI
// refuses it again (engine.HostedTargetHostRefusal).
func RefuseHostedTargetHosts(m *manifest.Manifest) error {
	if ids := TargetHostIDs(m); len(ids) > 0 {
		return fmt.Errorf("this QuickAgent works a server you name (%s) over SSH, which only a run on this computer does: a run In Agentry is not offered for it", strings.Join(ids, ", "))
	}
	return nil
}
