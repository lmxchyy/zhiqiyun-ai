<template>
  <section v-if="store.analysis" class="sv-analysis" aria-live="polite">
    <strong>分析状态：{{ store.analysis.status }}</strong>
    <span>
      就绪 {{ store.analysis.readyCount || 0 }} /
      失败 {{ store.analysis.failedCount || 0 }} /
      总计 {{ store.analysis.totalCount || store.assets.length }}
    </span>
  </section>
</template>

<script setup lang="ts">
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();
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
