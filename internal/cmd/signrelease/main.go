// Command signrelease signs a release's SHA256SUMS with Agentry's own
// update-signing key and then verifies what it wrote with the very code the
// application will use to check it.
//
// It exists rather than a `go run aead.dev/minisign/cmd/minisign@vX` line in
// the workflow for two reasons, and the second is the important one:
//
//  1. the minisign version that signs is then the one go.mod pins and go.sum
//     covers, not whatever the proxy serves at release time; and
//  2. the check afterwards runs internal/selfupdate against the COMMITTED key
//     set, so a release cannot be published whose signature the shipped
//     application would refuse. The mistake that guards against is the one
//     rotation invites: a new private half loaded into the Actions secret
//     without the matching public half committed, which no amount of "the
//     signing step succeeded" would catch.
//
// It never prints the private key, and it reads it from a file rather than an
// argument or an environment variable so it does not reach a process listing.
//
//	go run ./internal/cmd/signrelease -key <secret-key-file> -in SHA256SUMS \
//	  -version 0.2.0 -commit abc1234
//
// Writes <in>.minisig beside <in>.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "signrelease: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		keyPath  = flag.String("key", "", `file holding the minisign secret key, or "-" for standard input (required)`)
		passPath = flag.String("password-file", "", "file holding the key's password, when the key is password-protected")
		in       = flag.String("in", "", "file to sign, normally SHA256SUMS (required)")
		version  = flag.String("version", "", "the version this release is (required)")
		commit   = flag.String("commit", "", "the commit it was built from (required)")
		product  = flag.String("product", selfupdate.ThisProduct, "the product this release is of, as its repository is named (agentry-desktop, agentry-cli)")
		out      = flag.String("out", "", "signature file to write (default: <in>.minisig)")
	)
	flag.Parse()
	if *keyPath == "" || *in == "" || *version == "" || *commit == "" {
		flag.Usage()
		return errors.New("-key, -in, -version and -commit are all required")
	}
	comment, err := checkedComment(*product, *version, *commit)
	if err != nil {
		return err
	}
	if *out == "" {
		*out = *in + ".minisig"
	}

	message, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("reading the file to sign: %w", err)
	}
	if len(message) == 0 {
		return fmt.Errorf("%s is empty: a signature over nothing proves nothing", *in)
	}
	if err := sumsNameProduct(message, *product); err != nil {
		return err
	}

	priv, err := readKey(*keyPath, *passPath)
	if err != nil {
		return err
	}

	// The committed key set: what a shipped application will check this release
	// against, which is the only opinion that matters here.
	carried, err := selfupdate.Carried()
	if err != nil {
		return fmt.Errorf("the committed key set is not readable, so this release could not be checked: %w", err)
	}

	sig, key, err := signAndVerify(priv, message, comment, carried)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, sig, 0o644); err != nil {
		return fmt.Errorf("writing the signature: %w", err)
	}

	fmt.Printf("signrelease: %s signed with %s and verified against the committed key set\n", filepath.Base(*out), key)
	return nil
}

// sumsNameProduct refuses to sign one product's SHA256SUMS with another
// product's comment. The artifact names are Agentry's product contract — the
// site's importer reads the product off them: agentry-desktop-<v>-… is the
// desktop, agentry-<v>-… anything else is the CLI — so a sums file whose
// names are the other product's is the misuse this catches: a CLI release
// signed with the default product would carry a signed statement calling it
// the desktop, the site would import it, and the app would offer it and then
// find no artifact to download. BUILD-INFO is the one line that is not an
// artifact.
func sumsNameProduct(sums []byte, product string) error {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == "BUILD-INFO" {
			continue
		}
		isDesktop := strings.HasPrefix(name, "agentry-desktop-")
		isCLI := strings.HasPrefix(name, "agentry-") && !isDesktop
		switch {
		case product == "agentry-desktop" && !isDesktop, product == "agentry-cli" && !isCLI:
			return fmt.Errorf("refusing to sign: %s lists %q, which is not an artifact of %s — the wrong -product, or the wrong sums", "SHA256SUMS", name, product)
		case product != "agentry-desktop" && product != "agentry-cli" && !strings.HasPrefix(name, product+"-"):
			return fmt.Errorf("refusing to sign: %s lists %q, which does not carry the product name %s-", "SHA256SUMS", name, product)
		}
	}
	return nil
}

// checkedComment is the signed comment for a release of product, after a
// round trip through the reader. It is formatted by internal/selfupdate and
// not here and not by a workflow's shell, because the application PARSES its
// own product's comment to learn which release it is being offered — the
// manifest is unsigned, so this comment is the only version statement the
// key covers; one function writes it and one reads it, so the two cannot
// drift. A release cannot be signed with a comment that would not be read
// back as exactly this product, version and commit — a product name outside
// the format, a version with a space in it, a commit that is not hex.
func checkedComment(product, version, commit string) (string, error) {
	comment := selfupdate.TrustedCommentFor(product, version, commit)
	p, v, c, err := selfupdate.ParseAnyTrustedComment(comment)
	if err != nil || p != product || v != version || c != commit {
		return "", fmt.Errorf("refusing to sign: %q would not be read back as product %q, version %q, commit %q (%v)", comment, product, version, commit, err)
	}
	return comment, nil
}

// signAndVerify signs message and then checks the result against the key set a
// shipped application carries, returning the signature and the key that made
// it. Nothing is written until it has verified: a signature file on disk is one
// that has already been proven checkable.
//
// It is separate from run so that a test can drive it with generated keys. That
// matters more than it looks: the failures worth catching here — a secret that
// is not the committed key's other half, and a retired key signing something
// new — are exactly the ones that need real key material to reproduce, and a
// test can make key material where CI has no secret to borrow.
func signAndVerify(priv minisign.PrivateKey, message []byte, comment string, carried *selfupdate.KeySet) ([]byte, selfupdate.Key, error) {
	sig := minisign.SignWithComments(priv, message, comment, "signed by the agentry-desktop release workflow")

	// VerifyForInstall, not Verify: a release is exactly the thing a retired key
	// must not authorise, and asking through this call means the rule is not a
	// field this function has to remember to read.
	key, err := carried.VerifyForInstall(message, sig)
	switch {
	case errors.Is(err, selfupdate.ErrNoKeys):
		return nil, key, fmt.Errorf("this release was signed, but internal/selfupdate/keys.json carries no public key, so no build of the app could verify it: run scripts/operator/update-signing-key.sh and commit the key set (%w)", err)
	case errors.Is(err, selfupdate.ErrRetiredKey):
		return nil, key, fmt.Errorf("this release was signed with %s, which internal/selfupdate/keys.json marks retired: a retired key may prove an old release genuine, never authorise a new one (%w)", key, err)
	case err != nil:
		return nil, key, fmt.Errorf("the signature this just wrote does not verify against the committed key set: the signing key and internal/selfupdate/keys.json are not two halves of one pair (%w)", err)
	}
	return sig, key, nil
}

// readKey loads a minisign secret key in either shape it comes in: the
// unencrypted one `minisign -G -W` writes, and the password-protected one it
// writes by default.
//
// Both are supported because the two keys here want different things.
// The dev key is an Actions secret written to a job-scoped file and removed at
// the end of the job; a password on it would be a second Actions secret in the
// same store, which anything able to read the first can read too, so it would
// add ceremony rather than protection. The production key lives on hardware the
// maintainers hold, where a password is the difference between a stolen laptop
// and a stolen signing key.
//
// The password is read from a FILE, never a flag or an environment variable: a
// flag reaches the process listing and an environment variable reaches every
// child process and most crash reporters.
func readKey(keyPath, passPath string) (minisign.PrivateKey, error) {
	var priv minisign.PrivateKey
	var raw []byte
	var err error
	name := filepath.Base(keyPath)
	if keyPath == "-" {
		// "-" is standard input, which is how the release workflow hands the
		// key over: piped in, so it is never written to the runner host's disk
		// and never sits in a file some later step could read.
		name = "the key on standard input"
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(keyPath)
	}
	if err != nil {
		return priv, fmt.Errorf("reading the signing key: %w", err)
	}
	// A key that has been through a file, a clipboard or a secret store usually
	// has a newline on it that the key itself does not.
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return priv, fmt.Errorf("%s is empty", name)
	}

	if !minisign.IsEncrypted(raw) {
		if passPath != "" {
			return priv, fmt.Errorf("%s is not password-protected, but -password-file was given: refusing rather than silently ignoring it", name)
		}
		if err := priv.UnmarshalText(raw); err != nil {
			return priv, fmt.Errorf("%s is not a minisign secret key: %w", name, err)
		}
		return priv, nil
	}

	if passPath == "" {
		return priv, fmt.Errorf("%s is password-protected and no -password-file was given", name)
	}
	pass, err := os.ReadFile(passPath)
	if err != nil {
		return priv, fmt.Errorf("reading the key's password: %w", err)
	}
	// A password file written by `echo` carries a newline the password does not.
	priv, err = minisign.DecryptKey(strings.TrimRight(string(pass), "\r\n"), raw)
	if err != nil {
		return priv, fmt.Errorf("%s did not decrypt: is the password right? (%w)", name, err)
	}
	return priv, nil
}
