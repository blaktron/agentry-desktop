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

export function AgentMarkdown({ text }: { text: string }) {
  return (
    <div className="readme selectable">
      {HTML_RE.test(text) && <p className="muted small">This text contains HTML, which is not shown here.</p>}
      <Markdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRevealInvisible]} skipHtml components={components}>
        {text}
      </Markdown>
    </div>
  );
}
