import type { MemoryIngestReceipt, MemoryTaskStatus } from "./types";

const taskStates = new Set<MemoryTaskStatus["status"]>(["pending", "processing", "completed", "failed"]);

export function memoryTaskFromReceipt(receipt: MemoryIngestReceipt): MemoryTaskStatus {
  const status = taskStates.has(receipt.status as MemoryTaskStatus["status"])
    ? receipt.status as MemoryTaskStatus["status"]
    : "pending";
  return {
    task_id: receipt.task_id,
    status,
    progress: status === "completed" ? 1 : 0,
    message: receipt.message,
    conversation_id: receipt.conversation_id,
    steps: [],
  };
}

export function memoryTaskPercent(progress: number) {
  if (!Number.isFinite(progress)) return 0;
  return Math.round(Math.max(0, Math.min(1, progress)) * 100);
}

export function memoryTaskDisplayPercent(task: MemoryTaskStatus | null) {
  if (!task) return 0;
  if (task.status === "completed") return 100;
  const reported = memoryTaskPercent(task.progress);
  const step = task.current_step?.toLowerCase() ?? "";
  const milestones: Array<[RegExp, number]> = [
    [/fact|事实|抽取/, 12],
    [/entit|实体/, 28],
    [/relation|关系/, 42],
    [/graph|图谱/, 56],
    [/topic|cluster|主题|聚类/, 70],
    [/vector|embed|向量|语义索引/, 85],
    [/memor|长期记忆/, 95],
  ];
  return Math.max(reported, milestones.find(([pattern]) => pattern.test(step))?.[1] ?? 0);
}

export function isMemoryTaskActive(task: MemoryTaskStatus | null) {
  return task?.status === "pending" || task?.status === "processing";
}

export function memoryTaskStorageKey(agentID: string, conversationID: string) {
  return `catena.memory-task.${encodeURIComponent(agentID)}.${encodeURIComponent(conversationID)}`;
}
