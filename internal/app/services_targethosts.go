package app

// Target hosts in the run setup. A host question shows its address field,
// the hosts this computer's runs reached before, and whether a sign-in is
// saved for the account and host; the sign-in given there (a key file from
// the native picker, a password, a sudo password) goes to the run for this
// run alone, or is saved in the system keychain as the run starts, where the
// CLI and a schedule's fire read it too. A host key no known_hosts file
// holds is put to the person in a native dialog when the run starts. The
// calls that carry a sign-in take it inward only, and none returns it
// (internal/run/targethosts.go).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// TargetHostView is a host question's answer as the setup shows it: what
// is wrong with it, or the account and host it names, and whether a
// sign-in is saved for them. Never a credential.
type TargetHostView struct {
	Problem string `json:"problem,omitempty"`
	// Answer is user@host[:port], the account filled in where none was
	// named.
	Answer      string `json:"answer,omitempty"`
	User        string `json:"user,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	Account     string `json:"account,omitempty"`
	Saved       bool   `json:"saved"`
	KeychainOff bool   `json:"keychainOff"`
	KeyStoreErr string `json:"keyStoreErr,omitempty"`
}

// TargetHostStatus reads a host question's answer with the CLI's own
// parser and says whether a sign-in is saved for it. Read-only; it reaches
// no host.
func (r *RunAPI) TargetHostStatus(answer string) *TargetHostView {
	st := run.GetTargetHostStatus(answer)
	return &TargetHostView{
		Problem: st.Problem, Answer: st.Answer, User: st.User, Host: st.Host, Port: st.Port, Account: st.Account,
		Saved: st.Saved, KeychainOff: st.KeychainOff, KeyStoreErr: st.KeyStoreErr,
	}
}

// ForgetHostSignIn removes the sign-in saved for the account and host of
// answer, and from the keys sealed beside the schedules that run while the
// person is signed out (the CLI seals it there).
func (r *RunAPI) ForgetHostSignIn(answer string) (*TargetHostView, error) {
	if err := run.ForgetHostSignIn(answer); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.TargetHostStatus(answer), nil
}

// RecentHosts are the target hosts this computer's runs reached, newest
// first: addresses and account names, never a credential.
func (r *RunAPI) RecentHosts() []string { return run.RecentHosts() }

// ForgetRecentHost takes a host off this computer's recent hosts. Its
// saved sign-in, if any, stays.
func (r *RunAPI) ForgetRecentHost(answer string) ([]string, error) {
	if err := run.ForgetRecentHost(answer); err != nil {
		return nil, err
	}
	return run.RecentHosts(), nil
}

// ChooseHostKeyFile opens the native file dialog on ~/.ssh, hidden files
// shown, for a target host's private key, and returns the path ("" when
// cancelled). The pick is recorded apart from the run's input files: a key
// chosen here signs in to a target host and is never an input.
func (r *RunAPI) ChooseHostKeyFile(host string) (string, error) {
	title := "Choose the private key to sign in with"
	if host != "" {
		title = "Choose the private key for " + host
	}
	opts := pickOptions{Title: title, ShowHidden: true}
	if home, err := os.UserHomeDir(); err == nil {
		if st, err := os.Stat(filepath.Join(home, ".ssh")); err == nil && st.IsDir() {
			opts.Directory = filepath.Join(home, ".ssh")
		}
	}
	pick := r.app.pickKeyFile
	if pick == nil {
		pick = nativePickFile
	}
	p, err := pick(r.app.ctx, opts)
	if err != nil || p == "" {
		return "", err
	}
	r.app.keyFiles.mark(p)
	return p, nil
}

// nativePickFile is the OS file dialog for one file.
var nativePickFile = nativeOpenFile

// keyFilePicks records the key files the native picker returned.
type keyFilePicks struct {
	mu    sync.Mutex
	paths map[string]bool
}

func (k *keyFilePicks) mark(p string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.paths == nil {
		k.paths = map[string]bool{}
	}
	k.paths[filepath.Clean(p)] = true
}

// require refuses a key file the picker did not return: the page could
// otherwise name any file on this computer to be sent to a host it names.
func (k *keyFilePicks) require(what, p string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.paths[filepath.Clean(p)] {
		return nil
	}
	return fmt.Errorf("%s %q was not chosen through the file dialog; choose it there", what, p)
}

// trustHostKey puts a target host's unknown key to the person in a native
// dialog, No by default: prompt is the CLI's, with the key's type and
// SHA256 fingerprint.
func (a *App) trustHostKey(prompt string) (bool, error) {
	if a.confirm == nil {
		return false, errors.New("no dialog is available to accept a host key")
	}
	return a.confirm(a.ctx, "Accept this host key?", prompt)
}

// savesHostSignIn reports whether a run given these sign-ins saves one.
func savesHostSignIn(given map[string]run.HostSignIn) bool {
	for _, s := range given {
		if s.Save {
			return true
		}
	}
	return false
}
