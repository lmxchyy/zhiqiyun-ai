<template>
  <view class="asset-upload">
    <view class="asset-grid">
      <view v-for="(asset, index) in modelValue" :key="asset.assetId || asset.localPath || index" class="asset-card">
        <image class="asset-preview" :src="asset.previewUrl || asset.localPath" mode="aspectFill" />
        <view v-if="asset.status !== 'uploaded'" :class="['asset-state', asset.status]">
          <text>{{ asset.status === "uploading" ? "上传中" : "上传失败" }}</text>
          <button v-if="asset.status === 'failed'" @click="retry(index)">重试</button>
        </view>
        <button class="asset-remove" aria-label="删除素材" @click="remove(index)">×</button>
      </view>
      <button v-if="canAdd" class="asset-add" @click="choose">
        <text class="asset-add-icon">＋</text>
        <text>添加图片</text>
        <small>{{ modelValue.length }}/{{ maxItems }}</small>
      </button>
    </view>
    <text v-if="acceptLabel" class="asset-hint">支持 {{ acceptLabel }}</text>
    <text v-if="error" class="asset-error">{{ error }}</text>
  </view>
</template>

<script setup lang="ts">
import { computed, nextTick } from "vue";
import { uploadReferenceAsset } from "../../../api/files";
import { templateAssetMaxItems } from "../../../features/inspiration/contracts";
import type { PublicTemplateInput, TemplateUploadedAsset } from "../../../features/inspiration/types";

const props = defineProps<{
  input: PublicTemplateInput;
  modelValue: TemplateUploadedAsset[];
  error?: string;
}>();
const emit = defineEmits<{ "update:modelValue": [value: TemplateUploadedAsset[]] }>();

const maxItems = computed(() => templateAssetMaxItems(props.input));
const canAdd = computed(() => props.modelValue.length < maxItems.value);
const acceptLabel = computed(() => (props.input.validation?.accept || []).join("、"));

function inferredMime(path: string) {
  const extension = path.split(/[?#]/)[0].split(".").pop()?.toLowerCase();
  if (extension === "png") return "image/png";
  if (extension === "webp") return "image/webp";
  if (extension === "gif") return "image/gif";
  return "image/jpeg";
}

function replace(localPath: string, asset: TemplateUploadedAsset) {
  const index = props.modelValue.findIndex(item => item.localPath === localPath);
  if (index < 0) return;
  const next = props.modelValue.slice();
  next[index] = asset;
  emit("update:modelValue", next);
}

async function upload(localPath: string, name = "") {
  replace(localPath, {
    assetId: "",
    localPath,
    previewUrl: localPath,
    name: name || localPath.split(/[\\/]/).pop(),
    mimeType: inferredMime(localPath),
    status: "uploading",
  });
  try {
    const result = await uploadReferenceAsset(localPath);
    replace(localPath, {
      assetId: result.assetId,
      localPath,
      previewUrl: result.previewUrl || localPath,
      name: result.name || name,
      mimeType: result.mimeType || inferredMime(localPath),
      status: "uploaded",
    });
  } catch (reason) {
    const current = props.modelValue.find(item => item.localPath === localPath);
    replace(localPath, {
      ...current,
      assetId: "",
      localPath,
      previewUrl: localPath,
      mimeType: current?.mimeType || inferredMime(localPath),
      status: "failed",
      error: reason instanceof Error ? reason.message : "上传失败",
    });
  }
}

async function append(paths: string[]) {
  const available = Math.max(0, maxItems.value - props.modelValue.length);
  const existing = new Set(props.modelValue.map(item => item.localPath).filter(Boolean));
  const selected = paths.filter((path, index, all) => !existing.has(path) && all.indexOf(path) === index).slice(0, available);
  if (!selected.length) return;
  emit("update:modelValue", [
    ...props.modelValue,
    ...selected.map(localPath => ({
      assetId: "",
      localPath,
      previewUrl: localPath,
      mimeType: inferredMime(localPath),
      status: "uploading" as const,
    })),
  ]);
  await nextTick();
  for (const localPath of selected) {
    await upload(localPath);
    await nextTick();
  }
}

function choose() {
  if (props.input.type !== "IMAGE") {
    uni.showToast({ title: "本阶段仅开放图片素材上传", icon: "none" });
    return;
  }
  uni.chooseImage({
    count: Math.max(1, maxItems.value - props.modelValue.length),
    sizeType: ["compressed"],
    sourceType: ["album", "camera"],
    success: response => {
      const paths = Array.isArray(response.tempFilePaths) ? response.tempFilePaths : [response.tempFilePaths];
      void append(paths.map(String));
    },
  });
}

function remove(index: number) {
  emit("update:modelValue", props.modelValue.filter((_, current) => current !== index));
}

function retry(index: number) {
  const asset = props.modelValue[index];
  if (asset?.localPath) void upload(asset.localPath, asset.name);
}
</script>

<style scoped>
.asset-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.asset-card,.asset-add{position:relative;aspect-ratio:1;border-radius:12px;overflow:hidden}.asset-card{background:#eef1f6}.asset-preview{width:100%;height:100%}.asset-state{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;flex-direction:column;gap:6px;color:#fff;background:rgba(25,31,50,.62);font-size:11px}.asset-state.failed{background:rgba(142,48,55,.72)}.asset-state button{width:auto;padding:3px 9px;border:0;border-radius:12px;color:#8b3440;background:#fff;font-size:10px;line-height:22px}.asset-remove{position:absolute;z-index:2;right:5px;top:5px;width:25px;height:25px;padding:0;border:0;border-radius:50%;color:#fff;background:rgba(18,24,42,.68);font-size:18px;line-height:25px}.asset-add{display:flex;align-items:center;justify-content:center;flex-direction:column;border:1px dashed #cbd2df;color:#6f788a;background:#f8f9fc;font-size:11px}.asset-add-icon{color:#5269e8;font-size:25px;line-height:28px}.asset-add small{margin-top:3px;color:#9aa1b0;font-size:9px}.asset-hint,.asset-error{display:block;margin-top:7px;font-size:10px;line-height:16px}.asset-hint{color:#8b93a3}.asset-error{color:#d94d5e}button:after{border:0}
</style>
