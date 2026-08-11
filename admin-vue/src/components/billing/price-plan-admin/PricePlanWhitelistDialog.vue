<template>
  <el-dialog
    :model-value="modelValue"
    title="TEST 价格白名单"
    width="min(1180px, 96vw)"
    destroy-on-close
    :close-on-click-modal="false"
    @close="close"
  >
    <PricePlanWhitelistManager
      v-if="modelValue"
      :plan="plan"
      :fixed-price-plan-id="pricePlanId"
      :current-role="currentRole"
      :current-permissions="currentPermissions"
      @view-audit="forwardAudit"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import type { BusinessPlan, PricingAuditFilters } from "../../../types/pricePlanAdmin.ts";
import PricePlanWhitelistManager from "./PricePlanWhitelistManager.vue";

defineProps<{
  modelValue: boolean;
  plan: BusinessPlan;
  pricePlanId: string;
  currentRole: string;
  currentPermissions: string[];
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: boolean): void;
  (event: "view-audit", filters: PricingAuditFilters): void;
}>();

function forwardAudit(filters: PricingAuditFilters) {
  emit("view-audit", filters);
}

function close() {
  emit("update:modelValue", false);
}
</script>
