<template>
  <main class="authorization-page">
    <section class="hero">
      <div>
        <el-tag effect="dark" type="primary">企业连接中心</el-tag>
        <h1>一个二维码，连接你的协作平台</h1>
        <p>二维码只包含一次性授权票据，5 分钟后自动失效。扫码后选择飞书、企业微信、钉钉或微信完成绑定。</p>
      </div>
      <div class="hero-actions">
        <el-button type="primary" size="large" :loading="creating" @click="createSession('universal')">生成统一二维码</el-button>
        <el-button size="large" @click="goBack">返回工作台</el-button>
      </div>
    </section>

    <el-alert v-if="errorMessage" class="page-alert" :title="errorMessage" type="error" show-icon :closable="false" />

    <section class="platform-grid" v-loading="loading">
      <article v-for="platform in platforms" :key="platform.key" class="platform-card" :class="{ unavailable: !platform.available }">
        <header>
          <span class="platform-mark" :class="`mark-${platform.key}`">{{ platformMark(platform.key) }}</span>
          <div>
            <h2>{{ platform.name }}</h2>
            <el-tag :type="statusType(platform)" size="small">{{ statusText(platform) }}</el-tag>
          </div>
        </header>
        <p>{{ platform.description }}</p>
        <div class="platform-mode">{{ modeText(platform.mode) }}</div>
        <footer>
          <el-button v-if="platform.available" type="primary" plain :loading="creatingPlatform === platform.key" @click="createSession(platform.key)">
            生成专属二维码
          </el-button>
          <el-button v-else-if="platform.key === 'feishu'" @click="configureFeishu">配置飞书应用</el-button>
          <span v-else class="prerequisite">{{ platform.prerequisite }}</span>
        </footer>
      </article>
    </section>

    <section class="security-panel">
      <h2>安全说明</h2>
      <ul>
        <li>企业与操作人由登录态在服务端固化，扫码参数不能指定或篡改租户。</li>
        <li>二维码票据只使用一次，数据库仅保存摘要，不保存二维码原文。</li>
        <li>平台访问令牌只用于本次身份校验，不落库；应用密钥始终加密保存。</li>
      </ul>
    </section>

    <el-dialog v-model="dialogVisible" width="min(520px, 94vw)" :close-on-click-modal="false" @closed="closeSession">
      <template #header>
        <div class="dialog-head">
          <strong>{{ currentTitle }}</strong>
          <el-tag :type="sessionTagType">{{ sessionStatusText }}</el-tag>
        </div>
      </template>
      <div v-if="session" class="qr-dialog">
        <template v-if="isWaiting">
          <img :src="session.qrCodeDataUrl" alt="平台授权二维码" />
          <h3>请使用对应应用扫码</h3>
          <p>{{ session.platform === 'universal' ? '扫码后选择要连接的平台' : `使用${platformName(session.platform)}扫码并确认授权` }}</p>
          <el-progress :percentage="remainingPercent" :show-text="false" :stroke-width="6" />
          <small>二维码将在 {{ remainingSeconds }} 秒后失效</small>
          <div class="qr-actions">
            <el-button @click="copyLink">复制授权链接</el-button>
            <el-button type="primary" @click="openAuthorization">在当前设备打开</el-button>
          </div>
        </template>
        <el-result v-else-if="session.status === 'AUTHORIZED'" icon="success" title="连接成功" :sub-title="successSubtitle" />
        <el-result v-else icon="warning" :title="sessionStatusText" :sub-title="session.errorMessage || '请关闭窗口后重新生成二维码。'" />
      </div>
    </el-dialog>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { adminRequest } from "../../api/client";

type PlatformKey = "feishu" | "wecom" | "dingtalk" | "wechat";

interface PlatformView {
  key: PlatformKey;
  name: string;
  available: boolean;
  configured: boolean;
  connected: boolean;
  mode: string;
  description: string;
  prerequisite?: string;
}

interface AuthorizationSession {
  id: string;
  platform: PlatformKey | "universal";
  status: "PENDING" | "AUTHORIZING" | "AUTHORIZED" | "FAILED" | "EXPIRED" | "CANCELLED";
  authorizationUrl?: string;
  qrCodeDataUrl?: string;
  externalUserName?: string;
  errorMessage?: string;
  expiresAt: string;
}

const platforms = ref<PlatformView[]>([]);
const loading = ref(true);
const creating = ref(false);
const creatingPlatform = ref("");
const errorMessage = ref("");
const dialogVisible = ref(false);
const session = ref<AuthorizationSession | null>(null);
const now = ref(Date.now());
let pollTimer: number | undefined;
let clockTimer: number | undefined;

const isWaiting = computed(() => session.value?.status === "PENDING" || session.value?.status === "AUTHORIZING");
const remainingSeconds = computed(() => Math.max(0, Math.ceil((Date.parse(session.value?.expiresAt || "") - now.value) / 1000)));
const remainingPercent = computed(() => Math.min(100, Math.max(0, Math.round((remainingSeconds.value / 300) * 100))));
const currentTitle = computed(() => session.value?.platform === "universal" ? "统一扫码连接" : `${platformName(session.value?.platform || "")}扫码连接`);
const sessionStatusText = computed(() => ({ PENDING: "等待扫码", AUTHORIZING: "授权中", AUTHORIZED: "已连接", FAILED: "连接失败", EXPIRED: "二维码已过期", CANCELLED: "已取消" }[session.value?.status || "PENDING"]));
const sessionTagType = computed(() => session.value?.status === "AUTHORIZED" ? "success" : session.value?.status === "FAILED" ? "danger" : session.value?.status === "EXPIRED" ? "warning" : "primary");
const successSubtitle = computed(() => session.value?.externalUserName ? `已绑定账号：${session.value.externalUserName}` : "平台账号已绑定到当前企业");

onMounted(() => {
  loadPlatforms();
  clockTimer = window.setInterval(() => {
    now.value = Date.now();
    if (remainingSeconds.value === 0 && isWaiting.value) pollSession();
  }, 1000);
});

onBeforeUnmount(() => {
  stopPolling();
  if (clockTimer) window.clearInterval(clockTimer);
});

async function loadPlatforms() {
  loading.value = true;
  errorMessage.value = "";
  try {
    const result = await adminRequest<{ items: PlatformView[] }>({ method: "GET", url: "/enterprise/connector-authorizations/platforms" });
    platforms.value = result.items;
  } catch (error) {
    errorMessage.value = message(error, "连接平台状态加载失败");
  } finally {
    loading.value = false;
  }
}

async function createSession(platform: PlatformKey | "universal") {
  creating.value = platform === "universal";
  creatingPlatform.value = platform === "universal" ? "" : platform;
  try {
    session.value = await adminRequest<AuthorizationSession>({ method: "POST", url: "/enterprise/connector-authorizations", data: { platform } });
    now.value = Date.now();
    dialogVisible.value = true;
    startPolling();
  } catch (error) {
    ElMessage.error(message(error, "二维码生成失败"));
  } finally {
    creating.value = false;
    creatingPlatform.value = "";
  }
}

function startPolling() {
  stopPolling();
  pollTimer = window.setInterval(pollSession, 1800);
}

function stopPolling() {
  if (pollTimer) window.clearInterval(pollTimer);
  pollTimer = undefined;
}

async function pollSession() {
  if (!session.value?.id || !isWaiting.value) return;
  try {
    const result = await adminRequest<AuthorizationSession>({ method: "GET", url: `/enterprise/connector-authorizations/${session.value.id}` });
    session.value = { ...session.value, ...result };
    if (!isWaiting.value) {
      stopPolling();
      if (result.status === "AUTHORIZED") {
        ElMessage.success("平台连接成功");
        await loadPlatforms();
      }
    }
  } catch {
    // A transient polling failure should not invalidate the QR code.
  }
}

async function closeSession() {
  stopPolling();
  const current = session.value;
  session.value = null;
  if (current && (current.status === "PENDING" || current.status === "AUTHORIZING")) {
    await adminRequest({ method: "POST", url: `/enterprise/connector-authorizations/${current.id}/cancel`, data: {} }).catch(() => undefined);
  }
}

async function copyLink() {
  if (!session.value?.authorizationUrl) return;
  await navigator.clipboard.writeText(session.value.authorizationUrl);
  ElMessage.success("授权链接已复制");
}

function openAuthorization() {
  if (session.value?.authorizationUrl) window.open(session.value.authorizationUrl, "_blank", "noopener,noreferrer");
}

function statusText(platform: PlatformView) { return platform.connected ? "已连接" : platform.available ? "可扫码" : "需配置"; }
function statusType(platform: PlatformView) { return platform.connected ? "success" : platform.available ? "primary" : "info"; }
function modeText(mode: string) { return mode === "oauth_user_binding" ? "企业应用 OAuth" : mode === "website_oauth" ? "微信开放平台 OAuth" : "第三方应用套件"; }
function platformMark(key: string) { return ({ feishu: "飞", wecom: "企", dingtalk: "钉", wechat: "微" } as Record<string, string>)[key] || "连"; }
function platformName(key: string) { return platforms.value.find((item) => item.key === key)?.name || ({ universal: "统一", feishu: "飞书", wecom: "企业微信", dingtalk: "钉钉", wechat: "微信" } as Record<string, string>)[key] || "平台"; }
function userConsolePath(path: string) { return window.location.pathname.startsWith("/workspace") ? `/workspace${path.slice(4)}` : path; }
function configureFeishu() { window.location.assign(userConsolePath("/app/enterprise/feishu")); }
function goBack() { window.location.assign(userConsolePath("/app")); }
function message(error: unknown, fallback: string) { return error instanceof Error && error.message ? error.message : fallback; }
</script>

<style scoped>
.authorization-page{min-height:100vh;padding:40px;background:linear-gradient(160deg,#f5f8ff 0%,#f7f9fc 45%,#eef8f6 100%);color:#182230}.hero{max-width:1120px;margin:0 auto 24px;display:flex;align-items:flex-end;justify-content:space-between;gap:32px}.hero h1{margin:14px 0 10px;font-size:34px;letter-spacing:-.03em}.hero p{max-width:720px;margin:0;color:#667085;font-size:16px}.hero-actions{display:flex;white-space:nowrap}.page-alert{max-width:1120px;margin:0 auto 20px}.platform-grid{max-width:1120px;min-height:220px;margin:0 auto;display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:16px}.platform-card{display:flex;min-height:250px;flex-direction:column;background:#fff;border:1px solid #e4e7ec;border-radius:20px;padding:22px;box-shadow:0 8px 30px #1018280d;transition:.2s ease}.platform-card:hover{transform:translateY(-2px);box-shadow:0 16px 36px #10182814}.platform-card.unavailable{background:#ffffffb8}.platform-card header{display:flex;align-items:center;gap:13px}.platform-card h2{margin:0 0 5px;font-size:20px}.platform-card p{flex:1;margin:20px 0 14px;color:#667085}.platform-mark{display:grid;width:48px;height:48px;place-items:center;border-radius:15px;color:#fff;font-size:21px;font-weight:800}.mark-feishu{background:#3370ff}.mark-wecom{background:#2aae67}.mark-dingtalk{background:#1677ff}.mark-wechat{background:#07c160}.platform-mode{color:#475467;font-size:13px}.platform-card footer{min-height:40px;margin-top:18px;display:flex;align-items:center}.prerequisite{color:#98a2b3;font-size:13px}.security-panel{max-width:1074px;margin:22px auto 0;padding:20px 22px;border:1px solid #d0d5dd;border-radius:18px;background:#ffffffb8}.security-panel h2{margin:0 0 10px;font-size:17px}.security-panel ul{margin:0;padding-left:20px;color:#667085}.dialog-head{display:flex;align-items:center;justify-content:space-between;padding-right:28px}.dialog-head strong{font-size:20px}.qr-dialog{text-align:center}.qr-dialog img{display:block;width:300px;max-width:82vw;margin:0 auto;border-radius:18px}.qr-dialog h3{margin:16px 0 4px}.qr-dialog p{margin:0 0 18px;color:#667085}.qr-dialog small{display:block;margin-top:8px;color:#98a2b3}.qr-actions{display:flex;justify-content:center;gap:8px;margin-top:20px}@media(max-width:980px){.platform-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.hero{align-items:flex-start;flex-direction:column}}@media(max-width:620px){.authorization-page{padding:20px 12px}.hero h1{font-size:28px}.hero-actions{width:100%;flex-direction:column;gap:8px}.hero-actions .el-button{width:100%;margin-left:0}.platform-grid{grid-template-columns:1fr}.platform-card{min-height:220px}}
</style>
