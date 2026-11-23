import type { Plugin } from "@opencode-ai/plugin"
import { spawnSync } from "node:child_process"
import { existsSync } from "node:fs"
import { join } from "node:path"
import { homedir } from "node:os"

/**
 * The OpenCode half of `aval hook plan-gate` (internal/deck/gate.go).
 *
 * While the project's active plan (odd/tasks/*.md) has no deck, edits outside
 * odd/ are denied. Before every edit, write or patch this asks the same aval
 * binary, with the same PreToolUse payload Claude Code sends, so both agents
 * share one rule and one marker written by `aval deck build --plan`. There is
 * no gate logic here. A deny becomes a thrown error, which is upstream's
 * documented way to block a tool call.
 *
 * FAIL-OPEN: no binary, a timeout, odd output or any error means "allow".
 * A patch call that carries only a patch text and no path is not gated.
 */

function avalBin(): string {
  const local = join(process.env.HOME || homedir(), ".local", "bin", "aval")
  return existsSync(local) ? local : "aval"
}

/** null = allowed; otherwise the deny reason. */
export function denyReason(cwd: string, tool: string, file: string): string | null {
  try {
    const payload = JSON.stringify({
      cwd,
      tool_name: tool === "write" ? "Write" : "Edit",
      tool_input: { file_path: file },
    })
    const r = spawnSync(avalBin(), ["hook", "plan-gate"], { input: payload, encoding: "utf8", timeout: 2000 })
    if (r.status !== 0 || !r.stdout) return null
    const out = JSON.parse(r.stdout)?.hookSpecificOutput
    return out?.permissionDecision === "deny" ? String(out.permissionDecisionReason) : null
  } catch {
    return null
  }
}

export const PlanGate: Plugin = async ({ directory }: any) => {
  return {
    "tool.execute.before": async (input: any, output: any) => {
      const tool = input?.tool
      if (tool !== "edit" && tool !== "write" && tool !== "patch") return
      if (process.env.AVAL_PLAN_DECK_OFF) return
      const args = output?.args ?? {}
      const file = args.filePath ?? args.path ?? args.file
      if (typeof file !== "string" || file === "") return
      const reason = denyReason(directory || process.cwd(), tool, file)
      if (reason) throw new Error(reason)
    },
  }
}
