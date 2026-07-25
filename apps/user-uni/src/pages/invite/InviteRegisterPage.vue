<template>
  <view class="invite-page" :style="miniProgramNavigationStyle">
    <view class="invite-glow" />
    <view class="invite-shell">
      <view class="invite-brand">
        <image class="invite-logo" :src="loginLogo" mode="aspectFit" />
        <view>
          <text class="invite-brand-name">知启云AI</text>
          <text class="invite-brand-tag">企业级AI创作与智能体平台</text>
        </view>
      </view>

      <view v-if="loading" class="invite-card invite-state">
        <view class="invite-spinner" />
        <text class="invite-state-title">正在验证专属邀请</text>
      </view>

      <view v-else-if="loadError || !invite?.valid" class="invite-card invite-state">
        <text class="invite-state-icon">!</text>
        <text class="invite-state-title">邀请链接已失效</text>
        <text class="invite-state-copy">{{ loadError || "该邀请码不存在、已停用或对应代理商暂不可邀请新用户。" }}</text>
        <button class="invite-button invite-secondary" @click="loadInvite">重新验证</button>
      </view>

      <template v-else>
        <view class="invite-hero">
          <text class="invite-kicker">专属邀请</text>
          <text class="invite-title">释放你的 AI 创作力</text>
          <text class="invite-subtitle">{{ invite.activityIntro }}</text>
          <view class="invite-agent">
            <text class="invite-agent-dot" />
            <text>您正在接受 <strong>{{ invite.agentDisplayName }}</strong> 的邀请</text>
          </view>
        </view>

        <view v-if="!result" class="invite-card invite-form">
          <text class="invite-card-title">手机号注册</text>
          <text class="invite-card-copy">注册后，使用同一手机号即可登录安卓 APP</text>
          <view class="invite-field">
            <text class="invite-field-prefix">+86</text>
            <input v-model="mobile" type="number" maxlength="11" placeholder="请输入手机号" />
          </view>
          <view class="invite-field">
            <input v-model="smsCode" type="number" maxlength="6" placeholder="请输入短信验证码" />
            <button class="invite-sms" :disabled="sending || countdown > 0" @click="sendSMS">
              {{ countdown > 0 ? `${countdown}s` : sending ? "发送中" : "获取验证码" }}
            </button>
          </view>
          <view class="invite-agreement" @click="accepted = !accepted">
            <text class="invite-checkbox" :class="{ checked: accepted }">{{ accepted ? "✓" : "" }}</text>
            <text>我已阅读并同意</text>
            <text class="invite-link" @click.stop="openLegalDocument('user-agreement')">《用户协议》</text>
            <text>和</text>
            <text class="invite-link" @click.stop="openLegalDocument('privacy-policy')">《隐私政策》</text>
          </view>
          <text v-if="submitError" class="invite-error">{{ submitError }}</text>
          <button class="invite-button" :disabled="submitting" @click="register">
            {{ submitting ? "正在安全注册…" : "注册并下载安卓 APP" }}
          </button>
          <text class="invite-security">手机号验证成功后才会建立代理关系，且关系默认锁定</text>
        </view>

        <view v-else class="invite-card invite-success">
          <text class="invite-success-mark">✓</text>
          <text class="invite-state-title">注册成功</text>
          <text class="invite-state-copy">代理关系已安全绑定。请使用 {{ maskedMobile }} 登录知启云AI APP。</text>
          <view v-if="release" class="invite-release">
            <view><text>安卓正式版 {{ release.versionName }}</text><text>{{ formatSize(release.fileSize) }}</text></view>
            <text>SHA-256：{{ shortHash(release.sha256) }}</text>
          </view>
          <view v-if="isWeChat" class="invite-platform-tip warning">
            <text>微信内无法直接安装 APK</text>
            <text>请点击右上角「…」，选择“在浏览器中打开”</text>
          </view>
          <view v-else-if="isIOS" class="invite-platform-tip">
            <text>iOS 版本敬请期待</text>
            <text>你可以先使用知启云AI网页端，同一手机号账号数据互通。</text>
          </view>
          <button v-else class="invite-button" :disabled="!result.downloadPage.downloadUrl" @click="downloadAPK">
            下载最新版 APK
          </button>
          <text v-if="!isIOS" class="invite-security">安装时如系统提示风险，请确认下载入口来自 ai.zs-kjhn.cn</text>
        </view>

        <view class="invite-capabilities">
          <view><text>AI</text><strong>智能创作</strong><small>图片、视频、PPT 一站完成</small></view>
          <view><text>知</text><strong>企业知识</strong><small>知识库与智能体协同工作</small></view>
          <view><text>云</text><strong>多端同步</strong><small>H5、小程序与 APP 数据互通</small></view>
        </view>
      </template>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { openLegalDocument } from "../../features/legal/navigation";
import {
  inviteRegistrationAPI,
  type AndroidRelease,
  type InviteRegistrationResult,
  type PublicInviteInfo,
} from "../../features/invite/api";

const inviteCode = ref("");
const invite = ref<PublicInviteInfo | null>(null);
const loading = ref(true);
const loadError = ref("");
const mobile = ref("");
const smsCode = ref("");
const accepted = ref(false);
const sending = ref(false);
const submitting = ref(false);
const countdown = ref(0);
const submitError = ref("");
const result = ref<InviteRegistrationResult | null>(null);
const release = ref<AndroidRelease | null>(null);
const isWeChat = ref(false);
const isIOS = ref(false);
let countdownTimer: ReturnType<typeof setInterval> | null = null;

const maskedMobile = computed(() => {
  const value = mobile.value.replace(/\D/g, "");
  return value.length === 11 ? `${value.slice(0, 3)}****${value.slice(-4)}` : "该手机号";
});

onLoad((options) => {
  inviteCode.value = String(options?.inviteCode || options?.invite || "").trim().toUpperCase();
  // #ifdef H5
  const ua = navigator.userAgent.toLowerCase();
  isWeChat.value = ua.includes("micromessenger");
  isIOS.value = /iphone|ipad|ipod/.test(ua);
  // #endif
  void loadInvite();
});

onBeforeUnmount(() => {
  if (countdownTimer) clearInterval(countdownTimer);
});

async function loadInvite() {
  loading.value = true;
  loadError.value = "";
  try {
    if (!inviteCode.value) throw new Error("邀请链接缺少邀请码");
    invite.value = await inviteRegistrationAPI.resolve(inviteCode.value);
  } catch (error) {
    invite.value = null;
    loadError.value = error instanceof Error ? error.message : "邀请链接验证失败";
  } finally {
    loading.value = false;
  }
}

async function sendSMS() {
  submitError.value = "";
  if (!/^1[3-9]\d{9}$/.test(mobile.value)) {
    submitError.value = "请输入正确的11位手机号";
    return;
  }
  sending.value = true;
  try {
    const response = await inviteRegistrationAPI.sendSMS(mobile.value);
    countdown.value = response.retryAfterSeconds || 60;
    countdownTimer = setInterval(() => {
      countdown.value -= 1;
      if (countdown.value <= 0 && countdownTimer) {
        clearInterval(countdownTimer);
        countdownTimer = null;
      }
    }, 1000);
    uni.showToast({ title: "验证码已发送", icon: "success" });
  } catch (error) {
    submitError.value = error instanceof Error ? error.message : "验证码发送失败";
  } finally {
    sending.value = false;
  }
}

function registrationKey() {
  const storageKey = `agent-invite-register:${inviteCode.value}:${mobile.value}`;
  const existing = String(uni.getStorageSync(storageKey) || "");
  if (existing) return existing;
  const cryptoRuntime = globalThis.crypto;
  const created = cryptoRuntime?.randomUUID
    ? cryptoRuntime.randomUUID()
    : `invite_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}_${Math.random().toString(36).slice(2)}`;
  uni.setStorageSync(storageKey, created);
  return created;
}

async function register() {
  submitError.value = "";
  if (!/^1[3-9]\d{9}$/.test(mobile.value)) {
    submitError.value = "请输入正确的11位手机号";
    return;
  }
  if (!/^\d{4,6}$/.test(smsCode.value)) {
    submitError.value = "请输入短信验证码";
    return;
  }
  if (!accepted.value) {
    submitError.value = "请先阅读并同意用户协议和隐私政策";
    return;
  }
  submitting.value = true;
  try {
    result.value = await inviteRegistrationAPI.register({
      inviteCode: inviteCode.value,
      mobile: mobile.value,
      smsCode: smsCode.value,
      agreementAccepted: true,
      privacyAccepted: true,
      idempotencyKey: registrationKey(),
    });
    release.value = await inviteRegistrationAPI.latestAndroid().catch(() => null);
  } catch (error) {
    submitError.value = error instanceof Error ? error.message : "注册失败，请稍后重试";
  } finally {
    submitting.value = false;
  }
}

function downloadAPK() {
  if (!result.value?.downloadPage.downloadUrl) return;
  // #ifdef H5
  window.location.href = inviteRegistrationAPI.absoluteURL(result.value.downloadPage.downloadUrl);
  // #endif
}

function formatSize(bytes: number) {
  if (!bytes) return "文件大小待发布时确认";
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function shortHash(value: string) {
  return value ? `${value.slice(0, 10)}…${value.slice(-8)}` : "待发布时确认";
}
</script>

<style scoped>
.invite-page{min-height:100vh;padding-top:var(--header-height,64px);background:#07101f;color:#eef4ff;position:relative;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC",sans-serif}.invite-glow{position:absolute;width:560rpx;height:560rpx;right:-240rpx;top:-180rpx;border-radius:50%;background:radial-gradient(circle,rgba(74,125,255,.33),transparent 68%);pointer-events:none}.invite-shell{position:relative;z-index:1;width:min(100%,760px);box-sizing:border-box;margin:0 auto;padding:46rpx 30rpx 80rpx}.invite-brand{display:flex;align-items:center;gap:18rpx;margin-bottom:56rpx}.invite-logo{width:72rpx;height:72rpx}.invite-brand-name,.invite-brand-tag{display:block}.invite-brand-name{font-size:34rpx;font-weight:800;letter-spacing:1rpx}.invite-brand-tag{font-size:21rpx;color:#8ea1bd;margin-top:4rpx}.invite-hero{padding:8rpx 6rpx 38rpx}.invite-kicker{display:inline-flex;padding:8rpx 18rpx;border:1px solid rgba(104,145,255,.42);border-radius:999px;color:#9ab5ff;font-size:22rpx}.invite-title{display:block;font-size:58rpx;line-height:1.16;font-weight:900;letter-spacing:-2rpx;margin:26rpx 0 16rpx}.invite-subtitle{display:block;color:#aab8ce;font-size:27rpx;line-height:1.75}.invite-agent{display:flex;align-items:center;gap:12rpx;margin-top:26rpx;color:#d6e1f4;font-size:25rpx}.invite-agent-dot{width:13rpx;height:13rpx;border-radius:50%;background:#45d6a7;box-shadow:0 0 0 8rpx rgba(69,214,167,.12)}.invite-card{background:linear-gradient(145deg,rgba(20,34,58,.96),rgba(12,24,43,.96));border:1px solid rgba(134,165,216,.18);border-radius:32rpx;box-shadow:0 30rpx 80rpx rgba(0,0,0,.24);padding:36rpx}.invite-card-title{display:block;font-size:34rpx;font-weight:800}.invite-card-copy{display:block;color:#8fa2bd;font-size:23rpx;margin:10rpx 0 30rpx}.invite-field{display:flex;align-items:center;height:94rpx;background:#0a1729;border:1px solid #253a58;border-radius:20rpx;margin-top:20rpx;padding:0 24rpx;box-sizing:border-box}.invite-field input{flex:1;color:#f4f7ff;font-size:29rpx;height:100%}.invite-field-prefix{padding-right:22rpx;margin-right:20rpx;border-right:1px solid #30445f;color:#c4d0e2}.invite-sms{background:transparent;color:#86a7ff;font-size:24rpx;padding:12rpx;margin:0;border:0}.invite-sms::after,.invite-button::after{border:0}.invite-agreement{display:flex;align-items:center;flex-wrap:wrap;gap:5rpx;color:#8294ae;font-size:21rpx;line-height:1.7;margin:25rpx 2rpx}.invite-checkbox{width:30rpx;height:30rpx;border:1px solid #496181;border-radius:8rpx;display:inline-flex;align-items:center;justify-content:center;margin-right:6rpx}.invite-checkbox.checked{background:#5f86ff;border-color:#5f86ff;color:white}.invite-link{color:#9bb5ff}.invite-button{display:flex;align-items:center;justify-content:center;width:100%;height:94rpx;border-radius:20rpx;background:linear-gradient(135deg,#527cff,#7a5cff);color:white;font-size:29rpx;font-weight:750;border:0;box-shadow:0 18rpx 36rpx rgba(79,109,255,.25)}.invite-button[disabled]{opacity:.55}.invite-secondary{background:#182a43;box-shadow:none;margin-top:28rpx}.invite-security{display:block;text-align:center;color:#70829c;font-size:20rpx;margin-top:20rpx;line-height:1.6}.invite-error{display:block;color:#ff8f9d;font-size:23rpx;margin:-8rpx 0 18rpx}.invite-state{text-align:center;padding:70rpx 36rpx}.invite-spinner{width:54rpx;height:54rpx;border:5rpx solid #263c5b;border-top-color:#6f91ff;border-radius:50%;margin:0 auto 28rpx;animation:spin .8s linear infinite}.invite-state-icon,.invite-success-mark{width:72rpx;height:72rpx;display:flex;align-items:center;justify-content:center;margin:0 auto 24rpx;border-radius:50%;font-size:40rpx;font-weight:800}.invite-state-icon{background:#442733;color:#ff91a0}.invite-success-mark{background:rgba(61,213,160,.15);color:#4bdca9}.invite-state-title{display:block;font-size:34rpx;font-weight:800}.invite-state-copy{display:block;color:#93a4bc;font-size:24rpx;line-height:1.7;margin-top:16rpx}.invite-success{text-align:center}.invite-release{text-align:left;background:#0a1729;border-radius:20rpx;padding:24rpx;margin:30rpx 0}.invite-release view{display:flex;justify-content:space-between;color:#e4ecfa;font-size:24rpx}.invite-release>text{display:block;color:#7185a2;font-size:18rpx;margin-top:14rpx;word-break:break-all}.invite-platform-tip{padding:26rpx;border-radius:20rpx;background:#172c43;color:#b8d8ff;margin:24rpx 0}.invite-platform-tip.warning{background:#3b2d18;color:#ffd998}.invite-platform-tip text{display:block;font-size:24rpx;line-height:1.7}.invite-capabilities{display:grid;grid-template-columns:repeat(3,1fr);gap:14rpx;margin-top:28rpx}.invite-capabilities view{background:rgba(17,31,52,.72);border:1px solid rgba(121,151,201,.12);border-radius:22rpx;padding:24rpx 18rpx}.invite-capabilities text{display:flex;width:48rpx;height:48rpx;align-items:center;justify-content:center;background:#1b3153;color:#89a8ff;border-radius:14rpx;font-weight:800}.invite-capabilities strong,.invite-capabilities small{display:block}.invite-capabilities strong{font-size:23rpx;margin:18rpx 0 7rpx}.invite-capabilities small{font-size:18rpx;line-height:1.55;color:#7588a3}@keyframes spin{to{transform:rotate(360deg)}}@media(min-width:700px){.invite-shell{padding-top:70px}.invite-card{padding:34px}.invite-title{font-size:48px}}
</style>
