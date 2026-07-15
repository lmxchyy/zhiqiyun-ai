import { apiClient } from "../../api/client";
import type {
  AccountSecurityResponse,
  AuthAttributionInput,
  AuthFlowResponse,
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

  sendSms(mobile: string) {
    return apiClient.request<SmsSendResponse>("/api/v1/auth/sms/send", {
      method: "POST",
      body: { mobile, purpose: "login" },
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

  security() {
    return apiClient.request<AccountSecurityResponse>("/api/v1/auth/security");
  },

  changePassword(currentPassword: string, newPassword: string) {
    return apiClient.request<{ ok: boolean; passwordSet: boolean }>("/api/v1/auth/change-password", {
      method: "POST",
      body: { currentPassword, newPassword },
    });
  },
};
