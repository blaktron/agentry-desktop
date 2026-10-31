// Module hooks for the tests that render a page: a .ts or .tsx file is
// compiled with the transformer the bundler already brings (oxc, through
// rolldown; the bindings hold enums, which node's own type stripping refuses),
// an import that names no extension is tried as .ts, .tsx or a folder's
// index, one that names a .js the bundler would have made is its .ts
// source, a stylesheet import is an empty module, and "@wailsio/runtime" is
// tests/wails-mock.ts, whose bound calls are answered by the test's own
// mocks on globalThis.go. The pure-module tests load through the same
// hooks: their modules are compiled here rather than stripped by node.
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { transformSync } from "rolldown/experimental";

const here = path.dirname(fileURLToPath(import.meta.url));
const mockURL = pathToFileURL(path.join(here, "wails-mock.ts")).href;

export async function resolve(specifier, context, next) {
  if (specifier === "@wailsio/runtime" && context.parentURL !== mockURL) {
    return { url: mockURL, shortCircuit: true };
  }
  try {
    return await next(specifier, context);
  } catch (e) {
    if (e?.code !== "ERR_MODULE_NOT_FOUND" || !/^\.{1,2}\//.test(specifier)) throw e;
    // The generated bindings import one another as "./models.js", the name
    // the bundler gives the compiled file; here the source is the .ts.
    if (specifier.endsWith(".js")) return next(specifier.slice(0, -3) + ".ts", context);
    if (/\.[a-z]+$/.test(specifier)) throw e;
    for (const suffix of [".ts", ".tsx", "/index.ts", "/index.tsx"]) {
      try {
        return await next(specifier + suffix, context);
      } catch (e2) {
        if (e2?.code !== "ERR_MODULE_NOT_FOUND") throw e2;
      }
    }
    throw e;
  }
}

export async function load(url, context, next) {
  // Every TypeScript file outside node_modules, the generated bindings
  // included, whose enums node's own type stripping refuses.
  if (/\.tsx?$/.test(url) && !url.includes("/node_modules/")) {
    const file = fileURLToPath(url);
    const { code, errors } = transformSync(file, readFileSync(file, "utf8"), { jsx: { runtime: "automatic" } });
    if (errors.length > 0) throw new Error(`${file}: ${errors.map((e) => e.message).join("\n")}`);
    return { format: "module", source: code, shortCircuit: true };
  }
  if (url.endsWith(".css")) return { format: "module", source: "export default {};", shortCircuit: true };
  return next(url, context);
}
