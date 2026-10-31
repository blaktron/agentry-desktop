// Registers the module hooks in tests/loader.mjs for a test run (`node
// --import ./tests/register.mjs`): the pages are .tsx, which node's own
// type stripping does not read, and the generated bindings call the window
// toolkit's runtime, which a test replaces with tests/wails-mock.ts.
import { register } from "node:module";

register("./loader.mjs", import.meta.url);
