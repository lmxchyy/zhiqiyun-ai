<template>
  <view class="section-stack">
    <view class="v31-filter-card">
      <view class="v31-filter-row">
        <button
          v-for="filter in filters"
          :key="filter.id"
          :class="{ active: activeFilter === filter.id }"
          @click="emit('update:filter', filter.id)"
        >
          {{ filter.label }}
        </button>
      </view>
      <input
        :value="search"
        class="v31-search-strip"
        placeholder="搜索作品名称"
        @input="emit('update:search', inputValue($event))"
      />
    </view>
    <view class="v31-works-card">
      <text v-if="loading" class="empty-text">正在加载作品...</text>
      <text v-else-if="error" class="empty-text">{{ error }}</text>
      <view v-else-if="filteredAssets.length" class="v31-work-grid">
        <button v-for="asset in filteredAssets" :key="asset.id" class="v31-work-card" @click="emit('open-asset', asset)">
          <AppImage
            v-if="asset.thumbnailUrl"
            class="v31-work-preview"
            :src="asset.thumbnailUrl"
            :fallback="fallbackSlotFor(asset.mediaType)"
            :alt="asset.name"
            width="100%"
            height="86px"
            radius="12px"
          />
          <RemoteCover
            v-else
            class="v31-work-preview"
            page-code="assets"
            :slot-key="slotKeyFor(asset.mediaType)"
            :alt="asset.name"
            width="100%"
            height="86px"
            radius="12px"
          />
          <text class="v31-work-title">{{ asset.name || asset.id }}</text>
          <view class="v31-card-footer">
            <text :class="['v31-chip', asset.mediaType === 'video' ? 'green' : asset.mediaType === 'image' ? 'orange' : 'purple']">
              {{ asset.mediaType === 'video' ? '视频' : asset.mediaType === 'image' ? '图片' : 'PPT' }}
            </text>
            <text class="v31-link">继续改</text>
          </view>
        </button>
      </view>
      <view v-else class="v31-empty-state">
        <text>没有找到符合条件的作品</text>
        <button @click="emit('reset')">查看全部</button>
      </view>
      <text class="v31-works-note">每个作品保留生成参数、消耗点数、导出记录。</text>
      <view class="v31-batch-actions"><button class="active" @click="emit('continue-create')">继续创作</button></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import AppImage from "../../AppImage.vue";
import RemoteCover from "../../RemoteCover.vue";
import type { WorkbenchAssetFilter, WorkbenchAssetFilterOption } from "../../../features/workbench/catalog";
import type { Asset } from "../../../types";

interface Props {
  filters: WorkbenchAssetFilterOption[];
  activeFilter: WorkbenchAssetFilter;
  search: string;
  loading: boolean;
  error: string;
  filteredAssets: Asset[];
  slotKeyFor: (mediaType?: string | null) => string;
  fallbackSlotFor: (mediaType?: string | null) => string | undefined;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'update:filter', filterId: WorkbenchAssetFilter): void;
  (e: 'update:search', value: string): void;
  (e: 'open-asset', asset: Asset): void;
  (e: 'reset'): void;
  (e: 'continue-create'): void;
}>();

function inputValue(event: unknown) {
  const target = event as { detail?: { value?: string } } | { target?: { value?: string } };
  return target && typeof target === 'object'
    ? String((target as { detail?: { value?: string } }).detail?.value ?? (target as { target?: { value?: string } }).target?.value ?? '')
    : '';
}
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
