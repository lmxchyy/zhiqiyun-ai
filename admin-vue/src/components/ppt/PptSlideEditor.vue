<template>
  <section v-if="draft" class="ppt-slide-editor">
    <header>
      <strong>PPT内容编辑</strong>
      <span>第一版支持标题、正文和要点编辑。</span>
    </header>
    <label>
      <span>当前页标题</span>
      <input v-model="draft.title" title="当前页标题" aria-label="当前页标题" />
    </label>
    <label>
      <span>正文说明</span>
      <textarea v-model="draft.content" title="正文说明" aria-label="正文说明"></textarea>
    </label>
    <label>
      <span>要点列表（每行一个）</span>
      <textarea v-model="bulletText" title="要点列表" aria-label="要点列表"></textarea>
    </label>
    <footer>
      <button type="button" title="取消本次修改" aria-label="取消本次修改" @click="reset">取消修改</button>
      <button type="button" title="重新生成当前页" aria-label="重新生成当前页" @click="$emit('regenerate')">重新生成当前页</button>
      <button type="button" class="primary" title="保存当前页修改" aria-label="保存当前页修改" @click="save">保存修改</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import type { PptSlide } from "../../types/ppt";

const props = defineProps<{
  slide: PptSlide | null;
}>();

const emit = defineEmits<{
  save: [patch: Partial<PptSlide>];
  regenerate: [];
}>();

const draft = ref<PptSlide | null>(null);
const bulletText = ref("");

watch(() => props.slide, reset, { immediate: true });

function reset() {
  draft.value = props.slide ? { ...props.slide, bulletPoints: [...props.slide.bulletPoints] } : null;
  bulletText.value = props.slide?.bulletPoints.join("\n") || "";
}

function save() {
  if (!draft.value) return;
  emit("save", {
    title: draft.value.title,
    content: draft.value.content,
    bulletPoints: bulletText.value.split("\n").map(item => item.trim()).filter(Boolean)
  });
}
</script>

<style scoped>
.ppt-slide-editor {
  display: grid;
  gap: 12px;
}

.ppt-slide-editor header,
.ppt-slide-editor label {
  display: grid;
  gap: 6px;
}

.ppt-slide-editor strong {
  color: #f4f4f5;
}

.ppt-slide-editor span {
  color: #a1a1aa;
  font-size: 13px;
}

.ppt-slide-editor input,
.ppt-slide-editor textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0d0d0d;
  padding: 10px;
}

.ppt-slide-editor textarea {
  min-height: 82px;
  resize: vertical;
}

.ppt-slide-editor footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ppt-slide-editor button {
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
}

.ppt-slide-editor button.primary {
  color: #111;
  background: #f4f4f5;
}
</style>
