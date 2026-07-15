import { adminRequest } from "./client";
import type {
  AdminEnterpriseCreateRequest,
  AdminEnterpriseDetail,
  AdminEnterpriseListQuery,
  AdminEnterpriseListResult,
  AdminEnterpriseMutationRequest,
  AdminEnterpriseMutationResult,
  AdminEnterpriseSectionResult
} from "../types/adminEnterprise";

export function listAdminEnterprises(params: AdminEnterpriseListQuery) {
  return adminRequest<AdminEnterpriseListResult>({
    method: "GET",
    url: "/admin/enterprises",
    params
  });
}

export function getAdminEnterprise(enterpriseId: string) {
  return adminRequest<AdminEnterpriseDetail>({
    method: "GET",
    url: `/admin/enterprises/${encodeURIComponent(enterpriseId)}`
  });
}

export function createAdminEnterprise(data: AdminEnterpriseCreateRequest) {
  return adminRequest<AdminEnterpriseDetail>({
    method: "POST",
    url: "/admin/enterprises",
    data
  });
}

export function updateAdminEnterprise(enterpriseId: string, data: AdminEnterpriseMutationRequest) {
  return adminRequest<AdminEnterpriseMutationResult>({
    method: "PATCH",
    url: `/admin/enterprises/${encodeURIComponent(enterpriseId)}`,
    data
  });
}

export function exportAdminEnterprises(params: AdminEnterpriseListQuery) {
  return adminRequest<Blob>({
    method: "GET",
    url: "/admin/enterprises/export",
    params,
    responseType: "blob"
  });
}

export function listAdminEnterpriseCertifications() {
  return adminRequest<AdminEnterpriseSectionResult>({ method: "GET", url: "/admin/enterprises/certifications" });
}

export function getAdminEnterpriseSection(enterpriseId: string, section: string) {
  return adminRequest<AdminEnterpriseSectionResult>({
    method: "GET",
    url: `/admin/enterprises/${encodeURIComponent(enterpriseId)}/${section}`
  });
}

export function mutateAdminEnterprise(enterpriseId: string, actionPath: string, data: AdminEnterpriseMutationRequest) {
  return adminRequest<AdminEnterpriseMutationResult>({
    method: "POST",
    url: `/admin/enterprises/${encodeURIComponent(enterpriseId)}/${actionPath}`,
    data
  });
}
