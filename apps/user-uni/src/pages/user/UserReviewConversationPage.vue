<template>
  <view class="mpb-page" :style="miniProgramNavigationStyle"><view class="mpb-safe" /><view class="mpb-header"><button class="mpb-back" aria-label="返回" @click="backOrHome('/pages/user/UserReviewCreationPage')">‹</button><image class="mpb-logo" :src="loginLogo" mode="aspectFit" /><view class="mpb-header-copy"><text class="mpb-title">共识记录</text><text class="mpb-subtitle">问题、回答与引用结果</text></view><text class="mpb-role">普通用户</text></view><view class="mpb-stack"><view v-if="loading" class="mpb-card mpb-empty"><text class="mpb-empty-title">正在加载消息...</text></view><view v-else-if="messages.length" class="mpb-card mpb-list"><view class="mpb-section-head"><text class="mpb-card-title">验证过程</text><text class="mpb-card-copy">{{ messages.length }} 条消息</text></view><view v-for="item in messages" :key="rowString(item, 'id')" class="message-row"><text :class="['message-role', rowString(item, 'role').toLowerCase() === 'user' ? 'user' : 'assistant']">{{ rowString(item, 'role').toLowerCase() === 'user' ? '问题' : '结论' }}</text><text class="message-content">{{ rowString(item, 'content', 'text', 'answer') || '消息内容为空' }}</text><text class="message-time">{{ formatDate(rowString(item, 'createdAt')) }}</text></view></view><view v-else class="mpb-card mpb-empty"><text class="mpb-empty-title">暂无消息</text><text class="mpb-empty-copy">验证任务可能仍在处理中，请稍后刷新。</text></view><button class="mpb-button secondary" @click="load">刷新结果</button></view></view>
</template>
<script setup lang="ts">
import { ref } from "vue"; import { onLoad } from "@dcloudio/uni-app"; import { api } from "../../api/client"; import { backOrHome, formatDate, rowItems, rowString, type AnyRecord } from "../../utils/miniProgramBusiness"; import loginLogo from "../../assets/zhiqiyun-logo-transparent.png";
const id=ref("");const loading=ref(false);const messages=ref<AnyRecord[]>([]);async function load(){loading.value=true;try{messages.value=rowItems(await api(`/api/v1/knowledge-conversations/${encodeURIComponent(id.value)}/messages`));}catch(error){uni.showToast({title:error instanceof Error?error.message:"共识消息加载失败",icon:"none"});}finally{loading.value=false;}}onLoad(options=>{id.value=String(options?.id||"");void load();});
</script>
<style>
@import "../../styles/mini-program-business.css";
.message-row { display: flex; padding: 12px; flex-direction: column; gap: 7px; border: 1px solid #e5eaf6; border-radius: 8px; background: #fff; }
.message-role { align-self: flex-start; padding: 3px 7px; border-radius: 999px; font-size: 10px; font-weight: 600; }
.message-role.user { color: #5b55d6; background: #f4f3ff; }
.message-role.assistant { color: #087443; background: #ecfdf5; }
.message-content { color: #111827; font-size: 12px; line-height: 19px; white-space: pre-wrap; }
.message-time { color: #98a2b3; font-size: 10px; }
</style>
