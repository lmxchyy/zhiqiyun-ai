<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserUsageDetailsPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">消耗详情</text><text class="mpb-subtitle">任务、模型与点数变化</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载消耗记录...</text></view>
      <template v-else-if="record">
        <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">本次消耗</text><text class="mpb-hero-value">{{ formatNumber(pointCost) }} 点</text></view><text class="mpb-hero-badge purple">{{ rowString(record, 'status') || '已记录' }}</text></view><text class="mpb-hero-copy">数据来自会员钱包与计费流水接口。</text></view>
        <view class="mpb-card mpb-list">
          <view class="mpb-row"><text class="mpb-row-icon">项</text><view class="mpb-row-main"><text class="mpb-row-title">计费项目</text><text class="mpb-row-meta">{{ rowString(record, 'metricCode', 'type', 'title') || 'AI 创作' }}</text></view></view>
          <view class="mpb-row"><text class="mpb-row-icon green">任</text><view class="mpb-row-main"><text class="mpb-row-title">任务编号</text><text class="mpb-row-meta">{{ rowString(record, 'taskId', 'bizId') || '-' }}</text></view></view>
          <view class="mpb-row"><text class="mpb-row-icon orange">前</text><view class="mpb-row-main"><text class="mpb-row-title">扣减前余额</text><text class="mpb-row-meta">账户计费快照</text></view><text class="mpb-amount">{{ formatNumber(rowNumber(record, 'balanceBefore')) }} 点</text></view>
          <view class="mpb-row"><text class="mpb-row-icon">后</text><view class="mpb-row-main"><text class="mpb-row-title">扣减后余额</text><text class="mpb-row-meta">{{ formatDate(rowString(record, 'occurredAt', 'createdAt')) }}</text></view><text class="mpb-amount">{{ formatNumber(rowNumber(record, 'balanceAfter')) }} 点</text></view>
        </view>
      </template>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">未找到消耗记录</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { asRecord, backOrHome, formatDate, formatNumber, rowNumber, rowString, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const id = ref("");
const loading = ref(false);
const record = ref<AnyRecord | null>(null);
const pointCost = computed(() => rowNumber(record.value, "pointCost", "points", "amount"));
async function load() {
  loading.value = true;
  try {
    const payload = asRecord(await api(`/api/v1/user/usage/${encodeURIComponent(id.value)}`));
    const item = asRecord(payload.item);
    record.value = rowString(item, "id", "eventId", "taskId") ? item : null;
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "消耗记录加载失败", icon: "none" });
  } finally {
    loading.value = false;
  }
}
onLoad(options => { id.value = String(options?.id || ""); void load(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
