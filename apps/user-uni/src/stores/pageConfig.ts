import { defineStore } from "pinia";
import { api } from "../api/client";
import { v531SlotsByPage } from "../config/v531";

export type AppPageCode = "home" | "studio" | "assets" | "profile";
export interface AppPageSlot {
  assetId?: string;
  imageUrl?: string;
  fallbackUrl?: string;
  altText?: string;
  aspectRatio?: string;
  fit?: "cover" | "contain";
  enabled: boolean;
  extraConfig?: Record<string, unknown>;
}
export interface AppPageConfig {
  pageCode: AppPageCode;
  tenantId?: string;
  version: string;
  status?: string;
  modules: Array<Record<string, unknown>>;
  slots: Record<string, AppPageSlot>;
}
interface PageResponse {
  code: number;
  message: string;
  data: AppPageConfig;
}
const storagePrefix = "xianzhi-page-config:";

function localConfig(code: AppPageCode): AppPageConfig {
  const slots = Object.fromEntries(
    Object.values(v531SlotsByPage[code]).map((item) => [
      item.slotKey,
      {
        fallbackUrl: item.fallbackUrl,
        altText: item.altText,
        aspectRatio: item.aspectRatio,
        fit: item.fit,
        enabled: true,
      },
    ]),
  );
  return {
    pageCode: code,
    version: "5.3.1-local",
    status: "FALLBACK",
    modules: [],
    slots,
  };
}

function mergeConfig(code: AppPageCode, remote?: AppPageConfig): AppPageConfig {
  const base = localConfig(code);
  if (!remote) return base;
  return {
    ...base,
    ...remote,
    slots: { ...base.slots, ...(remote.slots || {}) },
  };
}

function readCached(code: AppPageCode): AppPageConfig | undefined {
  try {
    const raw = uni.getStorageSync(storagePrefix + code);
    return raw ? (JSON.parse(String(raw)) as AppPageConfig) : undefined;
  } catch {
    return undefined;
  }
}
function writeCached(config: AppPageConfig) {
  try {
    uni.setStorageSync(storagePrefix + config.pageCode, JSON.stringify(config));
  } catch {
    /* cache is optional */
  }
}

export const usePageConfigStore = defineStore("pageConfig", {
  state: () => ({
    pages: {} as Partial<Record<AppPageCode, AppPageConfig>>,
    loading: {} as Partial<Record<AppPageCode, boolean>>,
    errors: {} as Partial<Record<AppPageCode, string>>,
  }),
  actions: {
    hydrate(code: AppPageCode) {
      if (!this.pages[code])
        this.pages[code] = mergeConfig(code, readCached(code));
    },
    async refresh(code: AppPageCode) {
      this.hydrate(code);
      this.loading[code] = true;
      this.errors[code] = "";
      try {
        const response = await api<PageResponse>(`/api/v1/app/pages/${code}`);
        if (!response?.data) throw new Error("页面配置响应无效");
        const current = this.pages[code];
        const merged = mergeConfig(code, response.data);
        if (
          !current ||
          current.version !== merged.version ||
          JSON.stringify(current.slots) !== JSON.stringify(merged.slots)
        ) {
          this.pages[code] = merged;
          writeCached(merged);
        }
        return merged;
      } catch (error) {
        this.errors[code] =
          error instanceof Error ? error.message : "页面配置加载失败";
        return this.pages[code] || localConfig(code);
      } finally {
        this.loading[code] = false;
      }
    },
    async ensure(code: AppPageCode) {
      this.hydrate(code);
      return this.refresh(code);
    },
    slot(code: AppPageCode, key: string): AppPageSlot | undefined {
      return this.pages[code]?.slots?.[key];
    },
  },
});
