<template>
  <view :class="['promotion-state', `is-${tone}`]">
    <view v-if="tone === 'loading'" class="promotion-state-loader" />
    <text v-else class="promotion-state-icon">{{ tone === 'error' ? '!' : '↗' }}</text>
    <text class="promotion-state-title">{{ title }}</text>
    <text v-if="description" class="promotion-state-copy">{{ description }}</text>
    <button v-if="actionText" class="promotion-secondary-button compact" @click="$emit('action')"><text>{{ actionText }}</text></button>
  </view>
</template>
<script setup lang="ts">
withDefaults(defineProps<{ tone: "loading" | "empty" | "error"; title: string; description?: string; actionText?: string }>(), { description: "", actionText: "" });
defineEmits<{ action: [] }>();
</script>
