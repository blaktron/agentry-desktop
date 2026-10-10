// Package cliinstall installs the agentry command on macOS and Linux when the
// person asks for it, from the Scheduled page's Options menu
// (agentry-desktop#788; D-076).
//
// It runs the site's own install script, /install.sh (agentry-web
// scripts/install/install.sh; plan agentry-notes/plans/cli-on-path.md §4),
// rather than a copy of it: the script is the one place that knows which CLI
// the site serves, holds the download to the SHA-256 the site lists, puts it
// in ${XDG_BIN_HOME:-~/.local/bin} and adds the marked PATH line to the
// shell's rc file once (cli-on-path.md D1 rejects a copy that would drift).
// The site is the one this build's update channel names, so a dev build
// installs the dev CLI and a production build the production one, as the
// one-liner on that site's /download page does.
//
// The script runs as the person, never as root, with AGENTRY_NO_NODE=1: an
// app's PATH does not see a Node from Homebrew or a version manager, so the
// script would install a second, pinned Node in front of the person's own.
package cliinstall

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Name is the command's file name.
const Name = "agentry"

// ErrNoChannel: a build of the open core has no site to install from.
var ErrNoChannel = errors.New("this build names no site to install the agentry command from")

// maxScript caps what is read as the script. The site's is about 12 KB.
const maxScript = 1 << 20

// Timeout bounds a whole install: the script downloads a CLI of some tens
// of megabytes.
const Timeout = 10 * time.Minute

// Offered reports whether the app installs the command on goos, as the
// account euid: macOS and Linux, never as root, where sudo or pkexec could
// hand it another account's home. Windows carries the command in its
// installer (D-061).
func Offered(goos string, euid int) bool {
	return (goos == "darwin" || goos == "linux") && euid != 0
}

// ScriptURL is the install script of the site whose downloads manifest is
// channel (https://site/downloads/manifest.json → https://site/install.sh).
func ScriptURL(channel string) (string, error) {
	if strings.TrimSpace(channel) == "" {
		return "", ErrNoChannel
	}
	u, err := url.Parse(channel)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("the update channel %q is not a URL", channel)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("the update channel %q is not https, so no script is fetched from it", channel)
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment, u.User = "/install.sh", "", "", "", nil
	return u.String(), nil
}

// Dir is where the script puts the command: $AGENTRY_INSTALL_DIR, else
// $XDG_BIN_HOME, else ~/.local/bin; "" when none of them is an absolute
// path.
func Dir(getenv func(string) string) string {
	for _, k := range []string{"AGENTRY_INSTALL_DIR", "XDG_BIN_HOME"} {
		if d := strings.TrimSpace(getenv(k)); d != "" {
			if filepath.IsAbs(d) {
				return filepath.Clean(d)
			}
			return ""
		}
	}
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

// Path is the command the script installs, "" when Dir is.
func Path(getenv func(string) string) string {
	d := Dir(getenv)
	if d == "" {
		return ""
	}
	return filepath.Join(d, Name)
}

// Fetch reads the script at scriptURL. Redirects stay on its origin, so the
// script is always the site's own, and the body must be a POSIX sh script.
func Fetch(ctx context.Context, client *http.Client, scriptURL string) ([]byte, error) {
	base, err := url.Parse(scriptURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("the install script %q is not an https URL", scriptURL)
	}
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return fmt.Errorf("the site redirected the install script to %s, off its own origin", req.URL.Redacted())
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", base.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", base.Redacted(), resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScript+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", base.Redacted(), err)
	}
	if len(body) > maxScript {
		return nil, fmt.Errorf("%s is larger than an install script", base.Redacted())
	}
	if !bytes.HasPrefix(body, []byte("#!/bin/sh")) {
		return nil, fmt.Errorf("%s is not the agentry install script", base.Redacted())
	}
	return body, nil
}

// Result is what an install did.
type Result struct {
	// Path is the command the script installed (its AGENTRY_BIN line).
	Path string
	// Log is everything the script printed, for the person to read.
	Log string
	// Notes are the script's lines a person should see without opening the
	// log: the PATH line it added, a command that comes first on PATH, and
	// what to do in a terminal that is already open.
	Notes []string
}

// Runner runs a script with sh and returns its combined output.
type Runner func(ctx context.Context, script string, env []string) ([]byte, error)

// RunSh runs script with /bin/sh.
func RunSh(ctx context.Context, script string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", script)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(nil)
	return cmd.CombinedOutput()
}

// Run writes script to a private temporary directory and runs it as this
// account with env, plus AGENTRY_NO_NODE=1 and a SHELL when env has none
// (the script picks the rc file by the person's shell).
func Run(ctx context.Context, script []byte, env []string, goos string, run Runner) (Result, error) {
	dir, err := os.MkdirTemp("", "agentry-install-")
	if err != nil {
		return Result{}, fmt.Errorf("could not make a directory for the install script: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	file := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(file, script, 0o600); err != nil {
		return Result{}, fmt.Errorf("could not write the install script: %w", err)
	}
	out, err := run(ctx, file, Env(env, goos))
	r := Result{Log: strings.TrimSpace(string(out))}
	if err != nil {
		if ctx.Err() != nil {
			return r, fmt.Errorf("the install did not finish in time: %w", ctx.Err())
		}
		if why := Reason(out); why != "" {
			return r, errors.New(why)
		}
		return r, fmt.Errorf("the install script failed: %w", err)
	}
	r.Path, r.Notes = parse(out)
	if r.Path == "" {
		return r, errors.New("the install script finished without saying where it put the agentry command")
	}
	return r, nil
}

// Env is env with AGENTRY_NO_NODE=1, and SHELL set to the platform's
// default login shell when env has none.
func Env(env []string, goos string) []string {
	out := make([]string, 0, len(env)+2)
	shell := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "AGENTRY_NO_NODE=") {
			continue
		}
		if v, ok := strings.CutPrefix(kv, "SHELL="); ok && v != "" {
			shell = true
		}
		out = append(out, kv)
	}
	out = append(out, "AGENTRY_NO_NODE=1")
	if !shell {
		if goos == "darwin" {
			out = append(out, "SHELL=/bin/zsh")
		} else {
			out = append(out, "SHELL=/bin/bash")
		}
	}
	return out
}

// Reason is the script's own reason for stopping: its last
// "agentry install: …" line, without the prefix.
func Reason(out []byte) string {
	why := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if r, ok := strings.CutPrefix(sc.Text(), "agentry install: "); ok {
			why = strings.TrimSpace(r)
		}
	}
	return why
}

// parse reads the installed path and the lines worth showing from the
// script's output.
func parse(out []byte) (string, []string) {
	path := ""
	var notes []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "AGENTRY_BIN="):
			path = strings.TrimPrefix(line, "AGENTRY_BIN=")
		case strings.HasPrefix(line, "Note: this terminal"):
			// "This terminal" is the app's own environment, not one the
			// person has open.
		case strings.HasPrefix(line, "Added "), strings.HasPrefix(line, "Note: "), strings.HasPrefix(line, "Open a new terminal"), strings.HasPrefix(line, "Add "):
			notes = append(notes, line)
		}
	}
	return path, notes
}
