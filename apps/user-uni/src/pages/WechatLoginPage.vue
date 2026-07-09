<template>
  <view class="mini-login-page">
    <view class="mini-brand">
      <image class="mini-logo" :src="loginLogo" mode="aspectFit" />
      <view>
        <text class="mini-eyebrow">{{ texts.brandEyebrow }}</text>
        <text class="mini-product">{{ texts.productName }}</text>
      </view>
    </view>

    <view class="mini-heading">
      <text class="mini-title">{{ texts.title }}</text>
      <text class="mini-copy">{{ texts.copy }}</text>
    </view>

    <view class="mini-card">
      <view class="mini-form">
        <view class="mini-field">
          <text>{{ texts.emailLabel }}</text>
          <input v-model="loginEmail" type="text" placeholder="demo@xianzhi.ai" />
        </view>
        <view class="mini-field">
          <text>{{ texts.passwordLabel }}</text>
          <input v-model="loginPassword" type="password" placeholder="Password" />
        </view>

        <button
          type="button"
          class="tap-test-button"
          hover-class="button-hover"
          @click="testTap"
        >
          <text>{{ texts.tapTestButton }} {{ tapCount }}</text>
        </button>

        <button
          type="button"
          :class="['login-submit', { disabled: isBusy }]"
          :disabled="isBusy"
          hover-class="button-hover"
          @click="loginWithPassword"
        >
          <text>{{ passwordLoginLoading ? texts.passwordLoading : texts.passwordButton }}</text>
        </button>

        <view class="mini-divider">
          <view></view>
          <text>{{ texts.orText }}</text>
          <view></view>
        </view>

        <button
          type="button"
          :class="['wechat-login-button', { disabled: isBusy }]"
          :disabled="isBusy"
          hover-class="button-hover"
          @click="loginWithWechatMiniProgram"
        >
          <view class="wechat-login-icon" aria-hidden="true"></view>
          <text>{{ wechatLoginLoading ? texts.wechatLoading : texts.wechatButton }}</text>
        </button>

        <button
          v-if="enableMockLogin"
          type="button"
          :class="['mock-login-button', { disabled: isBusy }]"
          :disabled="isBusy"
          hover-class="button-hover"
          @click="loginWithMockCode"
        >
          <text>{{ mockLoginLoading ? texts.mockLoading : texts.mockButton }}</text>
        </button>

        <view :class="['mini-status', statusTone]">
          <text>{{ statusText }}</text>
        </view>

        <view class="mini-debug">
          <text>API: {{ apiEndpointLabel }}</text>
          <text>{{ texts.debugHint }}</text>
        </view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { authService, getApiBaseURL } from "../api/client";
import loginLogo from "../assets/zhiqiyun-logo-transparent.png";
import type { AuthResponse } from "../types";

type StatusTone = "idle" | "loading" | "success" | "error";

const texts = {
  brandEyebrow: "WeChat Mini Program",
  productName: "Zhiqiyun AI",
  title: "Sign in",
  copy: "Tap a button below. This debug page shows each login step on screen.",
  emailLabel: "Email",
  passwordLabel: "Password",
  tapTestButton: "Tap test",
  passwordButton: "Password login",
  passwordLoading: "Password login...",
  orText: "or",
  wechatButton: "Real wx.login",
  wechatLoading: "WeChat login...",
  mockButton: "Mock login test",
  mockLoading: "Mock login...",
  debugHint: "Use this page to locate tap, wx.login, domain, or backend errors."
};

const loginEmail = ref("demo@xianzhi.ai");
const loginPassword = ref("Demo123!");
const tapCount = ref(0);
const passwordLoginLoading = ref(false);
const wechatLoginLoading = ref(false);
const mockLoginLoading = ref(false);
const statusText = ref("Tap a login button to start.");
const statusTone = ref<StatusTone>("idle");
const apiEndpointLabel = computed(() => getApiBaseURL() || "same-origin API");
const isBusy = computed(() => passwordLoginLoading.value || wechatLoginLoading.value || mockLoginLoading.value);
const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {};
const enableMockLogin = Boolean(rawEnv.DEV === true && String(rawEnv.VITE_ENABLE_MOCK_LOGIN || "").toLowerCase() === "true");

function setStatus(text: string, tone: StatusTone = "idle") {
  statusText.value = text;
  statusTone.value = tone;
}

function testTap() {
  tapCount.value += 1;
  setStatus(`Tap event OK. Count=${tapCount.value}`, "success");
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (error && typeof error === "object" && "errMsg" in error) {
    const errMsg = (error as { errMsg?: unknown }).errMsg;
    if (typeof errMsg === "string" && errMsg.trim()) return errMsg;
  }
  return fallback;
}

function completeLogin(auth: AuthResponse, source: string) {
  const token = auth.accessToken || "";
  if (!token) {
    throw new Error("missing accessToken");
  }
  authService.storage.setToken(token);
  authService.storage.setAuth(auth);
  const name = auth.user?.name || auth.user?.email || "current user";
  setStatus(`${source} success: ${name}, workspace=${auth.workspace || "-"}`, "success");
  uni.showToast({ title: "Login OK", icon: "success" });
  setTimeout(() => {
    uni.reLaunch({
      url: "/pages/MiniProgramHomePage",
      fail: error => {
        setStatus(`Login succeeded, but workspace redirect failed: ${errorMessage(error, "redirect failed")}`, "error");
      }
    });
  }, 300);
}

function requestWechatMiniProgramCode() {
  return new Promise<string>((resolve, reject) => {
    setStatus("Calling wx.login for temporary code...", "loading");
    uni.login({
      provider: "weixin",
      success: result => {
        console.info("[wechat-login] wx.login success", {
          hasCode: Boolean(result.code),
          errMsg: result.errMsg
        });
        if (result.code) {
          setStatus("Got wx code. Calling backend login API...", "loading");
          resolve(result.code);
          return;
        }
        reject(new Error("wx.login returned no code"));
      },
      fail: error => {
        console.error("[wechat-login] wx.login failed", error);
        reject(new Error(errorMessage(error, "wx.login failed")));
      }
    });
  });
}

async function loginWithPassword() {
  if (isBusy.value) {
    setStatus("Busy, please wait...", "loading");
    return;
  }
  if (!loginEmail.value.trim() || !loginPassword.value.trim()) {
    setStatus("Please enter email and password.", "error");
    uni.showToast({ title: "Need account", icon: "none" });
    return;
  }

  passwordLoginLoading.value = true;
  setStatus("Submitting password login...", "loading");
  try {
    const auth = await authService.loginByPassword(loginEmail.value.trim(), loginPassword.value.trim());
    completeLogin(auth, "Password login");
  } catch (error) {
    const message = errorMessage(error, "login failed");
    setStatus(`Password login failed: ${message}`, "error");
    uni.showToast({ title: message, icon: "none" });
  } finally {
    passwordLoginLoading.value = false;
  }
}

async function loginWithWechatMiniProgram() {
  if (isBusy.value) {
    setStatus("Busy, please wait...", "loading");
    return;
  }
  wechatLoginLoading.value = true;
  try {
    const code = await requestWechatMiniProgramCode();
    const auth = await authService.loginByWechatMiniProgramCode(code);
    completeLogin(auth, "WeChat login");
  } catch (error) {
    const message = errorMessage(error, "WeChat login failed");
    setStatus(`WeChat login failed: ${message}`, "error");
    uni.showToast({ title: message, icon: "none" });
  } finally {
    wechatLoginLoading.value = false;
  }
}

async function loginWithMockCode() {
  if (!enableMockLogin) {
    setStatus("Mock login is disabled for this build.", "error");
    return;
  }
  if (isBusy.value) {
    setStatus("Busy, please wait...", "loading");
    return;
  }
  mockLoginLoading.value = true;
  setStatus("Calling backend with mock-devtools-code...", "loading");
  try {
    const auth = await authService.loginByWechatMiniProgramCode("mock-devtools-code");
    completeLogin(auth, "Mock login");
  } catch (error) {
    const message = errorMessage(error, "mock login failed");
    setStatus(`Mock login failed: ${message}`, "error");
    uni.showToast({ title: message, icon: "none" });
  } finally {
    mockLoginLoading.value = false;
  }
}
</script>

<style scoped>
.mini-login-page {
  min-height: 100vh;
  padding: 52px 24px 28px;
  box-sizing: border-box;
  color: #10233f;
  background:
    linear-gradient(150deg, rgba(37, 99, 235, 0.12), transparent 34%),
    linear-gradient(28deg, rgba(245, 111, 45, 0.1), transparent 30%),
    #f6f9fe;
}

.mini-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.mini-logo {
  width: 74px;
  height: 74px;
  flex: 0 0 74px;
}

.mini-brand view,
.mini-heading,
.mini-form,
.mini-field,
.mini-debug {
  display: flex;
  flex-direction: column;
}

.mini-brand view {
  gap: 5px;
}

.mini-eyebrow {
  color: #2563eb;
  font-size: 13px;
  font-weight: 800;
}

.mini-product {
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.mini-heading {
  margin-top: 18px;
  gap: 10px;
}

.mini-title {
  font-size: 30px;
  line-height: 1.15;
  font-weight: 900;
  color: #101c35;
}

.mini-copy {
  color: #65738c;
  font-size: 14px;
  line-height: 1.7;
}

.mini-card {
  margin-top: 26px;
  padding: 18px;
  border: 1px solid rgba(24, 50, 88, 0.1);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 18px 42px rgba(22, 44, 78, 0.12);
}

.mini-form {
  gap: 14px;
}

.mini-field {
  gap: 8px;
  color: #4a5a70;
  font-size: 13px;
  font-weight: 700;
}

.mini-field input {
  height: 46px;
  padding: 0 14px;
  box-sizing: border-box;
  border: 1px solid #d7dfec;
  border-radius: 8px;
  background: #f9fbff;
  color: #10233f;
  font-size: 15px;
}

.tap-test-button,
.login-submit,
.wechat-login-button,
.mock-login-button {
  height: 48px;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 800;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0;
  padding: 0 14px;
  line-height: 1;
}

.tap-test-button::after,
.login-submit::after,
.wechat-login-button::after,
.mock-login-button::after {
  display: none;
}

.tap-test-button {
  color: #1d4ed8;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}

.login-submit {
  margin-top: 4px;
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #1ea7a1);
}

.wechat-login-button {
  gap: 10px;
  color: #10233f;
  background: #fff;
  border: 1px solid rgba(31, 178, 116, 0.34);
}

.mock-login-button {
  color: #475467;
  background: #f8fafc;
  border: 1px dashed #cbd5e1;
}

.button-hover {
  opacity: 0.78;
}

.login-submit.disabled,
.wechat-login-button.disabled,
.mock-login-button.disabled {
  opacity: 0.66;
}

.mini-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #8b98aa;
  font-size: 12px;
}

.mini-divider view {
  height: 1px;
  flex: 1;
  background: #e2e8f3;
}

.wechat-login-icon {
  width: 23px;
  height: 18px;
  border-radius: 50%;
  background: #16a34a;
  position: relative;
}

.wechat-login-icon::before,
.wechat-login-icon::after {
  content: "";
  position: absolute;
  top: 7px;
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: #fff;
}

.wechat-login-icon::before {
  left: 7px;
}

.wechat-login-icon::after {
  right: 7px;
}

.mini-status {
  padding: 12px;
  border-radius: 8px;
  color: #475467;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
}

.mini-status.loading {
  color: #1d4ed8;
  background: #eff6ff;
  border-color: #bfdbfe;
}

.mini-status.success {
  color: #047857;
  background: #ecfdf3;
  border-color: #bbf7d0;
}

.mini-status.error {
  color: #b42318;
  background: #fef3f2;
  border-color: #fecaca;
}

.mini-debug {
  gap: 6px;
  color: #8b98aa;
  font-size: 11px;
  line-height: 1.5;
}
</style>
