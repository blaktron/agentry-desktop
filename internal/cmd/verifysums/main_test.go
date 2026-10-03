package main

import (
	"strings"
	"testing"

	"aead.dev/minisign"
	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

func TestVerifyTakesOnlyACurrentKeysSignatureOverTheExpectedRelease(t *testing.T) {
	current, currentEntry := keystest.Key(t, "current", false)
	retired, retiredEntry := keystest.Key(t, "retired", true)
	keys := keystest.Set(t, currentEntry, retiredEntry)
	sums := []byte("abc  agentry-0.6.52-linux-amd64\n")
	comment := selfupdate.TrustedCommentFor("agentry-cli", "0.6.52", "bacfff4")
	good := minisign.SignWithComments(current, sums, comment, "")

	if err := verify(keys, sums, good, "agentry-cli", "0.6.52"); err != nil {
		t.Fatalf("a current key's signature over the expected release: %v", err)
	}
	cases := []struct {
		name string
		sums []byte
		sig  []byte
		prod string
		ver  string
		want string
	}{
		{"another version", sums, good, "agentry-cli", "0.6.53", "not agentry-cli 0.6.53"},
		{"another product", sums, good, "agentry-desktop", "0.6.52", "not agentry-desktop 0.6.52"},
		{"changed sums", []byte("def  agentry-0.6.52-linux-amd64\n"), good, "agentry-cli", "0.6.52", "signature"},
		{"a retired key", sums, minisign.SignWithComments(retired, sums, comment, ""), "agentry-cli", "0.6.52", "retired"},
	}
	for _, c := range cases {
		err := verify(keys, c.sums, c.sig, c.prod, c.ver)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}
