// Invisible characters in text someone else wrote (the GlassWorm attack):
// zero-width, bidi, tag-block, private-use and other format characters are
// shown as visible chips, so a reader never sees less than is there.
//
// The list is the site's `isInvisible`, which mirrors the core's
// internal/render; the three must stay identical, or a character revealed
// by one and not another is a hiding place. It serves the run result view
// through reveal.tsx (an agent's final message, its files' previews and
// their names) and the voice editor through voiceMarkdown.ts. It is a
// module of its own, with no JSX, so node's test runner and the editor's
// chunk can import it.

const CF_RE = /\p{Cf}/u;

export function isInvisible(cp: number): boolean {
  if (cp === 0x200b || cp === 0x200c || cp === 0x200d || cp === 0x2060 || cp === 0xfeff) return true; // zero-width
  if ((cp >= 0x202a && cp <= 0x202e) || (cp >= 0x2066 && cp <= 0x2069)) return true; // bidi controls
  if (cp >= 0xe0000 && cp <= 0xe007f) return true; // Unicode tag block
  if ((cp >= 0xe000 && cp <= 0xf8ff) || (cp >= 0xf0000 && cp <= 0xffffd) || (cp >= 0x100000 && cp <= 0x10fffd))
    return true; // private use areas
  if (cp === 0x00ad) return true; // soft hyphen
  return CF_RE.test(String.fromCodePoint(cp)); // any other format character
}
