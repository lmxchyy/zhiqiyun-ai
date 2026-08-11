import { defineStore } from "pinia";
import { personalPointsAdminApi } from "../api/personalPointsAdmin.ts";
import { personalPointsErrorState } from "../domain/personalPointsAdmin.ts";
import type {
  AdminPointMutationRequest,
  PersonalPointLot,
  PersonalPointsErrorState,
  PointExpiryPolicy,
  PointLotFilters,
  UpdatePointExpiryPolicyRequest
} from "../types/personalPointsAdmin.ts";

interface PersonalPointsAdminState {
  policy: PointExpiryPolicy | null;
  lotsByUser: Record<string, PersonalPointLot[]>;
  loading: Record<string, boolean>;
  saving: Record<string, boolean>;
  errors: Record<string, PersonalPointsErrorState>;
}

export const usePersonalPointsAdminStore = defineStore("personalPointsAdmin", {
  state: (): PersonalPointsAdminState => ({
    policy: null,
    lotsByUser: {},
    loading: {},
    saving: {},
    errors: {}
  }),
  actions: {
    clearError(key: string) {
      delete this.errors[key];
    },
    async loadPolicy() {
      this.loading.policy = true;
      delete this.errors.policy;
      try {
        const response = await personalPointsAdminApi.getPolicy();
        this.policy = response.item;
        return response.item;
      } catch (error) {
        this.errors.policy = personalPointsErrorState(error);
        throw error;
      } finally {
        this.loading.policy = false;
      }
    },
    async publishPolicy(input: UpdatePointExpiryPolicyRequest) {
      this.saving.policy = true;
      delete this.errors.policy;
      try {
        const response = await personalPointsAdminApi.updatePolicy(input);
        this.policy = response.item;
        return response.item;
      } catch (error) {
        this.errors.policy = personalPointsErrorState(error);
        throw error;
      } finally {
        this.saving.policy = false;
      }
    },
    async loadLots(userId: string, filters: PointLotFilters = {}) {
      const key = `lots:${userId}`;
      this.loading[key] = true;
      delete this.errors[key];
      try {
        const response = await personalPointsAdminApi.listLots(userId, filters);
        this.lotsByUser[userId] = response.items;
        return response.items;
      } catch (error) {
        this.errors[key] = personalPointsErrorState(error);
        throw error;
      } finally {
        this.loading[key] = false;
      }
    },
    async grantGift(userId: string, input: AdminPointMutationRequest) {
      this.saving.gift = true;
      delete this.errors.gift;
      try {
        const response = await personalPointsAdminApi.grantGift(userId, input);
        await this.loadLots(userId).catch(() => undefined);
        return response;
      } catch (error) {
        this.errors.gift = personalPointsErrorState(error);
        throw error;
      } finally {
        this.saving.gift = false;
      }
    },
    async correctBalance(userId: string, input: AdminPointMutationRequest) {
      this.saving.correction = true;
      delete this.errors.correction;
      try {
        const response = await personalPointsAdminApi.correctBalance(userId, input);
        await this.loadLots(userId).catch(() => undefined);
        return response;
      } catch (error) {
        this.errors.correction = personalPointsErrorState(error);
        throw error;
      } finally {
        this.saving.correction = false;
      }
    }
  }
});
