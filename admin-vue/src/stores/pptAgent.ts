import { defineStore } from "pinia";
import {
  approvePptAgentOutline,
  downloadPptAgentDeck,
  getPptAgentPreview,
  getPptAgentState,
  guidePptAgent,
  retryPptAgentPlanning,
  updatePptAgentOutline
} from "../api/ppt";
import type { AgentGuideRequest, AgentPlanningStage, AgentPlanningState, OutlineEditCommand, PptAgentPreviewProjection } from "../types/pptAgent";

const activePlanningStorageKey = "xianzhi_ppt_agent_planning_job";
let pollingTimer: ReturnType<typeof setTimeout> | undefined;

const stageLabels: Record<AgentPlanningStage, string> = {
  CREATED: "正在理解需求",
  INTENT_RESOLVED: "正在研究资料",
  RESEARCHED: "正在规划叙事",
  STORYLINE_PLANNED: "正在生成大纲",
  OUTLINE_PLANNED: "大纲已生成，请确认",
  OUTLINE_APPROVED: "正在生成内容",
  CONTENT_READY: "正在准备图片",
  ASSETS_READY: "正在排版",
  LAYOUT_COMPILED: "正在检查",
  QUALITY_CHECKED: "正在生成 PPTX",
  RENDERED: "正在保存文件",
  FILE_STORED: "正在创建作品",
  ASSET_CREATED: "正在关联项目",
  TASK_RELATED: "即将完成",
  COMPLETED: "演示文稿已完成"
};

const structuredFailureMessages: Record<string, string> = {
  planning_provider_unavailable: "规划服务暂时不可用，请稍后重试。",
  planning_timeout: "规划服务响应超时，请重试。",
  planning_invalid_output: "规划结果格式不正确，请重试。",
  planning_contract_validation_failed: "规划结果未通过质量校验，请重试。",
  planning_evidence_mapping_invalid: "大纲中的证据关系不完整，请重试。",
  research_provider_unavailable: "研究服务暂时不可用，请稍后重试。",
  research_timeout: "研究资料请求超时，请重试。",
  research_invalid_result: "没有获得可验证的研究结论，请重试。",
  research_contract_validation_failed: "研究资料未通过结构校验，请重试。",
  content_provider_unavailable: "内容生成服务暂时不可用，请稍后重试。",
  content_timeout: "页面内容生成超时，请重试。",
  content_invalid_output: "页面内容格式不正确，请重试。",
  content_contract_validation_failed: "页面内容未通过质量校验，请重试。",
  content_evidence_mapping_invalid: "页面内容与已批准证据不一致，请重试。",
  image_provider_unavailable: "图片服务暂时不可用，请稍后重试。",
  image_timeout: "图片生成超时，请重试。",
  image_invalid_result: "图片结果无效，请重试。",
  image_storage_failed: "图片保存失败，请重试。",
  layout_compilation_failed: "演示文稿排版失败，请重试。",
  quality_gate_failed: "演示文稿存在阻断问题，暂时无法导出。",
  pptx_render_failed: "PPTX 生成失败，请重试。",
  artifact_storage_failed: "演示文稿保存失败，请重试。",
  artifact_relation_failed: "演示文稿作品关联失败，请重试。"
};

export function planningStageLabel(stage: AgentPlanningStage | string) {
  return stageLabels[stage as AgentPlanningStage] || "正在准备规划";
}

export function planningProductMessage(stage: AgentPlanningStage | string) {
  return planningStageLabel(stage);
}

export function planningFailureMessage(state: AgentPlanningState | null) {
  const error = state?.job.error;
  if (!error) return "规划未完成，请重试。";
  return structuredFailureMessages[error.code] || error.message || "规划未完成，请重试。";
}

function browserStorage() {
  return typeof window === "undefined" ? undefined : window.localStorage;
}

function readStoredPlanning(prompt: string) {
  try {
    const raw = browserStorage()?.getItem(activePlanningStorageKey);
    if (!raw) return "";
    const saved = JSON.parse(raw) as { jobId?: string; prompt?: string };
    return saved.prompt === prompt ? String(saved.jobId || "") : "";
  } catch {
    return "";
  }
}

function saveStoredPlanning(jobId: string, prompt: string) {
  browserStorage()?.setItem(activePlanningStorageKey, JSON.stringify({ jobId, prompt }));
}

export const usePptAgentStore = defineStore("pptAgent", {
  state: () => ({
    state: null as AgentPlanningState | null,
    prompt: "",
    clarificationQuestions: [] as string[],
    busy: false,
    requestError: "",
    preview: null as PptAgentPreviewProjection | null,
    previewLoading: false,
    previewError: ""
  }),
  getters: {
    stageLabel: state => planningStageLabel(state.state?.job.stage || ""),
    isPlanning: state => ["QUEUED", "RUNNING"].includes(state.state?.job.status || ""),
    isWaitingForApproval: state => state.state?.job.status === "WAITING_FOR_OUTLINE_APPROVAL",
    isApproved: state => Boolean(state.state?.approvedOutline),
    isCompleted: state => state.state?.job.status === "SUCCEEDED" && state.state?.job.stage === "COMPLETED",
    canRetry: state => Boolean(state.state?.job.error?.retryable && ["RETRY_WAIT", "FAILED"].includes(state.state.job.status)),
    failureMessage: state => planningFailureMessage(state.state)
  },
  actions: {
    async restoreOrStart(request: AgentGuideRequest) {
      const prompt = request.text.trim();
      if (!prompt) return;
      this.prompt = prompt;
      const jobId = readStoredPlanning(prompt);
      if (jobId) {
        try {
          this.state = await getPptAgentState(jobId);
          if (this.isCompleted) await this.loadPreview();
          this.startPolling();
          return;
        } catch {
          browserStorage()?.removeItem(activePlanningStorageKey);
        }
      }
      await this.start(request);
    },
    async start(request: AgentGuideRequest) {
      this.busy = true;
      this.requestError = "";
      this.clarificationQuestions = [];
      this.state = null;
      this.preview = null;
      this.previewError = "";
      try {
        const result = await guidePptAgent(request);
        this.clarificationQuestions = result.clarificationQuestions || [];
        this.state = result.state || null;
        if (this.state) {
          saveStoredPlanning(this.state.job.id, request.text.trim());
          if (this.isCompleted) await this.loadPreview();
          this.startPolling();
        }
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "无法创建规划任务，请稍后重试。";
      } finally {
        this.busy = false;
      }
    },
    async refresh() {
      if (!this.state?.job.id) return;
      try {
        this.state = await getPptAgentState(this.state.job.id);
        this.requestError = "";
        if (this.isCompleted) await this.loadPreview();
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "无法读取规划状态，请稍后重试。";
        this.stopPolling();
        return;
      }
      this.startPolling();
    },
    startPolling() {
      this.stopPolling();
      if (!this.isPlanning) return;
      pollingTimer = setTimeout(() => void this.refresh(), 1500);
    },
    stopPolling() {
      if (pollingTimer) clearTimeout(pollingTimer);
      pollingTimer = undefined;
    },
    async loadPreview(force = false) {
      const job = this.state?.job;
      if (!job?.id || !this.isCompleted || !job.revision) return;
      if (!force && this.preview?.deckId === job.deckId && this.preview?.revision === job.revision) return;
      this.previewLoading = true;
      this.previewError = "";
      try {
        this.preview = await getPptAgentPreview(job.id, job.revision);
      } catch (error) {
        this.previewError = error instanceof Error ? error.message : "预览暂时不可用，请稍后重试。";
      } finally {
        this.previewLoading = false;
      }
    },
    async refreshPreviewAssets() {
      await this.loadPreview(true);
    },
    async applyCommands(commands: OutlineEditCommand[]) {
      if (!this.state || !commands.length) return;
      this.busy = true;
      this.requestError = "";
      try {
        this.state = await updatePptAgentOutline(this.state.job.id, this.state.outline.revision, commands);
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "大纲更新失败，请刷新后重试。";
        await this.refresh();
      } finally {
        this.busy = false;
      }
    },
    async approve() {
      if (!this.state || !this.isWaitingForApproval) return;
      this.busy = true;
      this.requestError = "";
      try {
        this.state = await approvePptAgentOutline(this.state.job.id, this.state.outline.revision);
        this.startPolling();
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "确认大纲失败，请刷新后重试。";
        await this.refresh();
      } finally {
        this.busy = false;
      }
    },
    async retry() {
      if (!this.state?.job.id || !this.canRetry) return;
      this.busy = true;
      this.requestError = "";
      try {
        this.state = await retryPptAgentPlanning(this.state.job.id);
        this.startPolling();
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "重试失败，请稍后再试。";
      } finally {
        this.busy = false;
      }
    },
    async download() {
      if (!this.state?.job.id || !this.isCompleted) return;
      this.requestError = "";
      try {
        const ticket = await downloadPptAgentDeck(this.state.job.id);
        window.location.assign(ticket.url);
      } catch (error) {
        this.requestError = error instanceof Error ? error.message : "暂时无法下载 PPTX，请稍后重试。";
      }
    }
  }
});
