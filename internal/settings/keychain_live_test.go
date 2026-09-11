package settings

import (
	"fmt"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestKeychainRoundTrip exercises the OS credential store the token
// functions use — on macOS the Keychain through the Security framework —
// rather than merely compiling against it. It is opt-in (AGENTRY_KEYCHAIN_TEST=1)
// because a developer's machine may have no unlocked keychain, and it uses a
// host name no registry can have, so it never reads or disturbs a real
// credential. The macOS CI job runs it (agentry-desktop#57).
func TestKeychainRoundTrip(t *testing.T) {
	if os.Getenv("AGENTRY_KEYCHAIN_TEST") != "1" {
		t.Skip("set AGENTRY_KEYCHAIN_TEST=1 to exercise the OS keychain")
	}
	host := fmt.Sprintf("keychain-test-%d.invalid", os.Getpid())
	const secret = "not-a-real-token"
	if err := keyring.Set(keyringService, host, secret); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	t.Cleanup(func() { _ = keyring.Delete(keyringService, host) })
	got, err := keyring.Get(keyringService, host)
	if err != nil {
		t.Fatalf("keyring.Get: %v", err)
	}
	if got != secret {
		t.Fatalf("keyring.Get = %q, want %q", got, secret)
	}
	if err := keyring.Delete(keyringService, host); err != nil {
		t.Fatalf("keyring.Delete: %v", err)
	}
	if _, err := keyring.Get(keyringService, host); err == nil {
		t.Fatalf("keyring.Get after Delete: still present")
	}
}
