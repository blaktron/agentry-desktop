package run

import (
	"context"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// A Desktop VM can take ten seconds to wake. Give each probe time to
// answer, then retry twice before showing its last error (desktop#594).
// The interval is a seam for tests of callers whose engine stays down.
var dockerRetryInterval = 5 * time.Second

func dockerAvailable(ctx context.Context, docker dockerx.Client) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
