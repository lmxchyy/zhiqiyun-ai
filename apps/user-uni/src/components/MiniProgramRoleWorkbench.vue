<template>
  <view :class="['mini-workbench', { 'user-v531-shell': isV531PrimaryPage }]" :style="miniWorkbenchSafeAreaStyle">
    <view class="native-safe-note"></view>

    <view v-if="activeRole !== 'user' && !isUserMineDetail" class="business-header">
      <image class="business-logo" :src="loginLogo" mode="aspectFit" />
      <view class="business-copy">
        <text class="business-title">{{ currentPageTitle }}</text>
        <text class="business-subtitle">{{ currentPageSubtitle }}</text>
      </view>
      <view v-if="availableRoles.length === 1" class="role-badge">{{ roleLabel }}</view>
    </view>

    <view class="role-switcher" v-if="activeRole !== 'user' && availableRoles.length > 1 && !isUserMineDetail">
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

    <view v-if="pageLoading && activeRole !== 'user'" class="state-card">
      <text>正在同步小程序工作台...</text>
    </view>
    <view v-if="pageError && !(activeRole === 'user' && !isGuest && userStore.currentRole !== 'USER')" class="state-card error runtime-error-banner">
      <text>{{ pageError }}</text>
      <button type="button" class="small-button" @click="refreshAll">重新加载</button>
    </view>

    <view v-if="activeRole === 'user' && !isGuest && userStore.currentRole !== 'USER'" class="state-card user-role-switch-state">
      <text>{{ pageError || '正在切换到普通用户视图...' }}</text>
      <button v-if="pageError" type="button" class="small-button" @click="refreshAll">重新加载</button>
    </view>

    <view v-else class="role-content">
      <template v-if="activeRole === 'user'">
        <view v-if="isGuest && (activeTab === 'home' || activeTab === 'create')" class="guest-browse-banner">
          <view class="guest-browse-copy">
            <text class="guest-browse-title">欢迎先体验知启云 AI</text>
            <text class="guest-browse-detail">无需登录即可浏览功能；生成、上传或保存作品时再登录。</text>
          </view>
          <button type="button" class="guest-browse-button" @click="requestLogin('登录后可开始生成并保存作品')">登录 / 注册</button>
        </view>
        <AiGeneratedContentNotice v-if="activeTab === 'home' || activeTab === 'create'" />
        <V531HomePage
          v-if="activeTab === 'home'"
          :display-name="displayName"
          :avatar-url="userAvatarUrl"
          :avatar-fallback="pageConfigStore.slot('profile', 'profile.avatar')?.imageUrl || pageConfigStore.slot('profile', 'profile.avatar')?.fallbackUrl"
          :point-balance="pointBalance"
          :plan-name="planName"
          :subscription-expires-at="profileSubscriptionExpiresAt"
          :today-calls="todayCallCount"
          :assets="recentAssets"
          :tasks="generationTasks"
          :allowed-creation-modes="allowedCreationModes"
          @tab="selectUserTab"
          @open-mode="openCreation"
          @open-asset="openAssetDetail"
          @notice="showNotifications"
          @profile="selectUserTab('mine')"
        />
        <V531StudioPage
          v-else-if="activeTab === 'create' && !isCreationDetail"
          :point-balance="pointBalance"
          :plan-name="planName"
          :allowed-creation-modes="allowedCreationModes"
          @open-mode="openCreation"
          @recharge="openFeaturePage(miniProgramFeaturePages.userRechargePlans)"
        />
        <AssetCenterPage
          v-else-if="activeTab === 'assets'"
          :is-guest="isGuest"
          @create="selectUserTab('create')"
          @login="requestLogin('\u767b\u5f55\u540e\u53ef\u67e5\u770b\u5e76\u7ba1\u7406\u6211\u7684\u4f5c\u54c1')"
        />
        <V531ProfilePage
          v-else-if="activeTab === 'mine' && mineView === 'overview'"
          :display-name="displayName"
          :is-guest="isGuest"
          :user-id="displayUserId"
          :roles="userStore.roles"
          :current-role="userStore.currentRole"
          :permissions="userStore.permissions"
          :company-name="profileCompanyName"
          :plan-name="planName"
          :subscription-expires-at="profileSubscriptionExpiresAt"
          :point-balance="pointBalance"
          :monthly-point-cost="monthlyPointCost"
          :monthly-granted-points="monthlyGrantedPoints"
          :creation-count="generationTasks.length"
          :image-count="imageAssetCount"
          :video-count="videoAssetCount"
          :ppt-count="pptAssetCount"
          :avatar-url="userAvatarUrl"
          :avatar-fallback="pageConfigStore.slot('profile', 'profile.avatar')?.imageUrl || pageConfigStore.slot('profile', 'profile.avatar')?.fallbackUrl"
          @upgrade="openFeaturePage(miniProgramFeaturePages.userAgentDetail)"
          @edit="openFeaturePage(miniProgramFeaturePages.userProfileEdit)"
          @recharge="openFeaturePage(miniProgramFeaturePages.userRechargePlans)"
          @role-change="handleV531RoleChange"
          @service="handleV531ProfileService"
          @benefit="handleV531Benefit"
        />
        <template v-else>
        <view v-if="legacyActiveTab === 'home'" class="section-stack">
          <view class="v31-home-hero">
            <RemoteCover class="v31-hero-cover" page-code="home" slot-key="home.hero.background" alt="知启云 AI 首页主视觉" width="100%" height="100%" :lazy-load="false" />
            <text class="v31-kicker">一句话开始</text>
            <text class="v31-hero-title">用 AI 完成设计、视频与 PPT</text>
            <text class="v31-hero-copy">从创作到判断，一站式解决。</text>
            <view class="v31-hero-row">
              <button type="button" class="v31-mini-metric purple" @click="selectUserTab('wallet')">
                <text class="v31-metric-value">{{ formatNumber(pointBalance) }}</text>
                <text class="v31-metric-label">点数余额</text>
              </button>
              <button type="button" class="v31-mini-metric orange" @click="selectUserTab('assets')">
                <text class="v31-metric-value">{{ recentAssets.length }}</text>
                <text class="v31-metric-label">近期作品</text>
              </button>
              <button type="button" class="v31-orange-button" @click="selectUserTab('create')">去创作</button>
            </view>
          </view>

          <text class="v31-section-title">常用工具</text>
          <view class="v31-tool-grid">
            <button v-for="module in creationModules" :key="`home-${module.id}`" class="v31-tool-card" @click="openCreation(module.id)">
              <RemoteCover class="v31-tool-cover" page-code="home" :slot-key="homeModuleSlot(module.id)" :alt="module.name" width="36px" height="36px" radius="10px" />
              <view class="v31-tool-copy">
                <text class="v31-tool-name">{{ module.homeName || module.name }}</text>
                <text class="v31-tool-desc">{{ module.description }}</text>
              </view>
            </button>
          </view>

          <text class="v31-section-title">灵感推荐</text>
          <view class="v31-inspiration-grid">
            <button class="v31-inspiration-card" @click="openCreation('image')">
              <RemoteCover class="v31-preview" page-code="home" slot-key="home.inspiration.ecommerce" alt="水果电商主图" width="100%" height="86px" radius="12px" />
              <text class="v31-inspiration-title">水果电商主图</text>
              <view class="v31-card-footer"><text class="v31-chip orange">图片</text><text class="v31-link">继续改</text></view>
            </button>
            <button class="v31-inspiration-card" @click="openCreation('ppt')">
              <RemoteCover class="v31-preview" page-code="home" slot-key="home.inspiration.ppt" alt="招商路演 PPT" width="100%" height="86px" radius="12px" />
              <text class="v31-inspiration-title">招商路演PPT</text>
              <view class="v31-card-footer"><text class="v31-chip purple">PPT</text><text class="v31-link">继续改</text></view>
            </button>
          </view>
        </view>

        <view v-else-if="legacyActiveTab === 'create'" class="section-stack">
          <view class="v31-subpage-nav">
            <button
              class="v31-back-button"
              aria-label="返回上一页"
              data-return-fallback="/pages/user/UserCreationPage"
              @click="returnToCreationHub"
            >‹</button>
            <view>
              <text class="v31-subpage-title">{{ creationDetailTitle }}</text>
              <text class="v31-subpage-copy">返回上一页不会丢失当前草稿</text>
            </view>
          </view>
          <RemoteCover v-if="creationMode !== 'agent'" class="v31-studio-banner" page-code="studio" slot-key="studio.banner" alt="AI 创作中心" width="100%" height="118px" :lazy-load="false" radius="16px" />
          <template v-if="creationMode === 'agent'">
            <KnowledgeMiniChat embedded @close="returnToCreationHub" />
          </template>
          <template v-else-if="creationMode === 'ppt'">
            <view class="v31-ppt-panel">
              <text class="v31-ppt-title">您今天想制作什么样的演示文稿？</text>
              <textarea v-model="creationPrompt" class="v31-ppt-input" maxlength="500" placeholder="请输入主题，例如：AI赋能企业营销增长方案" @input="creationError = ''" />
              <view class="v31-ppt-options">
                <button @click="cyclePptSlides">{{ pptSlideCount }}张幻灯片</button>
                <button :class="{ active: pptDynamic }" @click="pptDynamic = !pptDynamic">{{ pptDynamic ? "动态的" : "静态的" }}</button>
                <button class="active" @click="togglePptLanguage">{{ pptLanguage === "zh" ? "中文" : "英文" }}</button>
                <button @click="cyclePptModel">{{ pptModel }}</button>
                <view
                  :class="['v31-ppt-submit', { disabled: generationBusy }]"
                  role="button"
                  hover-class="v31-action-pressed"
                  @click.stop="guestAwareGenerateTap"
                >{{ generationBusy ? "…" : "→" }}</view>
              </view>
              <text v-if="creationError" class="v31-generation-error">{{ creationError }}</text>
            </view>
            <text class="v31-section-title">示例主题</text>
            <view class="v31-example-grid">
              <button v-for="topic in pptTopics" :key="topic" @click="creationPrompt = topic; creationError = ''">{{ topic }}</button>
            </view>
            <view v-if="latestGenerationTask" :class="['v31-generation-state', latestGenerationTask.tone]">
              <view class="v31-generation-summary">
                <view class="v31-generation-title-row"><text class="v31-generation-title">{{ latestGenerationTask.title }}</text><text v-if="generationNoticePending" class="v31-live-badge">实时</text></view>
                <text class="v31-generation-meta">任务 {{ latestGenerationTask.id }} · {{ generationStatusLabel }}</text>
                <view v-if="generationNoticePending" class="v31-generation-progress-track">
                  <view :class="['v31-generation-progress-value', { indeterminate: !generationHasProgress }]" :style="generationProgressStyle" />
                </view>
                <text v-if="generationNoticePending" class="v31-generation-feedback">{{ generationFeedbackText }}</text>
              </view>
              <button v-if="latestGenerationTask.tone === 'success'" @click="openLatestGenerationResult">{{ latestGenerationTask.resultId ? "查看结果" : "查看作品" }}</button>
              <button v-else-if="latestGenerationTask.tone === 'danger'" @click="handleGenerateTap">重新生成</button>
              <text v-else class="v31-generation-running">{{ generationButtonLabel }}</text>
            </view>
            <view class="v31-draft-card">
              <text class="v31-draft-title">未完成项目会保留在最近浏览</text>
              <text class="v31-draft-copy">选择文本内容、自定义主题后，即使返回首页，也能继续从草稿进入。</text>
              <view class="v31-workflow-tags"><text>自动保存草稿</text><text>生成大纲</text><text>主题预览</text></view>
              <button class="v31-ppt-editor-entry" type="button" @click="openPptEditor()">管理已有 PPT 与单页视觉</button>
            </view>
          </template>

          <template v-else>
            <view v-if="creationReferenceEnabled" class="v31-reference-panel">
              <view class="v31-reference-head">
                <view class="v31-reference-copy">
                  <view class="v31-reference-title-row">
                    <text class="v31-reference-title">参考图</text>
                    <text v-if="creationReferencePaths.length" class="v31-reference-mode">{{ creationReferenceModeLabel }}</text>
                  </view>
                  <text class="v31-reference-description">
                    {{ creationSourceLoading ? "正在载入原作品..." : creationSourceError || (creationReferencePaths.length ? "生成时会保留参考图的主体与视觉特征" : "添加参考图后将自动使用参考图生成") }}
                  </text>
                </view>
                <button
                  v-if="!creationSourceLoading && creationReferencePaths.length < 3"
                  class="v31-reference-add"
                  type="button"
                  :data-reference-remaining="3 - creationReferencePaths.length"
                  :disabled="creationReferenceSelecting"
                  @click="chooseCreationReferenceImages"
                >{{ creationReferenceSelecting ? "选择中..." : creationReferencePaths.length ? "添加" : "选择图片" }}</button>
              </view>

              <view v-if="creationSourceLoading" class="v31-reference-loading">
                <view class="v31-reference-loading-image"></view>
                <text>正在读取当前作品并设置为参考图</text>
              </view>
              <scroll-view v-else-if="creationReferencePaths.length" class="v31-reference-scroll" scroll-x :show-scrollbar="false">
                <view class="v31-reference-row">
                  <view v-for="(path, index) in creationReferencePaths" :key="`${path}-${index}`" class="v31-reference-item">
                    <image class="v31-reference-image" :src="path" mode="aspectFill" @click="previewCreationReference(index)" />
                    <text v-if="path === creationSourceReferenceUrl" class="v31-reference-source">当前作品</text>
                    <button class="v31-reference-remove" type="button" aria-label="移除参考图" @click.stop="removeCreationReference(index)">×</button>
                  </view>
                </view>
              </scroll-view>
              <button
                v-else
                class="v31-reference-empty"
                type="button"
                :data-reference-remaining="3"
                :disabled="creationReferenceSelecting"
                @click="chooseCreationReferenceImages"
              >
                <text class="v31-reference-empty-icon">＋</text>
                <view>
                  <text class="v31-reference-empty-title">{{ creationReferenceSelecting ? "正在打开图片..." : "添加参考图" }}</text>
                  <text class="v31-reference-empty-copy">支持最多 3 张图片</text>
                </view>
              </button>
            </view>

            <view class="v31-prompt-panel">
              <text class="v31-prompt-title">今天想做什么？</text>
              <textarea v-model="creationPrompt" class="v31-one-line-input" maxlength="500" placeholder="例如：生成一张水果店开业促销海报，橙色系，高级感" @input="creationError = ''" />
              <view class="v31-prompt-actions">
                <text class="v31-chip purple">自动匹配工具</text>
                <text class="v31-chip green">多模型对比</text>
                <view
                  :class="['v31-generate-button', { disabled: generationBusy }]"
                  role="button"
                  hover-class="v31-action-pressed"
                  @click.stop="guestAwareGenerateTap"
                ><text v-if="generationBusy" class="v31-button-spinner" /><text>{{ generationButtonLabel }}</text></view>
              </view>
              <text v-if="creationError" class="v31-generation-error">{{ creationError }}</text>
            </view>

            <view v-if="latestGenerationTask" :class="['v31-generation-state', latestGenerationTask.tone]">
              <image
                v-if="latestGenerationTask.resultUrl && ['image', 'infographic'].includes(latestGenerationTask.resultType || '')"
                class="v31-generation-result"
                :src="latestGenerationTask.resultUrl"
                mode="aspectFill"
                @click="previewLatestGenerationResult"
              />
              <view class="v31-generation-summary">
                <view class="v31-generation-title-row"><text class="v31-generation-title">{{ latestGenerationTask.title }}</text><text v-if="generationNoticePending" class="v31-live-badge">实时</text></view>
                <text class="v31-generation-meta">任务 {{ latestGenerationTask.id }} · {{ generationStatusLabel }}</text>
                <view v-if="generationNoticePending" class="v31-generation-progress-track">
                  <view :class="['v31-generation-progress-value', { indeterminate: !generationHasProgress }]" :style="generationProgressStyle" />
                </view>
                <text v-if="generationNoticePending" class="v31-generation-feedback">{{ generationFeedbackText }}</text>
              </view>
              <button v-if="latestGenerationTask.tone === 'success'" @click="openLatestGenerationResult">{{ latestGenerationTask.resultId ? "查看结果" : "查看作品" }}</button>
              <button v-else-if="latestGenerationTask.tone === 'danger'" @click="handleGenerateTap">重新生成</button>
              <text v-else class="v31-generation-running">{{ generationButtonLabel }}</text>
            </view>

            <text class="v31-section-title">选择创作能力</text>
            <view class="v31-mode-grid">
              <button v-for="module in creationModules" :key="module.id" :class="['v31-mode-card', { active: creationMode === module.id }]" @click="selectCreationMode(module.id)">
                <RemoteCover class="v31-tool-cover" page-code="studio" :slot-key="studioModuleSlot(module.id)" :alt="module.name" width="36px" height="36px" radius="10px" />
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

        <view v-else-if="legacyActiveTab === 'assets'" class="section-stack">
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
              <button v-for="asset in filteredAssets" :key="asset.id" class="v31-work-card" @click="openAssetDetail(asset)">
                <AppImage v-if="asset.thumbnailUrl" class="v31-work-preview" :src="asset.thumbnailUrl" :fallback="pageConfigStore.slot('assets', assetDefaultSlot(asset.mediaType))?.fallbackUrl" :alt="asset.name" width="100%" height="86px" radius="12px" />
                <RemoteCover v-else class="v31-work-preview" page-code="assets" :slot-key="assetDefaultSlot(asset.mediaType)" :alt="asset.name" width="100%" height="86px" radius="12px" />
                <text class="v31-work-title">{{ asset.name || asset.id }}</text>
                <view class="v31-card-footer"><text :class="['v31-chip', asset.mediaType === 'video' ? 'green' : asset.mediaType === 'image' ? 'orange' : 'purple']">{{ asset.mediaType === "video" ? "视频" : asset.mediaType === "image" ? "图片" : "PPT" }}</text><text class="v31-link">继续改</text></view>
              </button>
            </view>
            <view v-else class="v31-empty-state"><text>没有找到符合条件的作品</text><button @click="assetFilter = 'all'; assetSearch = ''">查看全部</button></view>
            <text class="v31-works-note">每个作品保留生成参数、消耗点数、导出记录。</text>
            <view class="v31-batch-actions"><button class="active" @click="selectUserTab('create')">继续创作</button></view>
          </view>
        </view>

        <view v-else-if="activeTab === 'wallet'" class="section-stack">
          <view class="v31-subpage-nav">
            <button
              class="v31-back-button"
              aria-label="返回上一页"
              data-return-fallback="/pages/user/UserHomePage"
              @click="returnToPreviousPage('/pages/user/UserHomePage')"
            >‹</button>
            <view>
              <text class="v31-subpage-title">钱包与点数</text>
              <text class="v31-subpage-copy">余额、充值与积分消耗记录</text>
            </view>
          </view>
          <view class="wallet-card">
            <text class="wallet-label">钱包余额</text>
            <text class="wallet-value">{{ formatNumber(pointBalance) }}</text>
            <text class="wallet-copy">冻结 {{ formatNumber(pointFrozen) }} 点 · 订单 {{ userOrders.length }} 笔</text>
          </view>
          <view class="v31-batch-actions"><button class="active" @click="openFeaturePage(miniProgramFeaturePages.userRechargePlans)">全部充值方案</button><button @click="openFeaturePage(miniProgramFeaturePages.userOrders)">我的订单</button></view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">点数充值</text>
              <text class="soft-tag">微信支付</text>
            </view>
            <view class="recharge-grid">
              <button
                v-for="pack in rechargePackages"
                :key="rowString(pack, 'id')"
                type="button"
                class="recharge-card"
                @click="openFeaturePage(miniProgramFeaturePages.userRechargePlans)"
              >
                <text class="recharge-points">{{ formatNumber(rowNumber(pack, 'grantPoints') || rowNumber(pack, 'points') || rowNumber(pack, 'tokenAmount')) }} 点</text>
                <text class="recharge-price">{{ formatCurrency(rowNumber(pack, 'priceCents') || rowNumber(pack, 'amountCents')) }}</text>
              </button>
            </view>
          </view>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">积分消耗</text>
              <text class="soft-tag">{{ tokenRecords.length || userTransactions.length }} 条</text>
            </view>
            <view v-if="walletRecords.length" class="list-stack">
              <view v-for="record in walletRecords.slice(0, 6)" :key="rowKey(record)" class="list-item" @click="openUsageRecordDetail(record)">
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
          :avatar-url="pageConfigStore.slot('profile', 'profile.default_avatar')?.imageUrl"
          :avatar-fallback="pageConfigStore.slot('profile', 'profile.default_avatar')?.fallbackUrl"
          :header-background="pageConfigStore.slot('profile', 'profile.header_background')?.imageUrl"
          :member-background="pageConfigStore.slot('profile', 'profile.member_background')?.imageUrl"
          :point-balance="pointBalance"
          :monthly-point-cost="monthlyPointCost"
          :roles="userStore.roles"
          :current-role="userStore.currentRole"
          :permissions="userStore.permissions"
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
      </template>

      <template v-else-if="activeRole === 'agent'">
        <view v-if="activeTab === 'overview'" class="section-stack">
          <view class="agent-v4-hero">
            <view class="agent-v4-hero-top"><view><text>本月预估分润</text><text>{{ formatCurrency(summaryNumber(channelSummary, "totalCommission")) }}</text></view><text>{{ agentLevelLabel }}</text></view>
            <text class="agent-v4-copy">登录后优先查看推广增长、客户、订单与结算结果。</text>
            <view class="agent-v4-metrics"><button @click="selectAgentTab('customers')"><text>{{ formatNumber(summaryNumber(channelSummary, "directCustomers")) }}</text><text>客户</text></button><button @click="openFeaturePage(miniProgramFeaturePages.agentTeam)"><text>{{ formatNumber(summaryNumber(channelSummary, "childAgents")) }}</text><text>团队</text></button><button @click="openFeaturePage(miniProgramFeaturePages.agentWithdrawals)"><text>{{ formatCurrency(summaryNumber(channelSummary, "availableToWithdraw")) }}</text><text>可提现</text></button></view>
          </view>

          <view class="agent-v4-entry-card">
            <view class="section-header compact"><text class="section-title">经营入口</text><text class="soft-tag">{{ agentName }}</text></view>
            <button @click="selectAgentTab('promotion')"><text class="agent-v4-icon green">推</text><view><text>推广中心</text><text>专属链接、小程序分享与邀请记录</text></view><text>{{ conversionRate }}% 转化</text></button>
            <button @click="selectAgentTab('customers')"><text class="agent-v4-icon purple">客</text><view><text>客户管理</text><text>绑定客户与客户订单</text></view><text>{{ channelCustomers.length }} 人</text></button>
            <button @click="selectAgentTab('commission')"><text class="agent-v4-icon green">润</text><view><text>分润中心</text><text>订单分润与提现记录</text></view><text>{{ formatCurrency(summaryNumber(channelSummary, "availableToWithdraw")) }}</text></button>
            <button @click="openFeaturePage(miniProgramFeaturePages.agentTeam)"><text class="agent-v4-icon orange">队</text><view><text>团队管理</text><text>直属代理与成员业绩</text></view><text>{{ formatNumber(summaryNumber(channelSummary, "childAgents")) }} 人</text></button>
          </view>
          <button class="agent-v4-cta" @click="selectAgentTab('promotion')">查看推广数据</button>
        </view>

        <view v-else-if="activeTab === 'promotion'" class="agent-promotion-embed">
          <PromotionCenterScreen
            embedded
            :show-header="false"
            :show-back="false"
            :active="activeTab === 'promotion'"
          />
        </view>

        <view v-else-if="activeTab === 'customers'" class="section-stack">
          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">拓展客户</text>
              <text class="soft-tag">{{ channelCustomers.length }} 人</text>
            </view>
            <view v-if="channelCustomers.length" class="list-stack">
              <view v-for="customer in channelCustomers" :key="rowKey(customer)" class="list-item" @click="openCustomerDetail(customer)">
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
          <button type="button" class="outline-button" @click="openFeaturePage(miniProgramFeaturePages.agentWithdrawals)">查看提现记录</button>

          <view class="section-card">
            <view class="section-header compact">
              <text class="section-title">分润明细</text>
              <text class="soft-tag">{{ channelCommissions.length }} 条</text>
            </view>
            <view v-if="channelCommissions.length" class="list-stack">
              <view v-for="commission in channelCommissions.slice(0, 8)" :key="rowKey(commission)" class="list-item" @click="openAgentCommissionDetail(commission)">
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
            <view class="v31-batch-actions"><button class="active" @click="openFeaturePage(miniProgramFeaturePages.agentTeam)">团队成员</button><button @click="openFeaturePage(miniProgramFeaturePages.agentOrders)">客户订单</button></view>
          </view>
          <view class="section-card">
            <view class="section-header compact"><text class="section-title">{{ roleLabels[userStore.currentRole] }}功能</text><text class="soft-tag">按权限展示</text></view>
            <view class="v31-batch-actions"><button v-for="item in currentRoleMenuItems" :key="item.id" :class="{ active: item.primary }" @click="handleV531ProfileService(item.id)">{{ item.label }}</button></view>
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
              <button class="quick-item" @click="selectTab('agents')">
                <text class="quick-value">{{ operationAgents.length }}</text>
                <text class="quick-label">代理商</text>
              </button>
              <button class="quick-item" @click="selectTab('orders')">
                <text class="quick-value">{{ operationOrders.length }}</text>
                <text class="quick-label">订单</text>
              </button>
              <button class="quick-item" @click="selectTab('commission')">
                <text class="quick-value">{{ formatCurrency(operationCommissionTotal) }}</text>
                <text class="quick-label">中心分润</text>
              </button>
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
              <view v-for="agent in operationAgents" :key="rowKey(agent)" class="list-item" @click="openOperationAgentDetail(agent)">
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
              <view v-for="order in operationOrders" :key="rowKey(order)" class="list-item" @click="openOperationOrderDetail(order)">
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
              <view v-for="commission in operationCommissions" :key="rowKey(commission)" class="list-item" @click="openOperationCommissionDetail(commission)">
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
          <view class="section-card">
            <view class="section-header compact"><text class="section-title">{{ roleLabels[userStore.currentRole] }}功能</text><text class="soft-tag">按权限展示</text></view>
            <view class="v31-batch-actions"><button v-for="item in currentRoleMenuItems" :key="item.id" :class="{ active: item.primary }" @click="handleV531ProfileService(item.id)">{{ item.label }}</button></view>
          </view>
        </view>
      </template>
    </view>

    <!-- #ifdef MP-WEIXIN -->
    <V531TabBar
      v-if="activeRole !== 'user' && isPrimaryRoleTab && !isCreationDetail && !isUserMineDetail"
      :role="activeRole"
      :active="activeTab"
      @change="selectTab"
    />
    <!-- #endif -->
    <!-- #ifdef APP-PLUS -->
    <V531TabBar
      v-if="activeRole !== 'user' && isPrimaryRoleTab && !isCreationDetail && !isUserMineDetail"
      :role="activeRole"
      :active="activeTab"
      @change="selectTab"
    />
    <!-- #endif -->
    <!-- #ifndef MP-WEIXIN -->
    <!-- #ifndef APP-PLUS -->
    <V531TabBar
      v-if="isPrimaryRoleTab && !isCreationDetail && !isUserMineDetail"
      :role="activeRole"
      :active="activeTab"
      @change="selectTab"
    />
    <!-- #endif -->
    <!-- #endif -->
  </view>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { onBackPress, onPullDownRefresh, onReachBottom, onShareAppMessage } from "@dcloudio/uni-app";
import { useMiniProgramNavigation } from "../composables/useMiniProgramNavigation";
import { ApiClientError } from "@xianzhi/api-client";
import { api, authStorage, businessSdk, setAuthToken } from "../api/client";

const { navigationStyle: miniWorkbenchSafeAreaStyle } = useMiniProgramNavigation();
import { uploadReferenceImage } from "../api/files";
import { inspirationAPI } from "../features/inspiration/api";
import { readInspirationDraft } from "../features/inspiration/draft";
import KnowledgeMiniChat from "./KnowledgeMiniChat.vue";
import AiGeneratedContentNotice from "./compliance/AiGeneratedContentNotice.vue";
import MiniProgramMineExperience from "./MiniProgramMineExperience.vue";
import PromotionCenterScreen from "./promotion/PromotionCenterScreen.vue";
import AppImage from "./AppImage.vue";
import RemoteCover from "./RemoteCover.vue";
import AssetCenterPage from "./assets/AssetCenterPage.vue";
import V531HomePage from "./v531/V531HomePage.vue";
import V531ProfilePage from "./v531/V531ProfilePage.vue";
import V531StudioPage from "./v531/V531StudioPage.vue";
import V531TabBar from "./v531/V531TabBar.vue";
import { fetchAssetDetail } from "../features/assets/api";
import { beginWorksPerformanceStep } from "../features/assets/performance";
import { usePageConfigStore, type AppPageCode } from "../stores/pageConfig";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import { requireAuth as requireProtectedAction } from "../features/auth/gate";
import { trackLogin } from "../features/auth/analytics";
import { ensureWechatMiniProgramSession } from "../features/auth/wechatSession";
import { reviewModeHides } from "../features/reviewMode";
import { RoleMenuConfig, roleLabels } from "../config/permissions";
import type { MinePurchaseOption, MineView } from "../types";
import loginLogo from "../assets/zhiqiyun-logo-transparent.png";
import type { ItemsResponse, MemberProfileResponse, OperationProfileResponse, RoleWalletResponse } from "@xianzhi/business-sdk";
import type { AppRole, Asset, AuthResponse, ChannelAgent, ChannelCenterResponse, GenerationTask } from "../types";
import {
  miniProgramCreationPages,
  miniProgramEnterprisePages,
  miniProgramFeaturePages,
  miniProgramMinePages,
  rolePage
} from "../config/miniProgramPages";
import type {
  MiniProgramCreationMode,
  MiniProgramRoleId,
  MiniProgramTabId
} from "../config/miniProgramPages";

function guestAwareGenerateTap() {
  if (!allowedCreationModes.value.includes(creationMode.value)) {
    uni.showToast({ title: "该能力暂未向小程序开放", icon: "none" });
    return;
  }
  if (!isGuest.value) return handleGenerateTap();
  const prompt = String(creationPrompt.value || "").trim();
  if (!prompt) {
    creationError.value = "\u8bf7\u5148\u8f93\u5165\u521b\u4f5c\u9700\u6c42";
    uni.showToast({ title: creationError.value, icon: "none" });
    return;
  }
  trackLogin("guest_click_generate", { mode: creationMode.value });
  const payload = {
    prompt, mode: creationMode.value, model: activeCreationModel.value,
    referencePaths: creationReferencePaths.value, restoredParams: restoredCreationParams.value,
    slideCount: pptSlideCount.value, language: pptLanguage.value, dynamic: pptDynamic.value,
  };
  uni.setStorageSync("v532-studio-draft", payload);
  void requireProtectedAction({
    action: creationMode.value === "video" ? "generate_video" : creationMode.value === "ppt" ? "generate_ppt" : "generate_image",
    route: miniProgramCreationPages[creationMode.value], payload, resume: () => submitCreation(prompt),
  });
}

type NativeGenerateBridge = typeof globalThis & {
  __xianzhiMiniProgramGenerate?: () => void;
  __xianzhiMiniProgramBackToCreation?: () => void;
  __xianzhiMiniProgramChooseReference?: () => void;
  __xianzhiMiniProgramAppendReferences?: (paths: string[]) => void;
  __xianzhiMiniProgramSetReferenceSelecting?: (selecting: boolean) => void;
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
    },
    nativeChooseReferenceImages() {
      const handler = (globalThis as NativeGenerateBridge).__xianzhiMiniProgramChooseReference;
      if (typeof handler === "function") handler();
    }
  }
});

type AnyRecord = Record<string, unknown>;
type RoleId = MiniProgramRoleId;
type TabId = MiniProgramTabId;
type CreationMode = MiniProgramCreationMode;
type AssetFilter = "all" | "image" | "video" | "document" | "favorite";

const roleToAppRole: Record<RoleId, AppRole> = {
  user: "USER",
  agent: "AGENT",
  operation: "OPERATION",
};
const appRoleToRole: Partial<Record<AppRole, RoleId>> = {
  USER: "user",
  AGENT: "agent",
  OPERATION: "operation",
};

const props = withDefaults(defineProps<{
  initialRole?: RoleId;
  initialTab?: TabId;
  initialCreationMode?: CreationMode;
  initialCreationAssetId?: string;
  initialCreationIntent?: "edit" | "regenerate";
  initialInspirationTemplateId?: string;
  initialMineView?: MineView;
}>(), {
  initialRole: "user",
  initialTab: "home",
  initialCreationAssetId: "",
  initialCreationIntent: "edit",
  initialInspirationTemplateId: "",
  initialMineView: "overview"
});

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
  resultId?: string;
  resultUrl?: string;
  resultType?: CreationMode;
  progress?: number;
}

interface ActiveGenerationSnapshot {
  id: string;
  mode: CreationMode;
  prompt: string;
  status: string;
  progress: number;
  startedAt: number;
  inspirationTemplateId?: string;
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

const allCreationModules = [
  { id: "image" as CreationMode, icon: "图", name: "AI生图", homeName: "轻易海报", description: "主图/海报/配图", model: "gpt-image-2", cost: "约 10 点/张", tone: "orange" },
  { id: "ppt" as CreationMode, icon: "P", name: "PPT文档", homeName: "PPT文档", description: "方案/培训/路演", model: "ppt-generator", cost: "约 30 点/份", tone: "purple" },
  { id: "video" as CreationMode, icon: "视", name: "视频生成", homeName: "视频生成", description: "广告/口播/图生视频", model: "doubao-seedance-2.0", cost: "约 80 点/条", tone: "green" },
  { id: "agent" as CreationMode, icon: "星", name: "AI Agent", homeName: "LOGO", description: "经营助手与知识库", model: "agent-workflow", cost: "按任务计费", tone: "blue" },
  { id: "infographic" as CreationMode, icon: "表", name: "信息图", homeName: "信息图", description: "复杂信息一图讲清", model: "infographic", cost: "约 20 点/份", tone: "orange" },
  { id: "review" as CreationMode, icon: "查", name: "易找茬", homeName: "易共识", description: "多模型判断与风险", model: "multi-model", cost: "按模型计费", tone: "purple" }
];

const allowedCreationModes = ref<CreationMode[]>(["image", "infographic", "video"]);
const creationModules = computed(() => allCreationModules.filter(item => allowedCreationModes.value.includes(item.id)));

const pptTopics = ["企业营销增长", "数字员工方案", "GEO品牌曝光", "短视频矩阵", "项目路演计划", "糖尿病患教"];
const assetFilters: Array<{ id: AssetFilter; label: string }> = [
  { id: "all", label: "全部" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "document", label: "PPT" },
  { id: "favorite", label: "收藏" }
];

const auth = ref<AuthResponse | null>(null);
const token = ref("");
const isGuest = computed(() => !token.value);
const pageLoading = ref(false);
const pageError = ref("");
const activeRole = ref<RoleId>(props.initialRole);
const activeTab = ref<TabId>(props.initialTab);
const pageConfigStore = usePageConfigStore();
const authStore = useAuthStore();
const userStore = useUserStore();
const creationMode = ref<CreationMode>(props.initialCreationMode || "image");
const creationPrompt = ref("");
const creationReferencePaths = ref<string[]>([]);
const creationSourceReferenceUrl = ref("");
const creationSourceLoading = ref(false);
const creationSourceError = ref("");
const creationReferenceSelecting = ref(false);
const loadedCreationAssetKey = ref("");
const restoredCreationParams = ref<AnyRecord>({});
const activeInspirationTemplateId = ref("");
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
const generationPolling = ref(false);
const generationProgress = ref(0);
const generationElapsedSeconds = ref(0);
const latestGenerationTask = ref<GenerationNotice | null>(null);
const activeGenerationStorageKey = "zhiqiyun:active-generation-task";
let generationElapsedTimer: ReturnType<typeof setInterval> | null = null;
let generationRepollTimer: ReturnType<typeof setTimeout> | null = null;
let generationPollRun = 0;
const pptSlideCount = ref(10);
const pptDynamic = ref(true);
const pptLanguage = ref<"zh" | "en">("zh");
const pptModel = ref("GPT-4o-mini");
const assetFilter = ref<AssetFilter>("all");
const assetSearch = ref("");
const mineView = ref<MineView>(props.initialMineView);
const selectedMinePurchase = ref<MinePurchaseOption | null>(null);
const minePurchaseSubmitting = ref(false);
const mineLogoutConfirm = ref(false);

const profile = ref<MemberProfileResponse | null>(null);
const wallet = ref<RoleWalletResponse | null>(null);
const pointAccountResponse = ref<RoleWalletResponse | null>(null);
const recentAssets = ref<Asset[]>([]);
const generationTasks = ref<GenerationTask[]>([]);
const assetTotal = ref(0);
const assetMonthTotal = ref(0);
const assetFavoriteTotal = ref(0);
const assetStorageBytes = ref(0);
const rechargePackages = ref<AnyRecord[]>([]);
const assetsLoading = ref(false);
const assetsError = ref("");

const channelCenter = ref<ChannelCenterResponse | null>(null);
const operationProfile = ref<OperationProfileResponse | null>(null);
const operationAgentsResponse = ref<ItemsResponse | null>(null);
const operationOrdersResponse = ref<ItemsResponse | null>(null);
const operationCommissionsResponse = ref<ItemsResponse | null>(null);

const currentTabs = computed(() => roleTabs[activeRole.value]);
const isPrimaryRoleTab = computed(() => currentTabs.value.some(tab => tab.id === activeTab.value));
const legacyActiveTab = computed<TabId>(() => activeTab.value);
const isCreationDetail = computed(
  () => activeRole.value === "user" && activeTab.value === "create" && Boolean(props.initialCreationMode),
);
const creationReferenceEnabled = computed(() => (["image", "video", "infographic"] as CreationMode[]).includes(creationMode.value));
const creationReferenceModeLabel = computed(() => `${creationReferencePaths.value.length} 张 · ${creationMode.value === "video" ? "参考图模式" : "图生图模式"}`);
const roleLabel = computed(() => roleNames[activeRole.value]);
const isUserMineDetail = computed(() => activeRole.value === "user" && activeTab.value === "mine" && mineView.value !== "overview");
const isV531PrimaryPage = computed(
  () => activeRole.value === "user" && !isCreationDetail.value && !isUserMineDetail.value,
);
const displayedAssets = computed(() => recentAssets.value);
const filteredAssets = computed(() => displayedAssets.value.filter(asset => {
  const matchesType = assetFilter.value === "all"
    || (assetFilter.value === "favorite" && Boolean(asset.metadata?.favorite))
    || asset.mediaType === assetFilter.value;
  const matchesSearch = !assetSearch.value.trim() || asset.name.toLowerCase().includes(assetSearch.value.trim().toLowerCase());
  return matchesType && matchesSearch;
}));
const displayName = computed(() => profile.value?.user?.name || auth.value?.user?.name || profile.value?.user?.email || auth.value?.user?.email || "访客");
const displayUserId = computed(() => rowString(profile.value?.user || {}, "id", "userId") || rowString(auth.value?.user || {}, "id", "userId") || "--");
const userAvatarUrl = computed(() => rowString(profile.value?.user || {}, "avatarUrl", "avatar", "headImage") || rowString(auth.value?.user || {}, "avatarUrl", "avatar", "headImage"));
const userEmail = computed(() => profile.value?.user?.email || auth.value?.user?.email || "-");
const greetingText = computed(() => `${displayName.value}，欢迎回来`);
const planName = computed(() => rowString(profile.value?.plan || {}, "name") || rowString(profile.value?.plan || {}, "planName") || auth.value?.defaultModule || "AI 创作用户");
const profileCompanyName = computed(
  () => rowString(profile.value?.user || {}, "companyName", "company", "organization", "tenantName")
    || rowString(auth.value?.user || {}, "companyName", "company", "organization", "tenantName")
    || rowString(profile.value?.operationCenter || {}, "companyName", "name", "tenantName")
    || rowString(profile.value?.plan || {}, "companyName", "tenantName", "name", "planName")
    || "企业信息待完善",
);
const profileSubscriptionExpiresAt = computed(
  () => rowString(profile.value?.user || {}, "subscriptionExpiresAt", "expiresAt", "validUntil")
    || rowString(auth.value?.user || {}, "subscriptionExpiresAt", "expiresAt", "validUntil")
    || rowString(profile.value?.plan || {}, "expiresAt", "validUntil", "endedAt"),
);

const pointAccount = computed(() => wallet.value?.account || pointAccountResponse.value?.account || profile.value?.account || null);
const pointBalance = computed(() => asNumber(pointAccount.value?.available));
const pointFrozen = computed(() => asNumber(pointAccount.value?.frozen));
const userOrders = computed(() => listOf(wallet.value?.orders || pointAccountResponse.value?.orders));
const userTransactions = computed(() => listOf(wallet.value?.transactions || pointAccountResponse.value?.transactions));
const tokenRecords = computed(() => listOf(wallet.value?.tokenRecords));
const tokenUsageRecords = computed(() => tokenRecords.value.filter(item => {
  const changeType = (rowString(item, "changeType") || rowString(item, "type")).toUpperCase();
  const delta = rowNumber(item, "delta") || rowNumber(item, "amount");
  return delta < 0 || changeType.includes("CONSUME") || changeType.includes("USAGE") || changeType.includes("DEDUCT");
}));
const walletRecords = computed(() => [...userTransactions.value, ...tokenUsageRecords.value].sort((a, b) => {
  const timeA = new Date(rowDate(a)).getTime();
  const timeB = new Date(rowDate(b)).getTime();
  return (Number.isFinite(timeB) ? timeB : 0) - (Number.isFinite(timeA) ? timeA : 0);
}));
const monthlyPointCost = computed(() => walletRecords.value
  .filter(item => isCurrentMonth(rowDate(item)))
  .reduce((sum, item) => sum + Math.abs(rowPointCost(item)), 0));
const monthlyGrantedPoints = computed(() => tokenRecords.value.reduce((sum, item) => {
  const changeType = (rowString(item, "changeType") || rowString(item, "type")).toUpperCase();
  const amount = Math.abs(rowNumber(item, "amount") || rowNumber(item, "points") || rowNumber(item, "delta"));
  const granted = changeType.includes("GRANT") || changeType.includes("BONUS") || changeType.includes("GIFT");
  return granted ? sum + amount : sum;
}, 0));
const todayCallCount = computed(() => walletRecords.value.filter(item => isToday(rowDate(item))).length);
const monthlyAssetCount = computed(() => assetMonthTotal.value || recentAssets.value.filter(item => isCurrentMonth(String((item as unknown as AnyRecord).createdAt || (item as unknown as AnyRecord).created_at || ""))).length);
const imageAssetCount = computed(() => recentAssets.value.filter(item => {
  const type = rowString(item as unknown as AnyRecord, "mediaType", "type", "assetType").toLowerCase();
  return !type || type === "image" || type.includes("image");
}).length);
const videoAssetCount = computed(() => recentAssets.value.filter(item => rowString(item as unknown as AnyRecord, "mediaType", "type", "assetType").toLowerCase().includes("video")).length);
const pptAssetCount = computed(() => recentAssets.value.filter(item => {
  const type = rowString(item as unknown as AnyRecord, "mediaType", "type", "assetType").toLowerCase();
  return type.includes("ppt") || type.includes("document") || type.includes("presentation");
}).length);
const assetStorageLabel = computed(() => {
  const bytes = assetStorageBytes.value || recentAssets.value.reduce((sum, item) => {
    const metadata = (item as unknown as AnyRecord).metadata || {};
    return sum + (rowNumber(metadata, "fileSize") || rowNumber(metadata, "fileSizeBytes") || rowNumber(metadata, "sizeBytes"));
  }, 0);
  const capacity = recentAssets.value.reduce((largest, item) => {
    const metadata = (item as unknown as AnyRecord).metadata || {};
    const itemCapacity = rowNumber(metadata, "storageCapacity") || rowNumber(metadata, "storageLimit") || rowNumber(metadata, "storageLimitBytes");
    return Math.max(largest, itemCapacity);
  }, 0);
  if (capacity > 0) return `${Math.min(100, Math.round((bytes / capacity) * 100))}%`;
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)}GB`;
  if (bytes > 0) return `${Math.max(0.1, bytes / 1024 ** 2).toFixed(1)}MB`;
  return "0%";
});

const hasAgentRole = computed(() => userStore.hasRole("AGENT"));
const hasOperationRole = computed(() => userStore.hasRole("OPERATION"));
const availableRoles = computed(() => userStore.roles
  .map(role => ({ appRole: role, id: appRoleToRole[role], label: roleLabels[role] }))
  .filter(role => !(role.id === "agent" && reviewModeHides("hideAgentCenter")))
  .filter(role => !(role.id === "operation" && reviewModeHides("hideOperatorCenter")))
  .filter((role): role is { appRole: AppRole; id: RoleId; label: string } => Boolean(role.id)));
const currentRoleMenuItems = computed(() => RoleMenuConfig[userStore.currentRole]
  .filter(item => !item.permission || userStore.hasPermission(item.permission))
  .filter(item => !(item.id === "wallet" && reviewModeHides("hideWallet")))
  .filter(item => !(item.id === "upgrade-agent" && reviewModeHides("hideAgentCenter")))
  .filter(item => !(item.id.includes("commission") && reviewModeHides("hideCommission"))));

const activeCreation = computed(() => creationModules.value.find(item => item.id === creationMode.value) || creationModules.value[0] || allCreationModules[0]);
const activeCreationName = computed(() => activeCreation.value.name);
const activeCreationModel = computed(() => rowString(restoredCreationParams.value, "model", "modelName") || activeCreation.value.model);
const activeCreationCost = computed(() => activeCreation.value.cost);
const generationBusy = computed(() => generationSubmitting.value || generationPolling.value);
const generationNoticePending = computed(() => latestGenerationTask.value?.tone === "pending");
const generationHasProgress = computed(() => generationProgress.value > 0 && generationProgress.value < 100);
const generationProgressStyle = computed(() => generationHasProgress.value
  ? { width: `${Math.min(100, Math.max(0, generationProgress.value))}%` }
  : undefined);
const generationStatusLabel = computed(() => generationStatusText(latestGenerationTask.value?.status || ""));
const generationButtonLabel = computed(() => {
  if (generationSubmitting.value) return "提交中...";
  if (!generationPolling.value) return "生成";
  const stage = generationStatusLabel.value || "生成中";
  return generationHasProgress.value ? `${stage} ${generationProgress.value}%` : `${stage}...`;
});
const generationFeedbackText = computed(() => {
  const elapsed = generationElapsedSeconds.value > 0 ? `已等待 ${generationElapsedSeconds.value} 秒` : "刚刚提交";
  return generationHasProgress.value ? `后端进度 ${generationProgress.value}% · ${elapsed}` : `状态持续同步中 · ${elapsed}`;
});

function generationStatusText(status: string) {
  const normalized = String(status || "").toUpperCase();
  if (["PENDING", "QUEUED", "CREATED"].includes(normalized)) return "排队中";
  if (["PROCESSING", "RUNNING", "RETRYING", "IN_PROGRESS"].includes(normalized)) return "生成中";
  if (["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(normalized)) return "已完成";
  if (["FAILED", "ERROR"].includes(normalized)) return "生成失败";
  return normalized || "同步中";
}

function creationNameForMode(mode: CreationMode) {
  return allCreationModules.find(item => item.id === mode)?.name || "AI 创作";
}

function restoredCreationString(...keys: string[]) {
  return rowString(restoredCreationParams.value, ...keys);
}

function restoredCreationCount() {
  const parsed = Number(restoredCreationString("count", "generationCount", "imageCount"));
  return Number.isFinite(parsed) && parsed >= 1 ? Math.min(4, Math.floor(parsed)) : 1;
}

const currentPageTitle = computed(() => {
  if (activeRole.value === "agent") return "代理工作台";
  if (activeRole.value === "operation") return "运营中心";
  if (activeTab.value === "create") return creationMode.value === "ppt" ? "PPT文档生成" : "创作中心";
  if (activeTab.value === "assets") return "我的作品";
  if (activeTab.value === "wallet") return "钱包与点数";
  if (activeTab.value === "mine") return "我的";
  return "知启云 AI";
});
const creationDetailTitle = computed(
  () => allCreationModules.find(module => module.id === creationMode.value)?.name || "AI 创作",
);

const currentPageSubtitle = computed(() => {
  if (activeRole.value === "agent") return "用户身份与代理身份可切换";
  if (activeRole.value === "operation") return "区域经营、代理与订单";
  if (activeTab.value === "create") return creationMode.value === "ppt" ? "Gamma式输入，移动端轻量化" : "轻易设计 + 多模型工作流";
  if (activeTab.value === "assets") return "统一查看图片、视频、PPT";
  if (activeTab.value === "wallet") return "充值、点数余额与消耗记录";
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
const inviteCode = computed(() => promotionInfo.value.inviteCode || currentAgent.value?.inviteCode || rowString(currentAgent.value || {}, "inviteCode") || "未生成");
const inviteLink = computed(() => promotionInfo.value.inviteLink || promotionInfo.value.landingURL || rowString(currentAgent.value || {}, "inviteLink"));
const sharePath = computed(() => `/pages/WechatLoginPage?invite=${encodeURIComponent(inviteCode.value)}`);
const conversionRate = computed(() => {
  const visits = summaryNumber(channelSummary.value, "visits");
  const orders = summaryNumber(channelSummary.value, "orders");
  return visits > 0 ? Math.round(orders / visits * 100) : 0;
});

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

watch(() => props.initialRole, role => { activeRole.value = role; });
watch(() => props.initialTab, tab => { activeTab.value = tab; });
watch(activeTab, tab => { const code = ({ home: "home", create: "studio", assets: "assets", mine: "profile" } as Partial<Record<TabId, AppPageCode>>)[tab]; if (code) void pageConfigStore.ensure(code); }, { immediate: true });
watch(() => props.initialCreationMode, mode => {
  if (mode) creationMode.value = mode;
});
watch(
  () => [props.initialCreationAssetId, props.initialCreationIntent] as const,
  ([assetId, intent]) => {
    if (assetId) void initializeCreationFromAsset(assetId, intent);
  },
  { immediate: true },
);
watch(() => props.initialMineView, view => { mineView.value = view; });

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

function collectionOf<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[];
  if (!value || typeof value !== "object") return [];
  const record = value as AnyRecord;
  for (const key of ["items", "rows", "data"] as const) {
    if (Array.isArray(record[key])) return record[key] as T[];
  }
  return [];
}

function rowString(row: unknown, ...keys: string[]) {
  if (!row || typeof row !== "object") return "";
  for (const key of keys) {
    const value = asString((row as AnyRecord)[key]);
    if (value) return value;
  }
  return "";
}

function rowNumber(row: unknown, key: string) {
  if (!row || typeof row !== "object") return 0;
  return asNumber((row as AnyRecord)[key]);
}

function creationReferenceURLs(metadata: AnyRecord) {
  for (const key of ["referenceImages", "inputImagesSnapshot", "inputImages", "reference_urls"] as const) {
    const value = metadata[key];
    if (Array.isArray(value)) {
      return value.map(item => {
        if (typeof item === "string") return item.trim();
        return rowString(item, "url", "remoteUrl", "sourceUrl", "fileUrl");
      }).filter(Boolean);
    }
    if (typeof value === "string" && value.trim()) {
      return value.split(/[,，]/).map(item => item.trim()).filter(Boolean);
    }
  }
  return [];
}

function clampGenerationProgress(value: unknown) {
  const progress = Number(value);
  return Number.isFinite(progress) ? Math.min(100, Math.max(0, Math.round(progress))) : 0;
}

function startGenerationFeedback(startedAt = Date.now()) {
  generationPolling.value = true;
  const normalizedStart = Number.isFinite(startedAt) && startedAt > 0 ? startedAt : Date.now();
  const updateElapsed = () => {
    generationElapsedSeconds.value = Math.max(0, Math.floor((Date.now() - normalizedStart) / 1000));
  };
  if (generationElapsedTimer) clearInterval(generationElapsedTimer);
  updateElapsed();
  generationElapsedTimer = setInterval(updateElapsed, 1000);
}

function stopGenerationFeedback(clearStoredTask = true) {
  generationPolling.value = false;
  if (generationElapsedTimer) clearInterval(generationElapsedTimer);
  if (generationRepollTimer) clearTimeout(generationRepollTimer);
  generationElapsedTimer = null;
  generationRepollTimer = null;
  if (clearStoredTask) uni.removeStorageSync(activeGenerationStorageKey);
}

function persistActiveGeneration(snapshot: ActiveGenerationSnapshot) {
  uni.setStorageSync(activeGenerationStorageKey, snapshot);
}

function restoreActiveGeneration() {
  const raw = uni.getStorageSync(activeGenerationStorageKey);
  if (!raw || typeof raw !== "object") return;
  const snapshot = raw as Partial<ActiveGenerationSnapshot>;
  const id = String(snapshot.id || "").trim();
  const mode = String(snapshot.mode || "") as CreationMode;
  const startedAt = Number(snapshot.startedAt || 0);
  if (!id || mode !== creationMode.value) return;
  if (!startedAt || Date.now() - startedAt > 6 * 60 * 60 * 1000) {
    uni.removeStorageSync(activeGenerationStorageKey);
    return;
  }
  const status = String(snapshot.status || "PENDING").toUpperCase();
  if (["FAILED", "ERROR", "SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)) {
    uni.removeStorageSync(activeGenerationStorageKey);
    return;
  }
  if (!creationPrompt.value && snapshot.prompt) creationPrompt.value = String(snapshot.prompt);
  activeInspirationTemplateId.value = String(snapshot.inspirationTemplateId || "").trim();
  generationProgress.value = clampGenerationProgress(snapshot.progress);
  latestGenerationTask.value = {
    id,
    title: `${creationNameForMode(mode)}生成中`,
    status,
    tone: "pending",
    progress: generationProgress.value,
    resultType: mode,
  };
  startGenerationFeedback(startedAt);
  void pollGenerationTask(id, mode, startedAt, String(snapshot.prompt || creationPrompt.value));
}

async function initializeCreationFromAsset(assetId: string, intent: "edit" | "regenerate") {
  const normalizedId = String(assetId || "").trim();
  const requestKey = `${intent}:${normalizedId}`;
  if (!normalizedId || loadedCreationAssetKey.value === requestKey) return;
  loadedCreationAssetKey.value = requestKey;
  creationSourceLoading.value = true;
  creationSourceError.value = "";
  try {
    const sourceAsset = await fetchAssetDetail(normalizedId);
    const metadata = sourceAsset.metadata || {};
    const originalReferences = creationReferenceURLs(metadata);
    const editableOutput = intent === "edit" ? sourceAsset.remoteUrl || sourceAsset.thumbnailUrl : "";
    const references = [editableOutput, ...originalReferences]
      .filter((value, index, values) => Boolean(value) && values.indexOf(value) === index)
      .slice(0, 3);

    if (intent === "edit" && !editableOutput) {
      throw new Error("原作品暂无可用图片，无法载入参考图");
    }

    creationSourceReferenceUrl.value = editableOutput;
    creationReferencePaths.value = references;
    creationPrompt.value = sourceAsset.prompt || "";
    restoredCreationParams.value = {
      ...metadata,
      sourceAssetId: sourceAsset.id,
      sourceTaskId: sourceAsset.taskId || "",
      intent,
      model: sourceAsset.model || metadata.model,
      aspectRatio: sourceAsset.aspectRatio || metadata.aspectRatio,
      seed: sourceAsset.seed ?? metadata.seed,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "原作品载入失败";
    creationSourceError.value = message;
    loadedCreationAssetKey.value = "";
    uni.showToast({ title: message, icon: "none" });
  } finally {
    creationSourceLoading.value = false;
  }
}

function chooseCreationReferenceImages() {
  if (!requestLogin("登录后可上传参考图片")) return;
  if (creationReferenceSelecting.value) return;
  const remaining = Math.max(0, 3 - creationReferencePaths.value.length);
  if (!remaining) {
    uni.showToast({ title: "最多添加 3 张参考图", icon: "none" });
    return;
  }
  creationReferenceSelecting.value = true;
  uni.chooseImage({
    count: remaining,
    sizeType: ["compressed"],
    sourceType: ["album", "camera"],
    success: result => {
      const selectedPaths = result.tempFilePaths;
      const paths = Array.isArray(selectedPaths) ? selectedPaths : selectedPaths ? [selectedPaths] : [];
      appendCreationReferencePaths(paths);
    },
    fail: error => {
      if (!String(error.errMsg || "").toLowerCase().includes("cancel")) {
        uni.showToast({ title: "参考图选择失败", icon: "none" });
      }
    },
    complete: () => {
      creationReferenceSelecting.value = false;
    },
  });
}

function appendCreationReferencePaths(paths: string[]) {
  if (!requestLogin("登录后可上传参考图片")) return;
  creationReferencePaths.value = [...creationReferencePaths.value, ...paths]
    .filter((value, index, values) => Boolean(value) && values.indexOf(value) === index)
    .slice(0, 3);
  creationSourceError.value = "";
  creationError.value = "";
}

function setCreationReferenceSelecting(selecting: boolean) {
  creationReferenceSelecting.value = selecting;
}

function previewCreationReference(index: number) {
  const current = creationReferencePaths.value[index];
  if (!current) return;
  uni.previewImage({ urls: creationReferencePaths.value, current });
}

function removeCreationReference(index: number) {
  creationReferencePaths.value = creationReferencePaths.value.filter((_, itemIndex) => itemIndex !== index);
}

function isToday(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
}

function isCurrentMonth(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth();
}

function isWithinDays(value: string, days: number) {
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) && timestamp >= Date.now() - days * 86400000;
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

function readAuth() {
  const legacyAuthValue = uni.getStorageSync("xianzhiMiniProgramAuth") as AuthResponse | "";
  const legacyAuth = legacyAuthValue && typeof legacyAuthValue === "object" ? legacyAuthValue : null;
  const storedAuth = authStorage.getAuth() || legacyAuth || null;
  token.value = authStorage.getToken()
    || String(uni.getStorageSync("token") || "")
    || rowString(storedAuth || {}, "accessToken", "token");
  auth.value = storedAuth;
  if (token.value) setAuthToken(token.value);
  if (storedAuth) {
    authStorage.setAuth(storedAuth);
    if (rowString(storedAuth, "refreshToken")) authStorage.setRefreshToken(rowString(storedAuth, "refreshToken"));
  }
}

function requestLogin(reason = "登录后可继续使用此功能") {
  if (!isGuest.value) return true;
  uni.showModal({
    title: "登录后使用",
    content: `${reason}。你也可以取消并继续浏览。`,
    confirmText: "去登录",
    cancelText: "继续浏览",
    confirmColor: "#4A6BFF",
    success: result => {
      if (!result.confirm) return;
      const pages = getCurrentPages();
      const current = pages[pages.length - 1] as { route?: string } | undefined;
      const redirectPath = current?.route ? `/${String(current.route).replace(/^\/+/, "")}` : "/pages/user/UserHomePage";
      const query = `?redirectPath=${encodeURIComponent(redirectPath)}&sourcePage=${encodeURIComponent(redirectPath)}`;
      uni.navigateTo({
        url: `/pages/WechatLoginPage${query}`,
        fail: () => uni.reLaunch({ url: `/pages/WechatLoginPage${query}` }),
      });
    },
  });
  return false;
}

function replacePage(url: string) {
  const pages = getCurrentPages();
  const currentPage = pages[pages.length - 1] as { route?: string } | undefined;
  const targetRoute = url.replace(/^\//, "").split("?")[0];
  if (currentPage?.route === targetRoute) return;
  const primaryTabRoutes = new Set(
    (Object.keys(roleTabs) as RoleId[]).flatMap(role => roleTabs[role].map(tab => rolePage(role, tab.id).replace(/^\//, ""))),
  );
  const userNativeTabRoutes = new Set([
    "pages/user/UserHomePage",
    "pages/user/UserCreationPage",
    "pages/user/UserAssetsPage",
    "pages/user/UserMinePage",
  ]);
  if (userNativeTabRoutes.has(targetRoute)) {
    uni.switchTab({
      url,
      fail: () => uni.reLaunch({ url }),
    });
    return;
  }
  if (primaryTabRoutes.has(targetRoute)) {
    uni.reLaunch({ url });
    return;
  }
  uni.redirectTo({
    url,
    fail: () => uni.reLaunch({ url })
  });
}

function openStandalonePage(url: string) {
  if (!url) {
    uni.showToast({ title: "页面地址为空", icon: "none" });
    return;
  }
  const pages = getCurrentPages();
  const currentPage = pages[pages.length - 1] as { route?: string } | undefined;
  const targetRoute = url.replace(/^\//, "").split("?")[0];
  if (currentPage?.route === targetRoute) return;
  uni.navigateTo({
    url,
    fail(navigateError: unknown) {
      uni.redirectTo({
        url,
        fail(redirectError: unknown) {
          uni.reLaunch({
            url,
            fail(relaunchError: unknown) {
              console.warn("[xianzhi] failed to open standalone page", url, navigateError, redirectError, relaunchError);
              uni.showToast({ title: "页面打开失败，请重试", icon: "none" });
            }
          });
        }
      });
    }
  });
}

async function switchRole(role: RoleId) {
  if (!requestLogin("登录后可切换工作台身份")) return;
  const appRole = roleToAppRole[role];
  try {
    await userStore.switchRole(appRole);
    activeRole.value = role;
    replacePage(rolePage(role, role === "user" ? "mine" : "overview"));
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "角色切换失败", icon: "none" });
  }
}

function handleV531RoleChange(role: AppRole) {
  const targetRole = appRoleToRole[role];
  if (!targetRole) {
    uni.showToast({ title: `${roleLabels[role]}工作台即将开放`, icon: "none" });
    return;
  }
  if (targetRole === "user") {
    void switchRole("user");
    return;
  }
  if (targetRole === "agent") {
    if (hasAgentRole.value) void switchRole("agent");
    else openFeaturePage(miniProgramFeaturePages.userAgentDetail);
    return;
  }
  if (hasOperationRole.value) void switchRole("operation");
  else uni.showToast({ title: "当前账号未开通运营中心", icon: "none" });
}

function cycleRole() {
  const index = availableRoles.value.findIndex(role => role.id === activeRole.value);
  const next = availableRoles.value[(index + 1) % availableRoles.value.length];
  if (next) void switchRole(next.id);
}

function cyclePptSlides() {
  const counts = [5, 8, 10, 15, 20];
  const index = counts.indexOf(pptSlideCount.value);
  pptSlideCount.value = counts[(index + 1) % counts.length] || 10;
}

function togglePptLanguage() {
  pptLanguage.value = pptLanguage.value === "zh" ? "en" : "zh";
}

function cyclePptModel() {
  const models = ["GPT-4o-mini", "Kimi K2.6", "DeepSeek V3"];
  const index = models.indexOf(pptModel.value);
  pptModel.value = models[(index + 1) % models.length] || models[0];
}

function homeModuleSlot(mode: CreationMode) { return ({ image: "home.quick.poster", ppt: "home.quick.ppt", video: "home.quick.video", agent: "home.quick.knowledge", infographic: "home.capability.office", review: "home.capability.employee" } as Record<CreationMode, string>)[mode]; }
function studioModuleSlot(mode: CreationMode) { return ({ image: "studio.template.poster", ppt: "studio.template.ppt", video: "studio.template.video", agent: "studio.template.knowledge", infographic: "studio.template.office", review: "studio.template.employee" } as Record<CreationMode, string>)[mode]; }
function assetDefaultSlot(mediaType: string) { if (mediaType === "image") return "assets.default.image"; if (mediaType === "video") return "assets.default.video"; if (mediaType === "document") return "assets.default.document"; return "assets.default.other"; }

function selectUserTab(tab: TabId) {
  if (!["home", "create"].includes(tab) && !requestLogin("登录后可查看作品、账户与权益")) return;
  replacePage(rolePage("user", tab));
}

const agentWorkbenchTabs = new Set<TabId>(["overview", "promotion", "customers", "commission", "mine"]);
const operationWorkbenchTabs = new Set<TabId>(["overview", "agents", "orders", "commission", "mine"]);

function selectTab(tab: TabId) {
  if (activeRole.value === "agent") {
    selectAgentTab(tab);
    return;
  }
  if (activeRole.value === "operation" && operationWorkbenchTabs.has(activeTab.value) && operationWorkbenchTabs.has(tab)) {
    if (tab === activeTab.value) return;
    activeTab.value = tab;
    if (!operationProfile.value) void loadRoleData("operation");
    return;
  }
  replacePage(rolePage(activeRole.value, tab));
}

function openMineView(view: MineView) {
  replacePage(miniProgramMinePages[view]);
}

function selectMinePurchase(purchase: MinePurchaseOption) {
  if (purchase.kind === "agent") {
    selectedMinePurchase.value = null;
    openFeaturePage(miniProgramFeaturePages.userAgentDetail);
    return;
  }
  selectedMinePurchase.value = purchase;
  mineLogoutConfirm.value = false;
}

async function confirmMinePurchase() {
  const purchase = selectedMinePurchase.value;
  if (!purchase || minePurchaseSubmitting.value) return;
  minePurchaseSubmitting.value = true;
  try {
    if (purchase.kind === "agent") {
      selectedMinePurchase.value = null;
      openFeaturePage(miniProgramFeaturePages.userAgentDetail);
      return;
    }
    const created = await createRechargeOrder({ id: purchase.id, amountCents: purchase.amountCents, points: purchase.points });
    if (created) selectedMinePurchase.value = null;
  } finally {
    minePurchaseSubmitting.value = false;
  }
}

function showInvoiceNotice() {
  openFeaturePage(miniProgramFeaturePages.userInvoices);
}

async function showUsageExportNotice() {
  try {
    const payload = await api<AnyRecord | AnyRecord[]>("/api/v1/user/usage");
    const rows = Array.isArray(payload) ? payload : listOf(payload.items || payload.rows || payload.data);
    const csv = ["时间,项目,点数", ...rows.map(row => `${rowDate(row)},${usageTitle(row)},${rowPointCost(row)}`)].join("\n");
    uni.setClipboardData({ data: csv, success: () => uni.showToast({ title: "明细已复制", icon: "success" }) });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "明细导出失败", icon: "none" });
  }
}

function showPosterNotice() {
  if (hasAgentRole.value) replacePage(rolePage("agent", "promotion"));
  else uni.showToast({ title: "请先成为代理商", icon: "none" });
}

async function showNotifications() {
  if (!requestLogin("登录后可查看通知")) return;
  try {
    const dashboard = await api<AnyRecord>("/api/v1/user/dashboard");
    const recentTasks = listOf(dashboard.recentTasks);
    const recentAssets = listOf(dashboard.recentAssets);
    uni.showModal({
      title: "通知中心",
      content: `已同步后端工作台：近期任务 ${recentTasks.length} 条，近期作品 ${recentAssets.length} 项。`,
      showCancel: false
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "通知加载失败", icon: "none" });
  }
}

function showBatchManager() {
  openFeaturePage(`${miniProgramFeaturePages.userAssetsList}?manage=1`);
}

function openAllAssets() {
  openFeaturePage(miniProgramFeaturePages.userAssetsList);
}

function openTaskRecords() {
  openFeaturePage(miniProgramFeaturePages.userTasksList);
}

async function showHelpCenter() {
  const [pageResult, settingsResult] = await Promise.allSettled([
    api<AnyRecord>("/api/v1/app/page-config/profile"),
    api<AnyRecord>("/api/v1/user/api-settings")
  ]);
  const pageReady = pageResult.status === "fulfilled";
  const settingsReady = settingsResult.status === "fulfilled";
  const slots = pageReady ? listOf(pageResult.value.slots || pageResult.value.items) : [];
  uni.showModal({
    title: "帮助客服",
    content: `已连接后端帮助配置。个人中心页面配置：${pageReady ? `${slots.length} 个槽位` : "暂不可用"}；用户 API 设置：${settingsReady ? "已同步" : "暂不可用"}。`,
    showCancel: false
  });
}

async function showApiSettingsSummary() {
  try {
    const settings = await api<AnyRecord>("/api/v1/user/api-settings");
    const summary = (settings.summary || {}) as AnyRecord;
    const quota = (settings.quota || {}) as AnyRecord;
    const modelCount = asNumber(summary.models);
    const capabilityCount = asNumber(summary.capabilities);
    const apiKeyCount = asNumber(summary.apiKeyCount);
    const userGroup = rowString(settings, "userGroup") || "默认用户组";
    const defaultModel = rowString(settings, "defaultModel") || "未配置默认模型";
    uni.showModal({
      title: "API 管理",
      content: `用户组：${userGroup}\n默认模型：${defaultModel}\n可用模型：${modelCount} 个\n能力类型：${capabilityCount} 类\n平台密钥：${apiKeyCount} 个\n可用点数：${formatNumber(quota.available)}`,
      showCancel: false
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "API 设置加载失败", icon: "none" });
  }
}

function showRecycleBinStatus() {
  uni.showModal({
    title: "回收站",
    content: "当前后台仅开放画布回收站兼容接口，作品回收站尚未独立开放；删除作品时仍会以二次确认为准，不会在本页伪造回收数据。",
    showCancel: false
  });
}

function showFeedbackDialog() {
  uni.showModal({
    title: "反馈与建议",
    content: "已接入帮助客服入口。提交产品反馈、异常截图或账号问题时，可先同步后端帮助配置并联系管理员处理。",
    confirmText: "联系帮助",
    success: result => {
      if (result.confirm) void showHelpCenter();
    }
  });
}

async function showCompanyCertification() {
  try {
    const payload = await businessSdk.enterprise.contexts();
    const enterpriseContexts = payload.contexts.filter(item => item.type === "ENTERPRISE" && item.memberStatus === "ACTIVE");
    const current = enterpriseContexts.find(item => item.current) || enterpriseContexts[0];
    if (!current) {
      uni.showModal({
        title: "尚未加入企业",
        content: "企业中心后端已就绪。创建企业或通过邀请码、加入申请加入后，可使用企业知识库、共享智能体、成员协作和统一算力管理。",
        showCancel: false
      });
      return;
    }
    uni.showModal({
      title: current.tenantName || "企业中心",
      content: `成员状态：${current.memberStatus}\n认证状态：${current.certificationStatus}\n当前部门：${current.organizationName}\n当前角色：${roleLabels[current.currentRole]}\n企业中心完整页面将在 Figma 设计稿确认后接入。`,
      showCancel: false
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "企业认证加载失败", icon: "none" });
  }
}

async function showAboutBackend() {
  try {
    const health = await api<AnyRecord>("/api/v1/health");
    uni.showModal({
      title: "关于知启云AI",
      content: `版本 5.3.1 RC\n后端服务：${rowString(health, "service") || "xianzhi-ai"}\n状态：${rowString(health, "status") || "ok"}`,
      showCancel: false
    });
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "服务状态加载失败", icon: "none" });
  }
}

function v531ActionId(payload: unknown): string {
  if (typeof payload === "string") return payload;
  if (Array.isArray(payload)) {
    for (const item of payload) {
      const id = v531ActionId(item);
      if (id) return id;
    }
    return "";
  }
  if (!payload || typeof payload !== "object") return "";
  const record = payload as AnyRecord;
  const directId = rowString(record, "serviceId", "service-id", "id");
  if (directId) return directId;
  const detailId = v531ActionId(record.detail);
  if (detailId) return detailId;
  const currentTarget = record.currentTarget as AnyRecord | undefined;
  const target = record.target as AnyRecord | undefined;
  return (
    rowString(currentTarget?.dataset, "serviceId", "service-id", "id") ||
    rowString(target?.dataset, "serviceId", "service-id", "id")
  );
}

function handleV531ProfileService(payload: unknown) {
  const id = v531ActionId(payload);
  const protectedActions = new Map<string, "open_wallet" | "open_order" | "open_member_center" | "recharge" | "create_knowledge_base">([
    ["wallet", "open_wallet"], ["orders", "open_order"], ["membership", "open_member_center"],
    ["recharge", "recharge"], ["points", "open_wallet"], ["usage", "open_wallet"],
    ["projects", "open_member_center"], ["tasks", "open_member_center"], ["favorites", "open_member_center"],
    ["downloads", "open_member_center"], ["invite", "open_member_center"], ["company", "open_member_center"],
    ["knowledge", "create_knowledge_base"], ["ai-knowledge", "create_knowledge_base"], ["login", "open_member_center"],
  ]);
  const protectedAction = protectedActions.get(id);
  if (isGuest.value && protectedAction) {
    const pages = getCurrentPages();
    const page = pages[pages.length - 1] as { route?: string } | undefined;
    const route = page?.route ? `/${String(page.route).replace(/^\/+/, "")}` : rolePage("user", activeTab.value);
    void requireProtectedAction({ action: protectedAction, route, payload: { service: id }, resume: () => handleV531ProfileService(id) });
    return;
  }
  const actions: Record<string, () => void | Promise<void>> = {
    ai: () => selectUserTab("create"),
    recharge: () => openFeaturePage(miniProgramFeaturePages.userRechargePlans),
    membership: () => openFeaturePage(miniProgramFeaturePages.userRechargePlans),
    wallet: () => selectUserTab("wallet"),
    assets: () => selectUserTab("assets"),
    recent: () => selectUserTab("assets"),
    projects: () => openFeaturePage(`${miniProgramFeaturePages.userAssetsList}?view=projects`),
    tasks: openTaskRecords,
    favorites: () => openFeaturePage(`${miniProgramFeaturePages.userAssetsList}?filter=favorite`),
    downloads: () => openFeaturePage(`${miniProgramFeaturePages.userAssetsList}?filter=download`),
    points: () => openMineView("usage-details"),
    usage: () => openMineView("usage-details"),
    orders: () => openFeaturePage(miniProgramFeaturePages.userOrders),
    invoices: () => openFeaturePage(miniProgramFeaturePages.userInvoices),
    invite: () => openMineView("invite-promotion"),
    roles: () => openMineView("role-permissions"),
    team: () => openMineView("role-permissions"),
    company: () => openStandalonePage(miniProgramEnterprisePages.entry),
    "enterprise-overview": () => openStandalonePage(miniProgramEnterprisePages.overview),
    "enterprise-members": () => openStandalonePage(miniProgramEnterprisePages.members),
    "enterprise-organizations": () => openStandalonePage(miniProgramEnterprisePages.organizations),
    "enterprise-ai-employees": () => openStandalonePage(miniProgramEnterprisePages.aiEmployees),
    "enterprise-billing": () => openStandalonePage(miniProgramEnterprisePages.billing),
    "enterprise-roles": () => openStandalonePage(miniProgramEnterprisePages.roles),
    "enterprise-settings": () => openStandalonePage(miniProgramEnterprisePages.settings),
    messages: showNotifications,
    knowledge: () => openCreation("agent"),
    "ai-employees": () => openCreation("agent"),
    "customer-service": showHelpCenter,
    "ai-image": () => openCreation("image"),
    "ai-video": () => openCreation("video"),
    "ai-ppt": () => openCreation("ppt"),
    "ai-agent": () => openCreation("agent"),
    "ai-knowledge": () => openCreation("agent"),
    "ai-infographic": () => openCreation("infographic"),
    "upgrade-agent": () => hasAgentRole.value ? switchRole("agent") : openFeaturePage(miniProgramFeaturePages.userAgentDetail),
    "agent-promotion": () => selectAgentTab("promotion"),
    "agent-qrcode": () => selectAgentTab("promotion"),
    "agent-customers": () => selectAgentTab("customers"),
    "agent-commission": () => selectAgentTab("commission"),
    "agent-withdraw": () => openFeaturePage(miniProgramFeaturePages.agentWithdrawals),
    "agent-materials": () => selectAgentTab("promotion"),
    "operation-agents": () => replacePage(rolePage("operation", "agents")),
    "operation-orders": () => replacePage(rolePage("operation", "orders")),
    "operation-customers": () => replacePage(rolePage("operation", "agents")),
    "operation-reports": () => replacePage(rolePage("operation", "overview")),
    "operation-announcements": () => uni.showToast({ title: "公告管理入口即将开放", icon: "none" }),
    "operation-renew": () => void createOperationOrder(),
    coupons: () => uni.showToast({ title: "暂无可用优惠券", icon: "none" }),
    settings: () => openFeaturePage(miniProgramFeaturePages.userSettings),
    benefits: () => openFeaturePage(miniProgramFeaturePages.userRechargePlans),
    api: showApiSettingsSummary,
    recycle: showRecycleBinStatus,
    notifications: () => openFeaturePage(miniProgramFeaturePages.userSettings),
    security: () => openFeaturePage(miniProgramFeaturePages.userSettings),
    feedback: showFeedbackDialog,
    about: showAboutBackend,
    help: showHelpCenter,
    logout: confirmV531Logout,
  };
  const action = actions[id];
  if (!action) {
    uni.showToast({ title: id ? "当前入口暂不可用" : "服务入口未识别，请重试", icon: "none" });
    return;
  }
  void action();
}

function handleV531Benefit(payload: unknown) {
  const id = v531ActionId(payload);
  if (id === "member") openFeaturePage(miniProgramFeaturePages.userRechargePlans);
  else if (id === "company") openStandalonePage(miniProgramEnterprisePages.entry);
  else void showAboutBackend();
}

async function preloadCreationBackend(mode: CreationMode) {
  if (isGuest.value) return;
  try {
    if (mode === "ppt") {
      await resolvePptGenerationModels();
      return;
    }
    if (mode === "agent") {
      await api("/api/v1/knowledge-agents");
      return;
    }
    if (mode === "review") {
      await Promise.allSettled([
        api("/api/v1/knowledge-agents"),
        api("/api/v1/knowledge-conversations")
      ]);
      return;
    }
    const moduleCode = mode === "video" ? "video_generation" : "image_generation";
    await api(`/api/v1/module-schema?module_code=${encodeURIComponent(moduleCode)}`);
  } catch (error) {
    console.warn("[创作接口预检失败]", mode, error);
  }
}

function openCreation(mode: CreationMode) {
	if (!allowedCreationModes.value.includes(mode)) {
		uni.showToast({ title: "该能力暂未向小程序开放", icon: "none" });
		return;
	}
  openStandalonePage(miniProgramCreationPages[mode]);
  void preloadCreationBackend(mode);
}

function selectCreationMode(mode: CreationMode) {
	if (!allowedCreationModes.value.includes(mode)) {
		uni.showToast({ title: "该能力暂未向小程序开放", icon: "none" });
		return;
	}
  creationPromptDrafts.value[creationMode.value] = creationPrompt.value;
  openStandalonePage(miniProgramCreationPages[mode]);
  void preloadCreationBackend(mode);
}

function returnToCreationHub() {
  creationPromptDrafts.value[creationMode.value] = creationPrompt.value;
  returnToPreviousPage(rolePage("user", "create"));
}

function returnToPreviousPage(fallback: string) {
  const pages = getCurrentPages();
  if (pages.length > 1) {
    uni.navigateBack({
      delta: 1,
      fail: () => replacePage(fallback),
    });
    return;
  }
  replacePage(fallback);
}

function selectAgentTab(tab: TabId) {
  // Stay on the same workbench instance for overview/customers/commission/mine so we
  // don't reLaunch + re-download the heavy assets bundle before customers can paint.
  if (activeRole.value === "agent" && agentWorkbenchTabs.has(activeTab.value) && agentWorkbenchTabs.has(tab)) {
    if (tab === activeTab.value) return;
    activeTab.value = tab;
    if (!channelCenter.value) void loadRoleData("agent");
    return;
  }
  replacePage(rolePage("agent", tab));
}

let refreshQueued = false;

function clearAuthenticatedData() {
  token.value = "";
  auth.value = null;
  profile.value = null;
  wallet.value = null;
  pointAccountResponse.value = null;
  recentAssets.value = [];
  generationTasks.value = [];
  pageError.value = "";
}

async function refreshAll() {
  if (pageLoading.value) {
    refreshQueued = true;
    return;
  }
  readAuth();
  if (!token.value) {
    pageLoading.value = false;
    clearAuthenticatedData();
    userStore.reset();
    return;
  }
  pageLoading.value = true;
  pageError.value = "";
  try {
    const isWorksEntry = activeRole.value === "user" && activeTab.value === "assets";
    const profileTiming = beginWorksPerformanceStep("user_profile_initialization", {
      serialWait: !isWorksEntry,
      source: "MiniProgramRoleWorkbench.refreshAll",
      requestUrl: "/api/v1/user/profile",
    });
    await userStore.loadProfile(!isWorksEntry);
    profileTiming.end({
      cacheHit: isWorksEntry && userStore.loaded,
      note: isWorksEntry ? "cached_profile_allowed" : "forced_refresh",
    });
    const requestedRole = roleToAppRole[activeRole.value];
    if (!userStore.hasRole(requestedRole)) {
      uni.reLaunch({ url: `/pages/ForbiddenPage?role=${encodeURIComponent(requestedRole)}` });
      return;
    }
    if (userStore.currentRole !== requestedRole) {
      await userStore.switchRole(requestedRole);
    }
    if (activeRole.value === "user" && activeTab.value === "assets") {
      return;
    }
    if (activeRole.value === "agent" && !hasAgentRole.value) {
      uni.showToast({ title: "当前账号尚未开通代理商", icon: "none" });
      replacePage(rolePage("user", "home"));
      return;
    }
    if (activeRole.value === "operation" && !hasOperationRole.value) {
      uni.showToast({ title: "当前账号尚未开通运营中心", icon: "none" });
      replacePage(rolePage("user", "home"));
      return;
    }
    // Agent/operation: paint channel data first. Never wait on 作品 assets
    // (limit=4 can still be multi‑MB thumbnails and take several seconds).
    if (activeRole.value === "agent" || activeRole.value === "operation") {
      await loadRoleData(activeRole.value);
      pageLoading.value = false;
      void Promise.all([loadMemberProfile(), loadWallet()]);
      if (props.initialMineView === "invite-promotion" && hasAgentRole.value && !channelCenter.value) {
        void loadRoleData("agent");
      }
      return;
    }
    await Promise.all([loadMemberProfile(), loadWallet(), loadAssets(false)]);
    if (props.initialMineView === "invite-promotion" && hasAgentRole.value && !channelCenter.value) {
      await loadRoleData("agent");
    }
  } catch (error) {
    pageError.value = error instanceof Error ? error.message : "工作台加载失败";
  } finally {
    pageLoading.value = false;
    if (refreshQueued) {
      refreshQueued = false;
      void refreshAll();
    }
  }
}

async function loadMemberProfile() {
  profile.value = await businessSdk.roleWorkbench.memberProfile();
}

async function loadWallet() {
  const [walletResult, pointsResult, plansResult] = await Promise.allSettled([
    businessSdk.roleWorkbench.wallet(),
    businessSdk.roleWorkbench.pointsAccount(),
    api<AnyRecord[] | { items?: AnyRecord[] }>("/api/v1/plans?planType=recharge")
  ]);
  if (walletResult.status === "fulfilled") wallet.value = walletResult.value;
  if (pointsResult.status === "fulfilled") pointAccountResponse.value = pointsResult.value;
  if (plansResult.status === "fulfilled") rechargePackages.value = Array.isArray(plansResult.value) ? plansResult.value : listOf(plansResult.value.items);
}

async function loadAssets(showLoading = true) {
  if (!token.value) return;
  if (showLoading) assetsLoading.value = true;
  assetsError.value = "";
  try {
    const [assetResult, taskResult] = await Promise.allSettled([
      businessSdk.assets.listPage({ limit: 4, offset: 0 }),
      businessSdk.generation.listTaskPage({ limit: 5, offset: 0, prioritizeActive: true }),
    ]);
    if (assetResult.status === "rejected") throw assetResult.reason;
    const assetPayload = assetResult.value as unknown;
    const assetRecord = assetPayload && typeof assetPayload === "object" && !Array.isArray(assetPayload)
      ? assetPayload as AnyRecord
      : {};
    const assetSummary = assetRecord.summary && typeof assetRecord.summary === "object"
      ? assetRecord.summary as AnyRecord
      : {};
    const loadedAssets = collectionOf<Asset>(assetPayload);
    recentAssets.value = loadedAssets;
    assetTotal.value = asNumber(assetRecord.total, loadedAssets.length);
    assetMonthTotal.value = asNumber(assetSummary.monthTotal);
    assetFavoriteTotal.value = asNumber(assetSummary.favoriteTotal);
    assetStorageBytes.value = asNumber(assetSummary.storageBytes);
    if (taskResult.status === "fulfilled") {
      generationTasks.value = collectionOf<GenerationTask>(taskResult.value as unknown);
    }
  } catch (error) {
    assetsError.value = error instanceof Error ? error.message : "作品加载失败";
  } finally {
    if (showLoading) assetsLoading.value = false;
  }
}

async function loadRoleData(role: RoleId) {
  if (!token.value) return;
  if (role === "agent" && hasAgentRole.value) {
    try {
      channelCenter.value = await businessSdk.roleWorkbench.channelCenter();
    } catch (error) {
      channelCenter.value = null;
    }
  }
  if (role === "operation" && hasOperationRole.value) {
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

function requestWithdrawal() {
  openFeaturePage(miniProgramFeaturePages.agentWithdrawalApply);
}

function copyInviteLink() {
  if (!inviteLink.value) {
    uni.showToast({ title: "后端暂未生成推广链接", icon: "none" });
    return;
  }
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
  openFeaturePage(miniProgramFeaturePages.agentTeam);
}

function openFeaturePage(url: string) {
  if (!requestLogin("登录后可查看个人数据与账户功能")) return;
  openStandalonePage(url);
}

function openCustomerDetail(customer: AnyRecord) {
  const id = rowString(customer, "id") || rowString(customer, "userId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.agentCustomerDetail}?id=${encodeURIComponent(id)}`);
}

function openAssetDetail(asset: unknown) {
  if (!requestLogin("登录后可查看作品详情")) return;
  const id = rowString(asset, "id", "assetId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(id)}`);
}

function openUsageRecordDetail(record: AnyRecord) {
  const id = rowString(record, "id", "eventId", "taskId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.userUsageRecordDetail}?id=${encodeURIComponent(id)}`);
}

function openAgentCommissionDetail(commission: AnyRecord) {
  const id = rowString(commission, "id", "commissionId", "orderId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.agentCommissionDetail}?id=${encodeURIComponent(id)}`);
}

function openOperationAgentDetail(agent: AnyRecord) {
  const id = rowString(agent, "id", "agentId", "userId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.operationAgentDetail}?id=${encodeURIComponent(id)}`);
}

function openOperationOrderDetail(order: AnyRecord) {
  const id = rowString(order, "id", "orderId", "orderNo");
  if (id) openFeaturePage(`${miniProgramFeaturePages.operationOrderDetail}?id=${encodeURIComponent(id)}`);
}

function openOperationCommissionDetail(commission: AnyRecord) {
  const id = rowString(commission, "id", "commissionId", "orderId");
  if (id) openFeaturePage(`${miniProgramFeaturePages.operationCommissionDetail}?id=${encodeURIComponent(id)}`);
}

function handleGenerateTap() {
  if (!requestLogin("登录后可提交生成任务")) return;
  if (generationBusy.value) {
    uni.showToast({ title: "当前任务正在生成，请勿重复提交", icon: "none" });
    return;
  }
  const prompt = String(creationPrompt.value || "").trim();
  creationError.value = "";
  if (!prompt) {
    creationError.value = creationMode.value === "ppt" ? "请先输入演示文稿主题" : "请先输入创作需求";
    uni.showToast({ title: creationError.value, icon: "none" });
    return;
  }
  if (!(["image", "video", "ppt", "infographic"] as CreationMode[]).includes(creationMode.value)) {
    creationError.value = `${activeCreationName.value}暂未开放小程序生成`;
    uni.showToast({ title: creationError.value, icon: "none" });
    return;
  }

  void submitCreation(prompt);
}

let pendingLegalGenerationPrompt = "";

function handleLegalAcceptanceCompleted() {
  const prompt = pendingLegalGenerationPrompt.trim();
  pendingLegalGenerationPrompt = "";
  if (!prompt || generationBusy.value) return;
  creationError.value = "";
  void submitCreation(prompt);
}

type BackendGenerationConfig = {
  model: string;
  schema: AnyRecord;
};

function moduleSchemaFields(schema: AnyRecord) {
  const nested = schema.schema && typeof schema.schema === "object" && !Array.isArray(schema.schema)
    ? schema.schema as AnyRecord
    : {};
  const directFields = listOf(schema.fields);
  return directFields.length ? directFields : listOf(nested.fields);
}

function constrainedSchemaString(schema: AnyRecord, key: string, requested: string, fallback: string) {
  const field = moduleSchemaFields(schema).find(item => rowString(item, "key") === key);
  if (!field) return requested || fallback;
  const options = Array.isArray(field.options)
    ? field.options.map(item => typeof item === "string" ? item.trim() : "").filter(Boolean)
    : [];
  if (!options.length || options.includes(requested)) return requested || fallback;
  const defaultValue = rowString(field, "default");
  return options.includes(defaultValue) ? defaultValue : options[0] || fallback;
}

function constrainedSchemaNumber(schema: AnyRecord, key: string, requested: number, fallback: number) {
  const field = moduleSchemaFields(schema).find(item => rowString(item, "key") === key);
  let value = Number.isFinite(requested) && requested > 0 ? requested : fallback;
  if (!field) return value;
  const min = Number(field.min);
  const max = Number(field.max);
  if (Number.isFinite(min)) value = Math.max(value, min);
  if (Number.isFinite(max)) value = Math.min(value, max);
  return value;
}

async function resolveBackendGenerationConfig(
  mode: "image" | "video",
  fallback: string,
): Promise<BackendGenerationConfig> {
  const moduleCode = mode === "video" ? "video_generation" : "image_generation";
  const loadSchema = (modelName = "") => api<AnyRecord>(
    `/api/v1/module-schema?module_code=${encodeURIComponent(moduleCode)}${modelName ? `&model_name=${encodeURIComponent(modelName)}` : ""}`,
  );
  try {
    const schema = await loadSchema(fallback);
    return { model: rowString(schema, "model_name", "modelName") || fallback, schema };
  } catch (preferredModelError) {
    try {
      const schema = await loadSchema();
      const availableModel = rowString(schema, "model_name", "modelName");
      if (availableModel) {
        console.warn("[创作模型自动回退]", { moduleCode, fallback, availableModel, preferredModelError });
        return { model: availableModel, schema };
      }
    } catch (defaultModelError) {
      console.warn("[创作模型预检降级]", { moduleCode, fallback, preferredModelError, defaultModelError });
    }
    return { model: fallback, schema: {} };
  }
}

async function resolvePptGenerationModels() {
  const [textResult, imageResult] = await Promise.allSettled([
    api<AnyRecord[] | AnyRecord>("/api/v1/ppt/models/text"),
    api<AnyRecord[] | AnyRecord>("/api/v1/ppt/models/image"),
  ]);
  const modelRows = (result: PromiseSettledResult<AnyRecord[] | AnyRecord>) => {
    if (result.status !== "fulfilled") return [];
    if (Array.isArray(result.value)) return listOf(result.value);
    return listOf(result.value.items || result.value.rows || result.value.data);
  };
  return {
    textModel: rowString(modelRows(textResult)[0], "value", "model", "id") || pptModel.value.toLowerCase(),
    imageModel: rowString(modelRows(imageResult)[0], "value", "model", "id") || "default-image",
  };
}

async function submitCreation(prompt: string) {
  const startedAt = Date.now();
  generationSubmitting.value = true;
  generationProgress.value = 0;
  startGenerationFeedback(startedAt);
  latestGenerationTask.value = {
    id: "正在创建",
    title: `${activeCreationName.value}任务提交中`,
    status: "提交中",
    tone: "pending",
    progress: 0,
    resultType: creationMode.value,
  };
  try {
    // #ifdef MP-WEIXIN
    // Password/SMS login is fine; silently refresh device openid for WeChat content security only.
    try {
      await ensureWechatMiniProgramSession();
    } catch {
      throw new Error("内容安全检测暂不可用，请稍后重试");
    }
    // #endif
    let taskId = "";
    let taskStatus = "PENDING";
    let taskProgress = 0;
    if (creationMode.value === "ppt") {
      const models = await resolvePptGenerationModels();
      const result = await api<{ taskId?: string; id?: string; status?: string; progress?: number }>("/api/v1/ppt/generate", {
        method: "POST",
        body: JSON.stringify({
          prompt,
          slideCount: pptSlideCount.value,
          language: pptLanguage.value,
          tone: pptDynamic.value ? "dynamic" : "concise",
          theme: "business",
          autoThemeEnabled: true,
          enableWebSearch: false,
          textModel: models.textModel,
          imageSource: "ai",
          imageModel: models.imageModel
        })
      });
      taskId = String(result.taskId || result.id || "ppt-task");
      taskStatus = String(result.status || "PENDING").toUpperCase();
      taskProgress = clampGenerationProgress(result.progress);
    } else {
      const mode: "image" | "video" = creationMode.value === "video" ? "video" : "image";
      const referenceImages = await uploadCreationReferenceImages(creationReferencePaths.value);
      const generationConfig = await resolveBackendGenerationConfig(
        mode,
        activeCreationModel.value,
      );
      const requestedQuality = restoredCreationString("quality", "imageQuality") || (mode === "video" ? "720p" : "standard");
      const requestedSize = restoredCreationString("size", "aspectRatio", "aspect_ratio") || (mode === "video" ? "16:9" : "1024x1024");
      const result = await businessSdk.generation.createTask({
        mode,
        prompt,
        model: generationConfig.model,
        style: restoredCreationString("style", "stylePreset") || (mode === "video" ? "cinematic" : creationMode.value === "infographic" ? "infographic" : "commercial"),
        size: constrainedSchemaString(generationConfig.schema, mode === "video" ? "aspect_ratio" : "size", requestedSize, mode === "video" ? "16:9" : "1024x1024"),
        quality: constrainedSchemaString(generationConfig.schema, mode === "video" ? "resolution" : "quality", requestedQuality, mode === "video" ? "720p" : "standard"),
        count: constrainedSchemaNumber(generationConfig.schema, "n", restoredCreationCount(), 1),
        referenceImages,
        negativePrompt: restoredCreationString("negativePrompt", "negative_prompt"),
        duration: mode === "video"
          ? constrainedSchemaNumber(generationConfig.schema, "duration", rowNumber(restoredCreationParams.value, "duration"), 5)
          : undefined,
        parameters: restoredCreationParams.value,
      });
      taskId = String(result.id || "generation-task");
      taskStatus = String(result.status || "PENDING").toUpperCase();
      taskProgress = clampGenerationProgress(result.progress);
    }

    generationProgress.value = taskProgress;
    latestGenerationTask.value = {
      id: taskId,
      title: `${activeCreationName.value}生成中`,
      status: taskStatus,
      tone: ["FAILED", "ERROR"].includes(taskStatus) ? "danger" : ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(taskStatus) ? "success" : "pending",
      progress: taskProgress,
      resultType: creationMode.value,
    };
    persistActiveGeneration({
      id: taskId,
      mode: creationMode.value,
      prompt,
      status: taskStatus,
      progress: taskProgress,
      startedAt,
      inspirationTemplateId: activeInspirationTemplateId.value,
    });
    uni.showToast({ title: "任务已提交，正在生成", icon: "success" });
    void pollGenerationTask(taskId, creationMode.value, startedAt, prompt);
  } catch (error) {
    generationPollRun += 1;
    stopGenerationFeedback();
    generationProgress.value = 0;
    const rawMessage = error instanceof Error ? error.message : "生成任务创建失败";
    if ((error instanceof ApiClientError && error.statusCode === 428) || rawMessage.includes("请先确认最新版本")) {
      pendingLegalGenerationPrompt = prompt;
      creationError.value = "首次生成前，请先阅读并确认用户协议、隐私政策和 AI 生成内容使用规范";
      uni.showToast({ title: "请先确认必要协议，返回后将保留当前创作内容", icon: "none" });
      setTimeout(() => uni.navigateTo({ url: "/pages/user/ComplianceCenterPage" }), 300);
      return;
    }
    const message = rawMessage.includes("所发布内容含违规信息")
      ? "所发布内容含违规信息"
      : rawMessage.includes("内容安全检测暂不可用")
        ? "内容安全检测暂不可用，请稍后重试"
        : rawMessage;
    creationError.value = message;
    latestGenerationTask.value = { id: "-", title: "任务创建失败", status: message, tone: "danger" };
    const toastTitle =
      message === "所发布内容含违规信息" || message.includes("内容安全检测")
        ? message
        : "生成失败，请重试";
    uni.showToast({ title: toastTitle, icon: "none" });
  } finally {
    generationSubmitting.value = false;
  }
}

async function uploadCreationReferenceImage(filePath: string, index: number) {
  try {
    return await uploadReferenceImage(filePath);
  } catch (error) {
    const message = error instanceof Error ? error.message : "参考图上传失败";
    if (message.includes("所发布内容含违规信息")) throw new Error("所发布内容含违规信息");
    throw new Error(`第 ${index + 1} 张参考图上传失败：${message}`);
  }
}

async function uploadCreationReferenceImages(paths: string[]) {
  if (!paths.length || creationMode.value === "ppt") return [];
  const localPaths = paths.filter(path => !/^https?:\/\//i.test(path));
  if (localPaths.length) uni.showToast({ title: "正在上传参考图", icon: "loading", duration: 2000 });
  return Promise.all(paths.map((path, index) => /^https?:\/\//i.test(path) ? Promise.resolve(path) : uploadCreationReferenceImage(path, index)));
}

async function pollGenerationTask(
  taskId: string,
  mode: CreationMode,
  startedAt = Date.now(),
  prompt = creationPrompt.value,
) {
  const pollRun = ++generationPollRun;
  const maxAttempts = 600;
  const pollInterval = mode === "ppt" ? 3000 : 3000;
  let consecutiveErrors = 0;
  startGenerationFeedback(startedAt);

  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    if (attempt > 0) await new Promise(resolve => setTimeout(resolve, pollInterval));
    if (pollRun !== generationPollRun) return;

    try {
      let status = "PENDING";
      let progress = generationProgress.value;
      let resultId = "";
      let resultUrl = "";

      if (mode === "ppt") {
        const task = await api<AnyRecord>(`/api/v1/ppt/tasks/${encodeURIComponent(taskId)}`);
        if (pollRun !== generationPollRun) return;
        status = rowStatus(task).toUpperCase();
        const backendProgress = clampGenerationProgress(rowNumber(task, "progress"));
        progress = backendProgress || progress;
        resultId = rowString(task, "assetId", "resultId", "documentId");
        resultUrl = rowString(task, "outputUrl", "resultUrl", "downloadUrl");
      } else {
        const task = await api<GenerationTask>(`/api/v1/generation-tasks/${encodeURIComponent(taskId)}`);
        if (pollRun !== generationPollRun) return;
        const taskRecord = task as unknown as AnyRecord;
        status = String(task.status || "PENDING").toUpperCase();
        const backendProgress = clampGenerationProgress(task.progress ?? rowNumber(taskRecord, "progress"));
        progress = backendProgress || progress;
        const resultIds = Array.isArray(taskRecord.resultIds)
          ? taskRecord.resultIds.map(value => String(value || "").trim()).filter(Boolean)
          : [];
        resultId = resultIds[0] || rowString(taskRecord, "resultId", "assetId");
        resultUrl = rowString(taskRecord, "outputUrl", "resultUrl", "imageUrl", "thumbnailUrl");
      }

      consecutiveErrors = 0;
      creationError.value = "";
      const succeeded = ["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status);
      const failed = ["FAILED", "ERROR"].includes(status);
      if (succeeded) progress = 100;
      generationProgress.value = progress;
      latestGenerationTask.value = {
        id: taskId,
        title: succeeded ? `${creationNameForMode(mode)}生成成功` : failed ? `${creationNameForMode(mode)}生成失败` : `${creationNameForMode(mode)}生成中`,
        status,
        tone: failed ? "danger" : succeeded ? "success" : "pending",
        progress,
        resultId,
        resultUrl,
        resultType: mode,
      };

      if (succeeded || failed) {
        stopGenerationFeedback();
        if (succeeded) {
          const inspirationTemplateId = activeInspirationTemplateId.value;
          if (inspirationTemplateId) {
            activeInspirationTemplateId.value = "";
            void inspirationAPI.event(inspirationTemplateId, "generate_success", taskId);
          }
          await loadAssets(false);
          uni.showToast({ title: "生成完成", icon: "success" });
        } else {
          uni.showToast({ title: "生成失败，请检查后重试", icon: "none" });
        }
        return;
      }

      persistActiveGeneration({ id: taskId, mode, prompt, status, progress, startedAt });
    } catch (error) {
      if (pollRun !== generationPollRun) return;
      consecutiveErrors += 1;
      const message = error instanceof Error ? error.message : "任务状态同步失败";
      latestGenerationTask.value = {
        id: taskId,
        title: `${creationNameForMode(mode)}仍在后台生成`,
        status: "RETRYING",
        tone: "pending",
        progress: generationProgress.value,
        resultType: mode,
      };
      persistActiveGeneration({
        id: taskId,
        mode,
        prompt,
        status: "RETRYING",
        progress: generationProgress.value,
        startedAt,
      });
      if (consecutiveErrors === 1) {
        console.warn("[生成任务状态同步重试]", { taskId, message });
        uni.showToast({ title: "网络波动，任务仍在后台生成", icon: "none", duration: 2000 });
      }
    }
  }

  if (pollRun !== generationPollRun) return;
  latestGenerationTask.value = {
    id: taskId,
    title: `${creationNameForMode(mode)}仍在后台生成`,
    status: "PROCESSING",
    tone: "pending",
    progress: generationProgress.value,
    resultType: mode,
  };
  persistActiveGeneration({
    id: taskId,
    mode,
    prompt,
    status: "PROCESSING",
    progress: generationProgress.value,
    startedAt,
  });
  generationRepollTimer = setTimeout(() => {
    void pollGenerationTask(taskId, mode, startedAt, prompt);
  }, 15000);
}

function openLatestGenerationResult() {
  if (latestGenerationTask.value?.resultType === "ppt") {
    openPptEditor(latestGenerationTask.value.id);
    return;
  }
  const resultId = latestGenerationTask.value?.resultId;
  if (resultId) {
    openFeaturePage(`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(resultId)}`);
    return;
  }
  selectUserTab("assets");
}

function openPptEditor(taskId = "") {
  const query = taskId && taskId !== "-" ? `?taskId=${encodeURIComponent(taskId)}` : "";
  uni.navigateTo({ url: `/pages/user/UserPptEditorPage${query}` });
}

function previewLatestGenerationResult() {
  const url = latestGenerationTask.value?.resultUrl;
  if (!url) return;
  uni.previewImage({ current: url, urls: [url] });
}

function logout() {
  mineLogoutConfirm.value = false;
  clearAuthenticatedData();
  authStore.logout();
  uni.removeStorageSync("xianzhiMiniProgramAuth");
  uni.switchTab({ url: "/pages/user/UserHomePage" });
}

function confirmV531Logout() {
  uni.showModal({
    title: "退出登录",
    content: "退出后需要重新登录才能继续使用知启云 AI。",
    confirmText: "退出",
    confirmColor: "#D64545",
    success: result => {
      if (result.confirm) logout();
    },
  });
}

onMounted(() => {
  uni.$on("legal-acceptance-completed", handleLegalAcceptanceCompleted);
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramGenerate = guestAwareGenerateTap;
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramBackToCreation = returnToCreationHub;
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramChooseReference = chooseCreationReferenceImages;
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramAppendReferences = appendCreationReferencePaths;
  (globalThis as NativeGenerateBridge).__xianzhiMiniProgramSetReferenceSelecting = setCreationReferenceSelecting;
  if (props.initialCreationMode) {
    const inspirationDraft = readInspirationDraft(props.initialInspirationTemplateId);
    const savedPrompt = String(uni.getStorageSync("v531-creation-prompt") || "").trim();
    const rawStudioDraft = uni.getStorageSync("v532-studio-draft");
    const studioDraft = inspirationDraft
      ? {
          ...inspirationDraft.parameters,
          prompt: inspirationDraft.prompt,
          negativePrompt: inspirationDraft.negativePrompt,
          model: inspirationDraft.modelId,
          referenceImages: inspirationDraft.referenceAssets,
        }
      : rawStudioDraft && typeof rawStudioDraft === "object" ? rawStudioDraft as AnyRecord : {};
    if (inspirationDraft) activeInspirationTemplateId.value = inspirationDraft.templateId;
    const draftPrompt = rowString(studioDraft, "prompt");
    if (savedPrompt || draftPrompt) {
      creationPrompt.value = savedPrompt || draftPrompt;
      uni.removeStorageSync("v531-creation-prompt");
    }
    const draftReferences = Array.isArray(studioDraft.referencePaths)
      ? studioDraft.referencePaths
      : Array.isArray(studioDraft.referenceImages) ? studioDraft.referenceImages : [];
    if (draftReferences.length) {
      creationReferencePaths.value = draftReferences.filter((item): item is string => typeof item === "string" && Boolean(item));
    }
    restoredCreationParams.value = studioDraft;
    restoreActiveGeneration();
  }
  void loadTerminalCapabilities();
  void refreshAll();
});

watch(() => authStore.token, (nextToken, previousToken) => {
  if (nextToken === previousToken) return;
  if (!nextToken) clearAuthenticatedData();
  void refreshAll();
});

async function loadTerminalCapabilities() {
  try {
    const result = await api<{ creationModes?: string[] }>("/api/v1/public/terminal-capabilities");
    const allowed = (result.creationModes || []).filter((item): item is CreationMode =>
      (["image", "video", "ppt", "agent", "infographic", "review"] as string[]).includes(item),
    );
    allowedCreationModes.value = allowed.length ? allowed : ["image"];
    if (!allowedCreationModes.value.includes(creationMode.value)) creationMode.value = "image";
  } catch {
    allowedCreationModes.value = ["image", "infographic", "video"];
  }
}

onBeforeUnmount(() => {
  uni.$off("legal-acceptance-completed", handleLegalAcceptanceCompleted);
  generationPollRun += 1;
  stopGenerationFeedback(false);
  const bridge = globalThis as NativeGenerateBridge;
  if (bridge.__xianzhiMiniProgramGenerate === guestAwareGenerateTap) delete bridge.__xianzhiMiniProgramGenerate;
  if (bridge.__xianzhiMiniProgramBackToCreation === returnToCreationHub) delete bridge.__xianzhiMiniProgramBackToCreation;
  if (bridge.__xianzhiMiniProgramChooseReference === chooseCreationReferenceImages) delete bridge.__xianzhiMiniProgramChooseReference;
  if (bridge.__xianzhiMiniProgramAppendReferences === appendCreationReferencePaths) delete bridge.__xianzhiMiniProgramAppendReferences;
  if (bridge.__xianzhiMiniProgramSetReferenceSelecting === setCreationReferenceSelecting) delete bridge.__xianzhiMiniProgramSetReferenceSelecting;
});

onPullDownRefresh(() => {
  if (activeRole.value === "user" && activeTab.value === "assets") return;
  void refreshAll().finally(() => {
    uni.stopPullDownRefresh();
  });
});

onReachBottom(() => {
  // The enterprise asset center owns its paged list lifecycle.
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
      replacePage(miniProgramMinePages.overview);
      return true;
    }
  }
  if (isCreationDetail.value) {
    creationPromptDrafts.value[creationMode.value] = creationPrompt.value;
    if (getCurrentPages().length > 1) return false;
    replacePage(rolePage("user", "create"));
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
  font-weight: 600;
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
  font-weight: 600;
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

.runtime-error-banner {
  min-height: 54px;
  margin: 8px 20px 0;
  padding: 9px 12px;
  border-radius: 14px;
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
  font-weight: 600;
}

.home-command-title {
  margin-top: 7px;
  color: #111827;
  font-size: 23px;
  font-weight: 700;
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
  font-weight: 600;
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
  font-weight: 700;
}

.home-capability-name {
  margin-top: 11px;
  overflow: hidden;
  color: #111827;
  font-size: 13px;
  font-weight: 600;
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
  font-weight: 600;
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
  width: 100%;
  margin: 0;
  min-width: 0;
  padding: 12px 8px;
  border: 0;
  border-radius: 12px;
  background: #f7f8fc;
  line-height: 1.4;
  text-align: left;
}
.quick-item::after { display: none; }

.quick-value {
  display: block;
  min-height: 24px;
  font-size: 18px;
  font-weight: 600;
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
  font-weight: 600;
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
  font-weight: 600;
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
  font-weight: 600;
  color: #5a4db2;
}

.creation-name,
.recharge-points {
  margin-top: 8px;
  font-size: 13px;
  font-weight: 600;
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
  font-weight: 600;
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
  font-weight: 700;
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
  font-weight: 600;
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

.share-code-box {
  width: 168px;
  height: 168px;
  margin: 0 auto 14px;
  box-sizing: border-box;
  display: grid;
  place-items: center;
  border-radius: 18px;
  border: 1px solid #e5e7eb;
  background: #ffffff;
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
  font-weight: 700;
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

.bottom-tabs.mine-bottom-tabs {
  left: 0;
  right: 0;
  bottom: 0;
  gap: 8px;
  padding: 7px 16px calc(8px + env(safe-area-inset-bottom));
  border-right: 0;
  border-bottom: 0;
  border-left: 0;
  border-radius: 0;
  border-color: #eceef3;
  box-shadow: 0 -8px 24px rgba(23, 28, 56, 0.06);
  backdrop-filter: none;
}

.bottom-tabs.mine-bottom-tabs .tab-button {
  height: 54px;
  min-height: 54px;
  border-radius: 14px;
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
  font-weight: 600;
}

/* V3.1 mobile workbench */
.mini-workbench {
  padding: 8px 15px calc(110px + env(safe-area-inset-bottom));
  background: #f8faff;
}

.mini-workbench.user-v531-shell {
  min-height: 100vh;
  padding: 0;
  background: #f7f8fc;
}

.user-v531-shell .native-safe-note {
  height: var(--header-height, 64px);
  min-height: 0;
}

.user-v531-shell .role-content {
  margin-top: 0;
}

.native-safe-note {
  height: var(--header-padding-top, max(0px, env(safe-area-inset-top, 0px)));
  min-height: 0;
}

.business-header {
  display: flex;
  min-height: var(--navigation-bar-height, 44px);
  margin-top: 5px;
  padding-right: var(--capsule-right-space, 0px);
  box-sizing: border-box;
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
  font-weight: 600;
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
  display: flex;
  gap: 4px;
  margin-top: 8px;
  padding: 4px;
  border: 1px solid #e3e7f4;
  border-radius: 14px;
  background: #eef1f8;
  box-shadow: 0 6px 18px rgba(31, 41, 55, 0.05);
}

.role-switcher .role-pill {
  min-width: 0;
  height: 44px;
  margin: 0;
  padding: 0 12px;
  border: 0;
  border-radius: 10px;
  color: #697386;
  background: transparent;
  font-size: 13px;
  font-weight: 600;
  line-height: 44px;
}

.role-switcher .role-pill::after {
  border: 0;
}

.role-switcher .role-pill.active {
  color: #ffffff;
  background: linear-gradient(135deg, #7d8df6 0%, #6f68d9 100%);
  box-shadow: 0 5px 14px rgba(90, 77, 178, 0.22);
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
  position: relative;
  overflow: hidden;
  min-height: 156px;
  padding: 17px;
  box-sizing: border-box;
  border-radius: 12px;
  color: #ffffff;
  background: #15192d;
  box-shadow: 0 14px 30px rgba(23, 28, 56, 0.16);
}
.v31-hero-cover { position: absolute; z-index: 0; inset: 0; width: 100% !important; height: 100% !important; opacity: .34; }
.v31-tool-cover { position: relative; z-index: 1; width: 36px !important; height: 36px !important; flex: 0 0 36px; }
.v31-studio-banner { width: 100% !important; height: 118px !important; }

.v31-kicker {
  position: relative;
  z-index: 1;
  color: #aeb8ff;
  font-size: 11px;
  font-weight: 700;
}

.v31-hero-title {
  position: relative;
  z-index: 1;
  margin-top: 6px;
  font-size: 19px;
  font-weight: 700;
  line-height: 28px;
}

.v31-hero-copy {
  position: relative;
  z-index: 1;
  margin-top: 1px;
  color: #cdd5f5;
  font-size: 12px;
}

.v31-hero-row {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: 104px 88px minmax(90px, 1fr);
  gap: 10px;
  margin-top: 11px;
  align-items: center;
}

.v31-mini-metric {
  width: 100%;
  height: 58px;
  margin: 0;
  padding: 7px 9px;
  box-sizing: border-box;
  border: 0;
  border-radius: 8px;
  line-height: 1.4;
  text-align: left;
}
.v31-mini-metric::after { display: none; }

.v31-mini-metric.purple { background: #f4f3ff; }
.v31-mini-metric.orange { background: #fff7ed; }
.v31-mini-metric.purple .v31-metric-value { color: #5b55d6; }
.v31-mini-metric.orange .v31-metric-value { color: #ff6b1a; }

.v31-metric-value {
  font-size: 16px;
  font-weight: 700;
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
  font-weight: 600;
}

.v31-section-title {
  color: #111827;
  font-size: 15px;
  font-weight: 700;
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
  position: relative;
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
  font-weight: 700;
}

.v31-tool-icon.orange, .v31-menu-icon.orange { color: #ff6b1a; border-color: #ffe2cc; background: #fff7ed; }
.v31-tool-icon.green, .v31-menu-icon.green { color: #079455; border-color: #cbf5df; background: #ecfdf5; }
.v31-tool-icon.blue { color: #2563eb; border-color: #cfe1ff; background: #eff6ff; }

.v31-tool-copy {
  position: relative;
  z-index: 2;
  min-width: 0;
  flex: 1;
}

.v31-tool-name {
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-tool-desc {
  display: -webkit-box;
  margin-top: 3px;
  overflow: hidden;
  color: #697386;
  font-size: 10px;
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
  position: relative;
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
  position: relative;
  z-index: 1;
  display: flex;
  width: 100%;
  height: 86px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #5b55d6;
  background: #eef2ff;
  font-size: 25px;
  font-weight: 700;
}

.v31-preview.orange, .v31-work-preview.orange { color: #ff6b1a; background: #fff2e8; }
.v31-work-preview.green { color: #079455; background: #eafbf2; }

.v31-inspiration-title,
.v31-work-title {
  position: relative;
  z-index: 2;
  margin-top: 9px;
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.v31-card-footer {
  position: relative;
  z-index: 2;
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

.v31-reference-panel {
  padding: 15px;
  border: 1px solid #d9e0ff;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 8px 20px rgba(23, 28, 56, 0.06);
}

.v31-reference-head,
.v31-reference-title-row,
.v31-reference-loading,
.v31-reference-empty {
  display: flex;
  align-items: center;
}

.v31-reference-head { justify-content: space-between; gap: 12px; }
.v31-reference-copy { min-width: 0; flex: 1; }
.v31-reference-title-row { gap: 8px; }
.v31-reference-title { color: #111827; font-size: 15px; font-weight: 700; }
.v31-reference-mode { padding: 3px 8px; border-radius: 999px; color: #5a4db2; background: #eeedff; font-size: 10px; }
.v31-reference-description { display: block; margin-top: 4px; color: #697386; font-size: 10px; line-height: 16px; }

.v31-reference-add {
  width: auto;
  min-width: 58px;
  height: 30px;
  margin: 0;
  padding: 0 10px;
  border: 1px solid #c9d2ff;
  border-radius: 8px;
  color: #5a4db2;
  background: #f4f3ff;
  font-size: 11px;
  line-height: 28px;
}

.v31-reference-scroll { width: 100%; margin-top: 12px; white-space: nowrap; }
.v31-reference-row { display: inline-flex; padding-right: 4px; gap: 10px; }
.v31-reference-item { position: relative; width: 108px; height: 108px; flex: 0 0 108px; overflow: hidden; border: 1px solid #dfe5f2; border-radius: 10px; background: #eef1f8; }
.v31-reference-image { display: block; width: 108px; height: 108px; }
.v31-reference-source { position: absolute; left: 6px; bottom: 6px; padding: 3px 7px; border-radius: 999px; color: #ffffff; background: rgba(15, 23, 42, 0.72); font-size: 9px; }
.v31-reference-remove { position: absolute; top: 5px; right: 5px; width: 24px; height: 24px; margin: 0; padding: 0; border: 0; border-radius: 50%; color: #ffffff; background: rgba(15, 23, 42, 0.72); font-size: 17px; line-height: 22px; }
.v31-reference-add::after,
.v31-reference-remove::after,
.v31-reference-empty::after { display: none; }
.v31-reference-add:disabled,
.v31-reference-empty:disabled { opacity: 0.58; }

.v31-reference-loading {
  min-height: 86px;
  margin-top: 12px;
  padding: 10px;
  box-sizing: border-box;
  gap: 12px;
  border-radius: 10px;
  color: #697386;
  background: #f7f8fc;
  font-size: 11px;
}

.v31-reference-loading-image { width: 66px; height: 66px; flex: 0 0 66px; border-radius: 8px; background: #e7eaf3; }
.v31-reference-empty { width: 100%; min-height: 76px; margin: 12px 0 0; padding: 12px; box-sizing: border-box; gap: 10px; border: 1px dashed #c9d2ff; border-radius: 10px; color: #5a4db2; background: #f8f8ff; text-align: left; }
.v31-reference-empty-icon { display: grid; width: 36px; height: 36px; flex: 0 0 36px; place-items: center; border-radius: 9px; color: #ffffff; background: #7d8df6; font-size: 20px; }
.v31-reference-empty-title { display: block; font-size: 12px; font-weight: 600; }
.v31-reference-empty-copy { display: block; margin-top: 3px; color: #697386; font-size: 10px; }

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
.v31-subpage-title { display: block; color: #111827; font-size: 15px; font-weight: 700; }
.v31-subpage-copy { display: block; margin-top: 2px; color: #697386; font-size: 10px; }

.v31-prompt-title,
.v31-ppt-title {
  color: #0f172a;
  font-size: 18px;
  font-weight: 700;
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
  display: inline-flex;
  min-width: 106px;
  height: 30px;
  margin-left: auto;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.v31-button-spinner {
  width: 12px;
  height: 12px;
  box-sizing: border-box;
  border: 2px solid rgba(255, 255, 255, 0.45);
  border-top-color: #ffffff;
  border-radius: 50%;
  animation: v31-spin 0.8s linear infinite;
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
.v31-generation-result { width: 86px; height: 86px; flex: 0 0 86px; border-radius: 12px; background: #eef0f6; }
.v31-generation-summary { min-width: 0; flex: 1; }
.v31-generation-title-row { display: flex; min-width: 0; align-items: center; gap: 7px; }
.v31-live-badge { flex: 0 0 auto; padding: 2px 6px; border-radius: 999px; color: #4f46c7; background: #e5e7ff; font-size: 9px; font-weight: 600; }
.v31-generation-progress-track { width: 100%; height: 5px; margin-top: 9px; overflow: hidden; border-radius: 999px; background: #dedffc; }
.v31-generation-progress-value { height: 100%; border-radius: inherit; background: linear-gradient(90deg, #6f68d9, #7d8df6); transition: width 0.25s ease; }
.v31-generation-progress-value.indeterminate { width: 42%; animation: v31-progress 1.25s ease-in-out infinite; }
.v31-generation-feedback { display: block; margin-top: 6px; color: #5b55d6; font-size: 10px; line-height: 15px; }
.v31-generation-running { flex: 0 0 auto; color: #5b55d6; font-size: 11px; font-weight: 600; }

.v31-generation-state.success { border-color: #bdeecd; background: #ecfdf5; }
.v31-generation-state.danger { border-color: #fecaca; background: #fff1f2; }
.v31-generation-title { display: block; color: #111827; font-size: 13px; font-weight: 600; }
.v31-generation-meta { display: block; max-width: 230px; margin-top: 4px; overflow: hidden; color: #697386; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.v31-generation-state button { width: auto; min-width: 72px; height: 32px; margin: 0; padding: 0 10px; border-radius: 8px; color: #5b55d6; background: #ffffff; font-size: 11px; }

@keyframes v31-spin {
  to { transform: rotate(360deg); }
}

@keyframes v31-progress {
  0% { transform: translateX(-110%); }
  100% { transform: translateX(250%); }
}

.v31-workflow-card,
.v31-profile-hero {
  padding: 15px;
  border-radius: 12px;
  color: #ffffff;
  background: #15192d;
}

.v31-workflow-title { font-size: 15px; font-weight: 700; }
.v31-workflow-copy { margin-top: 4px; color: #cdd5f5; font-size: 12px; }
.v31-workflow-tags { display: flex; gap: 10px; margin-top: 9px; }
.v31-workflow-tags text { min-width: 76px; padding: 5px 8px; border-radius: 8px; background: #111827; color: #f8fafc; font-size: 10px; text-align: center; }
.v31-ppt-editor-entry { width: 100%; min-height: 40px; margin-top: 12px; border: 1px solid #dbe4f0; border-radius: 12px; background: #fff; color: #334155; font-size: 12px; font-weight: 700; }

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
.v31-draft-title { color: #111827; font-size: 15px; font-weight: 700; }
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
.v31-profile-name { font-size: 16px; font-weight: 700; }
.v31-profile-meta { margin-top: 4px; color: #cdd5f5; font-size: 11px; }
.v31-upgrade-button { grid-column: 2; width: 96px; height: 28px; margin: 0; color: #ff6b1a; border-radius: 8px; background: #fff2e8; font-size: 11px; line-height: 28px; }
.v31-wallet-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.v31-wallet-metric { padding: 12px; border: 1px solid #d8d5ff; border-radius: 8px; background: #f4f3ff; }
.v31-wallet-metric.orange { border-color: #ffe2cc; background: #fff7ed; }
.v31-wallet-metric text { display: block; color: #5b55d6; font-size: 16px; font-weight: 700; }
.v31-wallet-metric.orange text { color: #ff6b1a; }
.v31-wallet-metric text + text { margin-top: 4px; color: #697386; font-size: 10px; font-weight: 500; }
.v31-recharge-row button { min-width: 0; flex: 1; }
.v31-recharge-row button.primary { color: #5b55d6; border-color: #c9d2ff; background: #eef2ff; }
.v31-recharge-row button.orange { color: #ff6b1a; border-color: #ffd0b3; background: #fff7ed; }

.v31-menu-panel { display: flex; flex-direction: column; gap: 10px; padding: 11px; }
.v31-menu-panel > button { display: flex; width: 100%; min-height: 54px; margin: 0; padding: 10px; align-items: center; gap: 10px; border: 1px solid #e5eaf6; border-radius: 8px; background: #ffffff; text-align: left; }
.v31-menu-panel > button view { min-width: 0; flex: 1; }
.v31-menu-panel > button view text { display: block; color: #111827; font-size: 12px; font-weight: 600; }
.v31-menu-panel > button view text + text { margin-top: 3px; color: #697386; font-size: 10px; font-weight: 500; }
.v31-menu-panel > button.danger view text { color: #dc2626; }

.agent-v4-hero { padding: 15px; border: 1px solid #15192d; border-radius: 8px; color: #ffffff; background: #15192d; }
.agent-v4-hero-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.agent-v4-hero-top > view text { display: block; }
.agent-v4-hero-top > view text:first-child { color: #cdd5f5; font-size: 10px; }
.agent-v4-hero-top > view text:last-child { margin-top: 6px; font-size: 24px; font-weight: 700; }
.agent-v4-hero-top > text { padding: 5px 10px; border-radius: 999px; color: #ff6b1a; background: #fff2e8; font-size: 10px; font-weight: 700; }
.agent-v4-copy { display: block; margin-top: 8px; color: #b9c2db; font-size: 10px; line-height: 16px; }
.agent-v4-metrics { display: grid; margin-top: 12px; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 7px; }
.agent-v4-metrics button { width: 100%; min-width: 0; margin: 0; padding: 7px 8px; border: 0; border-radius: 8px; color: #fff; background: #23283d; line-height: 1.4; text-align: left; }
.agent-v4-metrics button::after { display: none; }
.agent-v4-metrics text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.agent-v4-metrics text:first-child { font-size: 12px; font-weight: 700; }
.agent-v4-metrics text:last-child { margin-top: 2px; color: #9fa8c2; font-size: 10px; }
.agent-v4-entry-card { display: flex; padding: 12px; flex-direction: column; gap: 9px; border: 1px solid #e5eaf6; border-radius: 8px; background: #ffffff; }
.agent-v4-entry-card > button { display: flex; width: 100%; min-height: 62px; margin: 0; padding: 10px; box-sizing: border-box; align-items: center; gap: 10px; border: 1px solid #e5eaf6; border-radius: 8px; background: #ffffff; text-align: left; }
.agent-v4-entry-card > button::after { display: none; }
.agent-v4-entry-card > button > view { min-width: 0; flex: 1; }
.agent-v4-entry-card > button > view text { display: block; color: #111827; font-size: 12px; font-weight: 600; }
.agent-v4-entry-card > button > view text + text { margin-top: 4px; overflow: hidden; color: #697386; font-size: 10px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.agent-v4-entry-card > button > text:last-child { color: #ff6b1a; font-size: 10px; font-weight: 700; }
.agent-v4-icon { display: grid; width: 34px; min-width: 34px; height: 34px; place-items: center; border-radius: 8px; font-size: 11px; font-weight: 700; }
.agent-v4-icon.purple { color: #5b55d6; background: #f1f0ff; }
.agent-v4-icon.green { color: #079455; background: #ecfdf3; }
.agent-v4-icon.orange { color: #ff6b1a; background: #fff2e8; }
.agent-v4-cta { width: 100%; height: 44px; margin: 0; border-radius: 8px; color: #ffffff; background: #ff6b1a; font-size: 13px; font-weight: 700; }
.agent-v4-cta::after { display: none; }

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

.guest-browse-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 0 16px 12px;
  padding: 13px 14px;
  border: 1px solid #cfd9ff;
  border-radius: 14px;
  background: #f5f7ff;
}
.guest-browse-copy { min-width: 0; flex: 1; }
.guest-browse-title { display: block; color: #1d2b56; font-size: 14px; font-weight: 700; }
.guest-browse-detail { display: block; margin-top: 4px; color: #697085; font-size: 11px; line-height: 1.5; }
.guest-browse-button { flex: 0 0 auto; min-width: 92px; margin: 0; padding: 0 12px; border: 0; border-radius: 10px; color: #fff; background: #4a6bff; font-size: 12px; line-height: 36px; }
.guest-browse-button::after { display: none; }

@media (max-width: 340px) {
  .v31-tool-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .v31-hero-row { grid-template-columns: 1fr 1fr; }
  .v31-orange-button { grid-column: 1 / -1; }
}
</style>
