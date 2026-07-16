import { adminRequest } from "./client";

export interface CustomerAttributionParty {
  id: string;
  userId?: string;
  name?: string;
  level?: number;
}

export interface CustomerAttributionItem {
  id: string;
  customerType: "PERSONAL" | "ENTERPRISE";
  customerId: string;
  customerName: string;
  email?: string;
  directAgent: CustomerAttributionParty;
  parentAgent: CustomerAttributionParty;
  operationCenter: CustomerAttributionParty;
  bindType: string;
  bindAt?: string;
  relationStatus: string;
  healthStatus: "COMPLETE" | "PARTIAL" | "UNASSIGNED" | "ANOMALY";
  issues: string[];
  source: string;
  createdAt?: string;
}

export interface CustomerAttributionOption {
  value: string;
  label: string;
}

export interface CustomerAttributionResponse {
  items: CustomerAttributionItem[];
  total: number;
  page: number;
  pageSize: number;
  stats: {
    total: number;
    complete: number;
    partial: number;
    unassigned: number;
    anomaly: number;
  };
  filters: {
    agents: CustomerAttributionOption[];
    operationCenters: CustomerAttributionOption[];
  };
}

export interface CustomerAttributionQuery {
  page: number;
  pageSize: number;
  keyword?: string;
  customerType?: string;
  healthStatus?: string;
  agentId?: string;
  operationCenterId?: string;
}

export function fetchCustomerAttributions(params: CustomerAttributionQuery) {
  return adminRequest<CustomerAttributionResponse>({
    method: "GET",
    url: "/admin/customer-attributions",
    params
  });
}
