<template>
  <section class="user-models-page">
    <header class="user-models-head">
      <div class="user-models-head-copy">
        <h2>模型列表</h2>
        <p>汇总平台当前可调用的全部模型，支持新增、编辑、启停与删除；修改后立即影响创作页可选项。</p>
      </div>
      <div class="user-models-head-actions">
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button v-if="!readonly" type="primary" :icon="Plus" @click="openCreate">新增模型</el-button>
      </div>
    </header>

    <div class="user-models-stats">
      <article><span>配置模型</span><strong>{{ models.length }}</strong></article>
      <article><span>当前可调用</span><strong>{{ callableCount }}</strong></article>
      <article><span>启用中</span><strong>{{ activeCount }}</strong></article>
      <article><span>上游渠道</span><strong>{{ channels.length }}</strong></article>
    </div>

    <el-alert
      v-if="loadError"
      :title="loadError"
      :type="readonly ? 'warning' : 'error'"
      :closable="false"
      show-icon
      class="user-models-alert"
    />

    <div v-if="!readonly" class="user-models-filters">
      <el-select v-model="filterModule" placeholder="全部模块" clearable style="width: 180px">
        <el-option label="AI 生图" value="image_generation" />
        <el-option label="视频生成" value="video_generation" />
        <el-option label="PPT 文档生成" value="ppt_generation" />
      </el-select>
      <el-select v-model="filterStatus" placeholder="全部状态" clearable style="width: 140px">
        <el-option label="启用" value="ACTIVE" />
        <el-option label="停用" value="INACTIVE" />
      </el-select>
      <el-input v-model="keyword" placeholder="搜索模型名称 / 供应商" clearable style="width: 260px" :prefix-icon="Search" />
    </div>

    <el-table v-if="visibleModels.length" v-loading="loading" :data="visibleModels" stripe>
      <el-table-column label="模型" min-width="200">
        <template #default="scope">
          <div class="user-models-main-cell">
            <strong>{{ modelText(scope.row, "model_name", "modelName") || scope.row.code }}</strong>
            <small>{{ scope.row.id }}</small>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="所属模块" min-width="130">
        <template #default="scope">
          <el-tag effect="plain">{{ moduleLabel(modelText(scope.row, "module_code", "moduleCode")) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="130">
        <template #default="scope">{{ modelText(scope.row, "model_type", "modelType") || "-" }}</template>
      </el-table-column>
      <el-table-column label="上游 / 渠道" min-width="150">
        <template #default="scope">{{ channelLabel(scope.row) }}</template>
      </el-table-column>
      <el-table-column label="能力" min-width="200">
        <template #default="scope">
          <div class="user-models-chip-list">
            <el-tag v-for="item in capabilityList(scope.row)" :key="item" size="small" effect="plain">{{ item }}</el-tag>
            <span v-if="!capabilityList(scope.row).length" class="user-models-muted">-</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="当前可调用" width="110">
        <template #default="scope">
          <el-tag :type="isCallable(scope.row) ? 'success' : 'info'" size="small">{{ isCallable(scope.row) ? "可调用" : "不可用" }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="scope">
          <el-tag :type="isActive(scope.row) ? 'success' : 'danger'" size="small">{{ isActive(scope.row) ? "启用" : "停用" }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" min-width="170">
        <template #default="scope">{{ formatTime(modelText(scope.row, "updated_at", "updatedAt")) }}</template>
      </el-table-column>
      <el-table-column v-if="!readonly" label="操作" fixed="right" width="180">
        <template #default="scope">
          <el-button link type="primary" @click="openEdit(scope.row)">编辑</el-button>
          <el-button link :type="isActive(scope.row) ? 'warning' : 'success'" @click="toggleModel(scope.row)">{{ isActive(scope.row) ? "停用" : "启用" }}</el-button>
          <el-button link type="danger" @click="removeModel(scope.row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-else-if="!loading" :description="readonly ? '暂无可调用的模型' : '暂无模型配置，点击右上角新增'" />

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑模型' : '新增模型'" width="560px" destroy-on-close>
      <el-form :model="form" label-width="96px">
        <el-form-item label="模型名称" required>
          <el-input v-model="form.model_name" placeholder="例如 gpt-image-1 / wan2.2-t2v" />
        </el-form-item>
        <el-form-item label="所属模块" required>
          <el-select v-model="form.module_code" style="width: 100%">
            <el-option label="AI 生图" value="image_generation" />
            <el-option label="视频生成" value="video_generation" />
            <el-option label="PPT 文档生成" value="ppt_generation" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型类型">
          <el-input v-model="form.model_type" placeholder="例如 text-to-image / image-to-video（留空自动）" />
        </el-form-item>
        <el-form-item label="供应商">
          <el-input v-model="form.provider" placeholder="默认 NewAPI" />
        </el-form-item>
        <el-form-item label="上游渠道">
          <el-select v-model="form.channel_id" clearable placeholder="不指定则自动路由" style="width: 100%">
            <el-option v-for="channel in channels" :key="channel.id" :label="channelLabel(channel)" :value="String(channel.id)" />
          </el-select>
        </el-form-item>
        <el-form-item label="回退模型">
          <el-input v-model="form.fallback_model" placeholder="失败时回退的模型名称（可选）" />
        </el-form-item>
        <el-form-item label="排序权重">
          <el-input-number v-model="form.sort_weight" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="状态">
          <el-radio-group v-model="form.status">
            <el-radio value="ACTIVE">启用</el-radio>
            <el-radio value="INACTIVE">停用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitForm">{{ editingId ? "保存修改" : "创建模型" }}</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { ElMessageBox } from "element-plus/es/components/message-box/index";
import { Plus, Refresh, Search } from "@element-plus/icons-vue";
import { adminRequest } from "../../api/client";

interface AdminAIModelRow {
  id: string;
  model_name?: string;
  modelName?: string;
  model_type?: string;
  modelType?: string;
  provider?: string;
  channel_id?: string;
  channelId?: string;
  capability_code?: string[];
  capabilityCode?: string[];
  module_code?: string;
  moduleCode?: string;
  status?: string;
  fallback_model?: string;
  fallbackModel?: string;
  sort_weight?: number;
  sortWeight?: number;
  updated_at?: string;
  updatedAt?: string;
}

interface ChannelRow {
  id?: string | number;
  name?: string;
  models?: string[];
  modelsSnake?: string[];
}

const models = ref<AdminAIModelRow[]>([]);
const channels = ref<ChannelRow[]>([]);
const callableCodes = ref<Set<string>>(new Set());
const loading = ref(false);
const saving = ref(false);
const loadError = ref("");
const readonly = ref(false);

const filterModule = ref("");
const filterStatus = ref("");
const keyword = ref("");

const dialogVisible = ref(false);
const editingId = ref("");
const form = reactive({
  model_name: "",
  module_code: "image_generation",
  model_type: "",
  provider: "",
  channel_id: "",
  fallback_model: "",
  sort_weight: 0,
  status: "ACTIVE"
});

const moduleLabels: Record<string, string> = {
  image_generation: "AI 生图",
  video_generation: "视频生成",
  ppt_generation: "PPT 文档生成"
};

const visibleModels = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return models.value.filter((row) => {
    const moduleCode = String(modelText(row, "module_code", "moduleCode") || "");
    if (filterModule.value && moduleCode !== filterModule.value) return false;
    if (filterStatus.value && String(row.status || "").toUpperCase() !== filterStatus.value) return false;
    if (kw) {
      const haystack = [modelText(row, "model_name", "modelName"), row.provider, row.id]
        .map((item) => String(item || "").toLowerCase())
        .join(" ");
      if (!haystack.includes(kw)) return false;
    }
    return true;
  });
});

const callableCount = computed(() => models.value.filter((row) => isCallable(row)).length);
const activeCount = computed(() => models.value.filter((row) => isActive(row)).length);

function modelText(row: AdminAIModelRow, snake: string, camel: string): string {
  const value = (row as unknown as Record<string, unknown>)[snake] ?? (row as unknown as Record<string, unknown>)[camel];
  return value == null ? "" : String(value);
}

function isActive(row: AdminAIModelRow) {
  return String(row.status || "").toUpperCase() === "ACTIVE";
}

function isCallable(row: AdminAIModelRow) {
  return callableCodes.value.has(modelText(row, "model_name", "modelName").toLowerCase());
}

function moduleLabel(code: string) {
  return moduleLabels[code] || code || "-";
}

function capabilityList(row: AdminAIModelRow) {
  const value = row.capability_code ?? row.capabilityCode ?? [];
  return Array.isArray(value) ? value : [];
}

function channelLabel(row: ChannelRow | AdminAIModelRow) {
  const channelId = String((row as AdminAIModelRow).channel_id ?? (row as AdminAIModelRow).channelId ?? (row as ChannelRow).id ?? "");
  const channel = channels.value.find((item) => String(item.id) === channelId);
  if (channel?.name) return channel.name;
  if (channelId) return `渠道 ${channelId}`;
  return String((row as AdminAIModelRow).provider || "") || "自动路由";
}

function formatTime(value: string) {
  if (!value) return "-";
  return String(value).replace("T", " ").split(".")[0] || value;
}

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    const [overview, publicModels] = await Promise.all([
      adminRequest<{ models?: AdminAIModelRow[]; channels?: ChannelRow[] }>({ method: "GET", url: "/admin/ai/overview" }),
      adminRequest<Array<{ code?: string }>>({ method: "GET", url: "/models" }).catch(() => [] as Array<{ code?: string }>)
    ]);
    models.value = Array.isArray(overview?.models) ? overview.models : [];
    channels.value = Array.isArray(overview?.channels) ? overview.channels : [];
    callableCodes.value = new Set(
      (Array.isArray(publicModels) ? publicModels : [])
        .map((item) => String(item?.code || "").toLowerCase())
        .filter(Boolean)
    );
    readonly.value = false;
  } catch (error) {
    readonly.value = true;
    models.value = [];
    channels.value = [];
    loadError.value = error instanceof Error
      ? `${error.message}（模型管理需要管理员权限，当前以只读视角打开）`
      : "模型管理需要管理员权限";
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingId.value = "";
  Object.assign(form, {
    model_name: "",
    module_code: "image_generation",
    model_type: "",
    provider: "",
    channel_id: "",
    fallback_model: "",
    sort_weight: 0,
    status: "ACTIVE"
  });
  dialogVisible.value = true;
}

function openEdit(row: AdminAIModelRow) {
  editingId.value = String(row.id || "");
  Object.assign(form, {
    model_name: modelText(row, "model_name", "modelName"),
    module_code: modelText(row, "module_code", "moduleCode") || "image_generation",
    model_type: modelText(row, "model_type", "modelType"),
    provider: String(row.provider || ""),
    channel_id: String(row.channel_id ?? row.channelId ?? ""),
    fallback_model: String(row.fallback_model ?? row.fallbackModel ?? ""),
    sort_weight: Number(row.sort_weight ?? row.sortWeight ?? 0) || 0,
    status: String(row.status || "ACTIVE").toUpperCase()
  });
  dialogVisible.value = true;
}

async function submitForm() {
  if (!form.model_name.trim()) {
    ElMessage.warning("请填写模型名称");
    return;
  }
  saving.value = true;
  try {
    const payload: Record<string, unknown> = {
      model_name: form.model_name.trim(),
      module_code: form.module_code,
      model_type: form.model_type.trim(),
      provider: form.provider.trim() || "NewAPI",
      status: form.status,
      sort_weight: Number(form.sort_weight) || 0
    };
    if (form.channel_id) payload.channel_id = form.channel_id;
    else payload.channel_id = "";
    if (form.fallback_model.trim()) payload.fallback_model = form.fallback_model.trim();
    else payload.fallback_model = "";
    if (editingId.value) {
      await adminRequest({ method: "PATCH", url: `/admin/ai/models/${encodeURIComponent(editingId.value)}`, data: payload });
      ElMessage.success("模型已更新");
    } else {
      await adminRequest({ method: "POST", url: "/admin/ai/models", data: payload });
      ElMessage.success("模型已创建");
    }
    dialogVisible.value = false;
    await load();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function toggleModel(row: AdminAIModelRow) {
  const next = isActive(row) ? "INACTIVE" : "ACTIVE";
  saving.value = true;
  try {
    await adminRequest({ method: "PATCH", url: `/admin/ai/models/${encodeURIComponent(String(row.id))}`, data: { status: next } });
    ElMessage.success(next === "ACTIVE" ? "模型已启用" : "模型已停用");
    await load();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "操作失败");
  } finally {
    saving.value = false;
  }
}

async function removeModel(row: AdminAIModelRow) {
  const name = modelText(row, "model_name", "modelName") || row.id;
  const confirmed = await ElMessageBox.confirm(
    `确定删除模型「${name}」吗？删除后创作页将不再显示该模型，且无法恢复。`,
    "删除模型",
    { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" }
  ).catch(() => false);
  if (!confirmed) return;
  saving.value = true;
  try {
    await adminRequest({ method: "DELETE", url: `/admin/ai/models/${encodeURIComponent(String(row.id))}` });
    ElMessage.success("模型已删除");
    await load();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "删除失败");
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void load();
});
</script>

<style scoped>
.user-models-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.user-models-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.user-models-head-copy h2 {
  margin: 0 0 6px;
  font-size: 20px;
}

.user-models-head-copy p {
  margin: 0;
  color: var(--el-text-color-secondary, #909399);
  font-size: 13px;
}

.user-models-head-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.user-models-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.user-models-stats article {
  background: var(--el-fill-color-light, #f5f7fa);
  border-radius: 10px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.user-models-stats article span {
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
}

.user-models-stats article strong {
  font-size: 22px;
}

.user-models-alert {
  border-radius: 8px;
}

.user-models-filters {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.user-models-main-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.user-models-main-cell small {
  color: var(--el-text-color-secondary, #909399);
  font-size: 12px;
}

.user-models-chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.user-models-muted {
  color: var(--el-text-color-secondary, #909399);
}

@media (max-width: 900px) {
  .user-models-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .user-models-head {
    flex-direction: column;
  }
}
</style>
