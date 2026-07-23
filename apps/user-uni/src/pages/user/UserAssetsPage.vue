<template><MiniProgramRoleWorkbench initial-role="user" initial-tab="assets" /></template>
<script setup lang="ts">
import { nextTick } from "vue";
import { onHide, onLoad, onPullDownRefresh, onShow, onUnload } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";
import { authStorage } from "../../api/client";
import { beginWorksPerformanceStep, recordWorksPerformance, worksTabClickStartedAt } from "../../features/assets/performance";
import { useAssetStore } from "../../stores/assets";
import { syncCustomTabBar } from "../../utils/customTabBar";

const assetStore = useAssetStore();
const pageCreatedAt = Date.now();
let lastBackgroundRefreshAt = 0;

function currentRecentWorksScope() {
  const auth = authStorage.getAuth();
  const userID = String(auth?.user?.id || "").trim();
  if (!userID) return "";
  const tenantID = String(auth?.tenantId || auth?.user?.tenantId || "tenant_default").trim();
  return `${userID}:${tenantID}`;
}

const initialScope = currentRecentWorksScope();
if (initialScope && assetStore.isDefaultRecentView) assetStore.hydrateRecentWorksCache(initialScope);
const initialClickStartedAt = worksTabClickStartedAt();
recordWorksPerformance("page_created", initialClickStartedAt || pageCreatedAt, pageCreatedAt, {
  serialWait: false,
  source: "UserAssetsPage.setup",
  cacheHit: assetStore.assets.length > 0,
  itemCount: assetStore.assets.length,
});

function refreshRecentWorks(source: string, force = false) {
  const scope = currentRecentWorksScope();
  if (!scope) return Promise.resolve();
  return assetStore.refreshRecentWorks({ scope, source, force, silent: assetStore.assets.length > 0 });
}

onLoad(() => {
  const timing = beginWorksPerformanceStep("onLoad", {
    serialWait: false,
    source: "UserAssetsPage.onLoad",
  });
  const scope = currentRecentWorksScope();
  if (scope && assetStore.isDefaultRecentView) assetStore.hydrateRecentWorksCache(scope);
  timing.end({ cacheHit: assetStore.assets.length > 0, itemCount: assetStore.assets.length });
});

onShow(() => {
  const showStartedAt = worksTabClickStartedAt() || Date.now();
  const timing = beginWorksPerformanceStep("onShow", {
    serialWait: false,
    source: "UserAssetsPage.onShow",
  });
  syncCustomTabBar(2);
  if (!authStorage.getToken()) {
    assetStore.stopTaskPolling();
    timing.end({ note: "guest" });
    return;
  }
  const scope = currentRecentWorksScope();
  if (scope && assetStore.isDefaultRecentView) assetStore.hydrateRecentWorksCache(scope);
  void nextTick().then(() => {
    const renderedAt = Date.now();
    recordWorksPerformance("first_screen_render", showStartedAt, renderedAt, {
      serialWait: false,
      source: "UserAssetsPage.onShow.nextTick",
      cacheHit: assetStore.assets.length > 0,
      itemCount: assetStore.assets.length,
    });
  });
  if (assetStore.isDefaultRecentView) void refreshRecentWorks("onShow");
  else if (!assetStore.assets.length) void assetStore.refreshAssets(4, { silent: true });
  const now = Date.now();
  if (now - lastBackgroundRefreshAt >= 10000) {
    lastBackgroundRefreshAt = now;
    setTimeout(() => {
      void assetStore.refreshAssetCenterBackground("UserAssetsPage.onShow");
    }, 0);
  } else {
    recordWorksPerformance("asset_center_background_refresh", now, now, {
      serialWait: false,
      source: "UserAssetsPage.onShow",
      duplicate: true,
      note: "dedupe_window",
    });
  }
  assetStore.startTaskPolling(4000);
  timing.end({ cacheHit: assetStore.assets.length > 0, itemCount: assetStore.assets.length });
});
onHide(() => assetStore.stopTaskPolling());
onUnload(() => assetStore.stopTaskPolling());
onPullDownRefresh(() => {
  if (!authStorage.getToken()) {
    uni.stopPullDownRefresh();
    return;
  }
  const request = assetStore.isDefaultRecentView
    ? refreshRecentWorks("pullDownRefresh", true)
    : assetStore.refreshAssets(4, { silent: true });
  void request.finally(() => uni.stopPullDownRefresh());
  void assetStore.refreshAssetCenterBackground("pullDownRefresh");
});
</script>
<style>page { background: #f7f8fc; }</style>
