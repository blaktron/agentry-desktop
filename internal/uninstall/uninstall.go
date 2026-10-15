// Package uninstall is the app's half of the Windows uninstaller's
// optional boxes: "Also remove my Agentry settings and sign-in", here,
// and "Keep my Docker images", whose unticked default removes Agentry's
// Docker images (images.go).
//
// The installer is a file-copy program: it does not know where the
// configuration lives, that AGENTRY_CONFIG_DIR and AGENTRY_RUNNER_HOME
// relocate it, or which keychain entries the two clients wrote. The app
// does, so the uninstaller runs `agentry-desktop.exe --remove-my-settings`
// while the binary is still installed and removes the application
// afterwards. Without the box, and under /S, this code never runs: an
// uninstall or an upgrade leaves the person's configuration alone.
//
// What goes, and only this: the agentry configuration directory
// (desktop.yaml, the CLI's config.yaml, installed.yaml, pinned keys), the
// runner home (policy.yaml, profiles, the image context), and every entry
// under the `agentry` keychain service (registry tokens of both clients,
// the model keys the CLI stores as provider:<name>). The CLI shares all
// three, and the uninstaller's page says so.
package uninstall

import (
	"fmt"
	"io"
	"os"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-desktop/internal/pathguard"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Flag is the command-line form the uninstaller invokes the app with.
const Flag = "--remove-my-settings"

// Main removes the three things and reports each to w. It returns 0 when
// everything was removed and 1 when anything remains, which the
// uninstaller turns into a message naming the places to look. A GUI
// process on Windows has no console, so w is best effort there.
func Main(w io.Writer) int {
	removed, problems := Remove()
	for _, r := range removed {
		_, _ = fmt.Fprintf(w, "removed %s\n", r)
	}
	for _, p := range problems {
		_, _ = fmt.Fprintf(w, "not removed: %v\n", p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// Remove deletes the configuration directory, the runner home and the
// agentry keychain entries. Each removal stands on its own: a keychain that
// cannot be reached does not keep the files, and a file tree that cannot be
// deleted does not keep the tokens. A tree that is not there and a keychain
// with nothing under the service are not problems.
func Remove() (removed []string, problems []error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, []error{fmt.Errorf("locate home directory: %w", err)}
	}

	trees := []struct {
		what string
		dir  func() (string, error)
	}{
		{"configuration directory", configdir.Dir},
		{"runner home", policy.Home},
	}
	for _, t := range trees {
		dir, err := t.dir()
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", t.what, err))
			continue
		}
		if err := guard(dir, home); err != nil {
			problems = append(problems, fmt.Errorf("%s %s: refused: %w", t.what, dir, err))
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			problems = append(problems, fmt.Errorf("%s %s: %w", t.what, dir, err))
			continue
		}
		removed = append(removed, fmt.Sprintf("%s %s", t.what, dir))
	}

	if err := settings.ForgetKeychain(); err != nil {
		problems = append(problems, fmt.Errorf("keychain entries under service %q: %w", settings.KeyringService, err))
	} else {
		removed = append(removed, fmt.Sprintf("keychain entries under service %q", settings.KeyringService))
	}
	return removed, problems
}

// guard is pathguard.RefuseDangerousDir: the refusal install's managed
// clear shares.
func guard(dir, home string) error { return pathguard.RefuseDangerousDir(dir, home) }
