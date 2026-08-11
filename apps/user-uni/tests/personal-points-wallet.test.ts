import assert from "node:assert/strict";
import test from "node:test";

import {
  createPersonalPointsWalletCacheKey,
  loadPersonalPointsWallet,
  personalPointEntryKind,
  personalPointsExpirySummary,
  type PersonalPointsWalletStorage,
} from "../src/features/wallet/personalPointsWallet.ts";

function memoryStorage(): PersonalPointsWalletStorage & { values: Map<string, unknown> } {
  const values = new Map<string, unknown>();
  return {
    values,
    get: key => values.get(key),
    set: (key, value) => values.set(key, value),
  };
}

const expiringPayload = {
  account: {
    id: "account-user-a",
    userId: "user-a",
    available: 120,
    frozen: 0,
    total: 120,
    permanentAvailable: 20,
    expiringAvailable: 100,
    nextExpiryAt: "2026-09-30T16:00:00Z",
    nextExpiryPoints: 40,
  },
  transactions: [],
  orders: [],
};

test("personal wallet cache is versioned and isolated by personal user scope", () => {
  assert.equal(createPersonalPointsWalletCacheKey("user-a", "PERSONAL"), "zhiqiyun:personal-points-wallet:v1:PERSONAL:user-a");
  assert.equal(createPersonalPointsWalletCacheKey("user-b", "PERSONAL"), "zhiqiyun:personal-points-wallet:v1:PERSONAL:user-b");
  assert.equal(createPersonalPointsWalletCacheKey("user-a", "ENTERPRISE"), null);
  assert.equal(createPersonalPointsWalletCacheKey("", "PERSONAL"), null);
});

test("enterprise context never reads cache or requests the personal wallet", async () => {
  const storage = memoryStorage();
  storage.values.set("zhiqiyun:personal-points-wallet:v1:PERSONAL:user-a", { payload: expiringPayload });
  let requests = 0;

  const state = await loadPersonalPointsWallet({
    userId: "user-a",
    contextType: "ENTERPRISE",
    storage,
    request: async () => {
      requests += 1;
      return expiringPayload;
    },
  });

  assert.equal(requests, 0);
  assert.equal(state.payload, null);
  assert.equal(state.status, "hidden");
});

test("first request failure keeps balance unknown instead of fabricating zero", async () => {
  const state = await loadPersonalPointsWallet({
    userId: "user-a",
    contextType: "PERSONAL",
    storage: memoryStorage(),
    request: async () => {
      throw new Error("network unavailable");
    },
  });

  assert.equal(state.payload, null);
  assert.equal(state.status, "error");
  assert.equal(state.stale, false);
  assert.match(state.error, /重试/);
});

test("request failure retains last successful cache and marks it explicitly stale", async () => {
  const storage = memoryStorage();
  const fresh = await loadPersonalPointsWallet({
    userId: "user-a",
    contextType: "PERSONAL",
    storage,
    now: () => 100,
    request: async () => expiringPayload,
  });
  assert.equal(fresh.status, "ready");
  assert.equal(fresh.stale, false);

  const stale = await loadPersonalPointsWallet({
    userId: "user-a",
    contextType: "PERSONAL",
    storage,
    now: () => 200,
    request: async () => {
      throw new Error("offline");
    },
  });

  assert.equal(stale.payload?.account.available, 120);
  assert.equal(stale.status, "stale");
  assert.equal(stale.stale, true);
  assert.match(stale.error, /数据可能已过期/);
});

test("cache and live payloads for another user are rejected", async () => {
  const storage = memoryStorage();
  storage.values.set("zhiqiyun:personal-points-wallet:v1:PERSONAL:user-a", {
    schemaVersion: 1,
    scope: "PERSONAL:user-a",
    storedAt: 100,
    payload: { ...expiringPayload, account: { ...expiringPayload.account, userId: "user-b" } },
  });

  const state = await loadPersonalPointsWallet({
    userId: "user-a",
    contextType: "PERSONAL",
    storage,
    request: async () => ({ ...expiringPayload, account: { ...expiringPayload.account, userId: "user-b" } }),
  });

  assert.equal(state.status, "error");
  assert.equal(state.payload, null);
});

test("expiry summary appears only for a real positive expiring balance", () => {
  assert.deepEqual(personalPointsExpirySummary(expiringPayload.account), {
    expiringPoints: 100,
    nextExpiryAt: "2026-09-30T16:00:00Z",
    nextExpiryPoints: 40,
  });
  assert.equal(personalPointsExpirySummary({
    ...expiringPayload.account,
    permanentAvailable: 120,
    expiringAvailable: 0,
    nextExpiryAt: undefined,
    nextExpiryPoints: 0,
  }), null);
});

test("GRANT and EXPIRE labels require explicit backend type or source fields", () => {
  assert.equal(personalPointEntryKind({ entryType: "GRANT", points: 10 }), "GRANT");
  assert.equal(personalPointEntryKind({ changeType: "EXPIRE", points: 10 }), "EXPIRE");
  assert.equal(personalPointEntryKind({ sourceType: "ADMIN_GIFT", points: 10 }), "GRANT");
  assert.equal(personalPointEntryKind({ type: "USAGE", delta: 10 }), null);
  assert.equal(personalPointEntryKind({ delta: 10 }), null);
  assert.equal(personalPointEntryKind({ balanceBefore: 10, balanceAfter: 20 }), null);
});
