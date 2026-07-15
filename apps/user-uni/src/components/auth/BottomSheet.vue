<template>
  <view v-if="visible" class="auth-sheet-layer" @touchmove.stop.prevent>
    <view class="auth-sheet-overlay" @click="closeOnOverlay && $emit('close')" />
    <view class="auth-bottom-sheet" :style="sheetStyle">
      <view class="auth-sheet-handle" />
      <view v-if="title" class="auth-sheet-heading">
        <text class="auth-sheet-title">{{ title }}</text>
        <button v-if="closable" class="auth-sheet-close" aria-label="关闭" @click="$emit('close')">×</button>
      </view>
      <slot />
      <view class="auth-sheet-safe" />
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
const props = withDefaults(defineProps<{ visible: boolean; title?: string; closable?: boolean; closeOnOverlay?: boolean; keyboardHeight?: number }>(), {
  title: "",
  closable: true,
  closeOnOverlay: true,
  keyboardHeight: 0,
});
defineEmits<{ close: [] }>();
const sheetStyle = computed(() => ({ bottom: `${Math.max(0, props.keyboardHeight)}px` }));
</script>

<style scoped>
.auth-sheet-layer { position: fixed; z-index: 90; inset: 0; }
.auth-sheet-overlay { position: absolute; inset: 0; background: rgba(10, 15, 28, .38); }
.auth-bottom-sheet { position: absolute; left: 0; right: 0; bottom: 0; padding: 36px 24px 18px; box-sizing: border-box; border-radius: 28px 28px 0 0; background: #fff; box-shadow: 0 -8px 30px rgba(20, 31, 61, .12); transition: bottom 160ms ease; }
.auth-sheet-handle { position: absolute; top: 10px; left: 50%; width: 55px; height: 5px; margin-left: -27.5px; border-radius: 3px; background: #d1d6e3; }
.auth-sheet-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 23px; }
.auth-sheet-title { color: #181c28; font-size: 20px; line-height: 30px; font-weight: 700; }
.auth-sheet-close { width: 36px; height: 36px; margin: -3px -8px -3px 0; padding: 0; border: 0; color: #697085; background: transparent; font-size: 24px; line-height: 36px; }
.auth-sheet-close::after { display: none; }
.auth-sheet-safe { height: env(safe-area-inset-bottom); }
</style>
