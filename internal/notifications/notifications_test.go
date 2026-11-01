package notifications

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

func inTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(configdir.EnvVar, dir)
	return dir
}

// An entry added is listed newest first and unread, with an id and a time
// of its own; its title and body are kept as one line of plain text each.
func TestAddKeepsEntriesNewestFirstAndUnread(t *testing.T) {
	inTempConfig(t)
	first, err := Add(Entry{Kind: KindRun, Title: "Reporter finished", Body: "The run finished.", Tone: ToneOK, Open: Open{Page: PageRun, ID: "r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.CreatedAt.IsZero() || first.Read() {
		t.Fatalf("first as kept: %+v", first)
	}
	second, err := Add(Entry{Kind: KindDraft, Title: "The\tdraft\nis ready\u200b", Body: strings.Repeat("x", MaxBody+20), Tone: "loud", CreatedAt: first.CreatedAt.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Title != "The draft is ready" || utf8Len(second.Body) != MaxBody || !strings.HasSuffix(second.Body, "…") || second.Tone != "" {
		t.Fatalf("second as kept: %q %d %q", second.Title, utf8Len(second.Body), second.Tone)
	}
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("list %+v", got)
	}
	n, _ := Unread()
	if n != 2 {
		t.Fatalf("unread %d", n)
	}
}

func utf8Len(s string) int { return len([]rune(s)) }

// An entry with no title, an unknown kind or a page the app does not have
// is refused, and nothing is written.
func TestAddRefusesWhatCannotBeShown(t *testing.T) {
	dir := inTempConfig(t)
	for name, e := range map[string]Entry{
		"no title":     {Kind: KindRun},
		"unknown kind": {Kind: "surprise", Title: "t"},
		"unknown page": {Kind: KindRun, Title: "t", Open: Open{Page: "elsewhere"}},
		"blank title":  {Kind: KindRun, Title: " \n\t "},
	} {
		if _, err := Add(e); err == nil {
			t.Errorf("%s: added", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Fatalf("a refused entry wrote the file: %v", err)
	}
}

// Marking one read keeps its first time; marking all read counts the
// unread ones; both report what they did.
func TestMarkReadAndMarkAllRead(t *testing.T) {
	inTempConfig(t)
	a, _ := Add(Entry{Kind: KindRun, Title: "a"})
	b, _ := Add(Entry{Kind: KindRun, Title: "b"})
	_, _ = Add(Entry{Kind: KindRun, Title: "c"})
	ok, err := MarkRead(a.ID)
	if err != nil || !ok {
		t.Fatalf("mark a: %v %v", ok, err)
	}
	got, _, _ := Find(a.ID)
	first := got.ReadAt
	if first.IsZero() {
		t.Fatal("a is not read")
	}
	time.Sleep(2 * time.Millisecond)
	if ok, _ := MarkRead(a.ID); !ok {
		t.Fatal("a again: not found")
	}
	got, _, _ = Find(a.ID)
	if !got.ReadAt.Equal(first) {
		t.Fatalf("a's read time moved: %v then %v", first, got.ReadAt)
	}
	if ok, _ := MarkRead("nobody"); ok {
		t.Fatal("an unknown id was found")
	}
	n, err := MarkAllRead()
	if err != nil || n != 2 {
		t.Fatalf("mark all: %d %v", n, err)
	}
	if n, _ := Unread(); n != 0 {
		t.Fatalf("unread after mark all: %d", n)
	}
	if n, _ := MarkAllRead(); n != 0 {
		t.Fatalf("mark all again: %d", n)
	}
	got, _, _ = Find(b.ID)
	if !got.Read() {
		t.Fatal("b is not read")
	}
}

// An entry with a key replaces the earlier entry with that key: a release
// found newer twice is one entry, the newer one, unread again.
func TestAKeyedEntryReplacesTheEarlierOne(t *testing.T) {
	inTempConfig(t)
	old, _ := Add(Entry{Kind: KindUpdate, Key: "update:available", Title: "Agentry Desktop 1.2.3 is available"})
	_, _ = MarkRead(old.ID)
	got, err := Add(Entry{Kind: KindUpdate, Key: "update:available", Title: "Agentry Desktop 1.2.4 is available"})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := List()
	if len(list) != 1 || list[0].ID != got.ID || list[0].Read() || list[0].Title != "Agentry Desktop 1.2.4 is available" {
		t.Fatalf("list %+v", list)
	}
}

// Only the newest Max entries are kept.
func TestTheNewestMaxAreKept(t *testing.T) {
	inTempConfig(t)
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for i := 0; i < Max+5; i++ {
		if _, err := Add(Entry{Kind: KindRun, Title: "run", CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := List()
	if len(list) != Max || !list[0].CreatedAt.Equal(base.Add(time.Duration(Max+4)*time.Second)) || !list[Max-1].CreatedAt.Equal(base.Add(5*time.Second)) {
		t.Fatalf("kept %d, newest %v, oldest %v", len(list), list[0].CreatedAt, list[len(list)-1].CreatedAt)
	}
}

// A hand-edited file: an entry with an unknown kind or page is dropped, a
// file of another version is read as empty, and the file is written 0600.
func TestAHandEditedFileIsReadLeniently(t *testing.T) {
	dir := inTempConfig(t)
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("version: 1\nentries:\n  - id: a\n    kind: run\n    title: kept\n    createdAt: 2026-10-31T12:00:00Z\n  - id: b\n    kind: surprise\n    title: dropped\n    createdAt: 2026-10-31T12:00:01Z\n  - id: c\n    kind: run\n    title: dropped too\n    createdAt: 2026-10-31T12:00:02Z\n    open:\n      page: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := List()
	if err != nil || len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := os.WriteFile(path, []byte("version: 7\nentries:\n  - id: a\n    kind: run\n    title: kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if list, _ := List(); len(list) != 0 {
		t.Fatalf("another version read: %+v", list)
	}
	if _, err := Add(Entry{Kind: KindRun, Title: "new"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode %o", perm)
	}
}

// The schedule fires' watermark only moves forward.
func TestFiresSeenOnlyMovesForward(t *testing.T) {
	inTempConfig(t)
	if got, _ := FiresSeen(); !got.IsZero() {
		t.Fatalf("fresh watermark %v", got)
	}
	later := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if err := SetFiresSeen(later); err != nil {
		t.Fatal(err)
	}
	if err := SetFiresSeen(later.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := FiresSeen(); !got.Equal(later) {
		t.Fatalf("watermark %v, want %v", got, later)
	}
}

// Each Open names the place the page reports when it is on screen.
func TestOpenKeys(t *testing.T) {
	for _, c := range []struct {
		o    Open
		want string
	}{
		{Open{Page: PageRun, ID: "r1", Hosted: true, Registry: "https://x"}, "run:r1"},
		{Open{Page: PageAgent, ID: "acme/reporter"}, "agent:acme/reporter"},
		{Open{Page: PagePersistent, ID: "assistant"}, "persistent:assistant"},
		{Open{Page: PageSettings, Section: "updates", Part: "desktop"}, "settings:updates"},
		{Open{Page: PageMeetings}, "meetings"},
		{Open{}, ""},
	} {
		if got := c.o.Key(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.o, got, c.want)
		}
	}
}
