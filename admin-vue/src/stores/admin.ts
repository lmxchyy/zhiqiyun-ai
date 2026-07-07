import { defineStore } from "pinia";
import { adminRequest } from "../api/client";
import { readAiImageSnapshot, writeAiImageSnapshot } from "../utils/aiImageDb";

export interface AdminModule {
  id: string;
  title: string;
  endpoint: string;
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

function usesInstantWorkspace(moduleId: string) {
  return ["userAiImage", "userWirelessCanvas", "userWorks", "userVideoGeneration"].includes(moduleId);
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
  { id: "userDashboard", title: "用户首页", endpoint: "/user/dashboard" },
  { id: "userAiImage", title: "AI生图", endpoint: "/user/online-image" },
  { id: "userWirelessCanvas", title: "无线画布", endpoint: "/user/online-image" },
  { id: "userVideoGeneration", title: "视频生成", endpoint: "/user/online-image" },
  { id: "userPptGeneration", title: "PPT文档生成", endpoint: "" },
  { id: "userApiSettings", title: "API 设置", endpoint: "/user/api-settings" },
  { id: "userWorks", title: "作品中心", endpoint: "/user/online-image" },
  { id: "userUsage", title: "使用记录", endpoint: "/user/usage" },
  { id: "userMembership", title: "身份/充值/订阅", endpoint: "/member/wallet" },
  { id: "userOrders", title: "订单明细", endpoint: "/member/wallet" },
  { id: "partnerDashboard", title: "代理商看板", endpoint: "/channel/me" },
  { id: "partnerCustomers", title: "客户管理", endpoint: "/channel/customers" },
  { id: "partnerOrders", title: "订单管理", endpoint: "/channel/orders" },
  { id: "partnerUsage", title: "消费明细", endpoint: "/channel/usage" },
  { id: "partnerCommissions", title: "佣金结算", endpoint: "/channel/commissions" },
  { id: "partnerChannels", title: "推广渠道", endpoint: "/channel/me" },
  { id: "partnerMaterials", title: "素材中心", endpoint: "/channel/me" },
  { id: "partnerAccount", title: "账户设置", endpoint: "/channel/me" },
  { id: "analysis", title: "分析页", endpoint: "/admin/overview" },
  { id: "workbench", title: "工作台", endpoint: "/admin/overview" },
  { id: "dashboard", title: "数据中心", endpoint: "/admin/overview" },
  { id: "customers", title: "客户中心", endpoint: "/admin/customers" },
  { id: "channels", title: "代理商中心", endpoint: "/admin/channel-agents/tree" },
  { id: "operationCenters", title: "运营中心", endpoint: "/admin/operation-centers" },
  { id: "products", title: "产品中心", endpoint: "/admin/products" },
  { id: "plans", title: "套餐中心", endpoint: "/admin/plans" },
  { id: "orders", title: "订单中心", endpoint: "/admin/orders" },
  { id: "usage", title: "用量中心", endpoint: "/admin/usage" },
  { id: "tokenRecords", title: "Token 流水", endpoint: "/admin/token-records" },
  { id: "marketingDashboard", title: "营销端总览", endpoint: "/admin/marketing/overview" },
  { id: "marketingAgentLevels", title: "代理等级", endpoint: "/admin/marketing/agent-levels" },
  { id: "marketingInvites", title: "邀请记录", endpoint: "/admin/marketing/invite-records" },
  { id: "marketingCommissionRules", title: "分佣规则", endpoint: "/admin/marketing/commission-rules" },
  { id: "marketingUpgradePlans", title: "升级方案", endpoint: "/admin/marketing/upgrade-plans" },
  { id: "marketingWallets", title: "营销钱包", endpoint: "/admin/marketing/wallets" },
  { id: "marketingWalletRecords", title: "钱包流水", endpoint: "/admin/marketing/wallet-records" },
  { id: "marketingSettlementStatements", title: "月度结算单", endpoint: "/admin/marketing/settlement-statements" },
  { id: "billingDashboard", title: "计费驾驶舱", endpoint: "/admin/billing/overview" },
  { id: "billingCustomers", title: "客户计费", endpoint: "/admin/billing/customers" },
  { id: "billingProducts", title: "套餐产品", endpoint: "/admin/billing/products" },
  { id: "billingSubscriptions", title: "订阅管理", endpoint: "/admin/billing/subscriptions" },
  { id: "billingEvents", title: "计量事件", endpoint: "/admin/billing/events" },
  { id: "billingBillableMetrics", title: "计量指标", endpoint: "/admin/billing/billable-metrics" },
  { id: "billingCharges", title: "计费规则", endpoint: "/admin/billing/charges" },
  { id: "billingFees", title: "费用明细", endpoint: "/admin/billing/fees" },
  { id: "billingWallets", title: "钱包预付", endpoint: "/admin/billing/wallets" },
  { id: "billingCoupons", title: "优惠券", endpoint: "/admin/billing/coupons" },
  { id: "billingInvoices", title: "账单发票", endpoint: "/admin/billing/invoices" },
  { id: "billingCreditNotes", title: "贷项红冲", endpoint: "/admin/billing/credit-notes" },
  { id: "billingPaymentRequests", title: "付款请求", endpoint: "/admin/billing/payment-requests" },
  { id: "billingPayments", title: "支付催收", endpoint: "/admin/billing/payments" },
  { id: "commissions", title: "分润中心", endpoint: "/admin/commissions" },
  { id: "commissionRecords", title: "分润明细", endpoint: "/admin/commission-records" },
  { id: "aiCapabilities", title: "能力模块", endpoint: "/admin/ai/overview" },
  { id: "aiCapabilityModels", title: "模型管理", endpoint: "/admin/ai/overview" },
  { id: "aiCapabilitySchemas", title: "参数 Schema", endpoint: "/admin/ai/overview" },
  { id: "aiCapabilityLimits", title: "租户限制", endpoint: "/admin/ai/overview" },
  { id: "aiCapabilityChannels", title: "上游通道", endpoint: "/admin/ai/overview" },
  { id: "aiCapabilityLogs", title: "调用日志", endpoint: "/admin/ai/overview" },
  { id: "apiSettings", title: "API 设置", endpoint: "/admin/system/settings" },
  { id: "system", title: "系统中心", endpoint: "/admin/system/settings" },
  { id: "departments", title: "部门管理", endpoint: "/admin/system/settings" },
  { id: "userManagement", title: "用户管理", endpoint: "/admin/customers" },
  { id: "menuManagement", title: "菜单管理", endpoint: "/admin/system/settings" }
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
        const data = await adminRequest<AdminRecord>({ method: "GET", url: endpoint });
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



