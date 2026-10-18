package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// Redeliver sends a finished run's result and deliverables again to the saved
// destinations named, as `agentry deliver RUN-FOLDER --destination NAME` does:
// the same delivery step in the CLI's engine, on this computer's container
// engine, its records added to the run folder's receipts/deliveries.ndjson and
// every other file of the folder left as it was. A name that is not saved, or
// has no signing secret, refuses before anything starts.
func Redeliver(ctx context.Context, runDir string, names []string, logw io.Writer) ([]receipt.Delivery, error) {
	if len(names) == 0 {
		return nil, errors.New("name a destination to send to")
	}
	dests, err := destination.Resolve(names)
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, err
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		return nil, fmt.Errorf("the container engine does not answer, and a delivery runs in a container: %w", err)
	}
	tag := runnerImageTag(pol)
	if err := gateway.EnsureRunnerImage(ctx, docker, pol, tag, filepath.Join(home, "image"), logw); err != nil {
		return nil, err
	}
	return engine.Redeliver(ctx, docker, engine.RedeliverParams{
		RunDir: runDir, Destinations: dests, Policy: pol, RunnerImage: tag, RunRoot: home, Stderr: logw,
	})
}
