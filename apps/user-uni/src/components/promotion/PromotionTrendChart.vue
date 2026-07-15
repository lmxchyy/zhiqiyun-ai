<template>
  <view class="promotion-trend-chart">
    <view v-for="item in items" :key="item.date" class="promotion-trend-column">
      <view class="promotion-trend-bars">
        <view class="promotion-trend-bar visit" :style="{ height: `${barHeight(item.visitCount)}px` }" />
        <view class="promotion-trend-bar register" :style="{ height: `${barHeight(item.registerCount)}px` }" />
        <view class="promotion-trend-bar paid" :style="{ height: `${barHeight(item.paidCount)}px` }" />
      </view>
      <text>{{ item.date.slice(5) }}</text>
    </view>
  </view>
</template>
<script setup lang="ts">
import { computed } from "vue";
import type { PromotionTrendItem } from "../../features/promotion/types";
const props = defineProps<{ items: PromotionTrendItem[] }>();
const maxValue = computed(() => Math.max(1, ...props.items.flatMap(item => [item.visitCount, item.registerCount, item.paidCount])));
function barHeight(value: number) { return Math.max(value ? 6 : 2, Math.round(value / maxValue.value * 92)); }
</script>
