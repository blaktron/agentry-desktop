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
// not a principal; D-070), and a refusal is said in the site's words
// (agentry-web lib/publisher.ts), never as a status code alone.
package main

import (
	"context"
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
var ownerActionScopes = []string{string(apiclient.AgentManage), string(apiclient.OrgAdmin)}

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
	Namespaces  []PublisherNamespace `json:"namespaces"`
	CanManage   bool                 `json:"canManage"`
	ScopesKnown bool                 `json:"scopesKnown"`
}

// PublisherForkOrigin is what a fork was taken from (agentry#943).
type PublisherForkOrigin struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
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
	// ForkState is a fork's copy: copying, ready or failed.
	ForkState   string `json:"forkState,omitempty"`
	Installs30d *int   `json:"installs30d,omitempty"`
	// LastSubmittedAt is present once a version was submitted, which is
	// also how the list tells a QuickAgent with versions (Delete's words).
	LastSubmittedAt string `json:"lastSubmittedAt,omitempty"`
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
// lists, and whether the token reaches the owner actions. Signed out, or a
// registry address Settings would not send the token to, is an answer, not
// an error.
func (p *PublisherAPI) Overview(reg string) (*PublisherView, error) {
	signedOut := &PublisherView{Namespaces: []PublisherNamespace{}}
	url, token, err := storedCredential(reg)
	if err != nil {
		return signedOut, nil
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
		return signedOut, nil
	}
	if res.JSON200 == nil {
		return nil, registry.Problem("read your account", res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body)
	}
	u := res.JSON200
	v := &PublisherView{SignedIn: true, Namespaces: []PublisherNamespace{{Handle: u.Handle, Kind: "user", Admin: true}}}
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
		v.CanManage = carriesOwnerActions(scopes)
	}
	return v, nil
}

func carriesOwnerActions(scopes []string) bool {
	for _, s := range ownerActionScopes {
		if hasString(scopes, s) {
			return true
		}
	}
	return false
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
		Installs30d: a.Installs30d,
	}
	if a.RetiredAt != nil {
		v.RetiredAt = a.RetiredAt.UTC().Format(time.RFC3339)
	}
	if a.LastSubmittedAt != nil {
		v.LastSubmittedAt = a.LastSubmittedAt.UTC().Format(time.RFC3339)
	}
	if a.ForkedFrom != nil {
		v.ForkedFrom = &PublisherForkOrigin{Namespace: a.ForkedFrom.Namespace, Name: a.ForkedFrom.Name, Version: a.ForkedFrom.Version}
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

// publisherWords are what an action asks in the native dialog, before
// anything is sent, and what it says once it went through (the site's
// doneSentence). Each is given the QuickAgent's ns/name.
type publisherWords struct {
	title, question, done func(ref string) string
}

var publisherActs = map[publisherAct]publisherWords{
	actPrivate: {
		title: func(ref string) string { return "Make " + ref + " private?" },
		question: func(ref string) string {
			return ref + " leaves Browse and search, and only its readers can open it. Anyone who installed it keeps their copy, but can no longer update it."
		},
		done: func(ref string) string {
			return ref + " is private now. It left Browse and search, and only its readers can open it."
		},
	},
	actPublic: {
		title:    func(ref string) string { return "Make " + ref + " public?" },
		question: func(ref string) string { return "Anyone can find " + ref + " in Browse and install it." },
		done:     func(ref string) string { return ref + " is public now: anyone can find it in Browse and install it." },
	},
	actRetire: {
		title: func(ref string) string { return "Retire " + ref + "?" },
		question: func(ref string) string {
			return ref + " becomes private and takes no new versions. Anyone who installed it keeps their copy. Restore brings it back as it was."
		},
		done: func(ref string) string {
			return ref + " is retired. It is private, takes no new versions, and Restore brings it back as it was."
		},
	},
	actRestore: {
		title: func(ref string) string { return "Restore " + ref + "?" },
		question: func(ref string) string {
			return ref + " goes back to the visibility it had when it was retired, which may be public, and takes new versions again."
		},
		done: func(ref string) string { return ref + " is restored, with the visibility it had when it was retired." },
	},
	// Delete's question depends on whether the QuickAgent has a version
	// (deleteQuestion); this one is for a version history.
	actDelete: {
		title: func(ref string) string { return "Delete " + ref + "?" },
		question: func(ref string) string {
			return "Every version of " + ref + " is yanked, and it is gone for everyone, you included. Anyone who installed it keeps their copy, but can no longer update it. This cannot be undone, and the name stays held in its namespace for good."
		},
		done: func(ref string) string { return ref + " is deleted." },
	},
}

// deleteQuestionNoVersions is Delete's question for a QuickAgent that has
// no version, which the core deletes outright (plan §4.1).
func deleteQuestionNoVersions(ref string) string {
	return ref + " has no version, so it is deleted outright with its repository, and the name is free to use again. This cannot be undone."
}

const publisherNothingChanged = "Nothing changed:"

// publisherUnreachable is what an action says when the registry did not
// answer: a sentence for the person, so it ends in a full stop.
func publisherUnreachable() string {
	return publisherNothingChanged + " the registry did not answer. Try again in a moment."
}

// publisherMistyped is what a Delete whose typed name does not match says.
func publisherMistyped(ns, name string) string {
	return fmt.Sprintf("%s to delete %s/%s, type its name, %s, exactly.", publisherNothingChanged, ns, name, name)
}

// publisherRefusal is a refused action's answer.
type publisherRefusal struct {
	status       int
	slug, detail string
	// scopeless: the token carries neither agent:manage nor org:admin.
	scopeless bool
}

// sentence says the refusal in the site's words (refusalSentence): the
// problem's slug decides the sentence, and its detail is used only where
// no sentence here fits. A 403 to a token without agent:manage says to
// sign in again, which is the fix; the site's 403 names the rung, which a
// session can only lack.
func (r publisherRefusal) sentence(act publisherAct, ns, name string) string {
	ref := ns + "/" + name
	nothing := publisherNothingChanged
	switch {
	case r.slug == "account-unactivated" && r.detail != "":
		return r.detail
	case r.status == http.StatusUnauthorized:
		return nothing + " you are signed out. Sign in again under Settings, then retry."
	case r.status == http.StatusForbidden && r.scopeless:
		return nothing + " this sign-in was made before Agentry Desktop could retire, restore, delete or change who sees a QuickAgent. Sign out and sign in again under Settings, then retry."
	case r.status == http.StatusForbidden:
		return fmt.Sprintf("%s only %s itself, or an admin of the %s organization, can change who sees %s, or retire or delete it.", nothing, ns, ns, ref)
	case r.status == http.StatusNotFound:
		return fmt.Sprintf("%s %s is not there any more. It may have been deleted.", nothing, ref)
	case r.slug == "private-agent-cap":
		return fmt.Sprintf("%s %s already holds as many private QuickAgents as its subscription allows. Make one of them public, retire one, or upgrade, then try again.", nothing, ns)
	case r.slug == "agent-unscanned":
		// The site sends the person to the Versions page and Settings,
		// which are the site's pages.
		return fmt.Sprintf("%s only a version that passed the gate is ever public, and %s has versions admitted without a scan, or still admits them. Yank those versions and turn off admitting unscanned submissions in its settings on the site, then try again.", nothing, ref)
	case r.slug == "agent-retired":
		return fmt.Sprintf("%s %s is retired. Restore it first.", nothing, ref)
	case r.slug == "agent-not-retired":
		return fmt.Sprintf("%s %s is not retired, so there was nothing to restore.", nothing, ref)
	case r.slug == "confirm-name":
		return publisherMistyped(ns, name)
	case r.slug == "fork-stays-private":
		return fmt.Sprintf("%s %s is a fork, and a fork stays private for good.", nothing, ref)
	case act == actDelete && r.status == http.StatusConflict && r.detail != "":
		return nothing + " " + r.detail
	case r.status == http.StatusTooManyRequests:
		return nothing + " too many requests. Wait a minute, then try again."
	case r.status >= 500:
		return publisherUnreachable()
	case r.detail != "":
		return nothing + " " + r.detail
	}
	return nothing + " the registry refused it."
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

// publisherAnswer is the registry's answer to one owner action.
type publisherAnswer struct {
	status  int
	agent   *apiclient.Agent
	problem *apiclient.Problem
	body    []byte
}

// act asks the native question, then makes the one call.
func (p *PublisherAPI) act(reg, ns, name string, act publisherAct, confirm string) (*PublisherDone, error) {
	words, ok := publisherActs[act]
	if !ok {
		return nil, fmt.Errorf("%q is not an owner action", act)
	}
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
	if p.app == nil {
		return nil, errors.New("no confirmation dialog is available; refusing")
	}
	ctx := p.ctx()
	ref := ns + "/" + name
	question := words.question(ref)
	if act == actDelete && !hasVersions(ctx, client, ns, name) {
		question = deleteQuestionNoVersions(ref)
	}
	if err := p.app.askConsent(words.title(ref), question); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return &PublisherDone{Cancelled: true}, nil
		}
		return nil, err
	}

	ans, err := sendOwnerAction(ctx, client, act, ns, name, confirm)
	if err != nil {
		return nil, errors.New(publisherUnreachable())
	}
	if ans.status >= 200 && ans.status < 300 {
		done := &PublisherDone{Sentence: words.done(ref)}
		if ans.agent != nil {
			a := publisherAgent(*ans.agent)
			done.Agent = &a
		}
		return done, nil
	}
	r := publisherRefusal{status: ans.status}
	r.slug, r.detail = registry.ProblemSlug(ans.problem, ans.body)
	if ans.status == http.StatusForbidden {
		if scopes, ok := tokenScopes(ctx, url, token); ok {
			r.scopeless = !carriesOwnerActions(scopes)
		}
	}
	return nil, errors.New(r.sentence(act, ns, name))
}

// sendOwnerAction makes the action's one call to the core.
func sendOwnerAction(ctx context.Context, client *apiclient.ClientWithResponses, act publisherAct, ns, name, confirm string) (publisherAnswer, error) {
	switch act {
	case actPrivate, actPublic:
		vis := apiclient.UpdateAgentJSONBodyVisibility(act)
		res, err := client.UpdateAgentWithResponse(ctx, ns, name, apiclient.UpdateAgentJSONRequestBody{Visibility: &vis})
		if err != nil {
			return publisherAnswer{}, err
		}
		return publisherAnswer{res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body}, nil
	case actRetire:
		res, err := client.RetireAgentWithResponse(ctx, ns, name)
		if err != nil {
			return publisherAnswer{}, err
		}
		return publisherAnswer{res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body}, nil
	case actRestore:
		res, err := client.RestoreAgentWithResponse(ctx, ns, name)
		if err != nil {
			return publisherAnswer{}, err
		}
		return publisherAnswer{res.StatusCode(), res.JSON200, res.ApplicationproblemJSONDefault, res.Body}, nil
	default:
		res, err := client.DeleteAgentWithResponse(ctx, ns, name, &apiclient.DeleteAgentParams{Confirm: &confirm})
		if err != nil {
			return publisherAnswer{}, err
		}
		return publisherAnswer{res.StatusCode(), nil, res.ApplicationproblemJSONDefault, res.Body}, nil
	}
}

// hasVersions reports whether ns/name has a version row, for Delete's
// question. A read that does not answer counts as yes, the graver words.
func hasVersions(ctx context.Context, client *apiclient.ClientWithResponses, ns, name string) bool {
	res, err := client.ListVersionsWithResponse(ctx, ns, name)
	if err != nil || res.JSON200 == nil {
		return true
	}
	return len(*res.JSON200) > 0
}
