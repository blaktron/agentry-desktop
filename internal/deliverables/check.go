package deliverables

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/blaktron/agentry-desktop/internal/runfile"
)

// Why a file is not offered.
var (
	// ErrChanged says the file's bytes are not the ones the envelope names.
	ErrChanged = errors.New("it is not the file the run recorded: its contents differ")
	// ErrUnreadable says the file is gone, is not a regular file, is reached
	// through a link, or is past MaxFileBytes.
	ErrUnreadable = errors.New("it could not be read from the run folder: it is gone, not a regular file or too large")
)

// Checked is a file whose bytes were hashed against the envelope.
type Checked struct {
	File File
	// TypeErr is nil when one click may open the file: its extension is on
	// the list and its bytes are what that extension names.
	TypeErr error
}

// Check hashes the file in the run folder, streaming, and answers
// ErrChanged when the digest is not the envelope's, ErrUnreadable when it
// cannot be read as a regular file through no link. For a file of an
// allowed type it also checks what the bytes are.
func Check(runDir string, f File) (Checked, error) {
	file, size, err := runfile.Open(runDir, f.Path, MaxFileBytes)
	if err != nil {
		return Checked{}, ErrUnreadable
	}
	defer func() { _ = file.Close() }()
	h := sha256.New()
	head := make([]byte, 0, HeadBytes)
	n, err := io.Copy(io.MultiWriter(h, &capture{buf: &head}), io.LimitReader(file, size+1))
	if err != nil || n != size {
		return Checked{}, ErrUnreadable
	}
	// The envelope names a size as well as a digest: one that is not the
	// file's is not the file it recorded.
	if f.Bytes != size {
		return Checked{}, ErrChanged
	}
	if hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return Checked{}, ErrChanged
	}
	c := Checked{File: f}
	switch ext := f.Ext(); {
	case !CanOpen(ext):
		c.TypeErr = errNotAllowed
	default:
		c.TypeErr = CheckHead(ext, head)
		if c.TypeErr == nil && (ext == "docx" || ext == "xlsx" || ext == "pptx") {
			c.TypeErr = CheckOffice(file, size)
		}
	}
	return c, nil
}

var errNotAllowed = errors.New("this type is not opened from here")

// ErrNotAllowed says a file's type is not on the list one click opens.
func ErrNotAllowed() error { return errNotAllowed }

// capture keeps the first HeadBytes bytes written to it.
type capture struct{ buf *[]byte }

func (c *capture) Write(p []byte) (int, error) {
	if room := HeadBytes - len(*c.buf); room > 0 {
		*c.buf = append(*c.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

// ReadChecked reads the whole file, at most limit bytes, and answers it only
// when its SHA-256 is the envelope's: the bytes returned are the bytes
// hashed, so the file cannot change between the check and the use.
func ReadChecked(runDir string, f File, limit int64) ([]byte, error) {
	file, size, err := runfile.Open(runDir, f.Path, min(limit, MaxFileBytes))
	if err != nil {
		return nil, ErrUnreadable
	}
	defer func() { _ = file.Close() }()
	b, err := io.ReadAll(io.LimitReader(file, size+1))
	if err != nil || int64(len(b)) != size {
		return nil, ErrUnreadable
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != f.SHA256 {
		return nil, ErrChanged
	}
	if err := CheckHead(f.Ext(), b[:min(len(b), HeadBytes)]); err != nil {
		return nil, err
	}
	return b, nil
}
