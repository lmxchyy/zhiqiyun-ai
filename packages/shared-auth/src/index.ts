import type { ApiClient } from "@xianzhi/api-client";
import type { AuthResponse, WorkspaceRole } from "@xianzhi/shared-types";
import type { PlatformAdapter } from "@xianzhi/platform-adapter";

export interface AuthStorageOptions {
  tokenKey: string;
  refreshTokenKey?: string;
  authKey?: string;
  adapter: PlatformAdapter;
}

export interface AuthServiceOptions extends AuthStorageOptions {
  api: ApiClient;
  wechatMockCode?: string;
}

export interface RegisterByInviteInput {
  username: string;
  email: string;
  password: string;
  confirmPassword: string;
  inviteCode: string;
}

function persistAuth(storage: ReturnType<typeof createAuthStorage>, auth: AuthResponse) {
  storage.setToken(auth.accessToken || "");
  if (Object.prototype.hasOwnProperty.call(auth, "refreshToken")) {
    storage.setRefreshToken(auth.refreshToken || "");
  }
  storage.setAuth(auth);
  return auth;
}

export function workspaceFromAuth(auth: AuthResponse): WorkspaceRole {
  if (auth.workspace === "agent" || auth.workspace === "admin" || auth.workspace === "user") return auth.workspace;
  if (auth.user.role.startsWith("AGENT")) return "agent";
  if (auth.user.role === "SUPER_ADMIN") return "admin";
  return "user";
}

export function createAuthStorage(options: AuthStorageOptions) {
  return {
    getToken() {
      return options.adapter.getStorage<string>(options.tokenKey) || "";
    },
    setToken(token: string) {
      if (token) options.adapter.setStorage(options.tokenKey, token);
      else options.adapter.removeStorage(options.tokenKey);
    },
    getRefreshToken() {
      return options.refreshTokenKey ? options.adapter.getStorage<string>(options.refreshTokenKey) || "" : "";
    },
    setRefreshToken(token: string) {
      if (!options.refreshTokenKey) return;
      if (token) options.adapter.setStorage(options.refreshTokenKey, token);
      else options.adapter.removeStorage(options.refreshTokenKey);
    },
    getAuth() {
      return options.authKey ? options.adapter.getStorage<AuthResponse>(options.authKey) || null : null;
    },
    setAuth(auth: AuthResponse | null) {
      if (!options.authKey) return;
      if (auth) options.adapter.setStorage(options.authKey, auth);
      else options.adapter.removeStorage(options.authKey);
    },
    clearSession() {
      options.adapter.removeStorage(options.tokenKey);
      if (options.authKey) options.adapter.removeStorage(options.authKey);
    },
    clear() {
      options.adapter.removeStorage(options.tokenKey);
      if (options.refreshTokenKey) options.adapter.removeStorage(options.refreshTokenKey);
      if (options.authKey) options.adapter.removeStorage(options.authKey);
    }
  };
}

export function createAuthService(options: AuthServiceOptions) {
  const storage = createAuthStorage(options);
  let refreshPromise: Promise<AuthResponse> | null = null;
  const service = {
    storage,
    async loginByPassword(email: string, password: string) {
      const auth = await options.api.request<AuthResponse, { email: string; password: string }>("/api/v1/auth/login", {
        method: "POST",
        body: { email, password },
        auth: false
      });
      return persistAuth(storage, auth);
    },
    async loginByWechatMiniProgramCode(code: string) {
      const auth = await options.api.request<AuthResponse, { code: string }>("/api/v1/auth/wechat-mini-program/login", {
        method: "POST",
        body: { code },
        auth: false
      });
      return persistAuth(storage, auth);
    },
    async loginByWechatMiniProgram() {
      const loginResult = await options.adapter.login?.("weixin").catch(() => null);
      const code = String(loginResult?.code || options.wechatMockCode || "").trim();
      if (!code) {
        throw new Error("wechat login did not return a code");
      }
      const auth = await options.api.request<AuthResponse, { code: string }>("/api/v1/auth/wechat-mini-program/login", {
        method: "POST",
        body: { code },
        auth: false
      });
      return persistAuth(storage, auth);
    },
    async registerByInvite(input: RegisterByInviteInput) {
      const auth = await options.api.request<AuthResponse, RegisterByInviteInput>("/api/v1/auth/register", {
        method: "POST",
        body: input,
        auth: false
      });
      return persistAuth(storage, auth);
    },
    async me() {
      const auth = await options.api.request<AuthResponse>("/api/v1/auth/me");
      storage.setAuth(auth);
      if (auth.accessToken) storage.setToken(auth.accessToken);
      return auth;
    },
    async refresh() {
      if (refreshPromise) return refreshPromise;
      const refreshToken = storage.getRefreshToken();
      if (!refreshToken) throw new Error("refresh token is missing");
      refreshPromise = options.api.request<AuthResponse, { refreshToken: string }>("/api/v1/auth/refresh", {
        method: "POST",
        body: { refreshToken },
        auth: false
      }).then(auth => persistAuth(storage, auth)).finally(() => { refreshPromise = null; });
      return refreshPromise;
    },
    async restore() {
      if (storage.getToken()) {
        try {
          return await service.me();
        } catch {
          // Fall through to refresh token recovery.
        }
      }
      if (storage.getRefreshToken()) {
        try {
          return await service.refresh();
        } catch {
          storage.clear();
        }
      }
      return null;
    },
    async logout() {
      await options.api.request<{ ok?: boolean }>("/api/v1/auth/logout", { method: "POST" }).catch(() => null);
      storage.clear();
    }
  };
  return service;
}
