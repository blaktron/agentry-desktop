//go:build windows

package clipath

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const cliName = "agentry.exe"

func expandEnv(s string) string {
	out, err := registry.ExpandString(s)
	if err != nil {
		return s
	}
	return out
}

// editUserPath reads HKCU\Environment\Path unexpanded, applies edit, and
// writes the result back with the type it had (REG_EXPAND_SZ for a Path that
// did not exist), then tells running programs the environment changed, so a
// terminal opened from Explorer afterwards sees it.
func editUserPath(edit func(string) (string, bool)) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf(`open HKCU\Environment: %w`, err)
	}
	defer func() { _ = k.Close() }()
	cur, typ, err := k.GetStringValue("Path")
	switch {
	case errors.Is(err, registry.ErrNotExist):
		cur, typ = "", registry.EXPAND_SZ
	case err != nil:
		return false, fmt.Errorf("read the user Path: %w", err)
	}
	next, changed := edit(cur)
	if !changed {
		return false, nil
	}
	if typ == registry.SZ {
		err = k.SetStringValue("Path", next)
	} else {
		err = k.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return false, fmt.Errorf("write the user Path: %w", err)
	}
	broadcastEnvironmentChange()
	return true, nil
}

// broadcastEnvironmentChange sends WM_SETTINGCHANGE "Environment" to every
// top-level window, as the System Properties dialog does, waiting at most
// five seconds for a window that does not answer. Best effort: the Path is
// already written, and a new sign-in reads it either way.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
