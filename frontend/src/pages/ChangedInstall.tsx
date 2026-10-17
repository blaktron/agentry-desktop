// The refusal a run of a changed install meets, shown before Run with every
// changed path and, for a registry release, Repair.
import { useState } from "react";
import * as InstallAPI from "../../wailsjs/go/app/InstallAPI";
import { Button, ErrorBox, Notice } from "../ui";
import { RECORD_PROVES, changedLines, repairLine, type ChangedInstall } from "../changedInstall";

export function ChangedInstallNotice({ registry, changed, onRepaired }: {
  registry: string;
  changed: ChangedInstall;
  onRepaired: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function repair() {
    setBusy(true);
    setErr(null);
    try {
      // The one-click install of this exact version: the download, the
      // digest, the attestation and the consent are checked as on any
      // install, and a native dialog asks before the changed copy is
      // replaced.
      await InstallAPI.InstallRunner(registry, changed.ref);
      onRepaired();
    } catch (e) {
      const msg = String(e);
      if (!/not confirmed/i.test(msg)) setErr(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Notice tone="warn">
      <strong>The installed copy of {changed.ref} changed after it was installed. A run is refused.</strong>
      <ul className="small" style={{ margin: "6px 0" }}>
        {changedLines(changed).map((l) => <li key={l}>{l}</li>)}
      </ul>
      <p className="small">{RECORD_PROVES}</p>
      <p className="small" style={{ marginBottom: changed.local ? 0 : undefined }}>{repairLine(changed)}</p>
      {!changed.local && (
        <p className="small" style={{ marginBottom: 0 }}>
          <Button onClick={repair} busy={busy} busyLabel="Repairing…">Repair</Button>
        </p>
      )}
      <ErrorBox error={err} />
    </Notice>
  );
}
