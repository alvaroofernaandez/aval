// Pure logic for diff-review. No herdr imports, so `node --test` loads it as is.

/** Row heights in px at zoom 1. Every clickable row is ≥ 24 px (Fitts). */
export const SIZES = { line: 24, comment: 32, editor: 44 };

// Text draws a tab as nothing: pad each to the next 4-column stop (as `changes` does).
const untab = (text) => text.replace(/[^\t]*\t/g, (s) => s.slice(0, -1).padEnd((s.length - 1) - (s.length - 1) % 4 + 4));

const stripPrefix = (p) => p.replace(/^[ab]\//, "");

/**
 * Parses `git diff` output into files, each with rows numbered on both sides:
 * {kind: "@" | "+" | "-" | " " | "\\", old?, new?, text}.
 */
export function parseDiff(text) {
  const files = [];
  let file = null, old = 0, next = 0, inHunk = false;
  for (const line of text.split("\n")) {
    if (line.startsWith("diff --git ")) {
      const m = /^diff --git a\/(.*) b\/(.*)$/.exec(line);
      file = { path: m ? m[2] : line.slice(11), oldPath: m ? m[1] : null, status: "M", binary: false, rows: [] };
      files.push(file); inHunk = false;
      continue;
    }
    if (!file) continue;
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
    if (hunk) { old = +hunk[1]; next = +hunk[2]; inHunk = true; file.rows.push({ kind: "@", text: line }); continue; }
    if (!inHunk) {
      if (line.startsWith("new file mode")) file.status = "A";
      else if (line.startsWith("deleted file mode")) file.status = "D";
      else if (line.startsWith("rename from ")) { file.oldPath = line.slice(12); file.status = "R"; }
      else if (line.startsWith("rename to ")) file.path = line.slice(10);
      else if (line.startsWith("Binary files ") || line === "GIT binary patch") file.binary = true;
      else if (line.startsWith("+++ ") && line !== "+++ /dev/null") file.path = stripPrefix(line.slice(4));
      continue;
    }
    const kind = line[0], body = untab(line.slice(1));
    if (kind === "+") file.rows.push({ kind, new: next++, text: body });
    else if (kind === "-") file.rows.push({ kind, old: old++, text: body });
    else if (kind === " ") file.rows.push({ kind, old: old++, new: next++, text: body });
    else if (kind === "\\") file.rows.push({ kind, text: line });
  }
  return files;
}

/** An untracked file shown as one all-added hunk. */
export function addedFileRows(text) {
  const lines = text.replace(/\n$/, "").split("\n");
  return [{ kind: "@", text: `@@ -0,0 +1,${lines.length} @@` }, ...lines.map((l, i) => ({ kind: "+", new: i + 1, text: untab(l) }))];
}

const commentable = (row) => row && (row.kind === "+" || row.kind === "-" || row.kind === " ");

/** A line's identity: removed lines by their old number, the rest by their new one. */
export function commentKey(path, row) {
  if (!commentable(row)) return null;
  return row.kind === "-" ? `${path}:old:${row.old}` : `${path}:new:${row.new}`;
}

/** The rows a virtual list draws: diff lines, each followed by its comments and the open editor. */
export function buildDisplay(rows, comments, editingKey, path) {
  const byKey = new Map();
  for (const c of comments) byKey.set(c.key, [...(byKey.get(c.key) ?? []), c]);
  const items = [];
  rows.forEach((row, index) => {
    items.push({ type: "line", index, size: SIZES.line });
    const key = commentKey(path, row);
    if (!key) return;
    for (const comment of byKey.get(key) ?? []) items.push({ type: "comment", index, comment, size: SIZES.comment });
    if (key === editingKey) items.push({ type: "editor", index, size: SIZES.editor });
  });
  return items;
}

/** The next commentable row from `index` in `delta`'s direction; stays put at the ends. */
export function moveCursor(rows, index, delta) {
  for (let i = index + delta; i >= 0 && i < rows.length; i += delta) if (commentable(rows[i])) return i;
  if (commentable(rows[index])) return index;
  return rows.findIndex(commentable);
}

/** The one message pasted into the agent. */
export function formatMessage({ repo, branch, comments }) {
  const sorted = [...comments].sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : a.line - b.line);
  const items = sorted.map((c, i) => {
    const where = `${c.path}:${c.line}${c.side === "old" ? " (removed line)" : ""}`;
    const text = c.text.trim().split("\n").join("\n   ");
    return `${i + 1}. ${where} — ${text}\n   > ${c.code.trim()}`;
  });
  return [`Review comments on ${repo}@${branch} (from Herdr diff-review):`, "", ...items].join("\n");
}

const RANK = { idle: 0, done: 0, blocked: 1, working: 2 };

/**
 * Agents the comments can go to: those in the mod's own tab, else those in its space,
 * ready ones first. `preferred` keeps a still-present `chosen` address.
 */
export function pickTargets(agents, { space, tab, chosen = null }) {
  const inTab = agents.filter((a) => a.space === space && a.tab === tab);
  const pool = inTab.length ? inTab : agents.filter((a) => a.space === space);
  const candidates = [...pool].sort((a, b) => (RANK[a.status] ?? 3) - (RANK[b.status] ?? 3));
  const preferred = candidates.some((a) => a.address === chosen) ? chosen : candidates[0]?.address ?? null;
  return { candidates, preferred };
}

/** Why the comments can't go to this agent now, or null. */
export function sendBlocker(agent) {
  if (!agent) return "No agent in this space";
  if (agent.status === "working") return `${agent.program?.name ?? agent.address} is working; send when it stops`;
  if (agent.status === "blocked") return `${agent.program?.name ?? agent.address} is waiting on a prompt; answer it first`;
  return null;
}

/** Shared-state key for one repo and branch on one machine. */
export function storeKey(machine, root, branch) {
  return `diff-review:${machine ?? ""}:${root}@${branch}`;
}
