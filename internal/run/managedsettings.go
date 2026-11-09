package run

import (
	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/compile"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// currentManaged reads the organization's managed settings held on this
// computer, a package variable purely as a test seam. Production code never
// reassigns it.
var currentManaged = managed.Current

// managedRunRefusal refuses a run before it starts while the managed
// settings held here refuse every run: a document locks a setting this
// version does not know, or a copy cannot be verified. The engine refuses
// the same run on its own; this says why before anything is prepared or
// sent. It is a policy refusal (exit 11), as the runner's own is.
func managedRunRefusal(held managed.Effective) error {
	if reason := held.RunRefusal(); reason != "" {
		return engine.Classify(engine.ExitPolicy, &compile.PolicyRefusal{Reason: reason})
	}
	return nil
}
