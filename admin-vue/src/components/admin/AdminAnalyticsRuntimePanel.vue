<template>
  <el-card shadow="never" class="runtime-panel">
    <template #header>
      <div class="runtime-panel__head">
        <div>
          <strong>任务运行态透视</strong>
          <small>数据库排队/处理中任务与工单统计（仅平台管理员可见）</small>
        </div>
        <el-tag :type="healthStatusType" size="small">{{ healthStatusText }}</el-tag>
      </div>
    </template>

    <div class="runtime-grid">
      <div class="runtime-item">
        <label>排队/处理中任务 (QUEUED/PROCESSING)</label>
        <div class="item-val" :class="{ 'is-warn': processingTasks > 20 }">
          {{ processingTasks }}
          <small>个活跃生成任务</small>
        </div>
      </div>
      <div class="runtime-item">
        <label>异常处置工单</label>
        <div class="item-val" :class="{ 'is-danger': exceptionCount > 0 }">
          {{ exceptionCount }}
          <small>待处理 / 处理中工单</small>
        </div>
      </div>
      <div class="runtime-item">
        <label>今日失败任务数</label>
        <div class="item-val" :class="{ 'is-warn': failedTasks > 10 }">
          {{ failedTasks }}
          <small>次上游/系统失败</small>
        </div>
      </div>
      <div class="runtime-item">
        <label>今日平均延迟 (AVG)</label>
        <div class="item-val">
          {{ avgLatencyMs }}
          <small>ms (端到端响应)</small>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  processingTasks: number;
  exceptionCount: number;
  failedTasks: number;
  avgLatencyMs: number;
}>();

const healthStatusType = computed(() => {
  if (props.exceptionCount > 0 || props.failedTasks > 50) return "danger";
  if (props.processingTasks > 50 || props.failedTasks > 10) return "warning";
  return "success";
});

const healthStatusText = computed(() => {
  if (props.exceptionCount > 0) return "存在未解除工单";
  if (props.failedTasks > 50) return "失败任务偏高";
  return "运行正常";
});
</script>

<style scoped>
.runtime-panel {
  border-radius: 8px;
  background: #ffffff;
  border: 1px solid var(--el-border-color-lighter, #ebeef5);
  margin-top: 20px;
}
.runtime-panel__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.runtime-panel__head strong {
  font-size: 15px;
  color: var(--el-text-color-primary, #303133);
  margin-right: 8px;
}
.runtime-panel__head small {
  color: var(--el-text-color-secondary, #909399);
  font-size: 12px;
}
.runtime-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  padding: 6px 0;
}
.runtime-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  background: var(--el-fill-color-light, #f5f7fa);
  border-radius: 6px;
}
.runtime-item label {
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
}
.item-val {
  font-size: 20px;
  font-weight: 700;
  color: var(--el-text-color-primary, #303133);
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.item-val small {
  font-size: 11px;
  font-weight: normal;
  color: var(--el-text-color-secondary, #909399);
}
.item-val.is-warn {
  color: #e6a23c;
}
.item-val.is-danger {
  color: #f56c6c;
}
</style>
