// Invisible characters in text someone else wrote (the GlassWorm lesson):
// zero-width, bidi, tag-block, private-use and other format characters are
// shown as visible chips, so a reader never sees less than is there.
//
// The list is the site's `isInvisible` (agentry-web lib/reveal.tsx), which
// mirrors the core's internal/render; the three must stay identical, or a
// character revealed by one and not another is a hiding place. Here it
// serves the run result view (agentry-desktop#227): an agent's final
// message, its files' previews and their names.

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

// RevealText renders text with every invisible character as a chip naming
// its code point. React escapes the rest; nothing here emits markup.
export function RevealText({ text }: { text: string }) {
  const parts: React.ReactNode[] = [];
  let run = "";
  let key = 0;
  for (const ch of text) {
    const cp = ch.codePointAt(0)!;
    if (!isInvisible(cp)) {
      run += ch;
      continue;
    }
    if (run) parts.push(run);
    run = "";
    parts.push(
      <span key={key++} className="agy-invisible" title="invisible character revealed">
        U+{cp.toString(16).toUpperCase().padStart(4, "0")}
      </span>,
    );
  }
  if (run) parts.push(run);
  return <>{parts}</>;
}

// plainName is a name with every invisible character replaced by "_": what
// a label says when it cannot hold a chip (an aria-label, a title).
export function plainName(name: string): string {
  let out = "";
  for (const ch of name) out += isInvisible(ch.codePointAt(0)!) ? "_" : ch;
  return out;
}
