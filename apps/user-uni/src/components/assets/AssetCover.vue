<template>
  <view :class="['asset-cover-shell', `type-${asset.type}`]">
    <AppImage v-if="hasImage" :src="asset.thumbnailUrl || asset.remoteUrl" :fallback="asset.fallbackUrl" :alt="asset.name" width="100%" height="100%" radius="11px 11px 0 0" :lazy-load="true" @load="$emit('thumbnail-load', asset.id)" />
    <view v-else class="asset-cover-placeholder"><text class="asset-cover-symbol">{{ symbol }}</text><text class="asset-cover-type">{{ label }}</text></view>
    <text class="cover-type-tag">{{ coverTag }}</text>
    <view v-if="asset.type === 'video'" class="play-mark"><text>▶</text></view>
    <text v-if="asset.type === 'video' && durationLabel" class="cover-duration">{{ durationLabel }}</text>
    <view v-if="asset.type === 'knowledge' && asset.documentCount" class="cover-count"><text>{{ asset.documentCount }} 篇</text></view>
  </view>
</template>
<script setup lang="ts">
import { computed } from "vue";
import type { AssetItem } from "../../features/assets/types";
import AppImage from "../AppImage.vue";
const props = defineProps<{ asset: AssetItem }>();
defineEmits<{ "thumbnail-load": [assetID: string] }>();
const hasImage = computed(() => Boolean(props.asset.thumbnailUrl || props.asset.remoteUrl || props.asset.fallbackUrl));
const symbol = computed(() => ({ image: "图", video: "视", ppt: "P", document: "文", agent: "A", infographic: "表", knowledge: "知", prompt: "{ }", template: "模" } as Record<string, string>)[props.asset.type] || "作");
const label = computed(() => ({ image: "AI 图片", video: "AI 视频", ppt: "PPT", document: "文档", agent: "Agent", infographic: "信息图", knowledge: "知识库", prompt: "Prompt", template: "模板" } as Record<string, string>)[props.asset.type] || "数字资产");
const coverTag = computed(() => ({ queued:"排队中",generating:"生成中",failed:"生成失败" } as Record<string,string>)[props.asset.status] || label.value);
const durationLabel = computed(() => { const duration = Math.max(0,Math.round(Number(props.asset.duration || 0))); if (!duration) return ""; return `${String(Math.floor(duration / 60)).padStart(2,"0")}:${String(duration % 60).padStart(2,"0")}`; });
</script>
<style scoped>
.asset-cover-shell{position:relative;width:100%;height:112px;overflow:hidden;border-radius:11px 11px 0 0;background:#f1f3fa}.asset-cover-placeholder{display:flex;width:100%;height:100%;flex-direction:column;align-items:center;justify-content:center;color:#5a4db2;background:#f0effd}.asset-cover-symbol{font-size:28px;font-weight:700}.asset-cover-type{margin-top:6px;font-size:10px}.type-video .asset-cover-placeholder,.type-infographic .asset-cover-placeholder{color:#d76522;background:#fff3ea}.type-agent .asset-cover-placeholder,.type-knowledge .asset-cover-placeholder{color:#177c59;background:#eaf7f1}.cover-type-tag{position:absolute;top:8px;left:8px;max-width:72px;overflow:hidden;box-sizing:border-box;padding:3px 7px;border:1px solid rgba(255,255,255,.35);border-radius:8px;color:#fff;background:linear-gradient(105deg,rgba(74,108,255,.92),rgba(123,92,255,.86));font-size:8px;font-weight:650;line-height:13px;text-overflow:ellipsis;white-space:nowrap}.play-mark{position:absolute;top:50%;left:50%;display:flex;width:36px;height:36px;align-items:center;justify-content:center;transform:translate(-50%,-50%);border:1px solid rgba(255,255,255,.82);border-radius:50%;color:#fff;background:rgba(20,24,36,.48);font-size:12px}.cover-duration,.cover-count{position:absolute;right:8px;bottom:8px;padding:3px 7px;border-radius:8px;color:#fff;background:rgba(20,24,36,.6);font-size:8px;line-height:13px}.cover-count{right:8px}
</style>
