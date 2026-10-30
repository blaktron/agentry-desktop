package run

import (
	"github.com/blaktron/agentry-cli/pkg/runner/inference"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// CreditsClient is the credits-session client for a policy that pays for
// its model with Agentry credits (model provider agentry): the registry its
// model endpoint is under, as the account this computer is signed in to
// there. Nil for any other provider, and for a registry with no sign-in
// here, which a run then refuses as signed out. The sign-in is read at each
// call, so a renewal an hour on uses the sign-in as it is then, and it never
// reaches the run.
func CreditsClient(pol *policy.Policy) *inference.Client {
	if pol == nil || pol.Model.Provider != policy.ProviderAgentry {
		return nil
	}
	reg, err := policy.AgentryRegistry(pol.Model)
	if err != nil || settings.TokenFor(reg) == "" {
		return nil
	}
	return inference.New(reg, func() (string, error) { return settings.TokenFor(reg), nil }, registry.HTTPClient)
}
