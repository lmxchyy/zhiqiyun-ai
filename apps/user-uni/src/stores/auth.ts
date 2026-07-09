import { defineStore } from "pinia";
import { authService, getAuthToken, setAuthToken } from "../api/client";
import { workspaceFromAuth } from "@xianzhi/shared-auth";
import type { AuthResponse, WorkspaceRole } from "../types";

interface AuthState {
  token: string;
  user: AuthResponse["user"] | null;
  workspace: WorkspaceRole;
  defaultModule: string;
}



export const useAuthStore = defineStore("auth", {
  state: (): AuthState => ({
    token: getAuthToken() || "",
    user: null,
    workspace: "user",
    defaultModule: "dashboard"
  }),
  getters: {
    isLoggedIn: (state) => Boolean(state.token)
  },
  actions: {
    applyAuth(auth: AuthResponse) {
      this.token = auth.accessToken || "";
      this.user = auth.user;
      this.workspace = workspaceFromAuth(auth);
      this.defaultModule = auth.defaultModule || "dashboard";
      setAuthToken(this.token);
    },
    async login(email: string, password: string) {
      const auth = await authService.loginByPassword(email, password);
      this.applyAuth(auth);
      return auth;
    },
    async restore() {
      const auth = await authService.restore();
      if (auth) this.applyAuth(auth);
      return auth;
    },
    logout() {
      this.token = "";
      this.user = null;
      this.workspace = "user";
      this.defaultModule = "dashboard";
      authService.storage.clear();
    }
  }
});
