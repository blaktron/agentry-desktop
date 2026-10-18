// Package destinations is the desktop's side of the places a run's
// deliverables may be sent. The CLI owns them: a destination's name, address
// and "On my network" tick are in destinations.yaml in the CLI's configuration
// folder, and its signing secret and optional Authorization value are entries
// of the system keychain under the CLI's service, so `agentry run
// --destination` and a schedule's fire read exactly what this app saves.
//
// A credential crosses the app's IPC inward only: Save takes one, and no view
// carries one back, nor a path or query of an address (which may hold the
// receiver's own token).
package destinations

import (
	"errors"
	"fmt"
	"slices"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/runnerimage/send"
)

// View is a saved destination as the page draws it. It never carries a
// credential, a path or a query.
type View struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Where is the scheme, host and port the delivery reaches.
	Where       string `json:"where"`
	OnMyNetwork bool   `json:"onMyNetwork"`
	// HasSecret and HasAuthorization say the keychain holds the credential
	// (read, never returned). A destination without a signing secret cannot
	// be sent to.
	HasSecret        bool `json:"hasSecret"`
	HasAuthorization bool `json:"hasAuthorization"`
	// DefaultFor are the QuickAgents (namespace/name) it is in the default set of.
	DefaultFor []string `json:"defaultFor"`
}

// Status is the list and whether the keychain can be used at all.
type Status struct {
	Destinations []View `json:"destinations"`
	// KeychainOff: AGENTRY_NO_KEYRING is set, so no credential is read or kept
	// and no destination can be saved.
	KeychainOff bool `json:"keychainOff"`
}

// Input is a destination the page saves.
type Input struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	OnMyNetwork bool   `json:"onMyNetwork"`
	// Secret signs each body; empty keeps the saved one. Authorization is the
	// value of an Authorization header; empty keeps the saved one unless
	// ClearAuthorization says to remove it, and is dropped when the address
	// names another host than the saved one.
	Secret             string `json:"secret"`
	Authorization      string `json:"authorization"`
	ClearAuthorization bool   `json:"clearAuthorization"`
}

// Load reads the saved destinations.
func Load() (*Status, error) {
	s, err := destination.Load()
	if err != nil {
		return nil, err
	}
	st := &Status{Destinations: []View{}, KeychainOff: modelkey.Disabled()}
	for _, d := range s.Destinations {
		v := View{Name: d.Name, Kind: d.Kind, Where: d.Where(), OnMyNetwork: d.OnMyNetwork,
			HasSecret: destination.HasSecret(d.Name), HasAuthorization: destination.HasAuthorization(d.Name), DefaultFor: []string{}}
		for agent, names := range s.Defaults {
			if slices.Contains(names, d.Name) {
				v.DefaultFor = append(v.DefaultFor, agent)
			}
		}
		slices.Sort(v.DefaultFor)
		st.Destinations = append(st.Destinations, v)
	}
	return st, nil
}

// Check says why an address cannot be a destination, in the delivery step's
// own words, or "" when it can.
func Check(url string, onMyNetwork bool) string {
	if _, err := send.CheckURL(url, onMyNetwork); err != nil {
		return err.Error()
	}
	return ""
}

// Save keeps a destination: refused where the CLI would refuse it, the
// credentials in the keychain and the rest in the CLI's file. A change that
// fails leaves no credential of a destination that was never saved.
func Save(in Input) error {
	d := destination.Destination{Name: in.Name, Kind: destination.KindWebhook, URL: in.URL, OnMyNetwork: in.OnMyNetwork}
	if err := d.Check(); err != nil {
		return err
	}
	if modelkey.Disabled() {
		return errors.New("the system keychain is off (AGENTRY_NO_KEYRING): a destination's credentials are kept there alone")
	}
	s, err := destination.Load()
	if err != nil {
		return err
	}
	prev, existed := s.Find(in.Name)
	secret := in.Secret
	if secret == "" && (!existed || !destination.HasSecret(in.Name)) {
		return errors.New("a destination needs a signing secret")
	}
	// What was kept before, for a change that fails half way.
	var oldSecret, oldAuth string
	if existed {
		var kerr error
		if oldSecret, oldAuth, kerr = destination.Secrets(in.Name); kerr != nil {
			return kerr
		}
	}
	auth := in.Authorization
	if auth == "" && existed && !in.ClearAuthorization && sameReceiver(prev, d) {
		// A saved value stays with the receiver it was saved for: the same
		// scheme, host and port under the same rule for an address. Another
		// receiver, or the same one over a weaker scheme or a wider rule,
		// never gets it.
		auth = oldAuth
	}
	if !destination.ValidSecretValues(secret, auth) {
		return errors.New("a credential holds a control character or is not valid text")
	}
	if err := s.Upsert(d); err != nil {
		return err
	}
	undo := func() {
		if !existed {
			_ = destination.DeleteSecrets(in.Name)
			return
		}
		_ = destination.SaveSecrets(in.Name, oldSecret, oldAuth)
	}
	if err := destination.SaveSecrets(in.Name, secret, auth); err != nil {
		undo()
		return err
	}
	if err := s.Save(); err != nil {
		undo()
		return err
	}
	return nil
}

// sameReceiver says two destinations reach the same host and port by the same
// scheme under the same rule for an address.
func sameReceiver(a, b destination.Destination) bool {
	ta, err1 := send.CheckURL(a.URL, a.OnMyNetwork)
	tb, err2 := send.CheckURL(b.URL, b.OnMyNetwork)
	return err1 == nil && err2 == nil && ta.HostPort == tb.HostPort && ta.URL.Scheme == tb.URL.Scheme && a.OnMyNetwork == b.OnMyNetwork
}

// Remove deletes a destination, its credentials and its place in every default
// set.
func Remove(name string) error {
	s, err := destination.Load()
	if err != nil {
		return err
	}
	if !s.Delete(name) {
		return fmt.Errorf("no destination is saved under %q", name)
	}
	if err := s.Save(); err != nil {
		return err
	}
	return destination.DeleteSecrets(name)
}

// SetDefault sets the destinations a QuickAgent's runs start with, which the
// run setup preselects; none clears it.
func SetDefault(agent string, names []string) error {
	s, err := destination.Load()
	if err != nil {
		return err
	}
	if err := s.SetDefault(agent, names); err != nil {
		return err
	}
	return s.Save()
}

// DefaultFor is the default set of a QuickAgent (a release ref, namespace/name
// or namespace/name@version).
func DefaultFor(ref string) []string {
	s, err := destination.Load()
	if err != nil {
		return []string{}
	}
	out := s.DefaultsFor(ref)
	if out == nil {
		return []string{}
	}
	return out
}

// Names are the destinations a run of a QuickAgent may be given: saved and with
// a signing secret, each once. A name that is not one refuses.
func Resolve(names []string) ([]destination.Resolved, error) { return destination.Resolve(names) }

// Sendable are the saved destinations that hold a signing secret, by name: the
// ones a delivery can be made to. The store is read once.
func Sendable() map[string]bool {
	out := map[string]bool{}
	s, err := destination.Load()
	if err != nil {
		return out
	}
	for _, d := range s.Destinations {
		if destination.HasSecret(d.Name) {
			out[d.Name] = true
		}
	}
	return out
}

// CanSend says a destination is saved and holds a signing secret, so a
// delivery to it can be made.
func CanSend(name string) bool {
	s, err := destination.Load()
	if err != nil {
		return false
	}
	_, ok := s.Find(name)
	return ok && destination.HasSecret(name)
}
