import { adminRequest } from "../api/client";

export type WebGuestExperienceEvent =
  | "guest_open_app"
  | "guest_view_home"
  | "guest_open_creator"
  | "guest_input_prompt"
  | "guest_click_generate"
  | "login_modal_show"
  | "login_start"
  | "login_success"
  | "login_cancel"
  | "pending_action_resume_success"
  | "pending_action_resume_failed"
  | "generation_success_after_login";

const safeMetadataKeys = new Set(["action", "authMethod", "module", "platform", "reason", "route"]);

function sanitizedMetadata(metadata: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(metadata).flatMap(([key, value]) => {
    if (!safeMetadataKeys.has(key) || typeof value !== "string") return [];
    const safeValue = value.trim().slice(0, 120);
    return safeValue ? [[key, safeValue]] : [];
  }));
}

export function trackWebGuestExperience(
  eventType: WebGuestExperienceEvent,
  moduleId = "",
  metadata: Record<string, unknown> = {}
) {
  void adminRequest<void>({
    method: "POST",
    url: "/public/experience-events",
    authMode: "none",
    retryOnUnauthorized: false,
    data: {
      eventType,
      moduleId: moduleId.trim().slice(0, 80),
      metadata: sanitizedMetadata({ platform: "web", ...metadata })
    }
  }).catch(() => undefined);
}
