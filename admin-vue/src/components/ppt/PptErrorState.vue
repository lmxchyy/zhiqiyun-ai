<template>
  <article class="ppt-error-state">
    <el-icon><WarningFilled /></el-icon>
    <div>
      <strong>{{ title || "操作失败" }}</strong>
      <span>{{ message || "请稍后重试" }}</span>
    </div>
    <button type="button" title="重试当前操作" aria-label="重试当前操作" @click="$emit('retry')">重试</button>
  </article>
</template>

<script setup lang="ts">
import { WarningFilled } from "@element-plus/icons-vue";

defineProps<{
  title?: string;
  message?: string;
}>();

defineEmits<{
  retry: [];
}>();
</script>

<style scoped>
.ppt-error-state {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid rgba(248, 113, 113, 0.28);
  border-radius: 10px;
  color: #fecaca;
  background: rgba(127, 29, 29, 0.22);
}

.ppt-error-state div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.ppt-error-state strong {
  color: #fff1f2;
}

.ppt-error-state span {
  color: #fecaca;
  font-size: 13px;
}

.ppt-error-state button {
  margin-left: auto;
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid rgba(248, 113, 113, 0.35);
  border-radius: 8px;
  color: #fff;
  background: rgba(185, 28, 28, 0.5);
  cursor: pointer;
}
</style>
