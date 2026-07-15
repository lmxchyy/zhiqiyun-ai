<template>
  <view :class="['asset-card', { selected, selecting }]" role="button" :aria-label="asset.name" :data-asset-id="asset.id" @click="handleOpen">
    <view class="asset-card-cover">
      <AssetCover :asset="asset" />
      <view v-if="selecting" class="select-indicator"><image v-if="selected" src="/static/icons/check.svg" mode="aspectFit" /></view>
      <button v-else class="favorite-button" :data-asset-id="asset.id" :aria-label="asset.favorite ? '取消收藏' : '收藏'" @click.stop="$emit('favorite', asset)"><image :src="asset.favorite ? '/static/icons/favorite-filled.svg' : '/static/icons/favorite-outline.svg'" mode="aspectFit" /></button>
    </view>
    <view class="asset-card-body">
      <view class="asset-card-head">
        <text class="asset-card-name">{{ asset.name }}</text>
        <button v-if="!selecting" class="more-button" :data-asset-id="asset.id" aria-label="更多操作" @click.stop="$emit('action', asset)"><image src="/static/icons/more-vertical.svg" mode="aspectFit" /></button>
      </view>
      <view class="asset-card-meta"><text>{{ typeLabel }} · {{ fileSizeLabel }}</text></view>
      <view class="asset-card-foot"><text>{{ timeLabel }}</text><text class="project-name">{{ asset.projectName || "未归属项目" }}</text></view>
    </view>
  </view>
</template>
<script setup lang="ts">
import { computed } from "vue";
import type { AssetItem } from "../../features/assets/types";
import AssetCover from "./AssetCover.vue";
const props = withDefaults(defineProps<{ asset: AssetItem; selecting?: boolean; selected?: boolean }>(), { selecting: false, selected: false });
const emit = defineEmits<{ open:[asset:AssetItem]; action:[asset:AssetItem]; favorite:[asset:AssetItem]; select:[asset:AssetItem] }>();
const typeLabel = computed(() => ({ image:"图片",video:"视频",ppt:"PPT",document:"文档",agent:"Agent",infographic:"信息图",knowledge:"知识库",prompt:"Prompt",template:"模板" } as Record<string,string>)[props.asset.type] || "作品");
const fileSizeLabel = computed(() => { const bytes = Number(props.asset.fileSize || 0); if (bytes <= 0) return "--"; if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)}GB`; if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(bytes >= 10 * 1024 ** 2 ? 0 : 1)}MB`; if (bytes >= 1024) return `${Math.round(bytes / 1024)}KB`; return `${bytes}B`; });
const timeLabel = computed(() => { const date = new Date(props.asset.createdAt); if (Number.isNaN(date.getTime())) return "刚刚"; const now = new Date(); const dayStart = new Date(now.getFullYear(),now.getMonth(),now.getDate()).getTime(); const assetDay = new Date(date.getFullYear(),date.getMonth(),date.getDate()).getTime(); const clock = `${String(date.getHours()).padStart(2,"0")}:${String(date.getMinutes()).padStart(2,"0")}`; if (assetDay === dayStart) return `今天 ${clock}`; if (assetDay === dayStart - 86400000) return `昨天 ${clock}`; return `${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")} ${clock}`; });
function handleOpen(){ if(props.selecting) emit("select",props.asset); else emit("open",props.asset); }
</script>
<style scoped>
.asset-card{position:relative;min-width:0;overflow:hidden;box-sizing:border-box;border:1px solid #e9ebf2;border-radius:12px;background:#fff;box-shadow:0 4px 13px rgba(32,39,64,.045);text-align:left}.asset-card.selected{border-color:#7d8df6;box-shadow:0 0 0 2px rgba(125,141,246,.14)}.asset-card-cover{position:relative}.asset-card-body{box-sizing:border-box;padding:9px 11px 10px}.asset-card-head{display:flex;min-width:0;height:25px;align-items:center;gap:5px}.asset-card-name{min-width:0;flex:1;overflow:hidden;color:#171b29;font-size:13px;font-weight:700;line-height:20px;text-overflow:ellipsis;white-space:nowrap}.more-button,.favorite-button{display:flex;margin:0;padding:0;align-items:center;justify-content:center;border:0}.more-button::after,.favorite-button::after{display:none}.more-button{width:25px;height:25px;flex:0 0 25px;background:transparent}.more-button image{width:16px;height:16px;opacity:.72}.favorite-button{position:absolute;z-index:3;top:8px;right:8px;width:27px;height:27px;border-radius:8px;background:rgba(27,32,48,.2);box-shadow:none}.favorite-button image{width:17px;height:17px}.asset-card-meta,.asset-card-foot{display:flex;align-items:center;justify-content:space-between;gap:6px}.asset-card-meta{margin-top:2px;color:#858b9b;font-size:10px;line-height:17px}.asset-card-foot{margin-top:5px;color:#9ba1b0;font-size:9px;line-height:16px}.asset-card-foot>text:first-child{flex:0 0 auto;white-space:nowrap}.project-name{min-width:0;overflow:hidden;color:#4f9b72;text-overflow:ellipsis;white-space:nowrap}.select-indicator{position:absolute;z-index:4;top:8px;right:8px;display:flex;width:27px;height:27px;box-sizing:border-box;align-items:center;justify-content:center;border:2px solid #fff;border-radius:50%;background:rgba(255,255,255,.72);box-shadow:0 3px 8px rgba(32,39,64,.14)}.select-indicator image{width:15px;height:15px}.asset-card.selected .select-indicator{background:#4a6cff}
</style>
