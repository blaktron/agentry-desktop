package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

// This command writes a trust root, so what it writes is worth more scrutiny
// than what it reads. The tests run against a copy of keys.json in a temporary
// directory: keysPath is relative and these chdir into that directory, which is
// also how the command is really invoked (from the repository root).

// inTempRepo makes a directory holding internal/selfupdate/keys.json with the
// given entries, chdirs into it for the test, and returns its path.
func inTempRepo(t *testing.T, entries ...selfupdate.KeyEntry) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(keysPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	body := keystest.File(t, entries...)
	if err := os.WriteFile(filepath.Join(dir, keysPath), body, 0o644); err != nil {
		t.Fatal(err)
	}
	saved, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(saved) })
	return dir
}

func readBack(t *testing.T) *selfupdate.KeyFile {
	t.Helper()
	raw, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := selfupdate.ParseFile(raw)
	if err != nil {
		t.Fatalf("the file this command wrote does not parse: %v", err)
	}
	return doc
}

func TestAddWritesAKeyTheApplicationCanRead(t *testing.T) {
	inTempRepo(t)
	_, entry := keystest.Key(t, "ignored", false)

	if err := add([]string{"-name", "dev", "-public", entry.PublicKey, "-note", "where it lives"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	doc := readBack(t)
	if len(doc.Keys) != 1 {
		t.Fatalf("%d keys, want 1", len(doc.Keys))
	}
	got := doc.Keys[0]
	if got.Name != "dev" || got.Retired || got.Note != "where it lives" {
		t.Errorf("wrote %+v", got)
	}
	// The id is derived, never taken on trust from the caller.
	if got.KeyID != entry.KeyID {
		t.Errorf("key id = %q, want %q", got.KeyID, entry.KeyID)
	}
	if got.AddedAt == "" {
		t.Error("no date recorded")
	}
	// And the whole point: the application's parser accepts it.
	set, err := selfupdate.NewKeySet(keystest.File(t, doc.Keys...))
	if err != nil {
		t.Fatalf("the application could not read what this wrote: %v", err)
	}
	if len(set.Keys()) != 1 {
		t.Errorf("the set has %d keys", len(set.Keys()))
	}
}

// Rotation, which is the operation that has to be right: the old key stays in
// the file so old releases remain checkable, and is marked retired so nothing
// installs from it.
func TestAddWithRetireCurrentRetiresWhatWasThereAndKeepsIt(t *testing.T) {
	_, old := keystest.Key(t, "dev", false)
	inTempRepo(t, old)
	_, fresh := keystest.Key(t, "prod", false)

	if err := add([]string{"-name", "prod", "-public", fresh.PublicKey, "-retire-current"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	doc := readBack(t)
	if len(doc.Keys) != 2 {
		t.Fatalf("%d keys, want the old one kept beside the new one", len(doc.Keys))
	}
	byName := map[string]selfupdate.KeyEntry{}
	for _, k := range doc.Keys {
		byName[k.Name] = k
	}
	if !byName["dev"].Retired {
		t.Error("the replaced key was not retired, so two keys can authorise an install")
	}
	if byName["prod"].Retired {
		t.Error("the new key was written retired")
	}
	if byName["dev"].PublicKey != old.PublicKey {
		t.Error("the replaced key's public half changed, so releases it signed stop verifying")
	}
}

// Without -retire-current the previous key stays current. That is deliberate —
// it is the ordering that lets a release teach every installed copy about the
// new key before the new key starts signing — so it must not happen by
// accident either.
func TestAddWithoutRetireLeavesThePreviousKeyCurrent(t *testing.T) {
	_, old := keystest.Key(t, "dev", false)
	inTempRepo(t, old)
	_, fresh := keystest.Key(t, "prod", false)

	if err := add([]string{"-name", "prod", "-public", fresh.PublicKey}); err != nil {
		t.Fatalf("add: %v", err)
	}
	for _, k := range readBack(t).Keys {
		if k.Retired {
			t.Errorf("%s was retired although -retire-current was not given", k.Name)
		}
	}
}

func TestAddRefusesWhatWouldMakeAnUnreadableFile(t *testing.T) {
	_, existing := keystest.Key(t, "dev", false)

	t.Run("a name already in the set", func(t *testing.T) {
		inTempRepo(t, existing)
		_, fresh := keystest.Key(t, "ignored", false)
		err := add([]string{"-name", "dev", "-public", fresh.PublicKey})
		if err == nil || !strings.Contains(err.Error(), "already in the set") {
			t.Fatalf("err = %v, want the duplicate name refused", err)
		}
		if len(readBack(t).Keys) != 1 {
			t.Error("the file changed although the command refused")
		}
	})

	t.Run("the same key under a second name", func(t *testing.T) {
		inTempRepo(t, existing)
		err := add([]string{"-name", "prod", "-public", existing.PublicKey})
		if err == nil || !strings.Contains(err.Error(), "could not read") {
			t.Fatalf("err = %v, want the duplicate key refused before it is written", err)
		}
		if len(readBack(t).Keys) != 1 {
			t.Error("the file changed although the command refused")
		}
	})

	t.Run("not a public key", func(t *testing.T) {
		inTempRepo(t)
		err := add([]string{"-name", "dev", "-public", "not-a-key"})
		if err == nil || !strings.Contains(err.Error(), "minisign public key") {
			t.Fatalf("err = %v, want the bad key refused", err)
		}
	})

	t.Run("a name with characters that are not allowed", func(t *testing.T) {
		inTempRepo(t)
		_, fresh := keystest.Key(t, "ignored", false)
		if err := add([]string{"-name", "Prod Key", "-public", fresh.PublicKey}); err == nil {
			t.Fatal("a name with a space and a capital was accepted")
		}
	})
}

// The explanation at the top of keys.json is the only place the file's own
// shape is documented; a tool that ate it would be worse than hand-editing.
func TestAddPreservesTheCommentBlock(t *testing.T) {
	dir := inTempRepo(t)
	withComment := `{"_comment": ["line one", "line two"], "keys": []}`
	if err := os.WriteFile(filepath.Join(dir, keysPath), []byte(withComment), 0o644); err != nil {
		t.Fatal(err)
	}
	_, fresh := keystest.Key(t, "ignored", false)

	if err := add([]string{"-name", "dev", "-public", fresh.PublicKey}); err != nil {
		t.Fatalf("add: %v", err)
	}
	body, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"line one", "line two"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the comment block lost %q", want)
		}
	}
}

func TestGenerateWritesAPrivateHalfOnlyItsOwnerCanRead(t *testing.T) {
	dir := t.TempDir()
	if err := generate([]string{"-name", "dev", "-out", dir}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "dev.key"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the private half is mode %o, want 600", perm)
	}

	// It must be the unencrypted shape, or the release workflow — which has no
	// password — could not read it.
	raw, err := os.ReadFile(filepath.Join(dir, "dev.key"))
	if err != nil {
		t.Fatal(err)
	}
	if minisign.IsEncrypted(raw) {
		t.Fatal("the private half is password-wrapped; the release workflow could not read it")
	}
	var priv minisign.PrivateKey
	if err := priv.UnmarshalText(raw); err != nil {
		t.Fatalf("the private half does not parse: %v", err)
	}

	// And the public half beside it must be that key's.
	pubRaw, err := os.ReadFile(filepath.Join(dir, "dev.pub"))
	if err != nil {
		t.Fatal(err)
	}
	var pub minisign.PublicKey
	if err := pub.UnmarshalText([]byte(strings.TrimSpace(string(pubRaw)))); err != nil {
		t.Fatalf("the public half does not parse: %v", err)
	}
	if !pub.Equal(priv.Public()) {
		t.Error("the two halves written are not a pair")
	}

	// Running it again over the same name must not silently replace a key.
	if err := generate([]string{"-name", "dev", "-out", dir}); err == nil {
		t.Error("generate overwrote an existing private half")
	}
}

// The backup the wizard offers for the production key: without it, a key handed
// to GitHub exists nowhere readable, and losing it strands every installed copy.
func TestProtectRoundTripsThroughAPassword(t *testing.T) {
	dir := t.TempDir()
	if err := generate([]string{"-name", "prod", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "prod.key")
	backup := filepath.Join(dir, "prod-backup.key")

	withStdin(t, "a passphrase\n", func() {
		if err := protect([]string{"-key", plain, "-out", backup, "-password-stdin"}); err != nil {
			t.Fatalf("protect: %v", err)
		}
	})

	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the backup is mode %o, want 600", perm)
	}
	sealed, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !minisign.IsEncrypted(sealed) {
		t.Fatal("the backup is not password-protected")
	}
	recovered, err := minisign.DecryptKey("a passphrase", sealed)
	if err != nil {
		t.Fatalf("the backup does not open with its password: %v", err)
	}
	original, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	var want minisign.PrivateKey
	if err := want.UnmarshalText(original); err != nil {
		t.Fatal(err)
	}
	if !recovered.Equal(want) {
		t.Error("the backup is not the key it was made from")
	}

	// Never silently replace a backup that is already there.
	withStdin(t, "a passphrase\n", func() {
		if err := protect([]string{"-key", plain, "-out", backup, "-password-stdin"}); err == nil {
			t.Error("protect overwrote an existing backup")
		}
	})
}

func TestProtectRefusesAnEmptyPassword(t *testing.T) {
	dir := t.TempDir()
	if err := generate([]string{"-name", "prod", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "\n", func() {
		err := protect([]string{"-key", filepath.Join(dir, "prod.key"), "-out", filepath.Join(dir, "b.key"), "-password-stdin"})
		if err == nil || !strings.Contains(err.Error(), "protect nothing") {
			t.Fatalf("err = %v, want an empty password refused", err)
		}
	})
}

func withStdin(t *testing.T, in string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(in)
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = saved; _ = r.Close() }()
	fn()
}
