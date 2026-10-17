// What a first install's capability consent accepts, named in the same
// words the CLI's install gate uses.

/** The consent's list as one phrase: "network egress, exec and a remote
 *  MCP server". */
export function consentWords(confirms: string[] | null | undefined): string {
  const what = confirms ?? [];
  if (what.length <= 1) return what[0] ?? "";
  return `${what.slice(0, -1).join(", ")} and ${what[what.length - 1]}`;
}

/** The checkbox's sentence for a first install. */
export function acceptSentence(confirms: string[] | null | undefined): string {
  const words = consentWords(confirms);
  return words ? `Accept what this release declares: ${words}.` : "Accept the declared capabilities.";
}
