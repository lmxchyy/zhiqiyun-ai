<template>
  <view
    :class="['app-image', { circle, 'is-error': exhausted }]"
    :style="rootStyle"
  >
    <view v-if="loading" class="app-image-skeleton"></view>
    <image
      v-if="currentSrc"
      class="app-image-media"
      :src="currentSrc"
      :mode="imageMode"
      :lazy-load="lazyLoad"
      :aria-label="alt"
      @load="handleLoad"
      @error="handleError"
    />
    <view v-else class="app-image-error" aria-label="图片加载失败"></view>
  </view>
</template>
<script setup lang="ts">
import { computed, ref, watch } from "vue";
const props = withDefaults(
  defineProps<{
    src?: string;
    fallback?: string;
    alt?: string;
    mode?:
      | "cover"
      | "contain"
      | "scaleToFill"
      | "aspectFit"
      | "aspectFill"
      | "widthFix"
      | "heightFix";
    lazyLoad?: boolean;
    width?: string | number;
    height?: string | number;
    radius?: string | number;
    localFallback?: string;
    circle?: boolean;
  }>(),
  {
    src: "",
    fallback: "",
    alt: "",
    mode: "aspectFill",
    lazyLoad: true,
    width: "100%",
    height: "100%",
    radius: 0,
    localFallback: "/static/fallbacks/default-cover.jpg",
    circle: false,
  },
);
const loading = ref(true);
const stage = ref(0);
const sourceChain = computed(() =>
  Array.from(
    new Set(
      [props.src, props.fallback, props.localFallback]
        .map((value) => String(value || "").trim())
        .filter((value) => value && isSupportedImageSource(value)),
    ),
  ),
);
const currentSrc = computed(() => sourceChain.value[stage.value] || "");
const exhausted = computed(() => !currentSrc.value);
const isFallbackSource = computed(() => {
  const source = currentSrc.value;
  return Boolean(source && (source === props.fallback || source === props.localFallback));
});
const imageMode = computed(() => {
  const requested =
    props.mode === "cover"
      ? "aspectFill"
      : props.mode === "contain"
        ? "aspectFit"
        : props.mode;
  return isFallbackSource.value && requested === "aspectFit" ? "aspectFill" : requested;
});
const cssValue = (value: string | number) =>
  typeof value === "number" ? `${value}px` : value;
const rootStyle = computed(() => ({
  width: cssValue(props.width),
  height: cssValue(props.height),
  borderRadius: props.circle ? "50%" : cssValue(props.radius),
}));
watch(
  () => [props.src, props.fallback, props.localFallback],
  () => {
    stage.value = 0;
    loading.value = true;
  },
);
function handleLoad() {
  loading.value = false;
}
function handleError() {
  if (stage.value < sourceChain.value.length - 1) {
    stage.value++;
    loading.value = true;
  } else {
    stage.value = sourceChain.value.length;
    loading.value = false;
  }
}
function isSupportedImageSource(value: string) {
  const globalWithWx = globalThis as typeof globalThis & { wx?: unknown };
  const isMiniProgram = typeof globalWithWx.wx !== "undefined";
  if (!isMiniProgram) return true;
  const normalized = value.toLowerCase();
  if (normalized.startsWith("data:image/svg")) return false;
  if (normalized.startsWith("http://127.0.0.1") || normalized.startsWith("http://localhost")) return false;
  return true;
}
</script>
<style scoped>
.app-image {
  position: relative;
  display: block;
  overflow: hidden;
  background: linear-gradient(135deg, #eef2ff, #f6efff);
}
.app-image.circle {
  border-radius: 50%;
}
.app-image-media,
.app-image-skeleton,
.app-image-error {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}
.app-image-skeleton {
  z-index: 1;
  background: linear-gradient(
    90deg,
    rgba(238, 242, 255, 0.8) 25%,
    rgba(255, 255, 255, 0.95) 50%,
    rgba(238, 242, 255, 0.8) 75%
  );
  background-size: 200% 100%;
  animation: app-image-loading 1.2s infinite;
}
.app-image-media {
  display: block;
  z-index: 2;
}
.app-image-error {
  z-index: 3;
  background:
    radial-gradient(
      circle at 70% 25%,
      rgba(123, 92, 255, 0.22),
      transparent 38%
    ),
    linear-gradient(135deg, #eef2ff, #f4eeff);
}
.app-image.is-error::after {
  position: absolute;
  inset: 25%;
  border: 2px solid rgba(74, 108, 255, 0.28);
  border-radius: 10px;
  content: "";
}
@keyframes app-image-loading {
  to {
    background-position: -200% 0;
  }
}
</style>
