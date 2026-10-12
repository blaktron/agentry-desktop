import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main, dockerimages } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice } from "../ui";

// Run defaults → Docker images (agentry-desktop#424): the images Agentry
// put on this computer, chosen by name, and a button that removes them
// after a native confirmation. The Windows uninstaller removes them unless
// "Keep my Docker images" is ticked; the Linux packages and the macOS app
// have no uninstaller that can ask, so this is where they are removed.
export function DockerImagesCard() {
  const [view, setView] = useState<main.ImagesView | null>(null);
  const [report, setReport] = useState<dockerimages.Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const read = () => RunAPI.AgentryImages().then(setView).catch((e) => setError(String(e)));
  useEffect(() => { void read(); }, []);

  const remove = async () => {
    setBusy(true);
    setError(null);
    setReport(null);
    try {
      setReport(await RunAPI.RemoveAgentryImages());
      await read();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  if (!view) return <Card>{error ? <ErrorBox error={error} /> : <p className="muted small">Reading Docker's images…</p>}</Card>;
  const n = view.images.length;
  return (
    <Card>
      <ErrorBox error={error} />
      {view.unavailable ? (
        <p className="muted small">Docker's images could not be listed: {view.unavailable}</p>
      ) : n === 0 ? (
        <p className="small">No Docker images of Agentry's are on this computer.</p>
      ) : (
        <>
          <p className="small">
            {n} Docker image{n === 1 ? "" : "s"} Agentry put on this computer. The agentry CLI uses the same images, and the
            next run downloads or builds what it needs again.
          </p>
          <ul className="small mono selectable">
            {view.images.map((i) => (
              <li key={i.ref}>{i.ref}{i.size ? ` (${i.size})` : ""}</li>
            ))}
          </ul>
          <Button disabled={busy} onClick={() => void remove()}>{busy ? "Removing…" : "Remove Agentry's Docker images"}</Button>
        </>
      )}
      {report && (
        <Notice tone={report.failed.length ? "warn" : "ok"}>
          Removed {report.removed.length} image{report.removed.length === 1 ? "" : "s"}.
          {report.failed.length > 0 && <> Not removed: {report.failed.join("; ")}.</>}
        </Notice>
      )}
      <p className="muted small">
        Listed are every image the runner built (labelled org.agentry.image, untagged leftovers included), every
        ghcr.io/blaktron/agentry-* image, the runner&apos;s earlier builds by name, and the exact images the runner policy
        pins. Each is removed by its image ID; one that also carries a name you gave it loses only Agentry&apos;s names
        and stays. An image you labelled org.agentry.image yourself is taken as Agentry&apos;s. No other image is
        selected.
      </p>
    </Card>
  );
}
