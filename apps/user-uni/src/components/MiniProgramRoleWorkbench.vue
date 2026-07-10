<template>
  <view class="mini-workbench">
    <view class="native-safe-note"></view>

    <view v-if="!isUserMineDetail" class="business-header">
      <image class="business-logo" :src="loginLogo" mode="aspectFit" />
      <view class="business-copy">
        <text class="business-title">{{ currentPageTitle }}</text>
        <text class="business-subtitle">{{ currentPageSubtitle }}</text>
      </view>
      <button v-if="availableRoles.length === 1" class="role-badge" type="button">{{ roleLabel }}</button>
      <button v-else class="role-badge switchable" type="button" @click="cycleRole">{{ roleLabel }}⌄</button>
    </view>

    <view class="role-switcher" v-if="availableRoles.length > 1 && !isUserMineDetail">
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
          <view class="v31-home-hero">
            <text class="v31-kicker">一句话开始</text>
            <text class="v31-hero-title">用 AI 完成设计、视频与 PPT</text>
            <text class="v31-hero-copy">从创作到判断，一站式解决。</text>
            <view class="v31-hero-row">
              <view class="v31-mini-metric purple">
                <text class="v31-metric-value">{{ formatNumber(pointBalance) }}</text>
                <text class="v31-metric-label">点数余额</text>
              </view>
              <view class="v31-mini-metric orange">
                <text class="v31-metric-value">{{ recentAssets.length }}</text>
                <text class="v31-metric-label">近期作品</text>
              </view>
              <button type="button" class="v31-orange-button" @click="selectUserTab('create')">去创作</button>
            </view>
          </view>

          <text class="v31-section-title">常用工具</text>
          <view class="v31-tool-grid">
            <button v-for="module in creationModules" :key="`home-${module.id}`" class="v31-tool-card" @click="openCreation(module.id)">
              <text :class="['v31-tool-icon', module.tone]">{{ module.icon }}</text>
              <view class="v31-tool-copy">
                <text class="v31-tool-name">{{ module.homeName || module.name }}</text>
                <text class="v31-tool-desc">{{ module.description }}</text>
              </view>
            </button>
          </view>

          <text class="v31-section-title">灵感推荐</text>
          <view class="v31-inspiration-grid">
            <button class="v31-inspiration-card" @click="openCreation('image')">
              <view class="v31-preview orange">图</view>
              <text class="v31-inspiration-title">水果电商主图</text>
              <view class="v31-card-footer"><text class="v31-chip orange">图片</text><text class="v31-link">继续改</text></view>
            </button>
            <button class="v31-inspiration-card" @click="openCreation('ppt')">
              <view class="v31-preview purple">P</view>
              <text class="v31-inspiration-title">招商路演PPT</text>
              <view class="v31-card-footer"><text class="v31-chip purple">PPT</text><text class="v31-link">继续改</text></view>
            </button>
          </view>
        </view>

        <view v-else-if="activeTab === 'create'" class="section-stack">
          <template v-if="creationMode === 'agent'">
            <KnowledgeMiniChat embedded @close="returnToCreationHub" />
          </template>
          <template v-else-if="creationMode === 'ppt'">
            <view class="v31-subpage-nav">
              <button class="v31-back-button" aria-label="返回创作" @click="returnToCreationHub">‹</button>
              <view>
                <text class="v31-subpage-title">PPT文档生成</text>
                <text class="v31-subpage-copy">返回创作不会丢失当前草稿</text>
              </view>
            </view>
            <view class="v31-ppt-panel">
              <text class="v31-ppt-title">您今天想制作什么样的演示文稿？</text>
              <textarea v-model="creationPrompt" class="v31-ppt-input" maxlength="500" placeholder="请输入主题，例如：AI赋能企业营销增长方案" @input="creationError = ''" />
              <view class="v31-ppt-options">
                <button @click="cyclePptSlides">{{ pptSlideCount }}张幻灯片</button>
                <button :class="{ active: pptDynamic }" @click="pptDynamic = !pptDynamic">{{ pptDynamic ? "动态的" : "静态的" }}</button>
                <button class="active" @click="togglePptLanguage">{{ pptLanguage === "zh" ? "中文" : "英文" }}</button>
                <button @click="cyclePptModel">{{ pptModel }}</button>
                <view
                  :class="['v31-ppt-submit', { disabled: generationSubmitting }]"
                  role="button"
                  hover-class="v31-action-pressed"
                  @touchend.stop="handleGenerateTap"
                >{{ generationSubmitting ? "…" : "→" }}</view>
              </view>
              <text v-if="creationError" class="v31-generation-error">{{ creationError }}</text>
            </view>
            <text class="v31-section-title">示例主题</text>
            <view class="v31-example-grid">
              <button v-for="topic in pptTopics" :key="topic" @click="creationPrompt = topic; creationError = ''">{{ topic }}</button>
            </view>
            <view v-if="latestGenerationTask" :class="['v31-generation-state', latestGenerationTask.tone]">
              <view><text class="v31-generation-title">{{ latestGenerationTask.title }}</text><text class="v31-generation-meta">任务 {{ latestGenerationTask.id }} · {{ latestGenerationTask.status }}</text></view>
              <button @click="selectUserTab('assets')">查看作品</button>
            </view>
            <view class="v31-draft-card">
              <text class="v31-draft-title">未完成项目会保留在最近浏览</text>
              <text class="v31-draft-copy">选择文本内容、自定义主题后，即使返回首页，也能继续从草稿进入。</text>
              <view class="v31-draft-actions"><button>草稿</button><button class="dark">生成大纲</button><button>主题预览</button></view>
            </view>
          </template>

          <template v-else>
            <view class="v31-prompt-panel">
              <text class="v31-prompt-title">今天想做什么？</text>
              <textarea v-model="creationPrompt" class="v31-one-line-input" maxlength="500" placeholder="例如：生成一张水果店开业促销海报，橙色系，高级感" @input="creationError = ''" />
              <view class="v31-prompt-actions">
                <text class="v31-chip purple">自动匹配工具</text>
                <text class="v31-chip green">多模型对比</text>
                <view
                  :class="['v31-generate-button', { disabled: generationSubmitting }]"
                  role="button"
                  hover-class="v31-action-pressed"
                  @touchend.stop="handleGenerateTap"
                >{{ generationSubmitting ? "提交中..." : "生成" }}</view>
              </view>
              <text v-if="creationError" class="v31-generation-error">{{ creationError }}</text>
            </view>

            <view v-if="latestGenerationTask" :class="['v31-generation-state', latestGenerationTask.tone]">
              <view><text class="v31-generation-title">{{ latestGenerationTask.title }}</text><text class="v31-generation-meta">任务 {{ latestGenerationTask.id }} · {{ latestGenerationTask.status }}</text></view>
              <button @click="selectUserTab('assets')">查看作品</button>
            </view>

            <text class="v31-section-title">选择创作能力</text>
            <view class="v31-mode-grid">
              <button v-for="module in creationModules" :key="module.id" :class="['v31-mode-card', { active: creationMode === module.id }]" @click="selectCreationMode(module.id)">
                <text :class="['v31-tool-icon', module.tone]">{{ module.icon }}</text>
                <view class="v31-tool-copy"><text class="v31-tool-name">{{ module.name }}</text><text class="v31-tool-desc">{{ module.description }}</text></view>
              </button>
            </view>
            <view class="v31-workflow-card">
              <text class="v31-workflow-title">不满意？在结果上继续改</text>
              <text class="v31-workflow-copy">生成、对比、再创作在一个地方完成。</text>
              <view class="v31-workflow-tags"><text>复用参数</text><text>一键导出</text><text>继续编辑</text></view>
            </view>
          </template>
        </view>

        <view v-else-if="activeTab === 'assets'" class="section-stack">
          <view class="v31-filter-card">
            <view class="v31-filter-row">
              <button v-for="filter in assetFilters" :key="filter.id" :class="{ active: assetFilter === filter.id }" @click="assetFilter = filter.id">{{ filter.label }}</button>
            </view>
            <input v-model="assetSearch" class="v31-search-strip" placeholder="搜索作品名称" />
          </view>
          <view class="v31-works-card">
            <text v-if="assetsLoading" class="empty-text">正在加载作品...</text>
            <text v-else-if="assetsError" class="empty-text">{{ assetsError }}</text>
            <view v-else-if="filteredAssets.length" class="v31-work-grid">
              <button v-for="asset in filteredAssets" :key="asset.id" class="v31-work-card">
                <image v-if="asset.mediaType === 'image' && asset.thumbnailUrl" class="v31-work-preview" :src="asset.thumbnailUrl" mode="aspectFill" />
                <view v-else :class="['v31-work-preview', asset.mediaType === 'video' ? 'green' : 'purple']">{{ asset.mediaType === "video" ? "视" : asset.mediaType === "document" ? "P" : "图" }}</view>
                <text class="v31-work-title">{{ asset.name || asset.id }}</text>
                <view class="v31-card-footer"><text :class="['v31-chip', asset.mediaType === 'video' ? 'green' : asset.mediaType === 'image' ? 'orange' : 'purple']">{{ asset.mediaType === "video" ? "视频" : asset.mediaType === "image" ? "图片" : "PPT" }}</text><text class="v31-link">继续改</text></view>
              </button>
            </view>
            <view v-else class="v31-empty-state"><text>没有找到符合条件的作品</text><button @click="assetFilter = 'all'; assetSearch = ''">查看全部</button></view>
            <text class="v31-works-note">每个作品保留生成参数、消耗点数、导出记录。</text>
            <view class="v31-batch-actions"><button>批量导出</button><button class="active">继续编辑</button></view>
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

        <MiniProgramMineExperience
          v-else
          :view="mineView"
          :logo="loginLogo"
          :display-name="displayName"
          :point-balance="pointBalance"
          :monthly-point-cost="monthlyPointCost"
          :is-agent-active="isAgentActive"
          :agent-level-label="agentLevelLabel"
          :orders="userOrders"
          :usage-records="walletRecords"
          :invite-code="inviteCode"
          :invite-link="inviteLink"
          :channel-summary="channelSummary"
          :purchase="selectedMinePurchase"
          :purchase-submitting="minePurchaseSubmitting"
          :logout-confirm="mineLogoutConfirm"
          @navigate="openMineView"
          @select-purchase="selectMinePurchase"
          @close-purchase="selectedMinePurchase = null"
          @confirm-purchase="confirmMinePurchase"
          @request-logout="mineLogoutConfirm = true"
          @close-logout="mineLogoutConfirm = false"
          @confirm-logout="logout"
          @switch-agent="switchRole('agent')"
          @copy-invite="copyInviteLink"
          @invoice="showInvoiceNotice"
          @export-usage="showUsageExportNotice"
          @poster="showPosterNotice"
        />
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

    <view v-if="!isUserMineDetail" class="bottom-tabs" :style="{ gridTemplateColumns: `repeat(${currentTabs.length}, minmax(0, 1fr))` }">
      <button
        v-for="tab in currentTabs"
        :key="tab.id"
        type="button"
        :class="['tab-button', { active: activeTab === tab.id }]"
        @click="selectTab(tab.id)"
      >
        <text class="tab-icon">{{ tab.icon }}</text>
        <text>{{ tab.label }}</text>
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { onBackPress, onShareAppMessage } from "@dcloudio/uni-app";
import { api, authStorage, businessSdk, setAuthToken } from "../api/client";
import KnowledgeMiniChat from "./KnowledgeMiniChat.vue";
import MiniProgramMineExperience from "./MiniProgramMineExperience.vue";
import type { MinePurchaseOption, MineView } from "../types";
import loginLogo from "../assets/zhiqiyun-logo-transparent.png";
import type { ItemsResponse, MemberProfileResponse, OperationProfileResponse, RoleWalletResponse } from "@xianzhi/business-sdk";
import type { Asset, AuthResponse, ChannelAgent, ChannelCenterResponse } from "../types";

type NativeGenerateBridge = typeof globalThis & {
  __xianzhiMiniProgramGenerate?: () => void;
  __xianzhiMiniProgramBackToCreation?: () => void;
};

defineOptions({
  methods: {
    nativeGenerate() {
      const handler = (globalThis as NativeGenerateBridge).__xianzhiMiniProgramGenerate;
      if (typeof handler === "function") handler();
    },
    nativeBackToCreation() {
      const handler = (globalThis as NativeGenerateBridge).__xianzhiMiniProgramBackToCreation;
      if (typeof handler === "function") handler();
    }
  }
});

type AnyRecord = Record<string, unknown>;
type RoleId = "user" | "agent" | "operation";
type TabId = "home" | "create" | "assets" | "wallet" | "mine" | "overview" | "promotion" | "customers" | "commission" | "agents" | "orders";
type CreationMode = "image" | "video" | "ppt" | "infographic" | "review" | "agent";
type AssetFilter = "all" | "image" | "video" | "document" | "favorite";

interface PromotionInfo {
  inviteCode?: string;
  inviteLink?: string;
  landingURL?: string;
}

interface GenerationNotice {
  id: string;
  title: string;
  status: string;
  tone: "pending" | "success" | "danger";
}

const roleTabs: Record<RoleId, Array<{ id: TabId; label: string; icon: string }>> = {
  user: [
    { id: "home", label: "首页", icon: "⌂" },
    { id: "create", label: "创作", icon: "＋" },
    { id: "assets", label: "作品", icon: "▣" },
    { id: "mine", label: "我的", icon: "○" }
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
  { id: "image" as CreationMode, icon: "图", name: "AI生图", homeName: "轻易海报", description: "主图/海报/配图", model: "gpt-image-2", cost: "约 10 点/张", tone: "orange" },
  { id: "ppt" as CreationMode, icon: "P", name: "PPT文档", homeName: "PPT文档", description: "方案/培训/路演", model: "ppt-generator", cost: "约 30 点/份", tone: "purple" },
  { id: "video" as CreationMode, icon: "视", name: "视频生成", homeName: "视频生成", description: "广告/口播/图生视频", model: "doubao-seedance-2.0", cost: "约 80 点/条", tone: "green" },
  { id: "agent" as CreationMode, icon: "星", name: "AI Agent", homeName: "LOGO", description: "经营助手与知识库", model: "agent-workflow", cost: "按任务计费", tone: "blue" },
  { id: "infographic" as CreationMode, icon: "表", name: "信息图", homeName: "信息图", description: "复杂信息一图讲清", model: "infographic", cost: "约 20 点/份", tone: "orange" },
  { id: "review" as CreationMode, icon: "查", name: "易找茬", homeName: "易共识", description: "多模型判断与风险", model: "multi-model", cost: "按模型计费", tone: "purple" }
];

const pptTopics = ["企业营销增长", "数字员工方案", "GEO品牌曝光", "短视频矩阵", "项目路演计划", "糖尿病患教"];
const assetFilters: Array<{ id: AssetFilter; label: string }> = [
  { id: "all", label: "全部" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "document", label: "PPT" },
  { id: "favorite", label: "收藏" }
];

const mockAssets: Asset[] = [
  { id: "mock-image", name: "水果电商主图", url: "", mediaType: "image" },
  { id: "mock-ppt-1", name: "营销拓展PPT", url: "", mediaType: "document" },
  { id: "mock-video", name: "门店开业短视频", url: "", mediaType: "video" },
  { id: "mock-ppt-2", name: "品牌升级方案", url: "", mediaType: "document" }
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
const creationPromptDrafts = ref<Record<CreationMode, string>>({
  image: "",
  video: "",
  ppt: "",
  infographic: "",
  review: "",
  agent: ""
});
const creationError = ref("");
const generationSubmitting = ref(false);
const latestGenerationTask = ref<GenerationNotice | null>(null);
const pptSlideCount = ref(5);
const pptDynamic = ref(true);
const pptLanguage = ref<"zh" | "en">("zh");
const pptModel = ref("GPT-4o-mini");
const assetFilter = ref<AssetFilter>("all");
const assetSearch = ref("");
const roleInitialized = ref(false);
const mineView = ref<MineView>("overview");
const selectedMinePurchase = ref<MinePurchaseOption | null>(null);
const minePurchaseSubmitting = ref(false);
const mineLogoutConfirm = ref(false);

const profile = ref<MemberProfileResponse | null>(null);
const wallet = ref<RoleWalletResponse | null>(null);
const pointAccountResponse = ref<RoleWalletResponse | null>(null);
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
const isUserMineDetail = computed(() => activeRole.value === "user" && activeTab.value === "mine" && mineView.value !== "overview");
const displayedAssets = computed(() => recentAssets.value.length ? recentAssets.value.slice(0, 4) : mockAssets);
const filteredAssets = computed(() => displayedAssets.value.filter(asset => {
  const matchesType = assetFilter.value === "all"
    || (assetFilter.value === "favorite" && Boolean(asset.metadata?.favorite))
    || asset.mediaType === assetFilter.value;
  const matchesSearch = !assetSearch.value.trim() || asset.name.toLowerCase().includes(assetSearch.value.trim().toLowerCase());
  return matchesType && matchesSearch;
}));
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

const currentPageTitle = computed(() => {
  if (activeRole.value === "agent") return "代理工作台";
  if (activeRole.value === "operation") return "运营中心";
  if (activeTab.value === "create") return creationMode.value === "ppt" ? "PPT文档生成" : "创作中心";
  if (activeTab.value === "assets") return "我的作品";
  if (activeTab.value === "mine") return "我的";
  return "知启云 AI";
});

const currentPageSubtitle = computed(() => {
  if (activeRole.value === "agent") return "用户身份与代理身份可切换";
  if (activeRole.value === "operation") return "区域经营、代理与订单";
  if (activeTab.value === "create") return creationMode.value === "ppt" ? "Gamma式输入，移动端轻量化" : "轻易设计 + 多模型工作流";
  if (activeTab.value === "assets") return "统一查看图片、视频、PPT";
  if (activeTab.value === "mine") return "账户、钱包、订单与身份";
  return "用 AI 完成创作和经营增长";
});

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
  mineView.value = "overview";
  selectedMinePurchase.value = null;
  mineLogoutConfirm.value = false;
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
  token.value = authStorage.getToken() || uni.getStorageSync("token") || "";
  const legacyAuth = uni.getStorageSync("xianzhiMiniProgramAuth") as AuthResponse | "";
  const storedAuth = authStorage.getAuth() || legacyAuth || null;
  auth.value = storedAuth;
  if (token.value) setAuthToken(token.value);
  if (storedAuth) authStorage.setAuth(storedAuth);
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

function cycleRole() {
  const index = availableRoles.value.findIndex(role => role.id === activeRole.value);
  const next = availableRoles.value[(index + 1) % availableRoles.value.length];
  if (next) switchRole(next.id);
}

function cyclePptSlides() {
  const counts = [5, 8, 10, 15, 20];
  const index = counts.indexOf(pptSlideCount.value);
  pptSlideCount.value = counts[(index + 1) % counts.length] || 5;
}

function togglePptLanguage() {
  pptLanguage.value = pptLanguage.value === "zh" ? "en" : "zh";
}

function cyclePptModel() {
  const models = ["GPT-4o-mini", "Kimi K2.6", "DeepSeek V3"];
  const index = models.indexOf(pptModel.value);
  pptModel.value = models[(index + 1) % models.length] || models[0];
}

function selectUserTab(tab: TabId) {
  activeRole.value = "user";
  activeTab.value = tab;
  if (tab !== "mine") mineView.value = "overview";
}

function selectTab(tab: TabId) {
  activeTab.value = tab;
  if (tab !== "mine") mineView.value = "overview";
}

async function openMineView(view: MineView) {
  mineView.value = view;
  selectedMinePurchase.value = null;
  mineLogoutConfirm.value = false;
  if (view === "invite-promotion" && isAgentActive.value && !channelCenter.value) {
    await loadRoleData("agent");
  }
}

function selectMinePurchase(purchase: MinePurchaseOption) {
  selectedMinePurchase.value = purchase;
  mineLogoutConfirm.value = false;
}

async function confirmMinePurchase() {
  const purchase = selectedMinePurchase.value;
  if (!purchase || minePurchaseSubmitting.value) return;
  minePurchaseSubmitting.value = true;
  try {
    const created = purchase.kind === "agent"
      ? await createAgentOrder()
      : await createRechargeOrder({ id: purchase.id, amountCents: purchase.amountCents, points: purchase.points });
    if (created) selectedMinePurchase.value = null;
  } finally {
    minePurchaseSubmitting.value = false;
  }
}

function showInvoiceNotice() {
  uni.showToast({ title: "电子发票申请入口已预留", icon: "none" });
}

function showUsageExportNotice() {
  uni.showToast({ title: "消耗明细导出接口已预留", icon: "none" });
}

function showPosterNotice() {
  uni.showToast({ title: isAgentActive.value ? "推广海报生成功能接入中" : "请先升级代理商", icon: "none" });
}

function openCreation(mode: CreationMode) {
  selectCreationMode(mode);
  selectUserTab("create");
}

function selectCreationMode(mode: CreationMode) {
  if (mode === creationMode.value) return;
  creationPromptDrafts.value[creationMode.value] = creationPrompt.value;
  creationMode.value = mode;
  creationPrompt.value = creationPromptDrafts.value[mode] || "";
  creationError.value = "";
}

function returnToCreationHub() {
  creationPromptDrafts.value.ppt = creationPrompt.value;
  selectCreationMode("image");
  activeTab.value = "create";
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
  profile.value = await businessSdk.roleWorkbench.memberProfile();
}

async function loadWallet() {
  const [walletResult, pointsResult] = await Promise.allSettled([
    businessSdk.roleWorkbench.wallet(),
    businessSdk.roleWorkbench.pointsAccount()
  ]);
  if (walletResult.status === "fulfilled") wallet.value = walletResult.value;
  if (pointsResult.status === "fulfilled") pointAccountResponse.value = pointsResult.value;
}

async function loadAssets(showLoading = true) {
  if (!token.value) return;
  if (showLoading) assetsLoading.value = true;
  assetsError.value = "";
  try {
    recentAssets.value = await businessSdk.roleWorkbench.recentAssets(8);
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
      channelCenter.value = await businessSdk.roleWorkbench.channelCenter();
    } catch (error) {
      channelCenter.value = null;
    }
  }
  if (role === "operation" && isOperationActive.value) {
    const [profileResult, agentsResult, ordersResult, commissionsResult] = await Promise.allSettled([
      businessSdk.roleWorkbench.operationProfile(),
      businessSdk.roleWorkbench.operationAgents(),
      businessSdk.roleWorkbench.operationOrders(),
      businessSdk.roleWorkbench.operationCommissions()
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
    return true;
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建订单失败", icon: "none" });
    return false;
  }
}

async function createAgentOrder() {
  return createOrder("/api/v1/agent/join-order", "plan_agent_join_996", 99600, "代理商订单已创建");
}

async function createMemberPackageOrder() {
  return createOrder("/api/v1/orders/create", "plan_ai_creator_996", 99600, "会员订单已创建");
}

async function createOperationOrder() {
  return createOrder("/api/v1/operation-center/join-order", "plan_operation_center_5000", 500000, "运营中心订单已创建");
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
    return true;
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建充值订单失败", icon: "none" });
    return false;
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

function handleGenerateTap() {
  if (generationSubmitting.value) return;
  const prompt = String(creationPrompt.value || "").trim();
  creationError.value = "";
  if (!prompt) {
    creationError.value = creationMode.value === "ppt" ? "请先输入演示文稿主题" : "请先输入创作需求";
    uni.showToast({ title: creationError.value, icon: "none" });
    return;
  }
  if (!(["image", "video", "ppt"] as CreationMode[]).includes(creationMode.value)) {
    creationError.value = `${activeCreationName.value}暂未开放小程序生成`;
    uni.showToast({ title: creationError.value, icon: "none" });
    return;
  }

  void submitCreation(prompt);
}

async function submitCreation(prompt: string) {
  generationSubmitting.value = true;
  latestGenerationTask.value = {
    id: "正在创建",
    title: `${activeCreationName.value}任务提交中`,
    status: "提交中",
    tone: "pending"
  };
  try {
    let taskId = "";
    let taskStatus = "PENDING";
    if (creationMode.value === "ppt") {
      const result = await api<{ taskId?: string; id?: string; status?: string }>("/api/v1/ppt/generate", {
        method: "POST",
        body: JSON.stringify({
          prompt,
          slideCount: pptSlideCount.value,
          language: pptLanguage.value,
          tone: pptDynamic.value ? "dynamic" : "concise",
          theme: "business",
          autoThemeEnabled: true,
          enableWebSearch: false,
          textModel: pptModel.value.toLowerCase(),
          imageSource: "ai",
          imageModel: "gpt-image-2"
        })
      });
      taskId = String(result.taskId || result.id || "ppt-task");
      taskStatus = String(result.status || "PENDING").toUpperCase();
    } else {
      const mode: "image" | "video" = creationMode.value === "video" ? "video" : "image";
      const result = await businessSdk.generation.createTask({
        mode,
        prompt,
        model: activeCreationModel.value,
        style: mode === "video" ? "cinematic" : "commercial",
        size: mode === "video" ? "16:9" : "1024x1024",
        quality: mode === "video" ? "720p" : "standard",
        count: 1,
        referenceImages: []
      });
      taskId = String(result.id || "generation-task");
      taskStatus = String(result.status || "PENDING").toUpperCase();
    }

    latestGenerationTask.value = {
      id: taskId,
      title: `${activeCreationName.value}任务已创建`,
      status: taskStatus,
      tone: ["FAILED", "ERROR"].includes(taskStatus) ? "danger" : ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(taskStatus) ? "success" : "pending"
    };
    uni.showToast({ title: "生成任务已创建", icon: "success" });
    void pollGenerationTask(taskId, creationMode.value);
  } catch (error) {
    const message = error instanceof Error ? error.message : "生成任务创建失败";
    creationError.value = message;
    latestGenerationTask.value = { id: "-", title: "任务创建失败", status: message, tone: "danger" };
    uni.showToast({ title: "生成失败，请重试", icon: "none" });
  } finally {
    generationSubmitting.value = false;
  }
}

async function pollGenerationTask(taskId: string, mode: CreationMode) {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    await new Promise(resolve => setTimeout(resolve, 2500));
    try {
      if (mode === "ppt") {
        const task = await api<AnyRecord>(`/api/v1/ppt/tasks/${encodeURIComponent(taskId)}`);
        const status = rowStatus(task).toUpperCase();
        latestGenerationTask.value = {
          id: taskId,
          title: status === "SUCCESS" ? "PPT 文档生成成功" : "PPT 文档生成中",
          status,
          tone: status === "FAILED" ? "danger" : status === "SUCCESS" ? "success" : "pending"
        };
        if (["SUCCESS", "FAILED"].includes(status)) {
          if (status === "SUCCESS") await loadAssets(false);
          return;
        }
      } else {
        const tasks = await businessSdk.generation.listTasks();
        const task = tasks.find(item => item.id === taskId);
        if (!task) continue;
        const status = String(task.status || "PENDING").toUpperCase();
        latestGenerationTask.value = {
          id: taskId,
          title: ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status) ? `${activeCreationName.value}生成成功` : `${activeCreationName.value}生成中`,
          status,
          tone: ["FAILED", "ERROR"].includes(status) ? "danger" : ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status) ? "success" : "pending"
        };
        if (["FAILED", "ERROR", "SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)) {
          if (!["FAILED", "ERROR"].includes(status)) await loadAssets(false);
          return;
        }
      }
    } catch {
      return;
    }
  }
}

function showComingSoon(moduleName: string) {
  uni.showToast({ title: `${moduleName}入口已就绪，真实生成任务沿用桌面端接口接入`, icon: "none" });
}

function logout() {
  mineLogoutConfirm.value = false;
  authStorage.clear();
  uni.removeStorageSync("xianzhiMiniProgramAuth");
  uni.reLaunch({ url: "/pages/WechatLoginPage" });
}

onMounted(() => {
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramGenerate = handleGenerateTap;
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramBackToCreation = returnToCreationHub;
  void refreshAll();
});

onBackPress(() => {
  if (activeRole.value === "user" && activeTab.value === "mine") {
    if (selectedMinePurchase.value) {
      selectedMinePurchase.value = null;
      return true;
    }
    if (mineLogoutConfirm.value) {
      mineLogoutConfirm.value = false;
      return true;
    }
    if (mineView.value !== "overview") {
      mineView.value = "overview";
      return true;
    }
  }
  if (activeRole.value === "user" && activeTab.value === "create" && creationMode.value === "ppt") {
    returnToCreationHub();
    return true;
  }
  return false;
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

.home-command-card {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  padding: 20px;
  border: 1px solid #dfe4ff;
  border-radius: 18px;
  background:
    radial-gradient(circle at 92% 12%, rgba(255, 119, 27, 0.18), transparent 36%),
    linear-gradient(145deg, #ffffff 0%, #f1f3ff 100%);
  box-shadow: 0 14px 34px rgba(90, 77, 178, 0.1);
}

.home-command-copy {
  min-width: 0;
  flex: 1;
}

.home-command-kicker,
.home-command-title,
.home-command-desc,
.home-capability-icon,
.home-capability-name,
.home-capability-cost {
  display: block;
}

.home-command-kicker {
  color: #5a4db2;
  font-size: 12px;
  font-weight: 800;
}

.home-command-title {
  margin-top: 7px;
  color: #111827;
  font-size: 23px;
  font-weight: 900;
  line-height: 1.2;
}

.home-command-desc {
  margin-top: 7px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.home-command-button {
  width: 88px;
  height: 42px;
  margin: 0;
  flex: 0 0 88px;
  border-radius: 13px;
  background: #ff771b;
  color: #ffffff;
  font-size: 13px;
  font-weight: 800;
  box-shadow: 0 10px 22px rgba(255, 119, 27, 0.22);
}

.home-capability-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.home-capability-card {
  min-width: 0;
  min-height: 105px;
  margin: 0;
  padding: 13px 10px;
  border: 1px solid #e5e7eb;
  border-radius: 16px;
  background: #ffffff;
  text-align: left;
}

.home-capability-icon {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: #eef1ff;
  color: #5a4db2;
  font-size: 14px;
  font-weight: 900;
}

.home-capability-name {
  margin-top: 11px;
  overflow: hidden;
  color: #111827;
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home-capability-cost {
  margin-top: 4px;
  overflow: hidden;
  color: #94a3b8;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.wallet-card {
  color: #ffffff;
  background: #111827;
  border-color: #111827;
}

.wallet-card.agent {
  background: #0f766e;
  border-color: #0f766e;
}

.profile-wallet-card {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  background: linear-gradient(135deg, #17152b 0%, #5a4db2 100%);
  border-color: transparent;
  box-shadow: 0 16px 36px rgba(90, 77, 178, 0.2);
}

.wallet-unit {
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.82);
  font-size: 11px;
  font-weight: 700;
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

.danger-label {
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

/* V3.1 mobile workbench */
.mini-workbench {
  padding: 8px 15px calc(110px + env(safe-area-inset-bottom));
  background: #f8faff;
}

.native-safe-note {
  height: env(safe-area-inset-top);
  min-height: 4px;
}

.business-header {
  display: flex;
  height: 46px;
  margin-top: 5px;
  align-items: center;
  gap: 10px;
}

.business-logo {
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  border: 1px solid #dde5ff;
  border-radius: 8px;
  background: #ffffff;
}

.business-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.business-title {
  color: #0f172a;
  font-size: 17px;
  font-weight: 800;
  line-height: 24px;
}

.business-subtitle {
  overflow: hidden;
  color: #697386;
  font-size: 10px;
  line-height: 15px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.role-badge {
  width: auto;
  min-width: 70px;
  height: 26px;
  margin: 0;
  padding: 0 10px;
  border: 1px solid #c9d2ff;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 11px;
  line-height: 24px;
}

.role-badge.switchable {
  padding-right: 8px;
  border-color: #bfcaff;
  box-shadow: 0 4px 12px rgba(91, 85, 214, 0.1);
}

.role-switcher {
  display: none;
}

.role-content {
  margin-top: 16px;
}

.section-stack {
  gap: 14px;
}

.mini-workbench button {
  letter-spacing: 0;
}

.v31-kicker,
.v31-hero-title,
.v31-hero-copy,
.v31-metric-value,
.v31-metric-label,
.v31-section-title,
.v31-tool-name,
.v31-tool-desc,
.v31-inspiration-title,
.v31-profile-name,
.v31-profile-meta,
.v31-work-title,
.v31-works-note,
.v31-ppt-title,
.v31-draft-title,
.v31-draft-copy,
.v31-prompt-title,
.v31-workflow-title,
.v31-workflow-copy {
  display: block;
}

.v31-home-hero {
  min-height: 156px;
  padding: 17px;
  box-sizing: border-box;
  border-radius: 12px;
  color: #ffffff;
  background: #15192d;
  box-shadow: 0 14px 30px rgba(23, 28, 56, 0.16);
}

.v31-kicker {
  color: #aeb8ff;
  font-size: 11px;
  font-weight: 700;
}

.v31-hero-title {
  margin-top: 6px;
  font-size: 19px;
  font-weight: 900;
  line-height: 28px;
}

.v31-hero-copy {
  margin-top: 1px;
  color: #cdd5f5;
  font-size: 12px;
}

.v31-hero-row {
  display: grid;
  grid-template-columns: 104px 88px minmax(90px, 1fr);
  gap: 10px;
  margin-top: 11px;
  align-items: center;
}

.v31-mini-metric {
  height: 58px;
  padding: 7px 9px;
  box-sizing: border-box;
  border-radius: 8px;
}

.v31-mini-metric.purple { background: #f4f3ff; }
.v31-mini-metric.orange { background: #fff7ed; }
.v31-mini-metric.purple .v31-metric-value { color: #5b55d6; }
.v31-mini-metric.orange .v31-metric-value { color: #ff6b1a; }

.v31-metric-value {
  font-size: 16px;
  font-weight: 900;
  line-height: 21px;
}

.v31-metric-label {
  margin-top: 2px;
  color: #667085;
  font-size: 10px;
}

.v31-orange-button,
.v31-generate-button {
  display: grid;
  height: 38px;
  margin: 0;
  padding: 0 16px;
  place-items: center;
  border-radius: 10px;
  color: #ffffff;
  background: #ff7a1a;
  font-size: 13px;
  font-weight: 800;
}

.v31-section-title {
  color: #111827;
  font-size: 15px;
  font-weight: 900;
  line-height: 20px;
}

.v31-section-title.inside { margin-bottom: 12px; }

.v31-tool-grid,
.v31-mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  padding: 11px;
  border: 1px solid #e4e9f7;
  border-radius: 12px;
  background: #ffffff;
}

.v31-tool-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.v31-tool-card,
.v31-mode-card {
  display: flex;
  min-width: 0;
  height: 64px;
  margin: 0;
  padding: 8px;
  align-items: center;
  gap: 8px;
  border: 1px solid #e5eaf6;
  border-radius: 8px;
  background: #ffffff;
  text-align: left;
  box-shadow: 0 3px 10px rgba(23, 28, 56, 0.025);
}

.v31-tool-card {
  height: 58px;
  padding: 6px;
  gap: 6px;
}

.v31-mode-card.active { border-color: #c9d2ff; background: #f8f8ff; }

.v31-tool-icon,
.v31-menu-icon {
  display: inline-flex;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: center;
  border: 1px solid #d8d5ff;
  border-radius: 8px;
  color: #5b55d6;
  background: #f4f3ff;
  font-size: 12px;
  font-weight: 900;
}

.v31-tool-icon.orange, .v31-menu-icon.orange { color: #ff6b1a; border-color: #ffe2cc; background: #fff7ed; }
.v31-tool-icon.green, .v31-menu-icon.green { color: #079455; border-color: #cbf5df; background: #ecfdf5; }
.v31-tool-icon.blue { color: #2563eb; border-color: #cfe1ff; background: #eff6ff; }

.v31-tool-copy {
  min-width: 0;
  flex: 1;
}

.v31-tool-name {
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-tool-desc {
  display: -webkit-box;
  margin-top: 3px;
  overflow: hidden;
  color: #697386;
  font-size: 9px;
  line-height: 13px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.v31-inspiration-grid,
.v31-work-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 11px;
  border: 1px solid #e4e9f7;
  border-radius: 12px;
  background: #ffffff;
}

.v31-inspiration-card,
.v31-work-card {
  min-width: 0;
  margin: 0;
  padding: 9px;
  border: 1px solid #e5eaf6;
  border-radius: 8px;
  background: #ffffff;
  text-align: left;
  box-shadow: 0 4px 12px rgba(23, 28, 56, 0.035);
}

.v31-preview,
.v31-work-preview {
  display: flex;
  width: 100%;
  height: 86px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 25px;
  font-weight: 900;
}

.v31-preview.orange, .v31-work-preview.orange { color: #ff6b1a; background: #fff2e8; }
.v31-work-preview.green { color: #079455; background: #eafbf2; }

.v31-inspiration-title,
.v31-work-title {
  margin-top: 9px;
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-card-footer {
  display: flex;
  margin-top: 10px;
  align-items: center;
  justify-content: space-between;
}

.v31-chip {
  display: inline-flex;
  min-height: 24px;
  padding: 0 12px;
  align-items: center;
  justify-content: center;
  border: 1px solid #c9d2ff;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 11px;
}

.v31-chip.orange { color: #ff6b1a; border-color: #ffd0b3; background: #fff2e8; }
.v31-chip.green { color: #168a54; border-color: #bdeecd; background: #eafbf2; }
.v31-link { color: #5b55d6; font-size: 10px; }

.v31-prompt-panel,
.v31-ppt-panel,
.v31-draft-card,
.v31-filter-card,
.v31-works-card,
.v31-wallet-panel,
.v31-menu-panel {
  padding: 15px;
  border: 1px solid #dde5ff;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 8px 20px rgba(23, 28, 56, 0.06);
}

.v31-subpage-nav {
  display: flex;
  min-height: 48px;
  align-items: center;
  gap: 10px;
}

.v31-back-button {
  display: grid;
  width: 40px;
  min-width: 40px;
  height: 40px;
  margin: 0;
  padding: 0;
  place-items: center;
  border: 1px solid #dfe5f2;
  border-radius: 10px;
  color: #344054;
  background: #ffffff;
  font-size: 27px;
  line-height: 1;
}

.v31-back-button::after { display: none; }
.v31-subpage-title { display: block; color: #111827; font-size: 15px; font-weight: 900; }
.v31-subpage-copy { display: block; margin-top: 2px; color: #697386; font-size: 10px; }

.v31-prompt-title,
.v31-ppt-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 900;
  line-height: 26px;
}

.v31-one-line-input,
.v31-ppt-input {
  width: 100%;
  height: 78px;
  margin-top: 10px;
  padding: 12px;
  box-sizing: border-box;
  border: 1px solid #e3e8f2;
  border-radius: 8px;
  color: #111827;
  background: #f8fafc;
  font-size: 12px;
  line-height: 18px;
}

.v31-ppt-input { height: 96px; }

.v31-prompt-actions,
.v31-ppt-options,
.v31-draft-actions,
.v31-recharge-row,
.v31-batch-actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
  align-items: center;
  flex-wrap: wrap;
}

.v31-prompt-actions .v31-generate-button {
  min-width: 90px;
  height: 30px;
  margin-left: auto;
}

.v31-generate-button.disabled,
.v31-ppt-submit.disabled {
  opacity: 0.58;
  pointer-events: none;
}

.v31-action-pressed { opacity: 0.76; transform: scale(0.98); }

.v31-generation-error {
  display: block;
  margin-top: 9px;
  color: #dc2626;
  font-size: 11px;
  line-height: 16px;
}

.v31-generation-state {
  display: flex;
  min-height: 62px;
  padding: 12px 14px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid #c9d2ff;
  border-radius: 12px;
  background: #f4f3ff;
}

.v31-generation-state.success { border-color: #bdeecd; background: #ecfdf5; }
.v31-generation-state.danger { border-color: #fecaca; background: #fff1f2; }
.v31-generation-title { display: block; color: #111827; font-size: 13px; font-weight: 800; }
.v31-generation-meta { display: block; max-width: 230px; margin-top: 4px; overflow: hidden; color: #697386; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.v31-generation-state button { width: auto; min-width: 72px; height: 32px; margin: 0; padding: 0 10px; border-radius: 8px; color: #5b55d6; background: #ffffff; font-size: 11px; }

.v31-workflow-card,
.v31-profile-hero {
  padding: 15px;
  border-radius: 12px;
  color: #ffffff;
  background: #15192d;
}

.v31-workflow-title { font-size: 15px; font-weight: 900; }
.v31-workflow-copy { margin-top: 4px; color: #cdd5f5; font-size: 12px; }
.v31-workflow-tags { display: flex; gap: 10px; margin-top: 9px; }
.v31-workflow-tags text { min-width: 76px; padding: 5px 8px; border-radius: 8px; background: #111827; font-size: 10px; text-align: center; }

.v31-ppt-options button,
.v31-example-grid button,
.v31-draft-actions button,
.v31-filter-card button,
.v31-recharge-row button,
.v31-batch-actions button {
  width: auto;
  min-width: 92px;
  height: 32px;
  margin: 0;
  padding: 0 12px;
  border: 1px solid #e3e8f2;
  border-radius: 8px;
  color: #475467;
  background: #f5f7fb;
  font-size: 11px;
  line-height: 30px;
}

.v31-ppt-options button.active,
.v31-filter-card button.active,
.v31-batch-actions button.active { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }
.v31-ppt-options .v31-ppt-submit { display: grid; min-width: 50px; height: 50px; margin-left: auto; place-items: center; border: 0; border-radius: 14px; color: #ffffff; background: #ffb47d; font-size: 24px; line-height: 1; }

.v31-example-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 18px;
  padding: 13px 11px;
  border: 1px solid #dde5ff;
  border-radius: 12px;
  background: #ffffff;
}

.v31-example-grid button { width: 100%; color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }
.v31-draft-title { color: #111827; font-size: 15px; font-weight: 900; }
.v31-draft-copy { margin-top: 5px; color: #697386; font-size: 11px; line-height: 18px; }
.v31-draft-actions button { color: #ff6b1a; border-color: #ffd0b3; background: #fff7ed; }
.v31-draft-actions button.dark { color: #ffffff; border-color: #15192d; background: #15192d; }

.v31-filter-card { padding: 12px; }
.v31-filter-row { display: flex; gap: 8px; overflow-x: auto; }
.v31-filter-card button { min-width: 54px; flex: 0 0 auto; }
.v31-search-strip { width: 100%; height: 34px; margin-top: 10px; padding: 0 12px; box-sizing: border-box; border: 1px solid #e5eaf6; border-radius: 8px; color: #111827; background: #f8fafc; font-size: 11px; }
.v31-works-card { padding: 11px; }
.v31-works-card .v31-work-grid { padding: 0; border: 0; }
.v31-works-note { margin-top: 14px; color: #697386; font-size: 11px; }
.v31-empty-state { display: flex; min-height: 150px; flex-direction: column; align-items: center; justify-content: center; gap: 12px; color: #77829a; font-size: 12px; }
.v31-empty-state button { width: auto; height: 32px; margin: 0; padding: 0 16px; border-radius: 8px; color: #5b55d6; background: #eef2ff; font-size: 11px; }

.v31-profile-hero {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  gap: 12px;
  align-items: center;
}

.v31-avatar { width: 54px; height: 54px; border-radius: 14px; background: #eef2ff; }
.v31-profile-name { font-size: 16px; font-weight: 900; }
.v31-profile-meta { margin-top: 4px; color: #cdd5f5; font-size: 11px; }
.v31-upgrade-button { grid-column: 2; width: 96px; height: 28px; margin: 0; color: #ff6b1a; border-radius: 8px; background: #fff2e8; font-size: 11px; line-height: 28px; }
.v31-wallet-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.v31-wallet-metric { padding: 12px; border: 1px solid #d8d5ff; border-radius: 8px; background: #f4f3ff; }
.v31-wallet-metric.orange { border-color: #ffe2cc; background: #fff7ed; }
.v31-wallet-metric text { display: block; color: #5b55d6; font-size: 16px; font-weight: 900; }
.v31-wallet-metric.orange text { color: #ff6b1a; }
.v31-wallet-metric text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }
.v31-recharge-row button { min-width: 0; flex: 1; }
.v31-recharge-row button.primary { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }
.v31-recharge-row button.orange { color: #ff6b1a; border-color: #ffd0b3; background: #fff7ed; }

.v31-menu-panel { display: flex; flex-direction: column; gap: 10px; padding: 11px; }
.v31-menu-panel > button { display: flex; width: 100%; min-height: 54px; margin: 0; padding: 10px; align-items: center; gap: 10px; border: 1px solid #e5eaf6; border-radius: 8px; background: #ffffff; text-align: left; }
.v31-menu-panel > button view { min-width: 0; flex: 1; }
.v31-menu-panel > button view text { display: block; color: #111827; font-size: 12px; font-weight: 800; }
.v31-menu-panel > button view text + text { margin-top: 3px; color: #697386; font-size: 9px; font-weight: 500; }
.v31-menu-panel > button.danger view text { color: #dc2626; }

.bottom-tabs {
  left: 10px;
  right: 10px;
  bottom: max(10px, env(safe-area-inset-bottom));
  gap: 6px;
  padding: 7px;
  border-color: #e4e9f7;
  border-radius: 16px;
  box-shadow: 0 10px 28px rgba(23, 28, 56, 0.12);
  backdrop-filter: blur(14px);
}

.tab-button {
  display: flex;
  width: 100%;
  min-width: 0;
  height: 56px;
  min-height: 56px;
  margin: 0;
  padding: 6px 2px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  border-radius: 10px;
  color: #697386;
  line-height: 1.15;
}

.tab-button::after { display: none; }

.tab-button.active {
  color: #5b55d6;
  background: #f1f0ff;
}

.tab-button .tab-icon {
  margin-bottom: 3px;
  font-size: 17px;
  line-height: 1;
}

@media (max-width: 340px) {
  .v31-tool-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .v31-hero-row { grid-template-columns: 1fr 1fr; }
  .v31-orange-button { grid-column: 1 / -1; }
}
</style>
