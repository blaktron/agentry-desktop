package computeruse

import (
	"fmt"
	"strings"
	"unicode"
)

// X keysyms of the keys a computer-use agent names. Every window system
// this package drives takes them: X11 natively, the RemoteDesktop portal
// as its keysym notifications.
const (
	keyReturn    = 0xff0d
	keyTab       = 0xff09
	keyEscape    = 0xff1b
	keyBackSpace = 0xff08
	keyShiftL    = 0xffe1
	keyControlL  = 0xffe3
	keyAltL      = 0xffe9
	keySuperL    = 0xffeb
)

var namedKeys = map[string]uint32{
	"return": keyReturn, "enter": keyReturn, "kp_enter": 0xff8d,
	"tab": keyTab, "iso_left_tab": 0xfe20,
	"escape": keyEscape, "esc": keyEscape,
	"backspace": keyBackSpace,
	"delete":    0xffff, "del": 0xffff,
	"insert": 0xff63,
	"home":   0xff50, "end": 0xff57,
	"page_up": 0xff55, "pageup": 0xff55, "prior": 0xff55,
	"page_down": 0xff56, "pagedown": 0xff56, "next": 0xff56,
	"left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
	"space": 0x20,
	"shift": keyShiftL, "shift_l": keyShiftL, "shift_r": 0xffe2,
	"ctrl": keyControlL, "control": keyControlL, "control_l": keyControlL, "ctrl_l": keyControlL, "control_r": 0xffe4, "ctrl_r": 0xffe4,
	"alt": keyAltL, "alt_l": keyAltL, "alt_r": 0xffea, "option": keyAltL,
	"super": keySuperL, "super_l": keySuperL, "super_r": 0xffec, "meta": keySuperL, "cmd": keySuperL, "command": keySuperL, "win": keySuperL, "hyper": 0xffed,
	"caps_lock": 0xffe5, "menu": 0xff67, "print": 0xff61,
	"minus": '-', "plus": '+', "equal": '=', "comma": ',', "period": '.', "slash": '/', "backslash": '\\',
	"semicolon": ';', "apostrophe": '\'', "grave": '`', "bracketleft": '[', "bracketright": ']',
}

func init() {
	for i := 1; i <= 12; i++ {
		namedKeys[fmt.Sprintf("f%d", i)] = uint32(0xffbe + i - 1)
	}
}

// modifierKeys are the keysyms held during a combination.
var modifierKeys = map[uint32]bool{keyShiftL: true, 0xffe2: true, keyControlL: true, 0xffe4: true, keyAltL: true, 0xffea: true, keySuperL: true, 0xffec: true, 0xffed: true}

// runeKeysym is the keysym that types r: Latin-1 directly, a newline and a
// tab as their keys, and every other character in the Unicode keysym range.
func runeKeysym(r rune) (uint32, error) {
	switch {
	case r == '\n' || r == '\r':
		return keyReturn, nil
	case r == '\t':
		return keyTab, nil
	case r >= 0x20 && r <= 0x7e, r >= 0xa0 && r <= 0xff:
		return uint32(r), nil
	case unicode.IsControl(r) || r > unicode.MaxRune || r < 0:
		return 0, fmt.Errorf("the character %U cannot be typed", r)
	}
	return 0x01000000 | uint32(r), nil
}

// parseKey reads one key name: a named key, or one character. A letter is
// its lower-case key: a combination names shift itself.
func parseKey(name string) (uint32, error) {
	if k, ok := namedKeys[strings.ToLower(name)]; ok {
		return k, nil
	}
	rs := []rune(name)
	if len(rs) == 1 {
		return runeKeysym(unicode.ToLower(rs[0]))
	}
	return 0, fmt.Errorf("%q is not a key this tool knows: name it as Return, Tab, Escape, BackSpace, Delete, Home, End, Page_Up, Page_Down, an arrow (Left, Up, Right, Down), F1 to F12, a modifier (ctrl, shift, alt, super) or one character", name)
}

// parseCombo reads a combination such as "ctrl+shift+s" into its keys, the
// modifiers first, in the order given.
func parseCombo(text string) ([]uint32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("no key named")
	}
	var parts []string
	if text == "+" {
		parts = []string{"+"}
	} else {
		parts = strings.Split(text, "+")
	}
	var keys []uint32
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("%q has an empty key", text)
		}
		k, err := parseKey(p)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if len(keys) > 5 {
		return nil, fmt.Errorf("%q presses more than five keys at once", text)
	}
	return keys, nil
}

// desktopCombo says a combination is one the desktop itself takes rather
// than the focused window: a key with the super key, switching or closing
// windows, the run dialog, ctrl+alt with any key, and the print key. In app
// scope such a key would act outside the window, so it is refused.
func desktopCombo(keys []uint32) bool {
	var ctrl, alt bool
	for _, k := range keys {
		switch k {
		case keySuperL, 0xffec, 0xffed, 0xff61:
			return true
		case keyControlL, 0xffe4:
			ctrl = true
		case keyAltL, 0xffea:
			alt = true
		}
	}
	if ctrl && alt {
		return true
	}
	if ctrl {
		for _, k := range keys {
			if k == keyEscape {
				return true
			}
		}
	}
	if alt {
		for _, k := range keys {
			if k == keyTab || k == 0xfe20 || k == keyEscape || (k >= 0xffbe && k <= 0xffc9) || k == ' ' {
				return true
			}
		}
	}
	return false
}
