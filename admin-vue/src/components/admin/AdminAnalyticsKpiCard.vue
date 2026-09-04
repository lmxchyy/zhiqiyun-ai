<template>
  <el-card shadow="never" class="kpi-card" :class="{ 'is-alert': alert && Number(value) > 0 }">
    <div class="kpi-card__header">
      <span class="kpi-card__title">{{ title }}</span>
      <el-tag v-if="tag" size="small" :type="tagType || 'info'" effect="plain">{{ tag }}</el-tag>
    </div>
    <div class="kpi-card__body">
      <div class="kpi-card__value">
        <span class="value-number">{{ formattedValue }}</span>
        <span v-if="unit" class="value-unit">{{ unit }}</span>
      </div>
      <div class="kpi-card__footer" v-if="comparisonLabel || subValue !== undefined">
        <span class="footer-label">{{ comparisonLabel || '基准' }}: {{ formattedSubValue }}</span>
        <span v-if="trendPercent !== undefined" class="footer-trend" :class="trendClass">
          {{ trendPrefix }}{{ Math.abs(trendPercent).toFixed(1) }}%
        </span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  title: string;
  value: number | string;
  unit?: string;
  subValue?: number | string;
  comparisonLabel?: string;
  trendPercent?: number;
  tag?: string;
  tagType?: "" | "success" | "warning" | "info" | "danger";
  alert?: boolean;
  precision?: number;
}>();

const formattedValue = computed(() => {
  if (typeof props.value === "number") {
    return props.precision !== undefined ? props.value.toFixed(props.precision) : props.value.toLocaleString();
  }
  return props.value || "0";
});

const formattedSubValue = computed(() => {
  if (props.subValue === undefined) return "-";
  if (typeof props.subValue === "number") {
    return props.precision !== undefined ? props.subValue.toFixed(props.precision) : props.subValue.toLocaleString();
  }
  return props.subValue;
});

const trendClass = computed(() => {
  if (props.trendPercent === undefined || props.trendPercent === 0) return "trend-neutral";
  return props.trendPercent > 0 ? "trend-up" : "trend-down";
});

const trendPrefix = computed(() => {
  if (props.trendPercent === undefined || props.trendPercent === 0) return "";
  return props.trendPercent > 0 ? "↑ " : "↓ ";
});
</script>

<style scoped>
.kpi-card {
  border-radius: 8px;
  background: #ffffff;
  border: 1px solid var(--el-border-color-lighter, #ebeef5);
  transition: box-shadow 0.2s ease, transform 0.2s ease;
}
.kpi-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}
.kpi-card.is-alert {
  border-color: var(--el-color-danger-light-5, #fde2e2);
  background: #fef0f0;
}
.kpi-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.kpi-card__title {
  font-size: 13px;
  color: var(--el-text-color-secondary, #909399);
  font-weight: 500;
}
.kpi-card__value {
  display: flex;
  align-items: baseline;
  gap: 4px;
  margin: 4px 0 10px;
}
.value-number {
  font-size: 24px;
  font-weight: 700;
  color: var(--el-text-color-primary, #303133);
  line-height: 1.1;
}
.value-unit {
  font-size: 13px;
  color: var(--el-text-color-secondary, #909399);
}
.kpi-card__footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  border-top: 1px dashed var(--el-border-color-extra-light, #f2f6fc);
  padding-top: 8px;
  color: var(--el-text-color-secondary, #909399);
}
.trend-up {
  color: #67c23a;
  font-weight: 600;
}
.trend-down {
  color: #f56c6c;
  font-weight: 600;
}
.trend-neutral {
  color: var(--el-text-color-secondary, #909399);
}
</style>
