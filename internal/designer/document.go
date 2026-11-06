package designer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blaktron/agentry-cli/pkg/reportdesigner/build"
)

// A Design may start from one document the person already has, a PDF or a
// Word document, which the Report Designer reads in full and keeps only for
// the run: it goes through the decode step as any input does, and the
// Designer derives the palette, faces, header, footer, cover and logo from
// what it sees. DocumentHere says the Report Designer this app carries
// reads one.
const DocumentHere = true

// MaxDocumentBytes caps the document, as the CLI does.
const MaxDocumentBytes = build.MaxDocumentBytes

// The two kinds of document a Design takes.
const (
	DocumentPDF  = "pdf"
	DocumentDOCX = "docx"
)

// CheckDocument holds a picked document to the CLI's own rule before a run
// reads it: one regular file, not empty, at most 20 MB, and a PDF or a Word
// document as its bytes and its name say (build.DocumentKind). It answers
// the kind.
func CheckDocument(path string) (string, error) {
	d, err := ReadDocument(path)
	if err != nil {
		return "", err
	}
	kind, err := build.DocumentKind(d.Name, d.Data)
	if err != nil {
		return "", fmt.Errorf("the document %s: %w", d.Name, err)
	}
	return string(kind), nil
}

// ReadDocument reads a picked document within its cap, as the run is given
// it: its file name and its bytes.
func ReadDocument(path string) (*build.Document, error) {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("the document: %w", err)
	}
	switch {
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("the document %s is not a file", name)
	case info.Size() > MaxDocumentBytes:
		return nil, fmt.Errorf("the document %s is over %d MB", name, MaxDocumentBytes>>20)
	case info.Size() == 0:
		return nil, fmt.Errorf("the document %s is empty", name)
	}
	f, err := os.Open(path) //nolint:gosec // a file a native picker returned
	if err != nil {
		return nil, fmt.Errorf("the document: %w", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the document %s: %w", name, err)
	}
	if len(b) > MaxDocumentBytes {
		return nil, fmt.Errorf("the document %s is over %d MB", name, MaxDocumentBytes>>20)
	}
	return &build.Document{Name: name, Data: b}, nil
}
