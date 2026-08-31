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

## D-008 — 2026-08-31: run-surface helpers replicated from cmd/agentry, not promoted

**What:** The run surface (`internal/run`, RUNNER-PLAN-2 §P6) replicates
four pieces of `agentry-cli/cmd/agentry` instead of importing them:
`runnerHome`, the `installMeta` struct + `writeInstall`/`loadInstallMeta`
cache procedure, `capabilityLines` (§5.4 rendering), and `copyTree`.
**Why:** `cmd/agentry` is package main and cannot be imported; RUNNER-PLAN-2
scoped the shared code to `pkg/runner`/`pkg/runner/setup`, which the app
does drive for everything with product semantics (verify gate text,
compile, engine, profiles, guided setup). The replicas are layout/rendering
plumbing whose shape is pinned by unit tests here (install.json round-trip,
digest suffix, capability lines).
**Impact:** The install cache under `~/.agentry/runner/agents/…` is
byte-compatible both ways (same install.json fields, same reuse-iff-digest
rule, same plan-reuse conditions). If the CLI's copies grow semantics,
promote them to a pkg and delete these.

## D-009 — 2026-08-31: runner image tag derives from the DESKTOP version

**What:** The app checks/builds the local runner image as
`policy.RunnerImageTag(desktop version)` — e.g. `agentry-runner:0.0.0-dev`
— while the CLI derives the tag from ITS version. On a machine using both
clients each may build its own runner image, and a plan compiled by one
client references the tag it derived (plans embed `proxy.image`), so the
other client can hit "image not found" on a plan it would otherwise reuse
(it recompiles only when digest/policy/adapter change, not on image tag).
**Why:** The tag derivation input is the client version by upstream design
(`§R7`: a new CLI version triggers rebuilds); the CLI's version constant is
`internal/` and unimportable, and inventing a cross-client constant here
would silently decouple the tag from the code that built the image.
**Impact:** Wasted disk (two runner images) and a possible one-time exit-13
when mixing clients on one install. Operator fix (documented in the policy
file itself): pin `images.runner` in policy.yaml — both clients honor the
override, restoring a single shared image AND identical plans. Revisit if
pkg/runner grows a shared runner-image version.

## D-010 — 2026-08-31: run-capability consent is the visible contract + Run

**What:** The CLI's run prompts for consent on declared egress/exec (and on
a widened contract vs the last accepted run). The desktop run panel instead
always renders the full declared contract (capability lines + the MCP
servers & tools section) and treats the Run click as the acceptance; the
acceptance is recorded to the same `installed.yaml` key the CLI uses
(registry "runner"/target "runner"), unconditionally per run.
**Why:** A modal-on-top-of-a-modal re-asking about text already on screen
is a worse consent surface than showing the contract at the moment of the
click; recording to the shared key keeps the CLI from re-asking about a
contract the operator already accepted here (and vice versa).
**Impact:** Consent is per-run and always-current (a widened v2 contract is
simply the contract shown next time), so nothing is accepted sight-unseen;
the record the two clients share stays coherent. No enforcement semantics
change — the compiler/engine gates (wildcard refusal, exit 11 on missing
secrets) are untouched.

## D-011 — 2026-08-31: unsigned-run consent lives in the panel, not RunAPI

**What:** `agentry run` refuses an unsigned release without
`--allow-unsigned`. The desktop's `RunAPI.RunAgent` proceeds on unsigned
(marking `attestation: unsigned` in the envelope and receipts, and warning
in the run log); the explicit consent — a checkbox mirroring the install
wizard's unsigned gate, wired to the page's provenance verification — is
enforced by the run panel, which keeps Run disabled until it is ticked.
Digest mismatch and unverifiable/mismatched attestations still refuse in
Go with no override, exactly like install and the CLI.
**Why:** The §P6 API shape has no allow-unsigned parameter (the desktop
collects consent as UI state, the same split Prepare/Confirm uses for
installs), and the page verifies provenance before the panel ever opens.
**Impact:** A caller invoking the bound method directly (not through the
panel) could run an unsigned release without the checkbox; the run is
still loudly marked unsigned in envelope, receipts, and log. If a second
consumer of RunAPI ever appears, add an explicit acceptUnsigned parameter
and move the gate into Go.

## D-012 — 2026-08-31: no empty MCP profile is created for no-secret runs

**What:** `agentry run --keep-profile=true` on a release declaring no
secrets creates an empty profile to record the keep choice. The desktop
run panel always sends its keep toggle, but `RunAgent` never creates a
profile — profiles are created only by the guided form (SaveProfile), and
the keep choice governs an existing profile's completion behavior.
**Why:** The panel sends keep on every run (a toggle has no "unset"), so
mirroring the CLI would grow an empty profile per no-secret BriefAgent —
`agentry profiles list` noise recording nothing.
**Impact:** The keep choice for a no-secret release is not persisted for
future schema growth (P2 §P3's edge case). If the profile schema ever
stores more than secrets+keep, add an explicit "create profile" affordance.

