//go:build windows

package hostpath

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// registryPath is the PATH Windows would give a process started now: the
// machine's Path, then the user's, each expanded (both are usually
// REG_EXPAND_SZ). A key that cannot be read contributes nothing.
func registryPath() []string {
	var dirs []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		dirs = append(dirs, readPath(k.root, k.path)...)
	}
	return dirs
}

func readPath(root registry.Key, path string) []string {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = key.Close() }()
	v, typ, err := key.GetStringValue("Path")
	if err != nil {
		return nil
	}
	if typ == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
	}
	return strings.Split(v, ";")
}
