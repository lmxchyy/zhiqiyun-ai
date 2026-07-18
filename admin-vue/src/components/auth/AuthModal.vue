<template>
  <div v-if="modelValue" class="auth-modal-overlay" role="presentation" @click.self="cancelLogin">
    <section class="auth-modal" role="dialog" aria-modal="true" aria-labelledby="auth-modal-title">
      <button class="auth-modal-close" type="button" aria-label="暂不登录" @click="cancelLogin">×</button>
      <header><img :src="logo" alt="知启云 AI" /><div><strong>知启云 AI</strong><span>登录后继续使用</span></div></header>
      <div class="auth-modal-heading"><h2 id="auth-modal-title">登录后继续使用</h2><p>当前页面和已填写内容会保留，登录成功后可继续刚才的操作。</p></div>
      <ul><li>保存历史作品</li><li>同步创作记录</li><li>查看账户额度</li><li>使用完整模型能力</li></ul>
      <WebLoginForm submit-label="登录并继续" allow-guest @authenticated="handleAuthenticated" @guest="cancelLogin" />
    </section>
  </div>
</template>

<script setup lang="ts">
import logo from "../../assets/xianzhi-ai-logo.webp";
import WebLoginForm from "./WebLoginForm.vue";

defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{ "update:modelValue": [value: boolean]; authenticated: [response: unknown, remember: boolean]; cancelled: [] }>();
function handleAuthenticated(response: unknown, remember: boolean) { emit("authenticated", response, remember); }
function cancelLogin() { emit("update:modelValue", false); emit("cancelled"); }
</script>

<style scoped>
.auth-modal-overlay{position:fixed;inset:0;z-index:6000;display:grid;place-items:center;padding:20px;background:rgba(15,23,42,.58);backdrop-filter:blur(10px)}.auth-modal{position:relative;width:min(520px,100%);max-height:calc(100vh - 40px);overflow:auto;box-sizing:border-box;padding:28px;border:1px solid rgba(255,255,255,.72);border-radius:24px;background:#fff;box-shadow:0 30px 90px rgba(18,24,48,.32)}.auth-modal-close{position:absolute;top:16px;right:16px;width:36px;height:36px;border:0;border-radius:50%;color:#667085;background:#f2f4f7;font-size:24px;cursor:pointer}.auth-modal header{display:flex;align-items:center;gap:12px;padding-right:42px}.auth-modal header img{width:50px;height:50px;object-fit:contain}.auth-modal header div{display:grid}.auth-modal header strong{font-size:19px;color:#101828}.auth-modal header span{font-size:12px;color:#667085}.auth-modal-heading{margin:22px 0 12px}.auth-modal-heading h2{margin:0;color:#101828;font-size:26px}.auth-modal-heading p{margin:8px 0 0;color:#667085;line-height:1.6}.auth-modal ul{display:grid;grid-template-columns:1fr 1fr;gap:8px 18px;margin:0 0 20px;padding:14px 18px 14px 34px;border-radius:14px;color:#475467;background:#f8f7ff;font-size:13px}.auth-modal li::marker{color:#6554e8}@media(max-width:520px){.auth-modal-overlay{align-items:end;padding:0}.auth-modal{width:100%;max-height:92vh;padding:22px 18px;border-radius:24px 24px 0 0}.auth-modal ul{grid-template-columns:1fr}}
</style>
