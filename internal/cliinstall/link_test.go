package cliinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// bundle makes a fake <root>/<name>.app carrying an agentry CLI and returns
// the CLI's path.
func bundle(t *testing.T, root, name string) string {
	t.Helper()
	cli := filepath.Join(root, name+".app", "Contents", "Resources", "bin", Name)
	if err := os.MkdirAll(filepath.Dir(cli), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cli, []byte("#!/bin/sh\necho agentry version 0.6.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return cli
}

// tree lists every path under root with what it is: a file's content, a
// link's target, or a directory.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			dest, _ := os.Readlink(p)
			out[rel] = "-> " + dest
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, _ := os.ReadFile(p)
			out[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// changed lists the paths whose entry differs between two trees.
func changed(before, after map[string]string) []string {
	var d []string
	for k, v := range after {
		if before[k] != v {
			d = append(d, k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			d = append(d, k+" (removed)")
		}
	}
	sort.Strings(d)
	return d
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestCarried(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Agentry Desktop.app/Contents/MacOS/agentry-desktop": "/Applications/Agentry Desktop.app/Contents/Resources/bin/agentry",
		"/Users/a/Applications/x.app/Contents/MacOS/x":                     "/Users/a/Applications/x.app/Contents/Resources/bin/agentry",
		"/usr/bin/agentry-desktop":                                         "",
		"/Applications/x/Contents/MacOS/x":                                 "",
		"/Applications/x.app/MacOS/x":                                      "",
	} {
		if got := Carried(exe); got != want {
			t.Errorf("Carried(%q) = %q, want %q", exe, got, want)
		}
	}
}

// The link and the zsh line are made once: a second run changes nothing,
// and nothing else in the home folder is touched.
func TestLinkIsIdempotentAndTouchesNothingElse(t *testing.T) {
	home := t.TempDir()
	apps := t.TempDir()
	cli := bundle(t, apps, "Agentry Desktop")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("alias ll='ls -l'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh"})
	before := tree(t, home)

	r, err := Link(cli, env, "/usr/bin:/bin")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".local", "bin", Name)
	if r.Path != target {
		t.Fatalf("Path = %q, want %q", r.Path, target)
	}
	if dest, err := os.Readlink(target); err != nil || dest != cli {
		t.Fatalf("the link: %q %v, want %q", dest, err, cli)
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	want := "alias ll='ls -l'\n\n# Added by the agentry CLI installer: " + filepath.Join(home, ".local", "bin") + "\nexport PATH=\"$HOME/.local/bin:$PATH\"\n"
	if string(rc) != want {
		t.Fatalf(".zshrc:\n%s\nwant:\n%s", rc, want)
	}
	if got := changed(before, tree(t, home)); strings.Join(got, ",") != ".local,.local/bin,.local/bin/agentry,.zshrc" {
		t.Fatalf("the first run changed %v", got)
	}

	after := tree(t, home)
	r2, err := Link(cli, env, "/usr/bin:/bin")
	if err != nil {
		t.Fatal(err)
	}
	if got := changed(after, tree(t, home)); len(got) != 0 {
		t.Fatalf("the second run changed %v", got)
	}
	if len(r2.Notes) != 0 {
		t.Fatalf("the second run had something to say: %v", r2.Notes)
	}
}

// An agentry the app did not make, a file or a link elsewhere, is refused
// and left as it was, and so is everything else.
func TestLinkRefusesAnAgentryItDidNotCreate(t *testing.T) {
	apps := t.TempDir()
	cli := bundle(t, apps, "Agentry Desktop")
	for name, make := range map[string]func(target string) error{
		"a file": func(target string) error { return os.WriteFile(target, []byte("the script's copy"), 0o755) },
		"a link elsewhere": func(target string) error {
			return os.Symlink("/opt/homebrew/bin/agentry", target)
		},
		"a relative link": func(target string) error {
			return os.Symlink("x.app/Contents/Resources/bin/agentry", target)
		},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local", "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := make(filepath.Join(bin, Name)); err != nil {
				t.Fatal(err)
			}
			before := tree(t, home)
			_, err := Link(cli, envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh"}), "")
			if !errors.Is(err, ErrNotOurs) {
				t.Fatalf("err = %v, want ErrNotOurs", err)
			}
			if got := changed(before, tree(t, home)); len(got) != 0 {
				t.Fatalf("a refused link changed %v", got)
			}
		})
	}
}

// A link to the CLI another bundle carries (an older copy of the app, or
// the app at another place) is the app's own, and is moved to this one.
func TestLinkReplacesALinkToAnotherBundle(t *testing.T) {
	home := t.TempDir()
	apps := t.TempDir()
	old := bundle(t, apps, "Agentry Desktop old")
	cli := bundle(t, apps, "Agentry Desktop")
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(old, filepath.Join(bin, Name)); err != nil {
		t.Fatal(err)
	}
	env := envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh"})
	if s := Status(cli, env, nil); s.Linked || s.Other {
		t.Fatalf("before: %+v", s)
	}
	if _, err := Link(cli, env, bin); err != nil {
		t.Fatal(err)
	}
	if dest, _ := os.Readlink(filepath.Join(bin, Name)); dest != cli {
		t.Fatalf("the link points at %q, want %q", dest, cli)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder already on PATH got a .zshrc line: %v", err)
	}
	if s := Status(cli, env, nil); !s.Linked || s.Other {
		t.Fatalf("after: %+v", s)
	}
}

// The startup file per shell, as the install script picks it on macOS.
func TestLinkWritesTheShellsStartupFile(t *testing.T) {
	cases := []struct {
		shell, make, want, line string
	}{
		{"/bin/bash", "", ".bash_profile", `export PATH="$HOME/.local/bin:$PATH"`},
		{"/bin/bash", ".profile", ".profile", `export PATH="$HOME/.local/bin:$PATH"`},
		{"/opt/homebrew/bin/fish", "", ".config/fish/conf.d/agentry.fish", `fish_add_path -g "<bin>"`},
		{"/bin/sh", "", ".profile", `export PATH="$HOME/.local/bin:$PATH"`},
	}
	for _, c := range cases {
		t.Run(c.shell+" "+c.make, func(t *testing.T) {
			home := t.TempDir()
			cli := bundle(t, t.TempDir(), "Agentry Desktop")
			if c.make != "" {
				if err := os.WriteFile(filepath.Join(home, c.make), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Link(cli, envOf(map[string]string{"HOME": home, "SHELL": c.shell}), ""); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(home, c.want))
			line := strings.ReplaceAll(c.line, "<bin>", filepath.Join(home, ".local", "bin"))
			if err != nil || !strings.HasSuffix(string(b), line+"\n") {
				t.Fatalf("%s: %q %v, want it to end with %q", c.want, b, err, line)
			}
		})
	}
	// csh writes nothing and says so.
	home := t.TempDir()
	r, err := Link(bundle(t, t.TempDir(), "A"), envOf(map[string]string{"HOME": home, "SHELL": "/bin/tcsh"}), "")
	if err != nil || len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "Add ") {
		t.Fatalf("tcsh: %+v %v", r, err)
	}
	if got := tree(t, home); len(got) != 4 { // ., .local, .local/bin, the link
		t.Fatalf("tcsh changed %v", got)
	}
}

// An app run from the disk image or a translocated copy links nothing.
func TestLinkRefusesAnAppNotInstalled(t *testing.T) {
	home := t.TempDir()
	for _, cli := range []string{
		"/Volumes/Agentry Desktop/Agentry Desktop.app/Contents/Resources/bin/agentry",
		"/private/var/folders/x/T/AppTranslocation/ABC/d/Agentry Desktop.app/Contents/Resources/bin/agentry",
	} {
		if s := Status(cli, envOf(map[string]string{"HOME": home}), nil); s.Installable {
			t.Fatalf("%s is installable", cli)
		}
	}
	root := filepath.Join(t.TempDir(), "AppTranslocation", "X")
	cli := bundle(t, root, "Agentry Desktop")
	if _, err := Link(cli, envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh"}), ""); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
	if got := tree(t, home); len(got) != 1 {
		t.Fatalf("a refused link changed %v", got)
	}
}

// A startup file that is a link (a dotfiles checkout) is appended through,
// as the install script's >> does, and the link stays a link.
func TestLinkAppendsThroughALinkedStartupFile(t *testing.T) {
	home := t.TempDir()
	dotfiles := filepath.Join(t.TempDir(), "zshrc")
	if err := os.WriteFile(dotfiles, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	if _, err := Link(bundle(t, t.TempDir(), "A"), envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh"}), ""); err != nil {
		t.Fatal(err)
	}
	if dest, err := os.Readlink(filepath.Join(home, ".zshrc")); err != nil || dest != dotfiles {
		t.Fatalf(".zshrc is no longer the link: %q %v", dest, err)
	}
	if b, _ := os.ReadFile(dotfiles); !strings.HasPrefix(string(b), "# mine\n\n# Added by the agentry CLI installer: ") {
		t.Fatalf("the linked file: %q", b)
	}
}

// A folder whose name would break the shell line is refused before
// anything is written.
func TestLinkRefusesAFolderTheShellLineCannotHold(t *testing.T) {
	home := t.TempDir()
	odd := filepath.Join(home, `bin"$(x)`)
	_, err := Link(bundle(t, t.TempDir(), "A"), envOf(map[string]string{"HOME": home, "SHELL": "/bin/zsh", "AGENTRY_INSTALL_DIR": odd}), "")
	if err == nil || !strings.Contains(err.Error(), "cannot be put on PATH") {
		t.Fatalf("err = %v", err)
	}
	if got := tree(t, home); len(got) != 1 {
		t.Fatalf("a refused folder changed %v", got)
	}
}
