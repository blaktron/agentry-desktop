// Command verifysums checks a release's SHA256SUMS against its
// SHA256SUMS.minisig with the key set this build carries
// (internal/selfupdate/keys.json), and that the signed trusted comment names
// the product and version expected. scripts/fetch-cli.sh runs it on the CLI
// release the desktop carries, when that release is signed.
//
//	go run ./internal/cmd/verifysums -product agentry-cli -version 0.6.52 SHA256SUMS SHA256SUMS.minisig
//
// A retired key is refused: it may prove an old release genuine and may
// never put one into a new build (VerifyForInstall).
package main

import (
	"flag"
	"fmt"
	"os"

	"aead.dev/minisign"
	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

func main() {
	product := flag.String("product", "", "the product the release must be of (required)")
	version := flag.String("version", "", "the version the release must be (required)")
	flag.Parse()
	if *product == "" || *version == "" || flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: verifysums -product P -version V SHA256SUMS SHA256SUMS.minisig")
		os.Exit(2)
	}
	sums, err := os.ReadFile(flag.Arg(0))
	if err == nil {
		var sig []byte
		if sig, err = os.ReadFile(flag.Arg(1)); err == nil {
			var keys *selfupdate.KeySet
			if keys, err = selfupdate.Carried(); err == nil {
				err = verify(keys, sums, sig, *product, *version)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "verifysums: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("verifysums: SHA256SUMS of %s %s is signed with a current key of this build's key set\n", *product, *version)
}

// verify is main's check, over a key set the tests can supply.
func verify(keys *selfupdate.KeySet, sums, sig []byte, product, version string) error {
	if _, err := keys.VerifyForInstall(sums, sig); err != nil {
		return err
	}
	var s minisign.Signature
	if err := s.UnmarshalText(sig); err != nil {
		return err
	}
	p, v, _, err := selfupdate.ParseAnyTrustedComment(s.TrustedComment)
	if err != nil {
		return err
	}
	if p != product || v != version {
		return fmt.Errorf("the signature is over %s %s, not %s %s", p, v, product, version)
	}
	return nil
}
