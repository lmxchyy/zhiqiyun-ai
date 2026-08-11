import type { PageAccess } from "@xianzhi/shared-auth";

const authenticatedPrefixes = [
  "/pages/agent/",
  "/pages/operation/",
  "/pages/enterprise/",
];

const authenticatedPages = new Set([
  "/pages/user/UserProfileEditPage",
  "/pages/user/UserWalletPage",
  "/pages/user/UserOrdersPage",
  "/pages/user/UserSettingsPage",
  "/pages/user/UserAgentCreationPage",
  "/pages/user/UserAssetDetailPage",
  "/pages/user/UserOrderConfirmPage",
  "/pages/user/UserVirtualPaymentPage",
  "/pages/user/UserVirtualPaymentTestPage",
]);

export function pageAccessFor(url: string): PageAccess {
  const path = String(url || "").split("?")[0];
  if (authenticatedPages.has(path) || authenticatedPrefixes.some(prefix => path.startsWith(prefix))) return "authenticated";
  if (path.includes("WechatLoginPage") || path.includes("ForbiddenPage")) return "public";
  return "guest-visible";
}
