package codingtools

// View is one tool as Settings → Coding tools shows it.
type View struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	Site string `json:"site"`
	// Found, Path and Source are where the tool is on this computer.
	Found  bool   `json:"found"`
	Path   string `json:"path"`
	Source Source `json:"source"`
	// Plugin is the Agentry plugin's state in the tool.
	Plugin PluginState `json:"plugin"`
	// Repository is the plugin package's public repository; "" for a tool
	// that loads another tool's plugins.
	Repository string `json:"repository"`
	// Commands are what Install would run, when it can run here; Blocked
	// says why it cannot when the tool installs with a command.
	Commands []string `json:"commands"`
	Blocked  string   `json:"blocked"`
	// Via names the tool whose command the install runs, when it is not
	// this one (an app whose plugins are its command-line tool's).
	Via string `json:"via"`
	// Manual and ManualURL are the steps in the tool's own window.
	Manual    string `json:"manual"`
	ManualURL string `json:"manualUrl"`
	// Uses names the tools whose plugins this one loads.
	Uses []string `json:"uses"`
	Note string   `json:"note"`
}

// Scan reads every catalog tool's state from the file system, in the
// catalog's order. It runs nothing.
func Scan() []View {
	out := make([]View, 0, len(catalog))
	for _, t := range catalog {
		out = append(out, viewOf(t))
	}
	return out
}

// ViewOf is one tool's state; false for an unknown id.
func ViewOf(id string) (View, bool) {
	t, ok := ByID(id)
	if !ok {
		return View{}, false
	}
	return viewOf(t), true
}

func viewOf(t Tool) View {
	v := View{
		ID: t.ID, Name: t.Name, Kind: t.Kind, Site: t.Site,
		Repository: t.Repository(), Manual: t.Manual, ManualURL: t.ManualURL, Note: t.Note,
		Plugin: PluginUnknown,
	}
	if f, ok := Find(t); ok {
		v.Found, v.Path, v.Source = true, f.Path, f.Source
	}
	for _, id := range t.Uses {
		if u, ok := ByID(id); ok {
			v.Uses = append(v.Uses, u.Name)
		}
	}
	if t.Package != "" || len(t.Steps) > 0 {
		v.Plugin = Plugin(t)
	}
	if len(t.Steps) > 0 && (t.Repeatable || v.Plugin != PluginInstalled) {
		exe, via, err := Installer(t)
		if via != nil {
			v.Via = via.Name
		}
		if err != nil {
			v.Blocked = err.Error()
		} else {
			for _, argv := range t.Steps {
				v.Commands = append(v.Commands, CommandLine(exe, argv))
			}
		}
	}
	return v
}

// ByID is the catalog entry for id.
func ByID(id string) (Tool, bool) {
	for _, t := range catalog {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}
