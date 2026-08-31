// BuildAPI: the Build surface's bindings — author a BriefAgent as a form,
// save it as a real workspace, validate it with the registry's own
// scanners, and publish it through the full pipeline. Publish progress
// streams as `build:publish-log` events.
package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

const eventBuildLog = "build:publish-log"

// BuildAPI drives the Build page.
type BuildAPI struct {
	app *App
	mu  sync.Mutex
}

// NewModel returns a fresh form model. The namespace defaults to the
// signed-in handle on the given registry when there is one.
func (b *BuildAPI) NewModel(reg string) (*build.Model, error) {
	ns := "example"
	if token := settings.TokenFor(reg); token != "" {
		if u, err := whoami(b.app, reg, token); err == nil {
			ns = u.Handle
		}
	}
	return build.Default(ns), nil
}

// Load reads an existing workspace back into the form.
func (b *BuildAPI) Load(dir string) (*build.Model, error) {
	if dir == "" {
		return nil, fmt.Errorf("pick a folder")
	}
	return build.Load(dir)
}

// Exists reports whether dir already holds an agent.yaml (the page asks
// before loading over unsaved form state).
func (b *BuildAPI) Exists(dir string) (bool, error) {
	if dir == "" {
		return false, nil
	}
	_, err := os.Stat(dir + "/agent.yaml")
	return err == nil, nil
}

// Save writes the workspace (agent.yaml + entrypoint always; README,
// LICENSE, .gitignore only when absent).
func (b *BuildAPI) Save(dir string, m build.Model) error {
	if dir == "" {
		return fmt.Errorf("pick a folder to save into")
	}
	return build.Save(dir, &m)
}

// Validate runs the same offline checks the Validate page runs (manifest
// V-rules + S1/S3/S4), on the saved workspace.
func (b *BuildAPI) Validate(dir string) (*validate.Result, error) {
	return validate.Run(dir)
}

// Publish uploads the workspace to the registry (creating the agent record
// if needed) and follows the pipeline to a verdict. visibility is
// "private" or "public".
func (b *BuildAPI) Publish(reg, dir, visibility string) (*build.PublishResult, error) {
	if !b.mu.TryLock() {
		return nil, fmt.Errorf("a publish is already in progress")
	}
	defer b.mu.Unlock()
	if visibility != "private" && visibility != "public" {
		return nil, fmt.Errorf("visibility must be private or public")
	}
	logw := (&RunAPI{app: b.app}).eventWriter(eventBuildLog)
	defer logw.Flush()
	return build.Publish(b.app.ctx, reg, settings.TokenFor(reg), dir, visibility, logw)
}
