package knowledge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// The ways a kept pack's pages leave the store here, and the only ones: a
// run's attachment, removed at teardown; an export into a folder the person
// picked; and a preview's reading of one page, opened into a 0700 directory
// inside the store and removed before the answer is returned. Each opens
// the pack through the store (store.OpenScratch, store.Open), which checks
// the sealed tree against its manifest before a page is written out.

// Attach opens each named pack for a run on this computer (plan §4.5), into
// a 0700 directory inside the store that done removes; the engine copies
// each into the run's own directory, mounts it read-only on the tools
// container and deletes its copy at teardown. A pack that is not kept or
// will not open (signed out of the registry that wraps its key, another
// account's, altered) is refused before anything runs, naming it.
func (e Env) Attach(ctx context.Context, current string, names []string) (_ []engine.AttachedPack, done func(), err error) {
	var cleanups []func()
	done = func() {
		for _, f := range cleanups {
			f()
		}
	}
	if len(names) == 0 {
		return nil, done, nil
	}
	defer func() {
		if err != nil {
			done()
		}
	}()
	s, err := e.Store(current)
	if err != nil {
		return nil, done, err
	}
	seen := map[string]bool{}
	var out []engine.AttachedPack
	for _, name := range names {
		if err := kn.ValidName(name); err != nil {
			return nil, done, fmt.Errorf("attach knowledge: %w", err)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		p, cleanup, err := s.OpenScratch(ctx, name)
		if err != nil {
			return nil, done, fmt.Errorf("attach knowledge: %w", err)
		}
		cleanups = append(cleanups, cleanup)
		out = append(out, engine.AttachPack(p))
	}
	return out, done, nil
}

// Export writes the pack, in the clear, into a new folder named for it
// under parent (a folder the person picked; the binding asks first and
// picks it). A folder of that name already there is refused rather than
// written into. It answers the folder written.
func (e Env) Export(ctx context.Context, current, name, parent string) (string, error) {
	if err := kn.ValidName(name); err != nil {
		return "", err
	}
	if parent == "" {
		return "", errors.New("export: no folder was chosen")
	}
	s, err := e.Store(current)
	if err != nil {
		return "", err
	}
	if _, err := s.Find(name); err != nil {
		return "", err
	}
	dst := filepath.Join(parent, name)
	if _, err := os.Lstat(dst); err == nil {
		return "", fmt.Errorf("%s already exists; choose another folder, or move it aside first", dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if _, err := s.Open(ctx, name, dst); err != nil {
		// The folder is this export's own, made by it: nothing half
		// written is left behind in the person's folder.
		_ = os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
}

// Preview is a pack's index and the pages it lists, for the panel's
// preview. Its text is the pack's, which a model wrote from the person's
// documents: the page renders it through the run-output reader.
type Preview struct {
	Name  string     `json:"name"`
	Index Page       `json:"index"`
	Pages []PageInfo `json:"pages"`
}

// PageInfo is one page of a pack.
type PageInfo struct {
	// Path is the page under the pack, "wiki/…/name.md".
	Path  string `json:"path"`
	Title string `json:"title"`
}

// Page is one page's text, its frontmatter taken off.
type Page struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Markdown string `json:"markdown"`
	// Truncated says the page was longer than a preview shows.
	Truncated bool `json:"truncated,omitempty"`
}

// previewCap bounds one page's preview, as the run folder's previews are
// bounded; a page past it says so.
const previewCap = 256 << 10

// Preview opens the pack name and answers its index and its pages.
func (e Env) Preview(ctx context.Context, current, name string) (*Preview, error) {
	var out *Preview
	err := e.withOpened(ctx, current, name, func(root *os.Root) error {
		idx, err := readPage(root, kn.WikiDir+"/"+kn.IndexFile)
		if err != nil {
			return err
		}
		pages, err := listPages(root)
		if err != nil {
			return err
		}
		out = &Preview{Name: name, Index: idx, Pages: pages}
		return nil
	})
	return out, err
}

// ReadPage opens the pack name and answers one of its pages. page is a
// path the preview listed; anything else under the pack is refused.
func (e Env) ReadPage(ctx context.Context, current, name, page string) (*Page, error) {
	if !pagePath(page) {
		return nil, fmt.Errorf("%q is not a page of a knowledge pack", page)
	}
	var out Page
	err := e.withOpened(ctx, current, name, func(root *os.Root) error {
		var err error
		out, err = readPage(root, page)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// withOpened opens the pack into a 0700 directory inside the store,
// hands f the pack's own directory as an os.Root, and removes the
// directory before returning.
func (e Env) withOpened(ctx context.Context, current, name string, f func(*os.Root) error) error {
	if err := kn.ValidName(name); err != nil {
		return err
	}
	s, err := e.Store(current)
	if err != nil {
		return err
	}
	p, done, err := s.OpenScratch(ctx, name)
	if err != nil {
		return err
	}
	defer done()
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return f(root)
}

// pagePath holds a page path to wiki/…/*.md with no step outside it.
func pagePath(p string) bool {
	return fs.ValidPath(p) && strings.HasPrefix(p, kn.WikiDir+"/") && strings.HasSuffix(p, ".md") && path.Clean(p) == p
}

// listPages is every page under wiki/ but the indexes and the log, by
// path, with the title its frontmatter gives.
func listPages(root *os.Root) ([]PageInfo, error) {
	var out []PageInfo
	err := fs.WalkDir(root.FS(), kn.WikiDir, func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() || !strings.HasSuffix(rel, ".md") {
			return err
		}
		if path.Base(rel) == kn.IndexFile || rel == kn.WikiDir+"/"+kn.LogFile {
			return nil
		}
		title := strings.TrimSuffix(path.Base(rel), ".md")
		if f, err := root.Open(rel); err == nil {
			if t, _ := frontmatter(io.LimitReader(f, 8<<10)); t.title != "" {
				title = t.title
			}
			_ = f.Close()
		}
		out = append(out, PageInfo{Path: rel, Title: title})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// readPage reads one page, a regular file, through the root, bounded.
func readPage(root *os.Root, rel string) (Page, error) {
	fi, err := root.Lstat(rel)
	if err != nil {
		return Page{}, err
	}
	if !fi.Mode().IsRegular() {
		return Page{}, fmt.Errorf("%s is not a page", rel)
	}
	f, err := root.Open(rel)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = f.Close() }()
	body, err := io.ReadAll(io.LimitReader(f, previewCap+1))
	if err != nil {
		return Page{}, err
	}
	p := Page{Path: rel, Title: strings.TrimSuffix(path.Base(rel), ".md")}
	if len(body) > previewCap {
		body, p.Truncated = body[:previewCap], true
	}
	fm, rest := frontmatter(bytes.NewReader(body))
	if fm.title != "" {
		p.Title = fm.title
	}
	p.Summary = fm.summary
	p.Markdown = rest
	return p, nil
}

type pageMeta struct{ title, summary string }

// frontmatter reads a page's YAML frontmatter for its title and summary
// (one-line scalars, as the Creator writes them) and answers the body
// after it. A page with none is all body.
func frontmatter(r io.Reader) (pageMeta, string) {
	var meta pageMeta
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), previewCap+1)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, strings.Join(lines, "\n")
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			return meta, strings.TrimLeft(strings.Join(lines[i+1:], "\n"), "\n")
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "title":
			meta.title = v
		case "summary":
			meta.summary = v
		}
	}
	// An opening fence with no closing one is not frontmatter.
	return pageMeta{}, strings.Join(lines, "\n")
}
