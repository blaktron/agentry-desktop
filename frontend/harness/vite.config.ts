import { defineConfig } from "vite";

// The harness runs the generated bindings against mocks: the toolkit's
// runtime package is replaced by runtime-mock.ts (see there).
export default defineConfig({
  resolve: {
    alias: { "@wailsio/runtime": new URL("./runtime-mock.ts", import.meta.url).pathname },
  },
});
