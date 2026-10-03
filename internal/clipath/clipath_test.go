package clipath

import (
	"strings"
	"testing"
)

// expand stands in for Windows' ExpandEnvironmentStrings in these tests.
func expand(s string) string {
	return strings.NewReplacer(`%LOCALAPPDATA%`, `C:\Users\Pat Q\AppData\Local`, `%USERPROFILE%`, `C:\Users\Pat Q`).Replace(s)
}

const dir = `C:\Users\Pat Q\AppData\Local\Programs\Agentry Desktop\bin`

func TestWithEntryAppendsOnceAndKeepsEverythingElse(t *testing.T) {
	cases := []struct{ in, want string }{
		{``, dir},
		{`C:\a;%USERPROFILE%\bin`, `C:\a;%USERPROFILE%\bin;` + dir},
		{`C:\a;`, `C:\a;` + dir},
	}
	for _, c := range cases {
		got, changed := WithEntry(c.in, dir, expand)
		if got != c.want || !changed {
			t.Errorf("WithEntry(%q) = %q, %v; want %q, true", c.in, got, changed, c.want)
		}
		// Idempotent: the second run changes nothing.
		again, changed := WithEntry(got, dir, expand)
		if again != got || changed {
			t.Errorf("WithEntry twice on %q changed it to %q", got, again)
		}
	}
}

func TestWithEntrySeesTheDirectoryHoweverItIsSpelled(t *testing.T) {
	for _, p := range []string{
		strings.ToUpper(dir),
		dir + `\`,
		`%LOCALAPPDATA%\Programs\Agentry Desktop\bin`,
		`C:\x;` + dir + `;C:\y`,
	} {
		if got, changed := WithEntry(p, dir, expand); changed || got != p {
			t.Errorf("WithEntry(%q) changed it to %q; it already names the directory", p, got)
		}
	}
}

func TestWithoutEntryRemovesOnlyTheAppsDirectory(t *testing.T) {
	in := `C:\a;;` + dir + `;%USERPROFILE%\bin;%LOCALAPPDATA%\Programs\Agentry Desktop\bin\`
	got, changed := WithoutEntry(in, dir, expand)
	want := `C:\a;;%USERPROFILE%\bin`
	if got != want || !changed {
		t.Fatalf("WithoutEntry = %q, %v; want %q, true", got, changed, want)
	}
	if again, changed := WithoutEntry(got, dir, expand); changed || again != got {
		t.Fatalf("WithoutEntry on a path without the directory changed it to %q", again)
	}
	// A parent or a sibling of the directory is not the directory.
	for _, p := range []string{`C:\Users\Pat Q\AppData\Local\Programs\Agentry Desktop`, dir + `2`, `C:\Users\Pat Q\AppData\Local\Programs\agentry\bin`} {
		if got, changed := WithoutEntry(p, dir, expand); changed || got != p {
			t.Errorf("WithoutEntry(%q) = %q; it does not name the directory", p, got)
		}
	}
}

func TestAnEmptyEntryNamesNothing(t *testing.T) {
	if same("", "", expand) || same(" ", dir, expand) {
		t.Fatal("an empty entry matched")
	}
}
