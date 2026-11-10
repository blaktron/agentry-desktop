package app

import (
	"log/slog"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventTextSize tells the page the window's text size changed from outside
// it (the macOS View menu), with the new size in percent.
const eventTextSize = "accessibility:text-size"

// windowZoom puts a text size, in percent, on the main window: the
// webview's own zoom, which enlarges text and layout together. The
// runtime's, a test's seam.
var windowZoom = func(percent int) {
	if w := mainWindow(); w != nil {
		w.SetZoom(float64(percent) / 100)
	}
}

// TextSize is Settings → Home → Accessibility's text size, in percent.
func (h *HomeAPI) TextSize() (int, error) { return settings.TextSizeNow() }

// TextSizes are the sizes Settings offers, in order.
func (h *HomeAPI) TextSizes() []int { return settings.TextSizes }

// SetTextSize keeps a text size and puts it on the window at once.
func (h *HomeAPI) SetTextSize(percent int) (int, error) {
	if err := settings.SetTextSize(percent); err != nil {
		return 0, err
	}
	windowZoom(percent)
	return percent, nil
}

// StepTextSize moves the text size one offered size larger (1) or smaller
// (-1), or back to the default (0): the zoom keys. It answers the new size.
func (h *HomeAPI) StepTextSize(step int) (int, error) { return stepTextSize(step) }

func stepTextSize(step int) (int, error) {
	now, err := settings.TextSizeNow()
	if err != nil {
		return 0, err
	}
	next := settings.TextSizeStep(now, step)
	if err := settings.SetTextSize(next); err != nil {
		return 0, err
	}
	windowZoom(next)
	return next, nil
}

// Motion is Settings → Home → Accessibility's motion choice: "system" to
// follow the computer's reduced-motion setting, "reduce" to reduce motion
// whatever it says.
func (h *HomeAPI) Motion() (string, error) { return settings.MotionNow() }

// SetMotion keeps the motion choice; the page applies it.
func (h *HomeAPI) SetMotion(choice string) error { return settings.SetMotion(choice) }

// StepTextSize is the View menu's Zoom In (1), Zoom Out (-1) and Actual
// Size (0): the same steps as the page's zoom keys, told to the page.
func (s *Services) StepTextSize(step int) {
	size, err := stepTextSize(step)
	if err != nil {
		slog.Warn("text size not changed", "err", err)
		return
	}
	emitEvent(s.app, eventTextSize, size)
}

// applyTextSize puts the stored text size on the window as the app starts.
func applyTextSize() {
	if percent, err := settings.TextSizeNow(); err == nil && percent != settings.TextSizeDefault {
		windowZoom(percent)
	}
}
