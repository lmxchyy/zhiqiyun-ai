<template>
  <view :class="['promotion-page', { embedded }]">
    <PromotionPageHeader
      v-if="showHeader"
      title="我的推广码"
      subtitle="分享知启云AI，与好友一起高效创作"
      :show-back="showBack"
      :fallback="headerFallback"
    />
    <view class="promotion-content">
      <PromotionStatePanel v-if="loading && !overview" tone="loading" title="正在生成专属推广码" description="正在校验账号、角色与推广模板" />
      <PromotionStatePanel v-else-if="error && !overview" tone="error" title="推广中心加载失败" :description="error" action-text="重新加载" @action="load(true)" />
      <template v-else-if="overview">
        <view class="promotion-identity-card">
          <view class="promotion-avatar">{{ overview.profile.name.slice(0, 1) }}</view>
          <view class="promotion-identity-copy">
            <text class="promotion-identity-name">{{ overview.profile.name }}</text>
            <text class="promotion-identity-company">{{ overview.profile.companyName }}</text>
          </view>
          <text class="promotion-role-pill">{{ overview.profile.roleLabel }}</text>
        </view>

        <view class="promotion-code-card">
          <view class="promotion-card-heading">
            <view><text class="promotion-section-title">微信扫码，立即体验</text><text class="promotion-section-copy">小程序码已绑定你的专属邀请码，好友扫码即可注册并建立绑定关系</text></view>
            <button class="promotion-icon-button" :disabled="codeLoading" @click="refreshCode"><text>↻</text></button>
          </view>
          <view v-if="agentInvite?.inviteLink" class="promotion-invite-row promotion-h5-link">
            <view><text>安卓邀请链接</text><text class="promotion-section-copy">{{ agentInvite.inviteLink }}</text></view>
            <button class="promotion-text-button" @click="copyInviteLink"><text>复制链接</text></button>
          </view>
          <view class="promotion-code-stage">
            <view v-if="codeLoading" class="promotion-code-skeleton" />
            <image v-else-if="codePath" class="promotion-code-image" :src="codePath" mode="aspectFit" />
            <PromotionStatePanel v-else tone="error" title="小程序码生成失败" action-text="重试" @action="loadCode(true)" />
          </view>
          <view v-if="promotionCode?.isPlaceholder" class="promotion-dev-notice"><text>开发联调码 · 正式环境将由微信接口生成官方小程序码</text></view>
          <view class="promotion-invite-row"><text>专属邀请码</text><text class="promotion-invite-code">{{ overview.profile.inviteCode }}</text><button class="promotion-text-button" @click="copyInvite"><text>复制</text></button></view>
          <view class="promotion-action-grid">
            <button class="promotion-primary-button" open-type="share" @click="trackShare"><text>分享给好友</text></button>
            <button class="promotion-secondary-button" @click="openPreview"><text>生成推广海报</text></button>
          </view>
        </view>

        <view class="promotion-metric-card">
          <view v-for="metric in metrics" :key="metric.label" class="promotion-metric-item">
            <text class="promotion-metric-value">{{ metric.value }}</text><text class="promotion-metric-label">{{ metric.label }}</text>
          </view>
        </view>

        <view class="promotion-section-head"><view><text class="promotion-section-title">选择推广模板</text><text class="promotion-section-copy">不同场景使用不同画面与文案</text></view><button class="promotion-text-button" @click="openTemplates"><text>全部 10 套 ›</text></button></view>
        <scroll-view scroll-x class="promotion-template-scroll" :show-scrollbar="false">
          <view class="promotion-template-row">
            <PromotionTemplateThumb v-for="template in visibleTemplates.slice(0, 4)" :key="template.id" compact :template="template" :selected="selectedTemplateId === template.id" @select="selectTemplate" />
          </view>
        </scroll-view>

        <view class="promotion-link-card">
          <button class="promotion-link-row" @click="openRecords"><view class="promotion-link-icon lavender">录</view><view><text>推广记录</text><text>查看访问、注册与成交状态</text></view><text>›</text></button>
          <button class="promotion-link-row" @click="openStats"><view class="promotion-link-icon blue">数</view><view><text>推广数据</text><text>趋势、转化率与来源渠道</text></view><text>›</text></button>
        </view>
      </template>
      <PromotionStatePanel v-else tone="loading" title="正在准备推广中心" description="首次进入会自动加载专属推广码" action-text="立即加载" @action="load(true)" />
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { onLoad, onShareAppMessage, onShareTimeline, onShow } from "@dcloudio/uni-app";
import PromotionPageHeader from "./PromotionPageHeader.vue";
import PromotionStatePanel from "./PromotionStatePanel.vue";
import PromotionTemplateThumb from "./PromotionTemplateThumb.vue";
import { promotionAPI } from "../../features/promotion/api";
import { trackPromotion } from "../../features/promotion/analytics";
import { imageDataUrlToLocalPath } from "../../features/promotion/platform";
import { promotionTemplateById, promotionTemplatesForRole } from "../../features/promotion/templates";
import type { AgentInviteProfile, PromotionCode, PromotionOverview, PromotionShareCopy, PromotionTemplateId } from "../../features/promotion/types";
import { useUserStore } from "../../stores/user";

const props = withDefaults(defineProps<{
  embedded?: boolean;
  showHeader?: boolean;
  showBack?: boolean;
  headerFallback?: string;
  active?: boolean;
}>(), {
  embedded: false,
  showHeader: true,
  showBack: true,
  headerFallback: "/pages/user/UserMinePage",
  active: true,
});

const userStore = useUserStore();
const overview = ref<PromotionOverview | null>(null);
const promotionCode = ref<PromotionCode | null>(null);
const codePath = ref("");
const shareCopy = ref<PromotionShareCopy | null>(null);
const agentInvite = ref<AgentInviteProfile | null>(null);
const selectedTemplateId = ref<PromotionTemplateId>("poster.brand.simple");
const loading = ref(false);
const codeLoading = ref(false);
const error = ref("");
const visibleTemplates = computed(() => promotionTemplatesForRole(userStore.currentRole));
const metrics = computed(() => {
  const value = overview.value?.summary;
  return [
    { label: "访问", value: value?.visitCount || 0 },
    { label: "注册", value: value?.registerCount || 0 },
    { label: "成交", value: value?.paidCount || 0 },
    { label: "奖励", value: `¥${((value?.rewardAmountCents || 0) / 100).toFixed(2)}` },
  ];
});

async function load(force: boolean) {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    await userStore.loadProfile(force);
    overview.value = await promotionAPI.overview(userStore.userId, userStore.tenantId, userStore.currentRole, force);
    if (userStore.currentRole === "AGENT") {
      try { agentInvite.value = await promotionAPI.agentInviteProfile(); }
      catch { agentInvite.value = null; }
    } else {
      agentInvite.value = null;
    }
    if (!visibleTemplates.value.some(item => item.id === selectedTemplateId.value)) {
      selectedTemplateId.value = overview.value.defaultTemplateId;
    }
    await Promise.all([loadCode(force), loadShareCopy()]);
    trackPromotion("promotion_page_view", { role: userStore.currentRole });
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "请稍后重试";
  } finally {
    loading.value = false;
  }
}

async function loadCode(invalidate = false) {
  if (!overview.value || codeLoading.value) return;
  codeLoading.value = true;
  codePath.value = "";
  try {
    const code = await promotionAPI.code({
      userId: userStore.userId,
      tenantId: userStore.tenantId,
      currentRole: userStore.currentRole,
      templateId: selectedTemplateId.value,
      activityId: overview.value.activity?.id,
      invalidate,
    });
    promotionCode.value = code;
    codePath.value = await imageDataUrlToLocalPath(code.imageDataUrl, code.cacheKey);
    trackPromotion("promotion_code_ready", { templateId: selectedTemplateId.value, placeholder: code.isPlaceholder });
  } catch (reason) {
    uni.showToast({ title: reason instanceof Error ? reason.message : "小程序码生成失败", icon: "none" });
  } finally {
    codeLoading.value = false;
  }
}

async function loadShareCopy() {
  try { shareCopy.value = await promotionAPI.shareCopy(selectedTemplateId.value); }
  catch { shareCopy.value = null; }
}

async function selectTemplate(id: PromotionTemplateId) {
  selectedTemplateId.value = id;
  trackPromotion("promotion_template_select", { templateId: id });
  await Promise.all([loadCode(false), loadShareCopy()]);
}

function refreshCode() { void loadCode(true); }
function copyInvite() {
  if (!overview.value) return;
  uni.setClipboardData({ data: overview.value.profile.inviteCode, success: () => uni.showToast({ title: "邀请码已复制", icon: "success" }) });
  trackPromotion("promotion_copy", { type: "invite_code" });
}
function copyInviteLink() {
  if (!agentInvite.value) return;
  uni.setClipboardData({ data: agentInvite.value.inviteLink, success: () => uni.showToast({ title: "邀请链接已复制", icon: "success" }) });
  trackPromotion("promotion_copy", { type: "agent_h5_link" });
}
function openPreview() {
  uni.navigateTo({ url: `/pages/promotion/PromotionPosterPreviewPage?templateId=${encodeURIComponent(selectedTemplateId.value)}` });
}
function openTemplates() {
  uni.navigateTo({ url: `/pages/promotion/PromotionTemplateCenterPage?templateId=${encodeURIComponent(selectedTemplateId.value)}` });
}
function openRecords() {
  trackPromotion("promotion_records_view");
  uni.navigateTo({ url: "/pages/promotion/PromotionRecordsPage" });
}
function openStats() {
  trackPromotion("promotion_stats_view");
  uni.navigateTo({ url: "/pages/promotion/PromotionStatsPage" });
}
function trackShare() {
  trackPromotion("promotion_share", { templateId: selectedTemplateId.value, channel: "wechat_friend" });
}

onLoad(options => {
  if (props.embedded) return;
  selectedTemplateId.value = promotionTemplateById(String(options?.templateId || "")).id;
});

onMounted(() => {
  // Page lifecycle hooks (onShow) do not reliably run inside nested components.
  // Always bootstrap on mount so embedded workbench / AgentPromotionPage are not blank.
  if (props.active) void load(false);
});

onShow(() => {
  if (!props.embedded) void load(false);
});

watch(() => props.active, (value, previous) => {
  if (props.embedded && value && !previous) void load(false);
});

onShareAppMessage(() => ({
  title: shareCopy.value?.title || "知启云AI，让创意更高效",
  path: shareCopy.value?.path || `/pages/promotion/PromotionLandingPage?invite=${overview.value?.profile.inviteCode || ""}&templateId=${selectedTemplateId.value}`,
}));
onShareTimeline(() => ({
  title: shareCopy.value?.title || "知启云AI，让创意更高效",
  query: `invite=${overview.value?.profile.inviteCode || ""}&templateId=${selectedTemplateId.value}&source=moments`,
}));

defineExpose({ load, shareCopy, overview, selectedTemplateId });
</script>

<style>
@import "../../styles/promotion-center.css";
.promotion-page.embedded {
  min-height: auto;
  background: transparent;
}
.promotion-page.embedded .promotion-content {
  padding-left: 0;
  padding-right: 0;
}
</style>
