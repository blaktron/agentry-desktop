// One line of an author's run guide (manifest v2's guide block, DEV-141;
// plan guided-runs.md §4, M7; agentry-desktop#228): the files hint and a
// key's connect help. The text comes from a release, so it is shown as
// text, never as markup:
//
//   - React escapes it, and every invisible character shows as a chip;
//   - an absolute http: or https: URL in it becomes a link that hands the
//     URL to the system browser through App.OpenExternal (which refuses any
//     other scheme a second time), and never navigates this window;
//   - nothing else in it is a link: a "javascript:" or "file:" string stays
//     text.
//
// The overview is markdown and goes through AgentMarkdown instead, under
// the same rules as a run's output.
import { OpenExternal } from "../wailsjs/go/main/App";
import { webHref } from "./AgentMarkdown";
import { RevealText } from "./reveal";

// A URL runs to the next space, bracket, control or format character (so
// a hidden override after a URL is shown as a chip, never carried into the
// link); trailing sentence punctuation is the sentence's, not the URL's.
const URL_RE = /https?:\/\/[^\s<>"'()[\]{}\p{Cc}\p{Cf}]+/gu;
const TRAILING_RE = /[.,;:!?]+$/;

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
      <a
        key={key++}
        href={href}
        title={href}
        draggable={false}
        onClick={(e) => {
          e.preventDefault();
          OpenExternal(href).catch(() => {});
        }}
        onAuxClick={(e) => e.preventDefault()}
        onDragStart={(e) => e.preventDefault()}
      >
        <RevealText text={url} />
      </a>,
    );
    at = start + url.length;
  }
  if (at < text.length) parts.push(<RevealText key={key++} text={text.slice(at)} />);
  return <span className="guide-text selectable">{parts}</span>;
}
