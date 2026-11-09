package app

import (
	"context"
	"io"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
)

// RunOnRunhost runs a release on remote runhost name, as RunAgent runs it on
// this computer: the same gates and native dialogs, the same run lock and
// queue, the same run log, and the result opened into the same output folder,
// naming the runhost it ran on. What a run from the app does not carry to a
// runhost yet is refused before anything is sent (run.RunhostCannotCarry).
func (r *RunAPI) RunOnRunhost(name, reg, ref, task string, inputPaths []string, outDir string, timeoutSec, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, acceptUnscanned bool, model string) (*RunResultView, error) {
	if err := r.localRunGates(reg, ref, inputPaths, outDir, acceptUnsigned, false, acceptUnscanned, false); err != nil {
		return nil, err
	}
	id, err := r.mintRunID()
	if err != nil {
		return nil, err
	}
	if err := r.acquireRun(id, ref, nil); err != nil {
		return nil, err
	}
	defer r.release()
	ctx, cancel := context.WithCancel(r.app.ctx)
	defer cancel()
	live, err := r.beginRunAs(jobRun, ref, id, cancel)
	if err != nil {
		return nil, err
	}
	defer r.endRun(live)
	live.registry = reg
	r.runMeter(live.id, ref).Emit()
	logw := r.runLineWriter(live.id)
	logw.also = live.keep
	defer logw.Flush()
	say := func(line string) { _, _ = io.WriteString(logw, line+"\n") }
	res, ranOn, err := r.runOnRunhost(ctx, run.RunhostRequest{
		Runhost: name, Registry: reg, Ref: ref, Task: task, InputPaths: inputPaths, Intake: intakeAnswers,
		OutDir: outDir, OutDirIsRoot: isSavedOutputDir(outDir), TimeoutSeconds: timeoutSec, MaxTurns: maxTurns,
		AcceptUnsigned: acceptUnsigned, AcceptUnscanned: acceptUnscanned, CLIVersion: carriedCLIVersion(), Say: say, Model: model,
		BindDesktop: r.bindDesktop,
	}, logw)
	recordLocal(ctx, r.app, runhistory.KindRun, live, ref, res, err)
	if err != nil {
		return nil, err
	}
	v := r.resultView(res)
	v.RanOn = ranOn
	return v, nil
}

// runOnRunhost is run.RunOnRunhost, a test's seam.
func (r *RunAPI) runOnRunhost(ctx context.Context, req run.RunhostRequest, logw io.Writer) (*run.Result, *run.RanOn, error) {
	if r.runhostRun != nil {
		return r.runhostRun(ctx, req, logw)
	}
	return run.RunOnRunhost(ctx, req, logw)
}
