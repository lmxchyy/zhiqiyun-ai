<template>
  <button :class="['promotion-template-thumb', { selected, compact }]" :style="{ background: template.background, borderColor: selected ? template.primaryColor : 'rgba(90,77,178,.12)' }" @click="$emit('select', template.id)">
    <view class="promotion-thumb-glow" :style="{ background: template.primaryColor }" />
    <text class="promotion-thumb-badge" :style="{ color: template.primaryColor }">{{ template.badge }}</text>
    <text class="promotion-thumb-title" :style="{ color: template.secondaryColor }">{{ template.name }}</text>
    <text class="promotion-thumb-copy">{{ template.description }}</text>
    <view class="promotion-thumb-qr"><view v-for="n in 9" :key="n" :style="{ background: n % 3 ? template.secondaryColor : '#fff' }" /></view>
    <text v-if="selected" class="promotion-thumb-check" :style="{ background: template.primaryColor }">✓</text>
  </button>
</template>
<script setup lang="ts">
import type { PromotionTemplate, PromotionTemplateId } from "../../features/promotion/types";
withDefaults(defineProps<{ template: PromotionTemplate; selected?: boolean; compact?: boolean }>(), { selected: false, compact: false });
defineEmits<{ select: [id: PromotionTemplateId] }>();
</script>
