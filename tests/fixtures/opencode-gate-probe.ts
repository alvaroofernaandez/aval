// Exercises the gate's pure predicate plus the deny path, so the bash suite can
// assert on them without standing up an OpenCode session.
//   probe ui <path>                     -> isUISurface
//   probe edit <path> <dir> [session]   -> "deny" | "allow" for an edit call
import { isUISurface, DesignGate } from "../../config/opencode/plugin/design-gate.ts"

const [, , mode, file, dir, session] = process.argv

if (mode === "ui") {
  console.log(String(isUISurface(file)))
} else {
  const hooks: any = await (DesignGate as any)({ directory: dir })
  try {
    await hooks["tool.execute.before"]({ tool: "edit", sessionID: session ?? "s1", callID: "c1" },
      { args: { filePath: file } })
    console.log("allow")
  } catch (e: any) {
    console.log("deny " + String(e?.message ?? e).split("\n").length)
  }
}
