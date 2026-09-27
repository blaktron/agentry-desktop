package main

// An organization's shared pack in the Knowledge panel live
// (agentry-desktop#276, Knowledge M8): the bound methods driven as the page
// drives them, against a real registry, as a member of an organization
// that shares a pack. Skipped unless these are set:
//
//	AGENTRY_LIVE_SHARED_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_SHARED_TOKEN=<the member's PAT, knowledge:read and knowledge:write> \
//	AGENTRY_LIVE_SHARED_ADMIN_TOKEN=<an admin's PAT with org:admin, to remove the member> \
//	AGENTRY_LIVE_SHARED_ORG=<the organization> AGENTRY_LIVE_SHARED_MEMBER=<the member's handle> \
//	AGENTRY_LIVE_SHARED_PACK=<a ready shared pack, not marked exportable> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home> \
//	go test -run TestLiveSharedKnowledge -v .
//
// It lists the organization's pack, downloads it, previews it twice, finds
// Export refused, has the admin remove the member, and finds the kept copy
// no longer opens; the keychain is an in-memory one that must record no
// write for the pack's key, and the kept copy is deleted however the test
// ends. The removal is not undone, so the member is a throwaway account.
// It refuses production (secureagentry.net and anything but a dev or local
// registry) and a runner home that is not named: it must never touch a
// person's own store.

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

// recordingKeys is a keychain in memory that records every account set.
type recordingKeys struct {
	mu   sync.Mutex
	m    map[string]string
	sets []string
}

func (k *recordingKeys) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[a]; ok {
		return v, nil
	}
	return "", store.ErrNoEntry
}

func (k *recordingKeys) Set(a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = v
	k.sets = append(k.sets, a)
	return nil
}

func (k *recordingKeys) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, a)
	return nil
}

func TestLiveSharedKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SHARED_REGISTRY"), "/")
	token, admin := os.Getenv("AGENTRY_LIVE_SHARED_TOKEN"), os.Getenv("AGENTRY_LIVE_SHARED_ADMIN_TOKEN")
	org, member, pack := os.Getenv("AGENTRY_LIVE_SHARED_ORG"), os.Getenv("AGENTRY_LIVE_SHARED_MEMBER"), os.Getenv("AGENTRY_LIVE_SHARED_PACK")
	if reg == "" || token == "" || admin == "" || org == "" || member == "" || pack == "" {
		t.Skip("set the AGENTRY_LIVE_SHARED_* variables to run the live shared-knowledge check")
	}
	if u, err := url.Parse(reg); err != nil || !devOrLocal(u.Hostname()) {
		t.Fatalf("the live shared-knowledge check removes a member and runs against dev or a local registry only, not %s", reg)
	}
	if os.Getenv("AGENTRY_RUNNER_HOME") == "" {
		t.Fatal("name a scratch runner home in AGENTRY_RUNNER_HOME: this check must not keep a pack in a person's own store")
	}
	tokenFor := func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] }
	keys := &recordingKeys{m: map[string]string{}}
	env := knowledge.Env{Home: policy.Home, Keys: keys, TokenFor: tokenFor, HTTP: registry.HTTPClient}
	app := &App{
		ctx:     context.Background(),
		confirm: func(context.Context, string, string) (bool, error) { return true, nil },
		pickDirectory: func(context.Context, string) (string, error) {
			return t.TempDir(), nil
		},
	}
	k := &KnowledgeAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}, openFolder: func(string) error { return nil }}

	find := func() knowledge.Pack {
		t.Helper()
		l, err := k.List(reg)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range l.Packs {
			if p.Name == pack {
				return p
			}
		}
		t.Fatalf("the list does not offer %s: %+v (%s)", pack, l.Packs, l.Agentry)
		return knowledge.Pack{}
	}
	if p := find(); !p.Shared || p.Org != org || p.Local {
		t.Fatalf("the organization's pack is listed as %+v", p)
	}
	msg, err := k.DownloadShared(reg, org, pack)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Delete(context.Background(), reg, pack, false); err != nil {
			t.Errorf("delete the kept copy: %v", err)
		}
	})
	t.Log(msg)
	p := find()
	if !p.Local || p.OpensOffline || p.CanExport {
		t.Fatalf("the kept shared pack reads %+v", p)
	}
	for i := 0; i < 2; i++ {
		if _, err := k.Preview(reg, pack); err != nil {
			t.Fatalf("a member's preview %d: %v", i, err)
		}
	}
	if _, err := k.Export(reg, pack); err == nil || !strings.Contains(err.Error(), "admins") {
		t.Fatalf("a member's export of an unmarked pack: %v", err)
	}

	// The admin removes the member.
	req, _ := http.NewRequest(http.MethodDelete, reg+"/api/v1/orgs/"+org+"/members/"+member, nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	res, err := registry.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("the admin's removal of %s: %d", member, res.StatusCode)
	}
	if _, err := k.Preview(reg, pack); err == nil {
		t.Fatal("the removed member previewed the organization's pack")
	} else {
		t.Logf("the removed member's preview: %v", err)
	}
	keys.mu.Lock()
	for _, a := range keys.sets {
		if strings.Contains(a, "knowledge:") && !strings.HasSuffix(a, "machine") {
			t.Errorf("a key reached the keychain: %s", a)
		}
	}
	keys.mu.Unlock()
}

// devOrLocal is a registry this check may remove a member at: dev's, or one
// on this machine.
func devOrLocal(host string) bool {
	switch host {
	case "dev.secureagentry.net", "127.0.0.1", "localhost":
		return true
	}
	return strings.HasSuffix(host, ".dev.secureagentry.net")
}
