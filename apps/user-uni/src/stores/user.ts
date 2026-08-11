import { defineStore } from "pinia";
import { api, authStorage } from "../api/client";
import { isAppRole, permissionsForRole } from "../config/permissions";
import { useEnterpriseStore } from "./enterprise";
import { clearPromotionCache } from "../features/promotion/cache";
import type { AppRole, AuthResponse, CurrentContextRequest, EnterpriseContext, EnterpriseContextsResponse, UserAccessProfile } from "../types";

interface UserState extends UserAccessProfile {
  loaded: boolean;
  loading: boolean;
  enterpriseContexts: EnterpriseContext[];
  currentContext: EnterpriseContext | null;
}

function normalizeRoles(values: unknown): AppRole[] {
  const roles: AppRole[] = ["USER"];
  if (Array.isArray(values)) {
    values.forEach(value => {
      if (isAppRole(value) && !roles.includes(value)) roles.push(value);
    });
  }
  return roles;
}

function profileFromAuth(auth: AuthResponse | null): UserAccessProfile {
  const roles = normalizeRoles(auth?.roles);
  const currentRole = isAppRole(auth?.currentRole) && roles.includes(auth.currentRole) ? auth.currentRole : "USER";
  return {
    userId: auth?.user?.id || "",
    tenantId: auth?.tenantId || auth?.user?.tenantId || "tenant_default",
    organizationId: auth?.organizationId || auth?.user?.organizationId || "organization_default",
    roles,
    currentRole,
    permissions: Array.isArray(auth?.permissions) && auth.permissions.length ? [...auth.permissions] : permissionsForRole(currentRole),
  };
}

const cachedProfile = profileFromAuth(authStorage.getAuth());

export const useUserStore = defineStore("user", {
  state: (): UserState => ({
    ...cachedProfile,
    loaded: Boolean(cachedProfile.userId && authStorage.getAuth()?.roles?.length),
    loading: false,
    enterpriseContexts: [],
    currentContext: null,
  }),
  getters: {
    hasRole: state => (role: AppRole) => state.roles.includes(role),
    hasPermission: state => (permission: string) => state.permissions.includes(permission),
    canSwitchRole: state => state.roles.length > 1,
  },
  actions: {
    applyProfile(profile: UserAccessProfile) {
      const roles = normalizeRoles(profile.roles);
      const currentRole = isAppRole(profile.currentRole) && roles.includes(profile.currentRole) ? profile.currentRole : "USER";
      this.userId = String(profile.userId || "");
      this.tenantId = String(profile.tenantId || "tenant_default");
      this.organizationId = String(profile.organizationId || "organization_default");
      this.roles = roles;
      this.currentRole = currentRole;
      this.permissions = Array.isArray(profile.permissions) && profile.permissions.length
        ? [...new Set(profile.permissions)]
        : permissionsForRole(currentRole);
      this.loaded = true;
      const auth = authStorage.getAuth();
      if (auth) {
        authStorage.setAuth({
          ...auth,
          tenantId: this.tenantId,
          organizationId: this.organizationId,
          roles: this.roles,
          currentRole: this.currentRole,
          permissions: this.permissions,
        });
      }
    },
    hydrateFromAuth(auth: AuthResponse) {
      this.applyProfile(profileFromAuth(auth));
    },
    async loadProfile(force = false) {
      if (this.loading) return null;
      if (this.loaded && !force) return {
        userId: this.userId,
        tenantId: this.tenantId,
        organizationId: this.organizationId,
        roles: this.roles,
        currentRole: this.currentRole,
        permissions: this.permissions,
      } satisfies UserAccessProfile;
      this.loading = true;
      try {
        const profile = await api<UserAccessProfile>("/api/v1/user/profile");
        this.applyProfile(profile);
        return profile;
      } finally {
        this.loading = false;
      }
    },
    async switchRole(role: AppRole) {
      if (!this.roles.includes(role)) throw new Error("当前账号未开通该角色");
      if (role === this.currentRole) return {
        userId: this.userId,
        tenantId: this.tenantId,
        organizationId: this.organizationId,
        roles: this.roles,
        currentRole: this.currentRole,
        permissions: this.permissions,
      } satisfies UserAccessProfile;
      const profile = await api<UserAccessProfile>("/api/v1/user/current-role", {
        method: "POST",
        body: JSON.stringify({ role }),
      });
      this.applyProfile(profile);
      return profile;
    },
    async fetchEnterpriseContexts() {
      return api<EnterpriseContextsResponse>("/api/v1/user/enterprise-contexts");
    },
    applyEnterpriseContexts(payload: EnterpriseContextsResponse) {
      this.enterpriseContexts = Array.isArray(payload.contexts) ? payload.contexts : [];
      this.currentContext = payload.current || this.enterpriseContexts.find(item => item.current) || null;
      return payload;
    },
    async loadEnterpriseContexts() {
      const payload = await this.fetchEnterpriseContexts();
      return this.applyEnterpriseContexts(payload);
    },
    async switchContext(input: CurrentContextRequest) {
      const previousContext = this.currentContext;
      const context = await api<EnterpriseContext>("/api/v1/user/current-context", {
        method: "POST",
        body: JSON.stringify(input),
      });
      const tenantChanged = previousContext?.tenantId !== context.tenantId || previousContext?.type !== context.type;
      const enterpriseStore = useEnterpriseStore();
      if (tenantChanged) enterpriseStore.clearTenantData();
      this.currentContext = context;
      this.enterpriseContexts = this.enterpriseContexts.map(item => ({
        ...item,
        current: item.type === context.type && item.tenantId === context.tenantId,
      }));
      this.applyProfile({
        userId: this.userId,
        tenantId: context.tenantId,
        organizationId: context.organizationId,
        roles: context.roles,
        currentRole: context.currentRole,
        permissions: context.permissions,
      });
      if (context.type === "ENTERPRISE") enterpriseStore.useTenant(context.tenantId);
      return context;
    },
    reset() {
      clearPromotionCache();
      const profile = profileFromAuth(null);
      this.$patch({ ...profile, loaded: false, loading: false, enterpriseContexts: [], currentContext: null });
      useEnterpriseStore().clearTenantData();
    },
  },
});
