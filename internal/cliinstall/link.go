package cliinstall

// The agentry command the macOS app carries. The bundle holds the CLI as
// Contents/Resources/bin/agentry, signed and notarized with the app, so the
// command is a symbolic link to it rather than a copy: an update that
// replaces the bundle updates the command with it, and the command is always
// the CLI version the app was built against. The link goes where the site's
// install script puts the command (Dir), and the folder goes on PATH with the
// script's own marked line in the same startup file, so the script and the
// app never write it twice.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrNotOurs: an agentry at the command's place that is not a link to a
// CLI an Agentry Desktop bundle carries, which the app does not replace.
var ErrNotOurs = errors.New("an agentry command the app did not put there is in the way")

// ErrNotInstalled: the app is running from the disk image or from the copy
// macOS makes of an app it has not moved yet, a path that is gone once the
// app quits, so a link to its CLI would point at nothing.
var ErrNotInstalled = errors.New("the app is not running from your Applications folder: move Agentry Desktop there, open it from there, and try again")

// Carried is the CLI the macOS bundle that exe belongs to carries:
// <bundle>.app/Contents/Resources/bin/agentry for an executable at
// <bundle>.app/Contents/MacOS/<name>, "" for any other path.
func Carried(exe string) string {
	macos := filepath.Dir(filepath.Clean(exe))
	contents := filepath.Dir(macos)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
		return ""
	}
	return filepath.Join(contents, "Resources", "bin", Name)
}

// isCarried says whether p is the path of a CLI some bundle carries.
func isCarried(p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	bin := filepath.Dir(p)
	res := filepath.Dir(bin)
	contents := filepath.Dir(res)
	return filepath.Base(p) == Name && filepath.Base(bin) == "bin" && filepath.Base(res) == "Resources" &&
		filepath.Base(contents) == "Contents" && strings.HasSuffix(filepath.Dir(contents), ".app")
}

// installedPath says whether carried is somewhere a link to it outlives the
// app: not on a mounted disk image, not in a translocated copy.
func installedPath(carried string) bool {
	return !strings.HasPrefix(carried, "/Volumes/") && !strings.Contains(carried, "/AppTranslocation/")
}

// LinkStatus is the command as the app finds it.
type LinkStatus struct {
	// Carried is the CLI this bundle carries.
	Carried string `json:"carried"`
	// Path is where the command goes (Path's place, ~/.local/bin/agentry).
	Path string `json:"path"`
	// Linked: Path is a link to Carried.
	Linked bool `json:"linked"`
	// Other: something else is at Path, which the app leaves alone.
	Other bool `json:"other"`
	// Installable: the app runs from a place a link can point at.
	Installable bool `json:"installable"`
	// Resolves is what `agentry` is in a new terminal of the person's shell,
	// "" when it finds none or could not be asked.
	Resolves string `json:"resolves,omitempty"`
	// ResolvesTo is Resolves with its links followed.
	ResolvesTo string `json:"resolvesTo,omitempty"`
}

// Status reads the command's state for the CLI carried, with resolve
// answering what a new terminal finds as `agentry` (ShellLookup outside
// tests; nil skips the question).
func Status(carried string, getenv func(string) string, resolve func() string) LinkStatus {
	s := LinkStatus{Carried: carried, Path: Path(getenv), Installable: carried != "" && installedPath(carried)}
	if s.Path != "" {
		if fi, err := os.Lstat(s.Path); err == nil {
			dest, _ := os.Readlink(s.Path)
			s.Linked = fi.Mode()&os.ModeSymlink != 0 && dest == carried
			s.Other = !s.Linked && (fi.Mode()&os.ModeSymlink == 0 || !isCarried(dest))
		}
	}
	if resolve != nil {
		if p := resolve(); p != "" {
			s.Resolves = p
			if to, err := filepath.EvalSymlinks(p); err == nil && to != p {
				s.ResolvesTo = to
			}
		}
	}
	return s
}

// Link puts the command on PATH for the CLI carried: a link at Path to it,
// replacing only a link to a CLI another (or an older) Agentry Desktop
// bundle carries, and, when its folder is not on shellPath (what a new
// terminal's PATH is, "" when unknown), the install script's marked PATH
// line in the shell's startup file, once. It touches nothing else.
func Link(carried string, getenv func(string) string, shellPath string) (Result, error) {
	var r Result
	if carried == "" {
		return r, errors.New("this app carries no agentry command")
	}
	if fi, err := os.Stat(carried); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return r, fmt.Errorf("the agentry command this app carries is missing at %s", carried)
	}
	if !installedPath(carried) {
		return r, ErrNotInstalled
	}
	dir := Dir(getenv)
	if dir == "" {
		return r, errors.New("there is no home folder to put the agentry command in")
	}
	target := filepath.Join(dir, Name)
	r.Path = target
	var log strings.Builder
	say := func(note bool, f string, a ...any) {
		line := fmt.Sprintf(f, a...)
		log.WriteString(line + "\n")
		if note {
			r.Notes = append(r.Notes, line)
		}
	}
	fi, err := os.Lstat(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return r, err
	case fi.Mode()&os.ModeSymlink == 0:
		return r, fmt.Errorf("%w: %s is not a link to the command this app carries", ErrNotOurs, target)
	default:
		dest, err := os.Readlink(target)
		if err != nil {
			return r, err
		}
		if !isCarried(dest) {
			return r, fmt.Errorf("%w: %s links to %s", ErrNotOurs, target, dest)
		}
	}
	if dest, _ := os.Readlink(target); dest == carried {
		say(false, "%s already links to %s", target, carried)
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return r, err
		}
		// Made beside the target and renamed over it, so a terminal never
		// finds the command missing.
		tmp := filepath.Join(dir, ".agentry.link."+strconv.Itoa(os.Getpid()))
		_ = os.Remove(tmp)
		if err := os.Symlink(carried, tmp); err != nil {
			return r, err
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return r, err
		}
		say(false, "Linked %s to %s", target, carried)
	}
	if onPath(dir, shellPath) {
		r.Log = log.String()
		return r, nil
	}
	rc, line := chooseRC(dir, getenv)
	if rc == "" {
		say(true, "Add %s to your PATH in your shell's startup file; the app writes sh, bash, zsh and fish ones only.", dir)
		r.Log = log.String()
		return r, nil
	}
	added, err := addLine(rc, "# Added by the agentry CLI installer: "+dir, line)
	if err != nil {
		return r, fmt.Errorf("could not add %s to PATH in %s: %w", dir, rc, err)
	}
	if added {
		say(true, "Added %s to PATH in %s", dir, rc)
		say(true, "Open a new terminal to use agentry; a terminal that is already open does not read it.")
	}
	r.Log = log.String()
	return r, nil
}

// onPath says whether dir is one of path's entries.
func onPath(dir, path string) bool {
	for _, p := range filepath.SplitList(path) {
		if p != "" && filepath.Clean(p) == dir {
			return true
		}
	}
	return false
}

// chooseRC is the startup file a new terminal of the person's shell reads on
// macOS and the line that puts dir on PATH there, as the install script
// chooses them; "" for a shell it does not write (csh, tcsh).
func chooseRC(dir string, getenv func(string) string) (string, string) {
	home := getenv("HOME")
	shown := dir
	if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		shown = "$HOME/" + rel
	}
	line := `export PATH="` + shown + `:$PATH"`
	shell := filepath.Base(getenv("SHELL"))
	if shell == "." || shell == "" {
		shell = "sh"
	}
	switch shell {
	case "zsh":
		if z := getenv("ZDOTDIR"); z != "" {
			return filepath.Join(z, ".zshrc"), line
		}
		return filepath.Join(home, ".zshrc"), line
	case "bash":
		profile := filepath.Join(home, ".profile")
		bashProfile := filepath.Join(home, ".bash_profile")
		if exists(bashProfile) || !exists(profile) {
			return bashProfile, line
		}
		// Creating .bash_profile would stop bash reading the .profile there.
		return profile, line
	case "fish":
		conf := getenv("XDG_CONFIG_HOME")
		if conf == "" {
			conf = filepath.Join(home, ".config")
		}
		return filepath.Join(conf, "fish", "conf.d", "agentry.fish"), `fish_add_path -g "` + dir + `"`
	case "csh", "tcsh":
		return "", ""
	}
	return filepath.Join(home, ".profile"), line
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// addLine appends the marked line to rc unless rc already has the marker,
// and says whether it wrote.
func addLine(rc, marker, line string) (bool, error) {
	if b, err := os.ReadFile(rc); err == nil && bytes.Contains(b, []byte(marker)) {
		return false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(rc, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(f, "\n%s\n%s\n", marker, line); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}

// sentinel marks the line the shell printed for the app, among whatever its
// startup files print.
const sentinel = "__agentry_desktop__"

// ShellLine runs script in a new interactive login shell of shell, as a new
// terminal starts one, and returns the line it printed after the sentinel;
// "" when the shell could not be asked. A GUI app's own PATH is not the one a
// terminal has, so this is how the app sees what a terminal would.
func ShellLine(ctx context.Context, shell, script string) string {
	if shell == "" || !filepath.IsAbs(shell) {
		return ""
	}
	switch filepath.Base(shell) {
	case "csh", "tcsh":
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-i", "-l", "-c", `printf '\n`+sentinel+`%s\n' "$(`+script+`)"`)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), sentinel); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ShellPath is a new terminal's PATH.
func ShellPath(ctx context.Context, shell string) string {
	return ShellLine(ctx, shell, `printf %s "$PATH"`)
}

// ShellLookup is what a new terminal runs as `agentry`.
func ShellLookup(ctx context.Context, shell string) string {
	p := ShellLine(ctx, shell, "command -v "+Name)
	if !filepath.IsAbs(p) {
		return ""
	}
	return p
}
