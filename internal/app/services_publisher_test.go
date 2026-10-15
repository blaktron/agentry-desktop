// The Publisher section against a registry that answers as the core does:
// who is signed in and in which organizations, what the token may do, the
// namespace's list with its filters, and the owner actions with their
// refusals.
package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// publisherFake is a registry for the reader "pat", a member of "acme"
// (admin), "crew" (member) and "later" (pending). Every field is the
// registry's answer.
type publisherFake struct {
	mu sync.Mutex
	// scopes are the token's, as the account's token list reports them.
	scopes []string
	// noTokenList answers the token list 500, so the scopes are unknown.
	noTokenList bool
	// status and problem answer the next owner action; zero is success.
	status  int
	problem string
	detail  string
	// calls records each owner action and list read: method, path, query,
	// body.
	calls []string
	// versionless lists no version for any QuickAgent.
	versionless bool
}

func (f *publisherFake) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{
			"id": "00000000-0000-0000-0000-0000000000a1", "handle": "pat", "displayName": "Pat", "totpEnabled": false,
			"orgs": []map[string]any{
				{"handle": "acme", "role": "admin", "state": "active"},
				{"handle": "crew", "role": "member", "state": "active"},
				{"handle": "later", "role": "member", "state": "pending"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/user/tokens", func(w http.ResponseWriter, _ *http.Request) {
		if f.noTokenList {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hostedJSON(w, http.StatusOK, []map[string]any{{
			"id": "00000000-0000-0000-0000-0000000000c1", "name": "desktop", "prefix": hostedTestToken[:patDisplayPrefixLen],
			"scopes": f.scopes, "createdAt": "2026-10-01T00:00:00Z", "expiresAt": "2026-11-01T00:00:00Z",
		}})
	})
	mux.HandleFunc("GET /api/v1/agents/{ns}/{name}/versions", func(w http.ResponseWriter, _ *http.Request) {
		if f.versionless {
			hostedJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		hostedJSON(w, http.StatusOK, []map[string]any{{
			"id": "00000000-0000-0000-0000-0000000000d1", "version": "1.0.0", "channel": "stable", "status": "published",
			"createdAt": "2026-09-01T00:00:00Z", "source": "git-tag",
		}})
	})
	mux.HandleFunc("GET /api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, "")
		hostedJSON(w, http.StatusOK, []map[string]any{
			publisherAgentDoc("pat", "kept", "private", map[string]any{
				"retiredAt": "2026-10-02T10:00:00Z", "installs30d": 3, "lastSubmittedAt": "2026-09-30T08:00:00Z",
			}),
			publisherAgentDoc("pat", "copy", "private", map[string]any{
				"forkedFrom": map[string]any{"namespace": "demo1", "name": "incident-summarizer", "version": "1.0.0", "available": true, "createdAt": "2026-10-06T00:00:00Z"},
				"forkState":  "ready",
			}),
		})
	})
	act := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.record(r, string(body))
		f.mu.Lock()
		status, slug, detail := f.status, f.problem, f.detail
		f.mu.Unlock()
		if status != 0 {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://agentry.dev/problems/" + slug, "title": "refused", "status": status, "detail": detail})
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		vis := "private"
		if strings.Contains(string(body), `"public"`) || strings.HasSuffix(r.URL.Path, "/restore") {
			vis = "public"
		}
		extra := map[string]any{}
		if strings.HasSuffix(r.URL.Path, "/retire") {
			extra["retiredAt"] = "2026-10-08T12:00:00Z"
		}
		hostedJSON(w, http.StatusOK, publisherAgentDoc(r.PathValue("ns"), r.PathValue("name"), vis, extra))
	}
	mux.HandleFunc("PATCH /api/v1/agents/{ns}/{name}", act)
	mux.HandleFunc("DELETE /api/v1/agents/{ns}/{name}", act)
	mux.HandleFunc("POST /api/v1/agents/{ns}/{name}/retire", act)
	mux.HandleFunc("POST /api/v1/agents/{ns}/{name}/restore", act)
	mux.HandleFunc("POST /api/v1/agents/{ns}/{name}/fork", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.record(r, string(body))
		f.mu.Lock()
		status, slug, detail := f.status, f.problem, f.detail
		f.mu.Unlock()
		if status != 0 {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://agentry.dev/problems/" + slug, "title": "refused", "status": status, "detail": detail})
			return
		}
		var req struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Name == "" {
			req.Name = r.PathValue("name")
		}
		hostedJSON(w, http.StatusAccepted, publisherAgentDoc(req.Namespace, req.Name, "private", map[string]any{
			"forkState":  "copying",
			"forkedFrom": map[string]any{"namespace": r.PathValue("ns"), "name": r.PathValue("name"), "version": "1.0.0", "available": true, "createdAt": "2026-10-08T00:00:00Z"},
		}))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *publisherFake) record(r *http.Request, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	line := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		line += "?" + r.URL.RawQuery
	}
	if body != "" {
		line += " " + strings.TrimSpace(body)
	}
	f.calls = append(f.calls, line)
}

func (f *publisherFake) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func publisherAgentDoc(ns, name, vis string, extra map[string]any) map[string]any {
	d := map[string]any{
		"id": "00000000-0000-0000-0000-0000000000b1", "namespace": ns, "name": name, "summary": "Summarizes things.",
		"visibility": vis, "quarantineState": "none", "modes": []string{"autonomous"}, "builtin": false,
		"defaultBranch": "main", "gitHttpUrl": "https://example.test/" + ns + "/" + name + ".git",
		"createdAt": "2026-09-01T00:00:00Z",
	}
	for k, v := range extra {
		d[k] = v
	}
	return d
}

// newPublisherAPI is the binding with a native dialog that answers yes or
// no and records each question.
// The questions are recorded as "title | message".
func newPublisherAPI(answer bool) (*PublisherAPI, *[]string) {
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+" | "+message)
		return answer, nil
	}}
	return &PublisherAPI{app: app}, &asked
}

func TestPublisherOverview(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:read", "agent:publish", "agent:manage", "user:profile"}}
	reg := f.start(t)
	p, _ := newPublisherAPI(true)

	// Signed out is an answer: the section draws its Sign in notice.
	signInElsewhere(t)
	v, err := p.Overview(reg)
	if err != nil || v.SignedIn {
		t.Fatalf("signed out: %+v %v", v, err)
	}

	signIn(t, reg)
	v, err = p.Overview(reg)
	if err != nil {
		t.Fatal(err)
	}
	if !v.SignedIn || !v.CanManage || !v.ScopesKnown {
		t.Fatalf("overview: %+v", v)
	}
	want := []PublisherNamespace{{"pat", "user", true}, {"acme", "org", true}, {"crew", "org", false}}
	if len(v.Namespaces) != len(want) {
		t.Fatalf("namespaces %+v, want %+v (a pending membership holds nothing)", v.Namespaces, want)
	}
	for i := range want {
		if v.Namespaces[i] != want[i] {
			t.Fatalf("namespace %d: %+v, want %+v", i, v.Namespaces[i], want[i])
		}
	}

	// A token minted before the grant carried agent:manage lacks it.
	f.scopes = []string{"agent:read", "agent:publish", "user:profile"}
	if v, _ = p.Overview(reg); v.CanManage || !v.ScopesKnown {
		t.Fatalf("an old sign-in reads as able to manage: %+v", v)
	}
	// A pasted token's org:admin admits the actions too.
	f.scopes = []string{"org:admin"}
	if v, _ = p.Overview(reg); !v.CanManage {
		t.Fatalf("org:admin reads as unable to manage: %+v", v)
	}
	// Unknown scopes are not a refusal: the registry decides.
	f.noTokenList = true
	if v, _ = p.Overview(reg); v.ScopesKnown || v.CanManage {
		t.Fatalf("unknown scopes: %+v", v)
	}
}

// signInElsewhere isolates the app's state with a token kept only for an
// address nothing answers on, so the registry under test is signed out.
func signInElsewhere(t *testing.T) {
	t.Helper()
	signIn(t, "http://127.0.0.1:1")
}

func TestPublisherListSendsTheCoresFilters(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:manage"}}
	reg := f.start(t)
	signIn(t, reg)
	p, _ := newPublisherAPI(true)

	rows, err := p.List(reg, "pat", PublisherQuery{Q: "  incident 50%_ ", State: "retired", Forks: "only", Sort: "installs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].RetiredAt != "2026-10-02T10:00:00Z" || rows[0].Installs30d == nil || *rows[0].Installs30d != 3 {
		t.Fatalf("rows: %+v", rows)
	}
	if fo := rows[1].ForkedFrom; fo == nil || fo.Namespace != "demo1" || fo.Version != "1.0.0" || rows[1].ForkState != "ready" {
		t.Fatalf("fork row: %+v", rows[1])
	}
	// Anything unknown is no filter, and the sort is newest.
	if _, err := p.List(reg, "pat", PublisherQuery{State: "archived", Forks: "maybe", Sort: "stars"}); err != nil {
		t.Fatal(err)
	}
	calls := f.sent()
	if len(calls) != 2 {
		t.Fatalf("calls: %v", calls)
	}
	for _, want := range []string{"namespace=pat", "q=incident+50%25_", "state=retired", "forks=only", "sort=installs"} {
		if !strings.Contains(calls[0], want) {
			t.Fatalf("list query %q lacks %q", calls[0], want)
		}
	}
	if calls[1] != "GET /api/v1/agents?namespace=pat&sort=newest" {
		t.Fatalf("unknown filters were sent: %q", calls[1])
	}
	if _, err := p.List(reg, "../x", PublisherQuery{}); err == nil {
		t.Fatal("a namespace outside the pattern was listed")
	}
}

func TestPublisherActionsAreAskedFirstAndCallTheCore(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:manage"}}
	reg := f.start(t)
	signIn(t, reg)

	// No in the native dialog sends nothing.
	no, asked := newPublisherAPI(false)
	d, err := no.Retire(reg, "pat", "kept")
	if err != nil || !d.Cancelled || len(*asked) != 1 || len(f.sent()) != 0 {
		t.Fatalf("a No: %+v %v asked %v sent %v", d, err, *asked, f.sent())
	}

	p, asked := newPublisherAPI(true)
	cases := []struct {
		run      func() (*PublisherDone, error)
		call     string
		sentence string
		vis      string
	}{
		{func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "kept", "private") }, `PATCH /api/v1/agents/pat/kept {"visibility":"private"}`, "pat/kept is private now.", "private"},
		{func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "kept", "public") }, `PATCH /api/v1/agents/pat/kept {"visibility":"public"}`, "pat/kept is public now", "public"},
		{func() (*PublisherDone, error) { return p.Retire(reg, "acme", "tool") }, "POST /api/v1/agents/acme/tool/retire", "acme/tool is retired.", "private"},
		{func() (*PublisherDone, error) { return p.Restore(reg, "acme", "tool") }, "POST /api/v1/agents/acme/tool/restore", "acme/tool is restored", "public"},
		{func() (*PublisherDone, error) { return p.Delete(reg, "pat", "kept", " kept ") }, "DELETE /api/v1/agents/pat/kept?confirm=kept", "pat/kept is deleted.", ""},
	}
	for i, c := range cases {
		before := len(f.sent())
		d, err := c.run()
		if err != nil || d.Cancelled || !strings.HasPrefix(d.Sentence, c.sentence) {
			t.Fatalf("case %d: %+v %v", i, d, err)
		}
		if got := f.sent(); len(got) != before+1 || got[before] != c.call {
			t.Fatalf("case %d sent %v, want %q", i, got[before:], c.call)
		}
		if c.vis == "" && d.Agent != nil || c.vis != "" && (d.Agent == nil || d.Agent.Visibility != c.vis) {
			t.Fatalf("case %d agent: %+v", i, d.Agent)
		}
	}
	if len(*asked) != len(cases) {
		t.Fatalf("asked %d native questions for %d actions: %v", len(*asked), len(cases), *asked)
	}
	if !strings.HasPrefix((*asked)[4], "Delete pat/kept? | Every version of pat/kept is yanked") || !strings.Contains((*asked)[4], "the name stays held") {
		t.Fatalf("delete's question for a QuickAgent with a version: %q", (*asked)[4])
	}
	// One with no version is deleted outright, and its name is free again.
	f.versionless = true
	if _, err := p.Delete(reg, "pat", "draft", "draft"); err != nil {
		t.Fatal(err)
	}
	if q := (*asked)[len(*asked)-1]; !strings.Contains(q, "pat/draft has no version, so it is deleted outright") || strings.Contains(q, "held") {
		t.Fatalf("delete's question for a QuickAgent with no version: %q", q)
	}
	f.versionless = false

	// A mistyped name costs no dialog and no call.
	before, beforeAsked := len(f.sent()), len(*asked)
	if _, err := p.Delete(reg, "pat", "kept", "kep"); err == nil || !strings.Contains(err.Error(), "type its name, kept, exactly") {
		t.Fatalf("mistyped delete: %v", err)
	}
	if len(f.sent()) != before || len(*asked) != beforeAsked {
		t.Fatal("a mistyped delete reached the dialog or the registry")
	}
	// A name outside the patterns is refused before either.
	if _, err := p.Retire(reg, "pat", "kept/../x"); err == nil || len(f.sent()) != before {
		t.Fatalf("a path-like name: %v", err)
	}
	if _, err := p.SetVisibility(reg, "pat", "kept", "internal"); err == nil {
		t.Fatal("a visibility the core does not know was accepted")
	}
}

func TestPublisherRefusalsInTheSitesWords(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:manage"}}
	reg := f.start(t)
	signIn(t, reg)
	p, _ := newPublisherAPI(true)
	cases := []struct {
		status       int
		slug, detail string
		scopes       []string
		run          func() (*PublisherDone, error)
		want         string
	}{
		{422, "private-agent-cap", "", nil, func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "kept", "private") }, "pat already holds as many private QuickAgents as its subscription allows"},
		{409, "agent-unscanned", "", nil, func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "kept", "public") }, "only a version that passed the gate is ever public"},
		{409, "fork-stays-private", "", nil, func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "copy", "public") }, "pat/copy is a fork, and a fork stays private for good."},
		{409, "agent-not-retired", "", nil, func() (*PublisherDone, error) { return p.Restore(reg, "pat", "kept") }, "pat/kept is not retired"},
		{409, "conflict", "This QuickAgent carries trust & safety records, so it cannot be deleted.", nil, func() (*PublisherDone, error) { return p.Delete(reg, "pat", "kept", "kept") }, "Nothing changed: This QuickAgent carries trust & safety records"},
		{404, "not-found", "", nil, func() (*PublisherDone, error) { return p.Retire(reg, "pat", "gone") }, "pat/gone is not there any more"},
		{403, "forbidden", "", nil, func() (*PublisherDone, error) { return p.Retire(reg, "crew", "tool") }, "only crew itself, or an admin of the crew organization"},
		{403, "forbidden", "", []string{"agent:read", "agent:publish", "user:profile"}, func() (*PublisherDone, error) { return p.Retire(reg, "pat", "kept") }, "this sign-in was made before Agentry Desktop could retire"},
		{503, "unavailable", "", nil, func() (*PublisherDone, error) { return p.Retire(reg, "pat", "kept") }, "the registry did not answer"},
	}
	for i, c := range cases {
		f.mu.Lock()
		f.status, f.problem, f.detail = c.status, c.slug, c.detail
		f.scopes = []string{"agent:manage"}
		if c.scopes != nil {
			f.scopes = c.scopes
		}
		f.mu.Unlock()
		d, err := c.run()
		if err == nil || d != nil {
			t.Fatalf("case %d: refusal answered %+v", i, d)
		}
		if !strings.HasPrefix(err.Error(), "Nothing changed:") || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("case %d: %q, want it to say %q", i, err, c.want)
		}
		if strings.Contains(err.Error(), "HTTP") || strings.Contains(err.Error(), hostedTestToken) {
			t.Fatalf("case %d says a status code or the token: %q", i, err)
		}
	}
}

// The token goes only where Settings would send it: a plain-http registry at
// a real host is refused before any dialog or request, and the section reads
// as signed out there.
func TestPublisherSendsNoTokenOverPlainHTTPToARealHost(t *testing.T) {
	signIn(t, "http://dev.secureagentry.net")
	p, asked := newPublisherAPI(true)
	const reg = "http://dev.secureagentry.net"
	if v, err := p.Overview(reg); err != nil || v.SignedIn {
		t.Fatalf("overview over plain http: %+v %v", v, err)
	}
	if _, err := p.List(reg, "pat", PublisherQuery{}); err == nil {
		t.Fatal("the list was read over plain http")
	}
	for _, run := range []func() (*PublisherDone, error){
		func() (*PublisherDone, error) { return p.SetVisibility(reg, "pat", "kept", "private") },
		func() (*PublisherDone, error) { return p.Retire(reg, "pat", "kept") },
		func() (*PublisherDone, error) { return p.Restore(reg, "pat", "kept") },
		func() (*PublisherDone, error) { return p.Delete(reg, "pat", "kept", "kept") },
	} {
		if d, err := run(); err == nil {
			t.Fatalf("an owner action went over plain http: %+v", d)
		}
	}
	if len(*asked) != 0 {
		t.Fatalf("a dialog was raised for a refused address: %v", *asked)
	}
}

// Fork against the same registry: the targets are pat and the organizations
// pat publishes for, a fork is asked first and sent as the core takes it,
// and a refusal is said in the site's words, asking for another name only
// when the name was the trouble.
func TestPublisherForkTargetsAndFork(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:manage"}}
	reg := f.start(t)
	signIn(t, reg)
	p, asked := newPublisherAPI(true)

	targets, err := p.ForkTargets(reg)
	if err != nil || strings.Join(targets, ",") != "pat,acme,crew" {
		t.Fatalf("fork targets %v %v, want pat,acme,crew (a pending membership publishes nothing)", targets, err)
	}

	v, err := p.Fork(reg, "demo1", "incident-summarizer", "acme", "")
	if err != nil || v.Cancelled || v.Refusal != "" || v.Namespace != "acme" || v.Name != "incident-summarizer" ||
		v.Sentence != "Forked demo1/incident-summarizer into acme/incident-summarizer." {
		t.Fatalf("fork: %+v %v", v, err)
	}
	if got := f.sent(); len(got) != 1 || got[0] != `POST /api/v1/agents/demo1/incident-summarizer/fork {"namespace":"acme"}` {
		t.Fatalf("fork sent %v", got)
	}
	if len(*asked) != 1 || !strings.HasPrefix((*asked)[0], "Fork demo1/incident-summarizer into acme/incident-summarizer? | A fork stays private for good.") ||
		!strings.Contains((*asked)[0], "Runs on your own computer pay nobody.") {
		t.Fatalf("fork's question: %v", *asked)
	}

	// Another name is sent as name.
	if v, err = p.Fork(reg, "demo1", "incident-summarizer", "pat", "my-summarizer"); err != nil || v.Name != "my-summarizer" {
		t.Fatalf("fork under another name: %+v %v", v, err)
	}
	if got := f.sent(); got[len(got)-1] != `POST /api/v1/agents/demo1/incident-summarizer/fork {"name":"my-summarizer","namespace":"pat"}` {
		t.Fatalf("fork under another name sent %v", got[len(got)-1])
	}

	// A No sends nothing.
	no, _ := newPublisherAPI(false)
	before := len(f.sent())
	if v, err = no.Fork(reg, "demo1", "incident-summarizer", "pat", ""); err != nil || !v.Cancelled || len(f.sent()) != before {
		t.Fatalf("a No: %+v %v", v, err)
	}
	// A name or a namespace outside the patterns is refused before the
	// dialog and before any request.
	beforeAsked := len(*asked)
	for _, c := range [][3]string{{"demo1", "incident-summarizer", "../x"}, {"demo1", "a/b", "pat"}} {
		if _, err := p.Fork(reg, c[0], c[1], c[2], ""); err == nil {
			t.Fatalf("fork of %v was not refused", c)
		}
	}
	if len(*asked) != beforeAsked || len(f.sent()) != before {
		t.Fatalf("a refused fork reached the dialog or the registry: asked %v", *asked)
	}
	if v, _ = p.Fork(reg, "demo1", "incident-summarizer", "pat", "Bad Name"); !v.AskName || !strings.HasPrefix(v.Refusal, "Not forked:") || len(f.sent()) != before {
		t.Fatalf("a bad new name: %+v", v)
	}
}

func TestPublisherForkRefusalsInTheSitesWords(t *testing.T) {
	f := &publisherFake{scopes: []string{"agent:manage"}}
	reg := f.start(t)
	signIn(t, reg)
	p, _ := newPublisherAPI(true)
	cases := []struct {
		status       int
		slug, detail string
		askName      bool
		want         string
	}{
		{409, "name-taken", "", true, "pat already has a QuickAgent called incident-summarizer. Choose another name for the fork."},
		{409, "name-held", "", true, "incident-summarizer belonged to a QuickAgent deleted from pat, and that name is never used again."},
		{409, "fork-source", "", false, "only a public QuickAgent that is not quarantined or blocked can be forked, and demo1/incident-summarizer is not one now."},
		{409, "nothing-to-fork", "", false, "demo1/incident-summarizer has no published version yet"},
		{422, "private-agent-cap", "", false, "pat already holds as many private QuickAgents as its subscription allows, and a fork is private."},
		{422, "validation", "namespace agent quota reached", false, "pat already holds as many QuickAgents as it may."},
		{403, "forbidden", "", false, "you cannot create QuickAgents in pat."},
		{502, "bad-gateway", "", false, "the registry did not answer"},
	}
	for i, c := range cases {
		f.mu.Lock()
		f.status, f.problem, f.detail = c.status, c.slug, c.detail
		f.mu.Unlock()
		v, err := p.Fork(reg, "demo1", "incident-summarizer", "pat", "")
		if err != nil || v.AskName != c.askName || !strings.HasPrefix(v.Refusal, "Not forked:") || !strings.Contains(v.Refusal, c.want) {
			t.Fatalf("case %d: %+v %v, want it to say %q", i, v, err, c.want)
		}
		if strings.Contains(v.Refusal, "HTTP") || strings.Contains(v.Refusal, hostedTestToken) {
			t.Fatalf("case %d says a status code or the token: %q", i, v.Refusal)
		}
	}
}

func TestPublisherForkSendsNoTokenOverPlainHTTPToARealHost(t *testing.T) {
	signIn(t, "http://dev.secureagentry.net")
	p, asked := newPublisherAPI(true)
	if targets, err := p.ForkTargets("http://dev.secureagentry.net"); err != nil || len(targets) != 0 {
		t.Fatalf("fork targets over plain http: %v %v", targets, err)
	}
	if v, err := p.Fork("http://dev.secureagentry.net", "demo1", "incident-summarizer", "pat", ""); err == nil {
		t.Fatalf("a fork went over plain http: %+v", v)
	}
	if len(*asked) != 0 {
		t.Fatalf("a dialog was raised for a refused address: %v", *asked)
	}
}
