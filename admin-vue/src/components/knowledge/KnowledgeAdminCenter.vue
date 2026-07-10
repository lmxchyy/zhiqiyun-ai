<template>
  <section class="kam-shell">
    <header class="kam-hero">
      <div>
        <span>Knowledge Operations</span>
        <h2>知识库管理中心</h2>
        <p>统一观察多租户知识库、解析索引、检索问答、模型配置与 Token 消耗。</p>
      </div>
      <div class="kam-actions">
        <label>租户 ID<input v-model.trim="tenantId" placeholder="留空查看全部租户" @keydown.enter="reload" /></label>
        <button type="button" :disabled="loading" @click="reload">{{ loading ? '刷新中…' : '刷新数据' }}</button>
      </div>
    </header>

    <div class="kam-metrics">
      <article v-for="metric in metrics" :key="metric.label" :class="metric.tone">
        <span>{{ metric.label }}</span><strong>{{ metric.value }}</strong><small>{{ metric.hint }}</small>
      </article>
    </div>

    <section class="kam-body">
      <aside>
        <button v-for="group in groups" :key="group.id" type="button" :class="{ active: activeResource === group.id }" @click="selectResource(group.id)">
          <span>{{ group.icon }}</span><div><strong>{{ group.label }}</strong><small>{{ group.hint }}</small></div>
        </button>
      </aside>
      <main>
        <header class="kam-table-head">
          <div><span>{{ activeMeta.eyebrow }}</span><h3>{{ activeMeta.label }}</h3><p>{{ activeMeta.description }}</p></div>
          <div><button v-if="isProfileResource" class="kam-add" type="button" @click="openProfileEditor()">+ 新增配置</button><label><span>⌕</span><input v-model.trim="keyword" placeholder="搜索当前列表" /></label><em>{{ filteredRecords.length }} 条</em></div>
        </header>

        <div v-if="error" class="kam-error"><b>加载失败</b><span>{{ error }}</span><button type="button" @click="reload">重试</button></div>
        <div v-else-if="loadingRecords" class="kam-state">正在读取 {{ activeMeta.label }}…</div>
        <div v-else-if="!filteredRecords.length" class="kam-empty"><span>{{ activeMeta.icon }}</span><h3>暂无{{ activeMeta.label }}</h3><p>数据产生后会自动显示在这里。</p></div>
        <div v-else class="kam-table-wrap">
          <table>
            <thead><tr><th v-for="column in activeColumns" :key="column.key">{{ column.label }}</th><th v-if="isProfileResource">操作</th></tr></thead>
            <tbody>
              <tr v-for="(record, index) in filteredRecords" :key="recordKey(record, index)">
                <td v-for="column in activeColumns" :key="column.key">
                  <template v-if="column.key === 'status'"><em :class="['kam-status', statusTone(record[column.key])]">{{ displayValue(record[column.key]) }}</em></template>
                  <template v-else-if="column.kind === 'time'">{{ formatTime(record[column.key]) }}</template>
                  <template v-else-if="column.kind === 'score'">{{ formatScore(record[column.key]) }}</template>
                  <template v-else-if="column.kind === 'number'">{{ formatNumber(record[column.key]) }}</template>
                  <template v-else><span :class="{ mono: column.kind === 'mono', primary: column.primary }">{{ displayValue(record[column.key]) }}</span></template>
                </td>
                <td v-if="isProfileResource"><button class="kam-edit" type="button" @click="openProfileEditor(record)">编辑</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </main>
    </section>

    <el-dialog v-model="profileEditorOpen" :title="editingProfileId ? '编辑知识库配置' : '新增知识库配置'" width="680px" append-to-body>
      <div class="kam-editor">
        <p>{{ profileHint }}</p>
        <el-input v-model="profileDraft" type="textarea" :rows="18" spellcheck="false" />
      </div>
      <template #footer><el-button @click="profileEditorOpen = false">取消</el-button><el-button type="primary" :loading="savingProfile" @click="saveProfile">保存并生效</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { adminKnowledgeAPI, type KnowledgeAdminOverview, type KnowledgeAdminRecord } from "../../api/adminKnowledge";

type ResourceId = "bases" | "documents" | "chunks" | "agents" | "parsing-logs" | "embedding-profiles" | "vector-stores" | "ingestion-profiles" | "retrieval-profiles" | "retrieval-logs" | "usage" | "hot-questions";
type Column = { key: string; label: string; kind?: "time" | "number" | "score" | "mono"; primary?: boolean };

const emptyOverview: KnowledgeAdminOverview = { tenantCount: 0, knowledgeBaseCount: 0, documentCount: 0, chunkCount: 0, readyDocumentCount: 0, failedDocumentCount: 0, agentCount: 0, ragRunCount: 0, completedRagRunCount: 0, inputTokens: 0, outputTokens: 0, pointCost: 0 };
const groups: Array<{ id: ResourceId; label: string; hint: string; icon: string; eyebrow: string; description: string }> = [
  { id: "bases", label: "知识库管理", hint: "空间、类型与状态", icon: "KB", eyebrow: "Knowledge Space", description: "查看所有租户的企业、部门、个人与智能体知识库。" },
  { id: "documents", label: "文档管理", hint: "文件与解析状态", icon: "DOC", eyebrow: "Document Registry", description: "追踪上传文档、类型、版本与解析就绪状态。" },
  { id: "chunks", label: "Chunk 管理", hint: "切片与 Token", icon: "CH", eyebrow: "Chunk Index", description: "审计规范化 Chunk、页码、Token 和索引状态。" },
  { id: "agents", label: "智能体绑定", hint: "Agent 实例", icon: "AI", eyebrow: "Agent Runtime", description: "查看知识库智能体的模型、所有者与运行状态。" },
  { id: "parsing-logs", label: "解析日志", hint: "阶段、进度与错误", icon: "LOG", eyebrow: "Ingestion Pipeline", description: "定位解析、清洗、切片、Embedding 和索引任务问题。" },
  { id: "embedding-profiles", label: "Embedding 管理", hint: "模型与维度", icon: "EMB", eyebrow: "Embedding Provider", description: "查看默认及租户级 Embedding 模型配置。" },
  { id: "vector-stores", label: "向量数据库", hint: "驱动与集合", icon: "VDB", eyebrow: "Vector Store", description: "查看 pgvector 及未来 Milvus、Qdrant、Weaviate 配置。" },
  { id: "ingestion-profiles", label: "解析配置", hint: "Parser 与 Chunk", icon: "CFG", eyebrow: "Ingestion Profile", description: "集中核对解析器、切片器、重叠与 Token 边界。" },
  { id: "retrieval-profiles", label: "检索配置", hint: "Hybrid 与 Rerank", icon: "RET", eyebrow: "Retrieval Profile", description: "查看 TopK、阈值、混合权重、Query Rewrite 与 Rerank。" },
  { id: "retrieval-logs", label: "检索日志", hint: "延迟与命中", icon: "RUN", eyebrow: "RAG Observability", description: "审计问题改写、检索延迟、生成耗时与运行状态。" },
  { id: "usage", label: "Token 统计", hint: "用量与点数", icon: "TOK", eyebrow: "Usage Metering", description: "按天和租户汇总问答次数、Token 与点数消耗。" },
  { id: "hot-questions", label: "热门问题", hint: "频次与趋势", icon: "HOT", eyebrow: "Question Intelligence", description: "发现高频问题并反向优化知识内容。" }
];

const columns: Record<ResourceId, Column[]> = {
  bases: [{ key: "name", label: "知识库", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "knowledgeType", label: "类型" }, { key: "documentCount", label: "文档", kind: "number" }, { key: "chunkCount", label: "Chunk", kind: "number" }, { key: "status", label: "状态" }, { key: "updatedAt", label: "更新时间", kind: "time" }],
  documents: [{ key: "name", label: "文档", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "knowledgeBaseId", label: "知识库", kind: "mono" }, { key: "documentType", label: "类型" }, { key: "mimeType", label: "MIME" }, { key: "status", label: "状态" }, { key: "updatedAt", label: "更新时间", kind: "time" }],
  chunks: [{ key: "id", label: "Chunk ID", kind: "mono", primary: true }, { key: "knowledgeBaseId", label: "知识库", kind: "mono" }, { key: "documentId", label: "文档", kind: "mono" }, { key: "title", label: "标题" }, { key: "tokenCount", label: "Token", kind: "number" }, { key: "pageStart", label: "起始页" }, { key: "status", label: "状态" }],
  agents: [{ key: "name", label: "智能体", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "ownerUserId", label: "所有者", kind: "mono" }, { key: "modelName", label: "模型" }, { key: "status", label: "状态" }, { key: "updatedAt", label: "更新时间", kind: "time" }],
  "parsing-logs": [{ key: "id", label: "任务", kind: "mono", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "documentId", label: "文档", kind: "mono" }, { key: "stage", label: "阶段" }, { key: "progress", label: "进度", kind: "number" }, { key: "status", label: "状态" }, { key: "errorMessage", label: "错误" }, { key: "updatedAt", label: "更新时间", kind: "time" }],
  "embedding-profiles": [{ key: "name", label: "配置", primary: true }, { key: "providerKey", label: "提供方" }, { key: "modelName", label: "模型" }, { key: "dimension", label: "维度", kind: "number" }, { key: "batchSize", label: "批量", kind: "number" }, { key: "normalized", label: "归一化" }, { key: "status", label: "状态" }],
  "vector-stores": [{ key: "name", label: "配置", primary: true }, { key: "providerKey", label: "驱动" }, { key: "endpoint", label: "Endpoint" }, { key: "collectionPrefix", label: "集合前缀", kind: "mono" }, { key: "distanceMetric", label: "距离" }, { key: "status", label: "状态" }],
  "ingestion-profiles": [{ key: "name", label: "配置", primary: true }, { key: "parserKey", label: "Parser" }, { key: "chunkerKey", label: "Chunker" }, { key: "chunkSize", label: "Chunk Size", kind: "number" }, { key: "overlap", label: "Overlap", kind: "number" }, { key: "minTokens", label: "Min", kind: "number" }, { key: "maxTokens", label: "Max", kind: "number" }, { key: "status", label: "状态" }],
  "retrieval-profiles": [{ key: "name", label: "配置", primary: true }, { key: "searchMode", label: "模式" }, { key: "topK", label: "TopK", kind: "number" }, { key: "threshold", label: "阈值", kind: "score" }, { key: "vectorWeight", label: "向量权重", kind: "score" }, { key: "keywordWeight", label: "全文权重", kind: "score" }, { key: "queryRewriteEnabled", label: "改写" }, { key: "status", label: "状态" }],
  "retrieval-logs": [{ key: "id", label: "Run", kind: "mono", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "originalQuery", label: "用户问题" }, { key: "status", label: "状态" }, { key: "retrievalLatencyMs", label: "检索 ms", kind: "number" }, { key: "generationLatencyMs", label: "生成 ms", kind: "number" }, { key: "inputTokens", label: "输入 Token", kind: "number" }, { key: "outputTokens", label: "输出 Token", kind: "number" }, { key: "createdAt", label: "时间", kind: "time" }],
  usage: [{ key: "usageDay", label: "日期", kind: "time", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "runCount", label: "问答", kind: "number" }, { key: "inputTokens", label: "输入 Token", kind: "number" }, { key: "outputTokens", label: "输出 Token", kind: "number" }, { key: "pointCost", label: "点数", kind: "number" }],
  "hot-questions": [{ key: "question", label: "问题", primary: true }, { key: "tenantId", label: "租户", kind: "mono" }, { key: "askCount", label: "提问次数", kind: "number" }, { key: "avgRetrievalLatencyMs", label: "平均检索 ms", kind: "number" }, { key: "lastAskedAt", label: "最近提问", kind: "time" }]
};

const overview = ref<KnowledgeAdminOverview>({ ...emptyOverview });
const records = ref<KnowledgeAdminRecord[]>([]);
const activeResource = ref<ResourceId>("bases");
const tenantId = ref("");
const keyword = ref("");
const loading = ref(false);
const loadingRecords = ref(false);
const error = ref("");
const profileEditorOpen = ref(false);
const profileDraft = ref("");
const editingProfileId = ref("");
const savingProfile = ref(false);
const activeMeta = computed(() => groups.find((item) => item.id === activeResource.value) || groups[0]);
const activeColumns = computed(() => columns[activeResource.value]);
const profileResourceIds: ResourceId[] = ["embedding-profiles", "vector-stores", "ingestion-profiles", "retrieval-profiles"];
const isProfileResource = computed(() => profileResourceIds.includes(activeResource.value));
const profileHint = computed(() => ({
  "embedding-profiles": "支持 OpenAI、Gemini、Qwen、BGE、BCE、Jina、SiliconFlow、OneAPI 和 NewAPI；config 可配置 baseUrl、timeoutMs，密钥使用平台已保存凭据或环境变量。",
  "vector-stores": "providerKey 可使用 pgvector、milvus、qdrant 或 weaviate；凭据建议使用 credentialRef 引用。",
  "ingestion-profiles": "组合 Embedding、向量库、Parser、OCR 与 Chunker，修改后对新的解析和索引任务生效。",
  "retrieval-profiles": "配置 Vector、Fulltext、Hybrid Search、TopK、Threshold、权重、Rerank 和 Query Rewrite。"
} as Partial<Record<ResourceId, string>>)[activeResource.value] || "");
const metrics = computed(() => [
  { label: "租户", value: formatNumber(overview.value.tenantCount), hint: "已纳入知识治理", tone: "purple" },
  { label: "知识库", value: formatNumber(overview.value.knowledgeBaseCount), hint: `${overview.value.documentCount} 份文档`, tone: "blue" },
  { label: "Chunk", value: formatNumber(overview.value.chunkCount), hint: `${overview.value.readyDocumentCount} 份已就绪`, tone: "green" },
  { label: "RAG 问答", value: formatNumber(overview.value.ragRunCount), hint: `${overview.value.completedRagRunCount} 次完成`, tone: "orange" },
  { label: "Token", value: formatCompact(overview.value.inputTokens + overview.value.outputTokens), hint: `${formatNumber(overview.value.pointCost)} 点消耗`, tone: "pink" }
]);
const filteredRecords = computed(() => {
  const query = keyword.value.toLowerCase();
  if (!query) return records.value;
  return records.value.filter((record) => Object.values(record).some((value) => String(value ?? "").toLowerCase().includes(query)));
});

onMounted(reload);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const [summary] = await Promise.all([adminKnowledgeAPI.overview(tenantId.value), loadRecords()]);
    overview.value = summary;
  } catch (reason) {
    error.value = errorMessage(reason);
  } finally {
    loading.value = false;
  }
}

async function loadRecords() {
  loadingRecords.value = true;
  error.value = "";
  try {
    records.value = (await adminKnowledgeAPI.records(activeResource.value, tenantId.value)).items || [];
  } catch (reason) {
    error.value = errorMessage(reason);
    records.value = [];
    throw reason;
  } finally {
    loadingRecords.value = false;
  }
}

function selectResource(resource: ResourceId) {
  activeResource.value = resource;
  keyword.value = "";
  void loadRecords();
}

function profileTemplate(resource: ResourceId): KnowledgeAdminRecord {
  const common = { id: "", tenantId: tenantId.value, name: "", status: "ACTIVE" };
  if (resource === "embedding-profiles") return { ...common, providerKey: "openai", modelName: "text-embedding-3-small", dimension: 1536, batchSize: 32, normalized: true, config: { baseUrl: "", timeoutMs: 30000 } };
  if (resource === "vector-stores") return { ...common, providerKey: "pgvector", endpoint: "", credentialRef: "", collectionPrefix: "xianzhi_kb", distanceMetric: "COSINE", config: {} };
  if (resource === "ingestion-profiles") return { ...common, embeddingProfileId: "embedding_deterministic_default", vectorStoreProfileId: "vector_pgvector_default", parserKey: "auto", ocrProviderKey: "", chunkerKey: "fixed", chunkSize: 800, overlap: 120, minTokens: 40, maxTokens: 1200, cleaningConfig: {} };
  return { ...common, searchMode: "HYBRID", topK: 8, threshold: 0.2, vectorWeight: 0.7, keywordWeight: 0.3, rerankProfileId: "", contextTokenLimit: 6000, queryRewriteEnabled: true, metadataFilterEnabled: true, config: {} };
}

function openProfileEditor(record?: KnowledgeAdminRecord) {
  const source = record ? { ...record } : profileTemplate(activeResource.value);
  editingProfileId.value = record ? String(record.id || "") : "";
  profileDraft.value = JSON.stringify(source, null, 2);
  profileEditorOpen.value = true;
}

async function saveProfile() {
  if (!isProfileResource.value) return;
  let payload: KnowledgeAdminRecord;
  try {
    payload = JSON.parse(profileDraft.value) as KnowledgeAdminRecord;
  } catch {
    ElMessage.error("配置 JSON 格式不正确");
    return;
  }
  if (!String(payload.name || "").trim()) {
    ElMessage.warning("请填写配置名称");
    return;
  }
  savingProfile.value = true;
  try {
    await adminKnowledgeAPI.saveProfile(activeResource.value, payload, editingProfileId.value);
    profileEditorOpen.value = false;
    await loadRecords();
    ElMessage.success("配置已保存并可供知识库切换");
  } catch (reason) {
    ElMessage.error(errorMessage(reason));
  } finally {
    savingProfile.value = false;
  }
}
function recordKey(record: KnowledgeAdminRecord, index: number) { return String(record.id || record.question || record.usageDay || index); }
function displayValue(value: unknown) { if (value === null || value === undefined || value === "") return "—"; if (typeof value === "boolean") return value ? "是" : "否"; return String(value); }
function formatNumber(value: unknown) { const number = Number(value || 0); return Number.isFinite(number) ? number.toLocaleString("zh-CN") : displayValue(value); }
function formatCompact(value: unknown) { const number = Number(value || 0); return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(number); }
function formatScore(value: unknown) { const number = Number(value); return Number.isFinite(number) ? `${(number * 100).toFixed(1)}%` : "—"; }
function formatTime(value: unknown) { if (!value) return "—"; const date = new Date(String(value)); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN", { hour12: false }); }
function statusTone(value: unknown) { const status = String(value || "").toUpperCase(); if (["ACTIVE", "READY", "COMPLETED"].includes(status)) return "success"; if (["FAILED", "DISABLED", "CANCELLED"].includes(status)) return "danger"; return "warning"; }
function errorMessage(reason: unknown) { return reason instanceof Error ? reason.message : "数据加载失败"; }
</script>

<style scoped>
.kam-shell{--ink:#1c2635;--muted:#758195;--line:#e7ebf1;--brand:#655ce7;color:var(--ink);display:grid;gap:16px}.kam-hero{display:flex;align-items:center;justify-content:space-between;gap:24px;padding:24px 26px;border-radius:18px;background:linear-gradient(120deg,#20254e,#4c489c 58%,#6c5ce7);color:#fff;box-shadow:0 18px 38px #3c34752b}.kam-hero span{font-size:11px;font-weight:800;letter-spacing:.14em;color:#bdb9ff}.kam-hero h2{font-size:28px;margin:5px 0}.kam-hero p{margin:0;color:#d8d9ec}.kam-actions{display:flex;align-items:flex-end;gap:9px}.kam-actions label{display:grid;gap:5px;font-size:11px;color:#d8d9ec}.kam-actions input{width:220px;height:38px;box-sizing:border-box;border:1px solid #ffffff28;border-radius:10px;background:#ffffff12;color:#fff;padding:0 11px;outline:0}.kam-actions input::placeholder{color:#ffffff80}.kam-actions button{height:38px;border:0;border-radius:10px;background:#fff;color:#4f48c4;font-weight:800;padding:0 15px;cursor:pointer}.kam-metrics{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:11px}.kam-metrics article{position:relative;overflow:hidden;background:#fff;border:1px solid var(--line);border-radius:14px;padding:16px}.kam-metrics article::before{content:"";position:absolute;left:0;top:0;width:4px;height:100%;background:#6b64dd}.kam-metrics article.blue::before{background:#3f8cdb}.kam-metrics article.green::before{background:#29a278}.kam-metrics article.orange::before{background:#e68c36}.kam-metrics article.pink::before{background:#d66eaa}.kam-metrics span,.kam-metrics small{display:block;color:var(--muted)}.kam-metrics span{font-size:12px}.kam-metrics strong{display:block;font-size:25px;margin:7px 0 5px}.kam-metrics small{font-size:11px}.kam-body{display:grid;grid-template-columns:230px minmax(0,1fr);min-height:610px;background:#fff;border:1px solid var(--line);border-radius:16px;overflow:hidden}.kam-body aside{padding:12px;background:#f7f8fb;border-right:1px solid var(--line);overflow:auto}.kam-body aside button{width:100%;display:flex;gap:10px;align-items:center;text-align:left;border:1px solid transparent;border-radius:11px;background:transparent;padding:9px;cursor:pointer;margin-bottom:3px}.kam-body aside button:hover{background:#fff}.kam-body aside button.active{background:#fff;border-color:#dcd9fb;box-shadow:0 5px 14px #29265e0b}.kam-body aside button>span{width:36px;height:36px;display:grid;place-items:center;flex:0 0 36px;border-radius:10px;background:#ecebff;color:#5c55d4;font-size:10px;font-weight:900}.kam-body aside strong,.kam-body aside small{display:block}.kam-body aside strong{font-size:12px}.kam-body aside small{font-size:10px;color:var(--muted);margin-top:3px}.kam-body main{min-width:0;padding:20px}.kam-table-head{display:flex;justify-content:space-between;align-items:flex-end;gap:20px;padding-bottom:16px;border-bottom:1px solid var(--line)}.kam-table-head>div:first-child>span{color:var(--brand);font-size:10px;font-weight:800;letter-spacing:.12em;text-transform:uppercase}.kam-table-head h3{font-size:20px;margin:3px 0}.kam-table-head p{margin:0;color:var(--muted);font-size:12px}.kam-table-head>div:last-child{display:flex;align-items:center;gap:8px}.kam-table-head label{height:36px;display:flex;align-items:center;gap:6px;border:1px solid var(--line);border-radius:9px;padding:0 10px}.kam-table-head input{border:0;outline:0;width:170px}.kam-table-head em{font-style:normal;color:var(--muted);font-size:11px;white-space:nowrap}.kam-table-wrap{overflow:auto;margin-top:14px;border:1px solid var(--line);border-radius:11px}.kam-table-wrap table{width:100%;min-width:920px;border-collapse:collapse;font-size:11px}.kam-table-wrap th{text-align:left;color:#788397;background:#f7f8fb;padding:11px 12px;white-space:nowrap}.kam-table-wrap td{padding:11px 12px;border-top:1px solid var(--line);max-width:260px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.kam-table-wrap tr:hover td{background:#fbfbfe}.kam-table-wrap .primary{font-weight:800;color:#2e394c}.kam-table-wrap .mono{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;color:#5d6680}.kam-status{display:inline-block;padding:4px 7px;border-radius:8px;font-style:normal;font-size:10px;background:#fff5dd;color:#a66b12}.kam-status.success{background:#e9f8f1;color:#147a59}.kam-status.danger{background:#ffeded;color:#bd4545}.kam-state,.kam-empty,.kam-error{display:flex;min-height:360px;align-items:center;justify-content:center;flex-direction:column;gap:7px;color:var(--muted)}.kam-empty>span{width:54px;height:54px;display:grid;place-items:center;border-radius:16px;background:#f0efff;color:var(--brand);font-weight:900}.kam-empty h3,.kam-empty p{margin:0}.kam-error{color:#a64444}.kam-error button{border:0;background:#f4e9e9;color:#9d4141;padding:8px 12px;border-radius:8px;cursor:pointer}@media(max-width:1200px){.kam-metrics{grid-template-columns:repeat(3,1fr)}.kam-body{grid-template-columns:190px minmax(0,1fr)}}@media(max-width:760px){.kam-hero{align-items:flex-start;flex-direction:column}.kam-actions{width:100%}.kam-actions label{flex:1}.kam-actions input{width:100%}.kam-metrics{grid-template-columns:repeat(2,1fr)}.kam-body{grid-template-columns:1fr}.kam-body aside{display:flex;overflow:auto;border-right:0;border-bottom:1px solid var(--line)}.kam-body aside button{min-width:150px}.kam-table-head{align-items:flex-start;flex-direction:column}.kam-table-head>div:last-child,.kam-table-head label{width:100%;box-sizing:border-box}.kam-table-head input{width:100%}}
.kam-add{height:36px;border:0;border-radius:9px;background:var(--brand);color:#fff;padding:0 11px;font-weight:800;cursor:pointer;white-space:nowrap}.kam-edit{border:0;background:#eeecff;color:#554dd2;border-radius:7px;padding:5px 9px;cursor:pointer}.kam-editor p{margin:0 0 12px;color:#68758a;line-height:1.7}.kam-editor :deep(textarea){font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12px;line-height:1.55}
</style>
