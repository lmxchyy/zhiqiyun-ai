import { computed, ref } from "vue";
import type { RoleWalletResponse } from "@xianzhi/business-sdk";
import { businessSdk } from "../api/client";
import {
  loadPersonalPointsWallet,
  readPersonalPointsWalletCache,
  type PersonalWalletContextType,
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

export function usePersonalPointsWallet() {
  const state = ref<PersonalPointsWalletState>(emptyState());
  const loading = ref(false);
  let requestEpoch = 0;

  function hide() {
    requestEpoch += 1;
    loading.value = false;
    state.value = emptyState();
  }

  function hydrate(userId: string, contextType: PersonalWalletContextType) {
    const cached = readPersonalPointsWalletCache(userId, contextType, storage);
    state.value = cached || emptyState();
  }

  async function refresh(userId: string, contextType: PersonalWalletContextType) {
    const normalizedUserId = String(userId || "").trim();
    if (!normalizedUserId || contextType !== "PERSONAL") {
      hide();
      return state.value;
    }

    const epoch = ++requestEpoch;
    hydrate(normalizedUserId, contextType);
    loading.value = true;
    const nextState = await loadPersonalPointsWallet({
      userId: normalizedUserId,
      contextType,
      storage,
      request: () => businessSdk.roleWorkbench.pointsAccount() as Promise<RoleWalletResponse>,
    });
    if (epoch !== requestEpoch) return state.value;
    state.value = nextState;
    loading.value = false;
    return nextState;
  }

  return {
    state,
    payload: computed(() => state.value.payload),
    account: computed(() => state.value.payload?.account || null),
    ready: computed(() => Boolean(state.value.payload?.account)),
    stale: computed(() => state.value.stale),
    error: computed(() => state.value.error),
    loading,
    refresh,
    hide,
  };
}
