# Agentry Desktop

A cross-platform desktop client (macOS, Windows, Linux) for
[Agentry](https://secureagentry.net) registries. Browse scanned and signed
agents, install them with **digest and DSSE-signature verification on this
machine**, track what this machine has accepted, and validate local agent
folders with the same scanners the registry runs.

The product never claims an agent is "safe". The language is always:
*scanned, N findings, signed.*

## Surfaces

- **Browse** — search the connected registry, with filters for run mode and
  declared capability, and a sort order. An agent page shows the summary,
  capability badges, the public security report card (findings by severity,
  ruleset version, scan date), version history, and provenance (archive
  digest, signing key id, DSSE verification state). READMEs are rendered
  from the API's server-sanitized HTML — the client never parses registry
  markdown itself.
- **Install** — pick a target, `claude-skill` or `agents-md` (the `mcp`
  target left with the withdrawn v1 tool-server kind, DEV-088), and a
  directory, then: resolve → download → **sha256 digest check → DSSE
  signature verification against TOFU-pinned keys** → review the declared
  capability contract → export → record. The app diffs that contract
  against what this machine previously accepted, and any widening needs
  explicit confirmation; an unsigned release requires an explicit checkbox,
  mirroring the CLI's `--allow-unsigned`, and a signature that fails to
  verify aborts — there is no override for that. The wizard shows the
  registry's verdict on each declared MCP server, and a known high or
  critical advisory refuses runs by default. The run panel's "allow
  vulnerable MCP servers" override lifts that for one run and is recorded in
  the receipt; the operator policy can lift it outright, and the agent page
  shows the verdict per server and, for the publisher, the open dependency
  alerts.
- **Run** — run a BriefAgent from the app (RUNNER-PLAN-2 §P6) with the same
  lifecycle `agentry run` executes: verify (digest + DSSE, the install
  gate) → materialize into the shared `~/.agentry/runner` cache → compile
  the capability contract into a run plan → execute in an isolated
  container topology where tools come only from a per-run Docker MCP
  Gateway. Agent pages gain an **MCP servers & tools** section: the tools
  the contract grants, the servers involved, declared key NAMES, and the
  embedded-install equivalent. A guided form collects a BriefAgent's keys
  once per release into its MCP profile — values 0600 on disk, never shown,
  never logged. Settings → **Agent runs** writes the LLM provider into the
  same operator policy the CLI reads, and can prepare the gateway images up
  front. On Linux the docker card names the machine's state — `installable`
  (the app installs the engine through `pkexec`), `groupMissing` (the engine
  is in, your user is not in the `docker` group: one button adds you, through
  `pkexec`), `pendingRestart` (installed and in the group, but this session
  cannot use it until it restarts: **Restart now** asks the session manager
  to reboot, **Restart later** keeps the card until the engine answers),
  `ready`, or `manual` (a hand-off to the official installer, or an installed
  engine that refuses for its own reason). The app never runs as root. Runs
  stream their log live and finish with the §R8 result envelope — status,
  usage, receipts, and the output folder.
- **In Agentry** — where the registry mounts the runs extension and the
  release declares the autonomous mode, the run panel offers a third
  choice: the platform runs it, not this machine (plan
  `agentry-notes/plans/hosted-runs.md`). The same consent panel, intake
  form, task and input files; the app uploads the files, creates the run
  with its own token, shows the queue position, streams the log into the
  same view, and downloads the result, reports, receipts and outputs into
  the run folder you picked. The panel states what the run holds in credits
  and what the balance is, both as the account reports them, and the
  finished run shows the outcome, the runner's exit class and the receipt's
  `capabilities_enforced` listed verbatim. It needs no docker engine and no
  operator policy on this machine; a release that declares a key cannot run
  there yet and says so. Sign-in asks for the `run:read` and `run:write`
  scopes; a token minted before that keeps working everywhere else, with In
  Agentry unavailable and the reason shown.
- **Installed** — the shared install state
  (`~/.config/agentry/installed.yaml`, the same file the `agentry` CLI
  writes), with per-item capabilities and update checks. Uninstall deletes
  client-managed directories only: a directory you picked is never deleted.
- **Validate** — an author drop-zone. Point it at a local agent folder and
  it runs the manifest rules, the S1 inventory (content-sniffed binaries,
  nested archives, encoding), and the S3/S4 hidden-content and
  injection-signature scanners. Those are the same Go packages the
  publication pipeline runs (D-12 local/remote parity by construction), with
  the rule bundle embedded in the binary.
- **Build** — author a BriefAgent as a form (manifest schema v2: run-mode
  checkboxes, an intake editor, the interactive opening line), validate
  it, and publish over the same upload path the CLI uses.

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

Bound services (`services.go`, `services_auth.go`, `services_build.go`,
`services_prereqs.go`, `services_run.go`) expose flat view models to the
frontend; engines live in
`internal/{registry,install,validate,settings,run,build,mcpvuln,version}`.

The app puts credentials in the OS keychain, under the same service/account
convention as the CLI, so signing in from either client signs in both. With
no keychain available the token falls back into the app's own settings file
at 0600, the CLI's posture too. Install records and pinned signing keys are
shared files under `~/.config/agentry/` (relocatable with
`AGENTRY_CONFIG_DIR`).

## Building

Prereqs for a platform bundle: Go ≥ 1.27, Node ≥ 20, the Wails v2 CLI
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`); Docker for the
Linux package (the GTK 3 and WebKit2GTK 4.1 headers live in its container,
or on the host for a direct `wails build`); `jq` for `make version-check`.

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
| macOS universal .app, zipped and as a .dmg | `make package-darwin` |
| Windows amd64 exe, zip and per-user installer (cross-compiled, unsigned) | `make package-windows` |
| Linux amd64 binary, .deb and .rpm (in a container) | `make package-linux` |
| install both Linux packages in clean containers | `make smoke-linux-packages` |
| drag the dmg's bundle over an installed copy (macOS) | `make smoke-darwin-dmg` |
| what update-signing keys this build trusts | `go run ./internal/cmd/keyset list` |
| load a new update-signing key (operator) | `scripts/operator/update-signing-key.sh` |
| the version every target stamps | `make version` |
| does this tag name `wails.json`'s version? | `make version-check RELEASE_TAG=v0.1.0` |

**Linux** links WebKit2GTK natively, so it is built where the headers are.
`make package-linux` runs `scripts/build-linux.sh`: it vendors the modules
and builds the frontend bundle on the host, then builds inside a Debian
container that installs the GTK 3 and WebKit2GTK 4.1 headers, and launches
the result under a virtual display as a smoke. It then runs
`scripts/package-linux.sh`, which wraps that binary in a `.deb` and an
`.rpm` from the one description in `build/nfpm/nfpm.yaml` (nfpm through
`go run` at a pinned version): the binary at `/usr/bin/agentry-desktop`, a
desktop entry, the icon at the hicolor sizes, the runtime libraries the
binary links declared as dependencies, no maintainer scripts and nothing
under anyone's home, so the package manager's upgrade or removal cannot
reach `~/.config/agentry`, `~/.agentry/runner` or the keyring.
`make smoke-linux-packages` installs a 0.0.0 build of the same package in
a clean container of each family (Debian bookworm, Ubuntu 24.04, Fedora
44), upgrades to the real one, and proves the file half of that promise
plus a launch under Xvfb and, on Debian and Fedora, that the declared
dependencies are exactly the packages owning the linked libraries; a
container has no session keyring, so the keychain half rests on the
package having no scripts, which the smoke reads from the control archive.
The release workflow runs it before it stages the packages. A build past a
tag is spelt `0.1.0+9.g<sha>` in the packages so it sorts above the tag it
follows (`scripts/package-linux.sh --self-test` pins the spellings). The
bare binary stays as its own download. A host with the headers can also
build directly:

```bash
# Debian/Ubuntu build deps
apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev npm golang
wails build -platform linux/amd64 -tags webkit2_41
```

**Windows** is cross-compiled from any host with Go, Node and the Wails
CLI: Wails loads WebView2 through a pure-Go loader, so `windows/amd64`
builds with cgo off (DEV-097). `make package-windows` also builds the
per-user installer, `agentry-desktop-<version>-windows-amd64-setup.exe`,
from the NSIS script in `build/windows/installer/project.nsi` with
`makensis` (`apt-get install nsis`, `dnf install mingw32-nsis`) and reads it
back with `scripts/check-windows-installer.sh` (needs `7z`): it installs
under the person's own programs directory without elevation, writes a Start
Menu shortcut and an Apps & features entry, and carries the same exe the
zip does. An upgrade replaces only the installed files; the uninstaller
leaves `%USERPROFILE%\.config\agentry`, `%USERPROFILE%\.agentry\runner`
and the Credential Manager entries alone unless its one optional box is
ticked, which runs `agentry-desktop.exe --remove-my-settings`
(`internal/uninstall`) before the files go. Both outputs are unsigned
(agentry-desktop#54). The `windows` CI job builds all of this on every PR;
what Linux cannot check — install N over N-1 with settings in place, the
default uninstall keeping them, the box removing them — is
`scripts/acceptance/windows-install.ps1`, run by hand in a plain user
session on a Windows machine until #55 gives CI one.

**macOS** cannot be cross-built (the macOS front end is Objective-C behind
cgo), so the universal bundle is built on a Mac that carries the Command
Line Tools, Go, Node and the Wails CLI. `scripts/build-darwin.sh [user@host]`
runs from a Linux workstation: it vendors the modules where the credential
helper lives, ships the tree without `.git` to the Mac, runs
`make package-darwin WAILS_FLAGS=-m` there (the Makefile says why `-m`),
copies the zip and the dmg back and prints their digests. The target wraps
one bundle twice: the zip for people who want the bundle, and a disk image
holding the bundle beside an `Applications` shortcut for people who install
a macOS application the usual way. `hdiutil` builds the image
(`scripts/package-darwin-dmg.sh`) and `scripts/check-darwin-dmg.sh` mounts
it and reads it back — the right name for the download manifest, the bundle
and the shortcut and nothing else, universal, its signature verifying, and
byte-identical to the bundle the zip carries.
`scripts/acceptance/darwin-dmg-install.sh` (`make smoke-darwin-dmg`) then
drags that bundle onto a temporary Applications folder over a copy already
there and checks the settings, the installed agents, the pinned key, the
runner policy and a keychain item come through byte-identical. The keychain
item goes into a keychain the script creates, unlocks by name and deletes,
which needs no login session, so that half runs on the Mac runner too and
never touches a real sign-in. The
bundle is ad-hoc signed only and the image is not signed at all (D-001: no
Developer ID, no notarization), so both meet the same Gatekeeper prompt.
The Mac needs no GitHub credential. Set `AGENTRY_DEFAULT_REGISTRY` to bake
in the registry the build connects to by default. The launch smoke at the end
needs the build user logged in at the Mac's console; without one it is
reported as skipped, not failed. The estate's Mac and the plan behind it:
`agentry-notes/plans/macos-build-host.md`.

CI (`ci.yaml`: `check`, `govulncheck` and `windows`) runs on the project's
own runners. The first two need neither the Wails CLI nor the WebKit
headers: they compile the app library, not the platform bundle. `windows`
cross-builds the Windows exe and its installer on the Linux runner and
keeps them as a workflow artifact, so a broken Windows build fails the
gate. A fourth job, `macos`, builds the universal bundle on the Mac build
host, builds and checks the dmg, runs the tests and the Keychain round
trip there, and uploads the zip and the dmg as a workflow artifact; it does
not block the merge (`continue-on-error`). Its Keychain round trip has
failed since the Mac became a runner, for want of a console session
(#106).

## Releasing

A release is a tag `v<productVersion>` on `main`. The version lives in one
place, `wails.json`'s `productVersion`, which the Wails build carries into
the macOS bundle's `CFBundleShortVersionString` and the Windows version-info
block; the Makefile stamps the same string into the binary (`make version`
prints it: `git describe` without the tag's leading `v`, so a tagged commit
is `0.1.0` and a commit past it is `0.1.0-3-g<sha>`). To cut `0.2.0`: bump
`productVersion` in a commit on `main`, then `git tag v0.2.0 && git push
origin v0.2.0`. The `release` workflow refuses a tag that does not name the
version in `wails.json`, and a tag whose pins of the core or the CLI are
behind their default branches (the desktop reads files the CLI writes;
D-020). It then builds every platform artifact — Windows and Linux on the
Linux runner, the universal macOS bundle and its dmg on the Mac — writes
`SHA256SUMS` over them, signs those sums with the estate's own update-signing
key (below), and uploads the set as one workflow artifact named
`agentry-desktop-<version>`. `BUILD-INFO`, covered by the sums, names the
commit, the event (a tag push, or a `workflow_dispatch` dry run the site's
importer refuses), `signed=false`, and the registry the build connects to
by default (`build/release.env`, committed: dev until the launch flips it;
the jobs check the URL is really in the binaries). Between tags the bundle
metadata keeps the last `productVersion` while the binary reports the
describe string; only a tagged build has one version everywhere, which is
what a release is.

### The release is signed with our own key

`SHA256SUMS` in the bundle is signed, and `SHA256SUMS.minisig` travels beside
it. That signature is the trust root for "a newer Agentry Desktop": the app
checks it before it replaces itself, and the site's downloads importer checks
it before it publishes anything. It is **our** key, not the operating
system's — independent of Developer ID signing (#6) and Authenticode (#54),
which secure a different thing and are not needed for this to work.

- `internal/selfupdate/keys.json` is the set of keys a build trusts: each with
  a name, its public half, its key id and whether it is retired. A retired key
  still *verifies*, so an old release stays checkable, and callers refuse to
  *install* from one — that is what makes a rotation a release rather than a
  reinstall. A build with an empty set trusts nothing and refuses every update,
  which is the correct state for the open core.
- `internal/cmd/signrelease` signs in the release job and then verifies what it
  wrote against that committed key set, using `internal/selfupdate` — the same
  code the app uses. A release whose signature a shipped app would refuse
  therefore fails the release instead of shipping, which is the mistake a
  rotation invites: a new secret without the matching public half committed.
- The private half is a repository Actions secret, `AGENTRY_UPDATE_SIGNING_KEY`,
  read into a job-scoped file that is removed in an `always()` step. No signing
  key ever reaches the runner host or the Mac.
- **`scripts/operator/update-signing-key.sh`** is how a key is loaded, and the
  only supported way: it generates the pair on the machine running it, sets the
  secret, writes the public half into the key set with
  `internal/cmd/keyset`, and never prints the private half. Run it once for
  `dev` and again for `prod` on operator-held hardware at launch, retiring
  `dev` in the same run.

Until a key has been loaded, `internal/selfupdate/keys.json` is empty and the
release workflow **refuses every tag** — it will not publish a release no
shipped app could check. A `workflow_dispatch` dry run warns and skips signing
instead, so the rest of the release path stays exercisable.

No GitHub Release is created before 1.0 (operator, 2026-09-19): the site's
downloads builder imports the bundle from the run and the download page is
where a release lives. `workflow_dispatch` runs the same jobs on any ref as
a dry run, named by the describe string instead of a tag. Every artifact
is unsigned (D-001 for macOS, agentry-desktop#54 for Windows) and the
download page says so. The plan behind this and the installers that
follow: `agentry-notes/plans/desktop-installers.md`.

## The registry it talks to

The app works against any Agentry registry — the hosted service or one you
run from the open core ([self-hosting guide](https://github.com/blaktron/agentry/blob/main/docs/self-hosting.md)).
Add one under Settings → Add a registry: its URL
(`http://localhost:8080` for a local stack), then Use. The default is the
compiled-in registry (`https://secureagentry.net`, or whatever the build
set through `AGENTRY_DEFAULT_REGISTRY`), recorded as `defaultRegistry` in
the app's config file; Use switches it.

Anonymous browsing works. **Sign in via browser** opens the registry's own
`/ui/authorize` page in your browser and brings a scoped token back to the
app. Signing in is also when the app pins the registry's signing keys.
Pasting a personal access token does the same — take one from the registry's
`/ui/account/tokens` page, the site's Settings → Authentication, or
`agentry login`.

A signed-in registry's card shows the account behind the credential and,
where the registry has subscription tiers (its `/meta` lists the billing
extension), the tier, the balance out of the allowance, what a submission
costs and when the allowance resets, with **Upgrade** opening the site's
subscription page in your browser — the app takes no payment. A Build
submission the allowance cannot pay for comes back refused with the
registry's sentence and the same button; the registry stored nothing.

## Conventions

Conventional commits; `make check` green before every push; every
deliberate departure gets a dated entry in [the deviations register](https://github.com/blaktron/agentry-notes/blob/main/agentry-desktop/DEVIATIONS.md).
