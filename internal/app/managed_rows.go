package app

// Every managed row as Settings shows it. A row is read by its id and its
// JSON value, so the view needs no knowledge of the catalogue beyond what a
// document carries: a part asks for the ids it governs and draws a lock, an
// organization default or nothing.

import (
	"errors"
	"fmt"
	"slices"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// Row states.
const (
	// ManagedLocked is a row the organization fixed: the member cannot
	// change it.
	ManagedLocked = "locked"
	// ManagedDefault is a row the organization set for members who set
	// none of their own.
	ManagedDefault = "default"
)

// ManagedRowView is one managed row that applies on this computer. A row no
// held document sets is not listed: it is not managed.
type ManagedRowView struct {
	// ID is the row's name in a document, such as "malwareCheck".
	ID string `json:"id"`
	// State is ManagedLocked or ManagedDefault.
	State string `json:"state"`
	// Value is the row's value as the JSON text the document holds: a string
	// for a choice, a number for a cap, an array or an object for a set or a
	// source list.
	Value string `json:"value"`
	// Organization is the organization whose document sets the row.
	Organization string `json:"organization"`
	// Applied says the agentry command this app links applies the row to a
	// run. A row it does not apply is still shown, and a run is not held to
	// it.
	Applied bool `json:"applied"`
}

// managedRows lists the rows the held documents set. A lock beats a default;
// among rows of the same strength the first document's wins.
func managedRows(e managed.Effective) []ManagedRowView {
	byID := map[string]ManagedRowView{}
	for i := range e.Documents {
		d := &e.Documents[i]
		for id, s := range d.Document.Settings {
			state := ManagedDefault
			if s.Locked {
				state = ManagedLocked
			}
			cur, seen := byID[id]
			if seen && (cur.State == ManagedLocked || state == ManagedDefault) {
				continue
			}
			byID[id] = ManagedRowView{ID: id, State: state, Value: string(s.Value), Organization: d.Document.Org, Applied: managed.Applies(id)}
		}
	}
	out := make([]ManagedRowView, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b ManagedRowView) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	return out
}

// ManagedRows are the managed rows held on this computer, for the Settings
// parts they govern. The page reads them again on managed:changed.
func (s *SettingsAPI) ManagedRows() []ManagedRowView {
	return managedRows(managedCalls.current())
}

// managedLockOf is the organization that locked the row, and whether one did.
func managedLockOf(id string) (string, bool) {
	for _, r := range managedRows(managedCalls.current()) {
		if r.ID == id && r.State == ManagedLocked {
			return r.Organization, true
		}
	}
	return "", false
}

// refuseIfManagedLock is the refusal of a write to a row an organization
// locked, in the words the part shows, or nil.
func refuseIfManagedLock(id string) error {
	if org, ok := managedLockOf(id); ok {
		return fmt.Errorf("%s is managed by %s and cannot be changed here", id, org)
	}
	return nil
}

// UseManagedMalwareCheck forgets the member's own malware check choice, so
// the organization's default applies. A lock leaves nothing to forget.
func (s *SettingsAPI) UseManagedMalwareCheck() (*RunDefaultsView, error) {
	if err := run.ClearMalwareCheck(); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// rowMalwareCheck is the catalogue's id for the malware check, read as a
// string so the app builds against a core that predates it.
const rowMalwareCheck = "malwareCheck"

// refuseManagedWhere is a setter's refusal of a default place to run that a
// held document's run.where leaves out, in the runner's words, or nil.
func refuseManagedWhere(member string) error {
	if reason := managedCalls.current().WhereRefusal(member); reason != "" {
		return errors.New(reason)
	}
	return nil
}

// refuseManagedBudgets is the refusal of default budgets above a held
// document's locked cap, or none where it sets one, or nil. A budget of 0
// is none.
func refuseManagedBudgets(timeoutSeconds, maxTurns int) error {
	held := managedCalls.current().Budgets(spec.Budgets{TimeoutSeconds: timeoutSeconds, MaxTurns: maxTurns})
	if held.TimeoutSeconds != timeoutSeconds {
		org, _ := managedLockOf(managedsettings.RunTimeout)
		return fmt.Errorf("the timeout is capped at %d seconds by %s's managed settings (%s)", held.TimeoutSeconds, org, managedsettings.RunTimeout)
	}
	if held.MaxTurns != maxTurns {
		org, _ := managedLockOf(managedsettings.RunMaxTurns)
		return fmt.Errorf("the turns are capped at %d by %s's managed settings (%s)", held.MaxTurns, org, managedsettings.RunMaxTurns)
	}
	return nil
}

// The app holds the rows the runner leaves to a client: runs In Agentry and
// hosted schedules ask internal/hostedrun, which refuses what run.where and
// schedules lock off before anything is sent, and Report a bug refuses
// what bugReports forbids. So a lock on either applies here, and is not a
// row this version cannot hold.
func init() { managed.Enforce(managed.ClientRows...) }
