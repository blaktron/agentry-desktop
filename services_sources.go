package main

// An input source's access token in the run setup (plan input-sources.md
// §5, D5, D13; agentry-desktop#794). A repository or url question shows its
// address fields and, beside them, whether a key is saved for the address's
// host in the system keychain; a token typed there goes to the run for this
// run alone, or is saved for the host, where the CLI and a schedule's fire
// read it too. The calls that carry a token take it inward only, and none
// returns it (internal/run/sourcekeys.go).

import (
	"github.com/blaktron/agentry-desktop/internal/run"
)

// SourceKeyView is a source question's answer as the setup shows it: what
// is wrong with it, or its host and whether a key is saved for that host.
// Never a token.
type SourceKeyView struct {
	Problem     string `json:"problem,omitempty"`
	Host        string `json:"host,omitempty"`
	Shown       string `json:"shown,omitempty"`
	Saved       bool   `json:"saved"`
	KeychainOff bool   `json:"keychainOff"`
	KeyStoreErr string `json:"keyStoreErr,omitempty"`
}

// SourceKeyStatus checks a source question's answer with the CLI's own
// parser (qtype is the question's type, repository or url) and says
// whether a key is saved for its host. Read-only.
func (r *RunAPI) SourceKeyStatus(qtype, answer string) (*SourceKeyView, error) {
	st, err := run.GetSourceKeyStatus(qtype, answer)
	if err != nil {
		return nil, err
	}
	return &SourceKeyView{Problem: st.Problem, Host: st.Host, Shown: st.Shown, Saved: st.Saved, KeychainOff: st.KeychainOff, KeyStoreErr: st.KeyStoreErr}, nil
}

// SaveSourceKey saves token in the system keychain for the host of
// answer, replacing a key saved there before. The host is the CLI's parse
// of the answer, never a name the page chose. It holds no job lock: the
// key is read when a run is prepared, so a run under way keeps the token
// it started with.
//
// A schedule that runs while the person is signed out has the key sealed
// beside it (agentry-cli#854, DR-84), so a save re-seals them, as every
// other key change does.
func (r *RunAPI) SaveSourceKey(qtype, answer, token string) (*SourceKeyView, error) {
	if err := run.SaveSourceKey(qtype, answer, token); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SourceKeyStatus(qtype, answer)
}

// ForgetSourceKey removes the key saved for the host of answer, and from
// the keys sealed beside the schedules that run while the person is
// signed out.
func (r *RunAPI) ForgetSourceKey(qtype, answer string) (*SourceKeyView, error) {
	if err := run.ForgetSourceKey(qtype, answer); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SourceKeyStatus(qtype, answer)
}
