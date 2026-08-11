import type { ApiClient } from "@xianzhi/api-client";
import type { CreateDraft, CreateGenerationTaskRequest, GenerationTask } from "@xianzhi/shared-types";
import { taskRequestFromDraft } from "./mappers";
import type { BusinessSdk, PagedItems, TaskPageOptions, VideoGenerationEstimate } from "./types";

export function listTasks(api: ApiClient) {
  return api.request<GenerationTask[]>("/api/v1/generation-tasks", { auth: "required" });
}

export function listTaskPage(api: ApiClient, options: TaskPageOptions = {}) {
  const params = [
    "paged=true",
    `limit=${encodeURIComponent(String(options.limit || 20))}`,
    `offset=${encodeURIComponent(String(options.offset || 0))}`,
  ];
  if (options.prioritizeActive) params.push("priority=active");
  return api.request<PagedItems<GenerationTask>>(`/api/v1/generation-tasks?${params.join("&")}`, { auth: "required" });
}

export function createGenerationSdk(api: ApiClient): BusinessSdk["generation"] {
  return {
    createTask(draft: CreateDraft) {
      if (draft.mode === "ppt") {
        return api.request<GenerationTask, Record<string, unknown>>("/api/v1/ppt/generate", {
          method: "POST",
          body: {
            topic: draft.prompt,
            model: draft.model,
            pageCount: draft.count,
            style: draft.style,
            params: taskRequestFromDraft(draft).params
          },
          auth: "required",
          retryOnUnauthorized: false
        });
      }
      return api.request<GenerationTask, CreateGenerationTaskRequest>("/api/v1/generation-tasks", {
        method: "POST",
        body: taskRequestFromDraft(draft),
        auth: "required",
        retryOnUnauthorized: false
      });
    },
    estimateVideo(request: CreateGenerationTaskRequest) {
      return api.request<VideoGenerationEstimate, CreateGenerationTaskRequest>("/api/v1/generation-tasks/estimate", {
        method: "POST",
        body: request,
        auth: "required",
        retryOnUnauthorized: false
      });
    },
    listTasks: () => listTasks(api),
    listTaskPage: options => listTaskPage(api, options)
  };
}
