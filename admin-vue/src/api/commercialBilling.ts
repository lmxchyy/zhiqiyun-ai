import { adminRequest } from "./client";

export type CommercialBillingRow = Record<string, any>;

const endpoints: Record<string, string> = {
  billingCustomers: "/admin/billing/customers",
  billingProducts: "/admin/billing/products",
  billingSubscriptions: "/admin/billing/subscriptions",
  billingCoupons: "/admin/billing/coupons",
  billingInvoices: "/admin/billing/invoices",
  billingCreditNotes: "/admin/billing/credit-notes",
  billingPaymentRequests: "/admin/billing/payment-requests",
  billingPayments: "/admin/billing/payments"
};

export const commercialBillingApi = {
  async list(moduleId: string) {
    const url = endpoints[moduleId];
    if (!url) throw new Error(`未知商业计费页面：${moduleId}`);
    return adminRequest<{ items: CommercialBillingRow[]; total: number; source: string }>({ method: "GET", url });
  },
  createCoupon(data: CommercialBillingRow) {
    return adminRequest({ method: "POST", url: "/admin/billing/coupons", data });
  },
  updateCoupon(id: string, data: CommercialBillingRow) {
    return adminRequest({ method: "PATCH", url: `/admin/billing/coupons/${encodeURIComponent(id)}`, data });
  },
  updateSubscription(id: string, data: CommercialBillingRow) {
    return adminRequest({ method: "PATCH", url: `/admin/billing/subscriptions/${encodeURIComponent(id)}`, data });
  },
  updateInvoice(id: string, data: CommercialBillingRow) {
    return adminRequest({ method: "PATCH", url: `/admin/billing/invoices/${encodeURIComponent(id)}`, data });
  },
  createCreditNote(data: CommercialBillingRow) {
    return adminRequest({ method: "POST", url: "/admin/billing/credit-notes", data });
  },
  reviewCreditNote(id: string, data: CommercialBillingRow) {
    return adminRequest({ method: "PATCH", url: `/admin/billing/credit-notes/${encodeURIComponent(id)}`, data });
  },
  recordDunning(id: string, data: CommercialBillingRow) {
    return adminRequest({ method: "POST", url: `/admin/billing/payment-requests/${encodeURIComponent(id)}/dunning`, data });
  }
};
