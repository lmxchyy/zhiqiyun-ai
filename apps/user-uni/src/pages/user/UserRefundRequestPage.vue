<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle">
    <view class="mpb-safe" />
    <view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="returnToOrder">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">退款申请</text><text class="mpb-subtitle">提交原因并等待人工审核</text></view><text class="mpb-role">普通用户</text></view>
    <view class="mpb-stack">
      <view class="mpb-hero"><view class="mpb-hero-top"><view><text class="mpb-hero-label">退款状态</text><text class="mpb-hero-value">{{ submitted ? '审核中' : '可提交申请' }}</text></view><text class="mpb-hero-badge">{{ submitted ? '已受理' : '人工审核' }}</text></view><text class="mpb-hero-copy">提交后会生成真实退款申请记录；退款到账仍需运营审核和支付渠道处理。</text><view class="mpb-hero-metrics"><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ order ? formatCurrency(orderAmount(order)) : '-' }}</text><text class="mpb-hero-metric-label">原支付</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">人工</text><text class="mpb-hero-metric-label">审核方式</text></view><view class="mpb-hero-metric"><text class="mpb-hero-metric-value">{{ submitted ? '已提交' : '待提交' }}</text><text class="mpb-hero-metric-label">当前进度</text></view></view></view>
      <view class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">退款说明</text><text class="mpb-card-copy">不伪造成功</text></view><view class="mpb-row"><text class="mpb-row-icon green">单</text><view class="mpb-row-main"><text class="mpb-row-title">原订单</text><text class="mpb-row-meta">{{ order ? orderTitle(order) : id }}</text></view><text class="mpb-status success">{{ order ? statusText(orderStatus(order)) : '待核对' }}</text></view><view class="mpb-row"><text class="mpb-row-icon orange">退</text><view class="mpb-row-main"><text class="mpb-row-title">可退金额</text><text class="mpb-row-meta">最终以后台人工审核为准</text></view><text class="mpb-amount">{{ order ? formatCurrency(orderAmount(order)) : '-' }}</text></view></view>
      <view class="mpb-card"><text class="mpb-card-title">选择退款原因</text><view class="mpb-radio-list"><view v-for="item in reasons" :key="item" :class="['mpb-radio', { active: reason === item }]" @click="reason = item"><text>{{ item }}</text><text>{{ reason === item ? '✓' : '' }}</text></view></view></view>
      <view class="mpb-card"><text class="mpb-label">补充说明</text><textarea v-model="remark" class="mpb-textarea" maxlength="200" placeholder="请描述具体问题，最多 200 字" /><text class="mpb-row-meta">{{ remark.length }}/200</text></view>
      <view class="mpb-note">申请提交后，订单状态会变为“退款审核中”；只有后台完成审核和支付渠道退款后，才代表退款成功。</view>
      <button class="mpb-button orange" :disabled="!order || submitting || submitted" @click="submit">{{ submitted ? '退款申请已提交' : submitting ? '正在提交...' : '提交退款申请' }}</button>
      <button class="mpb-button secondary" @click="returnToOrder">返回订单详情</button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import { api } from "../../api/client";
import { backOrHome, formatCurrency, loadUserOrders, orderAmount, orderId, orderStatus, orderTitle, statusText, type AnyRecord } from "../../utils/miniProgramBusiness";
import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id = ref(""); const reason = ref("重复购买"); const remark = ref(""); const order = ref<AnyRecord | null>(null); const submitting = ref(false); const submitted = ref(false);
const reasons = ["重复购买", "购买错误", "权益与描述不符", "服务未交付", "其他原因"];
async function submit() { if (!order.value || submitting.value || submitted.value) return; submitting.value = true; try { const result = await api<{ item?: AnyRecord; message?: string }>("/api/v1/member/refund-requests", { method: "POST", body: JSON.stringify({ orderId: id.value, reason: reason.value, remark: remark.value.trim() }) }); order.value = result.item || order.value; submitted.value = true; uni.showToast({ title: result.message || "退款申请已提交", icon: "success" }); } catch (error) { uni.showToast({ title: error instanceof Error ? error.message : "退款申请提交失败", icon: "none" }); } finally { submitting.value = false; } }
function returnToOrder() { if (id.value) backOrHome(`/pages/user/UserOrderDetailPage?id=${encodeURIComponent(id.value)}`); else backOrHome("/pages/user/UserOrdersPage"); }
onLoad(async options => { id.value = String(options?.id || ""); try { order.value = (await loadUserOrders()).find(item => orderId(item) === id.value) || null; submitted.value = orderStatus(order.value) === "REFUND_REQUESTED"; } catch (error) { order.value = null; uni.showToast({ title: error instanceof Error ? error.message : "退款订单加载失败", icon: "none" }); } });
</script>

<style>@import "../../styles/mini-program-business.css";</style>
