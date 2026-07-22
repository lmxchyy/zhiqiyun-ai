<template>
  <view class="v531-page v531-home">
    <view class="home-brand">
      <view
        ><text class="brand-title">知启云AI</text
        ><text class="brand-subtitle">Enterprise AI Workspace</text></view
      >
      <view class="brand-actions">
        <button class="brand-action" aria-label="通知" @click="$emit('notice')">
          <image
            class="brand-action-icon"
            src="/static/icons/bell.svg"
            mode="aspectFit"
          /><text class="notice-dot"></text>
        </button>
        <button
          class="brand-action"
          aria-label="个人中心"
          @click="$emit('profile')"
        >
          <AppImage
            :src="avatarUrl"
            :fallback="avatarFallback"
            local-fallback="/static/fallbacks/default-avatar.jpg"
            :alt="displayName"
            width="44px"
            height="44px"
            radius="14px"
            :lazy-load="false"
          />
        </button>
      </view>
    </view>

    <view class="home-hero">
      <RemoteCover
        class="hero-background"
        page-code="home"
        slot-key="home.hero.background"
        mode="cover"
        width="100%"
        height="100%"
        radius="26px"
        :lazy-load="false"
      />
      <RemoteCover
        class="hero-illustration"
        page-code="home"
        slot-key="home.hero.illustration"
        mode="cover"
        width="100%"
        height="100%"
        radius="26px"
        :lazy-load="false"
      />
      <view class="hero-scrim"></view>
      <text class="hero-greeting">👋 {{ greeting }}，{{ shortName }}</text>
      <text class="hero-title">今天，让 AI 帮你完成什么？</text>
      <text class="hero-subtitle">一句话启动企业级创作、分析与执行</text>
      <view class="hero-input">
        <input
          class="hero-text-input"
          v-model="prompt"
          maxlength="120"
          confirm-type="send"
          always-embed
          cursor-spacing="24"
          placeholder="输入一句需求……"
          @confirm="submitPrompt"
        />
        <view class="hero-input-actions">
          <button
            class="hero-input-action"
            aria-label="语音输入"
            role="button"
            hover-class="hero-input-action-pressed"
            @tap.stop="showVoiceInputLimit"
          >
            <image
              class="hero-input-icon"
              src="/static/icons/mic.svg"
              mode="aspectFit"
            />
          </button>
          <button
            class="hero-input-action submit"
            aria-label="发送需求"
            role="button"
            hover-class="hero-input-action-pressed"
            @touchend.stop.prevent="submitPrompt"
            @tap.stop="submitPrompt"
          >
            <image
              class="hero-input-icon"
              src="/static/icons/arrow-up.svg"
              mode="aspectFit"
            />
          </button>
          <view
            class="hero-input-action-hit-disabled"
            aria-label="语音输入"
            @tap.stop="showVoiceInputLimit"
          ></view>
          <view
            class="hero-input-action-hit-disabled"
            aria-label="发送需求"
            @tap.stop="submitPrompt"
          ></view>
        </view>
      </view>
      <text class="quick-title">热门快捷入口</text>
      <view class="quick-grid">
        <button
          class="quick-action"
          v-for="item in quickActions"
          :key="item.label"
          @click="runQuick(item)"
        >
          {{ item.label }}
        </button>
      </view>
      <view class="hero-tool-row">
        <button
          class="hero-tool"
          v-for="item in heroTools"
          :key="item.label"
          @click="openCreationMode(item.mode)"
        >
          <text :class="['hero-tool-icon', item.tone]">{{ item.icon }}</text>
          <text class="hero-tool-label">{{ item.label }}</text>
        </button>
      </view>
    </view>

    <view class="workspace-heading">
      <view
        ><text class="workspace-title">企业工作台</text
        ><text class="workspace-subtitle">Key signals at a glance</text></view
      >
      <button class="workspace-action" @click="openUserTab('mine')">查看全部 ›</button>
    </view>
    <view class="metric-grid">
      <button
        class="metric-card"
        v-for="metric in metrics"
        :key="metric.label"
        @click="metric.action && openUserTab(metric.action)"
      >
        <text class="metric-label">{{ metric.label }}</text>
        <view class="metric-value-row"
          ><text :class="metricValueClass(metric.value)">{{ metric.value }}</text
          ><text v-if="metric.unit" class="metric-unit">{{ metric.unit }}</text></view
        >
        <text class="metric-note">{{ metric.note }}</text>
      </button>
    </view>
    <button class="ai-suggestion" @click="openSuggestion"
      ><text class="suggestion-icon">✦</text
      ><view class="suggestion-copy"
        ><text class="suggestion-title">AI 今日建议</text
        ><text class="suggestion-body">{{ suggestionText }}</text></view
      ></button
    >

    <section-title
      title="AI 创作中心"
      subtitle="Create with enterprise AI"
      action="全部能力"
      @action="openUserTab('create')"
    />
    <view class="capability-feature-grid">
      <button
        :class="['capability-card', 'capability-feature-card', item.id]"
        v-for="item in featuredCapabilities"
        :key="item.id"
        @click="openCreationMode(item.routeMode)"
      >
        <RemoteCover
          class="capability-feature-cover"
          page-code="home"
          :slot-key="item.slotKey"
          :alt="item.title"
          mode="aspectFill"
          width="100%"
          height="100%"
          radius="16px"
        />
        <view class="capability-feature-overlay"></view>
        <view class="capability-feature-copy"
          ><text class="feature-title">{{ item.title }}</text
          ><text class="feature-subtitle">{{ item.description }}</text
          ><text class="feature-action">立即开始 →</text></view
        >
      </button>
    </view>
    <view class="capability-secondary-grid">
      <button
        :class="['capability-card', 'capability-secondary-card', item.id]"
        v-for="item in secondaryCapabilities"
        :key="item.id"
        @click="openCreationMode(item.routeMode)"
      >
        <text :class="['secondary-icon', item.id]">{{ item.title.slice(0, 1) }}</text>
        <view class="secondary-copy"
          ><text class="secondary-title">{{ item.title }}</text
          ><text class="secondary-subtitle">{{ item.description }}</text></view
        >
        <image
          class="secondary-chevron"
          src="/static/icons/chevron-right.svg"
          mode="aspectFit"
        />
      </button>
    </view>
    <view class="capability-compact-row">
      <button
        class="capability-card capability-compact-card"
        v-for="item in compactCapabilities"
        :key="item.id"
        @click="openCreationMode(item.routeMode)"
      >
        <text :class="['compact-icon', 'tone-' + item.tone]">{{ item.icon }}</text>
        <view class="compact-copy"
          ><text class="compact-title">{{ item.title }}</text
          ><text class="compact-subtitle">{{ item.description }}</text></view
        >
      </button>
    </view>

    <view id="v531-projects"></view>
    <view class="project-section-heading">
      <text class="project-section-title">继续工作</text>
      <button class="project-section-action" @click="openUserTab('assets')">
        查看全部 ›
      </button>
    </view>
    <view v-if="projectItems.length" class="project-list">
      <button
        class="project-card"
        v-for="item in projectItems"
        :key="item.id"
        @click="openProject(item)"
      >
        <view class="project-copy">
          <text class="project-name">{{ item.title }}</text>
          <text class="project-meta">{{ item.meta }}</text>
        </view>
        <view class="project-progress">
          <text class="project-percent">{{ item.progress }}%</text>
          <view class="progress"
            ><view
              class="progress-fill"
              :style="{ width: `${item.progress}%` }"
            ></view></view
          >
        </view>
        <view class="project-media">
          <RemoteCover
            class="project-thumb"
            page-code="home"
            :slot-key="item.slotKey"
            :alt="item.title"
            width="56px"
            height="38px"
            radius="10px"
          />
        </view>
        <text class="project-action">继续处理</text>
      </button>
    </view>
    <view v-else class="project-empty">
      <text>暂无最近作品，完成一次创作后可从这里继续。</text>
      <button @click="openUserTab('create')">开始创作</button>
    </view>

    <view class="employee-section-heading">
      <text class="employee-section-title">AI 员工</text>
      <button class="employee-section-action" @click="openCreationMode('agent')">
        查看全部 ›
      </button>
    </view>
    <scroll-view
      scroll-x
      class="horizontal-scroll employee-scroll"
      enhanced
      :show-scrollbar="false"
    >
      <view class="employee-row">
        <button
          class="employee-card"
          v-for="item in employeeItems"
          :key="item.id"
          @click="openCreationMode('agent')"
        >
          <RemoteCover
            class="employee-avatar"
            page-code="home"
            :slot-key="item.slotKey"
            :alt="item.name"
            mode="cover"
            width="100%"
            height="78px"
            radius="0"
          />
          <view class="employee-copy">
            <text class="employee-name">{{ item.name }}</text>
            <view class="employee-status-row"
              ><text :class="['employee-status-dot', item.statusTone]"></text
              ><text class="employee-status">{{ item.statusLabel }}</text></view
            >
          </view>
        </button>
      </view>
    </scroll-view>

    <view class="inspiration-section-heading">
      <text class="inspiration-section-title">创作灵感</text>
      <view class="inspiration-heading-actions"><button class="inspiration-refresh" :disabled="inspirationLoading" @click="refreshInspirations">换一换</button><button class="inspiration-more" @click="openInspirationSquare()">查看更多 ›</button></view>
    </view>
    <scroll-view
      scroll-x
      class="inspiration-tab-scroll"
      enhanced
      :show-scrollbar="false"
    >
      <view class="inspiration-tabs">
        <button
          v-for="tab in inspirationTabs"
          :key="tab.code"
          :class="['inspiration-tab', { active: activeInspirationTab === tab.code }]"
          @click="selectInspirationCategory(tab.code)"
        >
          {{ tab.name }}
        </button>
      </view>
    </scroll-view>
    <view v-if="inspirationLoading && !inspirationItems.length" class="home-inspiration-skeleton"><view v-for="n in 6" :key="n" /></view>
    <view v-else-if="inspirationError && !inspirationItems.length" class="home-inspiration-state"><text>灵感加载失败</text><button @click="loadInspirations(true)">重试</button></view>
    <view v-else-if="!inspirationItems.length" class="home-inspiration-state"><text>暂无精选灵感</text></view>
    <view v-else class="home-inspiration-grid"><InspirationCard v-for="item in inspirationItems" :key="item.id" :item="item" @open="openInspirationDetail" /></view>

    <view class="home-footer"
      ><text class="footer-title">让 AI 成为企业生产力</text
      ><text class="footer-subtitle"
        >知启云AI · Enterprise AI Workspace</text
      ></view
    >
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import AppImage from "../AppImage.vue";
import RemoteCover from "../RemoteCover.vue";
import SectionTitle from "./V531SectionTitle.vue";
import { v531Capabilities, v531Employees } from "../../config/v531";
import InspirationCard from "../inspiration/InspirationCard.vue";
import { inspirationAPI } from "../../features/inspiration/api";
import type { InspirationCategory, InspirationTemplate } from "../../features/inspiration/types";
import { miniProgramCreationPages, miniProgramFeaturePages, rolePage } from "../../config/miniProgramPages";

interface AssetLike {
  id: string;
  name: string;
  mediaType?: string;
  createdAt?: string;
  metadata?: Record<string, unknown>;
}

interface TaskLike {
  id: string;
  type?: string;
  status?: string;
  prompt?: string;
  createdAt?: string;
}

const props = defineProps<{
  displayName: string;
  avatarUrl?: string;
  avatarFallback?: string;
  pointBalance: number;
  planName: string;
  subscriptionExpiresAt?: string;
  todayCalls: number;
  assets?: AssetLike[];
  tasks?: TaskLike[];
  allowedCreationModes?: CreationMode[];
}>();
type CreationMode =
  "image" | "video" | "ppt" | "infographic" | "review" | "agent";
type CompactCapabilityTone =
  "blue" | "red" | "orange" | "green" | "purple" | "dark";
type CompactCapability = {
  id: string;
  title: string;
  description: string;
  icon: string;
  routeMode: CreationMode;
  tone: CompactCapabilityTone;
};
type EmployeeStatusTone = "online" | "busy" | "standby";
type UserTab = "home" | "create" | "assets" | "mine" | "wallet";
const emit = defineEmits<{
  tab: [tab: UserTab];
  "open-mode": [mode: CreationMode];
  "open-asset": [asset: AssetLike];
  notice: [];
  profile: [];
}>();
const prompt = ref("");
const shortName = computed(
  () => props.displayName.replace(/@.*$/, "").slice(0, 6) || "用户",
);
const greeting = computed(() => {
  const hour = new Date().getHours();
  return hour < 11
    ? "上午好"
    : hour < 14
      ? "中午好"
      : hour < 18
        ? "下午好"
        : "晚上好";
});
const isModeAllowed = (mode: CreationMode) =>
  (props.allowedCreationModes || ["image", "infographic"]).includes(mode);
const quickActionsSource = [
  { label: "宣传海报", mode: "image" },
  { label: "招商PPT", mode: "ppt" },
  { label: "短视频", mode: "video" },
  { label: "知识库", mode: "agent" },
];
const quickActions = computed(() =>
  quickActionsSource.filter((item) => isModeAllowed(item.mode as CreationMode)),
);
const heroToolsSource: Array<{
  label: string;
  icon: string;
  mode: CreationMode;
  tone: "blue" | "red" | "orange" | "green" | "purple";
}> = [
  { label: "AI 设计", icon: "设", mode: "image", tone: "blue" },
  { label: "AI 视频", icon: "视", mode: "video", tone: "red" },
  { label: "PPT 生成", icon: "P", mode: "ppt", tone: "orange" },
  { label: "知识库", icon: "库", mode: "agent", tone: "green" },
  { label: "AI 员工", icon: "员", mode: "agent", tone: "purple" },
];
const heroTools = computed(() =>
  heroToolsSource.filter((item) => isModeAllowed(item.mode)),
);
const availableCapabilities = computed(() =>
  v531Capabilities.filter((item) => isModeAllowed(item.routeMode as CreationMode)),
);
const featuredCapabilities = computed(() => availableCapabilities.value.slice(0, 2));
const secondaryCapabilities = computed(() => availableCapabilities.value.slice(2, 4));
const knowledgeCapability = v531Capabilities.find(
  (item) => item.id === "knowledge",
);
const employeeCapability = v531Capabilities.find(
  (item) => item.id === "employee",
);
const compactCapabilitiesSource: CompactCapability[] = [
  {
    id: knowledgeCapability?.id || "knowledge",
    title: knowledgeCapability?.title || "知识库",
    description: knowledgeCapability?.description || "企业知识管理",
    icon: "K",
    routeMode: (knowledgeCapability?.routeMode || "agent") as CreationMode,
    tone: "blue",
  },
  {
    id: employeeCapability?.id || "employee",
    title: employeeCapability?.title || "AI 员工",
    description: employeeCapability?.description || "数字员工协作",
    icon: "AI",
    routeMode: (employeeCapability?.routeMode || "agent") as CreationMode,
    tone: "purple",
  },
  {
    id: "workflow",
    title: "Workflow",
    description: "自动化工作流",
    icon: "W",
    routeMode: "agent",
    tone: "green",
  },
  {
    id: "more",
    title: "更多能力",
    description: "进入智能体中心",
    icon: "+",
    routeMode: "agent",
    tone: "dark",
  },
];
const compactCapabilities = computed(() =>
  compactCapabilitiesSource.filter((item) => isModeAllowed(item.routeMode)),
);
const modeForMedia = (value: string): CreationMode => value.toLowerCase().includes("video") ? "video" : value.toLowerCase().includes("ppt") || value.toLowerCase().includes("document") ? "ppt" : "image";
const projectItems = computed(() => (props.assets || []).slice(0, 3).map((asset) => {
  const mode = modeForMedia(String(asset.mediaType || asset.metadata?.type || "image"));
  return {
    id: asset.id,
    title: asset.name || "未命名作品",
    meta: asset.createdAt ? `最近更新 ${new Date(asset.createdAt).toLocaleDateString("zh-CN")}` : "最近作品",
    progress: 100,
    slotKey: mode === "video" ? "home.inspiration.video" : mode === "ppt" ? "home.inspiration.ppt" : "home.inspiration.ecommerce",
    mode,
    asset,
  };
}));
const employeeStatusMeta: Record<
  string,
  { label: string; tone: EmployeeStatusTone }
> = {
  designer: { label: "可使用", tone: "online" },
  sales: { label: "可使用", tone: "online" },
  operation: { label: "可使用", tone: "online" },
  service: { label: "可使用", tone: "online" },
  boss: { label: "可使用", tone: "online" },
};
const employeeItems = computed(() =>
  isModeAllowed("agent") ? v531Employees.map((item) => {
    const status = employeeStatusMeta[item.id] || {
      label: item.status,
      tone: "standby" as const,
    };
    return {
      ...item,
      statusLabel: status.label,
      statusTone: status.tone,
    };
  }) : [],
);
const inspirationCategories = ref<InspirationCategory[]>([]);
const inspirationTabs = computed(() => [
  { id: "recommend", code: "", name: "推荐", sort: 999 },
  ...inspirationCategories.value.filter((item) => item.code !== "recommend"),
]);
const activeInspirationTab = ref("");
const inspirationOffset = ref(0);
const inspirationItems = ref<InspirationTemplate[]>([]);
const inspirationLoading = ref(false);
const inspirationError = ref("");
async function loadInspirations(reset = false) {
  if (inspirationLoading.value) return;
  inspirationLoading.value = true;
  inspirationError.value = "";
  if (reset) inspirationItems.value = [];
  try {
    const result = await inspirationAPI.featured(activeInspirationTab.value, inspirationOffset.value, 8);
    inspirationItems.value = result.items;
  } catch (reason) {
    inspirationError.value = reason instanceof Error ? reason.message : "请稍后重试";
  } finally {
    inspirationLoading.value = false;
  }
}
function refreshInspirations() { inspirationOffset.value += 1; void loadInspirations(); }
function selectInspirationCategory(code: string) { activeInspirationTab.value = code; inspirationOffset.value = 0; void loadInspirations(true); }
function openInspirationDetail(item: InspirationTemplate) { uni.navigateTo({ url: `/pages/inspiration/InspirationDetailPage?templateId=${encodeURIComponent(item.id)}` }); }
function openInspirationSquare(category = activeInspirationTab.value) { uni.navigateTo({ url: `/pages/inspiration/InspirationSquarePage?category=${encodeURIComponent(category)}` }); }
onMounted(async () => {
  try { inspirationCategories.value = (await inspirationAPI.categories()).items; } catch { inspirationCategories.value = []; }
  await loadInspirations();
});
const metrics = computed(() => [
  {
    label: "剩余点数",
    value: props.pointBalance.toLocaleString("zh-CN"),
    unit: "点",
    note: "账户实时余额",
    action: "wallet" as const,
  },
  {
    label: "会员等级",
    value: props.planName || "PRO",
    unit: "",
    note: props.subscriptionExpiresAt ? `有效期至 ${new Date(props.subscriptionExpiresAt).toLocaleDateString("zh-CN")}` : "长期有效或待完善",
    action: "mine" as const,
  },
  {
    label: "今日 AI 调用",
    value: String(props.todayCalls),
    unit: "次",
    note: "今日实时调用",
    action: "assets" as const,
  },
  {
    label: "企业资产",
    value: String((props.assets || []).length),
    unit: "项",
    note: "知识库 / 作品 / 模板",
    action: "assets" as const,
  },
]);
const metricValueClass = (value: string) => [
  "metric-value",
  value.length > 7 ? "compact" : "",
  value.length > 10 ? "multi-line" : "",
];
const userNativeTabs = new Set<UserTab>(["home", "create", "assets", "mine"]);

function navigateStandalone(url: string) {
  if (!url) {
    uni.showToast({ title: "页面地址为空", icon: "none" });
    return;
  }
  uni.navigateTo({
    url,
    fail() {
      uni.redirectTo({
        url,
        fail() {
          uni.reLaunch({
            url,
            fail() {
              uni.showToast({ title: "页面打开失败，请重试", icon: "none" });
            },
          });
        },
      });
    },
  });
}

function openUserTab(tab: UserTab) {
  const url = rolePage("user", tab);
  if (userNativeTabs.has(tab)) {
    uni.switchTab({
      url,
      fail() {
        uni.reLaunch({ url });
      },
    });
    return;
  }
  navigateStandalone(url);
}

function openCreationMode(mode: CreationMode) {
  if (!isModeAllowed(mode)) {
    uni.showToast({ title: "当前小程序审核版本暂未开放该能力", icon: "none" });
    return;
  }
  navigateStandalone(miniProgramCreationPages[mode] || miniProgramCreationPages.image);
}

function runQuick(item: { mode: string }) {
  openCreationMode(item.mode as CreationMode);
}
let lastPromptSubmitAt = 0;
function submitPrompt() {
  const now = Date.now();
  if (now - lastPromptSubmitAt < 500) return;
  lastPromptSubmitAt = now;
  const value = prompt.value.trim();
  if (!value) {
    uni.showToast({ title: "请输入你的需求", icon: "none" });
    return;
  }
  uni.setStorageSync("v531-creation-prompt", value);
  const mode: CreationMode = /视频|短片|口播|分镜/.test(value) ? "video" : /ppt|演示|汇报|路演|方案/i.test(value) ? "ppt" : /智能体|agent|客服|销售助手|知识库/i.test(value) ? "agent" : /信息图|流程图|数据图|可视化/.test(value) ? "infographic" : "image";
  uni.showToast({ title: "正在进入创作", icon: "none", duration: 700 });
  openCreationMode(mode);
}
const suggestionText = computed(() => {
  const running = (props.tasks || []).find(task => ["PENDING", "QUEUED", "RUNNING", "PROCESSING"].includes(String(task.status || "").toUpperCase()));
  if (running) return `继续查看“${running.prompt?.trim().slice(0, 20) || running.id}”的生成进度`;
  const latest = projectItems.value[0];
  return latest ? `继续完善“${latest.title}”` : "从一句需求开始创建第一份企业 AI 作品";
});
function openSuggestion() {
  openUserTab(projectItems.value.length || (props.tasks || []).length ? "assets" : "create");
}
function openProject(item: (typeof projectItems.value)[number]) {
  const id = item.asset?.id;
  if (id) {
    navigateStandalone(`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(id)}`);
    return;
  }
  openCreationMode(item.mode);
}
function showVoiceInputLimit() {
  uni.showModal({
    title: "语音输入暂不可用",
    content: "当前后台尚未接入语音识别接口，请先使用文字输入需求；录音转写能力接入后会在这里启用。",
    showCancel: false,
  });
}
</script>

<style scoped>
.v531-page {
  padding: 0 20px 118px;
  color: #181c28;
  background: #f5f7fb;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif;
}
.home-brand {
  display: flex;
  min-height: 98px;
  align-items: center;
  justify-content: space-between;
}
.brand-title,
.brand-subtitle {
  display: block;
}
.brand-title {
  font-size: 24px;
  font-weight: 700;
  line-height: 32px;
}
.brand-subtitle {
  color: #697084;
  font-size: 11px;
  line-height: 18px;
}
.brand-actions {
  display: flex;
  align-items: center;
  gap: 7px;
}
.brand-action {
  position: relative;
  width: 44px;
  height: 44px;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 14px;
  background: transparent;
}
.brand-action::after {
  display: none;
}
.brand-action-icon {
  width: 26px;
  height: 26px;
}
.notice-dot {
  position: absolute;
  top: 9px;
  right: 8px;
  width: 6px;
  height: 6px;
  border: 2px solid #fff;
  border-radius: 50%;
  background: #f04438;
}
.home-hero {
  position: relative;
  min-height: 340px;
  padding: 20px 16px 18px;
  box-sizing: border-box;
  overflow: hidden;
  border-radius: 26px;
  color: #fff;
  background: linear-gradient(140deg, #0f1733, #332985 58%, #4a6cff);
  box-shadow: 0 12px 36px rgba(26, 36, 66, 0.1);
}
.hero-background {
  position: absolute;
  inset: 0;
  z-index: 0;
  width: 100% !important;
  height: 100% !important;
  opacity: 0.78;
}
.hero-illustration {
  position: absolute;
  z-index: 1;
  inset: 0;
  width: 100% !important;
  height: 100% !important;
  border: 0;
  opacity: 0.42;
  transform: scale(1.04);
}
.hero-scrim {
  position: absolute;
  inset: 0;
  z-index: 1;
  background:
    radial-gradient(
      circle at 80% 15%,
      rgba(111, 128, 255, 0.34),
      transparent 32%
    ),
    linear-gradient(
      135deg,
      rgba(13, 18, 47, 0.9) 0%,
      rgba(34, 31, 102, 0.72) 55%,
      rgba(70, 98, 255, 0.46) 100%
    );
}
.hero-greeting,
.hero-title,
.hero-subtitle,
.quick-title {
  position: relative;
  z-index: 2;
  display: block;
}
.hero-greeting {
  font-size: 17px;
  font-weight: 700;
  line-height: 25px;
  color: rgba(255, 255, 255, 0.96);
}
.hero-title {
  max-width: 246px;
  margin-top: 8px;
  font-size: 27px;
  font-weight: 700;
  line-height: 34px;
}
.hero-subtitle {
  margin-top: 4px;
  color: rgba(255, 255, 255, 0.72);
  font-size: 12px;
  line-height: 19px;
}
.hero-input {
  position: relative;
  z-index: 2;
  display: flex;
  height: 58px;
  margin-top: 18px;
  padding: 8px 8px 8px 15px;
  box-sizing: border-box;
  align-items: center;
  gap: 6px;
  border-radius: 17px;
  background: rgba(255, 255, 255, 0.97);
  box-shadow: 0 8px 24px rgba(26, 36, 66, 0.16);
}
.hero-input input {
  position: relative;
  z-index: 1;
  min-width: 0;
  width: 0;
  height: 40px;
  flex: 1;
  color: #181c28;
  font-size: 13px;
}
.hero-text-input {
  width: 0;
  flex: 1;
}
.hero-input-actions {
  position: relative;
  z-index: 2;
  display: grid;
  grid-template-columns: repeat(2, 42px);
  gap: 6px;
  flex: 0 0 auto;
}
.hero-input-action {
  position: relative;
  z-index: 11;
  display: grid;
  width: 42px;
  min-width: 42px;
  height: 42px;
  margin: 0;
  padding: 0;
  place-items: center;
  border: 0;
  border-radius: 14px;
  background: #fff;
}
.hero-input-action-hit {
  position: absolute;
  top: 0;
  z-index: 20;
  width: 42px;
  height: 42px;
  background: rgba(255, 255, 255, 0.01);
}
.hero-input-action-hit.mic {
  right: 48px;
}
.hero-input-action-hit.submit {
  right: 0;
}
.hero-input-action-hit-disabled {
  display: none;
}
.hero-input-action-pressed {
  opacity: 0.72;
  transform: scale(0.98);
}
.hero-input-action::after,
.quick-action::after,
.metric-card::after,
.capability-card::after,
.project-card::after,
.employee-card::after,
.inspiration-card::after {
  display: none;
}
.hero-input-action.submit {
  background: #4a6cff;
}
.hero-input-icon {
  width: 21px;
  height: 21px;
}
.quick-title {
  margin-top: 13px;
  color: rgba(255, 255, 255, 0.88);
  font-size: 14px;
  font-weight: 700;
  line-height: 22px;
}
.quick-grid {
  position: relative;
  z-index: 2;
  display: grid;
  margin-top: 8px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}
.quick-action {
  height: 30px;
  margin: 0;
  padding: 0;
  border: 1px solid rgba(255, 255, 255, 0.24);
  border-radius: 15px;
  color: #fff;
  background: rgba(255, 255, 255, 0.12);
  font-size: 10px;
  font-weight: 600;
  line-height: 28px;
}
.hero-tool-row {
  position: relative;
  z-index: 2;
  display: grid;
  margin-top: 16px;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 6px;
}
.hero-tool {
  width: 100%;
  height: 58px;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  line-height: 1;
}
.hero-tool::after {
  display: none;
}
.hero-tool-icon {
  display: grid;
  width: 34px;
  height: 34px;
  margin: 0 auto;
  place-items: center;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 700;
}
.hero-tool-icon.blue {
  color: #3157ff;
  background: #e8eeff;
}
.hero-tool-icon.red {
  color: #f04438;
  background: #fff0f0;
}
.hero-tool-icon.orange {
  color: #ff8a00;
  background: #fff2e0;
}
.hero-tool-icon.green {
  color: #16a085;
  background: #e9faf5;
}
.hero-tool-icon.purple {
  color: #7b5cff;
  background: #f0edff;
}
.hero-tool-label {
  display: block;
  margin-top: 7px;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.94);
  font-size: 10px;
  font-weight: 600;
  line-height: 13px;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.workspace-heading {
  display: flex;
  margin: 14px 0 12px;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.workspace-title,
.workspace-subtitle {
  display: block;
}
.workspace-title {
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 25px;
}
.workspace-subtitle {
  color: #697084;
  font-size: 10px;
  line-height: 16px;
}
.workspace-action {
  width: auto;
  height: 24px;
  margin: 0;
  padding: 0 2px;
  border: 0;
  color: #8b95a7;
  background: transparent;
  font-size: 10px;
  line-height: 24px;
}
.workspace-action::after {
  display: none;
}
.metric-grid {
  display: grid;
  width: 100%;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 7px;
}
.metric-card {
  position: relative;
  width: 100%;
  min-width: 0;
  height: 92px;
  margin: 0;
  padding: 10px 7px;
  overflow: hidden;
  text-align: left;
  border: 0;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 6px 16px rgba(26, 36, 66, 0.05);
}
.metric-label,
.metric-note {
  display: block;
}
.metric-label {
  color: #697084;
  font-size: 9px;
  line-height: 13px;
}
.metric-value-row {
  display: flex;
  min-width: 0;
  margin-top: 7px;
  align-items: baseline;
  gap: 3px;
}
.metric-value {
  max-width: 100%;
  overflow: hidden;
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-unit {
  color: #697084;
  font-size: 8px;
  line-height: 12px;
}
.metric-note {
  margin-top: 6px;
  overflow: hidden;
  color: #8b95a7;
  font-size: 8px;
  line-height: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-value.compact {
  font-size: 15px;
  line-height: 19px;
}
.metric-value.multi-line {
  max-width: 100%;
  font-size: 13px;
  line-height: 17px;
  white-space: nowrap;
}
.ai-suggestion {
  display: flex;
  min-height: 72px;
  margin-top: 12px;
  padding: 12px 14px;
  box-sizing: border-box;
  align-items: center;
  gap: 14px;
  border: 1px solid #d9e1ff;
  border-radius: 20px;
  background: linear-gradient(90deg, #f0f5ff, #faf2ff);
  box-shadow: 0 6px 20px rgba(26, 36, 64, 0.07);
  width: 100%;
  margin-left: 0;
  margin-right: 0;
  border-color: #d9e1ff;
  text-align: left;
}
.ai-suggestion::after { display: none; }
.suggestion-icon {
  display: grid;
  width: 44px;
  min-width: 44px;
  height: 44px;
  place-items: center;
  border-radius: 14px;
  color: #4a6cff;
  background: #fff;
  font-size: 20px;
}
.suggestion-title,
.suggestion-body {
  display: block;
}
.suggestion-title {
  color: #4a6cff;
  font-size: 13px;
  font-weight: 700;
}
.suggestion-body {
  margin-top: 6px;
  color: #181c28;
  font-size: 12px;
  line-height: 19px;
}
.capability-feature-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}
.capability-feature-card {
  position: relative;
  display: block;
  height: 152px;
  box-sizing: border-box;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid #e7eaf2;
  border-radius: 18px;
  background: #fff;
  text-align: left;
  box-shadow: 0 8px 22px rgba(26, 36, 66, 0.07);
}
.capability-feature-cover {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: block;
  width: 100% !important;
  height: 100% !important;
  overflow: hidden;
}
.capability-feature-overlay {
  position: absolute;
  inset: 0;
  z-index: 2;
  background: linear-gradient(
    180deg,
    rgba(15, 23, 51, 0.12) 0%,
    rgba(15, 23, 51, 0.34) 54%,
    rgba(15, 23, 51, 0.78) 100%
  );
}
.capability-feature-copy {
  position: absolute;
  z-index: 3;
  right: 12px;
  bottom: 12px;
  left: 12px;
  min-width: 0;
}
.feature-title,
.feature-subtitle,
.feature-action {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.feature-title {
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  line-height: 23px;
}
.feature-subtitle {
  margin-top: 2px;
  color: rgba(255, 255, 255, 0.84);
  font-size: 10px;
  line-height: 16px;
}
.feature-action {
  width: 74px;
  height: 28px;
  margin-top: 12px;
  border-radius: 14px;
  color: #4a6cff;
  background: #fff;
  font-size: 10px;
  font-weight: 600;
  line-height: 28px;
  text-align: center;
}
.capability-secondary-grid {
  display: grid;
  margin-top: 8px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}
.capability-secondary-card {
  display: grid;
  height: 64px;
  box-sizing: border-box;
  margin: 0;
  padding: 10px 11px;
  grid-template-columns: 34px minmax(0, 1fr) 13px;
  align-items: center;
  gap: 8px;
  border: 1px solid #e7eaf2;
  border-radius: 14px;
  background: #fff;
  text-align: left;
  box-shadow: 0 6px 16px rgba(26, 36, 66, 0.06);
}
.secondary-icon,
.compact-icon {
  display: grid;
  place-items: center;
  font-weight: 700;
}
.secondary-icon {
  width: 34px;
  height: 34px;
  border-radius: 11px;
  font-size: 14px;
}
.secondary-icon.ppt {
  color: #ff7a1a;
  background: #fff2e8;
}
.secondary-icon.office {
  color: #19b86d;
  background: #eafaf2;
}
.secondary-copy,
.compact-copy {
  min-width: 0;
}
.secondary-title,
.secondary-subtitle,
.compact-title,
.compact-subtitle {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.secondary-title {
  color: #181c28;
  font-size: 12px;
  font-weight: 700;
  line-height: 18px;
}
.secondary-subtitle {
  margin-top: 2px;
  color: #697084;
  font-size: 10px;
  line-height: 14px;
}
.secondary-chevron {
  width: 13px;
  height: 13px;
  opacity: 0.6;
}
.capability-compact-row {
  display: grid;
  margin-top: 8px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 7px;
}
.capability-compact-card {
  display: grid;
  height: 58px;
  box-sizing: border-box;
  margin: 0;
  padding: 8px 6px;
  grid-template-columns: 26px minmax(0, 1fr);
  align-items: center;
  gap: 5px;
  border: 1px solid #e7eaf2;
  border-radius: 13px;
  background: #fff;
  text-align: left;
  box-shadow: 0 6px 16px rgba(26, 36, 66, 0.05);
}
.compact-icon {
  width: 26px;
  height: 26px;
  border-radius: 9px;
  font-size: 10px;
  line-height: 1;
}
.compact-icon.tone-blue {
  color: #4a6cff;
  background: #eef2ff;
}
.compact-icon.tone-purple {
  color: #7c5cff;
  background: #f2edff;
}
.compact-icon.tone-green {
  color: #19b86d;
  background: #eafaf2;
}
.compact-icon.tone-dark {
  color: #181c28;
  background: #f0f2f7;
}
.compact-title {
  color: #181c28;
  font-size: 10px;
  font-weight: 700;
  line-height: 14px;
}
.compact-subtitle {
  color: #697084;
  font-size: 10px;
  line-height: 11px;
}
.project-section-heading {
  display: flex;
  margin: 18px 0 9px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.project-section-title {
  display: block;
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 25px;
}
.project-section-action {
  width: auto;
  height: 24px;
  margin: 0;
  padding: 0 2px;
  border: 0;
  color: #8b95a7;
  background: transparent;
  font-size: 10px;
  line-height: 24px;
}
.project-section-action::after {
  display: none;
}
.project-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 7px 10px;
  border: 1px solid #e7eaf2;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 8px 20px rgba(26, 36, 66, 0.05);
}
.project-empty { display: flex; min-height: 82px; padding: 14px; align-items: center; justify-content: space-between; gap: 12px; border: 1px dashed #d9deea; border-radius: 12px; color: #697084; background: #fff; font-size: 11px; }
.project-empty button { width: auto; min-width: 82px; height: 32px; margin: 0; padding: 0 12px; border: 0; border-radius: 16px; color: #fff; background: #4a6cff; font-size: 11px; line-height: 32px; }
.project-empty button::after { display: none; }
.project-card {
  position: relative;
  display: grid;
  width: 100%;
  height: 54px;
  margin: 0;
  padding: 7px 0;
  box-sizing: border-box;
  grid-template-columns: minmax(0, 1fr) 58px 56px 66px;
  align-items: center;
  column-gap: 8px;
  border: 0;
  border-bottom: 1px solid #eef1f7;
  border-radius: 0;
  background: transparent;
  text-align: left;
  box-shadow: none;
}
.project-card:last-child {
  border-bottom: 0;
}
.project-copy {
  min-width: 0;
}
.project-name,
.project-meta,
.project-percent {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.project-name {
  color: #181c28;
  font-size: 11px;
  font-weight: 700;
  line-height: 16px;
}
.project-meta {
  margin-top: 2px;
  color: #697084;
  font-size: 10px;
  line-height: 12px;
}
.project-progress {
  display: grid;
  width: 58px;
  justify-items: end;
}
.project-percent {
  color: #181c28;
  font-size: 10px;
  font-weight: 700;
  line-height: 14px;
  text-align: right;
}
.progress {
  width: 52px;
  height: 3px;
  margin-top: 6px;
  overflow: hidden;
  border-radius: 3px;
  background: #e6e9f2;
}
.project-media {
  width: 56px;
  height: 38px;
}
.project-thumb {
  width: 56px !important;
  height: 38px !important;
}
.project-action {
  display: block;
  width: 66px;
  height: 28px;
  overflow: hidden;
  border-radius: 15px;
  color: #fff;
  background: #4a6cff;
  font-size: 10px;
  font-weight: 600;
  line-height: 28px;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.progress-fill {
  height: 100%;
  border-radius: 3px;
  background: #4a6cff;
}
.employee-section-heading {
  display: flex;
  margin: 22px 0 9px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.employee-section-title {
  display: block;
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 25px;
}
.employee-section-action {
  width: auto;
  height: 24px;
  margin: 0;
  padding: 0 2px;
  border: 0;
  color: #8b95a7;
  background: transparent;
  font-size: 10px;
  line-height: 24px;
}
.employee-section-action::after {
  display: none;
}
.horizontal-scroll {
  width: calc(100% + 20px);
  overflow: hidden;
}
.employee-row,
.inspiration-row {
  display: flex;
  width: max-content;
  padding-right: 20px;
  gap: 10px;
}
.employee-row {
  gap: 9px;
}
.employee-card {
  display: block;
  width: 104px;
  height: 132px;
  margin: 0;
  padding: 0;
  box-sizing: border-box;
  overflow: hidden;
  border: 1px solid #e7eaf2;
  border-radius: 8px;
  background: #fff;
  text-align: left;
  box-shadow: 0 6px 16px rgba(26, 36, 66, 0.06);
}
.employee-avatar {
  display: block;
  width: 100% !important;
  height: 78px !important;
  background: #eef1f7;
}
.employee-copy {
  min-width: 0;
  padding: 8px 9px 9px;
}
.employee-name,
.employee-status {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.employee-name {
  color: #111827;
  font-size: 11px;
  font-weight: 700;
  line-height: 16px;
}
.employee-status-row {
  display: flex;
  min-width: 0;
  margin-top: 5px;
  align-items: center;
  gap: 4px;
}
.employee-status-dot {
  width: 6px;
  min-width: 6px;
  height: 6px;
  border-radius: 50%;
}
.employee-status-dot.online {
  background: #22c55e;
}
.employee-status-dot.busy {
  background: #ff8a00;
}
.employee-status-dot.standby {
  background: #c5ccd8;
}
.employee-status {
  color: #8b95a7;
  font-size: 10px;
  font-weight: 700;
  line-height: 12px;
}
.inspiration-section-heading {
  display: flex;
  margin: 22px 0 8px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.inspiration-section-title {
  display: block;
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 25px;
}
.inspiration-refresh {
  width: auto;
  height: 24px;
  margin: 0;
  padding: 0 2px;
  border: 0;
  color: #8b95a7;
  background: transparent;
  font-size: 10px;
  line-height: 24px;
}
.inspiration-refresh::after,
.inspiration-more::after,
.home-inspiration-state button::after {
  display: none;
}
.inspiration-heading-actions { display: flex; align-items: center; gap: 12px; }
.inspiration-more { width: auto; height: 24px; margin: 0; padding: 0; border: 0; color: #4a6cff; background: transparent; font-size: 10px; line-height: 24px; }
.inspiration-tab-scroll {
  width: calc(100% + 20px);
  overflow: hidden;
}
.inspiration-tabs {
  display: flex;
  width: max-content;
  padding-right: 20px;
  gap: 8px;
}
.inspiration-tab {
  width: auto;
  height: 28px;
  margin: 0;
  padding: 0 13px;
  border: 0;
  border-radius: 14px;
  color: #697084;
  background: transparent;
  font-size: 10px;
  font-weight: 600;
  line-height: 28px;
}
.inspiration-tab::after {
  display: none;
}
.inspiration-tab.active {
  color: #4a6cff;
  background: #e9eeff;
}
.home-inspiration-grid { display: grid; margin-top: 11px; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; gap: 10px; }
.home-inspiration-skeleton { display: grid; margin-top: 11px; grid-template-columns: repeat(2, 1fr); gap: 10px; }
.home-inspiration-skeleton view { height: 244px; border-radius: 8px; background: #e9edf5; animation: inspiration-pulse 1.1s infinite; }
.home-inspiration-state { display: flex; min-height: 130px; margin-top: 11px; align-items: center; justify-content: center; flex-direction: column; border: 1px solid #e8eaf1; border-radius: 8px; color: #8a92a5; background: #fff; font-size: 11px; }
.home-inspiration-state button { width: auto; height: 32px; margin-top: 10px; padding: 0 14px; border: 0; border-radius: 16px; color: #fff; background: #4a6cff; font-size: 10px; }
@keyframes inspiration-pulse { 50% { opacity: 0.55; } }
.home-footer {
  margin-top: 56px;
  padding: 28px 18px;
  border-radius: 20px;
  background: #eef1ff;
}
.footer-title,
.footer-subtitle {
  display: block;
}
.footer-title {
  font-size: 16px;
  font-weight: 600;
}
.footer-subtitle {
  margin-top: 8px;
  color: #697084;
  font-size: 10px;
}
</style>
