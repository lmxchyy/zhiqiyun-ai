<template><view class="enterprise-entry"><EnterpriseStatePanel :kind="stateKind" :title="stateTitle" :copy="stateCopy" action-label="重新加载" @action="enterEnterprise()" /></view></template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiClientError } from "@xianzhi/api-client";
import EnterpriseStatePanel from "../../components/enterprise/EnterpriseStatePanel.vue";
import { miniProgramEnterprisePages as pages } from "../../config/miniProgramPages";
import { useUserStore } from "../../stores/user";

type EntryState = "loading" | "error" | "forbidden";
const userStore = useUserStore();
const stateKind = ref<EntryState>("loading");
const stateTitle = ref("正在进入企业中心");
const stateCopy = ref("正在同步企业工作空间");

function replace(url: string) { uni.redirectTo({ url, fail: () => uni.reLaunch({ url }) }); }

async function enterEnterprise() {
  stateKind.value = "loading"; stateTitle.value = "正在进入企业中心"; stateCopy.value = "正在同步企业工作空间";
  try {
    await userStore.loadProfile(true);
    const payload = await userStore.loadEnterpriseContexts();
    const contexts = payload.contexts.filter(item => item.type === "ENTERPRISE");
    if (!contexts.length) { replace(pages.onboarding); return; }
    if (contexts.length > 1) { replace(pages.switcher); return; }
    const context = contexts[0];
    if (context.memberStatus !== "ACTIVE") { replace(`${pages.status}?state=disabled&reason=${encodeURIComponent("当前企业成员状态已停用")}`); return; }
    if (!context.current) await userStore.switchContext({ type: "ENTERPRISE", tenantId: context.tenantId, organizationId: context.organizationId, role: context.currentRole });
    replace(pages.overview);
  } catch (error) {
    stateKind.value = error instanceof ApiClientError && error.statusCode === 403 ? "forbidden" : "error";
    stateTitle.value = stateKind.value === "forbidden" ? "暂无企业权限" : "企业中心加载失败";
    stateCopy.value = error instanceof Error ? error.message : "请稍后重试";
  }
}

onMounted(() => { void enterEnterprise(); });
</script>

<style src="../../styles/enterprise-center.css"></style>
<style>.enterprise-entry { min-height: 100vh; padding-top: calc(var(--status-bar-height, env(safe-area-inset-top)) + 80px); box-sizing: border-box; background: #f7f8fc; }</style>
