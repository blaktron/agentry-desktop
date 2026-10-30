// Rewrites the product facts in the Windows installer's include file from
// wails.json, so the installer, the exe's resource and the bundle's
// Info.plist name one version. Everything else in the file is the include
// file's own (the install and uninstall macros project.nsi calls).
//
//   node scripts/build/installer-tools.mjs build/windows/installer/wails_tools.nsh
import { readFileSync, writeFileSync } from "node:fs";
import { facts } from "./stamp.mjs";

const HEADER = "# DO NOT EDIT the product facts below: scripts/build/installer-tools.mjs writes them from wails.json\n";

/** Returns text with each INFO_ default set to the value of wails.json. */
export function tools(text, values) {
  const defaults = {
    INFO_PROJECTNAME: values["{{.OutputFilename}}"],
    INFO_COMPANYNAME: values["{{.Info.CompanyName}}"],
    INFO_PRODUCTNAME: values["{{.Info.ProductName}}"],
    INFO_PRODUCTVERSION: values["{{.Info.ProductVersion}}"],
    INFO_COPYRIGHT: values["{{.Info.Copyright}}"],
  };
  let out = text.replace(/^# DO NOT EDIT[^\n]*\n/, HEADER);
  for (const [name, value] of Object.entries(defaults)) {
    if (!value) throw new Error(`wails.json has no value for ${name}`);
    const re = new RegExp(`(!define ${name} )"[^"\\n]*"`);
    if (!re.test(out)) throw new Error(`the include file has no default for ${name}`);
    out = out.replace(re, `$1"${value.replaceAll("$", "$$$$")}"`);
  }
  return out;
}

if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop())) {
  const file = process.argv[2];
  if (!file) {
    console.error("usage: node scripts/build/installer-tools.mjs <wails_tools.nsh>");
    process.exit(2);
  }
  const before = readFileSync(file, "utf8");
  const after = tools(before, facts());
  if (after !== before) writeFileSync(file, after);
}
