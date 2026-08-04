export type PersonalWalletContextType = "PERSONAL" | "ENTERPRISE" | "AGENT" | "OPERATION" | "";

export interface PersonalPointsAccount {
  id: string;
  userId: string;
  available: number;
  frozen: number;
  total?: number;
  permanentAvailable?: number;
  expiringAvailable?: number;
  nextExpiryAt?: string;
  nextExpiryPoints?: number;
}

export interface PersonalPointsWalletPayload {
  account?: PersonalPointsAccount;
  transactions?: Record<string, unknown>[];
  orders?: Record<string, unknown>[];
  tokenRecords?: Record<string, unknown>[];
}

export interface PersonalPointsWalletStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
}

export interface PersonalPointsWalletState {
  scope: string | null;
  payload: PersonalPointsWalletPayload | null;
  status: "hidden" | "ready" | "stale" | "error";
  stale: boolean;
  error: string;
  storedAt: number | null;
}

interface PersonalPointsWalletCacheEnvelope {
  schemaVersion: 1;
  scope: string;
  storedAt: number;
  payload: PersonalPointsWalletPayload;
}

interface LoadPersonalPointsWalletInput {
  userId: string;
  contextType: PersonalWalletContextType;
  storage: PersonalPointsWalletStorage;
  request: () => Promise<PersonalPointsWalletPayload>;
  now?: () => number;
  deferCacheWrite?: boolean;
}

export interface PersonalPointsWalletRuntimeScope {
  sessionKey: string;
  userId: string;
  contextType: PersonalWalletContextType;
  tenantId: string;
}

interface PersonalPointsWalletCoordinatorInput {
  getScope: () => PersonalPointsWalletRuntimeScope;
  storage: PersonalPointsWalletStorage;
  request: () => Promise<PersonalPointsWalletPayload>;
  now?: () => number;
  onChange?: (snapshot: { state: PersonalPointsWalletState; loading: boolean }) => void;
}

export interface PersonalPointsExpirySummary {
  expiringPoints: number;
  nextExpiryAt: string;
  nextExpiryPoints: number;
}

const personalPointsWalletCachePrefix = "zhiqiyun:personal-points-wallet:v1";
const giftSources = new Set([
  "REGISTRATION_GIFT",
  "ACTIVITY_GIFT",
  "ADMIN_GIFT",
  "COUPON_GRANT",
  "WECHAT_VIRTUAL_COUPON",
]);

function normalizedUserId(userId: string) {
  return String(userId || "").trim();
}

function walletScope(userId: string, contextType: PersonalWalletContextType) {
  const normalized = normalizedUserId(userId);
  return contextType === "PERSONAL" && normalized ? `PERSONAL:${normalized}` : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function validPayload(value: unknown, expectedUserId: string): value is PersonalPointsWalletPayload {
  if (!isRecord(value) || !isRecord(value.account)) return false;
  const account = value.account;
  return typeof account.id === "string"
    && account.userId === expectedUserId
    && Number.isFinite(account.available)
    && Number.isFinite(account.frozen);
}

function errorMessage(stale: boolean) {
  return stale
    ? "点数同步失败，当前显示上次成功数据，数据可能已过期，请重试"
    : "点数余额加载失败，请重试";
}

function emptyState(): PersonalPointsWalletState {
  return { scope: null, payload: null, status: "hidden", stale: false, error: "", storedAt: null };
}

function runtimeScopeKey(scope: PersonalPointsWalletRuntimeScope) {
  const sessionKey = String(scope.sessionKey || "").trim();
  const userId = normalizedUserId(scope.userId);
  if (!sessionKey || !userId || scope.contextType !== "PERSONAL") return null;
  return `${sessionKey}\u0000${userId}\u0000PERSONAL\u0000${String(scope.tenantId || "").trim()}`;
}

export function createPersonalPointsWalletCacheKey(userId: string, contextType: PersonalWalletContextType) {
  const scope = walletScope(userId, contextType);
  return scope ? `${personalPointsWalletCachePrefix}:${scope}` : null;
}

export function readPersonalPointsWalletCache(
  userId: string,
  contextType: PersonalWalletContextType,
  storage: PersonalPointsWalletStorage,
): PersonalPointsWalletState | null {
  const scope = walletScope(userId, contextType);
  const key = createPersonalPointsWalletCacheKey(userId, contextType);
  if (!scope || !key) return null;
  try {
    const value = storage.get(key);
    if (!isRecord(value)
      || value.schemaVersion !== 1
      || value.scope !== scope
      || !Number.isFinite(value.storedAt)
      || !validPayload(value.payload, normalizedUserId(userId))) return null;
    return {
      scope,
      payload: value.payload,
      status: "stale",
      stale: true,
      error: "当前显示上次成功数据，数据可能已过期",
      storedAt: value.storedAt as number,
    };
  } catch {
    return null;
  }
}

function writePersonalPointsWalletCache(
  scope: string,
  payload: PersonalPointsWalletPayload,
  storedAt: number,
  storage: PersonalPointsWalletStorage,
) {
  const key = `${personalPointsWalletCachePrefix}:${scope}`;
  const envelope: PersonalPointsWalletCacheEnvelope = {
    schemaVersion: 1,
    scope,
    storedAt,
    payload,
  };
  try {
    storage.set(key, envelope);
  } catch {
    // Cache failures must never turn a successful API response into a wallet failure.
  }
}

export async function loadPersonalPointsWallet(input: LoadPersonalPointsWalletInput): Promise<PersonalPointsWalletState> {
  const scope = walletScope(input.userId, input.contextType);
  if (!scope) {
    return { scope: null, payload: null, status: "hidden", stale: false, error: "", storedAt: null };
  }

  const cached = readPersonalPointsWalletCache(input.userId, input.contextType, input.storage);
  try {
    const payload = await input.request();
    if (!validPayload(payload, normalizedUserId(input.userId))) throw new Error("invalid personal point account response");
    const storedAt = (input.now || Date.now)();
    if (!input.deferCacheWrite) writePersonalPointsWalletCache(scope, payload, storedAt, input.storage);
    return { scope, payload, status: "ready", stale: false, error: "", storedAt };
  } catch {
    if (cached?.payload) return { ...cached, error: errorMessage(true) };
    return { scope, payload: null, status: "error", stale: false, error: errorMessage(false), storedAt: null };
  }
}

export function createPersonalPointsWalletCoordinator(input: PersonalPointsWalletCoordinatorInput) {
  let epoch = 0;
  let state = emptyState();
  let loading = false;
  const emit = () => input.onChange?.({ state, loading });
  const hide = () => {
    state = emptyState();
    loading = false;
    emit();
  };

  return {
    snapshot: () => ({ state, loading }),
    invalidate() {
      epoch += 1;
      hide();
    },
    async refresh() {
      const requestEpoch = ++epoch;
      const scope = input.getScope();
      const scopeKey = runtimeScopeKey(scope);
      if (!scopeKey) {
        hide();
        return state;
      }

      state = readPersonalPointsWalletCache(scope.userId, scope.contextType, input.storage) || emptyState();
      loading = true;
      emit();
      const nextState = await loadPersonalPointsWallet({
        userId: scope.userId,
        contextType: scope.contextType,
        storage: input.storage,
        request: input.request,
        now: input.now,
        deferCacheWrite: true,
      });
      if (requestEpoch !== epoch) return state;
      if (scopeKey !== runtimeScopeKey(input.getScope())) {
        hide();
        return state;
      }
      if (nextState.status === "ready" && nextState.scope && nextState.payload && nextState.storedAt !== null) {
        writePersonalPointsWalletCache(nextState.scope, nextState.payload, nextState.storedAt, input.storage);
      }
      state = nextState;
      loading = false;
      emit();
      return state;
    },
  };
}

export function personalPointsExpirySummary(account: PersonalPointsAccount | null | undefined): PersonalPointsExpirySummary | null {
  const expiringPoints = Number(account?.expiringAvailable);
  if (!Number.isFinite(expiringPoints) || expiringPoints <= 0) return null;
  const nextExpiryPoints = Number(account?.nextExpiryPoints);
  return {
    expiringPoints,
    nextExpiryAt: typeof account?.nextExpiryAt === "string" ? account.nextExpiryAt.trim() : "",
    nextExpiryPoints: Number.isFinite(nextExpiryPoints) && nextExpiryPoints > 0 ? nextExpiryPoints : 0,
  };
}

function explicitString(row: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = row[key];
    if (typeof value === "string" && value.trim()) return value.trim().toUpperCase();
  }
  return "";
}

export function personalPointEntryKind(row: unknown): "GRANT" | "EXPIRE" | null {
  if (!isRecord(row)) return null;
  const entryType = explicitString(row, ["entryType", "changeType", "type"]);
  if (entryType === "GRANT") return "GRANT";
  if (entryType === "EXPIRE") return "EXPIRE";
  const source = explicitString(row, ["sourceType", "source"]);
  if (giftSources.has(source)) return "GRANT";
  if (source === "EXPIRE" || source === "EXPIRY") return "EXPIRE";
  return null;
}
