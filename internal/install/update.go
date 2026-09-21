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
		info := UpdateInfo{
			Key: installed.Key{
				Registry: rec.Registry, Namespace: rec.Namespace, Name: rec.Name,
				Target: rec.Target, Dir: rec.Dir,
			},
			Current: rec.Version,
		}
		origin := originFor(rec.Record)
		token := settings.TokenFor(origin)
		client, cerr := registry.New(origin, token)
		if cerr != nil {
			info.Error = cerr.Error()
			out = append(out, info)
			continue
		}
		res, rerr := registry.Resolve(ctx, client, rec.Namespace+"/"+rec.Name, false)
		if rerr != nil {
			info.Error = rerr.Error()
			out = append(out, info)
			continue
		}
		info.Latest = res.Version
		info.UpdateAvailable = newerThan(res.Version, rec.Version)
		out = append(out, info)
	}
	return out, nil
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
