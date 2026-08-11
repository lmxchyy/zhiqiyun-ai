<template>
  <section v-if="store.analysis" class="sv-analysis" aria-live="polite">
    <strong>分析状态：{{ statusLabel }}</strong>
    <span>
      就绪 {{ succeededCount }} /
      失败 {{ failedCount }} /
      总计 {{ totalCount }}
    </span>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();

const succeededCount = computed(() => Number(store.analysis?.succeededCount || store.analysis?.readyCount || 0));
const failedCount = computed(() => Number(store.analysis?.failedCount || 0));
const totalCount = computed(() => Number(store.analysis?.totalAssets || store.analysis?.totalCount || store.assets.length || 0));

const statusLabel = computed(() => {
  const raw = String(store.analysis?.overallStatus || store.analysis?.status || "").toUpperCase();
  switch (raw) {
    case "SUCCEEDED":
    case "READY":
    case "COMPLETED":
    case "MATERIAL_READY":
      return "已完成";
    case "RUNNING":
    case "ANALYZING":
      return "分析中";
    case "FAILED":
    case "PARTIAL_FAILED":
      return "失败";
    case "QUEUED":
    case "PENDING":
      return "排队中";
    default:
      return raw || "未知";
  }
});
</script>

<style scoped>
.sv-analysis {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(66, 52, 153, 0.16);
  border: 1px solid rgba(66, 52, 153, 0.35);
  color: #d7dbff;
  font-size: 13px;
}
</style>
