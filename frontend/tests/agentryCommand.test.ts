// What the agentry command's card says on a Mac: installed or not, an
// agentry the app leaves alone, an app not yet in Applications, and what a
// new terminal finds.
import { test } from "node:test";
import assert from "node:assert/strict";
import { commandLines, type CommandView } from "../src/agentryCommand.ts";

const base = (status: Partial<CommandView["status"]>, version = "0.6.120"): CommandView => ({
  version,
  status: { path: "/Users/ada/.local/bin/agentry", linked: false, other: false, installable: true, ...status },
});

test("not installed: says what Install does, and that no terminal finds one", () => {
  assert.deepEqual(commandLines(base({})), [
    "Not installed. Install puts agentry 0.6.120, which this app carries, on your PATH as a link at /Users/ada/.local/bin/agentry, so it updates when the app does.",
    "A new terminal finds no agentry command.",
  ]);
});

test("installed: names the link and what a terminal runs, through the link", () => {
  const lines = commandLines(base({ linked: true, resolves: "/Users/ada/.local/bin/agentry", resolvesTo: "/Applications/Agentry Desktop.app/Contents/Resources/bin/agentry" }));
  assert.equal(lines[0], "Installed: /Users/ada/.local/bin/agentry links to agentry 0.6.120, which this app carries, so it updates when the app does.");
  assert.equal(lines[1], "In a new terminal, agentry is /Users/ada/.local/bin/agentry (/Applications/Agentry Desktop.app/Contents/Resources/bin/agentry).");
});

test("an agentry the app did not put there is left alone, and another on PATH is named", () => {
  const lines = commandLines(base({ other: true, resolves: "/opt/homebrew/bin/agentry" }));
  assert.equal(lines[0], "/Users/ada/.local/bin/agentry is an agentry this app did not put there, which it leaves as it is.");
  assert.equal(lines[1], "In a new terminal, agentry is /opt/homebrew/bin/agentry.");
});

test("an app run from the disk image is told to move first", () => {
  assert.ok(commandLines(base({ installable: false })).includes("Move Agentry Desktop to your Applications folder and open it from there to install the command."));
});

test("with no version read it names the command, not a version", () => {
  assert.match(commandLines(base({}, ""))[0], /Install puts the agentry command, which this app carries/);
});
