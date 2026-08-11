<template>
  <PromotionCenterScreen ref="screenRef" />
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import PromotionCenterScreen from "../../components/promotion/PromotionCenterScreen.vue";

const screenRef = ref<{ load?: (force: boolean) => Promise<void> | void } | null>(null);
onShow(() => { void screenRef.value?.load?.(false); });
</script>

<style>
page { background: #f7f8fc; }
</style>
