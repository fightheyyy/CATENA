import assert from "node:assert/strict";
import test from "node:test";
import { experimentArm, expectedTrials } from "../src/experiments.ts";

test("invalid trial rewards never inflate success or usage metrics", () => {
 const base = { variant: "baseline", task: "case", input_tokens: 10, output_tokens: 2, command_count: 3, missing_rg_errors: 1 };
 const summary = experimentArm({ trials: [{ ...base, valid: true, rewards: { reward: 0 } }, { ...base, valid: false, rewards: { reward: 1 } }] }, "baseline");
 assert.equal(summary.total, 2); assert.equal(summary.valid, 1); assert.equal(summary.passed, 0); assert.equal(summary.input, 10); assert.equal(summary.errors, 1);
 assert.equal(experimentArm({ trials: [] }, "candidate").valid, 0);
});

test("memory comparison preserves old experiments and reports missing duration honestly", () => {
 assert.equal(expectedTrials({}), 6);
 assert.equal(expectedTrials({study: "memory_skill", attempts: 2}), 18);
 assert.equal(experimentArm({trials: [{variant: "memory", valid: true, rewards: {reward: 1}}]}, "memory").seconds, null);
 const result = experimentArm({trials: [{variant: "memory", valid: true, rewards: {reward: 1}, duration_seconds: 3.5}, {variant: "memory", valid: false, rewards: {reward: 1}, duration_seconds: 99}]}, "memory");
 assert.equal(result.seconds, 3.5);
});
