import axios, { type AxiosRequestConfig } from "axios";

export const apiClient = axios.create({
  baseURL: "/api/v1",
  timeout: 180000,
  headers: {
    Accept: "application/json"
  }
});

apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem("token") || sessionStorage.getItem("token");
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
