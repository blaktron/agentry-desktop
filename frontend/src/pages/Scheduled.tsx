// The Scheduled page (plan quickagent-scheduling.md §7, D11, D12, D15;
// D-073; agentry-desktop#743): the schedules on this computer. A month
// calendar, hand-built, with each day's occurrences as dots (the past ones
// in the colour of what happened), the chosen day's list, what is
// Upcoming, the History, and each schedule with Edit, Pause or Resume,
// Delete and Run now. Schedule a new run picks an installed QuickAgent and
// opens its run setup with When on Repeat.
//
// The CLI keeps the schedules: Go reads its store for this page and runs
// the installed agentry command for every change (ScheduleAPI). A past
// occurrence says what happened, and never reads as done when nothing ran
// (plan invariant 8).
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as ScheduleAPI from "../../wailsjs/go/main/ScheduleAPI";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import * as HomeAPI from "../../wailsjs/go/main/HomeAPI";
import { install, main, schedules } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, EmptyState, ErrorBox, Modal, Notice, OptionsMenu, PageHeader } from "../ui";
import { scheduledOnHome } from "../homeCards";
import { CalendarKey, MonthGrid } from "./MonthGrid";
import {
  byDay, clockOf, gridRange, monthGrid, monthTitle, outcomeLabel, outcomeTone, scheduleState, whenForm, whenOf,
  sameWhen, whenProblem, whenSpec, ymd, type FireRow, type WhenForm,
} from "../scheduled";
import { RepeatFields, SignedOutBox, type Availability } from "./WhenStep";

// How many rows Upcoming and History show.
const UPCOMING = 8;
const HISTORY = 20;

/** Listens to a Go event while mounted, where the runtime is there (the
 *  harness has none). */
function useGoEvent(name: string, f: (...data: unknown[]) => void) {
  useEffect(() => {
    try {
      return EventsOn(name, f);
    } catch {
      return undefined;
    }
  }, [name, f]);
}

export default function Scheduled({ onOpenRun, onScheduleAgent, focus, now: nowProp }: {
  /** A fire's run page. */
  onOpenRun: (runId: string) => void;
  /** Schedule a new run: the QuickAgent's page with its setup open on
   *  Repeat. */
  onScheduleAgent: (registry: string, ns: string, name: string) => void;
  /** A schedule just made, to select its first day. */
  focus?: string;
  /** Harness-only: a fixed now. */
  now?: Date;
}) {
  const now = useMemo(() => nowProp ?? new Date(), [nowProp]);
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [list, setList] = useState<main.SchedulesView | null>(null);
  const [listErr, setListErr] = useState<string | null>(null);
  const [month, setMonth] = useState({ year: now.getFullYear(), month: now.getMonth() });
  const [cal, setCal] = useState<FireRow[]>([]);
  const [history, setHistory] = useState<FireRow[]>([]);
  const [day, setDay] = useState(ymd(now));
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [editing, setEditing] = useState<schedules.Item | null>(null);
  const [picking, setPicking] = useState(false);
  const [reseal, setReseal] = useState<string | null>(null);
  // Home's layout, for the options menu's Pin to Home or Unpin (D13;
  // agentry-desktop#744): null until read, or when it cannot be, and then
  // the menu offers nothing. pinSaid is what the last Pin or Unpin did.
  const [home, setHome] = useState<main.HomeLayoutView | null>(null);
  const [pinSaid, setPinSaid] = useState<{ text: string; error?: boolean } | null>(null);

  const weeks = useMemo(() => monthGrid(month.year, month.month, now), [month, now]);
  const range = useMemo(() => gridRange(weeks), [weeks]);

  // The newest read wins: paging months quickly must not let an older
  // month's answer land last.
  const reads = useRef(0);
  const read = useCallback(() => {
    const n = ++reads.current;
    ScheduleAPI.List()
      .then((l) => { setList(l); setListErr(null); setReseal(l.resealProblem || null); })
      .catch((e) => setListErr(String(e)));
    ScheduleAPI.Calendar(range.from, range.to)
      .then((c) => { if (n === reads.current) setCal(c as FireRow[]); })
      .catch(() => { if (n === reads.current) setCal([]); });
    ScheduleAPI.History(HISTORY).then((h) => setHistory(h as FireRow[])).catch(() => setHistory([]));
  }, [range]);
  useEffect(read, [read]);
  useEffect(() => {
    ScheduleAPI.Availability().then(setAvailability).catch((e) =>
      setAvailability({ available: false, reason: String(e), zone: "", signedOut: { available: false, reason: String(e) }, lingerOn: false }));
  }, []);
  useEffect(() => {
    let live = true;
    HomeAPI.Layout().then((v) => { if (live) setHome(v); }).catch(() => {});
    return () => { live = false; };
  }, []);
  useGoEvent("schedules:changed", read);
  useGoEvent("schedules:reseal", useCallback((msg: unknown) => setReseal(String(msg)), []));

  // A schedule just made: its first day is the one shown, once; the month
  // is the person's to page after that.
  const focused = useRef("");
  useEffect(() => {
    const made = list?.schedules.find((s) => s.id === focus);
    if (made?.next && focused.current !== made.id) {
      focused.current = made.id;
      const t = new Date(made.next);
      setMonth({ year: t.getFullYear(), month: t.getMonth() });
      setDay(ymd(t));
    }
  }, [focus, list]);

  const days = useMemo(() => byDay(cal), [cal]);
  const upcoming = useMemo(() => cal.filter((f) => f.outcome === "due" && new Date(f.at) >= now).slice(0, UPCOMING), [cal, now]);
  const running = new Set(list?.running ?? []);

  async function act(id: string, what: string, f: () => Promise<unknown>) {
    setBusy(id + what);
    setErr(null);
    try {
      await f();
      read();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(null);
    }
  }

  const shift = (n: number) => setMonth((m) => {
    const d = new Date(m.year, m.month + n, 1);
    return { year: d.getFullYear(), month: d.getMonth() };
  });

  const unavailable = availability && !availability.available ? availability.reason : null;
  // A layout this app keeps rather than uses takes no change (D-067).
  const homeUsable = !!home && !home.newer && !home.unreadable;
  const onHome = homeUsable && scheduledOnHome(home!.cards);
  async function pin() {
    setPinSaid(null);
    try {
      setHome(await (onHome ? HomeAPI.UnpinScheduled() : HomeAPI.PinScheduled()));
      setPinSaid({ text: onHome ? "Unpinned from Home." : "Pinned to Home: this month's calendar and the next three runs." });
    } catch (e) {
      setPinSaid({ text: String(e), error: true });
    }
  }
  const items = list?.schedules ?? [];
  const dayRows = days.get(day) ?? [];

  return (
    <div className="page scheduled-page" data-scheduled-page>
      <PageHeader
        title="Scheduled"
        sub="Runs on this computer for later, or on repeat like a calendar entry."
        actions={<>
          {homeUsable && (
            <OptionsMenu items={[{ key: onHome ? "unpin-home" : "pin-home", label: onHome ? "Unpin from Home" : "Pin to Home", onSelect: () => void pin() }]} />
          )}
          <Button variant="primary" onClick={() => setPicking(true)} disabled={!!unavailable}>Schedule a new run</Button>
        </>}
      />
      {pinSaid && (pinSaid.error ? <ErrorBox error={pinSaid.text} /> : <p className="small home-said" role="status" data-pin-said>{pinSaid.text}</p>)}
      {unavailable && <Notice tone="warn"><span data-scheduled-unavailable>{unavailable}</span></Notice>}
      {reseal && <Notice tone="warn"><span className="selectable" data-reseal-problem>{reseal}</span></Notice>}
      <ErrorBox error={err} />
      <ErrorBox error={listErr} />

      <div className="scheduled-grid">
        <section className="card scheduled-calendar" aria-label="Calendar">
          <div className="spread">
            <Button onClick={() => shift(-1)} aria-label="Previous month">←</Button>
            <h3 style={{ margin: 0 }}>{monthTitle(month.year, month.month)}</h3>
            <Button onClick={() => shift(1)} aria-label="Next month">→</Button>
          </div>
          <MonthGrid weeks={weeks} days={days} chosen={day} onChoose={setDay} />
          <CalendarKey />
        </section>

        <section className="card scheduled-day" aria-label="The chosen day">
          <h3>{new Date(day + "T00:00:00").toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long" })}</h3>
          {dayRows.length === 0 ? <p className="muted small">Nothing scheduled on this day.</p> : (
            <ul className="home-card-list" data-day-list>
              {dayRows.map((r, k) => <FireLine key={k} f={r} onOpenRun={onOpenRun} />)}
            </ul>
          )}
          <h3>Upcoming</h3>
          {upcoming.length === 0 ? <p className="muted small">Nothing due in this month&apos;s view.</p> : (
            <ul className="home-card-list" data-upcoming>
              {upcoming.map((r, k) => <li key={k}><span>{whenOf(r.at)}</span> · <span className="mono small">{r.name}</span></li>)}
            </ul>
          )}
        </section>
      </div>

      <section className="card" aria-label="Schedules">
        <h3>Schedules</h3>
        {list && items.length === 0 && (
          <EmptyState title="No schedules yet">
            Schedule a new run, or choose Later or Repeat under When on a run&apos;s Review and start.
          </EmptyState>
        )}
        {items.length > 0 && (
          <ul className="schedule-list" data-schedules>
            {items.map((s) => (
              <li key={s.id} className="schedule-row" data-schedule={s.id}>
                <div className="grow">
                  <strong>{s.name}</strong> <span className="mono small muted">{s.ref}@{s.version}</span>
                  <div className="small">{s.describe}{s.next ? ` · next ${whenOf(s.next)}` : ""}</div>
                  <div className="muted small">
                    {scheduleState(s)}
                    {s.last ? ` · last: ${outcomeLabel(s.last)}` : ""}
                    {running.has(s.id) ? " · running now" : ""}
                  </div>
                </div>
                <div className="row" style={{ gap: 6 }}>
                  <Button onClick={() => setEditing(s)} disabled={!!unavailable || s.local}>Edit</Button>
                  {s.paused
                    ? <Button onClick={() => act(s.id, "resume", () => ScheduleAPI.Resume(s.id))} disabled={!!busy || !!unavailable}>Resume</Button>
                    : <Button onClick={() => act(s.id, "pause", () => ScheduleAPI.Pause(s.id))} disabled={!!busy || !!unavailable || s.ended}>Pause</Button>}
                  <Button onClick={() => act(s.id, "run", () => ScheduleAPI.RunNow(s.id))} disabled={!!busy || !!unavailable || running.has(s.id)}>
                    {running.has(s.id) ? "Running…" : "Run now"}
                  </Button>
                  <Button variant="danger" onClick={() => act(s.id, "remove", () => ScheduleAPI.Remove(s.id, s.name))} disabled={!!busy || !!unavailable}>Delete</Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="card" aria-label="History">
        <h3>History</h3>
        {history.length === 0 ? <p className="muted small">No scheduled run has rung yet.</p> : (
          <ul className="home-card-list" data-history>
            {history.map((r, k) => <FireLine key={k} f={r} onOpenRun={onOpenRun} withDate />)}
          </ul>
        )}
      </section>

      {editing && (
        <EditSchedule item={editing} availability={availability} now={now}
          onClose={() => setEditing(null)} onSaved={() => { setEditing(null); read(); }} />
      )}
      {picking && (
        <PickAgent onClose={() => setPicking(false)} onPick={(r) => { setPicking(false); onScheduleAgent(r.origin || r.registry, r.namespace, r.name); }} />
      )}
    </div>
  );
}

/** One occurrence: when, what, what became of it, and its run's page. */
function FireLine({ f, onOpenRun, withDate }: { f: FireRow; onOpenRun: (id: string) => void; withDate?: boolean }) {
  return (
    <li data-fire-outcome={f.outcome}>
      <span className={`cal-dot tone-${outcomeTone(f.outcome)}`} aria-hidden />{" "}
      <span>{withDate ? whenOf(f.at) : clockOf(f.at)}</span> · <span className="mono small">{f.name}</span> ·{" "}
      <span className={`recent-run-end tone-${outcomeTone(f.outcome) === "due" ? "warn" : outcomeTone(f.outcome)}`}>{outcomeLabel(f)}</span>
      {f.runId && <> · <Button variant="link" onClick={() => onOpenRun(f.runId!)}>Open</Button></>}
      {f.outcome === "failed" && f.message && <div className="muted small selectable">{f.message}</div>}
    </li>
  );
}

/** Edit: the name, when it runs, and whether it also runs signed out. */
function EditSchedule({ item, availability, now, onClose, onSaved }: {
  item: schedules.Item;
  availability: Availability | null;
  now: Date;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(item.name);
  const [form, setForm] = useState<WhenForm>(() => whenForm(item.when, !!item.signedOut, now, availability?.zone || item.when.zone || ""));
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const problem = whenProblem(form);

  async function save() {
    setSaving(true);
    setErr(null);
    try {
      const spec = whenSpec(form);
      // The repeat is sent only when it changed: the command starts a
      // changed one afresh, its runs counted from naught again.
      const whenChanged = !sameWhen(spec, item.when);
      const keysGone = !!item.signedOut && !item.keysSealed && form.signedOut;
      await ScheduleAPI.Edit(item.id, schedules.Edit.createFrom({
        name: name.trim() !== item.name ? name.trim() : "",
        when: whenChanged ? spec : undefined,
        // Said only when it changes, or to seal keys a re-seal removed.
        signedOut: form.signedOut !== !!item.signedOut || keysGone ? form.signedOut : undefined,
      }));
      onSaved();
    } catch (e) {
      if (!/not confirmed/i.test(String(e))) setErr(String(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal title={`Edit ${item.name}`} onClose={onClose}>
      <label className="row" style={{ gap: 6 }}>
        <span className="muted small">Name</span>
        <input className="grow" value={name} onChange={(e) => setName(e.target.value)} aria-label="Name" />
      </label>
      <div className="row" role="radiogroup" aria-label="When" style={{ gap: 14, marginTop: 8 }}>
        {(["later", "repeat"] as const).map((c) => (
          <label key={c} className="check-row" style={{ margin: 0 }}>
            <input type="radio" name="edit-when" checked={form.choice === c} onChange={() => setForm({ ...form, choice: c })} />
            <span>{c === "later" ? "Once, later" : "Repeat"}</span>
          </label>
        ))}
      </div>
      <RepeatFields form={form} onChange={setForm} disabled={saving} />
      <SignedOutBox form={form} onChange={setForm} availability={availability} disabled={saving} />
      {item.signedOut && !item.keysSealed && (
        <p className="small">Its keys are not sealed, so it does not run while you are signed out: Save with the box ticked seals them again.</p>
      )}
      <p className="muted small">It keeps running the version it was made for, {item.ref}@{item.version}, with the same task and files.</p>
      {problem && <p className="run-step-problem">{problem}</p>}
      <ErrorBox error={err} />
      <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button variant="primary" onClick={save} disabled={saving || !!problem}>{saving ? "Saving…" : "Save"}</Button>
      </div>
    </Modal>
  );
}

/** Schedule a new run's first step: an installed QuickAgent from a
 *  registry (a folder install is scheduled with the agentry command). */
function PickAgent({ onClose, onPick }: { onClose: () => void; onPick: (r: install.Item) => void }) {
  const [items, setItems] = useState<install.Item[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    InstallAPI.ListInstalled()
      .then((l) => {
        const seen = new Set<string>();
        setItems(l.filter((r) => {
          const k = `${r.origin || r.registry} ${r.namespace}/${r.name}`;
          if (r.target !== "runner" || r.local || !r.present || seen.has(k)) return false;
          seen.add(k);
          return true;
        }));
      })
      .catch((e) => setErr(String(e)));
  }, []);
  return (
    <Modal title="Schedule a new run" onClose={onClose}>
      <p className="muted small">Choose a QuickAgent installed on this computer. Its run setup opens with When on Repeat: every day, at the next whole hour.</p>
      <ErrorBox error={err} />
      {items && items.length === 0 && <p className="small">No QuickAgent is installed on this computer yet: install one from Browse first.</p>}
      {items && items.length > 0 && (
        <ul className="schedule-pick" data-schedule-pick>
          {items.map((r) => (
            <li key={`${r.origin || r.registry}-${r.namespace}-${r.name}`}>
              <Button variant="link" onClick={() => onPick(r)}>
                <span className="mono">{r.namespace}/{r.name}</span>
              </Button>{" "}
              <span className="muted small">{r.version}</span>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
