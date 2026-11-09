//go:build !windows

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaude puts a claude on PATH that records its arguments, one call a
// line, and writes the plugin list Claude Code keeps once it installs.
func fakeClaude(t *testing.T, home string) (log string) {
	t.Helper()
	bin := t.TempDir()
	log = filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\n" +
		`echo "$*" >> '` + log + "'\n" +
		`if [ "$2" = install ]; then mkdir -p "$HOME/.claude/plugins" && echo '{"plugins":{"agentry@agentry":[{}]}}' > "$HOME/.claude/plugins/installed_plugins.json"; fi` + "\n" +
		"echo done\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	setUserHome(t, home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return log
}

func TestInstallCodingToolPluginAsksThenRunsTheToolsOwnCommands(t *testing.T) {
	home := t.TempDir()
	log := fakeClaude(t, home)
	var asked string
	answer := false
	s := &SettingsAPI{app: &App{ctx: context.Background(), confirm: func(_ context.Context, _, msg string) (bool, error) {
		asked = msg
		return answer, nil
	}}}

	before := s.CodingTools()
	var cc *struct{ plugin, cmds string }
	for _, v := range before {
		if v.ID == "claude-code" {
			cc = &struct{ plugin, cmds string }{string(v.Plugin), strings.Join(v.Commands, "\n")}
		}
	}
	if cc == nil || cc.plugin != "not installed" || !strings.Contains(cc.cmds, "plugin install agentry@agentry") {
		t.Fatalf("before: %+v", cc)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("reading the list ran the tool")
	}

	if _, err := s.InstallCodingToolPlugin("claude-code"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: got %v", err)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a No ran the tool")
	}
	if !strings.Contains(asked, "plugin marketplace add blaktron/agentry-plugin-claude-code") ||
		!strings.Contains(asked, "https://github.com/blaktron/agentry-plugin-claude-code") {
		t.Fatalf("the dialog does not name the commands and the package:\n%s", asked)
	}

	answer = true
	r, err := s.InstallCodingToolPlugin("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if r.Error != "" || len(r.Steps) != 2 || r.Steps[0].Output != "done" || r.Tool.Plugin != "installed" {
		t.Fatalf("got %+v", r)
	}
	calls, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(calls)); got != "plugin marketplace add blaktron/agentry-plugin-claude-code\nplugin install agentry@agentry --scope user" {
		t.Fatalf("ran:\n%s", got)
	}
}

func TestInstallCodingToolPluginRefusesWhatItCannotInstall(t *testing.T) {
	fakeClaude(t, t.TempDir())
	s := &SettingsAPI{app: &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) {
		t.Fatal("asked about an install that cannot run")
		return false, nil
	}}}
	for _, id := range []string{"no-such-tool", "cursor", "t3code", "codex"} {
		if _, err := s.InstallCodingToolPlugin(id); err == nil {
			t.Errorf("%s: an install was planned", id)
		}
	}
	if err := s.OpenCodingToolLink("codex"); err == nil {
		t.Error("a tool with no link opened one")
	}
}
