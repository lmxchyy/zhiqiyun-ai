<template>
  <view class="mpb-page detail-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回" @click="back">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy"><text class="mpb-title">灵感详情</text><text class="mpb-subtitle">从示例开始你的创作</text></view>
      <button class="share-button mpb-role" open-type="share">分享</button>
    </view>

    <view class="mpb-stack detail-stack">
      <view v-if="loading" class="detail-skeleton"><view /><text /><text /></view>
      <view v-else-if="error" class="detail-state">
        <text>模板暂时无法打开</text><small>{{ error }}</small><button @click="load">重新加载</button>
      </view>
      <template v-else-if="detail">
        <view class="result-preview">
          <video v-if="item.contentType === 'video'" :src="item.resultUrl || item.coverUrl" :poster="item.coverUrl" controls :autoplay="false" object-fit="cover" />
          <AppImage v-else :src="item.resultUrl || item.coverUrl" :fallback="item.coverUrl" :alt="item.title" width="100%" height="100%" radius="0" />
          <text class="preview-label">{{ previewLabel }}</text>
        </view>

        <view class="content-card summary-card">
          <view class="title-row">
            <view><text class="type-label">{{ typeLabel }}</text><text class="title">{{ item.title }}</text></view>
            <button :class="{ active: item.favorite }" @click="toggleFavorite">{{ item.favorite ? "♥" : "♡" }} {{ item.favoriteCount }}</button>
          </view>
          <text class="description">{{ item.description }}</text>
          <view v-if="item.tags?.length" class="tag-row"><text v-for="tag in item.tags" :key="tag">{{ tag }}</text></view>
          <view class="usage-row"><text>浏览 {{ item.viewCount }}</text><text>使用 {{ item.useCount }}</text><text>生成 {{ item.generateCount }}</text></view>
        </view>

        <TemplateInputForm
          :inputs="item.schema.inputs"
          :values="values"
          :assets="assets"
          :errors="formErrors"
          @update:values="values = $event"
          @update:assets="assets = $event"
        />

        <view class="safe-space" />
        <view class="fixed-action">
          <view><text>{{ item.schema.inputs.length ? "填写完成后进入创作页确认" : "进入创作页继续设置" }}</text><small>模型、积分和生成将在下一步确认</small></view>
          <button class="generate-action" :loading="using" :disabled="using" @click="useTemplate">{{ using ? "正在准备" : "使用此模板" }}</button>
        </view>
      </template>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad, onShareAppMessage } from "@dcloudio/uni-app";
import AppImage from "../../components/AppImage.vue";
import TemplateInputForm from "./components/TemplateInputForm.vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { hasValidToken, requireAuth } from "../../features/auth/gate";
import { inspirationAPI } from "../../features/inspiration/api";
import {
  buildInspirationComposeRequest,
  inspirationComposeErrorAction,
  templateInitialValues,
  validateTemplateInputValues,
} from "../../features/inspiration/contracts";
import { saveInspirationDraft } from "../../features/inspiration/draft";
import { completeInspirationHandoff } from "../../features/inspiration/handoff";
import type { InspirationDetailResponse, TemplateAssetValues } from "../../features/inspiration/types";

const slug = ref("");
const detail = ref<InspirationDetailResponse | null>(null);
const loading = ref(true);
const using = ref(false);
const error = ref("");
const values = ref<Record<string, unknown>>({});
const assets = ref<TemplateAssetValues>({});
const formErrors = ref<Record<string, string>>({});
const item = computed(() => detail.value!.item);
const typeLabel = computed(() => ({ image: "AI 图片", video: "AI 视频", ppt: "PPT 方案", text: "AI 文本", agent: "AI Agent", workflow: "AI 工作流" }[item.value.contentType]));
const previewLabel = computed(() => {
  const label = item.value.schema.presentation.heroLabel;
  return typeof label === "string" && label.trim() ? label : "AI 生成示例";
});

async function load() {
  if (!slug.value) return;
  loading.value = true;
  error.value = "";
  try {
    detail.value = await inspirationAPI.detail(slug.value);
    values.value = templateInitialValues(detail.value.item.schema.inputs, detail.value.item.schema.presets.inputDefaults);
    assets.value = {};
    formErrors.value = {};
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "请稍后重试";
  } finally {
    loading.value = false;
  }
}

async function requestLogin(resume: () => void | Promise<void>) {
  await requireAuth({
    action: item.value.contentType === "video" ? "generate_video" : item.value.contentType === "ppt" ? "generate_ppt" : "generate_image",
    route: "/pages/inspiration/InspirationDetailPage",
    payload: { slug: slug.value },
    resume,
  });
}

async function toggleFavorite() {
  if (!hasValidToken()) {
    await requestLogin(toggleFavorite);
    return;
  }
  try {
    const next = !item.value.favorite;
    await inspirationAPI.favorite(slug.value, next);
    item.value.favorite = next;
    item.value.favoriteCount = Math.max(0, item.value.favoriteCount + (next ? 1 : -1));
  } catch (reason) {
    uni.showToast({ title: reason instanceof Error ? reason.message : "收藏失败", icon: "none" });
  }
}

async function handleComposeError(reason: unknown) {
  const action = inspirationComposeErrorAction(reason);
  if (action === "auth") {
    await requestLogin(compose);
    return;
  }
  if (action === "reload") {
    uni.showModal({
      title: "模板已更新",
      content: "当前模板已有新版本，请重新加载后再填写。",
      showCancel: false,
      success: () => { void load(); },
    });
    return;
  }
  const fallback = action === "input" ? "请检查填写内容"
    : action === "material" ? "请检查上传素材"
      : action === "schema" ? "模板配置暂不可用"
        : action === "network" ? "网络连接失败，请重试"
          : "暂时无法使用此模板";
  uni.showToast({ title: reason instanceof Error && reason.message ? reason.message : fallback, icon: "none", duration: 2400 });
}

async function compose() {
  if (using.value || !detail.value) return;
  formErrors.value = validateTemplateInputValues(item.value.schema.inputs, values.value, assets.value);
  const firstError = Object.values(formErrors.value)[0];
  if (firstError) {
    uni.showToast({ title: firstError, icon: "none" });
    return;
  }
  using.value = true;
  try {
    const body = buildInspirationComposeRequest(item.value.templateVersion, values.value, assets.value);
    const response = await inspirationAPI.compose(slug.value, body);
    await completeInspirationHandoff(response.draft, {
      save: saveInspirationDraft,
      recordUse: () => inspirationAPI.event(slug.value, "use_template"),
      navigate: url => uni.navigateTo({ url }),
    });
  } catch (reason) {
    await handleComposeError(reason);
  } finally {
    using.value = false;
  }
}

async function useTemplate() {
  if (!hasValidToken()) {
    await requestLogin(compose);
    return;
  }
  await compose();
}

function back() {
  uni.navigateBack({ fail: () => uni.switchTab({ url: "/pages/user/UserHomePage" }) });
}

onLoad(options => {
  slug.value = String(options?.slug || "").trim();
  if (!slug.value) {
    error.value = "模板参数缺失";
    loading.value = false;
    return;
  }
  void load();
});

onShareAppMessage(() => ({
  title: item.value?.title || "知启云 AI 创作灵感",
  path: `/pages/inspiration/InspirationDetailPage?slug=${encodeURIComponent(slug.value)}`,
  imageUrl: item.value?.coverUrl,
}));
</script>

<style scoped>
@import "../../styles/mini-program-business.css";
.detail-page{min-height:100vh;color:#171c2d;background:#f5f6fa}.detail-stack{padding-bottom:0}.share-button{width:auto}.result-preview{position:relative;height:410px;overflow:hidden;background:#e9edf5}.result-preview video{width:100%;height:100%}.preview-label{position:absolute;z-index:4;right:14px;top:14px;padding:6px 9px;border-radius:6px;color:#fff;background:rgba(18,24,43,.72);font-size:10px}.content-card{margin:12px 0 0;padding:17px;border:1px solid #e8eaf1;border-radius:13px;background:#fff}.summary-card{margin-top:12px}.title-row{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.type-label,.title,.description{display:block}.type-label{color:#4a6cff;font-size:10px;font-weight:650}.title{margin-top:4px;font-size:20px;font-weight:750;line-height:28px}.title-row button{width:auto;height:36px;padding:0 11px;border-radius:18px;color:#727c91;background:#f3f5fa;font-size:12px}.title-row button.active{color:#f04e64;background:#fff0f2}.description{margin-top:10px;color:#737c90;font-size:12px;line-height:20px}.tag-row{display:flex;flex-wrap:wrap;margin-top:12px;gap:6px}.tag-row text{padding:4px 8px;border-radius:10px;color:#5f6a80;background:#f1f3f7;font-size:9px}.usage-row{display:flex;margin-top:14px;gap:18px;color:#9299aa;font-size:10px}.safe-space{height:105px}.fixed-action{position:fixed;z-index:12;right:0;bottom:0;left:0;display:grid;padding:11px 16px calc(11px + env(safe-area-inset-bottom));grid-template-columns:1fr 138px;align-items:center;gap:12px;background:rgba(255,255,255,.97);box-shadow:0 -6px 22px rgba(25,32,57,.08)}.fixed-action>view{display:flex;min-width:0;flex-direction:column}.fixed-action>view text{color:#424b5f;font-size:11px;font-weight:650}.fixed-action>view small{margin-top:3px;color:#9299a9;font-size:9px}.generate-action{height:46px;border-radius:23px;color:#fff;background:#4a6cff;font-size:14px;font-weight:650}.generate-action[disabled]{opacity:.65}.detail-skeleton,.detail-state{display:flex;min-height:500px;align-items:center;justify-content:center;flex-direction:column}.detail-skeleton view{width:100%;height:360px;background:#e8ebf2}.detail-skeleton text{width:80%;height:18px;margin-top:16px;background:#e8ebf2}.detail-state small{margin-top:8px;color:#8b93a5}.detail-state button{width:auto;margin-top:16px;padding:0 18px;border-radius:18px;color:#fff;background:#4a6cff}button:after{border:0}
</style>
