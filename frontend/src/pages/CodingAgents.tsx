// The coding agents a person may run QuickAgents through, and the export
// a one-click Install writes for them. HarnessChooser is the list the
// Welcome's Coding agent step shows: what the runner policy records, a
// Look that probes each harness once, Use this one. The
// Settings section adds Forget, and beneath the list what the choice
// implies — the Claude skill for Claude Code, AGENTS.md for Codex, by the
// model provider when neither is chosen — the folder for it, and the
// switch "also install into <coding agent>", off by default.
import { useCallback, useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { OpenExternal } from "../../wailsjs/go/app/App";
import { app } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice, Spinner } from "../ui";

// harnessLabel is the product's name the adapter carries ("Claude Code"),
// with the adapter's own name as the fallback.
export const harnessLabel = (h: { name: string; displayName?: string }) => h.displayName || h.name;

// exportName is the export's name as a heading ("Claude skill",
// "AGENTS.md"); exportLabel the same in a sentence ("the Claude skill").
export const exportName = (target: string) => (target === "claude-skill" ? "Claude skill" : "AGENTS.md");
export const exportLabel = (target: string) => (target === "claude-skill" ? "the Claude skill" : "AGENTS.md");

// codingAgentName names the coding agent the export serves — not every
// chosen one: with both chosen only the Claude skill is written, so the
// switch and the link say Claude Code. The Claude skill is Claude Code's
// whether it was chosen or implied by an Anthropic key; AGENTS.md is
// Codex's when Codex is chosen, else any coding agent's.
export function codingAgentName(view: app.CodingAgentsView | null): string {
  if (!view) return "a coding agent";
  if (view.export.target === "claude-skill") return "Claude Code";
  const codex = view.chosen.find((h) => h.name === "codex");
  return codex ? harnessLabel(codex) : "a coding agent";
}

export type HarnessState = {
  harnesses: app.HarnessView[] | null;
  chosen: app.HarnessView[] | null;
  // busy while a choice or a forget is being recorded.
  busy: boolean;
};

// HarnessChooser: Looking is a click, not the list opening: the probe
// runs each harness's version flag, and a binary named
// claude on the PATH of a person who never asked for coding agents should
// not run because a page was reached — the CLI's doctor probes only when
// asked, too. Until then the list shows what the policy already records,
// read without executing anything. onState reports what it knows, for a
// caller that decides on it (the Welcome's Skip); onChanged fires once per
// write to the policy — a choice, a forget — for a caller that reads
// something the policy implies (the Settings' export).
export function HarnessChooser({ probeOnEntry = false, allowForget = false, onState, onChanged }: {
  probeOnEntry?: boolean;
  allowForget?: boolean;
  onState?: (s: HarnessState) => void;
  onChanged?: () => void;
}) {
  const [chosen, setChosen] = useState<app.HarnessView[] | null>(null);
  const [harnesses, setHarnesses] = useState<app.HarnessView[] | null>(null);
  const [looking, setLooking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    RunAPI.ChosenHarnesses().then(setChosen).catch(() => setChosen([]));
  }, []);
  // The caller's callback, read through a ref: a fresh function on each of
  // its renders must not re-report an unchanged state.
  const report = useRef(onState);
  useEffect(() => { report.current = onState; }, [onState]);
  useEffect(() => { report.current?.({ harnesses, chosen, busy: busy !== null }); }, [harnesses, chosen, busy]);

  async function look() {
    setLooking(true); setError(null);
    try {
      setHarnesses(await RunAPI.HarnessStatus());
    } catch (e) {
      setError(String(e)); setHarnesses([]);
    } finally {
      setLooking(false);
    }
  }

  useEffect(() => {
    if (probeOnEntry) void look();
  }, [probeOnEntry]);

  async function choose(name: string) {
    setBusy(name); setError(null);
    try {
      const hs = await RunAPI.ChooseHarness(name);
      setHarnesses(hs);
      setChosen(hs.filter((h) => h.chosen));
      onChanged?.();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  // Forgetting reads the policy back, never probes: the list keeps what
  // Look found and only the chosen mark moves.
  async function forget(name: string) {
    setBusy(name); setError(null);
    try {
      const still = await RunAPI.ForgetHarness(name);
      setChosen(still);
      setHarnesses((hs) => hs && hs.map((h) => app.HarnessView.createFrom({ ...h, chosen: still.some((c) => c.name === h.name) })));
      onChanged?.();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <ErrorBox error={error} />
      {harnesses === null && (
        <div className="card wizard-harness">
          {chosen && chosen.length > 0 ? (
            <p className="muted">
              The runner policy already records {chosen.map(harnessLabel).join(" and ")}. Looking again
              checks it is still there and lets you pick another.
            </p>
          ) : (
            <p className="muted">
              Nothing is recorded yet. Looking runs each coding agent&apos;s own version command, once, and
              nothing else.
            </p>
          )}
          <div className="row mt">
            <Button variant="primary" disabled={looking} onClick={() => void look()}>
              {looking ? "Looking…" : "Look for coding agents on this machine"}
            </Button>
            {looking && <Spinner />}
            {allowForget && chosen?.map((c) => (
              <Button key={c.name} variant="link" disabled={busy !== null} onClick={() => void forget(c.name)}>
                {busy === c.name ? "Forgetting…" : `Forget ${harnessLabel(c)}`}
              </Button>
            ))}
          </div>
        </div>
      )}
      {harnesses?.map((h) => (
        <div key={h.name} className={`card wizard-harness${h.chosen ? " chosen" : ""}`}>
          <div className="row">
            <strong>{harnessLabel(h)}</strong>
            <span className="muted mono">{h.executable}</span>
            <div className="grow" />
            {allowForget && h.chosen && (
              <Button variant="link" disabled={busy !== null} onClick={() => void forget(h.name)}>
                {busy === h.name ? "Forgetting…" : "Forget"}
              </Button>
            )}
            {h.found ? (
              <Button
                variant={h.chosen ? "ghost" : "primary"}
                disabled={busy !== null || h.chosen}
                onClick={() => void choose(h.name)}
              >
                {h.chosen ? "Chosen ✓" : busy === h.name ? "Recording…" : "Use this one"}
              </Button>
            ) : (
              <Button variant="link" onClick={() => void OpenExternal(h.installUrl)}>Get {harnessLabel(h)}</Button>
            )}
          </div>
          {h.found ? (
            <p className="muted">
              Found at <span className="mono">{h.path}</span>
              {h.version ? <> — {h.version}</> : null}
              {h.pathFromPolicy ? <> (from the policy)</> : h.source === "known location" ? <> (a usual install location, not on this app&apos;s PATH)</> : null}
            </p>
          ) : (
            <p className="muted">Not on this machine. {h.reason}</p>
          )}
          {h.warnings?.map((w, i) => <Notice key={i} tone="warn">{w}</Notice>)}
        </div>
      ))}
    </>
  );
}

// CodingAgentsCard is Settings → Coding agents. The export line is what
// Go decided (SettingsAPI.CodingAgents reads the same rule the runner
// home's export/ follows), shown and not asked; the folder for AGENTS.md
// is the native picker's, opened by Go; the switch stays off
// until the person turns it on, and will not go on while AGENTS.md has
// nowhere to go.
export function CodingAgentsCard() {
  const [view, setView] = useState<app.CodingAgentsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [policyWrites, setPolicyWrites] = useState(0);

  // Read on mount and again after each write to the policy — a choice or a
  // forget moves the implied export; a Look moves nothing.
  useEffect(() => {
    SettingsAPI.CodingAgents().then(setView).catch((e) => setError(String(e)));
  }, [policyWrites]);
  const onPolicyWrite = useCallback(() => setPolicyWrites((n) => n + 1), []);

  const run = async (fn: () => Promise<app.CodingAgentsView>) => {
    setBusy(true); setError(null);
    try {
      setView(await fn());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const x = view?.export;
  const agent = codingAgentName(view);

  return (
    <Card>
      <HarnessChooser allowForget onChanged={onPolicyWrite} />
      <p className="muted small" style={{ marginTop: 12 }}>
        Claude Code reads a Claude skill; Codex reads AGENTS.md. With neither chosen the model provider
        decides: an Anthropic key means the Claude skill, anything else AGENTS.md. Claude Code counts
        first when both are chosen.
      </p>
      {x && (
        <>
          <div className="kv">
            <span className="k">installs as</span>
            <span>
              <strong>{exportName(x.target)}</strong>
              {x.target === "claude-skill"
                ? <span className="muted"> — in <span className="mono selectable">{x.dir}</span>, where Claude Code reads</span>
                : x.dir
                  ? <span className="muted"> — in <span className="mono selectable">{x.dir}</span> (its <span className="mono">agents/</span> folder and <span className="mono">AGENTS.md</span>)</span>
                  : <span className="muted"> — no project folder chosen yet</span>}
            </span>
          </div>
          {x.target === "agents-md" && (
            <div className="row gap" style={{ marginTop: 6 }}>
              <Button disabled={busy} onClick={() => run(SettingsAPI.ChooseCodingAgentProjectDir)}>
                {x.dir ? "Choose another project folder…" : "Choose the project folder…"}
              </Button>
              {x.needsDir && <span className="muted small">AGENTS.md needs a project to live in; the picker chooses it.</span>}
            </div>
          )}
          <label className="row gap" style={{ marginTop: 10 }}>
            <input
              type="checkbox"
              checked={x.enabled}
              disabled={busy || (!x.enabled && x.needsDir)}
              onChange={(e) => run(() => SettingsAPI.SetInstallIntoCodingAgent(e.target.checked))}
            />
            <span>Also install into {agent}</span>
          </label>
          <div className="muted small">
            Off, Install puts a QuickAgent in the runner home only, where runs read it. On, the same verified
            release is also written as {exportLabel(x.target)}, with its own entry in the Library. A coding agent
            enforces none of a QuickAgent&apos;s contract; the runner does.
          </div>
        </>
      )}
      <ErrorBox error={error} />
    </Card>
  );
}
