package linkservice

// The link socket: <runner home>/s/desktop.sock, owner-only, which a run
// from the CLI on this computer binds over while Agentry Desktop runs. The
// app's own runs bind in process and never use it.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
)

// ErrServedElsewhere says another process answers on the link socket.
var ErrServedElsewhere = errors.New("another process already answers on the link socket")

// Listen opens the link socket in home's socket folder: the folder made
// owner-only, a socket nothing answers on any more removed, one something
// answers on left alone (ErrServedElsewhere), and the new socket made
// owner-only before it is served.
func Listen(home string) (net.Listener, string, error) {
	path := desktop.SocketPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, "", fmt.Errorf("make the runner home's socket folder: %w", err)
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&fs.ModeSocket == 0 {
			return nil, "", fmt.Errorf("%s is not a socket; Agentry Desktop leaves it as it is", path)
		}
		if c, derr := net.DialTimeout("unix", path, time.Second); derr == nil {
			_ = c.Close()
			return nil, "", fmt.Errorf("%w: %s", ErrServedElsewhere, path)
		}
		if err := os.Remove(path); err != nil {
			return nil, "", fmt.Errorf("remove the link socket nothing answers on: %w", err)
		}
	}
	ln, err := listenOwnerOnly(path)
	if err != nil {
		return nil, "", err
	}
	return ln, path, nil
}

// Serve answers the link socket until ctx ends, then removes it.
func (s *Service) Serve(ctx context.Context, ln net.Listener, path string) error {
	defer func() { _ = os.Remove(path) }()
	return desktop.Serve(ctx, ln, s)
}
