//go:build !windows

package hostpath

// registryPath: only Windows keeps its PATH in a registry.
func registryPath() []string { return nil }
