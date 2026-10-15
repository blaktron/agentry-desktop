// The first-run wizard's two paths After Sign in the person picks where their
// QuickAgents run: Local walks the rest of the setup as it was (Runtime, AI
// Review key, Coding agent, Done); Cloud, open only to someone signed in,
// says how their data in Agentry's cloud is kept and finishes there, since
// a hosted run needs nothing set up on this computer. Pure, so node's test
// runner holds it.

export type WizardStep = "welcome" | "signin" | "where" | "runtime" | "reviewkey" | "codingagent" | "done" | "cloud";
export type WizardPath = "local" | "cloud";

const SHARED: WizardStep[] = ["welcome", "signin", "where"];
const LOCAL: WizardStep[] = [...SHARED, "runtime", "reviewkey", "codingagent", "done"];
const CLOUD: WizardStep[] = [...SHARED, "cloud"];

// The steps the dots show. Until a path is picked the dots show Local's,
// the longer, so picking Cloud only ever takes dots away.
export function stepsFor(path: WizardPath | null): WizardStep[] {
  return path === "cloud" ? CLOUD : LOCAL;
}

// The path a step belongs to, or null for the steps both share: how the
// harness, which opens the wizard on a step, knows which dots to draw.
export function pathOf(step: WizardStep): WizardPath | null {
  if (SHARED.includes(step)) return null;
  return step === "cloud" ? "cloud" : "local";
}

// The step after or before this one on its path; null past either end.
export function stepAfter(step: WizardStep, path: WizardPath | null): WizardStep | null {
  const steps = stepsFor(path ?? pathOf(step));
  const i = steps.indexOf(step);
  return i >= 0 && i < steps.length - 1 ? steps[i + 1] : null;
}

export function stepBefore(step: WizardStep, path: WizardPath | null): WizardStep | null {
  const steps = stepsFor(path ?? pathOf(step));
  const i = steps.indexOf(step);
  return i > 0 ? steps[i - 1] : null;
}

// The path the wizard is on once it stands on this step: back on a step
// both paths share, the choice is open again, so the dots show Local's
// until the person picks once more, not Cloud's four dots kept on the choice
// after Back.
export function pathAt(step: WizardStep, path: WizardPath | null): WizardPath | null {
  return pathOf(step) === null ? null : path;
}

// Cloud is open only to someone signed in to the registry the app is
// browsing: a hosted run is started, paid for and read back under that
// sign-in. With no registry chosen yet, any signed-in registry counts, as
// the Done recap counts it.
export function cloudOpen(registries: { url: string; hasToken: boolean }[], activeRegistry: string): boolean {
  const norm = (u: string) => u.trim().replace(/\/+$/, "");
  const active = norm(activeRegistry);
  if (!active) return registries.some((r) => r.hasToken);
  return registries.some((r) => norm(r.url) === active && r.hasToken);
}

// Where each path leaves Settings → Run defaults → Where to run, written
// through the same SetRunWhere that page uses, so the choice still means
// something after the wizard closes.
export function runWhereFor(path: WizardPath): "here" | "agentry" {
  return path === "cloud" ? "agentry" : "here";
}
