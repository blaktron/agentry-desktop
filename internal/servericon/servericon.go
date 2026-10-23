// Package servericon finds and keeps the icon of an MCP server a person
// added to a persistent agent: the `icons` of the server's entry in the
// official MCP Registry, the entry whose package (or remote address) is the
// server's own. One icon is fetched once and kept, never again; a server
// with none is remembered as having none.
//
// What is kept is held to rules a card can draw safely: fetched over https
// only, at most MaxBytes, and a PNG, JPEG or WebP by its own first bytes,
// whatever its address or its declared type says. SVG is refused, since an
// SVG can carry script. The page draws what is kept as a data: URL, so no
// image is ever loaded from the network by the page.
package servericon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"
)

// MaxBytes is the most an icon is.
const MaxBytes = 256 << 10

// RegistryBase is the official MCP Registry's API.
const RegistryBase = "https://registry.modelcontextprotocol.io/v0.1"

// maxRegistryBytes bounds one page of the registry's answer.
const maxRegistryBytes = 4 << 20

// Icon is one of a server.json's icons, as the registry serves it.
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// kinds are the image types an icon may be, by MIME type, with the
// extension it is kept under.
var kinds = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

// Sniff is the image type of b by its first bytes: PNG, JPEG or WebP, and
// nothing else.
func Sniff(b []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp", true
	}
	return "", false
}

// Choose picks the icon to fetch: one at an https address whose declared
// type, or else its address's extension, is PNG, JPEG or WebP, preferring
// the largest declared size up to 256 pixels and else the smallest above.
func Choose(icons []Icon) (Icon, bool) {
	best, bestSize, found := Icon{}, 0, false
	for _, ic := range icons {
		u, err := url.Parse(ic.Src)
		if err != nil || u.Scheme != "https" || u.Host == "" || !allowedType(ic, u) {
			continue
		}
		size := largestSize(ic.Sizes)
		if !found || better(size, bestSize) {
			best, bestSize, found = ic, size, true
		}
	}
	return best, found
}

func allowedType(ic Icon, u *url.URL) bool {
	if mt := strings.ToLower(strings.TrimSpace(ic.MimeType)); mt != "" {
		_, ok := kinds[mt]
		return ok
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

// largestSize is the largest "NxN" of sizes, 0 for none or "any".
func largestSize(sizes []string) int {
	n := 0
	for _, s := range sizes {
		w, _, ok := strings.Cut(strings.ToLower(s), "x")
		if !ok {
			continue
		}
		if v, err := strconv.Atoi(w); err == nil && v > n {
			n = v
		}
	}
	return n
}

// better says a size beats the best so far: up to 256 the larger, past it
// the smaller, and any declared size over none.
func better(size, best int) bool {
	switch {
	case best == 0:
		return size > 0
	case size == 0:
		return false
	case size <= 256 && best <= 256:
		return size > best
	case size <= 256:
		return true
	case best <= 256:
		return false
	}
	return size < best
}

// Lookup reads the icons of the registry entry that is srv: a package of
// the same ecosystem and name, or a remote at the same address. Entries are
// searched by the package's last name part (or the address's host), and
// only an exact match counts. None found is no icons and no error.
func Lookup(ctx context.Context, c *http.Client, base string, srv manifest.MCPServer) ([]Icon, error) {
	term := searchTerm(srv)
	if term == "" {
		return nil, nil
	}
	q := url.Values{"search": {term}, "limit": {"100"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/servers?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the MCP Registry answered %s", resp.Status)
	}
	var page struct {
		Servers []struct {
			Server struct {
				Icons    []Icon `json:"icons"`
				Packages []struct {
					RegistryType string `json:"registryType"`
					Identifier   string `json:"identifier"`
				} `json:"packages"`
				Remotes []struct {
					URL string `json:"url"`
				} `json:"remotes"`
			} `json:"server"`
		} `json:"servers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRegistryBytes)).Decode(&page); err != nil {
		return nil, fmt.Errorf("the MCP Registry's answer: %w", err)
	}
	for _, s := range page.Servers {
		e := s.Server
		for _, p := range e.Packages {
			if (srv.NPM != "" && p.RegistryType == "npm" && p.Identifier == srv.NPM) || (srv.PyPI != "" && p.RegistryType == "pypi" && strings.EqualFold(p.Identifier, srv.PyPI)) {
				return e.Icons, nil
			}
		}
		for _, r := range e.Remotes {
			if srv.IsRemote() && strings.TrimRight(r.URL, "/") == strings.TrimRight(srv.URL, "/") {
				return e.Icons, nil
			}
		}
	}
	return nil, nil
}

// searchTerm is what the registry's name search is asked for: a package's
// last name part, or a remote address's host.
func searchTerm(srv manifest.MCPServer) string {
	switch {
	case srv.NPM != "":
		return srv.NPM[strings.LastIndex(srv.NPM, "/")+1:]
	case srv.PyPI != "":
		return srv.PyPI
	case srv.URL != "":
		if u, err := url.Parse(srv.URL); err == nil {
			return u.Hostname()
		}
	}
	return ""
}

// ErrNotAnImage is a fetched icon that is not a PNG, JPEG or WebP, or is
// larger than MaxBytes.
var ErrNotAnImage = errors.New("not a PNG, JPEG or WebP of at most 256 KiB")

// Fetch reads one icon: over https alone, redirects included, at most
// MaxBytes, and a PNG, JPEG or WebP by its first bytes.
func Fetch(ctx context.Context, c *http.Client, src string) ([]byte, string, error) {
	u, err := url.Parse(src)
	if err != nil || u.Scheme != "https" {
		return nil, "", fmt.Errorf("an icon is fetched over https only")
	}
	cc := *c
	cc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("an icon is fetched over https only")
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := cc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("the icon's address answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > MaxBytes {
		return nil, "", ErrNotAnImage
	}
	mt, ok := Sniff(b)
	if !ok {
		return nil, "", ErrNotAnImage
	}
	return b, mt, nil
}

// Find is the icon of srv: its registry entry's chosen icon, fetched. No
// entry, or an entry with no icon it may draw, is (nil, "", nil).
func Find(ctx context.Context, c *http.Client, base string, srv manifest.MCPServer) ([]byte, string, error) {
	icons, err := Lookup(ctx, c, base, srv)
	if err != nil {
		return nil, "", err
	}
	ic, ok := Choose(icons)
	if !ok {
		return nil, "", nil
	}
	return Fetch(ctx, c, ic.Src)
}

// Store keeps each server's icon once, under Dir, by the hash of its source
// id: <hash>.png, .jpg or .webp, or <hash>.none for a server found to have
// none it may draw.
type Store struct{ Dir string }

func key(srv manifest.MCPServer) string {
	sum := sha256.Sum256([]byte(srv.SourceID()))
	return hex.EncodeToString(sum[:16])
}

// Get is srv's kept icon as a data: URL, "" when it has none, and whether
// a fetch for it was ever kept (an icon or none).
func (s Store) Get(srv manifest.MCPServer) (dataURL string, known bool) {
	k := key(srv)
	for mt, ext := range kinds {
		b, err := os.ReadFile(filepath.Join(s.Dir, k+"."+ext))
		if err != nil {
			continue
		}
		// What was kept is held to the rules again as it is read.
		if got, ok := Sniff(b); !ok || got != mt || len(b) > MaxBytes {
			return "", true
		}
		return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), true
	}
	if _, err := os.Stat(filepath.Join(s.Dir, k+".none")); err == nil {
		return "", true
	}
	return "", false
}

// Put keeps srv's icon b of type mt, or that it has none when b is nil.
func (s Store) Put(srv manifest.MCPServer, b []byte, mt string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	k := key(srv)
	if b == nil {
		return os.WriteFile(filepath.Join(s.Dir, k+".none"), nil, 0o600)
	}
	ext, ok := kinds[mt]
	if got, sniffed := Sniff(b); !ok || !sniffed || got != mt || len(b) > MaxBytes {
		return ErrNotAnImage
	}
	return os.WriteFile(filepath.Join(s.Dir, k+"."+ext), b, 0o600)
}
