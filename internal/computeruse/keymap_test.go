package computeruse

import "testing"

// A named key is its key on Windows and macOS; a character is typed as
// itself unless a modifier is held, when the US layout's key carries it.
func TestTheKeyMapsOfWindowsAndMacOS(t *testing.T) {
	ret, _ := parseKey("Return")
	if k, ok := WindowsKeyOf(ret, false); !ok || k.VK != 0x0d {
		t.Errorf("Return on Windows = %+v %v", k, ok)
	}
	if k, ok := MacKeyOf(ret, false); !ok || k != 0x24 {
		t.Errorf("Return on macOS = %#x %v", k, ok)
	}
	left, _ := parseKey("Left")
	if k, _ := WindowsKeyOf(left, false); !k.Extended || k.VK != 0x25 {
		t.Errorf("Left on Windows = %+v", k)
	}
	f12, _ := parseKey("F12")
	if k, _ := WindowsKeyOf(f12, false); k.VK != 0x7b {
		t.Errorf("F12 on Windows = %+v", k)
	}
	if k, _ := MacKeyOf(f12, false); k != 0x6f {
		t.Errorf("F12 on macOS = %#x", k)
	}
	c, _ := parseKey("c")
	if _, ok := WindowsKeyOf(c, false); ok {
		t.Error("a character with no modifier held is a key on Windows, not typed text")
	}
	if k, ok := WindowsKeyOf(c, true); !ok || k.VK != 'C' {
		t.Errorf("ctrl+c's c on Windows = %+v %v", k, ok)
	}
	if k, ok := MacKeyOf(c, true); !ok || k != 0x08 {
		t.Errorf("cmd+c's c on macOS = %#x %v", k, ok)
	}
	if k, ok := WindowsKeyOf('/', true); !ok || k.VK != 0xbf {
		t.Errorf("ctrl+/ on Windows = %+v %v", k, ok)
	}
	e, _ := runeKeysym('é')
	if r, ok := KeysymRune(e); !ok || r != 'é' {
		t.Errorf("é = %q %v", r, ok)
	}
	smile, _ := runeKeysym('😀')
	if r, ok := KeysymRune(smile); !ok || r != '😀' {
		t.Errorf("an emoji = %q %v", r, ok)
	}
	if _, ok := KeysymRune(ret); ok {
		t.Error("Return is a character")
	}
	for _, m := range []string{"shift", "ctrl", "alt", "cmd"} {
		k, _ := parseKey(m)
		if !IsModifier(k) || ModifierOf(k) == 0 {
			t.Errorf("%s is not a modifier", m)
		}
		if _, ok := WindowsKeyOf(k, false); !ok {
			t.Errorf("%s has no key on Windows", m)
		}
		if _, ok := MacKeyOf(k, false); !ok {
			t.Errorf("%s has no key on macOS", m)
		}
	}
}
