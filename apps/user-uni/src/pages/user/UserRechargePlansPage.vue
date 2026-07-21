<template>
  <view class="recharge-redirect">
    <text>正在进入点数充值页...</text>
  </view>
</template>

<script setup lang="ts">
import { onLoad } from '@dcloudio/uni-app'
import { miniProgramFeaturePages } from '../../config/miniProgramPages'

onLoad(() => {
  uni.redirectTo({
    url: miniProgramFeaturePages.userVirtualPayment,
    fail: () => uni.reLaunch({ url: miniProgramFeaturePages.userVirtualPayment }),
  })
})
</script>

<style>
.recharge-redirect { padding: 96rpx 32rpx; color: #667085; text-align: center; }
</style>
