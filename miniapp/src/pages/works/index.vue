<template>
  <zq-page-shell>
    <view class="page-head">
      <text class="page-head__title">作品</text>
      <text class="page-head__copy">管理图片、视频和 PPT 交付物。</text>
    </view>

    <wd-search v-model="keyword" placeholder="搜索作品、任务 ID 或模型" hide-cancel />

    <view class="filter-row">
      <text
        v-for="tab in typeTabs"
        :key="tab.value"
        class="filter-chip"
        :class="{ active: activeType === tab.value }"
        @click="activeType = tab.value"
      >
        {{ tab.label }}
      </text>
    </view>

    <view class="filter-row status-row">
      <text
        v-for="tab in statusTabs"
        :key="tab.value"
        class="status-chip"
        :class="{ active: activeStatus === tab.value }"
        @click="activeStatus = tab.value"
      >
        {{ tab.label }}
      </text>
    </view>

    <view v-if="filteredWorks.length" class="work-list">
      <zq-work-card v-for="work in filteredWorks" :key="work.id" :item="work">
        <template #actions>
          <view class="work-actions">
            <text @click="copyPrompt(work.prompt)">复制</text>
            <text @click="downloadWork(work.title)">下载</text>
            <text class="danger" @click="removeWork(work.id)">删除</text>
          </view>
        </template>
      </zq-work-card>
    </view>

    <view v-else class="empty zq-card">
      <wd-status-tip image="content" tip="暂无作品，先去创作一个内容吧" />
      <wd-button type="primary" custom-class="empty__button" @click="goCreate">去创作</wd-button>
    </view>
  </zq-page-shell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import ZqPageShell from '@/components/zq-page-shell.vue'
import ZqWorkCard from '@/components/zq-work-card.vue'
import { tabRoutes } from '@/constants/routes'
import { getWorks } from '@/services/miniapp'
import type { WorkItem } from '@/types/domain'
import { showMockToast, switchTab } from '@/utils/navigation'

const keyword = ref('')
const activeType = ref('all')
const activeStatus = ref('all')
const works = ref<WorkItem[]>([])

const typeTabs = [
  { value: 'all', label: '全部' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' },
  { value: 'ppt', label: 'PPT' },
]

const statusTabs = [
  { value: 'all', label: '全部状态' },
  { value: 'processing', label: '生成中' },
  { value: 'succeeded', label: '已完成' },
  { value: 'failed', label: '失败' },
]

const filteredWorks = computed(() => works.value.filter((work) => {
  const matchType = activeType.value === 'all' || work.type === activeType.value
  const matchStatus = activeStatus.value === 'all' || work.status === activeStatus.value
  const query = keyword.value.trim().toLowerCase()
  const matchKeyword = !query || `${work.title} ${work.model} ${work.prompt}`.toLowerCase().includes(query)
  return matchType && matchStatus && matchKeyword
}))

function copyPrompt(prompt: string) {
  uni.setClipboardData({ data: prompt })
}

function downloadWork(title: string) {
  showMockToast(`${title} 下载接口已预留`)
}

function removeWork(id: string) {
  works.value = works.value.filter(work => work.id !== id)
  showMockToast('已从本地 mock 列表移除')
}

function goCreate() {
  switchTab(tabRoutes.create)
}

onMounted(async () => {
  works.value = await getWorks()
})
</script>

<style scoped lang="scss">
.page-head {
  padding: 42rpx 4rpx 22rpx;
}

.page-head__title,
.page-head__copy {
  display: block;
}

.page-head__title {
  color: var(--color-text-primary);
  font-size: 46rpx;
  font-weight: 900;
}

.page-head__copy {
  margin-top: 10rpx;
  color: var(--color-text-secondary);
  font-size: 25rpx;
}

.filter-row {
  display: flex;
  gap: 14rpx;
  overflow-x: auto;
  margin-top: 22rpx;
  white-space: nowrap;
}

.status-row {
  margin-top: 16rpx;
}

.filter-chip,
.status-chip {
  display: inline-flex;
  align-items: center;
  min-height: 56rpx;
  padding: 0 22rpx;
  border: 1rpx solid var(--color-border);
  border-radius: 999rpx;
  background: #fff;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  font-weight: 700;
}

.filter-chip.active,
.status-chip.active {
  border-color: rgba(125, 141, 246, 0.42);
  background: rgba(125, 141, 246, 0.12);
  color: var(--color-primary-dark);
}

.work-list {
  display: grid;
  gap: 18rpx;
  margin-top: 28rpx;
}

.work-actions {
  display: flex;
  gap: 14rpx;
  color: var(--color-primary-dark);
  font-size: 22rpx;
  font-weight: 800;
}

.work-actions .danger {
  color: #dc2626;
}

.empty {
  display: grid;
  place-items: center;
  gap: 20rpx;
  margin-top: 32rpx;
  padding: 48rpx 24rpx;
}

:deep(.empty__button) {
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
}
</style>
