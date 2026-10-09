// Which declared keys a run needs (plan optional-secrets.md D2,
// agentry-desktop#655): the Builder's model holds the keys the author
// marked Required, and every other declared key, the QuickAgent's own or a
// server's, is written to capabilities.optionalSecrets, so a key the author
// attaches starts as Optional. Pure, so node tests it without the app.

import { names, serverKeys, type ServerRow } from "./remoteServers.ts";

/** The parts of a Builder model these rules read. */
export interface KeyModel {
  secrets?: string[];
  mcpServers?: ServerRow[];
  requiredSecrets?: string[];
}


/**
 * Every key the model declares, its own then each server's, each once. A
 * remote server declares its key only when it signs in with one
 * (agentry-desktop#668), as the writer does.
 */
export function declaredKeys(m: KeyModel): string[] {
  return [...new Set([...names(m.secrets), ...(m.mcpServers ?? []).flatMap(serverKeys)])];
}

/** The declared keys a run can start without. */
export function optionalKeys(m: KeyModel): string[] {
  const required = new Set(names(m.requiredSecrets));
  return declaredKeys(m).filter((k) => !required.has(k));
}

/**
 * The model's Required keys after one key's choice, or with none: the
 * declared keys marked Required, in the form's order. A mark on a key no
 * longer declared is dropped, so removing a key never leaves a mark the
 * form's rules refuse.
 */
export function requiredKeys(m: KeyModel, change?: { name: string; required: boolean }): string[] {
  const marked = new Set(names(m.requiredSecrets));
  return declaredKeys(m).filter((k) => (change && k === change.name ? change.required : marked.has(k)));
}

/**
 * A secret question's `required`: one carried by a key of the QuickAgent's
 * own follows that key's marking (plan D7, the core's V10); any other
 * follows its box. The writer applies the same rule (internal/build
 * questionRequired).
 */
export function questionRequired(m: KeyModel, q: { id: string; type: string; required: boolean }): { required: boolean; follows: boolean } {
  const key = q.id.trim().toUpperCase();
  if (q.type !== "secret" || !names(m.secrets).includes(key)) return { required: q.required, follows: false };
  return { required: !optionalKeys(m).includes(key), follows: true };
}

/**
 * The consent's secrets row: every declared secret, the release's own then
 * its servers', each optional one marked; "" for none. Moving a secret
 * between optional and required asks for no new consent (plan D6), which
 * holds only while this names each one.
 */
export function secretsRow(declared?: string[], optional?: string[]): string {
  const opt = new Set(optional ?? []);
  return (declared ?? []).map((n) => (opt.has(n) ? `${n} (optional)` : n)).join(", ");
}
