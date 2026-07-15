<template>
  <view v-if="asset" class="action-mask" @click="$emit('close')">
    <view class="action-panel" @click.stop>
      <view class="action-handle" />
      <view class="action-head">
        <AssetCover class="action-cover" :asset="asset" />
        <view>
          <text class="action-title">{{ asset.name }}</text>
          <AssetStatusBadge :status="asset.status" />
        </view>
      </view>
      <view class="action-grid">
        <button
          v-for="item in actions"
          :key="item.id"
          :class="['action-item', { danger: item.danger }]"
          @click="choose(item.id)"
        >
          <text class="action-icon">{{ item.icon }}</text>
          <text>{{ item.label }}</text>
        </button>
      </view>
      <button class="action-cancel" @click="$emit('close')">取消</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AssetItem } from "../../features/assets/types";
import AssetCover from "./AssetCover.vue";
import AssetStatusBadge from "./AssetStatusBadge.vue";

type ActionItem = { id: string; label: string; icon: string; danger?: boolean };

const props = withDefaults(defineProps<{ asset: AssetItem | null; mode?: "all" | "manage" }>(), {
  mode: "all",
});
const emit = defineEmits<{ close: []; action: [action: string, asset: AssetItem] }>();

const actions = computed<ActionItem[]>(() => {
  if (!props.asset) return [];
  if (props.mode === "manage") {
    return [
      { id: "favorite", label: props.asset.favorite ? "取消收藏" : "收藏", icon: "★" },
      { id: "move", label: "移动项目", icon: "移" },
      { id: "rename", label: "重命名", icon: "名" },
      { id: "archive", label: "归档", icon: "档" },
      { id: "delete", label: "删除", icon: "删", danger: true },
    ];
  }
  if (props.asset.status === "recycled") {
    return [
      { id: "restore", label: "恢复", icon: "↶" },
      { id: "permanent", label: "彻底删除", icon: "删", danger: true },
    ];
  }
  if (props.asset.status === "generating" || props.asset.status === "queued") {
    return [
      { id: "detail", label: "查看进度", icon: "进" },
      { id: "cancel", label: "取消任务", icon: "停", danger: true },
    ];
  }
  if (props.asset.status === "failed") {
    return [
      { id: "detail", label: "失败原因", icon: "!" },
      { id: "retry", label: "再次生成", icon: "重" },
      { id: "delete", label: "删除", icon: "删", danger: true },
    ];
  }
  return [
    { id: "detail", label: "查看详情", icon: "详" },
    { id: "edit", label: "继续编辑", icon: "编" },
    { id: "retry", label: "再次生成", icon: "重" },
    { id: "download", label: "下载", icon: "下" },
    { id: "share", label: "分享", icon: "享" },
    { id: "favorite", label: props.asset.favorite ? "取消收藏" : "收藏", icon: "★" },
    { id: "move", label: "移动项目", icon: "移" },
    { id: "rename", label: "重命名", icon: "名" },
    { id: "archive", label: "归档", icon: "档" },
    { id: "delete", label: "删除", icon: "删", danger: true },
  ];
});

function choose(action: string) {
  if (!props.asset) return;
  emit("action", action, props.asset);
  emit("close");
}
</script>

<style scoped>
.action-mask { position: fixed; z-index: 190; inset: 0; display: flex; align-items: flex-end; background: rgba(23,27,39,.38); }
.action-panel { width: 100%; box-sizing: border-box; padding: 10px 18px calc(16px + env(safe-area-inset-bottom)); border-radius: 24px 24px 0 0; background: #fff; }
.action-handle { width: 42px; height: 4px; margin: 0 auto 12px; border-radius: 4px; background: #d5d7de; }
.action-head { display: flex; align-items: center; gap: 12px; }
.action-cover { width: 58px!important; height: 58px!important; flex: 0 0 58px; }
.action-head > view:last-child { display: flex; min-width: 0; flex-direction: column; align-items: flex-start; gap: 6px; }
.action-title { max-width: 250px; overflow: hidden; color: #252a39; font-size: 15px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.action-grid { display: grid; margin-top: 16px; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 9px; }
.action-item { display: flex; height: 66px; margin: 0; padding: 7px 3px; flex-direction: column; align-items: center; justify-content: center; gap: 5px; border: 0; border-radius: 14px; color: #555d70; background: #f7f8fb; font-size: 10px; }
.action-item::after,.action-cancel::after { display: none; }
.action-icon { display: flex; width: 28px; height: 28px; align-items: center; justify-content: center; border-radius: 9px; color: #5a4db2; background: #eae9fc; font-size: 11px; font-weight: 700; }
.action-item.danger { color: #cc592e; background: #fff5ef; }
.action-item.danger .action-icon { color: #cc592e; background: #ffe8dc; }
.action-cancel { width: 100%; height: 44px; margin: 14px 0 0; border: 0; border-radius: 14px; color: #656c7d; background: #f0f1f4; font-size: 12px; }
</style>
