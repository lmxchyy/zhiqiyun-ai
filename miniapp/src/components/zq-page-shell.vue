<template>
  <wd-config-provider :theme-vars="wotThemeVars">
    <view class="zq-page">
      <scroll-view scroll-y class="zq-page__scroll">
        <slot />
      </scroll-view>
      <wd-toast />
    </view>
  </wd-config-provider>
</template>

<script setup lang="ts">
import { wotThemeVars } from '@/constants/theme'
</script>
