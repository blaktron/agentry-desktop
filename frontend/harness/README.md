# Design harness

A dev-only preview of the app's pages, dialogs and shared UI, rendered in a
plain browser against the real `src/style.css` and the vendored IBM Plex
fonts. The Go backend is replaced by mocks, so any state of the UI can
be opened, driven and screenshotted without WebKitGTK or the Go host.

The harness is not part of the app. It sits outside `tsconfig.json`'s
`include` and is never referenced by the app's Vite entry (`index.html` →
`src/main.tsx`), so the shipped `frontend/dist` never contains it. It is
type-checked with the app by `tsconfig.harness.json` (`make check` runs
`npx tsc --noEmit -p tsconfig.harness.json`), so a page's props cannot drift
from the states that render it. Vite builds the harness without type-checking.

## Run it

```
cd frontend
npx vite harness --port 5199
```

Open http://localhost:5199/. Both themes follow the OS setting
(`prefers-color-scheme`); a screenshot tool that emulates the colour scheme
(Playwright `colorScheme: 'light' | 'dark'`) captures both.

## Entries

Each entry is an HTML page with a React module that mocks the bound calls
(`window.go.<package>.<Type>.<Method>`, which `runtime-mock.ts` answers the
generated bindings' calls from) and the runtime events, and selects a state
from `location.hash`. The header comment of each `.tsx` file lists every hash it
understands and what that state shows.

| Entry | Module | Shows |
|---|---|---|
| `index.html` | `gallery.tsx` | the shell chrome and the shared primitives in `src/ui.tsx`, `src/primitives.tsx` and `src/brand.tsx` |
| `pages.html` | `pages.tsx` | Home, Library, Browse, an agent's page, Installed, Build, Validate, Voice, Publisher and Settings under the shell; `?settings=<section>` opens a Settings section |
| `modals.html` | `modals.tsx` | the run setup (`RunPanel`), the tools panel, the chat pane, the install review and the bug report card |
| `wizard.html` | `wizard.tsx` | the first-run wizard, step by step |
| `knowledge.html` | `knowledge.tsx` | the Knowledge pages, Create knowledge, Add documents and Preview |
| `drafts.html` | `drafts.tsx` | a draft's progress dialog and its corner card |
| `reviews.html` | `reviews.tsx` | Validate, a local review's progress and its report |
| `runs.html` | `runs.tsx` | a run on this computer, a run In Agentry and a chat, followed above the pages, and the run page |

The sidebar in every entry is the app's own `SidebarNav` (`src/nav.tsx`).

Each state is a hash on its entry, for example `modals.html#runpanel-hosted`
or `wizard.html#3-refused`. The mocks answer as Go does for that state, so a
state that needs a refusal, a slow save or a hostile payload has its own hash.

## `shots.mjs`

`shots.mjs` starts the harness itself, opens each state in light and dark,
runs the checks written beside the state, saves a screenshot and stops the
harness again. Playwright is not a dependency of this app, so point
`PLAYWRIGHT_MODULE` at an installation that has it:

```
cd frontend
PLAYWRIGHT_MODULE=/path/to/node_modules/playwright/index.mjs \
  node harness/shots.mjs
```

The screenshots are written to `frontend/harness/shots/`, which is ignored by
git. Environment and options:

- `STATES=name1,name2` limits the run to a comma-separated subset of state
  names.
- `HARNESS_PORT` sets the port the harness is started on (default 5199).

Each entry of the `states` list in `shots.mjs` has a `name`, a `url` (entry
and hash), a `note` and optional steps and assertions:

- `click` (a selector, or a list of selectors clicked in order), `fill` and
  `type` drive the page before the shot; `wait` pauses for the page's own
  timers; `locator` shoots one element instead of the page; `window: true`
  shoots the whole 1180×760 window.
- `expectText` and `expectNoText` assert text on the page; `expectSelector`
  and `expectNone` assert that an element shows or is absent.
- `expectOrder` asserts the document order of a set of elements,
  `expectMinWidth` that matching elements keep a minimum width,
  `expectChecked` and `expectValue` the state of a control, and
  `expectOpened` the addresses the app asked the system browser to open (the
  harness's `App.OpenExternal` records them in `window.__opened`).
- `expectSection` and `expectPart` check which Settings page shows and that a
  named part is in view; `expectGlow` checks the Unsafe Mode edge covers the
  window; `expectFootDisabled` checks every footer button of the wizard is
  held.

States that render content from a run, a draft, a guide or a pack carry
hostile markdown (raw HTML, a script, an image, a `javascript:` link, a
right-to-left override). `shots.mjs` fails them if any of it becomes markup
or a link.

## `docker-recheck.mjs`

Drives the container engine's explicit re-check through the real Provider
settings, the prerequisites card and the wizard, against the mocks. It is
run by hand, not by `make check`: it needs a Playwright installation this
app does not depend on. With the harness running on port 5199:

```
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
  node harness/docker-recheck.mjs
```

It picks the custom endpoint's provider tile and types an endpoint URL and
a key, holds the re-check pending to assert "Checking the container
engine…", then confirms the engine badge without the tile or the two
fields losing what was typed; on Run defaults it clicks the prerequisites
card's Re-detect and asserts the card gives way to the engine found; and
on the wizard's engine step it asserts a waking engine is found and
preparing the images is offered. It exits 0 when every step holds and
prints one line; a failed step raises Playwright's error. `HARNESS_URL`
overrides the harness address and `SCHEME` (`light` or `dark`) picks the
colour scheme.
