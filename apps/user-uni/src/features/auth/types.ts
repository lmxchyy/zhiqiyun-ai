import type { AuthResponse } from "../../types";

export type LoginMode = "wechat" | "sms" | "password";

export type InviteStatus =
  | "empty"
  | "resolving"
  | "valid"
  | "invalid"
  | "expired"
  | "disabled"
  | "agent_frozen"
  | "carried"
  | "filled";

export type InviteSource = "query" | "scene" | "agent_code" | "promotion" | "manual" | "none";

export type LoadingStep =
  | "authorizing"
  | "validating"
  | "logging_in"
  | "registering"
  | "entering";

export type LoginErrorState =
  | "network"
  | "frozen"
  | "deactivated"
  | "maintenance"
  | "service"
  | "timeout"
  | "token"
  | "profile";

export interface LoginSourceParams {
  inviteCode: string;
  inviteSource: InviteSource;
  sceneCode: string;
  promoterCode: string;
  campaignCode: string;
  channel: string;
  sourcePage: string;
}

export interface LoginRedirectInfo {
  path: string;
  query: Record<string, string>;
  action: string;
  sourcePage: string;
}

export interface NewcomerBenefit {
  title?: string;
  description?: string;
  status?: string;
}

export interface AuthFlowResponse extends AuthResponse {
  isNewUser?: boolean;
  registrationStatus?: string;
  inviteBindStatus?: string;
  newcomerBenefits?: NewcomerBenefit[];
  nextAction?: string;
  expiresIn?: number;
}

export interface InviteValidationResponse {
  valid: boolean;
  status: InviteStatus;
  message?: string;
}

export interface SmsSendResponse {
  sent: boolean;
  retryAfterSeconds: number;
  expiresInSeconds?: number;
}

export interface AccountSecurityResponse {
  passwordSet: boolean;
  mobileMasked: string;
  mobileBound?: boolean;
  wechatLinked: boolean;
  loginMethods?: string[];
  status: string;
}

export interface BindMobileResponse {
  bound: boolean;
  user?: Record<string, unknown>;
  auth?: AuthResponse;
  security?: AccountSecurityResponse;
}

export interface AuthAttributionInput {
  inviteCode?: string;
  scene?: string;
  promoterCode?: string;
  campaignCode?: string;
  redirectSource?: string;
  idempotencyKey: string;
}
