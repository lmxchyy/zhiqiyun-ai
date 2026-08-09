<template>
  <SafeAreaContainer>
    <BrandHeader />

    <SuccessState v-if="viewState === 'success'" :benefit-text="benefitText" @start="enterProduct()" />

    <ErrorState
      v-else-if="viewState === 'error'"
      :kind="errorState"
      :detail="errorDetail"
      @primary="handleErrorPrimary()"
      @secondary="returnToAvailableLogin()"
    />

    <!-- #ifdef MP-WEIXIN -->
    <LoginCard
      v-if="viewState === 'form' && mode === 'wechat'"
      title="欢迎使用知启云AI"
      subtitle="登录后即可继续创作与管理作品"
      mode="wechat"
    >
      <PrimaryLoginButton
        label="手机号快捷登录"
        :disabled="busy"
        :open-type="agreementAccepted ? 'getPhoneNumber' : ''"
        @activate="onWechatButtonClick()"
        @getphonenumber="onGetPhoneNumber($event)"
      />
      <text class="auth-auto-register">未注册手机号将自动创建账号</text>
      <button
        class="auth-guest-enter-button"
        hover-class="auth-guest-enter-pressed"
        :disabled="busy"
        @click="enterGuestBrowse()"
      >
        <text>暂不登录，进入首页</text>
      </button>
      <text class="auth-guest-hint">可先浏览功能，需要创作时再登录</text>
      <view class="auth-divider"><view /><text>其他登录方式</text><view /></view>
      <button class="auth-login-mode-button" hover-class="auth-login-mode-hover" @click="switchMode('sms')">
        <text>使用手机号验证码登录</text>
      </button>
      <button
        class="auth-login-mode-button muted auth-login-mode-password"
        hover-class="auth-login-mode-hover"
        @click="switchMode('password')"
      >
        <text>账号密码登录  ›</text>
      </button>
      <InviteCodeEntry class="auth-invite-spacing" :status="inviteStatus" @click="openInviteSheet()" />
      <AgreementCheckbox
        v-model="agreementAccepted"
        class="auth-agreement-spacing"
        :highlight="agreementHighlight"
        @open="openAgreement($event)"
      />
      <SecondaryLoginEntry class="auth-help-spacing" label="登录遇到问题？" muted @activate="showLoginHelp()" />
    </LoginCard>
    <!-- #endif -->

    <LoginCard
      v-if="viewState === 'form' && mode === 'sms'"
      title="手机号验证码登录"
      subtitle="登录与注册合并，系统自动识别账号"
      mode="sms"
    >
      <MobileInput v-model="mobile" :error="mobileError" />
      <VerificationCodeInput
        v-model="smsCode"
        :action-label="smsActionLabel"
        :disabled="smsSending || countdown > 0 || busy"
        :error="smsError"
        @send="sendSmsCode()"
        @confirm="loginWithSms()"
      />
      <PrimaryLoginButton label="登录 / 注册" :disabled="busy" @activate="loginWithSms()" />
      <button
        class="auth-guest-enter-button"
        hover-class="auth-guest-enter-pressed"
        :disabled="busy"
        @click="enterGuestBrowse()"
      >
        <text>暂不登录，进入首页</text>
      </button>
      <text class="auth-guest-hint">可先浏览功能，需要创作时再登录</text>
      <SecondaryLoginEntry class="auth-mode-back" label="账号密码登录" @activate="switchMode('password')" />
      <!-- #ifdef MP-WEIXIN -->
      <SecondaryLoginEntry class="auth-mode-back" label="返回手机号快捷登录" @activate="switchMode('wechat')" />
      <!-- #endif -->
      <InviteCodeEntry :status="inviteStatus" @click="openInviteSheet()" />
      <AgreementCheckbox
        v-model="agreementAccepted"
        class="auth-agreement-spacing sms"
        :highlight="agreementHighlight"
        @open="openAgreement($event)"
      />
      <SecondaryLoginEntry class="auth-help-spacing sms" label="登录遇到问题？" muted @activate="showLoginHelp()" />
    </LoginCard>

    <LoginCard
      v-if="viewState === 'form' && mode === 'password'"
      title="账号密码登录"
      subtitle="适用于企业员工及已设置密码的账号"
      mode="password"
    >
      <view class="auth-field-block">
        <text class="auth-field-label">手机号 / 账号</text>
        <view :class="['auth-account-shell', { error: accountError }]">
          <input
            id="auth-account-input"
            v-model="account"
            class="auth-account-input"
            maxlength="80"
            placeholder="请输入手机号或账号"
            confirm-type="next"
            @input="accountError = ''"
          />
        </view>
        <text v-if="accountError" class="auth-field-error">{{ accountError }}</text>
      </view>
      <view class="auth-field-block auth-password-field">
        <text class="auth-field-label">密码</text>
        <view :class="['auth-password-shell', { error: passwordError }]">
          <input
            id="auth-password-input"
            v-model="password"
            class="auth-password-input"
            :password="!passwordVisible"
            maxlength="64"
            placeholder="请输入登录密码"
            confirm-type="done"
            @input="passwordError = ''"
            @confirm="loginWithPassword()"
          />
          <button
            class="auth-password-toggle"
            :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
            @click="passwordVisible = !passwordVisible"
          >
            <text>{{ passwordVisible ? "◉" : "◎" }}</text>
          </button>
        </view>
        <text v-if="passwordError" class="auth-field-error">{{ passwordError }}</text>
      </view>
      <button class="auth-forgot" @click="forgotPassword()">忘记密码？</button>
      <view
        :class="['auth-password-submit', { disabled: busy }]"
        role="button"
        :aria-disabled="busy"
        hover-class="auth-password-submit-pressed"
        @tap="loginWithPassword()"
      >
        <text>{{ busy ? "正在登录…" : agreementAccepted ? "登录" : "同意协议并登录" }}</text>
      </view>
      <SecondaryLoginEntry label="使用手机号验证码登录" @activate="switchMode('sms')" />
      <!-- #ifdef MP-WEIXIN -->
      <SecondaryLoginEntry label="返回手机号快捷登录" muted @activate="switchMode('wechat')" />
      <!-- #endif -->
      <view class="auth-password-note"><text>首次快捷登录或验证码登录后，可在账号与安全中设置密码</text></view>
      <view :class="['auth-password-agreement', { highlight: agreementHighlight }]">
        <view :class="['auth-password-agreement-toggle', { checked: agreementAccepted }]" @tap.stop="togglePasswordAgreement()">
          <view :class="['auth-password-agreement-box', { checked: agreementAccepted }]">
            <text v-if="agreementAccepted">✓</text>
          </view>
          <text>我已阅读并同意</text>
        </view>
        <view class="auth-password-agreement-copy">
          <text class="auth-password-agreement-link" @click.stop="openAgreement('user')">《用户协议》</text>
          <text>和</text>
          <text class="auth-password-agreement-link" @click.stop="openAgreement('privacy')">《隐私政策》</text>
        </view>
      </view>
      <button
        class="auth-guest-enter-button"
        hover-class="auth-guest-enter-pressed"
        :disabled="busy"
        @click="enterGuestBrowse()"
      >
        <text>暂不登录，进入首页</text>
      </button>
      <text class="auth-guest-hint">可先浏览功能，需要创作时再登录</text>
    </LoginCard>

    <BottomSheet
      :visible="inviteSheetVisible"
      title="填写邀请码"
      :keyboard-height="keyboardHeight"
      @close="closeInviteSheet()"
    >
      <view class="auth-sheet-field">
        <text class="auth-sheet-label">邀请码</text>
        <input
          id="auth-invite-input"
          v-model="inviteDraft"
          class="auth-sheet-input"
          maxlength="32"
          placeholder="请输入邀请码"
          confirm-type="done"
          @input="inviteMessage = ''"
          @confirm="confirmInvite()"
        />
        <text :class="['auth-invite-message', { error: inviteMessageTone === 'error', success: inviteMessageTone === 'success' }]">
          {{ inviteMessage || "邀请码为选填，不影响正常登录注册" }}
        </text>
      </view>
      <PrimaryLoginButton label="确认填写" :loading="inviteValidating" loading-text="正在校验…" @activate="confirmInvite()" />
      <SecondaryLoginEntry label="暂不填写" muted @activate="closeInviteSheet()" />
      <SecondaryLoginEntry v-if="pendingInviteCode" label="删除邀请码" muted @activate="removeInvite()" />
    </BottomSheet>

    <!-- #ifdef MP-WEIXIN -->
    <BottomSheet
      :visible="authorizationSheetVisible"
      title="选择登录方式"
      @close="closeAuthorizationSheet()"
    >
      <view class="auth-permission-sheet">
        <view class="auth-permission-icon">!</view>
        <text class="auth-permission-title">未获得手机号授权</text>
        <text class="auth-permission-copy">你仍可以使用手机号验证码继续登录</text>
        <PrimaryLoginButton label="使用验证码登录" @activate="useSmsAfterAuthorizationFailure()" />
        <PrimaryLoginButton
          class="auth-retry-authorization"
          label="重新授权"
          open-type="getPhoneNumber"
          @getphonenumber="onGetPhoneNumber($event)"
        />
        <button
          class="auth-guest-enter-button auth-permission-guest-button"
          hover-class="auth-guest-enter-pressed"
          @click="enterGuestBrowse()"
        >
          <text>暂不登录，进入首页</text>
        </button>
      </view>
    </BottomSheet>
    <!-- #endif -->

    <BottomSheet :visible="agreementSheetVisible" :title="agreementSheetTitle" @close="agreementSheetVisible = false">
      <scroll-view class="auth-agreement-document" scroll-y>
        <text>{{ agreementSheetContent }}</text>
      </scroll-view>
      <PrimaryLoginButton label="我已阅读并同意" @activate="acceptAgreementFromSheet()" />
    </BottomSheet>

    <LoginLoading :visible="busy" :step="loadingStep" />
    <Toast :visible="toastVisible" :message="toastMessage" :tone="toastTone" />
  </SafeAreaContainer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { onLoad, onUnload } from "@dcloudio/uni-app";
import { ApiClientError } from "@xianzhi/api-client";
import { apiRequestTask, authStorage } from "../api/client";
import AgreementCheckbox from "../components/auth/AgreementCheckbox.vue";
import BottomSheet from "../components/auth/BottomSheet.vue";
import BrandHeader from "../components/auth/BrandHeader.vue";
import ErrorState from "../components/auth/ErrorState.vue";
import InviteCodeEntry from "../components/auth/InviteCodeEntry.vue";
import LoginCard from "../components/auth/LoginCard.vue";
import LoginLoading from "../components/auth/LoginLoading.vue";
import MobileInput from "../components/auth/MobileInput.vue";
import PrimaryLoginButton from "../components/auth/PrimaryLoginButton.vue";
import SafeAreaContainer from "../components/auth/SafeAreaContainer.vue";
import SecondaryLoginEntry from "../components/auth/SecondaryLoginEntry.vue";
import SuccessState from "../components/auth/SuccessState.vue";
import Toast from "../components/auth/Toast.vue";
import VerificationCodeInput from "../components/auth/VerificationCodeInput.vue";
import { trackLogin } from "../features/auth/analytics";
import { loginAPI } from "../features/auth/api";
import { clearGuestBrowse, enterGuestBrowseHome } from "../features/auth/guestBrowse";
import { redirectAfterAuth } from "../features/auth/redirect";
import { parseLoginSource, parseRedirectInfo } from "../features/auth/source";
import { recordAgentInviteAppActivation } from "../features/invite/activation";
import type {
  AuthFlowResponse,
  InviteStatus,
  LoadingStep,
  LoginErrorState,
  LoginMode,
  LoginRedirectInfo,
  LoginSourceParams,
} from "../features/auth/types";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import type { AppRole } from "../types";

type ViewState = "form" | "success" | "error";
type ToastTone = "info" | "error" | "success";

const authStore = useAuthStore();
const userStore = useUserStore();
const mode = ref<LoginMode>("wechat");
// App MVP uses backend-supported SMS/password login. WeChat App OAuth is enabled only after
// the dedicated Open Platform backend exchange is configured; the mini-program code path is not reused.
// #ifdef APP-PLUS
mode.value = "sms";
// #endif
const viewState = ref<ViewState>("form");
const errorState = ref<LoginErrorState>("network");
const errorDetail = ref("");
const loadingStep = ref<LoadingStep>("authorizing");
const busy = ref(false);
const agreementAccepted = ref(false);
const agreementHighlight = ref(false);
const mobile = ref("");
const smsCode = ref("");
const account = ref("");
const password = ref("");
const passwordVisible = ref(false);
const mobileError = ref("");
const smsError = ref("");
const accountError = ref("");
const passwordError = ref("");
const smsSending = ref(false);
const countdown = ref(0);
const keyboardHeight = ref(0);
const scrollTarget = ref("");
const inviteSheetVisible = ref(false);
const authorizationSheetVisible = ref(false);
const agreementSheetVisible = ref(false);
const agreementSheetTitle = ref("");
const agreementSheetContent = ref("");
const pendingInviteCode = ref("");
const pendingInviteToken = ref("");
const inviteDraft = ref("");
const inviteStatus = ref<InviteStatus>("empty");
const inviteValidating = ref(false);
const inviteMessage = ref("");
const inviteMessageTone = ref<"info" | "error" | "success">("info");
const benefitText = ref("新人体验权益已到账");
const toastVisible = ref(false);
const toastMessage = ref("");
const toastTone = ref<ToastTone>("info");
const idempotencyKeys = reactive<Partial<Record<LoginMode, string>>>({});
let countdownTimer: ReturnType<typeof setInterval> | null = null;
let toastTimer: ReturnType<typeof setTimeout> | null = null;
let agreementTimer: ReturnType<typeof setTimeout> | null = null;
let requestVersion = 0;
let destroyed = false;
let sourceParams: LoginSourceParams = {
  inviteCode: "", inviteToken: "", inviteSource: "none", sceneCode: "", promoterCode: "", campaignCode: "", channel: "", sourcePage: "",
};
let redirectInfo: LoginRedirectInfo = { path: "", query: {}, action: "", sourcePage: "" };

const smsActionLabel = computed(() => {
  if (smsSending.value) return "发送中";
  if (countdown.value > 0) return `${countdown.value}秒后重新获取`;
  return "获取验证码";
});

watch(mobile, () => { mobileError.value = ""; smsError.value = ""; idempotencyKeys.sms = ""; });
watch(smsCode, () => { smsError.value = ""; });
watch(account, () => { accountError.value = ""; idempotencyKeys.password = ""; });
watch(password, () => { passwordError.value = ""; });

function showToast(message: string, tone: ToastTone = "info") {
  if (toastTimer) clearTimeout(toastTimer);
  toastMessage.value = message;
  toastTone.value = tone;
  toastVisible.value = true;
  toastTimer = setTimeout(() => { toastVisible.value = false; }, 2200);
}

function ensureAgreement(): boolean {
  if (agreementAccepted.value) return true;
  agreementHighlight.value = true;
  scrollTarget.value = "auth-login-card";
  showToast("请先阅读并同意用户协议和隐私政策", "error");
  if (agreementTimer) clearTimeout(agreementTimer);
  agreementTimer = setTimeout(() => { agreementHighlight.value = false; }, 1600);
  return false;
}

function switchMode(next: LoginMode) {
  if (busy.value) return;
  viewState.value = "form";
  mode.value = next;
  mobileError.value = smsError.value = accountError.value = passwordError.value = "";
  scrollTarget.value = "auth-login-card";
  if (next === "sms") trackLogin("sms_login_click");
  if (next === "password") trackLogin("password_login_click");
}

function normalizeMobile(value: string): string {
  return value.replace(/\D/g, "").slice(0, 11);
}

function validMobile(value: string): boolean {
  return /^1[3-9]\d{9}$/.test(normalizeMobile(value));
}

function nextIdempotencyKey(method: LoginMode): string {
  if (!idempotencyKeys[method]) {
    idempotencyKeys[method] = `${method}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  }
  return idempotencyKeys[method] as string;
}

function attribution(method: LoginMode) {
  return {
    inviteCode: pendingInviteCode.value || undefined,
    inviteToken: pendingInviteToken.value || sourceParams.inviteToken || undefined,
    scene: sourceParams.sceneCode || undefined,
    promoterCode: sourceParams.promoterCode || undefined,
    campaignCode: sourceParams.campaignCode || undefined,
    redirectSource: redirectInfo.sourcePage || sourceParams.sourcePage || sourceParams.channel || undefined,
    idempotencyKey: nextIdempotencyKey(method),
  };
}

function defaultRole(roles: AppRole[] | undefined): AppRole {
  // Consumer home is the default product surface. Agent/operation are entered via explicit role switch.
  if (roles?.includes("USER")) return "USER";
  if (roles?.includes("AGENT")) return "AGENT";
  if (roles?.includes("OPERATION")) return "OPERATION";
  return "USER";
}

async function completeAuth(auth: AuthFlowResponse, version: number) {
  if (destroyed || version !== requestVersion) return;
  if (!auth.accessToken) throw new Error("TOKEN_SAVE_FAILED");
  try {
    authStorage.setToken(auth.accessToken);
    authStorage.setRefreshToken(auth.refreshToken || "");
    authStorage.setAuth(auth);
    authStore.applyAuth(auth);
    clearGuestBrowse();
    // #ifdef APP-PLUS
    void recordAgentInviteAppActivation();
    // #endif
  } catch {
    throw new Error("TOKEN_SAVE_FAILED");
  }
  const targetRole = defaultRole(auth.roles);
  if (userStore.currentRole !== targetRole) await userStore.switchRole(targetRole);
  pendingInviteCode.value = "";
  pendingInviteToken.value = "";
  inviteStatus.value = "empty";
  uni.removeStorageSync("zhiqiyun.promotion.pending-referral.v1");
  trackLogin(auth.isNewUser ? "register_success" : "login_success", { method: mode.value, isNewUser: Boolean(auth.isNewUser) });
  if (auth.isNewUser) {
    loadingStep.value = "registering";
    const firstBenefit = auth.newcomerBenefits?.find(item => item.title || item.description);
    benefitText.value = firstBenefit?.title || firstBenefit?.description || "新人体验权益已到账";
    busy.value = false;
    viewState.value = "success";
    return;
  }
  if (auth.inviteBindStatus === "ignored_existing" && (sourceParams.inviteCode || sourceParams.inviteToken)) {
    showToast("当前账号已注册，邀请码仅适用于新用户");
  }
  loadingStep.value = "entering";
  busy.value = false;
  redirectAfterAuth(redirectInfo, targetRole, () => showToast("页面打开失败，请重试", "error"));
}

function errorPayloadCode(error: unknown): string {
  if (error instanceof ApiClientError) {
    const payload = error.payload && typeof error.payload === "object" ? error.payload as Record<string, unknown> : {};
    return String(error.apiCode || payload.code || payload.errorCode || "").toUpperCase();
  }
  if (error instanceof Error) return error.message.toUpperCase();
  return "";
}

function errorPayloadValue(error: unknown, key: string): string {
  if (!(error instanceof ApiClientError) || !error.payload || typeof error.payload !== "object") return "";
  const payload = error.payload as Record<string, unknown>;
  return String(payload[key] || "").trim();
}

function errorStatusCode(error: unknown): number {
  return error instanceof ApiClientError ? error.statusCode : 0;
}

function handleLoginError(error: unknown, method: LoginMode) {
  const code = errorPayloadCode(error);
  const status = errorStatusCode(error);
  trackLogin("login_failed", { method, code: code.slice(0, 40), status });
  if (code.includes("SMS_CODE_INVALID") || code.includes("验证码错误")) {
    smsCode.value = "";
    smsError.value = "验证码错误，请重新输入";
    showToast(smsError.value, "error");
    return;
  }
  if (code.includes("SMS_CODE_EXPIRED") || code.includes("验证码过期")) {
    smsCode.value = "";
    smsError.value = "验证码已过期，请重新获取";
    showToast(smsError.value, "error");
    return;
  }
  if (code.includes("AUTH_ACCOUNT_MERGE_REQUIRED")) {
    const mergeRequestId = errorPayloadValue(error, "mergeRequestId");
    showToast(mergeRequestId ? `账号需要人工合并，工单号：${mergeRequestId}` : "账号需要人工合并，请联系客服处理", "error");
    return;
  }
  if (code.includes("ACCOUNT_FROZEN") || status === 423) return showErrorState("frozen");
  if (code.includes("ACCOUNT_DEACTIVATED")) return showErrorState("deactivated");
  if (code.includes("SYSTEM_MAINTENANCE")) return showErrorState("maintenance");
  if (status === 503 || code.includes("AUTH_SESSION_UNAVAILABLE")) return showErrorState("service");
  if (method === "wechat" && status === 502 && (code.includes("WECHAT") || code.includes("CODE2SESSION"))) {
    showToast("快捷登录凭证无效或已过期，请重新进行手机号快捷登录", "error");
    return;
  }
  if (code.includes("TOKEN_SAVE_FAILED")) return showErrorState("token");
  if (
    (error instanceof ApiClientError && status === 0)
    || code.includes("NETWORK")
    || code.includes("REQUEST:FAIL")
    || code.includes("CONNECTION")
  ) return showErrorState("network", error instanceof Error ? error.message : "请求未能到达服务器");
  if (code.includes("TIMEOUT")) return showErrorState("timeout");
  if (method === "password") {
    password.value = "";
    passwordError.value = status === 404 ? "账号不存在，请使用验证码登录" : "账号或密码不正确，请重试";
    showToast(passwordError.value, "error");
    return;
  }
  showToast(error instanceof Error ? error.message : "登录失败，请重试", "error");
}

function showErrorState(kind: LoginErrorState, detail = "") {
  errorState.value = kind;
  errorDetail.value = detail.slice(0, 180);
  viewState.value = "error";
}

async function runLogin(method: LoginMode, task: () => Promise<AuthFlowResponse>) {
  if (busy.value) return;
  const version = ++requestVersion;
  busy.value = true;
  loadingStep.value = method === "wechat" ? "authorizing" : "validating";
  try {
    const auth = await task();
    if (destroyed || version !== requestVersion) return;
    loadingStep.value = auth.isNewUser ? "registering" : "logging_in";
    await completeAuth(auth, version);
  } catch (error) {
    if (!destroyed && version === requestVersion) handleLoginError(error, method);
  } finally {
    if (!destroyed && version === requestVersion) busy.value = false;
  }
}

function requestWechatLoginCode() {
  return new Promise<string>((resolve, reject) => {
    uni.login({
      provider: "weixin",
      success: result => result.code ? resolve(result.code) : reject(new Error(`小程序登录凭证获取失败：${result.errMsg || "未返回 code"}`)),
      fail: result => reject(new Error(`小程序登录凭证获取失败：${result.errMsg || "未知错误"}`)),
    });
  });
}

function onWechatButtonClick() {
  if (!ensureAgreement()) return;
  trackLogin("wechat_login_click");
}

async function onGetPhoneNumber(event: unknown) {
  if (!ensureAgreement() || busy.value) return;
  const detail = (event && typeof event === "object" && "detail" in event ? (event as { detail?: Record<string, unknown> }).detail : {}) || {};
  const phoneCode = String(detail.code || "").trim();
  const errMsg = String(detail.errMsg || "");
  if (!phoneCode || !errMsg.toLowerCase().includes("ok")) {
    authorizationSheetVisible.value = true;
    trackLogin("phone_auth_cancel");
    return;
  }
  authorizationSheetVisible.value = false;
  trackLogin("phone_auth_success");
  await runLogin("wechat", async () => {
    loadingStep.value = "authorizing";
    const wxLoginCode = await requestWechatLoginCode();
    loadingStep.value = "validating";
    return loginAPI.wechatPhoneLogin({ wxLoginCode, phoneCode, ...attribution("wechat") });
  });
}

function useSmsAfterAuthorizationFailure() {
  authorizationSheetVisible.value = false;
  switchMode("sms");
}

async function sendSmsCode() {
  if (smsSending.value || countdown.value > 0 || busy.value) return;
  mobile.value = normalizeMobile(mobile.value);
  if (!validMobile(mobile.value)) {
    mobileError.value = mobile.value.length === 11 ? "请输入正确的手机号" : "请输入11位手机号";
    return;
  }
  smsSending.value = true;
  try {
    const result = await loginAPI.sendSms(mobile.value);
    countdown.value = Math.max(1, result.retryAfterSeconds || 60);
    startCountdown();
    trackLogin("sms_send_success");
    showToast("验证码已发送", "success");
  } catch (error) {
    const code = errorPayloadCode(error);
    if (code.includes("TOO_FREQUENT") || errorStatusCode(error) === 429) {
      showToast("发送过于频繁，请稍后再试", "error");
    } else {
      showToast("验证码发送失败，请重试", "error");
    }
  } finally {
    smsSending.value = false;
  }
}

function startCountdown() {
  if (countdownTimer) clearInterval(countdownTimer);
  countdownTimer = setInterval(() => {
    countdown.value = Math.max(0, countdown.value - 1);
    if (countdown.value === 0 && countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = null;
    }
  }, 1000);
}

async function loginWithSms() {
  if (!ensureAgreement()) return;
  mobile.value = normalizeMobile(mobile.value);
  if (!validMobile(mobile.value)) {
    mobileError.value = "请输入正确的11位手机号";
    return;
  }
  if (!/^\d{6}$/.test(smsCode.value)) {
    smsError.value = "请输入6位验证码";
    return;
  }
  await runLogin("sms", () => loginAPI.smsLogin({ mobile: mobile.value, smsCode: smsCode.value, ...attribution("sms") }));
  if (viewState.value !== "error" && !smsError.value) trackLogin("sms_login_success");
}

async function loginWithPassword() {
  if (!agreementAccepted.value) {
    agreementAccepted.value = true;
    agreementHighlight.value = false;
  }
  if (!account.value.trim()) {
    accountError.value = "请输入手机号或账号";
    return;
  }
  if (!password.value) {
    passwordError.value = "请输入登录密码";
    return;
  }
  await runLogin("password", () => loginAPI.passwordLogin(account.value.trim(), password.value, nextIdempotencyKey("password")));
}

function forgotPassword() {
  const digits = normalizeMobile(account.value);
  if (validMobile(digits)) mobile.value = digits;
  switchMode("sms");
  showToast("请使用手机号验证码登录后在账号与安全中设置密码");
}

function openInviteSheet() {
  if (busy.value) return;
  inviteDraft.value = pendingInviteCode.value;
  inviteMessage.value = "";
  inviteMessageTone.value = "info";
  inviteSheetVisible.value = true;
  trackLogin("invite_entry_click");
}

function closeInviteSheet() {
  inviteSheetVisible.value = false;
  keyboardHeight.value = 0;
}

function inviteStatusMessage(status: InviteStatus): string {
  const messages: Partial<Record<InviteStatus, string>> = {
    invalid: "邀请码无效，不影响正常登录注册",
    expired: "邀请码已过期，不影响正常登录注册",
    disabled: "邀请码已停用，不影响正常登录注册",
    agent_frozen: "该邀请码暂不可用，不影响正常登录注册",
  };
  return messages[status] || "邀请码校验失败，不影响正常登录注册";
}

async function validateInviteToken(token: string, carried: boolean) {
  inviteValidating.value = true;
  inviteStatus.value = "resolving";
  try {
    const result = await loginAPI.validateInviteToken(token);
    if (result.valid) {
      pendingInviteToken.value = token;
      pendingInviteCode.value = String(result.inviteCode || "").toUpperCase();
      sourceParams.inviteToken = token;
      if (pendingInviteCode.value) sourceParams.inviteCode = pendingInviteCode.value;
      inviteStatus.value = carried ? "carried" : "filled";
      inviteMessage.value = "邀请码有效";
      inviteMessageTone.value = "success";
      trackLogin("invite_validate_success", { source: carried ? sourceParams.inviteSource : "scene" });
      return true;
    }
    pendingInviteToken.value = token;
    inviteStatus.value = result.status || "invalid";
    inviteMessage.value = result.message || inviteStatusMessage(inviteStatus.value);
    inviteMessageTone.value = "error";
    trackLogin("invite_validate_failed", { status: inviteStatus.value });
    return false;
  } catch {
    pendingInviteToken.value = token;
    inviteStatus.value = "invalid";
    inviteMessage.value = "邀请码暂时无法校验，不影响正常登录注册";
    inviteMessageTone.value = "error";
    return false;
  } finally {
    inviteValidating.value = false;
  }
}

async function validateInvite(code: string, carried: boolean) {
  inviteValidating.value = true;
  inviteStatus.value = "resolving";
  try {
    const result = await loginAPI.validateInvite(code);
    if (result.valid) {
      pendingInviteCode.value = code;
      inviteStatus.value = carried ? "carried" : "filled";
      inviteMessage.value = "邀请码有效";
      inviteMessageTone.value = "success";
      trackLogin("invite_validate_success", { source: carried ? sourceParams.inviteSource : "manual" });
      return true;
    }
    pendingInviteCode.value = code;
    inviteStatus.value = result.status || "invalid";
    inviteMessage.value = result.message || inviteStatusMessage(inviteStatus.value);
    inviteMessageTone.value = "error";
    trackLogin("invite_validate_failed", { status: inviteStatus.value });
    return false;
  } catch {
    pendingInviteCode.value = code;
    inviteStatus.value = "invalid";
    inviteMessage.value = "邀请码暂时无法校验，不影响正常登录注册";
    inviteMessageTone.value = "error";
    return false;
  } finally {
    inviteValidating.value = false;
  }
}

async function confirmInvite() {
  if (inviteValidating.value) return;
  const code = inviteDraft.value.trim().toUpperCase().replace(/[^A-Z0-9_-]/g, "").slice(0, 32);
  inviteDraft.value = code;
  if (!code) return removeInvite();
  if (await validateInvite(code, false)) closeInviteSheet();
}

function removeInvite() {
  pendingInviteCode.value = "";
  pendingInviteToken.value = "";
  inviteDraft.value = "";
  inviteStatus.value = "empty";
  sourceParams.inviteCode = "";
  sourceParams.inviteToken = "";
  closeInviteSheet();
}

async function openAgreement(type: "user" | "privacy") {
  const code = type === "user" ? "user-agreement" : "privacy-policy";
  agreementSheetTitle.value = type === "user" ? "用户协议" : "隐私政策";
  agreementSheetContent.value = "正在加载协议正文...";
  agreementSheetVisible.value = true;
  try {
    const payload = await apiRequestTask<{ items: Array<{ code: string; title: string; content: string }> }>("/api/v1/public/legal-documents", { auth: false }).promise;
    const document = (payload.items || []).find(item => item.code === code);
    if (!document?.content || document.content === "待配置") throw new Error("协议正文尚未发布");
    agreementSheetTitle.value = document.title || agreementSheetTitle.value;
    agreementSheetContent.value = document.content;
  } catch (error) {
    agreementSheetContent.value = error instanceof Error ? error.message : "协议加载失败，请稍后重试";
  }
}

function acceptAgreementFromSheet() {
  agreementAccepted.value = true;
  agreementHighlight.value = false;
  agreementSheetVisible.value = false;
}

function togglePasswordAgreement() {
  agreementAccepted.value = !agreementAccepted.value;
  agreementHighlight.value = false;
}

function showLoginHelp() {
  uni.showModal({ title: "登录遇到问题？", content: "可切换手机号验证码登录；如账号被冻结或已注销，请联系平台客服处理。", confirmText: "联系客服", success: result => { if (result.confirm) contactService(); } });
}

function contactService() {
  uni.showModal({ title: "联系平台客服", content: "请通过知启云AI官方客服渠道反馈，并提供账号的脱敏手机号和问题发生时间。", showCancel: false });
}

function enterProduct() {
  viewState.value = "form";
  redirectAfterAuth(redirectInfo, userStore.currentRole || "USER", () => showToast("页面打开失败，请重试", "error"));
}

function handleErrorPrimary() {
  if (errorState.value === "frozen" || errorState.value === "deactivated") {
    contactService();
    return;
  }
  viewState.value = "form";
  showToast("请重新发起登录");
}

function returnToAvailableLogin() {
  viewState.value = "form";
  switchMode(errorState.value === "network" || errorState.value === "timeout" ? "sms" : "password");
}

function enterGuestBrowse() {
  authorizationSheetVisible.value = false;
  trackLogin("guest_browse_enter");
  enterGuestBrowseHome();
}

function closeAuthorizationSheet() {
  authorizationSheetVisible.value = false;
  trackLogin("phone_auth_sheet_close");
}

const keyboardHandler = (result: { height?: number }) => {
  keyboardHeight.value = Math.max(0, Number(result.height || 0));
  if (keyboardHeight.value > 0) {
    scrollTarget.value = inviteSheetVisible.value ? "auth-invite-input" : mode.value === "sms" ? "auth-code-input" : "auth-password-input";
  } else {
    scrollTarget.value = "";
  }
};

onLoad(async options => {
  const query = (options || {}) as Record<string, unknown>;
  sourceParams = parseLoginSource(query);
  redirectInfo = parseRedirectInfo(query);
  trackLogin("login_page_view", { hasInvite: Boolean(sourceParams.inviteCode || sourceParams.inviteToken), source: sourceParams.inviteSource });
  if (sourceParams.inviteToken) {
    pendingInviteToken.value = sourceParams.inviteToken;
    await validateInviteToken(sourceParams.inviteToken, true);
  } else if (sourceParams.inviteCode) {
    pendingInviteCode.value = sourceParams.inviteCode;
    await validateInvite(sourceParams.inviteCode, true);
  }
  if (typeof uni.onKeyboardHeightChange === "function") uni.onKeyboardHeightChange(keyboardHandler);
});

onUnload(() => {
  destroyed = true;
  requestVersion += 1;
  if (countdownTimer) clearInterval(countdownTimer);
  if (toastTimer) clearTimeout(toastTimer);
  if (agreementTimer) clearTimeout(agreementTimer);
  if (typeof uni.offKeyboardHeightChange === "function") uni.offKeyboardHeightChange(keyboardHandler);
});
</script>

<style scoped>
.auth-auto-register { display: block; margin-top: 10px; color: #8c94a8; font-size: 11px; line-height: 20px; text-align: center; }
.auth-guest-enter-button { width: 100%; min-height: 46px; margin: 16px 0 0; padding: 10px 16px; box-sizing: border-box; border: 1px solid #cfd8ff; border-radius: 14px; color: #3555e8; background: #f3f6ff; font-size: 14px; line-height: 24px; font-weight: 600; }
.auth-guest-enter-button::after { display: none; }
.auth-guest-enter-pressed { opacity: .72; }
.auth-guest-hint { display: block; margin-top: 7px; color: #697085; font-size: 11px; line-height: 18px; text-align: center; }
.auth-divider { display: flex; align-items: center; gap: 7px; margin: 24px 0 2px; color: #8c94a8; font-size: 12px; line-height: 20px; }
.auth-divider view { height: 1px; flex: 1; background: #e3e8f5; }
.auth-login-mode-button { width: 100%; min-height: 34px; margin: 0; padding: 5px 4px; box-sizing: border-box; border: 0; color: #4a6bff; background: transparent; font-size: 13px; line-height: 24px; font-weight: 500; }
.auth-login-mode-button::after { display: none; }
.auth-login-mode-button.muted { color: #697085; font-size: 12px; font-weight: 400; }
.auth-login-mode-hover { opacity: .68; }
.auth-invite-spacing { margin-top: 9px; }
.auth-agreement-spacing { margin-top: 18px; }
.auth-agreement-spacing.sms { margin-top: 16px; }
.auth-agreement-spacing.password { margin-top: 11px; }
.auth-help-spacing { margin-top: 22px; }
.auth-help-spacing.sms { margin-top: 7px; }
.auth-permission-guest-button { margin-top: 14px; }
.auth-mode-back { margin: 7px 0 5px; }
.auth-field-block { margin-bottom: 12px; }
.auth-field-label { display: block; margin-bottom: 6px; color: #181c28; font-size: 12px; line-height: 20px; font-weight: 500; }
.auth-account-shell { height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
.auth-account-shell:focus-within { border: 1.5px solid #4a6bff; background: #fff; }
.auth-account-shell.error { border-color: #eb404f; }
.auth-account-input { width: 100%; height: 50px; padding: 0 15px; box-sizing: border-box; color: #181c28; font-size: 14px; line-height: 50px; }
.auth-field-error { display: block; margin-top: 5px; color: #eb404f; font-size: 10px; line-height: 16px; }
.auth-password-field { margin-bottom: 4px; }
.auth-password-shell { display: flex; align-items: center; height: 50px; box-sizing: border-box; overflow: hidden; border: 1px solid #e0e5f2; border-radius: 14px; background: #f2f7ff; }
.auth-password-shell:focus-within { border: 1.5px solid #4a6bff; background: #fff; }
.auth-password-shell.error { border-color: #eb404f; }
.auth-password-input { min-width: 0; height: 50px; flex: 1; padding-left: 15px; color: #181c28; font-size: 14px; line-height: 50px; }
.auth-password-toggle { width: 52px; height: 50px; margin: 0; padding: 0; border: 0; color: #697085; background: transparent; font-size: 18px; line-height: 50px; }
.auth-password-toggle::after { display: none; }
.auth-forgot { min-height: 34px; margin: 0; padding: 4px 0; border: 0; color: #4a6bff; background: transparent; font-size: 12px; line-height: 22px; text-align: right; }
.auth-forgot::after { display: none; }
.auth-password-submit { width: 100%; height: 50px; margin: 4px 0 0; padding: 0 16px; box-sizing: border-box; display: flex; align-items: center; justify-content: center; border: 0; border-radius: 15px; color: #fff; background: #4a6bff; box-shadow: 0 8px 18px rgba(46, 71, 204, 0.22); font-size: 15px; line-height: 24px; font-weight: 500; }
.auth-password-submit.disabled { color: #9ca4b5; background: #d9deec; box-shadow: none; opacity: 1; }
.auth-password-submit-pressed { background: #3f5be0; opacity: .96; }
.auth-password-note { margin-top: 7px; padding: 8px 12px; border-radius: 12px; color: #697085; background: #f2f7ff; font-size: 11px; line-height: 24px; text-align: center; }
.auth-password-agreement { display: flex; align-items: center; gap: 0; margin-top: 11px; padding: 4px 0; border-radius: 8px; transition: background 160ms ease; }
.auth-password-agreement.highlight { padding: 5px 6px; background: #fff1f2; }
.auth-password-agreement-toggle { min-width: 0; min-height: 36px; flex: 0 0 auto; display: flex; align-items: center; color: #697085; font-size: 11px; line-height: 20px; }
.auth-password-agreement-box { width: 18px; height: 18px; margin: 0 5px 0 0; box-sizing: border-box; border: 1px solid #b9c1d2; border-radius: 5px; color: #fff; background: #fff; font-size: 12px; line-height: 16px; font-weight: 700; text-align: center; }
.auth-password-agreement-box.checked { border-color: #4a6bff; background: #4a6bff; }
.auth-password-agreement-copy { min-width: 0; min-height: 36px; display: flex; align-items: center; flex-wrap: wrap; color: #697085; font-size: 11px; line-height: 20px; }
.auth-password-agreement-link { color: #4a6bff; font-weight: 500; }
.auth-sheet-field { margin-bottom: 24px; }
.auth-sheet-label { display: block; margin-bottom: 7px; color: #181c28; font-size: 12px; line-height: 22px; font-weight: 500; }
.auth-sheet-input { width: 100%; height: 50px; padding: 0 15px; box-sizing: border-box; border: 1px solid #d6dff1; border-radius: 14px; color: #181c28; background: #f5f8ff; font-size: 14px; line-height: 50px; }
.auth-invite-message { display: block; min-height: 20px; margin-top: 9px; color: #697085; font-size: 11px; line-height: 20px; }
.auth-invite-message.error { color: #eb404f; } .auth-invite-message.success { color: #18a06a; }
.auth-permission-sheet { padding: 10px 0 0; text-align: center; }
.auth-permission-icon { width: 72px; height: 72px; margin: 0 auto 17px; border-radius: 24px; color: #4a6bff; background: #edf2ff; font-size: 28px; line-height: 72px; font-weight: 700; }
.auth-permission-title, .auth-permission-copy { display: block; }
.auth-permission-title { color: #181c28; font-size: 21px; line-height: 32px; font-weight: 700; }
.auth-permission-copy { margin: 8px 0 24px; color: #697085; font-size: 13px; line-height: 24px; }
.auth-retry-authorization { margin-top: 14px; color: #4a6bff !important; border: 1px solid #e0e5f2 !important; background: #fff !important; box-shadow: none !important; }
.auth-agreement-document { max-height: 220px; margin-bottom: 22px; color: #697085; font-size: 13px; line-height: 24px; }
@media (max-width: 340px) {
  .auth-auto-register { font-size: 10px; }
  .auth-divider { margin-top: 18px; }
  .auth-agreement-spacing { margin-top: 13px; }
  .auth-help-spacing { margin-top: 14px; }
}
</style>
