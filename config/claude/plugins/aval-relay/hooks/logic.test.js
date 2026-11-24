// Run: node --test config/claude/plugins/aval-relay/hooks/logic.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { newSessionId, envelope, answer, steered, newSessionPrompt, nextPending, STEER, PROTOCOL_VERSION } from "./logic.js";

const messages = [
  { handle: "h1", role: "user", text: "Turn 1", toolUses: [] },
  { handle: "h2", role: "assistant", text: "Answer 1", toolUses: [{ id: "t1" }] },
  { handle: "h3", role: "user", text: "Turn 2", toolUses: [] },
];

test("newSessionId is stable in shape and differs per call", () => {
  let n = 0.1;
  const random = () => (n += 0.1);
  const a = newSessionId(random), b = newSessionId(random);
  assert.match(a, /^relay-[0-9a-z]{8,}$/);
  assert.notEqual(a, b);
});

test("envelope carries the trigger, the exact usage and the messages", () => {
  const got = envelope({ trigger: "auto", messages }, { context: { tokens: 201000, window: 1000000, percent: 20 } }, "relay-abc12345");
  assert.deepEqual(got, {
    version: PROTOCOL_VERSION,
    event: "compact",
    trigger: "auto",
    session_id: "relay-abc12345",
    usage: { tokens: 201000, window: 1000000 },
    messages: [
      { handle: "h1", role: "user", text: "Turn 1", toolUses: [] },
      { handle: "h2", role: "assistant", text: "Answer 1", toolUses: [{ id: "t1" }] },
      { handle: "h3", role: "user", text: "Turn 2", toolUses: [] },
    ],
  });
});

test("envelope survives a missing usage and missing fields", () => {
  const got = envelope({ trigger: "manual", messages: [{ role: "user" }] }, undefined, "relay-abc12345");
  assert.deepEqual(got.usage, { tokens: 0, window: 0 });
  assert.deepEqual(got.messages, [{ handle: "", role: "user", text: "", toolUses: [] }]);
});

test("answer replaces with the capsule and the kept messages, in order", () => {
  const got = answer({ version: 1, action: "replace", capsule: "# capsule", keep: ["h3", "h2"] }, messages);
  assert.deepEqual(got, {
    messages: [
      { role: "user", text: "# capsule", toolUses: [] },
      messages[1],
      messages[2],
    ],
  });
});

test("answer is null for anything but a usable replace", () => {
  for (const r of [
    { version: 1, action: "passthrough", reason: "paused" },
    { version: 2, action: "replace", capsule: "x", keep: [] },
    { version: 1, action: "replace", capsule: "", keep: ["h3"] },
    { version: 1, action: "replace", capsule: "x", keep: ["nope"] },
    null, undefined, "x",
  ]) {
    assert.equal(answer(r, messages), null, JSON.stringify(r));
  }
});

test("no answer from aval steers the native summary with the fixed text", () => {
  const got = steered({ trigger: "auto", messages }, null);
  assert.equal(got.instructions, STEER);
  assert.match(STEER, /first request/);
  assert.deepEqual(got.messages, messages);
});

test("a steer answer passes aval's instructions to the summariser", () => {
  const got = steered({ trigger: "auto", messages }, { version: PROTOCOL_VERSION, action: "steer", instructions: "Keep branch feat/x." });
  assert.equal(got.instructions, "Keep branch feat/x.");
});

test("the person's /compact instructions come first and are kept", () => {
  const got = steered({ trigger: "manual", instructions: "focus on tests", messages }, null);
  assert.equal(got.instructions, "focus on tests\n\n" + STEER);
});

test("a deliberate passthrough leaves the event as it is", () => {
  const e = { trigger: "auto", messages };
  assert.equal(steered(e, { version: PROTOCOL_VERSION, action: "passthrough", reason: "paused" }), e);
});

test("a steer answer of another version is not trusted", () => {
  const e = { trigger: "auto", messages };
  assert.equal(steered(e, { version: 99, action: "steer", instructions: "x" }), e);
});

test("a replace the mod cannot apply steers with the fixed text", () => {
  const resp = { version: PROTOCOL_VERSION, action: "replace", capsule: "C", keep: ["gone"] };
  assert.equal(answer(resp, messages), null);
  assert.equal(steered({ trigger: "auto", messages }, resp).instructions, STEER);
});

const newSession = { version: PROTOCOL_VERSION, action: "new_session", reason: "brake:max_chain", capsule: "# aval relay capsule\nFirst request: x\n" };

test("a new_session answer compacts natively now, steered", () => {
  assert.equal(steered({ trigger: "auto", messages }, newSession).instructions, STEER);
});

test("a new_session answer gives the prompt for the new session", () => {
  assert.equal(newSessionPrompt(newSession), newSession.capsule);
});

test("no prompt for other answers, other versions or no capsule", () => {
  assert.equal(newSessionPrompt(null), null);
  assert.equal(newSessionPrompt({ ...newSession, version: 99 }), null);
  assert.equal(newSessionPrompt({ ...newSession, capsule: "" }), null);
  assert.equal(newSessionPrompt({ version: PROTOCOL_VERSION, action: "replace", capsule: "C", keep: ["h1"] }), null);
});

test("envelope carries the prompt in flight when there is one", () => {
  const got = envelope({ trigger: "auto", messages }, undefined, "relay-abc12345", "GAMMA");
  assert.equal(got.prompt, "GAMMA");
  assert.equal("prompt" in envelope({ trigger: "auto", messages }, undefined, "relay-abc12345"), false);
});

// Found in the T11 sandbox run: the clear waits behind queued input, so more
// compactions answer before it runs. The newest capsule wins, and an answer
// that is not new_session never drops one still to submit.
test("nextPending keeps the newest capsule until it is submitted", () => {
  const newer = { ...newSession, capsule: "# aval relay capsule\nFirst request: y\n" };
  assert.equal(nextPending(null, newSession), newSession.capsule);
  assert.equal(nextPending(newSession.capsule, newer), newer.capsule);
  assert.equal(nextPending(newSession.capsule, { version: PROTOCOL_VERSION, action: "steer", instructions: "x" }), newSession.capsule);
  assert.equal(nextPending(newSession.capsule, null), newSession.capsule);
  assert.equal(nextPending(null, { version: PROTOCOL_VERSION, action: "passthrough" }), null);
});
