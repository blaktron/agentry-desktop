// Stands in for @wailsio/runtime under node (tests/loader.mjs resolves the
// package here), so the generated bindings run against a test's own mocks:
// globalThis.go.<package>.<Type>.<Method> answers a bound call, and the
// events a test fires reach EventsOn through globalThis.runtime. The two
// helpers the generated models use are the package's own.
// @ts-expect-error TS7016: the package ships no declaration beside dist/cancellable.js
import { CancellablePromise } from "../node_modules/@wailsio/runtime/dist/cancellable.js";
// @ts-expect-error TS7016: the package ships no declaration beside dist/create.js
import * as Create from "../node_modules/@wailsio/runtime/dist/create.js";

export { CancellablePromise, Create };

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;

export const Call = {
  RuntimeError: class RuntimeError extends Error {},
  // A bound call by name: "<module path>/internal/app.RunAPI.SetupStatus"
  // is globalThis.go.app.RunAPI.SetupStatus.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ByName(name: string, ...args: any[]): Promise<any> {
    const [type, method] = name.split(".").slice(-2);
    const pkg = name.slice(0, name.lastIndexOf(".", name.lastIndexOf(".") - 1)).split("/").pop() as string;
    const fn = g.go?.[pkg]?.[type]?.[method];
    if (typeof fn !== "function") return Promise.reject(new Error(`the test has no mock for ${name}`));
    return Promise.resolve().then(() => fn(...args));
  },
};

type Listener = (event: { name: string; data: unknown }) => void;
const listeners: Record<string, Listener[]> = {};

export const Events = {
  On(name: string, cb: Listener): () => void {
    (listeners[name] ??= []).push(cb);
    return () => { listeners[name] = (listeners[name] ?? []).filter((c) => c !== cb); };
  },
};

/** Fires an event as Go would, to every listener of its name. */
export function emit(name: string, data: unknown): void {
  for (const cb of listeners[name] ?? []) cb({ name, data });
}

export const Clipboard = { async SetText(_text: string): Promise<void> {} };
export const Browser = { async OpenURL(_url: string): Promise<void> {} };
