<template>
  <view class="auth-brand-header">
    <view class="auth-brand-logo-frame" role="img" aria-label="知启云AI">
      <image
        class="auth-brand-logo"
        src="/static/brand/zhiqiyun-ai-logo.jpg"
        mode="aspectFill"
      />
    </view>
    <text class="auth-slogan">让AI成为企业生产力</text>
    <text class="auth-subtitle">企业级AI创作与智能体平台</text>
  </view>
</template>

<style scoped>
.auth-brand-header { position: relative; z-index: 1; display: flex; flex-direction: column; align-items: center; }
.auth-brand-logo-frame {
  width: 242px;
  height: 70px;
  overflow: hidden;
  border: 1px solid rgba(74, 107, 255, 0.08);
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 8px 22px rgba(26, 36, 64, 0.06);
}
.auth-brand-logo { display: block; width: 100%; height: 100%; }
.auth-slogan { margin-top: 16px; color: #181c28; font-size: 22px; line-height: 30px; font-weight: 700; text-align: center; }
.auth-subtitle { margin-top: 5px; color: #697085; font-size: 13px; line-height: 22px; text-align: center; }
@media (max-width: 340px) {
  .auth-brand-logo-frame { width: 216px; height: 62px; border-radius: 14px; }
  .auth-slogan { margin-top: 12px; font-size: 20px; line-height: 28px; }
  .auth-subtitle { font-size: 12px; line-height: 20px; }
}
</style>
