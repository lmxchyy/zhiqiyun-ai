export type PlatformKey =
  | "web"
  | "mp-weixin"
  | "app-android"
  | "app-ios"
  | "app-harmony"
  | "desktop"
  | "unknown";

export interface AdapterRequestOptions<TBody = unknown> {
  url: string;
  method?: string;
  header?: Record<string, string>;
  data?: TBody;
  timeout?: number;
}

export interface AdapterResponse<T = unknown> {
  statusCode: number;
  data: T;
  header?: Record<string, string>;
}

export interface PlatformClientInfo {
  platform: PlatformKey;
  appName?: string;
  appVersion?: string;
  language?: string;
  deviceBrand?: string;
  deviceModel?: string;
  osName?: string;
  osVersion?: string;
  raw?: Record<string, unknown>;
}

export interface AdapterUploadFileOptions {
  url: string;
  filePath?: string;
  file?: Blob;
  name?: string;
  formData?: Record<string, unknown>;
  header?: Record<string, string>;
  timeout?: number;
}

export interface AdapterDownloadFileOptions {
  url: string;
  header?: Record<string, string>;
  timeout?: number;
}

export interface AdapterDownloadFileResponse<T = unknown> {
  statusCode: number;
  tempFilePath?: string;
  data?: T;
  header?: Record<string, string>;
}

export interface AdapterPaymentOptions {
  provider?: string;
  orderInfo?: string | Record<string, unknown>;
  timeStamp?: string;
  nonceStr?: string;
  package?: string;
  signType?: string;
  paySign?: string;
  [key: string]: unknown;
}

export interface AdapterPaymentResponse {
  errMsg?: string;
  [key: string]: unknown;
}

export interface AdapterShareOptions {
  provider?: string;
  scene?: string;
  title?: string;
  text?: string;
  summary?: string;
  url?: string;
  path?: string;
  imageUrl?: string;
  [key: string]: unknown;
}

export interface AdapterShareResponse {
  errMsg?: string;
  [key: string]: unknown;
}

export interface AdapterChooseFileOptions {
  count?: number;
  multiple?: boolean;
  accept?: string;
  type?: "all" | "file" | "image" | "video" | "media";
  sourceType?: string[];
  sizeType?: string[];
  extension?: string[];
}

export interface AdapterChosenFile {
  name?: string;
  path?: string;
  filePath?: string;
  tempFilePath?: string;
  size?: number;
  type?: string;
  file?: File;
  raw?: unknown;
}

export interface AdapterChooseFileResponse {
  files: AdapterChosenFile[];
  raw?: unknown;
}

export interface PlatformAdapter {
  platform: PlatformKey;
  getClientInfo(): PlatformClientInfo;
  request<T = unknown, TBody = unknown>(options: AdapterRequestOptions<TBody>): Promise<AdapterResponse<T>>;
  uploadFile?<T = unknown>(options: AdapterUploadFileOptions): Promise<AdapterResponse<T>>;
  downloadFile?<T = unknown>(options: AdapterDownloadFileOptions): Promise<AdapterDownloadFileResponse<T>>;
  requestPayment?(options: AdapterPaymentOptions): Promise<AdapterPaymentResponse>;
  share?(options: AdapterShareOptions): Promise<AdapterShareResponse>;
  chooseFile?(options?: AdapterChooseFileOptions): Promise<AdapterChooseFileResponse>;
  getStorage<T = unknown>(key: string): T | undefined;
  setStorage<T = unknown>(key: string, value: T): void;
  removeStorage(key: string): void;
  login?(provider?: string): Promise<{ code?: string; [key: string]: unknown }>;
  setClipboardText?(text: string): Promise<void>;
  getClipboardText?(): Promise<string>;
  scanCode?(): Promise<{ result?: string; [key: string]: unknown }>;
  openURL?(url: string): Promise<void>;
  showToast?(message: string): void;
}

type UniLike = {
  getSystemInfoSync?: () => {
    uniPlatform?: string;
    platform?: string;
    appName?: string;
    appVersion?: string;
    language?: string;
    brand?: string;
    model?: string;
    osName?: string;
    osVersion?: string;
    system?: string;
    [key: string]: unknown;
  };
  request?: (options: Record<string, unknown>) => void;
  uploadFile?: (options: Record<string, unknown>) => void;
  downloadFile?: (options: Record<string, unknown>) => void;
  requestPayment?: (options: Record<string, unknown>) => void;
  share?: (options: Record<string, unknown>) => void;
  showShareMenu?: (options: Record<string, unknown>) => void;
  chooseFile?: (options: Record<string, unknown>) => void;
  chooseImage?: (options: Record<string, unknown>) => void;
  chooseMedia?: (options: Record<string, unknown>) => void;
  getStorageSync?: (key: string) => unknown;
  setStorageSync?: (key: string, value: unknown) => void;
  removeStorageSync?: (key: string) => void;
  login?: (options: Record<string, unknown>) => void;
  setClipboardData?: (options: Record<string, unknown>) => void;
  getClipboardData?: (options: Record<string, unknown>) => void;
  scanCode?: (options: Record<string, unknown>) => void;
  showToast?: (options: Record<string, unknown>) => void;
};

declare const uni: UniLike | undefined;

type DesktopWindow = Window & {
  __TAURI__?: unknown;
  __TAURI_INTERNALS__?: unknown;
  electronAPI?: unknown;
  ipcRenderer?: unknown;
};

function uniRuntime(): UniLike | undefined {
  return typeof uni === "undefined" ? undefined : uni;
}

function browserRuntime(): Window | undefined {
  return typeof window === "undefined" ? undefined : window;
}

function desktopWindowRuntime(): DesktopWindow | undefined {
  return browserRuntime() as DesktopWindow | undefined;
}

function navigatorRuntime(): Navigator | undefined {
  return typeof navigator === "undefined" ? undefined : navigator;
}

function documentRuntime(): Document | undefined {
  return typeof document === "undefined" ? undefined : document;
}

function unsupported(method: string): Promise<never> {
  return Promise.reject(new Error(`${method} is not available on this platform`));
}

function parseMaybeJson<T>(value: unknown): T {
  if (typeof value !== "string") return value as T;
  try {
    return JSON.parse(value) as T;
  } catch {
    return value as T;
  }
}

function normalizePlatform(raw?: string): PlatformKey {
  const value = String(raw || "").toLowerCase();
  if (value.includes("mp-weixin") || value.includes("weixin")) return "mp-weixin";
  if (value.includes("android")) return "app-android";
  if (value.includes("ios")) return "app-ios";
  if (value.includes("harmony")) return "app-harmony";
  if (value.includes("web") || value.includes("h5")) return "web";
  return "unknown";
}

function hasDesktopBridge(win?: DesktopWindow) {
  return Boolean(win?.__TAURI__ || win?.__TAURI_INTERNALS__ || win?.electronAPI || win?.ipcRenderer);
}

function normalizeFiles(result: Record<string, unknown>): AdapterChosenFile[] {
  const tempFiles = Array.isArray(result.tempFiles) ? result.tempFiles : [];
  if (tempFiles.length > 0) {
    return tempFiles.map(item => {
      const file = item as Record<string, unknown>;
      const path = String(file.path || file.tempFilePath || "");
      return {
        name: file.name ? String(file.name) : undefined,
        path: path || undefined,
        filePath: path || undefined,
        tempFilePath: path || undefined,
        size: typeof file.size === "number" ? file.size : undefined,
        type: file.type ? String(file.type) : undefined,
        raw: item
      };
    });
  }

  const tempFilePaths = Array.isArray(result.tempFilePaths) ? result.tempFilePaths : [];
  return tempFilePaths.map(item => {
    const path = String(item);
    return {
      path,
      filePath: path,
      tempFilePath: path,
      raw: item
    };
  });
}

function browserChooseFile(options: AdapterChooseFileOptions = {}): Promise<AdapterChooseFileResponse> {
  const doc = documentRuntime();
  if (!doc) return unsupported("chooseFile");

  return new Promise((resolve, reject) => {
    const input = doc.createElement("input");
    input.type = "file";
    input.style.display = "none";
    input.multiple = Boolean(options.multiple || (options.count && options.count > 1));
    if (options.accept) input.accept = options.accept;
    else if (options.type === "image") input.accept = "image/*";
    else if (options.type === "video") input.accept = "video/*";
    else if (options.type === "media") input.accept = "image/*,video/*";
    else if (options.extension?.length) input.accept = options.extension.map(item => (item.startsWith(".") ? item : `.${item}`)).join(",");

    input.addEventListener(
      "change",
      () => {
        const selected = Array.from(input.files || []);
        input.remove();
        resolve({
          files: selected.slice(0, options.count || selected.length).map(file => ({
            name: file.name,
            size: file.size,
            type: file.type,
            file,
            raw: file
          })),
          raw: selected
        });
      },
      { once: true }
    );

    input.addEventListener(
      "error",
      () => {
        input.remove();
        reject(new Error("chooseFile failed"));
      },
      { once: true }
    );

    doc.body.appendChild(input);
    input.click();
  });
}

export function detectPlatform(): PlatformKey {
  const win = desktopWindowRuntime();
  if (hasDesktopBridge(win)) return "desktop";

  const runtime = uniRuntime();
  try {
    const info = runtime?.getSystemInfoSync?.();
    const detected = normalizePlatform(info?.uniPlatform || info?.platform);
    if (detected !== "unknown") return detected;
  } catch {
    // Keep detection side-effect free on runtimes where system info is unavailable.
  }

  if (win?.document) return "web";
  return "unknown";
}

function getRuntimeClientInfo(platform: PlatformKey, runtime?: UniLike): PlatformClientInfo {
  let info: ReturnType<NonNullable<UniLike["getSystemInfoSync"]>> | undefined;
  try {
    info = runtime?.getSystemInfoSync?.();
  } catch {
    info = undefined;
  }

  const nav = navigatorRuntime();
  const system = String(info?.system || "");
  const [osName, osVersion] = system ? system.split(/\s+/, 2) : [];

  return {
    platform,
    appName: String(info?.appName || ""),
    appVersion: String(info?.appVersion || ""),
    language: String(info?.language || nav?.language || ""),
    deviceBrand: String(info?.brand || ""),
    deviceModel: String(info?.model || ""),
    osName: String(info?.osName || osName || ""),
    osVersion: String(info?.osVersion || osVersion || ""),
    raw: info as Record<string, unknown> | undefined
  };
}

export function createUniPlatformAdapter(platform: PlatformKey = detectPlatform()): PlatformAdapter {
  const runtime = uniRuntime();
  return {
    platform,
    getClientInfo() {
      return getRuntimeClientInfo(platform, runtime);
    },
    request<T = unknown, TBody = unknown>(options: AdapterRequestOptions<TBody>) {
      if (runtime?.request) {
        return new Promise<AdapterResponse<T>>((resolve, reject) => {
          runtime.request?.({
            url: options.url,
            method: options.method || "GET",
            header: options.header,
            data: options.data,
            timeout: options.timeout,
            success(response: { statusCode?: number; data?: T; header?: Record<string, string> }) {
              resolve({
                statusCode: Number(response.statusCode || 0),
                data: response.data as T,
                header: response.header
              });
            },
            fail(error: unknown) {
              reject(error instanceof Error ? error : new Error(String((error as { errMsg?: string })?.errMsg || error)));
            }
          });
        });
      }

      const fetchRuntime = globalThis.fetch;
      if (!fetchRuntime) {
        return Promise.reject(new Error("No request runtime is available"));
      }
      return fetchRuntime(options.url, {
        method: options.method || "GET",
        headers: options.header,
        body: options.data === undefined ? undefined : JSON.stringify(options.data)
      }).then(async response => ({
        statusCode: response.status,
        data: (await response.json().catch(() => ({}))) as T,
        header: {}
      }));
    },
    uploadFile<T = unknown>(options: AdapterUploadFileOptions) {
      if (runtime?.uploadFile && options.filePath) {
        return new Promise<AdapterResponse<T>>((resolve, reject) => {
          runtime.uploadFile?.({
            url: options.url,
            filePath: options.filePath,
            name: options.name || "file",
            formData: options.formData,
            header: options.header,
            timeout: options.timeout,
            success(response: { statusCode?: number; data?: unknown; header?: Record<string, string> }) {
              resolve({
                statusCode: Number(response.statusCode || 0),
                data: parseMaybeJson<T>(response.data),
                header: response.header
              });
            },
            fail: reject
          });
        });
      }

      const fetchRuntime = typeof globalThis.fetch === "function" ? globalThis.fetch.bind(globalThis) : undefined;
      if (fetchRuntime && options.file && typeof FormData !== "undefined") {
        const form = new FormData();
        form.append(options.name || "file", options.file);
        Object.entries(options.formData || {}).forEach(([key, value]) => form.append(key, String(value)));
        return fetchRuntime(options.url, {
          method: "POST",
          headers: options.header,
          body: form
        }).then(async response => ({
          statusCode: response.status,
          data: parseMaybeJson<T>(await response.text()),
          header: {}
        }));
      }

      return unsupported("uploadFile");
    },
    downloadFile<T = unknown>(options: AdapterDownloadFileOptions) {
      if (runtime?.downloadFile) {
        return new Promise<AdapterDownloadFileResponse<T>>((resolve, reject) => {
          runtime.downloadFile?.({
            url: options.url,
            header: options.header,
            timeout: options.timeout,
            success(response: { statusCode?: number; tempFilePath?: string; data?: T; header?: Record<string, string> }) {
              resolve({
                statusCode: Number(response.statusCode || 0),
                tempFilePath: response.tempFilePath,
                data: response.data,
                header: response.header
              });
            },
            fail: reject
          });
        });
      }

      const fetchRuntime = typeof globalThis.fetch === "function" ? globalThis.fetch.bind(globalThis) : undefined;
      if (fetchRuntime) {
        return fetchRuntime(options.url, { headers: options.header }).then(async response => ({
          statusCode: response.status,
          data: (await response.blob()) as T,
          header: {}
        }));
      }

      return unsupported("downloadFile");
    },
    requestPayment(options: AdapterPaymentOptions) {
      if (!runtime?.requestPayment) return unsupported("requestPayment");
      return new Promise((resolve, reject) => {
        runtime.requestPayment?.({
          ...options,
          success: resolve,
          fail: reject
        });
      });
    },
    share(options: AdapterShareOptions) {
      if (runtime?.share) {
        return new Promise((resolve, reject) => {
          runtime.share?.({
            ...options,
            href: options.url,
            success: resolve,
            fail: reject
          });
        });
      }

      if (runtime?.showShareMenu && platform === "mp-weixin") {
        return new Promise(resolve => {
          runtime.showShareMenu?.({
            withShareTicket: true,
            menus: ["shareAppMessage", "shareTimeline"],
            success: resolve,
            fail: resolve
          });
        });
      }

      const nav = navigatorRuntime() as (Navigator & { share?: (data: ShareData) => Promise<void> }) | undefined;
      if (nav?.share) {
        return nav.share({
          title: options.title,
          text: options.text || options.summary,
          url: options.url || options.path
        }).then(() => ({ errMsg: "share:ok" }));
      }

      return unsupported("share");
    },
    chooseFile(options: AdapterChooseFileOptions = {}) {
      if (runtime?.chooseFile) {
        return new Promise((resolve, reject) => {
          runtime.chooseFile?.({
            count: options.count,
            type: options.type,
            extension: options.extension,
            success(result: Record<string, unknown>) {
              resolve({ files: normalizeFiles(result), raw: result });
            },
            fail: reject
          });
        });
      }

      if (runtime?.chooseMedia && (options.type === "media" || options.type === "video")) {
        return new Promise((resolve, reject) => {
          runtime.chooseMedia?.({
            count: options.count,
            mediaType: options.type === "video" ? ["video"] : ["image", "video"],
            sourceType: options.sourceType,
            success(result: Record<string, unknown>) {
              resolve({ files: normalizeFiles(result), raw: result });
            },
            fail: reject
          });
        });
      }

      if (runtime?.chooseImage && (!options.type || options.type === "image")) {
        return new Promise((resolve, reject) => {
          runtime.chooseImage?.({
            count: options.count,
            sourceType: options.sourceType,
            sizeType: options.sizeType,
            success(result: Record<string, unknown>) {
              resolve({ files: normalizeFiles(result), raw: result });
            },
            fail: reject
          });
        });
      }

      return browserChooseFile(options);
    },
    getStorage<T = unknown>(key: string) {
      if (runtime?.getStorageSync) return runtime.getStorageSync(key) as T | undefined;
      const storage = browserRuntime()?.localStorage;
      const raw = storage?.getItem(key);
      if (!raw) return undefined;
      try {
        return JSON.parse(raw) as T;
      } catch {
        return raw as T;
      }
    },
    setStorage<T = unknown>(key: string, value: T) {
      if (runtime?.setStorageSync) {
        runtime.setStorageSync(key, value);
        return;
      }
      const raw = typeof value === "string" ? value : JSON.stringify(value);
      browserRuntime()?.localStorage?.setItem(key, raw);
    },
    removeStorage(key: string) {
      if (runtime?.removeStorageSync) {
        runtime.removeStorageSync(key);
        return;
      }
      browserRuntime()?.localStorage?.removeItem(key);
    },
    login(provider = "weixin") {
      if (!runtime?.login) return Promise.reject(new Error("Login is not available on this platform"));
      return new Promise((resolve, reject) => {
        runtime.login?.({
          provider,
          success: resolve,
          fail: reject
        });
      });
    },
    setClipboardText(text: string) {
      if (runtime?.setClipboardData) {
        return new Promise<void>((resolve, reject) => {
          runtime.setClipboardData?.({
            data: text,
            success: () => resolve(),
            fail: reject
          });
        });
      }
      const clipboard = navigatorRuntime()?.clipboard;
      if (clipboard?.writeText) return clipboard.writeText(text);
      return unsupported("setClipboardText");
    },
    getClipboardText() {
      if (runtime?.getClipboardData) {
        return new Promise<string>((resolve, reject) => {
          runtime.getClipboardData?.({
            success: (result: { data?: string }) => resolve(String(result.data || "")),
            fail: reject
          });
        });
      }
      const clipboard = navigatorRuntime()?.clipboard;
      if (clipboard?.readText) return clipboard.readText();
      return unsupported("getClipboardText");
    },
    scanCode() {
      if (!runtime?.scanCode) return unsupported("scanCode");
      return new Promise((resolve, reject) => {
        runtime.scanCode?.({
          success: resolve,
          fail: reject
        });
      });
    },
    openURL(url: string) {
      const win = browserRuntime();
      if (win?.open) {
        win.open(url, "_blank", "noopener,noreferrer");
        return Promise.resolve();
      }
      return unsupported("openURL");
    },
    showToast(message: string) {
      if (runtime?.showToast) {
        runtime.showToast({ title: message, icon: "none" });
        return;
      }
      // Browser toasts are intentionally left to the host app UI layer.
    }
  };
}
