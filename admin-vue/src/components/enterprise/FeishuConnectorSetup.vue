<template>
  <main class="connector-page">
    <el-card class="connector-card" shadow="never">
      <template #header>
        <div class="connector-head">
          <div>
            <el-tag type="primary" effect="dark">企业连接器</el-tag>
            <h1>飞书自建应用机器人</h1>
            <p>凭据仅发送到当前知启云服务并加密保存，页面不会回显密钥原文。</p>
          </div>
          <el-tag :type="statusType">{{ statusLabel }}</el-tag>
        </div>
      </template>

      <el-alert v-if="errorMessage" :title="errorMessage" type="error" show-icon :closable="false" />
      <el-form label-position="top" :model="draft" class="connector-form">
        <el-form-item label="连接名称"><el-input v-model.trim="draft.connectorName" placeholder="知启云 AI 飞书机器人" /></el-form-item>
        <el-form-item label="App ID"><el-input v-model.trim="draft.appId" placeholder="cli_xxxxxxxxx" /></el-form-item>
        <el-form-item label="默认生图模型">
          <el-select v-model="draft.defaultImageModel" style="width: 100%">
            <el-option label="GPT Image 2（真实生图）" value="gpt-image-2" />
            <el-option label="本地演示模型（仅生成记录）" value="mock-standard" />
          </el-select>
        </el-form-item>
        <el-form-item label="App Secret"><el-input v-model="draft.appSecret" type="password" show-password autocomplete="new-password" :placeholder="secretPlaceholder('appSecret')" /></el-form-item>
        <el-form-item label="Verification Token"><el-input v-model="draft.verificationToken" type="password" show-password autocomplete="new-password" :placeholder="secretPlaceholder('verificationToken')" /></el-form-item>
        <el-form-item label="Encrypt Key（可选）"><el-input v-model="draft.encryptKey" type="password" show-password autocomplete="new-password" :placeholder="secretPlaceholder('encryptKey')" /></el-form-item>
        <el-form-item v-if="connector" label="事件回调地址"><el-input :model-value="connector.callbackUrl" readonly /></el-form-item>
        <el-form-item v-if="connector" label="Connector Key"><el-input :model-value="connector.connectorKey" readonly /></el-form-item>
      </el-form>

      <div class="connector-actions">
        <el-button v-if="errorMessage === 'forbidden'" type="warning" :loading="submitting" @click="initializeEnterprise">初始化企业接入空间</el-button>
        <el-button type="primary" :loading="submitting" @click="save">保存配置</el-button>
        <el-button :disabled="!connector" :loading="submitting" @click="testConnection">测试连接</el-button>
        <el-button v-if="connector?.status !== 'active'" type="success" :disabled="!connector" :loading="submitting" @click="enable">启用机器人</el-button>
        <el-button @click="back">返回用户工作台</el-button>
      </div>
    </el-card>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { adminRequest } from "../../api/client";

interface ConnectorView {
  status: string;
  connectorName: string;
  connectorKey: string;
  appId: string;
  callbackUrl: string;
  lastErrorMessage?: string;
  config: { defaultImageModel?: string };
  secretsConfigured: { appSecret: boolean; verificationToken: boolean; encryptKey: boolean };
}

const connector = ref<ConnectorView | null>(null);
const submitting = ref(false);
const errorMessage = ref("");
const draft = reactive({ connectorName: "知启云 AI 飞书机器人", appId: "", appSecret: "", verificationToken: "", encryptKey: "", defaultImageModel: "gpt-image-2" });
const statusLabel = computed(() => connector.value?.status === "active" ? "已启用" : connector.value?.status === "error" ? "连接异常" : "未启用");
const statusType = computed(() => connector.value?.status === "active" ? "success" : connector.value?.status === "error" ? "danger" : "info");

onMounted(load);

async function load() {
  errorMessage.value = "";
  try {
    const result = await adminRequest<ConnectorView | { configured: false }>({ method: "GET", url: "/enterprise/connectors/feishu" });
    if ("configured" in result) return;
    connector.value = result;
    draft.connectorName = result.connectorName;
    draft.appId = result.appId;
    draft.defaultImageModel = result.config?.defaultImageModel || "gpt-image-2";
  } catch (error) {
    errorMessage.value = message(error, "飞书连接配置加载失败");
  }
}

async function initializeEnterprise() {
  submitting.value = true;
  try {
    await adminRequest({ method: "POST", url: "/enterprises", data: { name: "知启云 AI 飞书机器人企业" } });
    errorMessage.value = "";
    ElMessage.success("企业接入空间已创建，并已切换为企业管理员");
    await load();
  } catch (error) { ElMessage.error(message(error, "企业接入空间初始化失败")); }
  finally { submitting.value = false; }
}

async function save() {
  if (!draft.appId || (!connector.value && (!draft.appSecret || !draft.verificationToken))) {
    ElMessage.warning("请填写 App ID、App Secret 和 Verification Token");
    return;
  }
  submitting.value = true;
  try {
    connector.value = await adminRequest<ConnectorView>({
      method: connector.value ? "PUT" : "POST",
      url: "/enterprise/connectors/feishu",
      data: {
        connectorName: draft.connectorName,
        appId: draft.appId,
        appSecret: draft.appSecret || undefined,
        verificationToken: draft.verificationToken || undefined,
        encryptKey: draft.encryptKey || undefined,
        config: { aiImageEnabled: true, defaultImageModel: draft.defaultImageModel, defaultSize: "1024x1024", defaultImageCount: 1, memberDailyQuota: 20, allowGroupChat: true, groupRequireMention: true },
      },
    });
    draft.appSecret = ""; draft.verificationToken = ""; draft.encryptKey = "";
    ElMessage.success("飞书连接配置已加密保存");
  } catch (error) { ElMessage.error(message(error, "保存失败")); }
  finally { submitting.value = false; }
}

async function testConnection() {
  submitting.value = true;
  try {
    const result = await adminRequest<{ connector: ConnectorView }>({ method: "POST", url: "/enterprise/connectors/feishu/test", data: {} });
    connector.value = result.connector; ElMessage.success("飞书连接测试通过");
  } catch (error) { ElMessage.error(message(error, "连接测试失败")); await load(); }
  finally { submitting.value = false; }
}

async function enable() {
  submitting.value = true;
  try {
    const result = await adminRequest<{ connector: ConnectorView }>({ method: "POST", url: "/enterprise/connectors/feishu/enable", data: {} });
    connector.value = result.connector; ElMessage.success("飞书机器人已启用");
  } catch (error) { ElMessage.error(message(error, "启用失败")); }
  finally { submitting.value = false; }
}

function secretPlaceholder(key: keyof ConnectorView["secretsConfigured"]) { return connector.value?.secretsConfigured[key] ? "已配置，留空不修改" : key === "encryptKey" ? "未配置（可选）" : "请输入"; }
function message(error: unknown, fallback: string) { return error instanceof Error && error.message ? error.message : fallback; }
function back() { window.location.assign("/app"); }
</script>

<style scoped>
.connector-page{min-height:100vh;padding:36px;background:#f4f7fb}.connector-card{max-width:860px;margin:0 auto}.connector-head{display:flex;align-items:flex-start;justify-content:space-between;gap:24px}.connector-head h1{margin:12px 0 8px;font-size:28px;color:#182230}.connector-head p{margin:0;color:#667085}.connector-form{margin-top:20px}.connector-actions{display:flex;flex-wrap:wrap;gap:10px}@media(max-width:640px){.connector-page{padding:12px}.connector-head{flex-direction:column}.connector-actions .el-button{width:100%;margin-left:0}}
</style>
