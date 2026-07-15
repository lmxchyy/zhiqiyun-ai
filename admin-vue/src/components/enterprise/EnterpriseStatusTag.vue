<template>
  <el-tag :type="appearance.type" :effect="appearance.effect" round>{{ appearance.label }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{ value?: string; kind?: "enterprise" | "certification" | "plan" }>();

const appearances: Record<string, { label: string; type: "success" | "warning" | "danger" | "info" | "primary"; effect: "light" | "plain" }> = {
  ACTIVE: { label: "正常", type: "success", effect: "light" },
  DISABLED: { label: "已禁用", type: "danger", effect: "light" },
  SUSPENDED: { label: "已暂停", type: "danger", effect: "light" },
  VERIFIED: { label: "已认证", type: "success", effect: "light" },
  APPROVED: { label: "已认证", type: "success", effect: "light" },
  PENDING: { label: "待审核", type: "warning", effect: "light" },
  UNVERIFIED: { label: "未认证", type: "info", effect: "plain" },
  REJECTED: { label: "已驳回", type: "danger", effect: "light" },
  TRIALING: { label: "试用中", type: "primary", effect: "light" },
  EXPIRED: { label: "已到期", type: "danger", effect: "light" }
};

const appearance = computed(() => {
  const key = String(props.value || "UNKNOWN").toUpperCase();
  return appearances[key] || { label: props.value || "未知", type: "info" as const, effect: "plain" as const };
});
</script>
