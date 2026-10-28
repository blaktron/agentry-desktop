// The third-party notices file (THIRD_PARTY_NOTICES) as the Licences page
// reads it: a preface, then numbered sections, each opened by a title between
// two rules of "=" signs. The page shows one collapsible block per section,
// so the file's quarter of a megabyte is drawn only for the section a person
// opens.

export type NoticeSection = {
  // "1. Interface packages": the number the contents list gives, and the name.
  title: string;
  // The section's text, without its heading and without blank lines around it.
  body: string;
};

export type Notices = {
  // Everything before the first section: the title, what the file is, the contents.
  preface: string;
  sections: NoticeSection[];
};

const RULE = /^={20,}$/;
// A section's title is numbered, as the contents list numbers it: a licence
// text quoted in a section may frame its own headings with rules too.
const TITLE = /^[0-9]+\. \S/;

const trimBlank = (lines: string[]) => {
  let a = 0;
  let b = lines.length;
  while (a < b && lines[a].trim() === "") a++;
  while (b > a && lines[b - 1].trim() === "") b--;
  return lines.slice(a, b).join("\n");
};

// splitNotices reads the file's sections. A text without any (a file this
// reader does not know) comes back whole as the preface, so the page still
// shows it.
export function splitNotices(text: string): Notices {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const heads: { at: number; title: string }[] = [];
  for (let i = 0; i + 2 < lines.length; i++) {
    if (RULE.test(lines[i]) && TITLE.test(lines[i + 1].trim()) && RULE.test(lines[i + 2])) {
      heads.push({ at: i, title: lines[i + 1].trim() });
      i += 2;
    }
  }
  if (heads.length === 0) return { preface: trimBlank(lines), sections: [] };
  const sections = heads.map((h, k) => ({
    title: h.title,
    body: trimBlank(lines.slice(h.at + 3, k + 1 < heads.length ? heads[k + 1].at : lines.length)),
  }));
  return { preface: trimBlank(lines.slice(0, heads[0].at)), sections };
}

// licenceCount is how many separate licence texts a section holds: its blocks
// are set apart by a rule of "-" signs. The page says it beside the title.
export function licenceCount(body: string): number {
  return body.split("\n").filter((l) => /^-{20,}$/.test(l)).length;
}
