import { api, businessSdk } from "../api/client";

export type AnyRecord = Record<string, unknown>;

export interface CommercePlanRecord extends AnyRecord {
  id: string;
  name: string;
  planType?: string;
  priceCents?: number;
  points?: number;
  grantPoints?: number;
  tokenAmount?: number;
  benefits?: string[];
  recommended?: boolean;
}

export function asRecord(value: unknown): AnyRecord {
  return value && typeof value === "object" ? value as AnyRecord : {};
}

export function listOf(value: unknown): AnyRecord[] {
  return Array.isArray(value) ? value.filter(item => item && typeof item === "object") as AnyRecord[] : [];
}

export function rowString(row: unknown, ...keys: string[]) {
  const record = asRecord(row);
  for (const key of keys) {
    const value = record[key];
    if (value !== undefined && value !== null && String(value).trim()) return String(value);
  }
  return "";
}

export function rowNumber(row: unknown, ...keys: string[]) {
  const record = asRecord(row);
  for (const key of keys) {
    const value = Number(record[key]);
    if (Number.isFinite(value) && value !== 0) return value;
  }
  return 0;
}

export function rowItems(payload: unknown) {
  if (Array.isArray(payload)) return listOf(payload);
  const record = asRecord(payload);
  return listOf(record.items || record.rows || record.data);
}

export function formatNumber(value: unknown) {
  const numberValue = Number(value || 0);
  return Number.isFinite(numberValue) ? numberValue.toLocaleString("zh-CN") : "0";
}

export function formatCurrency(value: unknown) {
  const cents = Number(value || 0);
  return `¥${(Number.isFinite(cents) ? cents / 100 : 0).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function formatDate(value: unknown) {
  if (!value) return "-";
  const date = new Date(String(value));
  if (Number.isNaN(date.getTime())) return String(value);
  return `${String(date.getMonth() + 1).padStart(2, "0")}/${String(date.getDate()).padStart(2, "0")} ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

export function orderId(row: unknown) {
  return rowString(row, "id", "orderId", "orderNo");
}

export function orderStatus(row: unknown) {
  return rowString(row, "status", "orderStatus").toUpperCase() || "PENDING";
}

export function orderTitle(row: unknown) {
  return rowString(row, "planName", "productName", "name", "subject", "planId") || "知启云订单";
}

export function orderAmount(row: unknown) {
  return rowNumber(row, "amountCents", "amount", "priceCents", "payAmount");
}

export function statusTone(status: string) {
  const normalized = status.toUpperCase();
  if (["PAID", "SUCCESS", "SUCCEEDED", "COMPLETED", "SETTLED", "ACTIVE", "APPROVED"].includes(normalized)) return "success";
  if (["FAILED", "CANCELLED", "REJECTED", "REFUNDED"].includes(normalized)) return "danger";
  return "warning";
}

export function statusText(status: string) {
  const map: Record<string, string> = {
    PENDING: "待处理",
    PAID: "已支付",
    SUCCESS: "已完成",
    SUCCEEDED: "已完成",
    COMPLETED: "已完成",
    SETTLED: "已结算",
    PROCESSING: "处理中",
    ACTIVE: "生效中",
    CANCELLED: "已取消",
    FAILED: "失败",
    REJECTED: "已拒绝",
    REFUNDED: "已退款",
    APPROVED: "已通过"
  };
  return map[status.toUpperCase()] || status || "未知";
}

export function backOrHome(home = "/pages/user/UserHomePage") {
  const pages = getCurrentPages();
  if (pages.length > 1) uni.navigateBack();
  else uni.reLaunch({ url: home });
}

export async function loadUserOrders() {
  const wallet = await businessSdk.roleWorkbench.wallet();
  return listOf(wallet.orders);
}

export async function loadPlans(planType = "") {
  const query = planType ? `?planType=${encodeURIComponent(planType)}` : "";
  const payload = await api<CommercePlanRecord[] | { items?: CommercePlanRecord[] }>(`/api/v1/plans${query}`);
  return (Array.isArray(payload) ? payload : payload.items || []) as CommercePlanRecord[];
}

export async function loadPlan(id: string) {
  const payload = await api<CommercePlanRecord | { item?: CommercePlanRecord }>(`/api/v1/plans/${encodeURIComponent(id)}`);
  return ("item" in payload && payload.item ? payload.item : payload) as CommercePlanRecord;
}
