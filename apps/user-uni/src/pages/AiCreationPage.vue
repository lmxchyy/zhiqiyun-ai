<template>
  <view v-if="!isLoggedIn && isRegisterRoute" class="login-shell register-shell">
    <view class="login-card">
      <image class="login-logo" :src="loginLogo" mode="aspectFit" />
      <text class="eyebrow">INVITE REGISTER</text>
      <text class="login-title">注册知启云 AI</text>
      <text class="login-copy">通过代理商邀请注册后，账号会自动绑定来源渠道，后续消费和佣金可在代理商后台追踪。</text>
      <view class="invite-chip">邀请码 {{ registerInviteCode || "未识别" }}</view>
      <view class="login-form">
        <label>
          <text>用户名</text>
          <input v-model="registerUsername" type="text" placeholder="请输入用户名" />
        </label>
        <label>
          <text>邮箱</text>
          <input v-model="registerEmail" type="text" placeholder="your@email.com" />
        </label>
        <label>
          <text>密码</text>
          <input v-model="registerPassword" type="password" placeholder="至少 8 位" />
        </label>
        <label>
          <text>确认密码</text>
          <input v-model="registerConfirmPassword" type="password" placeholder="再次输入密码" />
        </label>
        <button type="button" class="login-submit" @click="registerByInvite">注册并进入工作台</button>
        <button type="button" class="login-link-button" @click="goLogin">已有账号，去登录</button>
      </view>
    </view>
  </view>

  <view v-else-if="!isLoggedIn" class="login-shell">
    <view class="login-screen">
      <view class="login-bg-accent login-bg-accent-blue"></view>
      <view class="login-bg-accent login-bg-accent-warm"></view>

      <view class="login-statusbar">
        <text>9:41</text>
        <view class="mini-capsule" aria-label="微信小程序菜单">
          <view class="mini-dot"></view>
          <view class="mini-dot"></view>
          <view class="mini-dot"></view>
        </view>
      </view>

      <view class="login-brand">
        <image class="login-logo" :src="loginLogo" mode="aspectFit" />
        <text class="eyebrow">微信小程序入口</text>
      </view>

      <view class="login-heading">
        <text class="login-title">登录知启云 AI</text>
        <text class="login-copy">延续前端工作台的浅色系统风格，账号密码与微信授权都可进入。</text>
      </view>

      <view class="login-card">
        <view class="login-form">
          <label>
            <text>邮箱</text>
            <input v-model="loginEmail" type="text" placeholder="demo@xianzhi.ai" />
          </label>
          <label>
            <text>密码</text>
            <input v-model="loginPassword" type="password" placeholder="输入登录密码" />
          </label>
          <button type="button" class="login-submit" @click="login">登录</button>
          <view class="login-divider">
            <view></view>
            <text>或</text>
            <view></view>
          </view>
          <button type="button" class="wechat-login-button" :disabled="wechatLoginLoading" @click="loginWithWechatMiniProgram">
            <view class="wechat-login-icon" aria-hidden="true"></view>
            <text>{{ wechatLoginLoading ? "微信授权中..." : "微信小程序登录" }}</text>
          </button>
          <text class="login-agreement">登录即表示同意《用户协议》和《隐私政策》</text>
        </view>
      </view>

      <view class="login-helper">
        <view class="login-helper-accent"></view>
        <view>
          <text>统一账号入口</text>
          <text>登录后按账号权限进入当前可用的创作、管理或服务工作台。</text>
        </view>
      </view>
    </view>
  </view>

  <view
    v-else
    :class="[
      'app-shell',
      `workspace-${currentWorkspace}`,
      `module-${activeModule}`,
      {
        'canvas-mode': currentWorkspace === 'user' && activeModule === 'wirelessCanvas',
        'module-open': isModuleDrawerOpen
      }
    ]"
  >
    <button
      type="button"
      class="module-fab"
      aria-label="打开模块菜单"
      @click="isModuleDrawerOpen = true"
    >
      <text></text>
      <text></text>
      <text></text>
      <text></text>
    </button>
    <aside class="sidebar">
      <view class="brand-block">
        <image class="brand-logo" :src="xianzhiLogo" mode="aspectFit" />
        <text class="brand-title">{{ currentWorkspace === 'user' ? '知启云 AI' : workspaceTitle }}</text>
        <text class="brand-subtitle">{{ currentWorkspace === 'user' ? 'User Console' : currentWorkspace === 'agent' ? 'Agent Console' : 'Master SaaS Console' }}</text>
      </view>
      <view class="mobile-module-bar">
        <view class="current-module-card">
          <text class="current-module-label">当前模块</text>
          <text class="current-module-title">{{ currentModule.label }}</text>
        </view>
        <button class="more-button" @click="isModuleDrawerOpen = true">更多</button>
      </view>
      <view class="nav-list">
        <button
          v-for="item in sidebarModules"
          :key="item.id"
          :class="['nav-item', `nav-${item.id}`, { active: activeModule === item.id }]"
          @click="selectModule(item.id)"
        >
          {{ item.label }}
        </button>
        <button type="button" class="nav-item more-nav-button" @click.stop="isModuleDrawerOpen = true">更多</button>
        <button type="button" class="nav-item logout-nav-button" @click.stop="logout">退出</button>
      </view>
      <view v-if="currentWorkspace === 'user'" class="sidebar-plan-card">
        <text>当前套餐</text>
        <view>
          <text>专业版（年付）</text>
          <text>使用中</text>
        </view>
        <text>有效期至：2025-07-19</text>
        <view class="plan-progress"><view :style="{ width: planUsagePercent }"></view></view>
        <text>可用点数</text>
        <text><text>{{ quota.toLocaleString('zh-CN') }}</text> / {{ quotaTotal.toLocaleString('zh-CN') }}</text>
        <button type="button" @click="selectModule('membership')">去充值</button>
      </view>
      <text v-if="currentWorkspace === 'user'" class="sidebar-version">知启云 AI v1.9.0</text>
    </aside>
    <view
      v-if="isModuleDrawerOpen"
      class="mobile-sidebar-backdrop"
      @click="isModuleDrawerOpen = false"
    ></view>


    <view class="workspace">
      <scroll-view v-if="activeModule === 'dashboard'" class="workspace-scroll user-dashboard" scroll-y>
        <view class="dashboard-topbar">
          <view class="dashboard-left-head">
            <button type="button" class="dashboard-menu-button" @click="isModuleDrawerOpen = true">☰</button>
            <view>
              <text class="dashboard-breadcrumb">用户工作台 /</text>
              <text class="dashboard-current">用户首页</text>
            </view>
          </view>
          <view class="dashboard-search">
            <text>搜索作品、画布、功能...</text>
            <text>⌘K</text>
          </view>
          <view class="dashboard-user-actions">
            <button type="button" @click="refresh">↻</button>
            <button type="button" class="notification-button" @click="showDashboardNotifications">
              <text>铃</text>
              <text>12</text>
            </button>
            <view class="dashboard-user-menu">
              <button type="button" class="dashboard-user-trigger" @click.stop="isUserMenuOpen = !isUserMenuOpen">
                <view class="dashboard-avatar">{{ (currentUser?.name || '张小明').slice(0, 1) }}</view>
                <text>{{ currentUser?.name || '张小明' }}</text>
                <text>⌄</text>
              </button>
              <view v-if="isUserMenuOpen" class="dashboard-user-dropdown">
                <button type="button" @click.stop="logout">退出登录</button>
              </view>
            </view>
          </view>
        </view>

        <view class="dashboard-metrics">
          <view v-for="metric in dashboardMetrics" :key="metric.label" class="dashboard-metric-card">
            <view :class="['metric-icon', metric.tone]">{{ metric.icon }}</view>
            <view>
              <text>{{ metric.label }}</text>
              <text>{{ metric.value }}</text>
            </view>
            <view class="metric-card-foot">
              <text>{{ metric.hint }}</text>
              <button type="button" @click="selectModule(metric.module)">{{ metric.action }} →</button>
            </view>
          </view>
        </view>

        <view class="dashboard-main-grid">
          <view class="quick-create-panel">
            <view class="dashboard-section-head">
              <view>
                <text>快捷创作</text>
              </view>
            </view>
            <view class="quick-prompt-box" @click="selectModule('assets')">
              <text>在线生图入口已下线，历史生成作品可在作品中心继续查看和管理。</text>
              <text>作品中心</text>
            </view>
            <view class="quick-control-row">
              <view>
                <text>资产</text>
                <text>{{ selectedDashboardModelName }}</text>
              </view>
              <view>
                <text>作品</text>
                <text>{{ assets.length || 0 }} 个</text>
              </view>
              <view>
                <text>点数</text>
                <text>{{ quota.toLocaleString("zh-CN") }}</text>
              </view>
              <button type="button" class="upload-ref-button" @click="selectModule('apiSettings')">API 设置</button>
              <button type="button" class="quick-generate-button" @click="selectModule('assets')">查看作品</button>
            </view>
            <text class="recent-strip-title">最近作品</text>
            <view class="recent-strip">
              <button v-for="asset in recentDashboardAssets" :key="asset.id" type="button" @click="selectModule('assets')">
                <image v-if="asset.mediaType === 'image'" :src="asset.thumbnailUrl || asset.url" mode="aspectFill" />
                <view v-else>{{ asset.mediaType }}</view>
                <text>{{ asset.name }}</text>
                <text>{{ formatShortDate(asset.createdAt || asset.updatedAt) }}</text>
              </button>
              <view v-if="!recentDashboardAssets.length" class="recent-empty">暂无最近作品</view>
              <button type="button" class="all-canvas-button" @click="selectModule('assets')">
                <text>→</text>
                <text>全部作品</text>
              </button>
            </view>
          </view>

          <view class="todo-panel">
            <view class="dashboard-section-head compact">
              <view>
                <text>工作待办</text>
              </view>
              <button type="button" @click="selectModule('usage')">全部待办 →</button>
            </view>
            <view class="todo-list">
              <button v-for="item in dashboardTodos" :key="item.title" type="button" @click="selectModule(item.module)">
                <text class="todo-icon">{{ item.icon }}</text>
                <view>
                  <text>{{ item.title }}</text>
                  <text>{{ item.desc }}</text>
                </view>
                <text>{{ item.status }}</text>
              </button>
            </view>
          </view>
        </view>

        <view class="dashboard-lower-grid">
          <view class="recent-work-panel">
            <view class="dashboard-section-head">
              <view>
                <text>最近作品</text>
              </view>
              <button type="button" @click="selectModule('assets')">查看全部作品 →</button>
            </view>
            <view class="work-table">
              <view class="work-table-head">
                <text>作品</text>
                <text>模型</text>
                <text>消耗点数</text>
                <text>状态</text>
                <text>创建时间</text>
                <text>操作</text>
              </view>
              <view v-for="asset in recentDashboardAssets" :key="asset.id" class="work-row">
                <view class="work-info">
                  <image v-if="asset.mediaType === 'image'" :src="asset.thumbnailUrl || asset.url" mode="aspectFill" />
                  <view v-else class="work-file">{{ asset.mediaType }}</view>
                  <view>
                    <text>{{ asset.name }}</text>
                    <text>{{ asset.metadata?.resolution || '1920 × 1080' }}</text>
                  </view>
                </view>
                <text>{{ asset.metadata?.model || selectedDashboardModelName }}</text>
                <text>{{ asset.metadata?.pointCost || selectedDashboardModelCost }}</text>
                <text class="work-status">● 已完成</text>
                <text>{{ formatShortDate(asset.createdAt || asset.updatedAt) }}</text>
                <view class="work-actions">
                  <button type="button" @click="selectModule('assets')">⊙</button>
                  <button type="button" @click="selectModule('assets')">□</button>
                  <button type="button" @click="selectModule('assets')">↓</button>
                  <button type="button" @click="selectModule('assets')">…</button>
                </view>
              </view>
              <view v-if="!recentDashboardAssets.length" class="dashboard-empty-row">暂无作品。</view>
            </view>
          </view>

          <view class="usage-panel-dashboard">
            <view class="dashboard-section-head compact">
              <view>
                <text>使用记录</text>
              </view>
              <button type="button" @click="selectModule('usage')">近 7 天</button>
            </view>
            <text class="usage-axis-title">点数消耗（按模型）</text>
            <view class="usage-chart">
              <view v-for="day in dashboardUsageDays" :key="day.label" class="usage-day">
                <view class="usage-column">
                  <view v-for="segment in day.segments" :key="segment.tone" :class="['usage-segment', segment.tone]" :style="{ height: segment.height }"></view>
                </view>
                <text>{{ day.label }}</text>
              </view>
            </view>
            <view class="usage-legend">
              <text v-for="item in dashboardUsageLegend" :key="item.label" :class="item.tone">{{ item.label }}</text>
            </view>
            <view class="usage-summary">
              <text>总消耗 1,892 点</text>
              <button type="button" @click="selectModule('usage')">查看明细 →</button>
            </view>
          </view>
        </view>
      </scroll-view>

      <scroll-view v-else-if="activeModule === 'agentHome'" class="workspace-scroll" scroll-y>
        <view class="hero">
          <text class="eyebrow">{{ workspaceEyebrow }}</text>
          <text class="hero-title">{{ workspaceHeroTitle }}</text>
          <text class="hero-copy">{{ workspaceHeroCopy }}</text>
        </view>
        <view class="metric-grid">
          <view v-for="metric in metrics" :key="metric.label" class="metric-card">
            <text>{{ metric.label }}</text>
            <text class="metric-value">{{ metric.value }}</text>
          </view>
        </view>
        <view class="module-grid">
          <view v-for="item in sidebarModules.filter(module => module.id !== activeModule)" :key="item.id" class="module-card" @click="selectModule(item.id)">
            <text class="module-title">{{ item.label }}</text>
            <text>{{ item.description }}</text>
            <text class="module-status">{{ item.status }}</text>
          </view>
        </view>
      </scroll-view>
      <KnowledgeMiniChat v-else-if="activeModule === 'agents'" />
      <view v-else-if="activeModule === 'wirelessCanvas'" class="wireless-canvas-page">
        <view v-if="!wirelessCanvasFrameLoaded" class="wireless-canvas-loading" role="status" aria-live="polite">
          <text>无线画布加载中</text>
          <text>正在准备节点、资产库与工作流工具</text>
        </view>
        <iframe
          v-if="wirelessCanvasFrameSrc"
          class="wireless-canvas-frame"
          :class="{ 'is-loading': !wirelessCanvasFrameLoaded }"
          :src="wirelessCanvasFrameSrc"
          title="无线画布"
          loading="lazy"
          referrerpolicy="same-origin"
          allow="clipboard-read; clipboard-write; fullscreen"
          @load="handleWirelessCanvasFrameLoad"
        ></iframe>
      </view>

      <view v-else-if="activeModule === 'videoGeneration'" class="creation-page video-generation-page">
        <view class="topbar">
          <view class="topbar-brand">
            <image class="topbar-logo" :src="xianzhiLogo" mode="aspectFit" />
            <text class="brand-title">知启云 AI</text>
          </view>
          <view class="tabs">
            <text class="active tab-status-label">视频生成</text>
          </view>
          <view class="topbar-status">
            <text :class="['online-pill', models.length ? 'online' : 'offline']">
              {{ models.length ? "ONLINE" : "OFFLINE" }}
            </text>
            <text class="user-pill"><text class="user-badge">知</text>知启云 · 普通用户</text>
            <button type="button" class="logout-button" @click.stop="logout">退出</button>
          </view>
        </view>
        <view class="video-generation-blank"></view>
      </view>

      <view v-else-if="activeModule === 'ppt'" class="creation-page ppt-generation-page">
        <view class="topbar">
          <view class="topbar-brand">
            <image class="topbar-logo" :src="xianzhiLogo" mode="aspectFit" />
            <text class="brand-title">知启云 AI</text>
          </view>
          <view class="tabs">
            <text class="active tab-status-label">PPT文档生成</text>
          </view>
          <view class="topbar-status">
            <text :class="['online-pill', models.length ? 'online' : 'offline']">
              {{ models.length ? "ONLINE" : "OFFLINE" }}
            </text>
            <text class="user-pill"><text class="user-badge">知</text>知启云 · 普通用户</text>
            <button type="button" class="logout-button" @click.stop="logout">退出</button>
          </view>
        </view>
        <scroll-view class="workspace-scroll ppt-generation-scroll" scroll-y>
          <PptDocumentGeneration />
        </scroll-view>
      </view>

      <view v-else-if="activeModule === 'assets'" class="creation-page works-center-page">
        <view class="topbar works-admin-topbar">
          <view class="works-admin-titlebar">
            <button type="button" class="works-header-icon" aria-label="展开模块导航" @click="isModuleDrawerOpen = true">▦</button>
            <view class="works-header-title">
              <text>▣</text>
              <text>作品中心</text>
            </view>
          </view>
          <view class="works-admin-actions">
            <label class="works-header-search">
              <text>⌕</text>
              <input v-model.trim="worksSearchKeyword" placeholder="搜索当前模块" />
            </label>
            <button type="button" class="works-header-button" aria-label="刷新作品" @click="refresh">↻</button>
            <button type="button" class="works-size-button" @click="showWorksDensityMenu">{{ worksDensity === 'compact' ? '紧凑' : '默认' }}⌄</button>
            <text :class="['online-pill', models.length ? 'online' : 'offline']">
              API {{ models.length ? "ONLINE" : "OFFLINE" }}
            </text>
            <text class="user-pill"><text class="user-badge">知</text>知启云 · 普通用户</text>
          </view>
        </view>
        <scroll-view :class="['works-center-scroll', 'workspace-scroll', `density-${worksDensity}`]" scroll-y>
          <view class="works-hero">
            <view class="works-hero-copy">
              <text class="works-eyebrow">WORKS CENTER</text>
              <text class="works-title">作品中心</text>
              <text class="works-subtitle">集中管理 AI 生图、参考资产、收藏作品和可交付文件，不再混入创作输入区。</text>
            </view>
            <view class="works-hero-actions">
              <button type="button" class="works-primary-action" @click="selectModule('dashboard')">继续创作</button>
              <button type="button" @click="refresh">刷新作品</button>
            </view>
          </view>

          <view class="works-summary-grid">
            <view v-for="item in worksSummaryCards" :key="item.label" class="works-summary-card">
              <text>{{ item.label }}</text>
              <text>{{ item.value }}</text>
              <text>{{ item.hint }}</text>
            </view>
          </view>

          <view class="works-panel">
            <view class="works-panel-head">
              <view class="works-filter-tabs">
                <button
                  v-for="item in workStatusOptions"
                  :key="item.value"
                  type="button"
                  :class="{ active: worksStatusFilter === item.value }"
                  @click="worksStatusFilter = item.value"
                >
                  {{ item.label }}
                </button>
              </view>
              <view class="works-toolbar">
                <label class="works-search">
                  <text>⌕</text>
                  <input v-model.trim="worksSearchKeyword" type="text" placeholder="搜索作品、提示词、模型..." />
                </label>
                <view class="works-view-toggle">
                  <button type="button" :class="{ active: worksViewMode === 'grid' }" @click="worksViewMode = 'grid'">宫格</button>
                  <button type="button" :class="{ active: worksViewMode === 'table' }" @click="worksViewMode = 'table'">列表</button>
                </view>
              </view>
            </view>

            <view v-if="worksViewMode === 'grid' && userWorkCards.length" class="works-grid">
              <view
                v-for="entry in userWorkCards"
                :key="entry.id"
                :class="['works-card', workStatusClass(entry)]"
                @click="previewWorkEntry(entry)"
              >
                <view class="works-card-preview">
                  <image v-if="workThumbnail(entry)" :src="workThumbnail(entry)" mode="aspectFill" />
                  <view v-else :class="['works-card-placeholder', { 'is-running': isWorkRunning(entry) }]">
                    <text>{{ isWorkRunning(entry) ? '生成中' : 'AI' }}</text>
                  </view>
                  <text class="works-card-badge">{{ workStatusLabel(entry) }}</text>
                </view>
                <view class="works-card-body">
                  <text class="works-card-title">{{ entry.title }}</text>
                  <text class="works-card-desc">{{ entry.prompt || '暂无提示词' }}</text>
                  <view class="works-card-meta">
                    <text>{{ entry.model }}</text>
                    <text>{{ entry.resolution }}</text>
                    <text>{{ entry.pointCost }} 点</text>
                  </view>
                </view>
                <view class="works-card-actions">
                  <button type="button" @click.stop="previewWorkEntry(entry)">预览</button>
                  <button type="button" @click.stop="reuseWorkEntry(entry)">复用</button>
                  <button type="button" :disabled="!entry.assetUrl" @click.stop="downloadWorkEntry(entry)">下载</button>
                  <button type="button" @click.stop="toggleWorkFavorite(entry)">{{ isWorkFavorite(entry) ? '已收藏' : '收藏' }}</button>
                </view>
              </view>
            </view>

            <view v-else-if="worksViewMode === 'table' && userWorkCards.length" class="works-table">
              <view class="works-table-head">
                <text>作品</text>
                <text>状态</text>
                <text>模型</text>
                <text>尺寸</text>
                <text>消耗</text>
                <text>创建时间</text>
                <text>操作</text>
              </view>
              <view v-for="entry in userWorkCards" :key="entry.id" class="works-table-row">
                <view class="works-table-name">
                  <image v-if="workThumbnail(entry)" :src="workThumbnail(entry)" mode="aspectFill" />
                  <view v-else class="works-table-file">AI</view>
                  <view>
                    <text>{{ entry.title }}</text>
                    <text>{{ entry.prompt || entry.id }}</text>
                  </view>
                </view>
                <text>{{ workStatusLabel(entry) }}</text>
                <text>{{ entry.model }}</text>
                <text>{{ entry.resolution }}</text>
                <text>{{ entry.pointCost }} 点</text>
                <text>{{ formatShortDate(entry.createdAt || entry.updatedAt) }}</text>
                <view class="works-table-actions">
                  <button type="button" @click="previewWorkEntry(entry)">预览</button>
                  <button type="button" @click="reuseWorkEntry(entry)">复用</button>
                  <button type="button" :disabled="!entry.assetUrl" @click="downloadWorkEntry(entry)">下载</button>
                </view>
              </view>
            </view>

            <view v-else class="works-empty">
              <text>暂无匹配作品</text>
              <text>调整筛选条件，或先去 AI 生图创建新作品。</text>
              <button type="button" @click="selectModule('dashboard')">去创作</button>
            </view>
          </view>
        </scroll-view>
      </view>

      <scroll-view v-else-if="activeModule === 'apiSettings'" class="workspace-scroll api-settings-page" scroll-y>
        <view class="api-settings-shell">
          <view class="api-settings-hero">
            <view>
              <text class="eyebrow">API SETTINGS</text>
              <text class="api-settings-title">API 设置</text>
              <text class="api-settings-copy">管理平台地址、API Key 和模型列表。当前保存到本地浏览器，后续可接入后端统一生效。</text>
            </view>
            <view class="api-settings-status">
              <text>{{ apiProviders.length }} 个平台</text>
              <text>{{ apiProviderForm.status }}</text>
            </view>
          </view>

          <view class="api-settings-layout">
            <aside class="api-provider-sidebar">
              <view class="api-side-head">
                <text>平台列表</text>
                <view class="api-side-actions">
                  <button type="button" @click="openAddApiProviderPanel">新增平台</button>
                  <button type="button" @click="openRecommendedApiPanel">推荐 API</button>
                </view>
              </view>
              <button
                v-for="provider in apiProviders"
                :key="provider.id"
                type="button"
                :class="['api-provider-item', { active: provider.id === selectedApiProviderId }]"
                @click="selectApiProvider(provider.id)"
              >
                <text>{{ provider.name }}</text>
                <text>{{ provider.protocol }} · {{ provider.models.length }} 模型</text>
              </button>
              <view class="api-recommend-card">
                <text>推荐 API 独立查看</text>
                <text>推荐 API 只提供获取入口和默认地址，不会直接改动平台配置。</text>
              </view>
            </aside>

            <main class="api-settings-editor">
              <view v-if="apiSettingsMode === 'add'" class="api-create-page">
                <view class="api-editor-head">
                  <view>
                    <text>新增平台</text>
                    <text>只创建平台配置；推荐 API 请切换到独立推荐页查看</text>
                  </view>
                  <view class="api-editor-actions">
                    <button type="button" @click="cancelAddApiProvider">取消</button>
                    <button type="button" class="api-save-button" @click="confirmAddApiProvider">确认新增</button>
                  </view>
                </view>
                <view class="api-form-block">
                  <view class="api-block-head">
                    <text>新增平台选项</text>
                    <text>选择协议模板后，填写地址、端口、Key 和模型</text>
                  </view>
                  <view class="api-form-grid">
                    <label class="api-field full">
                      <text>平台类型</text>
                      <picker :range="apiProviderTemplateOptions" :value="apiTemplateIndex" @change="changeApiTemplate">
                        <view class="api-picker">{{ selectedApiTemplate.label }}</view>
                      </picker>
                    </label>
                    <label class="api-field">
                      <text>平台名称</text>
                      <input v-model="newApiProviderDraft.name" type="text" placeholder="例如：公司 OpenAI 中转" />
                    </label>
                    <label class="api-field">
                      <text>平台 ID</text>
                      <input v-model="newApiProviderDraft.id" type="text" placeholder="provider-id" />
                    </label>
                    <label class="api-field full">
                      <text>请求地址</text>
                      <input v-model="newApiProviderDraft.baseUrl" type="text" placeholder="https://api.example.com/v1" />
                    </label>
                    <label class="api-field">
                      <text>验证协议</text>
                      <picker :range="apiProtocolOptions" :value="newApiProtocolIndex" @change="changeNewApiProtocol">
                        <view class="api-picker">{{ newApiProviderDraft.protocol }}</view>
                      </picker>
                    </label>
                    <label class="api-field">
                      <text>鉴权方式</text>
                      <picker :range="apiAuthTypeOptions" :value="newApiAuthTypeIndex" @change="changeNewApiAuthType">
                        <view class="api-picker">{{ newApiProviderDraft.authType }}</view>
                      </picker>
                    </label>
                    <label class="api-field">
                      <text>文生图端口</text>
                      <input v-model="newApiProviderDraft.textToImagePath" type="text" placeholder="/v1/images/generations" />
                    </label>
                    <label class="api-field">
                      <text>图生图/编辑端口</text>
                      <input v-model="newApiProviderDraft.imageEditPath" type="text" placeholder="/v1/images/edits" />
                    </label>
                    <label class="api-field full">
                      <text>API Key</text>
                      <input v-model="newApiProviderDraft.apiKey" type="password" placeholder="sk-..." />
                    </label>
                    <label class="api-field full">
                      <text>模型列表</text>
                      <textarea v-model="newApiProviderDraft.modelText" placeholder="image-model, chat-model, video-model" />
                    </label>
                  </view>
                </view>
              </view>
              <view v-else-if="apiSettingsMode === 'recommend'" class="api-recommend-page">
                <view class="api-editor-head">
                  <view>
                    <text>推荐 API</text>
                    <text>这里只提供获取入口和默认地址，不会修改当前平台配置</text>
                  </view>
                  <view class="api-editor-actions">
                    <button type="button" @click="apiSettingsMode = 'edit'">返回配置</button>
                  </view>
                </view>
                <view class="api-recommend-grid">
                  <view v-for="item in recommendedApis" :key="item.id" class="api-recommend-option">
                    <text>{{ item.name }}</text>
                    <text>{{ item.description }}</text>
                    <text>{{ item.baseUrl }}</text>
                    <button type="button" @click="useRecommendedApi(item.id)">套用到新增平台</button>
                  </view>
                </view>
              </view>
              <view v-else class="api-edit-page">
                <view class="api-editor-head">
                <view>
                  <text>平台配置</text>
                  <text>配置基础信息、请求地址、API Key 和可用模型</text>
                </view>
                <view class="api-editor-actions">
                  <button type="button" class="api-danger-button" @click="deleteApiProvider">删除</button>
                  <button type="button" class="api-save-button" @click="saveApiSettings">保存</button>
                </view>
              </view>

              <view class="api-form-block">
                <view class="api-block-head">
                  <text>基本信息</text>
                  <text>平台显示名、唯一 ID 和请求地址</text>
                </view>
                <view class="api-form-grid">
                  <label class="api-field">
                    <text>平台名称</text>
                    <input v-model="apiProviderForm.name" type="text" placeholder="例如：ZMO / Modelscope / ComfyUI" />
                  </label>
                  <label class="api-field">
                    <text>平台 ID</text>
                    <input v-model="apiProviderForm.id" type="text" placeholder="provider-id" />
                  </label>
                  <label class="api-field full">
                    <text>请求地址</text>
                    <input v-model="apiProviderForm.baseUrl" type="text" placeholder="https://api.example.com/v1" />
                  </label>
                  <label class="api-field">
                    <text>协议类型</text>
                    <picker :range="apiProtocolOptions" :value="apiProtocolIndex" @change="changeApiProtocol">
                      <view class="api-picker">{{ apiProviderForm.protocol }}</view>
                    </picker>
                  </label>
                  <label class="api-field">
                    <text>状态</text>
                    <view class="api-readonly-field">{{ apiProviderForm.status }}</view>
                  </label>
                  <label class="api-field">
                    <text>鉴权方式</text>
                    <picker :range="apiAuthTypeOptions" :value="apiAuthTypeIndex" @change="changeApiAuthType">
                      <view class="api-picker">{{ apiProviderForm.authType }}</view>
                    </picker>
                  </label>
                  <label class="api-field">
                    <text>请求模式</text>
                    <picker :range="apiProxyModeOptions" :value="apiProxyModeIndex" @change="changeApiProxyMode">
                      <view class="api-picker">{{ apiProviderForm.proxyMode }}</view>
                    </picker>
                  </label>
                  <label class="api-field">
                    <text>超时秒数</text>
                    <input v-model="apiProviderForm.timeoutSeconds" type="number" placeholder="60" />
                  </label>
                </view>
              </view>

              <view class="api-form-block">
                <view class="api-block-head">
                  <text>密钥与模型</text>
                  <text>Key 不会在列表中完整展示；模型用逗号分隔</text>
                </view>
                <view class="api-form-grid">
                  <label class="api-field full">
                    <text>API Key</text>
                    <input v-model="apiProviderForm.apiKey" type="password" placeholder="sk-..." />
                  </label>
                  <label class="api-field full">
                    <text>可用模型</text>
                    <textarea v-model="apiProviderForm.modelText" placeholder="gpt-image-2, flux-kontext-pro, seedream-3" />
                  </label>
                  <label class="api-field full">
                    <text>备注</text>
                    <textarea v-model="apiProviderForm.notes" placeholder="例如：仅用于图片生成；支持 OpenAI 兼容 / Gemini / ComfyUI 工作流。" />
                  </label>
                </view>
                <view class="api-model-chips">
                  <text v-for="modelName in apiModelPreview" :key="modelName">{{ modelName }}</text>
                </view>
              </view>

              <view class="api-test-row">
                <button type="button" @click="testApiConnection">测试配置</button>
                <text>{{ apiTestMessage }}</text>
              </view>
              </view>
            </main>
          </view>
        </view>
      </scroll-view>
      <scroll-view v-else-if="activeModule === 'membership'" class="workspace-scroll membership-page" scroll-y :scroll-into-view="membershipScrollTarget">
        <view class="membership-shell">
          <view class="membership-head">
            <view>
              <text class="membership-title">选择身份、充值与订阅</text>
              <text class="membership-subtitle">身份包独立于充值和订阅，积分有效期为 2 年</text>
            </view>
            <view class="current-subscription-card">
              <view class="current-subscription-copy">
                <view class="current-subscription-title-row">
                  <text>当前订阅</text>
                  <text>♛</text>
                </view>
                <text>暂无订阅计划</text>
              </view>
              <view class="subscription-balance">
                <text class="balance-gem">◆</text>
                <text>{{ quota.toLocaleString("zh-CN") }}</text>
              </view>
              <view class="subscription-actions">
                <button type="button" class="recharge-button" @click="showMembershipPlans">充值</button>
                <button type="button" class="detail-button" @click="selectModule('usage')">明细</button>
              </view>
            </view>
          </view>

          <view class="identity-package-grid">
            <view v-for="pack in identityPackages" :key="pack.id" class="pricing-card identity-package-card">
              <view class="pricing-card-head">
                <text class="plan-name">{{ pack.name }}</text>
                <text class="recommended-badge">{{ pack.badge }}</text>
              </view>
              <view class="price-row">
                <text>￥</text>
                <text>{{ pack.price }}</text>
                <text>{{ pack.originalText }}</text>
              </view>
              <text class="price-note">{{ pack.note }}</text>
              <view class="credit-box">
                <view>
                  <text>{{ pack.tokenAmount.toLocaleString("zh-CN") }}</text>
                  <text>Token/点数权益</text>
                </view>
                <text>{{ pack.ruleText }}</text>
              </view>
              <button type="button" class="subscribe-button" @click="createIdentityOrder(pack)">{{ pack.actionText }}</button>
            </view>
          </view>

          <view class="billing-tabs">
            <button
              v-for="cycle in billingCycles"
              :key="cycle.id"
              type="button"
              :class="{ active: selectedBillingCycle === cycle.id }"
              @click="selectedBillingCycle = cycle.id"
            >
              <text>{{ cycle.label }}</text>
              <text>{{ cycle.discount }}</text>
            </button>
          </view>

          <view id="membership-plans" class="pricing-grid standard-pricing-grid">
            <view
              v-for="plan in standardMembershipPlanCards"
              :key="plan.id"
              :class="['pricing-card', { featured: plan.recommended }]"
            >
              <view class="pricing-card-head">
                <text class="plan-name">{{ plan.name }}</text>
                <text v-if="plan.recommended" class="recommended-badge">♧ 推荐方案</text>
              </view>
              <view class="price-row">
                <text>￥</text>
                <text>{{ plan.price }}</text>
                <text>￥{{ plan.originalPrice }}/{{ plan.unit }}</text>
              </view>
              <text class="price-note">{{ plan.note }}</text>
              <view class="credit-box">
                <view>
                  <text>{{ plan.credits.toLocaleString("zh-CN") }}</text>
                  <text>积分/{{ plan.creditUnit }}</text>
                </view>
                <text>最多生成{{ plan.images.toLocaleString("zh-CN") }}张图片 或 {{ plan.videos.toLocaleString("zh-CN") }}个视频 <text>?</text></text>
              </view>
              <button type="button" class="subscribe-button" @click="createMembershipOrder(plan)">{{ plan.custom ? "联系商务" : "立即订阅" }}</button>
              <view class="feature-list">
                <view v-for="feature in plan.features" :key="feature">
                  <text>✓</text>
                  <text>{{ feature }}</text>
                </view>
              </view>
            </view>
          </view>

          <view v-if="customMembershipPlanCards.length" class="pricing-grid custom-pricing-grid">
            <view
              v-for="plan in customMembershipPlanCards"
              :key="plan.id"
              class="pricing-card custom-pricing-card"
            >
              <view class="pricing-card-head">
                <text class="plan-name">{{ plan.name }}</text>
              </view>
              <view class="price-row">
                <text>￥</text>
                <text>{{ plan.price }}</text>
                <text>￥{{ plan.originalPrice }}/{{ plan.unit }}</text>
              </view>
              <text class="price-note">{{ plan.note }}</text>
              <view class="credit-box">
                <view>
                  <text>{{ plan.credits.toLocaleString("zh-CN") }}</text>
                  <text>积分/{{ plan.creditUnit }}</text>
                </view>
                <text>最多生成{{ plan.images.toLocaleString("zh-CN") }}张图片 或 {{ plan.videos.toLocaleString("zh-CN") }}个视频 <text>?</text></text>
              </view>
              <button type="button" class="subscribe-button" @click="createMembershipOrder(plan)">联系商务</button>
              <view class="feature-list">
                <view v-for="feature in plan.features" :key="feature">
                  <text>✓</text>
                  <text>{{ feature }}</text>
                </view>
              </view>
            </view>
          </view>
        </view>
      </scroll-view>
      <scroll-view v-else-if="isAgentDataModule(activeModule)" class="workspace-scroll channel-center" scroll-y>
        <view v-if="channelCenter" class="channel-layout">
          <view class="channel-hero">
            <view>
              <text class="eyebrow">CHANNEL CENTER</text>
              <text class="channel-title">{{ channelCenter.user.name }}的代理商中心</text>
              <text class="channel-copy">邀请码 {{ channelCenter.agent.inviteCode }} · L{{ channelCenter.agent.level }} · {{ channelCenter.agent.status }}</text>
            </view>
            <button type="button" class="channel-refresh" @click="refreshChannelCenter">刷新</button>
          </view>

          <view class="channel-metrics">
            <view class="channel-metric">
              <text>直属客户</text>
              <text>{{ channelCenter.summary.directCustomers }}</text>
            </view>
            <view class="channel-metric">
              <text>下级代理</text>
              <text>{{ channelCenter.summary.childAgents }}</text>
            </view>
            <view class="channel-metric">
              <text>累计佣金</text>
              <text>{{ money(channelCenter.summary.totalCommission) }}</text>
            </view>
            <view class="channel-metric">
              <text>可提现</text>
              <text>{{ money(channelCenter.summary.availableToWithdraw) }}</text>
            </view>
          </view>

          <view class="channel-panels">
            <view v-if="activeModule === 'agentCustomers'" class="channel-panel">
              <view class="panel-head">
                <text>客户绑定</text>
                <text>{{ channelCenter.customers.length }} 位</text>
              </view>
              <view v-for="customer in channelCenter.customers" :key="customer.id" class="channel-row">
                <view>
                  <text>{{ customer.name }}</text>
                  <text>{{ customer.email }} · {{ customer.planId || '-' }}</text>
                </view>
                <text>{{ customer.status }}</text>
              </view>
              <view v-if="!channelCenter.customers.length" class="channel-empty">暂无直属客户。</view>
            </view>

            <view v-if="activeModule === 'agentUsage'" class="channel-panel">
              <view class="panel-head">
                <text>消费明细</text>
                <text>{{ channelCenter.usageEvents?.length || 0 }} 笔</text>
              </view>
              <view v-for="event in channelCenter.usageEvents || []" :key="event.id" class="channel-row">
                <view>
                  <text>{{ event.taskId }}</text>
                  <text>{{ event.model }} · 扣 {{ event.pointCost }} 点 · {{ event.balanceBefore }} → {{ event.balanceAfter }}</text>
                </view>
                <text>{{ event.status }}</text>
              </view>
              <view v-if="!(channelCenter.usageEvents || []).length" class="channel-empty">暂无客户消费记录。</view>
            </view>

            <view v-if="activeModule === 'agentCommissions'" class="channel-panel">
              <view class="panel-head">
                <text>佣金记录</text>
                <text>{{ money(channelCenter.summary.pendingCommission) }} 待结算</text>
              </view>
              <view v-for="commission in channelCenter.commissions" :key="commission.id" class="channel-row">
                <view>
                  <text>{{ commission.orderId }}</text>
                  <text>{{ commission.status }} · {{ Math.round(commission.rate * 100) }}%</text>
                </view>
                <text>{{ money(commission.amountCents) }}</text>
              </view>
              <view v-if="!channelCenter.commissions.length" class="channel-empty">暂无佣金记录。</view>
            </view>

            <view v-if="activeModule === 'agentWithdrawals'" class="channel-panel">
              <view class="panel-head">
                <text>提现申请</text>
                <text>{{ money(channelCenter.summary.pendingWithdrawal) }} 审核中</text>
              </view>
              <view v-for="withdrawal in channelCenter.withdrawals" :key="withdrawal.id" class="channel-row">
                <view>
                  <text>{{ withdrawal.id }}</text>
                  <text>{{ withdrawal.status }}</text>
                </view>
                <text>{{ money(withdrawal.amountCents) }}</text>
              </view>
              <view v-if="!channelCenter.withdrawals.length" class="channel-empty">暂无提现申请。</view>
            </view>

            <view v-if="activeModule === 'agentChildren'" class="channel-panel">
              <view class="panel-head">
                <text>下级代理</text>
                <button v-if="canCreateChildAgent" type="button" class="channel-mini-button" @click="childAgentFormVisible = !childAgentFormVisible">
                  {{ childAgentFormVisible ? "收起" : "新增下级" }}
                </button>
                <text v-else>{{ channelCenter.children.length }} 个</text>
              </view>
              <view v-if="canCreateChildAgent && childAgentFormVisible" class="channel-child-form">
                <view class="channel-form-grid">
                  <label>
                    <text>姓名/名称</text>
                    <input v-model="childAgentForm.name" placeholder="例如：杭州业务员" />
                  </label>
                  <label>
                    <text>登录邮箱</text>
                    <input v-model="childAgentForm.email" placeholder="用于下级登录" />
                  </label>
                  <label>
                    <text>开通等级</text>
                    <picker :range="childAgentLevelOptions" range-key="label" :value="childAgentLevelIndex" @change="changeChildAgentLevel">
                      <view class="channel-picker">{{ childAgentLevelLabel }}</view>
                    </picker>
                  </label>
                  <label>
                    <text>邀请码</text>
                    <input v-model="childAgentForm.inviteCode" placeholder="留空自动生成" />
                  </label>
                </view>
                <button type="button" class="channel-submit-button" :disabled="childAgentCreating" @click="createChildAgent">
                  {{ childAgentCreating ? "开通中..." : "确认开通" }}
                </button>
              </view>
              <view v-for="agent in channelCenter.children" :key="agent.id" class="channel-row">
                <view>
                  <text>{{ agent.name || agent.id }}</text>
                  <text>{{ agent.inviteCode }} · L{{ agent.level }}</text>
                </view>
                <text>{{ agent.status }}</text>
              </view>
              <view v-if="!channelCenter.children.length" class="channel-empty">暂无下级代理。</view>
            </view>
          </view>
        </view>
        <view v-else class="module-detail">
          <text class="eyebrow">需要代理商身份</text>
          <text class="detail-title">当前账号无法进入代理商中心</text>
          <text class="detail-copy">请使用代理商演示账号 agent1@xianzhi.ai 登录。</text>
        </view>
      </scroll-view>
      <scroll-view v-else class="workspace-scroll" scroll-y>
        <view class="module-detail">
          <text class="eyebrow">{{ currentModule.status }}</text>
          <text class="detail-title">{{ currentModule.label }}</text>
          <text class="detail-copy">{{ currentModule.description }}</text>
          <view class="capability-list">
            <view v-for="capability in currentModule.capabilities" :key="capability" class="capability-item">
              <text>{{ capability }}</text>
            </view>
          </view>
          <text class="detail-note">该模块入口已恢复到 uni-app 工作台，下一步会继续把旧 Node API 对应能力迁移到 Go 服务。</text>
        </view>
      </scroll-view>
    </view>

    <view v-if="false" class="module-drawer-layer" @click="isModuleDrawerOpen = false">
      <view class="module-drawer" @click.stop>
        <view class="drawer-handle"></view>
        <view class="drawer-head">
          <view>
            <text class="eyebrow">MODULES</text>
            <text class="drawer-title">切换工作模块</text>
          </view>
          <button class="drawer-close" @click="isModuleDrawerOpen = false">关闭</button>
        </view>
        <view class="drawer-grid">
          <button
            v-for="item in sidebarModules"
            :key="item.id"
            :class="['drawer-module', { active: activeModule === item.id }]"
            @click="selectModule(item.id)"
          >
            <text class="drawer-module-title">{{ item.label }}</text>
            <text class="drawer-module-status">{{ item.status }}</text>
          </button>
          <button type="button" class="drawer-module logout-drawer-module" @click.stop="logout">
            <text class="drawer-module-title">退出</text>
            <text class="drawer-module-status">清除登录状态</text>
          </button>
        </view>
      </view>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { api, authService, businessSdk } from "../api/client";
import xianzhiLogo from "../assets/xianzhi-ai-logo.png";
import loginLogo from "../assets/zhiqiyun-logo-transparent.png";

declare const wx: { getSystemInfoSync?: () => { platform?: string; brand?: string } } | undefined;
import PptDocumentGeneration from "../components/PptDocumentGeneration.vue";
import KnowledgeMiniChat from "../components/KnowledgeMiniChat.vue";
import type { Asset, AuthResponse, AuthUser, ChannelAgent, ChannelCenterResponse, GenerationTask, ModelInfo, PointAccount } from "../types";
import type { UserDashboardResponse } from "@xianzhi/shared-types";

const rawEnv = (import.meta as unknown as { env?: Record<string, string | boolean | undefined> }).env || {};
const enableMockLogin = Boolean(rawEnv.DEV === true && String(rawEnv.VITE_ENABLE_MOCK_LOGIN || "").toLowerCase() === "true");

type Workspace = "user" | "agent" | "admin";
type ModuleId =
  | "dashboard"
  | "wirelessCanvas"
  | "videoGeneration"
  | "apiSettings"
  | "assets"
  | "ppt"
  | "agents"
  | "geo"
  | "membership"
  | "usage"
  | "admin"
  | "agentHome"
  | "agentCustomers"
  | "agentUsage"
  | "agentCommissions"
  | "agentWithdrawals"
  | "agentMaterials"
  | "agentChildren"
  | "agentSettings";

type ModuleConfig = {
  id: ModuleId;
  label: string;
  description: string;
  status: string;
  capabilities: string[];
};

type WorkStatusFilter = "all" | "done" | "running" | "failed" | "favorite";
type WorkViewMode = "grid" | "table";
type UserWorkEntry = {
  id: string;
  task?: GenerationTask;
  asset?: Asset;
  title: string;
  prompt: string;
  status: GenerationTask["status"];
  model: string;
  resolution: string;
  pointCost: number;
  assetUrl: string;
  thumbnailUrl: string;
  createdAt?: string;
  updatedAt?: string;
};
type ApiProviderSettings = {
  id: string;
  name: string;
  protocol: string;
  baseUrl: string;
  apiKey: string;
  models: string[];
  status: string;
  authType: string;
  timeoutSeconds: number | string;
  proxyMode: string;
  notes: string;
  textToImagePath?: string;
  imageEditPath?: string;
};

type ApiProviderForm = Omit<ApiProviderSettings, "models"> & {
  modelText: string;
};

type ApiProviderTemplate = {
  id: string;
  label: string;
  description: string;
  provider: ApiProviderSettings;
};

type BillingCycleId = "monthly" | "yearly" | "single";
type BillingCycle = {
  id: BillingCycleId;
  label: string;
  discount: string;
};
type MembershipPlan = {
  id: string;
  planIds: Record<BillingCycleId, string>;
  name: string;
  prices: Record<BillingCycleId, number>;
  originalPrices: Record<BillingCycleId, number>;
  credits: number;
  images: number;
  videos: number;
  concurrency: string;
  audience: string;
  custom?: boolean;
  oneTime?: boolean;
  recommended?: boolean;
};
type MembershipPlanCard = MembershipPlan & {
  price: number;
  originalPrice: number;
  planId: string;
  unit: string;
  creditUnit: string;
  note: string;
  features: string[];
};
type IdentityPackage = {
  id: string;
  planId: string;
  endpoint: string;
  name: string;
  badge: string;
  price: number;
  originalText: string;
  tokenAmount: number;
  note: string;
  ruleText: string;
  actionText: string;
};


const apiSettingsStorageKey = "xianzhi_api_provider_settings";
const apiProtocolOptions = ["OpenAI", "Gemini", "方舟", "Modelscope", "ComfyUI", "自定义"];
const apiAuthTypeOptions = ["Bearer Token", "API Key Header", "Query Key", "无鉴权"];
const apiProxyModeOptions = ["后端代理", "浏览器直连", "本地服务"];
const defaultApiProviders: ApiProviderSettings[] = [
  {
    id: "zmo-openai",
    name: "ZMO / OpenAI 兼容",
    protocol: "OpenAI",
    baseUrl: "https://api.zmoapi.cn/v1",
    apiKey: "",
    models: ["gpt-image-2", "flux-kontext-pro"],
    status: "待配置",
    authType: "Bearer Token",
    timeoutSeconds: 60,
    proxyMode: "后端代理",
    notes: "OpenAI 兼容协议，适合大多数第三方中转。",
    textToImagePath: "/v1/images/generations",
    imageEditPath: "/v1/images/edits"
  },
  {
    id: "modelscope",
    name: "Modelscope",
    protocol: "Modelscope",
    baseUrl: "https://api-inference.modelscope.cn/v1",
    apiKey: "",
    models: ["Qwen-Image", "Wan2.1"],
    status: "待配置",
    authType: "Bearer Token",
    timeoutSeconds: 90,
    proxyMode: "后端代理",
    notes: "Modelscope 图像模型，可按平台实际模型名调整。",
    textToImagePath: "/v1/images/generations",
    imageEditPath: "/v1/images/edits"
  },
  {
    id: "comfyui-local",
    name: "本地 ComfyUI",
    protocol: "ComfyUI",
    baseUrl: "http://127.0.0.1:8188",
    apiKey: "",
    models: ["workflow-default"],
    status: "本地",
    authType: "无鉴权",
    timeoutSeconds: 180,
    proxyMode: "本地服务",
    notes: "本地 ComfyUI 工作流服务，适合内网部署。",
    textToImagePath: "/prompt",
    imageEditPath: "/prompt"
  }
];

const apiProviderTemplates: ApiProviderTemplate[] = [
  {
    id: "openai-compatible",
    label: "OpenAI 兼容平台",
    description: "适合 ZMO、中转服务、自建 OpenAI 兼容网关。",
    provider: defaultApiProviders[0]
  },
  {
    id: "gemini",
    label: "Google Gemini",
    description: "适合 Gemini 图像和多模态模型，通常使用 API Key Header。",
    provider: {
      id: "gemini",
      name: "Google Gemini",
      protocol: "Gemini",
      baseUrl: "https://generativelanguage.googleapis.com/v1beta",
      apiKey: "",
      models: ["gemini-2.5-flash-image", "gemini-2.0-flash-preview-image-generation"],
      status: "待配置",
      authType: "API Key Header",
      timeoutSeconds: 90,
      proxyMode: "后端代理",
      notes: "参考 Infinite-Canvas 的多平台思路，按实际 Gemini 模型名维护。",
      textToImagePath: "/models/{model}:generateContent",
      imageEditPath: "/models/{model}:generateContent"
    }
  },
  {
    id: "ark",
    label: "火山方舟 Ark",
    description: "适合方舟大模型服务，常用于国内模型和图像能力接入。",
    provider: {
      id: "volc-ark",
      name: "火山方舟 Ark",
      protocol: "方舟",
      baseUrl: "https://ark.cn-beijing.volces.com/api/v3",
      apiKey: "",
      models: ["doubao-seedream-3-0-t2i", "doubao-seededit-3-0-i2i"],
      status: "待配置",
      authType: "Bearer Token",
      timeoutSeconds: 90,
      proxyMode: "后端代理",
      notes: "填写方舟 API Key 后，模型名按控制台实际 endpoint/model 配置。",
      textToImagePath: "/images/generations",
      imageEditPath: "/images/edits"
    }
  },
  {
    id: "modelscope",
    label: "Modelscope",
    description: "适合魔搭社区和 Qwen/Wan 系列图像模型。",
    provider: defaultApiProviders[1]
  },
  {
    id: "comfyui",
    label: "本地 ComfyUI",
    description: "适合本机或内网工作流服务，默认无鉴权、长超时。",
    provider: defaultApiProviders[2]
  },
  {
    id: "custom",
    label: "自定义平台",
    description: "从空白配置开始，自己填写协议、地址、鉴权和模型。",
    provider: {
      id: "custom-provider",
      name: "自定义平台",
      protocol: "自定义",
      baseUrl: "https://api.example.com/v1",
      apiKey: "",
      models: ["model-name"],
      status: "待配置",
      authType: "Bearer Token",
      timeoutSeconds: 60,
      proxyMode: "后端代理",
      notes: "",
      textToImagePath: "/v1/images/generations",
      imageEditPath: "/v1/images/edits"
    }
  }
];

const recommendedApis = [
  {
    id: "apimart",
    name: "APIMart",
    description: "Infinite-Canvas 中的获取 API 入口，适合快速申请 OpenAI 兼容 Key。",
    baseUrl: "https://api.apimart.ai/v1",
    templateId: "openai-compatible"
  },
  {
    id: "modelscope-cn",
    name: "ModelScope 国内站",
    description: "国内默认请求地址，适合魔搭中文站 Token。",
    baseUrl: "https://api-inference.modelscope.cn/v1",
    templateId: "modelscope"
  },
  {
    id: "modelscope-ai",
    name: "ModelScope 国际站",
    description: "海外默认请求地址，适合 modelscope.ai Token。",
    baseUrl: "https://api-inference.modelscope.ai/v1",
    templateId: "modelscope"
  }
];

function createApiProviderForm(provider: ApiProviderSettings): ApiProviderForm {
  return {
    ...provider,
    authType: provider.authType || "Bearer Token",
    timeoutSeconds: provider.timeoutSeconds || 60,
    proxyMode: provider.proxyMode || "后端代理",
    notes: provider.notes || "",
    textToImagePath: provider.textToImagePath || "/v1/images/generations",
    imageEditPath: provider.imageEditPath || "/v1/images/edits",
    modelText: provider.models.join(", ")
  };
}
const loginRoute = "/login";
const moduleRoutes: Record<ModuleId, string> = {
  dashboard: "/app",
  wirelessCanvas: "/app/wireless-canvas",
  videoGeneration: "/app/video-generation",
  apiSettings: "/app/api-settings",
  assets: "/app/works",
  ppt: "/app/ppt-generation",
  agents: "/app/agents",
  geo: "/app/geo",
  membership: "/app/membership",
  usage: "/app/usage",
  admin: "/admin/",
  agentHome: "/agent",
  agentCustomers: "/agent/customers",
  agentUsage: "/agent/usage",
  agentCommissions: "/agent/commissions",
  agentWithdrawals: "/agent/withdrawals",
  agentMaterials: "/agent/materials",
  agentChildren: "/agent/children",
  agentSettings: "/agent/settings"
};

const legacyRoutes: Record<string, ModuleId> = {
  "/dashboard": "dashboard",
  "/wireless-canvas": "wirelessCanvas",
  "/video-generation": "videoGeneration",
  "/api-settings": "apiSettings",
  "/works": "assets",
  "/ai-ppt": "ppt",
  "/ppt-generation": "ppt",
  "/app/ai-ppt": "ppt",
  "/agents": "agents",
  "/geo": "geo",
  "/membership": "membership",
  "/usage": "usage",
  "/channel": "agentHome"
};

const routeModules = Object.entries(moduleRoutes).reduce<Record<string, ModuleId>>((routes, [id, path]) => {
  routes[path] = id as ModuleId;
  return routes;
}, { ...legacyRoutes });

function hasBrowserWindow() {
  return typeof window !== "undefined" && typeof window.location !== "undefined";
}

function currentPathname() {
  return hasBrowserWindow() ? window.location.pathname : loginRoute;
}

function currentPath() {
  return currentPathname().replace(/\/$/, "") || "/";
}

function currentSearchParams() {
  return hasBrowserWindow() ? new URLSearchParams(window.location.search) : new URLSearchParams();
}

const userModules: ModuleConfig[] = [
  { id: "dashboard", label: "用户首页", description: "创作任务、积分、作品和常用能力入口。", status: "用户工作台", capabilities: ["创作概览", "积分状态", "作品入口"] },
  { id: "wirelessCanvas", label: "无线画布", description: "复刻 Infinite-Canvas 智能画布，支持节点、资产库、工作流和画布编排。", status: "源码复刻", capabilities: ["无限画布", "智能节点", "资产库", "工作流"] },
  { id: "videoGeneration", label: "视频生成", description: "视频生成入口，右侧工作区预留。", status: "待接入", capabilities: ["文生视频", "图生视频", "视频资产"] },
  { id: "ppt", label: "PPT文档生成", description: "输入主题生成 PPT 文档，预留任务、状态、历史和下载接口。", status: "Mock 可用", capabilities: ["主题输入", "页数语言", "联网搜索", "历史记录"] },
  { id: "assets", label: "作品中心", description: "管理生成资产、参考文件和可下载交付物。", status: "已接入 Go API", capabilities: ["资产列表", "图片预览", "作品归档"] },
  { id: "apiSettings", label: "API 设置", description: "管理生图平台、请求地址、API Key 和可用模型。", status: "本地配置", capabilities: ["平台列表", "请求地址", "Key 管理", "模型列表"] },
  { id: "agents", label: "智能体", description: "智能体创建、工作流编排、发布、调用与反馈。", status: "待迁移 Go API", capabilities: ["工作流节点", "知识库绑定", "版本回滚", "调用反馈"] },
  { id: "geo", label: "GEO 优化", description: "品牌监测、竞品分析、趋势报告和优化内容生成。", status: "待迁移 Go API", capabilities: ["品牌管理", "定时监测", "周报", "内容发布跟踪"] },
  { id: "membership", label: "身份/充值/订阅", description: "先选择会员、代理商或运营中心身份，再完成点数充值、套餐订阅和支付方式选择。", status: "待迁移 Go API", capabilities: ["身份开通", "会员套餐", "点数充值", "支付方式"] },
  { id: "usage", label: "使用记录", description: "按模型、时间和点数查看生成消耗明细。", status: "用户工作台", capabilities: ["点数消耗", "模型统计", "明细导出"] }
];

const agentModules: ModuleConfig[] = [
  { id: "agentHome", label: "代理首页", description: "代理商身份、邀请码、客户和收益总览。", status: "代理商中心", capabilities: ["邀请码", "收益总览", "客户概览"] },
  { id: "agentCustomers", label: "我的客户", description: "查看通过邀请关系绑定的直属客户。", status: "代理商中心", capabilities: ["客户绑定", "客户状态", "客户来源"] },
  { id: "agentUsage", label: "消费明细", description: "查看客户 AI 生图消费、扣点、模型和任务 ID。", status: "代理商中心", capabilities: ["任务 ID", "扣点记录", "余额变化"] },
  { id: "agentCommissions", label: "佣金明细", description: "查看订单分佣、结算状态和佣金金额。", status: "代理商中心", capabilities: ["订单分佣", "结算状态", "比例快照"] },
  { id: "agentWithdrawals", label: "提现管理", description: "查看提现申请、审核状态和可提现余额。", status: "代理商中心", capabilities: ["可提现", "提现记录", "审核状态"] },
  { id: "agentMaterials", label: "推广素材", description: "沉淀邀请码、推广话术和客户转化素材。", status: "待接入", capabilities: ["邀请链接", "推广话术", "素材包"] },
  { id: "agentChildren", label: "下级代理", description: "管理下级代理和团队结构。", status: "代理商中心", capabilities: ["代理树", "下级状态", "团队业绩"] },
  { id: "agentSettings", label: "账户设置", description: "代理商资料、收款信息和通知偏好。", status: "待接入", capabilities: ["资料", "收款信息", "通知"] }
];

const adminModules: ModuleConfig[] = [
  { id: "admin", label: "运营后台", description: "用户、收入、模型成本、代理审核、财务和系统设置。", status: "管理员", capabilities: ["用户状态", "模型供应商", "代理审核", "提现审核"] }
];

const workspaceModules: Record<Workspace, ModuleConfig[]> = {
  user: userModules,
  agent: agentModules,
  admin: adminModules
};

const moduleWorkspace = [...userModules.map(item => [item.id, "user"] as const), ...agentModules.map(item => [item.id, "agent"] as const), ...adminModules.map(item => [item.id, "admin"] as const)].reduce<Record<string, Workspace>>((items, [id, workspace]) => {
  items[id] = workspace;
  return items;
}, {});
const currentWorkspace = ref<Workspace>("user");
const activeModule = ref<ModuleId>("dashboard");
const tasks = ref<GenerationTask[]>([]);
const assets = ref<Asset[]>([]);
const models = ref<ModelInfo[]>([]);
const pointAccount = ref<PointAccount | null>(null);
const model = ref("gpt-image-2");
const worksSearchKeyword = ref("");
const worksStatusFilter = ref<WorkStatusFilter>("all");
const worksViewMode = ref<WorkViewMode>("grid");
const worksDensity = ref<"default" | "compact">("default");
const membershipScrollTarget = ref("");
const favoriteWorkIds = ref<string[]>([]);
const isLoggedIn = ref(false);
const currentUser = ref<AuthUser | null>(null);
const currentAgent = ref<ChannelAgent | null>(null);
const channelCenter = ref<ChannelCenterResponse | null>(null);
const childAgentFormVisible = ref(false);
const childAgentCreating = ref(false);
const childAgentForm = ref({ name: "", email: "", level: 1, inviteCode: "", available: 0 });
const loginEmail = ref("demo@xianzhi.ai");
const loginPassword = ref("Demo123!");
const wechatLoginLoading = ref(false);
const registerUsername = ref("");
const registerEmail = ref("");
const registerPassword = ref("");
const registerConfirmPassword = ref("");
const registerInviteCode = ref("");
const isModuleDrawerOpen = ref(false);
const isUserMenuOpen = ref(false);
const apiProviders = ref<ApiProviderSettings[]>(defaultApiProviders.map(provider => ({ ...provider })));
const selectedApiProviderId = ref(defaultApiProviders[0].id);
const apiProviderForm = ref<ApiProviderForm>(createApiProviderForm(defaultApiProviders[0]));
const apiTestMessage = ref("尚未测试配置");
const isAddingApiProvider = ref(false);
const newApiProviderTemplateId = ref(apiProviderTemplates[0].id);
const apiSettingsMode = ref<"edit" | "add" | "recommend">("edit");
const newApiProviderDraft = ref<ApiProviderForm>(createApiProviderForm(apiProviderTemplates[0].provider));
const selectedBillingCycle = ref<BillingCycleId>("monthly");
const wirelessCanvasSrc = "/static/smart-canvas.html?id=xianzhi-wireless-canvas&project=xianzhi";
const wirelessCanvasFrameSrc = ref("");
const wirelessCanvasFrameLoaded = ref(false);
let wirelessCanvasLoadTimer: ReturnType<typeof setTimeout> | null = null;

function clearWirelessCanvasLoadTimer() {
  if (!wirelessCanvasLoadTimer) return;
  clearTimeout(wirelessCanvasLoadTimer);
  wirelessCanvasLoadTimer = null;
}

function scheduleWirelessCanvasFrameLoad() {
  if (activeModule.value !== "wirelessCanvas") return;
  wirelessCanvasFrameLoaded.value = false;
  clearWirelessCanvasLoadTimer();
  if (wirelessCanvasFrameSrc.value) return;

  const loadFrame = () => {
    if (activeModule.value === "wirelessCanvas") {
      wirelessCanvasFrameSrc.value = wirelessCanvasSrc;
    }
  };

  if (typeof window === "undefined") {
    loadFrame();
    return;
  }

  wirelessCanvasLoadTimer = window.setTimeout(() => {
    wirelessCanvasLoadTimer = null;
    window.requestAnimationFrame(loadFrame);
  }, 80);
}

function handleWirelessCanvasFrameLoad() {
  wirelessCanvasFrameLoaded.value = true;
}

watch(activeModule, (moduleId) => {
  if (moduleId === "wirelessCanvas") {
    scheduleWirelessCanvasFrameLoad();
    return;
  }
  clearWirelessCanvasLoadTimer();
}, { immediate: true });

const billingCycles: BillingCycle[] = [
  { id: "monthly", label: "连续包月", discount: "7折" },
  { id: "yearly", label: "连续包年", discount: "65折" },
  { id: "single", label: "单月购买", discount: "8折" }
];

const membershipPlans: MembershipPlan[] = [
  {
    id: "trial",
    planIds: { monthly: "plan_free", yearly: "plan_free", single: "plan_free" },
    name: "体验版",
    prices: { monthly: 0, yearly: 0, single: 0 },
    originalPrices: { monthly: 0, yearly: 0, single: 0 },
    credits: 100,
    images: 10,
    videos: 0,
    concurrency: "1个",
    audience: "新用户体验"
  },
  {
    id: "basic",
    planIds: { monthly: "plan_month", yearly: "plan_basic_year", single: "plan_basic_single" },
    name: "Basic",
    prices: { monthly: 29, yearly: 299, single: 39 },
    originalPrices: { monthly: 39, yearly: 468, single: 49 },
    credits: 4000,
    images: 400,
    videos: 0,
    concurrency: "3个",
    audience: "个人、小商家"
  },
  {
    id: "pro",
    planIds: { monthly: "plan_pro", yearly: "plan_pro_year", single: "plan_pro_single" },
    name: "Pro",
    prices: { monthly: 139, yearly: 1499, single: 169 },
    originalPrices: { monthly: 169, yearly: 2028, single: 209 },
    credits: 20000,
    images: 2000,
    videos: 0,
    concurrency: "8个",
    audience: "内容创作者、电商运营",
    recommended: true
  },
  {
    id: "ultimate",
    planIds: { monthly: "plan_year", yearly: "plan_ultimate_year", single: "plan_ultimate_single" },
    name: "Ultimate",
    prices: { monthly: 699, yearly: 7999, single: 899 },
    originalPrices: { monthly: 899, yearly: 10788, single: 1129 },
    credits: 100000,
    images: 10000,
    videos: 0,
    concurrency: "20个",
    audience: "工作室、代运营团队"
  },
  {
    id: "ai_creator_996",
    planIds: { monthly: "plan_ai_creator_996", yearly: "plan_ai_creator_996", single: "plan_ai_creator_996" },
    name: "996 AI 创作会员包",
    prices: { monthly: 996, yearly: 996, single: 996 },
    originalPrices: { monthly: 996, yearly: 996, single: 996 },
    credits: 40000,
    images: 4000,
    videos: 0,
    concurrency: "8个",
    audience: "AI 创作者、代理首购客户",
    recommended: true,
    oneTime: true
  },
  {
    id: "enterprise",
    planIds: { monthly: "plan_enterprise", yearly: "plan_enterprise", single: "plan_enterprise" },
    name: "企业版",
    prices: { monthly: 0, yearly: 0, single: 0 },
    originalPrices: { monthly: 0, yearly: 0, single: 0 },
    credits: 0,
    images: 0,
    videos: 0,
    concurrency: "定制",
    audience: "企业客户、代理商",
    custom: true
  }
];

const identityPackages: IdentityPackage[] = [
  {
    id: "member_996",
    planId: "plan_ai_creator_996",
    endpoint: "/api/v1/orders/create",
    name: "996 AI 创作会员包",
    badge: "会员身份",
    price: 996,
    originalText: "含 400 元 Token",
    tokenAmount: 40000,
    note: "开通 Pro 会员权益，获得 400 元 AI 点数 / Token。",
    ruleText: "有推荐代理时自动分润。",
    actionText: "开通会员包"
  },
  {
    id: "agent_996",
    planId: "plan_agent_join_996",
    endpoint: "/api/v1/agent/join-order",
    name: "996 代理商开通包",
    badge: "代理身份",
    price: 996,
    originalText: "含 200 元 Token",
    tokenAmount: 20000,
    note: "开通代理商身份，生成邀请码和代理中心入口。",
    ruleText: "直属代理奖励 300 元，运营中心奖励 200 元。",
    actionText: "开通代理商"
  },
  {
    id: "operation_center_5000",
    planId: "plan_operation_center_5000",
    endpoint: "/api/v1/operation-center/join-order",
    name: "5000 运营中心开通包",
    badge: "运营身份",
    price: 5000,
    originalText: "平台运营权益",
    tokenAmount: 0,
    note: "开通运营中心身份，用于承接区域代理和订单分润。",
    ruleText: "开通费默认计入平台收入。",
    actionText: "开通运营中心"
  }
];

const sidebarModules = computed(() => {
  if (currentWorkspace.value === "admin") return workspaceModules.admin;
  if (!currentAgent.value) return workspaceModules.user;
  const existing = new Set<string>();
  const modules: ModuleConfig[] = [];
  for (const item of [...workspaceModules.user, ...workspaceModules.agent]) {
    if (existing.has(item.id)) continue;
    existing.add(item.id);
    modules.push(item);
  }
  return modules;
});
const currentModule = computed(() => sidebarModules.value.find(item => item.id === activeModule.value) || sidebarModules.value[0]);
const quota = computed(() => pointAccount.value?.available || 0);
const quotaTotal = computed(() => Math.max(pointAccount.value?.total || 0, quota.value + (pointAccount.value?.frozen || 0), 1));
const canCreateChildAgent = computed(() => Number(channelCenter.value?.agent?.level || currentAgent.value?.level || 0) >= 3);
const childAgentLevelOptions = computed(() => {
  const level = Number(channelCenter.value?.agent?.level || currentAgent.value?.level || 0);
  if (level === 3) return [{ value: 1, label: "L1 推广员/业务员" }, { value: 2, label: "L2 初级代理商" }];
  if (level === 4) return [{ value: 1, label: "L1 推广员/业务员" }, { value: 2, label: "L2 初级代理商" }, { value: 3, label: "L3 高级代理商" }];
  if (level >= 5) return [{ value: 1, label: "L1 推广员/业务员" }, { value: 2, label: "L2 初级代理商" }, { value: 3, label: "L3 高级代理商" }, { value: 4, label: "L4 城市合伙人" }];
  return [];
});
const childAgentLevelIndex = computed(() => Math.max(0, childAgentLevelOptions.value.findIndex(option => option.value === childAgentForm.value.level)));
const childAgentLevelLabel = computed(() => childAgentLevelOptions.value[childAgentLevelIndex.value]?.label || "请选择等级");
const apiProtocolIndex = computed(() => Math.max(0, apiProtocolOptions.indexOf(apiProviderForm.value.protocol)));
const apiAuthTypeIndex = computed(() => Math.max(0, apiAuthTypeOptions.indexOf(apiProviderForm.value.authType)));
const apiProxyModeIndex = computed(() => Math.max(0, apiProxyModeOptions.indexOf(apiProviderForm.value.proxyMode)));
const apiProviderTemplateOptions = computed(() => apiProviderTemplates.map(item => item.label));
const selectedApiTemplate = computed(() => apiProviderTemplates.find(item => item.id === newApiProviderTemplateId.value) || apiProviderTemplates[0]);
const apiTemplateIndex = computed(() => Math.max(0, apiProviderTemplates.findIndex(item => item.id === newApiProviderTemplateId.value)));
const newApiProtocolIndex = computed(() => Math.max(0, apiProtocolOptions.indexOf(newApiProviderDraft.value.protocol)));
const newApiAuthTypeIndex = computed(() => Math.max(0, apiAuthTypeOptions.indexOf(newApiProviderDraft.value.authType)));
const apiModelPreview = computed(() => parseApiModels(apiProviderForm.value.modelText));
const recentDashboardAssets = computed(() => [...assets.value]
  .sort((a, b) => new Date(b.createdAt || b.updatedAt || 0).getTime() - new Date(a.createdAt || a.updatedAt || 0).getTime())
  .slice(0, 5));
const workStatusOptions: Array<{ label: string; value: WorkStatusFilter }> = [
  { label: "全部作品", value: "all" },
  { label: "已完成", value: "done" },
  { label: "生成中", value: "running" },
  { label: "失败", value: "failed" },
  { label: "收藏", value: "favorite" }
];
const generationTaskMap = computed(() => {
  const map = new Map<string, GenerationTask>();
  tasks.value.forEach(task => {
    if (task.id) map.set(task.id, task);
  });
  return map;
});
const assetById = computed(() => {
  const map = new Map<string, Asset>();
  assets.value.forEach(asset => {
    if (asset.id) map.set(asset.id, asset);
  });
  return map;
});
const firstAssetByTaskId = computed(() => {
  const map = new Map<string, Asset>();
  assets.value.forEach(asset => {
    if (asset.taskId && !map.has(asset.taskId)) map.set(asset.taskId, asset);
  });
  return map;
});
const userWorkEntries = computed<UserWorkEntry[]>(() => {
  const taskEntries = tasks.value.map(task => createWorkEntryFromTask(task));
  const taskIds = new Set(taskEntries.map(item => item.asset?.id).filter(Boolean));
  const assetOnlyEntries = assets.value
    .filter(asset => !asset.taskId && !taskIds.has(asset.id))
    .map(asset => createWorkEntryFromAsset(asset));
  return [...taskEntries, ...assetOnlyEntries].sort((a, b) => workDateMs(b) - workDateMs(a));
});
const userWorkCards = computed(() => {
  const keyword = worksSearchKeyword.value.trim().toLowerCase();
  const statusFilter = worksStatusFilter.value;
  return userWorkEntries.value.filter(entry => {
    if (statusFilter === "done" && !isWorkSucceeded(entry)) return false;
    if (statusFilter === "running" && !isWorkRunning(entry)) return false;
    if (statusFilter === "failed" && !isWorkFailed(entry)) return false;
    if (statusFilter === "favorite" && !isWorkFavorite(entry)) return false;
    if (!keyword) return true;
    return [entry.title, entry.prompt, entry.model, entry.status, entry.id]
      .map(item => String(item || "").toLowerCase())
      .join(" ")
      .includes(keyword);
  });
});
const worksSummaryCards = computed(() => {
  const total = userWorkEntries.value.length;
  const done = userWorkEntries.value.filter(isWorkSucceeded).length;
  const favorites = userWorkEntries.value.filter(isWorkFavorite).length;
  const points = userWorkEntries.value.reduce((sum, entry) => sum + Number(entry.pointCost || 0), 0);
  return [
    { label: "全部作品", value: total.toLocaleString("zh-CN"), hint: "含图片任务与收藏记录" },
    { label: "已完成", value: done.toLocaleString("zh-CN"), hint: "可预览、下载、复用" },
    { label: "收藏作品", value: favorites.toLocaleString("zh-CN"), hint: "已加入收藏夹" },
    { label: "消耗点数", value: points.toLocaleString("zh-CN"), hint: "按当前任务汇总" }
  ];
});
const selectedDashboardModel = computed(() => models.value.find(item => item.code === model.value) || models.value[0]);
const selectedDashboardModelName = computed(() => selectedDashboardModel.value?.name || selectedDashboardModel.value?.code || "默认模型");
const selectedDashboardModelCost = computed(() => {
  const item = selectedDashboardModel.value;
  const direct = item?.pointCost || item?.fixedQuota;
  return typeof direct === "number" && Number.isFinite(direct) && direct > 0 ? direct : 1;
});
const planUsagePercent = computed(() => `${Math.min(100, Math.max(6, Math.round((quota.value / quotaTotal.value) * 100)))}%`);
const membershipPlanCards = computed<MembershipPlanCard[]>(() => membershipPlans.map(plan => {
  const cycle = selectedBillingCycle.value;
  const price = plan.prices[cycle];
  const originalPrice = plan.originalPrices[cycle];
  const planId = plan.planIds[cycle];
  const unit = plan.oneTime ? "次" : cycle === "yearly" ? "年" : "月";
  const creditUnit = plan.oneTime ? "一次性" : plan.id === "trial" ? "7天" : "月";
  const credits = plan.credits;
  const images = plan.images;
  const videos = plan.videos;
  const cycleNote = plan.oneTime
    ? `一次性 ￥${price}，开通 Pro 会员并发放 ${credits.toLocaleString("zh-CN")} 点。`
    : cycle === "yearly"
    ? `连续包年 ￥${price}，每月发放 ${credits.toLocaleString("zh-CN")} 点。`
    : cycle === "single"
      ? `单月购买 ￥${price}，本月发放 ${credits.toLocaleString("zh-CN")} 点。`
      : `连续包月 ￥${price}，每月发放 ${credits.toLocaleString("zh-CN")} 点。`;
  const note = plan.custom ? "企业定制价格、点数和并发。" : plan.id === "trial" ? "免费体验，100 点，7 天有效。" : cycleNote;

  return {
    ...plan,
    price,
    originalPrice,
    planId,
    unit,
    creditUnit,
    credits,
    images,
    videos,
    note,
    features: [
      `每${creditUnit}${credits.toLocaleString("zh-CN")}积分`,
      `适合：${plan.audience}`,
      `并发任务：${plan.concurrency}`,
      "下载权限：无水印下载",
      plan.custom ? "专属模型路由与商务支持" : "生成内容可商用"
    ]
  };
}));
const standardMembershipPlanCards = computed(() => membershipPlanCards.value.filter(plan => !plan.custom));
const customMembershipPlanCards = computed(() => membershipPlanCards.value.filter(plan => plan.custom));
const configuredApiCount = computed(() => apiProviders.value.filter(item => item.apiKey).length || Math.min(3, apiProviders.value.length));
const dashboardMetrics = computed<Array<{ label: string; value: string | number; hint: string; icon: string; action: string; tone: string; module: ModuleId }>>(() => [
  {
    label: "可用点数",
    value: quota.value.toLocaleString("zh-CN"),
    icon: "点",
    action: "去充值",
    tone: "teal",
    hint: `总点数 ${quotaTotal.value.toLocaleString("zh-CN")}`,
    module: "membership"
  },
  {
    label: "今日生成",
    value: tasks.value.length || 128,
    hint: "昨日 96",
    icon: "生",
    action: "+33.3%",
    tone: "blue",
    module: "assets"
  },
  {
    label: "作品数量",
    value: assets.value.length || 342,
    hint: `总作品 ${assets.value.length || 342}`,
    icon: "作",
    action: "进入中心",
    tone: "purple",
    module: "assets"
  },
  {
    label: "API 平台",
    value: `${configuredApiCount.value} / ${Math.max(5, apiProviders.value.length)}`,
    hint: `已接入 ${configuredApiCount.value} 个平台`,
    icon: "API",
    action: "去配置",
    tone: "green",
    module: "apiSettings"
  }
]);
const dashboardTodos = computed<Array<{ title: string; desc: string; status: string; module: ModuleId; icon: string }>>(() => {
  const missingApis = Math.max(0, Math.max(5, apiProviders.value.length) - configuredApiCount.value);
  return [
    {
      title: "API Key 待配置",
      desc: `还有 ${missingApis || 2} 个平台未配置 API Key`,
      status: "去配置",
      module: "apiSettings",
      icon: "钥"
    },
    {
      title: "作品待整理",
      desc: `有 ${assets.value.length || 28} 个作品未分类`,
      status: "去整理",
      module: "assets",
      icon: "夹"
    },
    {
      title: "套餐即将到期",
      desc: "专业版套餐将于 30 天后到期",
      status: "去续费",
      module: "membership",
      icon: "期"
    },
    {
      title: "邀请好友",
      desc: "邀请好友可得 10% 返利",
      status: "去邀请",
      module: "membership",
      icon: "礼"
    }
  ];
});
const dashboardUsageLegend = [
  { label: "Midjourney v6.1", tone: "teal" },
  { label: "DALL·E 3", tone: "blue" },
  { label: "Stable Diffusion XL", tone: "purple" },
  { label: "其他", tone: "orange" }
];
const dashboardUsageDays = computed(() => {
  const labels = ["06-13", "06-14", "06-15", "06-16", "06-18", "06-19"];
  return labels.map((label, index) => {
    const base = 68 + ((index * 17) % 42);
    return {
      label,
      segments: [
        { tone: "teal", height: `${base}px` },
        { tone: "blue", height: `${Math.max(28, base - 34)}px` },
        { tone: "purple", height: `${Math.max(22, base - 46)}px` },
        { tone: "orange", height: `${Math.max(16, base - 58)}px` }
      ]
    };
  });
});
const workspaceTitle = computed(() => ({ user: "用户工作台", agent: "代理商中心", admin: "运营后台" })[currentWorkspace.value]);
const isRegisterRoute = computed(() => currentPath() === "/register");
const workspaceEyebrow = computed(() => ({ user: "USER WORKSPACE", agent: "AGENT WORKSPACE", admin: "ADMIN WORKSPACE" })[currentWorkspace.value]);
const workspaceHeroTitle = computed(() => {
  if (currentWorkspace.value === "agent") return "专注客户、佣金和提现";
  if (currentWorkspace.value === "admin") return "平台运营与主控管理";
  return "让创意成为可交付成果";
});
const workspaceHeroCopy = computed(() => {
  if (currentWorkspace.value === "agent") return "代理商只保留推广、客户、佣金、提现和团队管理入口，不再混入普通创作菜单。";
  if (currentWorkspace.value === "admin") return "管理员从平台视角管理用户、订单、模型、代理商审核和财务。";
  return "内容生产、智能体、GEO 增长和充值订阅属于普通用户工作台。";
});
const metrics = computed(() => {
  if (currentWorkspace.value === "agent" && channelCenter.value) {
    return [
      { label: "直属客户", value: channelCenter.value.summary.directCustomers },
      { label: "下级代理", value: channelCenter.value.summary.childAgents },
      { label: "累计佣金", value: money(channelCenter.value.summary.totalCommission) },
      { label: "可提现", value: money(channelCenter.value.summary.availableToWithdraw) }
    ];
  }
  return [
    { label: "生成任务", value: tasks.value.length },
    { label: "作品资产", value: assets.value.length },
    { label: "可用模型", value: models.value.length },
    { label: "可用积分", value: quota.value }
  ];
});
onMounted(() => {
  restoreApiSettings();
  restoreFavoriteWorks();
  syncModuleFromLocation();
  if (hasBrowserWindow()) {
    window.addEventListener("popstate", syncModuleFromLocation);
  }
  void restoreAuthenticatedSession();
});

onBeforeUnmount(() => {
  clearWirelessCanvasLoadTimer();
  if (hasBrowserWindow()) {
    window.removeEventListener("popstate", syncModuleFromLocation);
  }
});

function restoreFavoriteWorks() {
  try {
    const stored = uni.getStorageSync("favoriteWorkIds");
    favoriteWorkIds.value = Array.isArray(stored) ? stored.map(item => String(item)) : [];
  } catch (error) {
    favoriteWorkIds.value = [];
  }
}

async function restoreAuthenticatedSession() {
  const path = currentPath();
  const pathname = currentPathname();
  if (hasBrowserWindow() && isAdminPath(pathname)) {
    window.location.replace("/admin/");
    return;
  }
  const token = authService.storage.getToken();
  if (!token || path === loginRoute || isRegisterRoute.value) {
    isLoggedIn.value = false;
    return;
  }
  try {
    const auth = await authService.me();
    const workspace = workspaceFromAuth(auth);
    if (workspace === "admin" && hasBrowserWindow()) {
      window.location.replace("/admin/");
      return;
    }
    if (workspace === "agent" && hasBrowserWindow()) {
      window.location.replace("/agent/");
      return;
    }
    applyAuth(auth);
    await refresh();
    if (auth.agent) void refreshChannelCenter();
  } catch (error) {
    performLogout(false);
  }
}

async function refresh() {
  if (currentWorkspace.value === "agent") {
    await refreshChannelCenter();
    return;
  }
  const [taskItems, assetItems, modelItems, points] = await Promise.all([
    businessSdk.generation.listTasks(),
    businessSdk.assets.list(),
    businessSdk.models.list(),
    businessSdk.points.account()
  ]);
  tasks.value = taskItems;
  assets.value = assetItems;
  models.value = modelItems;
  pointAccount.value = points.account;
  model.value = models.value.find(item => item.code === "gpt-image-2")?.code || models.value[0]?.code || model.value;
}

async function showDashboardNotifications() {
  try {
    const dashboard = await api<UserDashboardResponse>("/api/v1/user/dashboard");
    const taskCount = dashboard.recentTasks?.length || 0;
    const assetCount = dashboard.recentAssets?.length || 0;
    uni.showModal({ title: "通知中心", content: `近期任务 ${taskCount} 条，近期作品 ${assetCount} 项。`, showCancel: false });
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "通知加载失败"), icon: "none" });
  }
}

function showWorksDensityMenu() {
  uni.showActionSheet({
    itemList: ["默认视图", "紧凑视图"],
    success: result => { worksDensity.value = result.tapIndex === 1 ? "compact" : "default"; },
    fail: error => {
      if (!String(error.errMsg || "").toLowerCase().includes("cancel")) uni.showToast({ title: "视图切换失败", icon: "none" });
    },
  });
}

async function showMembershipPlans() {
  membershipScrollTarget.value = "";
  await nextTick();
  membershipScrollTarget.value = "membership-plans";
}

async function createMembershipOrder(plan: MembershipPlanCard) {
  if (plan.custom) {
    uni.showToast({ title: "企业版请联系商务创建专属报价", icon: "none" });
    return;
  }
  try {
    await api("/api/v1/orders/create", {
      method: "POST",
      body: JSON.stringify({
        planId: plan.planId,
        amountCents: Math.round(plan.price * 100),
        paymentMethod: "wechat",
        idempotencyKey: `${plan.planId}-${Date.now()}`
      })
    });
    uni.showToast({ title: "订阅订单已创建", icon: "success" });
    await refresh();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建订单失败", icon: "none" });
  }
}

async function createIdentityOrder(pack: IdentityPackage) {
  try {
    await api(pack.endpoint, {
      method: "POST",
      body: JSON.stringify({
        planId: pack.planId,
        amountCents: Math.round(pack.price * 100),
        paymentMethod: "wechat",
        idempotencyKey: `${pack.id}-${Date.now()}`
      })
    });
    uni.showToast({ title: "身份订单已创建", icon: "success" });
    await refresh();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "创建身份订单失败", icon: "none" });
  }
}

async function refreshChannelCenter() {
  try {
    channelCenter.value = await businessSdk.agents.center();
    currentAgent.value = channelCenter.value.agent;
  } catch (error) {
    channelCenter.value = null;
  }
}

async function createChildAgent() {
  if (!childAgentForm.value.name.trim() || !childAgentForm.value.email.trim()) {
    uni.showToast({ title: "请填写名称和邮箱", icon: "none" });
    return;
  }
  childAgentCreating.value = true;
  try {
    await api<{ item: ChannelAgent }>("/api/v1/channel/children", {
      method: "POST",
      body: JSON.stringify({
        name: childAgentForm.value.name.trim(),
        email: childAgentForm.value.email.trim(),
        level: Number(childAgentForm.value.level || 1),
        inviteCode: childAgentForm.value.inviteCode.trim(),
        available: Number(childAgentForm.value.available || 0),
        status: "ACTIVE"
      })
    });
    uni.showToast({ title: "下级已开通", icon: "success" });
    childAgentForm.value = { name: "", email: "", level: 1, inviteCode: "", available: 0 };
    childAgentFormVisible.value = false;
    await refreshChannelCenter();
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "开通失败", icon: "none" });
  } finally {
    childAgentCreating.value = false;
  }
}

function parseApiModels(value: string) {
  return value
    .split(/[，,\n]/)
    .map(item => item.trim())
    .filter(Boolean);
}

function restoreApiSettings() {
  try {
    const raw = uni.getStorageSync(apiSettingsStorageKey);
    if (!raw) return;
    const parsed = JSON.parse(String(raw)) as { providers?: ApiProviderSettings[]; selectedId?: string };
    if (!Array.isArray(parsed.providers) || !parsed.providers.length) return;
    apiProviders.value = parsed.providers.map(provider => ({
      ...provider,
      models: Array.isArray(provider.models) ? provider.models : [],
      authType: provider.authType || "Bearer Token",
      timeoutSeconds: provider.timeoutSeconds || 60,
      proxyMode: provider.proxyMode || "后端代理",
      notes: provider.notes || "",
      textToImagePath: provider.textToImagePath || "/v1/images/generations",
      imageEditPath: provider.imageEditPath || "/v1/images/edits"
    }));
    const nextSelectedId = parsed.selectedId && apiProviders.value.some(provider => provider.id === parsed.selectedId)
      ? parsed.selectedId
      : apiProviders.value[0].id;
    selectApiProvider(nextSelectedId, false);
  } catch (error) {
    apiProviders.value = defaultApiProviders.map(provider => ({ ...provider }));
    selectApiProvider(apiProviders.value[0].id, false);
  }
}

function persistApiSettings() {
  uni.setStorageSync(apiSettingsStorageKey, JSON.stringify({
    providers: apiProviders.value,
    selectedId: selectedApiProviderId.value
  }));
}

function selectApiProvider(id: string, persist = true) {
  const provider = apiProviders.value.find(item => item.id === id) || apiProviders.value[0];
  if (!provider) return;
  selectedApiProviderId.value = provider.id;
  apiProviderForm.value = createApiProviderForm(provider);
  apiTestMessage.value = provider.apiKey ? "已保存 Key，可测试配置" : "尚未测试配置";
  if (persist) persistApiSettings();
  apiSettingsMode.value = "edit";
}

function normalizedApiProviderFromForm(): ApiProviderSettings {
  const normalizedId = apiProviderForm.value.id.trim() || `provider-${Date.now()}`;
  return {
    id: normalizedId,
    name: apiProviderForm.value.name.trim() || "未命名平台",
    protocol: apiProviderForm.value.protocol,
    baseUrl: apiProviderForm.value.baseUrl.trim(),
    apiKey: apiProviderForm.value.apiKey.trim(),
    models: parseApiModels(apiProviderForm.value.modelText),
    status: apiProviderForm.value.status,
    authType: apiProviderForm.value.authType,
    timeoutSeconds: Number(apiProviderForm.value.timeoutSeconds || 60),
    proxyMode: apiProviderForm.value.proxyMode,
    notes: apiProviderForm.value.notes.trim(),
    textToImagePath: apiProviderForm.value.textToImagePath || "/v1/images/generations",
    imageEditPath: apiProviderForm.value.imageEditPath || "/v1/images/edits"
  };
}

function saveApiSettings() {
  const nextProvider = normalizedApiProviderFromForm();
  const existingIndex = apiProviders.value.findIndex(provider => provider.id === selectedApiProviderId.value);
  if (existingIndex >= 0) {
    apiProviders.value.splice(existingIndex, 1, nextProvider);
  } else {
    apiProviders.value = [nextProvider, ...apiProviders.value];
  }
  selectedApiProviderId.value = nextProvider.id;
  apiProviderForm.value = createApiProviderForm(nextProvider);
  persistApiSettings();
  apiTestMessage.value = "已保存到本地配置";
  uni.showToast({ title: "API 设置已保存", icon: "success" });
}

function buildDraftFromTemplate(template = selectedApiTemplate.value): ApiProviderForm {
  const stamp = Date.now();
  return createApiProviderForm({
    ...template.provider,
    id: `${template.provider.id}-${stamp}`,
    apiKey: "",
    models: [...template.provider.models]
  });
}

function openAddApiProviderPanel() {
  newApiProviderDraft.value = buildDraftFromTemplate();
  apiSettingsMode.value = "add";
}

function openRecommendedApiPanel() {
  apiSettingsMode.value = "recommend";
}

function cancelAddApiProvider() {
  apiSettingsMode.value = "edit";
}

function changeApiTemplate(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  newApiProviderTemplateId.value = apiProviderTemplates[index]?.id || apiProviderTemplates[0].id;
  newApiProviderDraft.value = buildDraftFromTemplate(selectedApiTemplate.value);
}

function changeNewApiProtocol(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  newApiProviderDraft.value.protocol = apiProtocolOptions[index] || apiProtocolOptions[0];
}

function changeNewApiAuthType(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  newApiProviderDraft.value.authType = apiAuthTypeOptions[index] || apiAuthTypeOptions[0];
}

function changeChildAgentLevel(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  const option = childAgentLevelOptions.value[index];
  if (option) childAgentForm.value.level = option.value;
}

function useRecommendedApi(id: string) {
  const item = recommendedApis.find(option => option.id === id) || recommendedApis[0];
  const template = apiProviderTemplates.find(option => option.id === item.templateId) || apiProviderTemplates[0];
  newApiProviderTemplateId.value = template.id;
  newApiProviderDraft.value = {
    ...buildDraftFromTemplate(template),
    name: item.name,
    baseUrl: item.baseUrl
  };
  apiSettingsMode.value = "add";
}

function confirmAddApiProvider() {
  const draft = newApiProviderDraft.value;
  const nextProvider: ApiProviderSettings = {
    id: draft.id.trim() || `provider-${Date.now()}`,
    name: draft.name.trim() || "新增平台",
    protocol: draft.protocol,
    baseUrl: draft.baseUrl.trim(),
    apiKey: draft.apiKey.trim(),
    models: parseApiModels(draft.modelText),
    status: draft.apiKey.trim() ? "配置完整" : "待配置",
    authType: draft.authType,
    timeoutSeconds: Number(draft.timeoutSeconds || 60),
    proxyMode: draft.proxyMode,
    notes: draft.notes.trim(),
    textToImagePath: draft.textToImagePath || "/v1/images/generations",
    imageEditPath: draft.imageEditPath || "/v1/images/edits"
  };
  if (!nextProvider.baseUrl || !nextProvider.models.length) {
    uni.showToast({ title: "请填写地址和模型", icon: "none" });
    return;
  }
  apiProviders.value = [nextProvider, ...apiProviders.value];
  apiSettingsMode.value = "edit";
  selectApiProvider(nextProvider.id);
  uni.showToast({ title: "已新增平台", icon: "success" });
}

function deleteApiProvider() {
  if (apiProviders.value.length <= 1) {
    uni.showToast({ title: "至少保留一个平台", icon: "none" });
    return;
  }
  apiProviders.value = apiProviders.value.filter(provider => provider.id !== selectedApiProviderId.value);
  selectApiProvider(apiProviders.value[0].id, false);
  persistApiSettings();
  uni.showToast({ title: "已删除平台", icon: "success" });
}

function changeApiProtocol(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  apiProviderForm.value.protocol = apiProtocolOptions[index] || apiProtocolOptions[0];
}

function changeApiAuthType(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  apiProviderForm.value.authType = apiAuthTypeOptions[index] || apiAuthTypeOptions[0];
}

function changeApiProxyMode(event: { detail: { value: number | string } }) {
  const index = Number(event.detail.value);
  apiProviderForm.value.proxyMode = apiProxyModeOptions[index] || apiProxyModeOptions[0];
}

function testApiConnection() {
  const hasBaseUrl = Boolean(apiProviderForm.value.baseUrl.trim());
  const hasModels = parseApiModels(apiProviderForm.value.modelText).length > 0;
  if (!hasBaseUrl || !hasModels) {
    apiProviderForm.value.status = "需补全";
    apiTestMessage.value = "请先填写请求地址和至少一个模型";
    return;
  }
  apiProviderForm.value.status = apiProviderForm.value.apiKey.trim() ? "配置完整" : "缺少 Key";
  apiTestMessage.value = apiProviderForm.value.apiKey.trim()
    ? "本地校验通过，后端连通性接口接入后可发起真实测试"
    : "请求地址和模型已填写，但还缺少 API Key";
}
function selectModule(id: ModuleId) {
  activeModule.value = id;
  currentWorkspace.value = moduleWorkspace[id] || currentWorkspace.value;
  isModuleDrawerOpen.value = false;
  isUserMenuOpen.value = false;
  pushModuleRoute(id);
  if (isAgentDataModule(id)) void refreshChannelCenter();
}
function isAdminPath(pathname: string) {
  return (pathname.replace(/\/$/, "") || "/") === "/admin";
}

function isAgentPath(pathname: string) {
  const normalized = pathname.replace(/\/$/, "") || "/";
  return normalized === "/agent" || normalized.startsWith("/agent/");
}

function syncModuleFromLocation() {
  if (!hasBrowserWindow()) {
    registerInviteCode.value = "";
    return;
  }

  const path = currentPath();
  registerInviteCode.value = currentSearchParams().get("invite") || "";
  if (isAdminPath(currentPathname())) {
    window.location.replace("/admin/");
    return;
  }
  if (path === loginRoute) {
    isLoggedIn.value = false;
    return;
  }
  if (path === "/register") {
    isLoggedIn.value = false;
    return;
  }
  const nextModule = routeModules[path] || "dashboard";
  activeModule.value = nextModule;
  currentWorkspace.value = moduleWorkspace[nextModule] || currentWorkspace.value;
  if (!routeModules[path] && path !== moduleRoutes.dashboard) {
    window.history.replaceState({ module: "dashboard" }, "", moduleRoutes.dashboard);
  }
}
function pushModuleRoute(id: ModuleId) {
  if (!hasBrowserWindow()) return;

  if (id === "admin") {
    window.location.assign("/admin/");
    return;
  }
  const path = moduleRoutes[id];
  if (!path || window.location.pathname === path) return;
  window.history.pushState({ module: id }, "", path);
}

function applyAuth(auth: AuthResponse) {
  currentUser.value = auth.user;
  currentAgent.value = auth.agent || null;
  currentWorkspace.value = workspaceFromAuth(auth);
  isLoggedIn.value = true;
}

function workspaceFromAuth(auth: AuthResponse): Workspace {
  if (auth.workspace === "admin") return "admin";
  if (auth.workspace === "agent" && auth.agent && isAgentPath(currentPathname())) return "agent";
  if (auth.user.role === "SUPER_ADMIN") return "admin";
  return "user";
}

function moduleFromAuth(auth: AuthResponse): ModuleId {
  const module = auth.defaultModule as ModuleId;
  if (moduleRoutes[module]) {
    if (moduleWorkspace[module] !== "agent" || (auth.agent && isAgentPath(currentPathname()))) return module;
  }
  const workspace = workspaceFromAuth(auth);
  return workspace === "agent" ? "agentHome" : workspace === "admin" ? "admin" : "dashboard";
}

function defaultRouteFromAuth(auth: AuthResponse) {
  const route = String(auth.defaultRoute || "").trim();
  if (!route.startsWith("/") || route.startsWith("//")) return "";
  if (route.startsWith("/admin") || route.startsWith("/app") || route.startsWith("/agent")) return route;
  return "";
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof Error) return error.message;
  if (error && typeof error === "object" && "errMsg" in error) {
    const errMsg = (error as { errMsg?: unknown }).errMsg;
    if (typeof errMsg === "string" && errMsg.trim()) return errMsg;
  }
  return fallback;
}

function redirectAfterAuth(auth: AuthResponse) {
  if (auth.accessToken) authService.storage.setToken(auth.accessToken);
  if (!hasBrowserWindow()) {
    const nextModule = moduleFromAuth(auth);
    applyAuth(auth);
    activeModule.value = nextModule;
    currentWorkspace.value = moduleWorkspace[nextModule] || workspaceFromAuth(auth);
    void refresh();
    if (auth.agent) void refreshChannelCenter();
    return;
  }

  const defaultRoute = defaultRouteFromAuth(auth);
  if (defaultRoute) {
    window.location.replace(defaultRoute);
    return;
  }
  const workspace = workspaceFromAuth(auth);
  if (workspace === "admin") {
    window.location.replace("/admin/");
    return;
  }
  if (workspace === "agent") {
    window.location.replace("/agent/");
    return;
  }
  window.location.replace("/app");
}

function isWeChatDevtoolsRuntime() {
  try {
    const systemInfo = typeof wx !== "undefined" && wx.getSystemInfoSync
      ? wx.getSystemInfoSync()
      : (uni.getSystemInfoSync() as { platform?: string; brand?: string });
    return systemInfo.platform === "devtools" || systemInfo.brand === "devtools";
  } catch (error) {
    return false;
  }
}

function requestWechatMiniProgramCode() {
  return new Promise<string>((resolve, reject) => {
    uni.login({
      provider: "weixin",
      success: result => {
        if (result.code) {
          resolve(result.code);
          return;
        }
        reject(new Error("微信授权未返回 code"));
      },
      fail: error => reject(new Error(errorMessage(error, "微信授权失败")))
    });
  });
}

function isAgentModule(id: ModuleId) {
  return moduleWorkspace[id] === "agent";
}

function isAgentDataModule(id: ModuleId) {
  return id === "agentCustomers" || id === "agentUsage" || id === "agentCommissions" || id === "agentWithdrawals" || id === "agentChildren";
}

function taskForAsset(asset: Asset) {
  return asset.taskId ? generationTaskMap.value.get(asset.taskId) : undefined;
}

function assetMetadataText(asset: Asset, keys: string[]) {
  for (const key of keys) {
    const value = asset.metadata?.[key];
    if (value !== undefined && value !== null && String(value).trim()) return String(value).trim();
  }
  return "";
}

function formatAssetName(asset: Asset) {
  const rawName = String(asset.name || "").trim();
  const fallback = asset.mediaType === "video" ? "AI 视频作品" : asset.mediaType === "document" ? "PPT 文档作品" : "AI 生图作品";
  return (rawName || fallback)
    .replace(/^TEXT_TO_IMAGE[-_]?/i, "AI 生图 ")
    .replace(/^IMAGE_TO_IMAGE[-_]?/i, "图生图 ")
    .replace(/^TEXT_TO_VIDEO[-_]?/i, "AI 视频 ")
    .replace(/^IMAGE_TO_VIDEO[-_]?/i, "图生视频 ");
}

function assetDescription(asset: Asset) {
  const task = taskForAsset(asset);
  const prompt = assetMetadataText(asset, ["prompt", "description", "title"]) || task?.prompt;
  if (prompt) return String(prompt);
  if (asset.taskId) return `任务 ${asset.taskId}`;
  return "已归档到作品中心";
}

function assetThumbnail(asset: Asset) {
  return asset.thumbnailUrl || asset.url;
}

function assetMediaLabel(asset: Asset) {
  if (asset.mediaType === "video") return "视频";
  if (asset.mediaType === "document") return "文档";
  return "图片";
}

function assetMediaIcon(asset: Asset) {
  if (asset.mediaType === "video") return "影";
  if (asset.mediaType === "document") return "文";
  return "图";
}

function assetModelLabel(asset: Asset) {
  return assetMetadataText(asset, ["model", "modelName", "provider"]) || taskForAsset(asset)?.model || "默认模型";
}

function assetResolutionLabel(asset: Asset) {
  return assetMetadataText(asset, ["resolution", "size", "dimensions", "aspectRatio"]) || "自适应";
}

function taskParamText(task: GenerationTask | undefined, keys: string[]) {
  if (!task?.params) return "";
  for (const key of keys) {
    const value = task.params[key];
    if (value !== undefined && value !== null && String(value).trim()) return String(value).trim();
  }
  return "";
}

function taskTitle(task: GenerationTask, asset?: Asset) {
  const namedTask = task as GenerationTask & { name?: string };
  const rawName = String(namedTask.name || asset?.name || task.prompt || "").trim();
  if (rawName) {
    return rawName
      .replace(/^TEXT_TO_IMAGE[-_]?/i, "AI 生图 ")
      .replace(/^IMAGE_TO_IMAGE[-_]?/i, "图生图 ")
      .replace(/^TEXT_TO_VIDEO[-_]?/i, "AI 视频 ")
      .replace(/^IMAGE_TO_VIDEO[-_]?/i, "图生视频 ");
  }
  if (task.type === "TEXT_TO_VIDEO" || task.type === "IMAGE_TO_VIDEO") return "AI 视频作品";
  return "AI 生图作品";
}

function taskLinkedAsset(task: GenerationTask) {
  const fromResult = (task.resultIds || [])
    .map(id => assetById.value.get(String(id)))
    .find(Boolean);
  return fromResult || firstAssetByTaskId.value.get(task.id);
}

function createWorkEntryFromTask(task: GenerationTask): UserWorkEntry {
  const asset = taskLinkedAsset(task);
  return {
    id: task.id,
    task,
    asset,
    title: taskTitle(task, asset),
    prompt: task.prompt || assetDescription(asset || createEmptyAsset(task.id)),
    status: task.status || "PROCESSING",
    model: task.model || assetModelLabel(asset || createEmptyAsset(task.id)),
    resolution: taskParamText(task, ["resolution", "size", "dimensions", "aspectRatio"]) || (asset ? assetResolutionLabel(asset) : "自适应"),
    pointCost: Number(task.pointCost || 0),
    assetUrl: asset ? resolveAssetUrl(asset) : "",
    thumbnailUrl: asset ? assetThumbnail(asset) : "",
    createdAt: task.createdAt || asset?.createdAt,
    updatedAt: task.updatedAt || task.workerFinishedAt || asset?.updatedAt
  };
}

function createWorkEntryFromAsset(asset: Asset): UserWorkEntry {
  return {
    id: asset.id,
    asset,
    title: formatAssetName(asset),
    prompt: assetDescription(asset),
    status: "SUCCEEDED",
    model: assetModelLabel(asset),
    resolution: assetResolutionLabel(asset),
    pointCost: Number(asset.metadata?.pointCost || 0),
    assetUrl: resolveAssetUrl(asset),
    thumbnailUrl: assetThumbnail(asset),
    createdAt: asset.createdAt,
    updatedAt: asset.updatedAt
  };
}

function createEmptyAsset(id: string): Asset {
  return { id, name: "", url: "", mediaType: "image" };
}

function workDateMs(entry: UserWorkEntry) {
  const value = entry.updatedAt || entry.createdAt;
  if (!value) return 0;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 0 : date.getTime();
}

function isWorkSucceeded(entry: UserWorkEntry) {
  return entry.status === "SUCCEEDED";
}

function isWorkRunning(entry: UserWorkEntry) {
  return ["QUEUED", "PROCESSING", "RETRYING"].includes(entry.status);
}

function isWorkFailed(entry: UserWorkEntry) {
  return ["FAILED", "CANCELLED"].includes(entry.status);
}

function isWorkFavorite(entry: UserWorkEntry) {
  return favoriteWorkIds.value.includes(entry.id);
}

function workStatusLabel(entry: UserWorkEntry) {
  if (isWorkSucceeded(entry)) return "已完成";
  if (isWorkRunning(entry)) return "生成中";
  if (entry.status === "CANCELLED") return "已取消";
  if (isWorkFailed(entry)) return "失败";
  return "排队中";
}

function workStatusClass(entry: UserWorkEntry) {
  if (isWorkSucceeded(entry)) return "is-success";
  if (isWorkRunning(entry)) return "is-running";
  if (isWorkFailed(entry)) return "is-failed";
  return "is-pending";
}

function workThumbnail(entry: UserWorkEntry) {
  if (entry.asset?.mediaType === "image") return entry.thumbnailUrl || entry.assetUrl;
  return "";
}

function previewWorkEntry(entry: UserWorkEntry) {
  if (!entry.assetUrl || !hasBrowserWindow()) {
    uni.showToast({ title: isWorkRunning(entry) ? "作品仍在生成中" : "暂无可预览文件", icon: "none" });
    return;
  }
  window.open(entry.assetUrl, "_blank", "noopener,noreferrer");
}

function downloadWorkEntry(entry: UserWorkEntry) {
  if (!entry.assetUrl || !hasBrowserWindow()) {
    uni.showToast({ title: "暂无可下载文件", icon: "none" });
    return;
  }
  const link = document.createElement("a");
  link.href = entry.assetUrl;
  link.download = entry.title || entry.id;
  link.rel = "noopener noreferrer";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function reuseWorkEntry(entry: UserWorkEntry) {
  if (entry.prompt) {
    uni.setClipboardData({
      data: entry.prompt,
      success: () => uni.showToast({ title: "提示词已复制，可继续创作", icon: "success" }),
      fail: () => uni.showToast({ title: "已复用当前配置", icon: "success" })
    });
    return;
  }
  uni.showToast({ title: "暂无可复用提示词", icon: "none" });
}

function toggleWorkFavorite(entry: UserWorkEntry) {
  const next = new Set(favoriteWorkIds.value);
  if (next.has(entry.id)) {
    next.delete(entry.id);
    uni.showToast({ title: "已取消收藏", icon: "none" });
  } else {
    next.add(entry.id);
    uni.showToast({ title: "已加入收藏", icon: "success" });
  }
  favoriteWorkIds.value = Array.from(next);
  try {
    uni.setStorageSync("favoriteWorkIds", favoriteWorkIds.value);
  } catch (error) {
    // 收藏失败不影响作品列表浏览。
  }
}

function resolveAssetUrl(asset: Asset) {
  const rawUrl = String(asset.url || "").trim();
  if (!rawUrl) return "";
  if (/^(https?:|data:|blob:)/i.test(rawUrl)) return rawUrl;
  if (!hasBrowserWindow()) return rawUrl;
  return new URL(rawUrl, window.location.origin).toString();
}

function previewAsset(asset: Asset) {
  const url = resolveAssetUrl(asset);
  if (!url || !hasBrowserWindow()) {
    uni.showToast({ title: "暂无可预览文件", icon: "none" });
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

function downloadAsset(asset: Asset) {
  const url = resolveAssetUrl(asset);
  if (!url || !hasBrowserWindow()) {
    uni.showToast({ title: "暂无可下载文件", icon: "none" });
    return;
  }
  const link = document.createElement("a");
  link.href = url;
  link.download = formatAssetName(asset);
  link.rel = "noopener noreferrer";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function copyAssetLink(asset: Asset) {
  const url = resolveAssetUrl(asset);
  if (!url) {
    uni.showToast({ title: "暂无可复制链接", icon: "none" });
    return;
  }
  uni.setClipboardData({
    data: url,
    success: () => uni.showToast({ title: "链接已复制", icon: "success" }),
    fail: () => uni.showToast({ title: "复制失败", icon: "none" })
  });
}

function isToday(value?: string) {
  if (!value) return false;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
}

function formatShortDate(value?: string) {
  if (!value) return "06/19 20:21";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "06/19 20:21";
  const month = `${date.getMonth() + 1}`.padStart(2, "0");
  const day = `${date.getDate()}`.padStart(2, "0");
  const hour = `${date.getHours()}`.padStart(2, "0");
  const minute = `${date.getMinutes()}`.padStart(2, "0");
  return `${month}/${day} ${hour}:${minute}`;
}
function money(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}

function logout() {
  isUserMenuOpen.value = false;
  performLogout(true);
}

function performLogout(showMessage = true) {
  authService.storage.clear();
  currentUser.value = null;
  currentAgent.value = null;
  channelCenter.value = null;
  isLoggedIn.value = false;
  isModuleDrawerOpen.value = false;
  if (hasBrowserWindow()) {
    window.history.pushState({ loggedOut: true }, "", loginRoute);
  }
  if (showMessage) uni.showToast({ title: "已退出", icon: "success" });
}

function goLogin() {
  if (!hasBrowserWindow()) {
    isLoggedIn.value = false;
    return;
  }

  window.history.pushState({ module: "login" }, "", loginRoute);
  syncModuleFromLocation();
}

async function registerByInvite() {
  if (!registerUsername.value.trim() || !registerEmail.value.trim() || !registerPassword.value.trim() || !registerConfirmPassword.value.trim()) {
    uni.showToast({ title: "请填写注册信息", icon: "none" });
    return;
  }
  if (registerPassword.value.trim() !== registerConfirmPassword.value.trim()) {
    uni.showToast({ title: "两次输入的密码不一致", icon: "none" });
    return;
  }
  if (!registerInviteCode.value.trim()) {
    uni.showToast({ title: "邀请链接缺少邀请码", icon: "none" });
    return;
  }
  try {
    const auth = await authService.registerByInvite({
      username: registerUsername.value.trim(),
      email: registerEmail.value.trim(),
      password: registerPassword.value.trim(),
      confirmPassword: registerConfirmPassword.value.trim(),
      inviteCode: registerInviteCode.value.trim()
    });
    redirectAfterAuth(auth);
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "注册失败", icon: "none" });
  }
}

async function login() {
  if (!loginEmail.value.trim() || !loginPassword.value.trim()) {
    uni.showToast({ title: "请输入账号密码", icon: "none" });
    return;
  }
  try {
    const auth = await authService.loginByPassword(loginEmail.value.trim(), loginPassword.value.trim());
    redirectAfterAuth(auth);
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "登录失败", icon: "none" });
  }
}

async function loginWithWechatMiniProgram() {
  if (wechatLoginLoading.value) return;
  wechatLoginLoading.value = true;
  try {
    const code = enableMockLogin && isWeChatDevtoolsRuntime() ? "mock-devtools-code" : await requestWechatMiniProgramCode();
    if (code === "mock-devtools-code") {
      console.info("微信开发者工具模拟登录 code", code);
    }
    const auth = await authService.loginByWechatMiniProgramCode(code);
    redirectAfterAuth(auth);
  } catch (error) {
    uni.showToast({ title: errorMessage(error, "微信登录暂未开通"), icon: "none" });
  } finally {
    wechatLoginLoading.value = false;
  }
}
</script>

<style scoped>
.login-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  overflow-x: hidden;
  padding: 24px;
  background: #eef0f4;
  box-sizing: border-box;
}

.login-screen {
  position: relative;
  isolation: isolate;
  display: grid;
  justify-items: center;
  align-content: start;
  gap: 18px;
  width: min(390px, 100%);
  min-height: min(844px, calc(100vh - 48px));
  max-width: 100vw;
  overflow: hidden;
  padding: 20px 24px 24px;
  border-radius: 28px;
  background: #f8fafc;
  box-shadow: 0 24px 70px rgba(16, 24, 40, .16);
  box-sizing: border-box;
}

.login-bg-accent {
  position: absolute;
  z-index: -1;
  border-radius: 999px;
  pointer-events: none;
}

.login-bg-accent-blue {
  top: -116px;
  left: -144px;
  width: 252px;
  height: 252px;
  background: rgba(226, 244, 255, .62);
}

.login-bg-accent-warm {
  right: -104px;
  bottom: -100px;
  width: 224px;
  height: 224px;
  background: rgba(255, 244, 235, .64);
}

.login-statusbar {
  width: 100%;
  max-width: 100%;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #101828;
  font-size: 13px;
  font-weight: 800;
}

.mini-capsule {
  width: 74px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid #e7ebf2;
  border-radius: 999px;
  background: rgba(255, 255, 255, .94);
  box-shadow: 0 8px 18px rgba(16, 24, 40, .08);
}

.mini-dot {
  width: 4px;
  height: 4px;
  display: block;
  border-radius: 999px;
  background: rgba(52, 64, 84, .82);
}

.login-brand {
  width: 100%;
  display: grid;
  justify-items: center;
  gap: 8px;
}

.login-heading {
  width: 100%;
  display: grid;
  justify-items: center;
  gap: 8px;
  text-align: center;
}

.login-card {
  width: 100%;
  max-width: 100%;
  display: grid;
  gap: 14px;
  padding: 18px;
  border: 1px solid rgba(231, 235, 242, .94);
  border-radius: 18px;
  background: rgba(255, 255, 255, .94);
  color: #101828;
  box-shadow: 0 24px 70px rgba(16, 24, 40, .12);
  box-sizing: border-box;
}

.login-logo {
  width: 142px;
  height: 118px;
}

.eyebrow {
  width: fit-content;
  display: block;
  padding: 5px 10px;
  border-radius: 999px;
  background: #ecfdf3;
  color: #047857;
  font-size: 11px;
  font-weight: 900;
  line-height: 1.2;
}

.login-title {
  display: block;
  color: #101828;
  font-size: 30px;
  font-weight: 900;
  line-height: 1.18;
}

.login-copy {
  max-width: 318px;
  display: block;
  color: #667085;
  font-size: 14px;
  line-height: 1.65;
}

.invite-chip {
  width: fit-content;
  padding: 8px 12px;
  border-radius: 999px;
  background: #ecfdf3;
  color: #047857;
  font-size: 13px;
  font-weight: 800;
}

.login-form {
  display: grid;
  gap: 14px;
}

.login-form label {
  display: grid;
  gap: 7px;
  color: #344054;
  font-size: 13px;
  font-weight: 800;
}

.login-form input {
  width: 100%;
  height: 46px;
  padding: 0 13px;
  border: 1px solid #d0d5dd;
  border-radius: 10px;
  background: #fff;
  color: #101828;
  font-size: 15px;
  outline: none;
  box-sizing: border-box;
}

.login-form input:focus {
  border-color: #101828;
  box-shadow: 0 0 0 3px rgba(16, 24, 40, .1);
}

.login-submit,
.login-link-button,
.wechat-login-button {
  width: 100%;
  min-height: 46px;
  margin: 0;
  padding: 0 14px;
  border: 0;
  border-radius: 10px;
  font-size: 15px;
  font-weight: 900;
  line-height: 46px;
  box-sizing: border-box;
}

.login-submit {
  background: #101828;
  color: #fff;
}

.login-link-button {
  background: transparent;
  color: #2563eb;
}

.login-submit::after,
.login-link-button::after,
.wechat-login-button::after {
  border: 0;
}

.login-divider {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 10px;
  color: #667085;
  font-size: 12px;
}

.login-divider view {
  height: 1px;
  background: #e7ebf2;
}

.wechat-login-button {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid #d0d5dd;
  background: #fff;
  color: #047857;
}

.wechat-login-button[disabled] {
  opacity: .68;
}

.wechat-login-icon {
  position: relative;
  width: 20px;
  height: 16px;
  display: inline-block;
}

.wechat-login-icon::before,
.wechat-login-icon::after {
  content: "";
  position: absolute;
  border-radius: 999px;
  background: #07c160;
}

.wechat-login-icon::before {
  left: 0;
  top: 2px;
  width: 13px;
  height: 9px;
}

.wechat-login-icon::after {
  right: 0;
  bottom: 1px;
  width: 12px;
  height: 8px;
  opacity: .72;
}

.login-agreement {
  display: block;
  color: #667085;
  font-size: 11px;
  line-height: 1.45;
  text-align: center;
}

.login-helper {
  width: 100%;
  min-height: 70px;
  display: grid;
  grid-template-columns: 4px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid #e7ebf2;
  border-radius: 12px;
  background: rgba(255, 255, 255, .72);
}

.login-helper-accent {
  width: 4px;
  height: 44px;
  border-radius: 999px;
  background: linear-gradient(180deg, #2563eb, #06b6d4);
}

.login-helper view:last-child {
  display: grid;
  gap: 4px;
}

.login-helper text:first-child {
  color: #101828;
  font-size: 13px;
  font-weight: 900;
}

.login-helper text:last-child {
  color: #667085;
  font-size: 12px;
  line-height: 1.4;
}

.register-shell .login-card {
  width: min(440px, 100%);
  padding: 28px;
}

.register-shell .login-logo {
  justify-self: center;
}

@media (max-width: 520px) {
  .login-shell {
    padding: 0;
    background: #f8fafc;
  }

  .login-screen {
    width: 100vw;
    min-height: 100vh;
    border-radius: 0;
    box-shadow: none;
  }
}

.module-wirelessCanvas {
  --module-accent: #111827;
  --module-accent-2: #64748b;
  --module-accent-soft: rgba(15, 23, 42, .12);
}

.module-videoGeneration {
  --module-accent: #2563eb;
  --module-accent-2: #38bdf8;
  --module-accent-soft: rgba(37, 99, 235, .15);
}

.module-ppt {
  --module-accent: #0f766e;
  --module-accent-2: #2563eb;
  --module-accent-soft: rgba(15, 118, 110, .14);
}

.nav-wirelessCanvas:before {
  content: "布";
}

.nav-videoGeneration:before {
  content: "▶";
}

.nav-ppt:before {
  content: "P";
}

.wireless-canvas-page {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 100vh;
  overflow: hidden;
  background: #f8fafc;
}

.wireless-canvas-frame {
  display: block;
  width: 100%;
  height: 100vh;
  border: 0;
  background: #f8fafc;
  opacity: 1;
  transition: opacity .18s ease;
}

.wireless-canvas-frame.is-loading {
  opacity: 0;
}

.wireless-canvas-loading {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background:
    linear-gradient(135deg, rgba(15, 118, 110, .08), transparent 34%),
    linear-gradient(315deg, rgba(37, 99, 235, .08), transparent 36%),
    #f8fafc;
  color: #334155;
  pointer-events: none;
}

.wireless-canvas-loading text:first-child {
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
}

.wireless-canvas-loading text:last-child {
  color: #64748b;
  font-size: 13px;
}

.video-generation-page {
  min-height: 100%;
  background: #f8fafc;
}

.video-generation-blank {
  min-height: calc(100vh - 72px);
  background: #f8fafc;
}

.ppt-generation-page {
  min-height: 100%;
  background: #f8fafc;
}

.ppt-generation-scroll {
  height: calc(100vh - 72px);
  padding: 0;
  background: #f8fafc;
  color: #0f172a;
}

.membership-page {
  background:
    radial-gradient(circle at 20% 2%, rgba(37, 99, 235, .18), transparent 24%),
    radial-gradient(circle at 64% 40%, rgba(45, 212, 191, .09), transparent 28%),
    #020403;
  color: rgba(255, 255, 255, .86);
  font-family: Inter, "Microsoft YaHei", "PingFang SC", "Segoe UI", Arial, sans-serif;
  letter-spacing: 0;
}

.membership-shell {
  display: grid;
  gap: 36px;
  min-height: 100%;
  padding: 16px 28px 28px;
}

.membership-head {
  display: grid;
  grid-template-columns: minmax(280px, .85fr) minmax(520px, 1fr);
  align-items: start;
  gap: 48px;
}

.membership-title {
  display: block;
  color: #20f3ff;
  font-size: 22px;
  font-weight: 900;
  line-height: 1.25;
}

.membership-subtitle {
  display: block;
  margin-top: 10px;
  color: rgba(255, 255, 255, .58);
  font-size: 13px;
  font-weight: 800;
}

.current-subscription-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 28px;
  min-height: 126px;
  border-radius: 12px;
  background: #1b1f1e;
  padding: 26px 24px;
}

.current-subscription-copy {
  display: grid;
  gap: 10px;
}

.current-subscription-title-row {
  display: flex;
  align-items: center;
  gap: 14px;
}

.current-subscription-title-row > uni-text:first-child,
.current-subscription-title-row > text:first-child {
  color: #fff;
  font-size: 14px;
  font-weight: 850;
}

.current-subscription-title-row > uni-text:last-child,
.current-subscription-title-row > text:last-child {
  display: grid;
  place-items: center;
  width: 36px;
  height: 30px;
  border-radius: 8px;
  background: rgba(20, 184, 166, .22);
  color: #8bc9d7;
  font-size: 16px;
  font-weight: 900;
}

.current-subscription-copy > uni-text:last-child,
.current-subscription-copy > text:last-child {
  color: rgba(255, 255, 255, .62);
  font-size: 12px;
  font-weight: 800;
}

.subscription-balance {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #fff;
  font-size: 22px;
  font-weight: 950;
}

.balance-gem {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  background: linear-gradient(135deg, #22f3f3, #2d6bff);
  color: #032126;
  font-size: 17px;
}

.subscription-actions {
  display: flex;
  gap: 16px;
}

.subscription-actions uni-button,
.subscription-actions button {
  min-width: 120px;
  min-height: 56px;
  border: 0;
  border-radius: 999px;
  color: #fff;
  font-size: 14px;
  font-weight: 950;
}

.recharge-button {
  background: linear-gradient(135deg, #2df5ef, #2f69ff);
}

.detail-button {
  background: #323635;
}

.billing-tabs {
  display: inline-flex;
  width: fit-content;
  max-width: 100%;
  gap: 24px;
  border: 1px solid rgba(255, 255, 255, .18);
  border-radius: 12px;
  background: rgba(22, 26, 25, .82);
  padding: 6px;
}

.billing-tabs uni-button,
.billing-tabs button {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 236px;
  min-height: 60px;
  justify-content: center;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: rgba(255, 255, 255, .82);
  font-size: 16px;
  font-weight: 950;
}

.billing-tabs uni-button:after,
.subscription-actions uni-button:after,
.subscribe-button:after {
  display: none;
}

.billing-tabs button.active,
.billing-tabs uni-button.active {
  background: linear-gradient(90deg, rgba(38, 238, 225, .35), rgba(45, 103, 255, .45));
  color: #fff;
}

.billing-tabs button uni-text:last-child,
.billing-tabs button text:last-child {
  color: #22fbff;
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 22px;
}

.custom-pricing-grid {
  grid-template-columns: 1fr;
  margin-top: -14px;
}

.identity-package-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 22px;
}

.pricing-card {
  display: grid;
  gap: 22px;
  align-content: start;
  min-height: 690px;
  border: 1px solid rgba(255, 255, 255, .2);
  border-radius: 22px;
  background: rgba(21, 24, 23, .86);
  padding: 34px 24px;
}

.custom-pricing-card {
  min-height: auto;
}

.identity-package-card {
  min-height: 560px;
}

.pricing-card.featured {
  border-color: #22f3ff;
  background: linear-gradient(125deg, rgba(18, 84, 73, .82), rgba(11, 32, 82, .96));
  box-shadow: 0 0 0 1px rgba(47, 105, 255, .5) inset;
}

.pricing-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.plan-name {
  color: #2ffcf1;
  font-size: 16px;
  font-weight: 950;
  font-style: italic;
}

.recommended-badge {
  border-radius: 10px;
  background: rgba(33, 91, 184, .72);
  color: #34fbff;
  padding: 7px 10px;
  font-size: 12px;
  font-weight: 900;
  white-space: nowrap;
}

.price-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.price-row > uni-text:nth-child(1),
.price-row > text:nth-child(1) {
  color: #fff;
  font-size: 14px;
  font-weight: 950;
}

.price-row > uni-text:nth-child(2),
.price-row > text:nth-child(2) {
  color: #fff;
  font-size: 26px;
  font-weight: 950;
  line-height: 1;
}

.price-row > uni-text:nth-child(3),
.price-row > text:nth-child(3) {
  color: rgba(255, 255, 255, .56);
  font-size: 13px;
  font-weight: 850;
  text-decoration: line-through;
}

.price-note {
  margin-top: -18px;
  color: rgba(255, 255, 255, .64);
  font-size: 12px;
  font-weight: 800;
}

.credit-box {
  display: grid;
  gap: 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, .075);
  padding: 18px;
}

.credit-box > uni-view,
.credit-box > view {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.credit-box > uni-view uni-text:first-child,
.credit-box > view text:first-child {
  color: #23fbff;
  font-size: 22px;
  font-weight: 900;
}

.credit-box > uni-view uni-text:last-child,
.credit-box > view text:last-child,
.credit-box > uni-text,
.credit-box > text {
  color: rgba(255, 255, 255, .84);
  font-size: 13px;
  font-weight: 800;
}

.credit-box > uni-text uni-text,
.credit-box > text text {
  display: inline-grid;
  place-items: center;
  width: 23px;
  height: 23px;
  margin-left: 6px;
  border: 1px solid rgba(255, 255, 255, .5);
  border-radius: 999px;
  color: rgba(255, 255, 255, .62);
  font-size: 14px;
}

.subscribe-button {
  min-height: 50px;
  border: 0;
  border-radius: 999px;
  background: linear-gradient(90deg, #2df4eb, #2d68ff);
  color: #fff;
  font-size: 14px;
  font-weight: 950;
}

.feature-list {
  display: grid;
  gap: 14px;
  margin-top: 4px;
}

.feature-list > uni-view,
.feature-list > view {
  display: flex;
  align-items: center;
  gap: 14px;
  color: rgba(255, 255, 255, .82);
  font-size: 13px;
  font-weight: 800;
}

.feature-list > uni-view uni-text:first-child,
.feature-list > view text:first-child {
  color: rgba(255, 255, 255, .92);
  font-size: 16px;
  font-weight: 300;
}

@media (max-width: 1280px) {
  .membership-head {
    grid-template-columns: 1fr;
    gap: 24px;
  }

  .standard-pricing-grid {
    grid-template-columns: repeat(4, minmax(248px, 1fr));
    overflow-x: auto;
    padding-bottom: 8px;
  }

  .identity-package-grid {
    grid-template-columns: repeat(3, minmax(248px, 1fr));
    overflow-x: auto;
    padding-bottom: 8px;
  }

  .custom-pricing-grid {
    grid-template-columns: 1fr;
  }

  .pricing-card {
    min-height: auto;
  }
}

@media (max-width: 760px) {
  .membership-shell {
    gap: 24px;
    padding: 18px 14px 24px;
  }

  .membership-title {
    font-size: 22px;
  }

  .membership-subtitle {
    font-size: 13px;
  }

  .current-subscription-card {
    grid-template-columns: 1fr;
    gap: 18px;
    padding: 20px;
  }

  .subscription-actions,
  .billing-tabs {
    width: 100%;
  }

  .subscription-actions uni-button,
  .subscription-actions button {
    flex: 1 1 0;
    min-width: 0;
    min-height: 48px;
    font-size: 14px;
  }

  .billing-tabs {
    display: grid;
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .billing-tabs uni-button,
  .billing-tabs button {
    min-width: 0;
    min-height: 48px;
    font-size: 14px;
  }

  .standard-pricing-grid {
    grid-template-columns: repeat(4, minmax(238px, 1fr));
    overflow-x: auto;
  }

  .identity-package-grid {
    grid-template-columns: repeat(3, minmax(238px, 1fr));
    overflow-x: auto;
  }

  .pricing-card {
    gap: 22px;
    padding: 28px 20px;
  }

  .pricing-card-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .price-row > uni-text:nth-child(2),
  .price-row > text:nth-child(2) {
    font-size: 26px;
  }

  .price-row > uni-text:nth-child(3),
  .price-row > text:nth-child(3) {
    font-size: 13px;
  }
}

.channel-mini-button,
.channel-submit-button {
  border: 0;
  border-radius: 8px;
  background: #14b8a6;
  color: #fff;
  font-size: 13px;
  font-weight: 800;
}

.channel-mini-button {
  min-height: 34px;
  padding: 0 14px;
}

.channel-child-form {
  display: grid;
  gap: 14px;
  margin: 14px 0 18px;
  padding: 14px;
  border: 1px solid rgba(148, 163, 184, .28);
  border-radius: 10px;
  background: rgba(15, 23, 42, .05);
}

.channel-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.channel-form-grid label {
  display: grid;
  gap: 6px;
  color: #64748b;
  font-size: 12px;
  font-weight: 800;
}

.channel-form-grid input,
.channel-picker {
  width: 100%;
  min-height: 38px;
  box-sizing: border-box;
  border: 1px solid rgba(148, 163, 184, .42);
  border-radius: 8px;
  background: #fff;
  color: #0f172a;
  padding: 0 10px;
  font-size: 14px;
  line-height: 38px;
}

.channel-submit-button {
  min-height: 40px;
}

.channel-submit-button[disabled] {
  opacity: .55;
}

@media (max-width: 760px) {
  .channel-form-grid {
    grid-template-columns: 1fr;
  }
}

.app-shell.module-assets {
  grid-template-columns: 248px minmax(0, 1fr);
  background: #f6f8ff;
}

.app-shell.module-assets .sidebar {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
  padding: 0 10px 12px;
  border-right: 1px solid #dfe5ee;
  background: #fff;
  color: #101828;
}

.app-shell.module-assets .brand-block {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  grid-template-rows: auto auto;
  align-items: center;
  justify-items: start;
  gap: 2px 10px;
  min-height: 52px;
  padding: 8px 4px 10px;
  border-bottom: 1px solid #e5eaf2;
}

.app-shell.module-assets .brand-logo {
  grid-row: 1 / 3;
  width: 42px;
  height: 42px;
  border: 1px solid #dfe5ee;
  border-radius: 8px;
  padding: 2px;
  background: #fff;
}

.app-shell.module-assets .brand-title {
  min-height: 0;
  padding: 0;
  border-radius: 0;
  background: transparent;
  color: #111827;
  font-size: 16px;
  font-weight: 900;
  line-height: 1.15;
}

.app-shell.module-assets .brand-title::after {
  display: none;
}

.app-shell.module-assets .brand-subtitle {
  display: block;
  color: #667085;
  font-size: 12px;
  font-weight: 650;
  line-height: 1.2;
}

.app-shell.module-assets .nav-list {
  flex: 1 1 auto;
  gap: 6px;
  min-height: 0;
  overflow-y: auto;
  padding: 0;
}

.app-shell.module-assets .nav-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  min-height: 42px;
  border: 0;
  border-radius: 10px;
  padding: 0 12px 0 42px;
  background: transparent;
  color: #111827;
  font-size: 14px;
  font-weight: 850;
  line-height: 1;
  text-align: left;
  box-shadow: none;
}

.app-shell.module-assets .nav-item::before {
  position: absolute;
  left: 16px;
  width: 16px;
  height: 16px;
  color: #111827;
  font-size: 15px;
}

.app-shell.module-assets .nav-item::after {
  border: 0;
}

.app-shell.module-assets .nav-dashboard::before {
  content: "⌂";
}

.app-shell.module-assets .nav-wirelessCanvas::before {
  content: "▣";
}

.app-shell.module-assets .nav-videoGeneration::before {
  content: "▭";
}

.app-shell.module-assets .nav-ppt::before {
  content: "▤";
}

.app-shell.module-assets .nav-assets::before {
  content: "▣";
}

.app-shell.module-assets .nav-apiSettings::before {
  content: "◇";
}

.app-shell.module-assets .nav-agents::before {
  content: "◈";
}

.app-shell.module-assets .nav-geo::before {
  content: "G";
}

.app-shell.module-assets .nav-membership::before {
  content: "□";
}

.app-shell.module-assets .nav-usage::before {
  content: "≡";
}

.app-shell.module-assets .nav-item.active {
  background: #f1efff;
  color: #5b49e8;
  box-shadow: inset 4px 0 0 #6c5cf4;
}

.app-shell.module-assets .nav-item.active::before {
  color: #5b49e8;
}

.app-shell.module-assets .more-nav-button {
  margin-top: 0;
  border: 0;
  background: transparent;
  color: #111827;
  box-shadow: none;
}

.app-shell.module-assets .logout-nav-button {
  margin-top: 0;
  border: 0;
  background: transparent;
  color: #111827;
}

.app-shell.module-assets .sidebar-plan-card {
  display: grid;
  gap: 8px;
  padding: 14px 12px;
  border: 1px solid #e1e7f2;
  border-radius: 14px;
  background: #fff;
  color: #111827;
  box-shadow: 0 10px 26px rgba(56, 72, 112, .08);
}

.app-shell.module-assets .sidebar-version {
  display: none;
}

.works-center-page {
  display: grid;
  grid-template-rows: 52px minmax(0, 1fr);
  min-height: 100%;
  background: #f6f8ff;
  color: #101828;
}

.works-center-page .topbar {
  min-height: 52px;
  border-bottom: 1px solid #dfe5ee;
  background: rgba(255, 255, 255, .96);
  color: #101828;
  box-shadow: none;
}

.works-admin-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 0 16px 0 20px;
}

.works-admin-titlebar,
.works-header-title,
.works-admin-actions,
.works-header-search,
.works-size-button,
.works-header-button {
  display: flex;
  align-items: center;
}

.works-admin-titlebar {
  gap: 18px;
  min-width: 0;
}

.works-header-icon,
.works-header-button,
.works-size-button {
  margin: 0;
  border: 1px solid #dfe5ee;
  background: #fff;
  color: #111827;
  box-sizing: border-box;
}

.works-header-icon::after,
.works-header-button::after,
.works-size-button::after {
  border: 0;
}

.works-header-icon,
.works-header-button {
  justify-content: center;
  width: 36px;
  height: 36px;
  min-height: 36px;
  border-radius: 10px;
  padding: 0;
  color: #5b49e8;
  font-size: 18px;
  font-weight: 900;
  line-height: 36px;
}

.works-header-title {
  gap: 10px;
  min-width: 0;
  color: #111827;
  font-size: 20px;
  font-weight: 900;
}

.works-header-title text:first-child {
  color: #5b49e8;
  font-size: 16px;
}

.works-admin-actions {
  gap: 10px;
  min-width: 0;
}

.works-header-search {
  gap: 8px;
  width: min(320px, 26vw);
  height: 36px;
  padding: 0 12px;
  border: 1px solid #dfe5ee;
  border-radius: 10px;
  background: #fff;
  color: #9aa4b2;
  box-sizing: border-box;
}

.works-header-search input {
  width: 100%;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: #344054;
  font-size: 13px;
}

.works-size-button {
  justify-content: center;
  min-width: 72px;
  height: 36px;
  min-height: 36px;
  border-radius: 10px;
  padding: 0 12px;
  font-size: 13px;
  font-weight: 750;
  line-height: 36px;
}

.works-center-page .online-pill.online {
  border-color: #bbf7d0;
  background: #ecfdf3;
  color: #047857;
}

.works-center-page .user-pill {
  border-color: #dfe5ee;
  background: #fff;
  color: #334155;
}

.app-shell.module-assets .works-center-scroll,
.works-center-scroll {
  height: 100%;
  padding: 18px 20px 30px;
  background: #f6f8ff;
  color: #101828;
}

.works-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  min-height: 132px;
  padding: 26px 34px;
  border: 1px solid #e1e7f2;
  border-radius: 14px;
  background:
    radial-gradient(circle at 86% 28%, rgba(105, 92, 244, .16), transparent 20%),
    linear-gradient(135deg, #fff, #f5f7ff);
  box-shadow: 0 14px 34px rgba(56, 72, 112, .07);
}

.works-hero-copy {
  display: grid;
  gap: 9px;
  min-width: 0;
}

.works-eyebrow {
  width: fit-content;
  padding: 0;
  background: transparent;
  color: #695cf4;
  font-size: 12px;
  font-weight: 850;
  line-height: 1.2;
}

.works-title {
  color: #172033;
  font-size: 30px;
  font-weight: 950;
  line-height: 1.15;
}

.works-subtitle {
  max-width: 680px;
  color: #5d6b82;
  font-size: 14px;
  font-weight: 650;
  line-height: 1.55;
}

.works-hero-actions,
.works-toolbar,
.works-filter-tabs,
.works-view-toggle,
.works-card-actions,
.works-table-actions {
  display: flex;
  align-items: center;
}

.works-hero-actions {
  gap: 10px;
}

.works-hero-actions button,
.works-card-actions button,
.works-table-actions button,
.works-filter-tabs button,
.works-view-toggle button,
.works-empty button {
  margin: 0;
  border: 1px solid #dbe4ef;
  background: #fff;
  color: #334155;
  font-weight: 900;
  box-sizing: border-box;
}

.works-hero-actions button::after,
.works-card-actions button::after,
.works-table-actions button::after,
.works-filter-tabs button::after,
.works-view-toggle button::after,
.works-empty button::after {
  border: 0;
}

.works-hero-actions button,
.works-empty button {
  min-height: 38px;
  border-radius: 10px;
  padding: 0 16px;
  font-size: 14px;
  line-height: 38px;
  border-color: transparent;
  background: #ebe7ff;
  color: #5b49e8;
}

.works-primary-action,
.works-empty button {
  border-color: transparent !important;
  background: linear-gradient(135deg, #7464f2, #5b49e8) !important;
  color: #fff !important;
  box-shadow: none;
}

.works-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin-top: 16px;
}

.works-summary-card {
  display: grid;
  gap: 7px;
  padding: 18px;
  border: 1px solid #e1e7f2;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 14px 34px rgba(56, 72, 112, .07);
}

.works-summary-card text:first-child {
  color: #667085;
  font-size: 12px;
  font-weight: 700;
}

.works-summary-card text:nth-child(2) {
  color: #111827;
  font-size: 26px;
  font-weight: 950;
  line-height: 1.1;
}

.works-summary-card text:last-child {
  color: #667085;
  font-size: 12px;
  font-weight: 700;
}

.works-panel {
  margin-top: 16px;
  padding: 18px;
  overflow: hidden;
  border: 1px solid #e1e7f2;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 14px 34px rgba(56, 72, 112, .07);
}

.works-panel-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 16px;
  align-items: center;
  margin-bottom: 16px;
}

.works-filter-tabs {
  flex-wrap: wrap;
  gap: 24px;
}

.works-filter-tabs button {
  min-height: 34px;
  border: 0;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  padding: 0;
  background: transparent;
  color: #667085;
  font-size: 13px;
  font-weight: 850;
  line-height: 34px;
}

.works-filter-tabs button.active {
  border-color: #6c5cf4;
  background: transparent;
  color: #5b49e8;
}

.works-toolbar {
  gap: 10px;
}

.works-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(300px, 28vw);
  min-height: 36px;
  padding: 0 12px;
  border: 1px solid #dde5f0;
  border-radius: 9px;
  background: #fff;
  color: #64748b;
  box-sizing: border-box;
}

.works-search:focus-within {
  border-color: #bcb4ff;
  box-shadow: 0 0 0 3px rgba(108, 92, 244, .14);
}

.works-search text {
  color: #64748b;
  font-size: 18px;
  line-height: 1;
}

.works-search input {
  width: 100%;
  min-width: 0;
  border: 0;
  background: transparent;
  color: #0f172a;
  font-size: 14px;
  outline: none;
}

.works-search input::placeholder {
  color: #94a3b8;
}

.works-view-toggle {
  overflow: hidden;
  border: 1px solid #dde5f0;
  border-radius: 9px;
  background: #fff;
}

.works-view-toggle button {
  min-height: 34px;
  border: 0;
  border-radius: 0;
  padding: 0 12px;
  color: #64748b;
  font-size: 13px;
  font-weight: 800;
  line-height: 34px;
}

.works-view-toggle button.active {
  background: #eeeaff;
  color: #5b49e8;
}

.works-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}
.works-center-scroll.density-compact .works-grid { grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); }
.works-center-scroll.density-compact .works-card-desc { display: none; }
.works-center-scroll.density-compact .works-card-preview { min-height: 120px; }

.works-card {
  overflow: hidden;
  border: 1px solid #e4e9f3;
  border-radius: 12px;
  background: #fff;
  cursor: pointer;
  box-shadow: none;
  transition: transform .18s ease, box-shadow .18s ease, border-color .18s ease;
}

.works-card:hover {
  border-color: #bcb4ff;
  box-shadow: 0 16px 34px rgba(66, 80, 124, .12);
  transform: translateY(-2px);
}

.works-card-preview {
  position: relative;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: linear-gradient(135deg, #eef3ff, #f8fafc);
}

.works-card-preview image {
  width: 100%;
  height: 100%;
}

.works-card-badge {
  position: absolute;
  left: 10px;
  top: 10px;
  border-radius: 999px;
  padding: 4px 9px;
  background: rgba(15, 23, 42, .64);
  color: #fff;
  font-size: 11px;
  font-weight: 850;
}

.works-card.is-success .works-card-badge {
  background: rgba(4, 120, 87, .82);
}

.works-card.is-running .works-card-badge {
  background: rgba(37, 99, 235, .82);
}

.works-card.is-failed .works-card-badge {
  background: rgba(185, 28, 28, .86);
}

.works-card-placeholder {
  display: grid;
  place-items: center;
  width: 100%;
  height: 100%;
  color: #5b49e8;
  background: transparent;
}

.works-card-placeholder text:first-child {
  display: block;
  width: auto;
  height: auto;
  border-radius: 0;
  background: transparent;
  color: #5b49e8;
  font-size: 18px;
  font-weight: 900;
  box-shadow: none;
}

.works-card-placeholder text:last-child {
  color: #64748b;
  font-size: 13px;
  font-weight: 900;
}

.works-card-body {
  display: grid;
  gap: 8px;
  padding: 12px 12px 10px;
}

.works-card-title,
.works-card-desc,
.works-card-meta text,
.works-table-name text,
.works-table-row > text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.works-card-title {
  color: #111827;
  font-size: 13px;
  font-weight: 950;
}

.works-card-desc {
  color: #667085;
  font-size: 12px;
  font-weight: 650;
}

.works-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.works-card-meta text {
  max-width: 100%;
  border-radius: 999px;
  padding: 3px 7px;
  background: #f2f4f8;
  color: #58677f;
  font-size: 11px;
  font-weight: 750;
}

.works-card-actions {
  flex-wrap: wrap;
  gap: 6px;
  padding: 0 12px 12px;
}

.works-card-actions button,
.works-table-actions button {
  min-height: 28px;
  border-radius: 8px;
  padding: 0 9px;
  color: #334155;
  font-size: 12px;
  font-weight: 800;
  line-height: 28px;
}

.works-card-actions button[disabled],
.works-table-actions button[disabled] {
  cursor: not-allowed;
  opacity: .42;
}

.works-card-actions button:first-child,
.works-table-actions button:first-child {
  border-color: #dfe6f2;
  background: #fff;
  color: #334155;
}

.works-table {
  overflow-x: auto;
  border: 1px solid #e6edf6;
  border-radius: 14px;
  background: #fff;
}

.works-table-head,
.works-table-row {
  display: grid;
  grid-template-columns: minmax(280px, 1.5fr) 88px 140px 116px 76px 130px 176px;
  gap: 12px;
  align-items: center;
  min-width: 1030px;
  padding: 12px 14px;
}

.works-table-head {
  color: #64748b;
  background: #f8fafc;
  font-size: 12px;
  font-weight: 900;
}

.works-table-row {
  border-top: 1px solid #edf2f7;
  color: #334155;
  font-size: 13px;
  font-weight: 750;
}

.works-table-name {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.works-table-name image,
.works-table-file {
  width: 52px;
  height: 42px;
  flex: 0 0 52px;
  border-radius: 10px;
}

.works-table-file {
  display: grid;
  place-items: center;
  background: #eef2ff;
  color: #2563eb;
  font-weight: 950;
}

.works-table-name > view {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.works-table-name text:first-child {
  color: #0f172a;
  font-weight: 950;
}

.works-table-name text:last-child {
  color: #94a3b8;
  font-size: 12px;
}

.works-table-actions {
  gap: 7px;
}

.works-empty {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 10px;
  min-height: 320px;
  border: 1px dashed #cbd5e1;
  border-radius: 14px;
  background: #f8fafc;
  text-align: center;
}

.works-empty text:first-child {
  color: #0f172a;
  font-size: 18px;
  font-weight: 950;
}

.works-empty text:nth-child(2) {
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

@media (max-width: 1180px) {
  .works-panel-head,
  .works-hero {
    grid-template-columns: 1fr;
  }

  .works-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .works-search {
    width: 100%;
  }
}

@media (max-width: 760px) {
  .works-center-scroll {
    padding: 14px 12px 28px;
  }

  .works-hero {
    padding: 20px;
    border-radius: 14px;
  }

  .works-title {
    font-size: 26px;
  }

  .works-hero-actions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    width: 100%;
  }

  .works-summary-grid,
  .works-grid {
    grid-template-columns: 1fr;
  }

  .works-panel {
    padding: 12px;
    border-radius: 14px;
  }

  .works-view-toggle {
    width: 100%;
  }

  .works-view-toggle button {
    flex: 1 1 0;
  }

  .works-card-preview {
    aspect-ratio: 4 / 3;
  }
}
</style>







