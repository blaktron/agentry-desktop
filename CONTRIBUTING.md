# Contributing to Agentry Desktop

This repository holds the desktop client for macOS, Windows and Linux: a
[Wails v2](https://wails.io) application with a Go backend and a React,
TypeScript and Vite frontend. Verification, scanning and running use the
same Go libraries as the server and the CLI, so most behaviour a QuickAgent
run depends on lives in those modules, not here.

## Before you start

- **Security issues do not go in pull requests or public issues.** See
  [SECURITY.md](SECURITY.md).
- Everyone who takes part follows the [Code of Conduct](CODE_OF_CONDUCT.md).
- For a large change, open an issue first. A change to how a run is
  compiled or enforced belongs in the CLI,
  [blaktron/agentry-cli](https://github.com/blaktron/agentry-cli). A change
  to the package format, the scanners or the registry API belongs in the
  core, [blaktron/agentry](https://github.com/blaktron/agentry).

## Contributing from outside

Fork, branch from `dev`, and open a pull request against `dev`. `dev` is
where changes land, and `main` takes only promotions from `dev` (the
`branch-model` check fails any other pull request into `main`). A
maintainer approves the CI run for a first-time contributor. Sign off every
commit (see "Certificate of origin" below), keep a change to one concern,
and say in the pull request what you verified and on which platform.

## Development setup

The prerequisites are in the [README](README.md#prerequisites): Go at the
version `go.mod` names, Node 22 and npm, the Wails CLI at the version
`go.mod` pins, and on Linux the GTK 3 and WebKit2GTK 4.1 headers.

| Command | What |
|---|---|
| `make check` | frontend type check and bundle, lint, Go tests, the copy check and `go build ./...`: run it before you push |
| `make dev` | the live-reload development shell (`wails dev`) |
| `make test` | `go test ./...` |
| `make lint` | gofmt, `go vet`, golangci-lint (when installed) and the scripts check |
| `make frontend` | the frontend type check, `vite build` and the frontend unit tests |
| `make copy` | the copy check over the text the app ships |
| `make bindings` | regenerate the TypeScript bindings after changing a bound Go method or type |
| `make workflows-check` | no workflow job that a fork's pull request can start runs on a self-hosted runner without the fork guard (needs python3 with PyYAML) |

`frontend/wailsjs/` is generated: never edit it by hand, run `make
bindings`. A pull request that changes a bound method carries the
regenerated bindings.

## The rules that matter

**The frontend does nothing the backend does not allow.** The bound services
in `internal/app/` are the only way the frontend reaches the machine, the
registry or a run. A new capability is a bound Go method with a test, never
a direct call from the webview.

**Installs verify before anything lands.** An install checks the archive
digest and the signature before a file is written, through the same code the
CLI uses. Keep its refusal tests passing, and give a new refusal its own test.

**Credentials stay in the OS keychain.** Keys and tokens go in the keychain
under the names the CLI uses. They never go in a settings file, a log or a
policy file. The policy carries a keyring source, never the value.

**The update path trusts only its signing keys.** `internal/selfupdate/`
verifies every update against `internal/selfupdate/keys.json` before
applying it. A change there needs a test that a wrongly signed or tampered
update is refused.

**Copy is checked.** Text the app shows passes `make copy`. Product copy
never calls a QuickAgent "safe": it says "scanned", "N findings" or
"signed".

## Code style

Match the file you are editing. Go: wrap errors with context, pass
`context.Context` first, and keep no package-level mutable state. Frontend:
TypeScript strict mode, components that take view models from the bound
services, and no new runtime dependency without a reason in the pull
request. Commits follow conventional commits (`feat:`, `fix:`, `docs:`,
`refactor:`, `test:`, `ci:`, `chore:`, `security:`, `deps:`). A release's
notes are generated from the merged pull requests.

## Tests

Go tests sit beside the code they cover. The live end-to-end tests
(`internal/app/live*_test.go`) run only when `AGENTRY_LIVE_REGISTRY` is set.
The frontend's tests run with node's own test runner (`npm test` in
`frontend/`). Fixtures never contain real endpoints or real secrets.

## Certificate of origin

This project uses the [Developer Certificate of Origin](https://developercertificate.org/)
instead of a CLA. Sign off your commits:

```bash
git commit -s -m "fix(install): refuse an archive whose digest does not match"
```

By signing off you state that you wrote the contribution or have the right
to submit it under the project's licence.

## Licensing

The project licence is MIT ([LICENSE](LICENSE)). Do not add dependencies,
Go or npm, under the AGPL or the GPL. A new dependency needs a line in the
pull request that explains why an existing one will not do.

## Releasing

Releases are cut by the maintainers. Each one is a `vX.Y.Z` tag that names
`wails.json`'s version, with a GitHub Release carrying the signed
installers and packages for each platform. How to install and check them:
<https://docs.secureagentry.net/agentry/downloads/>.
