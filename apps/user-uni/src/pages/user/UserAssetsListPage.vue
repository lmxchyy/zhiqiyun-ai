<template>
  <AssetLibraryPage v-if="enterpriseAssetCenter" />
  <view v-else class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header">
      <button class="mpb-back" aria-label="返回作品页" @click="backOrHome('/pages/user/UserAssetsPage')">‹</button>
      <image class="mpb-logo" :src="loginLogo" mode="aspectFit" />
      <view class="mpb-header-copy">
        <text class="mpb-title">全部作品</text>
        <text class="mpb-subtitle">{{ total }} 个作品 · 按创建时间倒序</text>
      </view>
      <button class="mpb-link-button manage-button" @click="toggleManage">{{ manageMode ? '完成' : '管理' }}</button>
    </view>

    <view class="mpb-stack">

    <view v-if="error && !items.length" class="state-card error">
      <text>{{ error }}</text>
      <button class="state-button" @click="refresh">重新加载</button>
    </view>
    <view v-else-if="loading && !items.length" class="state-card"><text>正在加载作品...</text></view>
    <view v-else-if="items.length" class="asset-grid">
      <button
        v-for="item in items"
        :key="item.id"
        :class="['asset-card', { selected: selectedIds.includes(item.id) }]"
        @click="openAsset(item)"
      >
        <AppImage
          v-if="item.thumbnailUrl"
          :src="item.thumbnailUrl"
          :alt="item.name"
          width="100%"
          height="92px"
          radius="14px"
        />
        <view v-else :class="['asset-placeholder', typeTone(item.mediaType)]">
          <text>{{ typeSymbol(item.mediaType) }}</text>
        </view>
        <view v-if="manageMode" class="select-mark">{{ selectedIds.includes(item.id) ? '✓' : '' }}</view>
        <text class="asset-name">{{ item.name || '未命名作品' }}</text>
        <view class="asset-meta">
          <text>{{ typeLabel(item.mediaType) }}</text>
          <text>{{ dateLabel(item.createdAt) }}</text>
        </view>
      </button>
    </view>
    <view v-else class="state-card"><text>暂无作品</text></view>

    <view class="load-state">
      <text v-if="loadingMore">正在加载更多...</text>
      <button v-else-if="hasMore" @click="loadMore">加载更多</button>
      <text v-else-if="items.length">已加载全部 {{ total }} 个作品</text>
    </view>
    </view>

    <view v-if="manageMode" class="manage-bar">
      <text>已选择 {{ selectedIds.length }} 项</text>
      <button class="clear-button" @click="selectedIds = []">取消选择</button>
      <button class="delete-button" :disabled="!selectedIds.length || deleting" @click="deleteSelected">
        {{ deleting ? '删除中...' : '删除所选' }}
      </button>
    </view>
    <view class="bottom-safe" />
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad, onPullDownRefresh, onReachBottom, onShow } from "@dcloudio/uni-app";
import type { Asset } from "@xianzhi/shared-types";
import { api, businessSdk } from "../../api/client";
import AppImage from "../../components/AppImage.vue";
import { miniProgramFeaturePages } from "../../config/miniProgramPages";
import { backOrHome } from "../../utils/miniProgramBusiness";
import AssetLibraryPage from "../../components/assets/AssetLibraryPage.vue";
import { useAssetStore } from "../../stores/assets";
import type { AssetStatus, AssetType } from "../../features/assets/types";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const enterpriseAssetCenter = true;
const enterpriseStore = useAssetStore();
const enterpriseLoaded = ref(false);

const pageSize = 12;
const items = ref<Asset[]>([]);
const total = ref(0);
const hasMore = ref(false);
const loading = ref(false);
const loadingMore = ref(false);
const deleting = ref(false);
const error = ref("");
const manageMode = ref(false);
const selectedIds = ref<string[]>([]);

async function load(reset = false) {
  if ((reset && loading.value) || (!reset && (loadingMore.value || !hasMore.value))) return;
  if (reset) loading.value = true;
  else loadingMore.value = true;
  error.value = "";
  try {
    const offset = reset ? 0 : items.value.length;
    const page = await businessSdk.assets.listPage({ limit: pageSize, offset });
    items.value = reset ? page.items : mergeAssets(items.value, page.items);
    total.value = page.total;
    hasMore.value = page.hasMore;
  } catch (loadError) {
    error.value = loadError instanceof Error ? loadError.message : "作品加载失败";
  } finally {
    loading.value = false;
    loadingMore.value = false;
  }
}

function mergeAssets(current: Asset[], incoming: Asset[]) {
  const byId = new Map(current.map(item => [item.id, item]));
  incoming.forEach(item => byId.set(item.id, item));
  return [...byId.values()];
}

function refresh() {
  selectedIds.value = [];
  return load(true);
}

function loadMore() {
  return load(false);
}

function toggleManage() {
  manageMode.value = !manageMode.value;
  if (!manageMode.value) selectedIds.value = [];
}

function openAsset(item: Asset) {
  if (manageMode.value) {
    selectedIds.value = selectedIds.value.includes(item.id)
      ? selectedIds.value.filter(id => id !== item.id)
      : [...selectedIds.value, item.id];
    return;
  }
  uni.navigateTo({ url: `${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(item.id)}` });
}

function deleteSelected() {
  if (!selectedIds.value.length || deleting.value) return;
  uni.showModal({
    title: "删除所选作品",
    content: `确定删除已选择的 ${selectedIds.value.length} 个作品吗？删除后将从作品列表移除。`,
    confirmColor: "#ef4444",
    success: result => { if (result.confirm) void confirmDeleteSelected(); },
  });
}

async function confirmDeleteSelected() {
  deleting.value = true;
  try {
    for (const id of selectedIds.value) {
      await api(`/api/v1/assets/${encodeURIComponent(id)}`, { method: "DELETE" });
    }
    uni.showToast({ title: "作品已删除", icon: "success" });
    selectedIds.value = [];
    await load(true);
  } catch (deleteError) {
    uni.showToast({ title: deleteError instanceof Error ? deleteError.message : "删除失败", icon: "none" });
  } finally {
    deleting.value = false;
  }
}

function normalizedType(mediaType?: string) {
  const value = String(mediaType || "").toLowerCase();
  if (value.includes("video")) return "video";
  if (value.includes("ppt") || value.includes("presentation")) return "ppt";
  if (value.includes("agent")) return "agent";
  if (value.includes("document") || value.includes("pdf")) return "document";
  return "image";
}

function typeLabel(mediaType?: string) {
  return ({ image: "图片", video: "视频", ppt: "PPT", agent: "Agent", document: "文档" } as Record<string, string>)[normalizedType(mediaType)];
}

function typeSymbol(mediaType?: string) {
  return ({ image: "图", video: "▶", ppt: "P", agent: "A", document: "文" } as Record<string, string>)[normalizedType(mediaType)];
}

function typeTone(mediaType?: string) {
  const type = normalizedType(mediaType);
  return type === "video" ? "orange" : type === "agent" ? "green" : "purple";
}

function dateLabel(value?: string) {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) return "刚刚";
  return `${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

onLoad(options => {
  if (enterpriseAssetCenter) {
    const type = String(options?.type || "") as AssetType;
    const status = String(options?.status || options?.filter || "") as AssetStatus;
    if (["all", "image", "video", "ppt", "document", "agent", "infographic", "knowledge", "prompt", "template"].includes(type)) {
      enterpriseStore.filters.type = type;
    }
    if (["recent", "queued", "generating", "completed", "failed", "favorite", "archived", "recycled"].includes(status)) {
      enterpriseStore.filters.status = status;
      enterpriseStore.filters.favorite = status === "favorite" ? true : undefined;
    }
    enterpriseStore.persistPreferences();
    enterpriseStore.setMultiSelectMode(String(options?.manage || "") === "1");
    void enterpriseStore.refreshAssets(20).then(async () => {
      if (enterpriseStore.pagination.pageSize < 20) await enterpriseStore.fetchAssets({ reset: true, pageSize: 20 });
    }).finally(() => { enterpriseLoaded.value = true; });
    return;
  }
  manageMode.value = String(options?.manage || "") === "1";
  void load(true);
});
onShow(() => {
  if (enterpriseAssetCenter && enterpriseLoaded.value) void enterpriseStore.refreshAssets(20, { silent: true });
});
onReachBottom(() => {
  if (enterpriseAssetCenter) void enterpriseStore.loadMoreAssets();
  else void loadMore();
});
onPullDownRefresh(() => {
  const request = enterpriseAssetCenter ? enterpriseStore.refreshAssets(20) : refresh();
  void request.finally(() => uni.stopPullDownRefresh());
});
</script>

<style scoped>
@import "../../styles/mini-program-business.css";

.manage-button { width: auto; height: 34px; padding: 0 12px; border-radius: 17px; color: #594db2; background: #fff; font-size: 12px; line-height: 34px; }
.asset-grid { display: grid; padding-top: 12px; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.asset-card { position: relative; min-width: 0; padding: 10px; overflow: hidden; border: 1px solid #e3e5f0; border-radius: 16px; background: #fff; text-align: left; }
.asset-card.selected { border-color: #7d8cf5; box-shadow: 0 0 0 2px rgba(125, 140, 245, .14); }
.asset-placeholder { display: flex; width: 100%; height: 92px; align-items: center; justify-content: center; border-radius: 14px; color: #594db2; background: #f0f2ff; font-size: 30px; font-weight: 700; }
.asset-placeholder.orange { color: #ff781c; background: #fff2e5; }
.asset-placeholder.green { color: #198c62; background: #eaf8f1; }
.select-mark { position: absolute; top: 16px; right: 16px; display: flex; width: 22px; height: 22px; align-items: center; justify-content: center; border: 2px solid #fff; border-radius: 50%; color: #fff; background: #7d8cf5; font-size: 12px; }
.asset-name { display: block; margin-top: 9px; overflow: hidden; font-size: 13px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.asset-meta { display: flex; margin-top: 5px; justify-content: space-between; color: #6e758c; font-size: 10px; }
.state-card { display: flex; min-height: 320px; flex-direction: column; align-items: center; justify-content: center; color: #6e758c; font-size: 12px; text-align: center; }
.state-card.error { color: #d84a42; }
.state-button { width: auto; height: 36px; margin-top: 12px; padding: 0 16px; border-radius: 18px; color: #fff; background: #7d8cf5; font-size: 12px; }
.load-state { display: flex; min-height: 72px; align-items: center; justify-content: center; color: #8b91a5; font-size: 11px; }
.load-state button { width: auto; height: 34px; padding: 0 18px; border-radius: 17px; color: #594db2; background: #fff; font-size: 11px; }
.manage-bar { position: fixed; z-index: 80; right: 14px; bottom: calc(12px + env(safe-area-inset-bottom)); left: 14px; display: flex; height: 62px; box-sizing: border-box; padding: 10px 12px; align-items: center; gap: 8px; border: 1px solid #e3e5f0; border-radius: 18px; background: #fff; box-shadow: 0 10px 28px rgba(15, 23, 42, .12); font-size: 11px; }
.manage-bar > text { min-width: 0; flex: 1; }
.manage-bar button { width: auto; height: 34px; padding: 0 12px; border-radius: 17px; font-size: 11px; }
.clear-button { color: #6e758c; background: #f7f8fc; }
.delete-button { color: #fff; background: #ef4444; }
.delete-button[disabled] { opacity: .45; }
.bottom-safe { height: 12px; }
</style>
