<template>
  <section class="ppt-config-panel">
    <div class="ppt-config-row">
      <span>PPT页数</span>
      <div class="ppt-chip-row" role="group" aria-label="PPT页数">
        <button
          v-for="count in slideCountOptions"
          :key="count"
          type="button"
          :class="{ active: slideCount === count }"
          :aria-pressed="slideCount === count"
          :title="`选择 ${count} 页幻灯片`"
          :aria-label="`选择 ${count} 页幻灯片`"
          @click="$emit('update:slideCount', count)"
        >
          {{ count }}页
        </button>
      </div>
    </div>

    <div class="ppt-config-row ppt-text-content-section">
      <div class="ppt-config-title-line">
        <span>Text Content</span>
        <small>每张卡片的文字量</small>
      </div>
      <div class="ppt-text-content-grid">
        <button
          v-for="option in textContentOptions"
          :key="option.value"
          type="button"
          :class="{ active: textContent === option.value }"
          :aria-pressed="textContent === option.value"
          :title="`文字量：${option.label}`"
          :aria-label="`文字量：${option.label}，${option.description}`"
          @click="$emit('update:textContent', option.value)"
        >
          <span class="ppt-text-lines" aria-hidden="true">
            <i v-for="line in option.lines" :key="line" :style="{ width: line === option.lines ? '58%' : '82%' }"></i>
          </span>
          <strong>{{ option.label }}</strong>
          <small>{{ option.description }}</small>
        </button>
      </div>
    </div>

    <div class="ppt-config-grid">
      <label>
        <span>演示格式</span>
        <select
          :value="generationAspectRatio"
          title="演示格式"
          aria-label="演示格式"
          @change="$emit('update:generationAspectRatio', ($event.target as HTMLSelectElement).value as PptGenerationAspectRatio)"
        >
          <option value="dynamic">动态的</option>
          <option value="16:9">16:9</option>
        </select>
      </label>
      <label>
        <span>语言</span>
        <select
          :value="language"
          title="语言"
          aria-label="语言"
          @change="$emit('update:language', ($event.target as HTMLSelectElement).value as PptLanguage)"
        >
          <option value="zh">中文</option>
          <option value="en">英文</option>
        </select>
      </label>
      <label>
        <span>文案语气</span>
        <select
          :value="tone"
          title="文案语气"
          aria-label="文案语气"
          @change="$emit('update:tone', ($event.target as HTMLSelectElement).value as PptTone)"
        >
          <option v-for="option in toneOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label>
        <span>受众</span>
        <select
          :value="audience"
          title="受众"
          aria-label="受众"
          @change="$emit('update:audience', ($event.target as HTMLSelectElement).value as PptAudience)"
        >
          <option v-for="option in audienceOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label>
        <span>使用场景</span>
        <select
          :value="scenario"
          title="使用场景"
          aria-label="使用场景"
          @change="$emit('update:scenario', ($event.target as HTMLSelectElement).value as PptScenario)"
        >
          <option v-for="option in scenarioOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label>
        <span>图片来源</span>
        <select
          :value="imageSource"
          title="图片来源"
          aria-label="图片来源"
          @change="$emit('update:imageSource', ($event.target as HTMLSelectElement).value as PptImageSource)"
        >
          <option value="ai">AI生成图片</option>
          <option value="stock">图库图片</option>
          <option value="none">无图片</option>
        </select>
      </label>
      <label>
        <span>文本模型</span>
        <select
          :value="textModel"
          title="文本模型"
          aria-label="文本模型"
          @change="$emit('update:textModel', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="model in textModels" :key="model.value" :value="model.value">{{ model.label }}</option>
        </select>
      </label>
      <label v-if="imageSource === 'ai'">
        <span>图片模型</span>
        <select
          :value="imageModel"
          title="图片模型"
          aria-label="图片模型"
          @change="$emit('update:imageModel', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="model in imageModels" :key="model.value" :value="model.value">{{ model.label }}</option>
        </select>
      </label>
      <label class="ppt-switch">
        <span>联网搜索</span>
        <el-switch
          :model-value="enableWebSearch"
          title="联网搜索"
          aria-label="联网搜索"
          @update:model-value="$emit('update:enableWebSearch', Boolean($event))"
        />
      </label>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { PptAudience, PptGenerationAspectRatio, PptImageSource, PptLanguage, PptModelOption, PptScenario, PptTextContent, PptTone } from "../../types/ppt";

defineProps<{
  slideCount: number;
  language: PptLanguage;
  tone: PptTone;
  textContent: PptTextContent;
  audience: PptAudience;
  scenario: PptScenario;
  generationAspectRatio: PptGenerationAspectRatio;
  imageSource: PptImageSource;
  textModel: string;
  imageModel: string;
  enableWebSearch: boolean;
  textModels: PptModelOption[];
  imageModels: PptModelOption[];
}>();

defineEmits<{
  "update:slideCount": [value: number];
  "update:language": [value: PptLanguage];
  "update:tone": [value: PptTone];
  "update:textContent": [value: PptTextContent];
  "update:audience": [value: PptAudience];
  "update:scenario": [value: PptScenario];
  "update:generationAspectRatio": [value: PptGenerationAspectRatio];
  "update:imageSource": [value: PptImageSource];
  "update:textModel": [value: string];
  "update:imageModel": [value: string];
  "update:enableWebSearch": [value: boolean];
}>();

const slideCountOptions = Array.from({ length: 12 }, (_, index) => index + 1);
const textContentOptions: Array<{ value: PptTextContent; label: string; description: string; lines: number }> = [
  { value: "minimal", label: "Minimal", description: "只保留核心句", lines: 2 },
  { value: "concise", label: "Concise", description: "适合路演汇报", lines: 3 },
  { value: "detailed", label: "Detailed", description: "包含解释说明", lines: 3 },
  { value: "extensive", label: "Extensive", description: "内容更完整", lines: 4 }
];
const toneOptions: Array<{ value: PptTone; label: string }> = [
  { value: "professional", label: "专业正式" },
  { value: "simple", label: "轻松易懂" },
  { value: "marketing", label: "营销转化" },
  { value: "education", label: "教育培训" },
  { value: "pitch", label: "汇报路演" }
];
const audienceOptions: Array<{ value: PptAudience; label: string }> = [
  { value: "auto", label: "自动判断" },
  { value: "general", label: "大众用户" },
  { value: "business", label: "企业客户" },
  { value: "investor", label: "投资人" },
  { value: "teacher", label: "教师/讲师" },
  { value: "student", label: "学生/学员" }
];
const scenarioOptions: Array<{ value: PptScenario; label: string }> = [
  { value: "auto", label: "自动判断" },
  { value: "general", label: "通用演示" },
  { value: "analysis-report", label: "分析报告" },
  { value: "teaching-training", label: "教学培训" },
  { value: "promotional-materials", label: "营销物料" },
  { value: "public-speeches", label: "公开演讲" }
];
</script>

<style scoped>
.ppt-config-panel {
  display: grid;
  gap: 16px;
}

.ppt-config-row {
  display: grid;
  gap: 10px;
}

.ppt-config-row > span,
.ppt-config-title-line > span,
.ppt-config-grid span {
  color: #d4d4d8;
  font-size: 13px;
  font-weight: 760;
}

.ppt-config-title-line {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}

.ppt-config-title-line small {
  color: #8f8f95;
  font-size: 12px;
}

.ppt-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ppt-chip-row button {
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid #2b2b2b;
  border-radius: 999px;
  color: #f4f4f5;
  background: #0d0d0d;
  cursor: pointer;
}

.ppt-chip-row button.active {
  border-color: #737373;
  background: #292929;
}

.ppt-text-content-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.ppt-text-content-grid button {
  display: grid;
  justify-items: center;
  gap: 8px;
  min-height: 118px;
  padding: 14px 10px;
  border: 2px solid #242424;
  border-radius: 8px;
  color: #e4e4e7;
  background: #0d0d0d;
  cursor: pointer;
  transition: border-color 0.16s ease, background-color 0.16s ease, color 0.16s ease;
}

.ppt-text-content-grid button:hover,
.ppt-text-content-grid button.active {
  border-color: #52525b;
  background: #18181b;
}

.ppt-text-content-grid button.active {
  color: #f8fafc;
  box-shadow: inset 0 0 0 1px rgba(244, 244, 245, 0.18);
}

.ppt-text-lines {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 5px;
  width: 100%;
  height: 38px;
}

.ppt-text-lines i {
  display: block;
  height: 4px;
  border-radius: 999px;
  background: #71717a;
}

.ppt-text-content-grid button.active .ppt-text-lines i {
  background: #f4f4f5;
}

.ppt-text-content-grid strong {
  color: currentColor;
  font-size: 13px;
}

.ppt-text-content-grid small {
  color: #8f8f95;
  font-size: 11px;
}

.ppt-config-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.ppt-config-grid label {
  display: grid;
  gap: 8px;
}

.ppt-config-grid select {
  min-height: 38px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0d0d0d;
  padding: 0 10px;
}

.ppt-switch {
  align-content: start;
}

@media (max-width: 900px) {
  .ppt-text-content-grid,
  .ppt-config-grid {
    grid-template-columns: 1fr;
  }
}
</style>
