import { onBeforeUnmount, ref, watch, type Ref } from "vue";
import { adminWorkspaceApi, type GlobalSearchItem } from "../api/adminWorkspaces";

export function useAdminGlobalSearch(query: Ref<string>) {
  const results = ref<GlobalSearchItem[]>([]);
  const loading = ref(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let requestVersion = 0;

  watch(query, (value) => {
    if (timer) clearTimeout(timer);
    const keyword = value.trim();
    if (keyword.length < 2) {
      requestVersion += 1;
      results.value = [];
      loading.value = false;
      return;
    }
    timer = setTimeout(async () => {
      const version = ++requestVersion;
      loading.value = true;
      try {
        const response = await adminWorkspaceApi.globalSearch(keyword);
        if (version === requestVersion) results.value = response.items || [];
      } catch {
        if (version === requestVersion) results.value = [];
      } finally {
        if (version === requestVersion) loading.value = false;
      }
    }, 220);
  });

  onBeforeUnmount(() => {
    if (timer) clearTimeout(timer);
    requestVersion += 1;
  });

  return { results, loading };
}
