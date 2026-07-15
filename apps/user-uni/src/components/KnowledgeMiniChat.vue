<template>
  <view class="km-shell">
    <view class="km-header">
      <button v-if="embedded" class="km-back" type="button" @click="$emit('close')">←</button>
      <view class="km-title"><text>知识库问答</text><text>答案可追溯，引用可核验</text></view>
      <button class="km-new" type="button" :disabled="!selectedAgentId" @click="createConversation">新对话</button>
    </view>

    <view v-if="loading" class="km-state"><text>正在同步知识库智能体…</text></view>
    <view v-else-if="error && !agents.length" class="km-state error"><text>{{ error }}</text><button @click="load">重新加载</button></view>
    <template v-else>
      <scroll-view class="km-agent-strip" scroll-x>
        <button v-for="agent in agents" :key="agent.id" :class="{ active: selectedAgentId === agent.id }" @click="selectAgent(agent.id)">
          <text>{{ agent.name.slice(0, 1) }}</text><view><text>{{ agent.name }}</text><text>{{ agent.description || '知识库智能体' }}</text></view>
        </button>
        <view v-if="!agents.length" class="km-no-agent"><text>暂无可用智能体</text><text>请先在 PC 端创建并绑定知识库</text></view>
      </scroll-view>

      <view class="km-body">
        <scroll-view class="km-history" scroll-y>
          <text class="km-section-label">历史记录</text>
          <button v-for="item in conversations" :key="item.id" :class="{ active: activeConversationId === item.id }" @click="openConversation(item.id)">
            <text>{{ item.title }}</text><text>{{ formatDate(item.updatedAt) }}</text>
          </button>
          <text v-if="!conversations.length" class="km-history-empty">暂无历史对话</text>
        </scroll-view>

        <view class="km-chat">
          <scroll-view class="km-messages" scroll-y :scroll-top="scrollTop">
            <view v-if="!messages.length" class="km-empty">
              <text class="km-empty-icon">KA</text><text class="km-empty-title">向企业知识库提问</text><text class="km-empty-copy">回答会展示引用文档、相似度和原文片段。</text>
              <view><button v-for="item in suggestions" :key="item" @click="draft = item">{{ item }}</button></view>
            </view>
            <view v-for="message in messages" :key="message.id" :class="['km-message', message.role]">
              <text class="km-avatar">{{ message.role === 'user' ? '我' : agentInitial }}</text>
              <view><text class="km-bubble">{{ message.content || (generating ? '正在检索知识库…' : '') }}</text>
                <view v-if="message.citations?.length" class="km-citations">
                  <button v-for="citation in message.citations" :key="citation.id" @click="activeCitation = citation">[{{ citation.order }}] {{ citation.documentName }}</button>
                </view>
              </view>
            </view>
          </scroll-view>

          <view v-if="error" class="km-inline-error"><text>{{ error }}</text></view>
          <view class="km-composer">
            <textarea v-model="draft" maxlength="1000" :disabled="generating" auto-height placeholder="输入问题，答案将基于已绑定知识库生成" />
            <view><text>{{ draft.length }}/1000</text><button v-if="generating" class="stop" @click="stopGeneration">停止</button><button v-else :disabled="!canSend" @click="send">发送</button></view>
          </view>
          <button v-if="lastQuestion && !generating" class="km-retry" type="button" @click="retryLast">重新回答上一个问题</button>
        </view>
      </view>
    </template>

    <view v-if="activeCitation" class="km-mask" @click.self="activeCitation = null">
      <view class="km-source-card">
        <view><view><text>引用 [{{ activeCitation.order }}]</text><text>{{ activeCitation.documentName }}</text></view><button @click="activeCitation = null">×</button></view>
        <text class="km-source-label">原文片段</text><text class="km-quote">{{ activeCitation.quote }}</text>
        <view class="km-source-meta"><text>相似度 {{ score(activeCitation.similarityScore) }}</text><text>Chunk {{ activeCitation.chunkId }}</text><text v-if="citationPage(activeCitation)">页码 {{ citationPage(activeCitation) }}</text></view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { miniKnowledgeAPI, startMiniKnowledgeRun, type KnowledgeRunHandle, type MiniKnowledgeAgent, type MiniKnowledgeCitation, type MiniKnowledgeConversation, type MiniKnowledgeMessage, type MiniKnowledgeRunResult, type MiniKnowledgeStreamEvent } from "../api/knowledge";

defineProps<{ embedded?: boolean }>();
defineEmits<{ close: [] }>();
type ChatMessage = MiniKnowledgeMessage & { citations?: MiniKnowledgeCitation[] };

const agents = ref<MiniKnowledgeAgent[]>([]);
const conversations = ref<MiniKnowledgeConversation[]>([]);
const messages = ref<ChatMessage[]>([]);
const selectedAgentId = ref("");
const activeConversationId = ref("");
const activeRunId = ref("");
const activeCitation = ref<MiniKnowledgeCitation | null>(null);
const draft = ref("");
const lastQuestion = ref("");
const loading = ref(false);
const generating = ref(false);
const error = ref("");
const scrollTop = ref(0);
let runHandle: KnowledgeRunHandle | null = null;
const suggestions = ["请总结核心产品能力", "售后处理流程是什么？", "有哪些必须遵守的制度？"];
const selectedAgent = computed(() => agents.value.find(item => item.id === selectedAgentId.value));
const agentInitial = computed(() => selectedAgent.value?.name.slice(0, 1) || "AI");
const canSend = computed(() => Boolean(selectedAgentId.value && draft.value.trim()));

onMounted(load);

async function load() {
  loading.value = true; error.value = "";
  try {
    agents.value = (await miniKnowledgeAPI.agents()).items || [];
    if (!selectedAgentId.value && agents.value.length) selectedAgentId.value = agents.value[0].id;
    await loadConversations();
  } catch (reason) { error.value = errorMessage(reason); }
  finally { loading.value = false; }
}
async function loadConversations() {
  conversations.value = selectedAgentId.value ? (await miniKnowledgeAPI.conversations(selectedAgentId.value)).items || [] : [];
  if (!activeConversationId.value && conversations.value.length) await openConversation(conversations.value[0].id);
}
async function selectAgent(id: string) { selectedAgentId.value = id; activeConversationId.value = ""; messages.value = []; await loadConversations(); }
async function openConversation(id: string) {
  activeConversationId.value = id; error.value = "";
  try { messages.value = ((await miniKnowledgeAPI.messages(id)).items || []).map(item => ({ ...item })); scrollBottom(); }
  catch (reason) { error.value = errorMessage(reason); }
}
async function createConversation() {
  if (!selectedAgentId.value) return;
  try {
    const item = await miniKnowledgeAPI.createConversation(selectedAgentId.value, "新对话");
    conversations.value.unshift(item); activeConversationId.value = item.id; messages.value = []; error.value = "";
  } catch (reason) { error.value = errorMessage(reason); }
}
async function ensureConversation(question: string) {
  if (activeConversationId.value) return activeConversationId.value;
  const item = await miniKnowledgeAPI.createConversation(selectedAgentId.value, question.slice(0, 24));
  conversations.value.unshift(item); activeConversationId.value = item.id; return item.id;
}
async function send() {
  const question = draft.value.trim(); if (!canSend.value || generating.value) return;
  draft.value = ""; lastQuestion.value = question; error.value = "";
  messages.value.push({ id: `local-user-${Date.now()}`, role: "user", content: question, createdAt: new Date().toISOString() });
  const assistant: ChatMessage = { id: `local-assistant-${Date.now()}`, role: "assistant", content: "", createdAt: new Date().toISOString(), citations: [] };
  messages.value.push(assistant); generating.value = true; activeRunId.value = ""; scrollBottom();
  try {
    const conversationId = await ensureConversation(question);
    runHandle = startMiniKnowledgeRun(conversationId, question, (name, value) => {
      if (name === "run.started") activeRunId.value = (value as MiniKnowledgeStreamEvent).ragRunId || "";
      if (name === "answer.delta") assistant.content += String((value as MiniKnowledgeStreamEvent).data?.delta || "");
      if (name === "result") applyResult(assistant, value as MiniKnowledgeRunResult);
      scrollBottom();
    });
    const result = await runHandle.promise;
    applyResult(assistant, result);
    await loadConversations();
  } catch (reason) {
    if (!String(errorMessage(reason)).toLowerCase().includes("abort")) error.value = errorMessage(reason);
    if (!assistant.content) assistant.content = error.value ? "本次回答生成失败，请稍后重试。" : "回答已停止。";
  } finally { generating.value = false; runHandle = null; scrollBottom(); }
}
function applyResult(message: ChatMessage, result: MiniKnowledgeRunResult) { activeRunId.value = result.run.id; message.id = result.message.id; message.content = result.message.content; message.citations = result.citations || []; }
async function stopGeneration() { runHandle?.abort(); if (activeRunId.value) { try { await miniKnowledgeAPI.cancel(activeRunId.value); } catch (reason) { uni.showToast({ title: errorMessage(reason), icon: "none" }); } } generating.value = false; }
function retryLast() { if (!lastQuestion.value) return; draft.value = lastQuestion.value; void send(); }
function citationPage(citation: MiniKnowledgeCitation) { const locator = citation.locator || {}; return locator.page || locator.pageStart || locator.pageNumber || ""; }
function score(value?: number) { return typeof value === "number" ? `${(value * 100).toFixed(1)}%` : "—"; }
function formatDate(value: string) { if (!value) return ""; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : `${date.getMonth() + 1}/${date.getDate()}`; }
function scrollBottom() { scrollTop.value += 100000; }
function errorMessage(reason: unknown) { return reason instanceof Error ? reason.message : "知识问答加载失败"; }
</script>

<style scoped>
.km-shell{min-height:100%;background:#f5f6fa;color:#202939;display:flex;flex-direction:column}.km-header{height:64px;padding:0 16px;display:flex;align-items:center;gap:12px;background:#fff;border-bottom:1px solid #e8ebf1}.km-back,.km-new{border:0;border-radius:10px;background:#efeffb;color:#5751c8;padding:8px 11px}.km-title{flex:1;display:flex;flex-direction:column}.km-title text:first-child{font-size:18px;font-weight: 600}.km-title text:last-child{font-size:11px;color:#7b8596;margin-top:3px}.km-agent-strip{white-space:nowrap;background:#fff;padding:10px 12px;box-sizing:border-box;border-bottom:1px solid #e8ebf1}.km-agent-strip button{display:inline-flex;vertical-align:top;align-items:center;gap:8px;width:210px;text-align:left;margin-right:8px;padding:9px;border:1px solid #e4e7ee;border-radius:12px;background:#fff}.km-agent-strip button.active{border-color:#8179ea;background:#f5f4ff}.km-agent-strip button>text{width:34px;height:34px;line-height:34px;text-align:center;border-radius:10px;background:#6962d8;color:#fff;font-weight: 600}.km-agent-strip button view{display:flex;flex-direction:column;min-width:0}.km-agent-strip button view text:first-child{font-weight:700;overflow:hidden;text-overflow:ellipsis}.km-agent-strip button view text:last-child{font-size:10px;color:#7b8596;margin-top:3px;overflow:hidden;text-overflow:ellipsis}.km-no-agent{padding:12px;display:flex;flex-direction:column;color:#7b8596}.km-body{display:grid;grid-template-columns:180px minmax(0,1fr);flex:1;min-height:0}.km-history{background:#fff;border-right:1px solid #e8ebf1;padding:12px;box-sizing:border-box;height:calc(100vh - 130px)}.km-section-label{display:block;color:#8a94a6;font-size:10px;font-weight: 600;margin:5px 4px 10px}.km-history button{width:100%;display:flex;flex-direction:column;text-align:left;border:0;background:transparent;padding:10px;border-radius:9px;margin-bottom:4px}.km-history button.active{background:#f0effd;color:#5851ca}.km-history button text:first-child{font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.km-history button text:last-child{font-size: 10px;color:#8b95a5;margin-top:4px}.km-history-empty{font-size:11px;color:#9aa3b2;padding:10px}.km-chat{display:flex;flex-direction:column;min-width:0;height:calc(100vh - 130px)}.km-messages{flex:1;height:0;padding:16px;box-sizing:border-box}.km-empty{text-align:center;margin:54px auto;max-width:460px;display:flex;flex-direction:column;align-items:center}.km-empty-icon{width:54px;height:54px;line-height:54px;border-radius:18px;background:#eceafd;color:#625bd3;font-weight: 700}.km-empty-title{font-size:18px;font-weight: 600;margin-top:13px}.km-empty-copy{color:#7b8596;font-size:12px;margin-top:8px}.km-empty view{display:flex;flex-wrap:wrap;justify-content:center;gap:7px;margin-top:15px}.km-empty button{font-size:10px;border:1px solid #e2e5ec;background:#fff;border-radius:16px;padding:7px 10px}.km-message{display:flex;gap:8px;margin-bottom:16px}.km-message.user{flex-direction:row-reverse}.km-avatar{width:30px;height:30px;line-height:30px;text-align:center;flex:0 0 30px;border-radius:9px;background:#ebeafd;color:#5d56cd;font-size:11px;font-weight: 600}.km-message.user .km-avatar{background:#e9edf2;color:#606b7b}.km-message>view{max-width:82%}.km-bubble{display:block;white-space:pre-wrap;background:#fff;border:1px solid #e7e9ef;border-radius:4px 13px 13px 13px;padding:11px 13px;line-height:1.7;font-size:13px}.km-message.user .km-bubble{background:#6861d9;color:#fff;border-color:#6861d9;border-radius:13px 4px 13px 13px}.km-citations{display:flex;flex-wrap:wrap;gap:5px;margin-top:6px}.km-citations button{border:1px solid #dcd9fa;background:#f8f7ff;color:#5d57c7;border-radius:7px;padding:5px 7px;font-size: 10px}.km-composer{background:#fff;border-top:1px solid #e6e9ef;padding:10px 12px}.km-composer textarea{width:100%;min-height:44px;max-height:110px;background:#f7f8fa;border:1px solid #e3e6ec;border-radius:11px;padding:9px;box-sizing:border-box;font-size:13px}.km-composer>view{display:flex;justify-content:space-between;align-items:center;margin-top:7px}.km-composer>view>text{font-size: 10px;color:#8b95a5}.km-composer button{border:0;background:#625bd3;color:#fff;border-radius:9px;padding:7px 14px}.km-composer button.stop{background:#dd5555}.km-composer button[disabled]{opacity:.45}.km-retry{border:0;background:#fff;color:#5d57c8;font-size:11px;padding:8px}.km-inline-error{background:#fff1f1;color:#ad4545;padding:7px 12px;font-size:10px}.km-state{padding:80px 20px;text-align:center;color:#788296}.km-state.error button{display:block;margin:12px auto;border:0;background:#615bd3;color:#fff;border-radius:8px;padding:7px 12px}.km-mask{position:fixed;inset:0;z-index:99;background:#11182770;display:flex;align-items:flex-end}.km-source-card{width:100%;max-height:76vh;background:#fff;border-radius:22px 22px 0 0;padding:20px;box-sizing:border-box}.km-source-card>view:first-child{display:flex;justify-content:space-between}.km-source-card>view:first-child view{display:flex;flex-direction:column}.km-source-card>view:first-child view text:first-child{font-size:10px;color:#625bd3}.km-source-card>view:first-child view text:last-child{font-size:16px;font-weight: 600;margin-top:4px}.km-source-card>view:first-child button{border:0;background:#f0f2f5;border-radius:9px;width:34px;height:34px}.km-source-label{display:block;color:#7f8999;font-size:10px;margin-top:18px}.km-quote{display:block;background:#f7f6ff;border-left:3px solid #655ee0;padding:13px;margin-top:8px;line-height:1.7;white-space:pre-wrap}.km-source-meta{display:flex;flex-direction:column;gap:6px;margin-top:12px;color:#7d8797;font-size:10px}@media(max-width:700px){.km-body{grid-template-columns:1fr}.km-history{height:auto;max-height:100px;white-space:nowrap;border-right:0;border-bottom:1px solid #e8ebf1}.km-history button{display:inline-flex;width:150px;vertical-align:top;margin-right:5px}.km-chat{height:calc(100vh - 230px)}.km-header{padding-top:env(safe-area-inset-top);height:calc(58px + env(safe-area-inset-top))}.km-messages{padding:12px}.km-message>view{max-width:86%}}
</style>
