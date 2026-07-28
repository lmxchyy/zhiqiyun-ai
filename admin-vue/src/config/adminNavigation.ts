import { adminModules, type AdminModule } from "../stores/admin";
import { moduleById, moduleIdsForDomain, moduleIdsForSurface } from "./moduleRegistry";

export type AdminNavigationIconKey = "overview" | "customers" | "commercial" | "ai" | "growth" | "platform";

export interface AdminNavigationSection {
  id: string;
  title: string;
  primaryModuleId: string;
  moduleIds: string[];
  tabModuleIds?: string[];
  requiresEnterpriseManagement?: boolean;
}

export interface AdminNavigationGroup {
  id: string;
  title: string;
  icon: AdminNavigationIconKey;
  sections: AdminNavigationSection[];
}

export const enterpriseModuleIds = moduleIdsForDomain("enterprise");
export const customerAttributionModuleIds = ["customerAttributions"];
export const billingV1ModuleIds = ["billingOverview", "billingRules", "billingProviderCosts", "billingEvents", "billingReconciliation", "billingWalletLedger"];
export const commercialBillingModuleIds = ["billingCustomers", "billingProducts", "billingSubscriptions", "billingCoupons", "billingInvoices", "billingCreditNotes", "billingPaymentRequests", "billingPayments"];
export const billingModuleIds = moduleIdsForDomain("billing");
export const aiCapabilityModuleIds = ["aiCapabilities", "aiCapabilityModels", "aiCapabilitySchemas", "aiCapabilityLimits", "aiCapabilityChannels", "aiCapabilityLogs"];
export const mediaDecorationModuleIds = ["pageDecoration", "pageHomeConfig", "pageStudioConfig", "pageAssetsConfig", "pageProfileConfig"];
export const mediaOperationModuleIds = ["inspirationManagement", "mediaAssets", "mediaCategories", ...mediaDecorationModuleIds];
export const operationCenterModuleIds = moduleIdsForSurface("operation-center");
export const agentModuleIds = moduleIdsForSurface("agent");
export const userModuleIds = moduleIdsForSurface("user");

export const adminNavigationGroups: AdminNavigationGroup[] = [
  { id: "overview", title: "管理总览", icon: "overview", sections: [
    { id: "management-overview", title: "管理总览", primaryModuleId: "analysis", moduleIds: ["analysis", "workbench", "dashboard"] }
  ] },
  { id: "customers-enterprises", title: "客户与企业", icon: "customers", sections: [
    { id: "customers", title: "客户中心", primaryModuleId: "customers", moduleIds: ["customers"] },
    { id: "enterprises", title: "企业中心", primaryModuleId: "enterpriseList", moduleIds: enterpriseModuleIds, tabModuleIds: ["enterpriseList", "enterpriseCertifications"], requiresEnterpriseManagement: true },
    { id: "attribution", title: "归属关系", primaryModuleId: "customerAttributions", moduleIds: ["customerAttributions"] }
  ] },
  { id: "products-billing", title: "商品与计费", icon: "commercial", sections: [
    { id: "catalog", title: "商品与权益", primaryModuleId: "products", moduleIds: ["products", "plans", "billingProducts", "billingSubscriptions"] },
    { id: "price-plan-governance", title: "套餐与价格配置", primaryModuleId: "pricePlanGovernance", moduleIds: ["pricePlanGovernance"] },
    { id: "orders-payments", title: "订单与支付", primaryModuleId: "orders", moduleIds: ["orders", "billingPaymentRequests", "billingPayments", "billingInvoices", "billingCreditNotes"] },
    { id: "pricing-costs", title: "定价与成本", primaryModuleId: "billingRules", moduleIds: ["billingRules", "billingProviderCosts"] },
    { id: "accounting", title: "账务与对账", primaryModuleId: "billingOverview", moduleIds: ["billingOverview", "billingEvents", "billingReconciliation", "billingWalletLedger", "billingCustomers", "usage", "tokenRecords"] },
    { id: "promotions", title: "营销优惠", primaryModuleId: "billingCoupons", moduleIds: ["billingCoupons"] }
  ] },
  { id: "ai-content", title: "AI 与内容", icon: "ai", sections: [
    { id: "ai-capabilities", title: "AI 能力配置", primaryModuleId: "aiCapabilities", moduleIds: ["aiCapabilities", "aiCapabilityModels", "aiCapabilitySchemas"] },
    { id: "ai-governance", title: "接入与调用治理", primaryModuleId: "aiCapabilityChannels", moduleIds: ["aiCapabilityChannels", "aiCapabilityLimits", "aiCapabilityLogs", "apiSettings"] },
    { id: "knowledge", title: "知识库", primaryModuleId: "knowledgeAdmin", moduleIds: ["knowledgeAdmin"] },
    { id: "content-operations", title: "内容运营", primaryModuleId: "inspirationManagement", moduleIds: ["inspirationManagement"] },
    { id: "media", title: "素材库", primaryModuleId: "mediaAssets", moduleIds: ["mediaAssets", "mediaCategories"] },
    { id: "page-operations", title: "页面运营", primaryModuleId: "pageDecoration", moduleIds: mediaDecorationModuleIds }
  ] },
  { id: "channels-growth", title: "渠道与增长", icon: "growth", sections: [
    { id: "growth-overview", title: "渠道总览", primaryModuleId: "marketingDashboard", moduleIds: ["marketingDashboard"] },
    { id: "partners", title: "伙伴与运营中心", primaryModuleId: "channels", moduleIds: ["channels", "operationCenters", "marketingAgentLevels"] },
    { id: "acquisition", title: "邀新与升级", primaryModuleId: "marketingInvites", moduleIds: ["marketingInvites", "marketingUpgradePlans"] },
    { id: "settlement", title: "分佣与结算", primaryModuleId: "commissions", moduleIds: ["commissions", "commissionRecords", "marketingCommissionRules", "marketingWallets", "marketingWalletRecords", "marketingSettlementStatements"] }
  ] },
  { id: "platform-governance", title: "系统管理", icon: "platform", sections: [
    { id: "system", title: "系统设置", primaryModuleId: "system", moduleIds: ["system"] },
    { id: "organization", title: "组织与后台账号", primaryModuleId: "userManagement", moduleIds: ["userManagement", "departments"] },
    { id: "permissions", title: "权限与菜单", primaryModuleId: "menuManagement", moduleIds: ["menuManagement"] },
    { id: "storage", title: "存储资源", primaryModuleId: "storageCenter", moduleIds: ["storageCenter"] }
  ] }
];

export const channelGrowthModuleIds = adminNavigationGroups
  .find((group) => group.id === "channels-growth")
  ?.sections.flatMap((section) => section.moduleIds) || [];

export function adminNavigationSectionForModule(moduleId: string) {
  for (const group of adminNavigationGroups) {
    const section = group.sections.find((item) => item.moduleIds.includes(moduleId));
    if (section) return { group, section };
  }
  return null;
}

export function adminModuleById(moduleId: string): AdminModule | undefined {
  return moduleById(moduleId);
}
