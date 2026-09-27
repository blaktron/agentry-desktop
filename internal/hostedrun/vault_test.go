package hostedrun

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeVault serves the vault's two write routes as the enterprise build
// does (agentry-enterprise#251): a token with secret:write sets and
// deletes; the body is recorded whole.
type fakeVault struct {
	mu     sync.Mutex
	bodies []string
	paths  []string
	// answer, when set, answers every request instead.
	answer func(w http.ResponseWriter, r *http.Request, body []byte)
}

func (f *fakeVault) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, string(body))
		f.paths = append(f.paths, r.Method+" "+r.URL.EscapedPath())
		answer := f.answer
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			problemJSON(w, http.StatusUnauthorized, "unauthorized", "Sign in required", "no token", nil)
			return
		}
		if answer != nil {
			answer(w, r, body)
			return
		}
		switch r.Method {
		case http.MethodPut:
			writeJSON(w, http.StatusCreated, map[string]any{"name": r.PathValue("name"), "keep": false})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The value travels once, in the body, with keep as a literal boolean —
// false as well as true, never left out (plan hosted-credentials-vault.md
// D12, agentry-desktop#300).
func TestSetSecretSendsTheValueOnceWithALiteralKeep(t *testing.T) {
	f := &fakeVault{}
	c := New(f.start(t), testToken)
	if err := c.SetSecret(t.Context(), "GITHUB_TOKEN", "ghp_value", false); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSecret(t.Context(), "SEARCH_KEY", "s-value", true); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteSecret(t.Context(), "GITHUB_TOKEN"); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT /api/v1/x/vault/me/secrets/GITHUB_TOKEN", "PUT /api/v1/x/vault/me/secrets/SEARCH_KEY", "DELETE /api/v1/x/vault/me/secrets/GITHUB_TOKEN"}
	if strings.Join(f.paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths %v, want %v", f.paths, want)
	}
	for i, keep := range []bool{false, true} {
		var got map[string]any
		if err := json.Unmarshal([]byte(f.bodies[i]), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got["keep"] != keep {
			t.Fatalf("body %s: want value and a literal keep %v", f.bodies[i], keep)
		}
	}
	if f.bodies[2] != "" {
		t.Fatalf("a delete carried a body: %q", f.bodies[2])
	}
}

// A name outside the manifest's grammar never leaves the machine.
func TestSetSecretRefusesANameBeforeAnyRequest(t *testing.T) {
	f := &fakeVault{}
	c := New(f.start(t), testToken)
	for _, n := range []string{"", "1KEY", "A-B", "../x", strings.Repeat("A", 129)} {
		if err := c.SetSecret(t.Context(), n, "v", false); err == nil {
			t.Fatalf("name %q was accepted", n)
		}
		if err := c.DeleteSecret(t.Context(), n); err == nil {
			t.Fatalf("delete of name %q was accepted", n)
		}
	}
	if len(f.paths) != 0 {
		t.Fatalf("requests were made: %v", f.paths)
	}
}

// No refusal and no error carries the value, whatever the registry says —
// even a registry that echoes the body back (D-024's rule: the value is
// never logged, never shown).
func TestAVaultRefusalNeverCarriesTheValue(t *testing.T) {
	const value = "ghp_do-not-leak-300"
	answers := map[string]func(w http.ResponseWriter, r *http.Request, body []byte){
		"kept": func(w http.ResponseWriter, _ *http.Request, body []byte) {
			problemJSON(w, http.StatusConflict, "secret-kept", "The secret is kept", "echo: "+string(body), nil)
		},
		"not json": func(w http.ResponseWriter, _ *http.Request, body []byte) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write(append([]byte("upstream said "), body...))
		},
		"scope": func(w http.ResponseWriter, _ *http.Request, body []byte) {
			problemJSON(w, http.StatusForbidden, "forbidden", "Scope missing "+string(body), "This token needs the secret:write scope.", nil)
		},
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			f := &fakeVault{answer: answer}
			c := New(f.start(t), testToken)
			err := c.SetSecret(t.Context(), "GITHUB_TOKEN", value, false)
			if err == nil {
				t.Fatal("a refusal was not an error")
			}
			ref := RefusalOf(err)
			if ref == nil {
				t.Fatalf("not a refusal: %v", err)
			}
			for _, s := range []string{err.Error(), ref.Sentence(), ref.Title, ref.Detail} {
				if strings.Contains(s, value) || strings.Contains(s, "do-not-leak") {
					t.Fatalf("the value is in %q", s)
				}
			}
		})
	}
}

// secret-missing names what to ask for.
func TestSecretMissingCarriesTheNames(t *testing.T) {
	f := &fakeRuns{enabled: true, createRefusal: func(w http.ResponseWriter) {
		problemJSON(w, http.StatusConflict, "secret-missing", "A declared secret is missing", "The release declares secrets you have not set: GITHUB_TOKEN.",
			map[string]any{"missing": []string{"GITHUB_TOKEN"}, "secrets": []string{"GITHUB_TOKEN", "SEARCH_KEY"}})
	}}
	c := New(f.start(t), testToken)
	_, err := c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	ref := RefusalOf(err)
	if ref == nil || ref.Slug != "secret-missing" || strings.Join(ref.Missing, ",") != "GITHUB_TOKEN" {
		t.Fatalf("refusal = %+v (%v)", ref, err)
	}
}
