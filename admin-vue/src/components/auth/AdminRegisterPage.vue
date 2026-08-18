<template>
  <section class="admin-auth-shell">
    <div class="admin-auth-card">
      <div class="admin-auth-brand">
        <img :src="logo" alt="知启云 AI" />
        <div>
          <strong>知启云 AI</strong>
          <span>Invite Register</span>
        </div>
      </div>
      <div class="admin-auth-head">
        <el-tag effect="dark" type="primary">邀请注册</el-tag>
        <h1>注册知启云 AI</h1>
        <p>通过代理商邀请注册后，账号会自动绑定来源渠道。</p>
      </div>

      <form class="admin-auth-form" @submit.prevent="$emit('submit')">
        <label>
          <span>用户名</span>
          <input v-model.trim="username" autocomplete="name" placeholder="请输入用户名" />
        </label>
        <label>
          <span>邮箱</span>
          <input v-model.trim="email" autocomplete="email" placeholder="your@email.com" />
        </label>
        <label>
          <span>密码</span>
          <input v-model="password" autocomplete="new-password" type="password" placeholder="至少 8 位" />
        </label>
        <label>
          <span>确认密码</span>
          <input v-model="confirmPassword" autocomplete="new-password" type="password" placeholder="再次输入密码" />
        </label>
        <label>
          <span>邀请码</span>
          <input v-model.trim="inviteCode" autocomplete="off" placeholder="可选，代理邀请链接会自动带入" />
        </label>
        <label class="admin-auth-check">
          <input v-model="agreementAccepted" type="checkbox" />
          <span>我已阅读并同意《用户协议》和《隐私政策》</span>
        </label>
        <button class="admin-auth-submit" type="submit" :disabled="submitting">{{ submitting ? '注册中...' : '注册并进入工作台' }}</button>
        <a class="admin-auth-link" :href="registerHref">已有账号，去登录</a>
      </form>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import logo from "../../assets/xianzhi-ai-logo.webp";

type RegisterForm = {
  username: string;
  email: string;
  password: string;
  confirmPassword: string;
  inviteCode: string;
};

const props = defineProps<{
  registerHref: string;
  registerForm: RegisterForm;
  registerAgreementAccepted: boolean;
  submitting: boolean;
}>();

const emit = defineEmits<{
  submit: [];
  "update:registerForm": [value: RegisterForm];
  "update:registerAgreementAccepted": [value: boolean];
}>();

const form = computed({
  get: () => props.registerForm,
  set: (value: RegisterForm) => emit("update:registerForm", value)
});

const username = computed({
  get: () => form.value.username,
  set: (value: string) => emit("update:registerForm", { ...form.value, username: value })
});
const email = computed({
  get: () => form.value.email,
  set: (value: string) => emit("update:registerForm", { ...form.value, email: value })
});
const password = computed({
  get: () => form.value.password,
  set: (value: string) => emit("update:registerForm", { ...form.value, password: value })
});
const confirmPassword = computed({
  get: () => form.value.confirmPassword,
  set: (value: string) => emit("update:registerForm", { ...form.value, confirmPassword: value })
});
const inviteCode = computed({
  get: () => form.value.inviteCode,
  set: (value: string) => emit("update:registerForm", { ...form.value, inviteCode: value })
});
const agreementAccepted = computed({
  get: () => props.registerAgreementAccepted,
  set: (value: boolean) => emit("update:registerAgreementAccepted", value)
});
</script>

<style scoped>
.admin-auth-shell{min-height:100vh;display:grid;place-items:center;padding:24px;background:radial-gradient(circle at top left,#efeaff 0,#f7f8fc 34%,#edf1ff 100%)}.admin-auth-card{width:min(520px,100%);box-sizing:border-box;padding:30px;border:1px solid #eaecf0;border-radius:28px;background:#fff;box-shadow:0 30px 90px rgba(16,24,40,.12)}.admin-auth-brand{display:flex;align-items:center;gap:14px}.admin-auth-brand img{width:56px;height:56px;object-fit:contain;border-radius:16px;background:#fff;box-shadow:0 8px 24px rgba(91,73,232,.14)}.admin-auth-brand strong{display:block;font-size:22px}.admin-auth-brand span{color:#667085;font-size:13px}.admin-auth-head{margin-top:24px}.admin-auth-head :deep(.el-tag){margin-bottom:14px}.admin-auth-head h1{margin:0 0 10px;color:#101828;font-size:32px;line-height:1.1}.admin-auth-head p{margin:0;color:#667085;line-height:1.7}.admin-auth-form{display:grid;gap:14px;margin-top:24px}.admin-auth-form label{display:grid;gap:7px;color:#344054;font-size:13px;font-weight:700}.admin-auth-form input{width:100%;height:46px;padding:0 14px;border:1px solid #d0d5dd;border-radius:12px;outline:none;box-sizing:border-box;font:inherit;background:#fff;color:#101828}.admin-auth-form input:focus{border-color:#6c5cf4;box-shadow:0 0 0 3px rgba(108,92,244,.14)}.admin-auth-check{display:flex;align-items:flex-start;gap:8px;padding:12px 14px;border-radius:12px;background:#f9fafb}.admin-auth-check input{width:16px;height:16px;margin-top:1px;flex:0 0 auto}.admin-auth-submit{height:46px;border:0;border-radius:12px;color:#fff;background:linear-gradient(135deg,#7464f2,#5b49e8);font-size:14px;font-weight:900;cursor:pointer}.admin-auth-submit:disabled{cursor:not-allowed;opacity:.62}.admin-auth-link{justify-self:center;color:#5b49e8;font-size:13px;font-weight:700;text-decoration:none}@media(max-width:560px){.admin-auth-shell{padding:16px}.admin-auth-card{padding:24px 18px;border-radius:22px}.admin-auth-head h1{font-size:26px}}
</style>
