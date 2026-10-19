#!/usr/bin/env node
// Holds every font file the app embeds to the upstream file it claims to be.
//
//   node scripts/fonts/check.mjs             offline: the embedded files against PROVENANCE.json
//   node scripts/fonts/check.mjs --upstream  also fetches each recorded source and compares
//
// (make fonts-check, part of make check.) The app ships each font exactly as
// its authors publish it: a subset or a conversion is a modified version of
// the font, which the SIL Open Font License restricts for families that
// reserve their name. So each directory of fonts under frontend/src has a
// PROVENANCE.json naming, for every file, the upstream URL at a pinned
// commit, the font's own version string and the SHA-256 of the upstream file.
// The check fails when
//   - an embedded file's SHA-256 differs from the recorded one,
//   - a font file under frontend (.ttf .otf .woff .woff2) is not recorded,
//   - a recorded file is missing,
//   - a recorded source is not under the pinned commit of the recorded
//     repository, or an entry lacks a field,
//   - the directory's stylesheet names a file that is not recorded, or a
//     recorded file no @font-face uses.
// With --upstream the sources are downloaded as well and must hash to the
// recorded value; that needs the network, so it is run by hand when a font
// is added or its commit moves, not by make check.
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const frontend = path.join(root, "frontend");

// Each directory of embedded fonts and the stylesheet that declares them.
const SETS = [
  { dir: "src/fonts", css: "src/fonts.css" },
  { dir: "src/reportFonts", css: "src/reportFonts/fonts.css" },
];
const FONT_EXT = /\.(ttf|otf|woff2?|eot)$/i;

const problems = [];
const bad = (msg) => problems.push(msg);
const sha256 = (buf) => createHash("sha256").update(buf).digest("hex");

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (e.name === "node_modules" || e.name === "dist" || e.name === "shots") continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else out.push(p);
  }
  return out;
}

const recorded = new Map(); // absolute path -> entry
for (const set of SETS) {
  const dir = path.join(frontend, set.dir);
  const label = `frontend/${set.dir}`;
  let prov;
  try {
    prov = JSON.parse(fs.readFileSync(path.join(dir, "PROVENANCE.json"), "utf8"));
  } catch (e) {
    bad(`${label}/PROVENANCE.json: ${e.message}`);
    continue;
  }
  if (!/^[0-9a-f]{40}$/.test(prov.commit ?? "")) bad(`${label}/PROVENANCE.json: commit is not a 40-character commit id`);
  if (!/^https:\/\/github\.com\/[\w.-]+\/[\w.-]+$/.test(prov.repository ?? "")) bad(`${label}/PROVENANCE.json: repository is not a GitHub repository URL`);
  const rawPrefix = `${(prov.repository ?? "").replace("https://github.com/", "https://raw.githubusercontent.com/")}/${prov.commit}/`;
  for (const f of prov.files ?? []) {
    const where = `${label}/${f.file}`;
    for (const k of ["family", "file", "version", "source", "sha256"]) {
      if (typeof f[k] !== "string" || f[k] === "") bad(`${where}: PROVENANCE.json has no ${k}`);
    }
    if (typeof f.file !== "string" || typeof f.sha256 !== "string") continue;
    if (!/^[0-9a-f]{64}$/.test(f.sha256)) bad(`${where}: sha256 is not 64 hex digits`);
    if (typeof f.source === "string" && !f.source.startsWith(rawPrefix)) bad(`${where}: source is not under ${rawPrefix}`);
    const abs = path.join(dir, f.file);
    recorded.set(abs, { ...f, set, label });
    if (!fs.existsSync(abs)) {
      bad(`${where}: recorded but missing`);
      continue;
    }
    const got = sha256(fs.readFileSync(abs));
    if (got !== f.sha256) bad(`${where}: SHA-256 is ${got}, the upstream file's is ${f.sha256}`);
  }

  // The stylesheet and the provenance must name the same files.
  const cssPath = path.join(frontend, set.css);
  const css = fs.readFileSync(cssPath, "utf8");
  const used = new Set();
  for (const m of css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g)) {
    const abs = path.resolve(path.dirname(cssPath), m[1]);
    used.add(abs);
    if (!recorded.has(abs) || recorded.get(abs).set !== set) bad(`${set.css}: url(${m[1]}) names a file not recorded in ${label}/PROVENANCE.json`);
  }
  for (const [abs, e] of recorded) {
    if (e.set === set && !used.has(abs)) bad(`${e.label}/${e.file}: recorded but no @font-face in ${set.css} uses it`);
  }
}

for (const abs of walk(frontend)) {
  if (FONT_EXT.test(abs) && !recorded.has(abs)) bad(`${path.relative(root, abs)}: a font file with no entry in a PROVENANCE.json`);
}

if (process.argv.includes("--upstream")) {
  for (const [, e] of recorded) {
    const res = await fetch(e.source);
    if (!res.ok) {
      bad(`${e.label}/${e.file}: ${e.source} answered ${res.status}`);
      continue;
    }
    const got = sha256(Buffer.from(await res.arrayBuffer()));
    if (got !== e.sha256) bad(`${e.label}/${e.file}: ${e.source} hashes to ${got}, not the recorded ${e.sha256}`);
  }
}

if (problems.length > 0) {
  console.error(problems.join("\n"));
  console.error(`fonts-check: ${problems.length} problem${problems.length === 1 ? "" : "s"}`);
  process.exit(1);
}
console.log(`fonts-check: ${recorded.size} embedded font files match their recorded upstream hashes${process.argv.includes("--upstream") ? " and their upstream sources" : ""}`);
