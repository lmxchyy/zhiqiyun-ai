<template>
  <AssetDetailCenterPage
    v-if="enterpriseAssetCenter"
    :asset-id="enterpriseAssetId"
    :autoplay="enterpriseAutoplay"
  />
  <view v-else class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserAssetsPage')">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy">
        <text class="mpb-title">作品详情</text>
        <text class="mpb-subtitle">作品文件与生成参数</text>
      </view>
      <text class="mpb-role">普通用户</text>
    </view>

    <view class="mpb-stack">
      <view v-if="loading" class="mpb-card mpb-empty">
        <text class="mpb-empty-title">正在加载作品...</text>
      </view>
      <template v-else-if="asset">
        <image v-if="mediaType === 'image' && previewUrl" class="asset-preview" :src="previewUrl" mode="aspectFit" @click="previewAsset" />
        <video v-else-if="mediaType === 'video' && previewUrl" class="asset-preview" :src="previewUrl" controls />
        <view v-else class="mpb-hero light">
          <view class="mpb-hero-top">
            <view>
              <text class="mpb-hero-label">作品类型</text>
              <text class="mpb-hero-value">{{ mediaTypeLabel }}</text>
            </view>
            <text class="mpb-hero-badge purple">{{ rowString(asset, "status") || "已保存" }}</text>
          </view>
          <text class="mpb-hero-copy">当前作品没有可直接预览的媒体地址，生成信息仍可在下方查看。</text>
        </view>

        <view class="mpb-card mpb-list">
          <view class="mpb-section-head">
            <text class="mpb-card-title">{{ assetTitle }}</text>
            <text class="mpb-card-copy">接口实时数据</text>
          </view>
          <view class="mpb-row"><text class="mpb-row-icon">号</text><view class="mpb-row-main"><text class="mpb-row-title">作品编号</text><text class="mpb-row-meta">{{ assetId }}</text></view></view>
          <view class="mpb-row"><text class="mpb-row-icon green">型</text><view class="mpb-row-main"><text class="mpb-row-title">媒体类型</text><text class="mpb-row-meta">{{ mediaTypeLabel }}</text></view><text class="mpb-status success">{{ rowString(asset, "status") || "可用" }}</text></view>
          <view class="mpb-row"><text class="mpb-row-icon orange">模</text><view class="mpb-row-main"><text class="mpb-row-title">生成模型</text><text class="mpb-row-meta">{{ rowString(asset, "model", "modelName") || "自动匹配" }}</text></view></view>
          <view class="mpb-row"><text class="mpb-row-icon">时</text><view class="mpb-row-main"><text class="mpb-row-title">创建时间</text><text class="mpb-row-meta">{{ formatDate(rowString(asset, "createdAt", "updatedAt")) }}</text></view><text class="mpb-amount">{{ formatNumber(rowNumber(asset, "pointCost", "points")) }} 点</text></view>
        </view>

        <view v-if="rowString(asset, 'prompt')" class="mpb-note">{{ rowString(asset, "prompt") }}</view>

        <view class="mpb-inline-actions asset-actions">
          <button class="mpb-button secondary" :disabled="!previewUrl" @click="previewAsset">预览</button>
          <button class="mpb-button secondary" :disabled="!assetLink" @click="copyAssetLink">复制链接</button>
        </view>
        <view class="mpb-inline-actions asset-actions">
          <button class="mpb-button" :disabled="downloading" @click="downloadAsset">{{ downloading ? "下载中..." : "下载作品" }}</button>
          <button class="mpb-button danger" :disabled="deleting" @click="deleteAsset">{{ deleting ? "删除中..." : "删除作品" }}</button>
        </view>
        <button class="mpb-button secondary" @click="continueCreation">继续创作</button>
      </template>
      <view v-else class="mpb-card mpb-empty">
        <text class="mpb-empty-title">未找到作品</text>
        <text class="mpb-empty-copy">作品可能已删除，或当前账户无权查看。</text>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad, onShareAppMessage } from "@dcloudio/uni-app";
import { api, getApiBaseURL } from "../../api/client";
import { downloadTemporaryFile } from "../../api/files";
import { asRecord, backOrHome, formatDate, formatNumber, rowNumber, rowString, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import AssetDetailCenterPage from "../../components/assets/AssetDetailCenterPage.vue";

const enterpriseAssetCenter = true;

const id = ref("");
const enterpriseAssetId = ref("");
const enterpriseAutoplay = ref(false);
const loading = ref(false);
const downloading = ref(false);
const deleting = ref(false);
const asset = ref<AnyRecord | null>(null);
const assetId = computed(() => rowString(asset.value, "id", "assetId") || id.value);
const assetTitle = computed(() => rowString(asset.value, "name", "title") || "未命名作品");
const mediaType = computed(() => rowString(asset.value, "mediaType", "type").toLowerCase());
const mediaTypeLabel = computed(() => mediaType.value === "video" ? "视频" : mediaType.value === "image" ? "图片" : mediaType.value.includes("ppt") ? "PPT" : mediaType.value || "文件");
const assetLink = computed(() => rowString(asset.value, "url", "outputUrl", "fileUrl", "downloadUrl"));
const previewUrl = computed(() => rowString(asset.value, "thumbnailUrl", "previewUrl") || assetLink.value);
const downloadURL = computed(() => {
  const base = getApiBaseURL();
  return assetId.value ? `${base}/api/v1/assets/${encodeURIComponent(assetId.value)}/download` : "";
});

async function load() {
  loading.value = true;
  try {
    const payload = asRecord(await api(`/api/v1/assets/${encodeURIComponent(id.value)}`));
    const item = asRecord(payload.item);
    asset.value = rowString(item, "id", "assetId") ? item : null;
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "作品加载失败", icon: "none" });
  } finally {
    loading.value = false;
  }
}

function previewAsset() {
  if (!previewUrl.value) {
    uni.showToast({ title: "当前作品暂无可预览地址", icon: "none" });
    return;
  }
  if (mediaType.value === "image") {
    uni.previewImage({ urls: [previewUrl.value], current: previewUrl.value });
    return;
  }
  if (mediaType.value === "video") {
    uni.showToast({ title: "视频可在当前页面直接播放", icon: "none" });
    return;
  }
  uni.showToast({ title: "该类型请下载后查看", icon: "none" });
}

function copyAssetLink() {
  const link = assetLink.value || downloadURL.value;
  if (!link) {
    uni.showToast({ title: "当前作品暂无可复制地址", icon: "none" });
    return;
  }
  uni.setClipboardData({
    data: link,
    success: () => uni.showToast({ title: "作品链接已复制", icon: "success" })
  });
}

async function downloadAsset() {
  if (!downloadURL.value || downloading.value) return;
  downloading.value = true;
  try {
    openDownloadedFile(await downloadTemporaryFile(downloadURL.value));
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "下载失败，请检查网络", icon: "none" });
  } finally {
    downloading.value = false;
  }
}

function openDownloadedFile(filePath: string) {
  if (mediaType.value === "image") {
    uni.saveImageToPhotosAlbum({
      filePath,
      success: () => uni.showToast({ title: "图片已保存", icon: "success" }),
      fail: () => uni.previewImage({ urls: [filePath], current: filePath })
    });
    return;
  }
  if (mediaType.value === "video") {
    uni.saveVideoToPhotosAlbum({
      filePath,
      success: () => uni.showToast({ title: "视频已保存", icon: "success" }),
      fail: () => uni.openDocument({ filePath, fail: () => uni.showToast({ title: "已下载，打开失败", icon: "none" }) })
    });
    return;
  }
  uni.openDocument({
    filePath,
    showMenu: true,
    fail: () => uni.showToast({ title: "已下载，当前文件类型无法预览", icon: "none" })
  });
}

function deleteAsset() {
  if (!assetId.value || deleting.value) return;
  uni.showModal({
    title: "删除作品",
    content: "删除后作品将从列表移除，生成消耗记录不会被删除。是否继续？",
    confirmColor: "#ef4444",
    success: result => {
      if (!result.confirm) return;
      void confirmDeleteAsset();
    }
  });
}

async function confirmDeleteAsset() {
  deleting.value = true;
  try {
    await api(`/api/v1/assets/${encodeURIComponent(assetId.value)}`, { method: "DELETE" });
    uni.showToast({ title: "作品已删除", icon: "success" });
    uni.redirectTo({
      url: "/pages/user/UserAssetsPage",
      fail: () => uni.reLaunch({ url: "/pages/user/UserAssetsPage" })
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "删除失败，请稍后重试", icon: "none" });
  } finally {
    deleting.value = false;
  }
}

function continueCreation() {
  const mode = mediaType.value === "video" ? "UserVideoCreationPage" : mediaType.value.includes("ppt") ? "UserPptCreationPage" : "UserImageCreationPage";
  uni.navigateTo({ url: `/pages/user/${mode}` });
}

onShareAppMessage(() => ({
  title: assetTitle.value,
  path: `/pages/user/UserAssetDetailPage?id=${encodeURIComponent(assetId.value)}`
}));

onLoad(options => {
  const routeId = String(options?.id || "").trim();
  if (enterpriseAssetCenter) {
    enterpriseAssetId.value = routeId;
    enterpriseAutoplay.value = String(options?.autoplay || "") === "1";
    return;
  }
  id.value = routeId;
  void load();
});
</script>

<style>
@import "../../styles/mini-program-business.css";
.asset-preview { display: block; width: 100%; height: 260px; border: 1px solid #e5eaf6; border-radius: 8px; background: #fff; }
.asset-actions { gap: 10px; }
.asset-actions .mpb-button { flex: 1; }
</style>
