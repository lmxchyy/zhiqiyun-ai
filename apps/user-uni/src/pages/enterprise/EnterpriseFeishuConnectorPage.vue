<template>
  <EnterprisePageShell title="飞书企业连接">
    <view class="enterprise-card connector-status-card">
      <view><text class="enterprise-title">飞书自建应用机器人</text><text class="enterprise-copy">{{ statusCopy }}</text></view>
      <text :class="['connector-status', connector?.status || 'disabled']">{{ statusLabel }}</text>
    </view>

    <view class="enterprise-filters connector-tabs">
      <button v-for="item in tabs" :key="item.key" :class="['enterprise-chip', { active: tab === item.key }]" @click="selectTab(item.key)">{{ item.label }}</button>
    </view>

    <EnterpriseStatePanel v-if="loading" kind="loading" />
    <EnterpriseStatePanel v-else-if="errorMessage" kind="error" :copy="errorMessage" action-label="重试" @action="loadConnector" />

    <template v-else-if="tab === 'config'">
      <view class="enterprise-section enterprise-form">
        <view><text class="enterprise-field-label">连接名称</text><input v-model="draft.connectorName" class="enterprise-input" maxlength="60" placeholder="例如：知启云生图机器人" /></view>
        <view><text class="enterprise-field-label">App ID</text><input v-model="draft.appId" class="enterprise-input" maxlength="100" placeholder="cli_xxxxxxxxx" /></view>
        <view><text class="enterprise-field-label">App Secret</text><input v-model="draft.appSecret" class="enterprise-input" password maxlength="200" :placeholder="secretPlaceholder('appSecret')" /></view>
        <view><text class="enterprise-field-label">Verification Token</text><input v-model="draft.verificationToken" class="enterprise-input" password maxlength="300" :placeholder="secretPlaceholder('verificationToken')" /></view>
        <view><text class="enterprise-field-label">Encrypt Key（可选）</text><input v-model="draft.encryptKey" class="enterprise-input" password maxlength="300" :placeholder="secretPlaceholder('encryptKey')" /></view>
        <view v-if="connector"><text class="enterprise-field-label">事件回调地址</text><button class="connector-copy-field" @click="copy(connector.callbackUrl)"><text>{{ connector.callbackUrl }}</text><text class="connector-copy">复制</text></button></view>
        <view v-if="connector"><text class="enterprise-field-label">Connector Key</text><button class="connector-copy-field" @click="copy(connector.connectorKey)"><text>{{ connector.connectorKey }}</text><text class="connector-copy">复制</text></button></view>
        <text class="connector-note">密钥留空表示不修改；服务端只返回配置状态，不回传密钥原文。</text>
      </view>
      <view class="connector-actions">
        <button class="enterprise-primary-button" :disabled="submitting || !canManage" @click="save">{{ submitting ? '保存中…' : '保存配置' }}</button>
        <button class="connector-secondary" :disabled="submitting || !connector || !canManage" @click="testConnection">测试连接</button>
        <button v-if="connector?.status !== 'active'" class="connector-secondary" :disabled="submitting || !connector || !canManage" @click="setEnabled(true)">启用</button>
        <button v-else class="connector-danger" :disabled="submitting || !canManage" @click="setEnabled(false)">停用</button>
      </view>
    </template>

    <template v-else-if="tab === 'permission'">
      <view class="enterprise-section enterprise-form connector-switches">
        <view class="connector-switch-row"><view><text class="enterprise-field-label">开启 AI 生图</text><text class="enterprise-copy">允许飞书成员创建单轮图片任务</text></view><switch :checked="draft.config.aiImageEnabled" :disabled="!canManage" @change="draft.config.aiImageEnabled = switchValue($event)" /></view>
        <view><text class="enterprise-field-label">默认生图模型</text><input v-model="draft.config.defaultImageModel" class="enterprise-input" placeholder="留空使用平台默认模型" /></view>
        <view><text class="enterprise-field-label">默认尺寸</text><picker :range="sizes" @change="draft.config.defaultSize = sizes[Number($event.detail.value)]"><view class="enterprise-select">{{ draft.config.defaultSize }} ›</view></picker></view>
        <view><text class="enterprise-field-label">默认图片数量</text><input v-model.number="draft.config.defaultImageCount" class="enterprise-input" type="number" /></view>
        <view><text class="enterprise-field-label">每个成员每日额度</text><input v-model.number="draft.config.memberDailyQuota" class="enterprise-input" type="number" /></view>
        <view class="connector-switch-row"><view><text class="enterprise-field-label">允许群聊</text><text class="enterprise-copy">允许在飞书群内调用机器人</text></view><switch :checked="draft.config.allowGroupChat" :disabled="!canManage" @change="draft.config.allowGroupChat = switchValue($event)" /></view>
        <view class="connector-switch-row"><view><text class="enterprise-field-label">群聊必须 @机器人</text><text class="enterprise-copy">防止普通群消息误触发扣费任务</text></view><switch :checked="draft.config.groupRequireMention" :disabled="!canManage" @change="draft.config.groupRequireMention = switchValue($event)" /></view>
      </view>
      <button class="enterprise-primary-button connector-save-permission" :disabled="submitting || !connector || !canManage" @click="save">保存 AI 权限</button>
    </template>

    <template v-else-if="tab === 'users'">
      <view v-if="users.length" class="enterprise-list">
        <view v-for="item in users" :key="item.id" class="enterprise-card connector-user-card">
          <view class="connector-avatar">飞</view><view class="connector-grow"><text class="enterprise-list-title">{{ item.externalName }}</text><text class="enterprise-list-meta">{{ item.internalUserName || '未绑定内部用户' }} · {{ item.organizationName || '未分配部门' }}</text><text class="enterprise-list-meta">今日 {{ item.dailyUsage }} / {{ item.dailyQuota }} 次 · {{ formatTime(item.lastActiveAt) }}</text></view>
          <switch :checked="item.status === 'active' && item.permission.imageGenerate !== false" :disabled="!canManage" @change="toggleUser(item, switchValue($event))" />
        </view>
      </view>
      <EnterpriseStatePanel v-else kind="empty" title="暂无飞书成员" copy="成员首次向机器人发送消息后会自动建立企业成员映射" />
    </template>

    <template v-else>
      <view v-if="tasks.length" class="enterprise-list">
        <view v-for="item in tasks" :key="item.id" class="enterprise-card connector-task-card">
          <view class="connector-task-head"><text class="enterprise-list-title">{{ item.originalText || item.intent }}</text><text :class="['connector-status', item.status]">{{ taskStatus(item.status) }}</text></view>
          <text class="enterprise-list-meta">{{ item.externalUserName || item.externalUserId || '未知飞书用户' }} · {{ item.modelId || '平台默认模型' }} · 消耗 {{ item.pointsCost }} 点</text>
          <text class="enterprise-list-meta">{{ formatTime(item.createdAt) }}</text>
          <text v-if="item.errorMessage" class="connector-error">{{ item.errorMessage }}</text>
        </view>
      </view>
      <EnterpriseStatePanel v-else kind="empty" title="暂无任务记录" copy="飞书生图任务会显示在这里，并同步保存到作品中心" />
      <view v-if="logs.length" class="enterprise-section"><text class="enterprise-section-title">最近消息日志</text><view class="enterprise-card"><text v-for="item in logs.slice(0, 8)" :key="item.id" class="connector-log">{{ item.direction === 'inbound' ? '接收' : '发送' }} · {{ item.messageType }} · {{ item.processingStatus }} · {{ formatTime(item.createdAt) }}</text></view></view>
    </template>
  </EnterprisePageShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import EnterprisePageShell from "../../components/enterprise/EnterprisePageShell.vue";
import EnterpriseStatePanel from "../../components/enterprise/EnterpriseStatePanel.vue";
import { enterpriseAPI } from "../../features/enterprise/api";
import type { ConnectorAITask, ConnectorMessageLog, ConnectorUserBinding, EnterpriseConnector, EnterpriseConnectorConfig } from "../../features/enterprise/types";
import { useUserStore } from "../../stores/user";

type Tab = "config" | "permission" | "users" | "tasks";
const tabs: Array<{ key: Tab; label: string }> = [{ key: "config", label: "连接配置" }, { key: "permission", label: "AI权限" }, { key: "users", label: "成员" }, { key: "tasks", label: "任务" }];
const sizes = ["1024x1024", "1024x1536", "1536x1024"];
const defaults: EnterpriseConnectorConfig = { aiImageEnabled: true, defaultImageModel: "", defaultSize: "1024x1024", defaultImageCount: 1, memberDailyQuota: 20, allowGroupChat: true, groupRequireMention: true };
const userStore = useUserStore();
const connector = ref<EnterpriseConnector | null>(null);
const users = ref<ConnectorUserBinding[]>([]);
const tasks = ref<ConnectorAITask[]>([]);
const logs = ref<ConnectorMessageLog[]>([]);
const tab = ref<Tab>("config");
const loading = ref(true);
const submitting = ref(false);
const errorMessage = ref("");
const draft = reactive({ connectorName: "飞书机器人", appId: "", appSecret: "", verificationToken: "", encryptKey: "", config: { ...defaults } });
const permissions = computed(() => userStore.currentContext?.permissions || []);
const canManage = computed(() => permissions.value.includes("enterprise.connector.manage"));
const statusLabel = computed(() => ({ active: "已启用", error: "连接异常", disabled: "未启用" }[connector.value?.status || "disabled"]));
const statusCopy = computed(() => connector.value?.lastErrorMessage || (connector.value?.lastConnectedAt ? `最近连接 ${formatTime(connector.value.lastConnectedAt)}` : "配置企业自建应用后即可在飞书中发起 AI 生图"));

onMounted(async () => {
  try {
    await userStore.loadProfile(true);
    await userStore.loadEnterpriseContexts();
  }
  catch (error) {
    errorMessage.value = message(error, "企业身份加载失败");
    loading.value = false;
    return;
  }
  await loadConnector();
});

async function loadConnector() {
  loading.value = true; errorMessage.value = "";
  try {
    const result = await enterpriseAPI.feishuConnector();
    if ("configured" in result && result.configured === false) { connector.value = null; Object.assign(draft.config, defaults, result.config || {}); }
    else { connector.value = result as EnterpriseConnector; hydrate(connector.value); }
  } catch (error) { errorMessage.value = message(error, "飞书连接配置加载失败"); }
  finally { loading.value = false; }
}

function hydrate(value: EnterpriseConnector) { draft.connectorName = value.connectorName; draft.appId = value.appId; draft.appSecret = ""; draft.verificationToken = ""; draft.encryptKey = ""; Object.assign(draft.config, defaults, value.config); }
async function selectTab(value: Tab) { tab.value = value; if (value === "users" && connector.value) await loadUsers(); if (value === "tasks" && connector.value) await loadTasks(); }
async function save() { if (!canManage.value) return; if (!draft.appId.trim()) { uni.showToast({ title: "请输入 App ID", icon: "none" }); return; } submitting.value = true; try { connector.value = await enterpriseAPI.saveFeishuConnector(Boolean(connector.value), { connectorName: draft.connectorName.trim(), appId: draft.appId.trim(), appSecret: draft.appSecret.trim() || undefined, verificationToken: draft.verificationToken.trim() || undefined, encryptKey: draft.encryptKey.trim() || undefined, config: { ...draft.config } }); hydrate(connector.value); uni.showToast({ title: "配置已保存", icon: "success" }); } catch (error) { uni.showToast({ title: message(error, "保存失败"), icon: "none" }); } finally { submitting.value = false; } }
async function testConnection() { submitting.value = true; try { const result = await enterpriseAPI.testFeishuConnector(); connector.value = result.connector; uni.showToast({ title: "连接成功", icon: "success" }); } catch (error) { uni.showToast({ title: message(error, "连接失败"), icon: "none" }); await loadConnector(); } finally { submitting.value = false; } }
async function setEnabled(enabled: boolean) { submitting.value = true; try { const result = await enterpriseAPI.setFeishuConnectorEnabled(enabled); connector.value = result.connector; uni.showToast({ title: enabled ? "已启用" : "已停用", icon: "success" }); } catch (error) { uni.showToast({ title: message(error, "状态更新失败"), icon: "none" }); } finally { submitting.value = false; } }
async function loadUsers() { try { users.value = (await enterpriseAPI.feishuUsers()).items; } catch (error) { uni.showToast({ title: message(error, "成员加载失败"), icon: "none" }); } }
async function loadTasks() { try { const [taskResult, logResult] = await Promise.all([enterpriseAPI.feishuTasks(), enterpriseAPI.feishuLogs()]); tasks.value = taskResult.items; logs.value = logResult.items; } catch (error) { uni.showToast({ title: message(error, "任务加载失败"), icon: "none" }); } }
async function toggleUser(item: ConnectorUserBinding, enabled: boolean) { try { const updated = await enterpriseAPI.updateFeishuUser(item.id, { permission: { ...item.permission, imageGenerate: enabled }, status: enabled ? "active" : "disabled" }); users.value = users.value.map(value => value.id === item.id ? updated : value); } catch (error) { uni.showToast({ title: message(error, "成员权限更新失败"), icon: "none" }); await loadUsers(); } }
function secretPlaceholder(key: "appSecret" | "verificationToken" | "encryptKey") { return connector.value?.secretsConfigured[key] ? "已配置，留空不修改" : key === "encryptKey" ? "未配置（可选）" : "请输入"; }
function switchValue(event: unknown) { return Boolean((event as { detail?: { value?: boolean } })?.detail?.value); }
function copy(value: string) { uni.setClipboardData({ data: value, success: () => uni.showToast({ title: "已复制", icon: "success" }) }); }
function formatTime(value?: string) { if (!value) return "尚未活跃"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false }); }
function taskStatus(value: string) { return ({ pending: "等待中", processing: "生成中", succeeded: "已完成", failed: "失败", ignored: "已忽略" } as Record<string, string>)[value] || value; }
function message(error: unknown, fallback: string) { return error instanceof Error && error.message ? error.message : fallback; }
</script>

<style src="../../styles/enterprise-center.css"></style>
<style scoped>
.connector-status-card,.connector-user-card,.connector-task-head,.connector-switch-row,.connector-copy-field,.connector-actions{display:flex;align-items:center}.connector-status-card,.connector-task-head,.connector-switch-row,.connector-copy-field{justify-content:space-between}.connector-tabs{overflow-x:auto}.connector-status{padding:4px 9px;border-radius:999px;font-size:12px;color:#667085;background:#f2f4f7;white-space:nowrap}.connector-status.active,.connector-status.succeeded{color:#067647;background:#ecfdf3}.connector-status.error,.connector-status.failed{color:#b42318;background:#fef3f2}.connector-status.processing{color:#175cd3;background:#eff8ff}.connector-note{font-size:12px;line-height:1.6;color:#667085}.connector-copy-field{width:100%;border:1px solid #e4e7ec;border-radius:10px;background:#fff;padding:11px 12px;text-align:left;gap:10px}.connector-copy-field text:first-child{max-width:78%;font-size:12px;color:#344054;word-break:break-all}.connector-copy{color:#7f56d9;font-size:13px}.connector-actions{gap:8px;flex-wrap:wrap;padding:0 16px 24px}.connector-actions .enterprise-primary-button{flex:1;min-width:130px;margin:0}.connector-secondary,.connector-danger{height:42px;line-height:42px;padding:0 14px;border-radius:10px;font-size:13px}.connector-secondary{color:#344054;background:#fff;border:1px solid #d0d5dd}.connector-danger{color:#b42318;background:#fff1f0;border:1px solid #fecdca}.connector-switches{gap:16px}.connector-switch-row{gap:16px}.connector-switch-row>view{display:flex;flex-direction:column;gap:4px}.connector-save-permission{margin:16px}.connector-user-card{gap:12px}.connector-avatar{display:flex;align-items:center;justify-content:center;width:38px;height:38px;border-radius:10px;color:#fff;background:#3370ff;font-weight:700}.connector-grow{display:flex;flex:1;min-width:0;flex-direction:column;gap:3px}.connector-task-card{display:flex;flex-direction:column;gap:8px}.connector-error{font-size:12px;line-height:1.5;color:#b42318}.connector-log{display:block;padding:7px 0;border-bottom:1px solid #f2f4f7;font-size:12px;color:#667085}
</style>
