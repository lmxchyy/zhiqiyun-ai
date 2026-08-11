import { computed, reactive, ref } from "vue";
import type {
  SmartVideoAnalysisSummary,
  SmartVideoEditPlanV1,
  SmartVideoPlanTask,
  SmartVideoProject,
  SmartVideoProjectAsset,
  SmartVideoProjectVersion,
  SmartVideoRenderQuote,
  SmartVideoRenderTask,
  SmartVideoSceneV1,
} from "@xianzhi/shared-types";
import {
  addSmartVideoAsset,
  analyzeSmartVideoProject,
  cancelSmartVideoRenderTask,
  confirmSmartVideoVersion,
  createSmartVideoExport,
  createSmartVideoPlanTask,
  createSmartVideoProject,
  estimateSmartVideoRender,
  getSmartVideoAnalysis,
  getSmartVideoPlanTask,
  getSmartVideoProject,
  getSmartVideoRenderTask,
  getSmartVideoVersion,
  listSmartVideoAssets,
  listSmartVideoVersions,
  reviseSmartVideoVersion,
  retrySmartVideoRenderTask,
  type LocalMediaPick,
  updateSmartVideoProject,
  uploadSmartVideoLocalFile,
} from "../api/smart-video";

export type SmartVideoFlowPhase =
  | "idle"
  | "draft"
  | "uploading"
  | "analyzing"
  | "planning"
  | "storyboard"
  | "quoting"
  | "rendering"
  | "completed"
  | "failed";

type UploadRow = {
  id: string;
  name: string;
  progress: number;
  status: "queued" | "uploading" | "completed" | "failed";
  error?: string;
};

const state = reactive({
  phase: "idle" as SmartVideoFlowPhase,
  project: null as SmartVideoProject | null,
  assets: [] as SmartVideoProjectAsset[],
  analysis: null as SmartVideoAnalysisSummary | null,
  versions: [] as SmartVideoProjectVersion[],
  currentVersion: null as SmartVideoProjectVersion | null,
  draftPlan: null as SmartVideoEditPlanV1 | null,
  planDirty: false,
  planTask: null as SmartVideoPlanTask | null,
  quote: null as SmartVideoRenderQuote | null,
  renderTask: null as SmartVideoRenderTask | null,
  uploads: [] as UploadRow[],
  title: "",
  requirement: "",
  busy: false,
  errorMessage: "",
});

const foreground = ref(true);
let pollTimer: ReturnType<typeof setTimeout> | null = null;
let pollToken = 0;
let activeUploadAbort: (() => Promise<void>) | null = null;

function clearPoll() {
  if (pollTimer) clearTimeout(pollTimer);
  pollTimer = null;
}

function schedulePoll(fn: () => void | Promise<void>, delayMs = 1500) {
  clearPoll();
  const token = pollToken;
  const delay = foreground.value ? delayMs : Math.max(delayMs, 8000);
  pollTimer = setTimeout(() => {
    if (token !== pollToken) return;
    void Promise.resolve(fn());
  }, delay);
}

function toErrorMessage(error: unknown) {
  if (error instanceof Error && error.message) return error.message;
  return "操作失败，请稍后重试";
}

function setError(error: unknown) {
  state.errorMessage = toErrorMessage(error);
  if (state.phase !== "idle") state.phase = "failed";
}

function clonePlan(plan: SmartVideoEditPlanV1): SmartVideoEditPlanV1 {
  return JSON.parse(JSON.stringify(plan)) as SmartVideoEditPlanV1;
}

function resolvePhase(project: SmartVideoProject): SmartVideoFlowPhase {
  const status = String(project.status || "").toUpperCase();
  if (status === "COMPLETED") return "completed";
  if (status === "FAILED") return "failed";
  if (status === "RENDERING") return "rendering";
  if (status === "CONFIRMED") return "quoting";
  if (status === "STORYBOARD_READY") return "storyboard";
  if (status === "PLANNING") return "planning";
  if (status === "ANALYZING") return "analyzing";
  return "draft";
}

async function reloadVersions(preferVersionId?: string) {
  if (!state.project) return;
  state.versions = await listSmartVideoVersions(state.project.id);
  state.project = await getSmartVideoProject(state.project.id);
  const versionId = preferVersionId || state.project.currentVersionId || state.versions[0]?.id;
  if (!versionId) {
    state.currentVersion = null;
    state.draftPlan = null;
    return;
  }
  state.currentVersion =
    state.versions.find((item) => item.id === versionId) ||
    (await getSmartVideoVersion(state.project.id, versionId));
  state.draftPlan = clonePlan(state.currentVersion.planSnapshot);
  state.planDirty = false;
}

async function pollAnalysis() {
  if (!state.project) return;
  try {
    state.analysis = await getSmartVideoAnalysis(state.project.id);
    state.assets = await listSmartVideoAssets(state.project.id);
    const status = String(state.analysis.status || "").toUpperCase();
    if (["READY", "SUCCEEDED", "COMPLETED", "MATERIAL_READY"].includes(status)) {
      state.phase = "draft";
      await startPlan();
      return;
    }
    if (["FAILED", "PARTIAL_FAILED"].includes(status)) {
      state.errorMessage = "部分素材分析失败，请返回检查后重试";
      state.phase = "failed";
      return;
    }
    schedulePoll(pollAnalysis, 1500);
  } catch (error) {
    setError(error);
  }
}

async function pollPlanTask(taskId: string) {
  if (!state.project) return;
  try {
    state.planTask = await getSmartVideoPlanTask(state.project.id, taskId);
    const planState = String(state.planTask.state || "").toUpperCase();
    if (planState === "SUCCEEDED") {
      await reloadVersions(state.planTask.outputVersionId);
      state.phase = "storyboard";
      return;
    }
    if (planState === "FAILED") {
      state.errorMessage = state.planTask.errorMessage || "方案生成失败";
      state.phase = "failed";
      return;
    }
    schedulePoll(() => pollPlanTask(taskId), 1500);
  } catch (error) {
    setError(error);
  }
}

async function pollRenderTask(taskId: string) {
  if (!state.project) return;
  try {
    state.renderTask = await getSmartVideoRenderTask(state.project.id, taskId);
    const status = String(state.renderTask.status || "").toUpperCase();
    if (status === "SUCCEEDED") {
      state.project = await getSmartVideoProject(state.project.id);
      state.phase = "completed";
      return;
    }
    if (status === "FAILED" || status === "CANCELLED") {
      state.errorMessage =
        state.renderTask.errorMessage || (status === "CANCELLED" ? "导出已取消" : "导出失败");
      state.phase = status === "CANCELLED" ? "quoting" : "failed";
      return;
    }
    schedulePoll(() => pollRenderTask(taskId), 1800);
  } catch (error) {
    setError(error);
  }
}

async function startPlan() {
  if (!state.project) return;
  state.busy = true;
  state.phase = "planning";
  state.errorMessage = "";
  try {
    state.planTask = await createSmartVideoPlanTask(state.project.id, {
      instruction: state.requirement.trim(),
    });
    schedulePoll(() => pollPlanTask(state.planTask!.id), 1200);
  } catch (error) {
    setError(error);
  } finally {
    state.busy = false;
  }
}

export function useSmartVideoProject() {
  function reset() {
    pollToken += 1;
    clearPoll();
    activeUploadAbort = null;
    state.phase = "idle";
    state.project = null;
    state.assets = [];
    state.analysis = null;
    state.versions = [];
    state.currentVersion = null;
    state.draftPlan = null;
    state.planDirty = false;
    state.planTask = null;
    state.quote = null;
    state.renderTask = null;
    state.uploads = [];
    state.title = "";
    state.requirement = "";
    state.busy = false;
    state.errorMessage = "";
  }

  async function ensureProject() {
    const title = state.title.trim() || "未命名混剪项目";
    const requirement = state.requirement.trim();
    if (!requirement) throw new Error("请填写混剪需求");
    if (!state.project) {
      state.project = await createSmartVideoProject({ title, requirement });
    } else {
      state.project = await updateSmartVideoProject(state.project.id, { title, requirement });
    }
    state.title = state.project.title;
    state.requirement = state.project.requirement;
    return state.project;
  }

  async function openProject(projectId: string) {
    if (!projectId) return;
    state.busy = true;
    state.errorMessage = "";
    pollToken += 1;
    clearPoll();
    try {
      const [project, assets, versions] = await Promise.all([
        getSmartVideoProject(projectId),
        listSmartVideoAssets(projectId),
        listSmartVideoVersions(projectId),
      ]);
      state.project = project;
      state.assets = assets;
      state.versions = versions;
      state.title = project.title;
      state.requirement = project.requirement;
      const versionId = project.currentVersionId || versions[0]?.id;
      if (versionId) {
        state.currentVersion =
          versions.find((item) => item.id === versionId) ||
          (await getSmartVideoVersion(projectId, versionId));
        state.draftPlan = clonePlan(state.currentVersion.planSnapshot);
        state.planDirty = false;
      } else {
        state.currentVersion = null;
        state.draftPlan = null;
        state.planDirty = false;
      }
      state.phase = resolvePhase(project);
      if (project.activePlanTaskId) schedulePoll(() => pollPlanTask(project.activePlanTaskId!), 800);
      if (project.activeRenderTaskId) schedulePoll(() => pollRenderTask(project.activeRenderTaskId!), 800);
      if (["ANALYZING", "MATERIAL_READY"].includes(String(project.status).toUpperCase())) {
        schedulePoll(pollAnalysis, 800);
      }
      if (state.phase === "quoting" || state.phase === "completed") {
        const confirmedId = project.confirmedVersionId || state.currentVersion?.id;
        if (confirmedId) {
          state.currentVersion =
            state.versions.find((item) => item.id === confirmedId) ||
            (await getSmartVideoVersion(projectId, confirmedId));
          state.draftPlan = clonePlan(state.currentVersion.planSnapshot);
        }
      }
    } catch (error) {
      setError(error);
    } finally {
      state.busy = false;
    }
  }

  async function uploadMedia(files: LocalMediaPick[]) {
    if (!files.length) throw new Error("请先选择素材");
    state.busy = true;
    state.errorMessage = "";
    try {
      await ensureProject();
      if (!state.project) return;
      state.phase = "uploading";
      for (const file of files) {
        const row: UploadRow = {
          id: `up_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
          name: file.name,
          progress: 0,
          status: "queued",
        };
        state.uploads.unshift(row);
        const handle = uploadSmartVideoLocalFile(file, {
          businessId: state.project.id,
          onProgress: (progress) => {
            row.status = progress.status === "failed" ? "failed" : "uploading";
            row.progress = progress.totalBytes
              ? Math.min(100, Math.round((progress.uploadedBytes / progress.totalBytes) * 100))
              : row.progress;
          },
        });
        activeUploadAbort = () => handle.abort();
        try {
          const uploaded = await handle.promise;
          row.progress = 100;
          row.status = "completed";
          const asset = await addSmartVideoAsset(state.project.id, {
            fileId: uploaded.file.fileId,
            assetType: file.assetType,
            sortOrder: state.assets.length,
          });
          state.assets = [...state.assets, asset];
        } catch (error) {
          row.status = "failed";
          row.error = toErrorMessage(error);
          throw error;
        } finally {
          activeUploadAbort = null;
        }
      }
      state.phase = "draft";
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function startAnalysis() {
    if (!state.assets.length) throw new Error("请先上传至少一个素材");
    state.busy = true;
    state.errorMessage = "";
    state.phase = "analyzing";
    try {
      await ensureProject();
      if (!state.project) return;
      state.analysis = await analyzeSmartVideoProject(state.project.id);
      schedulePoll(pollAnalysis, 1200);
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  function updateSceneLight(index: number, patch: Pick<SmartVideoSceneV1, "title" | "narration"> | Partial<SmartVideoSceneV1>) {
    if (!state.draftPlan) return;
    const scene = state.draftPlan.scenes[index];
    if (!scene) return;
    if (typeof patch.title === "string") scene.title = patch.title;
    if (typeof patch.narration === "string") scene.narration = patch.narration;
    state.planDirty = true;
  }

  function updatePlanTitle(title: string) {
    if (!state.draftPlan) return;
    state.draftPlan.title = title;
    state.planDirty = true;
  }

  async function saveRevision(changeNote = "轻量修订旁白与标题") {
    if (!state.project || !state.currentVersion || !state.draftPlan) {
      throw new Error("当前没有可保存的方案");
    }
    state.busy = true;
    state.errorMessage = "";
    try {
      const created = await reviseSmartVideoVersion(state.project.id, state.currentVersion.id, {
        plan: state.draftPlan,
        changeNote,
      });
      state.versions = [created, ...state.versions.filter((item) => item.id !== created.id)];
      state.currentVersion = created;
      state.draftPlan = clonePlan(created.planSnapshot);
      state.planDirty = false;
      state.project = await getSmartVideoProject(state.project.id);
      state.phase = "storyboard";
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function confirmPlan() {
    if (!state.project || !state.currentVersion) throw new Error("请先生成方案");
    if (state.planDirty) await saveRevision("确认前自动保存");
    state.busy = true;
    state.errorMessage = "";
    try {
      const result = await confirmSmartVideoVersion(state.project.id, state.currentVersion.id);
      state.project = result.project;
      state.currentVersion = result.confirmedVersion;
      state.draftPlan = clonePlan(result.confirmedVersion.planSnapshot);
      state.planDirty = false;
      state.versions = await listSmartVideoVersions(state.project.id);
      state.phase = "quoting";
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function loadQuote() {
    if (!state.project || !state.currentVersion) throw new Error("请先确认方案");
    state.busy = true;
    state.errorMessage = "";
    state.phase = "quoting";
    try {
      state.quote = await estimateSmartVideoRender(state.project.id, state.currentVersion.id);
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function startExport() {
    if (!state.project || !state.currentVersion) throw new Error("请先确认方案");
    if (!state.quote) await loadQuote();
    state.busy = true;
    state.errorMessage = "";
    state.phase = "rendering";
    try {
      state.renderTask = await createSmartVideoExport(state.project.id, {
        versionId: state.currentVersion.id,
      });
      schedulePoll(() => pollRenderTask(state.renderTask!.id), 1500);
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function cancelExport() {
    if (!state.project || !state.renderTask) return;
    state.busy = true;
    try {
      state.renderTask = await cancelSmartVideoRenderTask(state.project.id, state.renderTask.id);
      state.phase = "quoting";
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  async function retryExport() {
    if (!state.project || !state.renderTask) {
      await startExport();
      return;
    }
    state.busy = true;
    state.errorMessage = "";
    state.phase = "rendering";
    try {
      state.renderTask = await retrySmartVideoRenderTask(state.project.id, state.renderTask.id);
      schedulePoll(() => pollRenderTask(state.renderTask!.id), 1200);
    } catch (error) {
      setError(error);
      throw error;
    } finally {
      state.busy = false;
    }
  }

  function setForeground(visible: boolean) {
    foreground.value = visible;
    if (!visible) return;
    // Foreground resume: only refresh in-flight background polls.
    if (state.phase === "analyzing") schedulePoll(pollAnalysis, 200);
    else if (state.phase === "planning" && state.planTask?.id) {
      schedulePoll(() => pollPlanTask(state.planTask!.id), 200);
    } else if (state.phase === "rendering" && state.renderTask?.id) {
      schedulePoll(() => pollRenderTask(state.renderTask!.id), 200);
    }
  }

  return {
    state,
    phase: computed(() => state.phase),
    scenes: computed(() => state.draftPlan?.scenes || []),
    busy: computed(() => state.busy),
    errorMessage: computed(() => state.errorMessage),
    quotePoints: computed(() => state.quote?.points ?? 0),
    renderProgress: computed(() => Number(state.renderTask?.progress || 0)),
    reset,
    openProject,
    ensureProject,
    uploadMedia,
    startAnalysis,
    startPlan,
    updateSceneLight,
    updatePlanTitle,
    saveRevision,
    confirmPlan,
    loadQuote,
    startExport,
    cancelExport,
    retryExport,
    setForeground,
    clearError: () => {
      state.errorMessage = "";
    },
  };
}
