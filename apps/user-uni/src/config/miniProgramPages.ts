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
  ppt: "/packagePpt/pages/index",
  infographic: "/pages/user/UserInfographicCreationPage",
  review: "/pages/user/UserReviewCreationPage",
  agent: "/pages/user/UserAgentCreationPage"
};

export const miniProgramAssetCreationPages = {
  image: "/pages/user/UserImageCreationPage",
  video: "/pages/user/UserVideoCreationPage",
  ppt: "/pages/user/UserPptCreationPage",
  agent: "/pages/user/UserAgentCreationPage",
  infographic: "/pages/user/UserInfographicCreationPage"
} as const;

export const miniProgramPptPages = {
  index: "/packagePpt/pages/index",
  create: "/packagePpt/pages/create",
  outline: "/packagePpt/pages/outline",
  progress: "/packagePpt/pages/progress",
  detail: "/packagePpt/pages/detail",
  preview: "/packagePpt/pages/preview",
  editText: "/packagePpt/pages/edit-text",
  editVisual: "/packagePpt/pages/edit-visual",
  layout: "/packagePpt/pages/layout",
  export: "/packagePpt/pages/export",
  desktop: "/packagePpt/pages/desktop",
  error: "/packagePpt/pages/error"
} as const;

export const miniProgramMinePages: Record<MineView, string> = {
  overview: "/pages/user/UserMinePage",
  "agent-upgrade": "/pages/user/UserAgentDetailPage",
  "recharge-history": "/pages/user/UserRechargeHistoryPage",
  "usage-details": "/pages/user/UserUsageDetailsPage",
  "role-permissions": "/pages/user/UserRolePermissionsPage",
  "invite-promotion": "/pages/promotion/PromotionCenterPage"
};

export const miniProgramDefaultPage = miniProgramRolePages.user.home as string;

export const miniProgramFeaturePages = {
  promotionCenter: "/pages/promotion/PromotionCenterPage",
  promotionTemplates: "/pages/promotion/PromotionTemplateCenterPage",
  promotionPosterPreview: "/pages/promotion/PromotionPosterPreviewPage",
  promotionRecords: "/pages/promotion/PromotionRecordsPage",
  promotionStats: "/pages/promotion/PromotionStatsPage",
  userProfileEdit: "/pages/user/UserProfileEditPage",
  userSettings: "/pages/user/UserSettingsPage",
  userRechargePlans: "/pages/user/UserRechargePlansPage",
  userVirtualPayment: "/pages/user/UserVirtualPaymentPage",
  userOrders: "/pages/user/UserOrdersPage",
  userOrderDetail: "/pages/user/UserOrderDetailPage",
  userOrderConfirm: "/pages/user/UserOrderConfirmPage",
  userCommerceOrderConfirm: "/pages/user/UserCommerceOrderConfirmPage",
  userMembershipDetail: "/pages/user/UserMembershipDetailPage",
  userAgentDetail: "/pages/user/UserAgentDetailPage",
  userOrderResult: "/pages/user/UserOrderResultPage",
  userRefundRequest: "/pages/user/UserRefundRequestPage",
  userInvoices: "/pages/user/UserInvoicesPage",
  userAssetsList: "/pages/user/UserAssetsListPage",
  userAssetDetail: "/pages/user/UserAssetDetailPage",
  userTasksList: "/pages/user/UserTasksPage",
  userUsageRecordDetail: "/pages/user/UserUsageRecordDetailPage",
  userKnowledgeAgentDetail: "/pages/user/UserKnowledgeAgentDetailPage",
  userReviewConversation: "/pages/user/UserReviewConversationPage",
  agentTeam: "/pages/agent/AgentTeamPage",
  agentTeamMember: "/pages/agent/AgentTeamMemberPage",
  agentCustomerDetail: "/pages/agent/AgentCustomerDetailPage",
  agentOrders: "/pages/agent/AgentOrdersPage",
  agentOrderDetail: "/pages/agent/AgentOrderDetailPage",
  agentWithdrawals: "/pages/agent/AgentWithdrawalsPage",
  agentWithdrawalApply: "/pages/agent/AgentWithdrawalApplyPage",
  agentWithdrawalDetail: "/pages/agent/AgentWithdrawalDetailPage",
  agentInviteRecords: "/pages/agent/AgentInviteRecordsPage",
  agentCommissionDetail: "/pages/agent/AgentCommissionDetailPage",
  operationAgentDetail: "/pages/operation/OperationAgentDetailPage",
  operationOrderDetail: "/pages/operation/OperationOrderDetailPage",
  operationCommissionDetail: "/pages/operation/OperationCommissionDetailPage"
} as const;

export const miniProgramEnterprisePages = {
  entry: "/pages/enterprise/EnterpriseEntryPage",
  onboarding: "/pages/enterprise/EnterpriseOnboardingPage",
  create: "/pages/enterprise/EnterpriseCreatePage",
  join: "/pages/enterprise/EnterpriseJoinPage",
  switcher: "/pages/enterprise/EnterpriseSwitcherPage",
  overview: "/pages/enterprise/EnterpriseOverviewPage",
  organizations: "/pages/enterprise/EnterpriseOrganizationsPage",
  members: "/pages/enterprise/EnterpriseMembersPage",
  memberDetail: "/pages/enterprise/EnterpriseMemberDetailPage",
  invitations: "/pages/enterprise/EnterpriseInvitationsPage",
  roles: "/pages/enterprise/EnterpriseRolesPage",
  aiEmployees: "/pages/enterprise/EnterpriseAIEmployeesPage",
  aiEmployeeCreate: "/pages/enterprise/EnterpriseAIEmployeeCreatePage",
  aiEmployeeDetail: "/pages/enterprise/EnterpriseAIEmployeeDetailPage",
  billing: "/pages/enterprise/EnterpriseBillingPage",
  usage: "/pages/enterprise/EnterpriseUsagePage",
  settings: "/pages/enterprise/EnterpriseSettingsPage",
  feishu: "/pages/enterprise/EnterpriseFeishuConnectorPage",
  certification: "/pages/enterprise/EnterpriseCertificationPage",
  status: "/pages/enterprise/EnterpriseStatusPage",
} as const;

export function rolePage(role: MiniProgramRoleId, tab: MiniProgramTabId) {
  return miniProgramRolePages[role][tab] || miniProgramRolePages[role][role === "user" ? "home" : "overview"] || miniProgramDefaultPage;
}
