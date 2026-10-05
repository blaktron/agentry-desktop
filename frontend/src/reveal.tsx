// The invisible-character reveal (the GlassWorm lesson; the list itself is
// invisible.ts): every invisible character in text someone else wrote is
// drawn as a chip naming its code point.
import { isInvisible } from "./invisible";

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
