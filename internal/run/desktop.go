package run

// The person's computer. A release that declares capabilities.desktop uses
// applications on this computer through Agentry Desktop's link service: the
// run binds to it last before its sandbox starts, in this process, naming
// its release and each application server with the tools the release
// gives it, and the engine carries the run's calls to it and unbinds at
// teardown. A run with no link service to bind to, or one the link service
// refuses, is refused before the sandbox with exit 11 and the step to take,
// as `agentry run` refuses it.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// DesktopBinder binds a run to the link service and hands back the binding
// the engine carries the run's calls over. A refusal is returned as a
// *desktop.Refusal; desktop.ErrNoService says there is no link service.
type DesktopBinder func(ctx context.Context, req desktop.BindRequest) (*engine.DesktopLink, error)

// bindDesktop binds the prepared run when its plan declares the person's
// computer, and gives the engine the binding; nothing for a plan that does
// not.
func (p *prepared) bindDesktop(ctx context.Context, bind DesktopBinder) error {
	d := p.plan.Capabilities.Desktop
	if d == nil {
		return nil
	}
	if bind == nil {
		return engine.NoDesktopRefusal(d)
	}
	// The request is the one `agentry run` sends: the link protocol's
	// revision, each application's narrowing (none for every tool its
	// server lists), the chosen entry, and computer use's windows.
	link, err := bind(ctx, engine.DesktopBindRequest(d, p.params.RunID, p.plan.Ref, p.plan.Digest, p.params.OutDir))
	if err != nil {
		return desktopBindError(d, err)
	}
	// The engine unbinds at teardown, and the run unbinds again once the
	// engine has returned, for a run that ended before its teardown was
	// set: the second is a no-op.
	var once sync.Once
	unbind := link.Unbind
	link.Unbind = func(ctx context.Context) error {
		var err error
		once.Do(func() {
			if unbind != nil {
				err = unbind(ctx)
			}
		})
		return err
	}
	p.params.Desktop = link
	return nil
}

// desktopBindError is a failed bind as the run's refusal: the link
// service's own, no link service, or the bind's failure.
func desktopBindError(d *spec.DesktopCap, err error) error {
	var r *desktop.Refusal
	switch {
	case errors.As(err, &r):
		return engine.DesktopBindRefusal(d, r)
	case errors.Is(err, desktop.ErrNoService):
		return engine.NoDesktopRefusal(d)
	}
	return engine.Classify(engine.ExitCLI, fmt.Errorf("bind the run to Agentry Desktop: %w", err))
}

// unbindDesktop ends the run's binding, if it has one and the engine did
// not end it.
func (p *prepared) unbindDesktop() {
	if p == nil || p.params.Desktop == nil || p.params.Desktop.Unbind == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = p.params.Desktop.Unbind(ctx)
}

// DesktopIDs are the application servers a release declares, by catalogue
// id; nil for one that declares none.
func DesktopIDs(m *manifest.Manifest) []string {
	d := m.Capabilities.Desktop
	if d == nil {
		return nil
	}
	ids := make([]string, 0, len(d.Servers))
	for _, s := range d.Servers {
		ids = append(ids, s.ID)
	}
	return ids
}

// The uses a release's entry makes of an application.
const (
	UseMCP      = "mcp"
	UseComputer = "computer"
	UseBoth     = "both"
)

// DesktopEntry is one application a release names, as the run setup
// resolves it: its id ("" for an application the person chooses) and what
// the run uses of it, its MCP server, its windows in app scope, or both.
type DesktopEntry struct {
	ID  string `json:"id"`
	Use string `json:"use"`
}

// DesktopEntries are the applications a release's desktop line names, in
// its order, with an application the person chooses last where app-scope
// computer use names no window of its own.
func DesktopEntries(m *manifest.Manifest) []DesktopEntry {
	d := m.Capabilities.Desktop
	if d == nil {
		return nil
	}
	appScope := d.ComputerUse == manifest.ComputerUseApp
	var out []DesktopEntry
	chooserWindows := false
	for _, s := range d.Servers {
		mcp, win := s.UsesServer(), appScope && d.UsesWindows(s)
		if !mcp && !win {
			continue
		}
		use := UseMCP
		switch {
		case mcp && win:
			use = UseBoth
		case win:
			use = UseComputer
		}
		id := s.ID
		if s.Choose {
			id = ""
			chooserWindows = win
		}
		out = append(out, DesktopEntry{ID: id, Use: use})
	}
	if d.ChoosesWindows() && !chooserWindows {
		out = append(out, DesktopEntry{Use: UseComputer})
	}
	return out
}

// RefuseHostedDesktop refuses a run In Agentry of a release that declares
// the person's computer, which only a run on that computer reaches.
func RefuseHostedDesktop(m *manifest.Manifest) error {
	if m.Capabilities.Desktop != nil {
		return fmt.Errorf("this QuickAgent uses an application on your computer through Agentry Desktop, which only a run on this computer does: a run In Agentry is not offered for it")
	}
	return nil
}
