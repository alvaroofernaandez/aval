// Drives the rate-guard plugin's tool.execute.before without an OpenCode
// session:
//   probe <tool> <command>  -> "allow" | "deny <reason>"
// The import below is rewritten by the test to an absolute path.
import { RateGuard } from "../../config/opencode/plugin/rate-guard.ts"

const [, , tool, command] = process.argv
const hooks: any = await (RateGuard as any)({ directory: process.cwd() })
try {
  await hooks["tool.execute.before"]({ tool, sessionID: "s1", callID: "c1" }, { args: { command } })
  console.log("allow")
} catch (e: any) {
  console.log("deny " + String(e?.message ?? e))
}
