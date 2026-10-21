package run

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// The Coding agent's dev server is readied as the CLI's configure does: the
// unknown host key put to the dialog, the key pinned in the answer, the
// sign-in saved in the keychain as the key's path, and no credential in
// what is answered. A key file the picker did not give is refused.
func TestTheDevServerIsReadiedPinnedAndItsSignInSaved(t *testing.T) {
	home := hostKeychain(t)
	h := startSSHHost(t)
	key := keyFile(t, testPassphrase)
	d := &dialog{answer: true}
	dev, err := ReadyDevHost(context.Background(), h.answer(), HostSignIn{KeyFile: key, Passphrase: testPassphrase},
		DevHost{Pol: policy.Default(), Home: home, Picked: pickedOnly(key), Trust: d.trust})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Host != h.answer() || dev.HostKey != h.fingerprint() {
		t.Fatalf("the dev server %+v, want %s pinned to %s", dev, h.answer(), h.fingerprint())
	}
	if len(d.asked) != 1 || !strings.Contains(d.asked[0], h.fingerprint()) {
		t.Fatalf("the dialog was asked %q", d.asked)
	}
	noCredential(t, "the answer", fmt.Sprintf("%+v", dev))
	v, err := keyring.Get(modelkey.Service, "host:"+h.answer())
	if err != nil || !strings.Contains(v, key) || strings.Contains(v, "PRIVATE KEY") {
		t.Fatalf("the keychain entry (%v)", err)
	}

	if _, err := ReadyDevHost(context.Background(), h.answer(), HostSignIn{KeyFile: key},
		DevHost{Pol: policy.Default(), Home: home, Picked: pickedOnly(), Trust: d.trust}); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("a key the picker did not give: %v", err)
	}
	// A key the person declines is no dev server.
	other := startSSHHost(t)
	if _, err := ReadyDevHost(context.Background(), other.answer(), HostSignIn{Password: testHostPassword},
		DevHost{Pol: policy.Default(), Home: home, Trust: (&dialog{}).trust}); err == nil {
		t.Fatal("a declined host key was taken")
	} else {
		noCredential(t, "the refusal", err.Error())
	}
}
