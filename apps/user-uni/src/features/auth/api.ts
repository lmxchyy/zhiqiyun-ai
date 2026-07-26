import { apiClient, authService } from "../../api/client";
import type {
  AccountSecurityResponse,
  AuthAttributionInput,
  AuthFlowResponse,
  BindMobileResponse,
  InviteValidationResponse,
  SmsSendResponse,
} from "./types";

export const loginAPI = {
  wechatPhoneLogin(input: AuthAttributionInput & { wxLoginCode: string; phoneCode: string }) {
    return apiClient.request<AuthFlowResponse>("/api/v1/auth/wechat/phone-login", {
      method: "POST",
      body: input,
      timeout: 20000,
      auth: false,
    });
  },

  sendSms(mobile: string, purpose = "login") {
    return apiClient.request<SmsSendResponse>("/api/v1/auth/sms/send", {
      method: "POST",
      body: { mobile, purpose },
      timeout: 15000,
      auth: false,
    });
  },

  smsLogin(input: AuthAttributionInput & { mobile: string; smsCode: string }) {
    return apiClient.request<AuthFlowResponse>("/api/v1/auth/sms/login", {
      method: "POST",
      body: input,
      timeout: 20000,
      auth: false,
    });
  },

  passwordLogin(account: string, password: string, idempotencyKey: string) {
    return apiClient.request<AuthFlowResponse>("/api/v1/auth/login", {
      method: "POST",
      body: { account, email: account, username: account, mobile: account, password, idempotencyKey },
      timeout: 15000,
      auth: false,
    });
  },

  validateInvite(inviteCode: string) {
    return apiClient.request<InviteValidationResponse>(
      `/api/v1/invite/agent/resolve?inviteCode=${encodeURIComponent(inviteCode)}`,
      { timeout: 10000, auth: false },
    );
  },

  validateInviteToken(inviteToken: string) {
    return apiClient.request<InviteValidationResponse & { inviteCode?: string; inviteToken?: string }>(
      `/api/v1/invite/resolve?inviteToken=${encodeURIComponent(inviteToken)}`,
      { timeout: 10000, auth: false },
    );
  },

  security() {
    return apiClient.request<AccountSecurityResponse>("/api/v1/auth/security");
  },

  bindMobile(mobile: string, smsCode: string) {
    return apiClient.request<BindMobileResponse>("/api/v1/auth/mobile/bind", {
      method: "POST",
      body: { mobile, smsCode },
      timeout: 15000,
    });
  },

  linkWechat(wxLoginCode: string) {
    return apiClient.request<{ linked: boolean; userId: string }>("/api/v1/auth/wechat-mini-program/link", {
      method: "POST",
      body: { wxLoginCode },
      timeout: 15000,
    });
  },

  changePassword(currentPassword: string, newPassword: string) {
    return apiClient.request<{ ok: boolean; passwordSet: boolean }>("/api/v1/auth/change-password", {
      method: "POST",
      body: { currentPassword, newPassword },
    });
  },

  logout() {
    return apiClient.request<{ ok: boolean }>("/api/v1/auth/logout", {
      method: "POST",
      body: { refreshToken: authService.storage.getRefreshToken() },
      retryOnUnauthorized: false,
    });
  },

  logoutAll() {
    return apiClient.request<{ ok: boolean; userId?: string; revokedSessions?: number }>("/api/v1/auth/logout-all", {
      method: "POST",
    });
  },
};
