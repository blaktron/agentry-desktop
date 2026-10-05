// Publisher Earnings in the desktop (plan creator-monetization.md D10,
// D13; agentry-desktop#613): the one line the In Agentry run setup carries
// when a release's publisher earns a share of what runs of it spend, and
// the ask to star someone else's QuickAgent once a hosted run or session
// of it ended well. Both follow the site's rules (agentry-web#1087,
// lib/earnings.ts): neither is for the reader's own QuickAgent or one of an
// organization they are an active member of, whose runs earn nothing (D5);
// the ask is gone once the QuickAgent is starred; and a read that does not
// answer draws nothing, so neither is ever shown on a guess. The earnings
// pages themselves are the site's.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// publisherEarnsHere reports whether the run setup says ref's publisher
// earns a share: the registry mounts billing (exts, its /meta), the program
// says the publisher earns now, the QuickAgent is public, and the reader is
// neither its publisher nor an active member of it. The program's public
// read goes first, since for most publishers it ends the question.
func publisherEarnsHere(ctx context.Context, reg, token, ref string, exts []string) bool {
	if !hasString(exts, "billing") {
		return false
	}
	p, err := manifest.ParseRef(ref)
	if err != nil || !publisherEarns(ctx, reg, p.Namespace) {
		return false
	}
	client, err := registry.New(reg, token)
	if err != nil {
		return false
	}
	a, err := client.GetAgentWithResponse(ctx, p.Namespace, p.Name)
	if err != nil || a.JSON200 == nil || a.JSON200.Visibility != apiclient.AgentVisibilityPublic {
		return false
	}
	own, known := ownPublisher(ctx, client, p.Namespace)
	return known && !own
}

// publisherEarns is the program's public answer for a namespace (GET
// /x/billing/earnings/publishers/{handle}, agentry-enterprise#393): true
// while the program is on and the publisher has joined. It is asked with
// no token, as it is public; false wherever it does not answer, including
// a registry from before the program (404).
func publisherEarns(ctx context.Context, reg, ns string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(reg, "/")+"/api/v1/x/billing/earnings/publishers/"+neturl.PathEscape(ns), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	res, err := registry.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return false
	}
	var v struct {
		Earns bool `json:"earns"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&v) != nil {
		return false
	}
	return v.Earns
}

// ownPublisher reports whether the signed-in reader is ns or an active
// member of it (the site's ownPublisher: a pending membership holds
// nothing yet), and whether the registry said who the reader is at all.
func ownPublisher(ctx context.Context, client *apiclient.ClientWithResponses, ns string) (own, known bool) {
	res, err := client.GetCurrentUserWithResponse(ctx)
	if err != nil || res.JSON200 == nil {
		return false, false
	}
	u := res.JSON200
	if u.Handle == ns {
		return true, true
	}
	if u.Orgs != nil {
		for _, o := range *u.Orgs {
			if o.Handle == ns && (o.State == nil || string(*o.State) != "pending") {
				return true, true
			}
		}
	}
	return false, true
}

// StarAskView is the ask a finished hosted run or a closed session shows:
// whether to ask, and the QuickAgent's "ns/name" that the ask names and
// StarQuickAgent stars.
type StarAskView struct {
	Ask  bool   `json:"ask"`
	Name string `json:"name,omitempty"`
}

// StarAsk reports whether the result of a hosted run or session of ref
// asks the person to star the QuickAgent: it is not theirs or an
// organization's they are an active member of, and the registry says they
// have not starred it. The panel asks only once the run ended well
// (starAskWanted, frontend/src/earnings.ts). It asks the same whether or
// not the publisher earns (D10). Nothing signed in, a ref that is not one
// and any read that does not answer ask nothing.
func (r *RunAPI) StarAsk(reg, ref string) (*StarAskView, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, err
	}
	no := &StarAskView{}
	token := settings.TokenFor(reg)
	p, err := manifest.ParseRef(ref)
	if token == "" || err != nil {
		return no, nil
	}
	ctx := r.ctx()
	client, err := registry.New(reg, token)
	if err != nil {
		return no, nil
	}
	if own, known := ownPublisher(ctx, client, p.Namespace); own || !known {
		return no, nil
	}
	st, err := client.GetAgentStatsWithResponse(ctx, p.Namespace, p.Name)
	if err != nil || st.JSON200 == nil || st.JSON200.Starred == nil || *st.JSON200.Starred {
		return no, nil
	}
	return &StarAskView{Ask: true, Name: p.Namespace + "/" + p.Name}, nil
}

// StarQuickAgent stars name ("ns/name", as StarAsk named it) for the
// signed-in person: PUT /user/stars/{ns}/{name}, which the desktop's
// agent:read grant may do. Anything but a bare ns/name is refused unsent.
func (r *RunAPI) StarQuickAgent(reg, name string) error {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return err
	}
	ns, n, ok := strings.Cut(name, "/")
	if !ok || !starNamespaceRE.MatchString(ns) || !starNameRE.MatchString(n) {
		return fmt.Errorf("%q is not a QuickAgent's name", name)
	}
	token := settings.TokenFor(reg)
	if token == "" {
		return fmt.Errorf("not signed in to %s", originOf(reg))
	}
	client, err := registry.New(reg, token)
	if err != nil {
		return err
	}
	res, err := client.StarAgentWithResponse(r.ctx(), ns, n)
	if err != nil {
		return fmt.Errorf("reach %s: %w", originOf(reg), err)
	}
	if res.StatusCode() != http.StatusNoContent {
		return registry.Problem("star "+name, res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body)
	}
	return nil
}

// The spec's Namespace and AgentName patterns (schemas/openapi.yaml), so no
// path, range or dot segment reaches the star's URL.
var (
	starNamespaceRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?$`)
	starNameRE      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,63}[a-z0-9])?$`)
)
