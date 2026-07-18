import { adminWorkspaceApi } from "../api/adminWorkspaces";

export function trackAdminExperience(eventType: string, moduleId = "", targetId = "", metadata: Record<string, unknown> = {}) {
  if (typeof window !== "undefined" && !window.localStorage.getItem("token") && !window.sessionStorage.getItem("token")) return;
  void adminWorkspaceApi.recordExperience(eventType, moduleId, targetId, metadata).catch(() => undefined);
}
