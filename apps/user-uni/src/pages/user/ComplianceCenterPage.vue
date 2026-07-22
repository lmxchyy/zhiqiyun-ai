<template>
  <view class="compliance-page">
    <view class="compliance-header">
      <button class="back" aria-label="返回" @click="back">‹</button>
      <view>
        <text class="title">协议与安全</text>
        <text class="subtitle">知启云AI小程序合规信息</text>
      </view>
    </view>

    <view v-if="loading" class="state">正在加载...</view>
    <view v-else-if="error" class="state error">
      <text>{{ error }}</text>
      <button class="retry" @click="load">重新加载</button>
    </view>
    <view v-else class="document-list">
      <button v-for="item in documents" :key="item.code" class="document" @click="openDocument(item)">
        <view>
          <text class="document-title">{{ item.title }}</text>
          <text class="document-version">版本 {{ item.version }}</text>
        </view>
        <text>›</text>
      </button>

      <view class="acceptance-card">
        <text class="acceptance-title">首次生成前协议确认</text>
        <text class="acceptance-summary">
          {{ acceptanceReady ? "当前必要协议已确认" : "请阅读并确认当前发布版本；协议更新后需要重新确认。" }}
        </text>
        <view v-for="item in acceptanceItems" :key="`accept-${item.code}`" class="acceptance-row">
          <text>{{ item.title }}（{{ item.version }}）</text>
          <text :class="item.accepted ? 'accepted' : 'pending'">{{ item.accepted ? "已确认" : "待确认" }}</text>
        </view>
        <button
          v-if="!acceptanceReady"
          class="accept-button"
          :disabled="accepting || acceptanceItems.length !== 3"
          @click="acceptCurrent"
        >
          {{ accepting ? "正在提交..." : "我已阅读并同意以上协议" }}
        </button>
      </view>

      <button class="document" @click="openExternal(complaintUrl, '投诉举报')">
        <text class="document-title">投诉举报</text><text>›</text>
      </button>
      <button class="document" @click="openExternal(infringementUrl, '侵权投诉')">
        <text class="document-title">侵权投诉</text><text>›</text>
      </button>
    </view>

    <view v-if="active" class="sheet" @click="active = null">
      <view class="sheet-card" @click.stop>
        <text class="sheet-title">{{ active.title }}</text>
        <text class="sheet-version">版本 {{ active.version }}</text>
        <scroll-view scroll-y class="sheet-content"><text>{{ active.content }}</text></scroll-view>
        <button class="close" @click="active = null">我已阅读</button>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { apiRequestTask } from "../../api/client";

interface LegalDocument { code: string; title: string; version: string; content: string }
interface AcceptanceDocument { code: string; title: string; version: string; accepted: boolean }

const documents = ref<LegalDocument[]>([]);
const active = ref<LegalDocument | null>(null);
const complaintUrl = ref("");
const infringementUrl = ref("");
const loading = ref(true);
const error = ref("");
const acceptanceItems = ref<AcceptanceDocument[]>([]);
const acceptanceReady = ref(false);
const accepting = ref(false);

onMounted(load);

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [publicPayload, status] = await Promise.all([
      apiRequestTask<{ items: LegalDocument[]; complaintUrl: string; infringementUrl: string }>("/api/v1/public/legal-documents", { auth: false }).promise,
      apiRequestTask<{ ready: boolean; items: AcceptanceDocument[] }>("/api/v1/legal/acceptance-status").promise,
    ]);
    documents.value = publicPayload.items || [];
    complaintUrl.value = publicPayload.complaintUrl || "";
    infringementUrl.value = publicPayload.infringementUrl || "";
    acceptanceReady.value = Boolean(status.ready);
    acceptanceItems.value = status.items || [];
    if (acceptanceItems.value.length !== 3) error.value = "必要协议尚未全部发布，请联系平台管理员";
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "合规信息加载失败";
  } finally {
    loading.value = false;
  }
}

function back() {
  uni.navigateBack({ fail: () => uni.switchTab({ url: "/pages/user/UserMinePage" }) });
}

function openDocument(item: LegalDocument) {
  active.value = item;
}

async function acceptCurrent() {
  if (accepting.value || acceptanceItems.value.length !== 3) return;
  accepting.value = true;
  try {
    const result = await apiRequestTask<{ ready: boolean; items: AcceptanceDocument[] }>("/api/v1/legal/acceptances", { method: "POST" }).promise;
    acceptanceReady.value = Boolean(result.ready);
    acceptanceItems.value = (result.items || []).map(item => ({ ...item, accepted: true }));
    uni.showToast({ title: "协议确认成功", icon: "success" });
    uni.$emit("legal-acceptance-completed");
    setTimeout(back, 500);
  } catch (reason) {
    uni.showToast({ title: reason instanceof Error ? reason.message : "协议确认失败", icon: "none" });
  } finally {
    accepting.value = false;
  }
}

function openExternal(value: string, title: string) {
  if (!value || value === "待配置") {
    uni.showToast({ title: `${title}入口待运营配置`, icon: "none" });
    return;
  }
  // #ifdef H5
  window.location.href = value;
  // #endif
  // #ifndef H5
  uni.setClipboardData({ data: value, success: () => uni.showToast({ title: "入口地址已复制", icon: "none" }) });
  // #endif
}
</script>

<style scoped>
.compliance-page{min-height:100vh;padding:24px 16px;background:#f8fafc;color:#101828}.compliance-header{display:flex;align-items:center;gap:12px;margin-bottom:20px}.back{width:42px;height:42px;padding:0;border:0;border-radius:14px;background:#fff;font-size:30px;line-height:38px}.title,.subtitle,.document-title,.document-version,.sheet-title,.sheet-version{display:block}.title{font-size:22px;font-weight:800}.subtitle,.document-version,.sheet-version{margin-top:3px;color:#667085;font-size:12px}.document-list{display:grid;gap:10px}.document{display:flex;align-items:center;justify-content:space-between;width:100%;padding:16px;border:1px solid #eaecf0;border-radius:14px;background:#fff;text-align:left}.document-title{font-size:15px;font-weight:700}.state{display:grid;gap:14px;padding:30px;text-align:center;color:#667085}.error{color:#b42318}.retry{margin:auto;border:0;border-radius:10px;background:#4f46e5;color:#fff}.sheet{position:fixed;inset:0;z-index:20;display:flex;align-items:flex-end;background:rgba(16,24,40,.45)}.sheet-card{width:100%;max-height:78vh;padding:22px 18px;background:#fff;border-radius:24px 24px 0 0}.sheet-title{font-size:20px;font-weight:800}.sheet-content{height:46vh;margin:18px 0;color:#344054;line-height:1.75;white-space:pre-wrap}.close{width:100%;border:0;border-radius:12px;background:#4f46e5;color:#fff}.acceptance-card{padding:16px;border:1px solid #d0d5dd;border-radius:14px;background:#fff}.acceptance-title,.acceptance-summary{display:block}.acceptance-title{font-weight:800}.acceptance-summary{margin:6px 0 12px;color:#667085;font-size:12px;line-height:1.6}.acceptance-row{display:flex;justify-content:space-between;gap:12px;padding:8px 0;border-top:1px solid #f2f4f7;font-size:12px}.accepted{color:#067647}.pending{color:#b54708}.accept-button{margin-top:12px;width:100%;border:0;border-radius:12px;background:#4f46e5;color:#fff}.accept-button[disabled]{opacity:.5}.close:after,.document:after,.back:after,.retry:after,.accept-button:after{border:0}
</style>
