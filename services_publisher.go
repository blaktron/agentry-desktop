// The Publisher section (plan forks-and-publisher.md §4.2, D12; M6,
// agentry-desktop#705): the QuickAgents in the signed-in person's own
// namespace and their organizations', searched and filtered as the site's
// Publisher list does (agentry-web#1236), and the owner actions on them:
// Make private, Make public, Retire, Restore and Delete. Each action is one
// call to the core (agentry#942), which decides every rule (the
// administrators' rung, the private-agent cap, DEV-115's unscanned
// versions, a fork staying private, the typed name of a delete), and the
// sign-in token reaches them through agent:manage (agentry#993, DEV-225).
// A token minted before the grant carried it is refused, and the section
// says to sign in again.
//
// Every action is asked again in a native dialog first (D-030: the page is
// not a principal), and a refusal is said in the site's words
// (agentry-web lib/publisher.ts), never as a status code alone.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// PublisherAPI is the Publisher section's binding.
type PublisherAPI struct {
	app *App
}

func (p *PublisherAPI) ctx() context.Context {
	if p.app != nil && p.app.ctx != nil {
		return p.app.ctx
	}
	return context.Background()
}

// The owner-action scopes: the sign-in grant's agent:manage, or a pasted
// token's org:admin, which also admits them (DEV-225).
const (
	scopeAgentManage = "agent:manage"
	scopeOrgAdmin    = "org:admin"
)

// PublisherNamespace is a namespace whose QuickAgents the section lists:
// the person's own, then each organization they are an active member of.
// Admin says whether the person may take the owner actions there, which
// only the namespace itself and an organization's admins may.
type PublisherNamespace struct {
	Handle string `json:"handle"`
	Kind   string `json:"kind"` // "user" or "org"
	Admin  bool   `json:"admin"`
}

// PublisherView is what the section opens with. SignedIn false draws the
// Sign in notice and nothing else. CanManage says the token carries a scope
// that admits the owner actions; ScopesKnown false means the registry did
// not say, and the actions are offered for the registry to decide.
type PublisherView struct {
	SignedIn    bool                 `json:"signedIn"`
	Handle      string               `json:"handle,omitempty"`
	Namespaces  []PublisherNamespace `json:"namespaces"`
	CanManage   bool                 `json:"canManage"`
	ScopesKnown bool                 `json:"scopesKnown"`
}

// PublisherForkOrigin is what a fork was taken from (agentry#943).
type PublisherForkOrigin struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	// Available says the origin can still be read, so its name is a link.
	Available bool `json:"available"`
}

// PublisherAgent is a row of the list.
type PublisherAgent struct {
	Namespace       string               `json:"namespace"`
	Name            string               `json:"name"`
	Summary         string               `json:"summary"`
	Visibility      string               `json:"visibility"`
	RetiredAt       string               `json:"retiredAt,omitempty"`
	QuarantineState string               `json:"quarantineState"`
	ForkedFrom      *PublisherForkOrigin `json:"forkedFrom,omitempty"`
	ForkState       string               `json:"forkState,omitempty"`
	ForkCount       *int                 `json:"forkCount,omitempty"`
	Installs30d     *int                 `json:"installs30d,omitempty"`
	LastSubmittedAt string               `json:"lastSubmittedAt,omitempty"`
	CreatedAt       string               `json:"createdAt"`
}

// PublisherQuery is the list's search, filters and sort, in the core's
// terms (GET /agents?namespace=, agentry#942, #943). Visibility is not
// the core's and is applied by the page. Anything unknown reads as no
// filter, and the sort as newest, as the site reads a URL.
type PublisherQuery struct {
	Q     string `json:"q"`
	State string `json:"state"` // "", "active", "retired"
	Forks string `json:"forks"` // "", "only", "none"
	Sort  string `json:"sort"`  // "newest", "installs", "submitted", "name"
}

// PublisherDone is what an owner action answers: the sentence it says, and
// the QuickAgent as the registry now has it (none after a delete).
// Cancelled is a No in the native dialog: nothing was sent.
type PublisherDone struct {
	Cancelled bool            `json:"cancelled"`
	Sentence  string          `json:"sentence,omitempty"`
	Agent     *PublisherAgent `json:"agent,omitempty"`
}

// Overview says who is signed in to reg, which namespaces the section
// lists, and whether the token reaches the owner actions. Signed out is an
// answer, not an error.
func (p *PublisherAPI) Overview(reg string) (*PublisherView, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return &PublisherView{Namespaces: []PublisherNamespace{}}, nil
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	ctx := p.ctx()
	res, err := client.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if res.StatusCode() == http.StatusUnauthorized {
		return &PublisherView{Namespaces: []PublisherNamespace{}}, nil
	}
	if res.JSON200 == nil {
		return nil, registry.Problem("read your account", res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body)
	}
	u := res.JSON200
	v := &PublisherView{SignedIn: true, Handle: u.Handle, Namespaces: []PublisherNamespace{{Handle: u.Handle, Kind: "user", Admin: true}}}
	if u.Orgs != nil {
		for _, o := range *u.Orgs {
			// A pending membership holds nothing yet (agentry#361).
			if o.State != nil && *o.State == apiclient.MembershipStatePending {
				continue
			}
			v.Namespaces = append(v.Namespaces, PublisherNamespace{Handle: o.Handle, Kind: "org", Admin: o.Role == "admin"})
		}
	}
	if scopes, ok := tokenScopes(ctx, url, token); ok {
		v.ScopesKnown = true
		v.CanManage = hasString(scopes, scopeAgentManage) || hasString(scopes, scopeOrgAdmin)
	}
	return v, nil
}

var (
	publisherStates = map[string]apiclient.ListAgentsParamsState{
		"active": apiclient.ListAgentsParamsStateActive, "retired": apiclient.ListAgentsParamsStateRetired,
	}
	publisherForks = map[string]apiclient.ListAgentsParamsForks{
		"only": apiclient.ListAgentsParamsForksOnly, "none": apiclient.ListAgentsParamsForksNone,
	}
	publisherSorts = map[string]apiclient.ListAgentsParamsSort{
		"newest": apiclient.ListAgentsParamsSortNewest, "installs": apiclient.ListAgentsParamsSortInstalls,
		"submitted": apiclient.ListAgentsParamsSortSubmitted, "name": apiclient.ListAgentsParamsSortName,
	}
)

// publisherQueryMax is the core's own bound on the search term.
const publisherQueryMax = 200

// List reads ns's QuickAgents for the section: private ones too, for its
// owner and its organization's active members (the core decides that).
func (p *PublisherAPI) List(reg, ns string, q PublisherQuery) ([]PublisherAgent, error) {
	if !manifest.ValidNamespace(ns) {
		return nil, fmt.Errorf("%q is not a namespace", ns)
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	params := &apiclient.ListAgentsParams{Namespace: ns}
	if term := strings.TrimSpace(q.Q); term != "" {
		if r := []rune(term); len(r) > publisherQueryMax {
			term = string(r[:publisherQueryMax])
		}
		params.Q = &term
	}
	if s, ok := publisherStates[q.State]; ok {
		params.State = &s
	}
	if f, ok := publisherForks[q.Forks]; ok {
		params.Forks = &f
	}
	sort, ok := publisherSorts[q.Sort]
	if !ok {
		sort = apiclient.ListAgentsParamsSortNewest
	}
	params.Sort = &sort
	res, err := client.ListAgentsWithResponse(p.ctx(), params)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if res.JSON200 == nil {
		return nil, registry.Problem("list "+ns+"'s QuickAgents", res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body)
	}
	out := make([]PublisherAgent, 0, len(*res.JSON200))
	for _, a := range *res.JSON200 {
		out = append(out, publisherAgent(a))
	}
	return out, nil
}

func publisherAgent(a apiclient.Agent) PublisherAgent {
	v := PublisherAgent{
		Namespace: a.Namespace, Name: a.Name, Summary: a.Summary,
		Visibility: string(a.Visibility), QuarantineState: string(a.QuarantineState),
		ForkCount: a.ForkCount, Installs30d: a.Installs30d,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if a.RetiredAt != nil {
		v.RetiredAt = a.RetiredAt.UTC().Format(time.RFC3339)
	}
	if a.LastSubmittedAt != nil {
		v.LastSubmittedAt = a.LastSubmittedAt.UTC().Format(time.RFC3339)
	}
	if a.ForkedFrom != nil {
		v.ForkedFrom = &PublisherForkOrigin{
			Namespace: a.ForkedFrom.Namespace, Name: a.ForkedFrom.Name,
			Version: a.ForkedFrom.Version, Available: a.ForkedFrom.Available,
		}
	}
	if a.ForkState != nil {
		v.ForkState = string(*a.ForkState)
	}
	return v
}

// publisherAct names an owner action, as the site's OwnerAct does.
type publisherAct string

const (
	actPrivate publisherAct = "private"
	actPublic  publisherAct = "public"
	actRetire  publisherAct = "retire"
	actRestore publisherAct = "restore"
	actDelete  publisherAct = "delete"
)

// The native dialog's question per action: what it does to the people who
// use the QuickAgent, before anything is sent.
func publisherQuestion(act publisherAct, ref string) (title, message string) {
	switch act {
	case actPrivate:
		return "Make " + ref + " private?", ref + " leaves Browse and search, and only its readers can open it. People who installed it keep their copy, but cannot update it or install it again."
	case actPublic:
		return "Make " + ref + " public?", "Anyone can find " + ref + " in Browse and install it."
	case actRetire:
		return "Retire " + ref + "?", ref + " becomes private and takes no new versions. People who installed it keep their copy. Restore brings it back as it was."
	case actRestore:
		return "Restore " + ref + "?", ref + " goes back to the visibility it had when it was retired, which may be public, and takes new versions again."
	default:
		return "Delete " + ref + "?", "Every version of " + ref + " is withdrawn, nobody can install or run it again, and the name stays held in its namespace for good. This cannot be undone."
	}
}

// publisherDoneSentence is what an action that went through says (the
// site's doneSentence).
func publisherDoneSentence(act publisherAct, ref string) string {
	switch act {
	case actPrivate:
		return ref + " is private now. It left Browse and search, and only its readers can open it."
	case actPublic:
		return ref + " is public now: anyone can find it in Browse and install it."
	case actRetire:
		return ref + " is retired. It is private, takes no new versions, and Restore brings it back as it was."
	case actRestore:
		return ref + " is restored, with the visibility it had when it was retired."
	default:
		return ref + " is deleted."
	}
}

const nothingChanged = "Nothing changed:"

// publisherMistyped is what a Delete whose typed name does not match says.
func publisherMistyped(ns, name string) string {
	return fmt.Sprintf("%s to delete %s/%s, type its name, %s, exactly.", nothingChanged, ns, name, name)
}

// publisherRefusal is what an action the registry refused says, in the
// site's words (refusalSentence): the problem's slug decides the
// sentence, and its detail is used only where no sentence here fits. A
// 403 to a token without agent:manage says to sign in again, which is the
// fix; the site's 403 names the rung, which a session can only lack.
func publisherRefusal(act publisherAct, ns, name string, status int, slug, detail string, scopeless bool) error {
	ref := ns + "/" + name
	var s string
	switch {
	case slug == "account-unactivated" && detail != "":
		s = detail
	case status == http.StatusUnauthorized:
		s = nothingChanged + " you are signed out. Sign in again under Settings, then retry."
	case status == http.StatusForbidden && scopeless:
		s = nothingChanged + " this sign-in was made before Agentry Desktop could retire, restore, delete or change who sees a QuickAgent. Sign out and sign in again under Settings, then retry."
	case status == http.StatusForbidden:
		s = fmt.Sprintf("%s only %s itself, or an admin of the %s organization, can change who sees %s, or retire or delete it.", nothingChanged, ns, ns, ref)
	case status == http.StatusNotFound:
		s = fmt.Sprintf("%s %s is not there any more. It may have been deleted.", nothingChanged, ref)
	case slug == "private-agent-cap":
		s = fmt.Sprintf("%s %s already holds as many private QuickAgents as its subscription allows. Make one of them public, retire one, or upgrade, then try again.", nothingChanged, ns)
	case slug == "agent-unscanned":
		s = fmt.Sprintf("%s only a version that passed the gate is ever public, and %s has versions admitted without a scan, or still admits them. Yank those versions and turn off admitting unscanned submissions in its settings on the site, then try again.", nothingChanged, ref)
	case slug == "agent-retired":
		s = fmt.Sprintf("%s %s is retired. Restore it first.", nothingChanged, ref)
	case slug == "agent-not-retired":
		s = fmt.Sprintf("%s %s is not retired, so there was nothing to restore.", nothingChanged, ref)
	case slug == "confirm-name":
		s = publisherMistyped(ns, name)
	case slug == "fork-stays-private":
		s = fmt.Sprintf("%s %s is a fork, and a fork stays private for good.", nothingChanged, ref)
	case act == actDelete && status == http.StatusConflict && detail != "":
		s = nothingChanged + " " + detail
	case status == http.StatusTooManyRequests:
		s = nothingChanged + " too many requests. Wait a minute, then try again."
	case status >= 500:
		s = nothingChanged + " the registry did not answer. Try again in a moment."
	case detail != "":
		s = nothingChanged + " " + detail
	default:
		s = nothingChanged + " the registry refused it."
	}
	return errors.New(s)
}

// problemSlug is the last segment of a problem's type URI
// (https://agentry.dev/problems/<slug>), and its detail. The generated
// client decodes the problem only for a status the spec leaves to its
// default, so for a declared one (the delete's 409 and 422) it is read
// from the body here.
func problemSlug(pr *apiclient.Problem, body []byte) (slug, detail string) {
	if pr == nil {
		var doc apiclient.Problem
		if json.Unmarshal(body, &doc) != nil {
			return "", ""
		}
		pr = &doc
	}
	if i := strings.LastIndex(pr.Type, "/"); i >= 0 {
		slug = pr.Type[i+1:]
	}
	if pr.Detail != nil {
		detail = *pr.Detail
	}
	return slug, detail
}

// SetVisibility makes ns/name public or private.
func (p *PublisherAPI) SetVisibility(reg, ns, name, visibility string) (*PublisherDone, error) {
	switch visibility {
	case "public":
		return p.act(reg, ns, name, actPublic, "")
	case "private":
		return p.act(reg, ns, name, actPrivate, "")
	}
	return nil, fmt.Errorf("visibility must be public or private, not %q", visibility)
}

// Retire retires ns/name.
func (p *PublisherAPI) Retire(reg, ns, name string) (*PublisherDone, error) {
	return p.act(reg, ns, name, actRetire, "")
}

// Restore restores a retired ns/name.
func (p *PublisherAPI) Restore(reg, ns, name string) (*PublisherDone, error) {
	return p.act(reg, ns, name, actRestore, "")
}

// Delete deletes ns/name once the person has typed its name: checked here
// first, so a slip costs no dialog and no call, and sent on, since the core
// asks for it on a QuickAgent with versions.
func (p *PublisherAPI) Delete(reg, ns, name, typed string) (*PublisherDone, error) {
	if strings.TrimSpace(typed) != name {
		return nil, errors.New(publisherMistyped(ns, name))
	}
	return p.act(reg, ns, name, actDelete, name)
}

// act asks the native question, then makes the one call.
func (p *PublisherAPI) act(reg, ns, name string, act publisherAct, confirm string) (*PublisherDone, error) {
	if !manifest.ValidNamespace(ns) || !manifest.ValidName(name) {
		return nil, fmt.Errorf("%q is not a QuickAgent's name", ns+"/"+name)
	}
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	ref := ns + "/" + name
	if p.app == nil {
		return nil, errors.New("no confirmation dialog is available; refusing")
	}
	title, message := publisherQuestion(act, ref)
	if err := p.app.askConsent(title, message); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return &PublisherDone{Cancelled: true}, nil
		}
		return nil, err
	}

	ctx := p.ctx()
	var (
		status int
		agent  *apiclient.Agent
		pr     *apiclient.Problem
		body   []byte
	)
	switch act {
	case actPrivate, actPublic:
		vis := apiclient.UpdateAgentJSONBodyVisibility(act)
		res, err := client.UpdateAgentWithResponse(ctx, ns, name, apiclient.UpdateAgentJSONRequestBody{Visibility: &vis})
		if err != nil {
			return nil, unreachable(err)
		}
		status, agent, pr, body = res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body
	case actRetire:
		res, err := client.RetireAgentWithResponse(ctx, ns, name)
		if err != nil {
			return nil, unreachable(err)
		}
		status, agent, pr, body = res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body
	case actRestore:
		res, err := client.RestoreAgentWithResponse(ctx, ns, name)
		if err != nil {
			return nil, unreachable(err)
		}
		status, agent, pr, body = res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body
	case actDelete:
		res, err := client.DeleteAgentWithResponse(ctx, ns, name, &apiclient.DeleteAgentParams{Confirm: &confirm})
		if err != nil {
			return nil, unreachable(err)
		}
		status, pr, body = res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body
	}
	if status >= 200 && status < 300 {
		done := &PublisherDone{Sentence: publisherDoneSentence(act, ref)}
		if agent != nil {
			a := publisherAgent(*agent)
			done.Agent = &a
		}
		return done, nil
	}
	slug, detail := problemSlug(pr, body)
	scopeless := false
	if status == http.StatusForbidden {
		if scopes, ok := tokenScopes(ctx, url, token); ok {
			scopeless = !hasString(scopes, scopeAgentManage) && !hasString(scopes, scopeOrgAdmin)
		}
	}
	return nil, publisherRefusal(act, ns, name, status, slug, detail, scopeless)
}

// unreachable is an action the registry did not answer.
func unreachable(err error) error {
	return fmt.Errorf("%s the registry did not answer. Try again in a moment. (%w)", nothingChanged, err)
}
