// Home's grid: each card's size, where a dragged card lands, and what a
// dragged corner or an arrow key asks for.
import { test } from "node:test";
import assert from "node:assert/strict";
import { cardSize, moved, placeByKey, sizeByKey, snappedSize, spanOn } from "../src/homeGrid.ts";

test("a card the person never sized is two by two, a sized one keeps its own", () => {
  assert.deepEqual(cardSize(), { cols: 2, rows: 2 });
  assert.deepEqual(cardSize(1, 3), { cols: 1, rows: 3 });
});

test("a stored size outside the grid is drawn within it", () => {
  assert.deepEqual(cardSize(9, 9), { cols: 4, rows: 3 });
  assert.deepEqual(cardSize(-1, 0), { cols: 2, rows: 2 });
});

test("on a narrow window a wide card spans what there is, and its size is left alone", () => {
  assert.equal(spanOn(4, 4), 4);
  assert.equal(spanOn(4, 2), 2);
  assert.equal(spanOn(3, 1), 1);
  assert.equal(spanOn(1, 2), 1);
});

test("a dropped card takes the place of the card it is dropped on, the others keep their order", () => {
  const keys = ["a", "b", "c", "d"];
  assert.deepEqual(moved(keys, 0, 2), ["b", "c", "a", "d"]);
  assert.deepEqual(moved(keys, 3, 1), ["a", "d", "b", "c"]);
  assert.deepEqual(moved(keys, 1, 1), keys);
  assert.deepEqual(moved(keys, 1, 9), keys);
  assert.notEqual(moved(keys, 1, 1), keys, "a copy, never the list it was given");
});

test("a dragged corner snaps to the nearest whole cell, within the grid and the columns shown", () => {
  const cell = { col: 200, row: 172, gap: 16, columns: 4 };
  const start = { cols: 2, rows: 2 };
  assert.deepEqual(snappedSize(start, 0, 0, cell), start);
  // Past half a column and half a row: one more of each.
  assert.deepEqual(snappedSize(start, 120, 100, cell), { cols: 3, rows: 3 });
  // Short of half: unchanged.
  assert.deepEqual(snappedSize(start, 90, 80, cell), start);
  assert.deepEqual(snappedSize(start, -500, -500, cell), { cols: 1, rows: 1 });
  assert.deepEqual(snappedSize(start, 2000, 2000, cell), { cols: 4, rows: 3 });
  assert.deepEqual(snappedSize(start, 2000, 0, { ...cell, columns: 2 }), { cols: 2, rows: 2 });
});

test("the resize handle's arrow keys grow or shrink a card by one cell, and stop at the grid's edge", () => {
  assert.deepEqual(sizeByKey({ cols: 2, rows: 2 }, "ArrowRight"), { cols: 3, rows: 2 });
  assert.deepEqual(sizeByKey({ cols: 2, rows: 2 }, "ArrowUp"), { cols: 2, rows: 1 });
  assert.equal(sizeByKey({ cols: 4, rows: 1 }, "ArrowRight"), null);
  assert.equal(sizeByKey({ cols: 1, rows: 1 }, "ArrowUp"), null);
  assert.equal(sizeByKey({ cols: 2, rows: 2 }, "Enter"), null);
});

test("the move handle's arrow keys move a card one place, and not past either end", () => {
  assert.equal(placeByKey(1, 4, "ArrowLeft"), 0);
  assert.equal(placeByKey(1, 4, "ArrowDown"), 2);
  assert.equal(placeByKey(0, 4, "ArrowUp"), null);
  assert.equal(placeByKey(3, 4, "ArrowRight"), null);
  assert.equal(placeByKey(1, 4, "Tab"), null);
});
