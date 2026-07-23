import { defineStore } from "pinia";
import {
  archiveAsset as archiveAssetRequest,
  batchAssets,
  cancelGenerationTask,
  deleteAsset as deleteAssetRequest,
  fetchAssetDetail,
  fetchAssetOverview,
  fetchAssetPage,
  fetchProjects,
  fetchRecentWorksTask,
  fetchTaskPage,
  moveAssetToProject,
  permanentlyDeleteAsset,
  renameAsset,
  restoreAsset as restoreAssetRequest,
  retryGenerationTask,
  setAssetFavorite,
} from "../features/assets/api";
import {
  defaultAssetFilter,
  type AssetBatchPayload,
  type AssetDetail,
  type AssetFilter,
  type AssetItem,
  type AssetOverview,
  type AssetSort,
  type AssetStatus,
  type AssetType,
  type GenerationTask,
  type ProjectOption,
} from "../features/assets/types";
import { beginWorksPerformanceStep, recordWorksPerformance } from "../features/assets/performance";
import {
  isCurrentRecentRequest,
  mergeRecentWorks,
  shouldDedupeRecentRequest,
  stableAsset,
} from "../features/assets/recent";

const filterStorageKey = "zhiqiyun:asset-center:filters";
const sortStorageKey = "zhiqiyun:asset-center:sort";
const recentCacheStorageKey = "recent_works_cache";
const recentRequestDedupeMs = 3000;

interface RecentWorksCache {
  scope: string;
  storedAt: number;
  assets: AssetItem[];
}

function emptyOverview(): AssetOverview {
  return { total: 0, monthTotal: 0, favoriteTotal: 0, storageBytes: 0, storageQuotaBytes: 0, storageUsagePercent: 0 };
}

function readFilters(): AssetFilter {
  try {
    const value = uni.getStorageSync(filterStorageKey) as Partial<AssetFilter> | "";
    return value && typeof value === "object" ? { ...defaultAssetFilter(), ...value, tagIds: Array.isArray(value.tagIds) ? value.tagIds : [] } : defaultAssetFilter();
  } catch {
    return defaultAssetFilter();
  }
}

function readSort(): AssetSort {
  try {
    const value = String(uni.getStorageSync(sortStorageKey) || "created_desc") as AssetSort;
    return ["created_desc", "created_asc", "updated_desc", "name_asc", "size_desc", "usage_desc"].includes(value) ? value : "created_desc";
  } catch {
    return "created_desc";
  }
}

function persist(filters: AssetFilter, sort: AssetSort) {
  try {
    uni.setStorageSync(filterStorageKey, filters);
    uni.setStorageSync(sortStorageKey, sort);
  } catch {
    /* persistence is optional */
  }
}

function recentCacheKey(scope: string) {
  return `${recentCacheStorageKey}:${encodeURIComponent(scope)}`;
}

function readRecentCache(scope: string): RecentWorksCache | null {
  const timing = beginWorksPerformanceStep("local_cache_read", {
    serialWait: true,
    source: "assetStore.hydrateRecentWorksCache",
  });
  try {
    const value = uni.getStorageSync(recentCacheKey(scope)) as Partial<RecentWorksCache> | "";
    if (!value || typeof value !== "object" || value.scope !== scope || !Number.isFinite(value.storedAt) || !Array.isArray(value.assets)) {
      timing.end({ cacheHit: false, itemCount: 0 });
      return null;
    }
    const cache = value as RecentWorksCache;
    timing.end({ cacheHit: true, itemCount: cache.assets.length });
    return cache;
  } catch {
    timing.end({ cacheHit: false, note: "cache_read_failed" });
    return null;
  }
}

function cacheAsset(item: AssetItem): AssetItem {
  return {
    ...item,
    remoteUrl: "",
    metadata: {},
  };
}

function writeRecentCache(scope: string, assets: AssetItem[]) {
  if (!scope) return;
  const timing = beginWorksPerformanceStep("local_cache_write", {
    serialWait: true,
    source: "assetStore.persistRecentCache",
  });
  try {
    const cachedAssets = assets.slice(0, 20).map(cacheAsset);
    uni.setStorageSync(recentCacheKey(scope), {
      scope,
      storedAt: Date.now(),
      assets: cachedAssets,
    } satisfies RecentWorksCache);
    timing.end({ itemCount: cachedAssets.length });
  } catch {
    timing.end({ note: "cache_write_failed" });
  }
}

function errorText(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function errorFlags(error: unknown) {
  const message = errorText(error, "").toLowerCase();
  return {
    offline: message.includes("network") || message.includes("offline") || message.includes("网络"),
    noPermission: message.includes("403") || message.includes("forbidden") || message.includes("无权") || message.includes("权限"),
  };
}

let pollingTimer: ReturnType<typeof setTimeout> | null = null;
let activeRefreshPromise: Promise<void> | null = null;
let activeRecentRequest: { scope: string; promise: Promise<void>; abort: () => void } | null = null;

export const useAssetStore = defineStore("assets", {
  state: () => {
    return {
    assets: [] as AssetItem[],
    overview: emptyOverview(),
    filters: readFilters(),
    sort: readSort(),
    pagination: { page: 1, pageSize: 20, total: 0, hasMore: false },
    loading: false,
    overviewLoading: false,
    overviewError: "",
    refreshing: false,
    loadingMore: false,
    error: "",
    offline: false,
    noPermission: false,
    selectedIds: [] as string[],
    multiSelectMode: false,
    recentTasks: [] as GenerationTask[],
    tasks: [] as GenerationTask[],
    taskPagination: { page: 1, pageSize: 20, total: 0, hasMore: false },
    tasksLoading: false,
    taskListLoading: false,
    taskListLoadingMore: false,
    taskError: "",
    retryingTaskIds: [] as string[],
    currentAsset: null as AssetDetail | null,
    currentLoading: false,
    projects: [] as ProjectOption[],
    requestSequence: 0,
    recentRequestSequence: 0,
    pollingFailures: 0,
    pageVisible: false,
    lastRefreshAt: 0,
    lastRecentRequestStartedAt: 0,
    recentCacheScope: "",
    recentCacheHydrated: false,
    showingCachedWorks: false,
  };
  },
  getters: {
    selectedAssets(state): AssetItem[] {
      return state.assets.filter(item => state.selectedIds.includes(item.id));
    },
    hasActiveTasks(state): boolean {
      return state.recentTasks.some(item => item.status === "queued" || item.status === "generating");
    },
    isSearchResult(state): boolean {
      return Boolean(state.filters.keyword.trim());
    },
    isDefaultRecentView(state): boolean {
      return state.filters.type === "all"
        && state.filters.status === "recent"
        && !state.filters.keyword.trim()
        && !state.filters.projectId
        && !state.filters.tagIds.length
        && !state.filters.model
        && !state.filters.createdFrom
        && !state.filters.createdTo
        && state.sort === "created_desc";
    },
  },
  actions: {
    persistPreferences() {
      persist(this.filters, this.sort);
    },
    persistRecentCache() {
      if (this.isDefaultRecentView) writeRecentCache(this.recentCacheScope, this.assets);
    },
    hydrateRecentWorksCache(scope: string) {
      const normalizedScope = String(scope || "").trim();
      if (!normalizedScope) return false;
      if (this.recentCacheHydrated && this.recentCacheScope === normalizedScope) {
        const now = Date.now();
        recordWorksPerformance("local_cache_read", now, now, {
          serialWait: false,
          source: "assetStore.hydrateRecentWorksCache",
          duplicate: true,
          cacheHit: this.assets.length > 0,
          itemCount: this.assets.length,
        });
        return this.assets.length > 0;
      }
      if (activeRecentRequest && activeRecentRequest.scope !== normalizedScope) {
        activeRecentRequest.abort();
        activeRecentRequest = null;
        this.recentRequestSequence += 1;
      }
      const cache = readRecentCache(normalizedScope);
      this.recentCacheScope = normalizedScope;
      this.recentCacheHydrated = true;
      this.assets = cache?.assets || [];
      this.lastRefreshAt = Number(cache?.storedAt || 0);
      this.showingCachedWorks = Boolean(cache?.assets.length);
      this.loading = !cache?.assets.length;
      this.error = "";
      this.offline = false;
      return Boolean(cache?.assets.length);
    },
    async refreshRecentWorks(options: { scope?: string; source?: string; force?: boolean; silent?: boolean } = {}) {
      const scope = String(options.scope || this.recentCacheScope || "").trim();
      const source = options.source || "unknown";
      if (!scope) return;
      if (!this.recentCacheHydrated || this.recentCacheScope !== scope) this.hydrateRecentWorksCache(scope);
      if (activeRecentRequest && activeRecentRequest.scope === scope) {
        const now = Date.now();
        recordWorksPerformance("recent_works_request", now, now, {
          serialWait: false,
          source,
          requestUrl: "/api/v1/works/recent?limit=20",
          duplicate: true,
          itemCount: this.assets.length,
          note: "reuse_in_flight_request",
        });
        await activeRecentRequest.promise;
        return;
      }
      const requestStartedAt = Date.now();
      if (shouldDedupeRecentRequest(this.lastRecentRequestStartedAt, requestStartedAt, options.force, recentRequestDedupeMs)) {
        const now = requestStartedAt;
        recordWorksPerformance("recent_works_request", now, now, {
          serialWait: false,
          source,
          requestUrl: "/api/v1/works/recent?limit=20",
          duplicate: true,
          itemCount: this.assets.length,
          note: "dedupe_window",
        });
        return;
      }
      if (activeRecentRequest && activeRecentRequest.scope !== scope) {
        activeRecentRequest.abort();
        activeRecentRequest = null;
      }
      const sequence = ++this.recentRequestSequence;
      this.lastRecentRequestStartedAt = requestStartedAt;
      this.loading = this.assets.length === 0;
      this.refreshing = false;
      this.error = "";
      this.offline = false;
      this.noPermission = false;
      const timing = beginWorksPerformanceStep("recent_works_request", {
        serialWait: false,
        source,
        requestUrl: "/api/v1/works/recent?limit=20",
      });
      const request = fetchRecentWorksTask(20);
      const promise = (async () => {
        try {
          const incoming = await request.promise;
          if (!isCurrentRecentRequest(sequence, this.recentRequestSequence, scope, this.recentCacheScope)) return;
          this.assets = mergeRecentWorks(this.assets, incoming, 20);
          this.pagination = {
            page: 1,
            pageSize: 20,
            total: this.assets.length,
            hasMore: false,
          };
          this.lastRefreshAt = Date.now();
          this.showingCachedWorks = false;
          this.persistRecentCache();
          timing.end({ itemCount: incoming.length });
        } catch (error) {
          if (!isCurrentRecentRequest(sequence, this.recentRequestSequence, scope, this.recentCacheScope)) return;
          this.error = errorText(error, "作品加载失败");
          const flags = errorFlags(error);
          this.offline = flags.offline;
          this.noPermission = flags.noPermission;
          this.showingCachedWorks = this.assets.length > 0;
          timing.end({ itemCount: this.assets.length, note: this.offline ? "offline_cache_retained" : "request_failed" });
        } finally {
          if (sequence === this.recentRequestSequence) this.loading = false;
        }
      })();
      activeRecentRequest = { scope, promise, abort: request.abort };
      try {
        await promise;
      } finally {
        if (activeRecentRequest?.promise === promise) activeRecentRequest = null;
      }
    },
    async fetchOverview() {
      const timing = beginWorksPerformanceStep("asset_overview_refresh", {
        serialWait: false,
        source: "assetStore.fetchOverview",
        requestUrl: "/api/v1/assets/overview",
      });
      this.overviewLoading = true;
      this.overviewError = "";
      try {
        this.overview = await fetchAssetOverview();
        timing.end();
      } catch (error) {
        this.overviewError = errorText(error, "资产概览加载失败");
        timing.end({ note: "request_failed" });
      } finally {
        this.overviewLoading = false;
      }
    },
    async fetchAssets(options: { reset?: boolean; pageSize?: number } = {}) {
      const reset = options.reset !== false;
      if (!reset && (this.loading || this.loadingMore || !this.pagination.hasMore)) return;
      const sequence = ++this.requestSequence;
      const page = reset ? 1 : this.pagination.page + 1;
      const pageSize = options.pageSize || this.pagination.pageSize || 20;
      if (reset) {
        this.loading = true;
        this.loadingMore = false;
      }
      else this.loadingMore = true;
      this.error = "";
      this.offline = false;
      this.noPermission = false;
      try {
        const result = await fetchAssetPage(page, pageSize, this.filters, this.sort);
        if (sequence !== this.requestSequence) return;
        const previousById = new Map(this.assets.map(item => [item.id, item]));
        const incoming = result.items.map(item => stableAsset(previousById.get(item.id), item));
        const merged = reset ? incoming : [...this.assets, ...incoming];
        this.assets = [...new Map(merged.map(item => [item.id, item])).values()];
        this.pagination = { page: result.page, pageSize: result.pageSize, total: result.total, hasMore: result.hasMore };
        if (result.overview) this.overview = { ...this.overview, ...result.overview };
      } catch (error) {
        if (sequence !== this.requestSequence) return;
        this.error = errorText(error, "作品加载失败");
        const flags = errorFlags(error);
        this.offline = flags.offline;
        this.noPermission = flags.noPermission;
      } finally {
        if (sequence === this.requestSequence) {
          this.loading = false;
          this.loadingMore = false;
        }
      }
    },
    async refreshAssets(pageSize?: number, options: { silent?: boolean } = {}) {
      if (activeRefreshPromise) {
        await activeRefreshPromise;
        return;
      }
      this.refreshing = !options.silent;
      this.selectedIds = [];
      activeRefreshPromise = this.fetchAssets({ reset: true, pageSize });
      try {
        await activeRefreshPromise;
        this.lastRefreshAt = Date.now();
        this.persistRecentCache();
      } finally {
        activeRefreshPromise = null;
        this.refreshing = false;
      }
    },
    refreshAssetCenterBackground(source = "page_show") {
      const timing = beginWorksPerformanceStep("asset_center_background_refresh", {
        serialWait: false,
        source,
        requestUrl: "/api/v1/assets/overview + /api/v1/generation-tasks",
      });
      return Promise.allSettled([
        this.fetchOverview(),
        this.fetchRecentTasks({ refreshCompletedAssets: false }),
      ]).then(() => {
        timing.end();
      });
    },
    loadMoreAssets() {
      return this.fetchAssets({ reset: false });
    },
    async setType(type: AssetType, pageSize?: number) {
      this.filters.type = type;
      this.persistPreferences();
      await this.fetchAssets({ reset: true, pageSize });
    },
    async setStatus(status: AssetStatus, pageSize?: number) {
      this.filters.status = status;
      this.filters.favorite = status === "favorite" ? true : undefined;
      this.persistPreferences();
      await this.fetchAssets({ reset: true, pageSize });
    },
    async setFilters(filters: Partial<AssetFilter>, pageSize?: number) {
      this.filters = { ...this.filters, ...filters, tagIds: filters.tagIds || this.filters.tagIds };
      this.filters.favorite = this.filters.status === "favorite" ? true : undefined;
      this.persistPreferences();
      await this.fetchAssets({ reset: true, pageSize });
    },
    async clearFilters(pageSize?: number) {
      this.filters = defaultAssetFilter();
      this.persistPreferences();
      await this.fetchAssets({ reset: true, pageSize });
    },
    async setSort(sort: AssetSort, pageSize?: number) {
      this.sort = sort;
      this.persistPreferences();
      await this.fetchAssets({ reset: true, pageSize });
    },
    toggleSelect(id: string) {
      this.selectedIds = this.selectedIds.includes(id) ? this.selectedIds.filter(item => item !== id) : [...this.selectedIds, id];
    },
    selectAll() {
      this.selectedIds = this.assets.map(item => item.id);
    },
    clearSelection() {
      this.selectedIds = [];
    },
    setMultiSelectMode(value: boolean) {
      this.multiSelectMode = value;
      if (!value) this.clearSelection();
    },
    replaceAsset(item: AssetItem) {
      const index = this.assets.findIndex(asset => asset.id === item.id);
      if (index >= 0) this.assets.splice(index, 1, item);
      if (this.currentAsset?.id === item.id) this.currentAsset = { ...this.currentAsset, ...item };
    },
    primeCurrentAsset(item: AssetItem) {
      this.currentAsset = {
        ...item,
        downloadUrl: "",
        shareUrl: item.remoteUrl,
        variables: item.metadata?.variables && typeof item.metadata.variables === "object" && !Array.isArray(item.metadata.variables)
          ? item.metadata.variables as Record<string, unknown>
          : {},
      };
      this.error = "";
    },
    async toggleFavorite(id: string) {
      const item = this.assets.find(asset => asset.id === id) || this.currentAsset;
      if (!item) return;
      const previous = item.favorite;
      item.favorite = !previous;
      this.overview.favoriteTotal = Math.max(0, this.overview.favoriteTotal + (item.favorite ? 1 : -1));
      try {
        this.replaceAsset(await setAssetFavorite(id, item.favorite));
        uni.showToast({ title: item.favorite ? "已收藏" : "已取消收藏", icon: "success" });
      } catch (error) {
        item.favorite = previous;
        this.overview.favoriteTotal = Math.max(0, this.overview.favoriteTotal + (previous ? 1 : -1));
        uni.showToast({ title: errorText(error, "收藏操作失败"), icon: "none" });
        throw error;
      }
    },
    async renameAsset(id: string, name: string) {
      this.replaceAsset(await renameAsset(id, name));
    },
    async moveToProject(id: string, projectId: string, projectName = "") {
      this.replaceAsset(await moveAssetToProject(id, projectId, projectName));
    },
    async archiveAsset(id: string) {
      this.replaceAsset(await archiveAssetRequest(id));
      if (this.filters.status !== "archived") this.assets = this.assets.filter(item => item.id !== id);
    },
    async deleteAsset(id: string) {
      await deleteAssetRequest(id);
      this.assets = this.assets.filter(item => item.id !== id);
      this.pagination.total = Math.max(0, this.pagination.total - 1);
      this.overview.total = Math.max(0, this.overview.total - 1);
    },
    async restoreAsset(id: string) {
      const item = await restoreAssetRequest(id);
      this.assets = this.assets.filter(asset => asset.id !== id);
      this.replaceAsset(item);
    },
    async permanentlyDeleteAsset(id: string) {
      await permanentlyDeleteAsset(id);
      this.assets = this.assets.filter(item => item.id !== id);
      this.pagination.total = Math.max(0, this.pagination.total - 1);
    },
    async applyBatch(payload: Omit<AssetBatchPayload, "ids">) {
      if (!this.selectedIds.length) return;
      const ids = [...this.selectedIds];
      await batchAssets({ ...payload, ids });
      this.clearSelection();
      await Promise.all([this.fetchOverview(), this.fetchAssets({ reset: true })]);
    },
    async loadCurrentAsset(id: string) {
      this.currentLoading = true;
      this.error = "";
      try {
        this.currentAsset = await fetchAssetDetail(id);
        return this.currentAsset;
      } catch (error) {
        this.currentAsset = null;
        this.error = errorText(error, "作品详情加载失败");
        throw error;
      } finally {
        this.currentLoading = false;
      }
    },
    async fetchRecentTasks(options: { refreshCompletedAssets?: boolean } = {}) {
      if (this.tasksLoading) {
        const now = Date.now();
        recordWorksPerformance("task_sync", now, now, {
          serialWait: false,
          source: "assetStore.fetchRecentTasks",
          requestUrl: "/api/v1/generation-tasks",
          duplicate: true,
        });
        return;
      }
      const timing = beginWorksPerformanceStep("task_sync", {
        serialWait: false,
        source: "assetStore.fetchRecentTasks",
        requestUrl: "/api/v1/generation-tasks",
      });
      this.tasksLoading = true;
      this.taskError = "";
      try {
        const nextTasks = (await fetchTaskPage(1, 5)).items;
        const assetIds = new Set(this.assets.map(item => item.id));
        const hasNewCompletedResult = nextTasks.some(task => task.status === "completed" && task.resultIds.some(id => !assetIds.has(id)));
        this.recentTasks = nextTasks;
        this.pollingFailures = 0;
        if (options.refreshCompletedAssets !== false && hasNewCompletedResult && !this.loading && !activeRefreshPromise && this.isDefaultRecentView) {
          await this.refreshRecentWorks({ source: "task_sync", force: true, silent: true });
        }
        timing.end({ itemCount: nextTasks.length });
      } catch (error) {
        this.taskError = errorText(error, "任务加载失败");
        this.pollingFailures += 1;
        timing.end({ note: "request_failed" });
      } finally {
        this.tasksLoading = false;
      }
    },
    async fetchTasks(reset = true) {
      if ((reset && this.taskListLoading) || (!reset && (this.taskListLoadingMore || !this.taskPagination.hasMore))) return;
      if (reset) this.taskListLoading = true;
      else this.taskListLoadingMore = true;
      this.taskError = "";
      try {
        const page = reset ? 1 : this.taskPagination.page + 1;
        const result = await fetchTaskPage(page, this.taskPagination.pageSize);
        const merged = reset ? result.items : [...this.tasks, ...result.items];
        this.tasks = [...new Map(merged.map(item => [item.id, item])).values()];
        this.taskPagination = { page: result.page, pageSize: result.pageSize, total: result.total, hasMore: result.hasMore };
      } catch (error) {
        this.taskError = errorText(error, "任务加载失败");
      } finally {
        this.taskListLoading = false;
        this.taskListLoadingMore = false;
      }
    },
    loadMoreTasks() {
      return this.fetchTasks(false);
    },
    async retryTask(id: string) {
      if (this.retryingTaskIds.includes(id)) throw new Error("任务正在重新提交，请稍候");
      this.retryingTaskIds.push(id);
      try {
        const task = await retryGenerationTask(id);
        this.recentTasks = [task, ...this.recentTasks.filter(item => item.id !== task.id)].slice(0, 5);
        this.tasks = [task, ...this.tasks.filter(item => item.id !== task.id)];
        this.scheduleTaskPolling(1000);
        return task;
      } finally {
        this.retryingTaskIds = this.retryingTaskIds.filter(item => item !== id);
      }
    },
    async cancelTask(id: string) {
      const task = await cancelGenerationTask(id);
      const index = this.recentTasks.findIndex(item => item.id === id);
      if (index >= 0) this.recentTasks.splice(index, 1, task);
      const listIndex = this.tasks.findIndex(item => item.id === id);
      if (listIndex >= 0) this.tasks.splice(listIndex, 1, task);
      return task;
    },
    async loadProjects() {
      try {
        this.projects = await fetchProjects();
      } catch {
        this.projects = [];
      }
    },
    startTaskPolling(initialDelay = 4000) {
      this.pageVisible = true;
      this.scheduleTaskPolling(initialDelay);
    },
    stopTaskPolling() {
      this.pageVisible = false;
      if (pollingTimer) clearTimeout(pollingTimer);
      pollingTimer = null;
    },
    scheduleTaskPolling(delay?: number) {
      if (pollingTimer) clearTimeout(pollingTimer);
      if (!this.pageVisible) return;
      const backoff = Math.min(30000, 4000 * Math.max(1, 2 ** this.pollingFailures));
      pollingTimer = setTimeout(async () => {
        await this.fetchRecentTasks();
        if (this.tasks.length) await this.fetchTasks(true);
        if (this.hasActiveTasks || this.pollingFailures > 0) this.scheduleTaskPolling();
      }, delay ?? backoff);
    },
  },
});
