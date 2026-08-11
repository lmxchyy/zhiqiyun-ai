<template>
  <section class="sv-card" aria-label="素材上传">
    <div class="sv-head">
      <h2>素材</h2>
      <div class="sv-inline">
        <button type="button" class="sv-btn ghost" :disabled="!store.activeUpload" @click="store.pauseUpload()">暂停</button>
        <button type="button" class="sv-btn ghost" :disabled="!store.activeUpload" @click="store.resumeUpload()">继续</button>
        <button type="button" class="sv-btn ghost" :disabled="!store.activeUpload" @click="store.cancelUpload()">取消</button>
      </div>
    </div>

    <label
      class="sv-drop"
      :class="{ 'is-dragging': dragging }"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      <input ref="fileInput" type="file" accept="video/*,image/*" multiple hidden @change="onPick" />
      <strong>拖拽上传视频 / 图片</strong>
      <span>支持分片续传，适合大文件</span>
      <button type="button" class="sv-btn primary" @click.prevent="fileInput?.click()">选择文件</button>
    </label>

    <ul v-if="store.uploads.length" class="sv-list" role="list">
      <li v-for="item in store.uploads" :key="item.id">
        <div>
          <strong>{{ item.name }}</strong>
          <span>{{ item.status }} · {{ item.progress }}%</span>
          <p v-if="item.error">{{ item.error }}</p>
        </div>
        <div class="sv-progress" :aria-valuenow="item.progress" aria-valuemin="0" aria-valuemax="100" role="progressbar">
          <i :style="{ width: `${item.progress}%` }" />
        </div>
      </li>
    </ul>

    <ul v-if="store.sortedAssets.length" class="sv-assets" role="list">
      <li v-for="asset in store.sortedAssets" :key="asset.id">
        <div>
          <strong>{{ asset.metadata?.originalName || asset.assetType }}</strong>
          <span>{{ asset.analysisStatus || "待分析" }} · {{ asset.assetType }}</span>
        </div>
        <div class="sv-inline">
          <button
            v-if="String(asset.analysisStatus || '').toUpperCase() === 'FAILED'"
            type="button"
            class="sv-btn ghost"
            @click="store.retryAssetAnalysis(asset.id)"
          >
            重试分析
          </button>
          <button type="button" class="sv-btn danger ghost" @click="store.removeAsset(asset.id)">移除</button>
        </div>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();
const fileInput = ref<HTMLInputElement | null>(null);
const dragging = ref(false);

function onPick(event: Event) {
  const input = event.target as HTMLInputElement;
  if (input.files?.length) void store.uploadFiles(input.files);
  input.value = "";
}

function onDrop(event: DragEvent) {
  dragging.value = false;
  const files = event.dataTransfer?.files;
  if (files?.length) void store.uploadFiles(files);
}
</script>

<style scoped>
.sv-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  background: rgba(23, 27, 36, 0.92);
  padding: 16px;
}

.sv-head,
.sv-inline {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sv-head {
  justify-content: space-between;
  margin-bottom: 12px;
}

.sv-drop {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  padding: 18px;
  border: 1px dashed rgba(255, 255, 255, 0.18);
  border-radius: 14px;
  cursor: pointer;
}

.sv-drop.is-dragging {
  border-color: #ff771b;
  background: rgba(255, 119, 27, 0.08);
}

.sv-drop span,
.sv-list span,
.sv-assets span,
.sv-list p {
  color: #9aa3b5;
  font-size: 12px;
}

.sv-list,
.sv-assets {
  list-style: none;
  margin: 14px 0 0;
  padding: 0;
  display: grid;
  gap: 10px;
}

.sv-list li,
.sv-assets li {
  display: grid;
  gap: 8px;
  padding: 10px 0;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.sv-assets li {
  grid-template-columns: 1fr auto;
  align-items: center;
}

.sv-progress {
  height: 6px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.sv-progress i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, #423499, #ff771b);
}

.sv-btn {
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: #f4f6fb;
  padding: 6px 12px;
  cursor: pointer;
}

.sv-btn.primary {
  border: 0;
  background: #ff771b;
  color: #111;
  font-weight: 600;
}

.sv-btn.ghost {
  background: transparent;
}

.sv-btn.danger {
  color: #ff8f8f;
}

.sv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
