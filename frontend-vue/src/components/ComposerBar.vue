<template>
  <view class="composer">
    <textarea v-model="prompt" placeholder="输入你想要生成的画面，也可以先上传参考图" />
    <view class="composer-controls">
      <button type="button" class="upload-button">
        <text class="upload-icon">⌘</text>
        <text>上传</text>
      </button>
      <view class="credit-control">剩余 <text>959</text></view>
      <view class="number-control">
        <text>张数</text>
        <picker :range="countOptions" :value="countIndex" @change="onCountChange">
          <view class="inline-picker"><text>{{ count }}</text><text class="chevron">⌄</text></view>
        </picker>
      </view>
      <picker :range="ratios" :value="ratioIndex" @change="onRatioChange">
        <view class="picker-control">
          <text>比例</text>
          <text class="ratio-icon"></text>
          <text>{{ ratio }}</text>
          <text class="chevron">⌄</text>
        </view>
      </picker>
      <button class="send-button" aria-label="生成" @click="$emit('submit')">↑</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ModelInfo } from "../types";

const props = defineProps<{ models: ModelInfo[] }>();
defineEmits<{ submit: [] }>();

const prompt = defineModel<string>("prompt", { required: true });
const model = defineModel<string>("model", { required: true });
const count = defineModel<number>("count", { required: true });
const ratio = defineModel<string>("ratio", { required: true });

const ratios = ["4:3", "1:1", "3:4", "16:9", "9:16"];
const countOptions = [1, 2, 3, 4];
const modelNames = computed(() => props.models.map(item => item.name));
const modelIndex = computed(() => Math.max(0, props.models.findIndex(item => item.code === model.value)));
const countIndex = computed(() => Math.max(0, countOptions.indexOf(count.value)));
const ratioIndex = computed(() => Math.max(0, ratios.indexOf(ratio.value)));
const currentModelName = computed(() => props.models[modelIndex.value]?.name || "加载中");

type PickerChangeEvent = { detail: { value: string | number } };

function onModelChange(event: PickerChangeEvent) {
  const index = Number(event.detail.value);
  model.value = props.models[index]?.code || model.value;
}

function onCountChange(event: PickerChangeEvent) {
  const index = Number(event.detail.value);
  count.value = countOptions[index] || count.value;
}

function onRatioChange(event: PickerChangeEvent) {
  const index = Number(event.detail.value);
  ratio.value = ratios[index] || ratio.value;
}
</script>
