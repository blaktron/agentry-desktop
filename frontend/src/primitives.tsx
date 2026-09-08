// The shared primitive layer (agentry-desktop#36). One set of building
// blocks in the website's design language, so every page reads as the same
// product and later milestones stop sprinkling inline styles. Presentational
// only; the trust/mode/capability badges live in ui.tsx, which re-exports
// these so a page imports everything from "../ui".
import React from "react";

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

export function ErrorBox({ error }: { error: string | null }) {
  if (!error) return null;
  return <div className="error-box selectable">{error}</div>;
}

/** A one-line status message. `tone` picks the accent; default is neutral. */
export function Notice({ tone = "info", children }: { tone?: "info" | "ok" | "warn"; children: React.ReactNode }) {
  return <div className={`notice notice-${tone} selectable`} role="status">{children}</div>;
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
