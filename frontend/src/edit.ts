// Edit on the QuickAgent page (agentry-desktop#500; plan drafter-revisions.md
// §4, D1, D5, D6, D7; DEV-199): the page's Edit opens the Build tab on the
// QuickAgent's current release, under the same name at its next minor.
//
// Who is offered Edit is the registry's answer, canPublish (GET /agents,
// agentry#875): the desktop's sign-in token publishes and never maintains,
// so it never reads the maintainer's settings the site reads. The upload
// decides again.
//
// The request travels from the QuickAgent page to the Build tab through this
// module, as a landed draft travels through drafts.ts: the page asks, App
// opens the tab, the tab takes the request once and reads the release
// (BuildAPI.EditRelease). Pure and free of the Wails bindings, so the rules
// below are tested on their own.

/** Which QuickAgent the page asked to edit, and on which registry. */
export interface EditRequest {
  registry: string;
  namespace: string;
  name: string;
}

/** What the Build tab keeps beside the form while it edits a release. */
export interface Editing {
  namespace: string;
  name: string;
  /** The release the edit started from. */
  from: string;
  /** The version the edit set: the next minor, which the person can change. */
  version: string;
}

/** Edit is offered for a QuickAgent with a published release that the
 *  registry says this sign-in may add a version to. */
export function offersEdit(d: { canPublish?: boolean; latest?: string } | null | undefined): boolean {
  return !!d && d.canPublish === true && !!d.latest;
}

/** The edit, while the form still names its QuickAgent; null once the form
 *  holds anything else (a folder opened, a draft taken, Start over), as the
 *  site forgets its edit marker. */
export function stillEditing(e: Editing | null, m: { namespace: string; name: string } | null | undefined): Editing | null {
  if (!e || !m) return null;
  return m.namespace === e.namespace && m.name === e.name ? e : null;
}

/** The line above the form while it edits a release. */
export function editingLine(e: Editing, version: string): string {
  const v = version.trim() || e.version;
  return `Editing ${e.namespace}/${e.name}, from version ${e.from}. Submit adds version ${v} to it.`;
}

/** Whether a QuickAgent page on registry `page` may open an edit in the
 *  Build tab, which submits to `build`: the same registry, however its
 *  address was written (a trailing slash, the scheme's case). */
export function sameRegistry(page: string, build: string): boolean {
  const norm = (u: string) => u.trim().replace(/\/+$/, "").toLowerCase();
  return norm(page) !== "" && norm(page) === norm(build);
}

// held names the QuickAgent the Build tab's form holds, which Edit would
// replace: the tab says so whenever its kept work changes.
let held: string | null = null;

/** The Build tab: what its form holds now, or null when it holds nothing. */
export function noteHeldForm(name: string | null): void {
  held = name;
}

/** What the QuickAgent page's Edit asks before replacing. */
export function heldForm(): string | null {
  return held;
}

let pending: EditRequest | null = null;

/** The QuickAgent page's Edit: the Build tab takes it when it opens. */
export function requestEdit(r: EditRequest): void {
  pending = { ...r };
}

/** The request the page made and no tab has taken yet, taken. */
export function takeEditRequest(): EditRequest | null {
  const r = pending;
  pending = null;
  return r;
}
