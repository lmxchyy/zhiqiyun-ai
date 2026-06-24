import { defineStore } from "pinia";

export interface AiSettingsDraft {
  currentProfile: string;
  profileName: string;
  providerType: string;
  apiUrl: string;
  apiProxy: boolean;
  apiKey: string;
  model: string;
  apiMode: string;
  timeout: number;
  streamImages: boolean;
  submitMode: string;
  clearInputAfterSubmit: boolean;
  persistInputOnRestart: boolean;
  reuseTaskApiProfile: boolean;
  showRetryButton: boolean;
  allowPromptRewrite: boolean;
  taskDoneNotify: boolean;
  agentScrollBottom: boolean;
  mathPromptTip: boolean;
  referenceEditAction: string;
  zipDownloadRoutes: string;
  agentApiMode: string;
  agentTextProfile: string;
  agentImageProfile: string;
  agentMaxToolRounds: number;
  agentWebSearch: boolean;
}

const aiSettingsStorageKey = "xianzhi-ai-image-settings";
const aiSettingsVersion = 1;

export function createAiSettingsDefaults(model = "gpt-image-2"): AiSettingsDraft {
  return {
    currentProfile: "default",
    profileName: "默认",
    providerType: "openai",
    apiUrl: "https://api.openai.com/v1",
    apiProxy: true,
    apiKey: "",
    model,
    apiMode: "images",
    timeout: 300,
    streamImages: true,
    submitMode: "ctrl-enter",
    clearInputAfterSubmit: true,
    persistInputOnRestart: true,
    reuseTaskApiProfile: true,
    showRetryButton: false,
    allowPromptRewrite: false,
    taskDoneNotify: false,
    agentScrollBottom: true,
    mathPromptTip: true,
    referenceEditAction: "ask",
    zipDownloadRoutes: "任务列表多选、收藏夹多选",
    agentApiMode: "off",
    agentTextProfile: "default",
    agentImageProfile: "default",
    agentMaxToolRounds: 15,
    agentWebSearch: false
  };
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function stringValue(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function numberValue(value: unknown, fallback: number, min: number, max: number): number {
  const next = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(next)) return fallback;
  return Math.min(max, Math.max(min, next));
}

export function normalizeAiSettings(input: unknown, fallbackModel = "gpt-image-2"): AiSettingsDraft {
  const fallback = createAiSettingsDefaults(fallbackModel);
  const record = asRecord(input);
  return {
    currentProfile: stringValue(record.currentProfile, fallback.currentProfile),
    profileName: stringValue(record.profileName, fallback.profileName),
    providerType: stringValue(record.providerType, fallback.providerType),
    apiUrl: stringValue(record.apiUrl, fallback.apiUrl),
    apiProxy: booleanValue(record.apiProxy, fallback.apiProxy),
    apiKey: stringValue(record.apiKey, fallback.apiKey),
    model: stringValue(record.model, fallback.model),
    apiMode: stringValue(record.apiMode, fallback.apiMode),
    timeout: numberValue(record.timeout, fallback.timeout, 30, 900),
    streamImages: booleanValue(record.streamImages, fallback.streamImages),
    submitMode: stringValue(record.submitMode, fallback.submitMode),
    clearInputAfterSubmit: booleanValue(record.clearInputAfterSubmit, fallback.clearInputAfterSubmit),
    persistInputOnRestart: booleanValue(record.persistInputOnRestart, fallback.persistInputOnRestart),
    reuseTaskApiProfile: booleanValue(record.reuseTaskApiProfile, fallback.reuseTaskApiProfile),
    showRetryButton: booleanValue(record.showRetryButton, fallback.showRetryButton),
    allowPromptRewrite: booleanValue(record.allowPromptRewrite, fallback.allowPromptRewrite),
    taskDoneNotify: booleanValue(record.taskDoneNotify, fallback.taskDoneNotify),
    agentScrollBottom: booleanValue(record.agentScrollBottom, fallback.agentScrollBottom),
    mathPromptTip: booleanValue(record.mathPromptTip, fallback.mathPromptTip),
    referenceEditAction: stringValue(record.referenceEditAction, fallback.referenceEditAction),
    zipDownloadRoutes: stringValue(record.zipDownloadRoutes, fallback.zipDownloadRoutes),
    agentApiMode: stringValue(record.agentApiMode, fallback.agentApiMode),
    agentTextProfile: stringValue(record.agentTextProfile, fallback.agentTextProfile),
    agentImageProfile: stringValue(record.agentImageProfile, fallback.agentImageProfile),
    agentMaxToolRounds: numberValue(record.agentMaxToolRounds, fallback.agentMaxToolRounds, 1, 50),
    agentWebSearch: booleanValue(record.agentWebSearch, fallback.agentWebSearch)
  };
}

function readPersistedSettings(fallbackModel: string): AiSettingsDraft {
  const fallback = createAiSettingsDefaults(fallbackModel);
  if (typeof window === "undefined") return fallback;
  try {
    const raw = window.localStorage.getItem(aiSettingsStorageKey);
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as unknown;
    const record = asRecord(parsed);
    const payload = "settings" in record ? record.settings : parsed;
    return normalizeAiSettings(payload, fallbackModel);
  } catch {
    return fallback;
  }
}

function persistSettings(settings: AiSettingsDraft) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(aiSettingsStorageKey, JSON.stringify({
    version: aiSettingsVersion,
    settings
  }));
}

export const useAiSettingsStore = defineStore("aiSettings", {
  state: () => ({
    loaded: false,
    settings: createAiSettingsDefaults()
  }),
  actions: {
    load(fallbackModel?: string) {
      const model = fallbackModel || this.settings.model;
      this.settings = readPersistedSettings(model);
      this.loaded = true;
      return this.settings;
    },
    createDraft(fallbackModel?: string) {
      const model = fallbackModel || this.settings.model;
      if (!this.loaded) this.load(model);
      return normalizeAiSettings(this.settings, model);
    },
    save(draft: unknown) {
      this.settings = normalizeAiSettings(draft, this.settings.model);
      this.loaded = true;
      persistSettings(this.settings);
      return this.settings;
    },
    reset(fallbackModel = "gpt-image-2") {
      this.settings = createAiSettingsDefaults(fallbackModel);
      this.loaded = true;
      if (typeof window !== "undefined") window.localStorage.removeItem(aiSettingsStorageKey);
      return this.settings;
    }
  }
});
