<template>
  <section class="enterprise-integration-center">
    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="主控端仅展示连接状态与密钥配置状态；凭据、Connector Key、外部消息和任务内容不会在此返回。"
    />
    <div class="integration-summary-grid">
      <article><span>支持平台</span><strong>{{ value('supported') }}</strong><small>统一 PlatformConnector 边界</small></article>
      <article><span>已配置</span><strong>{{ value('configured') }}</strong><small>由企业管理员维护凭据</small></article>
      <article><span>已启用</span><strong>{{ value('active') }}</strong><small>当前可接收外部事件</small></article>
      <article><span>异常</span><strong>{{ value('errors') }}</strong><small>仅显示异常状态，不返回敏感错误正文</small></article>
    </div>
    <div class="integration-card-grid">
      <article v-for="item in items" :key="String(item.connectorType)" class="integration-card" :class="`is-${String(item.status || 'unconfigured').toLowerCase()}`">
        <header>
          <span class="integration-mark">{{ platformMark(String(item.connectorType)) }}</span>
          <div><h3>{{ item.platformName || item.connectorName }}</h3><small>{{ item.connectorName }}</small></div>
          <el-tag :type="statusType(item.status)">{{ statusLabel(item.status) }}</el-tag>
        </header>
        <dl>
          <div><dt>适配器边界</dt><dd><code>{{ item.adapterBoundary || 'PlatformConnector' }}</code></dd></div>
          <div><dt>App Secret</dt><dd>{{ secretState(item, 'appSecret') }}</dd></div>
          <div><dt>Verification Token</dt><dd>{{ secretState(item, 'verificationToken') }}</dd></div>
          <div><dt>Encrypt Key</dt><dd>{{ secretState(item, 'encryptKey', true) }}</dd></div>
          <div><dt>最近连接</dt><dd>{{ formatTime(item.lastConnectedAt) }}</dd></div>
        </dl>
        <footer><span>{{ item.configured ? '企业侧已配置' : '等待企业管理员配置' }}</span><small v-if="item.hasError">检测到连接异常</small></footer>
      </article>
    </div>
    <p class="integration-boundary-note">飞书、钉钉、企业微信和微信开放平台均通过统一 Connector 抽象接入；平台 SDK 类型不得进入 AI 生图核心业务。</p>
  </section>
</template>

<script setup lang="ts">
type RecordValue = Record<string, any>;
const props = defineProps<{ items: RecordValue[]; summary: RecordValue }>();

function value(key: string) { return String(props.summary[key] ?? 0); }
function platformMark(key: string) { return ({ feishu: "飞", dingtalk: "钉", wecom: "企", wechat: "微" } as Record<string, string>)[key] || "连"; }
function statusLabel(value: unknown) { return ({ active: "已启用", disabled: "已停用", error: "异常", unconfigured: "未配置" } as Record<string, string>)[String(value || "").toLowerCase()] || String(value || "未知"); }
function statusType(value: unknown) { const status = String(value || "").toLowerCase(); return status === "active" ? "success" : status === "error" ? "danger" : status === "unconfigured" ? "info" : "warning"; }
function secretState(item: RecordValue, key: string, optional = false) { return item.secretsConfigured?.[key] ? "已加密配置" : optional ? "未配置（可选）" : "未配置"; }
function formatTime(value: unknown) { if (!value) return "暂无连接记录"; const date = new Date(String(value)); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN", { hour12: false }); }
</script>

<style scoped>
.enterprise-integration-center{display:grid;gap:18px}.integration-summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px}.integration-summary-grid article{display:grid;gap:5px;padding:18px;border:1px solid var(--admin-border);border-radius:12px;background:var(--admin-panel)}.integration-summary-grid span,.integration-summary-grid small{color:var(--admin-muted)}.integration-summary-grid strong{font-size:26px;color:var(--admin-text)}.integration-card-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.integration-card{padding:20px;border:1px solid var(--admin-border);border-radius:14px;background:var(--admin-panel)}.integration-card header{display:flex;align-items:center;gap:12px}.integration-card header div{min-width:0;flex:1}.integration-card h3{margin:0 0 3px}.integration-card header small{color:var(--admin-muted)}.integration-mark{display:grid;width:44px;height:44px;place-items:center;border-radius:12px;background:var(--color-primary-light);color:var(--color-primary);font-weight:800}.integration-card dl{display:grid;gap:9px;margin:18px 0}.integration-card dl div{display:flex;justify-content:space-between;gap:16px}.integration-card dt{color:var(--admin-muted)}.integration-card dd{margin:0;text-align:right}.integration-card footer{display:flex;justify-content:space-between;color:var(--admin-muted)}.integration-card footer small{color:var(--el-color-danger)}.integration-boundary-note{margin:0;padding:14px 16px;border-radius:10px;background:var(--color-primary-light);color:var(--admin-muted)}@media(max-width:900px){.integration-summary-grid,.integration-card-grid{grid-template-columns:1fr 1fr}}@media(max-width:620px){.integration-summary-grid,.integration-card-grid{grid-template-columns:1fr}}
</style>
