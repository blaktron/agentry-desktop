// How far a download or an image pull has got, read from the lines the
// tools write: the docker command's pull output, one line per layer
// ("a1b2c3: Downloading [=====>   ]  12.3MB/45.6MB"), and the app's own
// download lines ("Downloading colima: 12 of 40 MB"). Pure, so the node
// tests can import it; the progress module draws a bar from what it reads.

/** How far something has got, in bytes; total is 0 when it is not known. */
export type BytesProgress = { done: number; total: number };

const UNITS: Record<string, number> = { B: 1, kB: 1e3, KB: 1e3, KiB: 1024, MB: 1e6, MiB: 1024 ** 2, GB: 1e9, GiB: 1024 ** 3 };

/** A size as docker writes it ("12.3MB", "45.6 kB") in bytes, or null. */
export function parseSize(s: string): number | null {
  const m = /^([\d.]+)\s*([kKMG]i?B|B)$/.exec(s.trim());
  if (!m) return null;
  const unit = UNITS[m[2]];
  const n = Number(m[1]);
  return unit && Number.isFinite(n) ? n * unit : null;
}

// One layer's line in a pull: its id, its state, and for a download or an
// extraction, how far.
const LAYER = /^([0-9a-f]{6,}):\s+(Downloading|Extracting|Verifying Checksum|Download complete|Pull complete|Already exists|Pulling fs layer|Waiting)\s*(?:\[[^\]]*\]\s*)?(?:([\d.]+\s*[kKMG]?i?B)\/([\d.]+\s*[kKMG]?i?B))?/;
// The app's own download line.
const OWN = /Downloading (.+?): (\d+) of (\d+) MB/;

/** The progress the newest lines describe: the sum over a pull's layers
 *  of what each has downloaded against its size (a layer that is complete
 *  counts whole), or the app's own newest "x of y MB" line; null when the
 *  lines say nothing of the kind. */
export function bytesProgress(lines: readonly string[]): BytesProgress | null {
  let own: BytesProgress | null = null;
  const layers = new Map<string, BytesProgress>();
  for (const line of lines) {
    const o = OWN.exec(line);
    if (o) {
      own = { done: Number(o[2]) * 1e6, total: Number(o[3]) * 1e6 };
      continue;
    }
    const l = LAYER.exec(line);
    if (!l) continue;
    const [, id, state, cur, size] = l;
    const was = layers.get(id);
    switch (state) {
      case "Downloading": {
        const done = parseSize(cur ?? "");
        const total = parseSize(size ?? "");
        if (done != null && total != null) layers.set(id, { done, total });
        break;
      }
      case "Verifying Checksum":
      case "Download complete":
      case "Pull complete":
      case "Already exists":
        if (was) layers.set(id, { done: was.total, total: was.total });
        break;
      // Extracting re-reports the layer's size as it unpacks; the download
      // is done by then, so the layer stays whole.
      case "Extracting":
        if (was) layers.set(id, { done: was.total, total: was.total });
        break;
    }
  }
  if (layers.size > 0) {
    let done = 0;
    let total = 0;
    for (const p of layers.values()) {
      done += Math.min(p.done, p.total);
      total += p.total;
    }
    if (total > 0) return { done, total };
  }
  return own;
}

/** How far an image build has got: its newest step against the steps it
 *  has. The docker command writes one line per step, "#7 [3/6] RUN …" with
 *  BuildKit or "Step 3/6 : RUN …" without; null when the lines hold none. */
export function stepProgress(lines: readonly string[]): { step: number; steps: number } | null {
  let out: { step: number; steps: number } | null = null;
  for (const line of lines) {
    const m = /^(?:#\d+ )?\[\s*(\d+)\/(\d+)\]|^Step (\d+)\/(\d+)\b/.exec(line);
    if (!m) continue;
    const step = Number(m[1] ?? m[3]);
    const steps = Number(m[2] ?? m[4]);
    if (steps > 0 && step > 0) out = { step: Math.min(step, steps), steps };
  }
  return out;
}

/** A progress's words: "12 of 40 MB", or "12 MB so far" with no total. */
export function bytesWords(p: BytesProgress): string {
  const mb = (n: number) => `${Math.round(n / 1e6)}`;
  return p.total > 0 ? `${mb(Math.min(p.done, p.total))} of ${mb(p.total)} MB` : `${mb(p.done)} MB so far`;
}

/** How much of the whole is done, 0 to 1; 0 with no total. */
export function bytesShare(p: BytesProgress): number {
  return p.total > 0 ? Math.min(1, p.done / p.total) : 0;
}

/** What an install of a newer Agentry Desktop is doing, by the stage Go
 *  reports; "Installing…" for one it does not name. */
export function updateStageWords(stage: string | undefined): string {
  switch (stage) {
    case "checking": return "Checking the release…";
    case "downloading": return "Downloading…";
  }
  return "Installing…";
}
