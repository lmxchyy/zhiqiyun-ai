import assert from "node:assert/strict";
import test from "node:test";

import * as walletModule from "../src/features/wallet/personalPointsWallet.ts";

type ContextType = "PERSONAL" | "ENTERPRISE" | "AGENT" | "OPERATION" | "";
type RuntimeScope = { sessionKey: string; userId: string; contextType: ContextType; tenantId: string };
type WalletState = { status: string; payload: null | { account?: { userId?: string; available?: number } } };
type Coordinator = {
  refresh(): Promise<WalletState>;
  invalidate(): void;
  snapshot(): { state: WalletState; loading: boolean };
};
type CoordinatorFactory = (input: {
  getScope(): RuntimeScope;
  storage: { get(key: string): unknown; set(key: string, value: unknown): void };
  request(): Promise<ReturnType<typeof payloadFor>>;
}) => Coordinator;

function coordinatorFactory() {
  const factory = (walletModule as Record<string, unknown>).createPersonalPointsWalletCoordinator;
  assert.equal(typeof factory, "function", "wallet coordinator must exist");
  return factory as CoordinatorFactory;
}

function payloadFor(userId: string, available = 10) {
  return {
    account: {
      id: `account-${userId}`,
      userId,
      available,
      frozen: 0,
      total: available,
      permanentAvailable: available,
      expiringAvailable: 0,
      nextExpiryPoints: 0,
    },
    transactions: [],
    orders: [],
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function countingStorage() {
  const values = new Map<string, unknown>();
  let gets = 0;
  let sets = 0;
  return {
    values,
    get gets() { return gets; },
    get sets() { return sets; },
    get(key: string) { gets += 1; return values.get(key); },
    set(key: string, value: unknown) { sets += 1; values.set(key, value); },
  };
}

test("non-personal scopes never read personal cache or request points", async () => {
  const createCoordinator = coordinatorFactory();
  const storage = countingStorage();
  let requests = 0;

  for (const contextType of ["ENTERPRISE", "AGENT", "OPERATION", ""] as ContextType[]) {
    const scope: RuntimeScope = { sessionKey: "token-a", userId: "user-a", contextType, tenantId: "tenant-a" };
    const coordinator = createCoordinator({
      getScope: () => scope,
      storage,
      request: async () => { requests += 1; return payloadFor("user-a"); },
    });
    await coordinator.refresh();
    assert.equal(coordinator.snapshot().state.status, "hidden");
  }

  assert.equal(storage.gets, 0);
  assert.equal(storage.sets, 0);
  assert.equal(requests, 0);
});

test("PERSONAL to ENTERPRISE invalidation discards deferred response without cache write", async () => {
  const createCoordinator = coordinatorFactory();
  const storage = countingStorage();
  const response = deferred<ReturnType<typeof payloadFor>>();
  const scope: RuntimeScope = { sessionKey: "token-a", userId: "user-a", contextType: "PERSONAL", tenantId: "personal" };
  const coordinator = createCoordinator({ getScope: () => scope, storage, request: () => response.promise });

  const pending = coordinator.refresh();
  scope.contextType = "ENTERPRISE";
  scope.tenantId = "tenant-enterprise";
  coordinator.invalidate();
  assert.equal(coordinator.snapshot().state.status, "hidden");
  assert.equal(coordinator.snapshot().loading, false);
  response.resolve(payloadFor("user-a", 101));
  await pending;

  assert.equal(coordinator.snapshot().state.status, "hidden");
  assert.equal(coordinator.snapshot().state.payload, null);
  assert.equal(storage.sets, 0);
});

test("user switch discards the old user's deferred response", async () => {
  const createCoordinator = coordinatorFactory();
  const storage = countingStorage();
  const response = deferred<ReturnType<typeof payloadFor>>();
  const scope: RuntimeScope = { sessionKey: "token-a", userId: "user-a", contextType: "PERSONAL", tenantId: "personal" };
  const coordinator = createCoordinator({ getScope: () => scope, storage, request: () => response.promise });

  const pending = coordinator.refresh();
  scope.userId = "user-b";
  scope.sessionKey = "token-b";
  response.resolve(payloadFor("user-a", 102));
  await pending;

  assert.equal(coordinator.snapshot().state.status, "hidden");
  assert.equal(coordinator.snapshot().loading, false);
  assert.equal(storage.sets, 0);
});

test("logout discards the authenticated deferred response", async () => {
  const createCoordinator = coordinatorFactory();
  const storage = countingStorage();
  const response = deferred<ReturnType<typeof payloadFor>>();
  const scope: RuntimeScope = { sessionKey: "token-a", userId: "user-a", contextType: "PERSONAL", tenantId: "personal" };
  const coordinator = createCoordinator({ getScope: () => scope, storage, request: () => response.promise });

  const pending = coordinator.refresh();
  scope.sessionKey = "";
  scope.userId = "";
  response.resolve(payloadFor("user-a", 103));
  await pending;

  assert.equal(coordinator.snapshot().state.status, "hidden");
  assert.equal(coordinator.snapshot().loading, false);
  assert.equal(storage.sets, 0);
});

test("consecutive retries commit only the latest deferred response", async () => {
  const createCoordinator = coordinatorFactory();
  const storage = countingStorage();
  const first = deferred<ReturnType<typeof payloadFor>>();
  const second = deferred<ReturnType<typeof payloadFor>>();
  const requests = [first.promise, second.promise];
  const scope: RuntimeScope = { sessionKey: "token-a", userId: "user-a", contextType: "PERSONAL", tenantId: "personal" };
  const coordinator = createCoordinator({ getScope: () => scope, storage, request: () => requests.shift()! });

  const firstPending = coordinator.refresh();
  const secondPending = coordinator.refresh();
  second.resolve(payloadFor("user-a", 222));
  await secondPending;
  first.resolve(payloadFor("user-a", 111));
  await firstPending;

  assert.equal(coordinator.snapshot().state.status, "ready");
  assert.equal(coordinator.snapshot().state.payload?.account?.available, 222);
  assert.equal(storage.sets, 1);
});
