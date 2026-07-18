<template>
  <div class="web-login-form">
    <div class="web-login-tabs" role="tablist" aria-label="登录方式">
      <button v-for="tab in loginTabs" :key="tab.id" type="button" role="tab" :aria-selected="mode === tab.id" :class="{ active: mode === tab.id }" @click="mode = tab.id">
        {{ tab.label }}
      </button>
    </div>

    <section v-if="mode === 'wechat'" class="wechat-login-panel" aria-live="polite">
      <div v-if="!agreementAccepted" class="wechat-login-placeholder">
        <div class="wechat-placeholder-icon">微</div>
        <strong>同意协议后显示登录二维码</strong>
        <span>使用微信扫一扫，在手机上确认登录</span>
      </div>
      <div v-else-if="qrLoading" class="wechat-login-placeholder"><span class="auth-spinner"></span><strong>正在加载微信二维码</strong></div>
      <div v-else-if="qrStatus === 'MOBILE_REQUIRED'" class="wechat-bind-mobile">
        <strong>首次使用，请绑定手机号</strong>
        <p>绑定后将与小程序、会员、余额、订单和作品使用同一账号。</p>
        <input v-model.trim="bindMobileForm.mobile" inputmode="numeric" maxlength="11" autocomplete="tel" placeholder="请输入手机号" />
        <div class="web-login-code-row">
          <input v-model.trim="bindMobileForm.code" inputmode="numeric" maxlength="6" autocomplete="one-time-code" placeholder="短信验证码" />
          <button type="button" :disabled="smsSending || countdown > 0 || submitting" @click="sendSmsCode(bindMobileForm.mobile)">{{ smsButtonText }}</button>
        </div>
        <button class="web-login-submit" type="button" :disabled="submitting" @click="bindWechatMobile">{{ submitting ? "绑定中..." : "绑定并登录" }}</button>
      </div>
      <div v-else-if="qrUrl && qrStatus !== 'EXPIRED' && qrStatus !== 'UNAVAILABLE'" class="wechat-qr-frame-wrap">
        <iframe class="wechat-qr-frame" :src="qrUrl" title="微信扫码登录二维码" sandbox="allow-scripts allow-same-origin allow-forms allow-popups" />
        <strong>{{ qrStatus === "CONFIRMED" ? "手机已确认，正在登录" : "微信扫码登录" }}</strong>
        <span>{{ qrStatus === "CONFIRMED" ? "请稍候，不要重复扫码" : "二维码将在 5 分钟后失效" }}</span>
      </div>
      <div v-else class="wechat-login-placeholder">
        <div class="wechat-placeholder-icon">↻</div>
        <strong>{{ qrStatus === "UNAVAILABLE" ? "微信扫码登录暂不可用" : "二维码已过期" }}</strong>
        <span>{{ qrMessage || "请刷新二维码后重试" }}</span>
        <button type="button" class="wechat-refresh-button" @click="loadQRCode">刷新二维码</button>
      </div>
    </section>

    <form v-else-if="mode === 'sms'" class="web-login-fields" @submit.prevent="submitSmsLogin">
      <label><span>手机号</span><input v-model.trim="smsForm.mobile" autocomplete="tel" inputmode="numeric" maxlength="11" placeholder="请输入手机号" /></label>
      <label>
        <span>短信验证码</span>
        <div class="web-login-code-row">
          <input v-model.trim="smsForm.code" autocomplete="one-time-code" inputmode="numeric" maxlength="6" placeholder="请输入验证码" />
          <button type="button" :disabled="smsSending || countdown > 0 || submitting" @click="sendSmsCode(smsForm.mobile)">{{ smsButtonText }}</button>
        </div>
      </label>
      <p class="web-login-tip">未注册手机号验证成功后将自动创建账号，并与小程序手机号账号保持一致。</p>
      <button class="web-login-submit" type="submit" :disabled="submitting">{{ submitting ? "登录中..." : submitLabel }}</button>
    </form>

    <form v-else class="web-login-fields" @submit.prevent="submitPasswordLogin">
      <label><span>手机号或邮箱</span><input v-model.trim="passwordForm.account" autocomplete="username" placeholder="请输入手机号或邮箱" /></label>
      <label>
        <span>密码</span>
        <div class="web-password-row">
          <input v-model="passwordForm.password" autocomplete="current-password" :type="passwordVisible ? 'text' : 'password'" placeholder="请输入密码" />
          <button type="button" :aria-label="passwordVisible ? '隐藏密码' : '显示密码'" @click="passwordVisible = !passwordVisible">{{ passwordVisible ? "隐藏" : "显示" }}</button>
        </div>
      </label>
      <button class="web-login-submit" type="submit" :disabled="submitting">{{ submitting ? "登录中..." : submitLabel }}</button>
    </form>

    <label class="web-login-agreement">
      <input v-model="agreementAccepted" type="checkbox" />
      <span>我已阅读并同意 <button type="button" @click="legalDocument = 'agreement'">《用户协议》</button> 和 <button type="button" @click="legalDocument = 'privacy'">《隐私政策》</button></span>
    </label>
    <label class="web-login-remember"><input v-model="remember" type="checkbox" /><span>保持登录</span></label>
    <a v-if="registerHref" class="web-login-link" :href="registerHref">没有账号，通过邀请码注册</a>
    <button v-if="allowGuest" class="web-login-link web-login-guest" type="button" @click="$emit('guest')">暂不登录，继续浏览</button>

    <Teleport to="body">
      <div v-if="legalDocument" class="web-legal-overlay" @click.self="legalDocument = ''">
        <section class="web-legal-dialog" role="dialog" aria-modal="true" :aria-label="legalTitle">
          <button class="web-legal-close" type="button" aria-label="关闭" @click="legalDocument = ''">×</button>
          <h2>{{ legalTitle }}</h2>
          <p v-for="paragraph in legalParagraphs" :key="paragraph">{{ paragraph }}</p>
          <button class="web-login-submit" type="button" @click="legalDocument = ''">我已阅读</button>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { adminRequest } from "../../api/client";
import { trackWebGuestExperience } from "../../utils/webGuestAnalytics";

type LoginMode = "wechat" | "sms" | "password";
type QRStatus = "IDLE" | "PENDING" | "CONFIRMED" | "MOBILE_REQUIRED" | "EXPIRED" | "UNAVAILABLE";
type AuthResponse = { user: Record<string, unknown>; accessToken?: string; defaultRoute?: string; status?: string; [key: string]: unknown };
type QRCodeResponse = { qrCodeId: string; qrUrl: string; status: string; expiresInSeconds?: number; pollIntervalMs?: number };

const props = withDefaults(defineProps<{ submitLabel?: string; registerHref?: string; allowGuest?: boolean; initialMode?: LoginMode }>(), {
  submitLabel: "登录",
  registerHref: "",
  allowGuest: false
});
const emit = defineEmits<{ authenticated: [response: AuthResponse, remember: boolean]; guest: [] }>();
const loginTabs: Array<{ id: LoginMode; label: string }> = [
  { id: "wechat", label: "微信扫码登录" }, { id: "sms", label: "手机号登录" }, { id: "password", label: "密码登录" }
];

const isMobileBrowser = typeof window !== "undefined" && window.matchMedia("(max-width: 960px)").matches;
const isWeChatBrowser = typeof navigator !== "undefined" && /MicroMessenger/i.test(navigator.userAgent);
const mode = ref<LoginMode>(props.initialMode || (isMobileBrowser && !isWeChatBrowser ? "sms" : "wechat"));
const remember = ref(true);
const agreementAccepted = ref(false);
const legalDocument = ref<"" | "agreement" | "privacy">("");
const passwordVisible = ref(false);
const submitting = ref(false);
const smsSending = ref(false);
const countdown = ref(0);
const passwordForm = reactive({ account: "", password: "" });
const smsForm = reactive({ mobile: "", code: "" });
const bindMobileForm = reactive({ mobile: "", code: "" });
const qrLoading = ref(false);
const qrStatus = ref<QRStatus>("IDLE");
const qrMessage = ref("");
const qrCodeId = ref("");
const qrUrl = ref("");
let countdownTimer: number | null = null;
let qrPollTimer: number | null = null;
let qrExpiresAt = 0;
let pollIntervalMs = 2000;

const smsButtonText = computed(() => smsSending.value ? "发送中..." : countdown.value > 0 ? `${countdown.value}s 后重发` : "获取验证码");
const legalTitle = computed(() => legalDocument.value === "privacy" ? "知启云 AI 隐私政策" : "知启云 AI 用户协议");
const legalParagraphs = computed(() => legalDocument.value === "privacy" ? [
  "为提供账号登录、创作、作品同步、订单和企业服务，平台会在必要范围内处理账号标识、手机号、设备与服务使用信息。",
  "平台采取访问控制、加密和审计措施保护个人信息，不会在未经授权的情况下公开 Token、密码或创作中的敏感内容。",
  "你可以通过账号设置或联系客服申请查询、更正、删除个人信息或注销账号。正式政策版本以运营后台发布内容为准。"
] : [
  "使用知启云 AI 前，请妥善保管账号和登录凭证，不得利用平台生成或传播违法违规、侵权或危害他人权益的内容。",
  "会员、额度、订单和付费能力以服务端记录及页面展示规则为准。涉及正式生成或扣费的操作会按产品流程另行确认。",
  "你上传的素材应拥有合法使用权。正式协议版本以运营后台发布内容为准；如不同意协议，请选择暂不登录并继续浏览公开内容。"
]);
function validMobile(value: string) { return /^1\d{10}$/.test(value.trim()); }
function requireAgreement() { if (agreementAccepted.value) return true; ElMessage.warning("请先阅读并同意用户协议和隐私政策"); return false; }
function stopQRPolling() { if (qrPollTimer !== null) window.clearTimeout(qrPollTimer); qrPollTimer = null; }
function scheduleQRPoll() { stopQRPolling(); if (mode.value !== "wechat" || document.hidden || !qrCodeId.value) return; qrPollTimer = window.setTimeout(pollQRCode, pollIntervalMs); }

function startCountdown(seconds: number) {
  countdown.value = Math.max(1, Math.min(300, Math.round(seconds || 60)));
  if (countdownTimer !== null) window.clearInterval(countdownTimer);
  countdownTimer = window.setInterval(() => { countdown.value = Math.max(0, countdown.value - 1); if (!countdown.value && countdownTimer !== null) { window.clearInterval(countdownTimer); countdownTimer = null; } }, 1000);
}

async function loadQRCode() {
  if (!requireAgreement() || qrLoading.value) return;
  trackWebGuestExperience("login_start", "webLogin", { authMethod: "wechat_qrcode" });
  stopQRPolling(); qrLoading.value = true; qrMessage.value = ""; qrStatus.value = "IDLE";
  try {
    const response = await adminRequest<QRCodeResponse>({ method: "GET", url: "/auth/wechat/qrcode", authMode: "none", retryOnUnauthorized: false });
    qrCodeId.value = response.qrCodeId; qrUrl.value = response.qrUrl; qrStatus.value = "PENDING";
    pollIntervalMs = Math.max(1500, Number(response.pollIntervalMs || 2000)); qrExpiresAt = Date.now() + Number(response.expiresInSeconds || 300) * 1000;
    scheduleQRPoll();
  } catch (error) {
    qrStatus.value = "UNAVAILABLE"; qrMessage.value = error instanceof Error ? error.message : "微信扫码登录暂不可用";
  } finally { qrLoading.value = false; }
}

async function pollQRCode() {
  if (!qrCodeId.value || mode.value !== "wechat") return;
  if (Date.now() >= qrExpiresAt) { qrStatus.value = "EXPIRED"; stopQRPolling(); return; }
  try {
    const response = await adminRequest<AuthResponse & { status?: string }>({ method: "GET", url: `/auth/wechat/status?qrCodeId=${encodeURIComponent(qrCodeId.value)}`, authMode: "none", retryOnUnauthorized: false });
    const status = String(response.status || "PENDING").toUpperCase();
    if (status === "SUCCESS" && response.accessToken) { stopQRPolling(); emit("authenticated", response, remember.value); return; }
    qrStatus.value = status === "MOBILE_REQUIRED" ? "MOBILE_REQUIRED" : status === "CONFIRMED" ? "CONFIRMED" : "PENDING";
    if (qrStatus.value !== "MOBILE_REQUIRED") scheduleQRPoll();
  } catch (error) {
    const message = error instanceof Error ? error.message : "二维码状态读取失败";
    if (message.includes("过期")) { qrStatus.value = "EXPIRED"; qrMessage.value = message; stopQRPolling(); }
    else scheduleQRPoll();
  }
}

async function submitPasswordLogin() {
  if (submitting.value || !requireAgreement()) return;
  const account = passwordForm.account.trim();
  if (!account || !passwordForm.password.trim()) { ElMessage.warning("请输入手机号或邮箱和密码"); return; }
  trackWebGuestExperience("login_start", "webLogin", { authMethod: "password" });
  submitting.value = true;
  try { emit("authenticated", await adminRequest<AuthResponse>({ method: "POST", url: "/auth/login", authMode: "none", retryOnUnauthorized: false, data: { account, email: account, mobile: account, password: passwordForm.password } }), remember.value); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "登录失败"); }
  finally { submitting.value = false; }
}

async function sendSmsCode(mobileValue: string) {
  if (smsSending.value || countdown.value > 0 || submitting.value || !requireAgreement()) return;
  const mobile = mobileValue.trim();
  if (!validMobile(mobile)) { ElMessage.warning("请输入正确的 11 位手机号"); return; }
  smsSending.value = true;
  try { const response = await adminRequest<{ retryAfterSeconds?: number }>({ method: "POST", url: "/auth/sms/send", authMode: "none", retryOnUnauthorized: false, data: { mobile, purpose: "login" } }); startCountdown(Number(response.retryAfterSeconds || 60)); ElMessage.success("验证码已发送"); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "验证码发送失败"); }
  finally { smsSending.value = false; }
}

async function submitSmsLogin() {
  if (submitting.value || !requireAgreement()) return;
  const mobile = smsForm.mobile.trim(); const smsCode = smsForm.code.trim();
  if (!validMobile(mobile)) { ElMessage.warning("请输入正确的 11 位手机号"); return; }
  if (!/^\d{4,6}$/.test(smsCode)) { ElMessage.warning("请输入正确的短信验证码"); return; }
  trackWebGuestExperience("login_start", "webLogin", { authMethod: "sms" });
  submitting.value = true;
  try { emit("authenticated", await adminRequest<AuthResponse>({ method: "POST", url: "/auth/sms/login", authMode: "none", retryOnUnauthorized: false, data: { mobile, smsCode } }), remember.value); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "登录失败"); }
  finally { submitting.value = false; }
}

async function bindWechatMobile() {
  if (submitting.value || !requireAgreement()) return;
  if (!validMobile(bindMobileForm.mobile) || !/^\d{4,6}$/.test(bindMobileForm.code)) { ElMessage.warning("请输入正确的手机号和验证码"); return; }
  trackWebGuestExperience("login_start", "webLogin", { authMethod: "wechat_bind_mobile" });
  submitting.value = true;
  try { emit("authenticated", await adminRequest<AuthResponse>({ method: "POST", url: "/auth/wechat/bind-mobile", authMode: "none", retryOnUnauthorized: false, data: { qrCodeId: qrCodeId.value, mobile: bindMobileForm.mobile.trim(), smsCode: bindMobileForm.code.trim() } }), remember.value); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "手机号绑定失败"); }
  finally { submitting.value = false; }
}

function handleVisibilityChange() { if (document.hidden) stopQRPolling(); else if (mode.value === "wechat" && qrCodeId.value && qrStatus.value !== "MOBILE_REQUIRED") scheduleQRPoll(); }
watch(mode, (value) => { if (value === "wechat" && agreementAccepted.value && !qrCodeId.value) void loadQRCode(); else if (value !== "wechat") stopQRPolling(); });
watch(agreementAccepted, (accepted) => { if (accepted && mode.value === "wechat" && !qrCodeId.value) void loadQRCode(); if (!accepted) stopQRPolling(); });
onMounted(() => document.addEventListener("visibilitychange", handleVisibilityChange));
onBeforeUnmount(() => { if (countdownTimer !== null) window.clearInterval(countdownTimer); stopQRPolling(); document.removeEventListener("visibilitychange", handleVisibilityChange); });
</script>

<style scoped>
.web-login-form,.web-login-fields,.web-login-fields label{display:grid}.web-login-form,.web-login-fields{gap:14px}.web-login-tabs{display:grid;grid-template-columns:repeat(3,1fr);gap:3px;padding:4px;border-radius:12px;background:#f2f4f7}.web-login-tabs button{min-height:40px;padding:0 8px;border:0;border-radius:9px;color:#667085;background:transparent;font-size:13px;font-weight:800;cursor:pointer}.web-login-tabs button.active{color:#5b49e8;background:#fff;box-shadow:0 2px 8px rgba(16,24,40,.08)}.web-login-fields label{gap:7px;color:#344054;font-size:13px;font-weight:800}.web-login-fields input,.wechat-bind-mobile input{width:100%;height:46px;padding:0 12px;border:1px solid #d0d5dd;border-radius:10px;outline:none;box-sizing:border-box;color:#101828;background:#fff;font:inherit}.web-login-fields input:focus,.wechat-bind-mobile input:focus{border-color:#6c5cf4;box-shadow:0 0 0 3px rgba(108,92,244,.14)}.web-login-code-row,.web-password-row{display:grid;grid-template-columns:1fr 118px;gap:8px}.web-login-code-row button,.web-password-row button{border:1px solid #d9d5ff;border-radius:10px;color:#5b49e8;background:#f8f7ff;font-weight:800;cursor:pointer}.web-login-code-row button:disabled{cursor:not-allowed;opacity:.58}.web-login-agreement,.web-login-remember{display:flex;align-items:flex-start;gap:8px;color:#667085;font-size:12px;line-height:1.55}.web-login-agreement input,.web-login-remember input{flex:0 0 auto;width:16px;height:16px;margin-top:1px}.web-login-agreement button{padding:0;border:0;color:#5b49e8;background:transparent;font:inherit;cursor:pointer}.web-login-submit{height:46px;border:0;border-radius:10px;color:#fff;background:linear-gradient(135deg,#7464f2,#5b49e8);font-size:14px;font-weight:900;cursor:pointer}.web-login-submit:disabled{cursor:not-allowed;opacity:.62}.web-login-tip{margin:-2px 0 0;color:#667085;font-size:12px;line-height:1.55}.web-login-link{justify-self:center;border:0;color:#5b49e8;background:transparent;font-size:13px;font-weight:800;text-decoration:none;cursor:pointer}.wechat-login-panel{min-height:310px;display:grid;place-items:center}.wechat-login-placeholder,.wechat-qr-frame-wrap{display:grid;justify-items:center;gap:10px;text-align:center;color:#667085}.wechat-login-placeholder strong,.wechat-qr-frame-wrap strong{color:#101828}.wechat-placeholder-icon{display:grid;place-items:center;width:112px;height:112px;border-radius:22px;color:#fff;background:linear-gradient(135deg,#14b86e,#079455);font-size:44px;font-weight:900}.wechat-refresh-button{min-height:38px;padding:0 18px;border:1px solid #d9d5ff;border-radius:10px;color:#5b49e8;background:#fff;cursor:pointer}.wechat-qr-frame{width:260px;height:280px;border:0;border-radius:12px;background:#fff}.wechat-bind-mobile{width:100%;display:grid;gap:10px}.wechat-bind-mobile p{margin:0;color:#667085;font-size:12px;line-height:1.5}.auth-spinner{width:34px;height:34px;border:3px solid #ebe9ff;border-top-color:#6554e8;border-radius:50%;animation:auth-spin .8s linear infinite}.web-legal-overlay{position:fixed;inset:0;z-index:8000;display:grid;place-items:center;padding:20px;background:rgba(15,23,42,.62)}.web-legal-dialog{position:relative;width:min(560px,100%);box-sizing:border-box;padding:28px;border-radius:20px;background:#fff;box-shadow:0 30px 90px rgba(15,23,42,.35)}.web-legal-dialog h2{margin:0 42px 18px 0;color:#101828}.web-legal-dialog p{margin:0 0 12px;color:#475467;line-height:1.75}.web-legal-dialog .web-login-submit{width:100%;margin-top:8px}.web-legal-close{position:absolute;top:16px;right:16px;width:34px;height:34px;border:0;border-radius:50%;color:#667085;background:#f2f4f7;font-size:22px;cursor:pointer}@keyframes auth-spin{to{transform:rotate(360deg)}}@media(max-width:620px){.web-login-tabs{grid-template-columns:1fr}.wechat-login-panel{min-height:280px}.wechat-qr-frame{width:238px;height:260px}.web-legal-overlay{align-items:end;padding:0}.web-legal-dialog{border-radius:20px 20px 0 0}}@media(max-width:420px){.web-login-code-row,.web-password-row{grid-template-columns:1fr 104px}}
</style>
