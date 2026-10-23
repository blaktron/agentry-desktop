package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// liveKeychainEnv asks for the keychain's live test, against this
// computer's own system keychain: the macOS login keychain, Windows'
// Credential Manager, or the Secret Service on Linux.
const liveKeychainEnv = "AGENTRY_LIVE_KEYCHAIN"

// TestLiveStoredSecretInTheSystemKeychain drives the secret card's bindings
// against the real keychain: a secret is set (its entry present in the
// keychain), used as a segment reads it, switched off, and removed (its
// entry gone). Run it alone, since the switch also leaves the keychain real
// for every other test of this package:
//
//	AGENTRY_LIVE_KEYCHAIN=1 go test ./internal/app -run TestLiveStoredSecretInTheSystemKeychain -v
//
// The secret is a random value under a name of its own, never printed, and
// the keychain entry is deleted however the test ends.
func TestLiveStoredSecretInTheSystemKeychain(t *testing.T) {
	if os.Getenv(liveKeychainEnv) != "1" {
		t.Skip("set " + liveKeychainEnv + "=1 to run the stored secret against this computer's keychain")
	}
	rig := newPersistentRig(t)
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	value := hex.EncodeToString(b)
	name := fmt.Sprintf("LIVE_CHECK_%d", time.Now().UnixNano()%1_000_000)
	keys := persistent.KeychainSecrets{}
	account := persistent.SecretAccount("assistant", name)
	t.Cleanup(func() { _ = keys.Delete(account) })

	if _, err := rig.p.SetSecret("assistant", name, value); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, err := keys.Get(account); err != nil || got != value {
		t.Fatalf("after the set the keychain entry %s is not the value set (%v)", account, err)
	}
	t.Logf("set: the keychain holds %s", account)

	got, err := persistent.ReadSecrets(keys, "assistant", []string{name})
	if err != nil || got[name] != value {
		t.Fatalf("a segment's read of %s: %v", name, err)
	}
	t.Logf("used: a segment's read of the agent's secrets gives %s its value", name)

	v, err := rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: persistent.SecretEntryID(name), On: false}})
	if err != nil {
		t.Fatalf("switch off: %v", err)
	}
	if e, ok := rosterEntry(v, "Your secret "+name); !ok || e.On {
		t.Fatalf("after the switch the entry is %+v (found %v)", e, ok)
	}
	t.Logf("switched off: the card reads %q", "Off")

	if _, err := rig.p.RemoveSecret("assistant", name); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Fatalf("after the remove the keychain still holds %s", account)
	}
	t.Logf("removed: the keychain no longer holds %s", account)
}
