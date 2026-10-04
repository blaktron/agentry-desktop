package voice

import (
	"testing"
	"time"
)

// Download Cloud version and Upload to Cloud judge newer and older with no
// backend change (plan voice-page-redesign.md §8, D2; agentry-desktop#610):
// equal digests are the same voice, otherwise this computer's updated stamp
// against the account version's createdAt.
func TestSyncJudgesTheSameByDigestAndNewerByTheDates(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	here := func(digest, updated string) *Local {
		return &Local{Exists: true, Digest: digest, Updated: updated}
	}
	live := &Version{Review: "passed", Digest: "sha256:live", CreatedAt: at("2026-10-03T09:15:00Z")}
	reviewing := &Version{Review: "reviewing", Digest: "sha256:pending", CreatedAt: at("2026-10-04T10:00:00Z")}
	flagged := &Version{Review: "flagged", Digest: "sha256:pending", CreatedAt: at("2026-10-04T10:00:00Z")}

	for _, c := range []struct {
		name string
		l    *Local
		a    *Account
		want Sync
	}{
		{"no account voice", here("sha256:x", "2026-10-04T14:02:00Z"), &Account{}, Sync{}},
		{"nothing here", &Local{}, &Account{Exists: true, Live: live}, Sync{}},
		{"the same as the live one", here("sha256:live", "2026-10-01T00:00:00Z"), &Account{Exists: true, Live: live},
			Sync{DownloadSame: true, UploadSame: true}},
		{"saved here after the live one", here("sha256:x", "2026-10-04T14:02:00Z"), &Account{Exists: true, Live: live},
			Sync{LocalAfterLive: true}},
		{"saved here before the live one", here("sha256:x", "2026-10-02T08:00:00Z"), &Account{Exists: true, Live: live},
			Sync{AccountAfterLocal: true}},
		{"a newer version in review, uploaded from here", here("sha256:pending", "2026-10-04T09:59:00Z"), &Account{Exists: true, Live: live, Pending: reviewing},
			Sync{UploadSame: true, LocalAfterLive: true, InReview: true}},
		{"a newer version in review, set after this computer's", here("sha256:x", "2026-10-04T08:00:00Z"), &Account{Exists: true, Live: live, Pending: reviewing},
			Sync{LocalAfterLive: true, AccountAfterLocal: true, InReview: true}},
		// A flagged version will not go live: newer for an upload, but not
		// "in review" for a download.
		{"a flagged version", here("sha256:x", "2026-10-04T08:00:00Z"), &Account{Exists: true, Live: live, Pending: flagged},
			Sync{LocalAfterLive: true, AccountAfterLocal: true}},
		{"only a pending version", here("sha256:x", "2026-10-04T11:00:00Z"), &Account{Exists: true, Pending: reviewing}, Sync{}},
		// A stamp that does not read judges nothing newer: the digests still say "the same".
		{"an unreadable stamp", here("sha256:x", "yesterday"), &Account{Exists: true, Live: live}, Sync{}},
	} {
		if got := SyncOf(c.l, c.a); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := SyncOf(here("sha256:x", "2026-10-04T14:02:00Z"), nil); got != (Sync{}) {
		t.Errorf("no account read: %+v", got)
	}
}
