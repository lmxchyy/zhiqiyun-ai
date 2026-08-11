import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

let domain = {};
let client = {};
let apiModule = {};
let storeModule = {};
let governance = {};
try {
  domain = await import("../admin-vue/src/domain/pricePlanAdmin.ts");
} catch {
  // The first RED run intentionally happens before the production module exists.
}
try {
  client = await import("../admin-vue/src/api/client.ts");
} catch {
  // The client enhancement is introduced only after this suite establishes RED.
}
try {
  apiModule = await import("../admin-vue/src/api/pricePlanAdmin.ts");
  storeModule = await import("../admin-vue/src/stores/pricePlanAdmin.ts");
} catch {
  // The Pinia foundation is added after its behavioral contract establishes RED.
}
try {
  governance = await import("../admin-vue/src/domain/pricePlanGovernance.ts");
} catch {
  // Task 3 establishes navigation, list, and legacy-editor behavior before implementation.
}

function requiredFunction(module, name) {
  assert.equal(typeof module[name], "function", `${name} must be implemented`);
  return module[name];
}

test("pricing permissions require an exact pricing grant except for SUPER_ADMIN", () => {
  const hasPricingPermission = requiredFunction(domain, "hasPricingPermission");
  assert.equal(hasPricingPermission({ role: "SUPER_ADMIN", permissions: [] }, "pricing:price-plan:default"), true);
  assert.equal(hasPricingPermission({ role: "ADMIN", permissions: ["admin.full"] }, "pricing:plan:view"), false);
  assert.equal(hasPricingPermission({ role: "ADMIN", permissions: ["pricing:plan:view"] }, "pricing:plan:view"), true);
  assert.equal(hasPricingPermission({ role: "ADMIN", permissions: ["pricing:plan:view"] }, "pricing:price-plan:manage"), false);
});

test("an auth/me payload becomes an exact pricing principal without admin.full expansion", () => {
  const pricingPrincipalFromAuthResponse = requiredFunction(domain, "pricingPrincipalFromAuthResponse");
  const hasPricingPermission = requiredFunction(domain, "hasPricingPermission");
  const operator = pricingPrincipalFromAuthResponse({
    user: { id: "operator-1", role: "PRICING_OPERATOR" },
    permissions: ["admin.full", "pricing:plan:view", "pricing:price-plan:manage"]
  });
  assert.equal(hasPricingPermission(operator, "pricing:plan:view"), true);
  assert.equal(hasPricingPermission(operator, "pricing:price-plan:default"), false);
  const legacy = pricingPrincipalFromAuthResponse({ user: { id: "legacy", role: "ADMIN" }, permissions: ["admin.full"] });
  assert.equal(hasPricingPermission(legacy, "pricing:plan:view"), false);
  const superAdmin = pricingPrincipalFromAuthResponse({ user: { id: "root", role: "SUPER_ADMIN" }, permissions: [] });
  assert.equal(hasPricingPermission(superAdmin, "pricing:audit:view"), true);
});

test("stable pricing error codes produce explicit Chinese recovery guidance", () => {
  const pricingErrorMessage = requiredFunction(domain, "pricingErrorMessage");
  const expected = {
    REVISION_CONFLICT: "数据已被其他操作修改，请刷新后重试；不会自动覆盖服务器版本。",
    PRICE_PLAN_CLONE_REQUIRED: "当前价格方案不能原地修改经济字段，请克隆为新方案后调整。",
    PRICE_PLAN_TEST_REQUIRED: "只有 TEST 价格方案可以管理测试白名单。",
    PRICE_PLAN_WECHAT_PRICE_MISMATCH: "价格方案、支付绑定与微信商品价格不一致，请修正后重试。",
    PRICE_PLAN_PAYMENT_ENV_MISMATCH: "价格方案、支付绑定与微信商品的渠道或环境不一致。",
    WECHAT_GOOD_VERIFICATION_EXPIRED: "微信商品人工确认已过期，请重新人工核验后确认。",
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
    REASON_REQUIRED: "必须填写变更原因。"
  };
  for (const [code, message] of Object.entries(expected)) {
    assert.equal(pricingErrorMessage({ code, message: "backend fallback" }), message, code);
  }
  assert.equal(pricingErrorMessage({ message: "服务端返回的中文说明" }), "服务端返回的中文说明");
});

test("AdminApiError preserves backend code, HTTP status, and structured payload", () => {
  const createAdminApiError = requiredFunction(client, "createAdminApiError");
  const payload = { code: "REVISION_CONFLICT", message: "stale revision", details: { serverRevision: 8 } };
  const error = createAdminApiError(payload, 409);
  assert.equal(error.name, "AdminApiError");
  assert.equal(error.code, "REVISION_CONFLICT");
  assert.equal(error.status, 409);
  assert.deepEqual(error.payload, payload);
  assert.match(error.message, /刷新/);
});

test("Task 5 price-plan errors keep stable operator guidance", () => {
  const pricingErrorMessage = requiredFunction(domain, "pricingErrorMessage");
  for (const code of [
    "PRICE_PLAN_CODE_FORMAT_INVALID",
    "PRICE_PLAN_CODE_HAS_PRICE",
    "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE",
    "PRICE_PLAN_VERSION_NOT_ACTIVE",
    "PRICE_PLAN_OUTSIDE_VALIDITY",
    "PRICE_PLAN_BINDING_NOT_ACTIVE",
    "WECHAT_GOOD_NOT_CONFIRMED",
    "WECHAT_GOOD_NOT_AVAILABLE",
    "PRICE_PLAN_DEFAULT_TEST_FORBIDDEN",
    "PRICE_PLAN_DEFAULT_HIDDEN",
    "PRICE_PLAN_DEFAULT_AUDIENCE_INVALID",
    "PRICE_PLAN_DEFAULT_DISABLE_FORBIDDEN",
    "PRICE_PLAN_CONFIGURATION_CHANGED",
    "PRICE_PLAN_DEFAULT_CONFLICT"
  ]) {
    const message = pricingErrorMessage({ code, message: "backend fallback" });
    assert.notEqual(message, "操作失败，请稍后重试", code);
    assert.notEqual(message, "backend fallback", code);
  }
});

test("entitlement version actions keep ACTIVE and RETIRED snapshots immutable", () => {
  const entitlementVersionActions = requiredFunction(domain, "entitlementVersionActions");
  assert.deepEqual(entitlementVersionActions({ status: "DRAFT" }), {
    canEdit: true,
    canActivate: true,
    canRetire: false,
    canClone: true,
    cloneRequired: false
  });
  assert.deepEqual(entitlementVersionActions({ status: "ACTIVE" }), {
    canEdit: false,
    canActivate: false,
    canRetire: true,
    canClone: true,
    cloneRequired: true
  });
  assert.deepEqual(entitlementVersionActions({ status: "RETIRED" }), {
    canEdit: false,
    canActivate: false,
    canRetire: false,
    canClone: true,
    cloneRequired: true
  });
});

test("entitlement-version UI actions require the exact manage permission", () => {
  const entitlementVersionUIActions = requiredFunction(domain, "entitlementVersionUIActions");
  const draft = { status: "DRAFT" };
  const active = { status: "ACTIVE" };
  const retired = { status: "RETIRED" };

  assert.deepEqual(entitlementVersionUIActions(draft, { role: "ADMIN", permissions: ["admin.full"] }), {
    readOnly: true,
    canEdit: false,
    canActivate: false,
    canRetire: false,
    canClone: false,
    cloneRequired: false
  });
  assert.deepEqual(entitlementVersionUIActions(draft, { role: "PRICING_OPERATOR", permissions: ["pricing:entitlement:manage"] }), {
    readOnly: false,
    canEdit: true,
    canActivate: true,
    canRetire: false,
    canClone: true,
    cloneRequired: false
  });
  assert.deepEqual(entitlementVersionUIActions(active, { role: "SUPER_ADMIN", permissions: [] }), {
    readOnly: true,
    canEdit: false,
    canActivate: false,
    canRetire: true,
    canClone: true,
    cloneRequired: true
  });
  assert.deepEqual(entitlementVersionUIActions(retired, { role: "SUPER_ADMIN", permissions: [] }), {
    readOnly: true,
    canEdit: false,
    canActivate: false,
    canRetire: false,
    canClone: true,
    cloneRequired: true
  });
});

test("entitlement-version list never reports a first-load failure as an empty result", () => {
  const planVersionListDisplayState = requiredFunction(domain, "planVersionListDisplayState");
  assert.equal(planVersionListDisplayState({ loading: false, loaded: false, error: "", versionCount: 0 }), "LOADING");
  assert.equal(planVersionListDisplayState({ loading: true, loaded: false, error: "", versionCount: 0 }), "LOADING");
  assert.equal(planVersionListDisplayState({ loading: false, loaded: false, error: "forbidden", versionCount: 0 }), "ERROR");
  assert.equal(planVersionListDisplayState({ loading: false, loaded: true, error: "", versionCount: 0 }), "EMPTY");
  assert.equal(planVersionListDisplayState({ loading: false, loaded: true, error: "refresh failed", versionCount: 2 }), "LIST");
  assert.equal(planVersionListDisplayState({ loading: false, loaded: true, error: "", versionCount: 2 }), "LIST");
});

test("cloning an entitlement version deep-copies only new-DRAFT business fields", () => {
  const cloneEntitlementVersionDraft = requiredFunction(domain, "cloneEntitlementVersionDraft");
  const source = {
    id: "version-active",
    planId: "plan-member",
    versionNo: 8,
    businessType: "MEMBER",
    memberLevel: "PRO",
    tokenAmount: 2000,
    pointsAmount: 30,
    durationDays: 365,
    rightsSnapshot: { export: true, nested: { quota: 3 } },
    commissionRuleVersion: "commission-v2",
    commissionSnapshot: { levels: [{ rate: 12 }] },
    effectiveAt: "2026-07-28T00:00:00Z",
    expiresAt: "2027-07-28T00:00:00Z",
    status: "ACTIVE",
    revision: 9,
    createdBy: "operator-1",
    activatedBy: "operator-2",
    changeReason: "old reason",
    salePriceCents: 99600,
    productId: "wx-old"
  };
  const draft = cloneEntitlementVersionDraft(source);
  assert.deepEqual(draft, {
    memberLevel: "PRO",
    tokenAmount: 2000,
    pointsAmount: 30,
    durationDays: 365,
    rightsSnapshot: { export: true, nested: { quota: 3 } },
    commissionRuleVersion: "commission-v2",
    commissionSnapshot: { levels: [{ rate: 12 }] },
    effectiveAt: "2026-07-28T00:00:00Z",
    expiresAt: "2027-07-28T00:00:00Z",
    changeReason: ""
  });
  draft.rightsSnapshot.nested.quota = 99;
  draft.commissionSnapshot.levels[0].rate = 88;
  assert.equal(source.rightsSnapshot.nested.quota, 3);
  assert.equal(source.commissionSnapshot.levels[0].rate, 12);
});

test("entitlement transition confirmation reads the real ACTIVE version list", () => {
  const entitlementVersionTransitionPreview = requiredFunction(domain, "entitlementVersionTransitionPreview");
  const versions = [
    { id: "health-stale", versionNo: 11, status: "RETIRED" },
    { id: "draft-new", versionNo: 12, status: "DRAFT" },
    { id: "active-real", versionNo: 10, status: "ACTIVE", revision: 4 }
  ];
  assert.deepEqual(entitlementVersionTransitionPreview(versions[1], versions), {
    action: "ACTIVATE",
    currentActiveVersionId: "active-real",
    currentActiveVersionNo: 10,
    willRetireCurrentActive: true,
    mayLeaveNoActive: false
  });
  assert.deepEqual(entitlementVersionTransitionPreview(versions[2], versions), {
    action: "RETIRE",
    currentActiveVersionId: "active-real",
    currentActiveVersionNo: 10,
    willRetireCurrentActive: false,
    mayLeaveNoActive: true
  });
});

test("entitlement JSON snapshots accept objects and reject arrays or scalar JSON", () => {
  const parseEntitlementJSONObject = requiredFunction(domain, "parseEntitlementJSONObject");
  assert.deepEqual(parseEntitlementJSONObject('{"nested":{"enabled":true}}', "权益 JSON"), { nested: { enabled: true } });
  for (const invalid of ["[]", "null", '"text"', "1", "not-json"]) {
    assert.throws(() => parseEntitlementJSONObject(invalid, "权益 JSON"), /权益 JSON.*JSON 对象/);
  }
});

test("price plan actions enforce TEST, visibility, audience, economic lock, V132, and gift-points gates", () => {
  const pricePlanActions = requiredFunction(domain, "pricePlanActions");
  const active = {
    status: "ACTIVE",
    kind: "NORMAL",
    isVisible: true,
    audienceType: "PUBLIC",
    isEnabled: true,
    isDefault: false,
    economicFieldsLocked: false,
    giftPoints: 0
  };
  assert.equal(pricePlanActions(active, { validationValid: true }).canMakeDefault, true);
  assert.equal(pricePlanActions({ ...active, kind: "TEST", isVisible: false, audienceType: "TEST" }, { validationValid: true }).canMakeDefault, false);
  assert.equal(pricePlanActions({ ...active, isVisible: false }, { validationValid: true }).canMakeDefault, false);
  assert.equal(pricePlanActions({ ...active, audienceType: "WHITELIST" }, { validationValid: true }).canMakeDefault, false);
  assert.equal(pricePlanActions({ ...active, isDefault: true }, { validationValid: true }).canMakeDefault, false);
  assert.equal(pricePlanActions({ ...active, isDefault: true }, { validationValid: true }).canDisable, false);
  assert.equal(pricePlanActions({ ...active, economicFieldsLocked: true }, { validationValid: true }).canEditEconomicFields, false);
  assert.equal(pricePlanActions({ ...active, economicFieldsLocked: true }, { validationValid: true }).mustCloneForEconomicChange, true);
  assert.equal(pricePlanActions({ ...active, status: "DRAFT", isEnabled: false, giftPoints: 1 }, { validationValid: true }).canEnable, false);
  assert.equal(pricePlanActions({ ...active, status: "DRAFT", isEnabled: false }, { validationValid: true, v132Blocked: true }).canEnable, false);
  assert.equal(pricePlanActions({ ...active, status: "INACTIVE", isEnabled: false }, { validationValid: true }).canClone, true);
});

test("price-plan rows merge lifecycle, health, validation, binding, and good without semantic overwrite", () => {
  const mergePricePlanRows = requiredFunction(domain, "mergePricePlanRows");
  const plans = [{
    pricePlanId: "price-promo",
    planId: "plan-member",
    planVersionId: "version-active",
    code: "member_campaign",
    name: "会员活动价",
    kind: "PROMOTION",
    status: "ACTIVE",
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    currency: "CNY",
    salePriceCents: 100,
    listPriceCents: 99600,
    giftPoints: 0,
    giftTokens: 0,
    audienceType: "PUBLIC",
    audienceRule: {},
    isVisible: true,
    isDefault: false,
    isEnabled: true,
    revision: 7,
    hasQuote: true,
    hasOrder: false,
    economicFieldsLocked: true
  }, {
    pricePlanId: "price-no-health",
    planId: "plan-member",
    planVersionId: "version-active",
    code: "member_pending",
    name: "待检查方案",
    kind: "NORMAL",
    status: "DRAFT",
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    currency: "CNY",
    salePriceCents: 99600,
    listPriceCents: 99600,
    giftPoints: 0,
    giftTokens: 0,
    audienceType: "PUBLIC",
    audienceRule: {},
    isVisible: true,
    isDefault: false,
    isEnabled: false,
    revision: 1,
    hasQuote: false,
    hasOrder: false,
    economicFieldsLocked: false
  }];
  const validation = {
    pricePlanId: "price-promo",
    valid: false,
    checkedAt: "2026-07-28T08:00:00Z",
    paymentBindingId: "binding-1",
    wechatGoodId: "good-1",
    wechatProductId: "wx-product-1",
    pricePlanPriceCents: 100,
    bindingPriceCents: 100,
    wechatGoodPriceCents: 99,
    checks: [{ code: "PRICE_PLAN_WECHAT_PRICE_MISMATCH", passed: false }]
  };
  const binding = {
    id: "binding-1",
    pricePlanId: "price-promo",
    wechatGoodId: "good-1",
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    providerPriceSnapshotCents: 100,
    pricePlanSalePriceCents: 100,
    wechatGoodPriceCents: 99,
    wechatProductId: "wx-product-1",
    enabled: true,
    status: "ACTIVE"
  };
  const good = {
    id: "good-1",
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    productId: "wx-product-1",
    offerId: "offer-1",
    platformPriceCents: 99,
    enabled: true,
    verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED"
  };

  const rows = mergePricePlanRows({
    plans,
    healthPlans: [{
      pricePlanId: "price-promo",
      planId: "plan-member",
      planVersionId: "version-active",
      name: "health copy",
      priceType: "ACTIVITY",
      channel: "WECHAT_VIRTUAL",
      environment: "SANDBOX",
      status: "BLOCKED",
      issueCodes: ["PRICE_PLAN_WECHAT_PRICE_MISMATCH"],
      salePriceCents: 100,
      currency: "CNY",
      paymentBindingId: "binding-1",
      wechatGoodId: "good-1",
      quoteCount: 3,
      orderCount: 2
    }],
    validationsByPricePlanId: { "price-promo": validation },
    freshValidationIds: new Set(["price-promo"]),
    bindingsByPricePlanId: { "price-promo": [binding] },
    goodsById: { "good-1": good }
  });

  assert.equal(rows[0].status, "ACTIVE", "health.status is not lifecycle status");
  assert.equal(rows[0].kind, "PROMOTION", "health.priceType must not overwrite API kind");
  assert.equal(rows[0].healthStatus, "BLOCKED");
  assert.equal(rows[0].quoteCount, 3);
  assert.equal(rows[0].orderCount, 2);
  assert.equal(rows[0].validation, validation);
  assert.equal(rows[0].validationFresh, true);
  assert.equal(rows[0].paymentBinding, binding);
  assert.equal(rows[0].wechatGood, good);
  assert.equal(rows[0].paymentDataComplete, true);
  assert.equal(rows[1].healthStatus, "UNKNOWN");
  assert.equal(rows[1].quoteCount, null, "missing health is unknown, not a fake zero");
  assert.equal(rows[1].orderCount, null);
  assert.equal(rows[1].healthAvailable, false);
  assert.equal(rows[1].paymentDataComplete, false);

  const missingExactBinding = mergePricePlanRows({
    plans: [plans[0]],
    healthPlans: [],
    validationsByPricePlanId: { "price-promo": { ...validation, paymentBindingId: "binding-missing" } },
    freshValidationIds: new Set(["price-promo"]),
    bindingsByPricePlanId: { "price-promo": [{ ...binding, id: "binding-other" }] },
    goodsById: { "good-1": good }
  })[0];
  assert.equal(missingExactBinding.paymentBinding, undefined, "an explicit validation bindingId must never fall back to another ACTIVE binding");
  assert.equal(missingExactBinding.wechatGood, undefined);
  assert.equal(missingExactBinding.paymentDataComplete, false);

  const mismatchedProduct = mergePricePlanRows({
    plans: [plans[0]],
    healthPlans: [],
    validationsByPricePlanId: { "price-promo": validation },
    freshValidationIds: new Set(["price-promo"]),
    bindingsByPricePlanId: { "price-promo": [{ ...binding, wechatProductId: "wx-product-other" }] },
    goodsById: { "good-1": good }
  })[0];
  assert.equal(mismatchedProduct.paymentDataComplete, false, "binding and good product identities cannot be stitched together");
});

test("price-plan UI actions split metadata/economics and keep manage/default permissions independent", () => {
  const pricePlanUIActions = requiredFunction(domain, "pricePlanUIActions");
  const ready = {
    validationValid: true,
    validationFresh: true,
    runtimeSafetyKnown: true,
    v132Blocked: false,
    paymentDataComplete: true,
    hasActiveBinding: false
  };
  const draft = { status: "DRAFT", kind: "NORMAL", isVisible: true, audienceType: "PUBLIC", isEnabled: false, isDefault: false, giftPoints: 0, healthAvailable: true };
  const active = { ...draft, status: "ACTIVE", isEnabled: true };
  const manage = { role: "PRICING_OPERATOR", permissions: ["pricing:price-plan:manage"] };
  const defaultOwner = { role: "PRICING_OWNER", permissions: ["pricing:price-plan:default"] };

  assert.deepEqual(pricePlanUIActions({ ...draft, economicFieldsLocked: false }, ready, manage), {
    canEditMetadata: true,
    canEditEconomicFields: true,
    mustCloneForEconomicChange: false,
    economicBlocker: "",
    canClone: true,
    canValidate: true,
    canEnable: true,
    canDisable: false,
    canMakeDefault: false,
    canManageWhitelist: false
  });
  const boundDraft = pricePlanUIActions(draft, { ...ready, hasActiveBinding: true }, manage);
  assert.equal(boundDraft.canEditMetadata, true);
  assert.equal(boundDraft.canEditEconomicFields, false);
  assert.equal(boundDraft.economicBlocker, "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE");
  for (const status of ["ACTIVE", "INACTIVE", "EXPIRED"]) {
    const actions = pricePlanUIActions({ ...active, status }, ready, manage);
    assert.equal(actions.canEditMetadata, true, status);
    assert.equal(actions.canEditEconomicFields, false, status);
    assert.equal(actions.mustCloneForEconomicChange, true, status);
    assert.equal(actions.economicBlocker, "PRICE_PLAN_CLONE_REQUIRED", status);
  }
  assert.equal(pricePlanUIActions(active, ready, manage).canMakeDefault, false);
  assert.equal(pricePlanUIActions(active, ready, defaultOwner).canMakeDefault, true);
  assert.equal(pricePlanUIActions(active, ready, defaultOwner).canDisable, false);
  assert.equal(pricePlanUIActions(active, ready, { role: "ADMIN", permissions: ["admin.full"] }).canMakeDefault, false);
  assert.equal(pricePlanUIActions({ ...active, kind: "TEST", isVisible: false, audienceType: "TEST" }, ready, { role: "SUPER_ADMIN", permissions: [] }).canMakeDefault, false);
  assert.equal(pricePlanUIActions(active, { ...ready, validationFresh: false }, defaultOwner).canMakeDefault, false);
  assert.equal(pricePlanUIActions(active, { ...ready, v132Blocked: true }, defaultOwner).canMakeDefault, false);
  assert.equal(pricePlanUIActions(active, { ...ready, v132Blocked: undefined }, defaultOwner).canMakeDefault, false, "missing V132 state is not safe");
  assert.equal(pricePlanUIActions({ ...draft, healthAvailable: false }, ready, manage).canEnable, false, "missing health row blocks sensitive actions");
  assert.equal(pricePlanUIActions({ ...draft, giftPoints: undefined }, ready, manage).canEnable, false, "missing giftPoints is not zero");
  assert.equal(pricePlanUIActions({ ...draft, giftPoints: 0.5 }, ready, manage).canEnable, false, "giftPoints must be a safe integer");
});

test("price-plan enable fails closed on malformed lifecycle, booleans, or safety fields", () => {
  const pricePlanUIActions = requiredFunction(domain, "pricePlanUIActions");
  const context = {
    validationValid: true,
    validationFresh: true,
    runtimeSafetyKnown: true,
    v132Blocked: false,
    paymentDataComplete: true,
    hasActiveBinding: false
  };
  const manage = { role: "PRICING_OPERATOR", permissions: ["pricing:price-plan:manage"] };
  const safe = {
    status: "DRAFT", kind: "NORMAL", audienceType: "PUBLIC",
    isVisible: true, isEnabled: false, isDefault: false,
    giftPoints: 0, healthAvailable: true
  };

  assert.equal(pricePlanUIActions(safe, context, manage).canEnable, true);
  assert.equal(pricePlanUIActions({ ...safe, status: "INACTIVE" }, context, manage).canEnable, true);
  for (const status of [undefined, "", "ACTIVE", "EXPIRED", "CORRUPT"]) {
    assert.equal(pricePlanUIActions({ ...safe, status }, context, manage).canEnable, false, `status=${status}`);
  }
  for (const isEnabled of [undefined, null, "false", 0, true]) {
    assert.equal(pricePlanUIActions({ ...safe, isEnabled }, context, manage).canEnable, false, `isEnabled=${isEnabled}`);
  }
  for (const malformed of [
    { kind: undefined },
    { kind: "CORRUPT" },
    { audienceType: undefined },
    { isVisible: undefined },
    { isDefault: undefined }
  ]) {
    assert.equal(pricePlanUIActions({ ...safe, ...malformed }, context, manage).canEnable, false, JSON.stringify(malformed));
  }
});

test("TEST badges and ACTIVE entitlement options are explicit and revision-safe", () => {
  const pricePlanBadges = requiredFunction(domain, "pricePlanBadges");
  const activeEntitlementVersionOptions = requiredFunction(domain, "activeEntitlementVersionOptions");
  const pricePlanEditorEconomicFieldsEditable = requiredFunction(domain, "pricePlanEditorEconomicFieldsEditable");
  assert.deepEqual(pricePlanBadges({ kind: "TEST", isVisible: false, isDefault: false, audienceType: "TEST" }), [
    "测试", "隐藏", "非默认", "白名单限定"
  ]);
  assert.deepEqual(pricePlanBadges({ kind: "TEST", isVisible: true, isDefault: true, audienceType: "PUBLIC" }), [
    "测试", "配置异常：TEST 不得公开", "配置异常：TEST 不得默认", "配置异常：TEST 受众范围无效"
  ]);
  assert.deepEqual(activeEntitlementVersionOptions([
    { id: "active-member", planId: "plan-member", versionNo: 3, status: "ACTIVE", revision: 8 },
    { id: "draft-member", planId: "plan-member", versionNo: 4, status: "DRAFT", revision: 1 },
    { id: "active-agent", planId: "plan-agent", versionNo: 2, status: "ACTIVE", revision: 5 }
  ], "plan-member"), [{ id: "active-member", versionNo: 3, revision: 8 }]);
  assert.equal(pricePlanEditorEconomicFieldsEditable("CREATE", { canEditEconomicFields: false }), true);
  assert.equal(pricePlanEditorEconomicFieldsEditable("EDIT", { canEditEconomicFields: true }), true);
  assert.equal(pricePlanEditorEconomicFieldsEditable("EDIT", { canEditEconomicFields: false }), false);
  assert.equal(pricePlanEditorEconomicFieldsEditable("CLONE", { canEditEconomicFields: true }), false, "clone API accepts only new code/name/reason");

  const ready = { validationValid: true, validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, paymentDataComplete: true };
  const superAdmin = { role: "SUPER_ADMIN", permissions: [] };
  const normalTestActions = domain.pricePlanUIActions({ status: "DRAFT", kind: "TEST", isVisible: false, isDefault: false, audienceType: "TEST", isEnabled: false, giftPoints: 0, healthAvailable: true }, ready, superAdmin);
  assert.equal(normalTestActions.canEnable, true);
  assert.equal(normalTestActions.canManageWhitelist, true);
  const abnormalTestActions = domain.pricePlanUIActions({ status: "DRAFT", kind: "TEST", isVisible: true, isDefault: true, audienceType: "PUBLIC", isEnabled: false, giftPoints: 0, healthAvailable: true }, ready, superAdmin);
  assert.equal(abnormalTestActions.canEnable, false);
  assert.equal(abnormalTestActions.canMakeDefault, false);
  assert.equal(abnormalTestActions.canManageWhitelist, false);
});

test("price-plan codes use stable syntax and reject explicit price semantics without rejecting neutral digits", () => {
  const pricePlanCodeIssue = requiredFunction(domain, "pricePlanCodeIssue");
  assert.equal(pricePlanCodeIssue("member_campaign_v10"), "");
  assert.equal(pricePlanCodeIssue("agent_level_02"), "");
  assert.equal(pricePlanCodeIssue("plan_member_996"), "PRICE_PLAN_CODE_HAS_PRICE");
  assert.equal(pricePlanCodeIssue("member_1yuan"), "PRICE_PLAN_CODE_HAS_PRICE");
  assert.equal(pricePlanCodeIssue("agent_price_996"), "PRICE_PLAN_CODE_HAS_PRICE");
  assert.equal(pricePlanCodeIssue("Member Campaign"), "PRICE_PLAN_CODE_FORMAT_INVALID");
});

test("price-plan names are required after trimming in every editor mode", () => {
  const pricePlanNameIssue = requiredFunction(domain, "pricePlanNameIssue");
  assert.equal(pricePlanNameIssue("会员正常价"), "");
  assert.equal(pricePlanNameIssue(" 会员正常价 "), "");
  assert.equal(pricePlanNameIssue(""), "PRICE_PLAN_NAME_REQUIRED");
  assert.equal(pricePlanNameIssue("   \t\n"), "PRICE_PLAN_NAME_REQUIRED");
  assert.equal(pricePlanNameIssue(undefined), "PRICE_PLAN_NAME_REQUIRED");
});

test("price formatter never fabricates zero for missing or malformed cents", () => {
  const formatPriceCents = requiredFunction(domain, "formatPriceCents");
  assert.equal(formatPriceCents(100), "¥1.00");
  assert.equal(formatPriceCents(0), "¥0.00");
  for (const value of [null, undefined, "", "100", Number.NaN, Number.POSITIVE_INFINITY, -1, 1.5, {}]) {
    assert.equal(formatPriceCents(value), "未知", String(value));
  }
  assert.equal(formatPriceCents(99600, "CNY "), "CNY 996.00");
});

test("default switch preview uses exact group and payment records and fails closed on missing data", () => {
  const buildDefaultPricePlanPreview = requiredFunction(domain, "buildDefaultPricePlanPreview");
  const target = {
    pricePlanId: "new-default", planId: "plan-member", planVersionId: "version-1", kind: "NORMAL", status: "ACTIVE",
    channel: "WECHAT_VIRTUAL", environment: "SANDBOX", currency: "CNY", salePriceCents: 100,
    isDefault: false, isEnabled: true, isVisible: true, audienceType: "PUBLIC", revision: 7, giftPoints: 0
  };
  const old = { ...target, pricePlanId: "old-default", salePriceCents: 99600, isDefault: true, revision: 3 };
  const validation = { pricePlanId: "new-default", valid: true, checkedAt: "2026-07-28T08:00:00Z", paymentBindingId: "binding-1", wechatGoodId: "good-1", wechatProductId: "wx-test-100", pricePlanPriceCents: 100, bindingPriceCents: 100, wechatGoodPriceCents: 100, checks: [] };
  const binding = { id: "binding-1", pricePlanId: "new-default", wechatGoodId: "good-1", wechatProductId: "wx-test-100", channel: "WECHAT_VIRTUAL", environment: "SANDBOX", providerPriceSnapshotCents: 100, pricePlanSalePriceCents: 100, wechatGoodPriceCents: 100, enabled: true, status: "ACTIVE" };
  const good = { id: "good-1", productId: "wx-test-100", offerId: "offer-test", channel: "WECHAT_VIRTUAL", environment: "SANDBOX", platformPriceCents: 100, enabled: true, verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED" };
  const currentDefaultValidation = { pricePlanId: "old-default", valid: true, checkedAt: "2026-07-28T08:00:00Z", paymentBindingId: "binding-old", wechatGoodId: "good-old", wechatProductId: "wx-prod-996", pricePlanPriceCents: 99600, bindingPriceCents: 99600, wechatGoodPriceCents: 99600, checks: [] };
  const currentDefaultBinding = { id: "binding-old", pricePlanId: "old-default", wechatGoodId: "good-old", wechatProductId: "wx-prod-996", channel: "WECHAT_VIRTUAL", environment: "SANDBOX", providerPriceSnapshotCents: 99600, pricePlanSalePriceCents: 99600, wechatGoodPriceCents: 99600, enabled: true, status: "ACTIVE" };
  const currentDefaultGood = { id: "good-old", productId: "wx-prod-996", offerId: "offer-prod", channel: "WECHAT_VIRTUAL", environment: "SANDBOX", platformPriceCents: 99600, enabled: true, verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED" };
  const preview = buildDefaultPricePlanPreview({
    target, plans: [old, target], validation, binding, good, validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true,
    currentDefaultValidation, currentDefaultBinding, currentDefaultGood, currentDefaultValidationFresh: true
  });
  assert.equal(preview.currentDefault?.pricePlanId, "old-default");
  assert.equal(preview.target.pricePlanId, "new-default");
  assert.equal(preview.binding?.id, "binding-1");
  assert.equal(preview.good?.productId, "wx-test-100");
  assert.equal(preview.currentDefaultBinding?.id, "binding-old");
  assert.equal(preview.currentDefaultGood?.productId, "wx-prod-996");
  assert.equal(preview.currentDefaultGood?.platformPriceCents, 99600);
  assert.equal(preview.ready, true);
  assert.deepEqual(preview.blockers, []);
  const missing = buildDefaultPricePlanPreview({ target, plans: [old, target], validationFresh: false, runtimeSafetyKnown: false, v132Blocked: false, targetHealthAvailable: false });
  assert.equal(missing.ready, false);
  assert.ok(missing.blockers.includes("VALIDATION_NOT_FRESH"));
  assert.ok(missing.blockers.includes("RUNTIME_SAFETY_UNKNOWN"));
  assert.ok(missing.blockers.includes("MANAGED_PLAN_REQUIRES_PAYMENT_BINDING"));
  const drifted = buildDefaultPricePlanPreview({
    target: { ...target, kind: "TEST", status: "INACTIVE", isEnabled: false, isVisible: false, audienceType: "TEST" },
    plans: [old], validation, binding, good, validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true
  });
  assert.equal(drifted.ready, false);
  assert.ok(drifted.blockers.includes("PRICE_PLAN_DEFAULT_TEST_FORBIDDEN"));
  assert.ok(drifted.blockers.includes("PRICE_PLAN_DEFAULT_NOT_ACTIVE"));
  assert.ok(drifted.blockers.includes("PRICE_PLAN_DEFAULT_HIDDEN"));
  assert.ok(drifted.blockers.includes("PRICE_PLAN_DEFAULT_AUDIENCE_INVALID"));

  const stitched = buildDefaultPricePlanPreview({
    target, plans: [old, target], validation, binding: { ...binding, wechatGoodId: "good-other" }, good,
    validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true,
    currentDefaultValidation, currentDefaultBinding, currentDefaultGood, currentDefaultValidationFresh: true
  });
  assert.equal(stitched.ready, false);
  assert.ok(stitched.blockers.includes("PRICE_PLAN_CONFIGURATION_CHANGED"));

  const unsafeUnknown = buildDefaultPricePlanPreview({
    target, plans: [target], validation, binding, good,
    validationFresh: true, runtimeSafetyKnown: true, targetHealthAvailable: false
  });
  assert.equal(unsafeUnknown.ready, false);
  assert.ok(unsafeUnknown.blockers.includes("RUNTIME_SAFETY_UNKNOWN"));
  assert.ok(unsafeUnknown.blockers.includes("PRICE_PLAN_HEALTH_UNKNOWN"));
  const invalidGift = buildDefaultPricePlanPreview({
    target: { ...target, giftPoints: undefined }, plans: [target], validation, binding, good,
    validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true
  });
  assert.equal(invalidGift.ready, false);
  assert.ok(invalidGift.blockers.includes("PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE"));

  const badOldValidation = { ...currentDefaultValidation, valid: false, checks: [{ code: "PRICE_PLAN_WECHAT_PRICE_MISMATCH", passed: false }] };
  const switchAwayFromBadOld = buildDefaultPricePlanPreview({
    target, plans: [old, target], validation, binding, good,
    validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true,
    currentDefaultValidation: badOldValidation, currentDefaultBinding, currentDefaultGood, currentDefaultValidationFresh: true
  });
  assert.equal(switchAwayFromBadOld.ready, true, "an unhealthy old default must not prevent switching to a healthy target");
  assert.ok(switchAwayFromBadOld.warnings.includes("CURRENT_DEFAULT_CONFIGURATION_INVALID"));

  const incompleteJoinedBinding = buildDefaultPricePlanPreview({
    target, plans: [target], validation,
    binding: { ...binding, pricePlanSalePriceCents: undefined, wechatGoodPriceCents: undefined }, good,
    validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false, targetHealthAvailable: true
  });
  assert.equal(incompleteJoinedBinding.ready, false, "missing joined binding snapshots cannot be inferred from other objects");
  assert.ok(incompleteJoinedBinding.blockers.includes("PRICE_PLAN_CONFIGURATION_CHANGED"));
});

test("price-plan display facts expose every operator-required field from exact merged records", () => {
  const pricePlanDisplayFacts = requiredFunction(domain, "pricePlanDisplayFacts");
  assert.deepEqual(pricePlanDisplayFacts({
    kind: "PROMOTION", planVersionId: "version-8", audienceType: "PUBLIC",
    isVisible: true, isEnabled: true, isDefault: false,
    validFrom: "2026-07-28T00:00:00Z", validUntil: "2026-08-28T00:00:00Z",
    wechatGood: { productId: "wx-product-8", platformPriceCents: 8800 }, revision: 9
  }), {
    kind: "PROMOTION", planVersionId: "version-8", audienceType: "PUBLIC",
    isVisible: true, isEnabled: true, isDefault: false,
    validFrom: "2026-07-28T00:00:00Z", validUntil: "2026-08-28T00:00:00Z",
    wechatProductId: "wx-product-8", wechatGoodPriceCents: 8800, revision: 9
  });
});

test("price-plan display facts preserve unknown booleans instead of fabricating no", () => {
  const pricePlanDisplayFacts = requiredFunction(domain, "pricePlanDisplayFacts");
  const facts = pricePlanDisplayFacts({ isVisible: undefined, isEnabled: "false", isDefault: null });
  assert.equal(facts.isVisible, null);
  assert.equal(facts.isEnabled, null);
  assert.equal(facts.isDefault, null);
});

test("legacy pricing health without pricePlans is UNKNOWN and blocks default preview", () => {
  const pricingHealthPricePlanFact = requiredFunction(domain, "pricingHealthPricePlanFact");
  const buildDefaultPricePlanPreview = requiredFunction(domain, "buildDefaultPricePlanPreview");
  assert.deepEqual(pricingHealthPricePlanFact({ runtime: { v132Blocked: false } }, "price-1"), {
    available: false,
    status: "UNKNOWN"
  });
  assert.deepEqual(pricingHealthPricePlanFact({ pricePlans: null }, "price-1"), {
    available: false,
    status: "UNKNOWN"
  });
  assert.deepEqual(pricingHealthPricePlanFact({ pricePlans: [{ pricePlanId: "price-1", status: "HEALTHY" }] }, "price-1"), {
    available: true,
    status: "HEALTHY"
  });
  const preview = buildDefaultPricePlanPreview({
    target: { pricePlanId: "price-1", kind: "NORMAL", status: "ACTIVE", isEnabled: true, isVisible: true, audienceType: "PUBLIC", giftPoints: 0 },
    plans: [], validationFresh: true, runtimeSafetyKnown: true, v132Blocked: false,
    targetHealthAvailable: pricingHealthPricePlanFact({ runtime: { v132Blocked: false } }, "price-1").available
  });
  assert.equal(preview.ready, false);
  assert.ok(preview.blockers.includes("PRICE_PLAN_HEALTH_UNKNOWN"));
});

test("default-switch refresh treats the full dependency round as one freshness gate", () => {
  const defaultSwitchRefreshGate = requiredFunction(domain, "defaultSwitchRefreshGate");
  const defaultSwitchCanSubmit = requiredFunction(domain, "defaultSwitchCanSubmit");
  const allFresh = defaultSwitchRefreshGate([
    { status: "fulfilled" }, { status: "fulfilled" }, { status: "fulfilled" }, { status: "fulfilled" }
  ]);
  assert.deepEqual(allFresh, { complete: true, validationFresh: true, runtimeSafetyKnown: true });
  const oneFailed = defaultSwitchRefreshGate([
    { status: "fulfilled" }, { status: "rejected", reason: new Error("old binding failed") }, { status: "fulfilled" }
  ]);
  assert.deepEqual(oneFailed, { complete: false, validationFresh: false, runtimeSafetyKnown: false });
  const ready = { permission: true, previewReady: true, secondConfirmed: true, hasReason: true, loading: false, refreshComplete: true, loadError: "" };
  assert.equal(defaultSwitchCanSubmit(ready), true);
  assert.equal(defaultSwitchCanSubmit({ ...ready, loadError: "旧默认绑定加载失败" }), false, "loadError must block stale cached preview submission");
  assert.equal(defaultSwitchCanSubmit({ ...ready, refreshComplete: false }), false);
});

test("validation and binding inspections never present failed current loads as fresh cache", () => {
  const pricingInspectionState = requiredFunction(domain, "pricingInspectionState");
  assert.equal(pricingInspectionState({ loading: true, fresh: false, error: "", hasCached: true }), "LOADING");
  assert.equal(pricingInspectionState({ loading: false, fresh: true, error: "", hasCached: true }), "FRESH");
  assert.equal(pricingInspectionState({ loading: false, fresh: false, error: "timeout", hasCached: true }), "STALE_ERROR");
  assert.equal(pricingInspectionState({ loading: false, fresh: false, error: "timeout", hasCached: false }), "ERROR");
  assert.equal(pricingInspectionState({ loading: false, fresh: false, error: "", hasCached: true }), "STALE");
});

test("a server-confirmed mutation with refresh failure locks form resubmission", () => {
  const pricePlanMutationSubmitAllowed = requiredFunction(domain, "pricePlanMutationSubmitAllowed");
  assert.equal(pricePlanMutationSubmitAllowed({ allowedByPolicy: true, saving: false, committedStale: false }), true);
  assert.equal(pricePlanMutationSubmitAllowed({ allowedByPolicy: true, saving: false, committedStale: true }), false);
  assert.equal(pricePlanMutationSubmitAllowed({ allowedByPolicy: true, saving: true, committedStale: false }), false);
  const readyDefault = { permission: true, previewReady: true, secondConfirmed: true, hasReason: true, loading: false, refreshComplete: true, loadError: "", committedStale: false };
  assert.equal(domain.defaultSwitchCanSubmit(readyDefault), true);
  assert.equal(domain.defaultSwitchCanSubmit({ ...readyDefault, committedStale: true }), false);
});

test("three-price, channel, environment, confirmation, and binding validation gates enable/default", () => {
  const paymentValidationState = requiredFunction(domain, "paymentValidationState");
  const plan = { salePriceCents: 100, channel: "WECHAT_VIRTUAL", environment: "SANDBOX" };
  const binding = {
    providerPriceSnapshotCents: 100,
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    enabled: true,
    status: "ACTIVE"
  };
  const good = {
    platformPriceCents: 100,
    channel: "WECHAT_VIRTUAL",
    environment: "SANDBOX",
    enabled: true,
    verificationStatus: "MANUALLY_CONFIRMED_PUBLISHED"
  };
  assert.deepEqual(paymentValidationState(plan, binding, good), { valid: true, blockers: [] });
  assert.deepEqual(
    paymentValidationState(plan, { ...binding, providerPriceSnapshotCents: 99 }, good),
    { valid: false, blockers: ["PRICE_PLAN_WECHAT_PRICE_MISMATCH"] }
  );
  assert.deepEqual(
    paymentValidationState(plan, binding, { ...good, environment: "PRODUCTION" }),
    { valid: false, blockers: ["PRICE_PLAN_PAYMENT_ENV_MISMATCH"] }
  );
  assert.deepEqual(
    paymentValidationState(plan, binding, { ...good, verificationStatus: "VERIFICATION_EXPIRED" }),
    { valid: false, blockers: ["WECHAT_GOOD_VERIFICATION_EXPIRED"] }
  );
  assert.deepEqual(
    paymentValidationState(plan, undefined, good),
    { valid: false, blockers: ["MANAGED_PLAN_REQUIRES_PAYMENT_BINDING"] }
  );
});

test("health status and all Phase 2F issue codes have operator-facing labels", () => {
  const healthStatusLabel = requiredFunction(domain, "healthStatusLabel");
  const healthIssueLabel = requiredFunction(domain, "healthIssueLabel");
  assert.equal(healthStatusLabel("HEALTHY"), "正常");
  assert.equal(healthStatusLabel("DEGRADED"), "需关注");
  assert.equal(healthStatusLabel("BLOCKED"), "已阻断");
  const codes = [
    "ENTITLEMENT_VERSION_MISSING",
    "PRICE_PLAN_MISSING",
    "DEFAULT_PRICE_PLAN_MISSING",
    "WECHAT_GOOD_NOT_CONFIRMED",
    "WECHAT_GOOD_VERIFICATION_EXPIRED",
    "PAYMENT_BINDING_MISSING",
    "PRICE_PLAN_WECHAT_PRICE_MISMATCH",
    "PRICE_PLAN_PAYMENT_ENV_MISMATCH",
    "TEST_WHITELIST_MISSING",
    "V132_BLOCKED",
    "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE",
    "DISABLED"
  ];
  for (const code of codes) assert.doesNotMatch(healthIssueLabel(code), /未知|undefined/i, code);
});

test("TEST whitelist terminal states cannot be edited or restored", () => {
  const whitelistEntryActions = requiredFunction(domain, "whitelistEntryActions");
  assert.deepEqual(whitelistEntryActions({ status: "PENDING" }), { canEdit: true, canDisable: true, requiresNewEntry: false });
  assert.deepEqual(whitelistEntryActions({ status: "ACTIVE" }), { canEdit: true, canDisable: true, requiresNewEntry: false });
  assert.deepEqual(whitelistEntryActions({ status: "EXPIRED" }), { canEdit: false, canDisable: false, requiresNewEntry: true });
  assert.deepEqual(whitelistEntryActions({ status: "DISABLED" }), { canEdit: false, canDisable: false, requiresNewEntry: true });
  assert.equal(
    domain.TEST_WHITELIST_ORDINARY_ENTRY_NOTICE,
    "加入白名单不会改变普通购买价格，用户仍需通过专用测试入口。"
  );
});

test("entitlement payload builders exclude prices and product identifiers", () => {
  const buildPlanVersionPayload = requiredFunction(domain, "buildPlanVersionPayload");
  const payload = buildPlanVersionPayload({
    revision: 4,
    memberLevel: "VIP",
    tokenAmount: 200,
    pointsAmount: 0,
    durationDays: 365,
    rightsSnapshot: { export: true },
    commissionRuleVersion: "r2",
    commissionSnapshot: { rate: 10 },
    changeReason: "调整会员权益",
    salePriceCents: 1,
    platformPriceCents: 1,
    productId: "forged-product"
  });
  assert.deepEqual(payload, {
    revision: 4,
    memberLevel: "VIP",
    tokenAmount: 200,
    pointsAmount: 0,
    durationDays: 365,
    rightsSnapshot: { export: true },
    commissionRuleVersion: "r2",
    commissionSnapshot: { rate: 10 },
    reason: "调整会员权益"
  });
});

test("entitlement create omits revision while update and transitions send the listed revision", async () => {
  const apiClient = client.apiClient;
  const api = apiModule.pricePlanAdminApi;
  const originalAdapter = apiClient.defaults.adapter;
  const requests = [];
  try {
    apiClient.defaults.adapter = async (config) => {
      requests.push({
        method: String(config.method).toUpperCase(),
        url: config.url,
        data: typeof config.data === "string" ? JSON.parse(config.data) : config.data
      });
      return {
        data: { item: { id: "version-1", planId: "plan-1", revision: 8 } },
        status: 200,
        statusText: "OK",
        headers: {},
        config
      };
    };
    const fields = {
      revision: 999,
      memberLevel: "PRO",
      tokenAmount: 100,
      pointsAmount: 5,
      durationDays: 30,
      rightsSnapshot: { export: true },
      commissionRuleVersion: "commission-v1",
      commissionSnapshot: { rules: [] },
      changeReason: "权益版本变更"
    };
    await api.createPlanVersion("plan/1", fields);
    await api.updatePlanVersion("version/1", { ...fields, revision: 7 });
    await api.activatePlanVersion("version/1", { revision: 7, changeReason: "激活权益版本" });
    await api.retirePlanVersion("version/1", { revision: 8, changeReason: "退休权益版本" });

    assert.deepEqual(requests, [
      {
        method: "POST",
        url: "/admin/business-plans/plan%2F1/versions",
        data: {
          memberLevel: "PRO",
          tokenAmount: 100,
          pointsAmount: 5,
          durationDays: 30,
          rightsSnapshot: { export: true },
          commissionRuleVersion: "commission-v1",
          commissionSnapshot: { rules: [] },
          reason: "权益版本变更"
        }
      },
      {
        method: "PATCH",
        url: "/admin/plan-versions/version%2F1",
        data: {
          revision: 7,
          memberLevel: "PRO",
          tokenAmount: 100,
          pointsAmount: 5,
          durationDays: 30,
          rightsSnapshot: { export: true },
          commissionRuleVersion: "commission-v1",
          commissionSnapshot: { rules: [] },
          reason: "权益版本变更"
        }
      },
      {
        method: "POST",
        url: "/admin/plan-versions/version%2F1/activate",
        data: { revision: 7, reason: "激活权益版本" }
      },
      {
        method: "POST",
        url: "/admin/plan-versions/version%2F1/retire",
        data: { revision: 8, reason: "退休权益版本" }
      }
    ]);
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("price-plan payload builders preserve allowed amounts but never accept a direct WeChat product", () => {
  const buildPricePlanCreatePayload = requiredFunction(domain, "buildPricePlanCreatePayload");
  const payload = buildPricePlanCreatePayload({
    revision: 6,
    planVersionId: "version-1",
    code: "member_normal",
    name: "会员正常价",
    kind: "NORMAL",
    channel: "WECHAT_VIRTUAL",
    environment: "PRODUCTION",
    currency: "CNY",
    salePriceCents: 99600,
    listPriceCents: 99600,
    giftPoints: 0,
    giftTokens: 100,
    audienceType: "PUBLIC",
    audienceRule: {},
    isVisible: true,
    changeReason: "创建正式方案",
    productId: "forged-product",
    wechatGoodId: "forged-good"
  });
  assert.deepEqual(payload, {
    revision: 6,
    planVersionId: "version-1",
    code: "member_normal",
    name: "会员正常价",
    kind: "NORMAL",
    channel: "WECHAT_VIRTUAL",
    environment: "PRODUCTION",
    currency: "CNY",
    salePriceCents: 99600,
    listPriceCents: 99600,
    giftPoints: 0,
    giftTokens: 100,
    audienceType: "PUBLIC",
    audienceRule: {},
    isVisible: true,
    changeReason: "创建正式方案"
  });
});

test("transition and separated binding payloads require revision/reason and reject client price injection", () => {
  const buildPricePlanTransitionPayload = requiredFunction(domain, "buildPricePlanTransitionPayload");
  const buildVersionTransitionPayload = requiredFunction(domain, "buildVersionTransitionPayload");
  const buildPaymentBindingCreatePayload = requiredFunction(domain, "buildPaymentBindingCreatePayload");
  const buildPaymentBindingRebindPayload = requiredFunction(domain, "buildPaymentBindingRebindPayload");
  const buildPaymentBindingTransitionPayload = requiredFunction(domain, "buildPaymentBindingTransitionPayload");
  assert.deepEqual(buildPricePlanTransitionPayload({ revision: 7, changeReason: "启用新方案", salePriceCents: 1 }), {
    revision: 7,
    changeReason: "启用新方案"
  });
  assert.deepEqual(buildVersionTransitionPayload({ revision: 3, changeReason: "激活新权益", productId: "forged" }), {
    revision: 3,
    reason: "激活新权益"
  });
  assert.deepEqual(buildPaymentBindingCreatePayload({ wechatGoodId: "good-1", changeReason: "绑定商品", salePriceCents: 1, productId: "forged" }), {
    wechatGoodId: "good-1",
    reason: "绑定商品"
  });
  assert.deepEqual(buildPaymentBindingRebindPayload({ revision: 5, enabled: true, wechatGoodId: "good-2", changeReason: "换绑", platformPriceCents: 1, productId: "forged" }), {
    revision: 5,
    wechatGoodId: "good-2",
    reason: "换绑"
  });
  assert.deepEqual(buildPaymentBindingTransitionPayload({ revision: 5, enabled: true, wechatGoodId: "good-2", changeReason: "启用", platformPriceCents: 1, productId: "forged" }), {
    revision: 5,
    enabled: true,
    reason: "启用"
  });
});

test("whitelist payloads always include revision and changeReason without unrelated payment fields", () => {
  const buildWhitelistCreatePayload = requiredFunction(domain, "buildWhitelistCreatePayload");
  const buildWhitelistUpdatePayload = requiredFunction(domain, "buildWhitelistUpdatePayload");
  const buildWhitelistDisablePayload = requiredFunction(domain, "buildWhitelistDisablePayload");
  assert.deepEqual(buildWhitelistCreatePayload({ revision: 0, userId: "user-1", reason: "验收测试", validUntil: "2026-08-01T00:00:00Z", changeReason: "新增测试账号", salePriceCents: 1 }), {
    revision: 0,
    userId: "user-1",
    reason: "验收测试",
    validUntil: "2026-08-01T00:00:00Z",
    changeReason: "新增测试账号"
  });
  assert.deepEqual(buildWhitelistUpdatePayload({ revision: 2, reason: "延长验收", clearValidFrom: true, changeReason: "调整有效期", productId: "forged" }), {
    revision: 2,
    reason: "延长验收",
    clearValidFrom: true,
    changeReason: "调整有效期"
  });
  assert.deepEqual(buildWhitelistDisablePayload({ revision: 3, changeReason: "验收结束", userId: "forged" }), {
    revision: 3,
    changeReason: "验收结束"
  });
});

test("pricing store preserves loaded page data when a refresh fails", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  assert.ok(api, "pricePlanAdminApi must be implemented");
  const original = api.listBusinessPlans;
  const items = [{ id: "plan-1", code: "plan_member", name: "会员", businessType: "MEMBER", legacyCode: false, codeReadOnly: true, active: true }];
  try {
    api.listBusinessPlans = async () => ({ items, total: 1 });
    await store.loadBusinessPlans();
    api.listBusinessPlans = async () => { throw new Error("network down"); };
    await assert.rejects(store.loadBusinessPlans(), /network down/);
    assert.deepEqual(store.businessPlans, items);
    assert.match(store.errors.businessPlans.message, /操作失败|network down/);
  } finally {
    api.listBusinessPlans = original;
  }
});

test("pricing mutations do not reload or optimistically overwrite cached data before server success", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  assert.ok(api, "pricePlanAdminApi must be implemented");
  const originalDefault = api.makeDefaultPricePlan;
  const originalList = api.listPricePlans;
  const oldDefault = { pricePlanId: "price-old", planId: "plan-1", isDefault: true, revision: 2 };
  const target = { pricePlanId: "price-new", planId: "plan-1", isDefault: false, revision: 4 };
  store.pricePlansByPlanId["plan-1"] = [oldDefault, target];
  let resolveDefault;
  let reloads = 0;
  try {
    api.makeDefaultPricePlan = () => new Promise((resolve) => { resolveDefault = resolve; });
    api.listPricePlans = async () => {
      reloads += 1;
      return { items: [{ ...oldDefault, isDefault: false }, { ...target, isDefault: true, revision: 5 }], total: 2 };
    };
    const pending = store.makeDefaultPricePlan("price-new", { revision: 4, changeReason: "切换默认" });
    await Promise.resolve();
    assert.equal(store.pricePlansByPlanId["plan-1"][0].isDefault, true);
    assert.equal(reloads, 0);
    resolveDefault({ item: { ...target, isDefault: true, revision: 5 }, alreadyDefault: false });
    await pending;
    assert.equal(reloads, 1);
    assert.equal(store.pricePlansByPlanId["plan-1"][1].isDefault, true);
  } finally {
    api.makeDefaultPricePlan = originalDefault;
    api.listPricePlans = originalList;
  }
});

test("price-plan writes refresh every decision resource and distinguish post-write refresh failure", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originals = {
    enablePricePlan: api.enablePricePlan,
    listPricePlans: api.listPricePlans,
    getPricePlan: api.getPricePlan,
    validatePricePlan: api.validatePricePlan,
    getPricingHealth: api.getPricingHealth,
    listPaymentBindings: api.listPaymentBindings,
    listWechatVirtualGoods: api.listWechatVirtualGoods
  };
  const calls = [];
  try {
    api.enablePricePlan = async () => ({ item: { pricePlanId: "price-1", planId: "plan-1", revision: 2 } });
    api.listPricePlans = async (planId) => { calls.push(`list:${planId}`); return { items: [], total: 0 }; };
    api.getPricePlan = async (pricePlanId) => { calls.push(`detail:${pricePlanId}`); return { item: { pricePlanId, planId: "plan-1", revision: 2 } }; };
    api.validatePricePlan = async (pricePlanId) => { calls.push(`validation:${pricePlanId}`); return { pricePlanId, valid: true, checkedAt: "now", checks: [] }; };
    api.getPricingHealth = async () => { calls.push("health"); return { checkedAt: "now", status: "HEALTHY", summary: {}, issues: [], businessPlans: [], pricePlans: [], wechatGoods: [], runtime: { v132Blocked: false } }; };
    api.listPaymentBindings = async (pricePlanId) => { calls.push(`bindings:${pricePlanId}`); return { items: [], total: 0 }; };
    api.listWechatVirtualGoods = async () => { calls.push("goods"); return { items: [], total: 0, verificationSource: "LOCAL_MANUAL_ONLY" }; };

    await store.enablePricePlan("price-1", { revision: 1, changeReason: "启用新方案" });
    assert.deepEqual(new Set(calls), new Set([
      "list:plan-1", "detail:price-1", "validation:price-1", "health", "bindings:price-1", "goods"
    ]));
    assert.equal(store.refreshWarnings["enablePricePlan:price-1"], undefined);

    calls.length = 0;
    api.getPricingHealth = async () => { calls.push("health"); throw new Error("health refresh down"); };
    const result = await store.enablePricePlan("price-1", { revision: 2, changeReason: "幂等启用" });
    assert.equal(result.item.pricePlanId, "price-1", "a successful write must not be reported as failed");
    assert.match(store.refreshWarnings["enablePricePlan:price-1"].message, /操作失败|health refresh down/);
    assert.equal(store.errors["enablePricePlan:price-1"], undefined, "write error and refresh warning are separate states");
  } finally {
    Object.assign(api, originals);
  }
});

test("a committed price-plan clone keeps a persistent recovery gate across dialog reopen", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originalClone = api.clonePricePlan;
  const originalRefresh = store.refreshPricePlanDecisionResources;
  const key = "clonePricePlan:price-source";
  let cloneCalls = 0;
  try {
    api.clonePricePlan = async () => {
      cloneCalls += 1;
      return { item: { pricePlanId: "price-clone", planId: "plan-1", revision: 1 } };
    };
    store.refreshPricePlanDecisionResources = async () => { throw new Error("refresh unavailable"); };

    const result = await store.clonePricePlan("price-source", { revision: 7, code: "plan_promo", name: "Promo", changeReason: "new campaign" });
    assert.equal(result.item.pricePlanId, "price-clone");
    assert.equal(cloneCalls, 1);
    assert.deepEqual(store.pricePlanRefreshGatesByMutationKey[key], {
      mutationKey: key,
      planId: "plan-1",
      pricePlanId: "price-clone",
      revision: 1
    });

    store.clearRefreshWarning(key);
    assert.ok(store.refreshWarnings[key], "a dialog reopen must not clear a committed-write recovery warning");
    await assert.rejects(
      store.clonePricePlan("price-source", { revision: 7, code: "plan_promo_retry", name: "Promo retry", changeReason: "retry" }),
      (error) => error?.code === "PRICE_PLAN_REFRESH_REQUIRED"
    );
    assert.equal(cloneCalls, 1, "the second clone must be blocked before transport");

    store.refreshPricePlanDecisionResources = async () => {
      store.pricePlanById["price-clone"] = { pricePlanId: "price-clone", planId: "plan-1", revision: 1 };
    };
    await store.recoverPricePlanMutation(key);
    assert.equal(store.pricePlanRefreshGatesByMutationKey[key], undefined);
    assert.equal(store.refreshWarnings[key], undefined);

    const editorSource = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PricePlanEditorDialog.vue", import.meta.url), "utf8");
    assert.match(editorSource, /pricePlanRefreshGatesByMutationKey\[mutationKey\.value\]/);
    assert.match(editorSource, /recoverPricePlanMutation\(mutationKey\.value\)/);
  } finally {
    api.clonePricePlan = originalClone;
    store.refreshPricePlanDecisionResources = originalRefresh;
  }
});

test("plan-version reads are latest-wins and committed writes require exact recovery", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originals = {
    listPlanVersions: api.listPlanVersions,
    createPlanVersion: api.createPlanVersion,
    refreshPlanVersionResources: store.refreshPlanVersionResources
  };
  let resolveOld;
  let resolveNew;
  try {
    api.listPlanVersions = () => new Promise((resolve) => {
      if (!resolveOld) resolveOld = resolve;
      else resolveNew = resolve;
    });
    const oldRequest = store.loadPlanVersions("plan-1");
    const newRequest = store.loadPlanVersions("plan-1");
    resolveNew({ items: [{ id: "version-new", planId: "plan-1", revision: 2 }], total: 1 });
    await newRequest;
    resolveOld({ items: [{ id: "version-old", planId: "plan-1", revision: 1 }], total: 1 });
    await oldRequest;
    assert.equal(store.planVersionsByPlanId["plan-1"][0].id, "version-new");

    const key = "createPlanVersion:plan-1";
    let createCalls = 0;
    api.createPlanVersion = async () => {
      createCalls += 1;
      return { item: { id: "version-created", planId: "plan-1", revision: 1 } };
    };
    store.refreshPlanVersionResources = async () => { throw new Error("version refresh unavailable"); };
    const result = await store.createPlanVersion("plan-1", { changeReason: "new draft" });
    assert.equal(result.item.id, "version-created");
    assert.deepEqual(store.planVersionRefreshGatesByMutationKey[key], {
      mutationKey: key,
      planId: "plan-1",
      planVersionId: "version-created",
      revision: 1,
      includeBusinessPlan: false
    });
    store.clearRefreshWarning(key);
    assert.ok(store.refreshWarnings[key]);
    await assert.rejects(
      store.createPlanVersion("plan-1", { changeReason: "duplicate draft" }),
      (error) => error?.code === "PLAN_VERSION_REFRESH_REQUIRED"
    );
    assert.equal(createCalls, 1);

    store.refreshPlanVersionResources = async () => {
      store.planVersionsByPlanId["plan-1"] = [{ id: "version-created", planId: "plan-1", revision: 1 }];
    };
    await store.recoverPlanVersionMutation(key);
    assert.equal(store.planVersionRefreshGatesByMutationKey[key], undefined);
    assert.equal(store.refreshWarnings[key], undefined);

    const managerSource = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PlanVersionManager.vue", import.meta.url), "utf8");
    assert.match(managerSource, /planVersionRefreshGatesByMutationKey/);
    assert.match(managerSource, /recoverPlanVersionMutation/);
  } finally {
    api.listPlanVersions = originals.listPlanVersions;
    api.createPlanVersion = originals.createPlanVersion;
    store.refreshPlanVersionResources = originals.refreshPlanVersionResources;
  }
});

test("default-price switch performs its authoritative reload on the first conditional mount", () => {
  const source = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/DefaultPricePlanSwitchDialog.vue", import.meta.url), "utf8");
  assert.match(source, /watch\(\(\) => props\.modelValue,[\s\S]*?\{\s*immediate:\s*true\s*\}\s*\)/);
});

test("whitelist list filters serialize only supported status, user, and pagination fields", () => {
  const buildWhitelistQuery = requiredFunction(apiModule, "buildWhitelistQuery");
  assert.equal(
    buildWhitelistQuery({ status: "ACTIVE", userId: " user-1 ", page: 2, pageSize: 20, pricePlanId: "forged" }),
    "?status=ACTIVE&userId=user-1&page=2&pageSize=20"
  );
  assert.equal(buildWhitelistQuery({}), "?page=1&pageSize=50");
});

test("successful mutations reload the server-reported affected resource even when it was not cached", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originalTransition = api.transitionPaymentBinding;
  const originalRefresh = store.refreshPaymentBindingDecisionResources;
  let reloadedPricePlanId = "";
  try {
    api.transitionPaymentBinding = async () => ({ item: { id: "binding-1", pricePlanId: "price-uncached", wechatGoodId: "good-1" } });
    store.refreshPaymentBindingDecisionResources = async (pricePlanId) => {
      reloadedPricePlanId = pricePlanId;
    };
    await store.transitionPaymentBinding("binding-1", { revision: 1, enabled: true, changeReason: "启用绑定" });
    assert.equal(reloadedPricePlanId, "price-uncached");
  } finally {
    api.transitionPaymentBinding = originalTransition;
    store.refreshPaymentBindingDecisionResources = originalRefresh;
  }
});

test("whitelist mutations refresh the current server page and never fall back to an unpaged legacy request", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originalCreate = api.createWhitelistEntry;
  const originalList = api.listWhitelist;
  let capturedFilters;
  try {
    store.whitelistFiltersByPricePlanId["price-test"] = { status: "ACTIVE", userId: "user-1", page: 3, pageSize: 20 };
    api.createWhitelistEntry = async () => ({ item: { whitelistEntryId: "entry-2", pricePlanId: "price-test" } });
    api.listWhitelist = async (_pricePlanId, filters) => {
      capturedFilters = filters;
      return { items: [], total: 0, page: filters.page, pageSize: filters.pageSize };
    };
    await store.createWhitelistEntry("price-test", {
      revision: 0,
      userId: "user-2",
      reason: "测试",
      changeReason: "增加白名单"
    });
    assert.deepEqual(capturedFilters, { status: "ACTIVE", userId: "user-1", page: 3, pageSize: 20 });
  } finally {
    api.createWhitelistEntry = originalCreate;
    api.listWhitelist = originalList;
  }
});

test("pricing API uses the unified Axios adapter with exact method, endpoint, query, and allowlisted payload", async () => {
  const apiClient = client.apiClient;
  const api = apiModule.pricePlanAdminApi;
  assert.ok(apiClient && api, "pricing API and unified client must be available");
  const originalAdapter = apiClient.defaults.adapter;
  const requests = [];
  try {
    apiClient.defaults.adapter = async (config) => {
      requests.push({
        method: String(config.method).toUpperCase(),
        url: config.url,
        data: typeof config.data === "string" ? JSON.parse(config.data) : config.data
      });
      return { data: { items: [], total: 0, page: 1, pageSize: 50 }, status: 200, statusText: "OK", headers: {}, config };
    };
    await api.listWhitelist("price/test", { status: "ACTIVE", userId: "user-1", page: 2, pageSize: 25 });
    await api.createPaymentBinding("price/test", {
      wechatGoodId: "good-1",
      changeReason: "绑定商品",
      salePriceCents: 1,
      productId: "forged-product"
    });
    assert.deepEqual(requests, [
      {
        method: "GET",
        url: "/admin/price-plans/price%2Ftest/whitelist?status=ACTIVE&userId=user-1&page=2&pageSize=25",
        data: undefined
      },
      {
        method: "POST",
        url: "/admin/price-plans/price%2Ftest/payment-bindings",
        data: { wechatGoodId: "good-1", reason: "绑定商品" }
      }
    ]);
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("unified Axios response interception preserves stable backend code, status, and payload", async () => {
  const apiClient = client.apiClient;
  const api = apiModule.pricePlanAdminApi;
  const originalAdapter = apiClient.defaults.adapter;
  const payload = {
    code: "PRICE_PLAN_WECHAT_PRICE_MISMATCH",
    message: "prices differ",
    details: { pricePlanPriceCents: 100, wechatGoodPriceCents: 99 }
  };
  try {
    apiClient.defaults.adapter = async (config) => Promise.reject({
      config,
      message: "Request failed with status code 422",
      response: { status: 422, data: payload, headers: {}, config }
    });
    await assert.rejects(api.validatePricePlan("price-1"), (error) => {
      assert.equal(error.name, "AdminApiError");
      assert.equal(error.code, "PRICE_PLAN_WECHAT_PRICE_MISMATCH");
      assert.equal(error.status, 422);
      assert.deepEqual(error.payload, payload);
      return true;
    });
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("pricing navigation keeps pricing permissions exact and fails closed to an authorized fallback", () => {
  const canAccessAdminModule = requiredFunction(governance, "canAccessAdminModule");
  const resolveAuthorizedAdminModule = requiredFunction(governance, "resolveAuthorizedAdminModule");
  const legacyAdmin = { role: "ADMIN", permissions: ["admin.full"] };
  const pricingViewer = { role: "FINANCE", permissions: ["pricing:plan:view"] };
  const superAdmin = { role: "SUPER_ADMIN", permissions: [] };

  assert.equal(canAccessAdminModule(legacyAdmin, "pricing:plan:view"), false);
  assert.equal(canAccessAdminModule(legacyAdmin, "enterprise:list"), true);
  assert.equal(canAccessAdminModule(pricingViewer, "pricing:plan:view"), true);
  assert.equal(canAccessAdminModule(pricingViewer, "pricing:price-plan:manage"), false);
  assert.equal(canAccessAdminModule(superAdmin, "pricing:plan:view"), true);

  const navigation = {
    requestedModuleId: "pricePlanGovernance",
    fallbackModuleId: "analysis",
    allowedModuleIds: ["analysis", "pricePlanGovernance"],
    modulePermissions: { analysis: "", pricePlanGovernance: "pricing:plan:view" }
  };
  assert.equal(resolveAuthorizedAdminModule({ ...navigation, principal: legacyAdmin }), "analysis");
  assert.equal(resolveAuthorizedAdminModule({ ...navigation, principal: pricingViewer }), "pricePlanGovernance");
  assert.equal(resolveAuthorizedAdminModule({ ...navigation, principal: superAdmin }), "pricePlanGovernance");
  assert.equal(resolveAuthorizedAdminModule({ ...navigation, requestedModuleId: "unknown", principal: superAdmin }), "analysis");
});

test("business-plan rows include only V2 member and agent plans and keep codes read-only", () => {
  const filterBusinessPlanRows = requiredFunction(governance, "filterBusinessPlanRows");
  const plans = [
    { id: "member-1", code: "plan_member", name: "会员套餐", businessType: "MEMBER", legacyCode: false, codeReadOnly: false, active: true, activeVersionId: "version-member" },
    { id: "agent-1", code: "plan_agent_join_996", name: "代理商套餐", businessType: "AGENT", legacyCode: true, codeReadOnly: true, active: true, activeVersionId: "version-agent" },
    { id: "center-1", code: "plan_operation_center_5000", name: "运营中心", businessType: "OPERATION_CENTER", legacyCode: true, codeReadOnly: true, active: true }
  ];
  const health = [
    { planId: "member-1", name: "会员套餐", status: "HEALTHY", issueCodes: [], activeVersionId: "version-member", pricePlanCount: 2, defaults: { production: { pricePlanId: "price-member-prod", salePriceCents: 99600, currency: "CNY" }, sandbox: null } },
    { planId: "agent-1", name: "代理商套餐", status: "BLOCKED", issueCodes: ["PAYMENT_BINDING_MISSING"], activeVersionId: "version-agent", pricePlanCount: 1, defaults: { production: null, sandbox: { pricePlanId: "price-agent-test", salePriceCents: 100, currency: "CNY" } } }
  ];

  const rows = filterBusinessPlanRows(plans, health, { keyword: "套餐", businessType: "ALL", status: "ALL" });
  assert.deepEqual(rows.map((row) => row.id), ["member-1", "agent-1"]);
  assert.equal(rows[0].code, "plan_member");
  assert.equal(rows[0].codeReadOnly, true);
  assert.equal(rows[0].activeVersionId, "version-member");
  assert.equal(rows[0].productionDefault.pricePlanId, "price-member-prod");
  assert.equal(rows[1].legacyCode, true);
  assert.equal(rows[1].healthStatus, "BLOCKED");
  assert.equal(rows[1].sandboxDefault.salePriceCents, 100);
  assert.deepEqual(
    filterBusinessPlanRows(plans, health, { keyword: "agent_join", businessType: "AGENT", status: "BLOCKED" }).map((row) => row.id),
    ["agent-1"]
  );
});

test("business-plan list distinguishes first-load errors from a legitimate empty result", () => {
  const businessPlanListDisplayState = requiredFunction(governance, "businessPlanListDisplayState");
  assert.equal(businessPlanListDisplayState({ initialLoading: true, error: "", rowCount: 0 }), "LOADING");
  assert.equal(businessPlanListDisplayState({ initialLoading: false, error: "forbidden", rowCount: 0 }), "ERROR");
  assert.equal(businessPlanListDisplayState({ initialLoading: false, error: "refresh failed", rowCount: 2 }), "TABLE");
  assert.equal(businessPlanListDisplayState({ initialLoading: false, error: "", rowCount: 0 }), "EMPTY");
  assert.equal(businessPlanListDisplayState({ initialLoading: false, error: "", rowCount: 2 }), "TABLE");
});

test("pricing health cards use the four backend-owned summary counts without recomputing health", () => {
  const buildPricingHealthCards = requiredFunction(governance, "buildPricingHealthCards");
  assert.deepEqual(buildPricingHealthCards({
    businessPlanCount: 2,
    pricePlanCount: 5,
    wechatGoodCount: 4,
    issueCount: 3,
    blockedIssueCount: 1,
    degradedIssueCount: 2,
    healthyResourceCount: 8
  }), [
    { key: "businessPlans", label: "业务套餐", value: 2, tone: "primary", detail: "仅统计 V2 会员与代理商套餐" },
    { key: "pricePlans", label: "价格方案", value: 5, tone: "success", detail: "由服务端价格方案汇总" },
    { key: "wechatGoods", label: "微信商品", value: 4, tone: "info", detail: "本地微信虚拟商品记录" },
    { key: "issues", label: "健康问题", value: 3, tone: "danger", detail: "阻断 1 · 关注 2" }
  ]);
});

test("cached pricing health stays visible but refresh failures are never hidden", () => {
  const pricingHealthDisplayState = requiredFunction(governance, "pricingHealthDisplayState");
  assert.deepEqual(pricingHealthDisplayState({ hasCachedHealth: true, error: "timeout" }), {
    showCards: true,
    showError: true,
    stale: true
  });
  assert.deepEqual(pricingHealthDisplayState({ hasCachedHealth: false, error: "forbidden" }), {
    showCards: false,
    showError: true,
    stale: false
  });
  assert.deepEqual(pricingHealthDisplayState({ hasCachedHealth: true, error: "" }), {
    showCards: true,
    showError: false,
    stale: false
  });
});

test("business-plan detail keeps exactly six governance tabs in the agreed order", () => {
  assert.deepEqual(governance.PRICE_PLAN_DETAIL_TABS, [
    { id: "basic", label: "基本信息", ready: true },
    { id: "entitlements", label: "权益版本", ready: true },
    { id: "pricePlans", label: "价格方案", ready: true },
    { id: "wechatGoods", label: "微信商品", ready: true },
    { id: "testWhitelist", label: "测试白名单", ready: true },
    { id: "audit", label: "审计日志", ready: true }
  ]);
});

test("legacy plan editor gate permits IO only after authoritative legacy proof", async () => {
  const resolveLegacyPlanEditorGate = requiredFunction(governance, "resolveLegacyPlanEditorGate");
  const legacyPlanEditorAllowsIO = requiredFunction(governance, "legacyPlanEditorAllowsIO");
  const managedPlanHandoff = requiredFunction(governance, "managedPlanHandoff");

  const managed = await resolveLegacyPlanEditorGate("member-1", async () => ({ item: { id: "member-1", businessType: "MEMBER", code: "plan_member" } }));
  assert.equal(managed.status, "MANAGED");
  assert.deepEqual(managedPlanHandoff(managed), { moduleId: "pricePlanGovernance", planId: "member-1" });

  const explicitLegacy = await resolveLegacyPlanEditorGate("center-1", async () => ({ item: { id: "center-1", businessType: "OPERATION_CENTER_PACKAGE" } }));
  assert.equal(explicitLegacy.status, "LEGACY");

  const exactNotFound = await resolveLegacyPlanEditorGate("legacy-1", async () => {
    throw new client.AdminApiError("not managed", 404, "BUSINESS_PLAN_NOT_FOUND", { code: "BUSINESS_PLAN_NOT_FOUND" });
  });
  assert.equal(exactNotFound.status, "LEGACY");

  const blockedCases = [
    new client.AdminApiError("forbidden", 403, "ADMIN_PERMISSION_DENIED"),
    new client.AdminApiError("bare 404", 404, ""),
    new client.AdminApiError("server error", 500, "INTERNAL_ERROR")
  ];
  for (const error of blockedCases) {
    const result = await resolveLegacyPlanEditorGate("unsafe-1", async () => { throw error; });
    assert.equal(result.status, "BLOCKED", `${error.status}:${error.code}`);
  }
  assert.equal((await resolveLegacyPlanEditorGate("bad-1", async () => ({ item: {} }))).status, "BLOCKED");
  assert.equal((await resolveLegacyPlanEditorGate("bad-2", async () => ({ item: { id: "bad-2", businessType: "UNKNOWN" } }))).status, "BLOCKED");

  assert.equal(legacyPlanEditorAllowsIO("CHECKING"), false);
  assert.equal(legacyPlanEditorAllowsIO("MANAGED"), false);
  assert.equal(legacyPlanEditorAllowsIO("BLOCKED"), false);
  assert.equal(legacyPlanEditorAllowsIO("LEGACY"), true);
});

test("legacy plan editor save revalidates authoritative ownership before any write", async () => {
  const revalidateLegacyPlanEditorForSave = requiredFunction(governance, "revalidateLegacyPlanEditorForSave");
  const legacyPlanSaveContextIsCurrent = requiredFunction(governance, "legacyPlanSaveContextIsCurrent");
  const openedAsLegacy = { status: "LEGACY", planId: "plan-1", message: "legacy" };
  const changedToManaged = await revalidateLegacyPlanEditorForSave(openedAsLegacy, "plan-1", async () => ({
    item: { id: "plan-1", businessType: "MEMBER", code: "plan_member" }
  }));
  assert.equal(changedToManaged.status, "MANAGED");

  let lookedUp = false;
  const mismatched = await revalidateLegacyPlanEditorForSave(openedAsLegacy, "plan-2", async () => {
    lookedUp = true;
    return { item: { id: "plan-2", businessType: "OPERATION_CENTER_PACKAGE" } };
  });
  assert.equal(mismatched.status, "BLOCKED");
  assert.equal(lookedUp, false);

  const currentContext = {
    saveSequence: 4,
    currentSequence: 4,
    dialogOpen: true,
    expectedPlanId: "plan-1",
    currentPlanId: "plan-1",
    gate: openedAsLegacy
  };
  assert.equal(legacyPlanSaveContextIsCurrent(currentContext), true);
  assert.equal(legacyPlanSaveContextIsCurrent({ ...currentContext, currentSequence: 5 }), false);
  assert.equal(legacyPlanSaveContextIsCurrent({ ...currentContext, dialogOpen: false }), false);
  assert.equal(legacyPlanSaveContextIsCurrent({ ...currentContext, currentPlanId: "plan-2" }), false);
  assert.equal(legacyPlanSaveContextIsCurrent({ ...currentContext, gate: { status: "CHECKING", planId: "plan-1", message: "checking" } }), false);
});

test("price-plan list visibly treats an unknown V132 flag as unsafe", () => {
  const listSource = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PricePlanList.vue", import.meta.url), "utf8");
  assert.match(listSource, /store\.health\.runtime\?\.v132Blocked !== false/);
  assert.match(listSource, /V132[^\n]*(?:未知|未明确)/);
});

test("PlanEditorDialog owns one fail-closed gate before capability IO or save", () => {
  const editorSource = readFileSync(new URL("../admin-vue/src/components/billing/PlanEditorDialog.vue", import.meta.url), "utf8");
  const appSource = readFileSync(new URL("../admin-vue/src/App.vue", import.meta.url), "utf8");

  assert.match(editorSource, /gate:\s*LegacyPlanEditorGate/);
  assert.match(editorSource, /gate\.status === 'CHECKING'/);
  assert.match(editorSource, /gate\.status === 'LEGACY'/);
  assert.match(editorSource, /legacyPlanEditorAllowsIO\(props\.gate\.status\)/);
  assert.match(editorSource, /emit\(['"]managed-handoff['"]\)/);
  assert.match(editorSource, /emit\(['"]retry-gate['"]\)/);
  assert.match(editorSource, /:disabled="saving"/);
  assert.match(editorSource, /if \(props\.saving\) return/);
  assert.match(appSource, /:gate="planEditorGate"/);
  assert.match(appSource, /planEditorSaveSequence/);
  assert.match(appSource, /legacyPlanSaveContextIsCurrent/);
  assert.doesNotMatch(appSource, /planEditorGateOpen/);
  assert.doesNotMatch(appSource, /title="套餐编辑入口检查"/);
});

test("legacy editor loads the V2 pricing store only when its ownership gate is used", () => {
  const appSource = readFileSync(new URL("../admin-vue/src/App.vue", import.meta.url), "utf8");

  assert.doesNotMatch(
    appSource,
    /import\s*\{[^}]*usePricePlanAdminStore[^}]*\}\s*from\s*["']\.\/stores\/pricePlanAdmin(?:\.ts)?["']/,
    "the pricing store must not be pulled into the admin entry chunk"
  );
  assert.match(appSource, /async function loadPricePlanAdminStore\(\)/);
  assert.ok(
    (appSource.match(/await loadPricePlanAdminStore\(\)/g) || []).length >= 3,
    "gate lookup, managed handoff, and save revalidation must all await the same lazy store loader"
  );
  assert.doesNotMatch(appSource, /const\s+pricingAdminStore\s*=\s*usePricePlanAdminStore\(\)/);
});
