import type { ApiEnvelope } from "@xianzhi/shared-types";
import { createUniPlatformAdapter, type PlatformAdapter, type PlatformClientInfo } from "@xianzhi/platform-adapter";

export interface ApiUnauthorizedContext {
  path: string;
  statusCode: number;
  requestId: string;
  payload: unknown;
  retryAttempt: number;
  authMode: ApiAuthMode;
}

export type ApiAuthMode = "none" | "optional" | "required";

export interface ApiClientErrorOptions {
  path: string;
  statusCode: number;
  requestId: string;
  payload?: unknown;
  apiCode?: unknown;
  cause?: unknown;
}

export interface ApiErrorMessageOptions {
  statusCode?: number;
  apiCode?: unknown;
  fallback?: string;
  payload?: unknown;
}

export class ApiClientError extends Error {
  readonly path: string;
  readonly statusCode: number;
  readonly requestId: string;
  readonly payload?: unknown;
  readonly apiCode?: unknown;
  readonly cause?: unknown;

  constructor(message: string, options: ApiClientErrorOptions) {
    super(message);
    this.name = "ApiClientError";
    this.path = options.path;
    this.statusCode = options.statusCode;
    this.requestId = options.requestId;
    this.payload = options.payload;
    this.apiCode = options.apiCode;
    this.cause = options.cause;
  }
}

const statusMessageMap: Record<number, string> = {
  400: "请求参数不正确，请检查后重试",
  401: "登录状态已失效，请重新登录",
  403: "暂无权限执行此操作",
  404: "请求的内容不存在或已被删除",
  405: "当前操作不受支持",
  408: "请求超时，请稍后重试",
  409: "当前数据状态已变化，请刷新后重试",
  413: "提交的内容过大，请调整后重试",
  415: "提交的文件或内容格式不受支持",
  422: "提交的信息不符合要求，请检查后重试",
  429: "操作过于频繁，请稍后重试",
  500: "服务器处理失败，请稍后重试",
  502: "服务暂时不可用，请稍后重试",
  503: "服务繁忙，请稍后重试",
  504: "服务响应超时，请稍后重试",
};

const apiCodeMessageMap: Record<string, string> = {
  AUTH_REQUIRED: "请先登录后再继续",
  UNAUTHORIZED: "登录状态已失效，请重新登录",
  FORBIDDEN: "暂无权限执行此操作",
  NOT_FOUND: "请求的内容不存在或已被删除",
  CONFLICT: "当前数据状态已变化，请刷新后重试",
  RATE_LIMITED: "操作过于频繁，请稍后重试",
  TOO_MANY_REQUESTS: "操作过于频繁，请稍后重试",
  INSUFFICIENT_BALANCE: "账户余额不足，请充值后重试",
  INSUFFICIENT_CREDITS: "可用额度不足，请充值或升级套餐",
  INSUFFICIENT_POINTS: "当前积分不足，请充值或升级套餐后再试",
  VALIDATION_FAILED: "提交的信息不符合要求，请检查后重试",
};

function containsChinese(value: string) {
  return /[\u3400-\u9fff]/.test(value);
}

function knownEnglishMessage(value: string) {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return "";
  if (/network error|failed to fetch|network request failed|request:fail|connection (?:refused|reset)|load failed/.test(normalized)) {
    return "网络连接失败，请检查网络后重试";
  }
  if (/timeout|timed out/.test(normalized)) return "请求超时，请稍后重试";
  if (/invalid (?:username|email|mobile|phone|account|password|credentials)|incorrect password|bad credentials/.test(normalized)) {
    return "账号或密码不正确";
  }
  if (/token.*(?:expired|invalid)|(?:expired|invalid).*token|session.*expired/.test(normalized)) {
    return "登录状态已失效，请重新登录";
  }
  if (/unauthorized|authentication required|not authenticated|please log in/.test(normalized)) {
    return "请先登录后再继续";
  }
  if (/not included in package|module .+ is not included/.test(normalized)) {
    return "当前套餐不支持该能力，请升级后重试";
  }
  if (/not allowed by tenant\/package limit|no models are allowed by tenant\/package limit/.test(normalized)) {
    return "当前套餐未开放该视频模型，请更换模型或联系管理员开通";
  }
  if (/not allowed by schema|parameter .+ is required|exceeds tenant\/package|is not in schema options/.test(normalized)) {
    return "提交的信息不符合要求，请检查后重试";
  }
  if (/upstream|api (?:provider|channel|配置)|not bound to an api provider|does not support this model/.test(normalized)) {
    return "视频模型上游渠道未启用，请先在主控后台完成 API 配置";
  }
  if (/forbidden|permission denied|access denied|not allowed/.test(normalized)) {
    return "暂无权限执行此操作";
  }
  if (/insufficient (?:balance|funds)|balance is not enough/.test(normalized)) {
    return "账户余额不足，请充值后重试";
  }
  if (/insufficient (?:credits?|quota)|quota exceeded/.test(normalized)) {
    return "可用额度不足，请充值或升级套餐";
  }
  if (/too many requests|rate limit|rate exceeded/.test(normalized)) {
    return "操作过于频繁，请稍后重试";
  }
  if (/already exists|duplicate|conflict/.test(normalized)) {
    return "该数据已存在，请勿重复提交";
  }
  if (/not found|does not exist|no such/.test(normalized)) {
    return "请求的内容不存在或已被删除";
  }
  if (/required|invalid|validation|malformed|bad request/.test(normalized)) {
    return "提交的信息不符合要求，请检查后重试";
  }
  if (/service unavailable|bad gateway|gateway timeout|internal server error/.test(normalized)) {
    return "服务暂时不可用，请稍后重试";
  }
  return "";
}

/**
 * Converts transport and server errors into text that is safe to display in a
 * Chinese UI. Original payloads remain available on ApiClientError for logs.
 */
export function toChineseApiErrorMessage(
  message: unknown,
  options: ApiErrorMessageOptions = {},
) {
  const source = String(message || "").trim();
  const apiCode = String(options.apiCode ?? "").trim().toUpperCase();
  if (apiCode === "INSUFFICIENT_POINTS") {
    const payload = options.payload && typeof options.payload === "object" ? options.payload as Record<string, unknown> : {};
    const current = Number(payload.currentPoints);
    const required = Number(payload.requiredPoints);
    if (Number.isFinite(current) && Number.isFinite(required) && current >= 0 && required > 0) {
      return `积分不足，当前 ${current} 积分，本次需要 ${required} 积分，还差 ${Math.max(0, required - current)} 积分。`;
    }
    return apiCodeMessageMap[apiCode];
  }
  if (containsChinese(source)) return source;

  if (apiCode && apiCodeMessageMap[apiCode]) return apiCodeMessageMap[apiCode];

  const knownMessage = knownEnglishMessage(source);
  if (knownMessage) return knownMessage;

  const statusCode = Number(options.statusCode || 0);
  if (statusMessageMap[statusCode]) return statusMessageMap[statusCode];
  if (statusCode >= 500) return "服务器处理失败，请稍后重试";
  if (statusCode >= 400) return "请求失败，请检查后重试";

  const fallback = String(options.fallback || "").trim();
  if (containsChinese(fallback)) return fallback;
  return "请求失败，请稍后重试";
}

export interface ApiRequestContext {
  path: string;
  method: string;
  requestId: string;
  clientInfo: PlatformClientInfo;
}

export interface ApiClientOptions {
  baseURL?: string;
  timeout?: number;
  clientName?: string;
  clientVersion?: string;
  defaultHeaders?: Record<string, string> | ((context: ApiRequestContext) => Record<string, string>);
  createRequestId?: () => string;
  getToken?: () => string;
  onUnauthorized?: (context: ApiUnauthorizedContext) => void | boolean | Promise<void | boolean>;
  adapter?: PlatformAdapter;
}

export interface ApiRequestOptions<TBody = unknown> {
  method?: string;
  headers?: Record<string, string>;
  body?: TBody;
  data?: TBody;
  timeout?: number;
  requestId?: string;
  /** Boolean compatibility: false=none, true=required. */
  auth?: boolean | ApiAuthMode;
  retryOnUnauthorized?: boolean;
}

export interface ApiClient {
  getBaseURL(): string;
  setBaseURL(baseURL: string): void;
  request<T = unknown, TBody = unknown>(path: string, options?: ApiRequestOptions<TBody>): Promise<T>;
}

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}

function resolveURL(baseURL: string, path: string) {
  if (/^https?:\/\//i.test(path)) return path;
  const normalizedBase = trimTrailingSlash(baseURL);
  if (!normalizedBase) return path;
  return `${normalizedBase}${path.startsWith("/") ? path : `/${path}`}`;
}

function normalizeApiError(error: unknown): Error {
  if (error instanceof Error) return error;
  if (error && typeof error === "object" && "errMsg" in error) {
    const errMsg = (error as { errMsg?: unknown }).errMsg;
    if (typeof errMsg === "string" && errMsg.trim()) return new Error(errMsg);
  }
  return new Error(String(error || ""));
}

function createDefaultRequestId() {
  const cryptoRuntime = globalThis.crypto;
  if (cryptoRuntime?.randomUUID) return cryptoRuntime.randomUUID();
  return `req_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
}

function addHeader(headers: Record<string, string>, key: string, value?: string) {
  if (value) headers[key] = value;
}

function isApiEnvelope<T>(payload: unknown): payload is ApiEnvelope<T> {
  return Boolean(payload && typeof payload === "object" && ("data" in payload || "code" in payload || "message" in payload || "error" in payload));
}

function isUnauthorizedApiCode(code: unknown) {
  return code === 401 || code === "401" || code === "UNAUTHORIZED" || code === "AUTH_REQUIRED";
}

function responseHeader(headers: Record<string, string> | undefined, name: string) {
  if (!headers) return "";
  const target = name.toLowerCase();
  const entry = Object.entries(headers).find(([key]) => key.toLowerCase() === target);
  return String(entry?.[1] || "");
}

function isUnexpectedHTMLResponse(payload: unknown, headers?: Record<string, string>) {
  const contentType = responseHeader(headers, "content-type").toLowerCase();
  if (contentType.includes("text/html")) return true;
  return typeof payload === "string" && /^\s*(?:<!doctype\s+html|<html\b)/i.test(payload);
}

export function createApiClient(options: ApiClientOptions = {}): ApiClient {
  let baseURL = trimTrailingSlash(options.baseURL || "");
  const timeout = options.timeout || 600000;
  const adapter = options.adapter || createUniPlatformAdapter();
  const requestIdFactory = options.createRequestId || createDefaultRequestId;

  return {
    getBaseURL() {
      return baseURL;
    },
    setBaseURL(nextBaseURL: string) {
      baseURL = trimTrailingSlash(nextBaseURL || "");
    },
    async request<T = unknown, TBody = unknown>(path: string, requestOptions: ApiRequestOptions<TBody> = {}) {
      const authMode: ApiAuthMode = requestOptions.auth === false || requestOptions.auth === "none"
        ? "none"
        : requestOptions.auth === true || requestOptions.auth === "required"
          ? "required"
          : "optional";
      const usesSession = authMode !== "none";
      const body = requestOptions.body ?? requestOptions.data;
      const method = requestOptions.method || "GET";
      const clientInfo = adapter.getClientInfo();

      const send = async (retryAttempt: number): Promise<T> => {
        const token = options.getToken?.() || "";
        const requestId = requestOptions.requestId || requestIdFactory();
        const context: ApiRequestContext = {
          path,
          method,
          requestId,
          clientInfo
        };
        const defaultHeaders = typeof options.defaultHeaders === "function" ? options.defaultHeaders(context) : options.defaultHeaders || {};
        const headers: Record<string, string> = {
          "Content-Type": "application/json",
          "X-Request-Id": requestId,
          "X-Client-Platform": clientInfo.platform,
          ...(requestOptions.headers || {})
        };
        addHeader(headers, "X-Client-Name", options.clientName || clientInfo.appName);
        addHeader(headers, "X-Client-Version", options.clientVersion || clientInfo.appVersion);
        addHeader(headers, "X-Client-Language", clientInfo.language);
        Object.assign(headers, defaultHeaders, requestOptions.headers || {});
        if (usesSession && token) headers.Authorization = `Bearer ${token}`;

        let response;
        try {
          response = await adapter.request<ApiEnvelope<T> | T, TBody>({
            url: resolveURL(baseURL, path),
            method,
            header: headers,
            data: body,
            timeout: requestOptions.timeout || timeout
          });
        } catch (error) {
          const normalized = normalizeApiError(error);
          throw new ApiClientError(toChineseApiErrorMessage(normalized.message), {
            path,
            statusCode: 0,
            requestId,
            cause: error
          });
        }

        const payload = response.data;
        if (usesSession && response.statusCode === 401) {
          const recovered = await options.onUnauthorized?.({ path, statusCode: response.statusCode, requestId, payload, retryAttempt, authMode });
          const retryAllowed = requestOptions.retryOnUnauthorized !== false && (method.toUpperCase() === "GET" || requestOptions.retryOnUnauthorized === true);
          if (recovered === true && retryAttempt === 0 && retryAllowed) return send(1);
        }
        if (response.statusCode < 200 || response.statusCode >= 300) {
          const message = isApiEnvelope<T>(payload) ? payload.message || payload.error : "";
          throw new ApiClientError(toChineseApiErrorMessage(message, {
            statusCode: response.statusCode,
            apiCode: isApiEnvelope<T>(payload) ? payload.code : undefined,
            payload,
          }), {
            path,
            statusCode: response.statusCode,
            requestId,
            payload,
            apiCode: isApiEnvelope<T>(payload) ? payload.code : undefined,
          });
        }

        if (isUnexpectedHTMLResponse(payload, response.header)) {
          throw new ApiClientError("服务端接口版本不匹配，请更新服务后重试", {
            path,
            statusCode: response.statusCode,
            requestId,
            payload,
            apiCode: "INVALID_API_RESPONSE"
          });
        }

        if (isApiEnvelope<T>(payload)) {
          const code = payload.code;
          if (code !== undefined && code !== 0 && code !== "0") {
            if (usesSession && isUnauthorizedApiCode(code)) {
              const recovered = await options.onUnauthorized?.({ path, statusCode: response.statusCode, requestId, payload, retryAttempt, authMode });
              const retryAllowed = requestOptions.retryOnUnauthorized !== false && (method.toUpperCase() === "GET" || requestOptions.retryOnUnauthorized === true);
              if (recovered === true && retryAttempt === 0 && retryAllowed) return send(1);
            }
            throw new ApiClientError(toChineseApiErrorMessage(payload.message || payload.error, {
              statusCode: response.statusCode,
              apiCode: code,
              payload,
            }), {
              path,
              statusCode: response.statusCode,
              requestId,
              payload,
              apiCode: code
            });
          }
          return (Object.prototype.hasOwnProperty.call(payload, "data") ? payload.data : payload) as T;
        }
        return payload as T;
      };

      return send(0);
    }
  };
}

let defaultClient = createApiClient();

export function configureApiClient(options: ApiClientOptions) {
  defaultClient = createApiClient(options);
  return defaultClient;
}

export function getDefaultApiClient() {
  return defaultClient;
}

export function api<T = unknown, TBody = unknown>(path: string, options?: ApiRequestOptions<TBody>) {
  return defaultClient.request<T, TBody>(path, options);
}
