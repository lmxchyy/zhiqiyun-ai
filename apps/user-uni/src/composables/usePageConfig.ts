import { computed } from "vue";
import { usePageConfigStore, type AppPageCode } from "../stores/pageConfig";

export function usePageConfig(code: AppPageCode) {
  const store = usePageConfigStore();
  store.hydrate(code);
  return {
    config: computed(() => store.pages[code]),
    loading: computed(() => Boolean(store.loading[code])),
    error: computed(() => store.errors[code] || ""),
    slot: (key: string) => computed(() => store.pages[code]?.slots?.[key]),
    refresh: () => store.refresh(code),
  };
}
