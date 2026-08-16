import { defineStore } from "pinia";
import {
  approvePptAgentOutline,
  getPptAgentState,
  guidePptAgent,
  retryPptAgentPlanning,
  updatePptAgentOutline
} from "../api/ppt";
import type { AgentGuideRequest, AgentPlanningStage, AgentPlanningState, OutlineEditCommand } from "../types/pptAgent";

const activePlanningStorageKey = "xianzhi_ppt_agent_planning_job";
let pollingTimer: ReturnType<typeof setTimeout> | undefined;

const stageLabels: Record<AgentPlanningStage, string> = {
  CREATED: "正在理解需求",
  INTENT_RESOLVED: "正在研究资料",
  RESEARCHED: "正在规划叙事",
  STORYLINE_PLANNED: "正在生成大纲",
  OUTLINE_PLANNED: "大纲已生成，请确认",
  OUTLINE_APPROVED: "方案已确认，大纲已经安全保存"
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
  research_contract_validation_failed: "研究资料未通过结构校验，请重试。"
};

export function planningStageLabel(stage: AgentPlanningStage | string) {
  return stageLabels[stage as AgentPlanningStage] || "正在准备规划";
}

export function planningProductMessage(stage: AgentPlanningStage | string) {
  return stage === "OUTLINE_APPROVED" ? stageLabels.OUTLINE_APPROVED : planningStageLabel(stage);
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
    requestError: ""
  }),
  getters: {
    stageLabel: state => planningStageLabel(state.state?.job.stage || ""),
    isPlanning: state => ["QUEUED", "RUNNING"].includes(state.state?.job.status || ""),
    isWaitingForApproval: state => state.state?.job.status === "WAITING_FOR_OUTLINE_APPROVAL",
    isApproved: state => state.state?.job.stage === "OUTLINE_APPROVED",
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
      try {
        const result = await guidePptAgent(request);
        this.clarificationQuestions = result.clarificationQuestions || [];
        this.state = result.state || null;
        if (this.state) {
          saveStoredPlanning(this.state.job.id, request.text.trim());
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
        this.stopPolling();
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
    }
  }
});
