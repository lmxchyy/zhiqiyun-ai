import type { ApiEnvelope } from "@xianzhi/shared-types";
import { createUniPlatformAdapter, type PlatformAdapter, type PlatformClientInfo } from "@xianzhi/platform-adapter";

export interface ApiUnauthorizedContext {
  path: string;
  statusCode: number;
  requestId: string;
  payload: unknown;
}

export interface ApiClientErrorOptions {
  path: string;
  statusCode: number;
  requestId: string;
  payload?: unknown;
  apiCode?: unknown;
  cause?: unknown;
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
  onUnauthorized?: (context: ApiUnauthorizedContext) => void;
  adapter?: PlatformAdapter;
}

export interface ApiRequestOptions<TBody = unknown> {
  method?: string;
  headers?: Record<string, string>;
  body?: TBody;
  data?: TBody;
  timeout?: number;
  requestId?: string;
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
  return new Error(String(error || "Request failed"));
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
  return Boolean(payload && typeof payload === "object" && ("data" in payload || "code" in payload || "message" in payload));
}

function isUnauthorizedApiCode(code: unknown) {
  return code === 401 || code === "401" || code === "UNAUTHORIZED" || code === "AUTH_REQUIRED";
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
      const token = options.getToken?.() || "";
      const body = requestOptions.body ?? requestOptions.data;
      const method = requestOptions.method || "GET";
      const requestId = requestOptions.requestId || requestIdFactory();
      const clientInfo = adapter.getClientInfo();
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
      if (token) headers.Authorization = `Bearer ${token}`;

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
        throw new ApiClientError(normalized.message, {
          path,
          statusCode: 0,
          requestId,
          cause: error
        });
      }

      const payload = response.data;
      if (response.statusCode === 401) {
        options.onUnauthorized?.({ path, statusCode: response.statusCode, requestId, payload });
      }
      if (response.statusCode < 200 || response.statusCode >= 300) {
        const message = isApiEnvelope<T>(payload) ? payload.message || payload.error : "";
        throw new ApiClientError(message || `HTTP ${response.statusCode}`, {
          path,
          statusCode: response.statusCode,
          requestId,
          payload
        });
      }

      if (isApiEnvelope<T>(payload)) {
        const code = payload.code;
        if (code !== undefined && code !== 0 && code !== "0") {
          if (isUnauthorizedApiCode(code)) {
            options.onUnauthorized?.({ path, statusCode: response.statusCode, requestId, payload });
          }
          throw new ApiClientError(payload.message || payload.error || `API code ${code}`, {
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
