import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

import * as domain from "../admin-vue/src/domain/pricePlanAdmin.ts";
import * as apiModule from "../admin-vue/src/api/pricePlanAdmin.ts";
import * as storeModule from "../admin-vue/src/stores/pricePlanAdmin.ts";

function requiredFunction(module, name) {
  assert.equal(typeof module[name], "function", `${name} must be implemented`);
  return module[name];
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

const fullGood = {
  id: "good-1",
  channel: "WECHAT_VIRTUAL",
  environment: "PRODUCTION",
  offerId: "offer-prod",
  productId: "wx-product-100",
  goodsName: "会员正式商品",
  platformPriceCents: 100,
  mode: "short_series_goods",
  published: true,
  enabled: true,
  status: "PUBLISHED",
  verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED",
  verificationSource: "LOCAL_MANUAL_OPERATOR",
  platformRealtimeVerified: false,
  verifiedBy: "wechat-owner",
  verifiedAt: "2026-07-28T02:00:00Z",
  verificationReason: "人工核对微信后台",
  verificationEvidence: "ticket-100",
  verificationSnapshot: { productId: "wx-product-100", offerId: "offer-prod", environment: "PRODUCTION", platformPriceCents: 100 },
  verificationExpiresAt: "2099-12-31T23:59:59Z",
  revision: 4,
  createdAt: "2026-07-27T02:00:00Z",
  updatedAt: "2026-07-28T02:00:00Z"
};

const fullPlan = {
  pricePlanId: "price-1",
  planId: "plan-1",
  planVersionId: "version-1",
  code: "member_normal",
  name: "会员正常价",
  kind: "NORMAL",
  channel: "WECHAT_VIRTUAL",
  environment: "PRODUCTION",
  currency: "CNY",
  salePriceCents: 100,
  listPriceCents: 100,
  giftPoints: 0,
  giftTokens: 0,
  audienceType: "PUBLIC",
  audienceRule: {},
  isVisible: true,
  isDefault: false,
  isEnabled: false,
  status: "DRAFT",
  revision: 3,
  createdAt: "2026-07-27T02:00:00Z",
  updatedAt: "2026-07-28T02:00:00Z",
  hasQuote: false,
  hasOrder: false,
  economicFieldsLocked: false
};

const fullBinding = {
  id: "binding-1",
  pricePlanId: "price-1",
  wechatGoodId: "good-1",
  channel: "WECHAT_VIRTUAL",
  environment: "PRODUCTION",
  providerPriceSnapshotCents: 100,
  enabled: false,
  status: "DRAFT",
  revision: 2,
  createdAt: "2026-07-28T02:00:00Z",
  updatedAt: "2026-07-28T02:00:00Z",
  pricePlanSalePriceCents: 100,
  wechatGoodPriceCents: 100,
  wechatProductId: "wx-product-100",
  verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED",
  priceConsistent: true,
  environmentConsistent: true
};

const fullReference = {
  bindingId: "binding-1",
  pricePlanId: "price-1",
  pricePlanCode: "member_normal",
  pricePlanName: "会员正常价",
  planId: "plan-1",
  planName: "会员套餐",
  isDefault: false,
  bindingStatus: "DRAFT",
  bindingEnabled: false,
  salePriceCents: 100,
  providerPriceSnapshotCents: 100,
  channel: "WECHAT_VIRTUAL",
  environment: "PRODUCTION",
  wechatGoodId: "good-1",
  quoteCount: 0,
  orderCount: 0
};

test("manual confirmation keeps change reason and verification reason independent", () => {
  const buildWechatGoodConfirmationPayload = requiredFunction(domain, "buildWechatGoodConfirmationPayload");
  assert.deepEqual(buildWechatGoodConfirmationPayload({
    revision: 4,
    changeReason: "重新确认本地记录",
    verificationReason: "人工核对微信后台商品页",
    evidence: "ticket-100",
    verificationExpiresAt: "2026-08-28T02:00:00Z",
    productId: "must-not-leak",
    platformPriceCents: 1
  }), {
    revision: 4,
    verificationReason: "人工核对微信后台商品页",
    evidence: "ticket-100",
    verificationExpiresAt: "2026-08-28T02:00:00Z",
    reason: "重新确认本地记录"
  });
});

test("manual confirmation notice never claims realtime WeChat verification", () => {
  assert.equal(domain.WECHAT_MANUAL_CONFIRMATION_NOTICE, "人工确认已发布仅代表本地人工记录，系统未实时连接微信公众平台验证。");
});

test("wechat-good rows expose complete local facts and never fabricate health references", () => {
  const mergeWechatGoodRows = requiredFunction(domain, "mergeWechatGoodRows");
  const second = { ...fullGood, id: "good-2", productId: "wx-product-200", revision: 1 };
  const rows = mergeWechatGoodRows({
    goods: [fullGood, second],
    healthGoods: [{ wechatGoodId: "good-1", wechatProductId: "wx-product-100", environment: "PRODUCTION", referenceCount: 3 }]
  });
  assert.equal(rows[0].productId, "wx-product-100");
  assert.equal(rows[0].referenceCount, 3);
  assert.equal(rows[0].healthAvailable, true);
  assert.equal(rows[1].referenceCount, null);
  assert.equal(rows[1].healthAvailable, false);
});

test("fresh exact reference totals override health summaries and disclose their source", () => {
  const wechatGoodReferenceDisplay = requiredFunction(domain, "wechatGoodReferenceDisplay");
  assert.deepEqual(wechatGoodReferenceDisplay({ healthCount: 8, exactTotal: 2, exactFresh: true }), {
    count: 2,
    source: "精确引用接口 total",
    exact: true
  });
  assert.deepEqual(wechatGoodReferenceDisplay({ healthCount: 8, exactTotal: 2, exactFresh: false }), {
    count: 8,
    source: "Pricing Health 汇总（非操作门禁）",
    exact: false
  });
  assert.deepEqual(wechatGoodReferenceDisplay({ healthCount: 8, exactTotal: -1, exactFresh: true }), {
    count: null,
    source: "精确引用资料无效",
    exact: false
  });
});

test("wechat-good actions split read/write permissions and require fresh references", () => {
  const wechatGoodUIActions = requiredFunction(domain, "wechatGoodUIActions");
  const viewer = { role: "FINANCE_VIEWER", permissions: ["pricing:plan:view"] };
  const owner = { role: "WECHAT_OWNER", permissions: ["pricing:plan:view", "pricing:wechat-good:manage"] };
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 0, hasDefaultActiveDependency: false }, viewer).canView, true);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 0, hasDefaultActiveDependency: false }, viewer).canEdit, false);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 0, hasDefaultActiveDependency: false }, { role: "ADMIN", permissions: ["admin.full"] }).canEdit, false);
  const safe = wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 0, hasDefaultActiveDependency: false }, owner);
  assert.equal(safe.canEdit, true);
  assert.equal(safe.canConfirm, true);
  assert.equal(safe.canDisable, true);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: false, referenceCount: null }, owner).canDisable, false);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: null, hasDefaultActiveDependency: false }, owner).canEdit, false);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 1, hasDefaultActiveDependency: false }, owner).canEdit, false);
  assert.equal(wechatGoodUIActions({ good: fullGood, referencesFresh: true, referenceCount: 1, hasDefaultActiveDependency: true }, owner).canDisable, false);
});

test("payment-binding policy blocks default dependencies, history, drift, and invalid confirmation", () => {
  const paymentBindingMutationPolicy = requiredFunction(domain, "paymentBindingMutationPolicy");
  const owner = { role: "PRICE_OPERATOR", permissions: ["pricing:plan:view", "pricing:price-plan:manage"] };
  const safe = paymentBindingMutationPolicy({
    plan: fullPlan,
    binding: fullBinding,
    good: fullGood,
    references: [fullReference],
    configurationFresh: true,
    referencesFresh: true
  }, owner);
  assert.equal(safe.canEnable, true);
  assert.equal(safe.canRebind, true);
  assert.equal(safe.canDisable, false);

  const priceDrift = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: { ...fullGood, platformPriceCents: 101 }, references: [fullReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(priceDrift.canEnable, false);
  assert.ok(priceDrift.activationBlockers.includes("PRICE_PLAN_WECHAT_PRICE_MISMATCH"));
  const environmentDrift = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: { ...fullGood, environment: "SANDBOX" }, references: [fullReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(environmentDrift.canRebind, true);
  assert.equal(environmentDrift.canEnable, false);
  assert.ok(environmentDrift.activationBlockers.includes("PRICE_PLAN_PAYMENT_ENV_MISMATCH"));
  const unconfirmed = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: { ...fullGood, published: false, enabled: false, status: "DRAFT", verificationStatus: "UNCONFIRMED" }, references: [fullReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(unconfirmed.canEnable, false);
  assert.ok(unconfirmed.activationBlockers.includes("WECHAT_GOOD_NOT_CONFIRMED"));
  const expired = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: { ...fullGood, verificationStatus: "VERIFICATION_EXPIRED" }, references: [fullReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(expired.canEnable, false);
  assert.ok(expired.activationBlockers.includes("WECHAT_GOOD_VERIFICATION_EXPIRED"));
  const historical = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: fullGood, references: [{ ...fullReference, quoteCount: 1 }], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(historical.canRebind, false);
  assert.ok(historical.blockers.includes("PAYMENT_BINDING_HAS_HISTORY"));
  const activeDefaultBinding = { ...fullBinding, enabled: true, status: "ACTIVE" };
  const defaultDependency = paymentBindingMutationPolicy({ plan: { ...fullPlan, isDefault: true, isEnabled: true, status: "ACTIVE" }, binding: activeDefaultBinding, good: fullGood, references: [{ ...fullReference, isDefault: true, bindingEnabled: true, bindingStatus: "ACTIVE" }], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(defaultDependency.canDisable, false);
  assert.equal(defaultDependency.canRebind, false);
  assert.ok(defaultDependency.blockers.includes("PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN"));

  for (const malformed of [
    { binding: { ...fullBinding, pricePlanId: "price-other" }, good: fullGood, references: [fullReference] },
    { binding: { ...fullBinding, wechatGoodId: "good-other" }, good: fullGood, references: [fullReference] },
    { binding: { ...fullBinding, wechatProductId: "wx-product-other" }, good: fullGood, references: [fullReference] },
    { binding: fullBinding, good: fullGood, references: [] },
    { binding: fullBinding, good: fullGood, references: [{ ...fullReference, planId: "plan-other" }] },
    { binding: fullBinding, good: fullGood, references: [{ ...fullReference, wechatGoodId: "good-other" }] }
  ]) {
    const policy = paymentBindingMutationPolicy({ plan: fullPlan, ...malformed, configurationFresh: true, referencesFresh: true }, owner);
    assert.equal(policy.canEnable, false);
    assert.equal(policy.canRebind, false);
    assert.ok(policy.blockers.includes("PAYMENT_BINDING_CONFIGURATION_CHANGED"));
  }

  const enabledNonDefault = { ...fullBinding, enabled: true, status: "ACTIVE" };
  const enabledReference = { ...fullReference, bindingEnabled: true, bindingStatus: "ACTIVE" };
  assert.equal(paymentBindingMutationPolicy({ plan: fullPlan, binding: enabledNonDefault, good: fullGood, references: [enabledReference], configurationFresh: true, referencesFresh: true }, owner).canDisable, true);
  for (const malformed of [
    { binding: { ...enabledNonDefault, id: "" }, good: fullGood, references: [enabledReference] },
    { binding: { ...enabledNonDefault, revision: undefined }, good: fullGood, references: [enabledReference] },
    { binding: { ...enabledNonDefault, revision: null }, good: fullGood, references: [enabledReference] },
    { binding: { ...enabledNonDefault, pricePlanId: "price-other" }, good: fullGood, references: [enabledReference] },
    { binding: { ...enabledNonDefault, wechatGoodId: "good-other" }, good: fullGood, references: [enabledReference] },
    { binding: { ...enabledNonDefault, wechatProductId: "wx-product-other" }, good: fullGood, references: [enabledReference] },
    { binding: enabledNonDefault, good: fullGood, references: [] },
    { binding: enabledNonDefault, good: fullGood, references: [{ ...enabledReference, pricePlanId: "price-other" }] }
  ]) {
    const policy = paymentBindingMutationPolicy({ plan: fullPlan, ...malformed, configurationFresh: true, referencesFresh: true }, owner);
    assert.equal(policy.canDisable, false);
  }
});

test("payment-binding enable requires a fresh validation that explicitly passed", () => {
  const paymentBindingEnableReady = requiredFunction(domain, "paymentBindingEnableReady");
  const baseline = { reasonReady: true, selectedCurrentGood: true, policyCanEnable: true };
  assert.equal(paymentBindingEnableReady({ ...baseline, validationFresh: true, validationValid: true }), true);
  assert.equal(paymentBindingEnableReady({ ...baseline, validationFresh: true, validationValid: false }), false);
  assert.equal(paymentBindingEnableReady({ ...baseline, validationFresh: true, validationValid: undefined }), false);
  assert.equal(paymentBindingEnableReady({ ...baseline, validationFresh: false, validationValid: true }), false);
});

test("drifted bindings remain recoverable without weakening exact identity or activation gates", () => {
  const paymentBindingMutationPolicy = requiredFunction(domain, "paymentBindingMutationPolicy");
  const owner = { role: "PRICE_OPERATOR", permissions: ["pricing:plan:view", "pricing:price-plan:manage"] };
  const driftedGood = { ...fullGood, platformPriceCents: 101 };
  const activeBinding = { ...fullBinding, enabled: true, status: "ACTIVE", wechatGoodPriceCents: 101, priceConsistent: false };
  const activeReference = { ...fullReference, bindingEnabled: true, bindingStatus: "ACTIVE", isDefault: false };
  const activePolicy = paymentBindingMutationPolicy({ plan: { ...fullPlan, status: "ACTIVE", isEnabled: true }, binding: activeBinding, good: driftedGood, references: [activeReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(activePolicy.canDisable, true);
  assert.equal(activePolicy.canEnable, false);
  assert.ok(activePolicy.activationBlockers.includes("PRICE_PLAN_WECHAT_PRICE_MISMATCH"));

  const disabledBinding = { ...fullBinding, enabled: false, status: "DISABLED", wechatGoodPriceCents: 101, priceConsistent: false };
  const disabledReference = { ...fullReference, bindingEnabled: false, bindingStatus: "DISABLED", isDefault: false, quoteCount: 0, orderCount: 0 };
  const disabledPolicy = paymentBindingMutationPolicy({ plan: fullPlan, binding: disabledBinding, good: driftedGood, references: [disabledReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(disabledPolicy.canRebind, true);
  assert.equal(disabledPolicy.canEnable, false);
  assert.ok(disabledPolicy.activationBlockers.includes("PRICE_PLAN_WECHAT_PRICE_MISMATCH"));

  const legacyBindingFacts = { channel: "LEGACY_WECHAT", environment: "SANDBOX" };
  const disabledEnvironmentDrift = paymentBindingMutationPolicy({
    plan: fullPlan,
    binding: { ...fullBinding, ...legacyBindingFacts, enabled: false, status: "DISABLED" },
    good: fullGood,
    references: [{ ...fullReference, ...legacyBindingFacts, bindingEnabled: false, bindingStatus: "DISABLED" }],
    configurationFresh: true,
    referencesFresh: true
  }, owner);
  assert.equal(disabledEnvironmentDrift.canRebind, true);
  assert.equal(disabledEnvironmentDrift.canEnable, false);
  assert.ok(disabledEnvironmentDrift.activationBlockers.includes("PRICE_PLAN_PAYMENT_ENV_MISMATCH"));

  const activeEnvironmentDrift = paymentBindingMutationPolicy({
    plan: { ...fullPlan, status: "ACTIVE", isEnabled: true },
    binding: { ...fullBinding, ...legacyBindingFacts, enabled: true, status: "ACTIVE" },
    good: fullGood,
    references: [{ ...fullReference, ...legacyBindingFacts, bindingEnabled: true, bindingStatus: "ACTIVE", isDefault: false }],
    configurationFresh: true,
    referencesFresh: true
  }, owner);
  assert.equal(activeEnvironmentDrift.canDisable, true);
  assert.equal(activeEnvironmentDrift.canEnable, false);
  assert.ok(activeEnvironmentDrift.activationBlockers.includes("PRICE_PLAN_PAYMENT_ENV_MISMATCH"));

  const identityDrift = paymentBindingMutationPolicy({ plan: fullPlan, binding: { ...disabledBinding, wechatGoodId: "wrong-good" }, good: driftedGood, references: [disabledReference], configurationFresh: true, referencesFresh: true }, owner);
  assert.equal(identityDrift.canRebind, false);
  assert.equal(identityDrift.canDisable, false);
});

test("wechat-good disable impact lists only ACTIVE non-default cascades and blocks defaults", () => {
  const buildWechatGoodDisableImpact = requiredFunction(domain, "buildWechatGoodDisableImpact");
  const impact = buildWechatGoodDisableImpact([
    { ...fullReference, bindingId: "binding-active", pricePlanId: "price-active", bindingEnabled: true, bindingStatus: "ACTIVE", isDefault: false },
    { ...fullReference, bindingId: "binding-default", pricePlanId: "price-default", bindingEnabled: true, bindingStatus: "ACTIVE", isDefault: true },
    { ...fullReference, bindingId: "binding-disabled", pricePlanId: "price-disabled", bindingEnabled: false, bindingStatus: "DISABLED", isDefault: false }
  ]);
  assert.deepEqual(impact.affectedBindings, [{ bindingId: "binding-active", pricePlanId: "price-active" }]);
  assert.deepEqual(impact.defaultDependencies, [{ bindingId: "binding-default", pricePlanId: "price-default" }]);
  assert.equal(impact.canDisable, false);
});

test("wechat-good disable impact fails closed when ACTIVE reference identity or default facts are incomplete", () => {
  const buildWechatGoodDisableImpact = requiredFunction(domain, "buildWechatGoodDisableImpact");
  for (const reference of [
    { ...fullReference, bindingEnabled: true, bindingStatus: "ACTIVE", isDefault: undefined },
    { ...fullReference, bindingEnabled: true, bindingStatus: "ACTIVE", bindingId: "" },
    { ...fullReference, bindingEnabled: true, bindingStatus: "ACTIVE", pricePlanId: "" }
  ]) {
    const impact = buildWechatGoodDisableImpact([reference]);
    assert.equal(impact.canDisable, false);
    assert.equal(impact.unknownDependencies.length, 1);
    assert.equal(impact.unknownDependencies[0].errorCode, "PAYMENT_BINDING_CONFIGURATION_CHANGED");
  }
});

test("malformed reference counts and safety facts fail closed instead of becoming zero", () => {
  const paymentBindingMutationPolicy = requiredFunction(domain, "paymentBindingMutationPolicy");
  const owner = { role: "PRICE_OPERATOR", permissions: ["pricing:plan:view", "pricing:price-plan:manage"] };
  const malformedReferences = [
    { ...fullReference, quoteCount: undefined },
    { ...fullReference, orderCount: undefined },
    { ...fullReference, quoteCount: -1 },
    { ...fullReference, orderCount: Number.MAX_SAFE_INTEGER + 1 },
    { ...fullReference, bindingEnabled: "false" },
    { ...fullReference, isDefault: undefined },
    { ...fullReference, bindingStatus: "" },
    { ...fullReference, providerPriceSnapshotCents: 101 },
    { ...fullReference, salePriceCents: 101 },
    { ...fullReference, environment: "SANDBOX" }
  ];
  for (const reference of malformedReferences) {
    const policy = paymentBindingMutationPolicy({ plan: fullPlan, binding: fullBinding, good: fullGood, references: [reference], configurationFresh: true, referencesFresh: true }, owner);
    assert.equal(policy.canRebind, false);
    assert.ok(policy.integrityBlockers.includes("PAYMENT_BINDING_CONFIGURATION_CHANGED"));
  }
});

test("binding mutation builders keep create, rebind, and state transition requests separate", () => {
  const buildPaymentBindingCreatePayload = requiredFunction(domain, "buildPaymentBindingCreatePayload");
  const buildPaymentBindingRebindPayload = requiredFunction(domain, "buildPaymentBindingRebindPayload");
  const buildPaymentBindingTransitionPayload = requiredFunction(domain, "buildPaymentBindingTransitionPayload");
  assert.deepEqual(buildPaymentBindingCreatePayload({ wechatGoodId: "good-1", changeReason: "创建绑定", productId: "forged", platformPriceCents: 1 }), { wechatGoodId: "good-1", reason: "创建绑定" });
  assert.deepEqual(buildPaymentBindingRebindPayload({ revision: 2, wechatGoodId: "good-2", changeReason: "换绑商品", enabled: true, productId: "forged", platformPriceCents: 1 }), { revision: 2, wechatGoodId: "good-2", reason: "换绑商品" });
  assert.deepEqual(buildPaymentBindingTransitionPayload({ revision: 2, enabled: true, changeReason: "启用绑定", wechatGoodId: "forged", productId: "forged", platformPriceCents: 1 }), { revision: 2, enabled: true, reason: "启用绑定" });
});

test("generic payment-binding update surface is absent", async () => {
  assert.equal(domain.buildPaymentBindingUpdatePayload, undefined);
  assert.equal(apiModule.pricePlanAdminApi.updatePaymentBinding, undefined);
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  assert.equal(usePricePlanAdminStore().updatePaymentBinding, undefined);
});

test("references and separated binding APIs use exact routes and allowlisted payloads", async () => {
  const api = apiModule.pricePlanAdminApi;
  const { apiClient } = await import("../admin-vue/src/api/client.ts");
  const originalAdapter = apiClient.defaults.adapter;
  const requests = [];
  apiClient.defaults.adapter = async (config) => {
    requests.push({ method: config.method?.toUpperCase(), url: config.url, data: typeof config.data === "string" ? JSON.parse(config.data) : config.data });
    return { data: { items: [], total: 0 }, status: 200, statusText: "OK", headers: {}, config };
  };
  try {
    await api.listWechatVirtualGoodReferences("good/1");
    await api.confirmWechatVirtualGood("good/1", { revision: 4, changeReason: "记录变更", verificationReason: "人工核验", evidence: "ticket", productId: "forged" });
    await api.rebindPaymentBinding("binding/1", { revision: 2, wechatGoodId: "good-2", changeReason: "换绑", platformPriceCents: 1 });
    await api.transitionPaymentBinding("binding/1", { revision: 3, enabled: false, changeReason: "停用", wechatGoodId: "forged" });
    assert.deepEqual(requests, [
      { method: "GET", url: "/admin/wechat-virtual-goods/good%2F1/references", data: undefined },
      { method: "POST", url: "/admin/wechat-virtual-goods/good%2F1/confirm-published", data: { revision: 4, verificationReason: "人工核验", evidence: "ticket", reason: "记录变更" } },
      { method: "PATCH", url: "/admin/payment-bindings/binding%2F1", data: { revision: 2, wechatGoodId: "good-2", reason: "换绑" } },
      { method: "PATCH", url: "/admin/payment-bindings/binding%2F1", data: { revision: 3, enabled: false, reason: "停用" } }
    ]);
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("wechat-good writes refresh global references and every affected price decision resource", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const names = ["confirmWechatVirtualGood", "listWechatVirtualGoods", "getWechatVirtualGood", "listWechatVirtualGoodReferences", "getPricingHealth", "getPricePlan", "listPricePlans", "listPaymentBindings", "validatePricePlan"];
  const originals = Object.fromEntries(names.map((name) => [name, api[name]]));
  const calls = [];
  try {
    api.confirmWechatVirtualGood = async () => ({ item: fullGood, confirmation: "LOCAL_MANUAL_ONLY", wechatRealtimeVerified: false });
    api.listWechatVirtualGoods = async () => { calls.push("goods"); return { items: [fullGood], total: 1, verificationSource: "LOCAL_MANUAL_OPERATOR" }; };
    api.getWechatVirtualGood = async (id) => { calls.push(`good:${id}`); return { item: fullGood }; };
    api.listWechatVirtualGoodReferences = async (id) => { calls.push(`references:${id}`); return { items: [fullReference], total: 1 }; };
    api.getPricingHealth = async () => { calls.push("health"); return { status: "HEALTHY", summary: {}, issues: [], businessPlans: [], pricePlans: [], wechatGoods: [], runtime: { v132Blocked: false } }; };
    api.getPricePlan = async (id) => { calls.push(`price:${id}`); return { item: fullPlan }; };
    api.listPricePlans = async (id) => { calls.push(`prices:${id}`); return { items: [fullPlan], total: 1 }; };
    api.listPaymentBindings = async (id) => { calls.push(`bindings:${id}`); return { items: [fullBinding], total: 1 }; };
    api.validatePricePlan = async (id) => { calls.push(`validation:${id}`); return { pricePlanId: id, valid: false, checkedAt: "now", checks: [] }; };
    await store.confirmWechatGood("good-1", { revision: 4, changeReason: "记录变更", verificationReason: "人工核验" });
    assert.deepEqual(new Set(calls), new Set(["goods", "good:good-1", "references:good-1", "health", "price:price-1", "prices:plan-1", "bindings:price-1", "validation:price-1"]));
    assert.equal(store.refreshWarnings["confirmWechatGood:good-1"], undefined);
  } finally {
    Object.assign(api, originals);
  }
});

test("wechat-good refresh failure still attempts cached affected price decisions and locks resubmission", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.wechatGoodReferencesById["good-1"] = [fullReference];
  const api = apiModule.pricePlanAdminApi;
  const names = ["confirmWechatVirtualGood", "listWechatVirtualGoods", "getWechatVirtualGood", "listWechatVirtualGoodReferences", "getPricingHealth", "getPricePlan", "listPricePlans", "listPaymentBindings", "validatePricePlan"];
  const originals = Object.fromEntries(names.map((name) => [name, api[name]]));
  const calls = [];
  try {
    api.confirmWechatVirtualGood = async () => ({ item: fullGood, confirmation: "LOCAL_MANUAL_ONLY", wechatRealtimeVerified: false });
    api.listWechatVirtualGoods = async () => { calls.push("goods"); return { items: [fullGood], total: 1, verificationSource: "LOCAL_MANUAL_OPERATOR" }; };
    api.getWechatVirtualGood = async () => { calls.push("good"); return { item: fullGood }; };
    api.listWechatVirtualGoodReferences = async () => { calls.push("references"); throw new Error("references unavailable"); };
    api.getPricingHealth = async () => { calls.push("health"); return { status: "HEALTHY", summary: {}, issues: [], businessPlans: [], pricePlans: [], wechatGoods: [], runtime: { v132Blocked: false } }; };
    api.getPricePlan = async () => { calls.push("price"); return { item: fullPlan }; };
    api.listPricePlans = async () => { calls.push("prices"); return { items: [fullPlan], total: 1 }; };
    api.listPaymentBindings = async () => { calls.push("bindings"); return { items: [fullBinding], total: 1 }; };
    api.validatePricePlan = async () => { calls.push("validation"); return { pricePlanId: "price-1", valid: false, checkedAt: "now", checks: [] }; };
    await store.confirmWechatGood("good-1", { revision: 4, changeReason: "记录变更", verificationReason: "人工核验" });
    for (const expected of ["goods", "good", "references", "health", "price", "prices", "bindings", "validation"]) assert.ok(calls.includes(expected), expected);
    assert.ok(store.refreshWarnings["confirmWechatGood:good-1"]);
  } finally {
    Object.assign(api, originals);
  }
});

test("binding rebind refreshes old and new goods plus the complete price decision set", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.bindingsByPricePlanId["price-1"] = [fullBinding];
  store.pricePlanById["price-1"] = fullPlan;
  const api = apiModule.pricePlanAdminApi;
  const names = ["rebindPaymentBinding", "listWechatVirtualGoods", "getWechatVirtualGood", "listWechatVirtualGoodReferences", "getPricingHealth", "getPricePlan", "listPricePlans", "listPaymentBindings", "validatePricePlan"];
  const originals = Object.fromEntries(names.map((name) => [name, api[name]]));
  const calls = [];
  try {
    api.rebindPaymentBinding = async () => ({ item: { ...fullBinding, wechatGoodId: "good-2", revision: 3 } });
    api.listWechatVirtualGoods = async () => { calls.push("goods"); return { items: [], total: 0, verificationSource: "LOCAL_MANUAL_OPERATOR" }; };
    api.getWechatVirtualGood = async (id) => { calls.push(`good:${id}`); return { item: { ...fullGood, id } }; };
    api.listWechatVirtualGoodReferences = async (id) => { calls.push(`references:${id}`); return { items: id === "good-2" ? [{ ...fullReference, wechatGoodId: id }] : [], total: id === "good-2" ? 1 : 0 }; };
    api.getPricingHealth = async () => { calls.push("health"); return { status: "HEALTHY", summary: {}, issues: [], businessPlans: [], pricePlans: [], wechatGoods: [], runtime: { v132Blocked: false } }; };
    api.getPricePlan = async (id) => { calls.push(`price:${id}`); return { item: fullPlan }; };
    api.listPricePlans = async (id) => { calls.push(`prices:${id}`); return { items: [fullPlan], total: 1 }; };
    api.listPaymentBindings = async (id) => { calls.push(`bindings:${id}`); return { items: [{ ...fullBinding, wechatGoodId: "good-2", revision: 3 }], total: 1 }; };
    api.validatePricePlan = async (id) => { calls.push(`validation:${id}`); return { pricePlanId: id, valid: false, checkedAt: "now", checks: [] }; };
    await store.rebindPaymentBinding("binding-1", { revision: 2, wechatGoodId: "good-2", changeReason: "换绑" });
    assert.deepEqual(new Set(calls), new Set(["goods", "good:good-1", "good:good-2", "references:good-1", "references:good-2", "health", "price:price-1", "prices:plan-1", "bindings:price-1", "validation:price-1"]));
  } finally {
    Object.assign(api, originals);
  }
});

test("binding refresh failure still attempts all known price and good dependencies", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.bindingsByPricePlanId["price-1"] = [fullBinding];
  store.pricePlanById["price-1"] = fullPlan;
  const api = apiModule.pricePlanAdminApi;
  const names = ["rebindPaymentBinding", "listWechatVirtualGoods", "getWechatVirtualGood", "listWechatVirtualGoodReferences", "getPricingHealth", "getPricePlan", "listPricePlans", "listPaymentBindings", "validatePricePlan"];
  const originals = Object.fromEntries(names.map((name) => [name, api[name]]));
  const calls = [];
  try {
    api.rebindPaymentBinding = async () => ({ item: { ...fullBinding, wechatGoodId: "good-2", revision: 3 } });
    api.getPricePlan = async () => { calls.push("price"); throw new Error("detail unavailable"); };
    api.listPricePlans = async () => { calls.push("prices"); return { items: [fullPlan], total: 1 }; };
    api.listPaymentBindings = async () => { calls.push("bindings"); return { items: [], total: 0 }; };
    api.validatePricePlan = async () => { calls.push("validation"); return { pricePlanId: "price-1", valid: false, checkedAt: "now", checks: [] }; };
    api.listWechatVirtualGoods = async () => { calls.push("goods"); return { items: [], total: 0, verificationSource: "LOCAL_MANUAL_OPERATOR" }; };
    api.getPricingHealth = async () => { calls.push("health"); return { status: "HEALTHY", summary: {}, issues: [], businessPlans: [], pricePlans: [], wechatGoods: [], runtime: { v132Blocked: false } }; };
    api.getWechatVirtualGood = async (id) => { calls.push(`good:${id}`); return { item: { ...fullGood, id } }; };
    api.listWechatVirtualGoodReferences = async (id) => { calls.push(`references:${id}`); return { items: [], total: 0 }; };
    await store.rebindPaymentBinding("binding-1", { revision: 2, wechatGoodId: "good-2", changeReason: "换绑" });
    for (const expected of ["price", "prices", "bindings", "validation", "goods", "health", "good:good-1", "good:good-2", "references:good-1", "references:good-2"]) assert.ok(calls.includes(expected), expected);
    assert.ok(store.refreshWarnings["rebindBinding:binding-1"]);
  } finally {
    Object.assign(api, originals);
  }
});

test("latest request gate prevents reverse-order completion from restoring stale local freshness", async () => {
  const createLatestRequestGate = requiredFunction(domain, "createLatestRequestGate");
  const gate = createLatestRequestGate();
  const older = deferred();
  const newer = deferred();
  let state = { revision: 0, fresh: false, error: "" };
  async function load(request, revision) {
    const token = gate.begin();
    try {
      await request.promise;
      if (gate.isLatest(token)) state = { revision, fresh: true, error: "" };
    } catch (error) {
      if (gate.isLatest(token)) state = { revision: 0, fresh: false, error: String(error) };
    }
  }
  const oldTask = load(older, 1);
  const newTask = load(newer, 2);
  newer.resolve();
  await newTask;
  assert.deepEqual(state, { revision: 2, fresh: true, error: "" });
  older.reject(new Error("stale failure"));
  await oldTask;
  assert.deepEqual(state, { revision: 2, fresh: true, error: "" });
});

test("store resource loads are latest-wins for data, errors, and loading", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originals = {
    getWechatVirtualGood: api.getWechatVirtualGood,
    listWechatVirtualGoodReferences: api.listWechatVirtualGoodReferences,
    listPaymentBindings: api.listPaymentBindings
  };
  try {
    const oldGood = deferred();
    const newGood = deferred();
    let goodCall = 0;
    api.getWechatVirtualGood = () => (++goodCall === 1 ? oldGood.promise : newGood.promise);
    const oldGoodTask = store.loadWechatGood("good-1");
    const newGoodTask = store.loadWechatGood("good-1");
    newGood.resolve({ item: { ...fullGood, revision: 9 } });
    await newGoodTask;
    assert.equal(store.wechatGoodById["good-1"].revision, 9);
    assert.equal(store.loading["wechatGood:good-1"], false);
    oldGood.resolve({ item: { ...fullGood, revision: 2 } });
    await oldGoodTask;
    assert.equal(store.wechatGoodById["good-1"].revision, 9);
    assert.equal(store.loading["wechatGood:good-1"], false);

    const oldReferences = deferred();
    const newReferences = deferred();
    let referenceCall = 0;
    api.listWechatVirtualGoodReferences = () => (++referenceCall === 1 ? oldReferences.promise : newReferences.promise);
    const oldReferenceTask = store.loadWechatGoodReferences("good-1");
    const newReferenceTask = store.loadWechatGoodReferences("good-1");
    newReferences.resolve({ items: [{ ...fullReference, quoteCount: 2 }], total: 1 });
    await newReferenceTask;
    oldReferences.resolve({ items: [{ ...fullReference, quoteCount: 8 }], total: 7 });
    await oldReferenceTask;
    assert.equal(store.wechatGoodReferencePagesById["good-1"].total, 1);
    assert.equal(store.wechatGoodReferencesById["good-1"][0].quoteCount, 2);

    const oldBindings = deferred();
    const newBindings = deferred();
    let bindingCall = 0;
    api.listPaymentBindings = () => (++bindingCall === 1 ? oldBindings.promise : newBindings.promise);
    const oldBindingTask = store.loadPaymentBindings("price-1");
    const newBindingTask = store.loadPaymentBindings("price-1");
    newBindings.resolve({ items: [{ ...fullBinding, revision: 7 }], total: 1 });
    await newBindingTask;
    oldBindings.reject(new Error("stale binding failure"));
    await assert.rejects(oldBindingTask, /stale binding failure/);
    assert.equal(store.bindingsByPricePlanId["price-1"][0].revision, 7);
    assert.equal(store.errors["bindings:price-1"], undefined);
    assert.equal(store.loading["bindings:price-1"], false);
  } finally {
    Object.assign(api, originals);
  }
});

test("list responses cannot overwrite newer entity detail caches across request keys", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originals = {
    listWechatVirtualGoods: api.listWechatVirtualGoods,
    getWechatVirtualGood: api.getWechatVirtualGood,
    listPricePlans: api.listPricePlans,
    getPricePlan: api.getPricePlan
  };
  try {
    const oldGoodsList = deferred();
    api.listWechatVirtualGoods = () => oldGoodsList.promise;
    api.getWechatVirtualGood = async () => ({ item: { ...fullGood, revision: 10 } });
    const oldGoodsTask = store.loadWechatGoods();
    await store.loadWechatGood("good-1");
    oldGoodsList.resolve({ items: [{ ...fullGood, revision: 3 }], total: 1, verificationSource: "LOCAL_MANUAL_ONLY" });
    await oldGoodsTask;
    assert.equal(store.wechatGoodById["good-1"].revision, 10);
    assert.equal(store.wechatGoods[0].revision, 3, "list cache remains independently available for rendering");

    const oldPriceList = deferred();
    api.listPricePlans = () => oldPriceList.promise;
    api.getPricePlan = async () => ({ item: { ...fullPlan, revision: 11 } });
    const oldPricesTask = store.loadPricePlans("plan-1");
    await store.loadPricePlan("price-1");
    oldPriceList.resolve({ items: [{ ...fullPlan, revision: 4 }], total: 1 });
    await oldPricesTask;
    assert.equal(store.pricePlanById["price-1"].revision, 11);
    assert.equal(store.pricePlansByPlanId["plan-1"][0].revision, 4, "list cache remains independently available for rendering");
  } finally {
    Object.assign(api, originals);
  }
});

test("cold decision stores load the exact good detail selected by validation and binding", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.validationByPricePlanId["price-1"] = { pricePlanId: "price-1", paymentBindingId: "binding-1", wechatGoodId: "good-1", valid: true, checkedAt: "now", checks: [] };
  store.bindingsByPricePlanId["price-1"] = [fullBinding];
  store.wechatGoods = [{ ...fullGood, revision: 2 }];
  const api = apiModule.pricePlanAdminApi;
  const originalGet = api.getWechatVirtualGood;
  let requestedGoodId = "";
  try {
    api.getWechatVirtualGood = async (goodId) => {
      requestedGoodId = goodId;
      return { item: { ...fullGood, id: goodId, revision: 12 } };
    };
    await store.loadExactPaymentGood("price-1");
    assert.equal(requestedGoodId, "good-1");
    assert.equal(store.wechatGoodById["good-1"].revision, 12);
    assert.equal(store.wechatGoods[0].revision, 2, "the independent list cache remains untouched");

    store.bindingsByPricePlanId["price-1"] = [{ ...fullBinding, wechatGoodId: "wrong-good" }];
    await assert.rejects(() => store.loadExactPaymentGood("price-1"), /PAYMENT_BINDING_CONFIGURATION_CHANGED/);
  } finally {
    api.getWechatVirtualGood = originalGet;
  }
});

test("Task 6 error codes have stable operator guidance", () => {
  for (const code of [
    "WECHAT_GOOD_CHANNEL_INVALID", "PAYMENT_ENVIRONMENT_INVALID", "WECHAT_GOOD_MODE_INVALID", "WECHAT_GOOD_INVALID",
    "WECHAT_GOOD_NOT_FOUND", "WECHAT_GOOD_ALREADY_EXISTS", "WECHAT_GOOD_HAS_PAYMENT_BINDING", "WECHAT_GOOD_HAS_LIVE_QUOTE",
    "WECHAT_GOOD_VERIFICATION_EXPIRY_INVALID", "WECHAT_GOOD_VERIFICATION_REASON_REQUIRED",
    "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN", "WECHAT_GOOD_REQUIRED", "PAYMENT_BINDING_ALREADY_EXISTS",
    "PAYMENT_BINDING_NOT_FOUND", "PAYMENT_BINDING_MUTATION_REQUIRED", "PAYMENT_BINDING_MUTATION_CONFLICT",
    "PAYMENT_BINDING_CONFIGURATION_CHANGED", "PAYMENT_BINDING_ACTIVE", "PAYMENT_BINDING_HAS_HISTORY",
    "PRICE_PLAN_NOT_ACTIVE", "PRICE_PLAN_STATE_INVALID", "PRICE_PLAN_NOT_MANAGED"
  ]) {
    const message = domain.pricingErrorMessage({ code, message: "backend fallback" });
    assert.notEqual(message, "backend fallback", code);
    assert.notEqual(message, "操作失败，请稍后重试", code);
  }
});

test("Task 6 Vue integration exposes local-only goods and separated binding management", () => {
  const manager = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/WechatVirtualGoodsManager.vue", import.meta.url), "utf8");
  const binding = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PaymentBindingDialog.vue", import.meta.url), "utf8");
  const list = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PricePlanList.vue", import.meta.url), "utf8");
  const governance = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PricePlanGovernance.vue", import.meta.url), "utf8");
  assert.match(manager, /WECHAT_MANUAL_CONFIRMATION_NOTICE/);
  assert.match(manager, /loadWechatGoodReferences/);
  assert.match(manager, /verificationReason/);
  assert.match(manager, /confirmationGood\.mode/);
  assert.match(manager, /buildWechatGoodDisableImpact/);
  assert.match(manager, /unknownDependencies/);
  assert.match(manager, /PAYMENT_BINDING_CONFIGURATION_CHANGED/);
  assert.match(manager, /wechatGoodReferenceDisplay/);
  assert.match(manager, /createLatestRequestGate/);
  assert.match(binding, /createLatestRequestGate/);
  assert.match(manager + binding, /referenceGates/);
  assert.match(binding, /validationValid:\s*validation\.value\?\.valid === true/);
  assert.match(list, /loadExactPaymentGood/);
  const defaultSwitch = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/DefaultPricePlanSwitchDialog.vue", import.meta.url), "utf8");
  assert.match(defaultSwitch, /loadExactPaymentGood/);
  assert.match(manager, /后端将在同一数据库事务中级联停用/);
  assert.doesNotMatch(manager, /停用只修改本地记录/);
  assert.doesNotMatch(manager + binding, /uni\.request|axios\.|fetch\(/);
  for (const fact of ["方案售价", "绑定快照价", "微信商品价", "渠道", "环境", "人工确认", "服务端校验"]) {
    assert.match(binding, new RegExp(fact));
  }
  assert.match(binding, /createPaymentBinding/);
  assert.match(binding, /rebindPaymentBinding/);
  assert.match(binding, /transitionPaymentBinding/);
  assert.match(list, /PaymentBindingDialog/);
  assert.doesNotMatch(list, /Task 5 仅展示本地记录/);
  assert.match(governance, /WechatVirtualGoodsManager/);
});
