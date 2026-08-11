import { api } from "../../api/client";

export type InspirationEventType = "copy_prompt" | "use_template" | "generate_success";

export function recordInspirationEvent(slug: string, eventType: InspirationEventType, generationTaskId = "") {
  return api(`/api/v1/inspirations/${encodeURIComponent(slug)}/events`, {
    method: "POST",
    body: JSON.stringify({ eventType, generationTaskId, platform: "miniprogram" }),
  });
}
