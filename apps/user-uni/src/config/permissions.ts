import type { AppRole } from "../types";

export const roleLabels: Record<AppRole, string> = {
  USER: "普通用户",
  AGENT: "代理商",
  OPERATION: "运营中心",
  ENTERPRISE_ADMIN: "企业管理员",
  AI_ADMIN: "AI 管理员",
  FINANCE: "财务",
  CUSTOMER_SERVICE: "客服",
  ENTERPRISE_MEMBER: "企业成员",
};

export const PermissionConfig: Record<AppRole, readonly string[]> = {
  USER: ["ai:use", "assets:view", "project:view", "wallet:view", "settings:view"],
  AGENT: [
    "agent:promotion",
    "agent:promotion:create",
    "agent:qrcode:view",
    "agent:customer:view",
    "agent:commission:view",
    "agent:withdraw",
    "agent:material:view",
  ],
  OPERATION: [
    "operation:dashboard",
    "operation:agent:list",
    "operation:agent:approve",
    "operation:order:view",
    "operation:customer:view",
    "operation:report:view",
    "operation:announcement:manage",
    "operation:renew",
  ],
  ENTERPRISE_ADMIN: [
    "enterprise.overview.read",
    "enterprise.organization.read",
    "enterprise.organization.create",
    "enterprise.organization.update",
    "enterprise.organization.delete",
    "enterprise.member.read",
    "enterprise.member.invite",
    "enterprise.member.update",
    "enterprise.member.disable",
    "enterprise.member.remove",
    "enterprise.role.read",
    "enterprise.role.assign",
    "enterprise.billing.read",
    "enterprise.audit.read",
    "enterprise.settings.read",
    "enterprise.settings.update",
    "enterprise.certification.submit",
    "enterprise.connector.read",
    "enterprise.connector.manage",
  ],
  AI_ADMIN: ["ai:admin", "enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read", "enterprise.role.read", "enterprise.settings.read", "enterprise.connector.read", "enterprise.connector.manage"],
  FINANCE: ["finance:view", "finance:approve", "enterprise.overview.read", "enterprise.member.read", "enterprise.billing.read", "enterprise.audit.read"],
  CUSTOMER_SERVICE: ["customer-service:manage", "enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read"],
  ENTERPRISE_MEMBER: ["enterprise.overview.read", "enterprise.organization.read", "enterprise.member.read", "enterprise.settings.read"],
};

export interface RoleMenuItem {
  id: string;
  label: string;
  permission?: string;
  primary?: boolean;
}

export const RoleMenuConfig: Record<AppRole, readonly RoleMenuItem[]> = {
  USER: [
    { id: "ai", label: "AI能力", permission: "ai:use" },
    { id: "assets", label: "作品", permission: "assets:view" },
    { id: "projects", label: "项目", permission: "project:view" },
    { id: "wallet", label: "钱包", permission: "wallet:view" },
    { id: "settings", label: "设置", permission: "settings:view" },
    { id: "upgrade-agent", label: "升级代理商", primary: true },
  ],
  AGENT: [
    { id: "agent-promotion", label: "推广中心", permission: "agent:promotion", primary: true },
    { id: "agent-qrcode", label: "推广二维码", permission: "agent:qrcode:view" },
    { id: "agent-customers", label: "客户管理", permission: "agent:customer:view" },
    { id: "agent-commission", label: "分润中心", permission: "agent:commission:view" },
    { id: "agent-withdraw", label: "提现中心", permission: "agent:withdraw" },
    { id: "agent-materials", label: "推广素材", permission: "agent:material:view" },
  ],
  OPERATION: [
    { id: "operation-agents", label: "代理管理", permission: "operation:agent:list", primary: true },
    { id: "operation-orders", label: "区域订单", permission: "operation:order:view" },
    { id: "operation-customers", label: "区域客户", permission: "operation:customer:view" },
    { id: "operation-reports", label: "数据报表", permission: "operation:report:view" },
    { id: "operation-announcements", label: "公告管理", permission: "operation:announcement:manage" },
    { id: "operation-renew", label: "运营中心续费", permission: "operation:renew" },
  ],
  ENTERPRISE_ADMIN: [
    { id: "enterprise-overview", label: "企业概览", permission: "enterprise.overview.read", primary: true },
    { id: "enterprise-members", label: "成员管理", permission: "enterprise.member.read" },
    { id: "enterprise-organizations", label: "组织管理", permission: "enterprise.organization.read" },
    { id: "enterprise-ai-employees", label: "AI员工", permission: "enterprise.overview.read" },
    { id: "enterprise-billing", label: "企业算力", permission: "enterprise.billing.read" },
    { id: "enterprise-roles", label: "角色权限", permission: "enterprise.role.read" },
    { id: "enterprise-settings", label: "企业设置", permission: "enterprise.settings.read" },
  ],
  AI_ADMIN: [
    { id: "enterprise-ai-employees", label: "AI员工", permission: "ai:admin", primary: true },
    { id: "enterprise-roles", label: "角色权限", permission: "enterprise.role.read" },
  ],
  FINANCE: [
    { id: "enterprise-billing", label: "企业算力", permission: "enterprise.billing.read", primary: true },
    { id: "finance-approval", label: "资金审核", permission: "finance:approve" },
  ],
  CUSTOMER_SERVICE: [
    { id: "enterprise-members", label: "成员管理", permission: "enterprise.member.read", primary: true },
    { id: "enterprise-organizations", label: "组织管理", permission: "enterprise.organization.read" },
  ],
  ENTERPRISE_MEMBER: [
    { id: "enterprise-overview", label: "企业概览", permission: "enterprise.overview.read", primary: true },
    { id: "enterprise-organizations", label: "组织架构", permission: "enterprise.organization.read" },
    { id: "enterprise-members", label: "企业成员", permission: "enterprise.member.read" },
    { id: "enterprise-settings", label: "企业设置", permission: "enterprise.settings.read" },
  ],
};

export function permissionsForRole(role: AppRole): string[] {
  const permissions = new Set<string>(PermissionConfig.USER);
  if (role !== "USER") PermissionConfig[role].forEach(permission => permissions.add(permission));
  return [...permissions].sort();
}

export function isAppRole(value: unknown): value is AppRole {
  return typeof value === "string" && Object.prototype.hasOwnProperty.call(PermissionConfig, value);
}
