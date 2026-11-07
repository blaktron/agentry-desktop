//go:build darwin

package computeruse

// On macOS the command key is the application's own (copy, paste, save,
// command-click), so it is not refused as a desktop key there. What macOS
// itself takes is: switching applications (command-tab), Spotlight and the
// input source (command- or control-space), force quit
// (command-option-escape), screenshots (command-shift-3 to 6), Spaces and
// Mission Control (control with an arrow), locking the screen
// (control-command-q), the keyboard's focus to the menu bar, Dock and
// windows (control with a function key), and the Dock (command-option-d).
// A click with any modifier is the application's.
func init() {
	systemCombo = macCombo
	clickModifierTaken = func(uint32) bool { return false }
}

func macCombo(keys []uint32) bool {
	var cmd, ctrl, alt, shift bool
	for _, k := range keys {
		switch ModifierOf(k) {
		case ModSuper:
			cmd = true
		case ModControl:
			ctrl = true
		case ModAlt:
			alt = true
		case ModShift:
			shift = true
		}
	}
	for _, k := range keys {
		switch {
		case k == 0xff61: // the print key
			return true
		case cmd && (k == keyTab || k == 0xfe20 || k == ' '):
			return true
		case ctrl && k == ' ':
			return true
		case cmd && alt && (k == keyEscape || k == 'd'):
			return true
		case cmd && shift && (k == '3' || k == '4' || k == '5' || k == '6'):
			return true
		case ctrl && (k == 0xff51 || k == 0xff52 || k == 0xff53 || k == 0xff54):
			return true
		case ctrl && cmd && k == 'q':
			return true
		case ctrl && k >= 0xffbe && k <= 0xffc9:
			return true
		}
	}
	return false
}
