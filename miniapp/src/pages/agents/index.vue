<template>
  <zq-page-shell>
    <view class="page-head">
      <text class="page-head__title">Agent</text>
      <text class="page-head__copy">按企业内容生产场景组织智能体入口。</text>
    </view>

    <view class="agent-banner zq-card">
      <view>
        <text class="agent-banner__title">企业知识协作中枢</text>
        <text class="agent-banner__copy">品牌、电商、销售、PPT 和知识库 Agent 可组合为企业工作流。</text>
      </view>
      <wd-button size="small" type="primary" custom-class="agent-banner__button" @click="showMockToast('Agent 编排入口已预留')">
        编排
      </wd-button>
    </view>

    <view class="agent-list">
      <zq-agent-card
        v-for="agent in agents"
        :key="agent.id"
        :title="agent.title"
        :description="agent.description"
        :tags="agent.tags"
        :tone="agent.tone"
      />
    </view>
  </zq-page-shell>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ZqAgentCard from '@/components/zq-agent-card.vue'
import ZqPageShell from '@/components/zq-page-shell.vue'
import { getAgents } from '@/services/miniapp'
import type { AgentEntry } from '@/types/domain'
import { showMockToast } from '@/utils/navigation'

const agents = ref<AgentEntry[]>([])

onMounted(async () => {
  agents.value = await getAgents()
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

.agent-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;
  padding: 28rpx;
  background:
    linear-gradient(135deg, rgba(125, 141, 246, 0.16), rgba(255, 119, 27, 0.08)),
    #fff;
}

.agent-banner__title,
.agent-banner__copy {
  display: block;
}

.agent-banner__title {
  color: var(--color-text-primary);
  font-size: 32rpx;
  font-weight: 900;
}

.agent-banner__copy {
  margin-top: 10rpx;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  line-height: 1.45;
}

:deep(.agent-banner__button) {
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
}

.agent-list {
  display: grid;
  gap: 18rpx;
  margin-top: 28rpx;
}
</style>
