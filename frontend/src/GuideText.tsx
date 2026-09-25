// One line of the run setup's guide text (manifest v2's guide block,
// DEV-141; plan guided-runs.md §4, M7; agentry-desktop#228): a line of
// "What to have ready" (the author's prepare list, or the derived one,
// which can carry the author's files line), the files hint and a key's
// connect help. The author's text comes from a release, so it is shown as
// text, never as markup:
//
//   - React escapes it, and every invisible character shows as a chip;
//   - an absolute http: or https: URL in it becomes a link that hands the
//     URL to the system browser through App.OpenExternal (which refuses any
//     other scheme a second time), and never navigates this window. The
//     link reads as the parsed URL, so a look-alike host shows as the
//     punycode the browser will open, and a URL with a user name or
//     password stays text;
//   - nothing else in it is a link: a "javascript:" or "file:" string stays
//     text.
//
// The overview is markdown and goes through AgentMarkdown instead, under
// the same rules as a run's output.
import { ExternalLink, webHref } from "./AgentMarkdown";
import { RevealText } from "./reveal";

// A URL runs to the next space, bracket, control or format character (so
// a hidden override after a URL is shown as a chip, never carried into the
// link); trailing sentence punctuation is the sentence's, not the URL's.
const URL_RE = /https?:\/\/[^\s<>"'()[\]{}\p{Cc}\p{Cf}]+/gu;
const TRAILING_RE = /[.,;:!?]+$/;

// connectHelp is the author's help for one declared key, or "". The map is
// the author's, so only its own string entries count: a secret named
// "constructor" must not find Object.prototype's.
export function connectHelp(connect: { [key: string]: string } | undefined, name: string): string {
  if (!connect || !Object.prototype.hasOwnProperty.call(connect, name)) return "";
  const help = connect[name];
  return typeof help === "string" ? help : "";
}

export function GuideText({ text }: { text: string }) {
  const parts: React.ReactNode[] = [];
  let at = 0;
  let key = 0;
  for (const m of text.matchAll(URL_RE)) {
    const url = m[0].replace(TRAILING_RE, "");
    const start = m.index ?? 0;
    const href = webHref(url);
    if (!href) continue;
    if (start > at) parts.push(<RevealText key={key++} text={text.slice(at, start)} />);
    parts.push(
      <ExternalLink key={key++} href={href}>
        <RevealText text={href} />
      </ExternalLink>,
    );
    at = start + url.length;
  }
  if (at < text.length) parts.push(<RevealText key={key++} text={text.slice(at)} />);
  return <span className="guide-text selectable">{parts}</span>;
}
