<template>
  <view v-if="!isLoggedIn" class="login-shell">
    <view class="login-card">
      <image class="login-logo" :src="xianzhiLogo" mode="aspectFit" />
      <text class="eyebrow">WELCOME</text>
      <text class="login-title">登录先知 AI</text>
      <text class="login-copy">代理商可查看客户绑定、佣金、提现和下级渠道，平台账号仍可进入统一工作台。</text>
      <view class="login-form">
        <label>
          <text>邮箱</text>
          <input v-model="loginEmail" type="text" placeholder="demo@xianzhi.ai" />
        </label>
        <label>
          <text>密码</text>
          <input v-model="loginPassword" type="password" placeholder="Demo123!" />
        </label>
        <button type="button" class="login-submit" @click="login">登录</button>
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
        'canvas-mode': currentWorkspace === 'user' && (activeModule === 'inspiration' || activeModule === 'assets'),
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
        <text class="brand-title">{{ currentWorkspace === 'user' ? '先知 AI' : workspaceTitle }}</text>
        <text class="brand-subtitle">AI Operating System</text>
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
        <text><text>{{ quota.toLocaleString('zh-CN') }}</text> / 50,000</text>
        <button type="button" @click="selectModule('membership')">去充值</button>
      </view>
      <text v-if="currentWorkspace === 'user'" class="sidebar-version">先知 AI v1.9.0</text>
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
            <button type="button" class="notification-button">
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
            <view class="quick-prompt-box" @click="selectModule('inspiration')">
              <text>描述你想要生成的内容，例如：未来科技城市全景，赛博朋克风格，夜景...</text>
              <text>0 / 1000</text>
            </view>
            <view class="quick-control-row">
              <view>
                <text>模型</text>
                <text>{{ selectedDashboardModelName }}</text>
              </view>
              <view>
                <text>比例</text>
                <text>16:9</text>
              </view>
              <view>
                <text>风格</text>
                <text>自动</text>
              </view>
              <button type="button" class="upload-ref-button" @click="selectModule('inspiration')">上传参考图</button>
              <button type="button" class="quick-generate-button" @click="selectModule('inspiration')">生成（消耗 {{ selectedDashboardModelCost }} 点）</button>
            </view>
            <text class="recent-strip-title">最近画布</text>
            <view class="recent-strip">
              <button v-for="asset in recentDashboardAssets" :key="asset.id" type="button" @click="selectModule('assets')">
                <image v-if="asset.mediaType === 'image'" :src="asset.thumbnailUrl || asset.url" mode="aspectFill" />
                <view v-else>{{ asset.mediaType }}</view>
                <text>{{ asset.name }}</text>
                <text>{{ formatShortDate(asset.createdAt || asset.updatedAt) }}</text>
              </button>
              <view v-if="!recentDashboardAssets.length" class="recent-empty">暂无最近作品</view>
              <button type="button" class="all-canvas-button" @click="selectModule('inspiration')">
                <text>→</text>
                <text>全部画布</text>
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
              <view v-if="!recentDashboardAssets.length" class="dashboard-empty-row">暂无作品，先去灵感画布生成第一张图。</view>
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
      <InspirationCanvas
        v-else-if="activeModule === 'inspiration'"
        :assets="assets"
        :models="models"
        :quota="quota"
        :default-model="model"
        @refresh="refresh"
        @generated="onInspirationGenerated"
      />

      <view v-else-if="activeModule === 'assets'" class="creation-page">
        <view class="topbar">
          <view class="topbar-brand">
            <image class="topbar-logo" :src="xianzhiLogo" mode="aspectFit" />
            <text class="brand-title">先知 AI</text>
          </view>
          <view class="tabs">
            <button :class="{ active: activeModule === 'assets' }" @click="selectModule('assets')">我的作品</button>
            <button>画廊</button>
          </view>
          <view class="topbar-status">
            <text :class="['online-pill', models.length ? 'online' : 'offline']">
              {{ models.length ? "ONLINE" : "OFFLINE" }}
            </text>
            <text class="user-pill"><text class="user-badge">先</text>先知 · 普通用户</text>
            <button type="button" class="logout-button" @click.stop="logout">退出</button>
          </view>
        </view>
        <scroll-view class="creation-assets workspace-scroll" scroll-y>
          <view class="section-head">
            <text class="section-title">我的作品</text>
            <text>生成资产、参考图和可交付内容统一归档。</text>
          </view>
          <view class="asset-grid">
            <view v-for="asset in assets" :key="asset.id" class="asset-card">
              <image v-if="asset.mediaType === 'image'" :src="asset.url" mode="aspectFit" />
              <view v-else class="asset-placeholder">{{ asset.mediaType }}</view>
              <text class="asset-name">{{ asset.name }}</text>
            </view>
            <view v-if="!assets.length" class="empty-card">暂无作品。</view>
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
                  <text>{{ customer.email }}</text>
                </view>
                <text>{{ customer.status }}</text>
              </view>
              <view v-if="!channelCenter.customers.length" class="channel-empty">暂无直属客户。</view>
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
                <text>{{ channelCenter.children.length }} 个</text>
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
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "../api/client";
import InspirationCanvas from "../components/InspirationCanvas.vue";
import xianzhiLogo from "../assets/xianzhi-ai-logo.png";
import type { Asset, AuthResponse, AuthUser, ChannelAgent, ChannelCenterResponse, GenerationTask, ModelInfo, PointAccount } from "../types";

type Workspace = "user" | "agent" | "admin";
type ModuleId =
  | "dashboard"
  | "inspiration"
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
  inspiration: "/app/inspiration-canvas",
  apiSettings: "/app/api-settings",
  assets: "/app/works",
  ppt: "/app/ai-ppt",
  agents: "/app/agents",
  geo: "/app/geo",
  membership: "/app/membership",
  usage: "/app/usage",
  admin: "/admin/",
  agentHome: "/agent",
  agentCustomers: "/agent/customers",
  agentCommissions: "/agent/commissions",
  agentWithdrawals: "/agent/withdrawals",
  agentMaterials: "/agent/materials",
  agentChildren: "/agent/children",
  agentSettings: "/agent/settings"
};

const legacyRoutes: Record<string, ModuleId> = {
  "/dashboard": "dashboard",
  "/inspiration-canvas": "inspiration",
  "/api-settings": "apiSettings",
  "/works": "assets",
  "/ai-ppt": "ppt",
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

const userModules: ModuleConfig[] = [
  { id: "dashboard", label: "用户首页", description: "创作任务、积分、作品和常用能力入口。", status: "用户工作台", capabilities: ["创作概览", "积分状态", "作品入口"] },
  { id: "inspiration", label: "灵感画布", description: "在线生图、参考图、模型、尺寸、生成和归档复用。", status: "已接入 Go API", capabilities: ["在线生图", "参考图", "画布归档", "Reuse"] },
  { id: "apiSettings", label: "API 设置", description: "管理生图平台、请求地址、API Key 和可用模型。", status: "本地配置", capabilities: ["平台列表", "请求地址", "Key 管理", "模型列表"] },
  { id: "assets", label: "作品中心", description: "管理生成资产、参考文件和可下载交付物。", status: "已接入 Go API", capabilities: ["资产列表", "图片预览", "作品归档"] },
  { id: "ppt", label: "AI PPT", description: "选题、大纲、页面编辑和 PPTX/PDF 导出。", status: "待迁移 Go API", capabilities: ["PPT 项目", "页面编辑", "PPTX 导出", "PDF 导出"] },
  { id: "agents", label: "智能体", description: "智能体创建、工作流编排、发布、调用与反馈。", status: "待迁移 Go API", capabilities: ["工作流节点", "知识库绑定", "版本回滚", "调用反馈"] },
  { id: "geo", label: "GEO 优化", description: "品牌监测、竞品分析、趋势报告和优化内容生成。", status: "待迁移 Go API", capabilities: ["品牌管理", "定时监测", "周报", "内容发布跟踪"] },
  { id: "membership", label: "会员订单", description: "套餐、积分、订单、支付、发票和兑换码。", status: "待迁移 Go API", capabilities: ["会员套餐", "支付订单", "优惠券", "兑换码"] },
  { id: "usage", label: "使用记录", description: "按模型、时间和点数查看生成消耗明细。", status: "用户工作台", capabilities: ["点数消耗", "模型统计", "明细导出"] }
];

const agentModules: ModuleConfig[] = [
  { id: "agentHome", label: "代理首页", description: "代理商身份、邀请码、客户和收益总览。", status: "代理商中心", capabilities: ["邀请码", "收益总览", "客户概览"] },
  { id: "agentCustomers", label: "我的客户", description: "查看通过邀请关系绑定的直属客户。", status: "代理商中心", capabilities: ["客户绑定", "客户状态", "客户来源"] },
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
const isLoggedIn = ref(false);
const currentUser = ref<AuthUser | null>(null);
const currentAgent = ref<ChannelAgent | null>(null);
const channelCenter = ref<ChannelCenterResponse | null>(null);
const loginEmail = ref("demo@xianzhi.ai");
const loginPassword = ref("Demo123!");
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

const sidebarModules = computed(() => workspaceModules[currentWorkspace.value]);
const currentModule = computed(() => sidebarModules.value.find(item => item.id === activeModule.value) || sidebarModules.value[0]);
const quota = computed(() => pointAccount.value?.available || 0);
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
const selectedDashboardModel = computed(() => models.value.find(item => item.code === model.value) || models.value[0]);
const selectedDashboardModelName = computed(() => selectedDashboardModel.value?.name || selectedDashboardModel.value?.code || "默认模型");
const selectedDashboardModelCost = computed(() => {
  const item = selectedDashboardModel.value;
  const direct = item?.pointCost || item?.fixedQuota;
  return typeof direct === "number" && Number.isFinite(direct) && direct > 0 ? direct : 1;
});
const planUsagePercent = computed(() => `${Math.min(100, Math.max(6, Math.round((quota.value / 50000) * 100)))}%`);
const configuredApiCount = computed(() => apiProviders.value.filter(item => item.apiKey).length || Math.min(3, apiProviders.value.length));
const dashboardMetrics = computed<Array<{ label: string; value: string | number; hint: string; icon: string; action: string; tone: string; module: ModuleId }>>(() => [
  {
    label: "可用点数",
    value: quota.value.toLocaleString("zh-CN"),
    hint: "总点数 50,000",
    icon: "点",
    action: "去充值",
    tone: "teal",
    module: "membership"
  },
  {
    label: "今日生成",
    value: tasks.value.length || 128,
    hint: "昨日 96",
    icon: "生",
    action: "+33.3%",
    tone: "blue",
    module: "inspiration"
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
const workspaceEyebrow = computed(() => ({ user: "USER WORKSPACE", agent: "AGENT WORKSPACE", admin: "ADMIN WORKSPACE" })[currentWorkspace.value]);
const workspaceHeroTitle = computed(() => {
  if (currentWorkspace.value === "agent") return "专注客户、佣金和提现";
  if (currentWorkspace.value === "admin") return "平台运营与主控管理";
  return "让创意成为可交付成果";
});
const workspaceHeroCopy = computed(() => {
  if (currentWorkspace.value === "agent") return "代理商只保留推广、客户、佣金、提现和团队管理入口，不再混入普通创作菜单。";
  if (currentWorkspace.value === "admin") return "管理员从平台视角管理用户、订单、模型、代理商审核和财务。";
  return "内容生产、智能体、GEO 增长和会员订单属于普通用户工作台。";
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
  syncModuleFromLocation();
  window.addEventListener("popstate", syncModuleFromLocation);
  void restoreAuthenticatedSession();
});

onBeforeUnmount(() => {
  window.removeEventListener("popstate", syncModuleFromLocation);
});


async function restoreAuthenticatedSession() {
  if (isAdminPath(window.location.pathname)) {
    window.location.replace("/admin/");
    return;
  }
  if (isAgentPath(window.location.pathname)) {
    window.location.replace("/agent/");
    return;
  }
  const token = uni.getStorageSync("token");
  if (!token || window.location.pathname === loginRoute) {
    isLoggedIn.value = false;
    return;
  }
  try {
    const auth = await api<AuthResponse>("/api/v1/auth/me");
    const workspace = workspaceFromAuth(auth);
    if (workspace === "admin") {
      window.location.replace("/admin/");
      return;
    }
    if (workspace === "agent") {
      window.location.replace("/agent/");
      return;
    }
    applyAuth(auth);
    await refresh();
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
    api<GenerationTask[]>("/api/v1/generation-tasks"),
    api<Asset[]>("/api/v1/assets"),
    api<ModelInfo[]>("/api/v1/models"),
    api<{ account: PointAccount }>("/api/v1/points/account")
  ]);
  tasks.value = taskItems;
  assets.value = assetItems;
  models.value = modelItems;
  pointAccount.value = points.account;
  model.value = models.value.find(item => item.code === "gpt-image-2")?.code || models.value[0]?.code || model.value;
}
async function refreshChannelCenter() {
  try {
    channelCenter.value = await api<ChannelCenterResponse>("/api/v1/channel/me");
    currentAgent.value = channelCenter.value.agent;
  } catch (error) {
    channelCenter.value = null;
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
function onInspirationGenerated(task: GenerationTask) {
  tasks.value = [task, ...tasks.value.filter(item => item.id !== task.id)];
}

function isAdminPath(pathname: string) {
  return (pathname.replace(/\/$/, "") || "/") === "/admin";
}

function isAgentPath(pathname: string) {
  const normalized = pathname.replace(/\/$/, "") || "/";
  return normalized === "/agent" || normalized.startsWith("/agent/");
}

function syncModuleFromLocation() {
  const path = window.location.pathname.replace(/\/$/, "") || "/";
  if (isAdminPath(window.location.pathname)) {
    window.location.replace("/admin/");
    return;
  }
  if (path === loginRoute) {
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
  if (auth.workspace === "agent" || auth.workspace === "admin" || auth.workspace === "user") return auth.workspace;
  if (auth.user.role.startsWith("AGENT")) return "agent";
  if (auth.user.role === "SUPER_ADMIN") return "admin";
  return "user";
}

function moduleFromAuth(auth: AuthResponse): ModuleId {
  const module = auth.defaultModule as ModuleId;
  if (moduleRoutes[module]) return module;
  const workspace = workspaceFromAuth(auth);
  return workspace === "agent" ? "agentHome" : workspace === "admin" ? "admin" : "dashboard";
}

function isAgentModule(id: ModuleId) {
  return moduleWorkspace[id] === "agent";
}

function isAgentDataModule(id: ModuleId) {
  return id === "agentCustomers" || id === "agentCommissions" || id === "agentWithdrawals" || id === "agentChildren";
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
  uni.removeStorageSync("token");
  currentUser.value = null;
  currentAgent.value = null;
  channelCenter.value = null;
  isLoggedIn.value = false;
  isModuleDrawerOpen.value = false;
  window.history.pushState({ loggedOut: true }, "", loginRoute);
  if (showMessage) uni.showToast({ title: "已退出", icon: "success" });
}

async function login() {
  if (!loginEmail.value.trim() || !loginPassword.value.trim()) {
    uni.showToast({ title: "请输入账号密码", icon: "none" });
    return;
  }
  try {
    const auth = await api<AuthResponse>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email: loginEmail.value.trim(), password: loginPassword.value.trim() })
    });
    if (auth.accessToken) uni.setStorageSync("token", auth.accessToken);
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
    return;
  } catch (error) {
    uni.showToast({ title: error instanceof Error ? error.message : "登录失败", icon: "none" });
  }
}
</script>







