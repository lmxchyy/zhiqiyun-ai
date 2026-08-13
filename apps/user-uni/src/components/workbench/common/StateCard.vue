<template>
  <view :class="['state-card', { error }]">
    <text>{{ message }}</text>
    <button v-if="actionLabel && onAction" type="button" class="small-button" @click="onAction">
      {{ actionLabel }}
    </button>
  </view>
</template>

<script setup lang="ts">
interface StateCardProps {
  message: string;
  error?: boolean;
  actionLabel?: string;
  onAction?: () => void;
}

withDefaults(defineProps<StateCardProps>(), {
  error: false,
  actionLabel: "",
});
</script>

<style scoped>
.state-card {
  margin-top: 14px;
  padding: 18px;
  color: #6b7280;
  border-radius: 16px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
}

.state-card.error {
  color: #dc2626;
  border-color: #fecaca;
  background: #fff7f7;
}

.small-button {
  margin-top: 10px;
  padding: 0 10px;
  height: 30px;
  border-radius: 10px;
  color: #ffffff;
  background: #5a4db2;
  font-size: 12px;
}
</style>
