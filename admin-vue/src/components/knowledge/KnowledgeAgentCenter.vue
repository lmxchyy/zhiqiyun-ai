<template>
  <section class="ka-shell" aria-label="知识库智能体工作台">
    <header class="ka-topbar">
      <button class="ka-back" type="button" @click="$emit('close')">← 返回智能体中心</button>
      <div>
        <span>Knowledge Agent</span>
        <h2>知识库智能体</h2>
        <p>上传企业资料，绑定智能体，并在回答中核验原文引用。</p>
      </div>
      <div class="ka-agent-picker">
        <label for="knowledge-agent-select">当前智能体</label>
        <select id="knowledge-agent-select" v-model="selectedAgentId" :disabled="loadingAgents" @change="handleAgentChange">
          <option value="">选择智能体</option>
          <option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option>
        </select>
        <button type="button" @click="agentDialogOpen = true">+ 新建</button>
      </div>
    </header>

    <div class="ka-layout">
      <aside class="ka-library-rail">
        <header>
          <div><span>知识空间</span><strong>知识库</strong></div>
          <button type="button" aria-label="创建知识库" @click="baseDialogOpen = true">+</button>
        </header>
        <label class="ka-search">
          <span aria-hidden="true">⌕</span>
          <input v-model.trim="baseKeyword" placeholder="搜索知识库" />
        </label>
        <div v-if="loadingBases" class="ka-rail-state">正在加载知识库…</div>
        <button
          v-for="base in filteredBases"
          v-else
          :key="base.id"
          type="button"
          :class="['ka-base-item', { active: selectedBaseId === base.id }]"
          @click="selectBase(base.id)"
        >
          <span>{{ base.knowledgeType === 'PERSONAL' ? '个' : '企' }}</span>
          <div><strong>{{ base.name }}</strong><small>{{ base.documentCount }} 文档 · {{ base.chunkCount }} 切片</small></div>
          <em :class="base.status.toLowerCase()">{{ base.status === 'ACTIVE' ? '可用' : base.status }}</em>
        </button>
        <div v-if="!loadingBases && !filteredBases.length" class="ka-empty-rail">
          <b>还没有知识库</b>
          <span>创建后即可上传资料并绑定智能体。</span>
          <button type="button" @click="baseDialogOpen = true">创建知识库</button>
        </div>
      </aside>

      <main class="ka-main">
        <nav class="ka-tabs" aria-label="知识库智能体功能">
          <button v-for="item in tabs" :key="item.id" type="button" :class="{ active: activeTab === item.id }" @click="activeTab = item.id">
            {{ item.label }}<span v-if="item.id === 'documents' && documents.length">{{ documents.length }}</span>
          </button>
        </nav>

        <section v-if="activeTab === 'chat'" class="ka-chat-panel">
          <div class="ka-chat-head">
            <div>
              <span :class="['ka-agent-avatar', { empty: !selectedAgent }]">{{ selectedAgent?.name?.slice(0, 1) || 'AI' }}</span>
              <div><strong>{{ selectedAgent?.name || '请选择或创建智能体' }}</strong><small>{{ boundBaseSummary }}</small></div>
            </div>
            <button type="button" @click="activeTab = 'settings'">知识库配置</button>
          </div>

          <div ref="messageScroller" class="ka-messages" aria-live="polite">
            <div v-if="!messages.length" class="ka-chat-empty">
              <span>KA</span>
              <h3>从可信资料中找到答案</h3>
              <p>当前回答会经过 Query Rewrite、Hybrid Search 与引用生成。每条引用都保留文档、Chunk 和原文定位。</p>
              <div>
                <button v-for="question in quickQuestions" :key="question" type="button" @click="draft = question">{{ question }}</button>
              </div>
            </div>
            <article v-for="message in messages" :key="message.id" :class="['ka-message', message.role]">
              <span>{{ message.role === 'user' ? '我' : selectedAgent?.name?.slice(0, 1) || 'AI' }}</span>
              <div>
                <p>{{ message.content || (generating ? '正在检索知识库…' : '') }}</p>
                <div v-if="message.role === 'assistant' && message.citations?.length" class="ka-inline-citations">
                  <button v-for="citation in message.citations" :key="citation.id" type="button" @click="openCitation(citation)">
                    [{{ citation.order }}] {{ citation.documentName }}
                  </button>
                </div>
              </div>
            </article>
          </div>

          <footer class="ka-composer">
            <textarea v-model="draft" rows="3" :disabled="generating" placeholder="向绑定的知识库提问，Enter 发送，Shift + Enter 换行" @keydown.enter.exact.prevent="sendMessage" />
            <div>
              <span>{{ selectedAgent ? `已选择 ${selectedAgent.name}` : '尚未选择智能体' }}</span>
              <button v-if="generating" class="stop" type="button" @click="stopGeneration">■ 停止生成</button>
              <button v-else type="button" :disabled="!canSend" @click="sendMessage">发送 ↑</button>
            </div>
          </footer>
        </section>

        <section v-else-if="activeTab === 'documents'" class="ka-documents-panel">
          <header>
            <div><span>Document Pipeline</span><h3>{{ selectedBase?.name || '请选择知识库' }}</h3><p>文档上传后自动解析、切片、Embedding 并写入向量索引。</p></div>
            <label :class="['ka-upload', { disabled: !selectedBaseId || uploading }]">
              <input type="file" accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.md,.markdown,.html,.htm,.csv" :disabled="!selectedBaseId || uploading" @change="uploadDocument" />
              {{ uploading ? '处理中…' : '+ 上传文档' }}
            </label>
          </header>
          <div class="ka-pipeline-note"><b>当前可用解析器</b><span>PDF、Word、Excel、PPT、Markdown、TXT、HTML、CSV</span><em>图片 PDF 可由后台 OCR Provider 接管</em></div>
          <div v-if="loadingDocuments" class="ka-panel-state">正在读取文档…</div>
          <div v-else-if="!documents.length" class="ka-panel-empty"><span>DOC</span><h3>暂无文档</h3><p>上传企业资料后即可开始解析、切片与知识检索。</p></div>
          <div v-else class="ka-document-table">
            <div class="head"><span>文档名称</span><span>类型</span><span>状态</span><span>更新时间</span><span>操作</span></div>
            <div v-for="document in documents" :key="document.id" class="row">
              <span><b>{{ document.name }}</b><small>{{ document.id }}</small></span>
              <span>{{ document.documentType }}</span>
              <span><em :class="document.status.toLowerCase()">{{ document.status }}</em></span>
              <span>{{ formatDate(document.updatedAt) }}</span>
              <span><button class="ka-danger-link" type="button" :disabled="deletingDocumentId === document.id" @click="removeDocument(document)">{{ deletingDocumentId === document.id ? '删除中…' : '删除' }}</button></span>
            </div>
          </div>
        </section>

        <section v-else class="ka-settings-panel">
          <header><span>Agent × Knowledge</span><h3>知识库绑定</h3><p>一个智能体可绑定多个知识库，并在运行时保存绑定快照。</p></header>
          <div v-if="!selectedAgent" class="ka-panel-empty"><span>AI</span><h3>请先选择智能体</h3><p>新建智能体后即可配置知识优先级与启停状态。</p></div>
          <template v-else>
            <article v-if="selectedBase" class="ka-runtime-card">
              <header><div><span>Runtime Profile</span><strong>{{ selectedBase.name }}</strong></div><small>配置修改对后续解析和检索生效</small></header>
              <div>
                <label><span>解析 / Embedding</span><select v-model="runtimeForm.ingestionProfileId"><option v-for="profile in ingestionProfiles" :key="profile.id" :value="profile.id">{{ profile.name }} · {{ profile.id }}</option></select></label>
                <label><span>检索 / Rerank</span><select v-model="runtimeForm.retrievalProfileId"><option v-for="profile in retrievalProfiles" :key="profile.id" :value="profile.id">{{ profile.name }} · {{ profile.id }}</option></select></label>
                <button type="button" :disabled="savingRuntime" @click="saveRuntimeProfiles">{{ savingRuntime ? '保存中…' : '应用配置' }}</button>
              </div>
            </article>
            <article class="ka-agent-summary">
              <span class="ka-agent-avatar">{{ selectedAgent.name.slice(0, 1) }}</span>
              <div><strong>{{ selectedAgent.name }}</strong><small>{{ selectedAgent.description || '企业知识库问答智能体' }}</small></div>
              <em>{{ selectedAgent.status }}</em>
            </article>
            <div class="ka-binding-list">
              <label v-for="base in bases" :key="base.id" :class="{ selected: boundBaseIds.includes(base.id) }">
                <input v-model="boundBaseIds" type="checkbox" :value="base.id" />
                <span>{{ base.name.slice(0, 1) }}</span>
                <div><strong>{{ base.name }}</strong><small>{{ base.documentCount }} 文档 · {{ base.chunkCount }} 切片</small></div>
                <em>{{ boundBaseIds.includes(base.id) ? '已启用' : '未绑定' }}</em>
                <div v-if="boundBaseIds.includes(base.id)" class="ka-binding-controls" @click.stop>
                  <div><span>优先级</span><input v-model.number="bindingSetting(base.id).priority" type="number" min="1" max="999" /></div>
                  <div><span>权重</span><input v-model.number="bindingSetting(base.id).weight" type="number" min="0.1" max="10" step="0.1" /></div>
                  <div><span>检索配置</span><select v-model="bindingSetting(base.id).retrievalProfileId"><option value="">跟随知识库</option><option v-for="profile in retrievalProfiles" :key="profile.id" :value="profile.id">{{ profile.name }}</option></select></div>
                </div>
              </label>
            </div>
            <footer><span>已选择 {{ boundBaseIds.length }} 个知识库</span><button type="button" :disabled="savingBindings" @click="saveBindings">{{ savingBindings ? '保存中…' : '保存配置' }}</button></footer>
          </template>
        </section>
      </main>
    </div>

    <div v-if="citationOpen" class="ka-citation-mask" role="presentation" @click.self="citationOpen = null">
      <aside class="ka-citation-drawer" role="dialog" aria-modal="true" aria-label="查看引用原文">
        <header><div><span>引用 [{{ citationOpen.order }}]</span><strong>{{ citationOpen.documentName }}</strong></div><button type="button" aria-label="关闭" @click="citationOpen = null">×</button></header>
        <section><span>原文片段</span><blockquote>{{ citationOpen.quote }}</blockquote></section>
        <dl><div><dt>相似度</dt><dd>{{ formatScore(citationOpen.similarityScore) }}</dd></div><div><dt>Chunk ID</dt><dd>{{ citationOpen.chunkId }}</dd></div><div><dt>文档 ID</dt><dd>{{ citationOpen.documentId }}</dd></div><div v-if="citationPage(citationOpen)"><dt>页码</dt><dd>{{ citationPage(citationOpen) }}</dd></div></dl>
      </aside>
    </div>

    <el-dialog v-model="baseDialogOpen" title="创建知识库" width="520px" append-to-body>
      <div class="ka-dialog-form">
        <label><span>知识库名称</span><el-input v-model="baseForm.name" maxlength="150" placeholder="例如：产品与售后知识库" /></label>
        <label><span>描述</span><el-input v-model="baseForm.description" type="textarea" :rows="3" placeholder="说明资料范围和适用团队" /></label>
        <label><span>知识库类型</span><el-select v-model="baseForm.knowledgeType"><el-option label="个人知识库" value="PERSONAL" /><el-option label="智能体知识库" value="AGENT" /></el-select></label>
        <label><span>分类</span><el-input v-model="baseForm.categoryName" placeholder="例如：产品文档（不存在时自动创建）" /></label>
        <label><span>标签</span><el-input v-model="baseForm.tags" placeholder="多个标签用逗号分隔" /></label>
        <label><span>可见性</span><el-select v-model="baseForm.visibility"><el-option label="仅自己" value="PRIVATE" /><el-option label="租户成员" value="TENANT" /><el-option label="组织成员" value="ORGANIZATION" /><el-option label="共享" value="SHARED" /></el-select></label>
        <label><span>Logo 对象键</span><el-input v-model="baseForm.logoObjectKey" placeholder="可选，例如 knowledge-logos/product.png" /></label>
      </div>
      <template #footer><el-button @click="baseDialogOpen = false">取消</el-button><el-button type="primary" :loading="creatingBase" @click="createBase">创建</el-button></template>
    </el-dialog>

    <el-dialog v-model="agentDialogOpen" title="创建知识库智能体" width="560px" append-to-body>
      <div class="ka-dialog-form">
        <label><span>智能体名称</span><el-input v-model="agentForm.name" maxlength="120" placeholder="例如：产品知识顾问" /></label>
        <label><span>描述</span><el-input v-model="agentForm.description" placeholder="说明智能体负责的业务范围" /></label>
        <label><span>模型（可选）</span><el-input v-model="agentForm.modelName" placeholder="留空则使用后台默认对话模型" /></label>
        <label><span>系统提示词</span><el-input v-model="agentForm.systemPrompt" type="textarea" :rows="4" placeholder="约束回答语气、边界和格式" /></label>
      </div>
      <template #footer><el-button @click="agentDialogOpen = false">取消</el-button><el-button type="primary" :loading="creatingAgent" @click="createAgent">创建并配置</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { ElMessageBox } from "element-plus/es/components/message-box/index";
import {
  knowledgeAPI,
  streamKnowledgeRun,
  type KnowledgeAgent,
  type KnowledgeBase,
  type KnowledgeCategory,
  type KnowledgeCitation,
  type KnowledgeDocument,
  type KnowledgeProfile,
  type KnowledgeTag,
  type KnowledgeRunResult,
  type KnowledgeStreamEvent
} from "../../api/knowledge";

defineEmits<{ close: [] }>();

type TabId = "chat" | "documents" | "settings";
type ChatMessage = { id: string; role: "user" | "assistant"; content: string; citations?: KnowledgeCitation[] };
type BindingSetting = { priority: number; weight: number; retrievalProfileId: string };

const tabs: Array<{ id: TabId; label: string }> = [
  { id: "chat", label: "知识问答" },
  { id: "documents", label: "文档管理" },
  { id: "settings", label: "智能体配置" }
];
const quickQuestions = ["知识库里有哪些核心制度？", "请归纳产品的主要卖点", "售后问题应该如何处理？"];
const activeTab = ref<TabId>("chat");
const bases = ref<KnowledgeBase[]>([]);
const agents = ref<KnowledgeAgent[]>([]);
const documents = ref<KnowledgeDocument[]>([]);
const deletingDocumentId = ref("");
const ingestionProfiles = ref<KnowledgeProfile[]>([]);
const retrievalProfiles = ref<KnowledgeProfile[]>([]);
const knowledgeTags = ref<KnowledgeTag[]>([]);
const knowledgeCategories = ref<KnowledgeCategory[]>([]);
const runtimeForm = ref({ ingestionProfileId: "ingestion_default", retrievalProfileId: "retrieval_default" });
const selectedBaseId = ref("");
const selectedAgentId = ref("");
const boundBaseIds = ref<string[]>([]);
const bindingSettings = ref<Record<string, BindingSetting>>({});
const baseKeyword = ref("");
const draft = ref("");
const messages = ref<ChatMessage[]>([]);
const activeConversationId = ref("");
const activeRunId = ref("");
const citationOpen = ref<KnowledgeCitation | null>(null);
const messageScroller = ref<HTMLElement | null>(null);
const loadingBases = ref(false);
const loadingAgents = ref(false);
const loadingDocuments = ref(false);
const uploading = ref(false);
const generating = ref(false);
const savingBindings = ref(false);
const savingRuntime = ref(false);
const baseDialogOpen = ref(false);
const agentDialogOpen = ref(false);
const creatingBase = ref(false);
const creatingAgent = ref(false);
const abortController = ref<AbortController | null>(null);
const baseForm = ref({ name: "", description: "", knowledgeType: "PERSONAL" as KnowledgeBase["knowledgeType"], categoryName: "", tags: "", visibility: "PRIVATE", logoObjectKey: "" });
const agentForm = ref({ name: "", description: "", modelName: "", systemPrompt: "" });

const selectedBase = computed(() => bases.value.find((item) => item.id === selectedBaseId.value));
const selectedAgent = computed(() => agents.value.find((item) => item.id === selectedAgentId.value));
const filteredBases = computed(() => {
  const keyword = baseKeyword.value.toLowerCase();
  return keyword ? bases.value.filter((item) => `${item.name} ${item.description}`.toLowerCase().includes(keyword)) : bases.value;
});
const boundBaseSummary = computed(() => {
  if (!selectedAgent.value) return "选择智能体后开始问答";
  if (!boundBaseIds.value.length) return "尚未绑定知识库";
  const names = bases.value.filter((item) => boundBaseIds.value.includes(item.id)).map((item) => item.name);
  return `已绑定 ${names.join("、")}`;
});
const canSend = computed(() => Boolean(selectedAgentId.value && boundBaseIds.value.length && draft.value.trim()));

onMounted(async () => {
  await Promise.all([loadBases(), loadAgents(), loadProfiles(), loadCatalogs()]);
});

watch(selectedBase, (base) => {
  runtimeForm.value = {
    ingestionProfileId: base?.ingestionProfileId || "ingestion_default",
    retrievalProfileId: base?.retrievalProfileId || "retrieval_default"
  };
}, { immediate: true });

watch(activeTab, (value) => {
  if (value === "documents" && selectedBaseId.value) void loadDocuments();
});

async function loadBases() {
  loadingBases.value = true;
  try {
    bases.value = (await knowledgeAPI.listBases()).items || [];
    if (!selectedBaseId.value && bases.value.length) selectedBaseId.value = bases.value[0].id;
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    loadingBases.value = false;
  }
}

async function loadAgents() {
  loadingAgents.value = true;
  try {
    agents.value = (await knowledgeAPI.listAgents()).items || [];
    if (!selectedAgentId.value && agents.value.length) {
      selectedAgentId.value = agents.value[0].id;
      await loadAgentBindings();
    }
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    loadingAgents.value = false;
  }
}

async function loadProfiles() {
  try {
    const [ingestion, retrieval] = await Promise.all([knowledgeAPI.listProfiles("ingestion-profiles"), knowledgeAPI.listProfiles("retrieval-profiles")]);
    ingestionProfiles.value = ingestion.items || [];
    retrievalProfiles.value = retrieval.items || [];
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

async function loadCatalogs() {
  try {
    const [tags, categories] = await Promise.all([knowledgeAPI.listTags(), knowledgeAPI.listCategories()]);
    knowledgeTags.value = tags.items || [];
    knowledgeCategories.value = categories.items || [];
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

async function saveRuntimeProfiles() {
  if (!selectedBase.value) return;
  savingRuntime.value = true;
  try {
    const updated = await knowledgeAPI.updateBase(selectedBase.value.id, {
      ingestionProfileId: runtimeForm.value.ingestionProfileId,
      retrievalProfileId: runtimeForm.value.retrievalProfileId,
      expectedVersion: selectedBase.value.version
    });
    const index = bases.value.findIndex((item) => item.id === updated.id);
    if (index >= 0) bases.value[index] = updated;
    ElMessage.success("知识库运行配置已更新");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    savingRuntime.value = false;
  }
}

async function loadAgentBindings() {
  boundBaseIds.value = [];
  activeConversationId.value = "";
  messages.value = [];
  if (!selectedAgentId.value) return;
  try {
    const detail = await knowledgeAPI.getAgent(selectedAgentId.value);
    boundBaseIds.value = detail.knowledgeBindings.filter((item) => item.enabled).map((item) => item.knowledgeBaseId);
    bindingSettings.value = Object.fromEntries(detail.knowledgeBindings.map((item) => [item.knowledgeBaseId, { priority: item.priority || 100, weight: item.weight || 1, retrievalProfileId: item.retrievalProfileId || "" }]));
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

function bindingSetting(knowledgeBaseId: string): BindingSetting {
  if (!bindingSettings.value[knowledgeBaseId]) bindingSettings.value[knowledgeBaseId] = { priority: 100, weight: 1, retrievalProfileId: "" };
  return bindingSettings.value[knowledgeBaseId];
}

function bindingPayload() {
  return boundBaseIds.value.map((knowledgeBaseId) => ({ knowledgeBaseId, ...bindingSetting(knowledgeBaseId), enabled: true }));
}

function handleAgentChange() {
  void loadAgentBindings();
}

async function selectBase(id: string) {
  selectedBaseId.value = id;
  if (activeTab.value === "documents") await loadDocuments();
}

async function loadDocuments() {
  if (!selectedBaseId.value) {
    documents.value = [];
    return;
  }
  loadingDocuments.value = true;
  try {
    documents.value = (await knowledgeAPI.listDocuments(selectedBaseId.value)).items || [];
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    loadingDocuments.value = false;
  }
}

async function removeDocument(document: KnowledgeDocument) {
  if (!selectedBaseId.value || deletingDocumentId.value) return;
  try {
    await ElMessageBox.confirm(`确定删除文档「${document.name}」及其全部 Chunk 和向量索引吗？`, "删除文档", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消"
    });
    deletingDocumentId.value = document.id;
    await knowledgeAPI.deleteDocument(document.id);
    await Promise.all([loadDocuments(), loadBases()]);
    ElMessage.success("文档已删除");
  } catch (error) {
    if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error));
  } finally {
    deletingDocumentId.value = "";
  }
}

async function createBase() {
  if (!baseForm.value.name.trim()) {
    ElMessage.warning("请输入知识库名称");
    return;
  }
  creatingBase.value = true;
  try {
    let categoryId = "";
    const categoryName = baseForm.value.categoryName.trim();
    if (categoryName) {
      let category = knowledgeCategories.value.find((item) => item.name.toLowerCase() === categoryName.toLowerCase());
      if (!category) {
        category = await knowledgeAPI.createCategory(categoryName);
        knowledgeCategories.value.push(category);
      }
      categoryId = category.id;
    }
    const tagIds: string[] = [];
    for (const tagName of baseForm.value.tags.split(/[,，]/).map((item) => item.trim()).filter(Boolean)) {
      let tag = knowledgeTags.value.find((item) => item.name.toLowerCase() === tagName.toLowerCase());
      if (!tag) {
        tag = await knowledgeAPI.createTag(tagName);
        knowledgeTags.value.push(tag);
      }
      tagIds.push(tag.id);
    }
    const created = await knowledgeAPI.createBase({
      name: baseForm.value.name,
      description: baseForm.value.description,
      knowledgeType: baseForm.value.knowledgeType,
      visibility: baseForm.value.visibility,
      categoryId,
      logoObjectKey: baseForm.value.logoObjectKey,
      tagIds
    });
    bases.value.unshift(created);
    selectedBaseId.value = created.id;
    baseForm.value = { name: "", description: "", knowledgeType: "PERSONAL", categoryName: "", tags: "", visibility: "PRIVATE", logoObjectKey: "" };
    baseDialogOpen.value = false;
    activeTab.value = "documents";
    ElMessage.success("知识库已创建");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    creatingBase.value = false;
  }
}

async function createAgent() {
  if (!agentForm.value.name.trim()) {
    ElMessage.warning("请输入智能体名称");
    return;
  }
  creatingAgent.value = true;
  try {
    const created = await knowledgeAPI.createAgent({ ...agentForm.value, status: "ACTIVE" });
    agents.value.unshift(created);
    selectedAgentId.value = created.id;
    boundBaseIds.value = selectedBaseId.value ? [selectedBaseId.value] : [];
    if (boundBaseIds.value.length) await knowledgeAPI.replaceBindings(created.id, bindingPayload());
    agentForm.value = { name: "", description: "", modelName: "", systemPrompt: "" };
    agentDialogOpen.value = false;
    activeTab.value = "settings";
    ElMessage.success("智能体已创建");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    creatingAgent.value = false;
  }
}

async function saveBindings() {
  if (!selectedAgentId.value) return;
  savingBindings.value = true;
  try {
    await knowledgeAPI.replaceBindings(selectedAgentId.value, bindingPayload());
    activeConversationId.value = "";
    ElMessage.success("知识库绑定已保存");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    savingBindings.value = false;
  }
}

async function uploadDocument(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file || !selectedBaseId.value) return;
  uploading.value = true;
  try {
    await knowledgeAPI.uploadDocument(selectedBaseId.value, file, file.name.toLowerCase().endsWith(".md") ? "heading" : "fixed");
    await Promise.all([loadDocuments(), loadBases()]);
    ElMessage.success("文档解析与索引已完成");
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    uploading.value = false;
    input.value = "";
  }
}

async function sendMessage() {
  const question = draft.value.trim();
  if (!canSend.value || generating.value || !selectedAgentId.value) return;
  draft.value = "";
  messages.value.push({ id: `user-${Date.now()}`, role: "user", content: question });
  const assistant: ChatMessage = { id: `assistant-${Date.now()}`, role: "assistant", content: "", citations: [] };
  messages.value.push(assistant);
  await scrollMessages();
  generating.value = true;
  activeRunId.value = "";
  abortController.value = new AbortController();
  try {
    if (!activeConversationId.value) {
      const conversation = await knowledgeAPI.createConversation(selectedAgentId.value, question.slice(0, 36));
      activeConversationId.value = conversation.id;
    }
    await streamKnowledgeRun(
      activeConversationId.value,
      { question, topK: 8, threshold: 0.2, mode: "HYBRID" },
      (eventName, value) => {
        if (eventName === "run.started") activeRunId.value = (value as KnowledgeStreamEvent).ragRunId || "";
        if (eventName === "answer.delta") assistant.content += String((value as KnowledgeStreamEvent).data?.delta || "");
        if (eventName === "result") {
          const result = value as KnowledgeRunResult;
          activeRunId.value = result.run.id;
          assistant.content = result.message.content;
          assistant.citations = result.citations || [];
        }
        if (eventName === "error") throw new Error((value as KnowledgeStreamEvent).error || "回答生成失败");
        void scrollMessages();
      },
      abortController.value.signal
    );
  } catch (error) {
    if ((error as Error).name !== "AbortError") {
      assistant.content = assistant.content || `生成失败：${errorMessage(error)}`;
      ElMessage.error(errorMessage(error));
    } else if (!assistant.content) {
      assistant.content = "回答已停止。";
    }
  } finally {
    generating.value = false;
    abortController.value = null;
    await scrollMessages();
  }
}

async function stopGeneration() {
  abortController.value?.abort();
  if (activeRunId.value) {
    try {
      await knowledgeAPI.cancelRun(activeRunId.value);
    } catch {
      // The request may already have completed when cancellation arrives.
    }
  }
  generating.value = false;
}

function openCitation(citation: KnowledgeCitation) {
  citationOpen.value = citation;
}

function citationPage(citation: KnowledgeCitation) {
  const locator = citation.locator || {};
  return locator.page || locator.pageStart || locator.pageNumber || "";
}

function formatScore(value?: number) {
  return typeof value === "number" ? `${(value * 100).toFixed(1)}%` : "—";
}

function formatDate(value: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请稍后重试";
}

async function scrollMessages() {
  await nextTick();
  if (messageScroller.value) messageScroller.value.scrollTop = messageScroller.value.scrollHeight;
}
</script>

<style scoped>
.ka-shell { --ka-ink:#18212f; --ka-muted:#718096; --ka-line:#e8edf3; --ka-brand:#5d5ce2; --ka-brand-soft:#f1f0ff; min-height:calc(100vh - 126px); background:#f6f8fb; color:var(--ka-ink); border:1px solid #edf0f5; border-radius:20px; overflow:hidden; }
.ka-topbar { min-height:92px; display:flex; align-items:center; gap:20px; padding:18px 24px; background:#fff; border-bottom:1px solid var(--ka-line); }
.ka-topbar > div:nth-child(2) { flex:1; min-width:220px; }
.ka-topbar span,.ka-main header > div > span,.ka-settings-panel > header > span { color:var(--ka-brand); font-size:11px; font-weight:800; letter-spacing:.1em; text-transform:uppercase; }
.ka-topbar h2 { margin:3px 0; font-size:22px; }.ka-topbar p { margin:0; color:var(--ka-muted); font-size:13px; }
.ka-back { border:0; background:#f2f4f8; color:#445066; border-radius:10px; padding:10px 13px; cursor:pointer; }
.ka-agent-picker { display:grid; grid-template-columns:auto 190px auto; gap:8px; align-items:center; }.ka-agent-picker label { font-size:12px; color:var(--ka-muted); }.ka-agent-picker select { height:38px; border:1px solid var(--ka-line); border-radius:10px; padding:0 12px; background:#fff; }.ka-agent-picker button,.ka-settings-panel footer button { height:38px; border:0; border-radius:10px; padding:0 15px; color:#fff; background:var(--ka-brand); cursor:pointer; font-weight:700; }
.ka-layout { display:grid; grid-template-columns:270px minmax(0,1fr); min-height:720px; }.ka-library-rail { background:#fff; border-right:1px solid var(--ka-line); padding:20px 16px; }.ka-library-rail > header { display:flex; justify-content:space-between; align-items:center; margin-bottom:14px; }.ka-library-rail > header div { display:flex; flex-direction:column; }.ka-library-rail > header span { color:var(--ka-muted); font-size:11px; }.ka-library-rail > header strong { font-size:18px; }.ka-library-rail > header button { width:34px; height:34px; border:0; border-radius:10px; background:var(--ka-brand); color:#fff; font-size:20px; cursor:pointer; }
.ka-search { height:38px; display:flex; align-items:center; gap:7px; background:#f5f7fa; border-radius:10px; padding:0 10px; margin-bottom:14px; }.ka-search input { border:0; outline:0; background:transparent; width:100%; }.ka-base-item { width:100%; display:grid; grid-template-columns:38px minmax(0,1fr) auto; gap:10px; text-align:left; align-items:center; border:1px solid transparent; background:transparent; padding:10px; border-radius:12px; cursor:pointer; margin-bottom:5px; }.ka-base-item:hover { background:#f7f8fc; }.ka-base-item.active { background:var(--ka-brand-soft); border-color:#dcd9ff; }.ka-base-item > span { width:38px; height:38px; display:grid; place-items:center; border-radius:11px; color:#fff; background:linear-gradient(135deg,#706ff0,#9b8df2); font-weight:800; }.ka-base-item div { min-width:0; }.ka-base-item strong,.ka-base-item small { display:block; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }.ka-base-item strong { font-size:13px; }.ka-base-item small { color:var(--ka-muted); font-size:11px; margin-top:4px; }.ka-base-item em { font-style:normal; font-size:10px; color:#17966f; }.ka-rail-state,.ka-empty-rail { color:var(--ka-muted); padding:28px 8px; font-size:13px; }.ka-empty-rail { display:flex; flex-direction:column; gap:7px; text-align:center; }.ka-empty-rail button { border:0; color:var(--ka-brand); background:transparent; cursor:pointer; font-weight:700; }
.ka-main { min-width:0; padding:20px; }.ka-tabs { display:flex; gap:4px; width:max-content; background:#e9edf3; border-radius:12px; padding:4px; margin-bottom:16px; }.ka-tabs button { border:0; background:transparent; color:#657086; padding:9px 16px; border-radius:9px; cursor:pointer; font-weight:700; }.ka-tabs button.active { background:#fff; color:var(--ka-brand); box-shadow:0 2px 8px #1f293712; }.ka-tabs span { margin-left:6px; color:#fff; background:var(--ka-brand); border-radius:8px; padding:1px 6px; font-size:10px; }
.ka-chat-panel,.ka-documents-panel,.ka-settings-panel { background:#fff; border:1px solid var(--ka-line); border-radius:16px; min-height:640px; overflow:hidden; }.ka-chat-panel { display:grid; grid-template-rows:auto 1fr auto; }.ka-chat-head { display:flex; justify-content:space-between; align-items:center; padding:15px 18px; border-bottom:1px solid var(--ka-line); }.ka-chat-head > div,.ka-agent-summary { display:flex; align-items:center; gap:11px; }.ka-chat-head strong,.ka-chat-head small { display:block; }.ka-chat-head small { color:var(--ka-muted); margin-top:3px; }.ka-chat-head button { border:1px solid var(--ka-line); background:#fff; color:#536078; border-radius:9px; padding:8px 11px; cursor:pointer; }.ka-agent-avatar { width:38px; height:38px; display:grid; place-items:center; border-radius:12px; background:linear-gradient(135deg,#5b5bd6,#8d7be8); color:#fff; font-weight:800; }.ka-agent-avatar.empty { background:#aeb7c7; }
.ka-messages { height:480px; overflow:auto; padding:24px clamp(18px,5vw,70px); scroll-behavior:smooth; }.ka-chat-empty { max-width:620px; margin:70px auto; text-align:center; }.ka-chat-empty > span,.ka-panel-empty > span { width:56px; height:56px; display:grid; place-items:center; margin:0 auto 14px; border-radius:18px; background:var(--ka-brand-soft); color:var(--ka-brand); font-weight:900; }.ka-chat-empty h3,.ka-panel-empty h3 { margin:0 0 8px; }.ka-chat-empty p,.ka-panel-empty p { color:var(--ka-muted); line-height:1.7; }.ka-chat-empty div { display:flex; flex-wrap:wrap; gap:8px; justify-content:center; margin-top:18px; }.ka-chat-empty button { border:1px solid var(--ka-line); background:#fff; border-radius:18px; padding:8px 12px; cursor:pointer; color:#59667b; }.ka-message { display:flex; gap:10px; margin-bottom:22px; }.ka-message > span { width:34px; height:34px; flex:0 0 34px; display:grid; place-items:center; border-radius:10px; background:#eeeefd; color:var(--ka-brand); font-weight:800; }.ka-message.user { flex-direction:row-reverse; }.ka-message.user > span { background:#eef1f5; color:#566174; }.ka-message > div { max-width:min(760px,82%); }.ka-message p { white-space:pre-wrap; margin:0; padding:12px 15px; border-radius:4px 14px 14px 14px; background:#f4f5ff; line-height:1.75; }.ka-message.user p { background:#eef1f5; border-radius:14px 4px 14px 14px; }.ka-inline-citations { display:flex; gap:7px; flex-wrap:wrap; margin-top:8px; }.ka-inline-citations button { border:1px solid #dcdefa; background:#fafaff; color:#5655bf; border-radius:8px; padding:6px 9px; cursor:pointer; font-size:11px; }
.ka-composer { padding:12px 16px 14px; border-top:1px solid var(--ka-line); background:#fbfcfe; }.ka-composer textarea { width:100%; box-sizing:border-box; resize:none; outline:0; border:1px solid #dfe4ec; border-radius:12px; padding:12px; font:inherit; }.ka-composer textarea:focus { border-color:#8987ed; box-shadow:0 0 0 3px #6462e414; }.ka-composer > div { display:flex; justify-content:space-between; align-items:center; margin-top:8px; }.ka-composer span { color:var(--ka-muted); font-size:11px; }.ka-composer button { border:0; background:var(--ka-brand); color:#fff; border-radius:9px; padding:9px 15px; cursor:pointer; font-weight:700; }.ka-composer button:disabled { opacity:.45; cursor:not-allowed; }.ka-composer button.stop { background:#e85757; }
.ka-documents-panel,.ka-settings-panel { padding:22px; box-sizing:border-box; }.ka-documents-panel > header { display:flex; justify-content:space-between; align-items:flex-start; }.ka-documents-panel h3,.ka-settings-panel h3 { margin:4px 0; font-size:20px; }.ka-documents-panel p,.ka-settings-panel p { margin:0; color:var(--ka-muted); }.ka-upload { position:relative; display:inline-grid; place-items:center; height:38px; padding:0 15px; background:var(--ka-brand); color:#fff; border-radius:10px; cursor:pointer; font-weight:700; }.ka-upload input { position:absolute; inset:0; opacity:0; cursor:pointer; }.ka-upload.disabled { opacity:.45; cursor:not-allowed; }.ka-pipeline-note { display:flex; align-items:center; gap:10px; margin:20px 0 12px; padding:11px 13px; border-radius:10px; background:#f5f7fb; color:#667187; font-size:12px; }.ka-pipeline-note b { color:#333e52; }.ka-pipeline-note span { color:#16866a; }.ka-pipeline-note em { margin-left:auto; font-style:normal; }.ka-document-table { border:1px solid var(--ka-line); border-radius:12px; overflow:hidden; }.ka-document-table .head,.ka-document-table .row { display:grid; grid-template-columns:minmax(240px,1fr) 100px 100px 150px 64px; gap:12px; align-items:center; padding:12px 15px; }.ka-document-table .head { background:#f7f8fb; color:#788397; font-size:11px; font-weight:700; }.ka-document-table .row { border-top:1px solid var(--ka-line); font-size:12px; }.ka-document-table .row > span:first-child b,.ka-document-table .row > span:first-child small { display:block; }.ka-document-table small { color:var(--ka-muted); margin-top:4px; }.ka-document-table em { display:inline-block; font-style:normal; color:#13825f; background:#eaf8f2; padding:4px 8px; border-radius:9px; }.ka-danger-link { border:0; background:transparent; color:#d64b4b; cursor:pointer; padding:5px 0; }.ka-danger-link:disabled { opacity:.5; cursor:not-allowed; }.ka-panel-state,.ka-panel-empty { padding:100px 20px; text-align:center; color:var(--ka-muted); }
.ka-settings-panel > header { margin-bottom:20px; }.ka-agent-summary { border:1px solid var(--ka-line); padding:14px; border-radius:12px; }.ka-agent-summary div { flex:1; }.ka-agent-summary strong,.ka-agent-summary small { display:block; }.ka-agent-summary small { color:var(--ka-muted); margin-top:4px; }.ka-agent-summary em { color:#16866a; font-style:normal; }.ka-binding-list { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; margin:16px 0; }.ka-binding-list label { display:grid; grid-template-columns:auto 38px 1fr auto; gap:10px; align-items:center; padding:12px; border:1px solid var(--ka-line); border-radius:12px; cursor:pointer; }.ka-binding-list label.selected { border-color:#c8c5ff; background:#f8f7ff; }.ka-binding-list label > span { width:38px; height:38px; display:grid; place-items:center; border-radius:10px; color:#fff; background:#7776df; }.ka-binding-list strong,.ka-binding-list small { display:block; }.ka-binding-list small { color:var(--ka-muted); margin-top:3px; }.ka-binding-list em { font-style:normal; color:var(--ka-muted); font-size:11px; }.ka-settings-panel > footer { display:flex; justify-content:flex-end; align-items:center; gap:16px; border-top:1px solid var(--ka-line); padding-top:15px; color:var(--ka-muted); }.ka-settings-panel footer button:disabled { opacity:.5; }
.ka-citation-mask { position:fixed; inset:0; z-index:3000; background:#10152266; display:flex; justify-content:flex-end; }.ka-citation-drawer { width:min(520px,92vw); height:100%; background:#fff; box-shadow:-15px 0 40px #10152222; padding:22px; box-sizing:border-box; overflow:auto; }.ka-citation-drawer header { display:flex; justify-content:space-between; align-items:flex-start; padding-bottom:18px; border-bottom:1px solid var(--ka-line); }.ka-citation-drawer header span,.ka-citation-drawer header strong { display:block; }.ka-citation-drawer header span { color:var(--ka-brand); font-size:12px; }.ka-citation-drawer header strong { margin-top:5px; }.ka-citation-drawer header button { border:0; background:#f1f3f6; border-radius:9px; width:34px; height:34px; font-size:22px; cursor:pointer; }.ka-citation-drawer section { margin-top:22px; }.ka-citation-drawer section > span { color:var(--ka-muted); font-size:12px; }.ka-citation-drawer blockquote { margin:10px 0; padding:16px; background:#f7f7ff; border-left:3px solid var(--ka-brand); line-height:1.8; white-space:pre-wrap; }.ka-citation-drawer dl div { display:grid; grid-template-columns:90px 1fr; gap:10px; padding:9px 0; border-bottom:1px solid var(--ka-line); }.ka-citation-drawer dt { color:var(--ka-muted); }.ka-citation-drawer dd { margin:0; word-break:break-all; }
.ka-dialog-form { display:grid; gap:16px; }.ka-dialog-form label { display:grid; gap:7px; }.ka-dialog-form label > span { font-size:13px; font-weight:700; }
@media (max-width:1280px) { .ka-topbar { align-items:flex-start; flex-wrap:wrap; }.ka-agent-picker { width:100%; grid-template-columns:auto 1fr auto; }.ka-layout { grid-template-columns:1fr; }.ka-library-rail { border-right:0; border-bottom:1px solid var(--ka-line); max-height:270px; overflow:auto; }.ka-binding-list { grid-template-columns:1fr; } }
@media (max-width:640px) { .ka-shell { border-radius:0; min-height:100vh; }.ka-topbar { padding:14px; }.ka-topbar > div:nth-child(2) p { display:none; }.ka-agent-picker { grid-template-columns:1fr auto; }.ka-agent-picker label { grid-column:1/-1; }.ka-main { padding:10px; }.ka-tabs { width:100%; }.ka-tabs button { flex:1; padding:9px 6px; }.ka-messages { padding:18px 10px; }.ka-message > div { max-width:88%; }.ka-documents-panel,.ka-settings-panel { padding:14px; }.ka-documents-panel > header { gap:10px; }.ka-document-table { overflow:auto; }.ka-document-table .head,.ka-document-table .row { min-width:720px; }.ka-pipeline-note em { display:none; } }
.ka-runtime-card{border:1px solid #dedcf8;background:#faf9ff;padding:14px;border-radius:12px;margin-bottom:14px}.ka-runtime-card>header{display:flex;justify-content:space-between;gap:12px;align-items:flex-start}.ka-runtime-card header span,.ka-runtime-card header strong{display:block}.ka-runtime-card header span{font-size:10px;color:var(--ka-brand);font-weight:800}.ka-runtime-card header strong{margin-top:3px}.ka-runtime-card header small{color:var(--ka-muted)}.ka-runtime-card>div{display:grid;grid-template-columns:1fr 1fr auto;gap:10px;align-items:end;margin-top:12px}.ka-runtime-card label{display:grid;gap:5px;color:var(--ka-muted);font-size:11px}.ka-runtime-card select{height:36px;border:1px solid var(--ka-line);border-radius:9px;background:#fff;padding:0 9px;min-width:0}.ka-runtime-card button{height:36px;border:0;border-radius:9px;background:var(--ka-brand);color:#fff;padding:0 13px;font-weight:700;cursor:pointer}.ka-runtime-card button:disabled{opacity:.5}@media(max-width:760px){.ka-runtime-card>div{grid-template-columns:1fr}.ka-runtime-card>header{flex-direction:column}}
.ka-binding-controls{grid-column:1/-1;display:grid;grid-template-columns:100px 100px minmax(160px,1fr);gap:8px;padding-top:9px;border-top:1px dashed var(--ka-line)}.ka-binding-controls>div{display:grid;gap:4px}.ka-binding-controls span{width:auto!important;height:auto!important;display:block!important;background:transparent!important;color:var(--ka-muted)!important;font-size:10px}.ka-binding-controls input,.ka-binding-controls select{height:32px;min-width:0;border:1px solid var(--ka-line);border-radius:8px;background:#fff;padding:0 8px}.ka-binding-controls input{width:100%;box-sizing:border-box}@media(max-width:760px){.ka-binding-controls{grid-template-columns:1fr 1fr}.ka-binding-controls>div:last-child{grid-column:1/-1}}
</style>
