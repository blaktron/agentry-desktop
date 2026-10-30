// Writes a copy of a template with the product facts of wails.json in place
// of its placeholders: the resource strings of the Windows exe, its manifest
// and the macOS bundle's Info.plist.
//
//   node scripts/build/stamp.mjs <template> <output>
//
// {{.Info.ProductName}}, {{.Info.ProductVersion}}, {{.Info.CompanyName}},
// {{.Info.Copyright}}, {{.Info.Comments}} and {{.OutputFilename}} are the only
// placeholders; any other {{ left in a template is an error, not output.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

export function facts(path = "wails.json") {
  const j = JSON.parse(readFileSync(path, "utf8"));
  return {
    "{{.Info.ProductName}}": j.info.productName,
    "{{.Info.ProductVersion}}": j.info.productVersion,
    "{{.Info.CompanyName}}": j.info.companyName,
    "{{.Info.Copyright}}": j.info.copyright,
    "{{.Info.Comments}}": j.info.comments,
    "{{.OutputFilename}}": j.outputfilename,
  };
}

export function stamp(text, values) {
  let out = text;
  for (const [k, v] of Object.entries(values)) {
    if (typeof v !== "string" || v === "") throw new Error(`wails.json has no value for ${k}`);
    out = out.split(k).join(v);
  }
  const left = out.match(/\{\{[^}]*\}\}/);
  if (left) throw new Error(`the template has a placeholder this script does not know: ${left[0]}`);
  return out;
}

if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop())) {
  const [template, output] = process.argv.slice(2);
  if (!template || !output) {
    console.error("usage: node scripts/build/stamp.mjs <template> <output>");
    process.exit(2);
  }
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, stamp(readFileSync(template, "utf8"), facts()));
}
