<template>
  <view class="section-stack">
    <view class="v31-home-hero">
      <RemoteCover class="v31-hero-cover" page-code="home" slot-key="home.hero.background" alt="知启云 AI 首页主视觉" width="100%" height="100%" :lazy-load="false" />
      <text class="v31-kicker">一句话开始</text>
      <text class="v31-hero-title">用 AI 完成设计、视频与 PPT</text>
      <text class="v31-hero-copy">从创作到判断，一站式解决。</text>
      <view class="v31-hero-row">
        <button type="button" class="v31-mini-metric purple" @click="emit('tab', 'wallet')">
          <text class="v31-metric-value">{{ formatNumber(pointBalance) }}</text>
          <text class="v31-metric-label">点数余额</text>
        </button>
        <button type="button" class="v31-mini-metric orange" @click="emit('tab', 'assets')">
          <text class="v31-metric-value">{{ recentAssetsCount }}</text>
          <text class="v31-metric-label">近期作品</text>
        </button>
        <button type="button" class="v31-orange-button" @click="emit('tab', 'create')">去创作</button>
      </view>
    </view>

    <text class="v31-section-title">常用工具</text>
    <view class="v31-tool-grid">
      <button v-for="module in creationModules" :key="`home-${module.id}`" class="v31-tool-card" @click="emit('open-mode', module.id)">
        <RemoteCover class="v31-tool-cover" page-code="home" :slot-key="homeModuleSlot(module.id)" :alt="module.name" width="36px" height="36px" radius="10px" />
        <view class="v31-tool-copy">
          <text class="v31-tool-name">{{ module.homeName || module.name }}</text>
          <text class="v31-tool-desc">{{ module.description }}</text>
        </view>
      </button>
    </view>

    <text class="v31-section-title">灵感推荐</text>
    <view class="v31-inspiration-grid">
      <button class="v31-inspiration-card" @click="emit('open-mode', 'image')">
        <RemoteCover class="v31-preview" page-code="home" slot-key="home.inspiration.ecommerce" alt="水果电商主图" width="100%" height="86px" radius="12px" />
        <text class="v31-inspiration-title">水果电商主图</text>
        <view class="v31-card-footer"><text class="v31-chip orange">图片</text><text class="v31-link">继续改</text></view>
      </button>
      <button class="v31-inspiration-card" @click="emit('open-mode', 'ppt')">
        <RemoteCover class="v31-preview" page-code="home" slot-key="home.inspiration.ppt" alt="招商路演 PPT" width="100%" height="86px" radius="12px" />
        <text class="v31-inspiration-title">招商路演PPT</text>
        <view class="v31-card-footer"><text class="v31-chip purple">PPT</text><text class="v31-link">继续改</text></view>
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import RemoteCover from "../../RemoteCover.vue";
import type { WorkbenchCreationModule } from "../../../features/workbench/catalog";
import type { MiniProgramCreationMode, MiniProgramTabId } from "../../../config/miniProgramPages";

interface Props {
  pointBalance: number;
  recentAssetsCount: number;
  creationModules: WorkbenchCreationModule[];
  homeModuleSlot: (mode: MiniProgramCreationMode) => string;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: "tab", tab: Extract<MiniProgramTabId, "wallet" | "assets" | "create">): void;
  (e: "open-mode", mode: MiniProgramCreationMode): void;
}>();

function formatNumber(value: number) {
  return Number(value || 0).toLocaleString("zh-CN");
}
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.v31-home-hero {
  position: relative;
  overflow: hidden;
  min-height: 156px;
  padding: 17px;
  box-sizing: border-box;
  border-radius: 12px;
  color: #ffffff;
  background: #15192d;
  box-shadow: 0 14px 30px rgba(23, 28, 56, 0.16);
}

.v31-hero-cover { position: absolute; z-index: 0; inset: 0; width: 100% !important; height: 100% !important; opacity: .34; }
.v31-tool-cover { position: relative; z-index: 1; width: 36px !important; height: 36px !important; flex: 0 0 36px; }

.v31-kicker,
.v31-hero-title,
.v31-hero-copy,
.v31-metric-value,
.v31-metric-label,
.v31-section-title,
.v31-tool-name,
.v31-tool-desc,
.v31-inspiration-title {
  display: block;
}

.v31-kicker {
  position: relative;
  z-index: 1;
  color: #aeb8ff;
  font-size: 11px;
  font-weight: 700;
}

.v31-hero-title {
  position: relative;
  z-index: 1;
  margin-top: 6px;
  font-size: 19px;
  font-weight: 700;
  line-height: 28px;
}

.v31-hero-copy {
  position: relative;
  z-index: 1;
  margin-top: 1px;
  color: #cdd5f5;
  font-size: 12px;
}

.v31-hero-row {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: 104px 88px minmax(90px, 1fr);
  gap: 10px;
  margin-top: 11px;
  align-items: center;
}

.v31-mini-metric {
  width: 100%;
  height: 58px;
  margin: 0;
  padding: 7px 9px;
  box-sizing: border-box;
  border: 0;
  border-radius: 8px;
  line-height: 1.4;
  text-align: left;
}
.v31-mini-metric::after { display: none; }

.v31-mini-metric.purple { background: #f4f3ff; }
.v31-mini-metric.orange { background: #fff7ed; }
.v31-mini-metric.purple .v31-metric-value { color: #5b55d6; }
.v31-mini-metric.orange .v31-metric-value { color: #ff6b1a; }

.v31-metric-value {
  font-size: 16px;
  font-weight: 700;
  line-height: 21px;
}

.v31-metric-label {
  margin-top: 2px;
  color: #667085;
  font-size: 10px;
}

.v31-orange-button {
  display: grid;
  height: 38px;
  margin: 0;
  padding: 0 16px;
  place-items: center;
  border-radius: 10px;
  color: #ffffff;
  background: #ff7a1a;
  font-size: 13px;
  font-weight: 600;
}

.v31-section-title {
  color: #111827;
  font-size: 15px;
  font-weight: 700;
  line-height: 20px;
}

.v31-tool-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  padding: 11px;
  border: 1px solid #e4e9f7;
  border-radius: 12px;
  background: #ffffff;
}

.v31-tool-card {
  position: relative;
  display: flex;
  min-width: 0;
  height: 58px;
  margin: 0;
  padding: 6px;
  align-items: center;
  gap: 6px;
  border: 1px solid #e5eaf6;
  border-radius: 8px;
  background: #ffffff;
  text-align: left;
  box-shadow: 0 3px 10px rgba(23, 28, 56, 0.025);
}

.v31-tool-card::after,
.v31-inspiration-card::after {
  display: none;
}

.v31-tool-copy {
  position: relative;
  z-index: 2;
  min-width: 0;
  flex: 1;
}

.v31-tool-name {
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-tool-desc {
  display: -webkit-box;
  margin-top: 3px;
  overflow: hidden;
  color: #697386;
  font-size: 10px;
  line-height: 13px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.v31-inspiration-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 11px;
  border: 1px solid #e4e9f7;
  border-radius: 12px;
  background: #ffffff;
}

.v31-inspiration-card {
  position: relative;
  min-width: 0;
  margin: 0;
  padding: 9px;
  border: 1px solid #e5eaf6;
  border-radius: 8px;
  background: #ffffff;
  text-align: left;
  box-shadow: 0 4px 12px rgba(23, 28, 56, 0.035);
}

.v31-preview {
  position: relative;
  z-index: 1;
  display: flex;
  width: 100%;
  height: 86px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 25px;
  font-weight: 700;
}

.v31-preview.orange { color: #ff6b1a; background: #fff2e8; }

.v31-inspiration-title {
  position: relative;
  z-index: 2;
  margin-top: 9px;
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-card-footer {
  position: relative;
  z-index: 2;
  display: flex;
  margin-top: 10px;
  align-items: center;
  justify-content: space-between;
}

.v31-chip {
  display: inline-flex;
  min-height: 24px;
  padding: 0 12px;
  align-items: center;
  justify-content: center;
  border: 1px solid #c9d2ff;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 11px;
}

.v31-chip.orange { color: #ff6b1a; border-color: #ffd0b3; background: #fff2e8; }
.v31-chip.purple { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }
.v31-link { color: #5b55d6; font-size: 10px; }
</style>
