# Agentry Desktop

A cross-platform desktop client (macOS, Windows, Linux) for
[Agentry](https://secureagentry.net) registries. Browse scanned and signed
agents, install them with **digest and DSSE-signature verification on this
machine**, track what this machine has accepted, and validate local agent
folders with the same scanners the registry runs.

The product never claims an agent is "safe". The language is always:
*scanned, N findings, signed.*

## Surfaces

- **First run** — a skippable wizard until the app is onboarded: Welcome,
  Sign in, then **Local or Cloud** (agentry-desktop#842, D-081), two cards
  with a light-line figure each. **Local** goes on through Runtime (the
  model provider and the images), **AI Review key** (agentry-desktop#462,
  D-051: the recommendation to get your own TypeSafe key, with *Get a key
  from TypeSafe*, and the field that keeps it in the keychain), Coding
  agent, and Done. **Cloud** is locked, with *Please sign in* across it,
  until the registry is signed in; it ends on one page saying how data kept
  in Agentry's cloud is protected, with Back and Finish. Each choice sets
  Settings → Run defaults → Where to run. Done lists what is set up and what
  is not; "nothing is set up yet" goes by whether a model provider was set
  up, and the AI Review key's row never counts toward it.
- **Home** — the page the app opens on (agentry-desktop#658, D-067): a card
  per thing that is yours, in the Build, Voice and Knowledge cards'
  language. **Recent runs** (agentry-desktop#659, D-068) lists the last
  five runs, chats and built-in jobs (drafts, reviews, knowledge and voice
  builds) on this computer and, signed in, In Agentry, newest first; a row
  opens the **run page**: how it ended, its result and files, or for a run
  In Agentry whose output is not here **Download output** (no credits), and
  for a built-in's job a button to Build, Knowledge or Voice. **Open in
  Build** loads the selected completed draft, including its notes and AI
  Review result; a hosted draft's existing output is downloaded without
  starting another run or spending credits. Unavailable or invalid output
  is reported on the run page instead of opening an empty Builder (#722).
  **Installed QuickAgents** counts what is on this computer and
  what has an update, and names the newest five; **Profile** shows who is
  signed in, the plan and the credit balance (a **Sign in** card when not
  signed in); **Run defaults** shows where runs go, their time and turn
  limits and the output folder. Each card opens its page and edits
  nothing. Which cards Home shows, in what order, is the `home` key of
  `desktop.yaml`: a versioned, ordered list Go owns, which keeps whatever a
  newer app wrote there.
  **Pins** (agentry-desktop#660, D-067): **Pin to Home**, on a Library row
  or a QuickAgent's page, puts a QuickAgent installed on this computer on a
  **Pinned** card, four to a card and at most four cards, so 16 pins. Go
  places each one in the first card with room. A pin is written only to
  `desktop.yaml`, asks no registry and is not a star. Each pin shows the
  installed version and how its latest run ended, with **Open latest run**
  and **Run** (the usual run setup). Uninstall or Forget in the app removes
  the pin once no install of that kind is left.
  **Settings → Home** (agentry-desktop#661, D-067's second addendum), which
  Home's **Home settings** link opens, has a switch per default card (a
  hidden card keeps its place in the order), the pins with **Unpin**, and
  **Reset Home…**, which restores the default cards and removes every pin
  after a confirm that says so. A layout a newer app arranged is held as
  it is; Reset is the way back.
- **Scheduled** — runs on this computer for later, or on repeat like a
  calendar entry (plan `quickagent-scheduling.md` M4, agentry-desktop#743,
  D-073). A hand-built month calendar shows each day's occurrences as dots
  in the colour of what happened (ran, failed, missed while the computer
  was off) or hollow when due, beside the chosen day's list and
  **Upcoming**; **Schedules** lists each one with **Edit** (its name, its
  repeat, whether it also runs signed out), **Pause**/**Resume**,
  **Run now** and **Delete**; **History** lists every fire, a failed one
  with its exit class. **Schedule a new run** picks an installed QuickAgent
  and opens its run setup with **When** on Repeat (every day, the next
  whole hour). The run setup's Review and start carries **When** for every
  run: *Now*, *Later* or *Repeat* (every day, weekdays, chosen days of the
  week, or a day of the month, at a time, in a time zone, ending never, on
  a date or after N runs), with **Also run when I'm signed out** on Linux
  with Docker Engine (disabled elsewhere, with the reason). With Later or
  Repeat the button reads **Schedule**, and it stays off until the agentry
  command's check (`agentry schedule add --check`) finds nothing a fire
  would lack; each gap is listed. The CLI keeps the schedules: the app
  reads its store under the runner home in process, and makes every change
  with the **installed agentry command** (`bin\agentry.exe` beside the
  Windows app; on macOS and Linux the one the site's install script put in
  `~/.local/bin`, else the Linux packages' `/usr/bin/agentry`), whose path
  each schedule's entry in the system's scheduler pins. On macOS and Linux
  the page's **Options** menu offers **Install the agentry command** (or
  **Reinstall** it, once one is found), and the notice that says the
  command is missing or too old points there (agentry-desktop#788, D-076):
  after a native dialog it runs the site's own `/install.sh`, which puts
  the CLI that site serves in `~/.local/bin`, checked against the SHA-256
  the site lists, and adds that folder to PATH in the shell's startup file
  once; Node is left to the person. The app runs `agentry schedule sync` at start, and
  `agentry schedule reseal` after a model key, a provider key or a sign-in
  changes in Settings, so a signed-out schedule's sealed keys follow it.
  A run a schedule fired, the app open or not, shows in Recent runs marked
  *Scheduled* and opens on the run page.
  The page's **Options** menu offers **Pin to Home** (agentry-desktop#744,
  plan M5, D-073's addendum): one Large **Scheduled** card at the end of
  Home, this month's mini calendar with the same dots and the next three
  occurrences, which opens the page. It is a card, not one of the 16 pins;
  Settings → Home lists it after the default cards with its own switch,
  **Unpin from Home** in the same menu removes it, and Reset Home removes it
  with the pins. The card reads the store in process and runs nothing.
  **In Agentry** (agentry-desktop#745, plan M10, D-073's second addendum):
  signed in to a registry that schedules runs, the page also lists the
  account's schedules there, labelled *In Agentry*, on the same calendar,
  in Upcoming and in History, and the Home card counts and shows them. A
  fire that started a run reads *Run started* and opens the run's page In
  Agentry; one that started none says why (skipped for credits, a run
  still going, and so on: "Nothing ran."). Edit (name and repeat),
  **Pause**/**Resume**, **Run now** and **Delete** call the registry's
  schedules API with the token the app already holds (`run:read`,
  `run:write`). A registry that does not answer leaves them out, said in a
  sentence, and the schedules on this computer work as they did. In the run
  setup, *Later* and *Repeat* are offered In Agentry on Pro, Promo Pro,
  Business and Enterprise (Standard is told so, with the way up); the
  registry's create checks every input a fire will need before it keeps
  anything, and what it names is fixed in place: a secret the account does
  not keep, or holds for one run, is asked for and kept; a server Agentry
  keeps no connection to opens Connect tools in the browser, then
  **Schedule again**.
- **Library** — the QuickAgents installed on this computer (the shared
  install state below), and **Browse** beneath it in the sidebar, shown
  while either is open (agentry-desktop#657, D-066). The Library's search
  filters what is installed as you type; its button, **Search the registry
  for "…"** (**Find more QuickAgents** when the field is empty), opens
  Browse searched for those words.
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
  mirroring the CLI's `--allow-unsigned`; an unscanned release — one the
  registry admitted without a scan — requires its own checkbox, mirroring
  `--allow-unscanned`, which the unsigned one does not give, and a run of
  one names that consent in its receipt; a signature that fails to verify
  aborts — there is no override for that. The wizard shows the
  registry's verdict on each declared MCP server, and an advisory the
  registry's gate rates high or critical refuses runs. On this computer
  such a server runs only in Unsafe Mode, where the run panel's "allow
  vulnerable MCP servers" consent, asked again in a native dialog, lets one
  run through and the receipt names Unsafe Mode and the server (D-055); the
  operator policy's `mcpServers.allowVulnerable` is the CLI's way through,
  not this app's, and a run In Agentry has none. The agent page shows the
  verdict per server, with each advisory's own severity and the gate's
  rating, and, for the publisher, the open dependency alerts.
- **Run** — run a QuickAgent from the app (RUNNER-PLAN-2 §P6) with the same
  lifecycle `agentry run` executes: verify (digest + DSSE, the install
  gate) → materialize into the shared `~/.agentry/runner` cache → compile
  the capability contract into a run plan → execute in an isolated
  container topology where tools come only from a per-run Docker MCP
  Gateway. Agent pages gain an **MCP servers & tools** section: the tools
  the contract grants, the servers involved, declared key NAMES, and the
  embedded-install equivalent. A guided form collects a QuickAgent's keys
  once per release into its MCP profile — values 0600 on disk, never shown,
  never logged. Settings → **Provider settings** writes the LLM provider into the
  same operator policy the CLI reads, and can prepare the gateway images up
  front. On Linux the docker card in Settings → **Run defaults** names the machine's state — `installable`
  (the app installs the engine through `pkexec`), `groupMissing` (the engine
  is in, your user is not in the `docker` group: one button adds you, through
  `pkexec`), `pendingRestart` (installed and in the group, but this session
  cannot use it until it restarts: **Restart now** asks the session manager
  to reboot, **Restart later** keeps the card until the engine answers),
  `ready`, or `manual` (a hand-off to the official installer, or an installed
  engine that refuses for its own reason). The app never runs as root. Runs
  stream their log live and finish with the §R8 result envelope — status,
  usage, receipts, and the output folder. A run leaves its setup at **Run**
  and is followed above the pages (agentry-desktop#465, D-052): a progress
  dialog shows the time against its limit, turns and tokens over the log;
  **Run in the background** shrinks it to a card in the corner while the
  rest of the app stays usable; **Stop** ends it with its receipts written as
  far as it got; once over, the dialog is its result, **Back to the setup**
  reopens the setup as you left it, and **Dismiss** removes its card (its
  folder stays). Since D-068 the app keeps a **run history** of the last 50
  runs on this computer, the built-ins' jobs included, in `runs.yaml` beside
  `desktop.yaml` (0600): each run's id, release, times, how it ended (from
  the engine's own result) and its folder, never the task, inputs, result or
  log. Home's Recent runs and the run page read it, and only by run id;
  Settings → Run defaults → **Remember recent runs** (on by default) turns
  it off, which forgets every run as before, and **Clear run history**
  empties it. A QuickAgent that asks for a server (a **target host**,
  agentry-desktop#812, D-082) takes it in Details as `user@host[:port]`,
  with the sign-in saved for that account and host, or a private key file
  from the file dialog (and its passphrase) or a password, and an optional
  sudo password; **Save this sign-in** keeps it in the keychain as the CLI's
  own entry, service `agentry`, account `host:<user>@<host>:<port>` (a key
  as its file's path). When the run starts, a host key neither your
  `~/.ssh/known_hosts` nor the runner home's `known_hosts` holds is shown in
  a dialog with its type and SHA256 fingerprint, and accepting it keeps it
  in the runner home's file; a changed key refuses the run. The servers runs
  reached are listed as recent hosts in `recent-hosts.json` beside
  `desktop.yaml` (0600, the last ten, addresses and account names only).
  Such a QuickAgent runs on this computer only: nothing In Agentry is
  offered for it. A QuickAgent that tests websites declares a **browser**
  (`capabilities.browser`, agentry-desktop#878): the install review, the
  consent dialog, the Library and the agent page name it in the CLI's
  consent words (where it may go, and the keys it may type into the site's
  sign-in forms), the agent page lists the browser's tools, and Details asks
  each site question as the address of the site to test (`https://` or
  `http://`, a staging server on your network included), never as a file to
  fetch. The run's browser, its proxy and its tools are the CLI's
  (v0.6.117). In Agentry it runs too (agentry-desktop#903), its site held to
  what a sandbox host can open (a public address, ports 80 and 443) and never
  sent as a source. One local job runs at a time: a run, a chat, a
  draft, a knowledge build or a review started meanwhile is refused with
  what is running. What a run needs while it is live
  (its containers, volumes and networks, and a scratch directory under the
  runner home that holds its secrets file, staged inputs and copies of the
  attached knowledge) is removed when it ends. The app records each run it
  starts, and if it is killed mid-run the next start removes what that
  record names, as quitting does for anything still recorded (a review's
  staged copy in the system's temp directory included, D-056); the output
  folder, the QuickAgent's install and its profile are left alone. Not
  reached by that sweep: a profile the killed run was to remove
  (agentry-desktop#478); the opened copy of an attached knowledge pack,
  which the local store removes about an hour on; a knowledge build's work
  directory (agentry-cli#358); the gateway's anonymous volume, which a
  finished run leaves too (agentry-cli#458); on macOS and Windows, the
  containers the gateway itself started; the docker filter's socket
  directory under the system's temp directory, used when the runner home's
  path is too long for a socket; and files another user owns. The sweep
  takes the runner home to be this machine's alone (agentry-desktop#480).
- **PDF reports** (agentry-desktop#730, D-072; agentry-desktop#759, D-075) —
  a run can turn its report into a PDF with its `export_pdf` tool, drawn in
  the account's **report design**: Settings → **Report design**, the same
  setting as the site's. The page shows the core's five themes (Plain,
  Classic, Modern, Editorial, Technical) as thumbnails the runner's own
  template drew; choosing one loads it into an editor of the accent, the
  fonts, the title block, the headings, the tables, the quotes, the footer
  and the paper, beside a live likeness. The core's `pkg/reportdesign`
  judges every state, so an accent too pale for where it is used is
  refused with the nearest shade that passes. **Save** keeps the design on
  the account, whole; **Reset** clears it. A local run or chat reads it from
  the signed-in registry when it starts and hands it to the engine whole,
  and the receipt names it by its SHA-256; signed out, or never chosen, runs
  from this app use Plain. The QuickAgent never picks it. In the run's
  result, a PDF that the run's envelope (`result.json`'s `pdfs` block) lists with the
  SHA-256 of its bytes has **View**, which draws it in the app with a
  bundled pdf.js (no script, form or link in the PDF is live), **Open in
  system viewer**, which opens a fresh copy of the checked bytes, never the
  file in the run folder, and **Save**. Any other PDF in the folder is
  **Save** only, and says it is not one the run's PDF export made.
- **Sessions** — a release that declares the `interactive` mode can run as a
  chat instead: **Start session** in the run panel opens it under the same
  controls as an autonomous run. Every message, and every file you add
  (read-only under `/inputs`), goes through the runner's session API. A chat
  is followed in the same dialog and corner card as a run (agentry-desktop#467),
  so it can go to the background mid-turn and come back with the reply. A chat
  on this computer has no idle timeout, wall clock or turn count by default
  (agentry-desktop#468): it ends when you end it or quit the app. Settings →
  **Run defaults** → **Chat idle timeout** sets an idle limit for chats on
  this computer, kept in `desktop.yaml`, never in the policy file the CLI shares, whose
  chat caps are unchanged. **Finish** asks the agent for its final result
  before teardown; **Stop** ends without one. Either way the receipts folder
  holds `transcript.md` and the session's events.
- **In Agentry** — where the registry mounts the runs extension and the
  release declares the autonomous mode, the run panel offers a third
  choice: the platform runs it, not this machine (plan
  `agentry-notes/plans/hosted-runs.md`). The same consent panel, intake
  form, task and input files; the app uploads the files, creates the run
  with its own token, and shows the run queued (with its position), then
  running, then done with pass or fail — it keeps no log of the run — and
  downloads the result, reports, receipts and outputs into the run folder
  you picked. It is followed in the same dialog and corner card as a run
  here (agentry-desktop#466), and a run In Agentry still going when the app
  starts comes back as its card. The panel states what the run holds in credits
  and what the balance is, both as the account reports them, and the
  finished run shows the outcome, the runner's exit class and the receipt's
  `capabilities_enforced` listed verbatim. It needs no docker engine and no
  operator policy on this machine; a release that declares a key cannot run
  there yet and says so. Sign-in asks for the `run:read` and `run:write`
  scopes; a token minted before that keeps working everywhere else, with In
  Agentry unavailable and the reason shown.
- **The install state** — the Library lists the shared install state
  (`~/.config/agentry/installed.yaml`, the same file the `agentry` CLI
  writes), with per-item capabilities and update checks. Uninstall deletes
  client-managed directories only: a directory you picked is never deleted.
- **Knowledge** — the knowledge packs a run can search: cited pages the
  built-in Knowledge Creator compiles from your documents, kept with them, at
  most 10 MB each. They are kept in the runner home in the CLI's own sealed
  store (`agentry knowledge` lists and opens the same packs): each is
  encrypted under a key of its own, which the registry you are signed in to
  wraps for your account (on SecureAgentry to your account's key, which
  Agentry holds; on a core registry to a key it keeps for you), so a pack
  opens on any computer you sign in on. Signed out, this computer's key in the keychain
  wraps it instead, until your next sign-in moves it. **Create knowledge** and
  **Add documents** run the Creator on this computer against your runner
  policy's model, for no credits, and say before the build that nothing they
  read or write is scanned. Since agentry-desktop#619 (D-064) the page is two
  cards, as Voice's: **Local Knowledge** lists the packs kept here and
  **Cloud Knowledge** the ones your account keeps in Agentry, with your
  organizations' shared packs grouped by organization. Each card and page has
  Create knowledge, its Where step preset to that place. **Copy to Cloud**
  keeps a copy in Agentry, where it is scanned, and **Copy to Local** brings
  one here; neither removes the copy it starts from. A pack kept in both
  places is on both pages, and Delete on a page removes that place's copy
  alone. **Export unencrypted…** asks
  in a native dialog, then writes the pack into a folder you pick and shows
  that folder. A run's setup has a Knowledge step: an attached pack is mounted
  read-only for the run's tools, and the receipt names it.
- **Voice** — the Writing Voice, the instructions a run follows to write the
  way you do (plan `writing-voice.md` V6, agentry-desktop#510, D-054; since
  agentry-desktop#610, D-063, two cards as the Build page has). Two places,
  never synced on their own. **Local Voice** is this computer's, kept sealed
  in the runner home by the CLI's store, free and never reviewed: its page
  has the editor, **Download Cloud version**, **Upload to Cloud** (AI Review, for the
  registry's credit), **Edit with AI**, the switch that puts it on every run
  here, and Delete. Download and Upload say "Already the same" when the two
  voices match, and ask first in a native dialog when they would put an
  older voice over a newer one: equal digests are the same voice, otherwise
  this computer's last save is weighed against when the account's version
  was set. **Edit with AI** is one dialog, what should change and optional
  writing samples from the native pickers, and runs the Voice Writer on this
  computer for no credits; with no voice here it is **Write with AI** and
  writes one from samples. **Cloud Voice** is the account's, which AI Review
  reads before a version is live (since agentry-desktop#611, the site's Voice
  settings page in the desktop): a version waiting for AI Review is shown
  above the editor, in review (the page follows it) or flagged with what AI
  Review found, with **Discard**; the editor saves a new version for AI
  Review; **Edit with AI** is the same dialog run **In Agentry** at the
  registry's flat price (an AI Edit of the newest version with optional
  samples, or a Create from samples where the account keeps none; Standard's
  once-a-cycle limit and the upgrade are said in its place when used), and
  what it writes waits for AI Review; then the account's switch, **Restore
  previous** and **Delete**, the last behind a native dialog. Each voice
  has an always-attach switch (Settings → Run defaults for this computer's,
  shown on the Local Voice page too; Profile for the account's), and every
  run setup's Review shows **Write in my voice**, ticked from the switch of
  the voice that run uses. Both pages' editor is WYSIWYG since
  agentry-desktop#612 (plan `voice-page-redesign.md` §6, D1): Tiptap v3
  reads `VOICE.md` as Markdown and saves Markdown, in one view, with a small
  toolbar (heading, bold, italic, lists, quote, code), the Markdown input
  rules and shortcuts. A Voice Writer's output comes back byte for byte;
  Save sends nothing when the voice is as it was opened (on the Cloud page
  it would spend a credit), writes every block the person did not change
  back as the bytes it was read from, and checks 32 KiB on the Markdown it
  would send. Raw HTML, an image's Markdown and any link but an absolute
  http(s) or mailto one stay text; a link is drawn with no address to
  follow, so no click in the editor opens it, and its address and title are
  shown beside it, as a code block's info string is above it; invisible
  characters show as their code points. A voice
  with a table, a definition or a task list, which the editor cannot hold,
  opens as Markdown text in a plain box instead, so no edit can drop that
  part. Tiptap is loaded only when an editor is drawn, in a chunk of its own.
- **Validate** — under Build in the sidebar, shown while Build is open
  (#422): an author drop-zone. Point it at a local QuickAgent folder and
  it runs the manifest rules, the S1 inventory (content-sniffed binaries,
  nested archives, encoding), and the S3/S4 hidden-content and
  injection-signature scanners. Those are the same Go packages the
  publication pipeline runs (D-12 local/remote parity by construction), with
  the rule bundle embedded in the binary.
- **Build** — author a QuickAgent, validate it, and publish it over the same
  upload path the CLI uses. The tab opens on a start page of two cards, as
  the site's `/build` (agentry-desktop#598): **Describe it to AI**, which
  opens Describe it, and **Build it by hand**, which opens Step by step and
  offers Advanced from the card. Each card can be pressed anywhere on it.
  - **Describe it** drafts one from a plain description, with the site's
    three optional hints (the services or sites it works with, how it runs,
    an example of the output). The built-in Drafter (the one `agentry draft`
    runs) drafts it in one of two places, **In Agentry** or **on this
    computer**, and the page offers In Agentry first where it can be chosen.

    **In Agentry** the Drafter runs on Agentry's infrastructure against the
    platform's model, for credits, and needs you signed in.
    - Before Draft it the page states the hold in the site's sentence ("This
      draft holds 41 credits and settles from what it uses: …"), your
      balance, the draft's caps, and where your words go: to a search
      provider, to AI Review (TypeSafe's Jev) and to the sites the Drafter
      reads, so the description should hold no secret. AI Review reads every
      result through the registry before the Drafter's model does. A payer
      short of the hold is told so before anything is sent.
    - The page follows the draft (queued, then drafting) with its hold and
      **Cancel this draft**, and a reopened app picks it up again.
    - When it ends, its output is unpacked into
      `~/Downloads/agentry-out/agentry-drafter-<id>`, and `draft.json` there
      is read into the form as data. It must be a file of at most 4 MiB, the
      fields the form cannot hold are dropped and named, and a value past its
      bound refuses the whole draft. Nothing in the output is opened or
      launched.
    - The settlement names its searches' and AI Review's parts.

    **On this computer** it runs against the model your runner policy names,
    for no credits. It searches the registry and, when your policy names a
    search provider and you have a key for it, the web, then reads the pages
    it finds.
    - AI Review reads your description first, and then every search result,
      page and registry answer before the Drafter's model does, with your own
      AI Review key (Settings → **Provider settings** → **AI Review key**). What AI Review flags, or
      cannot scan, is held back: the model gets the address and the reason,
      never the text. A description it does not pass stops the draft before
      the Drafter starts.
    - Without a key, drafting on this computer is off, and the page says
      where to add one.
    - A native dialog asks before anything leaves this computer. Your
      description, the searches and the pages go to TypeSafe, to your model's
      provider, to your search provider when you have one, and to the sites
      read, so the description should hold no secret.
    - The draft is a run like any other: it holds the run lock, and quitting
      ends it. You can leave the page while it runs, and the draft opens on
      Build when it is done.

    **Either way:**
    - The draft opens on the **summary** (below), so describing it, **Draft
      it** and **Submit** are the whole path.
    - What does not depend on the task is the Builder's, whatever the draft
      said, and each change is named: your namespace, version `0.1.0`, the MIT
      licence (the licence file Save writes) unless your own words name the
      draft's, `AGENT.md`, the include list, and a name none of your own
      QuickAgents already has (`-2`, `-3`…).
    - Review, in the form views, keeps what the Drafter found folded under
      **How it was drafted**: AI Review's check of the finished draft (each
      rubric question it raised, with its file), what AI Review read and held
      back, **the hosts the draft may reach that your description did not
      name**, the rules the draft still fails, the published QuickAgents that
      already do the task (each with a link to its page), what the
      description leaves open, the Drafter's notes (shown as a run's result
      is, with no HTML and no images) and its sources.
    - **Describe again**, on Review, takes a note on what should change and
      makes a new draft in the same place from what you first asked and the
      draft as it stands in the form. The new draft replaces this one.
    - The draft is decoded into the form as data. Fields the form cannot hold
      are dropped and named, and an instruction file outside the folder
      becomes `AGENT.md`.
    - Nothing is written into a QuickAgent folder until you save it into one
      you pick. The run's receipts and `draft.json` are kept in
      `~/Downloads/agentry-out/agentry-drafter-<run id>`.
  - **Step by step** walks eight steps: Basics, Instructions, Tools and web
    access, Files, commands and data, Questions, Before a run, Publishing,
    Review. Each step is checked with the core's own manifest rules before
    Next, and shows the lines it writes to `agent.yaml`.
  - **Advanced** shows every field at once. It is the same form as Step by
    step, so switching loses nothing.

  **The summary** is the one publish page, where a draft lands and where
  both form views end (**Continue to submit**). It says how the QuickAgent
  works (its summary and the guide's overview) and everything it needs:
  each key with the author's help, the websites it reaches, each MCP server
  with its tools, keys, configuration and hosts, and each file, command or
  data grant, and nothing it does not declare. A host a draft's description
  did not name is marked, and AI Review's check of a draft gets one line.
  It ends on three actions (agentry-desktop#425):
  - **Save** writes the QuickAgent to a folder. The first save asks for one
    through the native picker, which opens in Documents.
  - **Install** puts it on this computer only, in the Library. It is private
    and local: no registry, no pipeline and no credit, through the CLI's
    local install. A native dialog shows the contract and says the install
    is unsigned. The Library runs it, and every run asks for the unsigned
    consent.
  - **Submit** sends it to the registry and makes it **public**. A native
    dialog first names what the submission costs, read from the account,
    and sends nothing on No. An agent that already exists private stays
    private: making it public is its namespace administrators' act, which a
    desktop sign-in does not carry, so the result links to its settings on
    the site. The form's check holds Submit to the public rules (V7): the
    publish list must carry README.md and LICENSE.

  Beside them, **Review Your QuickAgents locally** (agentry-desktop#469)
  saves the QuickAgent and reviews the folder with the built-in **Examiner**
  (the one `agentry review` runs), on this computer, before you submit. It
  is offered on Validate too, for a folder chosen with **Choose…**.
  - It never runs by itself, and Submit, Save, Install and Validate are the
    same with or without it.
  - It needs your own AI Review key; without one it says so and opens
    Provider settings at the key.
  - A native dialog asks first: the QuickAgent's text goes to TypeSafe (AI
    Review) with your key and to the model your runner policy names, and the
    registry is asked, with no sign-in, for its public policy and its record
    of each declared MCP server. Nothing of the QuickAgent runs and nothing
    is submitted.
  - The registry's own rules run here (the embedded rule bundle, as
    Validate's), then the Examiner runs over a read-only copy of the files a
    submission would carry, for no credits. It is a local job under the run
    lock, followed in a progress dialog and corner card, with its own
    **Stop** (`StopReview`, which stops only a review). The copy is removed
    when the review ends, and swept at the next start or quit if the app
    died first.
  - The report shows the outcome the pipeline would be expected to reach
    (published, held for review, rejected, refused at Submit, or not
    determined here) under the registry's public policy, from the rules and
    AI Review alone, and every finding under its kind with its file and
    line: the registry's rules, AI Review asked as the pipeline asks it, and
    the Examiner's own reading, which is never counted. It lists what did
    not run here and ends with the registry's notice: it scans again at
    Submit, and its answer is the one that counts.
  - `report.json` and the run's receipts are kept in
    `~/Downloads/agentry-out/agentry-examiner-<run id>`, a folder no earlier
    run wrote. **Dismiss** removes its card; the folder stays, and the run
    history keeps the review for Recent runs (D-068).

  A rule the form still fails is named with **Fix it**, and Install and
  Submit wait for it. **See it in the form** opens Step by step on Review,
  or Advanced.

  The form covers the whole of manifest schema v2, the run guide and each
  server's plain configuration included. Opening an existing workspace and
  saving it keeps every field; only comments are lost, and the page says
  so. The folder is chosen at the first save.
- **Report bug** (agentry-desktop#841, plan `report-bug.md` §6) — a small
  link directly above the version in the sidebar's bottom-left corner. It
  opens a card with a text box, Cancel and Submit. Submit files the text with
  the active registry's bug-reports extension as the account signed in there,
  with the app's version and OS and nothing else. Agentry's staff read it on
  the site and email you when it is fixed. Signed out, the card offers Sign in
  instead. A registry without the extension says it does not take bug reports.
- **Settings** opens on five cards, each a link to its own page with a
  **← Settings** link back; a card says in one line where things stand
  (read from this computer only, never from a registry) and links straight
  to each of its parts. "Run setup again" and the settings file's path stay
  on the cards' page (agentry-desktop#346, plan settings-cards.md).
  - **Registry settings**: the registries, with sign-in, the account line and
    the organizations; the secrets runs In Agentry use; Add a registry.
  - **Update settings**: whether the app looks for a newer Agentry Desktop,
    and the QuickAgents auto-update switch (not offered yet).
  - **Coding agents**: the coding agent, and whether Install also writes a
    QuickAgent where it reads.
  - **Run defaults** (agentry-desktop#347, D-044):
    - **Timeout and max turns**: the runner policy's budgets, so `agentry
      run` from the CLI runs with them too. A run's own fields under
      Advanced apply to that run only, longer or shorter, as the CLI's
      `--timeout` and `--max-turns` do. With the switch below turned off, a
      draft made on this computer has at least 30 minutes and a knowledge
      build at least 60, however short the timeout (agentry-desktop#458):
      the floors are the CLI's, so `agentry draft` and `agentry knowledge
      create` give the same.
    - **No time or turn limit for the built-in agents** (agentry-desktop#606,
      D-062; on by default and over the turns too since
      agentry-desktop#699, D-062's addendum): a switch under the timeout,
      kept as `builtinNoLimit` in `desktop.yaml` (absent means on) and never
      in the runner policy. While it is on, a draft (Describe it, Edit with
      AI), a knowledge build, a local review and a Writing Voice run on this
      computer have no timeout and no turn limit the app shows: each runs
      until it finishes or you stop it. A hidden failsafe of 200 turns (or
      the policy's max turns when that is higher, and 200 under a policy of
      0, unlimited) still ends one that loops; no progress line or log names
      it, and the receipt records it. The cost cap still applies, and a
      draft's repair turn has no timeout too, with at most eight tool calls
      and only the remaining turns and cost. A model that hangs mid-turn
      meets neither cap: Stop ends it. With a timed draft, repair keeps at
      most five minutes within the draft's remaining time.
      Turned off, the built-ins keep their floors above and the policy's max
      turns. Your QuickAgents' runs, the CLI and anything In Agentry keep
      their limits, and it applies from the next job started.
    - **Chat idle timeout** (agentry-desktop#468, D-052): a chat on this
      computer has no idle timeout, no wall clock and no count of your
      messages by default, so it ends when you end it or quit the app. Set
      a number of minutes here and the next chat ends after that long
      without a message from you. It is kept in `desktop.yaml`, not in the
      runner policy, so `agentry run` keeps the policy's session limits
      (900 s idle, 7,200 s, 200 turns) and a chat In Agentry keeps
      Agentry's idle close. Max turns above still limits a chat's tool
      calls unless it is 0 (unlimited), and the run setup offers no timeout
      for a chat.
    - **Where to run**: This computer or In Agentry. It only pre-selects the
      run setup's Where step. In Agentry is chosen only where it is offered
      and available and the release was scanned; otherwise the run starts
      on this computer and the step says why. The credits a run In Agentry
      holds are still shown before Run, and every consent is still asked on
      every run.
    - **Output folder**: chosen here through the file dialog and kept in
      `desktop.yaml`. The run setup fills it in and you can pick another for
      one run. Each run writes into a folder of its own inside it, and a run
      admits it as an output folder only, never as an input.
    - **Unsafe Mode** (agentry-desktop#418, D-050; plan unsafe-mode.md): a
      switch, off by default, kept as `unsafeMode` in `desktop.yaml` and
      never in the runner policy.
      - What it lifts: while it is on, a run on this computer no longer
        needs this computer's exec catalogue (the runner policy's
        `exec.packages`) to name each command the QuickAgent declares. The
        commands are installed from Kali Linux's repository or PyPI on an
        image built for that QuickAgent from the Unsafe Mode image
        (`agentry-unsafe-kali`, the policy's `images.unsafe`), and each runs
        with that image's own PATH (the CLI's DR-53). A command that cannot
        be installed is left out and the run goes on without it.
      - What stays: `run_command` still takes only the commands a
        QuickAgent declares (a declared interpreter, such as `python3`, can
        start anything the image holds, and the image is not minimized);
        the container's isolation, its network limits and every consent are
        unchanged. A run In Agentry never uses it, and `agentry run` has no
        such flag.
      - Turning it on: the app asks in a native dialog, downloads the image
        with its progress in the card, and only then does the switch read
        on. The page cannot turn it on by itself (D-030), and no other
        setting's save writes it: `settings.SetUnsafeMode` is its one
        writer. Turning it off is immediate and keeps the images; Docker
        images removes them.
      - While it is on: a still red edge runs round the window, over the
        app's own dialogs, and the sidebar's foot carries an "Unsafe Mode"
        badge that opens this part. Review and start shows the line a run
        carries (what Unsafe Mode does, the commands the release resolves
        with, and the ones the image cannot provide), the consent dialog of
        a run that accepts something carries the same line, and a finished
        run shows the receipt's exec line: what was installed from where
        and what did not install. The line lists only names that are
        command names, and a bounded number of them: it is built before the
        release is downloaded and verified, from what it resolves to.
    - The Docker engine's state and, on Linux, its setup.
    - **Docker images**: Agentry's images on this computer and a button
      that removes them (agentry-desktop#424, D-048), the Unsafe Mode image
      and the exec images built on it among them.
  - **Provider settings**: the LLM provider form and the AI Review key.

  The update line in the sidebar opens Update settings, the "Setup isn't
  finished" notice opens Provider settings, Knowledge's "Not signed in"
  opens Registry settings, and Describe it's missing AI Review key opens
  Provider settings at the key, and the sidebar's Unsafe Mode badge opens
  Run defaults at the switch.
- **Settings → Provider settings → AI Review key** — your own TypeSafe Jev key, which every
  local action that requires a scan uses (today, Describe it on this
  computer; plan builder-rework.md D25).
  - What it leads with: the operator's recommendation to get a key of your
    own, and *Get a key from TypeSafe*, which opens TypeSafe's site in the
    system browser. The words and the address are one module's
    (`frontend/src/keyRecommendation.ts`), so every place that offers the key
    says the same; Describe it's missing-key notice carries the same link
    (agentry-desktop#461). The paragraph beneath says what the key is used
    for, and that submitting needs no key of yours.
  - Where it is kept: in the OS keychain next to your model key, as the entry
    `provider:typesafe` under the CLI's `agentry` service. The runner policy
    records only `judge: {apiKeySource: keyring}`, so `agentry draft` reads
    the same key. `$TYPESAFE_API_KEY`, set where the app is started, is used
    first, and the page says so.
  - What the page shows: "saved", never the value. The key goes to the app's
    backend in a call of its own, and no log line or answer carries it back.
  - **Test key** makes one small call to TypeSafe with the key a draft would
    use and says the answer in plain words: it works, TypeSafe refused it
    (401, 403), is limiting it (429), or could not be reached.
  - **Forget the saved key** removes it from the keychain and the policy.
  - A keychain that cannot be reached refuses the save, and the policy is
    left as it was.
  - Inside a draft, the key reaches only the tools container, where the
    Drafter's web tools run, never the agent's container.

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
`services_knowledge.go`, `services_prereqs.go`, `services_run.go`) expose flat
view models to the frontend; engines live in
`internal/{registry,install,validate,settings,run,build,knowledge,mcpvuln,version}`.
The local knowledge store is the CLI's `pkg/knowledge/store` and its registry
client `pkg/knowledge/remote`. `internal/knowledge` wires them to this app's
runner home, keychain and sign-in, and repeats the CLI's create, add and attach
flows (which live in the CLI's `package main`) over the same library calls.

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
| lint + tests + tsc, and the frontend's unit tests (`npm test` in `frontend/`, node's own runner) | `make check` |
| regenerate TS bindings | `make bindings` (after changing bound Go APIs) |
| macOS universal .app, zipped and as a .dmg | `make package-darwin` |
| Windows amd64 exe, zip and per-user installer (cross-compiled; `SIGN_CMD=scripts/sign-windows.sh` signs, the release does) | `make package-windows` |
| Linux amd64 binary, .deb and .rpm (in a container) | `make package-linux` |
| install both Linux packages in clean containers | `make smoke-linux-packages` |
| drag the dmg's bundle over an installed copy (macOS) | `make smoke-darwin-dmg` |
| what update-signing keys this build trusts | `go run ./internal/cmd/keyset list` |
| load a new update-signing key (operator) | `scripts/operator/update-signing-key.sh` |
| connect the release workflow to Azure Artifact Signing (operator) | `scripts/operator/artifact-signing-setup.sh` |
| connect another repository's release (the CLI) to the same signing identity (operator) | `scripts/operator/artifact-signing-setup.sh --repo blaktron/agentry-cli` |
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
(`internal/uninstall`) before the files go. A release signs both outputs
and the uninstaller inside the installer (#54, "OS code signing" below); a
PR build does not. The `windows` CI job builds all of this on every PR;
what Linux cannot check — install N over N-1 with settings in place, the
default uninstall keeping them, the box removing them — is
`scripts/acceptance/windows-install.ps1`, run by hand in a plain user
session on a Windows machine until #55 gives CI one.

**The agentry command** a release carries (`bin\agentry.exe`, appended to
the user Path; `/usr/bin/agentry`; D-061) comes after a CLI the site's
install script put first on PATH. So that the one a terminal finds is never
the older of the two, the installer's `--add-cli-to-path` run (install and
every silent self-update) and every app start on Windows and Linux replace
the script's copy at its default place
(`%LOCALAPPDATA%\Programs\agentry\bin\agentry.exe`,
`${XDG_BIN_HOME:-~/.local/bin}/agentry`) with the carried one when its
`--version` is lower. A copy that is as new or newer, whose version cannot
be read, or that is a symlink, is left alone; on Linux only the packages'
`/usr/bin/agentry` counts as carried, and nothing runs as root
(`internal/clipath/refresh.go`; #770, D-074).

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
never touches a real sign-in. A
production release Developer ID-signs, notarizes and staples the bundle and
then the image (#6, below, "OS code signing"); anywhere else — a PR build, a
dev prerelease, a by-hand build — the bundle is ad-hoc signed only and the
image is not signed at all (D-001), and macOS refuses to open a downloaded
copy until the person allows it in System Settings → Privacy & Security.
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
trip there, and uploads the zip and the dmg as a workflow artifact. It is
a required check (#109): the dmg read-back and the install-over-install
smoke it runs are the ones that gate a release. The two steps that need
the runner's user at the console, the Keychain round trip and the launch
smoke, report themselves skipped without one (a warning on the run); the
Keychain round trip has never run on the runner for that reason (#106).
With the Mac offline the job stays queued, so the gate ends in a timeout
rather than green or red.

## Releasing

Every build that ships is released under a version (DEV-148,
agentry-desktop#279 and #192). A tag is one of two forms, and the branch line
its commit is on must agree:

- `vX.Y.Z` on a commit on `main`'s first-parent line (a promotion's merge
  commit) is a **production release**; it bakes `https://secureagentry.net`
  and checks that site's downloads manifest for newer production releases;
- `vX.Y.Z-dev.N` (N from 1) on a commit on `dev`'s first-parent line is a
  **dev prerelease** of the version `main` releases next; it bakes
  `https://dev.secureagentry.net` and is offered only newer dev prereleases.

`scripts/release-tag.sh` (the CLI's, copied unchanged) checks the tag in the
`release` workflow's preflight, before anything is built: any other form, a
form on the wrong line, a release that does not move forward, and any 1.x
unless the operator has set the repository variable `AGENTRY_ALLOW_V1=true`
(agentry#522) are refused. The version lives in one place, `wails.json`'s
`productVersion`, always `X.Y.Z`: the Wails build carries it into the macOS
bundle's `CFBundleShortVersionString` and the Windows version-info block,
which take numbers only, and a tag must name it (`vX.Y.Z` or
`vX.Y.Z-dev.N`). The Makefile stamps the tag into the binary (`make version`
prints `git describe` without the leading `v`: `0.6.0`, `0.6.1-dev.2`, or
`0.6.1-dev.2-3-g<sha>` for a commit past a tag), and the self-update check
reads that to keep each channel to itself.

**Cutting one** (agents may; only the operator moves the minor version or
cuts 1.0.0). `productVersion` on `dev` is the version `main` releases next:
after `main` releases `vX.Y.Z`, a PR on `dev` bumps it to `X.Y.(Z+1)`; the
promotion carries it to `main`. Then:

```bash
git fetch --tags origin
tag=$(scripts/release-tag.sh next dev) && make -s version-check RELEASE_TAG="$tag" && git tag "$tag" origin/dev && git push origin "$tag"    # a dev prerelease of dev's head
tag=$(scripts/release-tag.sh next main) && make -s version-check RELEASE_TAG="$tag" && git tag "$tag" origin/main && git push origin "$tag"  # a production release of main's head
```

Run it from a checkout of the branch being tagged, so `version-check` reads
that branch's `productVersion`. `next` must name `productVersion`'s `X.Y.Z`,
and `version-check` stops a tag that does not before it is pushed.
`RELEASE_VERSION=X.Y.0` is the operator's, for the first tag of a minor they
have chosen; the first cut of all, v0.6.0 (the operator's R1), is one:
`next` alone would name v0.5.2 after v0.5.1. Tag a head only after its merge is verified,
and watch the `release` run to green. The workflow refuses a tag whose pins of
the core or the CLI are below their newest releases (the desktop reads files
the CLI writes;
D-020). It then builds every platform artifact — Windows and Linux on the
Linux runner, the universal macOS bundle and its dmg on the Mac — writes
`SHA256SUMS` over them, signs those sums with the estate's own update-signing
key (below), and uploads the set as one workflow artifact named
`agentry-desktop-<version>`. `BUILD-INFO`, covered by the sums, names the
commit, the event (a tag push, or a `workflow_dispatch` dry run the site's
importer refuses), `signed=false`, and the registry the build connects to
by default, with the `channel` it was released on. `build/release.env`
names both registries and is the same on `main` and `dev`;
`scripts/release-registry.sh` picks one by the tagged commit's first-parent
history: a commit on `main`'s line is a production release and bakes
`https://secureagentry.net`, one on `dev`'s is a dev release and bakes
`https://dev.secureagentry.net`, and any other commit is refused before a
binary is built (agentry-desktop#183, the CLI's agentry-cli#240 copied
unchanged). The update channel is that registry's downloads manifest, and the
jobs check the URL and the channel are really in the binaries. Between tags the bundle
metadata keeps the last `productVersion` while the binary reports the
describe string; only a tagged build has one version everywhere, which is
what a release is.

### The release is signed with our own key

`SHA256SUMS` in the bundle is signed, and `SHA256SUMS.minisig` travels beside
it. That signature is the trust root for "a newer Agentry Desktop": the app will
check it before it replaces itself (M6, #87). It is **our** key, not the
operating system's — independent of Developer ID signing (#6) and
Authenticode (#54), which secure a different thing and are not needed for this
to work. The site's downloads importer checks it against this repository's
committed key set before it copies anything, and refuses a bundle whose
signature is missing or does not verify (agentry-web's
`scripts/release/manifest.py`, called by `build-downloads.sh --from-run`;
blaktron/agentry-web#154); it then publishes the sums and the signature beside
the artifacts.

- `internal/selfupdate/keys.json` is the set of keys a build trusts: each with
  a name, its public half, its key id and whether it is retired. A retired key
  still *verifies*, so an old release stays checkable, and callers refuse to
  *install* from one — that is what makes a rotation a release rather than a
  reinstall. A build with an empty set trusts nothing and refuses every update,
  which is the correct state for the open core. The same key signs the CLI's
  and the MCP server's release bundles, so its private half is meant to live
  under one name, `AGENTRY_UPDATE_SIGNING_KEY`, in three places: this
  repository's secrets (it does today), agentry-cli's (once agentry-cli#197
  loads it) and agentry-mcp's `release` environment (once agentry-mcp#33
  does). Each of those two pins
  `DESKTOP_SIGNRELEASE_REF` to the commit of this repository whose key set it
  is checked against. A rotation moves all three secrets and both pins
  together; either alone makes that product's next release fail closed.
  `scripts/operator/update-signing-key.sh` offers to load all three (#181).
- `internal/cmd/signrelease` signs in the release job and then verifies what it
  wrote against that committed key set, using `internal/selfupdate` — the same
  code the app uses. A release whose signature a shipped app would refuse
  therefore fails the release instead of shipping, which is the mistake a
  rotation invites: a new secret without the matching public half committed.
- The signed trusted comment names the product: `agentry-desktop 0.2.0
  (commit …)` for this application, `agentry-cli 0.1.0 (commit …)` when the
  CLI's release workflow runs `signrelease -product agentry-cli` at a pinned
  revision of this module (#171). The app's update check reads only its own
  product's comment, so a CLI release never reads as a desktop update.
- The private half is a repository Actions secret,
  `AGENTRY_UPDATE_SIGNING_KEY`. It is an environment variable on the signing
  step — as an Actions secret must be to reach a step at all — and from there
  it is piped to the signer on standard input, so it is never written to the
  runner's disk; `env -u` drops it from the signer's own environment, and it
  reaches no command-line flag, so it is not in a process listing. Nothing on
  the Mac ever sees it: the macOS job signs only with Apple's certificate (#6),
  never with this key.
- **`scripts/operator/update-signing-key.sh`** is how a key is loaded, and the
  only supported way: it generates the pair on the machine running it, sets the
  secret, writes the public half into the key set with
  `internal/cmd/keyset`, and never prints the private half. It also offers a
  password-protected backup, which matters for `prod`: a GitHub secret can be
  replaced but never read back, so without one the private half exists nowhere
  reachable. Run it once for `dev` (#110) and again for `prod` on
  operator-held hardware at launch (#111, labelled `launch-prerequisite`),
  retiring `dev` in the same run.

`internal/selfupdate/keys.json` holds the `dev` key since 2026-09-20 (#110);
`prod` is loaded at launch (#111). With an empty key set the release workflow
**refuses every tag** — it will not publish a release no shipped app could
check. A `workflow_dispatch` dry run warns and skips signing
instead, so the rest of the release path stays exercisable.

### OS code signing is a second, separate signature

Authenticode on Windows and Developer ID with notarization on macOS are what
the *operating system* checks, and they are being added as two independent
tracks (`agentry-notes/plans/os-code-signing.md`; epics #153 and #154). The
Windows half signs on the Linux runner against **Azure Artifact Signing**,
whose HSM holds the key (Microsoft's statement: the certificate "is never
given to you"): the release workflow's `linux-windows` job runs in the
`release` environment, proves who it is with the OIDC token GitHub mints for
it (`scripts/signing-token.sh`, the exchange the `signing-oidc-probe`
workflow runs on its own), gets an access token that lives for the job, and
hands it to `jsign` through the environment of the one make target that
signs (`make sign-windows`; `scripts/sign-windows.sh` pins jsign's version
and digest). The exe is signed before `makensis` embeds it; the installer and
its uninstaller are signed by `makensis` itself through the `!finalize`
hooks in `project.nsi`, which exist only when `SIGN_CMD` is defined, so a PR
build stays unsigned, and a signer that fails fails the build.
`scripts/check-windows-installer.sh --signed` then verifies all three with
`osslsigncode` to the Microsoft root pinned in `build/windows/signing/`,
insists on a verified timestamp (the certificate lives three days), requires
one CN across the three and, when `build/release.env` sets
`AGENTRY_WINDOWS_SIGNING_CN`, that CN — the pinned root is every Artifact
Signing customer's, so the name is what says the signature is ours. There
is no unsigned fallback: a missing variable, a refused exchange or a
signature that does not verify is a release that does not complete, and a
`workflow_dispatch` dry run signs too, from `main` or a `v*` tag (the
environment admits nothing else). `scripts/acceptance/windows-signing.sh`
proves the seam on every PR with a throwaway CA in place of the key. The
workflow holds no Azure secret: its identity is the OIDC token, and the six
repository variables are identifiers (`AZURE_SUBSCRIPTION_ID` is the
operator wizard's, unused by the release).

The macOS half (#6; plan §5, D4, D5) signs on the Mac runner, in the
`macos` job, with Apple's own tools, and only in a **production** release (a
`vX.Y.Z` tag on `main`'s line, or a dry run there): `make build-darwin`
builds the bundle; `scripts/sign-darwin.sh app` imports the
`APPLE_DEVELOPER_ID_P12` certificate into a keychain it makes in its own
`0700` directory and deletes on exit (failing if it is still on the search
list), signs the bundle inside-out under the hardened runtime with Apple's
timestamp and **no entitlements**, submits it to Apple's notary service with
the `APPLE_NOTARY_*` API key, waits for "Accepted" and staples the ticket;
`make wrap-darwin` then makes the zip and the dmg from the stapled bundle;
`scripts/sign-darwin.sh dmg` signs, notarizes and staples the image; and
`scripts/check-darwin-dmg.sh --signed` and `scripts/sign-darwin.sh assess`
read the shipped dmg, the bundle on it and the bundle in the zip back — the
identity and Team ID `build/release.env` names, a designated requirement
naming the team, a valid stapled ticket, and `spctl`'s "Notarized Developer
ID". The five secrets reach only the two signing steps, never `wails` or
`npm`, and a production release without them fails. A dev prerelease
(`-dev.N`) skips Apple and ships ad-hoc signed, so it does not wait on the
notary queue (each submission took 40 to 75 minutes in October 2026, and the
job makes two in a row); the CLI's dev prereleases do the same
(agentry-cli#570). Headless `codesign` on the Mac runner needs Apple's
Developer ID G2 intermediate in the host's System keychain (#152). The first
launch of a downloaded copy on a fresh Mac is a by-hand check: the runner has
no console session (#106).

`BUILD-INFO` records what the release verified, under the estate's own
signature over `SHA256SUMS`: `signed=` stays `false` because the Linux
packages carry no OS signature, and

```
signed_windows=authenticode CN=<subject CN> profile=<account>/<profile> timestamp=<osslsigncode's time> verified=osslsigncode root=<the root's subject>
```

— the keys in that order, each value running to the next key; the CN, the
timestamp and the root carry spaces. The site's importer reads this line
(blaktron/agentry-web#187). A production release also writes

```
signed_darwin=developer-id TEAM=<team id> notarized=true
```

(empty on a dev prerelease), the grammar the CLI's release already writes
for its darwin binaries; the importer's reading of it for the desktop is
blaktron/agentry-web#188.
**`scripts/operator/artifact-signing-setup.sh`** is how the operator connects
the repository to their validated identity (and, with `--repo OWNER/NAME`,
another repository this sign-in administers — the CLI's release will sign
with the same identity through its own federated credential and `release`
environment once agentry-cli#197 runs it) — it creates the Entra app
registration and its one federated credential (whose subject it reads from
the repository's own OIDC settings, since a subject carries the owner and
repository ids now), assigns the Certificate Profile Signer role on the
signing account and nowhere wider, creates the `release` environment with a
deployment policy that admits only `main` and `v*` tags, and sets six
repository *variables* (`AZURE_TENANT_ID`, `AZURE_CLIENT_ID`,
`AZURE_SUBSCRIPTION_ID`, `ARTIFACT_SIGNING_ENDPOINT`, `ARTIFACT_SIGNING_ACCOUNT`,
`ARTIFACT_SIGNING_PROFILE`; `ENDPOINT=<host>` in the wizard's environment
overrides the endpoint it derives). The `signing-oidc-probe` workflow proves
the exchange from a real job in that environment before any signing step
exists (#151). The signing step itself is #54: its `linux-windows` job joins
the `release` environment with `id-token: write`, which also means a
`workflow_dispatch` dry run must start from `main` or a `v*` tag once it does.

**Every tag publishes a GitHub Release** with the bundle (agentry-desktop#192;
DEV-148 reverses D-023's "no GitHub Release before 1.0"): a `-dev.N` tag's is a
prerelease, a `vX.Y.Z` tag's a full release and the newest the latest, with
notes generated from the commits under a header naming the signatures. Before
1.0.0 every release publishes at once; from 1.0.0 on a minor or major release
(`vX.Y.0`) is left a draft: the operator approves its notes and publishes it. The site's downloads builder imports the bundle from the run
(agentry-web#444 moves the import to the Release). `workflow_dispatch` runs the
same jobs as a dry run on a commit on `main`'s or `dev`'s line (the `release`
environment admits `main` and `v*` tags), named by the describe string
instead of a tag, and publishes no Release. Every artifact
on Windows is Authenticode-signed (#54, below); on macOS a production
release's bundle and dmg are Developer ID-signed and notarized (#6, below)
and a dev prerelease's are ad-hoc signed only (D-001); the Linux packages
carry no OS signature. The download page says which Windows and Linux
artifacts are signed; it says so of the macOS ones once
blaktron/agentry-web#188 reads `signed_darwin=`. The plan behind this and the installers that
follow: `agentry-notes/plans/desktop-installers.md`.

## The registry it talks to

The app works against any Agentry registry — the hosted service or one you
run from the open core ([self-hosting guide](https://github.com/blaktron/agentry/blob/main/docs/self-hosting.md)).
Add one under Settings → Registry settings → Add a registry: its URL
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
