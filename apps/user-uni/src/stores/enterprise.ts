import { defineStore } from "pinia";
import { enterpriseAPI } from "../features/enterprise/api";
import type {
  EnterpriseAIEmployee,
  EnterpriseAuditLog,
  EnterpriseBillingSummary,
  EnterpriseJoinRequest,
  EnterpriseKnowledgeBase,
  EnterpriseMember,
  EnterpriseOrganization,
  EnterpriseOverview,
  EnterpriseRoleDefinition,
} from "../features/enterprise/types";

interface EnterpriseState {
  tenantId: string;
  overview: EnterpriseOverview | null;
  members: EnterpriseMember[];
  organizations: EnterpriseOrganization[];
  roles: EnterpriseRoleDefinition[];
  billing: EnterpriseBillingSummary | null;
  joinRequests: EnterpriseJoinRequest[];
  auditLogs: EnterpriseAuditLog[];
  aiEmployees: EnterpriseAIEmployee[];
  knowledgeBases: EnterpriseKnowledgeBase[];
}

function emptyTenantState(): EnterpriseState {
  return {
    tenantId: "",
    overview: null,
    members: [],
    organizations: [],
    roles: [],
    billing: null,
    joinRequests: [],
    auditLogs: [],
    aiEmployees: [],
    knowledgeBases: [],
  };
}

export const useEnterpriseStore = defineStore("enterprise", {
  state: (): EnterpriseState => emptyTenantState(),
  actions: {
    useTenant(tenantId: string) {
      const normalized = String(tenantId || "").trim();
      if (this.tenantId && normalized && this.tenantId !== normalized) this.clearTenantData();
      this.tenantId = normalized;
    },
    clearTenantData() {
      this.tenantId = "";
      this.overview = null;
      this.members = [];
      this.organizations = [];
      this.roles = [];
      this.billing = null;
      this.joinRequests = [];
      this.auditLogs = [];
      this.aiEmployees = [];
      this.knowledgeBases = [];
    },
    async loadOverview(force = false) {
      if (this.overview && !force) return this.overview;
      const value = await enterpriseAPI.overview();
      this.useTenant(value.tenant.id);
      this.overview = value;
      return value;
    },
    async loadMembers(force = false) {
      if (this.members.length && !force) return this.members;
      const value = await enterpriseAPI.members();
      this.members = Array.isArray(value.items) ? value.items : [];
      return this.members;
    },
    async loadOrganizations(force = false) {
      if (this.organizations.length && !force) return this.organizations;
      const value = await enterpriseAPI.organizations();
      this.organizations = Array.isArray(value.items) ? value.items : [];
      return this.organizations;
    },
    async loadRoles(force = false) {
      if (this.roles.length && !force) return this.roles;
      const value = await enterpriseAPI.roles();
      this.roles = Array.isArray(value.items) ? value.items : [];
      return this.roles;
    },
    async loadBilling(force = false) {
      if (this.billing && !force) return this.billing;
      this.billing = await enterpriseAPI.billing();
      return this.billing;
    },
    async loadJoinRequests(force = false) {
      if (this.joinRequests.length && !force) return this.joinRequests;
      const value = await enterpriseAPI.joinRequests();
      this.joinRequests = Array.isArray(value.items) ? value.items : [];
      return this.joinRequests;
    },
    async loadAuditLogs(force = false) {
      if (this.auditLogs.length && !force) return this.auditLogs;
      const value = await enterpriseAPI.auditLogs();
      this.auditLogs = Array.isArray(value.items) ? value.items : [];
      return this.auditLogs;
    },
    async loadAIEmployees(force = false) {
      if (this.aiEmployees.length && !force) return this.aiEmployees;
      const value = await enterpriseAPI.aiEmployees();
      this.aiEmployees = Array.isArray(value.items) ? value.items : [];
      return this.aiEmployees;
    },
    async loadKnowledgeBases(force = false) {
      if (this.knowledgeBases.length && !force) return this.knowledgeBases;
      const value = await enterpriseAPI.knowledgeBases();
      this.knowledgeBases = Array.isArray(value.items) ? value.items : [];
      return this.knowledgeBases;
    },
  },
});
