package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"
)

// DevHost is what readying the Coding agent's dev server needs besides the
// address and the sign-in.
type DevHost struct {
	Pol  *policy.Policy
	Home string
	// Picked judges a key file's path: one a native picker returned, or it
	// is refused.
	Picked func(what, path string) error
	// Trust asks the person whether to accept a host key no known_hosts file
	// holds, in a native dialog.
	Trust func(prompt string) (bool, error)
	Say   func(string)
}

// ReadyDevHost readies the Coding agent's dev server as `agentry agent
// configure coder --dev-host` does at the terminal: the address under the
// policy's host lists, its host key checked or put to the person, and the
// sign-in signed in with and saved in the system keychain, where the agent's
// segments read it. It answers the server as the agent's config keeps it,
// its host key pinned. The sign-in travels inward only and is never
// answered.
func ReadyDevHost(ctx context.Context, answer string, sign HostSignIn, d DevHost) (*persistent.DevHost, error) {
	a, err := targethost.ParseAnswer(strings.TrimSpace(answer))
	if err != nil {
		return nil, err
	}
	if k := strings.TrimSpace(sign.KeyFile); k != "" && d.Picked != nil {
		if err := d.Picked("the dev server's key file", k); err != nil {
			return nil, err
		}
	}
	if d.Pol == nil {
		return nil, errors.New("no runner policy to read the host lists from")
	}
	flags, getenv := hostFlags(map[string]HostSignIn{spec.DevHostID: sign})
	say := d.Say
	if say == nil {
		say = func(string) {}
	}
	o := targethost.Options{
		Hosts: d.Pol.Hosts, KnownHosts: targethost.DefaultKnownHostsFiles(d.Home), Flags: flags,
		Getenv: getenv, Say: say, Keep: true,
	}
	if d.Trust != nil {
		o.Ask = setupAsker{trust: d.Trust}
	}
	ready, err := prepareTargetHost(ctx, spec.DevHostID, a, o)
	if err != nil {
		if errors.Is(err, errNoSignIn) {
			return nil, fmt.Errorf("the dev server %s: no sign-in was given and none is saved for it: choose a key file or type a password", a)
		}
		return nil, fmt.Errorf("the dev server %s: %w", a, err)
	}
	return &persistent.DevHost{Host: ready.Answer.String(), HostKey: ssh.FingerprintSHA256(ready.HostKey.Key)}, nil
}
