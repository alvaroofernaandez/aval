import type { Plugin } from "@opencode-ai/plugin"
import { createHash } from "node:crypto"
import { existsSync, readFileSync, realpathSync } from "node:fs"
import { dirname, isAbsolute, join, resolve } from "node:path"
import { homedir } from "node:os"

/**
 * The OpenCode half of the design-pipeline gate.
 *
 * Same contract as `aval hook design-gate` (internal/design/hook), same marker
 * file: `aval design ack <surface>` writes
 *
 *   ${XDG_CACHE_HOME:-~/.cache}/design-gate/<sha1 of project root>
 *
 * and an edit to a UI surface is allowed while that marker is younger than
 * DESIGN_ACK_TTL seconds (default 4 h) and the sha1 it recorded for
 * .agents/DESIGN.md still matches the file. One `aval design ack` therefore opens
 * both agents, and sub-agents and post-compaction turns need no special case:
 * there is no in-memory checklist to lose.
 *
 * `tool.execute.before` throwing is upstream's documented way to deny a call.
 *
 * FAIL-OPEN: a gate that hard-fails on anything it cannot determine is a gate
 * people delete. Unreadable marker or missing path means "cannot know", and the
 * only denial is a UI edit with a clearly absent, stale or mismatched marker.
 */

const UI_EXT = [
  ".tsx", ".jsx", ".vue", ".svelte", ".astro",
  ".css", ".scss", ".sass", ".less", ".styl",
  ".html", ".htm",
]

/**
 * Tests are not design surfaces. Blocking them would deadlock against the
 * strict-TDD rule, which requires the test to be written FIRST. The Go gate
 * carves out the same exception.
 */
export function isUISurface(filePath: string): boolean {
  if (!filePath) return false
  const base = filePath.split("/").pop() ?? ""
  if (/\.(test|spec|stories)\./.test(base)) return false
  if (filePath.includes("/__tests__/") || filePath.includes("/node_modules/")) return false
  return UI_EXT.some((e) => base.toLowerCase().endsWith(e))
}

const sha1 = (data: string | Buffer) => createHash("sha1").update(data).digest("hex")

/** Walk up from the file looking for .git; fall back to the session directory. */
function projectRoot(file: string, cwd: string): string {
  let d = dirname(file)
  while (true) {
    if (existsSync(join(d, ".git"))) return d
    const up = dirname(d)
    if (up === d) break
    d = up
  }
  return cwd
}

const physical = (p: string) => {
  try { return realpathSync(p) } catch { return p }
}

/** null = allowed; otherwise the reason it is not. */
export function denyReason(file: string, cwd: string, env = process.env, nowMs = Date.now()): string | null {
  if (!isUISurface(file)) return null
  const abs = isAbsolute(file) ? file : resolve(cwd, file)
  const root = physical(projectRoot(abs, cwd))
  const cache = join(env.XDG_CACHE_HOME || join(homedir(), ".cache"), "design-gate")

  let why = "no design-ack for this project yet"
  let marker: string | null = null
  try { marker = readFileSync(join(cache, sha1(root)), "utf8") } catch { /* absent */ }

  if (marker !== null) {
    const kv: Record<string, string> = {}
    for (const line of marker.split("\n")) {
      const i = line.indexOf("=")
      if (i > 0) kv[line.slice(0, i)] = line.slice(i + 1)
    }
    const ttlRaw = Number(env.DESIGN_ACK_TTL)
    const ttl = Number.isInteger(ttlRaw) && ttlRaw >= 0 && env.DESIGN_ACK_TTL !== "" ? ttlRaw : 14400
    let current = ""
    try { current = sha1(readFileSync(join(root, ".agents", "DESIGN.md"))) } catch { /* none */ }

    if (!/^\d+$/.test(kv.epoch ?? "")) why = "the design-ack marker is unreadable"
    else if (Math.floor(nowMs / 1000) - Number(kv.epoch) > ttl)
      why = `the last design-ack is older than ${Math.floor(ttl / 60)} min`
    else if ((kv.design_sha1 ?? "") !== current) why = ".agents/DESIGN.md changed since the last design-ack"
    else return null
  }

  const base = file.split("/").pop()
  return `Blocked: ${base} is a UI surface and ${why}. Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run \`aval design ack <surface>\` and retry.`
}

export const DesignGate: Plugin = async ({ directory }: any = {}) => {
  return {
    "tool.execute.before": async (input: any, output: any) => {
      if (process.env.DESIGN_PIPELINE_OFF) return
      if (!["edit", "write", "patch"].includes(input?.tool)) return

      const fp = output?.args?.filePath ?? output?.args?.path ?? output?.args?.file
      if (typeof fp !== "string") return

      const reason = denyReason(fp, directory || process.cwd())
      if (reason) throw new Error(reason)
    },
  }
}
