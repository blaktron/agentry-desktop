package main

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

// These cover what the release workflow's signing step cannot be made to prove
// on its own: it needs the real Actions secret, so until the operator has
// loaded one there is nothing in CI to exercise it with. Generated keys let the
// same logic run on every change.

var sums = []byte("aa1f56ffcb84076209f4e5cf3eb0282c  agentry-desktop-0.2.0-darwin-universal.dmg\n")

func TestSignAndVerifyProducesASignatureTheAppAccepts(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	sig, key, err := signAndVerify(priv, sums, "agentry-desktop 0.2.0 (commit abc1234)", set)
	if err != nil {
		t.Fatalf("signAndVerify: %v", err)
	}
	if key.Name != "prod" {
		t.Errorf("signed by %q, want prod", key.Name)
	}
	// The trusted comment is signed, so it is worth asserting it survived: it
	// is what binds a signature to the release it was made for.
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		t.Fatalf("the signature does not parse: %v", err)
	}
	if parsed.TrustedComment != "agentry-desktop 0.2.0 (commit abc1234)" {
		t.Errorf("trusted comment = %q", parsed.TrustedComment)
	}
	// And the app must accept it, which is the whole point.
	if _, err := set.Verify(sums, sig); err != nil {
		t.Errorf("the application would refuse this release: %v", err)
	}
}

// A CLI bundle signed by this tool names the CLI in its signed comment, and
// nothing else about the signature changes (agentry-desktop#171): the key set
// still has to carry the key as current, and the app's own reader refuses the
// comment as a desktop update.
func TestSignAndVerifyCarriesAnotherProductsComment(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	comment, err := checkedComment("agentry-cli", "0.1.0", "abc1234")
	if err != nil || comment != "agentry-cli 0.1.0 (commit abc1234)" {
		t.Fatalf("checkedComment = %q, %v", comment, err)
	}
	sig, _, err := signAndVerify(priv, sums, comment, set)
	if err != nil {
		t.Fatalf("signAndVerify: %v", err)
	}
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		t.Fatalf("the signature does not parse: %v", err)
	}
	if parsed.TrustedComment != comment {
		t.Errorf("trusted comment = %q", parsed.TrustedComment)
	}
	// The bytes are validly signed by the estate's key (Verify says so) —
	// and the desktop's update path, which reads the comment next, refuses
	// it as a release of another product.
	if _, err := set.Verify(sums, sig); err != nil {
		t.Errorf("a CLI release signed by the estate's key does not verify: %v", err)
	}
	if _, _, err := selfupdate.ParseTrustedComment(parsed.TrustedComment); err == nil {
		t.Errorf("the desktop's update path read a CLI release's comment as its own")
	}
	if c, err := checkedComment("agentry-desktop", "0.2.0", "abc1234"); err != nil || c != "agentry-desktop 0.2.0 (commit abc1234)" {
		t.Errorf("the default product's comment changed shape: %q, %v", c, err)
	}
	for _, bad := range []string{"", "Agentry", "agentry desktop", "agentry_cli"} {
		if _, err := checkedComment(bad, "0.1.0", "abc1234"); err == nil {
			t.Errorf("checkedComment(%q) accepted a product name outside the format", bad)
		}
	}
}

// The misuse the product flag invites: one product's sums signed under the
// other's comment. The artifact names say which product a sums file is.
func TestSumsNameProduct(t *testing.T) {
	desktop := []byte("aa1f56ffcb84076209f4e5cf3eb0282c  agentry-desktop-0.2.0-darwin-universal.dmg\nbb  BUILD-INFO\n")
	cli := []byte("cc  agentry-0.1.0-linux-amd64\ndd  agentry-0.1.0-windows-amd64.exe\nee  BUILD-INFO\n")
	if err := sumsNameProduct(desktop, "agentry-desktop"); err != nil {
		t.Errorf("the desktop's sums refused for the desktop: %v", err)
	}
	if err := sumsNameProduct(cli, "agentry-cli"); err != nil {
		t.Errorf("the CLI's sums refused for the CLI: %v", err)
	}
	if err := sumsNameProduct(cli, "agentry-desktop"); err == nil {
		t.Errorf("the CLI's sums were accepted under the desktop's comment")
	}
	if err := sumsNameProduct(desktop, "agentry-cli"); err == nil {
		t.Errorf("the desktop's sums were accepted under the CLI's comment")
	}
	if err := sumsNameProduct([]byte("ff  other-thing-1.0\n"), "other-thing"); err != nil {
		t.Errorf("a future product's own sums refused: %v", err)
	}
	if err := sumsNameProduct([]byte("ff  agentry-0.1.0-linux-amd64\n"), "other-thing"); err == nil {
		t.Errorf("the CLI's sums were accepted under a future product's comment")
	}
}

// The mistake a rotation invites: a new private half loaded into the Actions
// secret while the matching public half was never committed. Signing succeeds;
// the release is useless. This must fail the release, not ship it.
func TestSignAndVerifyRefusesAKeyThatIsNotTheCommittedOnesOtherHalf(t *testing.T) {
	stranger, _ := keystest.Key(t, "not-committed", false)
	_, committed := keystest.Key(t, "prod", false)
	set := keystest.Set(t, committed)

	_, _, err := signAndVerify(stranger, sums, "x", set)
	if err == nil {
		t.Fatal("a release signed by a key no shipped app carries was accepted")
	}
	if !strings.Contains(err.Error(), "two halves of one pair") {
		t.Errorf("message %q does not say what went wrong", err)
	}
}

// A retired key may prove an OLD release genuine; it must never authorise a new
// one, or retiring a key would mean nothing.
func TestSignAndVerifyRefusesARetiredKey(t *testing.T) {
	priv, entry := keystest.Key(t, "dev", true)
	set := keystest.Set(t, entry)

	_, _, err := signAndVerify(priv, sums, "x", set)
	if err == nil {
		t.Fatal("a retired key signed a new release")
	}
	if !strings.Contains(err.Error(), "retired") {
		t.Errorf("message %q does not name the reason", err)
	}
}

func TestSignAndVerifyRefusesWhenTheBuildCarriesNoKey(t *testing.T) {
	priv, _ := keystest.Key(t, "prod", false)

	_, _, err := signAndVerify(priv, sums, "x", keystest.Set(t))
	if !errors.Is(err, selfupdate.ErrNoKeys) {
		t.Fatalf("err = %v, want ErrNoKeys", err)
	}
	if !strings.Contains(err.Error(), "update-signing-key.sh") {
		t.Errorf("message %q does not say how to fix it", err)
	}
}

// Nothing is written until it has verified, so a failed release never leaves a
// signature file that a later step might pick up and publish.
func TestNoSignatureIsWrittenWhenVerificationFails(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(in, sums, 0o644); err != nil {
		t.Fatal(err)
	}
	stranger, _ := keystest.Key(t, "stranger", false)

	sig, _, err := signAndVerify(stranger, sums, "x", keystest.Set(t))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if sig != nil {
		t.Error("a signature was returned although verification failed")
	}
	if _, err := os.Stat(in + ".minisig"); !os.IsNotExist(err) {
		t.Error("a signature file exists although verification failed")
	}
}

func TestReadKeyHandlesBothKeyShapes(t *testing.T) {
	dir := t.TempDir()
	_, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unencrypted, as the release workflow's secret is", func(t *testing.T) {
		text, err := priv.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "plain.key")
		if err := os.WriteFile(path, text, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readKey(path, "")
		if err != nil {
			t.Fatalf("readKey: %v", err)
		}
		if !got.Equal(priv) {
			t.Error("the key read back is not the key written")
		}
	})

	t.Run("password-protected, as an operator-held key may be", func(t *testing.T) {
		enc, err := minisign.EncryptKey("a passphrase", priv)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "sealed.key")
		if err := os.WriteFile(path, enc, 0o600); err != nil {
			t.Fatal(err)
		}
		passPath := filepath.Join(dir, "pass")
		// With the trailing newline `echo` would leave, which must be ignored.
		if err := os.WriteFile(passPath, []byte("a passphrase\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readKey(path, passPath)
		if err != nil {
			t.Fatalf("readKey: %v", err)
		}
		if !got.Equal(priv) {
			t.Error("the key read back is not the key written")
		}

		if _, err := readKey(path, ""); err == nil {
			t.Error("a password-protected key was read with no password")
		}
		wrong := filepath.Join(dir, "wrong")
		if err := os.WriteFile(wrong, []byte("nope"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readKey(path, wrong); err == nil {
			t.Error("a password-protected key was read with the wrong password")
		}
	})

	t.Run("a password given for a key that has none is refused, not ignored", func(t *testing.T) {
		text, _ := priv.MarshalText()
		path := filepath.Join(dir, "plain2.key")
		if err := os.WriteFile(path, text, 0o600); err != nil {
			t.Fatal(err)
		}
		passPath := filepath.Join(dir, "pass2")
		if err := os.WriteFile(passPath, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readKey(path, passPath); err == nil {
			t.Error("a password was silently ignored, which hides a mismatched key")
		}
	})
}

// The release workflow pipes the key on standard input so it never reaches the
// runner host's disk. That path has no secret to exercise it in CI, so it is
// exercised here.
func TestReadKeyFromStandardInput(t *testing.T) {
	_, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	text, err := priv.MarshalText()
	if err != nil {
		t.Fatal(err)
	}

	// With the trailing newline a shell, a secret store or an editor adds, and
	// which the key itself does not carry.
	for _, suffix := range []string{"", "\n", "\r\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			_, _ = w.Write(append(append([]byte{}, text...), []byte(suffix)...))
			_ = w.Close()
		}()
		saved := os.Stdin
		os.Stdin = r
		got, err := readKey("-", "")
		os.Stdin = saved
		_ = r.Close()
		if err != nil {
			t.Fatalf("readKey(\"-\") with suffix %q: %v", suffix, err)
		}
		if !got.Equal(priv) {
			t.Errorf("suffix %q: the key read back is not the key written", suffix)
		}
	}
}

func TestReadKeyRefusesAnEmptyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readKey(path, ""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want an empty-key refusal", err)
	}
}
