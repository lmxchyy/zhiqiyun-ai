<template>
  <section class="storage-center">
    <header class="storage-hero">
      <div>
        <el-tag effect="dark" type="primary">Object Storage</el-tag>
        <h2>对象存储与文件中心</h2>
        <p>统一管理存储服务商、租户文件、容量配额与回收站。业务模块仅保存 file_id，不直接依赖厂商 SDK。</p>
      </div>
      <el-button :icon="Refresh" :loading="loading" @click="loadAll">刷新</el-button>
    </header>

    <div class="storage-metrics">
      <article><span>有效文件</span><strong>{{ overview.totalFiles || 0 }}</strong><small>{{ formatBytes(overview.totalBytes || 0) }}</small></article>
      <article><span>上传中</span><strong>{{ overview.pendingFiles || 0 }}</strong><small>容量已预占</small></article>
      <article><span>回收站</span><strong>{{ overview.recycleFiles || 0 }}</strong><small>到期后物理清理</small></article>
      <article :class="{ danger: (overview.abnormalFiles || 0) > 0 }"><span>异常文件</span><strong>{{ overview.abnormalFiles || 0 }}</strong><small>失败或隔离</small></article>
    </div>

    <el-tabs v-model="activeTab" class="storage-tabs">
      <el-tab-pane label="存储概览" name="overview">
        <div class="overview-grid">
          <el-card shadow="never">
            <template #header><strong>租户容量</strong></template>
            <div class="quota-summary">
              <el-progress :percentage="quotaPercent" :stroke-width="12" :status="quotaPercent >= 95 ? 'exception' : quotaPercent >= 80 ? 'warning' : 'success'" />
              <dl>
                <div><dt>已使用</dt><dd>{{ formatBytes(overview.quota?.usedBytes || 0) }}</dd></div>
                <div><dt>已预占</dt><dd>{{ formatBytes(overview.quota?.reservedBytes || 0) }}</dd></div>
                <div><dt>总配额</dt><dd>{{ formatBytes(overview.quota?.quotaBytes || 0) }}</dd></div>
              </dl>
            </div>
          </el-card>
          <el-card shadow="never">
            <template #header><strong>服务商容量分布</strong></template>
            <el-empty v-if="providerUsage.length === 0" description="暂无已完成文件" />
            <div v-else class="provider-usage">
              <div v-for="item in providerUsage" :key="item.provider">
                <span>{{ providerLabel(item.provider) }}</span><strong>{{ formatBytes(item.bytes) }}</strong>
                <el-progress :percentage="item.percent" :show-text="false" />
              </div>
            </div>
          </el-card>
        </div>
      </el-tab-pane>

      <el-tab-pane label="存储服务商" name="configs">
        <div class="table-toolbar">
          <div><strong>存储配置</strong><span>密钥加密保存，编辑时不会回显 Secret Key</span></div>
          <el-button type="primary" :icon="Plus" @click="openCreateConfig">新增配置</el-button>
        </div>
        <el-table :data="configs" stripe>
          <el-table-column label="配置" min-width="180">
            <template #default="scope">
              <div class="primary-cell"><strong>{{ scope.row.name }}</strong><small>{{ scope.row.tenantId }}</small></div>
            </template>
          </el-table-column>
          <el-table-column label="服务商" width="130"><template #default="scope">{{ providerLabel(scope.row.provider) }}</template></el-table-column>
          <el-table-column prop="endpoint" label="Endpoint" min-width="210" show-overflow-tooltip />
          <el-table-column prop="bucket" label="Bucket" min-width="150" show-overflow-tooltip />
          <el-table-column label="默认" width="80"><template #default="scope"><el-tag v-if="scope.row.isDefault" size="small">默认</el-tag><span v-else>-</span></template></el-table-column>
          <el-table-column label="状态" width="100"><template #default="scope"><el-tag :type="scope.row.status === 'ENABLED' ? 'success' : 'info'">{{ scope.row.status === 'ENABLED' ? '启用' : '停用' }}</el-tag></template></el-table-column>
          <el-table-column label="检测" min-width="150">
            <template #default="scope">
              <div class="primary-cell"><el-tag size="small" :type="scope.row.lastTestStatus === 'SUCCESS' ? 'success' : scope.row.lastTestStatus === 'FAILED' ? 'danger' : 'info'">{{ scope.row.lastTestStatus || '未检测' }}</el-tag><small>{{ formatTime(scope.row.lastTestAt) }}</small></div>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="240" fixed="right">
            <template #default="scope">
              <el-button link type="primary" :loading="testingId === scope.row.id" @click="runConnectionTest(scope.row)">连接测试</el-button>
              <el-button link :disabled="scope.row.isSystem" @click="openEditConfig(scope.row)">编辑</el-button>
              <el-button link type="danger" :disabled="scope.row.isSystem" @click="removeConfig(scope.row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="文件管理" name="files">
        <div class="table-toolbar file-toolbar">
          <div class="filters">
            <el-input v-model="fileQuery" clearable placeholder="文件名或 file_id" :prefix-icon="Search" @keyup.enter="loadFiles" />
            <el-select v-model="fileStatus" clearable placeholder="全部状态" @change="loadFiles">
              <el-option label="有效" value="ACTIVE" /><el-option label="待上传" value="PENDING_UPLOAD" />
              <el-option label="回收站" value="DELETE_PENDING" /><el-option label="上传失败" value="UPLOAD_FAILED" />
              <el-option label="隔离" value="QUARANTINED" />
            </el-select>
            <el-button :icon="Search" @click="loadFiles">查询</el-button>
          </div>
          <span>共 {{ fileTotal }} 个文件</span>
        </div>
        <el-table :data="files" stripe>
          <el-table-column label="文件" min-width="220">
            <template #default="scope"><div class="primary-cell"><strong>{{ scope.row.originalName }}</strong><small>{{ scope.row.fileId }}</small></div></template>
          </el-table-column>
          <el-table-column prop="businessType" label="业务类型" width="140" />
          <el-table-column label="大小" width="110"><template #default="scope">{{ formatBytes(scope.row.fileSize) }}</template></el-table-column>
          <el-table-column prop="provider" label="服务商" width="120" />
          <el-table-column label="可见性" width="100"><template #default="scope"><el-tag size="small" effect="plain">{{ scope.row.visibility }}</el-tag></template></el-table-column>
          <el-table-column label="状态" width="130"><template #default="scope"><el-tag :type="fileStatusType(scope.row.status)">{{ scope.row.status }}</el-tag></template></el-table-column>
          <el-table-column label="创建时间" width="170"><template #default="scope">{{ formatTime(scope.row.createdAt) }}</template></el-table-column>
          <el-table-column label="操作" width="220" fixed="right">
            <template #default="scope">
              <el-button v-if="scope.row.status === 'ACTIVE'" link type="primary" @click="downloadFile(scope.row)">下载</el-button>
              <el-button v-if="scope.row.status === 'ACTIVE'" link type="danger" @click="moveToRecycle(scope.row)">删除</el-button>
              <el-button v-if="scope.row.status === 'DELETE_PENDING'" link type="primary" @click="restoreFile(scope.row)">恢复</el-button>
              <el-button v-if="scope.row.status === 'DELETE_PENDING'" link type="danger" @click="permanentDelete(scope.row)">永久删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination v-if="fileTotal > filePageSize" v-model:current-page="filePage" :page-size="filePageSize" :total="fileTotal" layout="prev, pager, next" @current-change="loadFiles" />
      </el-tab-pane>

      <el-tab-pane label="租户容量" name="quota">
        <el-card shadow="never" class="quota-card">
          <template #header><strong>当前租户容量策略</strong></template>
          <el-form label-position="top" :model="quotaForm">
            <el-form-item label="容量上限（GB）"><el-input-number v-model="quotaForm.quotaGB" :min="0" :max="1048576" /></el-form-item>
            <el-form-item label="预警阈值（%）"><el-slider v-model="quotaForm.warningPercent" :min="1" :max="99" show-input /></el-form-item>
            <el-form-item label="严重阈值（%）"><el-slider v-model="quotaForm.criticalPercent" :min="1" :max="100" show-input /></el-form-item>
            <el-button type="primary" :loading="savingQuota" @click="saveQuota">保存容量策略</el-button>
          </el-form>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="configDialogVisible" :title="editingConfigId ? '编辑存储配置' : '新增存储配置'" width="620px" destroy-on-close>
      <el-form label-position="top" :model="configForm">
        <div class="form-grid">
          <el-form-item label="配置名称"><el-input v-model="configForm.name" placeholder="例如：生产 MinIO" /></el-form-item>
          <el-form-item label="服务商"><el-select v-model="configForm.provider"><el-option v-for="item in providerOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
        </div>
        <el-form-item label="Endpoint"><el-input v-model="configForm.endpoint" placeholder="https://storage.example.com" /></el-form-item>
        <el-form-item label="客户端签名 Endpoint"><el-input v-model="configForm.signingEndpoint" placeholder="浏览器可访问，例如 https://files.example.com" /></el-form-item>
        <div class="form-grid">
          <el-form-item label="Bucket"><el-input v-model="configForm.bucket" /></el-form-item>
          <el-form-item label="Region"><el-input v-model="configForm.region" placeholder="可选" /></el-form-item>
        </div>
        <div class="form-grid">
          <el-form-item label="Access Key"><el-input v-model="configForm.accessKey" autocomplete="off" :placeholder="editingConfigId ? '留空则不修改' : '请输入 Access Key'" /></el-form-item>
          <el-form-item label="Secret Key"><el-input v-model="configForm.secretKey" type="password" show-password autocomplete="new-password" :placeholder="editingConfigId ? '留空则不修改' : '请输入 Secret Key'" /></el-form-item>
        </div>
        <div class="switch-row">
          <el-checkbox v-model="configForm.useSSL">使用 HTTPS</el-checkbox>
          <el-checkbox v-model="configForm.forcePathStyle">Path Style</el-checkbox>
          <el-checkbox v-model="configForm.isDefault">设为默认</el-checkbox>
          <el-switch v-model="configEnabled" active-text="启用" inactive-text="停用" />
        </div>
      </el-form>
      <template #footer><el-button @click="configDialogVisible = false">取消</el-button><el-button type="primary" :loading="savingConfig" @click="saveConfig">保存</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Plus, Refresh, Search } from "@element-plus/icons-vue";
import {
  createStorageConfig, deleteStorageConfig, deleteStorageFile, getStorageFileDownloadURL, getStorageOverview,
  listStorageConfigs, listStorageFiles, listStorageQuotas, permanentlyDeleteStorageFile, restoreStorageFile,
  testStorageConfig, updateStorageConfig, updateStorageQuota,
  type FileObject, type StorageConfig, type StorageConfigMutation, type StorageOverview
} from "../../api/storage";

const activeTab = ref("overview");
const loading = ref(false);
const configs = ref<StorageConfig[]>([]);
const files = ref<FileObject[]>([]);
const fileTotal = ref(0);
const filePage = ref(1);
const filePageSize = 30;
const fileQuery = ref("");
const fileStatus = ref("");
const overview = ref<StorageOverview>({ totalFiles: 0, totalBytes: 0, pendingFiles: 0, recycleFiles: 0, abnormalFiles: 0, temporaryBytes: 0, providerBytes: {}, quota: { tenantId: "", quotaBytes: 0, usedBytes: 0, reservedBytes: 0, fileCount: 0, warningPercent: 80, criticalPercent: 95 } });
const testingId = ref("");
const configDialogVisible = ref(false);
const editingConfigId = ref("");
const savingConfig = ref(false);
const savingQuota = ref(false);
const quotaForm = reactive({ tenantId: "", quotaGB: 10, warningPercent: 80, criticalPercent: 95 });

const providerOptions = [
  { label: "MinIO", value: "minio" }, { label: "通用 S3", value: "s3" }, { label: "阿里云 OSS", value: "aliyun_oss" },
  { label: "腾讯云 COS", value: "tencent_cos" }, { label: "华为云 OBS", value: "huawei_obs" }, { label: "Cloudflare R2", value: "cloudflare_r2" }
];

const emptyConfig = (): StorageConfigMutation => ({ name: "", provider: "minio", endpoint: "http://minio:9000", signingEndpoint: "http://localhost:9000", region: "", bucket: "xianzhi-assets", accessKey: "", secretKey: "", useSSL: false, forcePathStyle: true, isDefault: false, status: "ENABLED" });
const configForm = reactive<StorageConfigMutation>(emptyConfig());
const configEnabled = computed({ get: () => configForm.status === "ENABLED", set: (value: boolean) => { configForm.status = value ? "ENABLED" : "DISABLED"; } });

const quotaPercent = computed(() => {
  const quota = overview.value.quota?.quotaBytes || 0;
  if (quota <= 0) return 0;
  return Math.min(100, Math.round(((overview.value.quota.usedBytes + overview.value.quota.reservedBytes) / quota) * 100));
});
const providerUsage = computed(() => {
  const entries = Object.entries(overview.value.providerBytes || {});
  const total = entries.reduce((sum, [, bytes]) => sum + Number(bytes || 0), 0);
  return entries.map(([provider, bytes]) => ({ provider, bytes: Number(bytes), percent: total ? Math.round(Number(bytes) * 100 / total) : 0 }));
});

async function loadAll() {
  loading.value = true;
  try {
    const [overviewData, configData, quotaData] = await Promise.all([getStorageOverview(), listStorageConfigs(), listStorageQuotas()]);
    overview.value = overviewData;
    configs.value = configData.items || [];
    const quota = quotaData.items?.[0] || overviewData.quota;
    if (quota) {
      quotaForm.tenantId = quota.tenantId;
      quotaForm.quotaGB = Math.round((quota.quotaBytes || 0) / 1073741824);
      quotaForm.warningPercent = quota.warningPercent || 80;
      quotaForm.criticalPercent = quota.criticalPercent || 95;
    }
    await loadFiles();
  } catch (error) {
    ElMessage.error(errorMessage(error));
  } finally {
    loading.value = false;
  }
}

async function loadFiles() {
  try {
    const data = await listStorageFiles({ page: filePage.value, pageSize: filePageSize, q: fileQuery.value || undefined, status: fileStatus.value || undefined });
    files.value = data.items || [];
    fileTotal.value = data.total || 0;
  } catch (error) {
    ElMessage.error(errorMessage(error));
  }
}

function openCreateConfig() {
  editingConfigId.value = "";
  Object.assign(configForm, emptyConfig());
  configDialogVisible.value = true;
}

function openEditConfig(item: StorageConfig) {
  editingConfigId.value = item.id;
  Object.assign(configForm, { tenantId: item.tenantId, name: item.name, provider: item.provider, endpoint: item.endpoint, signingEndpoint: item.signingEndpoint || "", region: item.region || "", bucket: item.bucket, accessKey: "", secretKey: "", publicDomain: item.publicDomain || "", cdnDomain: item.cdnDomain || "", useSSL: item.useSSL, forcePathStyle: item.forcePathStyle, isDefault: item.isDefault, status: item.status });
  configDialogVisible.value = true;
}

async function saveConfig() {
  if (!configForm.name || !configForm.endpoint || !configForm.bucket) {
    ElMessage.warning("请填写配置名称、Endpoint 和 Bucket"); return;
  }
  savingConfig.value = true;
  try {
    if (editingConfigId.value) await updateStorageConfig(editingConfigId.value, { ...configForm });
    else await createStorageConfig({ ...configForm });
    ElMessage.success("存储配置已保存");
    configDialogVisible.value = false;
    await loadAll();
  } catch (error) { ElMessage.error(errorMessage(error)); }
  finally { savingConfig.value = false; }
}

async function runConnectionTest(item: StorageConfig) {
  testingId.value = item.id;
  try { await testStorageConfig(item.id); ElMessage.success("连接成功，Bucket 可用"); await loadAll(); }
  catch (error) { ElMessage.error(errorMessage(error)); }
  finally { testingId.value = ""; }
}

async function removeConfig(item: StorageConfig) {
  try {
    await ElMessageBox.confirm(`确认删除存储配置“${item.name}”？仍被文件引用时后端会阻止删除。`, "删除存储配置", { type: "warning" });
    await deleteStorageConfig(item.id); ElMessage.success("已删除"); await loadAll();
  } catch (error) { if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error)); }
}

async function downloadFile(item: FileObject) {
  try { const data = await getStorageFileDownloadURL(item.fileId); window.open(data.url, "_blank", "noopener,noreferrer"); }
  catch (error) { ElMessage.error(errorMessage(error)); }
}

async function moveToRecycle(item: FileObject) {
  try { await ElMessageBox.confirm(`将“${item.originalName}”移入回收站？`, "删除文件", { type: "warning" }); await deleteStorageFile(item.fileId); ElMessage.success("已移入回收站"); await loadAll(); }
  catch (error) { if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error)); }
}

async function restoreFile(item: FileObject) { try { await restoreStorageFile(item.fileId); ElMessage.success("文件已恢复"); await loadAll(); } catch (error) { ElMessage.error(errorMessage(error)); } }
async function permanentDelete(item: FileObject) {
  try { await ElMessageBox.confirm(`永久删除“${item.originalName}”？对象存储中的文件将被物理删除，此操作不可恢复。`, "永久删除", { type: "error", confirmButtonText: "永久删除" }); await permanentlyDeleteStorageFile(item.fileId); ElMessage.success("文件已永久删除"); await loadAll(); }
  catch (error) { if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error)); }
}

async function saveQuota() {
  if (!quotaForm.tenantId) { ElMessage.warning("未识别当前租户"); return; }
  savingQuota.value = true;
  try { await updateStorageQuota(quotaForm.tenantId, { quotaBytes: quotaForm.quotaGB * 1073741824, warningPercent: quotaForm.warningPercent, criticalPercent: quotaForm.criticalPercent }); ElMessage.success("容量策略已保存"); await loadAll(); }
  catch (error) { ElMessage.error(errorMessage(error)); }
  finally { savingQuota.value = false; }
}

function providerLabel(provider: string) { return providerOptions.find((item) => item.value === provider)?.label || provider || "-"; }
function formatBytes(value: number) { if (!value) return "0 B"; const units = ["B", "KB", "MB", "GB", "TB"]; const index = Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024))); return `${(value / Math.pow(1024, index)).toFixed(index > 1 ? 2 : 0)} ${units[index]}`; }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "-"; }
function fileStatusType(status: string) { if (status === "ACTIVE") return "success"; if (status === "DELETE_PENDING") return "warning"; if (status.includes("FAILED") || status === "QUARANTINED") return "danger"; return "info"; }
function errorMessage(error: unknown) { const value = error as { response?: { data?: { error?: string; message?: string } }; message?: string }; return value?.response?.data?.error || value?.response?.data?.message || value?.message || "操作失败"; }

onMounted(loadAll);
</script>

<style scoped>
.storage-center { display: grid; gap: 18px; }
.storage-hero { display: flex; justify-content: space-between; align-items: flex-start; padding: 24px 28px; border: 1px solid #dbe7f5; border-radius: 18px; background: linear-gradient(135deg, #f8fbff, #eef5ff); }
.storage-hero h2 { margin: 12px 0 6px; font-size: 26px; color: #17243b; }
.storage-hero p { margin: 0; color: #66758d; max-width: 720px; line-height: 1.7; }
.storage-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
.storage-metrics article { display: grid; gap: 6px; padding: 18px 20px; border: 1px solid #e3e9f2; border-radius: 14px; background: #fff; }
.storage-metrics span, .storage-metrics small { color: #7a8799; }.storage-metrics strong { font-size: 26px; color: #1d2b43; }.storage-metrics .danger strong { color: #d94a4a; }
.storage-tabs { padding: 18px 22px; border: 1px solid #e3e9f2; border-radius: 16px; background: #fff; }
.overview-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }.quota-summary { display: grid; gap: 22px; padding: 6px; }
.quota-summary dl { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin: 0; }.quota-summary dl div { padding: 12px; border-radius: 10px; background: #f6f8fb; }.quota-summary dt { color: #8290a3; }.quota-summary dd { margin: 8px 0 0; font-weight: 700; color: #26354d; }
.provider-usage { display: grid; gap: 18px; }.provider-usage > div { display: grid; grid-template-columns: 1fr auto; gap: 8px; }.provider-usage .el-progress { grid-column: 1 / -1; }
.table-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 14px; }.table-toolbar > div:first-child { display: grid; gap: 4px; }.table-toolbar span { color: #8390a3; font-size: 13px; }
.filters { display: flex !important; grid-auto-flow: column; grid-template-columns: minmax(240px, 1fr) 160px auto; align-items: center; }.filters .el-input { width: 280px; }.filters .el-select { width: 160px; }
.primary-cell { display: grid; gap: 4px; }.primary-cell small { color: #8a96a8; overflow: hidden; text-overflow: ellipsis; }.el-pagination { justify-content: flex-end; margin-top: 16px; }
.quota-card { max-width: 680px; }.quota-card .el-input-number { width: 220px; }.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }.switch-row { display: flex; flex-wrap: wrap; gap: 18px; align-items: center; padding: 8px 0; }
@media (max-width: 1000px) { .storage-metrics { grid-template-columns: repeat(2, 1fr); }.overview-grid { grid-template-columns: 1fr; } }
@media (max-width: 640px) { .storage-hero { padding: 18px; }.storage-metrics { grid-template-columns: 1fr; }.table-toolbar, .file-toolbar, .filters { display: grid !important; grid-template-columns: 1fr; }.filters .el-input, .filters .el-select { width: 100%; }.form-grid { grid-template-columns: 1fr; } }
</style>
