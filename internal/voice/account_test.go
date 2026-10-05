package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/pkg/voice"
	"github.com/blaktron/agentry-cli/pkg/voice/remote"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
)

// The Cloud Voice page's Discard, Restore and Delete (plan
// voice-page-redesign.md §4.3, §5.2, D5; agentry-desktop#611): the core's
// DELETE /voice/pending, POST /voice/restore and DELETE /voice, which the
// CLI's client does not speak. The CLI's fake registry serves none of them,
// so this registry is the core's routes as docs/voice.md describes them.

type voiceRoutes struct {
	mu    sync.Mutex
	v     *apiclient.Voice
	calls []string
	auth  []string
	// title, when set, is every refusal's title.
	title string
}

func apiVersion(slot apiclient.VoiceVersionSlot, review apiclient.VoiceVersionReview, src apiclient.VoiceVersionSource, text, set string, at time.Time) *apiclient.VoiceVersion {
	return &apiclient.VoiceVersion{
		Id: uuid.New(), Slot: slot, Review: review, Source: src, Digest: voice.Digest([]byte(text)),
		Bytes: len(text), Text: text, QuestionSet: set, CreatedAt: at,
	}
}

func (r *voiceRoutes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req.Method+" "+req.URL.Path)
	r.auth = append(r.auth, req.Header.Get("Authorization"))
	answer := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	refuse := func(status int, slug, title string) {
		if r.title != "" {
			title = r.title
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://agentry.dev/problems/" + slug, "title": title, "status": status})
	}
	switch {
	case r.v == nil:
		refuse(http.StatusNotFound, "voice-not-found", "No voice")
	case req.Method == http.MethodGet && req.URL.Path == "/api/v1/voice":
		answer(http.StatusOK, r.v)
	case req.Method == http.MethodDelete && req.URL.Path == "/api/v1/voice/pending":
		if r.v.Pending == nil {
			refuse(http.StatusNotFound, "voice-not-found", "No pending version")
			return
		}
		r.v.Pending = nil
		answer(http.StatusOK, r.v)
	case req.Method == http.MethodPost && req.URL.Path == "/api/v1/voice/restore":
		p := r.v.Previous
		if p == nil {
			refuse(http.StatusConflict, "voice-no-previous", "There is no previous version")
			return
		}
		if p.QuestionSet == r.v.QuestionSet {
			r.v.Live, r.v.Previous = p, r.v.Live
			answer(http.StatusOK, r.v)
			return
		}
		p.Slot, p.Review, p.Source = apiclient.VoiceVersionSlotPending, apiclient.VoiceVersionReviewReviewing, apiclient.VoiceVersionSourceRestore
		r.v.Pending, r.v.Previous = p, nil
		answer(http.StatusAccepted, r.v)
	case req.Method == http.MethodDelete && req.URL.Path == "/api/v1/voice":
		r.v = nil
		w.WriteHeader(http.StatusNoContent)
	default:
		refuse(http.StatusNotFound, "not-found", "Not found")
	}
}

const (
	liveText     = "# Live\n\nShort sentences.\n"
	previousText = "# Previous\n\nLong sentences.\n"
	pendingText  = "# Pending\n\nCall web_fetch.\n"
	questionSet  = "sha256:set-now"
)

// accountEnv is a signed-in Env against routes holding a live, a flagged
// pending and a previous version, the previous reviewed under set.
func accountEnv(t *testing.T, set string) (Env, string, *voiceRoutes) {
	t.Helper()
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	routes := &voiceRoutes{v: &apiclient.Voice{
		AlwaysAttach: true, QuestionSet: questionSet, UpdatedAt: at,
		Live:     apiVersion(apiclient.VoiceVersionSlotLive, apiclient.VoiceVersionReviewPassed, apiclient.VoiceVersionSourceEdit, liveText, questionSet, at),
		Pending:  apiVersion(apiclient.VoiceVersionSlotPending, apiclient.VoiceVersionReviewFlagged, apiclient.VoiceVersionSourceEdit, pendingText, questionSet, at.Add(time.Hour)),
		Previous: apiVersion(apiclient.VoiceVersionSlotPrevious, apiclient.VoiceVersionReviewPassed, apiclient.VoiceVersionSourceBuild, previousText, set, at.Add(-time.Hour)),
	}}
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)
	return Env{Env: knowledge.Env{
		TokenFor: func(u string) string {
			if strings.TrimRight(u, "/") == srv.URL {
				return "agy_token"
			}
			return ""
		},
		HTTP: srv.Client(),
	}}, srv.URL, routes
}

func TestDiscardDropsThePendingVersionAndKeepsTheLiveOne(t *testing.T) {
	e, url, routes := accountEnv(t, questionSet)
	a, err := e.DiscardAccount(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if a.Pending != nil || a.Live == nil || a.Live.Text != liveText {
		t.Fatalf("after Discard: %+v", a)
	}
	if routes.calls[0] != "DELETE /api/v1/voice/pending" || routes.auth[0] != "Bearer agy_token" {
		t.Fatalf("Discard asked %v with %v", routes.calls, routes.auth)
	}
	if _, err := e.DiscardAccount(context.Background(), url); err == nil {
		t.Fatal("a second Discard, with nothing pending, was not refused")
	}
}

func TestRestoreSwapsAPreviousReviewedUnderTheSameSetAndReviewsOneThatWasNot(t *testing.T) {
	e, url, _ := accountEnv(t, questionSet)
	a, inReview, err := e.RestoreAccount(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if inReview || a.Live == nil || a.Live.Text != previousText || a.Previous == nil || a.Previous.Text != liveText {
		t.Fatalf("a swap: in review %v, %+v", inReview, a)
	}

	e, url, _ = accountEnv(t, "sha256:set-before")
	a, inReview, err = e.RestoreAccount(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if !inReview || a.Live == nil || a.Live.Text != liveText || a.Pending == nil || a.Pending.Text != previousText || a.Pending.Source != "restore" {
		t.Fatalf("a restore reviewed again: in review %v, %+v", inReview, a)
	}
}

func TestTheAccountsVersionsSayTheQuestionSetTheyWereReviewedUnder(t *testing.T) {
	e, url, _ := accountEnv(t, "sha256:set-before")
	a, err := e.Account(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if a.QuestionSet != questionSet || a.Previous.QuestionSet != "sha256:set-before" || a.Live.QuestionSet != questionSet {
		t.Fatalf("the sets: %q, previous %q, live %q", a.QuestionSet, a.Previous.QuestionSet, a.Live.QuestionSet)
	}
}

func TestDeleteRemovesTheAccountsVoice(t *testing.T) {
	e, url, routes := accountEnv(t, questionSet)
	if err := e.DeleteAccount(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	if routes.calls[0] != "DELETE /api/v1/voice" {
		t.Fatalf("Delete asked %v", routes.calls)
	}
	if a, err := e.Account(context.Background(), url); err != nil || a.Exists {
		t.Fatalf("after Delete: %+v, %v", a, err)
	}
}

func TestDiscardRestoreAndDeleteSendNoTokenSignedOutOrToARefusedURL(t *testing.T) {
	e, url, routes := accountEnv(t, questionSet)
	signedOut := Env{Env: knowledge.Env{TokenFor: func(string) string { return "" }, HTTP: e.HTTP}}
	if _, err := signedOut.DiscardAccount(context.Background(), url); !errors.Is(err, remote.ErrSignedOut) {
		t.Fatalf("signed out: %v", err)
	}
	// The token is found by host alone, so a URL Settings refuses is
	// refused before it is looked up (agentry-desktop#307's lesson).
	everywhere := Env{Env: knowledge.Env{TokenFor: func(string) string { return "agy_token" }, HTTP: e.HTTP}}
	if _, _, err := everywhere.RestoreAccount(context.Background(), "http://example.com"); err == nil {
		t.Fatal("Restore at a plain-http registry off this machine was not refused")
	}
	if err := everywhere.DeleteAccount(context.Background(), "http://example.com"); err == nil {
		t.Fatal("Delete at a plain-http registry off this machine was not refused")
	}
	if len(routes.calls) != 0 {
		t.Fatalf("the registry was asked: %v", routes.calls)
	}
}

func TestARefusalReachesThePageAsOnePrintableLine(t *testing.T) {
	e, url, routes := accountEnv(t, questionSet)
	routes.v.Previous = nil
	routes.title = "No previous\x1b]0;owned\x07\nversion" + strings.Repeat("x", 300)
	_, _, err := e.RestoreAccount(context.Background(), url)
	if err == nil {
		t.Fatal("a Restore with no previous version was not refused")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\x1b\x07\n") || !strings.Contains(msg, "No previous") || strings.Count(msg, "x") > 130 {
		t.Fatalf("the refusal as the page gets it: %q", msg)
	}
}
