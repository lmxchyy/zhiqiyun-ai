<template>
  <article class="ppt-empty-state">
    <el-icon><FolderOpened /></el-icon>
    <strong>{{ title }}</strong>
    <span>{{ description }}</span>
  </article>
</template>

<script setup lang="ts">
import { FolderOpened } from "@element-plus/icons-vue";

defineProps<{
  title?: string;
  description?: string;
}>();
</script>

<style scoped>
.ppt-empty-state {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  min-height: 220px;
  color: #8d8d93;
  text-align: center;
}

.ppt-empty-state .el-icon {
  color: #666;
  font-size: 46px;
}

.ppt-empty-state strong {
  color: #f5f5f5;
  font-size: 18px;
}
</style>
