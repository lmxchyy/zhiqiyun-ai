import { defineStore } from "pinia";
import { authService, authStorage, getAuthToken, setAuthToken } from "../api/client";
import { workspaceFromAuth } from "@xianzhi/shared-auth";
import type { AuthResponse, WorkspaceRole } from "../types";
import { useUserStore } from "./user";

interface AuthState {
  authStatus: "idle" | "loading" | "authenticated" | "error";
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
    authStatus: getAuthToken() ? "authenticated" : "idle",
    token: getAuthToken() || "",
    refreshToken: authStorage.getRefreshToken(),
    user: null,
    workspace: "user",
    defaultModule: "dashboard",
    isNewUser: false,
    loginMethod: "",
  }),
  getters: {
    isLoggedIn: (state) => Boolean(state.token)
  },
  actions: {
    applyAuth(auth: AuthResponse & { isNewUser?: boolean }, loginMethod: AuthState["loginMethod"] = "") {
      this.authStatus = "authenticated";
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
      this.authStatus = "loading";
      const auth = await authService.restore();
      if (auth) this.applyAuth(auth);
      else this.authStatus = "idle";
      return auth;
    },
    logout() {
      this.authStatus = "idle";
      this.token = "";
      this.refreshToken = "";
      this.user = null;
      this.workspace = "user";
      this.defaultModule = "dashboard";
      this.isNewUser = false;
      this.loginMethod = "";
      useUserStore().reset();
      authService.storage.clear();
    }
  }
});
