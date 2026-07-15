<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><NativePageBack fallback="/pages/user/UserHomePage" /><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">AI Agent</text><text class="mpb-subtitle">创建并管理知识智能体</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-card"><view class="mpb-section-head"><text class="mpb-card-title">创建智能体</text><text class="mpb-card-copy">接入知识库运行接口</text></view><view class="mpb-field"><text class="mpb-label">智能体名称</text><input v-model="draft.name" class="mpb-input" maxlength="60" placeholder="例如：招商方案顾问" /></view><view class="mpb-field"><text class="mpb-label">角色说明</text><textarea v-model="draft.description" class="mpb-textarea" maxlength="200" placeholder="说明智能体负责解决的问题" /></view><button class="mpb-button" :disabled="creating || !draft.name.trim()" @click="createAgent">{{ creating ? '创建中...' : '创建智能体' }}</button></view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载智能体...</text></view>
      <view v-else-if="agents.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">我的智能体</text><text class="mpb-card-copy">{{ agents.length }} 个</text></view><button v-for="item in agents" :key="rowString(item, 'id')" class="mpb-row-button" @click="openAgent(item)"><text class="mpb-row-icon green">智</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'name') || '未命名智能体' }}</text><text class="mpb-row-meta">{{ rowString(item, 'description') || rowString(item, 'modelName') || '知识问答智能体' }}</text></view><text :class="['mpb-status', statusTone(rowString(item, 'status'))]">{{ statusText(rowString(item, 'status')) }}</text></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无智能体</text><text class="mpb-empty-copy">填写名称和角色说明，创建第一个知识智能体。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { rowItems, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import NativePageBack from "../../components/NativePageBack.vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const creating = ref(false);
const agents = ref<AnyRecord[]>([]);
const draft = reactive({ name: "", description: "" });
async function load() { loading.value = true; try { agents.value = rowItems(await api("/api/v1/knowledge-agents")); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "智能体加载失败", icon: "none" }); } finally { loading.value = false; } }
async function createAgent() { if (!draft.name.trim() || creating.value) return; creating.value = true; try { await api("/api/v1/knowledge-agents", { method: "POST", body: JSON.stringify({ name: draft.name.trim(), description: draft.description.trim(), modelName: "gpt-5.2-chat-latest", systemPrompt: draft.description.trim() || `你是${draft.name.trim()}，请提供清晰、可执行的回答。`, status: "ACTIVE", config: {} }) }); draft.name = ""; draft.description = ""; await load(); uni.showToast({ title: "智能体已创建", icon: "success" }); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "智能体创建失败", icon: "none" }); } finally { creating.value = false; } }
function openAgent(item: AnyRecord) { const id = rowString(item, "id"); if (id) uni.navigateTo({ url: `/pages/user/UserKnowledgeAgentDetailPage?id=${encodeURIComponent(id)}` }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css";</style>
