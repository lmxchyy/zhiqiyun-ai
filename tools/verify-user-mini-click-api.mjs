import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const apiBaseURL = (process.env.XIANZHI_API_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const userEmail = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const userPassword = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";
const agentEmail = process.env.XIANZHI_VERIFY_AGENT_EMAIL || "agent1@xianzhi.ai";
const agentPassword = process.env.XIANZHI_VERIFY_AGENT_PASSWORD || "Agent123!";
const operationEmail = process.env.XIANZHI_VERIFY_OPERATION_EMAIL || "operation@xianzhi.ai";
const operationPassword = process.env.XIANZHI_VERIFY_OPERATION_PASSWORD || "Demo123!";
const centerFilter = String(process.env.XIANZHI_VERIFY_CENTER || "" ).trim();

const files = new Map();
function read(relativePath) {
  if (!files.has(relativePath)) {
    files.set(relativePath, fs.readFileSync(path.join(repoRoot, relativePath), "utf8"));
  }
  return files.get(relativePath);
}

function request(method, requestPath, token, body) {
  const url = new URL(requestPath, apiBaseURL);
  const transport = url.protocol === "https:" ? https : http;
  const payload = body === undefined ? undefined : JSON.stringify(body);
  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: {
        Accept: "application/json",
        ...(payload ? { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload) } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      timeout: 15000,
    }, (res) => {
      let data = "";
      res.setEncoding("utf8");
      res.on("data", chunk => { data += chunk; });
      res.on("end", () => {
        let parsed = data;
        try {
          parsed = data ? JSON.parse(data) : {};
        } catch {
          // Keep raw text for diagnostics.
        }
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          resolve({ status: res.statusCode, data: parsed });
        } else {
          const message = typeof parsed === "object" && parsed && "error" in parsed ? parsed.error : data;
          reject(new Error(`${method} ${requestPath} returned ${res.statusCode}: ${message || "empty response"}`));
        }
      });
    });
    req.on("timeout", () => {
      req.destroy(new Error(`${method} ${requestPath} timed out`));
    });
    req.on("error", reject);
    if (payload) req.write(payload);
    req.end();
  });
}

async function login(email, password) {
  const res = await request("POST", "/api/v1/auth/login", "", { email, password });
  const token = res.data?.accessToken || res.data?.token;
  if (!token) throw new Error(`login for ${email} did not return accessToken`);
  return token;
}

const srcWorkbench = "apps/user-uni/src/components/MiniProgramRoleWorkbench.vue";
const srcHome = "apps/user-uni/src/components/v531/V531HomePage.vue";
const srcNativePageBack = "apps/user-uni/src/components/NativePageBack.vue";
const srcAgentCreationPage = "apps/user-uni/src/pages/user/UserAgentCreationPage.vue";
const srcStudio = "apps/user-uni/src/components/v531/V531StudioPage.vue";
const srcAssets = "apps/user-uni/src/components/assets/AssetCenterPage.vue";
const srcAssetNativeBridge = "apps/user-uni/src/features/assets/nativeBridge.ts";
const srcAssetTypeTabs = "apps/user-uni/src/components/assets/AssetTypeTabs.vue";
const srcAssetStatusTabs = "apps/user-uni/src/components/assets/AssetStatusTabs.vue";
const srcMpPatch = "apps/user-uni/scripts/patch-mp-native-login.cjs";
const srcProfile = "apps/user-uni/src/components/v531/V531ProfilePage.vue";
const srcConfig = "apps/user-uni/src/config/v531.ts";
const srcPermissions = "apps/user-uni/src/config/permissions.ts";
const srcUserStore = "apps/user-uni/src/stores/user.ts";
const srcPermissionGuard = "apps/user-uni/src/router/permissionGuard.ts";
const srcForbidden = "apps/user-uni/src/pages/ForbiddenPage.vue";
const srcPages = "apps/user-uni/src/config/miniProgramPages.ts";
const srcRoleSdk = "packages/business-sdk/src/role-workbench.ts";
const srcGenerationSdk = "packages/business-sdk/src/generation.ts";
const srcAssetsSdk = "packages/business-sdk/src/assets.ts";
const srcProfileEdit = "apps/user-uni/src/pages/user/UserProfileEditPage.vue";
const srcInvoices = "apps/user-uni/src/pages/user/UserInvoicesPage.vue";
const srcOrders = "apps/user-uni/src/pages/user/UserOrdersPage.vue";
const srcSettings = "apps/user-uni/src/pages/user/UserSettingsPage.vue";
const srcAuthApi = "apps/user-uni/src/features/auth/api.ts";
const distWorkbench = "apps/user-uni/dist/build/mp-weixin/components/MiniProgramRoleWorkbench.js";
const distHome = "apps/user-uni/dist/build/mp-weixin/components/v531/V531HomePage.wxml";
const distHomeJs = "apps/user-uni/dist/build/mp-weixin/components/v531/V531HomePage.js";
const distNativePageBack = "apps/user-uni/dist/build/mp-weixin/components/NativePageBack.wxml";
const distConfig = "apps/user-uni/dist/build/mp-weixin/config/v531.js";
const distStudio = "apps/user-uni/dist/build/mp-weixin/components/v531/V531StudioPage.wxml";
const distStudioJs = "apps/user-uni/dist/build/mp-weixin/components/v531/V531StudioPage.js";
const distAssets = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetCenterPage.wxml";
const distAssetsJs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetCenterPage.js";
const distAssetTypeTabs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetTypeTabs.wxml";
const distAssetTypeTabsJs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetTypeTabs.js";
const distAssetStatusTabs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetStatusTabs.wxml";
const distAssetStatusTabsJs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetStatusTabs.js";
const distAssetEmptyState = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetEmptyState.wxml";
const distAssetEmptyStateJs = "apps/user-uni/dist/build/mp-weixin/components/assets/AssetEmptyState.js";
const distProfile = "apps/user-uni/dist/build/mp-weixin/components/v531/V531ProfilePage.wxml";
const distProfileJs = "apps/user-uni/dist/build/mp-weixin/components/v531/V531ProfilePage.js";
const distPermissions = "apps/user-uni/dist/build/mp-weixin/config/permissions.js";

const clickChecks = [
  {
    center: "home",
    component: "hero.prompt-submit",
    static: [
      [srcHome, ["always-embed", "class=\"hero-input-action submit\"", "@tap.stop=\"submitPrompt\"", "v531-creation-prompt", "正在进入创作", "openCreationMode(mode)"]],
      [srcWorkbench, ["const savedPrompt = String(uni.getStorageSync(\"v531-creation-prompt\")", "openCreation(mode: CreationMode)", "preloadCreationBackend(mode)"]],
    ],
    live: ["/api/v1/module-schema?module_code=image_generation"],
  },
  {
    center: "home",
    component: "capability-card-navigation",
    static: [
      [srcHome, [
        "@click=\"openCreationMode(item.routeMode)\"",
        "featuredCapabilities",
        "secondaryCapabilities",
        "compactCapabilities",
        "@click=\"openCreationMode('agent')\"",
        "miniProgramCreationPages[mode]",
        "navigateStandalone",
      ]],
      [srcPages, ["UserImageCreationPage", "UserVideoCreationPage", "UserPptCreationPage", "UserInfographicCreationPage", "UserAgentCreationPage"]],
    ],
  },
  {
    center: "home",
    component: "continue-work-navigation",
    static: [
      [srcHome, ["openUserTab(projectItems.value.length || (props.tasks || []).length ? \"assets\" : \"create\")", "function openProject(item:", "miniProgramFeaturePages.userAssetDetail"]],
      [srcPages, ["UserCreationPage", "UserAssetsPage", "UserAssetDetailPage"]],
    ],
  },
  {
    center: "home",
    component: "entry-return-navigation",
    static: [
      [srcWorkbench, ["class=\"v31-back-button\"", "returnToPreviousPage", "uni.navigateBack"]],
      [srcNativePageBack, ["open-type=\"switchTab\"", ":url=\"fallback\"", "/pages/user/UserHomePage"]],
      [srcAgentCreationPage, ["<NativePageBack fallback=\"/pages/user/UserHomePage\"", "UserKnowledgeAgentDetailPage"]],
      [srcMpPatch, ["nativeBackToCreation", "Expected 2 native back bindings"]],
      [distNativePageBack, ["open-type=\"switchTab\"", "url=\"{{a}}\""]],
      [srcPages, ["UserHomePage", "UserAgentCreationPage", "UserKnowledgeAgentDetailPage"]],
    ],
  },
  {
    center: "creation",
    component: "studio.banner.start",
    static: [[srcStudio, ["@click=\"startCreation\"", "persistDraft(mode)"]], [srcWorkbench, ["/api/v1/module-schema?module_code"]]],
    live: ["/api/v1/module-schema?module_code=image_generation"],
  },
  {
    center: "creation",
    component: "capability.image",
    static: [[srcStudio, ["id: \"image\"", "mode: \"image\"", "@click=\"openMode(item.mode)\""]], [srcWorkbench, ["/api/v1/module-schema?module_code"]]],
    live: ["/api/v1/module-schema?module_code=image_generation"],
  },
  {
    center: "creation",
    component: "capability.video",
    static: [[srcConfig, ["id: \"video\"", "title: \"AI视频\"", "routeMode: \"video\""]], [srcWorkbench, ["video_generation"]]],
    live: ["/api/v1/module-schema?module_code=video_generation"],
  },
  {
    center: "creation",
    component: "capability.ppt",
    static: [[srcConfig, ["id: \"ppt\"", "title: \"PPT生成\"", "routeMode: \"ppt\""]], [srcWorkbench, ["/api/v1/ppt/models/text", "/api/v1/ppt/models/image"]], [srcGenerationSdk, ["/api/v1/ppt/generate"]]],
    live: ["/api/v1/ppt/models/text", "/api/v1/ppt/models/image"],
  },
  {
    center: "creation",
    component: "capability.agent",
    static: [[srcConfig, ["id: \"agent\"", "title: \"数字员工\"", "routeMode: \"agent\""]], [srcWorkbench, ["/api/v1/knowledge-agents"]]],
    live: ["/api/v1/knowledge-agents"],
  },
  {
    center: "creation",
    component: "capability.chart",
    static: [[srcConfig, ["id: \"chart\"", "title: \"智能图表\"", "routeMode: \"infographic\""]], [srcWorkbench, ["creationMode.value === \"infographic\"", "businessSdk.generation.createTask"]], [srcGenerationSdk, ["/api/v1/generation-tasks"]]],
    live: ["/api/v1/module-schema?module_code=image_generation"],
  },
  {
    center: "creation",
    component: "capability.copywriting",
    static: [[srcConfig, ["id: \"copywriting\"", "title: \"文案创作\"", "routeMode: \"review\""]], [srcWorkbench, ["/api/v1/knowledge-agents", "/api/v1/knowledge-conversations"]]],
    live: ["/api/v1/knowledge-agents", "/api/v1/knowledge-conversations"],
  },
  {
    center: "creation",
    component: "capability.voice",
    static: [[srcConfig, ["id: \"voice\"", "title: \"AI配音\"", "routeMode: \"video\""]], [srcWorkbench, ["video_generation"]]],
    live: ["/api/v1/module-schema?module_code=video_generation"],
  },
  {
    center: "creation",
    component: "capability.knowledge",
    static: [[srcConfig, ["id: \"knowledge\"", "title: \"知识库\"", "routeMode: \"agent\""]], [srcWorkbench, ["/api/v1/knowledge-agents"]]],
    live: ["/api/v1/knowledge-agents"],
  },
  {
    center: "creation",
    component: "studio.categories-and-banner",
    static: [[srcStudio, ["prompt-composer", "chooseReferenceImage", "chooseFile", "startCreation"]], [srcWorkbench, ["preloadCreationBackend"]]],
    live: ["/api/v1/module-schema?module_code=image_generation"],
  },
  {
    center: "creation",
    component: "studio.marketing-scenarios",
    static: [[srcStudio, ["const scenes =", "applyScene(item)", "小红书爆款", "朋友圈海报", "企业宣传"]], [srcWorkbench, ["preloadCreationBackend"]]],
    live: ["/api/v1/module-schema?module_code=image_generation", "/api/v1/module-schema?module_code=video_generation", "/api/v1/ppt/models/text", "/api/v1/knowledge-agents"],
  },
  {
    center: "creation",
    component: "studio.popular-templates",
    static: [[srcStudio, ["V532SceneCenter", "activeView = 'scenes'", "applyScenePreset"]], [srcWorkbench, ["preloadCreationBackend"]]],
    live: ["/api/v1/module-schema?module_code=image_generation", "/api/v1/ppt/models/text"],
  },
  {
    center: "works",
    component: "assets.initial-list",
    static: [[srcWorkbench, ["loadAssets(false)"]], [srcRoleSdk, ["/api/v1/assets"]], [srcAssets, ["store.refreshAssets(4)", "AssetGrid", "store.assets.slice(0,4)"]]],
    live: ["/api/v1/assets"],
  },
  {
    center: "works",
    component: "assets.search-and-filter",
    static: [[srcAssets, ["searchVisible", "applySearch", "changeType", "changeStatus", "AssetFilterDrawer"]], [srcRoleSdk, ["/api/v1/assets"]]],
    live: ["/api/v1/assets"],
  },
  {
    center: "works",
    component: "assets.native-mini-bindings",
    static: [
      [srcAssets, ["registerAssetNativeBridge", "handleNativeEmptyAction", "miniProgramRolePages.user.create", "uni.switchTab", "openAllAssets"]],
      [srcAssetNativeBridge, ["__xianzhiAssetNativeBridge", "setType", "setStatus", "emptyAction"]],
      [srcAssetTypeTabs, ["data-asset-value", "update:modelValue"]],
      [srcAssetStatusTabs, ["data-asset-value", "update:modelValue"]],
      [srcMpPatch, ["nativeAssetTypeSelect", "nativeAssetStatusSelect", "nativeAssetOpenAll", "nativeAssetEmptyAction"]],
    ],
    live: ["/api/v1/assets"],
  },  {
    center: "works",
    component: "assets.card-detail",
    static: [[srcAssets, ["@open=\"openAsset\"", "openDetail(asset)", "miniProgramFeaturePages.userAssetDetail"]], [srcWorkbench, ["openAssetDetail"]], [srcAssetsSdk, ["/api/v1/assets"]], [srcPages, ["userAssetDetail"]]],
    live: ["/api/v1/assets"],
  },
  {
    center: "works",
    component: "assets.batch-manager",
    static: [[srcAssets, ["openManage", "manage=1", "AssetActionSheet"]], [srcGenerationSdk, ["/api/v1/generation-tasks"]]],
    live: ["/api/v1/assets", "/api/v1/generation-tasks"],
  },
  {
    center: "works",
    component: "assets.retry-and-create",
    static: [[srcAssets, ["@retry=\"refresh\"", "emit(\"create\")"]], [srcWorkbench, ["@create=\"selectUserTab('create')"]]],
    live: ["/api/v1/assets"],
  },
  {
    center: "mine",
    component: "profile.initial-load",
    static: [[srcWorkbench, ["loadMemberProfile()", "loadWallet()", "loadAssets(false)"]], [srcRoleSdk, ["/api/v1/member/profile", "/api/v1/member/wallet", "/api/v1/points/account"]]],
    live: ["/api/v1/member/profile", "/api/v1/member/wallet", "/api/v1/points/account"],
  },
  {
    center: "mine",
    component: "profile.notifications",
    static: [[srcWorkbench, ["showNotifications", "/api/v1/user/dashboard"]]],
    live: ["/api/v1/user/dashboard"],
  },
  {
    center: "mine",
    component: "profile.edit",
    static: [[srcProfile, ["$emit('edit')"]], [srcProfileEdit, ["/api/v1/member/profile", "method: \"PATCH\""]]],
    live: ["/api/v1/member/profile"],
  },
  {
    center: "mine",
    component: "profile.resources-and-recharge",
    static: [[srcProfile, ["profile-v55-wallet-card", "$emit('recharge')", "$emit('service', 'wallet')", "$emit('service', 'usage')"]], [srcWorkbench, ["monthlyGrantedPoints", "monthlyPointCost", "/api/v1/plans?planType=recharge", "/api/v1/points/recharge-orders"]]],
    live: ["/api/v1/plans?planType=recharge"],
  },
  {
    center: "mine",
    component: "service.points-and-usage",
    static: [[srcProfile, ["$emit('service', 'usage')", "查看明细"]], [srcWorkbench, ["points: () => openMineView(\"usage-details\")", "usage: () => openMineView(\"usage-details\")", "/api/v1/user/usage"]]],
    live: ["/api/v1/user/usage"],
  },
  {
    center: "mine",
    component: "workbench.assets-and-tasks",
    static: [[srcProfile, ["id: \"assets\"", "id: \"tasks\"", "AI资产", "最近任务"]], [srcWorkbench, ["assets: () => selectUserTab(\"assets\")", "tasks: openTaskRecords"]], [srcRoleSdk, ["/api/v1/assets"]]],
    live: ["/api/v1/assets", "/api/v1/generation-tasks"],
  },
  {
    center: "mine",
    component: "service.company-and-settings",
    static: [[srcProfile, ["$emit('service', 'company')", "$emit('service', 'settings')", "企业中心", "设置中心"]], [srcWorkbench, ["company: () => openStandalonePage(miniProgramEnterprisePages.entry)", "settings: () => openFeaturePage(miniProgramFeaturePages.userSettings)"]], [srcSettings, ["loginAPI.changePassword", "/api/v1/auth/logout"]], [srcAuthApi, ["/api/v1/auth/change-password"]]],
    live: ["/api/v1/member/profile"],
  },
  {
    center: "mine",
    component: "service.role-functions",
    static: [[srcProfile, ["RoleMenuConfig[props.currentRole]", "hasPermission(item.permission)", "角色功能"]], [srcPermissions, ["agent:promotion", "agent:withdraw", "operation:dashboard", "operation:agent:list"]], [srcWorkbench, ["userStore.switchRole", "upgrade-agent", "agent-promotion", "operation-agents"]]],
    live: ["/api/v1/member/profile", "/api/v1/user/profile"],
  },
  {
    center: "authorization",
    component: "role-switch-and-route-guard",
    static: [[srcUserStore, ["roles: this.roles", "currentRole: this.currentRole", "/api/v1/user/current-role", "hasPermission"]], [srcPermissionGuard, ["operation:agent:list", "agent:promotion", "/pages/ForbiddenPage", "uni.addInterceptor"]], [srcForbidden, ["403", "暂无访问权限"]]],
    live: ["/api/v1/user/profile"],
  },
  {
    center: "mine",
    component: "service.help",
    static: [[srcProfile, ["id: \"help\"", "帮助中心"]], [srcWorkbench, ["showHelpCenter", "/api/v1/app/page-config/profile", "/api/v1/user/api-settings"]]],
    live: ["/api/v1/app/page-config/profile", "/api/v1/user/api-settings"],
  },
  {
    center: "mine",
    component: "common-services",
    static: [[srcProfile, ["commonServices", "id: \"messages\"", "id: \"knowledge\"", "id: \"ai-employees\"", "id: \"customer-service\"", "id: \"feedback\""]], [srcWorkbench, ["messages: showNotifications", "knowledge: () => openCreation(\"agent\")", "showFeedbackDialog", "/api/v1/user/dashboard"]]],
    live: ["/api/v1/user/dashboard", "/api/v1/knowledge-agents", "/api/v1/health"],
  },
  {
    center: "mine",
    component: "invoice-and-usage-export",
    static: [[srcWorkbench, ["showInvoiceNotice", "showUsageExportNotice", "/api/v1/user/usage"]], [srcInvoices, ["/api/v1/member/invoices"]]],
    live: ["/api/v1/member/invoices", "/api/v1/user/usage"],
  },
  {
    center: "agent",
    component: "overview-and-center",
    static: [[srcWorkbench, ["channelCenter.value = await businessSdk.roleWorkbench.channelCenter()", "selectAgentTab('customers')", "selectAgentTab('commission')"]], [srcRoleSdk, ["/api/v1/channel/me"]]],
    agentLive: ["/api/v1/channel/me"],
  },
  {
    center: "agent",
    component: "promotion-and-invites",
    static: [[srcWorkbench, ["activeTab === 'promotion'", "open-type=\"share\"", "copyInviteLink", "后端暂未生成推广链接", "agentInviteRecords"]]],
    agentLive: ["/api/v1/channel/me", "/api/v1/channel/invite-records"],
  },
  {
    center: "agent",
    component: "orders-commissions-withdrawals",
    static: [[srcWorkbench, ["channelCustomers", "channelCommissions", "channelWithdrawals", "agentCommissionDetail", "agentWithdrawals"]]],
    agentLive: ["/api/v1/channel/me", "/api/v1/channel/orders", "/api/v1/channel/withdrawals"],
  },
  {
    center: "operation",
    component: "overview-agents-orders",
    static: [[srcWorkbench, ["operationProfile()", "operationAgents()", "operationOrders()", "openOperationAgentDetail", "openOperationOrderDetail"]], [srcRoleSdk, ["/api/v1/operation-center/profile", "/api/v1/operation-center/agents", "/api/v1/operation-center/orders"]]],
    operationLive: ["/api/v1/operation-center/profile", "/api/v1/operation-center/agents", "/api/v1/operation-center/orders"],
  },
  {
    center: "operation",
    component: "commission-and-details",
    static: [[srcWorkbench, ["operationCommissions()", "openOperationCommissionDetail", "operationCommissionDetail"]], [srcRoleSdk, ["/api/v1/operation-center/commissions"]]],
    operationLive: ["/api/v1/operation-center/commissions"],
  },
];

const distChecks = [
  [distConfig, ["home.inspiration.ecommerce", "studio.scene.ecommerce_main", "电商主图"]],
  [distWorkbench, ["/api/v1/user/dashboard", "/api/v1/app/page-config/profile", "/api/v1/user/api-settings", "/api/v1/knowledge-conversations"]],
  [distHome, ["hero-input", "always-embed", "bindinput=\"nativeHomePromptInput\"", "bindtap=\"nativeHomePromptSubmit\"", "hero-input-action submit"]],
  [distHomeJs, ["nativeHomePromptSubmit", "v531-creation-prompt", "wx.navigateTo", "UserImageCreationPage", "UserVideoCreationPage", "UserPptCreationPage", "wxsCallMethods"]],
  [distStudio, ["一句话开始创作", "AI 核心能力", "AI 场景", "data-studio-action=\"reference\"", "bindtap=\"nativeStudioChooseReference\"", "data-studio-action=\"file\"", "bindtap=\"nativeStudioChooseFile\""]],
  [distStudioJs, ["nativeStudioChooseReference", "nativeStudioChooseFile", "__xianzhiV531StudioChooseReference", "__xianzhiV531StudioChooseFile", "wxsCallMethods"]],
  [distAssets, ["作品类型", "状态筛选", "最近作品", "bindtap=\"nativeAssetOpenAll\"", "bindtap=\"nativeAssetOpenTasks\"", "bindtap=\"nativeAssetOpenFilter\"", "bindtap=\"nativeAssetOpenSort\"", "bindtap=\"nativeAssetOpenManage\""]],
  [distAssetsJs, ["nativeAssetOpenAll", "nativeAssetOpenTasks", "__xianzhiAssetNativeBridge", "UserAssetsListPage", "wxsCallMethods"]],
  [distAssetTypeTabs, ["data-asset-value", "bindtap=\"nativeAssetTypeSelect\""]],
  [distAssetTypeTabsJs, ["nativeAssetTypeSelect", "bridge.setType", "wxsCallMethods"]],
  [distAssetStatusTabs, ["data-asset-value", "bindtap=\"nativeAssetStatusSelect\""]],
  [distAssetStatusTabsJs, ["nativeAssetStatusSelect", "bridge.setStatus", "wxsCallMethods"]],
  [distAssetEmptyState, ["bindtap=\"nativeAssetEmptyAction\""]],
  [distAssetEmptyStateJs, ["nativeAssetEmptyAction", "bridge.emptyAction", "wx.switchTab", "UserCreationPage", "wxsCallMethods"]],
  [distProfile, ["我的 AI", "钱包摘要", "企业中心", "我的工作台", "我的数据", "常用服务", "角色功能", "退出登录", "bindtap"]],
  [distProfileJs, ["AI生图", "消息中心", "代理商工作台"]],
  [distPermissions, ["升级代理商", "推广中心", "提现中心", "代理管理", "运营中心续费"]],
];

const distForbiddenChecks = [
  [distAssetTypeTabs, ['bindtap="{{item.']],
  [distAssetStatusTabs, ['bindtap="{{item.']],
  [distAssetEmptyState, ['class="empty-action', 'bindtap="{{']],
  [distAssets, ['$1bindtap=']],
];

function verifyStatic(check) {
  const missing = [];
  for (const [file, snippets] of check.static || []) {
    const text = read(file);
    for (const snippet of snippets) {
      if (!text.includes(snippet)) missing.push(`${file} missing ${JSON.stringify(snippet)}`);
    }
  }
  return missing;
}

function verifyDist() {
  const missing = [];
  for (const [file, snippets] of distChecks) {
    if (!fs.existsSync(path.join(repoRoot, file))) {
      missing.push(`${file} missing; run npm run build:user-mp-weixin first`);
      continue;
    }
    const text = read(file);
    for (const snippet of snippets) {
      if (!text.includes(snippet)) missing.push(`${file} missing ${JSON.stringify(snippet)}`);
    }
  }
  for (const [file, forbiddenSnippets] of distForbiddenChecks) {
    if (!fs.existsSync(path.join(repoRoot, file))) continue;
    const text = read(file);
    if (file === distAssetEmptyState) {
      const actionStart = text.indexOf('class="empty-action');
      const actionEnd = text.indexOf('</button>', actionStart);
      const actionButton = actionStart >= 0 && actionEnd >= 0 ? text.slice(actionStart, actionEnd) : "";
      if (actionButton.includes('bindtap="{{')) missing.push(`${file} still uses a dynamic empty-state tap binding`);
      continue;
    }
    for (const snippet of forbiddenSnippets) {
      if (text.includes(snippet)) missing.push(`${file} still contains forbidden ${JSON.stringify(snippet)}`);
    }
  }
  return missing;
}

async function verifyLiveEndpoints(check, token, agentToken, operationToken) {
  const results = [];
  for (const endpoint of check.live || []) {
    await request("GET", endpoint, token);
    results.push(endpoint);
  }
  for (const endpoint of check.agentLive || []) {
    await request("GET", endpoint, agentToken);
    results.push(endpoint);
  }
  for (const endpoint of check.operationLive || []) {
    await request("GET", endpoint, operationToken);
    results.push(endpoint);
  }
  return results;
}

const byCenter = new Map();
function record(center, ok) {
  const entry = byCenter.get(center) || { ok: 0, failed: 0 };
  if (ok) entry.ok += 1;
  else entry.failed += 1;
  byCenter.set(center, entry);
}

async function main() {
  const distMissing = verifyDist();
  if (distMissing.length) {
    throw new Error(`mp-weixin build artifact check failed:\n${distMissing.join("\n")}`);
  }

  const userToken = await login(userEmail, userPassword);
  let agentToken = userToken;
  try {
    agentToken = await login(agentEmail, agentPassword);
  } catch {
    // Agent-only checks still have static coverage; keep user token as a fallback
    // so the verifier reports the real endpoint error if the route is inaccessible.
  }
  let operationToken = userToken;
  try {
    operationToken = await login(operationEmail, operationPassword);
  } catch {
    // Operation-only checks retain static coverage and report the live authorization failure.
  }

  const failures = [];
  const rows = [];
  for (const check of clickChecks.filter(check => !centerFilter || check.center === centerFilter)) {
    const staticMissing = verifyStatic(check);
    if (staticMissing.length) {
      failures.push(`${check.center}/${check.component}\n${staticMissing.join("\n")}`);
      record(check.center, false);
      continue;
    }
    try {
      const endpoints = await verifyLiveEndpoints(check, userToken, agentToken, operationToken);
      rows.push({
        center: check.center,
        component: check.component,
        endpoints: endpoints.join(", ") || "static-only",
      });
      record(check.center, true);
    } catch (error) {
      failures.push(`${check.center}/${check.component}\n${error instanceof Error ? error.message : String(error)}`);
      record(check.center, false);
    }
  }

  console.table(rows);
  console.log("Summary:");
  for (const [center, summary] of byCenter.entries()) {
    console.log(`- ${center}: ${summary.ok} passed, ${summary.failed} failed`);
  }
  if (failures.length) {
    throw new Error(`click/api verification failed:\n${failures.join("\n\n")}`);
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
