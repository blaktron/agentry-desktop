package run

// The two files that hold an engine's virtual machine size where no command
// sets it: Windows' %UserProfile%\.wslconfig, whose [wsl2] memory= sizes
// the WSL 2 virtual machine every WSL engine runs in, and Docker Desktop's
// settings-store.json, whose memoryMiB sizes its own machine. Each is
// raised only, and everything else in it is kept as it was.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// raiseWSLMemory returns .wslconfig's content with [wsl2] memory= at least
// gib GB, and whether it changed. The other sections, keys and comments are
// kept, and so are the file's line endings (CRLF for a new file). A memory
// value that cannot be read is replaced; one at or above gib is kept.
func raiseWSLMemory(content string, gib int) (string, bool) {
	nl := "\r\n"
	if content != "" && !strings.Contains(content, "\r\n") {
		nl = "\n"
	}
	want := fmt.Sprintf("memory=%dGB", gib)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	section, wslAt, memAt := "", -1, -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = strings.ToLower(strings.TrimSpace(t[1 : len(t)-1]))
			if section == "wsl2" && wslAt < 0 {
				wslAt = i
			}
			continue
		}
		if section != "wsl2" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok && strings.EqualFold(strings.TrimSpace(k), "memory") {
			memAt = i
		}
	}
	switch {
	case memAt >= 0:
		_, v, _ := strings.Cut(lines[memAt], "=")
		if have, ok := parseWSLSize(v); ok && have >= uint64(gib)<<30 {
			return content, false
		}
		lines[memAt] = want
	case wslAt >= 0:
		lines = append(lines[:wslAt+1], append([]string{want}, lines[wslAt+1:]...)...)
	default:
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "[wsl2]", want)
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return strings.ReplaceAll(out, "\n", nl), true
}

// parseWSLSize reads a .wslconfig size: a whole number of bytes, or with
// KB, MB, GB or TB (B optional), case ignored.
func parseWSLSize(v string) (uint64, bool) {
	v = strings.ToUpper(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "#;"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	v = strings.TrimSuffix(v, "B")
	mult := uint64(1)
	for _, u := range []struct {
		suffix string
		mult   uint64
	}{{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"T", 1 << 40}} {
		if strings.HasSuffix(v, u.suffix) {
			v, mult = strings.TrimSpace(strings.TrimSuffix(v, u.suffix)), u.mult
			break
		}
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return n * mult, true
}

// errNoMemoryKey is a Docker Desktop settings file that names no memory:
// its size is then set in Docker Desktop's own settings.
var errNoMemoryKey = errors.New("the settings file of Docker Desktop names no memoryMiB")

// raiseDockerDesktopMemory returns Docker Desktop's settings with memoryMiB
// at least mib, and whether it changed. Every other setting is kept; the
// file is written without a byte-order mark, which Docker Desktop does not
// read.
func raiseDockerDesktopMemory(content []byte, mib int) ([]byte, bool, error) {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(content, &m); err != nil {
		return nil, false, fmt.Errorf("the settings file of Docker Desktop: %w", err)
	}
	raw, ok := m["memoryMiB"]
	if !ok {
		return nil, false, errNoMemoryKey
	}
	var have float64
	if err := json.Unmarshal(raw, &have); err != nil {
		return nil, false, fmt.Errorf("the memoryMiB of Docker Desktop: %w", err)
	}
	if have >= float64(mib) {
		return content, false, nil
	}
	m["memoryMiB"] = json.RawMessage(strconv.Itoa(mib))
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}
