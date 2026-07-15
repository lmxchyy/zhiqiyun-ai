import {
  miniProgramCreationPages,
  miniProgramDefaultPage,
  miniProgramEnterprisePages,
  miniProgramFeaturePages,
  miniProgramMinePages,
  miniProgramRolePages,
} from "../../config/miniProgramPages";
import type { AppRole } from "../../types";
import type { LoginRedirectInfo } from "./types";

const tabPages = new Set([
  "/pages/user/UserHomePage",
  "/pages/user/UserCreationPage",
  "/pages/user/UserAssetsPage",
  "/pages/user/UserMinePage",
]);

const allowedPages = new Set<string>([
  ...Object.values(miniProgramCreationPages),
  ...Object.values(miniProgramFeaturePages),
  ...Object.values(miniProgramMinePages),
  ...Object.values(miniProgramEnterprisePages),
  ...Object.values(miniProgramRolePages).flatMap(item => Object.values(item)),
].filter((item): item is string => Boolean(item)));

function roleLanding(role: AppRole): string {
  if (role === "AGENT") return "/pages/agent/AgentOverviewPage";
  if (role === "OPERATION") return "/pages/operation/OperationOverviewPage";
  return miniProgramDefaultPage;
}

export function safeRedirectPath(path: string): string {
  const clean = String(path || "").split("?")[0];
  return allowedPages.has(clean) ? clean : "";
}

export function redirectAfterAuth(info: LoginRedirectInfo, role: AppRole, onFailure?: (error: unknown) => void) {
  const requested = safeRedirectPath(info.path);
  const path = requested || roleLanding(role);
  const query = requested
    ? Object.entries(info.query).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`).join("&")
    : "";
  const url = query ? `${path}?${query}` : path;
  const fail = (error: unknown) => {
    if (path !== miniProgramDefaultPage) {
      uni.switchTab({ url: miniProgramDefaultPage, fail: onFailure });
      return;
    }
    onFailure?.(error);
  };
  if (tabPages.has(path)) {
    uni.switchTab({ url: path, fail: error => uni.reLaunch({ url: path, fail: () => fail(error) }) });
    return;
  }
  uni.reLaunch({ url, fail });
}
