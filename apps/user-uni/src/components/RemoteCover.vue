<template>
  <AppImage
    :src="src || slot?.imageUrl"
    :fallback="fallback || slot?.fallbackUrl"
    :alt="slot?.altText || alt"
    :mode="mode"
    :lazy-load="lazyLoad"
    :width="width"
    :height="height"
    :radius="radius"
    :local-fallback="localFallback"
  />
</template>
<script setup lang="ts">
import { computed } from "vue";
import AppImage from "./AppImage.vue";
import { usePageConfigStore, type AppPageCode } from "../stores/pageConfig";
const props = withDefaults(
  defineProps<{
    pageCode: AppPageCode;
    slotKey: string;
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
    localFallback: "",
  },
);
const store = usePageConfigStore();
store.hydrate(props.pageCode);
const slot = computed(() => store.slot(props.pageCode, props.slotKey));
const mode = computed(() =>
  props.mode === "aspectFill" && slot.value?.fit ? slot.value.fit : props.mode,
);
const localFallback = computed(
  () =>
    props.localFallback ||
    slot.value?.fallbackUrl ||
    "/static/fallbacks/default-cover.jpg",
);
</script>
