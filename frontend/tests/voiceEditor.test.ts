// The Voice pages' WYSIWYG editor (plan voice-page-redesign.md §6, D1,
// invariants 2-5; agentry-desktop#612, the site's voice-editor.test.ts of
// agentry-web#1070 under node's runner). It reads VOICE.md as Markdown and
// saves Markdown, in one view, so two things must hold before it replaces
// the textarea: a voice the Voice Writer wrote comes back byte for byte (and
// where the serializer would normalize, Save sends nothing until the person
// edits), and Markdown an agent wrote carries no markup of its own into the
// editor and makes no request from it.
//
// Every case goes through a real Editor in jsdom with the page's own
// options, so ProseMirror's schema, not only the Markdown layer, decides
// what survives.
import { window } from "./dom.ts";
import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const { Editor } = await import("@tiptap/core");
const { docToVoice, isAllowedVoiceHref, voiceEditorOptions, voiceSave, voiceToDoc } = await import("../src/voiceMarkdown.ts");
const { MAX_VOICE_BYTES } = await import("../src/voice.ts");
type Editor = InstanceType<typeof Editor>;

const HERE = dirname(fileURLToPath(import.meta.url));
const FIXTURES = join(HERE, "fixtures", "voice");
const fixture = (name: string) => readFileSync(join(FIXTURES, name), "utf8");
const FIXTURE_NAMES = readdirSync(FIXTURES).filter((n) => n.endsWith(".md"));

const editors: Editor[] = [];
function open(markdown: string, label?: string): Editor {
  const doc = voiceToDoc(markdown);
  if (!doc) throw new Error("the editor cannot hold this voice");
  const editor = new Editor({ ...voiceEditorOptions(doc, label), element: document.createElement("div") });
  editors.push(editor);
  return editor;
}
afterEach(() => {
  while (editors.length) editors.pop()!.destroy();
});

// The Markdown an editor holds now, as Save would send it.
const markdownOf = (editor: Editor) => docToVoice(editor.getJSON());
const paragraph = (text: string) => ({ type: "paragraph", content: [{ type: "text", text }] });

describe("the voice editor's round trip", () => {
  test("has the Voice Writer's real outputs to read", () => {
    // Four runs of agentry/voice-writer@1.1.0 (PROVENANCE.txt): three
    // Creates and an AI Edit with a sample.
    assert.equal(FIXTURE_NAMES.filter((n) => n.startsWith("writer-")).length, 4);
  });

  for (const name of FIXTURE_NAMES) {
    test(`gives back ${name} byte for byte`, () => {
      const src = fixture(name);
      assert.equal(markdownOf(open(src)), src);
    });

    test(`sends nothing for ${name} when the person has not edited it`, () => {
      const src = fixture(name);
      assert.deepEqual(voiceSave(src, open(src).getJSON()), { kind: "unchanged" });
    });
  }

  // Markdown the serializer writes another way: a different list marker, the
  // other emphasis character, a `)` list, lazy continuation lines, no final
  // newline, a setext heading. The voice means the same, so an untouched
  // editor must not send the rewrite.
  for (const src of [
    "* a star list\n* two\n",
    "+ a plus list\n",
    "_under_ and __strong__\n",
    "1) a paren list\n2) two\n",
    "- lazy\ncontinuation\n",
    "No final newline",
    "Setext heading\n==============\n",
  ]) {
    test(`sends nothing for ${JSON.stringify(src)} untouched, though it would write it another way`, () => {
      const editor = open(src);
      assert.notEqual(markdownOf(editor), src);
      assert.deepEqual(voiceSave(src, editor.getJSON()), { kind: "unchanged" });
    });
  }

  // Prose the model reads as written: an ampersand, angle brackets, an
  // arrow, an entity, raw HTML and struck text come back as they were, and an
  // edit elsewhere does not turn them into entities or drop their markers.
  for (const src of [
    "R&D, Q&A and AT&T stay as they are.\n",
    "Write a < b and b > a, and -> for a step.\n",
    "Keep &copy; and &amp; as typed.\n",
    "Keep <b>this</b> as text.\n",
    "Never ~~strike~~ a word out.\n",
  ]) {
    test(`gives back ${JSON.stringify(src)} as written, and keeps it so after an edit`, () => {
      const editor = open(src);
      assert.equal(markdownOf(editor), src);
      editor.commands.insertContentAt(editor.state.doc.content.size, paragraph("More."));
      assert.deepEqual(voiceSave(src, editor.getJSON()), { kind: "send", text: `${src}\nMore.\n` });
    });
  }

  // What the schema has no place for (a table, a reference definition, a
  // task list) is never opened in the editor, where an edit would send the
  // voice without it: the page keeps the textarea.
  for (const src of [
    "| a | b |\n| - | - |\n| 1 | 2 |\n\nAfter.\n",
    "See [the guide][g].\n\n[g]: https://example.com/guide\n",
    "- [ ] a task\n- [x] done\n",
  ]) {
    test(`does not open ${JSON.stringify(src)}, so nothing in it can be lost`, () => {
      assert.equal(voiceToDoc(src), null);
    });
  }

  test("keeps a footnote's text, which CommonMark reads as text", () => {
    const editor = open("A claim.[^1]\n\n[^1]: The footnote.\n");
    assert.ok(editor.getText().includes("[^1]: The footnote."));
    assert.equal(markdownOf(editor), "A claim.\\[^1\\]\n\n\\[^1\\]: The footnote.\n");
  });

  test("sends the Markdown once the person edits, and nothing again after an undo", () => {
    const src = fixture("writer-create-casual.md");
    const editor = open(src);
    editor.commands.insertContentAt(editor.state.doc.content.size, paragraph("One more rule."));
    assert.deepEqual(voiceSave(src, editor.getJSON()), { kind: "send", text: `${src}\nOne more rule.\n` });
    editor.commands.undo();
    assert.deepEqual(voiceSave(src, editor.getJSON()), { kind: "unchanged" });
  });

  test("checks 32 KiB on the serialized bytes, not on what the person sees", () => {
    // Each `*` is written back escaped (`\*`), so the text the editor shows
    // is under the cap and the Markdown Save would send is past it.
    const editor = open("Start.\n");
    editor.commands.insertContentAt(editor.state.doc.content.size, paragraph("*".repeat(MAX_VOICE_BYTES / 2 + 16)));
    assert.ok(editor.state.doc.textContent.length < MAX_VOICE_BYTES);
    const save = voiceSave("Start.\n", editor.getJSON());
    assert.equal(save.kind, "problem");
    if (save.kind === "problem") assert.match(save.message, /at most 32 KiB/);
  });

  test("will not save an emptied voice", () => {
    const editor = open("# Voice\n\nShort sentences.\n");
    editor.commands.clearContent();
    assert.deepEqual(voiceSave("# Voice\n\nShort sentences.\n", editor.getJSON()), { kind: "problem", message: "Write the voice before you save it." });
  });

  test("opens an empty voice as an empty editor, and a first voice is sent with a final newline", () => {
    const editor = open("");
    assert.deepEqual(voiceSave("", editor.getJSON()), { kind: "unchanged" });
    editor.commands.insertContent("Write plainly.");
    assert.deepEqual(voiceSave("", editor.getJSON()), { kind: "send", text: "Write plainly.\n" });
  });

  test("injects no style element: its CSS ships with the editor's chunk, and the window's policy does not change", () => {
    const before = document.head.querySelectorAll("style").length;
    open(fixture("hand-every-node.md"));
    assert.equal(voiceEditorOptions(voiceToDoc("")!).injectCSS, false);
    assert.equal(document.head.querySelectorAll("style").length, before);
    assert.equal(document.querySelector("style[data-tiptap-style]"), null);
  });
});

describe("the voice editor holds hostile Markdown as text", () => {
  // What the editor's schema may hold at all: no image node, no raw HTML
  // node, no table, nothing that loads or runs.
  test("knows only the nodes and marks a VOICE.md uses", () => {
    const { schema } = open("");
    assert.deepEqual(
      Object.keys(schema.nodes).sort(),
      ["blockquote", "bulletList", "codeBlock", "doc", "hardBreak", "heading", "horizontalRule", "listItem", "orderedList", "paragraph", "text"].sort(),
    );
    assert.deepEqual(Object.keys(schema.marks).sort(), ["bold", "code", "italic", "link", "strike"].sort());
  });

  test("keeps raw HTML, script included, as literal text and never as a node", () => {
    const src =
      'before <script>alert(1)</script> <b onclick="x()">bold</b> <img src="https://evil.example/p.gif" onerror="x()"> <a href="https://evil.example">a</a>\n\n<div>\n<iframe src="https://evil.example"></iframe>\n</div>\n\n<style>body{display:none}</style>\n';
    const editor = open(src);
    const dom = editor.view.dom as HTMLElement;
    assert.equal(dom.querySelector("script, b, img, iframe, div, style, a"), null);
    assert.ok(dom.textContent!.includes("<script>alert(1)</script>"));
    assert.ok(dom.textContent!.includes('<b onclick="x()">bold</b>'));
    assert.ok(dom.textContent!.includes("<iframe"));
    assert.doesNotMatch(JSON.stringify(editor.getJSON()), /"type":"(image|html|link)"/);
    // Untouched, the text goes nowhere; edited, every tag is written back as
    // the text it was, never as an entity.
    assert.deepEqual(voiceSave(src, editor.getJSON()), { kind: "unchanged" });
    const md = markdownOf(editor);
    for (const tag of ["<script>alert(1)</script>", '<b onclick="x()">bold</b>', '<iframe src="https://evil.example"></iframe>', "<style>body{display:none}</style>"]) {
      assert.ok(md.includes(tag), tag);
    }
    assert.ok(!md.includes("&lt;"));
  });

  test("has no image: an image's Markdown stays text, and nothing is fetched", () => {
    const src = "Look: ![tracking pixel](https://evil.example/p.gif) and ![](data:image/png;base64,AAAA)\n";
    const editor = open(src);
    const dom = editor.view.dom as HTMLElement;
    assert.equal(dom.querySelector("img"), null);
    assert.ok(dom.textContent!.includes("![tracking pixel](https://evil.example/p.gif)"));
    assert.ok(!JSON.stringify(editor.getJSON()).includes('"image"'));
  });

  test("allows a link only to an absolute http(s) or mailto address", () => {
    for (const ok of ["https://example.com/a", "http://example.com", "HTTPS://EXAMPLE.COM", "mailto:me@example.com"]) {
      assert.equal(isAllowedVoiceHref(ok), true, ok);
    }
    for (const bad of [
      "javascript:alert(1)",
      "JavaScript:alert(1)",
      " javascript:alert(1)",
      "java\tscript:alert(1)",
      "data:text/html,<script>alert(1)</script>",
      "vbscript:x",
      "/relative",
      "relative/path",
      "//evil.example/x",
      "#anchor",
      "file:///etc/passwd",
      "https:no-slashes",
      "",
    ]) {
      assert.equal(isAllowedVoiceHref(bad), false, JSON.stringify(bad));
    }
  });

  test("keeps any other link's Markdown as text, never as a link", () => {
    const src = "[run](javascript:alert(1)) [rel](/x) [data](data:text/html,x) [ok](https://example.com) [mail](mailto:a@example.com)\n";
    const editor = open(src);
    const hrefs = JSON.stringify(editor.getJSON()).match(/"href":"[^"]*"/g);
    assert.deepEqual(hrefs, ['"href":"https://example.com"', '"href":"mailto:a@example.com"']);
    assert.ok((editor.view.dom as HTMLElement).textContent!.includes("[run](javascript:alert(1))"));
  });

  test("draws a link with no href, so no click inside the editor can open it", () => {
    const editor = open("See [the guide](https://example.com/guide).\n");
    const dom = editor.view.dom as HTMLElement;
    assert.equal(dom.querySelector("[href]"), null);
    const link = dom.querySelector("a");
    assert.equal(link?.getAttribute("data-href"), "https://example.com/guide");
    assert.equal(link?.hasAttribute("target"), false);
    let opened = 0;
    const realOpen = window.open;
    window.open = () => {
      opened++;
      return null;
    };
    try {
      const before = window.location.href;
      for (const init of [{}, { ctrlKey: true }, { metaKey: true }, { button: 1 }]) {
        link!.dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true, ...init }));
        link!.dispatchEvent(new window.MouseEvent("auxclick", { bubbles: true, cancelable: true, ...init }));
      }
      assert.equal(opened, 0);
      assert.equal(window.location.href, before);
    } finally {
      window.open = realOpen;
    }
    // And it is still a link in the Markdown Save sends.
    assert.equal(markdownOf(editor), "See [the guide](https://example.com/guide).\n");
  });

  test("drops hostile markup pasted as HTML: no script link, no image, no handler", () => {
    const editor = open("Start.\n");
    // A real paste: ProseMirror's clipboard parser, into the editor's schema
    // (jsdom has no ClipboardEvent, so a plain paste event stands in).
    editor.commands.focus("end");
    editor.view.pasteHTML(
      '<p><a href="javascript:alert(1)">x</a> <img src="https://evil.example/p.gif"> <span onclick="x()">y</span> <a href="https://example.com">z</a></p><script>alert(1)</script>',
      new window.Event("paste") as unknown as ClipboardEvent,
    );
    assert.ok(editor.getText().includes("z"));
    const json = JSON.stringify(editor.getJSON());
    assert.ok(!json.includes("javascript:"));
    assert.ok(!json.includes("image"));
    assert.ok(!json.includes("onclick"));
    assert.ok(json.includes('"href":"https://example.com"'));
    assert.equal((editor.view.dom as HTMLElement).querySelector("script, img, [onclick], [href]"), null);
  });

  test("shows each invisible character as a visible code point, and keeps it in the voice", () => {
    // DEV-138's reveal, as the Preview drew it: zero-width, bidi, tag.
    const src = "Plain​ text ‮evil‬ and a tag\u{e0041}.\n";
    const editor = open(src);
    const chips = [...(editor.view.dom as HTMLElement).querySelectorAll(".agy-invisible")].map((el) => el.textContent);
    assert.deepEqual(chips, ["U+200B", "U+202E", "U+202C", "U+E0041"]);
    // Each character is kept, alone in an isolate, so a bidi override
    // reverses nothing after it (on the site's dev check, U+202E reversed
    // the rest of the line, its chips included).
    const isolated = [...(editor.view.dom as HTMLElement).querySelectorAll(".voice-invisible")].map((el) => el.textContent);
    assert.deepEqual(isolated, ["​", "‮", "‬", "\u{e0041}"]);
    assert.equal(markdownOf(editor), src);
  });

  test("turns a typed URL into nothing but text (no autolink)", () => {
    const editor = open("");
    editor.commands.insertContent("see https://example.com ");
    assert.ok(!JSON.stringify(editor.getJSON()).includes('"link"'));
  });

  test("names the text VOICE.md for a screen reader", () => {
    assert.equal(open("Hello.\n").view.dom.getAttribute("aria-label"), "VOICE.md");
  });
});

describe("the voice editor's code", () => {
  // Tiptap is loaded only where a voice is edited: the Voice page's editor
  // import()s its view, which brings the Markdown module, once it is drawn.
  // So the app's main bundle does not carry it.
  const SRC = join(HERE, "..", "src");
  function sources(dir: string): string[] {
    return readdirSync(join(SRC, dir), { withFileTypes: true }).flatMap((e) => {
      const path = dir ? `${dir}/${e.name}` : e.name;
      if (e.isDirectory()) return sources(path);
      return /\.(tsx?|jsx?|mjs)$/.test(e.name) ? [path] : [];
    });
  }
  const files = sources("").map((path) => ({ path, src: readFileSync(join(SRC, path), "utf8") }));
  const importing = (pattern: RegExp) => files.filter((f) => pattern.test(f.src)).map((f) => f.path).sort();

  test("is imported only by the editor's own two modules", () => {
    assert.deepEqual(importing(/^import (?!type )[^;]*from\s+["'](@tiptap\/[^"']+|marked)["']|import\(\s*["'](@tiptap\/|marked)/m), ["VoiceWysiwyg.tsx", "voiceMarkdown.ts"]);
    // The Voice page takes only types from Tiptap.
    const page = files.find((f) => f.path === "pages/Voice.tsx")!.src;
    assert.ok(page.match(/^import .*@tiptap.*$/gm)?.every((l) => l.startsWith("import type")));
  });

  test("reaches the view only through import(), from the Voice page's editor", () => {
    const value = (name: string) => new RegExp(`^import (?!type )[^;]*["'][./]+/?${name}(\\.tsx?)?["']`, "m");
    assert.deepEqual(importing(value("VoiceWysiwyg")), []);
    assert.deepEqual(importing(value("voiceMarkdown")), ["VoiceWysiwyg.tsx"]);
    assert.deepEqual(importing(/import\(\s*["']\.\.\/VoiceWysiwyg(\.tsx)?["']\)/), ["pages/Voice.tsx"]);
  });
});
