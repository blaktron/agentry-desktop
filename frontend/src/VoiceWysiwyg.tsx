// The Writing Voice's WYSIWYG editor, its view (plan voice-page-redesign.md
// §6, D1; agentry-desktop#612, the site's components/voice-wysiwyg.tsx of
// agentry-web#1070): Tiptap v3 with a small toolbar, the Markdown input
// rules (`## `, `- `, `**`) and keyboard shortcuts, one view with no
// Preview. The Voice page's editor (pages/Voice.tsx) imports it only with
// import(), so Tiptap is in this module's chunk and nowhere else. Every rule
// about what the editor holds and what Save sends is voiceMarkdown.ts's.
import { EditorContent, useEditor, useEditorState, type Editor } from "@tiptap/react";
import type { JSONContent } from "@tiptap/core";
import { useEffect } from "react";
import { docToVoice, voiceEditorOptions, voiceSave, voiceToDoc, type VoiceSave } from "./voiceMarkdown.ts";
import "./voiceWysiwyg.css";

export { voiceToDoc };

export type VoiceWysiwygHandle = {
  /** Save's decision on the editor's text now. */
  save: () => VoiceSave;
};

export default function VoiceWysiwyg({ opened, doc, describedBy, onReady, onBytes }: {
  /** The voice as the page opened it, the baseline Save compares with. */
  opened: string;
  /** The document to start from (the opened voice, or what was typed while the editor loaded). */
  doc: JSONContent;
  describedBy: string;
  onReady: (handle: VoiceWysiwygHandle) => void;
  /** The Markdown's size in bytes after each change, for the counter. */
  onBytes: (bytes: number) => void;
}) {
  const editor = useEditor({
    ...voiceEditorOptions(doc),
    onUpdate: ({ editor }) => onBytes(new TextEncoder().encode(docToVoice(editor.getJSON())).length),
  });

  useEffect(() => {
    if (!editor) return;
    editor.view.dom.setAttribute("aria-describedby", describedBy);
    onReady({ save: () => voiceSave(opened, editor.getJSON()) });
  }, [editor, opened, describedBy, onReady]);

  return (
    <div className="voice-wysiwyg-frame">
      {editor && <Toolbar editor={editor} />}
      <EditorContent editor={editor} />
    </div>
  );
}

// The toolbar: heading, bold, italic, list, numbered list, quote, code. Each
// button says whether it is on where the cursor is.
function Toolbar({ editor }: { editor: Editor }) {
  const on = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      heading: e.isActive("heading", { level: 2 }),
      bold: e.isActive("bold"),
      italic: e.isActive("italic"),
      bulletList: e.isActive("bulletList"),
      orderedList: e.isActive("orderedList"),
      blockquote: e.isActive("blockquote"),
      code: e.isActive("code"),
    }),
  });
  const buttons: { key: keyof typeof on; label: string; text: string; run: () => boolean }[] = [
    { key: "heading", label: "Heading", text: "H", run: () => editor.chain().focus().toggleHeading({ level: 2 }).run() },
    { key: "bold", label: "Bold", text: "B", run: () => editor.chain().focus().toggleBold().run() },
    { key: "italic", label: "Italic", text: "I", run: () => editor.chain().focus().toggleItalic().run() },
    { key: "bulletList", label: "List", text: "•", run: () => editor.chain().focus().toggleBulletList().run() },
    { key: "orderedList", label: "Numbered list", text: "1.", run: () => editor.chain().focus().toggleOrderedList().run() },
    { key: "blockquote", label: "Quote", text: "“", run: () => editor.chain().focus().toggleBlockquote().run() },
    { key: "code", label: "Code", text: "</>", run: () => editor.chain().focus().toggleCode().run() },
  ];
  return (
    <div role="toolbar" aria-label="Formatting" className="voice-wysiwyg-toolbar">
      {buttons.map((b) => (
        <button key={b.key} type="button" className="ghost" aria-label={b.label} title={b.label} aria-pressed={on[b.key]} onClick={b.run}>
          <span aria-hidden="true">{b.text}</span>
        </button>
      ))}
    </div>
  );
}
