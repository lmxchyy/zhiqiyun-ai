import { chromium, expect } from "@playwright/test";
import { spawn, spawnSync } from "node:child_process";
import { access, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptFile = fileURLToPath(import.meta.url);
const repoRoot = path.resolve(path.dirname(scriptFile), "..");
const adminDistIndex = path.join(repoRoot, "admin-vue", "dist", "index.html");
const artifactDir = path.join(repoRoot, "artifacts", "phase2f-admin-pricing");
const reportPath = path.join(artifactDir, "browser-qa-report.json");
let previewPort = Number(process.env.PRICE_PLAN_ADMIN_PREVIEW_PORT || 4174);
let baseURL = process.env.PRICE_PLAN_ADMIN_BASE_URL || `http://127.0.0.1:${previewPort}`;
const targetPath = "/admin/catalog/price-plans";

const report = {
  generatedAt: new Date().toISOString(),
  target: `${baseURL}${targetPath}`,
  flow: `${targetPath} -> V2 套餐列表与六个治理页签 -> 高风险动作成功/阻断与响应式状态`,
  browserPath: {
    classification: "invocation_failed_fallback",
    primaryFailure: "Browser-use doctor: ModuleNotFoundError: browser_use",
    fallback: "repository @playwright/test chromium"
  },
  environment: {
    mockedApi: true,
    productionDatabase: false,
    wechatPlatform: false,
    realAccount: false,
    realGoods: false,
    previewStartedByScript: false,
    browserVersion: ""
  },
  scenarios: [],
  screenshots: [],
  console: { errors: [], warnings: [], pageErrors: [] },
  api: { requests: [], unknown: [], externalNetwork: [] },
  findings: [],
  limitations: [
    "全部业务 API 均由 page.route 在浏览器上下文内模拟；未验证真实后端、真实 RBAC 数据或 PostgreSQL。",
    "默认切换成功路径使用 v132Blocked=false 的隔离模拟响应；默认初始场景仍验证 V132 fail-closed。",
    "只验证 Chromium；未覆盖 Firefox、WebKit、真实账号会话与生产网络条件。"
  ],
  summary: { passed: 0, failed: 0, status: "PENDING" }
};

let previewProcess = null;
let browser = null;
let fatalError = null;

function deferred() {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function nowISO() {
  return "2026-07-28T10:00:00+08:00";
}

function futureISO() {
  return "2099-12-31T23:59:59+08:00";
}

function makePricePlan({ id, code, name, kind, price, goodId, isDefault = false, isVisible = true, audienceType = "PUBLIC" }) {
  return {
    pricePlanId: id,
    planId: "plan-member-v2",
    planVersionId: "plan-version-member-active",
    code,
    name,
    kind,
    channel: "WECHAT_VIRTUAL",
    environment: "PRODUCTION",
    currency: "CNY",
    salePriceCents: price,
    listPriceCents: kind === "NORMAL" ? price : 99600,
    giftPoints: 0,
    giftTokens: kind === "TEST" ? 1 : 996,
    validFrom: "2026-01-01T00:00:00+08:00",
    validUntil: futureISO(),
    audienceType,
    audienceRule: kind === "TEST" ? { entry: "DEDICATED_TEST_ONLY" } : {},
    isVisible,
    isDefault,
    isEnabled: true,
    status: "ACTIVE",
    revision: isDefault ? 8 : 3,
    changeReason: "隔离浏览器 QA 数据",
    createdBy: "qa-operator",
    updatedBy: "qa-operator",
    enabledBy: "qa-operator",
    enabledAt: nowISO(),
    createdAt: nowISO(),
    updatedAt: nowISO(),
    hasQuote: isDefault,
    hasOrder: isDefault,
    economicFieldsLocked: true,
    _goodId: goodId
  };
}

function makeGood({ id, productId, offerId, name, price, confirmed = true }) {
  return {
    id,
    channel: "WECHAT_VIRTUAL",
    environment: "PRODUCTION",
    offerId,
    productId,
    goodsName: name,
    platformPriceCents: price,
    mode: "short_series_goods",
    published: confirmed,
    enabled: true,
    status: confirmed ? "PUBLISHED" : "UNCONFIRMED",
    verificationStatus: confirmed ? "MANUALLY_CONFIRMED_PUBLISHED" : "UNCONFIRMED",
    verificationSource: confirmed ? "LOCAL_MANUAL_CONFIRMATION" : "NONE",
    platformRealtimeVerified: false,
    verifiedBy: confirmed ? "wechat-qa-owner" : undefined,
    verifiedAt: confirmed ? nowISO() : undefined,
    verificationReason: confirmed ? "隔离 QA：依据工单人工核对" : undefined,
    verificationEvidence: confirmed ? "QA-WX-20260728" : undefined,
    verificationSnapshot: confirmed ? {
      productId,
      offerId,
      environment: "PRODUCTION",
      platformPriceCents: price
    } : {},
    verificationExpiresAt: confirmed ? futureISO() : undefined,
    revision: confirmed ? 4 : 1,
    createdBy: "qa-operator",
    updatedBy: "qa-operator",
    createdAt: nowISO(),
    updatedAt: nowISO()
  };
}

function makeMockState(overrides = {}) {
  const initialGate = deferred();
  const plans = [
    makePricePlan({
      id: "price-plan-member-default",
      code: "member_standard",
      name: "会员标准价",
      kind: "NORMAL",
      price: 99600,
      goodId: "wechat-good-standard",
      isDefault: true
    }),
    makePricePlan({
      id: "price-plan-member-promotion",
      code: "member_promotion_alpha",
      name: "会员活动价",
      kind: "PROMOTION",
      price: 100,
      goodId: "wechat-good-promotion"
    }),
    makePricePlan({
      id: "price-plan-member-test",
      code: "member_test_whitelist",
      name: "会员 ¥1 测试价",
      kind: "TEST",
      price: 100,
      goodId: "wechat-good-test",
      isVisible: false,
      audienceType: "WHITELIST"
    })
  ];
  const goods = [
    makeGood({ id: "wechat-good-standard", productId: "wx_member_standard", offerId: "offer-production", name: "会员标准微信商品", price: 99600 }),
    makeGood({ id: "wechat-good-promotion", productId: "wx_member_promotion", offerId: "offer-production", name: "会员活动微信商品", price: 100 }),
    makeGood({ id: "wechat-good-test", productId: "wx_member_test", offerId: "offer-production", name: "会员测试微信商品", price: 100 }),
    makeGood({ id: "wechat-good-unconfirmed", productId: "wx_local_unconfirmed", offerId: "offer-production", name: "未确认测试商品", price: 100, confirmed: false })
  ];
  return {
    name: "main",
    initialGate,
    holdInitial: false,
    emptyPlans: false,
    businessPlansError: 0,
    runtimeUnblocked: false,
    validationErrorId: "",
    auditErrorStatus: 0,
    defaultMutation: "success",
    plans,
    goods,
    requests: [],
    unknown: [],
    ...overrides
  };
}

function businessPlans(state) {
  if (state.emptyPlans) return [];
  return [
    {
      id: "plan-member-v2",
      code: "plan_member",
      name: "会员套餐",
      businessType: "MEMBER",
      legacyCode: false,
      codeReadOnly: true,
      active: true,
      activeVersionId: "plan-version-member-active"
    },
    {
      id: "plan-agent-v2",
      code: "plan_agent",
      name: "代理商套餐",
      businessType: "AGENT",
      legacyCode: false,
      codeReadOnly: true,
      active: true,
      activeVersionId: "plan-version-agent-active"
    }
  ];
}

function planVersions(planId) {
  const businessType = planId.includes("agent") ? "AGENT" : "MEMBER";
  const prefix = businessType === "AGENT" ? "agent" : "member";
  return [
    {
      id: `plan-version-${prefix}-active`,
      planId,
      versionNo: 2,
      businessType,
      rightsSnapshot: { level: businessType === "MEMBER" ? "VIP" : "AGENT_L1", featureFlags: ["V2_SNAPSHOT"] },
      memberLevel: businessType === "MEMBER" ? "VIP" : undefined,
      agentLevel: businessType === "AGENT" ? "AGENT_L1" : undefined,
      tokenAmount: 996,
      pointsAmount: 0,
      durationDays: 365,
      commissionRuleVersion: "commission-v2",
      commissionSnapshot: { version: "commission-v2", rateBasisPoints: businessType === "AGENT" ? 1000 : 0 },
      status: "ACTIVE",
      revision: 5,
      effectiveAt: "2026-01-01T00:00:00+08:00",
      expiresAt: futureISO(),
      createdBy: "qa-operator",
      updatedBy: "qa-operator",
      activatedBy: "qa-operator",
      activatedAt: nowISO(),
      changeReason: "隔离 QA ACTIVE 快照",
      createdAt: nowISO(),
      updatedAt: nowISO()
    },
    {
      id: `plan-version-${prefix}-draft`,
      planId,
      versionNo: 3,
      businessType,
      rightsSnapshot: { level: businessType === "MEMBER" ? "VIP_NEXT" : "AGENT_L2" },
      memberLevel: businessType === "MEMBER" ? "VIP_NEXT" : undefined,
      agentLevel: businessType === "AGENT" ? "AGENT_L2" : undefined,
      tokenAmount: 1200,
      pointsAmount: 0,
      durationDays: 365,
      commissionRuleVersion: "commission-v2",
      commissionSnapshot: { version: "commission-v2" },
      status: "DRAFT",
      revision: 1,
      createdBy: "qa-operator",
      updatedBy: "qa-operator",
      changeReason: "隔离 QA DRAFT",
      createdAt: nowISO(),
      updatedAt: nowISO()
    }
  ];
}

function bindingFor(state, pricePlanId) {
  const plan = state.plans.find((item) => item.pricePlanId === pricePlanId);
  if (!plan) return null;
  const good = state.goods.find((item) => item.id === plan._goodId);
  if (!good) return null;
  return {
    id: `binding-${pricePlanId}`,
    pricePlanId,
    wechatGoodId: good.id,
    channel: plan.channel,
    environment: plan.environment,
    providerPriceSnapshotCents: plan.salePriceCents,
    enabled: true,
    status: "ACTIVE",
    revision: 2,
    createdBy: "qa-operator",
    updatedBy: "qa-operator",
    enabledBy: "qa-operator",
    enabledAt: nowISO(),
    createdAt: nowISO(),
    updatedAt: nowISO(),
    pricePlanSalePriceCents: plan.salePriceCents,
    wechatGoodPriceCents: good.platformPriceCents,
    wechatProductId: good.productId,
    verificationStatus: good.verificationStatus,
    priceConsistent: true,
    environmentConsistent: true
  };
}

function validationFor(state, pricePlanId) {
  const plan = state.plans.find((item) => item.pricePlanId === pricePlanId);
  const binding = bindingFor(state, pricePlanId);
  const good = plan && state.goods.find((item) => item.id === plan._goodId);
  if (!plan || !binding || !good) return null;
  return {
    pricePlanId,
    valid: true,
    checkedAt: nowISO(),
    paymentBindingId: binding.id,
    wechatGoodId: good.id,
    wechatProductId: good.productId,
    pricePlanPriceCents: plan.salePriceCents,
    bindingPriceCents: binding.providerPriceSnapshotCents,
    wechatGoodPriceCents: good.platformPriceCents,
    checks: [
      { code: "PRICE_PLAN_WECHAT_PRICE_MATCH", passed: true, message: "三价一致" },
      { code: "PRICE_PLAN_PAYMENT_ENV_MATCH", passed: true, message: "渠道与环境一致" },
      { code: "WECHAT_GOOD_MANUALLY_CONFIRMED", passed: true, message: "本地人工确认有效" }
    ]
  };
}

function referencesFor(state, goodId) {
  const plan = state.plans.find((item) => item._goodId === goodId);
  const binding = plan ? bindingFor(state, plan.pricePlanId) : null;
  if (!plan || !binding) return [];
  return [{
    bindingId: binding.id,
    pricePlanId: plan.pricePlanId,
    pricePlanCode: plan.code,
    pricePlanName: plan.name,
    planId: plan.planId,
    planName: "会员套餐",
    isDefault: plan.isDefault,
    bindingStatus: binding.status,
    bindingEnabled: binding.enabled,
    salePriceCents: plan.salePriceCents,
    providerPriceSnapshotCents: binding.providerPriceSnapshotCents,
    channel: plan.channel,
    environment: plan.environment,
    wechatGoodId: goodId,
    quoteCount: plan.hasQuote ? 2 : 0,
    orderCount: plan.hasOrder ? 1 : 0
  }];
}

function healthResponse(state) {
  const defaults = state.plans.find((item) => item.isDefault);
  const runtimeBlocked = !state.runtimeUnblocked;
  const issues = runtimeBlocked ? [{
    code: "V132_VALUE_CONSERVATION_NOT_READY",
    severity: "BLOCKING",
    scope: "RUNTIME",
    planId: "plan-member-v2",
    environment: "PRODUCTION",
    message: "V132 价值守恒适配未完成，V2 创建继续 fail-closed。"
  }] : [];
  return {
    checkedAt: nowISO(),
    status: runtimeBlocked ? "BLOCKED" : "HEALTHY",
    summary: {
      businessPlanCount: state.emptyPlans ? 0 : 2,
      pricePlanCount: state.emptyPlans ? 0 : state.plans.length,
      wechatGoodCount: state.goods.length,
      issueCount: issues.length,
      blockedIssueCount: issues.length,
      degradedIssueCount: 0,
      healthyResourceCount: state.plans.length + state.goods.length
    },
    issues,
    businessPlans: state.emptyPlans ? [] : [
      {
        planId: "plan-member-v2",
        name: "会员套餐",
        status: runtimeBlocked ? "BLOCKED" : "HEALTHY",
        issueCodes: issues.map((item) => item.code),
        activeVersionId: "plan-version-member-active",
        pricePlanCount: state.plans.length,
        defaults: {
          production: defaults ? {
            pricePlanId: defaults.pricePlanId,
            salePriceCents: defaults.salePriceCents,
            currency: defaults.currency,
            wechatGoodId: defaults._goodId,
            wechatProductId: state.goods.find((item) => item.id === defaults._goodId)?.productId
          } : null,
          sandbox: null
        }
      },
      {
        planId: "plan-agent-v2",
        name: "代理商套餐",
        status: "DEGRADED",
        issueCodes: ["DEFAULT_PRICE_PLAN_MISSING"],
        activeVersionId: "plan-version-agent-active",
        pricePlanCount: 0,
        defaults: { production: null, sandbox: null }
      }
    ],
    pricePlans: state.plans.map((plan) => {
      const good = state.goods.find((item) => item.id === plan._goodId);
      const binding = bindingFor(state, plan.pricePlanId);
      return {
        pricePlanId: plan.pricePlanId,
        planId: plan.planId,
        planVersionId: plan.planVersionId,
        name: plan.name,
        priceType: plan.kind,
        channel: plan.channel,
        environment: plan.environment,
        status: "HEALTHY",
        issueCodes: [],
        salePriceCents: plan.salePriceCents,
        currency: plan.currency,
        paymentBindingId: binding?.id,
        wechatGoodId: good?.id,
        wechatProductId: good?.productId,
        quoteCount: plan.hasQuote ? 2 : 0,
        orderCount: plan.hasOrder ? 1 : 0
      };
    }),
    wechatGoods: state.goods.map((good) => ({
      wechatGoodId: good.id,
      wechatProductId: good.productId,
      environment: good.environment,
      referenceCount: referencesFor(state, good.id).length
    })),
    runtime: {
      pricePlanCreationEnabled: !runtimeBlocked,
      pricePlanTestEntryEnabled: false,
      snapshotV2FulfillmentEnabled: true,
      v132Blocked: runtimeBlocked,
      v132Scope: runtimeBlocked ? "MEMBER_AGENT_V2_ORDER_CREATION" : "",
      v132AffectedTenantCount: runtimeBlocked ? 1 : 0,
      v132AffectedTenantIds: runtimeBlocked ? ["tenant-v132-qa"] : []
    }
  };
}

function whitelistRows() {
  return [
    {
      whitelistEntryId: "whitelist-active",
      planId: "plan-member-v2",
      pricePlanId: "price-plan-member-test",
      userId: "user-white-active",
      status: "ACTIVE",
      validFrom: "2026-01-01T00:00:00+08:00",
      validUntil: futureISO(),
      reason: "隔离 QA 有效测试用户",
      revision: 2,
      createdBy: "qa-operator",
      updatedBy: "qa-operator",
      createdAt: nowISO(),
      updatedAt: nowISO()
    },
    {
      whitelistEntryId: "whitelist-expired",
      planId: "plan-member-v2",
      pricePlanId: "price-plan-member-test",
      userId: "user-white-expired",
      status: "EXPIRED",
      validFrom: "2026-01-01T00:00:00+08:00",
      validUntil: "2026-06-01T00:00:00+08:00",
      reason: "已过期终态示例",
      revision: 4,
      createdBy: "qa-operator",
      updatedBy: "qa-operator",
      createdAt: nowISO(),
      updatedAt: nowISO()
    },
    {
      whitelistEntryId: "whitelist-disabled",
      planId: "plan-member-v2",
      pricePlanId: "price-plan-member-test",
      userId: "user-white-disabled",
      status: "DISABLED",
      validFrom: "2026-01-01T00:00:00+08:00",
      validUntil: futureISO(),
      reason: "已停用终态示例",
      revision: 5,
      createdBy: "qa-operator",
      updatedBy: "qa-operator",
      disabledBy: "qa-operator",
      disabledAt: nowISO(),
      createdAt: nowISO(),
      updatedAt: nowISO()
    }
  ];
}

function auditPage() {
  return {
    items: [{
      auditLogId: "audit-default-switch-001",
      operatorId: "qa-operator",
      operatorRole: "FINANCE",
      operationTime: nowISO(),
      action: "price_plan.make_default",
      entityType: "PRICE_PLAN",
      entityId: "price-plan-member-promotion",
      changeReason: "隔离 QA 默认价格切换",
      beforeSnapshot: {
        pricePlanId: "price-plan-member-default",
        salePriceCents: 99600,
        appSecret: "DO_NOT_RENDER_APP_SECRET",
        sessionKey: "DO_NOT_RENDER_SESSION_KEY",
        databaseUrl: "postgresql://qa-user:qa-password@db.example/pricing"
      },
      afterSnapshot: {
        pricePlanId: "price-plan-member-promotion",
        salePriceCents: 100,
        authorization: "Bearer DO_NOT_RENDER_TOKEN",
        evidence: "QA-WX-20260728"
      },
      revisionBefore: 2,
      revisionAfter: 3,
      requestId: "request-phase2f-browser-qa",
      result: "SUCCEEDED",
      planId: "plan-member-v2",
      planVersionId: "plan-version-member-active",
      pricePlanId: "price-plan-member-promotion",
      wechatGoodId: "wechat-good-promotion",
      bindingId: "binding-price-plan-member-promotion",
      environment: "PRODUCTION",
      metadata: { accessToken: "DO_NOT_RENDER_ACCESS_TOKEN", safeTicket: "QA-PRICING-2F" }
    }],
    total: 1,
    page: 1,
    pageSize: 50
  };
}

async function fulfill(route, body, status = 200) {
  await route.fulfill({ status, contentType: "application/json; charset=utf-8", body: JSON.stringify(clone(body)) });
}

async function installApiMocks(page, state) {
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const method = request.method().toUpperCase();
    const apiPath = url.pathname.replace(/^\/api\/v1/, "");
    state.requests.push({ method, path: apiPath, query: url.search });
    report.api.requests.push({ scenario: state.name, method, path: apiPath, query: url.search });

    if (method === "GET" && apiPath === "/auth/me") {
      return fulfill(route, {
        accessToken: "qa-local-browser-token",
        workspace: "admin",
        availableWorkspaces: ["admin"],
        defaultModule: "pricePlanGovernance",
        defaultRoute: targetPath,
        permissions: [
          "pricing:plan:view",
          "pricing:entitlement:manage",
          "pricing:price-plan:manage",
          "pricing:price-plan:default",
          "pricing:wechat-good:manage",
          "pricing:test-whitelist:manage",
          "pricing:audit:view"
        ],
        user: {
          id: "qa-operator",
          email: "qa-operator@example.invalid",
          name: "隔离 QA 操作员",
          role: "FINANCE",
          status: "ACTIVE"
        }
      });
    }

    if (state.holdInitial && ["/admin/business-plans", "/admin/pricing-health"].includes(apiPath)) {
      await state.initialGate.promise;
    }

    if (method === "GET" && apiPath === "/admin/business-plans") {
      if (state.businessPlansError) {
        return fulfill(route, { code: "PRICE_PLAN_ADMIN_STORE_UNAVAILABLE", error: "业务套餐服务当前不可用（隔离 QA）" }, state.businessPlansError);
      }
      const items = businessPlans(state);
      return fulfill(route, { items, total: items.length });
    }
    if (method === "GET" && apiPath === "/admin/pricing-health") return fulfill(route, healthResponse(state));

    const planVersionMatch = apiPath.match(/^\/admin\/business-plans\/([^/]+)\/versions$/);
    if (method === "GET" && planVersionMatch) {
      const items = planVersions(decodeURIComponent(planVersionMatch[1]));
      return fulfill(route, { items, total: items.length });
    }
    const planPriceMatch = apiPath.match(/^\/admin\/business-plans\/([^/]+)\/price-plans$/);
    if (method === "GET" && planPriceMatch) {
      const planId = decodeURIComponent(planPriceMatch[1]);
      const items = planId === "plan-member-v2" ? state.plans.map(({ _goodId, ...item }) => item) : [];
      return fulfill(route, { items, total: items.length });
    }
    const businessPlanMatch = apiPath.match(/^\/admin\/business-plans\/([^/]+)$/);
    if (method === "GET" && businessPlanMatch) {
      const plan = businessPlans(state).find((item) => item.id === decodeURIComponent(businessPlanMatch[1]));
      return plan ? fulfill(route, { item: plan }) : fulfill(route, { code: "PLAN_NOT_FOUND", error: "not found" }, 404);
    }

    const validationMatch = apiPath.match(/^\/admin\/price-plans\/([^/]+)\/validation$/);
    if (method === "GET" && validationMatch) {
      const pricePlanId = decodeURIComponent(validationMatch[1]);
      if (state.validationErrorId === pricePlanId) {
        return fulfill(route, { code: "PRICE_PLAN_WECHAT_PRICE_MISMATCH", error: "provider price differs" }, 422);
      }
      const validation = validationFor(state, pricePlanId);
      return validation ? fulfill(route, validation) : fulfill(route, { code: "PRICE_PLAN_NOT_FOUND", error: "not found" }, 404);
    }
    const bindingMatch = apiPath.match(/^\/admin\/price-plans\/([^/]+)\/payment-bindings$/);
    if (method === "GET" && bindingMatch) {
      const binding = bindingFor(state, decodeURIComponent(bindingMatch[1]));
      return fulfill(route, { items: binding ? [binding] : [], total: binding ? 1 : 0 });
    }
    const whitelistMatch = apiPath.match(/^\/admin\/price-plans\/([^/]+)\/whitelist$/);
    if (method === "GET" && whitelistMatch) {
      const pricePlanId = decodeURIComponent(whitelistMatch[1]);
      const all = pricePlanId === "price-plan-member-test" ? whitelistRows() : [];
      const status = url.searchParams.get("status");
      const userId = url.searchParams.get("userId");
      const items = all.filter((item) => (!status || item.status === status) && (!userId || item.userId === userId));
      return fulfill(route, {
        items,
        total: items.length,
        page: Number(url.searchParams.get("page") || 1),
        pageSize: Number(url.searchParams.get("pageSize") || 50)
      });
    }
    const makeDefaultMatch = apiPath.match(/^\/admin\/price-plans\/([^/]+)\/make-default$/);
    if (method === "POST" && makeDefaultMatch) {
      const pricePlanId = decodeURIComponent(makeDefaultMatch[1]);
      if (state.defaultMutation === "conflict") {
        return fulfill(route, { code: "REVISION_CONFLICT", error: "stale revision" }, 409);
      }
      const target = state.plans.find((item) => item.pricePlanId === pricePlanId);
      if (!target) return fulfill(route, { code: "PRICE_PLAN_NOT_FOUND", error: "not found" }, 404);
      const alreadyDefault = target.isDefault === true;
      for (const item of state.plans) {
        if (item.planId === target.planId && item.channel === target.channel && item.environment === target.environment && item.currency === target.currency) {
          item.isDefault = item.pricePlanId === target.pricePlanId;
        }
      }
      target.revision += alreadyDefault ? 0 : 1;
      return fulfill(route, { item: (({ _goodId, ...item }) => item)(target), alreadyDefault });
    }
    const pricePlanMatch = apiPath.match(/^\/admin\/price-plans\/([^/]+)$/);
    if (method === "GET" && pricePlanMatch) {
      const plan = state.plans.find((item) => item.pricePlanId === decodeURIComponent(pricePlanMatch[1]));
      return plan ? fulfill(route, { item: (({ _goodId, ...item }) => item)(plan) }) : fulfill(route, { code: "PRICE_PLAN_NOT_FOUND", error: "not found" }, 404);
    }

    if (method === "GET" && apiPath === "/admin/wechat-virtual-goods") {
      return fulfill(route, { items: state.goods, total: state.goods.length, verificationSource: "LOCAL_MANUAL_ONLY" });
    }
    const goodRefsMatch = apiPath.match(/^\/admin\/wechat-virtual-goods\/([^/]+)\/references$/);
    if (method === "GET" && goodRefsMatch) {
      const items = referencesFor(state, decodeURIComponent(goodRefsMatch[1]));
      return fulfill(route, { items, total: items.length });
    }
    const goodMatch = apiPath.match(/^\/admin\/wechat-virtual-goods\/([^/]+)$/);
    if (method === "GET" && goodMatch) {
      const good = state.goods.find((item) => item.id === decodeURIComponent(goodMatch[1]));
      return good ? fulfill(route, { item: good }) : fulfill(route, { code: "WECHAT_GOOD_NOT_FOUND", error: "not found" }, 404);
    }

    if (method === "GET" && apiPath === "/admin/pricing-audit-logs") {
      if (state.auditErrorStatus) {
        return fulfill(route, { code: "ADMIN_PERMISSION_DENIED", error: "forbidden" }, state.auditErrorStatus);
      }
      return fulfill(route, auditPage());
    }

    state.unknown.push({ method, path: apiPath });
    report.api.unknown.push({ scenario: state.name, method, path: apiPath });
    return fulfill(route, { code: "QA_API_NOT_MOCKED", error: `No isolated QA mock for ${method} ${apiPath}` }, 404);
  });
}

function attachDiagnostics(page, state) {
  page.on("console", (message) => {
    const entry = { scenario: state.name, type: message.type(), text: message.text() };
    if (message.type() === "error") report.console.errors.push(entry);
    if (message.type() === "warning") report.console.warnings.push(entry);
  });
  page.on("pageerror", (error) => {
    report.console.pageErrors.push({ scenario: state.name, message: error.message, stack: error.stack || "" });
  });
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (!["http:", "https:"].includes(url.protocol)) return;
    if (["127.0.0.1", "localhost"].includes(url.hostname)) return;
    report.api.externalNetwork.push({ scenario: state.name, method: request.method(), url: request.url() });
  });
}

async function authenticatedPage(state, viewport) {
  const context = await browser.newContext({ viewport, locale: "zh-CN", timezoneId: "Asia/Shanghai" });
  await context.addInitScript(() => {
    window.localStorage.setItem("token", "qa-local-browser-token");
    window.localStorage.setItem("zhiqiyun.web.has-session", "1");
    window.localStorage.removeItem("xianzhi-admin-active-tab");
    window.localStorage.removeItem("xianzhi-admin-open-tabs");
  });
  const page = await context.newPage();
  attachDiagnostics(page, state);
  await installApiMocks(page, state);
  return { context, page };
}

async function recordScenario(name, fn) {
  const startedAt = new Date().toISOString();
  const start = Date.now();
  try {
    const detail = await fn();
    report.scenarios.push({ name, status: "PASS", startedAt, durationMs: Date.now() - start, detail: detail || {} });
    report.summary.passed += 1;
  } catch (error) {
    report.scenarios.push({
      name,
      status: "FAIL",
      startedAt,
      durationMs: Date.now() - start,
      error: error instanceof Error ? error.message : String(error)
    });
    report.summary.failed += 1;
    report.findings.push({ severity: "BLOCKING_QA", scenario: name, message: error instanceof Error ? error.message : String(error) });
  }
}

async function screenshot(page, filename, label, viewport) {
  const absolutePath = path.join(artifactDir, filename);
  await page.screenshot({ path: absolutePath, fullPage: false, animations: "disabled" });
  report.screenshots.push({ label, viewport, path: absolutePath });
}

async function clickTab(page, name) {
  const visibleOverlay = page.locator(".el-overlay:visible");
  if (await visibleOverlay.count()) {
    await page.keyboard.press("Escape");
    await expect(visibleOverlay).toHaveCount(0);
  }
  const tab = page.getByRole("tab", { name, exact: true });
  await expect(tab).toBeVisible();
  await tab.click();
}

async function waitForPricePlanRows(page) {
  await expect(page.locator(".price-plan-list .el-table__row").filter({ hasText: "会员活动价" }).last()).toBeVisible({ timeout: 15_000 });
}

async function verifyMainFlow() {
  const state = makeMockState({ name: "main", holdInitial: true });
  const { context, page } = await authenticatedPage(state, { width: 1586, height: 992 });
  try {
    await page.goto(`${baseURL}${targetPath}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: "套餐与价格配置", exact: true })).toBeVisible({ timeout: 15_000 });

    await recordScenario("loading-state-and-page-identity", async () => {
      await expect(page.locator(".price-plan-governance .el-skeleton").first()).toBeVisible();
      expect(page.url()).toBe(`${baseURL}${targetPath}`);
      expect(await page.title()).toBe("知启云 AI 后台");
      state.initialGate.resolve();
      await expect(page.getByText("会员套餐", { exact: true }).first()).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText("代理商套餐", { exact: true }).first()).toBeVisible();
      const bodyText = await page.locator("body").innerText();
      expect(bodyText.length).toBeGreaterThan(300);
      await expect(page.locator("vite-error-overlay, .vite-error-overlay, #webpack-dev-server-client-overlay")).toHaveCount(0);
      return { title: await page.title(), url: page.url(), bodyCharacters: bodyText.length };
    });

    await recordScenario("six-tabs-and-readonly-business-plan", async () => {
      const tabs = ["基本信息", "权益版本", "价格方案", "微信商品", "测试白名单", "审计日志"];
      for (const label of tabs) await expect(page.getByRole("tab", { name: label, exact: true })).toBeVisible();
      await clickTab(page, "基本信息");
      await expect(page.getByText("套餐编码（只读）", { exact: true })).toBeVisible();
      await expect(page.locator(".price-plan-basic-grid input").first()).toBeDisabled();
      await screenshot(page, "desktop-overview-1586x992.png", "desktop overview and six tabs", { width: 1586, height: 992 });
      return { tabs };
    });

    await recordScenario("entitlement-version-surface", async () => {
      await clickTab(page, "权益版本");
      await expect(page.getByRole("heading", { name: "权益版本", exact: true })).toBeVisible();
      await expect(page.getByText("版本 v2", { exact: true })).toBeVisible({ timeout: 10_000 });
      await expect(page.getByText("ACTIVE", { exact: true }).first()).toBeVisible();
      await expect(page.getByRole("button", { name: "新建 DRAFT", exact: true })).toBeVisible();
    });

    await recordScenario("v132-fail-closed-default-switch", async () => {
      await clickTab(page, "价格方案");
      await waitForPricePlanRows(page);
      await expect(page.getByText("V132 安全门禁已阻断", { exact: true })).toBeVisible();
      const row = page.locator(".price-plan-list .el-table__row").filter({ hasText: "会员活动价" }).last();
      await expect(row.getByRole("button", { name: "设为默认", exact: true })).toBeDisabled();
      await screenshot(page, "default-switch-v132-blocked.png", "V132 fail-closed default switch", { width: 1586, height: 992 });
      return { v132Blocked: true, buttonDisabled: true };
    });

    await recordScenario("422-price-validation-copy", async () => {
      state.validationErrorId = "price-plan-member-promotion";
      const row = page.locator(".price-plan-list .el-table__row").filter({ hasText: "会员活动价" }).last();
      await row.getByRole("button", { name: "查看校验", exact: true }).click();
      await expect(page.getByText("本次校验加载失败，下面仅展示旧缓存", { exact: true })).toBeVisible();
      await expect(page.getByText("价格方案、支付绑定与微信商品价格不一致，请修正后重试。", { exact: true })).toBeVisible();
      await page.locator(".el-dialog__headerbtn").last().click();
      state.validationErrorId = "";
      return { status: 422, code: "PRICE_PLAN_WECHAT_PRICE_MISMATCH" };
    });

    await recordScenario("409-and-success-default-switch", async () => {
      state.runtimeUnblocked = true;
      await page.reload({ waitUntil: "domcontentloaded" });
      await expect(page.getByRole("heading", { name: "套餐与价格配置", exact: true })).toBeVisible({ timeout: 15_000 });
      await clickTab(page, "价格方案");
      await waitForPricePlanRows(page);
      await expect(page.getByText("V132 安全门禁已阻断", { exact: true })).toHaveCount(0);
      const row = page.locator(".price-plan-list .el-table__row").filter({ hasText: "会员活动价" }).last();
      const switchButton = row.getByRole("button", { name: "设为默认", exact: true });
      await expect(switchButton).toBeEnabled();
      await switchButton.click();
      await expect(page.getByText("服务端将在提交事务内重新锁定并校验全部配置", { exact: true })).toBeVisible({ timeout: 15_000 });
      const drawer = page.locator(".el-drawer:visible").filter({ hasText: "切换默认价格方案" }).last();
      await expect(drawer).toBeVisible();
      await expect(drawer.locator(".el-skeleton")).toHaveCount(0, { timeout: 15_000 });
      const blockerPanel = page.locator(".default-preview__blockers");
      if (await blockerPanel.count()) {
        const blockerText = await blockerPanel.innerText();
        await screenshot(page, "default-switch-unexpected-blockers.png", "unexpected default switch blockers", { width: 1586, height: 992 });
        throw new Error(`default switch preview unexpectedly blocked: ${blockerText}`);
      }
      await drawer.locator("textarea").fill("隔离浏览器 QA 默认切换");
      await drawer.locator(".el-checkbox").click();
      await expect(drawer.getByRole("checkbox")).toBeChecked();

      state.defaultMutation = "conflict";
      await drawer.getByRole("button", { name: "确认切换默认方案", exact: true }).click();
      const confirmation = page.locator(".el-message-box:visible");
      await expect(confirmation).toBeVisible();
      await confirmation.locator(".el-message-box__btns .el-button--primary").click();
      await expect(page.getByText("数据已被其他操作修改，请刷新后重试；不会自动覆盖服务器版本。", { exact: true })).toBeVisible();

      state.defaultMutation = "success";
      await drawer.getByRole("button", { name: "确认切换默认方案", exact: true }).click();
      await expect(confirmation).toBeVisible();
      await confirmation.locator(".el-message-box__btns .el-button--primary").click();
      await expect(page.getByText("默认价格方案已由服务端事务切换", { exact: true })).toBeVisible();
      await waitForPricePlanRows(page);
      const refreshedRow = page.locator(".price-plan-list .el-table__row").filter({ hasText: "会员活动价" }).last();
      await expect(refreshedRow.getByText("默认", { exact: true })).toBeVisible();
      await screenshot(page, "default-switch-success.png", "mocked default switch success", { width: 1586, height: 992 });
      return { conflictStatus: 409, conflictCode: "REVISION_CONFLICT", success: true, syntheticV132Unblocked: true };
    });

    await recordScenario("wechat-manual-confirmation-warning", async () => {
      await clickTab(page, "微信商品");
      await expect(page.getByText("人工确认已发布仅代表本地人工记录，系统未实时连接微信公众平台验证。", { exact: true }).first()).toBeVisible();
      await expect(page.getByText("本页只维护本地商品记录与人工核验快照，不会调用微信公众平台，不会创建、发布或修改真实微信商品。", { exact: true })).toBeVisible();
      const row = page.locator(".wechat-goods-manager .el-table__row").filter({ hasText: "未确认测试商品" }).last();
      const confirmButton = row.getByRole("button", { name: "人工确认已发布", exact: true });
      await expect(confirmButton).toBeEnabled({ timeout: 15_000 });
      await confirmButton.click();
      await expect(page.getByRole("dialog").getByText("人工确认已发布仅代表本地人工记录，系统未实时连接微信公众平台验证。", { exact: true })).toBeVisible();
      await expect(page.getByRole("dialog").getByText("wx_local_unconfirmed", { exact: true })).toBeVisible();
      await screenshot(page, "wechat-manual-confirmation-warning.png", "manual WeChat confirmation warning", { width: 1586, height: 992 });
      await page.getByRole("dialog").getByRole("button", { name: "取消", exact: true }).click();
      return { externalWechatCalls: 0, localManualOnly: true };
    });

    await recordScenario("test-whitelist-terminal-rules", async () => {
      await clickTab(page, "测试白名单");
      await expect(page.getByText("加入白名单不会改变普通购买价格，用户仍需通过专用测试入口。", { exact: true })).toBeVisible();
      await expect(page.locator(".price-plan-whitelist-manager").getByText("会员 ¥1 测试价", { exact: false }).last()).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText("不可恢复，请新建记录", { exact: true })).toHaveCount(2);
      await expect(page.getByText("终态只读", { exact: true })).toHaveCount(2);
      await screenshot(page, "whitelist-terminal-states.png", "TEST whitelist terminal states", { width: 1586, height: 992 });
      return { active: 1, expiredTerminal: 1, disabledTerminal: 1 };
    });

    await recordScenario("audit-redaction-and-403-copy", async () => {
      await clickTab(page, "审计日志");
      const audit = page.locator(".pricing-audit-log");
      await expect(audit.getByText("price_plan.make_default", { exact: true })).toBeVisible({ timeout: 15_000 });
      await audit.getByRole("button", { name: "查看", exact: true }).click();
      const drawer = page.locator(".el-drawer").last();
      await expect(drawer.getByText("审计详情", { exact: true })).toBeVisible();
      await drawer.getByText("变更前快照", { exact: true }).click();
      await expect(drawer.locator('pre[data-audit-snapshot="before"]').filter({ hasText: "[REDACTED]" })).toBeVisible();
      const drawerText = await drawer.innerText();
      for (const secret of ["DO_NOT_RENDER_APP_SECRET", "DO_NOT_RENDER_SESSION_KEY", "qa-password", "DO_NOT_RENDER_TOKEN", "DO_NOT_RENDER_ACCESS_TOKEN"]) {
        expect(drawerText).not.toContain(secret);
      }
      await screenshot(page, "audit-detail-redacted.png", "pricing audit redacted detail", { width: 1586, height: 992 });
      await drawer.locator(".el-drawer__close-btn").click();

      state.auditErrorStatus = 403;
      await audit.locator(".pricing-audit-log__heading").getByRole("button", { name: "刷新", exact: true }).click();
      await expect(audit.getByText("审计日志刷新失败，当前显示同一筛选条件的缓存结果", { exact: true })).toBeVisible();
      await expect(audit.getByText("当前账号没有执行此操作的价格治理权限。", { exact: true })).toBeVisible();
      state.auditErrorStatus = 0;
      return { status: 403, code: "ADMIN_PERMISSION_DENIED", redacted: true };
    });

    await recordScenario("responsive-1024-and-390", async () => {
      await clickTab(page, "基本信息");
      await page.setViewportSize({ width: 1024, height: 900 });
      await expect(page.getByRole("heading", { name: "套餐与价格配置", exact: true })).toBeVisible();
      await screenshot(page, "tablet-overview-1024x900.png", "tablet overview", { width: 1024, height: 900 });

      await page.setViewportSize({ width: 390, height: 844 });
      await expect(page.getByRole("heading", { name: "套餐与价格配置", exact: true })).toBeVisible();
      const overflow = await page.evaluate(() => ({
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        bodyWidth: document.body.scrollWidth
      }));
      await screenshot(page, "mobile-overview-390x844.png", "mobile overview", { width: 390, height: 844 });
      expect(overflow.documentWidth).toBeLessThanOrEqual(overflow.viewportWidth + 2);
      return overflow;
    });
  } finally {
    state.initialGate.resolve();
    await context.close();
  }
}

async function verifyEmptyAndErrorStates() {
  await recordScenario("empty-business-plan-state", async () => {
    const state = makeMockState({ name: "empty", emptyPlans: true, runtimeUnblocked: false });
    const { context, page } = await authenticatedPage(state, { width: 1024, height: 900 });
    try {
      await page.goto(`${baseURL}${targetPath}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByText("没有符合筛选条件的 V2 会员或代理商套餐", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText("请选择一个 V2 业务套餐查看详情", { exact: true })).toBeVisible();
      await screenshot(page, "empty-business-plans.png", "empty V2 business plan state", { width: 1024, height: 900 });
      return { unknownApiCalls: state.unknown.length };
    } finally {
      await context.close();
    }
  });

  await recordScenario("business-plan-error-state", async () => {
    const state = makeMockState({ name: "error", businessPlansError: 503 });
    const { context, page } = await authenticatedPage(state, { width: 1024, height: 900 });
    try {
      await page.goto(`${baseURL}${targetPath}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByText("业务套餐服务当前不可用（隔离 QA）", { exact: false }).first()).toBeVisible({ timeout: 15_000 });
      await screenshot(page, "business-plan-error-503.png", "business plan 503 error state", { width: 1024, height: 900 });
      return { status: 503, stableChineseCopy: true };
    } finally {
      await context.close();
    }
  });
}

async function probePreview(url) {
  try {
    const response = await fetch(`${url}${targetPath}`, { signal: AbortSignal.timeout(1500) });
    const body = await response.text();
    return { available: response.ok, isAdmin: response.ok && body.includes("<title>知启云 AI 后台</title>") };
  } catch {
    return { available: false, isAdmin: false };
  }
}

async function waitForPreview() {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if ((await probePreview(baseURL)).isAdmin) return;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`admin-vue preview did not become ready at ${baseURL}`);
}

async function ensurePreview() {
  const requested = await probePreview(baseURL);
  if (requested.isAdmin) return false;
  if (requested.available && process.env.PRICE_PLAN_ADMIN_BASE_URL) {
    throw new Error(`PRICE_PLAN_ADMIN_BASE_URL is reachable but is not an admin-vue preview: ${baseURL}`);
  }
  if (requested.available) {
    let replacement = null;
    for (let candidate = previewPort + 1; candidate <= previewPort + 20; candidate += 1) {
      const candidateURL = `http://127.0.0.1:${candidate}`;
      const probe = await probePreview(candidateURL);
      if (probe.isAdmin) {
        previewPort = candidate;
        baseURL = candidateURL;
        report.target = `${baseURL}${targetPath}`;
        return false;
      }
      if (!probe.available) {
        replacement = { port: candidate, url: candidateURL };
        break;
      }
    }
    if (!replacement) throw new Error("No free local port found for admin-vue preview after the occupied default port.");
    previewPort = replacement.port;
    baseURL = replacement.url;
    report.target = `${baseURL}${targetPath}`;
  }
  await access(adminDistIndex).catch(() => {
    throw new Error(`Missing ${adminDistIndex}; run npm.cmd --prefix admin-vue run build first.`);
  });
  const command = process.platform === "win32" ? (process.env.ComSpec || "cmd.exe") : "npm";
  const args = process.platform === "win32"
    ? ["/d", "/s", "/c", `npm.cmd --prefix admin-vue run preview -- --host 127.0.0.1 --port ${previewPort} --strictPort`]
    : ["--prefix", "admin-vue", "run", "preview", "--", "--host", "127.0.0.1", "--port", String(previewPort), "--strictPort"];
  previewProcess = spawn(command, args, {
    cwd: repoRoot,
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"]
  });
  const logs = [];
  previewProcess.stdout?.on("data", (chunk) => logs.push(String(chunk)));
  previewProcess.stderr?.on("data", (chunk) => logs.push(String(chunk)));
  previewProcess.once("exit", (code) => {
    if (code && report.environment.previewStartedByScript) {
      report.findings.push({ severity: "INFRA", scenario: "preview", message: `preview exited with ${code}: ${logs.join("").slice(-2000)}` });
    }
  });
  await waitForPreview();
  report.environment.previewStartedByScript = true;
  return true;
}

function stopPreview() {
  if (!previewProcess?.pid) return;
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/PID", String(previewProcess.pid), "/T", "/F"], { stdio: "ignore", windowsHide: true });
  } else {
    previewProcess.kill("SIGTERM");
  }
}

async function writeReport() {
  report.summary.status = report.summary.failed === 0
    && report.api.unknown.length === 0
    && report.api.externalNetwork.length === 0
    && report.console.pageErrors.length === 0
    ? "PASS"
    : "FAIL";
  if (report.api.unknown.length) report.findings.push({ severity: "QA", scenario: "api-mocks", message: `${report.api.unknown.length} API request(s) were not explicitly mocked.` });
  if (report.api.externalNetwork.length) report.findings.push({ severity: "SECURITY", scenario: "network-isolation", message: `${report.api.externalNetwork.length} external network request(s) escaped localhost.` });
  if (report.console.pageErrors.length) report.findings.push({ severity: "RUNTIME", scenario: "page-errors", message: `${report.console.pageErrors.length} page error(s) occurred.` });
  await writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`, "utf8");
}

try {
  await mkdir(artifactDir, { recursive: true });
  await ensurePreview();
  browser = await chromium.launch({ headless: true });
  report.environment.browserVersion = browser.version();
  await verifyMainFlow();
  await verifyEmptyAndErrorStates();
} catch (error) {
  fatalError = error;
  report.summary.failed += 1;
  report.findings.push({ severity: "FATAL", scenario: "runner", message: error instanceof Error ? error.message : String(error) });
} finally {
  await browser?.close().catch(() => undefined);
  stopPreview();
  await writeReport();
}

console.log(JSON.stringify({
  status: report.summary.status,
  passed: report.summary.passed,
  failed: report.summary.failed,
  screenshots: report.screenshots.length,
  report: reportPath,
  fatalError: fatalError instanceof Error ? fatalError.message : fatalError
}, null, 2));

if (report.summary.status !== "PASS") process.exitCode = 1;
