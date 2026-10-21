import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import { HOME_CARDS, type DrawnCard } from "../../homeCards";
import { cardSize, GRID_COLS, moved, placeByKey, sizeByKey, snappedSize, spanOn, type CardSize } from "../../homeGrid";

// Home's cards on a grid the person arranges. A card is dragged by its frame
// (or its move handle) onto another card's place, and the others make room
// while it moves; its corner handle drags its size, snapping to whole cells.
// Both handles also take the arrow keys. Nothing is kept here: each drop or
// resize is one change Go makes (onMove, onResize), and the page draws what
// Go answers. Until it answers, the grid shows what was asked.

// How far the pointer travels before a press on a card becomes a drag, so a
// click still opens the card.
const DRAG_START = 6;

// Inside a card, what a press on does its own thing rather than drag it.
const OWN_PRESS = ".build-way-part, input, textarea, select, a, [contenteditable], .home-resize";

type Drag = {
  key: string;
  pointer: number;
  // Where on the card it was taken, and where the pointer is now.
  grabX: number; grabY: number;
  x: number; y: number;
  // The card last swapped with, which the pointer must leave before it can
  // be swapped with again (cards of different sizes would swap back and
  // forth otherwise).
  last: string | null;
  started: boolean;
  startX: number; startY: number;
};

export function HomeGrid({ cards, render, onMove, onResize, busy }: {
  cards: DrawnCard[];
  render: (c: DrawnCard) => ReactNode;
  // index: the layout's places, hidden cards counted (DrawnCard.index).
  onMove: (from: number, kind: string, to: number) => Promise<void>;
  onResize: (index: number, kind: string, size: CardSize) => Promise<void>;
  // A change is with Go: the handles wait.
  busy?: boolean;
}) {
  const grid = useRef<HTMLDivElement>(null);
  // The order and sizes shown while a drag or a change is in hand; null
  // shows the layout's.
  const [order, setOrder] = useState<string[] | null>(null);
  const [sizing, setSizing] = useState<{ key: string; size: CardSize } | null>(null);
  const [dragKey, setDragKey] = useState<string | null>(null);
  const drag = useRef<Drag | null>(null);
  // A drag ends in a click on the card it was taken by; that click opens
  // nothing.
  const swallowClick = useRef(false);
  // How many columns the window shows: four, or fewer when it is narrow.
  const [columns, setColumns] = useState(GRID_COLS);

  useEffect(() => {
    const el = grid.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const count = () => setColumns(getComputedStyle(el).gridTemplateColumns.split(" ").filter(Boolean).length || GRID_COLS);
    count();
    const ro = new ResizeObserver(count);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const byKey = new Map(cards.map((c) => [c.key, c]));
  const shown = (order ?? cards.map((c) => c.key)).map((k) => byKey.get(k)).filter((c): c is DrawnCard => !!c);

  const cell = (key: string) => grid.current?.querySelector<HTMLElement>(`[data-home-key="${CSS.escape(key)}"]`) ?? null;

  // The dragged card follows the pointer from wherever the grid now puts it.
  const follow = () => {
    const d = drag.current;
    const el = d && cell(d.key);
    if (!d || !el) return;
    el.style.transform = "";
    const r = el.getBoundingClientRect();
    el.style.transform = `translate(${d.x - d.grabX - r.left}px, ${d.y - d.grabY - r.top}px)`;
  };
  useLayoutEffect(() => { if (drag.current?.started) follow(); });

  const press = (c: DrawnCard) => (e: ReactPointerEvent<HTMLDivElement>) => {
    if (busy || e.button !== 0 || cards.length < 2) return;
    const t = e.target as HTMLElement;
    if (!t.closest(".home-grip") && t.closest(OWN_PRESS)) return;
    const r = e.currentTarget.getBoundingClientRect();
    drag.current = {
      key: c.key, pointer: e.pointerId, grabX: e.clientX - r.left, grabY: e.clientY - r.top,
      x: e.clientX, y: e.clientY, last: null, started: false, startX: e.clientX, startY: e.clientY,
    };
  };

  const move = (e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || d.pointer !== e.pointerId) return;
    d.x = e.clientX;
    d.y = e.clientY;
    if (!d.started) {
      if (Math.hypot(d.x - d.startX, d.y - d.startY) < DRAG_START) return;
      d.started = true;
      e.currentTarget.setPointerCapture(e.pointerId);
      setDragKey(d.key);
      setOrder(cards.map((c) => c.key));
    }
    // The card under the pointer, the dragged one aside.
    const over = shown.find((c) => {
      if (c.key === d.key) return false;
      const r = cell(c.key)?.getBoundingClientRect();
      return !!r && d.x >= r.left && d.x <= r.right && d.y >= r.top && d.y <= r.bottom;
    });
    if (!over) d.last = null;
    else if (over.key !== d.last) {
      d.last = over.key;
      const keys = shown.map((c) => c.key);
      setOrder(moved(keys, keys.indexOf(d.key), keys.indexOf(over.key)));
    }
    follow();
  };

  const drop = (e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || d.pointer !== e.pointerId) return;
    drag.current = null;
    const el = cell(d.key);
    if (el) el.style.transform = "";
    if (!d.started) return;
    swallowClick.current = true;
    setDragKey(null);
    const from = cards.findIndex((c) => c.key === d.key);
    const to = (order ?? []).indexOf(d.key);
    if (from < 0 || to < 0 || from === to) { setOrder(null); return; }
    // Go moves the card into the place of the card drawn there, the same
    // move the grid has just shown.
    void onMove(cards[from].index, cards[from].kind, cards[to].index).finally(() => setOrder(null));
  };

  const keyMove = (c: DrawnCard, at: number) => (e: KeyboardEvent) => {
    const to = placeByKey(at, cards.length, e.key);
    if (to === null) return;
    e.preventDefault();
    if (busy) return;
    setOrder(moved(cards.map((x) => x.key), at, to));
    void onMove(c.index, c.kind, cards[to].index).finally(() => {
      setOrder(null);
      // Keep the keyboard on the card that moved.
      requestAnimationFrame(() => cell(c.key)?.querySelector<HTMLElement>(".home-grip")?.focus());
    });
  };

  const resizePress = (c: DrawnCard, size: CardSize) => (e: ReactPointerEvent<HTMLButtonElement>) => {
    if (busy || e.button !== 0) return;
    e.stopPropagation();
    const g = grid.current;
    if (!g) return;
    const cs = getComputedStyle(g);
    const gap = parseFloat(cs.columnGap) || 0;
    const col = (g.clientWidth - gap * (columns - 1)) / columns;
    const row = parseFloat(cs.gridAutoRows) || 160;
    const handle = e.currentTarget;
    handle.setPointerCapture(e.pointerId);
    const startX = e.clientX, startY = e.clientY;
    let now = size;
    const onMovePtr = (ev: PointerEvent) => {
      now = snappedSize(size, ev.clientX - startX, ev.clientY - startY, { col, row, gap, columns });
      setSizing({ key: c.key, size: now });
    };
    const onUp = () => {
      handle.removeEventListener("pointermove", onMovePtr);
      handle.removeEventListener("pointerup", onUp);
      handle.removeEventListener("pointercancel", onUp);
      if (now.cols === size.cols && now.rows === size.rows) { setSizing(null); return; }
      void onResize(c.index, c.kind, now).finally(() => setSizing(null));
    };
    handle.addEventListener("pointermove", onMovePtr);
    handle.addEventListener("pointerup", onUp);
    handle.addEventListener("pointercancel", onUp);
  };

  const keyResize = (c: DrawnCard, size: CardSize) => (e: KeyboardEvent) => {
    const next = sizeByKey(size, e.key);
    if (!next && !e.key.startsWith("Arrow")) return;
    e.preventDefault();
    if (!next || busy) return;
    setSizing({ key: c.key, size: next });
    void onResize(c.index, c.kind, next).finally(() => setSizing(null));
  };

  return (
    <div ref={grid} className={`home-grid${dragKey ? " is-arranging" : ""}`} data-home-columns={columns}>
      {shown.map((c) => {
        const stored = cardSize(c.cols, c.rows);
        const size = sizing?.key === c.key ? sizing.size : stored;
        const title = HOME_CARDS[c.kind].title;
        const at = cards.findIndex((x) => x.key === c.key);
        return (
          <div key={c.key} className={`home-card-cell${dragKey === c.key ? " is-dragged" : ""}${sizing?.key === c.key ? " is-sizing" : ""}`}
            data-home-card={c.kind} data-home-key={c.key} data-cols={size.cols} data-rows={size.rows}
            style={{ gridColumn: `span ${spanOn(size.cols, columns)}`, gridRow: `span ${size.rows}` }}
            onPointerDown={press(c)} onPointerMove={move} onPointerUp={drop} onPointerCancel={drop}
            onClickCapture={(e) => { if (swallowClick.current) { swallowClick.current = false; e.preventDefault(); e.stopPropagation(); } }}>
            {render(c)}
            {cards.length > 1 && (
              <button type="button" className="home-grip" disabled={busy} data-home-grip={c.kind}
                aria-label={`Move ${title}: drag it, or use the arrow keys`} title="Drag to move, or use the arrow keys"
                onKeyDown={keyMove(c, at)}>
                <span aria-hidden="true">⠿</span>
              </button>
            )}
            <button type="button" className="home-resize" disabled={busy} data-home-resize={c.kind}
              aria-label={`Resize ${title}, now ${size.cols} wide and ${size.rows} tall: drag the corner, or use the arrow keys`}
              title="Drag to resize, or use the arrow keys"
              onPointerDown={resizePress(c, stored)} onKeyDown={keyResize(c, stored)}>
              <span aria-hidden="true" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
