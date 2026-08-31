package authflow

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // loopback test URL
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestHappyPathDeliversToken(t *testing.T) {
	f, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if !strings.Contains(f.AuthorizeURL("https://reg.example"), fmt.Sprintf("port=%d", f.Port)) {
		t.Fatalf("authorize URL misses the port: %s", f.AuthorizeURL("https://reg.example"))
	}

	resp := get(t, fmt.Sprintf("http://127.0.0.1:%d/callback?state=%s&token=agy_test", f.Port, f.State))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tok, err := f.Wait(ctx)
	if err != nil || tok != "agy_test" {
		t.Fatalf("Wait = %q, %v", tok, err)
	}
}

func TestStateMismatchIsRefused(t *testing.T) {
	f, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	resp := get(t, fmt.Sprintf("http://127.0.0.1:%d/callback?state=WRONG&token=agy_evil", f.Port))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched state answered %d, want 403", resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.Wait(ctx); err == nil {
		t.Fatal("Wait accepted a token delivered with the wrong state")
	}
}

func TestContextCancelUnblocksWait(t *testing.T) {
	f, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Wait(ctx); err == nil {
		t.Fatal("Wait returned no error on a canceled context")
	}
}
