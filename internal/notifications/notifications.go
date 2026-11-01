// Package notifications is the desktop's own record of what it noticed on
// this computer: a run, a draft, a knowledge build or a Writing Voice build
// that ended, a schedule's fire, a persistent agent's task that answered, a
// meeting's note, a QuickAgent installed or updated, and a newer Agentry
// Desktop. Each is one entry with a title, a body, a tone, when it was
// noticed, when the person read it and what it opens in the app, kept in
// notifications.yaml beside desktop.yaml. The Notifications page lists
// them beside the account's own notices read from the registry, and the
// bell counts the unread ones.
//
// Only Go writes it, from what Go itself saw end. An entry never holds a
// run's task, a secret, a file's content or anything an agent wrote: a
// title and a body of the app's own words (a QuickAgent's name, the
// person's own name for a schedule or a persistent agent's task among
// them), and the id of what it opens. The page names an entry by its id
// and nothing else.
package notifications

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Max is how many entries are kept; the oldest go first.
const Max = 200

// FileName is the record's file in the config directory.
const FileName = "notifications.yaml"

// The longest title and body an entry keeps, in characters; a longer one
// is cut and ends in an ellipsis.
const (
	MaxTitle = 200
	MaxBody  = 1000
)

// Kinds of entry, by what ended.
const (
	KindRun       = "run"
	KindChat      = "chat"
	KindDraft     = "draft"
	KindReview    = "review"
	KindKnowledge = "knowledge"
	KindVoice     = "voice"
	KindSchedule  = "schedule"
	KindUpdate    = "update"
	KindInstall   = "install"
	KindTask      = "task"
	KindMeeting   = "meeting"
)

var kinds = map[string]bool{
	KindRun: true, KindChat: true, KindDraft: true, KindReview: true, KindKnowledge: true, KindVoice: true,
	KindSchedule: true, KindUpdate: true, KindInstall: true, KindTask: true, KindMeeting: true,
}

// Tones an entry is drawn in: "" for news, ok for a job that finished,
// warn for one stopped or blocked, bad for one that failed.
const (
	ToneOK   = "ok"
	ToneWarn = "warn"
	ToneBad  = "bad"
)

// Pages an entry opens, with what Open names on each.
const (
	// PageRun opens the run page: ID is the run's id, Registry the
	// registry of a run In Agentry, Hosted whether it ran there.
	PageRun = "run"
	// PageAgent opens a QuickAgent's page: ID is namespace/name, Registry
	// its registry.
	PageAgent = "agent"
	// PagePersistent opens a persistent agent's page: ID is its name.
	PagePersistent = "persistent"
	// PageMeetings opens Meeting notes.
	PageMeetings = "meetings"
	// PageScheduled opens the Scheduled page: ID is the schedule's id.
	PageScheduled = "scheduled"
	// PageSettings opens a Settings section: Section and Part name it.
	PageSettings = "settings"
)

var pages = map[string]bool{PageRun: true, PageAgent: true, PagePersistent: true, PageMeetings: true, PageScheduled: true, PageSettings: true}

// Open is what a click on an entry opens in the app.
type Open struct {
	Page     string `yaml:"page" json:"page"`
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Registry string `yaml:"registry,omitempty" json:"registry,omitempty"`
	Hosted   bool   `yaml:"hosted,omitempty" json:"hosted,omitempty"`
	Section  string `yaml:"section,omitempty" json:"section,omitempty"`
	Part     string `yaml:"part,omitempty" json:"part,omitempty"`
}

// Key names the place an Open shows, as the page reports the place that is
// on screen: an entry whose result is on screen raises no desktop
// notification. A run page is its run, a QuickAgent's page its reference,
// a persistent agent's page its agent, a Settings section its section; the
// Scheduled page shows every schedule, so it is one place.
func (o Open) Key() string {
	switch o.Page {
	case PageRun, PageAgent, PagePersistent:
		return o.Page + ":" + o.ID
	case PageSettings:
		return o.Page + ":" + o.Section
	}
	return o.Page
}

// idPattern is a run's id as the run page takes one, or a schedule's or a
// persistent agent's name: letters, digits and a few marks.
var idPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`)

// agentPattern is a QuickAgent's namespace/name.
var agentPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,63}/[0-9A-Za-z][0-9A-Za-z._-]{0,63}$`)

// valid is what an Open must be for the app to follow it: a page the app
// has, an id of the shape that page takes, and a registry that is a
// registry URL. An Open that names no page opens nothing and is valid.
func (o Open) valid() bool {
	if o.Page == "" {
		return o == Open{}
	}
	if !pages[o.Page] {
		return false
	}
	if o.Registry != "" && settings.ValidateURL(o.Registry) != nil {
		return false
	}
	switch o.Page {
	case PageRun, PagePersistent, PageScheduled:
		return o.ID == "" || idPattern.MatchString(o.ID)
	case PageAgent:
		return agentPattern.MatchString(o.ID)
	case PageSettings:
		return o.Section != "" && idPattern.MatchString(o.Section) && (o.Part == "" || idPattern.MatchString(o.Part))
	}
	return o.ID == ""
}

// Entry is one thing the app noticed.
type Entry struct {
	ID    string `yaml:"id"`
	Kind  string `yaml:"kind"`
	Title string `yaml:"title"`
	Body  string `yaml:"body,omitempty"`
	Tone  string `yaml:"tone,omitempty"`
	// Key, when set, names the one thing the entry is about (a release
	// that is available): a later entry with the same key replaces it.
	Key       string    `yaml:"key,omitempty"`
	CreatedAt time.Time `yaml:"createdAt"`
	// ReadAt is when the person read it; zero while unread.
	ReadAt time.Time `yaml:"readAt,omitempty"`
	Open   Open      `yaml:"open,omitempty"`
}

// Read reports whether the entry was read.
func (e Entry) Read() bool { return !e.ReadAt.IsZero() }

// file is notifications.yaml.
type file struct {
	Version int     `yaml:"version"`
	Entries []Entry `yaml:"entries"`
	// FiresSeen is the end time of the newest schedule fire the app has
	// made an entry for, so a fire that ended while the app was closed is
	// noticed once on the next start, and none twice.
	FiresSeen time.Time `yaml:"firesSeen,omitempty"`
}

// version is the file's shape. A file of another version is read as empty
// and replaced on the next write.
const version = 1

// valid is what an entry must be to be kept or read: an id, a known kind, a
// title, and an Open the app can follow. An entry that is not (a
// hand-edited file) is dropped, never shown.
func (e Entry) valid() bool {
	return e.ID != "" && kinds[e.Kind] && e.Title != "" && e.Open.valid()
}

// normalized is e as Add keeps it: the title and the body plain text, one
// line each, cut to their lengths, and the tone one of the three or none.
func (e Entry) normalized() Entry {
	e.Title = plain(e.Title, MaxTitle)
	e.Body = plain(e.Body, MaxBody)
	switch e.Tone {
	case ToneOK, ToneWarn, ToneBad:
	default:
		e.Tone = ""
	}
	return e
}

// plain is s as one line of text, control and format characters replaced,
// runs of space collapsed, cut to limit characters.
func plain(s string, limit int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit-1])) + "…"
}

// mu serializes this process's reads and writes of the file.
var mu sync.Mutex

// Path is the record's file.
func Path() (string, error) {
	dir, err := configdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// load reads the file; a missing file is empty, a file of another version
// or one that cannot be parsed is read as empty and replaced on the next
// write. Entries that are not valid are dropped.
func load() (file, error) {
	path, err := Path()
	if err != nil {
		return file{}, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // the app's own file in its config directory
	if errors.Is(err, fs.ErrNotExist) {
		return file{Version: version}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("read the notifications: %w", err)
	}
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil || f.Version != version {
		return file{Version: version}, nil
	}
	kept := f.Entries[:0]
	for _, e := range f.Entries {
		if e.valid() {
			kept = append(kept, e)
		}
	}
	f.Entries = kept
	return f, nil
}

// save writes the file, 0600, beside its place and renamed in, so a reader
// never sees half of it.
func save(f file) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write the notifications: %w", err)
	}
	f.Version = version
	sortNewestFirst(f.Entries)
	if len(f.Entries) > Max {
		f.Entries = f.Entries[:Max]
	}
	b, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("write the notifications: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+FileName+"-*")
	if err != nil {
		return fmt.Errorf("write the notifications: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write the notifications: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write the notifications: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write the notifications: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write the notifications: %w", err)
	}
	return nil
}

// sortNewestFirst orders entries by when they were noticed, newest first.
func sortNewestFirst(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
}

// newID is a fresh entry id.
func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "n" + hex.EncodeToString(b[:])
}

// Add keeps e, unread, and returns it as kept: with an id when it had
// none, noticed now when it had no time, its text made plain, and in place
// of an earlier entry with the same Key. An entry with no title or an
// unknown kind is refused. The newest Max are kept.
func Add(e Entry) (Entry, error) {
	e = e.normalized()
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	e.ReadAt = time.Time{}
	if !e.valid() {
		return Entry{}, fmt.Errorf("a notification needs a title, a known kind and a page the app has (%q, %q, %q)", e.Kind, e.Title, e.Open.Page)
	}
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return Entry{}, err
	}
	kept := make([]Entry, 0, len(f.Entries)+1)
	for _, k := range f.Entries {
		if k.ID == e.ID || (e.Key != "" && k.Key == e.Key) {
			continue
		}
		kept = append(kept, k)
	}
	f.Entries = append(kept, e)
	return e, save(f)
}

// List is every entry, newest first.
func List() ([]Entry, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return nil, err
	}
	sortNewestFirst(f.Entries)
	return f.Entries, nil
}

// Find is the entry with id.
func Find(id string) (Entry, bool, error) {
	entries, err := List()
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

// Unread counts the entries not yet read.
func Unread() (int, error) {
	entries, err := List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.Read() {
			n++
		}
	}
	return n, nil
}

// MarkRead marks the entry with id read now, and reports whether there was
// one; one already read keeps its first time.
func MarkRead(id string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return false, err
	}
	for i := range f.Entries {
		if f.Entries[i].ID != id {
			continue
		}
		if f.Entries[i].ReadAt.IsZero() {
			f.Entries[i].ReadAt = time.Now().UTC()
			return true, save(f)
		}
		return true, nil
	}
	return false, nil
}

// MarkAllRead marks every unread entry read now, and returns how many were.
func MarkAllRead() (int, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	n := 0
	for i := range f.Entries {
		if f.Entries[i].ReadAt.IsZero() {
			f.Entries[i].ReadAt = now
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, save(f)
}

// FiresSeen is the end time of the newest schedule fire an entry was made
// for; zero when none was yet.
func FiresSeen() (time.Time, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return time.Time{}, err
	}
	return f.FiresSeen, nil
}

// SetFiresSeen records the end time of the newest schedule fire an entry
// was made for. An earlier time than the one kept changes nothing.
func SetFiresSeen(t time.Time) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := load()
	if err != nil {
		return err
	}
	if !t.After(f.FiresSeen) {
		return nil
	}
	f.FiresSeen = t.UTC()
	return save(f)
}
