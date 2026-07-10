<template>
  <view class="mpb-page">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="returnToOrder">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">退款申请</text><text class="mpb-subtitle">展示可退范围与服务边界</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">退款状态</text><text class="mpb-hero-value">暂不可提交</text></view><text class="mpb-hero-badge">接口未开放</text></view><text class="mpb-hero-copy">当前 Go 后端尚未开放退款接口，本页展示规则并明确阻断提交。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ order ? formatCurrency(orderAmount(order)) : '-' }}</text><text class="mpb-hero-metric-label">原支付</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">人工</text><text class="mpb-hero-metric-label">审核方式</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">受限</text><text class="mpb-hero-metric-label">当前能力</text></view></view></view>
      <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">退款说明</text><text class="mpb-card-copy">不伪造成功</text></view><view class="mpb-row"><text class="mpb-row-icon green">单</text><view class="mpb-row-main"><text class="mpb-row-title">原订单</text><text class="mpb-row-meta">{{ order ? orderTitle(order) : id }}</text></view><text class="mpb-status success">{{ order ? statusText(orderStatus(order)) : '待核对' }}</text></view><view class="mpb-row"><text class="mpb-row-icon orange">退</text><view class="mpb-row-main"><text class="mpb-row-title">可退金额</text><text class="mpb-row-meta">最终以后台人工审核为准</text></view><text class="mpb-amount">{{ order ? formatCurrency(orderAmount(order)) : '-' }}</text></view></view>
      <view class="mpb-card"><text class="mpb-card-title">选择退款原因</text><view class="mpb-radio-list"><view v-for="item in reasons" :key="item" :class="['mpb-radio', { active: reason === item }]" @click="reason = item"><text>{{ item }}</text><text>{{ reason === item ? '✓' : '' }}</text></view></view></view>
      <view class="mpb-card"><text class="mpb-label">补充说明</text><textarea v-model="remark" class="mpb-textarea" maxlength="200" placeholder="请描述具体问题，最多 200 字" /><text class="mpb-row-meta">{{ remark.length }}/200</text></view>
      <view class="mpb-note mpb-danger-note">当前 Go 后端尚未提供用户退款提交接口。本页已完成信息收集和校验，但不会伪造退款成功；正式开放前需要接入支付渠道退款能力和审核流程。</view>
      <button class="mpb-button orange" @click="returnToOrder">返回订单详情</button>
      <button class="mpb-button secondary" @click="showLimit">查看退款处理说明</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { backOrHome, formatCurrency, loadUserOrders, orderAmount, orderId, orderStatus, orderTitle, statusText, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const reason = ref("重复购买"); const remark = ref(""); const order = ref<AnyRecord | null>(null);
const reasons = ["重复购买", "购买错误", "权益与描述不符", "服务未交付", "其他原因"];
function showLimit() { uni.showModal({ title: "暂未开放线上退款", content: "退款接口和支付渠道审核尚未接入，请联系平台客服并提供订单编号。当前页面不会生成退款成功记录。", showCancel: false }); }
function returnToOrder() { if (id.value) backOrHome(`/pages/user/UserOrderDetailPage?id=${encodeURIComponent(id.value)}`); else backOrHome("/pages/user/UserOrdersPage"); }
onLoad(async options => { id.value = String(options?.id || ""); try { order.value = (await loadUserOrders()).find(item => orderId(item) === id.value) || null; } catch { order.value = null; } });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
