package computeruse

// The keys of the window systems that take no X keysym: Windows' virtual-key
// codes and macOS's virtual key codes. A named key (Return, an arrow, a
// modifier, F1 to F12) maps to its key; a character is typed as itself
// (KeysymRune), except while a modifier is held, when a combination such as
// ctrl+c needs the key that carries the character: the letters, digits and
// punctuation of a US layout.

// KeysymRune is the character a keysym types, for keysyms of one
// character: Latin-1 and the Unicode keysym range.
func KeysymRune(keysym uint32) (rune, bool) {
	switch {
	case keysym >= 0x20 && keysym <= 0x7e, keysym >= 0xa0 && keysym <= 0xff:
		return rune(keysym), true
	case keysym&0xff000000 == 0x01000000:
		return rune(keysym & 0x00ffffff), true
	}
	return 0, false
}

// IsModifier reports a modifier keysym: shift, control, alt, super, hyper.
func IsModifier(keysym uint32) bool { return modifierKeys[keysym] }

// Modifier kinds, for the flags a window system sets on an event.
const (
	ModShift = 1 << iota
	ModControl
	ModAlt
	ModSuper
)

// ModifierOf is the kind of a modifier keysym, 0 for any other key.
func ModifierOf(keysym uint32) int {
	switch keysym {
	case keyShiftL, 0xffe2:
		return ModShift
	case keyControlL, 0xffe4:
		return ModControl
	case keyAltL, 0xffea:
		return ModAlt
	case keySuperL, 0xffec, 0xffed:
		return ModSuper
	}
	return 0
}

// WindowsKey is a Windows virtual-key code for a keysym, and whether it is
// an extended key (KEYEVENTF_EXTENDEDKEY). Characters other than the US
// layout's letters, digits and space are not keys here: they are typed as
// themselves.
type WindowsKey struct {
	VK       uint16
	Extended bool
}

var windowsKeys = map[uint32]WindowsKey{
	keyReturn: {VK: 0x0d}, 0xff8d: {VK: 0x0d, Extended: true},
	keyTab: {VK: 0x09}, 0xfe20: {VK: 0x09},
	keyEscape: {VK: 0x1b}, keyBackSpace: {VK: 0x08},
	0xffff: {VK: 0x2e, Extended: true}, 0xff63: {VK: 0x2d, Extended: true},
	0xff50: {VK: 0x24, Extended: true}, 0xff57: {VK: 0x23, Extended: true},
	0xff55: {VK: 0x21, Extended: true}, 0xff56: {VK: 0x22, Extended: true},
	0xff51: {VK: 0x25, Extended: true}, 0xff52: {VK: 0x26, Extended: true},
	0xff53: {VK: 0x27, Extended: true}, 0xff54: {VK: 0x28, Extended: true},
	0x20:      {VK: 0x20},
	keyShiftL: {VK: 0xa0}, 0xffe2: {VK: 0xa1},
	keyControlL: {VK: 0xa2}, 0xffe4: {VK: 0xa3, Extended: true},
	keyAltL: {VK: 0xa4}, 0xffea: {VK: 0xa5, Extended: true},
	keySuperL: {VK: 0x5b, Extended: true}, 0xffec: {VK: 0x5c, Extended: true},
	0xffe5: {VK: 0x14}, 0xff67: {VK: 0x5d, Extended: true}, 0xff61: {VK: 0x2c, Extended: true},
}

// windowsCharKeys are the US layout's keys for punctuation in a
// combination: VK_OEM_*.
var windowsCharKeys = map[rune]uint16{
	'-': 0xbd, '=': 0xbb, ',': 0xbc, '.': 0xbe, '/': 0xbf, ';': 0xba, '\'': 0xde,
	'`': 0xc0, '[': 0xdb, ']': 0xdd, '\\': 0xdc,
}

func init() {
	for i := uint32(0); i < 12; i++ {
		windowsKeys[0xffbe+i] = WindowsKey{VK: uint16(0x70 + i)}
	}
}

// WindowsKeyOf is the key for a keysym: a named key always; a character
// only when combo is set (a modifier is held), from the US layout's letters,
// digits and punctuation.
func WindowsKeyOf(keysym uint32, combo bool) (WindowsKey, bool) {
	if k, ok := windowsKeys[keysym]; ok {
		return k, true
	}
	if !combo {
		return WindowsKey{}, false
	}
	r, ok := KeysymRune(keysym)
	switch {
	case !ok:
		return WindowsKey{}, false
	case r >= 'a' && r <= 'z':
		return WindowsKey{VK: uint16(r - 'a' + 'A')}, true
	case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return WindowsKey{VK: uint16(r)}, true
	}
	if vk, ok := windowsCharKeys[r]; ok {
		return WindowsKey{VK: vk}, true
	}
	return WindowsKey{}, false
}

// macKeys are macOS's virtual key codes (kVK_*) for named keys.
var macKeys = map[uint32]uint16{
	keyReturn: 0x24, 0xff8d: 0x4c, keyTab: 0x30, 0xfe20: 0x30,
	keyEscape: 0x35, keyBackSpace: 0x33, 0xffff: 0x75, 0xff63: 0x72,
	0xff50: 0x73, 0xff57: 0x77, 0xff55: 0x74, 0xff56: 0x79,
	0xff51: 0x7b, 0xff52: 0x7e, 0xff53: 0x7c, 0xff54: 0x7d,
	0x20:      0x31,
	keyShiftL: 0x38, 0xffe2: 0x3c, keyControlL: 0x3b, 0xffe4: 0x3e,
	keyAltL: 0x3a, 0xffea: 0x3d, keySuperL: 0x37, 0xffec: 0x36,
	0xffe5: 0x39,
	0xffbe: 0x7a, 0xffbf: 0x78, 0xffc0: 0x63, 0xffc1: 0x76, 0xffc2: 0x60, 0xffc3: 0x61,
	0xffc4: 0x62, 0xffc5: 0x64, 0xffc6: 0x65, 0xffc7: 0x6d, 0xffc8: 0x67, 0xffc9: 0x6f,
}

// macCharKeys are the US (ANSI) layout's keys for characters in a
// combination.
var macCharKeys = map[rune]uint16{
	'a': 0x00, 's': 0x01, 'd': 0x02, 'f': 0x03, 'h': 0x04, 'g': 0x05, 'z': 0x06, 'x': 0x07,
	'c': 0x08, 'v': 0x09, 'b': 0x0b, 'q': 0x0c, 'w': 0x0d, 'e': 0x0e, 'r': 0x0f, 'y': 0x10,
	't': 0x11, '1': 0x12, '2': 0x13, '3': 0x14, '4': 0x15, '6': 0x16, '5': 0x17, '=': 0x18,
	'9': 0x19, '7': 0x1a, '-': 0x1b, '8': 0x1c, '0': 0x1d, ']': 0x1e, 'o': 0x1f, 'u': 0x20,
	'[': 0x21, 'i': 0x22, 'p': 0x23, 'l': 0x25, 'j': 0x26, '\'': 0x27, 'k': 0x28, ';': 0x29,
	'\\': 0x2a, ',': 0x2b, '/': 0x2c, 'n': 0x2d, 'm': 0x2e, '.': 0x2f, '`': 0x32,
}

// MacKeyOf is macOS's virtual key code for a keysym: a named key always; a
// character only when combo is set, from the US layout.
func MacKeyOf(keysym uint32, combo bool) (uint16, bool) {
	if k, ok := macKeys[keysym]; ok {
		return k, true
	}
	if !combo {
		return 0, false
	}
	r, ok := KeysymRune(keysym)
	if !ok {
		return 0, false
	}
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	k, ok := macCharKeys[r]
	return k, ok
}
