// The step the image preparation is on, read from the lines the CLI's setup
// writes to the setup log, and the sentence a model sign-in's step is said
// with. The log stays below for anyone who wants every line; this is the one
// line a person waiting reads.

// The images setup prepares, by the start of their local name or published
// repository, and what a person calls each.
const IMAGES: [prefix: string, name: string][] = [
  ["agentry-claude-code", "Claude Code"],
  ["agentry-codex", "Codex"],
  ["ghcr.io/blaktron/agentry-codex", "Codex"],
  ["agentry-copilot", "Copilot CLI"],
  ["agentry-runner", "the runner"],
  ["ghcr.io/blaktron/agentry-mcp-gateway", "the tool gateway"],
  ["ghcr.io/blaktron/agentry-docker-agent", "the agent harness"],
];

function imageName(ref: string): string {
  const hit = IMAGES.find(([prefix]) => ref.startsWith(prefix));
  return hit ? hit[1] : "";
}

// "the Claude Code 2.1.316 image", "the runner image", or "an image".
function theImage(ref: string, version = ""): string {
  const name = imageName(ref);
  if (!name) return "an image";
  return `${name.startsWith("the ") ? name : `the ${name}`}${version} image`;
}

// The version a local tag names: "agentry-claude-code:2.1.316" → "2.1.316".
function tagVersion(ref: string): string {
  const tag = ref.slice(ref.lastIndexOf(":") + 1);
  return /^\d+\.\d+\.\d+$/.test(tag) ? ` ${tag}` : "";
}

/** The step the latest line of the setup log says preparation is on, or
 *  null before any line says one. */
export function setupStep(log: string[]): string | null {
  for (let i = log.length - 1; i >= 0; i--) {
    const line = log[i].trim();
    let m = /^pulling (\S+)/.exec(line);
    if (m) return `Downloading ${theImage(m[1])}…`;
    m = /^building (\S+)/.exec(line);
    if (m) return `Building ${theImage(m[1], tagVersion(m[1]))} on this computer — this can take a few minutes…`;
    m = /^(claude-image|codex-image): the vendor's newest release is (\S+?),/.exec(line);
    if (m) return `Getting ${m[1] === "claude-image" ? "Claude Code" : "Codex"} ${m[2]}, the newest release, ready…`;
    if (/could not be used|could not ask for the newest/.test(line)) return "The newest release could not be used; preparing the tested one…";
    if (/^setup complete/.test(line)) return "Finishing…";
  }
  return null;
}

/** What a model sign-in waiting on its vendor says before it names a step:
 *  the Claude sign-in prepares Claude Code first, every other one opens the
 *  vendor's page straight away. */
export function signInWaiting(wayId: string): string {
  return wayId === "claude" ? "Getting Claude Code ready…" : "Waiting for you in your browser…";
}
