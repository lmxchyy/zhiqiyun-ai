<template>
  <section class="sv-card" aria-label="分镜方案">
    <div class="sv-head">
      <div>
        <h2>分镜与旁白</h2>
        <p class="sv-muted">
          {{ store.draftPlan?.title || "尚未生成方案" }}
          <template v-if="store.planDirty"> · 有未保存修改</template>
        </p>
      </div>
      <div class="sv-inline">
        <button type="button" class="sv-btn" :disabled="store.busy || !store.draftPlan?.scenes?.length || !store.planDirty" @click="store.saveRevision()">
          保存为新版本
        </button>
        <button type="button" class="sv-btn primary" :disabled="store.busy || !store.currentVersion" @click="store.confirmCurrentVersion()">
          确认方案
        </button>
      </div>
    </div>

    <div v-if="store.draftPlan" class="sv-plan-meta">
      <label>
        <span>方案标题</span>
        <input :value="store.draftPlan.title" @input="onTitle" />
      </label>
      <label>
        <span>字幕预设</span>
        <select :value="store.draftPlan.subtitles.preset" @change="onSubtitlePreset">
          <option value="clean">简洁</option>
          <option value="emphasis">强调</option>
        </select>
      </label>
      <label>
        <span>旁白开关</span>
        <select :value="store.draftPlan.voice.enabled ? '1' : '0'" @change="onVoiceEnabled">
          <option value="1">开启</option>
          <option value="0">关闭</option>
        </select>
      </label>
    </div>

    <p v-if="!store.draftPlan?.scenes?.length" class="sv-muted">生成方案后可在此编辑场景、旁白与转场。</p>

    <article v-for="(scene, index) in store.draftPlan?.scenes || []" :key="scene.id || index" class="sv-scene">
      <header>
        <strong>场景 {{ index + 1 }} · {{ scene.title || "未命名" }}</strong>
        <span>{{ Math.round((scene.durationMs || 0) / 1000) }}s · {{ scene.transition?.type || "cut" }}</span>
      </header>
      <label>
        <span>场景标题</span>
        <input :value="scene.title" @input="(e) => patch(index, { title: (e.target as HTMLInputElement).value })" />
      </label>
      <label>
        <span>旁白</span>
        <textarea
          rows="3"
          :value="scene.narration"
          @input="(e) => patch(index, { narration: (e.target as HTMLTextAreaElement).value })"
        />
      </label>
      <label>
        <span>转场时长 (ms)</span>
        <input
          type="number"
          min="0"
          max="2000"
          :value="scene.transition?.durationMs || 0"
          @input="(e) => patchTransition(index, Number((e.target as HTMLInputElement).value) || 0)"
        />
      </label>
      <ul class="sv-clips">
        <li v-for="(clip, clipIndex) in scene.clips || []" :key="`${scene.id}-${clipIndex}`">
          素材 {{ clip.assetId.slice(0, 8) }} · {{ clip.sourceInMs }}-{{ clip.sourceOutMs }}ms · {{ clip.fitMode }}/{{ clip.motion }}
        </li>
      </ul>
    </article>
  </section>
</template>

<script setup lang="ts">
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();

function patch(index: number, data: Record<string, unknown>) {
  store.updateScene(index, data);
}

function patchTransition(index: number, durationMs: number) {
  store.updateDraftPlan((plan) => {
    const scene = plan.scenes[index];
    if (!scene) return;
    scene.transition = { ...(scene.transition || { type: "cut", durationMs: 0 }), durationMs };
  });
}

function onTitle(event: Event) {
  const value = (event.target as HTMLInputElement).value;
  store.updateDraftPlan((plan) => {
    plan.title = value;
  });
}

function onSubtitlePreset(event: Event) {
  const value = (event.target as HTMLSelectElement).value;
  store.updateDraftPlan((plan) => {
    plan.subtitles.preset = value;
  });
}

function onVoiceEnabled(event: Event) {
  const enabled = (event.target as HTMLSelectElement).value === "1";
  store.updateDraftPlan((plan) => {
    plan.voice.enabled = enabled;
  });
}
</script>

<style scoped>
.sv-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  background: rgba(23, 27, 36, 0.92);
  padding: 18px;
}

.sv-head,
.sv-inline,
.sv-plan-meta {
  display: flex;
  gap: 12px;
}

.sv-head {
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.sv-plan-meta {
  flex-wrap: wrap;
  margin-bottom: 14px;
}

.sv-plan-meta label,
.sv-scene label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 1;
  min-width: 160px;
}

.sv-muted,
.sv-scene span,
.sv-clips {
  color: #9aa3b5;
  font-size: 12px;
}

.sv-scene {
  display: grid;
  gap: 10px;
  padding: 14px 0;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.sv-scene header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

input,
textarea,
select {
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  color: #f4f6fb;
  padding: 8px 10px;
}

.sv-clips {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
}

.sv-btn {
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: #f4f6fb;
  padding: 8px 14px;
  cursor: pointer;
}

.sv-btn.primary {
  border: 0;
  background: #ff771b;
  color: #111;
  font-weight: 600;
}

.sv-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 720px) {
  .sv-head {
    flex-direction: column;
  }
}
</style>
