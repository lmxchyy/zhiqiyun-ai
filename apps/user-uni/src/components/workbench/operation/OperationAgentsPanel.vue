<template>
  <view class="section-stack">
    <view class="section-card">
      <view class="section-header compact">
        <text class="section-title">代理商团队</text>
        <text class="soft-tag">{{ agents.length }} 人</text>
      </view>
      <view v-if="agents.length" class="list-stack">
        <view v-for="agent in agents" :key="rowKey(agent)" class="list-item" @click="emit('open-detail', agent)">
          <view>
            <text class="list-title">{{ customerName(agent) }}</text>
            <text class="list-meta">{{ rowString(agent, 'levelLabel') || rowString(agent, 'levelName') || '代理商' }}</text>
          </view>
          <text :class="['status-tag', statusTone(rowStatus(agent))]">{{ rowStatus(agent) }}</text>
        </view>
      </view>
      <text v-else class="empty-text">暂无代理商数据。</text>
    </view>
  </view>
</template>

<script setup lang="ts">
type AgentRow = Record<string, unknown>;

interface Props {
  agents: AgentRow[];
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'open-detail', agent: AgentRow): void;
}>();

function rowKey(row: unknown) {
  const value = row && typeof row === 'object' ? (row as Record<string, unknown>).id : '';
  return String(value || Math.random());
}

function customerName(row: unknown) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>).name || (row as Record<string, unknown>).nickname || '代理商') : '代理商';
}

function rowString(row: unknown, key: string) {
  return row && typeof row === 'object' ? String((row as Record<string, unknown>)[key] || '') : '';
}

function rowStatus(row: unknown) {
  return rowString(row, 'status') || '代理商';
}

function statusTone(status: string) {
  return /success|done|active/i.test(status) ? 'success' : 'pending';
}
</script>

<style scoped>
.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
