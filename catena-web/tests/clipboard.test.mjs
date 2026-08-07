import assert from "node:assert/strict";
import test from "node:test";
import { copyText } from "../src/clipboard.ts";

test("copyText prefers the async Clipboard API", async () => {
  const calls = [];
  const copied = await copyText("secret", {
    write: async (value) => { calls.push(`write:${value}`); },
    fallback: () => { calls.push("fallback"); return true; },
  });
  assert.equal(copied, true);
  assert.deepEqual(calls, ["write:secret"]);
});

test("copyText falls back when the user agent denies Clipboard API access", async () => {
  const copied = await copyText("secret", {
    write: async () => { throw new Error("NotAllowedError"); },
    fallback: (value) => value === "secret",
  });
  assert.equal(copied, true);
});

test("copyText reports failure so the UI can keep plaintext selectable", async () => {
  const copied = await copyText("secret", {
    write: async () => { throw new Error("NotAllowedError"); },
    fallback: () => false,
  });
  assert.equal(copied, false);
});
