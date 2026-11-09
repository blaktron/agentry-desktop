package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Repository is where the library's file lives, read over https.
const Repository = "https://raw.githubusercontent.com/blaktron/agentry-desktop"

// Path is the library's file in the repository.
const Path = "internal/library/library.yaml"

// maxFetched bounds a copy read from the repository.
const maxFetched = 4 << 20

// URLFor is the address of the library for an app of this version: a
// release reads the copy on main, which only takes what was released to
// dev first, and a prerelease or a local build reads dev's.
func URLFor(appVersion string) string {
	branch := "main"
	if appVersion == "" || strings.Contains(appVersion, "-") {
		branch = "dev"
	}
	return Repository + "/" + branch + "/" + Path
}

// Origin says which copy of the library is in use.
type Origin string

// The origins.
const (
	// OriginBuiltin is the copy released with this build.
	OriginBuiltin Origin = "built-in"
	// OriginRepository is a copy read from the source repository, now or
	// on an earlier start.
	OriginRepository Origin = "repository"
)

// Status is what the app says of its library.
type Status struct {
	Version int    `json:"version"`
	Presets int    `json:"presets"`
	Origin  Origin `json:"origin"`
	// Checked is when the repository last answered with a copy that could
	// be read, zero for never.
	Checked time.Time `json:"checked"`
	// Error is why the last read of the repository failed, "" when it did
	// not.
	Error string `json:"error"`
}

// Live is the library an app keeps current: the copy released with it, the
// last copy read from the repository and kept beside its settings, and a
// fresh read. Of those, the newest by version is the one in use; a copy
// that fails Parse's checks, names another schema or is not newer than the
// one in use changes nothing.
//
// Moving the library approves nothing: an application already on the
// person's list keeps the server and tools it was added with, so a newer
// copy changes what the library offers and the steps it shows, never what
// a run may call.
type Live struct {
	url    string
	cache  string
	client *http.Client
	// onChange is told the library in use changed.
	onChange func(Library)

	mu      sync.Mutex
	cur     Library
	origin  Origin
	checked time.Time
	lastTry time.Time
	err     string
	// fetching is set while a read of the repository is under way.
	fetching bool
}

// LiveOptions build a Live.
type LiveOptions struct {
	// URL is the library's address; "" reads nothing and keeps the
	// built-in copy, or the cached one.
	URL string
	// Cache is the file the last copy read is kept in; "" keeps none.
	Cache string
	// Client reads the URL; nil is a client with a 20-second timeout.
	Client *http.Client
	// OnChange is told the library in use changed, at start and after a
	// read; nil tells no one.
	OnChange func(Library)
}

// NewLive starts from the newer of the built-in copy and the cached one.
func NewLive(o LiveOptions) *Live {
	l := &Live{url: o.URL, cache: o.Cache, client: o.Client, onChange: o.OnChange, cur: builtin, origin: OriginBuiltin}
	if l.client == nil {
		l.client = &http.Client{Timeout: 20 * time.Second}
	}
	if l.cache != "" {
		if data, err := os.ReadFile(l.cache); err == nil {
			if c, err := ParseFetched(data); err == nil && c.version > l.cur.version {
				l.cur, l.origin = c, OriginRepository
			}
		}
	}
	if l.onChange != nil {
		l.onChange(l.cur)
	}
	return l
}

// Current is the library in use.
func (l *Live) Current() Library {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cur
}

// Status says which copy is in use and how the last read went.
func (l *Live) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Status{Version: l.cur.version, Presets: len(l.cur.entries), Origin: l.origin, Checked: l.checked, Error: l.err}
}

// Stale reports whether the repository was last tried longer ago than age.
func (l *Live) Stale(age time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.fetching && time.Since(l.lastTry) > age
}

// Refresh reads the repository's copy and uses it if it is newer than the
// one in use. It reports whether the library in use changed. Only one
// read runs at a time; a call while one runs returns false at once.
func (l *Live) Refresh(ctx context.Context) (bool, error) {
	if l.url == "" {
		return false, nil
	}
	l.mu.Lock()
	if l.fetching {
		l.mu.Unlock()
		return false, nil
	}
	l.fetching, l.lastTry = true, time.Now()
	l.mu.Unlock()

	c, data, err := l.fetch(ctx)

	l.mu.Lock()
	l.fetching = false
	if err != nil {
		l.err = err.Error()
		l.mu.Unlock()
		return false, err
	}
	l.checked, l.err = time.Now(), ""
	if c.version <= l.cur.version {
		l.mu.Unlock()
		return false, nil
	}
	l.cur, l.origin = c, OriginRepository
	l.mu.Unlock()
	if l.cache != "" {
		_ = writeFile(l.cache, data)
	}
	if l.onChange != nil {
		l.onChange(c)
	}
	return true, nil
}

func (l *Live) fetch(ctx context.Context) (Library, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return Library{}, nil, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := l.client.Do(req)
	if err != nil {
		return Library{}, nil, fmt.Errorf("the library could not be read: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Library{}, nil, fmt.Errorf("the library could not be read: %s answered %s", resp.Request.URL.Host, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetched+1))
	if err != nil {
		return Library{}, nil, fmt.Errorf("the library could not be read: %w", err)
	}
	if len(data) > maxFetched {
		return Library{}, nil, errors.New("the library read from the repository is larger than any library")
	}
	c, err := ParseFetched(data)
	if err != nil {
		return Library{}, nil, fmt.Errorf("the library read from the repository was not used: %w", err)
	}
	return c, data, nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
