<template>
  <button :class="['auth-secondary-entry', { muted }]" hover-class="auth-secondary-hover" @click="$emit('activate')">
    <text>{{ label }}</text>
  </button>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ label: string; muted?: boolean }>(), { muted: false });
defineEmits<{ activate: [] }>();
</script>

<style scoped>
.auth-secondary-entry { width: 100%; min-height: 34px; margin: 0; padding: 5px 4px; border: 0; color: #4a6bff; background: transparent; font-size: 13px; line-height: 24px; font-weight: 500; }
.auth-secondary-entry::after { display: none; }
.auth-secondary-entry.muted { color: #697085; font-size: 12px; font-weight: 400; }
.auth-secondary-hover { opacity: .68; }
</style>
