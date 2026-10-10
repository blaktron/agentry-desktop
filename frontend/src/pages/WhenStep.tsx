// The run setup's When (plan quickagent-scheduling.md §7, D12, D19; D-073;
// agentry-desktop#743, #745): Now, the default, or Later or Repeat, which make
// the run a schedule: on this computer, or In Agentry when the run goes
// there. With Later or Repeat, Review and start's button reads Schedule. On
// this computer it stays off until the agentry command's check says a
// scheduled run would have everything it needs (D8), and each gap it names
// is shown here; In Agentry the registry's create is that check, and the
// setup asks for what it names. The same fields edit a schedule's repeat on
// the Scheduled page.
import { DAYS, signedOutUnavailable, zoneChoices, type WhenForm } from "../scheduled";
import { Button } from "../ui";

export type Availability = {
  available: boolean;
  reason?: string;
  zone: string;
  signedOut: { available: boolean; reason?: string };
  lingerOn: boolean;
};

export type CheckState =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "done"; ready: boolean; gaps: string[]; installed: boolean; describe?: string; next?: string; needsConsent?: boolean }
  | { state: "error"; error: string };

/** The zones the runtime knows, where it can list them. */
function supportedZones(): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  try {
    return intl.supportedValuesOf ? intl.supportedValuesOf("timeZone") : [];
  } catch {
    return [];
  }
}

const LINGER = "Agentry's schedules will start when this computer starts, even if you are not signed in.";

/** The repeat's fields (Later's day and time, or Repeat's shape, time, end
 *  and zone), shared by the When step and the Scheduled page's Edit. */
export function RepeatFields({ form, onChange, disabled }: { form: WhenForm; onChange: (w: WhenForm) => void; disabled?: boolean }) {
  const set = (patch: Partial<WhenForm>) => onChange({ ...form, ...patch });
  const zones = zoneChoices(form.zone, supportedZones());
  return (
    <div className="when-fields" data-when-fields>
      {form.choice === "later" && (
        <div className="row" style={{ gap: 10 }}>
          <label className="row" style={{ gap: 6 }}>
            <span className="muted small">Day</span>
            <input type="date" aria-label="Day" value={form.date} onChange={(e) => set({ date: e.target.value })} disabled={disabled} />
          </label>
          <label className="row" style={{ gap: 6 }}>
            <span className="muted small">Time</span>
            <input type="time" aria-label="Time" value={form.time} onChange={(e) => set({ time: e.target.value })} disabled={disabled} />
          </label>
        </div>
      )}
      {form.choice === "repeat" && (
        <>
          <div className="row" style={{ gap: 10, flexWrap: "wrap" }}>
            <label className="row" style={{ gap: 6 }}>
              <span className="muted small">Repeat</span>
              <select aria-label="Repeat" value={form.repeat} onChange={(e) => set({ repeat: e.target.value as WhenForm["repeat"] })} disabled={disabled}>
                <option value="daily">Every day</option>
                <option value="weekdays">Weekdays (Monday to Friday)</option>
                <option value="weekly">Every week on…</option>
                <option value="monthly">Every month on day…</option>
              </select>
            </label>
            {form.repeat === "monthly" && (
              <label className="row" style={{ gap: 6 }}>
                <span className="muted small">Day of the month</span>
                <input type="number" min={1} max={31} style={{ width: 70 }} aria-label="Day of the month" value={form.monthDay}
                  onChange={(e) => set({ monthDay: parseInt(e.target.value, 10) || 0 })} disabled={disabled} />
              </label>
            )}
            <label className="row" style={{ gap: 6 }}>
              <span className="muted small">at</span>
              <input type="time" aria-label="Time" value={form.time} onChange={(e) => set({ time: e.target.value })} disabled={disabled} />
            </label>
          </div>
          {form.repeat === "weekly" && (
            <div className="row when-days" role="group" aria-label="Days of the week" style={{ gap: 8 }}>
              {DAYS.map((d) => (
                <label key={d.id} className="check-row" style={{ margin: 0 }}>
                  <input type="checkbox" checked={form.days.includes(d.id)} disabled={disabled}
                    onChange={(e) => set({ days: e.target.checked ? [...form.days, d.id] : form.days.filter((x) => x !== d.id) })} />
                  <span>{d.label}</span>
                </label>
              ))}
            </div>
          )}
          {form.repeat === "monthly" && form.monthDay > 28 && (
            <p className="muted small" style={{ margin: "2px 0" }}>A month without day {form.monthDay} runs on its last day.</p>
          )}
          <div className="row" role="radiogroup" aria-label="Ends" style={{ gap: 12, flexWrap: "wrap" }}>
            <span className="muted small">Ends</span>
            <label className="check-row" style={{ margin: 0 }}>
              <input type="radio" name="when-end" checked={form.end === "never"} onChange={() => set({ end: "never" })} disabled={disabled} />
              <span>Never</span>
            </label>
            <label className="check-row" style={{ margin: 0 }}>
              <input type="radio" name="when-end" checked={form.end === "on"} onChange={() => set({ end: "on" })} disabled={disabled} />
              <span>On</span>
            </label>
            {form.end === "on" && (
              <input type="date" aria-label="Last day" value={form.until} onChange={(e) => set({ until: e.target.value })} disabled={disabled} />
            )}
            <label className="check-row" style={{ margin: 0 }}>
              <input type="radio" name="when-end" checked={form.end === "after"} onChange={() => set({ end: "after" })} disabled={disabled} />
              <span>After</span>
            </label>
            {form.end === "after" && (
              <label className="row" style={{ gap: 6 }}>
                <input type="number" min={1} style={{ width: 70 }} aria-label="Number of runs" value={form.times}
                  onChange={(e) => set({ times: parseInt(e.target.value, 10) || 0 })} disabled={disabled} />
                <span className="muted small">runs</span>
              </label>
            )}
          </div>
        </>
      )}
      <label className="row" style={{ gap: 6 }}>
        <span className="muted small">Time zone</span>
        <select aria-label="Time zone" value={form.zone} onChange={(e) => set({ zone: e.target.value })} disabled={disabled}>
          {!form.zone && <option value="">Choose a time zone</option>}
          {zones.map((z) => <option key={z} value={z}>{z}</option>)}
        </select>
      </label>
    </div>
  );
}

/** The signed-out box (D19), with its disabled reason, and the linger
 *  sentence the first time (D17). */
export function SignedOutBox({ form, onChange, availability, disabled }: {
  form: WhenForm;
  onChange: (w: WhenForm) => void;
  availability: Availability | null;
  disabled?: boolean;
}) {
  const why = signedOutUnavailable(availability);
  return (
    <div className="when-signed-out" data-signed-out>
      <label className="check-row" style={{ margin: 0 }}>
        <input type="checkbox" checked={form.signedOut && !why} disabled={disabled || !!why}
          onChange={(e) => onChange({ ...form, signedOut: e.target.checked })} />
        <span>Also run when I&apos;m signed out</span>
      </label>
      {why && <p className="muted small" style={{ margin: "2px 0 0 24px" }} data-signed-out-reason>{why}</p>}
      {!why && form.signedOut && !availability?.lingerOn && (
        <p className="muted small" style={{ margin: "2px 0 0 24px" }}>
          {LINGER} Schedule asks you first, and your distribution may ask for an administrator&apos;s password. Its keys are
          sealed beside it, encrypted to this computer and to you.
        </p>
      )}
    </div>
  );
}

/** What a schedule In Agentry holds for each run (agentry-desktop#745):
 *  the registry's numbers, as Review and start states a run's. */
export type HostedTerms = { holdCredits: number; balance: number; balanceKnown: boolean };

/** The When step on Review and start. */
export function WhenStep({ form, onChange, unavailable, availability, check, disabled, inAgentry, onUpgrade }: {
  form: WhenForm;
  onChange: (w: WhenForm) => void;
  /** Why only Now is offered for this run, or null. */
  unavailable: string | null;
  availability: Availability | null;
  check: CheckState;
  disabled?: boolean;
  /** The run goes In Agentry: a schedule there, which the registry keeps
   *  and checks when it is made (D8; agentry-desktop#745), with each run's
   *  hold and the balance. */
  inAgentry?: HostedTerms | null;
  /** The way up, where the plan is what holds When In Agentry (D7). */
  onUpgrade?: () => void;
}) {
  const choose = (choice: WhenForm["choice"]) => onChange({ ...form, choice });
  return (
    <div className="run-when" data-when-step>
      <h4>When</h4>
      <div className="row" role="radiogroup" aria-label="When" style={{ gap: 14 }}>
        {(["now", "later", "repeat"] as const).map((c) => (
          <label key={c} className="check-row" style={{ margin: 0 }}>
            <input type="radio" name="run-when" checked={form.choice === c} onChange={() => choose(c)}
              disabled={disabled || (c !== "now" && !!unavailable)} />
            <span>{c === "now" ? "Now" : c === "later" ? "Later" : "Repeat"}</span>
          </label>
        ))}
      </div>
      {unavailable && <p className="muted small" style={{ margin: "2px 0" }} data-when-unavailable>{unavailable}</p>}
      {unavailable && onUpgrade && <p className="small" style={{ margin: "2px 0" }}><Button onClick={onUpgrade}>Credits and upgrade</Button></p>}
      {form.choice !== "now" && !unavailable && inAgentry && (
        <>
          <RepeatFields form={form} onChange={onChange} disabled={disabled} />
          <p className="muted small" style={{ margin: "4px 0" }} data-when-hosted>
            A schedule In Agentry runs this exact version, autonomous, whether or not this computer is on. Each run is an
            ordinary run of yours there: it holds {inAgentry.holdCredits} credits while it runs
            {inAgentry.balanceKnown ? `, and your balance is ${inAgentry.balance}` : ""}, and a run that finds too few
            credits is skipped. Its result stays with the run In Agentry; open it from Scheduled.
          </p>
          <p className="muted small" style={{ margin: "4px 0" }}>
            Schedule has Agentry check what every run will need before it keeps the schedule: each secret kept in your
            account, each connection made for this version, and the files copied. Anything missing is asked for here.
          </p>
        </>
      )}
      {form.choice !== "now" && !unavailable && !inAgentry && (
        <>
          <RepeatFields form={form} onChange={onChange} disabled={disabled} />
          <SignedOutBox form={form} onChange={onChange} availability={availability} disabled={disabled} />
          <p className="muted small" style={{ margin: "4px 0" }}>
            A schedule runs this exact version, autonomous, on this computer
            {form.signedOut ? ", whether or not you are signed in" : ", while you are signed in"}; it never wakes the
            computer. One missed while the computer was off runs once when it is back, if the next one has not come
            first. Each run gets its own folder in the output folder below.
          </p>
          {check.state === "checking" && <p className="muted small" data-when-check="checking">Checking what a scheduled run needs…</p>}
          {check.state === "error" && <p className="small run-step-problem" data-when-check="error">The check could not be made: {check.error}</p>}
          {check.state === "done" && check.ready && (
            <p className="small" data-when-check="ready">
              Ready: {check.describe}
              {check.next ? `, first on ${new Date(check.next).toLocaleString()}` : ""}.
              {!check.installed && " Schedule installs this version first."}
              {check.needsConsent && " Schedule asks for your consent to its capabilities first."}
            </p>
          )}
        </>
      )}
    </div>
  );
}
