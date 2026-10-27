package wasapi

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sdkTable reads testdata/sdk-interfaces.txt: each interface's IID and
// methods in vtable order, and each class's CLSID.
func sdkTable(t *testing.T) (ids map[string]string, methods map[string][]string, lines []string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "sdk-interfaces.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	ids, methods = map[string]string{}, map[string][]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
		fields := strings.Fields(line)
		switch fields[0] {
		case "class":
			ids["class "+fields[1]] = fields[2]
			continue
		case "key":
			ids["key "+fields[1]] = fields[2] + " " + fields[3]
			continue
		}
		ids[fields[0]], methods[fields[0]] = fields[1], fields[2:]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ids, methods, lines
}

// Every slot the package calls is that method's place in the SDK's table,
// and every identifier is the SDK's.
func TestEverySlotAndIdentifierIsTheSDKs(t *testing.T) {
	ids, methods, _ := sdkTable(t)
	for _, c := range calls {
		ms, ok := methods[c.Interface]
		if !ok {
			t.Errorf("%s is not in the SDK table", c.Interface)
			continue
		}
		if c.Slot >= len(ms) || ms[c.Slot] != c.Method {
			t.Errorf("%s.%s is called at slot %d; the SDK has %v", c.Interface, c.Method, c.Slot, ms)
		}
	}
	for name, id := range identifiers {
		if !strings.EqualFold(ids[name], id) {
			t.Errorf("%s is %s here and %s in the SDK", name, id, ids[name])
		}
	}
}

// The table is what the SDK's headers declare. The headers are not kept in
// this repository: the test reads them from the folder
// AGENTRY_WINDOWS_SDK_HEADERS names (mmdeviceapi.h, Audioclient.h,
// audiopolicy.h, propsys.h and functiondiscoverykeys_devpkey.h, as the
// Windows SDK or Microsoft's win32metadata repository carries them), and is
// skipped without it.
func TestTheTableIsTheSDKs(t *testing.T) {
	dir := os.Getenv("AGENTRY_WINDOWS_SDK_HEADERS")
	if dir == "" {
		t.Skip("AGENTRY_WINDOWS_SDK_HEADERS names no folder of SDK headers")
	}
	var all strings.Builder
	for _, name := range []string{"mmdeviceapi.h", "Audioclient.h", "audiopolicy.h", "propsys.h", "functiondiscoverykeys_devpkey.h"} {
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // a folder the person running the test names
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
		all.WriteByte('\n')
	}
	src := all.String()
	_, _, lines := sdkTable(t)
	for _, line := range lines {
		fields := strings.Fields(line)
		if fields[0] == "class" {
			m := regexp.MustCompile(`class DECLSPEC_UUID\("([0-9A-Fa-f-]+)"\)\s*` + fields[1]).FindStringSubmatch(src)
			if m == nil || !strings.EqualFold(m[1], fields[2]) {
				t.Errorf("class %s: the headers say %v, the table %s", fields[1], m, fields[2])
			}
			continue
		}
		if fields[0] == "key" {
			if got := headerPropertyKey(src, fields[1]); !strings.EqualFold(got, fields[2]+" "+fields[3]) {
				t.Errorf("key %s: the headers say %q, the table %s %s", fields[1], got, fields[2], fields[3])
			}
			continue
		}
		name := fields[0]
		m := regexp.MustCompile(`MIDL_INTERFACE\("([0-9A-Fa-f-]+)"\)\s*` + name + `\s*:`).FindStringSubmatch(src)
		if m == nil || !strings.EqualFold(m[1], fields[1]) {
			t.Errorf("%s: the headers' IID is %v, the table's %s", name, m, fields[1])
		}
		v := regexp.MustCompile(`(?s)typedef struct ` + name + `Vtbl\s*\{(.*?)\}\s*` + name + `Vtbl;`).FindStringSubmatch(src)
		if v == nil {
			t.Errorf("%s: the headers declare no table", name)
			continue
		}
		var got []string
		for _, mm := range regexp.MustCompile(`\(\s*STDMETHODCALLTYPE\s*\*\s*(\w+)\s*\)`).FindAllStringSubmatch(v[1], -1) {
			got = append(got, mm[1])
		}
		if strings.Join(got, " ") != strings.Join(fields[2:], " ") {
			t.Errorf("%s: the headers' table is %v, the table's %v", name, got, fields[2:])
		}
	}
}

// headerPropertyKey reads a DEFINE_PROPERTYKEY of the headers as the table writes
// it: the format id as a GUID, then the property id.
func headerPropertyKey(src, name string) string {
	hex := `0x([0-9A-Fa-f]+)`
	m := regexp.MustCompile(`DEFINE_PROPERTYKEY\(\s*` + name + `\s*,\s*` + strings.Repeat(hex+`\s*,\s*`, 11) + `(\d+)\s*\)`).FindStringSubmatch(src)
	if m == nil {
		return ""
	}
	pad := func(s string, n int) string { return strings.Repeat("0", n-len(s)) + s }
	b := make([]string, 8)
	for i := range b {
		b[i] = pad(m[4+i], 2)
	}
	return pad(m[1], 8) + "-" + pad(m[2], 4) + "-" + pad(m[3], 4) + "-" + b[0] + b[1] + "-" + strings.Join(b[2:], "") + " " + m[12]
}

// Every stream is opened in shared mode, so the meeting app keeps its
// device as it had it; only an output endpoint's stream is a loopback.
func TestStreamsAreSharedAndLoopbackOnlyForOutputs(t *testing.T) {
	for _, loopback := range []bool{false, true} {
		for _, convert := range []bool{false, true} {
			mode, flags := initArgs(loopback, convert)
			if mode != shareModeShared {
				t.Errorf("loopback=%v convert=%v: share mode %d, want shared", loopback, convert, mode)
			}
			if (flags&streamFlagsLoopback != 0) != loopback {
				t.Errorf("loopback=%v: flags %#x", loopback, flags)
			}
			if (flags&streamFlagsAutoConvert != 0) != convert {
				t.Errorf("convert=%v: flags %#x", convert, flags)
			}
		}
	}
}
