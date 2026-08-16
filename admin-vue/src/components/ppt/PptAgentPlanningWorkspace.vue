<template>
  <div class="ppt-agent-planning-workspace">
    <header class="ppt-agent-workspace-header">
      <button type="button" @click="emit('back')">返回修改需求</button>
      <div>
        <span>PPT Agent</span>
        <h1>{{ prompt }}</h1>
        <p>确认大纲后，任务会在后台生成完整演示文稿；关闭页面不会中断。</p>
      </div>
      <strong v-if="agent.state">{{ agent.stageLabel }}</strong>
    </header>

    <section class="ppt-agent-stage-track" aria-label="真实规划阶段">
      <article v-for="item in durableStages" :key="item.stage" :class="item.state">
        <span></span>
        <div><strong>{{ item.label }}</strong><small>{{ item.description }}</small></div>
      </article>
    </section>

    <section v-if="agent.requestError" class="ppt-agent-notice is-error" role="alert">
      <strong>暂时无法读取规划任务</strong>
      <p>{{ agent.requestError }}</p>
    </section>

    <section v-if="agent.clarificationQuestions.length" class="ppt-agent-notice">
      <strong>还需要确认一项关键信息</strong>
      <p v-for="question in agent.clarificationQuestions" :key="question">{{ question }}</p>
      <button type="button" @click="emit('back')">返回补充需求</button>
    </section>

    <section v-else-if="agent.canRetry" class="ppt-agent-notice is-error" role="alert">
      <strong>任务没有完成</strong>
      <p>{{ agent.failureMessage }}</p>
      <button type="button" :disabled="agent.busy" @click="agent.retry">重试当前阶段</button>
    </section>

    <section v-else-if="agent.isCompleted" class="ppt-agent-approved" role="status">
      <strong>演示文稿已完成</strong>
      <p>完整 PPTX 已保存到私有作品空间，可以下载并继续在 PowerPoint 中编辑。</p>
      <button type="button" :disabled="agent.busy" @click="agent.download">下载 PPTX</button>
    </section>

    <PptAgentOutlineReview
      v-else-if="agent.isWaitingForApproval && agent.state"
      :state="agent.state"
      :busy="agent.busy"
      @commands="agent.applyCommands"
    />

    <section v-else class="ppt-agent-planning-state" role="status" aria-live="polite">
      <span class="ppt-agent-spinner"></span>
      <div>
        <strong>{{ agent.state ? agent.stageLabel : "正在创建规划任务" }}</strong>
        <p>任务在后台持续执行，关闭页面不会中断；重新打开后会从已保存的阶段继续。</p>
      </div>
    </section>

    <footer v-if="agent.isWaitingForApproval" class="ppt-agent-workspace-footer">
      <span>确认后会保存不可变的大纲版本，并在后台生成完整 PPT。</span>
      <button type="button" :disabled="agent.busy" @click="agent.approve">确认大纲并生成</button>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue";
import { planningStageLabel, usePptAgentStore } from "../../stores/pptAgent";
import type { AgentGuideRequest, AgentPlanningStage } from "../../types/pptAgent";
import PptAgentOutlineReview from "./PptAgentOutlineReview.vue";

const props = defineProps<{
  prompt: string;
  pageCount?: number;
  language: "zh" | "en";
  audience?: string;
  scenario?: string;
  researchRequired?: boolean;
}>();
const emit = defineEmits<{ back: [] }>();
const agent = usePptAgentStore();

const stageOrder: AgentPlanningStage[] = ["CREATED", "INTENT_RESOLVED", "RESEARCHED", "STORYLINE_PLANNED", "OUTLINE_PLANNED", "OUTLINE_APPROVED", "CONTENT_READY", "ASSETS_READY", "LAYOUT_COMPILED", "QUALITY_CHECKED", "RENDERED", "FILE_STORED", "ASSET_CREATED", "TASK_RELATED", "COMPLETED"];
const durableStages = computed(() => {
  const current = agent.state?.job.stage || "CREATED";
  const currentIndex = stageOrder.indexOf(current as AgentPlanningStage);
  return stageOrder.map((stage, index) => ({
    stage,
    label: planningStageLabel(stage),
    description: stage === "OUTLINE_PLANNED" ? "可查看证据并编辑页面目标" : "以服务器持久化状态为准",
    state: agent.isCompleted || index < currentIndex ? "is-done" : index === currentIndex ? "is-active" : "is-pending"
  }));
});

function request(): AgentGuideRequest {
  const value: AgentGuideRequest = {
    idempotencyKey: typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : `ppt-agent-${Date.now()}`,
    text: props.prompt,
    language: props.language,
    professionalStyle: "professional-business"
  };
  if (props.pageCount && props.pageCount >= 6 && props.pageCount <= 12) value.pageCount = props.pageCount;
  if (props.audience && props.audience !== "auto") value.audience = props.audience;
  if (props.scenario && props.scenario !== "auto") value.scenario = props.scenario;
  if (props.researchRequired) value.researchRequired = true;
  return value;
}

onMounted(() => void agent.restoreOrStart(request()));
onBeforeUnmount(() => agent.stopPolling());
</script>

<style scoped>
.ppt-agent-planning-workspace { display: grid; gap: 20px; min-height: 100%; padding: 24px clamp(18px, 3vw, 44px) 100px; background: #f7f8fb; color: #172033; }
.ppt-agent-workspace-header { display: grid; grid-template-columns: auto 1fr auto; align-items: start; gap: 18px; padding: 20px; border-radius: 18px; background: #fff; border: 1px solid #e5e8ef; }
.ppt-agent-workspace-header button, .ppt-agent-notice button, .ppt-agent-workspace-footer button { border: 0; border-radius: 9px; padding: 9px 14px; cursor: pointer; }
.ppt-agent-workspace-header button { background: #eef1f6; }
.ppt-agent-workspace-header h1 { margin: 4px 0; font-size: 22px; }
.ppt-agent-workspace-header p { margin: 0; color: #697386; }
.ppt-agent-workspace-header > strong { color: #315eaa; }
.ppt-agent-stage-track { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 10px; }
.ppt-agent-stage-track article { display: flex; gap: 9px; min-height: 64px; padding: 12px; border-radius: 12px; background: #fff; border: 1px solid #e5e8ef; color: #8a93a3; }
.ppt-agent-stage-track article > span { width: 9px; height: 9px; margin-top: 5px; border-radius: 50%; background: #cbd2dc; }
.ppt-agent-stage-track article.is-active { border-color: #91aee0; color: #234e97; }
.ppt-agent-stage-track article.is-active > span { background: #315eaa; }
.ppt-agent-stage-track article.is-done { color: #356147; }
.ppt-agent-stage-track article.is-done > span { background: #4d8a65; }
.ppt-agent-stage-track small { display: block; margin-top: 4px; font-size: 11px; }
.ppt-agent-notice, .ppt-agent-approved, .ppt-agent-planning-state { padding: 18px; border-radius: 15px; background: #fff; border: 1px solid #e5e8ef; }
.ppt-agent-notice.is-error { border-color: #edc7c7; background: #fff8f8; }
.ppt-agent-notice p, .ppt-agent-approved p, .ppt-agent-planning-state p { margin: 6px 0 0; color: #697386; }
.ppt-agent-notice button { margin-top: 12px; background: #172033; color: #fff; }
.ppt-agent-approved { border-color: #c8e1d0; background: #f5fbf7; }
.ppt-agent-approved button { margin-top: 14px; border: 0; border-radius: 9px; padding: 10px 16px; background: #ff6b00; color: #fff; font-weight: 700; cursor: pointer; }
.ppt-agent-planning-state { display: flex; align-items: center; gap: 14px; }
.ppt-agent-spinner { width: 20px; height: 20px; border: 2px solid #d7deea; border-top-color: #315eaa; border-radius: 50%; animation: spin .8s linear infinite; }
.ppt-agent-workspace-footer { position: sticky; bottom: 14px; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 18px; border-radius: 15px; background: rgba(23, 32, 51, .96); color: #fff; box-shadow: 0 12px 28px rgba(23, 32, 51, .2); }
.ppt-agent-workspace-footer button { background: #ff6b00; color: #fff; font-weight: 700; }
.ppt-agent-workspace-footer button:disabled { opacity: .55; cursor: wait; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 900px) {
  .ppt-agent-workspace-header { grid-template-columns: 1fr; }
  .ppt-agent-stage-track { grid-template-columns: 1fr; }
  .ppt-agent-workspace-footer { align-items: stretch; flex-direction: column; }
}
</style>
