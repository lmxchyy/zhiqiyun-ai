import { defineStore } from "pinia";
import { adminRequest } from "../api/client";
import { readAiImageSnapshot, writeAiImageSnapshot } from "../utils/aiImageDb";
import { moduleListQuery, usesInstantWorkspace } from "../utils/userWorkspaceLoad";
import { getWebAccessToken } from "../utils/webAuthSession";

export interface AdminModule {
  id: string;
  title: string;
  endpoint: string;
  surface?: "admin" | "user" | "agent" | "operation-center";
  path?: string;
  aliases?: string[];
  domain?: "enterprise" | "billing";
  enterpriseSuffix?: string;
  permission?: string;
}

export type AdminRecord = Record<string, unknown>;

function isAiGenerationTaskRunning(task: AdminRecord) {
  return ["PENDING", "RUNNING", "QUEUED", "PROCESSING"].includes(String(task.status || "").toUpperCase());
}

function isVideoGenerationRecord(task: AdminRecord) {
  const type = String(task.type || task.sourceType || "").toUpperCase();
  const mediaType = String(task.mediaType || "").toLowerCase();
  const model = String(task.model || "").toLowerCase();
  const params = task.params && typeof task.params === "object" ? task.params as AdminRecord : {};
  const inputMode = String(params.inputMode || task.mode || "").toLowerCase();
  const url = String(task.outputUrl || task.resultUrl || task.imageUrl || task.url || "");
  return type.includes("VIDEO")
    || mediaType === "video"
    || inputMode.includes("video")
    || model.includes("video")
    || /\.mp4(\?|$)/i.test(url);
}

function hasRunningAiGenerationSnapshot(data: AdminRecord) {
  const recentTasks = Array.isArray(data.recentTasks) ? data.recentTasks : [];
  return recentTasks.some((task) => task && typeof task === "object" && !isVideoGenerationRecord(task as AdminRecord) && isAiGenerationTaskRunning(task as AdminRecord));
}

function usesAiImageSnapshot(moduleId: string) {
  return ["userAiImage", "userWirelessCanvas", "userWorks"].includes(moduleId);
}

function emptyOnlineWorkspaceData(): AdminRecord {
  return {
    summary: {},
    metrics: [],
    providers: [],
    models: [],
    recentTasks: [],
    recentAssets: [],
    assets: [],
    aiState: {},
    queue: {}
  };
}

export const adminModules: AdminModule[] = [
  { id: "userDashboard", title: "用户首页", endpoint: "/user/dashboard", surface: "user", path: "/app" },
  { id: "userAiImage", title: "AI生图", endpoint: "/user/online-image", surface: "user", path: "/app/ai-image", aliases: ["/app/image-generation"] },
  { id: "userAgentCenter", title: "智能体中心", endpoint: "", surface: "user", path: "/app/agents", aliases: ["/app/agent-center"] },
  { id: "userWirelessCanvas", title: "无线画布", endpoint: "/user/online-image", surface: "user", path: "/app/wireless-canvas" },
  { id: "userVideoGeneration", title: "视频生成", endpoint: "/user/online-image", surface: "user", path: "/app/video-generation" },
  { id: "userPptGeneration", title: "PPT文档生成", endpoint: "", surface: "user", path: "/app/ppt-generation", aliases: ["/app/ai-ppt"] },
  { id: "userSmartVideo", title: "AI自动混剪", endpoint: "", surface: "user", path: "/app/smart-video", aliases: ["/app/ai-montage", "/app/auto-montage"] },
  { id: "userWorks", title: "作品中心", endpoint: "/user/online-image", surface: "user", path: "/app/works" },
  { id: "userUsage", title: "使用记录", endpoint: "/user/usage", surface: "user", path: "/app/usage" },
  { id: "userMembership", title: "身份/充值/订阅", endpoint: "/member/wallet", surface: "user", path: "/app/membership" },
  { id: "userOrders", title: "订单明细", endpoint: "/member/wallet", surface: "user", path: "/app/orders" },
  { id: "partnerDashboard", title: "代理商看板", endpoint: "/channel/me", surface: "agent", path: "/app/agent" },
  { id: "partnerCustomers", title: "客户管理", endpoint: "/channel/customers", surface: "agent", path: "/app/agent/customers" },
  { id: "partnerOrders", title: "订单管理", endpoint: "/channel/orders", surface: "agent", path: "/app/agent/orders" },
  { id: "partnerUsage", title: "消费明细", endpoint: "/channel/usage", surface: "agent", path: "/app/agent/usage" },
  { id: "partnerCommissions", title: "佣金结算", endpoint: "/channel/commissions", surface: "agent", path: "/app/agent/commissions" },
  { id: "partnerChannels", title: "推广渠道", endpoint: "/channel/me", surface: "agent", path: "/app/agent/channels" },
  { id: "partnerMaterials", title: "素材中心", endpoint: "/channel/me", surface: "agent", path: "/app/agent/materials" },
  { id: "partnerAccount", title: "账户设置", endpoint: "/channel/me", surface: "agent", path: "/app/agent/account" },
  { id: "operationCenterDashboard", title: "运营中心看板", endpoint: "/operation-center/profile", surface: "operation-center", path: "/app/operation-center" },
  { id: "operationCenterAgents", title: "代理商管理", endpoint: "/operation-center/agents", surface: "operation-center", path: "/app/operation-center/agents" },
  { id: "operationCenterOrders", title: "订单归属", endpoint: "/operation-center/orders", surface: "operation-center", path: "/app/operation-center/orders" },
  { id: "operationCenterCommissions", title: "分润明细", endpoint: "/operation-center/commissions", surface: "operation-center", path: "/app/operation-center/commissions" },
  { id: "enterpriseList", title: "企业列表", endpoint: "", domain: "enterprise", path: "/admin/enterprises", permission: "enterprise:list" },
  { id: "enterpriseDetail", title: "企业详情", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId", enterpriseSuffix: "", permission: "enterprise:detail" },
  { id: "enterpriseCertifications", title: "认证审核", endpoint: "", domain: "enterprise", path: "/admin/enterprises/certifications", enterpriseSuffix: "certifications", permission: "enterprise:certification:review" },
  { id: "enterpriseMembers", title: "成员与组织", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/members", enterpriseSuffix: "members", permission: "enterprise:member:view" },
  { id: "enterprisePackage", title: "套餐与席位", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/package", enterpriseSuffix: "package", permission: "enterprise:package:view" },
  { id: "enterpriseCompute", title: "算力账户", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/compute", enterpriseSuffix: "compute", permission: "enterprise:compute:view" },
  { id: "enterpriseTransactions", title: "充值消费明细", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/transactions", enterpriseSuffix: "transactions", permission: "enterprise:transaction:view" },
  { id: "enterpriseOrders", title: "企业订单", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/orders", enterpriseSuffix: "orders", permission: "enterprise:order:view" },
  { id: "enterpriseAiCapabilities", title: "模型与 AI 能力", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/ai-capabilities", enterpriseSuffix: "ai-capabilities", permission: "enterprise:ai:view" },
  { id: "enterpriseAiEmployees", title: "AI 员工", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/ai-employees", enterpriseSuffix: "ai-employees", permission: "enterprise:employee:view" },
  { id: "enterpriseKnowledgeBases", title: "知识库概览", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/knowledge-bases", enterpriseSuffix: "knowledge-bases", permission: "enterprise:knowledge:view" },
  { id: "enterpriseAttribution", title: "客户归属", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/attribution", enterpriseSuffix: "attribution", permission: "enterprise:attribution:view" },
  { id: "enterpriseRelationships", title: "渠道关系", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/relationships", enterpriseSuffix: "relationships", permission: "enterprise:attribution:view" },
  { id: "enterpriseIntegrations", title: "集成中心", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/integrations", enterpriseSuffix: "integrations", permission: "enterprise:connector:view" },
  { id: "enterpriseRisk", title: "风控与禁用", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/risk", enterpriseSuffix: "risk", permission: "enterprise:risk:view" },
  { id: "enterpriseAuditLogs", title: "审计日志", endpoint: "", domain: "enterprise", path: "/admin/enterprises/:enterpriseId/audit-logs", enterpriseSuffix: "audit-logs", permission: "enterprise:audit:view" },
  { id: "analysis", title: "分析页", endpoint: "/admin/overview", path: "/admin/overview", aliases: ["/admin"] },
  { id: "workbench", title: "工作台", endpoint: "/admin/overview", path: "/admin/workbench" },
  { id: "dashboard", title: "数据中心", endpoint: "/admin/overview", path: "/admin/dashboard" },
  { id: "customers", title: "客户中心", endpoint: "/admin/customers", path: "/admin/customers" },
  { id: "personalPointsGovernance", title: "赠送积分到期策略", endpoint: "", path: "/admin/customers/point-expiry", permission: "points:gift-policy:view" },
  { id: "customerAttributions", title: "客户归属总览", endpoint: "", path: "/admin/customers/attributions" },
  { id: "channels", title: "代理商中心", endpoint: "/admin/channel-agents/tree", path: "/admin/channels/agents" },
  { id: "operationCenters", title: "运营中心", endpoint: "/admin/operation-centers", path: "/admin/channels/operation-centers" },
  { id: "products", title: "产品目录", endpoint: "/admin/products", path: "/admin/catalog/products" },
  { id: "plans", title: "套餐权益", endpoint: "/admin/plans", path: "/admin/catalog/plans" },
  { id: "pricePlanGovernance", title: "套餐与价格配置", endpoint: "", path: "/admin/catalog/price-plans", permission: "pricing:plan:view" },
  { id: "orders", title: "订单中心", endpoint: "/admin/orders", path: "/admin/orders" },
  { id: "usage", title: "用量中心", endpoint: "/admin/usage", path: "/admin/usage" },
  { id: "tokenRecords", title: "Token 流水", endpoint: "/admin/token-records", path: "/admin/usage/token-records" },
  { id: "marketingDashboard", title: "营销端总览", endpoint: "/admin/marketing/overview", path: "/admin/growth/overview" },
  { id: "marketingAgentLevels", title: "代理等级", endpoint: "/admin/marketing/agent-levels", path: "/admin/growth/agent-levels" },
  { id: "marketingInvites", title: "邀请记录", endpoint: "/admin/marketing/invite-records", path: "/admin/growth/invites" },
  { id: "marketingCommissionRules", title: "分佣规则", endpoint: "/admin/marketing/commission-rules", path: "/admin/growth/commission-rules" },
  { id: "marketingUpgradePlans", title: "升级方案", endpoint: "/admin/marketing/upgrade-plans", path: "/admin/growth/upgrade-plans" },
  { id: "marketingWallets", title: "营销钱包", endpoint: "/admin/marketing/wallets", path: "/admin/growth/wallets" },
  { id: "marketingWalletRecords", title: "渠道佣金钱包流水", endpoint: "/admin/marketing/wallet-records", path: "/admin/growth/wallet-records" },
  { id: "marketingSettlementStatements", title: "月度结算单", endpoint: "/admin/marketing/settlement-statements", path: "/admin/growth/settlement-statements" },
  { id: "billingOverview", title: "计费总览", endpoint: "/admin/billing/overview", domain: "billing", path: "/admin/billing/overview", aliases: ["/admin/billing"] },
  { id: "billingRules", title: "模型计费规则", endpoint: "/admin/billing/rules", domain: "billing", path: "/admin/billing/rules" },
  { id: "billingProviderCosts", title: "供应商成本", endpoint: "/admin/billing/provider-costs", domain: "billing", path: "/admin/billing/provider-costs" },
  { id: "billingEvents", title: "计费事件", endpoint: "/admin/billing/events", domain: "billing", path: "/admin/billing/events" },
  { id: "billingReconciliation", title: "任务对账", endpoint: "/admin/billing/reconciliation", domain: "billing", path: "/admin/billing/reconciliation" },
  { id: "billingWalletLedger", title: "用户积分钱包流水", endpoint: "/admin/billing/wallet-ledger", domain: "billing", path: "/admin/billing/wallet-ledger" },
  { id: "billingCustomers", title: "客户计费", endpoint: "/admin/billing/customers", domain: "billing", path: "/admin/billing/customers" },
  { id: "billingProducts", title: "支付商品", endpoint: "/admin/billing/products", domain: "billing", path: "/admin/billing/products" },
  { id: "billingSubscriptions", title: "订阅实例", endpoint: "/admin/billing/subscriptions", domain: "billing", path: "/admin/billing/subscriptions" },
  { id: "billingCoupons", title: "优惠券", endpoint: "/admin/billing/coupons", domain: "billing", path: "/admin/billing/coupons" },
  { id: "billingInvoices", title: "发票与账单", endpoint: "/admin/billing/invoices", domain: "billing", path: "/admin/billing/invoices" },
  { id: "billingCreditNotes", title: "贷项红冲", endpoint: "/admin/billing/credit-notes", domain: "billing", path: "/admin/billing/credit-notes" },
  { id: "billingPaymentRequests", title: "付款请求与催收", endpoint: "/admin/billing/payment-requests", domain: "billing", path: "/admin/billing/payment-requests" },
  { id: "billingPayments", title: "支付记录", endpoint: "/admin/billing/payments", domain: "billing", path: "/admin/billing/payments" },
  { id: "commissions", title: "分润中心", endpoint: "/admin/commissions", path: "/admin/growth/commissions" },
  { id: "commissionRecords", title: "分润明细", endpoint: "/admin/commission-records", path: "/admin/growth/commission-records" },
  { id: "aiCapabilities", title: "能力模块", endpoint: "/admin/ai/overview", path: "/admin/ai/capabilities" },
  { id: "aiCapabilityModels", title: "模型管理", endpoint: "/admin/ai/overview", path: "/admin/ai/models" },
  { id: "aiCapabilitySchemas", title: "参数 Schema", endpoint: "/admin/ai/overview", path: "/admin/ai/schemas" },
  { id: "aiCapabilityLimits", title: "租户限制", endpoint: "/admin/ai/overview", path: "/admin/ai/limits" },
  { id: "aiCapabilityChannels", title: "上游通道", endpoint: "/admin/ai/overview", path: "/admin/ai/channels" },
  { id: "aiCapabilityLogs", title: "调用日志", endpoint: "/admin/ai/overview", path: "/admin/ai/logs" },
  { id: "knowledgeAdmin", title: "知识库中心", endpoint: "", path: "/admin/knowledge" },
  { id: "inspirationManagement", title: "创作灵感管理", endpoint: "", path: "/admin/content/inspirations" },
  { id: "mediaAssets", title: "素材中心", endpoint: "", path: "/admin/media/assets" },
  { id: "pageDecoration", title: "页面装修", endpoint: "", path: "/admin/page-operations" },
  { id: "pageHomeConfig", title: "首页配置", endpoint: "", path: "/admin/page-operations/home" },
  { id: "pageStudioConfig", title: "创作页配置", endpoint: "", path: "/admin/page-operations/studio" },
  { id: "pageAssetsConfig", title: "作品页配置", endpoint: "", path: "/admin/page-operations/assets" },
  { id: "pageProfileConfig", title: "我的页配置", endpoint: "", path: "/admin/page-operations/profile" },
  { id: "mediaCategories", title: "素材分类", endpoint: "", path: "/admin/media/categories" },
  { id: "storageCenter", title: "对象存储", endpoint: "", path: "/admin/storage" },
  { id: "apiSettings", title: "API 设置", endpoint: "/admin/system/settings", path: "/admin/ai/api-settings" },
  { id: "system", title: "系统中心", endpoint: "/admin/system/settings", path: "/admin/system" },
  { id: "departments", title: "部门管理", endpoint: "/admin/system/settings", path: "/admin/system/departments" },
  { id: "userManagement", title: "后台账号", endpoint: "/admin/customers", path: "/admin/system/users" },
  { id: "menuManagement", title: "菜单管理", endpoint: "/admin/system/settings", path: "/admin/system/menu" }
];

export const useAdminStore = defineStore("admin", {
  state: () => ({
    activeModuleId: "analysis",
    loading: false,
    saving: false,
    error: "",
    data: {} as AdminRecord,
    dataByModule: {} as Record<string, AdminRecord>,
    dataByEndpoint: {} as Record<string, AdminRecord>
  }),
  getters: {
    activeModule: (state) => adminModules.find((item) => item.id === state.activeModuleId) || adminModules[0]
  },
  actions: {
    async selectModule(moduleId: string) {
      this.activeModuleId = moduleId;
      await this.loadActiveModule();
    },
    async loadActiveModule(options: { preferCache?: boolean; silent?: boolean } = {}) {
      const moduleId = this.activeModuleId;
      const endpoint = this.activeModule.endpoint;
	  if (this.activeModule.surface === "user" && !getWebAccessToken()) {
		this.data = usesInstantWorkspace(moduleId) ? emptyOnlineWorkspaceData() : {};
		this.error = "";
		this.loading = false;
		return;
	  }
      const preferCache = options.preferCache !== false;
      const silent = options.silent === true;
      let hasCachedAiImageData = false;
      let hasCachedModuleData = false;
      const shouldUseAiImageSnapshot = usesAiImageSnapshot(moduleId);
      const shouldRenderInstantly = usesInstantWorkspace(moduleId);
      if (shouldUseAiImageSnapshot && preferCache) {
        try {
          const cached = await readAiImageSnapshot();
          if (cached?.data && !hasRunningAiGenerationSnapshot(cached.data)) {
            this.data = cached.data;
            hasCachedAiImageData = true;
          }
        } catch {
          // IndexedDB is an optional UI cache; ignore read failures.
        }
      }
      const cachedModuleData = this.dataByModule[moduleId];
      const cachedEndpointData = endpoint ? this.dataByEndpoint[endpoint] : undefined;
      if (!hasCachedAiImageData && preferCache && (cachedModuleData || cachedEndpointData)) {
        this.data = cachedModuleData || cachedEndpointData || {};
        hasCachedModuleData = true;
      }
      if (shouldRenderInstantly && !hasCachedAiImageData && !hasCachedModuleData) {
        this.data = emptyOnlineWorkspaceData();
      }
      if (!silent) {
        this.loading = !shouldRenderInstantly && !hasCachedAiImageData && !hasCachedModuleData;
      }
      this.error = "";
      if (!endpoint) {
        this.data = {};
        if (!silent) {
          this.loading = false;
        }
        return;
      }
      try {
        const data = await adminRequest<AdminRecord>({
          method: "GET",
          url: endpoint,
          params: moduleListQuery(moduleId)
        });
        if (this.activeModuleId !== moduleId) return;
        this.data = data;
        this.dataByModule[moduleId] = data;
        if (endpoint) {
          this.dataByEndpoint[endpoint] = data;
        }
        if (shouldUseAiImageSnapshot) {
          void writeAiImageSnapshot(this.data).catch(() => undefined);
        }
      } catch (error) {
        if (this.activeModuleId === moduleId) {
          this.error = error instanceof Error ? error.message : "加载失败";
        }
      } finally {
        if (this.activeModuleId === moduleId && !silent) {
          this.loading = false;
        }
      }
    },
    async mutate(method: "POST" | "PATCH", url: string, data: AdminRecord = {}) {
      this.saving = true;
      this.error = "";
      try {
        await adminRequest<AdminRecord>({ method, url, data });
        this.dataByModule = {};
        this.dataByEndpoint = {};
        await this.loadActiveModule({ preferCache: false });
      } catch (error) {
        this.error = error instanceof Error ? error.message : "保存失败";
        throw error;
      } finally {
        this.saving = false;
      }
    }
  }
});



