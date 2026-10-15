package hostedrun

// What a finished run says. The outcome is the registry's, read from the run
// and never re-derived here; this file only decides how each one reads, so the
// seven outcomes are distinguishable and honest — a refusal says the platform's
// own verification refused the release and the credits came back, an error says
// the platform could not run it. A run In Agentry is stated in credits, time
// and turns, so no line names tokens: budget is the runner's turn budget or a
// policy's cost budget, and interrupted is a cancel or the sandbox host's stop
// at what the hold may buy, as the site words them.

// The outcomes a finished run carries.
const (
	OutcomeSucceeded   = "succeeded"
	OutcomeFailed      = "failed"
	OutcomeRefused     = "refused"
	OutcomeTimeout     = "timeout"
	OutcomeBudget      = "budget"
	OutcomeInterrupted = "interrupted"
	OutcomeError       = "error"
	// OutcomeBlocked: the QuickAgent could not do its task with what the run had
	// (the CLI's exit 18). The registry settles it when the QuickAgent said so and
	// refunds it when the runner's own tools failed it; the run's settled credits
	// say which.
	OutcomeBlocked = "blocked"
)

// Verdict is how one finished run reads.
type Verdict struct {
	// Outcome is the registry's word for it, verbatim.
	Outcome string
	// Tone is "ok", "warn" or "bad" — the badge classes the app already
	// has.
	Tone string
	// Line says what happened, in one sentence.
	Line string
	// Settled is false when the credits went back to the account.
	Settled bool
}

// Read renders a run's outcome. A run that is still moving, or one
// cancelled before a runner took it, has its own line.
func Read(run *Run) Verdict {
	if run == nil {
		return Verdict{Tone: "warn", Line: "no run"}
	}
	if run.Status == StatusCancelled {
		return Verdict{
			Outcome: StatusCancelled, Tone: "warn",
			Line:    "cancelled before a runner took it — the credits it held were released",
			Settled: false,
		}
	}
	if run.Status != StatusFinished {
		return Verdict{Outcome: run.Status, Tone: "warn", Line: statusLine(run)}
	}
	switch run.Outcome {
	case OutcomeSucceeded:
		return Verdict{Outcome: run.Outcome, Tone: "ok", Line: "ran to the end and wrote its result", Settled: true}
	case OutcomeFailed:
		return Verdict{Outcome: run.Outcome, Tone: "bad", Line: "the agent ran and failed — the receipt and the outputs say where", Settled: true}
	case OutcomeTimeout:
		return Verdict{Outcome: run.Outcome, Tone: "bad", Line: "stopped at the time limit for this tier", Settled: true}
	case OutcomeBudget:
		return Verdict{Outcome: run.Outcome, Tone: "bad", Line: "stopped at its turn or cost limit", Settled: true}
	case OutcomeInterrupted:
		return Verdict{Outcome: run.Outcome, Tone: "warn", Line: "interrupted, by your cancel or at its limit on model use — what had run was settled", Settled: true}
	case OutcomeRefused:
		return Verdict{
			Outcome: run.Outcome, Tone: "bad",
			Line:    "the platform's own verification refused this release before it ran — the credits it held were refunded",
			Settled: false,
		}
	case OutcomeError:
		return Verdict{
			Outcome: run.Outcome, Tone: "bad",
			Line:    "the platform could not run it — the credits it held were refunded",
			Settled: false,
		}
	case OutcomeBlocked:
		// Charged or refunded is the registry's to say, by what it settled.
		if run.SettledCredits != nil && *run.SettledCredits > 0 {
			return Verdict{
				Outcome: run.Outcome, Tone: "warn",
				Line:    "blocked: the QuickAgent could not do the task with what the run had, and its output says why — the model use it took to find that out was settled",
				Settled: true,
			}
		}
		return Verdict{
			Outcome: run.Outcome, Tone: "warn",
			Line:    "blocked: the runner's tools failed this run, so the QuickAgent could not do its task — the credits it held were refunded",
			Settled: false,
		}
	default:
		return Verdict{Outcome: run.Outcome, Tone: "warn", Line: "finished"}
	}
}

// State is where a run stands in the words the site uses (agentry-web
// lib/runs.ts stateWord): queued, running — a runner holding the run
// before its agent starts included — done with pass or fail, or
// cancelled. A pass is the one outcome that ran to the end; every other
// finish is a fail, and the verdict's line says which.
func State(run *Run) string {
	switch {
	case run == nil:
		return ""
	case run.Status == StatusCancelled:
		return "cancelled"
	case run.Status == StatusFinished && run.Outcome == "":
		return "done"
	case run.Status == StatusFinished && run.Outcome == OutcomeSucceeded:
		return "done — pass"
	case run.Status == StatusFinished:
		return "done — fail"
	case run.Status == StatusClaimed:
		return "running"
	default:
		return run.Status
	}
}

// statusLine says where a run that is still moving stands.
func statusLine(run *Run) string {
	switch run.Status {
	case StatusQueued:
		if run.CancelRequested {
			return "cancelling"
		}
		if run.QueuePosition > 1 {
			return "queued"
		}
		return "queued — next to run"
	case StatusClaimed, StatusRunning:
		// Claimed is a runner holding the run before its agent starts;
		// to the person it is already running, as the site words it.
		if run.CancelRequested {
			return "cancelling — the runner interrupts the agent and the run settles what it used"
		}
		return "running"
	default:
		return run.Status
	}
}
