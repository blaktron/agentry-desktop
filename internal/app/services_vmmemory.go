// The container engine's virtual machine and Runs at once. On macOS and
// Windows the engine runs in a virtual machine whose memory is often far
// below the computer's, so raising Runs at once past what that machine fits
// offers to raise its memory: a card names the engine, the new size and the
// restart, Yes restarts it with the new size while nothing runs, and No
// saves the number its memory fits now. Lowering the number changes no
// machine, and no machine is ever made smaller.
package app

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/sysmem"
)

// The engine's virtual machine and its resize; a test substitutes both.
var (
	engineVM = run.EngineVM
	resizeVM = run.ResizeVM
)

// vmReadWait bounds reading the engine's virtual machine for a page.
const vmReadWait = 20 * time.Second

// fitVM is what saving n runs at once does to the engine's virtual machine,
// with current the number saved now. It returns the number to save: n, or
// the number the machine fits when the person keeps its size.
func (s *SettingsAPI) fitVM(n, current int) (int, error) {
	if n <= current {
		return n, nil
	}
	ctx := s.app.ctx
	vm := engineVM(ctx)
	if !vm.InVM() || vm.MemoryBytes == 0 || !vm.Resizable {
		return n, nil
	}
	total, _ := memoryTotal()
	need := sysmem.VMMemoryGiB(n, total)
	have := sysmem.RoundGiB(vm.MemoryBytes)
	if have >= need {
		return n, nil
	}
	fits := min(n, sysmem.RunsFit(vm.MemoryBytes))
	title := fmt.Sprintf("Give %s %d GB of memory?", vm.Name, need)
	msg := fmt.Sprintf("%d runs at once need about %d GB of memory in %s's virtual machine, which has %d GB now. "+
		"Agentry Desktop restarts %s to give it %d GB. That takes a minute or two, and nothing can run on it meanwhile.\n\n"+
		"With No, Runs at once is set to %d, which its memory fits now.",
		n, need, vm.Name, have, vm.Name, need, fits)
	if s.app.confirm == nil {
		return fits, nil
	}
	yes, err := s.app.confirm(ctx, title, msg)
	if err != nil {
		return 0, fmt.Errorf("the question about %s's memory could not be asked: %w", vm.Name, err)
	}
	if !yes {
		return fits, nil
	}
	if s.run == nil {
		return 0, fmt.Errorf("%s's memory cannot be changed here", vm.Name)
	}
	if err := s.run.acquire(jobSetup, "the change of "+vm.Name+"'s memory"); err != nil {
		return 0, err
	}
	defer s.run.release()
	logw := s.run.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := resizeVM(ctx, vm, need, sysmem.VMCPUs(n, runtime.NumCPU()), logw); err != nil {
		return 0, fmt.Errorf("%s's memory was not changed, and Runs at once is as it was: %w", vm.Name, err)
	}
	return n, nil
}

// EngineMemoryView is what Runs at once says of the engine's virtual
// machine.
type EngineMemoryView struct {
	// Line says how much memory the machine has and how many runs it fits;
	// "" when the engine runs on this computer or its memory is unknown.
	Line string `json:"line,omitempty"`
	// Short says the machine fits fewer runs than Runs at once is set to.
	Short bool `json:"short"`
}

// EngineMemory reads the engine's virtual machine for Runs at once.
func (s *SettingsAPI) EngineMemory() (*EngineMemoryView, error) {
	ctx, cancel := context.WithTimeout(s.app.ctx, vmReadWait)
	defer cancel()
	vm := engineVM(ctx)
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	return engineMemoryView(vm, runsAtOnce(st)), nil
}

// engineMemoryView says what a machine fits against the number set.
func engineMemoryView(vm run.VM, n int) *EngineMemoryView {
	if !vm.InVM() || vm.MemoryBytes == 0 {
		return &EngineMemoryView{}
	}
	have := sysmem.RoundGiB(vm.MemoryBytes)
	fits := sysmem.RunsFit(vm.MemoryBytes)
	runs := func(k int) string {
		if k == 1 {
			return "1 run"
		}
		return fmt.Sprintf("%d runs", k)
	}
	line := fmt.Sprintf("%s has %d GB of memory, enough for %s at once.", capitalize(vm.Name), have, runs(fits))
	if fits >= n {
		return &EngineMemoryView{Line: line}
	}
	total, _ := memoryTotal()
	need := sysmem.VMMemoryGiB(n, total)
	if vm.Resizable {
		line += fmt.Sprintf(" %s need about %d GB: choose the number again to give it that memory.", capitalize(runs(n)), need)
	} else {
		line += fmt.Sprintf(" %s need about %d GB: raise its memory in %s's own settings.", capitalize(runs(n)), need, vm.Name)
	}
	return &EngineMemoryView{Line: line, Short: true}
}
