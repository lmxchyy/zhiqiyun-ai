import { rolePage, miniProgramFeaturePages, miniProgramMinePages, miniProgramEnterprisePages, type MiniProgramRoleId, type MiniProgramTabId } from "../../config/miniProgramPages";

export type WorkbenchPageNavigator = {
  navigate: (url: string) => void;
  redirect: (url: string) => void;
  reLaunch: (url: string) => void;
  switchTab: (url: string) => void;
};

export function isWorkbenchNativeTab(url: string) {
  const targetRoute = url.replace(/^\//, "").split("?")[0];
  const userNativeTabRoutes = new Set([
    "pages/user/UserHomePage",
    "pages/user/UserCreationPage",
    "pages/user/UserAssetsPage",
    "pages/user/UserMinePage",
  ]);
  return userNativeTabRoutes.has(targetRoute);
}

function rolePageTabs(role: MiniProgramRoleId) {
  const pages = {
    user: {
      home: "/pages/user/UserHomePage",
      create: "/pages/user/UserCreationPage",
      assets: "/pages/user/UserAssetsPage",
      wallet: "/pages/user/UserWalletPage",
      mine: "/pages/user/UserMinePage",
    },
    agent: {
      overview: "/pages/agent/AgentOverviewPage",
      promotion: "/pages/agent/AgentPromotionPage",
      customers: "/pages/agent/AgentCustomersPage",
      commission: "/pages/agent/AgentCommissionPage",
      mine: "/pages/agent/AgentMinePage",
    },
    operation: {
      overview: "/pages/operation/OperationOverviewPage",
      agents: "/pages/operation/OperationAgentsPage",
      orders: "/pages/operation/OperationOrdersPage",
      commission: "/pages/operation/OperationCommissionPage",
      mine: "/pages/operation/OperationMinePage",
    },
  } as const;
  return pages[role];
}

export function createWorkbenchNavigation() {
  function replacePage(url: string) {
    const pages = getCurrentPages();
    const currentPage = pages[pages.length - 1] as { route?: string } | undefined;
    const targetRoute = url.replace(/^\//, "").split("?")[0];
    if (currentPage?.route === targetRoute) return;
    const primaryTabRoutes = new Set([
      ...Object.values(rolePageTabs("user")),
      ...Object.values(rolePageTabs("agent")),
      ...Object.values(rolePageTabs("operation")),
    ].map(path => path.replace(/^\//, "")));
    if (isWorkbenchNativeTab(url)) {
      uni.switchTab({ url, fail: () => uni.reLaunch({ url }) });
      return;
    }
    if (primaryTabRoutes.has(targetRoute)) {
      uni.reLaunch({ url });
      return;
    }
    uni.redirectTo({ url, fail: () => uni.reLaunch({ url }) });
  }

  function openStandalonePage(url: string) {
    if (!url) {
      uni.showToast({ title: "页面地址为空", icon: "none" });
      return;
    }
    const pages = getCurrentPages();
    const currentPage = pages[pages.length - 1] as { route?: string } | undefined;
    const targetRoute = url.replace(/^\//, "").split("?")[0];
    if (currentPage?.route === targetRoute) return;
    uni.navigateTo({
      url,
      fail() {
        uni.redirectTo({
          url,
          fail() {
            uni.reLaunch({ url });
          },
        });
      },
    });
  }

  function openFeaturePage(url: string) {
    openStandalonePage(url);
  }

  function openMineView(view: keyof typeof miniProgramMinePages) {
    replacePage(miniProgramMinePages[view]);
  }

  function openEnterprisePage(key: keyof typeof miniProgramEnterprisePages) {
    openStandalonePage(miniProgramEnterprisePages[key]);
  }

  function openRolePage(role: MiniProgramRoleId, tab: MiniProgramTabId) {
    replacePage(rolePage(role, tab));
  }

  return {
    replacePage,
    openStandalonePage,
    openFeaturePage,
    openMineView,
    openEnterprisePage,
    openRolePage,
  };
}
