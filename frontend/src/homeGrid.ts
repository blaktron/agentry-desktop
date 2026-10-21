// Home's grid: how big each card is, where a dragged card lands, and what
// size a dragged corner snaps to. Pure, so the node tests can import it.
//
// The grid is four columns of equal width and rows of one fixed height. A
// card spans 1 to 4 columns and 1 to 3 rows; a card the person never sized
// takes the default. Go keeps the order and the sizes (HomeAPI.Move,
// HomeAPI.Resize); the page only works out what to ask for.

export const GRID_COLS = 4;
export const GRID_ROWS = 3;

export type CardSize = { cols: number; rows: number };

// Each card's size until the person changes it: two columns and two rows,
// room for what each of today's cards holds on a window of the app's
// default width.
const DEFAULT_SIZE: CardSize = { cols: 2, rows: 2 };

const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));

/** A card's size: the stored one, or the default where none is stored;
 *  either way within the grid. */
export function cardSize(cols?: number, rows?: number): CardSize {
  const d = DEFAULT_SIZE;
  return {
    cols: clamp(cols && cols > 0 ? cols : d.cols, 1, GRID_COLS),
    rows: clamp(rows && rows > 0 ? rows : d.rows, 1, GRID_ROWS),
  };
}

/** The columns a card spans on a grid of only `columns` (a narrow window):
 *  never wider than the grid, while its stored size stays as it is. */
export function spanOn(cols: number, columns: number): number {
  return clamp(cols, 1, Math.max(1, columns));
}

/** The order after the card at drawn place `from` is dropped on the card at
 *  drawn place `to`: it takes that card's place, the others keep their
 *  order. Go's move does the same over the stored list, so the page's
 *  preview is what Go then answers with. */
export function moved<T>(items: readonly T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= items.length || to >= items.length) return [...items];
  const out = [...items];
  const [item] = out.splice(from, 1);
  out.splice(to, 0, item);
  return out;
}

/** The size a corner dragged by (dx, dy) pixels snaps to, from a card of
 *  `start`, on a grid whose column and row are `col` and `row` pixels with
 *  `gap` between: the nearest whole cell, within the grid and never wider
 *  than the columns the window shows. */
export function snappedSize(start: CardSize, dx: number, dy: number, cell: { col: number; row: number; gap: number; columns: number }): CardSize {
  const width = start.cols * cell.col + (start.cols - 1) * cell.gap + dx;
  const height = start.rows * cell.row + (start.rows - 1) * cell.gap + dy;
  return {
    cols: clamp(Math.round((width + cell.gap) / (cell.col + cell.gap)), 1, Math.min(GRID_COLS, Math.max(1, cell.columns))),
    rows: clamp(Math.round((height + cell.gap) / (cell.row + cell.gap)), 1, GRID_ROWS),
  };
}

/** The size an arrow key on a card's resize handle asks for. */
export function sizeByKey(size: CardSize, key: string): CardSize | null {
  const by: Record<string, [number, number]> = { ArrowRight: [1, 0], ArrowLeft: [-1, 0], ArrowDown: [0, 1], ArrowUp: [0, -1] };
  const d = by[key];
  if (!d) return null;
  const next = { cols: clamp(size.cols + d[0], 1, GRID_COLS), rows: clamp(size.rows + d[1], 1, GRID_ROWS) };
  return next.cols === size.cols && next.rows === size.rows ? null : next;
}

/** The drawn place an arrow key on a card's move handle asks for: one
 *  earlier or one later, or none at either end. */
export function placeByKey(at: number, count: number, key: string): number | null {
  const step = key === "ArrowLeft" || key === "ArrowUp" ? -1 : key === "ArrowRight" || key === "ArrowDown" ? 1 : 0;
  const to = at + step;
  return step === 0 || to < 0 || to >= count ? null : to;
}
