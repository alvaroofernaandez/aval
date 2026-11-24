// Pure logic of the aval relay mod. No Claude Code imports, so `node --test`
// loads it as is. The policy lives in `aval relay compact`; this file only
// shapes what the mod sends and applies what aval answers.

export const PROTOCOL_VERSION = 1;

/**
 * A per-process session id. The mods API exposes none (no recorded payload of
 * spike #63 carries one), so the relay chain follows the Claude Code process:
 * it survives a clear and starts again on resume.
 */
export function newSessionId(random) {
  let id = "";
  while (id.length < 12) id += Math.floor(random() * 1e9).toString(36);
  return "relay-" + id.slice(0, 12);
}

/**
 * The envelope `aval relay compact` reads on stdin. prompt is the latest one
 * the person submitted: an auto compaction runs before it joins the messages.
 */
export function envelope(event, usage, sessionId, prompt) {
  const context = (usage && usage.context) || {};
  const env = {
    version: PROTOCOL_VERSION,
    event: "compact",
    trigger: event.trigger,
    session_id: sessionId,
    usage: { tokens: context.tokens || 0, window: context.window || 0 },
    messages: (event.messages || []).map((m) => ({
      handle: m.handle || "",
      role: m.role,
      text: m.text || "",
      toolUses: m.toolUses || [],
    })),
  };
  if (prompt) env.prompt = prompt;
  return env;
}

/**
 * The session.compact answer for aval's response, or null to let Claude Code
 * compact natively. Kept messages are the runtime's own objects, by handle and
 * in their original order; the capsule is a hand-built user message, which
 * needs `toolUses: []` (spike #63, S1).
 */
export function answer(response, messages) {
  if (!response || typeof response !== "object") return null;
  if (response.version !== PROTOCOL_VERSION || response.action !== "replace" || !response.capsule) return null;
  const keep = new Set(response.keep || []);
  const kept = messages.filter((m) => m.handle && keep.has(m.handle));
  if (kept.length === 0) return null;
  return { messages: [{ role: "user", text: response.capsule, toolUses: [] }, ...kept] };
}

/**
 * The fixed summariser instructions of ladder level 1, used when aval gives no
 * answer (missing, slow or failed). A constant, not a decision: the policy
 * stays in Go. Spike #63 P1 showed the native summary follows instructions.
 */
export const STEER = [
  "Write the summary so work can continue without the earlier messages.",
  "Keep, verbatim where short: the person's first request and their latest one,",
  "the repository and branch, every file path read or changed, commands still",
  "running or scheduled, decisions taken and why, and the next step.",
  "Drop tool output that was already acted on.",
].join(" ");

/**
 * The event to hand to next() when aval does not replace (ladder level 1).
 * No answer, a replace the mod could not apply, or a new_session answer (the
 * compaction must still happen now) steers with STEER; a steer
 * answer steers with aval's own
 * text; any other answer was a deliberate passthrough and changes nothing.
 * The person's own /compact instructions are kept first.
 */
export function steered(event, response) {
  let text = null;
  if (!response || typeof response !== "object" || response.action === "replace" || response.action === "new_session") {
    text = STEER;
  } else if (response.version === PROTOCOL_VERSION && response.action === "steer" && response.instructions) {
    text = response.instructions;
  }
  if (text === null) return event;
  return { ...event, instructions: event.instructions ? event.instructions + "\n\n" + text : text };
}

/**
 * The prompt that opens the new session of ladder level 3, or null. aval
 * answers new_session when the relay chain is at its limit; the mod compacts
 * natively now and, at the end of the turn, clears and submits this prompt.
 */
export function newSessionPrompt(response) {
  if (!response || typeof response !== "object") return null;
  if (response.version !== PROTOCOL_VERSION || response.action !== "new_session") return null;
  return typeof response.capsule === "string" && response.capsule ? response.capsule : null;
}
