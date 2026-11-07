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
	"path/filepath"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
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
	req := desktop.BindRequest{Run: p.params.RunID, Ref: p.plan.Ref, Digest: p.plan.Digest}
	for _, s := range d.Servers {
		req.Servers = append(req.Servers, desktop.BindServer{ID: s.ID, Tools: s.ToolNames()})
	}
	if c, ok := d.Computer(); ok {
		req.ComputerUse, req.ComputerTools = d.ComputerUse, c.ToolNames()
		if p.params.OutDir != "" {
			req.Reports = filepath.Join(p.params.OutDir, filepath.FromSlash(desktop.ReportsFolder))
		}
	}
	link, err := bind(ctx, req)
	if err != nil {
		var r *desktop.Refusal
		switch {
		case errors.As(err, &r):
			return engine.DesktopBindRefusal(d, r)
		case errors.Is(err, desktop.ErrNoService):
			return engine.NoDesktopRefusal(d)
		}
		return engine.Classify(engine.ExitCLI, fmt.Errorf("bind the run to Agentry Desktop: %w", err))
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

// RefuseHostedDesktop refuses a run In Agentry of a release that declares
// the person's computer, which only a run on that computer reaches.
func RefuseHostedDesktop(m *manifest.Manifest) error {
	if m.Capabilities.Desktop != nil {
		return fmt.Errorf("this QuickAgent uses an application on your computer through Agentry Desktop, which only a run on this computer does: a run In Agentry is not offered for it")
	}
	return nil
}
