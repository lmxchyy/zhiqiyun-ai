<template>
  <scroll-view class="auth-safe-scroll" scroll-y :scroll-into-view="scrollIntoView" enhanced>
    <view class="auth-safe-page" :style="pageStyle">
      <view class="auth-glow auth-glow-left" />
      <view class="auth-glow auth-glow-right" />
      <slot />
    </view>
  </scroll-view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";

const props = withDefaults(defineProps<{ keyboardHeight?: number; scrollIntoView?: string }>(), {
  keyboardHeight: 0,
  scrollIntoView: "",
});
const { navigationStyle } = useMiniProgramNavigation();

const pageStyle = computed(() => ({
  ...navigationStyle.value,
  paddingBottom: `${Math.max(28, props.keyboardHeight + 28)}px`,
}));
</script>

<style scoped>
.auth-safe-scroll { width: 100%; height: 100vh; background: #f7f9fd; }
.auth-safe-page {
  position: relative;
  min-height: 100vh;
  box-sizing: border-box;
  padding: var(--header-height, max(44px, env(safe-area-inset-top, 0px))) 20px 28px;
  overflow: hidden;
  color: #181c28;
  background: #f7f9fd;
  transition: padding-bottom 180ms ease;
}
.auth-glow { position: absolute; z-index: 0; border-radius: 50%; filter: blur(30px); pointer-events: none; }
.auth-glow-left { width: 210px; height: 170px; left: -70px; top: -40px; background: rgba(92, 132, 255, 0.22); }
.auth-glow-right { width: 170px; height: 160px; right: -55px; top: 70px; background: rgba(146, 102, 255, 0.17); }
@media (max-width: 340px) {
  .auth-safe-page { padding-left: 16px; padding-right: 16px; padding-top: var(--header-height, max(44px, env(safe-area-inset-top, 0px))); }
}
</style>
