<template>
  <div ref="viewport" class="ppt-agent-slide-viewport" :style="viewportStyle">
    <div
      class="ppt-agent-slide-canvas"
      :aria-label="label"
      :style="canvasStyle"
      role="img"
    >
      <template v-for="item in projectedElements" :key="item.element.id">
        <div
          v-if="item.element.type === 'text'"
          class="ppt-agent-preview-element is-text"
          :data-preview-element-id="item.element.id"
          :style="textStyle(item.layout)"
        >
          <ul v-if="item.element.content?.kind === 'bullets'">
            <li v-for="(bullet, index) in item.element.content.items || []" :key="index">{{ bullet }}</li>
          </ul>
          <span v-else>{{ item.element.content?.text }}</span>
        </div>
        <div
          v-else-if="item.element.type === 'shape'"
          class="ppt-agent-preview-element is-shape"
          :data-preview-element-id="item.element.id"
          :data-preview-shape="item.element.id"
          :style="shapeStyle(item.layout)"
          aria-hidden="true"
        ></div>
        <img
          v-else-if="item.element.type === 'image' && assetURLs[item.element.assetRef || '']"
          class="ppt-agent-preview-element is-image"
          :data-preview-element-id="item.element.id"
          :src="assetURLs[item.element.assetRef || '']"
          :alt="item.element.altText || '演示文稿图片'"
          :style="imageStyle(item.layout)"
          @error="emit('asset-error', item.element.assetRef || '')"
        />
        <div
          v-else
          class="ppt-agent-preview-element is-missing-asset"
          :data-preview-element-id="item.element.id"
          :style="authoritativeElementStyle(item.layout)"
          role="alert"
        >图片暂时无法加载</div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import type { PptPreviewLayoutElement, PptPreviewLayoutSlide, PptPreviewSlide } from "../../types/pptAgent";
import { authoritativeElementStyle, previewCanvasTransform } from "./pptAgentPreview";

const props = defineProps<{
  slide: PptPreviewSlide;
  layout: PptPreviewLayoutSlide;
  canvas: { width: number; height: number };
  assetURLs: Record<string, string>;
  label: string;
}>();
const emit = defineEmits<{ "asset-error": [assetId: string] }>();
const viewport = ref<HTMLElement | null>(null);
const viewportWidth = ref(0);
let resizeObserver: ResizeObserver | undefined;

const transform = computed(() => previewCanvasTransform(props.canvas, viewportWidth.value || props.canvas.width));
const viewportStyle = computed(() => ({ width: `${transform.value.width}px`, height: `${transform.value.height}px` }));
const canvasStyle = computed(() => ({
  width: `${props.canvas.width}px`,
  height: `${props.canvas.height}px`,
  background: props.layout.backgroundColor,
  transform: transform.value.transform,
  transformOrigin: "top left"
}));
const projectedElements = computed(() => {
  const byID = new Map(props.slide.elements.map(element => [element.id, element]));
  return props.layout.elements.map(layout => ({ layout, element: byID.get(layout.elementId)! })).filter(item => Boolean(item.element));
});

function verticalAlignment(value?: string) {
  if (value === "middle") return "center";
  if (value === "bottom") return "flex-end";
  return "flex-start";
}

function textStyle(layout: PptPreviewLayoutElement) {
  const style = layout.resolvedStyle;
  return {
    ...authoritativeElementStyle(layout),
    color: style.color,
    fontFamily: style.fontFace,
    fontSize: `${style.fontSizePt || 12}px`,
    fontWeight: style.bold ? "700" : "400",
    fontStyle: style.italic ? "italic" : "normal",
    textAlign: style.align || "left",
    padding: `${style.marginPt || 0}px`,
    alignItems: verticalAlignment(style.verticalAlign)
  };
}

function shapeStyle(layout: PptPreviewLayoutElement) {
  const style = layout.resolvedStyle;
  return {
    ...authoritativeElementStyle(layout),
    background: style.fillColor || "transparent",
    border: `${style.lineWidthPt || 0}px solid ${style.lineColor || "transparent"}`,
    borderRadius: style.shapeType === "ellipse" ? "50%" : style.shapeType === "roundRect" ? "18px" : "0",
    opacity: String(1 - Math.max(0, Math.min(100, style.transparency || 0)) / 100)
  };
}

function imageStyle(layout: PptPreviewLayoutElement) {
  return { ...authoritativeElementStyle(layout), objectFit: layout.resolvedStyle.fit || "cover" };
}

onMounted(() => {
  const measure = () => {
    const width = viewport.value?.parentElement?.clientWidth || viewport.value?.clientWidth || props.canvas.width;
    viewportWidth.value = Math.min(props.canvas.width, width);
  };
  measure();
  if (typeof ResizeObserver !== "undefined") {
    resizeObserver = new ResizeObserver(measure);
    if (viewport.value?.parentElement) resizeObserver.observe(viewport.value.parentElement);
  }
});
onBeforeUnmount(() => resizeObserver?.disconnect());
</script>

<style scoped>
.ppt-agent-slide-viewport { position: relative; max-width: 100%; overflow: hidden; }
.ppt-agent-slide-canvas { position: absolute; inset: 0 auto auto 0; overflow: hidden; box-shadow: 0 18px 48px rgba(23, 32, 51, .16); }
.ppt-agent-preview-element { position: absolute; box-sizing: border-box; overflow: hidden; }
.ppt-agent-preview-element.is-text { display: flex; white-space: pre-wrap; line-height: 1.28; overflow-wrap: break-word; }
.ppt-agent-preview-element.is-text > span { width: 100%; }
.ppt-agent-preview-element.is-text ul { width: 100%; margin: 0; padding-left: 1.25em; }
.ppt-agent-preview-element.is-text li + li { margin-top: .38em; }
.ppt-agent-preview-element.is-image { display: block; }
.ppt-agent-preview-element.is-missing-asset { display: grid; place-items: center; padding: 16px; background: #f3f4f6; color: #9b3b3b; font: 600 14px/1.4 system-ui, sans-serif; }
</style>
