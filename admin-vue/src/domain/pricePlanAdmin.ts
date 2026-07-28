import type {
  PricePlan,
  PricePlanPaymentBinding,
  PricePlanValidation,
  PricingAuditFilters,
  PricingHealthPricePlan,
  PricingHealthStatus,
  PricingHealthWechatGood,
  WechatGoodReference,
  WechatVirtualGood,
  WhitelistUpdateInput
} from "../types/pricePlanAdmin.ts";

export const TEST_WHITELIST_ORDINARY_ENTRY_NOTICE = "加入白名单不会改变普通购买价格，用户仍需通过专用测试入口。";
export const WECHAT_MANUAL_CONFIRMATION_NOTICE = "人工确认已发布仅代表本地人工记录，系统未实时连接微信公众平台验证。";
export const PRICING_AUDIT_QUICK_FILTERS = [
  { id: "defaultSwitch", label: "默认价格切换", action: "price_plan.make_default" },
  { id: "goodConfirmation", label: "微信商品人工确认", action: "wechat_good.confirm_published" },
  { id: "whitelistCreate", label: "白名单新增", action: "price_plan.test_whitelist.create" },
  { id: "whitelistUpdate", label: "白名单修改", action: "price_plan.test_whitelist.update" },
  { id: "whitelistDisable", label: "白名单停用", action: "price_plan.test_whitelist.disable" }
] as const;

export type PricingPermission =
  | "pricing:plan:view"
  | "pricing:entitlement:manage"
  | "pricing:price-plan:manage"
  | "pricing:price-plan:default"
  | "pricing:wechat-good:manage"
  | "pricing:test-whitelist:manage"
  | "pricing:audit:view";

export interface PricingPrincipal {
  role?: string;
  roles?: string[];
  permissions?: string[];
}

export function pricingPrincipalFromAuthResponse(response: unknown): PricingPrincipal {
  const auth = isRecord(response) ? response : {};
  const user = isRecord(auth.user) ? auth.user : {};
  const role = typeof user.role === "string" ? user.role : typeof auth.currentRole === "string" ? auth.currentRole : "";
  const roles = Array.isArray(auth.roles) ? auth.roles.filter((item): item is string => typeof item === "string") : [];
  const permissions = Array.isArray(auth.permissions)
    ? auth.permissions.filter((item): item is string => typeof item === "string")
    : [];
  return { role, roles, permissions };
}

export function hasPricingPermission(principal: PricingPrincipal | null | undefined, permission: PricingPermission): boolean {
  if (!principal) return false;
  const roles = [principal.role, ...(principal.roles || [])]
    .map((role) => String(role || "").trim().toUpperCase())
    .filter(Boolean);
  if (roles.includes("SUPER_ADMIN")) return true;
  return (principal.permissions || []).some((candidate) => candidate === permission);
}

export async function loadPricingAuditIfAllowed<T>(
  principal: PricingPrincipal | null | undefined,
  filters: PricingAuditFilters,
  loader: (filters: PricingAuditFilters) => Promise<T>
): Promise<T | null> {
  if (!hasPricingPermission(principal, "pricing:audit:view")) return null;
  return loader({ ...filters });
}

export function createPricingAuditLoadExecutor<T>(
  principal: () => PricingPrincipal | null | undefined,
  loader: (filters: PricingAuditFilters) => Promise<T>
) {
  return (filters: PricingAuditFilters): Promise<T | null> => (
    loadPricingAuditIfAllowed(principal(), filters, loader)
  );
}

export type PricingAuditDisplayState = "FORBIDDEN" | "LOADING" | "ERROR" | "EMPTY" | "TABLE" | "TABLE_STALE";

export function pricingAuditDisplayState(input: {
  canView: boolean;
  cacheMatches: boolean;
  hasPage: boolean;
  loading: boolean;
  error?: string;
  rowCount?: number;
}): PricingAuditDisplayState {
  if (!input.canView) return "FORBIDDEN";
  const error = String(input.error || "").trim();
  if (input.cacheMatches && input.hasPage) {
    if (error) return "TABLE_STALE";
    if ((input.rowCount || 0) === 0 && !input.loading) return "EMPTY";
    return "TABLE";
  }
  if (input.loading) return "LOADING";
  if (error) return "ERROR";
  return "LOADING";
}

const pricingErrorMessages: Record<string, string> = {
  ADMIN_AUTHENTICATION_REQUIRED: "登录状态已失效，请重新登录。",
  ADMIN_PERMISSION_DENIED: "当前账号没有执行此操作的价格治理权限。",
  REVISION_REQUIRED: "缺少版本号，请刷新最新数据后重试。",
  REVISION_CONFLICT: "数据已被其他操作修改，请刷新后重试；不会自动覆盖服务器版本。",
  PRICE_PLAN_CODE_FORMAT_INVALID: "价格方案编码格式无效，仅允许小写字母、数字和下划线，且必须以字母开头。",
  PRICE_PLAN_CODE_HAS_PRICE: "价格方案编码包含明确价格语义，请使用不随价格变化的稳定编码。",
  PRICE_PLAN_CODE_EXISTS: "价格方案编码已存在，请更换稳定编码。",
  PRICE_PLAN_AMOUNT_INVALID: "价格必须使用正整数分，且原价不能低于售价。",
  PRICE_PLAN_VALIDITY_INVALID: "价格方案生效时间或失效时间无效。",
  PRICE_PLAN_VALIDITY_MUTATION_CONFLICT: "有效期修改参数冲突，请刷新后重新编辑。",
  PRICE_PLAN_MUTATION_REQUIRED: "没有检测到可保存的变更。",
  PRICE_PLAN_CLONE_REQUIRED: "当前价格方案不能原地修改经济字段，请克隆为新方案后调整。",
  PRICE_PLAN_REFRESH_REQUIRED: "上次价格方案写入已经成功，但最新权威状态尚未恢复；请先完成刷新，禁止重复提交。",
  PLAN_VERSION_REFRESH_REQUIRED: "上次权益版本写入已经成功，但最新权威状态尚未恢复；请先完成刷新，禁止重复提交。",
  PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE: "当前 DRAFT 方案存在启用中的支付绑定，请先停用绑定再修改经济字段。",
  PRICE_PLAN_TEST_REQUIRED: "只有 TEST 价格方案可以管理测试白名单。",
  INVALID_WHITELIST_QUERY: "白名单筛选或分页参数无效，请检查后重试。",
  WHITELIST_CREATE_REVISION_INVALID: "新建白名单记录的 revision 必须为 0。",
  WHITELIST_USER_REQUIRED: "必须选择测试用户。",
  WHITELIST_REASON_REQUIRED: "必须填写白名单资格原因。",
  WHITELIST_VALIDITY_MUTATION_CONFLICT: "白名单有效期修改参数冲突，请刷新后重新编辑。",
  WHITELIST_MUTATION_REQUIRED: "没有检测到可保存的白名单变更。",
  WHITELIST_VALIDITY_INVALID: "白名单生效时间或失效时间无效，失效时间必须晚于生效时间。",
  WHITELIST_USER_NOT_FOUND: "指定用户不存在，请核对用户 ID。",
  WHITELIST_ACTIVE_EXISTS: "该用户在当前 TEST 价格方案下已有有效白名单记录。",
  WHITELIST_ENTRY_NOT_FOUND: "白名单记录不存在，请刷新列表。",
  WHITELIST_ENTRY_TERMINAL: "已过期或已停用的白名单记录不可修改或恢复，请新建记录。",
  WHITELIST_TERMINAL_IMMUTABLE: "终态白名单记录不可修改或恢复，请新建记录。",
  WHITELIST_TEMPORALLY_EXPIRED_IMMUTABLE: "该白名单已到期，不可原地恢复，请新建记录。",
  WHITELIST_ENTRY_PRICE_PLAN_MISMATCH: "白名单记录与当前价格方案不匹配，请刷新后重试。",
  PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE: "白名单服务当前不可用，请稍后重试。",
  WHITELIST_REFRESH_REQUIRED: "上次白名单写入已成功，但最新列表尚未确认；请先完成精确刷新，禁止重复提交。",
  PRICING_AUDIT_FILTER_INVALID: "审计筛选条件无效，请检查后重试。",
  PRICING_AUDIT_PAGE_INVALID: "审计页码无效，请返回有效页码后重试。",
  PRICING_AUDIT_PAGE_SIZE_INVALID: "审计每页数量必须在 1 到 200 之间。",
  PRICING_AUDIT_RESULT_INVALID: "审计结果只能筛选成功或失败。",
  PRICING_AUDIT_TIME_INVALID: "审计时间必须使用带时区的 RFC3339 格式。",
  PRICING_AUDIT_TIME_RANGE_INVALID: "审计结束时间不能早于开始时间。",
  PRICING_AUDIT_STORE_UNAVAILABLE: "审计服务当前不可用，已保留现有结果，请稍后重试。",
  PRICE_PLAN_VERSION_NOT_ACTIVE: "价格方案必须绑定 ACTIVE 权益版本；请先激活权益版本或新建方案。",
  PRICE_PLAN_VERSION_OUTSIDE_VALIDITY: "价格方案有效期超出权益版本有效期。",
  PRICE_PLAN_COMMISSION_SNAPSHOT_INVALID: "权益版本的分润规则快照无效。",
  PRICE_PLAN_OUTSIDE_VALIDITY: "当前时间不在价格方案有效期内。",
  PRICE_PLAN_BINDING_NOT_ACTIVE: "价格方案没有 ACTIVE 支付绑定。",
  PRICE_PLAN_WECHAT_PRICE_MISMATCH: "价格方案、支付绑定与微信商品价格不一致，请修正后重试。",
  PRICE_PLAN_PAYMENT_ENV_MISMATCH: "价格方案、支付绑定与微信商品的渠道或环境不一致。",
  WECHAT_GOOD_NOT_CONFIRMED: "微信商品尚未人工确认已发布。",
  WECHAT_GOOD_NOT_AVAILABLE: "微信商品已停用或当前不可用。",
  WECHAT_GOOD_VERIFICATION_EXPIRED: "微信商品人工确认已过期，请重新人工核验后确认。",
  PRICE_PLAN_DEFAULT_TEST_FORBIDDEN: "TEST 价格方案不能设置为默认方案。",
  PRICE_PLAN_DEFAULT_NOT_ACTIVE: "只有已启用的 ACTIVE 价格方案可以设为默认。",
  PRICE_PLAN_DEFAULT_HIDDEN: "隐藏价格方案不能设置为默认方案。",
  PRICE_PLAN_DEFAULT_AUDIENCE_INVALID: "只有 PUBLIC 价格方案可以设置为默认方案。",
  PRICE_PLAN_DEFAULT_DISABLE_FORBIDDEN: "默认价格方案不能直接停用，请先切换默认方案。",
  PRICE_PLAN_CONFIGURATION_CHANGED: "价格方案配置已变化，请重新校验后再操作。",
  PRICE_PLAN_DEFAULT_CONFLICT: "默认价格方案已被其他操作切换，请刷新后重试。",
  MANAGED_PLAN_REQUIRES_VERSION: "该套餐已由 V2 权益版本托管，请通过权益版本管理修改。",
  MANAGED_PLAN_REQUIRES_PAYMENT_BINDING: "该 V2 价格方案必须通过支付绑定选择微信商品。",
  PRICE_PLAN_NOT_ELIGIBLE: "当前用户不再具备该测试价格资格，请重新获取报价。",
  PRICE_PLAN_SETTLEMENT_CONFIGURATION_MISMATCH: "当前租户结算配置与该价格方案不兼容，已阻止创建订单。",
  PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE: "赠送积分履约能力尚未接入，当前价格方案不能启用或下单。",
  PLAN_VERSION_NOT_FOUND: "权益版本不存在或已被删除，请刷新列表后重试。",
  PLAN_VERSION_NOT_DRAFT: "只有 DRAFT 权益版本可以编辑或激活；请刷新后确认最新状态。",
  PLAN_VERSION_NOT_ACTIVE: "只有 ACTIVE 权益版本可以退休；请刷新后确认最新状态。",
  INVALID_PLAN_VERSION: "权益版本内容不符合要求，请检查等级、有效期和权益数值。",
  INVALID_PLAN_VERSION_TRANSITION: "权益版本状态流转无效，请刷新后重试。",
  WECHAT_GOOD_CHANNEL_INVALID: "微信商品渠道无效；会员和代理 V2 商品必须使用受支持的微信虚拟支付渠道。",
  PAYMENT_ENVIRONMENT_INVALID: "支付环境无效，请明确选择正式或沙箱环境。",
  WECHAT_GOOD_MODE_INVALID: "微信商品 mode 无效，请按微信后台登记值填写。",
  WECHAT_GOOD_INVALID: "微信商品本地记录不完整，请检查 productId、offerId、价格和环境。",
  WECHAT_GOOD_NOT_FOUND: "微信商品不存在或已不可见，请刷新商品列表。",
  WECHAT_GOOD_ALREADY_EXISTS: "相同渠道、环境、offerId 和 productId 的微信商品已存在。",
  WECHAT_GOOD_HAS_PAYMENT_BINDING: "微信商品已有支付绑定，不能原地修改关键字段；请新建商品后换绑。",
  WECHAT_GOOD_HAS_LIVE_QUOTE: "微信商品仍被有效报价引用，当前操作已阻止。",
  WECHAT_GOOD_VERIFICATION_EXPIRY_INVALID: "人工确认有效期无效，请填写未来的有效时间。",
  WECHAT_GOOD_VERIFICATION_REASON_REQUIRED: "人工确认必须填写独立的核验原因。",
  PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN: "当前绑定支撑默认价格方案，必须先安全切换默认方案。",
  WECHAT_GOOD_REQUIRED: "必须选择一个微信商品本地记录。",
  PAYMENT_BINDING_ALREADY_EXISTS: "该价格方案已存在支付绑定，请刷新后使用换绑或状态操作。",
  PAYMENT_BINDING_NOT_FOUND: "支付绑定不存在，请刷新价格方案后重试。",
  PAYMENT_BINDING_MUTATION_REQUIRED: "没有检测到可保存的支付绑定变更。",
  PAYMENT_BINDING_MUTATION_CONFLICT: "换绑与启停不能在同一次请求中执行。",
  PAYMENT_BINDING_CONFIGURATION_CHANGED: "支付绑定依赖配置已变化，请刷新商品、价格和引用后重试。",
  PAYMENT_BINDING_ACTIVE: "启用中的支付绑定不能直接换绑；请先按安全规则停用。",
  PAYMENT_BINDING_HAS_HISTORY: "支付绑定已有报价或订单历史，不能换绑；请创建新绑定。",
  PRICE_PLAN_NOT_ACTIVE: "价格方案不是 ACTIVE，不能执行该支付绑定操作。",
  PRICE_PLAN_STATE_INVALID: "当前价格方案状态不允许执行该操作；EXPIRED 方案必须克隆为新的 DRAFT。",
  PRICE_PLAN_NOT_MANAGED: "该价格方案不属于本阶段托管的会员或代理 V2 套餐。",
  REASON_REQUIRED: "必须填写变更原因。"
};

export function pricingErrorMessage(error: unknown, fallback = "操作失败，请稍后重试"): string {
  const record = isRecord(error) ? error : {};
  const code = typeof error === "string" ? error.trim() : stringField(record, "code");
  if (pricingErrorMessages[code]) return pricingErrorMessages[code];
  const message = stringField(record, "message") || (typeof error === "string" ? error.trim() : "");
  return /[\u3400-\u9fff]/.test(message) ? message : fallback;
}

export interface PricingAuditSnapshotLimits {
  maxDepth?: number;
  maxItems?: number;
  maxStringLength?: number;
  maxTotalCharacters?: number;
}

export interface PricingAuditSnapshotText {
  text: string;
  redacted: boolean;
  truncated: boolean;
}

const PRICING_AUDIT_SENSITIVE_KEY_MARKERS = [
  "appsecret", "appkey", "clientsecret", "secret", "privatekey", "verificationtoken", "encryptkey",
  "sessionkey", "accesstoken", "authorization", "cookie", "password", "credential", "databaseurl",
  "connectionstring", "dsn", "signature"
];

function isSensitivePricingAuditClientKey(key: string): boolean {
  const normalized = key.toLowerCase().replace(/[^a-z0-9]/g, "");
  return PRICING_AUDIT_SENSITIVE_KEY_MARKERS.some((marker) => normalized.includes(marker));
}

function sanitizePricingAuditClientString(value: string): { value: string; redacted: boolean } {
  const trimmed = value.trim();
  if (/^(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|redis|rediss|sqlserver):\/\//i.test(trimmed)
    || /\b(?:bearer\s+|access[_-]?token\s*[:=]|session[_-]?key\s*[:=]|app[_-]?secret\s*[:=]|password\s*[:=])/i.test(trimmed)) {
    return { value: "[REDACTED]", redacted: true };
  }
  if (/^https?:\/\//i.test(trimmed)) {
    try {
      const parsed = new URL(trimmed);
      const hasSensitiveQuery = [...parsed.searchParams.keys()].some(isSensitivePricingAuditClientKey);
      if (parsed.username || parsed.password || hasSensitiveQuery) {
        parsed.username = "";
        parsed.password = "";
        parsed.search = "";
        parsed.hash = "";
        return { value: parsed.toString(), redacted: true };
      }
    } catch {
      return { value: "[REDACTED]", redacted: true };
    }
  }
  return { value, redacted: false };
}

export function formatPricingAuditSnapshot(
  input: unknown,
  requestedLimits: PricingAuditSnapshotLimits = {}
): PricingAuditSnapshotText {
  const limits = {
    maxDepth: Math.max(1, Math.floor(requestedLimits.maxDepth ?? 8)),
    maxItems: Math.max(1, Math.floor(requestedLimits.maxItems ?? 500)),
    maxStringLength: Math.max(8, Math.floor(requestedLimits.maxStringLength ?? 2_000)),
    maxTotalCharacters: Math.max(64, Math.floor(requestedLimits.maxTotalCharacters ?? 50_000))
  };
  let redacted = false;
  let truncated = false;
  let visitedItems = 0;
  let estimatedCharacters = 0;
  let totalBudgetReached = false;
  const ancestors = new WeakSet<object>();

  const reserveCharacters = (count: number): boolean => {
    if (totalBudgetReached) return false;
    if (estimatedCharacters + Math.max(0, count) > limits.maxTotalCharacters) {
      totalBudgetReached = true;
      truncated = true;
      return false;
    }
    estimatedCharacters += Math.max(0, count);
    return true;
  };

  const visit = (value: unknown, depth: number): unknown => {
    if (totalBudgetReached) return "[TRUNCATED_TOTAL]";
    if (value === null || typeof value === "boolean" || typeof value === "number") {
      return reserveCharacters(String(value).length + 2) ? value : "[TRUNCATED_TOTAL]";
    }
    if (typeof value === "string") {
      const sanitized = sanitizePricingAuditClientString(value);
      if (sanitized.redacted) redacted = true;
      if (sanitized.value.length > limits.maxStringLength) {
        truncated = true;
        const shortened = `${sanitized.value.slice(0, limits.maxStringLength)}…[TRUNCATED_STRING]`;
        return reserveCharacters(shortened.length + 2) ? shortened : "[TRUNCATED_TOTAL]";
      }
      return reserveCharacters(sanitized.value.length + 2) ? sanitized.value : "[TRUNCATED_TOTAL]";
    }
    if (typeof value === "bigint") {
      const rendered = value.toString();
      return reserveCharacters(rendered.length + 2) ? rendered : "[TRUNCATED_TOTAL]";
    }
    if (typeof value !== "object") {
      const rendered = String(value ?? "");
      return reserveCharacters(rendered.length + 2) ? rendered : "[TRUNCATED_TOTAL]";
    }
    if (depth >= limits.maxDepth) {
      truncated = true;
      return "[TRUNCATED_DEPTH]";
    }
    if (ancestors.has(value)) {
      truncated = true;
      return "[TRUNCATED_CYCLE]";
    }
    ancestors.add(value);
    try {
      if (Array.isArray(value)) {
        const result: unknown[] = [];
        for (let index = 0; index < value.length; index += 1) {
          if (totalBudgetReached) {
            result.push("[TRUNCATED_TOTAL]");
            break;
          }
          if (visitedItems >= limits.maxItems) {
            truncated = true;
            result.push("[TRUNCATED_ITEMS]");
            break;
          }
          if (!reserveCharacters(3)) {
            result.push("[TRUNCATED_TOTAL]");
            break;
          }
          visitedItems += 1;
          result.push(visit(value[index], depth + 1));
        }
        return result;
      }
      const result: Record<string, unknown> = {};
      const record = value as Record<string, unknown>;
      for (const key in record) {
        if (!Object.prototype.hasOwnProperty.call(record, key)) continue;
        if (totalBudgetReached) {
          result["[TRUNCATED_TOTAL]"] = "remaining fields omitted after reaching the character budget";
          break;
        }
        if (visitedItems >= limits.maxItems) {
          truncated = true;
          result["[TRUNCATED_ITEMS]"] = `remaining fields omitted after ${limits.maxItems} items`;
          break;
        }
        if (!reserveCharacters(key.length + 6)) {
          result["[TRUNCATED_TOTAL]"] = "remaining fields omitted after reaching the character budget";
          break;
        }
        visitedItems += 1;
        if (isSensitivePricingAuditClientKey(key)) {
          redacted = true;
          result[key] = "[REDACTED]";
        } else {
          result[key] = visit(record[key], depth + 1);
        }
      }
      return result;
    } finally {
      ancestors.delete(value);
    }
  };

  const sanitized = visit(input, 0);
  let text = JSON.stringify(sanitized, null, 2) ?? "null";
  if (text.length > limits.maxTotalCharacters) {
    truncated = true;
    const marker = "\n[TRUNCATED_TOTAL]";
    text = `${text.slice(0, Math.max(0, limits.maxTotalCharacters - marker.length))}${marker}`;
  }
  return { text, redacted, truncated };
}

export interface EntitlementVersionActionInput {
  status?: string;
}

export type PlanVersionListDisplayState = "LOADING" | "ERROR" | "EMPTY" | "LIST";

export function planVersionListDisplayState(input: {
  loading: boolean;
  loaded: boolean;
  error?: string;
  versionCount: number;
}): PlanVersionListDisplayState {
  if (input.versionCount > 0) return "LIST";
  if (input.loading) return "LOADING";
  if (String(input.error || "").trim()) return "ERROR";
  return input.loaded ? "EMPTY" : "LOADING";
}

export function entitlementVersionActions(version: EntitlementVersionActionInput) {
  const status = String(version.status || "").toUpperCase();
  return {
    canEdit: status === "DRAFT",
    canActivate: status === "DRAFT",
    canRetire: status === "ACTIVE",
    canClone: ["DRAFT", "ACTIVE", "RETIRED"].includes(status),
    cloneRequired: status === "ACTIVE" || status === "RETIRED"
  };
}

export function entitlementVersionUIActions(version: EntitlementVersionActionInput, principal: PricingPrincipal) {
  const actions = entitlementVersionActions(version);
  const canManage = hasPricingPermission(principal, "pricing:entitlement:manage");
  return {
    readOnly: !canManage || !actions.canEdit,
    canEdit: canManage && actions.canEdit,
    canActivate: canManage && actions.canActivate,
    canRetire: canManage && actions.canRetire,
    canClone: canManage && actions.canClone,
    cloneRequired: actions.cloneRequired
  };
}

export interface EntitlementVersionDraft {
  memberLevel?: string;
  agentLevel?: string;
  tokenAmount: number;
  pointsAmount: number;
  durationDays: number;
  rightsSnapshot: Record<string, unknown>;
  commissionRuleVersion: string;
  commissionSnapshot: Record<string, unknown>;
  effectiveAt?: string;
  expiresAt?: string;
  changeReason: string;
}

export function cloneEntitlementVersionDraft(source: Record<string, unknown>): EntitlementVersionDraft {
  const cloneRecord = (value: unknown): Record<string, unknown> => {
    if (!isRecord(value)) return {};
    return JSON.parse(JSON.stringify(value)) as Record<string, unknown>;
  };
  const draft: EntitlementVersionDraft = {
    tokenAmount: Number(source.tokenAmount || 0),
    pointsAmount: Number(source.pointsAmount || 0),
    durationDays: Number(source.durationDays || 0),
    rightsSnapshot: cloneRecord(source.rightsSnapshot),
    commissionRuleVersion: String(source.commissionRuleVersion || ""),
    commissionSnapshot: cloneRecord(source.commissionSnapshot),
    changeReason: ""
  };
  if (typeof source.memberLevel === "string") draft.memberLevel = source.memberLevel;
  if (typeof source.agentLevel === "string") draft.agentLevel = source.agentLevel;
  if (typeof source.effectiveAt === "string" && source.effectiveAt) draft.effectiveAt = source.effectiveAt;
  if (typeof source.expiresAt === "string" && source.expiresAt) draft.expiresAt = source.expiresAt;
  return draft;
}

export function entitlementVersionTransitionPreview(
  target: { id?: string; versionNo?: number; status?: string },
  versions: Array<{ id?: string; versionNo?: number; status?: string }>
) {
  const targetStatus = normalizedUpper(target.status);
  const active = versions.find((version) => normalizedUpper(version.status) === "ACTIVE");
  return {
    action: targetStatus === "DRAFT" ? "ACTIVATE" : targetStatus === "ACTIVE" ? "RETIRE" : "NONE",
    currentActiveVersionId: String(active?.id || ""),
    currentActiveVersionNo: Number(active?.versionNo || 0),
    willRetireCurrentActive: targetStatus === "DRAFT" && Boolean(active?.id) && active?.id !== target.id,
    mayLeaveNoActive: targetStatus === "ACTIVE"
  };
}

export function parseEntitlementJSONObject(value: string, label = "JSON"): Record<string, unknown> {
  try {
    const parsed = JSON.parse(String(value || "")) as unknown;
    if (!isRecord(parsed)) throw new Error();
    return parsed;
  } catch {
    throw new Error(`${label}必须是 JSON 对象`);
  }
}

export interface PricePlanActionInput {
  status?: string;
  kind?: string;
  audienceType?: string;
  isVisible?: boolean;
  isEnabled?: boolean;
  isDefault?: boolean;
  economicFieldsLocked?: boolean;
  giftPoints?: number;
  healthAvailable?: boolean;
}

export interface PricePlanActionContext {
  validationValid?: boolean;
  v132Blocked?: boolean;
}

export function pricePlanActions(plan: PricePlanActionInput, context: PricePlanActionContext = {}) {
  const status = String(plan.status || "").toUpperCase();
  const kind = String(plan.kind || "").toUpperCase();
  const audienceType = String(plan.audienceType || "").toUpperCase();
  const isDraft = status === "DRAFT";
  const hasGiftPointsBlock = Number(plan.giftPoints || 0) > 0;
  const testConfigurationValid = kind !== "TEST" || isValidTestPricePlanConfiguration(plan);
  const configurationValid = context.validationValid === true && !context.v132Blocked && !hasGiftPointsBlock && testConfigurationValid;
  return {
    canEdit: isDraft,
    canEditEconomicFields: isDraft && plan.economicFieldsLocked !== true,
    mustCloneForEconomicChange: !isDraft || plan.economicFieldsLocked === true,
    canClone: ["DRAFT", "ACTIVE", "INACTIVE", "DISABLED", "RETIRED"].includes(status),
    canValidate: true,
    canEnable: !plan.isEnabled && configurationValid,
    canDisable: plan.isEnabled === true && plan.isDefault !== true,
    canMakeDefault: kind !== "TEST"
      && status === "ACTIVE"
      && plan.isEnabled === true
      && plan.isVisible === true
      && audienceType === "PUBLIC"
      && plan.isDefault !== true
      && configurationValid,
    canManageWhitelist: kind === "TEST"
  };
}

export interface PricePlanUIActionContext extends PricePlanActionContext {
  validationFresh?: boolean;
  runtimeSafetyKnown?: boolean;
  paymentDataComplete?: boolean;
  hasActiveBinding?: boolean;
}

export function pricePlanUIActions(
  plan: PricePlanActionInput,
  context: PricePlanUIActionContext,
  principal: PricingPrincipal
) {
  const status = normalizedUpper(plan.status);
  const kind = normalizedUpper(plan.kind);
  const audienceType = normalizedUpper(plan.audienceType);
  const canManage = hasPricingPermission(principal, "pricing:price-plan:manage");
  const canSwitchDefault = hasPricingPermission(principal, "pricing:price-plan:default");
  const isDraft = status === "DRAFT";
  const enableLifecycleValid = status === "DRAFT" || status === "INACTIVE";
  const lifecycleSupportsMetadata = ["DRAFT", "ACTIVE", "INACTIVE", "EXPIRED"].includes(status);
  const activeBindingBlocksEconomicEdit = isDraft && context.hasActiveBinding === true;
  const lockedEconomicFields = !isDraft || plan.economicFieldsLocked === true || activeBindingBlocksEconomicEdit;
  const giftPointsSafe = Number.isSafeInteger(plan.giftPoints) && plan.giftPoints === 0;
  const planSafetyFieldsKnown = ["NORMAL", "PROMOTION", "TEST"].includes(kind)
    && Boolean(audienceType)
    && typeof plan.isVisible === "boolean"
    && typeof plan.isEnabled === "boolean"
    && typeof plan.isDefault === "boolean";
  const testConfigurationValid = kind !== "TEST" || isValidTestPricePlanConfiguration(plan);
  const decisionDataReady = context.validationFresh === true
    && context.validationValid === true
    && context.runtimeSafetyKnown === true
    && context.paymentDataComplete === true
    && context.v132Blocked === false
    && giftPointsSafe
    && plan.healthAvailable === true
    && planSafetyFieldsKnown
    && testConfigurationValid;
  const canCloneLifecycle = ["DRAFT", "ACTIVE", "INACTIVE", "EXPIRED"].includes(status);
  return {
    canEditMetadata: canManage && lifecycleSupportsMetadata,
    canEditEconomicFields: canManage && isDraft && !lockedEconomicFields,
    mustCloneForEconomicChange: !isDraft || plan.economicFieldsLocked === true,
    economicBlocker: activeBindingBlocksEconomicEdit
      ? "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE"
      : (!isDraft || plan.economicFieldsLocked === true ? "PRICE_PLAN_CLONE_REQUIRED" : ""),
    canClone: canManage && canCloneLifecycle,
    canValidate: canManage,
    canEnable: canManage && enableLifecycleValid && plan.isEnabled === false && decisionDataReady,
    canDisable: canManage && plan.isEnabled === true && plan.isDefault !== true,
    canMakeDefault: canSwitchDefault
      && kind !== "TEST"
      && status === "ACTIVE"
      && plan.isEnabled === true
      && plan.isVisible === true
      && audienceType === "PUBLIC"
      && plan.isDefault !== true
      && decisionDataReady,
    canManageWhitelist: kind === "TEST" && testConfigurationValid && hasPricingPermission(principal, "pricing:test-whitelist:manage")
  };
}

export function pricePlanBadges(plan: Pick<PricePlanActionInput, "kind" | "isVisible" | "isDefault" | "audienceType">): string[] {
  if (normalizedUpper(plan.kind) !== "TEST") return [];
  const badges = ["测试"];
  badges.push(plan.isVisible === false ? "隐藏" : "配置异常：TEST 不得公开");
  badges.push(plan.isDefault === false ? "非默认" : "配置异常：TEST 不得默认");
  badges.push(["TEST", "WHITELIST"].includes(normalizedUpper(plan.audienceType))
    ? "白名单限定"
    : "配置异常：TEST 受众范围无效");
  return badges;
}

function isValidTestPricePlanConfiguration(plan: Pick<PricePlanActionInput, "isVisible" | "isDefault" | "audienceType">) {
  return plan.isVisible === false
    && plan.isDefault === false
    && ["TEST", "WHITELIST"].includes(normalizedUpper(plan.audienceType));
}

export function activeEntitlementVersionOptions(
  versions: Array<{ id?: string; planId?: string; versionNo?: number; status?: string; revision?: number }>,
  planId: string
) {
  return versions
    .filter((version) => String(version.planId || "") === String(planId || "") && normalizedUpper(version.status) === "ACTIVE")
    .map((version) => ({ id: String(version.id || ""), versionNo: Number(version.versionNo || 0), revision: Number(version.revision || 0) }));
}

export function pricePlanEditorEconomicFieldsEditable(
  mode: "CREATE" | "EDIT" | "CLONE",
  actions: { canEditEconomicFields?: boolean }
) {
  if (mode === "CREATE") return true;
  if (mode === "CLONE") return false;
  return actions.canEditEconomicFields === true;
}

const pricePlanCodeFormat = /^[a-z][a-z0-9_]{1,62}[a-z0-9]$/;
const explicitPriceWord = /(^|_)(price|rmb|amount|yuan)(_|$)|(^|_)[0-9]+yuan(_|$)/;
const pricedIdentity = /^(plan_)?(member|agent)_[1-9][0-9]{1,5}$/;
const adjacentPriceSemantic = /(^|_)((price|rmb|amount|yuan)[0-9]+|[0-9]+(price|rmb|amount|yuan))(_|$)/;

export function pricePlanCodeIssue(value: string): string {
  const code = String(value || "").trim();
  if (!pricePlanCodeFormat.test(code)) return "PRICE_PLAN_CODE_FORMAT_INVALID";
  if (explicitPriceWord.test(code) || pricedIdentity.test(code) || adjacentPriceSemantic.test(code)) return "PRICE_PLAN_CODE_HAS_PRICE";
  return "";
}

export function pricePlanNameIssue(value: unknown): string {
  return typeof value === "string" && value.trim() ? "" : "PRICE_PLAN_NAME_REQUIRED";
}

export function formatPriceCents(value: unknown, prefix = "¥"): string {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) return "未知";
  return `${prefix}${(value / 100).toFixed(2)}`;
}

export interface MergePricePlanRowsInput {
  plans: PricePlan[];
  healthPlans?: PricingHealthPricePlan[];
  validationsByPricePlanId?: Record<string, PricePlanValidation>;
  freshValidationIds?: ReadonlySet<string>;
  bindingsByPricePlanId?: Record<string, PricePlanPaymentBinding[]>;
  goodsById?: Record<string, WechatVirtualGood>;
}

export interface PricePlanListRow extends PricePlan {
  healthStatus: PricingHealthStatus | "UNKNOWN";
  healthIssueCodes: string[];
  healthAvailable: boolean;
  quoteCount: number | null;
  orderCount: number | null;
  validation: PricePlanValidation | null;
  validationFresh: boolean;
  paymentBinding?: PricePlanPaymentBinding;
  wechatGood?: WechatVirtualGood;
  paymentDataComplete: boolean;
}

interface ValidatedPaymentIdentityInput {
  plan: Record<string, unknown>;
  validation?: Record<string, unknown>;
  binding?: Record<string, unknown>;
  good?: Record<string, unknown>;
}

export function validatedPaymentIdentityState(input: ValidatedPaymentIdentityInput): { valid: boolean; blockers: string[] } {
  const { plan, validation, binding, good } = input;
  if (!validation || !binding || !good) return { valid: false, blockers: ["MANAGED_PLAN_REQUIRES_PAYMENT_BINDING"] };
  const planID = String(plan.pricePlanId || "");
  const bindingID = String(binding.id || "");
  const goodID = String(good.id || "");
  const productID = String(good.productId || "");
  const exactIdentity = Boolean(planID && bindingID && goodID && productID)
    && String(validation.pricePlanId || "") === planID
    && String(validation.paymentBindingId || "") === bindingID
    && String(binding.pricePlanId || "") === planID
    && String(validation.wechatGoodId || "") === goodID
    && String(binding.wechatGoodId || "") === goodID
    && String(validation.wechatProductId || "") === productID
    && String(binding.wechatProductId || "") === productID;
  const exactSnapshots = Number.isSafeInteger(validation.pricePlanPriceCents)
    && Number.isSafeInteger(validation.bindingPriceCents)
    && Number.isSafeInteger(validation.wechatGoodPriceCents)
    && Number.isSafeInteger(binding.pricePlanSalePriceCents)
    && Number.isSafeInteger(binding.wechatGoodPriceCents)
    && Number(validation.pricePlanPriceCents) === Number(plan.salePriceCents)
    && Number(validation.bindingPriceCents) === Number(binding.providerPriceSnapshotCents)
    && Number(validation.wechatGoodPriceCents) === Number(good.platformPriceCents)
    && Number(binding.pricePlanSalePriceCents) === Number(plan.salePriceCents)
    && Number(binding.wechatGoodPriceCents) === Number(good.platformPriceCents);
  return exactIdentity && exactSnapshots
    ? { valid: true, blockers: [] }
    : { valid: false, blockers: ["PRICE_PLAN_CONFIGURATION_CHANGED"] };
}

export function mergePricePlanRows(input: MergePricePlanRowsInput): PricePlanListRow[] {
  const healthById = new Map((input.healthPlans || []).map((item) => [String(item.pricePlanId || ""), item]));
  return (input.plans || []).map((plan) => {
    const pricePlanId = String(plan.pricePlanId || "");
    const health = healthById.get(pricePlanId);
    const validation = input.validationsByPricePlanId?.[pricePlanId];
    const bindings = input.bindingsByPricePlanId?.[pricePlanId] || [];
    const requestedBindingId = String(validation ? validation.paymentBindingId || "" : health?.paymentBindingId || "");
    const paymentBinding = requestedBindingId
      ? bindings.find((binding) => binding.id === requestedBindingId)
      : undefined;
    const requestedGoodId = String(validation ? validation.wechatGoodId || "" : health?.wechatGoodId || paymentBinding?.wechatGoodId || "");
    const wechatGood = paymentBinding && requestedGoodId && paymentBinding.wechatGoodId === requestedGoodId
      ? input.goodsById?.[requestedGoodId]
      : undefined;
    const identity = validatedPaymentIdentityState({
      plan: plan as unknown as Record<string, unknown>,
      validation: validation as unknown as Record<string, unknown> | undefined,
      binding: paymentBinding as unknown as Record<string, unknown> | undefined,
      good: wechatGood as unknown as Record<string, unknown> | undefined
    });
    return {
      ...plan,
      healthStatus: health ? health.status : "UNKNOWN" as const,
      healthIssueCodes: health && Array.isArray(health.issueCodes) ? [...health.issueCodes] : [],
      healthAvailable: Boolean(health),
      quoteCount: health && Number.isFinite(Number(health.quoteCount)) ? Number(health.quoteCount) : null,
      orderCount: health && Number.isFinite(Number(health.orderCount)) ? Number(health.orderCount) : null,
      validation: validation || null,
      validationFresh: Boolean(validation && input.freshValidationIds?.has(pricePlanId)),
      paymentBinding,
      wechatGood,
      paymentDataComplete: identity.valid
    };
  });
}

export function pricePlanListDisplayState(input: { loading: boolean; loaded: boolean; error?: string; rowCount: number }) {
  if (input.rowCount > 0) return "LIST" as const;
  if (input.loading) return "LOADING" as const;
  if (String(input.error || "").trim()) return "ERROR" as const;
  return input.loaded ? "EMPTY" as const : "LOADING" as const;
}

export interface DefaultPricePlanPreviewInput {
  target: Record<string, unknown>;
  plans: Array<Record<string, unknown>>;
  validation?: Record<string, unknown>;
  binding?: Record<string, unknown>;
  good?: Record<string, unknown>;
  currentDefaultValidation?: Record<string, unknown>;
  currentDefaultBinding?: Record<string, unknown>;
  currentDefaultGood?: Record<string, unknown>;
  currentDefaultValidationFresh?: boolean;
  validationFresh: boolean;
  runtimeSafetyKnown: boolean;
  v132Blocked?: boolean;
  targetHealthAvailable?: boolean;
}

export function buildDefaultPricePlanPreview(input: DefaultPricePlanPreviewInput) {
  const groupMatches = (candidate: Record<string, unknown>) => ["planId", "channel", "environment", "currency"]
    .every((field) => String(candidate[field] || "") === String(input.target[field] || ""));
  const currentDefault = input.plans.find((candidate) => candidate.isDefault === true && groupMatches(candidate)) || null;
  const blockers: string[] = [];
  const warnings: string[] = [];
  if (normalizedUpper(input.target.kind) === "TEST") blockers.push("PRICE_PLAN_DEFAULT_TEST_FORBIDDEN");
  if (normalizedUpper(input.target.status) !== "ACTIVE" || input.target.isEnabled !== true) blockers.push("PRICE_PLAN_DEFAULT_NOT_ACTIVE");
  if (input.target.isVisible !== true) blockers.push("PRICE_PLAN_DEFAULT_HIDDEN");
  if (normalizedUpper(input.target.audienceType) !== "PUBLIC") blockers.push("PRICE_PLAN_DEFAULT_AUDIENCE_INVALID");
  if (!input.validationFresh) blockers.push("VALIDATION_NOT_FRESH");
  if (!input.runtimeSafetyKnown || input.v132Blocked === undefined) blockers.push("RUNTIME_SAFETY_UNKNOWN");
  if (input.v132Blocked === true) blockers.push("V132_BLOCKED");
  if (input.targetHealthAvailable !== true) blockers.push("PRICE_PLAN_HEALTH_UNKNOWN");
  if (!Number.isSafeInteger(input.target.giftPoints) || input.target.giftPoints !== 0) {
    blockers.push("PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE");
  }
  if (!input.validation || input.validation.valid !== true) {
    blockers.push(...(Array.isArray(input.validation?.checks)
      ? input.validation.checks.filter((check) => isRecord(check) && check.passed !== true).map((check) => String(check.code || "")).filter(Boolean)
      : ["PRICE_PLAN_CONFIGURATION_CHANGED"]));
  }
  const targetIdentity = validatedPaymentIdentityState({ plan: input.target, validation: input.validation, binding: input.binding, good: input.good });
  blockers.push(...targetIdentity.blockers);
  if (targetIdentity.valid) blockers.push(...paymentValidationState(input.target, input.binding, input.good).blockers);
  if (currentDefault) {
    if (input.currentDefaultValidationFresh !== true) blockers.push("CURRENT_DEFAULT_VALIDATION_NOT_FRESH");
    const currentIdentity = validatedPaymentIdentityState({
      plan: currentDefault,
      validation: input.currentDefaultValidation,
      binding: input.currentDefaultBinding,
      good: input.currentDefaultGood
    });
    blockers.push(...currentIdentity.blockers);
    if (input.currentDefaultValidation && input.currentDefaultValidation.valid !== true) warnings.push("CURRENT_DEFAULT_CONFIGURATION_INVALID");
  }
  const uniqueBlockers = [...new Set(blockers)];
  return {
    target: input.target,
    currentDefault,
    validation: input.validation || null,
    binding: input.binding || null,
    good: input.good || null,
    currentDefaultValidation: input.currentDefaultValidation || null,
    currentDefaultBinding: input.currentDefaultBinding || null,
    currentDefaultGood: input.currentDefaultGood || null,
    blockers: uniqueBlockers,
    warnings: [...new Set(warnings)],
    ready: uniqueBlockers.length === 0
  };
}

export function pricePlanDisplayFacts(row: Record<string, unknown>) {
  const good = isRecord(row.wechatGood) ? row.wechatGood : {};
  return {
    kind: String(row.kind || ""),
    planVersionId: String(row.planVersionId || ""),
    audienceType: String(row.audienceType || ""),
    isVisible: typeof row.isVisible === "boolean" ? row.isVisible : null,
    isEnabled: typeof row.isEnabled === "boolean" ? row.isEnabled : null,
    isDefault: typeof row.isDefault === "boolean" ? row.isDefault : null,
    validFrom: String(row.validFrom || ""),
    validUntil: String(row.validUntil || ""),
    wechatProductId: String(good.productId || ""),
    wechatGoodPriceCents: Number.isSafeInteger(good.platformPriceCents) ? Number(good.platformPriceCents) : null,
    revision: Number.isSafeInteger(row.revision) ? Number(row.revision) : null
  };
}

export function pricingHealthPricePlanFact(health: unknown, pricePlanId: string): { available: boolean; status: string } {
  const pricePlans = isRecord(health) && Array.isArray(health.pricePlans) ? health.pricePlans : [];
  const item = pricePlans.find((candidate) => isRecord(candidate) && String(candidate.pricePlanId || "") === String(pricePlanId || ""));
  if (!isRecord(item)) return { available: false, status: "UNKNOWN" };
  const status = normalizedUpper(item.status);
  return { available: true, status: status || "UNKNOWN" };
}

export function defaultSwitchRefreshGate(results: ReadonlyArray<{ status: string }>) {
  const complete = results.length > 0 && results.every((result) => result.status === "fulfilled");
  return { complete, validationFresh: complete, runtimeSafetyKnown: complete };
}

export function defaultSwitchCanSubmit(input: {
  permission: boolean;
  previewReady: boolean;
  secondConfirmed: boolean;
  hasReason: boolean;
  loading: boolean;
  refreshComplete: boolean;
  loadError?: string;
  committedStale?: boolean;
}) {
  return input.permission
    && input.previewReady
    && input.secondConfirmed
    && input.hasReason
    && !input.loading
    && input.refreshComplete
    && input.committedStale !== true
    && !String(input.loadError || "").trim();
}

export function pricePlanMutationSubmitAllowed(input: { allowedByPolicy: boolean; saving: boolean; committedStale: boolean }) {
  return input.allowedByPolicy && !input.saving && !input.committedStale;
}

export function pricingInspectionState(input: { loading: boolean; fresh: boolean; error?: string; hasCached: boolean }) {
  if (input.loading) return "LOADING" as const;
  if (input.fresh && !String(input.error || "").trim()) return "FRESH" as const;
  if (String(input.error || "").trim()) return input.hasCached ? "STALE_ERROR" as const : "ERROR" as const;
  return input.hasCached ? "STALE" as const : "ERROR" as const;
}

export interface PaymentValidationPricePlan {
  salePriceCents?: number;
  channel?: string;
  environment?: string;
}

export interface PaymentValidationBinding {
  providerPriceSnapshotCents?: number;
  channel?: string;
  environment?: string;
  enabled?: boolean;
  status?: string;
}

export interface PaymentValidationGood {
  platformPriceCents?: number;
  channel?: string;
  environment?: string;
  enabled?: boolean;
  verificationStatus?: string;
}

export function paymentValidationState(
  plan: PaymentValidationPricePlan,
  binding?: PaymentValidationBinding,
  good?: PaymentValidationGood
): { valid: boolean; blockers: string[] } {
  if (!binding || !good) {
    return { valid: false, blockers: ["MANAGED_PLAN_REQUIRES_PAYMENT_BINDING"] };
  }
  const blockers: string[] = [];
  const prices = [Number(plan.salePriceCents), Number(binding.providerPriceSnapshotCents), Number(good.platformPriceCents)];
  if (!prices.every((value) => Number.isSafeInteger(value) && value > 0) || new Set(prices).size !== 1) {
    blockers.push("PRICE_PLAN_WECHAT_PRICE_MISMATCH");
  }
  const channels = [plan.channel, binding.channel, good.channel].map(normalizedUpper);
  const environments = [plan.environment, binding.environment, good.environment].map(normalizedUpper);
  if (channels.some((value) => !value) || environments.some((value) => !value)
    || new Set(channels).size !== 1 || new Set(environments).size !== 1) {
    blockers.push("PRICE_PLAN_PAYMENT_ENV_MISMATCH");
  }
  const verification = normalizedUpper(good.verificationStatus);
  if (verification === "VERIFICATION_EXPIRED") blockers.push("WECHAT_GOOD_VERIFICATION_EXPIRED");
  else if (verification !== "MANUALLY_CONFIRMED_PUBLISHED" || good.enabled !== true) blockers.push("WECHAT_GOOD_NOT_CONFIRMED");
  if (binding.enabled !== true || normalizedUpper(binding.status) !== "ACTIVE") blockers.push("PAYMENT_BINDING_INACTIVE");
  return { valid: blockers.length === 0, blockers };
}

const healthStatusLabels: Record<string, string> = {
  HEALTHY: "正常",
  DEGRADED: "需关注",
  BLOCKED: "已阻断",
  DISABLED: "已停用"
};

const healthIssueLabels: Record<string, string> = {
  ENTITLEMENT_VERSION_MISSING: "未配置 ACTIVE 权益版本",
  PRICE_PLAN_MISSING: "未配置价格方案",
  DEFAULT_PRICE_PLAN_MISSING: "无默认价格方案",
  WECHAT_GOOD_NOT_CONFIRMED: "微信商品未人工确认发布",
  WECHAT_GOOD_VERIFICATION_EXPIRED: "微信商品人工确认已过期",
  PAYMENT_BINDING_MISSING: "未绑定微信商品",
  MANAGED_PLAN_REQUIRES_PAYMENT_BINDING: "未绑定微信商品",
  PRICE_PLAN_WECHAT_PRICE_MISMATCH: "价格方案、绑定快照与微信商品价格不一致",
  PRICE_PLAN_PAYMENT_ENV_MISMATCH: "支付渠道或环境不一致",
  TEST_WHITELIST_MISSING: "TEST 价格方案无有效白名单",
  V132_BLOCKED: "V132 价值守恒适配未完成",
  PRICE_PLAN_SETTLEMENT_CONFIGURATION_MISMATCH: "结算配置不兼容",
  PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE: "赠送积分履约未接入",
  PRICE_PLAN_HEALTH_UNKNOWN: "缺少该价格方案的健康检查结果",
  DISABLED: "配置已停用"
};

export function healthStatusLabel(status: string): string {
  return healthStatusLabels[normalizedUpper(status)] || status || "未检查";
}

export function healthIssueLabel(code: string): string {
  return healthIssueLabels[String(code || "").trim()] || String(code || "未识别问题");
}

export function whitelistEntryActions(entry: { status?: string }) {
  const status = normalizedUpper(entry.status);
  const terminal = status === "EXPIRED" || status === "DISABLED";
  return {
    canEdit: !terminal && (status === "PENDING" || status === "ACTIVE"),
    canDisable: !terminal && (status === "PENDING" || status === "ACTIVE"),
    requiresNewEntry: terminal
  };
}

export function selectableWhitelistPricePlans(plans: readonly PricePlan[], planId: string): PricePlan[] {
  const normalizedPlanId = String(planId || "").trim();
  return plans.filter((plan) => String(plan.planId || "").trim() === normalizedPlanId && normalizedUpper(plan.kind) === "TEST");
}

export function whitelistEntryUIActions(entry: { status?: string }, principal: PricingPrincipal) {
  const lifecycle = whitelistEntryActions(entry);
  const canView = hasPricingPermission(principal, "pricing:plan:view");
  const canManage = canView && hasPricingPermission(principal, "pricing:test-whitelist:manage");
  return {
    canView,
    canManage,
    canEdit: canManage && lifecycle.canEdit,
    canDisable: canManage && lifecycle.canDisable,
    requiresNewEntry: lifecycle.requiresNewEntry
  };
}

const RFC3339_WITH_OFFSET = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;

export function whitelistValidityIssue(input: { validFrom?: string; validUntil?: string }): string {
  const validFrom = String(input.validFrom || "").trim();
  const validUntil = String(input.validUntil || "").trim();
  for (const value of [validFrom, validUntil].filter(Boolean)) {
    if (!RFC3339_WITH_OFFSET.test(value) || !Number.isFinite(Date.parse(value))) return "WHITELIST_RFC3339_OFFSET_REQUIRED";
  }
  if (validFrom && validUntil && Date.parse(validUntil) <= Date.parse(validFrom)) return "WHITELIST_VALIDITY_INVALID";
  return "";
}

export function whitelistMutationErrorState(error: unknown) {
  const record = isRecord(error) ? error : {};
  const code = typeof error === "string" ? error.trim() : stringField(record, "code");
  return {
    revisionConflict: code === "REVISION_CONFLICT",
    preserveForm: true,
    message: pricingErrorMessage(error)
  };
}

export function whitelistDisableResultMessage(response: { alreadyDisabled?: boolean }): string {
  return response.alreadyDisabled === true
    ? "白名单此前已停用，本次为幂等成功，未产生新的状态变更"
    : "白名单已停用";
}

const WHITELIST_REFRESH_GATE_PREFIX = "whitelistRefreshGate:";

export function whitelistRefreshGateKey(pricePlanId: string): string {
  return `${WHITELIST_REFRESH_GATE_PREFIX}${String(pricePlanId || "").trim()}`;
}

export function isWhitelistRefreshGateKey(key: string): boolean {
  return String(key || "").startsWith(WHITELIST_REFRESH_GATE_PREFIX);
}

export type WhitelistEditableField = "reason" | "validFrom" | "validUntil";

export interface WhitelistEditableValues {
  reason: string;
  validFrom: string;
  validUntil: string;
}

export interface WhitelistFieldConflict {
  field: WhitelistEditableField;
  original: string;
  local: string;
  remote: string;
}

export interface WhitelistRebaseResult {
  form: WhitelistEditableValues;
  baseline: WhitelistEditableValues;
  dirtyFields: WhitelistEditableField[];
  conflictingFields: WhitelistEditableField[];
  conflicts: WhitelistFieldConflict[];
}

const whitelistEditableFields: WhitelistEditableField[] = ["reason", "validFrom", "validUntil"];

function normalizedWhitelistEditableValues(input: Partial<WhitelistEditableValues>): WhitelistEditableValues {
  return {
    reason: String(input.reason || "").trim(),
    validFrom: String(input.validFrom || "").trim(),
    validUntil: String(input.validUntil || "").trim()
  };
}

export function rebaseWhitelistEditableFields(input: {
  original: Partial<WhitelistEditableValues>;
  local: Partial<WhitelistEditableValues>;
  latest: Partial<WhitelistEditableValues>;
}): WhitelistRebaseResult {
  const original = normalizedWhitelistEditableValues(input.original);
  const local = normalizedWhitelistEditableValues(input.local);
  const latest = normalizedWhitelistEditableValues(input.latest);
  const form = { ...latest };
  const conflicts: WhitelistFieldConflict[] = [];

  for (const field of whitelistEditableFields) {
    const localDirty = local[field] !== original[field];
    const remoteDirty = latest[field] !== original[field];
    if (localDirty) form[field] = local[field];
    if (localDirty && remoteDirty && local[field] !== latest[field]) {
      conflicts.push({ field, original: original[field], local: local[field], remote: latest[field] });
    }
  }

  const dirtyFields = whitelistEditableFields.filter((field) => form[field] !== latest[field]);
  return {
    form,
    baseline: latest,
    dirtyFields,
    conflictingFields: conflicts.map((conflict) => conflict.field),
    conflicts
  };
}

export function resolveWhitelistFieldConflict(
  input: Pick<WhitelistRebaseResult, "form" | "baseline" | "conflicts">,
  field: WhitelistEditableField,
  resolution: "SERVER" | "LOCAL"
): WhitelistRebaseResult {
  const baseline = normalizedWhitelistEditableValues(input.baseline);
  const form = normalizedWhitelistEditableValues(input.form);
  const conflict = input.conflicts.find((item) => item.field === field);
  if (conflict && resolution === "SERVER") form[field] = conflict.remote;
  const conflicts = input.conflicts.filter((item) => item.field !== field);
  const dirtyFields = whitelistEditableFields.filter((candidate) => form[candidate] !== baseline[candidate]);
  return {
    form,
    baseline,
    dirtyFields,
    conflictingFields: conflicts.map((item) => item.field),
    conflicts
  };
}

export function buildWhitelistUpdateFromBaseline(input: {
  revision: number;
  changeReason: string;
  baseline: Partial<WhitelistEditableValues>;
  current: Partial<WhitelistEditableValues>;
}): WhitelistUpdateInput {
  const baseline = normalizedWhitelistEditableValues(input.baseline);
  const current = normalizedWhitelistEditableValues(input.current);
  const payload: WhitelistUpdateInput = {
    revision: input.revision,
    changeReason: String(input.changeReason || "").trim()
  };
  if (current.reason !== baseline.reason) payload.reason = current.reason;
  if (current.validFrom !== baseline.validFrom) {
    if (current.validFrom) payload.validFrom = current.validFrom;
    else payload.clearValidFrom = true;
  }
  if (current.validUntil !== baseline.validUntil) {
    if (current.validUntil) payload.validUntil = current.validUntil;
    else payload.clearValidUntil = true;
  }
  return payload;
}

export interface LatestRequestGate {
  begin(): number;
  isLatest(token: number): boolean;
  invalidate(): void;
}

export function createLatestRequestGate(): LatestRequestGate {
  let sequence = 0;
  return {
    begin() {
      sequence += 1;
      return sequence;
    },
    isLatest(token: number) {
      return token === sequence;
    },
    invalidate() {
      sequence += 1;
    }
  };
}

export interface WechatGoodListRow extends WechatVirtualGood {
  referenceCount: number | null;
  healthAvailable: boolean;
}

export function mergeWechatGoodRows(input: {
  goods?: readonly WechatVirtualGood[];
  healthGoods?: readonly PricingHealthWechatGood[];
}): WechatGoodListRow[] {
  const healthByGoodId = new Map((input.healthGoods || []).map((item) => [String(item.wechatGoodId), item]));
  return (input.goods || []).map((good) => {
    const health = healthByGoodId.get(String(good.id));
    const referenceCount = health && typeof health.referenceCount === "number"
      && Number.isSafeInteger(health.referenceCount) && health.referenceCount >= 0
      ? health.referenceCount
      : null;
    return { ...good, referenceCount, healthAvailable: referenceCount !== null };
  });
}

export function wechatGoodReferenceDisplay(input: {
  healthCount?: number | null;
  exactTotal?: number | null;
  exactFresh?: boolean;
}) {
  const safe = (value: unknown): value is number => typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
  if (input.exactFresh === true) {
    if (safe(input.exactTotal)) return { count: input.exactTotal, source: "精确引用接口 total", exact: true };
    return { count: null, source: "精确引用资料无效", exact: false };
  }
  if (safe(input.healthCount)) return { count: input.healthCount, source: "Pricing Health 汇总（非操作门禁）", exact: false };
  return { count: null, source: "引用数未知", exact: false };
}

export interface WechatGoodActionContext {
  good?: WechatVirtualGood | null;
  referencesFresh?: boolean;
  referenceCount?: number | null;
  hasDefaultActiveDependency?: boolean;
}

export function wechatGoodUIActions(context: WechatGoodActionContext, principal: PricingPrincipal | null | undefined) {
  const canView = hasPricingPermission(principal, "pricing:plan:view");
  const canManage = hasPricingPermission(principal, "pricing:wechat-good:manage");
  const good = context.good;
  const factsComplete = Boolean(
    good
    && String(good.id || "").trim()
    && String(good.productId || "").trim()
    && String(good.offerId || "").trim()
    && String(good.channel || "").trim()
    && String(good.environment || "").trim()
    && Number.isSafeInteger(good.platformPriceCents)
    && good.platformPriceCents > 0
    && Number.isSafeInteger(good.revision)
    && good.revision >= 0
    && good.platformRealtimeVerified === false
  );
  const referencesFresh = context.referencesFresh === true;
  const referenceCount = typeof context.referenceCount === "number" && Number.isSafeInteger(context.referenceCount) && context.referenceCount >= 0
    ? context.referenceCount
    : null;
  const writable = canView && canManage && factsComplete && referencesFresh;
  return {
    canView,
    canManage,
    canCreate: canView && canManage,
    canEdit: writable && referenceCount === 0 && String(good?.status || "").toUpperCase() !== "DISABLED",
    canConfirm: writable && String(good?.status || "").toUpperCase() !== "DISABLED",
    canDisable: writable
      && context.hasDefaultActiveDependency === false
      && String(good?.status || "").toUpperCase() !== "DISABLED",
    referencesFresh,
    factsComplete
  };
}

export interface WechatGoodDisableBindingImpact {
  bindingId: string;
  pricePlanId: string;
}

export interface WechatGoodDisableUnknownDependency {
  bindingId: string | null;
  pricePlanId: string | null;
  errorCode: "PAYMENT_BINDING_CONFIGURATION_CHANGED";
}

export function buildWechatGoodDisableImpact(references: readonly WechatGoodReference[]) {
  const activeCandidates = (references || []).filter((item) => item.bindingEnabled === true
    || String(item.bindingStatus || "").trim().toUpperCase() === "ACTIVE");
  const unique = (items: readonly WechatGoodReference[]): WechatGoodDisableBindingImpact[] => {
    const byIdentity = new Map<string, WechatGoodDisableBindingImpact>();
    for (const item of items) {
      const bindingId = String(item.bindingId || "").trim();
      const pricePlanId = String(item.pricePlanId || "").trim();
      if (!bindingId || !pricePlanId) continue;
      byIdentity.set(`${bindingId}\u0000${pricePlanId}`, { bindingId, pricePlanId });
    }
    return [...byIdentity.values()];
  };
  const unknownDependencies: WechatGoodDisableUnknownDependency[] = [];
  const active = activeCandidates.filter((item) => {
    const bindingId = String(item.bindingId || "").trim();
    const pricePlanId = String(item.pricePlanId || "").trim();
    const complete = item.bindingEnabled === true
      && String(item.bindingStatus || "").trim().toUpperCase() === "ACTIVE"
      && typeof item.isDefault === "boolean"
      && Boolean(bindingId)
      && Boolean(pricePlanId);
    if (!complete) {
      unknownDependencies.push({
        bindingId: bindingId || null,
        pricePlanId: pricePlanId || null,
        errorCode: "PAYMENT_BINDING_CONFIGURATION_CHANGED"
      });
    }
    return complete;
  });
  const affectedBindings = unique(active.filter((item) => item.isDefault === false));
  const defaultDependencies = unique(active.filter((item) => item.isDefault === true));
  return {
    affectedBindings,
    defaultDependencies,
    unknownDependencies,
    canDisable: defaultDependencies.length === 0 && unknownDependencies.length === 0
  };
}

export interface PaymentBindingPolicyContext {
  plan?: PricePlan | null;
  binding?: PricePlanPaymentBinding | null;
  good?: WechatVirtualGood | null;
  references?: readonly WechatGoodReference[];
  configurationFresh?: boolean;
  referencesFresh?: boolean;
  now?: Date | string | number;
}

export function paymentBindingEnableReady(input: {
  reasonReady?: boolean;
  selectedCurrentGood?: boolean;
  policyCanEnable?: boolean;
  validationFresh?: boolean;
  validationValid?: boolean;
}) {
  return input.reasonReady === true
    && input.selectedCurrentGood === true
    && input.policyCanEnable === true
    && input.validationFresh === true
    && input.validationValid === true;
}

export function paymentBindingMutationPolicy(context: PaymentBindingPolicyContext, principal: PricingPrincipal | null | undefined) {
  const canView = hasPricingPermission(principal, "pricing:plan:view");
  const canManage = hasPricingPermission(principal, "pricing:price-plan:manage");
  const plan = context.plan;
  const binding = context.binding;
  const good = context.good;
  const blockers: string[] = [];
  const integrityBlockers: string[] = [];
  const historyBlockers: string[] = [];
  const defaultDependencyBlockers: string[] = [];
  const stateBlockers: string[] = [];
  const activationBlockers: string[] = [];
  const add = (target: string[], code: string) => { if (!target.includes(code)) target.push(code); };
  const addIntegrity = (code: string) => { add(integrityBlockers, code); add(blockers, code); };

  if (context.configurationFresh !== true || context.referencesFresh !== true) addIntegrity("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  const hasCompletePlan = Boolean(plan
    && String(plan.pricePlanId || "").trim()
    && String(plan.planId || "").trim()
    && String(plan.channel || "").trim()
    && String(plan.environment || "").trim()
    && Number.isSafeInteger(plan.salePriceCents)
    && plan.salePriceCents > 0
    && Number.isSafeInteger(plan.revision));
  const hasCompleteGood = Boolean(good
    && String(good.id || "").trim()
    && String(good.productId || "").trim()
    && String(good.offerId || "").trim()
    && String(good.channel || "").trim()
    && String(good.environment || "").trim()
    && Number.isSafeInteger(good.platformPriceCents)
    && good.platformPriceCents > 0
    && Number.isSafeInteger(good.revision)
    && good.platformRealtimeVerified === false);
  const hasCompleteBinding = !binding || Boolean(
    String(binding.id || "").trim()
    && String(binding.pricePlanId || "").trim()
    && String(binding.wechatGoodId || "").trim()
    && String(binding.channel || "").trim()
    && String(binding.environment || "").trim()
    && Number.isSafeInteger(binding.providerPriceSnapshotCents)
    && binding.providerPriceSnapshotCents > 0
    && Number.isSafeInteger(binding.revision)
  );
  if (!hasCompletePlan || !hasCompleteGood || !hasCompleteBinding) addIntegrity("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  const identityMatches = Boolean(plan && binding && good
    && String(binding.pricePlanId || "") === String(plan.pricePlanId || "")
    && String(binding.wechatGoodId || "") === String(good.id || "")
    && String(binding.wechatProductId || "") === String(good.productId || ""));
  if (binding && !identityMatches) {
    addIntegrity("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  }

  if (plan && good) {
    const prices = [Number(plan.salePriceCents), Number(good.platformPriceCents)];
    if (binding) prices.push(Number(binding.providerPriceSnapshotCents));
    if (prices.some((value) => !Number.isInteger(value) || value <= 0) || new Set(prices).size !== 1) {
      add(activationBlockers, "PRICE_PLAN_WECHAT_PRICE_MISMATCH");
    }
    const channels = [String(plan.channel || ""), String(good.channel || "")];
    const environments = [String(plan.environment || ""), String(good.environment || "")];
    if (binding) {
      channels.push(String(binding.channel || ""));
      environments.push(String(binding.environment || ""));
    }
    if (channels.some((value) => !value) || environments.some((value) => !value)
      || new Set(channels).size !== 1 || new Set(environments).size !== 1) {
      add(activationBlockers, "PRICE_PLAN_PAYMENT_ENV_MISMATCH");
    }
  }

  const now = new Date(context.now ?? Date.now()).getTime();
  const expiresAt = good?.verificationExpiresAt ? new Date(good.verificationExpiresAt).getTime() : Number.NaN;
  const verificationStatus = String(good?.verificationStatus || "").toUpperCase();
  if (verificationStatus === "VERIFICATION_EXPIRED" || (Number.isFinite(expiresAt) && expiresAt <= now)) {
    add(activationBlockers, "WECHAT_GOOD_VERIFICATION_EXPIRED");
  }
  if (!good?.published || !good?.enabled || String(good?.status || "").toUpperCase() !== "PUBLISHED"
    || verificationStatus !== "MANUALLY_CONFIRMED_PUBLISHED") {
    add(activationBlockers, "WECHAT_GOOD_NOT_CONFIRMED");
  }

  const relatedReferences = (context.references || []).filter((item) => !binding || item.bindingId === binding.id);
  const referenceCountIsSafe = (value: unknown) => typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
  const hasExactBindingReference = Boolean(binding && relatedReferences.some((item) => (
    String(item.bindingId || "") === String(binding.id || "")
    && String(item.pricePlanId || "") === String(plan?.pricePlanId || "")
    && String(item.planId || "") === String(plan?.planId || "")
    && String(item.wechatGoodId || "") === String(good?.id || "")
    && typeof item.bindingEnabled === "boolean"
    && item.bindingEnabled === binding.enabled
    && typeof item.isDefault === "boolean"
    && item.isDefault === plan?.isDefault
    && String(item.bindingStatus || "").trim().toUpperCase() === String(binding.status || "").trim().toUpperCase()
    && referenceCountIsSafe(item.quoteCount)
    && referenceCountIsSafe(item.orderCount)
    && Number.isSafeInteger(item.salePriceCents)
    && item.salePriceCents === plan?.salePriceCents
    && Number.isSafeInteger(item.providerPriceSnapshotCents)
    && item.providerPriceSnapshotCents === binding.providerPriceSnapshotCents
    && String(item.channel || "") === String(binding.channel || "")
    && String(item.environment || "") === String(binding.environment || "")
  )));
  if (binding && context.referencesFresh === true && !hasExactBindingReference) {
    addIntegrity("PAYMENT_BINDING_CONFIGURATION_CHANGED");
  }
  const hasHistory = relatedReferences.some((item) => (referenceCountIsSafe(item.quoteCount) && item.quoteCount > 0)
    || (referenceCountIsSafe(item.orderCount) && item.orderCount > 0));
  if (hasHistory) { add(historyBlockers, "PAYMENT_BINDING_HAS_HISTORY"); add(blockers, "PAYMENT_BINDING_HAS_HISTORY"); }
  const hasDefaultDependency = Boolean(binding?.enabled && (
    plan?.isDefault === true
    || relatedReferences.some((item) => item.isDefault === true && item.bindingEnabled === true && String(item.bindingStatus || "").toUpperCase() === "ACTIVE")
  ));
  if (hasDefaultDependency) {
    add(defaultDependencyBlockers, "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN");
    add(blockers, "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN");
  }
  if (binding?.enabled) { add(stateBlockers, "PAYMENT_BINDING_ACTIVE"); add(blockers, "PAYMENT_BINDING_ACTIVE"); }

  const authorized = canView && canManage;
  const integritySafe = integrityBlockers.length === 0;
  return {
    canView,
    canManage,
    blockers,
    integrityBlockers,
    historyBlockers,
    defaultDependencyBlockers,
    stateBlockers,
    activationBlockers,
    hasHistory,
    hasDefaultDependency,
    canCreate: authorized && integritySafe && activationBlockers.length === 0 && !binding,
    canRebind: authorized && integritySafe && !hasHistory && !hasDefaultDependency && Boolean(binding) && binding?.enabled === false,
    canEnable: authorized && integritySafe && activationBlockers.length === 0 && Boolean(binding) && binding?.enabled === false,
    canDisable: authorized && context.configurationFresh === true && context.referencesFresh === true
      && Boolean(binding) && binding?.enabled === true
      && hasCompletePlan && hasCompleteGood && hasCompleteBinding
      && identityMatches && hasExactBindingReference
      && integritySafe
      && !hasDefaultDependency
  };
}

type UnknownMutation = Record<string, unknown>;

function copyDefined(source: UnknownMutation, keys: string[]): UnknownMutation {
  const payload: UnknownMutation = {};
  for (const key of keys) {
    if (source[key] !== undefined) payload[key] = source[key];
  }
  return payload;
}

export function buildPlanVersionPayload(input: UnknownMutation): UnknownMutation {
  const payload = copyDefined(input, [
    "revision", "memberLevel", "agentLevel", "tokenAmount", "pointsAmount", "durationDays",
    "rightsSnapshot", "commissionRuleVersion", "commissionSnapshot", "effectiveAt", "expiresAt"
  ]);
  payload.reason = String(input.changeReason || input.reason || "").trim();
  return payload;
}

export function buildPlanVersionCreatePayload(input: UnknownMutation): UnknownMutation {
  const payload = copyDefined(input, [
    "memberLevel", "agentLevel", "tokenAmount", "pointsAmount", "durationDays",
    "rightsSnapshot", "commissionRuleVersion", "commissionSnapshot", "effectiveAt", "expiresAt"
  ]);
  payload.reason = String(input.changeReason || input.reason || "").trim();
  return payload;
}

export function buildVersionTransitionPayload(input: UnknownMutation): UnknownMutation {
  return { revision: input.revision, reason: String(input.changeReason || input.reason || "").trim() };
}

export function buildPricePlanCreatePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, [
    "revision", "planVersionId", "code", "name", "kind", "channel", "environment", "currency",
    "salePriceCents", "listPriceCents", "giftPoints", "giftTokens", "validFrom", "validUntil",
    "audienceType", "audienceRule", "isVisible", "changeReason"
  ]);
}

export function buildPricePlanUpdatePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, [
    "revision", "name", "planVersionId", "kind", "channel", "environment", "currency",
    "salePriceCents", "listPriceCents", "giftPoints", "giftTokens", "validFrom", "validUntil",
    "clearValidFrom", "clearValidUntil", "audienceType", "audienceRule", "isVisible", "changeReason"
  ]);
}

export function buildPricePlanClonePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, ["revision", "code", "name", "changeReason"]);
}

export function buildPricePlanTransitionPayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, ["revision", "changeReason"]);
}

export function buildWechatGoodCreatePayload(input: UnknownMutation): UnknownMutation {
  const payload = copyDefined(input, ["channel", "environment", "offerId", "productId", "goodsName", "platformPriceCents", "mode"]);
  payload.reason = String(input.changeReason || input.reason || "").trim();
  return payload;
}

export function buildWechatGoodUpdatePayload(input: UnknownMutation): UnknownMutation {
  const payload = copyDefined(input, ["revision", "channel", "environment", "offerId", "productId", "goodsName", "platformPriceCents", "mode"]);
  payload.reason = String(input.changeReason || input.reason || "").trim();
  return payload;
}

export function buildWechatGoodConfirmationPayload(input: UnknownMutation): UnknownMutation {
  return {
    ...copyDefined(input, ["revision", "verificationReason", "evidence", "verificationExpiresAt"]),
    reason: String(input.changeReason || input.reason || "").trim()
  };
}

export function buildWechatGoodDisablePayload(input: UnknownMutation): UnknownMutation {
  return { revision: input.revision, reason: String(input.changeReason || input.reason || "").trim() };
}

export function buildPaymentBindingCreatePayload(input: UnknownMutation): UnknownMutation {
  return { wechatGoodId: input.wechatGoodId, reason: String(input.changeReason || input.reason || "").trim() };
}

export function buildPaymentBindingRebindPayload(input: UnknownMutation): UnknownMutation {
  return {
    revision: input.revision,
    wechatGoodId: input.wechatGoodId,
    reason: String(input.changeReason || input.reason || "").trim()
  };
}

export function buildPaymentBindingTransitionPayload(input: UnknownMutation): UnknownMutation {
  return {
    revision: input.revision,
    enabled: input.enabled,
    reason: String(input.changeReason || input.reason || "").trim()
  };
}

export function buildWhitelistCreatePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, ["revision", "userId", "reason", "validFrom", "validUntil", "changeReason"]);
}

export function buildWhitelistUpdatePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, [
    "revision", "reason", "validFrom", "validUntil", "clearValidFrom", "clearValidUntil", "changeReason"
  ]);
}

export function buildWhitelistDisablePayload(input: UnknownMutation): UnknownMutation {
  return copyDefined(input, ["revision", "changeReason"]);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringField(record: Record<string, unknown>, key: string): string {
  return typeof record[key] === "string" ? record[key].trim() : "";
}

function normalizedUpper(value: unknown): string {
  return String(value || "").trim().toUpperCase();
}
