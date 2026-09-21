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
with its position), `#runpanel-streaming` and `#runpanel-finished` (the
outcome, the receipt listed verbatim, and the downloaded artefacts). The
bindings are mocked in `modals.tsx`, so no registry is involved.

`shots/` holds those six states in light and dark, re-made by
`node harness/shots.mjs` — it starts the harness itself, shoots each state
and stops it again. Playwright is not a dependency of this app; point
`PLAYWRIGHT_MODULE` at an installation that has it:

```
cd frontend
PLAYWRIGHT_MODULE=../../agentry-web/node_modules/playwright/index.mjs \
  node harness/shots.mjs
```
