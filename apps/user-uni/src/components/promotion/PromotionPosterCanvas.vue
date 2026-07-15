<template>
  <view class="promotion-canvas-shell" :style="{ background: template.background }">
    <canvas class="promotion-poster-canvas" :canvas-id="canvasId" :id="canvasId" />
  </view>
</template>

<script setup lang="ts">
import { getCurrentInstance, nextTick, watch } from "vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { renderPromotionPoster } from "../../features/promotion/posterRenderer";
import type { PromotionProfile, PromotionTemplate } from "../../features/promotion/types";

const props = defineProps<{ template: PromotionTemplate; profile: PromotionProfile; qrPath: string }>();
const emit = defineEmits<{ ready: []; error: [message: string] }>();
const canvasId = `promotionPosterCanvas${Math.random().toString(36).slice(2, 8)}`;
const instance = getCurrentInstance();

async function generate() {
  if (!props.qrPath) throw new Error("小程序码尚未就绪");
  await nextTick();
  return new Promise<void>((resolve, reject) => {
    try {
      const context = uni.createCanvasContext(canvasId, instance?.proxy);
      renderPromotionPoster({ context: context as unknown as Record<string, (...args: any[]) => any>, width: 354, height: 472, template: props.template, profile: props.profile, qrPath: props.qrPath, logoPath: loginLogo });
      context.draw(false, () => { emit("ready"); resolve(); });
    } catch (error) {
      const message = error instanceof Error ? error.message : "海报生成失败";
      emit("error", message); reject(error);
    }
  });
}

async function exportPoster() {
  await generate();
  return new Promise<string>((resolve, reject) => {
    uni.canvasToTempFilePath({ canvasId, x: 0, y: 0, width: 354, height: 472, destWidth: 1080, destHeight: 1440, fileType: "png", quality: 1, success: result => resolve(result.tempFilePath), fail: reject }, instance?.proxy);
  });
}

watch(() => [props.template.id, props.qrPath, props.profile.inviteCode], () => { if (props.qrPath) void generate(); }, { immediate: true });
defineExpose({ generate, exportPoster });
</script>
