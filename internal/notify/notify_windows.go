package notify

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"git.sr.ht/~jackmordaunt/go-toast/v2/wintoast"
	"golang.org/x/sys/windows/registry"
)

// The app's identity to Windows' notifications: its AppUserModelID,
// registered under HKCU\Software\Classes\AppUserModelId with the name and
// icon a toast shows, and the class Windows calls back on a click while the
// app runs. The uninstaller removes the key.
const (
	appUserModelID = "SecureAgentry.AgentryDesktop"
	activatorGUID  = "{CECEF98B-F4FD-4B39-97F5-4AF15BDD6290}"
	displayName    = "Agentry Desktop"
)

//go:embed icons/notify.png
var iconPNG []byte

var (
	initOnce sync.Once
	initErr  error
	// pushes runs every toast on one OS thread, the thread the Windows
	// Runtime was initialised on.
	pushes = make(chan func())
)

// Init registers the app's AppUserModelID and its click handler, once.
func Init() error {
	initOnce.Do(func() {
		go func() {
			runtime.LockOSThread()
			for f := range pushes {
				f()
			}
		}()
		// The toast's icon is a file: the app's icon, written under the
		// local app data folder, which the uninstaller removes.
		icon := ""
		if dir, err := os.UserCacheDir(); err == nil {
			p := filepath.Join(dir, "agentry-desktop", "notify.png")
			if os.MkdirAll(filepath.Dir(p), 0o700) == nil && os.WriteFile(p, iconPNG, 0o600) == nil {
				icon = p
			}
		}
		if initErr = wintoast.SetAppData(wintoast.AppData{AppID: appUserModelID, GUID: activatorGUID, IconPath: icon}); initErr != nil {
			return
		}
		wintoast.SetActivationCallback(func(_, args string, _ []wintoast.UserData) { clicked(args) })
		// The library names the app by its id; a toast's header shows this
		// name.
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appUserModelID, registry.SET_VALUE)
		if err != nil {
			initErr = err
			return
		}
		defer func() { _ = k.Close() }()
		initErr = k.SetStringValue("DisplayName", displayName)
	})
	return initErr
}

func show(n Notice) error {
	if err := Init(); err != nil {
		return fmt.Errorf("the app is not registered with Windows for notifications: %w", err)
	}
	done := make(chan error, 1)
	// No PowerShell fallback: a toast this app raises is its own or none.
	pushes <- func() { done <- wintoast.Push(appUserModelID, toastXML(n)) }
	if err := <-done; err != nil {
		return fmt.Errorf("the notification was refused: %w", err)
	}
	return nil
}
