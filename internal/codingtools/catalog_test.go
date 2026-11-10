package codingtools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogEntriesAreWhole(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range catalog {
		if tool.ID == "" || tool.Name == "" || tool.Kind == "" {
			t.Errorf("%+v: id, name and kind are required", tool)
		}
		if seen[tool.ID] {
			t.Errorf("%s: duplicate id", tool.ID)
		}
		seen[tool.ID] = true
		if !strings.HasPrefix(tool.Site, "https://") {
			t.Errorf("%s: the vendor page must be https, got %q", tool.ID, tool.Site)
		}
		if len(tool.Commands) == 0 && len(tool.Locations) == 0 {
			t.Errorf("%s: nothing to look for", tool.ID)
		}
		installs := len(tool.Steps) > 0
		if installs && tool.Package == "" {
			t.Errorf("%s: an install names the package it installs", tool.ID)
		}
		if !installs && tool.Manual == "" && len(tool.Uses) == 0 && tool.Note == "" {
			t.Errorf("%s: a tool with no install says what to do instead", tool.ID)
		}
		if tool.ManualURL != "" && !strings.Contains(tool.ManualURL, "://") {
			t.Errorf("%s: manual link %q is not a URL", tool.ID, tool.ManualURL)
		}
		for _, argv := range tool.Steps {
			for _, a := range argv {
				if strings.ContainsAny(a, "|&;<>$`\n") {
					t.Errorf("%s: argument %q reads like shell", tool.ID, a)
				}
			}
		}
		for _, id := range tool.Uses {
			if _, ok := ByID(id); !ok {
				t.Errorf("%s: uses unknown tool %q", tool.ID, id)
			}
		}
	}
	for _, tool := range catalog {
		if tool.Installer == "" {
			continue
		}
		owner, ok := ByID(tool.Installer)
		switch {
		case !ok:
			t.Errorf("%s: unknown installer %q", tool.ID, tool.Installer)
		case owner.Kind != KindCLI || len(owner.Commands) == 0:
			t.Errorf("%s: installer %s is not a command-line tool", tool.ID, owner.ID)
		case len(tool.Steps) == 0:
			t.Errorf("%s: an installer with no steps", tool.ID)
		}
	}
}

// Every home-relative location of every tool, on every platform, is found
// when it exists: the templates expand on each system the app ships for.
func TestCatalogLocationsResolvePerPlatform(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		for _, tool := range catalog {
			for _, tpl := range tool.Locations.For(platform) {
				env := map[string]string{}
				local := t.TempDir()
				for _, v := range []string{"LOCALAPPDATA", "APPDATA", "XDG_CONFIG_HOME"} {
					env[v] = filepath.Join(local, v)
				}
				home := isolate(t, platform, nil, env, tool)
				p, ok := expand(tpl)
				if !ok {
					t.Fatalf("%s on %s: %q does not expand", tool.ID, platform, tpl)
				}
				if !strings.HasPrefix(p, home) && !strings.HasPrefix(p, local) {
					continue // a system directory the test cannot write
				}
				if strings.HasSuffix(p, string(filepath.Separator)+"*") {
					p = filepath.Join(strings.TrimSuffix(p, string(filepath.Separator)+"*"), "1.2.3")
				}
				touch(t, p)
				f, found := Find(tool)
				if !found || f.Source != SourceUsual {
					t.Errorf("%s on %s: %q was not found (%+v)", tool.ID, platform, tpl, f)
				}
			}
		}
	}
}

// The commands each tool installs with, as they will run: the README's
// line with the flags that keep it from asking.
func TestCatalogInstallCommands(t *testing.T) {
	want := map[string][]string{
		"claude-code": {"claude plugin marketplace add blaktron/agentry-plugin-claude-code", "claude plugin install agentry@agentry --scope user"},
		"codex":       {"codex plugin marketplace add blaktron/agentry-plugin-codex", "codex plugin add agentry@agentry"},
		"opencode":    {"opencode plugin add github:blaktron/agentry-plugin-opencode"},
		"pi":          {"pi install git:github.com/blaktron/agentry-plugin-pi"},
		"qwen":        {"qwen extensions install https://github.com/blaktron/agentry-plugin-claude-code:agentry --consent"},
		"gemini-cli":  {"gemini extensions install https://github.com/blaktron/agentry-plugin-gemini-cli --consent --skip-settings"},
		"copilot":     {"copilot plugin install blaktron/agentry-plugin-agent-plugins"},
		"agy":         {"agy plugin install https://github.com/blaktron/agentry-plugin-antigravity"},
	}
	for id, cmds := range want {
		tool, ok := ByID(id)
		if !ok {
			t.Fatalf("%s is not in the catalog", id)
		}
		isolate(t, "linux", map[string]string{tool.Commands[0]: tool.Commands[0]}, nil)
		p, err := PlanInstall(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := p.Commands(); strings.Join(got, "\n") != strings.Join(cmds, "\n") {
			t.Errorf("%s: got %q, want %q", id, got, cmds)
		}
	}
}

// An app installs with its command-line tool's command, and the Claude
// app with the build of Claude Code its Code tab keeps when the claude
// command is not on this computer.
func TestAppsInstallThroughTheirCommandLineTool(t *testing.T) {
	home := isolate(t, "linux", map[string]string{"claude-desktop": "/usr/bin/claude-desktop"}, nil)
	v, _ := ViewOf("claude-app")
	if !v.Found || v.Blocked == "" || !strings.Contains(v.Blocked, "Claude Code") {
		t.Fatalf("no claude anywhere: %+v", v)
	}
	touch(t, filepath.Join(home, ".claude", "remote", "ccd-cli", "2.1.321-c6c6b56"))
	v, _ = ViewOf("claude-app")
	if v.Blocked != "" || len(v.Commands) != 2 || !strings.HasPrefix(v.Commands[0], filepath.Join(home, ".claude", "remote", "ccd-cli", "2.1.321-c6c6b56")+" plugin") {
		t.Fatalf("the Code tab's build: %+v", v)
	}

	isolate(t, "darwin", map[string]string{"codex": "/opt/homebrew/bin/codex"}, nil)
	v, _ = ViewOf("codex-app")
	if v.Via != "Codex" || len(v.Commands) != 2 || !strings.HasPrefix(v.Commands[0], "/opt/homebrew/bin/codex ") {
		t.Fatalf("the codex command: %+v", v)
	}
}

// A tool that refuses a second install is not offered one, and Go
// refuses it whatever the page asks.
func TestInstalledPluginIsNotInstalledAgainWhereThatFails(t *testing.T) {
	home := isolate(t, "linux", map[string]string{"qwen": "/usr/bin/qwen", "claude": "/usr/bin/claude"}, nil)
	touch(t, filepath.Join(home, ".qwen", "extensions", "agentry", "qwen-extension.json"))
	v, _ := ViewOf("qwen")
	if v.Plugin != PluginInstalled || len(v.Commands) != 0 {
		t.Fatalf("qwen: %+v", v)
	}
	if _, err := PlanInstall("qwen"); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("qwen install again: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(home, ".claude", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), []byte(`{"plugins":{"agentry@agentry":[{}]}}`), 0o600)
	v, _ = ViewOf("claude-code")
	if v.Plugin != PluginInstalled || len(v.Commands) != 2 {
		t.Fatalf("claude code repeats safely: %+v", v)
	}
	runCommand = func(context.Context, string, []string) ([]byte, int, error) { return nil, 0, nil }
	p, err := PlanInstall("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestScanRunsNothing(t *testing.T) {
	isolate(t, "linux", map[string]string{"claude": "/usr/bin/claude", "codex": "/usr/bin/codex", "qwen": "/usr/bin/qwen"}, nil)
	views := Scan()
	if len(views) != len(catalog) {
		t.Fatalf("got %d views for %d tools", len(views), len(catalog))
	}
}
