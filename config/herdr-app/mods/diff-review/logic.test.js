// Run: node --test config/herdr-app/mods/diff-review/logic.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  parseDiff, addedFileRows, commentKey, buildDisplay, moveCursor,
  formatMessage, pickTargets, sendBlocker, storeKey,
} from "./logic.js";

const DIFF = [
  "diff --git a/src/a.js b/src/a.js",
  "index 111..222 100644",
  "--- a/src/a.js",
  "+++ b/src/a.js",
  "@@ -1,3 +1,4 @@ function a() {",
  " const x = 1;",
  "-const y = 2;",
  "+const y = 3;",
  "+const z = 4;",
  " return x;",
  "@@ -10,2 +11,2 @@",
  " a",
  "-b",
  "+c",
  "\\ No newline at end of file",
  "diff --git a/new.txt b/new.txt",
  "new file mode 100644",
  "index 000..333",
  "--- /dev/null",
  "+++ b/new.txt",
  "@@ -0,0 +1 @@",
  "+hello",
  "diff --git a/gone.txt b/gone.txt",
  "deleted file mode 100644",
  "--- a/gone.txt",
  "+++ /dev/null",
  "@@ -1 +0,0 @@",
  "-bye",
  "diff --git a/img.png b/img.png",
  "Binary files a/img.png and b/img.png differ",
  "",
].join("\n");

test("parseDiff splits files with status and paths", () => {
  const files = parseDiff(DIFF);
  assert.deepEqual(files.map((f) => [f.path, f.status, f.binary]), [
    ["src/a.js", "M", false], ["new.txt", "A", false], ["gone.txt", "D", false], ["img.png", "M", true],
  ]);
});

test("parseDiff numbers old and new lines per hunk", () => {
  const rows = parseDiff(DIFF)[0].rows;
  assert.deepEqual(rows.map((r) => [r.kind, r.old ?? null, r.new ?? null, r.text]), [
    ["@", null, null, "@@ -1,3 +1,4 @@ function a() {"],
    [" ", 1, 1, "const x = 1;"],
    ["-", 2, null, "const y = 2;"],
    ["+", null, 2, "const y = 3;"],
    ["+", null, 3, "const z = 4;"],
    [" ", 3, 4, "return x;"],
    ["@", null, null, "@@ -10,2 +11,2 @@"],
    [" ", 10, 11, "a"],
    ["-", 11, null, "b"],
    ["+", null, 12, "c"],
    ["\\", null, null, "\\ No newline at end of file"],
  ]);
});

test("parseDiff handles one-line hunks without counts", () => {
  const [, added, deleted] = parseDiff(DIFF);
  assert.deepEqual(added.rows.at(-1), { kind: "+", new: 1, text: "hello" });
  assert.deepEqual(deleted.rows.at(-1), { kind: "-", old: 1, text: "bye" });
});

test("parseDiff reads renames", () => {
  const files = parseDiff("diff --git a/x b/y\nsimilarity index 90%\nrename from x\nrename to y\n");
  assert.equal(files[0].path, "y");
  assert.equal(files[0].oldPath, "x");
  assert.equal(files[0].status, "R");
});

test("parseDiff expands tabs so they render", () => {
  const rows = parseDiff("diff --git a/t b/t\n--- a/t\n+++ b/t\n@@ -1 +1 @@\n-\tx\n+ab\ty\n")[0].rows;
  assert.equal(rows[1].text, "    x");
  assert.equal(rows[2].text, "ab  y");
});

test("addedFileRows numbers an untracked file as all added", () => {
  assert.deepEqual(addedFileRows("one\ntwo\n"), [
    { kind: "@", text: "@@ -0,0 +1,2 @@" },
    { kind: "+", new: 1, text: "one" },
    { kind: "+", new: 2, text: "two" },
  ]);
});

test("commentKey identifies a line by side and number; hunks have none", () => {
  assert.equal(commentKey("a.js", { kind: "+", new: 3 }), "a.js:new:3");
  assert.equal(commentKey("a.js", { kind: " ", old: 2, new: 3 }), "a.js:new:3");
  assert.equal(commentKey("a.js", { kind: "-", old: 2 }), "a.js:old:2");
  assert.equal(commentKey("a.js", { kind: "@" }), null);
});

test("buildDisplay puts comments and the editor under their line", () => {
  const rows = parseDiff(DIFF)[0].rows;
  const comments = [{ key: "src/a.js:new:2", text: "why 3?" }, { key: "src/a.js:old:11", text: "keep b" }];
  const items = buildDisplay(rows, comments, "src/a.js:new:4", "src/a.js");
  const shape = items.map((i) => i.type === "line" ? `L${i.index}` : i.type === "comment" ? `C:${i.comment.text}` : "E");
  assert.deepEqual(shape, ["L0", "L1", "L2", "L3", "C:why 3?", "L4", "L5", "E", "L6", "L7", "L8", "C:keep b", "L9", "L10"]);
  assert.ok(items.every((i) => i.size >= 24), "every row is at least 24 px (Fitts)");
});

test("moveCursor skips hunk headers and clamps at the ends", () => {
  const rows = parseDiff(DIFF)[0].rows;
  assert.equal(moveCursor(rows, -1, 1), 1);
  assert.equal(moveCursor(rows, 5, 1), 7);
  assert.equal(moveCursor(rows, 7, -1), 5);
  assert.equal(moveCursor(rows, 9, 1), 9);
  assert.equal(moveCursor(rows, 1, -1), 1);
  assert.equal(moveCursor([{ kind: "@" }], -1, 1), -1);
});

test("formatMessage lists comments in file and line order with the code", () => {
  const msg = formatMessage({
    repo: "aval", branch: "main",
    comments: [
      { path: "b.js", side: "new", line: 9, code: "  return 1;", text: "use a const" },
      { path: "a.js", side: "old", line: 4, code: "old()", text: "why removed?" },
      { path: "a.js", side: "new", line: 2, code: "x()", text: "rename" },
    ],
  });
  assert.equal(msg, [
    "Review comments on aval@main (from Herdr diff-review):",
    "",
    "1. a.js:2 — rename",
    "   > x()",
    "2. a.js:4 (removed line) — why removed?",
    "   > old()",
    "3. b.js:9 — use a const",
    "   > return 1;",
  ].join("\n"));
});

test("formatMessage indents multi-line comments under their number", () => {
  const msg = formatMessage({ repo: "r", branch: "b", comments: [{ path: "a", side: "new", line: 1, code: "c", text: "one\ntwo" }] });
  assert.ok(msg.endsWith("1. a:1 — one\n   two\n   > c"));
});

const agent = (address, tab, status, space = "s1") => ({ address, tab, space, status, program: { name: "Claude Code" }, tab_name: tab });

test("pickTargets prefers the mod's own tab, then idle/done, then blocked, then working", () => {
  const all = [agent("t1", "tabA", "working"), agent("t2", "tabA", "blocked"), agent("t3", "tabA", "idle"), agent("t4", "tabB", "idle"), agent("t9", "x", "idle", "s2")];
  const r = pickTargets(all, { space: "s1", tab: "tabA" });
  assert.deepEqual(r.candidates.map((a) => a.address), ["t3", "t2", "t1"]);
  assert.equal(r.preferred, "t3");
});

test("pickTargets falls back to the space when the mod's tab has no agent", () => {
  const all = [agent("t4", "tabB", "working"), agent("t5", "tabC", "done"), agent("t9", "x", "idle", "s2")];
  const r = pickTargets(all, { space: "s1", tab: "mine" });
  assert.deepEqual(r.candidates.map((a) => a.address), ["t5", "t4"]);
  assert.equal(r.preferred, "t5");
});

test("pickTargets keeps a still-valid chosen target", () => {
  const all = [agent("t3", "tabA", "idle"), agent("t2", "tabA", "blocked")];
  assert.equal(pickTargets(all, { space: "s1", tab: "tabA", chosen: "t2" }).preferred, "t2");
  assert.equal(pickTargets(all, { space: "s1", tab: "tabA", chosen: "gone" }).preferred, "t3");
});

test("pickTargets with no agent has no preferred target", () => {
  assert.deepEqual(pickTargets([], { space: "s1", tab: "t" }), { candidates: [], preferred: null });
});

test("sendBlocker refuses working and blocked agents, allows idle and done", () => {
  assert.match(sendBlocker(agent("t1", "a", "working")), /working/);
  assert.match(sendBlocker(agent("t1", "a", "blocked")), /waiting/);
  assert.equal(sendBlocker(agent("t1", "a", "idle")), null);
  assert.equal(sendBlocker(agent("t1", "a", "done")), null);
  assert.match(sendBlocker(null), /No agent/);
});

test("storeKey is per machine, repo and branch", () => {
  assert.equal(storeKey(null, "/r", "main"), "diff-review::/r@main");
  assert.equal(storeKey("vps", "/r", "dev"), "diff-review:vps:/r@dev");
});
