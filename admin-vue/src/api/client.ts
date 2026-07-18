import axios, { type AxiosRequestConfig } from "axios";
import { clearWebAuthSession, getWebAccessToken, hasPersistentWebSessionMarker, hasWebSessionMarker, isPersistentWebSession, persistWebAccessToken } from "../utils/webAuthSession";

export type WebApiAuthMode = "none" | "optional" | "required";

export interface WebApiRequestConfig extends AxiosRequestConfig {
  authMode?: WebApiAuthMode;
  retryOnUnauthorized?: boolean;
  _authRetry?: boolean;
}

let refreshPromise: Promise<string> | null = null;

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
    throw error;
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
    return String(payload.error || payload.message || fallback);
  } catch {
    return raw.trim() || fallback;
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
    const message = error.response?.data?.error || error.response?.data?.message || error.message || "请求失败";
    return Promise.reject(new Error(message));
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
