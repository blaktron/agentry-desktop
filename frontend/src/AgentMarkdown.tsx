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
//   - headings start at h4, under the panel's own h2 and the result's h3;
//   - invisible characters show as chips, in the text and in alt text.
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { OpenExternal } from "../wailsjs/go/main/App";
import { RevealText, isInvisible } from "./reveal";

// webHref is the URL a link may keep, or null.
export function webHref(raw: string | undefined): string | null {
  if (!raw) return null;
  try {
    const u = new URL(raw);
    return u.protocol === "https:" || u.protocol === "http:" ? u.href : null;
  } catch {
    return null;
  }
}

const heading =
  (Tag: "h4" | "h5" | "h6") =>
  ({ children }: { children?: React.ReactNode }) => <Tag>{children}</Tag>;

const components: Components = {
  a: ({ href, children }) => {
    const h = webHref(href);
    if (!h) return <span>{children}</span>;
    return (
      <a
        href={h}
        title={h}
        onClick={(e) => {
          e.preventDefault();
          OpenExternal(h).catch(() => {});
        }}
      >
        {children}
      </a>
    );
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
  h1: heading("h4"),
  h2: heading("h5"),
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

export function AgentMarkdown({ text }: { text: string }) {
  return (
    <div className="readme selectable">
      <Markdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRevealInvisible]} skipHtml components={components}>
        {text}
      </Markdown>
    </div>
  );
}
