package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/blaktron/agentry-desktop/internal/computeruse/stopkey"
)

// StopKeyName is the key that stops every run's computer use, as the
// person is told it.
const StopKeyName = stopkey.Name

// bannerName is the computer-use banner's window, and bannerTitle its
// title.
const (
	bannerName  = "computer-use"
	bannerTitle = "Agentry: using your computer"
)

// bannerWindow is the small always-on-top window that shows while a run
// uses the computer: who uses what, with Pause and Stop. A window system
// that does not keep a window above the others (Wayland lets the
// compositor decide) shows it as an ordinary window; the tray item shows
// the same either way.
type bannerWindow struct {
	mu sync.Mutex
	w  *application.WebviewWindow
	// key is the stop key while a run uses the computer, and binding says
	// it is being bound.
	key     stopkey.Binding
	binding bool
}

// bindStopKey binds the stop key, and lets it go again if computer use
// ended while it was bound.
func (y *YourComputerAPI) bindStopKey() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	k, err := stopkey.Bind(ctx, y.stopComputerUse)
	cancel()
	y.banner.mu.Lock()
	defer y.banner.mu.Unlock()
	y.banner.binding = false
	switch {
	case err != nil:
		log.Printf("computer use: the stop key %s is not bound: %v (the tray and the banner still stop)", StopKeyName, err)
	case !y.usingComputer():
		_ = k.Close()
	default:
		y.banner.key = k
	}
}

// showBanner opens the banner while a run uses the computer and closes it
// when none does.
func (y *YourComputerAPI) showBanner(context.Context) {
	wails := application.Get()
	if wails == nil {
		return
	}
	using := y.usingComputer()
	y.banner.mu.Lock()
	defer y.banner.mu.Unlock()
	switch {
	case using && !y.banner.binding && y.banner.key == nil:
		// The desktop may ask the person to confirm the shortcut, so it is
		// bound beside the banner, never before it.
		y.banner.binding = true
		go y.bindStopKey()
	case !using && y.banner.key != nil:
		_ = y.banner.key.Close()
		y.banner.key = nil
	}
	switch {
	case using && y.banner.w == nil:
		y.banner.w = wails.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:                       bannerName,
			Title:                      bannerTitle,
			Width:                      560,
			Height:                     72,
			AlwaysOnTop:                true,
			Frameless:                  true,
			DisableResize:              true,
			URL:                        "/#computer-use",
			DefaultContextMenuDisabled: true,
			InitialPosition:            application.WindowXY,
			X:                          40,
			Y:                          40,
		})
		y.banner.w.Show()
		go keepBannerAbove(120)
	case !using && y.banner.w != nil:
		y.banner.w.Close()
		y.banner.w = nil
	}
}
