<template><text :class="['asset-status-badge', `tone-${tone}`]">{{ label }}</text></template>
<script setup lang="ts">
import { computed } from "vue";
import type { AssetStatus, GenerationTaskStatus } from "../../features/assets/types";
const props = defineProps<{ status: AssetStatus | GenerationTaskStatus }>();
const labels: Record<string, string> = { recent: "最近", queued: "排队中", generating: "生成中", completed: "已完成", failed: "失败", favorite: "收藏", archived: "已归档", recycled: "回收站", cancelled: "已取消" };
const label = computed(() => labels[props.status] || props.status);
const tone = computed(() => ({ queued: "queued", generating: "primary", completed: "success", failed: "danger", favorite: "favorite", archived: "muted", recycled: "recycled", cancelled: "muted", recent: "muted" } as Record<string, string>)[props.status] || "muted");
</script>
<style scoped>
.asset-status-badge{display:inline-flex;min-height:20px;box-sizing:border-box;padding:2px 8px;align-items:center;border-radius:999px;font-size:10px;font-weight:600;line-height:16px;white-space:nowrap}.tone-queued{color:#60708f;background:#edf1f8}.tone-primary{color:#5a4db2;background:#eeedff}.tone-success{color:#16845b;background:#e8f7f0}.tone-danger{color:#d75b2b;background:#fff0e8}.tone-favorite{color:#7746b6;background:#f3eafb}.tone-muted{color:#73798b;background:#f0f1f4}.tone-recycled{color:#c75b63;background:#fff0f1}
</style>
