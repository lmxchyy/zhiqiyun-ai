<template>
  <section class="ppt-slide-preview">
    <aside role="listbox" aria-label="幻灯片预览列表">
      <button
        v-for="(slide, index) in slides"
        :key="slide.id"
        type="button"
        role="option"
        :class="{ active: currentIndex === index }"
        :aria-selected="currentIndex === index"
        :title="`预览第 ${index + 1} 页：${slide.title}`"
        :aria-label="`预览第 ${index + 1} 页：${slide.title}`"
        @click="$emit('select', index)"
      >
        <span>{{ index + 1 }}</span>
        <strong>{{ slide.title }}</strong>
      </button>
    </aside>
    <main v-if="currentSlide" :class="`theme-${theme}`">
      <div class="ppt-slide-canvas">
        <span>{{ currentIndex + 1 }} / {{ slides.length }}</span>
        <h3>{{ currentSlide.title }}</h3>
        <p>{{ currentSlide.content }}</p>
        <ul>
          <li v-for="point in currentSlide.bulletPoints" :key="point">{{ point }}</li>
        </ul>
      </div>
      <footer>
        <button type="button" :disabled="currentIndex <= 0" :title="currentIndex <= 0 ? '已经是第一页' : '查看上一页'" :aria-label="currentIndex <= 0 ? '已经是第一页' : '查看上一页'" @click="$emit('prev')">上一页</button>
        <button type="button" :disabled="currentIndex >= slides.length - 1" :title="currentIndex >= slides.length - 1 ? '已经是最后一页' : '查看下一页'" :aria-label="currentIndex >= slides.length - 1 ? '已经是最后一页' : '查看下一页'" @click="$emit('next')">下一页</button>
        <button type="button" title="全屏预览当前页" aria-label="全屏预览当前页" @click="$emit('fullscreen')">全屏预览</button>
        <button type="button" title="重新生成当前页" aria-label="重新生成当前页" @click="$emit('regenerate')">重新生成当前页</button>
      </footer>
    </main>
    <PptEmptyState v-else title="暂无预览" description="生成 PPT 后将在这里展示幻灯片预览。" />
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { PptSlide, PptTheme } from "../../types/ppt";
import PptEmptyState from "./PptEmptyState.vue";

const props = defineProps<{
  slides: PptSlide[];
  currentIndex: number;
  theme: PptTheme;
}>();

defineEmits<{
  select: [index: number];
  prev: [];
  next: [];
  fullscreen: [];
  regenerate: [];
}>();

const currentSlide = computed(() => props.slides[props.currentIndex] || null);
</script>

<style scoped>
.ppt-slide-preview {
  display: grid;
  grid-template-columns: 190px minmax(0, 1fr);
  gap: 14px;
  min-width: 0;
}

.ppt-slide-preview aside {
  display: grid;
  gap: 8px;
  align-content: start;
  max-height: 540px;
  overflow: auto;
  min-width: 0;
}

.ppt-slide-preview aside button {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 8px;
  min-height: 58px;
  border: 1px solid #282828;
  border-radius: 8px;
  color: #d4d4d8;
  background: #0d0d0d;
  text-align: left;
  cursor: pointer;
}

.ppt-slide-preview aside button.active {
  border-color: #737373;
  background: #1d1d1d;
}

.ppt-slide-preview main {
  display: grid;
  gap: 12px;
  min-width: 0;
}

.ppt-slide-canvas {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  aspect-ratio: 16 / 9;
  padding: clamp(24px, 3vw, 44px);
  border-radius: 10px;
  color: #0f172a;
  background: linear-gradient(135deg, #f8fafc, #dbeafe);
  overflow: hidden;
}

.ppt-slide-canvas span {
  color: #2563eb;
  font-weight: 800;
}

.ppt-slide-canvas h3 {
  margin: 20px 0 12px;
  font-size: clamp(22px, 2.4vw, 34px);
  line-height: 1.2;
  word-break: break-word;
}

.ppt-slide-canvas p {
  max-width: 720px;
  line-height: 1.7;
  word-break: break-word;
}

.ppt-slide-canvas li {
  margin: 8px 0;
}

.theme-blackGold .ppt-slide-canvas {
  color: #f8fafc;
  background: linear-gradient(135deg, #050505, #78350f);
}

.theme-techBlue .ppt-slide-canvas {
  color: #eff6ff;
  background: linear-gradient(135deg, #08111f, #1d4ed8);
}

.ppt-slide-preview footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ppt-slide-preview footer button {
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
}

.ppt-slide-preview button:disabled {
  opacity: 0.45;
}

@media (max-width: 980px) {
  .ppt-slide-preview {
    grid-template-columns: 1fr;
  }

  .ppt-slide-preview aside {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
