// The browser's site in the run setup's Details step: a url question the
// release's browser names, asked as the address of the site to test. No
// token: nothing is fetched, the browser opens it, and the CLI hands it to
// the run's browse proxy, which admits it and its subdomains.
import { siteProblem } from "../browser";

export function SiteQuestion({
  q, value, onChange, disabled,
}: {
  q: { id: string; prompt: string; required: boolean };
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const problem = value.trim() === "" ? null : siteProblem(value);
  return (
    <div style={{ marginBottom: 8 }}>
      <div className="row">
        <span style={{ minWidth: 260 }}>{q.prompt}{q.required ? " *" : ""}</span>
        <input
          className="grow mono"
          type="url"
          inputMode="url"
          spellCheck={false}
          autoComplete="off"
          aria-label={q.prompt}
          placeholder="https://staging.example.com"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        />
      </div>
      <p className="muted small" style={{ margin: "4px 0 0 260px" }}>
        {problem
          ? `This address ${problem}.`
          : "The site to test, in a web browser the run drives: it opens this site and its subdomains, and the hosts the QuickAgent lists. A staging server on your network works too."}
      </p>
    </div>
  );
}
