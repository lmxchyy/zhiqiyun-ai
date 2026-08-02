import { createAuthGate, createPendingActionStore, type AuthStatus, type PendingActionInput } from "@xianzhi/shared-auth";
import { createUniPlatformAdapter } from "@xianzhi/platform-adapter";
import { authStorage } from "../../api/client";
import { trackLogin } from "./analytics";
import { acceptGuestBrowse, clearGuestBrowse, isLoginPromptSuppressed, suppressLoginPrompt } from "./guestBrowse";

const adapter = createUniPlatformAdapter();
export const pendingActions = createPendingActionStore({ adapter });

let expired = false;

export function authStatus(): AuthStatus {
  if (authStorage.getToken()) return "authenticated";
  return expired ? "expired" : "guest";
}

export function initializeAuth() {
  expired = false;
  pendingActions.get();
  if (authStorage.getToken()) {
    clearGuestBrowse();
    trackLogin("authenticated_open_app");
  } else {
    acceptGuestBrowse();
    trackLogin("guest_open_app");
  }
  return authStatus();
}

export function hasValidToken() {
  return authStatus() === "authenticated";
}

export function handleTokenExpired() {
  expired = true;
  authStorage.clearSession();
  acceptGuestBrowse();
}

function currentRoute() {
  const pages = typeof getCurrentPages === "function" ? getCurrentPages() : [];
  const current = pages[pages.length - 1] as { route?: string; options?: Record<string, unknown> } | undefined;
  const path = current?.route ? `/${String(current.route).replace(/^\/+/, "")}` : "/pages/user/UserHomePage";
  const query = Object.fromEntries(Object.entries(current?.options || {}).map(([key, value]) => [key, String(value ?? "")]));
  return { path, query };
}

function openLoginPage() {
  return new Promise<void>(resolve => {
    if (isLoginPromptSuppressed()) {
      trackLogin("login_prompt_suppressed");
      resolve();
      return;
    }
    trackLogin("login_modal_show");
    uni.showModal({
      title: "\u767b\u5f55\u540e\u7ee7\u7eed\u4f7f\u7528",
      content: "\u767b\u5f55\u540e\u53ef\u4fdd\u5b58\u5386\u53f2\u4f5c\u54c1\u3001\u540c\u6b65\u521b\u4f5c\u8bb0\u5f55\u3001\u67e5\u770b\u8d26\u6237\u989d\u5ea6\uff0c\u5e76\u7ee7\u7eed\u521a\u624d\u7684\u521b\u4f5c\u3002",
      confirmText: "\u7acb\u5373\u767b\u5f55",
      cancelText: "\u6682\u4e0d\u767b\u5f55",
      confirmColor: "#4A6BFF",
      success(result) {
        if (!result.confirm) {
          trackLogin("login_cancel");
          acceptGuestBrowse();
          suppressLoginPrompt();
          pendingActions.clear();
          if (adapter.platform === "web" && typeof window !== "undefined") window.dispatchEvent(new CustomEvent("zhiqiyun:auth-cancelled"));
          resolve();
          return;
        }
        trackLogin("login_start");
        if (adapter.platform === "web" && typeof window !== "undefined") {
          window.history.pushState({ auth: "login" }, "", "/login");
          window.dispatchEvent(new PopStateEvent("popstate"));
          resolve();
          return;
        }
        const route = currentRoute();
        const query = Object.entries(route.query).map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`).join("&");
        const url = `/pages/WechatLoginPage?redirectPath=${encodeURIComponent(route.path)}&redirectQuery=${encodeURIComponent(query)}&sourcePage=${encodeURIComponent(route.path)}&pendingAction=1`;
        uni.navigateTo({ url, complete: () => resolve(), fail: () => uni.redirectTo({ url }) });
      },
      fail: () => resolve(),
    });
  });
}

const gate = createAuthGate({
  getStatus: authStatus,
  pendingActions,
  openLogin: openLoginPage,
});

export function requireAuth(input: PendingActionInput) {
  if (authStatus() !== "authenticated" && isLoginPromptSuppressed()) {
    trackLogin("login_prompt_suppressed", { action: input.action });
    return Promise.resolve(false);
  }
  return gate.requireAuth(input);
}

export async function resumePendingAction() {
  try {
    const resumed = await gate.resumePendingAction();
    trackLogin(resumed ? "pending_action_resume_success" : "pending_action_resume_failed", { reason: resumed ? "resumed" : "missing_callback" });
    return resumed;
  } catch {
    trackLogin("pending_action_resume_failed", { reason: "execution_error" });
    return false;
  }
}

export function clearPendingAction() {
  gate.clearPendingAction();
}
