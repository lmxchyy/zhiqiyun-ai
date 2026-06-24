import { defineStore } from "pinia";
import { adminRequest } from "../api/client";
import { readAiImageSnapshot, writeAiImageSnapshot } from "../utils/aiImageDb";

export interface AdminModule {
  id: string;
  title: string;
  endpoint: string;
}

export type AdminRecord = Record<string, unknown>;

export const adminModules: AdminModule[] = [
  { id: "userDashboard", title: "用户首页", endpoint: "/user/dashboard" },
  { id: "userOnlineImage", title: "在线生图", endpoint: "/user/online-image" },
  { id: "userAiImage", title: "AI生图", endpoint: "/user/online-image" },
  { id: "userCanvas", title: "灵感画布", endpoint: "/generation-tasks" },
  { id: "userApiSettings", title: "API 设置", endpoint: "/user/api-settings" },
  { id: "userWorks", title: "作品中心", endpoint: "/assets" },
  { id: "userUsage", title: "使用记录", endpoint: "/user/usage" },
  { id: "userMembership", title: "会员订单", endpoint: "/points/account" },
  { id: "partnerDashboard", title: "代理商看板", endpoint: "/channel/me" },
  { id: "partnerCustomers", title: "客户管理", endpoint: "/channel/me" },
  { id: "partnerOrders", title: "订单管理", endpoint: "/channel/me" },
  { id: "partnerCommissions", title: "佣金结算", endpoint: "/channel/me" },
  { id: "partnerChannels", title: "推广渠道", endpoint: "/channel/me" },
  { id: "partnerMaterials", title: "素材中心", endpoint: "/channel/me" },
  { id: "partnerAccount", title: "账户设置", endpoint: "/channel/me" },
  { id: "analysis", title: "分析页", endpoint: "/admin/overview" },
  { id: "workbench", title: "工作台", endpoint: "/admin/overview" },
  { id: "dashboard", title: "数据中心", endpoint: "/admin/overview" },
  { id: "customers", title: "客户中心", endpoint: "/admin/customers" },
  { id: "channels", title: "代理商中心", endpoint: "/admin/channel-agents/tree" },
  { id: "products", title: "产品中心", endpoint: "/admin/products" },
  { id: "plans", title: "套餐中心", endpoint: "/admin/plans" },
  { id: "orders", title: "订单中心", endpoint: "/admin/orders" },
  { id: "usage", title: "用量中心", endpoint: "/admin/usage" },
  { id: "commissions", title: "分润中心", endpoint: "/admin/commissions" },
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
    data: {} as AdminRecord
  }),
  getters: {
    activeModule: (state) => adminModules.find((item) => item.id === state.activeModuleId) || adminModules[0]
  },
  actions: {
    async selectModule(moduleId: string) {
      this.activeModuleId = moduleId;
      await this.loadActiveModule();
    },
    async loadActiveModule() {
      let hasCachedAiImageData = false;
      if (this.activeModuleId === "userAiImage") {
        try {
          const cached = await readAiImageSnapshot();
          if (cached?.data) {
            this.data = cached.data;
            hasCachedAiImageData = true;
          }
        } catch {
          // IndexedDB is an optional UI cache; ignore read failures.
        }
      }
      this.loading = !hasCachedAiImageData;
      this.error = "";
      try {
        this.data = await adminRequest<AdminRecord>({ method: "GET", url: this.activeModule.endpoint });
        if (this.activeModuleId === "userAiImage") {
          void writeAiImageSnapshot(this.data).catch(() => undefined);
        }
      } catch (error) {
        this.error = error instanceof Error ? error.message : "加载失败";
      } finally {
        this.loading = false;
      }
    },
    async mutate(method: "POST" | "PATCH", url: string, data: AdminRecord = {}) {
      this.saving = true;
      this.error = "";
      try {
        await adminRequest<AdminRecord>({ method, url, data });
        await this.loadActiveModule();
      } catch (error) {
        this.error = error instanceof Error ? error.message : "保存失败";
        throw error;
      } finally {
        this.saving = false;
      }
    }
  }
});



