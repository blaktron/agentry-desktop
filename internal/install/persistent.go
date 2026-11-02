package install

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Manifest is a pending plan's manifest, as Prepare validated it: what a
// caller checks before the person is asked, such as whether the release
// runs as a persistent agent here.
func (e *Engine) Manifest(id string) (*manifest.Manifest, bool) {
	if e == nil {
		return nil, false
	}
	p, ok := e.pending[id]
	if !ok {
		return nil, false
	}
	return p.manifest, true
}

// maxReasonBytes bounds a registry's reason a subagent did not resolve,
// as the consent quotes it.
const maxReasonBytes = 300

// ResolveSubagents pins each subagent a persistent release declares to the
// newest version its constraint admits on the registry regURL, with what
// that version's contract reaches: what an install records and the consent
// shows. A subagent whose newest version within its constraint is
// withdrawn, persistent or does not run autonomously resolves to none, and
// its pin says why.
func ResolveSubagents(ctx context.Context, regURL string, m *manifest.Manifest) []persistent.SubagentPin {
	subs := m.Subagents()
	if len(subs) == 0 {
		return nil
	}
	pins := make([]persistent.SubagentPin, 0, len(subs))
	client, err := registry.New(regURL, settings.TokenFor(regURL))
	for _, dep := range subs {
		pin := persistent.SubagentPin{Ref: dep.Ref, Constraint: dep.Version, Why: dep.Why}
		if err != nil {
			pin.Unresolved = cut(err.Error(), maxReasonBytes)
		} else {
			pin.Version, pin.Reach, pin.Unresolved = resolveSubagent(ctx, client, dep)
		}
		pins = append(pins, pin)
	}
	return pins
}

// resolveSubagent is the version one subagent's constraint resolves to now
// and its reach, or why none does.
func resolveSubagent(ctx context.Context, client *apiclient.ClientWithResponses, dep manifest.AgentDependency) (version, reach, unresolved string) {
	if err := ctx.Err(); err != nil {
		return "", "", err.Error()
	}
	res, err := registry.Resolve(ctx, client, dep.Ref+"@"+dep.Version, false)
	if err != nil {
		return "", "", cut(err.Error(), maxReasonBytes)
	}
	sm, err := registry.ManifestFromResolve(res)
	if err != nil {
		return "", "", cut(err.Error(), maxReasonBytes)
	}
	switch {
	case res.Yanked:
		return "", "", res.Version + " is withdrawn"
	case sm.IsPersistent():
		return "", "", res.Version + " is a persistent QuickAgent, which never runs as a subagent"
	case !sm.Supports(manifest.ModeAutonomous):
		return "", "", res.Version + " does not run autonomously"
	case !dep.SubagentAllows(res.Version):
		return "", "", res.Version + " is outside the constraint " + dep.Version
	}
	return res.Version, SubagentReach(sm.Capabilities), ""
}

// SubagentReach is what a subagent's contract reaches, in a line: what a
// first install's consent asks about, and the keys it takes.
func SubagentReach(c manifest.Capabilities) string {
	what := confirmedCapabilities(c)
	var reach string
	switch len(what) {
	case 0:
		reach = "nothing beyond its model: no network, no programs, no server"
	case 1:
		reach = what[0]
	default:
		reach = strings.Join(what[:len(what)-1], ", ") + " and " + what[len(what)-1]
	}
	if len(c.Secrets) > 0 {
		reach += "; the keys " + strings.Join(c.Secrets, ", ")
	}
	return cut(reach, 400)
}

// cut keeps s within n bytes, on a rune boundary, marking the cut.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}
