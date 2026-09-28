// A release guide's help for a key (DEV-167; plan builder-simplify.md §4.4,
// agentry-desktop#343) is markdown an author wrote for a person to read
// before a run, never the agent. It is drawn with AgentMarkdown's rules (no
// raw HTML, no image, a link only to the system browser, invisible characters
// shown), with each link's host beside it, and inside a frame captioned as
// the author's, so a heading, a list or a task-list box in it cannot pass for
// the app's own fields around it. AuthorHelp draws one key's help (the run
// panel's key step, the tools panel); KeyHelpList draws the Build tab's rows
// (the guide step's preview, Review).
import { AgentMarkdown } from "./AgentMarkdown";

export function AuthorHelp({ name, help }: { name: string; help: string }) {
  return (
    <div className="key-help">
      <p className="muted small key-help-caption">
        The author&apos;s help for <span className="mono">{name}</span>
      </p>
      <AgentMarkdown text={help} showHosts />
    </div>
  );
}

export function KeyHelpList({ rows }: { rows: { secret: string; help: string }[] }) {
  const shown = rows.filter((r) => r.secret.trim() && r.help.trim());
  if (shown.length === 0) return null;
  return (
    <div className="key-help-list">
      {/* By place, not by name: the form may still name one key twice. */}
      {shown.map((r, i) => (
        <AuthorHelp key={i} name={r.secret.trim()} help={r.help.trim()} />
      ))}
    </div>
  );
}
