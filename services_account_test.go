package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The account on Settings (agentry-desktop#74): the credential's user;
// the subscription only where the registry mounts the billing extension;
// Upgrade opens the site's subscription page for the registry.
func TestAccountShowsTheSubscriptionWhereThereIsOne(t *testing.T) {
	billing := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if billing {
			_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{}}`))
	})
	userDoc := `{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z"}`
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userDoc))
	})
	var accountDoc string
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agy_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(accountDoc))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	accountDoc = `{"kind":"user","tier":"standard","balance":40,"allowance":50,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":10,"used":4},"upgrade":{"available":true,"manage":false,"url":"` + srv.URL + `/settings/subscription"}}`

	sub, err := fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err != nil || sub != nil {
		t.Fatalf("a core registry: sub=%+v err=%v; want none", sub, err)
	}
	billing = true
	sub, err = fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Tier != "standard" || sub.Balance != 40 || sub.Allowance != 50 || sub.PrivateAgents.Max != 10 || sub.MemberOf != "" {
		t.Fatalf("subscription = %+v", sub)
	}
	// The destination is the registry's answer, never a path this app
	// builds (agentry-enterprise#41, D28).
	if sub.Upgrade.URL != srv.URL+"/settings/subscription" || !sub.Upgrade.Available {
		t.Fatalf("upgrade = %+v", sub.Upgrade)
	}
	if _, err := fetchSubscription(context.Background(), srv.URL, "agy_wrong"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a refused read must be an error, got %v", err)
	}

	ctx := context.Background()
	got, err := upgradeDestination(ctx, srv.URL, "agy_test")
	if err != nil || got != srv.URL+"/settings/subscription" {
		t.Fatalf("a person is sent to %q (%v)", got, err)
	}

	// A member of an organization: the account answered is the
	// organization's, so the tier is its tier, the balance its pool's,
	// and the page is its console — not this person's own (D22).
	userDoc = `{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z","orgs":[{"handle":"acme","role":"admin","state":"active"}]}`
	accountDoc = `{"kind":"org","tier":"business","standing":"active","memberOf":"acme","seats":{"used":3,"max":10},"balance":3960,"allowance":5000,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":0,"used":4},"upgrade":{"available":false,"manage":false,"url":"` + srv.URL + `/orgs/acme/admin"}}`
	sub, err = fetchSubscription(ctx, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if sub.MemberOf != "acme" || sub.Tier != "business" || sub.Standing != "active" || sub.Seats == nil || sub.Seats.Max != 10 || sub.Balance != 3960 {
		t.Fatalf("a member's subscription = %+v", sub)
	}
	got, err = upgradeDestination(ctx, srv.URL, "agy_test")
	if err != nil || got != srv.URL+"/orgs/acme/admin" {
		t.Fatalf("a member is sent to %q (%v)", got, err)
	}
	// Nothing can be bought or managed there, and the page is still worth
	// opening: it is where the standing and the seats are shown. The URL
	// says where, the booleans say whether.
	if sub.Upgrade.Available || sub.Upgrade.Manage {
		t.Fatalf("a fully paid organization offered a purchase: %+v", sub.Upgrade)
	}

	app := &App{ctx: ctx}
	u, err := whoami(app, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Orgs) != 1 || u.Orgs[0].Handle != "acme" || u.Orgs[0].Role != "admin" || u.Orgs[0].State != "active" || u.PendingOrg != nil {
		t.Fatalf("memberships = %+v pending=%+v", u.Orgs, u.PendingOrg)
	}

	// A pending fold-in: the membership is reported but not in force, the
	// door it came through is named, and the subscription answered is
	// still their own, because that is what they spend from until an
	// admin approves them.
	userDoc = `{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z","orgs":[{"handle":"acme","role":"member","state":"pending"}],"pendingOrg":{"handle":"acme","displayName":"Acme","via":"domain"}}`
	accountDoc = `{"kind":"user","tier":"standard","balance":40,"allowance":50,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":10,"used":4},"upgrade":{"available":true,"manage":false,"url":"` + srv.URL + `/settings/subscription"}}`
	u, err = whoami(app, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Orgs) != 1 || u.Orgs[0].State != "pending" || u.PendingOrg == nil || u.PendingOrg.Via != "domain" || u.PendingOrg.DisplayName != "Acme" {
		t.Fatalf("a pending fold-in = %+v pending=%+v", u.Orgs, u.PendingOrg)
	}
	sub, err = fetchSubscription(ctx, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if sub.MemberOf != "" || sub.Tier != "standard" {
		t.Fatalf("a pending fold-in was given the organization's account: %+v", sub)
	}
	got, err = upgradeDestination(ctx, srv.URL, "agy_test")
	if err != nil || got != srv.URL+"/settings/subscription" {
		t.Fatalf("a pending fold-in is sent to %q (%v)", got, err)
	}

	// End to end, through the method the front end actually calls: it
	// resolves the destination and opens it, and the front end never names
	// one. A member is back on the organization's account for this.
	userDoc = `{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z","orgs":[{"handle":"acme","role":"admin","state":"active"}]}`
	accountDoc = `{"kind":"org","tier":"business","standing":"active","memberOf":"acme","seats":{"used":3,"max":10},"balance":3960,"allowance":5000,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":0,"used":4},"upgrade":{"available":false,"manage":true,"url":"` + srv.URL + `/orgs/acme/admin"}}`
	opened := make(chan string, 1)
	sapi := &SettingsAPI{app: app, open: func(u string) error { opened <- u; return nil }}
	t.Setenv(configdir.EnvVar, t.TempDir())
	// The keychain in memory: StoreToken writes it first, and the real one
	// would keep agy_test under 127.0.0.1:<port> after the test, where a
	// later test's server on the same port reads it (agentry-desktop#497).
	keyring.MockInit()
	if _, err := settings.StoreToken(srv.URL, "agy_test"); err != nil {
		t.Fatal(err)
	}
	if err := sapi.OpenUpgrade(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}
	if got := <-opened; got != srv.URL+"/orgs/acme/admin" {
		t.Fatalf("OpenUpgrade opened %q", got)
	}
}

// A registry that sells nothing still reports the memberships, so the app
// can say which organization a person belongs to and in what role without
// a word about tiers (the issue: "a core registry shows the organization
// and role without tier words").
func TestACoreRegistryStillShowsTheOrganization(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{}}`))
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z","orgs":[{"handle":"acme","role":"admin","state":"active"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	app := &App{ctx: context.Background()}

	u, err := whoami(app, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Orgs) != 1 || u.Orgs[0].Handle != "acme" || u.Orgs[0].Role != "admin" {
		t.Fatalf("memberships = %+v", u.Orgs)
	}
	// No billing extension, so no subscription and nothing to say about a
	// tier; the organization above still stands.
	sub, err := fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err != nil || sub != nil {
		t.Fatalf("a core registry: sub=%+v err=%v; want none", sub, err)
	}
}

// A registry names the page, so the app checks it before opening it: a
// destination off the registry's own origin is not followed, and a
// registry that names none is an error rather than a silent nothing.
func TestTheAppOpensOnlyTheRegistrysOwnPage(t *testing.T) {
	var accountDoc string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing"]}`))
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(accountDoc))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	for _, tc := range []struct {
		name, url, wantErr string
	}{
		{"another site entirely", "https://evil.test/settings/subscription", "another site"},
		{"the same host on another scheme", strings.Replace(srv.URL, "http://", "https://", 1) + "/x", "another site"},
		{"no page at all", "", "names no page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accountDoc = `{"kind":"user","tier":"standard","balance":40,"allowance":50,"submissionCost":10,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","tierSince":"2026-09-11T00:00:00Z","privateAgents":{"max":10,"used":4},"upgrade":{"available":true,"manage":false,"url":"` + tc.url + `"}}`
			got, err := upgradeDestination(ctx, srv.URL, "agy_test")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("destination %q gave %q, %v; want an error naming %q", tc.url, got, err, tc.wantErr)
			}
		})
	}

	// Without a credential there is nothing to read and nothing to open.
	if _, err := upgradeDestination(ctx, srv.URL, ""); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("no credential gave %v", err)
	}
}

// Top up credits (agentry-desktop#511, plan credit-top-ups.md D7): the
// view carries the registry's packs, and Buy opens the site's card only
// where packs are offered (or a suspension sends the person to pay), never
// for a member and never off the registry's origin.
func TestTopUpOpensOnlyTheRegistrysOwnCard(t *testing.T) {
	var accountDoc string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing"]}`))
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(accountDoc))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()
	doc := func(extra string) string {
		return `{"kind":"user","tier":"pro","balance":80,"allowance":1000,"submissionCost":1,"periodStart":"2026-10-01T00:00:00Z","periodEnd":"2026-11-01T00:00:00Z","tierSince":"2026-09-01T00:00:00Z","privateAgents":{"max":0,"used":0},"upgrade":{"available":false,"manage":true}` + extra + `}`
	}
	packs := `"packs":[{"id":"1250","credits":1250,"baseCredits":1000,"amount":999,"currency":"usd"},{"id":"5000","credits":5000,"baseCredits":3500,"amount":3499,"currency":"usd"}]`

	accountDoc = doc(`,"bought":300,"topUp":{"available":true,"url":"` + srv.URL + `/settings/profile#top-up","processing":false,` + packs + `}`)
	sub, err := fetchSubscription(ctx, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Bought != 300 || sub.TopUp == nil || !sub.TopUp.Available || len(sub.TopUp.Packs) != 2 ||
		sub.TopUp.Packs[0] != (TopUpPack{ID: "1250", Credits: 1250, BaseCredits: 1000, Amount: 999, Currency: "usd"}) {
		t.Fatalf("the packs read = %+v bought=%d", sub.TopUp, sub.Bought)
	}
	got, err := topUpDestination(ctx, srv.URL, "agy_test")
	if err != nil || got != srv.URL+"/settings/profile#top-up" {
		t.Fatalf("a Pro person is sent to %q (%v)", got, err)
	}

	// A suspended account is sent to the card to pay what it owes.
	accountDoc = doc(`,"suspended":{"since":"2026-10-02T00:00:00Z","owed":40},"topUp":{"available":false,"url":"` + srv.URL + `/settings/profile#top-up","processing":false,"packs":[]}`)
	if got, err = topUpDestination(ctx, srv.URL, "agy_test"); err != nil || got != srv.URL+"/settings/profile#top-up" {
		t.Fatalf("a suspended account is sent to %q (%v)", got, err)
	}
	if sub, err = fetchSubscription(ctx, srv.URL, "agy_test"); err != nil || sub == nil || sub.Suspended == nil || sub.Suspended.Owed != 40 {
		t.Fatalf("the suspension read = %+v (%v)", sub, err)
	}

	for _, tc := range []struct{ name, doc, wantErr string }{
		{"another site entirely", doc(`,"topUp":{"available":true,"url":"https://evil.test/settings/profile#top-up","processing":false,` + packs + `}`), "another site"},
		{"a script URL", doc(`,"topUp":{"available":true,"url":"javascript:alert(1)","processing":false,` + packs + `}`), "another site"},
		{"a file URL", doc(`,"topUp":{"available":true,"url":"file:///etc/passwd","processing":false,` + packs + `}`), "another site"},
		{"the registry as userinfo before another host", doc(`,"topUp":{"available":true,"url":"http://` + strings.TrimPrefix(srv.URL, "http://") + `@evil.test/x","processing":false,` + packs + `}`), "another site"},
		{"a scheme-relative URL", doc(`,"topUp":{"available":true,"url":"//evil.test/settings/profile#top-up","processing":false,` + packs + `}`), "another site"},
		{"the same host on another scheme", doc(`,"topUp":{"available":true,"url":"` + strings.Replace(srv.URL, "http://", "https://", 1) + `/settings/profile#top-up","processing":false,` + packs + `}`), "another site"},
		{"no page at all", doc(`,"topUp":{"available":true,"processing":false,` + packs + `}`), "names no page"},
		{"no packs offered", doc(`,"topUp":{"available":false,"url":"` + srv.URL + `/settings/profile#top-up","processing":false,"packs":[]}`), "no credit packs"},
		{"a registry from before packs", doc(``), "no credit packs"},
		{"a member's view", `{"kind":"org","tier":"business","memberOf":"acme","balance":3960,"allowance":5000,"submissionCost":1,"periodStart":"2026-09-11T00:00:00Z","periodEnd":"2026-10-11T00:00:00Z","privateAgents":{"max":0,"used":4},"upgrade":{"available":false,"manage":false},"topUp":{"available":true,"url":"` + srv.URL + `/settings/profile#top-up","processing":false,` + packs + `}}`, "no credit packs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accountDoc = tc.doc
			got, err := topUpDestination(ctx, srv.URL, "agy_test")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("gave %q, %v; want an error naming %q", got, err, tc.wantErr)
			}
		})
	}
	if _, err := topUpDestination(ctx, srv.URL, ""); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("no credential gave %v", err)
	}

	// End to end, through the method the front end calls with a registry
	// and never a page: it opens the card the registry named, and opens
	// nothing for a page off its origin.
	opened := make(chan string, 1)
	sapi := &SettingsAPI{app: &App{ctx: ctx}, open: func(u string) error { opened <- u; return nil }}
	t.Setenv(configdir.EnvVar, t.TempDir())
	keyring.MockInit()
	if _, err := settings.StoreToken(srv.URL, "agy_test"); err != nil {
		t.Fatal(err)
	}
	accountDoc = doc(`,"topUp":{"available":true,"url":"` + srv.URL + `/settings/profile#top-up","processing":false,` + packs + `}`)
	if err := sapi.OpenTopUp(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}
	if got := <-opened; got != srv.URL+"/settings/profile#top-up" {
		t.Fatalf("OpenTopUp opened %q", got)
	}
	accountDoc = doc(`,"topUp":{"available":true,"url":"https://evil.test/settings/profile#top-up","processing":false,` + packs + `}`)
	if err := sapi.OpenTopUp(srv.URL); err == nil {
		t.Fatal("OpenTopUp opened a page on another site")
	}
	select {
	case u := <-opened:
		t.Fatalf("OpenTopUp opened %q off the registry's origin", u)
	default:
	}
}

// The hand-rolled /x/ calls set the token themselves, so they hold
// agentry-desktop#856's rule through the shared client: a registry whose
// billing route answers 307 to another server gets the refusal, and the
// other server sees nothing.
func TestTheSubscriptionReadNeverFollowsARedirect(t *testing.T) {
	var elsewhere int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"user","tier":"standard"}`))
	}))
	defer other.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{},"extensions":["billing"]}`))
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sub, err := fetchSubscription(context.Background(), srv.URL, "agy_test")
	if err == nil || sub != nil || elsewhere != 0 {
		t.Fatalf("sub=%+v err=%v, %d requests elsewhere; want the redirect refused and none", sub, err, elsewhere)
	}
}
