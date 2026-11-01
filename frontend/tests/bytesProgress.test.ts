// How far a pull or a download has got, read from the lines the tools
// write: the docker command's per-layer lines and the app's own download
// lines.
import { test } from "node:test";
import assert from "node:assert/strict";
import { bytesProgress, bytesShare, bytesWords, parseSize, updateStageWords } from "../src/bytesProgress.ts";

test("docker's sizes read in bytes", () => {
  assert.equal(parseSize("12.5MB"), 12_500_000);
  assert.equal(parseSize("45.6 kB"), 45_600);
  assert.equal(parseSize("1.2GB"), 1_200_000_000);
  assert.equal(parseSize("3MiB"), 3 * 1024 * 1024);
  assert.equal(parseSize("512B"), 512);
  assert.equal(parseSize("lots"), null);
});

test("a pull's progress is the sum over its layers, each at its newest line, complete layers whole", () => {
  const lines = [
    "latest: Pulling from library/alpine",
    "a1b2c3d4e5f6: Pulling fs layer",
    "ffffff000000: Pulling fs layer",
    "a1b2c3d4e5f6: Downloading [=====>                 ]  10MB/40MB",
    "ffffff000000: Downloading [==>                    ]  5MB/60MB",
    "a1b2c3d4e5f6: Downloading [==========>            ]  20MB/40MB",
    "ffffff000000: Download complete",
    "ffffff000000: Extracting [=====>                ]  12MB/60MB",
  ];
  assert.deepEqual(bytesProgress(lines), { done: 80_000_000, total: 100_000_000 });
  assert.equal(bytesWords(bytesProgress(lines)!), "80 of 100 MB");
  assert.equal(bytesShare(bytesProgress(lines)!), 0.8);
  // Layers already there count whole only once their size is known.
  assert.equal(bytesProgress(["a1b2c3d4e5f6: Already exists", "latest: Pulling from x"]), null);
});

test("the app's own download line is the progress, the newest one winning", () => {
  const lines = ["Downloading colima-Darwin-arm64…", "Downloading colima-Darwin-arm64: 4 of 40 MB", "Downloading colima-Darwin-arm64: 12 of 40 MB"];
  assert.deepEqual(bytesProgress(lines), { done: 12_000_000, total: 40_000_000 });
  assert.equal(bytesWords({ done: 12_000_000, total: 0 }), "12 MB so far");
  assert.equal(bytesShare({ done: 12_000_000, total: 0 }), 0);
  assert.equal(bytesProgress(["Installing…", "Verified."]), null);
  assert.equal(bytesProgress([]), null);
});

test("an update's stage has its words", () => {
  assert.equal(updateStageWords("checking"), "Checking the release…");
  assert.equal(updateStageWords("downloading"), "Downloading…");
  assert.equal(updateStageWords("installing"), "Installing…");
  assert.equal(updateStageWords(undefined), "Installing…");
});
