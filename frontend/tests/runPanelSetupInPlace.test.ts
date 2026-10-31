// The run setup, rendered with the bindings mocked: a run on this computer
// that finds no container engine, or no way to pay for the model, shows
// that setup inside the panel and keeps what was entered; the Run button
// says in words why it is off; and once the engine answers, then the model
// is set up, the same panel goes on with the same answers.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
g.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { default: RunPanel } = await import("../src/pages/RunPanel.tsx");

const tick = () => new Promise((r) => setTimeout(r, 0));
// Renders, then lets every mocked call answer and every effect run.
async function settle(fn?: () => void) {
  await React.act(async () => { fn?.(); await tick(); await tick(); await tick(); });
}

const profile = {
  ref: "demo1/incident-summarizer@1.2.0", digest: "sha256:9c356fd0d20fef07",
  capabilityLines: ["network egress: api.pagerduty.com"],
  declaredSecrets: [], storedSecretNames: [], optionalSecrets: [], exists: false, keep: true,
  modes: ["autonomous"], intake: [{ id: "since", prompt: "Summarise incidents since when?", type: "text", required: true, choices: [] }],
  opening: "", shipsKnowledge: [],
};
const mcp = { refusal: "", vulnerabilityLine: "have no known high or critical advisories", toolRows: [], servers: [], vulnerabilities: [], dataAccess: [], declaredServers: [], secretNames: [], embeddedTools: [] };
const att = { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified" } as never;

type Setup = Record<string, unknown>;
// What Go would say of this machine, changed by each test as the machine
// changes; every bound call the panel and the two setups make.
function mocks(setup: Setup, prereq: Record<string, unknown>) {
  const calls: string[] = [];
  const log = (name: string) => calls.push(name);
  g.go = { app: {
    RunAPI: {
      SetupStatus: async () => { log("SetupStatus"); return { ...setup }; },
      PrereqStatus: async () => { log("PrereqStatus"); return { ...prereq }; },
      RedetectEngine: async () => { log("RedetectEngine"); },
      ProfileStatus: async () => profile,
      GatewayRequirements: async () => mcp,
      HostedAvailability: async () => ({ offered: false, available: false, reason: "" }),
      UnsafeNotice: async () => ({ on: false, line: "", unplaced: [] }),
      InstalledChanges: async () => null,
      ReleaseConnections: async () => ({ servers: [] }),
      QueuedRuns: async () => [],
      CreditsOffer: async () => ({ signedIn: false }),
      ModelSignIns: async () => [],
      EndpointWarning: async () => "",
      StoreModelKey: async (provider: string, key: string) => { log(`StoreModelKey ${provider} ${key.length}`); return { ...setup, keysStored: [provider] }; },
      ApplySetup: async (provider: string, model: string) => {
        log(`ApplySetup ${provider} ${model}`);
        Object.assign(setup, { modelReady: true, modelGap: "", providerSetUp: true, provider, model, keysStored: [provider], apiKeySource: "keyring" });
        return { ...setup };
      },
    },
    SettingsAPI: { RunDefaults: async () => ({ outputDir: "", where: "here" }) },
    KnowledgeAPI: { List: async () => ({ packs: [], registry: "https://secureagentry.net", signedIn: true }) },
    VoiceAPI: { View: async () => ({ registry: "https://secureagentry.net", signedIn: false, accountOffered: false, model: "", local: { exists: false } }) },
    DestinationsAPI: { List: async () => ({ destinations: [], keychainOff: false }), DefaultFor: async () => [] },
    ScheduleAPI: { Availability: async () => ({ available: true, zone: "Europe/London", lingerOn: false, signedOut: { available: true } }) },
    App: { ChooseFiles: async () => [], ChooseDirectory: async () => "", OpenExternal: async () => {} },
  } };
  return calls;
}

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
});

async function open(step: "details" | "review") {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await settle(() => root!.render(React.createElement(RunPanel, {
    registry: "https://secureagentry.net", ns: "demo1", name: "incident-summarizer", version: "1.2.0",
    summary: "Summarises an incident channel.", attestation: att, onClose: () => {}, initialStep: step,
  })));
  return el;
}

const text = (node: Element | null) => (node?.textContent ?? "").replace(/\s+/g, " ").trim();
const buttons = (node: Element) => Array.from(node.querySelectorAll("button"));
const button = (node: Element, label: RegExp) => {
  const b = buttons(node).find((b) => label.test(text(b)));
  assert.ok(b, `no button ${label}`);
  return b;
};
// Types into a field as a person would, through the native setter and an
// input event, which React reads.
function typeInto(field: HTMLTextAreaElement | HTMLInputElement, value: string) {
  const proto = field instanceof window.HTMLTextAreaElement ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(field, value);
  field.dispatchEvent(new window.Event("input", { bubbles: true }));
}
const runButton = (node: Element) => button(node, /^Run$/);
const why = (node: Element) => text(node.querySelector("#run-button-why"));

test("no engine: the engine setup is in the panel, the answers stay, and the run goes on once it answers", async () => {
  const setup: Setup = { dockerOk: false, dockerErr: "no engine answered", modelReady: false, modelGap: "$ANTHROPIC_API_KEY is not set in this app's environment, and no anthropic key is saved on this computer.", imagesReady: false, keysStored: [], policyExists: false, provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", timeoutSeconds: 900, maxTurns: 60 };
  const prereq: Record<string, unknown> = { dockerOk: false, state: "manual", os: "linux", engineInstalled: false, dockerErr: "no engine answered", downloadUrl: "https://docs.docker.com/engine/install/", warnings: [] };
  const calls = mocks(setup, prereq);
  const panel = await open("details");

  // The answers: the intake question and the task.
  const since = panel.querySelector<HTMLInputElement>("input[type=text]");
  assert.ok(since, "the intake question's field");
  await settle(() => typeInto(since, "last Tuesday"));
  const task = panel.querySelector<HTMLTextAreaElement>("textarea");
  assert.ok(task, "the task field");
  await settle(() => typeInto(task, "Summarise the checkout incident."));
  await settle(() => button(panel, /^Next: Review and start$/).click());

  // Review: the engine's card is in the panel, and Run says why it is off.
  assert.equal(panel.querySelector("[data-setup-gap]")?.getAttribute("data-setup-gap"), "engine");
  assert.match(text(panel.querySelector(".run-setup-in-place")), /No container engine answers on this computer, and a QuickAgent run here needs one\./);
  // The engine's own words are on the card, once.
  assert.equal(text(panel.querySelector(".run-setup-in-place")).split("no engine answered").length - 1, 1);
  assert.match(text(panel.querySelector(".run-setup-in-place")), /everything you entered stays/);
  assert.ok(button(panel, /^Re-detect$/), "the engine card's Re-detect");
  assert.equal(runButton(panel).disabled, true);
  assert.match(why(panel), /^No container engine answers on this computer/);
  assert.equal(runButton(panel).getAttribute("aria-describedby"), "run-button-why");
  // The recap still shows what was entered.
  assert.match(text(panel.querySelector(".run-recap-task")), /^Summarise the checkout incident\.$/);

  // The engine answers: Re-detect asks Go, the card reports it, the panel
  // reads the status again, and the model's setup takes the engine's place.
  Object.assign(prereq, { dockerOk: true, state: "ready" });
  Object.assign(setup, { dockerOk: true, dockerErr: "" });
  await settle(() => button(panel, /^Re-detect$/).click());
  await settle();
  assert.ok(calls.includes("RedetectEngine") && calls.filter((c) => c === "SetupStatus").length >= 2, `calls: ${calls.join(", ")}`);
  assert.equal(panel.querySelector("[data-setup-gap]")?.getAttribute("data-setup-gap"), "model");
  assert.match(why(panel), /^\$ANTHROPIC_API_KEY is not set in this app's environment, and no anthropic key is saved on this computer\./);
  assert.match(text(panel.querySelector(".run-recap-task")), /^Summarise the checkout incident\.$/);
  assert.match(text(panel.querySelector(".run-recap")), /last Tuesday/);

  // Back on Details, the fields hold the answers.
  await settle(() => button(panel, /^Back$/).click());
  assert.equal(panel.querySelector<HTMLTextAreaElement>("textarea")?.value, "Summarise the checkout incident.");
  assert.equal(panel.querySelector<HTMLInputElement>("input[type=text]")?.value, "last Tuesday");
});

test("no model: the model provider's setup is in the panel, and a save there turns Run on with the answers kept", async () => {
  const setup: Setup = { dockerOk: true, modelReady: false, modelGap: "$ANTHROPIC_API_KEY is not set in this app's environment, and no anthropic key is saved on this computer.", imagesReady: true, keysStored: [], policyExists: false, provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", timeoutSeconds: 900, maxTurns: 60, engine: { kind: "docker", version: "28.0", summary: "docker 28.0" } };
  const calls = mocks(setup, { dockerOk: true, state: "ready" });
  const panel = await open("details");
  await settle(() => typeInto(panel.querySelector<HTMLInputElement>("input[type=text]")!, "last Tuesday"));
  await settle(() => typeInto(panel.querySelector<HTMLTextAreaElement>("textarea")!, "Summarise the checkout incident."));
  await settle(() => button(panel, /^Next: Review and start$/).click());

  assert.equal(panel.querySelector("[data-setup-gap]")?.getAttribute("data-setup-gap"), "model");
  const inPlace = panel.querySelector(".run-setup-in-place")!;
  assert.match(text(inPlace), /The model provider first\. \$ANTHROPIC_API_KEY is not set/);
  assert.match(text(inPlace.querySelector("h2")), /^Model provider$/);
  assert.equal(runButton(panel).disabled, true);
  assert.match(why(panel), /^\$ANTHROPIC_API_KEY is not set/);
  assert.equal(panel.querySelector("[data-setup-gap]")?.closest(".run-panel") !== null, true, "the setup is inside the panel");

  // A key pasted and saved, through the form's own calls: the key goes to
  // the keychain in its own call, the policy follows, and the status the
  // save returns reaches the panel. Nothing entered moved. Anthropic's form
  // opens on the Claude sign-in; the key is its second way.
  const keyWay = Array.from(inPlace.querySelectorAll("label")).find((l) => /^API key/.test(text(l)))?.querySelector<HTMLInputElement>("input[type=radio]");
  assert.ok(keyWay, "the API key way");
  await settle(() => keyWay.click());
  const key = inPlace.querySelector<HTMLInputElement>("input[type=password]");
  assert.ok(key, "the API key field");
  await settle(() => typeInto(key, "sk-ant-api-example-0000"));
  await settle(() => button(inPlace, /^Save provider settings$/).click());
  await settle();
  assert.ok(calls.includes("StoreModelKey anthropic 23"), `calls: ${calls.join(", ")}`);
  assert.ok(calls.some((c) => c.startsWith("ApplySetup anthropic ")), `calls: ${calls.join(", ")}`);
  assert.equal(panel.querySelector("[data-setup-gap]"), null, "the model setup left once the model was ready");
  assert.equal(runButton(panel).disabled, false, `Run is still off: ${why(panel)}`);
  assert.equal(panel.querySelector("#run-button-why"), null);
  assert.match(text(panel.querySelector(".run-recap-task")), /^Summarise the checkout incident\.$/);
  assert.match(text(panel.querySelector(".run-recap")), /last Tuesday/);
});

test("a run In Agentry needs nothing from this machine: no setup is shown and the engine's absence is no blocker", async () => {
  const setup: Setup = { dockerOk: false, dockerErr: "no engine answered", modelReady: false, modelGap: "no key", imagesReady: false, keysStored: [], provider: "anthropic", model: "m", apiKeyEnv: "ANTHROPIC_API_KEY", timeoutSeconds: 900, maxTurns: 60 };
  mocks(setup, { dockerOk: false, state: "manual", os: "linux" });
  g.go.app.RunAPI.HostedAvailability = async () => ({ offered: true, available: true, reason: "", credits: 0 });
  g.go.app.RunAPI.AttachHostedRun = async () => ({ run: null });
  g.go.app.SettingsAPI.RunDefaults = async () => ({ outputDir: "", where: "agentry" });
  const panel = await open("review");
  await settle();
  assert.equal(panel.querySelector("[data-setup-gap]"), null);
  assert.doesNotMatch(why(panel), /container engine|ANTHROPIC_API_KEY/);
});
