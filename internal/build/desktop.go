package build

import (
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/library"
)

// DesktopApp is one preset of Agentry Desktop's library as the form offers
// it: the id a release names, the application, the systems it is offered
// on, and the tools the library classes.
type DesktopApp struct {
	ID          string           `json:"id"`
	Application string           `json:"application"`
	Platforms   []string         `json:"platforms"`
	Tools       []DesktopAppTool `json:"tools"`
}

// DesktopAppTool is one tool of a preset's server: read, act or exec (code
// the application runs, which the person is asked about before each call),
// and whether it reaches the preset's hosts.
type DesktopAppTool struct {
	Name   string `json:"name"`
	Class  string `json:"class"`
	Egress bool   `json:"egress,omitempty"`
}

// DesktopApps are the presets of the library in use, in its order. A
// withheld tool is never offered.
func DesktopApps() []DesktopApp {
	all := library.Current().All()
	out := make([]DesktopApp, 0, len(all))
	for _, e := range all {
		app := DesktopApp{ID: e.ID, Application: e.Application, Platforms: append([]string{}, e.Platforms...), Tools: []DesktopAppTool{}}
		for _, t := range e.Tools {
			app.Tools = append(app.Tools, DesktopAppTool{Name: t.Name, Class: string(t.Class), Egress: t.Egress})
		}
		out = append(out, app)
	}
	return out
}

// DesktopConsent is what the install consent will ask of the model's
// desktop line, in its words: one line an application, and one for its
// computer use. None for no line.
func DesktopConsent(m *Model) []string {
	if m.Desktop.empty() {
		return nil
	}
	d := m.Desktop.manifest()
	var out []string
	for _, s := range d.Servers {
		out = append(out, "Can use "+manifest.DescribeDesktopServer(s)+".")
	}
	if words := manifest.DescribeComputerUse(d); words != "" {
		out = append(out, "C"+words[1:]+".")
	}
	return out
}
