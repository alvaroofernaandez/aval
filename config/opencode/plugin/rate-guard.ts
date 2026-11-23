import type { Plugin } from "@opencode-ai/plugin"
import { spawnSync } from "node:child_process"
import { existsSync } from "node:fs"
import { join } from "node:path"
import { homedir } from "node:os"

/**
 * The OpenCode half of `aval hook rate-guard` (internal/agent/rateguard).
 *
 * Some hosts (Helice: innohelp.es, helice.app) ban the client IP after a burst
 * of requests. Before every bash call this asks the same aval binary, with the
 * same PreToolUse payload Claude Code sends, so both agents share one rule set
 * and one hosts list. A deny becomes a thrown error, which is upstream's
 * documented way to block a tool call.
 *
 * FAIL-OPEN: no binary, a timeout, odd output or any error means "allow".
 */

function avalBin(): string {
  const local = join(process.env.HOME || homedir(), ".local", "bin", "aval")
  return existsSync(local) ? local : "aval"
}

/** null = allowed; otherwise the deny reason. */
export function denyReason(command: string): string | null {
  try {
    const payload = JSON.stringify({ tool_name: "Bash", tool_input: { command } })
    const r = spawnSync(avalBin(), ["hook", "rate-guard"], { input: payload, encoding: "utf8", timeout: 5000 })
    if (r.status !== 0 || !r.stdout) return null
    const out = JSON.parse(r.stdout)?.hookSpecificOutput
    return out?.permissionDecision === "deny" ? String(out.permissionDecisionReason) : null
  } catch {
    return null
  }
}

export const RateGuard: Plugin = async () => {
  return {
    "tool.execute.before": async (input: any, output: any) => {
      if (input?.tool !== "bash") return
      const command = output?.args?.command
      if (typeof command !== "string" || command === "") return
      const reason = denyReason(command)
      if (reason) throw new Error(reason)
    },
  }
}
