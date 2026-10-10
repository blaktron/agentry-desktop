package cliinstall

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestOffered(t *testing.T) {
	for _, c := range []struct {
		goos string
		euid int
		want bool
	}{
		{"darwin", 501, true},
		{"linux", 1000, true},
		{"linux", 0, false},
		{"darwin", 0, false},
		{"windows", 1000, false},
	} {
		if got := Offered(c.goos, c.euid); got != c.want {
			t.Errorf("Offered(%q, %d) = %v, want %v", c.goos, c.euid, got, c.want)
		}
	}
}

func TestScriptURL(t *testing.T) {
	got, err := ScriptURL("https://dev.secureagentry.net/downloads/manifest.json?x=1#y")
	if err != nil || got != "https://dev.secureagentry.net/install.sh" {
		t.Fatalf("ScriptURL = %q, %v", got, err)
	}
	if _, err := ScriptURL(""); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("no channel: %v", err)
	}
	if _, err := ScriptURL("http://dev.secureagentry.net/downloads/manifest.json"); err == nil {
		t.Fatal("an http channel gave a script URL")
	}
	if got, err := ScriptURL("https://user:pw@example.test/downloads/manifest.json"); err != nil || strings.Contains(got, "pw") {
		t.Fatalf("userinfo kept: %q %v", got, err)
	}
}

func TestDirAndPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HOME": "/home/ada"}, "/home/ada/.local/bin"},
		{map[string]string{"HOME": "/home/ada", "XDG_BIN_HOME": "/home/ada/bin"}, "/home/ada/bin"},
		{map[string]string{"HOME": "/home/ada", "XDG_BIN_HOME": "/x", "AGENTRY_INSTALL_DIR": "/opt/a"}, "/opt/a"},
		{map[string]string{"HOME": "/home/ada", "XDG_BIN_HOME": "rel/bin"}, ""},
		{map[string]string{}, ""},
		{map[string]string{"HOME": "relative"}, ""},
	} {
		if got := Dir(env(c.env)); got != c.want {
			t.Errorf("Dir(%v) = %q, want %q", c.env, got, c.want)
		}
	}
	if got := Path(env(map[string]string{"HOME": "/home/ada"})); got != "/home/ada/.local/bin/agentry" {
		t.Errorf("Path = %q", got)
	}
	if got := Path(env(map[string]string{})); got != "" {
		t.Errorf("Path with no home = %q", got)
	}
}

func TestFetch(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\necho elsewhere\n"))
	}))
	defer other.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/install.sh":
			_, _ = w.Write([]byte("#!/bin/sh\nmain() { :; }\nmain \"$@\"\n"))
		case "/moved":
			http.Redirect(w, r, "/install.sh", http.StatusFound)
		case "/away":
			http.Redirect(w, r, other.URL+"/install.sh", http.StatusFound)
		case "/html":
			_, _ = w.Write([]byte("<!doctype html><p>not found</p>"))
		case "/big":
			_, _ = w.Write([]byte("#!/bin/sh\n" + strings.Repeat("#", maxScript)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := srv.Client()
	ctx := context.Background()

	if b, err := Fetch(ctx, c, srv.URL+"/install.sh"); err != nil || !strings.HasPrefix(string(b), "#!/bin/sh") {
		t.Fatalf("Fetch = %q, %v", b, err)
	}
	if _, err := Fetch(ctx, c, srv.URL+"/moved"); err != nil {
		t.Fatalf("a redirect on the same origin: %v", err)
	}
	for path, want := range map[string]string{
		"/away":    "off its own origin",
		"/html":    "is not the agentry install script",
		"/big":     "larger than an install script",
		"/missing": "404",
	} {
		if _, err := Fetch(ctx, c, srv.URL+path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Fetch %s: %v, want %q", path, err, want)
		}
	}
	if _, err := Fetch(ctx, c, "http://example.test/install.sh"); err == nil {
		t.Error("an http script URL was fetched")
	}
}

func TestEnv(t *testing.T) {
	got := Env([]string{"HOME=/h", "AGENTRY_NO_NODE=0", "SHELL=/usr/bin/fish"}, "darwin")
	if !slices.Contains(got, "AGENTRY_NO_NODE=1") || slices.Contains(got, "AGENTRY_NO_NODE=0") {
		t.Errorf("AGENTRY_NO_NODE not forced: %v", got)
	}
	if !slices.Contains(got, "SHELL=/usr/bin/fish") || slices.Contains(got, "SHELL=/bin/zsh") {
		t.Errorf("the person's shell was not kept: %v", got)
	}
	if got := Env([]string{"HOME=/h"}, "darwin"); !slices.Contains(got, "SHELL=/bin/zsh") {
		t.Errorf("macOS with no SHELL: %v", got)
	}
	if got := Env([]string{"HOME=/h", "SHELL="}, "linux"); !slices.Contains(got, "SHELL=/bin/bash") {
		t.Errorf("Linux with an empty SHELL: %v", got)
	}
}

// fakeOutput is what the site's script prints on a first install that added
// the PATH line.
const fakeOutput = `Downloading agentry 0.6.93 for darwin/arm64 from https://secureagentry.net
Installed agentry 0.6.93: /Users/ada/.local/bin/agentry
Added /Users/ada/.local/bin to PATH in /Users/ada/.zshrc
Note: this terminal finds /usr/local/bin/agentry; new terminals find /Users/ada/.local/bin/agentry first.
Node.js 20 or later was not found, and AGENTRY_NO_NODE=1 skips it (the agentry MCP server needs it)
Docker: running
python3: found

Open a new terminal, or run  . "/Users/ada/.zshrc"  to use agentry here.
AGENTRY_BIN=/Users/ada/.local/bin/agentry
`

func TestRun(t *testing.T) {
	ctx := context.Background()
	var gotScript string
	var gotEnv []string
	ok := func(_ context.Context, script string, env []string) ([]byte, error) {
		b, err := os.ReadFile(script)
		gotScript, gotEnv = string(b), env
		return []byte(fakeOutput), err
	}
	r, err := Run(ctx, []byte("#!/bin/sh\necho hi\n"), []string{"HOME=/Users/ada"}, "darwin", ok)
	if err != nil {
		t.Fatal(err)
	}
	if gotScript != "#!/bin/sh\necho hi\n" || !slices.Contains(gotEnv, "AGENTRY_NO_NODE=1") {
		t.Errorf("ran %q with %v", gotScript, gotEnv)
	}
	if r.Path != "/Users/ada/.local/bin/agentry" {
		t.Errorf("Path = %q", r.Path)
	}
	wantNotes := []string{
		"Added /Users/ada/.local/bin to PATH in /Users/ada/.zshrc",
		`Open a new terminal, or run  . "/Users/ada/.zshrc"  to use agentry here.`,
	}
	if !slices.Equal(r.Notes, wantNotes) {
		t.Errorf("Notes = %q, want %q", r.Notes, wantNotes)
	}
	if !strings.Contains(r.Log, "Docker: running") {
		t.Errorf("Log = %q", r.Log)
	}

	failed := func(context.Context, string, []string) ([]byte, error) {
		return []byte("Downloading agentry\nagentry install: the download's SHA-256 is aa, but https://x lists bb; nothing was installed\n"), errors.New("exit status 1")
	}
	if _, err := Run(ctx, []byte("#!/bin/sh\n"), nil, "linux", failed); err == nil || err.Error() != "the download's SHA-256 is aa, but https://x lists bb; nothing was installed" {
		t.Errorf("a refusal: %v", err)
	}
	silent := func(context.Context, string, []string) ([]byte, error) { return []byte("done\n"), nil }
	if _, err := Run(ctx, []byte("#!/bin/sh\n"), nil, "linux", silent); err == nil || !strings.Contains(err.Error(), "without saying where") {
		t.Errorf("no AGENTRY_BIN: %v", err)
	}
}

// TestRunShRunsTheScript runs a stand-in for the site's script with the real
// /bin/sh: it gets the forced environment, and the temporary copy is gone
// afterwards.
func TestRunShRunsTheScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /bin/sh")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nmain() {\n  mkdir -p \"$HOME/.local/bin\"\n  printf '#!/bin/sh\\necho agentry version 0.6.93\\n' > \"$HOME/.local/bin/agentry\"\n  echo \"node=$AGENTRY_NO_NODE shell=$SHELL here=$0\"\n  echo \"AGENTRY_BIN=$HOME/.local/bin/agentry\"\n}\nmain \"$@\"\n"
	r, err := Run(context.Background(), []byte(script), []string{"HOME=" + dir, "PATH=/usr/bin:/bin"}, "linux", RunSh)
	if err != nil {
		t.Fatalf("%v\n%s", err, r.Log)
	}
	if r.Path != filepath.Join(dir, ".local", "bin", "agentry") {
		t.Errorf("Path = %q", r.Path)
	}
	if !strings.Contains(r.Log, "node=1 shell=/bin/bash") {
		t.Errorf("Log = %q", r.Log)
	}
	ran := strings.TrimPrefix(strings.Fields(r.Log)[2], "here=")
	if _, err := os.Stat(ran); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the script's temporary copy %s is still there: %v", ran, err)
	}
}
