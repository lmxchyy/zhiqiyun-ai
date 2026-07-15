<template>
  <view class="forbidden-page">
    <view class="forbidden-card">
      <text class="forbidden-code">403</text>
      <text class="forbidden-title">暂无访问权限</text>
      <text class="forbidden-copy">当前角色不能访问该页面，请返回“我的”切换到已授权角色。</text>
      <text v-if="permission" class="forbidden-permission">需要权限：{{ permission }}</text>
      <button type="button" class="forbidden-primary" @click="goMine">返回我的</button>
      <button type="button" class="forbidden-secondary" @click="goHome">返回首页</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";

const permission = ref("");

onLoad(options => {
  permission.value = decodeURIComponent(String(options?.permission || ""));
});

function goMine() {
  uni.switchTab({ url: "/pages/user/UserMinePage" });
}

function goHome() {
  uni.switchTab({ url: "/pages/user/UserHomePage" });
}
</script>

<style scoped>
.forbidden-page { min-height: 100vh; padding: 96px 24px 40px; box-sizing: border-box; background: #f7f8fc; }
.forbidden-card { padding: 34px 24px; border: 1px solid #e8eaf2; border-radius: 22px; background: #fff; box-shadow: 0 18px 45px rgba(23, 28, 56, .08); text-align: center; }
.forbidden-code, .forbidden-title, .forbidden-copy, .forbidden-permission { display: block; }
.forbidden-code { color: #7d8df6; font-size: 54px; font-weight: 700; line-height: 1; }
.forbidden-title { margin-top: 16px; color: #171c29; font-size: 22px; font-weight: 700; }
.forbidden-copy { margin-top: 10px; color: #697386; font-size: 13px; line-height: 21px; }
.forbidden-permission { margin-top: 14px; color: #5a4db2; font-size: 12px; }
.forbidden-primary, .forbidden-secondary { width: 100%; height: 46px; margin: 22px 0 0; border-radius: 14px; font-size: 14px; font-weight: 600; }
.forbidden-primary { color: #fff; background: #7d8df6; }
.forbidden-secondary { margin-top: 10px; color: #5a4db2; background: #eef0ff; }
.forbidden-primary::after, .forbidden-secondary::after { display: none; }
</style>
