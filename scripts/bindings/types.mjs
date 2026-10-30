// Run after the bindings are generated (make bindings). The generator types
// every bound method that returns a pointer as "T | null", because a Go
// pointer can be nil. Most of this app's methods return a pointer to a view
// they always fill when they report no error: a failure is the rejected
// promise, never a null. For those the signature drops "| null" and the one
// place that carries the nullable conversion asserts the value is there. The
// methods in NULLABLE can return nil without an error (a No answer, nothing
// to report), so they keep the generator's type and their callers handle the
// null. The list is the bound methods whose Go body returns a nil pointer
// with no error; TestNullableBindingsAreListed in internal/app keeps it
// complete.
//
//   node scripts/bindings/types.mjs [bindings dir]
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export const NULLABLE = new Set([
  "buildapi.Install",
  "buildapi.ReviewLocally",
  "runapi.InFlight",
  "scheduleapi.HostedOffer",
  "scheduleapi.InstallCLI",
]);

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else if (name.endsWith(".ts") && name !== "models.ts" && name !== "index.ts") yield p;
  }
}

/** Returns the source of one service file with its non-nullable returns narrowed. */
export function narrow(service, src) {
  // A function runs from "export function" to the "}" that starts a line.
  return src.replace(/export function (\w+)\(([^\n]*)\): \$CancellablePromise<([^\n]*?) \| null> \{\n([\s\S]*?)\n\}\n/g,
    (whole, name, params, type, body) => {
      if (NULLABLE.has(`${service}.${name}`)) return whole;
      const asserted = body.replace(/return (\$\$createType\d+\(\$result\));/g, "return $1!;");
      return `export function ${name}(${params}): $CancellablePromise<${type}> {\n${asserted}\n}\n`;
    });
}

if (process.argv[1] && import.meta.url.endsWith(process.argv[1].split("/").pop())) {
  const dir = process.argv[2] ?? "frontend/bindings";
  let n = 0;
  for (const file of walk(dir)) {
    const service = file.split("/").pop().replace(/\.ts$/, "");
    const before = readFileSync(file, "utf8");
    const after = narrow(service, before);
    if (after !== before) { writeFileSync(file, after); n++; }
  }
  console.log(`bindings: ${n} service files narrowed`);
}
