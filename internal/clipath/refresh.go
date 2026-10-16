package clipath

// The refresh. The site's install script puts the CLI first on the person's
// PATH, at its own default place, and the desktop appends its copy so that a
// CLI installed on purpose wins. Nothing else updates the script's copy, so
// it would win forever at the version it was installed at: a terminal's
// `agentry --version` would report an old version under a newer desktop, and
// an MCP host registered with the script's AGENTRY_BIN would run the same
// stale copy. The desktop replaces that copy with its own when the copy is
// older. It never downgrades, and it leaves alone anything it cannot read the
// version of, so a newer CLI installed on purpose still wins.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Outcome says what Refresh did.
type Outcome int

const (
	// Nothing: no script-installed CLI, or no CLI carried by this build.
	Nothing Outcome = iota
	// Current: the script's copy is the carried CLI's bytes already.
	Current
	// Kept: the script's copy is as new or newer, or could not be read; it
	// stays as it is.
	Kept
	// Replaced: the script's copy was older and now holds the carried CLI.
	Replaced
)

// Result is what Refresh found and did, for the log line.
type Result struct {
	Outcome      Outcome
	Target       string // the script's copy
	TargetVer    string // the script's copy's version before, "" when unread
	BundledVer   string
	KeptBecause  string // why a Kept copy was left alone
	OldLeftAside bool   // Windows: the replaced copy was running and is left as <target>.old
}

func (r Result) String() string {
	switch r.Outcome {
	case Nothing:
		return "no agentry CLI from the install script to refresh"
	case Current:
		return fmt.Sprintf("%s is the CLI this app carries", r.Target)
	case Kept:
		return fmt.Sprintf("kept %s: %s", r.Target, r.KeptBecause)
	default:
		s := fmt.Sprintf("replaced %s (%s) with the agentry CLI this app carries (%s)", r.Target, r.TargetVer, r.BundledVer)
		if r.OldLeftAside {
			s += "; the old copy was running and is left beside it as .old"
		}
		return s
	}
}

// versionLine matches the CLI's `agentry version 0.6.87 default registry: …`.
var versionLine = regexp.MustCompile(`(?m)^agentry version (\S+)`)

// parseVersion reads the CLI's version out of its `--version` output.
func parseVersion(out []byte) (*semver.Version, error) {
	m := versionLine.FindSubmatch(out)
	if m == nil {
		return nil, errors.New("its --version does not name an agentry version")
	}
	v, err := semver.NewVersion(string(m[1]))
	if err != nil {
		return nil, fmt.Errorf("its version %q is not semver: %w", m[1], err)
	}
	return v, nil
}

// VersionFunc runs a CLI's `--version` and returns its output.
type VersionFunc func(path string) ([]byte, error)

// runVersion runs path --version with a short deadline and no window.
func runVersion(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	hideWindow(cmd)
	return cmd.Output()
}

// Refresh replaces target, the CLI the site's install script put at its
// default place, with bundled, the CLI this build carries, when target's
// version is strictly lower. It leaves target alone when either is missing,
// when target is a symlink or the same file as bundled, when either version
// cannot be read, and when target is as new or newer. version runs a CLI's
// --version (runVersion outside tests).
func Refresh(bundled, target string, version VersionFunc) (Result, error) {
	r := Result{Target: target}
	if bundled == "" || target == "" {
		return r, nil
	}
	bi, err := os.Stat(bundled)
	if err != nil || !bi.Mode().IsRegular() {
		return r, nil // a build that carries no CLI
	}
	ti, err := os.Lstat(target)
	if err != nil || ti.Mode()&os.ModeSymlink != 0 || !ti.Mode().IsRegular() {
		return r, nil // no script copy, or a link someone made on purpose
	}
	if os.SameFile(bi, ti) {
		return r, nil
	}
	if same, err := sameBytes(bundled, target); err == nil && same {
		r.Outcome = Current
		return r, nil
	}
	keep := func(why string) (Result, error) {
		r.Outcome, r.KeptBecause = Kept, why
		return r, nil
	}
	out, err := version(bundled)
	if err != nil {
		return keep("the carried CLI's version could not be read: " + err.Error())
	}
	bv, err := parseVersion(out)
	if err != nil {
		return keep("the carried CLI: " + err.Error())
	}
	r.BundledVer = bv.Original()
	out, err = version(target)
	if err != nil {
		return keep("its version could not be read: " + err.Error())
	}
	tv, err := parseVersion(out)
	if err != nil {
		return keep(err.Error())
	}
	r.TargetVer = tv.Original()
	if !tv.LessThan(bv) {
		return keep(fmt.Sprintf("its version %s is not older than %s", r.TargetVer, r.BundledVer))
	}
	aside, err := replace(bundled, target, ti.Mode().Perm())
	if err != nil {
		return r, fmt.Errorf("replace %s (%s) with %s (%s): %w", target, r.TargetVer, bundled, r.BundledVer, err)
	}
	r.Outcome, r.OldLeftAside = Replaced, aside
	return r, nil
}

// replace copies bundled into a hidden file beside target and renames it
// into place, so a reader sees either the old CLI or the new one, never half
// of it. A running exe on Windows cannot be replaced in place, so it is first
// renamed aside to <target>.old and removed when nothing holds it; aside
// reports one that could not be removed yet.
func replace(bundled, target string, perm os.FileMode) (aside bool, err error) {
	src, err := os.Open(bundled)
	if err != nil {
		return false, err
	}
	defer func() { _ = src.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".new-*")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Close(); err != nil {
		return false, err
	}
	if perm&0o111 == 0 {
		perm = 0o755
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return false, err
	}
	var same bool
	if same, err = sameBytes(bundled, tmpName); err == nil && !same {
		err = errors.New("the copy differs from the carried CLI")
	}
	if err != nil {
		return false, err
	}
	return swapIn(tmpName, target)
}

// sameBytes reports whether two files hold the same bytes.
func sameBytes(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if ai.Size() != bi.Size() {
		return false, nil
	}
	ha, err := fileSHA256(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSHA256(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ha, hb), nil
}

func fileSHA256(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// bundledCLI is the CLI this build carries, beside the running executable:
// <dir>\bin\agentry.exe on Windows (the installer's $INSTDIR\bin), <dir>/agentry
// on Linux (/usr/bin/agentry beside /usr/bin/agentry-desktop), and the
// bundle's Contents/Resources/bin/agentry on macOS.
func bundledCLI() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return bundledBeside(filepath.Dir(exe))
}

// RefreshInstalled runs Refresh for this machine: the CLI this build carries
// against the install script's default place. It is called by the
// installer's AddFlag run (install and every silent self-update on Windows)
// and once at app start.
func RefreshInstalled() (Result, error) {
	if !refreshAllowed() {
		return Result{}, nil
	}
	return Refresh(bundledCLI(), scriptCLI(os.Getenv), runVersion)
}
