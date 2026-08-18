<template>
  <view v-if="purchase" class="mine-modal-layer" @click="emit('close')">
    <view class="mine-bottom-sheet" @click.stop>
      <view class="sheet-handle"></view>
      <view class="sheet-title-row">
        <view>
          <text>{{ purchase.kind === 'agent' ? '开通代理套餐' : `充值 ${purchase.amountCents / 100} 元` }}</text>
          <text>{{ purchase.kind === 'agent' ? '获得代理角色、推广工具与分润资格' : `支付成功后即时到账 ${formatNumber(purchase.points)} 点` }}</text>
        </view>
        <text v-if="purchase.recommended" class="recommend-tag">推荐</text>
      </view>
      <view :class="['purchase-summary', { agent: purchase.kind === 'agent' }]">
        <text>{{ purchase.kind === 'agent' ? '代理套餐' : '到账点数' }}</text>
        <text>{{ purchase.kind === 'agent' ? '年度代理权益' : `${formatNumber(purchase.points)} 点` }}</text>
        <text>{{ formatCurrency(purchase.amountCents) }}</text>
      </view>
      <view class="payment-method">
        <view><text>微信支付</text><text>推荐使用当前微信账户完成支付</text></view>
        <text>✓</text>
      </view>
      <view class="purchase-note">
        <text>{{ purchase.kind === 'agent' ? '开通即表示同意' : '充值即表示同意' }}</text>
        <text class="purchase-note-link" @click="emit('open-agreement')">《{{ purchase.kind === 'agent' ? '代理商服务协议' : '点数充值服务协议' }}》</text>
      </view>
      <button :class="['sheet-primary', { agent: purchase.kind === 'agent' }]" :disabled="submitting" @click="emit('confirm')">{{ submitting ? '正在创建订单...' : purchase.kind === 'agent' ? `支付 ${formatCurrency(purchase.amountCents)} 并开通` : `确认支付 ${formatCurrency(purchase.amountCents)}` }}</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import type { MinePurchaseOption } from "../../types";

interface Props {
  purchase: MinePurchaseOption | null;
  submitting: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'confirm'): void;
  (e: 'open-agreement'): void;
}>();

function formatNumber(value: number) {
  return String(Math.max(0, Math.round(value || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

function formatCurrency(cents: number) {
  return `¥${(Math.max(0, cents || 0) / 100).toFixed(2)}`;
}
</script>
