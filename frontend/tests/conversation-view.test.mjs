import assert from "node:assert/strict";
import test from "node:test";
import {
  conversationKey,
  conversationMessageText,
  orderedConversationMessages,
} from "../src/conversationView.ts";

test("Conversation identity keeps Agent deployments separate", () => {
  assert.notEqual(
    conversationKey({ agent_id: "xiaoba-a", conversation_id: "pet:main" }),
    conversationKey({ agent_id: "xiaoba-b", conversation_id: "pet:main" }),
  );
});

test("Conversation visible content excludes implementation fields by construction", () => {
  assert.equal(conversationMessageText([
    { type: "text", text: " 已完成。 " },
    { type: "file", name: "report.pdf", ref: "xiaoba-file:report" },
  ]), "已完成。\n[report.pdf]");
});

test("Conversation messages follow source sequence", () => {
  const messages = [
    { message_id: "m2", sequence: 2, occurred_at: "2026-08-06T02:00:00Z" },
    { message_id: "m1", sequence: 1, occurred_at: "2026-08-06T03:00:00Z" },
    { message_id: "m3", sequence: 2, occurred_at: "2026-08-06T01:00:00Z" },
  ];
  assert.deepEqual(
    orderedConversationMessages(messages).map((message) => message.message_id),
    ["m1", "m3", "m2"],
  );
});
