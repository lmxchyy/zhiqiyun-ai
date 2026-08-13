<template>
  <view class="v31-mode-grid">
    <button
      v-for="module in creationModules"
      :key="module.id"
      :class="['v31-mode-card', { active: activeMode === module.id }]"
      @click="emit('select-mode', module.id)"
    >
      <RemoteCover
        class="v31-tool-cover"
        page-code="studio"
        :slot-key="slotKeyForMode(module.id)"
        :alt="module.name"
        width="36px"
        height="36px"
        radius="10px"
      />
      <view class="v31-tool-copy">
        <text class="v31-tool-name">{{ module.name }}</text>
        <text class="v31-tool-desc">{{ module.description }}</text>
      </view>
    </button>
  </view>
</template>

<script setup lang="ts">
import RemoteCover from "../../RemoteCover.vue";
import type { WorkbenchCreationModule } from "../../../features/workbench/catalog";
import type { MiniProgramCreationMode } from "../../../config/miniProgramPages";

interface Props {
  creationModules: WorkbenchCreationModule[];
  activeMode: MiniProgramCreationMode;
  slotKeyForMode: (mode: MiniProgramCreationMode) => string;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: "select-mode", mode: MiniProgramCreationMode): void;
}>();
</script>

<style scoped>
.v31-mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.v31-mode-card {
  display: flex;
  min-width: 0;
  min-height: 58px;
  margin: 0;
  padding: 8px;
  align-items: center;
  gap: 8px;
  border: 1px solid #e4e9f7;
  border-radius: 12px;
  background: #ffffff;
  text-align: left;
  box-shadow: 0 4px 12px rgba(23, 28, 56, 0.03);
}

.v31-mode-card::after {
  display: none;
}

.v31-mode-card.active {
  border-color: #c9d2ff;
  background: linear-gradient(180deg, #f7f8ff 0%, #ffffff 100%);
  box-shadow: 0 8px 18px rgba(93, 83, 208, 0.12);
}

.v31-tool-cover {
  width: 36px !important;
  height: 36px !important;
  flex: 0 0 36px;
}

.v31-tool-copy {
  min-width: 0;
  flex: 1;
}

.v31-tool-name {
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 600;
  line-height: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-tool-desc {
  display: -webkit-box;
  margin-top: 2px;
  overflow: hidden;
  color: #697386;
  font-size: 10px;
  line-height: 13px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
</style>
