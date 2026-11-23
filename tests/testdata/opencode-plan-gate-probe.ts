// Drives the plan-gate plugin's tool.execute.before without an OpenCode
// session:
//   probe <tool> <file> <dir>  -> "allow" | "deny <reason>"
// The import below is rewritten by the test to an absolute path.
import { PlanGate } from "../../config/opencode/plugin/plan-gate.ts"

const [, , tool, file, dir] = process.argv
const hooks: any = await (PlanGate as any)({ directory: dir })
const started = performance.now()
try {
  await hooks["tool.execute.before"]({ tool, sessionID: "s1", callID: "c1" }, { args: { filePath: file } })
  console.log("allow")
} catch (e: any) {
  console.log("deny " + String(e?.message ?? e))
}
if (process.env.AVAL_PROBE_TIMING) console.error(`ms ${(performance.now() - started).toFixed(1)}`)
