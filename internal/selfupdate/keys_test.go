package selfupdate

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aead.dev/minisign"
)

// newTestKey makes a key pair and the key-set entry that carries its public
// half, so the tests exercise the same path a committed key takes.
func newTestKey(t *testing.T, name string, retired bool) (minisign.PrivateKey, map[string]any) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, map[string]any{
		"name":      name,
		"publicKey": pub.String(),
		"keyID":     fmt.Sprintf("%016X", pub.ID()),
		"retired":   retired,
	}
}

func keySet(t *testing.T, entries ...map[string]any) *KeySet {
	t.Helper()
	if entries == nil {
		entries = []map[string]any{}
	}
	data, err := json.Marshal(map[string]any{"keys": entries})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	set, err := NewKeySet(data)
	if err != nil {
		t.Fatalf("NewKeySet: %v", err)
	}
	return set
}

const sums = "aa1f56ffcb84076209f4e5cf3eb0282c  agentry-desktop-0.1.0-darwin-universal.dmg\n"

// The committed file must parse, and every key in it must be internally
// consistent. This is the test that keeps a bad keys.json out of a release:
// nothing else reads the file until an update check runs on someone's machine.
func TestCommittedKeySetIsValid(t *testing.T) {
	set, err := Carried()
	if err != nil {
		t.Fatalf("the committed keys.json does not parse: %v", err)
	}
	for _, k := range set.Keys() {
		if k.Name == "" {
			t.Error("a committed key has no name")
		}
		if len(k.ID()) != 16 {
			t.Errorf("key %s has a malformed id", k.Name)
		}
	}
	// Not an error today: the key set is empty until the operator loads the
	// first key (agentry-desktop#86). The test says so out loud rather than
	// letting an empty set look like a passing check of something.
	if len(set.Keys()) == 0 {
		t.Log("the committed key set is empty: this build trusts no release, which is correct until scripts/operator/update-signing-key.sh has been run")
	}
}

func TestVerifyAcceptsTheCurrentKey(t *testing.T) {
	priv, entry := newTestKey(t, "prod", false)
	set := keySet(t, entry)

	got, err := set.Verify([]byte(sums), minisign.Sign(priv, []byte(sums)))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Name != "prod" {
		t.Errorf("signed by %q, want %q", got.Name, "prod")
	}
	if got.Retired {
		t.Error("the key reports itself retired and it is not")
	}
}

// The acceptance's "a signature by a retired key is reported as such": it
// verifies — an old release stays checkable — and the caller is told the key is
// retired so it can refuse to install from it.
func TestVerifyReportsARetiredKeyRatherThanRefusingIt(t *testing.T) {
	oldPriv, oldEntry := newTestKey(t, "dev", true)
	_, newEntry := newTestKey(t, "prod", false)
	set := keySet(t, oldEntry, newEntry)

	got, err := set.Verify([]byte(sums), minisign.Sign(oldPriv, []byte(sums)))
	if err != nil {
		t.Fatalf("a retired key must still verify, so an old release stays checkable: %v", err)
	}
	if got.Name != "dev" {
		t.Errorf("signed by %q, want %q", got.Name, "dev")
	}
	if !got.Retired {
		t.Fatal("the retired key did not report itself retired, so a caller would install from it")
	}
	if !strings.Contains(got.String(), "retired") {
		t.Errorf("%q does not say retired, and that string is what a person reads", got.String())
	}
}

// The acceptance's "a signature by an unknown key is refused".
func TestVerifyRefusesAKeyTheBuildDoesNotCarry(t *testing.T) {
	strangerPriv, _ := newTestKey(t, "not-in-the-set", false)
	_, mine := newTestKey(t, "prod", false)
	set := keySet(t, mine)

	_, err := set.Verify([]byte(sums), minisign.Sign(strangerPriv, []byte(sums)))
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Verify err = %v, want ErrUnknownKey", err)
	}
}

func TestVerifyRefusesTamperedSums(t *testing.T) {
	priv, entry := newTestKey(t, "prod", false)
	set := keySet(t, entry)
	sig := minisign.Sign(priv, []byte(sums))

	tampered := strings.Replace(sums, "aa1f", "bb1f", 1)
	_, err := set.Verify([]byte(tampered), sig)
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("Verify err = %v, want ErrBadSignature", err)
	}
	// The message must say the key IS carried, so a person is not sent looking
	// for a key rotation when the bytes are what changed.
	if !strings.Contains(err.Error(), "does carry") {
		t.Errorf("message %q does not distinguish tampered bytes from an unknown key", err)
	}
}

func TestVerifyRefusesAMalformedSignature(t *testing.T) {
	_, entry := newTestKey(t, "prod", false)
	set := keySet(t, entry)

	for _, junk := range []string{"", "not a signature", "untrusted comment: x\nnope\n"} {
		if _, err := set.Verify([]byte(sums), []byte(junk)); err == nil {
			t.Errorf("Verify(%q) accepted a malformed signature", junk)
		}
	}
}

// A build of the open core carries no key and must refuse everything rather
// than fall back to "the checksum matched" (plan §3).
func TestVerifyRefusesEverythingWhenTheBuildCarriesNoKey(t *testing.T) {
	priv, _ := newTestKey(t, "prod", false)
	set := keySet(t)

	_, err := set.Verify([]byte(sums), minisign.Sign(priv, []byte(sums)))
	if !errors.Is(err, ErrNoKeys) {
		t.Fatalf("Verify err = %v, want ErrNoKeys", err)
	}
}

// A signature carries its signer's key id in the clear and nothing protects it,
// so the key id must not be what chooses the key: if it were, writing a trusted
// key's id into a signature made by another key would steer the check. Here the
// signature is made by a stranger but claims the trusted key's id, and it must
// still be refused.
func TestVerifyDoesNotTrustTheKeyIDInTheSignature(t *testing.T) {
	strangerPriv, _ := newTestKey(t, "stranger", false)
	_, mine := newTestKey(t, "prod", false)
	set := keySet(t, mine)

	sig := minisign.Sign(strangerPriv, []byte(sums))
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	trustedID := set.Keys()[0].ID()
	forged, err := forgeKeyID(sig, trustedID)
	if err != nil {
		t.Skipf("could not rewrite the signature's key id: %v", err)
	}
	if _, err := set.Verify([]byte(sums), forged); err == nil {
		t.Fatal("a signature claiming a trusted key's id verified although another key made it")
	}
}

// forgeKeyID rewrites the key id inside a minisign signature, which is what an
// attacker steering a key-id lookup would do. It exists only for the test above.
func forgeKeyID(sig []byte, id string) ([]byte, error) {
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		return nil, err
	}
	var want uint64
	if _, err := fmt.Sscanf(id, "%016X", &want); err != nil {
		return nil, err
	}
	parsed.KeyID = want
	return parsed.MarshalText()
}

func TestNewKeySetRefusesAFileThatContradictsItself(t *testing.T) {
	_, a := newTestKey(t, "a", false)
	_, b := newTestKey(t, "b", false)

	t.Run("id does not match the key", func(t *testing.T) {
		entry := map[string]any{}
		for k, v := range a {
			entry[k] = v
		}
		entry["keyID"] = b["keyID"]
		if _, err := NewKeySet(mustJSON(t, entry)); err == nil || !strings.Contains(err.Error(), "its id is") {
			t.Fatalf("err = %v, want the id mismatch refused", err)
		}
	})

	t.Run("the same name twice", func(t *testing.T) {
		dup := map[string]any{}
		for k, v := range b {
			dup[k] = v
		}
		dup["name"] = a["name"]
		if _, err := NewKeySet(mustJSON(t, a, dup)); err == nil || !strings.Contains(err.Error(), "appears twice") {
			t.Fatalf("err = %v, want the duplicate name refused", err)
		}
	})

	t.Run("the same key twice under two names", func(t *testing.T) {
		alias := map[string]any{}
		for k, v := range a {
			alias[k] = v
		}
		alias["name"] = "another-name"
		if _, err := NewKeySet(mustJSON(t, a, alias)); err == nil || !strings.Contains(err.Error(), "same key id") {
			t.Fatalf("err = %v, want the duplicate key refused", err)
		}
	})

	t.Run("no name", func(t *testing.T) {
		anon := map[string]any{"publicKey": a["publicKey"]}
		if _, err := NewKeySet(mustJSON(t, anon)); err == nil || !strings.Contains(err.Error(), "no name") {
			t.Fatalf("err = %v, want the nameless key refused", err)
		}
	})

	t.Run("not a public key", func(t *testing.T) {
		bad := map[string]any{"name": "x", "publicKey": "not-base64-at-all"}
		if _, err := NewKeySet(mustJSON(t, bad)); err == nil {
			t.Fatal("a key set with an unparseable public key was accepted")
		}
	})
}

func mustJSON(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"keys": entries})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
