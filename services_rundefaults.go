package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// RunDefaultsView is Settings → Run defaults (plan settings-cards.md D2,
// D13; agentry-desktop#347). The timeout and max turns are the runner
// policy's budgets, which the CLI runs with too; where runs go, the output
// folder and the Chat idle timeout are this app's own, in desktop.yaml. A
// broken policy file is said, not hidden behind the defaults, as Settings'
// status says it.
type RunDefaultsView struct {
	TimeoutSeconds int    `json:"timeoutSeconds"`
	MaxTurns       int    `json:"maxTurns"`
	PolicyPath     string `json:"policyPath"`
	// Where is settings.RunWhereHere or settings.RunWhereAgentry.
	Where string `json:"where"`
	// OutputDir is the saved output folder, "" for none.
	OutputDir string `json:"outputDir,omitempty"`
	// ChatIdleTimeoutSeconds is the Chat idle timeout (desktop.yaml, D32;
	// agentry-desktop#468), 0 for none: a chat on this computer has no
	// limit but this one.
	ChatIdleTimeoutSeconds int `json:"chatIdleTimeoutSeconds"`
	// UnsafeMode is the switch (desktop.yaml's unsafeMode, D-050), written
	// only by RunAPI.SetUnsafeMode; UnsafeImage the image its runs' commands
	// are installed on, the runner policy's images.unsafe or the pinned
	// default.
	UnsafeMode  bool   `json:"unsafeMode"`
	UnsafeImage string `json:"unsafeImage,omitempty"`
	// BuiltinNoTimeout is the Built-in agents switch (desktop.yaml's
	// builtinNoTimeout, D-062): the built-in agents' runs on this computer
	// have no timeout while it is on.
	BuiltinNoTimeout bool `json:"builtinNoTimeout"`
}

// RunDefaults reads the defaults. With no policy yet the budgets are
// the ones a first run would write.
func (s *SettingsAPI) RunDefaults() (*RunDefaultsView, error) { return runDefaultsView() }

// SetRunBudgets writes the default timeout and max turns into the runner
// policy (run.SetBudgets), refused where the CLI would refuse them.
func (s *SettingsAPI) SetRunBudgets(timeoutSeconds, maxTurns int) (*RunDefaultsView, error) {
	if err := run.SetBudgets(timeoutSeconds, maxTurns); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// SetRunWhere records where the run setup's Where step starts. It is one of
// two words; anything else is refused and the file is left as it was.
func (s *SettingsAPI) SetRunWhere(where string) (*RunDefaultsView, error) {
	if where != settings.RunWhereHere && where != settings.RunWhereAgentry {
		return nil, fmt.Errorf("where to run is %q or %q, not %q", settings.RunWhereHere, settings.RunWhereAgentry, where)
	}
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.RunWhere = where
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// SetChatIdleTimeout records the Chat idle timeout for the next chat on this
// computer, 0 for none. Anything but none or whole minutes up to a week is
// refused, and the file is left as it was.
func (s *SettingsAPI) SetChatIdleTimeout(seconds int) (*RunDefaultsView, error) {
	if err := settings.CheckChatIdleTimeout(seconds); err != nil {
		return nil, err
	}
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.ChatIdleTimeoutSeconds = seconds
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// SetBuiltinNoTimeout turns the Built-in agents switch on or off
// (agentry-desktop#606, D-062): while it is on, the Drafter, the Knowledge
// Creator, the Examiner and the Voice Writer run on this computer with no
// timeout. It asks no confirmation: it gives no run a reach it lacked, the
// turn and cost caps still hold, and a run that has started keeps the
// budgets it started with.
func (s *SettingsAPI) SetBuiltinNoTimeout(on bool) (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.BuiltinNoTimeout = on
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// chatIdleTimeout is Run defaults' Chat idle timeout, 0 for none. A
// settings file that cannot be read, or holds a value its save would refuse,
// refuses the chat: a person who chose an idle timeout is told, not given a
// chat without one.
func chatIdleTimeout() (int, error) {
	st, err := settings.Load()
	if err != nil {
		return 0, err
	}
	if err := settings.CheckChatIdleTimeout(st.ChatIdleTimeoutSeconds); err != nil {
		return 0, fmt.Errorf("%w: set it again in Settings → Run defaults", err)
	}
	return st.ChatIdleTimeoutSeconds, nil
}

// ChooseDefaultOutputDir opens the native picker for the default output
// folder and saves what it returned. The binding takes no path, so the
// folder is the picker's and never the page's (D-030), and it is not added
// to this session's picked set: it is an output folder, admitted at run time
// from this file (App.requireOutputDir, D-044), never an input root. A
// cancelled picker changes nothing.
func (s *SettingsAPI) ChooseDefaultOutputDir() (*RunDefaultsView, error) {
	dir, err := s.app.pickDirectoryUnmarked("Choose the default output folder for runs")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		st, err := settings.Load()
		if err != nil {
			return nil, err
		}
		st.DefaultOutputDir = dir
		if err := settings.Save(st); err != nil {
			return nil, err
		}
	}
	return runDefaultsView()
}

// ClearDefaultOutputDir forgets the saved output folder; runs go to the
// default the CLI uses again, and the folder is admitted only when picked.
func (s *SettingsAPI) ClearDefaultOutputDir() (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.DefaultOutputDir = ""
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

func runDefaultsView() (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	path, err := policy.DefaultPath()
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.Load(path)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		pol = policy.Default()
	default:
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	budgets := pol.Budgets
	where := st.RunWhere
	if where != settings.RunWhereAgentry {
		where = settings.RunWhereHere
	}
	return &RunDefaultsView{
		TimeoutSeconds: budgets.TimeoutSeconds, MaxTurns: budgets.MaxTurns, PolicyPath: path,
		Where: where, OutputDir: st.DefaultOutputDir, ChatIdleTimeoutSeconds: st.ChatIdleTimeoutSeconds,
		UnsafeMode: st.UnsafeMode, UnsafeImage: pol.Images.UnsafeImage(), BuiltinNoTimeout: st.BuiltinNoTimeout,
	}, nil
}
