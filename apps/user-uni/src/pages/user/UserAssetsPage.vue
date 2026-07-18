<template><MiniProgramRoleWorkbench initial-role="user" initial-tab="assets" /></template>
<script setup lang="ts">
import { onHide, onPullDownRefresh, onShow, onUnload } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";
import { authStorage } from "../../api/client";
import { useAssetStore } from "../../stores/assets";
import { syncCustomTabBar } from "../../utils/customTabBar";

const assetStore = useAssetStore();

function refreshAssets(silent = false) {
  return assetStore.refreshAssets(4, { silent });
}

onShow(() => {
  syncCustomTabBar(2);
  if (!authStorage.getToken()) {
    assetStore.stopTaskPolling();
    return;
  }
  void refreshAssets(assetStore.assets.length > 0);
  assetStore.startTaskPolling();
});
onHide(() => assetStore.stopTaskPolling());
onUnload(() => assetStore.stopTaskPolling());
onPullDownRefresh(() => {
  if (!authStorage.getToken()) {
    uni.stopPullDownRefresh();
    return;
  }
  void refreshAssets(false).finally(() => uni.stopPullDownRefresh());
});
</script>
<style>page { background: #f7f8fc; }</style>
