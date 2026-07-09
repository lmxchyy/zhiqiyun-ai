<template>
  <zq-page-shell>
    <view class="home-hero">
      <view>
        <text class="home-hero__brand">知启云 AI</text>
        <text class="home-hero__slogan">让 AI 成为企业生产力</text>
      </view>
      <view class="home-hero__badge">MVP</view>
    </view>

    <zq-balance-card
      :points-text="formatNumber(user.points)"
      :member-level="user.memberLevel"
      @recharge="goProfile"
      @membership="goProfile"
    />

    <view class="quick-create zq-card">
      <text class="quick-create__title">快速创作</text>
      <wd-textarea
        v-model="quickPrompt"
        :maxlength="160"
        clearable
        show-word-limit
        placeholder="一句话描述你想生成的图片、视频、PPT 或 Agent 任务"
      />
      <view class="quick-create__actions">
        <wd-button plain custom-class="quick-create__upload" @click="showMockToast('参考图上传入口已预留')">
          上传参考图
        </wd-button>
        <wd-button type="primary" custom-class="quick-create__submit" @click="goCreate">
          去创作
        </wd-button>
      </view>
    </view>

    <view class="zq-section">
      <view class="zq-section__head">
        <text class="zq-section__title">核心能力</text>
        <text class="zq-section__more" @click="goCreate">全部创作</text>
      </view>
      <view class="feature-grid">
        <zq-feature-card
          v-for="feature in features"
          :key="feature.id"
          :title="feature.title"
          :subtitle="feature.subtitle"
          :icon="feature.icon"
          :tone="feature.tone"
          @tap="openFeature(feature.path)"
        />
      </view>
    </view>

    <view class="template-strip zq-card">
      <view>
        <text class="template-strip__title">模板广场</text>
        <text class="template-strip__copy">电商主图、活动海报、企业介绍、朋友圈营销模板</text>
      </view>
      <wd-button plain size="small" @click="showMockToast('模板广场页面已预留')">查看</wd-button>
    </view>

    <view class="zq-section">
      <view class="zq-section__head">
        <text class="zq-section__title">最近作品</text>
        <text class="zq-section__more" @click="goWorks">查看全部</text>
      </view>
      <view class="work-list">
        <zq-work-card v-for="work in works.slice(0, 3)" :key="work.id" :item="work" />
      </view>
    </view>

    <view class="home-links">
      <view class="home-link-card zq-card" @click="goProfile">
        <text>会员套餐</text>
        <text>权益、积分与充值</text>
      </view>
      <view class="home-link-card zq-card" @click="showMockToast('代理商入口已预留到个人中心')">
        <text>代理商入口</text>
        <text>邀请客户与收益查看</text>
      </view>
    </view>
  </zq-page-shell>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ZqBalanceCard from '@/components/zq-balance-card.vue'
import ZqFeatureCard from '@/components/zq-feature-card.vue'
import ZqPageShell from '@/components/zq-page-shell.vue'
import ZqWorkCard from '@/components/zq-work-card.vue'
import { tabRoutes } from '@/constants/routes'
import { getHomeOverview } from '@/services/miniapp'
import type { FeatureEntry, UserProfile, WorkItem } from '@/types/domain'
import { formatNumber } from '@/utils/format'
import { showMockToast, switchTab } from '@/utils/navigation'

const quickPrompt = ref('')
const user = ref<UserProfile>({
  id: '',
  name: '',
  avatarText: '',
  memberLevel: '',
  points: 0,
  agentEnabled: false,
})
const features = ref<FeatureEntry[]>([])
const works = ref<WorkItem[]>([])

function goCreate() {
  switchTab(tabRoutes.create)
}

function goWorks() {
  switchTab(tabRoutes.works)
}

function goProfile() {
  switchTab(tabRoutes.profile)
}

function openFeature(path?: string) {
  if (!path) {
    showMockToast()
    return
  }
  switchTab(path)
}

onMounted(async () => {
  const data = await getHomeOverview()
  user.value = data.user
  features.value = data.features
  works.value = data.works
})
</script>

<style scoped lang="scss">
.home-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24rpx;
  padding: 42rpx 4rpx 26rpx;
}

.home-hero__brand,
.home-hero__slogan {
  display: block;
}

.home-hero__brand {
  color: var(--color-text-primary);
  font-size: 48rpx;
  font-weight: 900;
  letter-spacing: 0;
}

.home-hero__slogan {
  margin-top: 12rpx;
  color: var(--color-text-secondary);
  font-size: 26rpx;
}

.home-hero__badge {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 88rpx;
  height: 52rpx;
  border-radius: 999rpx;
  background: rgba(255, 119, 27, 0.12);
  color: var(--color-accent);
  font-size: 22rpx;
  font-weight: 900;
}

.quick-create {
  margin-top: 24rpx;
  padding: 26rpx;
}

.quick-create__title {
  display: block;
  margin-bottom: 16rpx;
  color: var(--color-text-primary);
  font-size: 30rpx;
  font-weight: 800;
}

.quick-create__actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18rpx;
  margin-top: 18rpx;
}

:deep(.quick-create__submit) {
  background: var(--color-accent) !important;
  border-color: var(--color-accent) !important;
}

:deep(.quick-create__upload) {
  color: var(--color-primary-dark) !important;
  border-color: rgba(125, 141, 246, 0.35) !important;
}

.feature-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18rpx;
}

.template-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  margin-top: 30rpx;
  padding: 26rpx;
}

.template-strip__title,
.template-strip__copy {
  display: block;
}

.template-strip__title {
  color: var(--color-text-primary);
  font-size: 30rpx;
  font-weight: 800;
}

.template-strip__copy {
  margin-top: 8rpx;
  color: var(--color-text-secondary);
  font-size: 23rpx;
  line-height: 1.45;
}

.work-list {
  display: grid;
  gap: 18rpx;
}

.home-links {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18rpx;
  margin-top: 30rpx;
}

.home-link-card {
  padding: 24rpx;
}

.home-link-card text {
  display: block;
}

.home-link-card text:first-child {
  color: var(--color-text-primary);
  font-size: 28rpx;
  font-weight: 800;
}

.home-link-card text:last-child {
  margin-top: 10rpx;
  color: var(--color-text-secondary);
  font-size: 22rpx;
  line-height: 1.45;
}
</style>
