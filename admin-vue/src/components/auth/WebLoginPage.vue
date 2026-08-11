<template>
  <main class="web-auth-page">
    <section class="web-auth-hero">
      <a class="web-auth-brand" href="/"><img :src="logo" alt="知启云 AI" /><span><strong>知启云 AI</strong><small>ZHIQIYUN AI</small></span></a>
      <div class="web-auth-copy"><span class="web-auth-eyebrow">企业级 AI 创作与智能体平台</span><h1>让 AI 成为<br />企业生产力</h1><p>从灵感、内容创作到智能体与知识库，让团队在一个工作台完成 AI 生产。</p></div>
      <div class="web-auth-capabilities"><span v-for="item in capabilities" :key="item">{{ item }}</span></div>
      <div class="web-product-preview" aria-label="知启云 AI 产品工作台预览">
        <aside><i></i><i></i><i></i><i></i></aside>
        <div><header><b>AI 创作工作台</b><em>在线</em></header><section><article><small>今日创作</small><strong>128</strong></article><article><small>可用模型</small><strong>36</strong></article><article class="wide"><small>输入你的创作想法</small><p>为新品发布会生成一组科技感主视觉…</p></article></section></div>
      </div>
    </section>
    <section class="web-auth-form-column">
      <a class="web-auth-back" href="/">← 返回首页</a>
      <div class="web-auth-card"><header><img :src="logo" alt="" /><div><strong>欢迎使用知启云 AI</strong><span>选择适合你的方式登录</span></div></header><WebLoginForm :register-href="registerHref" :initial-mode="preferredMode" @authenticated="handleAuthenticated" /></div>
      <small class="web-auth-footer">登录即代表你已了解账号会在网页端与小程序端同步使用。</small>
    </section>
  </main>
</template>

<script setup lang="ts">
import logo from "../../assets/xianzhi-ai-logo.webp";
import WebLoginForm from "./WebLoginForm.vue";
defineProps<{ registerHref?: string }>();
const emit = defineEmits<{ authenticated: [response: unknown, remember: boolean] }>();
const capabilities = ["AI 对话", "AI 生图", "AI 视频", "PPT 生成", "数字员工", "企业知识库"];
const preferredMode = typeof window !== "undefined" && window.location.pathname.startsWith("/admin")
  ? "password" as const
  : undefined;
function handleAuthenticated(response: unknown, remember: boolean) { emit("authenticated", response, remember); }
</script>

<style scoped>
.web-auth-page{min-height:100vh;display:grid;grid-template-columns:minmax(0,1.18fr) minmax(420px,.82fr);background:#f7f8fc}.web-auth-hero{position:relative;overflow:hidden;display:flex;flex-direction:column;padding:54px clamp(42px,6vw,92px);color:#fff;background:radial-gradient(circle at 80% 15%,rgba(116,100,242,.7),transparent 36%),linear-gradient(145deg,#151b3d,#27245f 58%,#4b3cc4)}.web-auth-hero::after{content:"";position:absolute;inset:auto -160px -240px auto;width:560px;height:560px;border:1px solid rgba(255,255,255,.18);border-radius:50%}.web-auth-brand{position:relative;z-index:1;display:flex;align-items:center;gap:12px;color:#fff;text-decoration:none}.web-auth-brand img{width:54px;height:54px;object-fit:contain;border-radius:14px;background:#fff}.web-auth-brand span{display:grid}.web-auth-brand strong{font-size:22px}.web-auth-brand small{letter-spacing:.12em;opacity:.65}.web-auth-copy{position:relative;z-index:1;margin-top:clamp(58px,9vh,110px);max-width:620px}.web-auth-eyebrow{display:inline-flex;padding:8px 13px;border:1px solid rgba(255,255,255,.24);border-radius:999px;background:rgba(255,255,255,.08);font-size:13px}.web-auth-copy h1{margin:24px 0 18px;font-size:clamp(48px,5vw,76px);line-height:1.08;letter-spacing:-.05em}.web-auth-copy p{max-width:570px;margin:0;color:rgba(255,255,255,.72);font-size:17px;line-height:1.8}.web-auth-capabilities{position:relative;z-index:1;display:flex;flex-wrap:wrap;gap:9px;margin-top:30px}.web-auth-capabilities span{padding:8px 12px;border-radius:9px;background:rgba(255,255,255,.1);font-size:13px}.web-product-preview{position:relative;z-index:1;display:grid;grid-template-columns:58px 1fr;min-height:220px;margin-top:auto;border:1px solid rgba(255,255,255,.18);border-radius:20px;background:rgba(12,17,47,.52);box-shadow:0 30px 70px rgba(4,8,28,.36);backdrop-filter:blur(14px)}.web-product-preview aside{display:grid;align-content:start;justify-items:center;gap:14px;padding-top:24px;border-right:1px solid rgba(255,255,255,.1)}.web-product-preview aside i{width:22px;height:22px;border-radius:7px;background:rgba(255,255,255,.16)}.web-product-preview>div{padding:20px}.web-product-preview header{display:flex;justify-content:space-between}.web-product-preview header em{color:#75e0a7;font-style:normal}.web-product-preview section{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:20px}.web-product-preview article{display:grid;gap:8px;padding:16px;border-radius:14px;background:rgba(255,255,255,.08)}.web-product-preview article strong{font-size:28px}.web-product-preview article small{color:rgba(255,255,255,.62)}.web-product-preview article.wide{grid-column:1/-1}.web-product-preview article p{margin:0;color:rgba(255,255,255,.8)}.web-auth-form-column{display:flex;flex-direction:column;justify-content:center;padding:42px clamp(30px,6vw,92px)}.web-auth-back{align-self:flex-start;margin-bottom:24px;color:#667085;text-decoration:none}.web-auth-card{width:min(470px,100%);box-sizing:border-box;padding:30px;border:1px solid #eaecf0;border-radius:24px;background:#fff;box-shadow:0 24px 70px rgba(16,24,40,.12)}.web-auth-card>header{display:flex;align-items:center;gap:12px;margin-bottom:24px}.web-auth-card>header img{width:50px;height:50px;object-fit:contain}.web-auth-card>header div{display:grid;gap:3px}.web-auth-card>header strong{color:#101828;font-size:20px}.web-auth-card>header span{color:#667085;font-size:13px}.web-auth-footer{width:min(470px,100%);margin-top:18px;color:#98a2b3;text-align:center;line-height:1.6}@media(max-width:960px){.web-auth-page{grid-template-columns:1fr}.web-auth-hero{display:none}.web-auth-form-column{min-height:100vh;align-items:center;padding:24px 16px;background:radial-gradient(circle at 15% 10%,#e9e7ff,transparent 35%),#f7f8fc}.web-auth-back{width:min(470px,100%)}.web-auth-card{padding:24px 18px}}
</style>
