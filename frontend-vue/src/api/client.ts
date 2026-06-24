const tokenKey = "token";
const requestTimeout = 180000;
const env = (import.meta as unknown as { env?: Record<string, string | undefined> }).env || {};
const apiBaseURL = String(env.VITE_API_BASE_URL || "").replace(/\/+$/, "");

function resolveURL(path: string) {
  if (/^https?:\/\//i.test(path)) return path;
  return apiBaseURL ? `${apiBaseURL}${path.startsWith("/") ? path : `/${path}`}` : path;
}

export function getAuthToken(): string {
  return (uni.getStorageSync(tokenKey) as string | undefined) || "";
}

export function setAuthToken(token: string) {
  if (token) {
    uni.setStorageSync(tokenKey, token);
  } else {
    uni.removeStorageSync(tokenKey);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getAuthToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string> | undefined)
  };
  if (token) headers.Authorization = `Bearer ${token}`;

  const response = await new Promise<UniApp.RequestSuccessCallbackResult>((resolve, reject) => {
    uni.request({
      url: resolveURL(path),
      method: (init.method || "GET") as UniApp.RequestOptions["method"],
      header: headers,
      data: init.body ? JSON.parse(String(init.body)) : undefined,
      timeout: requestTimeout,
      success: resolve,
      fail: (error) => reject(new Error(error.errMsg || "请求失败"))
    });
  });

  const body = (response.data && typeof response.data === "object" ? response.data : {}) as Record<string, unknown>;
  if (response.statusCode < 200 || response.statusCode >= 300 || (body.code && body.code !== "0")) {
    throw new Error(String(body.message || body.error || response.data || `HTTP ${response.statusCode}`));
  }
  return (Object.prototype.hasOwnProperty.call(body, "data") ? body.data : body) as T;
}
