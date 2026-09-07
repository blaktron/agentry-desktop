# Agentry Desktop

A cross-platform desktop client (macOS, Windows, Linux) for
[Agentry](https://secureagentry.net) registries — browse scanned and
signed agents, install them with **digest and DSSE-signature verification on
this machine**, track what this machine has accepted, and validate local
agent folders with the same scanners the registry runs.

The product never claims an agent is "safe". The language is always:
*scanned, N findings, signed.*

## Surfaces

- **Browse** — search the connected registry (kind / capability / license /
  sort facets); agent pages show the summary, capability badges, the public
  security report card (findings by severity, ruleset version, scan date),
  version history, and provenance (archive digest, signing key id, DSSE
  verification state). READMEs are rendered from the API's server-sanitized
  HTML — the client never parses registry markdown itself.
- **Install** — pick a target (`claude-skill` / `mcp` / `agents-md`) and
  directory, then: resolve → download → **sha256 digest check → DSSE
  signature verification against TOFU-pinned keys** → review the declared
  capability contract (diffed against what this machine previously accepted,
  with explicit confirmation on any widening) → export → record. An unsigned
  release requires an explicit checkbox, mirroring the CLI's
  `--allow-unsigned`. A signature that fails to verify aborts — there is no
  override for that.
- **Run** — run a BriefAgent from the app (RUNNER-PLAN-2 §P6) with the same
  lifecycle `agentry run` executes: verify (digest + DSSE, the install
  gate) → materialize into the shared `~/.agentry/runner` cache → compile
  the capability contract into a run plan → execute in an isolated
  container topology where tools come only from a per-run Docker MCP
  Gateway. Agent pages gain an **MCP servers & tools** section (the tools
  the contract grants, the servers involved, declared key NAMES, and the
  embedded-install equivalent); a guided form collects a BriefAgent's keys
  once per release into its MCP profile (values 0600 on disk, never shown,
  never logged); Settings → **Agent runs** writes the LLM provider into the
  same operator policy the CLI reads and can prepare the gateway images
  up front. Runs stream their log live and finish with the §R8 result
  envelope — status, usage, receipts, and the output folder.
- **Installed** — the shared install state
  (`~/.config/agentry/installed.yaml`, the same file the `agentry` CLI
  writes), per-item capabilities, update checks, uninstall (client-managed
  directories only; a directory you picked is never deleted).
- **Validate** — an author drop-zone: point at a local agent folder and run
  manifest rules V1–V8, the S1 inventory (content-sniffed binaries, nested
  archives, encoding), and the S3/S4 hidden-content + injection-signature
  scanners — the same Go packages the publication pipeline runs (D-12
  local/remote parity by construction), with the rule bundle embedded in
  the binary.

## Architecture

Wails v2 (Go + native webview) with a React + TypeScript + Vite frontend
sharing the web app's design tokens.

Go is the whole point: verification and scanning run **in-process** through
the same libraries the server and CLI use —

| concern | package |
|---|---|
| REST client | `github.com/blaktron/agentry/pkg/apiclient` |
| manifest rules, refs | `github.com/blaktron/agentry/pkg/manifest` |
| S3/S4 scanners + embedded bundle | `…/pkg/rules`, `…/pkg/ruleset` |
| hardened extract, content sniffers | `…/pkg/archive` |
| DSSE / in-toto verification | `…/pkg/attest` |
| install-target exporters | `github.com/blaktron/agentry-cli/pkg/export` |
| shared install state + capability diff | `…/pkg/installed` |
| TOFU key pins + attestation flow | `…/pkg/trust` |
| run lifecycle (policy, compile, engine, gateway, profiles, setup) | `…/pkg/runner/*` |

Bound services (`services.go`, `services_run.go`) expose flat view models
to the frontend; engines live in
`internal/{registry,install,validate,settings,run}`.

Credentials are stored in the OS keychain under the same service/account
convention as the CLI, so signing in from either client signs in both.
Install records and pinned signing keys are shared files under
`~/.config/agentry/` (relocatable with `AGENTRY_CONFIG_DIR`).

## Building

Prereqs: Go ≥ 1.27, Node ≥ 20, the Wails v2 CLI
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`); on Linux the
GTK 3 and WebKit2GTK 4.1 development headers.

**Until the repositories are public** (the open core is private until the
operator flips it; see `PUBLIC-FLIP.md` in agentry-notes), the core and CLI
modules cannot be fetched anonymously. Either build from a workspace whose
`go.work` resolves `github.com/blaktron/agentry` and
`github.com/blaktron/agentry-cli` against sibling checkouts, or:

```bash
export GOPRIVATE='github.com/blaktron/*' GONOSUMDB='github.com/blaktron/*'
# plus a read token for github.com/blaktron via a credential helper or insteadOf
```

CI does the same with the `AGENTRY_BUILD_TOKEN` secret. All of this goes
away at the flip.

| task | command |
|---|---|
| dev shell (live reload) | `make dev` |
| host-platform build | `make build` → `build/bin/` |
| lint + tests + tsc | `make check` |
| regenerate TS bindings | `make bindings` (after changing bound Go APIs) |
| macOS universal .app (zipped) | `make package-darwin` |
| Windows amd64 (cross-compiled) | `make package-windows` |

**Linux** builds on a Linux host (webkit2gtk links natively):

```bash
# Debian/Ubuntu build deps
apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev npm golang
wails build -platform linux/amd64
```

(A host without those headers can build inside an Ubuntu container — see
`scripts/build-linux.sh`.)

CI (`ci.yaml`: `check` and `govulncheck`) runs on the project's own
runners. Artifacts are not committed; they attach to GitHub releases, still
a manual step (the hosted service builds and serves its own binaries from
the site repository).

## The registry it talks to

The app works against any Agentry registry — the hosted service or one you
run from the open core ([self-hosting guide](https://github.com/blaktron/agentry/blob/main/docs/self-hosting.md)).
Settings → Add a registry → its URL (`http://localhost:8080` for a local
stack) → Use; the first one added becomes the default (`defaultRegistry`
in the app's config file), and a hosted build may compile one in. Anonymous
browsing works; **Sign in** opens the registry's own `/ui/authorize` page
in your browser and brings a scoped token back to the app, which is when
the registry's signing keys are pinned — a personal access token pasted from
the registry (its `/ui/account/tokens` page, the site's Settings → Tokens,
or `agentry login`) does the same.

## Conventions

Conventional commits; `make check` green before every push; every
deliberate departure gets a dated entry in [the deviations register](https://github.com/blaktron/agentry-notes/blob/main/agentry-desktop/DEVIATIONS.md).
