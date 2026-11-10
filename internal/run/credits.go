package run

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/managed"
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

// operationsClient is the client operations are opened and ended with; a
// test replaces it.
var operationsClient = func() *http.Client { return registry.HTTPClient }

// CreditsPay opens an AI Review or web search operation an organization's
// credits pay for (a drafter.Pay), on the registry its managed settings came
// from, else the one this app is on, as the account signed in there, and
// says it and what it cost on w. The caller ends it.
func CreditsPay(w io.Writer) drafter.Pay {
	return func(ctx context.Context, reg, purpose string) (*managed.Operation, error) {
		if reg == "" {
			if s, err := settings.Load(); err == nil {
				reg = s.DefaultRegistry
			}
		}
		reg = strings.TrimRight(reg, "/")
		op, err := managed.OpenOperation(ctx, operationsClient(), reg, settings.TokenFor(reg), purpose)
		if err != nil {
			return nil, err
		}
		what := "AI Review"
		if purpose == managed.PurposeSearch {
			what = "Web search, with AI Review reading every result,"
		}
		cost := "with no credit charged"
		switch {
		case op.Credits == 1:
			cost = "1 credit, spent whether or not the work finishes"
		case op.Credits > 1:
			cost = fmt.Sprintf("%d credits, spent whether or not the work finishes", op.Credits)
		}
		if w != nil {
			_, _ = fmt.Fprintf(w, "%s is paid with your organization's credits: an operation on %s, %s.\n", what, reg, cost)
		}
		return op, nil
	}
}
