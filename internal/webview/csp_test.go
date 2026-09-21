package webview

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareSetsThePolicyOnEveryResponse(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<!doctype html>"))
	})
	h := Middleware(inner)
	for _, p := range []string{"/", "/index.html", "/assets/index-abc123.js", "/missing"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if got := rec.Header().Get("Content-Security-Policy"); got != CSP {
			t.Errorf("%s: CSP = %q", p, got)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: companion headers missing: %v", p, rec.Header())
		}
	}
}

// The directives that make a README's script inert. A change to any of
// these is a decision about what registry-supplied HTML may do here.
func TestPolicyRefusesScriptFromAnywhereButTheBundle(t *testing.T) {
	for _, want := range []string{
		"script-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'none'",
	} {
		if !strings.Contains(CSP, want) {
			t.Errorf("policy lacks %q: %s", want, CSP)
		}
	}
	for _, never := range []string{"'unsafe-inline'", "'unsafe-eval'", "*"} {
		// 'unsafe-inline' may appear only under style-src.
		for _, d := range strings.Split(CSP, "; ") {
			if strings.HasPrefix(d, "style-src") {
				continue
			}
			if strings.Contains(d, never) {
				t.Errorf("directive %q admits %s", d, never)
			}
		}
	}
}
