import assert from "node:assert/strict";
import test from "node:test";
import { existsSync, readFileSync } from "node:fs";

import * as apiModule from "../admin-vue/src/api/pricePlanAdmin.ts";
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

const testPlan = {
  pricePlanId: "price-test",
  planId: "plan-member",
  planVersionId: "version-active",
  code: "member_test",
  name: "会员测试价",
  kind: "TEST",
  channel: "WECHAT_VIRTUAL",
  environment: "SANDBOX",
  currency: "CNY",
  salePriceCents: 100,
  listPriceCents: 100,
  giftPoints: 0,
  giftTokens: 0,
  audienceType: "WHITELIST",
  audienceRule: {},
  isVisible: false,
  isDefault: false,
  isEnabled: true,
  status: "ACTIVE",
  revision: 3,
  createdAt: "2026-07-27T02:00:00Z",
  updatedAt: "2026-07-28T02:00:00Z",
  hasQuote: false,
  hasOrder: false,
  economicFieldsLocked: false
};

const activeEntry = {
  whitelistEntryId: "entry-active",
  planId: "plan-member",
  pricePlanId: "price-test",
  userId: "user-1",
  status: "ACTIVE",
  validFrom: "2026-07-28T00:00:00+08:00",
  validUntil: "2026-08-28T00:00:00+08:00",
  reason: "验收测试",
  revision: 2,
  createdBy: "operator-1",
  updatedBy: "operator-2",
  createdAt: "2026-07-27T02:00:00Z",
  updatedAt: "2026-07-28T02:00:00Z"
};

test("whitelist selector exposes only TEST plans from the selected business plan", () => {
  const selectableWhitelistPricePlans = requiredFunction(domain, "selectableWhitelistPricePlans");
  const plans = [
    testPlan,
    { ...testPlan, pricePlanId: "price-normal", code: "member_normal", kind: "NORMAL" },
    { ...testPlan, pricePlanId: "price-other", planId: "plan-agent" }
  ];
  assert.deepEqual(selectableWhitelistPricePlans(plans, "plan-member").map((item) => item.pricePlanId), ["price-test"]);
});

test("whitelist UI permissions require plan view for reads and the exact manage permission for writes", () => {
  const whitelistEntryUIActions = requiredFunction(domain, "whitelistEntryUIActions");
  const adminFull = whitelistEntryUIActions(activeEntry, { role: "ADMIN", permissions: ["admin.full", "pricing:plan:view"] });
  assert.deepEqual(adminFull, { canView: true, canManage: false, canEdit: false, canDisable: false, requiresNewEntry: false });

  const manager = whitelistEntryUIActions(activeEntry, {
    role: "ADMIN",
    permissions: ["pricing:plan:view", "pricing:test-whitelist:manage"]
  });
  assert.deepEqual(manager, { canView: true, canManage: true, canEdit: true, canDisable: true, requiresNewEntry: false });

  const expired = whitelistEntryUIActions({ ...activeEntry, status: "EXPIRED" }, {
    role: "ADMIN",
    permissions: ["pricing:plan:view", "pricing:test-whitelist:manage"]
  });
  assert.deepEqual(expired, { canView: true, canManage: true, canEdit: false, canDisable: false, requiresNewEntry: true });

  const disabled = whitelistEntryUIActions({ ...activeEntry, status: "DISABLED" }, {
    role: "ADMIN",
    permissions: ["pricing:plan:view", "pricing:test-whitelist:manage"]
  });
  assert.deepEqual(disabled, { canView: true, canManage: true, canEdit: false, canDisable: false, requiresNewEntry: true });

  const superAdmin = whitelistEntryUIActions({ ...activeEntry, status: "PENDING" }, { role: "SUPER_ADMIN", permissions: [] });
  assert.equal(superAdmin.canEdit, true);
  assert.equal(superAdmin.canDisable, true);
});

test("whitelist validity requires RFC3339 offsets and an increasing interval", () => {
  const whitelistValidityIssue = requiredFunction(domain, "whitelistValidityIssue");
  assert.equal(whitelistValidityIssue({ validFrom: "2026-07-28T09:00:00+08:00", validUntil: "2026-07-29T09:00:00+08:00" }), "");
  assert.equal(whitelistValidityIssue({ validFrom: "2026-07-28T01:00:00Z", validUntil: "2026-07-29T01:00:00Z" }), "");
  assert.equal(whitelistValidityIssue({ validFrom: "2026-07-28 09:00:00", validUntil: "2026-07-29T09:00:00+08:00" }), "WHITELIST_RFC3339_OFFSET_REQUIRED");
  assert.equal(whitelistValidityIssue({ validFrom: "2026-07-29T09:00:00+08:00", validUntil: "2026-07-28T09:00:00+08:00" }), "WHITELIST_VALIDITY_INVALID");
});

test("revision conflicts preserve the operator form and repeated disable is disclosed as idempotent", () => {
  const whitelistMutationErrorState = requiredFunction(domain, "whitelistMutationErrorState");
  const whitelistDisableResultMessage = requiredFunction(domain, "whitelistDisableResultMessage");
  assert.deepEqual(whitelistMutationErrorState({ code: "REVISION_CONFLICT", status: 409 }), {
    revisionConflict: true,
    preserveForm: true,
    message: "数据已被其他操作修改，请刷新后重试；不会自动覆盖服务器版本。"
  });
  assert.equal(whitelistDisableResultMessage({ alreadyDisabled: false }), "白名单已停用");
  assert.equal(whitelistDisableResultMessage({ alreadyDisabled: true }), "白名单此前已停用，本次为幂等成功，未产生新的状态变更");
});

test("whitelist reads are latest-wins for data, filters, errors, and loading state", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listWhitelist;
  const oldRequest = deferred();
  const newRequest = deferred();
  let call = 0;
  try {
    api.listWhitelist = async () => (++call === 1 ? oldRequest.promise : newRequest.promise);
    const oldLoad = store.loadWhitelist("price-test", { userId: "old-user", status: "PENDING", page: 1, pageSize: 20 });
    const newLoad = store.loadWhitelist("price-test", { userId: "new-user", status: "ACTIVE", page: 2, pageSize: 20 });
    newRequest.resolve({ items: [{ ...activeEntry, userId: "new-user" }], total: 1, page: 2, pageSize: 20 });
    await newLoad;
    oldRequest.resolve({ items: [{ ...activeEntry, userId: "old-user" }], total: 1, page: 1, pageSize: 20 });
    await oldLoad;

    assert.equal(store.whitelistByPricePlanId["price-test"][0].userId, "new-user");
    assert.deepEqual(store.whitelistFiltersByPricePlanId["price-test"], { userId: "new-user", status: "ACTIVE", page: 2, pageSize: 20 });
    assert.equal(store.errors["whitelist:price-test"], undefined);
    assert.equal(store.loading["whitelist:price-test"], false);
  } finally {
    api.listWhitelist = original;
  }
});

test("an obsolete failed whitelist read cannot replace a newer successful page", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listWhitelist;
  const oldRequest = deferred();
  const newRequest = deferred();
  let call = 0;
  try {
    api.listWhitelist = async () => (++call === 1 ? oldRequest.promise : newRequest.promise);
    const oldLoad = store.loadWhitelist("price-test", { userId: "old-user", page: 1, pageSize: 20 }).catch(() => undefined);
    const newLoad = store.loadWhitelist("price-test", { userId: "new-user", page: 1, pageSize: 20 });
    newRequest.resolve({ items: [{ ...activeEntry, userId: "new-user" }], total: 1, page: 1, pageSize: 20 });
    await newLoad;
    oldRequest.reject({ code: "WHITELIST_READ_FAILED", status: 503 });
    await oldLoad;

    assert.equal(store.whitelistByPricePlanId["price-test"][0].userId, "new-user");
    assert.equal(store.errors["whitelist:price-test"], undefined);
    assert.equal(store.loading["whitelist:price-test"], false);
  } finally {
    api.listWhitelist = original;
  }
});

test("a committed whitelist write with refresh failure preserves cache and locks repeat submission", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.whitelistByPricePlanId["price-test"] = [activeEntry];
  store.whitelistFiltersByPricePlanId["price-test"] = { status: "ACTIVE", page: 2, pageSize: 20 };
  const api = apiModule.pricePlanAdminApi;
  const originalCreate = api.createWhitelistEntry;
  const originalList = api.listWhitelist;
  try {
    api.createWhitelistEntry = async () => ({ item: { ...activeEntry, whitelistEntryId: "entry-new", userId: "user-new" } });
    api.listWhitelist = async () => { throw { code: "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE", status: 503 }; };
    const response = await store.createWhitelistEntry("price-test", {
      revision: 0,
      userId: "user-new",
      reason: "测试",
      changeReason: "新增测试账号"
    });
    assert.equal(response.item.userId, "user-new");
    assert.deepEqual(store.whitelistByPricePlanId["price-test"], [activeEntry], "no optimistic row may be inserted");
    assert.ok(store.refreshWarnings["createWhitelist:price-test"]);
    assert.equal(store.saving["createWhitelist:price-test"], false);
  } finally {
    api.createWhitelistEntry = originalCreate;
    api.listWhitelist = originalList;
  }
});

test("whitelist refresh gate survives close-reopen and blocks create, edit, and disable until exact recovery", async () => {
  const whitelistRefreshGateKey = requiredFunction(domain, "whitelistRefreshGateKey");
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  store.whitelistByPricePlanId["price-test"] = [activeEntry];
  store.whitelistFiltersByPricePlanId["price-test"] = { status: "ACTIVE", page: 1, pageSize: 20 };
  const api = apiModule.pricePlanAdminApi;
  const originals = {
    create: api.createWhitelistEntry,
    update: api.updateWhitelistEntry,
    disable: api.disableWhitelistEntry,
    list: api.listWhitelist
  };
  let mutationCalls = 0;
  let listShouldFail = true;
  const committedEntry = { ...activeEntry, whitelistEntryId: "entry-new", userId: "user-new", revision: 1 };
  try {
    api.createWhitelistEntry = async () => {
      mutationCalls += 1;
      return { item: committedEntry };
    };
    api.updateWhitelistEntry = async () => {
      mutationCalls += 1;
      return { item: { ...activeEntry, revision: 3, reason: "changed" } };
    };
    api.disableWhitelistEntry = async () => {
      mutationCalls += 1;
      return { item: { ...activeEntry, status: "DISABLED", revision: 3 }, alreadyDisabled: false };
    };
    api.listWhitelist = async (_pricePlanId, filters) => {
      if (listShouldFail) throw { code: "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE", status: 503 };
      const items = filters.userId === committedEntry.userId ? [committedEntry] : [activeEntry];
      return { items, total: 1, page: filters.page, pageSize: filters.pageSize };
    };

    await store.createWhitelistEntry("price-test", {
      revision: 0,
      userId: "user-new",
      reason: "测试",
      changeReason: "新增测试账号"
    });
    const gateKey = whitelistRefreshGateKey("price-test");
    assert.ok(store.refreshWarnings[gateKey], "resource gate must survive the failed post-write refresh");
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], {
      pricePlanId: "price-test",
      whitelistEntryId: committedEntry.whitelistEntryId,
      userId: committedEntry.userId,
      revision: committedEntry.revision,
      status: committedEntry.status
    });
    assert.equal(mutationCalls, 1);

    // Simulate closing and reopening every write surface: the persistent Store gate remains authoritative.
    store.clearRefreshWarning(gateKey);
    assert.ok(store.refreshWarnings[gateKey], "generic dialog cleanup must not clear the resource gate");
    await assert.rejects(
      store.createWhitelistEntry("price-test", { revision: 0, userId: "user-2", reason: "测试", changeReason: "重试创建" }),
      (error) => error?.code === "WHITELIST_REFRESH_REQUIRED"
    );
    await assert.rejects(
      store.updateWhitelistEntry("price-test", activeEntry.whitelistEntryId, { revision: 2, reason: "changed", changeReason: "重试编辑" }),
      (error) => error?.code === "WHITELIST_REFRESH_REQUIRED"
    );
    await assert.rejects(
      store.disableWhitelistEntry("price-test", activeEntry.whitelistEntryId, { revision: 2, changeReason: "重试停用" }),
      (error) => error?.code === "WHITELIST_REFRESH_REQUIRED"
    );
    assert.equal(mutationCalls, 1, "no second mutation may reach the API while the gate is locked");

    listShouldFail = false;
    await store.loadWhitelist("price-test", { status: "ACTIVE", page: 1, pageSize: 20 });
    assert.ok(store.refreshWarnings[gateKey], "an ordinary list read must not silently clear the gate");
    await store.recoverWhitelistRefreshGate("price-test", { status: "ACTIVE", page: 1, pageSize: 20 });
    assert.equal(store.refreshWarnings[gateKey], undefined, "only explicit exact recovery may clear the gate");

    await store.updateWhitelistEntry("price-test", activeEntry.whitelistEntryId, { revision: 2, reason: "changed", changeReason: "恢复后编辑" });
    assert.equal(mutationCalls, 2);
  } finally {
    api.createWhitelistEntry = originals.create;
    api.updateWhitelistEntry = originals.update;
    api.disableWhitelistEntry = originals.disable;
    api.listWhitelist = originals.list;
  }
});

test("whitelist recovery verifies the pinned mutation identity and visible page before clearing its gate", async () => {
  const whitelistRefreshGateKey = requiredFunction(domain, "whitelistRefreshGateKey");
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const originals = { create: api.createWhitelistEntry, list: api.listWhitelist };
  const committedEntry = { ...activeEntry, whitelistEntryId: "entry-pinned", userId: "user-pinned", revision: 7 };
  const visibleEntry = { ...activeEntry, whitelistEntryId: "entry-visible", userId: "user-visible" };
  const currentFilters = { status: "PENDING", userId: "visible-filter", page: 3, pageSize: 20 };
  let mode = "initial-failure";
  const calls = [];
  try {
    api.createWhitelistEntry = async () => ({ item: committedEntry });
    api.listWhitelist = async (_pricePlanId, filters) => {
      calls.push({ ...filters });
      if (mode === "initial-failure") throw { code: "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE", status: 503 };
      if (filters.userId === committedEntry.userId) {
        if (mode === "empty") return { items: [], total: 0, page: filters.page, pageSize: filters.pageSize };
        if (mode === "wrong") return { items: [{ ...committedEntry, whitelistEntryId: "entry-wrong" }], total: 1, page: filters.page, pageSize: filters.pageSize };
        if (mode === "stale-revision") return { items: [{ ...committedEntry, revision: committedEntry.revision - 1 }], total: 1, page: filters.page, pageSize: filters.pageSize };
        return { items: [committedEntry], total: 1, page: filters.page, pageSize: filters.pageSize };
      }
      if (mode === "visible-error") throw { code: "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE", status: 503 };
      return { items: [visibleEntry], total: 1, page: filters.page, pageSize: filters.pageSize };
    };

    await store.createWhitelistEntry("price-test", {
      revision: 0,
      userId: committedEntry.userId,
      reason: "测试",
      changeReason: "新增并验证写后状态"
    });
    const gateKey = whitelistRefreshGateKey("price-test");
    const expectedGate = {
      pricePlanId: "price-test",
      whitelistEntryId: committedEntry.whitelistEntryId,
      userId: committedEntry.userId,
      revision: committedEntry.revision,
      status: committedEntry.status
    };
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], expectedGate);
    assert.deepEqual(Object.keys(expectedGate).sort(), ["pricePlanId", "revision", "status", "userId", "whitelistEntryId"], "gate must contain no reason, credential, or other sensitive payload");

    mode = "empty";
    await assert.rejects(store.recoverWhitelistRefreshGate("price-test", currentFilters), (error) => error?.code === "WHITELIST_REFRESH_REQUIRED");
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], expectedGate, "an empty exact result cannot clear the gate");

    mode = "wrong";
    await assert.rejects(store.recoverWhitelistRefreshGate("price-test", currentFilters), (error) => error?.code === "WHITELIST_REFRESH_REQUIRED");
    assert.ok(store.refreshWarnings[gateKey], "an identity mismatch cannot clear the warning or gate");

    mode = "stale-revision";
    await assert.rejects(store.recoverWhitelistRefreshGate("price-test", currentFilters), (error) => error?.code === "WHITELIST_REFRESH_REQUIRED");
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], expectedGate, "a revision older than the committed write cannot clear the gate");

    mode = "visible-error";
    await assert.rejects(store.recoverWhitelistRefreshGate("price-test", currentFilters), (error) => error?.code === "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE" || error?.status === 503);
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], expectedGate, "a failed visible-page refresh cannot clear a verified pinned gate");

    mode = "success";
    const recoveredPage = await store.recoverWhitelistRefreshGate("price-test", currentFilters);
    assert.deepEqual(recoveredPage.items, [visibleEntry]);
    assert.equal(store.whitelistRefreshGatesByPricePlanId["price-test"], undefined);
    assert.equal(store.refreshWarnings[gateKey], undefined);
    assert.deepEqual(store.whitelistFiltersByPricePlanId["price-test"], currentFilters);
    assert.ok(calls.some((filters) => filters.userId === committedEntry.userId && filters.page === 1 && filters.pageSize <= 200), "recovery must pin the written user independently of the visible filters");
  } finally {
    api.createWhitelistEntry = originals.create;
    api.listWhitelist = originals.list;
  }
});

test("exact whitelist entry loads scan server pages without replacing the visible list", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listWhitelist;
  const visibleEntry = { ...activeEntry, whitelistEntryId: "entry-visible", userId: "visible-user" };
  const soughtEntry = { ...activeEntry, whitelistEntryId: "entry-sought", userId: "sought-user", revision: 9 };
  store.whitelistByPricePlanId["price-test"] = [visibleEntry];
  store.whitelistFiltersByPricePlanId["price-test"] = { status: "PENDING", userId: "visible-user", page: 4, pageSize: 20 };
  const calls = [];
  try {
    api.listWhitelist = async (_pricePlanId, filters) => {
      calls.push({ ...filters });
      if (filters.page === 1) {
        return { items: [{ ...activeEntry, whitelistEntryId: "entry-other", userId: soughtEntry.userId }], total: 201, page: 1, pageSize: 200 };
      }
      return { items: [soughtEntry], total: 201, page: 2, pageSize: 200 };
    };
    const result = await store.loadWhitelistEntryExact("price-test", soughtEntry.whitelistEntryId, soughtEntry.userId);
    assert.deepEqual(result, soughtEntry);
    assert.deepEqual(calls, [
      { userId: soughtEntry.userId, page: 1, pageSize: 200 },
      { userId: soughtEntry.userId, page: 2, pageSize: 200 }
    ]);
    assert.deepEqual(store.whitelistByPricePlanId["price-test"], [visibleEntry]);
    assert.deepEqual(store.whitelistFiltersByPricePlanId["price-test"], { status: "PENDING", userId: "visible-user", page: 4, pageSize: 20 });
  } finally {
    api.listWhitelist = original;
  }
});

test("concurrent exact whitelist entry loads accept only the latest request", async () => {
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listWhitelist;
  const oldRequest = deferred();
  const newRequest = deferred();
  let call = 0;
  try {
    api.listWhitelist = async () => (++call === 1 ? oldRequest.promise : newRequest.promise);
    const oldLoad = store.loadWhitelistEntryExact("price-test", activeEntry.whitelistEntryId, activeEntry.userId).catch((error) => error);
    const newLoad = store.loadWhitelistEntryExact("price-test", activeEntry.whitelistEntryId, activeEntry.userId);
    const newest = { ...activeEntry, revision: 6 };
    newRequest.resolve({ items: [newest], total: 1, page: 1, pageSize: 200 });
    assert.deepEqual(await newLoad, newest);
    oldRequest.resolve({ items: [{ ...activeEntry, revision: 5 }], total: 1, page: 1, pageSize: 200 });
    const obsolete = await oldLoad;
    assert.equal(obsolete?.code, "WHITELIST_REFRESH_REQUIRED");
    assert.equal(store.errors[`whitelistEntryExact:price-test:${activeEntry.whitelistEntryId}`], undefined);
    assert.equal(store.loading[`whitelistEntryExact:price-test:${activeEntry.whitelistEntryId}`], false);
  } finally {
    api.listWhitelist = original;
  }
});

test("exact whitelist recovery stops at the client scan cap without clearing its gate or visible list", async () => {
  const whitelistRefreshGateKey = requiredFunction(domain, "whitelistRefreshGateKey");
  const usePricePlanAdminStore = requiredFunction(storeModule, "usePricePlanAdminStore");
  const { createPinia, setActivePinia } = await import("../admin-vue/node_modules/pinia/dist/pinia.mjs");
  setActivePinia(createPinia());
  const store = usePricePlanAdminStore();
  const api = apiModule.pricePlanAdminApi;
  const original = api.listWhitelist;
  const visibleEntry = { ...activeEntry, whitelistEntryId: "entry-visible", userId: "visible-user" };
  const gate = {
    pricePlanId: "price-test",
    whitelistEntryId: "entry-never-returned",
    userId: "user-pinned",
    revision: 8,
    status: "ACTIVE"
  };
  const gateKey = whitelistRefreshGateKey("price-test");
  store.whitelistByPricePlanId["price-test"] = [visibleEntry];
  store.whitelistFiltersByPricePlanId["price-test"] = { status: "ACTIVE", page: 2, pageSize: 20 };
  store.whitelistRefreshGatesByPricePlanId["price-test"] = gate;
  store.refreshWarnings[gateKey] = { message: "refresh failed", code: "STORE_UNAVAILABLE", status: 503 };
  const fullWrongPage = Array.from({ length: 200 }, (_, index) => ({
    ...activeEntry,
    whitelistEntryId: `entry-wrong-${index}`,
    userId: gate.userId
  }));
  let calls = 0;
  try {
    api.listWhitelist = async (_pricePlanId, filters) => {
      calls += 1;
      if (calls > 100) throw { code: "TEST_EXACT_SCAN_ESCAPED_CAP", status: 599 };
      return { items: fullWrongPage, total: -1, page: filters.page, pageSize: 200 };
    };
    await assert.rejects(
      store.recoverWhitelistRefreshGate("price-test", { status: "ACTIVE", page: 2, pageSize: 20 }),
      (error) => error?.code === "WHITELIST_REFRESH_REQUIRED"
    );
    assert.equal(calls, 100, "the client must stop after at most 100 pages / 20,000 inspected records");
    assert.deepEqual(store.whitelistRefreshGatesByPricePlanId["price-test"], gate);
    assert.ok(store.refreshWarnings[gateKey]);
    assert.deepEqual(store.whitelistByPricePlanId["price-test"], [visibleEntry]);
    assert.deepEqual(store.whitelistFiltersByPricePlanId["price-test"], { status: "ACTIVE", page: 2, pageSize: 20 });
  } finally {
    api.listWhitelist = original;
  }
});

test("three-way whitelist rebase keeps only local dirty fields and absorbs independent remote changes", () => {
  const rebaseWhitelistEditableFields = requiredFunction(domain, "rebaseWhitelistEditableFields");
  const buildWhitelistUpdateFromBaseline = requiredFunction(domain, "buildWhitelistUpdateFromBaseline");
  const original = { reason: "原原因", validFrom: "2026-07-28T00:00:00+08:00", validUntil: "2026-08-28T00:00:00+08:00" };
  const local = { ...original, reason: "本地新原因" };
  const latest = { ...original, validUntil: "2026-09-28T00:00:00+08:00" };
  const rebased = rebaseWhitelistEditableFields({ original, local, latest });
  assert.deepEqual(rebased.conflictingFields, []);
  assert.deepEqual(rebased.dirtyFields, ["reason"]);
  assert.deepEqual(rebased.form, { reason: "本地新原因", validFrom: original.validFrom, validUntil: latest.validUntil });
  assert.deepEqual(rebased.baseline, latest);
  assert.deepEqual(buildWhitelistUpdateFromBaseline({
    revision: 4,
    changeReason: "解决 revision 冲突",
    baseline: rebased.baseline,
    current: rebased.form
  }), {
    revision: 4,
    reason: "本地新原因",
    changeReason: "解决 revision 冲突"
  });
});

test("three-way whitelist rebase blocks a field changed both locally and remotely", () => {
  const rebaseWhitelistEditableFields = requiredFunction(domain, "rebaseWhitelistEditableFields");
  const resolveWhitelistFieldConflict = requiredFunction(domain, "resolveWhitelistFieldConflict");
  const original = { reason: "原原因", validFrom: "", validUntil: "2026-08-28T00:00:00+08:00" };
  const rebased = rebaseWhitelistEditableFields({
    original,
    local: { ...original, reason: "本地原因" },
    latest: { ...original, reason: "远端原因" }
  });
  assert.deepEqual(rebased.conflictingFields, ["reason"]);
  assert.equal(rebased.form.reason, "本地原因");
  assert.equal(rebased.baseline.reason, "远端原因");
  const resolvedLocal = resolveWhitelistFieldConflict(rebased, "reason", "LOCAL");
  assert.deepEqual(resolvedLocal.conflictingFields, []);
  assert.equal(resolvedLocal.form.reason, "本地原因");
  assert.equal(resolvedLocal.baseline.reason, "远端原因", "keeping local must not rewrite the remote baseline");
  assert.deepEqual(resolvedLocal.dirtyFields, ["reason"]);
  assert.deepEqual(domain.buildWhitelistUpdateFromBaseline({
    revision: 5,
    changeReason: "明确保留本地原因",
    baseline: resolvedLocal.baseline,
    current: resolvedLocal.form
  }), { revision: 5, reason: "本地原因", changeReason: "明确保留本地原因" });

  const resolvedServer = resolveWhitelistFieldConflict(rebased, "reason", "SERVER");
  assert.equal(resolvedServer.form.reason, "远端原因");
  assert.deepEqual(resolvedServer.dirtyFields, []);
});

test("Task 7 backend error codes have stable operator guidance", () => {
  for (const code of [
    "INVALID_WHITELIST_QUERY",
    "WHITELIST_USER_REQUIRED",
    "WHITELIST_ACTIVE_EXISTS",
    "WHITELIST_ENTRY_TERMINAL",
    "WHITELIST_ENTRY_PRICE_PLAN_MISMATCH",
    "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE",
    "WHITELIST_REFRESH_REQUIRED"
  ]) {
    const message = domain.pricingErrorMessage({ code });
    assert.match(message, /[\u3400-\u9fff]/, `${code} needs Chinese guidance`);
    assert.notEqual(message, "操作失败，请稍后重试");
  }
});

test("whitelist API keeps server filters, pagination, revision, and changeReason contracts", async () => {
  const { apiClient } = await import("../admin-vue/src/api/client.ts");
  const originalAdapter = apiClient.defaults.adapter;
  const requests = [];
  apiClient.defaults.adapter = async (config) => {
    requests.push({
      method: config.method?.toUpperCase(),
      url: config.url,
      data: typeof config.data === "string" ? JSON.parse(config.data) : config.data
    });
    return { data: { items: [], total: 0, page: 3, pageSize: 25, alreadyDisabled: true }, status: 200, statusText: "OK", headers: {}, config };
  };
  try {
    await apiModule.pricePlanAdminApi.listWhitelist("price/test", { status: "DISABLED", userId: " user-1 ", page: 3, pageSize: 25 });
    await apiModule.pricePlanAdminApi.createWhitelistEntry("price/test", { revision: 0, userId: "user-1", reason: "测试", changeReason: "新增" });
    await apiModule.pricePlanAdminApi.updateWhitelistEntry("price/test", "entry/1", { revision: 2, reason: "延期", changeReason: "调整" });
    await apiModule.pricePlanAdminApi.disableWhitelistEntry("price/test", "entry/1", { revision: 3, changeReason: "结束测试" });
    assert.deepEqual(requests, [
      { method: "GET", url: "/admin/price-plans/price%2Ftest/whitelist?status=DISABLED&userId=user-1&page=3&pageSize=25", data: undefined },
      { method: "POST", url: "/admin/price-plans/price%2Ftest/whitelist", data: { revision: 0, userId: "user-1", reason: "测试", changeReason: "新增" } },
      { method: "PATCH", url: "/admin/price-plans/price%2Ftest/whitelist/entry%2F1", data: { revision: 2, reason: "延期", changeReason: "调整" } },
      { method: "POST", url: "/admin/price-plans/price%2Ftest/whitelist/entry%2F1/disable", data: { revision: 3, changeReason: "结束测试" } }
    ]);
  } finally {
    apiClient.defaults.adapter = originalAdapter;
  }
});

test("Task 7 UI is wired into the governance tab and only TEST rows expose its entry", () => {
  const componentDir = new URL("../admin-vue/src/components/billing/price-plan-admin/", import.meta.url);
  const managerURL = new URL("PricePlanWhitelistManager.vue", componentDir);
  const dialogURL = new URL("PricePlanWhitelistDialog.vue", componentDir);
  assert.equal(existsSync(managerURL), true, "whitelist manager must exist");
  assert.equal(existsSync(dialogURL), true, "whitelist row dialog must exist");
  const manager = readFileSync(managerURL, "utf8");
  const dialog = readFileSync(dialogURL, "utf8");
  const priceList = readFileSync(new URL("PricePlanList.vue", componentDir), "utf8");
  const governanceView = readFileSync(new URL("PricePlanGovernance.vue", componentDir), "utf8");

  assert.ok(manager.includes(domain.TEST_WHITELIST_ORDINARY_ENTRY_NOTICE));
  assert.match(manager, /pricing:plan:view/);
  assert.match(manager, /pricing:test-whitelist:manage/);
  assert.match(manager, /value-format="YYYY-MM-DDTHH:mm:ssZ"/);
  assert.match(manager, /requiresNewEntry/);
  assert.match(manager, /formCommittedStale/);
  assert.match(manager, /disableCommittedStale/);
  assert.match(manager, /recoverWhitelistRefreshGate/);
  assert.ok((manager.match(/store\.loadWhitelistEntryExact/g) || []).length >= 2, "both revision refresh flows must load the pinned entry independently of the visible page");
  assert.doesNotMatch(manager, /const latest = entries\.value\.find\(\(item\) => item\.whitelistEntryId === formTargetId\.value\)/);
  assert.doesNotMatch(manager, /const latest = entries\.value\.find\(\(item\) => item\.whitelistEntryId === disableTargetId\.value\)/);
  assert.match(manager, /formFieldConflicts/);
  assert.match(manager, /resolveFieldConflict/);
  assert.match(manager, />使用服务器值</);
  assert.match(manager, />保留我的当前输入</);
  assert.equal((manager.match(/formCommittedStale\.value = whitelistWriteLocked\.value/g) || []).length, 1);
  assert.match(manager, /pricePlansFreshPlanId\.value !== props\.plan\.id/);
  assert.doesNotMatch(manager, /deleteWhitelist|removeWhitelist/);
  assert.match(dialog, /PricePlanWhitelistManager/);
  assert.match(governanceView, /tab\.id === ['"]testWhitelist['"]/);
  assert.match(governanceView, /PricePlanWhitelistManager/);
  assert.match(priceList, /v-if="row\.kind === 'TEST'"/);
  assert.match(priceList, /PricePlanWhitelistDialog/);
  assert.equal(governance.PRICE_PLAN_DETAIL_TABS.find((tab) => tab.id === "testWhitelist")?.ready, true);
});
