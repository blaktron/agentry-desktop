package run

import (
	"errors"
	"fmt"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// SetBudgets writes Run defaults' timeout and max turns into the runner
// policy's budgets, through the same policy package the provider form writes
// with and the CLI reads: `agentry run` then runs with them too. A run's own
// Advanced fields name that one run's budgets, longer or shorter, as the CLI's
// --timeout and --max-turns do. Nothing else in the policy changes; a policy
// that does not exist yet is created from the defaults first, as the provider
// form's write does. It refuses what the CLI's policy.Validate refuses, in the
// app's words, before the file is touched; policy.Save validates again on the
// way out.
func SetBudgets(timeoutSeconds, maxTurns int) error {
	if timeoutSeconds <= 0 {
		return errors.New("the timeout must be at least one second")
	}
	if maxTurns < 0 {
		return errors.New("max turns must be 0 (unlimited) or more")
	}
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	pol.Budgets.TimeoutSeconds = timeoutSeconds
	pol.Budgets.MaxTurns = maxTurns
	return policy.Save(path, pol)
}

// SetMalwareCheck turns the malware check of this computer's runs on or
// off in the runner policy (malwareCheck), so the agentry CLI checks the
// same way. Off, a run checks only the executables among its inputs, and
// only when the QuickAgent declares commands; a run In Agentry is checked
// whatever this says.
func SetMalwareCheck(on bool) error {
	path, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	pol.MalwareCheck = &on
	return policy.Save(path, pol)
}
