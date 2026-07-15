<template>
  <view v-if="visible" :class="['auth-toast', tone]"><text>{{ message }}</text></view>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ visible: boolean; message: string; tone?: "info" | "error" | "success" }>(), { tone: "info" });
</script>

<style scoped>
.auth-toast { position: fixed; z-index: 150; left: 50%; bottom: calc(env(safe-area-inset-bottom) + 42px); width: max-content; max-width: 300px; transform: translateX(-50%); padding: 10px 16px; box-sizing: border-box; border-radius: 12px; color: #fff; background: rgba(24,28,40,.92); box-shadow: 0 8px 24px rgba(20,31,61,.2); font-size: 12px; line-height: 20px; text-align: center; }
.auth-toast.error { background: rgba(183, 32, 48, .94); }
.auth-toast.success { background: rgba(14, 126, 82, .94); }
</style>
