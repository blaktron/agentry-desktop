package loginitem

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// runKey is the current user's Run key; a test points it elsewhere.
var runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// Enable sets the Run key's value that launches exe hidden at sign-in.
func Enable(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringValue(Name, runValue(exe))
}

// Disable deletes the value; none is not an error.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.DeleteValue(Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// Enabled reports whether the value is there and launches exe.
func Enabled(exe string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(Name)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == runValue(exe), nil
}
