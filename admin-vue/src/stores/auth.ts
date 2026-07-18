import { defineStore } from "pinia";
import { createPendingActionStore, safeInternalRedirect, type PendingAction, type PendingActionInput } from "@xianzhi/shared-auth";
import { createUniPlatformAdapter } from "@xianzhi/platform-adapter";
import { adminRequest, refreshWebAuthSession } from "../api/client";
import {
  clearWebAuthSession,
  getWebAccessToken,
  hasWebSessionMarker,
  isPersistentWebSession,
  onWebAuthChanged,
  persistWebAccessToken,
  type WebAuthStatus
} from "../utils/webAuthSession";

export interface WebAuthUser {
  id: string;
  email?: string;
  name?: string;
  role: string;
  status?: string;
}

export interface WebAuthResponse {
  accessToken?: string;
  user: WebAuthUser;
  permissions?: string[];
  workspace?: string;
  availableWorkspaces?: string[];
  defaultModule?: string;
  defaultRoute?: string;
  [key: string]: unknown;
}

interface WebAuthState {
  status: WebAuthStatus;
  accessToken: string;
  currentUser: WebAuthUser | null;
  authLoading: boolean;
  currentWorkspace: string;
  availableWorkspaces: string[];
  loginModalVisible: boolean;
  redirectUrl: string;
  pendingAction: PendingAction | null;
  sessionExpired: boolean;
  initialized: boolean;
  authResponse: WebAuthResponse | null;
}

let disposeAuthListener: (() => void) | null = null;
const pendingActions = createPendingActionStore({
  adapter: createUniPlatformAdapter("web"),
  storageKey: "zhiqiyun.web.pending-action.v1",
  ttlMs: 30 * 60 * 1000
});

export const useWebAuthStore = defineStore("webAuth", {
  state: (): WebAuthState => ({
    status: getWebAccessToken() ? "authenticated" : "guest",
    accessToken: getWebAccessToken(),
    currentUser: null,
    authLoading: false,
    currentWorkspace: "user",
    availableWorkspaces: [],
    loginModalVisible: false,
    redirectUrl: "",
    pendingAction: pendingActions.get(),
    sessionExpired: false,
    initialized: false,
    authResponse: null
  }),
  getters: {
    isAuthenticated: (state) => state.status === "authenticated" && Boolean(state.accessToken),
    isGuest: (state) => state.status !== "authenticated"
  },
  actions: {
    installSessionSync() {
      if (disposeAuthListener) return;
      disposeAuthListener = onWebAuthChanged((status) => {
        this.accessToken = getWebAccessToken();
        this.status = status;
        this.sessionExpired = status === "expired";
        if (status !== "authenticated") this.currentUser = null;
        if (!this.accessToken && hasWebSessionMarker()) void this.initializeAuth();
      });
    },
    applyAuth(response: WebAuthResponse, remember = true) {
      const token = String(response.accessToken || getWebAccessToken()).trim();
      if (!token) throw new Error("登录成功但没有返回访问令牌");
      persistWebAccessToken(token, remember);
      this.accessToken = token;
      this.currentUser = response.user;
      this.authResponse = response;
      this.currentWorkspace = String(response.workspace || "user");
      this.availableWorkspaces = Array.isArray(response.availableWorkspaces)
        ? response.availableWorkspaces.map(String)
        : [this.currentWorkspace];
      this.status = "authenticated";
      this.sessionExpired = false;
      this.loginModalVisible = false;
    },
    async initializeAuth() {
      if (this.authLoading) return this.currentUser;
      this.installSessionSync();
      this.authLoading = true;
      const hadSession = Boolean(getWebAccessToken()) || hasWebSessionMarker();
      try {
        if (!getWebAccessToken() && hasWebSessionMarker()) await refreshWebAuthSession();
        if (!getWebAccessToken()) {
          this.clear("guest");
          return null;
        }
        const response = await adminRequest<WebAuthResponse>({ method: "GET", url: "/auth/me", authMode: "optional" });
        this.applyAuth(response, isPersistentWebSession());
        return response.user;
      } catch {
        this.clear(hadSession ? "expired" : "guest");
        return null;
      } finally {
        this.authLoading = false;
        this.initialized = true;
      }
    },
    openLogin(options: { redirectUrl?: string; pendingAction?: PendingAction } = {}) {
      this.redirectUrl = safeInternalRedirect(options.redirectUrl || this.redirectUrl || currentInternalRoute(), "/");
      if (options.pendingAction) this.pendingAction = options.pendingAction;
      this.loginModalVisible = true;
    },
    requireAuth(input: PendingActionInput) {
      if (this.isAuthenticated) return true;
      this.pendingAction = pendingActions.save({
        ...input,
        route: safeInternalRedirect(input.route || currentInternalRoute(), "/")
      });
      this.openLogin({ redirectUrl: this.pendingAction.route, pendingAction: this.pendingAction });
      return false;
    },
    refreshPendingAction() {
      this.pendingAction = pendingActions.get();
      return this.pendingAction;
    },
    consumePendingAction() {
      const pending = pendingActions.consume();
      this.pendingAction = null;
      return pending;
    },
    clearPendingAction() {
      pendingActions.clear();
      this.pendingAction = null;
    },
    closeLogin() {
      this.loginModalVisible = false;
    },
    handleTokenExpired() {
      this.clear("expired");
    },
    clear(status: WebAuthStatus = "guest") {
      clearWebAuthSession(status);
      this.status = status;
      this.accessToken = "";
      this.currentUser = null;
      this.authResponse = null;
      this.currentWorkspace = "user";
      this.availableWorkspaces = [];
      this.sessionExpired = status === "expired";
    },
    async logout() {
      await adminRequest({ method: "POST", url: "/auth/logout", authMode: "optional", retryOnUnauthorized: false }).catch(() => undefined);
      this.clear("guest");
      this.clearPendingAction();
      this.redirectUrl = "";
      this.loginModalVisible = false;
    }
  }
});

function currentInternalRoute() {
  if (typeof window === "undefined") return "/";
  return `${window.location.pathname}${window.location.search}${window.location.hash}`;
}
