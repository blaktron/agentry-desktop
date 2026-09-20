// Package selfupdate holds the trust root for "a newer Agentry Desktop": the
// set of signing keys this build will accept a release from, and the check that
// says whether a given SHA256SUMS was signed by one of them.
//
// The vocabulary, kept apart on purpose (plan desktop-installers.md §7):
// internal/install already owns "update" for an installed BriefAgent, so the
// app's own mechanism is SELF-UPDATE in code and "a newer Agentry Desktop" in
// anything a person reads.
//
// What this package is NOT: it is not OS code signing. A Developer ID signature
// (agentry-desktop#6) or an Authenticode one (#54) tells the operating system
// who built the application; this key tells the application whether a release it
// is about to install came from us. The two are independent, and the plan's D3
// turns on exactly that: the installers ship unsigned by the OS today, and the
// updater's trust does not wait for them.
//
// This package verifies. It does not download, and it does not decide what to
// do with the answer — Verify reports which key signed and whether that key is
// retired, and the caller (M6, agentry-desktop#87) decides whether that is
// grounds to install. Keeping the decision out of here is what lets a retired
// key still prove an OLD release genuine while never authorising a new one.
package selfupdate

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aead.dev/minisign"
)

// keysJSON is the committed key set. It is embedded rather than read from disk
// because it is a trust root: a file beside the binary is a file an attacker who
// can write beside the binary can replace, and at that point the update check is
// theirs. Changing what this build trusts requires a new build.
//
//go:embed keys.json
var keysJSON []byte

// Key is one signing key this build trusts.
type Key struct {
	// Name is what this key is called in the record and in messages: "dev",
	// "prod". Stable for the life of the key.
	Name string
	// Retired is true once a newer key has taken over. A retired key still
	// verifies, so a release it signed stays checkable; callers refuse to
	// INSTALL from one. See the package comment.
	Retired bool
	// Note says where the private half lives and what it is for.
	Note string

	pub minisign.PublicKey
}

// ID is the 64-bit key id minisign prints, as 16 hex digits.
func (k Key) ID() string { return fmt.Sprintf("%016X", k.pub.ID()) }

// PublicKey is the bare minisign public-key line, the form `minisign -P` takes.
func (k Key) PublicKey() string { return k.pub.String() }

// String names the key the way an error message should: the name a person reads
// and the id they can compare against a .pub file.
func (k Key) String() string {
	if k.Retired {
		return fmt.Sprintf("%s (%s, retired)", k.Name, k.ID())
	}
	return fmt.Sprintf("%s (%s)", k.Name, k.ID())
}

// KeySet is an ordered set of keys a build trusts.
type KeySet struct{ keys []Key }

var (
	// ErrNoKeys means this build carries no signing key at all, so it can
	// verify nothing. That is the correct state for a build of the open core,
	// which anyone can make and which has no update channel (plan §7); it is a
	// broken state for a release build, which is why the release workflow
	// refuses a tag it cannot sign.
	ErrNoKeys = errors.New("this build carries no update-signing key, so it can verify no release")
	// ErrUnknownKey means the signature was made by a key this build does not
	// carry. Distinct from ErrBadSignature so the message can tell a person
	// which of the two happened: an unknown key usually means a build older
	// than the key rotation, and the answer is to download a fresh copy; a bad
	// signature means the bytes or the signature are wrong, and the answer is
	// never to install them.
	ErrUnknownKey = errors.New("the signature was made by a key this build does not trust")
	// ErrBadSignature means a key this build carries was named, and the
	// signature does not verify against it.
	ErrBadSignature = errors.New("the signature does not match the bytes it covers")
)

// KeyEntry is one key as keys.json carries it. It is the ONE definition of that
// file's shape: internal/cmd/keyset reads and writes through this type too, so
// a field cannot be added in one place and forgotten in the other.
type KeyEntry struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	KeyID     string `json:"keyID"`
	Retired   bool   `json:"retired"`
	AddedAt   string `json:"addedAt,omitempty"`
	Note      string `json:"note,omitempty"`
}

// KeyFile is keys.json itself. The comment block is kept as raw JSON so that a
// tool rewriting the file preserves the explanation at the top of it verbatim.
type KeyFile struct {
	Comment json.RawMessage `json:"_comment,omitempty"`
	Keys    []KeyEntry      `json:"keys"`
}

// ParseFile reads keys.json without interpreting it, for a tool that means to
// edit it. Use NewKeySet to get something that can verify.
func ParseFile(data []byte) (*KeyFile, error) {
	var f KeyFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("the update-signing key set is not readable: %w", err)
	}
	return &f, nil
}

// Marshal writes the file back in the shape it is committed in.
func (f *KeyFile) Marshal() ([]byte, error) {
	out, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// NewKeySet parses a key set from the JSON shape keys.json documents. It is
// exported so a test can build a set from keys it generated, and so the site's
// tooling has one definition of the file to read.
func NewKeySet(data []byte) (*KeySet, error) {
	doc, err := ParseFile(data)
	if err != nil {
		return nil, err
	}
	return doc.KeySet()
}

// KeySet turns a parsed file into something that can verify, refusing a file
// that contradicts itself.
func (f *KeyFile) KeySet() (*KeySet, error) {
	doc := f
	set := &KeySet{}
	seenName := map[string]bool{}
	seenID := map[string]bool{}
	for i, k := range doc.Keys {
		where := fmt.Sprintf("key %d", i)
		if k.Name != "" {
			where = fmt.Sprintf("key %q", k.Name)
		}
		if k.Name == "" {
			return nil, fmt.Errorf("%s has no name", where)
		}
		if seenName[k.Name] {
			return nil, fmt.Errorf("%s appears twice: a name identifies one key for the life of the key set", where)
		}
		var pub minisign.PublicKey
		if err := pub.UnmarshalText([]byte(strings.TrimSpace(k.PublicKey))); err != nil {
			return nil, fmt.Errorf("%s does not carry a minisign public key: %w", where, err)
		}
		// The id in the file is redundant with the key, which is why it is
		// worth checking rather than trusting: the two disagreeing is the
		// signature of a paste that took the comment line from one key and the
		// base64 from another, and it would otherwise show up as a release
		// that mysteriously fails to verify.
		got := fmt.Sprintf("%016X", pub.ID())
		if want := strings.ToUpper(strings.TrimSpace(k.KeyID)); want != "" && want != got {
			return nil, fmt.Errorf("%s says its id is %s but its public key's id is %s", where, want, got)
		}
		if seenID[got] {
			return nil, fmt.Errorf("%s has the same key id (%s) as an earlier key", where, got)
		}
		seenName[k.Name], seenID[got] = true, true
		set.keys = append(set.keys, Key{Name: k.Name, Retired: k.Retired, Note: k.Note, pub: pub})
	}
	return set, nil
}

var carried = sync.OnceValues(func() (*KeySet, error) { return NewKeySet(keysJSON) })

// Carried is the key set compiled into this build. The error is a malformed
// keys.json, which is a broken build rather than a runtime condition: the test
// in this package is what keeps a bad file from being committed, and `make
// check` is what runs it.
//
// The name says what this build CARRIES, not what it vouches for, and that is
// the more precise statement as well as the one the copy rule wants
// (scripts/copy-check.sh): whether a key should authorise an install is the
// caller's decision, never this function's.
func Carried() (*KeySet, error) { return carried() }

// Keys returns the keys in the set, in the order they were added.
func (s *KeySet) Keys() []Key { return append([]Key(nil), s.keys...) }

// Verify reports which key in the set signed message with signature, where
// signature is the content of a minisign .minisig file and message the content
// of the file it covers — for a release, SHA256SUMS and SHA256SUMS.minisig.
//
// It tries every key rather than looking the signature's key id up, because
// minisign's key id is an unauthenticated hint: anyone can write any id into a
// signature file. Using it to CHOOSE the key would let an attacker steer the
// check, so it is used only to tell a person which key was claimed when nothing
// verified. A handful of keys makes the cost of trying all of them nothing.
//
// A successful verification returns the key that made it, whose Retired field
// the caller must consider: this function answers "is this genuinely ours?",
// not "should this be installed?".
func (s *KeySet) Verify(message, signature []byte) (Key, error) {
	if len(s.keys) == 0 {
		return Key{}, ErrNoKeys
	}
	var sig minisign.Signature
	if err := sig.UnmarshalText(signature); err != nil {
		return Key{}, fmt.Errorf("the signature file is not a minisign signature: %w", err)
	}
	// The algorithm byte is not covered by either signature in the file, and
	// minisign's verifier reacts to it: a signature marked HashEdDSA is checked
	// against BLAKE2b-512 of the message instead of the message. We only ever
	// emit EdDSA, so anything else is either a file we did not make or a byte
	// somebody changed, and the cheapest correct answer to both is to refuse
	// before any of it is interpreted.
	if sig.Algorithm != minisign.EdDSA {
		return Key{}, fmt.Errorf("%w: it is marked algorithm %#04x, and releases of this product are signed with EdDSA (%#04x)", ErrBadSignature, sig.Algorithm, minisign.EdDSA)
	}
	for _, k := range s.keys {
		if minisign.Verify(k.pub, message, signature) {
			return k, nil
		}
	}
	claimed := fmt.Sprintf("%016X", sig.KeyID)
	for _, k := range s.keys {
		if k.ID() == claimed {
			return Key{}, fmt.Errorf("%w: it claims key %s, which this build does carry", ErrBadSignature, k)
		}
	}
	return Key{}, fmt.Errorf("%w: it claims key %s, and this build carries %s", ErrUnknownKey, claimed, s.describe())
}

// ErrRetiredKey means the signature is genuine but was made by a key that has
// been replaced. It is its own error because the two answers are different: the
// release is really ours, and it must still not be installed.
var ErrRetiredKey = errors.New("the signature was made by a key that has since been retired")

// VerifyForInstall is Verify plus the one policy decision this package is
// willing to make: a retired key may prove an old release genuine, and may
// never authorise installing anything.
//
// It exists so that authorising an install is a DIFFERENT CALL from checking a
// signature, rather than a field a caller has to remember to look at. Verify
// returning (Key, error) makes `if _, err := Verify(...); err == nil { install }`
// compile and silently accept a retired key; that shape is the bug this
// function is here to make unwritable.
func (s *KeySet) VerifyForInstall(message, signature []byte) (Key, error) {
	key, err := s.Verify(message, signature)
	if err != nil {
		return key, err
	}
	if key.Retired {
		return key, fmt.Errorf("%w: %s", ErrRetiredKey, key)
	}
	return key, nil
}

// describe names the keys a build carries, for the one error message where a
// person needs to know what it had instead.
func (s *KeySet) describe() string {
	if len(s.keys) == 0 {
		return "none"
	}
	names := make([]string, 0, len(s.keys))
	for _, k := range s.keys {
		names = append(names, k.String())
	}
	return strings.Join(names, ", ")
}
