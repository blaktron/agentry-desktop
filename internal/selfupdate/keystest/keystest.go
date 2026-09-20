// Package keystest makes update-signing keys for tests.
//
// It exists because the two packages that need them — internal/selfupdate and
// internal/cmd/signrelease — otherwise grow the same twenty lines under two
// names, and because the release workflow's signing step has no secret to
// borrow in CI: everything that would need real key material to reproduce is
// reproduced here instead.
package keystest

import (
	"crypto/rand"
	"fmt"
	"testing"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

// Key makes a key pair and the keys.json entry carrying its public half, so a
// test exercises the same path a committed key takes.
func Key(t *testing.T, name string, retired bool) (minisign.PrivateKey, selfupdate.KeyEntry) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating a test key: %v", err)
	}
	return priv, selfupdate.KeyEntry{
		Name:      name,
		PublicKey: pub.String(),
		KeyID:     fmt.Sprintf("%016X", pub.ID()),
		Retired:   retired,
	}
}

// Set builds a key set from entries. No entries means a build that carries no
// key, which is a state worth testing on its own.
func Set(t *testing.T, entries ...selfupdate.KeyEntry) *selfupdate.KeySet {
	t.Helper()
	file := &selfupdate.KeyFile{Keys: entries}
	set, err := file.KeySet()
	if err != nil {
		t.Fatalf("building a test key set: %v", err)
	}
	return set
}

// File renders entries as the bytes keys.json would hold, for a test that needs
// to drive the parser rather than the set.
func File(t *testing.T, entries ...selfupdate.KeyEntry) []byte {
	t.Helper()
	data, err := (&selfupdate.KeyFile{Keys: entries}).Marshal()
	if err != nil {
		t.Fatalf("marshalling a test key file: %v", err)
	}
	return data
}
