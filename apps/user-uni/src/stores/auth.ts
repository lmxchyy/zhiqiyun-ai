import { defineStore } from "pinia";
import { authService, authStorage, getAuthToken, setAuthToken } from "../api/client";
import { workspaceFromAuth, type AuthStatus, type PendingActionInput } from "@xianzhi/shared-auth";
import type { AuthResponse, WorkspaceRole } from "../types";
import { useUserStore } from "./user";
import { requireAuth as requireProtectedAction, resumePendingAction } from "../features/auth/gate";
import { acceptGuestBrowse } from "../features/auth/guestBrowse";

interface AuthState {
  status: AuthStatus;
  token: string;
  refreshToken: string;
  user: AuthResponse["user"] | null;
  workspace: WorkspaceRole;
  defaultModule: string;
  isNewUser: boolean;
  loginMethod: "wechat" | "sms" | "password" | "";
}



export const useAuthStore = defineStore("auth", {
  state: (): AuthState => ({
    status: getAuthToken() ? "authenticated" : "guest",
    token: getAuthToken() || "",
    refreshToken: authStorage.getRefreshToken(),
    user: null,
    workspace: "user",
    defaultModule: "dashboard",
    isNewUser: false,
    loginMethod: "",
  }),
  getters: {
    isLoggedIn: (state) => state.status === "authenticated" && Boolean(state.token),
    isGuest: (state) => state.status === "guest",
    isAuthenticated: (state) => state.status === "authenticated" && Boolean(state.token)
  },
  actions: {
    applyAuth(auth: AuthResponse & { isNewUser?: boolean }, loginMethod: AuthState["loginMethod"] = "") {
      this.status = "authenticated";
      this.token = auth.accessToken || "";
      this.refreshToken = auth.refreshToken || "";
      this.user = auth.user;
      this.workspace = workspaceFromAuth(auth);
      this.defaultModule = auth.defaultModule || "dashboard";
      this.isNewUser = Boolean(auth.isNewUser);
      this.loginMethod = loginMethod;
      setAuthToken(this.token);
      authStorage.setRefreshToken(this.refreshToken);
      authStorage.setAuth(auth);
      useUserStore().hydrateFromAuth(auth);
    },
    async login(email: string, password: string) {
      const auth = await authService.loginByPassword(email, password);
      this.applyAuth(auth);
      return auth;
    },
    async restore() {
      const auth = await authService.restore();
      if (auth) this.applyAuth(auth);
      else this.status = "guest";
      return auth;
    },
    initializeAuth() {
      return this.restore();
    },
    hasValidToken() {
      return this.status === "authenticated" && Boolean(this.token);
    },
    requireAuth(input: PendingActionInput) {
      return requireProtectedAction(input);
    },
    resumePendingAction() {
      return resumePendingAction();
    },
    handleTokenExpired() {
      this.status = "expired";
      this.token = "";
      this.refreshToken = "";
      authService.storage.clearSession();
      acceptGuestBrowse();
    },
    logout() {
      this.status = "guest";
      this.token = "";
      this.refreshToken = "";
      this.user = null;
      this.workspace = "user";
      this.defaultModule = "dashboard";
      this.isNewUser = false;
      this.loginMethod = "";
      useUserStore().reset();
      authService.storage.clear();
      acceptGuestBrowse();
    }
  }
});
