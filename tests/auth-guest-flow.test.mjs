import test from "node:test";
import assert from "node:assert/strict";
import { createAuthGate, createAuthService, createPendingActionStore, safeInternalRedirect } from "../packages/shared-auth/dist/index.js";
import { createApiClient } from "../packages/api-client/dist/index.js";

function storageAdapter(request) {
  const values = new Map();
  return {
    platform: "web",
    getClientInfo: () => ({ platform: "web" }),
    request: request || (async () => ({ statusCode: 200, data: {} })),
    getStorage: key => values.get(key),
    setStorage: (key, value) => values.set(key, value),
    removeStorage: key => values.delete(key),
  };
}

test("pending action persists safely, expires, and executes only once", async () => {
  let now = 1_000;
  let calls = 0;
  const pending = createPendingActionStore({
    adapter: storageAdapter(),
    now: () => now,
    ttlMs: 30 * 60 * 1000,
    createId: () => "pending-1",
  });
  const saved = pending.save({
    action: "generate_image",
    route: "/creator/image",
    payload: { prompt: "safe", token: "secret", nested: { apiKey: "hidden", ratio: "1:1" } },
    resume: async () => { calls += 1; },
  });
  assert.equal(saved.payload.prompt, "safe");
  assert.equal("token" in saved.payload, false);
  assert.deepEqual(saved.payload.nested, { ratio: "1:1" });
  assert.equal(await pending.resume(), true);
  assert.equal(await pending.resume(), false);
  assert.equal(calls, 1);

  pending.save({ action: "generate_video", route: "/creator/video" });
  now += 30 * 60 * 1000 + 1;
  assert.equal(pending.get(), null);
});

test("pending action can be consumed once for an explicit post-login confirmation", () => {
  const pending = createPendingActionStore({ adapter: storageAdapter(), createId: () => "pending-confirm" });
  pending.save({ action: "generate_image", route: "/", payload: { prompt: "keep me" }, autoResume: false });
  assert.equal(pending.consume()?.id, "pending-confirm");
  assert.equal(pending.consume(), null);
  assert.equal(pending.get(), null);
});

test("safe internal redirect rejects external and protocol-relative targets", () => {
  assert.equal(safeInternalRedirect("/creator/image?draft=1#prompt"), "/creator/image?draft=1#prompt");
  assert.equal(safeInternalRedirect("https://evil.example/steal", "/"), "/");
  assert.equal(safeInternalRedirect("//evil.example/steal", "/"), "/");
  assert.equal(safeInternalRedirect("/\\evil.example/steal", "/"), "/");
});

test("auth gate coalesces concurrent login requests", async () => {
  let openCount = 0;
  let release;
  const pending = createPendingActionStore({ adapter: storageAdapter() });
  const gate = createAuthGate({
    getStatus: () => "guest",
    pendingActions: pending,
    openLogin: () => new Promise(resolve => { openCount += 1; release = resolve; }),
  });
  const first = gate.requireAuth({ action: "open_wallet", route: "/wallet" });
  const second = gate.requireAuth({ action: "open_order", route: "/orders" });
  await Promise.resolve();
  assert.equal(openCount, 1);
  release();
  await Promise.all([first, second]);
});

test("api auth modes control token attachment and 401 retry", async () => {
  const seen = [];
  let calls = 0;
  const adapter = storageAdapter(async options => {
    seen.push(options);
    calls += 1;
    if (options.url.endsWith("/retry") && calls === 3) return { statusCode: 401, data: { message: "expired" } };
    return { statusCode: 200, data: { ok: true } };
  });
  const unauthorized = [];
  const client = createApiClient({
    baseURL: "https://example.test",
    adapter,
    getToken: () => "session-token",
    onUnauthorized: context => { unauthorized.push(context); return true; },
  });

  await client.request("/public", { auth: "none" });
  assert.equal(seen[0].header.Authorization, undefined);
  await client.request("/optional", { auth: "optional" });
  assert.equal(seen[1].header.Authorization, "Bearer session-token");
  await client.request("/retry", { auth: "required" });
  assert.equal(unauthorized[0].authMode, "required");
  assert.equal(calls, 4);
});

test("concurrent 401 responses share one refresh and replay only safe requests", async () => {
  let auth;
  let refreshCalls = 0;
  const endpointCalls = new Map();
  const adapter = storageAdapter(async options => {
    if (options.url.endsWith("/api/v1/auth/refresh")) {
      refreshCalls += 1;
      await new Promise(resolve => setTimeout(resolve, 10));
      return {
        statusCode: 200,
        data: { accessToken: "new-access-token", refreshToken: "rotated-refresh-token", user: { id: "user-1" } },
      };
    }
    endpointCalls.set(options.url, (endpointCalls.get(options.url) || 0) + 1);
    if (options.header.Authorization === "Bearer old-access-token") {
      return { statusCode: 401, data: { message: "expired" } };
    }
    return { statusCode: 200, data: { ok: true } };
  });
  const client = createApiClient({
    adapter,
    getToken: () => auth?.storage.getToken() || "",
    onUnauthorized: async () => {
      await auth.refresh();
      return true;
    },
  });
  auth = createAuthService({
    adapter,
    api: client,
    tokenKey: "token",
    refreshTokenKey: "refreshToken",
    authKey: "auth",
  });
  auth.storage.setToken("old-access-token");
  auth.storage.setRefreshToken("old-refresh-token");

  const responses = await Promise.all([
    client.request("/private/profile", { auth: "required" }),
    client.request("/private/works", { auth: "required" }),
    client.request("/private/orders", { auth: "required" }),
  ]);

  assert.equal(refreshCalls, 1);
  assert.equal(auth.storage.getToken(), "new-access-token");
  assert.equal(auth.storage.getRefreshToken(), "rotated-refresh-token");
  assert.deepEqual(responses, [{ ok: true }, { ok: true }, { ok: true }]);
  assert.deepEqual([...endpointCalls.values()], [2, 2, 2]);
});

test("unsafe required POST is not automatically retried after 401", async () => {
  let calls = 0;
  const client = createApiClient({
    adapter: storageAdapter(async () => {
      calls += 1;
      return { statusCode: 401, data: { message: "expired" } };
    }),
    getToken: () => "session-token",
    onUnauthorized: () => true,
  });
  await assert.rejects(client.request("/generate", {
    method: "POST",
    auth: "required",
    retryOnUnauthorized: false,
    body: { clientRequestId: "idem-1" },
  }));
  assert.equal(calls, 1);
});

test("logout sends the refresh token for server-side revocation and clears local auth", async () => {
  const seen = [];
  const adapter = storageAdapter(async options => {
    seen.push(options);
    return { statusCode: 200, data: { ok: true } };
  });
  const client = createApiClient({ adapter, getToken: () => "access-token" });
  const auth = createAuthService({
    adapter,
    api: client,
    tokenKey: "token",
    refreshTokenKey: "refreshToken",
    authKey: "auth",
  });
  auth.storage.setToken("access-token");
  auth.storage.setRefreshToken("refresh-token");
  await auth.logout();
  assert.equal(seen.length, 1);
  assert.equal(seen[0].data.refreshToken, "refresh-token");
  assert.equal(auth.storage.getToken(), "");
  assert.equal(auth.storage.getRefreshToken(), "");
});
