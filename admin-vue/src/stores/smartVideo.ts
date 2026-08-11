import { defineStore } from "pinia";
import type {
  SmartVideoAnalysisSummary,
  SmartVideoEditPlanV1,
  SmartVideoPlanTask,
  SmartVideoProject,
  SmartVideoProjectAsset,
  SmartVideoProjectVersion,
  SmartVideoRenderQuote,
  SmartVideoRenderTask,
  SmartVideoSceneV1
} from "@xianzhi/shared-types";
import { chineseAdminErrorMessage, createAdminApiError, AdminApiError } from "../api/client";
import {
  addSmartVideoAsset,
  analyzeSmartVideoProject,
  cancelSmartVideoRenderTask,
  confirmSmartVideoVersion,
  createSmartVideoExport,
  createSmartVideoPlanTask,
  createSmartVideoProject,
  deleteSmartVideoAsset,
  deleteSmartVideoProject,
  estimateSmartVideoRender,
  getSmartVideoAnalysis,
  getSmartVideoPlanTask,
  getSmartVideoProject,
  getSmartVideoRenderTask,
  getSmartVideoVersion,
  listSmartVideoAssets,
  listSmartVideoProjects,
  listSmartVideoVersions,
  reorderSmartVideoAssets,
  retrySmartVideoAssetAnalysis,
  retrySmartVideoRenderTask,
  reviseSmartVideoVersion,
  updateSmartVideoProject,
  uploadSmartVideoBlob,
  type MultipartUploadHandle,
  type MultipartUploadProgress
} from "../api/smartVideo";

export type SmartVideoWorkbenchPhase =
  | "list"
  | "draft"
  | "uploading"
  | "analyzing"
  | "planning"
  | "storyboard"
  | "confirmed"
  | "quoting"
  | "rendering"
  | "completed"
  | "failed";

export interface SmartVideoUploadItem {
  id: string;
  name: string;
  mimeType: string;
  size: number;
  progress: number;
  status: MultipartUploadProgress["status"] | "queued";
  error?: string;
  fileId?: string;
  assetId?: string;
}

interface SmartVideoState {
  phase: SmartVideoWorkbenchPhase;
  projects: SmartVideoProject[];
  project: SmartVideoProject | null;
  assets: SmartVideoProjectAsset[];
  analysis: SmartVideoAnalysisSummary | null;
  versions: SmartVideoProjectVersion[];
  currentVersion: SmartVideoProjectVersion | null;
  draftPlan: SmartVideoEditPlanV1 | null;
  planDirty: boolean;
  planTask: SmartVideoPlanTask | null;
  quote: SmartVideoRenderQuote | null;
  renderTask: SmartVideoRenderTask | null;
  uploads: SmartVideoUploadItem[];
  title: string;
  requirement: string;
  instruction: string;
  busy: boolean;
  errorMessage: string;
  successMessage: string;
  initialized: boolean;
  pollTimers: number[];
  activeUpload: MultipartUploadHandle | null;
}

function emptyPlan(): SmartVideoEditPlanV1 {
  return {
    schemaVersion: 1,
    title: "",
    summary: "",
    language: "zh-CN",
    target: { aspectRatio: "9:16", resolution: "1080p", durationMs: 15000 },
    voice: { enabled: true, modelKey: "", voiceKey: "", speed: 1 },
    subtitles: { enabled: true, preset: "clean", position: "bottom" },
    audio: { sourceGain: 0.2, voiceGain: 1 },
    scenes: []
  };
}

function clonePlan(plan: SmartVideoEditPlanV1): SmartVideoEditPlanV1 {
  return JSON.parse(JSON.stringify(plan)) as SmartVideoEditPlanV1;
}

function toErrorMessage(error: unknown) {
  if (error instanceof AdminApiError) return error.message;
  return chineseAdminErrorMessage(error instanceof Error ? error.message : error);
}

function assetTypeFromFile(file: File): "VIDEO" | "IMAGE" {
  if (file.type.startsWith("image/")) return "IMAGE";
  return "VIDEO";
}

export const useSmartVideoStore = defineStore("smartVideo", {
  state: (): SmartVideoState => ({
    phase: "list",
    projects: [],
    project: null,
    assets: [],
    analysis: null,
    versions: [],
    currentVersion: null,
    draftPlan: null,
    planDirty: false,
    planTask: null,
    quote: null,
    renderTask: null,
    uploads: [],
    title: "",
    requirement: "",
    instruction: "",
    busy: false,
    errorMessage: "",
    successMessage: "",
    initialized: false,
    pollTimers: [],
    activeUpload: null
  }),

  getters: {
    hasUnsavedChanges(state): boolean {
      return state.planDirty || state.uploads.some((item) => item.status === "uploading" || item.status === "paused");
    },
    statusText(state): string {
      const map: Record<SmartVideoWorkbenchPhase, string> = {
        list: "项目列表",
        draft: "编辑需求",
        uploading: "上传素材",
        analyzing: "分析素材",
        planning: "生成方案",
        storyboard: "编辑分镜",
        confirmed: "已确认方案",
        quoting: "估算积分",
        rendering: "导出成片",
        completed: "已完成",
        failed: "失败"
      };
      return map[state.phase];
    },
    sortedAssets(state): SmartVideoProjectAsset[] {
      return [...state.assets].sort((a, b) => (a.sortOrder ?? a.orderIndex ?? 0) - (b.sortOrder ?? b.orderIndex ?? 0));
    },
    confirmedVersion(state): SmartVideoProjectVersion | null {
      const id = state.project?.confirmedVersionId;
      if (!id) return null;
      return state.versions.find((item) => item.id === id) || null;
    }
  },

  actions: {
    clearError() {
      this.errorMessage = "";
    },

    clearSuccess() {
      this.successMessage = "";
    },

    setError(error: unknown) {
      this.successMessage = "";
      this.errorMessage = toErrorMessage(error);
      this.phase = this.phase === "list" ? "list" : "failed";
    },

    clearPolls() {
      for (const timer of this.pollTimers) window.clearTimeout(timer);
      this.pollTimers = [];
    },

    schedulePoll(fn: () => void | Promise<void>, delayMs = 1500) {
      const timer = window.setTimeout(() => {
        void Promise.resolve(fn());
      }, delayMs);
      this.pollTimers.push(timer);
    },

    resetWorkspace() {
      this.clearPolls();
      this.project = null;
      this.assets = [];
      this.analysis = null;
      this.versions = [];
      this.currentVersion = null;
      this.draftPlan = null;
      this.planDirty = false;
      this.planTask = null;
      this.quote = null;
      this.renderTask = null;
      this.uploads = [];
      this.title = "";
      this.requirement = "";
      this.instruction = "";
      this.activeUpload = null;
      this.busy = false;
      this.errorMessage = "";
      this.successMessage = "";
      this.phase = "list";
    },

    async initialize() {
      if (this.initialized && this.projects.length) return;
      this.busy = true;
      this.clearError();
      try {
        this.projects = await listSmartVideoProjects();
        this.initialized = true;
        this.phase = "list";
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    startCreate() {
      this.resetWorkspace();
      this.phase = "draft";
      this.title = "未命名混剪项目";
      this.requirement = "";
      this.draftPlan = emptyPlan();
    },

    async openProject(projectId: string) {
      this.busy = true;
      this.clearError();
      this.clearPolls();
      try {
        const [project, assets, versions] = await Promise.all([
          getSmartVideoProject(projectId),
          listSmartVideoAssets(projectId),
          listSmartVideoVersions(projectId)
        ]);
        this.project = project;
        this.assets = assets;
        this.versions = versions;
        this.title = project.title;
        this.requirement = project.requirement;
        const versionId = project.currentVersionId || versions[0]?.id;
        if (versionId) {
          const version = versions.find((item) => item.id === versionId) || (await getSmartVideoVersion(projectId, versionId));
          this.currentVersion = version;
          this.draftPlan = clonePlan(version.planSnapshot);
          this.planDirty = false;
        } else {
          this.currentVersion = null;
          this.draftPlan = emptyPlan();
          this.planDirty = false;
        }
        this.phase = this.resolvePhaseFromProject(project);
        if (project.activePlanTaskId) {
          void this.pollPlanTask(project.activePlanTaskId);
        }
        if (project.activeRenderTaskId) {
          void this.pollRenderTask(project.activeRenderTaskId);
        }
        if (["ANALYZING", "MATERIAL_READY"].includes(String(project.status).toUpperCase())) {
          void this.refreshAnalysis();
        }
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    resolvePhaseFromProject(project: SmartVideoProject): SmartVideoWorkbenchPhase {
      const status = String(project.status || "").toUpperCase();
      if (status === "COMPLETED") return "completed";
      if (status === "FAILED") return "failed";
      if (status === "RENDERING") return "rendering";
      if (status === "CONFIRMED") return "confirmed";
      if (status === "STORYBOARD_READY") return "storyboard";
      if (status === "PLANNING") return "planning";
      if (status === "ANALYZING") return "analyzing";
      if (this.assets.length) return "draft";
      return "draft";
    },

    async saveProjectMeta() {
      if (!this.title.trim()) {
        this.errorMessage = "请填写项目标题";
        return;
      }
      this.busy = true;
      this.clearError();
      try {
        if (!this.project) {
          this.project = await createSmartVideoProject({
            title: this.title.trim(),
            requirement: this.requirement.trim()
          });
          this.projects = [this.project, ...this.projects.filter((item) => item.id !== this.project?.id)];
        } else {
          this.project = await updateSmartVideoProject(this.project.id, {
            title: this.title.trim(),
            requirement: this.requirement.trim()
          });
        }
        this.phase = "draft";
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async removeProject(projectId: string) {
      this.busy = true;
      this.clearError();
      try {
        await deleteSmartVideoProject(projectId);
        this.projects = this.projects.filter((item) => item.id !== projectId);
        if (this.project?.id === projectId) this.resetWorkspace();
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async uploadFiles(files: FileList | File[]) {
      const list = Array.from(files || []);
      if (!list.length) return;
      if (!this.project) {
        await this.saveProjectMeta();
        if (!this.project) return;
      }
      this.phase = "uploading";
      this.clearError();
      for (const file of list) {
        const uploadId = `up_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
        const item: SmartVideoUploadItem = {
          id: uploadId,
          name: file.name,
          mimeType: file.type || "application/octet-stream",
          size: file.size,
          progress: 0,
          status: "queued"
        };
        this.uploads.unshift(item);
        const handle = uploadSmartVideoBlob(file, {
          businessId: this.project.id,
          onProgress: (progress) => {
            const target = this.uploads.find((row) => row.id === uploadId);
            if (!target) return;
            target.status = progress.status;
            target.progress = progress.totalBytes
              ? Math.min(100, Math.round((progress.uploadedBytes / progress.totalBytes) * 100))
              : 0;
          }
        });
        this.activeUpload = handle;
        try {
          const uploaded = await handle.promise;
          item.fileId = uploaded.fileId;
          item.status = "completed";
          item.progress = 100;
          const asset = await addSmartVideoAsset(this.project.id, {
            fileId: uploaded.fileId,
            assetType: assetTypeFromFile(file),
            sortOrder: this.assets.length
          });
          item.assetId = asset.id;
          this.assets = [...this.assets, asset];
        } catch (error) {
          item.status = "failed";
          item.error = toErrorMessage(error);
          this.errorMessage = item.error;
        } finally {
          if (this.activeUpload === handle) this.activeUpload = null;
        }
      }
      this.phase = "draft";
    },

    pauseUpload() {
      this.activeUpload?.pause();
    },

    resumeUpload() {
      this.activeUpload?.resume();
    },

    async cancelUpload() {
      if (this.activeUpload) await this.activeUpload.abort();
    },

    async removeAsset(assetId: string) {
      if (!this.project) return;
      this.busy = true;
      this.clearError();
      try {
        await deleteSmartVideoAsset(this.project.id, assetId);
        this.assets = this.assets.filter((item) => item.id !== assetId);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async reorderAssets(assetIds: string[]) {
      if (!this.project) return;
      this.busy = true;
      this.clearError();
      try {
        this.assets = await reorderSmartVideoAssets(this.project.id, { assetIds });
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async startAnalysis() {
      if (!this.project) {
        await this.saveProjectMeta();
        if (!this.project) return;
      }
      if (!this.assets.length) {
        this.errorMessage = "请先上传素材；分析至少需要 2 个已成功入库的素材";
        return;
      }
      if (this.assets.length < 2) {
        this.errorMessage = `当前仅有 ${this.assets.length} 个已成功素材，分析至少需要 2 个。请去掉上传失败项后，再补传图片或视频。`;
        return;
      }
      if (this.uploads.some((item) => item.status === "uploading" || item.status === "queued" || item.status === "paused")) {
        this.errorMessage = "仍有素材正在上传，请等待完成或取消失败项后再分析";
        return;
      }
      this.busy = true;
      this.clearError();
      this.phase = "analyzing";
      try {
        this.analysis = await analyzeSmartVideoProject(this.project.id);
        this.schedulePoll(() => this.pollAnalysis(), 1200);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async refreshAnalysis() {
      if (!this.project) return;
      try {
        this.analysis = await getSmartVideoAnalysis(this.project.id);
        this.assets = await listSmartVideoAssets(this.project.id);
        const status = String(this.analysis.overallStatus || this.analysis.status || "").toUpperCase();
        if (["READY", "SUCCEEDED", "COMPLETED", "MATERIAL_READY"].includes(status)) {
          this.phase = "draft";
          return;
        }
        if (["FAILED", "PARTIAL_FAILED"].includes(status) && (this.analysis.failedCount || 0) > 0) {
          this.phase = "analyzing";
          return;
        }
        this.schedulePoll(() => this.pollAnalysis(), 1500);
      } catch (error) {
        this.setError(error);
      }
    },

    async pollAnalysis() {
      await this.refreshAnalysis();
    },

    async retryAssetAnalysis(assetId: string) {
      if (!this.project) return;
      this.busy = true;
      this.clearError();
      try {
        this.analysis = await retrySmartVideoAssetAnalysis(this.project.id, assetId);
        this.phase = "analyzing";
        this.schedulePoll(() => this.pollAnalysis(), 1200);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async generatePlan() {
      if (!this.project) {
        await this.saveProjectMeta();
        if (!this.project) return;
      }
      if (this.assets.length < 2) {
        this.errorMessage = `生成方案至少需要 2 个已成功素材，当前仅有 ${this.assets.length} 个`;
        return;
      }
      const succeeded = this.assets.filter((item) => String(item.analysisStatus || "").toUpperCase() === "SUCCEEDED").length;
      if (succeeded < 2) {
        this.errorMessage = "请先完成素材分析（至少 2 个就绪）后再生成方案";
        return;
      }
      this.busy = true;
      this.clearError();
      this.phase = "planning";
      try {
        // Refresh analysis to reconcile MATERIAL_READY before planning.
        await this.refreshAnalysis();
        this.project = await getSmartVideoProject(this.project.id);
        this.planTask = await createSmartVideoPlanTask(this.project.id, {
          instruction: this.instruction.trim() || this.requirement.trim()
        });
        this.schedulePoll(() => this.pollPlanTask(this.planTask!.id), 1200);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async pollPlanTask(taskId: string) {
      if (!this.project) return;
      try {
        this.planTask = await getSmartVideoPlanTask(this.project.id, taskId);
        const state = String(this.planTask.state || "").toUpperCase();
        if (state === "SUCCEEDED") {
          await this.reloadVersions(this.planTask.outputVersionId);
          this.phase = "storyboard";
          return;
        }
        if (state === "FAILED") {
          this.errorMessage = toErrorMessage(createAdminApiError({
            error: this.planTask.errorMessage || "方案生成失败",
            code: this.planTask.errorCode || "provider_unavailable"
          }));
          this.phase = "failed";
          return;
        }
        this.schedulePoll(() => this.pollPlanTask(taskId), 1500);
      } catch (error) {
        this.setError(error);
      }
    },

    async reloadVersions(preferVersionId?: string) {
      if (!this.project) return;
      this.versions = await listSmartVideoVersions(this.project.id);
      this.project = await getSmartVideoProject(this.project.id);
      const versionId = preferVersionId || this.project.currentVersionId || this.versions[0]?.id;
      if (!versionId) return;
      this.currentVersion = this.versions.find((item) => item.id === versionId)
        || await getSmartVideoVersion(this.project.id, versionId);
      this.draftPlan = clonePlan(this.currentVersion.planSnapshot);
      this.planDirty = false;
    },

    updateDraftPlan(mutator: (plan: SmartVideoEditPlanV1) => void) {
      if (!this.draftPlan) this.draftPlan = emptyPlan();
      mutator(this.draftPlan);
      this.planDirty = true;
    },

    updateScene(index: number, patch: Partial<SmartVideoSceneV1>) {
      this.updateDraftPlan((plan) => {
        const scene = plan.scenes[index];
        if (!scene) return;
        Object.assign(scene, patch);
      });
    },

    async saveRevision(changeNote = "手动修订") {
      if (!this.project || !this.currentVersion || !this.draftPlan) {
        this.errorMessage = "当前没有可保存的方案版本";
        return;
      }
      this.busy = true;
      this.clearError();
      try {
        const created = await reviseSmartVideoVersion(this.project.id, this.currentVersion.id, {
          plan: this.draftPlan,
          changeNote
        });
        this.versions = [created, ...this.versions.filter((item) => item.id !== created.id)];
        this.currentVersion = created;
        this.draftPlan = clonePlan(created.planSnapshot);
        this.planDirty = false;
        this.project = await getSmartVideoProject(this.project.id);
        this.phase = "storyboard";
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async confirmCurrentVersion() {
      if (!this.project) return;
      if (!this.currentVersion) {
        this.errorMessage = "请先成功生成方案后再确认";
        return;
      }
      if (this.project.confirmedVersionId === this.currentVersion.id) {
        this.clearError();
        this.successMessage = "方案已确认，请下滑到「导出」估算积分并提交成片";
        this.phase = this.resolvePhaseFromProject(this.project);
        if (!this.quote) {
          try {
            await this.loadQuote();
          } catch {
            // keep confirm success even if quote temporarily fails
          }
        }
        return;
      }
      if (this.planDirty) {
        await this.saveRevision("确认前自动保存");
        if (this.planDirty) return;
      }
      this.busy = true;
      this.clearError();
      this.clearSuccess();
      try {
        const result = await confirmSmartVideoVersion(this.project.id, this.currentVersion.id);
        this.project = result.project;
        this.currentVersion = result.confirmedVersion;
        this.draftPlan = clonePlan(result.confirmedVersion.planSnapshot);
        this.planDirty = false;
        this.versions = await listSmartVideoVersions(this.project.id);
        this.phase = "confirmed";
        this.successMessage = "方案已确认，请下滑到「导出」估算积分并提交成片";
        try {
          await this.loadQuote();
        } catch {
          // confirm already succeeded
        }
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async loadQuote() {
      if (!this.project || !this.currentVersion) {
        this.errorMessage = "请先确认方案后再估算积分";
        return;
      }
      const nestedBusy = this.busy;
      if (!nestedBusy) this.busy = true;
      this.clearError();
      if (this.phase === "confirmed" || this.phase === "storyboard") {
        this.phase = "quoting";
      }
      try {
        this.quote = await estimateSmartVideoRender(this.project.id, this.currentVersion.id);
        if (this.phase === "quoting") this.phase = "confirmed";
      } catch (error) {
        this.setError(error);
      } finally {
        if (!nestedBusy) this.busy = false;
      }
    },

    async startExport() {
      if (!this.project || !this.currentVersion) {
        this.errorMessage = "请先确认方案后再提交导出";
        return;
      }
      if (this.project.confirmedVersionId !== this.currentVersion.id) {
        this.errorMessage = "请先点击「确认方案」后再提交导出";
        return;
      }
      if (!this.quote) await this.loadQuote();
      if (!this.quote) return;
      this.busy = true;
      this.clearError();
      this.clearSuccess();
      this.phase = "rendering";
      try {
        this.renderTask = await createSmartVideoExport(this.project.id, {
          versionId: this.currentVersion.id
        });
        this.successMessage = "导出任务已提交，正在渲染成片";
        this.schedulePoll(() => this.pollRenderTask(this.renderTask!.id), 1500);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async pollRenderTask(taskId: string) {
      if (!this.project) return;
      try {
        this.renderTask = await getSmartVideoRenderTask(this.project.id, taskId);
        const status = String(this.renderTask.status || "").toUpperCase();
        if (status === "SUCCEEDED") {
          this.project = await getSmartVideoProject(this.project.id);
          this.phase = "completed";
          return;
        }
        if (status === "FAILED" || status === "CANCELLED") {
          this.errorMessage = this.renderTask.errorMessage || (status === "CANCELLED" ? "导出已取消" : "导出失败");
          this.phase = status === "CANCELLED" ? "confirmed" : "failed";
          return;
        }
        this.schedulePoll(() => this.pollRenderTask(taskId), 1800);
      } catch (error) {
        this.setError(error);
      }
    },

    async cancelExport() {
      if (!this.project || !this.renderTask) return;
      this.busy = true;
      try {
        this.renderTask = await cancelSmartVideoRenderTask(this.project.id, this.renderTask.id);
        this.phase = "confirmed";
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    async retryExport() {
      if (!this.project || !this.renderTask) return;
      this.busy = true;
      this.clearError();
      this.phase = "rendering";
      try {
        this.renderTask = await retrySmartVideoRenderTask(this.project.id, this.renderTask.id);
        this.schedulePoll(() => this.pollRenderTask(this.renderTask!.id), 1500);
      } catch (error) {
        this.setError(error);
      } finally {
        this.busy = false;
      }
    },

    selectVersion(versionId: string) {
      const version = this.versions.find((item) => item.id === versionId);
      if (!version) return;
      if (this.planDirty && !window.confirm("当前方案有未保存修改，切换版本将丢弃修改，是否继续？")) {
        return;
      }
      this.currentVersion = version;
      this.draftPlan = clonePlan(version.planSnapshot);
      this.planDirty = false;
      this.phase = this.project?.confirmedVersionId === version.id ? "confirmed" : "storyboard";
    },

    dispose() {
      this.clearPolls();
      void this.cancelUpload();
    }
  }
});

export function smartVideoErrorFromUnknown(error: unknown) {
  if (error instanceof AdminApiError) return error;
  return createAdminApiError(error);
}
