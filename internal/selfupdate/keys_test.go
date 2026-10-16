package selfupdate_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

const sums = "aa1f56ffcb84076209f4e5cf3eb0282c  agentry-desktop-0.1.0-darwin-universal.dmg\n"

// The committed file must parse, and every key in it must be internally
// consistent. This is the test that keeps a bad keys.json out of a release:
// nothing else reads the file until an update check runs on someone's machine.
func TestCommittedKeySetIsValid(t *testing.T) {
	set, err := selfupdate.Carried()
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
	// Not an error: a build from source carries no keys, and a release writes
	// its own set over this file. The test says so out loud rather than
	// letting an empty set look like a passing check of something.
	if len(set.Keys()) == 0 {
		t.Log("the committed key set is empty: this build trusts no release")
	}
}

func TestVerifyAcceptsTheCurrentKey(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

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
	oldPriv, oldEntry := keystest.Key(t, "dev", true)
	_, newEntry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, oldEntry, newEntry)

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
	strangerPriv, _ := keystest.Key(t, "not-in-the-set", false)
	_, mine := keystest.Key(t, "prod", false)
	set := keystest.Set(t, mine)

	_, err := set.Verify([]byte(sums), minisign.Sign(strangerPriv, []byte(sums)))
	if !errors.Is(err, selfupdate.ErrUnknownKey) {
		t.Fatalf("Verify err = %v, want selfupdate.ErrUnknownKey", err)
	}
}

func TestVerifyRefusesTamperedSums(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	sig := minisign.Sign(priv, []byte(sums))

	tampered := strings.Replace(sums, "aa1f", "bb1f", 1)
	_, err := set.Verify([]byte(tampered), sig)
	if !errors.Is(err, selfupdate.ErrBadSignature) {
		t.Fatalf("Verify err = %v, want selfupdate.ErrBadSignature", err)
	}
	// The message must say the key IS carried, so a person is not sent looking
	// for a key rotation when the bytes are what changed.
	if !strings.Contains(err.Error(), "does carry") {
		t.Errorf("message %q does not distinguish tampered bytes from an unknown key", err)
	}
}

func TestVerifyRefusesAMalformedSignature(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	for _, junk := range []string{"", "not a signature", "untrusted comment: x\nnope\n"} {
		if _, err := set.Verify([]byte(sums), []byte(junk)); err == nil {
			t.Errorf("Verify(%q) accepted a malformed signature", junk)
		}
	}
}

// A build of the open core carries no key and must refuse everything rather
// than fall back to "the checksum matched".
func TestVerifyRefusesEverythingWhenTheBuildCarriesNoKey(t *testing.T) {
	priv, _ := keystest.Key(t, "prod", false)
	set := keystest.Set(t)

	_, err := set.Verify([]byte(sums), minisign.Sign(priv, []byte(sums)))
	if !errors.Is(err, selfupdate.ErrNoKeys) {
		t.Fatalf("Verify err = %v, want selfupdate.ErrNoKeys", err)
	}
}

// A signature carries its signer's key id in the clear and nothing protects it,
// so the key id must not be what chooses the key: if it were, writing a trusted
// key's id into a signature made by another key would steer the check. Here the
// signature is made by a stranger but claims the trusted key's id, and it must
// still be refused.
func TestVerifyDoesNotTrustTheKeyIDInTheSignature(t *testing.T) {
	strangerPriv, _ := keystest.Key(t, "stranger", false)
	_, mine := keystest.Key(t, "prod", false)
	set := keystest.Set(t, mine)

	sig := minisign.Sign(strangerPriv, []byte(sums))
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	trustedID := set.Keys()[0].ID()
	forged, err := forgeKeyID(sig, trustedID)
	if err != nil {
		t.Fatalf("could not rewrite the signature's key id, so this test proved nothing: %v", err)
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
	_, a := keystest.Key(t, "a", false)
	_, b := keystest.Key(t, "b", false)

	t.Run("id does not match the key", func(t *testing.T) {
		entry := a
		entry.KeyID = b.KeyID
		if _, err := selfupdate.NewKeySet(keystest.File(t, entry)); err == nil || !strings.Contains(err.Error(), "its id is") {
			t.Fatalf("err = %v, want the id mismatch refused", err)
		}
	})

	t.Run("the same name twice", func(t *testing.T) {
		dup := b
		dup.Name = a.Name
		if _, err := selfupdate.NewKeySet(keystest.File(t, a, dup)); err == nil || !strings.Contains(err.Error(), "appears twice") {
			t.Fatalf("err = %v, want the duplicate name refused", err)
		}
	})

	t.Run("the same key twice under two names", func(t *testing.T) {
		alias := a
		alias.Name = "another-name"
		if _, err := selfupdate.NewKeySet(keystest.File(t, a, alias)); err == nil || !strings.Contains(err.Error(), "same key id") {
			t.Fatalf("err = %v, want the duplicate key refused", err)
		}
	})

	t.Run("no name", func(t *testing.T) {
		anon := selfupdate.KeyEntry{PublicKey: a.PublicKey}
		if _, err := selfupdate.NewKeySet(keystest.File(t, anon)); err == nil || !strings.Contains(err.Error(), "no name") {
			t.Fatalf("err = %v, want the nameless key refused", err)
		}
	})

	t.Run("not a public key", func(t *testing.T) {
		bad := selfupdate.KeyEntry{Name: "x", PublicKey: "not-base64-at-all"}
		if _, err := selfupdate.NewKeySet(keystest.File(t, bad)); err == nil {
			t.Fatal("a key set with an unparseable public key was accepted")
		}
	})
}

// The algorithm byte is not covered by either signature in a minisign file, and
// minisign's verifier reacts to it: HashEdDSA means "check against BLAKE2b of
// the message" instead of the message. Flipping it on a release signature is
// not by itself an attack — the digest would not match either — but it is a
// byte an attacker controls that steers a verifier, and refusing anything we
// did not emit costs nothing.
func TestVerifyRefusesASignatureMarkedWithAnotherAlgorithm(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	var sig minisign.Signature
	if err := sig.UnmarshalText(minisign.Sign(priv, []byte(sums))); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if sig.Algorithm != minisign.EdDSA {
		t.Fatalf("this product signs with EdDSA; the library produced %#04x", sig.Algorithm)
	}
	sig.Algorithm = minisign.HashEdDSA
	flipped, err := sig.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}

	_, err = set.Verify([]byte(sums), flipped)
	if !errors.Is(err, selfupdate.ErrBadSignature) {
		t.Fatalf("Verify err = %v, want ErrBadSignature", err)
	}
	if !strings.Contains(err.Error(), "EdDSA") {
		t.Errorf("message %q does not name the algorithm it expected", err)
	}
}

// The trusted comment travels in the signature file and is covered by its own
// signature. It is where the release's version and commit are recorded, so a
// comment that could be altered would be a release that could lie about which
// release it is.
func TestVerifyRefusesATamperedTrustedComment(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	sig := minisign.SignWithComments(priv, []byte(sums), "agentry-desktop 0.2.0 (commit abc1234)", "x")
	if _, err := set.Verify([]byte(sums), sig); err != nil {
		t.Fatalf("the untampered signature did not verify: %v", err)
	}

	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	parsed.TrustedComment = "agentry-desktop 9.9.9 (commit deadbee)"
	tampered, err := parsed.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if _, err := set.Verify([]byte(sums), tampered); err == nil {
		t.Fatal("a signature whose trusted comment was rewritten still verified")
	}
}

// Verify answers "is this ours?"; VerifyForInstall answers "may this replace
// the application?". The difference is the retired key, and the reason the
// second call exists is that the first one's shape lets a caller forget.
func TestVerifyForInstallRefusesARetiredKeyThatVerifyAccepts(t *testing.T) {
	priv, entry := keystest.Key(t, "dev", true)
	set := keystest.Set(t, entry)
	sig := minisign.Sign(priv, []byte(sums))

	if _, err := set.Verify([]byte(sums), sig); err != nil {
		t.Fatalf("Verify must still accept a retired key, so an old release stays checkable: %v", err)
	}

	key, err := set.VerifyForInstall([]byte(sums), sig)
	if !errors.Is(err, selfupdate.ErrRetiredKey) {
		t.Fatalf("VerifyForInstall err = %v, want ErrRetiredKey", err)
	}
	if key.Name != "dev" {
		t.Errorf("the refusal does not name the key that signed it: %v", key)
	}
}

func TestVerifyForInstallAcceptsACurrentKey(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	key, err := set.VerifyForInstall([]byte(sums), minisign.Sign(priv, []byte(sums)))
	if err != nil {
		t.Fatalf("VerifyForInstall: %v", err)
	}
	if key.Name != "prod" {
		t.Errorf("signed by %q, want prod", key.Name)
	}
}
