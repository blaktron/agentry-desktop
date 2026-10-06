// Declared-MCP-server resolution for desktop runs (RUNNER-PLAN-3),
// mirroring the CLI: reuse the signature store's recorded resolutions
// offline; consult the configured registries only when the store is
// incomplete. EnsureInstalled later refuses recorded-pin drift.
package run

import (
	"context"
	"fmt"
	"io"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func ensureServerResolutions(ctx context.Context, logw io.Writer, serversDir string,
	pol *policy.Policy, m *manifest.Manifest,
) ([]mcpinstall.Resolved, error) {
	// A remote server (agentry-cli#663) is neither resolved nor installed,
	// as in the CLI.
	declared := mcpinstall.Packages(m.Capabilities.MCPServers)
	if len(declared) == 0 {
		return nil, nil
	}
	stored := make([]mcpinstall.Resolved, 0, len(declared))
	for _, srv := range declared {
		meta, err := mcpinstall.LoadMeta(serversDir, mcpinstall.IDFor(srv))
		if err != nil {
			return nil, err
		}
		pkg := srv.NPM
		if pkg == "" {
			pkg = srv.PyPI
		}
		if meta == nil || meta.Package != pkg || meta.Version != srv.Version {
			stored = nil
			break
		}
		stored = append(stored, *meta)
	}
	if len(stored) == len(declared) && len(declared) > 0 {
		return stored, nil
	}
	_, _ = fmt.Fprintf(logw, "resolving %d declared MCP server(s) against the configured registries\n", len(declared))
	return mcpinstall.ResolveAll(ctx, nil, pol.MCPRegistries, declared)
}
