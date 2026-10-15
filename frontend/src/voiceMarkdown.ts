// The Writing Voice's WYSIWYG editor, its Markdown half (the site's editor
// has the same rules, so the two clients read and write a voice alike). Tiptap
// v3 holds the voice as a ProseMirror document and this module turns
// VOICE.md into that document and back, with one MarkdownManager and one
// extension list, so the page and its tests share every rule:
//
// - Only what a VOICE.md uses: paragraphs, headings, bold, italic, inline
//   code, code blocks, block quotes, lists, rules and links. No image node,
//   no table, no raw HTML node.
// - Markdown an agent wrote carries no markup of its own: raw HTML is never
//   tokenized as HTML, so it stays literal text; an image's Markdown and a
//   link to anything but an absolute http(s) or mailto address stay literal
//   text too; a link is drawn with no href, so no click inside the editor
//   can open it (AgentMarkdown's rules, in an editor).
// - Invisible characters (zero-width, bidi, tag, format) are shown beside
//   themselves as their code points, as the hostile-markdown renderer shows
//   them (reveal.tsx), and kept in the voice. What the editor would hold out
//   of sight is shown too: a link's address and title beside its text, a
//   code block's info string above it. A link whose address or title holds
//   an invisible character stays text, where its chips show; a code block
//   whose info string holds one keeps the voice out of the editor.
// - The Voice Writer's own Markdown comes back byte for byte (invariant 4):
//   a list item's wrapped lines keep their indent, the file keeps its final
//   newline, and prose keeps its `&`, `<` and `>` as written, never as
//   entities. Where the serializer writes a voice another way (`*` bullets,
//   `_` emphasis), voiceSave sends nothing until the person edits; once they
//   do, every top-level block they did not change is written back as the
//   bytes it was read from, so an edit to one paragraph puts no escape
//   (`a \* b`, `snake\_case`) into another (the site writes the whole voice through the serializer). A voice
//   holding what the schema has no place for (a table, a definition, a task
//   list) is not opened at all, so no edit can send it without that part.
//
// It is loaded only with the editor (VoiceWysiwyg.tsx, through import()),
// so Tiptap is in the editor's own chunk and no other page loads it.
import { Extension, getSchema, type AnyExtension, type EditorOptions, type JSONContent } from "@tiptap/core";
import { Plugin } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import Link from "@tiptap/extension-link";
import { ListItem } from "@tiptap/extension-list";
import { MarkdownManager } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";
import { Marked, Tokenizer } from "marked";

import { isInvisible } from "./invisible.ts";
import { voiceTextProblem, type VoiceSave } from "./voice.ts";

// isAllowedVoiceHref is the one rule for a link in a voice: an absolute
// http(s) or mailto address, as AgentMarkdown's. A control or space
// character anywhere refuses it, so "java\tscript:" cannot pass as text the
// browser would strip, and so does an invisible one, which the address
// shown beside the link could not reveal.
export function isAllowedVoiceHref(href: string): boolean {
  if (/[\u0000- \u007f-\u009f]/.test(href) || hasInvisible(href)) return false;
  return /^https?:\/\/[^/]/i.test(href) || /^mailto:[^/]/i.test(href);
}

function hasInvisible(text: string): boolean {
  for (const ch of text) if (isInvisible(ch.codePointAt(0)!)) return true;
  return false;
}

// A link mark that keeps only its address and title, and is drawn as an
// <a> with the address in data-href and the title in data-title: no href,
// no target, so neither a click nor a modified click inside the editor
// opens anything, and voiceWysiwyg.css shows both beside the link's text.
// Pasted HTML gives a link only for an allowed address, and a title only
// with no invisible character in it.
const VoiceLink = Link.extend({
  addAttributes() {
    return {
      href: { default: null, parseHTML: (el: HTMLElement) => el.getAttribute("href") ?? el.getAttribute("data-href") },
      title: {
        default: null,
        parseHTML: (el: HTMLElement) => {
          const title = el.hasAttribute("data-href") ? el.getAttribute("data-title") : el.getAttribute("title");
          return title && !hasInvisible(title) ? title : null;
        },
      },
    };
  },
  parseHTML() {
    const allowed = (el: HTMLElement | string) => {
      if (typeof el === "string") return false;
      const href = el.getAttribute("href") ?? el.getAttribute("data-href") ?? "";
      return isAllowedVoiceHref(href) ? null : false;
    };
    return [
      { tag: "a[href]", getAttrs: allowed },
      { tag: "a[data-href]", getAttrs: allowed },
    ];
  },
  renderHTML({ mark }) {
    const href = String(mark.attrs.href ?? "");
    const title = mark.attrs.title ? String(mark.attrs.title) : null;
    return ["a", { "data-href": href, "data-title": title, title: title || href }, 0];
  },
  // Markdown's link to any other address, or with an invisible character in
  // its title, stays the text it was written as.
  parseMarkdown(token, helpers) {
    if (!isAllowedVoiceHref(String(token.href ?? "")) || hasInvisible(String(token.title ?? ""))) return { type: "text", text: String(token.raw ?? "") };
    return helpers.applyMark("link", helpers.parseInline(token.tokens || []), { href: token.href, title: token.title || null });
  },
}).configure({
  openOnClick: false,
  enableClickSelection: false,
  autolink: false,
  linkOnPaste: false,
  markdownLinks: false,
  HTMLAttributes: {},
  isAllowedUri: (url) => isAllowedVoiceHref(url),
});

// An image's Markdown is kept as the text it was written as: the schema has
// no image node, and nothing in the editor is fetched.
const ImageAsText = Extension.create({
  name: "voiceImageAsText",
  markdownTokenName: "image",
  parseMarkdown: (token) => ({ type: "text", text: String(token.raw ?? "") }),
});

// Each invisible character gets a chip beside it naming its code point, as
// RevealText draws one. The character itself stays in the text and the
// voice, wrapped alone in an isolate (.voice-invisible, unicode-bidi:
// isolate), so a bidi override or embedding ends where it starts and
// reorders nothing after it. RevealText drops the character; an editor
// cannot, or the voice would change.
const RevealInvisible = Extension.create({
  name: "voiceRevealInvisible",
  addProseMirrorPlugins() {
    return [
      new Plugin({
        props: {
          decorations(state) {
            const chips: Decoration[] = [];
            state.doc.descendants((node, pos) => {
              // A code block's info string, shown above it (voiceWysiwyg.css).
              if (node.type.name === "codeBlock" && node.attrs.language) {
                chips.push(Decoration.node(pos, pos + node.nodeSize, { "data-language": String(node.attrs.language) }));
              }
              if (!node.isText || !node.text) return;
              let offset = 0;
              for (const ch of node.text) {
                const cp = ch.codePointAt(0)!;
                if (isInvisible(cp)) {
                  chips.push(Decoration.inline(pos + offset, pos + offset + ch.length, { class: "voice-invisible" }));
                  chips.push(
                    Decoration.widget(pos + offset, () => {
                      const chip = document.createElement("span");
                      chip.className = "agy-invisible";
                      chip.title = "invisible character revealed";
                      chip.contentEditable = "false";
                      chip.textContent = `U+${cp.toString(16).toUpperCase().padStart(4, "0")}`;
                      return chip;
                    }),
                  );
                }
                offset += ch.length;
              }
            });
            return DecorationSet.create(state.doc, chips);
          },
        },
      }),
    ];
  },
});

// A list item whose first paragraph wraps keeps its wrapped lines indented
// under the marker, as the Voice Writer writes them; the stock renderer
// prefixes only the first line.
const VoiceListItem = ListItem.extend({
  renderMarkdown(node, h, ctx) {
    const out: string = this.parent?.(node, h, ctx) ?? "";
    const first = node.content?.[0];
    if (!first) return out;
    const head = h.renderChildren([first]);
    const at = out.indexOf(head);
    if (at < 0 || !head.includes("\n")) return out;
    const pad = " ".repeat(at);
    const indented = head
      .split("\n")
      .map((line, i) => (i === 0 || line === "" ? line : pad + line))
      .join("\n");
    return out.slice(0, at) + indented + out.slice(at + head.length);
  },
});

export function voiceExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      link: false,
      listItem: false,
      underline: false,
      trailingNode: false,
      codeBlock: { enableTabIndentation: false },
    }),
    VoiceLink,
    VoiceListItem,
    ImageAsText,
    RevealInvisible,
  ];
}

// The Markdown lexer: CommonMark and GFM, with raw HTML never tokenized as
// HTML (block or inline, so it stays text) and a bare URL never turned into
// a link (it stays what was typed). Tiptap decodes &amp;, &lt;, &gt;, &quot;
// and numeric entities in text as it parses; each `&` is escaped first, so
// the decode gives back exactly what was written ("&amp;" stays "&amp;").
function voiceMarked(): Marked {
  const marked = new Marked({ gfm: true });
  marked.use({
    tokenizer: {
      html: () => undefined,
      tag: () => undefined,
      url: () => undefined,
      inlineText(this: Tokenizer, src: string) {
        const token = Tokenizer.prototype.inlineText.call(this, src);
        if (token) token.text = token.text.replace(/&/g, "&amp;");
        return token;
      },
    },
  });
  return marked;
}

// The tokens the editor has a node or a rule for. Anything else (a table, a
// reference or footnote definition, a task list's checkbox) would be dropped
// by the parser, so a voice holding one is not opened in the editor.
const KEPT_TOKENS = new Set([
  "space",
  "paragraph",
  "heading",
  "text",
  "strong",
  "em",
  "del",
  "codespan",
  "code",
  "blockquote",
  "list",
  "list_item",
  "hr",
  "br",
  "escape",
  "link",
  "image",
]);
type TokenTree = { type: string; raw?: string; lang?: string; task?: boolean; tokens?: TokenTree[]; items?: TokenTree[] };
function onlyKeptTokens(tokens: readonly TokenTree[] | undefined): boolean {
  return (tokens ?? []).every((token) => {
    if (!KEPT_TOKENS.has(token.type) || token.task) return false;
    // An info string is shown as an attribute, where a chip cannot go.
    if (token.type === "code" && token.lang && hasInvisible(token.lang)) return false;
    // A list's items, from marked or from the list extension's own
    // tokenizer, are list items whether or not they carry a type.
    if (token.items && !token.items.every((item) => !item.task && onlyKeptTokens(item.tokens))) return false;
    return onlyKeptTokens(token.tokens);
  });
}

let manager: MarkdownManager | undefined;
let schema: ReturnType<typeof getSchema> | undefined;
function voiceMarkdown(): MarkdownManager {
  if (manager) return manager;
  // The option is typed as the marked singleton; a Marked instance has the
  // same Lexer, defaults, lexer and use the manager calls.
  const m = new MarkdownManager({ marked: voiceMarked() as never, extensions: voiceExtensions() });
  // Text is written with Markdown's own escapes and nothing else: the stock
  // serializer also writes &, < and > as entities, which a model reading
  // VOICE.md would read as "&amp;". Code keeps its characters as the stock
  // one does. (A private method of MarkdownManager; the AS_WRITTEN cases of
  // voice-editor.test.ts fail if a Tiptap upgrade stops calling it.)
  const internals = m as unknown as {
    codeTypes: Set<string>;
    escapeMarkdownSyntax: (text: string) => string;
    encodeTextForMarkdown: (text: string, node: JSONContent, parent?: JSONContent) => string;
  };
  internals.encodeTextForMarkdown = (text, node, parent) => {
    const inCode =
      (parent?.type != null && internals.codeTypes.has(parent.type)) ||
      (node.marks ?? []).some((mark) => internals.codeTypes.has(typeof mark === "string" ? mark : mark.type));
    return inCode ? text : internals.escapeMarkdownSyntax(text);
  };
  manager = m;
  return m;
}
function voiceSchema() {
  schema ??= getSchema(voiceExtensions());
  return schema;
}

// A paragraph's wrapped lines lose their leading spaces, as CommonMark
// reads them; the list tokenizer leaves some behind. Code keeps its own.
function tidy(node: JSONContent, inCode = false): JSONContent {
  const code = inCode || node.type === "codeBlock";
  if (node.type === "text" && !code && !node.marks?.some((m) => m.type === "code") && node.text) {
    return { ...node, text: node.text.replace(/\n[ \t]+/g, "\n") };
  }
  return node.content ? { ...node, content: node.content.map((c) => tidy(c, code)) } : node;
}

// Empty paragraphs (two presses of Enter) have no Markdown; the serializer
// would write them as &nbsp;.
function withoutEmptyParagraphs(node: JSONContent): JSONContent {
  if (!node.content) return node;
  const content = node.content
    .filter((c) => !(c.type === "paragraph" && !c.content?.length))
    .map(withoutEmptyParagraphs);
  return { ...node, content };
}

const EMPTY_DOC: JSONContent = { type: "doc", content: [{ type: "paragraph" }] };

// voiceToDoc reads VOICE.md into the editor's document, checked against the
// editor's schema, or null when the schema cannot hold it (the page then
// keeps the plain textarea rather than open a voice it would change).
export function voiceToDoc(text: string): JSONContent | null {
  if (text.trim() === "") return voiceSchema().nodeFromJSON(EMPTY_DOC).toJSON();
  try {
    if (!onlyKeptTokens(voiceMarkdown().instance.lexer(text) as unknown as TokenTree[])) return null;
    const node = voiceSchema().nodeFromJSON(tidy(voiceMarkdown().parse(text)));
    node.check();
    return node.toJSON();
  } catch {
    return null;
  }
}

// A voice's top-level blocks as it was read: each block's document node
// (as JSON, to compare) and its source bytes, and the bytes between it and
// the next. marked's tokens hold every byte of the source, and the parser
// gives one top-level node per token that is not space; where they do not
// line up, nothing is kept and the serializer writes the whole voice.
type SourceBlock = { json: string; raw: string; gap: string };
let lastSource: { text: string; blocks: SourceBlock[] | null } | undefined;
function sourceBlocks(text: string): SourceBlock[] | null {
  if (lastSource?.text === text) return lastSource.blocks;
  let blocks: SourceBlock[] | null = null;
  const doc = voiceToDoc(text);
  if (doc && text.trim() !== "") {
    const nodes = withoutEmptyParagraphs(doc).content ?? [];
    const tokens = voiceMarkdown().instance.lexer(text) as unknown as TokenTree[];
    const found: SourceBlock[] = [];
    for (const token of tokens) {
      const raw = String(token.raw ?? "");
      if (token.type === "space") {
        if (found.length) found[found.length - 1].gap += raw;
        continue;
      }
      const body = raw.replace(/\n+$/, "");
      found.push({ json: "", raw: body, gap: raw.slice(body.length) });
    }
    if (found.length === nodes.length) {
      nodes.forEach((node, i) => (found[i].json = JSON.stringify(node)));
      blocks = found;
    }
  }
  lastSource = { text, blocks };
  return blocks;
}

function serializeBlocks(nodes: JSONContent[]): string {
  return voiceMarkdown().serialize({ type: "doc", content: nodes }).replace(/\n+$/, "");
}

// docToVoice writes the editor's document as VOICE.md, ending in one
// newline. Given the voice it was opened from, each top-level block equal to
// one of that voice's, in order, is written as the bytes it was read from,
// with the bytes that followed it when the next block is its neighbour
// there too; the blocks between are written by the serializer.
export function docToVoice(doc: JSONContent, opened?: string): string {
  const clean = withoutEmptyParagraphs(doc);
  const source = opened === undefined ? null : sourceBlocks(opened);
  let text: string;
  if (!source) {
    text = voiceMarkdown().serialize(clean);
  } else {
    const nodes = clean.content ?? [];
    const pieces: string[] = [];
    let next = 0; // the first source block a later node may match
    let prev = -1; // the source block the previous node matched, or -1
    let run: JSONContent[] = []; // nodes waiting to be serialized together
    const flush = () => {
      if (!run.length) return;
      if (pieces.length) pieces.push("\n\n");
      pieces.push(serializeBlocks(run));
      run = [];
    };
    for (const node of nodes) {
      const json = JSON.stringify(node);
      let at = -1;
      for (let i = next; i < source.length; i++) {
        if (source[i].json === json) {
          at = i;
          break;
        }
      }
      if (at < 0) {
        run.push(node);
        prev = -1;
        continue;
      }
      const neighbour = !run.length && prev >= 0 && at === prev + 1;
      flush();
      if (pieces.length) pieces.push(neighbour ? source[prev].gap : "\n\n");
      pieces.push(source[at].raw);
      prev = at;
      next = at + 1;
    }
    flush();
    text = pieces.join("");
  }
  return text.trim() === "" ? "" : text.replace(/\n*$/, "\n");
}

// voiceSave is Save's decision: nothing when the person has not changed the
// voice they opened (however the serializer would write it), the size rule
// on the Markdown that would be sent (invariant 5), else that Markdown.
export function voiceSave(opened: string, doc: JSONContent): VoiceSave {
  const text = docToVoice(doc, opened);
  const before = voiceToDoc(opened);
  if (text === opened || (before && (text === docToVoice(before) || text === docToVoice(before, opened)))) return { kind: "unchanged" };
  const problem = voiceTextProblem(text);
  return problem ? { kind: "problem", message: problem } : { kind: "send", text };
}

// voiceEditorOptions is the Editor's configuration, the page's and the
// tests'. injectCSS is off: Tiptap would add a <style> element of its own,
// so its few rules ship in voiceWysiwyg.css, in the editor's chunk, and the
// window's Content-Security-Policy (internal/webview) stays as it is
// (invariant 3).
export function voiceEditorOptions(doc: JSONContent): Partial<EditorOptions> {
  return {
    extensions: voiceExtensions(),
    content: doc,
    injectCSS: false,
    enableContentCheck: true,
    editorProps: {
      attributes: {
        class: "voice-wysiwyg readme",
        role: "textbox",
        "aria-multiline": "true",
        "aria-label": "VOICE.md",
        spellcheck: "true",
      },
    },
  };
}
