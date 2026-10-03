// Edit on the QuickAgent page (agentry-desktop#500; DEV-199): offered on the
// registry's canPublish for a QuickAgent with a published release, carried
// from the page to the Build tab once, and kept beside the form only while
// the form still names the QuickAgent.
import { test } from "node:test";
import assert from "node:assert/strict";
import { editingLine, heldForm, noteHeldForm, offersEdit, requestEdit, sameRegistry, stillEditing, takeEditRequest } from "../src/edit.ts";

test("Edit is offered where the registry says this sign-in may publish, with a release to edit", () => {
  assert.equal(offersEdit({ canPublish: true, latest: "1.2.0" }), true);
  assert.equal(offersEdit({ canPublish: false, latest: "1.2.0" }), false);
  // A registry that predates canPublish, or a viewer not signed in.
  assert.equal(offersEdit({ latest: "1.2.0" }), false);
  // Nothing published: nothing to edit.
  assert.equal(offersEdit({ canPublish: true, latest: "" }), false);
  assert.equal(offersEdit(null), false);
});

const editing = { namespace: "chris", name: "meeting-actions", from: "1.2.0", version: "1.3.0" };

test("the edit holds while the form names its QuickAgent, and is forgotten once it names another", () => {
  assert.deepEqual(stillEditing(editing, { namespace: "chris", name: "meeting-actions" }), editing);
  assert.equal(stillEditing(editing, { namespace: "chris", name: "something-else" }), null);
  assert.equal(stillEditing(editing, { namespace: "acme", name: "meeting-actions" }), null);
  assert.equal(stillEditing(editing, null), null);
  assert.equal(stillEditing(null, { namespace: "chris", name: "meeting-actions" }), null);
});

test("the line names the QuickAgent, the release and the version Submit adds", () => {
  assert.equal(editingLine(editing, "1.3.0"), "Editing chris/meeting-actions, from version 1.2.0. Submit adds version 1.3.0 to it.");
  // The person changed the version: the line follows the form.
  assert.equal(editingLine(editing, "2.0.0"), "Editing chris/meeting-actions, from version 1.2.0. Submit adds version 2.0.0 to it.");
  assert.equal(editingLine(editing, " "), "Editing chris/meeting-actions, from version 1.2.0. Submit adds version 1.3.0 to it.");
});

test("a request is taken once", () => {
  requestEdit({ registry: "https://dev.example", namespace: "chris", name: "meeting-actions" });
  assert.deepEqual(takeEditRequest(), { registry: "https://dev.example", namespace: "chris", name: "meeting-actions" });
  assert.equal(takeEditRequest(), null);
});

test("the Build tab edits only its own registry's QuickAgents, however the address is written", () => {
  assert.equal(sameRegistry("https://dev.secureagentry.net/", "https://dev.secureagentry.net"), true);
  assert.equal(sameRegistry("HTTPS://dev.secureagentry.net", "https://dev.secureagentry.net"), true);
  assert.equal(sameRegistry("https://secureagentry.net", "https://dev.secureagentry.net"), false);
  assert.equal(sameRegistry("", ""), false);
});

test("Edit asks before replacing what the Build tab holds", () => {
  noteHeldForm("meeting-actions");
  assert.equal(heldForm(), "meeting-actions");
  noteHeldForm(null);
  assert.equal(heldForm(), null);
});
