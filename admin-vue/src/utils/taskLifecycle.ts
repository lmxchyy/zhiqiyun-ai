export type AdminTaskStatus = "queued" | "running" | "cancel_requested" | "completed" | "failed" | "expired" | "manual_review" | "cancelled" | "unknown";

const labels: Record<AdminTaskStatus, string> = {
  queued: "排队中", running: "生成中", cancel_requested: "取消中", completed: "已完成",
  failed: "失败", expired: "已超时", manual_review: "人工审核中", cancelled: "已取消", unknown: "处理中",
};

export function normalizeAdminTaskStatus(value: unknown): AdminTaskStatus {
  const status = String(value || "").trim().toLowerCase();
  if (["pending", "queued", "waiting"].includes(status)) return "queued";
  if (["running", "processing", "generating", "in_progress"].includes(status)) return "running";
  if (["cancel_requested", "cancelling", "canceling"].includes(status)) return "cancel_requested";
  if (["succeeded", "success", "completed", "done"].includes(status)) return "completed";
  if (["failed", "error"].includes(status)) return "failed";
  if (["expired", "timeout", "timed_out"].includes(status)) return "expired";
  if (["manual_review", "review", "under_review"].includes(status)) return "manual_review";
  if (["cancelled", "canceled"].includes(status)) return "cancelled";
  return "unknown";
}

export function adminTaskStatusLabel(value: unknown) { return labels[normalizeAdminTaskStatus(value)]; }
export function adminTaskPollingDelay(createdAt: string | number | Date | undefined, now = Date.now()) {
  const timestamp = createdAt instanceof Date ? createdAt.getTime() : new Date(createdAt || now).getTime();
  const age = Math.max(0, now - (Number.isFinite(timestamp) ? timestamp : now));
  return age < 300000 ? 5000 : age < 900000 ? 20000 : 60000;
}
export function adminTaskTimeoutMessage(createdAt: string | number | Date | undefined, now = Date.now()) {
  const timestamp = createdAt instanceof Date ? createdAt.getTime() : new Date(createdAt || now).getTime();
  return Number.isFinite(timestamp) && now - timestamp >= 900000 ? "任务处理时间较长，仍在后台处理中，请稍后查看结果。" : "";
}
