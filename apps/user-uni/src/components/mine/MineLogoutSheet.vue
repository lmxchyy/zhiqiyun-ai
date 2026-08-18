<template>
  <view v-if="visible" class="mine-modal-layer" @click="emit('close')">
    <view class="mine-bottom-sheet logout-sheet" @click.stop>
      <view class="sheet-handle"></view>
      <view class="logout-title-row"><text class="mine-menu-icon danger">退</text><view><text>退出当前账号？</text><text>退出后需要重新登录才能继续使用</text></view></view>
      <view class="logout-notice"><text>本机登录状态将被清除</text><text>创作记录与钱包数据不会删除</text></view>
      <view class="logout-actions"><button @click="emit('close')">取消</button><button class="danger-action" @click="emit('confirm')">退出登录</button></view>
    </view>
  </view>
</template>

<script setup lang="ts">
interface Props {
  visible: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'confirm'): void;
}>();
</script>
