// A changed install is shown with every path and the way back.
import { test } from "node:test";
import assert from "node:assert/strict";
import { changedLines, repairLine } from "../src/changedInstall.ts";

test("each kind of change is a line naming every path", () => {
  assert.deepEqual(
    changedLines({ ref: "acme/reporter@1.0.0", changed: ["SKILL.md", "agent.yaml"], added: ["extra.md"], removed: [], local: false }),
    ["Changed: SKILL.md, agent.yaml", "Added: extra.md"],
  );
});

test("a failed record is said before the files", () => {
  assert.deepEqual(
    changedLines({ ref: "acme/reporter@1.0.0", record: "install-json-changed", changed: null, added: null, removed: ["notes.md"], local: false }),
    ["Its install.json is not the one laid down.", "Removed: notes.md"],
  );
  assert.deepEqual(changedLines({ ref: "acme/reporter@1.0.0", local: false }), ["Its record no longer matches its files."]);
});

test("a registry release offers Repair, a local one its folder", () => {
  assert.match(repairLine({ ref: "a/b@1.0.0", local: false }), /^Repair downloads the release again/);
  assert.equal(
    repairLine({ ref: "a/b@1.0.0", local: true, repair: "Build → Open folder… → Install" }),
    "It was installed from a folder, so it is laid down again from there: Build → Open folder… → Install.",
  );
});
