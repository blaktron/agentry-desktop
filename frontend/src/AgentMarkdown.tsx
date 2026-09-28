// Markdown a run's agent wrote, rendered in the result view (plan
// guided-runs.md §5, D7; agentry-desktop#227). The site renders agent
// output under the same rules (agentry-web components/agent-markdown).
//
// The text comes from a BriefAgent's run, so it is treated as hostile:
//
//   - it becomes React elements, never an HTML string, and raw HTML in it
//     is dropped (skipHtml);
//   - no image is loaded: an image is its alt text in a placeholder;
//   - a link never navigates this window. It keeps its target only when
//     that is an absolute http: or https: URL, and a click hands it to the
//     system browser through App.OpenExternal, which refuses any other
//     scheme a second time. Anything else is shown as its text;
//   - headings start at h5, under the panel's h2, the outcome's h3 and the
//     result's h4;
//   - raw HTML dropped is said: a line under the text says it had some, so
//     a reader is not shown less than is there without knowing it;
//   - invisible characters show as chips, in the text and in alt text.
//
// With showHosts (a key's help, agentry-desktop#343) a link is also followed
// by the host it opens and cut at an invisible character (below).
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { OpenExternal } from "../wailsjs/go/main/App";
import { RevealText, isInvisible } from "./reveal";

// webHref is the URL a link may keep, or null. A URL carrying a user name
// or password is refused: "https://github.com@evil.example/" reads as one
// host and opens another (agentry-desktop#228).
export function webHref(raw: string | undefined): string | null {
  if (!raw) return null;
  try {
    const u = new URL(raw);
    if (u.username || u.password) return null;
    return u.protocol === "https:" || u.protocol === "http:" ? u.href : null;
  } catch {
    return null;
  }
}

const heading =
  (Tag: "h5" | "h6") =>
  ({ children }: { children?: React.ReactNode }) => <Tag>{children}</Tag>;

// ExternalLink is a link that never navigates this window: a click hands
// href (already passed through webHref) to the system browser through
// App.OpenExternal. The run's output and an author's guide (GuideText) both
// link only through it.
export function ExternalLink({ href, children }: { href: string; children?: React.ReactNode }) {
  return (
    <a
      href={href}
      title={href}
      draggable={false}
      onClick={(e) => {
        e.preventDefault();
        OpenExternal(href).catch(() => {});
      }}
      // A middle click would open the URL in a webview window of its own,
      // and a dragged link would navigate this one: neither is allowed.
      onAuxClick={(e) => e.preventDefault()}
      onDragStart={(e) => e.preventDefault()}
    >
      {children}
    </a>
  );
}

// linkHost is the host a kept link opens, as the browser will open it: a
// look-alike host reads as its punycode (xn--…), which the author's link text
// may not show.
function linkHost(href: string): string {
  try {
    return new URL(href).host;
  } catch {
    return "";
  }
}

// flatText is a link's children as plain text, or null when any child is
// an element (bold, code, a revealed character): then what the text reads
// as cannot be compared with the URL, and the host is always shown.
function flatText(children: React.ReactNode): string | null {
  if (typeof children === "string") return children;
  if (Array.isArray(children)) {
    let out = "";
    for (const c of children) {
      const t = flatText(c);
      if (t === null) return null;
      out += t;
    }
    return out;
  }
  return null;
}

const components: Components = {
  a: ({ href, children }) => {
    const h = webHref(href);
    if (!h) return <span>{children}</span>;
    return <ExternalLink href={h}>{children}</ExternalLink>;
  },
  img: ({ alt }) => (
    <span className="img-placeholder">
      Image not shown
      {alt ? (
        <>
          : <RevealText text={alt} />
        </>
      ) : null}
    </span>
  ),
  h1: heading("h5"),
  h2: heading("h6"),
  h3: heading("h6"),
  h4: heading("h6"),
  h5: heading("h6"),
  h6: heading("h6"),
};

// The hast shapes the reveal pass touches.
interface HastText {
  type: "text";
  value: string;
}
type HastNode = HastText | { type: string; tagName?: string; properties?: Record<string, unknown>; children?: HastNode[] };

function revealIn(value: string): HastNode[] {
  const out: HastNode[] = [];
  let run = "";
  for (const ch of value) {
    const cp = ch.codePointAt(0)!;
    if (!isInvisible(cp)) {
      run += ch;
      continue;
    }
    if (run) out.push({ type: "text", value: run });
    run = "";
    out.push({
      type: "element",
      tagName: "span",
      properties: { className: ["agy-invisible"], title: "invisible character revealed" },
      children: [{ type: "text", value: `U+${cp.toString(16).toUpperCase().padStart(4, "0")}` }],
    });
  }
  if (run) out.push({ type: "text", value: run });
  return out;
}

function revealTree(node: HastNode): void {
  if (!("children" in node) || !node.children) return;
  const next: HastNode[] = [];
  for (const child of node.children) {
    if (child.type === "text") next.push(...revealIn((child as HastText).value));
    else {
      revealTree(child);
      next.push(child);
    }
  }
  node.children = next;
}

function rehypeRevealInvisible() {
  return (tree: HastNode) => revealTree(tree);
}

// hasHTML is a rough test for markup in the text: a tag or a comment. It
// only decides whether the "not shown" line appears.
const HTML_RE = /<\/?[a-zA-Z][\w-]*[\s>/]|<!--/;

// cutAtInvisible is a link's URL, as written (still percent-encoded), up to
// the first invisible character it holds, raw or encoded, which an autolinked
// URL would otherwise carry inside it (GuideText's rule: a hidden override
// after a URL is shown as a chip, never carried into the link). Nothing else
// is decoded, so the cut URL opens what the original would have up to there.
function cutAtInvisible(href: string | undefined): string | undefined {
  if (!href) return href;
  const hex = /^[0-9a-fA-F]{2}$/;
  let i = 0;
  while (i < href.length) {
    if (href[i] === "%" && hex.test(href.slice(i + 1, i + 3))) {
      // One code point's UTF-8 bytes, percent-encoded.
      const bytes: number[] = [];
      let j = i;
      while (href[j] === "%" && hex.test(href.slice(j + 1, j + 3))) {
        bytes.push(parseInt(href.slice(j + 1, j + 3), 16));
        j += 3;
        const need = bytes[0] < 0x80 ? 1 : bytes[0] >= 0xf0 ? 4 : bytes[0] >= 0xe0 ? 3 : 2;
        if (bytes.length === need) break;
      }
      let ch = "";
      try {
        ch = new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array(bytes));
      } catch {
        ch = "";
      }
      if (ch && isInvisible(ch.codePointAt(0)!)) return href.slice(0, i);
      i = j;
      continue;
    }
    const cp = href.codePointAt(i)!;
    if (isInvisible(cp)) return href.slice(0, i);
    i += cp > 0xffff ? 2 : 1;
  }
  return href;
}

// showHosts is for an author's help about where to create a key
// (agentry-desktop#343). The key help was shown through GuideText, whose
// links read as the parsed URL (agentry-desktop#228); as markdown it keeps
// that property: each kept link is followed by the host it opens (a
// look-alike in punycode), unless its text is exactly the parsed URL it
// opens, and a link is cut at its first invisible character.
const hostComponents: Components = {
  ...components,
  a: ({ href, children }) => {
    const h = webHref(cutAtInvisible(href));
    if (!h) return <span>{children}</span>;
    return (
      <>
        <ExternalLink href={h}>{children}</ExternalLink>
        {flatText(children) !== h && <span className="link-host"> ({linkHost(h)})</span>}
      </>
    );
  },
};

export function AgentMarkdown({ text, showHosts = false }: { text: string; showHosts?: boolean }) {
  return (
    <div className="readme selectable">
      {HTML_RE.test(text) && <p className="muted small">This text contains HTML, which is not shown here.</p>}
      <Markdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRevealInvisible]} skipHtml components={showHosts ? hostComponents : components}>
        {text}
      </Markdown>
    </div>
  );
}
