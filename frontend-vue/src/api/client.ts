const tokenKey = "token";

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = uni.getStorageSync(tokenKey) as string | undefined;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string> | undefined)
  };
  if (token) headers.Authorization = `Bearer ${token}`;

  const response = await new Promise<UniApp.RequestSuccessCallbackResult>((resolve, reject) => {
    uni.request({
      url: path,
      method: (init.method || "GET") as UniApp.RequestOptions["method"],
      header: headers,
      data: init.body ? JSON.parse(String(init.body)) : undefined,
      success: resolve,
      fail: reject
    });
  });

  const body = response.data as Record<string, unknown>;
  if (response.statusCode < 200 || response.statusCode >= 300 || (body.code && body.code !== "0")) {
    throw new Error(String(body.message || body.error || `HTTP ${response.statusCode}`));
  }
  return (Object.prototype.hasOwnProperty.call(body, "data") ? body.data : body) as T;
}
