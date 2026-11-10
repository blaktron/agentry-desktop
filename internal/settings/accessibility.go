package settings

import (
	"fmt"
	"slices"
)

// The text sizes Settings → Home → Accessibility offers, in percent of the
// default; the window's zoom is the size over 100.
var TextSizes = []int{100, 110, 125, 150, 175, 200}

// TextSizeDefault is the size with nothing stored.
const TextSizeDefault = 100

// TextSizeNow is the stored text size, the default when none is stored or
// the stored one is not a size this app offers (a newer app's).
func TextSizeNow() (int, error) {
	s, err := Load()
	if err != nil {
		return 0, err
	}
	if !slices.Contains(TextSizes, s.TextSize) {
		return TextSizeDefault, nil
	}
	return s.TextSize, nil
}

// SetTextSize stores one of TextSizes; the default is stored as none.
func SetTextSize(percent int) error {
	if !slices.Contains(TextSizes, percent) {
		return fmt.Errorf("%d%% is not a text size Agentry Desktop offers", percent)
	}
	s, err := Load()
	if err != nil {
		return err
	}
	if percent == TextSizeDefault {
		percent = 0
	}
	if s.TextSize == percent {
		return nil
	}
	s.TextSize = percent
	return Save(s)
}

// TextSizeStep is the size one step from the given one: the next larger
// for a positive step, the next smaller for a negative one, held at the
// ends, and the default for a step of 0. A size not offered steps from the
// default.
func TextSizeStep(from, step int) int {
	if step == 0 {
		return TextSizeDefault
	}
	i := slices.Index(TextSizes, from)
	if i < 0 {
		i = slices.Index(TextSizes, TextSizeDefault)
	}
	switch {
	case step > 0 && i < len(TextSizes)-1:
		i++
	case step < 0 && i > 0:
		i--
	}
	return TextSizes[i]
}

// The motion choices: follow the computer's reduced-motion setting, or
// reduce motion whatever it says.
const (
	MotionSystem = "system"
	MotionReduce = "reduce"
)

// MotionNow is the stored motion choice, System when none is stored or the
// stored one is not a choice this app has.
func MotionNow() (string, error) {
	s, err := Load()
	if err != nil {
		return "", err
	}
	if s.Motion != MotionReduce {
		return MotionSystem, nil
	}
	return s.Motion, nil
}

// SetMotion stores a motion choice; System is stored as none.
func SetMotion(choice string) error {
	if choice != MotionSystem && choice != MotionReduce {
		return fmt.Errorf("%q is not a motion choice Agentry Desktop has", choice)
	}
	s, err := Load()
	if err != nil {
		return err
	}
	if choice == MotionSystem {
		choice = ""
	}
	if s.Motion == choice {
		return nil
	}
	s.Motion = choice
	return Save(s)
}
