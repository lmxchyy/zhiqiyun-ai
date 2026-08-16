<template>
  <section class="ppt-agent-outline-review" aria-label="大纲审阅">
    <header class="ppt-agent-outline-heading">
      <div>
        <span>规划结果</span>
        <h2>{{ state.outline.topic }}</h2>
        <p>{{ state.storyline.thesis }}</p>
      </div>
      <strong>{{ state.outline.pageCount }} 页</strong>
    </header>

    <div class="ppt-agent-storyline">
      <p><b>希望听众记住：</b>{{ state.storyline.audienceTakeaway }}</p>
      <p><b>收尾行动：</b>{{ state.storyline.closingAction }}</p>
    </div>

    <ol class="ppt-agent-slide-list">
      <li
        v-for="(slide, index) in state.outline.slides"
        :key="slide.slideId"
        class="ppt-agent-slide-card"
        :data-slide-id="slide.slideId"
      >
        <header>
          <span>第 {{ index + 1 }} 页</span>
          <div class="ppt-agent-slide-actions">
            <button type="button" data-action="move-up" :disabled="busy || index === 0" @click="move(slide.slideId, index)">上移</button>
            <button type="button" data-action="move-down" :disabled="busy || index === state.outline.slides.length - 1" @click="move(slide.slideId, index + 2)">下移</button>
            <button type="button" data-action="delete" :disabled="busy || state.outline.pageCount <= 6" @click="remove(slide.slideId)">删除</button>
          </div>
        </header>
        <label>
          <span>标题</span>
          <input :value="slide.title" data-field="title" :disabled="busy" @change="updateField(slide, 'title', $event)" />
        </label>
        <label>
          <span>页面目标</span>
          <textarea :value="slide.purpose" data-field="purpose" rows="2" :disabled="busy" @change="updateField(slide, 'purpose', $event)"></textarea>
        </label>
        <label>
          <span>核心信息</span>
          <textarea :value="slide.keyMessage" data-field="keyMessage" rows="2" :disabled="busy" @change="updateField(slide, 'keyMessage', $event)"></textarea>
        </label>
        <div class="ppt-agent-visual-intent">
          <span>视觉意图</span>
          <p>{{ slide.visualIntent }}</p>
        </div>

        <section v-if="slide.evidence.length" class="ppt-agent-evidence" :aria-label="`第 ${index + 1} 页证据`">
          <header>
            <strong>事实依据</strong>
            <span>支持第 {{ index + 1 }} 页</span>
          </header>
          <article v-for="assignment in slide.evidence" :key="assignment.claimId">
            <template v-if="evidenceDetails(assignment.claimId)">
              <div class="ppt-agent-evidence-source">
                <strong>{{ evidenceDetails(assignment.claimId)?.source.title }}</strong>
                <span>{{ evidenceDetails(assignment.claimId)?.source.type }}</span>
                <span>{{ evidenceDetails(assignment.claimId)?.claim.verificationStatus }}</span>
              </div>
              <p>{{ evidenceDetails(assignment.claimId)?.claim.text }}</p>
              <p class="ppt-agent-evidence-rationale"><b>为什么支持本页：</b>{{ assignment.rationale }}</p>
              <a
                v-for="citation in evidenceDetails(assignment.claimId)?.citations"
                :key="citation.id"
                :href="citation.locator"
                target="_blank"
                rel="noreferrer"
              >{{ citation.locator }}</a>
            </template>
          </article>
        </section>
        <p v-else class="ppt-agent-no-evidence">本页为结构或行动页面，不依赖事实型证据。</p>
      </li>
    </ol>

    <button
      type="button"
      class="ppt-agent-add-slide"
      data-action="add-slide"
      :disabled="busy || state.outline.pageCount >= 12"
      @click="addSlide"
    >添加一页
    </button>
  </section>
</template>

<script setup lang="ts">
import type { AgentPlanningState, OutlineEditCommand, ResearchCitation, ResearchClaim, ResearchSource, SlideObjective } from "../../types/pptAgent";

const props = defineProps<{ state: AgentPlanningState; busy: boolean }>();
const emit = defineEmits<{ commands: [commands: OutlineEditCommand[]] }>();

function evidenceDetails(claimId: string): { claim: ResearchClaim; source: ResearchSource; citations: ResearchCitation[] } | undefined {
  const claim = props.state.research.claims.find(item => item.id === claimId);
  if (!claim) return undefined;
  const source = props.state.research.sources.find(item => item.id === claim.sourceId);
  if (!source) return undefined;
  const refs = new Set(claim.citationRefs);
  return { claim, source, citations: props.state.research.citations.filter(item => refs.has(item.id) && item.sourceId === source.id) };
}

function eventValue(event: Event) {
  return String((event.target as HTMLInputElement | HTMLTextAreaElement | null)?.value || "").trim();
}

function updateField(slide: SlideObjective, field: "title" | "purpose" | "keyMessage", event: Event) {
  const value = eventValue(event);
  if (!value || value === slide[field]) return;
  emit("commands", [{ type: "UPDATE_SLIDE_OBJECTIVE", slideId: slide.slideId, objective: { ...slide, [field]: value } }]);
}

function move(slideId: string, toIndex: number) {
  emit("commands", [{ type: "MOVE_SLIDE", slideId, toIndex }]);
}

function remove(slideId: string) {
  emit("commands", [{ type: "DELETE_SLIDE", slideId }]);
}

function addSlide() {
  const english = props.state.intent.language === "en-US";
  const afterSlideId = props.state.outline.slides.at(-1)?.slideId;
  emit("commands", [{
    type: "ADD_SLIDE",
    afterSlideId,
    objective: {
      slideId: "new",
      title: english ? "New slide" : "新增页面",
      purpose: english ? "Clarify the next decision in the storyline." : "补充叙事中的下一项管理决策。",
      keyMessage: english ? "Define the key message for this page." : "请明确这一页希望听众记住的信息。",
      evidenceRequired: false,
      evidenceRefs: [],
      evidence: [],
      visualIntent: english ? "A clear professional content structure" : "清晰的专业内容结构",
      expectedElementTypes: ["TEXT", "SHAPE"]
    }
  }]);
}
</script>

<style scoped>
.ppt-agent-outline-review { display: grid; gap: 18px; }
.ppt-agent-outline-heading, .ppt-agent-slide-card > header, .ppt-agent-evidence > header, .ppt-agent-evidence-source { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.ppt-agent-outline-heading h2 { margin: 4px 0; font-size: 24px; }
.ppt-agent-outline-heading p, .ppt-agent-storyline p, .ppt-agent-visual-intent p { margin: 4px 0; color: #526071; }
.ppt-agent-storyline { padding: 14px 16px; border-radius: 14px; background: #f5f7fb; }
.ppt-agent-slide-list { display: grid; gap: 14px; margin: 0; padding: 0; list-style: none; }
.ppt-agent-slide-card { display: grid; gap: 12px; padding: 18px; border: 1px solid #e1e6ef; border-radius: 16px; background: #fff; }
.ppt-agent-slide-card label { display: grid; gap: 6px; font-size: 13px; color: #526071; }
.ppt-agent-slide-card input, .ppt-agent-slide-card textarea { width: 100%; box-sizing: border-box; border: 1px solid #d6dce7; border-radius: 9px; padding: 9px 11px; color: #172033; background: #fff; font: inherit; }
.ppt-agent-slide-actions { display: flex; gap: 6px; }
.ppt-agent-slide-actions button, .ppt-agent-add-slide { border: 1px solid #d6dce7; border-radius: 8px; padding: 7px 10px; background: #fff; cursor: pointer; }
.ppt-agent-slide-actions button:disabled, .ppt-agent-add-slide:disabled { opacity: .45; cursor: not-allowed; }
.ppt-agent-evidence { display: grid; gap: 10px; padding: 13px; border-radius: 12px; background: #f7faf8; border: 1px solid #dce9e0; }
.ppt-agent-evidence article { display: grid; gap: 6px; padding-top: 9px; border-top: 1px solid #dce9e0; }
.ppt-agent-evidence article:first-of-type { border-top: 0; }
.ppt-agent-evidence-source { justify-content: flex-start; flex-wrap: wrap; }
.ppt-agent-evidence-source span { padding: 2px 7px; border-radius: 999px; background: #e8f2eb; color: #376047; font-size: 11px; }
.ppt-agent-evidence p { margin: 0; }
.ppt-agent-evidence-rationale { color: #526071; }
.ppt-agent-evidence a { color: #315eaa; overflow-wrap: anywhere; }
.ppt-agent-no-evidence { margin: 0; color: #7a8494; font-size: 13px; }
.ppt-agent-add-slide { justify-self: start; }
</style>
