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

const filterStorageKey = "zhiqiyun:asset-center:filters";
const sortStorageKey = "zhiqiyun:asset-center:sort";
const recentCacheStorageKey = "zhiqiyun:asset-center:recent-cache";
const recentCacheMaxAgeMs = 10 * 60 * 1000;

interface RecentAssetCache {
  storedAt: number;
  assets: AssetItem[];
  overview: AssetOverview;
  recentTasks: GenerationTask[];
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

function readRecentCache(): RecentAssetCache | null {
  try {
    const value = uni.getStorageSync(recentCacheStorageKey) as Partial<RecentAssetCache> | "";
    if (!value || typeof value !== "object" || !Number.isFinite(value.storedAt)) return null;
    if (Date.now() - Number(value.storedAt) > recentCacheMaxAgeMs) return null;
    if (!Array.isArray(value.assets) || !Array.isArray(value.recentTasks) || !value.overview) return null;
    return value as RecentAssetCache;
  } catch {
    return null;
  }
}

function cacheAsset(item: AssetItem): AssetItem {
  const metadata = { ...item.metadata };
  delete metadata.thumbnailUrl;
  return { ...item, metadata };
}

function writeRecentCache(assets: AssetItem[], overview: AssetOverview, recentTasks: GenerationTask[]) {
  try {
    uni.setStorageSync(recentCacheStorageKey, {
      storedAt: Date.now(),
      assets: assets.slice(0, 4).map(cacheAsset),
      overview,
      recentTasks: recentTasks.slice(0, 5),
    } satisfies RecentAssetCache);
  } catch {
    /* the cache is an optional first-paint optimization */
  }
}

function stableAsset(previous: AssetItem | undefined, next: AssetItem): AssetItem {
  if (!previous || previous.updatedAt !== next.updatedAt || next.thumbnailUrl.startsWith("data:image/")) return next;
  return {
    ...next,
    remoteUrl: previous.remoteUrl || next.remoteUrl,
    thumbnailUrl: previous.thumbnailUrl || next.thumbnailUrl,
  };
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

export const useAssetStore = defineStore("assets", {
  state: () => {
    const cache = readRecentCache();
    return {
    assets: cache?.assets || [] as AssetItem[],
    overview: cache?.overview || emptyOverview(),
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
    recentTasks: cache?.recentTasks || [] as GenerationTask[],
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
    pollingFailures: 0,
    pageVisible: false,
    lastRefreshAt: cache?.storedAt || 0,
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
  },
  actions: {
    persistPreferences() {
      persist(this.filters, this.sort);
    },
    persistRecentCache() {
      writeRecentCache(this.assets, this.overview, this.recentTasks);
    },
    async fetchOverview() {
      this.overviewLoading = true;
      this.overviewError = "";
      try {
        this.overview = await fetchAssetOverview();
      } catch (error) {
        this.overviewError = errorText(error, "资产概览加载失败");
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
      activeRefreshPromise = Promise.all([this.fetchOverview(), this.fetchAssets({ reset: true, pageSize }), this.fetchRecentTasks()]).then(() => undefined);
      try {
        await activeRefreshPromise;
        this.lastRefreshAt = Date.now();
        this.persistRecentCache();
      } finally {
        activeRefreshPromise = null;
        this.refreshing = false;
      }
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
    async fetchRecentTasks() {
      if (this.tasksLoading) return;
      this.tasksLoading = true;
      this.taskError = "";
      try {
        const nextTasks = (await fetchTaskPage(1, 5)).items;
        const assetIds = new Set(this.assets.map(item => item.id));
        const hasNewCompletedResult = nextTasks.some(task => task.status === "completed" && task.resultIds.some(id => !assetIds.has(id)));
        this.recentTasks = nextTasks;
        this.pollingFailures = 0;
        if (hasNewCompletedResult && !this.loading && this.filters.status === "recent") {
          await this.fetchAssets({ reset: true, pageSize: 4 });
          this.persistRecentCache();
        }
      } catch (error) {
        this.taskError = errorText(error, "任务加载失败");
        this.pollingFailures += 1;
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
    startTaskPolling() {
      this.pageVisible = true;
      this.scheduleTaskPolling(0);
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
