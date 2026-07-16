import axios, { type AxiosRequestConfig } from "axios";

function getAdminToken() {
  return localStorage.getItem("token") || sessionStorage.getItem("token") || "";
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
  headers: {
    Accept: "application/json"
  }
});

apiClient.interceptors.request.use((config) => {
  const token = getAdminToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const message = error.response?.data?.error || error.response?.data?.message || error.message || "请求失败";
    return Promise.reject(new Error(message));
  }
);

export async function adminRequest<T>(config: AxiosRequestConfig): Promise<T> {
  return (await apiClient.request(config)) as T;
}

export async function adminFetchResponse(
  input: RequestInfo | URL,
  init: RequestInit = {},
  options: { auth?: boolean } = {}
) {
  const headers = new Headers(init.headers);
  const shouldAuthorize = options.auth ?? isSameOriginRequest(input);
  const token = shouldAuthorize ? getAdminToken() : "";
  if (token && !headers.has("Authorization")) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (!response.ok) throw new Error(await adminResponseError(response));
  return response;
}
