import type { AuthResponse } from "@xianzhi/shared-types";
import type { PlatformAdapter } from "@xianzhi/platform-adapter";

export type AuthStatus = "guest" | "authenticated" | "expired";
export type PageAccess = "public" | "guest-visible" | "authenticated";

export interface UnifiedAuthState<TUser = AuthResponse["user"]> {
  status: AuthStatus;
  token: string | null;
  user: TUser | null;
  isGuest: boolean;
  isAuthenticated: boolean;
}

export interface ReviewModeConfig {
  enabled: boolean;
  hideRecharge: boolean;
  hideWallet: boolean;
  hideInvite: boolean;
  hideCommission: boolean;
  hideAgentCenter: boolean;
  hideOperatorCenter: boolean;
  hideSensitiveMarketing: boolean;
}

export type ProtectedAction =
  | "generate_chat"
  | "generate_image"
  | "generate_video"
  | "generate_ppt"
  | "generate_document"
  | "upload_file"
  | "upload_image"
  | "save_work"
  | "download_work"
  | "copy_private_content"
  | "create_agent"
  | "create_knowledge_base"
  | "recharge"
  | "open_wallet"
  | "open_order"
  | "open_member_center";

export interface PendingAction<TPayload extends Record<string, unknown> = Record<string, unknown>> {
  id: string;
  action: ProtectedAction;
  route: string;
  payload: TPayload;
  createdAt: number;
  expiresAt: number;
  autoResume: boolean;
}

export interface PendingActionInput<TPayload extends Record<string, unknown> = Record<string, unknown>> {
  action: ProtectedAction;
  route: string;
  payload?: TPayload;
  autoResume?: boolean;
  resume?: () => void | Promise<void>;
}

export interface PendingActionStoreOptions {
  adapter: PlatformAdapter;
  storageKey?: string;
  ttlMs?: number;
  now?: () => number;
  createId?: () => string;
}

export function safeInternalRedirect(value: unknown, fallback = "/") {
  const candidate = String(value || "").trim();
  if (!candidate.startsWith("/") || candidate.startsWith("//") || candidate.includes("\\") || /[\u0000-\u001f]/.test(candidate)) {
    return fallback;
  }
  try {
    const parsed = new URL(candidate, "https://zhiqiyun.invalid");
    if (parsed.origin !== "https://zhiqiyun.invalid") return fallback;
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return fallback;
  }
}

const sensitivePayloadKeys = /^(?:token|accessToken|refreshToken|password|mobile|phone|apiKey|authorization|secret)$/i;

function safePendingPayload(value: unknown, depth = 0): unknown {
  if (depth > 8 || value === undefined || typeof value === "function" || typeof value === "symbol") return undefined;
  if (value === null || typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
  if (Array.isArray(value)) return value.map(item => safePendingPayload(item, depth + 1)).filter(item => item !== undefined);
  if (typeof value !== "object") return undefined;
  return Object.fromEntries(Object.entries(value as Record<string, unknown>)
    .filter(([key]) => !sensitivePayloadKeys.test(key))
    .map(([key, item]) => [key, safePendingPayload(item, depth + 1)])
    .filter(([, item]) => item !== undefined));
}

export function createPendingActionStore(options: PendingActionStoreOptions) {
  const storageKey = options.storageKey || "zhiqiyun.auth.pending-action.v1";
  const ttlMs = options.ttlMs || 30 * 60 * 1000;
  const now = options.now || Date.now;
  const createId = options.createId || (() => `pending_${now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`);
  let current: PendingAction | null = null;
  let resumeCallback: (() => void | Promise<void>) | null = null;
  let executingId = "";

  const clear = () => {
    current = null;
    resumeCallback = null;
    executingId = "";
    options.adapter.removeStorage(storageKey);
  };
  const read = () => {
    const stored = current || options.adapter.getStorage<PendingAction>(storageKey) || null;
    if (!stored) return null;
    if (!stored.expiresAt || stored.expiresAt <= now()) {
      clear();
      return null;
    }
    current = stored;
    return stored;
  };

  return {
    save<TPayload extends Record<string, unknown>>(input: PendingActionInput<TPayload>) {
      const createdAt = now();
      current = {
        id: createId(),
        action: input.action,
        route: input.route,
        payload: (safePendingPayload(input.payload || {}) || {}) as Record<string, unknown>,
        createdAt,
        expiresAt: createdAt + ttlMs,
        autoResume: input.autoResume !== false,
      };
      resumeCallback = input.resume || null;
      options.adapter.setStorage(storageKey, current);
      return current;
    },
    get: read,
    consume() {
      const pending = read();
      if (!pending || executingId === pending.id) return null;
      clear();
      return pending;
    },
    clear,
    async resume(resolver?: (pending: PendingAction) => void | Promise<void>) {
      const pending = read();
      if (!pending || !pending.autoResume || executingId === pending.id) return false;
      const execute = resumeCallback || (resolver ? () => resolver(pending) : null);
      if (!execute) return false;
      executingId = pending.id;
      try {
        await execute();
        clear();
        return true;
      } catch (error) {
        executingId = "";
        throw error;
      }
    },
  };
}

export interface AuthGateOptions {
  getStatus: () => AuthStatus;
  pendingActions: ReturnType<typeof createPendingActionStore>;
  openLogin: (pending: PendingAction) => void | Promise<void>;
}

export function createAuthGate(options: AuthGateOptions) {
  let loginPromise: Promise<void> | null = null;
  return {
    async requireAuth(input: PendingActionInput) {
      if (options.getStatus() === "authenticated") {
        await input.resume?.();
        return true;
      }
      const pending = options.pendingActions.save(input);
      if (!loginPromise) {
        loginPromise = Promise.resolve(options.openLogin(pending)).finally(() => { loginPromise = null; });
      }
      await loginPromise;
      return false;
    },
    resumePendingAction: options.pendingActions.resume,
    clearPendingAction: options.pendingActions.clear,
  };
}
