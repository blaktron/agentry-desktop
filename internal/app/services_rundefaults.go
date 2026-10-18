package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// RunDefaultsView is Settings → Run defaults. The timeout and max turns are
// the runner policy's budgets, which the CLI runs with too; where runs go,
// the output folder and the Chat idle timeout are this app's own, in
// desktop.yaml. A broken policy file is said, not hidden behind the
// defaults, as Settings' status says it.
type RunDefaultsView struct {
	TimeoutSeconds int    `json:"timeoutSeconds"`
	MaxTurns       int    `json:"maxTurns"`
	PolicyPath     string `json:"policyPath"`
	// Where is settings.RunWhereHere or settings.RunWhereAgentry.
	Where string `json:"where"`
	// OutputDir is the saved output folder, "" for none.
	OutputDir string `json:"outputDir,omitempty"`
	// ChatIdleTimeoutSeconds is the Chat idle timeout (desktop.yaml), 0 for
	// none: a chat on this computer has no limit but this one.
	ChatIdleTimeoutSeconds int `json:"chatIdleTimeoutSeconds"`
	// UnsafeMode is the switch (desktop.yaml's unsafeMode), written only by
	// RunAPI.SetUnsafeMode; UnsafeImage the image its runs' commands are
	// installed on, the runner policy's images.unsafe or the pinned default.
	UnsafeMode  bool   `json:"unsafeMode"`
	UnsafeImage string `json:"unsafeImage,omitempty"`
	// BuiltinNoLimit is the no-limit switch for the built-in agents
	// (desktop.yaml's builtinNoLimit), on unless turned off: the built-in
	// agents' runs on this computer have no timeout and no turn cap the app
	// shows while it is on.
	BuiltinNoLimit bool `json:"builtinNoLimit"`
	// MalwareCheck is the runner policy's malwareCheck: on, every input of
	// a run on this computer is checked, as In Agentry; off (the default),
	// only the executables of a run whose QuickAgent declares commands.
	MalwareCheck bool `json:"malwareCheck"`
	// RememberRuns is Remember recent runs (desktop.yaml's rememberRuns), on
	// unless turned off; RunHistoryCount how many runs the history keeps, -1
	// when it cannot be read.
	RememberRuns    bool `json:"rememberRuns"`
	RunHistoryCount int  `json:"runHistoryCount"`
	// Deliverables is the Deliverables switch (desktop.yaml's deliverables),
	// on unless turned off: while it is on, a run copies the files it was for
	// into DeliverablesDir. DeliverablesDir is the folder in effect, "" when
	// the default cannot be named (DeliverablesError says why);
	// DeliverablesDefault says the person has chosen none.
	Deliverables        bool   `json:"deliverables"`
	DeliverablesDir     string `json:"deliverablesDir,omitempty"`
	DeliverablesDefault bool   `json:"deliverablesDefault"`
	DeliverablesError   string `json:"deliverablesError,omitempty"`
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

// SetMalwareCheck turns the malware check of this computer's runs on or
// off (run.SetMalwareCheck). It applies from the next run.
func (s *SettingsAPI) SetMalwareCheck(on bool) (*RunDefaultsView, error) {
	if err := run.SetMalwareCheck(on); err != nil {
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

// SetBuiltinNoLimit turns the no-limit switch for the built-in agents on or
// off (its addendum): while it is on, the default, the Drafter, the
// Knowledge Creator, the Examiner and the Voice Writer run on this computer
// with no timeout and no turn cap the app shows, only the hidden
// settings.BuiltinFailsafeTurns. It asks no confirmation: it gives no run a
// reach it lacked, the cost cap still holds, and a run that has started
// keeps the budgets it started with.
func (s *SettingsAPI) SetBuiltinNoLimit(on bool) (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.BuiltinNoLimit = &on
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
// folder is the picker's and never the page's, and it is not added to this
// session's picked set: it is an output folder, admitted at run time from
// this file (App.requireOutputDir), never an input root. A cancelled picker
// changes nothing.
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
	dDir, dDefault, dOn, dErr := deliverablesFolder()
	v := &RunDefaultsView{
		TimeoutSeconds: budgets.TimeoutSeconds, MaxTurns: budgets.MaxTurns, PolicyPath: path,
		Where: where, OutputDir: st.DefaultOutputDir, ChatIdleTimeoutSeconds: st.ChatIdleTimeoutSeconds,
		UnsafeMode: st.UnsafeMode, UnsafeImage: pol.Images.UnsafeImage(), BuiltinNoLimit: st.BuiltinNoLimitOn(), MalwareCheck: pol.MalwareCheckOn(),
		RememberRuns: st.RememberRunsEnabled(), RunHistoryCount: historyCount(),
		Deliverables: dOn, DeliverablesDir: dDir, DeliverablesDefault: dDefault,
	}
	if dErr != nil && dDir == "" {
		v.DeliverablesError = dErr.Error()
	}
	return v, nil
}

// SetDeliverables turns the Deliverables switch on or off. Off, no run copies
// its files out and the Home card says so; a run's own folder is unchanged
// either way. It applies from the next run.
func (s *SettingsAPI) SetDeliverables(on bool) (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.Deliverables = &on
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// ChooseDeliverablesDir opens the native picker for the Deliverables folder
// and saves what it returned. The binding takes no path, so the folder is the
// picker's and never the page's. A cancelled picker changes nothing.
func (s *SettingsAPI) ChooseDeliverablesDir() (*RunDefaultsView, error) {
	dir, err := s.app.pickDirectoryUnmarked("Choose the Deliverables folder")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		st, err := settings.Load()
		if err != nil {
			return nil, err
		}
		st.DeliverablesDir = dir
		if err := settings.Save(st); err != nil {
			return nil, err
		}
	}
	return runDefaultsView()
}

// UseDefaultDeliverablesDir forgets the chosen folder: deliverables go to the
// Agentry folder in Documents again. Nothing in either folder is touched.
func (s *SettingsAPI) UseDefaultDeliverablesDir() (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.DeliverablesDir = ""
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}
