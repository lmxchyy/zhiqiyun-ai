import type { MineView } from "../types";

export type MiniProgramRoleId = "user" | "agent" | "operation";
export type MiniProgramTabId =
  | "home"
  | "create"
  | "assets"
  | "wallet"
  | "mine"
  | "overview"
  | "promotion"
  | "customers"
  | "commission"
  | "agents"
  | "orders";
export type MiniProgramCreationMode = "image" | "video" | "ppt" | "infographic" | "review" | "agent";

export const miniProgramRolePages: Record<MiniProgramRoleId, Partial<Record<MiniProgramTabId, string>>> = {
  user: {
    home: "/pages/user/UserHomePage",
    create: "/pages/user/UserCreationPage",
    assets: "/pages/user/UserAssetsPage",
    wallet: "/pages/user/UserWalletPage",
    mine: "/pages/user/UserMinePage"
  },
  agent: {
    overview: "/pages/agent/AgentOverviewPage",
    promotion: "/pages/agent/AgentPromotionPage",
    customers: "/pages/agent/AgentCustomersPage",
    commission: "/pages/agent/AgentCommissionPage",
    mine: "/pages/agent/AgentMinePage"
  },
  operation: {
    overview: "/pages/operation/OperationOverviewPage",
    agents: "/pages/operation/OperationAgentsPage",
    orders: "/pages/operation/OperationOrdersPage",
    commission: "/pages/operation/OperationCommissionPage",
    mine: "/pages/operation/OperationMinePage"
  }
};

export const miniProgramCreationPages: Record<MiniProgramCreationMode, string> = {
  image: "/pages/user/UserImageCreationPage",
  video: "/pages/user/UserVideoCreationPage",
  ppt: "/pages/user/UserPptCreationPage",
  infographic: "/pages/user/UserInfographicCreationPage",
  review: "/pages/user/UserReviewCreationPage",
  agent: "/pages/user/UserAgentCreationPage"
};

export const miniProgramMinePages: Record<MineView, string> = {
  overview: "/pages/user/UserMinePage",
  "agent-upgrade": "/pages/user/UserAgentUpgradePage",
  "recharge-history": "/pages/user/UserRechargeHistoryPage",
  "usage-details": "/pages/user/UserUsageDetailsPage",
  "identity-permissions": "/pages/user/UserIdentityPermissionsPage",
  "invite-promotion": "/pages/user/UserInvitePromotionPage"
};

export const miniProgramDefaultPage = miniProgramRolePages.user.home as string;

export const miniProgramFeaturePages = {
  userProfileEdit: "/pages/user/UserProfileEditPage",
  userSettings: "/pages/user/UserSettingsPage",
  userRechargePlans: "/pages/user/UserRechargePlansPage",
  userOrders: "/pages/user/UserOrdersPage",
  userOrderDetail: "/pages/user/UserOrderDetailPage",
  userOrderConfirm: "/pages/user/UserOrderConfirmPage",
  userOrderResult: "/pages/user/UserOrderResultPage",
  userRefundRequest: "/pages/user/UserRefundRequestPage",
  agentTeam: "/pages/agent/AgentTeamPage",
  agentTeamMember: "/pages/agent/AgentTeamMemberPage",
  agentCustomerDetail: "/pages/agent/AgentCustomerDetailPage",
  agentOrders: "/pages/agent/AgentOrdersPage",
  agentWithdrawals: "/pages/agent/AgentWithdrawalsPage",
  agentWithdrawalApply: "/pages/agent/AgentWithdrawalApplyPage",
  agentInviteRecords: "/pages/agent/AgentInviteRecordsPage"
} as const;

export function rolePage(role: MiniProgramRoleId, tab: MiniProgramTabId) {
  return miniProgramRolePages[role][tab] || miniProgramRolePages[role][role === "user" ? "home" : "overview"] || miniProgramDefaultPage;
}
