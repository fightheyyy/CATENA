import assert from "node:assert/strict";
import test from "node:test";
import {
  isMemoryTaskActive,
  memoryTaskDisplayPercent,
  memoryTaskFromReceipt,
  memoryTaskPercent,
  memoryTaskStorageKey,
} from "../src/memoryTaskView.ts";

test("memory receipt becomes a pollable pending task", () => {
  const task = memoryTaskFromReceipt({
    conversation_id: 7,
    task_id: "task-7",
    status: "indexing",
    indexed: false,
    message: "accepted",
  });
  assert.equal(task.status, "pending");
  assert.equal(task.progress, 0);
  assert.equal(isMemoryTaskActive(task), true);
});

test("memory progress is safe for display", () => {
  assert.equal(memoryTaskPercent(-1), 0);
  assert.equal(memoryTaskPercent(0.426), 43);
  assert.equal(memoryTaskPercent(2), 100);
  assert.equal(memoryTaskPercent(Number.NaN), 0);
});

test("memory progress follows the visible pipeline step when upstream progress is coarse", () => {
  assert.equal(memoryTaskDisplayPercent({
    task_id: "task-1",
    status: "processing",
    current_step: "关系分析",
    progress: 0.1,
    steps: [],
  }), 42);
  assert.equal(memoryTaskDisplayPercent({
    task_id: "task-1",
    status: "completed",
    progress: 0.1,
    steps: [],
  }), 100);
});

test("memory task storage is scoped to Agent and conversation", () => {
  assert.notEqual(
    memoryTaskStorageKey("xiaoba-a", "pet:main"),
    memoryTaskStorageKey("xiaoba-b", "pet:main"),
  );
});
