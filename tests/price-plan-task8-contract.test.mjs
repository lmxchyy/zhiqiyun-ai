import assert from "node:assert/strict";
import test from "node:test";
import { existsSync, readFileSync } from "node:fs";

import * as apiModule from "../admin-vue/src/api/pricePlanAdmin.ts";
import * as client from "../admin-vue/src/api/client.ts";
import * as domain from "../admin-vue/src/domain/pricePlanAdmin.ts";
import * as governance from "../admin-vue/src/domain/pricePlanGovernance.ts";
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

const auditRecord = {
  auditLogId: "audit-1",
  operatorId: "operator-1",
  operatorRole: "PRICE_OWNER",
  operationTime: "2026-07-28T02:03:04Z",
  action: "price_plan.make_default",
  entityType: "price_plan",
  entityId: "price-1",
  changeReason: "切换正式默认价",
  beforeSnapshot: null,
  afterSnapshot: { pricePlanId: "price-1", salePriceCents: 99600 },
  revisionBefore: null,
  revisionAfter: 4,
  requestId: "request-1",
  result: "SUCCEEDED",
  errorCode: "",
  planId: "plan-1",
  planVersionId: "version-1",
  pricePlanId: "price-1",
  wechatGoodId: "good-1",
  bindingId: "binding-1",
  whitelistEntryId: "whitelist-1",
  environment: "PRODUCTION",
  metadata: {}
};

function auditPage(item = auditRecord, page = 1) {
  return { items: [item], total: 1, page, pageSize: 25 };
}

test("pricing audit query emits only the 14 allowlisted keys after trim and omission", () => {
  const normalizePricingAuditFilters = requiredFunction(apiModule, "normalizePricingAuditFilters");
  const buildPricingAuditQuery = requiredFunction(apiModule, "buildPricingAuditQuery");
  const input = {
    planId: " plan/1 ",
    planVersionId: " version-1 ",
    pricePlanId: " price-1 ",
    wechatGoodId: " good-1 ",
    bindingId: " binding-1 ",
    whitelistEntryId: " whitelist-1 ",
    action: " price_plan.make_default ",
    operatorId: " operator-1 ",
    operatorRole: " PRICE_OWNER ",
    startTime: " 2026-07-01T00:00:00Z ",
    endTime: " 2026-07-31T23:59:59+08:00 ",
    result: "SUCCEEDED",
    page: 2,
    pageSize: 25,
    tenantId: "forged-tenant",
    accessToken: "must-not-leave-client"
  };
  assert.deepEqual(normalizePricingAuditFilters(input), {
    planId: "plan/1",
    planVersionId: "version-1",
    pricePlanId: "price-1",
    wechatGoodId: "good-1",
    bindingId: "binding-1",
    whitelistEntryId: "whitelist-1",
    action: "price_plan.make_default",
    operatorId: "operator-1",
    operatorRole: "PRICE_OWNER",
    startTime: "2026-07-01T00:00:00Z",
    endTime: "2026-07-31T23:59:59+08:00",
    result: "SUCCEEDED",
    page: 2,
    pageSize: 25
  });
  assert.equal(
    buildPricingAuditQuery(input),
    "?planId=plan%2F1&planVersionId=version-1&pricePlanId=price-1&wechatGoodId=good-1&bindingId=binding-1&whitelistEntryId=whitelist-1&action=price_plan.make_default&operatorId=operator-1&operatorRole=PRICE_OWNER&startTime=2026-07-01T00%3A00%3A00Z&endTime=2026-07-31T23%3A59%3A59%2B08%3A00&result=SUCCEEDED&page=2&pageSize=25"
  );
  assert.equal(buildPricingAuditQuery({ operatorId: "  " }), "?page=1&pageSize=50");
  assert.equal(input.planId, " plan/1 ", "normalization must not mutate caller-owned filters");
});

test("pricing audit query rejects invalid result, RFC3339, time order, and page bounds with stable codes", () => {
  const buildPricingAuditQuery = requiredFunction(apiModule, "buildPricingAuditQuery");
  const cases = [
    [{ result: "UNKNOWN" }, "PRICING_AUDIT_RESULT_INVALID"],
    [{ startTime: "2026-07-01 00:00:00" }, "PRICING_AUDIT_TIME_INVALID"],
    [{ endTime: "2026-07-01" }, "PRICING_AUDIT_TIME_INVALID"],
    [{ startTime: "2026-02-30T00:00:00Z" }, "PRICING_AUDIT_TIME_INVALID"],
    [{ startTime: "2026-07-02T00:00:00Z", endTime: "2026-07-01T00:00:00Z" }, "PRICING_AUDIT_TIME_RANGE_INVALID"],
    [{ page: 0 }, "PRICING_AUDIT_PAGE_INVALID"],
    [{ page: 1000001 }, "PRICING_AUDIT_PAGE_INVALID"],
    [{ page: 1.5 }, "PRICING_AUDIT_PAGE_INVALID"],
    [{ pageSize: 0 }, "PRICING_AUDIT_PAGE_SIZE_INVALID"],
    [{ pageSize: 201 }, "PRICING_AUDIT_PAGE_SIZE_INVALID"]
  ];
  for (const [filters, code] of cases) {
    assert.throws(() => buildPricingAuditQuery(filters), (error) => error instanceof client.AdminApiError && error.code === code);
  }
});

test("pricing audit permission gates the real loader and quick filters stay exact", async () => {
  const loadPricingAuditIfAllowed = requiredFunction(domain, "loadPricingAuditIfAllowed");
  const createPricingAuditLoadExecutor = requiredFunction(domain, "createPricingAuditLoadExecutor");
  assert.equal(domain.hasPricingPermission({ role: "ADMIN", permissions: ["admin.full"] }, "pricing:audit:view"), false);
  assert.equal(domain.hasPricingPermission({ role: "FINANCE", permissions: ["pricing:audit:view"] }, "pricing:audit:view"), true);
  assert.equal(domain.hasPricingPermission({ role: "SUPER_ADMIN", permissions: [] }, "pricing:audit:view"), true);
  let requests = 0;
  const loader = async (filters) => {
    requests += 1;
    return { filters };
  };
  assert.equal(await loadPricingAuditIfAllowed({ role: "ADMIN", permissions: ["admin.full"] }, { planId: "plan-1" }, loader), null);
  assert.equal(requests, 0, "an unauthorized audit view must not reach the API/store loader");
  assert.deepEqual(
    await loadPricingAuditIfAllowed({ role: "FINANCE", permissions: ["pricing:audit:view"] }, { planId: "plan-1" }, loader),
    { filters: { planId: "plan-1" } }
  );
  assert.equal(requests, 1);
  const blockedExecutor = createPricingAuditLoadExecutor(
    () => ({ role: "ADMIN", permissions: ["admin.full"] }),
    loader
  );
  assert.equal(await blockedExecutor({ planId: "plan-1" }), null);
  assert.equal(requests, 1, "the component-bound executor must make zero requests without pricing:audit:view");
  assert.deepEqual(domain.PRICING_AUDIT_QUICK_FILTERS, [
    { id: "defaultSwitch", label: "默认价格切换", action: "price_plan.make_default" },
    { id: "goodConfirmation", label: "微信商品人工确认", action: "wechat_good.confirm_published" },
    { id: "whitelistCreate", label: "白名单新增", action: "price_plan.test_whitelist.create" },
    { id: "whitelistUpdate", label: "白名单修改", action: "price_plan.test_whitelist.update" },
    { id: "whitelistDisable", label: "白名单停用", action: "price_plan.test_whitelist.disable" }
  ]);
});

test("pricing audit display state preserves matching cache on retryable errors", () => {
  const pricingAuditDisplayState = requiredFunction(domain, "pricingAuditDisplayState");
  assert.equal(pricingAuditDisplayState({ canView: false, cacheMatches: false, hasPage: false, loading: false, error: "" }), "FORBIDDEN");
  assert.equal(pricingAuditDisplayState({ canView: true, cacheMatches: false, hasPage: true, loading: true, error: "" }), "LOADING");
  assert.equal(pricingAuditDisplayState({ canView: true, cacheMatches: true, hasPage: true, loading: false, error: "timeout" }), "TABLE_STALE");
  assert.equal(pricingAuditDisplayState({ canView: true, cacheMatches: false, hasPage: true, loading: false, error: "timeout" }), "ERROR");
  assert.equal(pricingAuditDisplayState({ canView: true, cacheMatches: true, hasPage: true, loading: false, error: "", rowCount: 0 }), "EMPTY");
  assert.equal(pricingAuditDisplayState({ canView: true, cacheMatches: true, hasPage: true, loading: false, error: "", rowCount: 1 }), "TABLE");
});

test("audit snapshot text recursively redacts secrets and enforces depth, item, string, and total limits", () => {
  const formatPricingAuditSnapshot = requiredFunction(domain, "formatPricingAuditSnapshot");
  const safe = formatPricingAuditSnapshot({
    productId: "wx-product-1",
    appSecret: "sentinel-app-secret",
    nested: {
      session_key: "sentinel-session-key",
      child: { authorization: "Bearer sentinel-token", retained: "safe-value" }
    },
    records: [{ databaseUrl: "postgres://user:password@host/db" }]
  });
  assert.equal(safe.redacted, true);
  assert.doesNotMatch(safe.text, /sentinel|postgres:\/\//i);
  assert.match(safe.text, /\[REDACTED\]/);
  assert.match(safe.text, /wx-product-1/);

  const bounded = formatPricingAuditSnapshot({
    deep: { one: { two: { three: { four: "too-deep" } } } },
    many: Array.from({ length: 40 }, (_, index) => `item-${index}`),
    long: "x".repeat(1000),
    huge: Array.from({ length: 1000 }, () => "y".repeat(1000))
  }, { maxDepth: 3, maxItems: 12, maxStringLength: 40, maxTotalCharacters: 500 });
  assert.equal(bounded.truncated, true);
  assert.ok(bounded.text.length <= 500);
  assert.match(bounded.text, /TRUNCATED_(?:DEPTH|ITEMS|STRING|TOTAL)/);

  let arrayReads = 0;
  const lazyItems = new Array(100);
  for (let index = 0; index < lazyItems.length; index += 1) {
    Object.defineProperty(lazyItems, index, {
      enumerable: true,
      get() {
        arrayReads += 1;
        return `lazy-${index}`;
      }
    });
  }
  const itemLimited = formatPricingAuditSnapshot({ lazyItems }, { maxItems: 5, maxTotalCharacters: 5000 });
  assert.equal(itemLimited.truncated, true);
  assert.ok(arrayReads <= 4, `maxItems must stop traversal before reading the remaining array (${arrayReads} reads)`);

  let totalReads = 0;
  const lazyObject = {};
  for (let index = 0; index < 100; index += 1) {
    Object.defineProperty(lazyObject, `field-${index}`, {
      enumerable: true,
      get() {
        totalReads += 1;
        return "z".repeat(1000);
      }
    });
  }
  const totalLimited = formatPricingAuditSnapshot(lazyObject, {
    maxItems: 500,
    maxStringLength: 2000,
    maxTotalCharacters: 128
  });
  assert.equal(totalLimited.truncated, true);
  assert.ok(totalLimited.text.length <= 128);
  assert.ok(totalReads < 10, `maxTotalCharacters must stop traversal early (${totalReads} reads)`);

  const domainSource = readFileSync(new URL("../admin-vue/src/domain/pricePlanAdmin.ts", import.meta.url), "utf8");
  const formatterSource = domainSource.slice(
    domainSource.indexOf("export function formatPricingAuditSnapshot"),
    domainSource.indexOf("export interface EntitlementVersionActionInput")
  );
  assert.doesNotMatch(formatterSource, /Object\.keys\(/, "snapshot traversal must not allocate the complete key list before applying limits");
  assert.match(formatterSource, /for \(const key in record\)/);
  assert.match(formatterSource, /Object\.prototype\.hasOwnProperty\.call\(record, key\)/);
});

test("obsolete success finishing while the latest audit request is pending cannot clear loading", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  const oldRequest = deferred();
  const latestRequest = deferred();
  let call = 0;
  try {
    api.listPricingAuditLogs = async () => (++call === 1 ? oldRequest.promise : latestRequest.promise);
    const oldLoad = store.loadAuditLogs({ planId: "plan-old" });
    const latestLoad = store.loadAuditLogs({ planId: "plan-latest" });
    oldRequest.resolve(auditPage({ ...auditRecord, planId: "plan-old" }));
    await oldLoad;
    assert.equal(store.loading.audit, true);
    assert.equal(store.auditPage, null);
    latestRequest.resolve(auditPage({ ...auditRecord, planId: "plan-latest" }));
    await latestLoad;
    assert.equal(store.loading.audit, false);
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("obsolete failure finishing while the latest audit request is pending cannot clear loading or set error", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  const oldRequest = deferred();
  const latestRequest = deferred();
  let call = 0;
  try {
    api.listPricingAuditLogs = async () => (++call === 1 ? oldRequest.promise : latestRequest.promise);
    const oldLoad = store.loadAuditLogs({ planId: "plan-old" }).catch(() => undefined);
    const latestLoad = store.loadAuditLogs({ planId: "plan-latest" });
    oldRequest.reject(new client.AdminApiError("old failed", 503, "PRICING_AUDIT_STORE_UNAVAILABLE"));
    await oldLoad;
    assert.equal(store.loading.audit, true);
    assert.equal(store.errors.audit, undefined);
    latestRequest.resolve(auditPage({ ...auditRecord, planId: "plan-latest" }));
    await latestLoad;
    assert.equal(store.loading.audit, false);
    assert.equal(store.errors.audit, undefined);
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("pricing audit store snapshots filters and accepts only the latest response", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  const oldRequest = deferred();
  const newRequest = deferred();
  const seenFilters = [];
  let call = 0;
  try {
    api.listPricingAuditLogs = async (filters) => {
      seenFilters.push(filters);
      return ++call === 1 ? oldRequest.promise : newRequest.promise;
    };
    const oldInput = { planId: " plan-old ", page: 1, pageSize: 25 };
    const oldLoad = store.loadAuditLogs(oldInput).catch(() => undefined);
    oldInput.planId = "mutated-after-call";
    const newLoad = store.loadAuditLogs({ planId: " plan-new ", action: " price_plan.make_default ", page: 2, pageSize: 25 });
    const newestPage = auditPage({ ...auditRecord, auditLogId: "audit-new", planId: "plan-new" }, 2);
    newRequest.resolve(newestPage);
    await newLoad;
    oldRequest.reject(new client.AdminApiError("old failed", 503, "PRICING_AUDIT_STORE_UNAVAILABLE"));
    await oldLoad;

    assert.deepEqual(seenFilters[0], { planId: "plan-old", page: 1, pageSize: 25 });
    assert.notEqual(seenFilters[0], oldInput);
    assert.deepEqual(store.auditFilters, { planId: "plan-new", action: "price_plan.make_default", page: 2, pageSize: 25 });
    assert.deepEqual(store.auditPage, newestPage);
    assert.equal(store.errors.audit, undefined);
    assert.equal(store.loading.audit, false);
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("an obsolete successful pricing audit response cannot replace the newest page or loading state", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  const oldRequest = deferred();
  const newRequest = deferred();
  let call = 0;
  try {
    api.listPricingAuditLogs = async () => (++call === 1 ? oldRequest.promise : newRequest.promise);
    const oldLoad = store.loadAuditLogs({ planId: "plan-old", page: 1, pageSize: 25 });
    const newLoad = store.loadAuditLogs({ planId: "plan-new", page: 1, pageSize: 25 });
    const newestPage = auditPage({ ...auditRecord, auditLogId: "audit-new", planId: "plan-new" });
    newRequest.resolve(newestPage);
    await newLoad;
    oldRequest.resolve(auditPage({ ...auditRecord, auditLogId: "audit-old", planId: "plan-old" }));
    await oldLoad;
    assert.deepEqual(store.auditPage, newestPage);
    assert.deepEqual(store.auditFilters, { planId: "plan-new", page: 1, pageSize: 25 });
    assert.equal(store.errors.audit, undefined);
    assert.equal(store.loading.audit, false);
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("pricing audit store preserves a matching cached page when refresh fails", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const cachedPage = auditPage();
  store.auditPage = cachedPage;
  store.auditFilters = { planId: "plan-1", page: 1, pageSize: 25 };
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  try {
    api.listPricingAuditLogs = async () => { throw new client.AdminApiError("down", 503, "PRICING_AUDIT_STORE_UNAVAILABLE"); };
    await assert.rejects(store.loadAuditLogs({ planId: " plan-1 ", page: 1, pageSize: 25 }));
    assert.deepEqual(store.auditPage, cachedPage);
    assert.deepEqual(store.auditFilters, { planId: "plan-1", page: 1, pageSize: 25 });
    assert.equal(store.errors.audit.code, "PRICING_AUDIT_STORE_UNAVAILABLE");
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("a failed request for different filters retains cache without allowing it to match the new query", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const cachedPage = auditPage();
  store.auditPage = cachedPage;
  store.auditFilters = { planId: "plan-old", page: 1, pageSize: 25 };
  const api = apiModule.pricePlanAdminApi;
  const original = api.listPricingAuditLogs;
  try {
    api.listPricingAuditLogs = async () => { throw new client.AdminApiError("down", 503, "PRICING_AUDIT_STORE_UNAVAILABLE"); };
    const requested = { planId: "plan-new", page: 1, pageSize: 25 };
    await assert.rejects(store.loadAuditLogs(requested));
    assert.deepEqual(store.auditPage, cachedPage);
    assert.deepEqual(store.auditFilters, { planId: "plan-old", page: 1, pageSize: 25 });
    assert.notEqual(apiModule.buildPricingAuditQuery(store.auditFilters), apiModule.buildPricingAuditQuery(requested));
  } finally {
    api.listPricingAuditLogs = original;
  }
});

test("pricing audit API uses the unified client and the explicit allowlisted query", async () => {
  const apiClient = client.apiClient;
  const api = apiModule.pricePlanAdminApi;
  const originalAdapter = apiClient.defaults.adapter;
  let request;
  try {
    apiClient.defaults.adapter = async (config) => {
      request = { method: String(config.method).toUpperCase(), url: config.url };
      return { data: { items: [], total: 0, page: 3, pageSize: 50 }, status: 200, statusText: "OK", headers: {}, config };
    };
    await api.listPricingAuditLogs({ pricePlanId: " price/1 ", action: " price_plan.enable ", page: 3, pageSize: 50, sessionKey: "forged" });
    assert.deepEqual(request, {
      method: "GET",
      url: "/admin/pricing-audit-logs?pricePlanId=price%2F1&action=price_plan.enable&page=3&pageSize=50"
    });
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("pricing audit stable errors have explicit operator guidance", () => {
  const expected = {
    PRICING_AUDIT_FILTER_INVALID: "审计筛选条件无效，请检查后重试。",
    PRICING_AUDIT_PAGE_INVALID: "审计页码无效，请返回有效页码后重试。",
    PRICING_AUDIT_PAGE_SIZE_INVALID: "审计每页数量必须在 1 到 200 之间。",
    PRICING_AUDIT_RESULT_INVALID: "审计结果只能筛选成功或失败。",
    PRICING_AUDIT_TIME_INVALID: "审计时间必须使用带时区的 RFC3339 格式。",
    PRICING_AUDIT_TIME_RANGE_INVALID: "审计结束时间不能早于开始时间。",
    PRICING_AUDIT_STORE_UNAVAILABLE: "审计服务当前不可用，已保留现有结果，请稍后重试。"
  };
  for (const [code, message] of Object.entries(expected)) {
    assert.equal(domain.pricingErrorMessage({ code }), message);
  }
});

test("Task 8 UI mounts the read-only audit tab and entity-prefill links without unsafe rendering", () => {
  const componentURL = new URL("../admin-vue/src/components/billing/price-plan-admin/PricingAuditLog.vue", import.meta.url);
  assert.equal(existsSync(componentURL), true, "PricingAuditLog.vue must exist");
  const audit = existsSync(componentURL) ? readFileSync(componentURL, "utf8") : "";
  const governanceSource = readFileSync(new URL("../admin-vue/src/components/billing/price-plan-admin/PricePlanGovernance.vue", import.meta.url), "utf8");
  const entitySources = [
    "PlanVersionManager.vue",
    "PricePlanList.vue",
    "WechatVirtualGoodsManager.vue",
    "PaymentBindingDialog.vue",
    "PricePlanWhitelistManager.vue"
  ].map((name) => readFileSync(new URL(`../admin-vue/src/components/billing/price-plan-admin/${name}`, import.meta.url), "utf8"));

  assert.match(audit, /pricing:audit:view/);
  assert.match(audit, /if \(!canView\.value\) return/);
  assert.match(audit, /createPricingAuditLoadExecutor\(\s*\(\) => principal\.value,\s*\(filters\) => store\.loadAuditLogs\(filters\)\s*\)/s);
  assert.match(audit, /const auditRequestGate\s*=\s*createLatestRequestGate\(\)/);
  assert.match(audit, /const token\s*=\s*auditRequestGate\.begin\(\)/);
  assert.match(audit, /if \(auditRequestGate\.isLatest\(token\)\)\s*localError\.value\s*=\s*pricingErrorMessage\(error\)/);
  const loadAuditSource = audit.slice(audit.indexOf("async function loadAudit()"), audit.indexOf("function search()"));
  assert.ok(
    loadAuditSource.indexOf('requestedSignature.value = ""') >= 0
      && loadAuditSource.indexOf('requestedSignature.value = ""') < loadAuditSource.indexOf("const normalized = requestFilters()"),
    "the newest local intent must stop matching cached rows before local filter validation"
  );
  assert.match(audit, /const rows\s*=\s*computed\(\(\)\s*=>\s*cacheMatches\.value\s*\?\s*store\.auditPage\?\.items\s*\|\|\s*\[\]\s*:\s*\[\]\)/);
  assert.match(audit, /detailOpen\.value\s*=\s*false;\s*selectedLog\.value\s*=\s*null;\s*detailSections\.value\s*=\s*\[\];/s);
  assert.match(audit, /PRICING_AUDIT_QUICK_FILTERS/);
  assert.match(audit, /formatPricingAuditSnapshot/);
  assert.match(audit, /<pre[^>]*>\s*{{/);
  assert.match(audit, /el-drawer/);
  assert.doesNotMatch(audit, /v-html|console\.|<a\b|el-link|href=/i);
  for (const filter of [
    "planId", "planVersionId", "pricePlanId", "wechatGoodId", "bindingId", "whitelistEntryId",
    "action", "operatorId", "operatorRole", "startTime", "endTime", "result"
  ]) {
    assert.match(audit, new RegExp(`data-audit-filter=["']${filter}["']`), `missing server-side ${filter} filter`);
  }
  for (const column of ["operationTime", "operator", "action", "entity", "changeReason", "revision", "result", "requestId"]) {
    assert.match(audit, new RegExp(`data-audit-column=["']${column}["']`), `missing ${column} audit column`);
  }
  assert.match(audit, /data-audit-snapshot=["']before["']/);
  assert.match(audit, /data-audit-snapshot=["']after["']/);
  assert.match(governanceSource, /PricingAuditLog/);
  assert.match(governanceSource, /tab\.id === ['"]audit['"]/);
  assert.match(governanceSource, /@view-audit=/);
  assert.match(governanceSource, /const canViewAudit\s*=\s*computed\(\(\)\s*=>\s*hasPricingPermission\(principal\.value,\s*["']pricing:audit:view["']\)\)/);
  assert.match(governanceSource, /PRICE_PLAN_DETAIL_TABS\.filter\(\(tab\)\s*=>\s*tab\.id\s*!==\s*["']audit["']\s*\|\|\s*canViewAudit\.value\)/);
  assert.match(governanceSource, /v-for=["']tab in visibleDetailTabs["']/);
  const exactPrefilters = [
    /emit\("view-audit",\s*\{\s*planId:\s*props\.plan\.id,\s*planVersionId:\s*version\.id\s*\}\)/s,
    /emit\("view-audit",\s*\{\s*planId:\s*props\.plan\.id,\s*pricePlanId:\s*row\.pricePlanId\s*\}\)/s,
    /emit\("view-audit",\s*\{\s*wechatGoodId:\s*row\.id\s*\}\)/s,
    /emit\("view-audit",\s*\{\s*planId:\s*props\.plan\.id,\s*pricePlanId:\s*props\.pricePlanId,\s*bindingId:\s*binding\.id\s*\}\)/s,
    /emit\("view-audit",\s*\{\s*planId:\s*props\.plan\.id,\s*pricePlanId:\s*selectedPricePlanId\.value,\s*whitelistEntryId:\s*entry\.whitelistEntryId\s*\}\)/s
  ];
  entitySources.forEach((source, index) => {
    assert.match(source, exactPrefilters[index], "entity audit links must emit their exact server-side prefilter IDs");
    assert.match(source, /hasPricingPermission\(principal\.value,\s*["']pricing:audit:view["']\)/);
    assert.match(source, /v-if=["'](?:binding\s*&&\s*)?canViewAudit["']/, "unauthorized entity surfaces must not render audit actions");
  });
  assert.match(governanceSource, /function openAudit\(filters:\s*PricingAuditFilters\)/);
  assert.match(governanceSource, /auditPrefill\.value\s*=\s*\{\s*\.\.\.filters,\s*page:\s*1,\s*pageSize:\s*50\s*\}/s);
  assert.deepEqual(governance.PRICE_PLAN_DETAIL_TABS.at(-1), { id: "audit", label: "审计日志", ready: true });
});

test("pricing audit frontend types keep snapshots and revisions nullable and result strict", () => {
  const source = readFileSync(new URL("../admin-vue/src/types/pricePlanAdmin.ts", import.meta.url), "utf8");
  const auditType = source.slice(source.indexOf("export interface PricingAuditLog"), source.indexOf("export interface PricingAuditPage"));
  assert.match(auditType, /beforeSnapshot:\s*unknown\s*\|\s*null/);
  assert.match(auditType, /afterSnapshot:\s*unknown\s*\|\s*null/);
  assert.match(auditType, /revisionBefore:\s*number\s*\|\s*null/);
  assert.match(auditType, /revisionAfter:\s*number\s*\|\s*null/);
  assert.match(auditType, /result:\s*"SUCCEEDED"\s*\|\s*"FAILED"\s*;/);
  assert.doesNotMatch(auditType, /result:[^;]*string/);
});
