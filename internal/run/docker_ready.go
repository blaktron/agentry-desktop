package run

import (
	"context"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/hostpath"
)

// A Desktop VM can take ten seconds to wake. Give each probe time to
// answer, then retry twice before showing its last error. The interval
// is a seam for tests of callers whose engine stays down.
var dockerRetryInterval = 5 * time.Second

// refreshPath gives the process the PATH a terminal opened now would have
// before each probe, so a Docker installed or moved while the app is open
// (or since its first launch, which every self-update inherits) is found
// by Check again without a restart. A seam for tests.
var refreshPath = hostpath.ForGUI

func dockerAvailable(ctx context.Context, docker dockerx.Client) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		refreshPath()
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = docker.Available(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(dockerRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
