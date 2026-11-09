package hostedrun

import (
	"net/http"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"
)

// ManagedSlug is the slug of a refusal the organization's managed settings
// held on this computer give, before anything is sent to the registry.
const ManagedSlug = "managed-settings"

// currentManaged reads the managed settings held on this computer; a
// variable so a test can answer for them.
var currentManaged = managed.Current

// managedRefusal is the refusal of op that the managed settings held on
// this computer give, or nil: any run while they cannot be verified or
// this version is older than they allow, a run In Agentry where run.where
// leaves it out, and, for a schedule, schedules an organization forbids.
// The registry answers for its own admission; this holds the app to the
// same locks before it asks.
func managedRefusal(op string, schedule bool) error {
	held := currentManaged()
	reason := held.RunRefusal()
	if reason == "" {
		reason = held.WhereRefusal(managedsettings.WhereHosted)
	}
	if reason == "" && schedule {
		reason = held.Forbidden(managedsettings.Schedules)
	}
	if reason == "" {
		return nil
	}
	return &Refusal{Op: op, Status: http.StatusForbidden, Slug: ManagedSlug, Detail: reason}
}
