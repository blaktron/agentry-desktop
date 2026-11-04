import { CardFrame } from "../../cards";
import { HOME_CARDS, type ShortcutKind } from "../../homeCards";
import { SHORTCUT_FIGURES } from "./ShortcutFigures";

// A shortcut on Home: one cell, its figure and its title, the whole card one
// button that opens the page where the thing is done. It starts nothing.
export function ShortcutCard({ kind, onOpen }: { kind: ShortcutKind; onOpen: () => void }) {
  const Figure = SHORTCUT_FIGURES[kind];
  return <CardFrame title={HOME_CARDS[kind].title} onOpen={onOpen} figure={<Figure />} className="home-card home-shortcut" />;
}
