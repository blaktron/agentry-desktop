import { useEffect, useMemo, useState } from "react";
import * as NoticesAPI from "../../wailsjs/go/app/NoticesAPI";
import { Card, ErrorBox } from "../ui";
import { licenceCount, splitNotices, type NoticeSection } from "../licences";

// Settings → Licences: the notices file every package of Agentry Desktop
// ships beside the app, read from the copy inside it. One block per
// section, closed until it is opened, so the page draws the text a person
// asks for and not the whole file.

function Section({ section }: { section: NoticeSection }) {
  const [open, setOpen] = useState(false);
  const blocks = licenceCount(section.body);
  return (
    <details className="notices-section" open={open} onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}>
      <summary>
        <strong>{section.title}</strong>
        {blocks > 0 && <span className="muted small"> · {blocks} licence {blocks === 1 ? "text" : "texts"}</span>}
      </summary>
      {open && <pre className="notices-text selectable" tabIndex={0}>{section.body}</pre>}
    </details>
  );
}

export function LicencesSettings() {
  const [text, setText] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => NoticesAPI.Text()).then((t) => { if (live) setText(t); }).catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, []);

  const notices = useMemo(() => (text === null ? null : splitNotices(text)), [text]);

  if (err) return <ErrorBox error={`The licences could not be read: ${err}`} />;
  if (!notices) return <div className="empty">Reading the licences…</div>;

  return (
    <section id="settings-part-licences" className="settings-part">
      <h2>Third-party notices</h2>
      <Card>
        <p className="muted small" style={{ marginTop: 0 }}>
          The same file is installed beside the app by every package. Open a section to read its licence texts.
        </p>
        {notices.sections.length === 0 ? (
          <pre className="notices-text selectable" tabIndex={0}>{notices.preface}</pre>
        ) : (
          notices.sections.map((s) => <Section key={s.title} section={s} />)
        )}
      </Card>
    </section>
  );
}
