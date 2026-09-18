/** Queue semantics shared by Image/Video clients; never render queue progress. */
export const GENERATION_QUEUED_LABEL = "排队中 · 等待运行位";
export const GENERATION_QUEUED_TOAST = "任务已排队，等待运行";

export function generationDisplayStatus(task?: { status?: unknown; taskStatus?: unknown } | null): string {
  const status = String(task?.status || "").toUpperCase();
  // Old payloads can retain taskStatus after completion. Terminal truth wins.
  if (["SUCCEEDED", "SUCCESS", "COMPLETED", "DONE", "FAILED", "ERROR", "CANCELLED", "CANCELED"].includes(status)) return status;
  return String(task?.taskStatus || status).toUpperCase();
}

export function isGenerationQueued(task?: { status?: unknown; taskStatus?: unknown } | null): boolean {
  return generationDisplayStatus(task) === "QUEUED";
}
