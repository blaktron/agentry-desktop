// Package destinations is the desktop's side of the places a run's
// deliverables may be sent: a webhook, or a bucket of an S3 API. The CLI owns
// them: a destination's name, address (or bucket, prefix, region and endpoint)
// and "On my network" tick are in destinations.yaml in the CLI's configuration
// folder, and its credentials (a webhook's signing secret and optional
// Authorization value, a bucket's access key id and secret access key) are
// entries of the system keychain under the CLI's service, so `agentry run
// --destination` and a schedule's fire read exactly what this app saves.
//
// A credential crosses the app's IPC inward only: Save takes one, and no view
// carries one back, nor a path or query of a webhook's address (which may hold
// the receiver's own token). A bucket's endpoint has neither, so its view
// names it.
package destinations

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/runnerimage/send"
)

// View is a saved destination as the page draws it. It never carries a
// credential, a path or a query.
type View struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Where is the scheme, host and port the delivery reaches; a bucket's is
	// s3://bucket/prefix at the service's scheme, host and port.
	Where       string `json:"where"`
	OnMyNetwork bool   `json:"onMyNetwork"`
	// Bucket, Prefix, Region and Endpoint are a bucket's own fields, which
	// hold no credential, path or query.
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Region   string `json:"region,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	// HasSecret, HasAccessKey and HasAuthorization say the keychain holds the
	// credential (read, never returned): a webhook's signing secret or a
	// bucket's secret access key, a bucket's access key id, and a webhook's
	// Authorization value.
	HasSecret        bool `json:"hasSecret"`
	HasAccessKey     bool `json:"hasAccessKey"`
	HasAuthorization bool `json:"hasAuthorization"`
	// CanSend says a delivery can be made to it: a webhook holds its signing
	// secret, a bucket both halves of its key pair.
	CanSend bool `json:"canSend"`
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
	Name string `json:"name"`
	// Kind is "webhook" (none means a webhook) or "s3".
	Kind string `json:"kind"`
	// URL is a webhook's address.
	URL         string `json:"url"`
	OnMyNetwork bool   `json:"onMyNetwork"`
	// Bucket, Prefix, Region and Endpoint are a bucket's fields; Region is
	// us-east-1 when only an endpoint is given.
	Bucket   string `json:"bucket"`
	Prefix   string `json:"prefix"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
	// Secret signs each webhook body, or is a bucket's secret access key;
	// empty keeps the saved one. AccessKeyID is a bucket's access key id; empty
	// keeps the saved one. A saved credential is kept only while the change
	// leaves the receiver the same (Destination.SameReceiver): another host,
	// another service, a weaker scheme or a wider rule for an address drops it.
	// Authorization is the value of a webhook's Authorization header; empty
	// keeps the saved one under the same rule unless ClearAuthorization says to
	// remove it.
	Secret             string `json:"secret"`
	AccessKeyID        string `json:"accessKeyId"`
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
			Bucket: d.Bucket, Prefix: d.Prefix, Region: d.Region, Endpoint: d.Endpoint,
			HasSecret: destination.HasSecret(d.Name), HasAccessKey: destination.HasAccessKey(d.Name), HasAuthorization: destination.HasAuthorization(d.Name),
			CanSend: canSend(d), DefaultFor: []string{}}
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

// CheckBucket says why a bucket's fields cannot be a destination, in the CLI's
// own words, or "" when they can. The bucket is judged once it is typed; the
// prefix, region and endpoint are judged meanwhile.
func CheckBucket(in Input) string {
	d := bucketOf(in)
	if d.Bucket == "" {
		d.Bucket = "bucket"
	}
	if _, err := send.CheckS3(d.S3Spec()); err != nil {
		return err.Error()
	}
	return ""
}

// bucketOf is the bucket destination the input describes, as the CLI's own
// command would build it: the prefix without its outer slashes, and the region
// us-east-1 when only an endpoint is named.
func bucketOf(in Input) destination.Destination {
	region := strings.TrimSpace(in.Region)
	endpoint := strings.TrimSpace(in.Endpoint)
	if region == "" && endpoint != "" {
		region = "us-east-1"
	}
	return destination.Destination{
		Name: in.Name, Kind: destination.KindS3, Bucket: strings.TrimSpace(in.Bucket), Prefix: strings.Trim(strings.TrimSpace(in.Prefix), "/"),
		Region: region, Endpoint: endpoint, OnMyNetwork: in.OnMyNetwork,
	}
}

// KeepsKeyPair says saving the input as it stands would keep the key pair saved
// under its name: the destination is a bucket now, at the same service, and both
// halves are kept. The page asks so that it can say, as the person edits, that
// another service needs the pair typed again.
func KeepsKeyPair(in Input) bool {
	s, err := destination.Load()
	if err != nil {
		return false
	}
	prev, ok := s.Find(in.Name)
	return ok && prev.SameReceiver(bucketOf(in)) && destination.HasAccessKey(in.Name) && destination.HasSecret(in.Name)
}

// Save keeps a destination: refused where the CLI would refuse it, the
// credentials in the keychain and the rest in the CLI's file. A change that
// fails leaves no credential of a destination that was never saved, and puts
// back the ones a saved destination had.
func Save(in Input) error {
	switch in.Kind {
	case "", destination.KindWebhook:
		return saveWebhook(in)
	case destination.KindS3:
		return saveBucket(in)
	}
	return fmt.Errorf("the kind %q is not one a destination can be (webhook and s3 are)", in.Kind)
}

func saveWebhook(in Input) error {
	d := destination.Destination{Name: in.Name, Kind: destination.KindWebhook, URL: strings.TrimSpace(in.URL), OnMyNetwork: in.OnMyNetwork}
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
	// A secret saved for a bucket is its secret access key, never a webhook's
	// signing secret.
	secret := in.Secret
	if secret == "" && (!existed || prev.Kind != destination.KindWebhook || !destination.HasSecret(in.Name)) {
		return errors.New("a destination needs a signing secret")
	}
	auth := in.Authorization
	if auth == "" && existed && !in.ClearAuthorization && prev.SameReceiver(d) {
		// A saved value stays with the receiver it was saved for: the same
		// scheme, host and port under the same rule for an address. Another
		// receiver, or the same one over a weaker scheme or a wider rule,
		// never gets it.
		_, auth, err = destination.Secrets(in.Name)
		if err != nil {
			return err
		}
	}
	if !destination.ValidSecretValues(secret, auth) {
		return errors.New("a credential holds a control character or is not valid text")
	}
	if err := s.Upsert(d); err != nil {
		return err
	}
	before := destination.Snapshot(in.Name)
	if err := destination.SaveSecrets(in.Name, secret, auth); err != nil {
		destination.Restore(in.Name, before)
		return err
	}
	if err := s.Save(); err != nil {
		destination.Restore(in.Name, before)
		return err
	}
	return nil
}

func saveBucket(in Input) error {
	d := bucketOf(in)
	if err := d.Check(); err != nil {
		return err
	}
	if modelkey.Disabled() {
		return errors.New("the system keychain is off (AGENTRY_NO_KEYRING): a destination's credentials are kept there alone")
	}
	keyID, secret := strings.TrimSpace(in.AccessKeyID), strings.TrimSpace(in.Secret)
	s, err := destination.Load()
	if err != nil {
		return err
	}
	prev, existed := s.Find(in.Name)
	// A pair saved for one service is kept for a changed one only when it is
	// the same service: an access key saved for one is never sent to another.
	keep := existed && prev.SameReceiver(d)
	if keyID == "" && (!keep || !destination.HasAccessKey(in.Name)) {
		return errors.New("a bucket needs an access key id")
	}
	if secret == "" && (!keep || !destination.HasSecret(in.Name)) {
		return errors.New("a bucket needs a secret access key")
	}
	if !destination.ValidS3Credentials(keyID, secret) {
		return errors.New("the access key id or secret access key is not one a request can be signed with")
	}
	if err := s.Upsert(d); err != nil {
		return err
	}
	before := destination.Snapshot(in.Name)
	if existed && !keep {
		// The saved pair belongs to another service: it goes before the new
		// one is written, so a failure half way leaves none of it in place.
		if err := destination.DeleteSecrets(in.Name); err != nil {
			destination.Restore(in.Name, before)
			return err
		}
	}
	if err := destination.SaveS3Credentials(in.Name, keyID, secret); err != nil {
		destination.Restore(in.Name, before)
		return err
	}
	if err := s.Save(); err != nil {
		destination.Restore(in.Name, before)
		return err
	}
	return nil
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

// canSend says a delivery can be made to d: a webhook holds its signing secret,
// a bucket both halves of its key pair.
func canSend(d destination.Destination) bool {
	if d.Kind == destination.KindS3 {
		return destination.HasSecret(d.Name) && destination.HasAccessKey(d.Name)
	}
	return destination.HasSecret(d.Name)
}

// Sendable are the saved destinations a delivery can be made to, by name. The
// store is read once.
func Sendable() map[string]bool {
	out := map[string]bool{}
	s, err := destination.Load()
	if err != nil {
		return out
	}
	for _, d := range s.Destinations {
		if canSend(d) {
			out[d.Name] = true
		}
	}
	return out
}

// CanSend says a destination is saved with the credentials a delivery to it
// needs.
func CanSend(name string) bool {
	s, err := destination.Load()
	if err != nil {
		return false
	}
	d, ok := s.Find(name)
	return ok && canSend(d)
}

// AnyBucket says one of the saved destinations is a bucket. A command that does
// not know buckets refuses the whole destinations file once it holds one.
func AnyBucket() bool {
	s, err := destination.Load()
	if err != nil {
		return false
	}
	return slices.ContainsFunc(s.Destinations, func(d destination.Destination) bool { return d.Kind == destination.KindS3 })
}
