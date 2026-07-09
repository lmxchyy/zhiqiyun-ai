import type { ApiClient } from "@xianzhi/api-client";
import type { CreateDraft, CreateGenerationTaskRequest, GenerationTask } from "@xianzhi/shared-types";
import { taskRequestFromDraft } from "./mappers";
import type { BusinessSdk } from "./types";

export function listTasks(api: ApiClient) {
  return api.request<GenerationTask[]>("/api/v1/generation-tasks");
}

export function createGenerationSdk(api: ApiClient): BusinessSdk["generation"] {
  return {
    async createTask(draft: CreateDraft) {
      if (draft.mode === "ppt") {
        return api.request<GenerationTask, Record<string, unknown>>("/api/v1/ppt/generate", {
          method: "POST",
          body: {
            topic: draft.prompt,
            model: draft.model,
            pageCount: draft.count,
            style: draft.style,
            params: taskRequestFromDraft(draft).params
          }
        });
      }
      return api.request<GenerationTask, CreateGenerationTaskRequest>("/api/v1/generation-tasks", {
        method: "POST",
        body: taskRequestFromDraft(draft)
      });
    },
    listTasks: () => listTasks(api)
  };
}
