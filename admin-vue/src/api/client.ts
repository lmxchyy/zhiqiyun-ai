import axios, { type AxiosRequestConfig } from "axios";
import { clearWebAuthSession, getWebAccessToken, hasPersistentWebSessionMarker, hasWebSessionMarker, isPersistentWebSession, persistWebAccessToken } from "../utils/webAuthSession.ts";

export type WebApiAuthMode = "none" | "optional" | "required";

export interface WebApiRequestConfig extends AxiosRequestConfig {
  authMode?: WebApiAuthMode;
  retryOnUnauthorized?: boolean;
  _authRetry?: boolean;
}

export interface AdminApiErrorPayload {
  code?: string;
  error?: string;
  message?: string;
  [key: string]: unknown;
}

export class AdminApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly payload: unknown;

  constructor(message: string, status = 0, code = "", payload: unknown = undefined) {
    super(message);
    this.name = "AdminApiError";
    this.status = status;
    this.code = code;
    this.payload = payload;
  }
}

let refreshPromise: Promise<string> | null = null;

const statusMessages: Record<number, string> = {
  400: "请求参数不正确，请检查后重试",
  401: "登录状态已失效，请重新登录",
  403: "暂无权限执行此操作",
  404: "请求的内容不存在或已被删除",
  409: "当前数据状态已变化，请刷新后重试",
  413: "提交的内容过大，请调整后重试",
  422: "提交的信息不符合要求，请检查后重试",
  429: "操作过于频繁，请稍后重试",
  500: "服务器处理失败，请稍后重试",
  502: "服务暂时不可用，请稍后重试",
  503: "服务繁忙，请稍后重试",
  504: "服务响应超时，请稍后重试"
};

export function chineseAdminErrorMessage(message: unknown, status = 0, fallback = "请求失败，请稍后重试") {
  const source = String(message || "").trim();
  if (/[\u3400-\u9fff]/.test(source)) return source;
  const normalized = source.toLowerCase();
  if (/network error|failed to fetch|network request failed|connection (?:refused|reset)/.test(normalized)) return "网络连接失败，请检查网络后重试";
  if (/timeout|timed out/.test(normalized)) return "请求超时，请稍后重试";
  if (/invalid (?:username|email|mobile|phone|account|password|credentials)|incorrect password|bad credentials/.test(normalized)) return "账号或密码不正确";
  if (/token.*(?:expired|invalid)|(?:expired|invalid).*token|session.*expired/.test(normalized)) return "登录状态已失效，请重新登录";
  if (/unauthorized|authentication required|not authenticated|please log in/.test(normalized)) return "请先登录后再继续";
  if (/not included in package|module .+ is not included/.test(normalized)) return "当前套餐不支持该能力，请升级后重试";
  if (/not allowed by tenant\/package limit|no models are allowed by tenant\/package limit/.test(normalized)) return "当前套餐未开放该视频模型，请更换模型或联系管理员开通";
  if (/not allowed by schema|parameter .+ is required|exceeds tenant\/package|is not in schema options/.test(normalized)) return "提交的信息不符合要求，请检查后重试";
  if (/upstream|api (?:provider|channel)|not bound to an api provider|does not support this model/.test(normalized)) return "视频模型上游渠道未启用，请先在主控后台完成 API 配置";
  if (/forbidden|permission denied|access denied|not allowed/.test(normalized)) return "暂无权限执行此操作";
  if (/too many requests|rate limit/.test(normalized)) return "操作过于频繁，请稍后重试";
  if (/already exists|duplicate|conflict/.test(normalized)) return "该数据已存在，请勿重复提交";
  if (/not found|does not exist|no such/.test(normalized)) return "请求的内容不存在或已被删除";
  if (/required|invalid|validation|malformed|bad request/.test(normalized)) return "提交的信息不符合要求，请检查后重试";
  if (statusMessages[status]) return statusMessages[status];
  if (status >= 500) return "服务器处理失败，请稍后重试";
  if (status >= 400) return "请求失败，请检查后重试";
  return /[\u3400-\u9fff]/.test(fallback) ? fallback : "请求失败，请稍后重试";
}

export function createAdminApiError(payload: unknown, status = 0, fallback = "请求失败，请稍后重试"): AdminApiError {
  const record = payload && typeof payload === "object" && !Array.isArray(payload)
    ? payload as AdminApiErrorPayload
    : {};
  const source = record.error || record.message || payload;
  const code = typeof record.code === "string" ? record.code.trim() : "";
  return new AdminApiError(chineseAdminErrorMessage(source, status, fallback), status, code, payload);
}

function requestAuthMode(config: WebApiRequestConfig): WebApiAuthMode {
  if (config.authMode) return config.authMode;
  const url = String(config.url || "");
  if (/\/auth\/(?:login|register|sms\/send|sms\/login|refresh)$/.test(url)) return "none";
  return "optional";
}

function canReplayAfterRefresh(config: WebApiRequestConfig) {
  if (config.retryOnUnauthorized === false) return false;
  if (config.retryOnUnauthorized === true) return true;
  return ["GET", "HEAD", "OPTIONS"].includes(String(config.method || "GET").toUpperCase());
}

export async function refreshWebAuthSession() {
  if (refreshPromise) return refreshPromise;
  const remember = typeof window === "undefined" || isPersistentWebSession() || hasPersistentWebSessionMarker();
  refreshPromise = axios.post<{ accessToken?: string }>("/api/v1/auth/refresh", {}, {
    headers: { Accept: "application/json" },
    timeout: 30000,
    withCredentials: true
  }).then((response) => {
    const token = String(response.data?.accessToken || "").trim();
    if (!token) throw new Error("刷新登录状态失败：未返回访问令牌");
    persistWebAccessToken(token, remember);
    return token;
  }).catch((error) => {
    clearWebAuthSession("expired");
    const status = Number(axios.isAxiosError(error) ? error.response?.status || 0 : 0);
    const message = axios.isAxiosError(error)
      ? error.response?.data?.error || error.response?.data?.message || error.message
      : error instanceof Error ? error.message : error;
    throw new Error(chineseAdminErrorMessage(message, status, "刷新登录状态失败，请重新登录"));
  }).finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
}

function isSameOriginRequest(input: RequestInfo | URL) {
  if (typeof input !== "string") return true;
  if (!/^https?:\/\//i.test(input)) return true;
  if (typeof window === "undefined") return false;
  try {
    return new URL(input, window.location.origin).origin === window.location.origin;
  } catch {
    return false;
  }
}

async function adminResponseError(response: Response) {
  const fallback = `请求失败 (${response.status})`;
  const raw = await response.text().catch(() => "");
  if (!raw) return fallback;
  try {
    const payload = JSON.parse(raw) as { error?: unknown; message?: unknown };
    return chineseAdminErrorMessage(payload.error || payload.message, response.status, fallback);
  } catch {
    return chineseAdminErrorMessage(raw, response.status, fallback);
  }
}

export const apiClient = axios.create({
  baseURL: "/api/v1",
  timeout: 180000,
  withCredentials: true,
  headers: { Accept: "application/json" }
});

apiClient.interceptors.request.use((config) => {
  const authMode = requestAuthMode(config as WebApiRequestConfig);
  const token = authMode === "none" ? "" : getWebAccessToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

apiClient.interceptors.response.use(
  (response) => response.data,
  async (error) => {
    if (axios.isCancel(error)) return Promise.reject(error);
    const config = (error.config || {}) as WebApiRequestConfig;
    const status = Number(error.response?.status || 0);
    const authMode = requestAuthMode(config);
    const requestHadSession = Boolean(config.headers?.Authorization) || hasWebSessionMarker();
    if (status === 401 && authMode !== "none" && !config._authRetry && requestHadSession) {
      config._authRetry = true;
      try {
        const token = await refreshWebAuthSession();
        if (canReplayAfterRefresh(config)) {
          config.headers = { ...config.headers, Authorization: `Bearer ${token}` };
          return apiClient.request(config);
        }
      } catch {
        // refreshWebAuthSession already cleared the invalid local session.
      }
    }
    const payload = error.response?.data ?? error.message;
    return Promise.reject(createAdminApiError(payload, status));
  }
);

export async function adminRequest<T>(config: WebApiRequestConfig): Promise<T> {
  return (await apiClient.request(config)) as T;
}

export async function adminFetchResponse(
  input: RequestInfo | URL,
  init: RequestInit = {},
  options: { auth?: boolean; authMode?: WebApiAuthMode; retryOnUnauthorized?: boolean } = {}
) {
  const authMode = options.auth === false ? "none" : options.authMode || (isSameOriginRequest(input) ? "optional" : "none");
  const method = String(init.method || "GET").toUpperCase();
  const request = async (token = getWebAccessToken()) => {
    const headers = new Headers(init.headers);
    if (authMode !== "none" && token && !headers.has("Authorization")) headers.set("Authorization", `Bearer ${token}`);
    return fetch(input, { ...init, headers, credentials: init.credentials || "same-origin" });
  };
  let response = await request();
  const replayAllowed = options.retryOnUnauthorized === true
    || (options.retryOnUnauthorized !== false && ["GET", "HEAD", "OPTIONS"].includes(method));
  if (response.status === 401 && authMode !== "none" && (getWebAccessToken() || hasWebSessionMarker())) {
    try {
      const token = await refreshWebAuthSession();
      if (replayAllowed) response = await request(token);
    } catch {
      // Keep the original 401 response for the common error mapping below.
    }
  }
  if (!response.ok) throw new Error(await adminResponseError(response));
  return response;
}
