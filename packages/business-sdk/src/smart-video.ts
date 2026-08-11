import type { ApiClient, ApiRequestOptions } from "@xianzhi/api-client";
import type {
  CreateSmartVideoAssetInput,
  CreateSmartVideoExportInput,
  CreateSmartVideoPlanTaskInput,
  CreateSmartVideoProjectInput,
  ReorderSmartVideoAssetsInput,
  ReviseSmartVideoPlanInput,
  SmartVideoAnalysisSummary,
  SmartVideoPlanTask,
  SmartVideoProject,
  SmartVideoProjectAsset,
  SmartVideoProjectVersion,
  SmartVideoRenderQuote,
  SmartVideoRenderTask,
  UpdateSmartVideoProjectInput
} from "@xianzhi/shared-types";

function newIdempotencyKey(prefix = "sv") {
  const rand = Math.random().toString(36).slice(2, 10);
  return `${prefix}_${Date.now().toString(36)}_${rand}`;
}

function withIdempotency<TBody>(
  options: ApiRequestOptions<TBody> = {},
  key?: string
): ApiRequestOptions<TBody> {
  const idempotencyKey = (key || "").trim() || newIdempotencyKey();
  return {
    ...options,
    headers: {
      ...(options.headers || {}),
      "Idempotency-Key": idempotencyKey
    }
  };
}

export interface SmartVideoSdk {
  listProjects(): Promise<SmartVideoProject[]>;
  createProject(input: CreateSmartVideoProjectInput): Promise<SmartVideoProject>;
  getProject(projectId: string): Promise<SmartVideoProject>;
  updateProject(projectId: string, input: UpdateSmartVideoProjectInput): Promise<SmartVideoProject>;
  deleteProject(projectId: string): Promise<void>;

  listAssets(projectId: string): Promise<SmartVideoProjectAsset[]>;
  addAsset(projectId: string, input: CreateSmartVideoAssetInput): Promise<SmartVideoProjectAsset>;
  reorderAssets(projectId: string, input: ReorderSmartVideoAssetsInput): Promise<SmartVideoProjectAsset[]>;
  deleteAsset(projectId: string, assetId: string): Promise<void>;

  analyze(projectId: string, idempotencyKey?: string): Promise<SmartVideoAnalysisSummary>;
  getAnalysis(projectId: string): Promise<SmartVideoAnalysisSummary>;
  retryAnalysis(projectId: string, assetId: string): Promise<SmartVideoAnalysisSummary>;

  createPlanTask(projectId: string, input: CreateSmartVideoPlanTaskInput): Promise<SmartVideoPlanTask>;
  getPlanTask(projectId: string, taskId: string): Promise<SmartVideoPlanTask>;

  listVersions(projectId: string): Promise<SmartVideoProjectVersion[]>;
  getVersion(projectId: string, versionId: string): Promise<SmartVideoProjectVersion>;
  reviseVersion(projectId: string, versionId: string, input: ReviseSmartVideoPlanInput): Promise<SmartVideoProjectVersion>;
  confirmVersion(projectId: string, versionId: string): Promise<{ project: SmartVideoProject; confirmedVersion: SmartVideoProjectVersion }>;
  estimateRender(projectId: string, versionId: string): Promise<SmartVideoRenderQuote>;

  createExport(projectId: string, input: CreateSmartVideoExportInput): Promise<SmartVideoRenderTask>;
  getRenderTask(projectId: string, taskId: string): Promise<SmartVideoRenderTask>;
  cancelRenderTask(projectId: string, taskId: string): Promise<SmartVideoRenderTask>;
  retryRenderTask(projectId: string, taskId: string): Promise<SmartVideoRenderTask>;
}

export function createSmartVideoSdk(api: ApiClient): SmartVideoSdk {
  const base = (projectId: string) => `/api/v1/video-projects/${encodeURIComponent(projectId)}`;

  return {
    listProjects() {
      return api
        .request<{ items: SmartVideoProject[] }>("/api/v1/video-projects", { auth: "required" })
        .then(res => res.items || []);
    },
    createProject(input) {
      return api.request<SmartVideoProject, CreateSmartVideoProjectInput>("/api/v1/video-projects", {
        method: "POST",
        body: input,
        auth: "required"
      });
    },
    getProject(projectId) {
      return api.request<SmartVideoProject>(base(projectId), { auth: "required" });
    },
    updateProject(projectId, input) {
      return api.request<SmartVideoProject, UpdateSmartVideoProjectInput>(base(projectId), {
        method: "PATCH",
        body: input,
        auth: "required"
      });
    },
    deleteProject(projectId) {
      return api.request<void>(base(projectId), { method: "DELETE", auth: "required" });
    },

    listAssets(projectId) {
      return api
        .request<{ items: SmartVideoProjectAsset[] }>(`${base(projectId)}/assets`, { auth: "required" })
        .then(res => res.items || []);
    },
    addAsset(projectId, input) {
      return api.request<SmartVideoProjectAsset, CreateSmartVideoAssetInput>(`${base(projectId)}/assets`, {
        method: "POST",
        body: input,
        auth: "required"
      });
    },
    reorderAssets(projectId, input) {
      return api
        .request<{ items: SmartVideoProjectAsset[] }, ReorderSmartVideoAssetsInput>(`${base(projectId)}/assets/order`, {
          method: "PUT",
          body: input,
          auth: "required"
        })
        .then(res => res.items || []);
    },
    deleteAsset(projectId, assetId) {
      return api.request<void>(`${base(projectId)}/assets/${encodeURIComponent(assetId)}`, {
        method: "DELETE",
        auth: "required"
      });
    },

    analyze(projectId, idempotencyKey) {
      return api.request<SmartVideoAnalysisSummary>(`${base(projectId)}/analyze`, withIdempotency({
        method: "POST",
        auth: "required"
      }, idempotencyKey));
    },
    getAnalysis(projectId) {
      return api.request<SmartVideoAnalysisSummary>(`${base(projectId)}/analysis`, { auth: "required" });
    },
    retryAnalysis(projectId, assetId) {
      return api.request<SmartVideoAnalysisSummary>(
        `${base(projectId)}/assets/${encodeURIComponent(assetId)}/retry-analysis`,
        { method: "POST", auth: "required" }
      );
    },

    createPlanTask(projectId, input) {
      const key = input.idempotencyKey;
      const body = { ...input };
      delete (body as { idempotencyKey?: string }).idempotencyKey;
      return api.request<SmartVideoPlanTask, Omit<CreateSmartVideoPlanTaskInput, "idempotencyKey">>(
        `${base(projectId)}/plan-tasks`,
        withIdempotency({
          method: "POST",
          body: { ...body, idempotencyKey: key },
          auth: "required"
        }, key)
      );
    },
    getPlanTask(projectId, taskId) {
      return api.request<SmartVideoPlanTask>(
        `${base(projectId)}/plan-tasks/${encodeURIComponent(taskId)}`,
        { auth: "required" }
      );
    },

    listVersions(projectId) {
      return api
        .request<{ items: SmartVideoProjectVersion[] }>(`${base(projectId)}/versions`, { auth: "required" })
        .then(res => res.items || []);
    },
    getVersion(projectId, versionId) {
      return api.request<SmartVideoProjectVersion>(
        `${base(projectId)}/versions/${encodeURIComponent(versionId)}`,
        { auth: "required" }
      );
    },
    reviseVersion(projectId, versionId, input) {
      return api.request<SmartVideoProjectVersion, ReviseSmartVideoPlanInput>(
        `${base(projectId)}/versions/${encodeURIComponent(versionId)}/revisions`,
        { method: "POST", body: input, auth: "required" }
      );
    },
    confirmVersion(projectId, versionId) {
      return api.request<{ project: SmartVideoProject; confirmedVersion: SmartVideoProjectVersion }>(
        `${base(projectId)}/versions/${encodeURIComponent(versionId)}/confirm`,
        { method: "POST", auth: "required" }
      );
    },
    estimateRender(projectId, versionId) {
      return api.request<SmartVideoRenderQuote>(
        `${base(projectId)}/versions/${encodeURIComponent(versionId)}/render-estimate`,
        { auth: "required" }
      );
    },

    createExport(projectId, input) {
      return api.request<SmartVideoRenderTask, { versionId: string; idempotencyKey?: string }>(
        `${base(projectId)}/render-tasks`,
        withIdempotency({
          method: "POST",
          body: { versionId: input.versionId, idempotencyKey: input.idempotencyKey },
          auth: "required"
        }, input.idempotencyKey)
      );
    },
    getRenderTask(projectId, taskId) {
      return api.request<SmartVideoRenderTask>(
        `${base(projectId)}/render-tasks/${encodeURIComponent(taskId)}`,
        { auth: "required" }
      );
    },
    cancelRenderTask(projectId, taskId) {
      return api.request<SmartVideoRenderTask>(
        `${base(projectId)}/render-tasks/${encodeURIComponent(taskId)}/cancel`,
        { method: "POST", auth: "required" }
      );
    },
    retryRenderTask(projectId, taskId) {
      return api.request<SmartVideoRenderTask>(
        `${base(projectId)}/render-tasks/${encodeURIComponent(taskId)}/retry`,
        { method: "POST", auth: "required" }
      );
    }
  };
}
