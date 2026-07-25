<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentTeamPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">团队成员</text><text class="mpb-subtitle">成员资料、客户与业绩</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view v-if="member" class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">成员概览</text><text class="mpb-hero-value">{{ rowString(member, 'name', 'email') }}</text></view><text :class="['mpb-hero-badge', rowString(member, 'status').toUpperCase() === 'ACTIVE' ? 'success' : '']">{{ statusText(rowString(member, 'status')) }}</text></view><text class="mpb-hero-copy">L{{ rowNumber(member, 'level') || 1 }} 代理商 · 数据来自直属团队接口。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ rowNumber(member, 'customerCount', 'customers') }}</text><text class="mpb-hero-metric-label">绑定客户</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ rowNumber(member, 'orderCount', 'orders') }}</text><text class="mpb-hero-metric-label">订单</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ formatCurrency(rowNumber(member, 'commissionCents', 'commission')) }}</text><text class="mpb-hero-metric-label">累计分润</text></view></view></view>
      <view v-if="member" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">成员数据</text><text class="mpb-card-copy">真实字段</text></view><view class="mpb-row"><text class="mpb-row-icon green">邮</text><view class="mpb-row-main"><text class="mpb-row-title">邮箱</text><text class="mpb-row-meta">{{ rowString(member, 'email') || '-' }}</text></view></view><view class="mpb-row"><text class="mpb-row-icon">码</text><view class="mpb-row-main"><text class="mpb-row-title">邀请码</text><text class="mpb-row-meta">{{ rowString(member, 'inviteCode') || '-' }}</text></view></view><view class="mpb-row"><text class="mpb-row-icon orange">级</text><view class="mpb-row-main"><text class="mpb-row-title">代理等级</text><text class="mpb-row-meta">服务端返回的当前层级</text></view><text class="mpb-amount">L{{ rowNumber(member, 'level') || 1 }}</text></view><view class="mpb-row"><text class="mpb-row-icon">时</text><view class="mpb-row-main"><text class="mpb-row-title">加入时间</text><text class="mpb-row-meta">{{ formatDate(rowString(member, 'createdAt')) }}</text></view></view></view>
      <button v-if="member" class="mpb-button orange" @click="openCustomers">查看其客户</button>
      <view v-else-if="!loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">未找到团队成员</text><text class="mpb-empty-copy">当前接口只返回直属下级代理，不展示虚构的多级团队数据。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { asRecord, backOrHome, formatCurrency, formatDate, rowNumber, rowString, statusText, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const loading = ref(false); const member = ref<AnyRecord | null>(null);
async function load() { loading.value = true; try { const payload = asRecord(await api(`/api/v1/channel/children/${encodeURIComponent(id.value)}`)); const item = asRecord(payload.item); member.value = rowString(item, "id") ? item : null; } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "成员加载失败", icon: "none" }); } finally { loading.value = false; } }
function openCustomers() { uni.navigateTo({ url: "/pages/agent/AgentCustomersPage" }); }
onLoad(options => { id.value = String(options?.id || ""); void load(); });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
