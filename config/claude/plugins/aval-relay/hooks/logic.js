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

/**
 * The level 3 prompt still to submit after aval answers response. The clear
 * waits behind input already queued, so more compactions can answer before
 * it runs: the newest capsule wins, and an answer that is not new_session
 * never drops one still to submit.
 */
export function nextPending(current, response) {
  return newSessionPrompt(response) || current;
}

// measured is what the first session.measure after a relay sends: the commit
// of the tokens the new context holds, then the notification of that relay.
export function measured(sessionId, event) {
  const tokens = (event && event.context && event.context.tokens) || 0;
  return {
    commit: { version: PROTOCOL_VERSION, session_id: sessionId, tokens },
    notify: { version: PROTOCOL_VERSION, session_id: sessionId },
  };
}

// Boundary clear (relay-autoclear T4). The mod only shapes the last turn;
// `aval relay boundary` decides. Tool inputs are cut to the fields the policy
// reads and capped, so the process input stays small.
const INPUT_FIELDS = ["command", "file_path", "old_string", "new_string", "content"];
const INPUT_CAP = 20000;

function pickInput(input) {
  const out = {};
  for (const k of INPUT_FIELDS) {
    if (typeof (input || {})[k] === "string") out[k] = input[k].slice(0, INPUT_CAP);
  }
  return out;
}

/**
 * The envelope `aval relay boundary` reads: the tokens in context and the
 * last main-thread turn, that is the messages after the person's last prompt
 * (a user message with text and no tool results). Null when messages is not
 * a list (a denied read), so the caller does nothing.
 */
export function boundaryEnvelope(messages, usage) {
  if (!Array.isArray(messages)) return null;
  let start = 0;
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.role === "user" && m.text && !(m.toolResults && m.toolResults.length)) {
      start = i + 1;
      break;
    }
  }
  const turn = messages.slice(start);
  const failed = new Set();
  for (const m of turn) for (const r of m.toolResults || []) if (r.isError) failed.add(r.tool_use_id);
  const tools = [];
  let reply = "";
  for (const m of turn) {
    if (m.role !== "assistant") continue;
    if (m.text) reply = m.text;
    for (const u of m.toolUses || []) {
      const t = { tool: u.tool, input: pickInput(u.input) };
      if (u.isError || failed.has(u.tool_use_id)) t.isError = true;
      tools.push(t);
    }
  }
  const tokens = (usage && usage.context && usage.context.tokens) || 0;
  return { version: PROTOCOL_VERSION, tokens, turn: { reply, tools } };
}

/** The prompt to submit after the clear, or null when aval does not say clear. */
export function boundaryText(response) {
  if (!response || typeof response !== "object") return null;
  if (response.version !== PROTOCOL_VERSION || response.action !== "clear") return null;
  return typeof response.text === "string" && response.text ? response.text : null;
}
