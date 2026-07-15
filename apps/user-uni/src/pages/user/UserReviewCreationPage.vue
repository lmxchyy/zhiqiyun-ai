<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserCreationPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">易共识</text><text class="mpb-subtitle">提问、验证并沉淀结论</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero light"><view class="mpb-hero-top"><view><text class="mpb-hero-label">开始共识验证</text><text class="mpb-hero-value">输入一个需要判断的问题</text></view><text class="mpb-hero-badge purple">知识智能体</text></view><text class="mpb-hero-copy">系统使用当前第一个可用智能体创建会话并运行，结果和消息会保存到后台。</text></view>
      <view class="mpb-card"><view class="mpb-field"><text class="mpb-label">验证问题</text><textarea v-model="question" class="mpb-textarea" maxlength="500" placeholder="例如：这个产品方案是否适合中小企业？" /></view><button class="mpb-button" :disabled="submitting || !question.trim() || !agents.length" @click="startReview">{{ submitting ? '验证中...' : agents.length ? '开始验证' : '请先创建智能体' }}</button><button v-if="!agents.length" class="mpb-button secondary" @click="openAgents">去创建智能体</button></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载共识记录...</text></view>
      <view v-else-if="conversations.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">历史验证</text><text class="mpb-card-copy">{{ conversations.length }} 条</text></view><button v-for="item in conversations" :key="rowString(item, 'id')" class="mpb-row-button" @click="openConversation(item)"><text class="mpb-row-icon green">识</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'title') || '未命名验证' }}</text><text class="mpb-row-meta">{{ formatDate(rowString(item, 'updatedAt', 'createdAt')) }}</text></view><text :class="['mpb-status', statusTone(rowString(item, 'status'))]">{{ statusText(rowString(item, 'status')) }}</text></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无共识记录</text><text class="mpb-empty-copy">提交第一个问题后，验证过程会显示在这里。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { backOrHome, formatDate, rowItems, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const submitting = ref(false);
const question = ref("");
const agents = ref<AnyRecord[]>([]);
const conversations = ref<AnyRecord[]>([]);
async function load() { loading.value = true; try { const [agentPayload, conversationPayload] = await Promise.all([api("/api/v1/knowledge-agents"), api("/api/v1/knowledge-conversations")]); agents.value = rowItems(agentPayload); conversations.value = rowItems(conversationPayload); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "共识记录加载失败", icon: "none" }); } finally { loading.value = false; } }
async function startReview() { const text = question.value.trim(); const agentId = rowString(agents.value[0], "id"); if (!text || !agentId || submitting.value) return; submitting.value = true; try { const conversation = await api<AnyRecord>("/api/v1/knowledge-conversations", { method: "POST", body: JSON.stringify({ agentId, title: text.slice(0, 60) }) }); const conversationId = rowString(conversation, "id"); if (!conversationId) throw new Error("会话创建后未返回编号"); await api(`/api/v1/knowledge-conversations/${encodeURIComponent(conversationId)}/runs`, { method: "POST", body: JSON.stringify({ question: text, topK: 5, threshold: 0.2, mode: "hybrid" }) }); question.value = ""; await load(); uni.navigateTo({ url: `/pages/user/UserReviewConversationPage?id=${encodeURIComponent(conversationId)}` }); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "共识验证失败", icon: "none" }); } finally { submitting.value = false; } }
function openAgents() { uni.navigateTo({ url: "/pages/user/UserAgentCreationPage" }); }
function openConversation(item: AnyRecord) { const id = rowString(item, "id"); if (id) uni.navigateTo({ url: `/pages/user/UserReviewConversationPage?id=${encodeURIComponent(id)}` }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
