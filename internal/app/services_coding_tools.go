package app

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/codingtools"
)

// CodingTools is Settings → Updates and installs → Coding tools' list of
// the coding tools on this computer and the Agentry plugin in each, read
// from the file system: nothing is executed to draw it.
func (s *SettingsAPI) CodingTools() []codingtools.View { return codingtools.Scan() }

// CodingToolInstall is what an install did: each command with its output,
// the tool as it now reads, and the failure, if one stopped it.
type CodingToolInstall struct {
	Steps []codingtools.StepResult `json:"steps"`
	Tool  codingtools.View         `json:"tool"`
	Error string                   `json:"error"`
}

// codingToolInstalls lets one install run at a time: two tools' commands
// writing the same plugin directories at once (the Claude app's and Claude
// Code's are one) would race.
var codingToolInstalls sync.Mutex

// InstallCodingToolPlugin installs the Agentry plugin into the tool id
// with that tool's own commands. The page names the tool and nothing else:
// the commands, the executable and the package come from the catalog and
// the file system, and a native dialog that lists every command is the
// consent. A No is "not confirmed"; a command that fails is reported in
// the answer with its output, not as an error.
func (s *SettingsAPI) InstallCodingToolPlugin(id string) (*CodingToolInstall, error) {
	plan, err := codingtools.PlanInstall(id)
	if err != nil {
		return nil, err
	}
	if !codingToolInstalls.TryLock() {
		return nil, errors.New("another plugin install is running; wait for it to finish")
	}
	defer codingToolInstalls.Unlock()
	if err := s.app.askConsent("Install the Agentry plugin?", installConsentText(plan)); err != nil {
		return nil, err
	}
	steps, runErr := codingtools.Run(s.app.ctx, plan)
	view, _ := codingtools.ViewOf(id)
	out := &CodingToolInstall{Steps: steps, Tool: view}
	if runErr != nil {
		out.Error = runErr.Error()
	}
	return out, nil
}

// installConsentText is the dialog's message: which tool, from which
// package, and every command exactly as it will run.
func installConsentText(p codingtools.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Agentry Desktop will install the Agentry plugin into %s", p.Tool.Name)
	if p.Via != nil {
		fmt.Fprintf(&b, " with %s's command", p.Via.Name)
	}
	if repo := p.Tool.Repository(); repo != "" {
		fmt.Fprintf(&b, ", from %s", repo)
	}
	b.WriteString(", by running:\n\n")
	for _, c := range p.Commands() {
		b.WriteString(c)
		b.WriteString("\n")
	}
	b.WriteString("\nThe plugin drives the agentry command on this computer, which keeps your sign-in and enforces each QuickAgent's contract.")
	return b.String()
}

// OpenCodingToolLink opens the page or deep link that adds the plugin in a
// tool whose plugins are added in its own window. The page names the tool;
// the link is the catalog's, so a tool's own scheme (cursor:, kiro:) can be
// opened without the page being able to open any other.
func (s *SettingsAPI) OpenCodingToolLink(id string) error {
	t, ok := codingtools.ByID(id)
	if !ok {
		return fmt.Errorf("unknown coding tool %q", id)
	}
	if t.ManualURL == "" {
		return fmt.Errorf("%s has no link to open", t.Name)
	}
	browserOpenURL(s.app.ctx, t.ManualURL)
	return nil
}
