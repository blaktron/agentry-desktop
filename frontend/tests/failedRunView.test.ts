// The failure view, the error box and the signed-out card, rendered: a
// failed run shows its class and meaning, its cause as text, where the
// evidence is, and offers Run again only when the class says a retry may
// help; Report hands over the facts; an error box says the cause as a
// sentence and offers Try again; a registry that refused the sign-in this
// app holds reads as signed out, with Sign in.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { failureOf, localFailureInput } from "../src/failedRun.ts";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
g.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { FailedRunView } = await import("../src/pages/FailedRun.tsx");
const { ErrorBox } = await import("../src/primitives.tsx");
const { ProfileCard } = await import("../src/pages/home/ProfileCard.tsx");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
});

async function render(node: React.ReactElement) {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await React.act(async () => { root!.render(node); });
  return el;
}
const text = (node: Element | null) => (node?.textContent ?? "").replace(/\s+/g, " ").trim();
const buttons = (node: Element) => Array.from(node.querySelectorAll("button")).map((b) => text(b));
const click = async (node: Element, label: RegExp) => {
  const b = Array.from(node.querySelectorAll("button")).find((b) => label.test(text(b)));
  assert.ok(b, `no button ${label}`);
  await React.act(async () => { b.dispatchEvent(new window.MouseEvent("click", { bubbles: true })); });
};

const harness = {
  runId: "ab12cd34", status: "error", exitCode: 14, exitClass: "harness",
  exitClassMeaning: "the agent's harness failed or could not reach its model", retryLikelyHelps: true,
  error: "harness: the model answered 503‮", ref: "acme/agent@1.2.0", outDir: "/home/me/out", receiptsDir: "/home/me/out/receipts",
} as unknown as app.RunResultView;

test("a failed run shows its class, its meaning, its cause and its receipts, with Run again, Open folder and Report", async () => {
  const failure = failureOf(localFailureInput(harness))!;
  const acted: string[] = [];
  const node = await render(React.createElement(FailedRunView, {
    failure,
    runAgain: { label: "Run again with the same answers", onClick: () => acted.push("again") },
    openFolder: () => acted.push("folder"),
    report: () => acted.push("report"),
  }));
  assert.equal(text(node.querySelector("#run-failure-headline")), "Failed (harness): the agent's harness failed or could not reach its model.");
  assert.equal(node.querySelector("[data-run-failure]")?.getAttribute("data-run-failure"), "harness");
  assert.match(text(node.querySelector("[data-run-failure-retry]")), /may well succeed/);
  const facts = text(node.querySelector(".run-failure-facts"));
  assert.match(facts, /What failed\s*acme\/agent 1\.2\.0, on this computer\./);
  assert.match(facts, /Why\s*harness the agent's harness failed or could not reach its model\./);
  assert.match(facts, /Evidence\s*The receipts are in \/home\/me\/out\/receipts\./);
  // The cause is drawn as text: the invisible character the runner's line
  // carried is named, not rendered.
  const cause = node.querySelector("[data-run-failure-cause]")!;
  assert.match(text(cause), /harness: the model answered 503/);
  assert.ok(cause.querySelector(".invisible, [data-invisible], .chip, abbr") || !text(cause).includes("‮"), "the control character reached the page as itself");
  assert.deepEqual(buttons(node), ["Run again with the same answers", "Open folder", "Report"]);
  await click(node, /^Run again/);
  await click(node, /^Open folder$/);
  await click(node, /^Report$/);
  assert.deepEqual(acted, ["again", "folder", "report"]);
});

test("a class whose retry is unlikely to help offers no Run again, and says why", async () => {
  const policy = { ...harness, exitCode: 11, exitClass: "policy", exitClassMeaning: "refused by the operator policy or a missing secret before the agent started", retryLikelyHelps: false, error: "" } as unknown as app.RunResultView;
  const node = await render(React.createElement(FailedRunView, {
    failure: failureOf(localFailureInput(policy))!,
    runAgain: { label: "Run again with the same answers", onClick: () => {} },
    openFolder: () => {},
  }));
  assert.match(text(node.querySelector("[data-run-failure-retry]")), /would most likely fail the same way/);
  assert.deepEqual(buttons(node), ["Open folder"]);
  assert.match(text(node.querySelector(".run-failure-facts")), /Cause\s*The runner recorded no error line\./);
});

test("an error box says the cause as a sentence, as an alert, and offers Try again", async () => {
  let tries = 0;
  const node = await render(React.createElement(ErrorBox, { error: "read the run: HTTP 503", onRetry: () => tries++ }));
  const box = node.querySelector(".error-box")!;
  assert.equal(box.getAttribute("role"), "alert");
  assert.equal(text(box.querySelector(".error-box-cause")), "Read the run: HTTP 503.");
  await click(node, /^Try again$/);
  assert.equal(tries, 1);
  const quiet = await render(React.createElement(ErrorBox, { error: "the folder is gone" }));
  assert.deepEqual(buttons(quiet), [], "a box with no retry offers none");
  const none = await render(React.createElement(ErrorBox, { error: null }));
  assert.equal(none.querySelector(".error-box"), null);
});

test("a registry that refused the sign-in this app holds reads as signed out, with the reason and Sign in", async () => {
  let signIns = 0;
  const node = await render(React.createElement(ProfileCard, {
    registry: "https://r.example", signedIn: false, rejected: true, profile: null, error: null,
    onOpenProfile: () => {}, onSignIn: () => signIns++, onTopUp: () => {},
  }));
  assert.match(text(node.querySelector("[data-signin-rejected]")), /^r\.example no longer accepts this app's sign-in/);
  assert.equal(text(node.querySelector("h2")), "Sign in");
  await click(node, /^Sign in$/);
  assert.equal(signIns, 1);
  // Signed in with the profile unread: no Sign in, the cause of a failed
  // read in the status, with Try again.
  let retries = 0;
  const failed = await render(React.createElement(ProfileCard, {
    registry: "https://r.example", signedIn: true, profile: null, error: "reach r.example: connection refused", onRetry: () => retries++,
    onOpenProfile: () => {}, onSignIn: () => {}, onTopUp: () => {},
  }));
  assert.equal(text(failed.querySelector("[data-could-not-read]")), "Your profile could not be read: Reach r.example: connection refused. Try again");
  await click(failed, /^Try again$/);
  assert.equal(retries, 1);
});
