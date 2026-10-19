#!/usr/bin/env node
// Writes THIRD_PARTY_NOTICES, the licence texts of the third-party software
// and assets Agentry Desktop ships, or checks that the committed file is
// current.
//
//   node scripts/notices/generate.mjs           write THIRD_PARTY_NOTICES
//   node scripts/notices/generate.mjs --check   exit 1 when it is stale
//
// (make notices, make notices-check.) The file is a function of the lockfile
// and go.sum, the module cache and the licence files under frontend/src: no
// date, no host name, every list sorted by code unit, so two runs on the same
// tree write the same bytes.
//
// What it covers:
//   1. The npm packages compiled into the frontend bundle, found from the
//      source maps of a real production build (not the whole dependency
//      tree: a build tool is not shipped).
//   2. The Go modules compiled into the app, the union of the Linux, macOS
//      and Windows builds with the build tags the packages use, with each
//      module's licence and NOTICE files, and the Go licence for the
//      standard library the binary carries.
//   3. The fonts, from the licence files kept beside them.
//   4. The provider logos' licence.
//   5. Microsoft WebView2 on Windows.
//   6. The system libraries the packages do not carry.
//
// Needs the frontend's node_modules (npm ci) and the Go modules in the
// module cache. A package or module without a licence file stops the run
// naming it: a notice that cannot be written is not left out silently.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const frontend = path.join(root, "frontend");
const outFile = path.join(root, "THIRD_PARTY_NOTICES");

// The build tags each package's build uses, per target: scripts/build-linux.sh
// for Linux, wails build's own for the others. The union is what is listed.
const GO_TARGETS = [
  { goos: "linux", goarch: "amd64", cgo: "1", tags: "desktop production webkit2_41" },
  { goos: "darwin", goarch: "amd64", cgo: "1", tags: "desktop production" },
  { goos: "darwin", goarch: "arm64", cgo: "1", tags: "desktop production" },
  { goos: "windows", goarch: "amd64", cgo: "0", tags: "desktop production" },
];

// The report fonts' licence files, by family. A file in the directory that
// is not named here, or a name here without a file, stops the run: a font
// added or dropped is a decision about its notice.
const REPORT_FONT_LICENCES = {
  "ebgaramond.OFL.txt": "EB Garamond",
  "ibmplexmono.OFL.txt": "IBM Plex Mono",
  "ibmplexsans.OFL.txt": "IBM Plex Sans",
  "ibmplexserif.OFL.txt": "IBM Plex Serif",
  "inter.OFL.txt": "Inter",
  "jetbrainsmono.OFL.txt": "JetBrains Mono",
  "lora.OFL.txt": "Lora",
  "merriweather.OFL.txt": "Merriweather",
  "montserrat.OFL.txt": "Montserrat",
  "playfairdisplay.OFL.txt": "Playfair Display",
  "sourcecodepro.OFL.txt": "Source Code Pro",
  "sourcesans3.OFL.txt": "Source Sans 3",
  "sourceserif4.OFL.txt": "Source Serif 4",
  "worksans.OFL.txt": "Work Sans",
};

const RULE = "=".repeat(78);
const SUB = "-".repeat(78);

const byCode = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

function fail(msg) {
  console.error(`notices: ${msg}`);
  process.exit(1);
}

// clean: a licence text with LF line ends, no trailing space and no blank
// lines at either end, so a file checked out on Windows reads the same.
function clean(text) {
  return text
    .replace(/^﻿/, "")
    .replace(/\r\n?/g, "\n")
    .split("\n")
    .map((l) => l.replace(/[ \t]+$/, ""))
    .join("\n")
    .replace(/^\n+/, "")
    .replace(/\n+$/, "");
}

const readClean = (file) => clean(fs.readFileSync(file, "utf8"));

// A licence file's name: LICENSE, LICENCE, COPYING or UNLICENSE, with any
// suffix (LICENSE.md, LICENSE-MIT, LICENSE.txt); NOTICE separately.
const LICENCE_NAME = /^(licen[cs]e|copying|unlicense)([._-].*)?$/i;
const NOTICE_NAME = /^notice([._-].*)?$/i;

function filesIn(dir, re) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isFile() && re.test(e.name))
    .map((e) => e.name)
    .sort(byCode);
}

// ---- 1. npm packages in the bundle ---------------------------------------

function bundledPackages() {
  // Outside the tree: a source map written under node_modules names its
  // sources by paths that no longer say node_modules.
  const out = fs.mkdtempSync(path.join(os.tmpdir(), "agentry-notices-"));
  const dirs = new Set();
  try {
    execFileSync("npx", ["vite", "build", "--sourcemap", "--outDir", out, "--emptyOutDir", "--logLevel", "warn"], {
      cwd: frontend,
      stdio: ["ignore", "ignore", "inherit"],
    });
    const stack = [out];
    while (stack.length) {
      const d = stack.pop();
      for (const e of fs.readdirSync(d, { withFileTypes: true })) {
        const p = path.join(d, e.name);
        if (e.isDirectory()) stack.push(p);
        else if (e.name.endsWith(".map")) {
          for (const src of JSON.parse(fs.readFileSync(p, "utf8")).sources ?? []) {
            const abs = path.resolve(d, src).split(path.sep).join("/");
            const first = abs.indexOf("/node_modules/");
            if (first < 0) continue;
            // The package is the last node_modules/<name> in the path, so a
            // package nested under another is its own entry.
            const tail = abs.slice(first);
            let rel = "";
            const re = /\/node_modules\/((?:@[^/]+\/)?[^/]+)\//g;
            let m;
            while ((m = re.exec(tail)) !== null) rel = tail.slice(0, m.index + m[0].length - 1);
            if (rel) dirs.add(abs.slice(0, first) + rel);
          }
        }
      }
    }
  } finally {
    fs.rmSync(out, { recursive: true, force: true });
  }
  const pkgs = [];
  for (const dir of [...dirs].sort(byCode)) {
    const pj = path.join(dir, "package.json");
    if (!fs.existsSync(pj)) fail(`${dir} is in the bundle but has no package.json`);
    const meta = JSON.parse(fs.readFileSync(pj, "utf8"));
    const lic = typeof meta.license === "string" ? meta.license : meta.license?.type ?? "";
    const files = filesIn(dir, LICENCE_NAME);
    if (files.length === 0) fail(`${meta.name}@${meta.version} is compiled into the bundle and has no licence file`);
    pkgs.push({
      name: meta.name,
      version: meta.version,
      spdx: lic,
      text: files.map((f) => readClean(path.join(dir, f))).join("\n\n"),
      notices: filesIn(dir, NOTICE_NAME).map((f) => readClean(path.join(dir, f))),
    });
  }
  return pkgs.sort((a, b) => byCode(a.name + "@" + a.version, b.name + "@" + b.version));
}

// ---- 2. Go modules --------------------------------------------------------

function goEnv(t) {
  return {
    ...process.env,
    GOWORK: "off",
    GOFLAGS: "-mod=mod",
    GOOS: t.goos,
    GOARCH: t.goarch,
    CGO_ENABLED: t.cgo,
  };
}

function goModules() {
  const mods = new Map(); // path@version -> {path, version, dir}
  let windowsLoaderEmbeds = [];
  for (const t of GO_TARGETS) {
    // -e: the package main embeds frontend/dist, which a tree that has not
    // built the bundle lacks; the dependency list does not depend on it.
    const listing = execFileSync(
      "go",
      ["list", "-e", "-deps", "-tags", t.tags, "-f", "{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}\t{{.Module.Main}}{{end}}", "."],
      { cwd: root, env: goEnv(t), encoding: "utf8", stdio: ["ignore", "pipe", "inherit"], maxBuffer: 1 << 26 },
    );
    for (const line of listing.split("\n")) {
      if (!line) continue;
      const [p, version, dir, main] = line.split("\t");
      if (main === "true") continue;
      if (!dir) fail(`module ${p} ${version} is not in the module cache (run go mod download)`);
      mods.set(`${p}@${version}`, { path: p, version, dir });
    }
    if (t.goos === "windows") {
      windowsLoaderEmbeds = execFileSync(
        "go",
        ["list", "-e", "-tags", t.tags, "-f", "{{range .EmbedFiles}}{{.}}\n{{end}}", "github.com/wailsapp/go-webview2/webviewloader"],
        { cwd: root, env: goEnv(t), encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] },
      )
        .split("\n")
        .filter(Boolean);
    }
  }
  // The Windows build must not embed Microsoft's loader DLL: section 5 says it
  // does not. A build that does needs that section rewritten first.
  if (windowsLoaderEmbeds.length > 0) {
    fail(`the Windows build now embeds ${windowsLoaderEmbeds.join(", ")} from go-webview2; rewrite the WebView2 section of this script to say so`);
  }
  const out = [];
  for (const m of [...mods.values()].sort((a, b) => byCode(a.path + "@" + a.version, b.path + "@" + b.version))) {
    const files = filesIn(m.dir, LICENCE_NAME);
    if (files.length === 0) fail(`module ${m.path} ${m.version} is compiled in and has no licence file at ${m.dir}`);
    out.push({
      name: m.path,
      version: m.version,
      text: files.map((f) => readClean(path.join(m.dir, f))).join("\n\n"),
      notices: filesIn(m.dir, NOTICE_NAME).map((f) => readClean(path.join(m.dir, f))),
    });
  }
  // The standard library and runtime are in every Go binary. Its licence has
  // no version in this file: the text is the same across releases.
  const goroot = execFileSync("go", ["env", "GOROOT"], { cwd: root, env: { ...process.env, GOWORK: "off" }, encoding: "utf8" }).trim();
  out.push({ name: "The Go standard library and runtime", version: "", text: readClean(path.join(goroot, "LICENSE")), notices: [] });
  return out;
}

// ---- rendering ------------------------------------------------------------

// group: entries sharing one licence text become one block, so the Apache
// text is printed once for the modules that carry it unmodified.
function groups(entries) {
  const m = new Map();
  for (const e of entries) {
    const key = createHash("sha256").update(e.text).digest("hex");
    if (!m.has(key)) m.set(key, { text: e.text, items: [] });
    m.get(key).items.push(e);
  }
  const list = [...m.values()];
  for (const g of list) g.items.sort((a, b) => byCode(a.name + a.version, b.name + b.version));
  return list.sort((a, b) => byCode(a.items[0].name + a.items[0].version, b.items[0].name + b.items[0].version));
}

const label = (e) => (e.version ? `${e.name} ${e.version}` : e.name) + (e.spdx ? ` (${e.spdx})` : "");

function renderGroups(entries) {
  const out = [];
  for (const g of groups(entries)) {
    out.push(SUB, ...g.items.map(label), "", g.text, "");
    for (const e of g.items) {
      for (const n of e.notices) out.push(`NOTICE of ${e.name}${e.version ? " " + e.version : ""}:`, "", n, "");
    }
  }
  return out;
}

function heading(n, title) {
  return [RULE, `${n}. ${title}`, RULE, ""];
}

function fontSection() {
  const lic = path.join(frontend, "src", "reportFonts", "licenses");
  const have = fs.readdirSync(lic).sort(byCode);
  const want = Object.keys(REPORT_FONT_LICENCES).sort(byCode);
  if (have.join("\n") !== want.join("\n")) {
    fail(`frontend/src/reportFonts/licenses holds ${JSON.stringify(have)} but this script names ${JSON.stringify(want)}: update REPORT_FONT_LICENCES`);
  }
  const entries = [
    { name: "IBM Plex Sans", version: "", text: readClean(path.join(frontend, "src", "fonts", "ibmplexsans.OFL.txt")), notices: [] },
    { name: "IBM Plex Mono", version: "", text: readClean(path.join(frontend, "src", "fonts", "ibmplexmono.OFL.txt")), notices: [] },
    ...Object.entries(REPORT_FONT_LICENCES).map(([f, fam]) => ({
      name: fam,
      version: "",
      text: readClean(path.join(lic, f)),
      notices: [],
    })),
  ];
  // The same family is named once per licence text: the interface's IBM Plex
  // licences and the report copies of them are the same text, so they share
  // an entry.
  const out = [
    "The app's interface uses IBM Plex Sans and IBM Plex Mono, and the Report design page",
    "previews a report's fonts with the families below. Every font is licensed under the",
    "SIL Open Font License, Version 1.1, whose text is given with each copyright",
    "notice. The fonts are embedded as the unmodified font files their authors publish.",
    "",
  ];
  const keyed = new Map();
  for (const e of entries) {
    const key = createHash("sha256").update(e.text).digest("hex");
    if (!keyed.has(key)) keyed.set(key, { text: e.text, names: [] });
    keyed.get(key).names.push(e.name);
  }
  const list = [...keyed.values()];
  for (const g of list) g.names = [...new Set(g.names)].sort(byCode);
  list.sort((a, b) => byCode(a.names[0], b.names[0]));
  for (const g of list) out.push(SUB, ...g.names, "", g.text, "");
  return out;
}

function logoSection() {
  const text = readClean(path.join(frontend, "src", "providerLogos-license", "lobe-icons.MIT.txt"));
  return [
    "The provider logos on the model setup tiles are the icon paths of @lobehub/icons-static-svg",
    "1.95.1, copied unchanged, under the licence below. The logos themselves are the trademarks of",
    "the companies they stand for and are shown only to say which provider a tile means.",
    "",
    SUB,
    "@lobehub/icons-static-svg 1.95.1 (MIT)",
    "",
    text,
    "",
  ];
}

// The Microsoft SDK's licence, the BSD-3-Clause-style text that accompanies
// WebView2Loader.dll in the Microsoft.Web.WebView2 package.
const MS_SDK_LICENCE = `Copyright (C) Microsoft Corporation. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * The name of Microsoft Corporation, or the names of its contributors
may not be used to endorse or promote products derived from this
software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.`;

function webview2Section() {
  return [
    "On Windows the app's window is drawn by the Microsoft Edge WebView2 Runtime.",
    "",
    "WebView2 Runtime and its installer",
    "",
    "The Windows installer carries Microsoft's WebView2 Runtime Bootstrapper,",
    "MicrosoftEdgeWebview2Setup.exe, unmodified and with Microsoft's signature intact,",
    "and runs it silently only when the computer has no WebView2 Runtime. The",
    "Bootstrapper downloads the Runtime from Microsoft; the installer does not contain",
    "the Runtime itself. Neither the Runtime nor the Bootstrapper is open-source",
    "software: both are Microsoft's, and the Runtime is licensed to you by Microsoft",
    "under the Microsoft Software License Terms for the Microsoft Edge WebView2",
    "Runtime. To read those terms, open",
    "https://developer.microsoft.com/microsoft-edge/webview2 and choose any download",
    "button: the terms appear in a dialog.",
    "",
    "How Microsoft's terms apply to this app:",
    "  - The Bootstrapper is distributed only with this application, which adds its own",
    "    primary functionality, and only as Microsoft built it.",
    "  - The Runtime may collect diagnostic data and send it to Microsoft, as",
    "    described at https://learn.microsoft.com/microsoft-edge/webview2/concepts/data-privacy",
    "    and in the Microsoft Privacy Statement, https://aka.ms/privacy. The Windows",
    "    \"Diagnostic data\" setting governs the optional part.",
    "  - Agentry Desktop starts the Runtime with its SmartScreen protection feature",
    "    turned off, which is the window library's default. Microsoft's terms require",
    "    a notice to users only when SmartScreen is left on.",
    "  - The Runtime updates itself from Microsoft. Agentry Desktop runs on Windows 10",
    "    and later, so Microsoft's notice for Windows 7 and 8.1 does not apply.",
    "  - Microsoft's names and logos are not used to suggest that Microsoft makes or",
    "    endorses Agentry Desktop.",
    "Distribution guidance:",
    "https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution",
    "",
    "WebView2 loader",
    "",
    "The Windows build loads WebView2 through the loader written in Go that comes with",
    "github.com/wailsapp/go-webview2 (ISC licence, listed in section 2). It does not",
    "contain Microsoft's WebView2Loader.dll. The go-webview2 module also holds that DLL",
    "for builds made with the native_webview2loader build tag, which the Windows",
    "packages do not use. For a build that does carry the DLL, Microsoft's licence for",
    "it, from the Microsoft.Web.WebView2 package, is:",
    "",
    SUB,
    "Microsoft.Web.WebView2 (WebView2Loader.dll)",
    "",
    MS_SDK_LICENCE,
    "",
  ];
}

function systemSection() {
  return [
    "The Linux packages (.deb and .rpm) do not contain GTK, glib, libsoup, WebKitGTK or",
    "JavaScriptCore. They declare them as dependencies, and the package manager installs",
    "them from the distribution under their own licences. On macOS the app uses the",
    "system WebKit. Those libraries are not part of the files this notice ships with.",
    "",
  ];
}

function build() {
  const npm = bundledPackages();
  const gomods = goModules();
  const fonts = fontSection();
  const lines = [
    "Third-party notices for Agentry Desktop",
    "",
    "Agentry Desktop is licensed under the MIT licence (the LICENSE file of the source",
    "repository). It contains the third-party software and assets listed here, each under",
    "its own licence, whose text and notices follow. This file is generated from the",
    "dependencies of the build; do not edit it by hand.",
    "",
    "Contents",
    "",
    `  1. Interface packages (${npm.length} npm packages compiled into the app's window)`,
    `  2. Go modules (${gomods.length - 1} modules and the Go standard library)`,
    "  3. Fonts",
    "  4. Provider logos",
    "  5. Microsoft WebView2 (Windows)",
    "  6. System libraries (Linux and macOS)",
    "",
    ...heading(1, "Interface packages"),
    `${npm.length} npm packages are compiled into the interface bundle. Their licence`,
    "comments are not kept in the built files, so their licences are given here.",
    "",
    ...renderGroups(npm),
    ...heading(2, "Go modules"),
    `${gomods.length - 1} modules are compiled into the application, with the Go standard`,
    "library and runtime. Modules that share one licence text are listed together.",
    "",
    ...renderGroups(gomods),
    ...heading(3, "Fonts"),
    ...fonts,
    ...heading(4, "Provider logos"),
    ...logoSection(),
    ...heading(5, "Microsoft WebView2 (Windows)"),
    ...webview2Section(),
    ...heading(6, "System libraries (Linux and macOS)"),
    ...systemSection(),
  ];
  return lines.join("\n").replace(/\n+$/, "") + "\n";
}

const text = build();
if (process.argv.includes("--check")) {
  const have = fs.existsSync(outFile) ? fs.readFileSync(outFile, "utf8") : "";
  if (have !== text) {
    fail("THIRD_PARTY_NOTICES is stale: run `make notices` and commit the result");
  }
  console.log("notices: THIRD_PARTY_NOTICES is current");
} else {
  fs.writeFileSync(outFile, text);
  console.log(`notices: wrote ${path.relative(root, outFile)} (${text.length} bytes)`);
}
