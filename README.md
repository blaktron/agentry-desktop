# Agentry Desktop

The desktop client (macOS, Windows, Linux) for [Agentry](https://secureagentry.net)
registries. It browses scanned and signed QuickAgents, installs them with
digest and signature verification on the machine, builds and runs them, and
shares its install state, signing-key pins and credentials with the `agentry`
CLI.

It is a [Wails v2](https://wails.io) application: a Go backend and a React,
TypeScript and Vite frontend in a native webview. Verification, scanning and
running use the same Go libraries as the server and the CLI
(`github.com/blaktron/agentry` and `github.com/blaktron/agentry-cli`).

## Using it

- [Agentry Desktop](https://docs.secureagentry.net/agentry/desktop/): what
  the app does and how to use it
- [Downloads](https://docs.secureagentry.net/agentry/downloads/): installers
  and packages for each platform
- [Unsafe Mode](https://docs.secureagentry.net/agentry/unsafe-mode/)
- [The runner](https://docs.secureagentry.net/runner/)

## Prerequisites

- Go, at the version `go.mod` names (`go 1.27.2`)
- Node 22 (what CI uses) and npm
- The Wails v2 CLI, at the version `go.mod` pins:
  `go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0`
- Linux: the GTK 3 and WebKit2GTK 4.1 development headers and `pkg-config`
  (Debian and Ubuntu: `libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config`), and
  the `webkit2_41` build tag for a direct `wails build`. `make package-linux`
  needs only Docker, because it builds in a container that carries them.
- `jq` for `make version-check`; Docker for `make package-linux`; `makensis`
  and `7z` for `make package-windows`; ImageMagick for `make app-icons`

The Go modules `github.com/blaktron/agentry` and
`github.com/blaktron/agentry-cli` must be fetchable. If they are not
anonymously, export `GOPRIVATE='github.com/blaktron/*'` and give git a
credential for github.com/blaktron, or build inside a `go.work` workspace
that resolves them against sibling checkouts.

## Build, test, lint

| command | what it does |
|---|---|
| `make dev` | live-reload development shell (`wails dev`) |
| `make build` | production build for the host platform into `build/bin/` |
| `make check` | frontend type check and bundle, lint, Go tests, the copy check and `go build ./...` |
| `make test` | `go test ./...` |
| `make lint` | gofmt, `go vet`, golangci-lint (when installed) and a scripts check |
| `make copy` | the copy check over the text the app ships (`scripts/copy-check.sh`) |
| `make frontend` | frontend type check, `vite build` and the frontend unit tests |
| `make bindings` | regenerate the TypeScript bindings after changing a bound Go method or type |
| `make workflows-check` | no workflow job that a fork's pull request can start runs on a self-hosted runner without the fork guard (python3 with PyYAML) |
| `make package-darwin` | universal macOS `.app`, as a zip and a `.dmg` |
| `make package-windows` | Windows amd64 exe, zip and per-user installer (cross-compiled) |
| `make package-linux` | Linux amd64 binary, `.deb` and `.rpm` (built in a container) |
| `make smoke-linux-packages` | install both Linux packages in clean containers |
| `make smoke-darwin-dmg`, `make smoke-darwin-selfupdate` | macOS install and self-update checks |
| `make app-icons` | render the app icons from `build/appicon.svg` |
| `make version`, `make version-check RELEASE_TAG=vX.Y.Z` | the version the build stamps; whether a tag names `wails.json`'s version |

`make check` must pass before a change is pushed. The frontend's tests run
with node's own test runner (`npm test` in `frontend/`).

`build/README.md` describes the platform build files.

A build from source talks to `http://localhost:8080` until another registry
is set, has no update channel, so it never looks for a newer version, and
trusts no update-signing key (`internal/selfupdate/keys.json` is empty). It
carries no OAuth client metadata document, so a sign-in to an MCP server
registers a client with the vendor or asks for your own. `make
package-windows` builds an unsigned installer; `SIGN_CMD` names a command
that signs each Windows file in place, and the macOS bundle is ad-hoc
signed.

## Layout

| path | contents |
|---|---|
| `main.go` | the Wails entry point: builds the bound services, binds them and embeds the frontend bundle |
| `internal/app/` | the bound services that expose view models and events to the frontend (`app.go`, `services*.go`), with their tests; the live end-to-end tests (`live*_test.go`) run only with `AGENTRY_LIVE_REGISTRY` set |
| `internal/` | the engines behind the services: registry client, install, validate, settings, run, build, knowledge, schedules, self-update, version and others |
| `internal/selfupdate/keys.json` | the update-signing keys a build trusts |
| `frontend/` | the React, TypeScript and Vite frontend (`src/`, `tests/`), the generated Wails bindings (`wailsjs/`, never edited by hand) and `harness/`, a design preview for development |
| `build/` | platform files: icons, macOS plists, the Windows installer script and manifest, the Linux package definition |
| `scripts/` | build, packaging and acceptance scripts |
| `.github/workflows/` | CI (`ci.yaml`) and the daily pin check (`pins.yaml`) |

Credentials go in the OS keychain under the same service and account names
as the CLI, so signing in with either client signs in both. Install records
and pinned signing keys are shared files under `~/.config/agentry/`
(relocatable with `AGENTRY_CONFIG_DIR`).

## Releases

Signed builds of each release are published as this repository's GitHub
Releases, with `SHA256SUMS`, its signature `SHA256SUMS.minisig` and
`BUILD-INFO`. A tag is `vX.Y.Z` or `vX.Y.Z-dev.N` and names the
`productVersion` in `wails.json` (`make version-check`). How to install and
check a download: <https://docs.secureagentry.net/agentry/downloads/>.

## Conventions

Conventional commits. Run `make check` before every push.

## Security

See [SECURITY.md](SECURITY.md).

## Licence

[MIT](LICENSE)
