package deliverables

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// What a click opens. A file an agent wrote can be one the system would run
// (a shortcut, a script, a macro document), so one click opens only a short
// list of document types, each also checked by its first bytes: a file named
// .pdf that is something else opens nothing. A web page opens in the system's
// browser, from a copy that carries the system's download flag, so the browser
// treats it as a file from the internet. Any other type is only shown in its
// folder.
//
// The value is how the app can show the type itself, without handing it to
// another program: "pdf" (the bundled viewer), "markdown" and "text"
// (rendered or shown as plain text), or "" for a type that only opens
// outside the app.
var openable = map[string]string{
	"pdf": "pdf", "md": "markdown", "txt": "text", "csv": "text", "json": "text",
	"png": "", "jpg": "", "jpeg": "", "docx": "", "xlsx": "", "pptx": "",
	"html": "", "htm": "",
}

// Ext is the lower-case extension of a path without its dot, "" for none.
func Ext(p string) string { return strings.ToLower(strings.TrimPrefix(path.Ext(p), ".")) }

// CanOpen says whether a click opens files of this extension.
func CanOpen(ext string) bool { _, ok := openable[ext]; return ok }

// ViewKind is how the app shows a file of this extension itself: "pdf",
// "markdown" or "text"; "" when it cannot.
func ViewKind(ext string) string { return openable[ext] }

// Allowed lists the extensions a click opens, for a message.
func Allowed() []string {
	return []string{"pdf", "md", "txt", "csv", "json", "png", "jpg", "jpeg", "docx", "xlsx", "pptx", "html", "htm"}
}

// HeadBytes is how much of a file's start CheckType reads.
const HeadBytes = 8192

// ErrNotThatType says a file's bytes are not what its extension names.
var ErrNotThatType = errors.New("its contents are not what its file type says")

// CheckHead says whether head, the first bytes of a file, are what ext names.
func CheckHead(ext string, head []byte) error {
	ok := false
	switch ext {
	case "pdf":
		ok = bytes.HasPrefix(head, []byte("%PDF-"))
	case "png":
		ok = bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n"))
	case "jpg", "jpeg":
		ok = bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF})
	case "docx", "xlsx", "pptx":
		ok = bytes.HasPrefix(head, []byte("PK\x03\x04"))
	case "md", "txt", "csv", "html", "htm":
		ok = looksLikeText(head)
	case "json":
		ok = looksLikeText(head) && startsLikeJSON(head)
	}
	if !ok {
		return ErrNotThatType
	}
	return nil
}

// looksLikeText is valid UTF-8 (a character cut by the end of head is let
// pass) with no NUL byte.
func looksLikeText(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	for i := 0; i < len(head); {
		r, n := utf8.DecodeRune(head[i:])
		if r == utf8.RuneError && n <= 1 {
			return !utf8.FullRune(head[i:]) && len(head)-i < utf8.UTFMax
		}
		i += n
	}
	return true
}

func startsLikeJSON(head []byte) bool {
	t := bytes.TrimPrefix(head, []byte("\xEF\xBB\xBF"))
	t = bytes.TrimLeft(t, " \t\r\n")
	if len(t) == 0 {
		return false
	}
	return strings.IndexByte(`{["-0123456789tfn`, t[0]) >= 0
}

// CheckOffice looks inside an Office document (a zip) for what a plain
// document never holds: a macro project, ActiveX controls or an embedded
// object, each of which can run code when the file is opened.
func CheckOffice(r io.ReaderAt, size int64) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return ErrNotThatType
	}
	hasTypes := false
	for _, f := range zr.File {
		n := strings.ToLower(f.Name)
		switch {
		case n == "[content_types].xml":
			hasTypes = true
		case strings.HasSuffix(n, "vbaproject.bin"), strings.Contains(n, "/activex/"), strings.Contains(n, "/embeddings/"):
			return fmt.Errorf("it carries a macro project, a control or an embedded object")
		}
	}
	if !hasTypes {
		return ErrNotThatType
	}
	return nil
}
