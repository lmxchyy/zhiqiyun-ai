<template><view class="ppt-route-loading">正在打开 PPT 助手...</view></template>
<script setup lang="ts">
import { onMounted } from "vue";
onMounted(() => { uni.redirectTo({ url: "/packagePpt/pages/index" }); });
</script>
<style scoped>.ppt-route-loading{padding:160rpx 32rpx;text-align:center;color:#6b7387;font-size:28rpx}</style>
