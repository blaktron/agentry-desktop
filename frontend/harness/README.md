# Design harness

A dev-only preview of the shell chrome and the shared primitive layer
(`src/ui.tsx`, `src/primitives.tsx`, `src/brand.tsx`) rendered against the
real `src/style.css` and the vendored IBM Plex fonts. It exists so a
redesign can be **seen and screenshotted** without WebKitGTK or the Wails
Go backend — the same reason the site has a design page.

It is not part of the app: it sits outside `tsconfig.json`'s `include` and
is never referenced by the app's Vite entry (`index.html` → `src/main.tsx`),
so `make check` and the shipped `frontend/dist` never touch it.

Run it:

```
cd frontend
npx vite harness --port 5199
```

Then open http://localhost:5199/. Both themes follow the OS setting
(`prefers-color-scheme`); a screenshot tool that can emulate the colour
scheme (Playwright `colorScheme: 'light' | 'dark'`) captures both.

Since the guided run setup (agentry-desktop#226) RunPanel shows one step at
a time, so each `#runpanel-*` state opens on the step that shows it (a
blocker on Review and start, the key form on Connect tools), and
`#runpanel-step-files | -step-tools | -step-details | -step-review` show the
steps themselves. A state with a run attached (`-queued`, `-running`,
`-finished`, `-failed`) opens on the run, not the setup.

The result comes first since agentry-desktop#227: `#runpanel-finished` and
`#runpanel-failed` show how the run ended, the rendered result and the files
in the run folder with their previews over a collapsed Run details, and
`#runpanel-hostile` is a finished run whose result and files carry hostile
markdown (raw HTML, a script, an image, a `javascript:` link) and a file name
with a right-to-left override; `shots.mjs` fails it if any `img`, `script`,
`iframe`, `onclick` or `javascript:` link appears in the result view.

A run on this machine has two finished states since agentry-desktop#269:
`#runpanel-local-finished` and `#runpanel-local-finished-env` open on
Review and start, and `shots.mjs` clicks Run and then Run details (a state
may name its `click` as a list), so the shot shows the operator policy the
run enforced: its path, how the path was chosen (the runner home's default,
or `AGENTRY_RUNNER_HOME` from the environment) and its digest abbreviated,
from the envelope's policy block (agentry-cli#152).

The author's guide (manifest v2's `guide` block, agentry-desktop#228) has
its own states on pr-review-brief's shape: `#runpanel-guided` (Before you
start with the author's overview and "What to have ready"),
`#runpanel-guided-files`, `#runpanel-guided-tools` (each key's help under
its field) and `#runpanel-guided-details` (the author's task placeholder).
`#runpanel-guided-hostile` and `#runpanel-guided-hostile-tools` carry a
guide with raw HTML, a script, an image, `javascript:` and `file:` links, a
URL with a user name, a look-alike host and hidden characters, and a key
named `constructor`; `shots.mjs` fails them if any of it becomes markup or
if the links are not exactly the `https:` URLs expected (the look-alike as
its punycode).

The Build tab has a start page and two views since agentry-desktop#280
(plan builder-rework.md M2). `pages.html` opens it on the start page;
`#build-steps`, `#build-tools`, `#build-guide` and `#build-review` open Step
by step on Basics, Tools and web access (the reach summary and a server's
configuration), Before a run (its preview) and Review; `#build-advanced` is
every section at once, and `#build-comments` is a loaded `agent.yaml` with
comments, which a save does not keep. `BuildAPI.Check` is mocked with a
passing answer, so each step shows the lines it writes.

Page previews live in `pages.html` (`pages.tsx`) — Browse, AgentPage,
Installed, Build, Validate and Settings under the shell. Overlays live in
their own entries so each gets a clean screenshot over the shell backdrop:
the run surfaces in `modals.html` (`#runpanel | #toolspanel | #chat`) and
the first-run wizard in `wizard.html` (`#0` welcome, `#1` sign-in, `#2`
runtime, `#3` coding agent, `#4` done; `#3-missing` shows neither coding
agent on the machine, `#3-unprobed` the step before "Look for coding
agents" is clicked, `#4-nothing` the Nothing set up ending when every step
was skipped — agentry-desktop#89).

The **unscanned** states (M23, agentry-desktop#80) are `#agent-unscanned`
in `pages.html` — shot twice, on the provenance tab (the badge in the
header, the mark on the card) and on the report tab (unscanned as its
own verdict, never "scanned, 0 findings") — and `#runpanel-unscanned` /
`#install-unscanned` in `modals.html` (the notice with its own mark and
the consent checkbox; In Agentry not offered for such a release; the
install shot is the wizard's review, one click past Prepare). A state
behind a click names it in `shots.mjs` (`click`).

The run panel's **In Agentry** states (hosted runs M6, agentry-desktop#77)
are hashes of the same entry: `#runpanel-hosted` (the mode chosen, with the
hold, the balance and the tier's caps), `#runpanel-unavailable` (offered,
unavailable, with the reason and the remedy), `#runpanel-queued` (queued
with its position), `#runpanel-running` (its state and nothing else: a run
In Agentry keeps no log, agentry-desktop#182), `#runpanel-finished` (done —
pass: the outcome, the receipt listed verbatim, the credits and the
downloaded outputs) and `#runpanel-failed` (done — fail, a run pushed before
agentry-enterprise#121, for which the registry kept no receipt: the card
says so rather than read one from the outputs). The bindings are
mocked in `modals.tsx`, so no registry is involved.

The run form asks only for what the release declares (agentry-desktop#167):
`#runpanel-keys` in `modals.html` is `agentry/pr-review-brief`'s shape — one
GitHub MCP server with `GITHUB_TOKEN`, no intake — with the keys unsaved, so
the line above the grey Run names them and Input files is a collapsed
disclosure; `#runpanel-keys-saved` the same after Save keys (the line gone,
Run enabled); `#runpanel-file-intake` a release that asks for a file up
front. `shots.mjs` asserts the line, the button's state and the collapsed
disclosure.

The one-click install and its checks (agentry-desktop#146, #147) are
`#agent-installed`, `#agent-installed-older`, `#agent-installed-unreachable`,
`#agent-install-refused`, `#agent-installing` and `#installed-updates` in
`pages.html`; `#installed-update-warning` is Installed's Update of a
runner-home record completing with a warning (agentry-desktop#235), and
`shots.mjs` clicks Update and asserts the engine's sentence is on the page. **Settings → Coding agents** (agentry-desktop#148) reads
`#coding-<state>` there: the default has Claude Code chosen (the Claude
skill, the switch off — which also names the agent page's link),
`#coding-codex` Codex alone (AGENTS.md in a chosen project folder, the
switch on), `#coding-none` nothing chosen and no folder (the switch cannot
go on). `shots.mjs` asserts the last, and that the link opens the wizard on
the Claude skill.

`page-<name>` shoots every page under the shell whole, `toolspanel` and
`chat` the other two run surfaces, and `wizard-welcome`, `wizard-signin`,
`wizard-done` and `wizard-done-nothing` the wizard steps the states above
leave out, so a change to the token table (agentry-desktop#49, checked by
`internal/designtokens`) is seen everywhere it lands.

`shots/` holds those states in light and dark, re-made by
`node harness/shots.mjs` — it starts the harness itself, shoots each state
and stops it again. Playwright is not a dependency of this app; point
`PLAYWRIGHT_MODULE` at an installation that has it:

```
cd frontend
PLAYWRIGHT_MODULE=../../agentry-web/node_modules/playwright/index.mjs \
  node harness/shots.mjs
```
