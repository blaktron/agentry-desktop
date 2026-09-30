package install

// Update checks: for every installed record, ask its registry what the
// latest resolvable version is and compare. Errors are per-record data, not
// failures — an unreachable registry for one install must not hide the
// update that exists for another.

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Masterminds/semver/v3"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// CheckUpdates resolves the latest version for every install record.
func CheckUpdates(ctx context.Context) ([]UpdateInfo, error) {
	recs, err := List()
	if err != nil {
		return nil, err
	}
	out := make([]UpdateInfo, 0, len(recs))
	for _, rec := range recs {
		out = append(out, checkItem(ctx, rec))
	}
	return out, nil
}

// CheckUpdateForKey resolves the latest version for one record by its
// key — what a client refreshes after it installed, updated or removed
// that one record. Nil when no such record exists.
func CheckUpdateForKey(ctx context.Context, key installed.Key) (*UpdateInfo, error) {
	items, err := List()
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Registry == key.Registry && it.Namespace == key.Namespace && it.Name == key.Name && it.Target == key.Target && it.Dir == key.Dir {
			info := checkItem(ctx, it)
			return &info, nil
		}
	}
	return nil, nil //nolint:nilnil // no record is the answer, not an error
}

// CheckUpdateFor resolves the latest version for one QuickAgent's
// runner-home record — what the agent page asks on opening
// (agentry-desktop#147, D8) — through the record's origin, with the one
// rule every "newer" in this app follows (newerThan). Nil when nothing is
// installed in the runner home for it: there is nothing to update.
func CheckUpdateFor(ctx context.Context, namespace, name string) (*UpdateInfo, error) {
	items, err := List()
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Target == installdir.Target && it.Namespace == namespace && it.Name == name {
			info := checkItem(ctx, it)
			return &info, nil
		}
	}
	return nil, nil //nolint:nilnil // no record is the answer, not an error
}

// checkItem is checkUpdate with the item's two facts carried along.
func checkItem(ctx context.Context, it Item) UpdateInfo {
	info := checkUpdate(ctx, it.Record)
	info.Manageable, info.Present = it.Manageable, it.Present
	return info
}

// checkUpdate is one record's question to its registry. Errors are the
// record's data, not failures — an unreachable registry for one install
// must not hide the update that exists for another.
func checkUpdate(ctx context.Context, rec installed.Record) UpdateInfo {
	info := UpdateInfo{
		Key: installed.Key{
			Registry: rec.Registry, Namespace: rec.Namespace, Name: rec.Name,
			Target: rec.Target, Dir: rec.Dir,
		},
		Current: rec.Version,
	}
	if rec.Local() {
		// Installed from a folder: no registry holds it, so there is no
		// newer version to ask about, and nothing is sent anywhere.
		return info
	}
	origin := originFor(rec)
	token := settings.TokenFor(origin)
	client, cerr := registry.New(origin, token)
	if cerr != nil {
		info.Error = cerr.Error()
		return info
	}
	res, rerr := registry.Resolve(ctx, client, rec.Namespace+"/"+rec.Name, false)
	if rerr != nil {
		info.Error = rerr.Error()
		return info
	}
	info.Latest = res.Version
	info.UpdateAvailable = newerThan(res.Version, rec.Version)
	return info
}

// newerThan reports latest > current under semver; unparsable versions fall
// back to string inequality so a registry that serves something odd still
// surfaces as "different", never as silently up to date.
func newerThan(latest, current string) bool {
	lv, lerr := semver.NewVersion(latest)
	cv, cerr := semver.NewVersion(current)
	if lerr != nil || cerr != nil {
		return latest != current
	}
	return lv.GreaterThan(cv)
}

// DirForRecord reconstructs the Prepare `dir` argument that reproduces a
// record's destination, so "Update" resolves to the same key the original
// install recorded.
func DirForRecord(rec installed.Record) (string, error) {
	switch rec.Target {
	case manifest.InterfaceClaudeSkill:
		if rec.Managed {
			return "", nil // the default ~/.claude/skills layout
		}
		return filepath.Dir(rec.Dir), nil
	case installdir.Target:
		return "", nil // one home; the update is the one-click install
	case manifest.InterfaceMCP, manifest.InterfaceAgentsMD:
		// Managed layout is <project>/agents/<name>.
		return filepath.Dir(filepath.Dir(rec.Dir)), nil
	}
	return "", fmt.Errorf("unknown target %q", rec.Target)
}
