import { useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { app, reportdesign } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice, Select } from "../ui";
import {
  canonical, edit, fieldLabel, fontsFor, HEADING_LABELS, labelOf, PAGE_NUMBER_LABELS, PAPER_LABELS, QUOTE_LABELS,
  ratioText, sameDesign, savedSentence, standingSentence, startFrom, THEME_LOOKS, TITLE_BLOCK_LABELS, toDocument,
  cssColour, isLogoSrc, type FontRole, type ReportDesign,
} from "../reportDesign";
import { ReportDesignPreview } from "./ReportDesignPreview";

// Settings → Report design: the site's editor in the app, editing the same
// setting, the account's report design. The five themes as thumbnails the
// runner's template drew; choosing one loads it into the editor, whose controls
// are exactly the fields of version 2 (reportDesign.ts CONTROLS); beside them a
// live HTML likeness. The core's pkg/reportdesign judges every state through
// CheckReportDesign: the contrast guard refuses an accent the core would refuse
// and offers the nearest passing shade, and Save waits until the design passes.
// Save stores it whole; Reset clears it, and runs from this app then use Plain,
// as they do signed out. There is no form element, so Enter in a field presses
// nothing (the site's thumbnails-as-submit trap cannot happen).
//
// The logo has a part of its own: Choose opens the system's file picker in Go
// and uploads what is picked, Remove takes it off and keeps the design. Save
// never sends it, so the core keeps it. The preview and the part draw it only
// as the PNG data: URL Go's view built from the core's answer.
export function ReportDesignSettings({ registry, signedIn, onOpenRegistry }: {
  registry: string;
  signedIn: boolean;
  onOpenRegistry: () => void;
}) {
  const [opts, setOpts] = useState<app.ReportDesignOptions | null>(null);
  const [view, setView] = useState<app.ReportDesignView | null>(null);
  const [d, setD] = useState<ReportDesign | null>(null);
  const [check, setCheck] = useState<{ key: string; c: app.ReportDesignCheck } | null>(null);
  const [busy, setBusy] = useState<"" | "save" | "reset" | "logo" | "unlogo">("");
  const [err, setErr] = useState<string | null>(null);
  const [said, setSaid] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    SettingsAPI.ReportDesignOptions().then((o) => { if (live) setOpts(o); }).catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, []);

  useEffect(() => {
    if (!signedIn) return;
    let live = true;
    setView(null);
    setD(null);
    SettingsAPI.ReportDesign(registry)
      .then((v) => { if (live) setView(v); })
      .catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, [registry, signedIn]);

  // The editor opens on the saved design, or on the default theme.
  const plain = opts?.themes.find((t) => t.id === opts.defaultThemeId)?.design;
  useEffect(() => {
    if (d || !view || !plain) return;
    setD(startFrom(view.design ?? plain));
  }, [view, plain, d]);

  // The core's verdict on each state; only the latest counts.
  const seq = useRef(0);
  useEffect(() => {
    if (!d) return;
    const n = ++seq.current;
    const key = canonical(d);
    SettingsAPI.CheckReportDesign(reportdesign.Design.createFrom(toDocument(d)))
      .then((c) => { if (n === seq.current) setCheck({ key, c }); })
      .catch((e) => setErr(String(e)));
  }, [d]);

  if (!signedIn) {
    return (
      <Card>
        <p>Your report design is kept on your account. Runs from this app use Plain while you are not signed in.</p>
        <Button onClick={onOpenRegistry}>Sign in</Button>
      </Card>
    );
  }
  if (view && !view.available) {
    return <Notice tone="info">This registry does not keep a report design. Runs from this app use Plain.</Notice>;
  }
  if (!opts || !view || !d) {
    return (
      <>
        <ErrorBox error={err} />
        {!err && <p className="muted small">Reading your report design…</p>}
      </>
    );
  }

  const c = check?.key === canonical(d) ? check.c : null;
  const set = (field: string, v: string | boolean) => { setSaid(null); setD((x) => (x ? edit(x, field, v) : x)); };
  const themeName = (id: string) => opts.themes.find((t) => t.id === id)?.design.name ?? id;
  const unchangedSinceSave = !!view.design && sameDesign(d, view.design);
  const problem = c?.problem && c.problem.field !== "accent" ? c.problem : null;

  const save = async () => {
    setBusy("save");
    setErr(null);
    setSaid(null);
    try {
      const v = await SettingsAPI.SaveReportDesign(registry, reportdesign.Design.createFrom(toDocument(d)));
      setView(v);
      if (v.design) setD(startFrom(v.design));
      setSaid(savedSentence(v.label ?? null));
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  };
  const logoAction = async (kind: "logo" | "unlogo") => {
    setBusy(kind);
    setErr(null);
    setSaid(null);
    try {
      const v = kind === "logo" ? await SettingsAPI.ChooseReportLogo(registry) : await SettingsAPI.RemoveReportLogo(registry);
      if (!v) return; // the picker was cancelled
      setView(v);
      setSaid(kind === "logo"
        ? "Your logo is set. Your runs' PDFs draw it at the right of the title on their first page."
        : "Your logo is removed. Your report design is as it was.");
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  };
  const reset = async () => {
    setBusy("reset");
    setErr(null);
    setSaid(null);
    try {
      const v = await SettingsAPI.ResetReportDesign(registry);
      setView(v);
      if (plain) setD(startFrom(plain));
      setSaid(savedSentence(null));
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="report-design">
      {view.unreadable && (
        <Notice tone="warn">
          Your account holds a report design this app cannot read ({view.unreadable}). Runs from this app use Plain until
          the app is updated. Saving a design here replaces it.
        </Notice>
      )}

      <section id="settings-part-themes" className="settings-part">
        <h2>Themes</h2>
        <Card>
          <p className="muted small">
            Five starting points, each drawn by the runner's own template from the same sample report. Choose one to load
            it into the editor; your changes start from it. The QuickAgent never picks the look of a PDF, and asking it for a
            formal tone changes its words, not this.
          </p>
          <ul className="report-design-themes">
            {opts.themes.map((t) => {
              const current = d.basedOn === t.id;
              return (
                <li key={t.id}>
                  <button type="button" className={`report-design-theme${current ? " current" : ""}`} data-theme={t.id}
                    aria-pressed={current} disabled={!!busy} onClick={() => { setSaid(null); setD(startFrom(t.design)); }}>
                    {t.thumbnail && <img src={t.thumbnail} alt={`The ${t.design.name} theme's first page`} />}
                    <strong>{t.design.name}</strong>
                    <span className="muted small">{THEME_LOOKS[t.id] ?? ""}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </Card>
      </section>

      <div className="report-design-grid">
        <section id="settings-part-design-editor" className="settings-part">
          <h2>Editor</h2>
          <Card>
            <p className="muted small" data-starts-from>
              Starts from {themeName(d.basedOn)}.{" "}
              {c ? (c.unedited ? "Unchanged." : "Changed: Save keeps your changes, and the theme stays as it is.") : ""}
            </p>

            <label className="field">
              <span className="field-label">Name</span>
              <input type="text" value={d.name} maxLength={opts.maxName} onChange={(e) => set("name", e.target.value)} />
              <span className="muted small">A run's receipt names the design it used by this.</span>
            </label>

            <Radios legend="Paper" field="paper" value={d.paper} values={opts.papers} labels={PAPER_LABELS} onChange={set} inline />

            <fieldset className="report-design-set">
              <legend className="field-label">Fonts</legend>
              {(["headings", "body", "code"] as FontRole[]).map((role) => (
                <label key={role} className="row report-design-font">
                  <span>{role === "headings" ? "Headings" : role === "body" ? "Body" : "Code"}</span>
                  <Select value={d.fonts[role]} onChange={(e) => set(`fonts.${role}`, e.target.value)} aria-label={`${role} font`}>
                    {fontsFor(role, opts).map((f) => (
                      <option key={f} value={f}>
                        {f}{role === "headings" && opts.families.find((x) => x.name === f)?.headingsOnly ? " (headings only)" : ""}
                      </option>
                    ))}
                  </Select>
                </label>
              ))}
              <span className="muted small">A script a face lacks, Chinese or Arabic say, is set in Noto in the PDF.</span>
            </fieldset>

            <fieldset className="report-design-set">
              <legend className="field-label">Accent</legend>
              <div className="row">
                <input type="color" aria-label="Accent colour picker" value={cssColour(d.accent)} onChange={(e) => set("accent", e.target.value)} />
                <input type="text" className="mono report-design-hex" aria-label="Accent colour, #rrggbb" value={d.accent} maxLength={7}
                  spellCheck={false} onChange={(e) => set("accent", e.target.value)} />
              </div>
              {c && <AccentLine accent={d.accent} check={c} onUse={(shade) => set("accent", shade)} />}
              <span className="muted small">
                Body text stays near-black whatever the accent. The tint behind table headers and callouts is the accent at 12%
                on white.
              </span>
            </fieldset>

            <Radios legend="Title block" field="titleBlock" value={d.titleBlock} values={opts.titleBlocks} labels={TITLE_BLOCK_LABELS} onChange={set} />
            <Radios legend="Headings" field="headings" value={d.headings} values={opts.headingStyles} labels={HEADING_LABELS} onChange={set} />

            <fieldset className="report-design-set">
              <legend className="field-label">Tables</legend>
              <label className="row">
                <input type="checkbox" checked={d.tables.tinted} onChange={(e) => set("tables.tinted", e.target.checked)} />
                Tint the header row
              </label>
              <label className="row">
                <input type="checkbox" checked={d.tables.banded} onChange={(e) => set("tables.banded", e.target.checked)} />
                Tint every other row
              </label>
            </fieldset>

            <Radios legend="Quotes" field="quotes" value={d.quotes} values={opts.quoteStyles} labels={QUOTE_LABELS} onChange={set} />

            <fieldset className="report-design-set">
              <legend className="field-label">Footer</legend>
              <Radios legend="Page numbers" field="footer.pageNumbers" value={d.footer.pageNumbers} values={opts.pageNumbers}
                labels={PAGE_NUMBER_LABELS} onChange={set} inline sub />
              <label className="field">
                <span>Footer line</span>
                <input type="text" value={d.footer.text} maxLength={opts.maxFooterText} placeholder="None"
                  onChange={(e) => set("footer.text", e.target.value)} />
                <span className="muted small">One line of up to {opts.maxFooterText} characters, printed as you write it on every page.</span>
              </label>
            </fieldset>

            {problem && (
              <p role="alert" className="report-design-problem">
                This design cannot be saved yet: {fieldLabel(problem.field)} {problem.message}.
              </p>
            )}
            <ErrorBox error={err} />
            <div className="row">
              <Button variant="primary" busy={busy === "save"} busyLabel="Saving…"
                disabled={!c || !!c.problem || !!busy || unchangedSinceSave} onClick={() => void save()}>
                Save report design
              </Button>
              {view.design && (
                <Button variant="danger" busy={busy === "reset"} busyLabel="Resetting…" disabled={!!busy} onClick={() => void reset()}>
                  Reset
                </Button>
              )}
            </div>
            {said && <Notice tone="ok">{said}</Notice>}
            <p className="muted small" data-standing>{standingSentence(view.design ? view.label ?? null : null)}</p>
          </Card>
        </section>

        <section id="settings-part-design-preview" className="settings-part report-design-preview">
          <h2>Preview</h2>
          <Card>
            <p className="muted small">A likeness drawn from the controls; the thumbnails above are the real render.</p>
            <div className="report-design-paper">
              <ReportDesignPreview design={d} tint={c?.tint ?? check?.c.tint ?? ""} families={opts.families} logo={view.logo ?? ""} />
            </div>
          </Card>
        </section>
      </div>

      <section id="settings-part-design-logo" className="settings-part">
        <h2>Logo</h2>
        <Card>
          <p className="muted small">
            Your logo, drawn at the right of the title on the first page of every PDF your runs export, and nowhere else. On a
            band title block it sits on a small white ground. The QuickAgent never chooses or changes it.
          </p>
          {!view.design ? (
            <p data-logo-needs-design>Save a report design first: a logo is part of one.</p>
          ) : (
            <>
              {isLogoSrc(view.logo) ? (
                <img className="report-design-logo" src={view.logo} alt="Your logo" />
              ) : (
                <p className="muted small">No logo yet.</p>
              )}
              <div className="row">
                <Button variant="primary" busy={busy === "logo"} busyLabel="Uploading…" disabled={!!busy} onClick={() => void logoAction("logo")}>
                  {isLogoSrc(view.logo) ? "Replace the logo…" : "Choose a logo…"}
                </Button>
                {isLogoSrc(view.logo) && (
                  <Button variant="danger" busy={busy === "unlogo"} busyLabel="Removing…" disabled={!!busy} onClick={() => void logoAction("unlogo")}>
                    Remove the logo
                  </Button>
                )}
              </div>
              <p className="muted small">
                A PNG or a JPEG of at most 512 KB. The registry decodes it, scales it to fit 1024 pixels each way and encodes it
                again from the pixels, so nothing of the file but its picture is kept. SVG is not accepted. Saving the design
                keeps the logo.
              </p>
            </>
          )}
        </Card>
      </section>
    </div>
  );
}

// AccentLine is the contrast guard: the accent's ratio on white and
// what it needs, or the core's refusal with the nearest shade that passes.
function AccentLine({ accent, check, onUse }: { accent: string; check: app.ReportDesignCheck; onUse: (shade: string) => void }) {
  const a = check.accent;
  const use = a.onText ? "colours text" : "colours only graphics";
  if (!a.valid) {
    return <p className="small report-design-problem" role="alert">Write the accent as #rrggbb, for example #1f6feb.</p>;
  }
  if (a.ok) {
    return <p className="muted small" id="accent-contrast">{ratioText(a.ratio)} on white. This design's accent {use}, which needs {a.need.toFixed(1)}:1.</p>;
  }
  return (
    <p className="small report-design-problem" id="accent-contrast" role="alert">
      {accent} reaches {ratioText(a.ratio)} on white, and an accent that {use} needs {a.need.toFixed(1)}:1, so it cannot be
      saved. The nearest shade that does is {a.nearest}.{" "}
      <Button variant="link" onClick={() => onUse(a.nearest)}>Use {a.nearest}</Button>
    </p>
  );
}

function Radios({ legend, field, value, values, labels, onChange, inline = false, sub = false }: {
  legend: string;
  field: string;
  value: string;
  values: string[];
  labels: Record<string, string>;
  onChange: (field: string, v: string) => void;
  inline?: boolean;
  sub?: boolean;
}) {
  return (
    <fieldset className={`report-design-set${inline ? " inline" : ""}`}>
      <legend className={sub ? "small" : "field-label"}>{legend}</legend>
      {values.map((v) => (
        <label key={v} className="row">
          <input type="radio" name={field} value={v} checked={value === v} onChange={() => onChange(field, v)} />
          {labelOf(labels, v)}
        </label>
      ))}
    </fieldset>
  );
}

