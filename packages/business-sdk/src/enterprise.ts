import type { ApiClient } from "@xianzhi/api-client";
import type { CurrentContextRequest, EnterpriseCertification, EnterpriseCertificationSubmitRequest, EnterpriseContext, EnterpriseContextsResponse } from "@xianzhi/shared-types";
import type { BusinessSdk } from "./types";

export function createEnterpriseSdk(api: ApiClient): BusinessSdk["enterprise"] {
  return {
    contexts: () => api.request<EnterpriseContextsResponse>("/api/v1/user/enterprise-contexts"),
    switchContext: (input: CurrentContextRequest) => api.request<EnterpriseContext>("/api/v1/user/current-context", { method: "POST", body: input }),
    create: (name: string) => api.request("/api/v1/enterprises", { method: "POST", body: { name } }),
    overview: () => api.request("/api/v1/enterprise/overview"),
    members: () => api.request("/api/v1/enterprise/members"),
    organizationTree: () => api.request("/api/v1/enterprise/organizations/tree"),
    submitCertification: (input: EnterpriseCertificationSubmitRequest) => api.request<EnterpriseCertification>("/api/v1/enterprise/certifications", { method: "POST", body: input })
  };
}
