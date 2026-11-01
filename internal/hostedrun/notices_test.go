package hostedrun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeNotices serves one account's notices the way the notifications
// extension does, and records the calls it took.
type fakeNotices struct {
	mu      sync.Mutex
	notices []Notice
	calls   []string
}

func (f *fakeNotices) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			problemJSON(w, http.StatusUnauthorized, "unauthorized", "Sign in required", "This request carried no valid session or token.", nil)
			return false
		}
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
		f.mu.Unlock()
		return true
	}
	mux.HandleFunc("GET /api/v1/x/notifications/notices", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		out := []Notice{}
		for _, n := range f.notices {
			if r.URL.Query().Get("unread") == "true" && n.Read() {
				continue
			}
			out = append(out, n)
		}
		writeJSON(w, http.StatusOK, map[string]any{"notices": out, "next": 7})
	})
	mux.HandleFunc("GET /api/v1/x/notifications/notices/unread-count", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		n := 0
		for _, x := range f.notices {
			if !x.Read() {
				n++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"unread": n})
	})
	mux.HandleFunc("POST /api/v1/x/notifications/notices/read-all", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		now := time.Now().UTC()
		marked := 0
		for i := range f.notices {
			if !f.notices[i].Read() {
				f.notices[i].ReadAt = &now
				marked++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"marked": marked})
	})
	mux.HandleFunc("POST /api/v1/x/notifications/notices/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for i := range f.notices {
			if json.Number(r.PathValue("id")).String() == jsonInt(f.notices[i].ID) {
				if !f.notices[i].Read() {
					now := time.Now().UTC()
					f.notices[i].ReadAt = &now
				}
				writeJSON(w, http.StatusOK, f.notices[i])
				return
			}
		}
		problemJSON(w, http.StatusNotFound, "not-found", "No such notice", "", nil)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// The notices are read newest first with the account's token, only the
// unread ones on request, and marking one or all read goes to the route
// that does it.
func TestTheAccountsNoticesAreReadAndMarked(t *testing.T) {
	f := &fakeNotices{notices: []Notice{
		{ID: 2, Kind: "runFinished", Title: "Reporter finished", Body: "It completed.", Link: "/runs/abc", CreatedAt: time.Now().UTC()},
		{ID: 1, Kind: "billing", Title: "Low balance", Body: "12 credits left.", CreatedAt: time.Now().UTC().Add(-time.Hour)},
	}}
	reg := f.start(t)
	ctx := context.Background()
	c := New(reg, testToken)
	page, err := c.Notices(ctx, false, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Notices) != 2 || page.Notices[0].ID != 2 || page.Notices[0].Link != "/runs/abc" || page.Next != 7 {
		t.Fatalf("page %+v", page)
	}
	n, err := c.UnreadNotices(ctx)
	if err != nil || n != 2 {
		t.Fatalf("unread %d %v", n, err)
	}
	read, err := c.ReadNotice(ctx, 2)
	if err != nil || !read.Read() {
		t.Fatalf("read %+v %v", read, err)
	}
	page, err = c.Notices(ctx, true, 0, 0)
	if err != nil || len(page.Notices) != 1 || page.Notices[0].ID != 1 {
		t.Fatalf("unread page %+v %v", page, err)
	}
	marked, err := c.ReadAllNotices(ctx)
	if err != nil || marked != 1 {
		t.Fatalf("read all %d %v", marked, err)
	}
	f.mu.Lock()
	calls := strings.Join(f.calls, "\n")
	f.mu.Unlock()
	for _, want := range []string{
		"GET /api/v1/x/notifications/notices?limit=20",
		"GET /api/v1/x/notifications/notices/unread-count",
		"POST /api/v1/x/notifications/notices/2/read",
		"GET /api/v1/x/notifications/notices?unread=true",
		"POST /api/v1/x/notifications/notices/read-all",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("no call %q in\n%s", want, calls)
		}
	}
}

// Without the account's token the routes refuse, and the refusal reaches
// the caller as one.
func TestNoticesNeedTheAccountsToken(t *testing.T) {
	reg := (&fakeNotices{}).start(t)
	_, err := New(reg, "").Notices(context.Background(), false, 0, 0)
	if RefusalOf(err) == nil || RefusalOf(err).Status != http.StatusUnauthorized {
		t.Fatalf("err %v", err)
	}
	if _, err := New(reg, "").ReadNotice(context.Background(), 1); RefusalOf(err) == nil {
		t.Fatalf("read: %v", err)
	}
}
