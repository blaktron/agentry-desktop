package meeting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Models keeps the downloaded models in one folder. A file is used only
// after its SHA-256 matches the catalogue's, every time it is used: a file
// that does not match, a download included, is deleted and refused.
type Models struct {
	Dir    string
	Client *http.Client

	mu sync.Mutex // one download at a time
}

// DownloadClient is the client models are fetched with: https only, through
// redirects too, with a timeout on every stage but the body, which can take
// a while on a slow line.
func DownloadClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   20 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing a redirect to %s: not https", req.URL.Scheme)
			}
			if len(via) > 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// ErrDamaged is a file whose SHA-256 is not the one the catalogue pins.
var ErrDamaged = errors.New("the file is not the one Agentry pins")

// Path is where an asset is kept.
func (m *Models) Path(a Asset) string { return filepath.Join(m.Dir, a.File) }

// Present says whether the asset's file is there, without checking it.
func (m *Models) Present(a Asset) bool {
	st, err := os.Stat(m.Path(a))
	return err == nil && st.Mode().IsRegular() && st.Size() == a.Size
}

// Verify checks the kept file against the catalogue. A file that does not
// match is deleted, and ErrDamaged returned.
func (m *Models) Verify(a Asset) (string, error) {
	p := m.Path(a)
	sum, n, err := fileSHA256(p)
	if err != nil {
		return "", err
	}
	if n != a.Size || sum != a.SHA256 {
		_ = os.Remove(p)
		return "", fmt.Errorf("%s: %w; it was deleted", a.File, ErrDamaged)
	}
	return p, nil
}

// Ensure returns the checked path of the asset, downloading it first when
// it is missing or damaged. progress, when set, is told the bytes so far.
func (m *Models) Ensure(ctx context.Context, a Asset, progress func(done, total int64)) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Present(a) {
		if p, err := m.Verify(a); err == nil {
			return p, nil
		}
	}
	if err := m.download(ctx, a, progress); err != nil {
		return "", err
	}
	return m.Verify(a)
}

// download fetches a into a .part file beside its place, checks its length
// and SHA-256 as it arrives, and only then renames it into place. A file
// that does not match is deleted.
func (m *Models) download(ctx context.Context, a Asset, progress func(done, total int64)) error {
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return err
	}
	part := m.Path(a) + ".part"
	_ = os.Remove(part)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	client := m.Client
	if client == nil {
		client = DownloadClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", a.File, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: the server answered %s", a.File, resp.Status)
	}
	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the models folder's own .part file
	if err != nil {
		return err
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if done+int64(n) > a.Size {
				rerr = fmt.Errorf("longer than the %d bytes pinned", a.Size)
			} else if _, werr := w.Write(buf[:n]); werr != nil {
				rerr = werr
			} else {
				done += int64(n)
				if progress != nil {
					progress(done, a.Size)
				}
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			_ = f.Close()
			_ = os.Remove(part)
			return fmt.Errorf("download %s: %w", a.File, rerr)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(part)
		return err
	}
	if sum := hex.EncodeToString(h.Sum(nil)); done != a.Size || sum != a.SHA256 {
		_ = os.Remove(part)
		return fmt.Errorf("download %s: %w; it was deleted", a.File, ErrDamaged)
	}
	return os.Rename(part, m.Path(a))
}

func fileSHA256(p string) (string, int64, error) {
	f, err := os.Open(p) //nolint:gosec // a file in the models folder
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
