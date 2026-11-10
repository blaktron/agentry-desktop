// The coding agents a person may run QuickAgents through, the export
// a one-click Install writes for them, and the Agentry plugin in every
// coding tool on this computer (CodingToolPlugins). HarnessChooser is the list the
// Welcome's Coding agent step shows: what the runner policy records, a
// Look that probes each harness once, Use this one. The
// Settings section adds Forget, and beneath the list what the choice
// implies — the Claude skill for Claude Code, AGENTS.md for Codex, by the
// model provider when neither is chosen — the folder for it, and the
// switch "also install into <coding agent>", off by default.
import { useCallback, useEffect, useRef, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import { OpenExternal } from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/app";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import * as codingtools from "../../bindings/github.com/blaktron/agentry-desktop/internal/codingtools/models";
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

// appBuild names the desktop app a coding agent's executable came with,
// for a path the probe found inside one: the Claude app keeps the Claude
// Code its Code tab runs under ccd-cli, and the Codex app carries codex in
// its bundle.
export function appBuild(path: string): string | null {
  const p = path.replace(/\\/g, "/");
  if (p.includes("/.claude/remote/ccd-cli/")) return "the Claude app's own Claude Code";
  if (/\/Codex\.app\//.test(p)) return "the Codex app's own codex";
  return null;
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
              {h.pathFromPolicy ? <> (from the policy)</> : h.source === "known location" ? <> ({appBuild(h.path ?? "") ?? "a usual install location"}, not on this app&apos;s PATH)</> : null}
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

// CodingAgentsCard is Settings → Updates and installs → Coding tools. The export line is what
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

// pluginLine is a found tool's plugin state as one phrase.
export function pluginLine(t: codingtools.View): string {
  if (t.plugin === "installed") return "Agentry plugin installed";
  if (t.uses?.length) return `uses the plugins of ${t.uses.join(" and ")}`;
  if (t.plugin === "not installed") return "no Agentry plugin yet";
  return "";
}

// CodingToolPlugins is the second half of Coding tools: every coding tool
// the app knows, found or not, and the Agentry plugin in each. Go reads it
// from the file system alone, so it is read on opening; Install is Go's
// to run, after a native dialog that lists each command, and the page
// passes only the tool's id.
export function CodingToolPlugins() {
  const [tools, setTools] = useState<codingtools.View[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [results, setResults] = useState<Record<string, app.CodingToolInstall>>({});
  const [showMissing, setShowMissing] = useState(false);

  const read = useCallback(() => {
    SettingsAPI.CodingTools().then((ts) => setTools(ts ?? [])).catch((e) => setError(String(e)));
  }, []);
  useEffect(read, [read]);

  async function install(t: codingtools.View) {
    setBusy(t.id); setError(null);
    try {
      const r = await SettingsAPI.InstallCodingToolPlugin(t.id);
      if (r) {
        setResults((rs) => ({ ...rs, [t.id]: r }));
        setTools((ts) => ts && ts.map((x) => (x.id === r.tool.id ? r.tool : x)));
      }
    } catch (e) {
      if (!/not confirmed/i.test(String(e))) setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  const found = tools?.filter((t) => t.found) ?? [];
  const missing = tools?.filter((t) => !t.found) ?? [];

  return (
    <Card>
      <div className="row">
        <strong>The Agentry plugin in your coding tools</strong>
        <div className="grow" />
        <Button variant="link" disabled={busy !== null} onClick={read}>Look again</Button>
      </div>
      <p className="muted small">
        The coding tools on this computer, command-line tools, desktop apps and editors, found in their usual
        places without running any of them. The plugin lets a tool find, check, install and run QuickAgents through
        the agentry command here, which keeps your sign-in and enforces each QuickAgent&apos;s contract. Install runs
        the tool&apos;s own plugin command once you confirm it.
      </p>
      <ErrorBox error={error} />
      {tools === null && !error && <Spinner />}
      {tools !== null && found.length === 0 && (
        <p className="muted">None of the coding tools below is on this computer.</p>
      )}
      {found.map((t) => (
        <ToolRow key={t.id} t={t} busy={busy} result={results[t.id]} onInstall={() => void install(t)} />
      ))}
      {missing.length > 0 && (
        <>
          <Button variant="link" onClick={() => setShowMissing((v) => !v)}>
            {showMissing ? "Hide" : "Show"} the {missing.length} not on this computer
          </Button>
          {showMissing && missing.map((t) => (
            <div key={t.id} className="card wizard-harness" data-tool={t.id}>
              <div className="row">
                <strong>{t.name}</strong>
                <span className="badge kind">{t.kind}</span>
                <div className="grow" />
                {t.site && <Button variant="link" onClick={() => void OpenExternal(t.site)}>Get {t.name}</Button>}
              </div>
              {t.note && <p className="muted small">{t.note}</p>}
            </div>
          ))}
        </>
      )}
    </Card>
  );
}

function ToolRow({ t, busy, result, onInstall }: {
  t: codingtools.View;
  busy: string | null;
  result?: app.CodingToolInstall;
  onInstall: () => void;
}) {
  const installed = t.plugin === "installed";
  const line = pluginLine(t);
  return (
    <div className={`card wizard-harness${installed ? " chosen" : ""}`} data-tool={t.id}>
      <div className="row">
        <strong>{t.name}</strong>
        <span className="badge kind">{t.kind}</span>
        {line && <span className="muted small">{line}</span>}
        <div className="grow" />
        {t.commands?.length ? (
          <Button variant={installed ? "ghost" : "primary"} disabled={busy !== null} onClick={onInstall}>
            {busy === t.id ? "Installing…" : installed ? "Install again" : "Install the plugin"}
          </Button>
        ) : t.manualUrl ? (
          <Button variant="link" onClick={() => void SettingsAPI.OpenCodingToolLink(t.id)}>How to add it</Button>
        ) : null}
      </div>
      <p className="muted small">
        Found at <span className="mono selectable">{t.path}</span>
        {t.source === "usual location" ? <> (a usual install location, not on this app&apos;s PATH)</> : null}
      </p>
      {t.commands?.length ? (
        <div className="muted small">
          {t.via ? <>Installed with {t.via}&apos;s command:</> : <>Install runs:</>}
          {t.commands.map((c, i) => <div key={i} className="mono selectable">{c}</div>)}
        </div>
      ) : null}
      {t.blocked && <p className="muted small">{t.blocked}</p>}
      {t.manual && <p className="muted small">{t.manual}</p>}
      {t.plugin === "unknown" && !!t.commands?.length && (
        <p className="muted small">This tool keeps its plugins where the app cannot read whether Agentry is among them.</p>
      )}
      {t.note && <p className="muted small">{t.note}</p>}
      {result && <InstallResult r={result} />}
    </div>
  );
}

function InstallResult({ r }: { r: app.CodingToolInstall }) {
  return (
    <>
      {r.error
        ? <Notice tone="warn">{r.error}</Notice>
        : <Notice tone="ok">Done. Restart {r.tool.name}, or start a new session in it, for the plugin to load.</Notice>}
      <details>
        <summary className="muted small">What ran</summary>
        {r.steps?.map((s, i) => (
          <div key={i}>
            <div className="mono small">{s.command}{s.exitCode ? ` (exit ${s.exitCode})` : ""}</div>
            {s.output && <pre className="mono small selectable">{s.output}</pre>}
          </div>
        ))}
      </details>
    </>
  );
}
