<template>
  <el-config-provider :size="currentElementSize">
  <el-container :class="['admin-shell', 'pure-admin-shell', { 'mobile-drawer-open': mobileDrawerOpen, 'desktop-sidebar-collapsed': desktopSidebarCollapsed }]">
    <div v-if="mobileDrawerOpen" class="mobile-drawer-mask" @click="mobileDrawerOpen = false"></div>
    <el-aside width="200px" class="admin-sidebar">
      <div class="brand">
        <img class="brand-logo" :src="xianzhiLogo" alt="先知 AI" />
        <div class="brand-copy">
          <strong>先知 AI</strong>
          <small>{{ isUserConsole ? "User Console" : isAgentConsole ? "Agent Console" : "Master SaaS Console" }}</small>
        </div>
      </div>
      <div class="sidebar-section-label">{{ isUserConsole ? "用户导航" : isAgentConsole ? "代理导航" : "平台导航" }}</div>
      <nav class="collapsed-icon-menu" aria-label="折叠模块导航">
        <div v-for="group in visibleModuleGroups" :key="group.id" :class="['collapsed-icon-group', { 'is-active': isGroupActive(group) }]">
          <button class="collapsed-icon-button" type="button" :aria-label="group.title" @click="selectAdminModule(group.items[0]?.id || store.activeModuleId)">
            <el-icon><component :is="group.icon" /></el-icon>
          </button>
          <div class="collapsed-flyout" role="menu">
            <strong>{{ group.title }}</strong>
            <button v-for="item in group.items" :key="item.id" :class="{ 'is-active': item.id === store.activeModuleId }" type="button" role="menuitem" @click.stop="selectAdminModule(item.id)">
              <el-icon><component :is="iconFor(item.id)" /></el-icon>
              <span>{{ item.title }}</span>
            </button>
          </div>
        </div>
      </nav>
      <el-menu class="sidebar-menu" :default-active="store.activeModuleId" @select="selectAdminModule">
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
      </aside>
    </el-aside>
    <el-container class="admin-workspace">
      <div class="mobile-admin-bar">
        <el-button class="mobile-collapse-button" :icon="Grid" aria-label="打开模块导航" @click="mobileDrawerOpen = true" />
        <div class="mobile-admin-title">
          <strong>{{ isUserConsole ? "用户后台" : isAgentConsole ? "代理商后台" : "主控 SaaS" }}</strong>
          <small>{{ store.activeModule.title }}</small>
        </div>
        <div class="mobile-admin-actions">
          <el-tag :type="store.error ? 'danger' : 'success'" effect="light">{{ store.error ? "API ERROR" : "API ONLINE" }}</el-tag>
          <el-dropdown trigger="click" @command="handleAccountCommand">
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
            <el-breadcrumb separator="/">
              <el-breadcrumb-item>{{ activeGroupLabel }}</el-breadcrumb-item>
              <el-breadcrumb-item>{{ store.activeModule.title }}</el-breadcrumb-item>
            </el-breadcrumb>
          </div>
        </div>
        <div class="header-actions">
          <el-input v-model="searchKeyword" class="header-search" :prefix-icon="Search" clearable placeholder="搜索当前模块" />
          <el-button :icon="Refresh" circle :loading="store.loading" @click="store.loadActiveModule" />
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
          <el-dropdown trigger="click" @command="handleAccountCommand">
            <el-button class="account-button">
              <el-icon><UserFilled /></el-icon>
              <span>{{ currentAdmin?.name || "平台管理员" }}</span>
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
      <nav class="admin-page-tabs" aria-label="已打开页面标签">
        <button class="tabs-rail-button" type="button" aria-label="向左滚动标签" @click="scrollOpenTabs(-1)">«</button>
        <div ref="tabsScrollRef" class="tabs-scroll">
          <button v-for="tab in openTabs" :key="tab.id" :class="['page-tab', { 'is-active': tab.id === store.activeModuleId }]" type="button" @click="selectAdminModule(tab.id)">
            <span>{{ tab.title }}</span>
            <i v-if="openTabs.length > 1" role="button" aria-label="关闭标签" @click.stop="closeOpenTab(tab.id)">×</i>
          </button>
        </div>
        <button class="tabs-rail-button" type="button" aria-label="向右滚动标签" @click="scrollOpenTabs(1)">»</button>
        <button class="tabs-tool-button" type="button" aria-label="刷新当前页" @click="store.loadActiveModule"><el-icon><Refresh /></el-icon></button>
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
          <section v-if="searchKeyword.trim()" class="global-search-panel">
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
                <button v-for="item in currentRecordResults" :key="item.key" type="button"><strong>{{ item.title }}</strong><small>{{ item.desc }}</small></button>
                <el-empty v-if="!currentRecordResults.length" description="当前模块没有匹配记录" :image-size="56" />
              </article>
            </div>
          </section>
          <section v-if="!['analysis', 'workbench', 'partnerDashboard', 'userAiImage', 'apiSettings'].includes(store.activeModuleId)" class="module-hero">
            <div>
              <el-tag effect="dark" type="primary">{{ activeModuleMeta.badge }}</el-tag>
              <h2>{{ store.activeModule.title }}</h2>
              <p>{{ activeModuleMeta.description }}</p>
            </div>
            <div class="module-hero-actions">
              <el-button v-for="action in toolbarActions" :key="action.action" type="primary" :icon="Plus" @click="runAction(action.action)">{{ action.label }}</el-button>
              <el-button :icon="Refresh" @click="store.loadActiveModule">刷新数据</el-button>
            </div>
          </section>
          <div v-if="!['analysis', 'workbench', 'partnerDashboard', 'userAiImage', 'apiSettings'].includes(store.activeModuleId)" class="metric-grid">
            <article v-for="metric in metrics" :key="metric.label" class="metric-card">
              <span>{{ metric.label }}</span>
              <strong>{{ metric.value }}</strong>
              <small>{{ metricHint(metric.label) }}</small>
            </article>
          </div>
          <section v-if="store.activeModuleId === 'userOnlineImage'" class="online-image-page online-image-studio">
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
                    <el-upload v-for="slot in onlineReferenceSlots" :key="slot" :auto-upload="false" :show-file-list="false" class="online-upload-slot">
                      <button type="button" class="online-upload-card">
                        <el-icon><Plus /></el-icon>
                        <strong>参考图 {{ slot }}</strong>
                        <small>点击上传</small>
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
                  <el-button @click="selectAdminModule('userApiSettings')">API 设置</el-button>
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
                  <div class="online-history-thumb"><img v-if="item.thumbnailUrl" :src="String(item.thumbnailUrl)" alt="作品缩略图" /><span v-else>{{ String(item.model || 'AI').slice(0, 2) }}</span></div>
                  <strong>{{ item.name || item.prompt || item.id }}</strong>
                  <small>{{ item.model || '-' }} · {{ item.pointCost || 0 }} 点</small>
                </article>
              </div>
            </section>

            <el-card shadow="never" class="data-panel online-task-panel">
              <template #header><div class="panel-head"><div><span>生成队列</span><small>{{ onlineRecentTasks.length }} 条任务</small></div><el-segmented v-model="onlineStatusFilter" :options="onlineStatusOptions" /></div></template>
              <el-table v-if="filteredOnlineTasks.length" :data="filteredOnlineTasks" height="420" stripe>
                <el-table-column prop="id" label="任务" min-width="130" />
                <el-table-column prop="model" label="模型" min-width="150" />
                <el-table-column prop="type" label="类型" width="130" />
                <el-table-column prop="pointCost" label="消耗点数" width="110" />
                <el-table-column prop="status" label="状态" width="120"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ scope.row.status }}</el-tag></template></el-table-column>
                <el-table-column prop="createdAt" label="创建时间" min-width="210" show-overflow-tooltip />
              </el-table>
              <el-empty v-else description="暂无在线生图任务" />
            </el-card>
          </section>
          <section v-else-if="store.activeModuleId === 'userAiImage'" class="ai-image-page">
            <section class="ai-playground">
              <div class="ai-playground-header">
                <h3>AI Image Playground</h3>
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
                <div v-else-if="aiGalleryCards.length" class="ai-gallery-grid">
                  <article
                    v-for="task in aiGalleryCards"
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
                      <img v-if="aiTaskThumbnailUrl(task)" :src="aiTaskThumbnailUrl(task)" alt="AI 生图任务缩略图" />
                      <div v-else-if="isAiTaskRunning(task)" class="ai-task-running">
                        <span class="ai-task-spinner"></span>
                        <strong>生成中...</strong>
                      </div>
                      <div v-else-if="isAiTaskFailed(task)" class="ai-task-failed">
                        <el-icon><Monitor /></el-icon>
                        <strong>生成失败</strong>
                      </div>
                      <el-icon v-else><Monitor /></el-icon>
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
                        <button type="button" :class="{ 'is-favorite': isAiTaskFavorite(task) }" :aria-label="isAiTaskFavorite(task) ? '编辑收藏夹' : '收藏任务'" @click.stop="openAiFavoritePicker([aiTaskId(task)])">
                          <el-icon><component :is="isAiTaskFavorite(task) ? StarFilled : Star" /></el-icon>
                        </button>
                        <button type="button" aria-label="复用配置" @click.stop="reuseAiTask(task)">
                          <el-icon><Refresh /></el-icon>
                        </button>
                        <button type="button" aria-label="编辑输出" @click.stop="editAiTaskOutput(task)">
                          <el-icon><EditPen /></el-icon>
                        </button>
                        <button type="button" class="danger" aria-label="删除任务" title="删除任务" @click.stop="deleteAiTask(task)">
                          <el-icon><Delete /></el-icon>
                        </button>
                      </div>
                    </div>
                  </article>
                </div>
                <div v-else class="ai-empty-state">
                  <el-icon><Monitor /></el-icon>
                  <span>输入提示词开始生成图片</span>
                </div>
              </div>

              <div class="ai-floating-composer">
                <div v-if="aiReferenceImages.length" class="ai-reference-strip">
                  <div v-for="(image, index) in aiReferenceImages" :key="image.id" class="ai-reference-thumb">
                    <img :src="image.url" :alt="`参考图 ${index + 1}`" />
                    <span>{{ index + 1 }}</span>
                    <button type="button" aria-label="移除参考图" @click="confirmRemoveAiReferenceImage(index)">×</button>
                  </div>
                  <button type="button" class="ai-reference-clear" @click="confirmClearAiReferenceImages">清空</button>
                </div>
                <el-input v-model="onlineImageForm.prompt" class="ai-floating-prompt" maxlength="1000" clearable placeholder="描述你想生成的图片，可输入 @ 来指定参考图..." @keyup.ctrl.enter="submitAiImage" />
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
                  <button type="button" @click="editAiContextImage"><el-icon><EditPen /></el-icon><span>编辑</span></button>
                </div>
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
                <el-table-column prop="status" label="状态" width="110"><template #default="scope"><el-tag :type="statusType(scope.row.status)">{{ scope.row.status }}</el-tag></template></el-table-column>
              </el-table>
            </el-card>
          </section>
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
                  <div class="traffic-legend">
                    <div v-for="source in trafficSources" :key="source.label"><i :style="{ backgroundColor: source.color }"></i><span>{{ source.label }}</span></div>
                  </div>
                  <div class="donut-chart" :style="trafficDonutStyle"><span>来源</span></div>
                </div>
              </el-card>
              <el-card shadow="never" class="analysis-card">
                <template #header><div class="panel-head"><span>每周生成任务活跃量</span><small>任务提交与完成趋势</small></div></template>
                <div class="bar-chart">
                  <div v-for="item in weeklyActivity" :key="item.day" class="bar-item"><span class="bar" :style="{ height: item.height + '%' }"></span><small>{{ item.day }}</small></div>
                </div>
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
              <div class="overview-board">
                <div v-for="metric in metrics.slice(0, 4)" :key="metric.label" class="overview-item"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong></div>
              </div>
            </el-card>
            <el-card shadow="never" class="dashboard-card">
              <template #header><div class="panel-head"><span>待办动作</span><el-tag type="warning">运营</el-tag></div></template>
              <div class="todo-list">
                <button v-for="item in quickTodos" :key="item.action" type="button" @click="selectAdminModule(item.module)"><span>{{ item.title }}</span><small>{{ item.desc }}</small></button>
              </div>
            </el-card>
          </section>
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
                      </select>
                      <select v-model="apiProviderDraft.imageRequestMode">
                        <option value="openai">图片：OpenAI 标准</option>
                        <option value="openai-json">图片：OpenAI JSON</option>
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
                      <button class="api-source-action" type="button" :disabled="apiFetchingDraftModels" @click="fetchApiDraftModels"><el-icon><Download /></el-icon>{{ apiFetchingDraftModels ? "拉取中..." : "拉取模型" }}</button>
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
          <el-card v-else-if="!['userOnlineImage', 'userAiImage', 'apiSettings'].includes(store.activeModuleId)" shadow="never" class="data-panel">
            <template #header>
              <div class="panel-head">
                <div><span>{{ store.activeModule.title }}</span><small>{{ filteredRows.length }} 条记录</small></div>
                <div class="toolbar"><el-button v-for="action in toolbarActions" :key="action.action" size="small" type="primary" :icon="Plus" @click="runAction(action.action)">{{ action.label }}</el-button></div>
              </div>
            </template>
            <template>
              <div class="table-tools"><el-input v-model="searchKeyword" class="table-search" :prefix-icon="Search" clearable placeholder="按名称、邮箱、ID、状态搜索" /><el-segmented v-model="statusFilter" :options="statusFilterOptions" /></div>
              <el-table v-if="filteredRows.length" :data="filteredRows" height="560" v-loading="store.saving" stripe>
                <el-table-column v-for="column in columns" :key="column" :prop="column" :label="columnLabels[column] || column" min-width="140" show-overflow-tooltip>
                  <template #default="scope"><el-tag v-if="isStatusColumn(column)" :type="statusType(scope.row[column])">{{ scope.row[column] || '-' }}</el-tag><span v-else>{{ formatCell(scope.row[column], column) }}</span></template>
                </el-table-column>
                <el-table-column v-if="rowActions.length" label="操作" fixed="right" width="190"><template #default="scope"><el-button v-for="action in visibleRowActions(scope.row)" :key="action.action" link type="primary" size="small" @click="runAction(action.action, scope.row)">{{ labelForRowAction(action, scope.row) }}</el-button></template></el-table-column>
              </el-table>
              <el-empty v-else description="暂无记录" />
            </template>
          </el-card>
        </section>
      </el-main>
    </el-container>
  </el-container>
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
  </el-config-provider>
</template>
<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox, type ComponentSize } from "element-plus";
import { ArrowDown, Check, Collection, Connection, CopyDocument, Cpu, DataAnalysis, Delete, Download, EditPen, Goods, Grid, House, Key, Lock, Money, Monitor, Operation, Plus, QuestionFilled, Refresh, Search, Setting, Star, StarFilled, SwitchButton, Tickets, User, UserFilled, Wallet } from "@element-plus/icons-vue";
import { adminRequest } from "./api/client";
import { adminModules, type AdminRecord, useAdminStore } from "./stores/admin";
import { type AiSettingsDraft, useAiSettingsStore } from "./stores/aiSettings";
import { readAiImageDraft, readCachedOriginalImage, writeAiImageDraft, writeCachedOriginalImage, type CachedReferenceImage } from "./utils/aiImageDb";
import xianzhiLogo from "./assets/xianzhi-ai-logo.png";

const store = useAdminStore();
const aiSettingsStore = useAiSettingsStore();
const modules = adminModules;
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
const statusFilter = ref("ALL");
const statusFilterOptions = [
  { label: "全部", value: "ALL" },
  { label: "启用", value: "ACTIVE" },
  { label: "待处理", value: "PENDING" },
  { label: "停用", value: "DISABLED" }
];

const adminModuleGroups = [
  { id: "home", title: "首页", icon: House, items: modules.filter((item) => ["analysis", "workbench", "dashboard"].includes(item.id)) },
  { id: "business", title: "业务运营", icon: Collection, items: modules.filter((item) => ["customers", "orders", "products", "plans"].includes(item.id)) },
  { id: "growth", title: "渠道增长", icon: Connection, items: modules.filter((item) => ["channels", "commissions", "usage"].includes(item.id)) },
  { id: "governance", title: "技术治理", icon: Cpu, items: modules.filter((item) => ["apiSettings", "system"].includes(item.id)) },
  { id: "permission", title: "权限管理", icon: Lock, items: modules.filter((item) => ["departments", "userManagement", "menuManagement"].includes(item.id)) }
];

const agentModuleGroups = [
  { id: "agentHome", title: "代理商后台", icon: Wallet, items: modules.filter((item) => ["partnerDashboard"].includes(item.id)) },
  { id: "agentBusiness", title: "业务管理", icon: Collection, items: modules.filter((item) => ["partnerCustomers", "partnerOrders", "partnerCommissions"].includes(item.id)) },
  { id: "agentGrowth", title: "推广增长", icon: Connection, items: modules.filter((item) => ["partnerChannels", "partnerMaterials"].includes(item.id)) },
  { id: "agentAccount", title: "账户中心", icon: Setting, items: modules.filter((item) => ["partnerAccount"].includes(item.id)) }
];
const userModuleGroups = [
  { id: "userHome", title: "用户后台", icon: House, items: modules.filter((item) => ["userDashboard"].includes(item.id)) },
  { id: "userCreation", title: "创作中心", icon: Collection, items: modules.filter((item) => ["userOnlineImage", "userAiImage", "userCanvas", "userWorks"].includes(item.id)) },
  { id: "userConfig", title: "配置中心", icon: Setting, items: modules.filter((item) => ["userApiSettings", "userMembership"].includes(item.id)) },
  { id: "userData", title: "数据记录", icon: DataAnalysis, items: modules.filter((item) => ["userUsage"].includes(item.id)) }
];

const pageMeta: Record<string, { badge: string; description: string }> = {
  userDashboard: { badge: "用户工作台", description: "聚合点数、生成、作品、API 和使用记录，作为用户登录后的 PC 中后台首页。" },
  userOnlineImage: { badge: "多平台生成", description: "统一接入 OpenAI 协议、Gemini、即梦 CLI、ComfyUI 本地等上游，按模型和数量扣减点数。" },
  userAiImage: { badge: "智能生成", description: "聚合 Prompt、参考图、模型参数和结果预览，快速生成商业素材。" },
  userCanvas: { badge: "灵感画布", description: "管理用户生成任务、画布记录和创作入口，PC 端作为工作台入口，移动端继续由 Uni-app 承载。" },
  userApiSettings: { badge: "API 设置", description: "用户侧查看 API 平台、模型计费、Key 和分组配置，复用 Element Plus 表格和表单密度。" },
  userWorks: { badge: "作品中心", description: "集中查看生成资产、缩略图、下载和编辑入口。" },
  userUsage: { badge: "使用记录", description: "按模型、时间和点数查看生成消耗明细，支撑不同模型扣不同点数。" },
  userMembership: { badge: "会员订单", description: "查看点数账户、套餐权益和交易状态。" },
  analysis: { badge: "数据分析", description: "按客户增长、交易收入、积分消耗和生成任务活跃度观察平台经营状态。" },
  workbench: { badge: "运营工作台", description: "聚合待办、快捷入口和平台健康状态，帮助主控团队快速处理日常运营动作。" },
  dashboard: { badge: "运营驾驶舱", description: "汇总客户、订单、渠道、用量和上游服务状态，帮助主控端快速判断平台健康度。" },
  customers: { badge: "客户资产", description: "管理客户账号、套餐、点数、状态和角色，支撑 SaaS 客户全生命周期运营。" },
  channels: { badge: "渠道网络", description: "维护一级/二级代理商、邀请码和启停状态，承接代理分销体系。" },
  products: { badge: "产品矩阵", description: "配置 AI 产品能力、权益和状态，让移动端与管理后台共享统一商品口径。" },
  plans: { badge: "商业套餐", description: "维护套餐价格、赠送点数、并发和有效期，作为订单与权益结算基础。" },
  orders: { badge: "交易履约", description: "跟踪订单、收款、续费和权益发放，关键动作由后端事务保证一致性。" },
  usage: { badge: "消耗分析", description: "查看模型、素材、生成任务等使用量，为成本和定价提供依据。" },
  commissions: { badge: "结算中心", description: "处理代理分润、提现审核和结算状态，形成渠道财务闭环。" },
  apiSettings: { badge: "模型网关", description: "管理上游模型渠道、API Key、客户分组和计费倍率。" },
  system: { badge: "系统治理", description: "沉淀品牌、密钥、模型、审计和运维配置，避免继续依赖临时聚合结构。" },
  departments: { badge: "组织权限", description: "维护主控后台部门结构，为账号权限和操作审计提供组织归属。" },
  userManagement: { badge: "账号权限", description: "管理平台账号、角色、状态和联系方式，对齐 RBAC 权限治理入口。" },
  menuManagement: { badge: "菜单权限", description: "维护后台菜单入口、模块可见性和权限点，支撑主控 SaaS 权限配置。" },
  partnerDashboard: { badge: "代理经营", description: "汇总代理商客户、订单、佣金、推广渠道和转化状态，形成经营驾驶舱。" },
  partnerCustomers: { badge: "客户资产", description: "查看通过邀请码绑定的客户、状态和来源，辅助跟进转化。" },
  partnerOrders: { badge: "订单跟进", description: "聚合代理商名下待支付、已成交和待续费订单，提升回款效率。" },
  partnerCommissions: { badge: "佣金结算", description: "查看分佣明细、结算状态、可提现金额和提现记录。" },
  partnerChannels: { badge: "推广渠道", description: "管理邀请码、下级代理和渠道转化表现。" },
  partnerMaterials: { badge: "素材中心", description: "沉淀推广海报、话术和专属链接，提升获客转化。" },
  partnerAccount: { badge: "账户设置", description: "维护代理商资料、收款信息和通知偏好。" }
};

const quickTodos = [
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
const aiDraftReferenceMaxBytes = 2_500_000;
const onlineReferenceSlots = [1, 2, 3];
const onlineCountOptions = [1, 2, 3, 4];
const onlineStatusFilter = ref("ALL");
const aiPlaygroundMode = ref("gallery");
const aiPlaygroundModeOptions = [
  { label: "画廊", value: "gallery" },
  { label: "Agent", value: "agent" }
];
type AiReferenceImage = { id: string; name: string; url: string; file?: File };
type AiFavoriteCollection = { id: string; name: string; taskIds: string[]; createdAt?: string; updatedAt?: string };
type AiAgentMessage = { role: "user" | "assistant"; content: string; createdAt?: string };
type AiAgentConversation = { id: string; title: string; messages: AiAgentMessage[]; createdAt: string; updatedAt?: string };
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
const aiLightboxTaskId = ref("");
const aiLightboxViewportRef = ref<HTMLElement | null>(null);
const aiLightboxScale = ref(1);
const aiLightboxTx = ref(0);
const aiLightboxTy = ref(0);
const aiLightboxDragging = ref(false);
const aiImageContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  taskId: ""
});
const aiTaskClockNow = ref(Date.now());
let aiTaskClockTimer: number | null = null;
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
const apiDraggingProviderId = ref("");
const apiReorderingProviders = ref(false);
const apiProviderDraft = ref({
  id: "",
  name: "API",
  baseUrl: "https://api.example.com/v1",
  apiKey: "",
  protocol: "openai",
  imageRequestMode: "openai",
  priority: 50,
  imageModels: ["gpt-image-2"],
  chatModels: ["gpt-4o"],
  videoModels: ["veo3-fast"],
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
const onlineRecentTasks = computed<AdminRecord[]>(() => {
  const serverTasks = Array.isArray(onlineImageData.value.recentTasks) ? onlineImageData.value.recentTasks : [];
  if (!aiOptimisticTasks.value.length) return serverTasks;
  const serverIds = new Set(serverTasks.map((task) => String(task.id || "")));
  const pendingTasks = aiOptimisticTasks.value.filter((task) => !serverIds.has(String(task.id || "")));
  return [...pendingTasks, ...serverTasks];
});
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
      .filter((item) => item.url.startsWith("data:"))
      .map<CachedReferenceImage>((item) => ({ id: item.id, name: item.name, url: item.url })),
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
    aiReferenceImages.value = (draft.referenceImages || []).map((item) => ({
      id: item.id,
      name: item.name,
      url: item.url
    }));
  } catch {
    // IndexedDB is optional; the page should still work without it.
  } finally {
    aiImageDraftHydrated = true;
  }
}

watch(
  [
    onlineImageForm,
    aiReferenceImages,
    aiPlaygroundMode,
    aiGalleryFilter,
    aiFavoriteOnly,
    aiPromptSearch,
    aiActiveFavoriteCollectionId
  ],
  scheduleAiImageDraftSave,
  { deep: true }
);

const onlineProviderOptions = computed(() => {
  const items = onlineProviders.value.map((provider) => ({ label: String(provider.name || provider.id || "API 平台"), value: String(provider.id || provider.name || "") })).filter((item) => item.value);
  return items.length ? items : [{ label: "请先在主控 SaaS 配置 API 渠道", value: "" }];
});
const onlineProviderModels = computed<AdminRecord[]>(() => {
  const items: AdminRecord[] = [];
  const seen = new Set<string>();
  onlineProviders.value.forEach((provider) => {
    const providerId = String(provider.id || provider.name || "");
    const providerName = String(provider.name || provider.id || "API 平台");
    const models = Array.isArray(provider.models) ? provider.models : [];
    models.forEach((rawModel) => {
      const model = String(rawModel || "").trim();
      if (!model || seen.has(model)) return;
      seen.add(model);
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
const activeOnlineModel = computed(() => [...onlineProviderModels.value, ...onlineModels.value].find((model) => String(model.model || model.id) === onlineImageForm.value.model));
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
const onlinePreviewTask = computed(() => onlineRecentTasks.value.find((task) => String(task.status || "").toUpperCase() === "SUCCEEDED") || onlineRecentTasks.value[0]);
const onlinePreviewImage = computed(() => onlinePreviewTask.value ? aiTaskImageUrl(onlinePreviewTask.value) : "");
const onlinePreviewStatus = computed(() => {
  const status = String(onlinePreviewTask.value?.status || "PENDING").toUpperCase();
  if (status === "SUCCEEDED") return { label: "已完成", type: "success" as const };
  if (status === "RUNNING") return { label: "生成中", type: "warning" as const };
  if (status === "FAILED") return { label: "失败", type: "danger" as const };
  return { label: "队列中", type: "info" as const };
});
const onlineHistoryItems = computed(() => onlineRecentTasks.value.slice(0, 8));
const aiGalleryCards = computed(() => {
  const keyword = aiPromptSearch.value.trim().toLowerCase();
  const filter = aiGalleryFilter.value;
  return onlineRecentTasks.value.filter((task) => {
    const taskId = String(task.id || task.name || task.prompt || "");
    if (aiHiddenTaskIds.value.includes(taskId)) return false;
    const status = String(task.status || "").toUpperCase();
    const searchable = [task.prompt, task.name, task.model, task.status, task.id].map((item) => String(item || "").toLowerCase()).join(" ");
    if (filter === "done" && status !== "SUCCEEDED") return false;
    if (filter === "running" && !["PENDING", "RUNNING"].includes(status)) return false;
    if (filter === "error" && !["FAILED", "ERROR"].includes(status)) return false;
    if (aiActiveFavoriteCollectionId.value) {
      const collectionTaskIds = aiActiveFavoriteCollectionId.value === "all-favorites"
        ? aiFavoriteTaskIds.value
        : aiFavoriteCollections.value.find((item) => item.id === aiActiveFavoriteCollectionId.value)?.taskIds;
      if (!collectionTaskIds?.includes(taskId)) return false;
    }
    if (aiFavoriteOnly.value && !isAiTaskFavorite(task)) return false;
    return !keyword || searchable.includes(keyword);
  }).slice(0, 12);
});
const aiFavoriteCollectionCards = computed(() => {
  const allFavoriteTaskIds = Array.from(new Set(aiFavoriteTaskIds.value));
  return [
    { id: "all-favorites", name: "全部收藏", taskIds: allFavoriteTaskIds, virtual: true },
    ...aiFavoriteCollections.value.map((collection) => ({ ...collection, virtual: false }))
  ].map((collection) => {
    const taskIds = collection.id === "all-favorites" ? allFavoriteTaskIds : collection.taskIds;
    const tasks = onlineRecentTasks.value.filter((task) => taskIds.includes(aiTaskId(task)));
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
const aiDetailTask = computed(() => onlineRecentTasks.value.find((task) => aiTaskId(task) === aiDetailTaskId.value));
const aiLightboxTask = computed(() => onlineRecentTasks.value.find((task) => aiTaskId(task) === aiLightboxTaskId.value));
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
  if (status === "ALL") return onlineRecentTasks.value;
  return onlineRecentTasks.value.filter((task) => String(task.status || "").toUpperCase() === status);
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
    selectAdminModule("userCanvas");
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

function aiTaskOriginalCacheId(task: AdminRecord) {
  const asset = aiTaskAsset(task);
  return String(asset?.id || task.id || task.resultUrl || task.outputUrl || task.imageUrl || "");
}

function aiTaskImageUrl(task: AdminRecord) {
  const directUrl = String(task.outputUrl || task.resultUrl || task.imageUrl || "");
  if (directUrl) return directUrl;
  const asset = aiTaskAsset(task);
  return String(asset?.url || asset?.imageUrl || asset?.outputUrl || asset?.resultUrl || task.thumbnailUrl || asset?.thumbnailUrl || "");
}

function aiTaskThumbnailUrl(task: AdminRecord) {
  const directUrl = String(task.thumbnailUrl || "");
  if (directUrl) return directUrl;
  const asset = aiTaskAsset(task);
  return String(asset?.thumbnailUrl || asset?.url || task.imageUrl || task.outputUrl || task.resultUrl || "");
}

function aiTaskDisplayImageUrl(task: AdminRecord) {
  const cacheId = aiTaskOriginalCacheId(task);
  return (cacheId && aiOriginalImageCache.value[cacheId]) || aiTaskImageUrl(task);
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
  const token = window.localStorage.getItem("token") || window.sessionStorage.getItem("token") || "";
  let response: Response;
  try {
    response = await fetch(directUrl);
    if (!response.ok) throw new Error(`direct image fetch returned ${response.status}`);
  } catch {
    if (!assetId) throw new Error("原图缓存失败");
    response = await fetch(`/api/v1/assets/${encodeURIComponent(assetId)}/download`, {
      headers: token ? { Authorization: `Bearer ${token}` } : undefined
    });
  }
  if (!response.ok) throw new Error("原图缓存失败");
  const blob = await response.blob();
  return blobToDataUrl(blob);
}

async function ensureAiOriginalImageCached(task: AdminRecord) {
  const cacheId = aiTaskOriginalCacheId(task);
  if (!cacheId || aiOriginalImageCache.value[cacheId] || aiOriginalImageCachePending.has(cacheId)) return;
  aiOriginalImageCachePending.add(cacheId);
  try {
    const cached = await readCachedOriginalImage(cacheId);
    if (cached?.dataUrl) {
      aiOriginalImageCache.value = { ...aiOriginalImageCache.value, [cacheId]: cached.dataUrl };
      return;
    }
    const dataUrl = await fetchAiOriginalImageDataUrl(task);
    if (!dataUrl) return;
    aiOriginalImageCache.value = { ...aiOriginalImageCache.value, [cacheId]: dataUrl };
    await writeCachedOriginalImage({ id: cacheId, dataUrl, sourceUrl: aiTaskImageUrl(task) });
  } catch (error) {
    console.warn("AI original image cache skipped", error);
  } finally {
    aiOriginalImageCachePending.delete(cacheId);
  }
}

function prefetchAiOriginalImage(task: AdminRecord) {
  if (!aiTaskImageUrl(task)) return;
  void ensureAiOriginalImageCached(task);
}

function aiTaskParams(task: AdminRecord) {
  return (task.params && typeof task.params === "object" ? task.params : {}) as Record<string, unknown>;
}

function aiTaskStatus(task: AdminRecord) {
  return String(task.status || "PENDING").toUpperCase();
}

function isAiTaskRunning(task: AdminRecord) {
  return ["PENDING", "RUNNING"].includes(aiTaskStatus(task));
}

function isAiTaskFailed(task: AdminRecord) {
  return ["FAILED", "ERROR"].includes(aiTaskStatus(task));
}

function aiTaskStatusClass(task: AdminRecord) {
  const status = aiTaskStatus(task);
  return {
    "is-running": ["PENDING", "RUNNING"].includes(status),
    "is-done": status === "SUCCEEDED",
    "is-failed": ["FAILED", "ERROR"].includes(status),
    "is-favorite": isAiTaskFavorite(task)
  };
}

function aiTaskModelLabel(task: AdminRecord) {
  const params = aiTaskParams(task);
  return String(task.model || params.model || onlineImageForm.value.model || "gpt-image-2");
}

function aiTaskErrorMessage(task: AdminRecord) {
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
  ElMessage.success(aiFavoritePickerCheckedIds.value.length ? "已保存到收藏夹" : "已从收藏夹移除");
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
  aiSelectedTaskIds.value = Array.from(new Set([...aiSelectedTaskIds.value, ...aiGalleryCards.value.map(aiTaskId).filter(Boolean)]));
}

function invertAiVisibleTaskSelection() {
  const visibleIds = aiGalleryCards.value.map(aiTaskId).filter(Boolean);
  const next = aiSelectedTaskIds.value.filter((id) => !visibleIds.includes(id));
  visibleIds.forEach((id) => {
    if (!aiSelectedTaskIds.value.includes(id)) next.push(id);
  });
  aiSelectedTaskIds.value = next;
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
  const selectedTasks = onlineRecentTasks.value.filter((task) => aiSelectedTaskIds.value.includes(aiTaskId(task)) && aiTaskImageUrl(task));
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
  ElMessage.success("任务已删除");
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
  const selectedTasks = onlineRecentTasks.value.filter((task) => selectedTaskIds.includes(aiTaskId(task)) && aiTaskImageUrl(task));
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

function reuseAiTask(task: AdminRecord) {
  const prompt = String(task.prompt || task.name || "");
  if (prompt) onlineImageForm.value.prompt = prompt;
  if (task.model) onlineImageForm.value.model = String(task.model);
  const params = (task.params || {}) as Record<string, unknown>;
  if (params.size) onlineImageForm.value.size = String(params.size);
  if (params.quality || params.imageQuality) onlineImageForm.value.quality = String(params.quality || params.imageQuality);
  if (params.output_format) onlineImageForm.value.outputFormat = String(params.output_format);
  ElMessage.success("已复用任务参数");
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
    ElMessage.warning("图片生成完成后才能编辑输出");
    return;
  }
  if (aiReferenceImages.value.length >= 10) {
    ElMessage.warning("参考图最多上传 10 张");
    return;
  }
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    {
      id: `task-output-${Date.now()}`,
      name: `${aiTaskId(task) || "output"}-输出图`,
      url: imageUrl
    }
  ];
  reuseAiTask(task);
  ElMessage.success("已加入参考图，可继续编辑生成");
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
  const menuHeight = 148;
  const left = Math.min(event.clientX, Math.max(12, window.innerWidth - menuWidth - 12));
  const top = Math.min(event.clientY, Math.max(12, window.innerHeight - menuHeight - 12));
  aiImageContextMenu.value = {
    visible: true,
    x: left,
    y: top,
    taskId: aiTaskId(task)
  };
}

const aiContextMenuTask = computed(() => onlineRecentTasks.value.find((task) => aiTaskId(task) === aiImageContextMenu.value.taskId));

function handleAiImageContextMenuDismiss(event?: Event) {
  if (!aiImageContextMenu.value.visible) return;
  if (event?.target instanceof Element && event.target.closest(".ai-image-context-menu")) return;
  closeAiImageContextMenu();
}

async function imageUrlToBlob(url: string) {
  const response = await fetch(url);
  if (!response.ok) throw new Error("读取图片失败");
  return response.blob();
}

async function copyAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  const imageUrl = task ? aiTaskImageUrl(task) : "";
  if (!imageUrl) {
    ElMessage.warning("暂无可复制图片");
    return;
  }
  try {
    const blob = await imageUrlToBlob(imageUrl);
    if (!navigator.clipboard || typeof ClipboardItem === "undefined") throw new Error("当前浏览器不支持复制图片");
    await navigator.clipboard.write([new ClipboardItem({ [blob.type || "image/png"]: blob })]);
    ElMessage.success("图片已复制");
  } catch (error) {
    console.error(error);
    ElMessage.error(error instanceof Error ? error.message : "复制失败");
  }
}

async function downloadAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  await downloadAiTask(task);
}

async function editAiContextImage() {
  const task = aiContextMenuTask.value;
  closeAiImageContextMenu();
  const imageUrl = task ? aiTaskImageUrl(task) : "";
  if (!task || !imageUrl) {
    ElMessage.warning("暂无可编辑图片");
    return;
  }
  if (aiReferenceImages.value.length >= 10) {
    ElMessage.warning("参考图最多上传 10 张");
    return;
  }
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    {
      id: `context-${Date.now()}`,
      name: `${aiTaskId(task) || "output"}-输出图`,
      url: imageUrl
    }
  ];
  reuseAiTask(task);
  closeAiLightbox();
  ElMessage.success("已加入参考图，可继续编辑生成");
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

async function downloadUrl(url: string, fileName: string) {
  let objectUrl = "";
  const anchor = document.createElement("a");
  try {
    const token = window.localStorage.getItem("token") || window.sessionStorage.getItem("token") || "";
    const response = await fetch(url, {
      headers: token && url.startsWith("/") ? { Authorization: `Bearer ${token}` } : undefined
    });
    if (!response.ok) throw new Error(`download failed: ${response.status}`);
    const blob = await response.blob();
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
  const fileName = `ai-image-${aiTaskId(target) || Date.now()}.png`;
  try {
    await downloadUrl(imageUrl, fileName);
    ElMessage.success("已开始下载");
  } catch {
    ElMessage.error("下载失败，请稍后重试");
  }
}

function fileToDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(reader.error || new Error("读取图片失败"));
    reader.readAsDataURL(file);
  });
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
  let url = "";
  let cacheable = false;
  if (file.size <= aiDraftReferenceMaxBytes) {
    try {
      url = await fileToDataUrl(file);
      cacheable = true;
    } catch {
      // Keep the temporary preview URL when the browser cannot read the file.
    }
  }
  if (!url) url = URL.createObjectURL(file);
  aiReferenceImages.value = [
    ...aiReferenceImages.value,
    {
      id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
      name: uploadFile.name || file.name,
      url,
      ...(cacheable ? {} : { file })
    }
  ];
  if (!cacheable) ElMessage.info("参考图较大，本次可用但不会写入本地草稿缓存");
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
}

function removeAiReferenceImage(index: number) {
  const item = aiReferenceImages.value[index];
  revokeAiReferenceImage(item);
  aiReferenceImages.value = aiReferenceImages.value.filter((_, currentIndex) => currentIndex !== index);
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
  aiReferenceImages.value.forEach(revokeAiReferenceImage);
  aiReferenceImages.value = [];
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
  onlineSubmitting.value = true;
  try {
    await adminRequest({
      method: "POST",
      url: "/generation-tasks",
      data: {
        type: "TEXT_TO_IMAGE",
        prompt,
        model: onlineImageForm.value.model,
        params: {
          count: onlineImageForm.value.count,
          imageRatio: onlineImageForm.value.ratio,
          imageQuality: onlineImageForm.value.quality,
          provider: onlineImageForm.value.provider,
          resolution: onlineImageForm.value.resolution,
          width: onlineImageForm.value.width,
          height: onlineImageForm.value.height,
          sourceModule: "online-image"
        }
      }
    });
    onlineImageForm.value.prompt = "";
    ElMessage.success("在线生图任务已提交");
    await store.loadActiveModule();
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
    return;
  }
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
    onlineImageForm.value.prompt = "";
    ElMessage.success("Agent 已开始处理");
    void saveAiState();
    return;
  }
  onlineSubmitting.value = true;
  const imageSize = resolveAiImageSize();
  const requestParams = {
    n: onlineImageForm.value.count,
    count: onlineImageForm.value.count,
    size: onlineImageForm.value.size,
    imageRatio: onlineImageForm.value.ratio,
    imageQuality: onlineImageForm.value.quality,
    quality: onlineImageForm.value.quality,
    output_format: onlineImageForm.value.outputFormat,
    output_compression: onlineImageForm.value.outputFormat === "png" ? null : onlineImageForm.value.outputCompression,
    transparent_output: onlineImageForm.value.outputFormat === "png" ? onlineImageForm.value.transparentOutput : false,
    moderation: onlineImageForm.value.moderation,
    provider: onlineImageForm.value.provider,
    resolution: imageSize.resolution,
    width: imageSize.width,
    height: imageSize.height,
    referenceImageCount: aiReferenceImages.value.length,
    referenceImageNames: aiReferenceImages.value.map((item) => item.name),
    sourceModule: "ai-image"
  };
  const optimisticTaskId = `optimistic_${Date.now()}`;
  const optimisticCreatedAt = new Date().toISOString();
  aiPlaygroundMode.value = "gallery";
  aiGalleryFilter.value = "all";
  aiFavoriteOnly.value = false;
  aiActiveFavoriteCollectionId.value = "";
  aiPromptSearch.value = "";
  aiOptimisticTasks.value = [{
    id: optimisticTaskId,
    userId: "user_000002",
    type: "TEXT_TO_IMAGE",
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
  try {
    await adminRequest({
      method: "POST",
      url: "/generation-tasks",
      data: {
        type: "TEXT_TO_IMAGE",
        prompt,
        model: onlineImageForm.value.model,
        params: requestParams
      }
    });
    onlineImageForm.value.prompt = "";
    if (aiSettingsDraft.value.clearInputAfterSubmit) clearAiReferenceImages();
    ElMessage.success("AI 生图任务已提交");
    await store.loadActiveModule();
    aiOptimisticTasks.value = aiOptimisticTasks.value.filter((task) => String(task.id || "") !== optimisticTaskId);
  } catch (error) {
    const message = error instanceof Error ? error.message : "提交失败";
    aiOptimisticTasks.value = aiOptimisticTasks.value.map((task) => String(task.id || "") === optimisticTaskId
      ? { ...task, status: "FAILED", error: { message }, progress: 100, updatedAt: new Date().toISOString() }
      : task);
    ElMessage.error(message);
  } finally {
    onlineSubmitting.value = false;
  }
}
interface AdminUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  planId?: string;
  subscriptionExpiresAt?: string;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("en-US").format(Math.max(0, Math.round(value)));
}

const sidebarPlan = computed(() => {
  const summary = (store.data?.summary || {}) as Record<string, unknown>;
  const planId = String(currentAdmin.value?.planId || summary.planId || "plan_year");
  const rawAvailable = Number(summary.availablePoints ?? summary.pointsAvailable ?? store.data?.availablePoints ?? store.data?.pointsAvailable ?? 28560);
  const planTotalMap: Record<string, number> = { plan_free: 100, plan_month: 3000, plan_year: 50000 };
  const planNameMap: Record<string, string> = { plan_free: "免费版", plan_month: "专业版（月付）", plan_year: "专业版（年付）" };
  const total = Math.max(rawAvailable, planTotalMap[planId] || 50000);
  const expiresAt = String(currentAdmin.value?.subscriptionExpiresAt || summary.subscriptionExpiresAt || "2026-07-19").slice(0, 10);
  return {
    name: planNameMap[planId] || "专业版（年付）",
    status: "使用中",
    expiresAt,
    availableText: formatNumber(rawAvailable),
    totalText: formatNumber(total),
    percent: Math.min(100, Math.max(4, Math.round((rawAvailable / Math.max(1, total)) * 100)))
  };
});
const currentAdmin = ref<AdminUser | null>(null);
const isAgentConsole = ref(typeof window !== "undefined" && window.location.pathname.startsWith("/agent"));
const isUserConsole = ref(typeof window !== "undefined" && (window.location.pathname.startsWith("/user") || window.location.pathname.startsWith("/app")));
const mobileDrawerOpen = ref(false);
const desktopSidebarCollapsed = ref(false);
const tabsScrollRef = ref<HTMLElement | null>(null);
const openTabStorageKey = isUserConsole.value ? "xianzhi-user-open-tabs" : isAgentConsole.value ? "xianzhi-agent-open-tabs" : "xianzhi-admin-open-tabs";
const activeTabStorageKey = isUserConsole.value ? "xianzhi-user-active-tab" : isAgentConsole.value ? "xianzhi-agent-active-tab" : "xianzhi-admin-active-tab";
const agentModuleIds = ["partnerDashboard", "partnerCustomers", "partnerOrders", "partnerCommissions", "partnerChannels", "partnerMaterials", "partnerAccount"];
const userModuleIds = ["userDashboard", "userOnlineImage", "userAiImage", "userCanvas", "userApiSettings", "userWorks", "userUsage", "userMembership"];
const adminModuleIds = modules.map((item) => item.id).filter((id) => !agentModuleIds.includes(id) && !userModuleIds.includes(id));
const allowedModuleIds = isUserConsole.value ? userModuleIds : isAgentConsole.value ? agentModuleIds : adminModuleIds;
const defaultOpenTabIds = isUserConsole.value ? ["userDashboard", "userOnlineImage", "userAiImage", "userCanvas"] : isAgentConsole.value ? ["partnerDashboard", "partnerCustomers"] : ["analysis", "workbench"];
store.activeModuleId = defaultOpenTabIds[0];

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
  analysis: DataAnalysis,
  workbench: House,
  dashboard: DataAnalysis,
  customers: User,
  channels: Operation,
  products: Goods,
  plans: Tickets,
  orders: Money,
  usage: DataAnalysis,
  commissions: Wallet,
  apiSettings: Setting,
  system: Setting,
  departments: Collection,
  userManagement: UserFilled,
  menuManagement: Operation,
  partnerDashboard: DataAnalysis,
  partnerCustomers: User,
  partnerOrders: Money,
  partnerCommissions: Wallet,
  partnerChannels: Connection,
  partnerMaterials: Collection,
  partnerAccount: Setting,
  userDashboard: House,
  userOnlineImage: Monitor,
  userAiImage: Plus,
  userCanvas: DataAnalysis,
  userApiSettings: Setting,
  userWorks: Collection,
  userUsage: DataAnalysis,
  userMembership: Tickets
};

const columnLabels: Record<string, string> = {
  id: "ID",
  category: "类型",
  secret: "密钥",
  name: "名称",
  email: "邮箱",
  role: "角色",
  plan: "套餐",
  planId: "套餐 ID",
  pointsAvailable: "余额",
  status: "状态",
  level: "等级",
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
  thumbnailUrl: "缩略图"
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
  apiProviderDraft.value = {
    id: String(channel.id || ""),
    name: String(channel.name || "API"),
    baseUrl: String(channel.baseUrl || "https://api.example.com/v1"),
    apiKey: "",
    protocol: String(channel.protocol || "openai"),
    imageRequestMode: String(channel.imageRequestMode || "openai"),
    priority: apiProviderPriority(channel),
    imageModels: models.filter((model) => /image|gpt|z-image|flux|banana/i.test(model)).slice(0, 6),
    chatModels: models.filter((model) => /gpt|qwen|chat|llm|claude/i.test(model)).slice(0, 6),
    videoModels: models.filter((model) => /veo|sora|video|seedance/i.test(model)).slice(0, 6),
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
    jimeng: "即梦 CLI"
  };
  return labels[protocol] || protocol || "OpenAI 直连";
}

function apiImageRequestModeDisplayLabel(mode: string) {
  return mode === "openai-json" ? "OpenAI JSON" : "OpenAI 标准";
}

function inferApiProviderProtocol(baseUrl: string, currentProtocol: string, currentMode: string) {
  const value = baseUrl.toLowerCase();
  const manuallyPinned = ["gemini", "volcengine", "runninghub", "jimeng"].includes(currentProtocol);
  if (manuallyPinned) {
    return { protocol: currentProtocol, imageRequestMode: currentMode || "openai" };
  }
  if (/apimart/.test(value)) return { protocol: "apimart", imageRequestMode: "openai-json" };
  if (/volces|volcengine|ark\.cn|ark\.volc/.test(value)) return { protocol: "volcengine", imageRequestMode: "openai-json" };
  if (/runninghub/.test(value)) return { protocol: "runninghub", imageRequestMode: "openai-json" };
  if (/generativelanguage|googleapis|gemini/.test(value)) return { protocol: "gemini", imageRequestMode: "openai-json" };
  if (/jimeng|jianying|capcut/.test(value)) return { protocol: "jimeng", imageRequestMode: "openai-json" };
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

async function ensureApiProviderDraftSaved() {
  const baseUrl = normalizeApiBaseUrl(apiProviderDraft.value.baseUrl || "");
  if (!baseUrl || !isValidApiBaseUrl(baseUrl)) {
    throw new Error("请先填写正确的平台地址，例如 https://api.example.com/v1");
  }
  apiProviderDraft.value.baseUrl = baseUrl;
  const savedItem = await saveApiProviderDraft({ silent: true });
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
    const savedItem = await ensureApiProviderDraftSaved();
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
    const savedItem = await ensureApiProviderDraftSaved();
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
    await saveApiProviderDraft({ silent: true });
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

async function saveApiProviderDraft(options: { silent?: boolean } = {}) {
  if (apiSavingProviderDraft.value) return;
  apiSavingProviderDraft.value = true;
  const draft = {
    ...apiProviderDraft.value,
    imageModels: [...apiProviderDraft.value.imageModels],
    chatModels: [...apiProviderDraft.value.chatModels],
    videoModels: [...apiProviderDraft.value.videoModels]
  };
  const selectedModels = [...draft.imageModels, ...draft.chatModels, ...draft.videoModels].filter(Boolean);
  const models = selectedModels.length ? selectedModels : [...apiFetchedModelIds.value].filter(Boolean);
  const hasKey = Boolean(draft.apiKey.trim() || selectedApiReferenceChannel.value.apiKeyConfigured);
  const payload = apiChannelMutationPayload({
    id: draft.id,
    name: draft.name,
    baseUrl: draft.baseUrl,
    protocol: draft.protocol,
    imageRequestMode: draft.imageRequestMode,
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
      await adminRequest({
        method: "POST",
        url: "/admin/api/keys",
        data: { customer: draft.name, status: "ACTIVE", quotaLimit: 100000, models, secret: apiKey, apiKey }
      });
    }
    await store.loadActiveModule();
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
      hydrateApiProviderDraft(apiReferenceChannels.value[nextIndex] || {});
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
    volcengine: "火山方舟"
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
const partnerModuleIds = ["partnerDashboard", "partnerCustomers", "partnerOrders", "partnerCommissions", "partnerChannels", "partnerMaterials", "partnerAccount"];
const isPartnerModule = computed(() => partnerModuleIds.includes(store.activeModuleId));
const partnerData = computed(() => store.data as AdminRecord & {
  user?: AdminRecord;
  agent?: AdminRecord;
  summary?: Record<string, unknown>;
  customers?: AdminRecord[];
  commissions?: AdminRecord[];
  withdrawals?: AdminRecord[];
  children?: AdminRecord[];
});

function moneyYuan(value: unknown) {
  return `￥${(Number(value || 0) / 100).toFixed(2)}`;
}

function partnerSummaryValue(key: string) {
  return partnerData.value.summary?.[key] ?? 0;
}

const partnerDashboardMetrics = computed(() => {
  const directCustomers = Number(partnerSummaryValue("directCustomers"));
  const childAgents = Number(partnerSummaryValue("childAgents"));
  const totalCommission = Number(partnerSummaryValue("totalCommission"));
  const availableToWithdraw = Number(partnerSummaryValue("availableToWithdraw"));
  const pendingCommission = Number(partnerSummaryValue("pendingCommission"));
  const commissions = Array.isArray(partnerData.value.commissions) ? partnerData.value.commissions : [];
  return [
    { label: "今日新增客户", value: String(Math.max(1, Math.round(directCustomers / 3))), hint: "代理获客口径" },
    { label: "有效客户", value: String(directCustomers), hint: "已绑定客户" },
    { label: "待支付订单", value: String(commissions.filter((item) => String(item.status || "").toUpperCase() === "PENDING").length), hint: "需要跟进" },
    { label: "累计佣金", value: moneyYuan(totalCommission), hint: "历史分佣" },
    { label: "可提现金额", value: moneyYuan(availableToWithdraw), hint: "可申请提现" },
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
  const agent = partnerData.value.agent || {};
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
  const agent = data.agent || {};
  const customers = Array.isArray(data.customers) ? data.customers : [];
  const commissions = Array.isArray(data.commissions) ? data.commissions : [];
  const withdrawals = Array.isArray(data.withdrawals) ? data.withdrawals : [];
  const children = Array.isArray(data.children) ? data.children : [];
  if (store.activeModuleId === "partnerCustomers") return customers;
  if (store.activeModuleId === "partnerOrders") return commissions.map((item) => ({ id: item.orderId || item.id, orderId: item.orderId, amountCents: item.amountCents, commissionCents: item.amountCents, status: item.status || "PENDING", createdAt: item.createdAt || "-" }));
  if (store.activeModuleId === "partnerCommissions") return [
    ...commissions.map((item) => ({ ...item, recordType: "佣金" })),
    ...withdrawals.map((item) => ({ ...item, recordType: "提现", orderId: "-", rate: "-" }))
  ];
  if (store.activeModuleId === "partnerChannels") return [{ ...agent, channel: "直属推广", customers: customers.length, children: children.length }, ...children];
  if (store.activeModuleId === "partnerMaterials") return [
    { id: "invite-link", name: "专属邀请链接", type: "链接", value: `/register?invite=${agent.inviteCode || ""}`, status: "ACTIVE" },
    { id: "poster", name: "朋友圈推广海报", type: "海报", value: "可复制邀请码与二维码", status: "ACTIVE" },
    { id: "script", name: "客户转化话术", type: "话术", value: "适合私域跟进和社群转化", status: "ACTIVE" }
  ];
  if (store.activeModuleId === "partnerAccount") return [
    { id: "profile", item: "代理商账号", value: `${data.user?.name || "-"} / ${data.user?.email || "-"}`, status: data.user?.status || "-" },
    { id: "inviteCode", item: "邀请码", value: agent.inviteCode || "-", status: agent.status || "-" },
    { id: "withdraw", item: "可提现金额", value: moneyYuan(partnerSummaryValue("availableToWithdraw")), status: "ACTIVE" }
  ];
  return partnerSourceRows.value;
}
const activeModuleMeta = computed(() => pageMeta[store.activeModuleId] || { badge: "主控模块", description: "管理当前业务域的数据和动作。" });
const visibleModuleGroups = computed(() => isUserConsole.value ? userModuleGroups : isAgentConsole.value ? agentModuleGroups : adminModuleGroups);
const activeGroup = computed(() => visibleModuleGroups.value.find((group) => group.items.some((item) => item.id === store.activeModuleId)));
const activeGroupLabel = computed(() => activeGroup.value?.title || "工作台");
const activeGroupIcon = computed(() => activeGroup.value?.icon || House);
const isGroupActive = (group: { items: Array<{ id: string }> }) => group.items.some((item) => item.id === store.activeModuleId);

function toggleDesktopSidebar() {
  desktopSidebarCollapsed.value = !desktopSidebarCollapsed.value;
}

function scrollOpenTabs(direction: -1 | 1) {
  tabsScrollRef.value?.scrollBy({ left: direction * 260, behavior: "smooth" });
}

function ensureOpenTab(moduleId: string) {
  if (!allowedModuleIds.includes(moduleId)) return;
  const module = modules.find((item) => item.id === moduleId);
  if (!module || openTabs.value.some((item) => item.id === moduleId)) return;
  openTabs.value.push(module);
}

async function selectAdminModule(moduleId: string) {
  if (!allowedModuleIds.includes(moduleId)) return;
  mobileDrawerOpen.value = false;
  ensureOpenTab(moduleId);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(activeTabStorageKey, moduleId);
  }
  await store.selectModule(moduleId);
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

onMounted(() => {
  if (typeof window === "undefined") return;
  if (isUserConsole.value) void hydrateAiImageDraft();
  if (isUserConsole.value) hydrateAiSettingsFromStore();
  aiTaskClockTimer = window.setInterval(() => {
    aiTaskClockNow.value = Date.now();
  }, 1000);
  window.addEventListener("mousedown", handleAiImageContextMenuDismiss, true);
  window.addEventListener("wheel", handleAiImageContextMenuDismiss, true);
  window.addEventListener("scroll", handleAiImageContextMenuDismiss, true);
  window.addEventListener("resize", handleAiImageContextMenuDismiss);
  window.addEventListener("keydown", handleAiLightboxKeydown);
  const savedActiveTab = window.localStorage.getItem(activeTabStorageKey);
  if (savedActiveTab && allowedModuleIds.includes(savedActiveTab) && modules.some((item) => item.id === savedActiveTab)) {
    ensureOpenTab(savedActiveTab);
    void store.selectModule(savedActiveTab);
  }
});

onBeforeUnmount(() => {
  clearAiTaskLongPressTimer();
  if (aiImageDraftSaveTimer) {
    window.clearTimeout(aiImageDraftSaveTimer);
    aiImageDraftSaveTimer = null;
  }
  if (aiTaskClockTimer) {
    window.clearInterval(aiTaskClockTimer);
    aiTaskClockTimer = null;
  }
  if (typeof window !== "undefined") {
    window.removeEventListener("mousedown", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("wheel", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("scroll", handleAiImageContextMenuDismiss, true);
    window.removeEventListener("resize", handleAiImageContextMenuDismiss);
    window.removeEventListener("keydown", handleAiLightboxKeydown);
    document.body.style.overflow = "";
  }
  if (aiImageDraftHydrated) void writeAiImageDraft(aiImageDraftPayload()).catch(() => undefined);
});
function settingsList(key: string): AdminRecord[] {
  const value = (store.data as Record<string, unknown>)[key];
  return Array.isArray(value) ? (value as AdminRecord[]) : [];
}

const rows = computed(() => {
  if (Array.isArray(store.data)) return flattenRows(store.data as AdminRecord[]);
  const data = store.data as { items?: unknown[]; withdrawals?: unknown[]; apiChannels?: unknown[]; apiModels?: unknown[]; apiKeys?: unknown[]; customerGroups?: unknown[]; brand?: Record<string, unknown>; recentTasks?: unknown[]; recentAssets?: unknown[]; account?: AdminRecord };
  if (store.activeModuleId === "userDashboard") {
    const assets = Array.isArray(data.recentAssets) ? data.recentAssets : [];
    const tasks = Array.isArray(data.recentTasks) ? data.recentTasks : [];
    return flattenRows([...assets, ...tasks].slice(0, 10) as AdminRecord[]);
  }
  if (store.activeModuleId === "userApiSettings") {
    return systemRows(data);
  }
  if (store.activeModuleId === "userMembership") {
    return data.account ? [data.account] : [];
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
  const items = Array.isArray(data.items) ? data.items : Array.isArray(data.withdrawals) ? data.withdrawals : [];
  return flattenRows(items as AdminRecord[]);
});

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
  const keyword = searchKeyword.value.trim().toLowerCase();
  if (!keyword) return [];
  return modules.filter((item) => allowedModuleIds.includes(item.id)).filter((item) => {
    const meta = pageMeta[item.id];
    return [item.id, item.title, meta?.badge, meta?.description].some((value) => String(value || "").toLowerCase().includes(keyword));
  }).slice(0, 6);
});

const currentRecordResults = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase();
  if (!keyword) return [];
  return rows.value
    .filter((row) => Object.values(row).some((value) => String(Array.isArray(value) ? value.join(" ") : value ?? "").toLowerCase().includes(keyword)))
    .slice(0, 6)
    .map((row, index) => {
      const record = row as AdminRecord;
      const title = String(record["name"] || record["customer"] || record["email"] || record["id"] || record["item"] || `${store.activeModule.title}记录 ${index + 1}`);
      const desc = Object.entries(record).slice(0, 4).map(([key, value]) => `${columnLabels[key] || key}: ${formatCell(value, key)}`).join(" / ");
      return { key: `${store.activeModuleId}-${index}-${title}`, title, desc };
    });
});

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
    customers: ["name", "email", "sourceAgentName", "sourceInviteCode", "sourceChannelLevel", "sourceParentAgentName", "plan", "pointsAvailable", "status"],
    userManagement: ["name", "email", "role", "plan", "pointsAvailable", "status"],
    partnerCustomers: ["name", "email", "role", "status"],
    partnerOrders: ["orderId", "amountCents", "commissionCents", "status", "createdAt"],
    partnerCommissions: ["recordType", "id", "orderId", "amountCents", "rate", "status"],
    partnerChannels: ["name", "email", "level", "inviteCode", "customers", "children", "status"],
    partnerMaterials: ["name", "type", "value", "status"],
    partnerAccount: ["item", "value", "status"],
    channels: ["name", "email", "level", "inviteCode", "status"],
    products: ["name", "type", "status", "usage"],
    plans: ["name", "priceCents", "grantPoints", "durationDays", "concurrency", "active"],
    orders: ["id", "customer", "plan", "amountCents", "status", "createdAt"],
    usage: ["product", "metric", "usage", "costCents"],
    commissions: ["recordType", "id", "agentId", "orderId", "amountCents", "rate", "status"],
    system: ["category", "item", "value", "secret", "status"],
    userDashboard: ["name", "model", "status", "pointCost", "createdAt"],
    userOnlineImage: ["id", "model", "type", "pointCost", "status", "createdAt"],
    userAiImage: ["id", "model", "type", "pointCost", "status", "createdAt"],
    userCanvas: ["id", "model", "status", "pointCost", "createdAt"],
    userWorks: ["name", "mediaType", "model", "pointCost", "createdAt"],
    userApiSettings: ["category", "item", "value", "secret", "status"],
    userUsage: ["id", "model", "type", "status", "pointCost", "createdAt"],
    userMembership: ["id", "userId", "available", "frozen"]
  };
  const first = rows.value[0];
  const fields = preferred[store.activeModuleId] || (first ? Object.keys(first).slice(0, 8) : []);
  return first ? fields.filter((field) => field in first) : fields;
});

const metrics = computed(() => {
  const data = store.data as { metrics?: Array<{ label: string; value: unknown }>; summary?: Record<string, unknown> };
  if (["userDashboard", "userOnlineImage", "userAiImage"].includes(store.activeModuleId) && Array.isArray(data.metrics)) return data.metrics.map((item) => ({ label: item.label, value: String(item.value) }));
  if (store.activeModuleId === "apiSettings" || store.activeModuleId === "userApiSettings") {
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
    channels: [{ action: "createChannel", label: "新增代理商" }],
    orders: [{ action: "createOrder", label: "新建订单" }],
    commissions: [
      { action: "createCommission", label: "登记分润" },
      { action: "createWithdrawal", label: "申请提现" }
    ],
    apiSettings: [],
    system: [
      { action: "editSystem", label: "品牌域名" },
      { action: "importApiRecommendations", label: "导入推荐平台" },
      { action: "createApiChannel", label: "新增上游" },
      { action: "createApiKey", label: "新增 API Key" }
    ]
  };
  return actions[store.activeModuleId] || [];
});

const rowActions = computed(() => {
  const actions: Record<string, Array<{ action: string; label: string }>> = {
    customers: [{ action: "editCustomer", label: "编辑" }],
    userManagement: [{ action: "editCustomer", label: "编辑" }],
    channels: [{ action: "toggleChannel", label: "启停" }],
    products: [{ action: "editProduct", label: "编辑" }],
    plans: [{ action: "editPlan", label: "保存价格" }],
    orders: [
      { action: "markPaid", label: "标记收款" },
      { action: "renewOrder", label: "续费" }
    ],
    commissions: [
      { action: "approveWithdrawal", label: "通过" },
      { action: "rejectWithdrawal", label: "驳回" }
    ]
  };
  return actions[store.activeModuleId] || [];
});

function systemRows(data: { apiChannels?: unknown[]; apiModels?: unknown[]; apiKeys?: unknown[]; brand?: Record<string, unknown> }): AdminRecord[] {
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
  return [...brandRows, ...channels, ...models, ...keys];
}
function flattenRows(items: AdminRecord[]): AdminRecord[] {
  return items.flatMap((item) => [item, ...((item.children as AdminRecord[] | undefined) || []).map((child) => ({ ...child, name: `二级 - ${child.name || child.id}` }))]);
}

function formatCell(value: unknown, column: string) {
  if (["sourceAgentName", "sourceInviteCode", "sourceParentAgentName", "referredBy"].includes(column) && !value) return "未归因";
  if (Array.isArray(value)) return value.join("、");
  if (column.toLowerCase().includes("cents")) return `￥${(Number(value || 0) / 100).toFixed(2)}`;
  if (typeof value === "object" && value) return JSON.stringify(value);
  return value ?? "-";
}

function isStatusColumn(column: string) {
  return column.toLowerCase().includes("status") || column === "active";
}

function statusType(value: unknown) {
  const text = String(value).toUpperCase();
  if (["ACTIVE", "PAID", "APPROVED", "true"].includes(text)) return "success";
  if (["PENDING", "CONFIGURABLE"].includes(text)) return "warning";
  if (["DISABLED", "REJECTED", "FAILED", "false"].includes(text)) return "danger";
  return "info";
}

function visibleRowActions(row: AdminRecord) {
  if (store.activeModuleId !== "commissions") return rowActions.value;
  if (row.recordType === "提现" && String(row.status).toUpperCase() === "PENDING") return rowActions.value;
  return [];
}

function labelForRowAction(action: { action: string; label: string }, row: AdminRecord) {
  if (action.action === "toggleChannel") return String(row.status).toUpperCase() === "ACTIVE" ? "停用" : "启用";
  return action.label;
}

async function openCreateCustomerDialog() {
  const form = {
    name: "",
    email: `customer${Date.now()}@example.com`,
    role: "MEMBER",
    status: "ACTIVE",
    planId: "plan_free",
    referredBy: "",
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
        h("span", null, "客户角色"),
        h("select", {
          class: "channel-dialog-input",
          value: form.role,
          onChange: (event: Event) => {
            form.role = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "MEMBER" }, "普通会员"),
          h("option", { value: "ADMIN" }, "管理员"),
          h("option", { value: "FINANCE" }, "财务"),
          h("option", { value: "DELIVERY_MANAGER" }, "交付负责人")
        ])
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
        h("span", null, "来源代理用户 ID"),
        h("input", {
          class: "channel-dialog-input",
          value: form.referredBy,
          placeholder: "例如：user_000003，可留空",
          onInput: (event: Event) => {
            form.referredBy = (event.target as HTMLInputElement).value;
          }
        })
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
          role: form.role,
          status: form.status,
          planId: form.planId,
          referredBy: form.referredBy.trim(),
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
async function openEditCustomerDialog(row: AdminRecord) {
  const form = {
    name: String(row.name || ""),
    email: String(row.email || ""),
    role: String(row.role || "MEMBER"),
    status: String(row.status || "ACTIVE"),
    planId: String(row.planId || "plan_free"),
    referredBy: String(row.referredBy || ""),
    available: String(row.pointsAvailable ?? 0)
  };
  await ElMessageBox({
    title: "编辑客户",
    message: h("div", { class: "channel-dialog-form" }, [
      h("label", { class: "channel-dialog-field channel-dialog-field-wide" }, [
        h("span", null, "客户名称"),
        h("input", {
          class: "channel-dialog-input",
          value: form.name,
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
          onInput: (event: Event) => {
            form.email = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "客户角色"),
        h("select", {
          class: "channel-dialog-input",
          value: form.role,
          onChange: (event: Event) => {
            form.role = (event.target as HTMLSelectElement).value;
          }
        }, [
          h("option", { value: "MEMBER" }, "普通会员"),
          h("option", { value: "ADMIN" }, "管理员"),
          h("option", { value: "FINANCE" }, "财务"),
          h("option", { value: "CHANNEL_MANAGER" }, "渠道负责人"),
          h("option", { value: "DELIVERY_MANAGER" }, "交付负责人"),
          h("option", { value: "SUPER_ADMIN" }, "超级管理员")
        ])
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
        h("span", null, "来源代理用户 ID"),
        h("input", {
          class: "channel-dialog-input",
          value: form.referredBy,
          placeholder: "例如：user_000003，可留空",
          onInput: (event: Event) => {
            form.referredBy = (event.target as HTMLInputElement).value;
          }
        })
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "可用点数"),
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
      instance.confirmButtonLoading = true;
      try {
        await store.mutate("PATCH", `/admin/customers/${row.id}`, {
          name,
          email,
          role: form.role,
          status: form.status,
          planId: form.planId,
          referredBy: form.referredBy.trim(),
          available
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
          placeholder: "例如：华东一级代理",
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
        }, [
          h("option", { value: "1" }, "一级代理"),
          h("option", { value: "2" }, "二级代理")
        ])
      ]),
      h("label", { class: "channel-dialog-field" }, [
        h("span", null, "上级代理 ID"),
        h("input", {
          class: "channel-dialog-input",
          placeholder: "二级代理必填，如 channel_000001",
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
      if (level === 2 && !form.parentId.trim()) {
        ElMessage.error("二级代理必须填写上级代理 ID");
        return;
      }
      if (!Number.isFinite(available) || available < 0) {
        ElMessage.error("初始点数必须是大于等于 0 的数字");
        return;
      }
      instance.confirmButtonLoading = true;
      try {
        await store.mutate("POST", "/admin/channel-agents", {
          name,
          email,
          level,
          parentId: form.parentId.trim(),
          inviteCode: form.inviteCode.trim(),
          status: form.status,
          available
        });
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

const recommendedApiChannels: AdminRecord[] = [
  {
    name: "APIMart 生图聚合",
    baseUrl: "https://api.apimart.ai",
    protocol: "apimart",
    imageRequestMode: "openai-json",
    imageGenerationEndpoint: "/v1/images/generations",
    imageEditEndpoint: "/v1/images/edits",
    fetchModelsPath: "/v1/models",
    apiKeyEnv: "APIMART_API_KEY",
    status: "CONFIGURABLE",
    priority: 10,
    models: ["gpt-image-2", "nano-banana-edit", "veo3.1-fast"],
    notes: "参考 Infinite-Canvas 推荐平台，适合聚合图片、视频和 LLM 模型。"
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
    fetchModelsPath: String(channel.fetchModelsPath || "/models"),
    apiKeyEnv: String(channel.apiKeyEnv || ""),
    comfyInstances: Array.isArray(channel.comfyInstances) ? channel.comfyInstances : [],
    notes: String(channel.notes || ""),
    primary: Boolean(channel.primary),
    status: String(channel.status || "CONFIGURABLE"),
    priority: Number(channel.priority || 50),
    models: Array.isArray(channel.models) ? channel.models.map((model) => String(model)) : ["gpt-image-2", "mock-standard"]
  };
}

async function runAction(action: string, row: AdminRecord = {}) {
  try {
    if (action === "createCustomer") {
      await openCreateCustomerDialog();
    } else if (action === "editCustomer") {
      await openEditCustomerDialog(row);
    } else if (action === "createChannel") {
      await openCreateChannelDialog();
    } else if (action === "toggleChannel") {
      const status = String(row.status).toUpperCase() === "ACTIVE" ? "DISABLED" : "ACTIVE";
      await store.mutate("PATCH", `/admin/channel-agents/${row.id}`, { status });
    } else if (action === "editProduct") {
      const status = await ask("产品状态", String(row.status || "ACTIVE"));
      const name = await ask("产品名称", String(row.name || ""));
      await store.mutate("PATCH", `/admin/products/${row.id}`, { name, type: row.type, status, entitlements: Array.isArray(row.entitlements) ? row.entitlements : [] });
    } else if (action === "editPlan") {
      const priceCents = await askNumber("价格（分）", Number(row.priceCents || 0));
      const grantPoints = await askNumber("权益点数", Number(row.grantPoints || 0));
      await store.mutate("PATCH", `/admin/plans/${row.id}`, {
        name: row.name,
        priceCents,
        grantPoints,
        durationDays: Number(row.durationDays || 30),
        concurrency: Number(row.concurrency || 1),
        active: Boolean(row.active),
        entitlements: typeof row.entitlements === "object" && row.entitlements ? row.entitlements : {}
      });
    } else if (action === "createOrder") {
      const userId = await ask("客户 userId", "user_000002");
      const planId = await ask("套餐 ID", "plan_month");
      const amountCents = await askNumber("订单金额（分）", 9900);
      await store.mutate("POST", "/admin/orders", { userId, planId, amountCents, status: "PENDING" });
    } else if (action === "markPaid") {
      await store.mutate("POST", `/admin/orders/${row.id}/mark-paid`);
    } else if (action === "renewOrder") {
      await store.mutate("POST", `/admin/orders/${row.id}/renew`);
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
      await store.mutate("POST", `/admin/withdrawals/${row.id}/approve`);
    } else if (action === "rejectWithdrawal" && String(row.status).toUpperCase() === "PENDING") {
      await store.mutate("POST", `/admin/withdrawals/${row.id}/reject`);
    } else if (action === "editSystem") {
      const name = await ask("品牌名称", "先知 AI");
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
  return Boolean(localStorage.getItem("token") || sessionStorage.getItem("token"));
}

function redirectToLogin() {
  localStorage.removeItem("token");
  sessionStorage.removeItem("token");
  window.location.href = "/login";
}

async function loadCurrentAdmin() {
  if (!hasAuthToken()) {
    redirectToLogin();
    return false;
  }
  try {
    const response = await adminRequest<{ user: AdminUser }>({ method: "GET", url: "/auth/me" });
    currentAdmin.value = response.user;
    const role = String(response.user.role || "").toUpperCase();
    if (isUserConsole.value) return true;
    if (isAgentConsole.value && !role.startsWith("AGENT")) {
      ElMessage.error("当前账号不是代理商，请进入主控后台");
      window.location.href = "/admin/";
      return false;
    }
    if (!isAgentConsole.value && !isUserConsole.value && !role.includes("ADMIN") && !role.startsWith("AGENT")) {
      window.location.href = "/app";
      return false;
    }
    if (!isAgentConsole.value && !isUserConsole.value && role.startsWith("AGENT")) {
      window.location.href = "/agent/";
      return false;
    }
    return true;
  } catch {
    currentAdmin.value = null;
    redirectToLogin();
    return false;
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
  try {
    await adminRequest({ method: "POST", url: "/auth/logout" });
  } catch {
    // 本地退出优先，服务端 token 失效失败不阻塞退出。
  }
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
  if (await loadCurrentAdmin()) {
    await store.loadActiveModule();
  }
});
</script>























