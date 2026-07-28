import { defineStore } from "pinia";
import { normalizePricingAuditFilters, pricePlanAdminApi } from "../api/pricePlanAdmin.ts";
import { AdminApiError } from "../api/client.ts";
import {
  isWhitelistRefreshGateKey,
  pricingErrorMessage,
  whitelistRefreshGateKey
} from "../domain/pricePlanAdmin.ts";
import type {
  BusinessPlan,
  PaymentBindingCreateInput,
  PaymentBindingRebindInput,
  PaymentBindingTransitionInput,
  PlanVersion,
  PlanVersionCreateInput,
  PlanVersionUpdateInput,
  PricePlan,
  PricePlanCloneInput,
  PricePlanCreateInput,
  PricePlanPaymentBinding,
  PricePlanValidation,
  PricePlanUpdateInput,
  PricePlanWhitelistEntry,
  PricingAuditFilters,
  PricingAuditPage,
  PricingHealth,
  RevisionReasonInput,
  WechatGoodReference,
  WechatGoodReferencesResponse,
  WechatVirtualGood,
  WechatVirtualGoodConfirmationInput,
  WechatVirtualGoodCreateInput,
  WechatVirtualGoodUpdateInput,
  WhitelistCreateInput,
  WhitelistFilters,
  WhitelistPage,
  WhitelistUpdateInput
} from "../types/pricePlanAdmin.ts";

export interface PricingStoreError {
  message: string;
  code: string;
  status: number;
}

export interface WhitelistRefreshGate {
  pricePlanId: string;
  whitelistEntryId: string;
  userId: string;
  revision: number;
  status: PricePlanWhitelistEntry["status"];
}

export interface PricePlanRefreshGate {
  mutationKey: string;
  planId: string;
  pricePlanId: string;
  revision: number;
}

export interface PlanVersionRefreshGate {
  mutationKey: string;
  planId: string;
  planVersionId: string;
  revision: number;
  includeBusinessPlan: boolean;
}

export interface PricePlanAdminState {
  businessPlans: BusinessPlan[];
  businessPlanById: Record<string, BusinessPlan>;
  planVersionsByPlanId: Record<string, PlanVersion[]>;
  pricePlansByPlanId: Record<string, PricePlan[]>;
  pricePlanById: Record<string, PricePlan>;
  validationByPricePlanId: Record<string, PricePlanValidation>;
  wechatGoods: WechatVirtualGood[];
  wechatGoodById: Record<string, WechatVirtualGood>;
  wechatGoodReferencesById: Record<string, WechatGoodReference[]>;
  wechatGoodReferencePagesById: Record<string, WechatGoodReferencesResponse>;
  bindingsByPricePlanId: Record<string, PricePlanPaymentBinding[]>;
  whitelistByPricePlanId: Record<string, PricePlanWhitelistEntry[]>;
  whitelistPageByPricePlanId: Record<string, WhitelistPage>;
  whitelistFiltersByPricePlanId: Record<string, WhitelistFilters>;
  whitelistRefreshGatesByPricePlanId: Record<string, WhitelistRefreshGate>;
  pricePlanRefreshGatesByMutationKey: Record<string, PricePlanRefreshGate>;
  planVersionRefreshGatesByMutationKey: Record<string, PlanVersionRefreshGate>;
  auditPage: PricingAuditPage | null;
  auditFilters: PricingAuditFilters;
  health: PricingHealth | null;
  selectedPlanId: string;
  selectedPricePlanId: string;
  loading: Record<string, boolean>;
  requestSequences: Record<string, number>;
  saving: Record<string, boolean>;
  errors: Record<string, PricingStoreError>;
  refreshWarnings: Record<string, PricingStoreError>;
}

function errorState(error: unknown): PricingStoreError {
  if (error instanceof AdminApiError) {
    return {
      message: pricingErrorMessage(error, error.message),
      code: error.code,
      status: error.status
    };
  }
  return {
    message: pricingErrorMessage(error),
    code: "",
    status: 0
  };
}

const WHITELIST_EXACT_PAGE_SIZE = 200;
const WHITELIST_EXACT_MAX_PAGES = 100;

export const usePricePlanAdminStore = defineStore("pricePlanAdmin", {
  state: (): PricePlanAdminState => ({
    businessPlans: [],
    businessPlanById: {},
    planVersionsByPlanId: {},
    pricePlansByPlanId: {},
    pricePlanById: {},
    validationByPricePlanId: {},
    wechatGoods: [],
    wechatGoodById: {},
    wechatGoodReferencesById: {},
    wechatGoodReferencePagesById: {},
    bindingsByPricePlanId: {},
    whitelistByPricePlanId: {},
    whitelistPageByPricePlanId: {},
    whitelistFiltersByPricePlanId: {},
    whitelistRefreshGatesByPricePlanId: {},
    pricePlanRefreshGatesByMutationKey: {},
    planVersionRefreshGatesByMutationKey: {},
    auditPage: null,
    auditFilters: { page: 1, pageSize: 50 },
    health: null,
    selectedPlanId: "",
    selectedPricePlanId: "",
    loading: {},
    requestSequences: {},
    saving: {},
    errors: {},
    refreshWarnings: {}
  }),
  getters: {
    selectedBusinessPlan(state): BusinessPlan | undefined {
      return state.businessPlanById[state.selectedPlanId]
        || state.businessPlans.find((plan) => plan.id === state.selectedPlanId);
    },
    selectedPricePlan(state): PricePlan | undefined {
      return state.pricePlanById[state.selectedPricePlanId]
        || Object.values(state.pricePlansByPlanId).flat().find((plan) => plan.pricePlanId === state.selectedPricePlanId);
    }
  },
  actions: {
    setSelection(planId: string, pricePlanId = "") {
      this.selectedPlanId = planId;
      this.selectedPricePlanId = pricePlanId;
    },
    clearError(key: string) {
      delete this.errors[key];
    },
    clearRefreshWarning(key: string) {
      if (isWhitelistRefreshGateKey(key) || this.pricePlanRefreshGatesByMutationKey[key] || this.planVersionRefreshGatesByMutationKey[key]) return;
      delete this.refreshWarnings[key];
    },
    beginLoadRequest(key: string) {
      const sequence = (this.requestSequences[key] || 0) + 1;
      this.requestSequences[key] = sequence;
      this.loading[key] = true;
      delete this.errors[key];
      return sequence;
    },
    isLatestLoadRequest(key: string, sequence: number) {
      return this.requestSequences[key] === sequence;
    },
    async runMutation<T>(key: string, mutation: () => Promise<T>, refresh: (result: T) => Promise<unknown>): Promise<T> {
      this.saving[key] = true;
      delete this.errors[key];
      delete this.refreshWarnings[key];
      try {
        const result = await mutation();
        try {
          await refresh(result);
        } catch (error) {
          this.refreshWarnings[key] = errorState(error);
        }
        return result;
      } catch (error) {
        this.errors[key] = errorState(error);
        throw error;
      } finally {
        this.saving[key] = false;
      }
    },
    async runWhitelistMutation<T extends { item: PricePlanWhitelistEntry }>(
      pricePlanId: string,
      key: string,
      mutation: () => Promise<T>
    ): Promise<T> {
      const gateKey = whitelistRefreshGateKey(pricePlanId);
      if (this.whitelistRefreshGatesByPricePlanId[pricePlanId]) {
        throw new AdminApiError(
          "上次白名单写入后的列表刷新尚未成功，请先恢复最新状态，禁止重复提交。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          { pricePlanId }
        );
      }
      const result = await this.runMutation(key, mutation, () => this.loadWhitelistExact(pricePlanId));
      const refreshWarning = this.refreshWarnings[key];
      if (refreshWarning) {
        const item = result.item;
        this.whitelistRefreshGatesByPricePlanId[pricePlanId] = {
          pricePlanId,
          whitelistEntryId: item.whitelistEntryId,
          userId: item.userId,
          revision: item.revision,
          status: item.status
        };
        this.refreshWarnings[gateKey] = { ...refreshWarning };
      }
      return result;
    },
    async runPricePlanMutation<T extends { item: PricePlan }>(
      key: string,
      mutation: () => Promise<T>
    ): Promise<T> {
      const gate = this.pricePlanRefreshGatesByMutationKey[key];
      if (gate) {
        throw new AdminApiError(
          "上次价格方案写入已经成功，但权威状态尚未恢复；请先完成刷新，禁止重复提交。",
          409,
          "PRICE_PLAN_REFRESH_REQUIRED",
          { mutationKey: key, planId: gate.planId, pricePlanId: gate.pricePlanId }
        );
      }
      const result = await this.runMutation(key, mutation,
        (response) => this.refreshPricePlanDecisionResources(response.item.planId, response.item.pricePlanId));
      if (this.refreshWarnings[key]) {
        this.pricePlanRefreshGatesByMutationKey[key] = {
          mutationKey: key,
          planId: result.item.planId,
          pricePlanId: result.item.pricePlanId,
          revision: result.item.revision
        };
      }
      return result;
    },
    async recoverPricePlanMutation(key: string) {
      const gate = this.pricePlanRefreshGatesByMutationKey[key];
      if (!gate) return;
      await this.refreshPricePlanDecisionResources(gate.planId, gate.pricePlanId);
      const current = this.pricePlanById[gate.pricePlanId];
      if (!current || current.planId !== gate.planId || current.revision < gate.revision) {
        throw new AdminApiError(
          "服务器返回的价格方案状态与已提交记录不一致，请继续保持锁定并联系管理员核对。",
          409,
          "PRICE_PLAN_CONFIGURATION_CHANGED",
          { mutationKey: key, planId: gate.planId, pricePlanId: gate.pricePlanId }
        );
      }
      delete this.pricePlanRefreshGatesByMutationKey[key];
      delete this.refreshWarnings[key];
    },
    async refreshPlanVersionResources(planId: string, includeBusinessPlan: boolean) {
      const requests: Promise<unknown>[] = [this.loadPlanVersions(planId)];
      if (includeBusinessPlan) requests.push(this.loadBusinessPlan(planId));
      await Promise.all(requests);
    },
    async runPlanVersionMutation<T extends { item: PlanVersion }>(
      key: string,
      includeBusinessPlan: boolean,
      mutation: () => Promise<T>
    ): Promise<T> {
      const gate = this.planVersionRefreshGatesByMutationKey[key];
      if (gate) {
        throw new AdminApiError(
          "上次权益版本写入已经成功，但权威状态尚未恢复；请先完成刷新，禁止重复提交。",
          409,
          "PLAN_VERSION_REFRESH_REQUIRED",
          { mutationKey: key, planId: gate.planId, planVersionId: gate.planVersionId }
        );
      }
      const result = await this.runMutation(key, mutation,
        (response) => this.refreshPlanVersionResources(response.item.planId, includeBusinessPlan));
      if (this.refreshWarnings[key]) {
        this.planVersionRefreshGatesByMutationKey[key] = {
          mutationKey: key,
          planId: result.item.planId,
          planVersionId: result.item.id,
          revision: result.item.revision,
          includeBusinessPlan
        };
      }
      return result;
    },
    async recoverPlanVersionMutation(key: string) {
      const gate = this.planVersionRefreshGatesByMutationKey[key];
      if (!gate) return;
      await this.refreshPlanVersionResources(gate.planId, gate.includeBusinessPlan);
      const current = (this.planVersionsByPlanId[gate.planId] || []).find((item) => item.id === gate.planVersionId);
      if (!current || current.revision < gate.revision) {
        throw new AdminApiError(
          "服务器返回的权益版本状态与已提交记录不一致，请继续保持锁定并联系管理员核对。",
          409,
          "PRICE_PLAN_CONFIGURATION_CHANGED",
          { mutationKey: key, planId: gate.planId, planVersionId: gate.planVersionId }
        );
      }
      delete this.planVersionRefreshGatesByMutationKey[key];
      delete this.refreshWarnings[key];
    },
    async loadBusinessPlans() {
      const key = "businessPlans";
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listBusinessPlans();
        if (this.isLatestLoadRequest(key, sequence)) {
          this.businessPlans = page.items;
          for (const item of page.items) this.businessPlanById[item.id] = item;
        }
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadBusinessPlan(planId: string) {
      const key = `businessPlan:${planId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const response = await pricePlanAdminApi.getBusinessPlan(planId);
        if (this.isLatestLoadRequest(key, sequence)) this.businessPlanById[planId] = response.item;
        return response.item;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadPlanVersions(planId: string) {
      const key = `planVersions:${planId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listPlanVersions(planId);
        if (this.isLatestLoadRequest(key, sequence)) this.planVersionsByPlanId[planId] = page.items;
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    createPlanVersion(planId: string, input: PlanVersionCreateInput) {
      return this.runPlanVersionMutation(`createPlanVersion:${planId}`, false,
        () => pricePlanAdminApi.createPlanVersion(planId, input));
    },
    updatePlanVersion(versionId: string, input: PlanVersionUpdateInput) {
      return this.runPlanVersionMutation(`updatePlanVersion:${versionId}`, false,
        () => pricePlanAdminApi.updatePlanVersion(versionId, input));
    },
    activatePlanVersion(versionId: string, input: RevisionReasonInput) {
      return this.runPlanVersionMutation(`activatePlanVersion:${versionId}`, true,
        () => pricePlanAdminApi.activatePlanVersion(versionId, input));
    },
    retirePlanVersion(versionId: string, input: RevisionReasonInput) {
      return this.runPlanVersionMutation(`retirePlanVersion:${versionId}`, true,
        () => pricePlanAdminApi.retirePlanVersion(versionId, input));
    },
    async loadPricePlans(planId: string) {
      const key = `pricePlans:${planId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listPricePlans(planId);
        if (this.isLatestLoadRequest(key, sequence)) {
          this.pricePlansByPlanId[planId] = page.items;
        }
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadPricePlan(pricePlanId: string) {
      const key = `pricePlan:${pricePlanId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const response = await pricePlanAdminApi.getPricePlan(pricePlanId);
        if (this.isLatestLoadRequest(key, sequence)) this.pricePlanById[pricePlanId] = response.item;
        return response.item;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async refreshPricePlanDecisionResources(planId: string, pricePlanId: string) {
      const results = await Promise.allSettled([
        this.loadPricePlans(planId),
        this.loadPricePlan(pricePlanId),
        this.validatePricePlan(pricePlanId),
        this.loadHealth(),
        this.loadPaymentBindings(pricePlanId),
        this.loadWechatGoods()
      ]);
      const failure = results.find((result): result is PromiseRejectedResult => result.status === "rejected");
      if (failure) throw failure.reason;
    },
    createPricePlan(planId: string, input: PricePlanCreateInput) {
      return this.runPricePlanMutation(`createPricePlan:${planId}`,
        () => pricePlanAdminApi.createPricePlan(planId, input));
    },
    updatePricePlan(pricePlanId: string, input: PricePlanUpdateInput) {
      return this.runPricePlanMutation(`updatePricePlan:${pricePlanId}`,
        () => pricePlanAdminApi.updatePricePlan(pricePlanId, input));
    },
    clonePricePlan(pricePlanId: string, input: PricePlanCloneInput) {
      return this.runPricePlanMutation(`clonePricePlan:${pricePlanId}`,
        () => pricePlanAdminApi.clonePricePlan(pricePlanId, input));
    },
    async validatePricePlan(pricePlanId: string) {
      const key = `validation:${pricePlanId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const validation = await pricePlanAdminApi.validatePricePlan(pricePlanId);
        if (this.isLatestLoadRequest(key, sequence)) this.validationByPricePlanId[pricePlanId] = validation;
        return validation;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    enablePricePlan(pricePlanId: string, input: RevisionReasonInput) {
      return this.runMutation(`enablePricePlan:${pricePlanId}`,
        () => pricePlanAdminApi.enablePricePlan(pricePlanId, input),
        (response) => this.refreshPricePlanDecisionResources(response.item.planId, response.item.pricePlanId));
    },
    disablePricePlan(pricePlanId: string, input: RevisionReasonInput) {
      return this.runMutation(`disablePricePlan:${pricePlanId}`,
        () => pricePlanAdminApi.disablePricePlan(pricePlanId, input),
        (response) => this.refreshPricePlanDecisionResources(response.item.planId, response.item.pricePlanId));
    },
    makeDefaultPricePlan(pricePlanId: string, input: RevisionReasonInput) {
      return this.runMutation(`makeDefaultPricePlan:${pricePlanId}`,
        () => pricePlanAdminApi.makeDefaultPricePlan(pricePlanId, input),
        (response) => this.refreshPricePlanDecisionResources(response.item.planId, response.item.pricePlanId));
    },
    async loadWechatGoods() {
      const key = "wechatGoods";
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listWechatVirtualGoods();
        if (this.isLatestLoadRequest(key, sequence)) {
          this.wechatGoods = page.items;
        }
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadWechatGood(goodId: string) {
      const key = `wechatGood:${goodId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const response = await pricePlanAdminApi.getWechatVirtualGood(goodId);
        if (this.isLatestLoadRequest(key, sequence)) this.wechatGoodById[goodId] = response.item;
        return response.item;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadWechatGoodReferences(goodId: string) {
      const key = `wechatGoodReferences:${goodId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listWechatVirtualGoodReferences(goodId);
        if (this.isLatestLoadRequest(key, sequence)) {
          this.wechatGoodReferencesById[goodId] = page.items;
          this.wechatGoodReferencePagesById[goodId] = page;
        }
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async refreshWechatGoodDecisionResources(goodId: string) {
      const base = await Promise.allSettled([
        this.loadWechatGoods(),
        this.loadWechatGood(goodId),
        this.loadWechatGoodReferences(goodId),
        this.loadHealth()
      ]);
      const baseFailure = base.find((result): result is PromiseRejectedResult => result.status === "rejected");
      const references = this.wechatGoodReferencesById[goodId] || [];
      const byPricePlan = new Map<string, WechatGoodReference>();
      for (const reference of references) byPricePlan.set(reference.pricePlanId, reference);
      const dependent = await Promise.allSettled([...byPricePlan.values()].flatMap((reference) => [
        this.loadPricePlan(reference.pricePlanId),
        this.loadPricePlans(reference.planId),
        this.loadPaymentBindings(reference.pricePlanId),
        this.validatePricePlan(reference.pricePlanId)
      ]));
      const dependentFailure = dependent.find((result): result is PromiseRejectedResult => result.status === "rejected");
      if (baseFailure) throw baseFailure.reason;
      if (dependentFailure) throw dependentFailure.reason;
    },
    createWechatGood(input: WechatVirtualGoodCreateInput) {
      return this.runMutation("createWechatGood",
        () => pricePlanAdminApi.createWechatVirtualGood(input),
        (response) => this.refreshWechatGoodDecisionResources(response.item.id));
    },
    updateWechatGood(goodId: string, input: WechatVirtualGoodUpdateInput) {
      return this.runMutation(`updateWechatGood:${goodId}`,
        () => pricePlanAdminApi.updateWechatVirtualGood(goodId, input),
        (response) => this.refreshWechatGoodDecisionResources(response.item.id));
    },
    confirmWechatGood(goodId: string, input: WechatVirtualGoodConfirmationInput) {
      return this.runMutation(`confirmWechatGood:${goodId}`,
        () => pricePlanAdminApi.confirmWechatVirtualGood(goodId, input),
        (response) => this.refreshWechatGoodDecisionResources(response.item.id));
    },
    disableWechatGood(goodId: string, input: RevisionReasonInput) {
      return this.runMutation(`disableWechatGood:${goodId}`,
        () => pricePlanAdminApi.disableWechatVirtualGood(goodId, input),
        (response) => this.refreshWechatGoodDecisionResources(response.item.id));
    },
    async loadPaymentBindings(pricePlanId: string) {
      const key = `bindings:${pricePlanId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listPaymentBindings(pricePlanId);
        if (this.isLatestLoadRequest(key, sequence)) this.bindingsByPricePlanId[pricePlanId] = page.items;
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadExactPaymentGood(pricePlanId: string) {
      const validation = this.validationByPricePlanId[pricePlanId];
      const bindingId = String(validation?.paymentBindingId || "").trim();
      const goodId = String(validation?.wechatGoodId || "").trim();
      if (!bindingId && !goodId) return undefined;
      const binding = (this.bindingsByPricePlanId[pricePlanId] || []).find((item) => item.id === bindingId);
      if (!bindingId || !goodId || !binding || binding.pricePlanId !== pricePlanId || binding.wechatGoodId !== goodId) {
        throw new Error("PAYMENT_BINDING_CONFIGURATION_CHANGED");
      }
      return this.loadWechatGood(goodId);
    },
    createPaymentBinding(pricePlanId: string, input: PaymentBindingCreateInput) {
      return this.runMutation(`createBinding:${pricePlanId}`,
        () => pricePlanAdminApi.createPaymentBinding(pricePlanId, input),
        (response) => this.refreshPaymentBindingDecisionResources(response.item.pricePlanId, [response.item.wechatGoodId]));
    },
    async refreshPaymentBindingDecisionResources(pricePlanId: string, goodIds: string[]) {
      const cachedPlanId = this.pricePlanById[pricePlanId]?.planId
        || Object.entries(this.pricePlansByPlanId).find(([, plans]) => plans.some((plan) => plan.pricePlanId === pricePlanId))?.[0]
        || "";
      const normalizedGoodIds = [...new Set(goodIds.map((id) => String(id || "").trim()).filter(Boolean))];
      const results = await Promise.allSettled([
        this.loadPricePlan(pricePlanId),
        ...(cachedPlanId ? [this.loadPricePlans(cachedPlanId)] : []),
        this.loadPaymentBindings(pricePlanId),
        this.validatePricePlan(pricePlanId),
        this.loadWechatGoods(),
        this.loadHealth(),
        ...normalizedGoodIds.flatMap((goodId) => [
          this.loadWechatGood(goodId),
          this.loadWechatGoodReferences(goodId)
        ])
      ]);
      let failure = results.find((result): result is PromiseRejectedResult => result.status === "rejected");
      const refreshedPlanId = this.pricePlanById[pricePlanId]?.planId || cachedPlanId;
      if (refreshedPlanId && !cachedPlanId) {
        const listResult = await Promise.allSettled([this.loadPricePlans(refreshedPlanId)]);
        failure ||= listResult.find((result): result is PromiseRejectedResult => result.status === "rejected");
      }
      if (failure) throw failure.reason;
    },
    rebindPaymentBinding(bindingId: string, input: PaymentBindingRebindInput) {
      const previous = Object.values(this.bindingsByPricePlanId).flat().find((binding) => binding.id === bindingId);
      const oldGoodId = previous?.wechatGoodId || "";
      return this.runMutation(`rebindBinding:${bindingId}`,
        () => pricePlanAdminApi.rebindPaymentBinding(bindingId, input),
        (response) => this.refreshPaymentBindingDecisionResources(response.item.pricePlanId, [oldGoodId, response.item.wechatGoodId]));
    },
    transitionPaymentBinding(bindingId: string, input: PaymentBindingTransitionInput) {
      const previous = Object.values(this.bindingsByPricePlanId).flat().find((binding) => binding.id === bindingId);
      return this.runMutation(`transitionBinding:${bindingId}`,
        () => pricePlanAdminApi.transitionPaymentBinding(bindingId, input),
        (response) => this.refreshPaymentBindingDecisionResources(response.item.pricePlanId, [previous?.wechatGoodId || response.item.wechatGoodId]));
    },
    async loadWhitelist(pricePlanId: string, requestedFilters?: WhitelistFilters) {
      const filters: WhitelistFilters = {
        page: 1,
        pageSize: 50,
        ...(requestedFilters || this.whitelistFiltersByPricePlanId[pricePlanId] || {})
      };
      const key = `whitelist:${pricePlanId}`;
      const sequence = this.beginLoadRequest(key);
      try {
        const page = await pricePlanAdminApi.listWhitelist(pricePlanId, filters);
        const normalized: WhitelistPage = {
          ...page,
          page: Number(page.page || filters.page || 1),
          pageSize: Number(page.pageSize || filters.pageSize || Math.max(page.items.length, 1))
        };
        if (this.isLatestLoadRequest(key, sequence)) {
          this.whitelistFiltersByPricePlanId[pricePlanId] = { ...filters };
          this.whitelistByPricePlanId[pricePlanId] = normalized.items;
          this.whitelistPageByPricePlanId[pricePlanId] = normalized;
        }
        return normalized;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadWhitelistExact(pricePlanId: string, requestedFilters?: WhitelistFilters) {
      const key = `whitelist:${pricePlanId}`;
      const pending = this.loadWhitelist(pricePlanId, requestedFilters);
      const sequence = this.requestSequences[key];
      const page = await pending;
      if (!this.isLatestLoadRequest(key, sequence)) {
        throw new AdminApiError(
          "白名单刷新被更新的请求替代，尚不能确认最终服务器状态。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          { pricePlanId }
        );
      }
      return page;
    },
    async loadWhitelistEntryExact(pricePlanId: string, entryId: string, userId: string) {
      const key = `whitelistEntryExact:${pricePlanId}:${entryId}`;
      const sequence = this.beginLoadRequest(key);
      const pageSize = WHITELIST_EXACT_PAGE_SIZE;
      let requestedPage = 1;
      try {
        while (requestedPage <= WHITELIST_EXACT_MAX_PAGES) {
          const page = await pricePlanAdminApi.listWhitelist(pricePlanId, {
            userId,
            page: requestedPage,
            pageSize
          });
          if (!this.isLatestLoadRequest(key, sequence)) {
            throw new AdminApiError(
              "白名单精确查询已被更新的请求替代，当前结果不可用于 revision 或写后状态确认。",
              409,
              "WHITELIST_REFRESH_REQUIRED",
              { pricePlanId, whitelistEntryId: entryId, userId }
            );
          }
          const exact = page.items.find((item) => item.pricePlanId === pricePlanId
            && item.whitelistEntryId === entryId && item.userId === userId);
          if (exact) return exact;

          const total = Number(page.total);
          const responsePageSize = Number(page.pageSize);
          const effectivePageSize = Number.isInteger(responsePageSize) && responsePageSize > 0
            ? Math.min(responsePageSize, pageSize)
            : pageSize;
          const exhausted = page.items.length === 0
            || (Number.isFinite(total) && total >= 0 && requestedPage * effectivePageSize >= total)
            || (!Number.isFinite(total) && page.items.length < effectivePageSize);
          if (exhausted) {
            throw new AdminApiError(
              "未能在服务器白名单中精确确认目标记录，当前操作仍保持阻断。",
              409,
              "WHITELIST_REFRESH_REQUIRED",
              { pricePlanId, whitelistEntryId: entryId, userId }
            );
          }
          requestedPage += 1;
        }
        throw new AdminApiError(
          "白名单精确查询已达到客户端安全扫描上限，当前结果不足以确认写入状态。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          {
            pricePlanId,
            whitelistEntryId: entryId,
            userId,
            maxPages: WHITELIST_EXACT_MAX_PAGES,
            maxRecords: WHITELIST_EXACT_PAGE_SIZE * WHITELIST_EXACT_MAX_PAGES
          }
        );
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async recoverWhitelistRefreshGate(pricePlanId: string, requestedFilters?: WhitelistFilters) {
      const gate = this.whitelistRefreshGatesByPricePlanId[pricePlanId];
      if (!gate) {
        throw new AdminApiError(
          "没有可核验的白名单写入身份，不能解除刷新门禁。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          { pricePlanId }
        );
      }
      const pinned = await this.loadWhitelistEntryExact(
        gate.pricePlanId,
        gate.whitelistEntryId,
        gate.userId
      );
      const revisionConfirmed = Number.isInteger(pinned.revision) && pinned.revision >= gate.revision;
      const statusConfirmed = pinned.revision > gate.revision || pinned.status === gate.status;
      if (pinned.pricePlanId !== gate.pricePlanId
        || pinned.whitelistEntryId !== gate.whitelistEntryId
        || pinned.userId !== gate.userId
        || !revisionConfirmed
        || !statusConfirmed) {
        throw new AdminApiError(
          "服务器记录尚未达到上次成功写入的 revision 或状态，当前写入门禁继续保留。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          {
            pricePlanId: gate.pricePlanId,
            whitelistEntryId: gate.whitelistEntryId,
            userId: gate.userId,
            requiredRevision: gate.revision,
            observedRevision: pinned.revision
          }
        );
      }
      const page = await this.loadWhitelistExact(pricePlanId, requestedFilters);
      const currentGate = this.whitelistRefreshGatesByPricePlanId[pricePlanId];
      if (!currentGate
        || currentGate.whitelistEntryId !== gate.whitelistEntryId
        || currentGate.userId !== gate.userId
        || currentGate.revision !== gate.revision
        || currentGate.status !== gate.status) {
        throw new AdminApiError(
          "白名单刷新门禁已发生变化，当前恢复结果不可用于解锁。",
          409,
          "WHITELIST_REFRESH_REQUIRED",
          { pricePlanId }
        );
      }
      delete this.whitelistRefreshGatesByPricePlanId[pricePlanId];
      delete this.refreshWarnings[whitelistRefreshGateKey(pricePlanId)];
      return page;
    },
    createWhitelistEntry(pricePlanId: string, input: WhitelistCreateInput) {
      return this.runWhitelistMutation(pricePlanId, `createWhitelist:${pricePlanId}`,
        () => pricePlanAdminApi.createWhitelistEntry(pricePlanId, input));
    },
    updateWhitelistEntry(pricePlanId: string, entryId: string, input: WhitelistUpdateInput) {
      return this.runWhitelistMutation(pricePlanId, `updateWhitelist:${entryId}`,
        () => pricePlanAdminApi.updateWhitelistEntry(pricePlanId, entryId, input));
    },
    disableWhitelistEntry(pricePlanId: string, entryId: string, input: RevisionReasonInput) {
      return this.runWhitelistMutation(pricePlanId, `disableWhitelist:${entryId}`,
        () => pricePlanAdminApi.disableWhitelistEntry(pricePlanId, entryId, input));
    },
    async loadAuditLogs(requestedFilters?: PricingAuditFilters) {
      const key = "audit";
      const sequence = this.beginLoadRequest(key);
      try {
        const filters = normalizePricingAuditFilters(requestedFilters || this.auditFilters);
        const page = await pricePlanAdminApi.listPricingAuditLogs(filters);
        if (this.isLatestLoadRequest(key, sequence)) {
          this.auditFilters = { ...filters };
          this.auditPage = page;
        }
        return page;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    },
    async loadHealth() {
      const key = "health";
      const sequence = this.beginLoadRequest(key);
      try {
        const health = await pricePlanAdminApi.getPricingHealth();
        if (this.isLatestLoadRequest(key, sequence)) this.health = health;
        return health;
      } catch (error) {
        if (this.isLatestLoadRequest(key, sequence)) this.errors[key] = errorState(error);
        throw error;
      } finally {
        if (this.isLatestLoadRequest(key, sequence)) this.loading[key] = false;
      }
    }
  }
});
