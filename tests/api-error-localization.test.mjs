import test from "node:test";
import assert from "node:assert/strict";
import { createApiClient, toChineseApiErrorMessage } from "../packages/api-client/dist/index.js";

function adapter(response) {
  return {
    getClientInfo: () => ({ platform: "web" }),
    request: async () => response,
    getStorage: () => undefined,
    setStorage: () => undefined,
    removeStorage: () => undefined,
  };
}

test("keeps Chinese server messages unchanged", () => {
  assert.equal(toChineseApiErrorMessage("余额不足，请先充值", { statusCode: 400 }), "余额不足，请先充值");
});

test("maps common English API messages to Chinese", () => {
  assert.equal(toChineseApiErrorMessage("invalid credentials", { statusCode: 401 }), "账号或密码不正确");
  assert.equal(toChineseApiErrorMessage("rate limit exceeded", { statusCode: 429 }), "操作过于频繁，请稍后重试");
  assert.equal(toChineseApiErrorMessage("some internal detail", { statusCode: 500 }), "服务器处理失败，请稍后重试");
});

test("shared client never exposes a pure English server error", async () => {
  const client = createApiClient({
    adapter: adapter({ statusCode: 403, data: { error: "Access denied by policy" } }),
  });
  await assert.rejects(client.request("/private"), error => {
    assert.equal(error.message, "暂无权限执行此操作");
    assert.deepEqual(error.payload, { error: "Access denied by policy" });
    return true;
  });
});

test("business error codes receive Chinese display messages", async () => {
  const client = createApiClient({
    adapter: adapter({ statusCode: 200, data: { code: "INSUFFICIENT_CREDITS", message: "credits exhausted" } }),
  });
  await assert.rejects(client.request("/generate"), {
    message: "可用额度不足，请充值或升级套餐",
  });
});

test("insufficient points code survives non-2xx responses with actionable details", async () => {
  const client = createApiClient({
    adapter: adapter({
      statusCode: 400,
      data: {
        code: "INSUFFICIENT_POINTS",
        message: "积分不足",
        currentPoints: 10,
        requiredPoints: 80,
        shortfall: 70,
      },
    }),
  });
  await assert.rejects(client.request("/generate"), error => {
    assert.equal(error.message, "积分不足，当前 10 积分，本次需要 80 积分，还差 70 积分。");
    assert.equal(error.apiCode, "INSUFFICIENT_POINTS");
    return true;
  });
});
