<template>
  <view class="sv-page" :style="navigationStyle">
    <view class="sv-nav">
      <button class="nav-back" aria-label="返回" @click="goBack">‹</button>
      <text class="nav-title">混剪方案</text>
      <view class="nav-spacer" />
    </view>

    <scroll-view class="sv-scroll" scroll-y>
      <view class="sv-body">
        <view v-if="loading" class="state-card">
          <text>{{ statusText }}</text>
          <text class="hint">可离开页面，回到前台后会自动继续</text>
        </view>

        <template v-else-if="scenes.length">
          <label class="field">
            <text class="label">方案标题</text>
            <input :value="planTitle" maxlength="80" placeholder="方案标题" @input="onTitleInput" />
          </label>

          <view v-for="(scene, index) in scenes" :key="scene.id || index" class="scene-card">
            <text class="scene-index">镜头 {{ index + 1 }}</text>
            <label class="mini-field">
              <text class="label">标题</text>
              <input
                :value="scene.title"
                maxlength="60"
                placeholder="镜头标题"
                @input="(e) => onSceneTitle(index, e)"
              />
            </label>
            <label class="mini-field">
              <text class="label">旁白</text>
              <textarea
                :value="scene.narration"
                maxlength="400"
                placeholder="旁白文案"
                @input="(e) => onSceneNarration(index, e)"
              />
            </label>
            <text class="meta">时长约 {{ formatDuration(scene.durationMs) }}</text>
          </view>
        </template>

        <view v-else class="state-card">
          <text>{{ errorMessage || "暂无方案，请返回重新分析" }}</text>
          <button class="ghost" @click="reload">重新加载</button>
        </view>

        <text v-if="errorMessage && scenes.length" class="error">{{ errorMessage }}</text>
      </view>
    </scroll-view>

    <view v-if="scenes.length" class="sv-footer">
      <button class="secondary" :disabled="busy" @click="saveOnly">保存修订</button>
      <button class="primary" :disabled="busy" @click="confirmAndGo">确认方案</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad, onHide, onShow } from "@dcloudio/uni-app";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";
import { useSmartVideoProject } from "../../composables/useSmartVideoProject";

const { navigationStyle } = useMiniProgramNavigation();
const flow = useSmartVideoProject();
const projectId = ref("");

const busy = computed(() => flow.busy.value);
const scenes = computed(() => flow.scenes.value);
const errorMessage = computed(() => flow.errorMessage.value);
const planTitle = computed(() => flow.state.draftPlan?.title || "");
const loading = computed(() =>
  ["analyzing", "planning", "uploading"].includes(flow.state.phase) || (!scenes.value.length && busy.value),
);
const statusText = computed(() => {
  if (flow.state.phase === "analyzing") return "正在分析素材…";
  if (flow.state.phase === "planning") return "正在生成混剪方案…";
  if (flow.state.phase === "uploading") return "正在上传素材…";
  return "正在加载方案…";
});

onLoad((query) => {
  projectId.value = String(query?.projectId || "").trim();
  if (!projectId.value) {
    uni.showToast({ title: "缺少项目 ID", icon: "none" });
    return;
  }
  if (flow.state.project?.id !== projectId.value || !flow.state.draftPlan) {
    void flow.openProject(projectId.value);
  }
});

onShow(() => flow.setForeground(true));
onHide(() => flow.setForeground(false));

function goBack() {
  if (getCurrentPages().length > 1) uni.navigateBack();
  else {
    uni.redirectTo({
      url: `/packageSmartVideo/pages/create?projectId=${encodeURIComponent(projectId.value)}`,
    });
  }
}

function reload() {
  if (projectId.value) void flow.openProject(projectId.value);
}

function formatDuration(ms?: number) {
  const seconds = Math.max(0, Math.round(Number(ms || 0) / 1000));
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}分${seconds % 60}秒`;
}

function inputValue(event: unknown) {
  const detail = (event as { detail?: { value?: string } } | null)?.detail;
  return String(detail?.value ?? "");
}

function onTitleInput(event: unknown) {
  flow.updatePlanTitle(inputValue(event));
}

function onSceneTitle(index: number, event: unknown) {
  flow.updateSceneLight(index, { title: inputValue(event) });
}

function onSceneNarration(index: number, event: unknown) {
  flow.updateSceneLight(index, { narration: inputValue(event) });
}

async function saveOnly() {
  try {
    await flow.saveRevision();
    uni.showToast({ title: "已保存", icon: "success" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "保存失败", icon: "none" });
  }
}

async function confirmAndGo() {
  try {
    await flow.confirmPlan();
    await flow.loadQuote();
    uni.redirectTo({
      url: `/packageSmartVideo/pages/render?projectId=${encodeURIComponent(projectId.value)}`,
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "确认失败", icon: "none" });
  }
}
</script>

<style scoped>
.sv-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f7fb;
  color: #181c28;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
}
.sv-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--header-padding-top, 20px) var(--capsule-right-space, 16px) 10px 12px;
  background: #fff;
}
.nav-back,
.ghost,
.secondary,
.primary {
  margin: 0;
  border: 0;
  background: transparent;
}
.nav-back::after,
.ghost::after,
.secondary::after,
.primary::after {
  display: none;
}
.nav-back {
  width: 36px;
  height: 36px;
  padding: 0;
  font-size: 28px;
}
.nav-title {
  font-size: 17px;
  font-weight: 700;
}
.nav-spacer {
  width: 36px;
}
.sv-scroll {
  flex: 1;
  height: 0;
}
.sv-body {
  padding: 16px 16px 120px;
}
.state-card,
.field,
.scene-card {
  margin-bottom: 12px;
  padding: 14px;
  border-radius: 14px;
  background: #fff;
}
.state-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  color: #697084;
  font-size: 14px;
}
.hint {
  font-size: 12px;
  color: #9aa3b5;
}
.label {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
}
.mini-field {
  display: block;
  margin-top: 10px;
}
.scene-index {
  display: block;
  color: #4a6bff;
  font-size: 12px;
  font-weight: 700;
}
.meta {
  display: block;
  margin-top: 8px;
  color: #9aa3b5;
  font-size: 12px;
}
input,
textarea {
  width: 100%;
  font-size: 14px;
  color: #181c28;
}
textarea {
  min-height: 72px;
}
.error {
  color: #d94848;
  font-size: 13px;
}
.ghost {
  margin-top: 8px;
  padding: 8px 12px;
  border-radius: 10px;
  background: #eef2ff;
  color: #4a6bff;
  font-size: 13px;
}
.sv-footer {
  display: flex;
  gap: 10px;
  padding: 12px 16px calc(12px + env(safe-area-inset-bottom));
  background: rgba(245, 247, 251, 0.96);
}
.secondary,
.primary {
  flex: 1;
  height: 48px;
  border-radius: 14px;
  font-size: 15px;
  font-weight: 700;
}
.secondary {
  background: #fff;
  color: #4a6bff;
  border: 1px solid #d9e1ff;
}
.primary {
  background: #4a6bff;
  color: #fff;
}
.secondary[disabled],
.primary[disabled] {
  opacity: 0.45;
}
</style>
