// diff-review: comment on lines of the repo's diff (GitHub-style) and send the
// comments to the agent working in this space as one message.
// Pure logic lives in ./logic.js (tested with `node --test logic.test.js`).
import { View, div } from "gpui-kit";
import { v_flex, h_flex, InputState, Input, Scrollbar, v_virtual_list, VirtualListScrollHandle } from "gpui-base";
import { spaces, panes, focused, agents, exec, readTextFile, watchFolder, theme, settings, props, shared, setShared, notify, nextCommand, checking } from "herdr";
import { parseDiff, addedFileRows, commentKey, buildDisplay, moveCursor, formatMessage, pickTargets, sendBlocker, storeKey } from "./logic.js";

// Reads write nothing under .git (no index refresh or lock), as `changes` does.
const READ = ["--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "diff.autoRefreshIndex=false"];
const EMPTY_TREE = "4b825dc642cb6eb9a060e54bf8d69288fbee4904";
const MAX_ROWS = 3000;
// A comment row's click zones, px from the row's left edge (see commentRow / itemClick).
const ZONES = { indent: 104, edit: 48, del: 60, gap: 6 };
const basename = (p) => p.replace(/\/+$/, "").split("/").pop() || p;

export default class DiffReview extends View {
  init(_props, cx) {
    const f = focused();
    // A tab mod is focused when it opens: that tab and space are "ours".
    this.space = f?.space ?? spaces().find((s) => s.active)?.id ?? null;
    this.tab = f?.tab ?? null;
    this.files = []; this.rows = []; this.open = null; this.cursor = -1;
    this.editing = null; this.draft = ""; this.chosen = null; this.confirmClear = false;
    this.handle = cx.focus_handle();
    this.scroll = VirtualListScrollHandle.new();
    this.input = InputState.new({ placeholder: "Leave a comment" });
    this.input.on("change", (_e, _cx) => { this.draft = this.input.value(); });
    this.input.on("submit", (_e, cx) => this.save(cx));
    if (checking()) return;
    this.handle.focus();
    this.refresh(cx);
    cx.timer.every(10000, (cx) => this.refresh(cx));
    cx.spawn(async (cx) => {
      for (;;) {
        const c = await nextCommand();
        // Bindable from keys.json ("mod:diff-review" or global "diff-review.<command>").
        if (c === "focus") this.handle.focus();
        else if (c === "next" || c === "prev") this.move(c === "next" ? 1 : -1, cx);
        else if (c === "comment" && this.cursor >= 0) this.startEdit(this.cursor, null, cx);
        else if (c === "cancel" && this.editing) this.cancel(cx);
        else if (c === "send") this.send(cx);
        else if (c === "clear") { this.confirmClear = true; this.clear(cx); }
        cx.notify();
      }
    });
  }

  // Our own tab and space: a mod has no pane id in props, so find our pane among
  // the diff-review panes (the focused one, else the only one), else what was focused at init.
  where() {
    const mine = panes().filter((p) => p.mod === "diff-review"), f = focused();
    const pane = mine.find((p) => p.pane === f?.pane) ?? (mine.length === 1 ? mine[0] : null);
    const tab = pane?.tab ?? this.tab;
    const space = spaces().find((s) => s.tabs.some((t) => t.id === tab))?.id ?? this.space;
    return { tab, space };
  }

  // Where to look: props.folder, else a terminal of this space (agents first), else its folder.
  location() {
    const { tab, space: spaceId } = this.where();
    const space = spaces().find((s) => s.id === spaceId) ?? spaces().find((s) => s.active);
    const on = space?.machine ?? null;
    if (props()?.folder) return { cwd: props().folder, on };
    const tabs = new Set((space?.tabs ?? []).map((t) => t.id));
    const mine = panes().filter((p) => tabs.has(p.tab) && p.terminal && p.cwd);
    const pick = mine.find((p) => p.status && p.tab === tab) ?? mine.find((p) => p.status) ?? mine[0];
    return { cwd: pick?.cwd || space?.folder || null, on };
  }

  git(args) { return exec("git", [...READ, ...args], { cwd: this.root, on: this.on }); }

  watch(cx) {
    if (this.on || this.watched === this.root) return;
    const root = this.watched = this.root;
    cx.spawn(async (cx) => {
      try { while (this.watched === root) { await watchFolder(root); if (this.watched === root) this.refresh(cx); } }
      catch { this.watched = null; }
    });
  }

  refresh(cx) {
    if (this.loading) return;
    const { cwd, on } = this.location();
    const context = JSON.stringify({ cwd, on });
    if (context !== this.context) { this.context = context; this.root = null; this.files = []; this.open = null; this.rows = []; this.cursor = -1; this.editing = null; this.error = null; }
    if (!cwd) { this.error = "No folder for this space"; cx.notify(); return; }
    this.loading = true;
    cx.spawn(async (cx) => {
      try {
        const top = await exec("git", ["rev-parse", "--show-toplevel"], { cwd, on });
        if (top.code !== 0) { this.error = "Not a git repository"; this.files = []; return; }
        this.root = top.stdout.trim(); this.on = on;
        const [head, branch] = await Promise.all([this.git(["rev-parse", "--verify", "-q", "HEAD"]), this.git(["rev-parse", "--abbrev-ref", "HEAD"])]);
        this.base = head.code === 0 ? "HEAD" : EMPTY_TREE;
        this.branch = branch.code === 0 ? branch.stdout.trim() : "HEAD";
        const [names, stats, fresh] = await Promise.all([
          this.git(["diff", this.base, "--name-status", "-z", "--no-renames"]),
          this.git(["diff", this.base, "--numstat", "-z", "--no-renames"]),
          this.git(["ls-files", "--others", "--exclude-standard", "-z"]),
        ]);
        if (names.code !== 0) throw new Error(names.stderr.trim());
        const stat = {};
        for (const entry of stats.stdout.split("\0")) {
          const [a, d, ...rest] = entry.split("\t");
          if (rest.length) stat[rest.join("\t")] = { add: Number(a) || 0, del: Number(d) || 0 };
        }
        const parts = names.stdout.split("\0").filter(Boolean), files = [];
        for (let i = 0; i + 1 < parts.length; i += 2) files.push({ path: parts[i + 1], status: parts[i][0], ...(stat[parts[i + 1]] ?? { add: 0, del: 0 }) });
        for (const path of fresh.stdout.split("\0").filter(Boolean).slice(0, 500)) files.push({ path, status: "A", untracked: true, add: 0, del: 0 });
        files.sort((a, b) => a.path < b.path ? -1 : 1);
        this.files = files; this.error = null;
        this.watch(cx);
        if (this.open && !files.some((f) => f.path === this.open)) { this.open = null; this.rows = []; }
        if (!this.open && files.length) this.open = files[0].path;
        if (this.open) await this.load(this.open);
      } catch (e) { this.error = String(e?.message ?? e); }
      finally { this.loading = false; cx.notify(); }
    });
  }

  async load(path) {
    const file = this.files.find((f) => f.path === path);
    if (!file) return;
    let rows;
    if (file.untracked) {
      const text = await readTextFile(`${this.root}/${path}`, { on: this.on }).catch(() => null);
      rows = text === null ? [{ kind: "\\", text: "Cannot preview this file (binary, too large or unreadable)" }]
        : text.includes("\0") ? [{ kind: "\\", text: "Binary file" }] : addedFileRows(text);
      if (text && !text.includes("\0")) file.add = rows.length - 1;
    } else {
      const r = await this.git(["diff", this.base, "--no-ext-diff", "--no-textconv", "--no-color", "--", path]);
      const parsed = parseDiff(r.stdout)[0];
      rows = !parsed ? [{ kind: "\\", text: "No text diff" }] : parsed.binary ? [{ kind: "\\", text: "Binary file changed" }] : parsed.rows;
    }
    if (this.open !== path) return;
    this.rows = rows.slice(0, MAX_ROWS);
    this.truncated = rows.length > MAX_ROWS;
    if (this.cursor >= this.rows.length || this.cursor < 0) this.cursor = moveCursor(this.rows, -1, 1);
  }

  pick(path, cx) {
    if (this.open === path) return;
    this.open = path; this.rows = []; this.cursor = -1; this.editing = null;
    cx.spawn(async (cx) => { try { await this.load(path); } catch (e) { this.error = String(e); } cx.notify(); });
    cx.notify();
  }

  // Comments: shared state per machine + repo + branch, so they outlive the panel.
  key() { return this.root ? storeKey(this.on, this.root, this.branch) : null; }
  comments() { const k = this.key(); return (k && shared(k)) || []; }
  setComments(list) { const k = this.key(); if (k) setShared(k, list.length ? list : null); }

  startEdit(index, comment, cx) {
    const row = this.rows[index], key = commentKey(this.open, row);
    if (!key) return;
    this.cursor = index; this.confirmClear = false;
    this.editing = { key, id: comment?.id ?? null };
    this.draft = comment?.text ?? "";
    this.input.set_value(this.draft);
    this.input.focus();
    cx.notify();
  }

  cancel(cx) { this.editing = null; this.draft = ""; this.input.set_value(""); this.handle.focus(); cx.notify(); }

  save(cx) {
    const text = this.input.value().trim(), ed = this.editing;
    if (!ed) return;
    const list = this.comments();
    if (!text) { if (ed.id) this.setComments(list.filter((c) => c.id !== ed.id)); return this.cancel(cx); }
    if (ed.id) this.setComments(list.map((c) => c.id === ed.id ? { ...c, text } : c));
    else {
      const row = this.rows[this.cursor], side = row.kind === "-" ? "old" : "new";
      this.setComments([...list, { id: `${Date.now()}-${list.length}`, key: ed.key, path: this.open, side, line: side === "old" ? row.old : row.new, code: row.text, text }]);
    }
    this.cancel(cx);
  }

  remove(id, cx) { this.setComments(this.comments().filter((c) => c.id !== id)); cx.notify(); }

  clear(cx) {
    if (!this.confirmClear) { this.confirmClear = true; return cx.notify(); }
    this.confirmClear = false; this.setComments([]); this.editing = null; cx.notify();
  }

  targets() {
    return pickTargets(agents(), { ...this.where(), chosen: this.chosen });
  }

  send(cx) {
    const comments = this.comments();
    if (!comments.length || this.sending) return;
    const { candidates, preferred } = this.targets();
    const agent = candidates.find((a) => a.address === preferred) ?? null;
    const blocker = sendBlocker(agent);
    if (blocker) { notify({ title: "Comments not sent", body: blocker, level: "warning" }); return; }
    const message = formatMessage({ repo: basename(this.root), branch: this.branch, comments });
    const key = this.key(), name = agent.program?.name ?? agent.address;
    this.sending = true; cx.notify();
    cx.spawn(async (cx) => {
      try {
        // hc send waits up to 60 s for a half-typed draft in the agent, past exec's 30 s default.
        const r = await exec("hc", ["send", "--json", agent.address, message], { timeout: 90000 });
        if (r.code !== 0) throw new Error((r.stderr || r.stdout).trim());
        if (this.key() === key) this.setComments([]);
        notify({ title: `${comments.length} comment${comments.length === 1 ? "" : "s"} sent to ${name}`, body: `${basename(this.root)}@${this.branch} · ${agent.tab_name ?? ""}`.trim(), level: "success" });
      } catch (e) { notify({ title: "Comments not sent", body: String(e?.message ?? e), level: "error" }); }
      finally { this.sending = false; cx.notify(); }
    });
  }

  move(delta, cx) { this.cursor = moveCursor(this.rows, this.cursor, delta); this.scrollToCursor = true; cx.notify(); }

  onKey(e, cx) {
    const k = e.keystroke;
    if (k === "cmd-enter") { this.send(cx); cx.stop_propagation(); return; }
    if (this.editing) { if (k === "escape") { this.cancel(cx); cx.stop_propagation(); } return; }
    if (k === "j" || k === "down" || k === "k" || k === "up") {
      this.move(k === "j" || k === "down" ? 1 : -1, cx); cx.stop_propagation();
    } else if ((k === "c" || k === "enter") && this.cursor >= 0) { this.startEdit(this.cursor, null, cx); cx.stop_propagation(); }
    else if (k === "escape" && this.confirmClear) { this.confirmClear = false; cx.notify(); }
  }

  // --- view ------------------------------------------------------------------

  button(id, label, { primary = false, enabled = true, tip = null } = {}, action) {
    const t = theme();
    return h_flex().id(id).h(28).px(12).flex_none().items_center().rounded(7).text_size(12).font_medium()
      .border_1().border_color(primary && enabled ? t.solid : t.input)
      .bg(primary && enabled ? t.solid : t.raised).text_color(!enabled ? t.dim : primary ? t.solid_foreground : t.foreground)
      .when(enabled, (e) => e.cursor_pointer().hover((s) => s.opacity(0.88)).on_click((_e, cx) => action(cx)))
      .when(Boolean(tip), (e) => e.tooltip(tip)).child(label);
  }

  // Rows of a virtual list can't register handlers (they're rebuilt every frame):
  // the list's on_item_click routes clicks, and the content wrapper's mouse-down
  // records where in the row the press landed, for a comment's Edit / Delete.
  lineRow(item, t) {
    const row = this.rows[item.index], i = item.index;
    if (row.kind === "@" || row.kind === "\\") return div().w_full().h(item.size).px(12).flex().items_center().flex_none().truncate().bg(t.raised).text_color(t.dim).child(row.text);
    const num = (n) => div().w(40).flex_none().text_right().text_color(t.dim).child(n === undefined ? "" : String(n));
    const sign = row.kind === " " ? " " : row.kind === "-" ? "−" : "+";
    return h_flex().id(`l-${i}`).w_full().h(item.size).flex_none().items_center().gap(8).pr(12).cursor_pointer()
      .when(row.kind === "+", (e) => e.bg(t.ansi2 + "18")).when(row.kind === "-", (e) => e.bg(t.ansi1 + "18"))
      .when(this.cursor === i, (e) => e.bg(t.selected))
      .hover((s) => s.bg(t.hover))
      .child(num(row.old)).child(num(row.new))
      .child(div().w(12).flex_none().text_color(row.kind === "+" ? t.ansi2 : row.kind === "-" ? t.ansi1 : t.dim).child(sign))
      .child(div().flex_1().min_w(0).truncate().child(row.text || " "));
  }

  commentRow(item, t) {
    const c = item.comment, zone = (id, label, w) => h_flex().id(id).w(w).h(24).flex_none().justify_center().items_center().rounded(6)
      .border_1().border_color(t.border).text_size(11.5).text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground)).child(label);
    return h_flex().id(`c-${c.id}`).w_full().h(item.size).flex_none().items_center().pl(ZONES.indent).pr(12).gap(ZONES.gap).bg(t.surface).cursor_pointer()
      .font_family(settings().ui_font ?? "Geist").text_size(12.5)
      .child(zone(`ce-${c.id}`, "Edit", ZONES.edit))
      .child(zone(`cd-${c.id}`, "Delete", ZONES.del))
      .child(div().w(2).h(16).flex_none().rounded(1).bg(t.accent))
      .child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(c.text));
  }

  editorRow(item, t) {
    return h_flex().w_full().h(item.size).flex_none().items_center().pl(ZONES.indent).pr(12).gap(10).bg(t.surface)
      .font_family(settings().ui_font ?? "Geist")
      .child(div().flex_1().min_w(0).max_w(720).h(32).px(10).flex().items_center().rounded(7).border_1().border_color(t.accent).bg(t.background)
        .child(Input.new(this.input).h_full().text_size(12.5).text_color(t.foreground)))
      .child(div().flex_none().text_size(11.5).text_color(t.dim).child(this.editing?.id ? "Enter saves · empty deletes · Esc cancels" : "Enter saves · Esc cancels"));
  }

  itemClick(key, cx) {
    if (key.startsWith("l")) { this.startEdit(Number(key.slice(1)), null, cx); return; }
    if (!key.startsWith("c")) return;
    const id = key.slice(1), c = this.comments().find((x) => x.id === id);
    if (!c) return;
    const x = this.pressX ?? 0, edit0 = ZONES.indent, del0 = edit0 + ZONES.edit + ZONES.gap;
    if (x >= del0 && x < del0 + ZONES.del) return this.remove(id, cx);
    const index = this.rows.findIndex((r) => commentKey(this.open, r) === c.key);
    if (index >= 0) this.startEdit(index, c, cx);
  }

  render(cx) {
    if (!checking() && JSON.stringify(this.location()) !== this.context) this.refresh(cx);
    const t = theme(), mono = settings().font_family ?? "JetBrains Mono";
    const all = this.comments(), mine = all.filter((c) => c.path === this.open);
    const counts = {}; for (const c of all) counts[c.path] = (counts[c.path] ?? 0) + 1;
    const items = buildDisplay(this.rows, mine, this.editing?.key ?? null, this.open);
    if (this.scrollToCursor) {
      this.scrollToCursor = false;
      const at = items.findIndex((it) => it.type === "line" && it.index === this.cursor);
      if (at >= 0) this.scroll.scroll_to_item(at);
    }
    const { candidates, preferred } = this.targets();
    const target = candidates.find((a) => a.address === preferred);
    const blocker = sendBlocker(target ?? null);
    const canSend = all.length > 0 && !this.sending && !blocker;
    const sendTip = !all.length ? "Add a comment first" : blocker ?? `Paste ${all.length} comment(s) into ${target.program?.name} and press Enter (⌘Enter)`;
    const width = Math.max(400, ...this.rows.map((r) => (r.text?.length ?? 0) * 7.3 + 140));

    const header = h_flex().h(42).flex_none().px(14).gap(8).items_center().border_b(1).border_color(t.border)
      .child(div().font_semibold().child("Review"))
      .child(div().text_color(t.muted).truncate().child(this.root ? `${basename(this.root)}@${this.branch}` : ""))
      .child(div().px(5).py(1).rounded(4).border_1().border_color(t.border).text_size(10).text_color(t.muted).child(`${all.length} comment${all.length === 1 ? "" : "s"}`))
      .child(div().flex_1())
      .when(candidates.length > 1, (e) => e.child(h_flex().gap(4).items_center().children(candidates.map((a) =>
        h_flex().id(`ag-${a.address}`).h(28).px(8).gap(6).items_center().rounded(7).text_size(11.5).cursor_pointer()
          .border_1().border_color(a.address === preferred ? t.accent : t.border)
          .text_color(a.address === preferred ? t.foreground : t.muted).hover((s) => s.bg(t.hover))
          .tooltip(`${a.program?.name} in tab ${a.tab_name} · ${a.status}`)
          .on_click((_e, cx) => { this.chosen = a.address; cx.notify(); })
          .child(div().size(7).rounded(4).flex_none().bg(t[a.status] ?? t.unknown))
          .child(`${a.program?.name ?? a.address} · ${a.tab_name}`)))))
      .when(all.length > 0, (e) => e.child(this.button("clear", this.confirmClear ? "Clear all?" : "Clear", { tip: "Delete every comment on this branch" }, (cx) => this.clear(cx))))
      .child(this.button("send", this.sending ? "Sending…" : "Send to agent", { primary: true, enabled: canSend, tip: sendTip }, (cx) => this.send(cx)));

    const fileList = v_flex().w(260).flex_none().h_full().min_h(0).relative().border_r(1).border_color(t.border)
      .child(v_flex().id("dr-files").absolute().inset_0().overflow_y_scroll().p(6).children(this.files.map((f) => {
        const parts = f.path.split("/"), name = parts.pop();
        return h_flex().id(`f-${f.path}`).h(28).flex_none().px(8).gap(8).items_center().rounded(7).cursor_pointer()
          .when(this.open === f.path, (e) => e.bg(t.selected)).hover((s) => s.bg(t.hover)).tooltip(f.path)
          .on_click((_e, cx) => { this.pick(f.path, cx); this.handle.focus(); })
          .child(div().w(12).text_size(11).text_color(f.status === "A" ? t.ansi2 : f.status === "D" ? t.ansi1 : t.blocked).child(f.untracked ? "U" : f.status))
          .child(h_flex().flex_1().min_w(0).child(div().text_color(t.dim).truncate().child(parts.length ? parts.join("/") + "/" : "")).child(div().flex_none().max_w(200).truncate().child(name)))
          .when(Boolean(counts[f.path]), (e) => e.child(div().px(5).rounded(4).bg(t.accent).text_color(t.accent_foreground).text_size(10.5).child(String(counts[f.path]))))
          .when(f.add > 0, (e) => e.child(div().text_size(11).text_color(t.ansi2).child(`+${f.add}`)))
          .when(f.del > 0, (e) => e.child(div().text_size(11).text_color(t.ansi1).child(`−${f.del}`)));
      })).when(!this.files.length, (e) => e.child(div().p(8).text_color(t.dim).child(this.error || (this.loading ? "Loading changes…" : this.root ? "No changes against HEAD" : "Looking for a repository…")))))
      .child(Scrollbar.vertical("dr-files").absolute().inset_0());

    const diff = div().flex_1().min_w(0).h_full().min_h(0).relative().font_family(mono).text_size(12)
      .child(div().id("dr-diff-x").absolute().inset_0().overflow_x_scrollbar()
        .child(div().id("dr-diff-content").w_full().min_w(width).h_full()
          .on_mouse_down("left", (e, cx) => { this.pressX = e.local_position?.x ?? null; })
          .child(v_virtual_list("dr-diff", items.length, items.map((i) => i.size), (i) => {
            const it = items[i]; return it.type === "line" ? `l${it.index}` : it.type === "comment" ? `c${it.comment.id}` : "editor";
          }, (range) => items.slice(range.start, range.end).map((it) =>
            it.type === "line" ? this.lineRow(it, t) : it.type === "comment" ? this.commentRow(it, t) : this.editorRow(it, t)))
            .on_item_click((key, cx) => this.itemClick(key, cx))
            .track_scroll(this.scroll)
            .size_full())))
      .child(Scrollbar.vertical("dr-diff").viewport_from_layout().absolute().inset_0())
      .when(!this.open && this.files.length > 0, (e) => e.child(div().absolute().top(6).px(12).text_color(t.dim).child("Select a file")))
      .when(Boolean(this.open) && !this.rows.length, (e) => e.child(div().absolute().top(6).px(12).text_color(t.dim).child("Loading diff…")));

    const footer = h_flex().h(28).flex_none().px(14).gap(14).items_center().border_t(1).border_color(t.border).text_size(11).text_color(t.dim)
      .child(div().child("j/k move · c comment · Enter save · Esc cancel · ⌘Enter send"))
      .child(div().flex_1())
      .when(Boolean(this.truncated), (e) => e.child(div().child(`Showing the first ${MAX_ROWS} lines`)))
      .child(div().truncate().child(target ? `To ${target.program?.name} · tab ${target.tab_name} · ${target.status}` : "No agent in this space"));

    return v_flex().id("diff-review").size_full().min_h(0).overflow_hidden().bg(t.background).text_color(t.foreground).text_size(13)
      .track_focus(this.handle).on_key_down((e, cx) => this.onKey(e, cx))
      .child(header)
      .child(h_flex().flex_1().min_h(0).child(fileList).child(diff))
      .child(footer);
  }
}
