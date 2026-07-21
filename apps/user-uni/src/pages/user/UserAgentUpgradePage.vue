<template>
  <view class="agent-redirect"><text>正在进入代理商开通页...</text></view>
</template>

<script setup lang="ts">
import { onLoad } from '@dcloudio/uni-app'
import { miniProgramFeaturePages } from '../../config/miniProgramPages'

onLoad(() => {
  uni.redirectTo({ url: miniProgramFeaturePages.userAgentDetail })
})
</script>

<style>
.agent-redirect { padding: 96rpx 32rpx; color: #667085; text-align: center; }
</style>
