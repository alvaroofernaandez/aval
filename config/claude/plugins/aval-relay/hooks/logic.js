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

/** The envelope `aval relay compact` reads on stdin. */
export function envelope(event, usage, sessionId) {
  const context = (usage && usage.context) || {};
  return {
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
