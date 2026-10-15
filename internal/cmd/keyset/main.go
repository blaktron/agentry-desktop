// Command keyset generates an update-signing key pair and maintains
// internal/selfupdate/keys.json, the set of keys a build of Agentry Desktop
// will accept a release from.
//
// It exists so that a trust root is never hand-edited. keys.json has three ways
// to be wrong that a person editing it would not notice until a release failed
// to verify on someone else's machine: a key id that does not match its public
// key, two keys sharing a name, and a rotation that forgets to retire what it
// replaced, leaving two keys able to authorise an install. Every write here
// goes through internal/selfupdate's own parser before it reaches the file, so
// a file this command wrote is a file the application can read.
//
//	keyset generate -name prod -out <dir>   # the pair; private half 0600, never printed
//	keyset list                             # what this build trusts
//	keyset add -name prod -public <line> [-retire-current]
//
// scripts/operator/update-signing-key.sh is the wizard that drives all three in
// the right order. Reach for that rather than this.
package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

// keysPath is where the key set lives, relative to the repository root. The
// commands are run from there (the wizard checks), so a relative path is what
// the error message should name.
const keysPath = "internal/selfupdate/keys.json"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = generate(os.Args[2:])
	case "list":
		err = list()
	case "add":
		err = add(os.Args[2:])
	case "protect":
		err = protect(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "keyset: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `keyset maintains the update-signing key set (`+keysPath+`).

  keyset generate -name <name> -out <dir>    generate a pair; the private half is written 0600 and never printed
  keyset list                                 print the keys this build trusts
  keyset add -name <name> -public <line> [-note <text>] [-retire-current]
  keyset protect -key <file> -out <file> -password-stdin    write a password-protected backup of a key

scripts/operator/update-signing-key.sh drives these in the right order.
`)
}

// The file's shape is internal/selfupdate's, not this command's: one definition
// means a new field cannot be added to the reader and forgotten in the writer.

func readDoc() (*selfupdate.KeyFile, error) {
	raw, err := os.ReadFile(keysPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s (run this from the root of an agentry-desktop clone): %w", keysPath, err)
	}
	return selfupdate.ParseFile(raw)
}

// writeDoc validates before it writes, with the parser the application uses, so
// that "keyset wrote it" and "the app can read it" are one statement rather
// than two hopes.
func writeDoc(d *selfupdate.KeyFile) error {
	out, err := d.Marshal()
	if err != nil {
		return err
	}
	if _, err := selfupdate.NewKeySet(out); err != nil {
		return fmt.Errorf("refusing to write a key set the application could not read: %w", err)
	}
	if err := os.WriteFile(keysPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", keysPath, err)
	}
	return nil
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	name := fs.String("name", "", "what this key is called: dev, prod (required)")
	out := fs.String("out", "", "directory to write the pair into (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *out == "" {
		return errors.New("-name and -out are both required")
	}
	if err := validName(*name); err != nil {
		return err
	}

	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating the key: %w", err)
	}
	// MarshalText, not EncryptKey(""): an empty password still produces a
	// password-WRAPPED key that IsEncrypted reports as encrypted and that the
	// release workflow could not read without a password it does not have.
	// MarshalText is the plain form `minisign -G -W` writes. Unencrypted is
	// right for this copy because it becomes a GitHub Actions secret that the
	// release workflow reads with no person present;
	// internal/cmd/signrelease records what protects it instead.
	privText, err := priv.MarshalText()
	if err != nil {
		return fmt.Errorf("encoding the private half: %w", err)
	}

	keyFile := filepath.Join(*out, *name+".key")
	pubFile := filepath.Join(*out, *name+".pub")
	// 0600 before a byte is written, not after: a chmod after the write leaves
	// a window in which the key is world-readable.
	f, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating the private half (does it already exist?): %w", err)
	}
	if _, err := f.Write(privText); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing the private half: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	pubText, err := pub.MarshalText()
	if err != nil {
		return err
	}
	if err := os.WriteFile(pubFile, append(pubText, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing the public half: %w", err)
	}

	// The public half and the id are printed; the private half never is.
	fmt.Printf("%s\n", pub.String())
	fmt.Fprintf(os.Stderr, "keyset: wrote %s (0600, not printed) and %s; key id %016X\n", keyFile, pubFile, pub.ID())
	return nil
}

func list() error {
	// The file on disk, not selfupdate.Carried(): `add` writes the file, so a
	// `list` that read the embedded copy would report on a different source
	// than it edits — true only for as long as `go run` happens to rebuild
	// after the embed changes, and quietly wrong for a compiled binary.
	doc, err := readDoc()
	if err != nil {
		return err
	}
	set, err := doc.KeySet()
	if err != nil {
		return err
	}
	keys := set.Keys()
	if len(keys) == 0 {
		fmt.Printf("  (none in %s: a build made from this tree accepts no release, and the release workflow refuses every tag)\n", keysPath)
		return nil
	}
	for _, k := range keys {
		state := "current"
		if k.Retired {
			state = "retired"
		}
		fmt.Printf("  %-10s %s  %s", k.Name, k.ID(), state)
		if k.Note != "" {
			fmt.Printf("  — %s", k.Note)
		}
		fmt.Println()
	}
	return nil
}

func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	name := fs.String("name", "", "what this key is called: dev, prod (required)")
	public := fs.String("public", "", "the bare minisign public-key line (required)")
	note := fs.String("note", "", "where the private half lives")
	retire := fs.Bool("retire-current", false, "mark every key that is current today as retired")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *public == "" {
		return errors.New("-name and -public are both required")
	}
	if err := validName(*name); err != nil {
		return err
	}

	var pub minisign.PublicKey
	if err := pub.UnmarshalText([]byte(strings.TrimSpace(*public))); err != nil {
		return fmt.Errorf("-public is not a minisign public key line: %w", err)
	}

	d, err := readDoc()
	if err != nil {
		return err
	}
	for _, k := range d.Keys {
		if k.Name == *name {
			return fmt.Errorf("a key called %q is already in the set: a name identifies one key for the life of the product, so choose another", *name)
		}
	}
	if *retire {
		for i := range d.Keys {
			d.Keys[i].Retired = true
		}
	}
	d.Keys = append(d.Keys, selfupdate.KeyEntry{
		Name:      *name,
		PublicKey: pub.String(),
		KeyID:     fmt.Sprintf("%016X", pub.ID()),
		Retired:   false,
		AddedAt:   time.Now().UTC().Format("2006-01-02"),
		Note:      *note,
	})
	if err := writeDoc(d); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "keyset: added %q (%016X) to %s\n", *name, pub.ID(), keysPath)
	return nil
}

// protect writes a password-protected copy of an existing secret key, for the
// backup the wizard offers. It matters most for the production key: a GitHub
// Actions secret can be replaced but never read back, so without a copy the
// private half exists nowhere reachable, and losing it means every installed
// copy of the app stops accepting releases until its owner downloads a fresh
// one by hand.
//
// The password arrives on standard input, never as a flag (a process listing)
// or an environment variable (inherited by every child).
func protect(args []string) error {
	fs := flag.NewFlagSet("protect", flag.ExitOnError)
	keyPath := fs.String("key", "", "the unencrypted secret key to protect (required)")
	out := fs.String("out", "", "where to write the password-protected copy (required)")
	stdin := fs.Bool("password-stdin", false, "read the password from standard input (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyPath == "" || *out == "" || !*stdin {
		return errors.New("-key, -out and -password-stdin are all required")
	}

	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return fmt.Errorf("reading the key to protect: %w", err)
	}
	var priv minisign.PrivateKey
	if err := priv.UnmarshalText([]byte(strings.TrimSpace(string(raw)))); err != nil {
		return fmt.Errorf("%s is not an unencrypted minisign secret key: %w", filepath.Base(*keyPath), err)
	}
	pass, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading the password: %w", err)
	}
	password := strings.TrimRight(string(pass), "\r\n")
	if password == "" {
		return errors.New("an empty password would protect nothing")
	}
	sealed, err := minisign.EncryptKey(password, priv)
	if err != nil {
		return fmt.Errorf("protecting the key: %w", err)
	}
	// O_EXCL: never silently replace a backup that is already there.
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating the backup: %w", err)
	}
	if _, err := f.Write(sealed); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing the backup: %w", err)
	}
	return f.Close()
}

func validName(name string) error {
	if name == "" {
		return errors.New("a key needs a name")
	}
	for _, r := range name {
		lower := r >= 'a' && r <= 'z'
		digit := r >= '0' && r <= '9'
		if !lower && !digit && r != '-' {
			return fmt.Errorf("a key name is lower-case letters, digits and hyphens: %q is not", name)
		}
	}
	return nil
}
