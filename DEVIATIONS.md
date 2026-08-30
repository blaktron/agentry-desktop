# Deviations

Deliberate departures from the plan (`desktop-app-plan.md`) and from CLI
behavior, dated, with what/why/impact. Silent deviations rot; these are loud.

## D-001 — 2026-08-30: macOS artifact is unsigned and un-notarized

**What:** `make package-darwin` produces an ad-hoc-signed `.app` (Wails
self-signs); no Developer ID signature, no notarization.
**Why:** Signing requires the operator's Apple ID and a Developer ID
certificate, which do not exist in this environment.
**Impact:** Gatekeeper shows the usual unidentified-developer prompt;
users right-click → Open. Revisit before any public distribution.

## D-002 — 2026-08-30: mcp / agents-md installs take a project directory

**What:** The CLI's `mcp` and `agents-md` targets default to paths relative
to the process working directory (`./agents/<name>`, `./.mcp.json`,
`./AGENTS.md`). The desktop app instead REQUIRES a picked project directory
and lays out `<project>/agents/<name>` + `<project>/.mcp.json` /
`<project>/AGENTS.md`.
**Why:** A GUI has no meaningful cwd; silently writing relative to wherever
the .app was launched from would scatter files.
**Impact:** Same on-disk layout as the CLI run at a project root; the
`installed.yaml` key uses the absolute destination, so CLI and desktop
installs into the same project diff against each other correctly.

## D-003 — 2026-08-30: the drop-zone scans folders without agent.yaml

**What:** `agentry validate` errors when `agent.yaml` is missing; the
desktop Validate surface reports the missing manifest, skips the V-rules,
and still runs S1 + S3/S4 over every regular file. The run can never PASS
without a manifest.
**Why:** A drop-zone that refuses to look at a folder teaches authors to
stop dropping folders in; the scanners need no manifest to be useful.
**Impact:** Superset of CLI behavior; parity preserved where the CLI
defines behavior (manifest present).

## D-004 — 2026-08-30: uninstall exists (the CLI has none)

**What:** The Installed surface can uninstall: a record whose directory the
client chose (managed) has the directory removed and its `.mcp.json` entry /
`AGENTS.md` section de-merged; a user-picked directory is left in place and
only the record is dropped.
**Why:** A GUI that lists installs and cannot remove them is a to-do list.
The managed/user split copies the CLI's reinstall rule (never `RemoveAll` a
user-supplied dir); the `Managed` flag was added to the shared
`pkg/installed.Record` (agentry-cli a19e465) so both clients record it.
**Impact:** Records written by CLI versions before a19e465 load as
unmanaged, so uninstalling them never deletes files — the safe reading.

## D-005 — 2026-08-30: desktop settings live in desktop.yaml, not config.yaml

**What:** Registries + default registry are stored in
`~/.config/agentry/desktop.yaml`, not the CLI's `config.yaml`. PATs use the
same OS-keychain convention as the CLI (service `agentry`, account =
registry host), with a 0600 file fallback in `desktop.yaml`. The
`AGENTRY_TOKEN` env override is not honored (GUI).
**Why:** Two programs upserting one YAML file with different schemas is how
a config file gets corrupted; the keychain is the credential interop point,
and install records + key pins (the files the plan requires to be shared)
ARE shared via `pkg/installed` / `pkg/trust`.
**Impact:** Signing in via either client signs in both (keychain hosts
match). A file-fallback token stored by the CLI in config.yaml is not read
by the desktop app; sign in once in the app in that (headless-ish) case.

## D-006 — 2026-08-30: copyTree/treeFiles replicated, not promoted

**What:** The CLI's `copyTree` and `treeFiles` helpers (~40 lines of
filesystem plumbing in `cmd/agentry`) are replicated in
`internal/install` rather than promoted to a `pkg/`.
**Why:** Plan §5 scoped the promotion to the exporters, install state, and
trust; these helpers carry no product semantics.
**Impact:** None expected; if their behavior ever diverges from the CLI's,
promote them.

## Build-time checks the plan required (not deviations)

- **Wails v3 stability (plan §4):** checked 2026-08-30 — newest v3 is
  v3.0.0-beta.16 (beta, not production-ready), newest v2 is v2.15.0.
  Built on v2 as planned.
- **Windows cross-compile (plan §8):** `wails build -platform windows/amd64`
  from macOS — see README for the result and any recorded gap.

## D-007 — 2026-08-30: `make check` builds the frontend first; CI added

**What:** The `check` target ran `lint test frontend`, but `go vet ./...`,
`go test ./...` and `go build ./...` all fail on a fresh tree because
`main.go` embeds `frontend/dist` (no matching files found). It now runs
`frontend lint test`, and the `frontend` target additionally emits the
bundle (`npx vite build`) after the type-check. Alongside this, the repo
gains `.github/workflows/ci.yaml` (check + govulncheck).
**Why:** A check that only passes when a previous build left `dist/` behind
is a trap for fresh clones and CI runners; the previous README note
("there is no Actions pipeline in this repo") aged into the same class of
overclaim the main repo's audit catalogued.
**Impact:** `make check` passes from a clean checkout (verified by deleting
`frontend/dist` and re-running). `wails build` packaging is untouched — it
still builds its own bundle.
