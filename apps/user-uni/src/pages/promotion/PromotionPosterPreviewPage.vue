<template>
  <view class="promotion-page has-fixed-action">
    <PromotionPageHeader title="海报预览" subtitle="1080 × 1440 PNG · 3:4" />
    <view class="promotion-content promotion-preview-content">
      <PromotionStatePanel v-if="loading" tone="loading" title="正在合成推广海报" description="正在加载品牌素材与专属小程序码" />
      <PromotionStatePanel v-else-if="error" tone="error" title="海报生成失败" :description="error" action-text="重新生成" @action="prepare(true)" />
      <template v-else-if="overview && qrPath">
        <PromotionPosterCanvas ref="canvasRef" :template="selectedTemplate" :profile="overview.profile" :qr-path="qrPath" @error="error = $event" />
        <view v-if="code?.isPlaceholder" class="promotion-dev-notice"><text>当前为开发联调码，正式发布必须配置微信 AppID 与 AppSecret。</text></view>
        <view class="promotion-preview-meta"><view><text>{{ selectedTemplate.name }}</text><text>{{ selectedTemplate.description }}</text></view><button class="promotion-text-button" @click="chooseTemplate"><text>更换模板</text></button></view>
        <view class="promotion-copy-card"><text>{{ shareCopy?.text || '专属推广文案加载中' }}</text><button class="promotion-text-button" @click="copyText"><text>复制文案</text></button></view>
      </template>
    </view>
    <view class="promotion-fixed-action promotion-preview-actions">
      <button class="promotion-secondary-button" :disabled="saving" @click="savePoster"><text>{{ saving ? '保存中…' : '保存到相册' }}</text></button>
      <button class="promotion-primary-button" open-type="share" @click="trackShare"><text>分享给好友</text></button>
    </view>
  </view>
</template>
<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { onLoad, onShareAppMessage, onShareTimeline } from "@dcloudio/uni-app";
import PromotionPageHeader from "../../components/promotion/PromotionPageHeader.vue";
import PromotionPosterCanvas from "../../components/promotion/PromotionPosterCanvas.vue";
import PromotionStatePanel from "../../components/promotion/PromotionStatePanel.vue";
import { promotionAPI } from "../../features/promotion/api";
import { trackPromotion } from "../../features/promotion/analytics";
import { imageDataUrlToLocalPath, savePosterToAlbum } from "../../features/promotion/platform";
import { promotionTemplateById } from "../../features/promotion/templates";
import type { PromotionCode, PromotionOverview, PromotionShareCopy, PromotionTemplateId } from "../../features/promotion/types";
import { useUserStore } from "../../stores/user";
const userStore = useUserStore(); const templateId = ref<PromotionTemplateId>("poster.brand.simple"); const overview = ref<PromotionOverview | null>(null); const code = ref<PromotionCode | null>(null); const shareCopy = ref<PromotionShareCopy | null>(null); const qrPath = ref(""); const posterPath = ref(""); const loading = ref(true); const saving = ref(false); const error = ref("");
const canvasRef = ref<InstanceType<typeof PromotionPosterCanvas> | null>(null); const selectedTemplate = computed(() => promotionTemplateById(templateId.value));
onLoad(options => { templateId.value = promotionTemplateById(String(options?.templateId || "")).id; void prepare(false); });
async function prepare(invalidate: boolean) { loading.value = true; error.value = ""; try { await userStore.loadProfile(); overview.value = await promotionAPI.overview(userStore.userId, userStore.tenantId, userStore.currentRole, invalidate); const [codePayload, copy] = await Promise.all([promotionAPI.code({ userId: userStore.userId, tenantId: userStore.tenantId, currentRole: userStore.currentRole, templateId: templateId.value, activityId: overview.value.activity?.id, invalidate }), promotionAPI.shareCopy(templateId.value), promotionAPI.renderConfig(templateId.value, overview.value.activity?.id)]); code.value = codePayload; shareCopy.value = copy; qrPath.value = await imageDataUrlToLocalPath(codePayload.imageDataUrl, codePayload.cacheKey); loading.value = false; await nextTick(); if (!canvasRef.value) throw new Error("海报画布初始化失败"); posterPath.value = await canvasRef.value.exportPoster(); trackPromotion("promotion_poster_generate", { templateId: templateId.value }); } catch (reason) { error.value = reason instanceof Error ? reason.message : "请稍后重试"; } finally { loading.value = false; } }
async function savePoster() { if (!canvasRef.value || saving.value) return; saving.value = true; try { posterPath.value = await canvasRef.value.exportPoster(); await savePosterToAlbum(posterPath.value); trackPromotion("promotion_poster_save", { templateId: templateId.value }); uni.showToast({ title: "已保存到相册", icon: "success" }); } catch (reason) { uni.showModal({ title: "保存失败", content: reason instanceof Error ? reason.message : "请检查相册权限后重试", showCancel: false }); } finally { saving.value = false; } }
function chooseTemplate() { uni.navigateTo({ url: `/pages/promotion/PromotionTemplateCenterPage?templateId=${encodeURIComponent(templateId.value)}` }); }
function copyText() { if (!shareCopy.value?.text) return; uni.setClipboardData({ data: shareCopy.value.text, success: () => uni.showToast({ title: "推广文案已复制", icon: "success" }) }); trackPromotion("promotion_copy", { type: "share_copy", templateId: templateId.value }); }
function trackShare() { trackPromotion("promotion_share", { templateId: templateId.value, channel: "wechat_friend" }); }
onShareAppMessage(() => ({ title: shareCopy.value?.title || selectedTemplate.value.title, path: shareCopy.value?.path || "/pages/promotion/PromotionLandingPage", imageUrl: posterPath.value || undefined }));
onShareTimeline(() => ({ title: shareCopy.value?.title || selectedTemplate.value.title, query: `invite=${overview.value?.profile.inviteCode || ""}&templateId=${templateId.value}&source=moments`, imageUrl: posterPath.value || undefined }));
</script>
<style>@import "../../styles/promotion-center.css";</style>
