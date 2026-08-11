<template>
  <view class="promotion-canvas-shell" :style="{ background: template.background }">
    <canvas
      class="promotion-poster-canvas"
      :canvas-id="canvasId"
      :id="canvasId"
      :style="{ width: `${width}px`, height: `${height}px` }"
    />
  </view>
</template>

<script setup lang="ts">
import { getCurrentInstance, nextTick, ref, watch } from "vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { getImageInfo } from "../../features/promotion/platform";
import { renderPromotionPoster } from "../../features/promotion/posterRenderer";
import type { PromotionProfile, PromotionTemplate } from "../../features/promotion/types";

const props = defineProps<{ template: PromotionTemplate; profile: PromotionProfile; qrPath: string }>();
const emit = defineEmits<{ ready: []; error: [message: string] }>();

const width = 354;
const height = 472;
const canvasId = `promotionPosterCanvas${Math.random().toString(36).slice(2, 8)}`;
const instance = getCurrentInstance();
const ready = ref(false);
let renderVersion = 0;
let drawChain: Promise<void> = Promise.resolve();

function componentProxy() {
  return instance?.proxy as unknown as object | undefined;
}

function errorMessage(reason: unknown, fallback: string) {
  if (reason instanceof Error && reason.message) return reason.message;
  if (reason && typeof reason === "object") {
    const record = reason as { errMsg?: unknown; message?: unknown };
    const message = String(record.errMsg || record.message || "").trim();
    if (message) return message;
  }
  return fallback;
}

async function resolveLocalImage(src: string) {
  if (!src) return "";
  try {
    const info = await getImageInfo(src);
    return info.path || src;
  } catch {
    return src;
  }
}

function drawOnce(version: number, logoPath: string, qrPath: string) {
  return new Promise<void>((resolve, reject) => {
    try {
      const context = uni.createCanvasContext(canvasId, componentProxy());
      renderPromotionPoster({
        context: context as unknown as Record<string, (...args: any[]) => any>,
        width,
        height,
        template: props.template,
        profile: props.profile,
        qrPath,
        logoPath,
      });
      context.draw(false, () => {
        if (version !== renderVersion) {
          resolve();
          return;
        }
        // WeChat old canvas needs a short settle window before export.
        setTimeout(() => {
          if (version !== renderVersion) {
            resolve();
            return;
          }
          ready.value = true;
          emit("ready");
          resolve();
        }, 48);
      });
    } catch (error) {
      reject(error);
    }
  });
}

async function generate() {
  if (!props.qrPath) throw new Error("小程序码尚未就绪");
  const version = ++renderVersion;
  ready.value = false;
  await nextTick();
  const [logoPath, qrPath] = await Promise.all([
    resolveLocalImage(loginLogo),
    resolveLocalImage(props.qrPath),
  ]);
  if (version !== renderVersion) return;
  const task = drawChain.then(() => drawOnce(version, logoPath, qrPath));
  drawChain = task.catch(() => undefined);
  try {
    await task;
  } catch (error) {
    if (version !== renderVersion) return;
    const message = errorMessage(error, "海报生成失败");
    emit("error", message);
    throw new Error(message);
  }
}

async function exportPoster() {
  await generate();
  if (!ready.value) throw new Error("海报画布尚未就绪，请重试");
  return new Promise<string>((resolve, reject) => {
    uni.canvasToTempFilePath({
      canvasId,
      x: 0,
      y: 0,
      width,
      height,
      destWidth: 1080,
      destHeight: 1440,
      fileType: "png",
      quality: 1,
      success: result => resolve(result.tempFilePath),
      fail: error => reject(new Error(errorMessage(error, "海报导出失败，请重试"))),
    }, componentProxy());
  });
}

watch(
  () => [props.template.id, props.qrPath, props.profile.inviteCode, props.profile.name] as const,
  () => {
    if (props.qrPath) void generate().catch(() => undefined);
  },
  { immediate: true },
);

defineExpose({ generate, exportPoster, ready });
</script>
