export type TaskLifecycleStatus =
  | "queued" | "running" | "generating" | "cancel_requested" | "completed"
  | "failed" | "expired" | "manual_review" | "cancelled" | "unknown";

export const TASK_STATUS_LABELS: Record<TaskLifecycleStatus, string> = {
  queued: "排队中", running: "生成中", generating: "生成中", cancel_requested: "取消中",
  completed: "已完成", failed: "失败", expired: "已超时", manual_review: "人工审核中",
  cancelled: "已取消", unknown: "处理中",
};

export function normalizeTaskLifecycleStatus(value: unknown): TaskLifecycleStatus {
  const status = String(value || "").trim().toLowerCase();
  if (["queued", "pending", "waiting"].includes(status)) return "queued";
  if (["running", "processing", "generating", "in_progress"].includes(status)) return "running";
  if (["cancel_requested", "cancelling", "canceling"].includes(status)) return "cancel_requested";
  if (["completed", "succeeded", "success", "done"].includes(status)) return "completed";
  if (["failed", "error"].includes(status)) return "failed";
  if (["expired", "timeout", "timed_out"].includes(status)) return "expired";
  if (["manual_review", "review", "under_review"].includes(status)) return "manual_review";
  if (["cancelled", "canceled"].includes(status)) return "cancelled";
  return "unknown";
}

export function taskStatusLabel(status: unknown): string {
  return TASK_STATUS_LABELS[normalizeTaskLifecycleStatus(status)];
}

export function isTaskActive(status: unknown): boolean {
  return ["queued", "running", "generating", "cancel_requested", "unknown"].includes(normalizeTaskLifecycleStatus(status));
}

/** Progressive polling: 5s for first 5m, 20s until 15m, then 60s. */
export function taskPollingDelay(createdAt: string | number | Date | undefined, now = Date.now()): number {
  const started = createdAt instanceof Date ? createdAt.getTime() : new Date(createdAt || now).getTime();
  const age = Math.max(0, now - (Number.isFinite(started) ? started : now));
  return age < 5 * 60_000 ? 5_000 : age < 15 * 60_000 ? 20_000 : 60_000;
}

export function taskTimeoutMessage(createdAt: string | number | Date | undefined, now = Date.now()): string {
  const started = createdAt instanceof Date ? createdAt.getTime() : new Date(createdAt || now).getTime();
  return Number.isFinite(started) && now - started >= 15 * 60_000 ? "任务处理时间较长，仍在后台处理中，请稍后查看结果。" : "";
}
