<template>
  <view class="route-loading">正在进入工作台...</view>
</template>

<script setup lang="ts">
import { onLoad } from "@dcloudio/uni-app";
import { miniProgramDefaultPage } from "../config/miniProgramPages";

onLoad(() => {
  uni.reLaunch({ url: miniProgramDefaultPage });
});
</script>

<style>
page {
  background: #f7f8fc;
}

.route-loading {
  padding: 120px 20px 0;
  color: #697386;
  font-size: 13px;
  text-align: center;
}
</style>
