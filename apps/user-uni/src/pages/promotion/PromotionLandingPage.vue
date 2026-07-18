<template><view class="promotion-page promotion-landing"><image :src="loginLogo" mode="aspectFit" /><text>正在进入知启云AI</text><text>{{ message }}</text><view class="promotion-state-loader" /></view></template>
<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
import { getAuthToken } from "../../api/client";
import { capturePromotionReferral, syncPendingPromotionReferral } from "../../features/promotion/referral";
const message = ref("正在识别专属推广信息");
onLoad(async options => { const referral = capturePromotionReferral(options as Record<string, unknown>); message.value = referral ? "推广信息已安全保存" : "正在打开小程序"; if (getAuthToken()) await syncPendingPromotionReferral(); uni.switchTab({ url: "/pages/user/UserHomePage", fail: () => uni.reLaunch({ url: "/pages/user/UserHomePage" }) }); });
</script>
<style>@import "../../styles/promotion-center.css";</style>
