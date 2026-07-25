<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/agent/AgentOverviewPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">代理团队</text><text class="mpb-subtitle">团队层级与增长概览</text></view><text class="mpb-role agent">代理商</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">团队规模</text><text class="mpb-hero-value">{{ members.length }} 名代理</text></view><text class="mpb-hero-badge">增长中</text></view><text class="mpb-hero-copy">当前展示服务端返回的直属团队，成员状态和加入时间可追溯。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ activeCount }}</text><text class="mpb-hero-metric-label">正常代理</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ recentCount }}</text><text class="mpb-hero-metric-label">近 30 天新增</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ members.length - activeCount }}</text><text class="mpb-hero-metric-label">其他状态</text></view></view></view>
      <input v-model="keyword" class="mpb-search" placeholder="搜索姓名、邮箱或邀请码" />
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载团队...</text></view>
      <view v-else-if="filtered.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">团队成员</text><text class="mpb-card-copy">{{ filtered.length }} 人</text></view><button v-for="item in filtered" :key="rowString(item, 'id')" class="mpb-row-button" @click="open(item)"><text :class="['mpb-row-icon', statusTone(rowString(item, 'status')) === 'success' ? 'green' : 'orange']">代</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'name', 'email') || '未命名代理' }}</text><text class="mpb-row-meta">{{ rowString(item, 'email') }} · {{ rowString(item, 'inviteCode') || '无邀请码' }}</text></view><view class="mpb-row-side"><text class="mpb-amount">L{{ rowNumber(item, 'level') || 1 }}</text><text :class="['mpb-status', statusTone(rowString(item, 'status'))]">{{ statusText(rowString(item, 'status')) }}</text></view></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无团队成员</text><text class="mpb-empty-copy">具备下级代理开通权限后，可在桌面端创建下级代理。</text></view>
      <button class="mpb-button orange" @click="inviteTeam">邀请新成员</button>
      <text class="mpb-footer-note">仅展示真实直属代理，不推算虚构多级团队</text>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { businessSdk } from "../../api/client";
import { asRecord, backOrHome, listOf, rowNumber, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const loading = ref(false); const members = ref<AnyRecord[]>([]); const keyword = ref(""); const inviteLink = ref("");
const activeCount = computed(() => members.value.filter(item => rowString(item, "status").toUpperCase() === "ACTIVE").length);
const recentCount = computed(() => { const cutoff = Date.now() - 30 * 86400000; return members.value.filter(item => new Date(rowString(item, "createdAt")).getTime() >= cutoff).length; });
const filtered = computed(() => { const word = keyword.value.trim().toLowerCase(); return members.value.filter(item => !word || ["name", "email", "inviteCode"].some(key => rowString(item, key).toLowerCase().includes(word))); });
async function load() { loading.value = true; try { const center = await businessSdk.roleWorkbench.channelCenter(); members.value = listOf(center.children); inviteLink.value = rowString(center.agent, "inviteLink") || rowString(asRecord(center).promotion, "inviteLink", "landingURL"); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "团队加载失败", icon: "none" }); } finally { loading.value = false; } }
function open(item: AnyRecord) { uni.navigateTo({ url: `/pages/agent/AgentTeamMemberPage?id=${encodeURIComponent(rowString(item, 'id'))}` }); }
function inviteTeam() { if (inviteLink.value) uni.setClipboardData({ data: inviteLink.value, success: () => uni.showToast({ title: "邀请链接已复制", icon: "success" }) }); else uni.showModal({ title: "暂无邀请链接", content: "当前服务端尚未返回团队邀请链接，请先在推广中心检查邀请码配置。", showCancel: false }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
