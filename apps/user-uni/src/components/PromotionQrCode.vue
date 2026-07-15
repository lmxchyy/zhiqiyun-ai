<template>
  <view
    class="promotion-qr-code"
    :style="{ width: `${size}px`, height: `${size}px` }"
    role="img"
    :aria-label="ariaLabel"
  >
    <canvas
      :id="canvasId"
      class="promotion-qr-code__canvas"
      :canvas-id="canvasId"
      :style="{ width: `${size}px`, height: `${size}px` }"
    />
    <view v-if="!ready" class="promotion-qr-code__state">
      <text>{{ errorMessage || "二维码生成中" }}</text>
      <view
        v-if="errorMessage"
        class="promotion-qr-code__retry"
        role="button"
        aria-label="重新生成推广二维码"
        @tap.stop="renderQrCode"
      >
        重试
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, nextTick, onMounted, ref, watch } from "vue";
import qrcode from "qrcode-generator";

const props = withDefaults(defineProps<{
  value: string;
  size?: number;
  label?: string;
}>(), {
  size: 168,
  label: "微信推广二维码",
});

const emit = defineEmits<{
  ready: [];
  error: [message: string];
}>();

let instanceSeed = 0;
const canvasId = `agent-promotion-qrcode-${Date.now()}-${++instanceSeed}`;
const componentInstance = getCurrentInstance();
const ready = ref(false);
const errorMessage = ref("");
let renderVersion = 0;

const size = computed(() => Math.max(120, Math.min(320, Math.round(props.size))));
const ariaLabel = computed(() => props.value.trim()
  ? `${props.label}，内容已生成`
  : `${props.label}，等待邀请码`);

async function renderQrCode() {
  const value = props.value.trim();
  const version = ++renderVersion;
  ready.value = false;
  errorMessage.value = "";

  if (!value) {
    errorMessage.value = "等待邀请码";
    return;
  }

  try {
    await nextTick();
    const qr = qrcode(0, "M");
    qr.addData(value, "Byte");
    qr.make();

    const context = uni.createCanvasContext(
      canvasId,
      componentInstance?.proxy as unknown as object,
    );
    const moduleCount = qr.getModuleCount();
    const quietZone = 4;
    const cellSize = Math.max(1, Math.floor(size.value / (moduleCount + quietZone * 2)));
    const qrSize = cellSize * (moduleCount + quietZone * 2);
    const offset = Math.floor((size.value - qrSize) / 2);

    context.setFillStyle("#FFFFFF");
    context.fillRect(0, 0, size.value, size.value);
    context.setFillStyle("#111827");
    for (let row = 0; row < moduleCount; row += 1) {
      for (let column = 0; column < moduleCount; column += 1) {
        if (!qr.isDark(row, column)) continue;
        context.fillRect(
          offset + (column + quietZone) * cellSize,
          offset + (row + quietZone) * cellSize,
          cellSize,
          cellSize,
        );
      }
    }
    context.draw(false, () => {
      if (version !== renderVersion) return;
      ready.value = true;
      emit("ready");
    });
  } catch (error) {
    if (version !== renderVersion) return;
    const message = error instanceof Error ? error.message : "二维码生成失败";
    errorMessage.value = "二维码生成失败";
    emit("error", message);
  }
}

watch(() => props.value, () => {
  void renderQrCode();
});

onMounted(() => {
  void renderQrCode();
});
</script>

<style scoped>
.promotion-qr-code {
  position: relative;
  overflow: hidden;
  background: #ffffff;
}

.promotion-qr-code__canvas {
  display: block;
}

.promotion-qr-code__state {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px;
  box-sizing: border-box;
  color: #6b7280;
  background: #ffffff;
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}

.promotion-qr-code__retry {
  min-width: 52px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  color: #ffffff;
  background: #7d8df6;
  font-size: 12px;
  line-height: 16px;
}
</style>
