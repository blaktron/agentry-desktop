// Fork in the desktop (plan forks-and-publisher.md §5, D6–D10, D12; M7,
// agentry-desktop#706): a signed-in person forks a public QuickAgent into
// their own namespace or an organization they publish for, under the same
// name unless that name is taken there. The core decides every rule
// (agentry#943): the origin public, not quarantined or blocked and with a
// published version; the caller able to create in the target; the name
// free; the quota and the private-agent cap. The fork's first version is
// a submission through the pipeline, charged like any. Fork needs only
// agent:publish, which every sign-in grant carries.
//
// The fork is asked again in a native dialog first (D-030), saying the
// site's three facts about a fork (agentry-web lib/fork.ts FORK_TERMS),
// and a refusal is said in the site's words.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// forkTerms are what a fork is, said before it is made: the site's
// FORK_TERMS, the third from DEV-224.
var forkTerms = []string{
	"A fork stays private for good. It can be changed and given new versions, but it can never be made public.",
	"Its first version is a submission through the pipeline, and it costs credits as any submission does.",
	"Hosted runs of the fork pay the original's publisher their Publisher Earnings share, unless the fork is in their own namespace or their organization's. Runs on your own computer pay nobody.",
}

// ForkView is what Fork answers. A fork that was made names itself and
// says so (Sentence). A refusal is Refusal, in words; AskName says it was
// the name (taken, or held by a deleted QuickAgent), so the page asks for
// another. Cancelled is a No in the native dialog: nothing was sent.
type ForkView struct {
	Cancelled bool   `json:"cancelled"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Sentence  string `json:"sentence,omitempty"`
	Refusal   string `json:"refusal,omitempty"`
	AskName   bool   `json:"askName,omitempty"`
}

// ForkTargets are the namespaces a fork may go into: the person's own,
// then each organization where they are an active admin or member (a
// security reviewer publishes nothing; the site's forkTargets). Nothing
// signed in, or an address Settings would not send the token to, lists
// none.
func (p *PublisherAPI) ForkTargets(reg string) ([]string, error) {
	acct, err := p.signedInAccount(reg)
	if err != nil || acct == nil {
		return []string{}, err
	}
	out := []string{acct.handle}
	for _, o := range acct.orgs {
		if (o.role == "admin" || o.role == "member") && o.handle != "" && !hasString(out, o.handle) {
			out = append(out, o.handle)
		}
	}
	return out, nil
}

// Fork forks ns/name into target, as newName when it is not "" (else the
// origin's name).
func (p *PublisherAPI) Fork(reg, ns, name, target, newName string) (*ForkView, error) {
	if !manifest.ValidNamespace(ns) || !manifest.ValidName(name) {
		return nil, fmt.Errorf("%q is not a QuickAgent's name", ns+"/"+name)
	}
	if !manifest.ValidNamespace(target) {
		return nil, fmt.Errorf("%q is not a namespace", target)
	}
	newName = strings.TrimSpace(newName)
	if newName != "" && !manifest.ValidName(newName) {
		return &ForkView{Refusal: forkNotForked + " " + newName + " is not a name a QuickAgent can have: lowercase letters, digits and hyphens, starting and ending with a letter or digit.", AskName: true}, nil
	}
	forkName := name
	if newName != "" {
		forkName = newName
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	if p.app == nil {
		return nil, errors.New("no confirmation dialog is available; refusing")
	}
	origin, fork := ns+"/"+name, target+"/"+forkName
	if err := p.app.askConsent("Fork "+origin+" into "+fork+"?", strings.Join(forkTerms, "\n\n")); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return &ForkView{Cancelled: true}, nil
		}
		return nil, err
	}
	body := apiclient.ForkAgentJSONRequestBody{Namespace: target}
	if newName != "" {
		body.Name = &newName
	}
	res, err := client.ForkAgentWithResponse(p.ctx(), ns, name, body)
	if err != nil {
		return &ForkView{Refusal: forkUnreachable()}, nil
	}
	if res.JSON202 != nil {
		return &ForkView{
			Namespace: res.JSON202.Namespace, Name: res.JSON202.Name,
			Sentence: "Forked " + origin + " into " + res.JSON202.Namespace + "/" + res.JSON202.Name + ".",
		}, nil
	}
	slug, detail := registry.ProblemSlug(res.ApplicationproblemJSONDefault, res.Body)
	return &ForkView{
		Refusal: forkRefusal(origin, target, forkName, res.StatusCode(), slug, detail),
		AskName: slug == "name-taken" || slug == "name-held",
	}, nil
}

const forkNotForked = "Not forked:"

func forkUnreachable() string {
	return forkNotForked + " the registry did not answer. Try again in a moment."
}

// forkRefusal says why a fork was not made, in the site's words
// (forkRefusal in agentry-web lib/fork.ts). Never a status code alone.
func forkRefusal(origin, target, name string, status int, slug, detail string) string {
	nf := forkNotForked
	switch {
	case slug == "account-unactivated" && detail != "":
		return detail
	case status == http.StatusUnauthorized:
		return nf + " you are signed out. Sign in again under Settings, then retry."
	case slug == "validation" && strings.Contains(strings.ToLower(detail), "quota"):
		// The namespace's agent quota is the core's validation refusal,
		// named in its detail.
		return fmt.Sprintf("%s %s already holds as many QuickAgents as it may. Delete one there, then try again.", nf, target)
	case slug == "name-taken":
		return fmt.Sprintf("%s %s already has a QuickAgent called %s. Choose another name for the fork.", nf, target, name)
	case slug == "name-held":
		return fmt.Sprintf("%s %s belonged to a QuickAgent deleted from %s, and that name is never used again. Choose another name for the fork.", nf, name, target)
	case slug == "fork-source":
		return fmt.Sprintf("%s only a public QuickAgent that is not quarantined or blocked can be forked, and %s is not one now.", nf, origin)
	case slug == "nothing-to-fork":
		return fmt.Sprintf("%s %s has no published version yet, and a fork starts from one.", nf, origin)
	case slug == "private-agent-cap":
		return fmt.Sprintf("%s %s already holds as many private QuickAgents as its subscription allows, and a fork is private. Retire one, or upgrade, then try again.", nf, target)
	case status == http.StatusForbidden && detail != "":
		return nf + " " + detail
	case status == http.StatusForbidden:
		return fmt.Sprintf("%s you cannot create QuickAgents in %s.", nf, target)
	case status == http.StatusNotFound && detail != "" && slug != "not-found":
		return nf + " " + detail
	case status == http.StatusNotFound:
		return fmt.Sprintf("%s %s or %s is not there any more.", nf, origin, target)
	case status == http.StatusTooManyRequests:
		return nf + " too many requests. Wait a minute, then try again."
	case status >= 500:
		return forkUnreachable()
	case detail != "":
		return nf + " " + detail
	}
	return nf + " the registry refused it."
}
