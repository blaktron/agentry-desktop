// The themes Settings → Home offers, and putting one on the window. Pure
// but for the root element and the storage it is handed, so the node tests
// can import it.
//
// Go keeps the choice (HomeAPI.Theme, SetTheme; desktop.yaml's theme key).
// This window also keeps a copy in its own storage, which index.html reads
// before the first paint, so a restart opens in the chosen theme rather
// than flashing the default one first. Go's answer wins when it comes.

export type ThemeId = "system" | "light" | "dark" | "slate" | "forest" | "midnight" | "contrast";

// swatch: the theme's page, card, accent and ink, light or as it is always
// shown, for its tile in Settings.
export type ThemeChoice = { id: ThemeId; title: string; line: string; swatch: [string, string, string, string] };

// The order Settings → Home lists them in; Go's settings.Themes is the same
// list, which internal/designtokens holds to themes.css.
export const THEMES: ThemeChoice[] = [
  { id: "system", title: "System", line: "Light or dark as your computer is set.", swatch: ["#faf6ee", "#16110d", "#0e7a63", "#231b12"] },
  { id: "light", title: "Light", line: "Cream paper and brown-black ink, always.", swatch: ["#faf6ee", "#f5efe4", "#0e7a63", "#231b12"] },
  { id: "dark", title: "Dark", line: "The same palette on a dark ground, always.", swatch: ["#16110d", "#201914", "#4fd1b0", "#ece4d8"] },
  { id: "slate", title: "Slate", line: "Cool blue-grey, light or dark as your computer is set.", swatch: ["#f7f8fa", "#eef1f5", "#1957c2", "#161b22"] },
  { id: "forest", title: "Forest", line: "A green tint, light or dark as your computer is set.", swatch: ["#f4f7f2", "#ecf2ea", "#2d662d", "#17201a"] },
  { id: "midnight", title: "Midnight", line: "Deep navy with a teal accent, always dark.", swatch: ["#0b1220", "#101a2e", "#3fd5c0", "#e6edf7"] },
  { id: "contrast", title: "High contrast", line: "Black and white grounds with stronger lines and text, light or dark as your computer is set.", swatch: ["#ffffff", "#f2f2f2", "#00574a", "#000000"] },
];

/** Where this window keeps its copy of the theme. index.html reads the
 *  same key. */
export const THEME_KEY = "agentry.theme";

export const isTheme = (v: unknown): v is ThemeId => typeof v === "string" && THEMES.some((t) => t.id === v);

type Root = { dataset: Record<string, string | undefined> };
type Store = { setItem(k: string, v: string): void };

/** Puts the theme on the window: data-theme on the root element, none for
 *  System (the base palette), and the window's copy for the next start. A
 *  name this app lacks is System. */
export function applyTheme(id: string, root: Root, store?: Store): ThemeId {
  const theme: ThemeId = isTheme(id) ? id : "system";
  if (theme === "system") delete root.dataset.theme;
  else root.dataset.theme = theme;
  try { store?.setItem(THEME_KEY, theme); } catch { /* no storage: the next start shows System until Go answers */ }
  return theme;
}

/** The window's storage, or none where reading it throws. */
export function windowStore(): Store | undefined {
  try { return globalThis.localStorage; } catch { return undefined; }
}

/** The theme on the window now. */
export const themeOn = (root: Root): ThemeId => (isTheme(root.dataset.theme) ? root.dataset.theme : "system");
