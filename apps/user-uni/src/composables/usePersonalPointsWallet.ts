import { computed, ref } from "vue";
import type { RoleWalletResponse } from "@xianzhi/business-sdk";
import { businessSdk } from "../api/client";
import {
  createPersonalPointsWalletCoordinator,
  type PersonalPointsWalletRuntimeScope,
  type PersonalPointsWalletState,
  type PersonalPointsWalletStorage,
} from "../features/wallet/personalPointsWallet";

const emptyState = (): PersonalPointsWalletState => ({
  scope: null,
  payload: null,
  status: "hidden",
  stale: false,
  error: "",
  storedAt: null,
});

const storage: PersonalPointsWalletStorage = {
  get: key => uni.getStorageSync(key),
  set: (key, value) => uni.setStorageSync(key, value),
};

export function usePersonalPointsWallet(getScope: () => PersonalPointsWalletRuntimeScope) {
  const state = ref<PersonalPointsWalletState>(emptyState());
  const loading = ref(false);
  const coordinator = createPersonalPointsWalletCoordinator({
    getScope,
    storage,
    request: () => businessSdk.roleWorkbench.pointsAccount() as Promise<RoleWalletResponse>,
    onChange: snapshot => {
      state.value = snapshot.state;
      loading.value = snapshot.loading;
    },
  });

  return {
    state,
    payload: computed(() => state.value.payload),
    account: computed(() => state.value.payload?.account || null),
    ready: computed(() => Boolean(state.value.payload?.account)),
    stale: computed(() => state.value.stale),
    error: computed(() => state.value.error),
    loading,
    refresh: coordinator.refresh,
    hide: coordinator.invalidate,
  };
}
