<template>
  <el-config-provider :size="currentElementSize">
  <ConnectorAuthorizationCenter v-if="isConnectorAuthorizationRoute" />
  <FeishuConnectorSetup v-else-if="isFeishuConnectorSetupRoute" />
  <WebLoginPage v-else-if="isLoginRoute" :register-href="authRegisterHref" @authenticated="handleWebLoginAuthenticated" />
  <section v-else-if="isRegisterRoute" class="admin-auth-shell">
    <div class="admin-auth-card">
      <div class="admin-auth-brand">
        <img :src="xianzhiLogo" alt="知启云 AI" />
        <div>
          <strong>知启云 AI</strong>
          <span>{{ isRegisterRoute ? "Invite Register" : "Unified Login" }}</span>
        </div>
      </div>
      <div class="admin-auth-head">
        <el-tag effect="dark" type="primary">{{ isRegisterRoute ? "邀请注册" : "统一入口" }}</el-tag>
        <h1>{{ isRegisterRoute ? "注册知启云 AI" : "登录知启云 AI" }}</h1>
        <p>{{ isRegisterRoute ? "通过代理商邀请注册后，账号会自动绑定来源渠道。" : "一个入口进入用户端、代理端和主控后台。" }}</p>
      </div>

      <form v-if="isRegisterRoute" class="admin-auth-form" @submit.prevent="submitRegister">
        <label>
          <span>用户名</span>
          <input v-model.trim="registerForm.username" autocomplete="name" placeholder="请输入用户名" />
        </label>
        <label>
          <span>邮箱</span>
          <input v-model.trim="registerForm.email" autocomplete="email" placeholder="your@email.com" />
        </label>
        <label>
          <span>密码</span>
          <input v-model="registerForm.password" autocomplete="new-password" type="password" placeholder="至少 8 位" />
        </label>
        <label>
          <span>确认密码</span>
          <input v-model="registerForm.confirmPassword" autocomplete="new-password" type="password" placeholder="再次输入密码" />
        </label>
        <label>
          <span>邀请码</span>
          <input v-model.trim="registerForm.inviteCode" autocomplete="off" placeholder="可选，代理邀请链接会自动带入" />
        </label>
        <label class="admin-auth-check">
          <input v-model="registerAgreementAccepted" type="checkbox" />
          <span>我已阅读并同意《用户协议》和《隐私政策》</span>
        </label>
        <button class="admin-auth-submit" type="submit" :disabled="authSubmitting">{{ authSubmitting ? "注册中..." : "注册并进入工作台" }}</button>
        <a class="admin-auth-link" :href="authLoginHref">已有账号，去登录</a>
      </form>

    </div>
  </section>
  <el-container v-else-if="authReady" :class="['admin-shell', 'pure-admin-shell', { 'user-console-shell': isUserConsole, 'user-agent-figma-shell': isUserConsole && store.activeModuleId === 'userAgentCenter', 'mobile-drawer-open': mobileDrawerOpen, 'desktop-sidebar-collapsed': desktopSidebarCollapsed }]">
    <div v-if="mobileDrawerOpen" class="mobile-drawer-mask" @click="mobileDrawerOpen = false"></div>
    <el-aside width="200px" class="admin-sidebar">
      <div class="brand">
        <button class="brand-home-button" type="button" :title="brandHomeTitle" :aria-label="brandHomeTitle" @click="goBrandHome">
          <img class="brand-logo" :src="xianzhiLogo" alt="知启云 AI" />
          <span class="brand-copy">
            <strong>知启云 AI</strong>
            <small>{{ isUserConsole ? "User Console" : isAgentConsole ? "Agent Console" : "Master SaaS Console" }}</small>
          </span>
        </button>
      </div>
      <div class="sidebar-section-label">{{ isUserConsole ? "用户导航" : isAgentConsole ? "代理导航" : "平台导航" }}</div>
      <nav v-if="!isUserConsole" class="collapsed-icon-menu" aria-label="折叠模块导航">
        <div v-for="group in visibleModuleGroups" :key="group.id" :class="['collapsed-icon-group', { 'is-active': isGroupActive(group) }]">
          <button class="collapsed-icon-button" type="button" :aria-label="group.title" @click="selectAdminModule(group.items[0]?.id || store.activeModuleId)">
            <el-icon><component :is="group.icon" /></el-icon>
          </button>
          <div class="collapsed-flyout" role="menu">
            <strong>{{ group.title }}</strong>
            <button v-for="item in group.items" :key="item.id" :class="{ 'is-active': item.id === activeSidebarModuleId }" type="button" role="menuitem" @click.stop="selectAdminModule(item.id)">
              <el-icon><component :is="iconFor(item.id)" /></el-icon>
              <span>{{ item.title }}</span>
            </button>
          </div>
        </div>
      </nav>
      <el-menu v-if="isUserConsole" class="sidebar-menu user-flat-sidebar-menu" :default-active="activeUserMenuId" @select="selectUserFlatMenu">
        <el-menu-item v-for="item in userFlatMenuItems" :key="item.id" :index="item.id" :aria-label="item.title" :title="desktopSidebarCollapsed ? item.title : undefined">
          <el-tooltip :content="item.title" placement="right" :disabled="!desktopSidebarCollapsed" :show-after="120" :hide-after="0" popper-class="user-sidebar-tooltip">
            <span class="user-sidebar-tooltip-target">
              <el-icon><component :is="item.icon" /></el-icon>
            </span>
          </el-tooltip>
          <span class="user-sidebar-menu-title">{{ item.title }}</span>
        </el-menu-item>
      </el-menu>
      <el-menu v-else class="sidebar-menu" :default-active="activeSidebarModuleId" @select="selectAdminModule">
        <el-sub-menu v-for="group in visibleModuleGroups" :key="group.id" :index="group.id">
          <template #title>
            <el-icon><component :is="group.icon" /></el-icon>
            <span>{{ group.title }}</span>
          </template>
          <el-menu-item v-for="item in group.items" :key="item.id" :index="item.id">
            <el-icon><component :is="iconFor(item.id)" /></el-icon>
            <span>{{ item.title }}</span>
          </el-menu-item>
        </el-sub-menu>
      </el-menu>

      <aside v-if="isUserConsole" class="sidebar-plan-card">
        <template v-if="isGuestUser">
          <span>当前身份</span>
          <div class="sidebar-plan-title"><strong>游客</strong><em>体验中</em></div>
          <small>登录后查看会员、额度和创作记录</small>
          <button type="button" @click="openWorkspaceLogin">登录后继续</button>
        </template>
        <template v-else>
        <span>当前套餐</span>
        <div class="sidebar-plan-title">
          <strong>{{ sidebarPlan.name }}</strong>
          <em>{{ sidebarPlan.status }}</em>
        </div>
        <small>有效期至：{{ sidebarPlan.expiresAt }}</small>
        <div class="sidebar-plan-progress"><i :style="{ width: sidebarPlan.percent + '%' }"></i></div>
        <span>可用点数</span>
        <strong class="sidebar-plan-points">{{ sidebarPlan.availableText }} <small>/ {{ sidebarPlan.totalText }}</small></strong>
        <button type="button" @click="selectAdminModule('userMembership')">去充值</button>
        </template>
      </aside>
    </el-aside>
    <el-container class="admin-workspace">
      <div class="mobile-admin-bar">
        <el-button class="mobile-collapse-button" :icon="Grid" aria-label="打开模块导航" @click="mobileDrawerOpen = true" />
        <div class="mobile-admin-title">
          <strong>{{ isUserConsole ? "用户后台" : isAgentConsole ? "代理商后台" : "主控 SaaS" }}</strong>
          <small>{{ activeHeaderModuleTitle }}</small>
        </div>
        <div class="mobile-admin-actions">
          <el-tag :type="store.error ? 'danger' : 'success'" effect="light">{{ store.error ? "API ERROR" : "API ONLINE" }}</el-tag>
          <el-button v-if="isGuestUser" class="mobile-account-button" :icon="UserFilled" circle aria-label="登录" @click="openWorkspaceLogin" />
          <el-dropdown v-else trigger="click" @command="handleAccountCommand">
            <el-button class="mobile-account-button" :icon="UserFilled" circle aria-label="账号操作" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile"><el-icon><UserFilled /></el-icon><span>账号信息</span></el-dropdown-item>
                <el-dropdown-item command="password"><el-icon><Lock /></el-icon><span>修改密码</span></el-dropdown-item>
                <el-dropdown-item class="logout-dropdown-item" command="logout" divided><el-icon><SwitchButton /></el-icon><span>退出登录</span></el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
      <el-header class="admin-header">
        <el-button class="admin-collapse-button" :icon="Grid" circle @click="toggleDesktopSidebar" />
        <div class="header-title">
          <div class="header-path">
            <el-icon><component :is="activeGroupIcon" /></el-icon>
            <span v-if="isUserConsole" class="header-single-title">{{ activeHeaderModuleTitle }}</span>
            <el-breadcrumb v-else separator="/">
              <el-breadcrumb-item>{{ activeGroupLabel }}</el-breadcrumb-item>
              <el-breadcrumb-item>{{ activeHeaderModuleTitle }}</el-breadcrumb-item>
            </el-breadcrumb>
          </div>
        </div>
        <div class="header-actions">
          <el-input v-if="!isUserConsole && !isAgentConsole" v-model="searchKeyword" class="header-search" :prefix-icon="Search" clearable placeholder="搜索当前模块" />
          <el-input v-else v-model="globalSearchKeyword" class="header-search" :prefix-icon="Search" clearable placeholder="全局搜索菜单与业务记录（Ctrl K）" @focus="commandPaletteOpen = true" @keydown.enter="commandPaletteOpen = true" />
          <el-button :icon="Refresh" circle :loading="store.loading" @click="() => store.loadActiveModule()" />
          <el-dropdown trigger="click" @command="setElementSize">
            <el-button class="size-button">
              <span>{{ elementSizeLabel }}</span>
              <el-icon><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="item in elementSizeOptions" :key="item.value" :command="item.value">{{ item.label }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-tag :type="store.error ? 'danger' : 'success'" effect="light">{{ store.error ? "API ERROR" : "API ONLINE" }}</el-tag>
          <el-button v-if="isGuestUser" class="account-button" @click="openWorkspaceLogin">
            <el-icon><UserFilled /></el-icon><span class="account-button-copy"><strong>游客</strong><small>点击登录</small></span>
          </el-button>
          <el-dropdown v-else trigger="click" @command="handleAccountCommand">
            <el-button class="account-button">
              <el-icon><UserFilled /></el-icon>
              <span class="account-button-copy">
                <strong>{{ currentAdmin?.name || "平台管理员" }}</strong>
                <small v-if="currentAdmin?.email">{{ currentAdmin.email }}</small>
              </span>
              <el-icon><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile"><el-icon><UserFilled /></el-icon><span>账号信息</span></el-dropdown-item>
                <el-dropdown-item command="password"><el-icon><Lock /></el-icon><span>修改密码</span></el-dropdown-item>
                <el-dropdown-item class="logout-dropdown-item" command="logout" divided><el-icon><SwitchButton /></el-icon><span>退出登录</span></el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <nav v-if="!isUserConsole" class="admin-page-tabs" aria-label="已打开页面标签">
        <button class="tabs-rail-button" type="button" aria-label="向左滚动标签" @click="scrollOpenTabs(-1)">«</button>
        <div ref="tabsScrollRef" class="tabs-scroll">
          <button v-for="tab in openTabs" :key="tab.id" :class="['page-tab', { 'is-active': tab.id === store.activeModuleId }]" type="button" @click="selectAdminModule(tab.id)">
            <span>{{ tab.title }}</span>
            <i v-if="openTabs.length > 1" role="button" aria-label="关闭标签" @click.stop="closeOpenTab(tab.id)">×</i>
          </button>
        </div>
        <button class="tabs-rail-button" type="button" aria-label="向右滚动标签" @click="scrollOpenTabs(1)">»</button>
        <button class="tabs-tool-button" type="button" aria-label="刷新当前页" @click="() => store.loadActiveModule()"><el-icon><Refresh /></el-icon></button>
        <el-dropdown trigger="click" @command="handleTabsCommand">
          <button class="tabs-tool-button" type="button" aria-label="标签页更多操作"><el-icon><Setting /></el-icon></button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="refresh"><el-icon><Refresh /></el-icon><span>刷新当前</span></el-dropdown-item>
              <el-dropdown-item command="closeOthers"><span>关闭其它</span></el-dropdown-item>
              <el-dropdown-item command="closeLeft"><span>关闭左侧</span></el-dropdown-item>
              <el-dropdown-item command="closeRight"><span>关闭右侧</span></el-dropdown-item>
              <el-dropdown-item command="closeAll" divided><span>关闭全部</span></el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </nav>
      <el-main class="admin-main">
        <el-alert v-if="store.error" :title="store.error" type="error" show-icon class="admin-alert" />
        <el-skeleton v-if="store.loading" :rows="10" animated />
        <section v-else class="page-stack">
          <AdminSectionTabs
            v-if="activeAdminNavigation && activeAdminSectionTabs.length > 1"
            :group-title="activeAdminNavigation.group.title"
            :section-title="activeAdminNavigation.section.title"
            :active-module-id="store.activeModuleId"
            :tabs="activeAdminSectionTabs"
            @select="selectAdminModule"
          />
          <section v-if="!isUserConsole && !isAgentConsole && searchKeyword.trim()" class="global-search-panel">
            <div class="global-search-head">
              <div><strong>搜索结果</strong><small>关键词：{{ searchKeyword.trim() }}</small></div>
              <el-button size="small" @click="searchKeyword = ''">清空</el-button>
            </div>
            <div class="global-search-grid">
              <article class="global-search-card">
                <span>模块入口</span>
                <button v-for="item in globalModuleResults" :key="item.id" type="button" @click="selectAdminModule(item.id)"><strong>{{ item.title }}</strong><small>{{ pageMeta[item.id]?.description || '进入模块查看详情' }}</small></button>
                <el-empty v-if="!globalModuleResults.length" description="没有匹配模块" :image-size="56" />
              </article>
              <article class="global-search-card">
                <span>当前模块数据</span>
                <button v-for="item in currentRecordResults" :key="item.key" type="button" @click="openCurrentRecordResult(item)"><strong>{{ item.title }}</strong><small>{{ item.desc }}</small></button>
                <el-empty v-if="!currentRecordResults.length" description="当前模块没有匹配记录" :image-size="56" />
              </article>
            </div>
          </section>
          <section v-if="!['analysis', 'workbench', 'partnerDashboard', 'operationCenterDashboard', 'userDashboard', 'userAgentCenter', 'knowledgeAdmin', 'storageCenter', ...mediaOperationModuleIds, ...imageWorkspaceModuleIds, 'userWirelessCanvas', 'userVideoGeneration', 'userPptGeneration', 'userMembership', 'userOrders', ...aiCapabilityModuleIds, 'apiSettings', ...billingModuleIds, ...enterpriseModuleIds, ...customerAttributionModuleIds].includes(store.activeModuleId)" class="module-hero">
            <div>
              <el-tag effect="dark" type="primary">{{ activeModuleMeta.badge }}</el-tag>
              <h2>{{ store.activeModule.title }}</h2>
              <p>{{ activeModuleMeta.description }}</p>
            </div>
            <div class="module-hero-actions">
              <el-button v-for="action in toolbarActions" :key="action.action" type="primary" :icon="Plus" @click="runAction(action.action)">{{ action.label }}</el-button>
              <el-button :icon="Refresh" @click="() => store.loadActiveModule()">刷新数据</el-button>
            </div>
          </section>
          <div v-if="!['analysis', 'workbench', 'partnerDashboard', 'operationCenterDashboard', 'userDashboard', 'userAgentCenter', 'knowledgeAdmin', 'storageCenter', ...mediaOperationModuleIds, ...imageWorkspaceModuleIds, 'userWirelessCanvas', 'userVideoGeneration', 'userPptGeneration', 'userMembership', 'userOrders', ...aiCapabilityModuleIds, 'apiSettings', ...billingModuleIds, ...enterpriseModuleIds, ...customerAttributionModuleIds].includes(store.activeModuleId)" class="metric-grid">
            <article v-for="metric in metrics" :key="metric.label" class="metric-card">
              <span>{{ metric.label }}</span>
              <strong>{{ metric.value }}</strong>
              <small>{{ metricHint(metric.label) }}</small>
            </article>
          </div>
          <AiCapabilityDomain v-if="aiCapabilityModuleIds.includes(store.activeModuleId)" :model="aiCapabilityViewModel" />
          <!-- AI capability presentation moved to components/ai/AiCapabilityDomain.vue. -->
          <section v-else-if="store.activeModuleId === 'userDashboard'" class="user-home-page">
            <section class="user-home-layout">
              <main class="user-home-main">
                <section class="user-home-hero">
                  <span class="user-home-shard user-home-shard-left"></span>
                  <span class="user-home-shard user-home-shard-right"></span>
                  <div class="user-home-title-block">
                    <h2>专属 AI 视觉设计师已待命，<em>即刻开启创作</em></h2>
                    <p>输入想法、上传参考图，或从下面的 Agent 入口开始一段创作。</p>
                  </div>

                  <div class="user-home-composer">
                    <div class="user-home-mode-tabs">
                      <button v-for="mode in userHomeCreationModes" :key="mode.id" type="button" :class="{ active: userHomeCreationMode === mode.id }" @click="selectUserHomeCreationMode(mode.id)">
                        <el-icon><component :is="mode.icon" /></el-icon>
                        <span>{{ mode.title }}</span>
                      </button>
                    </div>
                    <el-input v-model="onlineImageForm.prompt" type="textarea" :rows="4" maxlength="2000" resize="none" placeholder="描述你想生成的画面、视频或文档..." />
                    <div class="user-home-toolbar">
                      <button type="button" @click="selectAdminModule('userAiImage')"><el-icon><Plus /></el-icon>上传参考图</button>
                      <button type="button" @click="applyUserHomePrompt('润色')"><el-icon><EditPen /></el-icon>润色</button>
                      <button type="button" @click="applyUserHomePrompt('爆款复刻')"><el-icon><Star /></el-icon>爆款复刻</button>
                      <el-select v-model="onlineImageForm.model" class="user-home-select" size="default">
                        <el-option v-for="model in onlineModelOptions" :key="model.value" :label="model.label" :value="model.value" />
                      </el-select>
                      <el-select v-model="onlineImageForm.ratio" class="user-home-select is-compact" size="default">
                        <el-option v-for="ratio in userHomeRatioOptions" :key="ratio.value" :label="ratio.label" :value="ratio.value" />
                      </el-select>
                      <el-select v-model="onlineImageForm.quality" class="user-home-select is-compact" size="default">
                        <el-option label="标准" value="standard" />
                        <el-option label="高清" value="hd" />
                        <el-option label="高质量" value="high" />
                      </el-select>
                      <button type="button" class="user-home-generate" @click="launchUserHomeCreation">生成 <el-icon><Star /></el-icon></button>
                    </div>
                  </div>
                </section>

                <section class="user-home-agent-row" aria-label="快捷 Agent">
                  <button v-for="agent in userHomeAgentEntries" :key="agent.title" type="button" @click="openUserHomeEntry(agent)">
                    <el-icon><component :is="agent.icon" /></el-icon>
                    <span>{{ agent.title }}</span>
                  </button>
                </section>

                <section class="user-home-template-section">
                  <div class="user-home-section-head">
                    <h3>模板广场</h3>
                    <button type="button" @click="selectAdminModule('userAiImage')">查看全部</button>
                  </div>
                  <div class="user-home-template-grid">
                    <button v-for="template in userHomeTemplates" :key="template.title" type="button" class="user-home-template-card" @click="openUserHomeEntry(template)">
                      <span :class="['user-home-template-cover', template.coverClass]"><em>{{ template.coverText }}</em></span>
                      <strong>{{ template.title }}</strong>
                      <small>{{ template.desc }}</small>
                    </button>
                  </div>
                </section>
              </main>

              <aside class="user-home-inspiration">
                <div class="user-home-inspiration-head">
                  <h3>灵感模板</h3>
                </div>
                <button v-for="item in userHomeInspirations" :key="item.title" type="button" class="user-home-inspiration-item" @click="openUserHomeEntry(item)">
                  <span>
                    <strong>{{ item.title }}</strong>
                    <small>{{ item.desc }}</small>
                  </span>
                  <i :class="['user-home-inspiration-thumb', item.coverClass]"></i>
                </button>
              </aside>
            </section>
          </section>
          <section v-else-if="false" class="online-image-page online-image-studio">
            <section class="online-studio-shell">
              <div class="online-studio-compose">
                <div class="online-studio-head">
                  <div>
                    <span class="online-kicker">GPT / NANO BANANA IMAGE STUDIO</span>
                    <strong>在线生图</strong>
                    <small>参考 Infinite-Canvas 在线页面结构：Prompt、参考图、平台模型、尺寸比例、结果预览和任务记录。</small>
                  </div>
                  <div class="online-protocol-badges">
                    <el-tag size="small" effect="plain">OpenAI 协议</el-tag>
                    <el-tag size="small" effect="plain">RunningHub</el-tag>
                    <el-tag size="small" effect="plain">Gemini</el-tag>
                    <el-tag size="small" effect="plain">ComfyUI</el-tag>
                  </div>
                </div>

                <label class="online-prompt-block">
                  <span>Prompt</span>
                  <el-input v-model="onlineImageForm.prompt" type="textarea" :rows="6" maxlength="1000" show-word-limit placeholder="输入想生成或编辑的画面..." />
                </label>

                <div class="online-reference-section">
                  <div class="online-section-title"><span>Reference Images</span><small>最多 3 张参考图，支持文生图 / 图生图 / 风格复用</small></div>
                  <div class="online-reference-grid">
                    <el-upload v-for="slot in onlineReferenceSlots" :key="slot" :auto-upload="false" :show-file-list="false" :on-change="handleOnlineReferenceUpload" accept="image/*" :disabled="onlineReferenceImages.length >= onlineReferenceSlots.length" class="online-upload-slot">
                      <button type="button" class="online-upload-card">
                        <img v-if="onlineReferenceImages[slot - 1]" :src="aiReferencePreviewUrl(onlineReferenceImages[slot - 1])" :alt="onlineReferenceImages[slot - 1].name" />
                        <template v-else>
                          <el-icon><Plus /></el-icon>
                          <strong>参考图 {{ slot }}</strong>
                          <small>点击上传</small>
                        </template>
                      </button>
                    </el-upload>
                  </div>
                </div>

                <div class="online-section-title"><span>Model</span><small>{{ onlineProviderModeLabel }}</small></div>
                <div class="online-control-grid online-source-controls">
                  <label><span>平台</span><el-select v-model="onlineImageForm.provider"><el-option v-for="provider in onlineProviderOptions" :key="provider.value" :label="provider.label" :value="provider.value" /></el-select></label>
                  <label><span>模型</span><el-select v-model="onlineImageForm.model"><el-option v-for="model in onlineModelOptions" :key="model.value" :label="model.label" :value="model.value" /></el-select></label>
                  <label><span>质量</span><el-select v-model="onlineImageForm.quality"><el-option label="标准" value="standard" /><el-option label="高清" value="high" /><el-option label="快速草稿" value="draft" /></el-select></label>
                  <label><span>数量</span><el-select v-model="onlineImageForm.count"><el-option v-for="count in onlineCountOptions" :key="count" :label="`×${count}`" :value="count" /></el-select></label>
                </div>

                <div class="online-section-title"><span>Size</span><small>支持 1K、2K、自定义尺寸和自定义比例</small></div>
                <div class="online-control-grid online-size-controls">
                  <label><span>尺寸</span><el-select v-model="onlineImageForm.resolution"><el-option label="1K" value="1k" /><el-option label="2K" value="2k" /><el-option label="自定义尺寸" value="custom" /></el-select></label>
                  <label><span>比例</span><el-select v-model="onlineImageForm.ratio"><el-option label="比例为空" value="" /><el-option label="1:1 方图" value="square" /><el-option label="16:9 横图" value="16:9" /><el-option label="9:16 竖图" value="9:16" /><el-option label="自定义比例" value="custom" /></el-select></label>
                  <label><span>宽度</span><el-input-number v-model="onlineImageForm.width" :min="64" :step="64" controls-position="right" /></label>
                  <label><span>高度</span><el-input-number v-model="onlineImageForm.height" :min="64" :step="64" controls-position="right" /></label>
                  <label><span>消耗点数预估</span><strong class="online-cost">{{ onlineEstimatedCost }} 点</strong></label>
                  <label><span>尺寸工具</span><el-button class="online-fit-button" @click="fitOnlineImageSize">适配图片</el-button></label>
                </div>

                <div class="online-compose-actions">
                  <el-button type="primary" :icon="Plus" :loading="onlineSubmitting" @click="submitOnlineImage">生成图片</el-button>
                </div>
              </div>

              <aside class="online-preview-panel">
                <div class="online-panel-head">
                  <div><strong>生成结果</strong><small>预览、下载、复用风格、加入作品或发送到灵感画布</small></div>
                  <el-tag :type="onlinePreviewStatus.type">{{ onlinePreviewStatus.label }}</el-tag>
                </div>
                <div class="online-preview-canvas">
                  <img v-if="onlinePreviewImage" :src="onlinePreviewImage" alt="在线生图结果预览" />
                  <div v-else class="online-preview-empty">
                    <el-icon><Monitor /></el-icon>
                    <strong>等待生成结果</strong>
                    <small>提交任务后，最新结果将在这里预览。</small>
                  </div>
                </div>
                <div class="online-preview-actions">
                  <el-button @click="runOnlineResultAction('download')">下载</el-button>
                  <el-button @click="runOnlineResultAction('reuse')">复用风格</el-button>
                  <el-button @click="runOnlineResultAction('works')">加入作品</el-button>
                  <el-button type="primary" @click="runOnlineResultAction('canvas')">发送到灵感画布</el-button>
                </div>
                <div class="online-provider-list compact">
                  <article v-for="provider in onlineProviders.slice(0, 3)" :key="String(provider.id)" class="online-provider-card">
                    <div><i :class="['provider-dot', providerStatusClass(provider.status)]"></i><strong>{{ provider.name || provider.id }}</strong></div>
                    <small>{{ provider.baseUrl || '-' }}</small>
                    <footer><span>{{ provider.latencyMs || '-' }} ms</span><span>{{ provider.apiKeyConfigured ? '已配置' : '待配置' }}</span></footer>
                  </article>
                </div>
              </aside>
            </section>

            <section class="online-queue-grid">
              <article v-for="item in onlineQueueCards" :key="item.label" class="online-queue-card"><span>{{ item.label }}</span><strong>{{ item.value }}</strong></article>
            </section>

            <section class="online-history-panel">
              <div class="online-panel-head"><div><strong>最近生成</strong><small>按任务更新时间展示，可复用提示词和风格</small></div><el-button link type="primary" @click="selectAdminModule('userWorks')">作品中心</el-button></div>
              <div class="online-history-grid">
                <article v-for="item in onlineHistoryItems" :key="String(item.id)" class="online-history-card">
                  <div class="online-history-thumb"><img v-if="aiTaskThumbnailUrl(item)" :src="aiTaskThumbnailUrl(item)" alt="作品缩略图" /><span v-else>{{ String(item.model || 'AI').slice(0, 2) }}</span></div>
                  <strong>{{ item.name || item.prompt || item.id }}</strong>
                  <small>{{ item.model || '-' }} · {{ item.pointCost || 0 }} 点</small>
                </article>
              </div>
            </section>

            <el-card shadow="never" class="data-panel online-task-panel">
              <template #header><div class="panel-head"><div><span>生成队列</span><small>{{ onlineImageTasks.length }} 条任务</small></div><el-segmented v-model="onlineStatusFilter" :options="onlineStatusOptions" /></div></template>
              <el-table v-if="filteredOnlineTasks.length" :data="filteredOnlineTasks" height="420" stripe>
                <el-table-column prop="id" label="任务" min-width="130" />
                <el-table-column prop="model" label="模型" min-width="150" />
                <el-table-column prop="type" label="类型" width="130" />
                <el-table-column prop="pointCost" label="消耗点数" width="110" />
                <el-table-column prop="status" label="状态" width="120"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
                <el-table-column prop="createdAt" label="创建时间" min-width="210" show-overflow-tooltip />
              </el-table>
              <el-empty v-else description="暂无在线生图任务" />
            </el-card>
          </section>
          <section v-else-if="store.activeModuleId === 'userAgentCenter'" :class="['user-agent-center-page', { 'has-officecli-workspace': officeCLIWorkspaceOpen || agentCenterWorkspace, 'has-agent-workspace': officeCLIWorkspaceOpen || agentCenterWorkspace }]">
            <div class="user-agent-desktop-view">
              <section v-if="officeCLIWorkspaceOpen" class="user-agent-officecli-workspace">
                <header class="officecli-workspace-head">
                  <button type="button" @click="closeOfficeCLIWorkspace">返回智能体中心</button>
                  <div>
                    <span>OfficeCLI 文档智能体</span>
                    <h2>文档生成工作台</h2>
                    <p>选择 Word、Excel 或 PPT，输入需求后由后端 OfficeCLI 运行层生成可下载文件。</p>
                  </div>
                  <em :class="['officecli-status-badge', officeCLIStatusTone]">{{ officeCLIStatusLabel }}</em>
                </header>

                <section class="user-agent-officecli-workbench is-workspace">
                  <header>
                    <div>
                      <span>文档生成控制台</span>
                      <strong>输入需求，一键生成 Office 文件</strong>
                    </div>
                    <button type="button" :disabled="officeCLIDocumentGenerating" @click="submitOfficeCLIDocument">
                      {{ officeCLIDocumentGenerating ? "生成中..." : "生成文档" }}
                    </button>
                  </header>
                  <div class="officecli-workbench-body">
                    <div class="officecli-form-column">
                      <div class="officecli-format-switch" role="radiogroup" aria-label="选择文档格式">
                        <button v-for="format in officeCLIFormatOptions" :key="format.value" type="button" :class="{ active: officeCLIForm.format === format.value }" @click="officeCLIForm.format = format.value">
                          <b>{{ format.label }}</b>
                          <span>{{ format.desc }}</span>
                        </button>
                      </div>
                      <label class="officecli-field">
                        <span>文档标题</span>
                        <input v-model.trim="officeCLIForm.title" placeholder="例如：AI 产品周报" />
                      </label>
                      <label class="officecli-field">
                        <span>生成需求</span>
                        <textarea v-model.trim="officeCLIForm.prompt" rows="5" placeholder="描述你要生成的内容，例如：生成一份面向客户的 OfficeCLI 能力介绍，包含产品价值、适用场景和下一步计划。" />
                      </label>
                    </div>
                    <aside class="officecli-result-card">
                      <span>生成结果</span>
                      <template v-if="officeCLIDocumentResult">
                        <strong>{{ officeCLIDocumentResult.fileName }}</strong>
                        <small>{{ officeCLIDocumentResult.format.toUpperCase() }} · {{ officeCLIDocumentSizeText }}</small>
                        <button type="button" @click="downloadOfficeCLIDocument()">下载文件</button>
                      </template>
                      <template v-else>
                        <strong>等待生成</strong>
                        <small>生成后会在这里出现下载入口，并保存在后端容器数据目录。</small>
                      </template>
                    </aside>
                  </div>
                </section>
              </section>

              <KnowledgeAgentCenter v-else-if="agentCenterWorkspace?.agentKey === 'knowledge'" @close="closeAgentWorkspace" />
              <section v-else-if="agentCenterWorkspace" class="user-agent-workspace">
                <header class="agent-workspace-head">
                  <button type="button" @click="closeAgentWorkspace">返回智能体中心</button>
                  <span :class="['agent-workspace-avatar', agentCenterWorkspace.tone]">{{ agentCenterWorkspace.avatar }}</span>
                  <div>
                    <span>{{ agentCenterWorkspace.modeLabel }}</span>
                    <h2>{{ agentCenterWorkspace.name }}</h2>
                    <p>{{ agentCenterWorkspace.desc }}</p>
                  </div>
                  <em :class="['agent-workspace-status', { draft: agentCenterWorkspace.status === '草稿', disabled: agentCenterWorkspace.status === '已停用' }]">{{ agentCenterWorkspace.status }}</em>
                </header>

                <section class="agent-workspace-grid">
                  <main class="agent-workspace-chat">
                    <header>
                      <div>
                        <span>交互控制台</span>
                        <strong>{{ agentCenterWorkspace.headline }}</strong>
                      </div>
                      <button type="button" @click="sendAgentWorkspaceMessage">发送测试</button>
                    </header>
                    <div class="agent-workspace-dialog">
                      <article v-for="(message, index) in agentWorkspaceMessages" :key="`${message.role}-${index}`" :class="['agent-message', message.role]">
                        <span>{{ message.role === 'user' ? '我' : agentCenterWorkspace.avatar }}</span>
                        <p>{{ message.text }}</p>
                      </article>
                    </div>
                    <label class="agent-workspace-input">
                      <span>测试指令</span>
                      <textarea v-model.trim="agentWorkspaceDraft" rows="5" placeholder="输入一条测试指令，检查这个智能体的回复风格与业务边界。" />
                    </label>
                  </main>

                  <aside class="agent-workspace-config">
                    <section class="agent-workspace-card">
                      <strong>运行信息</strong>
                      <div class="agent-workspace-meta-grid">
                        <div><span>类型</span><b>{{ agentCenterWorkspace.type }}</b></div>
                        <div><span>模型</span><b>{{ agentCenterWorkspace.model }}</b></div>
                        <div><span>知识库</span><b>{{ agentCenterWorkspace.knowledge }}</b></div>
                        <div><span>调用次数</span><b>{{ agentCenterWorkspace.calls }}</b></div>
                      </div>
                    </section>
                    <section class="agent-workspace-card">
                      <strong>能力配置</strong>
                      <div class="agent-tool-tags">
                        <span v-for="tag in agentCenterWorkspace.toolTags" :key="tag">{{ tag }}</span>
                      </div>
                    </section>
                    <section class="agent-workspace-card">
                      <strong>快捷动作</strong>
                      <div class="agent-quick-actions">
                        <button v-for="action in agentCenterWorkspace.quickActions" :key="action" type="button" @click="agentWorkspaceDraft = action">{{ action }}</button>
                      </div>
                    </section>
                  </aside>
                </section>
              </section>

              <section v-else class="user-agent-center-layout">
                <main class="user-agent-main-column">
                  <section class="user-agent-center-hero">
                    <div>
                      <span>AGENT CENTER</span>
                      <h2>智能体中心</h2>
                      <p>创建、调试与运行你的 AI 智能体，连接知识库、工具与业务流程。</p>
                    </div>
                    <div class="user-agent-hero-robot" aria-hidden="true">
                      <i></i>
                      <b></b>
                      <em>AI</em>
                      <strong>BOT</strong>
                    </div>
                  </section>

                  <section class="user-agent-template-panel">
                    <header>
                      <strong>智能体模板</strong>
                      <button type="button" @click="selectAdminModule('userAgentCenter')">全部模板 ></button>
                    </header>
                    <div class="user-agent-template-grid">
                      <article
                        v-for="template in agentCenterTemplates"
                        :key="template.name"
                        :class="['user-agent-template-card', { 'is-featured': template.featured, 'is-clickable': true }]"
                        tabindex="0"
                        @click="handleAgentTemplateCardClick(template)"
                        @keydown.enter.prevent="handleAgentTemplateCardClick(template)"
                      >
                        <div :class="['user-agent-template-icon', template.tone]">{{ template.icon }}</div>
                        <strong>{{ template.name }}</strong>
                        <p>{{ template.desc }}</p>
                        <button type="button" @click.stop="handleAgentTemplateAction(template)">{{ template.action || "进入" }}</button>
                      </article>
                    </div>
                  </section>

                  <main class="user-agent-list-panel">
                    <header class="user-agent-list-head">
                      <div class="user-agent-tabs">
                        <button v-for="tab in agentCenterTabs" :key="tab.value" type="button" :class="{ active: agentCenterListTab === tab.value }" @click="agentCenterListTab = tab.value">{{ tab.label }}</button>
                      </div>
                      <div class="user-agent-list-tools">
                        <label><el-icon><Search /></el-icon><input v-model.trim="agentCenterSearch" placeholder="搜索智能体..." /></label>
                        <select v-model="agentCenterTypeFilter" aria-label="筛选智能体类型"><option value="all">全部类型</option><option v-for="type in agentCenterTypeOptions" :key="type" :value="type">{{ type }}</option></select>
                        <button type="button" @click="selectAdminModule('userAgentCenter')">+ 创建智能体</button>
                      </div>
                    </header>
                    <div class="user-agent-table">
                      <div class="user-agent-table-head">
                        <span>智能体名称</span><span>类型</span><span>状态</span><span>模型</span><span>知识库</span><span>调用次数</span><span>更新时间</span><span>操作</span>
                      </div>
                      <div
                        v-for="agent in visibleAgentCenterRows"
                        :key="agent.name"
                        :class="['user-agent-table-row', { 'is-officecli': agent.officecli, 'is-clickable': true }]"
                        tabindex="0"
                        @click="handleAgentRowAction(agent)"
                        @keydown.enter.prevent="handleAgentRowAction(agent)"
                      >
                        <div class="user-agent-name-cell">
                          <span :class="['user-agent-avatar', agent.tone]">{{ agent.avatar }}</span>
                          <div><strong>{{ agent.name }}</strong><small>{{ agent.desc }}</small></div>
                        </div>
                        <span :class="['user-agent-pill', agent.tone]">{{ agent.type }}</span>
                        <span :class="['user-agent-status', { disabled: agent.status === '已停用', draft: agent.status === '草稿' }]">{{ agent.status }}</span>
                        <span>{{ agent.model }}</span>
                        <span>{{ agent.knowledge }}</span>
                        <span>{{ agent.calls }}</span>
                        <span>{{ agent.updated }}</span>
                        <div class="user-agent-row-actions">
                          <button type="button" class="is-wide" @click.stop="handleAgentRowAction(agent)">进入</button>
                          <button type="button" title="编辑" aria-label="编辑智能体" @click.stop="openAgentWorkspace(agent)"><el-icon><EditPen /></el-icon></button>
                          <button type="button" title="复制" aria-label="复制智能体配置" @click.stop="copyAgentCenterConfig(agent)"><el-icon><CopyDocument /></el-icon></button>
                          <button type="button" :title="isAgentCenterFavorite(agent) ? '取消收藏' : '收藏'" :aria-label="isAgentCenterFavorite(agent) ? '取消收藏智能体' : '收藏智能体'" @click.stop="toggleAgentCenterFavorite(agent)"><el-icon><component :is="isAgentCenterFavorite(agent) ? StarFilled : Star" /></el-icon></button>
                        </div>
                      </div>
                    </div>
                  </main>

                </main>

                <aside class="user-agent-side-panel">
                  <article class="user-agent-side-card is-metrics">
                    <header><strong>智能体数据概览</strong><select v-model="agentCenterRange" aria-label="智能体数据时间范围"><option value="7">近 7 天</option><option value="30">近 30 天</option><option value="90">近 90 天</option></select></header>
                    <div class="user-agent-metric-grid">
                      <div v-for="metric in agentCenterMetrics" :key="metric.label">
                        <span>{{ metric.label }}</span>
                        <strong>{{ metric.value }}</strong>
                        <small>↑ {{ metric.trend }}</small>
                      </div>
                    </div>
                  </article>
                  <article class="user-agent-side-card is-trend">
                    <header><strong>使用趋势</strong><select v-model="agentCenterRange" aria-label="使用趋势时间范围"><option value="7">近 7 天</option><option value="30">近 30 天</option><option value="90">近 90 天</option></select></header>
                    <div class="user-agent-trend">
                      <span v-for="bar in agentCenterTrend" :key="bar.label"><i :style="{ height: bar.height }"></i><em>{{ bar.label }}</em></span>
                    </div>
                  </article>
                  <article class="user-agent-side-card is-ranking">
                    <strong>使用最多的智能体</strong>
                    <ol class="user-agent-ranking">
                      <li v-for="(item, index) in agentCenterRanking" :key="item.name"><span>{{ index + 1 }}</span><b>{{ item.name }}</b><em>{{ item.calls }}</em></li>
                    </ol>
                  </article>
                  <article class="user-agent-side-card is-shortcuts">
                    <strong>快速入口</strong>
                    <div class="user-agent-shortcuts">
                      <button v-for="item in agentCenterShortcuts" :key="item.label" type="button" @click="handleAgentCenterShortcut(item.label)"><span>{{ item.icon }}</span>{{ item.label }}</button>
                    </div>
                  </article>
                </aside>
              </section>
            </div>

            <div class="user-agent-mobile-view">
              <div class="user-agent-mobile-status">
                <span>9:41</span>
                <span>5G ▰</span>
              </div>
              <header class="user-agent-mobile-top">
                <div>
                  <h2>智能体中心</h2>
                  <p>创建、调试与发布你的 AI 智能体</p>
                </div>
                <button type="button" @click="selectAdminModule('userAgentCenter')">+</button>
              </header>
              <label class="user-agent-mobile-search"><i></i><input placeholder="搜索智能体名称或描述..." /></label>

              <section class="user-agent-mobile-overview">
                <div>
                  <strong>本周智能体运行概览</strong>
                  <p>7 个智能体正在服务业务流程</p>
                </div>
                <div class="user-agent-mobile-bot" aria-hidden="true">AI</div>
                <div class="user-agent-mobile-metrics">
                  <span>调用<strong>5,689</strong></span>
                  <span>对话<strong>12,458</strong></span>
                  <span>Token<strong>2.45M</strong></span>
                </div>
              </section>

              <section class="user-agent-mobile-section-head">
                <strong>智能体模板</strong>
                <button type="button" @click="selectAdminModule('userAgentCenter')">全部模板 ></button>
              </section>
              <div class="user-agent-mobile-template-scroll">
                <article
                  v-for="template in agentCenterTemplates.slice(0, 4)"
                  :key="template.name"
                  class="user-agent-mobile-template-card is-clickable"
                  @click="handleAgentTemplateCardClick(template)"
                >
                  <span :class="['user-agent-template-icon', template.tone]">{{ template.icon }}</span>
                  <strong>{{ template.name.replace('智能体', '').replace('企业知识库', '知识库问答') }}</strong>
                  <small>{{ template.desc.split('，')[0] }}</small>
                </article>
              </div>

              <section class="user-agent-mobile-section-head">
                <strong>我的智能体</strong>
                <button type="button" @click="agentCenterTypeFilter = agentCenterTypeFilter === 'all' ? (agentCenterTypeOptions[0] || 'all') : 'all'">{{ agentCenterTypeFilter === 'all' ? '全部类型⌄' : `${agentCenterTypeFilter}⌄` }}</button>
              </section>
              <div class="user-agent-mobile-list">
                <article v-for="agent in agentCenterMobileRows" :key="agent.name" class="user-agent-mobile-agent-card is-clickable" @click="handleAgentRowAction(agent)">
                  <span :class="['user-agent-avatar', agent.tone]">{{ agent.avatar }}</span>
                  <div>
                    <strong>{{ agent.name }}</strong>
                    <small>{{ agent.type }} · {{ agent.model }}</small>
                  </div>
                  <em>{{ agent.status }}</em>
                  <b>{{ agent.calls }} 次</b>
                </article>
              </div>
              <button class="user-agent-mobile-all" type="button" @click="selectAdminModule('userAgentCenter')">查看全部智能体 ></button>
              <nav class="user-agent-mobile-bottom" aria-label="移动端导航">
                <button v-for="item in agentMobileBottomNav" :key="item.label" type="button" :class="{ active: item.targetId === store.activeModuleId }" @click="selectAdminModule(item.targetId)">
                  <span>{{ item.letter }}</span>
                  <small>{{ item.label }}</small>
                </button>
              </nav>
            </div>
          </section>

          <section v-else-if="store.activeModuleId === 'userWorks'" class="user-works-page">
            <header class="user-works-hero">
              <div>
                <span>Works Center</span>
                <h2>作品中心</h2>
                <p>集中管理 AI 生图、参考资产、收藏作品和可交付文件，不再混入创作输入区。</p>
              </div>
              <div class="user-works-hero-actions">
                <button type="button" @click="selectAdminModule('userAiImage')">继续创作</button>
                <button type="button" @click="refreshWorksCenter">刷新作品</button>
              </div>
            </header>

            <nav class="user-works-source-tabs" aria-label="作品来源">
              <button type="button" :class="{ active: worksSourceTab === 'official' }" @click="worksSourceTab = 'official'">官方精选</button>
              <button type="button" :class="{ active: worksSourceTab === 'mine' }" @click="openMyWorks">
                我的作品 <span v-if="isGuestUser">登录后查看</span>
              </button>
            </nav>

            <div v-if="isGuestUser && worksSourceTab === 'official'" class="user-works-guest-tip">
              <div><strong>先浏览官方精选案例</strong><span>登录后可查看、保存和同步你的私有作品。</span></div>
              <button type="button" @click="openMyWorks">登录后查看我的作品</button>
            </div>

            <section class="user-works-summary">
              <article v-for="item in userWorkSummaryCards" :key="item.label">
                <span>{{ item.label }}</span>
                <strong>{{ item.value }}</strong>
                <small>{{ item.hint }}</small>
              </article>
            </section>

            <section class="user-works-panel">
              <header class="user-works-toolbar">
                <div class="user-works-tabs">
                  <button
                    v-for="item in userWorkStatusOptions"
                    :key="item.value"
                    type="button"
                    :class="{ active: worksStatusFilter === item.value }"
                    @click="worksStatusFilter = item.value"
                  >
                    {{ item.label }}
                  </button>
                </div>
                <div class="user-works-tools">
                  <label><el-icon><Search /></el-icon><input v-model.trim="worksSearchKeyword" placeholder="搜索作品、提示词、模型..." /></label>
                  <div class="user-works-view-switch">
                    <button type="button" :class="{ active: worksViewMode === 'grid' }" @click="worksViewMode = 'grid'">宫格</button>
                    <button type="button" :class="{ active: worksViewMode === 'table' }" @click="worksViewMode = 'table'">列表</button>
                  </div>
                </div>
              </header>

              <div v-if="worksViewMode === 'grid' && userWorkCards.length" class="user-works-grid">
                <article
                  v-for="task in userWorkCards"
                  :key="aiTaskId(task)"
                  :class="['user-work-card', aiTaskStatusClass(task)]"
                  @click="previewAiTask(task)"
                  @mouseenter="prefetchAiOriginalImage(task)"
                >
                  <div class="user-work-thumb">
                    <img v-if="aiTaskThumbnailUrl(task)" :src="aiTaskThumbnailUrl(task)" alt="作品缩略图" loading="lazy" decoding="async" />
                    <div v-else-if="isAiTaskRunning(task)" class="user-work-placeholder">生成中</div>
                    <div v-else class="user-work-placeholder">AI</div>
                    <span>{{ statusLabel(task.status) }}</span>
                  </div>
                  <div class="user-work-body">
                    <strong>{{ task.name || task.prompt || 'AI 生图作品' }}</strong>
                    <p>{{ task.prompt || '暂无提示词' }}</p>
                    <div>
                      <span>{{ aiTaskModelLabel(task) }}</span>
                      <span>{{ aiTaskDisplayResolutionLabel(task) }}</span>
                      <span>{{ task.pointCost || 0 }} 点</span>
                    </div>
                  </div>
                  <footer class="user-work-actions">
                    <button type="button" @click.stop="previewAiTask(task)">预览</button>
                    <button type="button" @click.stop="reuseUserWorkTask(task)">复用</button>
                    <button type="button" :disabled="!aiTaskImageUrl(task)" @click.stop="downloadAiTask(task)">下载</button>
                    <button type="button" @click.stop="openAiFavoritePicker([aiTaskId(task)])">收藏</button>
                  </footer>
                </article>
              </div>

              <div v-else-if="worksViewMode === 'table' && userWorkCards.length" class="user-works-table">
                <div class="user-works-table-head">
                  <span>作品</span><span>状态</span><span>模型</span><span>尺寸</span><span>消耗</span><span>创建时间</span><span>操作</span>
                </div>
                <div v-for="task in userWorkCards" :key="aiTaskId(task)" class="user-works-table-row">
                  <div class="user-works-name-cell">
                    <img v-if="aiTaskThumbnailUrl(task)" :src="aiTaskThumbnailUrl(task)" alt="作品缩略图" />
                    <span v-else>AI</span>
                    <div><strong>{{ task.name || task.prompt || 'AI 生图作品' }}</strong><small>{{ task.prompt || aiTaskId(task) }}</small></div>
                  </div>
                  <span>{{ statusLabel(task.status) }}</span>
                  <span>{{ aiTaskModelLabel(task) }}</span>
                  <span>{{ aiTaskDisplayResolutionLabel(task) }}</span>
                  <span>{{ task.pointCost || 0 }} 点</span>
                  <span>{{ formatAiTaskTime(task) }}</span>
                  <div class="user-works-row-actions">
                    <button type="button" @click="previewAiTask(task)">预览</button>
                    <button type="button" @click="reuseUserWorkTask(task)">复用</button>
                    <button type="button" :disabled="!aiTaskImageUrl(task)" @click="downloadAiTask(task)">下载</button>
                  </div>
                </div>
              </div>

              <div v-else class="user-works-empty">
                <strong>暂无匹配作品</strong>
                <span>调整筛选条件，或先去 AI 生图创建新作品。</span>
                <button type="button" @click="selectAdminModule('userAiImage')">去创作</button>
              </div>
            </section>
          </section>

          <section v-else-if="imageWorkspaceModuleIds.includes(store.activeModuleId)" class="ai-image-page">
            <section class="ai-playground">
              <div class="ai-playground-header">
                <h3>{{ imageWorkspaceTitle }}</h3>
                <div class="ai-playground-actions">
                  <el-segmented v-model="aiPlaygroundMode" :options="aiPlaygroundModeOptions" />
                  <el-button class="ai-header-action" :icon="Download" aria-label="导出图片" @click="runAiPlaygroundAction('download')" />
                  <el-button class="ai-header-action" :icon="QuestionFilled" aria-label="操作指南" @click="runAiPlaygroundAction('help')" />
                  <el-button class="ai-header-action" :icon="Setting" aria-label="参数设置" @click="runAiPlaygroundAction('settings')" />
                </div>
              </div>
              <div :class="['ai-playground-toolbar', { 'is-collection-overview': isAiFavoriteCollectionOverview }]">
                <el-button :class="['ai-icon-button', { 'is-active': aiFavoriteOnly }]" :icon="Star" :aria-label="aiActiveFavoriteCollectionId ? '返回收藏夹' : aiFavoriteOnly ? '退出收藏夹' : '收藏夹'" @click="handleAiFavoriteFilterClick" />
                <el-button v-if="isAiFavoriteCollectionOverview" class="ai-icon-button" :icon="Collection" aria-label="收藏夹管理" @click="aiFavoriteCollectionsVisible = true" />
                <template v-else>
                  <el-select v-model="aiGalleryFilter" class="ai-gallery-filter" placeholder="全部">
                    <el-option label="全部" value="all" />
                    <el-option label="已完成" value="done" />
                    <el-option label="生成中" value="running" />
                    <el-option label="失败" value="error" />
                  </el-select>
                  <el-input v-model="aiPromptSearch" class="ai-playground-search" :prefix-icon="Search" clearable placeholder="搜索提示词、参数..." />
                  <el-button v-if="aiGalleryFilter === 'error'" class="ai-clear-failed-button" :disabled="!aiFailedVisibleTasks.length" @click="clearAiFailedTasks">清除失败</el-button>
                </template>
              </div>
              <div v-if="aiSelectedFavoriteCollectionIds.length" class="ai-batch-bar">
                <span>已选择 {{ aiSelectedFavoriteCollectionIds.length }} 个收藏夹</span>
                <button type="button" @click="selectAllAiVisibleFavoriteCollections">全选收藏夹</button>
                <button type="button" @click="invertAiVisibleFavoriteCollectionSelection">反选收藏夹</button>
                <button type="button" @click="downloadSelectedAiFavoriteCollections">下载选中</button>
                <button type="button" class="danger" @click="confirmDeleteSelectedAiFavoriteCollections">删除选中</button>
                <button type="button" @click="aiSelectedFavoriteCollectionIds = []">取消</button>
              </div>
              <div v-else-if="aiSelectedTaskIds.length" class="ai-batch-bar">
                <button type="button" title="取消选择" aria-label="取消选择" @click="clearAiTaskSelection">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
                <i></i>
                <button type="button" class="is-blue" title="全选任务" aria-label="全选任务" @click="selectAllAiVisibleTasks">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><rect x="3" y="3" width="18" height="18" rx="2" ry="2" stroke-width="2" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4" /></svg>
                </button>
                <button type="button" class="is-purple" title="反选任务" aria-label="反选任务" @click="invertAiVisibleTaskSelection">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" stroke-dasharray="4 4" d="M19 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h8M13 9l3 3-3 3" /></svg>
                </button>
                <i></i>
                <button type="button" class="is-yellow" title="编辑收藏夹" aria-label="编辑收藏夹" @click="favoriteSelectedAiTasks">
                  <svg v-if="areSelectedAiTasksFavorite()" viewBox="0 0 24 24" fill="currentColor"><path d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118L3.077 10.1c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" /></svg>
                  <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" /></svg>
                </button>
                <i></i>
                <button type="button" class="is-green" title="下载选中" aria-label="下载选中" @click="downloadSelectedAiTasks">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
                </button>
                <i></i>
                <button type="button" class="is-red" title="删除选中" aria-label="删除选中" @click="confirmDeleteSelectedAiTasks">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                </button>
              </div>

              <div class="ai-playground-canvas">
                <div v-if="aiPlaygroundMode === 'agent'" class="ai-agent-workspace">
                  <aside class="ai-agent-history">
                    <header><strong>历史对话</strong><button type="button" @click="createAiAgentConversation">新对话</button></header>
                    <button v-for="conversation in aiAgentConversations" :key="conversation.id" type="button" :class="{ active: conversation.id === aiAgentActiveConversationId }" @click="selectAiAgentConversation(conversation.id)">
                      <strong>{{ conversation.title }}</strong>
                      <small>{{ conversation.messages.length }} 条消息</small>
                    </button>
                  </aside>
                  <section class="ai-agent-chat">
                    <div class="ai-agent-messages">
                      <article v-for="(message, index) in activeAiAgentConversation?.messages || []" :key="index" :class="['ai-agent-message', message.role]">
                        <span>{{ message.role === 'user' ? '你' : 'Agent' }}</span>
                        <p>{{ message.content }}</p>
                      </article>
                    </div>
                    <div class="ai-agent-status">
                      <span>{{ aiAgentRunning ? 'Agent 正在处理当前生成任务...' : 'Agent 空闲，可提交提示词开始规划。' }}</span>
                      <button v-if="aiAgentRunning" type="button" @click="stopAiAgent">停止生成</button>
                    </div>
                  </section>
                </div>
                <div v-else-if="isAiFavoriteCollectionOverview" class="ai-collection-overview-grid">
                  <article v-for="collection in aiFavoriteCollectionCards" :key="collection.id" class="ai-collection-card" @click="selectAiFavoriteCollection(collection.id)">
                    <label v-if="!collection.virtual" class="ai-task-select" @click.stop>
                      <input type="checkbox" :checked="aiSelectedFavoriteCollectionIds.includes(collection.id)" @change="toggleAiFavoriteCollectionSelection(collection.id)" />
                    </label>
                    <div class="ai-collection-card-icon"><el-icon><Collection /></el-icon></div>
                    <div class="ai-collection-card-main">
                      <strong>{{ collection.name }}</strong>
                      <span>{{ collection.count }} 项收藏 · {{ collection.imageCount }} 张图片</span>
                    </div>
                    <div v-if="!collection.virtual" class="ai-collection-card-actions" @click.stop>
                      <button type="button" @click="setAiDefaultFavoriteCollection(collection.id)">{{ aiDefaultFavoriteCollectionId === collection.id ? '默认' : '设默认' }}</button>
                      <button type="button" @click="startRenameAiFavoriteCollection(collection)">重命名</button>
                      <button type="button" class="danger" @click="confirmDeleteAiFavoriteCollection(collection.id)">删除</button>
                    </div>
                  </article>
                </div>
                <template v-else-if="aiGalleryCards.length">
                  <div class="ai-gallery-grid">
                    <article
                      v-for="task in visibleAiGalleryCards"
                      :key="String(task.id || task.name)"
                      :class="['ai-task-card', aiTaskStatusClass(task), { 'is-selected': aiSelectedTaskIds.includes(aiTaskId(task)), 'is-selection-mode': aiMobileSelectionMode }]"
                      @click="handleAiTaskCardClick(task, $event)"
                      @mouseenter="prefetchAiOriginalImage(task)"
                      @focusin="prefetchAiOriginalImage(task)"
                      @touchstart.passive="handleAiTaskTouchStart(task, $event)"
                      @touchmove.passive="handleAiTaskTouchMove"
                      @touchend.passive="handleAiTaskTouchEnd"
                      @touchcancel.passive="handleAiTaskTouchEnd"
                    >
                      <div v-if="aiSelectedTaskIds.includes(aiTaskId(task))" class="ai-task-selected-corner" aria-hidden="true">
                        <el-icon><Check /></el-icon>
                      </div>
                      <div class="ai-task-thumb">
                        <div class="ai-task-badges">
                          <span v-if="isAiTaskRunning(task)" class="ai-task-badge is-time">
                            <i></i>
                            {{ aiTaskDuration(task) }}
                          </span>
                          <template v-else>
                            <span class="ai-task-badge">{{ aiTaskRatioLabel(task) }}</span>
                            <span class="ai-task-badge">{{ aiTaskResolutionLabel(task) }}</span>
                          </template>
                        </div>
                        <img
                          v-if="aiTaskThumbnailUrl(task) && !isAiTaskThumbnailBroken(task)"
                          :src="aiTaskThumbnailUrl(task)"
                          alt=""
                          loading="lazy"
                          decoding="async"
                          @error="markAiTaskThumbnailFailed(task)"
                        />
                        <div v-else-if="isAiTaskRunning(task)" class="ai-task-running">
                          <span class="ai-task-spinner"></span>
                          <strong>生成中...</strong>
                        </div>
                        <div v-else-if="isAiTaskFailed(task)" class="ai-task-failed">
                          <el-icon><Monitor /></el-icon>
                          <strong>生成失败</strong>
                        </div>
                        <div v-else class="ai-task-empty-thumb">
                          <el-icon><Monitor /></el-icon>
                          <strong>暂无预览</strong>
                        </div>
                      </div>
                      <div class="ai-task-info">
                        <div class="ai-task-copy">
                          <strong>{{ task.prompt || task.name || 'AI 生图任务' }}</strong>
                          <span v-if="isAiTaskFailed(task)" class="ai-task-error">{{ aiTaskErrorMessage(task) }}</span>
                        </div>
                        <div class="ai-task-pills">
                          <span class="ai-task-model-pill">&lt;/&gt; {{ aiTaskModelLabel(task) }}</span>
                        </div>
                        <div class="ai-task-actions">
                          <el-tooltip :content="isAiTaskFavorite(task) ? '编辑收藏夹' : '收藏任务'" placement="top" :show-after="180">
                            <button
                              type="button"
                              :class="{ 'is-favorite': isAiTaskFavorite(task) }"
                              :aria-label="isAiTaskFavorite(task) ? '编辑收藏夹' : '收藏任务'"
                              :title="isAiTaskFavorite(task) ? '编辑收藏夹' : '收藏任务'"
                              @click.stop="openAiFavoritePicker([aiTaskId(task)])"
                            >
                              <el-icon><component :is="isAiTaskFavorite(task) ? StarFilled : Star" /></el-icon>
                            </button>
                          </el-tooltip>
                          <el-tooltip :content="isAiTaskFailed(task) ? '重试生成' : '复用配置'" placement="top" :show-after="180">
                            <button
                              type="button"
                              :class="{ retry: isAiTaskFailed(task) }"
                              :aria-label="isAiTaskFailed(task) ? '重试生成' : '复用配置'"
                              :title="isAiTaskFailed(task) ? '重试生成' : '复用配置'"
                              :disabled="onlineSubmitting && isAiTaskFailed(task)"
                              @click.stop="isAiTaskFailed(task) ? retryAiTask(task) : reuseAiTask(task)"
                            >
                              <el-icon><Refresh /></el-icon>
                              <span v-if="isAiTaskFailed(task)">重试</span>
                            </button>
                          </el-tooltip>
                          <el-tooltip :content="aiTaskImageUrl(task) ? '编辑输出' : '图片生成完成后才能编辑输出'" placement="top" :show-after="180">
                            <span class="ai-task-action-wrap">
                              <button
                                type="button"
                                aria-label="编辑输出"
                                :title="aiTaskImageUrl(task) ? '编辑输出' : '图片生成完成后才能编辑输出'"
                                :disabled="!aiTaskImageUrl(task)"
                                @click.stop="editAiTaskOutput(task)"
                              >
                                <el-icon><EditPen /></el-icon>
                              </button>
                            </span>
                          </el-tooltip>
                          <el-tooltip content="删除任务" placement="top" :show-after="180">
                            <button type="button" class="danger" aria-label="删除任务" title="删除任务" @click.stop="deleteAiTask(task)">
                              <el-icon><Delete /></el-icon>
                            </button>
                          </el-tooltip>
                        </div>
                      </div>
                    </article>
                    <div v-if="hasMoreAiGalleryCards" class="ai-gallery-load-more">
                      <span>已显示 {{ visibleAiGalleryCards.length }} / {{ aiGalleryCards.length }}</span>
                      <button type="button" @click="loadMoreAiGalleryCards">加载更多</button>
                    </div>
                  </div>
                </template>
                <div v-else class="ai-empty-state">
                  <el-icon><Monitor /></el-icon>
                  <span>输入提示词开始生成图片</span>
                </div>
              </div>

              <div v-if="!mobileDrawerOpen" ref="aiFloatingComposerRef" class="ai-floating-composer">
                <div v-if="aiReferenceImages.length" class="ai-reference-strip">
                  <div v-for="(image, index) in aiReferenceImages" :key="image.id" class="ai-reference-thumb">
                    <img v-if="aiReferencePreviewUrl(image)" :src="aiReferencePreviewUrl(image)" :alt="`参考图 ${index + 1}`" />
                    <strong v-else>参考图</strong>
                    <span>{{ index + 1 }}</span>
                    <em v-if="image.uploading">上传中</em>
                    <em v-else-if="image.error" class="is-error">重试中</em>
                    <button type="button" aria-label="移除参考图" @click="confirmRemoveAiReferenceImage(index)">×</button>
                  </div>
                  <button type="button" class="ai-reference-clear" @click="confirmClearAiReferenceImages">清空</button>
                </div>
                <PromptEditable
                  ref="aiPromptInputRef"
                  v-model="onlineImageForm.prompt"
                  class="ai-floating-prompt"
                  placeholder="描述你想生成的图片，可输入 @ 来指定参考图..."
                  :min-height="48"
                  :max-height="220"
                  @paste-images="handleAiPromptPasteImages"
                  @submit="submitAiImage"
                />
                <div class="ai-floating-controls">
                  <label>
                    <span>尺寸</span>
                    <button type="button" class="ai-size-picker-button" @click="openAiSizePicker">{{ displayAiImageSize }}</button>
                  </label>
                  <label>
                    <span>质量</span>
                    <el-select v-model="onlineImageForm.quality">
                      <el-option label="auto" value="auto" />
                      <el-option label="low" value="low" />
                      <el-option label="medium" value="medium" />
                      <el-option label="high" value="high" />
                    </el-select>
                  </label>
                  <label>
                    <span>格式</span>
                    <el-select v-model="onlineImageForm.outputFormat">
                      <el-option label="PNG" value="png" />
                      <el-option label="JPEG" value="jpeg" />
                      <el-option label="WebP" value="webp" />
                    </el-select>
                  </label>
                  <label>
                    <span>{{ onlineImageForm.outputFormat === 'png' ? '透明背景' : '压缩率' }}</span>
                    <el-select v-if="onlineImageForm.outputFormat === 'png'" v-model="onlineImageForm.transparentOutput">
                      <el-option label="false" :value="false" />
                      <el-option label="true" :value="true" />
                    </el-select>
                    <input
                      v-else
                      class="ai-param-input"
                      :value="aiOutputCompressionInput"
                      type="number"
                      min="0"
                      max="100"
                      placeholder="0-100"
                      @input="handleAiCompressionInput"
                      @blur="commitAiCompressionInput"
                    />
                  </label>
                  <label>
                    <span>审核</span>
                    <el-select v-model="onlineImageForm.moderation">
                      <el-option label="auto" value="auto" />
                      <el-option label="low" value="low" />
                    </el-select>
                  </label>
                  <label>
                    <span>数量</span>
                    <input
                      class="ai-param-input"
                      :value="aiCountInput"
                      type="number"
                      min="1"
                      max="4"
                      @input="handleAiCountInput"
                      @blur="commitAiCountInput"
                    />
                  </label>
                  <el-upload :auto-upload="false" :show-file-list="false" :on-change="handleAiReferenceUpload" accept="image/*" multiple class="ai-attach-upload">
                    <el-button class="ai-round-action ai-attach-action" aria-label="添加参考图">
                      <svg class="ai-paperclip-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13" />
                      </svg>
                    </el-button>
                  </el-upload>
                  <el-button class="ai-round-action ai-send-action" :type="aiAgentRunning ? 'danger' : 'primary'" :loading="onlineSubmitting && !aiAgentRunning" aria-label="生成图像" @click="aiAgentRunning ? stopAiAgent() : submitAiImage()">{{ aiAgentRunning ? '■' : '-&gt;' }}</el-button>
                </div>
              </div>
            </section>
            <teleport to="body">
              <div v-if="aiDetailTask && aiTaskDisplayImageUrl(aiDetailTask)" class="ai-detail-overlay" @click="closeAiDetailModal">
                <section class="ai-detail-modal" @click.stop>
                  <div class="ai-detail-preview">
                    <button type="button" class="ai-detail-download" aria-label="下载图片" @click.stop="downloadAiTask(aiDetailTask)">
                      <el-icon><Download /></el-icon>
                    </button>
                    <div class="ai-detail-badges">
                      <span>{{ aiTaskDisplayRatioLabel(aiDetailTask) }}</span>
                      <span>{{ aiTaskDisplayResolutionLabel(aiDetailTask) }}</span>
                    </div>
                    <img
                      :src="aiTaskDisplayImageUrl(aiDetailTask)"
                      alt="生成图片详情预览"
                      @load="handleAiDetailImageLoad($event, aiDetailTask)"
                      @click.stop="openAiDetailImageLightbox(aiDetailTask)"
                      @contextmenu.prevent.stop="openAiImageContextMenu($event, aiDetailTask)"
                    />
                  </div>
                  <aside class="ai-detail-info">
                    <button type="button" class="ai-detail-close" aria-label="关闭详情" @click="closeAiDetailModal">×</button>
                    <section class="ai-detail-section">
                      <div class="ai-detail-title-row">
                        <h3>输入内容</h3>
                        <button type="button" aria-label="复制提示词" @click="copyToClipboard(String(aiDetailTask.prompt || aiDetailTask.name || ''))">
                          <el-icon><CopyDocument /></el-icon>
                        </button>
                      </div>
                      <p class="ai-detail-prompt">{{ aiDetailTask.prompt || aiDetailTask.name || 'AI 生图任务' }}</p>
                    </section>
                    <section class="ai-detail-section">
                      <h3>参数配置</h3>
                      <div class="ai-detail-source">
                        <span>来源</span>
                        <strong>{{ aiTaskParamValue(aiDetailTask, 'provider', 'OpenAI') }} · {{ aiTaskModelLabel(aiDetailTask) }} · {{ aiTaskModelLabel(aiDetailTask) }}</strong>
                      </div>
                      <div class="ai-detail-param-grid">
                        <div><span>尺寸</span><strong>{{ aiTaskDetailSizeLabel(aiDetailTask) }}</strong></div>
                        <div><span>质量</span><strong>{{ aiTaskParamValue(aiDetailTask, 'quality') }}</strong></div>
                        <div><span>格式</span><strong>{{ aiTaskParamValue(aiDetailTask, 'output_format', aiTaskParamValue(aiDetailTask, 'outputFormat', 'png')) }}</strong></div>
                        <div><span>透明背景</span><strong>{{ aiTaskParamValue(aiDetailTask, 'transparent_output', aiTaskParamValue(aiDetailTask, 'transparentOutput', 'false')) }}</strong></div>
                        <div><span>审核</span><strong>{{ aiTaskParamValue(aiDetailTask, 'moderation') }}</strong></div>
                        <div><span>数量</span><strong>{{ aiTaskParamValue(aiDetailTask, 'n', aiTaskParamValue(aiDetailTask, 'count', '1')) }}</strong></div>
                      </div>
                      <p class="ai-detail-time">创建于 {{ formatAiTaskTime(aiDetailTask) }} · 耗时 {{ aiTaskDuration(aiDetailTask) }}</p>
                    </section>
                    <section class="ai-detail-section">
                      <h3>调试信息</h3>
                      <div class="ai-detail-debug-actions">
                        <button type="button" :disabled="!aiTaskRawImageUrls(aiDetailTask).length" @click="openAiRawUrlsModal(aiDetailTask)">
                          <el-icon><Link /></el-icon><span>原始链接</span>
                        </button>
                        <button type="button" @click="openAiRawResponseModal(aiDetailTask)">
                          <el-icon><Document /></el-icon><span>原始响应</span>
                        </button>
                      </div>
                    </section>
                    <footer class="ai-detail-actions">
                      <button type="button" class="reuse" @click="reuseAiTask(aiDetailTask); closeAiDetailModal()"><el-icon><Refresh /></el-icon>复用配置</button>
                      <button type="button" class="edit" @click="editAiTaskOutput(aiDetailTask); closeAiDetailModal()"><el-icon><EditPen /></el-icon>编辑输出</button>
                      <button type="button" class="delete" @click="deleteAiTask(aiDetailTask)"><el-icon><Delete /></el-icon>删除任务</button>
                      <button type="button" :class="['favorite', { active: isAiTaskFavorite(aiDetailTask) }]" :aria-label="isAiTaskFavorite(aiDetailTask) ? '编辑收藏夹' : '收藏任务'" :title="isAiTaskFavorite(aiDetailTask) ? '编辑收藏夹' : '收藏任务'" @click="openAiFavoritePicker([aiTaskId(aiDetailTask)])">
                        <el-icon><component :is="isAiTaskFavorite(aiDetailTask) ? StarFilled : Star" /></el-icon>
                      </button>
                    </footer>
                  </aside>
                  <div
                    v-if="aiImageContextMenu.visible"
                    class="ai-image-context-menu"
                    :style="{ left: `${aiImageContextMenu.x}px`, top: `${aiImageContextMenu.y}px` }"
                    @click.stop
                    @contextmenu.prevent
                  >
                    <button type="button" @click="copyAiContextImage"><el-icon><CopyDocument /></el-icon><span>复制</span></button>
                    <button type="button" @click="downloadAiContextImage"><el-icon><Download /></el-icon><span>下载</span></button>
                    <button v-if="aiContextMenuTask && aiTaskOutputItems(aiContextMenuTask).length > 1" type="button" @click="downloadAllAiContextImages"><el-icon><Download /></el-icon><span>下载全部</span></button>
                    <button type="button" @click="editAiContextImage"><el-icon><EditPen /></el-icon><span>编辑</span></button>
                  </div>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiLightboxTask && aiTaskDisplayImageUrl(aiLightboxTask)" class="ai-lightbox" @click="closeAiLightbox" @wheel.prevent="handleAiLightboxWheel">
                <div class="ai-lightbox-backdrop"></div>
                <div class="ai-lightbox-toolbar" @click.stop>
                  <span class="ai-lightbox-zoom">{{ aiLightboxZoomText }}</span>
                  <button type="button" aria-label="缩小图片" @click="zoomAiLightboxBy(1 / 1.25)"><span>−</span></button>
                  <button type="button" aria-label="放大图片" @click="zoomAiLightboxBy(1.25)"><span>+</span></button>
                  <button type="button" aria-label="适配窗口" @click="resetAiLightboxTransform"><span>适配</span></button>
                  <button type="button" aria-label="下载图片" @click="downloadAiTask(aiLightboxTask)"><el-icon><Download /></el-icon></button>
                  <button type="button" aria-label="关闭预览" @click="closeAiLightbox"><span>×</span></button>
                </div>
                <button v-if="aiLightboxTotal > 1" type="button" class="ai-lightbox-nav prev" aria-label="上一张" @click.stop="moveAiLightbox(-1)">‹</button>
                <div
                  ref="aiLightboxViewportRef"
                  class="ai-lightbox-canvas"
                  :class="{ 'is-zoomed': aiLightboxScale > 1.01, 'is-dragging': aiLightboxDragging }"
                  @click.stop
                  @dblclick.stop="handleAiLightboxDoubleClick"
                  @pointerdown="handleAiLightboxPointerDown"
                  @pointermove="handleAiLightboxPointerMove"
                  @pointerup="handleAiLightboxPointerUp"
                  @pointercancel="handleAiLightboxPointerUp"
                >
                  <figure class="ai-lightbox-figure" :style="aiLightboxTransformStyle">
                    <div class="ai-lightbox-badges">
                      <span>{{ aiTaskDisplayRatioLabel(aiLightboxTask) }}</span>
                      <span>{{ aiTaskDisplayResolutionLabel(aiLightboxTask) }}</span>
                    </div>
                    <img
                      :src="aiTaskDisplayImageUrl(aiLightboxTask)"
                      alt="生成图片预览"
                      draggable="false"
                      @load="handleAiDetailImageLoad($event, aiLightboxTask)"
                      @contextmenu.prevent.stop="openAiImageContextMenu($event, aiLightboxTask)"
                    />
                  </figure>
                </div>
                <button v-if="aiLightboxTotal > 1" type="button" class="ai-lightbox-nav next" aria-label="下一张" @click.stop="moveAiLightbox(1)">›</button>
                <div class="ai-lightbox-footer" @click.stop>
                  <span v-if="aiLightboxTotal > 1">{{ aiLightboxIndex + 1 }} / {{ aiLightboxTotal }}</span>
                  <span>滚轮缩放 · 拖拽移动 · Esc 关闭</span>
                </div>
                <div
                  v-if="aiImageContextMenu.visible"
                  class="ai-image-context-menu"
                  :style="{ left: `${aiImageContextMenu.x}px`, top: `${aiImageContextMenu.y}px` }"
                  @click.stop
                  @contextmenu.prevent
                >
                  <button type="button" @click="copyAiContextImage"><el-icon><CopyDocument /></el-icon><span>复制</span></button>
                  <button type="button" @click="downloadAiContextImage"><el-icon><Download /></el-icon><span>下载</span></button>
                  <button v-if="aiContextMenuTask && aiTaskOutputItems(aiContextMenuTask).length > 1" type="button" @click="downloadAllAiContextImages"><el-icon><Download /></el-icon><span>下载全部</span></button>
                  <button type="button" @click="editAiContextImage"><el-icon><EditPen /></el-icon><span>编辑</span></button>
                </div>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiRawUrlsTask" class="ai-raw-modal-overlay" @click="closeAiRawModals">
                <section class="ai-raw-modal" @click.stop>
                  <header>
                    <h3>原始图片链接</h3>
                    <button type="button" aria-label="关闭" @click="closeAiRawModals">×</button>
                  </header>
                  <div class="ai-raw-list ai-raw-modal-body">
                    <article v-for="(url, index) in aiTaskRawImageUrls(aiRawUrlsTask)" :key="`${index}-${url}`" class="ai-raw-list-row">
                      <span>图片 {{ index + 1 }}</span>
                      <code>{{ url }}</code>
                      <button type="button" @click="copyToClipboard(url)">复制</button>
                    </article>
                  </div>
                  <footer>
                    <button type="button" @click="copyToClipboard(aiTaskRawImageUrls(aiRawUrlsTask).join('\n'))">全部复制</button>
                  </footer>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiRawResponseTask" class="ai-raw-modal-overlay" @click="closeAiRawModals">
                <section class="ai-raw-modal is-wide" @click.stop>
                  <header>
                    <h3>原始响应数据</h3>
                    <button type="button" aria-label="关闭" @click="closeAiRawModals">×</button>
                  </header>
                  <pre>{{ aiTaskRawResponseText(aiRawResponseTask) }}</pre>
                  <footer>
                    <button type="button" @click="copyToClipboard(aiTaskRawResponseText(aiRawResponseTask))">全部复制</button>
                  </footer>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiFavoritePickerVisible" class="ai-favorite-picker-overlay" @click="closeAiFavoritePicker">
                <section class="ai-favorite-picker-modal" @click.stop>
                  <header class="ai-favorite-picker-head">
                    <button type="button" aria-label="关闭" class="ai-favorite-picker-close" @click="closeAiFavoritePicker">×</button>
                    <h3><el-icon><StarFilled /></el-icon><span>保存到收藏夹</span></h3>
                    <p>取消勾选会将任务从对应的收藏夹中移除。</p>
                  </header>
                  <div class="ai-favorite-picker-toolbar">
                    <span>选择要保存的收藏夹</span>
                    <div>
                      <button type="button" @click="selectAllAiFavoritePickerCollections">全选</button>
                      <button type="button" @click="clearAiFavoritePickerCollections">取消</button>
                    </div>
                  </div>
                  <div class="ai-favorite-picker-list">
                    <article
                      v-for="collection in aiFavoriteCollections"
                      :key="collection.id"
                      :class="['ai-favorite-picker-row', {
                        dragging: aiFavoritePickerDraggedId === collection.id,
                        'drop-before': aiFavoritePickerDragOverId === collection.id && aiFavoritePickerDropPosition === 'before' && aiFavoritePickerDraggedId !== collection.id,
                        'drop-after': aiFavoritePickerDragOverId === collection.id && aiFavoritePickerDropPosition === 'after' && aiFavoritePickerDraggedId !== collection.id
                      }]"
                      :draggable="aiEditingCollectionId !== collection.id"
                      @dragstart="startAiFavoritePickerDrag($event, collection.id)"
                      @dragover.prevent="updateAiFavoritePickerDragOver($event, collection.id)"
                      @drop.prevent.stop="dropAiFavoritePickerCollection(collection.id)"
                      @dragend="resetAiFavoritePickerDrag"
                      @click="toggleAiFavoritePickerCollection(collection.id)"
                    >
                      <button type="button" class="ai-favorite-picker-drag" aria-label="拖动排序" @click.stop><span></span><span></span></button>
                      <label class="ai-favorite-picker-check" @click.stop>
                        <input type="checkbox" :checked="aiFavoritePickerCheckedIds.includes(collection.id)" @change="toggleAiFavoritePickerCollection(collection.id)" />
                        <i></i>
                      </label>
                      <input
                        v-if="aiEditingCollectionId === collection.id"
                        v-model="aiEditingCollectionName"
                        class="ai-favorite-picker-rename"
                        @click.stop
                        @keyup.enter="confirmRenameAiFavoriteCollection"
                        @keyup.esc="cancelRenameAiFavoriteCollection"
                        @blur="confirmRenameAiFavoriteCollection"
                      />
                      <strong v-else>{{ collection.name }}</strong>
                      <div class="ai-favorite-picker-actions" @click.stop>
                        <button type="button" :class="{ active: aiDefaultFavoriteCollectionId === collection.id }" :title="aiDefaultFavoriteCollectionId === collection.id ? '取消默认收藏夹' : '设为默认收藏夹'" @click="setAiDefaultFavoriteCollection(collection.id)">
                          <el-icon><component :is="aiDefaultFavoriteCollectionId === collection.id ? StarFilled : Star" /></el-icon>
                        </button>
                        <button type="button" title="重命名" @click="startRenameAiFavoriteCollection(collection)"><el-icon><EditPen /></el-icon></button>
                        <button type="button" title="删除" :disabled="aiFavoriteCollections.length <= 1" @click="confirmDeleteAiFavoriteCollection(collection.id)"><el-icon><Delete /></el-icon></button>
                      </div>
                    </article>
                  </div>
                  <footer class="ai-favorite-picker-foot">
                    <div class="ai-favorite-picker-create">
                      <input v-model="aiNewCollectionName" placeholder="新建收藏夹..." @keyup.enter="createAiFavoriteCollectionFromPicker" />
                      <button type="button" :disabled="!aiNewCollectionName.trim()" @click="createAiFavoriteCollectionFromPicker">新建</button>
                    </div>
                    <div class="ai-favorite-picker-submit">
                      <button type="button" @click="closeAiFavoritePicker">取消</button>
                      <button type="button" class="primary" @click="confirmAiFavoritePicker">确认</button>
                    </div>
                  </footer>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiFavoriteCollectionsVisible" class="ai-collection-overlay" @click="aiFavoriteCollectionsVisible = false">
                <section class="ai-collection-modal" @click.stop>
                  <header>
                    <div><h3>收藏夹管理</h3><p>管理任务收藏、默认收藏夹和当前筛选范围。</p></div>
                    <button type="button" @click="aiFavoriteCollectionsVisible = false">×</button>
                  </header>
                  <div class="ai-collection-create">
                    <input v-model="aiNewCollectionName" placeholder="新收藏夹名称" @keyup.enter="createAiFavoriteCollection" />
                    <button type="button" @click="createAiFavoriteCollection">新建</button>
                  </div>
                  <div class="ai-collection-list">
                    <article v-for="collection in aiFavoriteCollections" :key="collection.id" :class="{ active: aiActiveFavoriteCollectionId === collection.id }">
                      <button type="button" class="ai-collection-main" @click="selectAiFavoriteCollection(collection.id)">
                        <strong v-if="aiEditingCollectionId !== collection.id">{{ collection.name }}</strong>
                        <input
                          v-else
                          v-model="aiEditingCollectionName"
                          class="ai-collection-rename-input"
                          @click.stop
                          @keyup.enter="confirmRenameAiFavoriteCollection"
                          @keyup.esc="cancelRenameAiFavoriteCollection"
                          @blur="confirmRenameAiFavoriteCollection"
                        />
                        <span>{{ collection.taskIds.length }} 项任务{{ aiDefaultFavoriteCollectionId === collection.id ? ' · 默认' : '' }}</span>
                      </button>
                      <button type="button" @click="setAiDefaultFavoriteCollection(collection.id)">{{ aiDefaultFavoriteCollectionId === collection.id ? '取消默认' : '设默认' }}</button>
                      <button type="button" @click="startRenameAiFavoriteCollection(collection)">重命名</button>
                      <button type="button" @click="addSelectedToCollection(collection)">加入选中</button>
                      <button type="button" :disabled="aiFavoriteCollections.length <= 1" @click="confirmDeleteAiFavoriteCollection(collection.id)">删除</button>
                    </article>
                  </div>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiSizePickerVisible" class="ai-size-picker-overlay" @mousedown="handleAiSizePickerBackdropDown" @mouseup="handleAiSizePickerBackdropUp">
                <section ref="aiSizePickerRef" class="ai-size-picker-modal">
                  <header class="ai-size-picker-head">
                    <div>
                      <h3>设置图像尺寸</h3>
                      <p>当前：{{ displayAiImageSize || 'auto' }}</p>
                    </div>
                    <button type="button" aria-label="关闭尺寸设置" @click="closeAiSizePicker">×</button>
                  </header>
                  <div class="ai-size-picker-segment">
                    <button v-for="mode in aiSizePickerModes" :key="mode.value" type="button" :class="{ active: aiSizePickerMode === mode.value }" @click="aiSizePickerMode = mode.value">
                      {{ mode.label }}
                    </button>
                  </div>
                  <div class="ai-size-picker-scroll">
                    <section v-if="aiImageSizeSchemaOptions.length || aiImageModuleSchemaLoading" class="ai-size-picker-section">
                      <p>Schema 尺寸<span v-if="aiImageModuleSchemaLoading">读取中...</span></p>
                      <div v-if="aiImageSizeSchemaOptions.length" class="ai-size-picker-schema-grid">
                        <button
                          v-for="option in aiImageSizeSchemaOptions"
                          :key="option"
                          type="button"
                          :class="{ active: isAiSchemaSizeOptionActive(option) }"
                          @click="selectAiSchemaSizeOption(option)"
                        >
                          <strong>{{ option }}</strong>
                          <small v-if="option === aiImageSizeSchemaDefault">默认</small>
                        </button>
                      </div>
                    </section>
                    <div v-if="aiSizePickerMode === 'auto'" class="ai-size-picker-auto">
                      <div class="ai-size-picker-auto-icon">⚡</div>
                      <h4>自动尺寸</h4>
                      <p>不向模型传递具体的分辨率参数<br />由模型自己决定生成尺寸</p>
                    </div>
                    <template v-else-if="aiSizePickerMode === 'ratio'">
                      <section class="ai-size-picker-section">
                        <p>基准分辨率</p>
                        <div class="ai-size-picker-tier-grid">
                          <button v-for="tier in aiSizeTiers" :key="tier" type="button" :class="{ active: aiSizeTier === tier }" @click="aiSizeTier = tier">{{ tier }}</button>
                        </div>
                      </section>
                      <section class="ai-size-picker-section">
                        <p>图像比例</p>
                        <div class="ai-size-picker-ratio-grid">
                          <button v-for="item in aiSizeRatios" :key="item.value" type="button" :class="{ active: aiSizeRatio === item.value }" @click="aiSizeRatio = item.value">
                            <span class="ai-size-ratio-icon"><i :style="ratioIconStyle(item.value)"></i></span>
                            <span>{{ item.label }}</span>
                          </button>
                          <button type="button" class="ai-size-picker-custom-ratio" :class="{ active: aiSizeRatio === 'custom' }" @click="aiSizeRatio = 'custom'">自定义比例</button>
                        </div>
                      </section>
                      <label v-if="aiSizeRatio === 'custom'" class="ai-size-picker-input">
                        <span>输入自定义比例</span>
                        <input v-model="aiCustomRatio" placeholder="例如 5:4 / 2.39:1" />
                      </label>
                    </template>
                    <template v-else>
                      <section class="ai-size-picker-section">
                        <p>输入具体像素值</p>
                        <div class="ai-size-picker-resolution">
                          <label><span>宽度 (Width)</span><input v-model="aiCustomWidth" type="number" placeholder="例如 1024" /></label>
                          <b>×</b>
                          <label><span>高度 (Height)</span><input v-model="aiCustomHeight" type="number" placeholder="例如 1024" /></label>
                        </div>
                      </section>
                      <div class="ai-size-picker-limit">
                        由于模型限制，最终输出会自动规整到合法尺寸：宽高均为 16 的倍数，最大边长 3840px，宽高比不超过 3:1，总像素限制为 655360-8294400。
                      </div>
                    </template>
                  </div>
                  <div class="ai-size-picker-preview">
                    <span>将使用</span>
                    <strong>{{ aiSizePickerPreview || '尺寸无效' }}</strong>
                    <small v-if="aiSizePickerClamped">已按模型限制自动规整</small>
                  </div>
                  <footer class="ai-size-picker-actions">
                    <button type="button" @click="closeAiSizePicker">取消</button>
                    <button type="button" :disabled="!aiSizePickerPreview" @click="applyAiSizePicker">确定</button>
                  </footer>
                </section>
              </div>
            </teleport>
            <teleport to="body">
              <div v-if="aiSettingsVisible" class="ai-settings-overlay" @click="closeAiSettings">
                <section class="ai-settings-modal" @click.stop>
                  <header class="ai-settings-head">
                    <h3><el-icon><Setting /></el-icon><span>设置</span></h3>
                    <div class="ai-settings-head-actions">
                      <span>v0.6.10</span>
                      <button type="button" aria-label="关闭设置" @click="closeAiSettings">×</button>
                    </div>
                  </header>
                  <div class="ai-settings-body">
                    <aside class="ai-settings-sidebar">
                      <button v-for="tab in aiSettingsTabs" :key="tab.id" type="button" :class="{ active: aiSettingsTab === tab.id }" @click="aiSettingsTab = tab.id">
                        <el-icon><component :is="tab.icon" /></el-icon>
                        <span>{{ tab.label }}</span>
                      </button>
                    </aside>
                    <main class="ai-settings-content">
                      <section v-if="aiSettingsTab === 'api'" class="ai-settings-form">
                        <label class="ai-settings-field">
                          <span>当前配置</span>
                          <el-select v-model="aiSettingsDraft.currentProfile">
                            <el-option label="默认 · OpenAI" value="default" />
                            <el-option label="备用 · OpenAI 兼容接口" value="compatible" />
                          </el-select>
                        </label>
                        <label class="ai-settings-field">
                          <span>配置名称</span>
                          <el-input v-model="aiSettingsDraft.profileName" placeholder="默认" />
                        </label>
                        <label class="ai-settings-field">
                          <span>服务商类型</span>
                          <el-select v-model="aiSettingsDraft.providerType">
                            <el-option label="OpenAI 兼容接口" value="openai" />
                            <el-option label="Gemini" value="gemini" />
                            <el-option label="自定义上游" value="custom" />
                          </el-select>
                        </label>
                        <label class="ai-settings-field">
                          <span>API URL</span>
                          <el-input v-model="aiSettingsDraft.apiUrl" placeholder="https://api.openai.com/v1" />
                        </label>
                        <p class="ai-settings-note is-warning">已开启代理，实际请求目标由后端服务端配置决定，此处 URL 可作为前端配置备注。</p>
                        <div class="ai-settings-switch-row">
                          <div><strong>API 代理</strong><small>开启后请求经服务器转发到上游 API，可绕过浏览器跨域限制。</small></div>
                          <el-switch v-model="aiSettingsDraft.apiProxy" />
                        </div>
                        <label class="ai-settings-field">
                          <span>API Key</span>
                          <el-input v-model="aiSettingsDraft.apiKey" show-password placeholder="sk-..." />
                        </label>
                        <label class="ai-settings-field">
                          <span>模型</span>
                          <el-select v-model="aiSettingsDraft.model" filterable allow-create>
                            <el-option v-for="model in onlineModelOptions" :key="model.value" :label="model.label" :value="model.value" />
                            <el-option label="gpt-image-2" value="gpt-image-2" />
                          </el-select>
                        </label>
                        <label class="ai-settings-field">
                          <span>API 模式</span>
                          <el-segmented v-model="aiSettingsDraft.apiMode" :options="aiSettingsApiModes" />
                        </label>
                        <div class="ai-settings-grid-two">
                          <label class="ai-settings-field">
                            <span>超时（秒）</span>
                            <el-input-number v-model="aiSettingsDraft.timeout" :min="30" :max="900" controls-position="right" />
                          </label>
                          <label class="ai-settings-field">
                            <span>流式图片</span>
                            <el-select v-model="aiSettingsDraft.streamImages">
                              <el-option label="开启" :value="true" />
                              <el-option label="关闭" :value="false" />
                            </el-select>
                          </label>
                        </div>
                        <footer class="ai-settings-footer">
                          <el-button @click="testAiSettingsConnection">测试连接</el-button>
                          <el-button type="primary" @click="saveAiSettings">保存设置</el-button>
                        </footer>
                      </section>
                      <section v-else-if="aiSettingsTab === 'general'" class="ai-settings-form">
                        <label class="ai-settings-field">
                          <span>任务提交方式</span>
                          <el-select v-model="aiSettingsDraft.submitMode">
                            <el-option label="Ctrl + Enter 提交" value="ctrl-enter" />
                            <el-option label="Enter 直接提交" value="enter" />
                            <el-option label="仅点击按钮提交" value="button" />
                          </el-select>
                        </label>
                        <div v-for="item in aiHabitSwitches" :key="item.key" class="ai-settings-switch-row">
                          <div><strong>{{ item.title }}</strong><small>{{ item.desc }}</small></div>
                          <el-switch v-model="aiSettingsDraft[item.key]" />
                        </div>
                        <label class="ai-settings-field">
                          <span>参考图编辑按钮</span>
                          <el-select v-model="aiSettingsDraft.referenceEditAction">
                            <el-option label="每次询问" value="ask" />
                            <el-option label="直接打开编辑" value="edit" />
                            <el-option label="隐藏按钮" value="hide" />
                          </el-select>
                        </label>
                        <label class="ai-settings-field">
                          <span>ZIP 批量下载途径</span>
                          <el-input v-model="aiSettingsDraft.zipDownloadRoutes" />
                        </label>
                        <footer class="ai-settings-footer">
                          <el-button type="primary" @click="saveAiSettings">保存习惯</el-button>
                        </footer>
                      </section>
                      <section v-else-if="aiSettingsTab === 'agent'" class="ai-settings-form">
                        <label class="ai-settings-field">
                          <span>独立 API 配置</span>
                          <el-segmented v-model="aiSettingsDraft.agentApiMode" :options="aiAgentApiModes" />
                        </label>
                        <label class="ai-settings-field">
                          <span>文本模型配置</span>
                          <el-select v-model="aiSettingsDraft.agentTextProfile">
                            <el-option label="默认配置" value="default" />
                            <el-option label="Agent 专用配置" value="agent" />
                          </el-select>
                        </label>
                        <label class="ai-settings-field">
                          <span>图像模型配置</span>
                          <el-select v-model="aiSettingsDraft.agentImageProfile">
                            <el-option label="跟随 API 配置" value="default" />
                            <el-option label="图像专用配置" value="image" />
                          </el-select>
                        </label>
                        <div class="ai-settings-grid-two">
                          <label class="ai-settings-field">
                            <span>最大工具轮数</span>
                            <el-input-number v-model="aiSettingsDraft.agentMaxToolRounds" :min="1" :max="50" controls-position="right" />
                          </label>
                          <div class="ai-settings-switch-row compact">
                            <div><strong>网络搜索</strong><small>允许 Agent 调用联网搜索能力。</small></div>
                            <el-switch v-model="aiSettingsDraft.agentWebSearch" />
                          </div>
                        </div>
                        <footer class="ai-settings-footer">
                          <el-button type="primary" @click="saveAiSettings">保存 Agent 配置</el-button>
                        </footer>
                      </section>
                      <section v-else-if="aiSettingsTab === 'data'" class="ai-settings-form">
                        <div class="ai-settings-data-card">
                          <strong>配置数据</strong>
                          <span>导出、导入或重置 AI 生图偏好配置。</span>
                          <div>
                            <el-button @click="exportAiSettings">导出配置</el-button>
                            <el-button @click="importAiSettings">导入配置</el-button>
                            <el-button type="danger" plain @click="resetAiSettings">重置配置</el-button>
                          </div>
                        </div>
                        <div class="ai-settings-data-card">
                          <strong>任务数据</strong>
                          <span>任务记录仍由后端生成任务表维护，这里只管理前端展示偏好。</span>
                          <el-tag type="success">已接入当前后台框架</el-tag>
                        </div>
                      </section>
                      <section v-else class="ai-settings-form">
                        <div class="ai-settings-about">
                          <strong>GPT Image Playground</strong>
                          <span>当前模块已按参考项目设置弹窗结构适配到用户后台 AI 生图工作区。</span>
                          <small>版本 v0.6.10 · Vue3 + Element Plus</small>
                        </div>
                        <footer class="ai-settings-footer">
                          <el-button @click="runAiPlaygroundAction('help')">查看操作指南</el-button>
                        </footer>
                      </section>
                    </main>
                  </div>
                </section>
              </div>
            </teleport>
          </section>
          <section v-if="store.activeModuleId === 'partnerDashboard'" class="partner-dashboard-page">
            <div class="partner-stat-grid">
              <article v-for="metric in partnerDashboardMetrics" :key="metric.label" class="partner-stat-card">
                <span>{{ metric.label }}</span>
                <strong>{{ metric.value }}</strong>
                <small>{{ metric.hint }}</small>
              </article>
            </div>
            <section class="partner-invite-card">
              <div class="partner-invite-copy">
                <el-tag type="success">推广链接</el-tag>
                <h3>专属开户链接</h3>
                <p>{{ partnerInviteLink() || '当前代理商还没有生成邀请码' }}</p>
              </div>
              <div class="partner-invite-code">
                <span>邀请码</span>
                <strong>{{ partnerInviteCode() || '-' }}</strong>
              </div>
              <div class="partner-invite-code">
                <span>代理商等级</span>
                <strong>{{ partnerAgentLevelLabel() }}</strong>
              </div>
              <div class="partner-invite-actions">
                <el-button type="primary" :icon="CopyDocument" @click="copyToClipboard(partnerInviteLink())">复制链接</el-button>
                <el-button :icon="CopyDocument" @click="copyToClipboard(partnerInviteCode())">复制邀请码</el-button>
                <el-button :icon="Link" @click="openPartnerInviteLink">打开链接</el-button>
              </div>
            </section>
            <section class="partner-dashboard-grid">
              <el-card shadow="never" class="partner-chart-card">
                <template #header><div class="panel-head"><span>推广转化趋势</span><el-tag type="success">实时</el-tag></div></template>
                <div class="partner-chart-bars">
                  <div v-for="item in partnerTrend" :key="item.day" class="partner-chart-bar">
                    <i :style="{ height: item.height + '%' }"></i>
                    <span>{{ item.day }}</span>
                  </div>
                </div>
              </el-card>
              <el-card shadow="never" class="partner-todo-card">
                <template #header><div class="panel-head"><span>待办动作</span><el-tag>代理</el-tag></div></template>
                <button v-for="todo in partnerTodos" :key="todo.title" type="button" @click="selectAdminModule(todo.module)">
                  <strong>{{ todo.title }}</strong>
                  <small>{{ todo.desc }}</small>
                </button>
              </el-card>
            </section>
            <el-card shadow="never" class="data-panel partner-source-card">
              <template #header><div class="panel-head"><span>客户来源渠道</span><small>{{ partnerSourceRows.length }} 条渠道记录</small></div></template>
              <el-table :data="partnerSourceRows" stripe>
                <el-table-column prop="channel" label="渠道" min-width="160" />
                <el-table-column prop="visits" label="访问量" width="120" />
                <el-table-column prop="customers" label="注册客户" width="120" />
                <el-table-column prop="orders" label="成交订单" width="120" />
                <el-table-column prop="commission" label="佣金贡献" min-width="140" />
                <el-table-column prop="status" label="状态" width="110"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
              </el-table>
            </el-card>
          </section>
          <section v-else-if="store.activeModuleId === 'operationCenterDashboard'" class="operation-center-page">
            <div class="operation-center-stat-grid">
              <article v-for="metric in operationCenterDashboardMetrics" :key="metric.label" class="operation-center-stat-card">
                <span>{{ metric.label }}</span>
                <strong>{{ metric.value }}</strong>
                <small>{{ metric.hint }}</small>
              </article>
            </div>
            <section class="operation-center-hero">
              <div class="operation-center-hero-copy">
                <el-tag type="success">运营中心</el-tag>
                <h3>{{ operationCenterRecord.name || '运营中心' }}</h3>
                <p>{{ operationCenterRecord.region || '未设置区域' }} · {{ statusLabel(operationCenterRecord.status) }}</p>
              </div>
              <div class="operation-center-code-card">
                <span>运营邀请码</span>
                <strong>{{ operationCenterInviteCode() || '-' }}</strong>
              </div>
              <div class="operation-center-code-card">
                <span>中心 ID</span>
                <strong>{{ operationCenterRecord.id || '-' }}</strong>
              </div>
              <div class="operation-center-actions">
                <el-button type="primary" :icon="CopyDocument" @click="copyToClipboard(operationCenterInviteCode())">复制邀请码</el-button>
                <el-button :icon="Refresh" :loading="store.loading" @click="() => store.loadActiveModule({ preferCache: false })">刷新</el-button>
              </div>
            </section>
            <section class="operation-center-tabs">
              <button v-for="tab in operationCenterTabs" :key="tab.module" type="button" @click="selectAdminModule(tab.module)">
                <el-icon><component :is="tab.icon" /></el-icon>
                <span>{{ tab.title }}</span>
                <strong>{{ tab.value }}</strong>
              </button>
            </section>
            <section class="operation-center-dashboard-grid">
              <el-card shadow="never" class="operation-center-chart-card">
                <template #header><div class="panel-head"><span>区域经营趋势</span><el-tag type="success">实时</el-tag></div></template>
                <div class="operation-center-chart-bars">
                  <div v-for="item in operationCenterTrend" :key="item.day" class="operation-center-chart-bar">
                    <i :style="{ height: item.height + '%' }"></i>
                    <span>{{ item.day }}</span>
                  </div>
                </div>
              </el-card>
              <el-card shadow="never" class="operation-center-todo-card">
                <template #header><div class="panel-head"><span>运营动作</span><el-tag>中心</el-tag></div></template>
                <button v-for="todo in operationCenterTodos" :key="todo.title" type="button" @click="selectAdminModule(todo.module)">
                  <strong>{{ todo.title }}</strong>
                  <small>{{ todo.desc }}</small>
                </button>
              </el-card>
            </section>
            <el-card shadow="never" class="data-panel operation-center-summary-card">
              <template #header><div class="panel-head"><span>中心结算概览</span><small>{{ operationCenterKpis.length }} 项核心指标</small></div></template>
              <div class="operation-center-kpi-grid">
                <article v-for="item in operationCenterKpis" :key="item.label">
                  <span>{{ item.label }}</span>
                  <strong>{{ item.value }}</strong>
                  <small>{{ item.desc }}</small>
                </article>
              </div>
            </el-card>
          </section>
          <StorageCenter v-else-if="store.activeModuleId === 'storageCenter'" />
          <InspirationManagement v-else-if="store.activeModuleId === 'inspirationManagement'" />
          <MediaCenter v-else-if="['mediaAssets', 'mediaCategories'].includes(store.activeModuleId)" />
          <PageDecoration v-else-if="mediaDecorationModuleIds.includes(store.activeModuleId)" :key="store.activeModuleId" :initial-page-code="decorationInitialPage" />
          <KnowledgeAdminCenter v-else-if="store.activeModuleId === 'knowledgeAdmin'" />
          <CustomerAttributionOverview v-else-if="customerAttributionModuleIds.includes(store.activeModuleId)" />
          <EnterpriseManagement
            v-else-if="enterpriseModuleIds.includes(store.activeModuleId)"
            :module-id="store.activeModuleId"
            :route-path="enterpriseRoutePath"
            :permissions="currentPermissions"
            :current-role="currentAdmin?.role || ''"
            @navigate="navigateEnterpriseRoute"
          />
          <section v-else-if="store.activeModuleId === 'analysis'" class="analysis-page">
            <div class="analysis-stat-grid">
              <article v-for="stat in analysisStats" :key="stat.label" class="analysis-stat-card">
                <el-icon><component :is="stat.icon" /></el-icon>
                <div><span>{{ stat.label }}</span><strong>{{ stat.value }}</strong></div>
              </article>
            </div>
            <section class="analysis-chart-grid">
              <el-card shadow="never" class="analysis-card">
                <template #header><div class="panel-head"><span>用户访问来源</span><small>客户、渠道与推广来源</small></div></template>
                <div class="traffic-layout">
                  <div class="traffic-legend"><div v-for="source in trafficSources" :key="source.label"><i :style="{ backgroundColor: source.color }"></i><span>{{ source.label }}</span></div></div>
                  <div class="donut-chart" :style="trafficDonutStyle"><span>来源</span></div>
                </div>
              </el-card>
              <el-card shadow="never" class="analysis-card">
                <template #header><div class="panel-head"><span>每周生成任务活跃量</span><small>任务提交与完成趋势</small></div></template>
                <div class="bar-chart"><div v-for="item in weeklyActivity" :key="item.day" class="bar-item"><span class="bar" :style="{ height: item.height + '%' }"></span><small>{{ item.day }}</small></div></div>
              </el-card>
            </section>
            <el-card shadow="never" class="analysis-card analysis-line-card">
              <template #header><div class="panel-head"><span>每月销售额 / 积分消耗趋势</span><el-tag type="success">实时</el-tag></div></template>
              <svg class="trend-chart" viewBox="0 0 960 220" role="img" aria-label="每月销售额和积分消耗趋势">
                <g class="trend-grid"><line v-for="y in [40, 80, 120, 160, 200]" :key="y" x1="32" :y1="y" x2="928" :y2="y" /></g>
                <polyline class="trend-line trend-line-primary" points="32,154 112,136 192,94 272,108 352,150 432,124 512,86 592,112 672,70 752,92 832,58 928,116" />
                <polyline class="trend-line trend-line-success" points="32,112 112,150 192,142 272,78 352,70 432,92 512,84 592,36 672,108 752,174 832,132 928,118" />
              </svg>
            </el-card>
          </section>
          <section v-else-if="store.activeModuleId === 'workbench'" class="workbench-page">
            <section class="workbench-hero">
              <div><el-tag type="success">API ONLINE</el-tag><h3>欢迎回来，{{ currentAdmin?.name || '平台管理员' }}</h3><p>今日重点关注客户余额、上游模型连通性、待支付订单和渠道启停状态。</p></div>
              <div class="workbench-health"><span>平台健康度</span><strong>98.6%</strong><small>数据同步正常</small></div>
            </section>
            <section class="workbench-grid">
              <el-card shadow="never" class="analysis-card">
                <template #header><div class="panel-head"><span>快捷入口</span><small>高频运营动作</small></div></template>
                <div class="shortcut-grid"><button v-for="item in quickTodos" :key="item.action" type="button" @click="selectAdminModule(item.module)"><span>{{ item.title }}</span><small>{{ item.desc }}</small></button></div>
              </el-card>
              <el-card shadow="never" class="analysis-card">
                <template #header><div class="panel-head"><span>待办队列</span><el-tag type="warning">运营</el-tag></div></template>
                <div class="todo-list workbench-todos"><button v-for="task in workbenchTasks" :key="task.title" type="button" @click="selectAdminModule(task.module)"><span>{{ task.title }}</span><small>{{ task.desc }}</small></button></div>
              </el-card>
            </section>
          </section>
          <section v-else-if="store.activeModuleId === 'dashboard'" class="dashboard-grid">
            <el-card shadow="never" class="dashboard-card dashboard-card-large">
              <template #header><div class="panel-head"><span>经营概览</span><el-tag type="success">实时</el-tag></div></template>
              <div class="overview-board"><div v-for="metric in metrics.slice(0, 4)" :key="metric.label" class="overview-item"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong></div></div>
            </el-card>
            <el-card shadow="never" class="dashboard-card">
              <template #header><div class="panel-head"><span>待办动作</span><el-tag type="warning">运营</el-tag></div></template>
              <div class="todo-list"><button v-for="item in quickTodos" :key="item.action" type="button" @click="selectAdminModule(item.module)"><span>{{ item.title }}</span><small>{{ item.desc }}</small></button></div>
            </el-card>
          </section>
          <section v-else-if="store.activeModuleId === 'userMembership'" class="user-membership-page">
            <div class="membership-plan-head">
              <div>
                <h2>选择身份、充值与订阅</h2>
                <p>积分有效期为 2 年</p>
              </div>
              <article class="membership-current-card">
                <div class="membership-current-copy">
                  <div>
                    <strong>当前订阅</strong>
                    <span><el-icon><Tickets /></el-icon></span>
                  </div>
                  <small>{{ userMembershipCurrentPlan }}</small>
                </div>
                <div class="membership-balance">
                  <i>◆</i>
                  <strong>{{ sidebarPlan.availableText }}</strong>
                  <small>总点数 {{ sidebarPlan.totalText }}</small>
                </div>
                <div class="membership-usage-meter">
                  <div>
                    <span>点数使用</span>
                    <strong>{{ sidebarPlan.percent }}%</strong>
                  </div>
                  <p><i :style="{ width: sidebarPlan.percent + '%' }"></i></p>
                </div>
                <div class="membership-current-actions">
                  <button type="button" @click="createUserRechargeOrder">充值</button>
                  <button type="button" @click="selectAdminModule('userOrders')">明细</button>
                </div>
              </article>
            </div>

            <div class="membership-mode-tabs">
              <button type="button" :class="{ active: selectedMembershipMode === 'identity' }" @click="selectedMembershipMode = 'identity'">身份</button>
              <button type="button" :class="{ active: selectedMembershipMode === 'recharge' }" @click="selectedMembershipMode = 'recharge'">充值</button>
              <button type="button" :class="{ active: selectedMembershipMode === 'subscribe' }" @click="selectedMembershipMode = 'subscribe'">订阅</button>
            </div>

            <section v-if="selectedMembershipMode === 'recharge'" class="membership-recharge-panel">
              <article class="membership-account-card">
                <span>充值账户</span>
                <strong>当前余额: {{ sidebarPlan.availableText }}</strong>
              </article>
              <article class="membership-recharge-card">
                <h3>快捷金额</h3>
                <div class="membership-amount-grid">
                  <button
                    v-for="amount in quickRechargeAmounts"
                    :key="amount"
                    type="button"
                    :class="{ active: selectedRechargeAmount === amount && !customRechargeAmount }"
                    @click="selectRechargeAmount(amount)"
                  >
                    {{ amount }}
                  </button>
                </div>
                <label class="membership-custom-amount">
                  <span>自定义金额</span>
                  <input v-model="customRechargeAmount" inputmode="decimal" placeholder="输入金额" />
                </label>
              </article>
              <article class="membership-payment-card">
                <h3>支付方式</h3>
                <div class="membership-payment-grid">
                  <button
                    v-for="method in paymentMethodOptions"
                    :key="method.id"
                    type="button"
                    :class="{ active: selectedPaymentMethod === method.id }"
                    @click="selectedPaymentMethod = method.id"
                  >
                    <el-icon><component :is="method.icon" /></el-icon>
                    <span>{{ method.label }}</span>
                  </button>
                </div>
              </article>
              <button class="membership-pay-submit" type="button" :disabled="!rechargeAmountYuan" @click="createUserRechargeOrder">
                确认支付 ￥{{ rechargeAmountYuan.toFixed(2) }}
              </button>
            </section>

            <template v-else-if="selectedMembershipMode === 'subscribe'">
              <div class="membership-cycle-tabs">
              <button
                v-for="cycle in membershipBillingCycles"
                :key="cycle.id"
                type="button"
                :class="{ active: selectedMembershipCycle === cycle.id }"
                @click="selectedMembershipCycle = cycle.id"
              >
                <span>{{ cycle.label }}</span>
                <em>{{ cycle.discount }}</em>
              </button>
              </div>

              <div class="membership-pricing-grid standard-membership-pricing-grid">
              <article
                v-for="plan in standardMembershipPlanCards"
                :key="plan.id"
                :class="['membership-pricing-card', { featured: plan.recommended, selected: selectedMembershipPlanId === plan.id }]"
                @click="selectMembershipPlan(plan)"
              >
                <header>
                  <strong>{{ plan.name }}</strong>
                  <span v-if="plan.recommended"><el-icon><Check /></el-icon>推荐方案</span>
                </header>
                <div class="membership-price-row">
                  <span>￥</span>
                  <strong>{{ plan.price }}</strong>
                  <del>￥{{ plan.originalPrice }}/{{ plan.unit }}</del>
                </div>
                <p>{{ plan.note }}</p>
                <div class="membership-credit-box">
                  <div><strong>{{ formatNumber(plan.credits) }}</strong><span>积分/{{ plan.creditUnit }}</span></div>
                  <small>最多生成{{ formatNumber(plan.images) }}张图片 或 {{ formatNumber(plan.videos) }}个视频 <i>?</i></small>
                </div>
                <button class="membership-subscribe-button" type="button" @click.stop="createUserSubscriptionOrder(plan)">{{ plan.custom ? "联系商务" : "立即开通" }}</button>
                <ul>
                  <li v-for="feature in plan.features" :key="feature"><span>✓</span>{{ feature }}</li>
                </ul>
              </article>
              </div>

              <div v-if="customMembershipPlanCards.length" class="membership-pricing-grid custom-membership-pricing-grid">
              <article
                v-for="plan in customMembershipPlanCards"
                :key="plan.id"
                :class="['membership-pricing-card custom-membership-pricing-card', { selected: selectedMembershipPlanId === plan.id }]"
                @click="selectMembershipPlan(plan)"
              >
                <header>
                  <strong>{{ plan.name }}</strong>
                </header>
                <div class="membership-price-row">
                  <span>￥</span>
                  <strong>{{ plan.price }}</strong>
                  <del>￥{{ plan.originalPrice }}/{{ plan.unit }}</del>
                </div>
                <p>{{ plan.note }}</p>
                <div class="membership-credit-box">
                  <div><strong>{{ formatNumber(plan.credits) }}</strong><span>积分/{{ plan.creditUnit }}</span></div>
                  <small>最多生成{{ formatNumber(plan.images) }}张图片 或 {{ formatNumber(plan.videos) }}个视频 <i>?</i></small>
                </div>
                <button class="membership-subscribe-button" type="button" @click.stop="createUserSubscriptionOrder(plan)">联系商务</button>
                <ul>
                  <li v-for="feature in plan.features" :key="feature"><span>✓</span>{{ feature }}</li>
                </ul>
              </article>
              </div>
            </template>

            <section v-if="selectedMembershipMode === 'identity'" class="membership-identity-panel">
              <article
                v-for="pack in identityPackageCards"
                :key="pack.id"
                :class="['membership-pricing-card', 'identity-package-card', 'identity-package-card--' + pack.id, { featured: pack.featured, selected: identityPackageIsActive(pack) }]"
              >
                <header>
                  <strong>{{ pack.name }}</strong>
                  <span v-if="pack.badge">{{ pack.badge }}</span>
                </header>
                <div class="membership-price-row">
                  <span>￥</span>
                  <strong>{{ pack.price }}</strong>
                  <del>{{ pack.originalText }}</del>
                </div>
                <p>{{ pack.note }}</p>
                <div class="membership-credit-box">
                  <div><strong>{{ formatNumber(pack.tokenAmount) }}</strong><span>Token/点数权益</span></div>
                  <small>{{ pack.ruleText }}</small>
                </div>
                <button class="membership-subscribe-button" type="button" @click="handleIdentityPackageAction(pack)">
                  {{ identityPackageActionText(pack) }}
                </button>
                <ul>
                  <li v-for="feature in pack.features" :key="feature"><span>✓</span>{{ feature }}</li>
                </ul>
              </article>
            </section>

          </section>
          <section v-else-if="store.activeModuleId === 'userOrders'" class="user-orders-page">
            <section class="membership-order-panel">
              <header>
                <div>
                  <strong>订单明细</strong>
                  <span>只展示当前账号的历史交易、支付状态和到账点数，充值/订阅请进入左侧独立模块操作</span>
                </div>
                <em>{{ userMembershipOrders.length }} 条</em>
              </header>
              <el-table v-if="userMembershipOrders.length" class="user-order-table" :data="userMembershipOrders" stripe>
                <el-table-column prop="id" label="订单号" min-width="130" show-overflow-tooltip />
                <el-table-column prop="orderTypeText" label="类型" min-width="120" />
                <el-table-column prop="plan" label="套餐/商品" min-width="140" show-overflow-tooltip />
                <el-table-column prop="paymentMethodText" label="支付方式" min-width="110" />
                <el-table-column prop="amountCents" label="金额" min-width="110">
                  <template #default="scope">{{ formatCell(scope.row.amountCents, 'amountCents') }}</template>
                </el-table-column>
                <el-table-column prop="rechargePoints" label="到账点数" min-width="110">
                  <template #default="scope">{{ scope.row.rechargePoints ? `${formatNumber(Number(scope.row.rechargePoints))} 点` : '-' }}</template>
                </el-table-column>
                <el-table-column prop="tokenGrantAmount" label="Token权益" min-width="110">
                  <template #default="scope">{{ scope.row.tokenGrantAmount ? `${formatNumber(Number(scope.row.tokenGrantAmount))} 点` : '-' }}</template>
                </el-table-column>
                <el-table-column prop="status" label="状态" min-width="100">
                  <template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ statusLabel(scope.row.status) }}</el-tag></template>
                </el-table-column>
                <el-table-column prop="fulfillmentStatus" label="履约" min-width="110">
                  <template #default="scope"><el-tag :type="scope.row.fulfillmentStatus === 'FULFILLED' ? 'success' : 'info'">{{ scope.row.fulfillmentStatus || '-' }}</el-tag></template>
                </el-table-column>
                <el-table-column prop="createdAt" label="创建时间" min-width="180" show-overflow-tooltip />
                <el-table-column prop="paidAt" label="支付时间" min-width="180" show-overflow-tooltip />
              </el-table>
              <el-empty v-else description="暂无订单明细记录" />
            </section>
          </section>
          <BillingDomain v-else-if="billingModuleIds.includes(store.activeModuleId)" :module-id="store.activeModuleId" />
          <section v-else-if="store.activeModuleId === 'apiSettings'" class="api-settings-admin">
            <section class="api-settings-titlebar">
              <div>
                <h2>API 设置</h2>
                <p>管理平台地址、模型列表和 Key。 Key 写入后端 env，页面不会回显完整内容。</p>
              </div>
              <span>已拉取 {{ apiAvailableModelCount }} 个模型 · 点「选择模型」勾选要导入的 · 已识别方舟协议，火山聊天建议改填 EP-... 接入点</span>
            </section>
            <section class="api-reference-layout">
              <aside class="api-reference-sidebar">
                <strong>平台列表</strong>
                <div class="api-provider-logos">
                  <span class="api-logo api-logo-modelscope"><i></i>ModelScope</span>
                  <span class="api-logo api-logo-runninghub">RunningHub</span>
                  <span class="api-logo api-logo-volc"><i></i>火山引擎</span>
                </div>
                <div class="api-reference-list">
                  <button
                    v-for="(channel, index) in apiReferenceChannels"
                    :key="String(channel.id || channel.name || index)"
                    class="api-reference-channel"
                    :class="{ 'is-active': selectedApiReferenceIndex === index, 'is-dragging': apiDraggingProviderId === apiProviderSortKey(channel, index), 'is-reordering': apiReorderingProviders }"
                    type="button"
                    :draggable="apiProviderCanReorder(channel)"
                    @dragstart="startApiProviderDrag($event, channel, index)"
                    @dragover.prevent="updateApiProviderDragOver($event, channel, index)"
                    @drop.prevent="dropApiProvider($event, channel, index)"
                    @dragend="resetApiProviderDrag"
                    @click="selectApiReferenceChannel(index)"
                  >
                    <span class="api-drag-handle" title="拖动调整顺序">::</span>
                    <span class="api-channel-key"><el-icon><Key /></el-icon></span>
                    <span class="api-channel-copy">
                      <b>{{ channel.name || 'API' }}</b>
                      <small>{{ channel.baseUrl || '未配置地址' }}</small>
                    </span>
                    <em>{{ apiProtocolLabel(channel) }}</em>
                  </button>
                </div>
                <button class="api-add-platform" type="button" @click="addApiProviderDraft"><el-icon><Plus /></el-icon>新增平台</button>
                <button class="api-recommend-button" type="button" @click="openApiRecommendMode"><el-icon><Star /></el-icon>推荐API</button>
              </aside>
              <main v-if="!apiRecommendMode" class="api-reference-main api-source-content">
                <section class="api-source-head">
                  <div>
                    <h3>{{ apiProviderDraft.name || '平台' }}</h3>
                    <p>配置基础信息、API Key 和可用模型</p>
                  </div>
                  <div class="api-source-actions">
                    <button class="api-source-action is-danger" type="button" @click="deleteSelectedApiProvider"><el-icon><Delete /></el-icon>删除</button>
                    <button class="api-source-action is-save" type="button" :disabled="apiSavingProviderDraft" @click="() => saveApiProviderDraft()"><el-icon><Check /></el-icon>{{ apiSavingProviderDraft ? '保存中' : '保存' }}</button>
                  </div>
                </section>
                <section class="api-source-block">
                  <header>
                    <div>
                      <h4>基本信息</h4>
                      <p>平台显示名、唯一 ID 和请求地址</p>
                    </div>
                  </header>
                  <div class="api-source-form">
                    <label class="api-source-field is-full">
                      <span>平台名称</span>
                      <div><input v-model="apiProviderDraft.name" placeholder="Comfly" /></div>
                      <small>平台 ID: {{ apiProviderIdPreview }}</small>
                    </label>
                    <label class="api-source-field is-full">
                      <span>请求地址</span>
                      <div><input v-model="apiProviderDraft.baseUrl" placeholder="https://api.example.com/v1" /></div>
                      <small>国内默认请求地址：<code>https://api-inference.modelscope.cn/v1</code></small>
                      <small>方舟默认请求地址：<code>https://ark.cn-beijing.volces.com/api/v3</code></small>
                    </label>
                    <label class="api-source-field">
                      <span>图片生成端点</span>
                      <div><input v-model="apiProviderDraft.imageGenerationEndpoint" placeholder="/v1/images/generations" /></div>
                      <small>以 / 开头表示从域名根路径请求。</small>
                    </label>
                    <label class="api-source-field">
                      <span>视频生成端点</span>
                      <div><input v-model="apiProviderDraft.videoGenerationEndpoint" placeholder="contents/generations/tasks" /></div>
                      <small>移动云 Seedance 使用 <code>contents/generations/tasks</code>。</small>
                    </label>
                    <label class="api-source-field is-full">
                      <span>API Key</span>
                      <div class="api-source-key-line">
                        <input v-model="apiProviderDraft.apiKey" type="password" :placeholder="apiProviderKeyPlaceholder" />
                        <button type="button" title="保存 Key" @click="() => saveApiProviderDraft()"><el-icon><Check /></el-icon></button>
                        <button type="button" title="清除 Key" @click="clearApiProviderKey"><el-icon><Delete /></el-icon></button>
                      </div>
                      <small>{{ apiProviderKeyHint }}</small>
                    </label>
                    <div class="api-source-verify-row">
                      <button type="button" :disabled="apiTestingProviderDraft || apiSavingProviderDraft" @click="testApiProviderDraft"><el-icon><Check /></el-icon>{{ apiTestingProviderDraft ? '验证中' : '验证地址' }}</button>
                      <button type="button" :disabled="apiProbingProviderProtocol || apiSavingProviderDraft" @click="probeApiProviderProtocol"><el-icon><Monitor /></el-icon>{{ apiProbingProviderProtocol ? '检测中' : '验证协议' }}</button>
                      <select v-model="apiProviderDraft.protocol">
                        <option value="openai">OpenAI 直连</option>
                        <option value="apimart">异步协议</option>
                        <option value="gemini">Gemini 协议</option>
                        <option value="volcengine">方舟/Ark 任务协议</option>
                        <option value="runninghub">RunningHub OpenAPI</option>
                        <option value="jimeng">即梦 CLI</option>
                        <option value="cloudbase-function">CloudBase 云函数</option>
                      </select>
                      <select v-model="apiProviderDraft.imageRequestMode">
                        <option value="openai">图片：OpenAI 标准</option>
                        <option value="openai-json">图片：OpenAI JSON</option>
                        <option value="cloudbase-function">图片：CloudBase 云函数</option>
                      </select>
                    </div>
                    <div v-if="apiVerifyPanel || apiVerifyResult" class="api-source-result" :class="apiVerifyPanel ? `is-${apiVerifyPanel.tone}` : ''">
                      <template v-if="apiVerifyPanel">
                        <div class="api-source-result-main">{{ apiVerifyPanel.icon }} {{ apiVerifyPanel.message }}</div>
                        <div class="api-source-result-sub">
                          {{ apiVerifyPanel.protocolPrefix }}：<strong>{{ apiVerifyPanel.protocolLabel }}</strong>
                          · 图片接口：<strong>{{ apiVerifyPanel.imageRequestModeLabel }}</strong>
                        </div>
                        <details v-if="apiVerifyPanel.raw" class="api-source-raw" open>
                          <summary>▸ 查看原始响应 (HTTP {{ apiVerifyPanel.statusCode }})</summary>
                          <pre>{{ prettyApiVerifyRaw(apiVerifyPanel.raw) }}</pre>
                        </details>
                      </template>
                      <template v-else>{{ apiVerifyResult }}</template>
                    </div>
                  </div>
                </section>
                <section class="api-source-block api-source-models">
                  <header>
                    <div>
                      <h4>模型列表</h4>
                      <p>从上游 API 自动拉取所有可用模型并按类型分类（image / chat / video）</p>
                    </div>
                    <div class="api-source-actions">
                      <button class="api-source-action" type="button" :disabled="apiFetchingDraftModels || apiSyncingDraftModels" @click="fetchApiDraftModels"><el-icon><Download /></el-icon>{{ apiFetchingDraftModels ? "拉取中..." : "拉取模型" }}</button>
                      <button class="api-source-action" type="button" :disabled="apiFetchingDraftModels || apiSyncingDraftModels" @click="syncAllApiDraftModels"><el-icon><Refresh /></el-icon>{{ apiSyncingDraftModels ? "同步中..." : "同步全部候选" }}</button>
                      <button class="api-source-action" type="button" @click="selectApiDraftModels"><el-icon><Tickets /></el-icon>选择模型</button>
                    </div>
                  </header>
                  <div class="api-source-model-grid">
                    <section>
                      <header><div><h4>生图模型</h4><p>在线生图和无限画布 API 生成使用</p></div><button type="button" @click="addApiDraftModel('image')"><el-icon><Plus /></el-icon>模型</button></header>
                      <div class="api-source-model-list">
                        <div v-if="!apiProviderDraft.imageModels.length" class="api-source-model-empty">暂无模型</div>
                        <div v-for="(_, index) in apiProviderDraft.imageModels" :key="`image-${index}`" class="api-source-model-row has-protocol">
                          <input v-model="apiProviderDraft.imageModels[index]" />
                          <select v-model="apiProviderDraft.modelProtocols[`image:${index}`]" class="api-source-model-protocol" title="该模型使用的协议，默认跟随平台全局协议">
                            <option value="">默认</option>
                            <option value="openai">OpenAI</option>
                            <option value="gemini">Gemini</option>
                          </select>
                          <button type="button" title="删除" @click="removeApiDraftModel('image', index)"><el-icon><Delete /></el-icon></button>
                        </div>
                      </div>
                    </section>
                    <section>
                      <header><div><h4>聊天模型</h4><p>GPT 对话和 LLM 节点使用</p></div><button type="button" @click="addApiDraftModel('chat')"><el-icon><Plus /></el-icon>模型</button></header>
                      <div class="api-source-model-list">
                        <div v-if="!apiProviderDraft.chatModels.length" class="api-source-model-empty">暂无模型</div>
                        <div v-for="(_, index) in apiProviderDraft.chatModels" :key="`chat-${index}`" class="api-source-model-row has-protocol">
                          <input v-model="apiProviderDraft.chatModels[index]" />
                          <select v-model="apiProviderDraft.modelProtocols[`chat:${index}`]" class="api-source-model-protocol" title="该模型使用的协议，默认跟随平台全局协议">
                            <option value="">默认</option>
                            <option value="openai">OpenAI</option>
                            <option value="gemini">Gemini</option>
                          </select>
                          <button type="button" title="删除" @click="removeApiDraftModel('chat', index)"><el-icon><Delete /></el-icon></button>
                        </div>
                      </div>
                    </section>
                    <section>
                      <header><div><h4>视频模型</h4><p>无限画布视频生成节点使用</p></div><button type="button" @click="addApiDraftModel('video')"><el-icon><Plus /></el-icon>模型</button></header>
                      <div class="api-source-model-list">
                        <div v-if="!apiProviderDraft.videoModels.length" class="api-source-model-empty">暂无模型</div>
                        <div v-for="(_, index) in apiProviderDraft.videoModels" :key="`video-${index}`" class="api-source-model-row">
                          <input v-model="apiProviderDraft.videoModels[index]" />
                          <button type="button" title="删除" @click="removeApiDraftModel('video', index)"><el-icon><Delete /></el-icon></button>
                        </div>
                      </div>
                    </section>
                    <section>
                      <header><div><h4>LoRA 管理</h4><p>为 ModelScope 生图模型绑定可用 LoRA。</p></div><button type="button" @click="addApiDraftModel('lora')"><el-icon><Plus /></el-icon>LoRA</button></header>
                      <div class="api-source-model-list"><label v-for="(_, index) in apiProviderDraft.loras" :key="`lora-${index}`"><input v-model="apiProviderDraft.loras[index]" /><button type="button" @click="removeApiDraftModel('lora', index)">×</button></label></div>
                    </section>
                  </div>
                </section>
              </main>
              <main v-else class="api-reference-main api-source-content">
                <section class="api-source-head">
                  <div>
                    <h3>推荐平台</h3>
                    <p>获取 Key 后保存，右侧会切换到刚保存的平台配置。</p>
                  </div>
                  <div class="api-source-actions">
                    <button class="api-source-action" type="button" @click="apiRecommendMode = false"><el-icon><ArrowDown /></el-icon>返回配置</button>
                  </div>
                </section>
                <section class="api-recommend-board">
                  <header>
                    <h3>推荐平台</h3>
                    <p>选择适合的平台，获取 Key 后在右侧保存为默认配置。</p>
                  </header>
                  <article v-for="platform in apiRecommendedPlatforms" :key="platform.name" class="api-recommend-row" @click="focusRecommendedPlatform(platform.name)">
                    <div class="api-platform-copy">
                      <h4>{{ platform.name }}</h4>
                      <p>{{ platform.desc }}</p>
                      <div class="api-platform-tags">
                        <span v-for="tag in platform.tags" :key="tag" :class="{ 'is-gift': tag.includes('签到'), 'is-free': tag === '免费' }">{{ tag }}</span>
                      </div>
                    </div>
                    <div class="api-quick-card">
                      <b>快捷设置</b>
                      <label>获取 Key</label>
                      <button type="button" @click="openApiKeyUrl(platform.keyUrl)"><el-icon><Key /></el-icon>获取 Key</button>
                    </div>
                    <span class="api-flow-arrow">→</span>
                    <div class="api-key-card">
                      <label>API Key</label>
                      <div>
                        <input v-model="apiQuickKeys[platform.name]" :placeholder="`粘贴 ${platform.name} API Key`" type="password" @click.stop />
                        <button type="button" @click.stop="saveRecommendedPlatform(platform)">保存</button>
                      </div>
                    </div>
                  </article>
                </section>
              </main>
            </section>
          </section>
          <section v-else-if="store.activeModuleId === 'userWirelessCanvas'" class="wireless-canvas-admin-page">
            <div v-if="!wirelessCanvasFrameLoaded" class="wireless-canvas-admin-loading" role="status" aria-live="polite">
              <strong>无线画布加载中</strong>
              <span>正在准备节点、资产库与工作流工具</span>
            </div>
            <iframe
              v-if="wirelessCanvasFrameSrc"
              class="wireless-canvas-admin-frame"
              :class="{ 'is-loading': !wirelessCanvasFrameLoaded }"
              :src="wirelessCanvasFrameSrc"
              title="无线画布"
              loading="lazy"
              referrerpolicy="same-origin"
              allow="clipboard-read; clipboard-write; fullscreen"
              @load="handleWirelessCanvasFrameLoad"
            ></iframe>
          </section>
          <section v-else-if="store.activeModuleId === 'userVideoGeneration'" class="video-generation-page" @click="openVideoDropdown = ''">
            <section class="video-studio-shell">
              <div class="video-gallery-stage">
                <div v-if="selectedVideoHistoryEntry" class="video-current-preview">
                  <video class="video-preview-frame" :src="selectedVideoHistoryEntry.url" controls playsinline preload="metadata" />
                  <div class="video-current-copy">
                    <span>{{ videoModeLabel(selectedVideoHistoryEntry.mode) }}</span>
                    <h2>{{ selectedVideoHistoryEntry.prompt || "未填写提示词" }}</h2>
                    <p>{{ selectedVideoHistoryEntry.model }} · {{ selectedVideoHistoryEntry.duration || "-" }}s · {{ selectedVideoHistoryEntry.aspect_ratio || "-" }} · {{ selectedVideoHistoryEntry.resolution || "-" }}</p>
                  </div>
                </div>
                <div v-else class="video-empty-hero">
                  <div class="video-empty-icon">
                    <el-icon><Monitor /></el-icon>
                  </div>
                  <h2><span>开始创作</span><strong>VIDEO STUDIO</strong></h2>
                  <p>上传图片生成动态视频，或直接输入提示词生成镜头。</p>
                </div>
                <section v-if="videoHistory.length" class="video-history-panel">
                  <header class="video-history-head">
                    <div>
                      <span>历史生成</span>
                      <strong>{{ filteredVideoHistory.length }} / {{ videoHistory.length }}</strong>
                    </div>
                    <div class="video-history-tools">
                      <label class="video-history-search">
                        <el-icon><Search /></el-icon>
                        <input
                          v-model="videoHistorySearchQuery"
                          type="search"
                          placeholder="搜索提示词、模型、ID..."
                          autocomplete="off"
                          spellcheck="false"
                          @click.stop
                        />
                        <button
                          v-if="videoHistorySearchQuery"
                          type="button"
                          class="video-history-search-clear"
                          aria-label="清空视频历史搜索"
                          title="清空搜索"
                          @click.stop="videoHistorySearchQuery = ''"
                        >
                          ×
                        </button>
                      </label>
                      <select v-model="videoHistoryFilter" @click.stop>
                        <option value="ALL">全部</option>
                        <option value="text-to-video">文生视频</option>
                        <option value="image-to-video">图生视频</option>
                        <option value="video-to-video">视频编辑</option>
                        <option value="success">生成成功</option>
                        <option value="generating">生成中</option>
                        <option value="failed">生成失败</option>
                      </select>
                      <select v-model="videoHistorySort" @click.stop>
                        <option value="desc">最新优先</option>
                        <option value="asc">最旧优先</option>
                      </select>
                    </div>
                  </header>
                  <div v-if="filteredVideoHistory.length" class="video-history-grid">
                    <article
                      v-for="entry in visibleVideoHistory"
                      :key="entry.id"
                      :class="['video-history-card', `is-${entry.status}`]"
                      @click="openVideoFullscreen(entry)"
                    >
                      <div class="video-history-media" @mouseenter="playVideoCardPreview($event, entry)" @mouseleave="resetVideoCardPreview">
                        <video v-if="videoHistoryCardSrc(entry)" :src="videoHistoryCardSrc(entry)" muted playsinline loop preload="none"></video>
                        <div v-else class="video-history-placeholder">
                          <el-icon><Monitor /></el-icon>
                          <span>{{ entry.url ? '悬停预览' : entry.status === 'failed' ? '生成失败' : '生成中' }}</span>
                        </div>
                        <em>{{ videoStatusLabel(entry.status) }}</em>
                      </div>
                      <div class="video-history-body">
                        <p :title="entry.prompt">{{ entry.prompt || "未填写提示词" }}</p>
                        <div class="video-history-meta">
                          <span>{{ entry.model }}</span>
                          <span>{{ entry.duration || "-" }}s</span>
                          <span>{{ entry.aspect_ratio || "-" }}</span>
                          <span>{{ entry.resolution || "-" }}</span>
                        </div>
                        <small>{{ formatVideoHistoryTime(entry.createdAt) }} · {{ videoModeLabel(entry.mode) }}</small>
                        <strong v-if="entry.status === 'failed'">{{ entry.errorMessage || "生成失败" }}</strong>
                      </div>
                      <footer class="video-history-actions" @click.stop>
                        <el-tooltip content="下载视频" placement="top" :show-after="160" popper-class="video-history-tooltip">
                          <span class="video-history-action-wrap">
                            <button
                              type="button"
                              aria-label="下载视频"
                              title="下载视频"
                              :disabled="!entry.url || isVideoHistoryActionBusy(entry.id, 'download')"
                              @click.stop="downloadVideoHistory(entry)"
                            >
                              <span v-if="isVideoHistoryActionBusy(entry.id, 'download')" class="video-action-spinner"></span>
                              <el-icon v-else><Download /></el-icon>
                            </button>
                          </span>
                        </el-tooltip>
                        <el-tooltip content="复制视频链接" placement="top" :show-after="160" popper-class="video-history-tooltip">
                          <span class="video-history-action-wrap">
                            <button
                              type="button"
                              aria-label="复制视频链接"
                              title="复制视频链接"
                              :disabled="!entry.url || isVideoHistoryActionBusy(entry.id, 'copy')"
                              @click.stop="copyVideoHistoryUrl(entry)"
                            >
                              <span v-if="isVideoHistoryActionBusy(entry.id, 'copy')" class="video-action-spinner"></span>
                              <el-icon v-else><CopyDocument /></el-icon>
                            </button>
                          </span>
                        </el-tooltip>
                        <el-tooltip content="删除历史记录" placement="top" :show-after="160" popper-class="video-history-tooltip">
                          <span class="video-history-action-wrap">
                            <button
                              type="button"
                              class="danger"
                              aria-label="删除历史记录"
                              title="删除历史记录"
                              :disabled="isVideoHistoryActionBusy(entry.id, 'delete')"
                              @click.stop="deleteVideoHistoryEntry(entry)"
                            >
                              <span v-if="isVideoHistoryActionBusy(entry.id, 'delete')" class="video-action-spinner"></span>
                              <el-icon v-else><Delete /></el-icon>
                            </button>
                          </span>
                        </el-tooltip>
                      </footer>
                    </article>
                  </div>
                  <div v-if="hasMoreVideoHistory" class="video-history-load-more">
                    <span>已显示 {{ visibleVideoHistory.length }} / {{ filteredVideoHistory.length }}</span>
                    <button type="button" @click.stop="loadMoreVideoHistory">加载更多</button>
                  </div>
                  <div v-if="!filteredVideoHistory.length" class="video-history-empty">
                    <strong>未找到相关视频</strong>
                    <span>换个关键词试试</span>
                  </div>
                </section>
              </div>

              <div class="video-bottom-composer">
                <div class="video-composer-input-row">
                  <div class="video-upload-actions">
                    <div class="video-upload-native">
                      <input type="file" accept="image/*" @change="handleVideoImageFile" />
                      <button
                        type="button"
                        :class="['video-circle-tool', { active: videoStudioMode === 'image' || Boolean(videoImagePreview) }]"
                        :title="videoImagePreview ? '清除图片' : '上传图片生成视频'"
                        @click="videoImagePreview ? clearVideoImageUpload() : undefined"
                      >
                        <img v-if="videoImagePreview" :src="videoImagePreview" alt="" />
                        <svg v-else width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                          <circle cx="8.5" cy="8.5" r="1.5" />
                          <polyline points="21 15 16 10 5 21" />
                        </svg>
                      </button>
                    </div>
                    <div class="video-upload-native">
                      <input type="file" accept="video/*" @change="handleVideoSourceFile" />
                      <button
                        type="button"
                        :class="['video-circle-tool', { active: videoStudioMode === 'video' || Boolean(videoSourcePreview) }]"
                        :title="videoSourcePreview ? '清除视频' : '上传视频处理'"
                        @click="videoSourcePreview ? clearVideoSourceUpload() : undefined"
                      >
                        <video v-if="videoSourcePreview" :src="videoSourcePreview" muted playsinline />
                        <svg v-else width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <polygon points="23 7 16 12 23 17 23 7" />
                          <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
                        </svg>
                      </button>
                    </div>
                  </div>
                  <textarea
                    ref="videoPromptTextareaRef"
                    v-model="videoPrompt"
                    class="video-prompt-native"
                    rows="2"
                    wrap="soft"
                    maxlength="1200"
                    :placeholder="videoStudioMode === 'image' ? '描述图片如何动起来...' : videoStudioMode === 'video' ? '描述要如何处理这个视频...' : '描述你想生成的视频画面...'"
                    @input="handleVideoPromptInput"
                    @keydown.ctrl.enter.prevent="submitVideoGeneration"
                    @keydown.meta.enter.prevent="submitVideoGeneration"
                  ></textarea>
                </div>

                <div class="video-control-row">
                  <div class="video-control-group">
                    <div class="video-control-popover">
                      <button type="button" class="video-control-button" @click.stop="toggleVideoDropdown('model')">
                        <span class="video-model-mark">V</span>
                        <span>{{ selectedVideoModel }}</span>
                        <el-icon class="video-chevron"><ArrowDown /></el-icon>
                      </button>
                      <div v-if="openVideoDropdown === 'model'" class="video-dropdown-panel video-model-dropdown" @click.stop>
                        <label class="video-dropdown-search">
                          <el-icon><Search /></el-icon>
                          <input v-model="videoModelSearch" type="text" placeholder="搜索模型..." @click.stop />
                        </label>
                        <small>视频模型</small>
                        <button
                          v-for="model in filteredVideoModelOptions"
                          :key="model.name"
                          type="button"
                          :class="['video-model-option', `is-${model.family}`, { active: selectedVideoModel === model.name }]"
                          @click="selectVideoOption('model', model.name)"
                        >
                          <i>{{ model.name.slice(0, 1) }}</i>
                          <span><b>{{ model.name }}</b><em>{{ model.desc }}</em></span>
                          <el-icon v-if="selectedVideoModel === model.name"><Check /></el-icon>
                        </button>
                        <small class="video-tools-title">视频工具</small>
                        <button
                          v-for="tool in filteredVideoToolOptions"
                          :key="tool.name"
                          type="button"
                          :class="['video-model-option', 'is-tool', { active: selectedVideoModel === tool.name }]"
                          @click="selectVideoOption('model', tool.name)"
                        >
                          <i>{{ tool.name.slice(0, 1) }}</i>
                          <span><b>{{ tool.name }}</b><em>{{ tool.desc }}</em></span>
                          <el-icon v-if="selectedVideoModel === tool.name"><Check /></el-icon>
                        </button>
                      </div>
                    </div>
                    <div class="video-control-popover">
                      <button type="button" class="video-control-button" @click.stop="toggleVideoDropdown('ratio')">
                        <svg class="video-control-svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                        </svg>
                        <span>{{ videoRatio }}</span>
                      </button>
                      <div v-if="openVideoDropdown === 'ratio'" class="video-dropdown-panel video-ratio-dropdown" @click.stop>
                        <small>画面比例</small>
                        <button
                          v-for="item in availableVideoRatioOptions"
                          :key="item"
                          type="button"
                          :class="{ active: videoRatio === item }"
                          @click="selectVideoOption('ratio', item)"
                        >
                          <span>{{ item }}</span>
                          <el-icon v-if="videoRatio === item"><Check /></el-icon>
                        </button>
                      </div>
                    </div>
                    <div class="video-control-popover">
                      <button type="button" class="video-control-button" @click.stop="toggleVideoDropdown('duration')">
                        <el-icon><Clock /></el-icon>
                        <span>{{ videoDuration }}s</span>
                      </button>
                      <div v-if="openVideoDropdown === 'duration'" class="video-dropdown-panel" @click.stop>
                        <small>视频时长</small>
                        <button
                          v-for="item in availableVideoDurationOptions"
                          :key="item"
                          type="button"
                          :class="{ active: videoDuration === item }"
                          @click="selectVideoOption('duration', item)"
                        >
                          <span>{{ item }}s</span>
                          <el-icon v-if="videoDuration === item"><Check /></el-icon>
                        </button>
                      </div>
                    </div>
                    <div class="video-control-popover">
                      <button type="button" class="video-control-button" @click.stop="toggleVideoDropdown('resolution')">
                        <svg class="video-control-svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M6 2L3 6v15a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4H6z" />
                        </svg>
                        <span>{{ videoResolution }}</span>
                      </button>
                      <div v-if="openVideoDropdown === 'resolution'" class="video-dropdown-panel video-resolution-dropdown" @click.stop>
                        <small>清晰度</small>
                        <button
                          v-for="item in availableVideoResolutionOptions"
                          :key="item"
                          type="button"
                          :class="{ active: videoResolution === item }"
                          @click="selectVideoOption('resolution', item)"
                        >
                          <span>{{ item }}</span>
                          <el-icon v-if="videoResolution === item"><Check /></el-icon>
                        </button>
                      </div>
                    </div>
                    <button
                      type="button"
                      :class="['video-control-button', { active: videoGenerateAudio }]"
                      :title="videoGenerateAudio ? '配音已开启' : '配音已关闭'"
                      @click="videoGenerateAudio = !videoGenerateAudio"
                    >
                      <svg class="video-control-svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M12 3a3 3 0 0 0-3 3v5a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3Z" />
                        <path d="M19 10v1a7 7 0 0 1-14 0v-1" />
                        <path d="M12 18v3" />
                        <path d="M8 21h8" />
                      </svg>
                      <span>{{ videoGenerateAudio ? '配音' : '静音' }}</span>
                    </button>
                  </div>
                  <div class="video-submit-group">
                    <button type="button" class="video-generate-button" :disabled="videoSubmitting" @click="submitVideoGeneration">
                      {{ videoSubmitting ? '生成中' : '生成' }}
                    </button>
                  </div>
                </div>
              </div>
            </section>
          </section>
          <section v-else-if="store.activeModuleId === 'userPptGeneration'" class="ppt-doc-shell">
            <PptDocumentGeneration />
          </section>
          <ChannelGrowthDomain
            v-else-if="channelGrowthModuleIds.includes(store.activeModuleId)"
            :module-title="store.activeModule.title"
            :persistence-key="store.activeModuleId"
            :rows="rows"
            :saving="store.saving"
            :toolbar-actions="toolbarActions"
            :columns="columns"
            :column-labels="columnLabels"
            :row-actions="rowActions"
            v-model:search-keyword="searchKeyword"
            v-model:status-filter="statusFilter"
            :status-filter-options="statusFilterOptions"
            :is-status-column="isStatusColumn"
            :status-type="statusType"
            :status-label="statusLabel"
            :format-cell="formatCell"
            :visible-row-actions="visibleRowActions"
            :label-for-row-action="labelForRowAction"
            @run-action="runAction"
            @batch-action="runBatchAction"
          />
          <Customer360Center
            v-else-if="['customers', 'userManagement'].includes(store.activeModuleId)"
            :rows="rows"
            :saving="store.saving"
            :toolbar-actions="toolbarActions"
            :row-actions="rowActions"
            :permissions="currentPermissions"
            :column-labels="columnLabels"
            :status-filter-options="statusFilterOptions"
            :is-status-column="isStatusColumn"
            :status-type="statusType"
            :status-label="statusLabel"
            :format-cell="formatCell"
            :visible-row-actions="visibleRowActions"
            :label-for-row-action="labelForRowAction"
            @run-action="runAction"
            @batch-action="runBatchAction"
          />
          <AdminDataTable v-else-if="!['userDashboard', 'userAiImage', 'userAgentCenter', 'userWirelessCanvas', 'userWorks', 'userVideoGeneration', 'userPptGeneration', 'apiSettings'].includes(store.activeModuleId)" :title="store.activeModule.title" :persistence-key="store.activeModuleId" :rows="rows" :columns="columns" :column-labels="columnLabels" :toolbar-actions="toolbarActions" :row-actions="rowActions" :batch-actions="rowActions" v-model:search-keyword="searchKeyword" v-model:status-filter="statusFilter" :status-filter-options="statusFilterOptions" :loading="store.saving" :is-status-column="isStatusColumn" :status-type="statusType" :status-label="statusLabel" :format-cell="formatCell" :visible-row-actions="visibleRowActions" :label-for-row-action="labelForRowAction" @run-action="runAction" @batch-action="runBatchAction" />
        </section>
      </el-main>
    </el-container>
  </el-container>
  <PlanEditorDialog
    v-model="planEditorOpen"
    :plan="editingPlan"
    :saving="store.saving"
    @save="savePlanConfiguration"
  />
  <GlobalCommandPalette v-if="isUserConsole || isAgentConsole" v-model:open="commandPaletteOpen" v-model:query="globalSearchKeyword" :module-results="globalModuleResults" :record-results="currentRecordResults" :business-results="globalBusinessResults" @select-module="openGlobalModuleResult" @select-record="openCurrentRecordResult" @select-business="openGlobalBusinessResult" />
  <div v-if="apiModelPickerOpen" class="api-model-picker-overlay" @click.self="closeApiModelPicker">
    <section class="api-model-picker-modal">
      <header class="api-model-picker-head">
        <div>
          <h3>从上游拉取的模型清单</h3>
          <p>共 {{ apiModelPickerCounts.all.total }} 个模型 · 当前显示 {{ filteredApiPickerModels.length }} 个</p>
        </div>
        <button type="button" class="api-model-picker-close" @click="closeApiModelPicker">×</button>
      </header>
      <div class="api-model-picker-toolbar">
        <input v-model="apiModelPickerFilter" class="api-model-picker-search" placeholder="按名称搜索模型..." />
        <div class="api-model-picker-tabs">
          <button
            v-for="tab in apiModelPickerTabs"
            :key="tab.value"
            type="button"
            :class="{ active: apiModelPickerTab === tab.value }"
            @click="apiModelPickerTab = tab.value"
          >
            {{ tab.label }} <span>{{ apiModelPickerCounts[tab.value].selected }}/{{ apiModelPickerCounts[tab.value].total }}</span>
          </button>
        </div>
      </div>
      <div class="api-model-picker-body">
        <button
          v-for="model in filteredApiPickerModels"
          :key="model.id"
          type="button"
          class="api-model-picker-row"
          :class="{ 'has-sel': model.selected }"
          @click="toggleApiPickerModel(model.id)"
        >
          <span class="api-model-picker-check" :class="{ checked: model.selected }"><Check /></span>
          <span class="api-model-picker-name" :title="model.id">{{ model.id }}</span>
          <span class="api-model-picker-kind" :class="`cat-${model.category}`">{{ apiModelCategoryLabel(model.category) }}</span>
        </button>
        <div v-if="!filteredApiPickerModels.length" class="api-model-picker-empty">无匹配模型</div>
      </div>
      <footer class="api-model-picker-summary">
        <span class="api-model-picker-summary-title">将应用：</span>
        <span :class="{ empty: apiModelPickerCounts.image.selected === 0 }">生图 {{ apiModelPickerCounts.image.selected }}</span>
        <span :class="{ empty: apiModelPickerCounts.chat.selected === 0 }">LLM {{ apiModelPickerCounts.chat.selected }}</span>
        <span :class="{ empty: apiModelPickerCounts.video.selected === 0 }">视频 {{ apiModelPickerCounts.video.selected }}</span>
        <em>未选 {{ apiModelPickerCounts.all.total - apiModelPickerCounts.all.selected }}</em>
      </footer>
      <div class="api-model-picker-foot">
        <button type="button" class="api-model-picker-btn" @click="closeApiModelPicker">取消</button>
        <button type="button" class="api-model-picker-btn is-save" @click="applyApiModelPicker">应用到模型列表</button>
      </div>
    </section>
  </div>
  <div v-if="videoFullscreenEntry" class="video-lightbox-overlay" @click.self="closeVideoFullscreen">
    <button type="button" class="video-lightbox-close" @click="closeVideoFullscreen">×</button>
    <section class="video-lightbox-card">
      <video :src="videoFullscreenEntry.url" controls autoplay playsinline></video>
      <div>
        <strong>{{ videoFullscreenEntry.prompt || "未填写提示词" }}</strong>
        <span>{{ videoFullscreenEntry.model }} · {{ formatVideoHistoryTime(videoFullscreenEntry.createdAt) }}</span>
      </div>
    </section>
  </div>
  <AuthModal v-model="workspaceLoginOpen" @authenticated="handleWebLoginAuthenticated" @cancelled="handleWorkspaceLoginCancelled" />
  </el-config-provider>
</template>
<script setup lang="ts">
import { computed, defineAsyncComponent, h, nextTick, onBeforeUnmount, onMounted, ref, watch, type Component } from "vue";
import { safeInternalRedirect, type ProtectedAction } from "@xianzhi/shared-auth";
import { ElMessage } from "element-plus/es/components/message/index";
import { ElMessageBox } from "element-plus/es/components/message-box/index";
import type { ComponentSize } from "element-plus";
import { ArrowDown, Check, Clock, Collection, Connection, CopyDocument, Cpu, Crop, DataAnalysis, Delete, Document, Download, EditPen, Goods, Grid, House, Key, Link, Lock, Money, Monitor, Operation, Plus, QuestionFilled, Refresh, Search, Setting, Star, StarFilled, SwitchButton, Tickets, User, UserFilled, Wallet } from "@element-plus/icons-vue";
import { adminRequest } from "./api/client";
import type { GlobalSearchItem } from "./api/adminWorkspaces";
import { downloadAssetBlob, fetchResourceBlob, uploadReferenceImage } from "./api/resources";
import { useAdminGlobalSearch } from "./composables/useAdminGlobalSearch";
import { trackAdminExperience } from "./composables/useAdminExperienceTracking";
import { trackWebGuestExperience } from "./utils/webGuestAnalytics";
import { useOfficeCLI } from "./composables/useOfficeCLI";
import PromptEditable from "./components/PromptEditable.vue";
import AuthModal from "./components/auth/AuthModal.vue";
import WebLoginPage from "./components/auth/WebLoginPage.vue";
import AdminSectionTabs from "./components/navigation/AdminSectionTabs.vue";
import GlobalCommandPalette from "./components/navigation/GlobalCommandPalette.vue";
import {
  adminModuleById,
  adminNavigationGroups,
  adminNavigationSectionForModule,
  agentModuleIds,
  aiCapabilityModuleIds,
  billingModuleIds,
  channelGrowthModuleIds,
  customerAttributionModuleIds,
  enterpriseModuleIds,
  mediaDecorationModuleIds,
  mediaOperationModuleIds,
  operationCenterModuleIds,
  userModuleIds,
  type AdminNavigationIconKey
} from "./config/adminNavigation";
import { modulePermission, resolveModuleIdFromPath, resolveModulePath } from "./config/moduleRegistry";
import { adminModules, type AdminRecord, useAdminStore } from "./stores/admin";
import { useWebAuthStore, type WebAuthResponse } from "./stores/auth";
import { type AiSettingsDraft, useAiSettingsStore } from "./stores/aiSettings";
import {
  agentCenterMetrics,
  agentCenterMobileRows,
  agentCenterRanking,
  agentCenterRows,
  agentCenterShortcuts,
  agentCenterTemplates,
  agentCenterTrend,
  buildAgentCenterWorkspace,
  findAgentCenterOpenable,
  isOfficeCLIItem,
  type AgentCenterOpenable,
  type AgentCenterWorkspace,
  type AgentWorkspaceMessage
} from "./utils/agentCenter";
import {
  clearCurrentAiImageCache,
  clearPendingReferenceImages,
  readAiImageDraft,
  readCachedOriginalImage,
  readPendingReferenceImages,
  writeAiImageDraft,
  writeCachedOriginalImage,
  writePendingReferenceImages,
  type CachedReferenceImage,
} from "./utils/aiImageDb";
import {
  isVideoGenerationTask,
  normalizeVideoTimestamp,
  videoDurationOptions,
  videoErrorMessage,
  videoInputImageUrlsFromTask,
  videoModeFromTask,
  videoModelId,
  videoModelOptions,
  videoModelParameterOptions,
  videoNumberOrString,
  videoRatioOptions,
  videoResolutionOptions,
  videoStatusFromTask,
  videoStringValue,
  videoTaskParams,
  videoTaskUrl,
  videoToolOptions,
  type VideoHistoryEntry,
  type VideoHistoryStatus
} from "./utils/videoGeneration";
import xianzhiLogo from "./assets/xianzhi-ai-logo.webp";
import { isPersistentWebSession } from "./utils/webAuthSession";

function aiPlaygroundMessage(type: "success" | "warning" | "error" | "info", message: string) {
  ElMessage({
    type,
    message,
    customClass: "ai-playground-message",
    duration: 2400,
    showClose: false,
    grouping: true
  });
}

const store = useAdminStore();
const authStore = useWebAuthStore();
const AdminDataTable = defineAsyncComponent(() => import("./components/admin/AdminDataTable.vue"));
const Customer360Center = defineAsyncComponent(() => import("./components/admin/Customer360Center.vue"));
const BillingDomain = defineAsyncComponent(() => import("./components/billing/BillingDomain.vue"));
const CustomerAttributionOverview = defineAsyncComponent(() => import("./components/attribution/CustomerAttributionOverview.vue"));
const AiCapabilityDomain = defineAsyncComponent(() => import("./components/ai/AiCapabilityDomain.vue"));
const EnterpriseManagement = defineAsyncComponent(() => import("./components/enterprise/EnterpriseManagement.vue"));
const ConnectorAuthorizationCenter = defineAsyncComponent(() => import("./components/enterprise/ConnectorAuthorizationCenter.vue"));
const FeishuConnectorSetup = defineAsyncComponent(() => import("./components/enterprise/FeishuConnectorSetup.vue"));
const ChannelGrowthDomain = defineAsyncComponent(() => import("./components/growth/ChannelGrowthDomain.vue"));
const KnowledgeAdminCenter = defineAsyncComponent(() => import("./components/knowledge/KnowledgeAdminCenter.vue"));
const KnowledgeAgentCenter = defineAsyncComponent(() => import("./components/knowledge/KnowledgeAgentCenter.vue"));
const InspirationManagement = defineAsyncComponent(() => import("./components/inspiration/InspirationManagement.vue"));
const MediaCenter = defineAsyncComponent(() => import("./components/media/MediaCenter.vue"));
const PageDecoration = defineAsyncComponent(() => import("./components/media/PageDecoration.vue"));
const PlanEditorDialog = defineAsyncComponent(() => import("./components/billing/PlanEditorDialog.vue"));
const StorageCenter = defineAsyncComponent(() => import("./components/storage/StorageCenter.vue"));
const planEditorOpen = ref(false);
const editingPlan = ref<AdminRecord | null>(null);
const aiSettingsStore = useAiSettingsStore();
const modules = adminModules;
const PptDocumentGeneration = defineAsyncComponent({
  loader: () => import("./components/PptDocumentGeneration.vue"),
  delay: 0,
  suspensible: false
});
const decorationInitialPage = computed(() => ({ pageHomeConfig: "home", pageStudioConfig: "studio", pageAssetsConfig: "assets", pageProfileConfig: "profile" } as Record<string, string>)[store.activeModuleId] || "home");
const elementSizeStorageKey = "xianzhi-admin-element-size";
const elementSizeOptions: Array<{ label: string; value: ComponentSize }> = [
  { label: "默认", value: "default" },
  { label: "大型", value: "large" },
  { label: "小型", value: "small" }
];
const savedElementSize = typeof window !== "undefined" ? window.localStorage.getItem(elementSizeStorageKey) : "";
const currentElementSize = ref<ComponentSize>(elementSizeOptions.some((item) => item.value === savedElementSize) ? (savedElementSize as ComponentSize) : "default");
const elementSizeLabel = computed(() => elementSizeOptions.find((item) => item.value === currentElementSize.value)?.label || "默认");
function setElementSize(command: string) {
  const nextSize = elementSizeOptions.find((item) => item.value === command)?.value;
  if (!nextSize) return;
  currentElementSize.value = nextSize;
  if (typeof window !== "undefined") window.localStorage.setItem(elementSizeStorageKey, nextSize);
}
const searchKeyword = ref("");
const globalSearchKeyword = ref("");
const commandPaletteOpen = ref(false);
const { results: globalBusinessResults } = useAdminGlobalSearch(globalSearchKeyword);
function handleGlobalCommandShortcut(event: KeyboardEvent) {
  if (!isUserConsole.value && !isAgentConsole.value) return;
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    commandPaletteOpen.value = true;
  }
}
const statusFilter = ref("ALL");
const statusFilterOptions = computed(() => {
  if (store.activeModuleId === "partnerCommissions") {
    return [
      { label: "全部", value: "ALL" },
      { label: "待结算", value: "PENDING" },
      { label: "已通过", value: "APPROVED" },
      { label: "已驳回", value: "REJECTED" }
    ];
  }
  if (["partnerUsage", "partnerOrders", "userUsage", "userAiImage"].includes(store.activeModuleId)) {
    return [
      { label: "全部", value: "ALL" },
      { label: "成功", value: "SUCCEEDED" },
      { label: "处理中", value: "PENDING" },
      { label: "失败", value: "FAILED" }
    ];
  }
  return [
    { label: "全部", value: "ALL" },
    { label: "启用", value: "ACTIVE" },
    { label: "待处理", value: "PENDING" },
    { label: "停用", value: "DISABLED" }
  ];
});

const videoStudioMode = ref("text");
const videoPrompt = ref("");
const selectedVideoModel = ref("Grok Image Video");
const videoModelSearch = ref("");
const videoDuration = ref(5);
const videoRatio = ref("16:9");
const videoResolution = ref("720p");
const videoGenerateAudio = ref(true);
const videoSubmitting = ref(false);
const videoResultTask = ref<AdminRecord | null>(null);
const openVideoDropdown = ref<"" | "model" | "ratio" | "duration" | "resolution">("");
const videoImagePreview = ref("");
const videoSourcePreview = ref("");
const videoHistory = ref<VideoHistoryEntry[]>([]);
const videoHiddenHistoryIds = ref<string[]>([]);
const selectedVideoHistoryId = ref("");
const videoHistorySearchQuery = ref("");
const videoHistoryFilter = ref("ALL");
const videoHistorySort = ref<"desc" | "asc">("desc");
const videoFullscreenEntry = ref<VideoHistoryEntry | null>(null);
const videoHistoryBusyActions = ref<Record<string, boolean>>({});
const videoHistoryInitialVisibleCount = 12;
const videoHistoryPageSize = 12;
const videoHistoryVisibleCount = ref(videoHistoryInitialVisibleCount);
const activeVideoHistoryPreviewIds = ref<string[]>([]);
const videoPromptTextareaRef = ref<HTMLTextAreaElement | null>(null);
const videoHistoryStorageBaseKey = "xianzhi_video_history";
const videoPromptDraftBaseKey = "xianzhi_video_prompt_draft";
const videoHistorySearchBaseKey = "xianzhi_video_history_search";
const videoHistoryLimit = 50;
let videoHistoryHydrated = false;
let videoHistorySaveTimer: number | null = null;
let videoInputDraftSaveTimer: number | null = null;
let videoHistoryPollTimer: number | null = null;
let videoImageObjectUrl = "";
let videoSourceObjectUrl = "";
let videoImageFile: File | null = null;
const filteredVideoModelOptions = computed(() => {
  const keyword = videoModelSearch.value.trim().toLowerCase();
  if (!keyword) return videoModelOptions;
  return videoModelOptions.filter((model) => `${model.name} ${model.desc}`.toLowerCase().includes(keyword));
});
const filteredVideoToolOptions = computed(() => {
  const keyword = videoModelSearch.value.trim().toLowerCase();
  if (!keyword) return videoToolOptions;
  return videoToolOptions.filter((tool) => `${tool.name} ${tool.desc}`.toLowerCase().includes(keyword));
});
const availableVideoDurationOptions = computed(() => videoModelParameterOptions[selectedVideoModel.value]?.durations ?? videoDurationOptions);
const availableVideoRatioOptions = computed(() => videoModelParameterOptions[selectedVideoModel.value]?.ratios ?? videoRatioOptions);
const availableVideoResolutionOptions = computed(() => videoModelParameterOptions[selectedVideoModel.value]?.resolutions ?? videoResolutionOptions);

function syncVideoModelParameters() {
  const durations = availableVideoDurationOptions.value;
  const ratios = availableVideoRatioOptions.value;
  const resolutions = availableVideoResolutionOptions.value;
  if (durations.length && !durations.includes(videoDuration.value)) videoDuration.value = durations[0];
  if (ratios.length && !ratios.includes(videoRatio.value)) videoRatio.value = ratios[0];
  if (resolutions.length && !resolutions.includes(videoResolution.value)) videoResolution.value = resolutions[0];
}

function selectedVideoModelId() {
  return videoModelId(selectedVideoModel.value);
}

function normalizeVideoHistoryEntry(entry: Partial<VideoHistoryEntry> | null | undefined): VideoHistoryEntry | null {
  if (!entry) return null;
  const timestamp = normalizeVideoTimestamp(entry.createdAt || entry.timestamp);
  const id = String(entry.id || entry.taskId || entry.backendTaskId || `video-${timestamp}`).trim();
  if (!id) return null;
  return {
    id,
    taskId: entry.taskId ? String(entry.taskId) : undefined,
    backendTaskId: entry.backendTaskId ? String(entry.backendTaskId) : undefined,
    url: String(entry.url || ""),
    prompt: String(entry.prompt || ""),
    model: String(entry.model || selectedVideoModelId()),
    mode: entry.mode || "text-to-video",
    aspect_ratio: String(entry.aspect_ratio || videoRatio.value || ""),
    duration: entry.duration || videoDuration.value || "",
    resolution: String(entry.resolution || videoResolution.value || ""),
    inputImageUrls: Array.isArray(entry.inputImageUrls) ? entry.inputImageUrls.map(String).filter(Boolean) : [],
    inputVideoUrl: String(entry.inputVideoUrl || ""),
    createdAt: entry.createdAt || new Date(timestamp).toISOString(),
    timestamp,
    status: entry.status || (entry.url ? "success" : "generating"),
    errorMessage: entry.errorMessage ? String(entry.errorMessage) : "",
    userId: entry.userId ? String(entry.userId) : undefined
  };
}

function taskToVideoHistoryEntry(task: AdminRecord): VideoHistoryEntry | null {
  if (!isVideoGenerationTask(task)) return null;
  const params = videoTaskParams(task);
  const createdAt = String(task.createdAt || task.created_at || task.updatedAt || new Date().toISOString());
  const status = videoStatusFromTask(task);
  return normalizeVideoHistoryEntry({
    id: String(task.id || task.taskId || `video-${Date.now()}`),
    taskId: String(task.taskId || task.providerTaskId || task.id || ""),
    backendTaskId: String(task.id || ""),
    url: videoTaskUrl(task),
    prompt: String(task.prompt || params.prompt || ""),
    model: String(task.model || params.model || selectedVideoModelId()),
    mode: videoModeFromTask(task, params),
    aspect_ratio: videoStringValue(params.ratio ?? params.aspect_ratio ?? task.aspect_ratio, videoRatio.value),
    duration: videoNumberOrString(params.duration ?? params.seconds ?? task.duration, videoDuration.value),
    resolution: videoStringValue(params.resolution ?? task.resolution, videoResolution.value),
    inputImageUrls: videoInputImageUrlsFromTask(task, params),
    inputVideoUrl: videoStringValue(params.inputVideoUrl ?? params.video_url ?? params.videoUrl ?? task.inputVideoUrl),
    createdAt,
    status,
    errorMessage: videoErrorMessage(task.failureReason ?? task.errorMessage ?? task.error ?? task.failReason),
    userId: videoStringValue(task.userId)
  });
}

function sortVideoHistory(entries: VideoHistoryEntry[]) {
  return [...entries].sort((left, right) => right.timestamp - left.timestamp).slice(0, videoHistoryLimit);
}

function commitVideoHistoryEntry(entry: Partial<VideoHistoryEntry>, replaceId = "") {
  const normalized = normalizeVideoHistoryEntry(entry);
  if (!normalized) return;
  videoHistory.value = sortVideoHistory([
    normalized,
    ...videoHistory.value.filter((item) => item.id !== normalized.id && item.id !== replaceId && item.taskId !== normalized.taskId && item.backendTaskId !== normalized.backendTaskId)
  ]);
  selectedVideoHistoryId.value = normalized.id;
}

function mergeVideoHistoryEntries(entries: Array<VideoHistoryEntry | null>) {
  const map = new Map<string, VideoHistoryEntry>();
  videoHistory.value.forEach((entry) => {
    if (!videoHiddenHistoryIds.value.includes(entry.id)) map.set(entry.id, entry);
  });
  entries.filter(Boolean).forEach((entry) => {
    const normalized = normalizeVideoHistoryEntry(entry);
    if (!normalized || videoHiddenHistoryIds.value.includes(normalized.id)) return;
    map.set(normalized.id, { ...(map.get(normalized.id) || {}), ...normalized });
  });
  videoHistory.value = sortVideoHistory(Array.from(map.values()));
  if (!selectedVideoHistoryId.value && videoHistory.value.length) selectedVideoHistoryId.value = videoHistory.value[0].id;
}

function videoScopedStorageKey(baseKey: string) {
  const userKey = String(currentAdmin.value?.id || currentAdmin.value?.email || "anonymous").replace(/[^\w.@-]/g, "_");
  return `${baseKey}:${userKey}`;
}

function videoHistoryStorageKey() {
  return videoScopedStorageKey(videoHistoryStorageBaseKey);
}

function hydrateVideoInputDraftsFromStorage() {
  if (typeof window === "undefined") return;
  const promptDraft = window.localStorage.getItem(videoScopedStorageKey(videoPromptDraftBaseKey));
  const searchDraft = window.localStorage.getItem(videoScopedStorageKey(videoHistorySearchBaseKey));
  if (promptDraft !== null) videoPrompt.value = promptDraft;
  if (searchDraft !== null) videoHistorySearchQuery.value = searchDraft;
}

function hydrateVideoHistoryFromStorage() {
  if (typeof window === "undefined" || videoHistoryHydrated) return;
  videoHistoryHydrated = true;
  hydrateVideoInputDraftsFromStorage();
  try {
    const raw = window.localStorage.getItem(videoHistoryStorageKey()) || window.localStorage.getItem(videoHistoryStorageBaseKey);
    if (!raw) return;
    const parsed = JSON.parse(raw);
    const entries = Array.isArray(parsed) ? parsed : Array.isArray(parsed?.videoHistory) ? parsed.videoHistory : [];
    videoHiddenHistoryIds.value = Array.isArray(parsed?.hiddenIds) ? parsed.hiddenIds.map(String).filter(Boolean) : [];
    selectedVideoHistoryId.value = String(parsed?.selectedId || "");
    videoHistory.value = sortVideoHistory(entries.map(normalizeVideoHistoryEntry).filter(Boolean) as VideoHistoryEntry[]);
  } catch {
    videoHistory.value = [];
  }
}

function slimVideoHistoryUrl(value: string) {
  const url = String(value || "");
  if (url.startsWith("data:") || url.startsWith("blob:")) return "";
  return url;
}

function slimVideoHistoryEntry(entry: VideoHistoryEntry): VideoHistoryEntry {
  return {
    ...entry,
    inputImageUrls: (entry.inputImageUrls || []).map(slimVideoHistoryUrl).filter(Boolean).slice(0, 4),
    inputVideoUrl: slimVideoHistoryUrl(entry.inputVideoUrl)
  };
}

function scheduleVideoHistorySave() {
  if (!videoHistoryHydrated || typeof window === "undefined") return;
  if (videoHistorySaveTimer) window.clearTimeout(videoHistorySaveTimer);
  videoHistorySaveTimer = window.setTimeout(() => {
    const payload = {
      version: 1,
      videoHistory: videoHistory.value.map(slimVideoHistoryEntry),
      hiddenIds: videoHiddenHistoryIds.value,
      selectedId: selectedVideoHistoryId.value,
      updatedAt: new Date().toISOString()
    };
    window.localStorage.setItem(videoHistoryStorageKey(), JSON.stringify(payload));
  }, 400);
}

function scheduleVideoInputDraftSave() {
  if (!videoHistoryHydrated || typeof window === "undefined") return;
  if (videoInputDraftSaveTimer) window.clearTimeout(videoInputDraftSaveTimer);
  videoInputDraftSaveTimer = window.setTimeout(() => {
    window.localStorage.setItem(videoScopedStorageKey(videoPromptDraftBaseKey), videoPrompt.value);
    window.localStorage.setItem(videoScopedStorageKey(videoHistorySearchBaseKey), videoHistorySearchQuery.value);
  }, 400);
}

function adjustVideoPromptHeight() {
  const textarea = videoPromptTextareaRef.value;
  if (!textarea) return;
  const viewportMax = typeof window === "undefined" ? 220 : Math.min(Math.max(Math.floor(window.innerHeight * 0.4), 160), 240);
  textarea.style.height = "0px";
  const targetHeight = Math.min(textarea.scrollHeight, viewportMax);
  textarea.style.height = `${Math.max(targetHeight, 48)}px`;
  textarea.style.overflowY = textarea.scrollHeight > viewportMax ? "auto" : "hidden";
}

function handleVideoPromptInput() {
  void nextTick(adjustVideoPromptHeight);
}

const selectedVideoHistoryEntry = computed(() => {
  const matched = videoHistory.value.find((entry) => entry.id === selectedVideoHistoryId.value);
  return matched || videoHistory.value.find((entry) => entry.url && entry.status === "success") || videoHistory.value[0] || null;
});

const filteredVideoHistory = computed(() => {
  const keyword = videoHistorySearchQuery.value.trim().toLowerCase();
  const filter = videoHistoryFilter.value;
  const filtered = videoHistory.value.filter((entry) => {
    if (videoHiddenHistoryIds.value.includes(entry.id)) return false;
    if (filter !== "ALL" && entry.status !== filter && entry.mode !== filter) return false;
    if (!keyword) return true;
    return [
      entry.id,
      entry.taskId || "",
      entry.prompt,
      entry.model,
      entry.mode,
      entry.createdAt,
      entry.timestamp,
      entry.duration,
      entry.aspect_ratio,
      entry.resolution,
      entry.status
    ].join(" ").toLowerCase().includes(keyword);
  });
  return [...filtered].sort((left, right) => videoHistorySort.value === "asc" ? left.timestamp - right.timestamp : right.timestamp - left.timestamp);
});
const visibleVideoHistory = computed(() => filteredVideoHistory.value.slice(0, videoHistoryVisibleCount.value));
const hasMoreVideoHistory = computed(() => visibleVideoHistory.value.length < filteredVideoHistory.value.length);

function loadMoreVideoHistory() {
  videoHistoryVisibleCount.value = Math.min(filteredVideoHistory.value.length, videoHistoryVisibleCount.value + videoHistoryPageSize);
}

async function submitVideoGeneration() {
  openVideoDropdown.value = "";
  const prompt = videoPrompt.value.trim();
  if (!prompt) {
    ElMessage.error("请输入视频提示词");
    return false;
  }
  if (videoImageFile && videoImagePreview.value) videoStudioMode.value = "image";
  if (selectedVideoModel.value === "Grok Video 1.5" && !videoImageFile) {
    ElMessage.error("Grok Video 1.5 必须上传 1 张参考图");
    return false;
  }
  if (videoStudioMode.value === "image" && !videoImageFile) {
    ElMessage.error("请先上传 1 张参考图");
    return false;
  }
  if (!ensureWorkspaceAuth("generate_video", "userVideoGeneration")) return false;
  const clientRequestId = createGenerationClientRequestId("video");
  let videoReferenceImageUrl = "";
  const snapshotId = `video-local-${Date.now()}`;
  const snapshotCreatedAt = new Date().toISOString();
  const snapshotMode: VideoHistoryEntry["mode"] = videoStudioMode.value === "image" ? "image-to-video" : videoStudioMode.value === "video" ? "video-to-video" : "text-to-video";
  commitVideoHistoryEntry({
    id: snapshotId,
    url: "",
    prompt,
    model: selectedVideoModelId(),
    mode: snapshotMode,
    aspect_ratio: videoRatio.value,
    duration: videoDuration.value,
    resolution: videoResolution.value,
    inputImageUrls: videoImagePreview.value ? [videoImagePreview.value] : [],
    inputVideoUrl: videoSourcePreview.value,
    createdAt: snapshotCreatedAt,
    status: "generating",
    userId: String(currentAdmin.value?.id || "")
  });
  videoSubmitting.value = true;
  try {
    const type = videoStudioMode.value === "image" ? "IMAGE_TO_VIDEO" : "TEXT_TO_VIDEO";
    if (type === "IMAGE_TO_VIDEO") {
      videoReferenceImageUrl = await blobToDataUrl(videoImageFile as File);
    }
    const createdTask = await adminRequest<AdminRecord>({
      method: "POST",
      url: "/generation-tasks",
      timeout: 900000,
      data: {
        clientRequestId,
        type,
        prompt,
        model: selectedVideoModelId(),
        moduleCode: "video_generation",
        params: {
          module_code: "video_generation",
          duration: videoDuration.value,
          ratio: videoRatio.value,
          resolution: videoResolution.value,
          generate_audio: videoGenerateAudio.value,
          generateAudio: videoGenerateAudio.value,
          sourceModule: "video-generation",
          inputMode: videoStudioMode.value,
          hasInputImage: Boolean(videoImagePreview.value),
          hasInputVideo: Boolean(videoSourcePreview.value),
          ...(videoReferenceImageUrl ? {
            image_urls: [videoReferenceImageUrl],
            referenceImages: [{ name: videoImageFile?.name || "video-reference-image", url: videoReferenceImageUrl }]
          } : {})
        }
      }
    });
    const taskId = String(createdTask.id || "");
    const task = taskId
      ? await adminRequest<AdminRecord>({ method: "GET", url: `/generation-tasks/${encodeURIComponent(taskId)}` })
      : createdTask;
    videoResultTask.value = task;
    const historyEntry = taskToVideoHistoryEntry(task);
    if (historyEntry) {
      commitVideoHistoryEntry(historyEntry, snapshotId);
    }
    ElMessage.success(historyEntry?.status === "success" ? "视频生成成功" : "视频任务已提交，正在生成中");
    return true;
  } catch (error) {
    commitVideoHistoryEntry({
      id: snapshotId,
      url: "",
      prompt,
      model: selectedVideoModelId(),
      mode: snapshotMode,
      aspect_ratio: videoRatio.value,
      duration: videoDuration.value,
      resolution: videoResolution.value,
      inputImageUrls: videoImagePreview.value ? [videoImagePreview.value] : [],
      inputVideoUrl: videoSourcePreview.value,
      createdAt: snapshotCreatedAt,
      status: "failed",
      errorMessage: error instanceof Error ? error.message : "视频生成失败",
      userId: String(currentAdmin.value?.id || "")
    }, snapshotId);
    ElMessage.error(error instanceof Error ? error.message : "视频生成失败");
    return false;
  } finally {
    videoSubmitting.value = false;
  }
}

function videoModeLabel(mode: string) {
  if (mode === "image-to-video") return "图生视频";
  if (mode === "video-to-video") return "视频编辑";
  return "文生视频";
}

function videoStatusLabel(status: VideoHistoryStatus) {
  if (status === "success") return "已完成";
  if (status === "failed") return "失败";
  return "生成中";
}

function formatVideoHistoryTime(value: string) {
  const date = value ? new Date(value) : new Date();
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

function activateVideoHistoryPreview(entry: VideoHistoryEntry) {
  if (!entry.url || activeVideoHistoryPreviewIds.value.includes(entry.id)) return;
  activeVideoHistoryPreviewIds.value = [...activeVideoHistoryPreviewIds.value, entry.id].slice(-videoHistoryLimit);
}

function videoHistoryCardSrc(entry: VideoHistoryEntry) {
  if (!entry.url) return "";
  return selectedVideoHistoryId.value === entry.id || activeVideoHistoryPreviewIds.value.includes(entry.id) ? entry.url : "";
}

function playVideoCardPreview(event: MouseEvent, entry: VideoHistoryEntry) {
  activateVideoHistoryPreview(entry);
  void nextTick(() => {
    const video = (event.currentTarget as HTMLElement | null)?.querySelector("video");
    if (!video) return;
    void video.play().catch(() => undefined);
  });
}

function resetVideoCardPreview(event: MouseEvent) {
  const video = (event.currentTarget as HTMLElement | null)?.querySelector("video");
  if (!video) return;
  video.pause();
  try {
    video.currentTime = 0;
  } catch {
    // Some remote streams do not allow seeking until metadata is ready.
  }
}

function openVideoFullscreen(entry: VideoHistoryEntry) {
  if (!entry.url || entry.status !== "success") {
    selectedVideoHistoryId.value = entry.id;
    return;
  }
  selectedVideoHistoryId.value = entry.id;
  videoFullscreenEntry.value = entry;
}

function closeVideoFullscreen() {
  videoFullscreenEntry.value = null;
}

type VideoHistoryAction = "download" | "copy" | "delete";

function videoHistoryBusyKey(id: string, action: VideoHistoryAction) {
  return `${id}:${action}`;
}

function isVideoHistoryActionBusy(id: string, action: VideoHistoryAction) {
  return Boolean(videoHistoryBusyActions.value[videoHistoryBusyKey(id, action)]);
}

function setVideoHistoryActionBusy(id: string, action: VideoHistoryAction, busy: boolean) {
  const key = videoHistoryBusyKey(id, action);
  if (busy) {
    videoHistoryBusyActions.value = { ...videoHistoryBusyActions.value, [key]: true };
    return;
  }
  const next = { ...videoHistoryBusyActions.value };
  delete next[key];
  videoHistoryBusyActions.value = next;
}

function safeVideoFileName(entry: VideoHistoryEntry) {
  const prompt = entry.prompt.trim().slice(0, 28).replace(/[\\/:*?"<>|]+/g, "").replace(/\s+/g, "-");
  const base = prompt || "video";
  return `${base}-${entry.id || Date.now()}.mp4`;
}

async function downloadVideoFile(url: string, fileName: string) {
  const proxyURL = `/api/v1/video/download?url=${encodeURIComponent(url)}&filename=${encodeURIComponent(fileName)}`;
  await downloadUrl(proxyURL, fileName);
  return "downloaded" as const;
}

async function downloadVideoHistory(entry: VideoHistoryEntry) {
  if (!entry.url) {
    ElMessage.warning("视频地址不存在，无法下载");
    return;
  }
  if (entry.status !== "success") {
    ElMessage.warning("视频尚未生成成功，暂不能下载");
    return;
  }
  if (!ensureWorkspaceAuth("download_work", "userVideoGeneration", { mediaKind: "video", taskId: entry.id })) return;
  if (isVideoHistoryActionBusy(entry.id, "download")) return;
  setVideoHistoryActionBusy(entry.id, "download", true);
  ElMessage.info("开始下载视频");
  try {
    const result = await downloadVideoFile(entry.url, safeVideoFileName(entry));
    if (result === "downloaded") {
      ElMessage.success("视频下载已开始");
    } else {
      ElMessage.warning("下载失败，已在新窗口打开视频");
    }
  } catch {
    ElMessage.error("视频下载失败");
  } finally {
    setVideoHistoryActionBusy(entry.id, "download", false);
  }
}

function copyTextWithFallback(text: string) {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text);
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  try {
    const ok = document.execCommand("copy");
    if (!ok) throw new Error("execCommand copy failed");
    return Promise.resolve();
  } catch (error) {
    return Promise.reject(error);
  } finally {
    textarea.remove();
  }
}

async function copyVideoHistoryUrl(entry: VideoHistoryEntry) {
  if (!entry.url) {
    ElMessage.warning("视频链接不存在，无法复制");
    return;
  }
  if (entry.status !== "success") {
    ElMessage.warning("视频尚未生成成功，暂不能复制链接");
    return;
  }
  if (isVideoHistoryActionBusy(entry.id, "copy")) return;
  setVideoHistoryActionBusy(entry.id, "copy", true);
  try {
    await copyTextWithFallback(entry.url);
    ElMessage.success("视频链接已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  } finally {
    setVideoHistoryActionBusy(entry.id, "copy", false);
  }
}

async function deleteVideoHistoryEntry(entry: VideoHistoryEntry) {
  if (isVideoHistoryActionBusy(entry.id, "delete")) return;
  try {
    await ElMessageBox.confirm("删除后将从历史列表移除，但不会影响已经下载到本地的视频。", "确认删除这个视频记录？", {
      confirmButtonText: "确认删除",
      cancelButtonText: "取消",
      type: "warning",
      customClass: "video-delete-confirm"
    });
  } catch {
    return;
  }
  setVideoHistoryActionBusy(entry.id, "delete", true);
  try {
    videoHiddenHistoryIds.value = Array.from(new Set([...videoHiddenHistoryIds.value, entry.id]));
    videoHistory.value = videoHistory.value.filter((item) => item.id !== entry.id);
    if (selectedVideoHistoryId.value === entry.id) selectedVideoHistoryId.value = videoHistory.value[0]?.id || "";
    if (videoFullscreenEntry.value?.id === entry.id) videoFullscreenEntry.value = null;
    ElMessage.success("已删除视频记录");
  } catch {
    ElMessage.error("删除失败，请稍后重试");
  } finally {
    setVideoHistoryActionBusy(entry.id, "delete", false);
  }
}

function handleVideoImageFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  clearVideoImageUpload();
  videoStudioMode.value = "image";
  videoImageFile = file;
  videoImageObjectUrl = URL.createObjectURL(file);
  videoImagePreview.value = videoImageObjectUrl;
}

function handleVideoSourceFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  clearVideoSourceUpload();
  videoStudioMode.value = "video";
  videoSourceObjectUrl = URL.createObjectURL(file);
  videoSourcePreview.value = videoSourceObjectUrl;
}

function clearVideoImageUpload() {
  if (videoImageObjectUrl) URL.revokeObjectURL(videoImageObjectUrl);
  videoImageObjectUrl = "";
  videoImageFile = null;
  videoImagePreview.value = "";
  if (videoStudioMode.value === "image") videoStudioMode.value = videoSourcePreview.value ? "video" : "text";
}

function clearVideoSourceUpload() {
  if (videoSourceObjectUrl) URL.revokeObjectURL(videoSourceObjectUrl);
  videoSourceObjectUrl = "";
  videoSourcePreview.value = "";
  if (videoStudioMode.value === "video") videoStudioMode.value = videoImagePreview.value ? "image" : "text";
}

function toggleVideoDropdown(type: "model" | "ratio" | "duration" | "resolution") {
  openVideoDropdown.value = openVideoDropdown.value === type ? "" : type;
}

function selectVideoOption(type: "model" | "ratio" | "duration" | "resolution", value: string | number) {
  if (type === "model") {
    selectedVideoModel.value = String(value);
    videoStudioMode.value = videoImagePreview.value ? "image" : videoToolOptions.some((tool) => tool.name === value) ? "video" : "text";
    syncVideoModelParameters();
    openVideoDropdown.value = "";
    return;
  }
  if (type === "ratio") {
    videoRatio.value = String(value);
    openVideoDropdown.value = "";
    return;
  }
  if (type === "duration") {
    videoDuration.value = Number(value);
    openVideoDropdown.value = "";
    return;
  }
  videoResolution.value = String(value);
  openVideoDropdown.value = "";
}

const adminNavigationIcons: Record<AdminNavigationIconKey, Component> = {
  overview: House,
  customers: User,
  commercial: Tickets,
  ai: Cpu,
  growth: Connection,
  platform: Setting
};

const adminModuleGroups = adminNavigationGroups.map((group) => ({
  id: group.id,
  title: group.title,
  icon: adminNavigationIcons[group.icon],
  items: group.sections.flatMap((section) => {
    const module = adminModuleById(section.primaryModuleId);
    return module ? [{ ...module, title: section.title, requiresEnterpriseManagement: section.requiresEnterpriseManagement === true }] : [];
  })
}));

const agentModuleGroups = [
  { id: "agentHome", title: "代理商后台", icon: Wallet, items: modules.filter((item) => ["partnerDashboard"].includes(item.id)) },
  { id: "agentBusiness", title: "业务管理", icon: Collection, items: modules.filter((item) => ["partnerCustomers", "partnerOrders", "partnerUsage", "partnerCommissions"].includes(item.id)) },
  { id: "agentGrowth", title: "推广增长", icon: Connection, items: modules.filter((item) => ["partnerChannels", "partnerMaterials"].includes(item.id)) },
  { id: "agentAccount", title: "账户中心", icon: Setting, items: modules.filter((item) => ["partnerAccount"].includes(item.id)) }
];
const userModuleGroups = [
  { id: "userHome", title: "用户后台", icon: House, items: modules.filter((item) => ["userDashboard"].includes(item.id)) },
  { id: "userCreation", title: "创作中心", icon: Collection, items: modules.filter((item) => ["userAiImage", "userAgentCenter", "userWirelessCanvas", "userVideoGeneration", "userPptGeneration", "userWorks"].includes(item.id)) },
  { id: "userOperationCenter", title: "运营中心", icon: Connection, items: modules.filter((item) => operationCenterModuleIds.includes(item.id)) },
  { id: "userBilling", title: "身份/充值/订阅", icon: Wallet, items: modules.filter((item) => ["userMembership"].includes(item.id)) },
  { id: "userData", title: "数据记录", icon: DataAnalysis, items: modules.filter((item) => ["userUsage", "userOrders"].includes(item.id)) }
];

const userFlatMenuDefs = [
  { id: "userDashboard", targetId: "userDashboard", title: "平台首页", icon: House },
  { id: "userAiImage", targetId: "userAiImage", title: "AI 生图", icon: Plus },
  { id: "userVideoGeneration", targetId: "userVideoGeneration", title: "视频生成", icon: Monitor },
  { id: "userWirelessCanvas", targetId: "userWirelessCanvas", title: "无限画布", icon: Collection },
  { id: "userPptGeneration", targetId: "userPptGeneration", title: "PPT 文档生成", icon: Document },
  { id: "userAgentCenter", targetId: "userAgentCenter", title: "智能体中心", icon: Cpu },
  { id: "userWorks", targetId: "userWorks", title: "作品中心", icon: Collection },
  { id: "userMembership", targetId: "userMembership", title: "身份充值订阅", icon: Tickets },
  { id: "userUsage", targetId: "userUsage", title: "使用记录", icon: DataAnalysis },
  { id: "userOrders", targetId: "userOrders", title: "订单明细", icon: Document }
];

const userAgentFlatMenuDefs = [
  { id: "partnerDashboard", targetId: "partnerDashboard", title: "代理首页", icon: Wallet },
  { id: "partnerCustomers", targetId: "partnerCustomers", title: "我的客户", icon: User },
  { id: "partnerOrders", targetId: "partnerOrders", title: "推广订单", icon: Money },
  { id: "partnerUsage", targetId: "partnerUsage", title: "客户消费", icon: DataAnalysis },
  { id: "partnerCommissions", targetId: "partnerCommissions", title: "佣金明细", icon: Wallet },
  { id: "partnerChannels", targetId: "partnerChannels", title: "推广渠道", icon: Connection },
  { id: "partnerMaterials", targetId: "partnerMaterials", title: "推广素材", icon: Collection },
  { id: "partnerAccount", targetId: "partnerAccount", title: "代理账户", icon: Setting }
];

const userOperationCenterFlatMenuDefs = [
  { id: "operationCenterDashboard", targetId: "operationCenterDashboard", title: "运营首页", icon: DataAnalysis },
  { id: "operationCenterAgents", targetId: "operationCenterAgents", title: "中心代理", icon: Connection },
  { id: "operationCenterOrders", targetId: "operationCenterOrders", title: "中心订单", icon: Money },
  { id: "operationCenterCommissions", targetId: "operationCenterCommissions", title: "中心分润", icon: Wallet }
];

const allUserFlatMenuDefs = [...userFlatMenuDefs, ...userAgentFlatMenuDefs, ...userOperationCenterFlatMenuDefs];

const userFlatMenuItems = computed(() => {
  const source = [
    ...userFlatMenuDefs,
    ...(hasAgentIdentity.value ? userAgentFlatMenuDefs : []),
    ...(hasOperationCenterIdentity.value ? userOperationCenterFlatMenuDefs : [])
  ];
  return source.filter((item) => modules.some((module) => module.id === item.targetId));
});

const pageMeta: Record<string, { badge: string; description: string }> = {
  userDashboard: { badge: "用户工作台", description: "聚合点数、生成、作品、API 和使用记录，作为用户登录后的 PC 中后台首页。" },
  userAiImage: { badge: "智能生成", description: "聚合 Prompt、参考图、模型参数和结果预览，快速生成商业素材。" },
  userAgentCenter: { badge: "智能体中心", description: "统一管理智能体模板、实例、调用数据、知识库绑定和发布入口。" },
  userWirelessCanvas: { badge: "源码复刻", description: "直接承载 hero8152/Infinite-Canvas 智能画布源码，保留节点、资产库、工作流、快捷键、日志和底部生成器布局。" },
  userVideoGeneration: { badge: "视频生成", description: "参考 Open-Generative-AI Video Studio，提供文生视频、图生视频和基础参数的轻量生成入口。" },
  userPptGeneration: { badge: "PPT文档生成", description: "输入主题生成演示文稿，预留 Presenton 或自研 PPT 生成服务接入。" },
  userWorks: { badge: "作品中心", description: "集中查看生成资产、缩略图、下载和编辑入口。" },
  userUsage: { badge: "使用记录", description: "按任务、模型、扣点和余额变化查看正式使用流水，和主控计费口径保持一致。" },
  userMembership: { badge: "身份/充值/订阅", description: "先开通会员、代理商或运营中心身份，再完成点数充值、套餐订阅和支付方式选择。" },
  userOrders: { badge: "交易记录", description: "查看历史订单、支付状态和点数到账结果；充值/订阅入口已拆分为独立模块。" },
  knowledgeAdmin: { badge: "知识库治理", description: "跨租户管理知识库、文档解析、Chunk、Embedding、向量索引、检索日志与问答统计。" },
  mediaAssets: { badge: "素材中心", description: "统一上传、分类、预览、启停并追踪运营素材的页面引用。" },
  inspirationManagement: { badge: "内容运营", description: "管理创作案例、提示词、模型参数、审核发布和同款转化数据。" },
  mediaCategories: { badge: "素材分类", description: "维护平台默认与租户专属的运营素材分类。" },
  pageDecoration: { badge: "页面装修", description: "可视化配置首页、创作、作品和我的页面素材位并发布版本。" },
  pageHomeConfig: { badge: "首页配置", description: "配置首页 Hero、快捷入口、能力卡和灵感推荐素材。" },
  pageStudioConfig: { badge: "创作页配置", description: "配置创作页 Banner 与模板封面。" },
  pageAssetsConfig: { badge: "作品页配置", description: "配置作品类型默认封面和失败回退。" },
  pageProfileConfig: { badge: "我的页配置", description: "配置用户头像、会员背景与个人中心头图。" },
  operationCenterDashboard: { badge: "运营中心", description: "汇总运营中心代理、订单、分润和邀请码，作为中心账号登录后的经营首页。" },
  operationCenterAgents: { badge: "中心代理", description: "查看归属本运营中心的代理商、等级、邀请码和启停状态。" },
  operationCenterOrders: { badge: "中心订单", description: "查看归属本运营中心的代理开通、会员充值和套餐订单。" },
  operationCenterCommissions: { badge: "中心分润", description: "核对运营中心奖励、待结算金额、已结算金额和分润记录。" },
  enterpriseList: { badge: "企业管理", description: "平台企业客户、认证、套餐、席位、算力与归属的统一管理入口。" },
  enterpriseDetail: { badge: "企业详情", description: "查看企业治理、计费和使用摘要，不返回企业敏感内容正文。" },
  enterpriseCertifications: { badge: "认证审核", description: "审核企业主体资质，并保留通过、驳回及审批审计记录。" },
  enterpriseMembers: { badge: "成员组织", description: "查看企业成员、席位与组织架构统计。" },
  enterprisePackage: { badge: "套餐席位", description: "查看和调整企业套餐、有效期与成员席位。" },
  enterpriseCompute: { badge: "算力账户", description: "按服务端 POINT 单位查看和调整企业算力。" },
  enterpriseTransactions: { badge: "资金流水", description: "查看企业充值、消费和余额变化明细。" },
  enterpriseOrders: { badge: "企业订单", description: "查看企业套餐、算力与服务订单。" },
  enterpriseAiCapabilities: { badge: "AI 能力", description: "查看和配置企业已开通模型与 AI 能力。" },
  enterpriseAiEmployees: { badge: "AI 员工", description: "仅查看企业 AI 员工数量和运行状态统计。" },
  enterpriseKnowledgeBases: { badge: "知识库概览", description: "仅查看知识库数量、容量和使用统计，不返回正文。" },
  enterpriseAttribution: { badge: "客户归属", description: "查看并通过审批流程变更客户归属。" },
  enterpriseRelationships: { badge: "渠道关系", description: "查看企业来源代理商与所属运营中心关系。" },
  enterpriseRisk: { badge: "企业风控", description: "查看风险记录并暂停或恢复企业服务。" },
  enterpriseAuditLogs: { badge: "审计日志", description: "查看主控管理员对企业数据执行的全部写操作。" },
  analysis: { badge: "数据分析", description: "按客户增长、交易收入、积分消耗和生成任务活跃度观察平台经营状态。" },
  workbench: { badge: "运营工作台", description: "聚合待办、快捷入口和平台健康状态，帮助主控团队快速处理日常运营动作。" },
  dashboard: { badge: "运营驾驶舱", description: "汇总客户、订单、渠道、用量和上游服务状态，帮助主控端快速判断平台健康度。" },
  customers: { badge: "客户资产", description: "管理客户账号、套餐、点数、状态和角色，支撑 SaaS 客户全生命周期运营。" },
  customerAttributions: { badge: "归属治理", description: "统一核对普通客户和企业客户对应的直接代理、上级代理与运营中心关系。" },
  channels: { badge: "渠道网络", description: "维护 L1-L5 代理商、邀请码和启停状态，承接代理分销体系。" },
  operationCenters: { badge: "运营中心", description: "查看运营中心身份、代理归属、开通订单和区域分润汇总。" },
  products: { badge: "产品矩阵", description: "配置 AI 产品能力、权益和状态，让移动端与管理后台共享统一商品口径。" },
  plans: { badge: "商业套餐", description: "维护套餐价格、赠送点数、并发和有效期，作为订单与权益结算基础。" },
  orders: { badge: "交易履约", description: "跟踪订单、收款、续费和权益发放，关键动作由后端事务保证一致性。" },
  tokenRecords: { badge: "Token 流水", description: "审计会员套餐、代理开通和充值产生的 Token/点数发放记录。" },
  marketingDashboard: { badge: "营销闭环", description: "对照运营中心营销端 P0 范围，聚合邀请、升级、分佣、钱包和权限风险。" },
  marketingAgentLevels: { badge: "代理等级", description: "维护 L0-L5 身份、开通方式、权限边界、返佣建议和升级保级条件。" },
  marketingInvites: { badge: "邀请转化", description: "追踪邀请人、被邀请人、邀请码、注册、充值和升级状态，支撑扫码裂变。" },
  marketingCommissionRules: { badge: "分佣规则", description: "沉淀直推、间推、运营中心奖励、算力充值分成等可审计规则。" },
  marketingUpgradePlans: { badge: "升级路径", description: "维护普通用户、代理和运营中心之间的升级价格、条件和周期口径。" },
  marketingWallets: { badge: "佣金钱包", description: "按代理汇总佣金收入、提现冻结、可提现余额和邀请入口。" },
  marketingWalletRecords: { badge: "渠道佣金钱包流水", description: "按佣金入账和提现冻结展示每笔资金变化，帮助财务核对来源和状态。" },
  marketingSettlementStatements: { badge: "月度结算", description: "按代理和月份聚合已结算佣金、提现和待结算金额，形成打款前核对口径。" },
  usage: { badge: "消耗分析", description: "查看模型、素材、生成任务等使用量，为成本和定价提供依据。" },
  billingOverview: { badge: "计费总览", description: "统一查看正式价格、供应商成本、任务对账异常和钱包流水。" },
  billingRules: { badge: "价格版本", description: "查看模型计费规则；修改时创建草稿，校验后发布，不覆盖正式版本。" },
  billingProviderCosts: { badge: "成本口径", description: "独立维护供应商和通道成本，与用户售价分开存储和计算。" },
  billingEvents: { badge: "计费事件", description: "查看报价、冻结、确认、解冻和退款事件与幂等键。" },
  billingReconciliation: { badge: "任务对账", description: "按任务核对报价、钱包、计费事件和供应商成本，识别八类异常。" },
  billingWalletLedger: { badge: "用户积分钱包流水", description: "追踪充值、赠送、冻结、确认、解冻、退款、调整与过期流水。" },
  billingCustomers: { badge: "客户计费", description: "聚合真实订单、订阅、付款和钱包余额，形成客户商业账务档案。" },
  billingProducts: { badge: "支付商品", description: "统一查看服务端权益套餐与微信虚拟支付商品的映射关系。" },
  billingSubscriptions: { badge: "订阅实例", description: "由真实支付订单和会员权益生成订阅实例，保留来源订单与有效期。" },
  billingCoupons: { badge: "权益优惠券", description: "优惠券只加赠服务端权益，不改变微信虚拟支付商品金额。" },
  billingInvoices: { badge: "发票与账单", description: "真实订单自动形成交易账单，税票采用人工申请、开具和驳回状态。" },
  billingCreditNotes: { badge: "贷项红冲", description: "退款通知自动生成贷项，人工贷项需审核且不直接触发退款。" },
  billingPaymentRequests: { badge: "付款与催收", description: "付款请求跟随订单状态，催收仅记录人工动作，不自动外发消息。" },
  billingPayments: { badge: "支付记录", description: "追踪微信签单、回调、交易号、失败原因以及账单关联。" },
  commissions: { badge: "结算中心", description: "处理代理分润、提现审核和结算状态，形成渠道财务闭环。" },
  commissionRecords: { badge: "分润明细", description: "按接收方拆解代理、运营中心与平台收入，核对固定分润规则结果。" },
  aiCapabilities: { badge: "AI 能力", description: "配置生图、视频、PPT 等 AI 能力模块的启停、开放套餐、默认 Schema 和模型绑定。" },
  aiCapabilityModels: { badge: "AI 模型", description: "维护模型类型、上游供应商、能力编码、所属模块、fallback 和启停状态。" },
  aiCapabilitySchemas: { badge: "参数 Schema", description: "定义各模型支持的参数字段、类型、默认值、选项、可见性和用户可编辑边界。" },
  aiCapabilityLimits: { badge: "租户限制", description: "按租户、套餐、代理限制可用模型和参数范围，形成最终用户可用能力边界。" },
  aiCapabilityChannels: { badge: "上游通道", description: "查看 AI 能力关联的上游通道、协议、Base URL、模型列表和 Key 配置状态。" },
  aiCapabilityLogs: { badge: "调用日志", description: "审计 AI 能力调用记录、参数快照、限制快照、上游成本、平台利润和任务状态。" },
  apiSettings: { badge: "模型网关", description: "管理上游模型渠道、API Key、客户分组和计费倍率。" },
  system: { badge: "系统治理", description: "沉淀品牌、密钥、模型、审计和运维配置，避免继续依赖临时聚合结构。" },
  departments: { badge: "组织权限", description: "维护主控后台部门结构，为账号权限和操作审计提供组织归属。" },
  userManagement: { badge: "账号权限", description: "管理平台账号、角色、状态和联系方式，对齐 RBAC 权限治理入口。" },
  menuManagement: { badge: "菜单权限", description: "维护后台菜单入口、模块可见性和权限点，支撑主控 SaaS 权限配置。" },
  partnerDashboard: { badge: "代理经营", description: "汇总代理商客户、订单、佣金、推广渠道和转化状态，形成经营驾驶舱。" },
  partnerCustomers: { badge: "客户资产", description: "查看通过邀请码绑定的客户、状态和来源，辅助跟进转化。" },
  partnerOrders: { badge: "订单跟进", description: "聚合代理商名下待支付、已成交和待续费订单，提升回款效率。" },
  partnerUsage: { badge: "消费明细", description: "按客户、任务、模型和扣点查看生图消费，和主控计量事件保持同一笔任务 ID。" },
  partnerCommissions: { badge: "佣金结算", description: "查看分佣明细、结算状态、可提现金额和提现记录。" },
  partnerChannels: { badge: "推广渠道", description: "管理邀请码、下级代理和渠道转化表现。" },
  partnerMaterials: { badge: "素材中心", description: "沉淀推广海报、话术和专属链接，提升获客转化。" },
  partnerAccount: { badge: "账户设置", description: "维护代理商资料、收款信息和通知偏好。" }
};

const quickTodos = [
  { action: "marketing", module: "marketingDashboard", title: "打开营销端", desc: "查看邀请、升级、分佣和钱包闭环" },
  { action: "customer", module: "customers", title: "检查客户余额", desc: "处理套餐、点数和账号状态" },
  { action: "channel", module: "channels", title: "维护代理网络", desc: "新增代理商或调整启停状态" },
  { action: "api", module: "apiSettings", title: "校验上游模型", desc: "确认 API Key、Base URL 和模型可用" },
  { action: "order", module: "orders", title: "跟进待支付订单", desc: "标记收款并触发权益发放" }
];

const analysisStats = computed(() => [
  { label: "新增客户", value: metricValue("客户", "1,248"), icon: User },
  { label: "待处理任务", value: metricValue("任务", "326"), icon: Monitor },
  { label: "成交金额", value: metricValue("收入", "99,000"), icon: Money },
  { label: "生成总量", value: metricValue("用量", "13,600"), icon: DataAnalysis }
]);

const trafficSources = [
  { label: "直接访问", value: 31, color: "#5b76d6" },
  { label: "套餐营销", value: 10, color: "#8ac873" },
  { label: "代理推广", value: 12, color: "#f7c958" },
  { label: "视频投放", value: 6, color: "#ef6a6a" },
  { label: "搜索引擎", value: 41, color: "#6dbbd5" }
];

const weeklyActivity = [
  { day: "周一", height: 38 },
  { day: "周二", height: 88 },
  { day: "周三", height: 68 },
  { day: "周四", height: 42 },
  { day: "周五", height: 76 },
  { day: "周六", height: 18 },
  { day: "周日", height: 24 }
];

const workbenchTasks = [
  { module: "marketingDashboard", title: "打开营销端", desc: "查看邀请、升级、分佣和钱包闭环" },
  { module: "customers", title: "检查客户余额", desc: "处理套餐、点数和账号状态" },
  { module: "channels", title: "维护代理网络", desc: "新增代理商或调整启停状态" },
  { module: "apiSettings", title: "校验上游模型", desc: "确认 API Key、Base URL 和模型可用" },
  { module: "orders", title: "跟进待支付订单", desc: "标记收款并触发权益发放" }
];

const trafficDonutStyle = computed(() => {
  let cursor = 0;
  const stops = trafficSources.map((source) => {
    const start = cursor;
    cursor += source.value;
    return `${source.color} ${start}% ${cursor}%`;
  });
  return { background: `conic-gradient(${stops.join(", ")})` };
});


const onlineImageForm = ref({
  prompt: "",
  model: "gpt-image-2",
  provider: "",
  ratio: "square",
  size: "auto",
  quality: "auto",
  outputFormat: "png",
  transparentOutput: false,
  outputCompression: null as number | null,
  moderation: "auto",
  count: 1,
  resolution: "1k",
  width: 1024,
  height: 1024
});
type UserHomeCreationMode = "image" | "video" | "ppt" | "agent";
type UserHomeEntry = {
  title: string;
  desc: string;
  icon?: Component;
  targetId: string;
  mode?: UserHomeCreationMode;
  prompt?: string;
  coverClass?: string;
  coverText?: string;
};
const userHomeCreationMode = ref<UserHomeCreationMode>("image");
const userHomeCreationModes: Array<{ id: UserHomeCreationMode; title: string; icon: Component }> = [
  { id: "image", title: "图片创作", icon: Collection },
  { id: "video", title: "视频创作", icon: Monitor },
  { id: "ppt", title: "PPT 文档", icon: Document },
  { id: "agent", title: "Agent 对话", icon: Cpu }
];
const userHomeRatioOptions = [
  { label: "1:1", value: "square" },
  { label: "16:9", value: "16:9" },
  { label: "9:16", value: "9:16" },
  { label: "自定义", value: "custom" }
];
const userHomeAgentEntries: UserHomeEntry[] = [
  { title: "品牌 Agent", desc: "品牌视觉、VI、海报", icon: Star, targetId: "userAiImage", mode: "agent", prompt: "为新品牌设计一组高级视觉方向，包含 Logo 延展、主视觉和社媒海报。" },
  { title: "电商 Agent", desc: "主图、详情、促销图", icon: Goods, targetId: "userAiImage", mode: "agent", prompt: "生成一套电商营销素材，包含产品主图、详情长图和促销海报。" },
  { title: "海报 Agent", desc: "活动、节日、招商", icon: Collection, targetId: "userAiImage", mode: "agent", prompt: "设计一张高转化活动海报，突出标题、卖点和橙色 CTA。" },
  { title: "IP / 角色", desc: "角色设定、形象定制", icon: User, targetId: "userAiImage", mode: "image", prompt: "创建一个适合品牌传播的 IP 角色，风格年轻、干净、有记忆点。" },
  { title: "视频 Agent", desc: "短片、封面、分镜", icon: Monitor, targetId: "userVideoGeneration", mode: "video", prompt: "策划一条 15 秒产品短视频，包含镜头节奏、画面风格和字幕建议。" },
  { title: "PPT Agent", desc: "提纲、页面、配图", icon: Document, targetId: "userPptGeneration", mode: "ppt", prompt: "生成一份商业计划书 PPT，包含封面、目录、市场、产品和财务页。" },
  { title: "商品图 Agent", desc: "白底图、场景图", icon: Goods, targetId: "userAiImage", mode: "image", prompt: "为产品生成一组干净白底主图和浅色高级场景图。" },
  { title: "朋友圈海报 Agent", desc: "日签、营销海报", icon: Tickets, targetId: "userAiImage", mode: "image", prompt: "生成一张适合朋友圈发布的轻营销海报，画面真实、留白干净。" }
];
const agentCenterWorkspace = ref<AgentCenterWorkspace | null>(null);
const agentWorkspaceDraft = ref("");
const agentWorkspaceMessages = ref<AgentWorkspaceMessage[]>([]);
type AgentCenterListTab = "mine" | "recent" | "favorite";
const agentCenterTabs: Array<{ label: string; value: AgentCenterListTab }> = [
  { label: "我的智能体", value: "mine" },
  { label: "最近使用", value: "recent" },
  { label: "收藏", value: "favorite" }
];
const agentCenterListTab = ref<AgentCenterListTab>("mine");
const agentCenterSearch = ref("");
const agentCenterTypeFilter = ref("all");
const agentCenterRange = ref("7");
const agentCenterFavoriteKeys = ref<string[]>([]);
const agentCenterTypeOptions = computed(() => [...new Set(agentCenterRows.map((item) => String(item.type || "")).filter(Boolean))]);
const visibleAgentCenterRows = computed(() => {
  const keyword = agentCenterSearch.value.trim().toLowerCase();
  let items = agentCenterRows.filter((item) => {
    const matchesType = agentCenterTypeFilter.value === "all" || item.type === agentCenterTypeFilter.value;
    const matchesKeyword = !keyword || [item.name, item.desc, item.type, item.model, item.knowledge].some((value) => String(value || "").toLowerCase().includes(keyword));
    return matchesType && matchesKeyword;
  });
  if (agentCenterListTab.value === "favorite") items = items.filter(isAgentCenterFavorite);
  if (agentCenterListTab.value === "recent") items = items.slice(0, 5);
  return items;
});
const agentCenterSubviewHistoryKey = "__xianzhiAgentSubview";
const {
  officeCLIFormatOptions,
  officeCLIStatus,
  officeCLIStatusLoading,
  officeCLIForm,
  officeCLIWorkspaceOpen,
  officeCLIDocumentGenerating,
  officeCLIDocumentResult,
  officeCLIStatusLabel,
  officeCLIStatusTone,
  officeCLIDocumentSizeText,
  loadOfficeCLIStatus,
  submitOfficeCLIDocument,
  downloadOfficeCLIDocument
} = useOfficeCLI({ hasAuthToken, downloadUrl });

function scrollAgentCenterTop() {
  if (typeof window !== "undefined") {
    void nextTick(() => window.scrollTo({ top: 0, behavior: "smooth" }));
  }
}

function agentCenterSubviewHistoryState() {
  if (typeof window === "undefined") return "";
  const state = window.history.state as Record<string, unknown> | null;
  return typeof state?.[agentCenterSubviewHistoryKey] === "string" ? String(state[agentCenterSubviewHistoryKey]) : "";
}

function pushAgentCenterSubviewHistory(agentKey: string) {
  if (typeof window === "undefined" || store.activeModuleId !== "userAgentCenter") return;
  if (agentCenterSubviewHistoryState() === agentKey) return;
  const baseState = { ...(window.history.state || {}) };
  delete baseState[agentCenterSubviewHistoryKey];
  window.history.replaceState(baseState, "", window.location.href);
  window.history.pushState({ ...baseState, [agentCenterSubviewHistoryKey]: agentKey }, "", window.location.href);
}

function replaceAgentCenterSubviewHistoryWithOverview() {
  if (typeof window === "undefined") return;
  const nextState = { ...(window.history.state || {}) };
  delete nextState[agentCenterSubviewHistoryKey];
  window.history.replaceState(nextState, "", window.location.href);
}

function clearAgentCenterSubview(updateHistory = true) {
  if (updateHistory) replaceAgentCenterSubviewHistoryWithOverview();
  officeCLIWorkspaceOpen.value = false;
  agentCenterWorkspace.value = null;
  agentWorkspaceDraft.value = "";
  agentWorkspaceMessages.value = [];
  scrollAgentCenterTop();
}

function closeAgentCenterSubview() {
  clearAgentCenterSubview();
}

function openOfficeCLIWorkspace(pushHistory = true) {
  agentCenterWorkspace.value = null;
  officeCLIWorkspaceOpen.value = true;
  if (!officeCLIStatus.value && !officeCLIStatusLoading.value) void loadOfficeCLIStatus();
  if (pushHistory) pushAgentCenterSubviewHistory("officecli");
  scrollAgentCenterTop();
}

function closeOfficeCLIWorkspace() {
  closeAgentCenterSubview();
}

function openAgentWorkspace(item: AgentCenterOpenable, pushHistory = true) {
  if (isOfficeCLIItem(item)) {
    openOfficeCLIWorkspace(pushHistory);
    return;
  }
  officeCLIWorkspaceOpen.value = false;
  const workspace = buildAgentCenterWorkspace(item);
  agentCenterWorkspace.value = workspace;
  agentWorkspaceDraft.value = workspace.prompt;
  agentWorkspaceMessages.value = workspace.sampleMessages.map((message) => ({ ...message }));
  if (pushHistory) pushAgentCenterSubviewHistory(workspace.agentKey);
  scrollAgentCenterTop();
}

function closeAgentWorkspace() {
  closeAgentCenterSubview();
}

function restoreAgentCenterSubviewFromHistory(agentKey: string) {
  if (!agentKey) {
    clearAgentCenterSubview(false);
    return;
  }
  if (agentKey === "officecli") {
    openOfficeCLIWorkspace(false);
    return;
  }
  const item = findAgentCenterOpenable(agentKey);
  if (item) {
    openAgentWorkspace(item, false);
    return;
  }
  clearAgentCenterSubview(false);
}

function handleAgentCenterHistoryPopState() {
  if (store.activeModuleId !== "userAgentCenter") return;
  restoreAgentCenterSubviewFromHistory(agentCenterSubviewHistoryState());
}

function handleAgentTemplateCardClick(item: AgentCenterOpenable) {
  openAgentWorkspace(item);
}

function handleAgentTemplateAction(item: AgentCenterOpenable) {
  openAgentWorkspace(item);
}

function handleAgentRowAction(item: AgentCenterOpenable) {
  openAgentWorkspace(item);
}

function agentCenterFavoriteKey(item: AgentCenterOpenable) {
  return item.agentKey || item.name;
}

function isAgentCenterFavorite(item: AgentCenterOpenable) {
  return agentCenterFavoriteKeys.value.includes(agentCenterFavoriteKey(item));
}

function toggleAgentCenterFavorite(item: AgentCenterOpenable) {
  const key = agentCenterFavoriteKey(item);
  const active = agentCenterFavoriteKeys.value.includes(key);
  agentCenterFavoriteKeys.value = active
    ? agentCenterFavoriteKeys.value.filter((itemKey) => itemKey !== key)
    : [...agentCenterFavoriteKeys.value, key];
  ElMessage.success(active ? "已取消收藏" : "已收藏智能体");
}

async function copyAgentCenterConfig(item: AgentCenterOpenable) {
  const workspace = buildAgentCenterWorkspace(item);
  await copyToClipboard(JSON.stringify({
    name: workspace.name,
    type: workspace.type,
    model: workspace.model,
    knowledge: workspace.knowledge,
    prompt: workspace.prompt,
    tools: workspace.toolTags
  }, null, 2));
}

function handleAgentCenterShortcut(label: string) {
  if (label.includes("模板")) {
    document.querySelector(".user-agent-template-grid")?.scrollIntoView({ behavior: "smooth", block: "center" });
    return;
  }
  if (label.includes("工具")) {
    selectAdminModule("apiSettings");
    return;
  }
  if (label.includes("知识")) {
    selectAdminModule("knowledgeAdmin");
    return;
  }
  ElMessage.info("调用日志暂未开放独立模块，当前可在数据概览查看调用量");
}

function sendAgentWorkspaceMessage() {
  const workspace = agentCenterWorkspace.value;
  const prompt = agentWorkspaceDraft.value.trim();
  if (!workspace) return;
  if (!prompt) {
    ElMessage.warning("请先输入测试指令");
    return;
  }
  agentWorkspaceMessages.value.push({ role: "user", text: prompt });
  agentWorkspaceMessages.value.push({
    role: "assistant",
    text: `${workspace.name} 已接收测试指令。我会围绕「${workspace.type}」场景处理，并结合 ${workspace.knowledge} 输出可执行回复。`
  });
  agentWorkspaceDraft.value = "";
}

const agentMobileBottomNav = [
  { label: "首页", letter: "H", targetId: "userDashboard" },
  { label: "创作", letter: "+", targetId: "userAiImage" },
  { label: "智能体", letter: "A", targetId: "userAgentCenter" },
  { label: "作品", letter: "W", targetId: "userWorks" },
  { label: "我的", letter: "M", targetId: "userMembership" }
];
const userHomeInspirations: UserHomeEntry[] = [
  { title: "产品海报", desc: "一张科技产品发布海报，白色空间，青紫主光，中心产品清晰，适合官网首屏展示", targetId: "userAiImage", mode: "image", coverClass: "is-product", prompt: "一张科技感产品海报，干净白色背景，青绿色主光，中心是一台未来感 AI 设备，适合官网首屏展示。" },
  { title: "电商主图", desc: "一张高级电商主图，干净浅灰背景，产品居中，柔和阴影，突出材质细节", targetId: "userAiImage", mode: "image", coverClass: "is-commerce", prompt: "生成一张高级电商主图，干净浅灰背景，产品居中，柔和阴影，突出材质细节。" },
  { title: "小红书封面", desc: "明亮自然光，画面留白充足，真实生活方式场景，适合种草内容", targetId: "userAiImage", mode: "image", coverClass: "is-xhs", prompt: "设计一张小红书风格封面，明亮自然光，画面留白充足，真实生活方式场景。" },
  { title: "无线画布", desc: "整理参考图和灵感节点，随意拖拽，自由创作", targetId: "userWirelessCanvas", mode: "image", coverClass: "is-canvas" },
  { title: "PPT 文档", desc: "把图片创意变成方案，自动生成演示文稿", targetId: "userPptGeneration", mode: "ppt", coverClass: "is-ppt" },
  { title: "作品中心", desc: "查看与管理你的全部创作", targetId: "userWorks", coverClass: "is-works" }
];
const userHomeTemplates: UserHomeEntry[] = [
  { title: "品牌设计", desc: "LOGO、VI、品牌形象设计", targetId: "userAiImage", mode: "image", coverClass: "is-brand", coverText: "BRAND DESIGN", prompt: "设计一组高端品牌视觉，包含主视觉、色彩和海报方向。" },
  { title: "电商营销", desc: "主图、海报、促销图", targetId: "userAiImage", mode: "image", coverClass: "is-sale", coverText: "SALE", prompt: "生成一张电商促销海报，浅色背景，橙色 CTA，突出产品卖点。" },
  { title: "海报设计", desc: "活动、宣传、视觉海报", targetId: "userAiImage", mode: "image", coverClass: "is-poster", coverText: "FUTURE", prompt: "生成一张未来科技主题活动海报，蓝紫光感，空间干净。" },
  { title: "IP 角色", desc: "角色设计、形象定制", targetId: "userAiImage", mode: "image", coverClass: "is-ip", coverText: "IP" },
  { title: "商品主图", desc: "电商主图、白底图", targetId: "userAiImage", mode: "image", coverClass: "is-product-main", coverText: "PRODUCT" },
  { title: "详情长图", desc: "产品详情、长图文案", targetId: "userAiImage", mode: "image", coverClass: "is-detail", coverText: "DETAIL" },
  { title: "短视频封面", desc: "爆款封面、吸睛标题", targetId: "userVideoGeneration", mode: "video", coverClass: "is-video", coverText: "PLAY" },
  { title: "PPT 文档", desc: "汇报、演示、课件", targetId: "userPptGeneration", mode: "ppt", coverClass: "is-doc", coverText: "BUSINESS PLAN" },
  { title: "朋友圈海报", desc: "节日、日签、营销海报", targetId: "userAiImage", mode: "image", coverClass: "is-moments", coverText: "Good Morning" },
  { title: "企业宣传图", desc: "企业介绍、宣传物料", targetId: "userAiImage", mode: "image", coverClass: "is-corporate", coverText: "COMPANY" }
];
function selectUserHomeCreationMode(mode: UserHomeCreationMode) {
  userHomeCreationMode.value = mode;
}
function userHomeTargetForMode(mode = userHomeCreationMode.value) {
  if (mode === "video") return "userVideoGeneration";
  if (mode === "ppt") return "userPptGeneration";
  return "userAiImage";
}
function launchUserHomeCreation() {
  if (userHomeCreationMode.value === "agent") aiPlaygroundMode.value = "agent";
  if (userHomeCreationMode.value === "image") aiPlaygroundMode.value = "gallery";
  void selectAdminModule(userHomeTargetForMode());
}
function openUserHomeEntry(entry: UserHomeEntry) {
  if (entry.prompt) onlineImageForm.value.prompt = entry.prompt;
  if (entry.mode) userHomeCreationMode.value = entry.mode;
  if (entry.mode === "agent") aiPlaygroundMode.value = "agent";
  if (entry.mode === "image") aiPlaygroundMode.value = "gallery";
  void selectAdminModule(entry.targetId);
}
function applyUserHomePrompt(action: string) {
  const prompt = onlineImageForm.value.prompt.trim();
  onlineImageForm.value.prompt = prompt ? `${action}：${prompt}` : `${action}：`;
}
const aiCountInput = ref(String(onlineImageForm.value.count));
const aiOutputCompressionInput = ref(onlineImageForm.value.outputCompression == null ? "" : String(onlineImageForm.value.outputCompression));
watch(
  () => onlineImageForm.value.count,
  (value) => {
    const nextValue = String(value);
    if (aiCountInput.value !== nextValue) aiCountInput.value = nextValue;
  }
);
watch(
  () => onlineImageForm.value.outputCompression,
  (value) => {
    const nextValue = value == null ? "" : String(value);
    if (aiOutputCompressionInput.value !== nextValue) aiOutputCompressionInput.value = nextValue;
  }
);
const onlineSubmitting = ref(false);
const onlineReferenceSlots = [1, 2, 3];
const onlineReferenceImages = computed(() => aiReferenceImages.value.slice(0, onlineReferenceSlots.length));
const onlineCountOptions = [1, 2, 3, 4];
const onlineStatusFilter = ref("ALL");
const aiPlaygroundMode = ref("gallery");
const aiPlaygroundModeOptions = [
  { label: "画廊", value: "gallery" },
  { label: "Agent", value: "agent" }
];
type AiReferenceImage = { id: string; name: string; url: string; file?: File; remoteUrl?: string; previewUrl?: string; uploading?: boolean; error?: string };
type AiGenerationReference = { id: string; name: string; url: string; order: number; source: "upload" | "task" | "draft" | "paste" | "local" };
type AiGenerationTaskSnapshot = {
  prompt: string;
  inputImageIds: string[];
  inputImagesSnapshot: AiGenerationReference[];
  maskDraft: null;
  maskTargetImageId: string;
  maskImageId: string;
  createdAt: string;
};
type AiFavoriteCollection = { id: string; name: string; taskIds: string[]; createdAt?: string; updatedAt?: string };
type AiAgentMessage = { role: "user" | "assistant"; content: string; createdAt?: string };
type AiAgentConversation = { id: string; title: string; messages: AiAgentMessage[]; createdAt: string; updatedAt?: string };
type PromptEditableExpose = { focus: () => void; adjustHeight: () => void };
type UserAIState = {
  userId?: string;
  favoriteTaskIds?: string[];
  hiddenTaskIds?: string[];
  favoriteCollections?: AiFavoriteCollection[];
  defaultFavoriteCollectionId?: string | null;
  agentConversations?: AiAgentConversation[];
  activeConversationId?: string;
  activeCollectionId?: string;
};
const aiGalleryFilter = ref("all");
const aiFavoriteOnly = ref(false);
const aiPromptSearch = ref("");
const aiGalleryInitialCount = 24;
const aiGalleryPageSize = 24;
const aiGalleryVisibleCount = ref(aiGalleryInitialCount);
const aiFavoriteTaskIds = ref<string[]>([]);
const aiSelectedTaskIds = ref<string[]>([]);
const aiMobileSelectionMode = ref(false);
const aiTouchSelectionTriggered = ref(false);
const aiHiddenTaskIds = ref<string[]>([]);
const aiFavoriteCollectionsVisible = ref(false);
const aiFavoritePickerVisible = ref(false);
const aiFavoritePickerTaskIds = ref<string[]>([]);
const aiFavoritePickerCheckedIds = ref<string[]>([]);
const aiFavoritePickerDraggedId = ref("");
const aiFavoritePickerDragOverId = ref("");
const aiFavoritePickerDropPosition = ref<"before" | "after" | "">("");
const aiFavoriteCollections = ref<AiFavoriteCollection[]>([{ id: "default", name: "默认", taskIds: [], createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }]);
const aiDefaultFavoriteCollectionId = ref<string | null>("default");
const aiActiveFavoriteCollectionId = ref("");
const aiOptimisticTasks = ref<AdminRecord[]>([]);
const aiSelectedFavoriteCollectionIds = ref<string[]>([]);
const aiNewCollectionName = ref("");
const aiEditingCollectionId = ref("");
const aiEditingCollectionName = ref("");
const aiReferenceImages = ref<AiReferenceImage[]>([]);
const aiDetailTaskId = ref("");
const aiDetailImageMeta = ref<Record<string, { width: number; height: number }>>({});
const aiOriginalImageCache = ref<Record<string, string>>({});
const aiRawUrlsTaskId = ref("");
const aiRawResponseTaskId = ref("");
const aiLightboxTaskId = ref("");
const aiLightboxViewportRef = ref<HTMLElement | null>(null);
const aiFloatingComposerRef = ref<HTMLElement | null>(null);
const aiPromptInputRef = ref<PromptEditableExpose | null>(null);
const aiLightboxScale = ref(1);
const aiLightboxTx = ref(0);
const aiLightboxTy = ref(0);
const aiLightboxDragging = ref(false);
const aiBrokenThumbnailKeys = ref<string[]>([]);
const aiImageContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  taskId: ""
});
const aiTaskClockNow = ref(Date.now());
let aiTaskClockTimer: number | null = null;
let aiGenerationPollTimer: number | null = null;
let aiOriginalImagePrefetchTimer: number | null = null;
const aiTrackedGenerationTaskIds = ref<string[]>([]);
const aiGenerationPollAttempts = new Map<string, number>();
const aiGenerationPollDelaysMs = [2000, 3000, 5000, 8000, 13000, 20000];
const aiGenerationPollMaxAttempts = 90;
let aiComposerResizeObserver: ResizeObserver | null = null;
const aiAgentRunning = ref(false);
const aiAgentActiveConversationId = ref("agent-default");
const aiAgentConversations = ref<AiAgentConversation[]>([{
  id: "agent-default",
  title: "默认对话",
  createdAt: new Date().toISOString(),
  messages: [{ role: "assistant", content: "我可以根据提示词、参考图和当前参数协助规划生成任务。" }]
}]);
let aiImageDraftHydrated = false;
let aiImageDraftSaveTimer: ReturnType<typeof window.setTimeout> | null = null;
let aiTaskLongPressTimer: ReturnType<typeof window.setTimeout> | null = null;
const aiOriginalImageCachePending = new Set<string>();
let aiLightboxPointerStart: { id: number; x: number; y: number; baseX: number; baseY: number; moved: boolean; startedAt: number } | null = null;
let aiLightboxPinchStart: { distance: number; scale: number; tx: number; ty: number; midX: number; midY: number } | null = null;
let aiLightboxLastTap = { time: 0, x: 0, y: 0 };
const aiLightboxPointers = new Map<number, { x: number; y: number }>();
const aiStateHydrated = ref(false);
const aiStateSaving = ref(false);
const aiStateSaveQueued = ref(false);
const aiSizePickerVisible = ref(false);
const aiSizePickerRef = ref<HTMLElement | null>(null);
const aiSizePickerMouseDownTarget = ref<EventTarget | null>(null);
const aiSizePickerMode = ref<"auto" | "ratio" | "resolution">("auto");
const aiSizeTier = ref<"1K" | "2K" | "4K">("1K");
const aiSizeRatio = ref("1:1");
const aiCustomRatio = ref("16:9");
const aiCustomWidth = ref("1024");
const aiCustomHeight = ref("1024");
const aiImageModuleSchema = ref<AdminRecord | null>(null);
const aiImageModuleSchemaLoading = ref(false);
const aiImageModuleSchemaKey = ref("");
const aiSizePickerModes = [
  { label: "自动", value: "auto" },
  { label: "按比例", value: "ratio" },
  { label: "自定义宽高", value: "resolution" }
] as const;
const aiSizeTiers = ["1K", "2K", "4K"] as const;
const aiSizeRatios = [
  { label: "1:1", value: "1:1" },
  { label: "3:2", value: "3:2" },
  { label: "2:3", value: "2:3" },
  { label: "16:9", value: "16:9" },
  { label: "9:16", value: "9:16" },
  { label: "4:3", value: "4:3" },
  { label: "3:4", value: "3:4" },
  { label: "21:9", value: "21:9" }
];
const selectedApiReferenceIndex = ref(0);
const apiQuickKeys = ref<Record<string, string>>({});
const apiRecommendMode = ref(false);
const apiVerifyResult = ref("");
type ApiVerifyPanel = {
  tone: "success" | "warning" | "info";
  icon: string;
  message: string;
  protocolPrefix: string;
  protocolLabel: string;
  imageRequestModeLabel: string;
  statusCode: number;
  raw: unknown;
};
const apiVerifyPanel = ref<ApiVerifyPanel | null>(null);
const apiPendingProviders = ref<AdminRecord[]>([]);
const apiSavingProviderDraft = ref(false);
const apiTestingProviderDraft = ref(false);
const apiProbingProviderProtocol = ref(false);
const apiFetchingDraftModels = ref(false);
const apiSyncingDraftModels = ref(false);
const apiDraggingProviderId = ref("");
const apiReorderingProviders = ref(false);
const apiProviderDraft = ref({
  id: "",
  name: "API",
  baseUrl: "https://api.example.com/v1",
  apiKey: "",
  protocol: "openai",
  imageRequestMode: "openai",
  imageGenerationEndpoint: "/v1/images/generations",
  imageEditEndpoint: "/v1/images/edits",
  videoGenerationEndpoint: "",
  priority: 50,
  imageModels: ["gpt-image-2"],
  chatModels: ["gpt-4o"],
  videoModels: ["doubao-seedance-2.0", "veo3-fast"],
  loras: ["Tongyi-MAI/Z-Image-Turbo"],
  modelProtocols: {} as Record<string, string>
});
type ApiModelCategory = "image" | "chat" | "video";
type ApiModelPickerTab = ApiModelCategory | "all";
type ApiModelPickerItem = { id: string; category: ApiModelCategory; selected: boolean };
const apiModelPickerOpen = ref(false);
const apiModelPickerFilter = ref("");
const apiModelPickerTab = ref<ApiModelPickerTab>("all");
const apiModelPickerItems = ref<ApiModelPickerItem[]>([]);
const apiFetchedModelIds = ref<string[]>([]);
const apiFetchedModelSuggestions = ref<Record<ApiModelCategory, Set<string>>>({
  image: new Set<string>(),
  chat: new Set<string>(),
  video: new Set<string>()
});
const apiModelPickerTabs: Array<{ label: string; value: ApiModelPickerTab }> = [
  { label: "全部", value: "all" },
  { label: "生图", value: "image" },
  { label: "LLM", value: "chat" },
  { label: "视频", value: "video" }
];
const aiCommonSizePresets: Record<(typeof aiSizeTiers)[number], Record<string, string>> = {
  "1K": {
    "1:1": "1024x1024",
    "3:2": "1536x1024",
    "2:3": "1024x1536",
    "16:9": "1280x720",
    "9:16": "720x1280",
    "4:3": "1024x768",
    "3:4": "768x1024",
    "21:9": "1280x544"
  },
  "2K": {
    "1:1": "2048x2048",
    "3:2": "2160x1440",
    "2:3": "1440x2160",
    "16:9": "2560x1440",
    "9:16": "1440x2560",
    "4:3": "2048x1536",
    "3:4": "1536x2048",
    "21:9": "2560x1088"
  },
  "4K": {
    "1:1": "2880x2880",
    "3:2": "3456x2304",
    "2:3": "2304x3456",
    "16:9": "3840x2160",
    "9:16": "2160x3840",
    "4:3": "3200x2400",
    "3:4": "2400x3200",
    "21:9": "3840x1600"
  }
};
function aiSchemaFieldsFromResponse(response: AdminRecord | null) {
  if (!response) return [];
  if (Array.isArray(response.fields)) return response.fields as AdminRecord[];
  const schema = response.schema;
  if (schema && typeof schema === "object" && Array.isArray((schema as AdminRecord).fields)) return (schema as AdminRecord).fields as AdminRecord[];
  const schemaJson = response.schema_json || response.schemaJson;
  if (schemaJson && typeof schemaJson === "object" && Array.isArray((schemaJson as AdminRecord).fields)) return (schemaJson as AdminRecord).fields as AdminRecord[];
  return [];
}

function aiSchemaStringValue(value: unknown) {
  if (value === null || value === undefined) return "";
  return String(value).trim();
}

const aiImageSizeSchemaField = computed(() => aiSchemaFieldsFromResponse(aiImageModuleSchema.value).find((field) => String(field.key || "") === "size") || null);
const aiImageSizeSchemaDefault = computed(() => {
  const rawValue = aiSchemaStringValue(aiImageSizeSchemaField.value?.default);
  if (!rawValue) return "";
  return rawValue.toLowerCase() === "auto" ? "auto" : normalizeAiImageSize(rawValue);
});
const aiImageSizeSchemaOptions = computed(() => {
  const field = aiImageSizeSchemaField.value;
  if (!field) return [];
  const sourceOptions = Array.isArray(field.options) ? field.options : [];
  const values = [field.default, ...sourceOptions]
    .map((item) => {
      const value = aiSchemaStringValue(item);
      if (!value) return "";
      return value.toLowerCase() === "auto" ? "auto" : normalizeAiImageSize(value);
    })
    .filter((item) => item && (item === "auto" || /^\d+x\d+$/.test(item)));
  return Array.from(new Set(values));
});
const aiSettingsVisible = ref(false);
const aiSettingsTab = ref("api");
const aiSettingsTabs = [
  { id: "api", label: "API 配置", icon: Key },
  { id: "general", label: "习惯配置", icon: Refresh },
  { id: "agent", label: "Agent 配置", icon: Cpu },
  { id: "data", label: "数据管理", icon: DataAnalysis },
  { id: "about", label: "关于", icon: QuestionFilled }
];
const aiSettingsApiModes = [
  { label: "Images", value: "images" },
  { label: "Responses", value: "responses" }
];
const aiAgentApiModes = [
  { label: "关闭", value: "off" },
  { label: "原生", value: "native" },
  { label: "混合", value: "hybrid" }
];
const aiHabitSwitches = [
  { key: "clearInputAfterSubmit", title: "提交后清空输入框", desc: "任务提交成功后自动清空 Prompt 输入。" },
  { key: "persistInputOnRestart", title: "重启后加载上次输入框", desc: "重新打开页面时恢复上一次未提交内容。" },
  { key: "reuseTaskApiProfile", title: "复用配置", desc: "复用历史任务时同步使用当时的 API 配置。" },
  { key: "showRetryButton", title: "成功任务展示重试按钮", desc: "在已完成任务卡片上保留重试入口。" },
  { key: "allowPromptRewrite", title: "允许提示词改写", desc: "提交前允许系统进行提示词增强。" },
  { key: "taskDoneNotify", title: "任务完成通知", desc: "生成完成后展示桌面或页面通知。" },
  { key: "agentScrollBottom", title: "Agent 滚动到底部", desc: "Agent 回复后自动滚动到最新消息。" },
  { key: "mathPromptTip", title: "公式输出提示", desc: "提示 Agent 对公式内容保持可读格式。" }
] as const;
const aiSettingsDraft = ref<AiSettingsDraft>(aiSettingsStore.createDraft(onlineImageForm.value.model));
const onlineStatusOptions = [
  { label: "全部", value: "ALL" },
  { label: "排队中", value: "PENDING" },
  { label: "生成中", value: "RUNNING" },
  { label: "已完成", value: "SUCCEEDED" },
  { label: "失败", value: "FAILED" }
];
const userWorkStatusOptions = [
  { label: "全部作品", value: "all" },
  { label: "已完成", value: "done" },
  { label: "生成中", value: "running" },
  { label: "失败", value: "failed" },
  { label: "收藏", value: "favorite" }
];
const worksSearchKeyword = ref("");
const worksStatusFilter = ref("all");
const worksViewMode = ref<"grid" | "table">("grid");
const worksSourceTab = ref<"official" | "mine">("official");
const publicOfficialCases = ref<AdminRecord[]>([]);
const publicOfficialCasesLoading = ref(false);

const onlineImageData = computed(() => store.data as AdminRecord & {
  providers?: AdminRecord[];
  models?: AdminRecord[];
  recentTasks?: AdminRecord[];
  assets?: AdminRecord[];
  recentAssets?: AdminRecord[];
  aiState?: UserAIState;
  queue?: Record<string, unknown>;
});

const onlineProviders = computed<AdminRecord[]>(() => Array.isArray(onlineImageData.value.providers) ? onlineImageData.value.providers : []);
const onlineModels = computed<AdminRecord[]>(() => Array.isArray(onlineImageData.value.models) ? onlineImageData.value.models : []);

function isOptimisticAiTaskReconciled(optimisticTask: AdminRecord, serverTasks: AdminRecord[]) {
  const id = String(optimisticTask.id || "");
  if (id && serverTasks.some((task) => String(task.id || "") === id)) return true;
  const prompt = String(optimisticTask.prompt || "").trim();
  const model = String(optimisticTask.model || "").trim();
  const userId = String(optimisticTask.userId || "").trim();
  return serverTasks.some((task) => {
    const taskPrompt = String(task.prompt || "").trim();
    const taskModel = String(task.model || "").trim();
    const taskUserId = String(task.userId || "").trim();
    if (prompt && taskPrompt !== prompt) return false;
    if (model && taskModel !== model) return false;
    if (userId && taskUserId && taskUserId !== userId) return false;
    return ["PENDING", "RUNNING", "SUCCEEDED", "FAILED", "ERROR"].includes(String(task.status || "").toUpperCase());
  });
}

const onlineRecentTasks = computed<AdminRecord[]>(() => {
  const serverTasks = Array.isArray(onlineImageData.value.recentTasks) ? onlineImageData.value.recentTasks : [];
  if (!aiOptimisticTasks.value.length) return serverTasks;
  const pendingTasks = aiOptimisticTasks.value.filter((task) => !isOptimisticAiTaskReconciled(task, serverTasks));
  return [...pendingTasks, ...serverTasks];
});
const onlineImageTasks = computed<AdminRecord[]>(() => onlineRecentTasks.value.filter((task) => !isVideoGenerationTask(task)));
const onlineRecentTasksVideoSignature = computed(() => onlineRecentTasks.value.map((task) => [
  String(task.id || ""),
  String(task.status || ""),
  String(task.progress || ""),
  String(task.updatedAt || ""),
  videoTaskUrl(task)
].join("\u001f")).join("\u001e"));
const videoHistorySaveSignature = computed(() => [
  videoHistory.value.map((entry) => {
    const slim = slimVideoHistoryEntry(entry);
    return [
      slim.id,
      slim.taskId,
      slim.backendTaskId,
      slim.status,
      slim.url,
      slim.prompt,
      slim.model,
      slim.createdAt,
      String(slim.timestamp),
      slim.errorMessage || ""
    ].join("\u001f");
  }).join("\u001e"),
  videoHiddenHistoryIds.value.join("\u001e"),
  selectedVideoHistoryId.value
].join("\u001d"));
const videoHistoryPollingSignature = computed(() => [
  store.activeModuleId,
  videoHistory.value
    .filter((entry) => entry.status === "generating" && entry.backendTaskId)
    .map((entry) => `${entry.id}:${entry.backendTaskId}:${entry.status}`)
    .join("|")
].join("::"));
watch(
  onlineRecentTasksVideoSignature,
  () => {
    const entries = onlineRecentTasks.value.map(taskToVideoHistoryEntry).filter(Boolean) as VideoHistoryEntry[];
    if (entries.length) mergeVideoHistoryEntries(entries);
  }
);
watch(videoHistorySaveSignature, scheduleVideoHistorySave);
watch([videoPrompt, videoHistorySearchQuery], scheduleVideoInputDraftSave);
watch(videoPrompt, () => void nextTick(adjustVideoPromptHeight));
let guestImagePromptTracked = false;
let guestVideoPromptTracked = false;
watch(() => onlineImageForm.value.prompt, (value) => {
  if (!guestImagePromptTracked && isGuestUser.value && String(value || "").trim()) {
    guestImagePromptTracked = true;
    trackWebGuestExperience("guest_input_prompt", "userAiImage", { module: "userAiImage" });
  }
});
watch(videoPrompt, (value) => {
  if (!guestVideoPromptTracked && isGuestUser.value && value.trim()) {
    guestVideoPromptTracked = true;
    trackWebGuestExperience("guest_input_prompt", "userVideoGeneration", { module: "userVideoGeneration" });
  }
});
watch([videoHistorySearchQuery, videoHistoryFilter, videoHistorySort], () => {
  videoHistoryVisibleCount.value = videoHistoryInitialVisibleCount;
});
watch(
  () => store.activeModuleId,
  (moduleId) => {
    if (moduleId !== "userVideoGeneration") activeVideoHistoryPreviewIds.value = [];
  }
);
watch([aiPromptSearch, aiGalleryFilter, aiFavoriteOnly, aiActiveFavoriteCollectionId], () => {
  aiGalleryVisibleCount.value = aiGalleryInitialCount;
});

async function pollVideoHistoryTasks() {
  const running = videoHistory.value.filter((entry) => entry.status === "generating" && entry.backendTaskId);
  if (!running.length) return;
  await Promise.all(running.map(async (entry) => {
    try {
      const task = await adminRequest<AdminRecord>({ method: "GET", url: `/generation-tasks/${encodeURIComponent(entry.backendTaskId || "")}` });
      const nextEntry = taskToVideoHistoryEntry(task);
      if (nextEntry) commitVideoHistoryEntry(nextEntry, entry.id);
    } catch {
      // Polling is best-effort; persisted generating entries remain visible.
    }
  }));
}

function refreshVideoHistoryPolling() {
  if (typeof window === "undefined") return;
  if (videoHistoryPollTimer) {
    window.clearInterval(videoHistoryPollTimer);
    videoHistoryPollTimer = null;
  }
  const shouldPoll = store.activeModuleId === "userVideoGeneration" && videoHistory.value.some((entry) => entry.status === "generating" && entry.backendTaskId);
  if (shouldPoll) videoHistoryPollTimer = window.setInterval(() => void pollVideoHistoryTasks(), 5000);
}

watch(
  videoHistoryPollingSignature,
  refreshVideoHistoryPolling,
);
const usesAiImageWorkspace = computed(() => ["userAiImage", "userWirelessCanvas"].includes(store.activeModuleId));
const hasRunningAiGenerationTasks = computed(() => onlineImageTasks.value.some((task) => isAiTaskRunning(task)));
const onlineAssets = computed<AdminRecord[]>(() => {
  if (Array.isArray(onlineImageData.value.assets)) return onlineImageData.value.assets;
  if (Array.isArray(onlineImageData.value.recentAssets)) return onlineImageData.value.recentAssets;
  return [];
});

function aiImageDraftPayload() {
  return {
    form: {
      prompt: onlineImageForm.value.prompt,
      model: onlineImageForm.value.model,
      provider: onlineImageForm.value.provider,
      ratio: onlineImageForm.value.ratio,
      size: onlineImageForm.value.size,
      quality: onlineImageForm.value.quality,
      outputFormat: onlineImageForm.value.outputFormat,
      transparentOutput: onlineImageForm.value.transparentOutput,
      outputCompression: onlineImageForm.value.outputCompression,
      moderation: onlineImageForm.value.moderation,
      count: onlineImageForm.value.count,
      resolution: onlineImageForm.value.resolution,
      width: onlineImageForm.value.width,
      height: onlineImageForm.value.height
    },
    ui: {
      playgroundMode: aiPlaygroundMode.value,
      galleryFilter: aiGalleryFilter.value,
      favoriteOnly: aiFavoriteOnly.value,
      promptSearch: aiPromptSearch.value,
      activeCollectionId: aiActiveFavoriteCollectionId.value
    },
    referenceImages: aiReferenceImages.value
      .filter((item) => item.url.startsWith("data:") || Boolean(item.file) || Boolean(item.remoteUrl))
      .map<CachedReferenceImage>((item) => ({
        id: item.id,
        name: item.name,
        url: item.url.startsWith("blob:") ? "" : item.url,
        file: item.file,
        remoteUrl: item.remoteUrl,
      })),
    savedAt: Date.now()
  };
}

function scheduleAiImageDraftSave() {
  if (!aiImageDraftHydrated || typeof window === "undefined") return;
  if (aiImageDraftSaveTimer) window.clearTimeout(aiImageDraftSaveTimer);
  aiImageDraftSaveTimer = window.setTimeout(() => {
    void writeAiImageDraft(aiImageDraftPayload()).catch(() => undefined);
  }, 260);
}

function aiReferenceDraftSignature(item: AiReferenceImage) {
  const url = String(item.url || "");
  const previewUrl = String(item.previewUrl || "");
  const remoteUrl = String(item.remoteUrl || "");
  return [
    item.id,
    item.name,
    url.startsWith("data:") ? `data:${url.length}` : url,
    previewUrl.startsWith("data:") ? `preview:${previewUrl.length}` : previewUrl,
    remoteUrl,
    String(Boolean(item.uploading)),
    item.error || ""
  ].join("\u001f");
}

const aiImageDraftSaveSignature = computed(() => [
  onlineImageForm.value.prompt,
  onlineImageForm.value.model,
  onlineImageForm.value.provider,
  onlineImageForm.value.ratio,
  onlineImageForm.value.size,
  onlineImageForm.value.quality,
  onlineImageForm.value.outputFormat,
  String(onlineImageForm.value.transparentOutput),
  String(onlineImageForm.value.outputCompression),
  onlineImageForm.value.moderation,
  String(onlineImageForm.value.count),
  onlineImageForm.value.resolution,
  String(onlineImageForm.value.width),
  String(onlineImageForm.value.height),
  aiPlaygroundMode.value,
  aiGalleryFilter.value,
  String(aiFavoriteOnly.value),
  aiPromptSearch.value,
  aiActiveFavoriteCollectionId.value,
  aiReferenceImages.value.map(aiReferenceDraftSignature).join("\u001e")
].join("\u001d"));

async function hydrateAiImageDraft() {
  try {
    const draft = await readAiImageDraft();
    if (!draft) {
      aiImageDraftHydrated = true;
      return;
    }
    const form = draft.form || {};
    onlineImageForm.value = {
      ...onlineImageForm.value,
      ...form,
      prompt: typeof form.prompt === "string" ? form.prompt : onlineImageForm.value.prompt,
      count: Number(form.count || onlineImageForm.value.count),
      width: Number(form.width || onlineImageForm.value.width),
      height: Number(form.height || onlineImageForm.value.height),
      outputCompression: form.outputCompression === null || form.outputCompression === "" ? null : Number(form.outputCompression)
    };
    aiCountInput.value = String(onlineImageForm.value.count);
    aiOutputCompressionInput.value = onlineImageForm.value.outputCompression == null ? "" : String(onlineImageForm.value.outputCompression);
    if (draft.ui?.playgroundMode) aiPlaygroundMode.value = draft.ui.playgroundMode;
    if (draft.ui?.galleryFilter) aiGalleryFilter.value = draft.ui.galleryFilter;
    aiFavoriteOnly.value = Boolean(draft.ui?.favoriteOnly);
    aiPromptSearch.value = draft.ui?.promptSearch || "";
    aiActiveFavoriteCollectionId.value = draft.ui?.activeCollectionId || "";
    aiReferenceImages.value.forEach(revokeAiReferenceImage);
    aiReferenceImages.value = (draft.referenceImages || []).flatMap((item) => {
      const url = item.file instanceof File ? URL.createObjectURL(item.file) : item.remoteUrl || item.url;
      if (!url) return [];
      return [{ id: item.id, name: item.name, url, file: item.file, remoteUrl: item.remoteUrl }];
    });
  } catch {
    // IndexedDB is optional; the page should still work without it.
  } finally {
    aiImageDraftHydrated = true;
  }
}

watch(aiImageDraftSaveSignature, scheduleAiImageDraftSave);

const onlineProviderOptions = computed(() => {
  const items = onlineProviders.value.map((provider) => ({ label: String(provider.name || provider.id || "API 平台"), value: String(provider.id || provider.name || "") })).filter((item) => item.value);
  return items.length ? items : [{ label: "请先在主控 SaaS 配置 API 渠道", value: "" }];
});
const onlineProviderModels = computed<AdminRecord[]>(() => {
  const items: AdminRecord[] = [];
  const seen = new Set<string>();
  const providers = [...onlineProviders.value].sort((left, right) => {
    if (Boolean(left.primary) !== Boolean(right.primary)) return Boolean(left.primary) ? -1 : 1;
    return Number(left.priority || 1000) - Number(right.priority || 1000);
  });
  providers.forEach((provider) => {
    const providerId = String(provider.id || provider.name || "");
    const providerName = String(provider.name || provider.id || "API 平台");
    const models = Array.isArray(provider.models) ? provider.models : [];
    models.forEach((rawModel) => {
      const model = String(rawModel || "").trim();
      const key = `${providerId}:${model}`;
      if (!model || seen.has(key)) return;
      seen.add(key);
      items.push({ id: model, model, name: model, providerId, providerName, fixedQuota: model === "mock-standard" ? 1 : 10 });
    });
  });
  return items;
});
const onlineModelOptions = computed(() => {
  const sourceModels = [...onlineProviderModels.value, ...onlineModels.value];
  const seen = new Set<string>();
  const items = sourceModels.map((model) => {
    const value = String(model.model || model.id || "");
    const providerName = String(model.providerName || "");
    const label = `${String(model.name || model.model || model.id || "模型")}${providerName ? ` · ${providerName}` : ""}`;
    return { label, value };
  }).filter((item) => {
    if (!item.value || seen.has(item.value)) return false;
    seen.add(item.value);
    return true;
  });
  return items.length ? items : [{ label: "当前图像模型", value: "gpt-image-2" }, { label: "本地演示模型", value: "mock-standard" }];
});
const activeOnlineModel = computed(() => {
  const currentProvider = String(onlineImageForm.value.provider || "");
  const currentModel = String(onlineImageForm.value.model || "");
  return onlineProviderModels.value.find((model) => String(model.providerId || "") === currentProvider && String(model.model || model.id) === currentModel)
    || [...onlineProviderModels.value, ...onlineModels.value].find((model) => String(model.model || model.id) === currentModel);
});
const onlineEstimatedCost = computed(() => Math.max(1, Number(activeOnlineModel.value?.fixedQuota || activeOnlineModel.value?.modelRatio || 1)) * Number(onlineImageForm.value.count || 1));
const onlineProviderModeLabel = computed(() => {
  const item = onlineProviders.value.find((provider) => String(provider.id || provider.name) === onlineImageForm.value.provider);
  const protocol = String(item?.protocol || item?.type || "openai");
  return `协议：${protocol} · 模型按点数扣费`;
});
function syncOnlineProviderForModel() {
  const providerModels = onlineProviderModels.value;
  if (!providerModels.length) {
    onlineImageForm.value.provider = "";
    return;
  }
  const currentModel = String(onlineImageForm.value.model || "");
  const currentProvider = String(onlineImageForm.value.provider || "");
  const primaryMatch = providerModels.find((item) => String(item.model || item.id || "") === currentModel && onlineProviders.value.some((provider) => String(provider.id || provider.name || "") === String(item.providerId || "") && Boolean(provider.primary)));
  if (primaryMatch) {
    onlineImageForm.value.provider = String(primaryMatch.providerId || "");
    return;
  }
  if (currentProvider && providerModels.some((item) => String(item.providerId || "") === currentProvider && String(item.model || item.id || "") === currentModel)) {
    return;
  }
  const matched = providerModels.find((item) => String(item.model || item.id || "") === currentModel) || providerModels[0];
  if (matched && String(matched.model || matched.id || "") !== currentModel) {
    onlineImageForm.value.model = String(matched.model || matched.id || currentModel);
  }
  const providerId = String(matched?.providerId || "");
  if (providerId) onlineImageForm.value.provider = providerId;
}
watch(
  () => onlineImageForm.value.model,
  () => syncOnlineProviderForModel()
);
watch(
  onlineProviderModels,
  () => syncOnlineProviderForModel()
);
const onlinePreviewTask = computed(() => onlineImageTasks.value.find((task) => String(task.status || "").toUpperCase() === "SUCCEEDED") || onlineImageTasks.value[0]);
const onlinePreviewImage = computed(() => onlinePreviewTask.value ? aiTaskImageUrl(onlinePreviewTask.value) : "");
const imageWorkspaceTitle = computed(() => {
  return "AI Image Playground";
});
const onlinePreviewStatus = computed(() => {
  const status = String(onlinePreviewTask.value?.status || "PENDING").toUpperCase();
  if (status === "SUCCEEDED") return { label: "已完成", type: "success" as const };
  if (status === "RUNNING") return { label: "生成中", type: "warning" as const };
  if (status === "FAILED") return { label: "失败", type: "danger" as const };
  return { label: "队列中", type: "info" as const };
});
const onlineHistoryItems = computed(() => onlineImageTasks.value.slice(0, 8));
const userWorkCards = computed(() => {
  const keyword = worksSearchKeyword.value.trim().toLowerCase();
  const statusFilter = worksStatusFilter.value;
  const source = worksSourceTab.value === "official" ? publicOfficialCases.value : onlineImageTasks.value;
  return source
    .filter((task) => {
      const taskId = aiTaskId(task);
      if (taskId && aiHiddenTaskIds.value.includes(taskId)) return false;
      if (statusFilter === "done" && !isAiTaskSucceeded(task)) return false;
      if (statusFilter === "running" && !isAiTaskRunning(task)) return false;
      if (statusFilter === "failed" && !isAiTaskFailed(task)) return false;
      if (statusFilter === "favorite" && !isAiTaskFavorite(task)) return false;
      if (!keyword) return true;
      return [task.name, task.prompt, task.model, task.status, task.id]
        .map((item) => String(item || "").toLowerCase())
        .join(" ")
        .includes(keyword);
    })
    .sort((left, right) => (aiTaskDateMs(right, "updatedAt") || aiTaskDateMs(right, "createdAt")) - (aiTaskDateMs(left, "updatedAt") || aiTaskDateMs(left, "createdAt")));
});
const userWorkSummaryCards = computed(() => {
  if (worksSourceTab.value === "official") {
    return [
      { label: "官方精选", value: String(publicOfficialCases.value.length), hint: "仅包含公开展示案例" },
      { label: "可复用提示", value: String(publicOfficialCases.value.length), hint: "可带入创作区继续编辑" },
      { label: "私有作品", value: isGuestUser.value ? "--" : String(onlineImageTasks.value.length), hint: isGuestUser.value ? "登录后查看" : "切换到我的作品查看" },
      { label: "作品同步", value: isGuestUser.value ? "未登录" : "已开启", hint: "不会公开你的私人作品" }
    ];
  }
  const total = onlineImageTasks.value.filter((task) => !aiHiddenTaskIds.value.includes(aiTaskId(task))).length;
  const done = onlineImageTasks.value.filter(isAiTaskSucceeded).length;
  const favorites = onlineImageTasks.value.filter(isAiTaskFavorite).length;
  const points = onlineImageTasks.value.reduce((sum, task) => sum + Number(task.pointCost || 0), 0);
  return [
    { label: "全部作品", value: String(total), hint: "含图片任务与收藏记录" },
    { label: "已完成", value: String(done), hint: "可预览、下载、复用" },
    { label: "收藏作品", value: String(favorites), hint: "已加入收藏夹" },
    { label: "消耗点数", value: formatNumber(points), hint: "按当前任务汇总" }
  ];
});
const aiGalleryCards = computed(() => {
  const keyword = aiPromptSearch.value.trim().toLowerCase();
  const filter = aiGalleryFilter.value;
  return onlineImageTasks.value.filter((task) => {
    const taskId = String(task.id || task.name || task.prompt || "");
    if (aiHiddenTaskIds.value.includes(taskId)) return false;
    const status = String(task.status || "").toUpperCase();
    const searchable = [task.prompt, task.name, task.model, task.status, task.id].map((item) => String(item || "").toLowerCase()).join(" ");
    if (filter === "done" && status !== "SUCCEEDED") return false;
    if (filter === "running" && !isAiTaskRunning(task)) return false;
    if (filter === "error" && !["FAILED", "ERROR"].includes(status)) return false;
    if (aiActiveFavoriteCollectionId.value) {
      const collectionTaskIds = aiActiveFavoriteCollectionId.value === "all-favorites"
        ? aiFavoriteTaskIds.value
        : aiFavoriteCollections.value.find((item) => item.id === aiActiveFavoriteCollectionId.value)?.taskIds;
      if (!collectionTaskIds?.includes(taskId)) return false;
    }
    if (aiFavoriteOnly.value && !isAiTaskFavorite(task)) return false;
    return !keyword || searchable.includes(keyword);
  });
});
const visibleAiGalleryCards = computed(() => aiGalleryCards.value.slice(0, aiGalleryVisibleCount.value));
const hasMoreAiGalleryCards = computed(() => visibleAiGalleryCards.value.length < aiGalleryCards.value.length);

function loadMoreAiGalleryCards() {
  aiGalleryVisibleCount.value = Math.min(aiGalleryCards.value.length, aiGalleryVisibleCount.value + aiGalleryPageSize);
}

const aiFavoriteCollectionCards = computed(() => {
  const allFavoriteTaskIds = Array.from(new Set(aiFavoriteTaskIds.value));
  return [
    { id: "all-favorites", name: "全部收藏", taskIds: allFavoriteTaskIds, virtual: true },
    ...aiFavoriteCollections.value.map((collection) => ({ ...collection, virtual: false }))
  ].map((collection) => {
    const taskIds = collection.id === "all-favorites" ? allFavoriteTaskIds : collection.taskIds;
    const tasks = onlineImageTasks.value.filter((task) => taskIds.includes(aiTaskId(task)));
    return {
      ...collection,
      taskIds,
      tasks,
      count: taskIds.length,
      imageCount: tasks.filter((task) => aiTaskImageUrl(task)).length
    };
  });
});
const isAiFavoriteCollectionOverview = computed(() => aiPlaygroundMode.value === "gallery" && aiFavoriteOnly.value && !aiActiveFavoriteCollectionId.value);
const aiFailedVisibleTasks = computed(() => aiGalleryCards.value.filter((task) => ["FAILED", "ERROR"].includes(String(task.status || "").toUpperCase())));
const aiDetailTask = computed(() => onlineImageTasks.value.find((task) => aiTaskId(task) === aiDetailTaskId.value));
const aiRawUrlsTask = computed(() => onlineImageTasks.value.find((task) => aiTaskId(task) === aiRawUrlsTaskId.value));
const aiRawResponseTask = computed(() => onlineImageTasks.value.find((task) => aiTaskId(task) === aiRawResponseTaskId.value));
const aiLightboxTask = computed(() => onlineImageTasks.value.find((task) => aiTaskId(task) === aiLightboxTaskId.value));
const aiLightboxTasks = computed(() => aiGalleryCards.value.filter((task) => aiTaskImageUrl(task)));
const aiLightboxIndex = computed(() => aiLightboxTasks.value.findIndex((task) => aiTaskId(task) === aiLightboxTaskId.value));
const aiLightboxTotal = computed(() => aiLightboxTasks.value.length);
const aiLightboxZoomText = computed(() => `${Math.round(aiLightboxScale.value * 100)}%`);
const aiLightboxTransformStyle = computed(() => ({
  transform: `translate3d(${aiLightboxTx.value}px, ${aiLightboxTy.value}px, 0) scale(${aiLightboxScale.value})`
}));
const activeAiAgentConversation = computed(() => aiAgentConversations.value.find((item) => item.id === aiAgentActiveConversationId.value) || aiAgentConversations.value[0]);

watch(
  () => onlineImageData.value.aiState,
  (state) => {
    if (!state || aiStateHydrated.value) return;
    hydrateAiState(state);
  },
  { immediate: true }
);

const onlineQueueCards = computed(() => {
  const queue = onlineImageData.value.queue || {};
  return [
    { label: "排队中", value: String(queue.queued || 0) },
    { label: "生成中", value: String(queue.running || 0) },
    { label: "已完成", value: String(queue.completed || 0) },
    { label: "失败", value: String(queue.failed || 0) }
  ];
});
const filteredOnlineTasks = computed(() => {
  const status = onlineStatusFilter.value;
  if (status === "ALL") return onlineImageTasks.value;
  return onlineImageTasks.value.filter((task) => String(task.status || "").toUpperCase() === status);
});

function providerStatusClass(value: unknown) {
  const status = String(value || "").toUpperCase();
  if (["ACTIVE", "ONLINE"].includes(status)) return "is-online";
  if (["CONFIGURABLE", "PENDING"].includes(status)) return "is-warning";
  return "is-offline";
}

function hydrateAiState(state: UserAIState) {
  aiFavoriteTaskIds.value = Array.isArray(state.favoriteTaskIds) ? [...state.favoriteTaskIds] : [];
  aiHiddenTaskIds.value = Array.isArray(state.hiddenTaskIds) ? [...state.hiddenTaskIds] : [];
  aiFavoriteCollections.value = normalizeAiFavoriteCollections(Array.isArray(state.favoriteCollections) ? state.favoriteCollections : []);
  aiDefaultFavoriteCollectionId.value = state.defaultFavoriteCollectionId || aiFavoriteCollections.value[0]?.id || null;
  if (aiDefaultFavoriteCollectionId.value && !aiFavoriteCollections.value.some((item) => item.id === aiDefaultFavoriteCollectionId.value)) {
    aiDefaultFavoriteCollectionId.value = aiFavoriteCollections.value[0]?.id || null;
  }
  syncAiFavoriteTaskIdsFromCollections();
  aiAgentConversations.value = Array.isArray(state.agentConversations) && state.agentConversations.length
    ? state.agentConversations.map((item) => ({
      id: item.id,
      title: item.title,
      createdAt: item.createdAt,
      updatedAt: item.updatedAt,
      messages: Array.isArray(item.messages) ? item.messages.map((message) => ({
        role: message.role === "user" ? "user" : "assistant",
        content: message.content,
        createdAt: message.createdAt
      })) : []
    }))
    : aiAgentConversations.value;
  aiActiveFavoriteCollectionId.value = state.activeCollectionId || "";
  aiAgentActiveConversationId.value = state.activeConversationId || aiAgentConversations.value[0]?.id || "agent-default";
  aiStateHydrated.value = true;
}

function aiStatePayload(): UserAIState {
  return {
    favoriteTaskIds: aiFavoriteTaskIds.value,
    hiddenTaskIds: aiHiddenTaskIds.value,
    favoriteCollections: aiFavoriteCollections.value,
    defaultFavoriteCollectionId: aiDefaultFavoriteCollectionId.value,
    agentConversations: aiAgentConversations.value,
    activeConversationId: aiAgentActiveConversationId.value,
    activeCollectionId: aiActiveFavoriteCollectionId.value
  };
}

async function saveAiState() {
  if (!aiStateHydrated.value) return;
  if (aiStateSaving.value) {
    aiStateSaveQueued.value = true;
    return;
  }
  aiStateSaving.value = true;
  try {
    const state = await adminRequest<UserAIState>({ method: "PATCH", url: "/user/ai-state", data: aiStatePayload() });
    if (state) {
      aiStateHydrated.value = false;
      hydrateAiState(state);
    }
  } catch (error) {
    ElMessage.warning(error instanceof Error ? `AI 生图状态保存失败：${error.message}` : "AI 生图状态保存失败");
  } finally {
    aiStateSaving.value = false;
    if (aiStateSaveQueued.value) {
      aiStateSaveQueued.value = false;
      void saveAiState();
    }
  }
}

function fitOnlineImageSize() {
  const ratio = onlineImageForm.value.ratio;
  if (ratio === "16:9") {
    onlineImageForm.value.width = 1344;
    onlineImageForm.value.height = 768;
  } else if (ratio === "9:16") {
    onlineImageForm.value.width = 768;
    onlineImageForm.value.height = 1344;
  } else {
    onlineImageForm.value.width = 1024;
    onlineImageForm.value.height = 1024;
  }
  ElMessage.success("已按当前比例适配尺寸");
}

function resolveAiImageSize() {
  const size = onlineImageForm.value.size;
  if (/^\d+x\d+$/.test(size)) {
    const [width, height] = size.split("x").map((item) => Number(item));
    if (width > 0 && height > 0) return { width, height, resolution: size };
  }
  return {
    width: onlineImageForm.value.width,
    height: onlineImageForm.value.height,
    resolution: onlineImageForm.value.resolution
  };
}

function roundToSizeMultiple(value: number, mode: "round" | "floor" | "ceil" = "round") {
  const multiple = 16;
  const method = mode === "floor" ? Math.floor : mode === "ceil" ? Math.ceil : Math.round;
  return Math.max(multiple, method(value / multiple) * multiple);
}

function normalizeAiImageSize(size: string) {
  const match = size.trim().match(/^(\d+)\s*[xX×]\s*(\d+)$/);
  if (!match) return size.trim();
  let width = roundToSizeMultiple(Number(match[1]));
  let height = roundToSizeMultiple(Number(match[2]));
  const scaleToFit = (scale: number) => {
    width = roundToSizeMultiple(width * scale, "floor");
    height = roundToSizeMultiple(height * scale, "floor");
  };
  const scaleToFill = (scale: number) => {
    width = roundToSizeMultiple(width * scale, "ceil");
    height = roundToSizeMultiple(height * scale, "ceil");
  };
  for (let index = 0; index < 4; index += 1) {
    const maxEdge = Math.max(width, height);
    if (maxEdge > 3840) scaleToFit(3840 / maxEdge);
    if (width / height > 3) width = roundToSizeMultiple(height * 3, "floor");
    if (height / width > 3) height = roundToSizeMultiple(width * 3, "floor");
    const pixels = width * height;
    if (pixels > 8294400) scaleToFit(Math.sqrt(8294400 / pixels));
    if (pixels < 655360) scaleToFill(Math.sqrt(655360 / pixels));
  }
  return `${width}x${height}`;
}

function aiRequestSizeParam(size: string) {
  const normalized = normalizeAiImageSize(size);
  if (!normalized || normalized.toLowerCase() === "auto") return "";
  return normalized;
}

function aiRequestQualityParam(value: string) {
  const normalized = normalizeAiImageQuality(value);
  return normalized === "high" ? "high" : "";
}

function parseAiRatio(ratio: string) {
  const match = ratio.trim().match(/^(\d+(?:\.\d+)?)\s*[:xX×]\s*(\d+(?:\.\d+)?)$/);
  if (!match) return null;
  const width = Number(match[1]);
  const height = Number(match[2]);
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return null;
  return { width, height };
}

function calculateAiImageSize(tier: (typeof aiSizeTiers)[number], ratio: string) {
  const parsed = parseAiRatio(ratio);
  if (!parsed) return "";
  const gcd = (a: number, b: number): number => b === 0 ? a : gcd(b, a % b);
  if (Number.isInteger(parsed.width) && Number.isInteger(parsed.height)) {
    const divisor = gcd(parsed.width, parsed.height);
    const key = `${parsed.width / divisor}:${parsed.height / divisor}`;
    if (aiCommonSizePresets[tier][key]) return aiCommonSizePresets[tier][key];
  }
  const targetRatio = parsed.width / parsed.height;
  const pixelBudget = tier === "1K" ? 1572864 : tier === "2K" ? 4194304 : 8294400;
  let bestWidth = 0;
  let bestHeight = 0;
  let bestPixels = 0;
  for (let width = 16; width <= 3840; width += 16) {
    const idealHeight = width / targetRatio;
    const candidates = [Math.floor(idealHeight / 16) * 16, Math.ceil(idealHeight / 16) * 16];
    for (const height of candidates) {
      if (height < 16 || height > 3840) continue;
      const pixels = width * height;
      if (pixels > pixelBudget || pixels < 655360) continue;
      if (Math.max(width / height, height / width) > 3) continue;
      const ratioError = Math.abs(width / height - targetRatio) / targetRatio;
      if (ratioError > 0.01) continue;
      if (pixels > bestPixels) {
        bestPixels = pixels;
        bestWidth = width;
        bestHeight = height;
      }
    }
  }
  return bestPixels ? `${bestWidth}x${bestHeight}` : "";
}

function findAiSizePreset(size: string) {
  const normalized = normalizeAiImageSize(size);
  for (const tier of aiSizeTiers) {
    for (const ratio of aiSizeRatios) {
      if (calculateAiImageSize(tier, ratio.value) === normalized) return { tier, ratio: ratio.value };
    }
  }
  return null;
}

const displayAiImageSize = computed(() => normalizeAiImageSize(onlineImageForm.value.size) || "auto");
const aiSizePickerPreview = computed(() => {
  if (aiSizePickerMode.value === "auto") return "auto";
  if (aiSizePickerMode.value === "ratio") {
    const activeRatio = aiSizeRatio.value === "custom" ? aiCustomRatio.value : aiSizeRatio.value;
    return normalizeAiImageSize(calculateAiImageSize(aiSizeTier.value, activeRatio));
  }
  const width = Number(aiCustomWidth.value);
  const height = Number(aiCustomHeight.value);
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return "";
  return normalizeAiImageSize(`${width}x${height}`);
});
const aiSizePickerClamped = computed(() => {
  if (aiSizePickerMode.value !== "resolution" || !aiSizePickerPreview.value) return false;
  return `${Number(aiCustomWidth.value)}x${Number(aiCustomHeight.value)}` !== aiSizePickerPreview.value;
});

function ratioIconStyle(ratio: string) {
  const parsed = parseAiRatio(ratio);
  if (!parsed) return { width: "22px", height: "16px" };
  const horizontal = parsed.width >= parsed.height;
  const width = horizontal ? 24 : Math.max(10, Math.round(24 * parsed.width / parsed.height));
  const height = horizontal ? Math.max(10, Math.round(24 * parsed.height / parsed.width)) : 24;
  return { width: `${width}px`, height: `${height}px` };
}

async function loadAiImageModuleSchema(force = false) {
  const modelName = String(onlineImageForm.value.model || "").trim();
  const schemaKey = modelName || "default";
  if (!force && aiImageModuleSchemaKey.value === schemaKey && aiImageModuleSchema.value) return;
  aiImageModuleSchemaLoading.value = true;
  try {
    const params = new URLSearchParams({ module_code: "image_generation" });
    if (modelName) params.set("model_name", modelName);
    aiImageModuleSchema.value = await adminRequest<AdminRecord>({ method: "GET", url: `/module-schema?${params.toString()}` });
    aiImageModuleSchemaKey.value = schemaKey;
  } catch (error) {
    aiImageModuleSchema.value = null;
    aiImageModuleSchemaKey.value = "";
    if (aiSizePickerVisible.value) {
      ElMessage.warning(error instanceof Error ? `图片尺寸 Schema 读取失败：${error.message}` : "图片尺寸 Schema 读取失败");
    }
  } finally {
    aiImageModuleSchemaLoading.value = false;
  }
}

function selectAiSchemaSizeOption(size: string) {
  const normalized = normalizeAiImageSize(size);
  if (!normalized || normalized.toLowerCase() === "auto") {
    aiSizePickerMode.value = "auto";
    return;
  }
  const preset = findAiSizePreset(normalized);
  const parsed = normalized.match(/^(\d+)x(\d+)$/);
  if (preset) {
    aiSizePickerMode.value = "ratio";
    aiSizeTier.value = preset.tier;
    aiSizeRatio.value = preset.ratio;
    return;
  }
  if (parsed) {
    aiSizePickerMode.value = "resolution";
    aiCustomWidth.value = parsed[1];
    aiCustomHeight.value = parsed[2];
  }
}

function isAiSchemaSizeOptionActive(size: string) {
  const normalized = normalizeAiImageSize(size);
  return normalized && normalized === aiSizePickerPreview.value;
}

function openAiSizePicker() {
  const currentSize = onlineImageForm.value.size;
  if (!currentSize || currentSize === "auto") {
    aiSizePickerMode.value = "auto";
  } else {
    const preset = findAiSizePreset(currentSize);
    const parsed = currentSize.match(/^(\d+)\s*[xX×]\s*(\d+)$/);
    if (preset) {
      aiSizePickerMode.value = "ratio";
      aiSizeTier.value = preset.tier;
      aiSizeRatio.value = preset.ratio;
    } else if (parsed) {
      aiSizePickerMode.value = "resolution";
      aiCustomWidth.value = parsed[1];
      aiCustomHeight.value = parsed[2];
    } else {
      aiSizePickerMode.value = "ratio";
    }
  }
  aiSizePickerVisible.value = true;
  void loadAiImageModuleSchema(true);
}

function closeAiSizePicker() {
  aiSizePickerVisible.value = false;
}

function handleAiSizePickerBackdropDown(event: MouseEvent) {
  aiSizePickerMouseDownTarget.value = event.target;
}

function handleAiSizePickerBackdropUp(event: MouseEvent) {
  const modal = aiSizePickerRef.value;
  const downTarget = aiSizePickerMouseDownTarget.value;
  aiSizePickerMouseDownTarget.value = null;
  if (!modal || !downTarget) return;
  if (!modal.contains(downTarget as Node) && !modal.contains(event.target as Node)) closeAiSizePicker();
}

function applyAiSizePicker() {
  if (!aiSizePickerPreview.value) return;
  onlineImageForm.value.size = aiSizePickerPreview.value;
  if (aiSizePickerPreview.value !== "auto") {
    const [width, height] = aiSizePickerPreview.value.split("x").map((item) => Number(item));
    onlineImageForm.value.width = width;
    onlineImageForm.value.height = height;
    onlineImageForm.value.resolution = aiSizeTier.value.toLowerCase();
  }
  closeAiSizePicker();
}

function runOnlineResultAction(action: "download" | "reuse" | "works" | "canvas") {
  if (action === "canvas") {
    selectAdminModule("userWorks");
    return;
  }
  if (action === "works") {
    selectAdminModule("userWorks");
    return;
  }
  if (action === "reuse") {
    const prompt = String(onlinePreviewTask.value?.prompt || onlinePreviewTask.value?.name || "");
    if (prompt) onlineImageForm.value.prompt = prompt;
    ElMessage.success("已复用当前风格提示词");
    return;
  }
  ElMessage.info("生成完成后可下载原图");
}

function aiTaskId(task: AdminRecord) {
  return String(task.id || task.name || task.prompt || "");
}

function aiTaskAsset(task: AdminRecord) {
  const taskId = aiTaskId(task);
  const resultIds = Array.isArray(task.resultIds) ? task.resultIds.map((item) => String(item)) : [];
  return onlineAssets.value.find((item) => {
    const assetId = String(item.id || "");
    const assetTaskId = String(item.taskId || "");
    return (taskId && assetTaskId === taskId) || (assetId && resultIds.includes(assetId));
  });
}

function aiTaskOutputItems(task: AdminRecord) {
  const taskId = aiTaskId(task);
  const resultIds = Array.isArray(task.resultIds) ? task.resultIds.map((item) => String(item)) : [];
  const seen = new Set<string>();
  const items: Array<{ id: string; url: string; name: string; assetId?: string }> = [];
  const addItem = (id: string, url: string, name: string, assetId = "") => {
    const key = url || id;
    if (!key || seen.has(key)) return;
    seen.add(key);
    items.push({ id, url, name, ...(assetId ? { assetId } : {}) });
  };

  onlineAssets.value.forEach((asset) => {
    const assetId = String(asset.id || "");
    const assetTaskId = String(asset.taskId || "");
    const matchesTask = taskId && assetTaskId === taskId;
    const matchesResult = assetId && resultIds.includes(assetId);
    if (!matchesTask && !matchesResult) return;
    const url = String(asset.url || asset.imageUrl || asset.outputUrl || asset.resultUrl || asset.thumbnailUrl || "");
    addItem(assetId || `${taskId}-${items.length + 1}`, url, String(asset.name || task.name || task.prompt || "AI 图片"), assetId);
  });

  const directUrl = String(task.outputUrl || task.resultUrl || task.imageUrl || task.thumbnailUrl || "");
  addItem(taskId || `${Date.now()}`, directUrl, String(task.name || task.prompt || "AI 图片"));
  return items.filter((item) => item.url);
}

function aiTaskOriginalCacheId(task: AdminRecord) {
  const asset = aiTaskAsset(task);
  return String(asset?.id || task.id || task.resultUrl || task.outputUrl || task.imageUrl || "");
}

function aiTaskImageUrl(task: AdminRecord) {
  const directUrl = String(task.outputUrl || task.resultUrl || task.imageUrl || "");
  if (directUrl) return directUrl;
  const firstOutput = aiTaskOutputItems(task)[0];
  if (firstOutput?.url) return firstOutput.url;
  const asset = aiTaskAsset(task);
  return String(asset?.url || asset?.imageUrl || asset?.outputUrl || asset?.resultUrl || task.thumbnailUrl || asset?.thumbnailUrl || "");
}

function aiTaskThumbnailUrl(task: AdminRecord) {
  const directUrl = String(task.thumbnailUrl || "");
  if (directUrl) return directUrl;
  const asset = aiTaskAsset(task);
  return String(asset?.thumbnailUrl || asset?.url || task.imageUrl || task.outputUrl || task.resultUrl || "");
}

function aiTaskThumbnailKey(task: AdminRecord) {
  const url = aiTaskThumbnailUrl(task);
  if (!url) return "";
  return `${aiTaskId(task) || task.id || task.name || "task"}::${url}`;
}

function isAiTaskThumbnailBroken(task: AdminRecord) {
  const key = aiTaskThumbnailKey(task);
  return Boolean(key && aiBrokenThumbnailKeys.value.includes(key));
}

function markAiTaskThumbnailFailed(task: AdminRecord) {
  const key = aiTaskThumbnailKey(task);
  if (!key || aiBrokenThumbnailKeys.value.includes(key)) return;
  aiBrokenThumbnailKeys.value = [...aiBrokenThumbnailKeys.value, key];
}

function aiTaskDisplayImageUrl(task: AdminRecord) {
  const cacheId = aiTaskOriginalCacheId(task);
  return (cacheId && aiOriginalImageCache.value[cacheId]) || aiTaskImageUrl(task);
}

function aiTaskRawImageUrls(task: AdminRecord) {
  const urls = new Set<string>();
  const addUrl = (value: unknown) => {
    const url = String(value || "").trim();
    if (url) urls.add(url);
  };
  aiTaskOutputItems(task).forEach((item) => addUrl(item.url));
  addUrl(task.outputUrl);
  addUrl(task.resultUrl);
  addUrl(task.imageUrl);
  addUrl(task.thumbnailUrl);
  const rawUrls = (task as Record<string, unknown>).rawImageUrls;
  if (Array.isArray(rawUrls)) rawUrls.forEach(addUrl);
  return Array.from(urls);
}

function sanitizedAiRawValue(value: unknown, depth = 0): unknown {
  if (depth > 5) return "[Object]";
  if (typeof value === "string") {
    if (value.startsWith("data:image/")) return `[data image omitted, ${value.length} chars]`;
    if (value.length > 1200) return `${value.slice(0, 1200)}... [truncated ${value.length - 1200} chars]`;
    return value;
  }
  if (Array.isArray(value)) return value.map((item) => sanitizedAiRawValue(item, depth + 1));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value as Record<string, unknown>).map(([key, item]) => [key, sanitizedAiRawValue(item, depth + 1)]));
  }
  return value;
}

function aiTaskRawResponseText(task: AdminRecord) {
  const payload = {
    task: sanitizedAiRawValue(task),
    outputs: aiTaskOutputItems(task),
    rawImageUrls: aiTaskRawImageUrls(task),
    params: sanitizedAiRawValue(aiTaskParams(task))
  };
  return JSON.stringify(payload, null, 2);
}

function openAiRawUrlsModal(task: AdminRecord) {
  aiRawUrlsTaskId.value = aiTaskId(task);
}

function openAiRawResponseModal(task: AdminRecord) {
  aiRawResponseTaskId.value = aiTaskId(task);
}

function closeAiRawModals() {
  aiRawUrlsTaskId.value = "";
  aiRawResponseTaskId.value = "";
}

function blobToDataUrl(blob: Blob) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(reader.error || new Error("读取图片失败"));
    reader.readAsDataURL(blob);
  });
}

async function fetchAiOriginalImageDataUrl(task: AdminRecord) {
  const asset = aiTaskAsset(task);
  const assetId = String(asset?.id || "");
  const directUrl = aiTaskImageUrl(task);
  if (!directUrl) return "";
  if (directUrl.startsWith("data:image/")) return directUrl;
  let blob: Blob;
  if (assetId) {
    blob = await downloadAssetBlob(assetId);
  } else {
    blob = await fetchResourceBlob(directUrl, { auth: false });
  }
  return blobToDataUrl(blob);
}

async function ensureAiOriginalImageCached(task: AdminRecord) {
  const cacheId = aiTaskOriginalCacheId(task);
  if (!cacheId || aiOriginalImageCache.value[cacheId] || aiOriginalImageCachePending.has(cacheId)) return;
  aiOriginalImageCachePending.add(cacheId);
  try {
    const sourceUrl = aiTaskImageUrl(task);
    const cached = await readCachedOriginalImage(cacheId);
    if (cached?.dataUrl && cached.version === 2 && cached.sourceUrl === sourceUrl) {
      aiOriginalImageCache.value = { ...aiOriginalImageCache.value, [cacheId]: cached.dataUrl };
      return;
    }
    const dataUrl = await fetchAiOriginalImageDataUrl(task);
    if (!dataUrl) return;
    aiOriginalImageCache.value = { ...aiOriginalImageCache.value, [cacheId]: dataUrl };
    await writeCachedOriginalImage({ id: cacheId, dataUrl, sourceUrl, version: 2 });
  } catch (error) {
    console.warn("AI original image cache skipped", error);
  } finally {
    aiOriginalImageCachePending.delete(cacheId);
  }
}

function prefetchAiOriginalImage(task: AdminRecord) {
  if (typeof window === "undefined") return;
  const imageUrl = aiTaskImageUrl(task);
  const cacheId = aiTaskOriginalCacheId(task);
  if (!imageUrl || imageUrl.startsWith("data:image/") || !cacheId || aiOriginalImageCache.value[cacheId] || aiOriginalImageCachePending.has(cacheId)) return;
  if (aiOriginalImagePrefetchTimer) window.clearTimeout(aiOriginalImagePrefetchTimer);
  aiOriginalImagePrefetchTimer = window.setTimeout(() => {
    aiOriginalImagePrefetchTimer = null;
    if (aiOriginalImageCachePending.size >= 2) return;
    void ensureAiOriginalImageCached(task);
  }, 360);
}

function aiTaskParams(task: AdminRecord) {
  return (task.params && typeof task.params === "object" ? task.params : {}) as Record<string, unknown>;
}

function aiTaskStatus(task: AdminRecord) {
  return String(task.status || "PENDING").toUpperCase();
}

function isAiTaskSucceeded(task: AdminRecord) {
  return ["SUCCEEDED", "COMPLETED", "DONE"].includes(aiTaskStatus(task));
}

function isAiTaskRunning(task: AdminRecord) {
  const status = aiTaskStatus(task);
  if (["PENDING", "RUNNING", "QUEUED", "PROCESSING"].includes(status)) return true;
  if (["SUCCEEDED", "COMPLETED", "DONE", "FAILED", "ERROR", "CANCELED", "CANCELLED"].includes(status)) return false;
  return !isAiTaskFailed(task) && !aiTaskImageUrl(task);
}

function isAiTaskFailed(task: AdminRecord) {
  return ["FAILED", "ERROR"].includes(aiTaskStatus(task));
}

function aiTaskStatusClass(task: AdminRecord) {
  const status = aiTaskStatus(task);
  return {
    "is-running": isAiTaskRunning(task),
    "is-done": isAiTaskSucceeded(task),
    "is-failed": ["FAILED", "ERROR"].includes(status),
    "is-favorite": isAiTaskFavorite(task)
  };
}

function clearAiGenerationPolling() {
  if (aiGenerationPollTimer && typeof window !== "undefined") {
    window.clearTimeout(aiGenerationPollTimer);
  }
  aiGenerationPollTimer = null;
}

function stopAiGenerationPolling() {
  clearAiGenerationPolling();
  aiTrackedGenerationTaskIds.value = [];
  aiGenerationPollAttempts.clear();
}

function trackAiGenerationTask(taskId: string) {
  const id = String(taskId || "").trim();
  if (!id || id.startsWith("optimistic_")) return;
  if (!aiTrackedGenerationTaskIds.value.includes(id)) {
    aiTrackedGenerationTaskIds.value = [id, ...aiTrackedGenerationTaskIds.value];
  }
  if (!aiGenerationPollAttempts.has(id)) {
    aiGenerationPollAttempts.set(id, 0);
  }
  scheduleAiGenerationPolling(0);
}

function scheduleAiGenerationPolling(delayMs?: number) {
  if (typeof window === "undefined") return;
  if (aiGenerationPollTimer) return;
  if (!aiTrackedGenerationTaskIds.value.length) return;
  const maxAttempt = Math.max(0, ...aiTrackedGenerationTaskIds.value.map((id) => aiGenerationPollAttempts.get(id) || 0));
  const nextDelay = delayMs ?? aiGenerationPollDelaysMs[Math.min(maxAttempt, aiGenerationPollDelaysMs.length - 1)];
  aiGenerationPollTimer = window.setTimeout(() => {
    aiGenerationPollTimer = null;
    void pollAiGenerationTasksOnce();
  }, nextDelay);
}

async function pollAiGenerationTasksOnce() {
  const taskIds = [...aiTrackedGenerationTaskIds.value];
  if (!taskIds.length) {
    clearAiGenerationPolling();
    return;
  }
  let shouldRefreshWorkspace = false;
  try {
    const polledTasks = await Promise.all(taskIds.map(async (taskId) => {
      aiGenerationPollAttempts.set(taskId, (aiGenerationPollAttempts.get(taskId) || 0) + 1);
      try {
        return await adminRequest<AdminRecord>({ method: "GET", url: `/generation-tasks/${encodeURIComponent(taskId)}` });
      } catch (error) {
        console.warn("AI generation task polling skipped", taskId, error);
        return null;
      }
    }));
    const completedTaskIds: string[] = [];
    polledTasks.forEach((task) => {
      if (!task) return;
      mergeAiGenerationTask(task);
      if (!isAiTaskRunning(task)) {
        completedTaskIds.push(aiTaskId(task));
        shouldRefreshWorkspace = true;
      }
    });
    if (completedTaskIds.length) {
      aiTrackedGenerationTaskIds.value = aiTrackedGenerationTaskIds.value.filter((id) => !completedTaskIds.includes(id));
      completedTaskIds.forEach((id) => aiGenerationPollAttempts.delete(id));
    }
    aiTrackedGenerationTaskIds.value = aiTrackedGenerationTaskIds.value.filter((id) => (aiGenerationPollAttempts.get(id) || 0) < aiGenerationPollMaxAttempts);
    const serverTasks = Array.isArray(onlineImageData.value.recentTasks) ? onlineImageData.value.recentTasks : [];
    aiOptimisticTasks.value = aiOptimisticTasks.value.filter((task) => !isOptimisticAiTaskReconciled(task, serverTasks));
  } catch (error) {
    console.warn("AI generation polling skipped", error);
  }
  if (shouldRefreshWorkspace) {
    void store.loadActiveModule({ preferCache: false, silent: true }).catch((error) => {
      console.warn("AI generation completion refresh skipped", error);
    });
  }
  if (!aiTrackedGenerationTaskIds.value.length) {
    clearAiGenerationPolling();
    return;
  }
  scheduleAiGenerationPolling();
}

function mergeAiGenerationTask(task: AdminRecord) {
  const taskId = aiTaskId(task);
  if (!taskId) return;
  const currentData = store.data as AdminRecord;
  const currentTasks = Array.isArray(currentData.recentTasks) ? currentData.recentTasks : [];
  let found = false;
  const nextTasks = currentTasks.map((item) => {
    if (aiTaskId(item) !== taskId) return item;
    found = true;
    return { ...item, ...task };
  });
  if (!found) {
    nextTasks.unshift(task);
  }
  store.data = { ...currentData, recentTasks: nextTasks };
}

function refreshAiGenerationTasksFromServer() {
  if (!usesAiImageWorkspace.value || !hasAuthToken()) return;
  if (!aiTrackedGenerationTaskIds.value.length) return;
  clearAiGenerationPolling();
  scheduleAiGenerationPolling(0);
}

function handleAiWorkspaceVisibilityRefresh() {
  if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
  refreshAiGenerationTasksFromServer();
}

function aiTaskModelLabel(task: AdminRecord) {
  const params = aiTaskParams(task);
  return String(task.model || params.model || onlineImageForm.value.model || "gpt-image-2");
}

function aiTaskErrorMessage(task: AdminRecord) {
  const failureReason = String(task.failureReason || task.failure_reason || "").trim();
  if (failureReason) return failureReason;
  const error = task.error;
  if (typeof error === "string") return error;
  if (error && typeof error === "object" && "message" in error) return String((error as { message?: unknown }).message || "生成失败");
  return "生成失败，可复用配置后重试";
}

function aiTaskDateMs(task: AdminRecord, key: "createdAt" | "updatedAt") {
  const raw = task[key];
  if (!raw) return 0;
  const value = new Date(String(raw)).getTime();
  return Number.isFinite(value) ? value : 0;
}

function formatAiTaskDuration(ms: number) {
  const safeMs = Math.max(0, ms);
  const totalSeconds = Math.floor(safeMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes >= 60) {
    const hours = Math.floor(minutes / 60);
    const remainMinutes = minutes % 60;
    return `${hours}:${String(remainMinutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

function aiTaskDuration(task: AdminRecord) {
  const startedAt = aiTaskDateMs(task, "createdAt") || aiTaskClockNow.value;
  const endedAt = isAiTaskRunning(task) ? aiTaskClockNow.value : aiTaskDateMs(task, "updatedAt") || aiTaskClockNow.value;
  return formatAiTaskDuration(endedAt - startedAt);
}

function formatAiTaskTime(task: AdminRecord) {
  const createdAt = aiTaskDateMs(task, "createdAt");
  if (!createdAt) return "-";
  const date = new Date(createdAt);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function aiTaskSizeValue(task: AdminRecord) {
  const params = aiTaskParams(task);
  const width = Number(params.width || task.width);
  const height = Number(params.height || task.height);
  const explicitSize = String(params.size || params.outputSize || params.actualSize || task.size || "").trim();
  if (explicitSize && explicitSize !== "undefined") return explicitSize;
  if (Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0) return `${width}x${height}`;
  return "auto";
}

function parseAiTaskSize(size: string) {
  const match = size.match(/^(\d+)\s*[xX×]\s*(\d+)$/);
  if (!match) return null;
  const width = Number(match[1]);
  const height = Number(match[2]);
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return null;
  return { width, height };
}

function formatAiImageRatio(width: number, height: number) {
  const gcd = (a: number, b: number): number => b === 0 ? a : gcd(b, a % b);
  const divisor = gcd(width, height);
  return `≈${width / divisor}:${height / divisor}`;
}

function aiTaskRatioLabel(task: AdminRecord) {
  const params = aiTaskParams(task);
  const size = parseAiTaskSize(aiTaskSizeValue(task));
  if (!size) return String(params.imageRatio || params.ratio || onlineImageForm.value.ratio || "auto");
  return formatAiImageRatio(size.width, size.height);
}

function aiTaskResolutionLabel(task: AdminRecord) {
  return aiTaskSizeValue(task);
}

function aiTaskImageMeta(task: AdminRecord) {
  return aiDetailImageMeta.value[aiTaskId(task)];
}

function aiTaskDisplayResolutionLabel(task: AdminRecord) {
  const meta = aiTaskImageMeta(task);
  if (meta) return `${meta.width}×${meta.height}`;
  return aiTaskResolutionLabel(task).replace(/x/g, "×");
}

function aiTaskDisplayRatioLabel(task: AdminRecord) {
  const meta = aiTaskImageMeta(task);
  if (meta) return formatAiImageRatio(meta.width, meta.height);
  return aiTaskRatioLabel(task);
}

function aiTaskDetailSizeLabel(task: AdminRecord) {
  const requestedSize = aiTaskSizeValue(task);
  const actualSize = aiTaskDisplayResolutionLabel(task);
  if (!actualSize || actualSize === requestedSize || actualSize.replace(/×/g, "x") === requestedSize) return requestedSize.replace(/x/g, "×");
  return `${requestedSize} | ${actualSize}`;
}

function handleAiDetailImageLoad(event: Event, task: AdminRecord) {
  const image = event.target as HTMLImageElement | null;
  if (!image || image.naturalWidth <= 0 || image.naturalHeight <= 0) return;
  aiDetailImageMeta.value = {
    ...aiDetailImageMeta.value,
    [aiTaskId(task)]: {
      width: image.naturalWidth,
      height: image.naturalHeight
    }
  };
}

function aiTaskParamValue(task: AdminRecord, key: string, fallback = "auto") {
  const params = aiTaskParams(task);
  const value = params[key] ?? task[key];
  if (value === undefined || value === null || value === "") return fallback;
  return String(value);
}

async function copyToClipboard(text: string) {
  if (!text.trim()) {
    ElMessage.warning("暂无可复制内容");
    return;
  }
  await navigator.clipboard?.writeText(text);
  ElMessage.success("已复制");
}

function isAiTaskFavorite(task: AdminRecord) {
  const id = aiTaskId(task);
  return Boolean(task.favorite || (id && aiFavoriteTaskIds.value.includes(id)));
}

function normalizeAiFavoriteCollections(collections: AiFavoriteCollection[]) {
  const now = new Date().toISOString();
  const seen = new Set<string>();
  const normalized = collections
    .map((collection, index) => {
      const id = String(collection.id || `collection-${index + 1}`).trim();
      const rawName = String(collection.name || "未命名收藏夹").trim();
      const name = id === "default" && rawName === "默认收藏夹" ? "默认" : rawName;
      if (!id || seen.has(id)) return null;
      seen.add(id);
      return {
        id,
        name,
        taskIds: Array.from(new Set((collection.taskIds || []).map(String).filter(Boolean))),
        createdAt: collection.createdAt || now,
        updatedAt: collection.updatedAt || now
      };
    })
    .filter(Boolean) as AiFavoriteCollection[];
  return normalized.length ? normalized : [{ id: "default", name: "默认", taskIds: [], createdAt: now, updatedAt: now }];
}

function currentDefaultFavoriteCollectionId() {
  if (aiDefaultFavoriteCollectionId.value && aiFavoriteCollections.value.some((item) => item.id === aiDefaultFavoriteCollectionId.value)) return aiDefaultFavoriteCollectionId.value;
  return aiFavoriteCollections.value[0]?.id || "default";
}

function handleAiFavoriteFilterClick() {
  if (aiActiveFavoriteCollectionId.value) {
    aiActiveFavoriteCollectionId.value = "";
    clearAiTaskSelection();
    void saveAiState();
    return;
  }
  aiFavoriteOnly.value = !aiFavoriteOnly.value;
  clearAiTaskSelection();
  aiSelectedFavoriteCollectionIds.value = [];
  if (!aiFavoriteOnly.value) aiActiveFavoriteCollectionId.value = "";
}

function toggleAiTaskFavorite(task: AdminRecord) {
  openAiFavoritePicker([aiTaskId(task)]);
}

function sameStringSet(left: string[], right: string[]) {
  if (left.length !== right.length) return false;
  const values = new Set(right);
  return left.every((item) => values.has(item));
}

function taskFavoriteCollectionIds(taskId: string) {
  return aiFavoriteCollections.value.filter((collection) => collection.taskIds.includes(taskId)).map((collection) => collection.id);
}

function initialAiFavoritePickerCheckedIds(taskIds: string[]) {
  const ids = taskIds.filter(Boolean);
  const defaultId = currentDefaultFavoriteCollectionId();
  if (!ids.length) return defaultId ? [defaultId] : [];
  const collectionIdSets = ids.map(taskFavoriteCollectionIds);
  const hasFavorite = collectionIdSets.some((items) => items.length > 0);
  if (!hasFavorite) return defaultId ? [defaultId] : [];
  const first = collectionIdSets[0] || [];
  return collectionIdSets.every((items) => sameStringSet(items, first)) ? first : [];
}

function openAiFavoritePicker(taskIds: string[]) {
  const ids = Array.from(new Set(taskIds.filter(Boolean)));
  if (!ids.length) return;
  if (!ensureWorkspaceAuth("save_work", "userAiImage", { taskIds: ids })) return;
  aiFavoritePickerTaskIds.value = ids;
  aiFavoritePickerCheckedIds.value = initialAiFavoritePickerCheckedIds(ids);
  aiNewCollectionName.value = "";
  cancelRenameAiFavoriteCollection();
  aiFavoritePickerVisible.value = true;
}

function closeAiFavoritePicker() {
  aiFavoritePickerVisible.value = false;
  aiFavoritePickerTaskIds.value = [];
  aiFavoritePickerCheckedIds.value = [];
  aiNewCollectionName.value = "";
  cancelRenameAiFavoriteCollection();
}

function toggleAiFavoritePickerCollection(id: string) {
  aiFavoritePickerCheckedIds.value = aiFavoritePickerCheckedIds.value.includes(id)
    ? aiFavoritePickerCheckedIds.value.filter((item) => item !== id)
    : [...aiFavoritePickerCheckedIds.value, id];
}

function selectAllAiFavoritePickerCollections() {
  aiFavoritePickerCheckedIds.value = aiFavoriteCollections.value.map((collection) => collection.id);
}

function clearAiFavoritePickerCollections() {
  aiFavoritePickerCheckedIds.value = [];
}

function resetAiFavoritePickerDrag() {
  aiFavoritePickerDraggedId.value = "";
  aiFavoritePickerDragOverId.value = "";
  aiFavoritePickerDropPosition.value = "";
}

function startAiFavoritePickerDrag(event: DragEvent, id: string) {
  if (!id || aiEditingCollectionId.value === id) return;
  aiFavoritePickerDraggedId.value = id;
  event.dataTransfer?.setData("text/plain", id);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
}

function updateAiFavoritePickerDragOver(event: DragEvent, targetId: string) {
  const draggedId = aiFavoritePickerDraggedId.value || event.dataTransfer?.getData("text/plain") || "";
  if (!draggedId || draggedId === targetId) return;
  const target = event.currentTarget as HTMLElement | null;
  if (!target) return;
  const rect = target.getBoundingClientRect();
  aiFavoritePickerDragOverId.value = targetId;
  aiFavoritePickerDropPosition.value = event.clientY < rect.top + rect.height / 2 ? "before" : "after";
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
}

function dropAiFavoritePickerCollection(targetId: string) {
  const sourceId = aiFavoritePickerDraggedId.value;
  const position = aiFavoritePickerDropPosition.value || "before";
  if (!sourceId || !targetId || sourceId === targetId) {
    resetAiFavoritePickerDrag();
    return;
  }
  const sourceIndex = aiFavoriteCollections.value.findIndex((collection) => collection.id === sourceId);
  const targetIndex = aiFavoriteCollections.value.findIndex((collection) => collection.id === targetId);
  if (sourceIndex < 0 || targetIndex < 0) {
    resetAiFavoritePickerDrag();
    return;
  }
  const next = [...aiFavoriteCollections.value];
  const [moved] = next.splice(sourceIndex, 1);
  let insertIndex = targetIndex;
  if (position === "after") insertIndex += 1;
  if (sourceIndex < targetIndex) insertIndex -= 1;
  next.splice(insertIndex, 0, moved);
  aiFavoriteCollections.value = next;
  resetAiFavoritePickerDrag();
  void saveAiState();
}

function syncAiFavoriteTaskIdsFromCollections() {
  aiFavoriteTaskIds.value = Array.from(new Set(aiFavoriteCollections.value.flatMap((collection) => collection.taskIds)));
}

function confirmAiFavoritePicker() {
  const taskIds = aiFavoritePickerTaskIds.value.filter(Boolean);
  const checked = new Set(aiFavoritePickerCheckedIds.value);
  const now = new Date().toISOString();
  aiFavoriteCollections.value = aiFavoriteCollections.value.map((collection) => {
    const nextTaskIds = checked.has(collection.id)
      ? Array.from(new Set([...collection.taskIds, ...taskIds]))
      : collection.taskIds.filter((taskId) => !taskIds.includes(taskId));
    return { ...collection, taskIds: nextTaskIds, updatedAt: now };
  });
  syncAiFavoriteTaskIdsFromCollections();
  aiPlaygroundMessage("success", aiFavoritePickerCheckedIds.value.length ? "已保存到收藏夹" : "已从收藏夹移除");
  closeAiFavoritePicker();
  void saveAiState();
}

function legacyToggleAiTaskFavorite(task: AdminRecord) {
  const id = aiTaskId(task);
  if (!id) return;
  if (aiFavoriteTaskIds.value.includes(id)) {
    aiFavoriteTaskIds.value = aiFavoriteTaskIds.value.filter((item) => item !== id);
    aiFavoriteCollections.value = aiFavoriteCollections.value.map((collection) => ({ ...collection, taskIds: collection.taskIds.filter((taskId) => taskId !== id) }));
    ElMessage.success("已取消收藏");
  } else {
    const defaultId = currentDefaultFavoriteCollectionId();
    aiFavoriteTaskIds.value = [...aiFavoriteTaskIds.value, id];
    aiFavoriteCollections.value = aiFavoriteCollections.value.map((collection) => collection.id === defaultId ? { ...collection, taskIds: Array.from(new Set([...collection.taskIds, id])), updatedAt: new Date().toISOString() } : collection);
    ElMessage.success("已加入收藏");
  }
  void saveAiState();
}

function toggleAiTaskSelection(task: AdminRecord) {
  const id = aiTaskId(task);
  if (!id) return;
  aiSelectedTaskIds.value = aiSelectedTaskIds.value.includes(id)
    ? aiSelectedTaskIds.value.filter((item) => item !== id)
    : [...aiSelectedTaskIds.value, id];
  if (!aiSelectedTaskIds.value.length) aiMobileSelectionMode.value = false;
}

function clearAiTaskSelection() {
  aiSelectedTaskIds.value = [];
  aiMobileSelectionMode.value = false;
}

function clearAiTaskLongPressTimer() {
  if (aiTaskLongPressTimer && typeof window !== "undefined") {
    window.clearTimeout(aiTaskLongPressTimer);
    aiTaskLongPressTimer = null;
  }
}

function enterAiMobileSelection(task: AdminRecord) {
  clearAiTaskLongPressTimer();
  aiMobileSelectionMode.value = true;
  aiTouchSelectionTriggered.value = true;
  const id = aiTaskId(task);
  if (id && !aiSelectedTaskIds.value.includes(id)) {
    aiSelectedTaskIds.value = [...aiSelectedTaskIds.value, id];
  }
}

function handleAiTaskTouchStart(task: AdminRecord, event: TouchEvent) {
  if (aiMobileSelectionMode.value || event.touches.length !== 1 || typeof window === "undefined") return;
  aiTouchSelectionTriggered.value = false;
  clearAiTaskLongPressTimer();
  aiTaskLongPressTimer = window.setTimeout(() => {
    enterAiMobileSelection(task);
  }, 520);
}

function handleAiTaskTouchMove() {
  clearAiTaskLongPressTimer();
}

function handleAiTaskTouchEnd() {
  clearAiTaskLongPressTimer();
}

function handleAiTaskCardClick(task: AdminRecord, event: MouseEvent) {
  if (aiTouchSelectionTriggered.value) {
    event.preventDefault();
    aiTouchSelectionTriggered.value = false;
    return;
  }
  const isMultiSelect = event.ctrlKey || event.metaKey || aiMobileSelectionMode.value;
  if (isMultiSelect) {
    event.preventDefault();
    toggleAiTaskSelection(task);
    return;
  }
  previewAiTask(task);
}

function selectAllAiVisibleTasks() {
  aiSelectedTaskIds.value = Array.from(new Set([...aiSelectedTaskIds.value, ...visibleAiGalleryCards.value.map(aiTaskId).filter(Boolean)]));
}

function invertAiVisibleTaskSelection() {
  const visibleIds = visibleAiGalleryCards.value.map(aiTaskId).filter(Boolean);
  const next = aiSelectedTaskIds.value.filter((id) => !visibleIds.includes(id));
  visibleIds.forEach((id) => {
    if (!aiSelectedTaskIds.value.includes(id)) next.push(id);
  });
  aiSelectedTaskIds.value = next;
}

function normalizeAiReferenceUrl(url: string) {
  const value = String(url || "").trim();
  if (!value || value.startsWith("data:image/") || value.startsWith("blob:") || value.startsWith("http://") || value.startsWith("https://")) return value;
  if (value.startsWith("/") && typeof window !== "undefined") return new URL(value, window.location.origin).href;
  return value;
}

function isHeavyAiImageDataUrl(url: string) {
  return url.startsWith("data:image/") && url.length > 180_000;
}

function aiReferencePreviewUrl(item: AiReferenceImage) {
  const previewUrl = normalizeAiReferenceUrl(item.previewUrl || item.url);
  if (isHeavyAiImageDataUrl(previewUrl)) return "";
  return previewUrl;
}

function aiReferenceSource(item: AiReferenceImage): AiGenerationReference["source"] {
  if (item.id.startsWith("task-output-") || item.id.startsWith("context-")) return "task";
  if (item.id.startsWith("draft-")) return "draft";
  if (item.name.includes("粘贴图片")) return "paste";
  if (item.file) return "upload";
  return "local";
}

function createAiTaskReferenceImage(task: AdminRecord, imageUrl: string, prefix = "task-output"): AiReferenceImage {
  const normalizedUrl = normalizeAiReferenceUrl(imageUrl);
  const remoteUrl = normalizedUrl && !normalizedUrl.startsWith("blob:") ? normalizedUrl : undefined;
  const previewUrl = normalizeAiReferenceUrl(aiTaskThumbnailUrl(task) || normalizedUrl);
  return {
    id: `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    name: `${aiTaskId(task) || "output"}-输出图`,
    url: isHeavyAiImageDataUrl(normalizedUrl) && previewUrl && !isHeavyAiImageDataUrl(previewUrl) ? previewUrl : normalizedUrl,
    ...(previewUrl && previewUrl !== normalizedUrl ? { previewUrl } : {}),
    ...(remoteUrl ? { remoteUrl } : {})
  };
}

function replaceAiReferenceMentionsForApi(prompt: string, referenceCount: number) {
  return prompt.replace(/@(?:图|image)\s*(\d+)/gi, (text, value) => {
    const index = Number(value);
    if (!Number.isFinite(index) || index < 1 || index > referenceCount) return text;
    return `[image ${index}]`;
  });
}

function effectiveAiPromptForReferences(prompt: string, references: AiGenerationReference[]) {
  return replaceAiReferenceMentionsForApi(prompt.trim(), references.length);
}

function favoriteSelectedAiTasks() {
  const ids = aiSelectedTaskIds.value;
  if (!ids.length) return;
  openAiFavoritePicker(ids);
}

function areSelectedAiTasksFavorite() {
  const ids = aiSelectedTaskIds.value;
  return Boolean(ids.length && ids.every((id) => aiFavoriteTaskIds.value.includes(id)));
}

async function downloadSelectedAiTasks() {
  const selectedTasks = onlineImageTasks.value.filter((task) => aiSelectedTaskIds.value.includes(aiTaskId(task)) && aiTaskImageUrl(task));
  if (!selectedTasks.length) {
    ElMessage.warning("选中任务没有可下载图片");
    return;
  }
  for (const task of selectedTasks.slice(0, 6)) {
    await downloadAiTask(task);
  }
  ElMessage.success(`已触发 ${selectedTasks.slice(0, 6).length} 项下载`);
}

function hideSelectedAiTasks() {
  aiHiddenTaskIds.value = Array.from(new Set([...aiHiddenTaskIds.value, ...aiSelectedTaskIds.value]));
  clearAiTaskSelection();
  ElMessage.success("已从当前视图移除");
  void saveAiState();
}

async function confirmDeleteSelectedAiTasks() {
  const ids = aiSelectedTaskIds.value.filter(Boolean);
  if (!ids.length) return;
  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${ids.length} 个任务吗？关联的图片资源也会被清理（如果没有其他任务引用）。`,
      "删除选中",
      {
        confirmButtonText: "删除选中",
        cancelButtonText: "取消",
        type: "warning",
        confirmButtonClass: "el-button--danger"
      }
    );
  } catch {
    return;
  }
  aiHiddenTaskIds.value = Array.from(new Set([...aiHiddenTaskIds.value, ...ids]));
  aiFavoriteTaskIds.value = aiFavoriteTaskIds.value.filter((item) => !ids.includes(item));
  aiFavoriteCollections.value = aiFavoriteCollections.value.map((collection) => ({
    ...collection,
    taskIds: collection.taskIds.filter((taskId) => !ids.includes(taskId)),
    updatedAt: new Date().toISOString()
  }));
  aiOptimisticTasks.value = aiOptimisticTasks.value.filter((item) => !ids.includes(aiTaskId(item)));
  if (ids.includes(aiDetailTaskId.value)) closeAiDetailModal();
  if (ids.includes(aiLightboxTaskId.value)) closeAiLightbox();
  clearAiTaskSelection();
  ElMessage.success(`已删除 ${ids.length} 个任务`);
  void saveAiState();
}

function clearAiFailedTasks() {
  const ids = aiFailedVisibleTasks.value.map(aiTaskId).filter(Boolean);
  if (!ids.length) return;
  aiHiddenTaskIds.value = Array.from(new Set([...aiHiddenTaskIds.value, ...ids]));
  aiSelectedTaskIds.value = aiSelectedTaskIds.value.filter((id) => !ids.includes(id));
  if (!aiSelectedTaskIds.value.length) aiMobileSelectionMode.value = false;
  ElMessage.success(`已清除 ${ids.length} 条失败记录`);
  void saveAiState();
}

async function deleteAiTask(task: AdminRecord) {
  const id = aiTaskId(task);
  if (!id) return;
  try {
    await ElMessageBox.confirm(
      "确定要删除这个任务吗？关联的图片资源也会被清理（如果没有其他任务引用）。",
      "删除任务",
      {
        confirmButtonText: "删除任务",
        cancelButtonText: "取消",
        type: "warning",
        confirmButtonClass: "el-button--danger"
      }
    );
  } catch {
    return;
  }
  aiHiddenTaskIds.value = Array.from(new Set([...aiHiddenTaskIds.value, id]));
  aiSelectedTaskIds.value = aiSelectedTaskIds.value.filter((item) => item !== id);
  if (!aiSelectedTaskIds.value.length) aiMobileSelectionMode.value = false;
  aiFavoriteTaskIds.value = aiFavoriteTaskIds.value.filter((item) => item !== id);
  aiFavoriteCollections.value = aiFavoriteCollections.value.map((collection) => ({
    ...collection,
    taskIds: collection.taskIds.filter((taskId) => taskId !== id),
    updatedAt: new Date().toISOString()
  }));
  aiOptimisticTasks.value = aiOptimisticTasks.value.filter((item) => aiTaskId(item) !== id);
  if (aiDetailTaskId.value === id) closeAiDetailModal();
  if (aiLightboxTaskId.value === id) closeAiLightbox();
  aiPlaygroundMessage("success", "任务已删除");
  void saveAiState();
}

function createAiFavoriteCollectionRecord(name: string) {
  const normalizedName = name.trim();
  if (!normalizedName) return null;
  if (Array.from(normalizedName).length > 60) {
    ElMessage.warning("收藏夹名称最多 60 个字符");
    return null;
  }
  const existing = aiFavoriteCollections.value.find((item) => item.name === normalizedName);
  if (existing) return existing;
  const now = new Date().toISOString();
  const collection = { id: `collection-${Date.now()}`, name: normalizedName, taskIds: [], createdAt: now, updatedAt: now };
  aiFavoriteCollections.value = [...aiFavoriteCollections.value, collection];
  if (!aiDefaultFavoriteCollectionId.value) aiDefaultFavoriteCollectionId.value = collection.id;
  ElMessage.success(`已创建收藏夹「${normalizedName}」`);
  return collection;
}

function createAiFavoriteCollection() {
  const name = aiNewCollectionName.value.trim();
  if (!name) {
    ElMessage.warning("请输入收藏夹名称");
    return;
  }
  const collection = createAiFavoriteCollectionRecord(name);
  if (!collection) return;
  aiNewCollectionName.value = "";
  void saveAiState();
}

function createAiFavoriteCollectionFromPicker() {
  const name = aiNewCollectionName.value.trim();
  if (!name) return;
  const collection = createAiFavoriteCollectionRecord(name);
  if (!collection) return;
  aiFavoritePickerCheckedIds.value = Array.from(new Set([...aiFavoritePickerCheckedIds.value, collection.id]));
  aiNewCollectionName.value = "";
  void saveAiState();
}

function setAiDefaultFavoriteCollection(id: string) {
  if (id === "all-favorites") return;
  aiDefaultFavoriteCollectionId.value = aiDefaultFavoriteCollectionId.value === id ? null : id;
  ElMessage.success(aiDefaultFavoriteCollectionId.value === id ? "已设为默认收藏夹" : "已取消默认收藏夹");
  void saveAiState();
}

function startRenameAiFavoriteCollection(collection: AiFavoriteCollection) {
  aiEditingCollectionId.value = collection.id;
  aiEditingCollectionName.value = collection.name;
}

function cancelRenameAiFavoriteCollection() {
  aiEditingCollectionId.value = "";
  aiEditingCollectionName.value = "";
}

function confirmRenameAiFavoriteCollection() {
  const id = aiEditingCollectionId.value;
  const name = aiEditingCollectionName.value.trim();
  if (!id) return;
  if (!name) {
    cancelRenameAiFavoriteCollection();
    return;
  }
  if (aiFavoriteCollections.value.some((item) => item.id !== id && item.name === name)) {
    ElMessage.warning("收藏夹名称已存在");
    return;
  }
  aiFavoriteCollections.value = aiFavoriteCollections.value.map((item) => item.id === id ? { ...item, name, updatedAt: new Date().toISOString() } : item);
  cancelRenameAiFavoriteCollection();
  void saveAiState();
}

function deleteAiFavoriteCollection(id: string, hideTasks = false) {
  if (aiFavoriteCollections.value.length <= 1) {
    ElMessage.warning("至少保留一个收藏夹");
    return;
  }
  const collection = aiFavoriteCollections.value.find((item) => item.id === id);
  if (!collection) return;
  const taskIds = collection.taskIds;
  aiFavoriteCollections.value = aiFavoriteCollections.value.filter((item) => item.id !== id);
  if (hideTasks) {
    aiHiddenTaskIds.value = Array.from(new Set([...aiHiddenTaskIds.value, ...taskIds]));
  }
  const remainingTaskIds = new Set(aiFavoriteCollections.value.flatMap((item) => item.taskIds));
  aiFavoriteTaskIds.value = aiFavoriteTaskIds.value.filter((taskId) => taskId !== id && (remainingTaskIds.has(taskId) || !taskIds.includes(taskId)));
  if (aiActiveFavoriteCollectionId.value === id) aiActiveFavoriteCollectionId.value = "";
  aiSelectedFavoriteCollectionIds.value = aiSelectedFavoriteCollectionIds.value.filter((item) => item !== id);
  aiFavoritePickerCheckedIds.value = aiFavoritePickerCheckedIds.value.filter((item) => item !== id);
  if (aiDefaultFavoriteCollectionId.value === id) aiDefaultFavoriteCollectionId.value = aiFavoriteCollections.value[0]?.id || null;
  ElMessage.success(`已删除收藏夹「${collection.name}」`);
  void saveAiState();
}

async function confirmDeleteAiFavoriteCollection(id: string) {
  const collection = aiFavoriteCollections.value.find((item) => item.id === id);
  if (!collection) return;
  if (aiFavoriteCollections.value.length <= 1) {
    ElMessage.warning("至少保留一个收藏夹");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确定要删除收藏夹「${collection.name}」吗？包含 ${collection.taskIds.length} 项收藏。`,
      "删除收藏夹",
      { confirmButtonText: "删除", cancelButtonText: "取消", type: "warning" }
    );
    deleteAiFavoriteCollection(id, false);
  } catch {
    // User cancelled.
  }
}

function toggleAiFavoriteCollectionSelection(id: string) {
  if (id === "all-favorites") return;
  aiSelectedFavoriteCollectionIds.value = aiSelectedFavoriteCollectionIds.value.includes(id)
    ? aiSelectedFavoriteCollectionIds.value.filter((item) => item !== id)
    : [...aiSelectedFavoriteCollectionIds.value, id];
}

function visibleRealFavoriteCollectionIds() {
  return aiFavoriteCollectionCards.value
    .filter((collection) => !collection.virtual)
    .map((collection) => collection.id);
}

function selectAllAiVisibleFavoriteCollections() {
  aiSelectedFavoriteCollectionIds.value = Array.from(new Set([...aiSelectedFavoriteCollectionIds.value, ...visibleRealFavoriteCollectionIds()]));
}

function invertAiVisibleFavoriteCollectionSelection() {
  const visibleIds = visibleRealFavoriteCollectionIds();
  const next = aiSelectedFavoriteCollectionIds.value.filter((id) => !visibleIds.includes(id));
  visibleIds.forEach((id) => {
    if (!aiSelectedFavoriteCollectionIds.value.includes(id)) next.push(id);
  });
  aiSelectedFavoriteCollectionIds.value = next;
}

async function downloadSelectedAiFavoriteCollections() {
  const selectedTaskIds = Array.from(new Set(
    aiFavoriteCollections.value
      .filter((collection) => aiSelectedFavoriteCollectionIds.value.includes(collection.id))
      .flatMap((collection) => collection.taskIds)
  ));
  const selectedTasks = onlineImageTasks.value.filter((task) => selectedTaskIds.includes(aiTaskId(task)) && aiTaskImageUrl(task));
  if (!selectedTasks.length) {
    ElMessage.warning("选中收藏夹没有可下载图片");
    return;
  }
  for (const task of selectedTasks.slice(0, 12)) {
    await downloadAiTask(task);
  }
  ElMessage.success(`已触发 ${selectedTasks.slice(0, 12).length} 张图片下载`);
}

async function confirmDeleteSelectedAiFavoriteCollections() {
  const ids = aiSelectedFavoriteCollectionIds.value.filter((id) => aiFavoriteCollections.value.some((collection) => collection.id === id));
  if (!ids.length) return;
  if (ids.length >= aiFavoriteCollections.value.length) {
    ElMessage.warning("至少保留一个收藏夹");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${ids.length} 个收藏夹吗？收藏任务会从这些收藏夹中移除。`,
      "批量删除收藏夹",
      { confirmButtonText: "删除", cancelButtonText: "取消", type: "warning" }
    );
    ids.forEach((id) => deleteAiFavoriteCollection(id, false));
    aiSelectedFavoriteCollectionIds.value = [];
  } catch {
    // User cancelled.
  }
}

function addSelectedToCollection(collection: AiFavoriteCollection) {
  if (!aiSelectedTaskIds.value.length) {
    ElMessage.warning("请先选择任务");
    return;
  }
  aiFavoriteCollections.value = aiFavoriteCollections.value.map((item) => item.id === collection.id ? { ...item, taskIds: Array.from(new Set([...item.taskIds, ...aiSelectedTaskIds.value])), updatedAt: new Date().toISOString() } : item);
  aiFavoriteTaskIds.value = Array.from(new Set([...aiFavoriteTaskIds.value, ...aiSelectedTaskIds.value]));
  ElMessage.success("已加入收藏夹");
  void saveAiState();
}

function selectAiFavoriteCollection(id: string) {
  aiActiveFavoriteCollectionId.value = aiActiveFavoriteCollectionId.value === id ? "" : id;
  void saveAiState();
}

function reuseAiTask(task: AdminRecord, options: { notify?: boolean } = {}) {
  const prompt = String(task.prompt || task.name || "");
  if (prompt) onlineImageForm.value.prompt = prompt;
  if (task.model) onlineImageForm.value.model = String(task.model);
  const params = (task.params || {}) as Record<string, unknown>;
  if (typeof params.provider === "string") onlineImageForm.value.provider = params.provider;
  const ratio = String(params.imageRatio || params.ratio || "").trim();
  if (ratio) onlineImageForm.value.ratio = ratio;
  if (params.size) onlineImageForm.value.size = String(params.size);
  const count = Number(params.count || params.n);
  if (Number.isFinite(count) && count > 0) onlineImageForm.value.count = Math.min(4, Math.max(1, Math.round(count)));
  const width = Number(params.width);
  const height = Number(params.height);
  if (Number.isFinite(width) && width > 0) onlineImageForm.value.width = Math.round(width);
  if (Number.isFinite(height) && height > 0) onlineImageForm.value.height = Math.round(height);
  if (typeof params.resolution === "string") onlineImageForm.value.resolution = params.resolution;
  const quality = normalizeAiImageQuality(params.quality || params.imageQuality);
  if (quality) onlineImageForm.value.quality = quality;
  if (params.output_format || params.outputFormat) onlineImageForm.value.outputFormat = String(params.output_format || params.outputFormat);
  if (typeof params.transparent_output === "boolean") onlineImageForm.value.transparentOutput = params.transparent_output;
  if (typeof params.transparentOutput === "boolean") onlineImageForm.value.transparentOutput = params.transparentOutput;
  if (typeof params.moderation === "string") onlineImageForm.value.moderation = params.moderation;
  if (options.notify !== false) aiPlaygroundMessage("success", "已复用任务参数");
  nextTick(() => aiPromptInputRef.value?.adjustHeight());
}

function reuseUserWorkTask(task: AdminRecord) {
  reuseAiTask(task);
  void selectAdminModule("userAiImage");
}

async function retryAiTask(task: AdminRecord) {
  if (onlineSubmitting.value) return;
  reuseAiTask(task, { notify: false });
  aiPlaygroundMessage("info", "已复用失败任务，正在重新生成");
  await nextTick();
  await submitAiImage();
}

function normalizeAiImageQuality(value: unknown) {
  const quality = String(value || "").trim().toLowerCase();
  if (["auto", "standard", "high", "medium", "low"].includes(quality)) return quality;
  if (quality === "draft") return "low";
  return "";
}

function previewAiTask(task: AdminRecord) {
  const imageUrl = aiTaskImageUrl(task);
  if (imageUrl) {
    aiDetailTaskId.value = aiTaskId(task);
    void ensureAiOriginalImageCached(task);
    closeAiImageContextMenu();
    return;
  }
  reuseAiTask(task);
}

function closeAiDetailModal() {
  closeAiImageContextMenu();
  aiDetailTaskId.value = "";
}

function openAiDetailImageLightbox(task: AdminRecord) {
  if (!aiTaskImageUrl(task)) return;
  aiLightboxTaskId.value = aiTaskId(task);
  void ensureAiOriginalImageCached(task);
  resetAiLightboxTransform();
  closeAiImageContextMenu();
}

function clampNumber(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

function resetAiLightboxTransform() {
  aiLightboxScale.value = 1;
  aiLightboxTx.value = 0;
  aiLightboxTy.value = 0;
  aiLightboxDragging.value = false;
  aiLightboxPointerStart = null;
  aiLightboxPinchStart = null;
  aiLightboxPointers.clear();
}

function applyAiLightboxTransform(scale: number, tx: number, ty: number) {
  const nextScale = clampNumber(scale, 1, 10);
  aiLightboxScale.value = nextScale;
  aiLightboxTx.value = nextScale <= 1.01 ? 0 : tx;
  aiLightboxTy.value = nextScale <= 1.01 ? 0 : ty;
}

function aiLightboxPointFromEvent(event: Pick<MouseEvent, "clientX" | "clientY">) {
  const rect = aiLightboxViewportRef.value?.getBoundingClientRect();
  if (!rect) return { x: 0, y: 0 };
  return {
    x: event.clientX - rect.left - rect.width / 2,
    y: event.clientY - rect.top - rect.height / 2
  };
}

function zoomAiLightboxAt(factor: number, event?: Pick<MouseEvent, "clientX" | "clientY">) {
  const currentScale = aiLightboxScale.value;
  const nextScale = clampNumber(currentScale * factor, 1, 10);
  if (Math.abs(nextScale - currentScale) < 0.001) return;
  const point = event ? aiLightboxPointFromEvent(event) : { x: 0, y: 0 };
  const ratio = nextScale / currentScale;
  applyAiLightboxTransform(
    nextScale,
    point.x - ratio * (point.x - aiLightboxTx.value),
    point.y - ratio * (point.y - aiLightboxTy.value)
  );
}

function zoomAiLightboxBy(factor: number) {
  zoomAiLightboxAt(factor);
}

function handleAiLightboxWheel(event: WheelEvent) {
  closeAiImageContextMenu();
  zoomAiLightboxAt(event.deltaY < 0 ? 1.15 : 1 / 1.15, event);
}

function handleAiLightboxDoubleClick(event: MouseEvent) {
  closeAiImageContextMenu();
  if (aiLightboxScale.value > 1.01) {
    resetAiLightboxTransform();
    return;
  }
  const point = aiLightboxPointFromEvent(event);
  applyAiLightboxTransform(3, -point.x * 2, -point.y * 2);
}

function aiLightboxPointerDistance() {
  const points = Array.from(aiLightboxPointers.values());
  if (points.length < 2) return 0;
  const [a, b] = points;
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function aiLightboxPointerMidpoint() {
  const points = Array.from(aiLightboxPointers.values());
  if (points.length < 2) return { x: 0, y: 0 };
  const [a, b] = points;
  const rect = aiLightboxViewportRef.value?.getBoundingClientRect();
  if (!rect) return { x: 0, y: 0 };
  return {
    x: (a.x + b.x) / 2 - rect.left - rect.width / 2,
    y: (a.y + b.y) / 2 - rect.top - rect.height / 2
  };
}

function handleAiLightboxPointerDown(event: PointerEvent) {
  if (event.target instanceof Element && event.target.closest("button")) return;
  closeAiImageContextMenu();
  aiLightboxPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
  (event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId);
  if (aiLightboxPointers.size === 2) {
    const mid = aiLightboxPointerMidpoint();
    aiLightboxPinchStart = {
      distance: aiLightboxPointerDistance(),
      scale: aiLightboxScale.value,
      tx: aiLightboxTx.value,
      ty: aiLightboxTy.value,
      midX: mid.x,
      midY: mid.y
    };
    aiLightboxDragging.value = true;
    return;
  }
  aiLightboxPointerStart = {
    id: event.pointerId,
    x: event.clientX,
    y: event.clientY,
    baseX: aiLightboxTx.value,
    baseY: aiLightboxTy.value,
    moved: false,
    startedAt: Date.now()
  };
  aiLightboxDragging.value = aiLightboxScale.value > 1.01;
}

function handleAiLightboxPointerMove(event: PointerEvent) {
  if (!aiLightboxPointers.has(event.pointerId)) return;
  aiLightboxPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
  if (aiLightboxPinchStart && aiLightboxPointers.size >= 2) {
    const nextDistance = aiLightboxPointerDistance();
    if (aiLightboxPinchStart.distance <= 0) return;
    const nextScale = clampNumber(aiLightboxPinchStart.scale * (nextDistance / aiLightboxPinchStart.distance), 1, 10);
    const ratio = nextScale / aiLightboxPinchStart.scale;
    applyAiLightboxTransform(
      nextScale,
      aiLightboxPinchStart.midX - ratio * (aiLightboxPinchStart.midX - aiLightboxPinchStart.tx),
      aiLightboxPinchStart.midY - ratio * (aiLightboxPinchStart.midY - aiLightboxPinchStart.ty)
    );
    return;
  }
  if (!aiLightboxPointerStart || aiLightboxPointerStart.id !== event.pointerId) return;
  const dx = event.clientX - aiLightboxPointerStart.x;
  const dy = event.clientY - aiLightboxPointerStart.y;
  if (Math.abs(dx) > 4 || Math.abs(dy) > 4) aiLightboxPointerStart.moved = true;
  if (aiLightboxScale.value > 1.01) {
    applyAiLightboxTransform(aiLightboxScale.value, aiLightboxPointerStart.baseX + dx, aiLightboxPointerStart.baseY + dy);
  }
}

function handleAiLightboxPointerUp(event: PointerEvent) {
  aiLightboxPointers.delete(event.pointerId);
  if (aiLightboxPointers.size < 2) aiLightboxPinchStart = null;
  const start = aiLightboxPointerStart;
  aiLightboxDragging.value = false;
  if (!start || start.id !== event.pointerId) return;
  const dx = event.clientX - start.x;
  const dy = event.clientY - start.y;
  const isQuickTap = !start.moved && Date.now() - start.startedAt < 320;
  if (isQuickTap && event.target instanceof HTMLImageElement) {
    const now = Date.now();
    if (now - aiLightboxLastTap.time < 350 && Math.abs(event.clientX - aiLightboxLastTap.x) < 40 && Math.abs(event.clientY - aiLightboxLastTap.y) < 40) {
      if (aiLightboxScale.value > 1.01) resetAiLightboxTransform();
      else zoomAiLightboxAt(3, event);
      aiLightboxLastTap = { time: 0, x: 0, y: 0 };
    } else {
      aiLightboxLastTap = { time: now, x: event.clientX, y: event.clientY };
    }
  }
  if (aiLightboxScale.value <= 1.01 && Math.abs(dx) > 48 && Math.abs(dx) > Math.abs(dy) * 1.4 && aiLightboxTotal.value > 1) {
    moveAiLightbox(dx < 0 ? 1 : -1);
  }
  aiLightboxPointerStart = null;
}

function editAiTaskOutput(task: AdminRecord) {
  const imageUrl = aiTaskImageUrl(task);
  if (!imageUrl) {
    aiPlaygroundMessage("warning", "图片生成完成后才能编辑输出");
    return;
  }
  if (aiReferenceImages.value.length >= 10) {
    aiPlaygroundMessage("warning", "参考图最多上传 10 张");
    return;
  }
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    createAiTaskReferenceImage(task, imageUrl)
  ];
  reuseAiTask(task);
  notifyAiImageEditReady();
}

function closeAiLightbox() {
  closeAiImageContextMenu();
  aiLightboxTaskId.value = "";
  resetAiLightboxTransform();
}

function moveAiLightbox(direction: number) {
  closeAiImageContextMenu();
  const tasks = aiLightboxTasks.value;
  if (!tasks.length) return;
  const currentIndex = Math.max(0, tasks.findIndex((task) => aiTaskId(task) === aiLightboxTaskId.value));
  const nextIndex = (currentIndex + direction + tasks.length) % tasks.length;
  aiLightboxTaskId.value = aiTaskId(tasks[nextIndex]);
  resetAiLightboxTransform();
}

function handleAiLightboxKeydown(event: KeyboardEvent) {
  if (!aiLightboxTaskId.value) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeAiLightbox();
    return;
  }
  if (event.key === "ArrowLeft" && aiLightboxTotal.value > 1) {
    event.preventDefault();
    moveAiLightbox(-1);
    return;
  }
  if (event.key === "ArrowRight" && aiLightboxTotal.value > 1) {
    event.preventDefault();
    moveAiLightbox(1);
    return;
  }
  if ((event.key === "+" || event.key === "=") && !event.ctrlKey && !event.metaKey) {
    event.preventDefault();
    zoomAiLightboxBy(1.25);
    return;
  }
  if (event.key === "-" && !event.ctrlKey && !event.metaKey) {
    event.preventDefault();
    zoomAiLightboxBy(1 / 1.25);
    return;
  }
  if (event.key === "0") {
    event.preventDefault();
    resetAiLightboxTransform();
  }
}

function closeAiImageContextMenu() {
  aiImageContextMenu.value.visible = false;
}

function openAiImageContextMenu(event: MouseEvent, task: AdminRecord) {
  const menuWidth = 148;
  const menuHeight = aiTaskOutputItems(task).length > 1 ? 188 : 148;
  const left = Math.min(event.clientX, Math.max(12, window.innerWidth - menuWidth - 12));
  const top = Math.min(event.clientY, Math.max(12, window.innerHeight - menuHeight - 12));
  aiImageContextMenu.value = {
    visible: true,
    x: left,
    y: top,
    taskId: aiTaskId(task)
  };
}

const aiContextMenuTask = computed(() => onlineImageTasks.value.find((task) => aiTaskId(task) === aiImageContextMenu.value.taskId));

function handleAiImageContextMenuDismiss(event?: Event) {
  if (!aiImageContextMenu.value.visible) return;
  if (event?.target instanceof Element && event.target.closest(".ai-image-context-menu")) return;
  closeAiImageContextMenu();
}

async function imageUrlToBlob(url: string) {
  return fetchResourceBlob(url, { auth: false });
}

async function copyAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  const imageUrl = task ? aiTaskImageUrl(task) : "";
  if (!imageUrl) {
    aiPlaygroundMessage("warning", "暂无可复制图片");
    return;
  }
  try {
    const blob = await imageUrlToBlob(imageUrl);
    if (!navigator.clipboard || typeof ClipboardItem === "undefined") throw new Error("当前浏览器不支持复制图片");
    await navigator.clipboard.write([new ClipboardItem({ [blob.type || "image/png"]: blob })]);
    aiPlaygroundMessage("success", "图片已复制");
  } catch (error) {
    console.error(error);
    aiPlaygroundMessage("error", error instanceof Error ? error.message : "复制失败");
  }
}

async function downloadAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  await downloadAiTask(task);
}

async function downloadAllAiContextImages() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  await downloadAllAiTaskOutputs(task);
}

async function editAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  const imageUrl = task ? aiTaskImageUrl(task) : "";
  if (!task || !imageUrl) {
    aiPlaygroundMessage("warning", "暂无可编辑图片");
    return;
  }
  if (aiReferenceImages.value.length >= 10) {
    aiPlaygroundMessage("warning", "参考图最多上传 10 张");
    return;
  }
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    createAiTaskReferenceImage(task, imageUrl, "context")
  ];
  reuseAiTask(task);
  closeAiLightbox();
  notifyAiImageEditReady();
}

function notifyAiImageEditReady() {
  aiPlaygroundMode.value = "gallery";
  onlineImageForm.value.prompt = "";
  void saveAiState();
  aiPlaygroundMessage("success", "图片已放入参考图，请输入要怎么编辑后再点生成");
  window.setTimeout(() => {
    aiPromptInputRef.value?.focus();
    aiPromptInputRef.value?.adjustHeight();
  }, 80);
  void ElMessageBox.alert(
    "图片已加入底部参考图。请在输入框写清楚要保留什么、修改什么，例如：保留产品主体，换成白底电商主图，加上京东 logo。写完后你再点击生成。",
    "请继续编辑照片",
    { confirmButtonText: "我来填写" }
  );
}

function createAiAgentConversation() {
  const id = `agent-${Date.now()}`;
  const now = new Date().toISOString();
  aiAgentConversations.value = [{
    id,
    title: `新对话 ${aiAgentConversations.value.length + 1}`,
    createdAt: now,
    updatedAt: now,
    messages: [{ role: "assistant", content: "新的 Agent 对话已创建，可以提交提示词开始规划。", createdAt: now }]
  }, ...aiAgentConversations.value];
  aiAgentActiveConversationId.value = id;
  void saveAiState();
}

function selectAiAgentConversation(id: string) {
  aiAgentActiveConversationId.value = id;
  void saveAiState();
}

function stopAiAgent() {
  aiAgentRunning.value = false;
  const conversation = activeAiAgentConversation.value;
  if (conversation) {
    const now = new Date().toISOString();
    conversation.messages.push({ role: "assistant", content: "已停止当前生成。", createdAt: now });
    conversation.updatedAt = now;
  }
  ElMessage.success("已停止 Agent 生成");
  void saveAiState();
}

async function downloadUrl(url: string, fileName: string, assetId = "") {
  let objectUrl = "";
  const anchor = document.createElement("a");
  try {
    const blob = assetId
      ? await downloadAssetBlob(assetId)
      : await fetchResourceBlob(url, { auth: url.startsWith("/") });
    objectUrl = URL.createObjectURL(blob);
    anchor.href = objectUrl;
    anchor.download = fileName;
    anchor.rel = "noopener";
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    if (objectUrl) {
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
    }
  }
}

async function downloadAiTask(task?: AdminRecord) {
  const target = task || onlinePreviewTask.value;
  if (!target) {
    ElMessage.warning("暂无可下载任务");
    return;
  }
  const imageUrl = aiTaskImageUrl(target);
  if (!imageUrl) {
    ElMessage.warning("当前任务还没有生成图片");
    return;
  }
  if (!ensureWorkspaceAuth("download_work", "userAiImage", { mediaKind: "image", taskId: aiTaskId(target) })) return;
  const fileName = `ai-image-${aiTaskId(target) || Date.now()}.png`;
  try {
    const assetId = String(aiTaskAsset(target)?.id || "");
    await downloadUrl(imageUrl, fileName, assetId);
    ElMessage.success("已开始下载");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "下载失败，请稍后重试");
  }
}

async function downloadAllAiTaskOutputs(task?: AdminRecord) {
  if (!task) {
    ElMessage.warning("暂无可下载任务");
    return;
  }
  const outputs = aiTaskOutputItems(task);
  if (!outputs.length) {
    ElMessage.warning("当前任务还没有生成图片");
    return;
  }
  if (!ensureWorkspaceAuth("download_work", "userAiImage", { mediaKind: "image-all", taskId: aiTaskId(task) })) return;
  let successCount = 0;
  for (let index = 0; index < outputs.length; index += 1) {
    const output = outputs[index];
    try {
      await downloadUrl(output.url, `ai-image-${aiTaskId(task) || Date.now()}-${index + 1}.png`, output.assetId || "");
      successCount += 1;
    } catch {
      // 继续尝试剩余图片，最后统一提示成功数量。
    }
  }
  if (successCount) {
    ElMessage.success(`已开始下载 ${successCount} 张图片`);
  } else {
    ElMessage.error("下载失败，请稍后重试");
  }
}

async function handleAiReferenceUpload(uploadFile: { raw?: File; name?: string }) {
  const file = uploadFile.raw;
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    ElMessage.error("请选择图片文件");
    return;
  }
  if (aiReferenceImages.value.length >= 10) {
    ElMessage.warning("参考图最多上传 10 张");
    return;
  }
  const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const url = URL.createObjectURL(file);
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    {
      id,
      name: uploadFile.name || file.name,
      url,
      file,
      uploading: hasAuthToken()
    }
  ];
  if (hasAuthToken()) void ensureAiReferenceRemoteUrl(id);
}

async function handleOnlineReferenceUpload(uploadFile: { raw?: File; name?: string }) {
  if (onlineReferenceImages.value.length >= onlineReferenceSlots.length) {
    ElMessage.warning(`在线生图最多上传 ${onlineReferenceSlots.length} 张参考图`);
    return;
  }
  await handleAiReferenceUpload(uploadFile);
}

function handleAiPromptPasteImages(files: File[]) {
  for (const file of files) {
    if (aiReferenceImages.value.length >= 10) {
      ElMessage.warning("参考图最多上传 10 张");
      break;
    }
    void handleAiReferenceUpload({ raw: file, name: file.name || "粘贴图片.png" });
  }
  nextTick(() => aiPromptInputRef.value?.adjustHeight());
}

function clampNumberInput(value: string, min: number, max: number, fallback: number) {
  const nextValue = Number(value);
  if (value.trim() === "" || Number.isNaN(nextValue)) return fallback;
  return Math.max(min, Math.min(max, Math.round(nextValue)));
}

function handleAiCountInput(event: Event) {
  aiCountInput.value = (event.target as HTMLInputElement).value;
}

function commitAiCountInput() {
  const nextValue = clampNumberInput(aiCountInput.value, 1, 4, onlineImageForm.value.count || 1);
  onlineImageForm.value.count = nextValue;
  aiCountInput.value = String(nextValue);
}

function handleAiCompressionInput(event: Event) {
  aiOutputCompressionInput.value = (event.target as HTMLInputElement).value;
}

function commitAiCompressionInput() {
  if (aiOutputCompressionInput.value.trim() === "") {
    onlineImageForm.value.outputCompression = null;
    return;
  }
  const nextValue = clampNumberInput(aiOutputCompressionInput.value, 0, 100, onlineImageForm.value.outputCompression ?? 0);
  onlineImageForm.value.outputCompression = nextValue;
  aiOutputCompressionInput.value = String(nextValue);
}

function revokeAiReferenceImage(item?: AiReferenceImage) {
  if (item?.url.startsWith("blob:")) URL.revokeObjectURL(item.url);
  if (item?.previewUrl?.startsWith("blob:") && item.previewUrl !== item.url) URL.revokeObjectURL(item.previewUrl);
}

function aiRecordReferencesImage(record: unknown, item: AiReferenceImage) {
  if (!record || typeof record !== "object") return false;
  const json = JSON.stringify(record);
  return json.includes(item.id) || (!!item.remoteUrl && json.includes(item.remoteUrl)) || (!!item.url && !item.url.startsWith("blob:") && json.includes(item.url));
}

function isAiReferenceImageReferencedByState(item?: AiReferenceImage) {
  if (!item) return false;
  if (aiReferenceImages.value.some((current) => current.id === item.id)) return true;
  if (aiRecordReferencesImage(aiImageDraftPayload(), item)) return true;
  if (onlineImageTasks.value.some((task) => aiRecordReferencesImage(aiTaskParams(task), item) || aiRecordReferencesImage(task, item))) return true;
  if (onlineAssets.value.some((asset) => aiRecordReferencesImage(asset, item))) return true;
  return false;
}

function deleteAiReferenceImageIfUnreferenced(item?: AiReferenceImage) {
  if (!item || isAiReferenceImageReferencedByState(item)) return;
  revokeAiReferenceImage(item);
}

function removeAiReferenceImage(index: number) {
  const item = aiReferenceImages.value[index];
  aiReferenceImages.value = aiReferenceImages.value.filter((_, currentIndex) => currentIndex !== index);
  deleteAiReferenceImageIfUnreferenced(item);
}

async function confirmRemoveAiReferenceImage(index: number) {
  const item = aiReferenceImages.value[index];
  if (!item) return;
  try {
    await ElMessageBox.confirm(
      `确定要移除参考图「${item.name || `参考图 ${index + 1}`}」吗？`,
      "移除参考图",
      { confirmButtonText: "移除", cancelButtonText: "取消", type: "warning" }
    );
    removeAiReferenceImage(index);
    ElMessage.success("已移除参考图");
  } catch {
    // User cancelled.
  }
}

function clearAiReferenceImages() {
  const removedItems = [...aiReferenceImages.value];
  aiReferenceImages.value = [];
  removedItems.forEach(deleteAiReferenceImageIfUnreferenced);
}

async function uploadAiReferenceFile(file: File) {
  const url = await uploadReferenceImage(file);
  return new URL(url, window.location.origin).href;
}

async function ensureAiReferenceRemoteUrl(id: string) {
  const current = aiReferenceImages.value.find((item) => item.id === id);
  if (!current?.file || current.remoteUrl) return current?.remoteUrl || "";
  try {
    const remoteUrl = await uploadAiReferenceFile(current.file);
    aiReferenceImages.value = aiReferenceImages.value.map((item) => (
      item.id === id ? { ...item, remoteUrl, uploading: false, error: "" } : item
    ));
    return remoteUrl;
  } catch (error) {
    aiReferenceImages.value = aiReferenceImages.value.map((item) => (
      item.id === id ? { ...item, uploading: false, error: error instanceof Error ? error.message : "参考图上传失败" } : item
    ));
    throw error;
  }
}

function readReferenceFileAsDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(reader.error || new Error("参考图读取失败"));
    reader.readAsDataURL(file);
  });
}

async function aiGenerationReferences() {
  const references: AiGenerationReference[] = [];
  for (const item of aiReferenceImages.value) {
    let url = normalizeAiReferenceUrl(item.remoteUrl || item.url);
    if (item.file && (!url || url.startsWith("blob:"))) {
      try {
        url = await ensureAiReferenceRemoteUrl(item.id);
      } catch {
        url = await readReferenceFileAsDataUrl(item.file);
        ElMessage.warning("参考图上传较慢，已临时使用本地图片数据提交");
      }
    }
    url = normalizeAiReferenceUrl(url);
    if (!url || url.startsWith("blob:")) continue;
    references.push({
      id: item.id,
      name: item.name || `reference-${references.length + 1}`,
      url,
      order: references.length + 1,
      source: aiReferenceSource(item)
    });
  }
  return references;
}

async function createAiGenerationTaskSnapshot(prompt: string): Promise<AiGenerationTaskSnapshot> {
  const inputImagesSnapshot = await aiGenerationReferences();
  return {
    prompt,
    inputImageIds: inputImagesSnapshot.map((item) => item.id),
    inputImagesSnapshot,
    maskDraft: null,
    maskTargetImageId: "",
    maskImageId: "",
    createdAt: new Date().toISOString()
  };
}

async function confirmClearAiReferenceImages() {
  if (!aiReferenceImages.value.length) return;
  try {
    await ElMessageBox.confirm(
      `确定要清空全部 ${aiReferenceImages.value.length} 张参考图吗？`,
      "清空参考图",
      { confirmButtonText: "清空", cancelButtonText: "取消", type: "warning" }
    );
    clearAiReferenceImages();
    ElMessage.success("已清空参考图");
  } catch {
    // User cancelled.
  }
}

function runAiPlaygroundAction(action: "download" | "help" | "settings") {
  if (action === "download") {
    void downloadAiTask();
    return;
  }
  if (action === "help") {
    ElMessageBox.alert(
      "画廊模式用于查看生成历史、筛选收藏和按提示词检索；底部输入栏支持 Prompt、参考图、尺寸、质量、格式、透明背景、审核和数量参数。",
      "AI生图操作指南",
      { confirmButtonText: "知道了" }
    );
    return;
  }
  aiSettingsDraft.value = aiSettingsStore.createDraft(onlineImageForm.value.model);
  aiSettingsVisible.value = true;
  aiSettingsTab.value = "api";
}

function closeAiSettings() {
  aiSettingsVisible.value = false;
}

function saveAiSettings() {
  const saved = aiSettingsStore.save(aiSettingsDraft.value);
  aiSettingsDraft.value = { ...saved };
  onlineImageForm.value.model = saved.model || onlineImageForm.value.model;
  syncOnlineProviderForModel();
  ElMessage.success("AI 生图设置已保存");
  closeAiSettings();
}

function hydrateAiSettingsFromStore() {
  const saved = aiSettingsStore.load(onlineImageForm.value.model);
  aiSettingsDraft.value = { ...saved };
  if (saved.model) onlineImageForm.value.model = saved.model;
  syncOnlineProviderForModel();
}

function testAiSettingsConnection() {
  ElMessage.success("已提交连接检测，请以服务端上游可用性为准");
}

async function exportAiSettings() {
  const payload = JSON.stringify(aiSettingsDraft.value, null, 2);
  await navigator.clipboard?.writeText(payload);
  ElMessage.success("配置已复制到剪贴板");
}

function importAiSettings() {
  ElMessage.info("导入入口已保留，可接入 JSON 配置文件解析");
}

async function resetAiSettings() {
  await ElMessageBox.confirm("确认重置 AI 生图设置为默认值？", "重置配置", {
    confirmButtonText: "确认重置",
    cancelButtonText: "取消",
    type: "warning"
  });
  aiSettingsDraft.value = { ...aiSettingsStore.reset(onlineImageForm.value.model) };
  ElMessage.success("已重置为默认配置");
}

async function submitOnlineImage() {
  const prompt = onlineImageForm.value.prompt.trim();
  if (!prompt) {
    ElMessage.error("请输入生图提示词");
    return;
  }
  if (!ensureWorkspaceAuth("generate_image", "userAiImage")) return;
  const clientRequestId = createGenerationClientRequestId("online-image");
  onlineSubmitting.value = true;
  try {
    const requestQuality = aiRequestQualityParam(onlineImageForm.value.quality);
    const taskSnapshot = await createAiGenerationTaskSnapshot(prompt);
    const referenceImages = taskSnapshot.inputImagesSnapshot.slice(0, onlineReferenceSlots.length);
    if (onlineReferenceImages.value.length && !referenceImages.length) {
      ElMessage.error("参考图还没有准备好，请稍后重试");
      return;
    }
    const createdTask = await adminRequest<AdminRecord>({
      method: "POST",
      url: "/generation-tasks",
      data: {
        clientRequestId,
        type: referenceImages.length ? "IMAGE_TO_IMAGE" : "TEXT_TO_IMAGE",
        prompt,
        model: onlineImageForm.value.model,
        params: {
          count: onlineImageForm.value.count,
          imageRatio: onlineImageForm.value.ratio,
          ...(requestQuality ? { imageQuality: requestQuality } : {}),
          provider: onlineImageForm.value.provider,
          resolution: onlineImageForm.value.resolution,
          width: onlineImageForm.value.width,
          height: onlineImageForm.value.height,
          taskSnapshot,
          inputImageIds: taskSnapshot.inputImageIds.slice(0, onlineReferenceSlots.length),
          inputImagesSnapshot: referenceImages,
          referenceImages,
          referenceImageCount: referenceImages.length,
          sourceModule: "online-image"
        }
      }
    });
    onlineImageForm.value.prompt = "";
    ElMessage.success("在线生图任务已提交");
    mergeAiGenerationTask(createdTask);
    trackAiGenerationTask(aiTaskId(createdTask));
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "提交失败");
  } finally {
    onlineSubmitting.value = false;
  }
}

async function submitAiImage() {
  const prompt = onlineImageForm.value.prompt.trim();
  if (!prompt) {
    ElMessage.error("请输入 AI 生图提示词");
    return false;
  }
  if (!ensureWorkspaceAuth("generate_image", "userAiImage")) return false;
  if (aiPlaygroundMode.value === "agent") {
    const conversation = activeAiAgentConversation.value;
    if (conversation) {
      const now = new Date().toISOString();
      conversation.messages.push({ role: "user", content: prompt, createdAt: now });
      conversation.messages.push({ role: "assistant", content: `已收到需求，将按当前尺寸 ${onlineImageForm.value.size}、质量 ${onlineImageForm.value.quality} 规划生成任务。`, createdAt: now });
      conversation.title = prompt.slice(0, 18) || conversation.title;
      conversation.updatedAt = now;
    }
    aiAgentRunning.value = true;
    if (aiSettingsDraft.value.clearInputAfterSubmit) {
      onlineImageForm.value.prompt = "";
      await nextTick();
      aiPromptInputRef.value?.focus();
    }
    ElMessage.success("Agent 已开始处理");
    void saveAiState();
    return true;
  }
  onlineSubmitting.value = true;
  const clientRequestId = createGenerationClientRequestId("ai-image");
  let optimisticTaskId = "";
  try {
    syncOnlineProviderForModel();
    const imageSize = resolveAiImageSize();
    const hadInputImages = aiReferenceImages.value.length > 0;
    const taskSnapshot = await createAiGenerationTaskSnapshot(prompt);
    const referenceImages = taskSnapshot.inputImagesSnapshot;
    if (hadInputImages && referenceImages.length === 0) {
      ElMessage.error("参考图还没有准备好，请稍后重试");
      return false;
    }
    const taskType = referenceImages.length > 0 ? "IMAGE_TO_IMAGE" : "TEXT_TO_IMAGE";
    const effectivePrompt = referenceImages.length ? effectiveAiPromptForReferences(prompt, referenceImages) : prompt;
    const requestSize = aiRequestSizeParam(onlineImageForm.value.size);
    const requestQuality = aiRequestQualityParam(onlineImageForm.value.quality);
    const requestParams = {
      n: onlineImageForm.value.count,
      count: onlineImageForm.value.count,
      ...(requestSize ? { size: requestSize } : {}),
      imageRatio: onlineImageForm.value.ratio,
      ...(requestQuality ? { imageQuality: requestQuality, quality: requestQuality } : {}),
      output_format: onlineImageForm.value.outputFormat,
      output_compression: onlineImageForm.value.outputFormat === "png" ? null : onlineImageForm.value.outputCompression,
      transparent_output: onlineImageForm.value.outputFormat === "png" ? onlineImageForm.value.transparentOutput : false,
      moderation: onlineImageForm.value.moderation,
      apiMode: aiSettingsDraft.value.apiMode,
      provider: onlineImageForm.value.provider,
      resolution: imageSize.resolution,
      width: imageSize.width,
      height: imageSize.height,
      taskSnapshot,
      inputImageIds: taskSnapshot.inputImageIds,
      inputImagesSnapshot: taskSnapshot.inputImagesSnapshot,
      maskDraft: taskSnapshot.maskDraft,
      maskTargetImageId: taskSnapshot.maskTargetImageId,
      maskImageId: taskSnapshot.maskImageId,
      referenceImageCount: referenceImages.length,
      referenceImageNames: referenceImages.map((item) => item.name),
      referenceImageOrder: referenceImages.map((item, index) => ({
        id: item.id,
        name: item.name,
        source: item.source,
        order: item.order,
        imageRef: `[image ${index + 1}]`,
        primary: index === 0
      })),
      referenceImages,
      userPrompt: prompt,
      effectivePrompt,
      sourceModule: "ai-image"
    };
    optimisticTaskId = `optimistic_${Date.now()}`;
    const optimisticCreatedAt = new Date().toISOString();
    aiPlaygroundMode.value = "gallery";
    aiGalleryFilter.value = "all";
    aiFavoriteOnly.value = false;
    aiActiveFavoriteCollectionId.value = "";
    aiPromptSearch.value = "";
    aiOptimisticTasks.value = [{
      id: optimisticTaskId,
      userId: currentAdmin.value?.id || "",
      type: taskType,
      prompt,
      params: requestParams,
      model: onlineImageForm.value.model,
      status: "RUNNING",
      progress: 1,
      pointCost: 0,
      resultIds: [],
      createdAt: optimisticCreatedAt,
      updatedAt: optimisticCreatedAt
    }, ...aiOptimisticTasks.value];
    const createdTask = await adminRequest<AdminRecord>({
      method: "POST",
      url: "/generation-tasks",
      data: {
        clientRequestId,
        userId: currentAdmin.value?.id,
        type: taskType,
        prompt,
        model: onlineImageForm.value.model,
        params: requestParams
      }
    });
    if (createdTask && typeof createdTask === "object" && createdTask.id) {
      aiOptimisticTasks.value = aiOptimisticTasks.value.map((task) => String(task.id || "") === optimisticTaskId
        ? { ...task, ...createdTask }
        : task);
      optimisticTaskId = String(createdTask.id);
    }
    if (aiSettingsDraft.value.clearInputAfterSubmit) {
      onlineImageForm.value.prompt = "";
      await nextTick();
      aiPromptInputRef.value?.focus();
    }
    if (aiSettingsDraft.value.clearInputAfterSubmit) clearAiReferenceImages();
    ElMessage.success("AI 生图任务已提交");
    if (createdTask && typeof createdTask === "object") {
      mergeAiGenerationTask(createdTask);
      trackAiGenerationTask(aiTaskId(createdTask));
    }
    aiOptimisticTasks.value = aiOptimisticTasks.value.filter((task) => !isOptimisticAiTaskReconciled(task, onlineRecentTasks.value));
    return true;
  } catch (error) {
    const message = error instanceof Error ? error.message : "提交失败";
    if (optimisticTaskId) {
      aiOptimisticTasks.value = aiOptimisticTasks.value.map((task) => String(task.id || "") === optimisticTaskId
        ? { ...task, status: "FAILED", error: { message }, progress: 100, updatedAt: new Date().toISOString() }
        : task);
    }
    ElMessage.error(message);
    return false;
  } finally {
    onlineSubmitting.value = false;
  }
}

function createGenerationClientRequestId(kind: string) {
  const suffix = typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
  return `web-${kind}-${suffix}`;
}
interface AdminUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  planId?: string;
  memberLevel?: string;
  agentStatus?: string;
  operationCenterStatus?: string;
  subscriptionExpiresAt?: string;
}
interface AuthMeResponse {
  user: AdminUser;
  agent?: AdminRecord | null;
  operationCenter?: AdminRecord | null;
  permissions?: string[];
  workspace?: string;
  defaultModule?: string;
  defaultRoute?: string;
  accessToken?: string;
}
type MembershipCycleId = "monthly" | "yearly" | "single";
type MembershipMode = "recharge" | "subscribe" | "identity";
type PaymentMethodId = "cash" | "wechat" | "alipay";
type MembershipBillingCycle = {
  id: MembershipCycleId;
  label: string;
  discount: string;
};
type UserMembershipPlan = {
  id: string;
  planIds: Record<MembershipCycleId, string>;
  name: string;
  prices: Record<MembershipCycleId, number>;
  originalPrices: Record<MembershipCycleId, number>;
  credits: number;
  images: number;
  videos: number;
  concurrency: string;
  audience: string;
  custom?: boolean;
  oneTime?: boolean;
  recommended?: boolean;
};
type UserMembershipPlanCard = UserMembershipPlan & {
  price: number;
  originalPrice: number;
  planId: string;
  unit: string;
  creditUnit: string;
  credits: number;
  images: number;
  videos: number;
  note: string;
  features: string[];
};
type UserIdentityPackage = {
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
  featured?: boolean;
  features: string[];
};

function formatNumber(value: number) {
  return new Intl.NumberFormat("en-US").format(Math.max(0, Math.round(value)));
}

const sidebarPlan = computed(() => {
  const summary = (store.data?.summary || {}) as Record<string, unknown>;
  const account = (userAccountSnapshot.value || store.data?.account || {}) as Record<string, unknown>;
  const planId = String(currentAdmin.value?.planId || summary.planId || "plan_year");
  const rawAvailable = Number(account.available ?? summary.availablePoints ?? summary.pointsAvailable ?? store.data?.availablePoints ?? store.data?.pointsAvailable ?? 0);
  const rawTotal = Number(account.total ?? summary.totalPoints ?? summary.pointsTotal ?? 0);
  const planNameMap: Record<string, string> = { plan_free: "体验版", plan_month: "Basic", plan_pro: "Pro", plan_year: "Ultimate", plan_enterprise: "企业版" };
  const total = Math.max(rawAvailable + Number(account.frozen || 0), rawTotal || rawAvailable);
  const expiresAt = String(currentAdmin.value?.subscriptionExpiresAt || summary.subscriptionExpiresAt || "2026-07-19").slice(0, 10);
  return {
    name: planNameMap[planId] || "Ultimate",
    status: "使用中",
    expiresAt,
    availableText: formatNumber(rawAvailable),
    totalText: formatNumber(total),
    percent: Math.min(100, Math.max(4, Math.round((rawAvailable / Math.max(1, total)) * 100)))
  };
});
const currentAdmin = ref<AdminUser | null>(null);
const currentAgent = ref<AdminRecord | null>(null);
const currentOperationCenter = ref<AdminRecord | null>(null);
const currentPermissions = ref<string[]>([]);
const canViewEnterpriseManagement = computed(() => {
  const role = String(currentAdmin.value?.role || "").toUpperCase();
  return role === "SUPER_ADMIN" || currentPermissions.value.includes("admin.full") || currentPermissions.value.includes("enterprise:list");
});
function canNavigateToModule(moduleId: string) {
  const permission = modulePermission(moduleId);
  if (!permission) return true;
  const role = String(currentAdmin.value?.role || "").toUpperCase();
  return role === "SUPER_ADMIN" || currentPermissions.value.includes("admin.full") || currentPermissions.value.includes(permission);
}
const hasAgentIdentity = computed(() => {
  return currentPermissions.value.some((permission) => String(permission).startsWith("agent:"));
});
const hasOperationCenterIdentity = computed(() => {
  return currentPermissions.value.some((permission) => String(permission).startsWith("operation:"));
});
const userAccountSnapshot = ref<AdminRecord | null>(null);
watch(
  () => store.data?.account,
  (account) => {
    if (account && typeof account === "object") {
      userAccountSnapshot.value = account as AdminRecord;
    }
  },
  { immediate: true }
);
const selectedMembershipCycle = ref<MembershipCycleId>("monthly");
const selectedMembershipMode = ref<MembershipMode>("identity");
const selectedMembershipPlanId = ref("pro");
const rechargePackages = [
  { id: "recharge_small", name: "小额包", amount: 19.9, points: 2500 },
  { id: "recharge_standard", name: "标准包", amount: 99, points: 15000 },
  { id: "recharge_business", name: "商业包", amount: 299, points: 50000 },
  { id: "recharge_enterprise", name: "企业包", amount: 999, points: 200000 }
];
const quickRechargeAmounts = rechargePackages.map(item => item.amount);
const selectedRechargeAmount = ref(99);
const customRechargeAmount = ref("");
const selectedPaymentMethod = ref<PaymentMethodId>("wechat");
const paymentMethodOptions = [
  { id: "wechat" as PaymentMethodId, label: "微信支付", icon: Wallet },
  { id: "alipay" as PaymentMethodId, label: "支付宝", icon: Money },
  { id: "cash" as PaymentMethodId, label: "现金", icon: Tickets }
];
const rechargeAmountYuan = computed(() => {
  const custom = Number(String(customRechargeAmount.value).replace(/[^\d.]/g, ""));
  if (custom > 0) return custom;
  return Number(selectedRechargeAmount.value || 0);
});
const membershipBillingCycles: MembershipBillingCycle[] = [
  { id: "monthly", label: "连续包月", discount: "7折" },
  { id: "yearly", label: "连续包年", discount: "65折" },
  { id: "single", label: "单月购买", discount: "8折" }
];
const userMembershipPlans: UserMembershipPlan[] = [
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
const identityPackageCards: UserIdentityPackage[] = [
  {
    id: "member_996",
    planId: "plan_ai_creator_996",
    endpoint: "/orders/create",
    name: "996 AI 创作会员包",
    badge: "会员身份",
    price: 996,
    originalText: "含 400 元 Token",
    tokenAmount: 40000,
    note: "开通 Pro 会员权益，获得 400 元 AI 点数 / Token。",
    ruleText: "有推荐代理时按固定分润规则自动记账。",
    actionText: "开通会员包",
    featured: true,
    features: ["会员身份升级", "Token 即时到账", "订单可追踪分润"]
  },
  {
    id: "agent_996",
    planId: "plan_agent_join_996",
    endpoint: "/agent/join-order",
    name: "996 代理商开通包",
    badge: "代理商身份",
    price: 996,
    originalText: "含 200 元 Token",
    tokenAmount: 20000,
    note: "开通代理商身份，生成邀请码和代理中心入口。",
    ruleText: "直属代理奖励 300 元，运营中心奖励 200 元，上级代理不奖励。",
    actionText: "开通代理商",
    features: ["代理身份激活", "专属邀请码", "佣金明细可查"]
  },
  {
    id: "operation_center_5000",
    planId: "plan_operation_center_5000",
    endpoint: "/operation-center/join-order",
    name: "5000 运营中心开通包",
    badge: "运营中心身份",
    price: 5000,
    originalText: "平台运营权益",
    tokenAmount: 0,
    note: "开通运营中心身份，用于承接区域代理和订单分润。",
    ruleText: "开通费默认计入平台收入，不产生代理奖励。",
    actionText: "开通运营中心",
    features: ["运营中心身份", "代理归属管理", "运营分润汇总"]
  }
];
const userMembershipCurrentPlan = computed(() => {
  const name = sidebarPlan.value.name;
  return name === "体验版" ? "暂无订阅计划" : name;
});
const membershipPlanCards = computed<UserMembershipPlanCard[]>(() => userMembershipPlans.map((plan) => {
  const cycle = selectedMembershipCycle.value;
  const price = plan.prices[cycle];
  const originalPrice = plan.originalPrices[cycle];
  const planId = plan.planIds[cycle];
  const unit = plan.oneTime ? "次" : cycle === "yearly" ? "年" : "月";
  const creditUnit = plan.oneTime ? "一次性" : plan.id === "trial" ? "7天" : "月";
  const credits = plan.credits;
  const images = plan.images;
  const videos = plan.videos;
  const cycleNote = plan.oneTime
    ? `一次性 ￥${price}，开通 Pro 会员并发放 ${formatNumber(credits)} 点。`
    : cycle === "yearly"
    ? `连续包年 ￥${price}，每月发放 ${formatNumber(credits)} 点。`
    : cycle === "single"
      ? `单月购买 ￥${price}，本月发放 ${formatNumber(credits)} 点。`
      : `连续包月 ￥${price}，每月发放 ${formatNumber(credits)} 点。`;
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
      `每${creditUnit}${formatNumber(credits)}积分`,
      `适合：${plan.audience}`,
      `并发任务：${plan.concurrency}`,
      "下载权限：无水印下载",
      plan.custom ? "专属模型路由与商务支持" : "生成内容可商用"
    ]
  };
}));
const standardMembershipPlanCards = computed(() => membershipPlanCards.value.filter((plan) => !plan.custom));
const customMembershipPlanCards = computed(() => membershipPlanCards.value.filter((plan) => plan.custom));

function selectMembershipPlan(plan: UserMembershipPlanCard) {
  selectedMembershipPlanId.value = plan.id;
}
const userMembershipOrders = computed<AdminRecord[]>(() => {
  const data = store.data as { orders?: unknown[] };
  const orders = Array.isArray(data.orders) ? data.orders : [];
  return orders.map((item) => {
    const row = item as AdminRecord;
    return {
      ...row,
      orderTypeText: membershipOrderType(row.orderType),
      paymentMethodText: membershipPaymentMethod(row.paymentMethod),
      plan: row.plan || row.planId || "-"
    };
  });
});

function membershipOrderType(value: unknown) {
  const type = String(value || "").toUpperCase();
  if (type === "COMPUTE_RECHARGE") return "点数充值";
  if (type === "PLAN_ORDER") return "套餐订阅";
  if (type === "USER_RECHARGE_DIRECT") return "会员/充值-直属";
  if (type === "USER_RECHARGE_SECOND_LEVEL") return "会员/充值-二级";
  if (type === "PLATFORM_DIRECT_USER_RECHARGE") return "平台直购";
  if (type === "AGENT_JOIN") return "代理商开通";
  if (type === "OPERATION_CENTER_JOIN") return "运营中心开通";
  if (type === "RENEWAL") return "续费订单";
  return type || "订单明细";
}

function membershipPaymentMethod(value: unknown) {
  const method = String(value || "").toLowerCase();
  if (method === "wechat") return "微信支付";
  if (method === "alipay") return "支付宝";
  if (method === "cash") return "现金";
  return method || "-";
}

function selectRechargeAmount(amount: number) {
  selectedRechargeAmount.value = amount;
  customRechargeAmount.value = "";
}

function selectedRechargePackageId() {
  if (customRechargeAmount.value.trim()) return "";
  return rechargePackages.find(item => Math.abs(item.amount - selectedRechargeAmount.value) < 0.001)?.id || "";
}

async function createUserRechargeOrder() {
  if (!ensureWorkspaceAuth("recharge", "userMembership")) return;
  try {
    const amount = rechargeAmountYuan.value;
    if (!amount || amount <= 0) {
      ElMessage.error("请选择或输入充值金额");
      return;
    }
    await ElMessageBox.confirm(`确认创建 ￥${amount.toFixed(2)} 充值订单？订单创建后需要主控后台标记收款，点数才会到账。`, "确认支付", {
      confirmButtonText: "创建订单",
      cancelButtonText: "取消",
      type: "warning"
    });
    const response = await adminRequest<{ item?: AdminRecord; rechargePoints?: number; message?: string }>({
      method: "POST",
      url: "/points/recharge-orders",
      data: { amountCents: Math.round(amount * 100), rechargePackageId: selectedRechargePackageId(), paymentMethod: selectedPaymentMethod.value }
    });
    ElMessage.success(response.message || `充值订单已创建，预计到账 ${formatNumber(Number(response.rechargePoints || 10000))} 点`);
    await store.loadActiveModule({ preferCache: false });
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "创建充值订单失败");
    }
  }
}

async function createUserSubscriptionOrder(plan: UserMembershipPlanCard) {
  if (!ensureWorkspaceAuth("open_member_center", "userMembership")) return;
  try {
    selectMembershipPlan(plan);
    if (plan.custom) {
      ElMessage.info("企业版为定制套餐，请由主控后台创建专属报价和模型分组。");
      return;
    }
    await ElMessageBox.confirm(`确认创建 ${plan.name} ${plan.unit}卡订阅订单 ￥${plan.price}？订单创建后需要主控后台标记收款。`, "确认订阅", {
      confirmButtonText: "创建订单",
      cancelButtonText: "取消",
      type: "success"
    });
    const response = await adminRequest<{ item?: AdminRecord; message?: string }>({
      method: "POST",
      url: "/points/subscription-orders",
      data: {
        planId: plan.planId,
        amountCents: Math.round(plan.price * 100),
        paymentMethod: selectedPaymentMethod.value
      }
    });
    ElMessage.success(response.message || "订阅订单已创建");
    selectedMembershipMode.value = "subscribe";
    await store.loadActiveModule();
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "创建订阅订单失败");
    }
  }
}

function identityPackageIsActive(pack: UserIdentityPackage) {
  if (pack.id.startsWith("agent_")) return hasAgentIdentity.value;
  if (pack.id.startsWith("operation_center")) return hasOperationCenterIdentity.value;
  const level = String(currentAdmin.value?.memberLevel || "").toUpperCase();
  const planId = String(currentAdmin.value?.planId || "");
  return level !== "" && level !== "FREE" || planId === pack.planId || planId === "plan_pro";
}

function identityPackageActionText(pack: UserIdentityPackage) {
  if (!identityPackageIsActive(pack)) return pack.actionText;
  if (pack.id.startsWith("agent_")) return "进入代理后台";
  if (pack.id.startsWith("operation_center")) return "进入运营中心";
  return "已开通会员";
}

async function handleIdentityPackageAction(pack: UserIdentityPackage) {
  if (!identityPackageIsActive(pack)) {
    await createUserIdentityOrder(pack);
    return;
  }
  if (pack.id.startsWith("agent_")) {
    await selectAdminModule("partnerDashboard");
    return;
  }
  if (pack.id.startsWith("operation_center")) {
    await selectAdminModule("operationCenterDashboard");
    return;
  }
  ElMessage.success("当前账号已具备会员身份");
}

async function createUserIdentityOrder(pack: UserIdentityPackage) {
  try {
    await ElMessageBox.confirm(`确认创建 ${pack.name} 订单 ￥${pack.price}？支付回调或主控确认收款后，身份和权益会自动生效。`, "确认开通", {
      confirmButtonText: "创建订单",
      cancelButtonText: "取消",
      type: "success"
    });
    const response = await adminRequest<{ item?: AdminRecord; message?: string }>({
      method: "POST",
      url: pack.endpoint,
      data: {
        planId: pack.planId,
        amountCents: Math.round(pack.price * 100),
        paymentMethod: selectedPaymentMethod.value,
        idempotencyKey: `${pack.id}-${Date.now()}`
      }
    });
    const order = response.item || {};
    ElMessage.success(`${pack.name}订单已创建：${String(order.id || "待支付")}`);
    selectedMembershipMode.value = "identity";
    await store.loadActiveModule({ preferCache: false });
    await loadUserAccountSnapshot();
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "创建身份订单失败");
    }
  }
}

const isAgentConsole = ref(typeof window !== "undefined" && window.location.pathname.startsWith("/agent"));
const initialBrowserPath = typeof window !== "undefined" ? window.location.pathname : "";
const userConsoleBasePath = initialBrowserPath === "/" ? "/" : initialBrowserPath.startsWith("/workspace") ? "/workspace" : "/app";
const isUserConsole = ref(initialBrowserPath === "/" || initialBrowserPath.startsWith("/app") || initialBrowserPath.startsWith("/workspace"));
const isFeishuConnectorSetupRoute = typeof window !== "undefined" && ["/app/enterprise/feishu", "/workspace/enterprise/feishu"].includes(window.location.pathname);
const isConnectorAuthorizationRoute = typeof window !== "undefined" && ["/app/enterprise/connectors", "/workspace/enterprise/connectors"].includes(window.location.pathname);
const authReady = ref(false);
const workspaceLoginOpen = computed({
  get: () => authStore.loginModalVisible,
  set: (visible: boolean) => visible ? authStore.openLogin() : authStore.closeLogin()
});
const authSessionVersion = ref(0);
const isGuestUser = computed(() => {
  void authSessionVersion.value;
  return isUserConsole.value && !hasAuthToken();
});
const guestVisibleModuleIds = new Set(["userDashboard", "userAiImage", "userAgentCenter", "userWirelessCanvas", "userVideoGeneration", "userPptGeneration", "userWorks"]);
const workspaceGuestDraftKey = "zhiqiyun.pc.workspace.pending-draft.v1";
function openWorkspaceLogin() {
  authStore.openLogin({ redirectUrl: currentWorkspaceRoute() });
}
function currentWorkspaceRoute() {
  if (typeof window === "undefined") return "/";
  return safeInternalRedirect(`${window.location.pathname}${window.location.search}${window.location.hash}`, "/");
}
function workspaceDraftPayload(moduleId = store.activeModuleId) {
  return {
    moduleId,
    imageForm: { ...onlineImageForm.value },
    imageMode: aiPlaygroundMode.value,
    referenceImages: aiReferenceImages.value.map((item) => ({
      id: item.id,
      name: item.name,
      remoteUrl: item.remoteUrl && !item.remoteUrl.startsWith("blob:") && !item.remoteUrl.startsWith("data:") ? item.remoteUrl : ""
    })),
    videoPrompt: videoPrompt.value,
    videoModel: selectedVideoModel.value,
    videoMode: videoStudioMode.value,
    videoDuration: videoDuration.value,
    videoRatio: videoRatio.value,
    videoResolution: videoResolution.value,
    videoGenerateAudio: videoGenerateAudio.value,
    rechargeAmount: selectedRechargeAmount.value,
    customRechargeAmount: customRechargeAmount.value,
    paymentMethod: selectedPaymentMethod.value
  };
}
function saveWorkspaceGuestDraft(moduleId = store.activeModuleId) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(workspaceGuestDraftKey, JSON.stringify({
    ...workspaceDraftPayload(moduleId),
    createdAt: Date.now(),
  }));
}
function applyWorkspaceDraft(draft: Record<string, unknown>) {
  if (draft.imageForm && typeof draft.imageForm === "object") onlineImageForm.value = { ...onlineImageForm.value, ...draft.imageForm } as typeof onlineImageForm.value;
  if (typeof draft.imageMode === "string" && ["gallery", "agent"].includes(draft.imageMode)) aiPlaygroundMode.value = draft.imageMode;
  if (typeof draft.videoPrompt === "string") videoPrompt.value = draft.videoPrompt;
  if (typeof draft.videoModel === "string") selectedVideoModel.value = draft.videoModel;
  if (typeof draft.videoMode === "string") videoStudioMode.value = draft.videoMode;
  if (typeof draft.videoDuration === "number") videoDuration.value = draft.videoDuration;
  if (typeof draft.videoRatio === "string") videoRatio.value = draft.videoRatio;
  if (typeof draft.videoResolution === "string") videoResolution.value = draft.videoResolution;
  if (typeof draft.videoGenerateAudio === "boolean") videoGenerateAudio.value = draft.videoGenerateAudio;
  if (typeof draft.rechargeAmount === "number") selectedRechargeAmount.value = draft.rechargeAmount;
  if (typeof draft.customRechargeAmount === "string") customRechargeAmount.value = draft.customRechargeAmount;
  if (draft.paymentMethod === "cash" || draft.paymentMethod === "wechat" || draft.paymentMethod === "alipay") selectedPaymentMethod.value = draft.paymentMethod;
  const storedReferences = Array.isArray(draft.referenceImages) ? draft.referenceImages : [];
  if (storedReferences.length && !aiReferenceImages.value.length) {
    aiReferenceImages.value = storedReferences.flatMap((raw) => {
      if (!raw || typeof raw !== "object") return [];
      const item = raw as Record<string, unknown>;
      const remoteUrl = String(item.remoteUrl || "");
      if (!remoteUrl) return [];
      return [{ id: String(item.id || `restored-${Date.now()}`), name: String(item.name || "参考图"), url: remoteUrl, remoteUrl, uploading: false }];
    });
  }
}
function restoreWorkspaceGuestDraft() {
  if (typeof window === "undefined" || !hasAuthToken()) return "";
  try {
    const draft = JSON.parse(window.localStorage.getItem(workspaceGuestDraftKey) || "null") as ({ moduleId?: string; createdAt?: number } & Record<string, unknown>) | null;
    if (!draft || !draft.createdAt || Date.now() - draft.createdAt > 30 * 60 * 1000) return "";
    applyWorkspaceDraft(draft);
    window.localStorage.removeItem(workspaceGuestDraftKey);
    return draft.moduleId && guestVisibleModuleIds.has(draft.moduleId) ? draft.moduleId : "";
  } catch {
    window.localStorage.removeItem(workspaceGuestDraftKey);
    return "";
  }
}
function ensureWorkspaceAuth(action: ProtectedAction, moduleId = store.activeModuleId, extraPayload: Record<string, unknown> = {}) {
  if (hasAuthToken()) return true;
  if (action === "generate_image" || action === "generate_video") {
    trackWebGuestExperience("guest_click_generate", moduleId, { action });
  }
  saveWorkspaceGuestDraft(moduleId);
  const loginAlreadyOpen = workspaceLoginOpen.value;
  authStore.requireAuth({
    action,
    route: currentWorkspaceRoute(),
    payload: { ...workspaceDraftPayload(moduleId), ...extraPayload, moduleId },
    autoResume: false
  });
  if (!loginAlreadyOpen) trackWebGuestExperience("login_modal_show", moduleId, { action });
  const pending = authStore.pendingAction;
  if (pending) {
    const localReferences = aiReferenceImages.value.flatMap((item) => item.file instanceof File
      ? [{ id: item.id, name: item.name, file: item.file }]
      : []);
    if (localReferences.length) {
      void writePendingReferenceImages(pending.id, localReferences, pending.expiresAt).catch(() => undefined);
    }
  }
  return false;
}

async function loadGuestPublicCases() {
  if (publicOfficialCasesLoading.value || publicOfficialCases.value.length) return;
  publicOfficialCasesLoading.value = true;
  try {
    const response = await adminRequest<{ items?: AdminRecord[] }>({ method: "GET", url: "/public/cases", authMode: "none" });
    publicOfficialCases.value = Array.isArray(response.items) ? response.items : [];
  } catch {
    publicOfficialCases.value = [];
  } finally {
    publicOfficialCasesLoading.value = false;
  }
}

async function openMyWorks() {
  if (!ensureWorkspaceAuth("save_work", "userWorks", { openMine: true })) return;
  worksSourceTab.value = "mine";
  await store.loadActiveModule({ preferCache: false });
}

async function refreshWorksCenter() {
  if (worksSourceTab.value === "official" || isGuestUser.value) {
    publicOfficialCases.value = [];
    await loadGuestPublicCases();
    return;
  }
  await store.loadActiveModule({ preferCache: false });
}
const authPath = ref(typeof window !== "undefined" ? window.location.pathname.replace(/\/$/, "") || "/" : "");
const authConsolePrefix = computed(() => authPath.value.startsWith("/agent") ? "/agent" : authPath.value.startsWith("/admin") ? "/admin" : "");
const authLoginHref = computed(() => `${authConsolePrefix.value}/login` || "/login");
const authRegisterHref = computed(() => `${authConsolePrefix.value}/register` || "/register");
const isLoginRoute = computed(() => authPath.value === authLoginHref.value);
const isRegisterRoute = computed(() => authPath.value === authRegisterHref.value);
const isAuthRoute = computed(() => isLoginRoute.value || isRegisterRoute.value);
const initialInviteCode = typeof window !== "undefined" ? new URLSearchParams(window.location.search).get("invite") || "" : "";
const registerForm = ref({
  username: "",
  email: "",
  password: "",
  confirmPassword: "",
  inviteCode: initialInviteCode
});
const registerAgreementAccepted = ref(false);
const authSubmitting = ref(false);
const mobileDrawerOpen = ref(false);
const desktopSidebarCollapsed = ref(false);
const tabsScrollRef = ref<HTMLElement | null>(null);
const openTabStorageKey = isUserConsole.value ? "xianzhi-user-open-tabs" : isAgentConsole.value ? "xianzhi-agent-open-tabs" : "xianzhi-admin-open-tabs";
const activeTabStorageKey = isUserConsole.value ? "xianzhi-user-active-tab" : isAgentConsole.value ? "xianzhi-agent-active-tab" : "xianzhi-admin-active-tab";
const imageWorkspaceModuleIds = ["userAiImage"];
const adminModuleIds = modules.map((item) => item.id).filter((id) => !agentModuleIds.includes(id) && !operationCenterModuleIds.includes(id) && !userModuleIds.includes(id));
const userConsoleModuleIds = [...userModuleIds, ...agentModuleIds, ...operationCenterModuleIds];
const allowedModuleIds = isUserConsole.value ? userConsoleModuleIds : isAgentConsole.value ? agentModuleIds : adminModuleIds;
const defaultOpenTabIds = isUserConsole.value ? ["userDashboard", "userAiImage", "userWirelessCanvas", "userVideoGeneration", "userPptGeneration"] : isAgentConsole.value ? ["partnerDashboard", "partnerCustomers"] : ["analysis"];
const enterpriseRoutePath = ref(typeof window !== "undefined" ? `${window.location.pathname}${window.location.search}` : "/admin/enterprises");
function isPptGenerationPath(pathname: string) {
  const normalizedPath = canonicalUserConsolePath(pathname).replace(/\/$/, "");
  return (
    normalizedPath === "/app/ppt-generation" ||
    normalizedPath === "/app/ai-ppt" ||
    normalizedPath.startsWith("/app/ppt-generation/generate/") ||
    normalizedPath.startsWith("/app/ppt-generation/presentation/")
  );
}

function canonicalUserConsolePath(pathname: string) {
  return pathname === "/workspace" || pathname.startsWith("/workspace/")
    ? `/app${pathname.slice("/workspace".length)}`
    : pathname;
}

function activeUserConsolePath(pathname: string) {
  if (userConsoleBasePath === "/") return "/";
  return userConsoleBasePath === "/workspace" && (pathname === "/app" || pathname.startsWith("/app/"))
    ? `/workspace${pathname.slice(4)}`
    : pathname;
}

function moduleIdFromLocationPath() {
  if (typeof window === "undefined") return "";
  return resolveModuleIdFromPath(canonicalUserConsolePath(window.location.pathname));
}

function syncUserModulePath(moduleId: string) {
  if (typeof window === "undefined" || !isUserConsole.value) return;
  const nextPath = activeUserConsolePath(resolveModulePath(moduleId) || "/app");
  const currentPath = window.location.pathname.replace(/\/$/, "");
  if (moduleId === "userPptGeneration" && isPptGenerationPath(currentPath)) return;
  if (currentPath === nextPath) return;
  window.history.pushState({}, "", nextPath);
}

function syncAdminModulePath(moduleId: string) {
  if (typeof window === "undefined" || isUserConsole.value || isAgentConsole.value) return;
  const enterpriseId = window.location.pathname.match(/^\/admin\/enterprises\/([^/]+)/)?.[1];
  const nextPath = resolveModulePath(moduleId, { enterpriseId });
  if (!nextPath || window.location.pathname.replace(/\/$/, "") === nextPath) {
    if (enterpriseModuleIds.includes(moduleId)) enterpriseRoutePath.value = `${window.location.pathname}${window.location.search}`;
    return;
  }
  window.history.pushState({}, "", nextPath);
  if (enterpriseModuleIds.includes(moduleId)) enterpriseRoutePath.value = nextPath;
}

async function navigateEnterpriseRoute(payload: { path: string; moduleId: string }) {
  if (typeof window !== "undefined") {
    const current = `${window.location.pathname}${window.location.search}`;
    if (current !== payload.path) window.history.pushState({}, "", payload.path);
    enterpriseRoutePath.value = payload.path;
  }
  await selectAdminModule(payload.moduleId);
}

function handleAdminEnterpriseHistoryPopState() {
  if (typeof window === "undefined") return;
  const moduleId = resolveModuleIdFromPath(canonicalUserConsolePath(window.location.pathname));
  if (!moduleId) return;
  if (!allowedModuleIds.includes(moduleId)) return;
  if (enterpriseModuleIds.includes(moduleId)) enterpriseRoutePath.value = `${window.location.pathname}${window.location.search}`;
  ensureOpenTab(moduleId);
  void store.selectModule(moduleId);
}

function initialActiveModuleId() {
  if (typeof window === "undefined") return defaultOpenTabIds[0];
  const routeModuleId = moduleIdFromLocationPath();
  if (routeModuleId && allowedModuleIds.includes(routeModuleId)) return routeModuleId;
  const currentPath = window.location.pathname.replace(/\/$/, "");
  if (isUserConsole.value && currentPath && currentPath !== userConsoleBasePath) {
    window.history.replaceState({}, "", userConsoleBasePath);
    return defaultOpenTabIds[0];
  }
  const saved = window.localStorage.getItem(activeTabStorageKey) || "";
  const savedModule = adminModuleById(saved);
  const savedNeedsEnterpriseContext = savedModule?.path?.includes(":enterpriseId") === true;
  if (saved && allowedModuleIds.includes(saved) && savedModule && !savedNeedsEnterpriseContext) return saved;
  return defaultOpenTabIds[0];
}

store.activeModuleId = initialActiveModuleId();
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
  if (store.activeModuleId !== "userWirelessCanvas") return;
  wirelessCanvasFrameLoaded.value = false;
  clearWirelessCanvasLoadTimer();
  if (wirelessCanvasFrameSrc.value) return;

  const loadFrame = () => {
    if (store.activeModuleId === "userWirelessCanvas") {
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

watch(() => store.activeModuleId, (moduleId) => {
  if (moduleId === "userWirelessCanvas") {
    scheduleWirelessCanvasFrameLoad();
    return;
  }
  clearWirelessCanvasLoadTimer();
}, { immediate: true });

function resolveOpenTabs() {
  if (typeof window === "undefined") return modules.filter((item) => defaultOpenTabIds.includes(item.id));

  try {
    const saved = JSON.parse(window.localStorage.getItem(openTabStorageKey) || "[]") as string[];
    const tabIds = Array.from(new Set([...defaultOpenTabIds, ...saved])).filter((id) => allowedModuleIds.includes(id));
    const tabs = tabIds.map((id) => modules.find((item) => item.id === id)).filter(Boolean) as typeof modules;
    return tabs.length ? tabs : modules.filter((item) => defaultOpenTabIds.includes(item.id));
  } catch {
    return modules.filter((item) => defaultOpenTabIds.includes(item.id));
  }
}

const openTabs = ref(resolveOpenTabs());

const iconMap = {
  enterpriseList: Goods,
  enterpriseDetail: Goods,
  enterpriseCertifications: Check,
  enterpriseMembers: UserFilled,
  enterprisePackage: Tickets,
  enterpriseCompute: Cpu,
  enterpriseTransactions: Money,
  enterpriseOrders: Document,
  enterpriseAiCapabilities: Monitor,
  enterpriseAiEmployees: Cpu,
  enterpriseKnowledgeBases: Collection,
  enterpriseAttribution: Connection,
  enterpriseRelationships: Link,
  enterpriseRisk: Lock,
  enterpriseAuditLogs: DataAnalysis,
  analysis: DataAnalysis,
  workbench: House,
  dashboard: DataAnalysis,
  customers: User,
  customerAttributions: Connection,
  channels: Operation,
  operationCenters: Connection,
  products: Goods,
  plans: Tickets,
  orders: Money,
  usage: DataAnalysis,
  tokenRecords: Tickets,
  billingOverview: DataAnalysis,
  billingRules: Tickets,
  billingProviderCosts: Money,
  billingEvents: DataAnalysis,
  billingReconciliation: Check,
  billingWalletLedger: Wallet,
  billingCustomers: User,
  billingProducts: Goods,
  billingSubscriptions: Tickets,
  billingCoupons: Tickets,
  billingInvoices: Document,
  billingCreditNotes: Money,
  billingPaymentRequests: Clock,
  billingPayments: Wallet,
  marketingAgentLevels: Connection,
  commissions: Wallet,
  commissionRecords: Money,
  aiCapabilities: Cpu,
  aiCapabilityModels: Monitor,
  aiCapabilitySchemas: Document,
  aiCapabilityLimits: Lock,
  aiCapabilityChannels: Link,
  aiCapabilityLogs: DataAnalysis,
  apiSettings: Setting,
  system: Setting,
  departments: Collection,
  userManagement: UserFilled,
  menuManagement: Operation,
  partnerDashboard: DataAnalysis,
  partnerCustomers: User,
  partnerOrders: Money,
  partnerUsage: DataAnalysis,
  partnerCommissions: Wallet,
  partnerChannels: Connection,
  partnerMaterials: Collection,
  partnerAccount: Setting,
  operationCenterDashboard: DataAnalysis,
  operationCenterAgents: Connection,
  operationCenterOrders: Money,
  operationCenterCommissions: Wallet,
  userDashboard: House,
  userAiImage: Plus,
  userAgentCenter: Cpu,
  userWirelessCanvas: Collection,
  userVideoGeneration: Monitor,
  userPptGeneration: Document,
  userWorks: Collection,
  userUsage: DataAnalysis,
  userMembership: Tickets,
  userOrders: Document
};

const columnLabels: Record<string, string> = {
  id: "ID",
  category: "类型",
  secret: "密钥",
  name: "名称",
  email: "邮箱",
  mobileMasked: "手机号",
  loginMethods: "登录方式",
  wechatBinding: "微信状态",
  wechatOpenIdMasked: "OpenID",
  role: "角色",
  plan: "套餐",
  planId: "套餐 ID",
  planCode: "套餐编码",
  planVersion: "套餐版本",
  planType: "套餐类型",
  billingMode: "计费方式",
  skuCode: "SKU 编码",
  externalId: "外部订阅 ID",
  subscription: "订阅",
  subscriptionId: "订阅 ID",
  subscriptionStatus: "订阅状态",
  billingStatus: "计费状态",
  billingCycle: "计费周期",
  cyclePolicy: "结算/有效期",
  billingTime: "计费时间",
  interval: "周期",
  onTerminationInvoice: "终止开票",
  onTerminationCredit: "终止贷项",
  startedAt: "开始时间",
  currentPeriodStart: "周期开始",
  currentPeriodEnd: "周期结束",
  monthlyAmountCents: "月经常收入",
  prepaidBalanceCents: "预付余额",
  lifetimeUsageCents: "累计用量金额",
  walletCode: "钱包编码",
  netPaymentTerm: "账期天数",
  invoiceGracePeriod: "开票宽限",
  customer: "客户",
  customerId: "客户 ID",
  usageCount: "消费笔数",
  orderCount: "订单数",
  consumedCents: "消费金额",
  latestTaskId: "最近任务",
  latestUsageAt: "最近消费",
  latestModel: "最近模型",
  customerValue: "客户价值",
  averagePointCost: "平均扣点",
  modelRoute: "模型路由",
  modelGroup: "模型分组",
  modelChannel: "模型渠道",
  modelKeyStatus: "Key 状态",
  modelRouteId: "路由 ID",
  modelApiKeyId: "API Key ID",
  balanceBefore: "扣前点数",
  balanceAfter: "扣后点数",
  taskId: "任务 ID",
    orderType: "订单类型",
  agentName: "代理商",
  bizType: "业务类型",
  bizId: "业务 ID",
  ruleId: "规则 ID",
  source: "来源",
  period: "结算月份",
  commissionCount: "佣金笔数",
  withdrawalCount: "提现笔数",
  withdrawalCents: "提现金额",
  pendingCents: "待结算金额",
  frozenCents: "冻结金额",
  totalIncomeCents: "累计入账",
  totalWithdrawCents: "累计提现",
  netPayableCents: "应打款金额",
  paymentMethodText: "支付方式",
  settlementSource: "来源",
  relatedAmountCents: "关联金额",
  commissionRate: "佣金比例",
  reviewedAt: "审核时间",
  customerGroup: "客户分组",
  code: "编码",
  product: "产品",
  metric: "指标",
  metricCode: "指标编码",
  billableMetricCode: "计量指标编码",
  aggregationType: "聚合方式",
  fieldName: "计量字段",
  expression: "表达式",
  recurring: "周期计量",
  rounding: "取整规则",
  chargeModels: "计费模型",
  quantity: "数量",
  units: "计费单位",
  eventsCount: "事件数",
  costCents: "成本",
  subtotalAmountCents: "小计",
  couponsAmountCents: "优惠抵扣",
  taxesAmountCents: "税费",
  creditNotesAmountCents: "贷项抵扣",
  prepaidCreditAmountCents: "预付抵扣",
  totalDueAmountCents: "应付金额",
  invoiceNo: "账单号",
  invoiceId: "账单 ID",
  invoiceType: "账单类型",
  invoiceStatus: "开票状态",
  paymentStatus: "支付状态",
  taxStatus: "税务状态",
  readyToFinalize: "可出账",
  billingPeriod: "账单周期",
  dueAt: "到期日",
  paymentMethod: "付款方式",
  invoiceTitle: "发票抬头",
  taxNumber: "税号",
  transactionId: "幂等交易 ID",
  occurredAt: "发生时间",
  unitAmountCents: "单价",
  chargeModel: "计费模型",
  freeQuota: "免费额度",
  includedQuota: "内含额度",
  freeUnits: "免费量",
  unitPriceCents: "单价",
  baseAmountCents: "固定费用",
  overageUnitPriceCents: "超额单价",
  minAmountCents: "最低金额",
  payInAdvance: "预付",
  invoiceable: "可开票",
  prorated: "按比例计费",
  pricingGroupKeys: "分组计价键",
  taxes: "税种",
  coupon: "优惠券",
  couponTargets: "优惠目标",
  entitlements: "权益",
  entitlementSnapshot: "权益快照",
  targetMetrics: "目标指标",
  balanceCents: "钱包余额",
  consumedAmountCents: "已消费",
  ongoingUsageBalanceCents: "本期占用",
  paidTopUpMinAmountCents: "最低充值",
  paymentMethodType: "支付方式类型",
  rateAmount: "兑换倍率",
  couponType: "优惠类型",
  percentageRate: "折扣比例",
  frequency: "频率",
  frequencyDuration: "周期次数",
  reusable: "可复用",
  targets: "适用范围",
  creditStatus: "贷项状态",
  refundStatus: "退款状态",
  reason: "原因",
  creditAmountCents: "贷项金额",
  refundAmountCents: "退款金额",
  offsetAmountCents: "抵扣金额",
  balanceAmountCents: "剩余金额",
  number: "编号",
  invoices: "关联账单",
  readyForPaymentProcessing: "可处理支付",
  dunningCampaign: "催收策略",
  paymentRequestId: "付款请求 ID",
  payableType: "支付对象",
  provider: "支付提供方",
  pointsAvailable: "余额",
  status: "状态",
  level: "等级",
  identity: "身份",
  openMethod: "开通方式",
  audience: "适合对象",
  permissions: "权益/权限",
  limitations: "限制",
  membershipCommission: "会员套餐返佣",
  rechargeCommission: "点数充值返佣",
  enterpriseCommission: "企业项目返佣",
  regionalRebate: "区域团队返点",
  openCondition: "开通条件",
  keepCondition: "保级条件",
  manualReview: "人工审核",
  levelLabel: "代理商等级",
  inviteCode: "邀请码",
  referredBy: "来源代理用户 ID",
  sourceAgentName: "来源渠道",
  sourceInviteCode: "来源邀请码",
  sourceChannelLevel: "渠道等级",
  sourceParentAgentName: "上级代理",
  type: "类型",
  usage: "用量",
  priceCents: "价格",
  grantPoints: "权益点数",
  amountCents: "金额",
  createdAt: "创建时间",
  priority: "优先级",
  baseUrl: "Base URL",
  quotaLimit: "配额",
  orderId: "订单 ID",
  commissionCents: "佣金",
  channel: "渠道",
  customers: "客户",
  children: "下级代理",
  item: "项目",
  value: "内容",
  availablePoints: "可用点数",
  todayGenerations: "今日生成",
  succeededGenerations: "成功生成",
  totalPointCost: "消耗点数",
  pointCost: "消耗点数",
  model: "模型",
  mediaType: "类型",
  thumbnailUrl: "缩略图",
  module_code: "模块编码",
  model_name: "模型",
  billing_type: "计费类型",
  tenant_id: "租户",
  agent_id: "代理",
  operation_center_id: "运营中心",
  upstream_provider: "上游",
  user_charge_amount: "用户扣费",
  upstream_cost: "上游成本",
  platform_profit: "平台利润"
};

const iconFor = (id: string) => iconMap[id as keyof typeof iconMap] || DataAnalysis;
const apiChannels = computed(() => settingsList("apiChannels"));
const apiModels = computed(() => settingsList("apiModels"));
const apiKeys = computed(() => settingsList("apiKeys"));
const apiGroups = computed(() => settingsList("customerGroups"));
const activeApiChannel = computed<AdminRecord>(() => {
  const channels = apiChannels.value as AdminRecord[];
  return channels.find((channel) => Boolean(channel.primary) || String(channel.status || "").toUpperCase() === "ACTIVE") || channels[0] || {};
});
const apiAvailableModelCount = computed(() => {
  const models = new Set<string>();
  apiModels.value.forEach((item) => {
    const row = item as AdminRecord;
    if (row.model) models.add(String(row.model));
  });
  apiChannels.value.forEach((item) => {
    const row = item as AdminRecord;
    if (Array.isArray(row.models)) row.models.forEach((model) => models.add(String(model)));
  });
  return models.size;
});
const apiConfiguredKeyCount = computed(() => apiKeys.value.filter((item) => String((item as AdminRecord).status || "").toUpperCase() === "ACTIVE").length);
const apiAccessScopes = [
  { name: "企业版客户", desc: "开放高质量图片、视频和 Agent 模型，按套餐权益扣点。" },
  { name: "渠道版客户", desc: "支持二级渠道 Key、分组倍率、模型白名单和独立用量日志。" },
  { name: "个人试用", desc: "只开放低成本与演示模型，避免试用流量击穿上游成本。" }
];
const apiComfyInstances = computed(() => {
  const instances = activeApiChannel.value.comfyInstances;
  return Array.isArray(instances) ? instances.map((item) => String(item)).filter(Boolean) : [];
});
const apiSourceModels = computed(() => {
  const models = new Set<string>();
  if (Array.isArray(activeApiChannel.value.models)) activeApiChannel.value.models.forEach((model) => models.add(String(model)));
  apiModels.value.forEach((item) => {
    const row = item as AdminRecord;
    if (row.model) models.add(String(row.model));
  });
  return Array.from(models).slice(0, 16);
});
const apiReferenceChannels = computed<AdminRecord[]>(() => {
  const savedChannels = [...(apiChannels.value as AdminRecord[])]
    .filter((channel) => String(channel.status || "").toUpperCase() !== "DISABLED")
    .sort((a, b) => apiProviderPriority(a) - apiProviderPriority(b));
  const channels = [...apiPendingProviders.value, ...savedChannels];
  const fallback: AdminRecord[] = [
    { name: "API", baseUrl: "https://api.zmoapi.com/v1", protocol: "volcengine", primary: true },
    { name: "API", baseUrl: "未配置地址", protocol: "openai" },
    { name: "API", baseUrl: "未配置地址", protocol: "openai" },
    { name: "API", baseUrl: "未配置地址", protocol: "openai" }
  ];
  return channels.length ? channels : fallback;
});
const selectedApiReferenceChannel = computed(() => apiReferenceChannels.value[selectedApiReferenceIndex.value] || apiReferenceChannels.value[0] || {});
const apiProviderIdPreview = computed(() => apiProviderDraft.value.id || apiProviderDraft.value.name.toLowerCase().replace(/[^\w]+/g, "-").replace(/^-|-$/g, "") || "provider");
const apiProviderKeyPlaceholder = computed(() => {
  const channel = selectedApiReferenceChannel.value;
  if (apiProviderDraft.value.apiKey) return "Key 写入后端安全存储，保存后不会回显完整内容";
  if (channel.apiKeyConfigured) return `保持当前 Key ${String(channel.keyPreview || "")}`.trim();
  return "输入 API Key";
});
const apiProviderKeyHint = computed(() => apiProviderDraft.value.apiKey ? "待保存 Key，保存后页面不会回显完整内容。" : apiKeyStatusLabel(selectedApiReferenceChannel.value));
const apiRecommendedPlatforms = [
  {
    name: "APIMART",
    desc: "聚合多类型生成模型，适合希望用一套配置快速接入图像、视频和 LLM 的用户。",
    tags: ["图像模型", "视频模型", "LLM模型"],
    keyUrl: "https://apimart.ai"
  },
  {
    name: "玉玉API",
    desc: "模型种类齐全，图像、视频、LLM 一站覆盖，支持每日签到送积分，适合低成本尝鲜各类型模型用户。",
    tags: ["签到送积分", "图像模型", "视频模型", "LLM模型"],
    keyUrl: "https://www.yuyapi.com"
  },
  {
    name: "Agnes AI",
    desc: "免费可用的 Agnes AI 接口，支持图像与视频生成，适合快速测试和低成本接入。",
    tags: ["免费", "图像模型", "视频模型", "LLM模型"],
    keyUrl: "https://agnes.ai"
  },
  {
    name: "FHL",
    desc: "偏向 OpenAI 兼容体验，适合需要 Codex、GPT image 2 等模型能力的配置。",
    tags: ["Codex", "GPT image 2模型"],
    keyUrl: "https://fhl.ai"
  }
];

function apiProviderPriority(channel: AdminRecord) {
  const priority = Number(channel.priority);
  return Number.isFinite(priority) && priority > 0 ? priority : 999;
}

function apiProviderSortKey(channel: AdminRecord, index: number) {
  return String(channel.id || channel.name || `provider-${index}`);
}

function apiProviderCanReorder(channel: AdminRecord) {
  return Boolean(channel.id);
}

function startApiProviderDrag(event: DragEvent, channel: AdminRecord, index: number) {
  if (!apiProviderCanReorder(channel)) {
    event.preventDefault();
    return;
  }
  const key = apiProviderSortKey(channel, index);
  apiDraggingProviderId.value = key;
  event.dataTransfer?.setData("text/plain", key);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
}

function updateApiProviderDragOver(event: DragEvent, channel: AdminRecord, index: number) {
  const targetKey = apiProviderSortKey(channel, index);
  if (!apiDraggingProviderId.value || apiDraggingProviderId.value === targetKey || !apiProviderCanReorder(channel)) return;
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
}

async function dropApiProvider(event: DragEvent, channel: AdminRecord, index: number) {
  const sourceKey = apiDraggingProviderId.value || event.dataTransfer?.getData("text/plain") || "";
  const targetKey = apiProviderSortKey(channel, index);
  resetApiProviderDrag();
  if (!sourceKey || sourceKey === targetKey || !apiProviderCanReorder(channel)) return;
  const current = [...apiReferenceChannels.value].filter(apiProviderCanReorder);
  const sourceIndex = current.findIndex((item, itemIndex) => apiProviderSortKey(item, itemIndex) === sourceKey);
  const targetIndex = current.findIndex((item, itemIndex) => apiProviderSortKey(item, itemIndex) === targetKey);
  if (sourceIndex < 0 || targetIndex < 0) return;
  const [moved] = current.splice(sourceIndex, 1);
  const adjustedTargetIndex = current.findIndex((item, itemIndex) => apiProviderSortKey(item, itemIndex) === targetKey);
  current.splice(adjustedTargetIndex, 0, moved);
  await persistApiProviderOrder(current, String(moved.id || ""));
}

function resetApiProviderDrag() {
  apiDraggingProviderId.value = "";
}

async function persistApiProviderOrder(orderedChannels: AdminRecord[], selectedId: string) {
  if (apiReorderingProviders.value) return;
  const savedChannels = orderedChannels.filter((channel) => !channel.pending && channel.id);
  const pendingChannels = orderedChannels.filter((channel) => channel.pending && channel.id);
  apiPendingProviders.value = pendingChannels;
  apiReorderingProviders.value = true;
  try {
    await Promise.all(savedChannels.map((channel, index) => {
      const priority = (index + 1) * 10;
      return adminRequest<{ item?: AdminRecord }>({
        method: "PATCH",
        url: `/admin/api/provider-channels/${channel.id}`,
        data: apiChannelMutationPayload({ ...channel, priority })
      });
    }));
    await store.loadActiveModule();
    const nextIndex = apiReferenceChannels.value.findIndex((item) => String(item.id || "") === selectedId);
    if (nextIndex >= 0) {
      selectedApiReferenceIndex.value = nextIndex;
      hydrateApiProviderDraft(apiReferenceChannels.value[nextIndex] || {});
    }
    ElMessage.success("平台顺序已保存");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "平台排序保存失败");
    await store.loadActiveModule();
  } finally {
    apiReorderingProviders.value = false;
  }
}

function selectApiReferenceChannel(index: number) {
  selectedApiReferenceIndex.value = index;
  apiRecommendMode.value = false;
  hydrateApiProviderDraft(apiReferenceChannels.value[index] || {});
  const channel = apiReferenceChannels.value[index] || {};
  ElMessage.success(`已切换到 ${channel.name || "API"} 配置`);
}

function hydrateApiProviderDraft(channel: AdminRecord) {
  const models = Array.isArray(channel.models) ? channel.models.map((model) => String(model)) : [];
  const imageModels: string[] = [];
  const chatModels: string[] = [];
  const videoModels: string[] = [];
  models.forEach((model) => {
    const category = inferApiModelCategory(model);
    if (category === "video") videoModels.push(model);
    else if (category === "image") imageModels.push(model);
    else chatModels.push(model);
  });
  apiProviderDraft.value = {
    id: String(channel.id || ""),
    name: String(channel.name || "API"),
    baseUrl: String(channel.baseUrl || "https://api.example.com/v1"),
    apiKey: "",
    protocol: String(channel.protocol || "openai"),
    imageRequestMode: String(channel.imageRequestMode || "openai"),
    imageGenerationEndpoint: String(channel.imageGenerationEndpoint || "/v1/images/generations"),
    imageEditEndpoint: String(channel.imageEditEndpoint || "/v1/images/edits"),
    videoGenerationEndpoint: String(channel.videoGenerationEndpoint || ""),
    priority: apiProviderPriority(channel),
    imageModels,
    chatModels,
    videoModels,
    loras: ["Tongyi-MAI/Z-Image-Turbo"],
    modelProtocols: {}
  };
  apiFetchedModelIds.value = [];
  apiFetchedModelSuggestions.value = { image: new Set<string>(), chat: new Set<string>(), video: new Set<string>() };
  apiModelPickerItems.value = [];
  apiVerifyResult.value = "";
  apiVerifyPanel.value = null;
}

function openApiRecommendMode() {
  apiRecommendMode.value = true;
}

function addApiProviderDraft() {
  apiRecommendMode.value = false;
  const existing = new Set([...apiPendingProviders.value, ...(apiChannels.value as AdminRecord[])].map((item) => String(item.id || "")));
  let id = "custom-api";
  let index = 2;
  while (existing.has(id)) id = `custom-api-${index++}`;
  const draft: AdminRecord = {
    id,
    name: "API",
    baseUrl: "",
    protocol: "openai",
    imageRequestMode: "openai",
    imageGenerationEndpoint: "",
    imageEditEndpoint: "",
    videoGenerationEndpoint: "",
    enabled: true,
    primary: false,
    models: [],
    status: "CONFIGURABLE",
    priority: 5,
    pending: true
  };
  apiPendingProviders.value.unshift(draft);
  selectedApiReferenceIndex.value = 0;
  hydrateApiProviderDraft(draft);
  ElMessage.success("已新增平台草稿，请在右侧填写后保存");
}

function focusRecommendedPlatform(name: string) {
  const existingIndex = apiReferenceChannels.value.findIndex((channel) => String(channel.name || "").toLowerCase().includes(name.toLowerCase()));
  if (existingIndex >= 0) selectedApiReferenceIndex.value = existingIndex;
}

function addApiDraftModel(type: "image" | "chat" | "video" | "lora") {
  const target = type === "image" ? apiProviderDraft.value.imageModels : type === "chat" ? apiProviderDraft.value.chatModels : type === "video" ? apiProviderDraft.value.videoModels : apiProviderDraft.value.loras;
  target.push(type === "lora" ? "Tongyi-MAI/Z-Image-Turbo" : "");
}

function removeApiDraftModel(type: "image" | "chat" | "video" | "lora", index: number) {
  const target = type === "image" ? apiProviderDraft.value.imageModels : type === "chat" ? apiProviderDraft.value.chatModels : type === "video" ? apiProviderDraft.value.videoModels : apiProviderDraft.value.loras;
  target.splice(index, 1);
}

const apiModelPickerCounts = computed(() => {
  const counts: Record<ApiModelPickerTab, { total: number; selected: number }> = {
    all: { total: apiModelPickerItems.value.length, selected: 0 },
    image: { total: 0, selected: 0 },
    chat: { total: 0, selected: 0 },
    video: { total: 0, selected: 0 }
  };
  apiModelPickerItems.value.forEach((item) => {
    counts[item.category].total += 1;
    if (item.selected) {
      counts.all.selected += 1;
      counts[item.category].selected += 1;
    }
  });
  return counts;
});

const filteredApiPickerModels = computed(() => {
  const keyword = apiModelPickerFilter.value.trim().toLowerCase();
  return apiModelPickerItems.value
    .filter((item) => {
      if (apiModelPickerTab.value !== "all" && item.category !== apiModelPickerTab.value) return false;
      return !keyword || item.id.toLowerCase().includes(keyword);
    })
    .sort((a, b) => a.id.localeCompare(b.id));
});

function apiModelCategoryLabel(category: ApiModelCategory) {
  return category === "image" ? "生图" : category === "video" ? "视频" : "LLM";
}

function inferApiModelCategory(model: string): ApiModelCategory {
  const value = model.toLowerCase();
  if (/veo|sora|video|seedance|hailuo|kling|wan2\.?2-v|vidu|runway/.test(value)) return "video";
  if (/image|img|gpt-image|z-image|flux|banana|sdxl|stable|dall|kolors|mj|midjourney|qwen-image/.test(value)) return "image";
  return "chat";
}

function openApiModelPicker() {
  if (!apiModelPickerItems.value.length) {
    buildApiModelPickerItems();
  }
  if (!apiModelPickerItems.value.length) {
    ElMessage.warning("没有拉取到模型");
    return;
  }
  apiModelPickerOpen.value = true;
}

function closeApiModelPicker() {
  apiModelPickerOpen.value = false;
}

function buildApiModelPickerItems(source: "fetched" | "configured" = "configured") {
  const existingImage = new Set(apiProviderDraft.value.imageModels.filter(Boolean));
  const existingChat = new Set(apiProviderDraft.value.chatModels.filter(Boolean));
  const existingVideo = new Set(apiProviderDraft.value.videoModels.filter(Boolean));
  const sourceIds = source === "fetched"
    ? apiFetchedModelIds.value
    : [...existingImage, ...existingChat, ...existingVideo];
  const allIds = Array.from(new Set(sourceIds)).filter(Boolean);
  apiModelPickerItems.value = allIds.map((id) => {
    let category: ApiModelCategory = "chat";
    if (existingImage.has(id)) category = "image";
    else if (existingVideo.has(id)) category = "video";
    else if (existingChat.has(id)) category = "chat";
    else if (apiFetchedModelSuggestions.value.image.has(id)) category = "image";
    else if (apiFetchedModelSuggestions.value.video.has(id)) category = "video";
    else if (apiFetchedModelSuggestions.value.chat.has(id)) category = "chat";
    else category = inferApiModelCategory(id);
    return { id, category, selected: existingImage.has(id) || existingChat.has(id) || existingVideo.has(id) };
  });
  apiModelPickerFilter.value = "";
  apiModelPickerTab.value = "all";
}

function arrayFromApiModels(value: unknown) {
  return Array.isArray(value) ? value.map((item) => String(item).trim()).filter(Boolean) : [];
}

function uniqueNonEmptyStrings(values: unknown[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  values.forEach((value) => {
    const item = String(value || "").trim();
    if (!item || seen.has(item)) return;
    seen.add(item);
    result.push(item);
  });
  return result;
}

async function fetchApiDraftModels() {
  if (apiFetchingDraftModels.value) return;
  apiFetchingDraftModels.value = true;
  apiVerifyResult.value = "正在从上游拉取模型列表...";
  apiVerifyPanel.value = null;
  try {
    const apiKey = apiProviderDraft.value.apiKey.trim();
    const savedItem = await ensureApiProviderDraftSaved();
    const result = await adminRequest<AdminRecord>({
      method: "POST",
      url: `/admin/api/provider-channels/${savedItem.id}/fetch-models`,
      data: apiProviderDraftTestPayload(apiKey)
    });
    const resultItem = (result.item || {}) as AdminRecord;
    const ok = Boolean(result.ok ?? resultItem.ok ?? true);
    if (!ok || String(resultItem.status || "").toUpperCase() === "ERROR") {
      throw new Error(String(resultItem.message || "拉取模型失败"));
    }
    const imageModels = arrayFromApiModels(result.imageModels || result.image_models || resultItem.imageModels || resultItem.image_models);
    const chatModels = arrayFromApiModels(result.chatModels || result.chat_models || resultItem.chatModels || resultItem.chat_models);
    const videoModels = arrayFromApiModels(result.videoModels || result.video_models || resultItem.videoModels || resultItem.video_models);
    const allModels = arrayFromApiModels(result.all || resultItem.all);
    const mergedModels = Array.from(new Set([...allModels, ...imageModels, ...chatModels, ...videoModels]));
    apiFetchedModelIds.value = mergedModels;
    apiFetchedModelSuggestions.value = {
      image: new Set(imageModels),
      chat: new Set(chatModels),
      video: new Set(videoModels)
    };
    const detectedProtocol = String(result.protocol || resultItem.protocol || "").trim();
    const detectedImageMode = String(result.imageRequestMode || result.image_request_mode || resultItem.imageRequestMode || "").trim();
    if (detectedProtocol) apiProviderDraft.value.protocol = detectedProtocol;
    if (detectedImageMode) apiProviderDraft.value.imageRequestMode = detectedImageMode;
    buildApiModelPickerItems("fetched");
    const total = Number(result.total || resultItem.modelCount || mergedModels.length);
    apiVerifyResult.value = `已拉取 ${total} 个模型 · 点「选择模型」勾选要导入的`;
    ElMessage.success(apiVerifyResult.value);
    openApiModelPicker();
  } catch (error) {
    const message = error instanceof Error ? error.message : "拉取模型失败";
    apiVerifyResult.value = message;
    ElMessage.error(message);
  } finally {
    apiFetchingDraftModels.value = false;
  }
}

async function syncAllApiDraftModels() {
  if (apiSyncingDraftModels.value || apiFetchingDraftModels.value) return;
  apiSyncingDraftModels.value = true;
  apiVerifyResult.value = "正在同步上游候选模型...";
  apiVerifyPanel.value = null;
  try {
    const apiKey = apiProviderDraft.value.apiKey.trim();
    const savedItem = await ensureApiProviderDraftSaved({ refresh: false });
    const result = await adminRequest<AdminRecord>({
      method: "POST",
      url: `/admin/api/provider-channels/${savedItem.id}/fetch-models`,
      data: { ...apiProviderDraftTestPayload(apiKey), syncModels: true }
    });
    const resultItem = (result.item || {}) as AdminRecord;
    const ok = Boolean(result.ok ?? resultItem.ok ?? true);
    if (!ok || String(resultItem.status || "").toUpperCase() === "ERROR") {
      throw new Error(String(resultItem.message || "同步候选模型失败"));
    }
    const candidateModels = arrayFromApiModels(result.candidateModels || result.candidate_models || result.all || resultItem.all);
    const addedModels = arrayFromApiModels(result.addedModels || result.added_models);
    apiFetchedModelIds.value = arrayFromApiModels(result.all || resultItem.all);
    apiProviderDraft.value.imageModels = candidateModels.filter((model) => inferApiModelCategory(model) === "image");
    apiProviderDraft.value.videoModels = candidateModels.filter((model) => inferApiModelCategory(model) === "video");
    apiProviderDraft.value.chatModels = candidateModels.filter((model) => inferApiModelCategory(model) === "chat");
    await store.loadActiveModule();
    const nextIndex = apiReferenceChannels.value.findIndex((channel) => String(channel.id || "") === String(savedItem.id || ""));
    if (nextIndex >= 0) {
      selectedApiReferenceIndex.value = nextIndex;
      hydrateApiProviderDraft(apiReferenceChannels.value[nextIndex] || {});
    }
    apiVerifyResult.value = `候选池已同步 ${candidateModels.length} 个模型 · 本次新增 ${addedModels.length} 个 · 尚未自动发布到用户端`;
    ElMessage.success(`候选模型同步完成，本次新增 ${addedModels.length} 个`);
  } catch (error) {
    const message = error instanceof Error ? error.message : "同步候选模型失败";
    apiVerifyResult.value = message;
    ElMessage.error(message);
  } finally {
    apiSyncingDraftModels.value = false;
  }
}

function selectApiDraftModels() {
  if (apiFetchedModelIds.value.length) {
    buildApiModelPickerItems("fetched");
  } else {
    buildApiModelPickerItems("configured");
  }
  openApiModelPicker();
}

function toggleApiPickerModel(id: string) {
  const item = apiModelPickerItems.value.find((model) => model.id === id);
  if (item) item.selected = !item.selected;
}

function applyApiModelPicker() {
  const image: string[] = [];
  const chat: string[] = [];
  const video: string[] = [];
  apiModelPickerItems.value.forEach((item) => {
    if (!item.selected) return;
    if (item.category === "image") image.push(item.id);
    else if (item.category === "video") video.push(item.id);
    else chat.push(item.id);
  });
  apiProviderDraft.value.imageModels = image;
  apiProviderDraft.value.chatModels = chat;
  apiProviderDraft.value.videoModels = video;
  apiVerifyResult.value = `已应用 · 生图 ${image.length} / LLM ${chat.length} / 视频 ${video.length}，点保存生效`;
  apiModelPickerOpen.value = false;
  ElMessage.success("已应用到模型列表");
}

function clearApiProviderKey() {
  apiProviderDraft.value.apiKey = "";
  apiVerifyResult.value = "已清空当前输入框中的 Key，后端已保存 Key 不会在页面回显。";
}

function normalizeApiBaseUrl(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function isValidApiBaseUrl(value: string) {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function apiProtocolDisplayLabel(protocol: string) {
  const labels: Record<string, string> = {
    openai: "OpenAI 直连",
    apimart: "APIMART 聚合协议",
    gemini: "Gemini 协议",
    volcengine: "方舟/Ark 任务协议",
    runninghub: "RunningHub OpenAPI",
    jimeng: "即梦 CLI",
    "cloudbase-function": "CloudBase 云函数"
  };
  return labels[protocol] || protocol || "OpenAI 直连";
}

function apiImageRequestModeDisplayLabel(mode: string) {
  if (mode === "cloudbase-function") return "CloudBase 云函数";
  return mode === "openai-json" ? "OpenAI JSON" : "OpenAI 标准";
}

function inferApiProviderProtocol(baseUrl: string, currentProtocol: string, currentMode: string) {
  const value = baseUrl.toLowerCase();
  const manuallyPinned = ["gemini", "volcengine", "runninghub", "jimeng", "cloudbase-function"].includes(currentProtocol);
  if (manuallyPinned) {
    return { protocol: currentProtocol, imageRequestMode: currentMode || "openai" };
  }
  if (/apimart/.test(value)) return { protocol: "apimart", imageRequestMode: "openai-json" };
  if (/volces|volcengine|ark\.cn|ark\.volc/.test(value)) return { protocol: "volcengine", imageRequestMode: "openai-json" };
  if (/runninghub/.test(value)) return { protocol: "runninghub", imageRequestMode: "openai-json" };
  if (/generativelanguage|googleapis|gemini/.test(value)) return { protocol: "gemini", imageRequestMode: "openai-json" };
  if (/jimeng|jianying|capcut/.test(value)) return { protocol: "jimeng", imageRequestMode: "openai-json" };
  if (/\.api\.tcloudbasegateway\.com/.test(value)) return { protocol: "cloudbase-function", imageRequestMode: "cloudbase-function" };
  return { protocol: currentProtocol || "openai", imageRequestMode: currentMode || "openai" };
}

function prettyApiVerifyRaw(raw: unknown) {
  try {
    return JSON.stringify(raw, null, 2);
  } catch {
    return String(raw ?? "");
  }
}

function apiVerifyStatusCode(item: AdminRecord) {
  const direct = Number(item.statusCode || item.status_code);
  if (Number.isFinite(direct) && direct > 0) return direct;
  return 200;
}

function apiVerifyModelMessage(item: AdminRecord, protocol: string) {
  const count = Number(item.modelCount || 0);
  const label = protocol === "openai" ? "OpenAI 兼容模型列表端点可用" : `${apiProtocolDisplayLabel(protocol)} 已验证`;
  if (count > 0) return `${label}，找到 ${count} 个模型`;
  return `${label}，平台配置可用`;
}

function apiAddressVerifyMessage(item: AdminRecord, imageRequestMode: string) {
  const count = Number(item.modelCount || 0);
  return `地址验证通过 · 找到 ${count} 个模型 · 图片接口：${apiImageRequestModeDisplayLabel(imageRequestMode)}`;
}

function apiVerifyErrorMessage(item: AdminRecord) {
  const message = String(item.message || "").trim();
  if (message) return message;
  return formatApiProviderTestResult(item);
}

function apiProviderDraftTestPayload(apiKey = "") {
  return {
    baseUrl: apiProviderDraft.value.baseUrl,
    apiKey,
    protocol: apiProviderDraft.value.protocol,
    imageRequestMode: apiProviderDraft.value.imageRequestMode,
    fetchModelsPath: "/models",
    probeProtocol: false
  };
}

function buildApiVerifyPanel(kind: "test" | "probe", item: AdminRecord, detected?: { protocol: string; imageRequestMode: string }) {
  const protocol = detected?.protocol || String(item.protocol || apiProviderDraft.value.protocol || "openai");
  const imageRequestMode = detected?.imageRequestMode || String(item.imageRequestMode || apiProviderDraft.value.imageRequestMode || "openai");
  const statusCode = apiVerifyStatusCode(item);
  const ok = Boolean(item.ok ?? (String(item.status || "OK").toUpperCase() !== "ERROR"));
  const raw = item.raw
    ? item.raw
    : {
      test_connection: {
        status: statusCode,
        message: formatApiProviderTestResult(item),
        raw: item
      },
      protocol,
      image_request_mode: imageRequestMode,
      ok,
      status_code: statusCode
    };
  apiVerifyPanel.value = {
    tone: ok ? "success" : "warning",
    icon: ok ? "✓" : "⚠",
    message: kind === "probe" ? apiVerifyModelMessage(item, protocol) : (ok ? apiAddressVerifyMessage(item, imageRequestMode) : apiVerifyErrorMessage(item)),
    protocolPrefix: kind === "probe" ? "协议已自动设置为" : "协议已验证为",
    protocolLabel: apiProtocolDisplayLabel(protocol),
    imageRequestModeLabel: apiImageRequestModeDisplayLabel(imageRequestMode),
    statusCode,
    raw
  };
}

function formatApiProviderTestResult(item: AdminRecord) {
  const status = String(item.status || "OK");
  const baseUrl = String(item.baseUrl || apiProviderDraft.value.baseUrl || "-");
  const latency = Number(item.latencyMs);
  const protocol = String(item.protocol || apiProviderDraft.value.protocol || "openai");
  const keyConfigured = Boolean(item.apiKeyConfigured);
  const envName = String(item.apiKeyEnv || "");
  const latencyText = Number.isFinite(latency) ? ` · ${latency}ms` : "";
  const keyText = envName ? ` · Key ${keyConfigured ? "已配置" : `未配置（${envName}）`}` : "";
  return `${status} · ${baseUrl}${latencyText} · ${apiProtocolDisplayLabel(protocol)}${keyText}`;
}

async function ensureApiProviderDraftSaved(options: { refresh?: boolean } = {}) {
  const baseUrl = normalizeApiBaseUrl(apiProviderDraft.value.baseUrl || "");
  if (!baseUrl || !isValidApiBaseUrl(baseUrl)) {
    throw new Error("请先填写正确的平台地址，例如 https://api.example.com/v1");
  }
  apiProviderDraft.value.baseUrl = baseUrl;
  const savedItem = await saveApiProviderDraft({ silent: true, refresh: options.refresh });
  if (savedItem?.id) return savedItem;
  const matched = apiReferenceChannels.value.find((channel) => {
    return String(channel.id || "") === apiProviderDraft.value.id ||
      (String(channel.name || "") === apiProviderDraft.value.name && normalizeApiBaseUrl(String(channel.baseUrl || "")) === baseUrl);
  });
  if (matched?.id) return matched;
  throw new Error("平台配置还没有保存成功，无法验证");
}

async function testApiProviderDraft() {
  if (apiTestingProviderDraft.value) return;
  apiTestingProviderDraft.value = true;
  apiVerifyResult.value = "正在验证地址...";
  apiVerifyPanel.value = null;
  try {
    const apiKey = apiProviderDraft.value.apiKey.trim();
    const savedItem = await ensureApiProviderDraftSaved({ refresh: false });
    const result = await adminRequest<{ item?: AdminRecord }>({ method: "POST", url: `/admin/api/provider-channels/${savedItem.id}/test`, data: apiProviderDraftTestPayload(apiKey) });
    apiVerifyResult.value = "";
    buildApiVerifyPanel("test", result.item || savedItem);
    ElMessage.success("验证地址已完成");
  } catch (error) {
    const message = error instanceof Error ? error.message : "验证地址失败";
    apiVerifyResult.value = message;
    apiVerifyPanel.value = null;
    ElMessage.error(message);
  } finally {
    apiTestingProviderDraft.value = false;
  }
}

async function probeApiProviderProtocol() {
  if (apiProbingProviderProtocol.value) return;
  apiProbingProviderProtocol.value = true;
  apiVerifyResult.value = "正在检测协议类型...";
  apiVerifyPanel.value = null;
  try {
    const apiKey = apiProviderDraft.value.apiKey.trim();
    const savedItem = await ensureApiProviderDraftSaved({ refresh: false });
    const result = await adminRequest<{ item?: AdminRecord }>({
      method: "POST",
      url: `/admin/api/provider-channels/${savedItem.id}/test`,
      data: { ...apiProviderDraftTestPayload(apiKey), probeProtocol: true }
    });
    const testedItem = result.item || savedItem;
    const detected = {
      protocol: String(testedItem.protocol || "openai"),
      imageRequestMode: String(testedItem.imageRequestMode || apiProviderDraft.value.imageRequestMode || "openai")
    };
    apiProviderDraft.value.protocol = detected.protocol;
    apiProviderDraft.value.imageRequestMode = detected.imageRequestMode;
    await saveApiProviderDraft({ silent: true, refresh: false });
    apiVerifyResult.value = "";
    buildApiVerifyPanel("probe", testedItem, detected);
    ElMessage.success(`已识别协议：${apiProtocolDisplayLabel(detected.protocol)}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : "验证协议失败";
    apiVerifyResult.value = message;
    apiVerifyPanel.value = null;
    ElMessage.error(message);
  } finally {
    apiProbingProviderProtocol.value = false;
  }
}

function syncSavedApiProviderLocally(savedItem: AdminRecord | undefined, draft: AdminRecord, payload: AdminRecord, apiKey: string) {
  const id = String(savedItem?.id || draft.id || "");
  const localItem: AdminRecord = {
    ...draft,
    ...payload,
    ...(savedItem || {}),
    id,
    pending: false
  };
  if (apiKey) {
    localItem.apiKeyConfigured = true;
    localItem.keyPreview = `${apiKey.slice(0, 7)}...${apiKey.slice(-4)}`;
  }
  apiPendingProviders.value = apiPendingProviders.value.filter((item) => String(item.id || "") !== id);
  const currentData = store.data as AdminRecord;
  const channels = Array.isArray(currentData.apiChannels) ? [...currentData.apiChannels] as AdminRecord[] : [];
  const index = channels.findIndex((channel) => String(channel.id || "") === id);
  if (index >= 0) channels[index] = { ...channels[index], ...localItem };
  else if (id) channels.push(localItem);
  store.data = { ...currentData, apiChannels: channels };
  apiProviderDraft.value.id = id;
  apiProviderDraft.value.apiKey = "";
}

async function saveApiProviderDraft(options: { silent?: boolean; refresh?: boolean } = {}) {
  if (apiSavingProviderDraft.value) return;
  apiSavingProviderDraft.value = true;
  const draft = {
    ...apiProviderDraft.value,
    imageModels: [...apiProviderDraft.value.imageModels],
    chatModels: [...apiProviderDraft.value.chatModels],
    videoModels: [...apiProviderDraft.value.videoModels]
  };
  const selectedModels = uniqueNonEmptyStrings([...draft.imageModels, ...draft.chatModels, ...draft.videoModels]);
  const models = selectedModels.length ? selectedModels : uniqueNonEmptyStrings([...apiFetchedModelIds.value]);
  const hasKey = Boolean(draft.apiKey.trim() || selectedApiReferenceChannel.value.apiKeyConfigured);
  const payload = apiChannelMutationPayload({
    id: draft.id,
    name: draft.name,
    baseUrl: draft.baseUrl,
    protocol: draft.protocol,
    imageRequestMode: draft.imageRequestMode,
    imageGenerationEndpoint: draft.imageGenerationEndpoint,
    imageEditEndpoint: draft.imageEditEndpoint,
    videoGenerationEndpoint: draft.videoGenerationEndpoint,
    status: hasKey ? "ACTIVE" : "CONFIGURABLE",
    priority: Number(draft.priority || 50),
    models
  });
  const pendingDraft = apiPendingProviders.value.find((item) => String(item.id) === draft.id);
  const apiKey = draft.apiKey.trim();
  try {
    let savedItem: AdminRecord | undefined;
    if (draft.id) {
      if (pendingDraft) {
        const result = await adminRequest<{ item?: AdminRecord }>({ method: "POST", url: "/admin/api/provider-channels", data: payload });
        savedItem = result.item;
        apiPendingProviders.value = apiPendingProviders.value.filter((item) => String(item.id) !== draft.id);
      } else {
        const result = await adminRequest<{ item?: AdminRecord }>({ method: "PATCH", url: `/admin/api/provider-channels/${draft.id}`, data: payload });
        savedItem = result.item;
      }
    } else {
      const result = await adminRequest<{ item?: AdminRecord }>({ method: "POST", url: "/admin/api/provider-channels", data: payload });
      savedItem = result.item;
    }
    if (apiKey) {
      const keyCustomer = String(savedItem?.id || draft.id || draft.name);
      await adminRequest({
        method: "POST",
        url: "/admin/api/keys",
        data: { customer: keyCustomer, status: "ACTIVE", quotaLimit: 100000, models, secret: apiKey, apiKey }
      });
    }
    const shouldRefresh = options.refresh !== false;
    if (shouldRefresh) await store.loadActiveModule();
    else syncSavedApiProviderLocally(savedItem, draft, payload, apiKey);
    const savedId = String(savedItem?.id || "");
    const savedName = String(savedItem?.name || draft.name || "");
    const savedBaseUrl = String(savedItem?.baseUrl || payload.baseUrl || "");
    const nextIndex = apiReferenceChannels.value.findIndex((channel) => {
      const channelId = String(channel.id || "");
      const channelName = String(channel.name || "");
      const channelBaseUrl = String(channel.baseUrl || "");
      return (savedId && channelId === savedId) || (channelName === savedName && channelBaseUrl === savedBaseUrl);
    });
    if (nextIndex >= 0) {
      selectedApiReferenceIndex.value = nextIndex;
      if (shouldRefresh) hydrateApiProviderDraft(apiReferenceChannels.value[nextIndex] || {});
    }
    apiProviderDraft.value.apiKey = "";
    if (!options.silent) {
      apiVerifyResult.value = "已保存平台配置。";
      ElMessage.success("平台配置已保存");
    }
    return savedItem;
  } catch (error) {
    if (pendingDraft && !apiPendingProviders.value.some((item) => String(item.id) === draft.id)) apiPendingProviders.value.unshift(pendingDraft);
    const message = error instanceof Error ? error.message : "保存平台配置失败";
    apiVerifyResult.value = message;
    ElMessage.error(message);
  } finally {
    apiSavingProviderDraft.value = false;
  }
}

async function deleteSelectedApiProvider() {
  apiVerifyResult.value = "当前后端未开放删除上游接口，已保留配置。";
  ElMessage.warning("暂无删除接口");
}

watch(apiReferenceChannels, (channels) => {
  if (!channels.length) return;
  if (selectedApiReferenceIndex.value >= channels.length) selectedApiReferenceIndex.value = 0;
  hydrateApiProviderDraft(channels[selectedApiReferenceIndex.value] || channels[0] || {});
}, { immediate: true });

function apiRecommendedChannelPayload(platform: (typeof apiRecommendedPlatforms)[number]) {
  const matched = recommendedApiChannels.find((channel) => String(channel.name || "").toLowerCase().includes(platform.name.toLowerCase()));
  if (matched) return apiChannelMutationPayload(matched);
  const baseUrls: Record<string, string> = {
    "玉玉API": "https://api.yuyapi.com/v1",
    "Agnes AI": "https://api.agnes.ai/v1",
    FHL: "https://api.fhl.ai/v1"
  };
  return apiChannelMutationPayload({
    name: platform.name,
    baseUrl: baseUrls[platform.name] || platform.keyUrl,
    protocol: "openai",
    imageRequestMode: "openai",
    imageGenerationEndpoint: "/v1/images/generations",
    imageEditEndpoint: "/v1/images/edits",
    videoGenerationEndpoint: "contents/generations/tasks",
    fetchModelsPath: "/models",
    apiKeyEnv: `${platform.name.replace(/\s+/g, "_").replace(/[^\w]/g, "").toUpperCase()}_API_KEY`,
    status: "CONFIGURABLE",
    priority: 20,
    models: platform.tags.includes("GPT image 2模型") ? ["gpt-image-2"] : ["gpt-image-2", "mock-standard"],
    notes: platform.desc
  });
}

async function saveRecommendedPlatform(platform: (typeof apiRecommendedPlatforms)[number]) {
  const key = String(apiQuickKeys.value[platform.name] || "").trim();
  if (!key) {
    focusRecommendedPlatform(platform.name);
    ElMessage.warning(`请先粘贴 ${platform.name} API Key`);
    return;
  }
  const exists = apiChannels.value.some((channel) => String((channel as AdminRecord).name || "").toLowerCase().includes(platform.name.toLowerCase()));
  if (!exists) await store.mutate("POST", "/admin/api/provider-channels", apiRecommendedChannelPayload(platform));
  await store.mutate("POST", "/admin/api/keys", {
    customer: platform.name,
    status: "ACTIVE",
    quotaLimit: 100000,
    models: platform.tags.includes("GPT image 2模型") ? ["gpt-image-2"] : ["mock-standard", "gpt-image-2"],
    secret: key,
    apiKey: key
  });
  apiQuickKeys.value[platform.name] = "";
  const nextIndex = apiReferenceChannels.value.findIndex((channel) => String(channel.name || "").toLowerCase().includes(platform.name.toLowerCase()));
  if (nextIndex >= 0) selectedApiReferenceIndex.value = nextIndex;
  ElMessage.success(`${platform.name} 已保存`);
}

function apiProtocolLabel(channel: AdminRecord) {
  const protocol = String(channel.protocol || "openai").toLowerCase();
  const labels: Record<string, string> = {
    openai: "OpenAI 兼容",
    apimart: "APIMart 异步",
    modelscope: "ModelScope",
    comfyui: "ComfyUI",
    runninghub: "RunningHub",
    gemini: "Gemini",
    volcengine: "火山方舟",
    "cloudbase-function": "CloudBase 云函数"
  };
  return labels[protocol] || protocol.toUpperCase();
}

function apiEndpointSummary(channel: AdminRecord) {
  const generation = String(channel.imageGenerationEndpoint || "/v1/images/generations");
  const edit = String(channel.imageEditEndpoint || "");
  return edit ? `${generation} / ${edit}` : generation;
}

function openApiKeyUrl(url: string) {
  if (typeof window === "undefined") return;
  window.open(url, "_blank", "noopener,noreferrer");
}

function apiKeyStatusLabel(channel: AdminRecord) {
  if (channel.apiKeyConfigured) return `Key 已保存：后端安全存储 ${String(channel.keyPreview || "")}`.trim();
  if (channel.apiKeyEnv) return "等待环境变量";
  return "还没有保存 Key";
}

function apiComfyLabel(channel: AdminRecord) {
  const instances = Array.isArray(channel.comfyInstances) ? channel.comfyInstances : [];
  if (instances.length) return `ComfyUI ${instances.length} 节点`;
  return "无 ComfyUI 节点";
}

function apiChannelModelsPreview(channel: AdminRecord) {
  const models = Array.isArray(channel.models) ? channel.models : [];
  if (!models.length) return "未映射模型";
  return `${models.length} 个模型`;
}

function apiKeyMasked(channel: AdminRecord) {
  if (channel.apiKeyConfigured) return String(channel.keyPreview || "sk-••••••••••••••••••••••••");
  if (channel.apiKeyEnv) return `${channel.apiKeyEnv}=未配置`;
  return "No API key required";
}
const partnerModuleIds = ["partnerDashboard", "partnerCustomers", "partnerOrders", "partnerUsage", "partnerCommissions", "partnerChannels", "partnerMaterials", "partnerAccount"];
const isPartnerModule = computed(() => partnerModuleIds.includes(store.activeModuleId));
const isOperationCenterModule = computed(() => operationCenterModuleIds.includes(store.activeModuleId));
const partnerData = computed(() => store.data as AdminRecord & {
  user?: AdminRecord;
  agent?: AdminRecord;
  promotion?: AdminRecord;
  summary?: Record<string, unknown>;
  customers?: AdminRecord[];
  orders?: AdminRecord[];
  usageEvents?: AdminRecord[];
  commissions?: AdminRecord[];
  withdrawals?: AdminRecord[];
  children?: AdminRecord[];
});

const partnerAgentRecord = computed<AdminRecord>(() => {
  const agent = partnerData.value.agent;
  if (agent && typeof agent === "object" && Object.keys(agent).length) return agent;
  return currentAgent.value || {};
});

const partnerPromotionRecord = computed<AdminRecord>(() => {
  const promotion = partnerData.value.promotion;
  if (promotion && typeof promotion === "object" && Object.keys(promotion).length) return promotion;
  return {};
});

function moneyYuan(value: unknown) {
  return `￥${(Number(value || 0) / 100).toFixed(2)}`;
}

function partnerSummaryValue(key: string) {
  return partnerData.value.summary?.[key] ?? 0;
}

function buildInviteLink(inviteCode: unknown) {
  const code = String(inviteCode || "").trim();
  if (!code) return "";
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  return `${origin || "http://localhost:3100"}/register?invite=${encodeURIComponent(code)}`;
}

function partnerInviteLink() {
  const promotion = partnerPromotionRecord.value;
  const agent = partnerAgentRecord.value;
  const inviteLink = String(promotion.inviteLink || agent.inviteLink || "").trim();
  if (inviteLink) return inviteLink;
  return buildInviteLink(promotion.inviteCode || agent.inviteCode);
}

function partnerInviteCode() {
  const promotion = partnerPromotionRecord.value;
  const agent = partnerAgentRecord.value;
  return String(promotion.inviteCode || agent.inviteCode || "").trim();
}

const agentLevelOptions = [
  { value: "1", label: "L1 推广员" },
  { value: "2", label: "L2 初级代理商" },
  { value: "3", label: "L3 高级代理商" },
  { value: "4", label: "L4 城市合伙人" },
  { value: "5", label: "L5 联合运营商" }
];

const agentLevelLabelMap: Record<number, string> = {
  0: "L0 普通用户",
  1: "L1 推广员",
  2: "L2 初级代理商",
  3: "L3 高级代理商",
  4: "L4 城市合伙人",
  5: "L5 联合运营商"
};

function partnerAgentLevelLabel(levelValue?: unknown) {
  const agent = partnerAgentRecord.value;
  const level = Number(levelValue ?? agent.level ?? 0);
  if (Number.isFinite(level) && level in agentLevelLabelMap) return agentLevelLabelMap[level];
  if (level > 0) return `L${level} 代理`;
  return "-";
}

function openPartnerInviteLink() {
  const link = partnerInviteLink();
  if (!link) {
    ElMessage.warning("暂无可打开的推广链接");
    return;
  }
  window.open(link, "_blank", "noopener,noreferrer");
}

const partnerDashboardMetrics = computed(() => {
  const directCustomers = Number(partnerSummaryValue("directCustomers"));
  const childAgents = Number(partnerSummaryValue("childAgents"));
  const totalCommission = Number(partnerSummaryValue("totalCommission"));
  const availableToWithdraw = Number(partnerSummaryValue("availableToWithdraw"));
  const rawAvailableToWithdraw = Number(partnerSummaryValue("rawAvailableToWithdraw"));
  const pendingCommission = Number(partnerSummaryValue("pendingCommission"));
  const commissions = Array.isArray(partnerData.value.commissions) ? partnerData.value.commissions : [];
  return [
    { label: "今日新增客户", value: String(Math.max(1, Math.round(directCustomers / 3))), hint: "代理获客口径" },
    { label: "有效客户", value: String(directCustomers), hint: "已绑定客户" },
    { label: "待支付订单", value: String(commissions.filter((item) => String(item.status || "").toUpperCase() === "PENDING").length), hint: "需要跟进" },
    { label: "累计佣金", value: moneyYuan(totalCommission), hint: "历史分佣" },
    { label: "可提现金额", value: moneyYuan(Math.max(0, availableToWithdraw)), hint: rawAvailableToWithdraw < 0 ? "历史提现已超出已结算佣金" : "可申请提现" },
    { label: "推广转化率", value: `${Math.min(68, 18 + directCustomers * 6 + childAgents * 3)}%`, hint: "注册到成交" }
  ];
});

const partnerTrend = computed(() => {
  const base = Math.max(18, Number(partnerSummaryValue("directCustomers")) * 12);
  return [
    { day: "周一", height: Math.min(92, base + 10) },
    { day: "周二", height: Math.min(92, base + 32) },
    { day: "周三", height: Math.min(92, base + 18) },
    { day: "周四", height: Math.min(92, base + 26) },
    { day: "周五", height: Math.min(92, base + 42) },
    { day: "周六", height: Math.min(92, base + 8) },
    { day: "周日", height: Math.min(92, base + 16) }
  ];
});

const partnerTodos = [
  { module: "partnerCustomers", title: "审核客户", desc: "确认新客户状态和来源" },
  { module: "partnerOrders", title: "跟进待支付订单", desc: "提醒客户完成付款" },
  { module: "partnerCommissions", title: "申请提现", desc: "核对可提现金额" },
  { module: "partnerMaterials", title: "更新推广素材", desc: "同步邀请码和海报" }
];

const partnerSourceRows = computed(() => {
  const agent = partnerAgentRecord.value;
  const customers = Array.isArray(partnerData.value.customers) ? partnerData.value.customers : [];
  const children = Array.isArray(partnerData.value.children) ? partnerData.value.children : [];
  const totalCommission = Number(partnerSummaryValue("totalCommission"));
  return [
    { channel: `邀请码 ${agent.inviteCode || "-"}`, visits: customers.length * 18 + 96, customers: customers.length, orders: Math.max(0, (partnerData.value.commissions || []).length), commission: moneyYuan(totalCommission), status: "ACTIVE" },
    { channel: "朋友圈海报", visits: customers.length * 12 + 58, customers: Math.max(0, customers.length - 1), orders: Math.max(0, Math.round((partnerData.value.commissions || []).length / 2)), commission: moneyYuan(totalCommission * 0.36), status: "ACTIVE" },
    { channel: "下级代理网络", visits: children.length * 40 + 22, customers: children.length, orders: children.length, commission: moneyYuan(totalCommission * 0.24), status: children.length ? "ACTIVE" : "PENDING" }
  ];
});

function partnerRows(): AdminRecord[] {
  const data = partnerData.value;
  const agent = partnerAgentRecord.value;
  const items = Array.isArray(data.items) ? data.items : [];
  const customers = Array.isArray(data.customers) ? data.customers : store.activeModuleId === "partnerCustomers" ? items : [];
  const orders = Array.isArray(data.orders) ? data.orders : store.activeModuleId === "partnerOrders" ? items : [];
  const usageEvents = partnerUsageEvents(Array.isArray(data.usageEvents) ? data.usageEvents : store.activeModuleId === "partnerUsage" ? items : []);
  const commissions = Array.isArray(data.commissions) ? data.commissions : store.activeModuleId === "partnerCommissions" ? items : [];
  const withdrawals = Array.isArray(data.withdrawals) ? data.withdrawals : [];
  const children = Array.isArray(data.children) ? data.children : [];
  if (store.activeModuleId === "partnerCustomers") return customers.map((customer) => {
    const customerId = String(customer.id || "");
    const customerUsage = usageEvents.filter((item) => String(item.userId || item.customerId || "") === customerId);
    const customerOrders = orders.filter((item) => String(item.userId || item.customerId || "") === customerId);
    const customerCommissions = commissions.filter((item) => customerUsage.some((event) => String(event.taskId || "") === String(item.orderId || "")));
    const latestUsage = customerUsage[0] || {};
    const totalPointCost = customerUsage.reduce((total, item) => total + Number(item.pointCost || 0), 0);
    const totalConsumedCents = customerUsage.reduce((total, item) => total + Number(item.amountCents || 0), 0);
    const totalCommissionCents = customerCommissions.reduce((total, item) => total + Number(item.amountCents || 0), 0);
    return {
      ...customer,
      customerId,
      usageCount: customerUsage.length,
      pointCost: totalPointCost,
      consumedCents: totalConsumedCents,
      commissionCents: totalCommissionCents,
      orderCount: customerOrders.length,
      latestModel: latestUsage.model || "-",
      latestTaskId: latestUsage.taskId || "-",
      latestUsageAt: latestUsage.occurredAt || "-",
      customerValue: totalConsumedCents >= 100 ? "高价值" : customerUsage.length ? "已消费" : "待激活",
      averagePointCost: customerUsage.length ? Math.round(totalPointCost / customerUsage.length) : 0
    };
  });
  if (store.activeModuleId === "partnerOrders") {
    const usageOrderIds = new Set(usageEvents.map((item) => String(item.taskId || "")).filter(Boolean));
    const usageOrders = usageEvents.map((item) => {
      const metadata = (item.metadata && typeof item.metadata === "object" ? item.metadata : {}) as Record<string, unknown>;
      const customerId = String(item.userId || item.customerId || "");
      const commission = commissions.find((commissionItem) => String(commissionItem.orderId || "") === String(item.taskId || ""));
      return {
        id: item.taskId || item.id,
        orderId: item.taskId || item.id,
        orderType: "生图消费",
        customerId,
        customer: customers.find((customer) => String(customer.id || "") === customerId)?.name || metadata.customer || customerId || "-",
        plan: customers.find((customer) => String(customer.id || "") === customerId)?.plan || "-",
        model: item.model || "-",
        pointCost: item.pointCost || 0,
        amountCents: item.amountCents || 0,
        commissionCents: commission?.amountCents || 0,
        commissionRate: commission?.rate ?? "-",
        status: item.status || commission?.status || "PENDING",
        createdAt: item.occurredAt || commission?.createdAt || "-",
        metricCode: item.metricCode,
        balanceBefore: item.balanceBefore,
        balanceAfter: item.balanceAfter,
        prompt: metadata.prompt || "-"
      };
    });
    const paymentOrders = orders.map((item) => ({
      ...item,
      orderType: "套餐订单",
      orderId: item.id || item.orderId,
      commissionCents: commissions.find((commissionItem) => String(commissionItem.orderId || "") === String(item.id || item.orderId || ""))?.amountCents || 0
    }));
    const manualOrders = commissions
      .filter((item) => !usageOrderIds.has(String(item.orderId || "")) && !orders.some((order) => String(order.id || order.orderId || "") === String(item.orderId || "")))
      .map((item) => ({
        id: item.orderId || item.id,
        orderId: item.orderId || item.id,
        orderType: "手工分润",
        customer: "-",
        plan: "-",
        model: "-",
        pointCost: "-",
        amountCents: item.ruleSnapshot && typeof item.ruleSnapshot === "object" && "amountCents" in item.ruleSnapshot ? (item.ruleSnapshot as Record<string, unknown>).amountCents : item.amountCents,
        commissionCents: item.amountCents,
        commissionRate: item.rate ?? "-",
        status: item.status || "PENDING",
        createdAt: item.createdAt || "-"
      }));
    return [...usageOrders, ...paymentOrders, ...manualOrders];
  }
  if (store.activeModuleId === "partnerUsage") return usageEvents.map((item) => {
    const metadata = (item.metadata && typeof item.metadata === "object" ? item.metadata : {}) as Record<string, unknown>;
    const commission = commissions.find((commissionItem) => String(commissionItem.orderId || "") === String(item.taskId || ""));
    return {
      ...item,
      id: item.id || item.taskId,
      customerId: item.userId || item.customerId,
      customer: customers.find((customer) => String(customer.id || "") === String(item.userId || item.customerId || ""))?.name || metadata.customer || item.userId || item.customerId || "-",
      consumedCents: item.amountCents,
      commissionCents: commission?.amountCents || 0,
      commissionRate: commission?.rate ?? "-",
      prompt: metadata.prompt || "-",
      createdAt: item.occurredAt
    };
  });
  if (store.activeModuleId === "partnerCommissions") return [
    ...commissions.map((item) => {
      const usage = usageEvents.find((usageItem) => String(usageItem.taskId || "") === String(item.orderId || ""));
      const metadata = (usage?.metadata && typeof usage.metadata === "object" ? usage.metadata : {}) as Record<string, unknown>;
      const ruleSnapshot = (item.ruleSnapshot && typeof item.ruleSnapshot === "object" ? item.ruleSnapshot : {}) as Record<string, unknown>;
      const customerId = String(usage?.userId || usage?.customerId || "");
      return {
        ...item,
        recordType: "佣金",
        settlementSource: ruleSnapshot.source === "image_generation" ? "生图分佣" : "手工分润",
        customerId,
        customer: customers.find((customer) => String(customer.id || "") === customerId)?.name || metadata.customer || "-",
        model: usage?.model || "-",
        pointCost: usage?.pointCost || ruleSnapshot.pointCost || "-",
        relatedAmountCents: ruleSnapshot.amountCents || usage?.amountCents || item.amountCents,
        commissionCents: item.amountCents,
        commissionRate: item.rate,
        reviewedAt: "-"
      };
    }),
    ...withdrawals.map((item) => ({
      ...item,
      recordType: "提现",
      settlementSource: "提现申请",
      orderId: "-",
      customer: "-",
      model: "-",
      pointCost: "-",
      relatedAmountCents: "-",
      commissionCents: "-",
      commissionRate: "-",
      rate: "-"
    }))
  ];
  if (store.activeModuleId === "partnerChannels") return [{ ...agent, levelLabel: partnerAgentLevelLabel(agent.level), channel: "直属推广", customers: customers.length, children: children.length }, ...children.map((child) => ({ ...child, levelLabel: partnerAgentLevelLabel(child.level) }))];
  if (store.activeModuleId === "partnerMaterials") return [
    { id: "invite-link", name: "专属邀请链接", type: "链接", value: partnerInviteLink(), status: "ACTIVE" },
    { id: "poster", name: "朋友圈推广海报", type: "海报", value: "可复制邀请码与二维码", status: "ACTIVE" },
    { id: "script", name: "客户转化话术", type: "话术", value: "适合私域跟进和社群转化", status: "ACTIVE" }
  ];
  if (store.activeModuleId === "partnerAccount") return [
    { id: "profile", item: "代理商账号", value: `${data.user?.name || "-"} / ${data.user?.email || "-"}`, status: data.user?.status || "-" },
    { id: "agentLevel", item: "代理商等级", value: partnerAgentLevelLabel(agent.level), status: agent.status || "-" },
    { id: "inviteCode", item: "邀请码", value: agent.inviteCode || "-", status: agent.status || "-" },
    { id: "withdraw", item: "可提现金额", value: moneyYuan(Math.max(0, Number(partnerSummaryValue("availableToWithdraw")))), status: "ACTIVE" }
  ];
  return partnerSourceRows.value;
}

const operationCenterData = computed(() => store.data as AdminRecord & {
  user?: AdminRecord;
  operationCenter?: AdminRecord | null;
  summary?: Record<string, unknown>;
  joinPlan?: AdminRecord;
  items?: AdminRecord[];
});

const operationCenterRecord = computed<AdminRecord>(() => {
  const center = operationCenterData.value.operationCenter;
  if (center && typeof center === "object" && Object.keys(center).length) return center;
  return currentOperationCenter.value || {};
});

function operationCenterSummaryValue(key: string) {
  return operationCenterData.value.summary?.[key] ?? 0;
}

function operationCenterInviteCode() {
  return String(operationCenterRecord.value.inviteCode || "").trim();
}

const operationCenterDashboardMetrics = computed(() => {
  const agents = Number(operationCenterSummaryValue("agents"));
  const orders = Number(operationCenterSummaryValue("orders"));
  const paidOrderAmount = Number(operationCenterSummaryValue("paidOrderAmountCents"));
  const totalCents = Number(operationCenterSummaryValue("totalCents"));
  const pendingCents = Number(operationCenterSummaryValue("pendingCents"));
  const settledCents = Number(operationCenterSummaryValue("settledCents"));
  return [
    { label: "中心代理", value: String(agents), hint: "归属代理商" },
    { label: "成交订单", value: String(orders), hint: "已支付订单" },
    { label: "归属成交额", value: moneyYuan(paidOrderAmount), hint: "订单金额口径" },
    { label: "累计分润", value: moneyYuan(totalCents), hint: "中心奖励" },
    { label: "待结算", value: moneyYuan(pendingCents), hint: "待审核/结算" },
    { label: "已结算", value: moneyYuan(settledCents), hint: "可归档收入" }
  ];
});

const operationCenterTrend = computed(() => {
  const base = Math.max(18, Number(operationCenterSummaryValue("orders")) * 10 + Number(operationCenterSummaryValue("agents")) * 8);
  return [
    { day: "周一", height: Math.min(92, base + 6) },
    { day: "周二", height: Math.min(92, base + 18) },
    { day: "周三", height: Math.min(92, base + 12) },
    { day: "周四", height: Math.min(92, base + 28) },
    { day: "周五", height: Math.min(92, base + 36) },
    { day: "周六", height: Math.min(92, base + 10) },
    { day: "周日", height: Math.min(92, base + 20) }
  ];
});

const operationCenterTodos = [
  { module: "operationCenterAgents", title: "查看中心代理", desc: "核对等级、邀请码和归属关系" },
  { module: "operationCenterOrders", title: "跟进归属订单", desc: "确认中心名下成交与待支付订单" },
  { module: "operationCenterCommissions", title: "核对分润", desc: "检查待结算与已结算金额" },
  { module: "userMembership", title: "管理身份套餐", desc: "查看身份、充值和订阅入口" }
];

const operationCenterTabs = computed(() => [
  { module: "operationCenterAgents", title: "代理商", icon: Connection, value: String(operationCenterSummaryValue("agents")) },
  { module: "operationCenterOrders", title: "订单", icon: Money, value: String(operationCenterSummaryValue("orders")) },
  { module: "operationCenterCommissions", title: "分润", icon: Wallet, value: moneyYuan(operationCenterSummaryValue("totalCents")) }
]);

const operationCenterKpis = computed(() => [
  { label: "运营中心状态", value: statusLabel(operationCenterRecord.value.status), desc: String(operationCenterRecord.value.region || "区域未配置") },
  { label: "开通订单", value: String(operationCenterRecord.value.joinOrderId || "-"), desc: operationCenterRecord.value.joinFeeCents ? moneyYuan(operationCenterRecord.value.joinFeeCents) : "暂无开通费记录" },
  { label: "分润记录", value: String(operationCenterSummaryValue("records")), desc: "运营中心奖励条数" },
  { label: "邀请代码", value: operationCenterInviteCode() || "-", desc: "用于中心归属识别" }
]);

function operationCenterRows(): AdminRecord[] {
  const data = operationCenterData.value;
  const items = Array.isArray(data.items) ? data.items : [];
  if (store.activeModuleId === "operationCenterAgents") {
    return items.map((item) => ({ ...item, levelLabel: item.levelLabel || partnerAgentLevelLabel(item.level), recordType: "代理商" }));
  }
  if (store.activeModuleId === "operationCenterOrders") {
    return items.map((item) => ({
      ...item,
      orderId: item.orderId || item.orderNo || item.id,
      orderTypeText: membershipOrderType(item.orderType),
      plan: item.plan || item.planId || "-",
      customer: item.customer || item.userId || "-"
    }));
  }
  if (store.activeModuleId === "operationCenterCommissions") {
    return items.map((item) => ({
      ...item,
      recordType: "运营中心奖励",
      settlementSource: item.commissionType || "OPERATION_CENTER_REWARD",
      commissionCents: item.amountCents,
      reviewedAt: item.updatedAt || "-"
    }));
  }
  return [];
}

const activeModuleMeta = computed(() => pageMeta[store.activeModuleId] || { badge: "主控模块", description: "管理当前业务域的数据和动作。" });
const activeAdminNavigation = computed(() => {
  if (isUserConsole.value || isAgentConsole.value) return null;
  return adminNavigationSectionForModule(store.activeModuleId);
});
const activeAdminSectionTabs = computed(() => {
  const navigation = activeAdminNavigation.value;
  if (!navigation) return [];
  const { section } = navigation;
  if (section.tabModuleIds && !section.tabModuleIds.includes(store.activeModuleId)) return [];
  return (section.tabModuleIds || section.moduleIds)
    .map((moduleId) => adminModuleById(moduleId))
    .filter((module): module is NonNullable<typeof module> => Boolean(module) && canNavigateToModule(module!.id));
});
const activeSidebarModuleId = computed(() => activeAdminNavigation.value?.section.primaryModuleId || store.activeModuleId);
const visibleModuleGroups = computed(() => {
  if (isUserConsole.value) return userModuleGroups;
  if (isAgentConsole.value) return agentModuleGroups;
  return adminModuleGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => !item.requiresEnterpriseManagement || canViewEnterpriseManagement.value)
    }))
    .filter((group) => group.items.length > 0);
});
const activeGroup = computed(() => {
  const navigationGroupId = activeAdminNavigation.value?.group.id;
  return visibleModuleGroups.value.find((group) => navigationGroupId ? group.id === navigationGroupId : group.items.some((item) => item.id === store.activeModuleId));
});
const activeUserMenuEntry = computed(() => {
  if (!isUserConsole.value) return null;
  return allUserFlatMenuDefs.find((item) => item.targetId === store.activeModuleId) || null;
});
const activeGroupLabel = computed(() => isUserConsole.value ? "用户端" : activeGroup.value?.title || "工作台");
const activeHeaderModuleTitle = computed(() => activeUserMenuEntry.value?.title || store.activeModule.title);
const activeGroupIcon = computed(() => isUserConsole.value ? activeUserMenuEntry.value?.icon || House : activeGroup.value?.icon || House);
const isGroupActive = (group: { id: string; items: Array<{ id: string }> }) => activeAdminNavigation.value ? group.id === activeAdminNavigation.value.group.id : group.items.some((item) => item.id === store.activeModuleId);
const brandHomeTitle = computed(() => isUserConsole.value ? "回到用户首页" : isAgentConsole.value ? "回到代理看板" : "回到主控工作台");
const activeUserMenuId = computed(() => activeUserMenuEntry.value?.id || store.activeModuleId);

function toggleDesktopSidebar() {
  desktopSidebarCollapsed.value = !desktopSidebarCollapsed.value;
}

async function goBrandHome() {
  const moduleId = isUserConsole.value ? "userDashboard" : isAgentConsole.value ? "partnerDashboard" : "analysis";
  await selectAdminModule(moduleId);
}

function scrollOpenTabs(direction: -1 | 1) {
  tabsScrollRef.value?.scrollBy({ left: direction * 260, behavior: "smooth" });
}

function ensureOpenTab(moduleId: string) {
  if (!allowedModuleIds.includes(moduleId)) return;
  if (agentModuleIds.includes(moduleId) && !hasAgentIdentity.value) return;
  if (operationCenterModuleIds.includes(moduleId) && !hasOperationCenterIdentity.value) return;
  const module = modules.find((item) => item.id === moduleId);
  if (!module || openTabs.value.some((item) => item.id === moduleId)) return;
  openTabs.value.push(module);
}

function protectedActionForModule(moduleId: string): ProtectedAction {
  if (moduleId === "userOrders") return "open_order";
  if (moduleId === "userUsage") return "open_wallet";
  if (moduleId === "userMembership") return "open_member_center";
  if (moduleId === "userWorks") return "save_work";
  if (moduleId.toLowerCase().includes("knowledge")) return "create_knowledge_base";
  if (moduleId.toLowerCase().includes("agent")) return "create_agent";
  return "open_member_center";
}

async function selectAdminModule(moduleId: string) {
  if (!allowedModuleIds.includes(moduleId)) return;
  if (isGuestUser.value && !guestVisibleModuleIds.has(moduleId)) {
    saveWorkspaceGuestDraft(moduleId);
    const loginAlreadyOpen = workspaceLoginOpen.value;
    authStore.requireAuth({
      action: protectedActionForModule(moduleId),
      route: currentWorkspaceRoute(),
      payload: workspaceDraftPayload(moduleId),
      autoResume: true
    });
    if (!loginAlreadyOpen) trackWebGuestExperience("login_modal_show", moduleId, { action: protectedActionForModule(moduleId) });
    return;
  }
  if (!canNavigateToModule(moduleId)) {
    ElMessage.warning("当前角色没有访问该模块的权限");
    return;
  }
  if (agentModuleIds.includes(moduleId) && !hasAgentIdentity.value) {
    ElMessage.warning("当前账号还没有代理身份");
    return;
  }
  if (operationCenterModuleIds.includes(moduleId) && !hasOperationCenterIdentity.value) {
    ElMessage.warning("当前账号还没有运营中心身份");
    return;
  }
  if (moduleId === "userAgentCenter" && (officeCLIWorkspaceOpen.value || agentCenterWorkspace.value)) {
    closeAgentCenterSubview();
    return;
  }
  if (moduleId !== "userAgentCenter") {
    officeCLIWorkspaceOpen.value = false;
    agentCenterWorkspace.value = null;
  }
  mobileDrawerOpen.value = false;
  ensureOpenTab(moduleId);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(activeTabStorageKey, moduleId);
    syncUserModulePath(moduleId);
    syncAdminModulePath(moduleId);
  }
  const previousModuleId = store.activeModuleId;
  if (isGuestUser.value) {
    store.activeModuleId = moduleId;
    store.data = {};
    store.error = "";
    if (["userAiImage", "userVideoGeneration", "userPptGeneration", "userAgentCenter", "userWirelessCanvas"].includes(moduleId)) {
      trackWebGuestExperience("guest_open_creator", moduleId, { module: moduleId });
    }
  } else {
    await store.selectModule(moduleId);
  }
  trackAdminExperience("MODULE_VIEW", moduleId, "", { fromModuleId: previousModuleId });
}

async function selectUserFlatMenu(menuId: string) {
  const target = allUserFlatMenuDefs.find((item) => item.id === menuId);
  if (!target) return;
  if (agentModuleIds.includes(target.targetId) && !hasAgentIdentity.value) {
    ElMessage.warning("当前账号还没有代理身份");
    return;
  }
  if (operationCenterModuleIds.includes(target.targetId) && !hasOperationCenterIdentity.value) {
    ElMessage.warning("当前账号还没有运营中心身份");
    return;
  }
  if (target.targetId === "userAiImage") {
    aiPlaygroundMode.value = "gallery";
  }
  await selectAdminModule(target.targetId);
}

function isDefaultTab(moduleId: string) {
  return defaultOpenTabIds.includes(moduleId);
}

async function activateTabAfterPrune(preferredId = store.activeModuleId) {
  if (!openTabs.value.length) {
    openTabs.value = modules.filter((item) => defaultOpenTabIds.includes(item.id));
  }
  const next = openTabs.value.find((item) => item.id === preferredId) || openTabs.value[0];
  if (!next) return;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(activeTabStorageKey, next.id);
  }
  await store.selectModule(next.id);
}

async function closeOtherTabs() {
  openTabs.value = openTabs.value.filter((item) => item.id === store.activeModuleId || isDefaultTab(item.id));
  await activateTabAfterPrune(store.activeModuleId);
}

async function closeLeftTabs() {
  const activeIndex = openTabs.value.findIndex((item) => item.id === store.activeModuleId);
  if (activeIndex < 0) return;
  openTabs.value = openTabs.value.filter((item, index) => index >= activeIndex || isDefaultTab(item.id));
  await activateTabAfterPrune(store.activeModuleId);
}

async function closeRightTabs() {
  const activeIndex = openTabs.value.findIndex((item) => item.id === store.activeModuleId);
  if (activeIndex < 0) return;
  openTabs.value = openTabs.value.filter((item, index) => index <= activeIndex || isDefaultTab(item.id));
  await activateTabAfterPrune(store.activeModuleId);
}

async function closeAllTabs() {
  openTabs.value = modules.filter((item) => defaultOpenTabIds.includes(item.id));
  await activateTabAfterPrune(defaultOpenTabIds[0]);
}

async function handleTabsCommand(command: string | number | object) {
  if (command === "refresh") {
    await store.loadActiveModule();
    return;
  }
  if (command === "closeOthers") return closeOtherTabs();
  if (command === "closeLeft") return closeLeftTabs();
  if (command === "closeRight") return closeRightTabs();
  if (command === "closeAll") return closeAllTabs();
}
async function closeOpenTab(moduleId: string) {
  if (openTabs.value.length <= 1) return;
  const index = openTabs.value.findIndex((item) => item.id === moduleId);
  if (index < 0) return;
  const closingActive = store.activeModuleId === moduleId;
  openTabs.value = openTabs.value.filter((item) => item.id !== moduleId);
  if (closingActive) {
    const next = openTabs.value[Math.max(0, index - 1)] || openTabs.value[0];
    if (typeof window !== "undefined") {
      window.localStorage.setItem(activeTabStorageKey, next.id);
    }
    await store.selectModule(next.id);
  }
}

watch(
  openTabs,
  (tabs) => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(openTabStorageKey, JSON.stringify(tabs.map((item) => item.id)));
  },
  { deep: true, immediate: true }
);

watch(
  aiLightboxTaskId,
  (id) => {
    if (typeof document === "undefined") return;
    document.body.style.overflow = id ? "hidden" : "";
  }
);

function updateAiComposerClearance() {
  if (typeof document === "undefined" || typeof window === "undefined") return;
  const composer = aiFloatingComposerRef.value;
  if (!composer || !imageWorkspaceModuleIds.includes(store.activeModuleId)) {
    document.documentElement.style.removeProperty("--ai-composer-clearance");
    return;
  }
  const rect = composer.getBoundingClientRect();
  const clearance = Math.max(184, Math.ceil(window.innerHeight - rect.top + 24));
  document.documentElement.style.setProperty("--ai-composer-clearance", `${clearance}px`);
}

async function refreshAiComposerClearance() {
  if (typeof window === "undefined") return;
  await nextTick();
  aiPromptInputRef.value?.adjustHeight();
  await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()));
  aiComposerResizeObserver?.disconnect();
  aiComposerResizeObserver = null;
  const composer = aiFloatingComposerRef.value;
  if (!composer || !imageWorkspaceModuleIds.includes(store.activeModuleId)) {
    updateAiComposerClearance();
    return;
  }
  updateAiComposerClearance();
  aiComposerResizeObserver = new ResizeObserver(updateAiComposerClearance);
  aiComposerResizeObserver.observe(composer);
}

watch(
  () => store.activeModuleId,
  () => {
    void refreshAiComposerClearance();
  },
  { flush: "post", immediate: true }
);

watch(
  () => aiReferenceImages.value.length,
  () => {
    void refreshAiComposerClearance();
  },
  { flush: "post" }
);

onMounted(() => {
  if (typeof window === "undefined") return;
  if (isUserConsole.value) void hydrateAiImageDraft();
  if (isUserConsole.value && hasAuthToken()) hydrateAiSettingsFromStore();
  void refreshAiComposerClearance();
  aiTaskClockTimer = window.setInterval(() => {
    aiTaskClockNow.value = Date.now();
  }, 1000);
  window.addEventListener("mousedown", handleAiImageContextMenuDismiss, true);
  window.addEventListener("wheel", handleAiImageContextMenuDismiss, true);
  window.addEventListener("scroll", handleAiImageContextMenuDismiss, true);
  window.addEventListener("resize", handleAiImageContextMenuDismiss);
  window.addEventListener("resize", updateAiComposerClearance);
  window.addEventListener("keydown", handleAiLightboxKeydown);
  window.addEventListener("keydown", handleGlobalCommandShortcut);
  window.addEventListener("focus", handleAiWorkspaceVisibilityRefresh);
  window.addEventListener("popstate", handleAgentCenterHistoryPopState);
  window.addEventListener("popstate", handleAdminEnterpriseHistoryPopState);
  document.addEventListener("visibilitychange", handleAiWorkspaceVisibilityRefresh);
});

onBeforeUnmount(() => {
  clearAiTaskLongPressTimer();
  clearWirelessCanvasLoadTimer();
  if (aiImageDraftSaveTimer) {
    window.clearTimeout(aiImageDraftSaveTimer);
    aiImageDraftSaveTimer = null;
  }
  if (aiTaskClockTimer) {
    window.clearInterval(aiTaskClockTimer);
    aiTaskClockTimer = null;
  }
  if (aiOriginalImagePrefetchTimer) {
    window.clearTimeout(aiOriginalImagePrefetchTimer);
    aiOriginalImagePrefetchTimer = null;
  }
  if (videoHistorySaveTimer) {
    window.clearTimeout(videoHistorySaveTimer);
    videoHistorySaveTimer = null;
  }
  if (videoInputDraftSaveTimer) {
    window.clearTimeout(videoInputDraftSaveTimer);
    videoInputDraftSaveTimer = null;
  }
  if (videoHistoryPollTimer) {
    window.clearInterval(videoHistoryPollTimer);
    videoHistoryPollTimer = null;
  }
  stopAiGenerationPolling();
  if (typeof window !== "undefined") {
    window.removeEventListener("mousedown", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("wheel", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("scroll", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("resize", handleAiImageContextMenuDismiss);
    window.removeEventListener("resize", updateAiComposerClearance);
    window.removeEventListener("resize", adjustVideoPromptHeight);
    window.removeEventListener("keydown", handleAiLightboxKeydown);
    window.removeEventListener("keydown", handleGlobalCommandShortcut);
    window.removeEventListener("focus", handleAiWorkspaceVisibilityRefresh);
    window.removeEventListener("popstate", handleAgentCenterHistoryPopState);
    window.removeEventListener("popstate", handleAdminEnterpriseHistoryPopState);
    document.removeEventListener("visibilitychange", handleAiWorkspaceVisibilityRefresh);
    document.body.style.overflow = "";
    document.documentElement.style.removeProperty("--ai-composer-clearance");
  }
  aiComposerResizeObserver?.disconnect();
  aiComposerResizeObserver = null;
  clearVideoImageUpload();
  clearVideoSourceUpload();
  if (aiImageDraftHydrated) void writeAiImageDraft(aiImageDraftPayload()).catch(() => undefined);
});
function settingsList(key: string): AdminRecord[] {
  const value = (store.data as Record<string, unknown>)[key];
  return Array.isArray(value) ? (value as AdminRecord[]) : [];
}

function aiDataList(key: string): AdminRecord[] {
  const value = (store.data as Record<string, unknown>)[key];
  return Array.isArray(value) ? (value as AdminRecord[]) : [];
}

const aiModules = computed(() => aiDataList("modules"));
const aiModels = computed(() => aiDataList("models"));
const aiSchemas = computed(() => aiDataList("schemas"));
const aiLimits = computed(() => aiDataList("limits"));
const aiChannels = computed(() => aiDataList("channels"));
const aiLogs = computed(() => aiDataList("logs"));
const aiCapabilitySummary = computed(() => {
  const value = (store.data as AdminRecord).summary;
  return value && typeof value === "object" && !Array.isArray(value) ? value as AdminRecord : {};
});
const aiCapabilityMetrics = computed(() => [
  { label: "能力模块", value: String(aiModules.value.length || aiCapabilitySummary.value.modules || 0), hint: "生图 / 视频 / PPT" },
  { label: "模型配置", value: String(aiModels.value.length || aiCapabilitySummary.value.models || 0), hint: "按 module_code 绑定" },
  { label: "参数 Schema", value: String(aiSchemas.value.length || aiCapabilitySummary.value.schemas || 0), hint: "模型支持参数" },
  { label: "租户限制", value: String(aiLimits.value.length || aiCapabilitySummary.value.limits || 0), hint: "套餐/租户可用范围" },
  { label: "上游通道", value: String(aiChannels.value.length), hint: "从 API 设置读取" },
  { label: "调用日志", value: String(aiLogs.value.length || aiCapabilitySummary.value.logs || 0), hint: "含扣费与成本快照" }
]);
const aiCapabilityViewModel = computed(() => ({
  activeModuleId: store.activeModuleId,
  loading: store.loading,
  saving: store.saving,
  metrics: aiCapabilityMetrics.value,
  modules: aiModules.value,
  models: aiModels.value,
  schemas: aiSchemas.value,
  limits: aiLimits.value,
  channels: aiChannels.value,
  logs: aiLogs.value,
  refresh: () => store.loadActiveModule(),
  navigate: selectAdminModule,
  text: aiText,
  list: aiList,
  object: aiObject,
  audienceLabel: aiAudienceLabel,
  moduleLabel: aiModuleLabel,
  limitScope: aiLimitScope,
  jsonPreview: aiJsonPreview,
  schemaFields: aiSchemaFields,
  schemaFieldLabel: aiSchemaFieldLabel,
  schemaFieldOptionsText: aiSchemaFieldOptionsText,
  statusType,
  statusLabel,
  isActiveStatus,
  moneyYuan,
  toggleModule: toggleAIModule,
  editModulePackages: editAIModulePackages,
  editModuleModels: editAIModuleModels,
  createModel: createAIModel,
  editModel: editAIModel,
  editModelCapabilities: editAIModelCapabilities,
  toggleModel: toggleAIModel,
  editSchema: editAIParameterSchema,
  toggleSchema: toggleAIParameterSchema,
  editLimit: editAILimitJSON,
  toggleLimit: toggleAILimit
}));

function aiValue(row: AdminRecord, ...keys: string[]) {
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(row, key) && row[key] !== undefined && row[key] !== null) {
      return row[key];
    }
  }
  return undefined;
}

function aiText(row: AdminRecord, ...keys: string[]) {
  const value = aiValue(row, ...keys);
  if (Array.isArray(value)) return value.map((item) => String(item)).join("、");
  if (value && typeof value === "object") return JSON.stringify(value);
  return value === undefined || value === null ? "" : String(value);
}

function aiList(row: AdminRecord, ...keys: string[]) {
  const value = aiValue(row, ...keys);
  if (Array.isArray(value)) return value.map((item) => String(item).trim()).filter(Boolean);
  if (typeof value === "string") return uniqueNonEmptyStrings(value.split(/[,，]/));
  return [];
}

function aiObject(row: AdminRecord, ...keys: string[]) {
  const value = aiValue(row, ...keys);
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function aiSchemaFields(row: AdminRecord): AdminRecord[] {
  const schema = aiObject(row, "schema_json", "schemaJson");
  return Array.isArray(schema.fields) ? schema.fields as AdminRecord[] : [];
}

function aiSchemaFieldLabel(field: AdminRecord) {
  const key = aiText(field, "key");
  const label = aiText(field, "label") || key;
  if (!key || key === label) return label || "-";
  return `${label} / ${key}`;
}

function aiSchemaFieldOptionsText(field: AdminRecord) {
  const options = aiValue(field, "options");
  if (Array.isArray(options) && options.length) {
    const text = options.map((item) => String(item)).join(" / ");
    return text.length > 56 ? `${text.slice(0, 53)}...` : text;
  }
  const type = aiText(field, "type");
  return type ? `类型: ${type}` : "";
}

function aiSchemaValueText(value: unknown) {
  if (value === undefined || value === null) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function aiSchemaOptionsText(value: unknown) {
  if (!Array.isArray(value)) return "";
  return value.map((item) => aiSchemaValueText(item)).join(",");
}

function parseAISchemaScalar(text: string) {
  const value = text.trim();
  if (value === "") return "";
  if (/^(true|false)$/i.test(value)) return value.toLowerCase() === "true";
  if (/^-?\d+(\.\d+)?$/.test(value)) return Number(value);
  try {
    if ((value.startsWith("{") && value.endsWith("}")) || (value.startsWith("[") && value.endsWith("]"))) {
      return JSON.parse(value);
    }
  } catch {
    return value;
  }
  return value;
}

function parseAISchemaOptions(text: string) {
  return text
    .split(/[\n,，]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .map(parseAISchemaScalar);
}

function aiSchemaEditableFields(schema: Record<string, unknown>) {
  const fields = Array.isArray(schema.fields) ? schema.fields as AdminRecord[] : [];
  return fields.map((field) => ({
    ...field,
    key: aiText(field, "key"),
    label: aiText(field, "label"),
    type: aiText(field, "type") || "text",
    required: Boolean(field.required),
    visible: field.visible !== false,
    user_editable: field.user_editable !== false,
    defaultText: aiSchemaValueText(field.default),
    optionsText: aiSchemaOptionsText(field.options),
    placeholderText: aiText(field, "placeholder"),
    unitText: aiText(field, "unit")
  }));
}

function normalizeAISchemaEditableField(field: AdminRecord) {
  const next: AdminRecord = { ...field };
  next.key = String(field.key || "").trim();
  next.label = String(field.label || "").trim();
  next.type = String(field.type || "text").trim();
  next.required = Boolean(field.required);
  next.visible = field.visible !== false;
  next.user_editable = field.user_editable !== false;
  const defaultText = String(field.defaultText || "").trim();
  if (defaultText) next.default = parseAISchemaScalar(defaultText);
  else delete next.default;
  const options = parseAISchemaOptions(String(field.optionsText || ""));
  if (options.length) next.options = options;
  else delete next.options;
  const placeholder = String(field.placeholderText || "").trim();
  if (placeholder) next.placeholder = placeholder;
  else delete next.placeholder;
  const unit = String(field.unitText || "").trim();
  if (unit) next.unit = unit;
  else delete next.unit;
  delete next.defaultText;
  delete next.optionsText;
  delete next.placeholderText;
  delete next.unitText;
  return next;
}

function assertAISchemaEditableFields(fields: AdminRecord[]) {
  const seen = new Set<string>();
  fields.forEach((field, index) => {
    const key = String(field.key || "").trim();
    if (!key) throw new Error(`第 ${index + 1} 行缺少 key`);
    if (seen.has(key)) throw new Error(`参数 key 重复：${key}`);
    seen.add(key);
  });
}

const aiSchemaFieldTypes = ["text", "textarea", "select", "number", "switch", "image_upload", "file_upload", "template_select"];

function aiJsonPreview(value: unknown) {
  const text = JSON.stringify(value || {});
  return text.length > 120 ? `${text.slice(0, 117)}...` : text;
}

function isActiveStatus(value: unknown) {
  return String(value || "").toUpperCase() === "ACTIVE" || value === true;
}

function nextAIStatus(row: AdminRecord) {
  return isActiveStatus(row.status) ? "DISABLED" : "ACTIVE";
}

function aiModuleLabel(moduleCode: string) {
  const code = moduleCode.trim();
  const module = aiModules.value.find((item) => aiText(item, "module_code", "moduleCode") === code);
  return module?.name ? `${module.name}` : code || "-";
}

function aiAudienceLabel(module: AdminRecord) {
  const targets = [];
  if (Boolean(aiValue(module, "allow_agents", "allowAgents"))) targets.push("代理商");
  if (Boolean(aiValue(module, "allow_end_users", "allowEndUsers"))) targets.push("终端用户");
  return targets.join("、") || "-";
}

function aiLimitScope(row: AdminRecord) {
  const packageId = aiText(row, "package_id", "packageId");
  const agentId = aiText(row, "agent_id", "agentId");
  const tenantId = aiText(row, "tenant_id", "tenantId");
  if (packageId) return `套餐：${packageId}`;
  if (agentId) return `代理：${agentId}`;
  return `租户：${tenantId || "default"}`;
}

async function askAIJson(title: string, value: unknown) {
  const result = await ElMessageBox.prompt("JSON 配置", title, {
    inputType: "textarea",
    inputValue: JSON.stringify(value || {}, null, 2),
    confirmButtonText: "保存",
    cancelButtonText: "取消",
    inputValidator: (text: string) => {
      try {
        JSON.parse(text || "{}");
        return true;
      } catch (error) {
        return error instanceof Error ? error.message : "JSON 格式不正确";
      }
    }
  });
  return JSON.parse(String(result.value || "{}"));
}

async function toggleAIModule(row: AdminRecord) {
  const code = aiText(row, "module_code", "moduleCode");
  if (!code) throw new Error("缺少 module_code");
  await store.mutate("PATCH", `/admin/ai/modules/${code}`, { status: nextAIStatus(row) });
  ElMessage.success("AI 能力模块状态已更新");
}

async function editAIModulePackages(row: AdminRecord) {
  const code = aiText(row, "module_code", "moduleCode");
  const text = await ask("开放套餐 ID，逗号分隔", aiList(row, "open_package_ids", "openPackageIds").join(","));
  await store.mutate("PATCH", `/admin/ai/modules/${code}`, { open_package_ids: uniqueNonEmptyStrings(text.split(/[,，]/)) });
  ElMessage.success("开放套餐已更新");
}

async function editAIModuleModels(row: AdminRecord) {
  const code = aiText(row, "module_code", "moduleCode");
  const text = await ask("绑定模型，逗号分隔", aiList(row, "bound_models", "boundModels").join(","));
  await store.mutate("PATCH", `/admin/ai/modules/${code}`, { bound_models: uniqueNonEmptyStrings(text.split(/[,，]/)) });
  ElMessage.success("绑定模型已更新");
}

async function toggleAIModel(row: AdminRecord) {
  await store.mutate("PATCH", `/admin/ai/models/${row.id}`, { status: nextAIStatus(row) });
  ElMessage.success("AI 模型状态已更新");
}

async function createAIModel() {
  await editAIModel({});
}

async function editAIModel(row: AdminRecord) {
  const isCreate = !row.id;
  const existingModelType = aiText(row, "model_type", "modelType") || "image";
  const form = {
    modelName: aiText(row, "model_name", "modelName"),
    modelType: existingModelType,
    provider: aiText(row, "provider") || "NewAPI",
    channelId: aiText(row, "channel_id", "channelId"),
    moduleCode: aiText(row, "module_code", "moduleCode") || "image_generation",
    capabilityCode: aiList(row, "capability_code", "capabilityCode").join(","),
    fallbackModel: aiText(row, "fallback_model", "fallbackModel"),
    sortWeight: String(Number(aiValue(row, "sort_weight", "sortWeight") || 10)),
    allowFallbackSwitch: Boolean(aiValue(row, "allow_fallback_switch", "allowFallbackSwitch")),
    providerName: aiText(row, "provider_name"),
    providerCompany: aiText(row, "provider_company"),
    algorithmName: aiText(row, "algorithm_name"),
    algorithmFilingNo: aiText(row, "algorithm_filing_no"),
    algorithmType: aiText(row, "algorithm_type") || existingModelType,
    contractStatus: aiText(row, "contract_status") || "draft",
    contractExpireAt: aiText(row, "contract_expire_at"),
    complianceStatus: aiText(row, "compliance_status") || "draft",
    allowedTerminals: aiList(row, "allowed_terminals").join(",") || "pc,web,h5,miniprogram",
    allowedCapabilities: aiList(row, "allowed_capabilities").join(",") || existingModelType,
    miniprogramEnabled: Boolean(aiValue(row, "miniprogram_enabled")),
    complianceRemark: aiText(row, "compliance_remark"),
    modelVersion: aiText(row, "model_version"),
    status: String(row.status || "ACTIVE")
  };
  const field = (label: string, key: keyof typeof form, placeholder = "") => h("label", { class: "channel-dialog-field" }, [
    h("span", null, label),
    h("input", {
      class: "channel-dialog-input",
      value: String(form[key] ?? ""),
      placeholder,
      onInput: (event: Event) => {
        (form[key] as string) = (event.target as HTMLInputElement).value;
      }
    })
  ]);
  const select = (label: string, key: keyof typeof form, options: Array<{ label: string; value: string }>) => h("label", { class: "channel-dialog-field" }, [
    h("span", null, label),
    h("select", {
      class: "channel-dialog-input",
      value: String(form[key] ?? ""),
      onChange: (event: Event) => {
        (form[key] as string) = (event.target as HTMLSelectElement).value;
      }
    }, options.map((option) => h("option", { value: option.value }, option.label)))
  ]);
  if (!form.channelId && form.modelName) {
    const matchedChannel = aiChannels.value.find(channel => aiList(channel, "models").some(modelName => modelName.toLowerCase() === form.modelName.toLowerCase()));
    if (matchedChannel) form.channelId = String(matchedChannel.id || "");
  }
  const channelOptions = [
    { label: "自动匹配（按通道优先级）", value: "", disabled: false },
    ...aiChannels.value.map(channel => ({
      label: `${String(channel.name || channel.id)} · ${statusLabel(channel.status)}`,
      value: String(channel.id || ""),
      disabled: !["ACTIVE", "ENABLED", "CONFIGURABLE"].includes(String(channel.status || "").toUpperCase())
    })).filter(option => option.value)
  ];
  const channelSelect = () => h("label", { class: "channel-dialog-field" }, [
    h("span", null, "上游供应商"),
    h("select", {
      class: "channel-dialog-input",
      value: form.channelId,
      onChange: (event: Event) => {
        form.channelId = (event.target as HTMLSelectElement).value;
        const channel = aiChannels.value.find(item => String(item.id || "") === form.channelId);
        form.provider = channel ? String(channel.name || channel.id) : (form.modelName.startsWith("mock-") ? "Local" : "Auto");
      }
    }, channelOptions.map(option => h("option", { value: option.value, disabled: option.disabled }, option.label))),
    h("small", { class: "channel-dialog-help" }, form.channelId ? "新任务将优先使用该通道；请确保通道模型列表包含当前模型名。" : "不指定时，按支持该模型的启用通道和优先级自动选择。")
  ]);
  await ElMessageBox({
    title: isCreate ? "新增模型" : "编辑模型",
    message: h("div", { class: "channel-dialog-form" }, [
      field("模型名称", "modelName", "例如 gpt-image-2"),
      select("模型类型", "modelType", [
        { label: "image", value: "image" },
        { label: "video", value: "video" },
        { label: "text", value: "text" },
        { label: "multimodal", value: "multimodal" }
      ]),
      channelSelect(),
      select("所属模块", "moduleCode", [
        { label: "图片生成", value: "image_generation" },
        { label: "视频生成", value: "video_generation" },
        { label: "PPT 文档生成", value: "ppt_generation" }
      ]),
      field("能力编码", "capabilityCode", "例如 text_to_image,image_to_image"),
      field("Fallback 模型", "fallbackModel", "例如 mock-standard，可留空"),
      field("排序权重", "sortWeight", "数字越小越靠前"),
      field("技术提供方", "providerName", "实际技术提供方，不能填写 new-api"),
      field("技术主体公司全称", "providerCompany", "按合作协议和备案材料填写"),
      field("算法名称", "algorithmName"),
      field("算法备案编号", "algorithmFilingNo"),
      field("算法类型", "algorithmType", "例如 image"),
      select("合作协议状态", "contractStatus", [{ label: "草稿", value: "draft" }, { label: "有效", value: "valid" }, { label: "已过期", value: "expired" }]),
      field("合作协议到期时间", "contractExpireAt", "YYYY-MM-DD"),
      select("合规状态", "complianceStatus", [{ label: "草稿", value: "draft" }, { label: "待审核", value: "pending" }, { label: "已通过", value: "approved" }, { label: "已驳回", value: "rejected" }, { label: "已过期", value: "expired" }]),
      field("允许终端", "allowedTerminals", "pc,web,h5,miniprogram"),
      field("允许能力", "allowedCapabilities", "text,image,video"),
      field("模型版本", "modelVersion"),
      field("合规备注", "complianceRemark"),
      select("状态", "status", [
        { label: "启用", value: "ACTIVE" },
        { label: "停用", value: "DISABLED" }
      ]),
      h("label", { class: "channel-dialog-check" }, [
        h("input", {
          type: "checkbox",
          checked: form.allowFallbackSwitch,
          onChange: (event: Event) => {
            form.allowFallbackSwitch = (event.target as HTMLInputElement).checked;
          }
        }),
        h("span", null, "允许用户切换 Fallback")
      ]),
      h("label", { class: "channel-dialog-check" }, [
        h("input", { type: "checkbox", checked: form.miniprogramEnabled, onChange: (event: Event) => { form.miniprogramEnabled = (event.target as HTMLInputElement).checked; } }),
        h("span", null, "允许小程序使用（后端强制检查备案、终端和协议有效期）")
      ])
    ]),
    confirmButtonText: "保存模型",
    cancelButtonText: "取消",
    beforeClose: async (action, instance, done) => {
      if (action !== "confirm") {
        done();
        return;
      }
      const modelName = form.modelName.trim();
      const sortWeight = Number(form.sortWeight);
      if (!modelName) {
        ElMessage.error("模型名称不能为空");
        return;
      }
      if (!Number.isFinite(sortWeight) || sortWeight <= 0) {
        ElMessage.error("排序权重必须是大于 0 的数字");
        return;
      }
      const selectedChannel = aiChannels.value.find(channel => String(channel.id || "") === form.channelId);
      if (selectedChannel && !aiList(selectedChannel, "models").some(item => item.toLowerCase() === modelName.toLowerCase())) {
        ElMessage.error(`上游通道“${String(selectedChannel.name || selectedChannel.id)}”尚未配置模型 ${modelName}`);
        return;
      }
      if (form.miniprogramEnabled) {
        const allowedTerminals = uniqueNonEmptyStrings(form.allowedTerminals.split(/[,，]/)).map(item => item.toLowerCase());
        const allowedCapabilities = uniqueNonEmptyStrings(form.allowedCapabilities.split(/[,，]/)).map(item => item.toLowerCase());
        const missing: string[] = [];
        if (!allowedTerminals.includes("miniprogram")) missing.push("允许终端需包含 miniprogram");
        if (!allowedCapabilities.includes(form.modelType.toLowerCase())) missing.push(`允许能力需包含 ${form.modelType.toLowerCase()}`);
        if (!form.providerName.trim()) missing.push("技术提供方");
        if (!form.providerCompany.trim()) missing.push("技术主体公司全称");
        if (!form.algorithmName.trim()) missing.push("算法名称");
        if (!form.algorithmFilingNo.trim()) missing.push("算法备案编号");
        if (!form.algorithmType.trim()) missing.push("算法类型");
        if (form.complianceStatus !== "approved") missing.push("合规状态需为已通过");
        if (form.contractStatus !== "valid") missing.push("合作协议状态需为有效");
        const contractExpiry = Date.parse(form.contractExpireAt.trim());
        if (!form.contractExpireAt.trim() || !Number.isFinite(contractExpiry) || contractExpiry <= Date.now()) missing.push("有效的合作协议到期时间");
        const filingSubjects = [form.providerName, form.providerCompany].map(value => value.toLowerCase().replace(/[-_\s]/g, ""));
        if (filingSubjects.includes("newapi")) missing.push("技术提供方/主体不能填写 NewAPI 网关");
        if (missing.length) {
          ElMessage.error(`开启小程序前请补齐：${missing.join("、")}`);
          return;
        }
      }
      instance.confirmButtonLoading = true;
      try {
        const payload = {
          model_name: modelName,
          model_type: form.modelType.trim(),
          provider: form.provider.trim(),
          channel_id: form.channelId.trim(),
          module_code: form.moduleCode.trim(),
          capability_code: uniqueNonEmptyStrings(form.capabilityCode.split(/[,，]/)),
          fallback_model: form.fallbackModel.trim(),
          sort_weight: Math.round(sortWeight),
          allow_fallback_switch: form.allowFallbackSwitch,
          status: form.status.trim(),
          provider_name: form.providerName.trim(),
          provider_company: form.providerCompany.trim(),
          algorithm_name: form.algorithmName.trim(),
          algorithm_filing_no: form.algorithmFilingNo.trim(),
          algorithm_type: form.algorithmType.trim(),
          contract_status: form.contractStatus.trim(),
          contract_expire_at: form.contractExpireAt.trim(),
          compliance_status: form.complianceStatus.trim(),
          allowed_terminals: uniqueNonEmptyStrings(form.allowedTerminals.split(/[,，]/)),
          allowed_capabilities: uniqueNonEmptyStrings(form.allowedCapabilities.split(/[,，]/)),
          miniprogram_enabled: form.miniprogramEnabled,
          compliance_remark: form.complianceRemark.trim(),
          model_version: form.modelVersion.trim()
        };
        if (isCreate) {
          await store.mutate("POST", "/admin/ai/models", payload);
        } else {
          await store.mutate("PATCH", `/admin/ai/models/${row.id}`, payload);
        }
        ElMessage.success(isCreate ? "AI 模型已新增" : "AI 模型已更新");
        done();
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : (isCreate ? "新增模型失败" : "保存模型失败"));
      } finally {
        instance.confirmButtonLoading = false;
      }
    },
    customClass: "channel-create-dialog"
  });
}

async function editAIModelCapabilities(row: AdminRecord) {
  const text = await ask("模型能力编码，逗号分隔", aiList(row, "capability_code", "capabilityCode").join(","));
  await store.mutate("PATCH", `/admin/ai/models/${row.id}`, { capability_code: uniqueNonEmptyStrings(text.split(/[,，]/)) });
  ElMessage.success("模型能力已更新");
}

async function toggleAIParameterSchema(row: AdminRecord) {
  await store.mutate("PATCH", `/admin/ai/parameter-schemas/${row.id}`, { status: nextAIStatus(row) });
  ElMessage.success("参数 Schema 状态已更新");
}

async function editAIParameterSchema(row: AdminRecord) {
  const schema = { ...aiObject(row, "schema_json", "schemaJson") };
  const fields = ref<AdminRecord[]>(aiSchemaEditableFields(schema));
  const title = `编辑参数 Schema：${row.id}`;
  const SchemaEditor = {
    name: "AISchemaEditorDialog",
    setup() {
      const addField = () => {
        fields.value.push({
          key: "",
          label: "",
          type: "text",
          required: false,
          visible: true,
          user_editable: true,
          defaultText: "",
          optionsText: "",
          placeholderText: "",
          unitText: ""
        });
      };
      const removeField = (index: number) => {
        fields.value.splice(index, 1);
      };
      const updateField = (index: number, key: string, value: unknown) => {
        fields.value[index] = { ...fields.value[index], [key]: value };
      };
      const input = (index: number, key: string, label: string, placeholder = "") => h("label", { class: "ai-schema-editor-cell" }, [
        h("span", label),
        h("input", {
          value: String(fields.value[index][key] || ""),
          placeholder,
          onInput: (event: Event) => updateField(index, key, (event.target as HTMLInputElement).value)
        })
      ]);
      const textarea = (index: number, key: string, label: string, placeholder = "") => h("label", { class: "ai-schema-editor-cell ai-schema-editor-cell-wide" }, [
        h("span", label),
        h("textarea", {
          value: String(fields.value[index][key] || ""),
          placeholder,
          rows: 3,
          onInput: (event: Event) => updateField(index, key, (event.target as HTMLTextAreaElement).value)
        })
      ]);
      const checkbox = (index: number, key: string, label: string) => h("label", { class: "ai-schema-editor-check" }, [
        h("input", {
          type: "checkbox",
          checked: Boolean(fields.value[index][key]),
          onChange: (event: Event) => updateField(index, key, (event.target as HTMLInputElement).checked)
        }),
        h("span", label)
      ]);
      return () => h("div", { class: "ai-schema-editor" }, [
        h("p", { class: "ai-schema-editor-hint" }, "可直接编辑字段、默认值和选项。选项支持逗号或换行分隔，例如：1024x1024,1024x1536,1536x1024。"),
        h("div", { class: "ai-schema-editor-rows" }, fields.value.map((field, index) => h("section", { class: "ai-schema-editor-row", key: `${field.key || "field"}-${index}` }, [
          h("div", { class: "ai-schema-editor-row-head" }, [
            h("strong", field.key ? `${field.label || field.key}` : `新参数 ${index + 1}`),
            h("button", { type: "button", onClick: () => removeField(index) }, "删除")
          ]),
          h("div", { class: "ai-schema-editor-grid" }, [
            input(index, "key", "Key", "duration"),
            input(index, "label", "名称", "视频时长"),
            h("label", { class: "ai-schema-editor-cell" }, [
              h("span", "类型"),
              h("select", {
                value: String(field.type || "text"),
                onChange: (event: Event) => updateField(index, "type", (event.target as HTMLSelectElement).value)
              }, aiSchemaFieldTypes.map((type) => h("option", { value: type }, type)))
            ]),
            input(index, "defaultText", "默认值", "5"),
            textarea(index, "optionsText", "选项", "1024x1024,1024x1536,1536x1024"),
            input(index, "unitText", "单位", "秒"),
            input(index, "placeholderText", "占位提示", "描述视频画面、运动和风格"),
            h("div", { class: "ai-schema-editor-switches" }, [
              checkbox(index, "required", "必填"),
              checkbox(index, "visible", "显示"),
              checkbox(index, "user_editable", "用户可编辑")
            ])
          ])
        ]))),
        h("button", { type: "button", class: "ai-schema-editor-add", onClick: addField }, "+ 新增参数")
      ]);
    }
  };

  await ElMessageBox({
    title,
    message: h(SchemaEditor),
    showCancelButton: true,
    confirmButtonText: "保存",
    cancelButtonText: "取消",
    customClass: "ai-schema-editor-dialog",
    beforeClose: async (action, instance, done) => {
      if (action !== "confirm") {
        done();
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        assertAISchemaEditableFields(fields.value);
        const nextSchema = {
          ...schema,
          fields: fields.value.map(normalizeAISchemaEditableField)
        };
        await store.mutate("PATCH", `/admin/ai/parameter-schemas/${row.id}`, { schema_json: nextSchema, status: row.status || "ACTIVE" });
        ElMessage.success("参数 Schema 已更新");
        done();
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "保存参数 Schema 失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}

async function toggleAILimit(row: AdminRecord) {
  await store.mutate("PATCH", `/admin/ai/tenant-module-limits/${row.id}`, { status: nextAIStatus(row) });
  ElMessage.success("租户限制状态已更新");
}

async function editAILimitJSON(row: AdminRecord) {
  const limit = await askAIJson("编辑租户参数限制", aiObject(row, "limit_json", "limitJson"));
  await store.mutate("PATCH", `/admin/ai/tenant-module-limits/${row.id}`, { limit_json: limit, status: row.status || "ACTIVE" });
  ElMessage.success("租户限制已更新");
}

const rows = computed(() => {
  if (Array.isArray(store.data)) return flattenRows(store.data as AdminRecord[]);
  const data = store.data as { items?: unknown[]; withdrawals?: unknown[]; apiChannels?: unknown[]; apiModels?: unknown[]; apiKeys?: unknown[]; customerGroups?: unknown[]; brand?: Record<string, unknown>; recentTasks?: unknown[]; recentAssets?: unknown[]; account?: AdminRecord };
  if (store.activeModuleId === "userDashboard") {
    const assets = Array.isArray(data.recentAssets) ? data.recentAssets : [];
    const tasks = Array.isArray(data.recentTasks) ? data.recentTasks : [];
    return flattenRows([...assets, ...tasks].slice(0, 10) as AdminRecord[]);
  }
  if (store.activeModuleId === "userMembership") {
    return data.account ? [data.account] : [];
  }
  if (store.activeModuleId === "userOrders") {
    return userMembershipOrders.value;
  }
  if (store.activeModuleId === "system") {
    return systemRows(data);
  }
  if (store.activeModuleId === "commissions") {
    const commissions = (Array.isArray(data.items) ? data.items : []).map((item) => ({ ...(item as AdminRecord), recordType: "分润" }));
    const withdrawals = (Array.isArray(data.withdrawals) ? data.withdrawals : []).map((item) => ({ ...(item as AdminRecord), recordType: "提现", orderId: "-", rate: "-" }));
    return [...commissions, ...withdrawals];
  }
  if (isPartnerModule.value) return partnerRows();
  if (isOperationCenterModule.value) return operationCenterRows();
  const items = Array.isArray(data.items) ? data.items : Array.isArray(data.withdrawals) ? data.withdrawals : [];
  if (["customers", "userManagement"].includes(store.activeModuleId)) return flattenRows((items as AdminRecord[]).map(enrichUserIdentityRow));
  return flattenRows(items as AdminRecord[]);
});

const selectedBillingCustomerId = ref("");
const billingData = computed(() => store.data as {
  metrics?: Array<{ label: string; value: unknown; unit?: string }>;
  workflow?: unknown[];
  customers?: unknown[];
  products?: unknown[];
  plans?: unknown[];
  subscriptions?: unknown[];
  events?: unknown[];
  usage?: unknown[];
  billableMetrics?: unknown[];
  charges?: unknown[];
  fees?: unknown[];
  wallets?: unknown[];
  coupons?: unknown[];
  invoices?: unknown[];
  creditNotes?: unknown[];
  paymentRequests?: unknown[];
  payments?: unknown[];
});

function billingList(key: "customers" | "products" | "plans" | "subscriptions" | "events" | "usage" | "billableMetrics" | "charges" | "fees" | "wallets" | "coupons" | "invoices" | "creditNotes" | "paymentRequests" | "payments") {
  const value = billingData.value[key];
  return Array.isArray(value) ? flattenRows(value as AdminRecord[]) : [];
}

function partnerUsageEvents(events: unknown): AdminRecord[] {
  if (!Array.isArray(events)) return [];
  return events.filter((item) => {
    const event = item as AdminRecord;
    return String(event.metricCode || "") === "image.generations" && Number(event.pointCost || 0) > 0;
  }) as AdminRecord[];
}

const billingCustomerRows = computed(() => billingList("customers"));
const billingProductRows = computed(() => billingList("products"));
const billingPlanRows = computed(() => billingList("plans"));
const billingSubscriptionRows = computed(() => billingList("subscriptions"));
const billingEventRows = computed(() => billingList("events"));
const billingUsageRows = computed(() => billingList("usage"));
const billingBillableMetricRows = computed(() => billingList("billableMetrics"));
const billingChargeRows = computed(() => billingList("charges"));
const billingFeeRows = computed(() => billingList("fees"));
const billingWalletRows = computed(() => billingList("wallets"));
const billingCouponRows = computed(() => billingList("coupons"));
const billingInvoiceRows = computed(() => billingList("invoices"));
const billingCreditNoteRows = computed(() => billingList("creditNotes"));
const billingPaymentRequestRows = computed(() => billingList("paymentRequests"));
const billingPaymentRows = computed(() => billingList("payments"));

const billingRows = computed(() => {
  const map: Record<string, AdminRecord[]> = {
    billingDashboard: billingCustomerRows.value,
    billingCustomers: billingCustomerRows.value,
    billingProducts: billingProductRows.value,
    billingBillableMetrics: billingBillableMetricRows.value,
    billingCharges: billingChargeRows.value,
    billingSubscriptions: billingSubscriptionRows.value,
    billingEvents: billingEventRows.value,
    billingFees: billingFeeRows.value,
    billingWallets: billingWalletRows.value,
    billingCoupons: billingCouponRows.value,
    billingInvoices: billingInvoiceRows.value,
    billingCreditNotes: billingCreditNoteRows.value,
    billingPaymentRequests: billingPaymentRequestRows.value,
    billingPayments: billingPaymentRows.value
  };
  return map[store.activeModuleId] || [];
});

const filteredBillingRows = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase();
  if (!keyword) return billingRows.value;
  return billingRows.value.filter((row) => Object.values(row).some((value) => String(Array.isArray(value) ? value.join(" ") : value ?? "").toLowerCase().includes(keyword)));
});

const billingColumns = computed(() => {
  const preferred: Record<string, string[]> = {
    billingDashboard: ["customer", "plan", "billingStatus", "prepaidBalanceCents", "paymentMethod"],
    billingCustomers: ["customer", "email", "plan", "billingStatus", "prepaidBalanceCents", "walletCode", "coupon", "taxStatus"],
    billingProducts: ["name", "skuCode", "planType", "billingMode", "plan", "product", "cyclePolicy", "monthlyAmountCents", "metricCode", "includedQuota", "overageUnitPriceCents", "chargeModel", "status"],
    billingBillableMetrics: ["code", "name", "product", "aggregationType", "fieldName", "expression", "recurring", "chargeModels", "status"],
    billingCharges: ["plan", "product", "billableMetricCode", "chargeModel", "amountCents", "freeUnits", "payInAdvance", "invoiceable", "taxes", "status"],
    billingSubscriptions: ["id", "externalId", "customer", "plan", "status", "billingTime", "monthlyAmountCents", "lifetimeUsageCents", "currentPeriodEnd"],
    billingEvents: ["transactionId", "customerId", "subscriptionId", "metricCode", "quantity", "unitAmountCents", "status", "occurredAt"],
    billingFees: ["id", "invoiceId", "feeType", "invoiceableType", "amountCents", "taxesAmountCents", "totalAmountCents", "units", "paymentStatus"],
    billingWallets: ["code", "customer", "status", "balanceCents", "consumedAmountCents", "paymentMethodType", "targetMetrics"],
    billingCoupons: ["code", "name", "couponType", "amountCents", "percentageRate", "frequency", "frequencyDuration", "targets", "status"],
    billingInvoices: ["invoiceNo", "customer", "invoiceType", "subtotalAmountCents", "taxesAmountCents", "totalDueAmountCents", "status", "paymentStatus", "taxStatus"],
    billingCreditNotes: ["number", "invoiceNo", "customer", "reason", "creditAmountCents", "offsetAmountCents", "balanceAmountCents", "creditStatus", "refundStatus"],
    billingPaymentRequests: ["id", "customer", "invoices", "totalDueAmountCents", "paymentStatus", "dunningCampaign", "dueAt", "readyForPaymentProcessing"],
    billingPayments: ["id", "paymentRequestId", "provider", "paymentMethodType", "amountCents", "paymentStatus", "status", "createdAt"]
  };
  const first = billingRows.value[0];
  const fields = preferred[store.activeModuleId] || (first ? Object.keys(first).slice(0, 8) : []);
  return first ? fields.filter((field) => field in first) : fields;
});

const billingTableTitle = computed(() => {
  const titleMap: Record<string, string> = {
    billingDashboard: "客户计费总览",
    billingCustomers: "客户计费档案",
    billingProducts: "支付商品与权益映射",
    billingBillableMetrics: "计量指标",
    billingCharges: "计费规则",
    billingSubscriptions: "订阅生命周期",
    billingEvents: "原始计量事件",
    billingFees: "费用明细",
    billingWallets: "钱包与预付额度",
    billingCoupons: "优惠券",
    billingInvoices: "账单与发票",
    billingCreditNotes: "贷项与红冲",
    billingPaymentRequests: "付款请求与催收",
    billingPayments: "支付与催收"
  };
  return titleMap[store.activeModuleId] || "计费记录";
});

const billingWorkflow = computed(() => {
  const value = billingData.value.workflow;
  return Array.isArray(value) ? flattenRows(value as AdminRecord[]) : [];
});

const billingMetrics = computed(() => {
  const items = Array.isArray(billingData.value.metrics) ? billingData.value.metrics : [];
  return items.map((item) => ({
    label: item.label,
    value: item.unit === "cents" ? formatCell(item.value, "amountCents") : String(item.value ?? "-"),
    hint: metricHint(item.label)
  }));
});

const selectedBillingCustomer = computed<AdminRecord>(() => {
  const customers = billingCustomerRows.value;
  if (!customers.length) return {};
  const selected = customers.find((item) => String(item.id || item.customerId || "") === selectedBillingCustomerId.value);
  return selected || customers[0];
});

const selectedBillingSubscription = computed<AdminRecord>(() => {
  const customerId = String(selectedBillingCustomer.value.id || selectedBillingCustomer.value.customerId || "");
  return billingSubscriptionRows.value.find((item) => String(item.customerId || "") === customerId) || {};
});

function selectBillingCustomer(row: AdminRecord) {
  const customerId = String(row.customerId || row.id || "");
  if (customerId) selectedBillingCustomerId.value = customerId;
}

const filteredRows = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase();
  const status = statusFilter.value;
  return rows.value.filter((row) => {
    const statusText = String(row.status ?? row.active ?? "").toUpperCase();
    const matchesStatus = status === "ALL" || statusText === status;
    const matchesKeyword = !keyword || Object.values(row).some((value) => String(Array.isArray(value) ? value.join(" ") : value ?? "").toLowerCase().includes(keyword));
    return matchesStatus && matchesKeyword;
  });
});
const globalModuleResults = computed(() => {
	const keyword = (isUserConsole.value || isAgentConsole.value ? globalSearchKeyword.value : searchKeyword.value).trim().toLowerCase();
  if (!keyword) return [];
  return modules
    .filter((item) => allowedModuleIds.includes(item.id) && canNavigateToModule(item.id))
    .map((item, index) => {
      const meta = pageMeta[item.id];
      const navigation = adminNavigationSectionForModule(item.id);
      const moduleMatch = [item.id, item.title, meta?.badge, meta?.description].some((value) => String(value || "").toLowerCase().includes(keyword));
      const sectionMatch = String(navigation?.section.title || "").toLowerCase().includes(keyword);
      const groupMatch = String(navigation?.group.title || "").toLowerCase().includes(keyword);
		return { item: { ...item, groupTitle: navigation?.group.title || "", sectionTitle: navigation?.section.title || "" }, index, score: moduleMatch ? 3 : sectionMatch ? 2 : groupMatch ? 1 : 0 };
    })
    .filter((entry) => entry.score > 0)
    .sort((left, right) => right.score - left.score || left.index - right.index)
    .slice(0, 6)
    .map((entry) => entry.item);
});

const currentRecordResults = computed(() => {
	const keyword = (isUserConsole.value || isAgentConsole.value ? globalSearchKeyword.value : searchKeyword.value).trim().toLowerCase();
  if (!keyword) return [];
  return rows.value
    .filter((row) => Object.values(row).some((value) => String(Array.isArray(value) ? value.join(" ") : value ?? "").toLowerCase().includes(keyword)))
    .slice(0, 6)
    .map((row, index) => {
      const record = row as AdminRecord;
      const title = String(record["name"] || record["customer"] || record["email"] || record["id"] || record["item"] || `${store.activeModule.title}记录 ${index + 1}`);
      const desc = Object.entries(record).slice(0, 4).map(([key, value]) => `${columnLabels[key] || key}: ${formatCell(value, key)}`).join(" / ");
      return { key: `${store.activeModuleId}-${index}-${title}`, title, desc, row: record };
    });
});

function openCurrentRecordResult(item: { title: string; row: AdminRecord }) {
  if (isUserConsole.value || isAgentConsole.value) {
    globalSearchKeyword.value = "";
    commandPaletteOpen.value = false;
  } else {
    searchKeyword.value = "";
  }
  const action = visibleRowActions(item.row)[0];
  if (action) {
    void runAction(action.action, item.row);
    return;
  }
  ElMessage.info(`已定位到「${item.title}」，当前模块没有独立详情操作`);
}

async function openGlobalModuleResult(moduleId: string) {
	const query = globalSearchKeyword.value.trim();
	globalSearchKeyword.value = "";
	commandPaletteOpen.value = false;
	trackAdminExperience("SEARCH_RESULT_CLICK", moduleId, moduleId, { resultType: "module", queryLength: query.length });
	await selectAdminModule(moduleId);
}

async function openGlobalBusinessResult(item: GlobalSearchItem) {
	const query = globalSearchKeyword.value.trim();
	globalSearchKeyword.value = "";
	commandPaletteOpen.value = false;
	trackAdminExperience("SEARCH_RESULT_CLICK", item.module, item.recordId, { resultType: item.type, queryLength: query.length });
	await selectAdminModule(item.module);
	searchKeyword.value = item.recordId;
	const typeLabel = ({ customer: "客户", order: "订单", enterprise: "企业", generation_task: "生成任务", payment: "支付流水", invoice: "发票" } as Record<string, string>)[item.type] || "业务记录";
	ElMessage.success(`已定位到${typeLabel}「${item.title}」`);
}

function metricValue(keyword: string, fallback: string) {
  const hit = metrics.value.find((metric) => metric.label.includes(keyword));
  return hit?.value || fallback;
}

function metricHint(label: string) {
  if (label.includes("订单") || label.includes("收入") || label.includes("金额")) return "交易与收款口径";
  if (label.includes("客户") || label.includes("用户")) return "客户运营口径";
  if (label.includes("渠道") || label.includes("代理")) return "渠道增长口径";
  if (label.includes("API") || label.includes("模型") || label.includes("上游")) return "模型网关口径";
  return "当前模块统计";
}

const columns = computed(() => {
  const preferred: Record<string, string[]> = {
    customers: ["name", "mobileMasked", "loginMethods", "wechatBinding", "sourceAgentName", "sourceInviteCode", "plan", "pointsAvailable", "modelRoute", "modelKeyStatus", "status"],
    userManagement: ["id", "name", "email", "mobileMasked", "loginMethods", "wechatBinding", "wechatOpenIdMasked", "role", "plan", "pointsAvailable", "status"],
    partnerCustomers: ["name", "email", "plan", "pointsAvailable", "customerValue", "usageCount", "pointCost", "averagePointCost", "consumedCents", "commissionCents", "latestModel", "latestTaskId", "latestUsageAt", "status"],
    partnerOrders: ["orderId", "orderType", "customer", "model", "pointCost", "amountCents", "commissionCents", "commissionRate", "status", "createdAt"],
    partnerUsage: ["taskId", "customer", "model", "pointCost", "consumedCents", "commissionCents", "balanceBefore", "balanceAfter", "status", "createdAt"],
    partnerCommissions: ["recordType", "settlementSource", "id", "customer", "orderId", "relatedAmountCents", "amountCents", "commissionRate", "status", "createdAt", "reviewedAt"],
    partnerChannels: ["name", "email", "levelLabel", "inviteCode", "customers", "children", "status"],
    partnerMaterials: ["name", "type", "value", "status"],
    partnerAccount: ["item", "value", "status"],
    operationCenterAgents: ["name", "email", "levelLabel", "inviteCode", "operationCenterId", "status", "createdAt"],
    operationCenterOrders: ["orderId", "orderTypeText", "customer", "plan", "amountCents", "platformIncomeCents", "operationCenterId", "fulfillmentStatus", "status", "createdAt"],
    operationCenterCommissions: ["recordType", "settlementSource", "id", "orderId", "amountCents", "settleStatus", "status", "createdAt", "reviewedAt"],
    channels: ["name", "email", "levelLabel", "identity", "openCondition", "keepCondition", "inviteCode", "operationCenterId", "status"],
    operationCenters: ["name", "owner", "region", "inviteCode", "status", "joinOrderId", "joinFeeCents", "approvedAt"],
    products: ["name", "type", "status", "usage"],
    plans: ["name", "priceCents", "grantPoints", "durationDays", "concurrency", "active"],
    orders: ["id", "customer", "orderType", "plan", "paymentMethod", "amountCents", "tokenGrantAmount", "platformIncomeCents", "fulfillmentStatus", "status", "createdAt"],
    usage: ["product", "metric", "usage", "costCents"],
    tokenRecords: ["user", "orderId", "changeType", "amount", "balanceAfter", "remark", "createdAt"],
    commissions: ["recordType", "id", "agentId", "orderId", "amountCents", "rate", "status"],
    commissionRecords: ["id", "receiverType", "receiverId", "commissionType", "orderId", "amountCents", "settleStatus", "status"],
    marketingWallets: ["name", "role", "level", "balanceCents", "frozenCents", "totalIncomeCents", "pendingCommission", "totalWithdrawCents", "availableToWithdraw", "status"],
    marketingAgentLevels: ["code", "name", "identity", "openMethod", "openCondition", "keepCondition", "membershipCommission", "rechargeCommission", "enterpriseCommission", "regionalRebate", "manualReview", "status"],
    marketingWalletRecords: ["agentName", "bizType", "bizId", "orderId", "amountCents", "source", "ruleId", "status", "createdAt", "reviewedAt"],
    marketingSettlementStatements: ["agentName", "period", "commissionCents", "withdrawalCents", "netPayableCents", "pendingCents", "commissionCount", "withdrawalCount", "status"],
    system: ["category", "item", "value", "secret", "status"],
    userDashboard: ["name", "model", "status", "pointCost", "createdAt"],
    userAiImage: ["id", "model", "type", "pointCost", "status", "createdAt"],
    userVideoGeneration: ["id", "model", "type", "pointCost", "status", "createdAt"],
    userPptGeneration: ["title", "slideCount", "language", "theme", "status", "createdAt"],
    userWorks: ["name", "mediaType", "model", "pointCost", "createdAt"],
    userUsage: ["taskId", "model", "metricCode", "quantity", "pointCost", "amountCents", "balanceBefore", "balanceAfter", "status", "occurredAt"],
    userMembership: ["id", "userId", "available", "frozen"],
    userOrders: ["id", "orderTypeText", "plan", "paymentMethodText", "amountCents", "rechargePoints", "status", "createdAt", "paidAt"]
  };
  const first = rows.value[0];
  const fields = preferred[store.activeModuleId] || (first ? Object.keys(first).slice(0, 8) : []);
  return first ? fields.filter((field) => field in first) : fields;
});

const metrics = computed(() => {
  const data = store.data as { metrics?: Array<{ label: string; value: unknown }>; summary?: Record<string, unknown> };
  if (store.activeModuleId === "partnerCustomers") {
    const customerRows = partnerRows();
    const activeCustomers = customerRows.filter((item) => String(item.status || "").toUpperCase() === "ACTIVE").length;
    const consumedCustomers = customerRows.filter((item) => Number(item.usageCount || 0) > 0).length;
    const totalPoints = customerRows.reduce((total, item) => total + Number(item.pointCost || 0), 0);
    const totalAmount = customerRows.reduce((total, item) => total + Number(item.consumedCents || 0), 0);
    const totalCommission = customerRows.reduce((total, item) => total + Number(item.commissionCents || 0), 0);
    return [
      { label: "绑定客户", value: String(customerRows.length) },
      { label: "活跃客户", value: String(activeCustomers) },
      { label: "消费客户", value: String(consumedCustomers) },
      { label: "累计消费", value: moneyYuan(totalAmount) },
      { label: "累计扣点", value: `${totalPoints} 点` },
      { label: "贡献佣金", value: moneyYuan(totalCommission) }
    ];
  }
  if (store.activeModuleId === "partnerUsage") {
    const usageRows = partnerRows();
    const totalPoints = usageRows.reduce((total, item) => total + Number(item.pointCost || 0), 0);
    const totalAmount = usageRows.reduce((total, item) => total + Number(item.consumedCents || item.amountCents || 0), 0);
    const totalCommission = usageRows.reduce((total, item) => total + Number(item.commissionCents || 0), 0);
    const succeededCount = usageRows.filter((item) => String(item.status || "").toUpperCase() === "SUCCEEDED").length;
    return [
      { label: "消费笔数", value: String(usageRows.length) },
      { label: "累计扣点", value: `${totalPoints} 点` },
      { label: "消费金额", value: moneyYuan(totalAmount) },
      { label: "代理佣金", value: moneyYuan(totalCommission) },
      { label: "成功率", value: usageRows.length ? `${Math.round((succeededCount / usageRows.length) * 100)}%` : "0%" }
    ];
  }
  if (store.activeModuleId === "partnerCommissions") {
    const summary = partnerData.value.summary || {};
    const rawAvailable = Number(summary.rawAvailableToWithdraw ?? summary.availableToWithdraw ?? 0);
    return [
      { label: "累计佣金", value: moneyYuan(Number(summary.totalCommission || 0)) },
      { label: "待结算佣金", value: moneyYuan(Number(summary.pendingCommission || 0)) },
      { label: "已结算佣金", value: moneyYuan(Number(summary.settledCommission || 0)) },
      { label: "审核中提现", value: moneyYuan(Number(summary.pendingWithdrawal || 0)) },
      { label: "已提现金额", value: moneyYuan(Number(summary.withdrawn || 0)) },
      { label: "可提现余额", value: moneyYuan(Math.max(0, Number(summary.availableToWithdraw || 0))) },
      { label: "超提金额", value: moneyYuan(Math.max(0, -rawAvailable)) }
    ];
  }
  if (store.activeModuleId === "operationCenterAgents") {
    const agentRows = operationCenterRows();
    const activeAgents = agentRows.filter((item) => String(item.status || "").toUpperCase() === "ACTIVE").length;
    return [
      { label: "中心代理", value: String(agentRows.length) },
      { label: "启用代理", value: String(activeAgents) },
      { label: "待处理", value: String(Math.max(0, agentRows.length - activeAgents)) }
    ];
  }
  if (store.activeModuleId === "operationCenterOrders") {
    const orderRows = operationCenterRows();
    const paidOrders = orderRows.filter((item) => ["PAID", "SUCCEEDED", "SUCCESS"].includes(String(item.status || "").toUpperCase()));
    const totalAmount = paidOrders.reduce((total, item) => total + Number(item.amountCents || 0), 0);
    return [
      { label: "归属订单", value: String(orderRows.length) },
      { label: "已支付订单", value: String(paidOrders.length) },
      { label: "成交金额", value: moneyYuan(totalAmount) }
    ];
  }
  if (store.activeModuleId === "operationCenterCommissions") {
    const commissionRows = operationCenterRows();
    const total = commissionRows.reduce((sum, item) => sum + Number(item.amountCents || 0), 0);
    const settled = commissionRows
      .filter((item) => ["SETTLED", "APPROVED"].includes(String(item.settleStatus || item.status || "").toUpperCase()))
      .reduce((sum, item) => sum + Number(item.amountCents || 0), 0);
    return [
      { label: "分润记录", value: String(commissionRows.length) },
      { label: "累计分润", value: moneyYuan(total) },
      { label: "已结算", value: moneyYuan(settled) },
      { label: "待结算", value: moneyYuan(Math.max(0, total - settled)) }
    ];
  }
  if (["userDashboard", "userAiImage"].includes(store.activeModuleId) && Array.isArray(data.metrics)) return data.metrics.map((item) => ({ label: item.label, value: String(item.value) }));
  if (store.activeModuleId === "userUsage") {
    const summary = data.summary || {};
    return [
      { label: "消费笔数", value: String(summary.records || 0) },
      { label: "累计扣点", value: `${Number(summary.totalPointCost || 0)} 点` },
      { label: "消费金额", value: moneyYuan(Number(summary.totalAmountCents || 0)) },
      { label: "成功笔数", value: String(summary.succeeded || 0) }
    ];
  }
  if (store.activeModuleId === "apiSettings") {
    return [
      { label: "上游渠道", value: String(apiChannels.value.length) },
      { label: "模型计费", value: String(apiModels.value.length) },
      { label: "API Key", value: String(apiKeys.value.length) },
      { label: "客户分组", value: String(apiGroups.value.length) }
    ];
  }
  if (Array.isArray(data.metrics)) return data.metrics.map((item) => ({ label: item.label, value: String(item.value) }));
  if (data.summary) return Object.entries(data.summary).slice(0, 6).map(([label, value]) => ({ label, value: String(value) }));
  return [{ label: "记录数", value: String(rows.value.length) }];
});

const toolbarActions = computed(() => {
  const actions: Record<string, Array<{ action: string; label: string }>> = {
    customers: [{ action: "createCustomer", label: "新建客户" }],
    userManagement: [{ action: "createCustomer", label: "新增用户" }],
    channels: [],
    orders: [{ action: "createOrder", label: "新建订单" }],
    commissions: [
      { action: "createCommission", label: "登记分润" },
      { action: "createWithdrawal", label: "申请提现" }
    ],
    apiSettings: [],
    system: [
      { action: "editSystem", label: "品牌域名" },
      { action: "configureNewAPI", label: "NewAPI 管理" },
      { action: "importApiRecommendations", label: "导入推荐平台" },
      { action: "createApiChannel", label: "新增上游" },
      { action: "createApiKey", label: "新增 API Key" }
    ]
  };
  return actions[store.activeModuleId] || [];
});

const rowActions = computed(() => {
  const actions: Record<string, Array<{ action: string; label: string }>> = {
    customers: [
      { action: "editCustomer", label: "编辑" },
      { action: "showCustomerIdentity", label: "身份" },
      { action: "showCustomerMergeRequests", label: "合并工单" },
      { action: "executeCustomerMergeRequest", label: "执行合并" },
      { action: "toggleCustomerLoginFreeze", label: "冻结/启用" },
      { action: "unlinkCustomerMobile", label: "解绑手机" },
      { action: "unlinkCustomerWechat", label: "解绑微信" },
      { action: "forceLogoutCustomer", label: "强制退出" },
      { action: "syncNewAPI", label: "同步 NewAPI" }
    ],
    userManagement: [
      { action: "editCustomer", label: "编辑" },
      { action: "showCustomerIdentity", label: "身份" },
      { action: "showCustomerMergeRequests", label: "合并工单" },
      { action: "executeCustomerMergeRequest", label: "执行合并" },
      { action: "toggleCustomerLoginFreeze", label: "冻结/启用" },
      { action: "unlinkCustomerMobile", label: "解绑手机" },
      { action: "unlinkCustomerWechat", label: "解绑微信" },
      { action: "forceLogoutCustomer", label: "强制退出" },
      { action: "syncNewAPI", label: "同步 NewAPI" }
    ],
    channels: [],
    products: [{ action: "editProduct", label: "编辑" }],
    plans: [{ action: "editPlan", label: "编辑套餐" }],
    orders: [
      { action: "markPaid", label: "标记收款" },
      { action: "renewOrder", label: "续费" }
    ],
    marketingCommissionRules: [{ action: "editCommissionRule", label: "编辑规则" }],
    partnerCustomers: [{ action: "showPartnerCustomerDetail", label: "查看明细" }],
    partnerOrders: [{ action: "showPartnerOrderDetail", label: "查看订单" }],
    partnerUsage: [{ action: "showPartnerUsageDetail", label: "查看消费" }],
    partnerCommissions: [{ action: "showPartnerSettlementDetail", label: "查看结算" }],
    commissions: [
      { action: "approveWithdrawal", label: "通过" },
      { action: "rejectWithdrawal", label: "驳回" }
    ]
  };
  return actions[store.activeModuleId] || [];
});

function systemRows(data: { apiChannels?: unknown[]; apiModels?: unknown[]; apiKeys?: unknown[]; brand?: Record<string, unknown>; apiGateway?: Record<string, unknown> }): AdminRecord[] {
  const channels = (Array.isArray(data.apiChannels) ? data.apiChannels : []).map((item) => {
    const row = item as AdminRecord;
    return {
      id: row.id,
      category: "上游渠道",
      item: String(row.name || row.id || "-"),
      value: `${row.baseUrl || "-"} / ${Array.isArray(row.models) ? row.models.join(", ") : "-"}`,
      secret: row.apiKeyConfigured ? "已配置" : "未配置",
      status: row.status || "-",
      priority: row.priority,
      baseUrl: row.baseUrl,
      models: row.models
    };
  });
  const models = (Array.isArray(data.apiModels) ? data.apiModels : []).map((item) => {
    const row = item as AdminRecord;
    return {
      id: row.id,
      category: "模型",
      item: String(row.name || row.model || row.id || "-"),
      value: `${row.model || "-"} / ${row.billingMode || "-"} / ${row.fixedQuota || row.modelRatio || "-"}`,
      secret: "-",
      status: row.status || "-"
    };
  });
  const keys = (Array.isArray(data.apiKeys) ? data.apiKeys : []).map((item) => {
    const row = item as AdminRecord;
    return {
      id: row.id,
      category: "客户 API Key",
      item: String(row.customer || row.id || "-"),
      value: `${row.prefix || "sk-***"} / ${Array.isArray(row.models) ? row.models.join(", ") : "-"}`,
      secret: "已隐藏",
      status: row.status || "-"
    };
  });
  const brandRows = data.brand ? [{ id: "brand", category: "品牌", item: "品牌域名", value: `${data.brand.name || "-"} / ${data.brand.domain || "-"}`, secret: "-", status: "ACTIVE" }] : [];
  const newapi = ((data.apiGateway || {}).newapi || {}) as AdminRecord;
  const gatewayRows = [{
    id: "newapi-management",
    category: "NewAPI 管理",
    item: "自动同步用户密钥",
    value: `${newapi.baseUrl || "未配置"} / 分组 ${newapi.defaultGroup || "生图备份"}`,
    secret: newapi.adminCookie || newapi.adminToken ? "已配置" : "未配置",
    status: newapi.enabled ? "ACTIVE" : "DISABLED"
  }];
  return [...brandRows, ...gatewayRows, ...channels, ...models, ...keys];
}
function flattenRows(items: AdminRecord[]): AdminRecord[] {
  return items.flatMap((item) => [item, ...((item.children as AdminRecord[] | undefined) || []).map((child) => ({ ...child, name: `二级 - ${child.name || child.id}` }))]);
}

function maskAdminMobile(value: unknown) {
  const mobile = String(value || "").replace(/\D/g, "").replace(/^86(?=\d{11}$)/, "");
  return /^1[3-9]\d{9}$/.test(mobile) ? `${mobile.slice(0, 3)}****${mobile.slice(7)}` : "";
}

function maskSensitiveId(value: unknown) {
  const text = String(value || "").trim();
  if (!text) return "";
  if (text.length <= 10) return `${text.slice(0, 2)}***`;
  return `${text.slice(0, 6)}...${text.slice(-4)}`;
}

function enrichUserIdentityRow(row: AdminRecord): AdminRecord {
  const wechatOpenIds = Array.isArray(row.wechatOpenIds) ? row.wechatOpenIds.map((item) => String(item || "").trim()).filter(Boolean) : [];
  const wechatLinked = wechatOpenIds.length > 0 || Boolean(String(row.wechatUnionId || "").trim());
  const methods = [
    maskAdminMobile(row.mobile) ? "短信" : "",
    wechatLinked ? "微信" : "",
    row.passwordHash ? "密码" : ""
  ].filter(Boolean);
  return {
    ...row,
    mobileMasked: maskAdminMobile(row.mobile) || "未绑定",
    loginMethods: methods.length ? methods.join("、") : "未绑定",
    wechatBinding: wechatLinked ? "已绑定" : "未绑定",
    wechatOpenIdMasked: maskSensitiveId(wechatOpenIds[0] || row.wechatUnionId) || "未绑定"
  };
}

function formatCell(value: unknown, column: string) {
  if (["sourceAgentName", "sourceInviteCode", "sourceParentAgentName", "referredBy"].includes(column) && !value) return "未归因";
  if (Array.isArray(value)) return value.join("、");
  if (typeof value === "boolean") return value ? "是" : "否";
  if (column.toLowerCase().includes("cents")) return `￥${(Number(value || 0) / 100).toFixed(2)}`;
  if (isStatusColumn(column)) return statusLabel(value);
  if (typeof value === "object" && value) return JSON.stringify(value);
  return value ?? "-";
}

const accountMergeMovedLabels: Record<string, string> = {
  pointAccounts: "点数账户",
  tokenRecords: "Token 记录",
  orders: "订单",
  channelAgents: "代理身份",
  operationCenters: "运营中心身份",
  generationTasks: "生成任务",
  assets: "作品素材",
  billingEvents: "计费流水",
  presentations: "PPT 演示文稿",
  agents: "AI 员工",
  agentCalls: "智能体调用",
  geoBrands: "GEO 品牌",
  geoTasks: "GEO 任务",
  aiState: "AI 创作状态",
  tenantOwners: "企业 Owner",
  tenantMembers: "企业成员",
  tenantMemberConflictsDisabled: "冲突企业成员已禁用",
  userRoles: "角色权限",
  userRoleContext: "当前角色上下文",
  tenantJoinRequests: "企业加入申请",
  knowledgeBases: "知识库",
  knowledgeDocuments: "知识文档",
  knowledgeDocumentVersions: "知识文档版本",
  knowledgeAgents: "知识库智能体",
  knowledgeAgentConversations: "知识库对话"
};

function formatAccountMergeMovedItems(moved: Record<string, unknown>) {
  return Object.entries(moved)
    .map(([key, value]) => ({ key, label: accountMergeMovedLabels[key] || key, count: Number(value || 0) }))
    .filter((item) => Number.isFinite(item.count) && item.count > 0);
}

function normalizeAccountMergeWarnings(value: unknown) {
  return Array.isArray(value) ? value.map((item) => String(item || "").trim()).filter(Boolean) : [];
}

function isStatusColumn(column: string) {
  return column.toLowerCase().includes("status") || column === "active";
}

function statusType(value: unknown) {
  const text = String(value).toUpperCase();
  if (["ACTIVE", "PAID", "APPROVED", "SUCCEEDED", "SUCCESS", "DONE", "true"].includes(text)) return "success";
  if (["PENDING", "CONFIGURABLE"].includes(text)) return "warning";
  if (["DISABLED", "REJECTED", "FAILED", "false"].includes(text)) return "danger";
  return "info";
}

function statusLabel(value: unknown) {
  const raw = String(value ?? "").trim();
  if (!raw) return "-";
  const text = raw.toUpperCase();
  const labels: Record<string, string> = {
    ACTIVE: "启用",
    DISABLED: "停用",
    CONFIGURABLE: "待配置",
    PENDING: "待处理",
    RUNNING: "处理中",
    QUEUED: "排队中",
    PAID: "已支付",
    PAYMENT_PENDING: "待支付",
    APPROVED: "已通过",
    REJECTED: "已驳回",
    SETTLED: "已结算",
    SUCCEEDED: "成功",
    SUCCESS: "成功",
    DONE: "已完成",
    FAILED: "失败",
    OVERDUE: "已逾期",
    DRAFT: "草稿",
    FINALIZED: "已定稿",
    TRUE: "是",
    FALSE: "否"
  };
  return labels[text] || raw;
}

function visibleRowActions(row: AdminRecord) {
  if (store.activeModuleId !== "commissions") return rowActions.value;
  if (["提现", "分润"].includes(String(row.recordType || "")) && String(row.status).toUpperCase() === "PENDING") return rowActions.value;
  return [];
}

function labelForRowAction(action: { action: string; label: string }, row: AdminRecord) {
  if (action.action === "toggleChannel") return String(row.status).toUpperCase() === "ACTIVE" ? "停用" : "启用";
  return action.label;
}

async function showPartnerCustomerDetail(row: AdminRecord) {
  const data = partnerData.value;
  const customerId = String(row.customerId || row.id || "");
  const usageEvents = partnerUsageEvents(data.usageEvents).filter((item) => String(item.userId || item.customerId || "") === customerId);
  const orders = (Array.isArray(data.orders) ? data.orders : []).filter((item) => String(item.userId || item.customerId || "") === customerId);
  const taskIds = new Set(usageEvents.map((item) => String(item.taskId || "")).filter(Boolean));
  const commissions = (Array.isArray(data.commissions) ? data.commissions : []).filter((item) => taskIds.has(String(item.orderId || "")));
  const totalPoints = usageEvents.reduce((total, item) => total + Number(item.pointCost || 0), 0);
  const totalConsumedCents = usageEvents.reduce((total, item) => total + Number(item.amountCents || 0), 0);
  const totalCommissionCents = commissions.reduce((total, item) => total + Number(item.amountCents || 0), 0);
  const latestUsage = usageEvents[0] || {};
  const usedPointTotal = totalPoints + Number(row.pointsAvailable || 0);
  const usedPercent = usedPointTotal > 0 ? Math.min(100, Math.round((totalPoints / usedPointTotal) * 100)) : 0;
  const metrics = [
    { label: "剩余点数", value: String(row.pointsAvailable ?? "-"), hint: "套餐可用余额" },
    { label: "累计扣点", value: `${totalPoints} 点`, hint: `${usageEvents.length} 笔消费` },
    { label: "累计消费", value: moneyYuan(totalConsumedCents), hint: "计费中心口径" },
    { label: "产生佣金", value: moneyYuan(totalCommissionCents), hint: "代理商分润" },
    { label: "客户价值", value: String(row.customerValue || "-"), hint: "按消费贡献判断" },
    { label: "平均扣点", value: `${Number(row.averagePointCost || 0)} 点`, hint: "单次任务均值" }
  ];
  const profileItems = [
    { label: "客户 ID", value: customerId || "-" },
    { label: "登录邮箱", value: row.email || "-" },
    { label: "来源代理", value: row.referredBy || "-" },
    { label: "最近任务", value: latestUsage.taskId || "-" },
    { label: "最近模型", value: latestUsage.model || "-" },
    { label: "最近消费时间", value: latestUsage.occurredAt || "-" }
  ];
  const planItems = [
    { label: "当前套餐", value: row.plan || "-" },
    { label: "账号状态", value: statusLabel(row.status) },
    { label: "可调用模型", value: latestUsage.model || "gpt-image-2" },
    { label: "扣点规则", value: usageEvents.length ? `${Number(latestUsage.pointCost || 0)} 点 / 次` : "暂无消费" }
  ];
  await ElMessageBox({
    title: "客户明细",
    message: h("div", { class: "partner-customer-detail" }, [
      h("header", { class: "partner-customer-detail-hero" }, [
        h("div", { class: "partner-customer-detail-avatar" }, String(row.name || row.email || "客").slice(0, 1)),
        h("div", { class: "partner-customer-detail-identity" }, [
          h("strong", null, String(row.name || "-")),
          h("span", null, String(row.email || "-")),
          h("div", { class: "partner-customer-detail-tags" }, [
            h("em", null, String(row.plan || "未配置套餐")),
            h("em", { class: "is-success" }, statusLabel(row.status || "ACTIVE")),
            h("em", null, `归属代理 ${row.referredBy || "-"}`)
          ])
        ]),
        h("div", { class: "partner-customer-detail-actions" }, [
          h("button", { type: "button" }, "复制客户ID"),
          h("button", { type: "button", class: "is-primary" }, "查看消费明细")
        ])
      ]),
      h("div", { class: "partner-customer-detail-metrics" }, metrics.map((item) => h("div", { class: "partner-customer-detail-metric" }, [
        h("span", null, item.label),
        h("strong", null, item.value),
        h("small", null, item.hint)
      ]))),
      h("section", { class: "partner-customer-detail-split" }, [
        h("article", { class: "partner-customer-detail-card" }, [
          h("div", { class: "partner-customer-detail-card-title" }, "客户资料"),
          h("div", { class: "partner-customer-detail-kv" }, profileItems.map((item) => h("div", null, [
            h("span", null, item.label),
            h("strong", null, String(item.value))
          ])))
        ]),
        h("article", { class: "partner-customer-detail-card" }, [
          h("div", { class: "partner-customer-detail-card-title" }, "套餐与权益"),
          h("div", { class: "partner-customer-detail-progress" }, [
            h("div", { class: "partner-customer-detail-progress-head" }, [
              h("span", null, "点数使用进度"),
              h("strong", null, `${usedPercent}%`)
            ]),
            h("i", null, h("b", { style: { width: `${usedPercent}%` } }))
          ]),
          h("div", { class: "partner-customer-detail-kv" }, planItems.map((item) => h("div", null, [
            h("span", null, item.label),
            h("strong", null, String(item.value))
          ])))
        ])
      ]),
      h("section", { class: "partner-customer-detail-records" }, [
        h("nav", { class: "partner-customer-detail-tabs" }, [
          h("span", { class: "active" }, `消费记录 ${usageEvents.length}`),
          h("span", null, `佣金记录 ${commissions.length}`),
          h("span", null, `订单记录 ${orders.length}`)
        ]),
        h("div", { class: "partner-customer-detail-table" }, [
          h("div", { class: "partner-customer-detail-table-head" }, ["任务ID", "模型", "扣点", "消费金额", "状态", "时间"].map((item) => h("span", null, item))),
          usageEvents.length
            ? usageEvents.slice(0, 8).map((item) => h("div", { class: "partner-customer-detail-table-row" }, [
                h("span", null, String(item.taskId || item.id || "-")),
                h("span", null, String(item.model || "-")),
                h("span", null, `${Number(item.pointCost || 0)} 点`),
                h("span", null, moneyYuan(Number(item.amountCents || 0))),
                h("span", { class: "is-success" }, statusLabel(item.status)),
                h("span", null, String(item.occurredAt || "-"))
              ]))
            : h("p", { class: "partner-customer-detail-empty" }, "该客户暂无消费记录")
        ])
      ])
    ]),
    confirmButtonText: "知道了",
    customClass: "partner-customer-detail-dialog"
  });
}

async function showPartnerOrderDetail(row: AdminRecord) {
  const detailItems = [
    { label: "订单 ID", value: row.orderId || row.id || "-" },
    { label: "订单类型", value: row.orderType || "-" },
    { label: "客户", value: row.customer || row.customerId || "-" },
    { label: "模型/套餐", value: row.model || row.plan || "-" },
    { label: "消耗点数", value: row.pointCost === "-" ? "-" : `${Number(row.pointCost || 0)} 点` },
    { label: "消费金额", value: moneyYuan(Number(row.amountCents || 0)) },
    { label: "代理佣金", value: moneyYuan(Number(row.commissionCents || 0)) },
    { label: "佣金比例", value: row.commissionRate === "-" ? "-" : `${Math.round(Number(row.commissionRate || 0) * 100)}%` },
    { label: "状态", value: statusLabel(row.status) },
    { label: "创建时间", value: row.createdAt || "-" },
    { label: "扣前点数", value: row.balanceBefore ?? "-" },
    { label: "扣后点数", value: row.balanceAfter ?? "-" }
  ];
  await ElMessageBox({
    title: "订单详情",
    message: h("div", { class: "partner-order-detail" }, [
      h("header", { class: "partner-order-detail-hero" }, [
        h("div", null, [
          h("span", null, String(row.orderType || "订单")),
          h("strong", null, String(row.orderId || row.id || "-"))
        ]),
        h("em", { class: String(row.status || "").toUpperCase() === "SUCCEEDED" ? "is-success" : "is-warning" }, statusLabel(row.status))
      ]),
      h("div", { class: "partner-order-detail-metrics" }, [
        h("div", null, [h("span", null, "消费金额"), h("strong", null, moneyYuan(Number(row.amountCents || 0)))]),
        h("div", null, [h("span", null, "代理佣金"), h("strong", null, moneyYuan(Number(row.commissionCents || 0)))]),
        h("div", null, [h("span", null, "消耗点数"), h("strong", null, row.pointCost === "-" ? "-" : `${Number(row.pointCost || 0)} 点`)])
      ]),
      h("div", { class: "partner-order-detail-grid" }, detailItems.map((item) => h("div", null, [
        h("span", null, item.label),
        h("strong", null, String(item.value))
      ]))),
      row.prompt ? h("div", { class: "partner-order-detail-prompt" }, [
        h("span", null, "生成提示词"),
        h("p", null, String(row.prompt))
      ]) : null
    ]),
    confirmButtonText: "知道了",
    customClass: "partner-order-detail-dialog"
  });
}

async function showPartnerUsageDetail(row: AdminRecord) {
  const detailItems = [
    { label: "任务 ID", value: row.taskId || row.id || "-" },
    { label: "交易 ID", value: row.transactionId || "-" },
    { label: "客户", value: row.customer || row.customerId || "-" },
    { label: "客户 ID", value: row.customerId || row.userId || "-" },
    { label: "模型", value: row.model || "-" },
    { label: "计量指标", value: row.metricCode || "-" },
    { label: "消耗点数", value: `${Number(row.pointCost || 0)} 点` },
    { label: "消费金额", value: moneyYuan(Number(row.consumedCents || row.amountCents || 0)) },
    { label: "代理佣金", value: moneyYuan(Number(row.commissionCents || 0)) },
    { label: "扣前点数", value: row.balanceBefore ?? "-" },
    { label: "扣后点数", value: row.balanceAfter ?? "-" },
    { label: "发生时间", value: row.createdAt || row.occurredAt || "-" }
  ];
  await ElMessageBox({
    title: "消费详情",
    message: h("div", { class: "partner-order-detail" }, [
      h("header", { class: "partner-order-detail-hero" }, [
        h("div", null, [
          h("span", null, "生图消费任务"),
          h("strong", null, String(row.taskId || row.id || "-"))
        ]),
        h("em", { class: String(row.status || "").toUpperCase() === "SUCCEEDED" ? "is-success" : "is-warning" }, statusLabel(row.status))
      ]),
      h("div", { class: "partner-order-detail-metrics" }, [
        h("div", null, [h("span", null, "消费金额"), h("strong", null, moneyYuan(Number(row.consumedCents || row.amountCents || 0)))]),
        h("div", null, [h("span", null, "代理佣金"), h("strong", null, moneyYuan(Number(row.commissionCents || 0)))]),
        h("div", null, [h("span", null, "消耗点数"), h("strong", null, `${Number(row.pointCost || 0)} 点`)])
      ]),
      h("div", { class: "partner-order-detail-grid" }, detailItems.map((item) => h("div", null, [
        h("span", null, item.label),
        h("strong", null, String(item.value))
      ]))),
      row.prompt ? h("div", { class: "partner-order-detail-prompt" }, [
        h("span", null, "生成提示词"),
        h("p", null, String(row.prompt))
      ]) : null
    ]),
    confirmButtonText: "知道了",
    customClass: "partner-order-detail-dialog"
  });
}

async function showPartnerSettlementDetail(row: AdminRecord) {
  const isWithdrawal = row.recordType === "提现";
  const detailItems = isWithdrawal
    ? [
        { label: "提现单号", value: row.id || "-" },
        { label: "代理 ID", value: row.agentId || "-" },
        { label: "提现金额", value: moneyYuan(Number(row.amountCents || 0)) },
        { label: "状态", value: statusLabel(row.status) },
        { label: "申请时间", value: row.createdAt || "-" },
        { label: "审核时间", value: row.reviewedAt || "-" }
      ]
    : [
        { label: "佣金单号", value: row.id || "-" },
        { label: "来源", value: row.settlementSource || "-" },
        { label: "客户", value: row.customer || "-" },
        { label: "关联订单", value: row.orderId || "-" },
        { label: "模型", value: row.model || "-" },
        { label: "消耗点数", value: row.pointCost === "-" ? "-" : `${Number(row.pointCost || 0)} 点` },
        { label: "关联消费", value: moneyYuan(Number(row.relatedAmountCents || 0)) },
        { label: "佣金金额", value: moneyYuan(Number(row.amountCents || 0)) },
        { label: "佣金比例", value: row.commissionRate === "-" ? "-" : `${Math.round(Number(row.commissionRate || row.rate || 0) * 100)}%` },
        { label: "状态", value: statusLabel(row.status) },
        { label: "创建时间", value: row.createdAt || "-" }
      ];
  await ElMessageBox({
    title: isWithdrawal ? "提现详情" : "佣金详情",
    message: h("div", { class: "partner-order-detail" }, [
      h("header", { class: "partner-order-detail-hero" }, [
        h("div", null, [
          h("span", null, String(row.recordType || "结算记录")),
          h("strong", null, String(row.id || "-"))
        ]),
        h("em", { class: String(row.status || "").toUpperCase() === "APPROVED" ? "is-success" : String(row.status || "").toUpperCase() === "REJECTED" ? "is-danger" : "is-warning" }, statusLabel(row.status))
      ]),
      h("div", { class: "partner-order-detail-metrics" }, [
        h("div", null, [h("span", null, isWithdrawal ? "提现金额" : "佣金金额"), h("strong", null, moneyYuan(Number(row.amountCents || 0)))]),
        h("div", null, [h("span", null, isWithdrawal ? "关联状态" : "关联消费"), h("strong", null, isWithdrawal ? statusLabel(row.status) : moneyYuan(Number(row.relatedAmountCents || 0)))]),
        h("div", null, [h("span", null, isWithdrawal ? "审核时间" : "佣金比例"), h("strong", null, isWithdrawal ? String(row.reviewedAt || "-") : row.commissionRate === "-" ? "-" : `${Math.round(Number(row.commissionRate || row.rate || 0) * 100)}%`)])
      ]),
      h("div", { class: "partner-order-detail-grid" }, detailItems.map((item) => h("div", null, [
        h("span", null, item.label),
        h("strong", null, String(item.value))
      ])))
    ]),
    confirmButtonText: "知道了",
    customClass: "partner-order-detail-dialog"
  });
}

async function openCreateCustomerDialog() {
  const form = {
    name: "",
    email: `customer${Date.now()}@example.com`,
    status: "ACTIVE",
    planId: "plan_free",
    available: "1000"
  };
  await ElMessageBox({
    title: "新建客户",
    message: h("div", { class: "channel-dialog-form" }, [
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "客户名称"),
        h("input", {
          class: "channel-dialog-input",
          placeholder: "例如：演示客户",
          onInput: (event: Event) => {
            form.name = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "登录邮箱"),
        h("input", {
          class: "channel-dialog-input",
          value: form.email,
          placeholder: "customer@example.com",
          onInput: (event: Event) => {
            form.email = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "账号状态"),
        h("select", {
          class: "channel-dialog-input",
          value: form.status,
          onChange: (event: Event) => {
            form.status = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "ACTIVE" }, "启用"),
          h("option", { value: "DISABLED" }, "停用")
        ])
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "套餐"),
        h("select", {
          class: "channel-dialog-input",
          value: form.planId,
          onChange: (event: Event) => {
            form.planId = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "plan_free" }, "免费套餐"),
          h("option", { value: "plan_month" }, "月度套餐"),
          h("option", { value: "plan_year" }, "年度套餐")
        ])
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "初始点数"),
        h("input", {
          class: "channel-dialog-input",
          type: "number",
          min: "0",
          value: form.available,
          onInput: (event: Event) => {
            form.available = (event.target as HTMLInputElement).value;
          }
        })
      ])
    ]),
    showCancelButton: true,
    confirmButtonText: "创建客户",
    cancelButtonText: "取消",
    beforeClose: async (dialogAction, instance, done) => {
      if (dialogAction !== "confirm") {
        done();
        return;
      }
      const name = form.name.trim();
      const email = form.email.trim();
      const available = Number(form.available || 0);
      if (!name) {
        ElMessage.error("请填写客户名称");
        return;
      }
      if (!email) {
        ElMessage.error("请填写登录邮箱");
        return;
      }
      if (!Number.isFinite(available) || available < 0) {
        ElMessage.error("初始点数必须是大于等于 0 的数字");
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        await store.mutate("POST", "/admin/customers", {
          name,
          email,
          status: form.status,
          planId: form.planId,
          available
        });
        done();
        ElMessage.success("客户已创建");
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "创建客户失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}

async function forceLogoutCustomer(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  const name = String(row.name || row.email || userId);
  await ElMessageBox.confirm(`确认强制 ${name} 退出全部设备？该操作不会删除用户资料、作品或订单。`, "强制退出全部设备", {
    type: "warning",
    confirmButtonText: "强制退出",
    cancelButtonText: "取消"
  });
  await store.mutate("POST", `/admin/customers/${encodeURIComponent(userId)}/logout-all`, {});
  ElMessage.success("已强制退出全部设备");
}

function customerActionName(row: AdminRecord) {
  return String(row.name || row.email || row.id || "该用户");
}

async function showCustomerIdentity(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  const response = await adminRequest<{ item?: AdminRecord }>({ method: "GET", url: `/admin/customers/${encodeURIComponent(userId)}/identities` });
  const item = response.item || {};
  const methods = Array.isArray(item.loginMethods) ? item.loginMethods.join("、") : String(item.loginMethods || "未绑定");
  await ElMessageBox.alert(
    h("div", { class: "channel-dialog-form" }, [
      h("p", null, `用户ID：${String(item.userId || userId)}`),
      h("p", null, `账号状态：${statusLabel(item.status || row.status)}`),
      h("p", null, `手机号：${String(item.mobileMasked || "未绑定")}`),
      h("p", null, `微信：${String(item.wechatLinked ? "已绑定" : "未绑定")}`),
      h("p", null, `OpenID：${String(item.wechatOpenIdMasked || "未绑定")}`),
      h("p", null, `密码登录：${item.passwordLoginEnabled ? "可用" : "不可用"}`),
      h("p", null, `登录方式：${methods}`),
      h("p", null, "敏感身份仅脱敏展示，解绑或冻结操作会写入后台审计并强制会话失效。")
    ]),
    `${customerActionName(row)} 的登录身份`,
    { confirmButtonText: "知道了" }
  );
}

async function showCustomerMergeRequests(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  const response = await adminRequest<{ items?: AdminRecord[]; total?: number }>({
    method: "GET",
    url: `/admin/customers/${encodeURIComponent(userId)}/account-merge-requests`
  });
  const items = Array.isArray(response.items) ? response.items : [];
  const content = items.length
    ? items.map((item) => h("div", { class: "channel-dialog-form", style: "padding:8px 0;border-bottom:1px solid #edf0f5;" }, [
        h("p", null, `工单：${String(item.id || "-")} / ${statusLabel(item.status || "PENDING")}`),
        h("p", null, `账号：${String(item.primaryUserId || "-")} ↔ ${String(item.secondaryUserId || "-")}`),
        h("p", null, `手机号：${String(item.mobileMasked || "未提供")}，OpenID：${String(item.wechatOpenIdMasked || "未提供")}`),
        h("p", null, `来源：${String(item.source || "-")}，原因：${String(item.reason || "-")}`),
        h("p", null, `处理意见：${String(item.reviewComment || "暂无")}`)
      ]))
    : [h("p", null, "暂无账号冲突合并工单。")];
  await ElMessageBox.alert(
    h("div", { class: "channel-dialog-form" }, content),
    `${customerActionName(row)} 的合并工单`,
    { confirmButtonText: "知道了" }
  );
}

async function executeCustomerMergeRequest(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  const response = await adminRequest<{ items?: AdminRecord[] }>({
    method: "GET",
    url: `/admin/customers/${encodeURIComponent(userId)}/account-merge-requests`
  });
  const request = (Array.isArray(response.items) ? response.items : []).find((item) => ["PENDING", "IN_REVIEW"].includes(String(item.status || "").toUpperCase()));
  if (!request?.id) {
    ElMessage.warning("当前用户暂无待处理合并工单");
    return;
  }
  const preview = await adminRequest<{ result?: AdminRecord }>({
    method: "GET",
    url: `/admin/account-merge-requests/${encodeURIComponent(String(request.id))}/preview?targetUserId=${encodeURIComponent(userId)}`
  });
  const previewMoved = preview.result?.moved && typeof preview.result.moved === "object" ? preview.result.moved as Record<string, unknown> : {};
  const previewMovedItems = formatAccountMergeMovedItems(previewMoved);
  const previewMovedCount = previewMovedItems.reduce<number>((sum, item) => sum + item.count, 0);
  const previewWarnings = normalizeAccountMergeWarnings(preview.result?.warnings);
  const previewBlockers = normalizeAccountMergeWarnings(preview.result?.blockers);
  if (previewBlockers.length || preview.result?.executable === false) {
    await ElMessageBox.alert(
      h("div", { class: "channel-dialog-form" }, [
        h("p", null, `工单：${String(request.id)}`),
        h("p", null, `目标账号：${String(preview.result?.targetUserId || userId)}`),
        h("p", null, `来源账号：${String(preview.result?.sourceUserId || request.secondaryUserId || "-")}`),
        h("strong", { style: "color:#b91c1c;" }, "当前不能自动执行合并："),
        ...previewBlockers.map((item) => h("p", null, item || "存在需要人工专项处理的冲突"))
      ]),
      "合并预检未通过",
      { confirmButtonText: "知道了", type: "warning" }
    );
    return;
  }
  await ElMessageBox.confirm(
    h("div", { class: "channel-dialog-form" }, [
      h("p", null, `确认将工单 ${String(request.id)} 合并到 ${customerActionName(row)}？`),
      h("p", null, `预计迁移资源：${previewMovedCount} 项`),
      previewMovedItems.length
        ? h("div", null, previewMovedItems.map((item) => h("p", null, `${item.label}：${item.count}`)))
        : h("p", null, "预检未发现可迁移资源。"),
      previewWarnings.length
        ? h("div", { style: "margin-top:8px;color:#b45309;" }, [
            h("strong", null, "预检提示："),
            ...previewWarnings.map((item) => h("p", null, item))
          ])
        : h("p", null, "执行后来源账号会被标记为已合并，并强制退出来源和目标账号全部设备。")
    ]),
    "执行账号合并",
    {
      type: "warning",
      confirmButtonText: "确认合并",
      cancelButtonText: "取消"
    }
  );
  const result = await adminRequest<{ result?: AdminRecord }>({
    method: "POST",
    url: `/admin/account-merge-requests/${encodeURIComponent(String(request.id))}/execute`,
    data: { targetUserId: userId, confirm: true, reviewComment: "后台人工确认合并" }
  });
  const moved = result.result?.moved && typeof result.result.moved === "object" ? result.result.moved as Record<string, unknown> : {};
  const movedItems = formatAccountMergeMovedItems(moved);
  const movedCount = movedItems.reduce<number>((sum, item) => sum + item.count, 0);
  const warnings = normalizeAccountMergeWarnings(result.result?.warnings);
  await store.loadActiveModule();
  await ElMessageBox.alert(
    h("div", { class: "channel-dialog-form" }, [
      h("p", null, `目标账号：${String(result.result?.targetUserId || userId)}`),
      h("p", null, `来源账号：${String(result.result?.sourceUserId || request.secondaryUserId || "-")}`),
      h("p", null, `迁移资源总数：${movedCount}`),
      movedItems.length
        ? h("div", null, movedItems.map((item) => h("p", null, `${item.label}：${item.count}`)))
        : h("p", null, "本次没有需要迁移的资源。"),
      warnings.length
        ? h("div", { style: "margin-top:8px;color:#b45309;" }, [
            h("strong", null, "需要人工关注："),
            ...warnings.map((item) => h("p", null, item))
          ])
        : h("p", { style: "margin-top:8px;color:#16a34a;" }, "未返回额外风险提示。")
    ]),
    "账号合并完成",
    { confirmButtonText: "知道了", type: warnings.length ? "warning" : "success" }
  );
  ElMessage.success(`账号合并完成，迁移资源 ${movedCount} 项`);
}

async function unlinkCustomerMobile(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  await ElMessageBox.confirm(`确认解绑 ${customerActionName(row)} 的手机号？解绑后会强制退出该账号全部设备。`, "解绑手机号", {
    type: "warning",
    confirmButtonText: "解绑手机号",
    cancelButtonText: "取消"
  });
  await store.mutate("POST", `/admin/customers/${encodeURIComponent(userId)}/identities/mobile/unlink`, { reason: "admin_unlink_mobile" });
  ElMessage.success("手机号已解绑");
}

async function unlinkCustomerWechat(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  await ElMessageBox.confirm(`确认解绑 ${customerActionName(row)} 的微信小程序身份？解绑后会强制退出该账号全部设备。`, "解绑微信", {
    type: "warning",
    confirmButtonText: "解绑微信",
    cancelButtonText: "取消"
  });
  await store.mutate("POST", `/admin/customers/${encodeURIComponent(userId)}/identities/wechat-mini-program/unlink`, { reason: "admin_unlink_wechat" });
  ElMessage.success("微信身份已解绑");
}

async function toggleCustomerLoginFreeze(row: AdminRecord) {
  const userId = String(row.id || "").trim();
  if (!userId) throw new Error("请选择用户");
  const active = String(row.status || "").toUpperCase() === "ACTIVE";
  const action = active ? "freeze-login" : "unfreeze-login";
  await ElMessageBox.confirm(
    active
      ? `确认冻结 ${customerActionName(row)} 的登录？冻结后会强制退出全部设备。`
      : `确认恢复 ${customerActionName(row)} 的登录？`,
    active ? "冻结登录" : "恢复登录",
    {
      type: active ? "warning" : "info",
      confirmButtonText: active ? "冻结登录" : "恢复登录",
      cancelButtonText: "取消"
    }
  );
  await store.mutate("POST", `/admin/customers/${encodeURIComponent(userId)}/${action}`, { reason: active ? "admin_freeze_login" : "admin_unfreeze_login" });
  ElMessage.success(active ? "登录已冻结" : "登录已恢复");
}

async function syncCustomerNewAPI(row: AdminRecord, overrides: AdminRecord = {}) {
  if (!row.id) throw new Error("请选择客户");
  const payload = {
    channelId: String(overrides.channelId || row.modelChannelId || ""),
    groupName: String(overrides.groupName || row.modelGroup || "生图备份"),
    models: String(overrides.models || row.modelModels || "gpt-image-2"),
    quotaLimit: Number(overrides.quotaLimit || row.modelQuotaLimit || row.pointsAvailable || 100000)
  };
  try {
    const response = await adminRequest<{ item?: AdminRecord }>({
      method: "POST",
      url: `/admin/customers/${row.id}/sync-newapi`,
      data: payload
    });
    const item = response.item || {};
    await store.loadActiveModule();
    ElMessage.success(`NewAPI 已同步：${item.channel || payload.channelId || "默认渠道"} / ${item.groupName || payload.groupName}`);
    return item;
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "NewAPI 同步失败");
    throw error;
  }
}

async function fetchNewAPIGroupOptions() {
  try {
    const response = await adminRequest<{ items?: string[]; warning?: string }>({ method: "GET", url: "/admin/newapi/groups" });
    if (response.warning) {
      ElMessage.warning(`NewAPI 分组暂不可用：${response.warning}`);
    }
    return Array.from(new Set((response.items || []).map((item) => String(item).trim()).filter(Boolean)));
  } catch (error) {
    ElMessage.warning(error instanceof Error ? `NewAPI 分组加载失败：${error.message}` : "NewAPI 分组加载失败");
    return [];
  }
}

async function openEditCustomerDialog(row: AdminRecord) {
  let routeChannelSource = apiChannels.value as AdminRecord[];
  if (routeChannelSource.length === 0) {
    try {
      const response = await adminRequest<{ items?: AdminRecord[] }>({ method: "GET", url: "/admin/api/provider-channels" });
      routeChannelSource = Array.isArray(response.items) ? response.items : [];
    } catch (error) {
      ElMessage.warning(error instanceof Error ? `可用渠道加载失败：${error.message}` : "可用渠道加载失败");
    }
  }
  const newapiGroups = await fetchNewAPIGroupOptions();
  const form = {
    name: String(row.name || ""),
    email: String(row.email || ""),
    status: String(row.status || "ACTIVE"),
    available: String(row.pointsAvailable ?? 0),
    modelChannelId: String(row.modelChannelId || ""),
    modelChannel: String(row.modelChannel || ""),
    modelGroup: String(row.modelGroup || "生图备份"),
    modelModels: String(row.modelModels || "gpt-image-2"),
    modelApiKey: "",
    modelKeyStatus: String(row.modelKeyStatus || "ACTIVE"),
    modelQuotaLimit: String(row.modelQuotaLimit || row.pointsAvailable || 100000)
  };
  const channelOptions = routeChannelSource.map((channel) => ({
    id: String(channel.id || ""),
    name: String(channel.name || channel.provider || channel.id || ""),
    status: String(channel.status || ""),
    models: Array.isArray(channel.models) ? channel.models.map((item) => String(item)).filter(Boolean) : []
  })).filter((channel) => channel.id || channel.name);
  const matchedChannel = channelOptions.find((channel) => channel.id === form.modelChannelId)
    || channelOptions.find((channel) => channel.name === form.modelChannel);
  if (matchedChannel) {
    form.modelChannelId = matchedChannel.id;
    form.modelChannel = matchedChannel.name;
    if (!form.modelModels.trim() && matchedChannel.models.length > 0) {
      form.modelModels = matchedChannel.models.join(",");
    }
  }
  const field = (label: string, inputNode: ReturnType<typeof h>, wide = false) => h("label", { class: ["channel-dialog-field", wide ? "channel-dialog-field-wide" : ""] }, [h("span", null, label), inputNode]);
  const textInput = (value: string, onInput: (value: string) => void, placeholder = "", attrs: Record<string, unknown> = {}) => h("input", {
    class: "channel-dialog-input",
    value,
    placeholder,
    ...attrs,
    onInput: (event: Event) => onInput((event.target as HTMLInputElement).value)
  });
  await ElMessageBox({
    title: "编辑客户",
    customClass: "channel-dialog-message-box",
    message: h("div", { class: "channel-dialog-form" }, [
      field("客户名称", textInput(form.name, (value) => { form.name = value; }), true),
      field("登录邮箱", textInput(form.email, (value) => { form.email = value; }), true),
      field("账号状态", h("select", {
        class: "channel-dialog-input",
        value: form.status,
        onChange: (event: Event) => { form.status = (event.target as HTMLSelectElement).value; }
      }, [h("option", { value: "ACTIVE" }, "启用"), h("option", { value: "DISABLED" }, "停用")])),
      field("可用点数", textInput(form.available, (value) => { form.available = value; }, "", { type: "number", min: "0" })),
      h("div", { class: "channel-dialog-section channel-dialog-field-wide" }, [
        h("strong", null, "模型路由配置"),
        h("small", null, "用户端 AI 生图会优先走这里配置的渠道、分组和模型。")
      ]),
      h("div", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("button", {
          class: "channel-dialog-action",
          type: "button",
          onClick: async () => {
            try {
              const result = await syncCustomerNewAPI(row, {
                channelId: form.modelChannelId.trim(),
                groupName: form.modelGroup.trim(),
                models: form.modelModels.trim(),
                quotaLimit: Number(form.modelQuotaLimit || 0)
              });
              if (result?.keyPrefix) form.modelApiKey = "";
            } catch {
              // syncCustomerNewAPI 已经展示错误提示。
            }
          }
        }, "同步 NewAPI 并自动写回 Key")
      ]),
      field("模型渠道", h("select", {
        class: "channel-dialog-input",
        value: form.modelChannelId,
        onChange: (event: Event) => {
          const selectedId = (event.target as HTMLSelectElement).value;
          const selected = channelOptions.find((channel) => channel.id === selectedId);
          form.modelChannelId = selected?.id || "";
          form.modelChannel = selected?.name || "";
          if (selected?.models?.length) {
            form.modelModels = selected.models.join(",");
          }
        }
      }, [
        h("option", { value: "" }, "自动选择可用渠道"),
        ...channelOptions.map((channel) => h("option", { value: channel.id }, `${channel.name}${channel.status ? `（${channel.status}）` : ""}`))
      ]), true),
      field("模型渠道 ID", textInput(form.modelChannelId, (value) => { form.modelChannelId = value; }, "选择渠道后自动带出", { readonly: true })),
      field("NewAPI 用户密钥", textInput(form.modelApiKey, (value) => { form.modelApiKey = value; }, "粘贴 NewAPI 后台新增的用户 Key，不填则保留原密钥", { type: "password", autocomplete: "new-password" }), true),
      field("模型分组", newapiGroups.length ? h("select", {
        class: "channel-dialog-input",
        value: form.modelGroup,
        onChange: (event: Event) => { form.modelGroup = (event.target as HTMLSelectElement).value; }
      }, [
        ...(!newapiGroups.includes(form.modelGroup) && form.modelGroup ? [h("option", { value: form.modelGroup }, form.modelGroup)] : []),
        ...newapiGroups.map((group) => h("option", { value: group }, group))
      ]) : textInput(form.modelGroup, (value) => { form.modelGroup = value; }, "例如：生图备份")),
      field("可用模型", textInput(form.modelModels, (value) => { form.modelModels = value; }, "多个模型用逗号分隔")),
      field("Key 状态", h("select", {
        class: "channel-dialog-input",
        value: form.modelKeyStatus,
        onChange: (event: Event) => { form.modelKeyStatus = (event.target as HTMLSelectElement).value; }
      }, [h("option", { value: "ACTIVE" }, "启用"), h("option", { value: "DISABLED" }, "停用")])),
      field("路由额度", textInput(form.modelQuotaLimit, (value) => { form.modelQuotaLimit = value; }, "", { type: "number", min: "0" }))
    ]),
    showCancelButton: true,
    confirmButtonText: "保存客户",
    cancelButtonText: "取消",
    beforeClose: async (dialogAction, instance, done) => {
      if (dialogAction !== "confirm") {
        done();
        return;
      }
      const name = form.name.trim();
      const email = form.email.trim();
      const available = Number(form.available || 0);
      const modelQuotaLimit = Number(form.modelQuotaLimit || 0);
      if (!name) {
        ElMessage.error("请填写客户名称");
        return;
      }
      if (!email) {
        ElMessage.error("请填写登录邮箱");
        return;
      }
      if (!Number.isFinite(available) || available < 0) {
        ElMessage.error("可用点数必须是大于等于 0 的数字");
        return;
      }
      if (!Number.isFinite(modelQuotaLimit) || modelQuotaLimit < 0) {
        ElMessage.error("路由额度必须是大于等于 0 的数字");
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        await store.mutate("PATCH", `/admin/customers/${row.id}`, {
          name,
          email,
          status: form.status,
          available,
          modelChannelId: form.modelChannelId.trim(),
          modelChannel: form.modelChannel.trim(),
          modelGroup: form.modelGroup.trim(),
          modelModels: form.modelModels.trim(),
          modelApiKey: form.modelApiKey.trim(),
          modelKeyStatus: form.modelKeyStatus,
          modelQuotaLimit
        });
        done();
        ElMessage.success("客户已保存");
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "保存客户失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}
async function openCreateChannelDialog() {
  const form = {
    name: "",
    email: `agent${Date.now()}@example.com`,
    level: "1",
    parentId: "",
    inviteCode: "",
    status: "ACTIVE",
    available: "0"
  };
  await ElMessageBox({
    title: "新增代理商",
    message: h("div", { class: "channel-dialog-form" }, [
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "代理商名称"),
        h("input", {
          class: "channel-dialog-input",
          placeholder: "例如：华东推广员 / 城市合伙人",
          onInput: (event: Event) => {
            form.name = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "登录邮箱"),
        h("input", {
          class: "channel-dialog-input",
          value: form.email,
          placeholder: "agent@example.com",
          onInput: (event: Event) => {
            form.email = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "代理等级"),
        h("select", {
          class: "channel-dialog-input",
          value: form.level,
          onChange: (event: Event) => {
            form.level = (event.target as HTMLSelectElement).value;
          }
        }, agentLevelOptions.map((option) => h("option", { value: option.value }, option.label)))
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "上级代理 ID"),
        h("input", {
          class: "channel-dialog-input",
          placeholder: "可选，如 channel_000001",
          onInput: (event: Event) => {
            form.parentId = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "邀请码"),
        h("input", {
          class: "channel-dialog-input",
          placeholder: "不填则系统生成",
          onInput: (event: Event) => {
            form.inviteCode = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "账号状态"),
        h("select", {
          class: "channel-dialog-input",
          value: form.status,
          onChange: (event: Event) => {
            form.status = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "ACTIVE" }, "启用"),
          h("option", { value: "DISABLED" }, "停用")
        ])
      ]),

      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "初始点数"),
        h("input", {
          class: "channel-dialog-input",
          type: "number",
          min: "0",
          value: form.available,
          onInput: (event: Event) => {
            form.available = (event.target as HTMLInputElement).value;
          }
        })
      ])
    ]),
    showCancelButton: true,
    confirmButtonText: "创建代理商",
    cancelButtonText: "取消",
    beforeClose: async (dialogAction, instance, done) => {
      if (dialogAction !== "confirm") {
        done();
        return;
      }
      const name = form.name.trim();
      const email = form.email.trim();
      const level = Number(form.level);
      const available = Number(form.available || 0);
      if (!name) {
        ElMessage.error("请填写代理商名称");
        return;
      }
      if (!email) {
        ElMessage.error("请填写登录邮箱");
        return;
      }
      if (!agentLevelOptions.some((option) => Number(option.value) === level)) {
        ElMessage.error("请选择 L1-L5 代理等级");
        return;
      }
      if (!Number.isFinite(available) || available < 0) {
        ElMessage.error("初始点数必须是大于等于 0 的数字");
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        void [name, email, level, available];
        throw new Error("旧代理商创建入口已关闭，请在客户360中搜索并选择已有用户后执行身份开通");
        done();
        ElMessage.success("代理商已创建");
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "创建代理商失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}

async function openEditChannelDialog(row: AdminRecord) {
  const form = {
    name: String(row.name || ""),
    email: String(row.email || ""),
    level: String(row.level || "1"),
    parentId: String(row.parentId || ""),
    inviteCode: String(row.inviteCode || ""),
    status: String(row.status || "ACTIVE").toUpperCase(),
    available: row.available === undefined || row.available === null ? "" : String(row.available)
  };
  await ElMessageBox({
    title: "编辑代理商",
    message: h("div", { class: "channel-dialog-form" }, [
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "代理商名称"),
        h("input", {
          class: "channel-dialog-input",
          value: form.name,
          placeholder: "例如：华东推广员 / 城市合伙人",
          onInput: (event: Event) => {
            form.name = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "登录邮箱"),
        h("input", {
          class: "channel-dialog-input",
          value: form.email,
          placeholder: "agent@example.com",
          onInput: (event: Event) => {
            form.email = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "代理等级"),
        h("select", {
          class: "channel-dialog-input",
          value: form.level,
          onChange: (event: Event) => {
            form.level = (event.target as HTMLSelectElement).value;
          }
        }, agentLevelOptions.map((option) => h("option", { value: option.value }, option.label)))
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "上级代理 ID"),
        h("input", {
          class: "channel-dialog-input",
          value: form.parentId,
          placeholder: "可选，如 channel_000001",
          onInput: (event: Event) => {
            form.parentId = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "邀请码"),
        h("input", {
          class: "channel-dialog-input",
          value: form.inviteCode,
          placeholder: "不填则保留当前邀请码",
          onInput: (event: Event) => {
            form.inviteCode = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "账号状态"),
        h("select", {
          class: "channel-dialog-input",
          value: form.status,
          onChange: (event: Event) => {
            form.status = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "ACTIVE" }, "启用"),
          h("option", { value: "DISABLED" }, "停用")
        ])
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "可用点数"),
        h("input", {
          class: "channel-dialog-input",
          type: "number",
          min: "0",
          value: form.available,
          placeholder: "不填则不修改",
          onInput: (event: Event) => {
            form.available = (event.target as HTMLInputElement).value;
          }
        })
      ])
    ]),
    showCancelButton: true,
    confirmButtonText: "保存代理商",
    cancelButtonText: "取消",
    beforeClose: async (dialogAction, instance, done) => {
      if (dialogAction !== "confirm") {
        done();
        return;
      }
      const name = form.name.trim();
      const email = form.email.trim();
      const level = Number(form.level);
      const availableText = form.available.trim();
      const payload: Record<string, unknown> = {
        name,
        email,
        level,
        parentId: form.parentId.trim(),
        inviteCode: form.inviteCode.trim(),
        status: form.status
      };
      if (!name) {
        ElMessage.error("请填写代理商名称");
        return;
      }
      if (!email) {
        ElMessage.error("请填写登录邮箱");
        return;
      }
      if (!agentLevelOptions.some((option) => Number(option.value) === level)) {
        ElMessage.error("请选择 L1-L5 代理等级");
        return;
      }
      if (availableText !== "") {
        const available = Number(availableText);
        if (!Number.isFinite(available) || available < 0) {
          ElMessage.error("可用点数必须是大于等于 0 的数字");
          return;
        }
        payload.available = available;
      }
      instance.confirmButtonLoading = true;
      try {
        void payload;
        throw new Error("代理商身份、关系和状态请在客户360身份管理中调整；此处仅保留档案读取");
        done();
        ElMessage.success("代理商已保存");
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "保存代理商失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}

async function ask(label: string, value = "") {
  const result = await ElMessageBox.prompt(label, "编辑", {
    inputValue: value,
    confirmButtonText: "确定",
    cancelButtonText: "取消"
  });
  return String(result.value || "").trim();
}

async function askNumber(label: string, value = 0) {
  const text = await ask(label, String(value));
  const next = Number(text);
  if (!Number.isFinite(next)) throw new Error(`${label} 必须是数字`);
  return next;
}

async function configureNewAPIManagement() {
  const current = (((store.data as AdminRecord)?.apiGateway as AdminRecord)?.newapi || {}) as AdminRecord;
  const form = {
    enabled: Boolean(current.enabled),
    baseUrl: String(current.baseUrl || "https://code.lai1758.dpdns.org"),
    adminCookie: String(current.adminCookie || current.adminToken || ""),
    adminUserId: String(current.adminUserId || current.adminUserID || "1"),
    defaultGroup: String(current.defaultGroup || "生图备份"),
    createUserPath: String(current.createUserPath || ""),
    createTokenPath: String(current.createTokenPath || "/api/token/"),
    rechargePath: String(current.rechargePath || ""),
    timeoutSeconds: String(current.timeoutSeconds || 30)
  };
  const newapiGroups = await fetchNewAPIGroupOptions();
  type NewAPITextField = Exclude<keyof typeof form, "enabled">;
  const input = (label: string, key: NewAPITextField, placeholder = "", attrs: Record<string, unknown> = {}) => h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
    h("span", null, label),
    h("input", {
      class: "channel-dialog-input",
      value: String(form[key]),
      placeholder,
      ...attrs,
      onInput: (event: Event) => {
        (form[key] as string) = (event.target as HTMLInputElement).value;
      }
    })
  ]);
  await ElMessageBox({
    title: "NewAPI 管理配置",
    message: h("div", { class: "channel-dialog-form" }, [
      h("label", { class: "channel-dialog-field channel-dialog-field-wide channel-dialog-check" }, [
        h("input", {
          type: "checkbox",
          checked: form.enabled,
          onChange: (event: Event) => { form.enabled = (event.target as HTMLInputElement).checked; }
        }),
        h("span", null, "启用主控 SaaS 自动同步 NewAPI 用户密钥")
      ]),
      input("NewAPI 管理地址", "baseUrl", "https://code.lai1758.dpdns.org"),
      input("管理员凭证（Token/Cookie）", "adminCookie", "可填管理令牌，或 session=...; i18next=zh-CN", { type: "password", autocomplete: "new-password" }),
      input("管理用户 ID", "adminUserId", "主通道通常为 1"),
      newapiGroups.length ? h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "默认分组"),
        h("select", {
          class: "channel-dialog-input",
          value: form.defaultGroup,
          onChange: (event: Event) => { form.defaultGroup = (event.target as HTMLSelectElement).value; }
        }, [
          ...(!newapiGroups.includes(form.defaultGroup) && form.defaultGroup ? [h("option", { value: form.defaultGroup }, form.defaultGroup)] : []),
          ...newapiGroups.map((group) => h("option", { value: group }, group))
        ])
      ]) : input("默认分组", "defaultGroup", "例如：生图备份"),
      input("创建 Token 路径", "createTokenPath", "/api/token/"),
      input("创建用户路径（可选）", "createUserPath", "不同 NewAPI 版本可留空"),
      input("充值额度路径（可选）", "rechargePath", "不同 NewAPI 版本可留空"),
      input("超时秒数", "timeoutSeconds", "30", { type: "number", min: "5" })
    ]),
    showCancelButton: true,
    confirmButtonText: "保存配置",
    cancelButtonText: "取消",
    beforeClose: async (dialogAction, instance, done) => {
      if (dialogAction !== "confirm") {
        done();
        return;
      }
      const timeoutSeconds = Number(form.timeoutSeconds || 30);
      if (form.enabled && !form.baseUrl.trim()) {
        ElMessage.error("请填写 NewAPI 管理地址");
        return;
      }
      if (form.enabled && !form.adminCookie.trim()) {
        ElMessage.error("请填写管理员凭证（Token/Cookie）");
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        await store.mutate("PATCH", "/admin/system/settings", {
          apiGateway: {
            newapi: {
              enabled: form.enabled,
              baseUrl: form.baseUrl.trim(),
              adminCookie: form.adminCookie.trim(),
              adminUserId: form.adminUserId.trim() || "1",
              defaultGroup: form.defaultGroup.trim() || "生图备份",
              createUserPath: form.createUserPath.trim(),
              createTokenPath: form.createTokenPath.trim() || "/api/token/",
              rechargePath: form.rechargePath.trim(),
              timeoutSeconds: Number.isFinite(timeoutSeconds) ? timeoutSeconds : 30
            }
          }
        });
        done();
        ElMessage.success("NewAPI 管理配置已保存");
      } catch (error) {
        ElMessage.error(error instanceof Error ? error.message : "保存 NewAPI 配置失败");
      } finally {
        instance.confirmButtonLoading = false;
      }
    }
  });
}

const recommendedApiChannels: AdminRecord[] = [
  {
    name: "APIMart 生图聚合",
    baseUrl: "https://api.apimart.ai",
    protocol: "apimart",
    imageRequestMode: "openai-json",
    imageGenerationEndpoint: "/v1/images/generations",
    imageEditEndpoint: "/v1/images/edits",
    videoGenerationEndpoint: "/v1/videos/generations",
    fetchModelsPath: "/v1/models",
    apiKeyEnv: "APIMART_API_KEY",
    status: "CONFIGURABLE",
    priority: 10,
    models: ["gpt-image-2", "nano-banana-edit", "veo3.1-fast"],
    notes: "参考 Infinite-Canvas 推荐平台，适合聚合图片、视频和 LLM 模型。"
  },
  {
    name: "移动云豆包视频",
    baseUrl: "https://zhenze-huhehaote.cmecloud.cn/api/v3",
    protocol: "openai",
    imageRequestMode: "openai",
    imageGenerationEndpoint: "/v1/images/generations",
    imageEditEndpoint: "/v1/images/edits",
    videoGenerationEndpoint: "contents/generations/tasks",
    fetchModelsPath: "/models",
    apiKeyEnv: "CME_CLOUD_API_KEY",
    status: "CONFIGURABLE",
    priority: 15,
    models: ["doubao-seedance-2.0"],
    notes: "用于豆包 Seedance 2.0 视频生成，API Key 通过后台 API Key 单独保存。"
  },
  {
    name: "ModelScope",
    baseUrl: "https://api-inference.modelscope.cn/v1",
    protocol: "openai",
    imageRequestMode: "openai",
    imageGenerationEndpoint: "/v1/images/generations",
    imageEditEndpoint: "/v1/images/edits",
    fetchModelsPath: "/models",
    apiKeyEnv: "MODELSCOPE_API_KEY",
    status: "CONFIGURABLE",
    priority: 30,
    models: ["Tongyi-MAI/Z-Image-Turbo", "Qwen/Qwen-Image-2512"],
    notes: "用于免费工作流、LoRA 和国产模型补充通道。"
  },
  {
    name: "本地 ComfyUI 集群",
    baseUrl: "http://127.0.0.1:8188",
    protocol: "comfyui",
    imageRequestMode: "workflow",
    fetchModelsPath: "/api/workflows",
    comfyInstances: ["127.0.0.1:8188"],
    status: "CONFIGURABLE",
    priority: 40,
    models: ["custom-workflow"],
    notes: "用于内网工作流和私有部署，主控后台管理节点与工作流可见范围。"
  }
];

function apiChannelMutationPayload(channel: AdminRecord) {
  return {
    name: String(channel.name || "OpenAI 兼容上游"),
    baseUrl: String(channel.baseUrl || "https://example.com/v1"),
    protocol: String(channel.protocol || "openai"),
    imageRequestMode: String(channel.imageRequestMode || "openai"),
    imageGenerationEndpoint: String(channel.imageGenerationEndpoint || "/v1/images/generations"),
    imageEditEndpoint: String(channel.imageEditEndpoint || "/v1/images/edits"),
    videoGenerationEndpoint: String(channel.videoGenerationEndpoint || ""),
    fetchModelsPath: String(channel.fetchModelsPath || "/models"),
    apiKeyEnv: String(channel.apiKeyEnv || ""),
    comfyInstances: Array.isArray(channel.comfyInstances) ? channel.comfyInstances : [],
    notes: String(channel.notes || ""),
    primary: Boolean(channel.primary),
    status: String(channel.status || "CONFIGURABLE"),
    priority: Number(channel.priority || 50),
    models: Array.isArray(channel.models) ? uniqueNonEmptyStrings(channel.models.map((model) => String(model))) : ["gpt-image-2", "mock-standard"]
  };
}

function openPlanEditor(row: AdminRecord) {
  editingPlan.value = { ...row };
  planEditorOpen.value = true;
}

async function savePlanConfiguration(payload: AdminRecord) {
  const planId = String(editingPlan.value?.id || "");
  if (!planId) {
    ElMessage.error("套餐 ID 不存在");
    return;
  }
  try {
    const planPayload = payload.plan && typeof payload.plan === "object" ? payload.plan as AdminRecord : payload;
    const capabilitiesPayload = payload.capabilities && typeof payload.capabilities === "object"
      ? payload.capabilities as AdminRecord
      : { modules: [] };
    await adminRequest({ method: "PUT", url: `/admin/plans/${planId}/capabilities`, data: capabilitiesPayload });
    await store.mutate("PATCH", `/admin/plans/${planId}`, planPayload);
    planEditorOpen.value = false;
    editingPlan.value = null;
    ElMessage.success("套餐配置已保存并立即生效");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "套餐保存失败");
  }
}

async function runAction(action: string, row: AdminRecord = {}) {
  try {
    if (action === "createCustomer") {
      await openCreateCustomerDialog();
    } else if (action === "editCustomer") {
      await openEditCustomerDialog(row);
    } else if (action === "showCustomerIdentity") {
      await showCustomerIdentity(row);
      return;
    } else if (action === "showCustomerMergeRequests") {
      await showCustomerMergeRequests(row);
      return;
    } else if (action === "executeCustomerMergeRequest") {
      await executeCustomerMergeRequest(row);
      return;
    } else if (action === "toggleCustomerLoginFreeze") {
      await toggleCustomerLoginFreeze(row);
      return;
    } else if (action === "unlinkCustomerMobile") {
      await unlinkCustomerMobile(row);
      return;
    } else if (action === "unlinkCustomerWechat") {
      await unlinkCustomerWechat(row);
      return;
    } else if (action === "syncNewAPI") {
      await syncCustomerNewAPI(row);
    } else if (action === "forceLogoutCustomer") {
      await forceLogoutCustomer(row);
      return;
    } else if (action === "showPartnerCustomerDetail") {
      await showPartnerCustomerDetail(row);
    } else if (action === "showPartnerOrderDetail") {
      await showPartnerOrderDetail(row);
    } else if (action === "showPartnerUsageDetail") {
      await showPartnerUsageDetail(row);
    } else if (action === "showPartnerSettlementDetail") {
      await showPartnerSettlementDetail(row);
    } else if (action === "createChannel") {
      await openCreateChannelDialog();
    } else if (action === "editChannel") {
      await openEditChannelDialog(row);
    } else if (action === "toggleChannel") {
      ElMessage.warning("代理商冻结、恢复或终止必须通过客户360身份管理流程");
      return;
    } else if (action === "editProduct") {
      const status = await ask("产品状态", String(row.status || "ACTIVE"));
      const name = await ask("产品名称", String(row.name || ""));
      await store.mutate("PATCH", `/admin/products/${row.id}`, { name, type: row.type, status, entitlements: Array.isArray(row.entitlements) ? row.entitlements : [] });
    } else if (action === "editPlan") {
      openPlanEditor(row);
      return;
    } else if (action === "createOrder") {
      const userId = await ask("客户 userId", "user_000002");
      const planId = await ask("套餐 ID", "plan_month");
      const amountCents = await askNumber("订单金额（分）", 9900);
      await store.mutate("POST", "/admin/orders", { userId, planId, amountCents, status: "PENDING" });
    } else if (action === "markPaid") {
      await store.mutate("POST", `/admin/orders/${row.id}/mark-paid`);
    } else if (action === "renewOrder") {
      await store.mutate("POST", `/admin/orders/${row.id}/renew`);
    } else if (action === "editCommissionRule") {
      const name = await ask("规则名称", String(row.name || ""));
      const orderType = await ask("订单类型", String(row.orderType || "COMPUTE_RECHARGE"));
      const earnerRole = await ask("获佣角色", String(row.earnerRole || "AGENT"));
      const relationDepth = await askNumber("关系层级（1=直推，2=间推）", Number(row.relationDepth || 1));
      const fixedAmountCents = await askNumber("固定金额（分，0 表示按比例）", Number(row.fixedAmountCents || 0));
      const rate = await askNumber("分佣比例（0.2 表示 20%）", Number(row.rate || 0));
      const maxTotalRate = await askNumber("总比例上限（0 表示不限）", Number(row.maxTotalRate || 0));
      const status = await ask("状态（ACTIVE / DISABLED）", String(row.status || "ACTIVE"));
      await store.mutate("PATCH", `/admin/marketing/commission-rules/${row.id}`, { name, orderType, earnerRole, relationDepth, fixedAmountCents, rate, maxTotalRate, status });
    } else if (action === "createCommission") {
      const orderId = await ask("订单 ID", "order_000001");
      const agentId = await ask("代理 ID", "channel_000001");
      const amountCents = await askNumber("分润金额（分）", 1000);
      const rate = await askNumber("分润比例", 0.1);
      await store.mutate("POST", "/admin/commissions", { orderId, agentId, amountCents, rate, status: "PENDING" });
    } else if (action === "createWithdrawal") {
      const agentId = await ask("代理 ID", "channel_000001");
      const amountCents = await askNumber("提现金额（分）", 1000);
      await store.mutate("POST", "/admin/withdrawals", { agentId, amountCents });
    } else if (action === "approveWithdrawal" && String(row.status).toUpperCase() === "PENDING") {
      const resource = row.recordType === "分润" ? "commissions" : "withdrawals";
      await store.mutate("POST", `/admin/${resource}/${row.id}/approve`);
    } else if (action === "rejectWithdrawal" && String(row.status).toUpperCase() === "PENDING") {
      const resource = row.recordType === "分润" ? "commissions" : "withdrawals";
      await store.mutate("POST", `/admin/${resource}/${row.id}/reject`);
    } else if (action === "editSystem") {
      const name = await ask("品牌名称", "知启云 AI");
      const domain = await ask("绑定域名", "localhost:3100");
      await store.mutate("PATCH", "/admin/system/settings", {
        brand: { name, domain, logo: name.slice(0, 1) },
        payments: [
          { channel: "wechat", status: "CONFIGURABLE" },
          { channel: "alipay", status: "CONFIGURABLE" },
          { channel: "manual", status: "ACTIVE" }
        ],
        permissions: ["SUPER_ADMIN", "ADMIN", "FINANCE", "CHANNEL_MANAGER", "DELIVERY_MANAGER"]
      });
    } else if (action === "configureNewAPI") {
      await configureNewAPIManagement();
    } else if (action === "testApiChannel") {
      if (!row.id) throw new Error("请先选择上游渠道");
      const result = await adminRequest<{ item?: AdminRecord }>({ method: "POST", url: `/admin/api/provider-channels/${row.id}/test`, data: {} });
      const item = result.item || {};
      ElMessage.success(`连接测试通过：${item.latencyMs || "-"} ms`);
    } else if (action === "toggleApiChannel") {
      const status = String(row.status).toUpperCase() === "ACTIVE" ? "DISABLED" : "ACTIVE";
      await store.mutate("PATCH", `/admin/api/provider-channels/${row.id}`, apiChannelMutationPayload({ ...row, status }));
    } else if (action === "createApiChannel") {
      const name = await ask("上游渠道名称", "OpenAI 兼容上游");
      const baseUrl = await ask("Base URL", "https://example.com/v1");
      const protocol = await ask("协议类型（openai / apimart / modelscope / comfyui）", "openai");
      await store.mutate("POST", "/admin/api/provider-channels", apiChannelMutationPayload({ name, baseUrl, protocol, status: "CONFIGURABLE", priority: 50, models: ["gpt-image-2", "mock-standard"] }));
    } else if (action === "importApiRecommendations") {
      const existing = new Set(apiChannels.value.map((item) => String((item as AdminRecord).name || "").toLowerCase()));
      let imported = 0;
      for (const channel of recommendedApiChannels) {
        if (existing.has(String(channel.name || "").toLowerCase())) continue;
        await store.mutate("POST", "/admin/api/provider-channels", apiChannelMutationPayload(channel));
        existing.add(String(channel.name || "").toLowerCase());
        imported += 1;
      }
      ElMessage.success(imported ? `已导入 ${imported} 个推荐平台` : "推荐平台已存在");
    } else if (action === "createApiKey") {
      const customer = await ask("客户名称", "演示用户");
      await store.mutate("POST", "/admin/api/keys", { customer, status: "ACTIVE", quotaLimit: 100000, models: ["mock-standard", "gpt-image-2"] });
    }
    ElMessage.success("已保存");
  } catch (error) {
    if (error !== "cancel") ElMessage.error(error instanceof Error ? error.message : "操作取消");
  }
}

function hasAuthToken() {
  return authStore.isAuthenticated || Boolean(authStore.accessToken);
}

async function restoreAndResumeWorkspacePendingAction() {
  const pending = authStore.refreshPendingAction();
  if (!pending || !hasAuthToken()) return false;
  applyWorkspaceDraft(pending.payload || {});
  try {
    const localReferences = await readPendingReferenceImages(pending.id);
    const existingIds = new Set(aiReferenceImages.value.map((item) => item.id));
    const restoredReferences = localReferences
      .filter((item) => !existingIds.has(item.id))
      .map<AiReferenceImage>((item) => ({
        id: item.id,
        name: item.name,
        file: item.file,
        url: URL.createObjectURL(item.file),
        uploading: false,
      }));
    if (restoredReferences.length) aiReferenceImages.value = [...aiReferenceImages.value, ...restoredReferences];
    await clearPendingReferenceImages(pending.id);
  } catch {
    // IndexedDB is best-effort. The remaining form fields still restore from localStorage.
  }
  const moduleId = String(pending.payload?.moduleId || "");
  if (moduleId && allowedModuleIds.includes(moduleId)) await selectAdminModule(moduleId);
  authStore.consumePendingAction();

  try {
    if (pending.action === "generate_image" || pending.action === "generate_video") {
      const label = pending.action === "generate_video" ? "视频" : "图片";
      await ElMessageBox.confirm(
        `登录前填写的${label}创作参数已恢复。是否确认继续生成？正式生成可能消耗账户额度。`,
        "继续刚才的创作",
        { confirmButtonText: "确认生成", cancelButtonText: "保留参数，暂不生成", type: "warning" }
      );
      const generated = pending.action === "generate_video"
        ? await submitVideoGeneration()
        : await submitAiImage();
      if (generated) {
        trackWebGuestExperience("generation_success_after_login", moduleId || store.activeModuleId, { action: pending.action });
      }
      return generated;
    }
    if (pending.action === "recharge") {
      await createUserRechargeOrder();
      return true;
    }
    if (pending.action === "save_work") {
      if (pending.payload?.openMine === true) {
        worksSourceTab.value = "mine";
        await store.loadActiveModule({ preferCache: false });
        return true;
      }
      const taskIds = Array.isArray(pending.payload?.taskIds) ? pending.payload.taskIds.map(String).filter(Boolean) : [];
      if (taskIds.length) openAiFavoritePicker(taskIds);
      else ElMessage.info("登录成功，请重新选择要收藏的作品");
      return true;
    }
    if (pending.action === "download_work") {
      const taskId = String(pending.payload?.taskId || "");
      const mediaKind = String(pending.payload?.mediaKind || "");
      if (mediaKind === "video") {
        const entry = videoHistory.value.find((item) => item.id === taskId);
        if (entry) await downloadVideoHistory(entry);
        else ElMessage.info("登录成功，请重新选择要下载的视频");
      } else {
        const task = onlineImageTasks.value.find((item) => aiTaskId(item) === taskId);
        if (task && mediaKind === "image-all") await downloadAllAiTaskOutputs(task);
        else if (task) await downloadAiTask(task);
        else ElMessage.info("登录成功，请重新选择要下载的作品");
      }
      return true;
    }
    ElMessage.success("登录成功，已返回刚才的位置");
    return true;
  } catch (error) {
    if (error === "cancel" || error === "close") {
      ElMessage.info("已保留登录前填写的内容，未执行生成或扣费");
      return false;
    }
    ElMessage.error(error instanceof Error ? error.message : "恢复刚才的操作失败，已保留页面内容");
    return false;
  }
}

async function runBatchAction(action: string, batchRows: AdminRecord[]) {
  for (const row of batchRows) await runAction(action, row);
  ElMessage.success(`批量操作完成：${batchRows.length} 条记录`);
}

function authRedirectPath(response: AuthMeResponse) {
  const requested = typeof window === "undefined" ? "" : new URLSearchParams(window.location.search).get("redirect_url") || authStore.redirectUrl;
  let route = safeInternalRedirect(requested || response.defaultRoute || "", "");
  const role = String(response.user?.role || "").toUpperCase();
  const workspace = String(response.workspace || "").toLowerCase();
  if (workspace === "admin" || role === "SUPER_ADMIN") {
    return route.startsWith("/admin") ? route : "/admin/";
  }
  if (route.startsWith("/admin") && role !== "SUPER_ADMIN") route = "";
  if (route.startsWith("/agent") && workspace !== "agent" && !role.startsWith("AGENT")) route = "";
  // The authenticated PC user console has one canonical entry. The backend
  // still returns legacy /app routes for compatibility with H5/uni-app, so
  // translate only those user-console routes at the desktop boundary.
  if (route === "/app" || route.startsWith("/app/")) {
    return "/";
  }
  if (route) return route;
  if (role === "SUPER_ADMIN") return "/admin/";
  return "/";
}

function completeAuth(response: AuthMeResponse, remember = true) {
  const token = String(response.accessToken || "").trim();
  if (!token) {
    ElMessage.error("登录成功但没有返回访问令牌");
    return;
  }
  authStore.applyAuth(response as unknown as WebAuthResponse, remember);
  window.location.replace(authRedirectPath(response));
}

async function handleWebLoginAuthenticated(response: unknown, remember: boolean) {
  const authResponse = response as AuthMeResponse;
  const token = String(authResponse.accessToken || "").trim();
  if (!token) {
    ElMessage.error("登录成功但没有返回访问令牌");
    return;
  }
  const loginMetadata = authResponse as unknown as Record<string, unknown>;
  trackWebGuestExperience("login_success", "webLogin", { authMethod: String(loginMetadata.authMethod || loginMetadata.loginMethod || "web") });
  const authenticatedRole = String(authResponse.user?.role || "").toUpperCase();
  const authenticatedWorkspace = String(authResponse.workspace || "").toLowerCase();
  const isPlatformAdmin = authenticatedWorkspace === "admin" || authenticatedRole === "SUPER_ADMIN";
  const shouldResumeWorkspace = !isPlatformAdmin && isUserConsole.value && workspaceLoginOpen.value && !isAuthRoute.value;
  if (!shouldResumeWorkspace) {
    completeAuth(authResponse, remember);
    return;
  }

  authStore.applyAuth(authResponse as unknown as WebAuthResponse, remember);
  workspaceLoginOpen.value = false;
  authSessionVersion.value += 1;
  authReady.value = false;
  try {
    await loadCurrentAdmin();
    const restoredModuleId = restoreWorkspaceGuestDraft();
    if (restoredModuleId) store.activeModuleId = restoredModuleId;
    await store.loadActiveModule({ preferCache: false });
    await loadUserAccountSnapshot();
    const pendingAction = authStore.pendingAction?.action || "";
    const resumed = await restoreAndResumeWorkspacePendingAction();
    if (pendingAction) {
      trackWebGuestExperience(resumed ? "pending_action_resume_success" : "pending_action_resume_failed", store.activeModuleId, {
        action: pendingAction,
        reason: resumed ? "completed" : "not_executed"
      });
    }
    if (!resumed) ElMessage.success("登录成功，已恢复当前工作区");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "登录成功，但恢复工作区失败");
  } finally {
    authReady.value = true;
  }
}

function handleWorkspaceLoginCancelled() {
  trackWebGuestExperience("login_cancel", store.activeModuleId, {
    action: authStore.pendingAction?.action || "browse"
  });
}

async function redirectAuthenticatedUserFromAuthRoute() {
  if (!hasAuthToken()) return;
  try {
    const response = authStore.authResponse
      ? authStore.authResponse as unknown as AuthMeResponse
      : await adminRequest<AuthMeResponse>({ method: "GET", url: "/auth/me" });
    window.location.replace(authRedirectPath(response));
  } catch {
    authStore.clear("guest");
  }
}

async function submitRegister() {
  if (authSubmitting.value) return;
  if (!registerForm.value.username.trim() || !registerForm.value.email.trim() || !registerForm.value.password.trim() || !registerForm.value.confirmPassword.trim()) {
    ElMessage.warning("请填写用户名、邮箱、密码和确认密码");
    return;
  }
  if (registerForm.value.password.length < 8) {
    ElMessage.warning("密码至少 8 位");
    return;
  }
  if (registerForm.value.confirmPassword !== registerForm.value.password) {
    ElMessage.warning("两次输入的密码不一致");
    return;
  }
  if (!registerAgreementAccepted.value) {
    ElMessage.warning("请先阅读并同意用户协议和隐私政策");
    return;
  }
  authSubmitting.value = true;
  try {
    const response = await adminRequest<AuthMeResponse>({
      method: "POST",
      url: "/auth/register",
      authMode: "none",
      retryOnUnauthorized: false,
      data: {
        username: registerForm.value.username.trim(),
        email: registerForm.value.email.trim(),
        password: registerForm.value.password,
        confirmPassword: registerForm.value.confirmPassword,
        inviteCode: registerForm.value.inviteCode.trim()
      }
    });
    completeAuth(response, true);
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "注册失败");
  } finally {
    authSubmitting.value = false;
  }
}

function redirectToLogin() {
  authStore.clear("guest");
  authSessionVersion.value += 1;
  authReady.value = false;
  if (isUserConsole.value) {
    workspaceLoginOpen.value = false;
    currentAdmin.value = null;
    currentAgent.value = null;
    currentOperationCenter.value = null;
    currentPermissions.value = [];
    store.activeModuleId = "userDashboard";
    store.data = {};
    store.dataByModule = {};
    store.dataByEndpoint = {};
    store.error = "";
    store.loading = false;
    openTabs.value = modules.filter(item => defaultOpenTabIds.includes(item.id));
    window.localStorage.setItem(activeTabStorageKey, "userDashboard");
    window.history.replaceState({ auth: "guest" }, "", "/");
    authPath.value = "/";
    authReady.value = true;
    return;
  }
  const prefix = isAgentConsole.value ? "/agent" : isUserConsole.value ? "" : "/admin";
  window.location.replace(`${prefix}/login`);
}

async function loadCurrentAdmin() {
  if (!hasAuthToken()) {
    if (isUserConsole.value) {
      currentAdmin.value = null;
      currentAgent.value = null;
      currentOperationCenter.value = null;
      currentPermissions.value = [];
      if (!guestVisibleModuleIds.has(store.activeModuleId)) store.activeModuleId = "userDashboard";
      store.data = {};
      store.error = "";
      authReady.value = true;
      return true;
    }
    redirectToLogin();
    return false;
  }
  try {
    const response = authStore.authResponse
      ? authStore.authResponse as unknown as AuthMeResponse
      : await adminRequest<AuthMeResponse>({ method: "GET", url: "/auth/me" });
    authStore.applyAuth(response as unknown as WebAuthResponse, isPersistentWebSession());
    currentAdmin.value = response.user;
    currentAgent.value = response.agent || null;
    currentOperationCenter.value = response.operationCenter || null;
    currentPermissions.value = Array.isArray(response.permissions) ? response.permissions : [];
    const role = String(response.user.role || "").toUpperCase();
    if (isUserConsole.value) {
      if (!hasAgentIdentity.value && agentModuleIds.includes(store.activeModuleId)) {
        store.activeModuleId = "userDashboard";
        if (typeof window !== "undefined") {
          window.localStorage.setItem(activeTabStorageKey, "userDashboard");
          syncUserModulePath("userDashboard");
        }
      }
      if (!hasOperationCenterIdentity.value && operationCenterModuleIds.includes(store.activeModuleId)) {
        store.activeModuleId = "userDashboard";
        if (typeof window !== "undefined") {
          window.localStorage.setItem(activeTabStorageKey, "userDashboard");
          syncUserModulePath("userDashboard");
        }
      }
      authReady.value = true;
      return true;
    }
    if (isAgentConsole.value && !hasAgentIdentity.value) {
      ElMessage.error("当前账号不是代理商，请进入主控后台");
      window.location.href = "/admin/";
      return false;
    }
    const platformAdminRoles = ["SUPER_ADMIN", "ENTERPRISE_OPERATOR", "CERTIFICATION_REVIEWER", "FINANCE", "RISK_MANAGER", "CUSTOMER_SERVICE"];
    const isPlatformAdmin = role.includes("ADMIN") || platformAdminRoles.includes(role) || currentPermissions.value.some((permission) => String(permission).startsWith("enterprise:"));
    if (!isAgentConsole.value && !isUserConsole.value && !isPlatformAdmin && !role.startsWith("AGENT")) {
      window.location.href = "/";
      return false;
    }
    if (!isAgentConsole.value && !isUserConsole.value && role.startsWith("AGENT")) {
      window.location.href = "/agent/";
      return false;
    }
    authReady.value = true;
    return true;
  } catch {
    currentAdmin.value = null;
    currentAgent.value = null;
    currentOperationCenter.value = null;
    currentPermissions.value = [];
    redirectToLogin();
    return false;
  }
}

async function loadUserAccountSnapshot() {
  if (!isUserConsole.value || !hasAuthToken()) return;
  try {
    const response = await adminRequest<{ account?: AdminRecord }>({ method: "GET", url: "/points/account" });
    if (response.account && typeof response.account === "object") {
      userAccountSnapshot.value = response.account;
    }
  } catch {
    // The active page can still render; the sidebar falls back to module data until the account endpoint is available.
  }
}

async function showAccountInfo() {
  const admin = currentAdmin.value;
  await ElMessageBox.alert(
    admin ? `姓名：${admin.name}\n邮箱：${admin.email}\n角色：${admin.role}\n状态：${admin.status}` : "当前账号信息暂未加载",
    "账号信息",
    { confirmButtonText: "知道了" }
  );
}

async function changePassword() {
  let currentPassword = "";
  let newPassword = "";
  try {
    await ElMessageBox({
      title: "修改密码",
      message: h("div", { class: "password-dialog-form" }, [
        h("label", { class: "password-dialog-field" }, [
          h("span", null, "当前密码"),
          h("input", {
            class: "password-dialog-input",
            type: "password",
            autocomplete: "current-password",
            placeholder: "请输入当前密码",
            onInput: (event: Event) => {
              currentPassword = (event.target as HTMLInputElement).value;
            }
          })
        ]),
        h("label", { class: "password-dialog-field" }, [
          h("span", null, "新密码"),
          h("input", {
            class: "password-dialog-input",
            type: "password",
            autocomplete: "new-password",
            placeholder: "请输入新密码，至少 6 位",
            onInput: (event: Event) => {
              newPassword = (event.target as HTMLInputElement).value;
            }
          })
        ])
      ]),
      showCancelButton: true,
      confirmButtonText: "确定修改",
      cancelButtonText: "取消",
      beforeClose: async (action, instance, done) => {
        if (action !== "confirm") {
          done();
          return;
        }
        if (!currentPassword.trim()) {
          ElMessage.error("请输入当前密码");
          return;
        }
        if (newPassword.trim().length < 6) {
          ElMessage.error("新密码至少 6 位");
          return;
        }
        instance.confirmButtonLoading = true;
        try {
          await adminRequest({ method: "POST", url: "/auth/change-password", data: { currentPassword, newPassword } });
          done();
          ElMessage.success("密码已修改，请重新登录");
          redirectToLogin();
        } catch (error) {
          ElMessage.error(error instanceof Error ? error.message : "修改密码失败");
        } finally {
          instance.confirmButtonLoading = false;
        }
      }
    });
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "操作取消");
    }
  }
}

async function logout() {
  authReady.value = false;
  await authStore.logout();
  await clearCurrentAiImageCache().catch(() => undefined);
  redirectToLogin();
}

async function handleAccountCommand(command: string | number | object) {
  if (command === "profile") {
    await showAccountInfo();
    return;
  }
  if (command === "password") {
    await changePassword();
    return;
  }
  if (command === "logout") {
    await logout();
  }
}

onMounted(async () => {
  if (typeof window !== "undefined") {
    window.addEventListener("resize", adjustVideoPromptHeight);
  }
  await authStore.initializeAuth();
  if (!authStore.isAuthenticated && isUserConsole.value) {
    trackWebGuestExperience("guest_open_app", store.activeModuleId || "userDashboard", { route: currentWorkspaceRoute() });
    trackWebGuestExperience("guest_view_home", "userDashboard", { route: currentWorkspaceRoute() });
  }
  worksSourceTab.value = authStore.isAuthenticated ? "mine" : "official";
  if (!authStore.isAuthenticated) void loadGuestPublicCases();
  if (isAuthRoute.value) {
    await redirectAuthenticatedUserFromAuthRoute();
    return;
  }
  if (await loadCurrentAdmin()) {
    hydrateVideoHistoryFromStorage();
    const restoredGuestModuleId = restoreWorkspaceGuestDraft();
    void nextTick(adjustVideoPromptHeight);
    if (typeof window !== "undefined") {
      const routeModuleId = moduleIdFromLocationPath();
      const savedActiveTab = window.localStorage.getItem(activeTabStorageKey);
      const canUseModule = (moduleId: string) => allowedModuleIds.includes(moduleId)
        && (!isGuestUser.value || guestVisibleModuleIds.has(moduleId))
        && (!agentModuleIds.includes(moduleId) || hasAgentIdentity.value)
        && (!operationCenterModuleIds.includes(moduleId) || hasOperationCenterIdentity.value);
      const nextActiveTab = restoredGuestModuleId && canUseModule(restoredGuestModuleId)
        ? restoredGuestModuleId
        : routeModuleId && canUseModule(routeModuleId)
        ? routeModuleId
        : savedActiveTab && canUseModule(savedActiveTab) && modules.some((item) => item.id === savedActiveTab)
          ? savedActiveTab
          : "";
      if (nextActiveTab) {
        ensureOpenTab(nextActiveTab);
        store.activeModuleId = nextActiveTab;
        window.localStorage.setItem(activeTabStorageKey, nextActiveTab);
      }
    }
    await loadUserAccountSnapshot();
    if (isUserConsole.value) void loadOfficeCLIStatus();
    if (!isGuestUser.value) {
      await store.loadActiveModule();
      void refreshAiComposerClearance();
      await restoreAndResumeWorkspacePendingAction();
    }
  }
});
</script>

<style scoped>
.workspace-guest-shell {
  min-height: 100vh;
  color: #172033;
  background: radial-gradient(circle at 12% 8%, #e9efff 0, transparent 34%), radial-gradient(circle at 88% 18%, #fff1e8 0, transparent 30%), #f7f9fc;
}
.workspace-guest-nav { height: 76px; padding: 0 max(28px, 5vw); display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid rgba(79, 95, 130, .12); background: rgba(255, 255, 255, .78); backdrop-filter: blur(18px); }
.workspace-guest-brand { display: flex; align-items: center; gap: 12px; }
.workspace-guest-brand img { width: 46px; height: 46px; object-fit: contain; }
.workspace-guest-brand div { display: grid; gap: 2px; }
.workspace-guest-brand strong { font-size: 20px; }
.workspace-guest-brand span { color: #7b8499; font-size: 12px; }
.workspace-guest-nav > button, .workspace-guest-prompt button { border: 0; border-radius: 12px; padding: 11px 24px; color: #fff; background: linear-gradient(135deg, #6457f5, #4a79f8); cursor: pointer; font-weight: 700; }
.workspace-guest-main { width: min(1200px, calc(100% - 48px)); margin: 0 auto; padding: 64px 0 80px; }
.workspace-guest-hero { display: grid; grid-template-columns: minmax(0, 1.15fr) minmax(340px, .85fr); gap: 58px; align-items: center; }
.workspace-guest-kicker { display: inline-flex; padding: 7px 12px; border-radius: 999px; color: #584de7; background: #eceaff; font-size: 13px; font-weight: 700; }
.workspace-guest-hero h1 { margin: 22px 0 18px; font-size: clamp(42px, 5vw, 68px); line-height: 1.08; letter-spacing: -.045em; }
.workspace-guest-hero h1 em { color: #5e55ed; font-style: normal; }
.workspace-guest-hero > div > p { max-width: 680px; color: #667087; font-size: 17px; line-height: 1.8; }
.workspace-guest-prompt { margin-top: 28px; padding: 16px; border: 1px solid #dfe4f0; border-radius: 20px; background: #fff; box-shadow: 0 22px 60px rgba(48, 61, 102, .1); }
.workspace-guest-prompt textarea { width: 100%; min-height: 105px; padding: 4px; border: 0; outline: 0; resize: vertical; color: #232c42; font: inherit; line-height: 1.6; }
.workspace-guest-prompt > div { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.workspace-guest-prompt small { color: #8a93a8; }
.workspace-guest-preview { min-height: 390px; padding: 38px; display: flex; flex-direction: column; justify-content: center; border: 1px solid rgba(101, 87, 245, .18); border-radius: 30px; background: linear-gradient(145deg, #1d2440, #3c356d); color: #fff; box-shadow: 0 30px 80px rgba(39, 44, 88, .22); }
.workspace-guest-preview > span { color: #b9c4ff; font-size: 13px; letter-spacing: .15em; text-transform: uppercase; }
.workspace-guest-preview > strong { margin: 14px 0 30px; font-size: 32px; }
.workspace-guest-preview > div { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
.workspace-guest-preview i { padding: 15px; border: 1px solid rgba(255,255,255,.12); border-radius: 14px; background: rgba(255,255,255,.07); font-style: normal; }
.workspace-guest-capabilities { margin-top: 60px; display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; }
.workspace-guest-capabilities article { padding: 24px; border: 1px solid #e2e6ef; border-radius: 20px; background: rgba(255,255,255,.85); }
.workspace-guest-capabilities article > span { width: 42px; height: 42px; display: grid; place-items: center; border-radius: 12px; color: #fff; background: #6258ed; font-weight: 800; }
.workspace-guest-capabilities strong { margin-top: 18px; display: block; font-size: 18px; }
.workspace-guest-capabilities p { min-height: 66px; color: #717b91; line-height: 1.6; }
.workspace-guest-capabilities button { padding: 0; border: 0; color: #5d53e7; background: transparent; cursor: pointer; font-weight: 700; }
.workspace-login-overlay { position: fixed; z-index: 3000; inset: 0; display: grid; place-items: center; padding: 24px; background: rgba(19, 25, 45, .52); backdrop-filter: blur(8px); }
.workspace-auth-modal { position: relative; min-height: auto; width: min(560px, 100%); border-radius: 26px; box-shadow: 0 30px 90px rgba(18, 24, 48, .32); }
.workspace-auth-close { position: absolute; z-index: 2; top: 16px; right: 18px; width: 36px; height: 36px; border: 0; border-radius: 50%; color: #6f778b; background: #eef1f6; cursor: pointer; font-size: 24px; }
.workspace-auth-modal .admin-auth-card { width: 100%; }
.workspace-stay-guest { border: 0; background: transparent; cursor: pointer; }
@media (max-width: 900px) {
  .workspace-guest-hero { grid-template-columns: 1fr; }
  .workspace-guest-capabilities { grid-template-columns: repeat(2, 1fr); }
}

.admin-auth-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  overflow-x: hidden;
  padding: 24px;
  background:
    radial-gradient(circle at 20% 10%, rgba(105, 92, 244, 0.16), transparent 28%),
    radial-gradient(circle at 82% 80%, rgba(255, 126, 36, 0.12), transparent 30%),
    #f6f8ff;
  color: #101828;
  box-sizing: border-box;
}

.admin-auth-card {
  width: min(420px, 100%);
  display: grid;
  gap: 20px;
  padding: 28px;
  border: 1px solid #e1e7f2;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 24px 70px rgba(16, 24, 40, 0.14);
  box-sizing: border-box;
}

.admin-auth-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.admin-auth-brand img {
  width: 52px;
  height: 52px;
  border: 1px solid #e1e7f2;
  border-radius: 12px;
  background: #fff;
  object-fit: contain;
}

.admin-auth-brand div,
.admin-auth-head,
.admin-auth-form,
.admin-auth-form label {
  display: grid;
}

.admin-auth-brand div {
  gap: 3px;
}

.admin-auth-brand strong {
  color: #111827;
  font-size: 18px;
  font-weight: 950;
}

.admin-auth-brand span {
  color: #667085;
  font-size: 12px;
  font-weight: 750;
}

.admin-auth-head {
  gap: 8px;
}

.admin-auth-head h1 {
  margin: 0;
  color: #111827;
  font-size: 30px;
  line-height: 1.16;
}

.admin-auth-head p {
  margin: 0;
  color: #667085;
  font-size: 14px;
  font-weight: 650;
  line-height: 1.65;
}

.admin-auth-form {
  gap: 14px;
}

.admin-auth-form label {
  gap: 7px;
  color: #344054;
  font-size: 13px;
  font-weight: 850;
}

.admin-auth-form input {
  width: 100%;
  height: 44px;
  border: 1px solid #d0d5dd;
  border-radius: 10px;
  background: #fff;
  color: #101828;
  padding: 0 12px;
  font: inherit;
  outline: none;
  box-sizing: border-box;
}

.admin-auth-form input:focus {
  border-color: #6c5cf4;
  box-shadow: 0 0 0 3px rgba(108, 92, 244, 0.14);
}

.admin-auth-check {
  display: flex !important;
  align-items: center;
  gap: 8px !important;
}

.admin-auth-check input {
  width: 16px;
  height: 16px;
}

.admin-auth-submit,
.admin-auth-wechat {
  height: 44px;
  border: 0;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 900;
  cursor: pointer;
}

.admin-auth-submit {
  background: linear-gradient(135deg, #7464f2, #5b49e8);
  color: #fff;
}

.admin-auth-wechat {
  border: 1px solid #d0d5dd;
  background: #fff;
  color: #047857;
}

.admin-auth-submit:disabled,
.admin-auth-wechat:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.admin-auth-link {
  justify-self: center;
  color: #5b49e8;
  font-size: 13px;
  font-weight: 850;
  text-decoration: none;
}

.ai-reference-strip {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow-x: auto;
}

.ai-task-action-wrap {
  display: inline-flex;
}

.ai-task-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.35;
}

:global(.el-message.ai-playground-message) {
  top: auto !important;
  bottom: 96px;
  left: 50%;
  transform: translateX(-50%);
  max-width: min(44rem, calc(100vw - 32px));
  border-radius: 999px;
  padding: 14px 20px;
  box-shadow: 0 8px 30px rgba(15, 23, 42, 0.14);
  backdrop-filter: blur(16px);
}

:global(.el-message.ai-playground-message .el-message__content) {
  line-height: 20px;
  text-align: center;
  white-space: pre-line;
}

.ai-reference-thumb {
  position: relative;
  width: 48px;
  height: 48px;
  flex: 0 0 48px;
  overflow: hidden;
  border-radius: 8px;
}

.ai-reference-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.ai-reference-thumb strong {
  display: grid;
  place-items: center;
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #eef2f7, #f8fafc);
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.ai-reference-thumb > span,
.ai-reference-thumb > button,
.ai-reference-thumb > em {
  position: absolute;
}

.ai-reference-thumb > span {
  left: 4px;
  top: 4px;
}

.ai-reference-thumb > button {
  right: 2px;
  top: 2px;
}

.ai-reference-thumb > em {
  left: 0;
  right: 0;
  bottom: 0;
  padding: 1px 3px;
  background: rgba(15, 23, 42, 0.72);
  color: #fff;
  font-size: 10px;
  line-height: 16px;
  font-style: normal;
  text-align: center;
}

.ai-reference-thumb > em.is-error {
  background: rgba(185, 28, 28, 0.78);
}

.ai-detail-debug-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ai-detail-debug-actions button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  background: #fff;
  color: #303133;
  cursor: pointer;
}

.ai-detail-debug-actions button:disabled {
  color: #a8abb2;
  cursor: not-allowed;
}

.ai-raw-modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 2200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.46);
}

.ai-raw-modal {
  width: min(760px, 100%);
  max-height: min(720px, calc(100vh - 48px));
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 24px 80px rgba(15, 23, 42, 0.22);
}

.ai-raw-modal > header,
.ai-raw-modal > footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid #ebeef5;
}

.ai-raw-modal > footer {
  border-top: 1px solid #ebeef5;
  border-bottom: 0;
  justify-content: flex-end;
}

.ai-raw-modal h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: #1f2937;
}

.ai-raw-modal-close,
.ai-raw-modal-copy,
.ai-raw-modal > footer button {
  min-height: 32px;
  padding: 0 12px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  background: #fff;
  color: #303133;
  cursor: pointer;
}

.ai-raw-modal-body {
  overflow: auto;
  padding: 16px 18px;
}

.ai-raw-list {
  display: grid;
  gap: 10px;
  margin: 0;
}

.ai-raw-list-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
}

.ai-raw-list-row > span {
  color: #606266;
  font-size: 12px;
  white-space: nowrap;
}

.ai-raw-list-row > button {
  height: 30px;
  padding: 0 10px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  background: #fff;
  color: #303133;
  cursor: pointer;
}

.ai-raw-list-row code {
  overflow: hidden;
  padding: 8px 10px;
  border-radius: 6px;
  background: #f5f7fa;
  color: #1f2937;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-raw-modal pre {
  margin: 0;
  padding: 12px;
  overflow: auto;
  border-radius: 8px;
  background: #111827;
  color: #e5e7eb;
  font-size: 12px;
  line-height: 1.6;
}

:global(.channel-dialog-form) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  width: 100%;
  min-width: 0;
  max-width: 100%;
}

:global(.channel-dialog-message-box) {
  width: min(760px, calc(100vw - 32px)) !important;
  max-width: calc(100vw - 32px);
}

:global(.channel-dialog-message-box .el-message-box__content) {
  max-height: calc(100vh - 190px);
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 18px;
}

:global(.channel-dialog-message-box .el-message-box__message) {
  width: 100%;
}

:global(.channel-dialog-message-box .el-message-box__btns) {
  padding-top: 14px;
}

:global(.channel-dialog-field) {
  display: grid;
  gap: 6px;
  color: #303133;
  font-size: 13px;
  font-weight: 600;
}

:global(.channel-dialog-field-wide),
:global(.channel-dialog-section) {
  grid-column: 1 / -1;
}

:global(.channel-dialog-input) {
  box-sizing: border-box;
  width: 100%;
  height: 36px;
  padding: 0 11px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  outline: 0;
  color: #303133;
  font-size: 14px;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

:global(.channel-dialog-input:focus) {
  border-color: #409eff;
  box-shadow: 0 0 0 2px rgba(64, 158, 255, 0.12);
}

:global(.channel-dialog-check) {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid #dcdfe6;
  border-radius: 8px;
  background: #f8fafc;
}

:global(.channel-dialog-check input) {
  width: 16px;
  height: 16px;
}

:global(.channel-dialog-section) {
  padding: 10px 12px;
  border-radius: 8px;
  background: #f5f7fa;
  color: #606266;
  font-size: 13px;
  line-height: 1.6;
}

:global(.channel-dialog-action) {
  width: 100%;
  min-width: 0;
  height: 38px;
  padding: 0 14px;
  border: 0;
  border-radius: 6px;
  background: #409eff;
  color: #fff;
  font-size: 14px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

:global(.channel-dialog-action:hover) {
  background: #337ecc;
}

.membership-order-panel .user-order-table {
  --el-table-bg-color: transparent;
  --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: rgba(255, 255, 255, 0.08);
  --el-table-row-hover-bg-color: rgba(20, 184, 166, 0.18);
  --el-table-border-color: rgba(255, 255, 255, 0.11);
  --el-table-text-color: #e5eef7;
  --el-table-header-text-color: #f8fafc;
}

.membership-order-panel :deep(.user-order-table.el-table) {
  background: transparent;
  color: #e5eef7;
}

.membership-order-panel :deep(.user-order-table .el-table__inner-wrapper::before) {
  background-color: rgba(255, 255, 255, 0.12);
}

.membership-order-panel :deep(.user-order-table th.el-table__cell) {
  background: rgba(255, 255, 255, 0.08);
  color: #f8fafc;
}

.membership-order-panel :deep(.user-order-table tr),
.membership-order-panel :deep(.user-order-table td.el-table__cell) {
  background: transparent;
  color: #e5eef7;
}

.membership-order-panel :deep(.user-order-table .el-table__row--striped td.el-table__cell) {
  background: rgba(255, 255, 255, 0.055);
  color: #f8fafc;
}

.membership-order-panel :deep(.user-order-table .el-table__row:hover > td.el-table__cell) {
  background: rgba(20, 184, 166, 0.18);
  color: #ffffff;
}

.user-works-page,
.user-agent-center-page {
  display: grid;
  gap: 16px;
  min-height: calc(100vh - 88px);
  padding: 18px;
  background: #f6f8ff;
}

.user-works-hero,
.user-works-summary article,
.user-works-panel,
.user-agent-center-hero,
.user-agent-template-panel,
.user-agent-list-panel,
.user-agent-side-card {
  border: 1px solid #e1e7f2;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 14px 34px rgba(56, 72, 112, 0.07);
}

.user-works-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 132px;
  padding: 26px 34px;
  background:
    radial-gradient(circle at 86% 28%, rgba(105, 92, 244, 0.16), transparent 20%),
    linear-gradient(135deg, #fff, #f5f7ff);
}

.user-works-hero span {
  color: #695cf4;
  font-size: 12px;
  font-weight: 850;
  text-transform: uppercase;
}

.user-works-hero h2 {
  margin: 8px 0 10px;
  color: #172033;
  font-size: 30px;
  line-height: 1.15;
}

.user-works-hero p {
  margin: 0;
  color: #5d6b82;
  font-size: 14px;
  font-weight: 650;
}

.user-works-hero-actions {
  display: flex;
  gap: 10px;
}

.user-works-hero-actions button,
.user-works-empty button {
  height: 38px;
  border: 0;
  border-radius: 10px;
  padding: 0 16px;
  background: #ebe7ff;
  color: #5b49e8;
  font-weight: 850;
  cursor: pointer;
}

.user-works-hero-actions button:first-child,
.user-works-empty button {
  background: linear-gradient(135deg, #7464f2, #5b49e8);
  color: #fff;
}

.user-works-source-tabs {
  display: flex;
  gap: 8px;
  padding: 5px;
  width: fit-content;
  border: 1px solid #e1e7f2;
  border-radius: 12px;
  background: #fff;
}

.user-works-source-tabs button {
  min-height: 38px;
  padding: 0 16px;
  border: 0;
  border-radius: 9px;
  color: #667085;
  background: transparent;
  font-weight: 800;
  cursor: pointer;
}

.user-works-source-tabs button.active {
  color: #5b49e8;
  background: #eeebff;
}

.user-works-source-tabs span {
  margin-left: 5px;
  color: #98a2b3;
  font-size: 11px;
}

.user-works-guest-tip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 22px;
  border: 1px solid #dcd7ff;
  border-radius: 14px;
  background: linear-gradient(135deg, #f8f7ff, #fff);
}

.user-works-guest-tip div { display: grid; gap: 5px; }
.user-works-guest-tip strong { color: #26213f; }
.user-works-guest-tip span { color: #667085; font-size: 13px; }
.user-works-guest-tip button {
  min-height: 38px;
  padding: 0 16px;
  border: 0;
  border-radius: 10px;
  color: #fff;
  background: #6554e8;
  font-weight: 800;
  cursor: pointer;
}

.user-works-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.user-works-summary article {
  display: grid;
  gap: 7px;
  padding: 18px;
}

.user-works-summary span,
.user-works-summary small {
  color: #667085;
  font-size: 12px;
  font-weight: 700;
}

.user-works-summary strong {
  color: #111827;
  font-size: 26px;
  line-height: 1.1;
}

.user-works-panel {
  padding: 18px;
  overflow: hidden;
}

.user-works-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  margin-bottom: 16px;
}

.user-works-tabs,
.user-works-tools,
.user-works-view-switch,
.user-work-actions,
.user-works-row-actions {
  display: flex;
  align-items: center;
}

.user-works-tabs {
  gap: 24px;
}

.user-works-tabs button {
  height: 34px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: #667085;
  font-weight: 850;
  cursor: pointer;
}

.user-works-tabs button.active {
  border-color: #6c5cf4;
  color: #5b49e8;
}

.user-works-tools {
  gap: 10px;
}

.user-works-tools label {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  border: 1px solid #dde5f0;
  border-radius: 9px;
  padding: 0 12px;
  background: #fff;
  color: #64748b;
}

.user-works-tools input {
  width: 220px;
  border: 0;
  outline: 0;
  color: #344054;
}

.user-works-view-switch {
  overflow: hidden;
  border: 1px solid #dde5f0;
  border-radius: 9px;
  background: #fff;
}

.user-works-view-switch button {
  height: 34px;
  border: 0;
  padding: 0 12px;
  background: transparent;
  color: #64748b;
  font-weight: 800;
  cursor: pointer;
}

.user-works-view-switch button.active {
  background: #eeeaff;
  color: #5b49e8;
}

.user-works-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}

.user-work-card {
  overflow: hidden;
  border: 1px solid #e4e9f3;
  border-radius: 12px;
  background: #fff;
  cursor: pointer;
  transition: transform 0.18s ease, box-shadow 0.18s ease, border-color 0.18s ease;
}

.user-work-card:hover {
  border-color: #bcb4ff;
  box-shadow: 0 16px 34px rgba(66, 80, 124, 0.12);
  transform: translateY(-2px);
}

.user-work-thumb {
  position: relative;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: linear-gradient(135deg, #eef3ff, #f8fafc);
}

.user-work-thumb img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.user-work-thumb > span {
  position: absolute;
  left: 10px;
  top: 10px;
  border-radius: 999px;
  padding: 4px 9px;
  background: rgba(15, 23, 42, 0.64);
  color: #fff;
  font-size: 11px;
  font-weight: 850;
}

.user-work-placeholder {
  display: grid;
  place-items: center;
  height: 100%;
  color: #5b49e8;
  font-size: 18px;
  font-weight: 900;
}

.user-work-body {
  display: grid;
  gap: 8px;
  padding: 12px 12px 10px;
}

.user-work-body strong,
.user-work-body p,
.user-work-body span,
.user-works-name-cell strong,
.user-works-name-cell small,
.user-works-table-row > span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-work-body strong {
  color: #111827;
  font-size: 13px;
}

.user-work-body p {
  margin: 0;
  color: #667085;
  font-size: 12px;
}

.user-work-body div {
  display: flex;
  gap: 6px;
  min-width: 0;
}

.user-work-body span {
  border-radius: 999px;
  padding: 3px 7px;
  background: #f2f4f8;
  color: #58677f;
  font-size: 11px;
  font-weight: 750;
}

.user-work-actions {
  gap: 6px;
  padding: 0 12px 12px;
}

.user-work-actions button,
.user-works-row-actions button {
  height: 28px;
  border: 1px solid #dfe6f2;
  border-radius: 8px;
  background: #fff;
  color: #334155;
  padding: 0 9px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.user-work-actions button:disabled,
.user-works-row-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.user-works-table {
  display: grid;
}

.user-works-table-head,
.user-works-table-row {
  display: grid;
  grid-template-columns: minmax(260px, 1.4fr) 90px 140px 116px 86px 150px 172px;
  gap: 10px;
  align-items: center;
}

.user-works-table-head {
  min-height: 38px;
  color: #667085;
  font-size: 12px;
  font-weight: 850;
}

.user-works-table-row {
  min-height: 68px;
  border-top: 1px solid #edf1f7;
  color: #111827;
  font-size: 12px;
  font-weight: 750;
}

.user-works-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.user-works-name-cell img,
.user-works-name-cell > span {
  width: 44px;
  height: 44px;
  flex: 0 0 44px;
  border-radius: 10px;
}

.user-works-name-cell img {
  object-fit: cover;
}

.user-works-name-cell > span {
  display: grid;
  place-items: center;
  background: #eeeaff;
  color: #5b49e8;
  font-weight: 900;
}

.user-works-name-cell div {
  min-width: 0;
}

.user-works-name-cell small {
  display: block;
  margin-top: 4px;
  color: #667085;
}

.user-works-row-actions {
  gap: 6px;
}

.user-works-empty {
  display: grid;
  place-items: center;
  gap: 10px;
  min-height: 260px;
  color: #667085;
  text-align: center;
}

.user-works-empty strong {
  color: #111827;
  font-size: 18px;
}

.user-agent-center-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  align-items: center;
  min-height: 150px;
  overflow: hidden;
  padding: 28px 36px;
  background: linear-gradient(135deg, #fff, #f5f7ff);
}

.user-agent-center-hero.is-officecli {
  background:
    radial-gradient(circle at 82% 24%, rgba(255, 122, 26, 0.12), transparent 24%),
    linear-gradient(135deg, #fff, #f6f7ff);
}

.user-agent-center-hero span {
  color: #695cf4;
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
}

.user-agent-center-hero h2 {
  margin: 8px 0 10px;
  color: #5b49e8;
  font-size: 32px;
  line-height: 1.15;
}

.user-agent-center-hero p {
  margin: 0;
  color: #58677f;
  font-size: 14px;
  font-weight: 600;
}

.user-agent-hero-robot {
  position: relative;
  height: 112px;
}

.user-agent-hero-robot i,
.user-agent-hero-robot b {
  position: absolute;
  left: 88px;
  top: 20px;
  width: 132px;
  height: 54px;
  border: 1px solid rgba(105, 92, 244, 0.42);
  border-radius: 50%;
  transform: rotate(-12deg);
}

.user-agent-hero-robot b {
  transform: rotate(16deg);
}

.user-agent-hero-robot::before {
  content: "";
  position: absolute;
  left: 126px;
  top: 28px;
  width: 78px;
  height: 78px;
  border-radius: 28px;
  background: linear-gradient(135deg, #b5a8ff, #6f5cf0);
  box-shadow: 0 18px 38px rgba(95, 76, 222, 0.24);
}

.user-agent-hero-robot::after {
  content: "";
  position: absolute;
  left: 150px;
  top: 50px;
  width: 32px;
  height: 26px;
  border-radius: 9px;
  background: #27245d;
  box-shadow: 10px 9px 0 -7px #dbe3ff, 22px 9px 0 -7px #dbe3ff;
}

.user-agent-hero-robot em,
.user-agent-hero-robot strong {
  position: absolute;
  display: grid;
  place-items: center;
  border-radius: 50%;
  color: #6650e8;
  font-size: 11px;
  font-style: normal;
  font-weight: 900;
}

.user-agent-hero-robot em {
  right: 56px;
  bottom: 22px;
  width: 36px;
  height: 36px;
  background: #7664f0;
  color: #fff;
}

.user-agent-hero-robot strong {
  right: 14px;
  top: 22px;
  width: 44px;
  height: 44px;
  background: #ded9ff;
}

.user-agent-template-panel {
  padding: 18px;
}

.user-agent-template-panel header,
.user-agent-list-head,
.user-agent-side-card header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.user-agent-template-panel header strong,
.user-agent-side-card > strong,
.user-agent-side-card header strong {
  color: #111827;
  font-size: 16px;
  font-weight: 800;
}

.user-agent-template-panel header button,
.user-agent-side-card header button {
  border: 0;
  background: transparent;
  color: #5f4ee7;
  font-weight: 700;
  cursor: pointer;
}

.user-agent-template-grid {
  display: grid;
  grid-template-columns: repeat(8, minmax(92px, 1fr));
  gap: 10px;
  margin-top: 18px;
}

.user-agent-template-card {
  display: grid;
  justify-items: center;
  gap: 9px;
  min-height: 168px;
  padding: 16px 10px 12px;
  border: 1px solid #e4e9f3;
  border-radius: 10px;
  text-align: center;
}

.user-agent-template-card.is-featured {
  border-color: #ffb06a;
  background: linear-gradient(180deg, #fff, #fff8f1);
}

.user-agent-template-icon,
.user-agent-avatar {
  display: grid;
  place-items: center;
  color: #fff;
  font-weight: 900;
}

.user-agent-template-icon {
  width: 46px;
  height: 46px;
  border-radius: 16px;
  font-size: 12px;
}

.user-agent-template-card strong {
  color: #111827;
  font-size: 13px;
  line-height: 1.3;
}

.user-agent-template-card p {
  margin: 0;
  min-height: 34px;
  color: #6b778d;
  font-size: 11px;
  line-height: 1.5;
}

.user-agent-template-card button,
.user-agent-list-tools > button {
  border: 0;
  border-radius: 8px;
  background: #eeeaff;
  color: #5f4ee7;
  font-weight: 800;
  cursor: pointer;
}

.user-agent-template-card button {
  width: 66px;
  height: 28px;
}

.user-agent-template-icon.purple,
.user-agent-avatar.purple {
  background: linear-gradient(135deg, #a994ff, #695cf4);
}

.user-agent-template-icon.green,
.user-agent-avatar.green {
  background: linear-gradient(135deg, #60d394, #1fa463);
}

.user-agent-template-icon.orange,
.user-agent-avatar.orange {
  background: linear-gradient(135deg, #ffb36f, #ff7a1a);
}

.user-agent-template-icon.blue,
.user-agent-avatar.blue {
  background: linear-gradient(135deg, #79b8ff, #3978f4);
}

.user-agent-center-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 292px;
  gap: 16px;
  align-items: start;
}

.user-agent-list-panel {
  min-width: 0;
  padding: 14px 18px;
  overflow: hidden;
}

.user-agent-tabs {
  display: flex;
  gap: 28px;
}

.user-agent-tabs button {
  height: 34px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: #667085;
  font-weight: 800;
  cursor: pointer;
}

.user-agent-tabs button.active {
  border-color: #6c5cf4;
  color: #5f4ee7;
}

.user-agent-list-tools {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user-agent-list-tools label,
.user-agent-list-tools select {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  border: 1px solid #dde5f0;
  border-radius: 9px;
  background: #fff;
  color: #64748b;
  padding: 0 11px;
}

.user-agent-list-tools input {
  width: 170px;
  border: 0;
  outline: 0;
  color: #344054;
}

.user-agent-list-tools select {
  min-width: 118px;
}

.user-agent-list-tools > button {
  height: 36px;
  padding: 0 16px;
  background: linear-gradient(135deg, #7464f2, #5b49e8);
  color: #fff;
}

.user-agent-table {
  margin-top: 10px;
}

.user-agent-table-head,
.user-agent-table-row {
  display: grid;
  grid-template-columns: minmax(190px, 1fr) 76px 70px 104px 104px 82px 126px 100px;
  gap: 8px;
  align-items: center;
}

.user-agent-table-head {
  height: 38px;
  color: #6b778d;
  font-size: 12px;
  font-weight: 800;
}

.user-agent-table-row {
  min-height: 58px;
  border-top: 1px solid #edf1f7;
  color: #101828;
  font-size: 12px;
  font-weight: 700;
}

.user-agent-table-row.is-officecli {
  background: #fff7ed;
}

.user-agent-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.user-agent-avatar {
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: 11px;
  font-size: 11px;
}

.user-agent-name-cell div,
.user-agent-name-cell strong,
.user-agent-name-cell small,
.user-agent-table-row > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-agent-name-cell small {
  display: block;
  margin-top: 3px;
  color: #7a879b;
  font-size: 11px;
  font-weight: 600;
}

.user-agent-pill,
.user-agent-status {
  width: fit-content;
  max-width: 100%;
  border-radius: 7px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 800;
}

.user-agent-pill.purple {
  background: #ede9ff;
  color: #5f4ee7;
}

.user-agent-pill.green,
.user-agent-status {
  background: #dcfce7;
  color: #15803d;
}

.user-agent-pill.orange {
  background: #ffead7;
  color: #e05718;
}

.user-agent-pill.blue {
  background: #e1efff;
  color: #2563eb;
}

.user-agent-row-actions {
  display: flex;
  gap: 4px;
}

.user-agent-row-actions button {
  width: 22px;
  height: 22px;
  border: 1px solid #e1e7f2;
  border-radius: 7px;
  background: #fff;
  color: #344054;
  font-size: 10px;
  cursor: pointer;
}

.officecli-status-badge {
  flex: 0 0 auto;
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  font-style: normal;
  font-weight: 900;
}

.officecli-status-badge.ready {
  background: #dcfce7;
  color: #15803d;
}

.officecli-status-badge.pending {
  background: #fff4df;
  color: #c2410c;
}

.officecli-status-badge.idle {
  background: #edf2f7;
  color: #475569;
}

.user-agent-officecli-workbench {
  margin-top: 16px;
  padding: 18px;
  border: 1px solid #e4e9f3;
  border-radius: 10px;
  background: #ffffff;
}

.user-agent-officecli-workbench header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.user-agent-officecli-workbench header div {
  display: grid;
  gap: 4px;
}

.user-agent-officecli-workbench header span {
  color: #5b49e8;
  font-size: 12px;
  font-weight: 900;
}

.user-agent-officecli-workbench header strong {
  color: #111827;
  font-size: 17px;
  font-weight: 900;
}

.user-agent-officecli-workbench header > button,
.officecli-result-card button {
  height: 36px;
  border: 0;
  border-radius: 8px;
  background: linear-gradient(135deg, #7464f2, #5b49e8);
  color: #ffffff;
  padding: 0 16px;
  font-size: 13px;
  font-weight: 900;
  cursor: pointer;
}

.user-agent-officecli-workbench header > button:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.officecli-workbench-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 280px;
  gap: 14px;
  margin-top: 16px;
}

.officecli-form-column {
  display: grid;
  gap: 12px;
  min-width: 0;
}

.officecli-format-switch {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.officecli-format-switch button {
  display: grid;
  gap: 4px;
  min-height: 58px;
  border: 1px solid #e1e7f2;
  border-radius: 9px;
  background: #f8fafc;
  color: #334155;
  padding: 10px;
  text-align: left;
  cursor: pointer;
}

.officecli-format-switch button.active {
  border-color: #ff9a4a;
  background: #fff7ed;
}

.officecli-format-switch b {
  color: #111827;
  font-size: 13px;
}

.officecli-format-switch span {
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.officecli-field {
  display: grid;
  gap: 7px;
}

.officecli-field span {
  color: #344054;
  font-size: 12px;
  font-weight: 900;
}

.officecli-field input,
.officecli-field textarea {
  width: 100%;
  border: 1px solid #dbe3ef;
  border-radius: 9px;
  background: #ffffff;
  color: #111827;
  padding: 10px 12px;
  font: inherit;
  outline: 0;
}

.officecli-field textarea {
  min-height: 128px;
  resize: vertical;
  line-height: 1.6;
}

.officecli-field input:focus,
.officecli-field textarea:focus {
  border-color: #8273f4;
  box-shadow: 0 0 0 3px rgba(105, 92, 244, 0.12);
}

.officecli-result-card {
  display: grid;
  align-content: start;
  gap: 10px;
  min-width: 0;
  border-radius: 10px;
  background: #f8fafc;
  padding: 16px;
}

.officecli-result-card > span {
  color: #ff7a1a;
  font-size: 12px;
  font-weight: 900;
}

.officecli-result-card strong {
  overflow-wrap: anywhere;
  color: #111827;
  font-size: 15px;
  font-weight: 900;
}

.officecli-result-card small {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.5;
}

.officecli-result-card button {
  width: fit-content;
  background: #ff7a1a;
}

.user-agent-side-panel {
  display: grid;
  gap: 14px;
}

.user-agent-side-card {
  padding: 18px;
}

.user-agent-metric-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-top: 16px;
  border-top: 1px solid #edf1f7;
  border-left: 1px solid #edf1f7;
}

.user-agent-metric-grid div {
  display: grid;
  gap: 6px;
  min-height: 82px;
  padding: 13px 12px;
  border-right: 1px solid #edf1f7;
  border-bottom: 1px solid #edf1f7;
}

.user-agent-metric-grid span {
  color: #64748b;
  font-size: 12px;
}

.user-agent-metric-grid strong {
  color: #111827;
  font-size: 23px;
}

.user-agent-metric-grid small {
  color: #059669;
  font-weight: 800;
}

.user-agent-trend {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  align-items: end;
  gap: 8px;
  height: 148px;
  margin-top: 14px;
}

.user-agent-trend span {
  display: grid;
  align-items: end;
  gap: 8px;
  height: 100%;
}

.user-agent-trend i {
  display: block;
  border-radius: 8px 8px 0 0;
  background: linear-gradient(180deg, #7060f2, #8fb2ff);
}

.user-agent-trend em {
  color: #667085;
  font-size: 10px;
  font-style: normal;
  text-align: center;
}

.user-agent-ranking {
  display: grid;
  gap: 13px;
  margin: 16px 0 0;
  padding: 0;
  list-style: none;
}

.user-agent-ranking li {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  color: #334155;
  font-size: 12px;
}

.user-agent-ranking li span {
  color: #ff6b18;
  font-weight: 900;
}

.user-agent-ranking li b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-agent-ranking li em {
  font-style: normal;
}

.user-agent-shortcuts {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-top: 16px;
}

.user-agent-shortcuts button {
  display: grid;
  justify-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  color: #344054;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.user-agent-shortcuts span {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 14px;
  background: #eaf2ff;
  color: #3478f6;
  font-size: 11px;
}

@media (max-width: 1360px) {
  .user-agent-center-layout {
    grid-template-columns: 1fr;
  }

  .user-agent-side-panel {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .user-agent-template-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .officecli-workbench-body {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 960px) {
  .user-agent-center-hero,
  .user-agent-list-head {
    grid-template-columns: 1fr;
  }

  .user-agent-center-hero {
    display: block;
  }

  .user-agent-officecli-workbench header {
    align-items: flex-start;
    flex-direction: column;
  }

  .officecli-format-switch {
    grid-template-columns: 1fr;
  }

  .user-agent-hero-robot {
    margin-top: 16px;
  }

  .user-agent-list-head,
  .user-agent-list-tools {
    flex-wrap: wrap;
  }

  .user-agent-table-head {
    display: none;
  }

  .user-agent-table-row {
    grid-template-columns: 1fr 1fr;
    padding: 12px 0;
  }

  .user-agent-name-cell,
  .user-agent-row-actions {
    grid-column: 1 / -1;
  }
}

@media (max-width: 720px) {
  .user-agent-center-page {
    padding: 12px;
  }

  .user-agent-template-grid,
  .user-agent-side-panel,
  .user-agent-metric-grid {
    grid-template-columns: 1fr;
  }

  :global(.channel-dialog-form) {
    grid-template-columns: 1fr;
  }
}

.user-agent-center-page {
  min-height: calc(100vh - 56px);
  padding: 16px;
  background: #f7f8fc;
  color: #111827;
}

.user-agent-center-page,
.user-agent-center-page * {
  box-sizing: border-box;
}

.user-agent-desktop-view {
  display: block;
}

.user-agent-mobile-view {
  display: none;
}

.user-agent-center-layout {
  display: grid;
  grid-template-columns: minmax(0, 884px) 292px;
  gap: 16px;
  align-items: start;
  width: min(1192px, 100%);
  margin: 0 auto;
}

.user-agent-officecli-workspace {
  display: grid;
  gap: 16px;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  margin: 0;
}

.user-agent-workspace {
  display: grid;
  gap: 16px;
  width: 100%;
  min-width: 0;
  max-width: 100%;
}

.officecli-workspace-head {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 18px;
  align-items: center;
  min-height: 116px;
  padding: 22px 24px;
  border: 1px solid #e6e9f2;
  border-radius: 12px;
  background: linear-gradient(135deg, #ffffff 0%, #fff7ed 100%);
  box-shadow: 0 18px 42px rgba(32, 47, 86, 0.05);
}

.officecli-workspace-head > button {
  height: 34px;
  border: 1px solid #e6e9f2;
  border-radius: 8px;
  background: #ffffff;
  color: #344054;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.officecli-workspace-head div {
  min-width: 0;
}

.officecli-workspace-head span {
  display: block;
  margin-bottom: 6px;
  color: #ff6b1a;
  font-size: 12px;
  font-weight: 900;
}

.officecli-workspace-head h2 {
  margin: 0;
  color: #111827;
  font-size: 26px;
  font-weight: 900;
  line-height: 34px;
}

.officecli-workspace-head p {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 20px;
}

.user-agent-officecli-workbench.is-workspace {
  margin-top: 0;
}

.user-agent-officecli-workbench.is-workspace .officecli-workbench-body {
  grid-template-columns: minmax(0, 1fr) 320px;
}

.agent-workspace-head,
.agent-workspace-chat,
.agent-workspace-card {
  border: 1px solid #e6e9f2;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 18px 42px rgba(32, 47, 86, 0.05);
}

.agent-workspace-head {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  gap: 16px;
  align-items: center;
  min-height: 116px;
  padding: 22px 24px;
}

.agent-workspace-head > button {
  height: 34px;
  border: 1px solid #e6e9f2;
  border-radius: 8px;
  background: #ffffff;
  color: #344054;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.agent-workspace-avatar {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  color: #ffffff;
  font-size: 12px;
  font-weight: 900;
}

.agent-workspace-avatar.purple {
  background: #7466ff;
}

.agent-workspace-avatar.green {
  background: #29b978;
}

.agent-workspace-avatar.orange {
  background: #ff8438;
}

.agent-workspace-avatar.blue {
  background: #3478f6;
}

.agent-workspace-head div {
  min-width: 0;
}

.agent-workspace-head div > span {
  display: block;
  margin-bottom: 6px;
  color: #5b4bff;
  font-size: 12px;
  font-weight: 900;
}

.agent-workspace-head h2 {
  margin: 0;
  color: #111827;
  font-size: 26px;
  font-weight: 900;
  line-height: 34px;
}

.agent-workspace-head p {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 20px;
}

.agent-workspace-status {
  border-radius: 999px;
  background: #dcfce7;
  color: #15803d;
  padding: 6px 10px;
  font-size: 12px;
  font-style: normal;
  font-weight: 900;
}

.agent-workspace-status.draft {
  background: #eaf2ff;
  color: #3478f6;
}

.agent-workspace-status.disabled {
  background: #fff0e4;
  color: #ff6b1a;
}

.agent-workspace-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 16px;
  align-items: start;
}

.agent-workspace-chat {
  padding: 18px;
}

.agent-workspace-chat > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.agent-workspace-chat > header div {
  display: grid;
  gap: 4px;
}

.agent-workspace-chat > header span,
.agent-workspace-card > strong {
  color: #5b4bff;
  font-size: 12px;
  font-weight: 900;
}

.agent-workspace-chat > header strong {
  color: #111827;
  font-size: 18px;
  font-weight: 900;
}

.agent-workspace-chat > header button {
  height: 36px;
  border: 0;
  border-radius: 8px;
  background: #6658ff;
  color: #ffffff;
  padding: 0 16px;
  font-size: 13px;
  font-weight: 900;
  cursor: pointer;
}

.agent-workspace-dialog {
  display: grid;
  gap: 12px;
  min-height: 224px;
  margin-top: 16px;
  border-radius: 12px;
  background: #f8fafc;
  padding: 16px;
}

.agent-message {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 10px;
  align-items: start;
}

.agent-message span {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: #6658ff;
  color: #ffffff;
  font-size: 11px;
  font-weight: 900;
}

.agent-message.user span {
  background: #ff8438;
}

.agent-message p {
  margin: 0;
  border: 1px solid #e7ebf3;
  border-radius: 10px;
  background: #ffffff;
  color: #223047;
  padding: 10px 12px;
  font-size: 13px;
  line-height: 1.6;
}

.agent-workspace-input {
  display: grid;
  gap: 8px;
  margin-top: 14px;
}

.agent-workspace-input span {
  color: #344054;
  font-size: 12px;
  font-weight: 900;
}

.agent-workspace-input textarea {
  width: 100%;
  min-height: 124px;
  border: 1px solid #dbe3ef;
  border-radius: 9px;
  background: #ffffff;
  color: #111827;
  padding: 10px 12px;
  font: inherit;
  line-height: 1.6;
  outline: 0;
  resize: vertical;
}

.agent-workspace-input textarea:focus {
  border-color: #8273f4;
  box-shadow: 0 0 0 3px rgba(105, 92, 244, 0.12);
}

.agent-workspace-config {
  display: grid;
  gap: 16px;
}

.agent-workspace-card {
  display: grid;
  gap: 14px;
  padding: 18px;
}

.agent-workspace-meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.agent-workspace-meta-grid div {
  display: grid;
  gap: 4px;
  min-width: 0;
  border-radius: 10px;
  background: #f8fafc;
  padding: 12px;
}

.agent-workspace-meta-grid span {
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.agent-workspace-meta-grid b {
  overflow: hidden;
  color: #111827;
  font-size: 12px;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-tool-tags,
.agent-quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.agent-tool-tags span,
.agent-quick-actions button {
  border-radius: 999px;
  background: #f0edff;
  color: #5b4bff;
  padding: 7px 10px;
  font-size: 12px;
  font-weight: 800;
}

.agent-quick-actions button {
  border: 0;
  cursor: pointer;
}

.user-agent-main-column {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.user-agent-center-hero,
.user-agent-template-panel,
.user-agent-list-panel,
.user-agent-side-card {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  border: 1px solid #e6e9f2;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 0 18px 42px rgba(32, 47, 86, 0.05);
}

.user-agent-center-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 330px;
  align-items: center;
  height: 116px;
  min-height: 0;
  padding: 0 32px;
  overflow: hidden;
}

.user-agent-center-hero h2 {
  margin: 0 0 8px;
  color: #4d42d8;
  font-size: 30px;
  font-weight: 800;
  line-height: 38px;
}

.user-agent-center-hero p {
  margin: 0;
  color: #69738a;
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
}

.user-agent-hero-robot {
  position: relative;
  height: 116px;
}

.user-agent-hero-robot i,
.user-agent-hero-robot b {
  position: absolute;
  border-radius: 999px;
}

.user-agent-hero-robot i {
  left: 124px;
  top: 14px;
  width: 120px;
  height: 88px;
  background: #eeecff;
}

.user-agent-hero-robot b {
  left: 274px;
  top: 64px;
  width: 28px;
  height: 28px;
  background: #7466ff;
}

.user-agent-hero-robot::before {
  content: "";
  position: absolute;
  left: 274px;
  top: 28px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #c9d6ff;
}

.user-agent-hero-robot::after {
  display: none;
}

.user-agent-hero-robot em {
  position: absolute;
  left: 162px;
  top: 30px;
  display: grid;
  place-items: center;
  width: 48px;
  height: 36px;
  border-radius: 12px;
  background: #5649dc;
  color: #ffffff;
  font-size: 14px;
  font-style: normal;
  font-weight: 900;
}

.user-agent-template-panel {
  height: 218px;
  padding: 18px;
}

.user-agent-template-panel header,
.user-agent-list-head,
.user-agent-side-card header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.user-agent-template-panel header strong,
.user-agent-side-card > strong,
.user-agent-side-card header strong {
  color: #111827;
  font-size: 15px;
  font-weight: 800;
  line-height: 22px;
}

.user-agent-template-panel header button,
.user-agent-side-card header button {
  border: 0;
  background: transparent;
  color: #5b4bff;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.user-agent-template-grid {
  display: grid;
  grid-template-columns: repeat(8, 99px);
  gap: 8px;
  margin-top: 14px;
}

.user-agent-template-card {
  display: grid;
  justify-items: center;
  align-content: start;
  gap: 7px;
  width: 99px;
  min-height: 168px;
  height: auto;
  max-height: none;
  padding: 18px 8px 10px;
  border: 1px solid #e6e9f2;
  border-radius: 8px;
  text-align: center;
}

.user-agent-template-card.is-clickable {
  cursor: pointer;
}

.user-agent-template-card.is-clickable:focus-visible,
.user-agent-table-row.is-clickable:focus-visible {
  outline: 2px solid #ff9a4a;
  outline-offset: 2px;
}

.user-agent-template-card.is-clickable:hover {
  border-color: #ffb06a;
  box-shadow: 0 12px 30px rgba(255, 122, 26, 0.14);
}

.user-agent-template-icon,
.user-agent-avatar {
  display: grid;
  place-items: center;
  color: #ffffff;
  font-weight: 900;
}

.user-agent-template-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  font-size: 13px;
}

.user-agent-template-card strong {
  color: #111827;
  font-size: 12px;
  font-weight: 800;
  line-height: 16px;
}

.user-agent-template-card p {
  display: -webkit-box;
  min-height: 30px;
  margin: 0;
  overflow: hidden;
  color: #7a849b;
  font-size: 10px;
  line-height: 15px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.user-agent-template-card button {
  width: 58px;
  height: 22px;
  margin-top: auto;
  border: 0;
  border-radius: 7px;
  background: #f0edff;
  color: #5b4bff;
  font-size: 11px;
  font-weight: 800;
  cursor: pointer;
}

.user-agent-template-icon.purple,
.user-agent-avatar.purple {
  background: #7466ff;
}

.user-agent-template-icon.green,
.user-agent-avatar.green {
  background: #29b978;
}

.user-agent-template-icon.orange,
.user-agent-avatar.orange {
  background: #ff8438;
}

.user-agent-template-icon.blue,
.user-agent-avatar.blue {
  background: #3478f6;
}

.user-agent-list-panel {
  height: 486px;
  margin-top: 2px;
  padding: 14px 18px 0;
  overflow: hidden;
}

.user-agent-list-head {
  height: 40px;
}

.user-agent-tabs {
  display: flex;
  gap: 28px;
  height: 100%;
}

.user-agent-tabs button {
  position: relative;
  height: 34px;
  border: 0;
  background: transparent;
  color: #6b7280;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
}

.user-agent-tabs button.active {
  color: #5b4bff;
}

.user-agent-tabs button.active::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -6px;
  width: 62px;
  height: 2px;
  border-radius: 1px;
  background: #5b4bff;
}

.user-agent-list-tools {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user-agent-list-tools label,
.user-agent-list-tools select {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  border: 1px solid #e6e9f2;
  border-radius: 8px;
  background: #ffffff;
  color: #8a94a8;
  padding: 0 10px;
}

.user-agent-list-tools input {
  width: 130px;
  border: 0;
  outline: 0;
  color: #334155;
  font-size: 12px;
}

.user-agent-list-tools select {
  width: 112px;
  font-size: 12px;
}

.user-agent-list-tools > button {
  height: 32px;
  border: 0;
  border-radius: 8px;
  padding: 0 12px;
  background: #6658ff;
  color: #ffffff;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.user-agent-table {
  margin-top: 12px;
}

.user-agent-table-head,
.user-agent-table-row {
  display: grid;
  grid-template-columns: minmax(216px, 1.6fr) minmax(68px, 0.55fr) minmax(68px, 0.5fr) minmax(92px, 0.68fr) minmax(102px, 0.72fr) minmax(68px, 0.5fr) minmax(122px, 0.8fr) minmax(132px, 0.8fr);
  gap: 8px;
  align-items: center;
}

.user-agent-table-head {
  height: 34px;
  color: #65708a;
  font-size: 11px;
  font-weight: 700;
}

.user-agent-table-row {
  min-height: 64px;
  border-top: 1px solid #eef0f6;
  color: #111827;
  font-size: 12px;
  font-weight: 700;
}

.user-agent-table-row.is-clickable {
  cursor: pointer;
}

.user-agent-table-row.is-clickable:hover {
  background: #f8fafc;
}

.user-agent-table-row.is-officecli {
  background: #fff8f1;
  cursor: pointer;
}

.user-agent-table-row.is-officecli:hover {
  background: #fff3e7;
}

.user-agent-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.user-agent-avatar {
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: 50%;
  font-size: 11px;
}

.user-agent-name-cell div,
.user-agent-name-cell strong,
.user-agent-name-cell small,
.user-agent-table-row > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-agent-name-cell strong {
  display: block;
  font-size: 12px;
  font-weight: 800;
}

.user-agent-name-cell small {
  display: block;
  margin-top: 2px;
  color: #7a849b;
  font-size: 10px;
  font-weight: 500;
}

.user-agent-pill,
.user-agent-status {
  width: fit-content;
  max-width: 100%;
  border-radius: 999px;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 800;
}

.user-agent-pill.purple {
  background: #eef2ff;
  color: #5b4bff;
}

.user-agent-pill.green,
.user-agent-status {
  background: #e7f8ed;
  color: #15a060;
}

.user-agent-pill.orange {
  background: #fff0e4;
  color: #ff6b1a;
}

.user-agent-pill.blue {
  background: #eaf2ff;
  color: #3478f6;
}

.user-agent-status.draft {
  background: #eaf2ff;
  color: #3478f6;
}

.user-agent-status.disabled {
  background: #fff0e4;
  color: #ff6b1a;
}

.user-agent-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 4px;
  min-width: 0;
  color: #56617a;
  white-space: nowrap;
}

.user-agent-row-actions button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  min-width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid #e6e9f2;
  border-radius: 999px;
  background: #ffffff;
  color: #56617a;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
}

.user-agent-row-actions button .el-icon {
  font-size: 13px;
}

.user-agent-row-actions button.is-wide {
  width: auto;
  min-width: 50px;
  height: 26px;
  border: 1px solid #ffcfaa;
  border-radius: 999px;
  background: #fff7ed;
  color: #ff6b1a;
  padding: 0 10px;
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.user-agent-side-panel {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.user-agent-side-card {
  width: 100%;
  min-width: 0;
  max-width: 292px;
  padding: 18px;
  overflow: hidden;
}

.user-agent-side-card.is-metrics {
  height: 248px;
}

.user-agent-side-card.is-trend {
  height: 204px;
}

.user-agent-side-card.is-ranking {
  height: 194px;
}

.user-agent-side-card.is-shortcuts {
  height: 160px;
}

.user-agent-metric-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0;
  margin-top: 28px;
}

.user-agent-metric-grid div {
  display: grid;
  gap: 8px;
  min-height: 74px;
  padding: 0 0 10px;
}

.user-agent-metric-grid div:nth-child(odd) {
  border-right: 1px solid #eef0f6;
}

.user-agent-metric-grid div:nth-child(even) {
  padding-left: 18px;
}

.user-agent-metric-grid span {
  color: #65708a;
  font-size: 11px;
  line-height: 16px;
}

.user-agent-metric-grid strong {
  color: #111827;
  font-size: 23px;
  font-weight: 850;
  line-height: 28px;
}

.user-agent-metric-grid small {
  color: #059669;
  font-size: 11px;
  font-weight: 800;
}

.user-agent-trend {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  align-items: end;
  gap: 14px;
  height: 140px;
  margin-top: 10px;
  padding: 20px 8px 0;
}

.user-agent-trend span {
  display: grid;
  align-items: end;
  gap: 8px;
  height: 100%;
}

.user-agent-trend i {
  display: block;
  width: 14px;
  border-radius: 999px;
  background: #d9d3ff;
}

.user-agent-trend span:last-child i {
  background: #6a55ff;
}

.user-agent-trend em {
  width: 34px;
  margin-left: -10px;
  color: #7b849a;
  font-size: 10px;
  font-style: normal;
  text-align: center;
}

.user-agent-ranking {
  display: grid;
  gap: 8px;
  margin: 16px 0 0;
  padding: 0;
  list-style: none;
}

.user-agent-ranking li {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 44px;
  gap: 8px;
  align-items: center;
  height: 16px;
  color: #334155;
  font-size: 11px;
}

.user-agent-ranking li span {
  color: #ff6b18;
  font-weight: 900;
}

.user-agent-ranking li:nth-child(n + 4) span {
  color: #8a94a8;
}

.user-agent-ranking li b {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-agent-ranking li em {
  color: #334155;
  font-style: normal;
  text-align: right;
}

.user-agent-shortcuts {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin-top: 22px;
}

.user-agent-shortcuts button {
  display: grid;
  justify-items: center;
  gap: 9px;
  border: 0;
  background: transparent;
  color: #344054;
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
}

.user-agent-shortcuts span {
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  border-radius: 12px;
  background: #f2f4ff;
  color: #5b4bff;
  font-size: 11px;
  font-weight: 900;
}

.user-agent-shortcuts button:nth-child(3) span {
  color: #21b868;
}

.user-agent-mobile-status,
.user-agent-mobile-top,
.user-agent-mobile-overview,
.user-agent-mobile-template-card,
.user-agent-mobile-agent-card,
.user-agent-mobile-all,
.user-agent-mobile-bottom {
  background: #ffffff;
}

@media (min-width: 761px) {
  .user-agent-center-page {
    padding: 0;
    overflow-x: hidden;
  }

  .user-agent-desktop-view,
  .user-agent-center-layout,
  .user-agent-officecli-workspace,
  .user-agent-workspace,
  .user-agent-main-column {
    width: 100%;
    min-width: 0;
    max-width: 100%;
  }

  .user-agent-center-layout {
    grid-template-columns: minmax(0, 1fr) minmax(286px, 340px);
    margin: 0;
  }

  .user-agent-center-hero {
    grid-template-columns: minmax(0, 1fr) minmax(210px, 28%);
  }

  .user-agent-hero-robot {
    justify-self: end;
    width: min(330px, 100%);
  }

  .user-agent-template-panel {
    height: auto;
    min-height: 218px;
  }

  .user-agent-template-grid {
    grid-template-columns: repeat(auto-fit, minmax(108px, 1fr));
    gap: 10px;
  }

  .user-agent-template-card {
    width: auto;
    min-width: 0;
    min-height: 168px;
    height: auto;
  }

  .user-agent-list-panel {
    height: auto;
    min-height: 486px;
    padding-bottom: 16px;
  }

  .user-agent-list-head {
    height: auto;
    min-height: 40px;
    flex-wrap: wrap;
  }

  .user-agent-list-tools {
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .user-agent-list-tools label {
    flex: 1 1 190px;
    max-width: 280px;
  }

  .user-agent-list-tools input {
    width: 100%;
    min-width: 0;
  }

  .user-agent-table {
    overflow-x: auto;
    padding-bottom: 6px;
    scrollbar-width: thin;
  }

  .user-agent-table-head,
  .user-agent-table-row {
    min-width: 0;
    grid-template-columns:
      minmax(216px, 1.6fr)
      minmax(68px, 0.55fr)
      minmax(68px, 0.5fr)
      minmax(92px, 0.68fr)
      minmax(102px, 0.72fr)
      minmax(68px, 0.5fr)
      minmax(122px, 0.8fr)
      minmax(132px, 0.8fr);
  }

  .user-agent-side-card {
    max-width: none;
  }
}

@media (min-width: 1501px) {
  .user-agent-table-head,
  .user-agent-table-row {
    min-width: 0;
  }
}

@media (min-width: 761px) and (max-width: 1500px) {
  .user-agent-center-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .user-agent-side-panel {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .user-agent-side-card {
    height: auto;
    min-height: 160px;
  }
}

@media (min-width: 761px) and (max-width: 1080px) {
  .user-agent-center-hero {
    grid-template-columns: minmax(0, 1fr);
    height: auto;
    min-height: 116px;
    padding: 24px;
  }

  .user-agent-hero-robot {
    display: none;
  }

  .user-agent-list-head,
  .user-agent-list-tools {
    justify-content: flex-start;
  }

  .user-agent-side-panel {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 760px) {
  .user-agent-center-page {
    min-height: 100vh;
    padding: 0;
    overflow: hidden;
    background: #f7f8fc;
  }

  .user-agent-desktop-view {
    display: none;
  }

  .user-agent-mobile-view {
    position: relative;
    display: block;
    width: min(100vw, 390px);
    min-height: 844px;
    margin: 0 auto;
    padding: 0 0 76px;
    overflow: hidden;
    background: #f7f8fc;
  }

  .user-agent-center-page.has-officecli-workspace,
  .user-agent-center-page.has-agent-workspace {
    overflow: auto;
  }

  .user-agent-center-page.has-officecli-workspace .user-agent-desktop-view,
  .user-agent-center-page.has-agent-workspace .user-agent-desktop-view {
    display: block;
    padding: 12px;
  }

  .user-agent-center-page.has-officecli-workspace .user-agent-mobile-view,
  .user-agent-center-page.has-agent-workspace .user-agent-mobile-view {
    display: none;
  }

  .user-agent-officecli-workspace,
  .user-agent-workspace {
    width: 100%;
  }

  .officecli-workspace-head,
  .agent-workspace-head {
    grid-template-columns: minmax(0, 1fr);
    align-items: start;
    min-height: 0;
    padding: 18px;
  }

  .officecli-workspace-head > button,
  .agent-workspace-head > button {
    width: fit-content;
  }

  .user-agent-officecli-workbench.is-workspace .officecli-workbench-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .agent-workspace-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .agent-workspace-chat > header {
    align-items: flex-start;
    flex-direction: column;
  }

  .user-agent-mobile-status {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 42px;
    padding: 0 24px;
    color: #111827;
    font-size: 14px;
    font-weight: 800;
  }

  .user-agent-mobile-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    height: 74px;
    padding: 13px 20px 0;
    border-bottom: 1px solid #eef0f6;
  }

  .user-agent-mobile-top h2 {
    margin: 0;
    color: #111827;
    font-size: 24px;
    font-weight: 900;
    line-height: 30px;
  }

  .user-agent-mobile-top p {
    margin: 3px 0 0;
    color: #69738a;
    font-size: 12px;
    line-height: 18px;
  }

  .user-agent-mobile-top button {
    width: 44px;
    height: 36px;
    border: 0;
    border-radius: 12px;
    background: #6658ff;
    color: #ffffff;
    font-size: 24px;
    font-weight: 800;
    line-height: 1;
  }

  .user-agent-mobile-search {
    display: flex;
    align-items: center;
    gap: 10px;
    width: calc(100% - 40px);
    height: 42px;
    margin: 16px 20px 0;
    border: 1px solid #e1e6f1;
    border-radius: 12px;
    padding: 0 15px;
    background: #ffffff;
  }

  .user-agent-mobile-search i {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #c8d0e0;
  }

  .user-agent-mobile-search input {
    min-width: 0;
    flex: 1;
    border: 0;
    outline: 0;
    color: #334155;
    font-size: 13px;
  }

  .user-agent-mobile-overview {
    position: relative;
    width: calc(100% - 40px);
    height: 126px;
    margin: 16px 20px 0;
    border: 1px solid #e1e6f1;
    border-radius: 14px;
    padding: 16px 18px;
    overflow: hidden;
    box-shadow: 0 16px 36px rgba(32, 47, 86, 0.06);
  }

  .user-agent-mobile-overview strong {
    display: block;
    color: #111827;
    font-size: 16px;
    font-weight: 900;
    line-height: 22px;
  }

  .user-agent-mobile-overview p {
    margin: 4px 0 0;
    color: #69738a;
    font-size: 12px;
    line-height: 18px;
  }

  .user-agent-mobile-bot {
    position: absolute;
    right: 30px;
    top: 16px;
    display: grid;
    place-items: center;
    width: 78px;
    height: 70px;
    border-radius: 28px;
    background: #eeecff;
    color: transparent;
    font-size: 0;
    font-weight: 900;
  }

  .user-agent-mobile-bot::before {
    content: "AI";
    display: grid;
    place-items: center;
    width: 48px;
    height: 36px;
    border-radius: 12px;
    background: #5649dc;
    color: #ffffff;
    font-size: 13px;
  }

  .user-agent-mobile-metrics {
    position: absolute;
    left: 18px;
    right: 18px;
    bottom: 12px;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
  }

  .user-agent-mobile-metrics span {
    display: grid;
    gap: 1px;
    color: #7a849b;
    font-size: 11px;
    line-height: 14px;
  }

  .user-agent-mobile-metrics strong {
    color: #111827;
    font-size: 19px;
    line-height: 22px;
  }

  .user-agent-mobile-section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: calc(100% - 40px);
    margin: 20px 20px 0;
  }

  .user-agent-mobile-section-head strong {
    color: #111827;
    font-size: 17px;
    font-weight: 900;
    line-height: 22px;
  }

  .user-agent-mobile-section-head button {
    height: 30px;
    border: 0;
    border-radius: 10px;
    padding: 0 11px;
    background: #ffffff;
    color: #5b4bff;
    font-size: 12px;
    font-weight: 800;
  }

  .user-agent-mobile-template-scroll {
    display: flex;
    gap: 12px;
    width: 100%;
    margin-top: 12px;
    padding: 0 20px;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .user-agent-mobile-template-scroll::-webkit-scrollbar {
    display: none;
  }

  .user-agent-mobile-template-card {
    display: grid;
    flex: 0 0 106px;
    justify-items: center;
    align-content: start;
    width: 106px;
    height: 118px;
    border: 1px solid #e1e6f1;
    border-radius: 12px;
    padding: 14px 8px;
    text-align: center;
    box-shadow: 0 12px 30px rgba(32, 47, 86, 0.05);
  }

  .user-agent-mobile-template-card .user-agent-template-icon {
    width: 42px;
    height: 42px;
    border-radius: 12px;
  }

  .user-agent-mobile-template-card strong {
    width: 90px;
    margin-top: 8px;
    overflow: hidden;
    color: #111827;
    font-size: 14px;
    font-weight: 900;
    line-height: 18px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .user-agent-mobile-template-card small {
    width: 90px;
    margin-top: 2px;
    overflow: hidden;
    color: #7a849b;
    font-size: 12px;
    line-height: 16px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .user-agent-mobile-list {
    display: grid;
    gap: 12px;
    width: calc(100% - 40px);
    margin: 12px 20px 0;
  }

  .user-agent-mobile-agent-card {
    position: relative;
    display: grid;
    grid-template-columns: 38px minmax(0, 1fr) 74px;
    align-items: center;
    gap: 12px;
    height: 74px;
    border: 1px solid #e1e6f1;
    border-radius: 14px;
    padding: 0 14px;
    box-shadow: 0 12px 30px rgba(32, 47, 86, 0.05);
  }

  .user-agent-mobile-agent-card .user-agent-avatar {
    width: 38px;
    height: 38px;
    flex-basis: 38px;
  }

  .user-agent-mobile-agent-card div {
    min-width: 0;
  }

  .user-agent-mobile-agent-card strong,
  .user-agent-mobile-agent-card small {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .user-agent-mobile-agent-card strong {
    color: #111827;
    font-size: 14px;
    font-weight: 900;
    line-height: 20px;
  }

  .user-agent-mobile-agent-card small {
    margin-top: 4px;
    color: #69738a;
    font-size: 12px;
    line-height: 16px;
  }

  .user-agent-mobile-agent-card em {
    position: absolute;
    top: 14px;
    right: 76px;
    border-radius: 999px;
    padding: 4px 10px;
    background: #e7f8ed;
    color: #15a060;
    font-size: 11px;
    font-style: normal;
    font-weight: 800;
  }

  .user-agent-mobile-agent-card b {
    justify-self: end;
    align-self: end;
    margin-bottom: 13px;
    color: #111827;
    font-size: 13px;
    font-weight: 900;
  }

  .user-agent-mobile-all {
    width: calc(100% - 40px);
    height: 38px;
    margin: 12px 20px 0;
    border: 1px solid #e1e6f1;
    border-radius: 12px;
    color: #5b4bff;
    font-size: 12px;
    font-weight: 900;
  }

  .user-agent-mobile-bottom {
    position: fixed;
    left: 50%;
    bottom: 0;
    z-index: 80;
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    width: min(100vw, 390px);
    height: 60px;
    border-top: 1px solid #eef0f6;
    transform: translateX(-50%);
  }

  .user-agent-mobile-bottom button {
    display: grid;
    place-items: center;
    align-content: center;
    gap: 4px;
    border: 0;
    background: transparent;
    color: #8a94a8;
    font-size: 10px;
    font-weight: 700;
  }

  .user-agent-mobile-bottom span {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: #d5ddec;
    color: #ffffff;
    font-size: 10px;
    font-weight: 900;
  }

  .user-agent-mobile-bottom button.active {
    color: #5b4bff;
  }

  .user-agent-mobile-bottom button.active span {
    background: #6658ff;
  }
}
/* DESIGN.md agent center console refinement. Scoped to the active module shell only. */
@media (min-width: 99999px) {
  .user-agent-figma-shell.admin-shell,
  .user-agent-figma-shell .admin-workspace,
  .user-agent-figma-shell .admin-main {
    background: #ffffff;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) {
    --agent-design-ink: #181d26;
    --agent-design-active: #0d1218;
    --agent-design-body: #333840;
    --agent-design-muted: #41454d;
    --agent-design-hairline: #dddddd;
    --agent-design-soft: #f8fafc;
    --agent-design-strong: #e0e2e6;
    --agent-design-coral: #aa2d00;
    --agent-design-forest: #0a2e0e;
    --agent-design-cream: #f5e9d4;
    --agent-design-peach: #fcab79;
    --agent-design-mint: #a8d8c4;
    display: block;
    min-height: calc(100vh - 56px);
    padding: 24px 32px 36px;
    overflow: auto;
    background: #ffffff;
    color: var(--agent-design-body);
    font-family: Inter, "Microsoft YaHei", "PingFang SC", "Segoe UI", Arial, sans-serif;
    letter-spacing: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) * {
    letter-spacing: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    gap: 24px;
    align-items: start;
    width: min(1280px, 100%);
    margin: 0 auto;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-main-column {
    gap: 24px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-panel,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card {
    border: 1px solid var(--agent-design-hairline);
    border-radius: 12px;
    background: #ffffff;
    box-shadow: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero {
    grid-template-columns: minmax(0, 1fr) 220px;
    min-height: 132px;
    height: auto;
    padding: 28px 32px;
    overflow: hidden;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero span {
    color: var(--agent-design-muted);
    font-size: 13px;
    font-weight: 500;
    text-transform: uppercase;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero h2 {
    margin: 8px 0 10px;
    color: var(--agent-design-ink);
    font-size: 32px;
    font-weight: 400;
    line-height: 1.2;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero p {
    max-width: 620px;
    color: var(--agent-design-body);
    font-size: 14px;
    font-weight: 400;
    line-height: 1.55;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot {
    justify-self: end;
    width: 210px;
    height: 96px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot i {
    left: 26px;
    top: 14px;
    width: 120px;
    height: 66px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 12px;
    background: var(--agent-design-cream);
    transform: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot b {
    left: 132px;
    top: 28px;
    width: 56px;
    height: 42px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: var(--agent-design-mint);
    transform: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot::before,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot::after {
    display: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot em,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot strong {
    position: absolute;
    display: grid;
    place-items: center;
    border-radius: 6px;
    font-style: normal;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot em {
    left: 48px;
    top: 34px;
    width: 52px;
    height: 28px;
    background: var(--agent-design-ink);
    color: #ffffff;
    font-size: 13px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot strong {
    left: 116px;
    top: 36px;
    width: 46px;
    height: 24px;
    background: #ffffff;
    color: var(--agent-design-ink);
    font-size: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-panel,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card {
    padding: 24px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel {
    height: auto;
    min-height: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel header,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-head,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card header {
    align-items: center;
    gap: 16px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel header strong,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card > strong,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card header strong {
    color: var(--agent-design-ink);
    font-size: 18px;
    font-weight: 500;
    line-height: 1.4;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-panel header button,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card header button {
    min-height: 34px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: #ffffff;
    color: var(--agent-design-ink);
    padding: 0 12px;
    font-size: 13px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-grid {
    grid-template-columns: repeat(auto-fit, minmax(136px, 1fr));
    gap: 12px;
    margin-top: 20px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card {
    width: auto;
    min-height: 170px;
    gap: 10px;
    align-content: start;
    justify-items: start;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: #ffffff;
    padding: 16px;
    text-align: left;
    box-shadow: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card.is-featured {
    border-color: var(--agent-design-hairline);
    background: var(--agent-design-cream);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card.is-clickable:hover {
    border-color: #9297a0;
    box-shadow: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-icon {
    width: 42px;
    height: 42px;
    border-radius: 10px;
    font-size: 12px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card strong {
    color: var(--agent-design-ink);
    font-size: 14px;
    font-weight: 500;
    line-height: 1.35;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card p {
    min-height: 36px;
    color: var(--agent-design-muted);
    font-size: 12px;
    font-weight: 400;
    line-height: 1.5;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-card button {
    width: auto;
    min-width: 62px;
    height: 32px;
    margin-top: auto;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: #ffffff;
    color: var(--agent-design-ink);
    padding: 0 12px;
    font-size: 13px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-icon.purple,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-avatar.purple {
    background: var(--agent-design-ink);
    color: #ffffff;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-icon.green,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-avatar.green {
    background: var(--agent-design-forest);
    color: #ffffff;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-icon.orange,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-avatar.orange {
    background: var(--agent-design-coral);
    color: #ffffff;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-template-icon.blue,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-avatar.blue {
    background: var(--agent-design-mint);
    color: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-panel {
    height: auto;
    min-height: 0;
    overflow: visible;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-head {
    min-height: 44px;
    height: auto;
    flex-wrap: wrap;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-tabs {
    gap: 22px;
    height: auto;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-tabs button {
    height: 40px;
    color: var(--agent-design-muted);
    font-size: 14px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-tabs button.active {
    color: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-tabs button.active::after {
    bottom: -1px;
    width: 100%;
    height: 2px;
    background: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools {
    flex: 1 1 420px;
    justify-content: flex-end;
    gap: 8px;
    min-width: 0;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: var(--agent-design-soft);
    padding: 6px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools label,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools select {
    flex: 0 1 auto;
    height: 44px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 6px;
    background: #ffffff;
    color: var(--agent-design-body);
    padding: 0 14px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools label {
    min-width: 220px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools input {
    width: 100%;
    min-width: 0;
    color: var(--agent-design-ink);
    font-size: 14px;
    font-weight: 400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools input::placeholder {
    color: var(--agent-design-muted);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools select {
    min-width: 118px;
    font-size: 14px;
    font-weight: 400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools > button {
    height: 44px;
    border: 1px solid var(--agent-design-ink);
    border-radius: 12px;
    background: var(--agent-design-ink);
    color: #ffffff;
    padding: 0 18px;
    font-size: 14px;
    font-weight: 500;
    white-space: nowrap;
    box-shadow: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools > button:active {
    background: var(--agent-design-active);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table {
    min-width: 0;
    margin-top: 20px;
    overflow-x: auto;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: #ffffff;
    scrollbar-width: thin;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-head,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row {
    min-width: 860px;
    grid-template-columns:
      minmax(190px, 1.42fr)
      minmax(64px, 0.46fr)
      minmax(62px, 0.44fr)
      minmax(78px, 0.58fr)
      minmax(86px, 0.64fr)
      minmax(58px, 0.42fr)
      minmax(104px, 0.72fr)
      minmax(144px, 0.92fr);
    gap: 10px;
    padding: 0 14px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-head {
    min-height: 44px;
    height: 44px;
    border-bottom: 1px solid var(--agent-design-hairline);
    background: var(--agent-design-soft);
    color: var(--agent-design-muted);
    font-size: 12px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row {
    min-height: 72px;
    border-top: 0;
    border-bottom: 1px solid var(--agent-design-hairline);
    background: #ffffff;
    color: var(--agent-design-body);
    font-size: 13px;
    font-weight: 400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row:last-child {
    border-bottom: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row.is-clickable:hover {
    background: var(--agent-design-soft);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row.is-officecli {
    background: #fffaf5;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-table-row.is-officecli:hover {
    background: var(--agent-design-cream);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-name-cell {
    gap: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-avatar {
    width: 36px;
    height: 36px;
    flex-basis: 36px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-name-cell strong {
    color: var(--agent-design-ink);
    font-size: 14px;
    font-weight: 500;
    line-height: 1.35;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-name-cell small {
    margin-top: 4px;
    color: var(--agent-design-muted);
    font-size: 12px;
    font-weight: 400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-pill,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-status {
    border-radius: 6px;
    padding: 4px 8px;
    font-size: 12px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-pill.purple {
    background: var(--agent-design-soft);
    color: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-pill.green,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-status {
    background: #eaf6ea;
    color: #006400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-pill.orange {
    background: var(--agent-design-cream);
    color: var(--agent-design-coral);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-pill.blue,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-status.draft {
    background: #edf4ff;
    color: #254fad;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-status.disabled {
    background: var(--agent-design-strong);
    color: var(--agent-design-muted);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-row-actions {
    justify-content: flex-start;
    gap: 4px;
    min-width: 144px;
    white-space: nowrap;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-row-actions button {
    width: 26px;
    min-width: 26px;
    height: 28px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 999px;
    background: #ffffff;
    color: var(--agent-design-ink);
    font-size: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-row-actions button.is-wide {
    width: auto;
    min-width: 54px;
    height: 28px;
    border-color: var(--agent-design-ink);
    border-radius: 10px;
    background: var(--agent-design-ink);
    color: #ffffff;
    padding: 0 12px;
    font-size: 13px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-panel {
    gap: 16px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card {
    max-width: none;
    height: auto;
    min-height: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card.is-metrics,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card.is-trend,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card.is-ranking,
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-card.is-shortcuts {
    height: auto;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid {
    gap: 8px;
    margin-top: 18px;
    border: 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid div {
    min-height: 86px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: var(--agent-design-soft);
    padding: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid div:nth-child(even) {
    padding-left: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid span {
    color: var(--agent-design-muted);
    font-size: 12px;
    font-weight: 400;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid strong {
    color: var(--agent-design-ink);
    font-size: 26px;
    font-weight: 400;
    line-height: 1.2;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-metric-grid small {
    color: #006400;
    font-size: 12px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-trend {
    height: 156px;
    gap: 10px;
    margin-top: 14px;
    padding: 18px 2px 0;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-trend i {
    width: 16px;
    border-radius: 6px 6px 0 0;
    background: var(--agent-design-strong);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-trend span:nth-child(3n) i {
    background: var(--agent-design-mint);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-trend span:last-child i {
    background: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-trend em {
    color: var(--agent-design-muted);
    font-size: 11px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking {
    gap: 8px;
    margin-top: 16px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking li {
    min-height: 32px;
    grid-template-columns: 22px minmax(0, 1fr) 54px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 8px;
    background: var(--agent-design-soft);
    padding: 0 10px;
    color: var(--agent-design-body);
    font-size: 12px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking li span {
    color: var(--agent-design-coral);
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking li:nth-child(n + 4) span {
    color: var(--agent-design-muted);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking li b {
    color: var(--agent-design-ink);
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-ranking li em {
    color: var(--agent-design-muted);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
    margin-top: 16px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts button {
    min-height: 68px;
    border: 1px solid var(--agent-design-hairline);
    border-radius: 10px;
    background: #ffffff;
    color: var(--agent-design-ink);
    padding: 10px;
    font-size: 12px;
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts span {
    width: 34px;
    height: 34px;
    border-radius: 10px;
    background: var(--agent-design-cream);
    color: var(--agent-design-ink);
    font-weight: 500;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts button:nth-child(2) span {
    background: var(--agent-design-mint);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts button:nth-child(3) span {
    background: var(--agent-design-peach);
    color: var(--agent-design-ink);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-shortcuts button:nth-child(4) span {
    background: var(--agent-design-ink);
    color: #ffffff;
  }
}

@media (min-width: 99999px) and (max-width: 1280px) {
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) {
    padding: 20px;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-panel {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 99999px) and (max-width: 980px) {
  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-center-hero {
    grid-template-columns: minmax(0, 1fr);
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-hero-robot {
    display: none;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-list-tools {
    justify-content: flex-start;
  }

  .user-agent-figma-shell .user-agent-center-page:not(.has-officecli-workspace):not(.has-agent-workspace) .user-agent-side-panel {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

















