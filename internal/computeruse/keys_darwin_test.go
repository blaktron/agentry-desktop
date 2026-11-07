//go:build darwin

package computeruse

import "testing"

// On macOS command combinations are the application's, and only what
// macOS itself takes is refused in app scope.
func TestMacOSRefusesOnlyItsOwnCombinations(t *testing.T) {
	for combo, taken := range map[string]bool{
		"cmd+c": false, "cmd+v": false, "cmd+s": false, "cmd+shift+z": false, "ctrl+a": false,
		"cmd+Tab": true, "cmd+space": true, "ctrl+space": true, "cmd+alt+Escape": true,
		"cmd+shift+4": true, "ctrl+Left": true, "ctrl+cmd+q": true, "ctrl+F2": true,
	} {
		keys, err := parseCombo(combo)
		if err != nil {
			t.Fatalf("%s: %v", combo, err)
		}
		if got := systemCombo(keys); got != taken {
			t.Errorf("%s: taken = %v, want %v", combo, got, taken)
		}
	}
	if clickModifierTaken(keySuperL) {
		t.Error("a command-click is refused")
	}
}
