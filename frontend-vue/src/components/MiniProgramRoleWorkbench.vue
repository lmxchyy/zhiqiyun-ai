<template>
  <view class="mini-workbench">
    <view class="status-bar-spacer"></view>

    <view class="hero-panel">
      <view class="brand-row">
        <image class="brand-logo" :src="loginLogo" mode="aspectFit" />
        <view class="brand-copy">
          <text class="brand-eyebrow">{{ roleLabel }}</text>
          <text class="brand-title">{{ greetingText }}</text>
        </view>
        <button class="icon-button" type="button" @click="refreshAll">
          <text>刷</text>
        </button>
      </view>

      <view class="role-switcher" v-if="availableRoles.length > 1">
        <button
          v-for="role in availableRoles"
          :key="role.id"
          type="button"
          :class="['role-pill', { active: activeRole === role.id }]"
          @click="switchRole(role.id)"
        >
          <text>{{ role.label }}</text>
        </button>
      </view>

      <view class="hero-metrics">
        <view class="metric-card primary">
          <text class="metric-label">{{ activeRole === "user" ? "可用点数" : "可提现收益" }}</text>
          <text class="metric-value">{{ activeRole === "user" ? formatNumber(pointBalance) : formatCurrency(roleWithdrawable) }}</text>
        </view>
        <view class="metric-card">
          <text class="metric-label">{{ secondaryMetricLabel }}</text>
          <text class="metric-value">{{ secondaryMetricValue }}</text>
        </view>
      </view>
    </view>

    <view v-if="pageLoading" class="state-card">
      <text>正在同步小程序工作台...</text>
    </view>
    <view v-else-if="pageError" class="state-card error">
      <text>{{ pageError }}</text>
      <button type="button" class="small-button" @click="refreshAll">重新加载</button>
    </view>

    <view v-else class="role-content">
      <template v-if="activeRole === 'user'">
        <view v-if="activeTab === 'home'" class="section-stack">
          <view class="section-card">
            <view class="section-header">
              <view>
                <text class="section-kicker">用户首页</text>
                <text class="section-title">创作资产与账户状态</text>
              </view>
              <text class="soft-tag">{{ planName }}</text>
            </view>
            <view class="quick-grid">
              <view class="quick-item">
                <text class="quick-value">{{ formatNumber(pointBalance) }}</text>
                <text class="quick-label">剩余点数</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ formatNumber(monthlyPointCost) }}</text>
                <text class="quick-label">本月消耗</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ recentAssets.length }}</text>
                <text class="quick-label">近期作品</text>
              </view>
            </view>
          </view>

          <view class="upgrade-strip">
            <view>
              <text class="strip-title">升级代理商</text>
              <text class="strip-copy">充值 996 元开通代理商，获得推广码、客户管理和分润入口。</text>
            </view>
            <button v-if="!isAgentActive" type="button" class="strip-button" @click="createAgentOrder">立即升级</button>
            <button v-else type="button" class="strip-button" @click="switchRole('agent')">进入代理</button>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">最近订单</text>
              <button type="button" class="text-button" @click="selectUserTab('wallet')">查看钱包</button>
            </view>
            <view v-if="userOrders.length" class="list-stack">
              <view v-for="order in userOrders.slice(0, 3)" :key="rowKey(order)" class="list-item">
                <view>
                  <text class="list-title">{{ orderTitle(order) }}</text>
                  <text class="list-meta">{{ formatDate(rowDate(order)) }}</text>
                </view>
                <view class="list-side">
                  <text class="price-text">{{ formatCurrency(rowAmount(order)) }}</text>
                  <text :class="['status-tag', statusTone(rowStatus(order))]">{{ rowStatus(order) }}</text>
                </view>
              </view>
            </view>
            <text v-else class="empty-text">暂无订单，完成套餐或点数充值后会在这里展示。</text>
          </view>
        </view>

        <view v-else-if="activeTab === 'create'" class="section-stack">
          <view class="section-card">
            <view class="section-header">
              <view>
                <text class="section-kicker">AI 创作</text>
                <text class="section-title">选择能力并填写提示词</text>
              </view>
              <text class="soft-tag">余额 {{ formatNumber(pointBalance) }}</text>
            </view>
            <view class="creation-grid">
              <button
                v-for="module in creationModules"
                :key="module.id"
                type="button"
                :class="['creation-card', { active: creationMode === module.id }]"
                @click="creationMode = module.id"
              >
                <text class="creation-icon">{{ module.icon }}</text>
                <text class="creation-name">{{ module.name }}</text>
                <text class="creation-cost">{{ module.cost }}</text>
              </button>
            </view>
            <textarea
              v-model="creationPrompt"
              class="prompt-input"
              maxlength="500"
              placeholder="请输入创作需求，例如：生成一张科技感产品海报，蓝紫配色，突出云端 AI 能力"
            />
            <button type="button" class="primary-button" @click="showComingSoon(activeCreationName)">开始生成</button>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">生成配置</text>
              <text class="soft-tag">小程序轻量版</text>
            </view>
            <view class="config-row">
              <text>模型</text>
              <text>{{ activeCreationModel }}</text>
            </view>
            <view class="config-row">
              <text>比例</text>
              <text>{{ creationMode === "video" ? "9:16 / 16:9" : "1:1 / 4:3" }}</text>
            </view>
            <view class="config-row">
              <text>扣点预估</text>
              <text>{{ activeCreationCost }}</text>
            </view>
          </view>
        </view>

        <view v-else-if="activeTab === 'assets'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">作品中心</text>
              <button type="button" class="text-button" @click="() => loadAssets()">刷新</button>
            </view>
            <text v-if="assetsLoading" class="empty-text">正在加载作品...</text>
            <text v-else-if="assetsError" class="empty-text">{{ assetsError }}</text>
            <view v-else-if="recentAssets.length" class="work-list">
              <view v-for="asset in recentAssets" :key="asset.id" class="work-item">
                <image v-if="asset.mediaType === 'image' && asset.thumbnailUrl" class="work-thumb" :src="asset.thumbnailUrl" mode="aspectFill" />
                <view v-else class="work-thumb fallback">
                  <text>{{ asset.mediaType === "video" ? "视" : asset.mediaType === "document" ? "文" : "图" }}</text>
                </view>
                <view class="work-main">
                  <text class="work-title">{{ asset.name || asset.id }}</text>
                  <text class="work-meta">{{ asset.mediaType }} · {{ formatDate(asset.createdAt) }}</text>
                </view>
                <text class="status-tag success">可用</text>
              </view>
            </view>
            <text v-else class="empty-text">暂无作品，可先在创作页生成图片、视频或 PPT。</text>
          </view>
        </view>

        <view v-else-if="activeTab === 'wallet'" class="section-stack">
          <view class="wallet-card">
            <text class="wallet-label">钱包余额</text>
            <text class="wallet-value">{{ formatNumber(pointBalance) }}</text>
            <text class="wallet-copy">冻结 {{ formatNumber(pointFrozen) }} 点 · 订单 {{ userOrders.length }} 笔</text>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">点数充值</text>
              <text class="soft-tag">微信支付</text>
            </view>
            <view class="recharge-grid">
              <button
                v-for="pack in rechargePackages"
                :key="pack.id"
                type="button"
                class="recharge-card"
                @click="createRechargeOrder(pack)"
              >
                <text class="recharge-points">{{ formatNumber(pack.points) }} 点</text>
                <text class="recharge-price">{{ formatCurrency(pack.amountCents) }}</text>
              </button>
            </view>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">积分消耗</text>
              <text class="soft-tag">{{ tokenRecords.length || userTransactions.length }} 条</text>
            </view>
            <view v-if="walletRecords.length" class="list-stack">
              <view v-for="record in walletRecords.slice(0, 6)" :key="rowKey(record)" class="list-item">
                <view>
                  <text class="list-title">{{ usageTitle(record) }}</text>
                  <text class="list-meta">{{ formatDate(rowDate(record)) }}</text>
                </view>
                <text class="cost-text">-{{ formatNumber(rowPointCost(record)) }}</text>
              </view>
            </view>
            <text v-else class="empty-text">暂无消耗记录。</text>
          </view>
        </view>

        <view v-else class="section-stack">
          <view class="profile-card">
            <image class="profile-logo" :src="loginLogo" mode="aspectFit" />
            <view>
              <text class="profile-name">{{ displayName }}</text>
              <text class="profile-meta">{{ userEmail }}</text>
            </view>
          </view>

          <view class="section-card">
            <view class="section-header">
              <view>
                <text class="section-kicker">身份升级</text>
                <text class="section-title">996 代理商开通包</text>
              </view>
              <text class="price-badge">996 元</text>
            </view>
            <text class="body-copy">开通后生成代理商邀请码，可查看推广客户、代理商分润、推广链接与拓展客户数据。</text>
            <button v-if="!isAgentActive" type="button" class="primary-button" @click="createAgentOrder">充值 996 升级代理商</button>
            <button v-else type="button" class="primary-button" @click="switchRole('agent')">进入代理商工作台</button>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">996 AI 创作会员包</text>
              <text class="price-badge">400 元 Token</text>
            </view>
            <text class="body-copy">与桌面端一致，开通 Pro 会员权益并获得 AI 点数。</text>
            <button type="button" class="outline-button" @click="createMemberPackageOrder">开通会员包</button>
          </view>

          <view class="menu-list">
            <view class="menu-row">
              <text>订单记录</text>
              <text>{{ userOrders.length }} 笔</text>
            </view>
            <view class="menu-row">
              <text>积分消耗</text>
              <text>{{ walletRecords.length }} 条</text>
            </view>
            <view class="menu-row danger" @click="logout">
              <text>退出登录</text>
              <text>重新登录</text>
            </view>
          </view>
        </view>
      </template>

      <template v-else-if="activeRole === 'agent'">
        <view v-if="activeTab === 'overview'" class="section-stack">
          <view class="section-card">
            <view class="section-header">
              <view>
                <text class="section-kicker">代理商概览</text>
                <text class="section-title">{{ agentName }}</text>
              </view>
              <text class="soft-tag">{{ agentLevelLabel }}</text>
            </view>
            <view class="quick-grid">
              <view class="quick-item">
                <text class="quick-value">{{ formatNumber(summaryNumber(channelSummary, "directCustomers")) }}</text>
                <text class="quick-label">拓展客户</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ formatNumber(summaryNumber(channelSummary, "childAgents")) }}</text>
                <text class="quick-label">下级代理</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ formatCurrency(summaryNumber(channelSummary, "totalCommission")) }}</text>
                <text class="quick-label">累计分润</text>
              </view>
            </view>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">待处理</text>
              <button type="button" class="text-button" @click="selectAgentTab('commission')">查看分润</button>
            </view>
            <view class="config-row">
              <text>待结算佣金</text>
              <text>{{ formatCurrency(summaryNumber(channelSummary, "pendingCommission")) }}</text>
            </view>
            <view class="config-row">
              <text>可提现金额</text>
              <text>{{ formatCurrency(summaryNumber(channelSummary, "availableToWithdraw")) }}</text>
            </view>
            <view class="config-row">
              <text>提现审核中</text>
              <text>{{ formatCurrency(summaryNumber(channelSummary, "pendingWithdrawal")) }}</text>
            </view>
          </view>
        </view>

        <view v-else-if="activeTab === 'promotion'" class="section-stack">
          <view class="promo-card">
            <view class="qr-box">
              <view class="qr-grid">
                <view v-for="cell in 25" :key="cell" :class="['qr-cell', { dark: cell % 2 === 0 || cell % 7 === 0 }]"></view>
              </view>
            </view>
            <text class="section-title">推广小程序码</text>
            <text class="body-copy">微信内优先分享小程序码；H5 推广链接用于朋友圈海报、社群文案和客服转发。</text>
            <view class="invite-code">
              <text>{{ inviteCode }}</text>
            </view>
            <button type="button" class="primary-button" open-type="share">微信分享</button>
            <button type="button" class="outline-button" @click="copyInviteLink">复制推广链接</button>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">推广路径</text>
              <text class="soft-tag">自动带 invite</text>
            </view>
            <view class="config-row">
              <text>小程序路径</text>
              <text>{{ sharePath }}</text>
            </view>
            <view class="config-row">
              <text>H5 链接</text>
              <text>{{ inviteLink }}</text>
            </view>
          </view>
        </view>

        <view v-else-if="activeTab === 'customers'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">拓展客户</text>
              <text class="soft-tag">{{ channelCustomers.length }} 人</text>
            </view>
            <view v-if="channelCustomers.length" class="list-stack">
              <view v-for="customer in channelCustomers" :key="rowKey(customer)" class="list-item">
                <view>
                  <text class="list-title">{{ customerName(customer) }}</text>
                  <text class="list-meta">{{ customerEmail(customer) }}</text>
                </view>
                <view class="list-side">
                  <text class="price-text">{{ formatNumber(rowNumber(customer, "pointsAvailable")) }} 点</text>
                  <text class="status-tag success">{{ rowString(customer, "plan") || "客户" }}</text>
                </view>
              </view>
            </view>
            <text v-else class="empty-text">暂无客户，先分享小程序码或推广链接。</text>
          </view>
        </view>

        <view v-else-if="activeTab === 'commission'" class="section-stack">
          <view class="wallet-card agent">
            <text class="wallet-label">可提现收益</text>
            <text class="wallet-value">{{ formatCurrency(summaryNumber(channelSummary, "availableToWithdraw")) }}</text>
            <text class="wallet-copy">累计 {{ formatCurrency(summaryNumber(channelSummary, "totalCommission")) }} · 已提现 {{ formatCurrency(summaryNumber(channelSummary, "withdrawn")) }}</text>
            <button type="button" class="wallet-button" @click="requestWithdrawal">申请提现</button>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">分润明细</text>
              <text class="soft-tag">{{ channelCommissions.length }} 条</text>
            </view>
            <view v-if="channelCommissions.length" class="list-stack">
              <view v-for="commission in channelCommissions.slice(0, 8)" :key="rowKey(commission)" class="list-item">
                <view>
                  <text class="list-title">订单 {{ rowString(commission, "orderId") || rowString(commission, "id") }}</text>
                  <text class="list-meta">{{ formatDate(rowDate(commission)) }}</text>
                </view>
                <view class="list-side">
                  <text class="price-text">{{ formatCurrency(rowAmount(commission)) }}</text>
                  <text :class="['status-tag', statusTone(rowStatus(commission))]">{{ rowStatus(commission) }}</text>
                </view>
              </view>
            </view>
            <text v-else class="empty-text">暂无分润记录。</text>
          </view>
        </view>

        <view v-else class="section-stack">
          <view class="profile-card">
            <image class="profile-logo" :src="loginLogo" mode="aspectFit" />
            <view>
              <text class="profile-name">{{ agentName }}</text>
              <text class="profile-meta">{{ agentLevelLabel }} · {{ agentStatus }}</text>
            </view>
          </view>
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">代理权益</text>
              <text class="soft-tag">已开通</text>
            </view>
            <view class="config-row">
              <text>邀请码</text>
              <text>{{ inviteCode }}</text>
            </view>
            <view class="config-row">
              <text>开通条件</text>
              <text>{{ agentCondition("openCondition") }}</text>
            </view>
            <view class="config-row">
              <text>保级条件</text>
              <text>{{ agentCondition("keepCondition") }}</text>
            </view>
            <button type="button" class="outline-button" @click="showChildAgentHint">拓展下级代理</button>
          </view>
        </view>
      </template>

      <template v-else>
        <view v-if="activeTab === 'overview'" class="section-stack">
          <view class="section-card">
            <view class="section-header">
              <view>
                <text class="section-kicker">运营中心</text>
                <text class="section-title">{{ operationName }}</text>
              </view>
              <text class="soft-tag">{{ operationStatus }}</text>
            </view>
            <view class="quick-grid">
              <view class="quick-item">
                <text class="quick-value">{{ operationAgents.length }}</text>
                <text class="quick-label">代理商</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ operationOrders.length }}</text>
                <text class="quick-label">订单</text>
              </view>
              <view class="quick-item">
                <text class="quick-value">{{ formatCurrency(operationCommissionTotal) }}</text>
                <text class="quick-label">中心分润</text>
              </view>
            </view>
          </view>
        </view>

        <view v-else-if="activeTab === 'agents'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">代理商团队</text>
              <text class="soft-tag">{{ operationAgents.length }} 人</text>
            </view>
            <view v-if="operationAgents.length" class="list-stack">
              <view v-for="agent in operationAgents" :key="rowKey(agent)" class="list-item">
                <view>
                  <text class="list-title">{{ customerName(agent) }}</text>
                  <text class="list-meta">{{ rowString(agent, "levelLabel") || rowString(agent, "levelName") || "代理商" }}</text>
                </view>
                <text :class="['status-tag', statusTone(rowStatus(agent))]">{{ rowStatus(agent) }}</text>
              </view>
            </view>
            <text v-else class="empty-text">暂无代理商数据。</text>
          </view>
        </view>

        <view v-else-if="activeTab === 'orders'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">区域订单</text>
              <text class="soft-tag">{{ operationOrders.length }} 笔</text>
            </view>
            <view v-if="operationOrders.length" class="list-stack">
              <view v-for="order in operationOrders" :key="rowKey(order)" class="list-item">
                <view>
                  <text class="list-title">{{ orderTitle(order) }}</text>
                  <text class="list-meta">{{ formatDate(rowDate(order)) }}</text>
                </view>
                <view class="list-side">
                  <text class="price-text">{{ formatCurrency(rowAmount(order)) }}</text>
                  <text :class="['status-tag', statusTone(rowStatus(order))]">{{ rowStatus(order) }}</text>
                </view>
              </view>
            </view>
            <text v-else class="empty-text">暂无区域订单。</text>
          </view>
        </view>

        <view v-else-if="activeTab === 'commission'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">中心分润</text>
              <text class="soft-tag">{{ operationCommissions.length }} 条</text>
            </view>
            <view v-if="operationCommissions.length" class="list-stack">
              <view v-for="commission in operationCommissions" :key="rowKey(commission)" class="list-item">
                <view>
                  <text class="list-title">{{ rowString(commission, "agentName") || "代理订单" }}</text>
                  <text class="list-meta">{{ formatDate(rowDate(commission)) }}</text>
                </view>
                <text class="price-text">{{ formatCurrency(rowAmount(commission)) }}</text>
              </view>
            </view>
            <text v-else class="empty-text">暂无中心分润。</text>
          </view>
        </view>

        <view v-else class="section-stack">
          <view class="profile-card">
            <image class="profile-logo" :src="loginLogo" mode="aspectFit" />
            <view>
              <text class="profile-name">{{ operationName }}</text>
              <text class="profile-meta">运营中心 · {{ operationStatus }}</text>
            </view>
          </view>
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">5000 运营中心开通包</text>
              <text class="price-badge">5000 元</text>
            </view>
            <text class="body-copy">用于开通区域运营中心，承接代理商团队、订单和分润看板。</text>
            <button type="button" class="outline-button" @click="createOperationOrder">创建运营中心订单</button>
          </view>
        </view>
      </template>
    </view>

    <view class="bottom-tabs">
      <button
        v-for="tab in currentTabs"
        :key="tab.id"
        type="button"
        :class="['tab-button', { active: activeTab === tab.id }]"
        @click="activeTab = tab.id"
      >
        <text class="tab-icon">{{ tab.icon }}</text>
        <text>{{ tab.label }}</text>
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { onShareAppMessage } from "@dcloudio/uni-app";
import { api, setAuthToken } from "../api/client";
import loginLogo from "../assets/zhiqiyun-logo-transparent.png";
import type { Asset, AuthResponse, ChannelAgent, ChannelCenterResponse, PointAccount } from "../types";

type AnyRecord = Record<string, unknown>;
type RoleId = "user" | "agent" | "operation";
type TabId = "home" | "create" | "assets" | "wallet" | "mine" | "overview" | "promotion" | "customers" | "commission" | "agents" | "orders";
type CreationMode = "image" | "video" | "ppt";

interface WalletResponse {
  account?: PointAccount;
  tokenRecords?: AnyRecord[];
  orders?: AnyRecord[];
  transactions?: AnyRecord[];
}

interface MemberProfileResponse {
  user?: AuthResponse["user"];
  account?: PointAccount;
  plan?: AnyRecord;
  agent?: (ChannelAgent & AnyRecord) | null;
  operationCenter?: AnyRecord | null;
}

interface PromotionInfo {
  inviteCode?: string;
  inviteLink?: string;
  landingURL?: string;
}

interface OperationProfileResponse {
  user?: AuthResponse["user"];
  operationCenter?: AnyRecord | null;
  joinPlan?: AnyRecord;
  summary?: AnyRecord;
}

interface ItemsResponse<T = AnyRecord> {
  items?: T[];
  rows?: T[];
  data?: T[];
  summary?: AnyRecord;
}

const roleTabs: Record<RoleId, Array<{ id: TabId; label: string; icon: string }>> = {
  user: [
    { id: "home", label: "首页", icon: "首" },
    { id: "create", label: "创作", icon: "创" },
    { id: "assets", label: "作品", icon: "作" },
    { id: "wallet", label: "钱包", icon: "钱" },
    { id: "mine", label: "我的", icon: "我" }
  ],
  agent: [
    { id: "overview", label: "概览", icon: "总" },
    { id: "promotion", label: "推广", icon: "推" },
    { id: "customers", label: "客户", icon: "客" },
    { id: "commission", label: "分润", icon: "润" },
    { id: "mine", label: "我的", icon: "我" }
  ],
  operation: [
    { id: "overview", label: "概览", icon: "总" },
    { id: "agents", label: "代理", icon: "代" },
    { id: "orders", label: "订单", icon: "单" },
    { id: "commission", label: "分润", icon: "润" },
    { id: "mine", label: "我的", icon: "我" }
  ]
};

const roleNames: Record<RoleId, string> = {
  user: "普通用户",
  agent: "代理商",
  operation: "运营中心"
};

const creationModules = [
  { id: "image" as CreationMode, icon: "图", name: "AI 生图", model: "gpt-image-2", cost: "约 10 点/张" },
  { id: "video" as CreationMode, icon: "视", name: "视频生成", model: "doubao-seedance-2.0", cost: "约 80 点/条" },
  { id: "ppt" as CreationMode, icon: "P", name: "PPT 文档", model: "ppt-generator", cost: "约 30 点/份" }
];

const rechargePackages = [
  { id: "recharge_1990", amountCents: 1990, points: 1990 },
  { id: "recharge_9900", amountCents: 9900, points: 9900 },
  { id: "recharge_29900", amountCents: 29900, points: 29900 },
  { id: "recharge_99900", amountCents: 99900, points: 99900 }
];

const auth = ref<AuthResponse | null>(null);
const token = ref("");
const pageLoading = ref(false);
const pageError = ref("");
const activeRole = ref<RoleId>("user");
const activeTab = ref<TabId>("home");
const creationMode = ref<CreationMode>("image");
const creationPrompt = ref("");
const roleInitialized = ref(false);

const profile = ref<MemberProfileResponse | null>(null);
const wallet = ref<WalletResponse | null>(null);
const pointAccountResponse = ref<WalletResponse | null>(null);
const recentAssets = ref<Asset[]>([]);
const assetsLoading = ref(false);
const assetsError = ref("");

const channelCenter = ref<ChannelCenterResponse | null>(null);
const operationProfile = ref<OperationProfileResponse | null>(null);
const operationAgentsResponse = ref<ItemsResponse | null>(null);
const operationOrdersResponse = ref<ItemsResponse | null>(null);
const operationCommissionsResponse = ref<ItemsResponse | null>(null);

const currentTabs = computed(() => roleTabs[activeRole.value]);
const roleLabel = computed(() => roleNames[activeRole.value]);
const displayName = computed(() => profile.value?.user?.name || auth.value?.user?.name || profile.value?.user?.email || auth.value?.user?.email || "当前用户");
const userEmail = computed(() => profile.value?.user?.email || auth.value?.user?.email || "-");
const greetingText = computed(() => `${displayName.value}，欢迎回来`);
const planName = computed(() => rowString(profile.value?.plan || {}, "name") || rowString(profile.value?.plan || {}, "planName") || auth.value?.defaultModule || "AI 创作用户");

const pointAccount = computed(() => wallet.value?.account || pointAccountResponse.value?.account || profile.value?.account || null);
const pointBalance = computed(() => asNumber(pointAccount.value?.available));
const pointFrozen = computed(() => asNumber(pointAccount.value?.frozen));
const userOrders = computed(() => listOf(wallet.value?.orders || pointAccountResponse.value?.orders));
const userTransactions = computed(() => listOf(wallet.value?.transactions || pointAccountResponse.value?.transactions));
const tokenRecords = computed(() => listOf(wallet.value?.tokenRecords));
const walletRecords = computed(() => tokenRecords.value.length ? tokenRecords.value : userTransactions.value);
const monthlyPointCost = computed(() => walletRecords.value.reduce((sum, item) => sum + Math.abs(rowPointCost(item)), 0));

const isAgentActive = computed(() => Boolean(profile.value?.agent || auth.value?.agent || channelCenter.value?.agent));
const isOperationActive = computed(() => Boolean(profile.value?.operationCenter || operationProfile.value?.operationCenter || hasOperationHint()));
const availableRoles = computed(() => {
  const roles = [{ id: "user" as RoleId, label: "用户" }];
  if (isAgentActive.value) roles.push({ id: "agent" as RoleId, label: "代理商" });
  if (isOperationActive.value) roles.push({ id: "operation" as RoleId, label: "运营中心" });
  return roles;
});

const activeCreation = computed(() => creationModules.find(item => item.id === creationMode.value) || creationModules[0]);
const activeCreationName = computed(() => activeCreation.value.name);
const activeCreationModel = computed(() => activeCreation.value.model);
const activeCreationCost = computed(() => activeCreation.value.cost);

const channelSummary = computed(() => (channelCenter.value?.summary || {}) as AnyRecord);
const channelCustomers = computed(() => listOf(channelCenter.value?.customers));
const channelCommissions = computed(() => listOf(channelCenter.value?.commissions));
const channelWithdrawals = computed(() => listOf(channelCenter.value?.withdrawals));
const currentAgent = computed(() => (channelCenter.value?.agent || profile.value?.agent || auth.value?.agent || null) as (ChannelAgent & AnyRecord) | null);
const agentName = computed(() => currentAgent.value?.name || displayName.value);
const agentLevelLabel = computed(() => rowString(currentAgent.value || {}, "levelLabel") || rowString(currentAgent.value || {}, "levelName") || `L${asNumber(currentAgent.value?.level, 1)} 代理商`);
const agentStatus = computed(() => rowString(currentAgent.value || {}, "status") || "ACTIVE");
const promotionInfo = computed<PromotionInfo>(() => (channelCenter.value as unknown as { promotion?: PromotionInfo } | null)?.promotion || {});
const inviteCode = computed(() => promotionInfo.value.inviteCode || currentAgent.value?.inviteCode || rowString(currentAgent.value || {}, "inviteCode") || "ZQAI996");
const inviteLink = computed(() => promotionInfo.value.inviteLink || promotionInfo.value.landingURL || rowString(currentAgent.value || {}, "inviteLink") || `https://xianzhi.ai/app?invite=${inviteCode.value}`);
const sharePath = computed(() => `/pages/WechatLoginPage?invite=${encodeURIComponent(inviteCode.value)}`);

const operationName = computed(() => rowString(operationProfile.value?.operationCenter || {}, "name") || "知启云运营中心");
const operationStatus = computed(() => rowString(operationProfile.value?.operationCenter || {}, "status") || "ACTIVE");
const operationAgents = computed(() => listOf(operationAgentsResponse.value?.items || operationAgentsResponse.value?.rows || operationAgentsResponse.value?.data));
const operationOrders = computed(() => listOf(operationOrdersResponse.value?.items || operationOrdersResponse.value?.rows || operationOrdersResponse.value?.data));
const operationCommissions = computed(() => listOf(operationCommissionsResponse.value?.items || operationCommissionsResponse.value?.rows || operationCommissionsResponse.value?.data));
const operationCommissionTotal = computed(() => operationCommissions.value.reduce((sum, item) => sum + rowAmount(item), 0));

const roleWithdrawable = computed(() => {
  if (activeRole.value === "operation") return operationCommissionTotal.value;
  return summaryNumber(channelSummary.value, "availableToWithdraw");
});
const secondaryMetricLabel = computed(() => {
  if (activeRole.value === "user") return "最近作品";
  if (activeRole.value === "operation") return "区域订单";
  return "拓展客户";
});
const secondaryMetricValue = computed(() => {
  if (activeRole.value === "user") return `${recentAssets.value.length}`;
  if (activeRole.value === "operation") return `${operationOrders.value.length}`;
  return `${summaryNumber(channelSummary.value, "directCustomers")}`;
});

watch(activeRole, (role) => {
  const nextTab = roleTabs[role][0]?.id || "home";
  if (!roleTabs[role].some(item => item.id === activeTab.value)) {
    activeTab.value = nextTab;
  } else {
    activeTab.value = nextTab;
  }
  void loadRoleData(role);
});

onShareAppMessage(() => ({
  title: "知启云 AI 邀请你一起创作",
  path: sharePath.value
}));

function asNumber(value: unknown, fallback = 0) {
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : fallback;
}

function asString(value: unknown, fallback = "") {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function listOf(value: unknown): AnyRecord[] {
  return Array.isArray(value) ? value.filter(item => item && typeof item === "object") as AnyRecord[] : [];
}

function rowString(row: unknown, key: string) {
  if (!row || typeof row !== "object") return "";
  return asString((row as AnyRecord)[key]);
}

function rowNumber(row: unknown, key: string) {
  if (!row || typeof row !== "object") return 0;
  return asNumber((row as AnyRecord)[key]);
}

function rowDate(row: unknown) {
  return rowString(row, "createdAt") || rowString(row, "occurredAt") || rowString(row, "updatedAt") || rowString(row, "paidAt");
}

function rowStatus(row: unknown) {
  return rowString(row, "status") || "PENDING";
}

function rowAmount(row: unknown) {
  return rowNumber(row, "amountCents") || rowNumber(row, "commissionCents") || rowNumber(row, "priceCents");
}

function rowPointCost(row: unknown) {
  return rowNumber(row, "pointCost") || rowNumber(row, "points") || Math.abs(rowNumber(row, "delta"));
}

function rowKey(row: unknown) {
  if (!row || typeof row !== "object") return String(Math.random());
  const record = row as AnyRecord;
  return String(record.id || record.orderId || record.transactionId || record.userId || record.email || JSON.stringify(record));
}

function orderTitle(row: unknown) {
  return rowString(row, "planName") || rowString(row, "plan") || rowString(row, "subject") || rowString(row, "orderId") || "知启云订单";
}

function usageTitle(row: unknown) {
  return rowString(row, "metricCode") || rowString(row, "model") || rowString(row, "type") || "AI 创作消耗";
}

function customerName(row: unknown) {
  return rowString(row, "name") || rowString(row, "nickname") || rowString(row, "email") || "未命名用户";
}

function customerEmail(row: unknown) {
  return rowString(row, "email") || rowString(row, "phone") || rowString(row, "userId") || "-";
}

function summaryNumber(summary: AnyRecord, key: string) {
  return asNumber(summary[key]);
}

function formatNumber(value: unknown) {
  return asNumber(value).toLocaleString("zh-CN");
}

function formatCurrency(amountCents: unknown) {
  return `¥${(asNumber(amountCents) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatDate(value?: string) {
  if (!value) return "刚刚";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hour = String(date.getHours()).padStart(2, "0");
  const minute = String(date.getMinutes()).padStart(2, "0");
  return `${month}/${day} ${hour}:${minute}`;
}

function statusTone(status: string) {
  const normalized = status.toUpperCase();
  if (["PAID", "ACTIVE", "SUCCEEDED", "SETTLED", "SUCCESS"].includes(normalized)) return "success";
  if (["FAILED", "CANCELLED", "REJECTED"].includes(normalized)) return "danger";
  return "warning";
}

function hasOperationHint() {
  const route = `${auth.value?.defaultRoute || ""} ${auth.value?.defaultModule || ""} ${auth.value?.workspace || ""}`.toLowerCase();
  const role = auth.value?.user?.role?.toLowerCase() || "";
  return route.includes("operation") || role.includes("operation");
}

function readAuth() {
  token.value = uni.getStorageSync("token") || "";
  const storedAuth = uni.getStorageSync("xianzhiMiniProgramAuth") as AuthResponse | "";
  auth.value = storedAuth || null;
  if (token.value) setAuthToken(token.value);
  if (!token.value) {
    uni.reLaunch({ url: "/pages/WechatLoginPage" });
  }
}

function inferInitialRole() {
  if (hasOperationHint()) return "operation";
  const workspace = String(auth.value?.workspace || "").toLowerCase();
  const role = String(auth.value?.user?.role || "").toLowerCase();
  if (workspace === "agent" || role.includes("agent") || auth.value?.agent || profile.value?.agent) return "agent";
  return "user";
}

function switchRole(role: RoleId) {
  activeRole.value = role;
}

function selectUserTab(tab: TabId) {
  activeRole.value = "user";
  activeTab.value = tab;
}

function selectAgentTab(tab: TabId) {
  activeRole.value = "agent";
  activeTab.value = tab;
}

async function refreshAll() {
  readAuth();
  if (!token.value) return;
  pageLoading.value = true;
  pageError.value = "";
  try {
    await Promise.all([loadMemberProfile(), loadWallet(), loadAssets(false)]);
    const inferredRole = inferInitialRole();
    if (!roleInitialized.value && inferredRole !== "user") {
      activeRole.value = inferredRole;
    }
    roleInitialized.value = true;
    await loadRoleData(activeRole.value);
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : "工作台加载失败";
  } finally {
    pageLoading.value = false;
  }
}

async function loadMemberProfile() {
  profile.value = await api<MemberProfileResponse>("/api/v1/member/profile");
}

async function loadWallet() {
  const [walletResult, pointsResult] = await Promise.allSettled([
    api<WalletResponse>("/api/v1/member/wallet"),
    api<WalletResponse>("/api/v1/points/account")
  ]);
  if (walletResult.status === "fulfilled") wallet.value = walletResult.value;
  if (pointsResult.status === "fulfilled") pointAccountResponse.value = pointsResult.value;
}

async function loadAssets(showLoading = true) {
  if (!token.value) return;
  if (showLoading) assetsLoading.value = true;
  assetsError.value = "";
  try {
    const items = await api<Asset[] | { items?: Asset[] }>("/api/v1/assets");
    const assetItems = Array.isArray(items) ? items : items.items || [];
    recentAssets.value = assetItems.slice(0, 8);
  } catch (error) {
    assetsError.value = error instanceof Error ? error.message : "作品加载失败";
  } finally {
    if (showLoading) assetsLoading.value = false;
  }
}

async function loadRoleData(role: RoleId) {
  if (!token.value) return;
  if (role === "agent" && isAgentActive.value) {
    try {
      channelCenter.value = await api<ChannelCenterResponse>("/api/v1/channel/me");
    } catch (error) {
      channelCenter.value = null;
    }
  }
  if (role === "operation" && isOperationActive.value) {
    const [profileResult, agentsResult, ordersResult, commissionsResult] = await Promise.allSettled([
      api<OperationProfileResponse>("/api/v1/operation-center/profile"),
      api<ItemsResponse>("/api/v1/operation-center/agents"),
      api<ItemsResponse>("/api/v1/operation-center/orders"),
      api<ItemsResponse>("/api/v1/operation-center/commissions")
    ]);
    if (profileResult.status === "fulfilled") operationProfile.value = profileResult.value;
    if (agentsResult.status === "fulfilled") operationAgentsResponse.value = agentsResult.value;
    if (ordersResult.status === "fulfilled") operationOrdersResponse.value = ordersResult.value;
    if (commissionsResult.status === "fulfilled") operationCommissionsResponse.value = commissionsResult.value;
  }
}

async function createOrder(endpoint: string, planId: string, amountCents: number, successTitle: string) {
  try {
    await api(endpoint, {
      method: "POST",
      body: JSON.stringify({
        planId,
        amountCents,
        paymentMethod: "wechat_mini_program",
        idempotencyKey: `${planId}-mini-${Date.now()}`
      })
    });
    uni.showToast({ title: successTitle, icon: "success" });
    await refreshAll();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建订单失败", icon: "none" });
  }
}

async function createAgentOrder() {
  await createOrder("/api/v1/agent/join-order", "plan_agent_join_996", 99600, "代理商订单已创建");
}

async function createMemberPackageOrder() {
  await createOrder("/api/v1/orders/create", "plan_ai_creator_996", 99600, "会员订单已创建");
}

async function createOperationOrder() {
  await createOrder("/api/v1/operation-center/join-order", "plan_operation_center_5000", 500000, "运营中心订单已创建");
}

async function createRechargeOrder(pack: { id: string; amountCents: number; points: number }) {
  try {
    await api("/api/v1/points/recharge-orders", {
      method: "POST",
      body: JSON.stringify({
        rechargePackageId: pack.id,
        amountCents: pack.amountCents,
        paymentMethod: "wechat_mini_program",
        idempotencyKey: `${pack.id}-mini-${Date.now()}`
      })
    });
    uni.showToast({ title: "充值订单已创建", icon: "success" });
    await loadWallet();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建充值订单失败", icon: "none" });
  }
}

async function requestWithdrawal() {
  const amountCents = summaryNumber(channelSummary.value, "availableToWithdraw");
  if (amountCents <= 0) {
    uni.showToast({ title: "暂无可提现收益", icon: "none" });
    return;
  }
  try {
    await api("/api/v1/channel/withdrawals", {
      method: "POST",
      body: JSON.stringify({ amountCents })
    });
    uni.showToast({ title: "提现申请已提交", icon: "success" });
    await loadRoleData("agent");
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "提现申请失败", icon: "none" });
  }
}

function copyInviteLink() {
  uni.setClipboardData({
    data: inviteLink.value,
    success: () => uni.showToast({ title: "推广链接已复制", icon: "success" })
  });
}

function agentCondition(key: "openCondition" | "keepCondition") {
  const value = currentAgent.value?.[key];
  if (typeof value === "string") return value;
  if (value && typeof value === "object") return "按平台规则";
  return key === "openCondition" ? "996 元开通代理身份" : "持续拓展客户并保持有效订单";
}

function showChildAgentHint() {
  uni.showToast({ title: "下级代理表单保留在桌面端，小程序展示团队入口", icon: "none" });
}

function showComingSoon(moduleName: string) {
  uni.showToast({ title: `${moduleName}入口已就绪，真实生成任务沿用桌面端接口接入`, icon: "none" });
}

function logout() {
  setAuthToken("");
  uni.removeStorageSync("xianzhiMiniProgramAuth");
  uni.reLaunch({ url: "/pages/WechatLoginPage" });
}

onMounted(() => {
  void refreshAll();
});
</script>

<style scoped>
.mini-workbench {
  min-height: 100vh;
  padding: 0 20px 112px;
  box-sizing: border-box;
  color: #111827;
  background: #f7f8fc;
}

.status-bar-spacer {
  height: 42px;
}

.hero-panel {
  padding: 18px;
  border-radius: 18px;
  background: linear-gradient(135deg, #fefefe 0%, #eef1ff 100%);
  border: 1px solid #e5e7eb;
  box-shadow: 0 16px 40px rgba(17, 24, 39, 0.08);
}

.brand-row,
.section-header,
.profile-card,
.list-item,
.menu-row,
.config-row {
  display: flex;
  align-items: center;
}

.brand-row {
  gap: 12px;
}

.brand-logo {
  width: 54px;
  height: 54px;
  flex: 0 0 54px;
}

.brand-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.brand-eyebrow,
.section-kicker,
.metric-label,
.quick-label,
.wallet-label,
.list-meta,
.work-meta,
.profile-meta,
.body-copy,
.strip-copy,
.empty-text {
  color: #6b7280;
}

.brand-eyebrow,
.section-kicker {
  font-size: 12px;
  font-weight: 700;
}

.brand-title {
  font-size: 20px;
  font-weight: 800;
  line-height: 1.25;
}

.icon-button {
  width: 40px;
  height: 40px;
  padding: 0;
  border-radius: 14px;
  color: #5a4db2;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  font-size: 13px;
}

.role-switcher {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}

.role-pill {
  flex: 1;
  height: 36px;
  padding: 0 8px;
  border-radius: 999px;
  color: #6b7280;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  font-size: 13px;
}

.role-pill.active {
  color: #ffffff;
  border-color: #7d8df6;
  background: #7d8df6;
}

.hero-metrics,
.quick-grid,
.creation-grid,
.recharge-grid {
  display: grid;
  gap: 10px;
}

.hero-metrics {
  grid-template-columns: 1.2fr 1fr;
  margin-top: 16px;
}

.metric-card,
.section-card,
.wallet-card,
.profile-card,
.promo-card,
.state-card,
.upgrade-strip {
  border-radius: 16px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
}

.metric-card {
  padding: 14px;
}

.metric-card.primary {
  color: #ffffff;
  border-color: #5a4db2;
  background: #5a4db2;
}

.metric-card.primary .metric-label {
  color: rgba(255, 255, 255, 0.78);
}

.metric-label {
  display: block;
  font-size: 12px;
}

.metric-value {
  display: block;
  margin-top: 8px;
  font-size: 20px;
  font-weight: 800;
}

.state-card {
  margin-top: 14px;
  padding: 18px;
  color: #6b7280;
}

.state-card.error {
  color: #dc2626;
  border-color: #fecaca;
  background: #fff7f7;
}

.role-content {
  margin-top: 16px;
}

.section-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-card,
.wallet-card,
.promo-card {
  padding: 16px;
}

.section-header {
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.section-header.compact {
  margin-bottom: 10px;
}

.section-title {
  display: block;
  margin-top: 3px;
  font-size: 17px;
  font-weight: 800;
}

.soft-tag,
.status-tag,
.price-badge {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 9px;
  border-radius: 999px;
  font-size: 12px;
  background: #f3f4f6;
  color: #6b7280;
}

.price-badge {
  color: #ff771b;
  background: #fff3ea;
}

.quick-grid {
  grid-template-columns: repeat(3, 1fr);
}

.quick-item {
  min-width: 0;
  padding: 12px 8px;
  border-radius: 12px;
  background: #f7f8fc;
}

.quick-value {
  display: block;
  min-height: 24px;
  font-size: 18px;
  font-weight: 800;
  color: #111827;
}

.quick-label {
  display: block;
  margin-top: 4px;
  font-size: 11px;
}

.upgrade-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  background: #fff7ed;
  border-color: #fed7aa;
}

.strip-title {
  display: block;
  font-size: 16px;
  font-weight: 800;
  color: #9a3412;
}

.strip-copy {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
}

.strip-button {
  flex: 0 0 90px;
  height: 38px;
  padding: 0 10px;
  border-radius: 12px;
  color: #ffffff;
  background: #ff771b;
  font-size: 13px;
  font-weight: 700;
}

.list-stack,
.work-list,
.menu-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.list-item {
  justify-content: space-between;
  gap: 10px;
  padding: 12px;
  border-radius: 12px;
  background: #f9fafb;
}

.list-title,
.work-title,
.profile-name {
  display: block;
  font-size: 14px;
  font-weight: 700;
}

.list-meta,
.work-meta {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}

.list-side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 5px;
}

.price-text,
.cost-text {
  font-size: 14px;
  font-weight: 800;
  color: #111827;
}

.cost-text {
  color: #dc2626;
}

.status-tag.success {
  color: #047857;
  background: #d1fae5;
}

.status-tag.warning {
  color: #b45309;
  background: #fef3c7;
}

.status-tag.danger {
  color: #dc2626;
  background: #fee2e2;
}

.text-button,
.small-button {
  margin: 0;
  padding: 0 10px;
  height: 30px;
  border-radius: 10px;
  color: #5a4db2;
  background: #eef1ff;
  font-size: 12px;
}

.small-button {
  margin-top: 10px;
  color: #ffffff;
  background: #5a4db2;
}

.creation-grid,
.recharge-grid {
  grid-template-columns: repeat(3, 1fr);
}

.creation-card,
.recharge-card {
  min-height: 92px;
  padding: 10px 6px;
  border-radius: 14px;
  background: #f9fafb;
  border: 1px solid #e5e7eb;
}

.creation-card.active {
  border-color: #7d8df6;
  background: #eef1ff;
}

.creation-icon,
.creation-name,
.creation-cost,
.recharge-points,
.recharge-price {
  display: block;
}

.creation-icon {
  font-size: 20px;
  font-weight: 800;
  color: #5a4db2;
}

.creation-name,
.recharge-points {
  margin-top: 8px;
  font-size: 13px;
  font-weight: 800;
}

.creation-cost,
.recharge-price {
  margin-top: 4px;
  font-size: 11px;
  color: #6b7280;
}

.prompt-input {
  width: 100%;
  min-height: 130px;
  margin-top: 12px;
  padding: 12px;
  box-sizing: border-box;
  border-radius: 14px;
  background: #f9fafb;
  border: 1px solid #e5e7eb;
  font-size: 14px;
  line-height: 1.6;
}

.primary-button,
.outline-button,
.wallet-button {
  width: 100%;
  height: 46px;
  margin-top: 14px;
  border-radius: 14px;
  color: #ffffff;
  background: #7d8df6;
  font-size: 15px;
  font-weight: 800;
}

.outline-button {
  color: #5a4db2;
  background: #ffffff;
  border: 1px solid #c7d2fe;
}

.body-copy {
  display: block;
  font-size: 13px;
  line-height: 1.6;
}

.config-row,
.menu-row {
  justify-content: space-between;
  gap: 12px;
  padding: 11px 0;
  border-top: 1px solid #f3f4f6;
  font-size: 13px;
}

.config-row:first-child,
.menu-row:first-child {
  border-top: 0;
}

.config-row text:last-child,
.menu-row text:last-child {
  color: #6b7280;
  text-align: right;
}

.wallet-card {
  color: #ffffff;
  background: #111827;
  border-color: #111827;
}

.wallet-card.agent {
  background: #0f766e;
  border-color: #0f766e;
}

.wallet-label,
.wallet-copy {
  color: rgba(255, 255, 255, 0.72);
}

.wallet-value {
  display: block;
  margin-top: 6px;
  font-size: 34px;
  font-weight: 900;
}

.wallet-copy {
  display: block;
  margin-top: 4px;
  font-size: 13px;
}

.wallet-button {
  color: #0f766e;
  background: #ffffff;
}

.work-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 12px;
  background: #f9fafb;
}

.work-thumb {
  width: 54px;
  height: 54px;
  flex: 0 0 54px;
  border-radius: 12px;
  overflow: hidden;
  background: #e5e7eb;
}

.work-thumb.fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #5a4db2;
  font-weight: 800;
}

.work-main {
  min-width: 0;
  flex: 1;
}

.profile-card {
  gap: 12px;
  padding: 14px;
}

.profile-logo {
  width: 50px;
  height: 50px;
  flex: 0 0 50px;
}

.profile-meta {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}

.menu-list {
  padding: 4px 16px;
  border-radius: 16px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
}

.menu-row.danger text:first-child {
  color: #dc2626;
}

.promo-card {
  text-align: center;
}

.qr-box {
  width: 168px;
  height: 168px;
  margin: 0 auto 14px;
  padding: 16px;
  box-sizing: border-box;
  border-radius: 18px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
}

.qr-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
}

.qr-cell {
  aspect-ratio: 1;
  border-radius: 4px;
  background: #e5e7eb;
}

.qr-cell.dark {
  background: #111827;
}

.invite-code {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 150px;
  height: 42px;
  margin-top: 12px;
  padding: 0 16px;
  border-radius: 14px;
  color: #5a4db2;
  background: #eef1ff;
  font-size: 18px;
  font-weight: 900;
  letter-spacing: 0;
}

.empty-text {
  display: block;
  padding: 16px 0;
  font-size: 13px;
  line-height: 1.6;
}

.bottom-tabs {
  position: fixed;
  left: 12px;
  right: 12px;
  bottom: 12px;
  z-index: 20;
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 4px;
  padding: 8px;
  border-radius: 20px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  box-shadow: 0 18px 50px rgba(17, 24, 39, 0.16);
}

.tab-button {
  height: 54px;
  padding: 5px 0;
  border-radius: 14px;
  color: #6b7280;
  background: transparent;
  font-size: 11px;
  line-height: 1.1;
}

.tab-button.active {
  color: #5a4db2;
  background: #eef1ff;
}

.tab-icon {
  display: block;
  margin-bottom: 4px;
  font-size: 15px;
  font-weight: 800;
}
</style>
