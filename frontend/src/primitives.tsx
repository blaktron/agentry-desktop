// The shared primitive layer. One set of building
// blocks in the website's design language, so every page reads as the same
// product and pages do not need inline styles. Presentational
// only; the trust/mode/capability badges live in ui.tsx, which re-exports
// these so a page imports everything from "../ui".
import React from "react";
import { causeOf } from "./errorText";

type ButtonVariant = "default" | "primary" | "danger" | "ghost" | "link";

export function Button({
  variant = "default", busy, busyLabel, children, className = "", ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant; busy?: boolean; busyLabel?: string;
}) {
  const cls = [variant === "default" ? "" : variant, className].filter(Boolean).join(" ");
  return (
    <button className={cls} disabled={busy || rest.disabled} {...rest}>
      {busy ? busyLabel ?? "Working…" : children}
    </button>
  );
}

/** A bordered surface panel. `pad={false}` for a flush container (e.g. a
 *  list of rows that draw their own padding). */
export function Card({ children, className = "", pad = true }: { children: React.ReactNode; className?: string; pad?: boolean }) {
  return <div className={`card${pad ? "" : " card-flush"} ${className}`.trim()}>{children}</div>;
}

/** The page title block: an h1, an optional one-line sub, and right-aligned
 *  actions. The website's page header, one register for "you are here". */
export function PageHeader({ title, sub, actions }: { title: React.ReactNode; sub?: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <div className="page-header">
      <div className="grow">
        <h1>{title}</h1>
        {sub && <p className="page-sub">{sub}</p>}
      </div>
      {actions && <div className="row nowrap page-header-actions">{actions}</div>}
    </div>
  );
}

/** A labelled form control. `hint` is muted help under the control. */
export function Field({ label, hint, htmlFor, children, className = "" }: {
  label: React.ReactNode; hint?: React.ReactNode; htmlFor?: string; children: React.ReactNode; className?: string;
}) {
  return (
    <div className={`field ${className}`.trim()}>
      <label className="field-label" htmlFor={htmlFor}>{label}</label>
      {children}
      {hint && <div className="field-hint muted small">{hint}</div>}
    </div>
  );
}

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className = "", ...rest }, ref) {
    return <input ref={ref} className={className} {...rest} />;
  },
);

export function Select({ className = "", children, ...rest }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={className} {...rest}>{children}</select>;
}

export function Textarea({ className = "", ...rest }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={`task-input ${className}`.trim()} {...rest} />;
}

/** A horizontal control strip (search bars, filters). */
export function Toolbar({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <div className={`toolbar ${className}`.trim()}>{children}</div>;
}

/** The centered "nothing here yet" state, optionally with an action. */
export function EmptyState({ title, children, action }: { title: React.ReactNode; children?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="empty-state">
      <div className="empty-state-title">{title}</div>
      {children && <div className="muted small">{children}</div>}
      {action && <div style={{ marginTop: 12 }}>{action}</div>}
    </div>
  );
}

export function Spinner({ label }: { label?: string }) {
  return <div className="spinner">{label ?? "Loading…"}</div>;
}

/** What went wrong, as a sentence (causeOf), announced as an alert. With
 *  onRetry, the box offers the action again: a read that can be made again,
 *  a start that can be tried again. retrying draws that button busy. */
export function ErrorBox({ error, onRetry, retryLabel = "Try again", retrying }: {
  error: string | null | undefined;
  onRetry?: () => void;
  retryLabel?: string;
  retrying?: boolean;
}) {
  const cause = causeOf(error);
  if (!cause) return null;
  return (
    <div className="error-box selectable" role="alert">
      <span className="error-box-cause">{cause}</span>
      {onRetry && (
        <Button className="error-box-retry" onClick={onRetry} busy={retrying} busyLabel="Trying again…">{retryLabel}</Button>
      )}
    </div>
  );
}

/** A one-line status message. `tone` picks the accent; default is neutral. */
export function Notice({ tone = "info", children }: { tone?: "info" | "ok" | "warn"; children: React.ReactNode }) {
  return <div className={`notice notice-${tone} selectable`} role="status">{children}</div>;
}

/** A clickable result/list card: a mono title, a right-aligned badges slot,
 *  a summary line, and a muted meta line — the shape the website's search
 *  results and the installed list share. */
export function AgentRow({ title, badges, summary, meta, actions, onClick }: {
  title: React.ReactNode; badges?: React.ReactNode; summary?: React.ReactNode;
  meta?: React.ReactNode; actions?: React.ReactNode; onClick?: () => void;
}) {
  return (
    <div className={`agent-row${onClick ? " clickable" : ""}`} onClick={onClick}>
      <div className="agent-row-main">
        <div className="agent-row-head">
          <span className="agent-row-title mono">{title}</span>
          {badges && <span className="row agent-row-badges">{badges}</span>}
        </div>
        {summary && <div className="agent-row-summary muted">{summary}</div>}
        {meta && <div className="agent-row-meta muted small">{meta}</div>}
      </div>
      {actions && <div className="row nowrap agent-row-actions" onClick={(e) => e.stopPropagation()}>{actions}</div>}
    </div>
  );
}

/** A modal dialog over a scrim. `wide` widens it for diffs and forms. */
export function Modal({ title, onClose, wide, children }: {
  title?: React.ReactNode; onClose: () => void; wide?: boolean; children: React.ReactNode;
}) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className={`modal${wide ? " modal-wide" : ""}`} onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
        {title && <h2 style={{ marginTop: 0 }}>{title}</h2>}
        {children}
      </div>
    </div>
  );
}

// The step chrome: the first-run wizard's dots and the run setup's "Step n of
// m" heading, foot and recap, extracted so a stepped flow is one set of pieces.
// The classes share the run setup's rules in style.css.

/** The steps as dots with their labels; the current one is marked. */
export function StepList({ labels, at }: { labels: string[]; at: number }) {
  return (
    <ol className="step-list" aria-label="Steps">
      {labels.map((label, i) => (
        <li key={label} aria-current={i === at ? "step" : undefined} className={i === at ? "active" : i < at ? "done" : ""}>
          <span className={`wizard-dot${i === at ? " active" : ""}${i < at ? " done" : ""}`} aria-hidden />
          <span>{label}</span>
        </li>
      ))}
    </ol>
  );
}

/** "Step n of m: Label", focusable so focus can follow the step. */
export const StepHeading = React.forwardRef<HTMLHeadingElement, { at: number; total: number; label: string; id?: string }>(
  function StepHeading({ at, total, label, id }, ref) {
    return (
      <h3 id={id} ref={ref} tabIndex={-1} className="step-heading">
        Step {at + 1} of {total}: {label}
      </h3>
    );
  },
);

/** The step's foot: the sentence that stopped Next, then Back and the
 *  step's own actions on the right. */
export function StepFoot({ problem, onBack, children }: { problem?: string | null; onBack?: () => void; children: React.ReactNode }) {
  return (
    <>
      {problem && <p className="step-problem" role="alert">{problem}</p>}
      <div className="row step-foot">
        {onBack && <button onClick={onBack}>Back</button>}
        <span className="grow" />
        {children}
      </div>
    </>
  );
}

/** A recap of answers, each row with an Edit that goes back to its step. */
export function Recap({ rows }: { rows: { label: string; value: React.ReactNode; onEdit?: () => void }[] }) {
  return (
    <dl className="step-recap">
      {rows.map((r) => (
        <React.Fragment key={r.label}>
          <dt>{r.label}</dt>
          <dd>
            <div className="step-recap-value">{r.value}</div>
            {r.onEdit && <button className="ghost small" onClick={r.onEdit} aria-label={`Edit ${r.label}`}>Edit</button>}
          </dd>
        </React.Fragment>
      ))}
    </dl>
  );
}

/** A page's options menu (the Scheduled page's): one
 *  button that opens a short list of actions under it. It closes on a
 *  choice, on Escape and on a press outside it, and the first item takes
 *  the focus when it opens. */
export function OptionsMenu({ label = "Options", ariaLabel, items }: {
  label?: string;
  // The button's name for a screen reader, where its label is a symbol.
  ariaLabel?: string;
  items: { label: string; onSelect: () => void; disabled?: boolean; key?: string }[];
}) {
  const [open, setOpen] = React.useState(false);
  const box = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    box.current?.querySelector<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')?.focus();
    return () => { document.removeEventListener("mousedown", away); document.removeEventListener("keydown", esc); };
  }, [open]);
  return (
    <div className="options-menu" ref={box}>
      <button type="button" aria-haspopup="menu" aria-expanded={open} aria-label={ariaLabel} data-options-menu onClick={() => setOpen((o) => !o)}>
        {label} <span aria-hidden>▾</span>
      </button>
      {open && (
        <div className="options-menu-list" role="menu">
          {items.map((it) => (
            <button key={it.key ?? it.label} type="button" role="menuitem" className="ghost" disabled={it.disabled} data-menu-item={it.key}
              onClick={() => { setOpen(false); it.onSelect(); }}>
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
