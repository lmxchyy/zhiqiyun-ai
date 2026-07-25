<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><NativePageBack fallback="/pages/user/UserHomePage" /><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">AI Agent</text><text class="mpb-subtitle">创建并管理知识智能体</text></view><text class="mpb-role">普通用户</text></view>
    <view v-if="guest" class="mpb-card mpb-guest"><text class="mpb-card-title">可先浏览，创建时再登录</text><text class="mpb-card-copy">登录仅在保存智能体时需要，不影响查看本页功能。</text><button class="mpb-guest-button" @click="requestLogin">登录 / 注册</button></view>
    <AiGeneratedContentNotice />
    <view class="mpb-stack">
      <view class="mpb-card">
        <view class="mpb-section-head"><text class="mpb-card-title">创建智能体</text><text class="mpb-card-copy">接入知识库运行接口</text></view>
        <view class="mpb-field"><text class="mpb-label">智能体名称</text><input v-model="draft.name" class="mpb-input" maxlength="60" placeholder="例如：招商方案顾问" /></view>
        <view class="mpb-field"><text class="mpb-label">角色说明</text><textarea v-model="draft.description" class="mpb-textarea" maxlength="200" placeholder="说明智能体负责解决的问题" /></view>
        <view class="mpb-field">
          <text class="mpb-label">对话模型</text>
          <picker mode="selector" :range="agentModels" range-key="label" :value="selectedAgentModelIndex" @change="selectAgentModel">
            <view class="mpb-input agent-model-picker"><text>{{ selectedAgentModelLabel }}</text><text class="agent-model-arrow">›</text></view>
          </picker>
          <text class="mpb-card-copy agent-model-help">DeepSeek V4 Flash 已通过上游连通性验证，适合知识问答与推理任务。</text>
        </view>
        <button class="mpb-button" :disabled="creating || !draft.name.trim()" @click="createAgent">{{ creating ? '创建中...' : '创建智能体' }}</button>
      </view>
      <view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载智能体...</text></view>
      <view v-else-if="agents.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">我的智能体</text><text class="mpb-card-copy">{{ agents.length }} 个</text></view><button v-for="item in agents" :key="rowString(item, 'id')" class="mpb-row-button" @click="openAgent(item)"><text class="mpb-row-icon green">智</text><view class="mpb-row-main"><text class="mpb-row-title">{{ rowString(item, 'name') || '未命名智能体' }}</text><text class="mpb-row-meta">{{ rowString(item, 'modelName') || '默认模型' }} · {{ rowString(item, 'description') || '知识问答智能体' }}</text></view><text :class="['mpb-status', statusTone(rowString(item, 'status'))]">{{ statusText(rowString(item, 'status')) }}</text></button></view>
      <view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无智能体</text><text class="mpb-empty-copy">填写名称和角色说明，创建第一个知识智能体。</text></view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { onShow } from "@dcloudio/uni-app";
import { api, getAuthToken } from "../../api/client";
import { rowItems, rowString, statusText, statusTone, type AnyRecord } from "../../utils/miniProgramBusiness";
import NativePageBack from "../../components/NativePageBack.vue";
import AiGeneratedContentNotice from "../../components/compliance/AiGeneratedContentNotice.vue";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";

const loading = ref(false);
const creating = ref(false);
const agents = ref<AnyRecord[]>([]);
const guest = ref(!getAuthToken());
const agentModels = [
  { label: "DeepSeek V4 Flash", value: "deepseek-v4-flash" },
  { label: "GPT-5.2 Chat", value: "gpt-5.2-chat-latest" },
];
const draft = reactive({ name: "", description: "", modelName: agentModels[0].value });
const selectedAgentModelIndex = computed(() => Math.max(0, agentModels.findIndex(item => item.value === draft.modelName)));
const selectedAgentModelLabel = computed(() => agentModels[selectedAgentModelIndex.value]?.label || agentModels[0].label);
function selectAgentModel(event: { detail?: { value?: string | number } }) { const index = Number(event.detail?.value ?? 0); draft.modelName = agentModels[index]?.value || agentModels[0].value; }
function requestLogin() { uni.navigateTo({ url: "/pages/WechatLoginPage?redirectPath=%2Fpages%2Fuser%2FUserAgentCreationPage", fail: () => uni.reLaunch({ url: "/pages/WechatLoginPage?redirectPath=%2Fpages%2Fuser%2FUserAgentCreationPage" }) }); }
function friendlyAgentError(error: unknown, fallback: string) { const message = error instanceof Error ? error.message : ""; if (/403|forbidden|暂无权限/i.test(message)) return "账号空间正在同步，请返回首页后重试"; if (/401|unauthorized|未登录/i.test(message)) return "请先登录后再继续"; return fallback; }
async function load() { guest.value = !getAuthToken(); if (guest.value) { agents.value = []; loading.value = false; return; } loading.value = true; try { agents.value = rowItems(await api("/api/v1/knowledge-agents")); } catch (error) { console.warn("[AI Agent 加载失败]", error); uni.showToast({ title: friendlyAgentError(error, "智能体加载失败，请稍后重试"), icon: "none" }); } finally { loading.value = false; } }
async function createAgent() { if (!draft.name.trim() || creating.value) return; if (!getAuthToken()) { requestLogin(); return; } creating.value = true; try { await api("/api/v1/knowledge-agents", { method: "POST", body: JSON.stringify({ name: draft.name.trim(), description: draft.description.trim(), modelName: draft.modelName, systemPrompt: draft.description.trim() || `你是${draft.name.trim()}，请提供清晰、可执行的回答。`, status: "ACTIVE", config: {} }) }); draft.name = ""; draft.description = ""; await load(); uni.showToast({ title: "智能体已创建", icon: "success" }); } catch (error) { console.warn("[AI Agent 创建失败]", error); uni.showToast({ title: friendlyAgentError(error, "智能体创建失败，请稍后重试"), icon: "none" }); } finally { creating.value = false; } }
function openAgent(item: AnyRecord) { const id = rowString(item, "id"); if (id) uni.navigateTo({ url: `/pages/user/UserKnowledgeAgentDetailPage?id=${encodeURIComponent(id)}` }); }
onShow(load);
</script>

<style>@import "../../styles/mini-program-business.css"; .mpb-guest{margin:0 16px 12px;padding:14px}.mpb-guest-button{margin-top:10px;border:0;border-radius:12px;background:#4a6bff;color:#fff;font-size:14px}.mpb-guest-button::after{display:none}.agent-model-picker{display:flex;align-items:center;justify-content:space-between;box-sizing:border-box}.agent-model-arrow{font-size:24px;color:#8090b5}.agent-model-help{display:block;margin-top:7px;line-height:1.5}</style>
