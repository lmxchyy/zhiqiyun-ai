import { defineStore } from "pinia";
import { api, getAuthToken, setAuthToken } from "../api/client";
import type { AuthResponse, WorkspaceRole } from "../types";

interface AuthState {
  token: string;
  user: AuthResponse["user"] | null;
  workspace: WorkspaceRole;
  defaultModule: string;
}

function workspaceFromAuth(auth: AuthResponse): WorkspaceRole {
  if (auth.workspace === "agent" || auth.workspace === "admin" || auth.workspace === "user") return auth.workspace;
  if (auth.user.role.startsWith("AGENT")) return "agent";
  if (auth.user.role === "SUPER_ADMIN") return "admin";
  return "user";
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
      const auth = await api<AuthResponse>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password })
      });
      this.applyAuth(auth);
      return auth;
    },
    logout() {
      this.token = "";
      this.user = null;
      this.workspace = "user";
      this.defaultModule = "dashboard";
      setAuthToken("");
    }
  }
});

