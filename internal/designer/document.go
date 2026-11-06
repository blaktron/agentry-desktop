package designer

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// A Design may start from one document the person already has, a PDF or a
// Word document, which the Report Designer reads in full and keeps only for
// the run. DocumentHere says whether the Report Designer this app carries
// reads one: while it is false the dialog draws no document field and a run
// given one is refused.
const DocumentHere = false

// MaxDocumentBytes caps the document.
const MaxDocumentBytes = 20 << 20

// The two kinds of document a Design takes, by their bytes.
const (
	DocumentPDF  = "pdf"
	DocumentDOCX = "docx"
)

// CheckDocument holds a picked document to the rules before anything reads
// it: one file, at most 20 MB, a PDF or a Word document by its bytes, never
// by its name alone. It answers the kind.
func CheckDocument(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("the document: %w", err)
	}
	switch {
	case !info.Mode().IsRegular():
		return "", fmt.Errorf("the document %s is not a file", filepath.Base(path))
	case info.Size() > MaxDocumentBytes:
		return "", fmt.Errorf("the document %s is over %d MB", filepath.Base(path), MaxDocumentBytes>>20)
	case info.Size() == 0:
		return "", fmt.Errorf("the document %s is empty", filepath.Base(path))
	}
	f, err := os.Open(path) //nolint:gosec // a file a native picker returned
	if err != nil {
		return "", fmt.Errorf("the document: %w", err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 5)
	if _, err := io.ReadFull(f, head); err != nil {
		return "", fmt.Errorf("the document %s is neither a PDF nor a Word document", filepath.Base(path))
	}
	switch {
	case bytes.Equal(head, []byte("%PDF-")):
		return DocumentPDF, nil
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		if isWordDocument(f, info.Size()) {
			return DocumentDOCX, nil
		}
	}
	return "", fmt.Errorf("the document %s is neither a PDF nor a Word document (.docx)", filepath.Base(path))
}

// isWordDocument says whether a zip archive is a Word document: it holds
// word/document.xml.
func isWordDocument(r io.ReaderAt, size int64) bool {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return false
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			return true
		}
	}
	return false
}

// ErrNoDocument is the refusal of a run given a document while
// DocumentHere is false.
var ErrNoDocument = errors.New("the Report Designer here does not read a document: describe it in the brief, or give its pages as reference images")
