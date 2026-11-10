# Screen-reader pass

A manual check of Agentry Desktop with a real screen reader. Run it on each
platform before a release:

| Platform | Screen reader | Webview |
|---|---|---|
| Windows 11 | NVDA (current release) | WebView2 |
| macOS (current release) | VoiceOver | WKWebView |
| Linux, GNOME | Orca | WebKitGTK |

The automated checks (`npm test`, `make a11y`) prove the markup: roles, names,
live regions, contrast and keyboard order. This pass proves what each
platform's webview actually exposes to its screen reader, which the markup
alone cannot. The native parts get the most attention: the card window, the
macOS menu bar, the context menu and the clipboard.

## Before you start

- Install the build under test from its release, on a user account where
  Agentry Desktop has never run, so the first-run wizard opens.
- Have a registry account to sign in with, a model key or sign-in for one run,
  and Docker or Podman running for a run on this computer.
- Note the app version (bottom of the sidebar), the OS version and the screen
  reader's version for the record.
- Keyboard only from step 1 on. Use the mouse only where a step says so.
- "Hear" below means what the screen reader should say, in substance; the
  exact wording differs between screen readers. Anything missing, wrong,
  doubled or out of order is a finding.

## The script

### 1. First-run wizard

1. Start the app. **Hear** a dialog named "Agentry setup", then the welcome
   heading. Focus is inside the dialog.
2. Read the step list. **Hear** "Setup steps", a list, with the current item
   read as "Step 1 of N: Welcome" and marked current.
3. Tab through the dialog several times past its last control. **Hear** focus
   go round its own controls only, never to the page behind it.
4. Press Escape. **Nothing happens**: the setup closes only by Skip or Finish.
5. Choose **Set up**. **Hear** the step move to "Step 2 of N".
6. Go back to the first step and choose **Skip for now**. **Hear** the Home
   page. Focus does not get lost.

### 2. Moving between pages

1. Tab to the sidebar and open **Library**. **Hear** the Library page's
   heading as focus lands on it. The window title reads "Library — Agentry
   Desktop".
2. Use the screen reader's landmark navigation. **Hear** a "Pages"
   navigation and a main region.
3. Open **Settings**, then **Home**. **Hear** each heading on arrival.

### 3. Settings → Home → Accessibility

1. Find the **Accessibility** heading. **Hear** a group "Text size" with
   radio buttons "100% (default)" to "200%", and a group "Motion".
2. Choose **150%**. **Hear** "Text size 150%". The whole window grows.
3. Press Ctrl+= (on a Mac, Cmd with +, View → Zoom In). **Hear** "Text
   size 175%". Press Ctrl+- (Cmd+-, Zoom Out), then Ctrl+0 (Cmd+0, Actual
   Size). **Hear** each new size; the last is 100%.
4. On a Mac, open the menu bar (VO+M) and read **View**. **Hear** Actual
   Size, Zoom In and Zoom Out, each with its shortcut.
5. Choose **200%**. The sidebar folds into a **Pages** button at the top.
   **Hear** "Pages", a button, collapsed. Press Enter. **Hear** it expanded,
   and the rows after it. Choose **Browse**: the rows fold and **hear**
   the Browse heading. Set the size back to 100%.
6. Choose **Reduce motion**. Spinners stop turning. Set it back.

### 4. Browse, install, run, result

1. In **Browse**, **hear** the search field named "Search agents" and the
   filters named "Run mode", "Declared capability" and "Sort".
2. Search for a QuickAgent that runs on this computer. Tab to the first
   result. **Hear** its name as a button. Press Enter. **Hear** the agent's
   page heading.
3. Choose **Install**. **Hear** the install review as a dialog named "Install
   …". Tab past its last control: focus stays inside. Press Escape: the
   review closes and **hear** focus back on the Install button.
4. Install it. Each consent the release asks for is a checkbox; **hear** its
   sentence with it. Install stays unavailable until every consent is
   checked.
5. Open the run setup (**Run**). **Hear** a dialog named "Run …" and the
   step heading "Step 1 of N: …". Choose **Next** without filling a required
   answer. **Hear** the sentence that holds Next, read out as it appears.
6. Fill the steps and run. While it runs, **hear** progress on request, not a
   stream of interruptions. At the end, **hear** the result page's heading;
   its files are reachable by Tab.

### 5. A chat turn

1. Open the Assistant (turn it on under Agents if it is off). Tab to its
   message field and send "Say hello in one sentence."
2. **Hear** nothing while the reply streams. When it is complete, **hear**
   the reply once, after the agent's name.
3. Send a second message. **Hear** only the new reply. The earlier
   conversation is not read again.
4. Clipboard in the message field: type a word, then Ctrl+A, Ctrl+C, End,
   Ctrl+V (Cmd on a Mac). The word is there twice.
5. Context menu in the message field: Shift+F10 or the menu key (on a Mac,
   VO+Shift+M). **Hear** a menu with Cut, Copy, Paste and Select All. Over
   a link in a reply, no context menu opens.

### 6. An error and its retry

1. Turn off the network. Open **Browse** and search.
2. **Hear** the error sentence read out as soon as it appears, and a **Try
   again** button after it.
3. Turn the network back on and choose **Try again**. **Hear** the results.

### 7. The card window

1. With the Assistant on: **Agents → the Assistant → Add a server**, search,
   and choose **Add** on a server. A card window opens for the consent.
2. **Hear** the card's title and its message read together, as an alert
   dialog. Focus starts on the answer that says no.
3. Press Escape. The card closes and nothing is added. **Hear** focus back
   in Agentry Desktop.
4. Add the server again and answer **Yes**. **Hear** its card appear in the
   page.
5. Start a run, switch to another application, and quit Agentry Desktop
   from its tray or Dock menu. A card asks "Quit Agentry Desktop?".
   **Hear** it although Agentry Desktop was not the active app. Answer no.
   Focus goes back to the application you were in.

### 8. Menus

1. An options menu: on the Assistant's page, a server or app card's options
   button (or **Scheduled → Options** where it shows). **Hear** a menu
   button, collapsed. Press Enter:
   **hear** the first item. Up and Down move between items, Home and End go
   to the first and last. Escape closes the menu and **hear** focus back on
   its button.
2. **Home → Deliverables** (after a run that made a file): Tab to a file and
   press Shift+F10 (menu key). **Hear** "Show in folder" as a menu item.
   Escape returns to the file.

### 9. Windows High Contrast (Windows only)

1. Turn on a contrast theme (Settings → Accessibility → Contrast themes).
2. Walk steps 1, 3 and 8 by sight. The wizard's current step dot is larger
   than the others, the open page in the sidebar is outlined, badges keep
   their border and the keyboard focus is always visible.

## Recording a run

Add one comment per platform to the release's screen-reader issue, with:

- the app version, the OS version and the screen reader with its version;
- each step's number and pass, or what was heard instead;
- a new issue for each finding, linked from the comment.

A step that cannot be run says why (no Assistant model, no runtime), rather
than passing.
