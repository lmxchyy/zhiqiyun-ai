<template>
  <BillingCenterV1 v-if="billingV1ModuleIds.includes(moduleId)" :module-id="moduleId" />
  <CommercialBillingCenter v-else :module-id="moduleId" />
</template>

<script setup lang="ts">
import { billingV1ModuleIds } from "../../config/adminNavigation";
import BillingCenterV1 from "./BillingCenterV1.vue";
import CommercialBillingCenter from "./CommercialBillingCenter.vue";

defineProps<{ moduleId: string }>();
</script>
