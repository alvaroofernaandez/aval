// Run: node --test config/claude/plugins/aval-relay/hooks/logic.test.js
import { test } from "node:test";
import assert from "node:assert/strict";
import { newSessionId, envelope, answer, PROTOCOL_VERSION } from "./logic.js";

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
