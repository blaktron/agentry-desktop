package yourcomputer

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/library"
)

// Probe is what the discovery reads of this computer.
type Probe struct {
	// LookPath finds a command on the PATH.
	LookPath func(string) (string, error)
	// Exists says a path exists.
	Exists func(string) bool
	// Flatpak says a Flatpak application of this id is installed.
	Flatpak func(id string) bool
	// Listening says something answers on the loopback port.
	Listening func(ctx context.Context, port int) bool
}

// Discover adds, once, each preset whose application and server are found
// ready on this computer: the launcher resolves and the application is
// found where the preset says it is installed, or, for a preset that names
// no such place on this system, its listener answers. Nothing it adds is approved. A preset
// whose id the list has already is left alone. screeningOff names the
// presets whose script screening the person had turned off before this
// version, whose application gets the preset without that variable. It
// says which ids it added; on a list it ran on before it does nothing.
func Discover(ctx context.Context, d *Doc, lib library.Library, goos string, p Probe, screeningOff func(id string) bool) []string {
	if d.Discovered {
		return nil
	}
	d.Discovered = true
	var added []string
	for _, pre := range lib.All() {
		if !pre.OfferedOn(goos) {
			continue
		}
		if _, have := d.Lookup(pre.ID); have {
			continue
		}
		if !found(ctx, pre, goos, p) {
			continue
		}
		a := FromPreset(pre, lib.Version(), goos)
		if pre.Screening != "" && screeningOff != nil && screeningOff(pre.ID) && a.Server != nil {
			delete(a.Server.Env, pre.Screening)
			a.From.Edited = true
		}
		if err := d.Add(a); err == nil {
			added = append(added, a.ID)
		}
	}
	return added
}

// found says a preset's application and server are ready here.
func found(ctx context.Context, pre library.Preset, goos string, p Probe) bool {
	if pre.Attachment == library.AttachmentStdio {
		if pre.Launch == nil || len(pre.Launch.Command) == 0 || p.LookPath == nil {
			return false
		}
		if _, err := p.LookPath(pre.Launch.Command[0]); err != nil {
			return false
		}
	}
	// A port that answers says only that something listens there. It finds
	// the application only when the preset names nowhere it is installed on
	// this system; otherwise one of those places must hold it.
	installed := pre.Installed[goos]
	if len(installed) == 0 {
		return pre.Listener.Port > 0 && p.Listening != nil && p.Listening(ctx, pre.Listener.Port)
	}
	for _, where := range installed {
		switch {
		case strings.HasPrefix(where, "flatpak:"):
			if p.Flatpak != nil && p.Flatpak(strings.TrimPrefix(where, "flatpak:")) {
				return true
			}
		case filepath.IsAbs(where) || strings.Contains(where, `:\`):
			if p.Exists != nil && p.Exists(where) {
				return true
			}
		default:
			if p.LookPath != nil {
				if _, err := p.LookPath(where); err == nil {
					return true
				}
			}
		}
	}
	return false
}
