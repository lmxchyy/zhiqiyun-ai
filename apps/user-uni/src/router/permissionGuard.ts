import type { Pinia } from "pinia";
import { useUserStore } from "../stores/user";
import type { AppRole } from "../types";
import type { ProtectedAction } from "@xianzhi/shared-auth";
import { getAuthToken } from "../api/client";
import { requireAuth } from "../features/auth/gate";
import { pageAccessFor } from "../features/auth/accessPolicy";

interface RouteRequirement {
  role?: AppRole;
  permission?: string;
}

const exactRequirements: Record<string, RouteRequirement> = {
  "/pages/agent/AgentPromotionPage": { role: "AGENT", permission: "agent:promotion" },
  "/pages/agent/AgentCustomersPage": { role: "AGENT", permission: "agent:customer:view" },
  "/pages/agent/AgentCustomerDetailPage": { role: "AGENT", permission: "agent:customer:view" },
  "/pages/agent/AgentCommissionPage": { role: "AGENT", permission: "agent:commission:view" },
  "/pages/agent/AgentCommissionDetailPage": { role: "AGENT", permission: "agent:commission:view" },
  "/pages/agent/AgentWithdrawalsPage": { role: "AGENT", permission: "agent:withdraw" },
  "/pages/agent/AgentWithdrawalApplyPage": { role: "AGENT", permission: "agent:withdraw" },
  "/pages/agent/AgentWithdrawalDetailPage": { role: "AGENT", permission: "agent:withdraw" },
  "/pages/operation/OperationOverviewPage": { role: "OPERATION", permission: "operation:dashboard" },
  "/pages/operation/OperationAgentsPage": { role: "OPERATION", permission: "operation:agent:list" },
  "/pages/operation/OperationAgentDetailPage": { role: "OPERATION", permission: "operation:agent:list" },
  "/pages/operation/OperationOrdersPage": { role: "OPERATION", permission: "operation:order:view" },
  "/pages/operation/OperationOrderDetailPage": { role: "OPERATION", permission: "operation:order:view" },
  "/pages/operation/OperationCommissionPage": { role: "OPERATION", permission: "operation:dashboard" },
  "/pages/operation/OperationCommissionDetailPage": { role: "OPERATION", permission: "operation:dashboard" },
  "/pages/enterprise/EnterpriseOverviewPage": { permission: "enterprise.overview.read" },
  "/pages/enterprise/EnterpriseOrganizationsPage": { permission: "enterprise.organization.read" },
  "/pages/enterprise/EnterpriseMembersPage": { permission: "enterprise.member.read" },
  "/pages/enterprise/EnterpriseMemberDetailPage": { permission: "enterprise.member.read" },
  "/pages/enterprise/EnterpriseInvitationsPage": { permission: "enterprise.member.read" },
  "/pages/enterprise/EnterpriseRolesPage": { permission: "enterprise.role.read" },
  "/pages/enterprise/EnterpriseAIEmployeesPage": { permission: "enterprise.overview.read" },
  "/pages/enterprise/EnterpriseAIEmployeeDetailPage": { permission: "enterprise.overview.read" },
  "/pages/enterprise/EnterpriseBillingPage": { permission: "enterprise.billing.read" },
  "/pages/enterprise/EnterpriseUsagePage": { permission: "enterprise.billing.read" },
  "/pages/enterprise/EnterpriseSettingsPage": { permission: "enterprise.settings.read" },
  "/pages/enterprise/EnterpriseCertificationPage": { permission: "enterprise.certification.submit" },
};

function normalizeRoute(url: unknown): string {
  const path = String(url || "").split("?")[0];
  return path.startsWith("/") ? path : `/${path}`;
}

export function permissionForRoute(url: string): RouteRequirement | null {
  const route = normalizeRoute(url);
  if (exactRequirements[route]) return exactRequirements[route];
  if (route.startsWith("/pages/agent/")) return { role: "AGENT", permission: "agent:promotion" };
  if (route.startsWith("/pages/operation/")) return { role: "OPERATION", permission: "operation:dashboard" };
  return null;
}

function actionForRoute(route: string): ProtectedAction {
  if (route.includes("Wallet")) return "open_wallet";
  if (route.includes("Order")) return "open_order";
  if (route.includes("Recharge") || route.includes("Membership")) return "open_member_center";
  if (route.includes("AgentCreation")) return "create_agent";
  return "save_work";
}

export function installPermissionRouterGuard(pinia: Pinia) {
  const userStore = useUserStore(pinia);
  let redirecting = false;
  const guard = {
    invoke(args: { url?: string }) {
      const route = normalizeRoute(args?.url);
      const access = pageAccessFor(route);
      if (access === "authenticated" && !getAuthToken()) {
        void requireAuth({
          action: actionForRoute(route),
          route,
          payload: {},
          resume: () => new Promise<void>(resolve => uni.navigateTo({ url: String(args?.url || route), complete: () => resolve() })),
        });
        return false;
      }
      const requirement = permissionForRoute(String(args?.url || ""));
      if (!requirement) return true;
      const roleAllowed = !requirement.role || (userStore.hasRole(requirement.role) && userStore.currentRole === requirement.role);
      const permissionAllowed = !requirement.permission || userStore.hasPermission(requirement.permission);
      if (roleAllowed && permissionAllowed) return true;
      if (!redirecting) {
        redirecting = true;
        const target = encodeURIComponent(normalizeRoute(args?.url));
        const permission = encodeURIComponent(requirement.permission || "");
        setTimeout(() => {
          uni.navigateTo({
            url: `/pages/ForbiddenPage?target=${target}&permission=${permission}`,
            complete: () => { redirecting = false; },
          });
        }, 0);
      }
      return false;
    },
  };
  (["navigateTo", "redirectTo", "reLaunch", "switchTab"] as const).forEach(method => {
    uni.addInterceptor(method, guard);
  });
}
