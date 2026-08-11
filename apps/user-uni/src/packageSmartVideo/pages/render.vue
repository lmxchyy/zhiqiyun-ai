<template>
  <view class="sv-page" :style="navigationStyle">
    <view class="sv-nav">
      <button class="nav-back" aria-label="返回" @click="goBack">‹</button>
      <text class="nav-title">导出成片</text>
      <view class="nav-spacer" />
    </view>

    <scroll-view class="sv-scroll" scroll-y>
      <view class="sv-body">
        <view class="card">
          <text class="card-title">预估积分</text>
          <text class="points">{{ quotePoints > 0 ? `${quotePoints} 点` : "估算中…" }}</text>
          <text class="hint">确认导出后将按预估积分扣费；失败可重试，不会重复生成方案。</text>
        </view>

        <view class="card">
          <text class="card-title">导出进度</text>
          <view class="progress-track">
            <view class="progress-bar" :style="{ width: `${progress}%` }" />
          </view>
          <text class="status">{{ statusText }}</text>
          <text v-if="errorMessage" class="error">{{ errorMessage }}</text>
        </view>

        <view v-if="completed" class="card success">
          <text class="card-title">成片已完成</text>
          <text class="hint">可在「作品」中查看与分享导出结果。</text>
          <button class="ghost" @click="openAssets">去作品中心</button>
        </view>
      </view>
    </scroll-view>

    <view class="sv-footer">
      <button
        v-if="canCancel"
        class="secondary"
        :disabled="busy"
        @click="onCancel"
      >
        取消导出
      </button>
      <button
        v-if="canRetry"
        class="secondary"
        :disabled="busy"
        @click="onRetry"
      >
        重试
      </button>
      <button
        v-if="canExport"
        class="primary"
        :disabled="busy || quotePoints <= 0"
        @click="onExport"
      >
        {{ busy ? "处理中…" : "开始导出" }}
      </button>
      <button v-if="completed" class="primary" @click="openAssets">查看作品</button>
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
const quotePoints = computed(() => flow.quotePoints.value);
const progress = computed(() => {
  if (flow.state.phase === "completed") return 100;
  return Math.max(0, Math.min(100, flow.renderProgress.value));
});
const errorMessage = computed(() => flow.errorMessage.value);
const completed = computed(() => flow.state.phase === "completed");
const canExport = computed(() => ["quoting", "failed", "confirmed"].includes(flow.state.phase) || (!flow.state.renderTask && !completed.value));
const canCancel = computed(() => flow.state.phase === "rendering");
const canRetry = computed(() => flow.state.phase === "failed" || String(flow.state.renderTask?.status || "").toUpperCase() === "FAILED");
const statusText = computed(() => {
  if (flow.state.phase === "completed") return "导出成功";
  if (flow.state.phase === "rendering") return `正在导出 ${progress.value}%`;
  if (flow.state.phase === "failed") return "导出失败";
  if (flow.state.phase === "quoting") return "已就绪，可开始导出";
  return "准备中";
});

onLoad((query) => {
  projectId.value = String(query?.projectId || "").trim();
  if (!projectId.value) {
    uni.showToast({ title: "缺少项目 ID", icon: "none" });
    return;
  }
  void (async () => {
    if (flow.state.project?.id !== projectId.value) {
      await flow.openProject(projectId.value);
    }
    if (!flow.state.quote && flow.state.currentVersion?.id) {
      try {
        await flow.loadQuote();
      } catch {
        // surface via errorMessage
      }
    }
  })();
});

onShow(() => flow.setForeground(true));
onHide(() => flow.setForeground(false));

function goBack() {
  if (getCurrentPages().length > 1) uni.navigateBack();
  else {
    uni.redirectTo({
      url: `/packageSmartVideo/pages/plan?projectId=${encodeURIComponent(projectId.value)}`,
    });
  }
}

async function onExport() {
  try {
    await flow.startExport();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "导出失败", icon: "none" });
  }
}

async function onCancel() {
  try {
    await flow.cancelExport();
    uni.showToast({ title: "已取消", icon: "none" });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "取消失败", icon: "none" });
  }
}

async function onRetry() {
  try {
    await flow.retryExport();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "重试失败", icon: "none" });
  }
}

function openAssets() {
  uni.switchTab({ url: "/pages/user/UserAssetsPage" });
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
.card {
  margin-bottom: 12px;
  padding: 16px;
  border-radius: 14px;
  background: #fff;
}
.card.success {
  background: #eefaf3;
}
.card-title {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 700;
  color: #697084;
}
.points {
  display: block;
  font-size: 28px;
  font-weight: 800;
  line-height: 36px;
}
.hint,
.status {
  display: block;
  margin-top: 8px;
  color: #697084;
  font-size: 13px;
  line-height: 20px;
}
.progress-track {
  height: 10px;
  margin-top: 10px;
  overflow: hidden;
  border-radius: 999px;
  background: #e8edf7;
}
.progress-bar {
  height: 100%;
  border-radius: 999px;
  background: #4a6bff;
  transition: width 0.25s ease;
}
.error {
  display: block;
  margin-top: 8px;
  color: #d94848;
  font-size: 13px;
}
.ghost {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  background: #fff;
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
