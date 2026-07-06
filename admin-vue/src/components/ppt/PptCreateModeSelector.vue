<template>
  <section class="ppt-mode-selector">
    <button
      v-for="item in modeOptions"
      :key="item.value"
      type="button"
      :class="{ active: modelValue === item.value }"
      :aria-pressed="modelValue === item.value"
      :title="`选择创建方式：${item.title}`"
      :aria-label="`选择创建方式：${item.title}，${item.description}`"
      @click="$emit('update:modelValue', item.value)"
    >
      <el-icon><component :is="item.icon" /></el-icon>
      <span>{{ item.title }}</span>
      <small>{{ item.description }}</small>
      <em>{{ item.badge }}</em>
    </button>
    <label class="ppt-document-upload">
      <input type="file" accept=".pdf,.doc,.docx,.txt,.md" title="上传文档生成PPT" aria-label="上传文档生成PPT" @change="handleFile" />
      <span>{{ uploadedDocumentName || "上传文档生成PPT" }}</span>
    </label>
  </section>
</template>

<script setup lang="ts">
import { Document, EditPen, MagicStick } from "@element-plus/icons-vue";
import type { PptCreateMode, PptCreateModeOption } from "../../types/ppt";

defineProps<{
  modelValue: PptCreateMode;
  uploadedDocumentName?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: PptCreateMode];
  "upload-document": [file?: File];
}>();

const modeOptions: PptCreateModeOption[] = [
  { value: "ai", title: "AI生成PPT", description: "输入主题，先生成大纲再生成PPT。", badge: "可用", icon: MagicStick },
  { value: "blank", title: "空白PPT", description: "从空白演示文稿开始手动编辑。", badge: "Mock", icon: EditPen },
  { value: "document", title: "文档生成PPT", description: "上传 PDF / Word / TXT / Markdown。", badge: "预留", icon: Document }
];

function handleFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  emit("update:modelValue", "document");
  emit("upload-document", file);
}
</script>

<style scoped>
.ppt-mode-selector {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.ppt-mode-selector button,
.ppt-document-upload {
  position: relative;
  display: grid;
  gap: 8px;
  min-height: 112px;
  padding: 14px;
  border: 1px solid #242424;
  border-radius: 10px;
  color: #f4f4f5;
  background: #0d0d0d;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.16s ease, background-color 0.16s ease, transform 0.16s ease;
}

.ppt-mode-selector button:hover,
.ppt-document-upload:hover,
.ppt-mode-selector button.active {
  border-color: #454545;
  background: #171717;
  transform: translateY(-1px);
}

.ppt-mode-selector .el-icon {
  font-size: 20px;
}

.ppt-mode-selector span {
  font-weight: 820;
}

.ppt-mode-selector small {
  color: #9b9b9f;
  line-height: 1.5;
}

.ppt-mode-selector em {
  position: absolute;
  right: 12px;
  top: 12px;
  color: #a7f3d0;
  font-size: 12px;
  font-style: normal;
}

.ppt-document-upload {
  min-height: 42px;
  grid-column: 1 / -1;
  place-items: center;
  text-align: center;
}

.ppt-document-upload input {
  display: none;
}

@media (max-width: 900px) {
  .ppt-mode-selector {
    grid-template-columns: 1fr;
  }
}
</style>
