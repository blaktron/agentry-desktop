# Design harness

A dev-only preview of the shell chrome and the shared primitive layer
(`src/ui.tsx`, `src/primitives.tsx`, `src/brand.tsx`) rendered against the
real `src/style.css` and the vendored IBM Plex fonts. It exists so a
redesign can be **seen and screenshotted** without WebKitGTK or the Wails
Go backend — the same reason the site has a design page.

It is not part of the app: it sits outside `tsconfig.json`'s `include` and
is never referenced by the app's Vite entry (`index.html` → `src/main.tsx`),
so `make check` and the shipped `frontend/dist` never touch it.

Every entry's sidebar is the app's own `SidebarNav` (`src/nav.tsx`), so
the preview shows the navigation the app has: Validate under Build, shown
only while Build or Validate is open (agentry-desktop#422). `shots.mjs`
holds the order and the lit items with `expectOrder`.

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
steps themselves. `#runpanel-step-review-long` is Review and start with
an intake prompt as long as binary-reverse-engineer's. `shots.mjs` fails
it, and the Build and Create knowledge reviews, if a recap value is
narrower than 320px (`expectMinWidth`): the layout that once put each
value one letter a line (agentry-desktop#420).

The result comes first since agentry-desktop#227: how the run ended, the
rendered result and the files in the run folder with their previews over a
collapsed Run details. A result is drawn by the run's flow since
agentry-desktop#465 and #466, so its states are in `runs.html` (below):
`#run-hostile` and `#hosted-hostile` are finished runs whose result and files
carry hostile markdown (raw HTML, a script, an image, a `javascript:` link)
and a file name with a right-to-left override; `shots.mjs` fails them if any
`img`, `script`, `iframe`, `onclick` or `javascript:` link appears in the
result view.

Talk to it In Agentry, a hosted session (agentry-desktop#403), has three
setup states on an interactive-only copy of the release:
`#runpanel-session-offer` (the choice made, with what a session costs from
the registry's terms), `#runpanel-session-tier` (a payer on Standard:
offered, unavailable, See Pro) and `#runpanel-session-nohost`. The chat
itself is the chat's flow's since agentry-desktop#467, in `runs.html`
(below).

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
(plan builder-rework.md M2). `pages.html` opens it on the start page,
whose two cards `shots.mjs` holds to the site's order and words, Describe it
to AI first (`expectOrder`, agentry-desktop#421, #598); `page-build-by-hand`
presses Build it by hand in the middle of its card and lands on Step by
step, `page-build-by-hand-advanced` presses the card's Switch to Advanced;
`#build-steps`, `#build-tools`, `#build-guide` and `#build-review` open Step
by step on Basics, Tools and web access (the reach summary and a server's
configuration), Before a run (its preview) and Review; `#build-advanced` is
every section at once, and `#build-comments` is a loaded `agent.yaml` with
comments, which a save does not keep. `BuildAPI.Check` is mocked with a
passing answer, so each step shows the lines it writes.

Describe it (#282, plan builder-rework.md M8) is the Build tab's first way
in since #421: `#build-describe` opens it with the person's AI Review key saved and
`#build-describe-nokey` without one (the Settings pointer, Draft it greyed).
`#build-drafted` opens Review with a draft the Drafter made, what AI Review
read and held back, the registry's match with a link to its page, the open
questions, the notes and the sources. `#build-drafted-hostile` is a draft whose
notes, match, source and withheld address carry raw HTML, a script, an image,
`javascript:` links and a right-to-left override; `shots.mjs` fails it if any
becomes markup or a link. Settings → Provider settings → AI Review key is `settings-aireview-key`
(saved), `#aireview-none` (none), `#aireview-env` (the key from the variable
the app was started with) and `#aireview-refused` (Test key answered 401).
Settings → Provider settings → Web search (agentry-desktop#301) is
`settings-web-search` (Brave saved), `#search-none` (no provider),
`#search-env` (the key from the variable) and `#search-refused` (Test key
answered 401).
Since agentry-desktop#461 the card leads with the operator's recommendation
to get a TypeSafe key and the link *Get a key from TypeSafe*, which Describe
it's missing-key notice carries too. `shots.mjs` holds the sentence word for
word in each state (spelled out there, not imported), and
`settings-aireview-key-none` and `page-build-describe-nokey` click the link
and fail unless the app asked the system browser for exactly
`https://typesafe.ai`: the harness's `App.OpenExternal` records each address
in `window.__opened`, and `expectOpened` reads it.

The Knowledge panel (agentry-desktop#273) has its own entry, `knowledge.html`
(`knowledge.tsx`). Since agentry-desktop#619 it is two places: `#knowledge` is
the start page's Local Knowledge and Cloud Knowledge cards signed in,
`#knowledge-local` the Local page (a pack kept here only, one in both places
scanned in Agentry, one changed since it was copied, one past Agentry's cap,
and acme's shared copy), `#knowledge-cloud` the Cloud page (the account's
packs, the allocation, acme's shared group), `#knowledge-signed-out` the Local
page signed out (a pack under this computer's key),
`#knowledge-signed-out-start` the cards signed out, `#knowledge-empty` and
`#knowledge-empty-cloud` none. `#hosted-where-pro-cloud` is Create knowledge
opened from the Cloud page for a Standard account (On this computer
selected), and `#create-done-cloud` a pack made here from it, offering Copy
to Cloud. The harness is type-checked with the app (`tsconfig.harness.json`,
agentry-desktop#485). Create knowledge's
steps are `#create-documents`, `#create-name`, `#create-review` (the recap and
the sentence that nothing the Creator reads or writes is scanned) and
`#create-blocked` (docker not ready, Create greyed); since agentry-desktop#352
`#create-review` and `#add-review` also say a build is a big job and how long it
may run, and `#create-building` shows the build's progress from scripted
`knowledge:build-progress` events (time against the limit, turns, pages
written, tokens) over its folded log, which `knowledge-create-building-log`
opens, `#create-done` shows what it made (`#create-stopped-brief`: a brief-only
build stopped at its turn budget, continued with the same brief, agentry-desktop#600); since agentry-desktop#353 `#card-building`,
`#card-done` and `#card-hosted` show a build shrunk to its corner card over the
page (no backdrop: `shots.mjs` fails them on one); a finished card offers Preview,
and a build In Agentry stays in its card until it is over (before #353 its dialog
could close and Create knowledge followed it again), `#hosted-where`, `#hosted-review`, `#hosted-running` and `#hosted-done` are a build In Agentry (agentry-desktop#274), and `#add-review` is Add
documents. `#preview` is a pack's index and pages, and `#preview-hostile` a
page with raw HTML, a script, an image, a `javascript:` link and a
right-to-left override, which `shots.mjs` fails if any of it becomes markup.
Since agentry-desktop#354 `#create-documents-large` (48 MB: the warning), `#create-documents-huge` (past 1 GB: Next refused) and `#hosted-where-large` (In Agentry not choosable) show the two caps, and `#knowledge-local` lists a 220 MB pack kept here with no Copy to Cloud.

Since agentry-desktop#498 a build takes documents, a brief, or both, with Search the web: `#create-documents-brief` (a brief alone: the warning, louder, and Search the web on by default with this computer's keys), `#create-documents-noweb` (no search key here: a build here runs without the web, and where to add one), `#create-empty` (neither: Next refused), `#create-review-brief` and `#hosted-review-brief` (the review on each side), and `#hosted-done-brief` (what the build's record counted of its searches).

A draft's progress (agentry-desktop#410) has its own entry, `drafts.html`
(`drafts.tsx`), over Describe it: the page starts a draft on load and the
dialog App mounts follows it. `#draft-local` is a draft on this computer under
way from scripted `build:draft-progress` events (the stage, time against the
limit, turns, tokens) over its folded log, which `draft-local-log` opens;
`#draft-agentry` a draft In Agentry under way; `#draft-landed`, `#draft-failed`
and `#draft-refused` how a draft ends; `#draft-hostile` a failed draft whose
error and log carry raw HTML, a script, an image and right-to-left overrides,
which `shots.mjs` fails if any becomes markup or an override is not revealed.
`#card-local`, `#card-agentry` and `#card-landed` are the corner card, and
`#card-stacked` a draft's card and a knowledge build's stacked in App's corner.
`#preview-empty` is a pack a build left short, its index and no page with the
list answered as null, and `#render-fault` a page that throws while rendering,
caught by `Recover` under a sidebar that still works, whose Try again
`shots.mjs` presses to see the page come back (agentry-desktop#351).
The run setup's Knowledge step is `modals.html#runpanel-step-knowledge`, and
`#runpanel-knowledge-finished` a local run with a pack attached, whose result
shows the receipt's knowledge line.

A run on this computer followed above the pages (agentry-desktop#465) has
its own entry, `runs.html`, driven through the run store (`runs.ts`) with
`runner:run-progress` and `runner:run-line` scripted: `#run-running`,
`#run-card`, `#run-stopped`, `#run-finished`, `#run-failed`, `#run-hostile`
and `#run-card-stacked`. The `modals.html` states that click Run on the review
(`#runpanel-local-finished` and the like) now close the setup once the run has
begun and show the result in the run's flow, as the app does.

A run In Agentry is followed the same way since agentry-desktop#466, by its
own store (`hostedRun.ts`) and `HostedRunFlow`, in the same entry with
`runner:hosted-run` scripted: `#hosted-queued`, `#hosted-running` (time
against the cap and no Turns row, since the registry reports turns only once a
run is over), `#hosted-cancel`, `#hosted-finished` (with the receipt's
knowledge line), `#hosted-failed`, `#hosted-hostile`, `#hosted-lost` (the app
lost touch: Follow it again), `#hosted-card` (a run attached at start, as its
card), `#hosted-card-finished` (a run that ended with its dialog closed lands in
its card) and `#hosted-card-stacked` (beside a run on this computer's card).

A chat, on this computer or In Agentry, is followed the same way since
agentry-desktop#467, by the chat's store (`sessionChat.ts`
`createChatStore`) and `ChatFlow`, with the session's events scripted:
`#chat-local` and `#chat-hosted` (the dialog is the chat; the hosted one is
replayed through `ReplayHostedSession`, with a file added, a hostile agent
reply that `shots.mjs` fails if it becomes markup or a link, the
time-and-credits strip and D2's sentence), `#chat-local-working`,
`#chat-local-waiting`, `#chat-hosted-working` and `#chat-hosted-waiting`
(the card: working or waiting for you, the turns, Finish and Open),
`#chat-local-ended` (the engine ended it with the dialog closed and the store
collected its result) and `#chat-hosted-ended` (the result view with the
session's credits and `receipts/transcript.md`).

Settings opens on five cards since agentry-desktop#346. `pages.html` shows the
cards (`page-settings`, which asserts each card and its status line), and
`pages.html?settings=<section>` opens a section page (`registry`, `updates`,
`coding-agents`, `run-defaults`, `provider`), where the Settings states above
are shot; the hash still picks each card's own state. The preview keeps its
own route as App does, so `settings-open-<section>` clicks a card,
`settings-part-link` follows a part's link, `settings-back-<section>` presses
**← Settings** on each section, and `settings-deeplink-<caller>` presses the harness-only
buttons that open Settings the way App's callers do (the sidebar's update
line, the setup notice, Knowledge's sign-in, Describe it's missing key);
`shots.mjs` checks which page shows and that a named part is in view.
Run defaults (agentry-desktop#347) is `settings-run-defaults` (the four
defaults and Docker) and `settings-run-defaults-saved` (a timeout typed and
saved, In Agentry chosen, a folder picked, as the mocks answer for Go);
its Chat idle timeout (agentry-desktop#468) is `settings-chat-idle-none`
(none, the default), `settings-chat-idle-set` (30 minutes saved) and
`settings-chat-idle-cleared` (saved, then No timeout), and
`modals.html#runpanel-chat-review` is a chat's Review and start, whose
Advanced offers no timeout and says why. In
the run setup, `modals.html#runpanel-default-agentry` opens with In Agentry
chosen by default, `#runpanel-default-agentry-unscanned` starts a release
admitted without a scan on this computer and says why,
`#runpanel-default-agentry-signedout` does the same signed out and names the
sign-in, and
`#runpanel-default-folder` shows the saved folder filled in on Review and
start. `shots.mjs` can `fill` a field before its clicks and assert
`expectChecked` and `expectValue`.
`#status-refused` makes every status binding refuse: the five cards stay and
only the registry's line, which reads no binding, is left.

Page previews live in `pages.html` (`pages.tsx`) — Browse, AgentPage,
Installed, Build, Validate and Settings under the shell. Overlays live in
their own entries so each gets a clean screenshot over the shell backdrop:
the run surfaces in `modals.html` (`#runpanel | #toolspanel | #chat`) and
the first-run wizard in `wizard.html` (`#0` welcome, `#1` sign-in, `#2`
runtime, `#3` AI Review key, `#4` coding agent, `#5` done; `#4-missing`
shows neither coding agent on the machine, `#4-unprobed` the step before
"Look for coding agents" is clicked, `#5-nothing` the Nothing set up ending
when every step was skipped — agentry-desktop#89). The AI Review key step
(agentry-desktop#462) is Provider settings' card in the wizard: no key is
saved unless the hash ends `-saved` (or the step is Done with something set
up), and `#3-refused` has the keychain refuse the save. `shots.mjs` reaches
the step the ways a person does, so its neighbours' buttons are held too:
`wizard-review-key` by Continue from Runtime, `wizard-review-key-saved` by
Back from Coding agent, `wizard-runtime` by Back from the key step and
`wizard-coding-agent-unprobed` by Skip on it; it holds the footer to Back,
Skip, Continue with no key and Back, Continue with one (`expectOrder`), the
recommendation word for word, and on `wizard-review-key-refused` types a
key, clicks Save and fails unless the refusal shows and the field still
holds what was typed. `#3-slow` is a save that never answers:
`wizard-review-key-saving` clicks Save and fails unless every footer button
is held (`expectFootDisabled`). `wizard-done` is reached by Continue from
the key step and then from Coding agent, and asserts the provider's row and
the key's; `wizard-done-nothing` asserts the key's row pending under
"Nothing is set up yet"; and `#5-keyonly` (`wizard-done-key-only`) is Done
after the key alone was saved, where the policy exists only because that
save wrote it from defaults: the ending is still Nothing set up, the
provider's row says the policy has only the default and where its key is
read from, and the key's row is checked. `#2-refused` is a save the backend
refuses after the key was stored: `shots.mjs` types the custom endpoint's
form, clicks Save, and fails unless the refusal shows and the key field is
empty and says the key is stored (agentry-desktop#141, #378). The wizard's
mock answers every binding the step calls, `EndpointWarning` included; a
missing one throws inside an effect and unmounts the whole wizard.

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
unavailable, with the reason and the remedy) and `#runpanel-hosted-secrets`
(a start refused secret-missing, the ask in the setup). Once the registry has
created a run, the setup closes and the run's flow follows it: those states
are `runs.html#hosted-*` (above), where `#hosted-failed` is a run pushed
before agentry-enterprise#121, for which the registry kept no receipt, and
the card says so rather than read one from the outputs. The bindings are
mocked in `modals.tsx` and `runs.tsx`, so no registry is involved.

The run form asks only for what the release declares (agentry-desktop#167):
`#runpanel-keys` in `modals.html` is `agentry/pr-review-brief`'s shape — one
GitHub MCP server with `GITHUB_TOKEN`, no intake — with the keys unsaved, so
the line above the grey Run names them and Input files is a collapsed
disclosure; `#runpanel-keys-saved` the same after Save keys (the line gone,
Run enabled); `#runpanel-file-intake` a release that asks for a file up
front. `shots.mjs` asserts the line, the button's state and the collapsed
disclosure.

A run In Agentry of a release that declares a secret (agentry-desktop#300):
`#runpanel-hosted-secrets` in `modals.html` is Review and start In Agentry,
where the Keys row says the account's secrets are used. `shots.mjs` clicks
Run In Agentry, the mock answers `secret-missing` for `PAGERDUTY_TOKEN`, and
the ask shows with Keep it for later runs unticked and Run held behind it.
Settings' Secrets card (`settings-secrets`) is in `pages.html`.

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

Unsafe Mode (agentry-desktop#418, plan unsafe-mode.md M3) has its states on
both entries. On `pages.html`, `?settings=run-defaults` shows the switch off
(`settings-unsafe-off`), `#unsafe-on` the switch on with the red edge round
the window and the sidebar's badge (`settings-unsafe-on`, and
`unsafe-glow-browse` for a page, `settings-landing-unsafe` for the landing's
status line), and `#unsafe-pulling` answers the switch's click as the Go host
does after the native Yes, with the image's pull under way and the switch
still off (`settings-unsafe-pulling`; the pull's lines arrive on the entry's
own event bus as `runner:unsafe-progress`). On `modals.html`,
`#runpanel-unsafe-review` is Review and start with the line a run carries,
naming the declared commands and the one the image cannot provide, under the
edge, and `#runpanel-unsafe-finished` that run finished, its result naming
what was installed from where and what did not install. These shots are of
the window (`window: true` in `shots.mjs`, 1180×760), because the edge is the
window's, and `expectGlow` fails a state whose edge does not cover the
window, sits under a dialog's backdrop, takes clicks, or has no badge saying
"Unsafe Mode" beside it. The native confirmation is the Go host's and is not
in the harness: `TestUnsafeModeIsTurnedOnOnlyThroughTheNativeDialog` holds it.

Docker detection's explicit re-check (`agentry-desktop#594`) is exercised
through the real Provider settings and prerequisite components. With the
harness running on port 5199, run:

```sh
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs node harness/docker-recheck.mjs
```

It holds the re-check pending to assert Checking, then confirms recovery
without replacing unsaved provider, endpoint or key fields, and verifies
that a successful prerequisite re-check clears its failure card.
