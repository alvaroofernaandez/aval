// aval relay mod for Claude Code. Plumbing only (ADR 0001 amendment): it
// forwards session.compact to `aval relay compact` and applies the answer.
// Every decision is in Go. With AVAL_RELAY other than 1 every handler returns
// next(e) and starts no process (ADR 0007). Any error, timeout or non-zero
// exit falls to ladder level 1: Claude Code compacts natively, steered by the
// fixed STEER text. A capsule that cannot be applied falls the same way.
import { newSessionId, envelope, answer, steered, nextPending, PROTOCOL_VERSION } from "./logic.js";

const TIMEOUT_MS = 2000;
let session = newSessionId(Math.random);
let commitPending = false;
let submitPending = null; // ladder level 3: the prompt of the new session
let clearing = false; // a level 3 clear is waiting in Claude Code's queue
let lastPrompt = ""; // the latest prompt submitted, the request in flight

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
    let resp;
    try {
      const usage = await $.session.usage();
      resp = await aval($, ["compact"], envelope(e, usage, session, lastPrompt));
    } catch {
      return next(steered(e, null));
    }
    submitPending = nextPending(submitPending, resp);
    const out = answer(resp, e.messages);
    if (!out) return next(steered(e, resp));
    commitPending = true;
    return out;
  });

  // Remember the request in flight; no process, no decision (ADR 0007).
  on("prompt.submit", async ($, e, next) => {
    if (typeof e.text === "string") lastPrompt = e.text;
    return next(e);
  });

  // Ladder level 3, from turn.complete only (spike #63 S6): clear, then open
  // the new session with the capsule. The new session gets a new id, so its
  // relay chain starts again (relay.Chain). Subagent turns are left alone.
  on("turn.complete", async ($, e, next) => {
    if (!submitPending || clearing || e.agentId !== undefined) return next(e);
    clearing = true;
    try {
      if (await relayOn($)) {
        await $.command.run({ command: "clear" });
        // Read after the clear: compactions that ran while it waited may
        // have left a newer capsule.
        const text = submitPending;
        submitPending = null;
        session = newSessionId(Math.random);
        await $.prompt.submit({ text });
      }
    } catch {
      // A refused clear leaves the compacted session running; the capsule
      // stays pending for the next turn.complete.
    } finally {
      clearing = false;
    }
    return next(e);
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
