// aval relay mod for Claude Code. Plumbing only (ADR 0001 amendment): it
// forwards session.compact to `aval relay compact` and applies the answer.
// Every decision is in Go. With AVAL_RELAY other than 1 every handler returns
// next(e) and starts no process (ADR 0007). Any error, timeout or non-zero
// exit falls back to next(e): Claude Code compacts as it would without aval.
import { newSessionId, envelope, answer, PROTOCOL_VERSION } from "./logic.js";

const TIMEOUT_MS = 2000;
const session = newSessionId(Math.random);
let commitPending = false;

async function relayOn($) {
  return (await $.env.get("AVAL_RELAY")) === "1";
}

async function aval($, args, input) {
  const r = await $.process.run(["aval", "relay", ...args], { stdin: JSON.stringify(input), timeoutMs: TIMEOUT_MS });
  if (r.exitCode !== 0) throw new Error(`aval relay ${args[0]} exited ${r.exitCode}`);
  return JSON.parse(r.stdout);
}

export const register = (on) => {
  on("session.compact", async ($, e, next) => {
    if (!(await relayOn($))) return next(e);
    try {
      const usage = await $.session.usage();
      const out = answer(await aval($, ["compact"], envelope(e, usage, session)), e.messages);
      if (!out) return next(e);
      commitPending = true;
      return out;
    } catch {
      return next(e);
    }
  });

  // The first measure after a relay records the tokens the new context holds.
  on("session.measure", async ($, e, next) => {
    if (!commitPending) return next(e);
    commitPending = false;
    try {
      if (await relayOn($)) {
        await aval($, ["commit"], { version: PROTOCOL_VERSION, session_id: session, tokens: (e.context && e.context.tokens) || 0 });
      }
    } catch {
      // A failed commit leaves the journal prepared; the next relay is not affected.
    }
    return next(e);
  });
};
