// Stands in for @wailsio/runtime in the harness (harness/vite.config.ts
// aliases the package to this file), so the generated bindings run in a plain
// browser against the mocks each entry sets up: window.go.<package>.<Type>.<Method>
// answers a bound call, and window.runtime carries the events and the
// clipboard. Not shipped.
// The package ships its declarations apart from dist/, so the two helpers the
// generated models use are imported from dist and typed as what they are.
// @ts-expect-error TS7016: no declaration file beside dist/cancellable.js
import { CancellablePromise } from "../node_modules/@wailsio/runtime/dist/cancellable.js";
// @ts-expect-error TS7016: no declaration file beside dist/create.js
import * as Create from "../node_modules/@wailsio/runtime/dist/create.js";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const w = window as any;

export { CancellablePromise, Create };

// A bound call by name: "<module path>/internal/app.RunAPI.SetupStatus" is
// window.go.app.RunAPI.SetupStatus.
export const Call = {
  RuntimeError: class RuntimeError extends Error {},
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ByName(name: string, ...args: any[]): Promise<any> {
    const [path, type, method] = [name.slice(0, name.lastIndexOf(".", name.lastIndexOf(".") - 1)), ...name.split(".").slice(-2)];
    const pkg = path.split("/").pop() as string;
    const fn = w.go?.[pkg]?.[type]?.[method];
    if (typeof fn !== "function") return Promise.reject(new Error(`the harness has no mock for ${name}`));
    return Promise.resolve().then(() => fn(...args));
  },
};

export const Events = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  On(name: string, cb: (event: { name: string; data: any }) => void): () => void {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const off = w.runtime?.EventsOn?.(name, (data: any) => cb({ name, data }));
    return typeof off === "function" ? off : () => {};
  },
};

export const Clipboard = {
  async SetText(text: string): Promise<void> {
    await w.runtime?.ClipboardSetText?.(text);
  },
};

export const Browser = {
  async OpenURL(url: string): Promise<void> {
    await w.runtime?.BrowserOpenURL?.(url);
  },
};
