<template>
  <view :class="['enterprise-state', tone]" role="status">
    <view class="enterprise-state-icon">{{ iconText }}</view>
    <text class="enterprise-state-title">{{ resolvedTitle }}</text>
    <text class="enterprise-state-copy">{{ resolvedCopy }}</text>
    <button v-if="actionLabel" class="enterprise-state-action" type="button" hover-class="enterprise-pressed" @click="$emit('action')">{{ actionLabel }}</button>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";

type StateKind = "loading" | "empty" | "error" | "forbidden" | "disabled" | "reviewing" | "low-balance" | "expired" | "success";

const props = defineProps<{ kind: StateKind; title?: string; copy?: string; actionLabel?: string }>();
defineEmits<{ action: [] }>();

const presets: Record<StateKind, { icon: string; title: string; copy: string; tone: string }> = {
  loading: { icon: "…", title: "正在加载", copy: "正在同步当前企业数据", tone: "blue" },
  empty: { icon: "空", title: "暂无数据", copy: "当前企业还没有相关记录", tone: "blue" },
  error: { icon: "!", title: "加载失败", copy: "网络异常，请稍后重试", tone: "red" },
  forbidden: { icon: "锁", title: "暂无操作权限", copy: "请联系企业管理员分配权限", tone: "purple" },
  disabled: { icon: "停", title: "企业服务已暂停", copy: "请联系平台客服了解原因", tone: "red" },
  reviewing: { icon: "审", title: "认证审核中", copy: "预计 1—2 个工作日完成审核", tone: "blue" },
  "low-balance": { icon: "点", title: "企业算力不足", copy: "当前任务无法继续执行", tone: "orange" },
  expired: { icon: "期", title: "企业套餐已到期", copy: "请联系企业管理员续费", tone: "orange" },
  success: { icon: "✓", title: "操作成功", copy: "企业数据已更新", tone: "green" },
};

const preset = computed(() => presets[props.kind]);
const iconText = computed(() => preset.value.icon);
const tone = computed(() => preset.value.tone);
const resolvedTitle = computed(() => props.title || preset.value.title);
const resolvedCopy = computed(() => props.copy || preset.value.copy);
</script>
