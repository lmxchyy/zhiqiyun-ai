<template>
  <view :class="['composer', { 'has-reference': referenceImages.length > 0 }]">
    <view v-if="referenceImages.length" class="reference-preview-list">
      <view v-for="(image, index) in referenceImages" :key="`${image.path}-${index}`" class="reference-preview">
        <image :src="image.path" mode="aspectFill" />
        <button type="button" class="reference-remove" :aria-label="`移除第 ${index + 1} 张参考图`" @click="removeReference(index)">×</button>
      </view>
    </view>
    <textarea v-model="prompt" placeholder="描述你希望如何修改参考图" />
    <view class="composer-controls">
      <button type="button" class="upload-button" @click="chooseImage">
        <text class="upload-icon" aria-hidden="true"></text>
        <text>{{ uploadedImageName || "上传" }}</text>
      </button>
      <view class="credit-control">剩余 <text>{{ quota }}</text></view>
      <view class="number-control count-control">
        <button type="button" class="count-trigger" aria-label="选择生成张数" @click.stop="countMenuOpen = !countMenuOpen">
          <text>张数</text>
          <text>{{ count }}</text>
          <text class="chevron">⌄</text>
        </button>
        <view v-if="countMenuOpen" class="count-menu">
          <button
            v-for="option in countOptions"
            :key="option"
            type="button"
            :class="['count-option', { active: option === count }]"
            @click.stop="selectCount(option)"
          >
            {{ option }}
          </button>
        </view>
      </view>
      <picker :range="ratios" :value="ratioIndex" @change="onRatioChange">
        <view class="picker-control">
          <text>比例</text>
          <text class="ratio-icon"></text>
          <text>{{ ratio }}</text>
          <text class="chevron">⌄</text>
        </view>
      </picker>
      <button class="send-button" aria-label="生成" :disabled="quota < count" @click="$emit('submit')">↑</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { ModelInfo, ReferenceImage } from "../types";

const props = defineProps<{ models: ModelInfo[]; quota: number }>();

const emit = defineEmits<{ submit: []; upload: [images: ReferenceImage[]] }>();

const prompt = defineModel<string>("prompt", { required: true });
const model = defineModel<string>("model", { required: true });
const count = defineModel<number>("count", { required: true });
const ratio = defineModel<string>("ratio", { required: true });
const referenceImages = defineModel<ReferenceImage[]>("referenceImages", { required: true });

const ratios = ["4:3", "1:1", "3:4", "16:9", "9:16"];
const countOptions = [1, 2, 3, 4, 5, 6, 7, 8];
const countMenuOpen = ref(false);
const uploadedImageName = computed(() => (referenceImages.value.length ? `已上传 ${referenceImages.value.length}` : ""));
const ratioIndex = computed(() => Math.max(0, ratios.indexOf(ratio.value)));

type PickerChangeEvent = { detail: { value: string | number } };

function selectCount(value: number) {
  count.value = Math.min(value, Math.max(1, props.quota || 1));
  countMenuOpen.value = false;
}

function onRatioChange(event: PickerChangeEvent) {
  const index = Number(event.detail.value);
  ratio.value = ratios[index] || ratio.value;
}

function chooseImage() {
  if (referenceImages.value.length >= 6) {
    uni.showToast({ title: "最多上传 6 张参考图", icon: "none" });
    return;
  }
  uni.chooseImage({
    count: Math.max(1, 6 - referenceImages.value.length),
    mediaType: ["image"],
    sourceType: ["album", "camera"],
    success: result => {
      const files = Array.isArray(result.tempFiles) ? result.tempFiles : result.tempFiles ? [result.tempFiles] : [];
      const paths = result.tempFilePaths || [];
      const imageCount = Math.max(files.length, paths.length);
      const pickedImages = Array.from({ length: imageCount })
        .map((_, index) => {
          const file = files[index];
          const filePath = file && "path" in file && typeof file.path === "string" ? file.path : "";
          const blobPath = typeof File !== "undefined" && file instanceof File ? URL.createObjectURL(file) : "";
          const path = paths[index] || filePath || blobPath;
          if (!path) return null;
          const name = file && "name" in file && typeof file.name === "string" ? file.name : path.split(/[\\/]/).pop() || `参考图 ${index + 1}`;
          return { path, name };
        })
        .filter((image): image is ReferenceImage => Boolean(image));
      if (!pickedImages.length) return;
      referenceImages.value = [...referenceImages.value, ...pickedImages].slice(0, 6);
      emit("upload", referenceImages.value);
    },
    fail: () => {
      uni.showToast({ title: "未选择图片", icon: "none" });
    }
  });
}

function removeReference(index: number) {
  referenceImages.value = referenceImages.value.filter((_, imageIndex) => imageIndex !== index);
  emit("upload", referenceImages.value);
}
</script>
